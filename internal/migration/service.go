package migration

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
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
	return NewServiceWithRuntime(clock, defaultLeaseTTL, defaultStreamLimit, target, runs, receipts, results, writer, quarantine, archive, mappings, policies)
}

func NewServiceWithRuntime(clock Clock, leaseTTL time.Duration, limit int, target TargetUnitOfWork, runs RunStore, receipts RowReceiptStore, results ResultReceiptStore, writer TargetWriter, quarantine QuarantineWriter, archive ArchiveWriter, mappings MappingRegistry, policies PolicyRegistry) *Service {
	return &Service{target: target, runs: runs, receipts: receipts, results: results, writer: writer, quarantine: quarantine, archive: archive, mappings: mappings, policies: policies, clock: clock, leaseTTL: leaseTTL, limit: limit}
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
		if definition.Mapper.MappingDigest(table.ID) != table.MappingDigest {
			return RunResult{}, ErrSourceDrift
		}
		policy, found := service.policies.Lookup(table.Policy)
		if !found || !policy.valid() {
			return RunResult{}, ErrUnknownPolicy
		}
		policyDigest, digestErr := policy.Digest()
		if digestErr != nil || policyDigest != table.PolicyDigest {
			return RunResult{}, ErrSourceDrift
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
		state, err = service.runs.Open(tx, StartRun{
			ID: request.ID, Adapter: request.Adapter,
			SourceIdentity: manifest.SourceIdentity, SourceSchemaDigest: manifest.SourceSchemaDigest,
			ManifestDigest: manifestDigest, Bounds: bounds,
		})
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
		fence, err = service.runTable(ctx, definition, table, checkpoint, fence, &result)
		if err != nil {
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
		if bound.Table == "" || bound.SourceIdentity == "" || !bound.UpperBound.valid() {
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
		if !found || bound.SourceIdentity != table.SourceIdentity || bound.SchemaDigest != table.SchemaDigest {
			return nil, ErrSourceDrift
		}
		result = append(result, TableBound{Table: table.ID, SourceIdentity: bound.SourceIdentity, SchemaDigest: bound.SchemaDigest, UpperBound: bound.UpperBound})
	}
	return result, nil
}

func (service *Service) runTable(ctx context.Context, definition AdapterDefinition, table TableSpec, checkpoint TableCheckpoint, fence LeaseFence, result *RunResult) (LeaseFence, error) {
	for !checkpoint.Complete {
		var err error
		err = service.target.Within(ctx, func(tx context.Context) error {
			fence, err = service.runs.RenewLease(tx, fence, service.clock.Now(), service.leaseTTL)
			return err
		})
		if err != nil {
			return LeaseFence{}, err
		}
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
			var recorded, replayed bool
			var disposition Disposition
			err := service.target.Within(ctx, func(tx context.Context) error {
				var applyErr error
				recorded, replayed, disposition, applyErr = service.applyRow(tx, definition, table, policy, fence, row)
				if applyErr != nil {
					return applyErr
				}
				if recorded {
					next.Processed++
				}
				return service.runs.Advance(tx, fence, table.ID, next)
			})
			if err == nil {
				checkpoint = next
				if replayed {
					result.Replayed++
				} else if recorded {
					incrementRunResult(result, disposition)
				}
			}
			return err
		})
		if err != nil {
			return LeaseFence{}, err
		}
		if seen == 0 && !streamed.Complete {
			return LeaseFence{}, ErrUnboundedStream
		}
		checkpoint.Complete = streamed.Complete
		if err := service.target.Within(ctx, func(tx context.Context) error { return service.runs.Advance(tx, fence, table.ID, checkpoint) }); err != nil {
			return LeaseFence{}, err
		}
	}
	return fence, nil
}

