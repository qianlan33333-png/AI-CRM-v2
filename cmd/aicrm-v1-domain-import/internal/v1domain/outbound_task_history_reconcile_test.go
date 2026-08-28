package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type outboundTaskHistoryReaderFake struct {
	value outboundport.HistoricalOutboundTask
	err   error
}

func (reader *outboundTaskHistoryReaderFake) GetHistoricalOutboundTask(_ context.Context, id int64) (outboundport.HistoricalOutboundTask, error) {
	if reader == nil || reader.err != nil {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	if id != reader.value.ID {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryConflict
	}
	return reader.value, nil
}

func (reader *outboundTaskHistoryReaderFake) ListHistoricalOutboundTasks(context.Context, outboundport.OutboundTaskHistoryQuery) ([]outboundport.HistoricalOutboundTask, int64, error) {
	if reader == nil || reader.err != nil {
		return nil, 0, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	return []outboundport.HistoricalOutboundTask{reader.value}, 1, nil
}

func outboundTaskReconcileDigest(value byte) [sha256.Size]byte {
	return sha256.Sum256([]byte{value})
}

func outboundTaskHistoryReconcileFixture(t *testing.T) (*outboundTaskHistoryReaderFake, reconciliationRow) {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.FixedZone("V1", 8*60*60))
	parent, legacy := int64(9), int64(-5)
	optionalDigest := outboundTaskReconcileDigest(9)
	value := outboundport.HistoricalOutboundTask{
		ID: 71, SourceID: -9007199254740993, TaskType: "", Status: "legacy_status", CreatedAt: stamp,
		BroadcastJobHistoryID: &parent, LegacyBroadcastJobID: &legacy, WeComTaskIDDigest: &optionalDigest,
		RequestPayloadDigest: outboundTaskReconcileDigest(1), ResponsePayloadDigest: outboundTaskReconcileDigest(2), TraceIDDigest: outboundTaskReconcileDigest(3),
		SourceKeyDigest: outboundTaskReconcileDigest(21), SourcePayloadDigest: outboundTaskReconcileDigest(22), SourceFieldDigest: outboundTaskReconcileDigest(23),
		RedactedRoots: []string{"request_payload"},
	}
	digest, err := outboundapp.HistoricalOutboundTaskDigest(value)
	if err != nil {
		t.Fatal("fixture_digest_failed")
	}
	domain, table, targetID := "outbound", outboundTaskHistoryTargetTable, strconv.FormatInt(value.ID, 10)
	return &outboundTaskHistoryReaderFake{value: value}, reconciliationRow{TableID: outboundTaskHistoryTableID,
		SourceKeyDigest: value.SourceKeyDigest[:], PayloadDigest: value.SourcePayloadDigest[:], FieldDigest: value.SourceFieldDigest[:],
		TargetDomain: &domain, TargetTable: &table, TargetID: &targetID, TargetDigest: digest[:]}
}

func TestVerifyOutboundTaskHistoryRowChecksCompletePrivateProjection(t *testing.T) {
	reader, row := outboundTaskHistoryReconcileFixture(t)
	proof, err := verifyOutboundTaskHistoryRow(context.Background(), reader, row)
	digest, _ := outboundapp.HistoricalOutboundTaskDigest(reader.value)
	if err != nil || proof != "history_only:"+hex.EncodeToString(digest[:]) {
		t.Fatal("complete_history_projection_rejected")
	}
	for _, digest := range [][32]byte{reader.value.RequestPayloadDigest, reader.value.ResponsePayloadDigest, reader.value.TraceIDDigest, *reader.value.WeComTaskIDDigest} {
		if digest == ([sha256.Size]byte{}) {
			t.Fatal("private_digest_missing_from_fixture")
		}
	}
}

func TestVerifyOutboundTaskHistoryRowFailsClosedOnDriftAndMissingReader(t *testing.T) {
	for name, mutate := range map[string]func(*outboundTaskHistoryReaderFake, *reconciliationRow){
		"private-digest": func(reader *outboundTaskHistoryReaderFake, _ *reconciliationRow) {
			reader.value.RequestPayloadDigest[0]++
		},
		"source-key":    func(_ *outboundTaskHistoryReaderFake, row *reconciliationRow) { row.SourceKeyDigest[0]++ },
		"payload":       func(_ *outboundTaskHistoryReaderFake, row *reconciliationRow) { row.PayloadDigest[0]++ },
		"field":         func(_ *outboundTaskHistoryReaderFake, row *reconciliationRow) { row.FieldDigest[0]++ },
		"target-digest": func(_ *outboundTaskHistoryReaderFake, row *reconciliationRow) { row.TargetDigest[0]++ },
		"domain": func(_ *outboundTaskHistoryReaderFake, row *reconciliationRow) {
			value := "campaign"
			row.TargetDomain = &value
		},
		"table": func(_ *outboundTaskHistoryReaderFake, row *reconciliationRow) {
			value := "other_history"
			row.TargetTable = &value
		},
		"id": func(_ *outboundTaskHistoryReaderFake, row *reconciliationRow) {
			value := "0071"
			row.TargetID = &value
		},
	} {
		t.Run(name, func(t *testing.T) {
			reader, row := outboundTaskHistoryReconcileFixture(t)
			mutate(reader, &row)
			if _, err := verifyOutboundTaskHistoryRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
				t.Fatal("projection_drift_accepted")
			}
		})
	}
	reader, row := outboundTaskHistoryReconcileFixture(t)
	reader.err = errors.New("reader unavailable")
	if _, err := verifyOutboundTaskHistoryRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatal("reader_error_accepted")
	}
	var missing *outboundTaskHistoryReaderFake
	if _, err := verifyOutboundTaskHistoryRow(context.Background(), missing, row); !errors.Is(err, ErrConflict) {
		t.Fatal("typed_nil_reader_accepted")
	}
}

func TestReconcileOutboundTaskHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	if _, err := ReconcileOutboundTaskHistory(context.Background(), nil, "v1-outbound-task-history-a2", "archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong_version_reached_database")
	}
}
