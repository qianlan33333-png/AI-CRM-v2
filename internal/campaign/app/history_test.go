package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

func TestCampaignHistoryWriterCreatesAndReplaysFiveFacts(t *testing.T) {
	store := &campaignHistoryStoreFake{}
	journal := &campaignHistoryJournalFake{receipts: map[string]campaignport.CampaignHistoryReceipt{}}
	writer := NewCampaignHistoryWriter(store, journal)
	segment, member, plan, recipient, message := campaignHistoryFixtures()

	checks := []struct {
		kind  string
		write func(string, [32]byte) (campaignport.CampaignHistoryReceipt, error)
	}{
		{"segment", func(source string, payload [32]byte) (campaignport.CampaignHistoryReceipt, error) {
			return writer.WriteSegment(context.Background(), source, payload, segment)
		}},
		{"member", func(source string, payload [32]byte) (campaignport.CampaignHistoryReceipt, error) {
			return writer.WriteMember(context.Background(), source, payload, member)
		}},
		{"plan", func(source string, payload [32]byte) (campaignport.CampaignHistoryReceipt, error) {
			return writer.WritePlan(context.Background(), source, payload, plan)
		}},
		{"recipient", func(source string, payload [32]byte) (campaignport.CampaignHistoryReceipt, error) {
			return writer.WriteRecipient(context.Background(), source, payload, recipient)
		}},
		{"message", func(source string, payload [32]byte) (campaignport.CampaignHistoryReceipt, error) {
			return writer.WriteMessage(context.Background(), source, payload, message)
		}},
	}
	for _, check := range checks {
		t.Run(check.kind, func(t *testing.T) {
			source, payload := campaignHistorySource(check.kind), campaignHistoryPayload(check.kind)
			first, err := check.write(source, payload)
			if err != nil || first.Replayed || first.TargetID < 1 || first.TargetDigest == ([32]byte{}) {
				t.Fatalf("first=%#v err=%v", first, err)
			}
			replay, err := check.write(source, payload)
			if err != nil || !replay.Replayed || replay != (campaignport.CampaignHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: first.TargetID, TargetDigest: first.TargetDigest, Replayed: true}) {
				t.Fatalf("replay=%#v err=%v", replay, err)
			}
		})
	}
	if store.segmentCreates != 1 || store.memberCreates != 1 || store.planCreates != 1 || store.recipientCreates != 1 || store.messageCreates != 1 {
		t.Fatalf("unexpected writes: %#v", store)
	}
	if store.segment.CreatedAt.Location() != time.UTC || store.message.CreatedAt.Nanosecond()%1000 != 0 || store.member.CustomerID == nil || *store.member.CustomerID != 7 {
		t.Fatalf("target was not normalized: segment=%#v message=%#v member=%#v", store.segment, store.message, store.member)
	}
}

func TestCampaignHistoryWriterRejectsReplayTargetDrift(t *testing.T) {
	store := &campaignHistoryStoreFake{}
	journal := &campaignHistoryJournalFake{receipts: map[string]campaignport.CampaignHistoryReceipt{}}
	writer := NewCampaignHistoryWriter(store, journal)
	_, _, _, _, message := campaignHistoryFixtures()
	source, payload := campaignHistorySource("message"), campaignHistoryPayload("message")
	receipt, err := writer.WriteMessage(context.Background(), source, payload, message)
	if err != nil {
		t.Fatal(err)
	}
	store.message.ContentMasked = "changed"
	if _, err = writer.WriteMessage(context.Background(), source, payload, message); !errors.Is(err, campaignport.ErrCampaignHistoryConflict) {
		t.Fatalf("target drift err=%v", err)
	}
	if store.messageCreates != 1 || store.messageGets != 1 {
		t.Fatalf("replay created or skipped target read: %#v", store)
	}
	_ = receipt
}

