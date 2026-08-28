package v1domain

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1externalidentitygap"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	externalIdentityGapImportVersion = "v1-external-identity-gap-a1"
	externalIdentityGapTargetDomain  = "identity"
	externalIdentityGapTargetTable   = "identities"
)

// ExternalIdentityGapImportOptions contains only fixed run/key references.
// None of its source credentials is persisted by this importer.
type ExternalIdentityGapImportOptions struct {
	ArchiveRunID      string
	DM01RunID         int64
	SourceHMACKey     []byte
	DM01SourceHMACKey []byte
	TargetHMACKey     []byte
	KeyVersion        int16
}

// ExternalIdentityGapRow is an authenticated V1 archive-only identity. Its
// source-only payload remains in the encrypted archive and is never copied to
// a V2 identity row or receipt metadata.
type ExternalIdentityGapRow struct {
	ArchivedRow v1archive.ArchivedRow
	Fact        v1externalidentitygap.Fact
}

// ExternalIdentityGapSelection is a deterministic full-collection result.
// SummaryDigest contains no raw identifier and is suitable only for local
// reconciliation evidence.
type ExternalIdentityGapSelection struct {
	ArchiveRows      int
	DM01TerminalRows int
	OnlyArchive      []ExternalIdentityGapRow
	SummaryDigest    [sha256.Size]byte
}

type ExternalIdentityGapImportResult struct {
	Selected int
	Imported int
	Replayed int
}

// ExternalIdentityGapImportJournal is the narrow generic-receipt boundary for
// the one immutable identity source table.
type ExternalIdentityGapImportJournal interface {
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
	ValidateExternalIdentityGapScope(ExternalIdentityGapImportOptions) error
}

type externalIdentityGapJournal struct{ journal *Journal }

func NewExternalIdentityGapImportJournal(journal *Journal) (ExternalIdentityGapImportJournal, error) {
	if journal == nil || !validExternalIdentityGapJournalScope(journal, journal.scope.ArchiveRunID) {
		return nil, ErrInvalidScope
	}
	return externalIdentityGapJournal{journal: journal}, nil
}

func (journal externalIdentityGapJournal) LoadTerminal(ctx context.Context, source string) (TerminalReceipt, bool, error) {
	return journal.journal.LoadTerminal(ctx, source)
}

func (journal externalIdentityGapJournal) Record(ctx context.Context, receipt TerminalReceipt) error {
	return journal.journal.Record(ctx, receipt)
}

func (journal externalIdentityGapJournal) ValidateExternalIdentityGapScope(options ExternalIdentityGapImportOptions) error {
	return journal.journal.ValidateExternalIdentityGapScope(options)
}

// ValidateExternalIdentityGapScope pins the generic receipt journal to the
// formal non-actionable identity target.
func (journal *Journal) ValidateExternalIdentityGapScope(options ExternalIdentityGapImportOptions) error {
	if !validExternalIdentityGapOptions(options) || !validExternalIdentityGapJournalScope(journal, options.ArchiveRunID) {
		return ErrInvalidScope
	}
	return nil
}

func validExternalIdentityGapJournalScope(journal *Journal, archiveRunID string) bool {
	return journal != nil && journal.tx != nil && archiveRunID != "" && journal.scope.valid() &&
		journal.scope.ImportVersion == externalIdentityGapImportVersion && journal.scope.ArchiveRunID == archiveRunID &&
		journal.scope.AdapterID == v1archive.DefaultAdapterID && journal.scope.TableID == v1externalidentitygap.TableID &&
		journal.scope.TargetDomain == externalIdentityGapTargetDomain && journal.scope.TargetTable == externalIdentityGapTargetTable
}

type ExternalIdentityGapImporter struct {
	archive  ArchiveSource
	uow      UnitOfWork
	target   identityport.ArchiveIdentityTarget
	roots    contactport.DM01CustomerRootLocker
	receipts DM01ExternalIdentityReceiptSource
	journal  ExternalIdentityGapImportJournal
}

