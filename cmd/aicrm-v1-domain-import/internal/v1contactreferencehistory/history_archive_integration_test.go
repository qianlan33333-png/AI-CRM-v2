package v1contactreferencehistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"sort"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const contactReferenceHistoryArchiveRun = "v1-full-archive-20260827"

var contactReferenceHistoryArchivePreflight = flag.Bool("contact-reference-history-archive-preflight", false, "read and validate the reconciled V2 archive for contact-reference history")

type contactReferenceArchiveSummary struct {
	Table          string         `json:"table"`
	Rows           int            `json:"rows"`
	Candidates     int            `json:"candidates"`
	Quarantined    int            `json:"quarantined"`
	Reasons        map[string]int `json:"reasons"`
	RedactionRoots map[string]int `json:"redaction_roots"`
}

type contactReferenceArchiveTable struct {
	table              string
	expectedRows       int
	expectedCandidates int
	adapt              func(v1archive.ArchivedRow, []byte) error
}

// TestReconciledContactReferenceArchivePreflight is explicitly opt-in. It
// streams only the reconciled encrypted V2 archive with repeatable-read,
// read-only transactions; it never opens V1 or a target-domain writer.
func TestReconciledContactReferenceArchivePreflight(t *testing.T) {
	if !*contactReferenceHistoryArchivePreflight {
		t.Skip("supply -contact-reference-history-archive-preflight and V2 archive environment for read-only contact-reference validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if environment.SourceDatabaseURL != "" {
		t.Fatal("contact_reference_history_v1_source_dsn_must_be_empty")
	}
	sourceHMACKey := []byte(environment.SourceHMACKey)
	if len(sourceHMACKey) < sha256.Size {
		t.Fatal("contact_reference_history_source_hmac_key_invalid")
	}
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("contact_reference_history_archive_open_failed")
	}
	defer archive.Close()

	tables := []contactReferenceArchiveTable{
		{table: ExternalContactBindingsTableID, expectedRows: 1370, expectedCandidates: 1370, adapt: func(row v1archive.ArchivedRow, key []byte) error {
			_, err := AdaptExternalContactBinding(row, key)
			return err
		}},
		{table: AdminWeComDirectoryMembersTableID, expectedRows: 44, expectedCandidates: 44, adapt: func(row v1archive.ArchivedRow, key []byte) error {
			_, err := AdaptDirectoryMember(row, key)
			return err
		}},
	}

	totalRows, totalCandidates, totalQuarantined := 0, 0, 0
	for _, table := range tables {
		summary, readErr := readContactReferenceArchiveTable(ctx, archive, sourceHMACKey, table)
		if readErr != nil {
			t.Fatal("contact_reference_history_archive_preflight_failed")
		}
		logContactReferenceArchiveSummary(t, summary)
		if summary.Rows != table.expectedRows || summary.Candidates != table.expectedCandidates || summary.Quarantined != 0 || summary.Rows != summary.Candidates+summary.Quarantined {
			t.Fatal("contact_reference_history_archive_conservation_failed")
		}
		totalRows += summary.Rows
		totalCandidates += summary.Candidates
		totalQuarantined += summary.Quarantined
	}
	if totalRows != 1414 || totalRows != totalCandidates+totalQuarantined {
		t.Fatal("contact_reference_history_archive_total_conservation_failed")
	}
	t.Logf("contact_reference_history_archive_total rows=%d candidates=%d quarantined=%d target_writes=0 staff_writes=0 customer_writes=0 identity_assurance_changes=0", totalRows, totalCandidates, totalQuarantined)
}

func readContactReferenceArchiveTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, sourceHMACKey []byte, table contactReferenceArchiveTable) (contactReferenceArchiveSummary, error) {
	summary := contactReferenceArchiveSummary{Table: table.table, Reasons: map[string]int{}, RedactionRoots: map[string]int{}}
	seen := map[[sha256.Size]byte]struct{}{}
	err := archive.EachTableRow(ctx, contactReferenceHistoryArchiveRun, table.table, func(row v1archive.ArchivedRow) error {
		if reason := contactReferenceArchiveRowReason(row, table.table, int64(summary.Rows+1)); reason != "" {
			return errors.New(reason)
		}
		if _, found := seen[row.SourceKeyHMAC]; found {
			return errors.New("contact_reference_history_archive_duplicate_source_key")
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		summary.Rows++
		for _, path := range row.RedactedFields {
			summary.RedactionRoots[contactReferenceRedactionRoot(path)]++
		}
		if err := table.adapt(row, sourceHMACKey); err != nil {
			summary.Quarantined++
			summary.Reasons[contactReferenceArchiveReason(err)]++
			return nil
		}
		summary.Candidates++
		return nil
	})
	if err != nil {
		return contactReferenceArchiveSummary{}, err
	}
	return summary, nil
}

func contactReferenceArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
		return "contact_reference_history_archive_identity_invalid"
	}
	return ""
}

func contactReferenceArchiveReason(err error) string {
	switch {
	case errors.Is(err, ErrRequiredFieldRedacted):
		return "contact_reference_history_required_source_field_redacted"
	case errors.Is(err, ErrArchiveRow):
		return "contact_reference_history_archive_integrity_invalid"
	default:
		return "contact_reference_history_source_fact_invalid"
	}
}

func contactReferenceRedactionRoot(path string) string {
	root := path
	if index := strings.IndexAny(root, ".[\""); index >= 0 {
		root = root[:index]
	}
	if root == "" {
		return "invalid"
	}
	return root
}

func logContactReferenceArchiveSummary(t *testing.T, summary contactReferenceArchiveSummary) {
	t.Helper()
	t.Logf("contact_reference_history_archive table=%s rows=%d candidates=%d quarantined=%d target_writes=0", summary.Table, summary.Rows, summary.Candidates, summary.Quarantined)
	logContactReferenceCountMap(t, "quarantine_reason", summary.Reasons)
	logContactReferenceCountMap(t, "redaction_root", summary.RedactionRoots)
}

func logContactReferenceCountMap(t *testing.T, label string, values map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Logf("%s=none count=0", label)
		return
	}
	for _, key := range keys {
		t.Logf("%s=%s count=%d", label, key, values[key])
	}
}
