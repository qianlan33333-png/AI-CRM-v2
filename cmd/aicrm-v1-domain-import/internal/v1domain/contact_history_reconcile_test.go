package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type contactHistoryReconcileReader struct {
	sidebar contactport.HistoricalSidebarProfile
	owner   contactport.HistoricalOwnerMigrationResult
	err     error
}

func (r contactHistoryReconcileReader) GetHistoricalSidebarProfile(context.Context, int64) (contactport.HistoricalSidebarProfile, error) {
	return r.sidebar, r.err
}
func (r contactHistoryReconcileReader) GetHistoricalOwnerMigrationResult(context.Context, int64) (contactport.HistoricalOwnerMigrationResult, error) {
	return r.owner, r.err
}
func (r contactHistoryReconcileReader) ListHistoricalSidebarProfiles(context.Context, contactport.ContactHistoryQuery) ([]contactport.HistoricalSidebarProfile, int64, error) {
	return nil, 0, r.err
}
func (r contactHistoryReconcileReader) ListHistoricalOwnerMigrationResults(context.Context, contactport.ContactHistoryQuery) ([]contactport.HistoricalOwnerMigrationResult, int64, error) {
	return nil, 0, r.err
}

func contactHistoryReconcileFixture(t *testing.T, owner bool) (contactHistoryReconcileReader, reconciliationRow) {
	t.Helper()
	key, payload := sha256.Sum256([]byte("source-key")), sha256.Sum256([]byte("original-payload"))
	now := time.Date(2026, 8, 27, 13, 36, 2, 123456000, time.UTC)
	r := contactHistoryReconcileReader{
		sidebar: contactport.HistoricalSidebarProfile{ID: 71, SourceKeyDigest: key, SourcePayloadDigest: payload, Source: " old source ", UpdatedAt: now},
		owner: contactport.HistoricalOwnerMigrationResult{ID: 71, SourceKeyDigest: key, SourcePayloadDigest: payload, WeComFailed: 3,
			SessionRelation: "unresolved", PreviewRelation: "unresolved", CreatedAt: now, ExecutedAt: now},
	}
	domain, target, id, source := "contact", "contact_v1_sidebar_profile_history", "71", "public/sidebar_customer_profile_fields"
	digest, err := contactapp.HistoricalSidebarProfileDigest(r.sidebar)
	if owner {
		target, source = "contact_v1_owner_migration_result_history", "public/owner_migration_results"
		digest, err = contactapp.HistoricalOwnerMigrationResultDigest(r.owner)
	}
	if err != nil {
		t.Fatal(err)
	}
	return r, reconciliationRow{TableID: source, SourceKeyDigest: key[:], PayloadDigest: payload[:], TargetDomain: &domain, TargetTable: &target, TargetID: &id, TargetDigest: digest[:]}
}

func TestContactHistoryReconcileReadsCompleteActualTarget(t *testing.T) {
	for _, owner := range []bool{false, true} {
		reader, row := contactHistoryReconcileFixture(t, owner)
		proof, err := verifyContactHistoryRow(context.Background(), reader, row)
		if err != nil || proof != "history_only:"+hex.EncodeToString(row.TargetDigest) {
			t.Fatalf("valid history: %v", err)
		}
		for name, mutate := range map[string]func(*contactHistoryReconcileReader, *reconciliationRow){
			"source-key":    func(_ *contactHistoryReconcileReader, row *reconciliationRow) { row.SourceKeyDigest[0]++ },
			"payload":       func(_ *contactHistoryReconcileReader, row *reconciliationRow) { row.PayloadDigest[0]++ },
			"target-digest": func(_ *contactHistoryReconcileReader, row *reconciliationRow) { row.TargetDigest[0]++ },
			"source-table": func(_ *contactHistoryReconcileReader, row *reconciliationRow) {
				row.TableID = "public/owner_migration_previews"
			},
			"target-table": func(_ *contactHistoryReconcileReader, row *reconciliationRow) { v := "customers"; row.TargetTable = &v },
			"domain":       func(_ *contactHistoryReconcileReader, row *reconciliationRow) { v := "wecom"; row.TargetDomain = &v },
			"id":           func(_ *contactHistoryReconcileReader, row *reconciliationRow) { v := "0071"; row.TargetID = &v },
			"actual-id":    func(r *contactHistoryReconcileReader, _ *reconciliationRow) { r.sidebar.ID++; r.owner.ID++ },
			"actual-fields": func(r *contactHistoryReconcileReader, _ *reconciliationRow) {
				r.sidebar.Source += "changed"
				r.owner.WeComFailed++
			},
			"read-error": func(r *contactHistoryReconcileReader, _ *reconciliationRow) { r.err = errors.New("unavailable") },
		} {
			t.Run(name, func(t *testing.T) {
				r, candidate := contactHistoryReconcileFixture(t, owner)
				mutate(&r, &candidate)
				if _, err := verifyContactHistoryRow(context.Background(), r, candidate); !errors.Is(err, ErrConflict) {
					t.Fatal("history drift accepted")
				}
			})
		}
	}
}

func TestContactHistoryReconcileRejectsWrongVersionBeforeDatabase(t *testing.T) {
	if _, err := ReconcileContactHistory(context.Background(), nil, "v1-contact-history-a2", "archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong version accepted")
	}
}
