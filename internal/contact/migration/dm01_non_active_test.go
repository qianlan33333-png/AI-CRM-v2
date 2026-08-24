package migration

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var errNonActiveInjected = errors.New("non-active injected failure")

type nonActiveState struct {
	receipts    map[string]contactport.NonActiveRowReceipt
	archives    map[string]contactport.NonActiveArchive
	quarantines map[string]contactport.NonActiveQuarantine
	writes      []string
}

func newNonActiveState() *nonActiveState {
	return &nonActiveState{receipts: map[string]contactport.NonActiveRowReceipt{}, archives: map[string]contactport.NonActiveArchive{}, quarantines: map[string]contactport.NonActiveQuarantine{}}
}
func (state *nonActiveState) clone() *nonActiveState {
	next := newNonActiveState()
	for k, v := range state.receipts {
		next.receipts[k] = v
	}
	for k, v := range state.archives {
		next.archives[k] = v
	}
	for k, v := range state.quarantines {
		next.quarantines[k] = v
	}
	next.writes = append([]string(nil), state.writes...)
	return next
}

type nonActiveWorld struct {
	committed, tx *nonActiveState
	failAt        string
}

func newNonActiveWorld() *nonActiveWorld { return &nonActiveWorld{committed: newNonActiveState()} }
func (world *nonActiveWorld) Within(ctx context.Context, fn func(context.Context) error) error {
	world.tx = world.committed.clone()
	defer func() { world.tx = nil }()
	if err := fn(ctx); err != nil {
		return err
	}
	world.committed = world.tx
	return nil
}
func nonActiveKey(runID int64, source contactport.NonActiveSource, key []byte) string {
	return fmt.Sprintf("%d:%d:%x", runID, source, key)
}
func (world *nonActiveWorld) fail(op string) error {
	if world.failAt == op {
		return errNonActiveInjected
	}
	return nil
}
func (world *nonActiveWorld) AssertNonActiveLease(context.Context, contactport.NonActiveLeaseFence) error {
	return world.fail("lease")
}
func (world *nonActiveWorld) LockNonActiveSource(context.Context, contactport.NonActiveSource, []byte) error {
	return world.fail("lock")
}
func (world *nonActiveWorld) FindNonActiveReceipt(_ context.Context, runID int64, source contactport.NonActiveSource, key []byte) (contactport.NonActiveRowReceipt, bool, error) {
	v, ok := world.tx.receipts[nonActiveKey(runID, source, key)]
	return v, ok, world.fail("find-receipt")
}
func (world *nonActiveWorld) FindNonActiveArchive(_ context.Context, runID int64, source contactport.NonActiveSource, key []byte) (contactport.NonActiveArchive, bool, error) {
	v, ok := world.tx.archives[nonActiveKey(runID, source, key)]
	return v, ok, world.fail("find-archive")
}
func (world *nonActiveWorld) FindNonActiveQuarantine(_ context.Context, runID int64, source contactport.NonActiveSource, key []byte) (contactport.NonActiveQuarantine, bool, error) {
	v, ok := world.tx.quarantines[nonActiveKey(runID, source, key)]
	return v, ok, world.fail("find-quarantine")
}
func (world *nonActiveWorld) AppendNonActiveArchive(_ context.Context, value contactport.NonActiveArchive) error {
	if err := world.fail("archive"); err != nil {
		return err
	}
	world.tx.archives[nonActiveKey(value.RunID, value.Source, value.SourceFact.SourceKeyHMAC)] = value
	world.tx.writes = append(world.tx.writes, fmt.Sprintf("%d:archive", value.Source))
	return nil
}
func (world *nonActiveWorld) AppendNonActiveQuarantine(_ context.Context, value contactport.NonActiveQuarantine) error {
	if err := world.fail("quarantine"); err != nil {
		return err
	}
	world.tx.quarantines[nonActiveKey(value.RunID, value.Source, value.SourceFact.SourceKeyHMAC)] = value
	world.tx.writes = append(world.tx.writes, fmt.Sprintf("%d:quarantine:%s", value.Source, value.ReasonCode))
	return nil
}
func (world *nonActiveWorld) AppendNonActiveReceipt(_ context.Context, fence contactport.NonActiveLeaseFence, source contactport.NonActiveSource, fact contactport.HistoricalImportSourceFact, disposition contactport.NonActiveDisposition) error {
	if err := world.fail("receipt"); err != nil {
		return err
	}
	world.tx.receipts[nonActiveKey(fence.RunID, source, fact.SourceKeyHMAC)] = contactport.NonActiveRowReceipt{PayloadHMAC: fact.PayloadHMAC, FieldDigest: fact.FieldDigest, Disposition: disposition}
	world.tx.writes = append(world.tx.writes, fmt.Sprintf("%d:receipt", source))
	return nil
}