func TestCampaignHistoryWriterRejectsUnsafeInputAndMissingCallerDependencies(t *testing.T) {
	segment, member, plan, recipient, message := campaignHistoryFixtures()
	source, payload := campaignHistorySource("unsafe"), campaignHistoryPayload("unsafe")
	writer := NewCampaignHistoryWriter(&campaignHistoryStoreFake{}, &campaignHistoryJournalFake{})

	segment.SourceParentState = "active"
	if _, err := writer.WriteSegment(context.Background(), source, payload, segment); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("bad segment state err=%v", err)
	}
	member.SegmentHistoryID = 0
	if _, err := writer.WriteMember(context.Background(), source, payload, member); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("missing member parent err=%v", err)
	}
	plan.SourcePlanID = ""
	if _, err := writer.WritePlan(context.Background(), source, payload, plan); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("empty plan source ID err=%v", err)
	}
	recipient.CustomerID = campaignHistoryPointer(0)
	if _, err := writer.WriteRecipient(context.Background(), source, payload, recipient); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("invalid customer err=%v", err)
	}
	message.ContentMasked = "bad\x00"
	if _, err := writer.WriteMessage(context.Background(), source, payload, message); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("NUL text err=%v", err)
	}

	_, _, plan, _, _ = campaignHistoryFixtures()
	if _, err := writer.WritePlan(context.Background(), "not-a-source-hmac", payload, plan); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("invalid source identifier err=%v", err)
	}
	if _, err := writer.WritePlan(context.Background(), source, sha256.Sum256([]byte("different")), plan); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("payload mismatch err=%v", err)
	}
	var nilStore *campaignHistoryStoreFake
	if _, err := NewCampaignHistoryWriter(nilStore, &campaignHistoryJournalFake{}).WritePlan(context.Background(), source, payload, plan); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
		t.Fatalf("typed nil store err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writer.WritePlan(ctx, source, payload, plan); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
		t.Fatalf("cancelled context err=%v", err)
	}
}

func TestHistoricalBroadcastMessageDigestPreservesCivilTextAndNormalizesTimes(t *testing.T) {
	_, _, _, _, message := campaignHistoryFixtures()
	message.ID = 91
	message.OriginalSendTime = "historical civil value; never parse this"
	before, err := HistoricalBroadcastMessageDigest(message)
	if err != nil {
		t.Fatalf("digest rejected opaque civil text: %v", err)
	}
	message.CreatedAt = message.CreatedAt.UTC().Truncate(time.Microsecond)
	message.UpdatedAt = message.UpdatedAt.UTC().Truncate(time.Microsecond)
	sent := message.SentAt.UTC().Truncate(time.Microsecond)
	message.SentAt = &sent
	after, err := HistoricalBroadcastMessageDigest(message)
	if err != nil || before != after {
		t.Fatalf("time normalization missing: before=%x after=%x err=%v", before, after, err)
	}
	message.ID++
	changed, err := HistoricalBroadcastMessageDigest(message)
	if err != nil || changed == after {
		t.Fatalf("target ID omitted from digest: %v", err)
	}
}

type campaignHistoryStoreFake struct {
	segment   campaignport.HistoricalCampaignSegment
	member    campaignport.HistoricalCampaignMember
	plan      campaignport.HistoricalBroadcastPlan
	recipient campaignport.HistoricalBroadcastRecipient
	message   campaignport.HistoricalBroadcastMessage

	segmentCreates, segmentGets     int
	memberCreates, memberGets       int
	planCreates, planGets           int
	recipientCreates, recipientGets int
	messageCreates, messageGets     int
}

