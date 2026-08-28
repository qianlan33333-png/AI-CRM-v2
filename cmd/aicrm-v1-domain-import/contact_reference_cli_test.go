package main

import (
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestContactReferenceHistoryCLIRejectsSourceConnectionsAndMissingKeys(t *testing.T) {
	if !validDomain("contact-reference-history") || validDomain("contact-reference-history-all") {
		t.Fatal("domain scope is not closed")
	}
	for _, mode := range []string{"import", "reconcile"} {
		for _, kind := range []string{"archive-source", "dm01-source", "missing-key"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "invalid-target", ArchiveKey: strings.Repeat("k", 32), SourceHMACKey: strings.Repeat("h", 32)}
				t.Setenv("AICRM_DM01_SOURCE_DATABASE_URL", "")
				t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", strings.Repeat("d", 32))
				switch kind {
				case "archive-source":
					env.SourceDatabaseURL = "prohibited-source"
				case "dm01-source":
					t.Setenv("AICRM_DM01_SOURCE_DATABASE_URL", "prohibited-source")
				case "missing-key":
					env.SourceHMACKey = ""
				}
				err := run([]string{"--domain=contact-reference-history", "--mode=" + mode, "--archive-run-id=test", "--dm01-run-id=2"}, env)
				if err == nil || !strings.Contains(err.Error(), "contact-reference-history requires local-only archive/DM01 keys") {
					t.Fatalf("must fail before connecting: %v", err)
				}
			})
		}
	}
}

func TestContactReferenceHistoryCLIRequiresExplicitCorporation(t *testing.T) {
	for _, mode := range []string{"import", "reconcile"} {
		for _, corp := range []string{"", " corp", "corp "} {
			t.Run(mode+"/"+corp, func(t *testing.T) {
				env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "invalid-target", ArchiveKey: strings.Repeat("k", 32), SourceHMACKey: strings.Repeat("h", 32)}
				t.Setenv("AICRM_DM01_SOURCE_DATABASE_URL", "")
				t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", strings.Repeat("d", 32))
				err := run([]string{"--domain=contact-reference-history", "--mode=" + mode, "--archive-run-id=test", "--dm01-run-id=2", "--reference-corp-id=" + corp}, env)
				if err == nil || err.Error() != "contact-reference-history requires reference-corp-id" {
					t.Fatalf("must require explicit attribution before connecting: %v", err)
				}
			})
		}
	}
}
