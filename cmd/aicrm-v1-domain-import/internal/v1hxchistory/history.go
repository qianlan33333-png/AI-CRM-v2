// Package v1hxchistory classifies frozen V1 HXC material as local,
// non-executable historical observations. It has no database, queue, Provider,
// message, sender-configuration, or current-entitlement dependency.
package v1hxchistory

import (
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DashboardMetaTableID       = "public/user_ops_hxc_dashboard_meta"
	DashboardSnapshotTableID   = "public/user_ops_hxc_dashboard_snapshot"
	ActivationStatusTableID    = "public/user_ops_activation_status_source"
	HuangxiaocanActivationID   = "public/user_ops_huangxiaocan_activation_source"
	ExperienceLeadsTableID     = "public/user_ops_experience_leads"
	ImportBatchesTableID       = "public/user_ops_import_batches"
	SendRecordsTableID         = "public/user_ops_send_records_next"
	SendConfigTableID          = "public/user_ops_hxc_send_config"
	ObservedSnapshot           = "observed_snapshot"
	reasonInvalidSource        = "hxc_history_source_invalid"
	reasonArchiveSendHistory   = "hxc_send_record_archive_only"
	reasonArchiveSenderConfig  = "hxc_sender_config_archive_only"
	reasonUnknownArchiveSource = "hxc_history_unknown_archive_source"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
	DispositionArchive    Disposition = "archive_only"
)

// OpaqueDigest identifies the sealed archive payload. It is not a payload and
// cannot be used to reconstruct a task, identity, message, or configuration.
type OpaqueDigest [sha256.Size]byte

type Decision[T any] struct {
	Disposition Disposition
	Reason      string
	Fact        *T
}

type DashboardMetaFact struct {
	SourceID      int64
	StartedAt     time.Time
	FinishedAt    *time.Time
	Status        string
	RowCount      int64
	MemberHit     int64
	UserHit       int64
	OnlyMember    int64
	TriggerSource string
	PayloadDigest OpaqueDigest
}

// DashboardSnapshotFact is an observation made at ObservedAt, not a current
// V2 customer, membership, entitlement, profile, task, or Provider state.
// UnionID is decoded only as a future private resolver input and never exposed.
type DashboardSnapshotFact struct {
	SourceID                int64
	Observation             string
	ObservedAt              time.Time
	InLeadPool              bool
	InPeople                bool
	InQuestionnaire         bool
	ClassTermNo             *int64
	ClassTermLabel          string
	CRMHXCState             string
	HXCMemberHit            bool
	HXCUserHit              bool
	FunnelState             string
	HXCMemberStatus         string
	HXCRegisteredAt         *time.Time
	HXCLastLoginAt          *time.Time
	MembershipType          string
	MembershipStatus        string
	MembershipEndAt         *time.Time
	MembershipDaysLeft      *int64
	ConsultationUsed        *int64
	ConsultationLimit       *int64
	ConversationChat        int64
	ConversationConsult     int64
	ConversationLesson      int64
	MessagesUser            int64
	MessagesAI              int64
	ConsultCompleted        int64
	LastMessageAt           *time.Time
	SubscriptionTier        string
	SubscriptionExpires     *time.Time
	SubscriptionQuota       *int64
	SubscriptionUsed        *int64
	CRMCreatedAt            *string
	LastQuestionnaireAt     *string
	SubscriptionPeriodStart *string
	PayloadDigest           OpaqueDigest
	resolverUnionID         string
}

// ResolverUnionID is private migration input; it never enters the owner Port or HTTP DTO.
func (fact DashboardSnapshotFact) ResolverUnionID() string { return fact.resolverUnionID }

type ActivationFact struct {
	SourceID             int64
	SourceTable          string
	OriginalState        string
	IsActive             bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	LegacyImportBatchRef *string
	PayloadDigest        OpaqueDigest
}

type ExperienceLeadFact struct {
	SourceID             int64
	OriginalType         string
	IsActive             bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	LegacyImportBatchRef *string
	PayloadDigest        OpaqueDigest
}

type ImportBatchFact struct {
	SourceID      int64
	ImportType    string
	TotalRows     int64
	SuccessRows   int64
	FailedRows    int64
	CreatedAt     time.Time
	PayloadDigest OpaqueDigest
}

type History struct {
	DashboardMeta     []Decision[DashboardMetaFact]
	DashboardSnapshot []Decision[DashboardSnapshotFact]
	ActivationStatus  []Decision[ActivationFact]
	Huangxiaocan      []Decision[ActivationFact]
	ExperienceLeads   []Decision[ExperienceLeadFact]
	ImportBatches     []Decision[ImportBatchFact]
}

