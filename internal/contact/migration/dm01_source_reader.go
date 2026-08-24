package migration

import (
	"context"
	"encoding/json"
	"errors"
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

// OpenSourceReader receives its DSN from typed command composition. Migration
// code never reads process environment values or records the DSN.
func OpenSourceReader(ctx context.Context, dsn string) (*SourceReader, error) {
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

func (r *SourceReader) Close() {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
}

type Snapshot struct {
	Bounds []SourceUpperBound
	tx     pgx.Tx
}

type OwnerRoleMapRow struct {
	UserID      string
	DisplayName string
	Active      bool
	UpdatedAt   time.Time
	Payload     json.RawMessage
}

type CustomerIdentityRow struct {
	UnionID          string
	CustomerName     string
	AvatarURL        string
	Gender           *int16
	PrimaryOwnerUser string
	FirstSeenAt      time.Time
	LastSeenAt       time.Time
	UpdatedAt        time.Time
	Payload          json.RawMessage
}

type ExternalIdentityMapRow struct {
	ID             int64
	ExternalUserID string
	UnionID        string
	CorpID         string
	UpdatedAt      time.Time
	Payload        json.RawMessage
}

// EachOwnerRoleMap is a closed, static projection. role/source/raw are kept
// only in Payload for restricted archive/digest handling; none becomes RBAC.
func (s *Snapshot) EachOwnerRoleMap(ctx context.Context, upper SourceUpperBound, fn func(OwnerRoleMapRow) error) error {
	if upper.Table != "owner_role_map" || upper.Empty {
		return nil
	}
	rows, err := s.tx.Query(ctx, `SELECT userid, display_name, active, updated_at, jsonb_build_object('userid',userid,'display_name',display_name,'role',role,'active',active,'source',source,'raw_payload_json',raw_payload_json,'created_at',created_at,'updated_at',updated_at) FROM owner_role_map WHERE (updated_at, userid) <= ($1, $2) ORDER BY updated_at, userid`, upper.Watermark, upper.SourceKey)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row OwnerRoleMapRow
		if err := rows.Scan(&row.UserID, &row.DisplayName, &row.Active, &row.UpdatedAt, &row.Payload); err != nil {
			return err
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// EachCustomerIdentity keeps unionid as a source key only. Identity hints,
// mobile, aliases and polling state remain inside the digest/archive payload.
func (s *Snapshot) EachCustomerIdentity(ctx context.Context, upper SourceUpperBound, fn func(CustomerIdentityRow) error) error {
	if upper.Table != "crm_user_identity" || upper.Empty {
		return nil
	}
	rows, err := s.tx.Query(ctx, `SELECT unionid, customer_name, avatar, gender, primary_owner_userid, first_seen_at, last_seen_at, updated_at, jsonb_build_object('unionid',unionid,'primary_external_userid',primary_external_userid,'external_userids_json',external_userids_json,'primary_openid',primary_openid,'openids_json',openids_json,'mobile',mobile,'mobile_normalized',mobile_normalized,'mobile_verified',mobile_verified,'mobile_source',mobile_source,'customer_name',customer_name,'remark',remark,'description',description,'avatar',avatar,'gender',gender,'profile_json',profile_json,'primary_owner_userid',primary_owner_userid,'follow_users_json',follow_users_json,'legacy_person_id',legacy_person_id,'legacy_identity_map_ids_json',legacy_identity_map_ids_json,'legacy_sources_json',legacy_sources_json,'identity_status',identity_status,'unionid_resolved_at',unionid_resolved_at,'first_seen_at',first_seen_at,'last_seen_at',last_seen_at,'last_polled_at',last_polled_at,'next_poll_at',next_poll_at,'poll_attempt_count',poll_attempt_count,'last_poll_error',last_poll_error,'created_at',created_at,'updated_at',updated_at) FROM crm_user_identity WHERE (updated_at, unionid) <= ($1, $2) ORDER BY updated_at, unionid`, upper.Watermark, upper.SourceKey)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row CustomerIdentityRow
		if err := rows.Scan(&row.UnionID, &row.CustomerName, &row.AvatarURL, &row.Gender, &row.PrimaryOwnerUser, &row.FirstSeenAt, &row.LastSeenAt, &row.UpdatedAt, &row.Payload); err != nil {
			return err
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Snapshot) EachExternalIdentityMap(ctx context.Context, upper SourceUpperBound, fn func(ExternalIdentityMapRow) error) error {
	if upper.Table != "wecom_external_contact_identity_map" || upper.Empty {
		return nil
	}
	rows, err := s.tx.Query(ctx, `SELECT id, external_userid, unionid, corp_id, updated_at, jsonb_build_object('id',id,'external_userid',external_userid,'unionid',unionid,'openid',openid,'follow_user_userid',follow_user_userid,'name',name,'status',status,'updated_at',updated_at,'corp_id',corp_id,'avatar',avatar,'gender',gender,'raw_profile',raw_profile,'first_seen_at',first_seen_at,'last_seen_at',last_seen_at,'created_at',created_at) FROM wecom_external_contact_identity_map WHERE (updated_at, id) <= ($1, $2::bigint) ORDER BY updated_at, id`, upper.Watermark, upper.SourceKey)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row ExternalIdentityMapRow
		if err := rows.Scan(&row.ID, &row.ExternalUserID, &row.UnionID, &row.CorpID, &row.UpdatedAt, &row.Payload); err != nil {
			return err
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	return rows.Err()
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
			"external_contact_bindings":           "SELECT updated_at, external_userid FROM external_contact_bindings ORDER BY updated_at DESC, external_userid DESC LIMIT 1",
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
