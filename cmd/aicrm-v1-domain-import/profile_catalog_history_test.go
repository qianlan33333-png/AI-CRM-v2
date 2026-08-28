package main

import (
	"context"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1profilecatalog"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"strings"
	"testing"
)

func TestProfileCatalogCommandIsExplicitAndScopeBound(t *testing.T) {
	if !validDomain("profile-catalog-history") || validDomain("profile_catalog_history") {
		t.Fatal("selector not exact")
	}
	env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres://must-not-connect.invalid/aicrm"}
	if err := run([]string{"--domain=profile-catalog-history", "--archive-run-id=archive"}, env); err == nil || !strings.Contains(err.Error(), "archive key") {
		t.Fatal("missing archive key did not fail before connection")
	}
	if _, err := newProfileCatalogHistoryJournals(""); err == nil {
		t.Fatal("empty run accepted")
	}
	journals, err := newProfileCatalogHistoryJournals("archive-run")
	if err != nil || len(journals) != 4 {
		t.Fatal("four exact scopes required")
	}
	profiles, err := v1domain.NewProfileCatalogHistoryJournal(journals[v1profilecatalog.ProfileTemplatesTableID], journals[v1profilecatalog.ProfileCategoriesTableID], journals[v1profilecatalog.ProfileOptionMappingsTableID])
	if err != nil || profiles == nil {
		t.Fatal("profile scopes rejected")
	}
	if tags, err := v1domain.NewSignupTagHistoryJournal(journals[v1profilecatalog.SignupTagRulesTableID]); err != nil || tags == nil {
		t.Fatal("tag scope rejected")
	}
	if _, err := v1domain.NewSignupTagHistoryJournal(journals[v1profilecatalog.ProfileTemplatesTableID]); err == nil {
		t.Fatal("owner scope mixed")
	}
	if _, err := v1domain.NewProfileCatalogHistoryJournal(journals[v1profilecatalog.ProfileTemplatesTableID], journals[v1profilecatalog.ProfileOptionMappingsTableID], journals[v1profilecatalog.ProfileCategoriesTableID]); err == nil {
		t.Fatal("source scopes mixed")
	}
}
func TestProfileCatalogTargetReadRequiresCallerTransaction(t *testing.T) {
	reader := profileCatalogHistoryTxReader{}
	if _, err := reader.ReadTemplate(context.Background(), 1); err == nil {
		t.Fatal("template read outside caller tx")
	}
	if _, err := reader.ReadCategory(context.Background(), 1); err == nil {
		t.Fatal("category read outside caller tx")
	}
	if _, err := reader.ReadOptionMapping(context.Background(), 1); err == nil {
		t.Fatal("mapping read outside caller tx")
	}
	if _, err := reader.ReadSignupTagRule(context.Background(), 1); err == nil {
		t.Fatal("rule read outside caller tx")
	}
}