func nonActiveFixture(t *testing.T) NonActiveCommand {
	t.Helper()
	hmacKey := []byte("source-hmac-key")
	sources := []contactport.NonActiveSource{contactport.NonActiveMergeAudit, contactport.NonActiveResolutionQueue, contactport.NonActiveContacts, contactport.NonActiveIdentityConflicts, contactport.NonActivePeople, contactport.NonActiveFollowUsers, contactport.NonActiveDirectoryMembers, contactport.NonActiveExternalBindings}
	rows := make([]NonActiveRow, len(sources))
	for i, source := range sources {
		payload := []byte(fmt.Sprintf(`{"source":%d}`, source))
		payloadHMAC, err := SourcePayloadHMAC(hmacKey, nonActiveSourceTable(source), payload)
		if err != nil {
			t.Fatal(err)
		}
		row := NonActiveRow{Source: source, Fact: contactport.HistoricalImportSourceFact{SourceKeyHMAC: bytes32(byte(i + 1)), PayloadHMAC: payloadHMAC, FieldDigest: bytes32(byte(i + 21))}}
		if source == contactport.NonActiveMergeAudit || source == contactport.NonActiveResolutionQueue {
			row.ArchivePayload = payload
		}
		rows[i] = row
	}
	return NonActiveCommand{Fence: contactport.NonActiveLeaseFence{RunID: 9, Generation: 2, TokenHMAC: bytes32(90)}, ArchiveKey: bytes32(91), ArchiveKeyVersion: 3, PayloadHMACKey: hmacKey, Rows: rows}
}
func bytes32(value byte) []byte {
	result := make([]byte, 32)
	for i := range result {
		result[i] = value
	}
	return result
}

func TestNonActiveActionsReceiptLastAndExactReplay(t *testing.T) {
	world := newNonActiveWorld()
	service := NewNonActiveService(world, world)
	command := nonActiveFixture(t)
	got, err := service.Process(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	want := NonActiveResult{Archived: 2, Skipped: 3, Quarantined: 3}
	if got != want {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
	wantWrites := []string{
		"1:archive", "1:receipt", "2:archive", "2:receipt", "3:receipt",
		"4:quarantine:target_schema_deferred", "4:receipt",
		"5:quarantine:target_schema_deferred", "5:receipt",
		"6:quarantine:multiple_follow_users_deferred", "6:receipt",
		"7:receipt", "8:receipt",
	}
	if !reflect.DeepEqual(world.committed.writes, wantWrites) {
		t.Fatalf("writes = %v, want receipt-last %v", world.committed.writes, wantWrites)
	}
	writes := append([]string(nil), world.committed.writes...)
	got, err = service.Process(context.Background(), command)
	if err != nil || got != (NonActiveResult{Replayed: 8}) || !reflect.DeepEqual(writes, world.committed.writes) {
		t.Fatalf("replay = %+v/%v writes=%v", got, err, world.committed.writes)
	}
}

func TestNonActiveDriftAndFailureRollback(t *testing.T) {
	world := newNonActiveWorld()
	service := NewNonActiveService(world, world)
	command := nonActiveFixture(t)
	if _, err := service.Process(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	before := world.committed.clone()
	command.Rows[3].Fact.PayloadHMAC = bytes32(99)
	if _, err := service.Process(context.Background(), command); !errors.Is(err, ErrNonActiveDrift) {
		t.Fatalf("payload drift = %v", err)
	}
	if !reflect.DeepEqual(before, world.committed) {
		t.Fatal("drift mutated committed state")
	}

	world = newNonActiveWorld()
	world.failAt = "receipt"
	service = NewNonActiveService(world, world)
	command = nonActiveFixture(t)
	if got, err := service.Process(context.Background(), command); !errors.Is(err, errNonActiveInjected) || got != (NonActiveResult{}) {
		t.Fatalf("receipt failure = %+v/%v", got, err)
	}
	if len(world.committed.archives) != 0 || len(world.committed.quarantines) != 0 || len(world.committed.receipts) != 0 {
		t.Fatal("receipt fence failure did not roll back companion")
	}
}

func TestNonActiveRejectsInvalidRowsBeforeUoW(t *testing.T) {
	world := newNonActiveWorld()
	command := nonActiveFixture(t)
	command.Rows[2].ArchivePayload = []byte("not allowed")
	if _, err := NewNonActiveService(world, world).Process(context.Background(), command); !errors.Is(err, ErrInvalidNonActive) {
		t.Fatalf("invalid command = %v", err)
	}
	if world.tx != nil || len(world.committed.writes) != 0 {
		t.Fatal("invalid command entered UoW")
	}
}
