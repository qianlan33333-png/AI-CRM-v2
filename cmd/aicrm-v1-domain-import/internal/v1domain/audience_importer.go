package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audiencehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// AudienceHistoryImporter writes only the eight frozen, non-executable
// Audience history tables. Runtime and derived tables remain encrypted archive
// material and deliberately receive neither a target write nor a receipt here.
type AudienceHistoryImporter struct {
	archive  ArchiveSource
	uow      UnitOfWork
	writer   AudienceHistoryWriter
	resolver AudienceHistoryResolver
	journals map[string]*Journal
	journal  *AudienceHistoryJournal
	actorID  int64
}

func NewAudienceHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer AudienceHistoryWriter, resolver AudienceHistoryResolver, journals map[string]*Journal, actorID int64) (*AudienceHistoryImporter, error) {
	journal, err := NewAudienceHistoryJournal(journals)
	if err != nil || archive == nil || uow == nil || writer == nil || resolver == nil || actorID < 1 {
		return nil, ErrInvalidScope
	}
	copy := make(map[string]*Journal, len(journals))
	for table, value := range journals {
		copy[table] = value
	}
	return &AudienceHistoryImporter{archive: archive, uow: uow, writer: writer, resolver: resolver, journals: copy, journal: journal, actorID: actorID}, nil
}

type audienceRows struct {
	rows     []v1archive.ArchivedRow
	payloads []json.RawMessage
	redacted []bool
}

func (importer *AudienceHistoryImporter) Import(ctx context.Context, archiveRun string) (AudienceHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.resolver == nil || importer.actorID < 1 ||
		!validAudienceHistoryJournals(importer.journals) || archiveRun == "" || archiveRun != importer.journals[v1audiencehistory.PackageGroupsTableID].scope.ArchiveRunID {
		return AudienceHistoryImportResult{}, ErrInvalidScope
	}
	loaded := make(map[string]audienceRows, len(audienceHistoryScopes))
	for _, scope := range audienceHistoryScopes {
		value, err := importer.loadRows(ctx, archiveRun, scope.source)
		if err != nil {
			return AudienceHistoryImportResult{}, err
		}
		loaded[scope.source] = value
	}
	history := v1audiencehistory.AdaptHistoryWithMembers(
		loaded[v1audiencehistory.PackageGroupsTableID].payloads,
		loaded[v1audiencehistory.PackagesTableID].payloads,
		loaded[v1audiencehistory.PackageVersionsTableID].payloads,
		loaded[v1audiencehistory.PackageSendersTableID].payloads,
		loaded[v1audiencehistory.RulesTableID].payloads,
		loaded[v1audiencehistory.RuleVersionsTableID].payloads,
		loaded[v1audiencehistory.SegmentsTableID].payloads,
		loaded[v1audiencehistory.AudienceMembersTableID].payloads,
	)
	result := AudienceHistoryImportResult{}
	groups, packages, rules := map[int64]int64{}, map[int64]int64{}, map[int64]int64{}
	if err := importer.importGroups(ctx, loaded[v1audiencehistory.PackageGroupsTableID], history.PackageGroups, groups, &result); err != nil {
		return AudienceHistoryImportResult{}, err
	}
	if err := importer.importPackages(ctx, loaded[v1audiencehistory.PackagesTableID], history.Packages, groups, packages, &result); err != nil {
		return AudienceHistoryImportResult{}, err
	}
	if err := importer.importVersions(ctx, loaded[v1audiencehistory.PackageVersionsTableID], history.PackageVersions, packages, &result); err != nil {
		return AudienceHistoryImportResult{}, err
	}
	if err := importer.importSenders(ctx, loaded[v1audiencehistory.PackageSendersTableID], history.PackageSenders, packages, &result); err != nil {
		return AudienceHistoryImportResult{}, err
	}
	if err := importer.importRules(ctx, loaded[v1audiencehistory.RulesTableID], history.Rules, rules, &result); err != nil {
		return AudienceHistoryImportResult{}, err
	}
	if err := importer.importRuleVersions(ctx, loaded[v1audiencehistory.RuleVersionsTableID], history.RuleVersions, rules, &result); err != nil {
		return AudienceHistoryImportResult{}, err
	}
	if err := importer.importDefinitions(ctx, loaded[v1audiencehistory.SegmentsTableID], history.Segments, &result); err != nil {
		return AudienceHistoryImportResult{}, err
	}
	if err := importer.importMembers(ctx, loaded[v1audiencehistory.AudienceMembersTableID], history.AudienceMembers, packages, &result); err != nil {
		return AudienceHistoryImportResult{}, err
	}
	return result, nil
}

