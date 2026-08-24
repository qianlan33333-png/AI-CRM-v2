package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	legacysourcedb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/legacysource/generated"
)

const dm01SourcePageSize = int32(500)

type DM01SourceReader struct {
	pool *pgxpool.Pool
}

var _ migration.SourceReader = (*DM01SourceReader)(nil)

// OpenDM01SourceReader is the Contact-store composition seam. The DSN is
// supplied by typed composition; neither migration nor store reads env.
func OpenDM01SourceReader(ctx context.Context, dsn string) (migration.SourceReader, error) {
	if dsn == "" {
		return nil, errors.New("DM01 source database URL is not configured")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return NewDM01SourceReader(pool), nil
}

func NewDM01SourceReader(pool *pgxpool.Pool) *DM01SourceReader {
	return &DM01SourceReader{pool: pool}
}

func (reader *DM01SourceReader) Close() {
	if reader != nil && reader.pool != nil {
		reader.pool.Close()
	}
}

func (reader *DM01SourceReader) WithSnapshot(ctx context.Context, manifest migration.Manifest, fn func(migration.SourceSnapshot) error) error {
	if err := manifest.Valid(); err != nil {
		return err
	}
	if reader == nil || reader.pool == nil || fn == nil {
		return errors.New("invalid DM01 source reader")
	}
	tx, err := reader.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := legacysourcedb.New(tx)
	if err := preflightDM01SourceSchema(ctx, queries, manifest); err != nil {
		return err
	}
	bounds := make([]migration.SourceUpperBound, 0, len(manifest.Tables))
	for _, table := range manifest.Tables {
		bound, err := dm01UpperBound(ctx, queries, table.Name)
		if err != nil {
			return err
		}
		bounds = append(bounds, bound)
	}
	includeUnionID, includeOpenIDs := dm01IdentityProjection(manifest)
	snapshot := &dm01SourceSnapshot{
		bounds:         bounds,
		queries:        queries,
		includeUnionID: includeUnionID,
		includeOpenIDs: includeOpenIDs,
	}
	if err := fn(snapshot); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func preflightDM01SourceSchema(ctx context.Context, queries *legacysourcedb.Queries, manifest migration.Manifest) error {
	for _, table := range manifest.Tables {
		rows, err := queries.ListDM01SourceColumns(ctx, table.Name)
		if err != nil {
			return err
		}
		if err := validateDM01SourceSchema(table, rows); err != nil {
			return err
		}
	}
	return nil
}

func validateDM01SourceSchema(table migration.Table, rows []legacysourcedb.ListDM01SourceColumnsRow) error {
	columns := make([]migration.SourceColumn, len(rows))
	for index, row := range rows {
		columns[index] = migration.SourceColumn{Ordinal: row.Ordinal, Name: row.ColumnName, DataType: row.DataType, NotNull: row.NotNull}
	}
	digest, err := migration.CanonicalSchemaDigest(columns)
	if err != nil || digest != table.SchemaDigest {
		return fmt.Errorf("%w: %s", migration.ErrSourceSchemaDrift, table.Name)
	}
	return nil
}

func dm01IdentityProjection(manifest migration.Manifest) (includeUnionID, includeOpenIDs bool) {
	return manifest.OpenPlatformAccount != "", len(manifest.WeChatAppScopes) > 0
}

type dm01UpperBoundRow struct {
	updatedAt pgtype.Timestamptz
	key       string
}

func dm01UpperBound(ctx context.Context, queries *legacysourcedb.Queries, table string) (migration.SourceUpperBound, error) {
	var result dm01UpperBoundRow
	var err error
	switch table {
	case "owner_role_map":
		row, rowErr := queries.GetDM01OwnerRoleMapUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "crm_user_identity":
		row, rowErr := queries.GetDM01CustomerIdentityUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "wecom_external_contact_identity_map":
		row, rowErr := queries.GetDM01ExternalIdentityUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "crm_user_identity_merge_audit":
		row, rowErr := queries.GetDM01MergeAuditUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "crm_user_identity_resolution_queue":
		row, rowErr := queries.GetDM01ResolutionQueueUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "admin_wecom_directory_members":
		row, rowErr := queries.GetDM01DirectoryMemberUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "contacts":
		row, rowErr := queries.GetDM01ContactUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "crm_user_identity_conflicts":
		row, rowErr := queries.GetDM01IdentityConflictUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "external_contact_bindings":
		row, rowErr := queries.GetDM01ExternalBindingUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "people":
		row, rowErr := queries.GetDM01PersonUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	case "wecom_external_contact_follow_users":
		row, rowErr := queries.GetDM01FollowUserUpperBound(ctx)
		result, err = dm01UpperBoundRow{row.UpdatedAt, row.SourceKey}, rowErr
	default:
		return migration.SourceUpperBound{}, errors.New("unsupported DM01 source table")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return migration.SourceUpperBound{Table: table, Empty: true}, nil
	}
	if err != nil {
		return migration.SourceUpperBound{}, err
	}
	if !result.updatedAt.Valid || result.key == "" {
		return migration.SourceUpperBound{}, migration.ErrSourceSchemaDrift
	}
	return migration.SourceUpperBound{Table: table, Watermark: result.updatedAt.Time.UTC(), SourceKey: result.key}, nil
}

type dm01SourceSnapshot struct {
	bounds         []migration.SourceUpperBound
	queries        *legacysourcedb.Queries
	includeUnionID bool
	includeOpenIDs bool
}

func (snapshot *dm01SourceSnapshot) Bounds() []migration.SourceUpperBound {
	return append([]migration.SourceUpperBound(nil), snapshot.bounds...)
}

func (snapshot *dm01SourceSnapshot) EachOwnerRoleMap(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.OwnerRoleMapRow) error) error {
	if err := snapshot.validateBound("owner_role_map", upper, fn != nil); err != nil {
		return err
	}
	if upper.Empty {
		return nil
	}
	for offset := int32(0); ; offset += dm01SourcePageSize {
		rows, err := snapshot.queries.ListDM01OwnerRoleMap(ctx, legacysourcedb.ListDM01OwnerRoleMapParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: upper.SourceKey, PageSize: dm01SourcePageSize, PageOffset: offset})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if !row.UpdatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			if err := fn(migration.OwnerRoleMapRow{UserID: row.Userid, DisplayName: row.DisplayName, Active: row.Active, UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload}); err != nil {
				return err
			}
		}
		if len(rows) < int(dm01SourcePageSize) {
			return nil
		}
	}
}

