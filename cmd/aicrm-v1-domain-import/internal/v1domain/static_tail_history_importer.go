package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1statictail"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

// StaticTailMediaWriter is the Media-owned boundary. Its implementation must
// write only through the caller transaction and never create a live invite.
type StaticTailMediaWriter interface {
	ImportGroupInvite(context.Context, string, mediaport.HistoricalGroupInvite) (mediaport.StaticMediaHistoryReceipt, error)
}

// StaticTailProductWriter writes immutable page-slice history only.
type StaticTailProductWriter interface {
	ImportProductPageSlice(context.Context, string, productport.HistoricalProductPageSlice) (productport.StaticProductHistoryReceipt, error)
}

// StaticTailCycleWriter writes disabled historical strategy definitions and
// their same-batch historical parents only.
type StaticTailCycleWriter interface {
	ImportCycleStrategy(context.Context, string, cycleport.HistoricalCycleStrategy) (cycleport.StaticCycleHistoryReceipt, error)
	ImportCycleVersion(context.Context, string, cycleport.HistoricalCycleVersion) (cycleport.StaticCycleHistoryReceipt, error)
	ImportCycleDocument(context.Context, string, cycleport.HistoricalCycleDocument) (cycleport.StaticCycleHistoryReceipt, error)
}

type StaticTailHistoryImportJournal interface {
	ValidateStaticTailHistoryImportScope(string) error
	LoadTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordTerminal(context.Context, string, TerminalReceipt) error
}

type StaticTailHistoryTableResult struct{ Imported, Quarantined, Replayed int }

type StaticTailHistoryImportResult struct {
	Tables map[string]StaticTailHistoryTableResult
}

func (result StaticTailHistoryImportResult) SourceCount() int {
	total := 0
	for _, value := range result.Tables {
		total += value.Imported + value.Quarantined
	}
	return total
}

// StaticTailHistoryImporter writes 54 sealed V1 facts. The command layer may
// subsequently project verified product page slices into current Product data;
// this importer itself has no current write path or external effect.
type StaticTailHistoryImporter struct {
	archive       ArchiveSource
	uow           UnitOfWork
	mediaWriter   StaticTailMediaWriter
	productWriter StaticTailProductWriter
	cycleWriter   StaticTailCycleWriter
	journal       StaticTailHistoryImportJournal
}

func NewStaticTailHistoryImporter(archive ArchiveSource, uow UnitOfWork, mediaWriter StaticTailMediaWriter, productWriter StaticTailProductWriter, cycleWriter StaticTailCycleWriter, journal StaticTailHistoryImportJournal) (*StaticTailHistoryImporter, error) {
	if archive == nil || uow == nil || mediaWriter == nil || productWriter == nil || cycleWriter == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &StaticTailHistoryImporter{archive: archive, uow: uow, mediaWriter: mediaWriter, productWriter: productWriter, cycleWriter: cycleWriter, journal: journal}, nil
}

type staticTailHistoryRows struct {
	archive []v1archive.ArchivedRow
	records []v1statictail.SourceRecord
}

