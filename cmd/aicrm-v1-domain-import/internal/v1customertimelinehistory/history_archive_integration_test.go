package v1customertimelinehistory

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

const archiveRunID = "v1-full-archive-20260827"

var archiveCensus = flag.Bool("customer-timeline-history-archive-census", false, "read and authenticate the reconciled V2 archive for customer timeline history without target writes")

type censusSummary struct {
	Rows                 int            `json:"rows"`
	RedactedCombinations map[string]int `json:"redacted_field_combinations"`
	SourceKeyFailures    int            `json:"source_key_hmac_failures"`
	PayloadFailures      int            `json:"payload_hmac_failures"`
	FieldFailures        int            `json:"field_hmac_failures"`
}

// TestReconciledCustomerTimelineArchiveCensus is explicitly opt-in. The
// archive reader uses a repeatable-read, read-only V2 transaction; this test
// refuses a V1 source DSN and has no target-domain writer.
func TestReconciledCustomerTimelineArchiveCensus(t *testing.T) {
	if !*archiveCensus {
		t.Skip("supply -customer-timeline-history-archive-census and V2 archive environment for read-only census")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if environment.SourceDatabaseURL != "" {
		t.Fatal("customer_timeline_history_v1_source_dsn_must_be_empty")
	}
	key := []byte(environment.SourceHMACKey)
	if len(key) < sha256.Size {
		t.Fatal("customer_timeline_history_source_hmac_key_invalid")
	}
	archive, err := v1archive.OpenPostgresArchiveReader(context.Background(), environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("customer_timeline_history_archive_open_failed")
	}
	defer archive.Close()

	summary, err := readArchiveCensus(context.Background(), archive, key)
	if err != nil {
		t.Fatal("customer_timeline_history_archive_census_read_failed")
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal("customer_timeline_history_archive_census_encode_failed")
	}
	t.Logf("customer_timeline_history_archive_census=%s target_writes=0", encoded)
	if summary.Rows != 49119 || summary.SourceKeyFailures != 0 || summary.PayloadFailures != 0 || summary.FieldFailures != 0 {
		t.Fatal("customer_timeline_history_archive_census_failed")
	}
}

func readArchiveCensus(ctx context.Context, archive *v1archive.PostgresArchiveReader, key []byte) (censusSummary, error) {
	summary := censusSummary{RedactedCombinations: map[string]int{}}
	err := archive.EachTableRow(ctx, archiveRunID, TableID, func(row v1archive.ArchivedRow) error {
		expectedOrdinal := int64(summary.Rows + 1)
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != TableID || row.SourceOrdinal != expectedOrdinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
			return ErrArchiveRow
		}
		summary.Rows++
		summary.RedactedCombinations[redactedCombination(row.RedactedFields)]++
		canonical, roots, redactErr := v1archive.RedactPayload(row.Payload)
		if redactErr != nil || !bytes.Equal(canonical, row.Payload) {
			summary.PayloadFailures++
			summary.FieldFailures++
		} else {
			expectedPayload, payloadErr := v1archive.PayloadHMAC(key, archiveTableName(), canonical)
			if payloadErr != nil || expectedPayload != row.PayloadHMAC {
				summary.PayloadFailures++
			}
			expectedFields, fieldErr := v1archive.FieldHMAC(key, archiveTableName(), roots)
			if fieldErr != nil || expectedFields != row.FieldHMAC || !sameStrings(roots, row.RedactedFields) {
				summary.FieldFailures++
			}
		}
		fields, fieldsErr := rawFields(row.Payload)
		if fieldsErr != nil {
			summary.SourceKeyFailures++
			return nil
		}
		id, found := fields["id"]
		keyJSON, marshalErr := json.Marshal([]json.RawMessage{id})
		expectedSource, sourceErr := v1archive.SourceKeyHMAC(key, archiveTableName(), keyJSON)
		if !found || isNull(id) || marshalErr != nil || sourceErr != nil || expectedSource != row.SourceKeyHMAC {
			summary.SourceKeyFailures++
		}
		return nil
	})
	return summary, err
}

func redactedCombination(fields []string) string {
	if len(fields) == 0 {
		return "none"
	}
	values := append([]string(nil), fields...)
	sort.Strings(values)
	return strings.Join(values, ",")
}
