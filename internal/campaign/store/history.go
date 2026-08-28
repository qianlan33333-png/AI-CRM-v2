package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CampaignHistoryStore struct{}
type CampaignHistoryReader struct{ db campaigndb.DBTX }

var _ campaignport.CampaignHistoryStore = (*CampaignHistoryStore)(nil)
var _ campaignport.CampaignHistoryReader = (*CampaignHistoryReader)(nil)

func NewCampaignHistoryStore() *CampaignHistoryStore { return &CampaignHistoryStore{} }
func NewCampaignHistoryReader(db campaigndb.DBTX) *CampaignHistoryReader {
	return &CampaignHistoryReader{db: db}
}

func (store *CampaignHistoryStore) queries(ctx context.Context) (*campaigndb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	return campaigndb.New(tx), nil
}

func (reader *CampaignHistoryReader) queries(ctx context.Context) (*campaigndb.Queries, error) {
	if reader == nil || reader.db == nil || ctx == nil || ctx.Err() != nil {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	value := reflect.ValueOf(reader.db)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	return campaigndb.New(reader.db), nil
}

func (store *CampaignHistoryStore) CreateHistoricalCampaignSegment(ctx context.Context, value campaignport.HistoricalCampaignSegment) (campaignport.HistoricalCampaignSegment, error) {
	if value.ID != 0 || !campaignHistorySegmentValid(value, 1) {
		return campaignport.HistoricalCampaignSegment{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignSegment{}, err
	}
	row, err := q.CreateHistoricalCampaignSegment(ctx, campaigndb.CreateHistoricalCampaignSegmentParams{
		SourceID: value.SourceID, CampaignSourceID: value.CampaignSourceID, SegmentSourceID: value.SegmentSourceID,
		SourceParentState: value.SourceParentState, Code: value.Code, Priority: value.Priority, Label: value.Label,
		CreatedAt: campaignHistoryTimestamp(value.CreatedAt), SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return campaignport.HistoricalCampaignSegment{}, campaignHistoryError(err)
	}
	return campaignHistorySegmentValue(row)
}

func (store *CampaignHistoryStore) GetHistoricalCampaignSegment(ctx context.Context, id int64) (campaignport.HistoricalCampaignSegment, error) {
	if id < 1 {
		return campaignport.HistoricalCampaignSegment{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignSegment{}, err
	}
	row, err := q.GetHistoricalCampaignSegment(ctx, id)
	if err != nil {
		return campaignport.HistoricalCampaignSegment{}, campaignHistoryError(err)
	}
	return campaignHistorySegmentValue(row)
}

func (reader *CampaignHistoryReader) GetHistoricalCampaignSegment(ctx context.Context, id int64) (campaignport.HistoricalCampaignSegment, error) {
	if id < 1 {
		return campaignport.HistoricalCampaignSegment{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignSegment{}, err
	}
	row, err := q.GetHistoricalCampaignSegment(ctx, id)
	if err != nil {
		return campaignport.HistoricalCampaignSegment{}, campaignHistoryError(err)
	}
	return campaignHistorySegmentValue(row)
}

func (store *CampaignHistoryStore) CreateHistoricalCampaignMember(ctx context.Context, value campaignport.HistoricalCampaignMember) (campaignport.HistoricalCampaignMember, error) {
	if value.ID != 0 || !campaignHistoryMemberValid(value, 1) {
		return campaignport.HistoricalCampaignMember{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignMember{}, err
	}
	row, err := q.CreateHistoricalCampaignMember(ctx, campaigndb.CreateHistoricalCampaignMemberParams{
		SourceID: value.SourceID, CampaignSourceID: value.CampaignSourceID, CampaignSegmentSourceID: value.CampaignSegmentSourceID,
		SegmentSourceID: value.SegmentSourceID, MemberSourceID: value.MemberSourceID, SegmentHistoryID: value.SegmentHistoryID,
		CustomerID: campaignHistoryInt(value.CustomerID), JoinedAt: campaignHistoryTimestamp(value.JoinedAt), AnchorDate: value.AnchorDate,
		CurrentStepIndex: value.CurrentStepIndex, NextDueAt: campaignHistoryOptionalTimestamp(value.NextDueAt), OriginalStatus: value.OriginalStatus,
		StopReason: value.StopReason, LastStepSentAt: campaignHistoryOptionalTimestamp(value.LastStepSentAt), RetryCount: value.RetryCount,
		CreatedAt: campaignHistoryTimestamp(value.CreatedAt), UpdatedAt: campaignHistoryTimestamp(value.UpdatedAt), SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return campaignport.HistoricalCampaignMember{}, campaignHistoryError(err)
	}
	return campaignHistoryMemberValue(row)
}

func (store *CampaignHistoryStore) GetHistoricalCampaignMember(ctx context.Context, id int64) (campaignport.HistoricalCampaignMember, error) {
	if id < 1 {
		return campaignport.HistoricalCampaignMember{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignMember{}, err
	}
	row, err := q.GetHistoricalCampaignMember(ctx, id)
	if err != nil {
		return campaignport.HistoricalCampaignMember{}, campaignHistoryError(err)
	}
	return campaignHistoryMemberValue(row)
}

func (reader *CampaignHistoryReader) GetHistoricalCampaignMember(ctx context.Context, id int64) (campaignport.HistoricalCampaignMember, error) {
	if id < 1 {
		return campaignport.HistoricalCampaignMember{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignMember{}, err
	}
	row, err := q.GetHistoricalCampaignMember(ctx, id)
	if err != nil {
		return campaignport.HistoricalCampaignMember{}, campaignHistoryError(err)
	}
	return campaignHistoryMemberValue(row)
}

func (store *CampaignHistoryStore) CreateHistoricalBroadcastPlan(ctx context.Context, value campaignport.HistoricalBroadcastPlan) (campaignport.HistoricalBroadcastPlan, error) {
	if value.ID != 0 || !campaignHistoryPlanValid(value, 1) {
		return campaignport.HistoricalBroadcastPlan{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastPlan{}, err
	}
	row, err := q.CreateHistoricalBroadcastPlan(ctx, campaigndb.CreateHistoricalBroadcastPlanParams{
		SourceID: value.SourceID, SourcePlanID: value.SourcePlanID, CampaignSourceID: campaignHistoryInt(value.CampaignSourceID),
		SegmentSourceID: campaignHistoryInt(value.SegmentSourceID), DisplayName: value.DisplayName, Intent: value.Intent,
		ContentStrategy: value.ContentStrategy, ContentTemplateMasked: value.ContentTemplateMasked, MaxRecipients: value.MaxRecipients,
		CandidateCount: value.CandidateCount, SkippedCount: value.SkippedCount, RequiresManualCopy: value.RequiresManualCopy,
		OriginalStatus: value.OriginalStatus, OriginalReviewStatus: value.OriginalReviewStatus, OriginalRunStatus: value.OriginalRunStatus,
		CommittedAt: campaignHistoryOptionalTimestamp(value.CommittedAt), ExpiresAt: campaignHistoryOptionalTimestamp(value.ExpiresAt),
		CreatedAt: campaignHistoryTimestamp(value.CreatedAt), UpdatedAt: campaignHistoryTimestamp(value.UpdatedAt), RuntimeDigest: value.RuntimeDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return campaignport.HistoricalBroadcastPlan{}, campaignHistoryError(err)
	}
	return campaignHistoryPlanValue(row)
}

func (store *CampaignHistoryStore) GetHistoricalBroadcastPlan(ctx context.Context, id int64) (campaignport.HistoricalBroadcastPlan, error) {
	if id < 1 {
		return campaignport.HistoricalBroadcastPlan{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastPlan{}, err
	}
	row, err := q.GetHistoricalBroadcastPlan(ctx, id)
	if err != nil {
		return campaignport.HistoricalBroadcastPlan{}, campaignHistoryError(err)
	}
	return campaignHistoryPlanValue(row)
}

func (reader *CampaignHistoryReader) GetHistoricalBroadcastPlan(ctx context.Context, id int64) (campaignport.HistoricalBroadcastPlan, error) {
	if id < 1 {
		return campaignport.HistoricalBroadcastPlan{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastPlan{}, err
	}
	row, err := q.GetHistoricalBroadcastPlan(ctx, id)
	if err != nil {
		return campaignport.HistoricalBroadcastPlan{}, campaignHistoryError(err)
	}
	return campaignHistoryPlanValue(row)
}

func (store *CampaignHistoryStore) CreateHistoricalBroadcastRecipient(ctx context.Context, value campaignport.HistoricalBroadcastRecipient) (campaignport.HistoricalBroadcastRecipient, error) {
	if value.ID != 0 || !campaignHistoryRecipientValid(value, 1) {
		return campaignport.HistoricalBroadcastRecipient{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, err
	}
	row, err := q.CreateHistoricalBroadcastRecipient(ctx, campaigndb.CreateHistoricalBroadcastRecipientParams{
		SourceID: value.SourceID, PlanHistoryID: value.PlanHistoryID, CustomerID: campaignHistoryInt(value.CustomerID),
		DisplayName: value.DisplayName, PlannedMessageCount: value.PlannedMessageCount, OriginalApprovalStatus: value.OriginalApprovalStatus,
		OriginalSendStatus: value.OriginalSendStatus, ApprovedAt: campaignHistoryOptionalTimestamp(value.ApprovedAt),
		RejectedAt: campaignHistoryOptionalTimestamp(value.RejectedAt), CreatedAt: campaignHistoryTimestamp(value.CreatedAt),
		UpdatedAt: campaignHistoryTimestamp(value.UpdatedAt), SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, campaignHistoryError(err)
	}
	return campaignHistoryRecipientValue(row)
}

func (store *CampaignHistoryStore) GetHistoricalBroadcastRecipient(ctx context.Context, id int64) (campaignport.HistoricalBroadcastRecipient, error) {
	if id < 1 {
		return campaignport.HistoricalBroadcastRecipient{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, err
	}
	row, err := q.GetHistoricalBroadcastRecipient(ctx, id)
	if err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, campaignHistoryError(err)
	}
	return campaignHistoryRecipientValue(row)
}

func (reader *CampaignHistoryReader) GetHistoricalBroadcastRecipient(ctx context.Context, id int64) (campaignport.HistoricalBroadcastRecipient, error) {
	if id < 1 {
		return campaignport.HistoricalBroadcastRecipient{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, err
	}
	row, err := q.GetHistoricalBroadcastRecipient(ctx, id)
	if err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, campaignHistoryError(err)
	}
	return campaignHistoryRecipientValue(row)
}

func (store *CampaignHistoryStore) CreateHistoricalBroadcastMessage(ctx context.Context, value campaignport.HistoricalBroadcastMessage) (campaignport.HistoricalBroadcastMessage, error) {
	if value.ID != 0 || !campaignHistoryMessageValid(value, 1) {
		return campaignport.HistoricalBroadcastMessage{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastMessage{}, err
	}
	row, err := q.CreateHistoricalBroadcastMessage(ctx, campaigndb.CreateHistoricalBroadcastMessageParams{
		SourceID: value.SourceID, PlanHistoryID: value.PlanHistoryID, RecipientHistoryID: value.RecipientHistoryID,
		CustomerID: campaignHistoryInt(value.CustomerID), SequenceIndex: value.SequenceIndex, DayOffset: value.DayOffset,
		OriginalSendTime: value.OriginalSendTime, ContentMasked: value.ContentMasked, OriginalStatus: value.OriginalStatus,
		SentAt: campaignHistoryOptionalTimestamp(value.SentAt), CreatedAt: campaignHistoryTimestamp(value.CreatedAt),
		UpdatedAt: campaignHistoryTimestamp(value.UpdatedAt), ContentPayloadDigest: value.ContentPayloadDigest[:],
		AttachmentsDigest: value.AttachmentsDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return campaignport.HistoricalBroadcastMessage{}, campaignHistoryError(err)
	}
	return campaignHistoryMessageValue(row)
}

func (store *CampaignHistoryStore) GetHistoricalBroadcastMessage(ctx context.Context, id int64) (campaignport.HistoricalBroadcastMessage, error) {
	if id < 1 {
		return campaignport.HistoricalBroadcastMessage{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastMessage{}, err
	}
	row, err := q.GetHistoricalBroadcastMessage(ctx, id)
	if err != nil {
		return campaignport.HistoricalBroadcastMessage{}, campaignHistoryError(err)
	}
	return campaignHistoryMessageValue(row)
}

func (reader *CampaignHistoryReader) GetHistoricalBroadcastMessage(ctx context.Context, id int64) (campaignport.HistoricalBroadcastMessage, error) {
	if id < 1 {
		return campaignport.HistoricalBroadcastMessage{}, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return campaignport.HistoricalBroadcastMessage{}, err
	}
	row, err := q.GetHistoricalBroadcastMessage(ctx, id)
	if err != nil {
		return campaignport.HistoricalBroadcastMessage{}, campaignHistoryError(err)
	}
	return campaignHistoryMessageValue(row)
}

func (reader *CampaignHistoryReader) ListHistoricalCampaignSegments(ctx context.Context, campaignSourceID *int64, limit, offset int32) ([]campaignport.HistoricalCampaignSegment, int64, error) {
	if !campaignHistoryPage(limit, offset) || !campaignHistoryFilter(campaignSourceID) {
		return nil, 0, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalCampaignSegments(ctx, campaignHistoryInt(campaignSourceID))
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	rows, err := q.ListHistoricalCampaignSegments(ctx, campaigndb.ListHistoricalCampaignSegmentsParams{CampaignSourceID: campaignHistoryInt(campaignSourceID), PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	items := make([]campaignport.HistoricalCampaignSegment, 0, len(rows))
	for _, row := range rows {
		value, err := campaignHistorySegmentValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (reader *CampaignHistoryReader) ListHistoricalCampaignMembers(ctx context.Context, segmentHistoryID, customerID *int64, limit, offset int32) ([]campaignport.HistoricalCampaignMember, int64, error) {
	if !campaignHistoryPage(limit, offset) || !campaignHistoryFilter(segmentHistoryID) || !campaignHistoryFilter(customerID) {
		return nil, 0, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	params := campaigndb.CountHistoricalCampaignMembersParams{SegmentHistoryID: campaignHistoryInt(segmentHistoryID), CustomerID: campaignHistoryInt(customerID)}
	total, err := q.CountHistoricalCampaignMembers(ctx, params)
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	rows, err := q.ListHistoricalCampaignMembers(ctx, campaigndb.ListHistoricalCampaignMembersParams{SegmentHistoryID: params.SegmentHistoryID, CustomerID: params.CustomerID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	items := make([]campaignport.HistoricalCampaignMember, 0, len(rows))
	for _, row := range rows {
		value, err := campaignHistoryMemberValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (reader *CampaignHistoryReader) ListHistoricalBroadcastPlans(ctx context.Context, limit, offset int32) ([]campaignport.HistoricalBroadcastPlan, int64, error) {
	if !campaignHistoryPage(limit, offset) {
		return nil, 0, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalBroadcastPlans(ctx)
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	rows, err := q.ListHistoricalBroadcastPlans(ctx, campaigndb.ListHistoricalBroadcastPlansParams{PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	items := make([]campaignport.HistoricalBroadcastPlan, 0, len(rows))
	for _, row := range rows {
		value, err := campaignHistoryPlanValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (reader *CampaignHistoryReader) ListHistoricalBroadcastRecipients(ctx context.Context, planHistoryID int64, limit, offset int32) ([]campaignport.HistoricalBroadcastRecipient, int64, error) {
	if planHistoryID < 1 || !campaignHistoryPage(limit, offset) {
		return nil, 0, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalBroadcastRecipients(ctx, planHistoryID)
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	rows, err := q.ListHistoricalBroadcastRecipients(ctx, campaigndb.ListHistoricalBroadcastRecipientsParams{PlanHistoryID: planHistoryID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	items := make([]campaignport.HistoricalBroadcastRecipient, 0, len(rows))
	for _, row := range rows {
		value, err := campaignHistoryRecipientValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (reader *CampaignHistoryReader) ListHistoricalBroadcastMessages(ctx context.Context, recipientHistoryID int64, limit, offset int32) ([]campaignport.HistoricalBroadcastMessage, int64, error) {
	if recipientHistoryID < 1 || !campaignHistoryPage(limit, offset) {
		return nil, 0, campaignport.ErrCampaignHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalBroadcastMessages(ctx, recipientHistoryID)
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	rows, err := q.ListHistoricalBroadcastMessages(ctx, campaigndb.ListHistoricalBroadcastMessagesParams{RecipientHistoryID: recipientHistoryID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, campaignHistoryError(err)
	}
	items := make([]campaignport.HistoricalBroadcastMessage, 0, len(rows))
	for _, row := range rows {
		value, err := campaignHistoryMessageValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func campaignHistorySegmentValid(value campaignport.HistoricalCampaignSegment, id int64) bool {
	value.ID = id
	_, err := campaignapp.HistoricalCampaignSegmentDigest(value)
	return err == nil
}

func campaignHistoryMemberValid(value campaignport.HistoricalCampaignMember, id int64) bool {
	value.ID = id
	_, err := campaignapp.HistoricalCampaignMemberDigest(value)
	return err == nil
}

func campaignHistoryPlanValid(value campaignport.HistoricalBroadcastPlan, id int64) bool {
	value.ID = id
	_, err := campaignapp.HistoricalBroadcastPlanDigest(value)
	return err == nil
}

func campaignHistoryRecipientValid(value campaignport.HistoricalBroadcastRecipient, id int64) bool {
	value.ID = id
	_, err := campaignapp.HistoricalBroadcastRecipientDigest(value)
	return err == nil
}

func campaignHistoryMessageValid(value campaignport.HistoricalBroadcastMessage, id int64) bool {
	value.ID = id
	_, err := campaignapp.HistoricalBroadcastMessageDigest(value)
	return err == nil
}

func campaignHistorySegmentValue(row campaigndb.CampaignV1HistorySegment) (campaignport.HistoricalCampaignSegment, error) {
	if !campaignHistoryFinite(row.CreatedAt) || len(row.SourcePayloadDigest) != 32 {
		return campaignport.HistoricalCampaignSegment{}, campaignport.ErrCampaignHistoryUnavailable
	}
	value := campaignport.HistoricalCampaignSegment{ID: row.ID, SourceID: row.SourceID, CampaignSourceID: row.CampaignSourceID, SegmentSourceID: row.SegmentSourceID,
		SourceParentState: row.SourceParentState, Code: row.Code, Priority: row.Priority, Label: row.Label, CreatedAt: campaignHistoryTime(row.CreatedAt)}
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	if !campaignHistorySegmentValid(value, value.ID) {
		return campaignport.HistoricalCampaignSegment{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return value, nil
}

func campaignHistoryMemberValue(row campaigndb.CampaignV1HistoryMember) (campaignport.HistoricalCampaignMember, error) {
	if !campaignHistoryFinite(row.JoinedAt) || !campaignHistoryFinite(row.CreatedAt) || !campaignHistoryFinite(row.UpdatedAt) || len(row.SourcePayloadDigest) != 32 {
		return campaignport.HistoricalCampaignMember{}, campaignport.ErrCampaignHistoryUnavailable
	}
	value := campaignport.HistoricalCampaignMember{ID: row.ID, SourceID: row.SourceID, CampaignSourceID: row.CampaignSourceID, CampaignSegmentSourceID: row.CampaignSegmentSourceID,
		SegmentSourceID: row.SegmentSourceID, MemberSourceID: row.MemberSourceID, SegmentHistoryID: row.SegmentHistoryID, JoinedAt: campaignHistoryTime(row.JoinedAt),
		AnchorDate: row.AnchorDate, CurrentStepIndex: row.CurrentStepIndex, OriginalStatus: row.OriginalStatus, StopReason: row.StopReason, RetryCount: row.RetryCount,
		CreatedAt: campaignHistoryTime(row.CreatedAt), UpdatedAt: campaignHistoryTime(row.UpdatedAt)}
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	var err error
	if value.CustomerID, err = campaignHistoryOptionalInt(row.CustomerID); err != nil {
		return campaignport.HistoricalCampaignMember{}, err
	}
	if value.NextDueAt, err = campaignHistoryOptionalTime(row.NextDueAt); err != nil {
		return campaignport.HistoricalCampaignMember{}, err
	}
	if value.LastStepSentAt, err = campaignHistoryOptionalTime(row.LastStepSentAt); err != nil {
		return campaignport.HistoricalCampaignMember{}, err
	}
	if !campaignHistoryMemberValid(value, value.ID) {
		return campaignport.HistoricalCampaignMember{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return value, nil
}

func campaignHistoryPlanValue(row campaigndb.CampaignV1HistoryBroadcastPlan) (campaignport.HistoricalBroadcastPlan, error) {
	if !campaignHistoryFinite(row.CreatedAt) || !campaignHistoryFinite(row.UpdatedAt) || len(row.RuntimeDigest) != 32 || len(row.SourcePayloadDigest) != 32 {
		return campaignport.HistoricalBroadcastPlan{}, campaignport.ErrCampaignHistoryUnavailable
	}
	value := campaignport.HistoricalBroadcastPlan{ID: row.ID, SourceID: row.SourceID, SourcePlanID: row.SourcePlanID, DisplayName: row.DisplayName,
		Intent: row.Intent, ContentStrategy: row.ContentStrategy, ContentTemplateMasked: row.ContentTemplateMasked, MaxRecipients: row.MaxRecipients,
		CandidateCount: row.CandidateCount, SkippedCount: row.SkippedCount, RequiresManualCopy: row.RequiresManualCopy, OriginalStatus: row.OriginalStatus,
		OriginalReviewStatus: row.OriginalReviewStatus, OriginalRunStatus: row.OriginalRunStatus, CreatedAt: campaignHistoryTime(row.CreatedAt), UpdatedAt: campaignHistoryTime(row.UpdatedAt)}
	copy(value.RuntimeDigest[:], row.RuntimeDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	var err error
	if value.CampaignSourceID, err = campaignHistoryOptionalInt(row.CampaignSourceID); err != nil {
		return campaignport.HistoricalBroadcastPlan{}, err
	}
	if value.SegmentSourceID, err = campaignHistoryOptionalInt(row.SegmentSourceID); err != nil {
		return campaignport.HistoricalBroadcastPlan{}, err
	}
	if value.CommittedAt, err = campaignHistoryOptionalTime(row.CommittedAt); err != nil {
		return campaignport.HistoricalBroadcastPlan{}, err
	}
	if value.ExpiresAt, err = campaignHistoryOptionalTime(row.ExpiresAt); err != nil {
		return campaignport.HistoricalBroadcastPlan{}, err
	}
	if !campaignHistoryPlanValid(value, value.ID) {
		return campaignport.HistoricalBroadcastPlan{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return value, nil
}

func campaignHistoryRecipientValue(row campaigndb.CampaignV1HistoryBroadcastRecipient) (campaignport.HistoricalBroadcastRecipient, error) {
	if !campaignHistoryFinite(row.CreatedAt) || !campaignHistoryFinite(row.UpdatedAt) || len(row.SourcePayloadDigest) != 32 {
		return campaignport.HistoricalBroadcastRecipient{}, campaignport.ErrCampaignHistoryUnavailable
	}
	value := campaignport.HistoricalBroadcastRecipient{ID: row.ID, SourceID: row.SourceID, PlanHistoryID: row.PlanHistoryID, DisplayName: row.DisplayName,
		PlannedMessageCount: row.PlannedMessageCount, OriginalApprovalStatus: row.OriginalApprovalStatus, OriginalSendStatus: row.OriginalSendStatus,
		CreatedAt: campaignHistoryTime(row.CreatedAt), UpdatedAt: campaignHistoryTime(row.UpdatedAt)}
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	var err error
	if value.CustomerID, err = campaignHistoryOptionalInt(row.CustomerID); err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, err
	}
	if value.ApprovedAt, err = campaignHistoryOptionalTime(row.ApprovedAt); err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, err
	}
	if value.RejectedAt, err = campaignHistoryOptionalTime(row.RejectedAt); err != nil {
		return campaignport.HistoricalBroadcastRecipient{}, err
	}
	if !campaignHistoryRecipientValid(value, value.ID) {
		return campaignport.HistoricalBroadcastRecipient{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return value, nil
}

func campaignHistoryMessageValue(row campaigndb.CampaignV1HistoryBroadcastMessage) (campaignport.HistoricalBroadcastMessage, error) {
	if !campaignHistoryFinite(row.CreatedAt) || !campaignHistoryFinite(row.UpdatedAt) || len(row.ContentPayloadDigest) != 32 || len(row.AttachmentsDigest) != 32 || len(row.SourcePayloadDigest) != 32 {
		return campaignport.HistoricalBroadcastMessage{}, campaignport.ErrCampaignHistoryUnavailable
	}
	value := campaignport.HistoricalBroadcastMessage{ID: row.ID, SourceID: row.SourceID, PlanHistoryID: row.PlanHistoryID, RecipientHistoryID: row.RecipientHistoryID,
		SequenceIndex: row.SequenceIndex, DayOffset: row.DayOffset, OriginalSendTime: row.OriginalSendTime, ContentMasked: row.ContentMasked,
		OriginalStatus: row.OriginalStatus, CreatedAt: campaignHistoryTime(row.CreatedAt), UpdatedAt: campaignHistoryTime(row.UpdatedAt)}
	copy(value.ContentPayloadDigest[:], row.ContentPayloadDigest)
	copy(value.AttachmentsDigest[:], row.AttachmentsDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	var err error
	if value.CustomerID, err = campaignHistoryOptionalInt(row.CustomerID); err != nil {
		return campaignport.HistoricalBroadcastMessage{}, err
	}
	if value.SentAt, err = campaignHistoryOptionalTime(row.SentAt); err != nil {
		return campaignport.HistoricalBroadcastMessage{}, err
	}
	if !campaignHistoryMessageValid(value, value.ID) {
		return campaignport.HistoricalBroadcastMessage{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return value, nil
}

func campaignHistoryPage(limit, offset int32) bool { return limit >= 1 && limit <= 100 && offset >= 0 }
func campaignHistoryFilter(value *int64) bool      { return value == nil || *value > 0 }

func campaignHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func campaignHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func campaignHistoryOptionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return campaignHistoryTimestamp(*value)
}

func campaignHistoryFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}

func campaignHistoryTime(value pgtype.Timestamptz) time.Time {
	return value.Time.UTC().Truncate(time.Microsecond)
}

func campaignHistoryOptionalInt(value pgtype.Int8) (*int64, error) {
	if !value.Valid {
		return nil, nil
	}
	result := value.Int64
	return &result, nil
}

func campaignHistoryOptionalTime(value pgtype.Timestamptz) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	if !campaignHistoryFinite(value) {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	result := campaignHistoryTime(value)
	return &result, nil
}

func campaignHistoryError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return campaignport.ErrCampaignHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return campaignport.ErrCampaignHistoryConflict
	}
	return campaignport.ErrCampaignHistoryUnavailable
}
