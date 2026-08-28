package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var campaignHistoryPostgresDSN = flag.String("campaign-history-store-postgres-dsn", "", "isolated PostgreSQL DSN for schema 118 rollback verification")

func TestCampaignHistoryStoreFailsClosedAndRejectsInvalidQueries(t *testing.T) {
	ctx := context.Background()
	store := NewCampaignHistoryStore()
	if _, err := store.GetHistoricalCampaignSegment(ctx, 1); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
		t.Fatal("store read escaped caller transaction")
	}
	var pool *pgxpool.Pool
	for _, reader := range []*CampaignHistoryReader{nil, NewCampaignHistoryReader(nil), NewCampaignHistoryReader(pool)} {
		if _, err := reader.GetHistoricalBroadcastPlan(ctx, 1); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
			t.Fatal("nil reader did not fail closed")
		}
	}
	if _, err := store.GetHistoricalBroadcastMessage(ctx, 0); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatal("invalid ID accepted")
	}
	reader := NewCampaignHistoryReader(nil)
	invalid := int64(0)
	if _, _, err := reader.ListHistoricalCampaignSegments(ctx, nil, 0, 0); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatal("segment page accepted")
	}
	if _, _, err := reader.ListHistoricalCampaignSegments(ctx, &invalid, 1, 0); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatal("segment filter accepted")
	}
	if _, _, err := reader.ListHistoricalCampaignMembers(ctx, nil, &invalid, 1, 0); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatal("member filter accepted")
	}
	if _, _, err := reader.ListHistoricalBroadcastPlans(ctx, 101, 0); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatal("plan page accepted")
	}
	if _, _, err := reader.ListHistoricalBroadcastRecipients(ctx, 0, 1, 0); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatal("recipient filter accepted")
	}
	if _, _, err := reader.ListHistoricalBroadcastMessages(ctx, 1, 1, -1); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatal("message page accepted")
	}
}

