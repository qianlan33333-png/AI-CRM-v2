// Package v1contacthistory parses V1 Contact history without assigning it to
// current V2 customers, staff, owner reassignment commands, or Provider state.
package v1contacthistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	SidebarProfileFieldsTableID   = "public/sidebar_customer_profile_fields"
	OwnerMigrationResultsTableID  = "public/owner_migration_results"
	OwnerMigrationSessionsTableID = "public/owner_migration_import_sessions"
	OwnerMigrationPreviewsTableID = "public/owner_migration_previews"
)

var ErrInvalidArchiveRow = errors.New("invalid V1 contact history archive row")

type Disposition string

const (
	DispositionCandidate  Disposition = "candidate"
	DispositionQuarantine Disposition = "quarantine"
)

const (
	ReasonInvalidArchiveRow       = "invalid_archive_row"
	ReasonRetainedFieldRedacted   = "retained_field_redacted"
	ReasonInvalidSourcePayload    = "invalid_source_payload"
	ReasonInvalidSidebarHistory   = "invalid_sidebar_history"
	ReasonInvalidOwnerResult      = "invalid_owner_migration_result"
	ReasonOwnerRelationMissing    = "owner_migration_relation_missing"
	ReasonOwnerRelationMismatched = "owner_migration_relation_mismatched"
)

const (
	OwnerSessionRelationResolved   = "resolved"
	OwnerSessionRelationUnresolved = "unresolved"
	OwnerPreviewRelationResolved   = "resolved"
	OwnerPreviewRelationUnresolved = "unresolved"
)

// ArchiveReader is deliberately read-only. It is satisfied by the V2 archive
// reader and intentionally has no target database or writer methods.
type ArchiveReader interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

type Decision[T any] struct {
	Disposition Disposition
	Reason      string
	Candidate   *T
}

// SidebarProfileHistory is a V1 snapshot. Its fields are not a V2 customer
// schema and UnionID is only retained for a later verified identity resolver.
type SidebarProfileHistory struct {
	Source                string            `json:"source"`
	Industry              string            `json:"industry"`
	IndustryDescription   string            `json:"industry_description"`
	NeedsBlockersFollowup string            `json:"needs_blockers_followup"`
	UpdatedBy             string            `json:"updated_by"`
	UpdatedAt             time.Time         `json:"updated_at"`
	UnionID               string            `json:"unionid"`
	SourceKeyHMAC         [sha256.Size]byte `json:"-"`
	SourcePayload         json.RawMessage   `json:"-"`
}

// OwnerMigrationResultHistory preserves the historical result only. It never
// retains a V1 preview token; WeCom counters are V1 evidence, not V2 Provider
// facts.
type OwnerMigrationResultHistory struct {
	ResultID               string            `json:"result_id"`
	JobID                  string            `json:"job_id"`
	ScopeType              string            `json:"scope_type"`
	SessionID              string            `json:"session_id"`
	FileHash               string            `json:"file_hash"`
	SourceOwnerUserID      string            `json:"source_owner_userid"`
	TargetOwnerUserID      string            `json:"target_owner_userid"`
	SourceOwnerDisplayName string            `json:"source_owner_display_name"`
	TargetOwnerDisplayName string            `json:"target_owner_display_name"`
	Operator               string            `json:"operator"`
	PreviewHash            string            `json:"preview_hash"`
	TotalRows              int               `json:"total_rows"`
	EligibleCount          int               `json:"eligible_count"`
	WeComSuccess           int               `json:"wecom_success"`
	WeComFailed            int               `json:"wecom_failed"`
	CRMUpdated             int               `json:"crm_updated"`
	IncludeWeComTransfer   bool              `json:"include_wecom_transfer"`
	TransferWelcomeMessage string            `json:"transfer_welcome_msg"`
	RowsJSON               json.RawMessage   `json:"rows_json"`
	StatsJSON              json.RawMessage   `json:"stats_json"`
	CreatedAt              time.Time         `json:"created_at"`
	ExecutedAt             time.Time         `json:"executed_at"`
	PreviewRelation        string            `json:"-"`
	SourceKeyHMAC          [sha256.Size]byte `json:"-"`
	SourcePayload          json.RawMessage   `json:"-"`
}

