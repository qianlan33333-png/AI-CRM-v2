package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type acquisitionAssetCommandStub struct {
	publishCommand   contactapp.PublishChannelAcquisitionAssetCommand
	reconcileCommand contactapp.ReconcileCurrentChannelAcquisitionAssetCommand
	publishResult    contactapp.ChannelAcquisitionAssetAcceptance
	reconcileResult  contactapp.ChannelAcquisitionAssetReconciliation
	calls            int
}

func (stub *acquisitionAssetCommandStub) Publish(_ context.Context, command contactapp.PublishChannelAcquisitionAssetCommand) (contactapp.ChannelAcquisitionAssetAcceptance, error) {
	stub.calls++
	stub.publishCommand = command
	return stub.publishResult, nil
}
func (stub *acquisitionAssetCommandStub) ReconcileCurrent(_ context.Context, command contactapp.ReconcileCurrentChannelAcquisitionAssetCommand) (contactapp.ChannelAcquisitionAssetReconciliation, error) {
	stub.calls++
	stub.reconcileCommand = command
	return stub.reconcileResult, nil
}

type acquisitionAssetQueryStub struct {
	item  contactapp.ChannelAcquisitionAssetItem
	page  contactapp.ChannelAcquisitionAssetPage
	err   error
	calls int
}

func (stub *acquisitionAssetQueryStub) Get(context.Context, int64, string) (contactapp.ChannelAcquisitionAssetItem, error) {
	stub.calls++
	return stub.item, stub.err
}
func (stub *acquisitionAssetQueryStub) List(context.Context, contactapp.ChannelAcquisitionAssetListInput) (contactapp.ChannelAcquisitionAssetPage, error) {
	stub.calls++
	return stub.page, stub.err
}

func TestCH02AcquisitionAssetPublishRequiresWriteCSRFAndReturnsSafeReceipt(t *testing.T) {
	commands := &acquisitionAssetCommandStub{publishResult: contactapp.ChannelAcquisitionAssetAcceptance{EffectID: "eer_7", ChannelID: 41, Kind: contactport.AcquisitionAssetQRCode, AssetVersion: 1, State: eer.StateQueued, AcceptReceiptID: "eerop_8", QueueReceiptID: "eerop_9"}}
	handler := mustAcquisitionAssetFragment(t, commands, &acquisitionAssetQueryStub{}, &channelAcquisitionCSRFStub{})
	request := acquisitionAssetRequest(http.MethodPost, "/api/admin/channels/41/acquisition-assets", `{"kind":"contact_way_qrcode"}`, authport.CapabilityChannelsWrite)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || commands.calls != 1 || commands.publishCommand.ChannelID != 41 || commands.publishCommand.Actor != 41 || commands.publishCommand.IdempotencyKey != "channel-acquisition-key-0001" {
		t.Fatalf("status/calls/command/body=%d/%d/%+v/%s", response.Code, commands.calls, commands.publishCommand, response.Body.String())
	}
	for _, forbidden := range []string{"corp", "correlation", "provider", "asset_url", "token", "secret", "river_job"} {
		if strings.Contains(strings.ToLower(response.Body.String()), forbidden) {
			t.Fatalf("unsafe field %q in %s", forbidden, response.Body.String())
		}
	}
	wrong := acquisitionAssetRequest(http.MethodPost, "/api/admin/channels/41/acquisition-assets", `{"kind":"contact_way_qrcode"}`, authport.CapabilityChannelsRead)
	wrongResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongResponse, wrong)
	if wrongResponse.Code != http.StatusForbidden || commands.calls != 1 {
		t.Fatalf("wrong capability status/calls=%d/%d", wrongResponse.Code, commands.calls)
	}
}

