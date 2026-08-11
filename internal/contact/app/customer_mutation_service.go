package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidCustomerMutation = errors.New("invalid customer mutation")
	ErrCustomerNotFound        = errors.New("customer not found")
	ErrCustomerTagNotFound     = errors.New("customer tag not found")
	ErrCustomerConflict        = errors.New("customer mutation conflict")
	ErrCustomerMutationFailed  = errors.New("customer mutation failed")
)

// NullablePatch distinguishes an omitted JSON property from an explicit null.
type NullablePatch[T any] struct {
	Set   bool
	Value *T
}

type CustomerUpdateCommand struct {
	ID           contactport.CustomerID
	Name         *string
	AvatarURL    NullablePatch[string]
	Gender       NullablePatch[int16]
	OwnerStaffID NullablePatch[int64]
	ChannelID    NullablePatch[int64]
	Extra        *json.RawMessage
	Actor        contactport.Actor
}

type CustomerStageCommand struct {
	ID      contactport.CustomerID
	StageID *int64
	Actor   contactport.Actor
}

type CustomerTagCommand struct {
	ID    contactport.CustomerID
	TagID int64
	Actor contactport.Actor
}

type CustomerStageMutation struct {
	Customer    CustomerRecord
	PreviousID  *int64
	StateChange bool
}

type CustomerEventAppend struct {
	CustomerID contactport.CustomerID
	EventType  string
	Payload    json.RawMessage
	Actor      contactport.Actor
	OccurredAt time.Time
}

// CustomerMutationStore is contact-internal and requires a transaction-bound
// context for every method.
type CustomerMutationStore interface {
	UpdateCustomer(context.Context, CustomerUpdateCommand) (CustomerRecord, error)
	SetCustomerStage(context.Context, CustomerStageCommand) (CustomerStageMutation, error)
	AddCustomerTag(context.Context, CustomerTagCommand) (bool, error)
	RemoveCustomerTag(context.Context, CustomerTagCommand) (bool, error)
	AppendCustomerEvent(context.Context, CustomerEventAppend) (contactport.EventID, error)
}

type CustomerMutationService struct {
	uow         platformport.UnitOfWork
	store       CustomerMutationStore
	events      eventport.Appender
	now         func() time.Time
	newEventKey func() (string, error)
}

func NewCustomerMutationService(
	uow platformport.UnitOfWork,
	store CustomerMutationStore,
	events eventport.Appender,
) *CustomerMutationService {
	return &CustomerMutationService{
		uow: uow, store: store, events: events,
		now: time.Now, newEventKey: randomEventKey,
	}
}

func (service *CustomerMutationService) Update(
	ctx context.Context,
	command CustomerUpdateCommand,
) (customer CustomerRecord, err error) {
	if err = validateCustomerUpdate(command); err != nil {
		return CustomerRecord{}, err
	}
	if err = service.ready(); err != nil {
		return CustomerRecord{}, err
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		occurredAt, key, prepareErr := service.eventMetadata("customer.updated")
		if prepareErr != nil {
			return prepareErr
		}
		customer, err = service.store.UpdateCustomer(txCtx, command)
		if err != nil {
			return err
		}
		payload, marshalErr := json.Marshal(struct {
			CustomerID contactport.CustomerID `json:"customer_id"`
			Actor      contactport.Actor      `json:"actor"`
		}{CustomerID: command.ID, Actor: command.Actor})
		if marshalErr != nil {
			return marshalErr
		}
		return service.appendEvents(txCtx, command.ID, "customer.updated", payload, command.Actor, occurredAt, key)
	})
	if err != nil {
		return CustomerRecord{}, errors.Join(ErrCustomerMutationFailed, err)
	}
	return customer, nil
}

func (service *CustomerMutationService) SetStage(
	ctx context.Context,
	command CustomerStageCommand,
) (customer CustomerRecord, err error) {
	if err = validateCustomerStage(command); err != nil {
		return CustomerRecord{}, err
	}
	if err = service.ready(); err != nil {
		return CustomerRecord{}, err
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		mutation, storeErr := service.store.SetCustomerStage(txCtx, command)
		if storeErr != nil {
			return storeErr
		}
		customer = mutation.Customer
		if !mutation.StateChange {
			return nil
		}
		occurredAt, key, prepareErr := service.eventMetadata("customer.stage_changed")
		if prepareErr != nil {
			return prepareErr
		}
		payload, marshalErr := json.Marshal(struct {
			CustomerID contactport.CustomerID `json:"customer_id"`
			FromStage  *int64                 `json:"from_stage_id"`
			ToStage    *int64                 `json:"to_stage_id"`
			Actor      contactport.Actor      `json:"actor"`
		}{CustomerID: command.ID, FromStage: mutation.PreviousID, ToStage: command.StageID, Actor: command.Actor})
		if marshalErr != nil {
			return marshalErr
		}
		return service.appendEvents(txCtx, command.ID, "customer.stage_changed", payload, command.Actor, occurredAt, key)
	})
	if err != nil {
		return CustomerRecord{}, errors.Join(ErrCustomerMutationFailed, err)
	}
	return customer, nil
}