func (importer *StaticTailHistoryImporter) Import(ctx context.Context, archiveRunID string) (StaticTailHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.mediaWriter == nil || importer.productWriter == nil || importer.cycleWriter == nil || importer.journal == nil ||
		archiveRunID == "" || importer.journal.ValidateStaticTailHistoryImportScope(archiveRunID) != nil {
		return StaticTailHistoryImportResult{}, ErrInvalidScope
	}
	loaded := make(map[string]staticTailHistoryRows, len(staticTailHistoryScopes))
	for _, scope := range staticTailHistoryScopes {
		rows, err := importer.readRows(ctx, archiveRunID, scope.table)
		if err != nil {
			return StaticTailHistoryImportResult{}, err
		}
		loaded[scope.table] = rows
	}
	history := v1statictail.AdaptHistory(
		loaded[staticTailGroupInviteTable].records,
		loaded[staticTailPageSliceTable].records,
		loaded[staticTailStrategyTable].records,
		loaded[staticTailVersionTable].records,
		loaded[staticTailDocumentTable].records,
	)
	if len(history.GroupInvites) != len(loaded[staticTailGroupInviteTable].archive) || len(history.PageSlices) != len(loaded[staticTailPageSliceTable].archive) ||
		len(history.Strategies) != len(loaded[staticTailStrategyTable].archive) || len(history.Versions) != len(loaded[staticTailVersionTable].archive) || len(history.Documents) != len(loaded[staticTailDocumentTable].archive) {
		return StaticTailHistoryImportResult{}, ErrConflict
	}
	result := staticTailHistoryResult()
	for index, decision := range history.GroupInvites {
		if err := importer.importGroupInvite(ctx, loaded[staticTailGroupInviteTable].archive[index], decision, &result); err != nil {
			return StaticTailHistoryImportResult{}, err
		}
	}
	for index, decision := range history.PageSlices {
		if err := importer.importPageSlice(ctx, loaded[staticTailPageSliceTable].archive[index], decision, &result); err != nil {
			return StaticTailHistoryImportResult{}, err
		}
	}
	strategies := make(map[int64]int64, len(history.Strategies))
	for index, decision := range history.Strategies {
		if err := importer.importCycleStrategy(ctx, loaded[staticTailStrategyTable].archive[index], decision, strategies, &result); err != nil {
			return StaticTailHistoryImportResult{}, err
		}
	}
	versions := make(map[int64]int64, len(history.Versions))
	for index, decision := range history.Versions {
		if err := importer.importCycleVersion(ctx, loaded[staticTailVersionTable].archive[index], decision, strategies, versions, &result); err != nil {
			return StaticTailHistoryImportResult{}, err
		}
	}
	for index, decision := range history.Documents {
		if err := importer.importCycleDocument(ctx, loaded[staticTailDocumentTable].archive[index], decision, versions, &result); err != nil {
			return StaticTailHistoryImportResult{}, err
		}
	}
	return result, nil
}

func staticTailHistoryResult() StaticTailHistoryImportResult {
	result := StaticTailHistoryImportResult{Tables: make(map[string]StaticTailHistoryTableResult, len(staticTailHistoryScopes))}
	for _, scope := range staticTailHistoryScopes {
		result.Tables[scope.table] = StaticTailHistoryTableResult{}
	}
	return result
}

func (importer *StaticTailHistoryImporter) readRows(ctx context.Context, archiveRunID, table string) (staticTailHistoryRows, error) {
	rows := staticTailHistoryRows{}
	seen := map[[sha256.Size]byte]struct{}{}
	ordinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, table, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) ||
			row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
			return ErrConflict
		}
		ordinal++
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		payload := append(json.RawMessage(nil), row.Payload...)
		if len(row.RedactedFields) != 0 {
			payload = json.RawMessage(`{}`)
		}
		rows.archive = append(rows.archive, row)
		rows.records = append(rows.records, v1statictail.SourceRecord{Payload: payload, PayloadHMAC: v1statictail.OpaqueDigest(row.PayloadHMAC)})
		return nil
	})
	return rows, err
}

func (importer *StaticTailHistoryImporter) importGroupInvite(ctx context.Context, row v1archive.ArchivedRow, decision v1statictail.GroupInviteResult, result *StaticTailHistoryImportResult) error {
	if reason := staticTailDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "static_tail_group_invite_invalid"); reason != "" {
		return importer.quarantine(ctx, row, reason, result)
	}
	fact := *decision.Fact
	if !staticTailDigestMatches(row, fact.SealedSourceDigest) {
		return ErrConflict
	}
	value := mediaport.HistoricalGroupInvite{SourceID: fact.SourceID, SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC,
		Name: fact.Name, Title: fact.Title, Description: fact.Description, OriginalState: fact.OriginalState, OriginalAutoCreate: fact.OriginalAutoCreate,
		RoomBaseName: fact.RoomBaseName, RoomBaseSourceID: copyInt64(fact.RoomBaseSourceID), OriginalEnabled: fact.OriginalEnabled, OriginalBindingState: fact.OriginalBindingState,
		CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt}
	return importer.write(ctx, staticTailGroupInviteKind, row, result, func(tx context.Context) (staticTailWriteReceipt, error) {
		receipt, err := importer.mediaWriter.ImportGroupInvite(tx, SourceIdentifier(row.SourceKeyHMAC), value)
		return staticTailMediaWriteReceipt(receipt), err
	})
}

