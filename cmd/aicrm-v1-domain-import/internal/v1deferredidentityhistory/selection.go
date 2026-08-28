package v1deferredidentityhistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	DM01PeopleSourceTable            = "people"
	DM01IdentityConflictsSourceTable = "crm_user_identity_conflicts"
	DM01IdentityMapSourceTable       = "wecom_external_contact_identity_map"

	TargetSchemaDeferredReason = "target_schema_deferred"
	MissingCustomerRootReason  = "missing_customer_root"

	expectedPeopleRows              = 1385
	expectedIdentityConflictRows    = 5
	expectedMissingCustomerRootRows = 2
)

var ErrInvalidDeferredIdentitySelection = errors.New("invalid deferred identity evidence selection")

// SelectionError exposes only a fixed validation stage. It never includes an
// archive payload, source identifier, HMAC value, or quarantine metadata.
type SelectionError struct{ Stage string }

func (err *SelectionError) Error() string {
	return "deferred identity evidence selection failed: " + err.Stage
}

func (*SelectionError) Unwrap() error { return ErrInvalidDeferredIdentitySelection }

func selectionFailure(stage string) error { return &SelectionError{Stage: stage} }

// ArchiveReader is the immutable V2 archive boundary. It has no target writer
// or source-database dependency.
type ArchiveReader interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

// DM01EvidenceReader is the minimum caller-owned, read-only projection of the
// sealed DM01 run. The selector intentionally cannot query a database itself.
type DM01EvidenceReader interface {
	ReadDM01Run(context.Context, int64) (DM01Run, error)
	ReadDM01Checkpoint(context.Context, int64, string) (DM01Checkpoint, error)
	EachDM01TerminalReceipt(context.Context, int64, string, func(DM01TerminalReceipt) error) error
	EachDM01Quarantine(context.Context, int64, string, func(DM01Quarantine) error) error
}

type DM01Run struct {
	ID             int64
	Mode           string
	State          string
	HMACKeyVersion int16
}

// DM01Checkpoint mirrors only immutable checkpoint evidence used to prove a
// complete non-empty table scan.
type DM01Checkpoint struct {
	SourceTable        string
	FinalSourceKeyHMAC [sha256.Size]byte
	PayloadHMAC        [sha256.Size]byte
	FieldDigest        [sha256.Size]byte
	Watermark          time.Time
	UpperSourceKeyHMAC [sha256.Size]byte
	UpperBoundEmpty    bool
}

type DM01TerminalReceipt struct {
	SourceTable   string
	SourceOrdinal int64
	SourceKeyHMAC [sha256.Size]byte
	PayloadHMAC   [sha256.Size]byte
	FieldDigest   [sha256.Size]byte
	Disposition   string
}

type DM01Quarantine struct {
	SourceTable   string
	SourceKeyHMAC [sha256.Size]byte
	PayloadHMAC   [sha256.Size]byte
	FieldDigest   [sha256.Size]byte
	ReasonCode    string
}

type DeferredIdentitySelectionOptions struct {
	ArchiveRunID       string
	DM01RunID          int64
	ArchiveHMACKey     []byte
	DM01HMACKey        []byte
	DM01HMACKeyVersion int16
}

// DM01SelectionLineage proves the separate DM01 source-key domain used for
// this historical-evidence selection. It is not an identity or Customer link.
type DM01SelectionLineage struct {
	SourceKeyHMAC [sha256.Size]byte `json:"-"`
	PayloadHMAC   [sha256.Size]byte `json:"-"`
	FieldDigest   [sha256.Size]byte `json:"-"`
	KeyVersion    int16             `json:"-"`
	ReasonCode    string            `json:"-"`
}

type SelectedPerson struct {
	ArchivedRow v1archive.ArchivedRow `json:"-"`
	Fact        PersonFact            `json:"-"`
	Lineage     DM01SelectionLineage  `json:"-"`
}

