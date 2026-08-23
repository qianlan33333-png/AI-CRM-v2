package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// CRUDRepository is the Segment-owned transaction-bound persistence adapter.
type CRUDRepository struct{}

var _ segmentapp.CRUDStore = (*CRUDRepository)(nil)

func NewCRUDRepository() *CRUDRepository { return &CRUDRepository{} }

func (repository *CRUDRepository) ListSegments(ctx context.Context, query segmentapp.SegmentPageQuery) ([]segmentport.Segment, error) {
	queries, err := crudQueries(ctx)
	if err != nil || repository == nil || query.Limit < 1 {
		return nil, crudStoreError(err)
	}
	rows, err := queries.ListSegments(ctx, segmentdb.ListSegmentsParams{
		AfterID: crudOptionalID(query.AfterID), RowLimit: query.Limit,
	})
	if err != nil {
		return nil, crudStoreError(err)
	}
	result := make([]segmentport.Segment, len(rows))
	for index, row := range rows {
		result[index], err = crudStoredLifecycleSegment(row.ID, row.Name, row.Definition, row.RefreshMode, row.RefreshCron, row.MemberCount,
			row.RefreshedAt, row.RefreshStatus, row.CreatedAt, row.UpdatedAt, row.LifecycleStatus)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (repository *CRUDRepository) GetSegment(ctx context.Context, id segmentport.SegmentID) (segmentport.Segment, error) {
	queries, err := crudQueries(ctx)
	if err != nil || repository == nil || id <= 0 {
		return segmentport.Segment{}, crudStoreError(err)
	}
	row, err := queries.GetSegment(ctx, int64(id))
	if err != nil {
		return segmentport.Segment{}, crudStoreError(err)
	}
	return crudStoredLifecycleSegment(row.ID, row.Name, row.Definition, row.RefreshMode, row.RefreshCron, row.MemberCount,
		row.RefreshedAt, row.RefreshStatus, row.CreatedAt, row.UpdatedAt, row.LifecycleStatus)
}

func (repository *CRUDRepository) LockSegment(ctx context.Context, id segmentport.SegmentID) (segmentport.Segment, error) {
	queries, err := crudQueries(ctx)
	if err != nil || repository == nil || id <= 0 {
		return segmentport.Segment{}, crudStoreError(err)
	}
	row, err := queries.LockSegmentForUpdate(ctx, int64(id))
	if err != nil {
		return segmentport.Segment{}, crudStoreError(err)
	}
	return crudStoredLifecycleSegment(row.ID, row.Name, row.Definition, row.RefreshMode, row.RefreshCron, row.MemberCount,
		row.RefreshedAt, row.RefreshStatus, row.CreatedAt, row.UpdatedAt, row.LifecycleStatus)
}

func (repository *CRUDRepository) CreateSegment(ctx context.Context, command segmentport.CreateCommand, now time.Time) (segmentport.Segment, error) {
	queries, err := crudQueries(ctx)
	if err != nil || repository == nil || now.IsZero() {
		return segmentport.Segment{}, crudStoreError(err)
	}
	row, err := queries.CreateSegment(ctx, segmentdb.CreateSegmentParams{
		Name: command.Name, Definition: append([]byte(nil), command.Definition...), RefreshMode: string(command.RefreshMode),
		RefreshCron: crudOptionalText(command.RefreshCron), CreatedAt: timestamp(now),
	})
	if err != nil {
		return segmentport.Segment{}, crudStoreError(err)
	}
	return crudSegment(row.ID, row.Name, row.Definition, row.RefreshMode, row.RefreshCron, row.MemberCount, row.RefreshedAt, row.RefreshStatus, row.CreatedAt, row.UpdatedAt), nil
}

func (repository *CRUDRepository) UpdateSegment(ctx context.Context, segment segmentport.Segment, now time.Time) (segmentport.Segment, error) {
	queries, err := crudQueries(ctx)
	if err != nil || repository == nil || segment.ID <= 0 || now.IsZero() {
		return segmentport.Segment{}, crudStoreError(err)
	}
	row, err := queries.UpdateSegment(ctx, segmentdb.UpdateSegmentParams{
		Name: segment.Name, Definition: append([]byte(nil), segment.Definition...), RefreshMode: string(segment.RefreshMode),
		RefreshCron: crudOptionalText(segment.RefreshCron), UpdatedAt: timestamp(now), SegmentID: int64(segment.ID),
	})
	if err != nil {
		return segmentport.Segment{}, crudStoreError(err)
	}
	return crudSegment(row.ID, row.Name, row.Definition, row.RefreshMode, row.RefreshCron, row.MemberCount, row.RefreshedAt, row.RefreshStatus, row.CreatedAt, row.UpdatedAt), nil
}

func (repository *CRUDRepository) ArchiveSegment(ctx context.Context, id segmentport.SegmentID, actor segmentport.Actor, now time.Time) (segmentport.Segment, error) {
	if repository == nil || id <= 0 || actor == "" || now.IsZero() {
		return segmentport.Segment{}, segmentapp.ErrSegmentCRUDUnavailable
	}
	queries, err := crudQueries(ctx)
	if err != nil {
		return segmentport.Segment{}, err
	}
	row, err := queries.ArchiveSegment(ctx, segmentdb.ArchiveSegmentParams{
		ArchivedAt: timestamp(now.UTC()), ArchivedBy: string(actor), SegmentID: int64(id),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return segmentport.Segment{}, segmentapp.ErrSegmentNotFound
	}
	if err != nil {
		return segmentport.Segment{}, err
	}
	return crudStoredLifecycleSegment(row.ID, row.Name, row.Definition, row.RefreshMode, row.RefreshCron, row.MemberCount,
		row.RefreshedAt, row.RefreshStatus, row.CreatedAt, row.UpdatedAt, row.LifecycleStatus)
}

func (repository *CRUDRepository) ListMemberRecords(ctx context.Context, segmentID segmentport.SegmentID, after *segmentport.CustomerID, limit int32) ([]segmentapp.MemberRecord, error) {
	queries, err := crudQueries(ctx)
	if err != nil || repository == nil || segmentID <= 0 || limit < 1 {
		return nil, crudStoreError(err)
	}
	rows, err := queries.ListSegmentMemberRecords(ctx, segmentdb.ListSegmentMemberRecordsParams{
		SegmentID: int64(segmentID), AfterCustomerID: crudOptionalCustomerID(after), RowLimit: limit,
	})
	if err != nil {
		return nil, crudStoreError(err)
	}
	result := make([]segmentapp.MemberRecord, len(rows))
	for index, row := range rows {
		result[index] = segmentapp.MemberRecord{
			ID: segmentport.CustomerID(row.ID), Name: row.Name, AvatarURL: crudText(row.AvatarUrl), Gender: crudInt16(row.Gender),
			StageID: crudInt64(row.StageID), OwnerStaffID: crudInt64(row.OwnerStaffID), ChannelID: crudInt64(row.ChannelID),
			AddedAt: crudTime(row.AddedAt), LastInteractAt: crudTime(row.LastInteractAt), IsDeleted: row.IsDeleted,
			Extra: append([]byte(nil), row.Extra...), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		}
	}
	return result, nil
}

func (repository *CRUDRepository) ReserveReceipt(ctx context.Context, reservation segmentapp.ReceiptReservation) (segmentapp.Receipt, bool, error) {
	queries, err := crudQueries(ctx)
	if err != nil || repository == nil {
		return segmentapp.Receipt{}, false, crudStoreError(err)
	}
	params := segmentdb.ReserveSegmentOperationReceiptParams{
		Operation: string(reservation.Operation), ActorScope: reservation.ActorScope,
		KeyDigest: append([]byte(nil), reservation.KeyDigest[:]...), PayloadDigest: append([]byte(nil), reservation.PayloadDigest[:]...),
		CreatedAt: timestamp(reservation.CreatedAt),
	}
	row, err := queries.ReserveSegmentOperationReceipt(ctx, params)
	if err == nil {
		receipt, mapErr := crudReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSegmentID)
		return receipt, true, mapErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return segmentapp.Receipt{}, false, crudStoreError(err)
	}
	existing, err := queries.GetSegmentOperationReceipt(ctx, segmentdb.GetSegmentOperationReceiptParams{
		Operation: params.Operation, ActorScope: params.ActorScope, KeyDigest: params.KeyDigest,
	})
	if err != nil {
		return segmentapp.Receipt{}, false, crudStoreError(err)
	}
	receipt, err := crudReceipt(existing.ID, existing.Operation, existing.ActorScope, existing.KeyDigest, existing.PayloadDigest, existing.State, existing.ResultSegmentID)
	return receipt, false, err
}

func (repository *CRUDRepository) CompleteReceipt(ctx context.Context, id int64, segmentID segmentport.SegmentID, now time.Time) (segmentapp.Receipt, error) {
	queries, err := crudQueries(ctx)
	if err != nil || repository == nil || id <= 0 || segmentID <= 0 || now.IsZero() {
		return segmentapp.Receipt{}, crudStoreError(err)
	}
	row, err := queries.CompleteSegmentOperationReceipt(ctx, segmentdb.CompleteSegmentOperationReceiptParams{
		ResultSegmentID: int64(segmentID), CompletedAt: timestamp(now), ID: id,
	})
	if err != nil {
		return segmentapp.Receipt{}, crudStoreError(err)
	}
	return crudReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSegmentID)
}

func crudQueries(ctx context.Context) (*segmentdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return segmentdb.New(tx), nil
}

func crudStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return segmentapp.ErrSegmentNotFound
	}
	if err != nil {
		return err
	}
	return segmentapp.ErrSegmentCRUDUnavailable
}

