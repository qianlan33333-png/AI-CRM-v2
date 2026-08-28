package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1profilecatalog"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// ProfileCatalogHistoryWriter is the private adapter boundary. Its concrete
// implementation calls Segment and Contact owner services in the caller Tx.
type ProfileCatalogHistoryWriter interface {
	ApplyTemplate(context.Context, v1profilecatalog.SourceBinding, v1profilecatalog.TemplateFact) (segmentport.ProfileCatalogHistoryReceipt, error)
	ApplyCategory(context.Context, v1profilecatalog.SourceBinding, v1profilecatalog.CategoryFact, segmentport.HistoricalProfileTemplate) (segmentport.ProfileCatalogHistoryReceipt, error)
	ApplyOptionMapping(context.Context, v1profilecatalog.SourceBinding, v1profilecatalog.OptionMappingFact, segmentport.HistoricalProfileTemplate, segmentport.HistoricalProfileCategory) (segmentport.ProfileCatalogHistoryReceipt, error)
	ApplySignupTagRule(context.Context, v1profilecatalog.SourceBinding, v1profilecatalog.SignupTagRuleFact) (contactport.SignupTagHistoryReceipt, error)
}

// ProfileCatalogHistoryTargetReader must read through the caller transaction:
// parent IDs written earlier in this transaction are not visible through a
// separate pool reader.
type ProfileCatalogHistoryTargetReader interface {
	ReadTemplate(context.Context, int64) (segmentport.HistoricalProfileTemplate, error)
	ReadCategory(context.Context, int64) (segmentport.HistoricalProfileCategory, error)
	ReadOptionMapping(context.Context, int64) (segmentport.HistoricalProfileOptionMapping, error)
	ReadSignupTagRule(context.Context, int64) (contactport.HistoricalSignupTagRule, error)
}

type profileCatalogTerminalJournal interface {
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
}

type ProfileCatalogHistoryTableResult struct{ Imported, Quarantined, Replayed int }

type ProfileCatalogHistoryImportResult struct {
	Templates, Categories, OptionMappings, SignupTagRules ProfileCatalogHistoryTableResult
}

func (result ProfileCatalogHistoryImportResult) SourceCount() int {
	return result.Templates.Imported + result.Templates.Quarantined + result.Categories.Imported + result.Categories.Quarantined +
		result.OptionMappings.Imported + result.OptionMappings.Quarantined + result.SignupTagRules.Imported + result.SignupTagRules.Quarantined
}

// ProfileCatalogHistoryImporter imports only the 30 non-executable source
// facts. It has no current Segment, tag catalogue, or Provider write path.
type ProfileCatalogHistoryImporter struct {
	archive      ArchiveSource
	uow          UnitOfWork
	writer       ProfileCatalogHistoryWriter
	reader       ProfileCatalogHistoryTargetReader
	journals     map[string]profileCatalogTerminalJournal
	archiveRunID string
}

func NewProfileCatalogHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer ProfileCatalogHistoryWriter, reader ProfileCatalogHistoryTargetReader, journals map[string]*Journal) (*ProfileCatalogHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || reader == nil || !validProfileCatalogHistoryJournals(journals) {
		return nil, ErrInvalidScope
	}
	terminals := make(map[string]profileCatalogTerminalJournal, len(journals))
	for table, journal := range journals {
		terminals[table] = journal
	}
	return newProfileCatalogHistoryImporter(archive, uow, writer, reader, terminals, journals[v1profilecatalog.ProfileTemplatesTableID].scope.ArchiveRunID)
}

func newProfileCatalogHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer ProfileCatalogHistoryWriter, reader ProfileCatalogHistoryTargetReader, journals map[string]profileCatalogTerminalJournal, archiveRunID string) (*ProfileCatalogHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || reader == nil || archiveRunID == "" || len(journals) != len(profileCatalogHistoryScopes) {
		return nil, ErrInvalidScope
	}
	for _, scope := range profileCatalogHistoryScopes {
		if journals[scope.source] == nil {
			return nil, ErrInvalidScope
		}
	}
	return &ProfileCatalogHistoryImporter{archive: archive, uow: uow, writer: writer, reader: reader, journals: journals, archiveRunID: archiveRunID}, nil
}

type profileCatalogRows struct {
	rows     []v1archive.ArchivedRow
	payloads []json.RawMessage
	redacted []bool
}

