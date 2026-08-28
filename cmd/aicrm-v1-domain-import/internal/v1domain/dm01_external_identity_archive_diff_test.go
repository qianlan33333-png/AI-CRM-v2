package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestDiffDM01ExternalIdentityArchiveAggregatesOnly(t *testing.T) {
	t.Parallel()
	archiveKey := []byte("archive-source-key-32-bytes-long!!")
	dm01Key := []byte("dm01-source-key-32-bytes-long!!!!!")
	archive := diffArchive{rows: []v1archive.ArchivedRow{
		diffArchiveRow(t, archiveKey, 1, 1),
		diffArchiveRow(t, archiveKey, 2, 2),
		diffArchiveRow(t, archiveKey, 3, 3),
	}}
	receipts := diffReceipts{rows: []DM01ExternalIdentityReceipt{
		diffReceipt(t, dm01Key, 1, 2, "imported"),
		diffReceipt(t, dm01Key, 2, 3, "quarantined"),
		diffReceipt(t, dm01Key, 3, 4, "imported"),
	}}

	result, err := DiffDM01ExternalIdentityArchive(context.Background(), archive, receipts, "v1-full-archive", 2, archiveKey, dm01Key)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if result.ArchiveRows != 3 || result.DM01TerminalRows != 3 || result.Intersection != 2 || result.OnlyArchive != 1 || result.OnlyDM01 != 1 || zeroDigest(result.SummaryDigest) {
		t.Fatalf("unexpected aggregate result: %+v", result)
	}

	again, err := DiffDM01ExternalIdentityArchive(context.Background(), archive, receipts, "v1-full-archive", 2, archiveKey, dm01Key)
	if err != nil || again.SummaryDigest != result.SummaryDigest {
		t.Fatalf("diff digest must be stable: result=%+v err=%v", again, err)
	}
}

func TestDiffDM01ExternalIdentityArchiveFailsClosed(t *testing.T) {
	t.Parallel()
	archiveKey := []byte("archive-source-key-32-bytes-long!!")
	dm01Key := []byte("dm01-source-key-32-bytes-long!!!!!")
	valid := diffArchiveRow(t, archiveKey, 1, 1)
	validReceipt := diffReceipt(t, dm01Key, 1, 1, "imported")

	for name, test := range map[string]struct {
		stage  string
		change func(*v1archive.ArchivedRow, *DM01ExternalIdentityReceipt)
	}{
		"source digest drift":  {"archive_source_hmac", func(row *v1archive.ArchivedRow, _ *DM01ExternalIdentityReceipt) { row.SourceKeyHMAC[0]++ }},
		"payload digest drift": {"archive_payload_hmac", func(row *v1archive.ArchivedRow, _ *DM01ExternalIdentityReceipt) { row.PayloadHMAC[0]++ }},
		"field digest drift":   {"archive_field_hmac", func(row *v1archive.ArchivedRow, _ *DM01ExternalIdentityReceipt) { row.FieldHMAC[0]++ }},
		"redacted id":          {"archive_id", func(row *v1archive.ArchivedRow, _ *DM01ExternalIdentityReceipt) { row.RedactedFields = []string{"id"} }},
		"noncanonical id":      {"archive_payload_hmac", func(row *v1archive.ArchivedRow, _ *DM01ExternalIdentityReceipt) { row.Payload = []byte(`{"id":1.0}`) }},
		"receipt ordinal gap":  {"receipt_envelope", func(_ *v1archive.ArchivedRow, receipt *DM01ExternalIdentityReceipt) { receipt.SourceOrdinal = 2 }},
		"receipt disposition":  {"receipt_envelope", func(_ *v1archive.ArchivedRow, receipt *DM01ExternalIdentityReceipt) { receipt.Disposition = "skipped" }},
	} {
		t.Run(name, func(t *testing.T) {
			row, receipt := valid, validReceipt
			test.change(&row, &receipt)
			_, err := DiffDM01ExternalIdentityArchive(context.Background(), diffArchive{rows: []v1archive.ArchivedRow{row}}, diffReceipts{rows: []DM01ExternalIdentityReceipt{receipt}}, "v1-full-archive", 2, archiveKey, dm01Key)
			if !errors.Is(err, errInvalidDM01ExternalIdentityArchiveDiff) {
				t.Fatalf("expected generic invalid-diff error, got %v", err)
			}
			var failure *DM01ExternalIdentityArchiveDiffError
			if !errors.As(err, &failure) || failure.Stage != test.stage {
				t.Fatalf("stage=%v want=%s", failure, test.stage)
			}
		})
	}
}

