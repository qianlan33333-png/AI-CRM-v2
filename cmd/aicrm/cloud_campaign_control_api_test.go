package main

import (
	"os"
	"strings"
	"testing"
)

func TestCloudCampaignControlRoutesStayExplicitAndGated(t *testing.T) {
	source, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	registrations := string(source)
	for _, exact := range []string{
		`{http.MethodGet, "/api/admin/cloud-orchestrator/audit", authport.CapabilityAdminRead, false, http.HandlerFunc(wrapper.ListCloudOrchestratorAudit)}`,
		`{http.MethodPost, "/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/recipients/{customer_id}/dispatch", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.DispatchOutboundCampaignRecipient)}`,
	} {
		if strings.Count(registrations, exact) != 1 {
			t.Fatalf("route registration count for %q != 1", exact)
		}
	}
	if strings.Contains(registrations, `/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/recipients/{customer_id}/send`) {
		t.Fatal("ungated recipient send alias is registered")
	}
}