func (service *CustomerMutationService) AddTag(ctx context.Context, command CustomerTagCommand) error {
	return service.mutateTag(ctx, command, true)
}

func (service *CustomerMutationService) RemoveTag(ctx context.Context, command CustomerTagCommand) error {
	return service.mutateTag(ctx, command, false)
}

func (service *CustomerMutationService) mutateTag(
	ctx context.Context,
	command CustomerTagCommand,
	add bool,
) error {
	if err := validateCustomerTag(command); err != nil {
		return err
	}
	if err := service.ready(); err != nil {
		return err
	}
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var changed bool
		var storeErr error
		if add {
			changed, storeErr = service.store.AddCustomerTag(txCtx, command)
		} else {
			changed, storeErr = service.store.RemoveCustomerTag(txCtx, command)
		}
		if storeErr != nil || !changed {
			return storeErr
		}
		eventType := "customer.tag_removed"
		if add {
			eventType = "customer.tag_applied"
		}
		occurredAt, key, prepareErr := service.eventMetadata(eventType)
		if prepareErr != nil {
			return prepareErr
		}
		payload, marshalErr := json.Marshal(struct {
			CustomerID contactport.CustomerID `json:"customer_id"`
			TagID      int64                  `json:"tag_id"`
			Actor      contactport.Actor      `json:"actor"`
		}{CustomerID: command.ID, TagID: command.TagID, Actor: command.Actor})
		if marshalErr != nil {
			return marshalErr
		}
		return service.appendEvents(txCtx, command.ID, eventType, payload, command.Actor, occurredAt, key)
	})
	if err != nil {
		return errors.Join(ErrCustomerMutationFailed, err)
	}
	return nil
}

func (service *CustomerMutationService) appendEvents(
	ctx context.Context,
	customerID contactport.CustomerID,
	eventType string,
	payload json.RawMessage,
	actor contactport.Actor,
	occurredAt time.Time,
	eventKey string,
) error {
	if _, err := service.store.AppendCustomerEvent(ctx, CustomerEventAppend{
		CustomerID: customerID, EventType: eventType, Payload: payload,
		Actor: actor, OccurredAt: occurredAt,
	}); err != nil {
		return err
	}
	_, err := service.events.Append(ctx, eventport.Event{
		Type: eventType, CustomerID: eventport.CustomerID(customerID), Payload: payload,
		OccurredAt: occurredAt, IdempotencyKey: eventKey,
	})
	return err
}

func (service *CustomerMutationService) eventMetadata(eventType string) (time.Time, string, error) {
	occurredAt := service.now().UTC()
	if occurredAt.IsZero() {
		return time.Time{}, "", errors.New("customer mutation clock is invalid")
	}
	suffix, err := service.newEventKey()
	if err != nil || suffix == "" {
		return time.Time{}, "", errors.Join(errors.New("customer mutation event key is invalid"), err)
	}
	return occurredAt, eventType + ":" + suffix, nil
}

func (service *CustomerMutationService) ready() error {
	if service == nil || service.uow == nil || service.store == nil || service.events == nil ||
		service.now == nil || service.newEventKey == nil {
		return ErrCustomerMutationFailed
	}
	return nil
}

func validateCustomerUpdate(command CustomerUpdateCommand) error {
	if command.ID <= 0 || !validCustomerActor(command.Actor) ||
		(command.Name == nil && !command.AvatarURL.Set && !command.Gender.Set &&
			!command.OwnerStaffID.Set && !command.ChannelID.Set && command.Extra == nil) {
		return ErrInvalidCustomerMutation
	}
	if command.Name != nil && !utf8.ValidString(*command.Name) {
		return ErrInvalidCustomerMutation
	}
	if command.AvatarURL.Set && command.AvatarURL.Value != nil && !validAvatarURL(*command.AvatarURL.Value) {
		return ErrInvalidCustomerMutation
	}
	for _, value := range []*int64{command.OwnerStaffID.Value, command.ChannelID.Value} {
		if value != nil && *value <= 0 {
			return ErrInvalidCustomerMutation
		}
	}
	if command.Extra != nil && !validJSONObject(*command.Extra) {
		return ErrInvalidCustomerMutation
	}
	return nil
}

func validateCustomerStage(command CustomerStageCommand) error {
	if command.ID <= 0 || !validCustomerActor(command.Actor) ||
		(command.StageID != nil && *command.StageID <= 0) {
		return ErrInvalidCustomerMutation
	}
	return nil
}

func validateCustomerTag(command CustomerTagCommand) error {
	if command.ID <= 0 || command.TagID <= 0 || !validCustomerActor(command.Actor) {
		return ErrInvalidCustomerMutation
	}
	return nil
}

func validCustomerActor(actor contactport.Actor) bool {
	value := string(actor)
	return value != "" && len(value) <= 200 && strings.TrimSpace(value) == value && utf8.ValidString(value)
}

func validAvatarURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Host != "" && parsed.User == nil
}
