package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"testing"
	"time"

	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestReconcileMarketingStateHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	if _, err := ReconcileMarketingStateHistory(context.Background(), nil, "other", "archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong version err=%v", err)
	}
}

type marketingStateReaderFake struct {
	snapshot      segmentport.HistoricalMarketingStateSnapshot
	change        segmentport.HistoricalMarketingStateChange
	valueSnapshot segmentport.HistoricalValueSegmentSnapshot
	valueChange   segmentport.HistoricalValueSegmentChange
}

func (reader marketingStateReaderFake) GetHistoricalMarketingStateSnapshot(_ context.Context, id int64) (segmentport.HistoricalMarketingStateSnapshot, error) {
	if id != reader.snapshot.ID {
		return segmentport.HistoricalMarketingStateSnapshot{}, segmentport.ErrMarketingStateHistoryUnavailable
	}
	return reader.snapshot, nil
}
func (reader marketingStateReaderFake) ListHistoricalMarketingStateSnapshot(_ context.Context, _ segmentport.MarketingStateHistoryQuery) ([]segmentport.HistoricalMarketingStateSnapshot, int64, error) {
	return []segmentport.HistoricalMarketingStateSnapshot{reader.snapshot}, 1, nil
}
func (reader marketingStateReaderFake) GetHistoricalMarketingStateChange(_ context.Context, id int64) (segmentport.HistoricalMarketingStateChange, error) {
	if id != reader.change.ID {
		return segmentport.HistoricalMarketingStateChange{}, segmentport.ErrMarketingStateHistoryUnavailable
	}
	return reader.change, nil
}
func (reader marketingStateReaderFake) ListHistoricalMarketingStateChange(_ context.Context, _ segmentport.MarketingStateHistoryQuery) ([]segmentport.HistoricalMarketingStateChange, int64, error) {
	return []segmentport.HistoricalMarketingStateChange{reader.change}, 1, nil
}
func (reader marketingStateReaderFake) GetHistoricalValueSegmentSnapshot(_ context.Context, id int64) (segmentport.HistoricalValueSegmentSnapshot, error) {
	if id != reader.valueSnapshot.ID {
		return segmentport.HistoricalValueSegmentSnapshot{}, segmentport.ErrMarketingStateHistoryUnavailable
	}
	return reader.valueSnapshot, nil
}
func (reader marketingStateReaderFake) ListHistoricalValueSegmentSnapshot(_ context.Context, _ segmentport.MarketingStateHistoryQuery) ([]segmentport.HistoricalValueSegmentSnapshot, int64, error) {
	return []segmentport.HistoricalValueSegmentSnapshot{reader.valueSnapshot}, 1, nil
}
func (reader marketingStateReaderFake) GetHistoricalValueSegmentChange(_ context.Context, id int64) (segmentport.HistoricalValueSegmentChange, error) {
	if id != reader.valueChange.ID {
		return segmentport.HistoricalValueSegmentChange{}, segmentport.ErrMarketingStateHistoryUnavailable
	}
	return reader.valueChange, nil
}
func (reader marketingStateReaderFake) ListHistoricalValueSegmentChange(_ context.Context, _ segmentport.MarketingStateHistoryQuery) ([]segmentport.HistoricalValueSegmentChange, int64, error) {
	return []segmentport.HistoricalValueSegmentChange{reader.valueChange}, 1, nil
}

func TestVerifyMarketingStateHistoryRowChecksAllFactsAndFieldDigest(t *testing.T) {
	reader := marketingStateHistoryTestReader()
	for _, test := range []struct {
		table, target       string
		id                  int64
		digest              [sha256.Size]byte
		key, payload, field [sha256.Size]byte
	}{
		{marketingStateSnapshotTable, marketingStateSnapshotTarget, reader.snapshot.ID, mustMarketingSnapshotDigest(t, reader.snapshot), reader.snapshot.SourceKeyDigest, reader.snapshot.SourcePayloadDigest, reader.snapshot.SourceFieldDigest},
		{marketingStateChangeTable, marketingStateChangeTarget, reader.change.ID, mustMarketingChangeDigest(t, reader.change), reader.change.SourceKeyDigest, reader.change.SourcePayloadDigest, reader.change.SourceFieldDigest},
		{valueSegmentSnapshotTable, valueSegmentSnapshotTarget, reader.valueSnapshot.ID, mustValueSnapshotDigest(t, reader.valueSnapshot), reader.valueSnapshot.SourceKeyDigest, reader.valueSnapshot.SourcePayloadDigest, reader.valueSnapshot.SourceFieldDigest},
		{valueSegmentChangeTable, valueSegmentChangeTarget, reader.valueChange.ID, mustValueChangeDigest(t, reader.valueChange), reader.valueChange.SourceKeyDigest, reader.valueChange.SourcePayloadDigest, reader.valueChange.SourceFieldDigest},
	} {
		t.Run(test.target, func(t *testing.T) {
			row := marketingStateReconciliationRow(test.table, test.target, test.id, test.digest, test.key, test.payload, test.field)
			if proof, err := verifyMarketingStateHistoryRow(context.Background(), reader, row); err != nil || proof == "" {
				t.Fatalf("proof=%q err=%v", proof, err)
			}
		})
	}
}

