package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrIdentityUpsertFailed = errors.New("identity upsert failed")

type UpsertStore interface {
	UpsertNormalized(context.Context, NormalizedIdentity) (identityID int64, created bool, err error)
}

type UpsertResult struct {
	IdentityID int64
	Created    bool
}

// UpsertService creates one floating identity or returns the existing OneID
// edge. The event is appended only for an insert and shares its UoW.
type UpsertService struct {
	uow    platformport.UnitOfWork
	store  UpsertStore
	events eventport.Appender
}

func NewUpsertService(uow platformport.UnitOfWork, store UpsertStore, events eventport.Appender) *UpsertService {
	return &UpsertService{uow: uow, store: store, events: events}
}

func (service *UpsertService) Upsert(ctx context.Context, ref identityport.IDRef) (UpsertResult, error) {
	normalized, err := Normalize(ref)
	if err != nil {
		return UpsertResult{}, err
	}
	if service == nil || service.uow == nil || service.store == nil || service.events == nil || ctx == nil {
		return UpsertResult{}, ErrIdentityUpsertFailed
	}

	var result UpsertResult
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		identityID, created, err := service.store.UpsertNormalized(txCtx, normalized)
		if err != nil {
			return err
		}
		if identityID <= 0 {
			return ErrIdentityUpsertFailed
		}
		if created {
			payload, err := json.Marshal(struct {
				IdentityID        int64               `json:"identity_id"`
				Kind              identityport.IDKind `json:"kind"`
				Scope             string              `json:"scope"`
				NormalizerVersion int16               `json:"normalizer_version"`
			}{identityID, normalized.Kind, normalized.Scope, normalized.NormalizerVersion})
			if err != nil {
				return err
			}
			if _, err = service.events.Append(txCtx, eventport.Event{
				Type:           "identity.created",
				Payload:        payload,
				OccurredAt:     time.Now().UTC(),
				IdempotencyKey: "identity.created:" + strconv.FormatInt(identityID, 10),
			}); err != nil {
				return err
			}
		}
		result = UpsertResult{IdentityID: identityID, Created: created}
		return nil
	})
	if err != nil {
		return UpsertResult{}, errors.Join(ErrIdentityUpsertFailed, err)
	}
	return result, nil
}
