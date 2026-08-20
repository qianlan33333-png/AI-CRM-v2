// Package app implements contact application services.
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
	GetStage(context.Context, contactport.StageID) (contactport.Stage, error)
	InsertStage(context.Context, contactport.CreateStageCommand) (contactport.Stage, error)
	RenameStage(context.Context, contactport.RenameStageCommand) (contactport.Stage, error)
	ReorderStages(context.Context, []contactport.StageID) ([]contactport.Stage, error)
	ArchiveStage(context.Context, contactport.ArchiveStageCommand, time.Time) (contactport.Stage, error)
	ReserveStageReceipt(context.Context, StageReceiptReservation) (StageReceipt, bool, error)
	CompleteStageReceipt(context.Context, int64, []contactport.StageID, time.Time) (StageReceipt, error)
}

type StageOperation string

const (
	stageOperationCreate  StageOperation = "create"
	stageOperationRename  StageOperation = "rename"
	stageOperationReorder StageOperation = "reorder"
	stageOperationArchive StageOperation = "archive"
)

type StageReceipt struct {
	ID            int64
	Operation     StageOperation
	Actor         contactport.Actor
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	State         string
	ResultIDs     []contactport.StageID
}

type StageReceiptReservation struct {
	Operation     StageOperation
	Actor         contactport.Actor
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type StageService struct {
	uow        platformport.UnitOfWork
	repository stageRepository
	events     eventport.Appender
	now        func() time.Time
}

var _ contactport.StageService = (*StageService)(nil)

func NewStageService(
	uow platformport.UnitOfWork,
	repository stageRepository,
	events eventport.Appender,
) *StageService {
	return &StageService{
		uow: uow, repository: repository, events: events,
		now: time.Now,
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
	stages, receiptErr := service.withStageReceipt(ctx, stageOperationCreate, command.Actor, command.IdempotencyKey, stagePayloadDigest(stageOperationCreate, command), func(txCtx context.Context) ([]contactport.Stage, error) {
		created, createErr := service.repository.InsertStage(txCtx, command)
		if createErr != nil {
			return nil, createErr
		}
		return []contactport.Stage{created}, nil
	})
	err = receiptErr
	if err != nil {
		return contactport.Stage{}, err
	}
	if len(stages) != 1 {
		return contactport.Stage{}, errors.New("stage create receipt result is invalid")
	}
	return stages[0], nil
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
	stages, receiptErr := service.withStageReceipt(ctx, stageOperationRename, command.Actor, command.IdempotencyKey, stagePayloadDigest(stageOperationRename, command), func(txCtx context.Context) ([]contactport.Stage, error) {
		renamed, renameErr := service.repository.RenameStage(txCtx, command)
		if renameErr != nil {
			return nil, renameErr
		}
		return []contactport.Stage{renamed}, nil
	})
	err = receiptErr
	if err != nil {
		return contactport.Stage{}, err
	}
	if len(stages) != 1 {
		return contactport.Stage{}, errors.New("stage rename receipt result is invalid")
	}
	return stages[0], nil
}

// ReorderStages accepts the complete current active set. The snapshot and
// update run inside one UoW so a client cannot accidentally omit a stage that
// was created concurrently.
func (service *StageService) ReorderStages(
	ctx context.Context,
	command contactport.ReorderStagesCommand,
) (stages []contactport.Stage, err error) {
	if err = validateReorderStages(command); err != nil {
		return nil, err
	}
	if err = service.ready(); err != nil {
		return nil, err
	}
	payloadDigest := stagePayloadDigest(stageOperationReorder, command)
	stages, err = service.withStageReceipt(ctx, stageOperationReorder, command.Actor, command.IdempotencyKey, payloadDigest, func(txCtx context.Context) ([]contactport.Stage, error) {
		active, listErr := service.repository.ListStages(txCtx)
		if listErr != nil {
			return nil, listErr
		}
		if !sameStageIDs(active, command.IDs) {
			return nil, contactport.ErrStageConflict
		}
		ordered, reorderErr := service.repository.ReorderStages(txCtx, command.IDs)
		if reorderErr != nil {
			return nil, reorderErr
		}
		if !sameStageIDs(ordered, command.IDs) {
			return nil, errors.New("stage reorder repository returned an incomplete result")
		}
		return ordered, nil
	})
	if err != nil {
		return nil, err
	}
	return stages, nil
}

// ArchiveStage is local-only. The repository must reject stages that still
// have customer references rather than clearing or rewriting customers.
func (service *StageService) ArchiveStage(
	ctx context.Context,
	command contactport.ArchiveStageCommand,
) (stage contactport.Stage, err error) {
	if err = validateArchiveStage(command); err != nil {
		return contactport.Stage{}, err
	}
	if err = service.ready(); err != nil {
		return contactport.Stage{}, err
	}
	payloadDigest := stagePayloadDigest(stageOperationArchive, command)
	var stages []contactport.Stage
	stages, err = service.withStageReceipt(ctx, stageOperationArchive, command.Actor, command.IdempotencyKey, payloadDigest, func(txCtx context.Context) ([]contactport.Stage, error) {
		archived, archiveErr := service.repository.ArchiveStage(txCtx, command, service.now().UTC())
		if archiveErr != nil {
			return nil, archiveErr
		}
		return []contactport.Stage{archived}, nil
	})
	if err != nil {
		return contactport.Stage{}, err
	}
	if len(stages) != 1 {
		return contactport.Stage{}, errors.New("stage archive receipt result is invalid")
	}
	return stages[0], nil
}

func validateCreateStage(command contactport.CreateStageCommand) (contactport.CreateStageCommand, error) {
	if !validStageText(command.Name) || !validStageText(string(command.Actor)) || !validStageIdempotencyKey(command.IdempotencyKey) {
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
	if command.ID <= 0 || !validStageText(command.Name) || !validStageText(string(command.Actor)) || !validStageIdempotencyKey(command.IdempotencyKey) {
		return contactport.ErrInvalidStage
	}
	return nil
}

func validateReorderStages(command contactport.ReorderStagesCommand) error {
	if !validStageText(string(command.Actor)) || !validStageIdempotencyKey(command.IdempotencyKey) || len(command.IDs) == 0 || len(command.IDs) > 1000 {
		return contactport.ErrInvalidStage
	}
	seen := make(map[contactport.StageID]struct{}, len(command.IDs))
	for _, id := range command.IDs {
		if id <= 0 {
			return contactport.ErrInvalidStage
		}
		if _, exists := seen[id]; exists {
			return contactport.ErrInvalidStage
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateArchiveStage(command contactport.ArchiveStageCommand) error {
	if command.ID <= 0 || !validStageText(string(command.Actor)) || !validStageIdempotencyKey(command.IdempotencyKey) {
		return contactport.ErrInvalidStage
	}
	return nil
}

func validStageIdempotencyKey(value string) bool {
	return len(value) >= 16 && len(value) <= 128 && strings.TrimSpace(value) == value
}

func stagePayloadDigest(operation StageOperation, command any) [32]byte {
	payload, _ := json.Marshal(struct {
		Operation StageOperation `json:"operation"`
		Command   any            `json:"command"`
	}{operation, command})
	return sha256.Sum256(payload)
}

func (service *StageService) withStageReceipt(ctx context.Context, operation StageOperation, actor contactport.Actor, key string, payloadDigest [32]byte, apply func(context.Context) ([]contactport.Stage, error)) ([]contactport.Stage, error) {
	if service.ready() != nil || apply == nil {
		return nil, errors.New("stage receipt dependencies are required")
	}
	now := service.now().UTC()
	if now.IsZero() {
		return nil, errors.New("stage service clock is invalid")
	}
	reservation := StageReceiptReservation{Operation: operation, Actor: actor, KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: payloadDigest, CreatedAt: now}
	var result []contactport.Stage
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, owned, reserveErr := service.repository.ReserveStageReceipt(txCtx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if receipt.ID <= 0 || receipt.Operation != operation || receipt.Actor != actor || receipt.KeyDigest != reservation.KeyDigest || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return contactport.ErrStageConflict
		}
		if !owned {
			if receipt.State != "completed" || len(receipt.ResultIDs) == 0 {
				return errors.New("incomplete stage receipt")
			}
			if operation == stageOperationReorder {
				var listErr error
				result, listErr = service.repository.ListStages(txCtx)
				if listErr != nil {
					return listErr
				}
				if !sameStageIDSlice(stageIDs(result), receipt.ResultIDs) {
					return contactport.ErrStageConflict
				}
				return nil
			}
			stage, getErr := service.repository.GetStage(txCtx, receipt.ResultIDs[0])
			if getErr != nil {
				return getErr
			}
			result = []contactport.Stage{stage}
			return nil
		}
		stages, applyErr := apply(txCtx)
		if applyErr != nil {
			return applyErr
		}
		ids := make([]contactport.StageID, len(stages))
		for index, stage := range stages {
			if stage.ID <= 0 {
				return errors.New("invalid stage receipt result")
			}
			ids[index] = stage.ID
		}
		payload, marshalErr := json.Marshal(struct {
			StageIDs []contactport.StageID `json:"stage_ids"`
			Actor    contactport.Actor     `json:"actor"`
		}{ids, actor})
		if marshalErr != nil {
			return marshalErr
		}
		if _, eventErr := service.events.Append(txCtx, eventport.Event{Type: stageEventType(operation), Payload: payload, OccurredAt: now, IdempotencyKey: "stage." + string(operation) + ":" + hex.EncodeToString(reservation.KeyDigest[:])}); eventErr != nil {
			return eventErr
		}
		completed, completeErr := service.repository.CompleteStageReceipt(txCtx, receipt.ID, ids, now)
		if completeErr != nil || completed.State != "completed" || !sameStageIDSlice(completed.ResultIDs, ids) {
			return errors.Join(errors.New("stage receipt completion failed"), completeErr)
		}
		result = stages
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func sameStageIDSlice(left, right []contactport.StageID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stageIDs(stages []contactport.Stage) []contactport.StageID {
	ids := make([]contactport.StageID, len(stages))
	for index, stage := range stages {
		ids[index] = stage.ID
	}
	return ids
}

func stageEventType(operation StageOperation) string {
	switch operation {
	case stageOperationCreate:
		return "stage.created"
	case stageOperationRename:
		return "stage.renamed"
	case stageOperationReorder:
		return "stage.reordered"
	default:
		return "stage.archived"
	}
}

func sameStageIDs(stages []contactport.Stage, ids []contactport.StageID) bool {
	if len(stages) != len(ids) {
		return false
	}
	seen := make(map[contactport.StageID]struct{}, len(stages))
	for _, stage := range stages {
		if stage.ID <= 0 {
			return false
		}
		seen[stage.ID] = struct{}{}
	}
	if len(seen) != len(stages) {
		return false
	}
	for _, id := range ids {
		if _, found := seen[id]; !found {
			return false
		}
	}
	return true
}

func validStageText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

func (service *StageService) ready() error {
	if service == nil || service.uow == nil || service.repository == nil || service.events == nil ||
		service.now == nil {
		return errors.New("stage service dependencies are required")
	}
	return nil
}

// randomEventKey remains the Contact package helper for unrelated customer
// mutations. Stage management commands deliberately use their receipt key.
func randomEventKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