func TestVerifyMarketingStateHistoryRowFailsClosedOnFieldAndTargetDrift(t *testing.T) {
	reader := marketingStateHistoryTestReader()
	row := marketingStateReconciliationRow(marketingStateSnapshotTable, marketingStateSnapshotTarget, reader.snapshot.ID, mustMarketingSnapshotDigest(t, reader.snapshot), reader.snapshot.SourceKeyDigest, reader.snapshot.SourcePayloadDigest, reader.snapshot.SourceFieldDigest)
	drifted := marketingStateDigest(99)
	row.FieldDigest = drifted[:]
	if _, err := verifyMarketingStateHistoryRow(context.Background(), reader, row); err == nil {
		t.Fatal("field drift accepted")
	}
	row = marketingStateReconciliationRow(marketingStateSnapshotTable, marketingStateSnapshotTarget, reader.snapshot.ID, mustMarketingSnapshotDigest(t, reader.snapshot), reader.snapshot.SourceKeyDigest, reader.snapshot.SourcePayloadDigest, reader.snapshot.SourceFieldDigest)
	reader.snapshot.AutomationKey = "drift"
	if _, err := verifyMarketingStateHistoryRow(context.Background(), reader, row); err == nil {
		t.Fatal("target drift accepted")
	}
}

func marketingStateHistoryTestReader() marketingStateReaderFake {
	at := time.Date(2026, 8, 28, 1, 2, 3, 456000000, time.UTC)
	return marketingStateReaderFake{
		snapshot:      segmentport.HistoricalMarketingStateSnapshot{ID: 1, SourceKeyDigest: marketingStateDigest(1), SourcePayloadDigest: marketingStateDigest(2), SourceFieldDigest: marketingStateDigest(3), ExternalUserIDDigest: marketingStateDigest(4), StatePayloadDigest: marketingStateDigest(5), SourceID: -1, CreatedAt: at, UpdatedAt: at},
		change:        segmentport.HistoricalMarketingStateChange{ID: 2, SourceKeyDigest: marketingStateDigest(6), SourcePayloadDigest: marketingStateDigest(7), SourceFieldDigest: marketingStateDigest(8), ExternalUserIDDigest: marketingStateDigest(9), StatePayloadDigest: marketingStateDigest(10), SourceID: 0, RecordedAt: at, CreatedAt: at},
		valueSnapshot: segmentport.HistoricalValueSegmentSnapshot{ID: 3, SourceKeyDigest: marketingStateDigest(11), SourcePayloadDigest: marketingStateDigest(12), SourceFieldDigest: marketingStateDigest(13), ExternalUserIDDigest: marketingStateDigest(14), MatchedQuestionIDsDigest: marketingStateDigest(15), StatePayloadDigest: marketingStateDigest(16), SourceID: -2, EvaluatedAt: at, ComputedAt: at, CreatedAt: at, UpdatedAt: at},
		valueChange:   segmentport.HistoricalValueSegmentChange{ID: 4, SourceKeyDigest: marketingStateDigest(17), SourcePayloadDigest: marketingStateDigest(18), SourceFieldDigest: marketingStateDigest(19), ExternalUserIDDigest: marketingStateDigest(20), MatchedQuestionIDsDigest: marketingStateDigest(21), StatePayloadDigest: marketingStateDigest(22), SourceID: -3, EvaluatedAt: at, RecordedAt: at, CreatedAt: at},
	}
}

func marketingStateReconciliationRow(table, target string, id int64, digest, key, payload, field [sha256.Size]byte) reconciliationRow {
	domain, tableName, targetID := marketingStateHistoryDomain, target, strconv.FormatInt(id, 10)
	return reconciliationRow{TableID: table, SourceKeyDigest: key[:], PayloadDigest: payload[:], FieldDigest: field[:], TargetDomain: &domain, TargetTable: &tableName, TargetID: &targetID, TargetDigest: digest[:]}
}
func mustMarketingSnapshotDigest(t *testing.T, value segmentport.HistoricalMarketingStateSnapshot) [sha256.Size]byte {
	t.Helper()
	digest, err := segmentapp.HistoricalMarketingStateSnapshotDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func mustMarketingChangeDigest(t *testing.T, value segmentport.HistoricalMarketingStateChange) [sha256.Size]byte {
	t.Helper()
	digest, err := segmentapp.HistoricalMarketingStateChangeDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func mustValueSnapshotDigest(t *testing.T, value segmentport.HistoricalValueSegmentSnapshot) [sha256.Size]byte {
	t.Helper()
	digest, err := segmentapp.HistoricalValueSegmentSnapshotDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func mustValueChangeDigest(t *testing.T, value segmentport.HistoricalValueSegmentChange) [sha256.Size]byte {
	t.Helper()
	digest, err := segmentapp.HistoricalValueSegmentChangeDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func marketingStateDigest(value byte) [sha256.Size]byte { return sha256.Sum256([]byte{value}) }