func TestCampaignHistoryPostgresRoundTripRollback(t *testing.T) {
	if *campaignHistoryPostgresDSN == "" {
		t.Skip("set -campaign-history-store-postgres-dsn for isolated schema 118 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *campaignHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	queries := campaigndb.New(pool)
	before, err := campaignHistoryCounts(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("campaign history forced rollback")
	var ids campaignHistoryIDs
	base := time.Now().UTC().UnixNano()
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		store, reader := NewCampaignHistoryStore(), NewCampaignHistoryReader(tx)
		segment, member, plan, recipient, message := campaignHistoryFixture(base)

		createdSegment, err := store.CreateHistoricalCampaignSegment(txCtx, segment)
		if err != nil {
			return err
		}
		segment.ID = createdSegment.ID
		if !reflect.DeepEqual(segment, createdSegment) {
			return fmt.Errorf("segment create changed historical fields")
		}
		if loaded, getErr := store.GetHistoricalCampaignSegment(txCtx, segment.ID); getErr != nil || !reflect.DeepEqual(loaded, segment) {
			return fmt.Errorf("segment round trip: equal=%t err=%v", reflect.DeepEqual(loaded, segment), getErr)
		}

		member.SegmentHistoryID = segment.ID
		createdMember, err := store.CreateHistoricalCampaignMember(txCtx, member)
		if err != nil {
			return err
		}
		member.ID = createdMember.ID
		if !reflect.DeepEqual(member, createdMember) {
			return fmt.Errorf("member create changed nullable historical fields")
		}
		if loaded, getErr := store.GetHistoricalCampaignMember(txCtx, member.ID); getErr != nil || !reflect.DeepEqual(loaded, member) {
			return fmt.Errorf("member round trip: equal=%t err=%v", reflect.DeepEqual(loaded, member), getErr)
		}

		createdPlan, err := store.CreateHistoricalBroadcastPlan(txCtx, plan)
		if err != nil {
			return err
		}
		plan.ID = createdPlan.ID
		if !reflect.DeepEqual(plan, createdPlan) {
			return fmt.Errorf("plan create changed nullable historical fields")
		}
		if loaded, getErr := store.GetHistoricalBroadcastPlan(txCtx, plan.ID); getErr != nil || !reflect.DeepEqual(loaded, plan) {
			return fmt.Errorf("plan round trip: equal=%t err=%v", reflect.DeepEqual(loaded, plan), getErr)
		}

		recipient.PlanHistoryID = plan.ID
		createdRecipient, err := store.CreateHistoricalBroadcastRecipient(txCtx, recipient)
		if err != nil {
			return err
		}
		recipient.ID = createdRecipient.ID
		if !reflect.DeepEqual(recipient, createdRecipient) {
			return fmt.Errorf("recipient create changed nullable historical fields")
		}
		if loaded, getErr := store.GetHistoricalBroadcastRecipient(txCtx, recipient.ID); getErr != nil || !reflect.DeepEqual(loaded, recipient) {
			return fmt.Errorf("recipient round trip: equal=%t err=%v", reflect.DeepEqual(loaded, recipient), getErr)
		}

		message.PlanHistoryID, message.RecipientHistoryID = plan.ID, recipient.ID
		createdMessage, err := store.CreateHistoricalBroadcastMessage(txCtx, message)
		if err != nil {
			return err
		}
		message.ID = createdMessage.ID
		if !reflect.DeepEqual(message, createdMessage) {
			return fmt.Errorf("message create changed nullable historical fields")
		}
		if loaded, getErr := store.GetHistoricalBroadcastMessage(txCtx, message.ID); getErr != nil || !reflect.DeepEqual(loaded, message) {
			return fmt.Errorf("message round trip: equal=%t err=%v", reflect.DeepEqual(loaded, message), getErr)
		}

		items, total, err := reader.ListHistoricalCampaignSegments(txCtx, &segment.CampaignSourceID, 20, 0)
		if err != nil || total != 1 || len(items) != 1 || items[0].ID != segment.ID {
			return fmt.Errorf("segment list/count: total=%d items=%d err=%v", total, len(items), err)
		}
		itemsMembers, total, err := reader.ListHistoricalCampaignMembers(txCtx, &segment.ID, nil, 20, 0)
		if err != nil || total != 1 || len(itemsMembers) != 1 || itemsMembers[0].ID != member.ID {
			return fmt.Errorf("member list/count: total=%d items=%d err=%v", total, len(itemsMembers), err)
		}
		itemsPlans, total, err := reader.ListHistoricalBroadcastPlans(txCtx, 100, 0)
		if err != nil || total != before.plans+1 || len(itemsPlans) < 1 {
			return fmt.Errorf("plan list/count: total=%d items=%d err=%v", total, len(itemsPlans), err)
		}
		itemsRecipients, total, err := reader.ListHistoricalBroadcastRecipients(txCtx, plan.ID, 20, 0)
		if err != nil || total != 1 || len(itemsRecipients) != 1 || itemsRecipients[0].ID != recipient.ID {
			return fmt.Errorf("recipient list/count: total=%d items=%d err=%v", total, len(itemsRecipients), err)
		}
		itemsMessages, total, err := reader.ListHistoricalBroadcastMessages(txCtx, recipient.ID, 20, 0)
		if err != nil || total != 1 || len(itemsMessages) != 1 || itemsMessages[0].ID != message.ID {
			return fmt.Errorf("message list/count: total=%d items=%d err=%v", total, len(itemsMessages), err)
		}
		ids = campaignHistoryIDs{segment: segment.ID, member: member.ID, plan: plan.ID, recipient: recipient.ID, message: message.ID}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := campaignHistoryCounts(ctx, queries)
	if err != nil || after != before {
		t.Fatalf("forced rollback changed history: before=%+v after=%+v err=%v", before, after, err)
	}
	for _, item := range []int64{ids.segment, ids.member, ids.plan, ids.recipient, ids.message} {
		if item < 1 {
			t.Fatal("rollback test did not create all five entities")
		}
	}
	if count, countErr := queries.CountHistoricalBroadcastRecipients(ctx, ids.plan); countErr != nil || count != 0 {
		t.Fatalf("rolled-back recipient count = %d, err=%v", count, countErr)
	}
	if count, countErr := queries.CountHistoricalBroadcastMessages(ctx, ids.recipient); countErr != nil || count != 0 {
		t.Fatalf("rolled-back message count = %d, err=%v", count, countErr)
	}
	if _, err := queries.GetHistoricalCampaignSegment(ctx, ids.segment); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rolled-back segment remained")
	}
	if _, err := queries.GetHistoricalCampaignMember(ctx, ids.member); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rolled-back member remained")
	}
	if _, err := queries.GetHistoricalBroadcastPlan(ctx, ids.plan); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rolled-back plan remained")
	}
	if _, err := queries.GetHistoricalBroadcastRecipient(ctx, ids.recipient); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rolled-back recipient remained")
	}
	if _, err := queries.GetHistoricalBroadcastMessage(ctx, ids.message); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rolled-back message remained")
	}
}

