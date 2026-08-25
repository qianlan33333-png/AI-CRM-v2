package migration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	preflight SourcePreflight
	rows      map[TableID][]SourceRow
	overLimit bool
}

func (source *fakeSource) Preflight(context.Context) (SourcePreflight, error) {
	return source.preflight, nil
}

func (source *fakeSource) Stream(_ context.Context, request StreamRequest, each func(SourceRow) error) (StreamResult, error) {
	rows := source.rows[request.Table.ID]
	start := 0
	if request.After != "" {
		for start < len(rows) && rows[start].Cursor != request.After {
			start++
		}
		if start == len(rows) {
			return StreamResult{}, ErrUnboundedStream
		}
		start++
	}
	limit := request.Limit
	if source.overLimit {
		limit++
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	for _, row := range rows[start:end] {
		if err := each(row); err != nil {
			return StreamResult{}, err
		}
	}
	return StreamResult{Complete: end == len(rows)}, nil
}

type fakeMapper struct{}

func (fakeMapper) Map(_ context.Context, table TableSpec, row SourceRow) (MappedRow, error) {
	return MappedRow{Operation: "apply_" + string(table.ID), Payload: append([]byte(nil), row.Payload...), Digest: Sum(append([]byte(table.ID), row.Payload...))}, nil
}

type lexicalCursors struct{}

func (lexicalCursors) Compare(left, right Cursor) (int, error) {
	switch {
	case left < right:
		return -1, nil
	case left > right:
		return 1, nil
	default:
		return 0, nil
	}
}

type fakeClock struct{ now time.Time }

func (clock *fakeClock) Now() time.Time { return clock.now }

func (clock *fakeClock) Advance(duration time.Duration) { clock.now = clock.now.Add(duration) }

type fakeRegistry map[AdapterID]AdapterDefinition

func (registry fakeRegistry) Lookup(id AdapterID) (AdapterDefinition, bool) {
	definition, found := registry[id]
	return definition, found
}

type fakePolicies map[PolicyID]Policy

func (registry fakePolicies) Lookup(id PolicyID) (Policy, bool) {
	policy, found := registry[id]
	return policy, found
}

type fakeTarget struct {
	mu            sync.Mutex
	clock         *fakeClock
	runs          map[RunID]RunState
	leases        map[RunID]LeaseFence
	receipts      map[string]RowReceipt
	results       []ResultReceipt
	imports       int
	archives      int
	quarantines   int
	failApplyAt   int
	applyAttempts int
	fenceOnApply  bool
	failReceipt   bool
	failResult    bool
	tampered      bool
}

func newFakeTarget(clock *fakeClock) *fakeTarget {
	return &fakeTarget{clock: clock, runs: map[RunID]RunState{}, leases: map[RunID]LeaseFence{}, receipts: map[string]RowReceipt{}}
}

func (target *fakeTarget) Within(ctx context.Context, fn func(context.Context) error) error {
	target.mu.Lock()
	defer target.mu.Unlock()
	snapshot := target.snapshot()
	if err := fn(ctx); err != nil {
		target.restore(snapshot)
		return err
	}
	return nil
}

func (target *fakeTarget) Open(_ context.Context, start StartRun) (RunState, error) {
	if state, found := target.runs[start.ID]; found {
		if state.Adapter != start.Adapter || state.ManifestDigest != start.ManifestDigest {
			return RunState{}, ErrSourceDrift
		}
		return cloneRun(state), nil
	}
	state := RunState{ID: start.ID, Adapter: start.Adapter, ManifestDigest: start.ManifestDigest, Phase: PhaseRunning, Tables: make(map[TableID]TableCheckpoint, len(start.Bounds))}
	for _, bound := range start.Bounds {
		state.Tables[bound.Table] = TableCheckpoint{UpperBound: UpperBound{Value: append([]byte(nil), bound.Value...), Empty: bound.Empty}}
	}
	target.runs[start.ID] = state
	return cloneRun(state), nil
}

func (target *fakeTarget) Load(_ context.Context, runID RunID) (RunState, error) {
	state, found := target.runs[runID]
	if !found {
		return RunState{}, ErrInvalidRun
	}
	return cloneRun(state), nil
}

func (target *fakeTarget) AcquireLease(_ context.Context, runID RunID, now time.Time, ttl time.Duration) (LeaseFence, error) {
	if _, found := target.runs[runID]; !found {
		return LeaseFence{}, ErrInvalidRun
	}
	if ttl <= 0 {
		return LeaseFence{}, ErrInvalidRun
	}
	previous := target.leases[runID]
	if previous.RunID != "" {
		if !previous.valid() || previous.ExpiresAt.After(now) {
			return LeaseFence{}, ErrLeaseFenced
		}
	}
	fence := LeaseFence{RunID: runID, Generation: previous.Generation + 1, Token: Sum([]byte(fmt.Sprintf("%s:%d", runID, previous.Generation+1))), ExpiresAt: now.Add(ttl)}
	target.leases[runID] = fence
	return fence, nil
}

func (target *fakeTarget) check(fence LeaseFence) error {
	if !fence.valid() || target.leases[fence.RunID] != fence || !target.clock.Now().Before(fence.ExpiresAt) {
		return ErrLeaseFenced
	}
	return nil
}

func (target *fakeTarget) Advance(_ context.Context, fence LeaseFence, table TableID, checkpoint TableCheckpoint) error {
	if err := target.check(fence); err != nil {
		return err
	}
	state := target.runs[fence.RunID]
	if _, found := state.Tables[table]; !found {
		return ErrInvalidRun
	}
	state.Tables[table] = TableCheckpoint{UpperBound: UpperBound{Value: append([]byte(nil), checkpoint.UpperBound.Value...), Empty: checkpoint.UpperBound.Empty}, Cursor: checkpoint.Cursor, Processed: checkpoint.Processed, Complete: checkpoint.Complete}
	target.runs[fence.RunID] = state
	return nil
}

func (target *fakeTarget) Finish(_ context.Context, fence LeaseFence) error {
	if err := target.check(fence); err != nil {
		return err
	}
	state := target.runs[fence.RunID]
	state.Phase = PhaseCompleted
	target.runs[fence.RunID] = state
	delete(target.leases, fence.RunID)
	return nil
}

func (target *fakeTarget) MarkReconciled(_ context.Context, fence LeaseFence) error {
	if err := target.check(fence); err != nil {
		return err
	}
	state := target.runs[fence.RunID]
	state.Phase = PhaseReconciled
	target.runs[fence.RunID] = state
	delete(target.leases, fence.RunID)
	return nil
}

func receiptKey(adapter AdapterID, table TableID, key Digest) string {
	return string(adapter) + ":" + string(table) + ":" + string(key[:])
}

func (target *fakeTarget) FindRowReceipt(_ context.Context, adapter AdapterID, table TableID, key Digest) (RowReceipt, bool, error) {
	receipt, found := target.receipts[receiptKey(adapter, table, key)]
	return receipt, found, nil
}

func (target *fakeTarget) AppendRowReceipt(_ context.Context, fence LeaseFence, receipt RowReceipt) error {
	if err := target.check(fence); err != nil {
		return err
	}
	key := receiptKey(receipt.Adapter, receipt.Table, receipt.SourceKey)
	if _, found := target.receipts[key]; found {
		return ErrSourcePayloadConflict
	}
	if target.failReceipt {
		return errors.New("injected row receipt failure")
	}
	target.receipts[key] = receipt
	return nil
}

func (target *fakeTarget) Apply(_ context.Context, fence LeaseFence, _ MappedRow) error {
	target.applyAttempts++
	if target.fenceOnApply {
		target.leases[fence.RunID] = LeaseFence{RunID: fence.RunID, Generation: fence.Generation + 1, Token: Sum([]byte("replaced")), ExpiresAt: fence.ExpiresAt}
	}
	if err := target.check(fence); err != nil {
		return err
	}
	if target.failApplyAt == target.applyAttempts {
		return errors.New("injected target write failure")
	}
	target.imports++
	return nil
}

func (target *fakeTarget) Quarantine(_ context.Context, fence LeaseFence, _ Quarantine) error {
	if err := target.check(fence); err != nil {
		return err
	}
	target.quarantines++
	return nil
}

func (target *fakeTarget) Archive(_ context.Context, fence LeaseFence, _ Archive) error {
	if err := target.check(fence); err != nil {
		return err
	}
	target.archives++
	return nil
}

func (target *fakeTarget) FindResultReceipt(_ context.Context, runID RunID, adapter AdapterID, table TableID, sourceKey Digest) (ResultReceipt, bool, error) {
	key := resultReceiptKey(adapter, table, sourceKey)
	for _, receipt := range target.results {
		if receipt.RunID == runID && resultReceiptKey(receipt.Adapter, receipt.Table, receipt.SourceKey) == key {
			return receipt, true, nil
		}
	}
	return ResultReceipt{}, false, nil
}

func (target *fakeTarget) AppendResultReceipt(_ context.Context, fence LeaseFence, receipt ResultReceipt) error {
	if err := target.check(fence); err != nil {
		return err
	}
	if target.failResult {
		return errors.New("injected result receipt failure")
	}
	target.results = append(target.results, receipt)
	return nil
}

func (target *fakeTarget) ListResultReceipts(_ context.Context, runID RunID) ([]ResultReceipt, error) {
	var result []ResultReceipt
	for _, receipt := range target.results {
		if receipt.RunID == runID {
			result = append(result, receipt)
		}
	}
	return result, nil
}

func (target *fakeTarget) VerifyResultReceipt(_ context.Context, _ ResultReceipt) error {
	if target.tampered {
		return ErrTargetTampered
	}
	return nil
}

type fakeTargetState struct {
	runs        map[RunID]RunState
	leases      map[RunID]LeaseFence
	receipts    map[string]RowReceipt
	results     []ResultReceipt
	imports     int
	archives    int
	quarantines int
}

func (target *fakeTarget) snapshot() fakeTargetState {
	state := fakeTargetState{runs: make(map[RunID]RunState, len(target.runs)), leases: make(map[RunID]LeaseFence, len(target.leases)), receipts: make(map[string]RowReceipt, len(target.receipts)), results: append([]ResultReceipt(nil), target.results...), imports: target.imports, archives: target.archives, quarantines: target.quarantines}
	for id, run := range target.runs {
		state.runs[id] = cloneRun(run)
	}
	for id, lease := range target.leases {
		state.leases[id] = lease
	}
	for key, receipt := range target.receipts {
		state.receipts[key] = receipt
	}
	return state
}

func (target *fakeTarget) restore(state fakeTargetState) {
	target.runs = state.runs
	target.leases = state.leases
	target.receipts = state.receipts
	target.results = state.results
	target.imports = state.imports
	target.archives = state.archives
	target.quarantines = state.quarantines
}

func cloneRun(state RunState) RunState {
	copy := state
	copy.Tables = make(map[TableID]TableCheckpoint, len(state.Tables))
	for id, checkpoint := range state.Tables {
		checkpoint.UpperBound.Value = append([]byte(nil), checkpoint.UpperBound.Value...)
		copy.Tables[id] = checkpoint
	}
	return copy
}

func fixture(disposition Disposition, rows ...SourceRow) (*Service, *fakeSource, *fakeTarget, AdapterID) {
	adapter := AdapterID("fixture")
	table := TableID("records")
	schema := Sum([]byte("records schema"))
	manifest := AdapterManifest{ID: adapter, Family: FamilyContact, SourceIdentity: "source-ref-01", SourceSchemaDigest: Sum([]byte("source schema")), Tables: []TableSpec{{ID: table, SourceIdentity: "legacy.records", SchemaDigest: schema, PrimaryKey: "id", Watermark: "updated_at", Mode: ModeSnapshot, Policy: "policy"}}}
	source := &fakeSource{preflight: SourcePreflight{Identity: manifest.SourceIdentity, SchemaDigest: manifest.SourceSchemaDigest, Bounds: []TableBound{{Table: table, SchemaDigest: schema, UpperBound: UpperBound{Value: []byte("2026-08-25T00:00:00Z")}}}}, rows: map[TableID][]SourceRow{table: rows}}
	clock := &fakeClock{now: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)}
	target := newFakeTarget(clock)
	service := NewServiceWithClock(clock, target, target, target, target, target, target, target, fakeRegistry{adapter: {Manifest: manifest, Source: source, Mapper: fakeMapper{}, Cursors: lexicalCursors{}}}, fakePolicies{"policy": {ID: "policy", Disposition: disposition}})
	service.limit = 1
	return service, source, target, adapter
}

