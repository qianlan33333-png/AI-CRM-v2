package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const dm01ExternalIdentityArchiveTableID = "public/wecom_external_contact_identity_map"

var errInvalidDM01ExternalIdentityArchiveDiff = errors.New("invalid DM01 external identity archive difference input")

// DM01ExternalIdentityArchiveDiffError exposes only a fixed validation stage.
// It deliberately never includes an archive ordinal, source key, or identity.
type DM01ExternalIdentityArchiveDiffError struct{ Stage string }

func (err *DM01ExternalIdentityArchiveDiffError) Error() string {
	return "DM01 external identity archive difference failed: " + err.Stage
}

func (*DM01ExternalIdentityArchiveDiffError) Unwrap() error {
	return errInvalidDM01ExternalIdentityArchiveDiff
}

func dm01ExternalIdentityArchiveDiffFailure(stage string) error {
	return &DM01ExternalIdentityArchiveDiffError{Stage: stage}
}

// DM01ExternalIdentityReceipt is the minimum sealed DM01 receipt evidence
// needed to compare membership. It intentionally carries no source payload or
// identity value.
type DM01ExternalIdentityReceipt struct {
	SourceOrdinal int64
	SourceKeyHMAC [sha256.Size]byte
	Disposition   string
}

// DM01ExternalIdentityReceiptSource is implemented by the caller-owned,
// read-only DM01 repository adapter.
type DM01ExternalIdentityReceiptSource interface {
	EachDM01ExternalIdentityReceipt(context.Context, int64, func(DM01ExternalIdentityReceipt) error) error
}

// DM01ExternalIdentityArchiveDiff contains only aggregate membership evidence.
// SummaryDigest is a SHA-256 digest of the sorted DM01-key sets by membership
// class; it never exposes an individual source key or source identity.
type DM01ExternalIdentityArchiveDiff struct {
	ArchiveRows      int64
	DM01TerminalRows int64
	Intersection     int64
	OnlyArchive      int64
	OnlyDM01         int64
	SummaryDigest    [sha256.Size]byte
}

// DiffDM01ExternalIdentityArchive compares the sealed full-archive table with
// one DM01 run. It is read-only: callers provide both iterators and this
// function never persists receipts or target mappings.
func DiffDM01ExternalIdentityArchive(
	ctx context.Context,
	archive ArchiveSource,
	receipts DM01ExternalIdentityReceiptSource,
	archiveRunID string,
	dm01RunID int64,
	archiveSourceHMACKey []byte,
	dm01SourceHMACKey []byte,
) (DM01ExternalIdentityArchiveDiff, error) {
	if archive == nil || receipts == nil || archiveRunID == "" || dm01RunID < 1 || len(archiveSourceHMACKey) < sha256.Size || len(dm01SourceHMACKey) < sha256.Size {
		return DM01ExternalIdentityArchiveDiff{}, dm01ExternalIdentityArchiveDiffFailure("scope")
	}

	archiveKeys := make(map[[sha256.Size]byte]struct{})
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
		key, err := contactmigration.SourceKeyHMAC(dm01SourceHMACKey, "wecom_external_contact_identity_map", strconv.FormatInt(id, 10))
		if err != nil || len(key) != sha256.Size {
			return dm01ExternalIdentityArchiveDiffFailure("dm01_source_hmac")
		}
		var digest [sha256.Size]byte
		copy(digest[:], key)
		if _, exists := archiveKeys[digest]; exists {
			return dm01ExternalIdentityArchiveDiffFailure("archive_duplicate")
		}
		archiveKeys[digest] = struct{}{}
		return nil
	}); err != nil {
		var failure *DM01ExternalIdentityArchiveDiffError
		if errors.As(err, &failure) {
			return DM01ExternalIdentityArchiveDiff{}, failure
		}
		return DM01ExternalIdentityArchiveDiff{}, dm01ExternalIdentityArchiveDiffFailure("archive_stream")
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
			return DM01ExternalIdentityArchiveDiff{}, failure
		}
		return DM01ExternalIdentityArchiveDiff{}, dm01ExternalIdentityArchiveDiffFailure("receipt_stream")
	}

	intersection := make([][sha256.Size]byte, 0)
	onlyArchive := make([][sha256.Size]byte, 0)
	for key := range archiveKeys {
		if _, exists := dm01Keys[key]; exists {
			intersection = append(intersection, key)
		} else {
			onlyArchive = append(onlyArchive, key)
		}
	}
	onlyDM01 := make([][sha256.Size]byte, 0)
	for key := range dm01Keys {
		if _, exists := archiveKeys[key]; !exists {
			onlyDM01 = append(onlyDM01, key)
		}
	}

	return DM01ExternalIdentityArchiveDiff{
		ArchiveRows:      int64(len(archiveKeys)),
		DM01TerminalRows: int64(len(dm01Keys)),
		Intersection:     int64(len(intersection)),
		OnlyArchive:      int64(len(onlyArchive)),
		OnlyDM01:         int64(len(onlyDM01)),
		SummaryDigest:    dm01ExternalIdentityDiffDigest(intersection, onlyArchive, onlyDM01),
	}, nil
}

