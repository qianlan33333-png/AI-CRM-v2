package main

import (
	"errors"
	"net/http"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

var errInvalidChannelAcquisitionAssetConfig = errors.New("invalid CH02 customer-acquisition configuration")

// newChannelAcquisitionAssetProvider is inert: token and Provider HTTP are
// deferred until a registered effect_id job executes. Disabled configuration
// returns before constructing any credential or network client.
func newChannelAcquisitionAssetProvider(config appconfig.WeComCustomerAcquisition, httpClient *http.Client, now func() time.Time) (*wecomclient.CustomerAcquisitionAssetProvider, error) {
	client, err := newChannelAcquisitionClient(config, httpClient, now)
	if err != nil || client == nil {
		return nil, err
	}
	provider, err := wecomclient.NewCustomerAcquisitionAssetProvider(client)
	if err != nil {
		return nil, errors.Join(errInvalidChannelAcquisitionAssetConfig, err)
	}
	return provider, nil
}

func newChannelAcquisitionClient(config appconfig.WeComCustomerAcquisition, httpClient *http.Client, now func() time.Time) (*wecomclient.CustomerAcquisitionClient, error) {
	if !config.Enabled {
		return nil, nil
	}
	if !config.PermissionConfirmed || httpClient == nil || now == nil {
		return nil, errInvalidChannelAcquisitionAssetConfig
	}
	credentials, err := wecomclient.NewCredentials(config.CorpID, config.Secret.Value())
	if err != nil {
		return nil, errors.Join(errInvalidChannelAcquisitionAssetConfig, err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
		BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: httpClient, Now: now,
	})
	if err != nil {
		return nil, errors.Join(errInvalidChannelAcquisitionAssetConfig, err)
	}
	client, err := wecomclient.NewCustomerAcquisitionClient(wecomclient.CustomerAcquisitionClientConfig{
		BaseURL: wecomclient.ProductionBaseURL, HTTPClient: httpClient, TokenProvider: tokens,
	})
	if err != nil {
		return nil, errors.Join(errInvalidChannelAcquisitionAssetConfig, err)
	}
	return client, nil
}