func (store *campaignHistoryStoreFake) CreateHistoricalCampaignSegment(_ context.Context, value campaignport.HistoricalCampaignSegment) (campaignport.HistoricalCampaignSegment, error) {
	store.segmentCreates++
	value.ID = 11
	store.segment = value
	return value, nil
}
func (store *campaignHistoryStoreFake) GetHistoricalCampaignSegment(_ context.Context, id int64) (campaignport.HistoricalCampaignSegment, error) {
	store.segmentGets++
	if id != store.segment.ID {
		return campaignport.HistoricalCampaignSegment{}, campaignport.ErrCampaignHistoryConflict
	}
	return store.segment, nil
}
func (store *campaignHistoryStoreFake) CreateHistoricalCampaignMember(_ context.Context, value campaignport.HistoricalCampaignMember) (campaignport.HistoricalCampaignMember, error) {
	store.memberCreates++
	value.ID = 21
	store.member = value
	return value, nil
}
func (store *campaignHistoryStoreFake) GetHistoricalCampaignMember(_ context.Context, id int64) (campaignport.HistoricalCampaignMember, error) {
	store.memberGets++
	if id != store.member.ID {
		return campaignport.HistoricalCampaignMember{}, campaignport.ErrCampaignHistoryConflict
	}
	return store.member, nil
}
func (store *campaignHistoryStoreFake) CreateHistoricalBroadcastPlan(_ context.Context, value campaignport.HistoricalBroadcastPlan) (campaignport.HistoricalBroadcastPlan, error) {
	store.planCreates++
	value.ID = 31
	store.plan = value
	return value, nil
}
func (store *campaignHistoryStoreFake) GetHistoricalBroadcastPlan(_ context.Context, id int64) (campaignport.HistoricalBroadcastPlan, error) {
	store.planGets++
	if id != store.plan.ID {
		return campaignport.HistoricalBroadcastPlan{}, campaignport.ErrCampaignHistoryConflict
	}
	return store.plan, nil
}
func (store *campaignHistoryStoreFake) CreateHistoricalBroadcastRecipient(_ context.Context, value campaignport.HistoricalBroadcastRecipient) (campaignport.HistoricalBroadcastRecipient, error) {
	store.recipientCreates++
	value.ID = 41
	store.recipient = value
	return value, nil
}
func (store *campaignHistoryStoreFake) GetHistoricalBroadcastRecipient(_ context.Context, id int64) (campaignport.HistoricalBroadcastRecipient, error) {
	store.recipientGets++
	if id != store.recipient.ID {
		return campaignport.HistoricalBroadcastRecipient{}, campaignport.ErrCampaignHistoryConflict
	}
	return store.recipient, nil
}
func (store *campaignHistoryStoreFake) CreateHistoricalBroadcastMessage(_ context.Context, value campaignport.HistoricalBroadcastMessage) (campaignport.HistoricalBroadcastMessage, error) {
	store.messageCreates++
	value.ID = 51
	store.message = value
	return value, nil
}
func (store *campaignHistoryStoreFake) GetHistoricalBroadcastMessage(_ context.Context, id int64) (campaignport.HistoricalBroadcastMessage, error) {
	store.messageGets++
	if id != store.message.ID {
		return campaignport.HistoricalBroadcastMessage{}, campaignport.ErrCampaignHistoryConflict
	}
	return store.message, nil
}

type campaignHistoryJournalFake struct {
	receipts map[string]campaignport.CampaignHistoryReceipt
}

func (journal *campaignHistoryJournalFake) LoadCampaignHistory(_ context.Context, kind, source string) (campaignport.CampaignHistoryReceipt, bool, error) {
	receipt, found := journal.receipts[kind+"/"+source]
	return receipt, found, nil
}
func (journal *campaignHistoryJournalFake) RecordCampaignHistory(_ context.Context, kind string, receipt campaignport.CampaignHistoryReceipt) error {
	if journal.receipts == nil {
		journal.receipts = map[string]campaignport.CampaignHistoryReceipt{}
	}
	key := kind + "/" + receipt.SourceIdentifier
	if _, found := journal.receipts[key]; found {
		return campaignport.ErrCampaignHistoryConflict
	}
	journal.receipts[key] = receipt
	return nil
}

