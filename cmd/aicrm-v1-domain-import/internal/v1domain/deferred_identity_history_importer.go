package v1domain

import (
	"context"
	"crypto/sha256"
	"reflect"
	"strconv"
	"time"

	v1deferredidentityhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// DeferredIdentityHistoryWriter owns only inert Contact history evidence. It
// cannot create a current Customer or bind an identity.
type DeferredIdentityHistoryWriter interface {
	ImportHistoricalDeferredPerson(context.Context, string, contactport.HistoricalDeferredPerson) (contactport.DeferredIdentityHistoryReceipt, error)
	ImportHistoricalDeferredIdentityConflict(context.Context, string, contactport.HistoricalDeferredIdentityConflict) (contactport.DeferredIdentityHistoryReceipt, error)
	ImportHistoricalMissingRootIdentity(context.Context, string, contactport.HistoricalMissingRootIdentity) (contactport.DeferredIdentityHistoryReceipt, error)
}

type DeferredIdentityHistoryImportResult struct {
	ImportedPeople, ImportedConflicts, ImportedMissingRoots int
	ReplayedPeople, ReplayedConflicts, ReplayedMissingRoots int
}

func (result DeferredIdentityHistoryImportResult) Count() int {
	return result.ImportedPeople + result.ImportedConflicts + result.ImportedMissingRoots + result.ReplayedPeople + result.ReplayedConflicts + result.ReplayedMissingRoots
}

type DeferredIdentityHistoryImporter struct {
	archive v1deferredidentityhistory.ArchiveReader
	dm01    v1deferredidentityhistory.DM01EvidenceReader
	uow     UnitOfWork
	writer  DeferredIdentityHistoryWriter
	journal DeferredIdentityHistoryImportJournal
	options v1deferredidentityhistory.DeferredIdentitySelectionOptions
}

func NewDeferredIdentityHistoryImporter(
	archive v1deferredidentityhistory.ArchiveReader,
	dm01 v1deferredidentityhistory.DM01EvidenceReader,
	uow UnitOfWork,
	writer DeferredIdentityHistoryWriter,
	journal DeferredIdentityHistoryImportJournal,
	options v1deferredidentityhistory.DeferredIdentitySelectionOptions,
) (*DeferredIdentityHistoryImporter, error) {
	if nilDeferredHistory(archive) || nilDeferredHistory(dm01) || nilDeferredHistory(uow) || nilDeferredHistory(writer) || nilDeferredHistory(journal) || !validDeferredIdentityHistoryOptions(options) {
		return nil, ErrInvalidScope
	}
	return &DeferredIdentityHistoryImporter{archive: archive, dm01: dm01, uow: uow, writer: writer, journal: journal, options: options}, nil
}

// Import selects and adapts all 1,392 source facts before it opens the first
// target transaction. Each later writer+receipt pair then uses one caller UoW.
func (importer *DeferredIdentityHistoryImporter) Import(ctx context.Context) (DeferredIdentityHistoryImportResult, error) {
	if importer == nil || ctx == nil || nilDeferredHistory(importer.archive) || nilDeferredHistory(importer.dm01) || nilDeferredHistory(importer.uow) || nilDeferredHistory(importer.writer) || nilDeferredHistory(importer.journal) || !validDeferredIdentityHistoryOptions(importer.options) || importer.journal.ValidateDeferredIdentityHistoryImportScope(importer.options.ArchiveRunID) != nil {
		return DeferredIdentityHistoryImportResult{}, ErrInvalidScope
	}
	selection, err := v1deferredidentityhistory.SelectDeferredIdentityEvidence(ctx, importer.archive, importer.dm01, importer.options)
	if err != nil {
		return DeferredIdentityHistoryImportResult{}, err
	}
	if selection.Count() != 1392 {
		return DeferredIdentityHistoryImportResult{}, ErrConflict
	}
	result := DeferredIdentityHistoryImportResult{}
	for _, selected := range selection.People {
		if !deferredPersonSelectionMatches(selected) {
			return DeferredIdentityHistoryImportResult{}, ErrConflict
		}
		value := deferredPersonValue(selected)
		replayed, err := importer.write(ctx, DeferredPersonHistoryKind, selected.ArchivedRow, func(tx context.Context) (contactport.DeferredIdentityHistoryReceipt, error) {
			return importer.writer.ImportHistoricalDeferredPerson(tx, SourceIdentifier(selected.ArchivedRow.SourceKeyHMAC), value)
		})
		if err != nil {
			return DeferredIdentityHistoryImportResult{}, err
		}
		if replayed {
			result.ReplayedPeople++
		} else {
			result.ImportedPeople++
		}
	}
	for _, selected := range selection.IdentityConflicts {
		if !deferredConflictSelectionMatches(selected) {
			return DeferredIdentityHistoryImportResult{}, ErrConflict
		}
		value := deferredConflictValue(selected)
		replayed, err := importer.write(ctx, DeferredConflictHistoryKind, selected.ArchivedRow, func(tx context.Context) (contactport.DeferredIdentityHistoryReceipt, error) {
			return importer.writer.ImportHistoricalDeferredIdentityConflict(tx, SourceIdentifier(selected.ArchivedRow.SourceKeyHMAC), value)
		})
		if err != nil {
			return DeferredIdentityHistoryImportResult{}, err
		}
		if replayed {
			result.ReplayedConflicts++
		} else {
			result.ImportedConflicts++
		}
	}
	for _, selected := range selection.MissingCustomerRootMaps {
		if !missingRootSelectionMatches(selected, importer.options) {
			return DeferredIdentityHistoryImportResult{}, ErrConflict
		}
		value := missingRootValue(selected, importer.options)
		replayed, err := importer.write(ctx, MissingRootIdentityKind, selected.ArchivedRow, func(tx context.Context) (contactport.DeferredIdentityHistoryReceipt, error) {
			return importer.writer.ImportHistoricalMissingRootIdentity(tx, SourceIdentifier(selected.ArchivedRow.SourceKeyHMAC), value)
		})
		if err != nil {
			return DeferredIdentityHistoryImportResult{}, err
		}
		if replayed {
			result.ReplayedMissingRoots++
		} else {
			result.ImportedMissingRoots++
		}
	}
	if result.Count() != selection.Count() {
		return DeferredIdentityHistoryImportResult{}, ErrConflict
	}
	return result, nil
}

func (importer *DeferredIdentityHistoryImporter) write(ctx context.Context, kind string, row v1archive.ArchivedRow, apply func(context.Context) (contactport.DeferredIdentityHistoryReceipt, error)) (replayed bool, err error) {
	if !validDeferredIdentityHistoryKind(kind) || !validDeferredArchiveRow(row) || apply == nil {
		return false, ErrInvalidScope
	}
	err = importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		receipt, writeErr := apply(tx)
		if writeErr != nil {
			return writeErr
		}
		if verifyErr := verifyDeferredIdentityHistoryReceipt(tx, importer.journal, kind, row, receipt); verifyErr != nil {
			return verifyErr
		}
		replayed = receipt.Replayed
		return nil
	})
	return replayed, err
}