func NewExternalIdentityGapImporter(archive ArchiveSource, uow UnitOfWork, target identityport.ArchiveIdentityTarget, roots contactport.DM01CustomerRootLocker, receipts DM01ExternalIdentityReceiptSource, journal ExternalIdentityGapImportJournal) (*ExternalIdentityGapImporter, error) {
	if nilExternalIdentityGap(archive) || nilExternalIdentityGap(uow) || nilExternalIdentityGap(target) || nilExternalIdentityGap(roots) || nilExternalIdentityGap(receipts) || nilExternalIdentityGap(journal) {
		return nil, ErrInvalidScope
	}
	return &ExternalIdentityGapImporter{archive: archive, uow: uow, target: target, roots: roots, receipts: receipts, journal: journal}, nil
}

// SelectExternalIdentityGap authenticates the full V1 table and proves that
// DM01 has no rows absent from the archive before it returns archive-only rows.
func SelectExternalIdentityGap(ctx context.Context, archive ArchiveSource, receipts DM01ExternalIdentityReceiptSource, options ExternalIdentityGapImportOptions) (ExternalIdentityGapSelection, error) {
	if ctx == nil || nilExternalIdentityGap(archive) || nilExternalIdentityGap(receipts) || !validExternalIdentityGapOptions(options) {
		return ExternalIdentityGapSelection{}, ErrInvalidScope
	}

	type selected struct {
		row  v1archive.ArchivedRow
		key  [sha256.Size]byte
		seen bool
	}
	all := make([]selected, 0)
	byDM01Key := make(map[[sha256.Size]byte]int)
	var expectedOrdinal int64 = 1
	if err := archive.EachTableRow(ctx, options.ArchiveRunID, v1externalidentitygap.TableID, func(row v1archive.ArchivedRow) error {
		if row.SourceOrdinal != expectedOrdinal {
			return ErrConflict
		}
		expectedOrdinal++
		id, stage := verifyExternalIdentityArchiveRow(row, options.SourceHMACKey)
		if stage != "" {
			return ErrConflict
		}
		dm01Key, err := contactmigration.SourceKeyHMAC(options.DM01SourceHMACKey, "wecom_external_contact_identity_map", strconv.FormatInt(id, 10))
		if err != nil || len(dm01Key) != sha256.Size {
			return ErrConflict
		}
		var key [sha256.Size]byte
		copy(key[:], dm01Key)
		if _, duplicate := byDM01Key[key]; duplicate {
			return ErrConflict
		}
		byDM01Key[key] = len(all)
		all = append(all, selected{row: row, key: key})
		return nil
	}); err != nil {
		return ExternalIdentityGapSelection{}, err
	}
	if len(all) == 0 {
		return ExternalIdentityGapSelection{}, ErrConflict
	}

	var expectedReceiptOrdinal int64 = 1
	seenReceipts := make(map[[sha256.Size]byte]struct{}, len(all))
	if err := receipts.EachDM01ExternalIdentityReceipt(ctx, options.DM01RunID, func(receipt DM01ExternalIdentityReceipt) error {
		if receipt.SourceOrdinal != expectedReceiptOrdinal || receipt.SourceKeyHMAC == ([sha256.Size]byte{}) || (receipt.Disposition != "imported" && receipt.Disposition != "quarantined") {
			return ErrConflict
		}
		expectedReceiptOrdinal++
		if _, duplicate := seenReceipts[receipt.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seenReceipts[receipt.SourceKeyHMAC] = struct{}{}
		index, found := byDM01Key[receipt.SourceKeyHMAC]
		if !found {
			return ErrConflict // OnlyDM01 is not permitted.
		}
		all[index].seen = true
		return nil
	}); err != nil {
		return ExternalIdentityGapSelection{}, err
	}

	selection := ExternalIdentityGapSelection{ArchiveRows: len(all), DM01TerminalRows: len(seenReceipts), OnlyArchive: make([]ExternalIdentityGapRow, 0, len(all)-len(seenReceipts))}
	keys := make([][sha256.Size]byte, 0, len(all)-len(seenReceipts))
	for _, value := range all {
		if value.seen {
			continue
		}
		fact, err := v1externalidentitygap.Adapt(value.row, options.SourceHMACKey)
		if err != nil {
			return ExternalIdentityGapSelection{}, ErrConflict
		}
		selection.OnlyArchive = append(selection.OnlyArchive, ExternalIdentityGapRow{ArchivedRow: value.row, Fact: fact})
		keys = append(keys, value.key)
	}
	selection.SummaryDigest = externalIdentityGapSelectionDigest(selection.ArchiveRows, selection.DM01TerminalRows, keys)
	return selection, nil
}

func (importer *ExternalIdentityGapImporter) Import(ctx context.Context, options ExternalIdentityGapImportOptions) (ExternalIdentityGapImportResult, error) {
	if importer == nil || ctx == nil || nilExternalIdentityGap(importer.archive) || nilExternalIdentityGap(importer.uow) || nilExternalIdentityGap(importer.target) || nilExternalIdentityGap(importer.roots) || nilExternalIdentityGap(importer.receipts) || nilExternalIdentityGap(importer.journal) || importer.journal.ValidateExternalIdentityGapScope(options) != nil {
		return ExternalIdentityGapImportResult{}, ErrInvalidScope
	}
	selection, err := SelectExternalIdentityGap(ctx, importer.archive, importer.receipts, options)
	if err != nil {
		return ExternalIdentityGapImportResult{}, err
	}
	return importer.importSelection(ctx, selection, options)
}

// Verify re-runs selection and validates every selected receipt against its
// owner target without creating identities or changing the DM01 ledger.
func (importer *ExternalIdentityGapImporter) Verify(ctx context.Context, options ExternalIdentityGapImportOptions) error {
	if importer == nil || ctx == nil || nilExternalIdentityGap(importer.archive) || nilExternalIdentityGap(importer.uow) || nilExternalIdentityGap(importer.target) || nilExternalIdentityGap(importer.roots) || nilExternalIdentityGap(importer.receipts) || nilExternalIdentityGap(importer.journal) || importer.journal.ValidateExternalIdentityGapScope(options) != nil {
		return ErrInvalidScope
	}
	selection, err := SelectExternalIdentityGap(ctx, importer.archive, importer.receipts, options)
	if err != nil {
		return err
	}
	for _, value := range selection.OnlyArchive {
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			customerID, err := importer.lockCustomerRoot(tx, value.Fact, options)
			if err != nil {
				return err
			}
			return verifyExternalIdentityGapTerminal(tx, importer.target, importer.journal, value, customerID, options)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (importer *ExternalIdentityGapImporter) importSelection(ctx context.Context, selection ExternalIdentityGapSelection, options ExternalIdentityGapImportOptions) (ExternalIdentityGapImportResult, error) {
	if !validExternalIdentityGapOptions(options) || selection.ArchiveRows < 1 || selection.DM01TerminalRows+len(selection.OnlyArchive) != selection.ArchiveRows {
		return ExternalIdentityGapImportResult{}, ErrInvalidScope
	}
	result := ExternalIdentityGapImportResult{Selected: len(selection.OnlyArchive)}
	for _, value := range selection.OnlyArchive {
		imported, replayed := false, false
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			imported, replayed = false, false
			customerID, err := importer.lockCustomerRoot(tx, value.Fact, options)
			if err != nil {
				return err
			}
			terminal, found, err := importer.journal.LoadTerminal(tx, SourceIdentifier(value.ArchivedRow.SourceKeyHMAC))
			if err != nil {
				return err
			}
			if found {
				if err := verifyExternalIdentityGapTerminalValue(tx, importer.target, terminal, value, customerID, options); err != nil {
					return err
				}
				replayed = true
				return nil
			}
			actual, err := importer.target.ImportArchiveWeComIdentity(tx, identityport.ArchiveIdentityInput{CustomerID: customerID, Scope: value.Fact.Scope, ExternalUserID: value.Fact.ExternalUserID, SourceKeyHMAC: value.Fact.SourceKeyHMAC, HMACKeyVersion: options.KeyVersion})
			if err != nil {
				return err
			}
			if !externalIdentityGapFactMatches(actual, value.Fact, customerID, options.KeyVersion) {
				return ErrConflict
			}
			digest, err := externalIdentityGapTargetDigest(options.TargetHMACKey, actual)
			if err != nil {
				return err
			}
			metadata, err := externalIdentityGapMetadata(value.Fact, customerID, options)
			if err != nil {
				return err
			}
			receipt := TerminalReceipt{SourceKeyDigest: value.Fact.SourceKeyHMAC, PayloadDigest: value.Fact.SourcePayloadHMAC, Disposition: "import", TargetID: strconv.FormatInt(actual.ID, 10), TargetDigest: digest, Metadata: metadata}
			if err := importer.journal.Record(tx, receipt); err != nil {
				return err
			}
			imported = true
			return nil
		}); err != nil {
			return ExternalIdentityGapImportResult{}, err
		}
		if imported {
			result.Imported++
		}
		if replayed {
			result.Replayed++
		}
	}
	if result.Imported+result.Replayed != result.Selected {
		return ExternalIdentityGapImportResult{}, ErrConflict
	}
	return result, nil
}

func (importer *ExternalIdentityGapImporter) lockCustomerRoot(ctx context.Context, fact v1externalidentitygap.Fact, options ExternalIdentityGapImportOptions) (*contactport.CustomerID, error) {
	if fact.RootRoute == v1externalidentitygap.RootRouteUnbound && fact.UnionID == nil {
		return nil, nil
	}
	if fact.RootRoute != v1externalidentitygap.RootRouteRequiresVerifiedRoot || fact.UnionID == nil {
		return nil, ErrConflict
	}
	key, err := contactmigration.SourceKeyHMAC(options.DM01SourceHMACKey, dm01CustomerIdentitySourceTable, *fact.UnionID)
	if err != nil || len(key) != sha256.Size {
		return nil, ErrConflict
	}
	var digest [sha256.Size]byte
	copy(digest[:], key)
	customerID, found, err := importer.roots.LockVerifiedDM01CustomerRoot(ctx, options.DM01RunID, digest)
	if err != nil {
		return nil, err
	}
	if !found || customerID < 1 {
		return nil, ErrConflict
	}
	return &customerID, nil
}

func verifyExternalIdentityGapTerminal(ctx context.Context, target identityport.ArchiveIdentityTarget, journal ExternalIdentityGapImportJournal, value ExternalIdentityGapRow, customerID *contactport.CustomerID, options ExternalIdentityGapImportOptions) error {
	terminal, found, err := journal.LoadTerminal(ctx, SourceIdentifier(value.ArchivedRow.SourceKeyHMAC))
	if err != nil || !found {
		return ErrConflict
	}
	return verifyExternalIdentityGapTerminalValue(ctx, target, terminal, value, customerID, options)
}

func verifyExternalIdentityGapTerminalValue(ctx context.Context, target identityport.ArchiveIdentityTarget, terminal TerminalReceipt, value ExternalIdentityGapRow, customerID *contactport.CustomerID, options ExternalIdentityGapImportOptions) error {
	metadata, err := externalIdentityGapMetadata(value.Fact, customerID, options)
	if err != nil {
		return err
	}
	if terminal.SourceKeyDigest != value.Fact.SourceKeyHMAC || terminal.PayloadDigest != value.Fact.SourcePayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetID == "" || terminal.TargetDigest == ([sha256.Size]byte{}) || !equalExternalIdentityGapMetadata(terminal.Metadata, metadata) {
		return ErrConflict
	}
	id, err := positiveID(terminal.TargetID)
	if err != nil {
		return err
	}
	actual, err := target.ReadArchiveWeComIdentity(ctx, id)
	if err != nil {
		return err
	}
	if !externalIdentityGapFactMatches(actual, value.Fact, customerID, options.KeyVersion) {
		return ErrConflict
	}
	digest, err := externalIdentityGapTargetDigest(options.TargetHMACKey, actual)
	if err != nil || digest != terminal.TargetDigest {
		return ErrConflict
	}
	return nil
}

func externalIdentityGapSelectionDigest(archiveRows, dm01Rows int, keys [][sha256.Size]byte) [sha256.Size]byte {
	sort.Slice(keys, func(left, right int) bool { return bytes.Compare(keys[left][:], keys[right][:]) < 0 })
	hash := sha256.New()
	_, _ = hash.Write([]byte("aicrm/v1-external-identity-gap-selection/v1"))
	for _, count := range []int{archiveRows, dm01Rows, len(keys)} {
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], uint64(count))
		_, _ = hash.Write(encoded[:])
	}
	for _, key := range keys {
		_, _ = hash.Write(key[:])
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}

