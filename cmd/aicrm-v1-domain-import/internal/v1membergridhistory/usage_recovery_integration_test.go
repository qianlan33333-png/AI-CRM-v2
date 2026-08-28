package v1membergridhistory

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var usageRecoveryEvidence = flag.String("usage-recovery-evidence", "", "frozen JSONL recovery evidence; read-only verification")

const frozenUsageRecoveryQuerySHA256 = "ba101cea73bc5e055cb1f46a9b9e71a739743248d383e8280513775709656859"

type usageRecoveryEvidenceHeader struct {
	Scope       UsageSnapshotRecoveryScope `json:"scope"`
	QuerySHA256 string                     `json:"query_sha256"`
	RowCount    int                        `json:"row_count"`
}

// This opt-in check verifies the already-frozen recovery evidence against the
// sealed V2 archive. It has no V1 connection, emits no evidence, and writes no
// target row.
func TestFrozenUsageSnapshotRecoveryEvidence(t *testing.T) {
	if *usageRecoveryEvidence == "" {
		t.Skip("set -usage-recovery-evidence to verify frozen recovery JSONL")
	}
	info, err := os.Lstat(*usageRecoveryEvidence)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal("usage_recovery_evidence_file_invalid")
	}
	file, err := os.Open(*usageRecoveryEvidence)
	if err != nil {
		t.Fatal("usage_recovery_evidence_open_failed")
	}
	defer file.Close()
	decoder := json.NewDecoder(bufio.NewReader(file))
	decoder.DisallowUnknownFields()
	var header usageRecoveryEvidenceHeader
	if err = decoder.Decode(&header); err != nil || header.Scope != FixedUsageSnapshotRecoveryScope() ||
		header.QuerySHA256 != frozenUsageRecoveryQuerySHA256 || header.RowCount != 2534 {
		t.Fatal("usage_recovery_evidence_header_invalid")
	}

	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	key := []byte(environment.SourceHMACKey)
	if len(key) < sha256.Size {
		t.Fatal("usage_recovery_source_hmac_key_invalid")
	}
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("usage_recovery_archive_open_failed")
	}
	defer archive.Close()
	originals := make(map[[sha256.Size]byte]v1archive.ArchivedRow, header.RowCount)
	err = archive.EachTableRow(ctx, header.Scope.ArchiveRunID, UsageSnapshotsTableID, func(row v1archive.ArchivedRow) error {
		if row.SourceOrdinal != int64(len(originals)+1) || row.SourceKeyHMAC == ([sha256.Size]byte{}) ||
			row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
			return ErrInvalidArchiveRow
		}
		if _, duplicate := originals[row.SourceKeyHMAC]; duplicate {
			return ErrInvalidArchiveRow
		}
		originals[row.SourceKeyHMAC] = row
		return nil
	})
	if err != nil || len(originals) != header.RowCount {
		t.Fatal("usage_recovery_archive_coverage_invalid")
	}

	seen := make(map[[sha256.Size]byte]struct{}, header.RowCount)
	trueCount := 0
	for {
		var entry UsageSnapshotRecoveryEntry
		err = decoder.Decode(&entry)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || entry.Scope != header.Scope || entry.SourceKeyHMAC == ([sha256.Size]byte{}) {
			t.Fatal("usage_recovery_evidence_entry_invalid")
		}
		original, found := originals[entry.SourceKeyHMAC]
		if !found {
			t.Fatal("usage_recovery_evidence_source_unbound")
		}
		if _, duplicate := seen[entry.SourceKeyHMAC]; duplicate {
			t.Fatal("usage_recovery_evidence_duplicate_source")
		}
		decision, adaptErr := AdaptUsageSnapshotRecovery(original, entry, key)
		if adaptErr != nil || decision.Disposition != DispositionCandidate || decision.Record == nil || decision.Record.HasTokenUsage != entry.HasTokenUsage {
			t.Fatal("usage_recovery_evidence_hmac_or_candidate_invalid")
		}
		if entry.HasTokenUsage {
			trueCount++
		}
		seen[entry.SourceKeyHMAC] = struct{}{}
	}
	if len(seen) != header.RowCount || trueCount != 1183 || header.RowCount-trueCount != 1351 {
		t.Fatal("usage_recovery_evidence_coverage_or_boolean_distribution_invalid")
	}
	t.Logf("usage_recovery_verified rows=%d true=%d false=%d sealed_archive_writes=0 target_writes=0", len(seen), trueCount, header.RowCount-trueCount)
}
