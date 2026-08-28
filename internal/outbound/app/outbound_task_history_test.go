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

func TestOutboundTaskHistoryWriterCreatesReplayWithReciprocalParent(t *testing.T) {
	value := outboundTaskHistoryFact(-7, ptr(-8))
	store := &outboundTaskHistoryStoreFake{parents: map[int64][]outboundport.OutboundTaskHistoryParent{
		-8: {{ID: 11, SourceID: -8, LegacyOutboundTaskID: ptr(-7)}},
	}}
	journal := &outboundTaskHistoryJournalFake{}
	writer := NewOutboundTaskHistoryWriter(store, journal)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	first, err := writer.Import(context.Background(), source, value)
	if err != nil || first.Replayed || first.TargetID != 41 || first.TargetDigest == ([32]byte{}) || store.creates != 1 || journal.records != 1 {
		t.Fatalf("first_import receipt=%#v err=%v", first, err)
	}
	stored := store.values[first.TargetID]
	if stored.BroadcastJobHistoryID == nil || *stored.BroadcastJobHistoryID != 11 || stored.CreatedAt.Location() != time.UTC || stored.CreatedAt.Nanosecond()%1000 != 0 {
		t.Fatalf("reciprocal_parent_or_time_lost=%#v", stored)
	}
	second, err := writer.Import(context.Background(), source, value)
	if err != nil || !second.Replayed || second.TargetDigest != first.TargetDigest || store.creates != 1 || store.gets != 1 || store.lookups != 2 {
		t.Fatalf("replay receipt=%#v err=%v", second, err)
	}
}

func TestOutboundTaskHistoryWriterLeavesUnprovenParentsNil(t *testing.T) {
	for name, parents := range map[string][]outboundport.OutboundTaskHistoryParent{
		"none":         nil,
		"multiple":     {{ID: 11, SourceID: -8, LegacyOutboundTaskID: ptr(-7)}, {ID: 12, SourceID: -8, LegacyOutboundTaskID: ptr(-7)}},
		"wrong_id":     {{ID: 0, SourceID: -8, LegacyOutboundTaskID: ptr(-7)}},
		"not_mutual":   {{ID: 11, SourceID: -8, LegacyOutboundTaskID: ptr(-6)}},
		"wrong_source": {{ID: 11, SourceID: -9, LegacyOutboundTaskID: ptr(-7)}},
	} {
		t.Run(name, func(t *testing.T) {
			store := &outboundTaskHistoryStoreFake{parents: map[int64][]outboundport.OutboundTaskHistoryParent{-8: parents}}
			value := outboundTaskHistoryFact(-7, ptr(-8))
			if _, err := NewOutboundTaskHistoryWriter(store, &outboundTaskHistoryJournalFake{}).Import(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); err != nil {
				t.Fatalf("unproven_parent_rejected=%v", err)
			}
			if got := store.values[41].BroadcastJobHistoryID; got != nil {
				t.Fatalf("unproven_parent_injected=%d", *got)
			}
		})
	}
	value := outboundTaskHistoryFact(0, nil)
	store := &outboundTaskHistoryStoreFake{}
	if _, err := NewOutboundTaskHistoryWriter(store, &outboundTaskHistoryJournalFake{}).Import(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); err != nil || store.lookups != 0 || store.values[41].BroadcastJobHistoryID != nil {
		t.Fatalf("nil_parent_or_signed_source_changed err=%v store=%#v", err, store)
	}
	value = outboundTaskHistoryFact(0, ptr(0))
	store = &outboundTaskHistoryStoreFake{parents: map[int64][]outboundport.OutboundTaskHistoryParent{
		0: {{ID: 13, SourceID: 0, LegacyOutboundTaskID: ptr(0)}},
	}}
	if _, err := NewOutboundTaskHistoryWriter(store, &outboundTaskHistoryJournalFake{}).Import(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); err != nil || store.values[41].BroadcastJobHistoryID == nil || *store.values[41].BroadcastJobHistoryID != 13 {
		t.Fatalf("zero_signed_source_or_legacy_changed err=%v store=%#v", err, store)
	}
}

