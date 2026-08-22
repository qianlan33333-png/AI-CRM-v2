// Package store implements the Radar local PostgreSQL persistence contract.
// Every method requires the transaction-bound context supplied by the platform
// UnitOfWork; this repository never begins or commits a transaction itself.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

type txResolver func(context.Context) (pgx.Tx, error)

type PostgresRepository struct {
	tx txResolver
}

var _ radarport.Repository = (*PostgresRepository)(nil)

func NewPostgresRepository() *PostgresRepository {
	return &PostgresRepository{tx: platformstore.TxFromContext}
}

// NewPostgresRepositoryWithTxResolver is intended for isolated composition and
// tests. Production composition should normally use NewPostgresRepository.
func NewPostgresRepositoryWithTxResolver(resolver func(context.Context) (pgx.Tx, error)) (*PostgresRepository, error) {
	if resolver == nil {
		return nil, radarport.ErrUnavailable
	}
	return &PostgresRepository{tx: resolver}, nil
}

func (repository *PostgresRepository) List(ctx context.Context, input radarport.ListInput) ([]radarport.Link, int64, error) {
	if repository == nil || repository.tx == nil || !input.Status.Valid() || !input.Sort.Valid() || input.Limit < 1 || input.Limit > radarport.MaximumLimit || input.Offset < 0 || input.Offset > radarport.MaximumOffset {
		return nil, 0, radarport.ErrInvalidArgument
	}
	tx, err := repository.tx(ctx)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err = tx.QueryRow(ctx, `
SELECT COUNT(*)
FROM public.radar_links
WHERE ($1::text = 'all' OR status = $1::text)
`, string(input.Status)).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := listQuery(input.Sort)
	rows, err := tx.Query(ctx, query, string(input.Status), input.Limit, input.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]radarport.Link, 0, input.Limit)
	for rows.Next() {
		link, scanErr := scanLink(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, link)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (repository *PostgresRepository) Get(ctx context.Context, id radarport.LinkID) (radarport.Link, error) {
	return repository.get(ctx, id, false)
}

func (repository *PostgresRepository) GetForUpdate(ctx context.Context, id radarport.LinkID) (radarport.Link, error) {
	return repository.get(ctx, id, true)
}

func (repository *PostgresRepository) get(ctx context.Context, id radarport.LinkID, lock bool) (radarport.Link, error) {
	if repository == nil || repository.tx == nil || id < 1 {
		return radarport.Link{}, radarport.ErrNotFound
	}
	tx, err := repository.tx(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	query := selectLinkSQL
	if lock {
		query += " FOR UPDATE"
	}
	link, err := scanLink(tx.QueryRow(ctx, query, int64(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.Link{}, radarport.ErrNotFound
	}
	return link, err
}

func (repository *PostgresRepository) Create(ctx context.Context, record radarport.CreateRecord, now time.Time) (radarport.Link, error) {
	if repository == nil || repository.tx == nil || record.PublicCode == "" || record.Name == "" || record.Title == "" || record.DestinationURL == "" || record.Status != radarport.StatusDraft || record.ActorID < 1 || now.IsZero() {
		return radarport.Link{}, radarport.ErrInvalidArgument
	}
	tx, err := repository.tx(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	link, err := scanLink(tx.QueryRow(ctx, `
INSERT INTO public.radar_links (
    public_code, name, title, destination_url, cover_image_id, attachment_id,
    status, version, created_by, updated_by, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 1, $8, $8, $9, $9)
ON CONFLICT (public_code) DO NOTHING
RETURNING id, public_code, name, title, destination_url, cover_image_id,
          attachment_id, status, version, created_by, updated_by, created_at, updated_at
`, record.PublicCode, record.Name, record.Title, record.DestinationURL,
		nullableID(record.CoverImageID), nullableID(record.AttachmentID), string(record.Status), record.ActorID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.Link{}, radarport.ErrPublicCodeCollision
	}
	return link, err
}

func (repository *PostgresRepository) Update(ctx context.Context, record radarport.UpdateRecord, now time.Time) (radarport.Link, error) {
	if repository == nil || repository.tx == nil || record.LinkID < 1 || record.ExpectedVersion < 1 || record.Name == "" || record.Title == "" || record.DestinationURL == "" || record.ActorID < 1 || now.IsZero() {
		return radarport.Link{}, radarport.ErrInvalidArgument
	}
	tx, err := repository.tx(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	link, err := scanLink(tx.QueryRow(ctx, `
UPDATE public.radar_links
SET name = $3,
    title = $4,
    destination_url = $5,
    cover_image_id = $6,
    attachment_id = $7,
    version = version + 1,
    updated_by = $8,
    updated_at = $9
WHERE id = $1 AND version = $2
RETURNING id, public_code, name, title, destination_url, cover_image_id,
          attachment_id, status, version, created_by, updated_by, created_at, updated_at
`, int64(record.LinkID), record.ExpectedVersion, record.Name, record.Title,
		record.DestinationURL, nullableID(record.CoverImageID), nullableID(record.AttachmentID), record.ActorID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.Link{}, radarport.ErrConflict
	}
	return link, err
}

func (repository *PostgresRepository) SetStatus(ctx context.Context, record radarport.StatusRecord, now time.Time) (radarport.Link, error) {
	if repository == nil || repository.tx == nil || record.LinkID < 1 || record.ExpectedVersion < 1 || (record.Target != radarport.StatusEnabled && record.Target != radarport.StatusDisabled) || record.ActorID < 1 || now.IsZero() {
		return radarport.Link{}, radarport.ErrInvalidArgument
	}
	tx, err := repository.tx(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	link, err := scanLink(tx.QueryRow(ctx, `
UPDATE public.radar_links
SET status = $3,
    version = version + 1,
    updated_by = $4,
    updated_at = $5
WHERE id = $1
  AND version = $2
  AND (($3::text = 'enabled' AND status IN ('draft', 'disabled'))
       OR ($3::text = 'disabled' AND status = 'enabled'))
RETURNING id, public_code, name, title, destination_url, cover_image_id,
          attachment_id, status, version, created_by, updated_by, created_at, updated_at
`, int64(record.LinkID), record.ExpectedVersion, string(record.Target), record.ActorID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.Link{}, radarport.ErrConflict
	}
	return link, err
}

func (repository *PostgresRepository) ReserveIdempotency(ctx context.Context, reservation radarport.ReserveIdempotencyRecord) (radarport.IdempotencyRecord, bool, error) {
	if repository == nil || repository.tx == nil || reservation.ActorID < 1 || reservation.Operation == "" || reservation.CreatedAt.IsZero() {
		return radarport.IdempotencyRecord{}, false, radarport.ErrInvalidArgument
	}
	tx, err := repository.tx(ctx)
	if err != nil {
		return radarport.IdempotencyRecord{}, false, err
	}
	inserted, err := scanIdempotency(tx.QueryRow(ctx, `
INSERT INTO public.radar_link_idempotency_records (
    actor_id, key_digest, operation, payload_digest, state, created_at
) VALUES ($1, $2, $3, $4, 'reserved', $5)
ON CONFLICT (actor_id, key_digest) DO NOTHING
RETURNING id, actor_id, key_digest, operation, payload_digest, state,
          result_link_id, result_public_code, result_name, result_title,
          result_destination_url, result_cover_image_id, result_attachment_id,
          result_status, result_version, result_created_by, result_updated_by,
          result_created_at, result_updated_at, created_at, completed_at
`, reservation.ActorID, reservation.KeyDigest[:], reservation.Operation, reservation.PayloadDigest[:], reservation.CreatedAt))
	if err == nil {
		return inserted, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return radarport.IdempotencyRecord{}, false, err
	}
	existing, err := scanIdempotency(tx.QueryRow(ctx, `
SELECT id, actor_id, key_digest, operation, payload_digest, state,
       result_link_id, result_public_code, result_name, result_title,
       result_destination_url, result_cover_image_id, result_attachment_id,
       result_status, result_version, result_created_by, result_updated_by,
       result_created_at, result_updated_at, created_at, completed_at
FROM public.radar_link_idempotency_records
WHERE actor_id = $1 AND key_digest = $2
FOR UPDATE
`, reservation.ActorID, reservation.KeyDigest[:]))
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.IdempotencyRecord{}, false, radarport.ErrUnavailable
	}
	return existing, false, err
}

func (repository *PostgresRepository) CompleteIdempotency(ctx context.Context, recordID int64, result radarport.Link, now time.Time) (radarport.IdempotencyRecord, error) {
	if repository == nil || repository.tx == nil || recordID < 1 || result.LinkID < 1 || result.PublicCode == "" || result.Name == "" || result.Title == "" || result.DestinationURL == "" || !result.Status.Valid() || result.Version < 1 || result.CreatedBy < 1 || result.UpdatedBy < 1 || result.CreatedAt.IsZero() || result.UpdatedAt.IsZero() || now.IsZero() {
		return radarport.IdempotencyRecord{}, radarport.ErrInvalidArgument
	}
	tx, err := repository.tx(ctx)
	if err != nil {
		return radarport.IdempotencyRecord{}, err
	}
	completed, err := scanIdempotency(tx.QueryRow(ctx, `
UPDATE public.radar_link_idempotency_records
SET state = 'completed',
    result_link_id = $2,
    result_public_code = $3,
    result_name = $4,
    result_title = $5,
    result_destination_url = $6,
    result_cover_image_id = $7,
    result_attachment_id = $8,
    result_status = $9,
    result_version = $10,
    result_created_by = $11,
    result_updated_by = $12,
    result_created_at = $13,
    result_updated_at = $14,
    completed_at = $15
WHERE id = $1 AND state = 'reserved'
RETURNING id, actor_id, key_digest, operation, payload_digest, state,
          result_link_id, result_public_code, result_name, result_title,
          result_destination_url, result_cover_image_id, result_attachment_id,
          result_status, result_version, result_created_by, result_updated_by,
          result_created_at, result_updated_at, created_at, completed_at
`, recordID, int64(result.LinkID), result.PublicCode, result.Name, result.Title,
		result.DestinationURL, nullableID(result.CoverImageID), nullableID(result.AttachmentID),
		string(result.Status), result.Version, result.CreatedBy, result.UpdatedBy,
		result.CreatedAt, result.UpdatedAt, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.IdempotencyRecord{}, radarport.ErrIdempotencyStateInvalid
	}
	return completed, err
}

const selectLinkSQL = `
SELECT id, public_code, name, title, destination_url, cover_image_id,
       attachment_id, status, version, created_by, updated_by, created_at, updated_at
FROM public.radar_links
WHERE id = $1`

func listQuery(sort radarport.Sort) string {
	order := "updated_at DESC, id DESC"
	switch sort {
	case radarport.SortCreatedDesc:
		order = "created_at DESC, id DESC"
	case radarport.SortNameAsc:
		order = "name ASC, id ASC"
	}
	return `
SELECT id, public_code, name, title, destination_url, cover_image_id,
       attachment_id, status, version, created_by, updated_by, created_at, updated_at
FROM public.radar_links
WHERE ($1::text = 'all' OR status = $1::text)
ORDER BY ` + order + `
LIMIT $2 OFFSET $3`
}

type scanner interface {
	Scan(...any) error
}

func scanLink(row scanner) (radarport.Link, error) {
	if row == nil || nilInterface(row) {
		return radarport.Link{}, radarport.ErrUnavailable
	}
	var link radarport.Link
	var linkID int64
	var coverImageID sql.NullInt64
	var attachmentID sql.NullInt64
	var status string
	err := row.Scan(
		&linkID,
		&link.PublicCode,
		&link.Name,
		&link.Title,
		&link.DestinationURL,
		&coverImageID,
		&attachmentID,
		&status,
		&link.Version,
		&link.CreatedBy,
		&link.UpdatedBy,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if err != nil {
		return radarport.Link{}, err
	}
	link.LinkID = radarport.LinkID(linkID)
	link.Status = radarport.Status(status)
	if coverImageID.Valid {
		link.CoverImageID = pointer(coverImageID.Int64)
	}
	if attachmentID.Valid {
		link.AttachmentID = pointer(attachmentID.Int64)
	}
	return link, nil
}

func scanIdempotency(row scanner) (radarport.IdempotencyRecord, error) {
	if row == nil || nilInterface(row) {
		return radarport.IdempotencyRecord{}, radarport.ErrUnavailable
	}
	var record radarport.IdempotencyRecord
	var keyDigest []byte
	var payloadDigest []byte
	var state string
	var resultLinkID sql.NullInt64
	var resultPublicCode sql.NullString
	var resultName sql.NullString
	var resultTitle sql.NullString
	var resultDestinationURL sql.NullString
	var resultCoverImageID sql.NullInt64
	var resultAttachmentID sql.NullInt64
	var resultStatus sql.NullString
	var resultVersion sql.NullInt64
	var resultCreatedBy sql.NullInt64
	var resultUpdatedBy sql.NullInt64
	var resultCreatedAt sql.NullTime
	var resultUpdatedAt sql.NullTime
	var completedAt sql.NullTime
	if err := row.Scan(
		&record.RecordID,
		&record.ActorID,
		&keyDigest,
		&record.Operation,
		&payloadDigest,
		&state,
		&resultLinkID,
		&resultPublicCode,
		&resultName,
		&resultTitle,
		&resultDestinationURL,
		&resultCoverImageID,
		&resultAttachmentID,
		&resultStatus,
		&resultVersion,
		&resultCreatedBy,
		&resultUpdatedBy,
		&resultCreatedAt,
		&resultUpdatedAt,
		&record.CreatedAt,
		&completedAt,
	); err != nil {
		return radarport.IdempotencyRecord{}, err
	}
	if len(keyDigest) != sha256.Size || len(payloadDigest) != sha256.Size {
		return radarport.IdempotencyRecord{}, radarport.ErrUnavailable
	}
	copy(record.KeyDigest[:], keyDigest)
	copy(record.PayloadDigest[:], payloadDigest)
	record.State = radarport.IdempotencyState(state)
	if completedAt.Valid {
		completed := completedAt.Time
		record.CompletedAt = &completed
	}

	resultRequired := []bool{
		resultLinkID.Valid,
		resultPublicCode.Valid,
		resultName.Valid,
		resultTitle.Valid,
		resultDestinationURL.Valid,
		resultStatus.Valid,
		resultVersion.Valid,
		resultCreatedBy.Valid,
		resultUpdatedBy.Valid,
		resultCreatedAt.Valid,
		resultUpdatedAt.Valid,
	}
	allRequired := true
	anyResult := resultCoverImageID.Valid || resultAttachmentID.Valid
	for _, valid := range resultRequired {
		allRequired = allRequired && valid
		anyResult = anyResult || valid
	}
	switch record.State {
	case radarport.IdempotencyReserved:
		if anyResult || record.CompletedAt != nil {
			return radarport.IdempotencyRecord{}, radarport.ErrUnavailable
		}
	case radarport.IdempotencyCompleted:
		if !allRequired || record.CompletedAt == nil {
			return radarport.IdempotencyRecord{}, radarport.ErrUnavailable
		}
		result := radarport.Link{
			LinkID:         radarport.LinkID(resultLinkID.Int64),
			PublicCode:     resultPublicCode.String,
			Name:           resultName.String,
			Title:          resultTitle.String,
			DestinationURL: resultDestinationURL.String,
			Status:         radarport.Status(resultStatus.String),
			Version:        resultVersion.Int64,
			CreatedBy:      resultCreatedBy.Int64,
			UpdatedBy:      resultUpdatedBy.Int64,
			CreatedAt:      resultCreatedAt.Time,
			UpdatedAt:      resultUpdatedAt.Time,
		}
		if resultCoverImageID.Valid {
			result.CoverImageID = pointer(resultCoverImageID.Int64)
		}
		if resultAttachmentID.Valid {
			result.AttachmentID = pointer(resultAttachmentID.Int64)
		}
		record.Result = &result
	default:
		return radarport.IdempotencyRecord{}, radarport.ErrUnavailable
	}
	return record, nil
}

func nullableID(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func pointer(value int64) *int64 {
	copyValue := value
	return &copyValue
}

func nilInterface(value any) bool {
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
