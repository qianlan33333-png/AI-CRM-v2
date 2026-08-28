package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAggregateDM01ExternalIdentityArchiveOnlyReportsSafeShapeCounts(t *testing.T) {
	archiveKey := []byte("archive-source-key-32-bytes-long!!")
	dm01Key := []byte("dm01-source-key-32-bytes-long!!!!!")
	archive := diffArchive{rows: []v1archive.ArchivedRow{
		diffIdentityArchiveShapeRow(t, archiveKey, 1, 1, map[string]any{"external_userid": "external", "unionid": "union", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00.000000+00:00"}, nil),
		diffIdentityArchiveShapeRow(t, archiveKey, 2, 2, map[string]any{"external_userid": "", "unionid": "union", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00.000000+00:00"}, nil),
		diffIdentityArchiveShapeRow(t, archiveKey, 3, 3, map[string]any{"external_userid": "external", "unionid": "union", "corp_id": "corp", "updated_at": "not-a-time"}, nil),
		diffIdentityArchiveShapeRow(t, archiveKey, 4, 4, map[string]any{"external_userid": "external", "unionid": "union", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00.000000+00:00"}, nil),
	}}
	receipts := diffReceipts{rows: []DM01ExternalIdentityReceipt{diffReceipt(t, dm01Key, 1, 1, "imported")}}

	result, err := AggregateDM01ExternalIdentityArchiveOnly(context.Background(), archive, receipts, "v1-full-archive", 2, archiveKey, dm01Key)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if result.ArchiveRows != 4 || result.DM01TerminalRows != 1 || result.OnlyArchive != 3 || result.CanonicalSourceShape != 1 || result.RequiredFieldRedacted != 0 || result.ExternalUserIDMissingOrBlank != 1 || result.UnionIDMissingOrBlank != 0 || result.CorpIDMissingOrBlank != 0 || result.UpdatedAtMissingOrInvalid != 1 || result.SummaryDigest == ([sha256.Size]byte{}) {
		t.Fatalf("unexpected safe distribution: %+v", result)
	}
	again, err := AggregateDM01ExternalIdentityArchiveOnly(context.Background(), archive, receipts, "v1-full-archive", 2, archiveKey, dm01Key)
	if err != nil || again != result {
		t.Fatalf("distribution must be stable: %+v/%v", again, err)
	}
}

func TestAggregateDM01ExternalIdentityArchiveOnlyFailsClosedBeforeCounts(t *testing.T) {
	archiveKey := []byte("archive-source-key-32-bytes-long!!")
	dm01Key := []byte("dm01-source-key-32-bytes-long!!!!!")
	row := diffIdentityArchiveShapeRow(t, archiveKey, 1, 1, map[string]any{"external_userid": "external", "unionid": "union", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00Z"}, nil)
	row.PayloadHMAC[0]++
	_, err := AggregateDM01ExternalIdentityArchiveOnly(context.Background(), diffArchive{rows: []v1archive.ArchivedRow{row}}, diffReceipts{}, "v1-full-archive", 2, archiveKey, dm01Key)
	if !errors.Is(err, errInvalidDM01ExternalIdentityArchiveDiff) {
		t.Fatalf("tampered payload accepted: %v", err)
	}
	var failure *DM01ExternalIdentityArchiveDiffError
	if !errors.As(err, &failure) || failure.Stage != "archive_payload_hmac" {
		t.Fatalf("failure=%v", failure)
	}
}

func diffIdentityArchiveShapeRow(t *testing.T, key []byte, ordinal, id int64, fields map[string]any, redacted []string) v1archive.ArchivedRow {
	t.Helper()
	payload := make(map[string]any, len(fields)+1)
	for name, value := range fields {
		payload[name] = value
	}
	payload["id"] = id
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	canonical, paths, err := v1archive.RedactPayload(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if redacted != nil {
		paths = redacted
	}
	source, err := v1archive.SourceKeyHMAC(key, "wecom_external_contact_identity_map", []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest, err := v1archive.PayloadHMAC(key, "wecom_external_contact_identity_map", canonical)
	if err != nil {
		t.Fatal(err)
	}
	fieldDigest, err := v1archive.FieldHMAC(key, "wecom_external_contact_identity_map", paths)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: dm01ExternalIdentityArchiveTableID, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadDigest, FieldHMAC: fieldDigest, Payload: canonical, RedactedFields: paths}
}