type ownerMigrationSession struct {
	SessionID              string            `json:"session_id"`
	FileName               string            `json:"file_name"`
	FileHash               string            `json:"file_hash"`
	SourceOwnerID          string            `json:"source_owner_userid"`
	TargetOwnerID          string            `json:"target_owner_userid"`
	IncludeWeComTransfer   bool              `json:"include_wecom_transfer"`
	TransferWelcomeMessage string            `json:"transfer_welcome_msg"`
	RowsJSON               json.RawMessage   `json:"rows_json"`
	RowStatsJSON           json.RawMessage   `json:"row_stats_json"`
	Operator               string            `json:"operator"`
	CreatedAt              time.Time         `json:"created_at"`
	SourceKeyHMAC          [sha256.Size]byte `json:"-"`
	SourcePayload          json.RawMessage   `json:"-"`
}

type ownerMigrationPreview struct {
	PreviewHash               string            `json:"preview_hash"`
	ScopeType                 string            `json:"scope_type"`
	SessionID                 string            `json:"session_id"`
	FileHash                  string            `json:"file_hash"`
	SourceOwnerID             string            `json:"source_owner_userid"`
	TargetOwnerID             string            `json:"target_owner_userid"`
	SourceOwnerDisplayName    string            `json:"source_owner_display_name"`
	TargetOwnerDisplayName    string            `json:"target_owner_display_name"`
	IncludeWeComTransfer      bool              `json:"include_wecom_transfer"`
	TransferWelcomeMessage    string            `json:"transfer_welcome_msg"`
	EligibleExternalUsersJSON json.RawMessage   `json:"eligible_external_userids_json"`
	RowsJSON                  json.RawMessage   `json:"rows_json"`
	RowStatsJSON              json.RawMessage   `json:"row_stats_json"`
	SurfaceCountsJSON         json.RawMessage   `json:"surface_counts_json"`
	PendingReviewJSON         json.RawMessage   `json:"pending_review_json"`
	Operator                  string            `json:"operator"`
	CreatedAt                 time.Time         `json:"created_at"`
	ExpiresAt                 time.Time         `json:"expires_at"`
	ExecutedResultID          string            `json:"executed_result_id"`
	SourceKeyHMAC             [sha256.Size]byte `json:"-"`
	SourcePayload             json.RawMessage   `json:"-"`
}

type parsedOwnerMigrationPreviews struct {
	values             []ownerMigrationPreview
	byExecutedResultID map[string]ownerMigrationPreview
}

// OwnerMigrationContext is a validated, batch-local view of the two V1 owner
// context tables. It contains no token or command material and must not be
// retained beyond the historical import batch.
type OwnerMigrationContext struct {
	sessions map[string]ownerMigrationSession
	previews map[string]ownerMigrationPreview
}

type OwnerMigrationRelations struct {
	SessionRelation string
	PreviewRelation string
}

// BuildOwnerMigrationContext verifies the source shape and source ordering
// before exposing any owner-result relation. Empty V1 session IDs are allowed
// historical facts but are always unresolved.
func BuildOwnerMigrationContext(sessionRows, previewRows []v1archive.ArchivedRow) (OwnerMigrationContext, error) {
	if !validRows(sessionRows, OwnerMigrationSessionsTableID) || !validRows(previewRows, OwnerMigrationPreviewsTableID) {
		return OwnerMigrationContext{}, ErrInvalidArchiveRow
	}
	sessions, err := parseSessions(sessionRows)
	if err != nil {
		return OwnerMigrationContext{}, err
	}
	previews, err := parsePreviews(previewRows)
	if err != nil {
		return OwnerMigrationContext{}, err
	}
	return OwnerMigrationContext{sessions: sessions, previews: previews.byExecutedResultID}, nil
}

