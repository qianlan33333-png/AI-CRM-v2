package migration

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SourceReader is deliberately closed: it has no caller supplied SQL, table,
// or DSN. Every source query below is a typed projection of the DM01 manifest.
type SourceReader struct {
	pool *pgxpool.Pool
}

type SourceUpperBound struct {
	Table     string
	Watermark time.Time
	SourceKey string
	Empty     bool
}

func OpenSourceReader(ctx context.Context) (*SourceReader, error) {
	dsn := os.Getenv(SourceEnvironment)
	if dsn == "" {
		return nil, errors.New("DM01 source database URL is not configured")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &SourceReader{pool: pool}, nil
}

func NewSourceReader(pool *pgxpool.Pool) *SourceReader { return &SourceReader{pool: pool} }

func (r *SourceReader) Close() { r.pool.Close() }

type Snapshot struct {
	Bounds []SourceUpperBound
	tx     pgx.Tx
}

// WithSnapshot is the only source-read entrypoint. Bounds and every caller
// scan happen in the same READ ONLY REPEATABLE READ transaction; callers
// cannot reuse a bound in a later transaction. Incremental callers may only
// append rows at or below the fixed bound and must never infer tombstones.
func (r *SourceReader) WithSnapshot(ctx context.Context, manifest Manifest, fn func(*Snapshot) error) error {
	if err := manifest.Valid(); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	bounds := make([]SourceUpperBound, 0, len(manifest.Tables))
	for _, table := range manifest.Tables {
		bound, err := upperBound(ctx, tx, table.Name)
		if err != nil {
			return err
		}
		bounds = append(bounds, bound)
	}
	if err := fn(&Snapshot{Bounds: bounds, tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func upperBound(ctx context.Context, tx pgx.Tx, table string) (SourceUpperBound, error) {
	var watermark time.Time
	var key string
	var err error
	switch table {
	case "owner_role_map":
		err = tx.QueryRow(ctx, `SELECT updated_at, userid FROM owner_role_map ORDER BY updated_at DESC, userid DESC LIMIT 1`).Scan(&watermark, &key)
	case "crm_user_identity":
		err = tx.QueryRow(ctx, `SELECT updated_at, unionid FROM crm_user_identity ORDER BY updated_at DESC, unionid DESC LIMIT 1`).Scan(&watermark, &key)
	case "wecom_external_contact_identity_map":
		err = tx.QueryRow(ctx, `SELECT updated_at, id::text FROM wecom_external_contact_identity_map ORDER BY updated_at DESC, id DESC LIMIT 1`).Scan(&watermark, &key)
	case "crm_user_identity_merge_audit":
		err = tx.QueryRow(ctx, `SELECT created_at, id::text FROM crm_user_identity_merge_audit ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&watermark, &key)
	case "crm_user_identity_resolution_queue":
		err = tx.QueryRow(ctx, `SELECT updated_at, id::text FROM crm_user_identity_resolution_queue ORDER BY updated_at DESC, id DESC LIMIT 1`).Scan(&watermark, &key)
	case "admin_wecom_directory_members":
		err = tx.QueryRow(ctx, `SELECT last_synced_at, id::text FROM admin_wecom_directory_members ORDER BY last_synced_at DESC, id DESC LIMIT 1`).Scan(&watermark, &key)
	case "contacts", "crm_user_identity_conflicts", "external_contact_bindings", "people", "wecom_external_contact_follow_users":
		queries := map[string]string{
			"contacts":                            "SELECT updated_at, id::text FROM contacts ORDER BY updated_at DESC, id DESC LIMIT 1",
			"crm_user_identity_conflicts":         "SELECT updated_at, id::text FROM crm_user_identity_conflicts ORDER BY updated_at DESC, id DESC LIMIT 1",
			"external_contact_bindings":           "SELECT updated_at, id::text FROM external_contact_bindings ORDER BY updated_at DESC, id DESC LIMIT 1",
			"people":                              "SELECT updated_at, id::text FROM people ORDER BY updated_at DESC, id DESC LIMIT 1",
			"wecom_external_contact_follow_users": "SELECT updated_at, id::text FROM wecom_external_contact_follow_users ORDER BY updated_at DESC, id DESC LIMIT 1",
		}
		err = tx.QueryRow(ctx, queries[table]).Scan(&watermark, &key)
	default:
		return SourceUpperBound{}, errors.New("unsupported DM01 source table")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceUpperBound{Table: table, Empty: true}, nil
	}
	if err != nil {
		return SourceUpperBound{}, err
	}
	return SourceUpperBound{Table: table, Watermark: watermark.UTC(), SourceKey: key}, nil
}
