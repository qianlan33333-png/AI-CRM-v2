package migration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var errReconcileInjected = errors.New("reconcile injected failure")

type reconcileState struct {
	rows         map[ReconcileTable][]ReconcileReceipt
	companions   map[ReconcileTable]ReconcileCompanionCounts
	archives     map[ReconcileTable]ReconcileArchive
	resultDigest []byte
	completed    bool
	writes       []string
}

func (state *reconcileState) clone() *reconcileState {
	next := &reconcileState{rows: map[ReconcileTable][]ReconcileReceipt{}, companions: map[ReconcileTable]ReconcileCompanionCounts{}, archives: map[ReconcileTable]ReconcileArchive{}, resultDigest: append([]byte(nil), state.resultDigest...), completed: state.completed, writes: append([]string(nil), state.writes...)}
	for table, rows := range state.rows {
		next.rows[table] = append([]ReconcileReceipt(nil), rows...)
	}
	for table, counts := range state.companions {
		next.companions[table] = counts
	}
	for table, archive := range state.archives {
		archive.Nonce = append([]byte(nil), archive.Nonce...)
		archive.Ciphertext = append([]byte(nil), archive.Ciphertext...)
		next.archives[table] = archive
	}
	return next
}

type reconcileWorld struct {
	committed, tx *reconcileState
	failAt        string
	entered       bool
}

