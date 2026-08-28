package v1domain

import (
	"context"
	"strconv"
	"testing"
	"time"

	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

func TestVerifyCampaignHistoryRowChecksEveryTypedTargetAndDigest(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.UTC)
	payload := func(first byte) (value [32]byte) { value[0] = first; return value }
	segment := campaignport.HistoricalCampaignSegment{ID: 1, SourceID: 11, CampaignSourceID: 12, SegmentSourceID: 13, SourceParentState: "observed", Code: "code", Priority: -1, Label: "label", CreatedAt: stamp, SourcePayloadDigest: payload(1)}
	member := campaignport.HistoricalCampaignMember{ID: 2, SourceID: 21, CampaignSourceID: 22, CampaignSegmentSourceID: 23, SegmentSourceID: 24, MemberSourceID: 25, SegmentHistoryID: 1, JoinedAt: stamp, AnchorDate: "", CurrentStepIndex: -1, OriginalStatus: "old", StopReason: "", RetryCount: -2, CreatedAt: stamp, UpdatedAt: stamp, SourcePayloadDigest: payload(2)}
	plan := campaignport.HistoricalBroadcastPlan{ID: 3, SourceID: 31, SourcePlanID: "plan", DisplayName: "plan", Intent: "", ContentStrategy: "", ContentTemplateMasked: "", MaxRecipients: -1, CandidateCount: -2, SkippedCount: -3, OriginalStatus: "old", OriginalReviewStatus: "old", OriginalRunStatus: "old", CreatedAt: stamp, UpdatedAt: stamp, RuntimeDigest: payload(3), SourcePayloadDigest: payload(3)}
	recipient := campaignport.HistoricalBroadcastRecipient{ID: 4, SourceID: 41, PlanHistoryID: 3, DisplayName: "", PlannedMessageCount: -1, OriginalApprovalStatus: "old", OriginalSendStatus: "old", CreatedAt: stamp, UpdatedAt: stamp, SourcePayloadDigest: payload(4)}
	message := campaignport.HistoricalBroadcastMessage{ID: 5, SourceID: 51, PlanHistoryID: 3, RecipientHistoryID: 4, SequenceIndex: -1, DayOffset: -2, OriginalSendTime: "old civil", ContentMasked: "masked", OriginalStatus: "old", CreatedAt: stamp, UpdatedAt: stamp, ContentPayloadDigest: payload(5), AttachmentsDigest: payload(6), SourcePayloadDigest: payload(5)}
	reader := &campaignHistoryReaderFake{segment: segment, member: member, plan: plan, recipient: recipient, message: message}
	segmentDigest, err := campaignapp.HistoricalCampaignSegmentDigest(segment)
	if err != nil {
		t.Fatal(err)
	}
	memberDigest, err := campaignapp.HistoricalCampaignMemberDigest(member)
	if err != nil {
		t.Fatal(err)
	}
	planDigest, err := campaignapp.HistoricalBroadcastPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	recipientDigest, err := campaignapp.HistoricalBroadcastRecipientDigest(recipient)
	if err != nil {
		t.Fatal(err)
	}
	messageDigest, err := campaignapp.HistoricalBroadcastMessageDigest(message)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		table, target string
		id            int64
		payload       [32]byte
		digest        [32]byte
	}{
		{campaignHistorySegmentsTable, "campaign_v1_history_segments", segment.ID, segment.SourcePayloadDigest, segmentDigest},
		{campaignHistoryMembersTable, "campaign_v1_history_members", member.ID, member.SourcePayloadDigest, memberDigest},
		{campaignHistoryPlansTable, "campaign_v1_history_broadcast_plans", plan.ID, plan.SourcePayloadDigest, planDigest},
		{campaignHistoryRecipientsTable, "campaign_v1_history_broadcast_recipients", recipient.ID, recipient.SourcePayloadDigest, recipientDigest},
		{campaignHistoryMessagesTable, "campaign_v1_history_broadcast_messages", message.ID, message.SourcePayloadDigest, messageDigest},
	}
	for _, item := range cases {
		item := item
		t.Run(item.table, func(t *testing.T) {
			domain, target, id := campaignHistoryTargetDomain, item.target, strconv.FormatInt(item.id, 10)
			row := reconciliationRow{TableID: item.table, PayloadDigest: item.payload[:], TargetDigest: item.digest[:], TargetDomain: &domain, TargetTable: &target, TargetID: &id}
			if _, err := verifyCampaignHistoryRow(context.Background(), reader, row); err != nil {
				t.Fatal(err)
			}
			row.TargetDigest = append([]byte(nil), row.TargetDigest...)
			row.TargetDigest[0]++
			if _, err := verifyCampaignHistoryRow(context.Background(), reader, row); err == nil {
				t.Fatal("target digest drift accepted")
			}
		})
	}
}

