package main

import (
	"strings"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestAudienceHistoryRequiresActorAndDM01BeforeConnecting(t *testing.T) {
	t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", "")
	environment := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres:///must-not-connect", ArchiveKey: strings.Repeat("k", 32)}
	args := []string{"--domain=audience-history", "--archive-run-id=archive"}
	if err := run(args, environment); err == nil || !strings.Contains(err.Error(), "migration-actor") {
		t.Fatalf("missing actor must fail before connecting: %v", err)
	}
	if err := run(append(args, "--migration-actor=1"), environment); err == nil || !strings.Contains(err.Error(), "dm01-run-id") {
		t.Fatalf("missing DM01 binding must fail before connecting: %v", err)
	}
	if !validDomain("audience-history") {
		t.Fatal("history must be explicitly selected")
	}
}

func TestAudienceHistoryJournalComposition(t *testing.T) {
	if _, err := newAudienceHistoryJournals(""); err == nil {
		t.Fatal("empty archive run accepted")
	}
	journals, err := newAudienceHistoryJournals("archive")
	if err != nil || len(journals) != 8 {
		t.Fatalf("eight source journals required: %v", err)
	}
	if _, err := v1domain.NewAudienceHistoryJournal(journals); err != nil {
		t.Fatalf("composition must match the closed eight-table scope: %v", err)
	}
	if v1domain.AudienceHistoryImportVersion == domainImportVersion {
		t.Fatal("history cannot reuse the first domain import version")
	}
}
