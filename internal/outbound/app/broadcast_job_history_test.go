package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

func TestBroadcastJobHistoryWriterCreatesAndReplaysHistoricalTarget(t *testing.T) {
	store := &broadcastJobHistoryStoreFake{values: map[int64]outboundport.HistoricalBroadcastJob{}}
	journal := &broadcastJobHistoryJournalFake{}
	value := broadcastJobHistoryFact()
	writer := NewBroadcastJobHistoryWriter(store, journal)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	first, err := writer.Import(context.Background(), source, value)
	if err != nil || first.Replayed || first.TargetID != 41 || store.creates != 1 || journal.records != 1 {
		t.Fatalf("first_import receipt=%#v err=%v", first, err)
	}
	second, err := writer.Import(context.Background(), source, value)
	if err != nil || !second.Replayed || second.TargetDigest != first.TargetDigest || store.creates != 1 || store.gets != 1 {
		t.Fatalf("replay receipt=%#v err=%v", second, err)
	}
	stored := store.values[first.TargetID]
	if stored.ScheduledFor.Location() != time.UTC || stored.ScheduledFor.Nanosecond()%1000 != 0 || stored.LegacyOutboundTaskID == nil || *stored.LegacyOutboundTaskID != -4 || stored.RedactedRoots[0] != "claim_token" {
		t.Fatalf("historical_fields_changed=%#v", stored)
	}
}

func TestBroadcastJobHistoryWriterRejectsSourcePayloadAndTargetDrift(t *testing.T) {
	store := &broadcastJobHistoryStoreFake{values: map[int64]outboundport.HistoricalBroadcastJob{}}
	journal := &broadcastJobHistoryJournalFake{}
	writer, value := NewBroadcastJobHistoryWriter(store, journal), broadcastJobHistoryFact()
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	if _, err := writer.Import(context.Background(), stringsUpper(source), value); !errors.Is(err, outboundport.ErrBroadcastJobHistoryInvalid) {
		t.Fatalf("noncanonical_source_accepted=%v", err)
	}
	if _, err := writer.Import(context.Background(), source, value); err != nil {
		t.Fatal(err)
	}
	changed := store.values[41]
	changed.OriginalStatus = "mutated"
	store.values[41] = changed
	if _, err := writer.Import(context.Background(), source, value); !errors.Is(err, outboundport.ErrBroadcastJobHistoryConflict) {
		t.Fatalf("target_drift_accepted=%v", err)
	}
}

func TestBroadcastJobHistoryDigestIncludesPrivateNullableAndRootFields(t *testing.T) {
	value := broadcastJobHistoryFact()
	value.ID = 9
	baseline, err := HistoricalBroadcastJobDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*outboundport.HistoricalBroadcastJob){
		func(v *outboundport.HistoricalBroadcastJob) { v.ClaimTokenDigest[0]++ },
		func(v *outboundport.HistoricalBroadcastJob) { v.SourceFieldDigest[0]++ },
		func(v *outboundport.HistoricalBroadcastJob) { v.RedactedRoots = []string{"different"} },
		func(v *outboundport.HistoricalBroadcastJob) { v.LegacyOutboundTaskID = nil },
		func(v *outboundport.HistoricalBroadcastJob) { v.ProviderResultReceived = false },
	} {
		changed := value
		mutate(&changed)
		got, err := HistoricalBroadcastJobDigest(changed)
		if err != nil || got == baseline {
			t.Fatalf("private_field_omitted digest=%x err=%v", got, err)
		}
	}
	bad := broadcastJobHistoryFact()
	bad.SourcePayloadDigest = [32]byte{}
	if _, err := NewBroadcastJobHistoryWriter(&broadcastJobHistoryStoreFake{}, &broadcastJobHistoryJournalFake{}).Import(context.Background(), hex.EncodeToString(bad.SourceKeyDigest[:]), bad); !errors.Is(err, outboundport.ErrBroadcastJobHistoryInvalid) {
		t.Fatalf("invalid_fact_accepted=%v", err)
	}
}

