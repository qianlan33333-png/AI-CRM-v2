package v1domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type externalIdentityGapReconciliationReceipt struct {
	SourceKeyDigest [sha256.Size]byte
	PayloadDigest   [sha256.Size]byte
	FieldDigest     [sha256.Size]byte
	Terminal        TerminalReceipt
}

func validateExternalIdentityGapReconciliation(selection ExternalIdentityGapSelection, receipts []externalIdentityGapReconciliationReceipt) ([sha256.Size]byte, error) {
	if selection.ArchiveRows <= 0 || selection.DM01TerminalRows < 0 || selection.DM01TerminalRows+len(selection.OnlyArchive) != selection.ArchiveRows ||
		len(selection.OnlyArchive) == 0 || selection.SummaryDigest == ([sha256.Size]byte{}) || len(receipts) != len(selection.OnlyArchive) {
		return [sha256.Size]byte{}, ErrConflict
	}
	expected := make(map[[sha256.Size]byte]ExternalIdentityGapRow, len(selection.OnlyArchive))
	for _, value := range selection.OnlyArchive {
		if value.ArchivedRow.AdapterID != v1archive.DefaultAdapterID || value.ArchivedRow.TableID != dm01ExternalIdentityArchiveTableID ||
			value.ArchivedRow.SourceKeyHMAC == ([sha256.Size]byte{}) || value.ArchivedRow.PayloadHMAC == ([sha256.Size]byte{}) || value.ArchivedRow.FieldHMAC == ([sha256.Size]byte{}) {
			return [sha256.Size]byte{}, ErrConflict
		}
		if _, duplicate := expected[value.ArchivedRow.SourceKeyHMAC]; duplicate {
			return [sha256.Size]byte{}, ErrConflict
		}
		expected[value.ArchivedRow.SourceKeyHMAC] = value
	}

	type sealedReceipt struct {
		SourceKey string          `json:"source_key"`
		Payload   string          `json:"payload"`
		Field     string          `json:"field"`
		TargetID  string          `json:"target_id"`
		Target    string          `json:"target_digest"`
		Metadata  json.RawMessage `json:"metadata"`
	}
	sealed := make([]sealedReceipt, 0, len(receipts))
	targets := make(map[string]struct{}, len(receipts))
	seen := make(map[[sha256.Size]byte]struct{}, len(receipts))
	for _, receipt := range receipts {
		value, found := expected[receipt.SourceKeyDigest]
		if !found || receipt.PayloadDigest != value.ArchivedRow.PayloadHMAC || receipt.FieldDigest != value.ArchivedRow.FieldHMAC ||
			receipt.Terminal.SourceKeyDigest != receipt.SourceKeyDigest || receipt.Terminal.PayloadDigest != receipt.PayloadDigest ||
			receipt.Terminal.Disposition != "import" || receipt.Terminal.Reason != "" || receipt.Terminal.TargetDigest == ([sha256.Size]byte{}) || receipt.Terminal.Metadata == nil {
			return [sha256.Size]byte{}, ErrConflict
		}
		if _, duplicate := seen[receipt.SourceKeyDigest]; duplicate {
			return [sha256.Size]byte{}, ErrConflict
		}
		seen[receipt.SourceKeyDigest] = struct{}{}
		id, err := positiveID(receipt.Terminal.TargetID)
		if err != nil || strconv.FormatInt(id, 10) != receipt.Terminal.TargetID {
			return [sha256.Size]byte{}, ErrConflict
		}
		if _, duplicate := targets[receipt.Terminal.TargetID]; duplicate {
			return [sha256.Size]byte{}, ErrConflict
		}
		targets[receipt.Terminal.TargetID] = struct{}{}
		metadata, err := json.Marshal(receipt.Terminal.Metadata)
		if err != nil || !json.Valid(metadata) {
			return [sha256.Size]byte{}, ErrConflict
		}
		sealed = append(sealed, sealedReceipt{SourceKey: hex.EncodeToString(receipt.SourceKeyDigest[:]), Payload: hex.EncodeToString(receipt.PayloadDigest[:]),
			Field: hex.EncodeToString(receipt.FieldDigest[:]), TargetID: receipt.Terminal.TargetID,
			Target: hex.EncodeToString(receipt.Terminal.TargetDigest[:]), Metadata: metadata})
	}
	if len(seen) != len(expected) {
		return [sha256.Size]byte{}, ErrConflict
	}
	sort.Slice(sealed, func(left, right int) bool { return sealed[left].SourceKey < sealed[right].SourceKey })
	payload, err := json.Marshal(struct {
		Summary  string          `json:"selection_summary_digest"`
		Receipts []sealedReceipt `json:"receipts"`
	}{Summary: hex.EncodeToString(selection.SummaryDigest[:]), Receipts: sealed})
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("seal external identity gap: %w", err)
	}
	return sha256.Sum256(payload), nil
}