func (snapshot *dm01SourceSnapshot) EachCustomerIdentity(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.CustomerIdentityRow) error) error {
	if err := snapshot.validateBound("crm_user_identity", upper, fn != nil); err != nil {
		return err
	}
	if upper.Empty {
		return nil
	}
	for offset := int32(0); ; offset += dm01SourcePageSize {
		rows, err := snapshot.queries.ListDM01CustomerIdentity(ctx, legacysourcedb.ListDM01CustomerIdentityParams{IncludeUnionid: snapshot.includeUnionID, IncludeOpenids: snapshot.includeOpenIDs, UpperWatermark: sourceTime(upper.Watermark), UpperKey: upper.SourceKey, PageSize: dm01SourcePageSize, PageOffset: offset})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if !row.FirstSeenAt.Valid || !row.LastSeenAt.Valid || !row.UpdatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			var gender *int16
			if row.Gender.Valid {
				if row.Gender.Int32 < math.MinInt16 || row.Gender.Int32 > math.MaxInt16 {
					return migration.ErrSourceSchemaDrift
				}
				value := int16(row.Gender.Int32)
				gender = &value
			}
			if err := fn(migration.CustomerIdentityRow{UnionID: row.Unionid, CustomerName: row.CustomerName, AvatarURL: row.Avatar, Gender: gender, PrimaryOwnerUser: row.PrimaryOwnerUserid, FirstSeenAt: row.FirstSeenAt.Time.UTC(), LastSeenAt: row.LastSeenAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload}); err != nil {
				return err
			}
		}
		if len(rows) < int(dm01SourcePageSize) {
			return nil
		}
	}
}

