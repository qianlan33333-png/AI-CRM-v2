package migration

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

var errActiveRootInjected = errors.New("active root injected failure")

type activeRootState struct {
	staff          map[int64]contactport.HistoricalImportStaffFact
	customers      map[int64]contactport.HistoricalImportCustomerFact
	identities     map[int64]identityport.HistoricalScopedIdentity
	identityByName map[string]int64
	lineage        map[string]contactport.HistoricalImportLineage
	receipts       map[string]contactport.HistoricalImportRowReceipt
	quarantines    []contactport.HistoricalImportQuarantine
	writes         []string
	targetWrites   []string
	nextID         int64
}

func newActiveRootState() *activeRootState {
	return &activeRootState{staff: map[int64]contactport.HistoricalImportStaffFact{}, customers: map[int64]contactport.HistoricalImportCustomerFact{}, identities: map[int64]identityport.HistoricalScopedIdentity{}, identityByName: map[string]int64{}, lineage: map[string]contactport.HistoricalImportLineage{}, receipts: map[string]contactport.HistoricalImportRowReceipt{}, nextID: 40}
}

func (state *activeRootState) clone() *activeRootState {
	next := newActiveRootState()
	next.nextID = state.nextID
	for key, value := range state.staff {
		next.staff[key] = value
	}
	for key, value := range state.customers {
		next.customers[key] = cloneCustomerFact(value)
	}
	for key, value := range state.identities {
		next.identities[key] = value
	}
	for key, value := range state.identityByName {
		next.identityByName[key] = value
	}
	for key, value := range state.lineage {
		next.lineage[key] = contactport.HistoricalImportLineage{TargetID: value.TargetID, PayloadHMAC: append([]byte(nil), value.PayloadHMAC...)}
	}
	for key, value := range state.receipts {
		next.receipts[key] = contactport.HistoricalImportRowReceipt{PayloadHMAC: append([]byte(nil), value.PayloadHMAC...), FieldDigest: append([]byte(nil), value.FieldDigest...), Disposition: value.Disposition}
	}
	next.quarantines = append([]contactport.HistoricalImportQuarantine(nil), state.quarantines...)
	next.writes = append([]string(nil), state.writes...)
	next.targetWrites = append([]string(nil), state.targetWrites...)
	return next
}

type activeRootWorld struct {
	committed *activeRootState
	tx        *activeRootState
	failAt    string
}

func newActiveRootWorld() *activeRootWorld { return &activeRootWorld{committed: newActiveRootState()} }

func (world *activeRootWorld) Within(ctx context.Context, fn func(context.Context) error) error {
	world.tx = world.committed.clone()
	defer func() { world.tx = nil }()
	if err := fn(ctx); err != nil {
		return err
	}
	world.committed = world.tx
	return nil
}

func (world *activeRootWorld) fail(operation string) error {
	if world.failAt == operation {
		return errActiveRootInjected
	}
	return nil
}

func lineageKey(source contactport.HistoricalImportSource, key []byte) string {
	return string(rune(source)) + string(key)
}

func receiptKey(runID int64, source contactport.HistoricalImportSource, key []byte) string {
	return time.Unix(runID, 0).String() + lineageKey(source, key)
}

func (world *activeRootWorld) LockHistoricalImportSource(context.Context, contactport.HistoricalImportSource, []byte) error {
	return world.fail("lock-source")
}

func (world *activeRootWorld) FindHistoricalImportRowReceipt(_ context.Context, runID int64, source contactport.HistoricalImportSource, key []byte) (contactport.HistoricalImportRowReceipt, bool, error) {
	if err := world.fail("find-receipt"); err != nil {
		return contactport.HistoricalImportRowReceipt{}, false, err
	}
	receipt, found := world.tx.receipts[receiptKey(runID, source, key)]
	return receipt, found, nil
}

func (world *activeRootWorld) LockHistoricalImportLineage(_ context.Context, source contactport.HistoricalImportSource, key []byte) (contactport.HistoricalImportLineage, bool, error) {
	if err := world.fail("lock-lineage"); err != nil {
		return contactport.HistoricalImportLineage{}, false, err
	}
	lineage, found := world.tx.lineage[lineageKey(source, key)]
	return lineage, found, nil
}