func verifyDeferredIdentityHistoryReceipt(ctx context.Context, journal DeferredIdentityHistoryImportJournal, kind string, row v1archive.ArchivedRow, receipt contactport.DeferredIdentityHistoryReceipt) error {
	source := SourceIdentifier(row.SourceKeyHMAC)
	if receipt.Kind != kind || receipt.SourceIdentifier != source || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := journal.LoadDeferredIdentityHistoryTerminal(ctx, kind, source)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	stored, found, err := journal.LoadDeferredIdentityHistory(ctx, kind, source)
	if err != nil || !found || stored.Kind != receipt.Kind || stored.SourceIdentifier != receipt.SourceIdentifier || stored.PayloadDigest != receipt.PayloadDigest || stored.TargetID != receipt.TargetID || stored.TargetDigest != receipt.TargetDigest {
		return ErrConflict
	}
	return nil
}

func deferredPersonValue(selected v1deferredidentityhistory.SelectedPerson) contactport.HistoricalDeferredPerson {
	fact := selected.Fact
	return contactport.HistoricalDeferredPerson{
		SourceKeyDigest: [32]byte(fact.Source.SourceKeyDigest), SourcePayloadDigest: [32]byte(fact.Source.PayloadDigest), SourceFieldDigest: [32]byte(fact.Source.FieldDigest),
		SourceID: fact.SourceID, MobileDigest: [32]byte(fact.MobileDigest), ThirdPartyUserIDDigest: [32]byte(fact.ThirdPartyUserIDDigest), PrivateDigest: [32]byte(fact.PrivateDigest),
		RedactedRoots: cloneDeferredHistoryRoots(fact.RedactedRoots), CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt,
	}
}

func deferredConflictValue(selected v1deferredidentityhistory.SelectedIdentityConflict) contactport.HistoricalDeferredIdentityConflict {
	fact := selected.Fact
	return contactport.HistoricalDeferredIdentityConflict{
		SourceKeyDigest: [32]byte(fact.Source.SourceKeyDigest), SourcePayloadDigest: [32]byte(fact.Source.PayloadDigest), SourceFieldDigest: [32]byte(fact.Source.FieldDigest), SourceID: fact.SourceID,
		ConflictType: fact.ConflictType, SourceType: fact.SourceType, Status: fact.Status, ResolutionStatus: fact.ResolutionStatus,
		UnionIDDigest: [32]byte(fact.UnionIDDigest), CandidateUnionIDDigest: [32]byte(fact.CandidateUnionIDDigest), ExternalUserIDDigest: [32]byte(fact.ExternalUserIDDigest), OpenIDDigest: [32]byte(fact.OpenIDDigest), MobileDigest: [32]byte(fact.MobileDigest),
		LegacySourceKeyDigest: [32]byte(fact.SourceKeyDigest), PayloadJSONDigest: [32]byte(fact.PayloadJSONDigest), SourcePayloadJSONDigest: [32]byte(fact.SourcePayloadDigest), ResolutionNoteDigest: [32]byte(fact.ResolutionNoteDigest), PrivateDigest: [32]byte(fact.PrivateDigest),
		RedactedRoots: cloneDeferredHistoryRoots(fact.RedactedRoots), CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt, ResolvedAt: cloneDeferredHistoryTime(fact.ResolvedAt),
	}
}

