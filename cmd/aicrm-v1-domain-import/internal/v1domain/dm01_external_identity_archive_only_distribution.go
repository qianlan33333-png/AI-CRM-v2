package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// DM01ExternalIdentityArchiveOnlyDistribution contains only aggregate source
// shape evidence for authenticated archive rows absent from a DM01 run. It
// intentionally has no source identifier, HMAC, or source field value.
type DM01ExternalIdentityArchiveOnlyDistribution struct {
	ArchiveRows                  int64
	DM01TerminalRows             int64
	OnlyArchive                  int64
	CanonicalSourceShape         int64
	RequiredFieldRedacted        int64
	ExternalUserIDMissingOrBlank int64
	UnionIDMissingOrBlank        int64
	CorpIDMissingOrBlank         int64
	UpdatedAtMissingOrInvalid    int64
	SummaryDigest                [sha256.Size]byte
}

type dm01ExternalIdentityArchiveShape struct {
	requiredFieldRedacted        bool
	externalUserIDMissingOrBlank bool
	unionIDMissingOrBlank        bool
	corpIDMissingOrBlank         bool
	updatedAtMissingOrInvalid    bool
}

// AggregateDM01ExternalIdentityArchiveOnly reads the same immutable inputs as
// DiffDM01ExternalIdentityArchive, but reports safe shape counts for the
// archive-only subset. It never persists or exposes a source identity.
func AggregateDM01ExternalIdentityArchiveOnly(
	ctx context.Context,
	archive ArchiveSource,
	receipts DM01ExternalIdentityReceiptSource,
	archiveRunID string,
	dm01RunID int64,
	archiveSourceHMACKey []byte,
	dm01SourceHMACKey []byte,
) (DM01ExternalIdentityArchiveOnlyDistribution, error) {
	if archive == nil || receipts == nil || archiveRunID == "" || dm01RunID < 1 || len(archiveSourceHMACKey) < sha256.Size || len(dm01SourceHMACKey) < sha256.Size {
		return DM01ExternalIdentityArchiveOnlyDistribution{}, dm01ExternalIdentityArchiveDiffFailure("scope")
	}

	archiveShapes := make(map[[sha256.Size]byte]dm01ExternalIdentityArchiveShape)
	var nextArchiveOrdinal int64 = 1
	if err := archive.EachTableRow(ctx, archiveRunID, dm01ExternalIdentityArchiveTableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != dm01ExternalIdentityArchiveTableID || row.SourceOrdinal != nextArchiveOrdinal || zeroDigest(row.SourceKeyHMAC) || zeroDigest(row.PayloadHMAC) || zeroDigest(row.FieldHMAC) {
			return dm01ExternalIdentityArchiveDiffFailure("archive_envelope")
		}
		nextArchiveOrdinal++
		id, stage := verifyExternalIdentityArchiveRow(row, archiveSourceHMACKey)
		if stage != "" {
			return dm01ExternalIdentityArchiveDiffFailure(stage)
		}
		shape, stage := dm01ExternalIdentityArchiveOnlyShape(row)
		if stage != "" {
			return dm01ExternalIdentityArchiveDiffFailure(stage)
		}
		key, err := contactmigration.SourceKeyHMAC(dm01SourceHMACKey, "wecom_external_contact_identity_map", strconv.FormatInt(id, 10))
		if err != nil || len(key) != sha256.Size {
			return dm01ExternalIdentityArchiveDiffFailure("dm01_source_hmac")
		}
		var digest [sha256.Size]byte
		copy(digest[:], key)
		if _, exists := archiveShapes[digest]; exists {
			return dm01ExternalIdentityArchiveDiffFailure("archive_duplicate")
		}
		archiveShapes[digest] = shape
		return nil
	}); err != nil {
		var failure *DM01ExternalIdentityArchiveDiffError
		if errors.As(err, &failure) {
			return DM01ExternalIdentityArchiveOnlyDistribution{}, failure
		}
		return DM01ExternalIdentityArchiveOnlyDistribution{}, dm01ExternalIdentityArchiveDiffFailure("archive_stream")
	}

	dm01Keys := make(map[[sha256.Size]byte]struct{})
	var nextReceiptOrdinal int64 = 1
	if err := receipts.EachDM01ExternalIdentityReceipt(ctx, dm01RunID, func(receipt DM01ExternalIdentityReceipt) error {
		if receipt.SourceOrdinal != nextReceiptOrdinal || zeroDigest(receipt.SourceKeyHMAC) || (receipt.Disposition != "imported" && receipt.Disposition != "quarantined") {
			return dm01ExternalIdentityArchiveDiffFailure("receipt_envelope")
		}
		nextReceiptOrdinal++
		if _, exists := dm01Keys[receipt.SourceKeyHMAC]; exists {
			return dm01ExternalIdentityArchiveDiffFailure("receipt_duplicate")
		}
		dm01Keys[receipt.SourceKeyHMAC] = struct{}{}
		return nil
	}); err != nil {
		var failure *DM01ExternalIdentityArchiveDiffError
		if errors.As(err, &failure) {
			return DM01ExternalIdentityArchiveOnlyDistribution{}, failure
		}
		return DM01ExternalIdentityArchiveOnlyDistribution{}, dm01ExternalIdentityArchiveDiffFailure("receipt_stream")
	}

	result := DM01ExternalIdentityArchiveOnlyDistribution{ArchiveRows: int64(len(archiveShapes)), DM01TerminalRows: int64(len(dm01Keys))}
	for key, shape := range archiveShapes {
		if _, found := dm01Keys[key]; found {
			continue
		}
		result.OnlyArchive++
		result.add(shape)
	}
	result.SummaryDigest = dm01ExternalIdentityArchiveOnlyDistributionDigest(result)
	return result, nil
}

