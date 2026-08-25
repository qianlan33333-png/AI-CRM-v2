package migration

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	defaultStreamLimit = 500
	defaultLeaseTTL    = time.Minute
)

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type Service struct {
	target     TargetUnitOfWork
	runs       RunStore
	receipts   RowReceiptStore
	results    ResultReceiptStore
	writer     TargetWriter
	quarantine QuarantineWriter
	archive    ArchiveWriter
	mappings   MappingRegistry
	policies   PolicyRegistry
	clock      Clock
	leaseTTL   time.Duration
	limit      int
}

func NewService(target TargetUnitOfWork, runs RunStore, receipts RowReceiptStore, results ResultReceiptStore, writer TargetWriter, quarantine QuarantineWriter, archive ArchiveWriter, mappings MappingRegistry, policies PolicyRegistry) *Service {
	return NewServiceWithClock(systemClock{}, target, runs, receipts, results, writer, quarantine, archive, mappings, policies)
}

func NewServiceWithClock(clock Clock, target TargetUnitOfWork, runs RunStore, receipts RowReceiptStore, results ResultReceiptStore, writer TargetWriter, quarantine QuarantineWriter, archive ArchiveWriter, mappings MappingRegistry, policies PolicyRegistry) *Service {
	return &Service{target: target, runs: runs, receipts: receipts, results: results, writer: writer, quarantine: quarantine, archive: archive, mappings: mappings, policies: policies, clock: clock, leaseTTL: defaultLeaseTTL, limit: defaultStreamLimit}
}

type RunRequest struct {
	ID      RunID
	Adapter AdapterID
}

type RunResult struct {
	Imported, Archived, Quarantined, Skipped, Rebuilt, Reset, Replayed int
}

func (service *Service) Run(ctx context.Context, request RunRequest) (result RunResult, err error) {
	if service == nil || ctx == nil || request.ID == "" || request.Adapter == "" || service.target == nil || service.runs == nil || service.receipts == nil || service.results == nil || service.writer == nil || service.quarantine == nil || service.archive == nil || service.mappings == nil || service.policies == nil || service.clock == nil || service.leaseTTL <= 0 || service.limit < 1 {
		return RunResult{}, ErrInvalidRun
	}
	definition, found := service.mappings.Lookup(request.Adapter)
	if !found {
		return RunResult{}, ErrUnknownAdapter
	}
	manifest := definition.Manifest
	if manifest.ID != request.Adapter || definition.Source == nil || definition.Mapper == nil || definition.Cursors == nil {
		return RunResult{}, ErrInvalidManifest
	}
	if err := manifest.Validate(); err != nil {
		return RunResult{}, err
	}
	for _, table := range manifest.Tables {
		policy, found := service.policies.Lookup(table.Policy)
		if !found || !policy.valid() {
			return RunResult{}, ErrUnknownPolicy
		}
	}
	preflight, err := definition.Source.Preflight(ctx)
	if err != nil {
		return RunResult{}, err
	}
	if preflight.Identity != manifest.SourceIdentity || preflight.SchemaDigest != manifest.SourceSchemaDigest {
		return RunResult{}, ErrSourceDrift
	}
	bounds, err := fixedBounds(manifest, preflight)
	if err != nil {
		return RunResult{}, err
	}
	manifestDigest, err := manifest.Digest()
	if err != nil {
		return RunResult{}, err
	}
	var state RunState
	var fence LeaseFence
	err = service.target.Within(ctx, func(tx context.Context) error {
		state, err = service.runs.Open(tx, StartRun{ID: request.ID, Adapter: request.Adapter, ManifestDigest: manifestDigest, Bounds: bounds})
		if err != nil {
			return err
		}
		fence, err = service.runs.AcquireLease(tx, request.ID, service.clock.Now(), service.leaseTTL)
		return err
	})
	if err != nil {
		return RunResult{}, err
	}
	for _, table := range manifest.Tables {
		checkpoint, exists := state.Tables[table.ID]
		if !exists {
			return RunResult{}, ErrInvalidRun
		}
		if checkpoint.Complete {
			continue
		}
		if err := service.runTable(ctx, definition, table, checkpoint, fence, &result); err != nil {
			return RunResult{}, err
		}
	}
	err = service.target.Within(ctx, func(tx context.Context) error { return service.runs.Finish(tx, fence) })
	if err != nil {
		return RunResult{}, err
	}
	return result, nil
}