func (world *reconcileWorld) Within(ctx context.Context, fn func(context.Context) error) error {
	world.entered = true
	world.tx = world.committed.clone()
	defer func() { world.tx = nil }()
	if err := fn(ctx); err != nil {
		return err
	}
	world.committed = world.tx
	return nil
}
func (world *reconcileWorld) fail(at string) error {
	if world.failAt == at {
		return errReconcileInjected
	}
	return nil
}
func (world *reconcileWorld) LockReconcileRun(context.Context, contactport.NonActiveLeaseFence) (int64, error) {
	return 41, world.fail("lock")
}
func (world *reconcileWorld) StreamReconcileReceipts(_ context.Context, _ int64, table ReconcileTable, emit func(ReconcileReceipt) error) error {
	if err := world.fail("stream"); err != nil {
		return err
	}
	for _, row := range world.tx.rows[table] {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}
func (world *reconcileWorld) CountReconcileCompanions(_ context.Context, _ int64, table ReconcileTable) (ReconcileCompanionCounts, error) {
	return world.tx.companions[table], world.fail("companions")
}
func (world *reconcileWorld) ReadReconcileArchive(_ context.Context, _ int64, table ReconcileTable, _ []byte) (ReconcileArchive, bool, error) {
	archive, found := world.tx.archives[table]
	return archive, found, world.fail("archive")
}
func (world *reconcileWorld) AppendReconcileResult(_ context.Context, _ contactport.NonActiveLeaseFence, digest []byte) error {
	if err := world.fail("result"); err != nil {
		return err
	}
	world.tx.resultDigest = append([]byte(nil), digest...)
	world.tx.writes = append(world.tx.writes, "result")
	return nil
}
func (world *reconcileWorld) CompleteReconcileRun(context.Context, contactport.NonActiveLeaseFence) error {
	if err := world.fail("complete"); err != nil {
		return err
	}
	world.tx.completed = true
	world.tx.writes = append(world.tx.writes, "complete")
	return nil
}

func reconcileFixture() (*reconcileWorld, ReconcileCommand) {
	state := &reconcileState{rows: map[ReconcileTable][]ReconcileReceipt{}, companions: map[ReconcileTable]ReconcileCompanionCounts{}, archives: map[ReconcileTable]ReconcileArchive{}}
	dispositions := map[ReconcileTable]string{ReconcileOwnerRoleMap: "imported", ReconcileCustomerIdentity: "imported", ReconcileExternalIdentity: "imported", ReconcileMergeAudit: "archived", ReconcileResolutionQueue: "archived", ReconcileDirectoryMembers: "skipped", ReconcileContacts: "skipped", ReconcileIdentityConflicts: "quarantined", ReconcileExternalBindings: "skipped", ReconcilePeople: "quarantined", ReconcileFollowUsers: "quarantined"}
	command := ReconcileCommand{Fence: contactport.NonActiveLeaseFence{RunID: 51, Generation: 4, TokenHMAC: digest(90)}, ArchiveKey: bytes.Repeat([]byte{7}, 32), HMACKey: bytes.Repeat([]byte{8}, 32)}
	for _, table := range reconcileTableOrder {
		row := ReconcileReceipt{SourceFact: sourceFact(byte(table * 3)), Disposition: dispositions[table]}
		switch row.Disposition {
		case "archived":
			row.ArchiveCount = 1
			plaintext := []byte(fmt.Sprintf(`{"table":%d}`, table))
			row.SourceFact.PayloadHMAC, _ = SourcePayloadHMAC(command.HMACKey, ReconcileTableName(table), plaintext)
			aad, _ := ArchiveAAD(41, ReconcileTableName(table), row.SourceFact.SourceKeyHMAC, row.SourceFact.PayloadHMAC, row.SourceFact.FieldDigest, 1)
			nonce, ciphertext, _ := EncryptArchiveBound(command.ArchiveKey, aad, plaintext)
			state.archives[table] = ReconcileArchive{SourceFact: row.SourceFact, Nonce: nonce, Ciphertext: ciphertext, KeyVersion: 1}
		case "quarantined":
			row.QuarantineCount = 1
		}
		if table == ReconcileCustomerIdentity {
			row.QuarantineCount = 1
		}
		state.rows[table] = []ReconcileReceipt{row}
		state.companions[table] = ReconcileCompanionCounts{Archives: row.ArchiveCount, Quarantines: row.QuarantineCount}
		digest := NewReconcileDigest()
		_ = digest.Add(row.SourceFact)
		command.Sources = append(command.Sources, ReconcileSourceSummary{Table: table, Count: 1, Digest: digest.Sum()})
	}
	return &reconcileWorld{committed: state}, command
}

func TestReconcileComparesElevenTablesAndCompletesLeaseCAS(t *testing.T) {
	world, command := reconcileFixture()
	beforeRows := world.committed.clone()
	result, err := NewReconcileService(world, world).Reconcile(context.Background(), command)
	if err != nil || result.ParentRunID != 41 || result.SourceRows != 11 || result.Dispositions != (ReconcileDispositionCounts{Imported: 3, Quarantined: 3, Archived: 2, Skipped: 3}) || len(result.ResultDigest) != 32 {
		t.Fatalf("reconcile = %+v/%v", result, err)
	}
	if !world.committed.completed || !reflect.DeepEqual(world.committed.writes, []string{"result", "complete"}) || !reflect.DeepEqual(world.committed.resultDigest, result.ResultDigest) {
		t.Fatalf("completion = %+v", world.committed)
	}
	if !reflect.DeepEqual(world.committed.rows, beforeRows.rows) || !reflect.DeepEqual(world.committed.companions, beforeRows.companions) {
		t.Fatal("reconcile repaired parent facts")
	}
}

func TestReconcileMismatchWritesNothing(t *testing.T) {
	for _, mutate := range []func(*reconcileWorld, *ReconcileCommand){
		func(_ *reconcileWorld, command *ReconcileCommand) { command.Sources[0].Digest[0]++ },
		func(world *reconcileWorld, _ *ReconcileCommand) {
			world.committed.companions[ReconcileMergeAudit] = ReconcileCompanionCounts{}
		},
		func(world *reconcileWorld, _ *ReconcileCommand) {
			row := world.committed.rows[ReconcileContacts][0]
			row.Disposition = "archived"
			world.committed.rows[ReconcileContacts][0] = row
		},
		func(world *reconcileWorld, _ *ReconcileCommand) {
			archive := world.committed.archives[ReconcileMergeAudit]
			archive.Ciphertext[0]++
			world.committed.archives[ReconcileMergeAudit] = archive
		},
	} {
		world, command := reconcileFixture()
		mutate(world, &command)
		before := world.committed.clone()
		if result, err := NewReconcileService(world, world).Reconcile(context.Background(), command); !errors.Is(err, ErrReconcileMismatch) || !reflect.DeepEqual(result, ReconcileResult{}) {
			t.Fatalf("mismatch = %+v/%v", result, err)
		}
		if !reflect.DeepEqual(world.committed, before) {
			t.Fatal("mismatch wrote or repaired state")
		}
	}
}

func TestReconcileDigestIsOrderSensitive(t *testing.T) {
	left, right := NewReconcileDigest(), NewReconcileDigest()
	first, second := sourceFact(1), sourceFact(2)
	_ = left.Add(first)
	_ = left.Add(second)
	_ = right.Add(second)
	_ = right.Add(first)
	if bytes.Equal(left.Sum(), right.Sum()) {
		t.Fatal("reconcile stream digest ignored row order")
	}
}

func TestReconcileCompletionFailureRollsBackUniqueResult(t *testing.T) {
	world, command := reconcileFixture()
	world.failAt = "complete"
	before := world.committed.clone()
	if result, err := NewReconcileService(world, world).Reconcile(context.Background(), command); !errors.Is(err, errReconcileInjected) || !reflect.DeepEqual(result, ReconcileResult{}) {
		t.Fatalf("complete failure = %+v/%v", result, err)
	}
	if !reflect.DeepEqual(world.committed, before) {
		t.Fatal("failed lease CAS retained result receipt")
	}
}

func TestReconcileRejectsIncompleteTableSetBeforeUoW(t *testing.T) {
	world, command := reconcileFixture()
	command.Sources = command.Sources[:10]
	if _, err := NewReconcileService(world, world).Reconcile(context.Background(), command); !errors.Is(err, ErrInvalidReconcile) {
		t.Fatalf("invalid = %v", err)
	}
	if world.entered {
		t.Fatal("invalid reconcile entered UoW")
	}
}