func (importer *AudienceHistoryImporter) loadRows(ctx context.Context, run, table string) (audienceRows, error) {
	result := audienceRows{}
	seen := map[[sha256.Size]byte]struct{}{}
	var expected int64
	err := importer.archive.EachTableRow(ctx, run, table, func(row v1archive.ArchivedRow) error {
		expected++
		if row.TableID != table || row.AdapterID != v1archive.DefaultAdapterID || row.SourceOrdinal != expected || row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} || row.FieldHMAC == [sha256.Size]byte{} || !json.Valid(row.Payload) {
			return ErrConflict
		}
		if _, exists := seen[row.SourceKeyHMAC]; exists {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		result.rows = append(result.rows, row)
		redacted := audienceRequiredFieldRedacted(table, row)
		// The original encrypted payload and its HMAC remain on row for the
		// quarantine receipt. The candidate parser sees an empty object so a
		// redacted parent also makes all dependent rows non-candidates.
		payload := append(json.RawMessage(nil), row.Payload...)
		if redacted {
			payload = json.RawMessage(`{}`)
		}
		result.payloads = append(result.payloads, payload)
		result.redacted = append(result.redacted, redacted)
		return nil
	})
	return result, err
}

// importCandidate always invokes the owner writer. It deliberately does not
// shortcut on a journal receipt: the writer is the only place that can detect
// historical target drift during a replay.
func (importer *AudienceHistoryImporter) importCandidate(ctx context.Context, kind string, row v1archive.ArchivedRow, write func(context.Context) (segmentport.AudienceHistoryReceipt, error)) (int64, bool, error) {
	targetID, replayed := int64(0), false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		targetID, replayed = 0, false
		receipt, err := write(tx)
		if err != nil {
			return err
		}
		if receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == [sha256.Size]byte{} {
			return ErrConflict
		}
		recorded, found, err := importer.journal.LoadAudienceHistory(tx, kind, receipt.SourceIdentifier)
		if err != nil {
			return err
		}
		if !found || recorded.SourceIdentifier != receipt.SourceIdentifier || recorded.PayloadDigest != receipt.PayloadDigest || recorded.TargetID != receipt.TargetID || recorded.TargetDigest != receipt.TargetDigest {
			return ErrConflict
		}
		targetID, replayed = receipt.TargetID, receipt.Replayed
		return nil
	})
	return targetID, replayed, err
}

func (importer *AudienceHistoryImporter) quarantine(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, ErrConflict
	}
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		existing, found, err := journal.LoadTerminal(tx, SourceIdentifier(row.SourceKeyHMAC))
		if err != nil {
			return err
		}
		if found {
			if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != [sha256.Size]byte{} || len(existing.Metadata) != 0 {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		return journal.Record(tx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
	})
	return replayed, err
}

func (importer *AudienceHistoryImporter) outcome(ctx context.Context, kind string, journal *Journal, row v1archive.ArchivedRow, redacted bool, candidate bool, reason string, sourceID int64, write func(context.Context) (segmentport.AudienceHistoryReceipt, error), result *AudienceHistoryImportResult) (int64, error) {
	if redacted {
		reason, candidate = "audience_required_field_redacted", false
	}
	if !candidate || write == nil {
		if reason == "" {
			return 0, ErrConflict
		}
		replayed, err := importer.quarantine(ctx, journal, row, reason)
		if err == nil {
			result.Quarantined++
			if replayed {
				result.Replayed++
			}
		}
		return 0, err
	}
	if sourceID < 1 {
		return 0, ErrConflict
	}
	target, replayed, err := importer.importCandidate(ctx, kind, row, write)
	if err == nil {
		result.Imported++
		if replayed {
			result.Replayed++
		}
	}
	return target, err
}

