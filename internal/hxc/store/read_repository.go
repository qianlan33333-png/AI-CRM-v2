package store

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	platform "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type SenderConfigRepository struct{ pool *pgxpool.Pool }

func NewSenderConfigRepository(pool *pgxpool.Pool) *SenderConfigRepository {
	return &SenderConfigRepository{pool: pool}
}
func (r *SenderConfigRepository) ListSenderConfigs(ctx context.Context) ([]hxc.SenderConfig, error) {
	var query interface {
		Query(context.Context, string, ...any) (pgx.Rows, error)
	} = r.pool
	if tx, err := platform.TxFromContext(ctx); err == nil {
		query = tx
	}
	rows, err := query.Query(ctx, `SELECT id,sender_userid,display_name,priority,is_active,created_at,updated_at FROM hxc_sender_configs ORDER BY priority,btrim(sender_userid),id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hxc.SenderConfig
	for rows.Next() {
		var x hxc.SenderConfig
		if err := rows.Scan(&x.ID, &x.SenderUserID, &x.DisplayName, &x.Priority, &x.IsActive, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
