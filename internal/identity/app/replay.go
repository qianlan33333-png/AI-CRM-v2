package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrPendingReplayFailed = errors.New("identity pending replay failed")

type PendingReplayStatus string

const (
	PendingReplayIdle      PendingReplayStatus = "idle"
	PendingReplayRetryable PendingReplayStatus = "retryable"
	PendingReplayCompleted PendingReplayStatus = "completed"
)

type PendingReplayIdentity struct {
	ID       int64
	Identity NormalizedIdentity
}

type PendingReplay struct {
	ID             int64
	Kind           string
	Identities     []PendingReplayIdentity
	EventType      string
	Payload        json.RawMessage
	Source         string
	IdempotencyKey string
	OccurredAt     time.Time
	Version        int64
}

type PendingReplayResult struct {
	Status         PendingReplayStatus
	PendingEventID int64
	CustomerID     contactport.CustomerID
	EventID        contactport.EventID
}

type PendingReplayStore interface {
	ClaimPendingReplay(context.Context) (PendingReplay, bool, error)
	DeferPendingReplay(context.Context, int64, int64) error
	CompletePendingReplay(context.Context, int64, int64) error
}

type PendingReplayService struct {
	uow    platformport.UnitOfWork
	store  PendingReplayStore
	ingest *IngestService
}

func NewPendingReplayService(
	uow platformport.UnitOfWork,
	store PendingReplayStore,
	ingest *IngestService,
) *PendingReplayService {
	return &PendingReplayService{uow: uow, store: store, ingest: ingest}
}

func (service *PendingReplayService) ReplayOnce(ctx context.Context) (PendingReplayResult, error) {
	if service == nil || service.uow == nil || service.store == nil || service.ingest == nil ||
		service.ingest.store == nil || service.ingest.contacts == nil || service.ingest.events == nil || ctx == nil {
		return PendingReplayResult{}, ErrPendingReplayFailed
	}
	result := PendingReplayResult{Status: PendingReplayIdle}
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		pending, found, err := service.store.ClaimPendingReplay(txCtx)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		if err = validatePendingReplay(pending); err != nil {
			return err
		}
		refs := make([]normalizedIngestRef, 0, len(pending.Identities))
		for _, identity := range pending.Identities {
			refs = append(refs, normalizedIngestRef{Identity: identity.Identity})
		}
		command := normalizedIngestCommand{
			Refs: refs, EventType: pending.EventType, Payload: pending.Payload, Source: pending.Source,
			OccurredAt: pending.OccurredAt, IdempotencyKey: pending.IdempotencyKey,
		}
		attributed, err := service.ingest.attributeKnown(
			txCtx, command, "identity.pending.replay:"+strconv.FormatInt(pending.ID, 10),
		)
		if err != nil {
			return err
		}
		if attributed.Status != identityport.IngestAttributed {
			if err = service.store.DeferPendingReplay(txCtx, pending.ID, pending.Version); err != nil {
				return err
			}
			result = PendingReplayResult{Status: PendingReplayRetryable, PendingEventID: pending.ID}
			return nil
		}
		if err = service.store.CompletePendingReplay(txCtx, pending.ID, pending.Version); err != nil {
			return err
		}
		result = PendingReplayResult{
			Status: PendingReplayCompleted, PendingEventID: pending.ID,
			CustomerID: attributed.CustomerID, EventID: attributed.EventID,
		}
		return nil
	})
	if err != nil {
		return PendingReplayResult{}, errors.Join(ErrPendingReplayFailed, err)
	}
	return result, nil
}

func validatePendingReplay(pending PendingReplay) error {
	if pending.ID <= 0 || (pending.Kind != "attribution" && pending.Kind != "conflict") ||
		len(pending.Identities) == 0 || !validIngestText(pending.EventType, 200) ||
		!validIngestText(pending.Source, 200) || !validIngestText(pending.IdempotencyKey, 512) ||
		pending.OccurredAt.IsZero() || pending.Version <= 0 {
		return ErrPendingReplayFailed
	}
	if _, err := canonicalJSONObject(pending.Payload); err != nil {
		return ErrPendingReplayFailed
	}
	previousID := int64(0)
	for _, identity := range pending.Identities {
		if identity.ID <= previousID || ValidateNormalized(identity.Identity) != nil {
			return ErrPendingReplayFailed
		}
		previousID = identity.ID
	}
	return nil
}