// AdaptHistory preserves every input position. Dashboard snapshots retain only
// safe observed metrics; sensitive/free-text fields remain available solely by
// the sealed source payload digest.
func AdaptHistory(meta, snapshots, activationStatus, huangxiaocan, leads, batches []json.RawMessage) History {
	result := History{
		DashboardMeta: make([]Decision[DashboardMetaFact], len(meta)), DashboardSnapshot: make([]Decision[DashboardSnapshotFact], len(snapshots)),
		ActivationStatus: make([]Decision[ActivationFact], len(activationStatus)), Huangxiaocan: make([]Decision[ActivationFact], len(huangxiaocan)),
		ExperienceLeads: make([]Decision[ExperienceLeadFact], len(leads)), ImportBatches: make([]Decision[ImportBatchFact], len(batches)),
	}
	for i := range meta {
		result.DashboardMeta[i] = adaptMeta(meta[i])
	}
	for i := range snapshots {
		result.DashboardSnapshot[i] = adaptSnapshot(snapshots[i])
	}
	for i := range activationStatus {
		result.ActivationStatus[i] = adaptActivation(activationStatus[i], ActivationStatusTableID, "activation_status")
	}
	for i := range huangxiaocan {
		result.Huangxiaocan[i] = adaptActivation(huangxiaocan[i], HuangxiaocanActivationID, "activation_state")
	}
	for i := range leads {
		result.ExperienceLeads[i] = adaptLead(leads[i])
	}
	for i := range batches {
		result.ImportBatches[i] = adaptBatch(batches[i])
	}
	return result
}

// ClassifyArchiveOnlyTable is terminal by design: legacy execution records and
// sender configuration must never become new V2 work or authorization.
func ClassifyArchiveOnlyTable(table string) Decision[struct{}] {
	switch table {
	case SendRecordsTableID:
		return Decision[struct{}]{Disposition: DispositionArchive, Reason: reasonArchiveSendHistory}
	case SendConfigTableID:
		return Decision[struct{}]{Disposition: DispositionArchive, Reason: reasonArchiveSenderConfig}
	default:
		return Decision[struct{}]{Disposition: DispositionQuarantine, Reason: reasonUnknownArchiveSource}
	}
}