func (snapshot *dm01SourceSnapshot) EachExternalIdentityMap(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.ExternalIdentityMapRow) error) error {
	if err := snapshot.validateBound("wecom_external_contact_identity_map", upper, fn != nil); err != nil {
		return err
	}
	if upper.Empty {
		return nil
	}
	upperKey, err := strconv.ParseInt(upper.SourceKey, 10, 64)
	if err != nil || upperKey < 1 {
		return migration.ErrSourceSchemaDrift
	}
	for offset := int32(0); ; offset += dm01SourcePageSize {
		rows, err := snapshot.queries.ListDM01ExternalIdentityMap(ctx, legacysourcedb.ListDM01ExternalIdentityMapParams{IncludeUnionid: snapshot.includeUnionID, IncludeOpenids: snapshot.includeOpenIDs, UpperWatermark: sourceTime(upper.Watermark), UpperKey: upperKey, PageSize: dm01SourcePageSize, PageOffset: offset})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if !row.UpdatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			if err := fn(migration.ExternalIdentityMapRow{ID: row.ID, ExternalUserID: row.ExternalUserid, UnionID: row.Unionid, CorpID: row.CorpID, UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload}); err != nil {
				return err
			}
		}
		if len(rows) < int(dm01SourcePageSize) {
			return nil
		}
	}
}

func eachNumericSource[T any](ctx context.Context, snapshot *dm01SourceSnapshot, table string, upper migration.SourceUpperBound, callbackPresent bool, list func(int64, int32) ([]T, error), emit func(T) error) error {
	if err := snapshot.validateBound(table, upper, callbackPresent); err != nil {
		return err
	}
	if upper.Empty {
		return nil
	}
	upperKey, err := strconv.ParseInt(upper.SourceKey, 10, 64)
	if err != nil || upperKey < 1 {
		return migration.ErrSourceSchemaDrift
	}
	for offset := int32(0); ; offset += dm01SourcePageSize {
		rows, err := list(upperKey, offset)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := emit(row); err != nil {
				return err
			}
		}
		if len(rows) < int(dm01SourcePageSize) {
			return nil
		}
	}
}

func (snapshot *dm01SourceSnapshot) EachMergeAudit(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.MergeAuditRow) error) error {
	return eachNumericSource(ctx, snapshot, "crm_user_identity_merge_audit", upper, fn != nil,
		func(key int64, offset int32) ([]legacysourcedb.ListDM01MergeAuditRow, error) {
			return snapshot.queries.ListDM01MergeAudit(ctx, legacysourcedb.ListDM01MergeAuditParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: key, PageSize: dm01SourcePageSize, PageOffset: offset})
		},
		func(row legacysourcedb.ListDM01MergeAuditRow) error {
			if !row.CreatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			return fn(migration.MergeAuditRow{ID: row.ID, CreatedAt: row.CreatedAt.Time.UTC(), Payload: row.Payload})
		})
}

func (snapshot *dm01SourceSnapshot) EachResolutionQueue(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.ResolutionQueueRow) error) error {
	return eachNumericSource(ctx, snapshot, "crm_user_identity_resolution_queue", upper, fn != nil,
		func(key int64, offset int32) ([]legacysourcedb.ListDM01ResolutionQueueRow, error) {
			return snapshot.queries.ListDM01ResolutionQueue(ctx, legacysourcedb.ListDM01ResolutionQueueParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: key, PageSize: dm01SourcePageSize, PageOffset: offset})
		},
		func(row legacysourcedb.ListDM01ResolutionQueueRow) error {
			if !row.UpdatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			return fn(migration.ResolutionQueueRow{ID: row.ID, UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload})
		})
}

func (snapshot *dm01SourceSnapshot) EachDirectoryMember(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.DirectoryMemberRow) error) error {
	return eachNumericSource(ctx, snapshot, "admin_wecom_directory_members", upper, fn != nil,
		func(key int64, offset int32) ([]legacysourcedb.ListDM01DirectoryMemberRow, error) {
			return snapshot.queries.ListDM01DirectoryMember(ctx, legacysourcedb.ListDM01DirectoryMemberParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: key, PageSize: dm01SourcePageSize, PageOffset: offset})
		},
		func(row legacysourcedb.ListDM01DirectoryMemberRow) error {
			if !row.LastSyncedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			return fn(migration.DirectoryMemberRow{ID: row.ID, LastSyncedAt: row.LastSyncedAt.Time.UTC(), Payload: row.Payload})
		})
}

