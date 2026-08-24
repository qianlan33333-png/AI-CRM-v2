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
		receipt, convertErr := internalEventSafeExportReceipt(row.ID, row.PayloadDigest, row.ResultDigest, row.ResultSnapshot, row.Completed)
		return receipt, true, convertErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return eventapp.InternalEventSafeExportReceipt{}, false, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	existing, err := q.GetInternalEventSafeExportReceipt(ctx, eventdb.GetInternalEventSafeExportReceiptParams{ActorID: actor, KeyDigest: key[:]})
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, false, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	receipt, err := internalEventSafeExportReceipt(existing.ID, existing.PayloadDigest, existing.ResultDigest, existing.ResultSnapshot, existing.Completed)
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, false, err
	}
	if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payload[:]) != 1 {
		return eventapp.InternalEventSafeExportReceipt{}, false, eventapp.ErrInternalEventSafeExportConflict
	}
	return receipt, false, nil
}

func (r *InternalEventSafeExportRepository) ReadInternalEventSafeExportSourceSnapshot(ctx context.Context, filter eventapp.InternalEventSafeExportFilter, limit int) (eventapp.InternalEventSafeExportSourceSnapshot, error) {
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return eventapp.InternalEventSafeExportSourceSnapshot{}, err
	}
	if limit < 1 || limit > eventapp.InternalEventSafeExportMaximumRows+1 {
		return eventapp.InternalEventSafeExportSourceSnapshot{}, eventapp.ErrInternalEventSafeExportUnavailable
	}
	source, err := q.ListInternalEventSafeExportSourceSnapshot(ctx, eventdb.ListInternalEventSafeExportSourceSnapshotParams{EventType: filter.EventType, Consumer: filter.Consumer, Status: filter.Status, RowLimit: int32(limit)})
	if err != nil || len(source) == 0 || !source[0].Watermark.Valid {
		return eventapp.InternalEventSafeExportSourceSnapshot{}, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	result := eventapp.InternalEventSafeExportSourceSnapshot{Watermark: source[0].Watermark.Time.UTC(), UpperEventID: source[0].UpperEventID, Rows: make([]eventapp.InternalEventSafeExportRow, 0, len(source))}
	for _, row := range source {
		if !row.Watermark.Valid || !row.Watermark.Time.Equal(source[0].Watermark.Time) || row.UpperEventID != result.UpperEventID {
			return eventapp.InternalEventSafeExportSourceSnapshot{}, eventapp.ErrInternalEventSafeExportUnavailable
		}
		if !row.ID.Valid {
			if len(source) != 1 || row.EventType.Valid || row.OccurredAt.Valid || row.Dispatched.Valid || row.Consumer.Valid || row.Status.Valid || row.AttemptCount.Valid || row.CompletedAt.Valid {
				return eventapp.InternalEventSafeExportSourceSnapshot{}, eventapp.ErrInternalEventSafeExportUnavailable
			}
			continue
		}
		if !row.EventType.Valid || !row.OccurredAt.Valid || !row.Dispatched.Valid {
			return eventapp.InternalEventSafeExportSourceSnapshot{}, eventapp.ErrInternalEventSafeExportUnavailable
		}
		result.Rows = append(result.Rows, eventapp.InternalEventSafeExportRow{EventID: eventport.EventID(row.ID.Int64), EventType: row.EventType.String, OccurredAt: row.OccurredAt.Time.UTC(), Dispatched: row.Dispatched.Bool, Consumer: row.Consumer.String, Status: row.Status.String, AttemptCount: int32Ptr(row.AttemptCount), CompletedAt: timePtr(row.CompletedAt)})
	}
	return result, nil
}

func (r *InternalEventSafeExportRepository) CreateInternalEventSafeExport(ctx context.Context, export eventapp.InternalEventSafeExport, actor int64, filterDigest [32]byte, upperEventID int64, rowsDigest, resultDigest [32]byte, rows []eventapp.InternalEventSafeExportRow) error {
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return err
	}
	if err = q.InsertInternalEventSafeExport(ctx, eventdb.InsertInternalEventSafeExportParams{ID: export.ID, ActorID: actor, FilterDigest: filterDigest[:], DigestVersion: eventapp.InternalEventSafeExportDigestVersion, RowsDigest: rowsDigest[:], ResultDigest: resultDigest[:], Watermark: eventTimestamp(export.Watermark), UpperEventID: upperEventID, RecordCount: int32(len(rows)), CreatedAt: eventTimestamp(export.CreatedAt)}); err != nil {
		return errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	for i, row := range rows {
		if err = q.InsertInternalEventSafeExportRow(ctx, eventdb.InsertInternalEventSafeExportRowParams{ExportID: export.ID, RowIndex: int32(i + 1), EventID: int64(row.EventID), EventType: row.EventType, OccurredAt: eventTimestamp(row.OccurredAt), Dispatched: row.Dispatched, Consumer: textValue(row.Consumer), Status: textValue(row.Status), AttemptCount: int32Value(row.AttemptCount), CompletedAt: timeValue(row.CompletedAt)}); err != nil {
			return errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
		}
	}
	return nil
}

