package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func TestWeComTagEffectCorpIDUsesSharedPriority(t *testing.T) {
	tests := []struct {
		name   string
		config appconfig.Root
		want   string
	}{
		{name: "tag catalog", config: appconfig.Root{WeCom: appconfig.WeCom{TagCatalog: appconfig.WeComTagCatalog{CorpID: "tag-corp"}, OAuth: appconfig.WeComOAuth{CorpID: "oauth-corp"}, Callback: appconfig.WeComCallback{CorpID: "callback-corp"}}}, want: "tag-corp"},
		{name: "oauth fallback", config: appconfig.Root{WeCom: appconfig.WeCom{OAuth: appconfig.WeComOAuth{CorpID: "oauth-corp"}, Callback: appconfig.WeComCallback{CorpID: "callback-corp"}}}, want: "oauth-corp"},
		{name: "callback fallback", config: appconfig.Root{WeCom: appconfig.WeCom{Callback: appconfig.WeComCallback{CorpID: "callback-corp"}}}, want: "callback-corp"},
		{name: "disabled", config: appconfig.Root{}, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := weComTagEffectCorpID(test.config); got != test.want {
				t.Fatalf("weComTagEffectCorpID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAPIConfigProjectsTagCatalogCorpIDWithoutSecret(t *testing.T) {
	const sentinel = "tag-secret-must-not-leak"
	for key, value := range map[string]string{
		"AICRM_DATABASE_URL": "postgres://db/aicrm", "AICRM_HTTP_LISTEN_ADDRESS": "127.0.0.1:8080",
		"AICRM_API_PGX_MAX_CONNS": "10", "AICRM_IDENTITY_HMAC_KEY": base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))),
		"AICRM_WECOM_TAG_CATALOG_ENABLED": "true", "AICRM_WECOM_TAG_CATALOG_CORP_ID": "tag-corp",
		"AICRM_WECOM_TAG_CATALOG_SECRET": sentinel, "AICRM_WECOM_TAG_CATALOG_PERMISSION_CONFIRMED": "true",
	} {
		t.Setenv(key, value)
	}
	config, err := appconfig.Load(appruntime.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	projection := config.WeCom.TagCatalog
	if !projection.Enabled || projection.CorpID != "tag-corp" || !projection.PermissionConfirmed || projection.Secret.Value() != "" {
		t.Fatalf("API tag catalog projection=%#v", projection)
	}
	if formatted := fmt.Sprintf("%#v", projection); strings.Contains(formatted, sentinel) {
		t.Fatalf("API projection exposed secret sentinel: %s", formatted)
	}
	if provider, providerErr := newWeComTagCatalogProvider(projection, http.DefaultClient, time.Now); providerErr == nil || provider != nil {
		t.Fatalf("API projection constructed provider=%v err=%v", provider, providerErr)
	}
}

func TestNewWeComTagCatalogProviderIsExplicitAndInert(t *testing.T) {
	provider, err := newWeComTagCatalogProvider(appconfig.WeComTagCatalog{}, nil, nil)
	if err != nil || provider != nil {
		t.Fatalf("disabled provider=%v err=%v", provider, err)
	}
	if provider, err = newWeComTagCatalogProvider(appconfig.WeComTagCatalog{Enabled: true}, http.DefaultClient, time.Now); err == nil || provider != nil {
		t.Fatalf("incomplete provider=%v err=%v", provider, err)
	}
	for key, value := range map[string]string{
		"AICRM_DATABASE_URL": "postgres://db/aicrm", "AICRM_WORKER_PGX_MAX_CONNS": "9",
		"AICRM_RIVER_CRITICAL_MAX_WORKERS": "2", "AICRM_RIVER_EVENT_MAX_WORKERS": "1", "AICRM_RIVER_OUTBOUND_MAX_WORKERS": "1",
		"AICRM_RIVER_SYNC_MAX_WORKERS": "1", "AICRM_RIVER_HEAVY_MAX_WORKERS": "1", "AICRM_RIVER_AI_MAX_WORKERS": "1",
		"AICRM_WECOM_TAG_CATALOG_ENABLED": "true", "AICRM_WECOM_TAG_CATALOG_CORP_ID": "tag-corp",
		"AICRM_WECOM_TAG_CATALOG_SECRET": "tag-secret-must-not-leak", "AICRM_WECOM_TAG_CATALOG_PERMISSION_CONFIRMED": "true",
	} {
		t.Setenv(key, value)
	}
	config, err := appconfig.Load(appruntime.RoleWorker)
	if err != nil {
		t.Fatal(err)
	}
	var networkCalls atomic.Int32
	provider, err = newWeComTagCatalogProvider(config.WeCom.TagCatalog, &http.Client{Transport: roundTripCounter{calls: &networkCalls}}, time.Now)
	if err != nil || provider == nil || networkCalls.Load() != 0 {
		t.Fatalf("enabled provider=%v calls=%d err=%v", provider, networkCalls.Load(), err)
	}
	if formatted := strings.TrimSpace(config.WeCom.TagCatalog.Secret.String()); formatted != "[REDACTED]" {
		t.Fatalf("secret formatting=%q", formatted)
	}
}