func (world *activeRootWorld) EnsureHistoricalImportStaff(_ context.Context, fact contactport.HistoricalImportStaffFact) (int64, error) {
	if err := world.fail("ensure-staff"); err != nil {
		return 0, err
	}
	world.tx.nextID++
	id := world.tx.nextID
	world.tx.staff[id] = fact
	world.tx.writes = append(world.tx.writes, "staff")
	world.tx.targetWrites = append(world.tx.targetWrites, "230:staff")
	return id, nil
}

func (world *activeRootWorld) CreateHistoricalImportCustomer(_ context.Context, fact contactport.HistoricalImportCustomerFact) (int64, error) {
	if err := world.fail("create-customer"); err != nil {
		return 0, err
	}
	world.tx.nextID++
	id := world.tx.nextID
	world.tx.customers[id] = cloneCustomerFact(fact)
	world.tx.writes = append(world.tx.writes, "customer")
	world.tx.targetWrites = append(world.tx.targetWrites, "152:customer")
	return id, nil
}

func (world *activeRootWorld) ValidateHistoricalImportStaff(_ context.Context, id int64, fact contactport.HistoricalImportStaffFact) error {
	if world.tx.staff[id] != fact {
		return ErrActiveRootDrift
	}
	return world.fail("validate-staff")
}

func (world *activeRootWorld) ValidateHistoricalImportCustomer(_ context.Context, id int64, fact contactport.HistoricalImportCustomerFact) error {
	if !reflect.DeepEqual(world.tx.customers[id], fact) {
		return ErrActiveRootDrift
	}
	return world.fail("validate-customer")
}

func (world *activeRootWorld) IsHistoricalImportActiveStaff(_ context.Context, id int64) (bool, error) {
	fact, found := world.tx.staff[id]
	if !found {
		return false, ErrActiveRootDrift
	}
	if err := world.fail("validate-active-staff"); err != nil {
		return false, err
	}
	return fact.Active, nil
}

func (world *activeRootWorld) ValidateHistoricalImportCustomerRoot(_ context.Context, id int64) error {
	if _, found := world.tx.customers[id]; !found {
		return ErrActiveRootDrift
	}
	return world.fail("validate-customer-root")
}

func (world *activeRootWorld) AppendHistoricalImportLineage(_ context.Context, _ int64, source contactport.HistoricalImportSource, fact contactport.HistoricalImportSourceFact, targetID int64) error {
	if err := world.fail("append-lineage"); err != nil {
		return err
	}
	world.tx.lineage[lineageKey(source, fact.SourceKeyHMAC)] = contactport.HistoricalImportLineage{TargetID: targetID, PayloadHMAC: append([]byte(nil), fact.PayloadHMAC...)}
	world.tx.writes = append(world.tx.writes, "lineage")
	return nil
}

func (world *activeRootWorld) AppendHistoricalImportQuarantine(_ context.Context, quarantine contactport.HistoricalImportQuarantine) error {
	if err := world.fail("append-quarantine"); err != nil {
		return err
	}
	world.tx.quarantines = append(world.tx.quarantines, quarantine)
	world.tx.writes = append(world.tx.writes, "quarantine")
	return nil
}

func (world *activeRootWorld) AppendHistoricalImportRowReceipt(_ context.Context, runID int64, source contactport.HistoricalImportSource, fact contactport.HistoricalImportSourceFact, disposition contactport.HistoricalImportDisposition) error {
	if err := world.fail("append-receipt"); err != nil {
		return err
	}
	world.tx.receipts[receiptKey(runID, source, fact.SourceKeyHMAC)] = contactport.HistoricalImportRowReceipt{PayloadHMAC: append([]byte(nil), fact.PayloadHMAC...), FieldDigest: append([]byte(nil), fact.FieldDigest...), Disposition: disposition}
	world.tx.writes = append(world.tx.writes, "receipt")
	return nil
}

