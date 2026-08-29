package store

import (
	"context"
	"encoding/json"
	"time"

	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcdb "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store/generated"
)

var _ hxc.HXCChatJobHistoryStore = (*HXCHistoryStore)(nil)
var _ hxc.HXCChatJobHistoryReader = (*HXCHistoryReader)(nil)

func (s *HXCHistoryStore) CreateHistoricalHXCChatJob(ctx context.Context, value hxc.HistoricalHXCChatJob) (hxc.HistoricalHXCChatJob, error) {
	value = normalizeHXCChatJobForStore(value)
	if value.ID != 0 || badHXCChatJob(value) {
		return hxc.HistoricalHXCChatJob{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := s.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCChatJob{}, err
	}
	row, err := queries.CreateHistoricalHXCChatJob(ctx, chatJobArg(value))
	if err != nil {
		return hxc.HistoricalHXCChatJob{}, dbErr(err)
	}
	return chatJob(row)
}

func (s *HXCHistoryStore) GetHistoricalHXCChatJob(ctx context.Context, id int64) (hxc.HistoricalHXCChatJob, error) {
	if id < 1 {
		return hxc.HistoricalHXCChatJob{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := s.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCChatJob{}, err
	}
	row, err := queries.GetHistoricalHXCChatJob(ctx, id)
	if err != nil {
		return hxc.HistoricalHXCChatJob{}, dbErr(err)
	}
	return chatJob(row)
}

func (r *HXCHistoryReader) GetHistoricalHXCChatJob(ctx context.Context, id int64) (hxc.HistoricalHXCChatJob, error) {
	if id < 1 {
		return hxc.HistoricalHXCChatJob{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := r.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCChatJob{}, err
	}
	row, err := queries.GetHistoricalHXCChatJob(ctx, id)
	if err != nil {
		return hxc.HistoricalHXCChatJob{}, dbErr(err)
	}
	return chatJob(row)
}

func (r *HXCHistoryReader) ListHistoricalHXCChatJob(ctx context.Context, query hxc.HXCChatJobHistoryQuery) ([]hxc.HistoricalHXCChatJob, int64, error) {
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	queries, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalHXCChatJob(ctx)
	if err != nil {
		return nil, 0, dbErr(err)
	}
	rows, err := queries.ListHistoricalHXCChatJob(ctx, hxcdb.ListHistoricalHXCChatJobParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, dbErr(err)
	}
	values := make([]hxc.HistoricalHXCChatJob, 0, len(rows))
	for _, row := range rows {
		value, err := chatJob(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func chatJobArg(value hxc.HistoricalHXCChatJob) hxcdb.CreateHistoricalHXCChatJobParams {
	return hxcdb.CreateHistoricalHXCChatJobParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		QueueSourceID: i8(value.QueueSourceID), MemberSourceID: i8(value.MemberSourceID), ExternalContactID: value.ExternalContactID, Phone: value.Phone,
		ExternalMessageID: value.ExternalMessageID, ExternalSessionID: value.ExternalSessionID, LaohuangTaskID: value.LaohuangTaskID,
		RequestPayloadJson: string(value.RequestPayloadJSON), AcceptedPayloadJson: string(value.AcceptedPayloadJSON), CallbackPayloadJson: string(value.CallbackPayloadJSON),
		OriginalStatus: value.OriginalStatus, ReplyText: value.ReplyText, ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage, SendChannel: value.SendChannel,
		SendRecordSourceID: i8(value.SendRecordSourceID), SendResultJson: string(value.SendResultJSON), CreatedAt: ts(value.CreatedAt), UpdatedAt: ts(value.UpdatedAt),
		FinishedAtSource: value.FinishedAtSource,
	}
}

func chatJob(row hxcdb.HxcV1ChatJobHistory) (hxc.HistoricalHXCChatJob, error) {
	if row.ID < 1 || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 || len(row.SourceFieldDigest) != 32 {
		return hxc.HistoricalHXCChatJob{}, hxc.ErrHXCHistoryUnavailable
	}
	createdAt, validCreatedAt := tsv(row.CreatedAt)
	updatedAt, validUpdatedAt := tsv(row.UpdatedAt)
	value := hxc.HistoricalHXCChatJob{
		ID: row.ID, SourceID: row.SourceID, QueueSourceID: i8v(row.QueueSourceID), MemberSourceID: i8v(row.MemberSourceID),
		ExternalContactID: row.ExternalContactID, Phone: row.Phone, ExternalMessageID: row.ExternalMessageID, ExternalSessionID: row.ExternalSessionID, LaohuangTaskID: row.LaohuangTaskID,
		RequestPayloadJSON: json.RawMessage(row.RequestPayloadJson), AcceptedPayloadJSON: json.RawMessage(row.AcceptedPayloadJson), CallbackPayloadJSON: json.RawMessage(row.CallbackPayloadJson),
		OriginalStatus: row.OriginalStatus, ReplyText: row.ReplyText, ErrorCode: row.ErrorCode, ErrorMessage: row.ErrorMessage, SendChannel: row.SendChannel,
		SendRecordSourceID: i8v(row.SendRecordSourceID), SendResultJSON: json.RawMessage(row.SendResultJson), CreatedAt: createdAt, UpdatedAt: updatedAt, FinishedAtSource: row.FinishedAtSource,
	}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	copy(value.SourceFieldDigest[:], row.SourceFieldDigest)
	if !validCreatedAt || !validUpdatedAt || badHXCChatJob(value) {
		return hxc.HistoricalHXCChatJob{}, hxc.ErrHXCHistoryUnavailable
	}
	return value, nil
}

func badHXCChatJob(value hxc.HistoricalHXCChatJob) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := hxcapp.HistoricalHXCChatJobDigest(value)
	return err != nil
}

func normalizeHXCChatJobForStore(value hxc.HistoricalHXCChatJob) hxc.HistoricalHXCChatJob {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	return value
}