func (r *InternalEventSafeExportRepository) CompleteInternalEventSafeExportReceipt(ctx context.Context, id int64, exportID string, resultDigest [32]byte, snapshot json.RawMessage, completed time.Time) (eventapp.InternalEventSafeExportReceipt, error) {
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, err
	}
	row, err := q.CompleteInternalEventSafeExportReceipt(ctx, eventdb.CompleteInternalEventSafeExportReceiptParams{ID: id, ExportID: exportID, ResultDigest: resultDigest[:], ResultSnapshot: snapshot, CompletedAt: eventTimestamp(completed)})
	if errors.Is(err, pgx.ErrNoRows) {
		return eventapp.InternalEventSafeExportReceipt{}, eventapp.ErrInternalEventSafeExportConflict
	}
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	return internalEventSafeExportReceipt(row.ID, row.PayloadDigest, row.ResultDigest, row.ResultSnapshot, row.Completed)
}

func (r *InternalEventSafeExportRepository) ReadInternalEventSafeExportSnapshot(ctx context.Context, id string, actor int64) (eventapp.InternalEventSafeExportStoredSnapshot, error) {
	q, err := internalEventSafeExportQueries(ctx, r)
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, err
	}
	header, err := q.GetInternalEventSafeExport(ctx, eventdb.GetInternalEventSafeExportParams{ID: id, ActorID: actor})
	if errors.Is(err, pgx.ErrNoRows) {
		receiptExists, existsErr := q.InternalEventSafeExportReceiptExists(ctx, eventdb.InternalEventSafeExportReceiptExistsParams{ExportID: id, ActorID: actor})
		if existsErr != nil || receiptExists {
			return eventapp.InternalEventSafeExportStoredSnapshot{}, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, existsErr)
		}
		return eventapp.InternalEventSafeExportStoredSnapshot{}, eventapp.ErrInternalEventSafeExportNotFound
	}
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	filterDigest, err := internalEventSafeExportDigest(header.FilterDigest)
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, err
	}
	rowsDigest, err := internalEventSafeExportDigest(header.RowsDigest)
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, err
	}
	resultDigest, err := internalEventSafeExportDigest(header.ResultDigest)
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, err
	}
	rows, err := q.ListInternalEventSafeExportRows(ctx, id)
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	integrity, err := q.GetInternalEventSafeExportIntegrity(ctx, id)
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, err)
	}
	receiptPayloadDigest, err := internalEventSafeExportDigest(integrity.PayloadDigest)
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, err
	}
	receiptResultDigest, err := internalEventSafeExportDigest(integrity.ReceiptResultDigest)
	if err != nil {
		return eventapp.InternalEventSafeExportStoredSnapshot{}, err
	}
	result := eventapp.InternalEventSafeExportStoredSnapshot{Export: eventapp.InternalEventSafeExport{ID: header.ID, RecordCount: int(header.RecordCount), Watermark: header.Watermark.Time.UTC(), CreatedAt: header.CreatedAt.Time.UTC()}, ActorID: header.ActorID, FilterDigest: filterDigest, UpperEventID: header.UpperEventID, DigestVersion: header.DigestVersion, RowsDigest: rowsDigest, ResultDigest: resultDigest, Rows: make([]eventapp.InternalEventSafeExportRow, 0, len(rows)), ReceiptID: integrity.ReceiptID, ReceiptPayloadDigest: receiptPayloadDigest, ReceiptResultDigest: receiptResultDigest, ReceiptResultSnapshot: append(json.RawMessage(nil), integrity.ResultSnapshot...), AuditEventType: integrity.AuditEventType, AuditIdempotencyKey: integrity.AuditIdempotencyKey, AuditOccurredAt: integrity.AuditOccurredAt.Time.UTC(), AuditPayload: append(json.RawMessage(nil), integrity.AuditPayload...)}
	for _, row := range rows {
		result.Rows = append(result.Rows, eventapp.InternalEventSafeExportRow{EventID: eventport.EventID(row.EventID), EventType: row.EventType, OccurredAt: row.OccurredAt.Time.UTC(), Dispatched: row.Dispatched, Consumer: row.Consumer.String, Status: row.Status.String, AttemptCount: int32Ptr(row.AttemptCount), CompletedAt: timePtr(row.CompletedAt)})
	}
	return result, nil
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

func internalEventSafeExportReceipt(id int64, payload, result, snapshot []byte, completed bool) (eventapp.InternalEventSafeExportReceipt, error) {
	payloadDigest, err := internalEventSafeExportDigest(payload)
	if err != nil {
		return eventapp.InternalEventSafeExportReceipt{}, err
	}
	var resultDigest [32]byte
	if completed {
		resultDigest, err = internalEventSafeExportDigest(result)
		if err != nil {
			return eventapp.InternalEventSafeExportReceipt{}, err
		}
	} else if len(result) != 0 || len(snapshot) != 0 {
		return eventapp.InternalEventSafeExportReceipt{}, eventapp.ErrInternalEventSafeExportUnavailable
	}
	return eventapp.InternalEventSafeExportReceipt{ID: id, PayloadDigest: payloadDigest, ResultDigest: resultDigest, ResultSnapshot: append(json.RawMessage(nil), snapshot...), Completed: completed}, nil
}

func internalEventSafeExportDigest(value []byte) ([32]byte, error) {
	if len(value) != 32 {
		return [32]byte{}, eventapp.ErrInternalEventSafeExportUnavailable
	}
	var digest [32]byte
	copy(digest[:], value)
	return digest, nil
}

func eventTimestamp(v time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: v.UTC(), Valid: true}
}
func textValue(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }
func int32Value(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}
func timeValue(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return eventTimestamp(*v)
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