func (importer *StaticTailHistoryImporter) importPageSlice(ctx context.Context, row v1archive.ArchivedRow, decision v1statictail.ProductPageSliceResult, result *StaticTailHistoryImportResult) error {
	if reason := staticTailDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "static_tail_page_slice_invalid"); reason != "" {
		return importer.quarantine(ctx, row, reason, result)
	}
	fact := *decision.Fact
	if !staticTailDigestMatches(row, fact.SealedSourceDigest) {
		return ErrConflict
	}
	value := productport.HistoricalProductPageSlice{SourceID: fact.SourceID, SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC,
		ProductSourceID: fact.ProductSourceID, ImageSourceID: fact.ImageSourceID, SortOrder: fact.SortOrder, OriginalEnabled: fact.OriginalEnabled, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt}
	return importer.write(ctx, staticTailPageSliceKind, row, result, func(tx context.Context) (staticTailWriteReceipt, error) {
		receipt, err := importer.productWriter.ImportProductPageSlice(tx, SourceIdentifier(row.SourceKeyHMAC), value)
		return staticTailProductWriteReceipt(receipt), err
	})
}

func (importer *StaticTailHistoryImporter) importCycleStrategy(ctx context.Context, row v1archive.ArchivedRow, decision v1statictail.OperationCycleStrategyResult, targets map[int64]int64, result *StaticTailHistoryImportResult) error {
	if reason := staticTailDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "static_tail_cycle_strategy_invalid"); reason != "" {
		return importer.quarantine(ctx, row, reason, result)
	}
	fact := *decision.Fact
	if !staticTailDigestMatches(row, fact.SealedSourceDigest) {
		return ErrConflict
	}
	value := cycleport.HistoricalCycleStrategy{SourceID: fact.SourceID, SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC,
		StrategyKey: fact.StrategyKey, Title: fact.Title, Description: fact.Description, Cadence: fact.Cadence, Timezone: fact.Timezone,
		OriginalStatus: fact.OriginalStatus, CurrentVersion: fact.CurrentVersion, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt}
	var targetID int64
	err := importer.write(ctx, staticTailCycleStrategyKind, row, result, func(tx context.Context) (staticTailWriteReceipt, error) {
		receipt, err := importer.cycleWriter.ImportCycleStrategy(tx, SourceIdentifier(row.SourceKeyHMAC), value)
		if err == nil {
			targetID = receipt.TargetID
		}
		return staticTailCycleWriteReceipt(receipt), err
	})
	if err != nil {
		return err
	}
	if targetID == 0 {
		return nil
	}
	if targetID < 1 {
		return ErrConflict
	}
	if prior, found := targets[fact.SourceID]; found && prior != targetID {
		return ErrConflict
	}
	targets[fact.SourceID] = targetID
	return nil
}

