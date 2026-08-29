package v1audienceactivityhistory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"sort"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const audienceActivityArchiveRunID = "v1-full-archive-20260827"

var audienceActivityArchiveCensus = flag.Bool("audience-activity-history-archive-census", false, "read and authenticate the reconciled V2 archive for audience activity history without target writes")

type audienceActivityCensus struct {
	Rows                 int            `json:"rows"`
	RedactedCombinations map[string]int `json:"redacted_field_combinations"`
	SourceKeyFailures    int            `json:"source_key_hmac_failures"`
	PayloadFailures      int            `json:"payload_hmac_failures"`
	FieldFailures        int            `json:"field_hmac_failures"`
}

// TestReconciledAudienceActivityArchiveCensus is explicitly opt-in. It only
// reads the sealed V2 archive and logs aggregate counts; it has no target
// writer and refuses a V1 source DSN.
func TestReconciledAudienceActivityArchiveCensus(t *testing.T) {
	if !*audienceActivityArchiveCensus {
		t.Skip("supply -audience-activity-history-archive-census and V2 archive environment for read-only census")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if environment.SourceDatabaseURL != "" {
		t.Fatal("audience_activity_history_v1_source_dsn_must_be_empty")
	}
	key := []byte(environment.SourceHMACKey)
	if len(key) < sha256.Size {
		t.Fatal("audience_activity_history_source_hmac_key_invalid")
	}
	archive, err := v1archive.OpenPostgresArchiveReader(context.Background(), environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("audience_activity_history_archive_open_failed")
	}
	defer archive.Close()

	for _, table := range []string{PackageRunsTableID, MemberEventsTableID} {
		summary, censusErr := readAudienceActivityCensus(context.Background(), archive, table, key)
		if censusErr != nil {
			t.Fatal("audience_activity_history_archive_census_read_failed")
		}
		encoded, marshalErr := json.Marshal(summary)
		if marshalErr != nil {
			t.Fatal("audience_activity_history_archive_census_encode_failed")
		}
		t.Logf("audience_activity_history_archive_census table=%s summary=%s target_writes=0", table, encoded)
		if summary.SourceKeyFailures != 0 || summary.PayloadFailures != 0 || summary.FieldFailures != 0 {
			t.Fatal("audience_activity_history_archive_census_failed")
		}
	}
}

func readAudienceActivityCensus(ctx context.Context, archive *v1archive.PostgresArchiveReader, table string, key []byte) (audienceActivityCensus, error) {
	summary := audienceActivityCensus{RedactedCombinations: map[string]int{}}
	err := archive.EachTableRow(ctx, audienceActivityArchiveRunID, table, func(row v1archive.ArchivedRow) error {
		summary.Rows++
		summary.RedactedCombinations[audienceActivityRedactedCombination(row.RedactedFields)]++
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != int64(summary.Rows) || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
			return ErrArchiveRow
		}
		canonical, _, redactErr := v1archive.RedactPayload(row.Payload)
		if redactErr != nil || !bytes.Equal(canonical, row.Payload) {
			summary.PayloadFailures++
			summary.FieldFailures++
			return nil
		}
		name := strings.TrimPrefix(table, "public/")
		expectedPayload, payloadErr := v1archive.PayloadHMAC(key, name, canonical)
		if payloadErr != nil || expectedPayload != row.PayloadHMAC {
			summary.PayloadFailures++
		}
		expectedField, fieldErr := v1archive.FieldHMAC(key, name, row.RedactedFields)
		if fieldErr != nil || expectedField != row.FieldHMAC {
			summary.FieldFailures++
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(canonical, &fields) != nil || fields == nil || fields["id"] == nil || bytes.Equal(bytes.TrimSpace(fields["id"]), []byte("null")) {
			summary.SourceKeyFailures++
			return nil
		}
		keyJSON, marshalErr := json.Marshal([]json.RawMessage{fields["id"]})
		expectedSource, sourceErr := v1archive.SourceKeyHMAC(key, name, keyJSON)
		if marshalErr != nil || sourceErr != nil || expectedSource != row.SourceKeyHMAC {
			summary.SourceKeyFailures++
		}
		return nil
	})
	return summary, err
}

func audienceActivityRedactedCombination(fields []string) string {
	if len(fields) == 0 {
		return "none"
	}
	values := append([]string(nil), fields...)
	sort.Strings(values)
	return strings.Join(values, ",")
}