func (importer *AudienceHistoryImporter) importGroups(ctx context.Context, data audienceRows, values []v1audiencehistory.PackageGroupResult, targets map[int64]int64, result *AudienceHistoryImportResult) error {
	if len(data.rows) != len(values) {
		return ErrConflict
	}
	journal := importer.journals[v1audiencehistory.PackageGroupsTableID]
	for n, value := range values {
		candidate := value.Disposition == v1audiencehistory.DispositionCandidate && value.Fact != nil
		var sourceID int64
		var write func(context.Context) (segmentport.AudienceHistoryReceipt, error)
		if candidate && !data.redacted[n] {
			fact := *value.Fact
			sourceID = fact.SourceID
			write = func(tx context.Context) (segmentport.AudienceHistoryReceipt, error) {
				return importer.writer.WriteGroup(tx, SourceIdentifier(data.rows[n].SourceKeyHMAC), data.rows[n].PayloadHMAC, segmentport.HistoricalAudienceGroup{SourceID: fact.SourceID, Name: fact.Name, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt})
			}
		}
		target, err := importer.outcome(ctx, "groups", journal, data.rows[n], data.redacted[n], candidate, value.Reason, sourceID, write, result)
		if err != nil {
			return err
		}
		if candidate && !data.redacted[n] {
			if target < 1 {
				return ErrConflict
			}
			if old, ok := targets[sourceID]; ok && old != target {
				return ErrConflict
			}
			targets[sourceID] = target
		}
	}
	return nil
}

func (importer *AudienceHistoryImporter) importPackages(ctx context.Context, data audienceRows, values []v1audiencehistory.PackageResult, groups, targets map[int64]int64, result *AudienceHistoryImportResult) error {
	if len(data.rows) != len(values) {
		return ErrConflict
	}
	journal := importer.journals[v1audiencehistory.PackagesTableID]
	for n, value := range values {
		candidate := value.Disposition == v1audiencehistory.DispositionCandidate && value.Fact != nil
		reason := value.Reason
		var sourceID int64
		var write func(context.Context) (segmentport.AudienceHistoryReceipt, error)
		if candidate && !data.redacted[n] {
			fact := *value.Fact
			sourceID = fact.SourceID
			var groupID *int64
			if fact.GroupSourceID != nil {
				target, ok := groups[*fact.GroupSourceID]
				if !ok {
					candidate, reason = false, "audience_package_group_unresolved"
				} else {
					groupID = &target
				}
			}
			if candidate {
				write = func(tx context.Context) (segmentport.AudienceHistoryReceipt, error) {
					return importer.writer.WritePackage(tx, SourceIdentifier(data.rows[n].SourceKeyHMAC), data.rows[n].PayloadHMAC, segmentport.HistoricalAudiencePackage{SourceID: fact.SourceID, GroupHistoryID: groupID, CurrentVersionSourceID: fact.CurrentVersionSourceID, PackageKey: fact.PackageKey, Name: fact.Name, NaturalLanguageDefinition: fact.NaturalLanguageDefinition, OriginalStatus: fact.OriginalStatus, QueryMode: fact.QueryMode, IdentityPolicy: fact.IdentityPolicy, IncrementalEnabled: fact.IncrementalEnabled, DailyEnabled: fact.DailyEnabled, IncrementalIntervalSecs: fact.IncrementalIntervalSecs, DailyRefreshTime: fact.DailyRefreshTime, Timezone: fact.Timezone, LookbackSecs: fact.LookbackSecs, LastIncrementalAt: fact.LastIncrementalAt, LastDailyRefreshedAt: fact.LastDailyRefreshedAt, NextIncrementalAt: fact.NextIncrementalAt, NextDailyAt: fact.NextDailyAt, PausedReason: fact.PausedReason, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt, RuntimeDigest: [sha256.Size]byte(fact.RuntimeDigest)})
				}
			}
		}
		target, err := importer.outcome(ctx, "packages", journal, data.rows[n], data.redacted[n], candidate, reason, sourceID, write, result)
		if err != nil {
			return err
		}
		if candidate && !data.redacted[n] {
			if target < 1 {
				return ErrConflict
			}
			targets[sourceID] = target
		}
	}
	return nil
}

