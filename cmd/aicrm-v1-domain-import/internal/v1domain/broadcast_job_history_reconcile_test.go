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

type broadcastJobHistoryReaderFake struct {
	value outboundport.HistoricalBroadcastJob
	err   error
}

func (reader *broadcastJobHistoryReaderFake) GetHistoricalBroadcastJob(_ context.Context, id int64) (outboundport.HistoricalBroadcastJob, error) {
	if reader == nil || reader.err != nil {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	if id != reader.value.ID {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryConflict
	}
	return reader.value, nil
}

func (reader *broadcastJobHistoryReaderFake) ListHistoricalBroadcastJobs(context.Context, outboundport.BroadcastJobHistoryQuery) ([]outboundport.HistoricalBroadcastJob, int64, error) {
	if reader == nil || reader.err != nil {
		return nil, 0, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	return []outboundport.HistoricalBroadcastJob{reader.value}, 1, nil
}

func broadcastJobHistoryDigest(value byte) [sha256.Size]byte {
	return sha256.Sum256([]byte{value})
}

func broadcastJobHistoryReconcileFixture(t *testing.T) (*broadcastJobHistoryReaderFake, reconciliationRow) {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.FixedZone("V1", 8*60*60))
	optionalText := "legacy"
	optionalDigest := broadcastJobHistoryDigest(9)
	value := outboundport.HistoricalBroadcastJob{
		ID: 71, SourceID: 9007199254740993, OriginalSourceType: "legacy_unknown", SourceReferenceDigest: broadcastJobHistoryDigest(1), SourceTable: "legacy_table",
		ScheduledFor: stamp, Priority: -1, BatchKeyDigest: broadcastJobHistoryDigest(2), OriginalStatus: "old_status", RequiresApproval: true,
		ApprovedByDigest: broadcastJobHistoryDigest(3), CancelledByDigest: broadcastJobHistoryDigest(4), CancelReasonDigest: broadcastJobHistoryDigest(5), TargetCount: -2,
		TargetSummaryDigest: broadcastJobHistoryDigest(6), ContentType: "legacy_blob", ContentPayloadDigest: broadcastJobHistoryDigest(7), ContentSummaryDigest: broadcastJobHistoryDigest(8),
		AttemptCount: -3, LastErrorDigest: broadcastJobHistoryDigest(10), SentCount: -4, FailedCount: -5, TraceIDDigest: broadcastJobHistoryDigest(11), CreatedByDigest: broadcastJobHistoryDigest(12),
		CreatedAt: stamp, UpdatedAt: stamp, ClaimTokenDigest: broadcastJobHistoryDigest(13), BusinessDomain: &optionalText, IdempotencyKeyDigest: &optionalDigest,
		Channel: &optionalText, TargetKind: &optionalText, FailureType: &optionalText, RetryPolicyDigest: broadcastJobHistoryDigest(14), MetadataDigest: broadcastJobHistoryDigest(15),
		TargetUnionIDsDigest: broadcastJobHistoryDigest(16), MaxAttempts: -6, SideEffectExecuted: true, ProviderResultReceived: true, ResultSummaryDigest: broadcastJobHistoryDigest(17),
		ReconciliationRequired: true, HoldReasonDigest: broadcastJobHistoryDigest(18), ExecutionIDDigest: broadcastJobHistoryDigest(19), ExecutionOwnerDigest: broadcastJobHistoryDigest(20),
		SourceKeyDigest: broadcastJobHistoryDigest(21), SourcePayloadDigest: broadcastJobHistoryDigest(22), SourceFieldDigest: broadcastJobHistoryDigest(23), RedactedRoots: []string{"claim_token"},
	}
	digest, err := outboundapp.HistoricalBroadcastJobDigest(value)
	if err != nil {
		t.Fatal("fixture_digest_failed")
	}
	domain, table, targetID := "outbound", broadcastJobHistoryTargetTable, strconv.FormatInt(value.ID, 10)
	return &broadcastJobHistoryReaderFake{value: value}, reconciliationRow{TableID: broadcastJobHistoryTableID,
		SourceKeyDigest: value.SourceKeyDigest[:], PayloadDigest: value.SourcePayloadDigest[:], FieldDigest: value.SourceFieldDigest[:],
		TargetDomain: &domain, TargetTable: &table, TargetID: &targetID, TargetDigest: digest[:]}
}

func TestVerifyBroadcastJobHistoryRowChecksCompletePrivateProjection(t *testing.T) {
	reader, row := broadcastJobHistoryReconcileFixture(t)
	proof, err := verifyBroadcastJobHistoryRow(context.Background(), reader, row)
	digest, _ := outboundapp.HistoricalBroadcastJobDigest(reader.value)
	if err != nil || proof != "history_only:"+hex.EncodeToString(digest[:]) {
		t.Fatal("complete_history_projection_rejected")
	}
	for _, digest := range [][32]byte{reader.value.SourceReferenceDigest, reader.value.BatchKeyDigest, reader.value.ApprovedByDigest, reader.value.CancelledByDigest,
		reader.value.CancelReasonDigest, reader.value.TargetSummaryDigest, reader.value.ContentPayloadDigest, reader.value.ContentSummaryDigest, reader.value.LastErrorDigest,
		reader.value.TraceIDDigest, reader.value.CreatedByDigest, reader.value.ClaimTokenDigest, reader.value.RetryPolicyDigest, reader.value.MetadataDigest,
		reader.value.TargetUnionIDsDigest, reader.value.ResultSummaryDigest, reader.value.HoldReasonDigest, reader.value.ExecutionIDDigest, reader.value.ExecutionOwnerDigest} {
		if digest == ([sha256.Size]byte{}) {
			t.Fatal("private_digest_missing_from_fixture")
		}
	}
}

func TestVerifyBroadcastJobHistoryRowFailsClosedOnDriftAndMissingReader(t *testing.T) {
	for name, mutate := range map[string]func(*broadcastJobHistoryReaderFake, *reconciliationRow){
		"private-digest": func(reader *broadcastJobHistoryReaderFake, _ *reconciliationRow) {
			reader.value.ContentPayloadDigest[0]++
		},
		"source-key":    func(_ *broadcastJobHistoryReaderFake, row *reconciliationRow) { row.SourceKeyDigest[0]++ },
		"payload":       func(_ *broadcastJobHistoryReaderFake, row *reconciliationRow) { row.PayloadDigest[0]++ },
		"field":         func(_ *broadcastJobHistoryReaderFake, row *reconciliationRow) { row.FieldDigest[0]++ },
		"target-digest": func(_ *broadcastJobHistoryReaderFake, row *reconciliationRow) { row.TargetDigest[0]++ },
		"domain": func(_ *broadcastJobHistoryReaderFake, row *reconciliationRow) {
			value := "campaign"
			row.TargetDomain = &value
		},
		"table": func(_ *broadcastJobHistoryReaderFake, row *reconciliationRow) {
			value := "other_history"
			row.TargetTable = &value
		},
		"id": func(_ *broadcastJobHistoryReaderFake, row *reconciliationRow) {
			value := "0071"
			row.TargetID = &value
		},
	} {
		t.Run(name, func(t *testing.T) {
			reader, row := broadcastJobHistoryReconcileFixture(t)
			mutate(reader, &row)
			if _, err := verifyBroadcastJobHistoryRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
				t.Fatal("projection_drift_accepted")
			}
		})
	}
	reader, row := broadcastJobHistoryReconcileFixture(t)
	reader.err = errors.New("reader unavailable")
	if _, err := verifyBroadcastJobHistoryRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatal("reader_error_accepted")
	}
	var missing *broadcastJobHistoryReaderFake
	if _, err := verifyBroadcastJobHistoryRow(context.Background(), missing, row); !errors.Is(err, ErrConflict) {
		t.Fatal("typed_nil_reader_accepted")
	}
}

func TestReconcileBroadcastJobHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	if _, err := ReconcileBroadcastJobHistory(context.Background(), nil, "v1-broadcast-job-history-a2", "archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong_version_reached_database")
	}
}
