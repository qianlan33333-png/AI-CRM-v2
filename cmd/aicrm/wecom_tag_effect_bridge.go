package main

import (
	"context"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	wecomtag "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
)

type legacyTagSyncCommitApplication interface {
	RequestWithCommitHook(context.Context, contactapp.LegacyTagSyncCommand, func(context.Context, contactapp.LegacyTagSyncAcceptance, bool) error) (contactapp.LegacyTagSyncAcceptance, error)
}

type legacyTagLiveCommitApplication interface {
	RequestWithCommitHook(context.Context, contactapp.LegacyTagLiveMutationCommand, func(context.Context, contactapp.LegacyTagLiveMutationAcceptance, bool) error) (contactapp.LegacyTagLiveMutationAcceptance, error)
}

type wecomTagQueueInTransaction interface {
	QueueInTransaction(context.Context, wecomtag.QueueCommand) (wecomtag.Acceptance, error)
	ReplayInTransaction(context.Context, wecomtag.QueueCommand) (wecomtag.Acceptance, error)
}

type legacyTagSyncEffectBridge struct {
	legacy  legacyTagSyncCommitApplication
	effects wecomTagQueueInTransaction
}

func (bridge *legacyTagSyncEffectBridge) Request(ctx context.Context, command contactapp.LegacyTagSyncCommand) (contactapp.LegacyTagSyncAcceptance, wecomtag.Acceptance, error) {
	if bridge == nil || nilLegacyDependency(bridge.legacy) || nilLegacyDependency(bridge.effects) {
		return contactapp.LegacyTagSyncAcceptance{}, wecomtag.Acceptance{}, wecomtag.ErrEffectUnavailable
	}
	var effect wecomtag.Acceptance
	legacy, err := bridge.legacy.RequestWithCommitHook(ctx, command, func(txCtx context.Context, acceptance contactapp.LegacyTagSyncAcceptance, replay bool) error {
		trigger := wecomtag.SyncTriggerManual
		if command.Kind == contactapp.LegacyTagSyncDue {
			trigger = wecomtag.SyncTriggerDue
		}
		var queueErr error
		queue := bridge.effects.QueueInTransaction
		if replay {
			queue = bridge.effects.ReplayInTransaction
		}
		effect, queueErr = queue(txCtx, wecomtag.QueueCommand{
			LegacyReceiptID: acceptance.ReceiptID, Actor: command.Actor, IdempotencyKey: command.IdempotencyKey,
			Operation: wecomtag.OperationCatalogSync, SyncTrigger: trigger,
		})
		return queueErr
	})
	if err != nil {
		return contactapp.LegacyTagSyncAcceptance{}, wecomtag.Acceptance{}, err
	}
	return legacy, effect, nil
}

type legacyTagLiveEffectBridge struct {
	legacy  legacyTagLiveCommitApplication
	effects wecomTagQueueInTransaction
}

func (bridge *legacyTagLiveEffectBridge) Request(ctx context.Context, command contactapp.LegacyTagLiveMutationCommand, externalUserID string, providerTagIDs []string) (contactapp.LegacyTagLiveMutationAcceptance, wecomtag.Acceptance, error) {
	if bridge == nil || nilLegacyDependency(bridge.legacy) || nilLegacyDependency(bridge.effects) || externalUserID == "" || len(providerTagIDs) == 0 {
		return contactapp.LegacyTagLiveMutationAcceptance{}, wecomtag.Acceptance{}, wecomtag.ErrInvalidCommand
	}
	operation := wecomtag.OperationMark
	if command.Operation == contactapp.LegacyTagLiveMutationUnmark {
		operation = wecomtag.OperationUnmark
	}
	var effect wecomtag.Acceptance
	legacy, err := bridge.legacy.RequestWithCommitHook(ctx, command, func(txCtx context.Context, acceptance contactapp.LegacyTagLiveMutationAcceptance, replay bool) error {
		var queueErr error
		queue := bridge.effects.QueueInTransaction
		if replay {
			queue = bridge.effects.ReplayInTransaction
		}
		effect, queueErr = queue(txCtx, wecomtag.QueueCommand{
			LegacyReceiptID: acceptance.ReceiptID, Actor: command.Actor, IdempotencyKey: command.IdempotencyKey,
			Operation: operation, ExternalUserID: externalUserID, ProviderTagIDs: append([]string(nil), providerTagIDs...),
		})
		return queueErr
	})
	if err != nil {
		return contactapp.LegacyTagLiveMutationAcceptance{}, wecomtag.Acceptance{}, err
	}
	return legacy, effect, nil
}

var (
	_ legacyTagSyncApplication         = (*legacyTagSyncEffectBridge)(nil)
	_ legacyTagLiveMutationApplication = (*legacyTagLiveEffectBridge)(nil)
)
