package main

import (
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"strings"
	"testing"
)

func TestCampaignDefinitionHistoryRequiresArchiveKeyBeforeConnecting(t *testing.T) {
	if !validDomain(campaignDefinitionHistoryDomain) || campaignDefinitionHistoryImportVersion == campaignHistoryImportVersion {
		t.Fatal("missing isolated history scope")
	}
	for _, mode := range []string{"import", "reconcile"} {
		err := run([]string{"--mode=" + mode, "--domain=" + campaignDefinitionHistoryDomain, "--archive-run-id=archive"}, appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres:///must-not-connect", ArchiveKey: strings.Repeat("k", 32)})
		if err == nil || !strings.Contains(err.Error(), "frozen archive source HMAC key") {
			t.Fatalf("%s did not reject missing key before connection: %v", mode, err)
		}
	}
}
