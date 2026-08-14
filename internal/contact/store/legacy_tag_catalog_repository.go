package store

import (
	"context"
	"errors"

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
	row, e := q.CreateLegacyTag(ctx, contactdb.CreateLegacyTagParams{GroupID: pgxInt8(g), Name: name})
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