type SelectedIdentityConflict struct {
	ArchivedRow v1archive.ArchivedRow `json:"-"`
	Fact        ConflictFact          `json:"-"`
	Lineage     DM01SelectionLineage  `json:"-"`
}

type SelectedMissingRootIdentity struct {
	ArchivedRow v1archive.ArchivedRow   `json:"-"`
	Fact        MissingRootIdentityFact `json:"-"`
	Lineage     DM01SelectionLineage    `json:"-"`
}

// DeferredIdentitySelection contains only the fixed deferred evidence scope.
// Map counters make the excluded 129 archive-only rows and non-target DM01
// outcomes observable without exposing identity material.
type DeferredIdentitySelection struct {
	People                  []SelectedPerson
	IdentityConflicts       []SelectedIdentityConflict
	MissingCustomerRootMaps []SelectedMissingRootIdentity

	MapImportedRows             int
	MapArchiveOnlyRows          int
	MapNonTargetQuarantinedRows int
}

func (selection DeferredIdentitySelection) Count() int {
	return len(selection.People) + len(selection.IdentityConflicts) + len(selection.MissingCustomerRootMaps)
}

type dm01TableEvidence struct {
	receipts     map[[sha256.Size]byte]DM01TerminalReceipt
	quarantines  map[[sha256.Size]byte]DM01Quarantine
	last         DM01TerminalReceipt
	receiptCount int
}

// SelectDeferredIdentityEvidence authenticates all three source tables, then
// selects exactly the immutable DM01 quarantine scope. It creates no target
// state and never infers a Customer or identity relation.
func SelectDeferredIdentityEvidence(ctx context.Context, archive ArchiveReader, dm01 DM01EvidenceReader, options DeferredIdentitySelectionOptions) (DeferredIdentitySelection, error) {
	if ctx == nil || nilDeferredIdentity(archive) || nilDeferredIdentity(dm01) || !validDeferredIdentityOptions(options) {
		return DeferredIdentitySelection{}, selectionFailure("scope")
	}
	run, err := dm01.ReadDM01Run(ctx, options.DM01RunID)
	if err != nil {
		return DeferredIdentitySelection{}, selectionFailure("run_read")
	}
	if run.ID != options.DM01RunID || run.Mode != "full" || run.State != "imported" || run.HMACKeyVersion != options.DM01HMACKeyVersion {
		return DeferredIdentitySelection{}, selectionFailure("run")
	}

	peopleEvidence, err := readDM01TableEvidence(ctx, dm01, options.DM01RunID, DM01PeopleSourceTable)
	if err != nil {
		return DeferredIdentitySelection{}, err
	}
	conflictEvidence, err := readDM01TableEvidence(ctx, dm01, options.DM01RunID, DM01IdentityConflictsSourceTable)
	if err != nil {
		return DeferredIdentitySelection{}, err
	}
	mapEvidence, err := readDM01TableEvidence(ctx, dm01, options.DM01RunID, DM01IdentityMapSourceTable)
	if err != nil {
		return DeferredIdentitySelection{}, err
	}

	selection := DeferredIdentitySelection{
		People:                  make([]SelectedPerson, 0, expectedPeopleRows),
		IdentityConflicts:       make([]SelectedIdentityConflict, 0, expectedIdentityConflictRows),
		MissingCustomerRootMaps: make([]SelectedMissingRootIdentity, 0, expectedMissingCustomerRootRows),
	}
	peopleOrdinal := int64(1)
	if err := archive.EachTableRow(ctx, options.ArchiveRunID, PeopleTableID, func(row v1archive.ArchivedRow) error {
		if row.SourceOrdinal != peopleOrdinal {
			return selectionFailure("people_ordinal")
		}
		peopleOrdinal++
		fact, err := AdaptPerson(row, options.ArchiveHMACKey)
		if err != nil {
			return selectionFailure("people_archive")
		}
		lineage, err := matchDeferredQuarantine(peopleEvidence, DM01PeopleSourceTable, TargetSchemaDeferredReason, fact.SourceID, options.DM01HMACKey, options.DM01HMACKeyVersion)
		if err != nil {
			return err
		}
		selection.People = append(selection.People, SelectedPerson{ArchivedRow: row, Fact: fact, Lineage: lineage})
		return nil
	}); err != nil {
		return DeferredIdentitySelection{}, normalizeSelectionError(err, "people_archive_stream")
	}
	if len(selection.People) != expectedPeopleRows || !allDM01QuarantinesConsumed(peopleEvidence) || peopleEvidence.receiptCount != expectedPeopleRows {
		return DeferredIdentitySelection{}, selectionFailure("people_coverage")
	}

	conflictOrdinal := int64(1)
	if err := archive.EachTableRow(ctx, options.ArchiveRunID, IdentityConflictsTableID, func(row v1archive.ArchivedRow) error {
		if row.SourceOrdinal != conflictOrdinal {
			return selectionFailure("conflicts_ordinal")
		}
		conflictOrdinal++
		fact, err := AdaptConflict(row, options.ArchiveHMACKey)
		if err != nil {
			return selectionFailure("conflicts_archive")
		}
		lineage, err := matchDeferredQuarantine(conflictEvidence, DM01IdentityConflictsSourceTable, TargetSchemaDeferredReason, fact.SourceID, options.DM01HMACKey, options.DM01HMACKeyVersion)
		if err != nil {
			return err
		}
		selection.IdentityConflicts = append(selection.IdentityConflicts, SelectedIdentityConflict{ArchivedRow: row, Fact: fact, Lineage: lineage})
		return nil
	}); err != nil {
		return DeferredIdentitySelection{}, normalizeSelectionError(err, "conflicts_archive_stream")
	}
	if len(selection.IdentityConflicts) != expectedIdentityConflictRows || !allDM01QuarantinesConsumed(conflictEvidence) || conflictEvidence.receiptCount != expectedIdentityConflictRows {
		return DeferredIdentitySelection{}, selectionFailure("conflicts_coverage")
	}

	seenMapReceipts := make(map[[sha256.Size]byte]bool, mapEvidence.receiptCount)
	seenMapQuarantines := make(map[[sha256.Size]byte]bool, len(mapEvidence.quarantines))
	mapRows := 0
	if err := archive.EachTableRow(ctx, options.ArchiveRunID, ExternalContactIdentityMapID, func(row v1archive.ArchivedRow) error {
		if row.SourceOrdinal != int64(mapRows+1) {
			return selectionFailure("map_ordinal")
		}
		mapRows++
		fact, err := AdaptMissingRootIdentity(row, options.ArchiveHMACKey)
		if err != nil {
			return selectionFailure("map_archive")
		}
		key, err := dm01SourceKey(options.DM01HMACKey, DM01IdentityMapSourceTable, fact.SourceID)
		if err != nil {
			return err
		}
		receipt, found := mapEvidence.receipts[key]
		if !found {
			selection.MapArchiveOnlyRows++ // handled exclusively by immutable 129 scope
			return nil
		}
		if seenMapReceipts[key] {
			return selectionFailure("map_receipt")
		}
		seenMapReceipts[key] = true
		switch receipt.Disposition {
		case "imported":
			if _, quarantined := mapEvidence.quarantines[key]; quarantined {
				return selectionFailure("map_imported_quarantine")
			}
			selection.MapImportedRows++
			return nil
		case "quarantined":
			quarantine, found := mapEvidence.quarantines[key]
			if !found || !sameDM01Quarantine(receipt, quarantine) || seenMapQuarantines[key] {
				return selectionFailure("map_quarantine")
			}
			seenMapQuarantines[key] = true
			if quarantine.ReasonCode != MissingCustomerRootReason {
				selection.MapNonTargetQuarantinedRows++
				return nil
			}
			selection.MissingCustomerRootMaps = append(selection.MissingCustomerRootMaps, SelectedMissingRootIdentity{
				ArchivedRow: row,
				Fact:        fact,
				Lineage:     selectionLineage(receipt, options.DM01HMACKeyVersion, quarantine.ReasonCode),
			})
			return nil
		default:
			return selectionFailure("map_disposition")
		}
	}); err != nil {
		return DeferredIdentitySelection{}, normalizeSelectionError(err, "map_archive_stream")
	}
	if mapRows == 0 || len(seenMapReceipts) != mapEvidence.receiptCount || len(seenMapQuarantines) != len(mapEvidence.quarantines) || len(selection.MissingCustomerRootMaps) != expectedMissingCustomerRootRows {
		return DeferredIdentitySelection{}, selectionFailure("map_coverage")
	}
	return selection, nil
}