func verifyExternalIdentityArchiveRow(row v1archive.ArchivedRow, sourceHMACKey []byte) (int64, string) {
	if v1archive.IsRedacted(row, "id") || !json.Valid(row.Payload) {
		return 0, "archive_id"
	}
	canonical, paths, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) {
		return 0, "archive_payload_hmac"
	}
	payloadHMAC, err := v1archive.PayloadHMAC(sourceHMACKey, "wecom_external_contact_identity_map", canonical)
	if err != nil || payloadHMAC != row.PayloadHMAC {
		return 0, "archive_payload_hmac"
	}
	fieldHMAC, err := v1archive.FieldHMAC(sourceHMACKey, "wecom_external_contact_identity_map", paths)
	if err != nil || fieldHMAC != row.FieldHMAC {
		return 0, "archive_field_hmac"
	}

	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	decoder.UseNumber()
	var payload map[string]any
	if err = decoder.Decode(&payload); err != nil {
		return 0, "archive_payload_shape"
	}
	if err = decoder.Decode(&struct{}{}); err == nil {
		return 0, "archive_payload_shape"
	}
	idNumber, ok := payload["id"].(json.Number)
	if !ok {
		return 0, "archive_payload_shape"
	}
	id, err := strconv.ParseInt(string(idNumber), 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != string(idNumber) {
		return 0, "archive_id"
	}
	keyJSON, err := json.Marshal([]int64{id})
	if err != nil {
		return 0, "archive_id"
	}
	sourceKey, err := v1archive.SourceKeyHMAC(sourceHMACKey, "wecom_external_contact_identity_map", keyJSON)
	if err != nil || sourceKey != row.SourceKeyHMAC {
		return 0, "archive_source_hmac"
	}
	return id, ""
}

func dm01ExternalIdentityDiffDigest(intersection, onlyArchive, onlyDM01 [][sha256.Size]byte) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("aicrm/dm01/external-identity-archive-diff/v1"))
	writeDiffDigestClass(hash, 1, intersection)
	writeDiffDigestClass(hash, 2, onlyArchive)
	writeDiffDigestClass(hash, 3, onlyDM01)
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}

func writeDiffDigestClass(hash interface{ Write([]byte) (int, error) }, class byte, values [][sha256.Size]byte) {
	sort.Slice(values, func(left, right int) bool { return bytes.Compare(values[left][:], values[right][:]) < 0 })
	_, _ = hash.Write([]byte{class})
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(values)))
	_, _ = hash.Write(count[:])
	for _, value := range values {
		_, _ = hash.Write(value[:])
	}
}

func zeroDigest(value [sha256.Size]byte) bool {
	return value == [sha256.Size]byte{}
}