// SessionRelation returns both persisted relation facts and a fixed quarantine
// reason when a non-empty result session cannot be proven against this batch.
func (context OwnerMigrationContext) SessionRelation(result OwnerMigrationResultHistory) (OwnerMigrationRelations, string) {
	relations := OwnerMigrationRelations{SessionRelation: OwnerSessionRelationUnresolved, PreviewRelation: OwnerPreviewRelationUnresolved}
	if result.SessionID == "" {
		return relations, ""
	}
	if _, found := context.sessions[result.SessionID]; !found {
		return OwnerMigrationRelations{}, ReasonOwnerRelationMissing
	}
	relations.SessionRelation = OwnerSessionRelationResolved
	preview, found := context.previews[result.ResultID]
	if !found || preview.SessionID == "" {
		return relations, ""
	}
	if preview.SessionID != result.SessionID {
		return OwnerMigrationRelations{}, ReasonOwnerRelationMismatched
	}
	relations.PreviewRelation = OwnerPreviewRelationResolved
	return relations, ""
}

type PreflightReport struct {
	SidebarRows              int
	OwnerResultRows          int
	OwnerSessionRows         int
	OwnerPreviewRows         int
	SidebarCandidates        int
	OwnerResultCandidates    int
	PreviewSessionResolved   int
	PreviewSessionUnresolved int
	ResultPreviewResolved    int
	ResultPreviewUnresolved  int
	Quarantined              int
	Reasons                  map[string]int
}

