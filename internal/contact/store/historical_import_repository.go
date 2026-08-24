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
	if strings.TrimSpace(userID) != userID || userID == "" || strings.TrimSpace(name) != name || name == "" || createdAt.IsZero() || updatedAt.IsZero() {
		return 0, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	return contactdb.New(tx).UpsertHistoricalImportStaff(ctx, contactdb.UpsertHistoricalImportStaffParams{WecomUserid: userID, Name: name, IsActive: active, CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true}})
}

var ErrInvalidHistoricalImport = historicalImportError("invalid DM01 historical import")

type historicalImportError string

func (e historicalImportError) Error() string { return string(e) }