func missingRootValue(selected v1deferredidentityhistory.SelectedMissingRootIdentity, options v1deferredidentityhistory.DeferredIdentitySelectionOptions) contactport.HistoricalMissingRootIdentity {
	fact := selected.Fact
	return contactport.HistoricalMissingRootIdentity{
		SourceKeyDigest: [32]byte(fact.Source.SourceKeyDigest), SourcePayloadDigest: [32]byte(fact.Source.PayloadDigest), SourceFieldDigest: [32]byte(fact.Source.FieldDigest), SourceID: fact.SourceID,
		DM01RunID: options.DM01RunID, DM01SourceKeyDigest: selected.Lineage.SourceKeyHMAC, DM01SourceHMACKeyVersion: strconv.FormatInt(int64(selected.Lineage.KeyVersion), 10), QuarantineReason: selected.Lineage.ReasonCode,
		Type: cloneDeferredHistoryInt32(fact.Type), Status: fact.Status, CorpIDDigest: [32]byte(fact.CorpIDDigest), ExternalUserIDDigest: [32]byte(fact.ExternalUserIDDigest), UnionIDDigest: [32]byte(fact.UnionIDDigest), OpenIDDigest: [32]byte(fact.OpenIDDigest), FollowUserIDDigest: [32]byte(fact.FollowUserIDDigest),
		NameDigest: [32]byte(fact.NameDigest), AvatarDigest: [32]byte(fact.AvatarDigest), GenderDigest: cloneDeferredHistoryDigest(fact.GenderDigest), RawProfileDigest: [32]byte(fact.RawProfileDigest), PrivateDigest: [32]byte(fact.PrivateDigest),
		RedactedRoots: cloneDeferredHistoryRoots(fact.RedactedRoots), FirstSeenAt: fact.FirstSeenAt, LastSeenAt: fact.LastSeenAt, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt,
	}
}

func deferredPersonSelectionMatches(selected v1deferredidentityhistory.SelectedPerson) bool {
	return sourceEnvelopeMatches(selected.ArchivedRow, selected.Fact.Source) && selected.Lineage.ReasonCode == v1deferredidentityhistory.TargetSchemaDeferredReason && validDeferredDM01Lineage(selected.Lineage)
}

func deferredConflictSelectionMatches(selected v1deferredidentityhistory.SelectedIdentityConflict) bool {
	return sourceEnvelopeMatches(selected.ArchivedRow, selected.Fact.Source) && selected.Lineage.ReasonCode == v1deferredidentityhistory.TargetSchemaDeferredReason && validDeferredDM01Lineage(selected.Lineage)
}

func missingRootSelectionMatches(selected v1deferredidentityhistory.SelectedMissingRootIdentity, options v1deferredidentityhistory.DeferredIdentitySelectionOptions) bool {
	return sourceEnvelopeMatches(selected.ArchivedRow, selected.Fact.Source) && selected.Lineage.ReasonCode == v1deferredidentityhistory.MissingCustomerRootReason && selected.Lineage.KeyVersion == options.DM01HMACKeyVersion && validDeferredDM01Lineage(selected.Lineage)
}

func sourceEnvelopeMatches(row v1archive.ArchivedRow, source v1deferredidentityhistory.SourceEnvelope) bool {
	return validDeferredArchiveRow(row) && [32]byte(source.SourceKeyDigest) == row.SourceKeyHMAC && [32]byte(source.PayloadDigest) == row.PayloadHMAC && [32]byte(source.FieldDigest) == row.FieldHMAC
}

func validDeferredDM01Lineage(lineage v1deferredidentityhistory.DM01SelectionLineage) bool {
	return lineage.SourceKeyHMAC != ([32]byte{}) && lineage.PayloadHMAC != ([32]byte{}) && lineage.FieldDigest != ([32]byte{}) && lineage.KeyVersion > 0 && lineage.ReasonCode != ""
}

func validDeferredArchiveRow(row v1archive.ArchivedRow) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.SourceOrdinal > 0 && row.SourceKeyHMAC != ([32]byte{}) && row.PayloadHMAC != ([32]byte{}) && row.FieldHMAC != ([32]byte{})
}

func validDeferredIdentityHistoryOptions(options v1deferredidentityhistory.DeferredIdentitySelectionOptions) bool {
	return options.ArchiveRunID != "" && options.DM01RunID > 0 && len(options.ArchiveHMACKey) >= sha256.Size && len(options.DM01HMACKey) >= sha256.Size && options.DM01HMACKeyVersion > 0
}

func cloneDeferredHistoryRoots(values []string) []string {
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func cloneDeferredHistoryTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneDeferredHistoryInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneDeferredHistoryDigest(value *v1deferredidentityhistory.OpaqueDigest) *[32]byte {
	if value == nil {
		return nil
	}
	copy := [32]byte(*value)
	return &copy
}

func nilDeferredHistory(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Func) && reflected.IsNil()
}