func crudSegment(id int64, name string, definition []byte, refreshMode string, refreshCron pgtype.Text, memberCount int64, refreshedAt pgtype.Timestamptz, refreshStatus string, createdAt, updatedAt pgtype.Timestamptz) segmentport.Segment {
	return crudLifecycleSegment(id, name, definition, refreshMode, refreshCron, memberCount, refreshedAt, refreshStatus, createdAt, updatedAt, "active")
}

func crudLifecycleSegment(id int64, name string, definition []byte, refreshMode string, refreshCron pgtype.Text, memberCount int64, refreshedAt pgtype.Timestamptz, refreshStatus string, createdAt, updatedAt pgtype.Timestamptz, lifecycle string) segmentport.Segment {
	return segmentport.Segment{
		ID: segmentport.SegmentID(id), Name: name, Definition: append([]byte(nil), definition...), RefreshMode: segmentport.RefreshMode(refreshMode),
		RefreshCron: crudText(refreshCron), MemberCount: memberCount, RefreshedAt: crudTime(refreshedAt), RefreshStatus: segmentport.RefreshStatus(refreshStatus),
		LifecycleStatus: segmentport.LifecycleStatus(lifecycle), CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time,
	}
}

func crudStoredLifecycleSegment(id int64, name string, definition []byte, refreshMode string, refreshCron pgtype.Text, memberCount int64, refreshedAt pgtype.Timestamptz, refreshStatus string, createdAt, updatedAt pgtype.Timestamptz, lifecycle string) (segmentport.Segment, error) {
	if lifecycle != "active" && lifecycle != "archived" {
		return segmentport.Segment{}, segmentapp.ErrSegmentCRUDUnavailable
	}
	return crudLifecycleSegment(id, name, definition, refreshMode, refreshCron, memberCount, refreshedAt, refreshStatus, createdAt, updatedAt, lifecycle), nil
}

func crudReceipt(id int64, operation, actor string, key, payload []byte, state string, result pgtype.Int8) (segmentapp.Receipt, error) {
	if len(key) != 32 || len(payload) != 32 {
		return segmentapp.Receipt{}, segmentapp.ErrSegmentCRUDUnavailable
	}
	receipt := segmentapp.Receipt{ID: id, Operation: segmentapp.Operation(operation), ActorScope: actor, State: state}
	copy(receipt.KeyDigest[:], key)
	copy(receipt.PayloadDigest[:], payload)
	if result.Valid {
		receipt.ResultSegmentID = segmentport.SegmentID(result.Int64)
	}
	return receipt, nil
}

func crudOptionalID(value *segmentport.SegmentID) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: int64(*value), Valid: true}
}

func crudOptionalCustomerID(value *segmentport.CustomerID) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: int64(*value), Valid: true}
}

func crudOptionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func crudText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func crudInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func crudInt16(value pgtype.Int2) *int16 {
	if !value.Valid {
		return nil
	}
	copy := value.Int16
	return &copy
}

func crudTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}
