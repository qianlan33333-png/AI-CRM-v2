package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type StaffDirectoryRepository struct{ pool *pgxpool.Pool }

var _ contact.ActiveStaffReader = (*StaffDirectoryRepository)(nil)
var _ contact.ActiveStaffSenderReader = (*StaffDirectoryRepository)(nil)
var _ contact.ActiveStaffWeComUserIDReader = (*StaffDirectoryRepository)(nil)
var _ contact.EligibleStaffReferenceReader = (*StaffDirectoryRepository)(nil)
var _ contact.HistoricalImportStaffReader = (*StaffDirectoryRepository)(nil)
var _ contact.StaffDirectoryReader = (*StaffDirectoryRepository)(nil)

func NewStaffDirectoryRepository(pool *pgxpool.Pool) *StaffDirectoryRepository {
	return &StaffDirectoryRepository{pool: pool}
}

func (r *StaffDirectoryRepository) ListEligibleStaff(ctx context.Context) ([]contact.StaffDirectoryEntry, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return nil, contact.ErrStaffReferenceUnavailable
	}
	rows, err := contactdb.New(r.pool).ListEligibleStaffDirectory(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]contact.StaffDirectoryEntry, len(rows))
	for index, row := range rows {
		if !finiteStaffTimestamp(row.UpdatedAt) {
			return nil, contact.ErrStaffReferenceUnavailable
		}
		result[index] = contact.StaffDirectoryEntry{
			StaffID: row.ID, WeComUserID: strings.TrimSpace(row.WecomUserid),
			DisplayName: strings.TrimSpace(row.Name), UpdatedAt: row.UpdatedAt.Time.UTC(),
		}
	}
	return result, nil
}
func (*StaffDirectoryRepository) LockEligibleStaffByWeComUserID(ctx context.Context, weComUserID string) (contact.StaffDirectoryEntry, error) {
	if ctx == nil || strings.TrimSpace(weComUserID) != weComUserID || weComUserID == "" {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceUnavailable
	}
	rows, err := contactdb.New(tx).LockEligibleStaffDirectoryByWeComUserID(ctx, weComUserID)
	if err != nil {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceUnavailable
	}
	if len(rows) == 0 {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceNotFound
	}
	if len(rows) != 1 || !finiteStaffTimestamp(rows[0].UpdatedAt) {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceUnavailable
	}
	return contact.StaffDirectoryEntry{
		StaffID: rows[0].ID, WeComUserID: rows[0].WecomUserid,
		DisplayName: rows[0].Name, UpdatedAt: rows[0].UpdatedAt.Time.UTC(),
	}, nil
}

func finiteStaffTimestamp(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}

func (*StaffDirectoryRepository) LockUniqueActiveStaffForHistoricalImport(ctx context.Context, weComUserID string) (contact.HistoricalImportStaff, error) {
	if ctx == nil || strings.TrimSpace(weComUserID) != weComUserID || weComUserID == "" {
		return contact.HistoricalImportStaff{}, contact.ErrStaffReferenceNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contact.HistoricalImportStaff{}, contact.ErrStaffReferenceUnavailable
	}
	id, err := contactdb.New(tx).LockUniqueActiveStaffForHistoricalImport(ctx, weComUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return contact.HistoricalImportStaff{}, contact.ErrStaffReferenceNotFound
	}
	if err != nil {
		return contact.HistoricalImportStaff{}, contact.ErrStaffReferenceUnavailable
	}
	return contact.HistoricalImportStaff{ID: id}, nil
}

// IsActiveStaff holds a shared lock on the Contact-owned active row in the
// caller's transaction, so a concurrent deactivation cannot race a Group Ops
// member write.
func (*StaffDirectoryRepository) IsActiveStaff(ctx context.Context, staffID int64) (bool, error) {
	if staffID < 1 {
		return false, nil
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return false, err
	}
	active, err := contactdb.New(tx).LockActiveStaffExists(ctx, staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return active, nil
}

func (*StaffDirectoryRepository) LockActiveWeComUserID(ctx context.Context, staffID int64) (string, error) {
	if staffID < 1 {
		return "", contact.ErrStaffReferenceNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return "", contact.ErrStaffReferenceUnavailable
	}
	userID, err := contactdb.New(tx).LockActiveStaffWeComUserID(ctx, staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", contact.ErrStaffReferenceNotFound
	}
	if err != nil || strings.TrimSpace(userID) != userID {
		return "", contact.ErrStaffReferenceUnavailable
	}
	return userID, nil
}

func (r *StaffDirectoryRepository) ReadActiveWeComUserID(ctx context.Context, staffID int64) (string, error) {
	if r == nil || r.pool == nil || ctx == nil || staffID < 1 {
		return "", contact.ErrStaffReferenceUnavailable
	}
	userID, err := contactdb.New(r.pool).GetActiveStaffWeComUserID(ctx, staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", contact.ErrStaffReferenceNotFound
	}
	if err != nil || strings.TrimSpace(userID) != userID {
		return "", contact.ErrStaffReferenceUnavailable
	}
	return userID, nil
}