func (world *activeRootWorld) BindHistoricalScopedWeComIdentity(_ context.Context, fact identityport.HistoricalScopedIdentity) (identityport.HistoricalScopedIdentityResult, error) {
	if err := world.fail("bind-identity"); err != nil {
		return identityport.HistoricalScopedIdentityResult{}, err
	}
	name := fact.Scope + "\x00" + fact.ExternalUserID
	if id, found := world.tx.identityByName[name]; found {
		if world.tx.identities[id].CustomerID != fact.CustomerID {
			return identityport.HistoricalScopedIdentityResult{}, identityport.ErrHistoricalScopedIdentityConflict
		}
		return identityport.HistoricalScopedIdentityResult{IdentityID: id, Bound: true}, nil
	}
	world.tx.nextID++
	id := world.tx.nextID
	world.tx.identities[id] = fact
	world.tx.identityByName[name] = id
	world.tx.writes = append(world.tx.writes, "identity")
	world.tx.targetWrites = append(world.tx.targetWrites, "314:identity")
	return identityport.HistoricalScopedIdentityResult{IdentityID: id, Bound: true}, nil
}

func (world *activeRootWorld) ValidateHistoricalScopedWeComIdentity(_ context.Context, id int64, fact identityport.HistoricalScopedIdentity) error {
	if !reflect.DeepEqual(world.tx.identities[id], fact) {
		return ErrActiveRootDrift
	}
	return world.fail("validate-identity")
}

func TestActiveRootServiceOrdersRootsAndExactReplayWritesNothing(t *testing.T) {
	world := newActiveRootWorld()
	service := NewActiveRootService(world, world, world)
	command := completeActiveRootsCommand()
	result, err := service.Process(context.Background(), command)
	if err != nil || result != (ActiveRootsResult{Imported: 3}) {
		t.Fatalf("first process = %+v, %v", result, err)
	}
	if got, want := world.committed.targetWrites, []string{"230:staff", "152:customer", "314:identity"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("target order = %v, want %v", got, want)
	}
	before := world.committed.clone()
	result, err = service.Process(context.Background(), command)
	if err != nil || result != (ActiveRootsResult{Replayed: 3}) {
		t.Fatalf("replay = %+v, %v", result, err)
	}
	if !reflect.DeepEqual(world.committed, before) {
		t.Fatalf("exact replay wrote target or ledger: before=%+v after=%+v", before, world.committed)
	}
}