func (importer *AudienceHistoryImporter) importVersions(ctx context.Context, data audienceRows, values []v1audiencehistory.PackageVersionResult, packages map[int64]int64, result *AudienceHistoryImportResult) error {
	if len(data.rows) != len(values) {
		return ErrConflict
	}
	journal := importer.journals[v1audiencehistory.PackageVersionsTableID]
	for n, value := range values {
		candidate := value.Disposition == v1audiencehistory.DispositionCandidate && value.Fact != nil
		reason := value.Reason
		var sourceID int64
		var write func(context.Context) (segmentport.AudienceHistoryReceipt, error)
		if candidate && !data.redacted[n] {
			fact := *value.Fact
			sourceID = fact.SourceID
			parent, ok := packages[fact.PackageSourceID]
			if !ok {
				candidate, reason = false, "audience_package_version_package_unresolved"
			}
			if candidate {
				write = func(tx context.Context) (segmentport.AudienceHistoryReceipt, error) {
					return importer.writer.WriteVersion(tx, SourceIdentifier(data.rows[n].SourceKeyHMAC), data.rows[n].PayloadHMAC, segmentport.HistoricalAudienceVersion{SourceID: fact.SourceID, PackageHistoryID: parent, VersionNumber: fact.VersionNumber, OriginalStatus: fact.OriginalStatus, AIPrompt: fact.AIPrompt, AIRationale: fact.AIRationale, NaturalLanguageExplanation: fact.NaturalLanguageExplanation, CreatedAt: fact.CreatedAt, PublishedAt: fact.PublishedAt, TemplateKey: fact.TemplateKey, TemplateVersion: fact.TemplateVersion, TemplateFingerprint: fact.TemplateFingerprint, DefinitionDigest: [sha256.Size]byte(fact.DefinitionDigest)})
				}
			}
		}
		if _, err := importer.outcome(ctx, "versions", journal, data.rows[n], data.redacted[n], candidate, reason, sourceID, write, result); err != nil {
			return err
		}
	}
	return nil
}

func (importer *AudienceHistoryImporter) importSenders(ctx context.Context, data audienceRows, values []v1audiencehistory.PackageSenderResult, packages map[int64]int64, result *AudienceHistoryImportResult) error {
	if len(data.rows) != len(values) {
		return ErrConflict
	}
	journal := importer.journals[v1audiencehistory.PackageSendersTableID]
	for n, value := range values {
		candidate := value.Disposition == v1audiencehistory.DispositionCandidate && value.Fact != nil
		reason := value.Reason
		var sourceID int64
		var write func(context.Context) (segmentport.AudienceHistoryReceipt, error)
		if candidate && !data.redacted[n] {
			fact := *value.Fact
			sourceID = fact.SourceID
			parent, ok := packages[fact.PackageSourceID]
			if !ok {
				candidate, reason = false, "audience_package_sender_package_unresolved"
			}
			if candidate {
				write = func(tx context.Context) (segmentport.AudienceHistoryReceipt, error) {
					var staff *int64
					var err error
					if fact.SenderUserID != "" {
						staff, err = importer.resolver.ResolveAudienceHistoryStaff(tx, fact.SenderUserID)
						if err != nil {
							return segmentport.AudienceHistoryReceipt{}, err
						}
					}
					return importer.writer.WriteSender(tx, SourceIdentifier(data.rows[n].SourceKeyHMAC), data.rows[n].PayloadHMAC, segmentport.HistoricalAudienceSender{SourceID: fact.SourceID, PackageHistoryID: parent, StaffID: staff, DisplayName: fact.DisplayName, Priority: fact.Priority, OriginalStatus: fact.OriginalStatus, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt})
				}
			}
		}
		if _, err := importer.outcome(ctx, "senders", journal, data.rows[n], data.redacted[n], candidate, reason, sourceID, write, result); err != nil {
			return err
		}
	}
	return nil
}

