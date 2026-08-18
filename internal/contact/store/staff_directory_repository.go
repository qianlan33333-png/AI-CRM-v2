package store

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"strings"
)

type StaffDirectoryRepository struct{ pool *pgxpool.Pool }

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