func row(cursor, key, payload string) SourceRow {
	return SourceRow{Cursor: Cursor(cursor), SourceKey: []byte(key), Payload: []byte(payload), FieldDigest: Sum([]byte("fields:" + payload))}
}

func TestServiceAppliesClosedPolicies(t *testing.T) {
	tests := []struct {
		name        string
		disposition Disposition
		want        RunResult
	}{
		{"import", DispositionImport, RunResult{Imported: 1}},
		{"archive", DispositionArchive, RunResult{Archived: 1}},
		{"quarantine", DispositionQuarantine, RunResult{Quarantined: 1}},
		{"skip", DispositionSkip, RunResult{Skipped: 1}},
		{"rebuild", DispositionRebuild, RunResult{Rebuilt: 1}},
		{"reset", DispositionReset, RunResult{Reset: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _, target, adapter := fixture(test.disposition, row("1", "legacy-1", "payload"))
			got, err := service.Run(context.Background(), RunRequest{ID: RunID(test.name), Adapter: adapter})
			if err != nil || got != test.want {
				t.Fatalf("Run() = %#v, %v; want %#v", got, err, test.want)
			}
			if len(target.receipts) != 1 || len(target.results) != 1 {
				t.Fatalf("receipts = %d/%d", len(target.receipts), len(target.results))
			}
		})
	}
}

