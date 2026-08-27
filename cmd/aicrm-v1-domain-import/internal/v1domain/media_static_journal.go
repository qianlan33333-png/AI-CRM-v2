package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var _ media.HistoricalStaticJournal = (*Journal)(nil)

func (journal *Journal) validMediaStaticScope(kind media.HistoricalStaticKind) bool {
	if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TargetDomain != "media" {
		return false
	}
	return kind == media.HistoricalImage && journal.scope.TableID == "public/image_library" && journal.scope.TargetTable == "media_images" ||
		kind == media.HistoricalAttachment && journal.scope.TableID == "public/attachment_library" && journal.scope.TargetTable == "media_attachments"
}

func (journal *Journal) LoadHistoricalStatic(ctx context.Context, sourceIdentifier string) (media.HistoricalStaticReceipt, bool, error) {
	if ctx == nil || (!journal.validMediaStaticScope(media.HistoricalImage) && !journal.validMediaStaticScope(media.HistoricalAttachment)) {
		return media.HistoricalStaticReceipt{}, false, ErrInvalidScope
	}
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	if err != nil || sourceKey == [sha256.Size]byte{} {
		return media.HistoricalStaticReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return media.HistoricalStaticReceipt{}, false, err
	}
	if terminal.Disposition != "import" || terminal.PayloadDigest == [sha256.Size]byte{} || len(terminal.Metadata) != 5 ||
		terminal.TargetDigest != mediaStaticTargetDigest(journal.scope.TargetTable, terminal.TargetID, terminal.PayloadDigest) {
		return media.HistoricalStaticReceipt{}, false, ErrConflict
	}
	var metadata struct {
		SourceID                string                     `json:"source_id"`
		Kind                    media.HistoricalStaticKind `json:"kind"`
		Checksum                string                     `json:"checksum"`
		DefinitionDigest        string                     `json:"definition_digest"`
		ProviderMaterialDropped *bool                      `json:"provider_material_dropped"`
	}
	raw, err := json.Marshal(terminal.Metadata)
	if err != nil || json.Unmarshal(raw, &metadata) != nil || !journal.validMediaStaticScope(metadata.Kind) || metadata.ProviderMaterialDropped == nil {
		return media.HistoricalStaticReceipt{}, false, ErrConflict
	}
	sourceID, sourceErr := strconv.ParseInt(metadata.SourceID, 10, 64)
	targetID, targetErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	checksum, checksumErr := ParseSourceIdentifier(metadata.Checksum)
	digest, digestErr := ParseSourceIdentifier(metadata.DefinitionDigest)
	if sourceErr != nil || targetErr != nil || checksumErr != nil || digestErr != nil || sourceID < 1 || targetID < 1 ||
		strconv.FormatInt(sourceID, 10) != metadata.SourceID || strconv.FormatInt(targetID, 10) != terminal.TargetID || checksum == [sha256.Size]byte{} || digest == [sha256.Size]byte{} {
		return media.HistoricalStaticReceipt{}, false, ErrConflict
	}
	return media.HistoricalStaticReceipt{Origin: media.HistoricalStaticOrigin{SourceIdentifier: sourceIdentifier, SourceID: sourceID, PayloadDigest: terminal.PayloadDigest},
		Kind: metadata.Kind, TargetID: targetID, Checksum: checksum, DefinitionDigest: digest, ProviderMaterialDropped: *metadata.ProviderMaterialDropped}, true, nil
}

func (journal *Journal) RecordHistoricalStatic(ctx context.Context, receipt media.HistoricalStaticReceipt) error {
	if ctx == nil || !journal.validMediaStaticScope(receipt.Kind) || receipt.Replayed || receipt.TargetID < 1 || receipt.Origin.SourceID < 1 || receipt.Origin.PayloadDigest == [sha256.Size]byte{} || receipt.Checksum == ([32]byte{}) || receipt.DefinitionDigest == ([32]byte{}) {
		return ErrInvalidScope
	}
	sourceKey, err := ParseSourceIdentifier(receipt.Origin.SourceIdentifier)
	if err != nil || sourceKey == [sha256.Size]byte{} {
		return ErrInvalidScope
	}
	targetID := strconv.FormatInt(receipt.TargetID, 10)
	digest := mediaStaticTargetDigest(journal.scope.TargetTable, targetID, receipt.Origin.PayloadDigest)
	return journal.Record(ctx, TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.Origin.PayloadDigest,
		Disposition: "import", TargetID: targetID, TargetDigest: digest, Metadata: map[string]any{
			"source_id": strconv.FormatInt(receipt.Origin.SourceID, 10), "kind": string(receipt.Kind),
			"checksum": hex.EncodeToString(receipt.Checksum[:]), "definition_digest": hex.EncodeToString(receipt.DefinitionDigest[:]),
			"provider_material_dropped": receipt.ProviderMaterialDropped,
		}})
}

func mediaStaticTargetDigest(table, targetID string, payload [sha256.Size]byte) [sha256.Size]byte {
	return sha256.Sum256([]byte("media\x00" + table + "\x00" + targetID + "\x00" + hex.EncodeToString(payload[:])))
}
