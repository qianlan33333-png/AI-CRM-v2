package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	config "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	configstore "github.com/qianlan33333-png/AI-CRM-v2/internal/config/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type repository interface {
	LockKey(context.Context, configport.Key) error
	Get(context.Context, configport.Key) (configport.Setting, bool, error)
	InsertAudit(context.Context, []byte, configport.SetCommand, []byte, time.Time) (configstore.Audit, bool, error)
	GetAuditByRequestID(context.Context, string) (configstore.Audit, error)
	Upsert(context.Context, configport.SetCommand, []byte, time.Time) (configport.Setting, error)
}

type Manager struct {
	uow    platformport.UnitOfWork
	repo   repository
	events eventport.Appender
	now    func() time.Time
}

var _ configport.Service = (*Manager)(nil)

func NewManager(uow platformport.UnitOfWork, repo repository, events eventport.Appender) *Manager {
	return &Manager{uow: uow, repo: repo, events: events, now: time.Now}
}

func (manager *Manager) Get(ctx context.Context, key configport.Key) (setting configport.Setting, err error) {
	if err = config.ValidateReadableSetting(key); err != nil {
		return configport.Setting{}, err
	}
	if err = manager.ready(); err != nil {
		return configport.Setting{}, err
	}
	err = manager.uow.Within(ctx, func(txCtx context.Context) error {
		var found bool
		setting, found, err = manager.repo.Get(txCtx, key)
		if err == nil && !found {
			err = configport.ErrSettingNotFound
		}
		return err
	})
	return setting, err
}

func (manager *Manager) Set(ctx context.Context, command configport.SetCommand) (setting configport.Setting, err error) {
	canonical, err := config.ValidateSetting(command.Key, command.Value)
	if err != nil {
		return configport.Setting{}, err
	}
	if strings.TrimSpace(command.Actor) == "" || len(command.Actor) > 200 ||
		strings.TrimSpace(command.RequestID) == "" || len(command.RequestID) > 200 {
		return configport.Setting{}, configport.ErrInvalidSetting
	}
	if err = manager.ready(); err != nil {
		return configport.Setting{}, err
	}
	updatedAt := manager.now().UTC()
	if updatedAt.IsZero() {
		return configport.Setting{}, fmt.Errorf("config manager clock is invalid")
	}
	err = manager.uow.Within(ctx, func(txCtx context.Context) error {
		if lockErr := manager.repo.LockKey(txCtx, command.Key); lockErr != nil {
			return lockErr
		}
		current, found, getErr := manager.repo.Get(txCtx, command.Key)
		if getErr != nil {
			return getErr
		}
		var oldValue []byte
		if found {
			oldValue = current.Value
		}
		audit, inserted, auditErr := manager.repo.InsertAudit(txCtx, oldValue, command, canonical, updatedAt)
		if auditErr != nil {
			return auditErr
		}
		if !inserted {
			audit, auditErr = manager.repo.GetAuditByRequestID(txCtx, command.RequestID)
			if auditErr != nil {
				return auditErr
			}
			if audit.Key != command.Key || audit.UpdatedBy != command.Actor || !bytes.Equal(audit.NewValue, canonical) {
				return configport.ErrIdempotencyConflict
			}
			setting = configport.Setting{
				Key: audit.Key, Value: audit.NewValue,
				UpdatedBy: audit.UpdatedBy, UpdatedAt: audit.UpdatedAt,
			}
			return nil
		}
		setting, err = manager.repo.Upsert(txCtx, command, canonical, updatedAt)
		if err != nil {
			return err
		}
		payload, marshalErr := json.Marshal(struct {
			AuditID int64          `json:"audit_id"`
			Key     configport.Key `json:"key"`
		}{AuditID: audit.ID, Key: command.Key})
		if marshalErr != nil {
			return marshalErr
		}
		_, err = manager.events.Append(txCtx, eventport.Event{
			Type: "setting.updated", Payload: payload, OccurredAt: updatedAt,
			IdempotencyKey: "setting.updated:" + command.RequestID,
		})
		return err
	})
	return setting, err
}

func (manager *Manager) ready() error {
	if manager == nil || manager.uow == nil || manager.repo == nil || manager.events == nil || manager.now == nil {
		return errors.New("config manager dependencies are required")
	}
	return nil
}
