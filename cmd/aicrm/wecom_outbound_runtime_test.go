package main

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func TestWeComOutboundProviderConstructionIsExplicitAndNetworkInert(t *testing.T) {
	provider, err := newWeComOutboundProvider(appconfig.WeComOutbound{}, nil, nil, nil)
	if err != nil || provider != nil {
		t.Fatalf("disabled provider=%v err=%v", provider, err)
	}
	if provider, err = newWeComOutboundProvider(appconfig.WeComOutbound{Enabled: true}, http.DefaultClient, time.Now, func(context.Context, int64) (string, string, bool, error) {
		return "", "", false, nil
	}); provider != nil || err == nil {
		t.Fatalf("partial provider=%v err=%v", provider, err)
	}

	for key, value := range map[string]string{
		"AICRM_DATABASE_URL":                        "postgres://db/aicrm",
		"AICRM_WORKER_PGX_MAX_CONNS":                "9",
		"AICRM_RIVER_CRITICAL_MAX_WORKERS":          "2",
		"AICRM_RIVER_EVENT_MAX_WORKERS":             "1",
		"AICRM_RIVER_OUTBOUND_MAX_WORKERS":          "1",
		"AICRM_RIVER_SYNC_MAX_WORKERS":              "1",
		"AICRM_RIVER_HEAVY_MAX_WORKERS":             "1",
		"AICRM_RIVER_AI_MAX_WORKERS":                "1",
		"AICRM_WECOM_OUTBOUND_ENABLED":              "true",
		"AICRM_WECOM_OUTBOUND_CORP_ID":              "outbound-corp",
		"AICRM_WECOM_OUTBOUND_SECRET":               "outbound-secret-must-not-leak",
		"AICRM_WECOM_OUTBOUND_PERMISSION_CONFIRMED": "true",
	} {
		t.Setenv(key, value)
	}
	config, err := appconfig.Load(appruntime.RoleWorker)
	if err != nil {
		t.Fatal(err)
	}
	var networkCalls atomic.Int32
	httpClient := &http.Client{Transport: roundTripCounter{calls: &networkCalls}}
	provider, err = newWeComOutboundProvider(config.WeCom.Outbound, httpClient, time.Now, func(context.Context, int64) (string, string, bool, error) {
		return "owner-1", "external-1", true, nil
	})
	if err != nil || provider == nil || networkCalls.Load() != 0 {
		t.Fatalf("provider=%v calls=%d err=%v", provider, networkCalls.Load(), err)
	}
	if formatted := strings.TrimSpace(config.WeCom.Outbound.Secret.String()); formatted != "[REDACTED]" {
		t.Fatalf("secret format=%q", formatted)
	}
}
