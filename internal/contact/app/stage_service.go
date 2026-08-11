// Package app implements contact application services.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type stageRepository interface {
	ListStages(context.Context) ([]contactport.Stage, error)
	InsertStage(context.Context, contactport.CreateStageCommand) (contactport.Stage, error)
	RenameStage(context.Context, contactport.RenameStageCommand) (contactport.Stage, error)
}

type StageService struct {
	uow         platformport.UnitOfWork
	repository  stageRepository
	events      eventport.Appender
	now         func() time.Time
	newEventKey func() (string, error)
}

var _ contactport.StageService = (*StageService)(nil)

func NewStageService(
	uow platformport.UnitOfWork,
	repository stageRepository,
	events eventport.Appender,
) *StageService {
	return &StageService{
		uow: uow, repository: repository, events: events,
		now: time.Now, newEventKey: randomEventKey,
	}
}

func (service *StageService) ListStages(ctx context.Context) (stages []contactport.Stage, err error) {
	if err = service.ready(); err != nil {
		return nil, err
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		stages, err = service.repository.ListStages(txCtx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return stages, err
}

func (service *StageService) CreateStage(
	ctx context.Context,
	command contactport.CreateStageCommand,
) (stage contactport.Stage, err error) {
	command, err = validateCreateStage(command)
	if err != nil {
		return contactport.Stage{}, err
	}
	if err = service.ready(); err != nil {
		return contactport.Stage{}, err
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		occurredAt, eventKey, prepareErr := service.eventMetadata("stage.created")
		if prepareErr != nil {
			return prepareErr
		}
		stage, err = service.repository.InsertStage(txCtx, command)
		if err != nil {
			return err
		}
		payload, marshalErr := json.Marshal(struct {
			StageID   contactport.StageID `json:"stage_id"`
			Name      string              `json:"name"`
			SortOrder int32               `json:"sort_order"`
			Actor     contactport.Actor   `json:"actor"`
		}{StageID: stage.ID, Name: stage.Name, SortOrder: stage.SortOrder, Actor: command.Actor})
		if marshalErr != nil {
			return marshalErr
		}
		_, err = service.events.Append(txCtx, eventport.Event{
			Type: "stage.created", Payload: payload,
			OccurredAt: occurredAt, IdempotencyKey: eventKey,
		})
		return err
	})
	if err != nil {
		return contactport.Stage{}, err
	}
	return stage, err
}

func (service *StageService) RenameStage(
	ctx context.Context,
	command contactport.RenameStageCommand,
) (stage contactport.Stage, err error) {
	if err = validateRenameStage(command); err != nil {
		return contactport.Stage{}, err
	}
	if err = service.ready(); err != nil {
		return contactport.Stage{}, err
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		occurredAt, eventKey, prepareErr := service.eventMetadata("stage.renamed")
		if prepareErr != nil {
			return prepareErr
		}
		stage, err = service.repository.RenameStage(txCtx, command)
		if err != nil {
			return err
		}
		payload, marshalErr := json.Marshal(struct {
			StageID contactport.StageID `json:"stage_id"`
			Name    string              `json:"name"`
			Actor   contactport.Actor   `json:"actor"`
		}{StageID: stage.ID, Name: stage.Name, Actor: command.Actor})
		if marshalErr != nil {
			return marshalErr
		}
		_, err = service.events.Append(txCtx, eventport.Event{
			Type: "stage.renamed", Payload: payload,
			OccurredAt: occurredAt, IdempotencyKey: eventKey,
		})
		return err
	})
	if err != nil {
		return contactport.Stage{}, err
	}
	return stage, err
}

func validateCreateStage(command contactport.CreateStageCommand) (contactport.CreateStageCommand, error) {
	if !validStageText(command.Name) || !validStageText(string(command.Actor)) {
		return contactport.CreateStageCommand{}, contactport.ErrInvalidStage
	}
	if len(command.Config) == 0 {
		command.Config = json.RawMessage(`{}`)
	}
	if !json.Valid(command.Config) {
		return contactport.CreateStageCommand{}, contactport.ErrInvalidStage
	}
	return command, nil
}

func validateRenameStage(command contactport.RenameStageCommand) error {
	if command.ID <= 0 || !validStageText(command.Name) || !validStageText(string(command.Actor)) {
		return contactport.ErrInvalidStage
	}
	return nil
}

func validStageText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

func (service *StageService) eventMetadata(eventType string) (time.Time, string, error) {
	occurredAt := service.now().UTC()
	if occurredAt.IsZero() {
		return time.Time{}, "", errors.New("stage service clock is invalid")
	}
	suffix, err := service.newEventKey()
	if err != nil {
		return time.Time{}, "", err
	}
	if suffix == "" {
		return time.Time{}, "", errors.New("stage service event key is empty")
	}
	return occurredAt, eventType + ":" + suffix, nil
}

func (service *StageService) ready() error {
	if service == nil || service.uow == nil || service.repository == nil || service.events == nil ||
		service.now == nil || service.newEventKey == nil {
		return errors.New("stage service dependencies are required")
	}
	return nil
}

func randomEventKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
