package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"testing"

	v1deferredidentityhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestDeferredIdentityHistoryReconcileTargetsVerifyCompleteSelection(t *testing.T) {
	t.Parallel()
	fixture := newDeferredIdentityHistoryReconcileFixture(t)
	targets, err := NewDeferredIdentityHistoryReconcileTargets(fixture.selection, fixture.options, fixture.reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range fixture.rows {
		value, verifyErr := targets.Verify(context.Background(), row)
		if verifyErr != nil || value == "" {
			t.Fatalf("row %s verification=%q err=%v", row.TableID, value, verifyErr)
		}
	}
	if err := targets.VerifyComplete(); err != nil {
		t.Fatalf("complete selection rejected: %v", err)
	}
}

func TestDeferredIdentityHistoryReconcileTargetsFailClosedOnDrift(t *testing.T) {
	t.Parallel()
	fixture := newDeferredIdentityHistoryReconcileFixture(t)
	for name, mutate := range map[string]func(*deferredIdentityHistoryReconcileFixture){
		"archive field receipt drift": func(fixture *deferredIdentityHistoryReconcileFixture) {
			fixture.rows[0].FieldDigest[0]++
		},
		"receipt target digest drift": func(fixture *deferredIdentityHistoryReconcileFixture) {
			fixture.rows[0].TargetDigest[0]++
		},
		"private target drift": func(fixture *deferredIdentityHistoryReconcileFixture) {
			row := fixture.rows[0]
			id, _ := positiveID(*row.TargetID)
			value := fixture.reader.people[id]
			value.PrivateDigest[0]++
			fixture.reader.people[id] = value
		},
		"target table drift": func(fixture *deferredIdentityHistoryReconcileFixture) {
			wrong := DeferredConflictHistoryTarget
			fixture.rows[0].TargetTable = &wrong
		},
		"unverified receipt": func(fixture *deferredIdentityHistoryReconcileFixture) {
			fixture.rows[0].Verified = false
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy := fixture.clone()
			mutate(&copy)
			targets, err := NewDeferredIdentityHistoryReconcileTargets(copy.selection, copy.options, copy.reader)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = targets.Verify(context.Background(), copy.rows[0]); !errors.Is(err, ErrConflict) {
				t.Fatalf("drift accepted: %v", err)
			}
		})
	}
}