func (importer *ProfileCatalogHistoryImporter) Import(ctx context.Context, archiveRunID string) (ProfileCatalogHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.reader == nil ||
		archiveRunID == "" || archiveRunID != importer.archiveRunID || len(importer.journals) != len(profileCatalogHistoryScopes) {
		return ProfileCatalogHistoryImportResult{}, ErrInvalidScope
	}
	loaded := make(map[string]profileCatalogRows, len(profileCatalogHistoryScopes))
	for _, scope := range profileCatalogHistoryScopes {
		rows, err := importer.readRows(ctx, archiveRunID, scope.source)
		if err != nil {
			return ProfileCatalogHistoryImportResult{}, err
		}
		loaded[scope.source] = rows
	}
	history := v1profilecatalog.AdaptHistory(
		loaded[v1profilecatalog.ProfileTemplatesTableID].payloads,
		loaded[v1profilecatalog.ProfileCategoriesTableID].payloads,
		loaded[v1profilecatalog.ProfileOptionMappingsTableID].payloads,
		loaded[v1profilecatalog.SignupTagRulesTableID].payloads,
	)
	if len(history.Templates) != len(loaded[v1profilecatalog.ProfileTemplatesTableID].rows) || len(history.Categories) != len(loaded[v1profilecatalog.ProfileCategoriesTableID].rows) ||
		len(history.OptionMappings) != len(loaded[v1profilecatalog.ProfileOptionMappingsTableID].rows) || len(history.SignupTagRules) != len(loaded[v1profilecatalog.SignupTagRulesTableID].rows) {
		return ProfileCatalogHistoryImportResult{}, ErrConflict
	}

	result := ProfileCatalogHistoryImportResult{}
	templates := map[int64]segmentport.HistoricalProfileTemplate{}
	if err := importer.importTemplates(ctx, loaded[v1profilecatalog.ProfileTemplatesTableID], history.Templates, templates, &result.Templates); err != nil {
		return ProfileCatalogHistoryImportResult{}, err
	}
	categories := map[int64]segmentport.HistoricalProfileCategory{}
	if err := importer.importCategories(ctx, loaded[v1profilecatalog.ProfileCategoriesTableID], history.Categories, templates, categories, &result.Categories); err != nil {
		return ProfileCatalogHistoryImportResult{}, err
	}
	if err := importer.importOptionMappings(ctx, loaded[v1profilecatalog.ProfileOptionMappingsTableID], history.OptionMappings, templates, categories, &result.OptionMappings); err != nil {
		return ProfileCatalogHistoryImportResult{}, err
	}
	if err := importer.importSignupTagRules(ctx, loaded[v1profilecatalog.SignupTagRulesTableID], history.SignupTagRules, &result.SignupTagRules); err != nil {
		return ProfileCatalogHistoryImportResult{}, err
	}
	return result, nil
}

func (importer *ProfileCatalogHistoryImporter) readRows(ctx context.Context, archiveRunID, tableID string) (profileCatalogRows, error) {
	result := profileCatalogRows{}
	seen := map[[sha256.Size]byte]struct{}{}
	expectedOrdinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal != expectedOrdinal ||
			row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
			return ErrConflict
		}
		expectedOrdinal++
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		redacted := len(row.RedactedFields) != 0
		payload := append(json.RawMessage(nil), row.Payload...)
		if redacted {
			payload = json.RawMessage(`{}`)
		}
		result.rows, result.payloads, result.redacted = append(result.rows, row), append(result.payloads, payload), append(result.redacted, redacted)
		return nil
	})
	return result, err
}