func (importer *AudienceHistoryImporter) importRules(ctx context.Context, data audienceRows, values []v1audiencehistory.RuleResult, targets map[int64]int64, result *AudienceHistoryImportResult) error {
	if len(data.rows) != len(values) {
		return ErrConflict
	}
	journal := importer.journals[v1audiencehistory.RulesTableID]
	for n, value := range values {
		candidate := value.Disposition == v1audiencehistory.DispositionCandidate && value.Fact != nil
		var sourceID int64
		var write func(context.Context) (segmentport.AudienceHistoryReceipt, error)
		if candidate && !data.redacted[n] {
			fact := *value.Fact
			sourceID = fact.SourceID
			write = func(tx context.Context) (segmentport.AudienceHistoryReceipt, error) {
				var staff *int64
				var err error
				if fact.SourceOwner != "" {
					staff, err = importer.resolver.ResolveAudienceHistoryStaff(tx, fact.SourceOwner)
					if err != nil {
						return segmentport.AudienceHistoryReceipt{}, err
					}
				}
				return importer.writer.WriteRule(tx, SourceIdentifier(data.rows[n].SourceKeyHMAC), data.rows[n].PayloadHMAC, segmentport.HistoricalAudienceRule{SourceID: fact.SourceID, RuleKey: fact.RuleKey, DisplayName: fact.DisplayName, Description: fact.Description, RuleType: fact.RuleType, OwnerStaffID: staff, OriginalStatus: fact.OriginalStatus, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt})
			}
		}
		target, err := importer.outcome(ctx, "rules", journal, data.rows[n], data.redacted[n], candidate, value.Reason, sourceID, write, result)
		if err != nil {
			return err
		}
		if candidate && !data.redacted[n] {
			if target < 1 {
				return ErrConflict
			}
			targets[sourceID] = target
		}
	}
	return nil
}

func (importer *AudienceHistoryImporter) importRuleVersions(ctx context.Context, data audienceRows, values []v1audiencehistory.RuleVersionResult, rules map[int64]int64, result *AudienceHistoryImportResult) error {
	if len(data.rows) != len(values) {
		return ErrConflict
	}
	journal := importer.journals[v1audiencehistory.RuleVersionsTableID]
	for n, value := range values {
		candidate := value.Disposition == v1audiencehistory.DispositionCandidate && value.Fact != nil
		reason := value.Reason
		var sourceID int64
		var write func(context.Context) (segmentport.AudienceHistoryReceipt, error)
		if candidate && !data.redacted[n] {
			fact := *value.Fact
			sourceID = fact.SourceID
			parent, ok := rules[fact.RuleSourceID]
			if !ok {
				candidate, reason = false, "audience_rule_version_rule_unresolved"
			}
			if candidate {
				write = func(tx context.Context) (segmentport.AudienceHistoryReceipt, error) {
					return importer.writer.WriteRuleVersion(tx, SourceIdentifier(data.rows[n].SourceKeyHMAC), data.rows[n].PayloadHMAC, segmentport.HistoricalAudienceRuleVersion{SourceID: fact.SourceID, RuleHistoryID: parent, Version: fact.Version, ExecutorType: fact.ExecutorType, OriginalStatus: fact.OriginalStatus, PublishedAt: fact.PublishedAt, CreatedAt: fact.CreatedAt, DefinitionDigest: [sha256.Size]byte(fact.DefinitionDigest)})
				}
			}
		}
		if _, err := importer.outcome(ctx, "rule_versions", journal, data.rows[n], data.redacted[n], candidate, reason, sourceID, write, result); err != nil {
			return err
		}
	}
	return nil
}

func (importer *AudienceHistoryImporter) importDefinitions(ctx context.Context, data audienceRows, values []v1audiencehistory.SegmentResult, result *AudienceHistoryImportResult) error {
	if len(data.rows) != len(values) {
		return ErrConflict
	}
	journal := importer.journals[v1audiencehistory.SegmentsTableID]
	for n, value := range values {
		candidate := value.Disposition == v1audiencehistory.DispositionCandidate && value.Fact != nil
		var sourceID int64
		var write func(context.Context) (segmentport.AudienceHistoryReceipt, error)
		if candidate && !data.redacted[n] {
			fact := *value.Fact
			sourceID = fact.SourceID
			write = func(tx context.Context) (segmentport.AudienceHistoryReceipt, error) {
				return importer.writer.WriteDefinition(tx, SourceIdentifier(data.rows[n].SourceKeyHMAC), data.rows[n].PayloadHMAC, segmentport.HistoricalAudienceDefinition{SourceID: fact.SourceID, Code: fact.SegmentCode, DisplayName: fact.DisplayName, Description: fact.Description, SourceType: fact.SourceType, SQLDialect: fact.SQLDialect, OriginalStatus: fact.OriginalStatus, Version: fact.Version, CachedHeadcount: fact.CachedHeadcount, LastRefreshedAt: fact.LastRefreshedAt, UsageCount: fact.UsageCount, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt, DefinitionDigest: [sha256.Size]byte(fact.DefinitionDigest)})
			}
		}
		if _, err := importer.outcome(ctx, "definitions", journal, data.rows[n], data.redacted[n], candidate, value.Reason, sourceID, write, result); err != nil {
			return err
		}
	}
	return nil
}

