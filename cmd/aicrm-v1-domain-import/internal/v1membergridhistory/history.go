// Package v1membergridhistory parses V1 Member Grid material as private,
// read-only history. It has no V2 writer, entitlement, sharing, or permission
// dependency.
package v1membergridhistory

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
	MemberViewsTableID         = "public/service_period_member_views"
	UsageSnapshotsTableID      = "public/service_period_huangyoucan_usage_snapshot"
	UsageSyncRunsTableID       = "public/service_period_huangyoucan_usage_sync_runs"
	MemberCollaboratorsTableID = "public/service_period_member_collaborators"
	MemberSharesTableID        = "public/service_period_member_shares"
)

var ErrInvalidArchiveRow = errors.New("invalid V1 member grid history archive row")

type Disposition string

const (
	DispositionCandidate  Disposition = "candidate"
	DispositionArchive    Disposition = "archive"
	DispositionQuarantine Disposition = "quarantine"
)

const (
	ReasonInvalidArchiveRow        = "invalid_archive_row"
	ReasonRetainedFieldRedacted    = "retained_field_redacted"
	ReasonInvalidSourcePayload     = "invalid_source_payload"
	ReasonInvalidMemberView        = "invalid_member_view"
	ReasonInvalidUsageSnapshot     = "invalid_usage_snapshot"
	ReasonInvalidUsageSyncRun      = "invalid_usage_sync_run"
	ReasonInvalidCollaborator      = "invalid_member_collaborator"
	ReasonInvalidShare             = "invalid_member_share"
	ReasonLegacyPermissionArchive  = "legacy_member_permission_archive_only"
	ReasonLegacyPublicShareArchive = "legacy_public_share_archive_only"
	ReasonLegacyUsageSyncArchive   = "legacy_usage_sync_archive_only"
	ReasonSchemaCrosswalkRequired  = "member_view_schema_crosswalk_required"
)

// ArchiveReader is deliberately read-only and is satisfied by the immutable
// V2 archive reader. It cannot call a V2 domain writer.
type ArchiveReader interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

type Decision[T any] struct {
	Disposition Disposition
	Reason      string
	Record      *T
}

// MemberViewHistory retains the full V1 config as private source input. It is
// never evidence that the config is supported by current V2 Member Grid.
type MemberViewHistory struct {
	ID                       int64           `json:"id"`
	TenantID                 string          `json:"tenant_id"`
	ServiceProductID         int64           `json:"service_product_id"`
	Name                     string          `json:"name"`
	Position                 int             `json:"position"`
	IsDefault                bool            `json:"is_default"`
	SchemaVersion            int16           `json:"schema_version"`
	ConfigJSON               json.RawMessage `json:"config_json"`
	Version                  int             `json:"version"`
	CreatedBy                string          `json:"created_by"`
	UpdatedBy                string          `json:"updated_by"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
	RequiresProductCrosswalk bool            `json:"-"`
	RequiresSchemaCrosswalk  bool            `json:"-"`
	SourcePayload            json.RawMessage `json:"-"`
}

// UsageSnapshotHistory is frozen V1 usage. It deliberately contains no V2
// product, entitlement, customer, or live-login assertion.
type UsageSnapshotHistory struct {
	HuangYouCanUserID   string          `json:"huangyoucan_user_id"`
	UnionID             string          `json:"unionid"`
	MobileMD5           string          `json:"mobile_md5"`
	FormallyLoggedIn    bool            `json:"formally_logged_in"`
	HasTokenUsage       bool            `json:"has_token_usage"`
	LearningPlanID      string          `json:"learning_plan_id"`
	LearningPlanCurrent *int            `json:"learning_plan_current"`
	LearningPlanTotal   *int            `json:"learning_plan_total"`
	OpenCount7D         int             `json:"open_count_7d"`
	LastOpenAt          *time.Time      `json:"last_open_at"`
	RefreshedAt         time.Time       `json:"refreshed_at"`
	SourcePayload       json.RawMessage `json:"-"`
}

type UsageSyncRunHistory struct {
	ID               int64           `json:"id"`
	TriggerSource    string          `json:"trigger_source"`
	Status           string          `json:"status"`
	SourceRowCount   int             `json:"source_row_count"`
	SnapshotRowCount int             `json:"snapshot_row_count"`
	StartedAt        time.Time       `json:"started_at"`
	FinishedAt       time.Time       `json:"finished_at"`
	ErrorSummary     string          `json:"error_summary"`
	SourcePayload    json.RawMessage `json:"-"`
}

// MemberCollaboratorHistory is retained only as historical source material;
// it can never re-create V2 staff or an authorization grant.
type MemberCollaboratorHistory struct {
	ID               int64           `json:"id"`
	TenantID         string          `json:"tenant_id"`
	ServiceProductID int64           `json:"service_product_id"`
	AdminUserID      int64           `json:"admin_user_id"`
	WeComUserID      string          `json:"wecom_userid"`
	DisplayName      string          `json:"display_name"`
	AvatarURL        string          `json:"avatar_url"`
	Permission       string          `json:"permission"`
	Version          int             `json:"version"`
	CreatedBy        string          `json:"created_by"`
	UpdatedBy        string          `json:"updated_by"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	SourcePayload    json.RawMessage `json:"-"`
}