func TestServiceExactReplayAndPayloadConflictFailClosed(t *testing.T) {
	service, source, target, adapter := fixture(DispositionImport, row("1", "a", "one"), row("2", "b", "two"), row("3", "c", "three"))
	first, err := service.Run(context.Background(), RunRequest{ID: "first", Adapter: adapter})
	if err != nil || first.Imported != 3 || target.imports != 3 {
		t.Fatalf("first = %#v, err = %v, writes = %d", first, err, target.imports)
	}
	replay, err := service.Run(context.Background(), RunRequest{ID: "replay", Adapter: adapter})
	if err != nil || replay.Replayed != 3 || target.imports != 3 || len(target.results) != 6 {
		t.Fatalf("replay = %#v, err = %v, writes = %d, result receipts = %d", replay, err, target.imports, len(target.results))
	}
	source.rows["records"][1] = row("2", "b", "changed")
	if _, err := service.Run(context.Background(), RunRequest{ID: "conflict", Adapter: adapter}); !errors.Is(err, ErrSourcePayloadConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	if target.imports != 3 || len(target.receipts) != 3 {
		t.Fatalf("conflict changed target: writes=%d receipts=%d", target.imports, len(target.receipts))
	}
}

func TestServiceRepairsCurrentRunReplayReceiptWithoutTargetRewrite(t *testing.T) {
	service, _, target, adapter := fixture(DispositionImport, row("1", "a", "one"))
	if _, err := service.Run(context.Background(), RunRequest{ID: "current-replay", Adapter: adapter}); err != nil {
		t.Fatal(err)
	}
	// Model a crash-recovery persistence audit: the target row receipt exists but
	// the run-scoped result receipt must be restored by an exact replay.
	target.results = nil
	state := target.runs["current-replay"]
	state.Phase = PhaseRunning
	state.Tables["records"] = TableCheckpoint{UpperBound: state.Tables["records"].UpperBound}
	target.runs["current-replay"] = state
	got, err := service.Run(context.Background(), RunRequest{ID: "current-replay", Adapter: adapter})
	if err != nil || got.Replayed != 1 || target.imports != 1 || len(target.results) != 1 || target.runs["current-replay"].Tables["records"].Processed != 1 {
		t.Fatalf("current replay = %#v, err=%v, imports=%d results=%d state=%#v", got, err, target.imports, len(target.results), target.runs["current-replay"])
	}
}

func TestServiceResumesFromAtomicCheckpoint(t *testing.T) {
	service, _, target, adapter := fixture(DispositionImport, row("1", "a", "one"), row("2", "b", "two"), row("3", "c", "three"))
	target.failApplyAt = 2
	if _, err := service.Run(context.Background(), RunRequest{ID: "resume", Adapter: adapter}); err == nil {
		t.Fatal("first attempt unexpectedly succeeded")
	}
	if checkpoint := target.runs["resume"].Tables["records"]; checkpoint.Cursor != "1" || checkpoint.Complete {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	target.failApplyAt = 0
	target.clock.Advance(service.leaseTTL)
	got, err := service.Run(context.Background(), RunRequest{ID: "resume", Adapter: adapter})
	if err != nil || got.Imported != 2 || target.imports != 3 {
		t.Fatalf("resume = %#v, err = %v, writes = %d", got, err, target.imports)
	}
	if state := target.runs["resume"]; state.Phase != PhaseCompleted || !state.Tables["records"].Complete {
		t.Fatalf("final state = %#v", state)
	}
}

func TestServiceFencesStaleLeaseBeforeTargetWrite(t *testing.T) {
	service, _, target, adapter := fixture(DispositionImport, row("1", "a", "one"))
	target.fenceOnApply = true
	if _, err := service.Run(context.Background(), RunRequest{ID: "fenced", Adapter: adapter}); !errors.Is(err, ErrLeaseFenced) {
		t.Fatalf("error = %v", err)
	}
	if target.imports != 0 || len(target.receipts) != 0 {
		t.Fatalf("stale lease wrote target: imports=%d receipts=%d", target.imports, len(target.receipts))
	}
}

func TestServiceOnlyTakesOverExpiredLeaseAndFencesStaleGeneration(t *testing.T) {
	service, _, target, adapter := fixture(DispositionImport, row("1", "a", "one"))
	target.failApplyAt = 1 // Simulate a crashed runner before its first target commit.
	if _, err := service.Run(context.Background(), RunRequest{ID: "lease", Adapter: adapter}); err == nil {
		t.Fatal("crashed run unexpectedly succeeded")
	}
	stale := target.leases["lease"]
	if _, err := service.Run(context.Background(), RunRequest{ID: "lease", Adapter: adapter}); !errors.Is(err, ErrLeaseFenced) {
		t.Fatalf("active lease takeover = %v", err)
	}
	target.failApplyAt = 0
	target.clock.Advance(service.leaseTTL)
	if _, err := service.Run(context.Background(), RunRequest{ID: "lease", Adapter: adapter}); err != nil {
		t.Fatalf("expired lease resume = %v", err)
	}
	if err := target.Within(context.Background(), func(ctx context.Context) error {
		return target.AppendRowReceipt(ctx, stale, RowReceipt{Adapter: adapter, Table: "records", SourceKey: Sum([]byte("late"))})
	}); !errors.Is(err, ErrLeaseFenced) {
		t.Fatalf("stale generation target UoW = %v", err)
	}
}

func TestServiceConcurrentRunsCreateNoDuplicateTargetWrites(t *testing.T) {
	rows := []SourceRow{row("1", "a", "one"), row("2", "b", "two"), row("3", "c", "three"), row("4", "d", "four")}
	service, _, target, adapter := fixture(DispositionImport, rows...)
	var group sync.WaitGroup
	errorsByRun := make(chan error, 8)
	for i := 0; i < cap(errorsByRun); i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			_, err := service.Run(context.Background(), RunRequest{ID: RunID(fmt.Sprintf("race-%d", i)), Adapter: adapter})
			errorsByRun <- err
		}(i)
	}
	group.Wait()
	close(errorsByRun)
	for err := range errorsByRun {
		if err != nil {
			t.Fatalf("concurrent Run() error = %v", err)
		}
	}
	if target.imports != len(rows) || len(target.receipts) != len(rows) {
		t.Fatalf("duplicate writes imports=%d receipts=%d", target.imports, len(target.receipts))
	}
}