func TestCH02AcquisitionAssetReadMasksUnknownAndNeverLeaksSensitiveFields(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	queries := &acquisitionAssetQueryStub{item: contactapp.ChannelAcquisitionAssetItem{EffectID: "eer_7", ChannelID: 41, Kind: contactport.AcquisitionAssetLink, AssetVersion: 2, SupersedesVersion: 1, State: eer.StateOutcomeUnknown, AcceptReceiptID: "eerop_1", QueueReceiptID: "eerop_2", AttemptReceiptDigest: eer.Digest("sha256:" + strings.Repeat("a", 64)), CreatedAt: now, UpdatedAt: now}}
	handler := mustAcquisitionAssetFragment(t, &acquisitionAssetCommandStub{}, queries, &channelAcquisitionCSRFStub{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/41/acquisition-assets/eer_7", "", authport.CapabilityChannelsRead))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"entrant_ready":false`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"corp_id", "correlation", "provider_asset", "asset_url", "raw_payload"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("unsafe field %q in %s", forbidden, response.Body.String())
		}
	}
	queries.item.State = eer.StateExecuted
	withoutURL := httptest.NewRecorder()
	handler.ServeHTTP(withoutURL, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/41/acquisition-assets/eer_7", "", authport.CapabilityChannelsRead))
	if withoutURL.Code != http.StatusOK || !strings.Contains(withoutURL.Body.String(), `"entrant_ready":false`) || strings.Contains(withoutURL.Body.String(), `"asset_url"`) {
		t.Fatalf("executed-without-url status/body=%d/%s", withoutURL.Code, withoutURL.Body.String())
	}
	queries.item.AssetURL = "https://work.weixin.qq.com/ca/link-safe"
	executed := httptest.NewRecorder()
	handler.ServeHTTP(executed, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/41/acquisition-assets/eer_7", "", authport.CapabilityChannelsRead))
	if executed.Code != http.StatusOK || !strings.Contains(executed.Body.String(), `"asset_url":"https://work.weixin.qq.com/ca/link-safe"`) {
		t.Fatalf("executed status/body=%d/%s", executed.Code, executed.Body.String())
	}
	queries.err = contactapp.ErrChannelAcquisitionAssetNotFound
	notFound := httptest.NewRecorder()
	handler.ServeHTTP(notFound, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/99/acquisition-assets/eer_7", "", authport.CapabilityChannelsRead))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("masked status/body=%d/%s", notFound.Code, notFound.Body.String())
	}
}

func TestCH02AcquisitionAssetReconcileRejectsClientLeaseFields(t *testing.T) {
	commands := &acquisitionAssetCommandStub{reconcileResult: contactapp.ChannelAcquisitionAssetReconciliation{EffectID: "eer_7", State: eer.StateReconciled, Resolution: contactapp.ChannelAcquisitionAssetProviderApplied, ReceiptID: "eerop_10"}}
	handler := mustAcquisitionAssetFragment(t, commands, &acquisitionAssetQueryStub{}, &channelAcquisitionCSRFStub{})
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, acquisitionAssetRequest(http.MethodPost, "/api/admin/channels/41/acquisition-assets/eer_7/reconcile", `{"resolution":"provider_applied","evidence_digest":"sha256:`+strings.Repeat("a", 64)+`","generation":2}`, authport.CapabilityChannelsWrite))
	if invalid.Code != http.StatusUnprocessableEntity || commands.calls != 0 {
		t.Fatalf("invalid status/calls/body=%d/%d/%s", invalid.Code, commands.calls, invalid.Body.String())
	}
	valid := httptest.NewRecorder()
	handler.ServeHTTP(valid, acquisitionAssetRequest(http.MethodPost, "/api/admin/channels/41/acquisition-assets/eer_7/reconcile", `{"resolution":"provider_applied","evidence_digest":"sha256:`+strings.Repeat("a", 64)+`"}`, authport.CapabilityChannelsWrite))
	if valid.Code != http.StatusOK || commands.calls != 1 || commands.reconcileCommand.ChannelID != 41 || commands.reconcileCommand.EffectID != "eer_7" {
		t.Fatalf("valid status/calls/command/body=%d/%d/%+v/%s", valid.Code, commands.calls, commands.reconcileCommand, valid.Body.String())
	}
}

func acquisitionAssetRequest(method, path, body string, capability authport.Capability) *http.Request {
	request := channelAcquisitionRequest(method, body, capability)
	request.URL.Path = path
	return request
}

func mustAcquisitionAssetFragment(t *testing.T, commands channelAcquisitionAssetCommands, queries channelAcquisitionAssetQueries, csrf channelAcquisitionCSRFValidator) http.Handler {
	t.Helper()
	handler, err := NewChannelAcquisitionAssetHandler(commands, queries, csrf)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := NewChannelAcquisitionAssetRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
}

func TestCH02DisabledAcquisitionAssetFragmentFailsClosed(t *testing.T) {
	response := httptest.NewRecorder()
	NewDisabledChannelAcquisitionAssetRouteFragment().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/admin/channels/41/acquisition-assets", nil))
	if response.Code != http.StatusServiceUnavailable || !errors.Is(contactapp.ErrChannelAcquisitionAssetUnavailable, contactapp.ErrChannelAcquisitionAssetUnavailable) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
}
