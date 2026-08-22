package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"strings"
)

type StaffDirectoryRepository struct{ pool *pgxpool.Pool }

var _ contact.ActiveStaffReader = (*StaffDirectoryRepository)(nil)

func NewStaffDirectoryRepository(pool *pgxpool.Pool) *StaffDirectoryRepository {
	return &StaffDirectoryRepository{pool: pool}
}
func (r *StaffDirectoryRepository) ListEligibleStaff(ctx context.Context) ([]contact.StaffDirectoryEntry, error) {
	rows, err := r.pool.Query(ctx, `SELECT wecom_userid,name,updated_at FROM staff WHERE is_active AND btrim(wecom_userid) <> '' ORDER BY btrim(wecom_userid),wecom_userid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contact.StaffDirectoryEntry
	for rows.Next() {
		var x contact.StaffDirectoryEntry
		if err := rows.Scan(&x.WeComUserID, &x.DisplayName, &x.UpdatedAt); err != nil {
			return nil, err
		}
		x.WeComUserID = strings.TrimSpace(x.WeComUserID)
		x.DisplayName = strings.TrimSpace(x.DisplayName)
		out = append(out, x)
	}
	return out, rows.Err()
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