func TestReconcileDetectsTargetTamper(t *testing.T) {
	service, _, target, adapter := fixture(DispositionImport, row("1", "a", "one"))
	if _, err := service.Run(context.Background(), RunRequest{ID: "reconcile", Adapter: adapter}); err != nil {
		t.Fatal(err)
	}
	reconciler := NewReconcileServiceWithClock(target.clock, target, target, target, target)
	if err := reconciler.Reconcile(context.Background(), "reconcile"); err != nil {
		t.Fatalf("clean reconcile = %v", err)
	}
	tamperedService, _, tamperedTarget, tamperedAdapter := fixture(DispositionImport, row("1", "a", "one"))
	if _, err := tamperedService.Run(context.Background(), RunRequest{ID: "tampered", Adapter: tamperedAdapter}); err != nil {
		t.Fatal(err)
	}
	tamperedTarget.tampered = true
	tamperedReconciler := NewReconcileServiceWithClock(tamperedTarget.clock, tamperedTarget, tamperedTarget, tamperedTarget, tamperedTarget)
	if err := tamperedReconciler.Reconcile(context.Background(), "tampered"); !errors.Is(err, ErrTargetTampered) {
		t.Fatalf("tampered reconcile = %v", err)
	}
}

func TestReconcileRejectsMissingOrDuplicateRunReceipts(t *testing.T) {
	service, _, target, adapter := fixture(DispositionImport, row("1", "a", "one"), row("2", "b", "two"))
	if _, err := service.Run(context.Background(), RunRequest{ID: "missing", Adapter: adapter}); err != nil {
		t.Fatal(err)
	}
	target.results = target.results[:1]
	reconciler := NewReconcileServiceWithClock(target.clock, target, target, target, target)
	if err := reconciler.Reconcile(context.Background(), "missing"); !errors.Is(err, ErrTargetTampered) {
		t.Fatalf("missing receipt reconcile = %v", err)
	}
}