func (service *Service) applyRow(ctx context.Context, definition AdapterDefinition, table TableSpec, policy Policy, fence LeaseFence, row SourceRow) (bool, bool, Disposition, error) {
	if !fence.valid() {
		return false, false, "", ErrLeaseFenced
	}
	receipt := RowReceipt{Adapter: definition.Manifest.ID, Table: table.ID, SourceKey: Sum(row.SourceKey), Payload: Sum(row.Payload), Field: row.FieldDigest, Disposition: policy.Disposition, Mapping: table.MappingDigest, Policy: table.PolicyDigest}
	previous, found, err := service.receipts.FindRowReceipt(ctx, receipt.Adapter, receipt.Table, receipt.SourceKey)
	if err != nil {
		return false, false, "", err
	}
	if found {
		if previous.Payload != receipt.Payload || previous.Field != receipt.Field || previous.Disposition != receipt.Disposition || previous.Mapping != receipt.Mapping || previous.Policy != receipt.Policy {
			return false, false, "", ErrSourcePayloadConflict
		}
		if previous.Mutation == (Digest{}) {
			return false, false, "", ErrTargetTampered
		}
		recorded, appendErr := service.appendRunReceipt(ctx, fence, previous)
		return recorded, true, previous.Disposition, appendErr
	}
	var mutation Digest
	switch policy.Disposition {
	case DispositionImport, DispositionRebuild, DispositionReset:
		mapped, err := definition.Mapper.Map(ctx, table, row)
		if err != nil {
			return false, false, "", err
		}
		if mapped.Operation == "" || mapped.Digest == (Digest{}) {
			return false, false, "", ErrInvalidRun
		}
		if err := service.writer.Apply(ctx, fence, mapped); err != nil {
			return false, false, "", err
		}
		receipt.Operation = mapped.Operation
		mutation = mapped.Digest
	case DispositionArchive:
		if err := service.archive.Archive(ctx, fence, Archive{Adapter: receipt.Adapter, Table: receipt.Table, SourceKey: receipt.SourceKey, Payload: receipt.Payload, Field: receipt.Field}); err != nil {
			return false, false, "", err
		}
	case DispositionQuarantine:
		if err := service.quarantine.Quarantine(ctx, fence, Quarantine{Adapter: receipt.Adapter, Table: receipt.Table, SourceKey: receipt.SourceKey, Payload: receipt.Payload, Field: receipt.Field, Reason: "policy"}); err != nil {
			return false, false, "", err
		}
	case DispositionSkip:
	default:
		return false, false, "", fmt.Errorf("%w: %s", ErrUnknownPolicy, policy.Disposition)
	}
	receipt.Mutation = mutation
	if mutation == (Digest{}) {
		receipt.Mutation = receiptMutation(receipt)
	}
	if err := service.receipts.AppendRowReceipt(ctx, fence, receipt); err != nil {
		return false, false, "", err
	}
	recorded, err := service.appendRunReceipt(ctx, fence, receipt)
	if err != nil {
		return false, false, "", err
	}
	return recorded, false, policy.Disposition, nil
}

