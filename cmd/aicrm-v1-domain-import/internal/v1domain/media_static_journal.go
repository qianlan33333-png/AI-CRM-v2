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
	if journal == nil || !journal.scope.valid() || journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TargetDomain != "media" {
		return false
	}
	return kind == media.HistoricalImage && journal.scope.TableID == "public/image_library" && journal.scope.TargetTable == "media_images" ||
		kind == media.HistoricalAttachment && journal.scope.TableID == "public/attachment_library" && journal.scope.TargetTable == "media_attachments"
}

func (journal *Journal) LoadHistoricalStatic(ctx context.Context, sourceIdentifier string) (media.HistoricalStaticReceipt, bool, error) {
	terminal, found, err := journal.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return media.HistoricalStaticReceipt{}, false, err
	}
	if terminal.Disposition != "import" {
		return media.HistoricalStaticReceipt{}, false, ErrConflict
	}
	var metadata struct {
		SourceID                string                     `json:"source_id"`
		Kind                    media.HistoricalStaticKind `json:"kind"`
		Checksum                string                     `json:"checksum"`
		DefinitionDigest        string                     `json:"definition_digest"`
		ProviderMaterialDropped bool                       `json:"provider_material_dropped"`
	}
	raw, err := json.Marshal(terminal.Metadata)
	if err != nil || json.Unmarshal(raw, &metadata) != nil || !journal.validMediaStaticScope(metadata.Kind) {
		return media.HistoricalStaticReceipt{}, false, ErrConflict
	}
	sourceID, sourceErr := strconv.ParseInt(metadata.SourceID, 10, 64)
	targetID, targetErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	checksum, checksumErr := ParseSourceIdentifier(metadata.Checksum)
	digest, digestErr := ParseSourceIdentifier(metadata.DefinitionDigest)
	if sourceErr != nil || targetErr != nil || checksumErr != nil || digestErr != nil || sourceID < 1 || targetID < 1 {
		return media.HistoricalStaticReceipt{}, false, ErrConflict
	}
	return media.HistoricalStaticReceipt{Origin: media.HistoricalStaticOrigin{SourceIdentifier: sourceIdentifier, SourceID: sourceID, PayloadDigest: terminal.PayloadDigest},
		Kind: metadata.Kind, TargetID: targetID, Checksum: checksum, DefinitionDigest: digest, ProviderMaterialDropped: metadata.ProviderMaterialDropped}, true, nil
}

func (journal *Journal) RecordHistoricalStatic(ctx context.Context, receipt media.HistoricalStaticReceipt) error {
	if !journal.validMediaStaticScope(receipt.Kind) || receipt.Replayed || receipt.TargetID < 1 || receipt.Origin.SourceID < 1 || receipt.Checksum == ([32]byte{}) || receipt.DefinitionDigest == ([32]byte{}) {
		return ErrInvalidScope
	}
	sourceKey, err := ParseSourceIdentifier(receipt.Origin.SourceIdentifier)
	if err != nil {
		return err
	}
	targetID := strconv.FormatInt(receipt.TargetID, 10)
	digest := sha256.Sum256([]byte("media\x00" + journal.scope.TargetTable + "\x00" + targetID + "\x00" + hex.EncodeToString(receipt.Origin.PayloadDigest[:])))
	return journal.Record(ctx, TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.Origin.PayloadDigest,
		Disposition: "import", TargetID: targetID, TargetDigest: digest, Metadata: map[string]any{
			"source_id": strconv.FormatInt(receipt.Origin.SourceID, 10), "kind": string(receipt.Kind),
			"checksum": hex.EncodeToString(receipt.Checksum[:]), "definition_digest": hex.EncodeToString(receipt.DefinitionDigest[:]),
			"provider_material_dropped": receipt.ProviderMaterialDropped,
		}})
}