func TestDiffDM01ExternalIdentityArchiveRejectsDuplicateAndWrongKeys(t *testing.T) {
	t.Parallel()
	archiveKey := []byte("archive-source-key-32-bytes-long!!")
	dm01Key := []byte("dm01-source-key-32-bytes-long!!!!!")
	valid := diffArchiveRow(t, archiveKey, 1, 1)
	receipt := diffReceipt(t, dm01Key, 1, 1, "imported")

	duplicate := valid
	duplicate.SourceOrdinal = 2
	if _, err := DiffDM01ExternalIdentityArchive(context.Background(), diffArchive{rows: []v1archive.ArchivedRow{valid, duplicate}}, diffReceipts{rows: []DM01ExternalIdentityReceipt{receipt}}, "v1-full-archive", 2, archiveKey, dm01Key); !errors.Is(err, errInvalidDM01ExternalIdentityArchiveDiff) {
		t.Fatalf("duplicate archive must fail closed: %v", err)
	}
	if _, err := DiffDM01ExternalIdentityArchive(context.Background(), diffArchive{rows: []v1archive.ArchivedRow{valid}}, diffReceipts{rows: []DM01ExternalIdentityReceipt{receipt}}, "v1-full-archive", 2, []byte("other-archive-source-key-32-bytes!"), dm01Key); !errors.Is(err, errInvalidDM01ExternalIdentityArchiveDiff) {
		t.Fatalf("wrong archive key must fail closed: %v", err)
	}
	result, err := DiffDM01ExternalIdentityArchive(context.Background(), diffArchive{rows: []v1archive.ArchivedRow{valid}}, diffReceipts{rows: []DM01ExternalIdentityReceipt{receipt}}, "v1-full-archive", 2, archiveKey, []byte("other-dm01-source-key-32-bytes!!!!"))
	if err != nil {
		t.Fatalf("different DM01 key comparison: %v", err)
	}
	if result.Intersection != 0 || result.OnlyArchive != 1 || result.OnlyDM01 != 1 {
		t.Fatalf("different DM01 key must not produce a false match: %+v", result)
	}
}

type diffArchive struct{ rows []v1archive.ArchivedRow }

func (source diffArchive) EachTableRow(_ context.Context, _ string, table string, callback func(v1archive.ArchivedRow) error) error {
	if table != dm01ExternalIdentityArchiveTableID {
		return errInvalidDM01ExternalIdentityArchiveDiff
	}
	for _, row := range source.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type diffReceipts struct{ rows []DM01ExternalIdentityReceipt }

func (source diffReceipts) EachDM01ExternalIdentityReceipt(_ context.Context, _ int64, callback func(DM01ExternalIdentityReceipt) error) error {
	for _, row := range source.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

func diffArchiveRow(t *testing.T, key []byte, ordinal, id int64) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": id, "status": "active"})
	if err != nil {
		t.Fatal(err)
	}
	canonical, fields, err := v1archive.RedactPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	source, err := v1archive.SourceKeyHMAC(key, "wecom_external_contact_identity_map", []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(key, "wecom_external_contact_identity_map", canonical)
	if err != nil {
		t.Fatal(err)
	}
	fieldHMAC, err := v1archive.FieldHMAC(key, "wecom_external_contact_identity_map", fields)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: dm01ExternalIdentityArchiveTableID, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: fieldHMAC, Payload: canonical, RedactedFields: fields}
}

func diffReceipt(t *testing.T, key []byte, ordinal, id int64, disposition string) DM01ExternalIdentityReceipt {
	t.Helper()
	digest, err := contactmigration.SourceKeyHMAC(key, "wecom_external_contact_identity_map", strconv.FormatInt(id, 10))
	if err != nil || len(digest) != sha256.Size {
		t.Fatalf("receipt digest: %v", err)
	}
	var value [sha256.Size]byte
	copy(value[:], digest)
	return DM01ExternalIdentityReceipt{SourceOrdinal: ordinal, SourceKeyHMAC: value, Disposition: disposition}
}
