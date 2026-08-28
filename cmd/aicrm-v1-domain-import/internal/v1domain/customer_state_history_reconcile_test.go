package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"
	"testing"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type customerStateReaderFake struct {
	snapshot contactport.HistoricalCustomerStatusSnapshot
	change   contactport.HistoricalCustomerStatusChange
	term     contactport.HistoricalClassTermTagMapping
}

func (reader customerStateReaderFake) GetHistoricalCustomerStatusSnapshot(_ context.Context, id int64) (contactport.HistoricalCustomerStatusSnapshot, error) {
	if id != reader.snapshot.ID {
		return contactport.HistoricalCustomerStatusSnapshot{}, contactport.ErrCustomerStateHistoryUnavailable
	}
	return reader.snapshot, nil
}
func (reader customerStateReaderFake) ListHistoricalCustomerStatusSnapshot(_ context.Context, _ contactport.CustomerStateHistoryQuery) ([]contactport.HistoricalCustomerStatusSnapshot, int64, error) {
	return []contactport.HistoricalCustomerStatusSnapshot{reader.snapshot}, 1, nil
}
func (reader customerStateReaderFake) GetHistoricalCustomerStatusChange(_ context.Context, id int64) (contactport.HistoricalCustomerStatusChange, error) {
	if id != reader.change.ID {
		return contactport.HistoricalCustomerStatusChange{}, contactport.ErrCustomerStateHistoryUnavailable
	}
	return reader.change, nil
}
func (reader customerStateReaderFake) ListHistoricalCustomerStatusChange(_ context.Context, _ contactport.CustomerStateHistoryQuery) ([]contactport.HistoricalCustomerStatusChange, int64, error) {
	return []contactport.HistoricalCustomerStatusChange{reader.change}, 1, nil
}
func (reader customerStateReaderFake) GetHistoricalClassTermTagMapping(_ context.Context, id int64) (contactport.HistoricalClassTermTagMapping, error) {
	if id != reader.term.ID {
		return contactport.HistoricalClassTermTagMapping{}, contactport.ErrCustomerStateHistoryUnavailable
	}
	return reader.term, nil
}
func (reader customerStateReaderFake) ListHistoricalClassTermTagMapping(_ context.Context, _ contactport.CustomerStateHistoryQuery) ([]contactport.HistoricalClassTermTagMapping, int64, error) {
	return []contactport.HistoricalClassTermTagMapping{reader.term}, 1, nil
}

func TestVerifyCustomerStateHistoryRowChecksAllFactsAndFieldDigest(t *testing.T) {
	reader := customerStateHistoryTestReader()
	for _, test := range []struct {
		table, target       string
		id                  int64
		digest              [sha256.Size]byte
		key, payload, field [sha256.Size]byte
	}{
		{customerStateHistorySnapshotTable, customerStateHistorySnapshotTarget, reader.snapshot.ID, mustCustomerStateSnapshotDigest(t, reader.snapshot), reader.snapshot.SourceKeyDigest, reader.snapshot.SourcePayloadDigest, reader.snapshot.SourceFieldDigest},
		{customerStateHistoryChangeTable, customerStateHistoryChangeTarget, reader.change.ID, mustCustomerStateChangeDigest(t, reader.change), reader.change.SourceKeyDigest, reader.change.SourcePayloadDigest, reader.change.SourceFieldDigest},
		{customerStateHistoryTermTable, customerStateHistoryTermTarget, reader.term.ID, mustCustomerStateTermDigest(t, reader.term), reader.term.SourceKeyDigest, reader.term.SourcePayloadDigest, reader.term.SourceFieldDigest},
	} {
		t.Run(test.target, func(t *testing.T) {
			row := customerStateHistoryReconciliationRow(test.table, test.target, test.id, test.digest, test.key, test.payload, test.field)
			if proof, err := verifyCustomerStateHistoryRow(context.Background(), reader, row); err != nil || proof == "" {
				t.Fatalf("proof=%q err=%v", proof, err)
			}
		})
	}
}

