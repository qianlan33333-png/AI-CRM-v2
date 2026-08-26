package outboundhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type audienceSendRecordHTTPFixture struct {
	page   outboundapp.AudienceSendRecordPage
	record outboundport.AudienceSendRecord
}

func (fixture *audienceSendRecordHTTPFixture) List(context.Context, int64, int32, int32) (outboundapp.AudienceSendRecordPage, error) {
	return fixture.page, nil
}
func (fixture *audienceSendRecordHTTPFixture) Get(context.Context, int64, int64) (outboundport.AudienceSendRecord, error) {
	return fixture.record, nil
}

func TestAudienceSendRecordHTTPListAndDetailArePIIClosed(t *testing.T) {
	now := time.Date(2026, 8, 26, 7, 0, 0, 0, time.UTC)
	record := outboundport.AudienceSendRecord{ID: 41, State: outbound.CampaignDispatchReconciled, TechnicalAttemptCount: 1, ProviderResultReceived: true, ReceiptPresent: true, DeliveryProven: true, BusinessCallDispatched: true, RealExternalCallExecuted: true, CreatedAt: now, UpdatedAt: now}
	handler, err := NewAudienceSendRecordHandler(&audienceSendRecordHTTPFixture{page: outboundapp.AudienceSendRecordPage{Items: []outboundport.AudienceSendRecord{record}, Total: 1, Limit: 20}, record: record})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"/api/admin/ai-audience/packages/7/send-records?limit=20&offset=0",
		"/api/admin/ai-audience/packages/7/send-records/campaign:41",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request = authorizedCampaignHandoffRequest(t, request, authport.CapabilitySegmentsRead)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"provider_result_received":true`) || !strings.Contains(response.Body.String(), `"delivery_proven":true`) {
			t.Fatalf("target=%s status=%d body=%s", target, response.Code, response.Body.String())
		}
		for _, forbidden := range []string{"external_userid", "sender_userid", "provider_message_id", "raw_response", "unionid", "mobile"} {
			if strings.Contains(response.Body.String(), forbidden) {
				t.Fatalf("target=%s leaked %q: %s", target, forbidden, response.Body.String())
			}
		}
	}
}

func TestAudienceSendRecordHTTPRejectsUnsafeInputAndAuthorization(t *testing.T) {
	handler, _ := NewAudienceSendRecordHandler(&audienceSendRecordHTTPFixture{})
	for _, test := range []struct {
		target     string
		capability authport.Capability
		status     int
	}{
		{target: "/api/admin/ai-audience/packages/07/send-records", capability: authport.CapabilitySegmentsRead, status: http.StatusBadRequest},
		{target: "/api/admin/ai-audience/packages/7/send-records?limit=101", capability: authport.CapabilitySegmentsRead, status: http.StatusBadRequest},
		{target: "/api/admin/ai-audience/packages/7/send-records?limit=20&limit=30", capability: authport.CapabilitySegmentsRead, status: http.StatusBadRequest},
		{target: "/api/admin/ai-audience/packages/7/send-records/campaign:41?refresh=true", capability: authport.CapabilitySegmentsRead, status: http.StatusBadRequest},
		{target: "/api/admin/ai-audience/packages/7/send-records", capability: authport.CapabilityOperationsRead, status: http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodGet, test.target, nil)
		request = authorizedCampaignHandoffRequest(t, request, test.capability)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("target=%s capability=%s status=%d body=%s", test.target, test.capability, response.Code, response.Body.String())
		}
	}
}