func (snapshot *dm01SourceSnapshot) EachContact(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.ContactRow) error) error {
	return eachNumericSource(ctx, snapshot, "contacts", upper, fn != nil,
		func(key int64, offset int32) ([]legacysourcedb.ListDM01ContactRow, error) {
			return snapshot.queries.ListDM01Contact(ctx, legacysourcedb.ListDM01ContactParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: key, PageSize: dm01SourcePageSize, PageOffset: offset})
		},
		func(row legacysourcedb.ListDM01ContactRow) error {
			if !row.UpdatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			return fn(migration.ContactRow{ID: row.ID, UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload})
		})
}

func (snapshot *dm01SourceSnapshot) EachIdentityConflict(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.IdentityConflictRow) error) error {
	return eachNumericSource(ctx, snapshot, "crm_user_identity_conflicts", upper, fn != nil,
		func(key int64, offset int32) ([]legacysourcedb.ListDM01IdentityConflictRow, error) {
			return snapshot.queries.ListDM01IdentityConflict(ctx, legacysourcedb.ListDM01IdentityConflictParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: key, PageSize: dm01SourcePageSize, PageOffset: offset})
		},
		func(row legacysourcedb.ListDM01IdentityConflictRow) error {
			if !row.UpdatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			return fn(migration.IdentityConflictRow{ID: row.ID, UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload})
		})
}

func (snapshot *dm01SourceSnapshot) EachPerson(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.PersonRow) error) error {
	return eachNumericSource(ctx, snapshot, "people", upper, fn != nil,
		func(key int64, offset int32) ([]legacysourcedb.ListDM01PersonRow, error) {
			return snapshot.queries.ListDM01Person(ctx, legacysourcedb.ListDM01PersonParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: key, PageSize: dm01SourcePageSize, PageOffset: offset})
		},
		func(row legacysourcedb.ListDM01PersonRow) error {
			if !row.UpdatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			return fn(migration.PersonRow{ID: row.ID, UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload})
		})
}

func (snapshot *dm01SourceSnapshot) EachFollowUser(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.FollowUserRow) error) error {
	return eachNumericSource(ctx, snapshot, "wecom_external_contact_follow_users", upper, fn != nil,
		func(key int64, offset int32) ([]legacysourcedb.ListDM01FollowUserRow, error) {
			return snapshot.queries.ListDM01FollowUser(ctx, legacysourcedb.ListDM01FollowUserParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: key, PageSize: dm01SourcePageSize, PageOffset: offset})
		},
		func(row legacysourcedb.ListDM01FollowUserRow) error {
			if !row.UpdatedAt.Valid {
				return migration.ErrSourceSchemaDrift
			}
			return fn(migration.FollowUserRow{ID: row.ID, UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload})
		})
}

func (snapshot *dm01SourceSnapshot) EachExternalBinding(ctx context.Context, upper migration.SourceUpperBound, fn func(migration.ExternalBindingRow) error) error {
	if err := snapshot.validateBound("external_contact_bindings", upper, fn != nil); err != nil {
		return err
	}
	if upper.Empty {
		return nil
	}
	for offset := int32(0); ; offset += dm01SourcePageSize {
		rows, err := snapshot.queries.ListDM01ExternalBinding(ctx, legacysourcedb.ListDM01ExternalBindingParams{UpperWatermark: sourceTime(upper.Watermark), UpperKey: upper.SourceKey, PageSize: dm01SourcePageSize, PageOffset: offset})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if !row.UpdatedAt.Valid || row.ExternalUserid == "" {
				return migration.ErrSourceSchemaDrift
			}
			if err := fn(migration.ExternalBindingRow{ExternalUserID: row.ExternalUserid, UpdatedAt: row.UpdatedAt.Time.UTC(), Payload: row.Payload}); err != nil {
				return err
			}
		}
		if len(rows) < int(dm01SourcePageSize) {
			return nil
		}
	}
}

func sourceTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero()}
}

func (snapshot *dm01SourceSnapshot) validateBound(table string, upper migration.SourceUpperBound, callbackPresent bool) error {
	if snapshot == nil || snapshot.queries == nil || !callbackPresent || upper.Table != table {
		return migration.ErrSourceSchemaDrift
	}
	for _, captured := range snapshot.bounds {
		if captured == upper {
			if !upper.Empty && (upper.Watermark.IsZero() || upper.SourceKey == "") {
				return migration.ErrSourceSchemaDrift
			}
			return nil
		}
	}
	return migration.ErrSourceSchemaDrift
}
