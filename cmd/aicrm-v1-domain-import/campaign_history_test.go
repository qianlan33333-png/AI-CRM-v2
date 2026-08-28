package main

import (
	"strings"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestCampaignHistoryRequiresDM01BeforeDatabaseAccess(t *testing.T) {
	if !validDomain("campaign-history") {
		t.Fatal("missing explicit history domain")
	}
	for _, test := range []struct{ name, key, run string }{
		{"missing run", strings.Repeat("k", 32), "0"}, {"missing key", "", "2"}, {"short key", "short", "2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", test.key)
			err := run([]string{"--domain=campaign-history", "--archive-run-id=frozen", "--dm01-run-id=" + test.run}, appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres://127.0.0.1:1/must-not-connect", ArchiveKey: strings.Repeat("a", 32)})
			if err == nil || !strings.Contains(err.Error(), "frozen DM01 source HMAC key") {
				t.Fatalf("expected DM01 precondition, got %v", err)
			}
		})
	}
}

func TestCampaignHistoryScopedJournals(t *testing.T) {
	journals, err := newCampaignHistoryJournals("frozen")
	if err != nil || len(journals) != 5 {
		t.Fatalf("journals: %v", err)
	}
	if _, err = v1domain.NewCampaignHistoryJournal(journals["public/campaign_segments"], journals["public/campaign_members"], journals["public/cloud_broadcast_plans"], journals["public/cloud_broadcast_plan_recipients"], journals["public/cloud_broadcast_plan_recipient_messages"]); err != nil {
		t.Fatal(err)
	}
	if _, err = newCampaignHistoryJournals(""); err == nil {
		t.Fatal("empty archive run accepted")
	}
}