func incrementRunResult(result *RunResult, disposition Disposition) {
	if result == nil {
		return
	}
	switch disposition {
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
}

func receiptMutation(receipt RowReceipt) Digest {
	value := append([]byte(string(receipt.Adapter)+"\x00"+string(receipt.Table)+"\x00"+string(receipt.Disposition)+"\x00"+receipt.Operation+"\x00"), receipt.SourceKey[:]...)
	value = append(value, receipt.Payload[:]...)
	value = append(value, receipt.Field[:]...)
	value = append(value, receipt.Mapping[:]...)
	value = append(value, receipt.Policy[:]...)
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
	target          TargetUnitOfWork
	runs            RunStore
	receipts        RowReceiptStore
	results         ResultReceiptStore
	reconciliations ReconciliationReceiptStore
	verifier        TargetVerifier
	mappings        MappingRegistry
	policies        PolicyRegistry
	clock           Clock
	leaseTTL        time.Duration
	limit           int
}

func NewFullReconcileService(target TargetUnitOfWork, runs RunStore, receipts RowReceiptStore, results ResultReceiptStore, reconciliations ReconciliationReceiptStore, verifier TargetVerifier, mappings MappingRegistry, policies PolicyRegistry) *ReconcileService {
	return NewFullReconcileServiceWithClock(systemClock{}, target, runs, receipts, results, reconciliations, verifier, mappings, policies)
}

func NewFullReconcileServiceWithClock(clock Clock, target TargetUnitOfWork, runs RunStore, receipts RowReceiptStore, results ResultReceiptStore, reconciliations ReconciliationReceiptStore, verifier TargetVerifier, mappings MappingRegistry, policies PolicyRegistry) *ReconcileService {
	return NewFullReconcileServiceWithRuntime(clock, defaultLeaseTTL, defaultStreamLimit, target, runs, receipts, results, reconciliations, verifier, mappings, policies)
}

func NewFullReconcileServiceWithRuntime(clock Clock, leaseTTL time.Duration, limit int, target TargetUnitOfWork, runs RunStore, receipts RowReceiptStore, results ResultReceiptStore, reconciliations ReconciliationReceiptStore, verifier TargetVerifier, mappings MappingRegistry, policies PolicyRegistry) *ReconcileService {
	return &ReconcileService{target: target, runs: runs, receipts: receipts, results: results, reconciliations: reconciliations, verifier: verifier, mappings: mappings, policies: policies, clock: clock, leaseTTL: leaseTTL, limit: limit}
}

func (service *ReconcileService) Reconcile(ctx context.Context, runID RunID) error {
	if service == nil || ctx == nil || runID == "" || service.target == nil || service.runs == nil || service.receipts == nil || service.results == nil || service.reconciliations == nil || service.verifier == nil || service.mappings == nil || service.policies == nil || service.clock == nil || service.leaseTTL <= 0 || service.limit < 1 {
		return ErrInvalidRun
	}
	state, err := service.runs.Load(ctx, runID)
	if err != nil {
		return err
	}
	if state.Phase != PhaseCompleted {
		return ErrTargetTampered
	}
	var fence LeaseFence
	if err = service.target.Within(ctx, func(tx context.Context) error {
		fence, err = service.runs.AcquireLease(tx, runID, service.clock.Now(), service.leaseTTL)
		return err
	}); err != nil {
		return err
	}

	comparison := sha256.New()
	fence, sourceRows, targetVerified, err := service.reconcileSource(ctx, state, fence, comparison)
	if err != nil {
		return err
	}
	receipts, err := service.results.ListResultReceipts(ctx, runID)
	if err != nil {
		return err
	}
	if uint64(len(receipts)) != sourceRows || targetVerified != sourceRows {
		return ErrTargetTampered
	}
	var comparisonDigest Digest
	copy(comparisonDigest[:], comparison.Sum(nil))
	return service.target.Within(ctx, func(tx context.Context) error {
		if err := service.reconciliations.AppendReconciliationReceipt(tx, fence, ReconciliationReceipt{
			RunID: runID, SourceRowCount: sourceRows, ResultRowCount: uint64(len(receipts)),
			TargetVerifiedCount: targetVerified, ComparisonDigest: comparisonDigest,
		}); err != nil {
			return err
		}
		return service.runs.MarkReconciled(tx, fence)
	})
}

func (service *ReconcileService) reconcileSource(ctx context.Context, state RunState, fence LeaseFence, comparison hash.Hash) (LeaseFence, uint64, uint64, error) {
	definition, found := service.mappings.Lookup(state.Adapter)
	if !found || definition.Source == nil || definition.Mapper == nil || definition.Cursors == nil {
		return LeaseFence{}, 0, 0, ErrUnknownAdapter
	}
	manifestDigest, err := definition.Manifest.Digest()
	if err != nil || manifestDigest != state.ManifestDigest || definition.Manifest.SourceIdentity != state.SourceIdentity || definition.Manifest.SourceSchemaDigest != state.SourceSchemaDigest {
		return LeaseFence{}, 0, 0, ErrSourceDrift
	}
	for _, table := range definition.Manifest.Tables {
		if definition.Mapper.MappingDigest(table.ID) != table.MappingDigest {
			return LeaseFence{}, 0, 0, ErrSourceDrift
		}
	}
	preflight, err := definition.Source.Preflight(ctx)
	if err != nil {
		return LeaseFence{}, 0, 0, err
	}
	if preflight.Identity != state.SourceIdentity || preflight.SchemaDigest != state.SourceSchemaDigest {
		return LeaseFence{}, 0, 0, ErrSourceDrift
	}
	bounds, err := fixedBounds(definition.Manifest, preflight)
	if err != nil {
		return LeaseFence{}, 0, 0, err
	}
	boundByTable := make(map[TableID]UpperBound, len(bounds))
	for _, bound := range bounds {
		boundByTable[bound.Table] = bound.UpperBound
	}
	var total, verified uint64
	for _, table := range definition.Manifest.Tables {
		checkpoint, found := state.Tables[table.ID]
		bound, boundFound := boundByTable[table.ID]
		if !found || !boundFound || checkpoint.SourceIdentity != table.SourceIdentity || checkpoint.SchemaDigest != table.SchemaDigest || !checkpoint.Complete || !sameUpperBound(checkpoint.UpperBound, bound) {
			return LeaseFence{}, 0, 0, ErrSourceDrift
		}
		policy, found := service.policies.Lookup(table.Policy)
		if !found || !policy.valid() {
			return LeaseFence{}, 0, 0, ErrUnknownPolicy
		}
		policyDigest, digestErr := policy.Digest()
		if digestErr != nil || policyDigest != table.PolicyDigest {
			return LeaseFence{}, 0, 0, ErrSourceDrift
		}
		var after Cursor
		var tableRows uint64
		for {
			if err := service.target.Within(ctx, func(tx context.Context) error {
				fence, err = service.runs.RenewLease(tx, fence, service.clock.Now(), service.leaseTTL)
				return err
			}); err != nil {
				return LeaseFence{}, 0, 0, err
			}
			seen := 0
			streamed, streamErr := definition.Source.Stream(ctx, StreamRequest{Table: table, UpperBound: checkpoint.UpperBound, After: after, Limit: service.limit}, func(row SourceRow) error {
				seen++
				if seen > service.limit || !row.valid() {
					return ErrUnboundedStream
				}
				if after != "" {
					order, compareErr := definition.Cursors.Compare(after, row.Cursor)
					if compareErr != nil || order >= 0 {
						return ErrUnboundedStream
					}
				}
				desired, buildErr := expectedReceipt(ctx, definition, table, policy, row)
				if buildErr != nil {
					return buildErr
				}
				var resultReceipt ResultReceipt
				verifyErr := service.target.Within(ctx, func(tx context.Context) error {
					stored, receiptFound, findErr := service.receipts.FindRowReceipt(tx, desired.Adapter, desired.Table, desired.SourceKey)
					if findErr != nil {
						return findErr
					}
					if !receiptFound || stored != desired {
						return ErrTargetTampered
					}
					result, resultFound, findErr := service.results.FindResultReceipt(tx, state.ID, desired.Adapter, desired.Table, desired.SourceKey)
					if findErr != nil {
						return findErr
					}
					expected := ResultReceipt{RunID: state.ID, RowReceipt: desired, Outcome: desired.Disposition, MutationDigest: desired.Mutation}
					if !resultFound || result != expected {
						return ErrTargetTampered
					}
					resultReceipt = result
					if err := service.verifier.VerifyResultReceipt(tx, result); err != nil {
						return err
					}
					return nil
				})
				if verifyErr != nil {
					return verifyErr
				}
				if err := writeComparisonReceipt(comparison, resultReceipt); err != nil {
					return err
				}
				after = row.Cursor
				tableRows++
				total++
				verified++
				return nil
			})
			if streamErr != nil {
				if errors.Is(streamErr, ErrTargetTampered) {
					return LeaseFence{}, 0, 0, streamErr
				}
				return LeaseFence{}, 0, 0, fmt.Errorf("reconcile migration source: %w", streamErr)
			}
			if seen == 0 && !streamed.Complete {
				return LeaseFence{}, 0, 0, ErrUnboundedStream
			}
			if streamed.Complete {
				break
			}
		}
		if tableRows != checkpoint.Processed || (tableRows > 0 && after != checkpoint.Cursor) || (tableRows == 0 && checkpoint.Cursor != "") {
			return LeaseFence{}, 0, 0, ErrTargetTampered
		}
	}
	return fence, total, verified, nil
}

func expectedReceipt(ctx context.Context, definition AdapterDefinition, table TableSpec, policy Policy, row SourceRow) (RowReceipt, error) {
	receipt := RowReceipt{Adapter: definition.Manifest.ID, Table: table.ID, SourceKey: Sum(row.SourceKey), Payload: Sum(row.Payload), Field: row.FieldDigest, Disposition: policy.Disposition, Mapping: table.MappingDigest, Policy: table.PolicyDigest}
	if policy.Disposition == DispositionImport || policy.Disposition == DispositionRebuild || policy.Disposition == DispositionReset {
		mapped, err := definition.Mapper.Map(ctx, table, row)
		if err != nil {
			return RowReceipt{}, err
		}
		if mapped.Operation == "" || mapped.Digest == (Digest{}) {
			return RowReceipt{}, ErrInvalidRun
		}
		receipt.Operation = mapped.Operation
		receipt.Mutation = mapped.Digest
	} else {
		receipt.Mutation = receiptMutation(receipt)
	}
	return receipt, nil
}

func sameUpperBound(left, right UpperBound) bool {
	if left.Empty != right.Empty || len(left.Value) != len(right.Value) {
		return false
	}
	for index := range left.Value {
		if left.Value[index] != right.Value[index] {
			return false
		}
	}
	return true
}

func writeComparisonReceipt(target hash.Hash, receipt ResultReceipt) error {
	if target == nil {
		return ErrInvalidRun
	}
	_, err := target.Write([]byte(resultReceiptKey(receipt.Adapter, receipt.Table, receipt.SourceKey) + "\x00" + string(receipt.Payload[:]) + string(receipt.Field[:]) + string(receipt.Mapping[:]) + string(receipt.Policy[:]) + string(receipt.Mutation[:]) + string(receipt.Disposition) + "\x00" + receipt.Operation))
	return err
}

func resultReceiptKey(adapter AdapterID, table TableID, sourceKey Digest) string {
	return string(adapter) + "\x00" + string(table) + "\x00" + string(sourceKey[:])
}
