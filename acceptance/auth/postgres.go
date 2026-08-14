package authacceptance

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgreSQL keeps direct acceptance SQL outside the runtime composition root.
// It is test-only evidence over the isolated aicrm_test database.
type PostgreSQL struct {
	pool *pgxpool.Pool
}

func OpenPostgreSQL(ctx context.Context, databaseURL string) (*PostgreSQL, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &PostgreSQL{pool: pool}, nil
}

func (fixture *PostgreSQL) Close() { fixture.pool.Close() }

func (fixture *PostgreSQL) Pool() *pgxpool.Pool { return fixture.pool }

func (fixture *PostgreSQL) ServerVersion(ctx context.Context) (int, error) {
	var version int
	err := fixture.pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version)
	return version, err
}

func (fixture *PostgreSQL) SeedAdmin(ctx context.Context) (int64, error) {
	var userID int64
	err := fixture.pool.QueryRow(ctx, `
INSERT INTO admin_users (
  auth_provider, wecom_corp_id, provider_subject_id, display_name, role,
  is_active, login_enabled, session_version
) VALUES ('wecom', 'corp-a01-fixture', 'member-a01-fixture', 'A01 fixture', 'admin', TRUE, TRUE, 1)
ON CONFLICT (auth_provider, wecom_corp_id, provider_subject_id) DO UPDATE SET
  display_name=EXCLUDED.display_name, role=EXCLUDED.role, staff_id=NULL,
  is_active=TRUE, login_enabled=TRUE, session_version=admin_users.session_version + 1,
  updated_at=now()
RETURNING id`).Scan(&userID)
	return userID, err
}

func (fixture *PostgreSQL) Reset(ctx context.Context, userID int64) error {
	if _, err := fixture.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE admin_user_id=$1`, userID); err != nil {
		return err
	}
	_, err := fixture.pool.Exec(ctx, `DELETE FROM admin_oauth_states`)
	return err
}

func (fixture *PostgreSQL) Persistence(ctx context.Context, state, session string) (bool, bool, bool, error) {
	var stateConsumed, rawStateStored, rawSessionStored bool
	err := fixture.pool.QueryRow(ctx, `
SELECT
  NOT EXISTS (SELECT 1 FROM admin_oauth_states),
  EXISTS (SELECT 1 FROM admin_oauth_states WHERE state_hash=convert_to($1, 'UTF8')),
  EXISTS (SELECT 1 FROM admin_sessions WHERE session_token_hash=convert_to($2, 'UTF8'))`,
		state, session).Scan(&stateConsumed, &rawStateStored, &rawSessionStored)
	return stateConsumed, rawStateStored, rawSessionStored, err
}

func (fixture *PostgreSQL) SessionRevoked(ctx context.Context, userID int64) (bool, error) {
	var revoked bool
	err := fixture.pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM admin_sessions WHERE admin_user_id=$1 ORDER BY id DESC LIMIT 1`, userID).Scan(&revoked)
	return revoked, err
}

func (fixture *PostgreSQL) PreparePlanCorpus(ctx context.Context) error {
	if _, err := fixture.pool.Exec(ctx, `DELETE FROM admin_oauth_states`); err != nil {
		return err
	}
	if _, err := fixture.pool.Exec(ctx, `
INSERT INTO admin_oauth_states (state_hash, auth_provider, next_path, created_at, expires_at)
SELECT decode(md5(i::text) || md5('a01-' || i::text), 'hex'), 'wecom', '/admin', now(), now() + interval '10 minutes'
FROM generate_series(1, 200000) AS i`); err != nil {
		return err
	}
	_, err := fixture.pool.Exec(ctx, `ANALYZE admin_oauth_states`)
	return err
}

func (fixture *PostgreSQL) ClaimPlan(ctx context.Context) (string, error) {
	return fixture.explain(ctx, `
EXPLAIN (COSTS OFF)
DELETE FROM admin_oauth_states
WHERE state_hash=decode(md5('100000') || md5('a01-100000'), 'hex')
  AND auth_provider='wecom' AND expires_at > now()
RETURNING next_path`)
}

func (fixture *PostgreSQL) ExpiryPlan(ctx context.Context) (string, error) {
	return fixture.explain(ctx, `EXPLAIN (COSTS OFF) DELETE FROM admin_oauth_states WHERE expires_at <= now()`)
}

func (fixture *PostgreSQL) explain(ctx context.Context, statement string) (string, error) {
	rows, err := fixture.pool.Query(ctx, statement)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			return "", err
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	return plan.String(), nil
}
