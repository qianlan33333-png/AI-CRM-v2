package main

import (
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
)

func newWeChatShopOrderProviderRuntime(config appconfig.WeChatShopOrderProvider, client orderprovider.HTTPDoer) (*orderprovider.WeChatShopOrder, error) {
	if !config.Enabled || !config.PermissionConfirmed {
		return nil, orderprovider.ErrInvalidProviderConfig
	}
	credential, err := orderprovider.NewWeChatShopCredential(config.AppID, config.AppSecret.Value())
	if err != nil {
		return nil, err
	}
	return orderprovider.NewWeChatShopOrder(orderprovider.WeChatShopOrderConfig{
		APIBaseURL: orderprovider.WeChatShopProductionBaseURL,
		Credential: credential,
	}, client)
}