func TestServiceRejectsSourcePreflightAndUnboundedStream(t *testing.T) {
	service, source, _, adapter := fixture(DispositionImport, row("1", "a", "one"), row("2", "b", "two"))
	source.preflight.SchemaDigest = Sum([]byte("changed"))
	if _, err := service.Run(context.Background(), RunRequest{ID: "drift", Adapter: adapter}); !errors.Is(err, ErrSourceDrift) {
		t.Fatalf("schema drift = %v", err)
	}
	service, source, _, adapter = fixture(DispositionImport, row("1", "a", "one"), row("2", "b", "two"))
	source.overLimit = true
	if _, err := service.Run(context.Background(), RunRequest{ID: "bounded", Adapter: adapter}); !errors.Is(err, ErrUnboundedStream) {
		t.Fatalf("unbounded stream = %v", err)
	}
}

func TestServiceRejectsOpaqueCursorRegression(t *testing.T) {
	tests := []struct {
		name string
		rows []SourceRow
	}{
		{"equal", []SourceRow{row("1", "a", "one"), row("1", "b", "two")}},
		{"descending", []SourceRow{row("2", "a", "one"), row("1", "b", "two")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _, _, adapter := fixture(DispositionImport, test.rows...)
			service.limit = len(test.rows)
			if _, err := service.Run(context.Background(), RunRequest{ID: RunID(test.name), Adapter: adapter}); !errors.Is(err, ErrUnboundedStream) {
				t.Fatalf("cursor regression = %v", err)
			}
		})
	}
}

