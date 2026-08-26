package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	outboundprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/provider"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

var errInvalidWeComOutboundConfig = errors.New("invalid WeCom outbound configuration")

func newWeComOutboundProvider(config appconfig.WeComOutbound, httpClient *http.Client, now func() time.Time, resolve func(context.Context, int64) (string, string, bool, error)) (*outboundprovider.WeComPrivateMessageProvider, error) {
	if !config.Enabled {
		return nil, nil
	}
	if !config.PermissionConfirmed || httpClient == nil || now == nil || resolve == nil {
		return nil, errInvalidWeComOutboundConfig
	}
	credentials, err := wecomclient.NewCredentials(config.CorpID, config.Secret.Value())
	if err != nil {
		return nil, errors.Join(errInvalidWeComOutboundConfig, err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
		BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: httpClient, Now: now,
	})
	if err != nil {
		return nil, errors.Join(errInvalidWeComOutboundConfig, err)
	}
	client, err := wecomclient.NewCustomerAcquisitionClient(wecomclient.CustomerAcquisitionClientConfig{
		BaseURL: wecomclient.ProductionBaseURL, HTTPClient: httpClient, TokenProvider: tokens,
	})
	if err != nil {
		return nil, errors.Join(errInvalidWeComOutboundConfig, err)
	}
	provider, err := outboundprovider.NewWeComPrivateMessageProvider(client, resolve)
	if err != nil {
		return nil, errors.Join(errInvalidWeComOutboundConfig, err)
	}
	return provider, nil
}
