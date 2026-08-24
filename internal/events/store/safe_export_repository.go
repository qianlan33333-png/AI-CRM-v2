package store

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type InternalEventSafeExportRepository struct{}

var _ eventapp.InternalEventSafeExportStore = (*InternalEventSafeExportRepository)(nil)

func NewInternalEventSafeExportRepository() *InternalEventSafeExportRepository {
	return &InternalEventSafeExportRepository{}
}

func (r *InternalEventSafeExportRepository) ReserveInternalEventSafeExportReceipt(ctx context.Context, actor int64, key, payload [32]byte, created time.Time) (eventapp.InternalEventSafeExportReceipt, bool, error) {
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, false, err
	}
	row, err := q.ReserveInternalEventSafeExportReceipt(ctx, eventdb.ReserveInternalEventSafeExportReceiptParams{ActorID: actor, KeyDigest: key[:], PayloadDigest: payload[:], CreatedAt: eventTimestamp(created)})
	if err == nil {
		return internalEventSafeExportReceipt(row.ID, row.PayloadDigest, row.ResultSnapshot, row.Completed), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return eventapp.InternalEventSafeExportReceipt{}, false, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	existing, err := q.GetInternalEventSafeExportReceipt(ctx, eventdb.GetInternalEventSafeExportReceiptParams{ActorID: actor, KeyDigest: key[:]})
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, false, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	receipt := internalEventSafeExportReceipt(existing.ID, existing.PayloadDigest, existing.ResultSnapshot, existing.Completed)
	if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payload[:]) != 1 {
		return eventapp.InternalEventSafeExportReceipt{}, false, eventapp.ErrInternalEventSafeExportConflict
	}
	return receipt, false, nil
}
func (r *InternalEventSafeExportRepository) CreateInternalEventSafeExport(ctx context.Context, export eventapp.InternalEventSafeExport, actor int64, digest [32]byte, filter eventapp.InternalEventSafeExportFilter) ([]eventapp.InternalEventSafeExportRow, error) {
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return nil, err
	}
	upperEventID, err := q.GetInternalEventSafeExportUpperEventID(ctx, eventTimestamp(export.Watermark))
	if err != nil {
		return nil, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	source, err := q.ListInternalEventSafeExportSourceRows(ctx, eventdb.ListInternalEventSafeExportSourceRowsParams{Watermark: eventTimestamp(export.Watermark), UpperEventID: upperEventID, EventType: filter.EventType, Consumer: filter.Consumer, Status: filter.Status, RowLimit: eventapp.InternalEventSafeExportMaximumRows + 1})
	if err != nil {
		return nil, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	if len(source) > eventapp.InternalEventSafeExportMaximumRows {
		return nil, eventapp.ErrInternalEventSafeExportConflict
	}
	if err = q.InsertInternalEventSafeExport(ctx, eventdb.InsertInternalEventSafeExportParams{ID: export.ID, ActorID: actor, FilterDigest: digest[:], Watermark: eventTimestamp(export.Watermark), UpperEventID: upperEventID, RecordCount: int32(len(source)), CreatedAt: eventTimestamp(export.CreatedAt)}); err != nil {
		return nil, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	out := make([]eventapp.InternalEventSafeExportRow, 0, len(source))
	for i, row := range source {
		value := eventapp.InternalEventSafeExportRow{EventID: eventport.EventID(row.ID), EventType: row.EventType, OccurredAt: row.OccurredAt.Time.UTC(), Dispatched: row.Dispatched, Consumer: row.Consumer.String, Status: row.Status.String, AttemptCount: int32Ptr(row.AttemptCount), CompletedAt: timePtr(row.CompletedAt)}
		if err = q.InsertInternalEventSafeExportRow(ctx, eventdb.InsertInternalEventSafeExportRowParams{ExportID: export.ID, RowIndex: int32(i + 1), EventID: row.ID, EventType: row.EventType, OccurredAt: row.OccurredAt, Dispatched: row.Dispatched, Consumer: row.Consumer, Status: row.Status, AttemptCount: row.AttemptCount, CompletedAt: row.CompletedAt}); err != nil {
			return nil, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
		}
		out = append(out, value)
	}
	return out, nil
}
func (r *InternalEventSafeExportRepository) CompleteInternalEventSafeExportReceipt(ctx context.Context, id int64, exportID string, snapshot json.RawMessage, completed time.Time) (eventapp.InternalEventSafeExportReceipt, error) {
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, err
	}
	row, err := q.CompleteInternalEventSafeExportReceipt(ctx, eventdb.CompleteInternalEventSafeExportReceiptParams{ID: id, ExportID: exportID, ResultSnapshot: snapshot, CompletedAt: eventTimestamp(completed)})
	if errors.Is(err, pgx.ErrNoRows) {
		return eventapp.InternalEventSafeExportReceipt{}, eventapp.ErrInternalEventSafeExportConflict
	}
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	return internalEventSafeExportReceipt(row.ID, row.PayloadDigest, row.ResultSnapshot, row.Completed), nil
}
func (r *InternalEventSafeExportRepository) ReadInternalEventSafeExport(ctx context.Context, id string, actor int64) (eventapp.InternalEventSafeExport, error) {
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return eventapp.InternalEventSafeExport{}, err
	}
	row, err := q.GetInternalEventSafeExport(ctx, eventdb.GetInternalEventSafeExportParams{ID: id, ActorID: actor})
	if errors.Is(err, pgx.ErrNoRows) {
		return eventapp.InternalEventSafeExport{}, eventapp.ErrInternalEventSafeExportNotFound
	}
	if err != nil {
		return eventapp.InternalEventSafeExport{}, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	return eventapp.InternalEventSafeExport{ID: row.ID, RecordCount: int(row.RecordCount), Watermark: row.Watermark.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC()}, nil
}
func (r *InternalEventSafeExportRepository) ReadInternalEventSafeExportRows(ctx context.Context, id string, actor int64) ([]eventapp.InternalEventSafeExportRow, error) {
	if _, err := r.ReadInternalEventSafeExport(ctx, id, actor); err != nil {
		return nil, err
	}
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInternalEventSafeExportRows(ctx, id)
	if err != nil {
		return nil, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	out := make([]eventapp.InternalEventSafeExportRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, eventapp.InternalEventSafeExportRow{EventID: eventport.EventID(row.EventID), EventType: row.EventType, OccurredAt: row.OccurredAt.Time.UTC(), Dispatched: row.Dispatched, Consumer: row.Consumer.String, Status: row.Status.String, AttemptCount: int32Ptr(row.AttemptCount), CompletedAt: timePtr(row.CompletedAt)})
	}
	return out, nil
}
func internalEventSafeExportQueries(ctx context.Context, r *InternalEventSafeExportRepository) (*eventdb.Queries, error) {
	if r == nil {
		return nil, eventapp.ErrInternalEventSafeExportUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	return eventdb.New(tx), nil
}
func internalEventSafeExportReceipt(id int64, payload, snapshot []byte, completed bool) eventapp.InternalEventSafeExportReceipt {
	var d [32]byte
	copy(d[:], payload)
	return eventapp.InternalEventSafeExportReceipt{ID: id, PayloadDigest: d, ResultSnapshot: append(json.RawMessage(nil), snapshot...), Completed: completed}
}
func eventTimestamp(v time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: v.UTC(), Valid: true}
}
func int32Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	x := v.Int32
	return &x
}
func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	x := v.Time.UTC()
	return &x
}