func campaignHistoryFixtures() (campaignport.HistoricalCampaignSegment, campaignport.HistoricalCampaignMember, campaignport.HistoricalBroadcastPlan, campaignport.HistoricalBroadcastRecipient, campaignport.HistoricalBroadcastMessage) {
	base := time.Date(2026, 8, 28, 9, 10, 11, 123456789, time.FixedZone("CST", 8*3600))
	customer := int64(7)
	next, last, committed, expires, approved, rejected, sent := base.Add(time.Minute), base.Add(2*time.Minute), base.Add(3*time.Minute), base.Add(4*time.Minute), base.Add(5*time.Minute), base.Add(6*time.Minute), base.Add(7*time.Minute)
	segmentPayload, memberPayload, planPayload, recipientPayload, messagePayload := campaignHistoryPayload("segment"), campaignHistoryPayload("member"), campaignHistoryPayload("plan"), campaignHistoryPayload("recipient"), campaignHistoryPayload("message")
	return campaignport.HistoricalCampaignSegment{SourceID: 101, CampaignSourceID: 0 + 1001, SegmentSourceID: -9, SourceParentState: "missing_campaign", Code: " raw-code ", Priority: -4, Label: " label\n", CreatedAt: base, SourcePayloadDigest: segmentPayload},
		campaignport.HistoricalCampaignMember{SourceID: 102, CampaignSourceID: -1, CampaignSegmentSourceID: -2, SegmentSourceID: -3, MemberSourceID: -4, SegmentHistoryID: 11, CustomerID: &customer, JoinedAt: base, AnchorDate: "", CurrentStepIndex: -5, NextDueAt: &next, OriginalStatus: "", StopReason: " raw ", LastStepSentAt: &last, RetryCount: -6, CreatedAt: base, UpdatedAt: base.Add(-time.Minute), SourcePayloadDigest: memberPayload},
		campaignport.HistoricalBroadcastPlan{SourceID: 103, SourcePlanID: " raw-plan ", CampaignSourceID: campaignHistoryPointer(-1), SegmentSourceID: campaignHistoryPointer(-2), DisplayName: "", Intent: "", ContentStrategy: "raw", ContentTemplateMasked: "text\n", MaxRecipients: -1, CandidateCount: -2, SkippedCount: -3, RequiresManualCopy: true, OriginalStatus: "", OriginalReviewStatus: "", OriginalRunStatus: "", CommittedAt: &committed, ExpiresAt: &expires, CreatedAt: base, UpdatedAt: base.Add(-time.Minute), RuntimeDigest: [32]byte{3}, SourcePayloadDigest: planPayload},
		campaignport.HistoricalBroadcastRecipient{SourceID: 104, PlanHistoryID: 31, CustomerID: &customer, DisplayName: "", PlannedMessageCount: -1, OriginalApprovalStatus: "", OriginalSendStatus: "", ApprovedAt: &approved, RejectedAt: &rejected, CreatedAt: base, UpdatedAt: base.Add(-time.Minute), SourcePayloadDigest: recipientPayload},
		campaignport.HistoricalBroadcastMessage{SourceID: 105, PlanHistoryID: 31, RecipientHistoryID: 41, CustomerID: &customer, SequenceIndex: -1, DayOffset: -2, OriginalSendTime: "old civil 09:00", ContentMasked: "content\n", OriginalStatus: "", SentAt: &sent, CreatedAt: base, UpdatedAt: base.Add(-time.Minute), ContentPayloadDigest: [32]byte{4}, AttachmentsDigest: [32]byte{5}, SourcePayloadDigest: messagePayload}
}

func campaignHistorySource(kind string) string {
	digest := sha256.Sum256([]byte("source/" + kind))
	return hex.EncodeToString(digest[:])
}

func campaignHistoryPayload(kind string) [sha256.Size]byte {
	return sha256.Sum256([]byte("payload/" + kind))
}

func campaignHistoryPointer(value int64) *int64 { return &value }