// MemberShareHistory retains an old public identifier only inside private
// source material. It can never restore a V2 share URL or access grant.
type MemberShareHistory struct {
	ID               int64           `json:"id"`
	TenantID         string          `json:"tenant_id"`
	ServiceProductID int64           `json:"service_product_id"`
	Enabled          bool            `json:"enabled"`
	PublicID         string          `json:"public_id"`
	Generation       int             `json:"generation"`
	Version          int             `json:"version"`
	CreatedBy        string          `json:"created_by"`
	UpdatedBy        string          `json:"updated_by"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	SourcePayload    json.RawMessage `json:"-"`
}

type PreflightReport struct {
	MemberViewRows    int
	UsageSnapshotRows int
	UsageSyncRunRows  int
	CollaboratorRows  int
	ShareRows         int
	Candidates        int
	Archived          int
	Quarantined       int
	Reasons           map[string]int
}

// SortedReasons is safe to log because it contains only aggregate counts.
func (report PreflightReport) SortedReasons() []string {
	keys := make([]string, 0, len(report.Reasons))
	for reason := range report.Reasons {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	return keys
}

func AdaptMemberView(row v1archive.ArchivedRow) Decision[MemberViewHistory] {
	if !validRow(row, MemberViewsTableID) {
		return quarantine[MemberViewHistory](ReasonInvalidArchiveRow)
	}
	if len(row.RedactedFields) != 0 {
		return quarantine[MemberViewHistory](ReasonRetainedFieldRedacted)
	}
	var value MemberViewHistory
	if err := json.Unmarshal(row.Payload, &value); err != nil {
		return quarantine[MemberViewHistory](ReasonInvalidSourcePayload)
	}
	if value.ID < 1 || value.ServiceProductID < 1 || !validJSON(value.ConfigJSON) || !validTimes(value.CreatedAt, value.UpdatedAt) {
		return quarantine[MemberViewHistory](ReasonInvalidMemberView)
	}
	value.ConfigJSON = cloneJSON(value.ConfigJSON)
	value.SourcePayload = cloneJSON(row.Payload)
	value.RequiresProductCrosswalk = true
	value.RequiresSchemaCrosswalk = true
	return Decision[MemberViewHistory]{Disposition: DispositionCandidate, Reason: ReasonSchemaCrosswalkRequired, Record: &value}
}

func AdaptUsageSnapshot(row v1archive.ArchivedRow) Decision[UsageSnapshotHistory] {
	if !validRow(row, UsageSnapshotsTableID) {
		return quarantine[UsageSnapshotHistory](ReasonInvalidArchiveRow)
	}
	if len(row.RedactedFields) != 0 {
		return quarantine[UsageSnapshotHistory](ReasonRetainedFieldRedacted)
	}
	var value UsageSnapshotHistory
	if err := json.Unmarshal(row.Payload, &value); err != nil {
		return quarantine[UsageSnapshotHistory](ReasonInvalidSourcePayload)
	}
	if value.RefreshedAt.IsZero() || value.OpenCount7D < 0 || invalidNullableCount(value.LearningPlanCurrent) || invalidNullableCount(value.LearningPlanTotal) {
		return quarantine[UsageSnapshotHistory](ReasonInvalidUsageSnapshot)
	}
	value.SourcePayload = cloneJSON(row.Payload)
	return candidate(value)
}

func AdaptUsageSyncRun(row v1archive.ArchivedRow) Decision[UsageSyncRunHistory] {
	if !validRow(row, UsageSyncRunsTableID) {
		return quarantine[UsageSyncRunHistory](ReasonInvalidArchiveRow)
	}
	if len(row.RedactedFields) != 0 {
		return quarantine[UsageSyncRunHistory](ReasonRetainedFieldRedacted)
	}
	var value UsageSyncRunHistory
	if err := json.Unmarshal(row.Payload, &value); err != nil {
		return quarantine[UsageSyncRunHistory](ReasonInvalidSourcePayload)
	}
	if value.ID < 1 || value.SourceRowCount < 0 || value.SnapshotRowCount < 0 || !validTimes(value.StartedAt, value.FinishedAt) {
		return quarantine[UsageSyncRunHistory](ReasonInvalidUsageSyncRun)
	}
	value.SourcePayload = cloneJSON(row.Payload)
	return archived(value, ReasonLegacyUsageSyncArchive)
}

func AdaptMemberCollaborator(row v1archive.ArchivedRow) Decision[MemberCollaboratorHistory] {
	if !validRow(row, MemberCollaboratorsTableID) {
		return quarantine[MemberCollaboratorHistory](ReasonInvalidArchiveRow)
	}
	if len(row.RedactedFields) != 0 {
		return quarantine[MemberCollaboratorHistory](ReasonRetainedFieldRedacted)
	}
	var value MemberCollaboratorHistory
	if err := json.Unmarshal(row.Payload, &value); err != nil {
		return quarantine[MemberCollaboratorHistory](ReasonInvalidSourcePayload)
	}
	if value.ID < 1 || value.ServiceProductID < 1 || !validTimes(value.CreatedAt, value.UpdatedAt) {
		return quarantine[MemberCollaboratorHistory](ReasonInvalidCollaborator)
	}
	value.SourcePayload = cloneJSON(row.Payload)
	return archived(value, ReasonLegacyPermissionArchive)
}

func AdaptMemberShare(row v1archive.ArchivedRow) Decision[MemberShareHistory] {
	if !validRow(row, MemberSharesTableID) {
		return quarantine[MemberShareHistory](ReasonInvalidArchiveRow)
	}
	if len(row.RedactedFields) != 0 {
		return quarantine[MemberShareHistory](ReasonRetainedFieldRedacted)
	}
	var value MemberShareHistory
	if err := json.Unmarshal(row.Payload, &value); err != nil {
		return quarantine[MemberShareHistory](ReasonInvalidSourcePayload)
	}
	if value.ID < 1 || value.ServiceProductID < 1 || !validTimes(value.CreatedAt, value.UpdatedAt) {
		return quarantine[MemberShareHistory](ReasonInvalidShare)
	}
	value.SourcePayload = cloneJSON(row.Payload)
	return archived(value, ReasonLegacyPublicShareArchive)
}

// Preflight reads the reconciled V2 archive and emits aggregate classifications
// only. It neither resolves references nor writes a V2 target.
func Preflight(ctx context.Context, archive ArchiveReader, runID string) (PreflightReport, error) {
	if archive == nil || strings.TrimSpace(runID) == "" {
		return PreflightReport{}, ErrInvalidArchiveRow
	}
	views, err := readRows(ctx, archive, runID, MemberViewsTableID)
	if err != nil {
		return PreflightReport{}, err
	}
	usage, err := readRows(ctx, archive, runID, UsageSnapshotsTableID)
	if err != nil {
		return PreflightReport{}, err
	}
	syncRuns, err := readRows(ctx, archive, runID, UsageSyncRunsTableID)
	if err != nil {
		return PreflightReport{}, err
	}
	collaborators, err := readRows(ctx, archive, runID, MemberCollaboratorsTableID)
	if err != nil {
		return PreflightReport{}, err
	}
	shares, err := readRows(ctx, archive, runID, MemberSharesTableID)
	if err != nil {
		return PreflightReport{}, err
	}
	report := PreflightReport{
		MemberViewRows: len(views), UsageSnapshotRows: len(usage), UsageSyncRunRows: len(syncRuns),
		CollaboratorRows: len(collaborators), ShareRows: len(shares), Reasons: map[string]int{},
	}
	for _, row := range views {
		decision := AdaptMemberView(row)
		report.record(decision.Disposition, decision.Reason)
	}
	for _, row := range usage {
		decision := AdaptUsageSnapshot(row)
		report.record(decision.Disposition, decision.Reason)
	}
	for _, row := range syncRuns {
		decision := AdaptUsageSyncRun(row)
		report.record(decision.Disposition, decision.Reason)
	}
	for _, row := range collaborators {
		decision := AdaptMemberCollaborator(row)
		report.record(decision.Disposition, decision.Reason)
	}
	for _, row := range shares {
		decision := AdaptMemberShare(row)
		report.record(decision.Disposition, decision.Reason)
	}
	return report, nil
}

func (report *PreflightReport) record(disposition Disposition, reason string) {
	switch disposition {
	case DispositionCandidate:
		report.Candidates++
	case DispositionArchive:
		report.Archived++
	case DispositionQuarantine:
		report.Quarantined++
	}
	if reason != "" {
		report.Reasons[reason]++
	}
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

func validRow(row v1archive.ArchivedRow, tableID string) bool {
	zero := [sha256.Size]byte{}
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == tableID && row.SourceOrdinal > 0 &&
		row.SourceKeyHMAC != zero && row.PayloadHMAC != zero && row.FieldHMAC != zero && json.Valid(row.Payload)
}

func validTimes(created, updated time.Time) bool {
	return !created.IsZero() && !updated.IsZero()
}

func invalidNullableCount(value *int) bool {
	return value != nil && *value < 0
}

func validJSON(value json.RawMessage) bool {
	return len(value) != 0 && json.Valid(value)
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func candidate[T any](value T) Decision[T] {
	return Decision[T]{Disposition: DispositionCandidate, Record: &value}
}

func archived[T any](value T, reason string) Decision[T] {
	return Decision[T]{Disposition: DispositionArchive, Reason: reason, Record: &value}
}

func quarantine[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: DispositionQuarantine, Reason: reason}
}
