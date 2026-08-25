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

func TestCH02ProviderConstructionIsDisabledAndNetworkInert(t *testing.T) {
	provider, err := newChannelAcquisitionAssetProvider(appconfig.WeComCustomerAcquisition{}, nil, nil)
	if err != nil || provider != nil {
		t.Fatalf("disabled provider=%v err=%v", provider, err)
	}
	if provider, err = newChannelAcquisitionAssetProvider(appconfig.WeComCustomerAcquisition{Enabled: true}, http.DefaultClient, time.Now); provider != nil || err == nil {
		t.Fatalf("unconfirmed provider=%v err=%v", provider, err)
	}

	for key, value := range map[string]string{
		"AICRM_DATABASE_URL":                                    "postgres://db/aicrm",
		"AICRM_WORKER_PGX_MAX_CONNS":                            "9",
		"AICRM_RIVER_CRITICAL_MAX_WORKERS":                      "2",
		"AICRM_RIVER_EVENT_MAX_WORKERS":                         "1",
		"AICRM_RIVER_OUTBOUND_MAX_WORKERS":                      "1",
		"AICRM_RIVER_SYNC_MAX_WORKERS":                          "1",
		"AICRM_RIVER_HEAVY_MAX_WORKERS":                         "1",
		"AICRM_RIVER_AI_MAX_WORKERS":                            "1",
		"AICRM_WECOM_CUSTOMER_ACQUISITION_ENABLED":              "true",
		"AICRM_WECOM_CUSTOMER_ACQUISITION_CORP_ID":              "ch02-corp",
		"AICRM_WECOM_CUSTOMER_ACQUISITION_SECRET":               "ch02-secret-must-not-leak",
		"AICRM_WECOM_CUSTOMER_ACQUISITION_PERMISSION_CONFIRMED": "true",
	} {
		t.Setenv(key, value)
	}
	config, err := appconfig.Load(appruntime.RoleWorker)
	if err != nil {
		t.Fatal(err)
	}
	var networkCalls atomic.Int32
	httpClient := &http.Client{Transport: roundTripCounter{calls: &networkCalls}}
	provider, err = newChannelAcquisitionAssetProvider(config.WeCom.CustomerAcquisition, httpClient, time.Now)
	if err != nil || provider == nil || networkCalls.Load() != 0 {
		t.Fatalf("enabled provider=%v calls=%d err=%v", provider, networkCalls.Load(), err)
	}
	if formatted := strings.Join([]string{strings.TrimSpace(config.WeCom.CustomerAcquisition.Secret.String())}, ""); formatted != "[REDACTED]" {
		t.Fatalf("secret formatting=%q", formatted)
	}
}

type roundTripCounter struct{ calls *atomic.Int32 }

func (counter roundTripCounter) RoundTrip(*http.Request) (*http.Response, error) {
	counter.calls.Add(1)
	return nil, errInvalidChannelAcquisitionAssetConfig
}