func (distribution *DM01ExternalIdentityArchiveOnlyDistribution) add(shape dm01ExternalIdentityArchiveShape) {
	if shape.requiredFieldRedacted {
		distribution.RequiredFieldRedacted++
	}
	if shape.externalUserIDMissingOrBlank {
		distribution.ExternalUserIDMissingOrBlank++
	}
	if shape.unionIDMissingOrBlank {
		distribution.UnionIDMissingOrBlank++
	}
	if shape.corpIDMissingOrBlank {
		distribution.CorpIDMissingOrBlank++
	}
	if shape.updatedAtMissingOrInvalid {
		distribution.UpdatedAtMissingOrInvalid++
	}
	if !shape.requiredFieldRedacted && !shape.externalUserIDMissingOrBlank && !shape.unionIDMissingOrBlank && !shape.corpIDMissingOrBlank && !shape.updatedAtMissingOrInvalid {
		distribution.CanonicalSourceShape++
	}
}

func dm01ExternalIdentityArchiveOnlyShape(row v1archive.ArchivedRow) (dm01ExternalIdentityArchiveShape, string) {
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return dm01ExternalIdentityArchiveShape{}, "archive_payload_shape"
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return dm01ExternalIdentityArchiveShape{}, "archive_payload_shape"
	}
	shape := dm01ExternalIdentityArchiveShape{
		requiredFieldRedacted: v1archive.IsRedacted(row, "external_userid") || v1archive.IsRedacted(row, "unionid") || v1archive.IsRedacted(row, "corp_id") || v1archive.IsRedacted(row, "updated_at"),
	}
	if !v1archive.IsRedacted(row, "external_userid") {
		shape.externalUserIDMissingOrBlank = !dm01ExternalIdentityArchiveOnlyNonBlank(payload["external_userid"])
	}
	if !v1archive.IsRedacted(row, "unionid") {
		shape.unionIDMissingOrBlank = !dm01ExternalIdentityArchiveOnlyNonBlank(payload["unionid"])
	}
	if !v1archive.IsRedacted(row, "corp_id") {
		shape.corpIDMissingOrBlank = !dm01ExternalIdentityArchiveOnlyNonBlank(payload["corp_id"])
	}
	if !v1archive.IsRedacted(row, "updated_at") {
		shape.updatedAtMissingOrInvalid = !dm01ExternalIdentityArchiveOnlyTimestamp(payload["updated_at"])
	}
	return shape, ""
}

func dm01ExternalIdentityArchiveOnlyNonBlank(value any) bool {
	text, ok := value.(string)
	return ok && text != "" && strings.TrimSpace(text) == text
}

func dm01ExternalIdentityArchiveOnlyTimestamp(value any) bool {
	text, ok := value.(string)
	if !ok || text == "" || strings.TrimSpace(text) != text {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, text)
	return err == nil
}

func dm01ExternalIdentityArchiveOnlyDistributionDigest(value DM01ExternalIdentityArchiveOnlyDistribution) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("aicrm/dm01/external-identity-archive-only-distribution/v1"))
	for _, count := range []int64{value.ArchiveRows, value.DM01TerminalRows, value.OnlyArchive, value.CanonicalSourceShape, value.RequiredFieldRedacted, value.ExternalUserIDMissingOrBlank, value.UnionIDMissingOrBlank, value.CorpIDMissingOrBlank, value.UpdatedAtMissingOrInvalid} {
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], uint64(count))
		_, _ = hash.Write(encoded[:])
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}