func TestOutboundTaskHistoryWriterRejectsInjectedParentInvalidSourceAndNilDependencies(t *testing.T) {
	value := outboundTaskHistoryFact(-7, ptr(-8))
	value.BroadcastJobHistoryID = ptr(11)
	store := &outboundTaskHistoryStoreFake{}
	writer := NewOutboundTaskHistoryWriter(store, &outboundTaskHistoryJournalFake{})
	if _, err := writer.Import(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, outboundport.ErrOutboundTaskHistoryInvalid) || store.lookups != 0 {
		t.Fatalf("injected_parent_accepted=%v", err)
	}
	value = outboundTaskHistoryFact(-7, nil)
	if _, err := writer.Import(context.Background(), "wrong", value); !errors.Is(err, outboundport.ErrOutboundTaskHistoryInvalid) {
		t.Fatalf("source_binding_accepted=%v", err)
	}
	value.SourceFieldDigest = [32]byte{}
	if _, err := writer.Import(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, outboundport.ErrOutboundTaskHistoryInvalid) {
		t.Fatalf("empty_envelope_accepted=%v", err)
	}
	valid := outboundTaskHistoryFact(-7, nil)
	source := hex.EncodeToString(valid.SourceKeyDigest[:])
	var nilStore *outboundTaskHistoryStoreFake
	if _, err := NewOutboundTaskHistoryWriter(nilStore, &outboundTaskHistoryJournalFake{}).Import(context.Background(), source, valid); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatalf("typed_nil_store=%v", err)
	}
	var nilJournal *outboundTaskHistoryJournalFake
	if _, err := NewOutboundTaskHistoryWriter(&outboundTaskHistoryStoreFake{}, nilJournal).Import(context.Background(), source, valid); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatalf("typed_nil_journal=%v", err)
	}
}

func TestHistoricalOutboundTaskDigestBindsPrivateNullableParentAndID(t *testing.T) {
	value := outboundTaskHistoryFact(-7, ptr(-8))
	value.BroadcastJobHistoryID = ptr(11)
	value.ID = 9
	baseline, err := HistoricalOutboundTaskDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*outboundport.HistoricalOutboundTask){
		func(v *outboundport.HistoricalOutboundTask) { v.SourceID++ },
		func(v *outboundport.HistoricalOutboundTask) { v.TaskType = "changed" },
		func(v *outboundport.HistoricalOutboundTask) { v.Status = "changed" },
		func(v *outboundport.HistoricalOutboundTask) { v.CreatedAt = v.CreatedAt.Add(time.Microsecond) },
		func(v *outboundport.HistoricalOutboundTask) { v.RequestPayloadDigest[0]++ },
		func(v *outboundport.HistoricalOutboundTask) { v.ResponsePayloadDigest[0]++ },
		func(v *outboundport.HistoricalOutboundTask) { v.WeComTaskIDDigest = nil },
		func(v *outboundport.HistoricalOutboundTask) { v.TraceIDDigest[0]++ },
		func(v *outboundport.HistoricalOutboundTask) { v.LegacyBroadcastJobID = nil },
		func(v *outboundport.HistoricalOutboundTask) { v.BroadcastJobHistoryID = nil },
		func(v *outboundport.HistoricalOutboundTask) { v.SourceKeyDigest[0]++ },
		func(v *outboundport.HistoricalOutboundTask) { v.SourcePayloadDigest[0]++ },
		func(v *outboundport.HistoricalOutboundTask) { v.SourceFieldDigest[0]++ },
		func(v *outboundport.HistoricalOutboundTask) { v.RedactedRoots = []string{"other"} },
		func(v *outboundport.HistoricalOutboundTask) { v.ID++ },
	} {
		changed := value
		mutate(&changed)
		if got, err := HistoricalOutboundTaskDigest(changed); err != nil || got == baseline {
			t.Fatalf("field_omitted digest=%x err=%v", got, err)
		}
	}
	value.WeComTaskIDDigest = nil
	if _, err := HistoricalOutboundTaskDigest(value); err != nil {
		t.Fatalf("nullable_wecom_rejected=%v", err)
	}
}

func TestOutboundTaskHistoryWriterRejectsReplayMutationAndReceiptDrift(t *testing.T) {
	value := outboundTaskHistoryFact(-7, nil)
	store := &outboundTaskHistoryStoreFake{}
	journal := &outboundTaskHistoryJournalFake{}
	writer := NewOutboundTaskHistoryWriter(store, journal)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	if _, err := writer.Import(context.Background(), source, value); err != nil {
		t.Fatal(err)
	}
	stored := store.values[41]
	stored.RequestPayloadDigest[0]++
	store.values[41] = stored
	if _, err := writer.Import(context.Background(), source, value); !errors.Is(err, outboundport.ErrOutboundTaskHistoryConflict) {
		t.Fatalf("private_target_mutation_accepted=%v", err)
	}
	store.values[41] = withHistoricalOutboundTaskID(value, 41)
	journal.receipt.PayloadDigest[0]++
	if _, err := writer.Import(context.Background(), source, value); !errors.Is(err, outboundport.ErrOutboundTaskHistoryConflict) {
		t.Fatalf("receipt_payload_drift_accepted=%v", err)
	}
	store.lookupErr = errors.New("lookup unavailable")
	value.LegacyBroadcastJobID = ptr(-8)
	if _, err := writer.Import(context.Background(), source, value); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatalf("lookup_error_not_closed=%v", err)
	}
}

