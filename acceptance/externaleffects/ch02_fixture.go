package externaleffects_acceptance

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpireCH02Lease advances only the isolated CH02 PG16 acceptance fixture.
func ExpireCH02Lease(ctx context.Context, pool *pgxpool.Pool, effectID int64) error {
	_, err := pool.Exec(ctx, `UPDATE external_effects SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, effectID)
	return err
}