type campaignHistoryCount struct{ segments, members, plans int64 }
type campaignHistoryIDs struct{ segment, member, plan, recipient, message int64 }

func campaignHistoryCounts(ctx context.Context, queries *campaigndb.Queries) (campaignHistoryCount, error) {
	var result campaignHistoryCount
	var err error
	if result.segments, err = queries.CountHistoricalCampaignSegments(ctx, campaignHistoryInt(nil)); err != nil {
		return result, err
	}
	if result.members, err = queries.CountHistoricalCampaignMembers(ctx, campaigndb.CountHistoricalCampaignMembersParams{}); err != nil {
		return result, err
	}
	if result.plans, err = queries.CountHistoricalBroadcastPlans(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func campaignHistoryFixture(base int64) (campaignport.HistoricalCampaignSegment, campaignport.HistoricalCampaignMember, campaignport.HistoricalBroadcastPlan, campaignport.HistoricalBroadcastRecipient, campaignport.HistoricalBroadcastMessage) {
	created := time.Date(2025, 2, 3, 4, 5, 6, 123456000, time.UTC)
	updated := created.Add(time.Minute)
	campaignSourceID := base + 20
	segmentSourceID := base + 30
	segment := campaignport.HistoricalCampaignSegment{SourceID: base + 1, CampaignSourceID: campaignSourceID, SegmentSourceID: segmentSourceID,
		SourceParentState: "missing_campaign", Code: "campaign-history-segment", Priority: -1, Label: "", CreatedAt: created, SourcePayloadDigest: [32]byte{1}}
	member := campaignport.HistoricalCampaignMember{SourceID: base + 2, CampaignSourceID: campaignSourceID, CampaignSegmentSourceID: base + 21,
		SegmentSourceID: segmentSourceID, MemberSourceID: base + 22, JoinedAt: created, AnchorDate: "2025-02-03", CurrentStepIndex: -3,
		NextDueAt: &updated, OriginalStatus: "", StopReason: "", RetryCount: -2, CreatedAt: created, UpdatedAt: updated, SourcePayloadDigest: [32]byte{2}}
	plan := campaignport.HistoricalBroadcastPlan{SourceID: base + 3, SourcePlanID: fmt.Sprintf("uat-campaign-history-%d", base), SegmentSourceID: &segmentSourceID,
		DisplayName: "", Intent: "", ContentStrategy: "", ContentTemplateMasked: " ", MaxRecipients: -1, CandidateCount: -2, SkippedCount: -3,
		OriginalStatus: "", OriginalReviewStatus: "", OriginalRunStatus: "", ExpiresAt: &updated, CreatedAt: created, UpdatedAt: updated, RuntimeDigest: [32]byte{3}, SourcePayloadDigest: [32]byte{4}}
	recipient := campaignport.HistoricalBroadcastRecipient{SourceID: base + 4, DisplayName: "", PlannedMessageCount: -1, OriginalApprovalStatus: "",
		OriginalSendStatus: "", RejectedAt: &updated, CreatedAt: created, UpdatedAt: updated, SourcePayloadDigest: [32]byte{5}}
	message := campaignport.HistoricalBroadcastMessage{SourceID: base + 5, SequenceIndex: -1, DayOffset: -2, OriginalSendTime: "civil 2025-02-03 12:05",
		ContentMasked: " \nmasked [redacted]\t", OriginalStatus: "", SentAt: &updated, CreatedAt: created, UpdatedAt: updated,
		ContentPayloadDigest: [32]byte{6}, AttachmentsDigest: [32]byte{7}, SourcePayloadDigest: [32]byte{8}}
	return segment, member, plan, recipient, message
}
