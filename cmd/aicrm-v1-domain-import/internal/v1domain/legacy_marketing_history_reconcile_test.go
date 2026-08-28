package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type legacyMarketingHistoryReconcileReader struct {
	state segmentport.HistoricalLegacyMarketingState
	value segmentport.HistoricalLegacyMarketingValue
	err   error
}

func (reader legacyMarketingHistoryReconcileReader) GetHistoricalLegacyMarketingState(_ context.Context, id int64) (segmentport.HistoricalLegacyMarketingState, error) {
	if reader.err != nil || reader.state.ID != id {
		return segmentport.HistoricalLegacyMarketingState{}, errors.New("state missing")
	}
	return reader.state, nil
}

func (reader legacyMarketingHistoryReconcileReader) ListHistoricalLegacyMarketingState(context.Context, segmentport.LegacyMarketingHistoryQuery) ([]segmentport.HistoricalLegacyMarketingState, int64, error) {
	return nil, 0, errors.New("not used")
}

func (reader legacyMarketingHistoryReconcileReader) GetHistoricalLegacyMarketingValue(_ context.Context, id int64) (segmentport.HistoricalLegacyMarketingValue, error) {
	if reader.err != nil || reader.value.ID != id {
		return segmentport.HistoricalLegacyMarketingValue{}, errors.New("value missing")
	}
	return reader.value, nil
}

func (reader legacyMarketingHistoryReconcileReader) ListHistoricalLegacyMarketingValue(context.Context, segmentport.LegacyMarketingHistoryQuery) ([]segmentport.HistoricalLegacyMarketingValue, int64, error) {
	return nil, 0, errors.New("not used")
}

func TestVerifyLegacyMarketingHistoryRowPinsAllArchiveAndPrivateFacts(t *testing.T) {
	for _, state := range []bool{true, false} {
		name := "value"
		if state {
			name = "state"
		}
		t.Run(name, func(t *testing.T) {
			reader, row := legacyMarketingHistoryReconcileFixture(t, state)
			proof, err := verifyLegacyMarketingHistoryRow(context.Background(), reader, row)
			if err != nil || proof != "history_only:"+hex.EncodeToString(row.TargetDigest) {
				t.Fatalf("proof=%q err=%v", proof, err)
			}
			for name, mutate := range map[string]func(*legacyMarketingHistoryReconcileReader, *reconciliationRow){
				"source_key":    func(_ *legacyMarketingHistoryReconcileReader, row *reconciliationRow) { row.SourceKeyDigest[0]++ },
				"payload":       func(_ *legacyMarketingHistoryReconcileReader, row *reconciliationRow) { row.PayloadDigest[0]++ },
				"field":         func(_ *legacyMarketingHistoryReconcileReader, row *reconciliationRow) { row.FieldDigest[0]++ },
				"target_digest": func(_ *legacyMarketingHistoryReconcileReader, row *reconciliationRow) { row.TargetDigest[0]++ },
				"source_table": func(_ *legacyMarketingHistoryReconcileReader, row *reconciliationRow) {
					if state {
						row.TableID = legacyMarketingValueTable
					} else {
						row.TableID = legacyMarketingStateTable
					}
				},
				"target_table": func(_ *legacyMarketingHistoryReconcileReader, row *reconciliationRow) {
					if state {
						target := legacyMarketingValueTarget
						row.TargetTable = &target
					} else {
						target := legacyMarketingStateTarget
						row.TargetTable = &target
					}
				},
				"domain": func(_ *legacyMarketingHistoryReconcileReader, row *reconciliationRow) {
					domain := "contact"
					row.TargetDomain = &domain
				},
				"id": func(_ *legacyMarketingHistoryReconcileReader, row *reconciliationRow) {
					id := "0071"
					row.TargetID = &id
				},
				"actual_id": func(reader *legacyMarketingHistoryReconcileReader, _ *reconciliationRow) {
					if state {
						reader.state.ID++
					} else {
						reader.value.ID++
					}
				},
				"private": func(reader *legacyMarketingHistoryReconcileReader, _ *reconciliationRow) {
					if state {
						reader.state.ExternalUserIDDigest[0]++
						reader.state.StatePayloadDigest[0]++
						batch := int64(9)
						reader.state.LastBatchSourceID = &batch
					} else {
						reader.value.ScoreBreakdownDigest[0]++
						reader.value.StatePayloadDigest[0]++
					}
				},
				"read_error": func(reader *legacyMarketingHistoryReconcileReader, _ *reconciliationRow) {
					reader.err = errors.New("unavailable")
				},
			} {
				t.Run(name, func(t *testing.T) {
					candidateReader, candidateRow := legacyMarketingHistoryReconcileFixture(t, state)
					mutate(&candidateReader, &candidateRow)
					if _, err := verifyLegacyMarketingHistoryRow(context.Background(), candidateReader, candidateRow); !errors.Is(err, ErrConflict) {
						t.Fatalf("drift accepted: %v", err)
					}
				})
			}
			if _, err := verifyLegacyMarketingHistoryRow(context.Background(), nil, row); !errors.Is(err, ErrConflict) {
				t.Fatalf("nil reader error = %v", err)
			}
		})
	}
}

func TestReconcileLegacyMarketingHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	var pool *pgxpool.Pool
	if _, err := ReconcileLegacyMarketingHistory(context.Background(), pool, "v1-legacy-marketing-history-a2", "archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong version error = %v", err)
	}
}

func legacyMarketingHistoryReconcileFixture(t *testing.T, state bool) (legacyMarketingHistoryReconcileReader, reconciliationRow) {
	t.Helper()
	at := time.Date(2026, 8, 28, 13, 14, 15, 123456000, time.UTC)
	reader := legacyMarketingHistoryReconcileReader{
		state: segmentport.HistoricalLegacyMarketingState{ID: 71, SourceKeyDigest: legacyMarketingHistoryDigest(1), SourcePayloadDigest: legacyMarketingHistoryDigest(2), SourceFieldDigest: legacyMarketingHistoryDigest(3), SourceID: -1, ExternalUserIDDigest: legacyMarketingHistoryDigest(4), ScenarioKey: "", MarketingPhase: "", PhaseLabel: "", PhaseReason: "", LifecycleStatus: "", LastBatchStatus: "", LastBatchWindowStart: "", LastBatchWindowEnd: "", LastTriggerMessageAt: "", ExitReason: "", StatePayloadDigest: legacyMarketingHistoryDigest(5), CreatedAt: at, UpdatedAt: at},
		value: segmentport.HistoricalLegacyMarketingValue{ID: 72, SourceKeyDigest: legacyMarketingHistoryDigest(11), SourcePayloadDigest: legacyMarketingHistoryDigest(12), SourceFieldDigest: legacyMarketingHistoryDigest(13), SourceID: 0, ExternalUserIDDigest: legacyMarketingHistoryDigest(14), ScenarioKey: "", ValueSegment: "", SegmentLabel: "", Score: -2, ScoreBreakdownDigest: legacyMarketingHistoryDigest(15), StatePayloadDigest: legacyMarketingHistoryDigest(16), CreatedAt: at, UpdatedAt: at},
	}
	if state {
		digest, err := segmentapp.HistoricalLegacyMarketingStateDigest(reader.state)
		if err != nil {
			t.Fatal(err)
		}
		return reader, legacyMarketingHistoryReconcileRow(legacyMarketingStateTable, legacyMarketingStateTarget, reader.state.ID, reader.state.SourceKeyDigest, reader.state.SourcePayloadDigest, reader.state.SourceFieldDigest, digest)
	}
	digest, err := segmentapp.HistoricalLegacyMarketingValueDigest(reader.value)
	if err != nil {
		t.Fatal(err)
	}
	return reader, legacyMarketingHistoryReconcileRow(legacyMarketingValueTable, legacyMarketingValueTarget, reader.value.ID, reader.value.SourceKeyDigest, reader.value.SourcePayloadDigest, reader.value.SourceFieldDigest, digest)
}

func legacyMarketingHistoryReconcileRow(tableID, targetTable string, id int64, source, payload, field, digest [sha256.Size]byte) reconciliationRow {
	domain, targetID := legacyMarketingHistoryDomain, strconv.FormatInt(id, 10)
	return reconciliationRow{TableID: tableID, SourceKeyDigest: source[:], PayloadDigest: payload[:], FieldDigest: field[:], TargetDomain: &domain, TargetTable: &targetTable, TargetID: &targetID, TargetDigest: digest[:]}
}

func legacyMarketingHistoryDigest(first byte) [sha256.Size]byte {
	var value [sha256.Size]byte
	value[0] = first
	return value
}
