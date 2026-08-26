package main

import (
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
)

func newWeChatShopRefundProviderRuntime(config appconfig.WeChatShopRefundProvider, client orderprovider.HTTPDoer) (*orderprovider.WeChatShopOrder, error) {
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

func newWeChatShopRefundCallbackRuntime(config appconfig.WeChatShopRefundProvider) (orderport.WeChatShopRefundCallbackVerifier, error) {
	if !config.Enabled {
		return orderprovider.DisabledWeChatShopCallbackVerifier{}, nil
	}
	credential, err := orderprovider.NewWeChatShopCallbackCredential(config.AppID, config.CallbackToken.Value(), config.CallbackAESKey.Value())
	if err != nil {
		return nil, err
	}
	return orderprovider.NewWeChatShopCallbackVerifier(credential)
}