func (importer *ProfileCatalogHistoryImporter) importTemplates(ctx context.Context, rows profileCatalogRows, values []v1profilecatalog.TemplateResult, targets map[int64]segmentport.HistoricalProfileTemplate, result *ProfileCatalogHistoryTableResult) error {
	if len(rows.rows) != len(values) {
		return ErrConflict
	}
	for index, decision := range values {
		row := rows.rows[index]
		if rows.redacted[index] {
			if err := importer.quarantine(ctx, v1profilecatalog.ProfileTemplatesTableID, row, "profile_catalog_redacted_field", result); err != nil {
				return err
			}
			continue
		}
		if decision.Disposition != v1profilecatalog.DispositionCandidate || decision.Fact == nil {
			if err := importer.quarantine(ctx, v1profilecatalog.ProfileTemplatesTableID, row, profileCatalogReason(decision.Reason, "profile_catalog_template_invalid"), result); err != nil {
				return err
			}
			continue
		}
		fact := *decision.Fact
		var target segmentport.HistoricalProfileTemplate
		replayed := false
		err := importer.uow.Within(ctx, func(tx context.Context) error {
			target, replayed = segmentport.HistoricalProfileTemplate{}, false
			receipt, err := importer.writer.ApplyTemplate(tx, profileCatalogBinding(row), fact)
			if err != nil {
				return err
			}
			if err = importer.verifyProfileReceipt(tx, v1profilecatalog.ProfileTemplatesTableID, row, v1profilecatalog.ProfileTemplatesKind, receipt); err != nil {
				return err
			}
			target, err = importer.reader.ReadTemplate(tx, receipt.TargetID)
			if err != nil || !sameProfileTemplateTarget(target, receipt.TargetID, row, fact) {
				if err != nil {
					return err
				}
				return ErrConflict
			}
			replayed = receipt.Replayed
			return nil
		})
		if errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
			if err = importer.quarantine(ctx, v1profilecatalog.ProfileTemplatesTableID, row, "profile_catalog_template_target_invalid", result); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if old, found := targets[fact.SourceID]; found && old.ID != target.ID {
			return ErrConflict
		}
		targets[fact.SourceID] = target
		result.Imported++
		if replayed {
			result.Replayed++
		}
	}
	return nil
}

func (importer *ProfileCatalogHistoryImporter) importCategories(ctx context.Context, rows profileCatalogRows, values []v1profilecatalog.CategoryResult, templates map[int64]segmentport.HistoricalProfileTemplate, targets map[int64]segmentport.HistoricalProfileCategory, result *ProfileCatalogHistoryTableResult) error {
	if len(rows.rows) != len(values) {
		return ErrConflict
	}
	for index, decision := range values {
		row := rows.rows[index]
		if rows.redacted[index] {
			if err := importer.quarantine(ctx, v1profilecatalog.ProfileCategoriesTableID, row, "profile_catalog_redacted_field", result); err != nil {
				return err
			}
			continue
		}
		if decision.Disposition != v1profilecatalog.DispositionCandidate || decision.Fact == nil {
			if err := importer.quarantine(ctx, v1profilecatalog.ProfileCategoriesTableID, row, profileCatalogReason(decision.Reason, "profile_catalog_category_invalid"), result); err != nil {
				return err
			}
			continue
		}
		fact := *decision.Fact
		parent, found := templates[fact.TemplateSourceID]
		if !found {
			if err := importer.quarantine(ctx, v1profilecatalog.ProfileCategoriesTableID, row, "profile_catalog_category_parent_unresolved", result); err != nil {
				return err
			}
			continue
		}
		var target segmentport.HistoricalProfileCategory
		replayed := false
		err := importer.uow.Within(ctx, func(tx context.Context) error {
			target, replayed = segmentport.HistoricalProfileCategory{}, false
			receipt, err := importer.writer.ApplyCategory(tx, profileCatalogBinding(row), fact, parent)
			if err != nil {
				return err
			}
			if err = importer.verifyProfileReceipt(tx, v1profilecatalog.ProfileCategoriesTableID, row, v1profilecatalog.ProfileCategoriesKind, receipt); err != nil {
				return err
			}
			target, err = importer.reader.ReadCategory(tx, receipt.TargetID)
			if err != nil || !sameProfileCategoryTarget(target, receipt.TargetID, row, fact, parent) {
				if err != nil {
					return err
				}
				return ErrConflict
			}
			replayed = receipt.Replayed
			return nil
		})
		if errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
			if err = importer.quarantine(ctx, v1profilecatalog.ProfileCategoriesTableID, row, "profile_catalog_category_target_invalid", result); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if old, found := targets[fact.SourceID]; found && old.ID != target.ID {
			return ErrConflict
		}
		targets[fact.SourceID] = target
		result.Imported++
		if replayed {
			result.Replayed++
		}
	}
	return nil
}

