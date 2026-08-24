package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// HistoricalImportRepository owns the narrow Contact-side DM01 writes. It is
// transaction-bound and intentionally has no role, Provider, or event method.
type HistoricalImportRepository struct{}

func (HistoricalImportRepository) UpsertStaff(ctx context.Context, userID, name string, active bool, createdAt, updatedAt time.Time) (int64, error) {
	if strings.TrimSpace(userID) != userID || userID == "" || strings.TrimSpace(name) != name || name == "" || createdAt.IsZero() || updatedAt.IsZero() || createdAt.After(updatedAt) {
		return 0, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	return contactdb.New(tx).InsertHistoricalImportStaff(ctx, contactdb.InsertHistoricalImportStaffParams{WecomUserid: userID, Name: name, IsActive: active, CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true}})
}

func (HistoricalImportRepository) CreateCustomer(ctx context.Context, name string, avatarURL *string, gender *int16, ownerStaffID *int64, firstSeenAt, lastSeenAt, createdAt, updatedAt time.Time) (int64, error) {
	if firstSeenAt.IsZero() || lastSeenAt.IsZero() || createdAt.IsZero() || updatedAt.IsZero() || createdAt.After(updatedAt) || firstSeenAt.After(lastSeenAt) {
		return 0, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	avatar := pgtype.Text{}
	if avatarURL != nil && *avatarURL != "" {
		avatar = pgtype.Text{String: *avatarURL, Valid: true}
	}
	genderValue := pgtype.Int2{}
	if gender != nil {
		genderValue = pgtype.Int2{Int16: *gender, Valid: true}
	}
	owner := pgtype.Int8{}
	if ownerStaffID != nil {
		owner = pgtype.Int8{Int64: *ownerStaffID, Valid: true}
	}
	return contactdb.New(tx).CreateHistoricalImportCustomer(ctx, contactdb.CreateHistoricalImportCustomerParams{Name: name, AvatarUrl: avatar, Gender: genderValue, OwnerStaffID: owner, FirstSeenAt: pgtype.Timestamptz{Time: firstSeenAt, Valid: true}, LastSeenAt: pgtype.Timestamptz{Time: lastSeenAt, Valid: true}, CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true}})
}

var ErrInvalidHistoricalImport = historicalImportError("invalid DM01 historical import")

type historicalImportError string

func (e historicalImportError) Error() string { return string(e) }
