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
)

type campaignDispatchHTTPApplication struct {
	recipientCommand outboundapp.CampaignRecipientDispatchCommand
}

func (*campaignDispatchHTTPApplication) Dispatch(context.Context, outboundapp.CampaignDispatchCommand) (outbound.CampaignDispatchSummary, error) {
	return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchUnavailable
}
func (*campaignDispatchHTTPApplication) Reconciliation(context.Context, string, string) (outbound.CampaignDispatchSummary, error) {
	return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchUnavailable
}
func (*campaignDispatchHTTPApplication) ManualReconcile(context.Context, outboundapp.CampaignDispatchReconcileCommand) (outbound.CampaignDispatchSummary, error) {
	return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchUnavailable
}
func (application *campaignDispatchHTTPApplication) DispatchRecipient(_ context.Context, command outboundapp.CampaignRecipientDispatchCommand) (outbound.CampaignDispatchSummary, error) {
	application.recipientCommand = command
	return outbound.CampaignDispatchSummary{HandoffID: 19, Queued: 1, UpdatedAt: time.Now().UTC()}, nil
}

func TestCampaignRecipientDispatchHTTPKeepsAcceptanceSeparateFromDelivery(t *testing.T) {
	application := &campaignDispatchHTTPApplication{}
	handler, err := NewCampaignDispatchHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	planID := "ctp_" + strings.Repeat("a", 64)
	request := httptest.NewRequest(http.MethodPost, "/dispatch-recipient", strings.NewReader(`{"external_gate":true}`))
	request.Header.Set("Idempotency-Key", "recipient-send-key")
	request = authorizedCampaignHandoffRequest(t, request, authport.CapabilityOperationsManage)
	response := httptest.NewRecorder()
	handler.DispatchRecipient(response, request, "spring-campaign", planID, 7)
	if response.Code != http.StatusOK || application.recipientCommand.CustomerID != 7 || !application.recipientCommand.ExternalGate {
		t.Fatalf("status=%d command=%+v body=%s", response.Code, application.recipientCommand, response.Body)
	}
	for _, expected := range []string{`"queued":1`, `"real_external_call_executed":false`, `"delivery_proven":false`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("missing %s in %s", expected, response.Body)
		}
	}
}

func TestCampaignRecipientDispatchHTTPRejectsDisabledActionGate(t *testing.T) {
	application := &campaignDispatchHTTPApplication{}
	handler, err := NewCampaignDispatchHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	planID := "ctp_" + strings.Repeat("a", 64)
	request := httptest.NewRequest(http.MethodPost, "/dispatch-recipient", strings.NewReader(`{"external_gate":false}`))
	request.Header.Set("Idempotency-Key", "recipient-disabled-key")
	request = authorizedCampaignHandoffRequest(t, request, authport.CapabilityOperationsManage)
	response := httptest.NewRecorder()
	handler.DispatchRecipient(response, request, "spring-campaign", planID, 7)
	if response.Code != http.StatusBadRequest || application.recipientCommand.CustomerID != 0 {
		t.Fatalf("status=%d command=%+v body=%s", response.Code, application.recipientCommand, response.Body)
	}
}
