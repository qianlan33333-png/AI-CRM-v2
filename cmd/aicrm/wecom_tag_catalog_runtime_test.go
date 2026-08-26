package main

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

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
