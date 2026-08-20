package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type LegacyTagCatalogRepository struct{}

func NewLegacyTagCatalogRepository() *LegacyTagCatalogRepository {
	return &LegacyTagCatalogRepository{}
}
func legacyTagQueries(ctx context.Context) (*contactdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return contactdb.New(tx), nil
}
func (*LegacyTagCatalogRepository) ListLegacyTagGroups(ctx context.Context) ([]contactapp.LegacyTagGroup, error) {
	q, e := legacyTagQueries(ctx)
	if e != nil {
		return nil, e
	}
	rows, e := q.ListLegacyTagGroups(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]contactapp.LegacyTagGroup, 0, len(rows))
	for _, r := range rows {
		out = append(out, contactapp.LegacyTagGroup{ID: r.ID, Name: r.Name, SortOrder: r.SortOrder})
	}
	return out, nil
}
func (*LegacyTagCatalogRepository) ListLegacyTags(ctx context.Context) ([]contactapp.LegacyTag, error) {
	q, e := legacyTagQueries(ctx)
	if e != nil {
		return nil, e
	}
	rows, e := q.ListLegacyTags(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]contactapp.LegacyTag, 0, len(rows))
	for _, r := range rows {
		if !r.GroupID.Valid || r.GroupName == "" {
			return nil, contactapp.ErrLegacyTagUnavailable
		}
		out = append(out, contactapp.LegacyTag{ID: r.ID, GroupID: r.GroupID.Int64, GroupName: r.GroupName, Name: r.Name, SortOrder: r.SortOrder})
	}
	return out, nil
}
func (*LegacyTagCatalogRepository) CreateLegacyTagGroup(ctx context.Context, name string) (contactapp.LegacyTagGroup, error) {
	q, e := legacyTagQueries(ctx)
	if e != nil {
		return contactapp.LegacyTagGroup{}, e
	}
	r, e := q.CreateLegacyTagGroup(ctx, name)
	return contactapp.LegacyTagGroup{ID: r.ID, Name: r.Name, SortOrder: r.SortOrder}, e
}
func (r *LegacyTagCatalogRepository) CreateLegacyTag(ctx context.Context, g int64, name string) (contactapp.LegacyTag, error) {
	q, e := legacyTagQueries(ctx)
	if e != nil {
		return contactapp.LegacyTag{}, e
	}
	row, e := q.CreateLegacyTag(ctx, contactdb.CreateLegacyTagParams{GroupID: g, Name: name})
	if e != nil {
		return contactapp.LegacyTag{}, classifyLegacyTagStore(e)
	}
	groups, e := r.ListLegacyTagGroups(ctx)
	if e != nil {
		return contactapp.LegacyTag{}, e
	}
	for _, x := range groups {
		if x.ID == g {
			return contactapp.LegacyTag{ID: row.ID, GroupID: g, GroupName: x.Name, Name: row.Name, SortOrder: row.SortOrder}, nil
		}
	}
	return contactapp.LegacyTag{}, contactapp.ErrLegacyTagNotFound
}
func (*LegacyTagCatalogRepository) UpdateLegacyTagGroup(ctx context.Context, id int64, name string) (contactapp.LegacyTagGroup, error) {
	q, e := legacyTagQueries(ctx)
	if e != nil {
		return contactapp.LegacyTagGroup{}, e
	}
	r, e := q.UpdateLegacyTagGroup(ctx, contactdb.UpdateLegacyTagGroupParams{Name: name, ID: id})
	return contactapp.LegacyTagGroup{ID: r.ID, Name: r.Name, SortOrder: r.SortOrder}, classifyLegacyTagStore(e)
}
func (*LegacyTagCatalogRepository) ArchiveLegacyTagGroup(ctx context.Context, id int64) (contactapp.LegacyTagGroup, error) {
	q, e := legacyTagQueries(ctx)
	if e != nil {
		return contactapp.LegacyTagGroup{}, e
	}
	r, e := q.ArchiveLegacyTagGroup(ctx, id)
	return contactapp.LegacyTagGroup{ID: r.ID, Name: r.Name, SortOrder: r.SortOrder}, classifyLegacyTagStore(e)
}
func (r *LegacyTagCatalogRepository) UpdateLegacyTag(ctx context.Context, id int64, name string) (contactapp.LegacyTag, error) {
	q, e := legacyTagQueries(ctx)
	if e != nil {
		return contactapp.LegacyTag{}, e
	}
	row, e := q.UpdateLegacyTag(ctx, contactdb.UpdateLegacyTagParams{Name: name, ID: id})
	if e != nil {
		return contactapp.LegacyTag{}, classifyLegacyTagStore(e)
	}
	return r.inflate(ctx, row.ID, row.GroupID, row.Name, row.SortOrder)
}
func (r *LegacyTagCatalogRepository) ArchiveLegacyTag(ctx context.Context, id int64) (contactapp.LegacyTag, error) {
	q, e := legacyTagQueries(ctx)
	if e != nil {
		return contactapp.LegacyTag{}, e
	}
	row, e := q.ArchiveLegacyTag(ctx, id)
	if e != nil {
		return contactapp.LegacyTag{}, classifyLegacyTagStore(e)
	}
	return r.inflate(ctx, row.ID, row.GroupID, row.Name, row.SortOrder)
}
func (*LegacyTagCatalogRepository) CountLegacyTagReferences(ctx context.Context, id int64) (int64, error) {
	if id <= 0 {
		return 0, contactapp.ErrLegacyTagNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	err = tx.QueryRow(ctx, `SELECT count(*) FROM customer_tags WHERE tag_id = $1`, id).Scan(&count)
	return count, err
}

func (*LegacyTagCatalogRepository) CountLegacyTagGroupReferences(ctx context.Context, id int64) (int64, error) {
	if id <= 0 {
		return 0, contactapp.ErrLegacyTagNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM customer_tags AS ct
		JOIN tags AS t ON t.id = ct.tag_id
		WHERE t.group_id = $1`, id).Scan(&count)
	return count, err
}

func (*LegacyTagCatalogRepository) GetLegacyTagGroup(ctx context.Context, id int64) (contactapp.LegacyTagGroup, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.LegacyTagGroup{}, err
	}
	var out contactapp.LegacyTagGroup
	err = tx.QueryRow(ctx, `SELECT id, name, sort_order FROM public.tag_groups WHERE id = $1`, id).Scan(&out.ID, &out.Name, &out.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.LegacyTagGroup{}, contactapp.ErrLegacyTagNotFound
	}
	return out, err
}

func (*LegacyTagCatalogRepository) GetLegacyTag(ctx context.Context, id int64) (contactapp.LegacyTag, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.LegacyTag{}, err
	}
	var out contactapp.LegacyTag
	err = tx.QueryRow(ctx, `SELECT t.id, t.group_id, g.name, t.name, t.sort_order FROM public.tags AS t JOIN public.tag_groups AS g ON g.id = t.group_id WHERE t.id = $1`, id).Scan(&out.ID, &out.GroupID, &out.GroupName, &out.Name, &out.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.LegacyTag{}, contactapp.ErrLegacyTagNotFound
	}
	return out, err
}

func (repository *LegacyTagCatalogRepository) ReorderLegacyTagGroups(ctx context.Context, ids []int64) ([]contactapp.LegacyTagGroup, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	for index, id := range ids {
		result, err := tx.Exec(ctx, `UPDATE public.tag_groups SET sort_order = $2 WHERE id = $1 AND name NOT LIKE 'archived:%'`, id, int32(index))
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() != 1 {
			return nil, contactapp.ErrLegacyTagConflict
		}
	}
	return repository.ListLegacyTagGroups(ctx)
}

func (repository *LegacyTagCatalogRepository) ReorderLegacyTags(ctx context.Context, ids []int64) ([]contactapp.LegacyTag, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	for index, id := range ids {
		result, err := tx.Exec(ctx, `UPDATE public.tags SET sort_order = $2 WHERE id = $1 AND name NOT LIKE 'archived:%'`, id, int32(index))
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() != 1 {
			return nil, contactapp.ErrLegacyTagConflict
		}
	}
	return repository.ListLegacyTags(ctx)
}

func (*LegacyTagCatalogRepository) ReserveLocalTagReceipt(ctx context.Context, reservation contactapp.LocalTagReceiptReservation) (contactapp.LocalTagReceipt, bool, error) {
	if reservation.Operation == "" || reservation.Actor < 1 || len(reservation.IdempotencyKey) < 16 || len(reservation.IdempotencyKey) > 128 || len(reservation.PayloadDigest) != sha256.Size {
		return contactapp.LocalTagReceipt{}, false, contactapp.ErrInvalidLegacyTag
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.LocalTagReceipt{}, false, err
	}
	keyDigest := sha256.Sum256([]byte(reservation.IdempotencyKey))
	actor := strconv.FormatInt(reservation.Actor, 10)
	var receipt contactapp.LocalTagReceipt
	err = tx.QueryRow(ctx, `
		INSERT INTO public.tag_catalog_operation_receipts (operation, actor, key_digest, payload_digest, created_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (operation, actor, key_digest) DO NOTHING
		RETURNING id, operation, actor::bigint, payload_digest, state, result_ids`, reservation.Operation, actor, keyDigest[:], reservation.PayloadDigest).
		Scan(&receipt.ID, &receipt.Operation, &receipt.Actor, &receipt.PayloadDigest, &receipt.State, &receipt.ResultIDs)
	if err == nil {
		receipt.IdempotencyKey = reservation.IdempotencyKey
		return receipt, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactapp.LocalTagReceipt{}, false, err
	}
	err = tx.QueryRow(ctx, `SELECT id, operation, actor::bigint, payload_digest, state, result_ids FROM public.tag_catalog_operation_receipts WHERE operation = $1 AND actor = $2 AND key_digest = $3`, reservation.Operation, actor, keyDigest[:]).
		Scan(&receipt.ID, &receipt.Operation, &receipt.Actor, &receipt.PayloadDigest, &receipt.State, &receipt.ResultIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.LocalTagReceipt{}, false, contactapp.ErrLegacyTagUnavailable
	}
	if err != nil {
		return contactapp.LocalTagReceipt{}, false, err
	}
	receipt.IdempotencyKey = reservation.IdempotencyKey
	return receipt, false, nil
}

func (*LegacyTagCatalogRepository) CompleteLocalTagReceipt(ctx context.Context, id int64, resultIDs []int64, completedAt time.Time) (contactapp.LocalTagReceipt, error) {
	if id <= 0 || len(resultIDs) == 0 || completedAt.IsZero() {
		return contactapp.LocalTagReceipt{}, contactapp.ErrLegacyTagConflict
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.LocalTagReceipt{}, err
	}
	var receipt contactapp.LocalTagReceipt
	err = tx.QueryRow(ctx, `
		UPDATE public.tag_catalog_operation_receipts
		SET state = 'completed', result_ids = $2, completed_at = $3
		WHERE id = $1 AND state = 'in_progress'
		RETURNING id, operation, actor::bigint, payload_digest, state, result_ids`, id, resultIDs, completedAt).
		Scan(&receipt.ID, &receipt.Operation, &receipt.Actor, &receipt.PayloadDigest, &receipt.State, &receipt.ResultIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.LocalTagReceipt{}, contactapp.ErrLegacyTagConflict
	}
	return receipt, err
}
func (r *LegacyTagCatalogRepository) inflate(ctx context.Context, id int64, g pgtype.Int8, name string, sort int32) (contactapp.LegacyTag, error) {
	if !g.Valid {
		return contactapp.LegacyTag{}, contactapp.ErrLegacyTagUnavailable
	}
	groups, e := r.ListLegacyTagGroups(ctx)
	if e != nil {
		return contactapp.LegacyTag{}, e
	}
	for _, x := range groups {
		if x.ID == g.Int64 {
			return contactapp.LegacyTag{ID: id, GroupID: g.Int64, GroupName: x.Name, Name: name, SortOrder: sort}, nil
		}
	}
	return contactapp.LegacyTag{ID: id, GroupID: g.Int64, Name: name, SortOrder: sort}, nil
}
func classifyLegacyTagStore(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return contactapp.ErrLegacyTagNotFound
	}
	return e
}
func pgxInt8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }
