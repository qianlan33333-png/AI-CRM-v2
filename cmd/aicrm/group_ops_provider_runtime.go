package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	outboundprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/provider"
	wecomruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

var errInvalidGroupOpsProviderConfig = errors.New("invalid Group Ops WeCom provider configuration")

func newGroupOpsTokens(config appconfig.WeComOutbound, httpClient *http.Client, now func() time.Time) (*wecomclient.CachingTokenProvider, error) {
	if !config.Enabled || !config.PermissionConfirmed || httpClient == nil || now == nil {
		return nil, errInvalidGroupOpsProviderConfig
	}
	credentials, err := wecomclient.NewCredentials(config.CorpID, config.Secret.Value())
	if err != nil {
		return nil, errors.Join(errInvalidGroupOpsProviderConfig, err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
		BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: httpClient, Now: now,
	})
	if err != nil {
		return nil, errors.Join(errInvalidGroupOpsProviderConfig, err)
	}
	return tokens, nil
}

func newGroupOpsDispatchProvider(config appconfig.WeComOutbound, httpClient *http.Client, now func() time.Time, receipts groupopsport.GroupMessageReceiptWriter) (groupopsport.DispatchProvider, error) {
	tokens, err := newGroupOpsTokens(config, httpClient, now)
	if err != nil {
		return nil, err
	}
	client, err := outboundprovider.NewWeComGroupMessageClient(outboundprovider.WeComGroupMessageClientConfig{BaseURL: wecomclient.ProductionBaseURL, HTTPClient: httpClient, Token: groupOpsTokenAdapter{provider: tokens}})
	if err != nil {
		return nil, errors.Join(errInvalidGroupOpsProviderConfig, err)
	}
	provider, err := outboundprovider.NewWeComGroupMessageProvider(client, receipts)
	if err != nil {
		return nil, errors.Join(errInvalidGroupOpsProviderConfig, err)
	}
	return provider, nil
}

type groupOpsTokenAdapter struct {
	provider *wecomclient.CachingTokenProvider
}

func (adapter groupOpsTokenAdapter) Token(ctx context.Context) (string, error) {
	token, err := adapter.provider.Token(ctx)
	return token.Value(), err
}

func (adapter groupOpsTokenAdapter) RefreshToken(ctx context.Context) (string, error) {
	token, err := adapter.provider.RefreshToken(ctx)
	return token.Value(), err
}

func newGroupOpsEvidenceVerifier(config appconfig.WeComOAuth, httpClient *http.Client, now func() time.Time, receipts groupopsport.GroupMessageReceiptReader) (groupopsport.ReconciliationEvidenceVerifier, error) {
	if !config.Enabled || httpClient == nil || now == nil {
		return nil, errInvalidGroupOpsProviderConfig
	}
	credentials, err := wecomclient.NewCredentials(config.CorpID, config.Secret.Value())
	if err != nil {
		return nil, errors.Join(errInvalidGroupOpsProviderConfig, err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: httpClient, Now: now})
	if err != nil {
		return nil, err
	}
	client, err := wecomruntime.NewGroupMessageReadClient(wecomruntime.GroupMessageReadClientConfig{BaseURL: wecomclient.ProductionBaseURL, HTTPClient: httpClient, Token: tokens})
	if err != nil {
		return nil, errors.Join(errInvalidGroupOpsProviderConfig, err)
	}
	verifier, err := wecomruntime.NewGroupMessageReconciliationVerifier(client, receipts)
	if err != nil {
		return nil, errors.Join(errInvalidGroupOpsProviderConfig, err)
	}
	return verifier, nil
}

type disabledGroupOpsDispatchProvider struct{}

func (disabledGroupOpsDispatchProvider) Dispatch(_ context.Context, _ groupopsport.DispatchRequest) (groupopsport.DispatchProviderResult, error) {
	return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchPreDispatchFailure}, nil
}