func readDM01TableEvidence(ctx context.Context, dm01 DM01EvidenceReader, runID int64, sourceTable string) (dm01TableEvidence, error) {
	checkpoint, err := dm01.ReadDM01Checkpoint(ctx, runID, sourceTable)
	if err != nil {
		return dm01TableEvidence{}, selectionFailure("checkpoint_read")
	}
	if checkpoint.SourceTable != sourceTable || checkpoint.UpperBoundEmpty || checkpoint.Watermark.IsZero() || zeroDigest(checkpoint.FinalSourceKeyHMAC) || zeroDigest(checkpoint.PayloadHMAC) || zeroDigest(checkpoint.FieldDigest) || zeroDigest(checkpoint.UpperSourceKeyHMAC) {
		return dm01TableEvidence{}, selectionFailure("checkpoint")
	}
	evidence := dm01TableEvidence{receipts: make(map[[sha256.Size]byte]DM01TerminalReceipt), quarantines: make(map[[sha256.Size]byte]DM01Quarantine)}
	nextOrdinal := int64(1)
	if err := dm01.EachDM01TerminalReceipt(ctx, runID, sourceTable, func(receipt DM01TerminalReceipt) error {
		if receipt.SourceTable != sourceTable || receipt.SourceOrdinal != nextOrdinal || zeroDigest(receipt.SourceKeyHMAC) || zeroDigest(receipt.PayloadHMAC) || zeroDigest(receipt.FieldDigest) || (receipt.Disposition != "imported" && receipt.Disposition != "quarantined") {
			return selectionFailure("receipt")
		}
		nextOrdinal++
		if _, duplicate := evidence.receipts[receipt.SourceKeyHMAC]; duplicate {
			return selectionFailure("receipt_duplicate")
		}
		evidence.receipts[receipt.SourceKeyHMAC] = receipt
		evidence.last = receipt
		evidence.receiptCount++
		return nil
	}); err != nil {
		return dm01TableEvidence{}, normalizeSelectionError(err, "receipt_stream")
	}
	// DM01 imported identities replace receipt.FieldDigest with the V2 target
	// digest, while checkpoints keep the source-scan field HMAC. Such rows are
	// outside this deferred scope; quarantines retain the source field HMAC.
	importedIdentityTerminal := sourceTable == DM01IdentityMapSourceTable && evidence.last.Disposition == "imported"
	if evidence.receiptCount == 0 || checkpoint.FinalSourceKeyHMAC != evidence.last.SourceKeyHMAC || checkpoint.PayloadHMAC != evidence.last.PayloadHMAC || (!importedIdentityTerminal && checkpoint.FieldDigest != evidence.last.FieldDigest) {
		return dm01TableEvidence{}, selectionFailure("checkpoint_terminal")
	}
	if err := dm01.EachDM01Quarantine(ctx, runID, sourceTable, func(quarantine DM01Quarantine) error {
		if quarantine.SourceTable != sourceTable || zeroDigest(quarantine.SourceKeyHMAC) || zeroDigest(quarantine.PayloadHMAC) || zeroDigest(quarantine.FieldDigest) || quarantine.ReasonCode == "" || strings.TrimSpace(quarantine.ReasonCode) != quarantine.ReasonCode {
			return selectionFailure("quarantine")
		}
		if _, duplicate := evidence.quarantines[quarantine.SourceKeyHMAC]; duplicate {
			return selectionFailure("quarantine_duplicate")
		}
		receipt, found := evidence.receipts[quarantine.SourceKeyHMAC]
		if !found || receipt.Disposition != "quarantined" || !sameDM01Quarantine(receipt, quarantine) {
			return selectionFailure("quarantine_receipt")
		}
		evidence.quarantines[quarantine.SourceKeyHMAC] = quarantine
		return nil
	}); err != nil {
		return dm01TableEvidence{}, normalizeSelectionError(err, "quarantine_stream")
	}
	for key, receipt := range evidence.receipts {
		_, quarantined := evidence.quarantines[key]
		if (receipt.Disposition == "quarantined") != quarantined {
			return dm01TableEvidence{}, selectionFailure("receipt_quarantine")
		}
	}
	return evidence, nil
}

