package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type ContactHistoryStore struct{}
type ContactHistoryReader struct{ db contactdb.DBTX }

var _ contactport.ContactHistoryStore = (*ContactHistoryStore)(nil)
var _ contactport.ContactHistoryReader = (*ContactHistoryReader)(nil)

func NewContactHistoryStore() *ContactHistoryStore { return &ContactHistoryStore{} }
func NewContactHistoryReader(db contactdb.DBTX) *ContactHistoryReader {
	return &ContactHistoryReader{db: db}
}

func (store *ContactHistoryStore) queries(ctx context.Context) (*contactdb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, contactport.ErrContactHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contactport.ErrContactHistoryUnavailable
	}
	return contactdb.New(tx), nil
}

func (reader *ContactHistoryReader) queries(ctx context.Context) (*contactdb.Queries, error) {
	if reader == nil || reader.db == nil || ctx == nil || ctx.Err() != nil {
		return nil, contactport.ErrContactHistoryUnavailable
	}
	v := reflect.ValueOf(reader.db)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return nil, contactport.ErrContactHistoryUnavailable
	}
	return contactdb.New(reader.db), nil
}

func (store *ContactHistoryStore) CreateHistoricalSidebarProfile(ctx context.Context, value contactport.HistoricalSidebarProfile) (contactport.HistoricalSidebarProfile, error) {
	if value.ID != 0 {
		return contactport.HistoricalSidebarProfile{}, contactport.ErrContactHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := contactapp.HistoricalSidebarProfileDigest(check); err != nil {
		return contactport.HistoricalSidebarProfile{}, contactport.ErrContactHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalSidebarProfile{}, err
	}
	row, err := q.CreateHistoricalSidebarProfile(ctx, contactdb.CreateHistoricalSidebarProfileParams{
		SourceKeyDigest: value.SourceKeyDigest[:], CustomerID: contactHistoryInt(value.CustomerID), Source: value.Source,
		Industry: value.Industry, IndustryDescription: value.IndustryDescription, NeedsBlockersFollowup: value.NeedsBlockersFollowup,
		UpdatedAt: contactHistoryTimestamp(value.UpdatedAt), SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return contactport.HistoricalSidebarProfile{}, contactHistoryError(err)
	}
	return contactHistorySidebarValue(row)
}

func (store *ContactHistoryStore) GetHistoricalSidebarProfile(ctx context.Context, id int64) (contactport.HistoricalSidebarProfile, error) {
	if id < 1 {
		return contactport.HistoricalSidebarProfile{}, contactport.ErrContactHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalSidebarProfile{}, err
	}
	row, err := q.GetHistoricalSidebarProfile(ctx, id)
	if err != nil {
		return contactport.HistoricalSidebarProfile{}, contactHistoryError(err)
	}
	return contactHistorySidebarValue(row)
}

func (reader *ContactHistoryReader) GetHistoricalSidebarProfile(ctx context.Context, id int64) (contactport.HistoricalSidebarProfile, error) {
	if id < 1 {
		return contactport.HistoricalSidebarProfile{}, contactport.ErrContactHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return contactport.HistoricalSidebarProfile{}, err
	}
	row, err := q.GetHistoricalSidebarProfile(ctx, id)
	if err != nil {
		return contactport.HistoricalSidebarProfile{}, contactHistoryError(err)
	}
	return contactHistorySidebarValue(row)
}

func (reader *ContactHistoryReader) ListHistoricalSidebarProfiles(ctx context.Context, query contactport.ContactHistoryQuery) ([]contactport.HistoricalSidebarProfile, int64, error) {
	if !contactHistoryPage(query) || (query.CustomerID != nil && *query.CustomerID < 1) {
		return nil, 0, contactport.ErrContactHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalSidebarProfiles(ctx, contactHistoryInt(query.CustomerID))
	if err != nil {
		return nil, 0, contactHistoryError(err)
	}
	rows, err := q.ListHistoricalSidebarProfiles(ctx, contactdb.ListHistoricalSidebarProfilesParams{CustomerID: contactHistoryInt(query.CustomerID), RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, contactHistoryError(err)
	}
	items := make([]contactport.HistoricalSidebarProfile, 0, len(rows))
	for _, row := range rows {
		value, err := contactHistorySidebarValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (store *ContactHistoryStore) CreateHistoricalOwnerMigrationResult(ctx context.Context, value contactport.HistoricalOwnerMigrationResult) (contactport.HistoricalOwnerMigrationResult, error) {
	if value.ID != 0 {
		return contactport.HistoricalOwnerMigrationResult{}, contactport.ErrContactHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := contactapp.HistoricalOwnerMigrationResultDigest(check); err != nil {
		return contactport.HistoricalOwnerMigrationResult{}, contactport.ErrContactHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalOwnerMigrationResult{}, err
	}
	row, err := q.CreateHistoricalOwnerMigrationResult(ctx, contactdb.CreateHistoricalOwnerMigrationResultParams{
		SourceKeyDigest: value.SourceKeyDigest[:], ScopeType: value.ScopeType, FileHash: value.FileHash, PreviewHash: value.PreviewHash,
		TotalRows: value.TotalRows, EligibleCount: value.EligibleCount, WecomSuccess: value.WeComSuccess, WecomFailed: value.WeComFailed,
		CrmUpdated: value.CRMUpdated, IncludeWecomTransfer: value.IncludeWeComTransfer, TransferWelcomeMessage: value.TransferWelcomeMessage,
		SessionRelation: value.SessionRelation, PreviewRelation: value.PreviewRelation, CreatedAt: contactHistoryTimestamp(value.CreatedAt),
		ExecutedAt: contactHistoryTimestamp(value.ExecutedAt), SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return contactport.HistoricalOwnerMigrationResult{}, contactHistoryError(err)
	}
	return contactHistoryOwnerValue(row)
}

func (store *ContactHistoryStore) GetHistoricalOwnerMigrationResult(ctx context.Context, id int64) (contactport.HistoricalOwnerMigrationResult, error) {
	if id < 1 {
		return contactport.HistoricalOwnerMigrationResult{}, contactport.ErrContactHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalOwnerMigrationResult{}, err
	}
	row, err := q.GetHistoricalOwnerMigrationResult(ctx, id)
	if err != nil {
		return contactport.HistoricalOwnerMigrationResult{}, contactHistoryError(err)
	}
	return contactHistoryOwnerValue(row)
}

func (reader *ContactHistoryReader) GetHistoricalOwnerMigrationResult(ctx context.Context, id int64) (contactport.HistoricalOwnerMigrationResult, error) {
	if id < 1 {
		return contactport.HistoricalOwnerMigrationResult{}, contactport.ErrContactHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return contactport.HistoricalOwnerMigrationResult{}, err
	}
	row, err := q.GetHistoricalOwnerMigrationResult(ctx, id)
	if err != nil {
		return contactport.HistoricalOwnerMigrationResult{}, contactHistoryError(err)
	}
	return contactHistoryOwnerValue(row)
}

func (reader *ContactHistoryReader) ListHistoricalOwnerMigrationResults(ctx context.Context, query contactport.ContactHistoryQuery) ([]contactport.HistoricalOwnerMigrationResult, int64, error) {
	if !contactHistoryPage(query) || query.CustomerID != nil {
		return nil, 0, contactport.ErrContactHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalOwnerMigrationResults(ctx)
	if err != nil {
		return nil, 0, contactHistoryError(err)
	}
	rows, err := q.ListHistoricalOwnerMigrationResults(ctx, contactdb.ListHistoricalOwnerMigrationResultsParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, contactHistoryError(err)
	}
	items := make([]contactport.HistoricalOwnerMigrationResult, 0, len(rows))
	for _, row := range rows {
		value, err := contactHistoryOwnerValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func contactHistoryPage(query contactport.ContactHistoryQuery) bool {
	return query.Limit >= 1 && query.Limit <= 100 && query.Offset >= 0
}

func contactHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func contactHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func contactHistorySidebarValue(row contactdb.ContactV1SidebarProfileHistory) (contactport.HistoricalSidebarProfile, error) {
	if !contactHistoryFinite(row.UpdatedAt) || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 {
		return contactport.HistoricalSidebarProfile{}, contactport.ErrContactHistoryUnavailable
	}
	value := contactport.HistoricalSidebarProfile{ID: row.ID, Source: row.Source, Industry: row.Industry, IndustryDescription: row.IndustryDescription,
		NeedsBlockersFollowup: row.NeedsBlockersFollowup, UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	if row.CustomerID.Valid {
		customerID := row.CustomerID.Int64
		value.CustomerID = &customerID
	}
	if _, err := contactapp.HistoricalSidebarProfileDigest(value); err != nil {
		return contactport.HistoricalSidebarProfile{}, contactport.ErrContactHistoryUnavailable
	}
	return value, nil
}

func contactHistoryOwnerValue(row contactdb.ContactV1OwnerMigrationResultHistory) (contactport.HistoricalOwnerMigrationResult, error) {
	if !contactHistoryFinite(row.CreatedAt) || !contactHistoryFinite(row.ExecutedAt) || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 {
		return contactport.HistoricalOwnerMigrationResult{}, contactport.ErrContactHistoryUnavailable
	}
	value := contactport.HistoricalOwnerMigrationResult{ID: row.ID, ScopeType: row.ScopeType, FileHash: row.FileHash, PreviewHash: row.PreviewHash,
		TotalRows: row.TotalRows, EligibleCount: row.EligibleCount, WeComSuccess: row.WecomSuccess, WeComFailed: row.WecomFailed,
		CRMUpdated: row.CrmUpdated, IncludeWeComTransfer: row.IncludeWecomTransfer, TransferWelcomeMessage: row.TransferWelcomeMessage,
		SessionRelation: row.SessionRelation, PreviewRelation: row.PreviewRelation, CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond),
		ExecutedAt: row.ExecutedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	if _, err := contactapp.HistoricalOwnerMigrationResultDigest(value); err != nil {
		return contactport.HistoricalOwnerMigrationResult{}, contactport.ErrContactHistoryUnavailable
	}
	return value, nil
}

func contactHistoryFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}

func contactHistoryError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return contactport.ErrContactHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.ErrContactHistoryConflict
	}
	return contactport.ErrContactHistoryUnavailable
}
