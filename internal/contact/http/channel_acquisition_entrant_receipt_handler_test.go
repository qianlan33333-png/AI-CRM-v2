package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type entrantReceiptServiceStub struct {
	command contactapp.ReconcileChannelAcquisitionEntrantReceiptCommand
	item    contactapp.ChannelAcquisitionEntrantReceiptItem
	page    contactapp.ChannelAcquisitionEntrantReceiptPage
	err     error
	calls   int
}

func (stub *entrantReceiptServiceStub) List(context.Context, contactapp.ChannelAcquisitionEntrantReceiptListInput) (contactapp.ChannelAcquisitionEntrantReceiptPage, error) {
	stub.calls++
	return stub.page, stub.err
}
func (stub *entrantReceiptServiceStub) Get(context.Context, int64, int64, int64) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	stub.calls++
	return stub.item, stub.err
}
func (stub *entrantReceiptServiceStub) Reconcile(_ context.Context, command contactapp.ReconcileChannelAcquisitionEntrantReceiptCommand) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	stub.calls++
	stub.command = command
	return stub.item, stub.err
}

func TestCH03EntrantReceiptReadIsSafeAndChannelScoped(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	stub := &entrantReceiptServiceStub{item: contactapp.ChannelAcquisitionEntrantReceiptItem{ReceiptID: 91, ChannelID: 41, EffectID: "eer_7", Kind: contactport.AcquisitionAssetQRCode, AssetVersion: 3, Status: contactport.ChannelAcquisitionEntrantPendingIdentity, OccurredAt: now, CreatedAt: now, UpdatedAt: now}}
	handler := mustEntrantReceiptFragment(t, stub)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/41/acquisition-entrant-receipts/91", "", authport.CapabilityChannelsRead))
	if response.Code != http.StatusOK || stub.calls != 1 {
		t.Fatalf("status/calls/body=%d/%d/%s", response.Code, stub.calls, response.Body.String())
	}
	for _, forbidden := range []string{"corp_id", "callback", "external_user", "input_digest", "correlation", "provider", "raw"} {
		if strings.Contains(strings.ToLower(response.Body.String()), forbidden) {
			t.Fatalf("unsafe field %q in %s", forbidden, response.Body.String())
		}
	}
	stub.err = contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
	masked := httptest.NewRecorder()
	handler.ServeHTTP(masked, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/99/acquisition-entrant-receipts/91", "", authport.CapabilityChannelsRead))
	if masked.Code != http.StatusNotFound {
		t.Fatalf("masked=%d body=%s", masked.Code, masked.Body.String())
	}
}

func TestCH03EntrantReceiptReadExposesUnboundDispositionWithoutAssetFields(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	stub := &entrantReceiptServiceStub{item: contactapp.ChannelAcquisitionEntrantReceiptItem{ReceiptID: 92, Status: contactport.ChannelAcquisitionEntrantIgnored, OccurredAt: now, CreatedAt: now, UpdatedAt: now}}
	handler := mustEntrantReceiptFragment(t, stub)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/41/acquisition-entrant-receipts/92", "", authport.CapabilityChannelsRead))
	if response.Code != http.StatusOK || stub.calls != 1 {
		t.Fatalf("status/calls/body=%d/%d/%s", response.Code, stub.calls, response.Body.String())
	}
	for _, absent := range []string{"channel_id", "effect_id", "kind", "asset_version"} {
		if strings.Contains(response.Body.String(), `"`+absent+`"`) {
			t.Fatalf("unbound response exposed %q: %s", absent, response.Body.String())
		}
	}
}

func TestCH03EntrantReceiptReconcileRequiresWriteCSRFFixedBodyAndIdempotency(t *testing.T) {
	stub := &entrantReceiptServiceStub{item: contactapp.ChannelAcquisitionEntrantReceiptItem{ReceiptID: 91, ChannelID: 41, EffectID: "eer_7", Kind: contactport.AcquisitionAssetQRCode, AssetVersion: 3, Status: contactport.ChannelAcquisitionEntrantReconciled, CustomerID: 22, CustomerEventID: 16, OccurredAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	handler := mustEntrantReceiptFragment(t, stub)
	badBody := httptest.NewRecorder()
	handler.ServeHTTP(badBody, acquisitionAssetRequest(http.MethodPost, "/api/admin/channels/41/acquisition-entrant-receipts/91/reconcile", `{"effect_id":"eer_7","customer_id":22,"reason":"verified","corp_id":"corp-a"}`, authport.CapabilityChannelsWrite))
	if badBody.Code != http.StatusUnprocessableEntity || stub.calls != 0 {
		t.Fatalf("bad body=%d calls=%d", badBody.Code, stub.calls)
	}
	valid := httptest.NewRecorder()
	handler.ServeHTTP(valid, acquisitionAssetRequest(http.MethodPost, "/api/admin/channels/41/acquisition-entrant-receipts/91/reconcile", `{"effect_id":"eer_7","customer_id":22,"reason":"verified"}`, authport.CapabilityChannelsWrite))
	if valid.Code != http.StatusOK || stub.calls != 1 || stub.command.ActorID != 41 || stub.command.ChannelID != 41 || stub.command.ReceiptID != 91 || stub.command.IdempotencyKey != "channel-acquisition-key-0001" {
		t.Fatalf("valid=%d calls=%d command=%+v body=%s", valid.Code, stub.calls, stub.command, valid.Body.String())
	}
	wrong := httptest.NewRecorder()
	handler.ServeHTTP(wrong, acquisitionAssetRequest(http.MethodPost, "/api/admin/channels/41/acquisition-entrant-receipts/91/reconcile", `{"effect_id":"eer_7","customer_id":22,"reason":"verified"}`, authport.CapabilityChannelsRead))
	if wrong.Code != http.StatusForbidden || stub.calls != 1 {
		t.Fatalf("wrong=%d calls=%d", wrong.Code, stub.calls)
	}
}

func TestCH03EntrantReceiptNonReconcileableStateReturnsConflict(t *testing.T) {
	stub := &entrantReceiptServiceStub{err: contactapp.ErrChannelAcquisitionEntrantReceiptConflict}
	handler := mustEntrantReceiptFragment(t, stub)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, acquisitionAssetRequest(http.MethodPost, "/api/admin/channels/41/acquisition-entrant-receipts/92/reconcile", `{"effect_id":"eer_7","customer_id":22,"reason":"verified"}`, authport.CapabilityChannelsWrite))
	if response.Code != http.StatusConflict || stub.calls != 1 {
		t.Fatalf("status/calls/body=%d/%d/%s", response.Code, stub.calls, response.Body.String())
	}
}

func mustEntrantReceiptFragment(t *testing.T, service channelAcquisitionEntrantReceiptService) http.Handler {
	t.Helper()
	handler, err := NewChannelAcquisitionEntrantReceiptHandler(service, &channelAcquisitionCSRFStub{})
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := NewChannelAcquisitionEntrantReceiptRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
}
