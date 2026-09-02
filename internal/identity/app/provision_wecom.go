package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrVerifiedWeComProvisionFailed = errors.New("verified WeCom identity provision failed")

type VerifiedWeComProvisionStore interface {
	UpsertVerifiedWeCom(context.Context, NormalizedIdentity, string) (int64, ResolveRecord, error)
	BindNormalized(context.Context, NormalizedIdentity, int64) (BindRecord, error)
}

type VerifiedWeComProvisionService struct {
	uow      platformport.UnitOfWork
	store    VerifiedWeComProvisionStore
	contacts contactport.MergePort
	events   eventport.Appender
	now      func() time.Time
}

func NewVerifiedWeComProvisionService(uow platformport.UnitOfWork, store VerifiedWeComProvisionStore, contacts contactport.MergePort, events eventport.Appender) *VerifiedWeComProvisionService {
	return &VerifiedWeComProvisionService{uow: uow, store: store, contacts: contacts, events: events, now: time.Now}
}

// ResolveOrCreate turns one provider-verified external_userid into a
// channel-neutral customer. It intentionally leaves owner_staff_id empty;
// customer ownership and follow relationships are separate concerns.
func (service *VerifiedWeComProvisionService) ResolveOrCreate(ctx context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	normalized, err := Normalize(ref)
	if err != nil || ref.Kind != identityport.KindWeComExternalUserID || ref.Assurance != identityport.AssuranceVerified || ref.Source == "" {
		return identityport.ResolveResult{}, ErrVerifiedWeComProvisionFailed
	}
	if service == nil || service.uow == nil || service.store == nil || service.contacts == nil || service.events == nil || service.now == nil || ctx == nil {
		return identityport.ResolveResult{}, ErrVerifiedWeComProvisionFailed
	}

	var customerID contactport.CustomerID
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		identityID, current, upsertErr := service.store.UpsertVerifiedWeCom(txCtx, normalized, ref.Source)
		if upsertErr != nil || identityID < 1 || current.Conflict {
			return errors.Join(ErrVerifiedWeComProvisionFailed, upsertErr)
		}
		if current.CustomerID > 0 {
			customerID = contactport.CustomerID(current.CustomerID)
			return nil
		}

		created, createErr := service.contacts.CreateForIdentity(txCtx, contactport.CreateForIdentityCommand{
			Name: "企微客户", Actor: contactport.Actor(ref.Source),
		})
		if createErr != nil || created < 1 {
			return errors.Join(ErrVerifiedWeComProvisionFailed, createErr)
		}
		bound, bindErr := service.store.BindNormalized(txCtx, normalized, int64(created))
		if bindErr != nil || bound.Status != identityport.BindBound || bound.IdentityID != identityID {
			return errors.Join(ErrVerifiedWeComProvisionFailed, bindErr)
		}

		now := service.now().UTC().Truncate(time.Microsecond)
		payload, marshalErr := json.Marshal(struct {
			CustomerID contactport.CustomerID `json:"customer_id"`
			Source     string                 `json:"source"`
		}{created, ref.Source})
		if marshalErr != nil {
			return marshalErr
		}
		key := "sidebar.customer.provisioned:" + strconv.FormatInt(int64(created), 10)
		if _, appendErr := service.contacts.AppendExternalEvent(txCtx, contactport.ExternalEventCommand{
			CustomerID: created, EventType: eventport.EvCustomerAdded, Payload: payload,
			Actor: contactport.Actor(ref.Source), OccurredAt: now, IdempotencyKey: key,
		}); appendErr != nil {
			return appendErr
		}
		if _, appendErr := service.events.Append(txCtx, eventport.Event{
			Type: eventport.EvCustomerAdded, CustomerID: eventport.CustomerID(created), Payload: payload,
			OccurredAt: now, IdempotencyKey: key,
		}); appendErr != nil {
			return appendErr
		}
		boundPayload, marshalErr := json.Marshal(struct {
			IdentityID int64                  `json:"identity_id"`
			CustomerID contactport.CustomerID `json:"customer_id"`
		}{identityID, created})
		if marshalErr != nil {
			return marshalErr
		}
		if _, appendErr := service.events.Append(txCtx, eventport.Event{
			Type: "identity.bound", CustomerID: eventport.CustomerID(created), Payload: boundPayload,
			OccurredAt: now, IdempotencyKey: "identity.bound:" + strconv.FormatInt(identityID, 10),
		}); appendErr != nil {
			return appendErr
		}
		customerID = created
		return nil
	})
	if err != nil || customerID < 1 {
		return identityport.ResolveResult{}, errors.Join(ErrVerifiedWeComProvisionFailed, err)
	}
	return identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: customerID}, nil
}