func (importer *ProfileCatalogHistoryImporter) importOptionMappings(ctx context.Context, rows profileCatalogRows, values []v1profilecatalog.OptionMappingResult, templates map[int64]segmentport.HistoricalProfileTemplate, categories map[int64]segmentport.HistoricalProfileCategory, result *ProfileCatalogHistoryTableResult) error {
	if len(rows.rows) != len(values) {
		return ErrConflict
	}
	for index, decision := range values {
		row := rows.rows[index]
		if rows.redacted[index] {
			if err := importer.quarantine(ctx, v1profilecatalog.ProfileOptionMappingsTableID, row, "profile_catalog_redacted_field", result); err != nil {
				return err
			}
			continue
		}
		if decision.Disposition != v1profilecatalog.DispositionCandidate || decision.Fact == nil {
			if err := importer.quarantine(ctx, v1profilecatalog.ProfileOptionMappingsTableID, row, profileCatalogReason(decision.Reason, "profile_catalog_option_mapping_invalid"), result); err != nil {
				return err
			}
			continue
		}
		fact := *decision.Fact
		template, hasTemplate := templates[fact.TemplateSourceID]
		category, hasCategory := categories[fact.CategorySourceID]
		if !hasTemplate || !hasCategory || category.TemplateHistoryID != template.ID || category.TemplateSourceID != fact.TemplateSourceID {
			if err := importer.quarantine(ctx, v1profilecatalog.ProfileOptionMappingsTableID, row, "profile_catalog_option_mapping_parent_unresolved", result); err != nil {
				return err
			}
			continue
		}
		replayed := false
		err := importer.uow.Within(ctx, func(tx context.Context) error {
			replayed = false
			receipt, err := importer.writer.ApplyOptionMapping(tx, profileCatalogBinding(row), fact, template, category)
			if err != nil {
				return err
			}
			if err = importer.verifyProfileReceipt(tx, v1profilecatalog.ProfileOptionMappingsTableID, row, v1profilecatalog.ProfileOptionMappingsKind, receipt); err != nil {
				return err
			}
			actual, err := importer.reader.ReadOptionMapping(tx, receipt.TargetID)
			if err != nil || !sameProfileOptionMappingTarget(actual, receipt.TargetID, row, fact, template, category) {
				if err != nil {
					return err
				}
				return ErrConflict
			}
			replayed = receipt.Replayed
			return nil
		})
		if errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
			if err = importer.quarantine(ctx, v1profilecatalog.ProfileOptionMappingsTableID, row, "profile_catalog_option_mapping_target_invalid", result); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		result.Imported++
		if replayed {
			result.Replayed++
		}
	}
	return nil
}

