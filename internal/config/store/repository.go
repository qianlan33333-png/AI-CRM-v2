package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	configdb "github.com/qianlan33333-png/AI-CRM-v2/internal/config/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

type Audit struct {
	ID        int64
	Key       configport.Key
	OldValue  []byte
	NewValue  []byte
	UpdatedBy string
	RequestID string
	UpdatedAt time.Time
}

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) LockKey(ctx context.Context, key configport.Key) error {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return err
	}
	return queries.LockSettingKey(ctx, string(key))
}

func (repository *Repository) Get(ctx context.Context, key configport.Key) (configport.Setting, bool, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return configport.Setting{}, false, err
	}
	row, err := queries.GetSetting(ctx, string(key))
	if errors.Is(err, pgx.ErrNoRows) {
		return configport.Setting{}, false, nil
	}
	if err != nil {
		return configport.Setting{}, false, err
	}
	return settingFromRow(row), true, nil
}

func (repository *Repository) InsertAudit(ctx context.Context, oldValue []byte, command configport.SetCommand, canonical []byte, updatedAt time.Time) (Audit, bool, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return Audit{}, false, err
	}
	params := configdb.InsertSettingsAuditParams{
		Key: string(command.Key), NewValue: canonical, UpdatedBy: command.Actor,
		RequestID: command.RequestID, UpdatedAt: timestamp(updatedAt),
	}
	if oldValue != nil {
		params.OldValue = oldValue
	}
	row, err := queries.InsertSettingsAudit(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return Audit{}, false, nil
	}
	if err != nil {
		return Audit{}, false, err
	}
	return auditFromInsert(row), true, nil
}

func (repository *Repository) GetAuditByRequestID(ctx context.Context, requestID string) (Audit, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return Audit{}, err
	}
	row, err := queries.GetSettingsAuditByRequestID(ctx, requestID)
	if err != nil {
		return Audit{}, err
	}
	return auditFromGet(row), nil
}

func (repository *Repository) Upsert(ctx context.Context, command configport.SetCommand, canonical []byte, updatedAt time.Time) (configport.Setting, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return configport.Setting{}, err
	}
	row, err := queries.UpsertSetting(ctx, configdb.UpsertSettingParams{
		Key: string(command.Key), Value: canonical, UpdatedBy: command.Actor, UpdatedAt: timestamp(updatedAt),
	})
	if err != nil {
		return configport.Setting{}, err
	}
	return settingFromUpsert(row), nil
}

func queriesFromContext(ctx context.Context) (*configdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return configdb.New(tx), nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func settingFromRow(row configdb.Setting) configport.Setting {
	return configport.Setting{Key: configport.Key(row.Key), Value: row.Value, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt.Time}
}

func settingFromUpsert(row configdb.Setting) configport.Setting {
	return configport.Setting{Key: configport.Key(row.Key), Value: row.Value, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt.Time}
}

func auditFromInsert(row configdb.SettingsAudit) Audit {
	return Audit{ID: row.ID, Key: configport.Key(row.Key), OldValue: row.OldValue, NewValue: row.NewValue, UpdatedBy: row.UpdatedBy, RequestID: row.RequestID, UpdatedAt: row.UpdatedAt.Time}
}

func auditFromGet(row configdb.SettingsAudit) Audit {
	return Audit{ID: row.ID, Key: configport.Key(row.Key), OldValue: row.OldValue, NewValue: row.NewValue, UpdatedBy: row.UpdatedBy, RequestID: row.RequestID, UpdatedAt: row.UpdatedAt.Time}
}