type campaignHistoryReaderFake struct {
	segment   campaignport.HistoricalCampaignSegment
	member    campaignport.HistoricalCampaignMember
	plan      campaignport.HistoricalBroadcastPlan
	recipient campaignport.HistoricalBroadcastRecipient
	message   campaignport.HistoricalBroadcastMessage
}

func (reader *campaignHistoryReaderFake) GetHistoricalCampaignSegment(_ context.Context, id int64) (campaignport.HistoricalCampaignSegment, error) {
	if id != reader.segment.ID {
		return campaignport.HistoricalCampaignSegment{}, campaignport.ErrCampaignHistoryConflict
	}
	return reader.segment, nil
}

func (reader *campaignHistoryReaderFake) GetHistoricalCampaignMember(_ context.Context, id int64) (campaignport.HistoricalCampaignMember, error) {
	if id != reader.member.ID {
		return campaignport.HistoricalCampaignMember{}, campaignport.ErrCampaignHistoryConflict
	}
	return reader.member, nil
}

func (reader *campaignHistoryReaderFake) GetHistoricalBroadcastPlan(_ context.Context, id int64) (campaignport.HistoricalBroadcastPlan, error) {
	if id != reader.plan.ID {
		return campaignport.HistoricalBroadcastPlan{}, campaignport.ErrCampaignHistoryConflict
	}
	return reader.plan, nil
}

func (reader *campaignHistoryReaderFake) GetHistoricalBroadcastRecipient(_ context.Context, id int64) (campaignport.HistoricalBroadcastRecipient, error) {
	if id != reader.recipient.ID {
		return campaignport.HistoricalBroadcastRecipient{}, campaignport.ErrCampaignHistoryConflict
	}
	return reader.recipient, nil
}

func (reader *campaignHistoryReaderFake) GetHistoricalBroadcastMessage(_ context.Context, id int64) (campaignport.HistoricalBroadcastMessage, error) {
	if id != reader.message.ID {
		return campaignport.HistoricalBroadcastMessage{}, campaignport.ErrCampaignHistoryConflict
	}
	return reader.message, nil
}

func (*campaignHistoryReaderFake) ListHistoricalCampaignSegments(context.Context, *int64, int32, int32) ([]campaignport.HistoricalCampaignSegment, int64, error) {
	return []campaignport.HistoricalCampaignSegment{}, 0, nil
}

func (*campaignHistoryReaderFake) ListHistoricalCampaignMembers(context.Context, *int64, *int64, int32, int32) ([]campaignport.HistoricalCampaignMember, int64, error) {
	return []campaignport.HistoricalCampaignMember{}, 0, nil
}

func (*campaignHistoryReaderFake) ListHistoricalBroadcastPlans(context.Context, int32, int32) ([]campaignport.HistoricalBroadcastPlan, int64, error) {
	return []campaignport.HistoricalBroadcastPlan{}, 0, nil
}

func (*campaignHistoryReaderFake) ListHistoricalBroadcastRecipients(context.Context, int64, int32, int32) ([]campaignport.HistoricalBroadcastRecipient, int64, error) {
	return []campaignport.HistoricalBroadcastRecipient{}, 0, nil
}

func (*campaignHistoryReaderFake) ListHistoricalBroadcastMessages(context.Context, int64, int32, int32) ([]campaignport.HistoricalBroadcastMessage, int64, error) {
	return []campaignport.HistoricalBroadcastMessage{}, 0, nil
}
