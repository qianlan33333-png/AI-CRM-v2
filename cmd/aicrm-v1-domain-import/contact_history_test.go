package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
)

func TestContactHistoryCommandRequiresDM01BeforeConnecting(t *testing.T) {
	environment := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres://must-not-connect.invalid/aicrm", ArchiveKey: strings.Repeat("k", 32)}
	for name, test := range map[string]struct {
		args []string
		key  string
	}{
		"missing-run": {[]string{"--domain=contact-history", "--archive-run-id=archive"}, strings.Repeat("h", 32)},
		"missing-key": {[]string{"--domain=contact-history", "--archive-run-id=archive", "--dm01-run-id=2"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", test.key)
			err := run(test.args, environment)
			if err == nil || !strings.Contains(err.Error(), "dm01-run-id") {
				t.Fatal("dm01_preflight_did_not_fail_before_connect")
			}
		})
	}
	if !validDomain("contact-history") || validDomain("contact_history") {
		t.Fatal("contact_history_domain_selector_is_not_exact")
	}
}

func TestContactHistoryJournalPinsItsOwnVersionAndScope(t *testing.T) {
	journal, err := newContactHistoryJournal("archive-run")
	if err != nil || journal.ValidateContactHistoryImportScope("archive-run") != nil {
		t.Fatal("contact_history_scope_not_exact")
	}
	if journal.ValidateContactHistoryImportScope("other-run") == nil {
		t.Fatal("contact_history_scope_accepts_other_run")
	}
	if contactHistoryImportVersion == domainImportVersion || contactHistoryImportVersion == staticImportVersion || contactHistoryImportVersion == financeImportVersion {
		t.Fatal("contact_history_version_is_not_isolated")
	}
}

func TestContactHistoryReferencesOnlyUsesStrictChannelResolution(t *testing.T) {
	var missing *contactHistoryReferences
	if _, err := missing.ResolveHistoricalContactCustomer(context.Background(), "union"); !errors.Is(err, v1domain.ErrInvalidScope) {
		t.Fatal("missing_strict_resolver_accepted")
	}
	references := &contactHistoryReferences{customer: &channelCustomerResolver{
		contacts: contactstore.HistoricalImportRepository{}, run: 2, key: bytes.Repeat([]byte("k"), 32),
	}}
	if customer, err := references.ResolveHistoricalContactCustomer(context.Background(), ""); err != nil || customer != nil {
		t.Fatal("empty_unionid_was_not_preserved_as_unresolved")
	}
	// No transaction or trusted lineage is supplied here. The delegated channel
	// resolver must fail closed rather than derive a V2 customer from unionid.
	if customer, err := references.ResolveHistoricalContactCustomer(context.Background(), "unverified-union"); err == nil || customer != nil {
		t.Fatal("unverified_unionid_was_guessed")
	}
}