func (importer *StaticTailHistoryImporter) importCycleVersion(ctx context.Context, row v1archive.ArchivedRow, decision v1statictail.OperationCycleVersionResult, strategies, targets map[int64]int64, result *StaticTailHistoryImportResult) error {
	if reason := staticTailDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "static_tail_cycle_version_invalid"); reason != "" {
		return importer.quarantine(ctx, row, reason, result)
	}
	fact := *decision.Fact
	strategyID, found := strategies[fact.StrategySourceID]
	if !found || strategyID < 1 {
		return importer.quarantine(ctx, row, "static_tail_cycle_version_parent_unresolved", result)
	}
	if !staticTailDigestMatches(row, fact.SealedSourceDigest) {
		return ErrConflict
	}
	value := cycleport.HistoricalCycleVersion{SourceID: fact.SourceID, SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC,
		StrategySourceID: fact.StrategySourceID, StrategyHistoryID: strategyID, Version: fact.Version, Label: fact.Label, Objective: fact.Objective,
		VersionHash: fact.VersionHash, EffectiveFrom: copyStaticTailTime(fact.EffectiveFrom), OriginalGovernance: fact.OriginalGovernance, ConfirmedAt: copyStaticTailTime(fact.ConfirmedAt),
		OperationSkillHash: fact.OperationSkillHash, CreatedAt: fact.CreatedAt}
	var targetID int64
	err := importer.write(ctx, staticTailCycleVersionKind, row, result, func(tx context.Context) (staticTailWriteReceipt, error) {
		receipt, err := importer.cycleWriter.ImportCycleVersion(tx, SourceIdentifier(row.SourceKeyHMAC), value)
		if err == nil {
			targetID = receipt.TargetID
		}
		return staticTailCycleWriteReceipt(receipt), err
	})
	if err != nil {
		return err
	}
	if targetID == 0 {
		return nil
	}
	if targetID < 1 {
		return ErrConflict
	}
	if prior, found := targets[fact.SourceID]; found && prior != targetID {
		return ErrConflict
	}
	targets[fact.SourceID] = targetID
	return nil
}

func (importer *StaticTailHistoryImporter) importCycleDocument(ctx context.Context, row v1archive.ArchivedRow, decision v1statictail.OperationCycleDocumentResult, versions map[int64]int64, result *StaticTailHistoryImportResult) error {
	if reason := staticTailDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "static_tail_cycle_document_invalid"); reason != "" {
		return importer.quarantine(ctx, row, reason, result)
	}
	fact := *decision.Fact
	versionID, found := versions[fact.StrategyVersionSourceID]
	if !found || versionID < 1 {
		return importer.quarantine(ctx, row, "static_tail_cycle_document_parent_unresolved", result)
	}
	if !staticTailDigestMatches(row, fact.SealedSourceDigest) {
		return ErrConflict
	}
	value := cycleport.HistoricalCycleDocument{SourceID: fact.SourceID, SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC,
		StrategyVersionSourceID: fact.StrategyVersionSourceID, VersionHistoryID: versionID, SchemaVersion: fact.SchemaVersion,
		ExecutionGuideSHA256: fact.ExecutionGuideSHA256, ExecutionGuideGeneratedAt: copyStaticTailTime(fact.ExecutionGuideGeneratedAt),
		CopyGuideSHA256: fact.CopyGuideSHA256, CopyGuideGeneratedAt: copyStaticTailTime(fact.CopyGuideGeneratedAt),
		MeasurementGuideSHA256: fact.MeasurementGuideSHA256, MeasurementGuideGeneratedAt: copyStaticTailTime(fact.MeasurementGuideGeneratedAt),
		DocumentPackHash: fact.DocumentPackHash, CreatedAt: fact.CreatedAt}
	return importer.write(ctx, staticTailCycleDocumentKind, row, result, func(tx context.Context) (staticTailWriteReceipt, error) {
		receipt, err := importer.cycleWriter.ImportCycleDocument(tx, SourceIdentifier(row.SourceKeyHMAC), value)
		return staticTailCycleWriteReceipt(receipt), err
	})
}

type staticTailWriteReceipt struct {
	kind, source    string
	payload, target [sha256.Size]byte
	targetID        int64
	replayed        bool
}

