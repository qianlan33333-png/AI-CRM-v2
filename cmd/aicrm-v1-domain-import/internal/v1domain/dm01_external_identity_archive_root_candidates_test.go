package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type dm01RootSnapshotVerifier struct {
	verified map[[sha256.Size]byte]bool
	calls    [][sha256.Size]byte
	err      error
}

func (verifier *dm01RootSnapshotVerifier) VerifyDM01CustomerIdentitySnapshot(_ context.Context, _ int64, key [sha256.Size]byte) (bool, error) {
	verifier.calls = append(verifier.calls, key)
	if verifier.err != nil {
		return false, verifier.err
	}
	return verifier.verified[key], nil
}

func TestAggregateDM01ExternalIdentityArchiveOnlyRootCandidatesAuthenticatesAndClassifies(t *testing.T) {
	archiveKey := []byte("archive-source-key-32-bytes-long!!")
	dm01Key := []byte("dm01-source-key-32-bytes-long!!!!!")
	archive := diffArchive{rows: []v1archive.ArchivedRow{
		diffIdentityArchiveShapeRow(t, archiveKey, 1, 1, map[string]any{"external_userid": "already", "unionid": "union-already", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00Z"}, nil),
		diffIdentityArchiveShapeRow(t, archiveKey, 2, 2, map[string]any{"external_userid": "blank-union", "unionid": "", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00Z"}, nil),
		diffIdentityArchiveShapeRow(t, archiveKey, 3, 3, map[string]any{"external_userid": "root-ok", "unionid": "union-ok", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00Z"}, nil),
		diffIdentityArchiveShapeRow(t, archiveKey, 4, 4, map[string]any{"external_userid": "root-missing", "unionid": "union-missing", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00Z"}, nil),
		diffIdentityArchiveShapeRow(t, archiveKey, 5, 5, map[string]any{"external_userid": "", "unionid": "union-shape", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00Z"}, nil),
	}}
	okKey := mustDM01CustomerKey(t, dm01Key, "union-ok")
	roots := &dm01RootSnapshotVerifier{verified: map[[sha256.Size]byte]bool{okKey: true}}

	result, err := AggregateDM01ExternalIdentityArchiveOnlyRootCandidates(context.Background(), archive, diffReceipts{rows: []DM01ExternalIdentityReceipt{diffReceipt(t, dm01Key, 1, 1, "imported")}}, roots, "v1-full-archive", 2, archiveKey, dm01Key)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if result.ArchiveRows != 5 || result.DM01TerminalRows != 1 || result.OnlyArchive != 4 || result.UnionIDNotVerifiable != 1 || result.SourceShapeNotEligible != 1 || result.CustomerRootVerified != 1 || result.CustomerRootMissing != 1 || result.SummaryDigest == ([sha256.Size]byte{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(roots.calls) != 2 {
		t.Fatalf("root verifier calls=%d, want only two canonical rows", len(roots.calls))
	}
	again, err := AggregateDM01ExternalIdentityArchiveOnlyRootCandidates(context.Background(), archive, diffReceipts{rows: []DM01ExternalIdentityReceipt{diffReceipt(t, dm01Key, 1, 1, "imported")}}, roots, "v1-full-archive", 2, archiveKey, dm01Key)
	if err != nil || again != result {
		t.Fatalf("stable aggregate=%+v err=%v", again, err)
	}
}

func TestAggregateDM01ExternalIdentityArchiveOnlyRootCandidatesFailsClosedOnRootReader(t *testing.T) {
	archiveKey := []byte("archive-source-key-32-bytes-long!!")
	dm01Key := []byte("dm01-source-key-32-bytes-long!!!!!")
	archive := diffArchive{rows: []v1archive.ArchivedRow{
		diffIdentityArchiveShapeRow(t, archiveKey, 1, 1, map[string]any{"external_userid": "external", "unionid": "union", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00Z"}, nil),
	}}
	_, err := AggregateDM01ExternalIdentityArchiveOnlyRootCandidates(context.Background(), archive, diffReceipts{}, &dm01RootSnapshotVerifier{err: errors.New("unavailable")}, "v1-full-archive", 2, archiveKey, dm01Key)
	var failure *DM01ExternalIdentityArchiveDiffError
	if !errors.As(err, &failure) || failure.Stage != "customer_root_snapshot" {
		t.Fatalf("failure=%v", err)
	}
}

func TestAggregateDM01ExternalIdentityArchiveOnlyRootCandidatesRejectsTamperedArchiveBeforeRootLookup(t *testing.T) {
	archiveKey := []byte("archive-source-key-32-bytes-long!!")
	dm01Key := []byte("dm01-source-key-32-bytes-long!!!!!")
	row := diffIdentityArchiveShapeRow(t, archiveKey, 1, 1, map[string]any{"external_userid": "external", "unionid": "union", "corp_id": "corp", "updated_at": "2026-08-26T11:00:00Z"}, nil)
	row.PayloadHMAC[0]++
	roots := &dm01RootSnapshotVerifier{}
	_, err := AggregateDM01ExternalIdentityArchiveOnlyRootCandidates(context.Background(), diffArchive{rows: []v1archive.ArchivedRow{row}}, diffReceipts{}, roots, "v1-full-archive", 2, archiveKey, dm01Key)
	var failure *DM01ExternalIdentityArchiveDiffError
	if !errors.As(err, &failure) || failure.Stage != "archive_payload_hmac" || len(roots.calls) != 0 {
		t.Fatalf("failure=%v calls=%d", err, len(roots.calls))
	}
}

func mustDM01CustomerKey(t *testing.T, key []byte, unionID string) [sha256.Size]byte {
	t.Helper()
	value, err := contactmigration.SourceKeyHMAC(key, dm01CustomerIdentitySourceTable, unionID)
	if err != nil || len(value) != sha256.Size {
		t.Fatalf("customer source key: %v", err)
	}
	var result [sha256.Size]byte
	copy(result[:], value)
	return result
}
