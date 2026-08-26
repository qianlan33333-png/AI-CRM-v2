package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outboundprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/provider"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	wecomprofile "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/profile"
)

var errInvalidWeComProfileRuntime = errors.New("invalid WeCom profile runtime")

type weComProfileEffectQueuer interface {
	QueueInTransaction(context.Context, wecomprofile.QueueCommand) (wecomprofile.Acceptance, error)
}

type sidebarWeComProfileEffect struct {
	targets contactport.WeComOutboundTargetResolver
	effects weComProfileEffectQueuer
}

func (effect *sidebarWeComProfileEffect) QueueInTransaction(ctx context.Context, command contactapp.SidebarProfileEffectCommand) (contactapp.SidebarProfileEffectAcceptance, error) {
	if effect == nil || effect.targets == nil || effect.effects == nil || ctx == nil || command.ReceiptID < 1 || command.ActorID < 1 || command.CustomerID < 1 {
		return contactapp.SidebarProfileEffectAcceptance{}, errInvalidWeComProfileRuntime
	}
	staffUserID, externalUserID, resolved, err := effect.targets.Resolve(ctx, int64(command.CustomerID))
	if err != nil || !resolved {
		return contactapp.SidebarProfileEffectAcceptance{}, errors.Join(errInvalidWeComProfileRuntime, err)
	}
	acceptance, err := effect.effects.QueueInTransaction(ctx, wecomprofile.QueueCommand{
		LegacyReceiptID: command.ReceiptID,
		Actor:           command.ActorID,
		IdempotencyKey:  command.IdempotencyKey,
		StaffUserID:     staffUserID,
		ExternalUserID:  externalUserID,
		Remark:          command.Profile.Name,
		Description:     command.Profile.Description,
	})
	if err != nil || acceptance.State != eer.StateQueued || acceptance.EffectID == "" || acceptance.QueueReceiptID == "" || acceptance.RiverJobID < 1 || acceptance.RealExternalCallExecuted {
		return contactapp.SidebarProfileEffectAcceptance{}, errors.Join(errInvalidWeComProfileRuntime, err)
	}
	return contactapp.SidebarProfileEffectAcceptance{Queued: true, ProviderExecutionEligible: true}, nil
}

type profileTokenAdapter struct {
	provider *wecomclient.CachingTokenProvider
}

func (adapter profileTokenAdapter) Token(ctx context.Context) (string, error) {
	token, err := adapter.provider.Token(ctx)
	return token.Value(), err
}

func (adapter profileTokenAdapter) RefreshToken(ctx context.Context) (string, error) {
	token, err := adapter.provider.RefreshToken(ctx)
	return token.Value(), err
}

func newWeComContactProfileWriter(config appconfig.WeComOutbound, httpClient *http.Client, now func() time.Time) (wecomport.ContactProfileWriter, error) {
	if !config.Enabled {
		return nil, nil
	}
	if !config.PermissionConfirmed || httpClient == nil || now == nil {
		return nil, errInvalidWeComProfileRuntime
	}
	credentials, err := wecomclient.NewCredentials(config.CorpID, config.Secret.Value())
	if err != nil {
		return nil, errors.Join(errInvalidWeComProfileRuntime, err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
		BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: httpClient, Now: now,
	})
	if err != nil {
		return nil, errors.Join(errInvalidWeComProfileRuntime, err)
	}
	writer, err := outboundprovider.NewWeComContactProfileClient(outboundprovider.WeComContactProfileClientConfig{
		BaseURL: wecomclient.ProductionBaseURL, HTTPClient: httpClient, Token: profileTokenAdapter{provider: tokens},
	})
	if err != nil {
		return nil, errors.Join(errInvalidWeComProfileRuntime, err)
	}
	return writer, nil
}

var _ contactapp.SidebarProfileEffect = (*sidebarWeComProfileEffect)(nil)
