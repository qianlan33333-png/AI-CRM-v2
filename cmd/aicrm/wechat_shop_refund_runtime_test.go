package main

import (
	"errors"
	"net/http"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
)

func TestWeChatShopRefundRuntimeRequiresExplicitWorkerPermissionWithoutCallingProvider(t *testing.T) {
	client := weChatShopRuntimeHTTPDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("runtime construction called the real Provider")
		return nil, nil
	})
	configured := appconfig.WeChatShopRefundProvider{Enabled: true, PermissionConfirmed: false}
	if _, err := newWeChatShopRefundProviderRuntime(configured, client); !errors.Is(err, orderprovider.ErrInvalidProviderConfig) {
		t.Fatalf("unconfirmed runtime error = %v", err)
	}
}

func TestWeChatShopRefundCallbackRuntimeDisabledIsFailClosed(t *testing.T) {
	verifier, err := newWeChatShopRefundCallbackRuntime(appconfig.WeChatShopRefundProvider{})
	if err != nil || verifier == nil {
		t.Fatalf("disabled callback verifier = %#v, %v", verifier, err)
	}
}