func matchDeferredQuarantine(evidence dm01TableEvidence, sourceTable, reason string, sourceID int64, dm01HMACKey []byte, keyVersion int16) (DM01SelectionLineage, error) {
	key, err := dm01SourceKey(dm01HMACKey, sourceTable, sourceID)
	if err != nil {
		return DM01SelectionLineage{}, err
	}
	receipt, found := evidence.receipts[key]
	if !found || receipt.Disposition != "quarantined" {
		return DM01SelectionLineage{}, selectionFailure("source_receipt")
	}
	quarantine, found := evidence.quarantines[key]
	if !found || !sameDM01Quarantine(receipt, quarantine) || quarantine.ReasonCode != reason {
		return DM01SelectionLineage{}, selectionFailure("source_quarantine")
	}
	delete(evidence.receipts, key)
	delete(evidence.quarantines, key)
	return selectionLineage(receipt, keyVersion, reason), nil
}

func allDM01QuarantinesConsumed(evidence dm01TableEvidence) bool {
	return len(evidence.receipts) == 0 && len(evidence.quarantines) == 0
}

func dm01SourceKey(key []byte, sourceTable string, sourceID int64) ([sha256.Size]byte, error) {
	value, err := contactmigration.SourceKeyHMAC(key, sourceTable, strconv.FormatInt(sourceID, 10))
	if err != nil || len(value) != sha256.Size {
		return [sha256.Size]byte{}, selectionFailure("dm01_source_hmac")
	}
	var result [sha256.Size]byte
	copy(result[:], value)
	return result, nil
}

func sameDM01Quarantine(receipt DM01TerminalReceipt, quarantine DM01Quarantine) bool {
	return receipt.SourceKeyHMAC == quarantine.SourceKeyHMAC && receipt.PayloadHMAC == quarantine.PayloadHMAC && receipt.FieldDigest == quarantine.FieldDigest
}

func selectionLineage(receipt DM01TerminalReceipt, keyVersion int16, reason string) DM01SelectionLineage {
	return DM01SelectionLineage{SourceKeyHMAC: receipt.SourceKeyHMAC, PayloadHMAC: receipt.PayloadHMAC, FieldDigest: receipt.FieldDigest, KeyVersion: keyVersion, ReasonCode: reason}
}

func validDeferredIdentityOptions(options DeferredIdentitySelectionOptions) bool {
	return options.ArchiveRunID != "" && options.DM01RunID > 0 && len(options.ArchiveHMACKey) >= sha256.Size && len(options.DM01HMACKey) >= sha256.Size && options.DM01HMACKeyVersion > 0
}

func normalizeSelectionError(err error, fallback string) error {
	var selectionErr *SelectionError
	if errors.As(err, &selectionErr) {
		return selectionErr
	}
	return selectionFailure(fallback)
}

func nilDeferredIdentity(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Func) && reflected.IsNil()
}

func zeroDigest(value [sha256.Size]byte) bool { return value == [sha256.Size]byte{} }