func externalIdentityGapMetadata(fact v1externalidentitygap.Fact, customerID *contactport.CustomerID, options ExternalIdentityGapImportOptions) (map[string]any, error) {
	customer, route, rootDigest := "", "", ""
	switch fact.RootRoute {
	case v1externalidentitygap.RootRouteUnbound:
		if fact.UnionID != nil || customerID != nil {
			return nil, ErrConflict
		}
		route = "unbound"
	case v1externalidentitygap.RootRouteRequiresVerifiedRoot:
		if fact.UnionID == nil || customerID == nil || *customerID < 1 {
			return nil, ErrConflict
		}
		root, err := contactmigration.SourceKeyHMAC(options.DM01SourceHMACKey, dm01CustomerIdentitySourceTable, *fact.UnionID)
		if err != nil || len(root) != sha256.Size {
			return nil, ErrConflict
		}
		customer = strconv.FormatInt(int64(*customerID), 10)
		route = "verified_root"
		rootDigest = hex.EncodeToString(root)
	default:
		return nil, ErrConflict
	}
	return map[string]any{"customer_id": customer, "hmac_key_version": strconv.FormatInt(int64(options.KeyVersion), 10), "root_route": route, "root_source_hmac": rootDigest, "source_field_hmac": hex.EncodeToString(fact.SourceFieldHMAC[:])}, nil
}