type metaSource struct {
	ID            int64      `json:"id"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	Status        string     `json:"status"`
	RowCount      int64      `json:"row_count"`
	MemberHit     int64      `json:"member_hit"`
	UserHit       int64      `json:"user_hit"`
	OnlyMember    int64      `json:"only_member"`
	TriggerSource string     `json:"trigger_source"`
}
type snapshotSource struct {
	ID                      int64      `json:"id"`
	UnionID                 string     `json:"unionid"`
	RefreshedAt             time.Time  `json:"refreshed_at"`
	InLeadPool              bool       `json:"in_lead_pool"`
	InPeople                bool       `json:"in_people"`
	InQuestionnaire         bool       `json:"in_questionnaire"`
	ClassTermNo             *int64     `json:"class_term_no"`
	ClassTermLabel          string     `json:"class_term_label"`
	CRMHXCState             string     `json:"crm_hxc_state"`
	HXCMemberHit            bool       `json:"hxc_member_hit"`
	HXCUserHit              bool       `json:"hxc_user_hit"`
	FunnelState             string     `json:"funnel_state"`
	HXCMemberStatus         string     `json:"hxc_member_status"`
	HXCRegisteredAt         *time.Time `json:"hxc_registered_at"`
	HXCLastLoginAt          *time.Time `json:"hxc_last_login_at"`
	MembershipType          string     `json:"membership_type"`
	MembershipStatus        string     `json:"membership_status"`
	MembershipEndAt         *time.Time `json:"membership_end_at"`
	MembershipDaysLeft      *int64     `json:"membership_days_left"`
	ConsultationUsed        *int64     `json:"consultation_used"`
	ConsultationLimit       *int64     `json:"consultation_limit"`
	ConversationChat        int64      `json:"conv_chat"`
	ConversationConsult     int64      `json:"conv_consult"`
	ConversationLesson      int64      `json:"conv_lesson"`
	MessagesUser            int64      `json:"msg_user"`
	MessagesAI              int64      `json:"msg_ai"`
	ConsultCompleted        int64      `json:"consult_completed"`
	LastMessageAt           *time.Time `json:"last_msg_at"`
	SubscriptionTier        string     `json:"subscription_tier"`
	SubscriptionExpires     *time.Time `json:"subscription_expires_at"`
	SubscriptionQuota       *int64     `json:"subscription_quota"`
	SubscriptionUsed        *int64     `json:"subscription_used"`
	CRMCreatedAt            *string    `json:"crm_created_at"`
	LastQuestionnaireAt     *string    `json:"last_questionnaire_at"`
	SubscriptionPeriodStart *string    `json:"subscription_period_start"`
}
type activationSource struct {
	ID            int64           `json:"id"`
	Mobile        string          `json:"mobile"`
	State         string          `json:"-"`
	ImportBatchID json.RawMessage `json:"import_batch_id"`
	IsActive      bool            `json:"is_active"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
type leadSource struct {
	ID            int64           `json:"id"`
	Mobile        string          `json:"mobile"`
	SourceType    string          `json:"source_type"`
	ImportBatchID json.RawMessage `json:"import_batch_id"`
	IsActive      bool            `json:"is_active"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
type batchSource struct {
	ID          int64     `json:"id"`
	ImportType  string    `json:"import_type"`
	TotalRows   int64     `json:"total_rows"`
	SuccessRows int64     `json:"success_rows"`
	FailedRows  int64     `json:"failed_rows"`
	CreatedAt   time.Time `json:"created_at"`
}

func adaptMeta(raw json.RawMessage) Decision[DashboardMetaFact] {
	var source metaSource
	digest, ok := decode(raw, &source, "id", "started_at", "status", "row_count", "member_hit", "user_hit", "only_member", "trigger_source")
	if !ok || !allPresent(raw, "finished_at") || source.StartedAt.IsZero() || !validText(source.Status) || !validText(source.TriggerSource) {
		return invalid[DashboardMetaFact]()
	}
	return candidate(DashboardMetaFact{SourceID: source.ID, StartedAt: source.StartedAt.UTC(), FinishedAt: utc(source.FinishedAt), Status: source.Status, RowCount: source.RowCount, MemberHit: source.MemberHit, UserHit: source.UserHit, OnlyMember: source.OnlyMember, TriggerSource: source.TriggerSource, PayloadDigest: digest})
}

func adaptSnapshot(raw json.RawMessage) Decision[DashboardSnapshotFact] {
	var source snapshotSource
	digest, ok := decode(raw, &source, "id", "unionid", "refreshed_at", "in_lead_pool", "in_people", "in_questionnaire", "class_term_label", "crm_hxc_state", "hxc_member_hit", "hxc_user_hit", "funnel_state", "hxc_member_status", "membership_type", "membership_status", "conv_chat", "conv_consult", "conv_lesson", "msg_user", "msg_ai", "consult_completed", "subscription_tier")
	if !ok || !allPresent(raw, "class_term_no", "hxc_registered_at", "hxc_last_login_at", "membership_end_at", "membership_days_left", "consultation_used", "consultation_limit", "last_msg_at", "subscription_expires_at", "subscription_quota", "subscription_used", "crm_created_at", "last_questionnaire_at", "subscription_period_start") || source.RefreshedAt.IsZero() || !validSnapshotText(source) || !validDate(source.CRMCreatedAt) || !validDate(source.LastQuestionnaireAt) || !validDate(source.SubscriptionPeriodStart) {
		return invalid[DashboardSnapshotFact]()
	}
	return candidate(DashboardSnapshotFact{SourceID: source.ID, Observation: ObservedSnapshot, ObservedAt: source.RefreshedAt.UTC(), InLeadPool: source.InLeadPool, InPeople: source.InPeople, InQuestionnaire: source.InQuestionnaire, ClassTermNo: source.ClassTermNo, ClassTermLabel: source.ClassTermLabel, CRMHXCState: source.CRMHXCState, HXCMemberHit: source.HXCMemberHit, HXCUserHit: source.HXCUserHit, FunnelState: source.FunnelState, HXCMemberStatus: source.HXCMemberStatus, HXCRegisteredAt: utc(source.HXCRegisteredAt), HXCLastLoginAt: utc(source.HXCLastLoginAt), MembershipType: source.MembershipType, MembershipStatus: source.MembershipStatus, MembershipEndAt: utc(source.MembershipEndAt), MembershipDaysLeft: source.MembershipDaysLeft, ConsultationUsed: source.ConsultationUsed, ConsultationLimit: source.ConsultationLimit, ConversationChat: source.ConversationChat, ConversationConsult: source.ConversationConsult, ConversationLesson: source.ConversationLesson, MessagesUser: source.MessagesUser, MessagesAI: source.MessagesAI, ConsultCompleted: source.ConsultCompleted, LastMessageAt: utc(source.LastMessageAt), SubscriptionTier: source.SubscriptionTier, SubscriptionExpires: utc(source.SubscriptionExpires), SubscriptionQuota: source.SubscriptionQuota, SubscriptionUsed: source.SubscriptionUsed, CRMCreatedAt: cloneDate(source.CRMCreatedAt), LastQuestionnaireAt: cloneDate(source.LastQuestionnaireAt), SubscriptionPeriodStart: cloneDate(source.SubscriptionPeriodStart), PayloadDigest: digest, resolverUnionID: source.UnionID})
}

func adaptActivation(raw json.RawMessage, table, stateField string) Decision[ActivationFact] {
	var source activationSource
	digest, ok := decode(raw, &source, "id", "mobile", stateField, "is_active", "created_at", "updated_at")
	if !ok || !allPresent(raw, "import_batch_id") || !validPrivateText(source.Mobile) || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return invalid[ActivationFact]()
	}
	var batchRef *string
	if table == HuangxiaocanActivationID {
		batchRef, ok = requiredTextRef(source.ImportBatchID)
	} else {
		batchRef, ok = nullableIntRef(source.ImportBatchID)
	}
	if !ok {
		return invalid[ActivationFact]()
	}
	var state struct {
		Value string `json:"value"`
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || json.Unmarshal(object[stateField], &state.Value) != nil || !validText(state.Value) {
		return invalid[ActivationFact]()
	}
	return candidate(ActivationFact{SourceID: source.ID, SourceTable: table, OriginalState: state.Value, IsActive: source.IsActive, CreatedAt: source.CreatedAt.UTC(), UpdatedAt: source.UpdatedAt.UTC(), LegacyImportBatchRef: batchRef, PayloadDigest: digest})
}

func adaptLead(raw json.RawMessage) Decision[ExperienceLeadFact] {
	var source leadSource
	digest, ok := decode(raw, &source, "id", "mobile", "source_type", "is_active", "created_at", "updated_at")
	batchRef, refOK := nullableIntRef(source.ImportBatchID)
	if !ok || !allPresent(raw, "import_batch_id") || !refOK || !validPrivateText(source.Mobile) || !validText(source.SourceType) || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return invalid[ExperienceLeadFact]()
	}
	return candidate(ExperienceLeadFact{SourceID: source.ID, OriginalType: source.SourceType, IsActive: source.IsActive, CreatedAt: source.CreatedAt.UTC(), UpdatedAt: source.UpdatedAt.UTC(), LegacyImportBatchRef: batchRef, PayloadDigest: digest})
}

func adaptBatch(raw json.RawMessage) Decision[ImportBatchFact] {
	var source batchSource
	digest, ok := decode(raw, &source, "id", "import_type", "total_rows", "success_rows", "failed_rows", "created_at")
	if !ok || !validText(source.ImportType) || source.CreatedAt.IsZero() {
		return invalid[ImportBatchFact]()
	}
	return candidate(ImportBatchFact{SourceID: source.ID, ImportType: source.ImportType, TotalRows: source.TotalRows, SuccessRows: source.SuccessRows, FailedRows: source.FailedRows, CreatedAt: source.CreatedAt.UTC(), PayloadDigest: digest})
}

func decode[T any](raw json.RawMessage, target *T, fields ...string) (OpaqueDigest, bool) {
	var empty OpaqueDigest
	if !json.Valid(raw) || json.Unmarshal(raw, target) != nil {
		return empty, false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return empty, false
	}
	for _, field := range fields {
		if value, found := object[field]; !found || string(value) == "null" {
			return empty, false
		}
	}
	return sha256.Sum256(raw), true
}

func allPresent(raw json.RawMessage, fields ...string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	for _, field := range fields {
		if _, found := object[field]; !found {
			return false
		}
	}
	return true
}

func validSnapshotText(source snapshotSource) bool {
	for _, value := range []string{source.ClassTermLabel, source.CRMHXCState, source.FunnelState, source.HXCMemberStatus, source.MembershipType, source.MembershipStatus, source.SubscriptionTier} {
		if !validText(value) {
			return false
		}
	}
	return true
}
func validText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
func validPrivateText(value string) bool { return value != "" && validText(value) }
func validDate(value *string) bool {
	if value == nil {
		return true
	}
	parsed, err := time.Parse("2006-01-02", *value)
	return err == nil && parsed.Format("2006-01-02") == *value
}
func cloneDate(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
func nullableIntRef(raw json.RawMessage) (*string, bool) {
	if string(raw) == "null" {
		return nil, true
	}
	var value int64
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	result := strconv.FormatInt(value, 10)
	return &result, true
}
func requiredTextRef(raw json.RawMessage) (*string, bool) {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || !validText(value) {
		return nil, false
	}
	return &value, true
}
func utc(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}
func candidate[T any](fact T) Decision[T] {
	return Decision[T]{Disposition: DispositionCandidate, Fact: &fact}
}
func invalid[T any]() Decision[T] {
	return Decision[T]{Disposition: DispositionQuarantine, Reason: reasonInvalidSource}
}