func TestActiveRootServicePayloadMismatchAndTargetDriftFailClosed(t *testing.T) {
	world := newActiveRootWorld()
	service := NewActiveRootService(world, world, world)
	command := completeActiveRootsCommand()
	if _, err := service.Process(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	before := world.committed.clone()
	drifted := completeActiveRootsCommand()
	drifted.Customers[0].Source.PayloadHMAC[0]++
	if _, err := service.Process(context.Background(), drifted); !errors.Is(err, ErrSourcePayloadDrift) {
		t.Fatalf("payload drift = %v", err)
	}
	if !reflect.DeepEqual(world.committed, before) {
		t.Fatal("payload drift changed committed state")
	}
	for id, customer := range world.committed.customers {
		customer.Name = "tampered"
		world.committed.customers[id] = customer
	}
	tampered := world.committed.clone()
	if _, err := service.Process(context.Background(), command); !errors.Is(err, ErrActiveRootDrift) {
		t.Fatalf("target drift = %v", err)
	}
	if !reflect.DeepEqual(world.committed, tampered) {
		t.Fatal("target drift attempt changed committed state")
	}
}

func TestActiveRootServiceChangedSnapshotBecomesCandidate(t *testing.T) {
	world := newActiveRootWorld()
	service := NewActiveRootService(world, world, world)
	command := completeActiveRootsCommand()
	if _, err := service.Process(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	changed := completeActiveRootsCommand()
	changed.RunID++
	changed.Customers[0].Source.PayloadHMAC[0]++
	result, err := service.Process(context.Background(), changed)
	if err != nil || result.ChangedSourceCandidates != 1 || result.Quarantined != 1 {
		t.Fatalf("changed source = %+v, %v", result, err)
	}
	if got := world.committed.customers[42].Name; got != "Customer One" {
		t.Fatalf("changed source mutated target: %q", got)
	}
}

func TestActiveRootServiceImportsInactiveStaffButLeavesOwnerUnresolved(t *testing.T) {
	world := newActiveRootWorld()
	service := NewActiveRootService(world, world, world)
	command := completeActiveRootsCommand()
	command.Staff[0].Target.Active = false
	result, err := service.Process(context.Background(), command)
	if err != nil || result.Imported != 3 || result.Quarantined != 1 {
		t.Fatalf("inactive owner = %+v, %v", result, err)
	}
	if got := world.committed.customers[42].OwnerStaffID; got != nil {
		t.Fatalf("inactive staff became owner: %v", *got)
	}
	if len(world.committed.quarantines) != 1 || world.committed.quarantines[0].ReasonCode != quarantineOwnerUnresolved {
		t.Fatalf("owner quarantine = %+v", world.committed.quarantines)
	}
}

func TestActiveRootServiceImportsCustomerWhenOwnerMappingIsMissing(t *testing.T) {
	world := newActiveRootWorld()
	service := NewActiveRootService(world, world, world)
	command := completeActiveRootsCommand()
	command.Staff = nil
	result, err := service.Process(context.Background(), command)
	if err != nil || result.Imported != 2 || result.Quarantined != 1 {
		t.Fatalf("missing owner = %+v, %v", result, err)
	}
	if got := world.committed.customers[41].OwnerStaffID; got != nil {
		t.Fatalf("missing owner was guessed: %v", *got)
	}
	if len(world.committed.quarantines) != 1 || world.committed.quarantines[0].ReasonCode != quarantineOwnerUnresolved {
		t.Fatalf("owner quarantine = %+v", world.committed.quarantines)
	}
}

func TestActiveRootServiceRejectsOversizedBatchBeforeUoW(t *testing.T) {
	world := newActiveRootWorld()
	service := NewActiveRootService(world, world, world)
	command := identityOnlyCommand()
	command.Identities = make([]ExternalIdentityActiveRoot, MaximumActiveRootBatchRows+1)
	for index := range command.Identities {
		command.Identities[index] = ExternalIdentityActiveRoot{Source: sourceFact(byte(index%250 + 1)), CustomerSourceKeyHMAC: digest(4), CorpID: "corp-a", ExternalUserID: "external-1"}
	}
	if _, err := service.Process(context.Background(), command); !errors.Is(err, ErrInvalidActiveRoots) {
		t.Fatalf("oversized = %v", err)
	}
	if world.tx != nil || !reflect.DeepEqual(world.committed, newActiveRootState()) {
		t.Fatal("oversized batch entered target UoW")
	}
}

func TestActiveRootServiceRollsBackEveryTargetWriteFailure(t *testing.T) {
	for _, failAt := range []string{"ensure-staff", "append-lineage", "append-receipt", "create-customer", "bind-identity"} {
		t.Run(failAt, func(t *testing.T) {
			world := newActiveRootWorld()
			world.failAt = failAt
			service := NewActiveRootService(world, world, world)
			if _, err := service.Process(context.Background(), completeActiveRootsCommand()); !errors.Is(err, errActiveRootInjected) {
				t.Fatalf("error = %v", err)
			}
			if !reflect.DeepEqual(world.committed, newActiveRootState()) {
				t.Fatalf("%s left committed writes: %+v", failAt, world.committed)
			}
		})
	}
}

func TestActiveRootServiceQuarantinesMissingRootAndDifferentCustomerConflict(t *testing.T) {
	t.Run("missing root", func(t *testing.T) {
		world := newActiveRootWorld()
		service := NewActiveRootService(world, world, world)
		command := identityOnlyCommand()
		result, err := service.Process(context.Background(), command)
		if err != nil || result != (ActiveRootsResult{Quarantined: 1}) || len(world.committed.quarantines) != 1 || len(world.committed.identities) != 0 {
			t.Fatalf("missing root = %+v, %v, state=%+v", result, err, world.committed)
		}
		if world.committed.quarantines[0].ReasonCode != quarantineMissingCustomerRoot {
			t.Fatalf("reason = %q", world.committed.quarantines[0].ReasonCode)
		}
	})
	t.Run("different customer", func(t *testing.T) {
		world := newActiveRootWorld()
		world.committed.nextID = 80
		world.committed.identities[80] = identityport.HistoricalScopedIdentity{CustomerID: 999, Scope: "wecom-corp:corp-a", ExternalUserID: "external-1", SourceKeyHMAC: digest(99), HMACKeyVersion: 1}
		world.committed.identityByName["wecom-corp:corp-a\x00external-1"] = 80
		service := NewActiveRootService(world, world, world)
		command := completeActiveRootsCommand()
		result, err := service.Process(context.Background(), command)
		if err != nil || result != (ActiveRootsResult{Imported: 2, Quarantined: 1}) || len(world.committed.quarantines) != 1 {
			t.Fatalf("identity conflict = %+v, %v, state=%+v", result, err, world.committed)
		}
		if world.committed.quarantines[0].ReasonCode != quarantineIdentityConflict || world.committed.identities[80].CustomerID != 999 {
			t.Fatalf("conflict was not fail-closed: %+v", world.committed)
		}
	})
}

func TestActiveRootServiceRollsBackQuarantineReceiptFailure(t *testing.T) {
	for _, failAt := range []string{"append-quarantine", "append-receipt"} {
		world := newActiveRootWorld()
		world.failAt = failAt
		service := NewActiveRootService(world, world, world)
		if _, err := service.Process(context.Background(), identityOnlyCommand()); !errors.Is(err, errActiveRootInjected) {
			t.Fatalf("%s error = %v", failAt, err)
		}
		if !reflect.DeepEqual(world.committed, newActiveRootState()) {
			t.Fatalf("%s left quarantine state: %+v", failAt, world.committed)
		}
	}
}

func completeActiveRootsCommand() ActiveRootsCommand {
	now := time.Date(2026, 8, 25, 1, 2, 3, 0, time.UTC)
	staffKey, customerKey := digest(1), digest(4)
	return ActiveRootsCommand{
		RunID: 7, CorpID: "corp-a", HMACKeyVersion: 1,
		Staff:      []StaffActiveRoot{{Source: sourceFact(1), Target: contactport.HistoricalImportStaffFact{WeComUserID: "staff-1", Name: "Staff One", Active: true, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}}},
		Customers:  []CustomerActiveRoot{{Source: sourceFact(4), OwnerStaffSourceKeyHMAC: staffKey, Target: contactport.HistoricalImportCustomerFact{Name: "Customer One", FirstSeenAt: now.Add(-time.Hour), LastSeenAt: now, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}}},
		Identities: []ExternalIdentityActiveRoot{{Source: sourceFact(7), CustomerSourceKeyHMAC: customerKey, CorpID: "corp-a", ExternalUserID: "external-1"}},
	}
}

func identityOnlyCommand() ActiveRootsCommand {
	return ActiveRootsCommand{RunID: 7, CorpID: "corp-a", HMACKeyVersion: 1, Identities: []ExternalIdentityActiveRoot{{Source: sourceFact(7), CustomerSourceKeyHMAC: digest(4), CorpID: "corp-a", ExternalUserID: "external-1"}}}
}

func sourceFact(seed byte) contactport.HistoricalImportSourceFact {
	return contactport.HistoricalImportSourceFact{SourceKeyHMAC: digest(seed), PayloadHMAC: digest(seed + 1), FieldDigest: digest(seed + 2)}
}

func digest(seed byte) []byte {
	value := make([]byte, 32)
	value[0] = seed
	return value
}

func cloneCustomerFact(fact contactport.HistoricalImportCustomerFact) contactport.HistoricalImportCustomerFact {
	result := fact
	if fact.OwnerStaffID != nil {
		owner := *fact.OwnerStaffID
		result.OwnerStaffID = &owner
	}
	return result
}