func fixedBounds(manifest AdapterManifest, preflight SourcePreflight) ([]TableBound, error) {
	if len(preflight.Bounds) != len(manifest.Tables) {
		return nil, ErrSourceDrift
	}
	byTable := make(map[TableID]TableBound, len(preflight.Bounds))
	for _, bound := range preflight.Bounds {
		if bound.Table == "" || !bound.UpperBound.valid() {
			return nil, ErrSourceDrift
		}
		if _, exists := byTable[bound.Table]; exists {
			return nil, ErrSourceDrift
		}
		bound.Value = append([]byte(nil), bound.Value...)
		byTable[bound.Table] = bound
	}
	result := make([]TableBound, 0, len(manifest.Tables))
	for _, table := range manifest.Tables {
		bound, found := byTable[table.ID]
		if !found || bound.SchemaDigest != table.SchemaDigest {
			return nil, ErrSourceDrift
		}
		result = append(result, TableBound{Table: table.ID, SchemaDigest: bound.SchemaDigest, UpperBound: bound.UpperBound})
	}
	return result, nil
}

func (service *Service) runTable(ctx context.Context, definition AdapterDefinition, table TableSpec, checkpoint TableCheckpoint, fence LeaseFence, result *RunResult) error {
	for !checkpoint.Complete {
		seen := 0
		streamed, err := definition.Source.Stream(ctx, StreamRequest{Table: table, UpperBound: checkpoint.UpperBound, After: checkpoint.Cursor, Limit: service.limit}, func(row SourceRow) error {
			seen++
			if seen > service.limit || !row.valid() {
				return ErrUnboundedStream
			}
			if checkpoint.Cursor != "" {
				comparison, err := definition.Cursors.Compare(checkpoint.Cursor, row.Cursor)
				if err != nil || comparison >= 0 {
					return ErrUnboundedStream
				}
			}
			policy, found := service.policies.Lookup(table.Policy)
			if !found || !policy.valid() {
				return ErrUnknownPolicy
			}
			next := checkpoint
			next.Cursor = row.Cursor
			err := service.target.Within(ctx, func(tx context.Context) error {
				recorded, err := service.applyRow(tx, definition, table, policy, fence, row, result)
				if err != nil {
					return err
				}
				if recorded {
					next.Processed++
				}
				return service.runs.Advance(tx, fence, table.ID, next)
			})
			if err == nil {
				checkpoint = next
			}
			return err
		})
		if err != nil {
			return err
		}
		if seen == 0 && !streamed.Complete {
			return ErrUnboundedStream
		}
		checkpoint.Complete = streamed.Complete
		if err := service.target.Within(ctx, func(tx context.Context) error { return service.runs.Advance(tx, fence, table.ID, checkpoint) }); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) applyRow(ctx context.Context, definition AdapterDefinition, table TableSpec, policy Policy, fence LeaseFence, row SourceRow, result *RunResult) (bool, error) {
	if !fence.valid() {
		return false, ErrLeaseFenced
	}
	receipt := RowReceipt{Adapter: definition.Manifest.ID, Table: table.ID, SourceKey: Sum(row.SourceKey), Payload: Sum(row.Payload), Field: row.FieldDigest, Disposition: policy.Disposition}
	previous, found, err := service.receipts.FindRowReceipt(ctx, receipt.Adapter, receipt.Table, receipt.SourceKey)
	if err != nil {
		return false, err
	}
	if found {
		if previous.Payload != receipt.Payload || previous.Field != receipt.Field || previous.Disposition != receipt.Disposition {
			return false, ErrSourcePayloadConflict
		}
		if previous.Mutation == (Digest{}) {
			return false, ErrTargetTampered
		}
		result.Replayed++
		return service.appendRunReceipt(ctx, fence, previous)
	}
	var mutation Digest
	switch policy.Disposition {
	case DispositionImport, DispositionRebuild, DispositionReset:
		mapped, err := definition.Mapper.Map(ctx, table, row)
		if err != nil {
			return false, err
		}
		if mapped.Operation == "" || mapped.Digest == (Digest{}) {
			return false, ErrInvalidRun
		}
		if err := service.writer.Apply(ctx, fence, mapped); err != nil {
			return false, err
		}
		mutation = mapped.Digest
	case DispositionArchive:
		if err := service.archive.Archive(ctx, fence, Archive{Adapter: receipt.Adapter, Table: receipt.Table, SourceKey: receipt.SourceKey, Payload: receipt.Payload, Field: receipt.Field}); err != nil {
			return false, err
		}
	case DispositionQuarantine:
		if err := service.quarantine.Quarantine(ctx, fence, Quarantine{Adapter: receipt.Adapter, Table: receipt.Table, SourceKey: receipt.SourceKey, Payload: receipt.Payload, Field: receipt.Field, Reason: "policy"}); err != nil {
			return false, err
		}
	case DispositionSkip:
	default:
		return false, fmt.Errorf("%w: %s", ErrUnknownPolicy, policy.Disposition)
	}
	receipt.Mutation = mutation
	if mutation == (Digest{}) {
		receipt.Mutation = receiptMutation(receipt)
	}
	if err := service.receipts.AppendRowReceipt(ctx, fence, receipt); err != nil {
		return false, err
	}
	recorded, err := service.appendRunReceipt(ctx, fence, receipt)
	if err != nil {
		return false, err
	}
	switch policy.Disposition {
	case DispositionImport:
		result.Imported++
	case DispositionArchive:
		result.Archived++
	case DispositionQuarantine:
		result.Quarantined++
	case DispositionSkip:
		result.Skipped++
	case DispositionRebuild:
		result.Rebuilt++
	case DispositionReset:
		result.Reset++
	}
	return recorded, nil
}

func receiptMutation(receipt RowReceipt) Digest {
	value := append([]byte(string(receipt.Adapter)+"\x00"+string(receipt.Table)+"\x00"+string(receipt.Disposition)+"\x00"), receipt.SourceKey[:]...)
	value = append(value, receipt.Payload[:]...)
	value = append(value, receipt.Field[:]...)
	return Sum(value)
}

func (service *Service) appendRunReceipt(ctx context.Context, fence LeaseFence, receipt RowReceipt) (bool, error) {
	existing, found, err := service.results.FindResultReceipt(ctx, fence.RunID, receipt.Adapter, receipt.Table, receipt.SourceKey)
	if err != nil {
		return false, err
	}
	desired := ResultReceipt{RunID: fence.RunID, RowReceipt: receipt, Outcome: receipt.Disposition, MutationDigest: receipt.Mutation}
	if found {
		if existing != desired {
			return false, ErrTargetTampered
		}
		return false, nil
	}
	if err := service.results.AppendResultReceipt(ctx, fence, desired); err != nil {
		return false, err
	}
	return true, nil
}

type ReconcileService struct {
	target   TargetUnitOfWork
	runs     RunStore
	results  ResultReceiptStore
	verifier TargetVerifier
	clock    Clock
	leaseTTL time.Duration
}

func NewReconcileService(target TargetUnitOfWork, runs RunStore, results ResultReceiptStore, verifier TargetVerifier) *ReconcileService {
	return NewReconcileServiceWithClock(systemClock{}, target, runs, results, verifier)
}

func NewReconcileServiceWithClock(clock Clock, target TargetUnitOfWork, runs RunStore, results ResultReceiptStore, verifier TargetVerifier) *ReconcileService {
	return &ReconcileService{target: target, runs: runs, results: results, verifier: verifier, clock: clock, leaseTTL: defaultLeaseTTL}
}

func (service *ReconcileService) Reconcile(ctx context.Context, runID RunID) error {
	if service == nil || ctx == nil || runID == "" || service.target == nil || service.runs == nil || service.results == nil || service.verifier == nil || service.clock == nil || service.leaseTTL <= 0 {
		return ErrInvalidRun
	}
	return service.target.Within(ctx, func(tx context.Context) error {
		state, err := service.runs.Load(tx, runID)
		if err != nil {
			return err
		}
		if state.Phase != PhaseCompleted {
			return ErrTargetTampered
		}
		fence, err := service.runs.AcquireLease(tx, runID, service.clock.Now(), service.leaseTTL)
		if err != nil {
			return err
		}
		receipts, err := service.results.ListResultReceipts(tx, runID)
		if err != nil {
			return err
		}
		expected := uint64(0)
		for _, checkpoint := range state.Tables {
			expected += checkpoint.Processed
		}
		if uint64(len(receipts)) != expected {
			return ErrTargetTampered
		}
		seen := make(map[string]struct{}, len(receipts))
		for _, receipt := range receipts {
			key := resultReceiptKey(receipt.Adapter, receipt.Table, receipt.SourceKey)
			if receipt.RunID != runID || receipt.Outcome != receipt.Disposition || receipt.Mutation == (Digest{}) || receipt.MutationDigest != receipt.Mutation {
				return ErrTargetTampered
			}
			if _, duplicate := seen[key]; duplicate {
				return ErrTargetTampered
			}
			seen[key] = struct{}{}
			if err := service.verifier.VerifyResultReceipt(tx, receipt); err != nil {
				if errors.Is(err, ErrTargetTampered) {
					return err
				}
				return fmt.Errorf("verify migration receipt: %w", err)
			}
		}
		return service.runs.MarkReconciled(tx, fence)
	})
}

func resultReceiptKey(adapter AdapterID, table TableID, sourceKey Digest) string {
	return string(adapter) + "\x00" + string(table) + "\x00" + string(sourceKey[:])
}
