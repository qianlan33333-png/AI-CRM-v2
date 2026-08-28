package store

import (
	"context"

	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcdb "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store/generated"
)

var _ hxc.HXCRuntimeHistoryStore = (*HXCHistoryStore)(nil)
var _ hxc.HXCRuntimeHistoryReader = (*HXCHistoryReader)(nil)

func (s *HXCHistoryStore) CreateHistoricalHXCSenderConfig(ctx context.Context, value hxc.HistoricalHXCSenderConfig) (hxc.HistoricalHXCSenderConfig, error) {
	if value.ID != 0 || badRuntimeSenderConfig(value) {
		return hxc.HistoricalHXCSenderConfig{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := s.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCSenderConfig{}, err
	}
	row, err := queries.CreateHistoricalHXCSenderConfig(ctx, senderConfigArg(value))
	if err != nil {
		return hxc.HistoricalHXCSenderConfig{}, dbErr(err)
	}
	return senderConfig(row)
}

func (s *HXCHistoryStore) GetHistoricalHXCSenderConfig(ctx context.Context, id int64) (hxc.HistoricalHXCSenderConfig, error) {
	if id < 1 {
		return hxc.HistoricalHXCSenderConfig{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := s.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCSenderConfig{}, err
	}
	row, err := queries.GetHistoricalHXCSenderConfig(ctx, id)
	if err != nil {
		return hxc.HistoricalHXCSenderConfig{}, dbErr(err)
	}
	return senderConfig(row)
}

func (r *HXCHistoryReader) GetHistoricalHXCSenderConfig(ctx context.Context, id int64) (hxc.HistoricalHXCSenderConfig, error) {
	if id < 1 {
		return hxc.HistoricalHXCSenderConfig{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := r.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCSenderConfig{}, err
	}
	row, err := queries.GetHistoricalHXCSenderConfig(ctx, id)
	if err != nil {
		return hxc.HistoricalHXCSenderConfig{}, dbErr(err)
	}
	return senderConfig(row)
}

func (r *HXCHistoryReader) ListHistoricalHXCSenderConfig(ctx context.Context, query hxc.HXCHistoryQuery) ([]hxc.HistoricalHXCSenderConfig, int64, error) {
	if badQ(query, false, false) {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	queries, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalHXCSenderConfig(ctx)
	if err != nil {
		return nil, 0, dbErr(err)
	}
	rows, err := queries.ListHistoricalHXCSenderConfig(ctx, hxcdb.ListHistoricalHXCSenderConfigParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, dbErr(err)
	}
	values := make([]hxc.HistoricalHXCSenderConfig, 0, len(rows))
	for _, row := range rows {
		value, err := senderConfig(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (s *HXCHistoryStore) CreateHistoricalHXCSendRecord(ctx context.Context, value hxc.HistoricalHXCSendRecord) (hxc.HistoricalHXCSendRecord, error) {
	if value.ID != 0 || badRuntimeSendRecord(value) {
		return hxc.HistoricalHXCSendRecord{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := s.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCSendRecord{}, err
	}
	row, err := queries.CreateHistoricalHXCSendRecord(ctx, sendRecordArg(value))
	if err != nil {
		return hxc.HistoricalHXCSendRecord{}, dbErr(err)
	}
	return sendRecord(row)
}

func (s *HXCHistoryStore) GetHistoricalHXCSendRecord(ctx context.Context, id int64) (hxc.HistoricalHXCSendRecord, error) {
	if id < 1 {
		return hxc.HistoricalHXCSendRecord{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := s.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCSendRecord{}, err
	}
	row, err := queries.GetHistoricalHXCSendRecord(ctx, id)
	if err != nil {
		return hxc.HistoricalHXCSendRecord{}, dbErr(err)
	}
	return sendRecord(row)
}

func (r *HXCHistoryReader) GetHistoricalHXCSendRecord(ctx context.Context, id int64) (hxc.HistoricalHXCSendRecord, error) {
	if id < 1 {
		return hxc.HistoricalHXCSendRecord{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := r.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCSendRecord{}, err
	}
	row, err := queries.GetHistoricalHXCSendRecord(ctx, id)
	if err != nil {
		return hxc.HistoricalHXCSendRecord{}, dbErr(err)
	}
	return sendRecord(row)
}

func (r *HXCHistoryReader) ListHistoricalHXCSendRecord(ctx context.Context, query hxc.HXCHistoryQuery) ([]hxc.HistoricalHXCSendRecord, int64, error) {
	if badQ(query, false, false) {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	queries, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalHXCSendRecord(ctx)
	if err != nil {
		return nil, 0, dbErr(err)
	}
	rows, err := queries.ListHistoricalHXCSendRecord(ctx, hxcdb.ListHistoricalHXCSendRecordParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, dbErr(err)
	}
	values := make([]hxc.HistoricalHXCSendRecord, 0, len(rows))
	for _, row := range rows {
		value, err := sendRecord(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func senderConfigArg(value hxc.HistoricalHXCSenderConfig) hxcdb.CreateHistoricalHXCSenderConfigParams {
	return hxcdb.CreateHistoricalHXCSenderConfigParams{SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], PrivateDigest: value.PrivateDigest[:], Priority: value.Priority, OriginalIsActive: value.OriginalIsActive, CreatedAt: ts(value.CreatedAt), UpdatedAt: ts(value.UpdatedAt)}
}

func sendRecordArg(value hxc.HistoricalHXCSendRecord) hxcdb.CreateHistoricalHXCSendRecordParams {
	return hxcdb.CreateHistoricalHXCSendRecordParams{SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], PrivateDigest: value.PrivateDigest[:], TaskType: value.TaskType, OriginalStatus: value.OriginalStatus, SelectedCount: value.SelectedCount, EligibleCount: value.EligibleCount, SentCount: value.SentCount, SkippedCount: value.SkippedCount, PlannedCount: value.PlannedCount, QueuedCount: value.QueuedCount, DispatchingCount: value.DispatchingCount, SucceededCount: value.SucceededCount, FailedCount: value.FailedCount, BlockedCount: value.BlockedCount, CancelledCount: value.CancelledCount, ImageCount: value.ImageCount, IncludeDoNotDisturb: value.IncludeDoNotDisturb, TargetSource: value.TargetSource, TargetSourceID: i8(value.TargetSourceID), CreatedAt: ts(value.CreatedAt), LastStatusSyncAt: pts(value.LastStatusSyncAt), LastRefreshedAt: pts(value.LastRefreshedAt)}
}

func runtimeIdentityFromRow(id, sourceID int64, key, payload, field, private []byte) (hxc.HistoricalHXCRuntimeIdentity, bool) {
	if id < 1 || len(key) != 32 || len(payload) != 32 || len(field) != 32 || len(private) != 32 {
		return hxc.HistoricalHXCRuntimeIdentity{}, false
	}
	value := hxc.HistoricalHXCRuntimeIdentity{ID: id, SourceID: sourceID}
	copy(value.SourceKeyDigest[:], key)
	copy(value.SourcePayloadDigest[:], payload)
	copy(value.SourceFieldDigest[:], field)
	copy(value.PrivateDigest[:], private)
	return value, true
}

func senderConfig(row hxcdb.HxcV1SenderConfigHistory) (hxc.HistoricalHXCSenderConfig, error) {
	identity, validIdentity := runtimeIdentityFromRow(row.ID, row.SourceID, row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest, row.PrivateDigest)
	createdAt, validCreatedAt := tsv(row.CreatedAt)
	updatedAt, validUpdatedAt := tsv(row.UpdatedAt)
	value := hxc.HistoricalHXCSenderConfig{HistoricalHXCRuntimeIdentity: identity, Priority: row.Priority, OriginalIsActive: row.OriginalIsActive, CreatedAt: createdAt, UpdatedAt: updatedAt}
	if !validIdentity || !validCreatedAt || !validUpdatedAt || badRuntimeSenderConfig(value) {
		return hxc.HistoricalHXCSenderConfig{}, hxc.ErrHXCHistoryUnavailable
	}
	return value, nil
}

func sendRecord(row hxcdb.HxcV1SendRecordHistory) (hxc.HistoricalHXCSendRecord, error) {
	identity, validIdentity := runtimeIdentityFromRow(row.ID, row.SourceID, row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest, row.PrivateDigest)
	createdAt, validCreatedAt := tsv(row.CreatedAt)
	lastStatusSyncAt, validLastStatusSyncAt := ptsv(row.LastStatusSyncAt)
	lastRefreshedAt, validLastRefreshedAt := ptsv(row.LastRefreshedAt)
	value := hxc.HistoricalHXCSendRecord{HistoricalHXCRuntimeIdentity: identity, TaskType: row.TaskType, OriginalStatus: row.OriginalStatus, SelectedCount: row.SelectedCount, EligibleCount: row.EligibleCount, SentCount: row.SentCount, SkippedCount: row.SkippedCount, PlannedCount: row.PlannedCount, QueuedCount: row.QueuedCount, DispatchingCount: row.DispatchingCount, SucceededCount: row.SucceededCount, FailedCount: row.FailedCount, BlockedCount: row.BlockedCount, CancelledCount: row.CancelledCount, ImageCount: row.ImageCount, IncludeDoNotDisturb: row.IncludeDoNotDisturb, TargetSource: row.TargetSource, TargetSourceID: i8v(row.TargetSourceID), CreatedAt: createdAt, LastStatusSyncAt: lastStatusSyncAt, LastRefreshedAt: lastRefreshedAt}
	if !validIdentity || !validCreatedAt || !validLastStatusSyncAt || !validLastRefreshedAt || badRuntimeSendRecord(value) {
		return hxc.HistoricalHXCSendRecord{}, hxc.ErrHXCHistoryUnavailable
	}
	return value, nil
}

func badRuntimeSenderConfig(value hxc.HistoricalHXCSenderConfig) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := hxcapp.HistoricalHXCSenderConfigDigest(value)
	return err != nil
}

func badRuntimeSendRecord(value hxc.HistoricalHXCSendRecord) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := hxcapp.HistoricalHXCSendRecordDigest(value)
	return err != nil
}
