package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// DM01CustomerIdentitySnapshotVerifier is the narrow, caller-owned read-only
// proof for an existing DM01 customer root. It intentionally exposes no
// customer ID or source value.
type DM01CustomerIdentitySnapshotVerifier interface {
	VerifyDM01CustomerIdentitySnapshot(context.Context, int64, [sha256.Size]byte) (bool, error)
}

// DM01ExternalIdentityArchiveOnlyRootCandidates reports only safe aggregate
// evidence for archive rows absent from DM01. A verified root is not an
// identity import result: a later transaction-bound writer must still decide
// the scoped identity conflict and bind.
type DM01ExternalIdentityArchiveOnlyRootCandidates struct {
	ArchiveRows            int64
	DM01TerminalRows       int64
	OnlyArchive            int64
	UnionIDNotVerifiable   int64
	SourceShapeNotEligible int64
	CustomerRootVerified   int64
	CustomerRootMissing    int64
	SummaryDigest          [sha256.Size]byte
}

// AggregateDM01ExternalIdentityArchiveOnlyRootCandidates authenticates the
// archive-only set before it asks the narrow root verifier. Empty union IDs
// are deliberately not transformed into a fallback source key.
func AggregateDM01ExternalIdentityArchiveOnlyRootCandidates(
	ctx context.Context,
	archive ArchiveSource,
	receipts DM01ExternalIdentityReceiptSource,
	roots DM01CustomerIdentitySnapshotVerifier,
	archiveRunID string,
	dm01RunID int64,
	archiveSourceHMACKey []byte,
	dm01SourceHMACKey []byte,
) (DM01ExternalIdentityArchiveOnlyRootCandidates, error) {
	if archive == nil || receipts == nil || roots == nil || archiveRunID == "" || dm01RunID < 1 || len(archiveSourceHMACKey) < sha256.Size || len(dm01SourceHMACKey) < sha256.Size {
		return DM01ExternalIdentityArchiveOnlyRootCandidates{}, dm01ExternalIdentityArchiveDiffFailure("scope")
	}

	type archived struct {
		shape   dm01ExternalIdentityArchiveShape
		unionID string
	}
	archiveRows := make(map[[sha256.Size]byte]archived)
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
		if _, exists := archiveRows[digest]; exists {
			return dm01ExternalIdentityArchiveDiffFailure("archive_duplicate")
		}
		archiveRows[digest] = archived{shape: shape, unionID: dm01ExternalIdentityArchiveOnlyUnionID(row)}
		return nil
	}); err != nil {
		var failure *DM01ExternalIdentityArchiveDiffError
		if errors.As(err, &failure) {
			return DM01ExternalIdentityArchiveOnlyRootCandidates{}, failure
		}
		return DM01ExternalIdentityArchiveOnlyRootCandidates{}, dm01ExternalIdentityArchiveDiffFailure("archive_stream")
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
			return DM01ExternalIdentityArchiveOnlyRootCandidates{}, failure
		}
		return DM01ExternalIdentityArchiveOnlyRootCandidates{}, dm01ExternalIdentityArchiveDiffFailure("receipt_stream")
	}

	result := DM01ExternalIdentityArchiveOnlyRootCandidates{ArchiveRows: int64(len(archiveRows)), DM01TerminalRows: int64(len(dm01Keys))}
	for sourceKey, row := range archiveRows {
		if _, found := dm01Keys[sourceKey]; found {
			continue
		}
		result.OnlyArchive++
		if row.shape.unionIDMissingOrBlank {
			result.UnionIDNotVerifiable++
			continue
		}
		if row.shape.requiredFieldRedacted || row.shape.externalUserIDMissingOrBlank || row.shape.corpIDMissingOrBlank || row.shape.updatedAtMissingOrInvalid {
			result.SourceShapeNotEligible++
			continue
		}
		customerKey, err := contactmigration.SourceKeyHMAC(dm01SourceHMACKey, dm01CustomerIdentitySourceTable, row.unionID)
		if err != nil || len(customerKey) != sha256.Size {
			return DM01ExternalIdentityArchiveOnlyRootCandidates{}, dm01ExternalIdentityArchiveDiffFailure("customer_source_hmac")
		}
		var customerDigest [sha256.Size]byte
		copy(customerDigest[:], customerKey)
		verified, err := roots.VerifyDM01CustomerIdentitySnapshot(ctx, dm01RunID, customerDigest)
		if err != nil {
			return DM01ExternalIdentityArchiveOnlyRootCandidates{}, dm01ExternalIdentityArchiveDiffFailure("customer_root_snapshot")
		}
		if verified {
			result.CustomerRootVerified++
		} else {
			result.CustomerRootMissing++
		}
	}
	if result.OnlyArchive != result.UnionIDNotVerifiable+result.SourceShapeNotEligible+result.CustomerRootVerified+result.CustomerRootMissing {
		return DM01ExternalIdentityArchiveOnlyRootCandidates{}, dm01ExternalIdentityArchiveDiffFailure("candidate_conservation")
	}
	result.SummaryDigest = dm01ExternalIdentityArchiveOnlyRootCandidateDigest(result)
	return result, nil
}

func dm01ExternalIdentityArchiveOnlyUnionID(row v1archive.ArchivedRow) string {
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	decoder.UseNumber()
	var payload map[string]any
	if decoder.Decode(&payload) != nil {
		return ""
	}
	value, _ := payload["unionid"].(string)
	return value
}

func dm01ExternalIdentityArchiveOnlyRootCandidateDigest(value DM01ExternalIdentityArchiveOnlyRootCandidates) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("aicrm/dm01/external-identity-archive-only-root-candidates/v1"))
	for _, count := range []int64{value.ArchiveRows, value.DM01TerminalRows, value.OnlyArchive, value.UnionIDNotVerifiable, value.SourceShapeNotEligible, value.CustomerRootVerified, value.CustomerRootMissing} {
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], uint64(count))
		_, _ = hash.Write(encoded[:])
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}