func TestServiceRejectsInvalidUpperBound(t *testing.T) {
	service, source, _, adapter := fixture(DispositionImport, row("1", "a", "one"))
	source.preflight.Bounds[0].Empty = true
	if _, err := service.Run(context.Background(), RunRequest{ID: "empty-with-value", Adapter: adapter}); !errors.Is(err, ErrSourceDrift) {
		t.Fatalf("empty with value = %v", err)
	}
	service, source, _, adapter = fixture(DispositionImport, row("1", "a", "one"))
	source.preflight.Bounds[0].Value = nil
	if _, err := service.Run(context.Background(), RunRequest{ID: "value-missing", Adapter: adapter}); !errors.Is(err, ErrSourceDrift) {
		t.Fatalf("nonempty without value = %v", err)
	}
}

func TestTargetUoWRollsBackMutationAndReceipts(t *testing.T) {
	for _, test := range []struct {
		name string
		set  func(*fakeTarget)
	}{
		{"row-receipt", func(target *fakeTarget) { target.failReceipt = true }},
		{"result-receipt", func(target *fakeTarget) { target.failResult = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _, target, adapter := fixture(DispositionImport, row("1", "a", "one"))
			test.set(target)
			if _, err := service.Run(context.Background(), RunRequest{ID: RunID(test.name), Adapter: adapter}); err == nil {
				t.Fatal("injected failure unexpectedly succeeded")
			}
			checkpoint := target.runs[RunID(test.name)].Tables["records"]
			if target.imports != 0 || len(target.receipts) != 0 || len(target.results) != 0 || checkpoint.Cursor != "" || checkpoint.Processed != 0 {
				t.Fatalf("rollback failed: imports=%d rows=%d results=%d checkpoint=%#v", target.imports, len(target.receipts), len(target.results), checkpoint)
			}
		})
	}
}