// SortedReasons is safe to log: it contains only reason names and counts.
func (report PreflightReport) SortedReasons() []string {
	keys := make([]string, 0, len(report.Reasons))
	for reason := range report.Reasons {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	return keys
}

func AdaptSidebarProfile(row v1archive.ArchivedRow) Decision[SidebarProfileHistory] {
	if !validRow(row, SidebarProfileFieldsTableID) {
		return quarantine[SidebarProfileHistory](ReasonInvalidArchiveRow)
	}
	if len(row.RedactedFields) != 0 {
		return quarantine[SidebarProfileHistory](ReasonRetainedFieldRedacted)
	}
	var source SidebarProfileHistory
	if err := json.Unmarshal(row.Payload, &source); err != nil {
		return quarantine[SidebarProfileHistory](ReasonInvalidSourcePayload)
	}
	if !validIdentifier(source.UnionID) || source.UpdatedAt.IsZero() {
		return quarantine[SidebarProfileHistory](ReasonInvalidSidebarHistory)
	}
	source.SourceKeyHMAC = row.SourceKeyHMAC
	source.SourcePayload = cloneJSON(row.Payload)
	return candidate(source)
}

func AdaptOwnerMigrationResult(row v1archive.ArchivedRow) Decision[OwnerMigrationResultHistory] {
	if !validRow(row, OwnerMigrationResultsTableID) {
		return quarantine[OwnerMigrationResultHistory](ReasonInvalidArchiveRow)
	}
	if !hasExactRedactions(row, "preview_token", "stats_json.preview_token") {
		return quarantine[OwnerMigrationResultHistory](ReasonRetainedFieldRedacted)
	}
	var source OwnerMigrationResultHistory
	if err := json.Unmarshal(row.Payload, &source); err != nil {
		return quarantine[OwnerMigrationResultHistory](ReasonInvalidSourcePayload)
	}
	if !validOwnerResult(source) {
		return quarantine[OwnerMigrationResultHistory](ReasonInvalidOwnerResult)
	}
	source.RowsJSON = cloneJSON(source.RowsJSON)
	source.StatsJSON = cloneJSON(source.StatsJSON)
	source.PreviewRelation = OwnerPreviewRelationUnresolved
	source.SourceKeyHMAC = row.SourceKeyHMAC
	source.SourcePayload = cloneJSON(row.Payload)
	return candidate(source)
}

// Preflight only reads a reconciled V2 archive and produces aggregate counts.
// It does not resolve identities or invoke a current owner migration service.
func Preflight(ctx context.Context, archive ArchiveReader, runID string) (PreflightReport, error) {
	if archive == nil || strings.TrimSpace(runID) == "" {
		return PreflightReport{}, ErrInvalidArchiveRow
	}
	sidebarRows, err := readRows(ctx, archive, runID, SidebarProfileFieldsTableID)
	if err != nil {
		return PreflightReport{}, err
	}
	resultRows, err := readRows(ctx, archive, runID, OwnerMigrationResultsTableID)
	if err != nil {
		return PreflightReport{}, err
	}
	sessionRows, err := readRows(ctx, archive, runID, OwnerMigrationSessionsTableID)
	if err != nil {
		return PreflightReport{}, err
	}
	previewRows, err := readRows(ctx, archive, runID, OwnerMigrationPreviewsTableID)
	if err != nil {
		return PreflightReport{}, err
	}

	sessions, err := parseSessions(sessionRows)
	if err != nil {
		return PreflightReport{}, err
	}
	previews, err := parsePreviews(previewRows)
	if err != nil {
		return PreflightReport{}, err
	}
	report := PreflightReport{
		SidebarRows:      len(sidebarRows),
		OwnerResultRows:  len(resultRows),
		OwnerSessionRows: len(sessionRows),
		OwnerPreviewRows: len(previewRows),
		Reasons:          map[string]int{},
	}
	for _, row := range sidebarRows {
		decision := AdaptSidebarProfile(row)
		if decision.Disposition == DispositionCandidate {
			report.SidebarCandidates++
			continue
		}
		report.record(decision.Reason)
	}
	for _, preview := range previews.values {
		// V1 stored historical previews with an allowed empty session_id. It is
		// provenance only and never selects or creates a V2 session.
		if preview.SessionID == "" {
			report.PreviewSessionUnresolved++
		} else if _, found := sessions[preview.SessionID]; found {
			report.PreviewSessionResolved++
		} else {
			report.PreviewSessionUnresolved++
		}
	}
	for _, row := range resultRows {
		decision := AdaptOwnerMigrationResult(row)
		if decision.Disposition == DispositionCandidate {
			relation, reason := validateOwnerRelation(*decision.Candidate, sessions, previews.byExecutedResultID)
			if reason == "" {
				decision.Candidate.PreviewRelation = relation
				report.OwnerResultCandidates++
				if relation == OwnerPreviewRelationResolved {
					report.ResultPreviewResolved++
				} else {
					report.ResultPreviewUnresolved++
				}
				continue
			} else {
				report.record(reason)
				continue
			}
		}
		report.record(decision.Reason)
	}
	return report, nil
}

func (report *PreflightReport) record(reason string) {
	report.Quarantined++
	report.Reasons[reason]++
}

func readRows(ctx context.Context, archive ArchiveReader, runID, tableID string) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0)
	var ordinal int64
	err := archive.EachTableRow(ctx, runID, tableID, func(row v1archive.ArchivedRow) error {
		ordinal++
		if !validRow(row, tableID) || row.SourceOrdinal != ordinal {
			return ErrInvalidArchiveRow
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func validRows(rows []v1archive.ArchivedRow, tableID string) bool {
	for index, row := range rows {
		if !validRow(row, tableID) || row.SourceOrdinal != int64(index+1) {
			return false
		}
	}
	return true
}

func parseSessions(rows []v1archive.ArchivedRow) (map[string]ownerMigrationSession, error) {
	values := make(map[string]ownerMigrationSession, len(rows))
	for _, row := range rows {
		if len(row.RedactedFields) != 0 {
			return nil, ErrInvalidArchiveRow
		}
		var value ownerMigrationSession
		if err := json.Unmarshal(row.Payload, &value); err != nil || !validIdentifier(value.SessionID) || value.CreatedAt.IsZero() ||
			!validJSON(value.RowsJSON) || !validJSON(value.RowStatsJSON) {
			return nil, ErrInvalidArchiveRow
		}
		if _, found := values[value.SessionID]; found {
			return nil, ErrInvalidArchiveRow
		}
		value.RowsJSON = cloneJSON(value.RowsJSON)
		value.RowStatsJSON = cloneJSON(value.RowStatsJSON)
		value.SourceKeyHMAC = row.SourceKeyHMAC
		value.SourcePayload = cloneJSON(row.Payload)
		values[value.SessionID] = value
	}
	return values, nil
}

func parsePreviews(rows []v1archive.ArchivedRow) (parsedOwnerMigrationPreviews, error) {
	values := parsedOwnerMigrationPreviews{
		values:             make([]ownerMigrationPreview, 0, len(rows)),
		byExecutedResultID: make(map[string]ownerMigrationPreview, len(rows)),
	}
	for _, row := range rows {
		if !hasExactRedactions(row, "preview_token") {
			return parsedOwnerMigrationPreviews{}, ErrInvalidArchiveRow
		}
		var value ownerMigrationPreview
		if err := json.Unmarshal(row.Payload, &value); err != nil ||
			!validOptionalIdentifier(value.SessionID) || value.CreatedAt.IsZero() || value.ExpiresAt.IsZero() ||
			value.ExpiresAt.Before(value.CreatedAt) || !validJSON(value.EligibleExternalUsersJSON) || !validJSON(value.RowsJSON) ||
			!validJSON(value.RowStatsJSON) || !validJSON(value.SurfaceCountsJSON) || !validJSON(value.PendingReviewJSON) ||
			!validOptionalIdentifier(value.ExecutedResultID) {
			return parsedOwnerMigrationPreviews{}, ErrInvalidArchiveRow
		}
		value.EligibleExternalUsersJSON = cloneJSON(value.EligibleExternalUsersJSON)
		value.RowsJSON = cloneJSON(value.RowsJSON)
		value.RowStatsJSON = cloneJSON(value.RowStatsJSON)
		value.SurfaceCountsJSON = cloneJSON(value.SurfaceCountsJSON)
		value.PendingReviewJSON = cloneJSON(value.PendingReviewJSON)
		value.SourceKeyHMAC = row.SourceKeyHMAC
		value.SourcePayload = cloneJSON(row.Payload)
		values.values = append(values.values, value)
		if value.ExecutedResultID == "" {
			continue
		}
		if _, found := values.byExecutedResultID[value.ExecutedResultID]; found {
			return parsedOwnerMigrationPreviews{}, ErrInvalidArchiveRow
		}
		values.byExecutedResultID[value.ExecutedResultID] = value
	}
	return values, nil
}

func validateOwnerRelation(result OwnerMigrationResultHistory, sessions map[string]ownerMigrationSession, previews map[string]ownerMigrationPreview) (string, string) {
	// V1 owner_migration_results.session_id was TEXT NOT NULL DEFAULT ''. An
	// empty historical value has no safe session relation, even if a preview
	// later names this result; do not infer one.
	if result.SessionID == "" {
		return OwnerPreviewRelationUnresolved, ""
	}
	if _, found := sessions[result.SessionID]; !found {
		return "", ReasonOwnerRelationMissing
	}
	preview, found := previews[result.ResultID]
	if !found {
		return OwnerPreviewRelationUnresolved, ""
	}
	if preview.SessionID == "" {
		return OwnerPreviewRelationUnresolved, ""
	}
	if preview.SessionID != result.SessionID {
		return "", ReasonOwnerRelationMismatched
	}
	return OwnerPreviewRelationResolved, ""
}

func validRow(row v1archive.ArchivedRow, tableID string) bool {
	zero := [sha256.Size]byte{}
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == tableID && row.SourceOrdinal > 0 &&
		row.SourceKeyHMAC != zero && row.PayloadHMAC != zero && row.FieldHMAC != zero && json.Valid(row.Payload)
}

func validOwnerResult(value OwnerMigrationResultHistory) bool {
	return validIdentifier(value.ResultID) && validOptionalIdentifier(value.SessionID) &&
		!value.CreatedAt.IsZero() && !value.ExecutedAt.IsZero() && !value.ExecutedAt.Before(value.CreatedAt) &&
		value.TotalRows >= 0 && value.EligibleCount >= 0 && value.WeComSuccess >= 0 && value.WeComFailed >= 0 && value.CRMUpdated >= 0 &&
		validJSON(value.RowsJSON) && validJSON(value.StatsJSON)
}

func validIdentifier(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func validOptionalIdentifier(value string) bool {
	return value == "" || validIdentifier(value)
}

func hasExactRedactions(row v1archive.ArchivedRow, allowed ...string) bool {
	if len(row.RedactedFields) != len(allowed) {
		return false
	}
	seen := make(map[string]struct{}, len(allowed))
	for _, field := range row.RedactedFields {
		seen[field] = struct{}{}
	}
	if len(seen) != len(allowed) {
		return false
	}
	for _, field := range allowed {
		if _, found := seen[field]; !found {
			return false
		}
	}
	return true
}

func validJSON(value json.RawMessage) bool {
	return len(value) != 0 && json.Valid(value)
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func candidate[T any](value T) Decision[T] {
	return Decision[T]{Disposition: DispositionCandidate, Candidate: &value}
}

func quarantine[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: DispositionQuarantine, Reason: reason}
}
