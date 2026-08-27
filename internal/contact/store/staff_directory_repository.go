package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
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
	rows, err := r.pool.Query(ctx, `SELECT id, wecom_userid, name, updated_at FROM staff WHERE is_active AND btrim(wecom_userid) <> '' ORDER BY btrim(wecom_userid), wecom_userid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []contact.StaffDirectoryEntry{}
	for rows.Next() {
		var entry contact.StaffDirectoryEntry
		if err := rows.Scan(&entry.StaffID, &entry.WeComUserID, &entry.DisplayName, &entry.UpdatedAt); err != nil {
			return nil, err
		}
		entry.WeComUserID = strings.TrimSpace(entry.WeComUserID)
		entry.DisplayName = strings.TrimSpace(entry.DisplayName)
		result = append(result, entry)
	}
	return result, rows.Err()
}
func (*StaffDirectoryRepository) LockEligibleStaffByWeComUserID(ctx context.Context, weComUserID string) (contact.StaffDirectoryEntry, error) {
	if ctx == nil || strings.TrimSpace(weComUserID) != weComUserID || weComUserID == "" {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceUnavailable
	}
	rows, err := tx.Query(ctx, `SELECT id, wecom_userid, name, updated_at FROM staff WHERE wecom_userid = $1 AND is_active FOR SHARE`, weComUserID)
	if err != nil {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceUnavailable
	}
	defer rows.Close()
	var result contact.StaffDirectoryEntry
	for rows.Next() {
		if result.WeComUserID != "" || rows.Scan(&result.StaffID, &result.WeComUserID, &result.DisplayName, &result.UpdatedAt) != nil {
			return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceUnavailable
		}
	}
	if rows.Err() != nil {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceUnavailable
	}
	if result.WeComUserID == "" {
		return contact.StaffDirectoryEntry{}, contact.ErrStaffReferenceNotFound
	}
	return result, nil
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
	var active bool
	err = tx.QueryRow(ctx, `SELECT TRUE FROM staff WHERE id = $1 AND is_active FOR SHARE`, staffID).Scan(&active)
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