type outboundTaskHistoryStoreFake struct {
	values                       map[int64]outboundport.HistoricalOutboundTask
	parents                      map[int64][]outboundport.OutboundTaskHistoryParent
	creates, gets, lookups       int
	createErr, getErr, lookupErr error
}

func (store *outboundTaskHistoryStoreFake) CreateHistoricalOutboundTask(_ context.Context, value outboundport.HistoricalOutboundTask) (outboundport.HistoricalOutboundTask, error) {
	if store.createErr != nil {
		return outboundport.HistoricalOutboundTask{}, store.createErr
	}
	if store.values == nil {
		store.values = map[int64]outboundport.HistoricalOutboundTask{}
	}
	store.creates++
	value.ID = 41
	store.values[value.ID] = value
	return value, nil
}
func (store *outboundTaskHistoryStoreFake) GetHistoricalOutboundTask(_ context.Context, id int64) (outboundport.HistoricalOutboundTask, error) {
	if store.getErr != nil {
		return outboundport.HistoricalOutboundTask{}, store.getErr
	}
	store.gets++
	value, found := store.values[id]
	if !found {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	return value, nil
}
func (store *outboundTaskHistoryStoreFake) LookupOutboundTaskHistoryParents(_ context.Context, sourceID int64) ([]outboundport.OutboundTaskHistoryParent, error) {
	if store.lookupErr != nil {
		return nil, store.lookupErr
	}
	store.lookups++
	return append([]outboundport.OutboundTaskHistoryParent(nil), store.parents[sourceID]...), nil
}

type outboundTaskHistoryJournalFake struct {
	receipt            outboundport.OutboundTaskHistoryReceipt
	found              bool
	records            int
	loadErr, recordErr error
}

func (journal *outboundTaskHistoryJournalFake) LoadOutboundTaskHistory(_ context.Context, source string) (outboundport.OutboundTaskHistoryReceipt, bool, error) {
	if journal.loadErr != nil {
		return outboundport.OutboundTaskHistoryReceipt{}, false, journal.loadErr
	}
	if journal.found && journal.receipt.SourceIdentifier != source {
		return outboundport.OutboundTaskHistoryReceipt{}, false, outboundport.ErrOutboundTaskHistoryConflict
	}
	return journal.receipt, journal.found, nil
}
func (journal *outboundTaskHistoryJournalFake) RecordOutboundTaskHistory(_ context.Context, receipt outboundport.OutboundTaskHistoryReceipt) error {
	if journal.recordErr != nil {
		return journal.recordErr
	}
	if journal.found {
		return outboundport.ErrOutboundTaskHistoryConflict
	}
	journal.receipt, journal.found, journal.records = receipt, true, journal.records+1
	return nil
}

func outboundTaskHistoryFact(sourceID int64, legacy *int64) outboundport.HistoricalOutboundTask {
	at := time.Date(2026, 8, 28, 13, 14, 15, 123456789, time.FixedZone("V1", 8*60*60))
	digest := func(label string) [32]byte { return sha256.Sum256([]byte(label)) }
	wecom := digest("wecom")
	return outboundport.HistoricalOutboundTask{
		SourceID: sourceID, TaskType: "", Status: "", CreatedAt: at, RequestPayloadDigest: digest("request"), ResponsePayloadDigest: digest("response"),
		WeComTaskIDDigest: &wecom, TraceIDDigest: digest("trace"), LegacyBroadcastJobID: legacy,
		SourceKeyDigest: digest("key"), SourcePayloadDigest: digest("payload"), SourceFieldDigest: digest("field"), RedactedRoots: []string{"trace_id"},
	}
}

func ptr(value int64) *int64 { return &value }

var _ outboundport.OutboundTaskHistoryStore = (*outboundTaskHistoryStoreFake)(nil)
var _ outboundport.OutboundTaskHistoryJournal = (*outboundTaskHistoryJournalFake)(nil)