func (importer *AudienceHistoryImporter) importMembers(ctx context.Context, data audienceRows, values []v1audiencehistory.AudienceMemberResult, packages map[int64]int64, result *AudienceHistoryImportResult) error {
	if len(data.rows) != len(values) {
		return ErrConflict
	}
	journal := importer.journals[v1audiencehistory.AudienceMembersTableID]
	for n, value := range values {
		candidate := value.Disposition == v1audiencehistory.DispositionCandidate && value.Fact != nil
		reason := value.Reason
		var sourceID int64
		var write func(context.Context) (segmentport.AudienceHistoryReceipt, error)
		if candidate && !data.redacted[n] {
			fact := *value.Fact
			sourceID = fact.SourceID
			parent, ok := packages[fact.PackageSourceID]
			if !ok {
				candidate, reason = false, "audience_member_package_unresolved"
			}
			if candidate {
				write = func(tx context.Context) (segmentport.AudienceHistoryReceipt, error) {
					var customer *int64
					var err error
					if fact.UnionID != "" {
						customer, err = importer.resolver.ResolveAudienceHistoryCustomer(tx, fact.UnionID)
						if err != nil {
							return segmentport.AudienceHistoryReceipt{}, err
						}
					}
					return importer.writer.WriteMember(tx, SourceIdentifier(data.rows[n].SourceKeyHMAC), data.rows[n].PayloadHMAC, segmentport.HistoricalAudienceMember{SourceID: fact.SourceID, PackageHistoryID: parent, CustomerID: customer, IdentityKind: fact.IdentityType, OriginalStatus: fact.OriginalStatus, FirstEnteredAt: fact.FirstEnteredAt, LastSeenAt: fact.LastSeenAt, LastUpdatedAt: fact.LastUpdatedAt, ExitedAt: fact.ExitedAt, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt, PayloadDigest: [sha256.Size]byte(fact.PayloadDigest)})
				}
			}
		}
		if _, err := importer.outcome(ctx, "members", journal, data.rows[n], data.redacted[n], candidate, reason, sourceID, write, result); err != nil {
			return err
		}
	}
	return nil
}

func audienceRequiredFieldRedacted(table string, row v1archive.ArchivedRow) bool {
	required := map[string][]string{
		v1audiencehistory.PackageGroupsTableID:   {"id", "name", "created_at", "updated_at"},
		v1audiencehistory.PackagesTableID:        {"id", "package_key", "name", "natural_language_definition", "status", "query_mode", "identity_policy", "current_version_id", "incremental_enabled", "daily_enabled", "incremental_interval_seconds", "daily_refresh_time", "timezone", "lookback_seconds", "last_incremental_watermark_at", "last_daily_refreshed_at", "next_incremental_refresh_at", "next_daily_refresh_at", "paused_reason", "created_at", "updated_at", "group_id"},
		v1audiencehistory.PackageVersionsTableID: {"id", "package_id", "version_number", "status", "ai_prompt", "ai_rationale", "natural_language_explanation", "created_at", "published_at", "template_key", "template_version", "template_fingerprint"},
		v1audiencehistory.PackageSendersTableID:  {"id", "package_id", "sender_userid", "display_name", "priority", "status", "created_at", "updated_at"},
		v1audiencehistory.RulesTableID:           {"id", "rule_key", "display_name", "description", "rule_type", "owner", "status", "created_at", "updated_at"},
		v1audiencehistory.RuleVersionsTableID:    {"id", "rule_id", "version", "executor_type", "status", "published_at", "created_at"},
		v1audiencehistory.SegmentsTableID:        {"id", "segment_code", "display_name", "description", "source_type", "sql_dialect", "status", "version", "created_by_agent", "created_by_session", "cached_headcount", "last_refreshed_at", "last_refresh_error", "usage_count", "created_at", "updated_at"},
		v1audiencehistory.AudienceMembersTableID: {"id", "package_id", "identity_type", "identity_value", "status", "mobile_hash", "owner_userid", "event_source_key", "payload_hash", "first_entered_at", "last_seen_at", "last_updated_at", "exited_at", "created_at", "updated_at", "unionid"},
	}
	for _, field := range required[table] {
		if v1archive.IsRedacted(row, field) {
			return true
		}
	}
	return false
}
