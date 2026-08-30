package authacceptance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateAdminUser(ctx context.Context, pool *pgxpool.Pool, subject string) (int64, error) {
	if pool == nil || subject == "" {
		return 0, fmt.Errorf("valid Auth admin fixture required")
	}
	var id int64
	err := pool.QueryRow(ctx, `
INSERT INTO admin_users(auth_provider,wecom_corp_id,provider_subject_id,display_name,role)
VALUES('wecom','acceptance-fixture',$1,$1,'admin') RETURNING id`, subject).Scan(&id)
	return id, err
}
