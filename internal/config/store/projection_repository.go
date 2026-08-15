package store

import (
	"context"
	"time"

	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
)

type ProjectionSetting struct {
	Key            configport.Key
	Value          []byte
	UpdatedAt      time.Time
	LastActionType string
	LastModifiedBy string
	LastModifiedAt *time.Time
}

type ProjectionAudit struct {
	ID         int64
	Operator   string
	ActionType string
	TargetID   configport.Key
	CreatedAt  time.Time
}

func (repository *Repository) ListAppSettings(ctx context.Context) ([]ProjectionSetting, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAppSettingsProjection(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ProjectionSetting, 0, len(rows))
	for _, row := range rows {
		var modified *time.Time
		if row.LastModifiedAt.Valid {
			value := row.LastModifiedAt.Time
			modified = &value
		}
		result = append(result, ProjectionSetting{
			Key: configport.Key(row.Key), Value: row.Value,
			UpdatedAt: row.UpdatedAt.Time, LastActionType: row.LastActionType,
			LastModifiedBy: row.LastModifiedBy, LastModifiedAt: modified,
		})
	}
	return result, nil
}

func (repository *Repository) ListAppSettingsAudit(ctx context.Context) ([]ProjectionAudit, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAppSettingsAudit(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ProjectionAudit, 0, len(rows))
	for _, row := range rows {
		result = append(result, ProjectionAudit{ID: row.ID, Operator: row.Operator, ActionType: row.ActionType, TargetID: configport.Key(row.TargetID), CreatedAt: row.CreatedAt.Time})
	}
	return result, nil
}
