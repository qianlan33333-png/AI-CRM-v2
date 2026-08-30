// Package acceptancefixture creates Auth-owned rows for cross-domain tests.
package acceptancefixture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func CreateAdminUser(ctx context.Context, db queryRower, subject string) (int64, error) {
	if db == nil || subject == "" {
		return 0, fmt.Errorf("valid Auth admin fixture required")
	}
	var id int64
	err := db.QueryRow(ctx, `
INSERT INTO admin_users(auth_provider,wecom_corp_id,provider_subject_id,display_name,role)
VALUES('wecom','acceptance-fixture',$1,$1,'admin') RETURNING id`, subject).Scan(&id)
	return id, err
}