func TestStaticRegistriesSealDefinitions(t *testing.T) {
	service, _, _, adapter := fixture(DispositionImport, row("1", "a", "one"))
	definition, found := service.mappings.Lookup(adapter)
	if !found {
		t.Fatal("fixture definition missing")
	}
	registry, err := NewStaticMappingRegistry(definition)
	if err != nil || registry == nil {
		t.Fatalf("mapping registry = %v, %v", registry, err)
	}
	if _, err := NewStaticMappingRegistry(definition, definition); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("duplicate adapter = %v", err)
	}
	policies, err := NewStaticPolicyRegistry(Policy{ID: "import", Disposition: DispositionImport})
	if err != nil || policies == nil {
		t.Fatalf("policy registry = %v, %v", policies, err)
	}
	if _, err := NewStaticPolicyRegistry(Policy{ID: "bad", Disposition: "write arbitrary SQL"}); !errors.Is(err, ErrUnknownPolicy) {
		t.Fatalf("invalid policy = %v", err)
	}
}

func TestFamilyIsClosedAcrossMigrationLanes(t *testing.T) {
	for _, family := range []Family{
		FamilyContact, FamilyIdentity, FamilyWeCom,
		FamilyCampaign, FamilyOutbound, FamilyAutomation,
		FamilySurvey, FamilyMedia, FamilyRadar,
		FamilyCommerce, FamilyPayment, FamilyEntitlement,
		FamilyProduct, FamilyOperations, FamilyPlatform,
	} {
		if !family.Valid() {
			t.Fatalf("family %q is unexpectedly invalid", family)
		}
	}
	if Family("arbitrary-table-family").Valid() {
		t.Fatal("arbitrary family was accepted")
	}
}