func TestDeferredIdentityHistoryReconcileTargetsRejectDuplicateAndIncompleteReceiptSet(t *testing.T) {
	t.Parallel()
	fixture := newDeferredIdentityHistoryReconcileFixture(t)
	targets, err := NewDeferredIdentityHistoryReconcileTargets(fixture.selection, fixture.options, fixture.reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := targets.Verify(context.Background(), fixture.rows[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := targets.Verify(context.Background(), fixture.rows[0]); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate receipt accepted: %v", err)
	}
	if err := targets.VerifyComplete(); !errors.Is(err, ErrConflict) {
		t.Fatalf("incomplete selection accepted: %v", err)
	}
}

func TestNewDeferredIdentityHistoryReconcileTargetsRejectsUnselectedScope(t *testing.T) {
	t.Parallel()
	fixture := newDeferredIdentityHistoryReconcileFixture(t)
	fixture.selection.People = fixture.selection.People[:len(fixture.selection.People)-1]
	if _, err := NewDeferredIdentityHistoryReconcileTargets(fixture.selection, fixture.options, fixture.reader); !errors.Is(err, ErrConflict) {
		t.Fatalf("short selection accepted: %v", err)
	}
}

type deferredIdentityHistoryReconcileFixture struct {
	selection v1deferredidentityhistory.DeferredIdentitySelection
	options   v1deferredidentityhistory.DeferredIdentitySelectionOptions
	reader    *deferredIdentityHistoryReconcileReaderFake
	rows      []reconciliationRow
}

func newDeferredIdentityHistoryReconcileFixture(t *testing.T) deferredIdentityHistoryReconcileFixture {
	t.Helper()
	source := newDeferredIdentityHistoryFixture(t)
	selection, err := v1deferredidentityhistory.SelectDeferredIdentityEvidence(context.Background(), source.archive, source.dm01, source.options)
	if err != nil || selection.Count() != 1392 {
		t.Fatalf("selection=%d err=%v", selection.Count(), err)
	}
	reader := &deferredIdentityHistoryReconcileReaderFake{
		people: map[int64]contactport.HistoricalDeferredPerson{}, conflicts: map[int64]contactport.HistoricalDeferredIdentityConflict{}, missingRoots: map[int64]contactport.HistoricalMissingRootIdentity{},
	}
	rows := make([]reconciliationRow, 0, selection.Count())
	nextID := int64(1)
	for _, selected := range selection.People {
		value := deferredPersonValue(selected)
		value.ID = nextID
		reader.people[nextID] = value
		digest, digestErr := contactapp.HistoricalDeferredPersonDigest(value)
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		rows = append(rows, deferredIdentityHistoryReconciliationRow(selected.ArchivedRow.TableID, DeferredPersonHistoryTarget, selected.ArchivedRow.SourceKeyHMAC, selected.ArchivedRow.PayloadHMAC, selected.ArchivedRow.FieldHMAC, nextID, digest))
		nextID++
	}
	for _, selected := range selection.IdentityConflicts {
		value := deferredConflictValue(selected)
		value.ID = nextID
		reader.conflicts[nextID] = value
		digest, digestErr := contactapp.HistoricalDeferredIdentityConflictDigest(value)
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		rows = append(rows, deferredIdentityHistoryReconciliationRow(selected.ArchivedRow.TableID, DeferredConflictHistoryTarget, selected.ArchivedRow.SourceKeyHMAC, selected.ArchivedRow.PayloadHMAC, selected.ArchivedRow.FieldHMAC, nextID, digest))
		nextID++
	}
	for _, selected := range selection.MissingCustomerRootMaps {
		value := missingRootValue(selected, source.options)
		value.ID = nextID
		reader.missingRoots[nextID] = value
		digest, digestErr := contactapp.HistoricalMissingRootIdentityDigest(value)
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		rows = append(rows, deferredIdentityHistoryReconciliationRow(selected.ArchivedRow.TableID, MissingRootIdentityTarget, selected.ArchivedRow.SourceKeyHMAC, selected.ArchivedRow.PayloadHMAC, selected.ArchivedRow.FieldHMAC, nextID, digest))
		nextID++
	}
	return deferredIdentityHistoryReconcileFixture{selection: selection, options: source.options, reader: reader, rows: rows}
}

func deferredIdentityHistoryReconciliationRow(table, target string, source, payload, field [sha256.Size]byte, id int64, digest [sha256.Size]byte) reconciliationRow {
	domain := DeferredIdentityHistoryDomain
	return reconciliationRow{
		TableID: table, SourceKeyDigest: source[:], PayloadDigest: payload[:], FieldDigest: field[:],
		Disposition: "import", TargetDomain: &domain, TargetTable: &target, TargetID: deferredIdentityHistoryStringPointer(strconv.FormatInt(id, 10)), TargetDigest: digest[:], Verified: true,
	}
}

func deferredIdentityHistoryStringPointer(value string) *string { return &value }

func (fixture deferredIdentityHistoryReconcileFixture) clone() deferredIdentityHistoryReconcileFixture {
	copy := fixture
	copy.reader = fixture.reader.clone()
	copy.rows = make([]reconciliationRow, len(fixture.rows))
	for index, row := range fixture.rows {
		copy.rows[index] = row
		copy.rows[index].SourceKeyDigest = append([]byte(nil), row.SourceKeyDigest...)
		copy.rows[index].PayloadDigest = append([]byte(nil), row.PayloadDigest...)
		copy.rows[index].FieldDigest = append([]byte(nil), row.FieldDigest...)
		copy.rows[index].TargetDigest = append([]byte(nil), row.TargetDigest...)
		if row.TargetDomain != nil {
			value := *row.TargetDomain
			copy.rows[index].TargetDomain = &value
		}
		if row.TargetTable != nil {
			value := *row.TargetTable
			copy.rows[index].TargetTable = &value
		}
		if row.TargetID != nil {
			value := *row.TargetID
			copy.rows[index].TargetID = &value
		}
	}
	return copy
}

type deferredIdentityHistoryReconcileReaderFake struct {
	people       map[int64]contactport.HistoricalDeferredPerson
	conflicts    map[int64]contactport.HistoricalDeferredIdentityConflict
	missingRoots map[int64]contactport.HistoricalMissingRootIdentity
}

func (reader *deferredIdentityHistoryReconcileReaderFake) GetHistoricalDeferredPerson(_ context.Context, id int64) (contactport.HistoricalDeferredPerson, error) {
	value, found := reader.people[id]
	if !found {
		return contactport.HistoricalDeferredPerson{}, errors.New("missing person")
	}
	return value, nil
}

func (reader *deferredIdentityHistoryReconcileReaderFake) ListHistoricalDeferredPerson(context.Context, contactport.DeferredIdentityHistoryQuery) ([]contactport.HistoricalDeferredPerson, int64, error) {
	return nil, 0, errors.New("not used")
}

func (reader *deferredIdentityHistoryReconcileReaderFake) GetHistoricalDeferredIdentityConflict(_ context.Context, id int64) (contactport.HistoricalDeferredIdentityConflict, error) {
	value, found := reader.conflicts[id]
	if !found {
		return contactport.HistoricalDeferredIdentityConflict{}, errors.New("missing conflict")
	}
	return value, nil
}

func (reader *deferredIdentityHistoryReconcileReaderFake) ListHistoricalDeferredIdentityConflict(context.Context, contactport.DeferredIdentityHistoryQuery) ([]contactport.HistoricalDeferredIdentityConflict, int64, error) {
	return nil, 0, errors.New("not used")
}

func (reader *deferredIdentityHistoryReconcileReaderFake) GetHistoricalMissingRootIdentity(_ context.Context, id int64) (contactport.HistoricalMissingRootIdentity, error) {
	value, found := reader.missingRoots[id]
	if !found {
		return contactport.HistoricalMissingRootIdentity{}, errors.New("missing root")
	}
	return value, nil
}

func (reader *deferredIdentityHistoryReconcileReaderFake) ListHistoricalMissingRootIdentity(context.Context, contactport.DeferredIdentityHistoryQuery) ([]contactport.HistoricalMissingRootIdentity, int64, error) {
	return nil, 0, errors.New("not used")
}

func (reader *deferredIdentityHistoryReconcileReaderFake) clone() *deferredIdentityHistoryReconcileReaderFake {
	copy := &deferredIdentityHistoryReconcileReaderFake{
		people: make(map[int64]contactport.HistoricalDeferredPerson, len(reader.people)), conflicts: make(map[int64]contactport.HistoricalDeferredIdentityConflict, len(reader.conflicts)), missingRoots: make(map[int64]contactport.HistoricalMissingRootIdentity, len(reader.missingRoots)),
	}
	for id, value := range reader.people {
		copy.people[id] = value
	}
	for id, value := range reader.conflicts {
		copy.conflicts[id] = value
	}
	for id, value := range reader.missingRoots {
		copy.missingRoots[id] = value
	}
	return copy
}