type broadcastJobHistoryStoreFake struct {
	values        map[int64]outboundport.HistoricalBroadcastJob
	creates, gets int
}

func (store *broadcastJobHistoryStoreFake) CreateHistoricalBroadcastJob(_ context.Context, value outboundport.HistoricalBroadcastJob) (outboundport.HistoricalBroadcastJob, error) {
	store.creates++
	if store.values == nil {
		store.values = map[int64]outboundport.HistoricalBroadcastJob{}
	}
	value.ID = 41
	store.values[value.ID] = value
	return value, nil
}
func (store *broadcastJobHistoryStoreFake) GetHistoricalBroadcastJob(_ context.Context, id int64) (outboundport.HistoricalBroadcastJob, error) {
	store.gets++
	value, found := store.values[id]
	if !found {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryConflict
	}
	return value, nil
}

type broadcastJobHistoryJournalFake struct {
	receipt outboundport.BroadcastJobHistoryReceipt
	found   bool
	records int
}

func (journal *broadcastJobHistoryJournalFake) LoadBroadcastJobHistory(_ context.Context, source string) (outboundport.BroadcastJobHistoryReceipt, bool, error) {
	if journal.found && journal.receipt.SourceIdentifier != source {
		return outboundport.BroadcastJobHistoryReceipt{}, false, outboundport.ErrBroadcastJobHistoryConflict
	}
	return journal.receipt, journal.found, nil
}
func (journal *broadcastJobHistoryJournalFake) RecordBroadcastJobHistory(_ context.Context, receipt outboundport.BroadcastJobHistoryReceipt) error {
	if journal.found {
		return outboundport.ErrBroadcastJobHistoryConflict
	}
	journal.receipt, journal.found, journal.records = receipt, true, journal.records+1
	return nil
}

func broadcastJobHistoryFact() outboundport.HistoricalBroadcastJob {
	at := time.Date(2026, 8, 28, 13, 14, 15, 123456789, time.FixedZone("V1", 8*60*60))
	legacy := int64(-4)
	digest := func(label string) [32]byte { return sha256.Sum256([]byte(label)) }
	return outboundport.HistoricalBroadcastJob{SourceID: 7, OriginalSourceType: "unknown_source", SourceReferenceDigest: digest("source"), SourceTable: "legacy_table", ScheduledFor: at, Priority: -1, BatchKeyDigest: digest("batch"), OriginalStatus: "unknown_status", RequiresApproval: true, ApprovedByDigest: digest("approved"), ApprovedAt: &at, CancelledByDigest: digest("cancelled"), CancelReasonDigest: digest("reason"), TargetCount: -2, TargetSummaryDigest: digest("targets"), ContentType: "unknown_content", ContentPayloadDigest: digest("payload"), ContentSummaryDigest: digest("summary"), AttemptCount: -3, LastErrorDigest: digest("error"), LegacyOutboundTaskID: &legacy, SentCount: -4, FailedCount: -5, TraceIDDigest: digest("trace"), CreatedByDigest: digest("creator"), CreatedAt: at, UpdatedAt: at, ClaimTokenDigest: digest("claim"), RetryPolicyDigest: digest("retry"), MetadataDigest: digest("metadata"), TargetUnionIDsDigest: digest("targets"), MaxAttempts: -6, SideEffectExecuted: true, ProviderResultReceived: true, ResultSummaryDigest: digest("result"), ReconciliationRequired: true, HoldReasonDigest: digest("hold"), ExecutionIDDigest: digest("execution"), ExecutionOwnerDigest: digest("owner"), SourceKeyDigest: digest("key"), SourcePayloadDigest: digest("payload-root"), SourceFieldDigest: digest("fields"), RedactedRoots: []string{"claim_token"}}
}
func stringsUpper(value string) string { return "A" + value[1:] }