func equalExternalIdentityGapMetadata(actual, expected map[string]any) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key, want := range expected {
		found, ok := actual[key]
		if !ok || found != want {
			return false
		}
	}
	return true
}

func externalIdentityGapFactMatches(actual identityport.ArchiveIdentityFact, fact v1externalidentitygap.Fact, customerID *contactport.CustomerID, keyVersion int16) bool {
	if actual.ID < 1 || actual.Scope != fact.Scope || actual.ExternalUserID != fact.ExternalUserID || actual.HMACKeyVersion != keyVersion || actual.Assurance != "declared" || actual.Source != "v1.archive_identity_gap" || actual.NormalizerVersion != identityapp.NormalizerVersion || actual.CreatedAt.IsZero() || !bytes.Equal(actual.ReviewFingerprint[:], fact.SourceKeyHMAC[:16]) {
		return false
	}
	if customerID == nil {
		return actual.CustomerID == nil && actual.BoundAt == nil
	}
	return actual.CustomerID != nil && *actual.CustomerID == *customerID && actual.BoundAt != nil && !actual.BoundAt.IsZero()
}

func externalIdentityGapTargetDigest(key []byte, value identityport.ArchiveIdentityFact) ([sha256.Size]byte, error) {
	if len(key) < sha256.Size || value.ID < 1 || value.CreatedAt.IsZero() {
		return [sha256.Size]byte{}, ErrInvalidScope
	}
	createdAt := value.CreatedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
	var customerID *int64
	if value.CustomerID != nil {
		id := int64(*value.CustomerID)
		customerID = &id
	}
	var boundAt *string
	if value.BoundAt != nil {
		stamp := value.BoundAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
		boundAt = &stamp
	}
	payload, err := json.Marshal(struct {
		ID                int64   `json:"id"`
		CustomerID        *int64  `json:"customer_id"`
		Scope             string  `json:"scope"`
		ExternalUserID    string  `json:"external_userid"`
		HMACKeyVersion    int16   `json:"hmac_key_version"`
		Assurance         string  `json:"assurance"`
		Source            string  `json:"source"`
		NormalizerVersion int16   `json:"normalizer_version"`
		ReviewFingerprint string  `json:"review_fingerprint"`
		CreatedAt         string  `json:"created_at"`
		BoundAt           *string `json:"bound_at"`
	}{value.ID, customerID, value.Scope, value.ExternalUserID, value.HMACKeyVersion, value.Assurance, value.Source, value.NormalizerVersion, hex.EncodeToString(value.ReviewFingerprint[:]), createdAt, boundAt})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("aicrm/v1-external-identity-gap-target/v1"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	var digest [sha256.Size]byte
	copy(digest[:], mac.Sum(nil))
	return digest, nil
}

func validExternalIdentityGapOptions(options ExternalIdentityGapImportOptions) bool {
	return options.ArchiveRunID != "" && options.DM01RunID > 0 && len(options.SourceHMACKey) >= sha256.Size && len(options.DM01SourceHMACKey) >= sha256.Size && len(options.TargetHMACKey) >= sha256.Size && options.KeyVersion > 0
}

func nilExternalIdentityGap(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Func) && reflected.IsNil()
}
