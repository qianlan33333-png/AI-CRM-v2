package v1messagehistory

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var messageHistoryArchiveRun = flag.String("message-history-archive-run", "", "optional reconciled V2 archive run for read-only message source validation")

func TestReconciledMessageHistoryArchiveShape(t *testing.T) {
	if *messageHistoryArchiveRun == "" {
		t.Skip("supply -message-history-archive-run and V2 archive environment for read-only source validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("message_history_archive_open_failed")
	}
	defer archive.Close()
	var stream StreamSummary
	seen := map[[32]byte]bool{}
	reason := ""
	err = archive.EachTableRow(ctx, *messageHistoryArchiveRun, MessageTable, func(row v1archive.ArchivedRow) error {
		reason = archiveRowReason(row, int64(stream.Summary.Rows+1))
		if reason == "" && seen[row.SourceKeyHMAC] {
			reason = "archive_duplicate_source_key"
		}
		if reason != "" {
			return errors.New(reason)
		}
		seen[row.SourceKeyHMAC] = true
		stream.Add(AdaptMessage(row.Payload))
		return nil
	})
	if err != nil {
		if reason != "" {
			t.Fatal(reason)
		}
		t.Fatal("message_history_archive_read_failed")
	}
	summary := stream.Summary
	if summary.Rows != 53882 {
		t.Fatalf("archive_count_mismatch count=%d", summary.Rows)
	}
	if summary.Rows != summary.Candidates+summary.Pending+summary.Invalid {
		t.Fatal("source_row_conservation_failed")
	}
	if summary.Decoded != summary.ParsedSendTime+summary.CivilSendTime+summary.UnmappedSendTime {
		t.Fatal("send_time_basis_conservation_failed")
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal("summary_encode_failed")
	}
	t.Logf("message_history_shape=%s customer_mapping_checked=0 target_writer_checked=0 target_fk_checked=0 target_writes=0", encoded)
	t.Logf("send_time_basis_explicit_offset=%d send_time_basis_civil_unzoned=%d send_time_basis_unmapped=%d null_byte_content=%d null_byte_send_time=%d null_byte_chat_or_type=%d", summary.ParsedSendTime, summary.CivilSendTime, summary.UnmappedSendTime, summary.NullByteContent, summary.NullByteSendTime, summary.NullByteChatOrType)
	if summary.Pending+summary.Invalid > 0 {
		t.Errorf("message_history_rows_isolated count=%d", summary.Pending+summary.Invalid)
	}
}

func archiveRowReason(row v1archive.ArchivedRow, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != MessageTable || row.SourceOrdinal != ordinal {
		return "archive_scope_or_ordinal_mismatch"
	}
	if row.SourceKeyHMAC == [32]byte{} || row.PayloadHMAC == [32]byte{} || row.FieldHMAC == [32]byte{} {
		return "archive_hmac_missing"
	}
	for _, field := range strings.Fields(requiredFields + " " + nullableFields) {
		for _, redacted := range row.RedactedFields {
			if redacted == field || strings.HasPrefix(redacted, field+".") {
				return "archive_retained_field_redacted"
			}
		}
	}
	return ""
}

func TestMessageArchiveScopeAndRetainedFields(t *testing.T) {
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: MessageTable, SourceOrdinal: 1, SourceKeyHMAC: [32]byte{1}, PayloadHMAC: [32]byte{2}, FieldHMAC: [32]byte{3}}
	if archiveRowReason(row, 1) != "" {
		t.Fatal("valid_archive_metadata_rejected")
	}
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(r *v1archive.ArchivedRow) { r.AdapterID = "other" }, func(r *v1archive.ArchivedRow) { r.TableID = "public/other" },
		func(r *v1archive.ArchivedRow) { r.SourceOrdinal = 2 }, func(r *v1archive.ArchivedRow) { r.SourceKeyHMAC = [32]byte{} },
		func(r *v1archive.ArchivedRow) { r.PayloadHMAC = [32]byte{} }, func(r *v1archive.ArchivedRow) { r.FieldHMAC = [32]byte{} },
	} {
		changed := row
		mutate(&changed)
		if archiveRowReason(changed, 1) == "" {
			t.Fatal("invalid_archive_metadata_accepted")
		}
	}
	for _, field := range strings.Fields(requiredFields + " " + nullableFields) {
		changed := row
		changed.RedactedFields = []string{field}
		if archiveRowReason(changed, 1) != "archive_retained_field_redacted" {
			t.Fatal("retained_field_redaction_accepted")
		}
	}
	row.RedactedFields = []string{"raw_payload.secret"}
	if archiveRowReason(row, 1) != "archive_retained_field_redacted" {
		t.Fatal("nested_retained_field_redaction_accepted")
	}
}
