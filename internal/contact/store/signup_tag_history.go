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

type SignupTagHistoryStore struct{}
type SignupTagHistoryReader struct{ db contactdb.DBTX }

var _ contactport.SignupTagHistoryStore = (*SignupTagHistoryStore)(nil)
var _ contactport.SignupTagHistoryReader = (*SignupTagHistoryReader)(nil)

func NewSignupTagHistoryStore() *SignupTagHistoryStore { return &SignupTagHistoryStore{} }
func NewSignupTagHistoryReader(db contactdb.DBTX) *SignupTagHistoryReader {
	return &SignupTagHistoryReader{db: db}
}

func (store *SignupTagHistoryStore) queries(ctx context.Context) (*contactdb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, contactport.ErrSignupTagHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contactport.ErrSignupTagHistoryUnavailable
	}
	return contactdb.New(tx), nil
}

func (reader *SignupTagHistoryReader) queries(ctx context.Context) (*contactdb.Queries, error) {
	if reader == nil || reader.db == nil || ctx == nil || ctx.Err() != nil {
		return nil, contactport.ErrSignupTagHistoryUnavailable
	}
	value := reflect.ValueOf(reader.db)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, contactport.ErrSignupTagHistoryUnavailable
	}
	return contactdb.New(reader.db), nil
}

func (store *SignupTagHistoryStore) CreateHistoricalSignupTagRule(ctx context.Context, value contactport.HistoricalSignupTagRule) (contactport.HistoricalSignupTagRule, error) {
	if value.ID != 0 {
		return contactport.HistoricalSignupTagRule{}, contactport.ErrSignupTagHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := contactapp.HistoricalSignupTagRuleDigest(check); err != nil {
		return contactport.HistoricalSignupTagRule{}, contactport.ErrSignupTagHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalSignupTagRule{}, err
	}
	row, err := queries.CreateHistoricalSignupTagRule(ctx, contactdb.CreateHistoricalSignupTagRuleParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:],
		TagSourceID: value.TagSourceID, TagName: value.TagName, SignupStatus: value.SignupStatus,
		OriginalActive: value.OriginalActive, UpdatedAt: signupTagHistoryTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return contactport.HistoricalSignupTagRule{}, signupTagHistoryStoreError(err)
	}
	return signupTagHistoryValue(row)
}

func (store *SignupTagHistoryStore) GetHistoricalSignupTagRule(ctx context.Context, id int64) (contactport.HistoricalSignupTagRule, error) {
	if id < 1 {
		return contactport.HistoricalSignupTagRule{}, contactport.ErrSignupTagHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalSignupTagRule{}, err
	}
	row, err := queries.GetHistoricalSignupTagRule(ctx, id)
	if err != nil {
		return contactport.HistoricalSignupTagRule{}, signupTagHistoryStoreError(err)
	}
	return signupTagHistoryValue(row)
}

func (reader *SignupTagHistoryReader) GetHistoricalSignupTagRule(ctx context.Context, id int64) (contactport.HistoricalSignupTagRule, error) {
	if id < 1 {
		return contactport.HistoricalSignupTagRule{}, contactport.ErrSignupTagHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return contactport.HistoricalSignupTagRule{}, err
	}
	row, err := queries.GetHistoricalSignupTagRule(ctx, id)
	if err != nil {
		return contactport.HistoricalSignupTagRule{}, signupTagHistoryStoreError(err)
	}
	return signupTagHistoryValue(row)
}

func (reader *SignupTagHistoryReader) ListHistoricalSignupTagRules(ctx context.Context, limit, offset int32) ([]contactport.HistoricalSignupTagRule, int64, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, contactport.ErrSignupTagHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalSignupTagRules(ctx)
	if err != nil {
		return nil, 0, signupTagHistoryStoreError(err)
	}
	rows, err := queries.ListHistoricalSignupTagRules(ctx, contactdb.ListHistoricalSignupTagRulesParams{RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, signupTagHistoryStoreError(err)
	}
	items := make([]contactport.HistoricalSignupTagRule, 0, len(rows))
	for _, row := range rows {
		value, valueErr := signupTagHistoryValue(row)
		if valueErr != nil {
			return nil, 0, valueErr
		}
		items = append(items, value)
	}
	return items, total, nil
}

func signupTagHistoryValue(row contactdb.ContactV1SignupTagRule) (contactport.HistoricalSignupTagRule, error) {
	if row.ID < 1 || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 || !signupTagHistoryFinite(row.UpdatedAt) {
		return contactport.HistoricalSignupTagRule{}, contactport.ErrSignupTagHistoryUnavailable
	}
	value := contactport.HistoricalSignupTagRule{ID: row.ID, TagSourceID: row.TagSourceID, TagName: row.TagName, SignupStatus: row.SignupStatus,
		OriginalActive: row.OriginalActive, UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	if _, err := contactapp.HistoricalSignupTagRuleDigest(value); err != nil {
		return contactport.HistoricalSignupTagRule{}, contactport.ErrSignupTagHistoryUnavailable
	}
	return value, nil
}

func signupTagHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func signupTagHistoryFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}

func signupTagHistoryStoreError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return contactport.ErrSignupTagHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.ErrSignupTagHistoryConflict
	}
	return contactport.ErrSignupTagHistoryUnavailable
}