func TestVerifyCustomerStateHistoryRowFailsClosedOnFieldOrTargetDrift(t *testing.T) {
	reader := customerStateHistoryTestReader()
	row := customerStateHistoryReconciliationRow(customerStateHistorySnapshotTable, customerStateHistorySnapshotTarget, reader.snapshot.ID, mustCustomerStateSnapshotDigest(t, reader.snapshot), reader.snapshot.SourceKeyDigest, reader.snapshot.SourcePayloadDigest, reader.snapshot.SourceFieldDigest)
	driftedField := customerStateDigest(99)
	row.FieldDigest = driftedField[:]
	if _, err := verifyCustomerStateHistoryRow(context.Background(), reader, row); err == nil {
		t.Fatal("field digest drift accepted")
	}
	row = customerStateHistoryReconciliationRow(customerStateHistorySnapshotTable, customerStateHistorySnapshotTarget, reader.snapshot.ID, mustCustomerStateSnapshotDigest(t, reader.snapshot), reader.snapshot.SourceKeyDigest, reader.snapshot.SourcePayloadDigest, reader.snapshot.SourceFieldDigest)
	reader.snapshot.SignupStatus = "drift"
	if _, err := verifyCustomerStateHistoryRow(context.Background(), reader, row); err == nil {
		t.Fatal("target drift accepted")
	}
}

func customerStateHistoryTestReader() customerStateReaderFake {
	at := time.Date(2026, 8, 28, 1, 2, 3, 456000000, time.UTC)
	return customerStateReaderFake{
		snapshot: contactport.HistoricalCustomerStatusSnapshot{ID: 1, SourceKeyDigest: customerStateDigest(1), SourcePayloadDigest: customerStateDigest(2), SourceFieldDigest: customerStateDigest(3), SignupStatus: "", SignupLabelName: "label", CustomerNameSnapshot: "customer", OwnerUserIDSnapshot: "owner", SetByUserIDDigest: customerStateDigest(4), SetAt: at, WeComTagSyncStatus: "", WeComTagSyncErrorHash: customerStateDigest(5), StatusFlagsDigest: customerStateDigest(6), CreatedAt: at, UpdatedAt: at, UnionID: "union"},
		change:   contactport.HistoricalCustomerStatusChange{ID: 2, SourceKeyDigest: customerStateDigest(7), SourcePayloadDigest: customerStateDigest(8), SourceFieldDigest: customerStateDigest(9), SourceID: -1, OldSignupStatus: "old", NewSignupStatus: "new", OldLabelName: "", NewLabelName: "label", CustomerNameSnapshot: "customer", OwnerUserIDSnapshot: "owner", SetByUserIDDigest: customerStateDigest(10), SetAt: at, WeComTagSyncStatus: "", WeComTagSyncErrorHash: customerStateDigest(11), StatusFlagsDigest: customerStateDigest(12), CreatedAt: at, UnionID: "union"},
		term:     contactport.HistoricalClassTermTagMapping{ID: 3, SourceKeyDigest: customerStateDigest(13), SourcePayloadDigest: customerStateDigest(14), SourceFieldDigest: customerStateDigest(15), SourceID: 0, TagGroupName: "group", TagName: "tag", ClassTermNo: -1, ClassTermLabel: "", OriginalActive: false, CreatedAt: at, UpdatedAt: at, StrategySourceID: "strategy", GroupSourceID: "group", TagSourceID: "tag"},
	}
}
func customerStateHistoryReconciliationRow(table, target string, id int64, digest, key, payload, field [sha256.Size]byte) reconciliationRow {
	domain, tableName, targetID := customerStateHistoryDomain, target, strconv.FormatInt(id, 10)
	return reconciliationRow{TableID: table, SourceKeyDigest: key[:], PayloadDigest: payload[:], FieldDigest: field[:], TargetDomain: &domain, TargetTable: &tableName, TargetID: &targetID, TargetDigest: digest[:]}
}
func mustCustomerStateSnapshotDigest(t *testing.T, value contactport.HistoricalCustomerStatusSnapshot) [sha256.Size]byte {
	t.Helper()
	digest, err := contactapp.HistoricalCustomerStatusSnapshotDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func mustCustomerStateChangeDigest(t *testing.T, value contactport.HistoricalCustomerStatusChange) [sha256.Size]byte {
	t.Helper()
	digest, err := contactapp.HistoricalCustomerStatusChangeDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func mustCustomerStateTermDigest(t *testing.T, value contactport.HistoricalClassTermTagMapping) [sha256.Size]byte {
	t.Helper()
	digest, err := contactapp.HistoricalClassTermTagMappingDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func customerStateDigest(value byte) [sha256.Size]byte { return sha256.Sum256([]byte{value}) }
