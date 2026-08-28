package v1customerstatehistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"sort"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var customerStateHistoryArchiveRun = flag.String("customer-state-history-archive-run", "", "optional reconciled V2 archive run for read-only customer-state history preflight")

type archivePreflightSummary struct {
	Table          string         `json:"table"`
	Rows           int            `json:"rows"`
	Candidates     int            `json:"candidates"`
	Quarantined    int            `json:"quarantined"`
	Reasons        map[string]int `json:"reasons"`
	RedactionRoots map[string]int `json:"redaction_roots"`
}

type archivePreflightTable struct {
	table    string
	expected int
	adapt    func(v1archive.ArchivedRow) (Disposition, string, bool)
}

// TestReconciledCustomerStateHistoryArchivePreflight is opt-in and read-only.
// It reads only a reconciled V2 archive run and never opens a current-state,
// journal, or target-domain transaction. The current snapshot has manifest
// PK=[], so source identity remains the authenticated archive SourceKeyHMAC;
// this test deliberately does not invent an ID from the payload.
func TestReconciledCustomerStateHistoryArchivePreflight(t *testing.T) {
	if *customerStateHistoryArchiveRun == "" {
		t.Skip("supply -customer-state-history-archive-run and V2 archive environment for read-only customer-state history preflight")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	sourceHMACKey := []byte(environment.SourceHMACKey)
	if len(sourceHMACKey) < sha256.Size {
		t.Fatal("customer_state_history_source_hmac_key_invalid")
	}
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("customer_state_history_archive_open_failed")
	}
	defer archive.Close()

	tables := []archivePreflightTable{
		{table: UserStatusCurrentTableID, expected: 72, adapt: func(row v1archive.ArchivedRow) (Disposition, string, bool) {
			result := AdaptUserStatusCurrent(row)
			return result.Disposition, result.Reason, result.Candidate != nil
		}},
		{table: UserStatusHistoryTableID, expected: 199, adapt: func(row v1archive.ArchivedRow) (Disposition, string, bool) {
			result := AdaptUserStatusHistory(row)
			return result.Disposition, result.Reason, result.Candidate != nil
		}},
		{table: TermTagMappingTableID, expected: 9, adapt: func(row v1archive.ArchivedRow) (Disposition, string, bool) {
			result := AdaptTermTagMapping(row)
			return result.Disposition, result.Reason, result.Candidate != nil
		}},
	}

	totalRows, totalCandidates, totalQuarantined := 0, 0, 0
	for _, table := range tables {
		summary, err := readCustomerStateHistoryArchiveTable(ctx, archive, *customerStateHistoryArchiveRun, sourceHMACKey, table)
		if err != nil {
			t.Fatal("customer_state_history_archive_preflight_failed")
		}
		if summary.Rows != table.expected || summary.Rows != summary.Candidates+summary.Quarantined {
			t.Fatal("customer_state_history_archive_conservation_failed")
		}
		encoded, err := json.Marshal(summary)
		if err != nil {
			t.Fatal("customer_state_history_archive_summary_encode_failed")
		}
		t.Logf("customer_state_history_archive_preflight=%s", encoded)
		totalRows += summary.Rows
		totalCandidates += summary.Candidates
		totalQuarantined += summary.Quarantined
	}
	if totalRows != 280 || totalRows != totalCandidates+totalQuarantined {
		t.Fatal("customer_state_history_archive_total_conservation_failed")
	}
	t.Logf("customer_state_history_archive_total rows=%d candidates=%d quarantined=%d target_writes=0 current_state_writes=0 journal_writes=0", totalRows, totalCandidates, totalQuarantined)
}

func readCustomerStateHistoryArchiveTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID string, sourceHMACKey []byte, table archivePreflightTable) (archivePreflightSummary, error) {
	summary := archivePreflightSummary{Table: table.table, Reasons: map[string]int{}, RedactionRoots: map[string]int{}}
	seenSourceKeys := map[[sha256.Size]byte]struct{}{}
	reason := ""
	err := archive.EachTableRow(ctx, runID, table.table, func(row v1archive.ArchivedRow) error {
		reason = customerStateHistoryArchiveRowReason(row, table.table, int64(summary.Rows+1), sourceHMACKey)
		if reason == "" {
			if _, found := seenSourceKeys[row.SourceKeyHMAC]; found {
				reason = "customer_state_history_archive_duplicate_source_key"
			}
		}
		if reason != "" {
			return errors.New(reason)
		}
		seenSourceKeys[row.SourceKeyHMAC] = struct{}{}
		summary.Rows++
		for _, path := range row.RedactedFields {
			summary.RedactionRoots[customerStateHistoryRedactionRoot(path)]++
		}
		disposition, resultReason, candidate := table.adapt(row)
		switch disposition {
		case DispositionCandidate:
			if !candidate || resultReason != "" {
				return errors.New("customer_state_history_candidate_result_invalid")
			}
			summary.Candidates++
		case DispositionQuarantine:
			if candidate || resultReason == "" {
				return errors.New("customer_state_history_quarantine_result_invalid")
			}
			summary.Quarantined++
			summary.Reasons[resultReason]++
		default:
			return errors.New("customer_state_history_disposition_invalid")
		}
		return nil
	})
	if err != nil {
		return archivePreflightSummary{}, err
	}
	return summary, nil
}

func customerStateHistoryArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64, sourceHMACKey []byte) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
		return "customer_state_history_archive_identity_invalid"
	}
	payloadDigest, err := v1archive.PayloadHMAC(sourceHMACKey, customerStateHistoryArchiveTableName(table), row.Payload)
	if err != nil || payloadDigest != row.PayloadHMAC {
		return "customer_state_history_archive_payload_hmac_invalid"
	}
	fieldDigest, err := v1archive.FieldHMAC(sourceHMACKey, customerStateHistoryArchiveTableName(table), row.RedactedFields)
	if err != nil || fieldDigest != row.FieldHMAC {
		return "customer_state_history_archive_field_hmac_invalid"
	}
	// ArchivedRow intentionally exposes no source-key JSON. Decryption already
	// authenticates SourceKeyHMAC as record AAD; uniqueness/non-zero above is
	// the strongest check possible without inventing a snapshot primary key.
	return ""
}

func customerStateHistoryArchiveTableName(table string) string {
	return strings.TrimPrefix(table, "public/")
}

func customerStateHistoryRedactionRoot(path string) string {
	root := path
	if index := strings.IndexAny(root, ".[\""); index >= 0 {
		root = root[:index]
	}
	if root == "" {
		return "invalid"
	}
	return root
}

func TestCustomerStateHistoryRedactionRoot(t *testing.T) {
	paths := map[string]string{
		"token":                        "token",
		"status_flags_json.entries[0]": "status_flags_json",
		"nested.secret.value":          "nested",
		"":                             "invalid",
	}
	keys := make([]string, 0, len(paths))
	for path := range paths {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		if got := customerStateHistoryRedactionRoot(path); got != paths[path] {
			t.Fatalf("redaction_root_mismatch path=%q got=%q", path, got)
		}
	}
}
