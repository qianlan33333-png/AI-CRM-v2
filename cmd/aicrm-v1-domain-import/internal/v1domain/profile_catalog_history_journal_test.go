package v1domain

import (
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1profilecatalog"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestProfileCatalogHistoryJournalsPinFourScopesAndReceiptShapes(t *testing.T) {
	journals := profileCatalogScopedJournals(t, "archive-run")
	if !validProfileCatalogHistoryJournals(journals) {
		t.Fatal("valid four scopes rejected")
	}
	if _, err := NewProfileCatalogHistoryJournal(journals[v1profilecatalog.ProfileTemplatesTableID], journals[v1profilecatalog.ProfileCategoriesTableID], journals[v1profilecatalog.ProfileOptionMappingsTableID]); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSignupTagHistoryJournal(journals[v1profilecatalog.SignupTagRulesTableID]); err != nil {
		t.Fatal(err)
	}

	profile := segmentport.ProfileCatalogHistoryReceipt{Kind: v1profilecatalog.ProfileTemplatesKind, SourceIdentifier: SourceIdentifier(profileCatalogDigest(1)), PayloadDigest: profileCatalogDigest(2), TargetID: 11, TargetDigest: profileCatalogDigest(3)}
	terminal, err := profileCatalogTerminalFromReceipt(profile)
	if err != nil {
		t.Fatal(err)
	}
	decoded, found, err := profileCatalogReceiptFromTerminal(v1profilecatalog.ProfileTemplatesKind, profile.SourceIdentifier, terminal)
	if err != nil || !found || decoded != profile {
		t.Fatalf("profile receipt roundtrip=%+v found=%t err=%v", decoded, found, err)
	}
	profile.Replayed = true
	if _, err = profileCatalogTerminalFromReceipt(profile); err == nil {
		t.Fatal("replayed write receipt accepted")
	}

	tag := contactport.SignupTagHistoryReceipt{SourceIdentifier: SourceIdentifier(profileCatalogDigest(4)), PayloadDigest: profileCatalogDigest(5), TargetID: 12, TargetDigest: profileCatalogDigest(6)}
	tagTerminal, err := signupTagTerminalFromReceipt(tag)
	if err != nil {
		t.Fatal(err)
	}
	decodedTag, found, err := signupTagReceiptFromTerminal(tag.SourceIdentifier, tagTerminal)
	if err != nil || !found || decodedTag != tag {
		t.Fatalf("signup receipt roundtrip=%+v found=%t err=%v", decodedTag, found, err)
	}
}

func TestProfileCatalogHistoryJournalsRejectMixedRunAndWrongTarget(t *testing.T) {
	journals := profileCatalogScopedJournals(t, "archive-run")
	journals[v1profilecatalog.ProfileCategoriesTableID].scope.ArchiveRunID = "other-run"
	if validProfileCatalogHistoryJournals(journals) {
		t.Fatal("mixed archive runs accepted")
	}
	journals = profileCatalogScopedJournals(t, "archive-run")
	journals[v1profilecatalog.SignupTagRulesTableID].scope.TargetTable = "wrong_table"
	if validProfileCatalogHistoryJournals(journals) {
		t.Fatal("wrong target table accepted")
	}
}

func profileCatalogScopedJournals(t *testing.T, run string) map[string]*Journal {
	t.Helper()
	result := map[string]*Journal{}
	for _, scope := range profileCatalogHistoryScopes {
		journal, err := NewJournal(Scope{ImportVersion: profileCatalogHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: scope.source, TargetDomain: scope.domain, TargetTable: scope.target})
		if err != nil {
			t.Fatal(err)
		}
		result[scope.source] = journal
	}
	return result
}