func (importer *ProfileCatalogHistoryImporter) importSignupTagRules(ctx context.Context, rows profileCatalogRows, values []v1profilecatalog.SignupTagRuleResult, result *ProfileCatalogHistoryTableResult) error {
	if len(rows.rows) != len(values) {
		return ErrConflict
	}
	for index, decision := range values {
		row := rows.rows[index]
		if rows.redacted[index] {
			if err := importer.quarantine(ctx, v1profilecatalog.SignupTagRulesTableID, row, "profile_catalog_redacted_field", result); err != nil {
				return err
			}
			continue
		}
		if decision.Disposition != v1profilecatalog.DispositionCandidate || decision.Fact == nil {
			if err := importer.quarantine(ctx, v1profilecatalog.SignupTagRulesTableID, row, profileCatalogReason(decision.Reason, "profile_catalog_signup_tag_rule_invalid"), result); err != nil {
				return err
			}
			continue
		}
		fact := *decision.Fact
		replayed := false
		err := importer.uow.Within(ctx, func(tx context.Context) error {
			replayed = false
			receipt, err := importer.writer.ApplySignupTagRule(tx, profileCatalogBinding(row), fact)
			if err != nil {
				return err
			}
			if err = importer.verifySignupTagReceipt(tx, row, receipt); err != nil {
				return err
			}
			actual, err := importer.reader.ReadSignupTagRule(tx, receipt.TargetID)
			if err != nil || !sameSignupTagRuleTarget(actual, receipt.TargetID, row, fact) {
				if err != nil {
					return err
				}
				return ErrConflict
			}
			replayed = receipt.Replayed
			return nil
		})
		if errors.Is(err, contactport.ErrSignupTagHistoryInvalid) {
			if err = importer.quarantine(ctx, v1profilecatalog.SignupTagRulesTableID, row, "profile_catalog_signup_tag_rule_target_invalid", result); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		result.Imported++
		if replayed {
			result.Replayed++
		}
	}
	return nil
}

func (importer *ProfileCatalogHistoryImporter) quarantine(ctx context.Context, table string, row v1archive.ArchivedRow, reason string, result *ProfileCatalogHistoryTableResult) error {
	if reason == "" || result == nil {
		return ErrConflict
	}
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		journal := importer.journals[table]
		terminal, found, err := journal.LoadTerminal(tx, SourceIdentifier(row.SourceKeyHMAC))
		if err != nil {
			return err
		}
		if found {
			if terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "quarantine" || terminal.Reason != reason || terminal.TargetID != "" || terminal.TargetDigest != ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		return journal.Record(tx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
	})
	if err != nil {
		return err
	}
	result.Quarantined++
	if replayed {
		result.Replayed++
	}
	return nil
}

func (importer *ProfileCatalogHistoryImporter) verifyProfileReceipt(ctx context.Context, table string, row v1archive.ArchivedRow, kind string, receipt segmentport.ProfileCatalogHistoryReceipt) error {
	if receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	return importer.verifyTerminal(ctx, table, row, receipt.TargetID, receipt.TargetDigest)
}

func (importer *ProfileCatalogHistoryImporter) verifySignupTagReceipt(ctx context.Context, row v1archive.ArchivedRow, receipt contactport.SignupTagHistoryReceipt) error {
	if receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	return importer.verifyTerminal(ctx, v1profilecatalog.SignupTagRulesTableID, row, receipt.TargetID, receipt.TargetDigest)
}

func (importer *ProfileCatalogHistoryImporter) verifyTerminal(ctx context.Context, table string, row v1archive.ArchivedRow, targetID int64, targetDigest [sha256.Size]byte) error {
	journal := importer.journals[table]
	terminal, found, err := journal.LoadTerminal(ctx, SourceIdentifier(row.SourceKeyHMAC))
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetID != formatProfileCatalogTargetID(targetID) || terminal.TargetDigest != targetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func profileCatalogBinding(row v1archive.ArchivedRow) v1profilecatalog.SourceBinding {
	return v1profilecatalog.SourceBinding{TableID: row.TableID, SourceIdentifier: SourceIdentifier(row.SourceKeyHMAC), SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, FieldDigest: row.FieldHMAC}
}

func profileCatalogReason(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func formatProfileCatalogTargetID(id int64) string {
	if id < 1 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

func sameProfileTemplateTarget(value segmentport.HistoricalProfileTemplate, id int64, row v1archive.ArchivedRow, fact v1profilecatalog.TemplateFact) bool {
	return value.ID == id && value.SourceID == fact.SourceID && value.SourceKeyDigest == row.SourceKeyHMAC && value.SourcePayloadDigest == row.PayloadHMAC
}

func sameProfileCategoryTarget(value segmentport.HistoricalProfileCategory, id int64, row v1archive.ArchivedRow, fact v1profilecatalog.CategoryFact, parent segmentport.HistoricalProfileTemplate) bool {
	return value.ID == id && value.SourceID == fact.SourceID && value.SourceKeyDigest == row.SourceKeyHMAC && value.SourcePayloadDigest == row.PayloadHMAC && value.TemplateSourceID == fact.TemplateSourceID && value.TemplateHistoryID == parent.ID
}

func sameProfileOptionMappingTarget(value segmentport.HistoricalProfileOptionMapping, id int64, row v1archive.ArchivedRow, fact v1profilecatalog.OptionMappingFact, template segmentport.HistoricalProfileTemplate, category segmentport.HistoricalProfileCategory) bool {
	return value.ID == id && value.SourceID == fact.SourceID && value.SourceKeyDigest == row.SourceKeyHMAC && value.SourcePayloadDigest == row.PayloadHMAC && value.TemplateSourceID == fact.TemplateSourceID && value.CategorySourceID == fact.CategorySourceID && value.TemplateHistoryID == template.ID && value.CategoryHistoryID == category.ID
}

func sameSignupTagRuleTarget(value contactport.HistoricalSignupTagRule, id int64, row v1archive.ArchivedRow, fact v1profilecatalog.SignupTagRuleFact) bool {
	return value.ID == id && value.SourceKeyDigest == row.SourceKeyHMAC && value.SourcePayloadDigest == row.PayloadHMAC && value.TagSourceID == fact.TagSourceID
}