func staticTailMediaWriteReceipt(receipt mediaport.StaticMediaHistoryReceipt) staticTailWriteReceipt {
	return staticTailWriteReceipt{kind: receipt.Kind, source: receipt.SourceIdentifier, payload: receipt.PayloadDigest, target: receipt.TargetDigest, targetID: receipt.TargetID, replayed: receipt.Replayed}
}
func staticTailProductWriteReceipt(receipt productport.StaticProductHistoryReceipt) staticTailWriteReceipt {
	return staticTailWriteReceipt{kind: receipt.Kind, source: receipt.SourceIdentifier, payload: receipt.PayloadDigest, target: receipt.TargetDigest, targetID: receipt.TargetID, replayed: receipt.Replayed}
}
func staticTailCycleWriteReceipt(receipt cycleport.StaticCycleHistoryReceipt) staticTailWriteReceipt {
	return staticTailWriteReceipt{kind: receipt.Kind, source: receipt.SourceIdentifier, payload: receipt.PayloadDigest, target: receipt.TargetDigest, targetID: receipt.TargetID, replayed: receipt.Replayed}
}

func (importer *StaticTailHistoryImporter) write(ctx context.Context, kind string, row v1archive.ArchivedRow, result *StaticTailHistoryImportResult, apply func(context.Context) (staticTailWriteReceipt, error)) error {
	imported, replayed := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		imported, replayed = false, false
		receipt, err := apply(tx)
		if staticTailTargetInvalid(err) {
			var quarantineErr error
			replayed, quarantineErr = importer.recordQuarantine(tx, row, "static_tail_history_target_invalid")
			return quarantineErr
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, kind, row, receipt); err != nil {
			return err
		}
		imported, replayed = true, receipt.replayed
		return nil
	})
	if err != nil {
		return err
	}
	value := result.Tables[row.TableID]
	if imported {
		value.Imported++
	} else {
		value.Quarantined++
	}
	if replayed {
		value.Replayed++
	}
	result.Tables[row.TableID] = value
	return nil
}

func (importer *StaticTailHistoryImporter) quarantine(ctx context.Context, row v1archive.ArchivedRow, reason string, result *StaticTailHistoryImportResult) error {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var err error
		replayed, err = importer.recordQuarantine(tx, row, reason)
		return err
	})
	if err != nil {
		return err
	}
	value := result.Tables[row.TableID]
	value.Quarantined++
	if replayed {
		value.Replayed++
	}
	result.Tables[row.TableID] = value
	return nil
}

func (importer *StaticTailHistoryImporter) recordQuarantine(ctx context.Context, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := importer.journal.LoadTerminal(ctx, row.TableID, source)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason ||
			existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, importer.journal.RecordTerminal(ctx, row.TableID, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func (importer *StaticTailHistoryImporter) verifyReceipt(ctx context.Context, kind string, row v1archive.ArchivedRow, receipt staticTailWriteReceipt) error {
	if receipt.kind != kind || receipt.source != SourceIdentifier(row.SourceKeyHMAC) || receipt.payload != row.PayloadHMAC || receipt.targetID < 1 || receipt.target == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := importer.journal.LoadTerminal(ctx, row.TableID, receipt.source)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetID != strconv.FormatInt(receipt.targetID, 10) || terminal.TargetDigest != receipt.target || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func staticTailTargetInvalid(err error) bool {
	return errors.Is(err, mediaport.ErrStaticMediaHistoryInvalid) || errors.Is(err, productport.ErrStaticProductHistoryInvalid) || errors.Is(err, cycleport.ErrStaticCycleHistoryInvalid)
}

func staticTailDecisionReason(disposition v1statictail.Disposition, hasFact bool, reason, fallback string) string {
	if disposition == v1statictail.DispositionCandidate && hasFact {
		return ""
	}
	if disposition == v1statictail.DispositionQuarantine && reason != "" {
		return reason
	}
	return fallback
}

func staticTailDigestMatches(row v1archive.ArchivedRow, digest v1statictail.OpaqueDigest) bool {
	return [sha256.Size]byte(digest) == row.PayloadHMAC && row.PayloadHMAC != ([sha256.Size]byte{})
}

func copyStaticTailTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
