package v1cycleobservationhistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var cycleObservationArchivePreflight = flag.Bool("cycle-observation-archive-preflight", false, "read only the frozen V2 archive for cycle observations")

func TestReconciledCycleObservationArchivePreflight(t *testing.T) {
	if !*cycleObservationArchivePreflight {
		t.Skip("opt-in frozen V2 archive validation")
	}
	env := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if env.SourceDatabaseURL != "" || len(env.SourceHMACKey) < sha256.Size || len(env.ArchiveKey) != sha256.Size {
		t.Fatal("local_archive_keys_required_source_dsn_forbidden")
	}
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, env.TargetDatabaseURL, []byte(env.ArchiveKey))
	if err != nil {
		t.Fatal("archive_open_failed")
	}
	defer archive.Close()
	for _, table := range []struct {
		name  string
		count int
		adapt func(v1archive.ArchivedRow, []byte) error
	}{
		{MetricsTableID, 21, func(row v1archive.ArchivedRow, key []byte) error { _, err := AdaptMetric(row, key); return err }},
		{ReferencesTableID, 18, func(row v1archive.ArchivedRow, key []byte) error { _, err := AdaptReference(row, key); return err }},
	} {
		count := 0
		seen := map[[sha256.Size]byte]bool{}
		err = archive.EachTableRow(ctx, "v1-full-archive-20260827", table.name, func(row v1archive.ArchivedRow) error {
			if row.TableID != table.name || row.AdapterID != v1archive.DefaultAdapterID || row.SourceOrdinal != int64(count+1) || seen[row.SourceKeyHMAC] {
				return errors.New("archive_scope_ordinal_or_duplicate")
			}
			if err := table.adapt(row, []byte(env.SourceHMACKey)); err != nil {
				return err
			}
			seen[row.SourceKeyHMAC] = true
			count++
			return nil
		})
		if err != nil || count != table.count {
			t.Fatalf("archive_preflight_failed table=%s accepted=%d expected=%d", table.name, count, table.count)
		}
		t.Logf("table=%s rows=%d accepted=%d source_payload_field_hmac=verified target_writes=0", table.name, count, count)
	}
}
