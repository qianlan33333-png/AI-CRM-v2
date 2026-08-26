package main

import (
	"errors"
	"net/http"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
)

func TestWeChatShopOrderProviderRuntimeRequiresWorkerPermissionWithoutCallingProvider(t *testing.T) {
	client := weChatShopRuntimeHTTPDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("runtime construction called the real Provider")
		return nil, nil
	})
	configured := appconfig.WeChatShopOrderProvider{Enabled: true, PermissionConfirmed: false}
	if _, err := newWeChatShopOrderProviderRuntime(configured, client); !errors.Is(err, orderprovider.ErrInvalidProviderConfig) {
		t.Fatalf("unconfirmed runtime error = %v", err)
	}
}

type weChatShopRuntimeHTTPDoer func(*http.Request) (*http.Response, error)

func (do weChatShopRuntimeHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}
