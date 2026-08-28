package v1domain

import (
	"crypto/sha256"
	"errors"
	"strconv"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestValidateExternalIdentityGapReconciliationSealsCompleteSelection(t *testing.T) {
	t.Parallel()
	selection, receipts := externalIdentityGapReconcileFixture()
	first, err := validateExternalIdentityGapReconciliation(selection, receipts)
	if err != nil || first == ([sha256.Size]byte{}) {
		t.Fatalf("valid reconciliation rejected: digest=%x err=%v", first, err)
	}
	second, err := validateExternalIdentityGapReconciliation(selection, receipts)
	if err != nil || second != first {
		t.Fatalf("seal is not stable: first=%x second=%x err=%v", first, second, err)
	}
}

func TestValidateExternalIdentityGapReconciliationFailsClosed(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*ExternalIdentityGapSelection, []externalIdentityGapReconciliationReceipt){
		"wrong archive count": func(selection *ExternalIdentityGapSelection, _ []externalIdentityGapReconciliationReceipt) {
			selection.ArchiveRows--
		},
		"missing selection": func(_ *ExternalIdentityGapSelection, receipts []externalIdentityGapReconciliationReceipt) {
			receipts[0].SourceKeyDigest = [sha256.Size]byte{99}
		},
		"payload drift": func(_ *ExternalIdentityGapSelection, receipts []externalIdentityGapReconciliationReceipt) {
			receipts[0].PayloadDigest[0]++
		},
		"field drift": func(_ *ExternalIdentityGapSelection, receipts []externalIdentityGapReconciliationReceipt) {
			receipts[0].FieldDigest[0]++
		},
		"zero target digest": func(_ *ExternalIdentityGapSelection, receipts []externalIdentityGapReconciliationReceipt) {
			receipts[0].Terminal.TargetDigest = [sha256.Size]byte{}
		},
		"duplicate target": func(_ *ExternalIdentityGapSelection, receipts []externalIdentityGapReconciliationReceipt) {
			receipts[1].Terminal.TargetID = receipts[0].Terminal.TargetID
		},
		"missing receipt": func(_ *ExternalIdentityGapSelection, receipts []externalIdentityGapReconciliationReceipt) {
			receipts[len(receipts)-1] = externalIdentityGapReconciliationReceipt{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			selection, receipts := externalIdentityGapReconcileFixture()
			mutate(&selection, receipts)
			if _, err := validateExternalIdentityGapReconciliation(selection, receipts); !errors.Is(err, ErrConflict) {
				t.Fatalf("drift accepted: %v", err)
			}
		})
	}
}

func TestValidateExternalIdentityGapReconciliationIncludesTargetDigestInSeal(t *testing.T) {
	t.Parallel()
	selection, receipts := externalIdentityGapReconcileFixture()
	first, err := validateExternalIdentityGapReconciliation(selection, receipts)
	if err != nil {
		t.Fatalf("initial seal: %v", err)
	}
	receipts[0].Terminal.TargetDigest[0]++
	second, err := validateExternalIdentityGapReconciliation(selection, receipts)
	if err != nil || second == first {
		t.Fatalf("target digest was not sealed: first=%x second=%x err=%v", first, second, err)
	}
}

func externalIdentityGapReconcileFixture() (ExternalIdentityGapSelection, []externalIdentityGapReconciliationReceipt) {
	rows := make([]ExternalIdentityGapRow, 63)
	receipts := make([]externalIdentityGapReconciliationReceipt, 63)
	for index := range rows {
		key := sha256.Sum256([]byte{byte(index), 1})
		payload := sha256.Sum256([]byte{byte(index), 2})
		field := sha256.Sum256([]byte{byte(index), 3})
		target := sha256.Sum256([]byte{byte(index), 4})
		rows[index] = ExternalIdentityGapRow{ArchivedRow: v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: dm01ExternalIdentityArchiveTableID, SourceKeyHMAC: key, PayloadHMAC: payload, FieldHMAC: field}}
		receipts[index] = externalIdentityGapReconciliationReceipt{SourceKeyDigest: key, PayloadDigest: payload, FieldDigest: field,
			Terminal: TerminalReceipt{SourceKeyDigest: key, PayloadDigest: payload, Disposition: "import", TargetID: positiveExternalIdentityGapReconcileID(index + 1), TargetDigest: target, Metadata: map[string]any{"root_route": "unbound", "hmac_key_version": "1"}}}
	}
	return ExternalIdentityGapSelection{ArchiveRows: len(rows), DM01TerminalRows: 0, OnlyArchive: rows, SummaryDigest: sha256.Sum256([]byte("selection"))}, receipts
}

func positiveExternalIdentityGapReconcileID(value int) string {
	return strconv.Itoa(value)
}
