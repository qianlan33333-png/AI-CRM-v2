package main

import (
	"errors"
	"net/http"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
	wecomtag "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
)

var errInvalidWeComTagCatalogConfig = errors.New("invalid WeCom tag catalog configuration")

func newWeComTagCatalogProvider(config appconfig.WeComTagCatalog, httpClient *http.Client, now func() time.Time) (*wecomtag.CatalogProvider, error) {
	if !config.Enabled {
		return nil, nil
	}
	if !config.PermissionConfirmed || httpClient == nil || now == nil {
		return nil, errInvalidWeComTagCatalogConfig
	}
	credentials, err := wecomclient.NewCredentials(config.CorpID, config.Secret.Value())
	if err != nil {
		return nil, errors.Join(errInvalidWeComTagCatalogConfig, err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
		BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: httpClient, Now: now,
	})
	if err != nil {
		return nil, errors.Join(errInvalidWeComTagCatalogConfig, err)
	}
	reader, err := wecomclient.NewTagCatalogClient(wecomclient.TagCatalogClientConfig{
		BaseURL: wecomclient.ProductionBaseURL, HTTPClient: httpClient, TokenProvider: tokens,
	})
	if err != nil {
		return nil, errors.Join(errInvalidWeComTagCatalogConfig, err)
	}
	provider, err := wecomtag.NewCatalogProvider(reader)
	if err != nil {
		return nil, errors.Join(errInvalidWeComTagCatalogConfig, err)
	}
	return provider, nil
}

func weComTagEffectCorpID(config appconfig.Root) string {
	if config.WeCom.TagCatalog.CorpID != "" {
		return config.WeCom.TagCatalog.CorpID
	}
	if config.WeCom.OAuth.CorpID != "" {
		return config.WeCom.OAuth.CorpID
	}
	return config.WeCom.Callback.CorpID
}
