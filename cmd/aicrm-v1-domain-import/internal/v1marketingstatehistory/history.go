// Package v1marketingstatehistory classifies V1 marketing-state and value
// segment rows as inert Segment-owned history. It has no target store, current
// customer resolver, score engine, trigger, queue, or Provider dependency.
package v1marketingstatehistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	MarketingStateCurrentTableID = "public/customer_marketing_state_current"
	MarketingStateHistoryTableID = "public/customer_marketing_state_history"
	ValueSegmentCurrentTableID   = "public/customer_value_segment_current"
	ValueSegmentHistoryTableID   = "public/customer_value_segment_history"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// OpaqueDigest is a non-recoverable summary. It is safe to expose; source
// identities and JSON material remain only in the authenticated archive.
type OpaqueDigest [sha256.Size]byte

// SourceEnvelope binds a fact to the immutable V1 archive record. These are
// archive HMACs, not source values and not V2 identifiers.
type SourceEnvelope struct {
	SourceKeyDigest OpaqueDigest
	PayloadDigest   OpaqueDigest
	FieldDigest     OpaqueDigest
}

// MarketingStateCurrentFact is a V1 snapshot, never a current V2 customer
// state. CustomerID is deliberately always nil: no verified unionid crosswalk
// is present in these tables.
type MarketingStateCurrentFact struct {
	SourceID               int64
	Source                 SourceEnvelope
	CustomerID             *int64
	PersonSourceID         *int64
	ExternalUserIDDigest   OpaqueDigest
	AutomationKey          string
	MainStage              string
	SubStage               string
	Activated              bool
	Converted              bool
	EligibleForConversion  bool
	LifecycleStatus        string
	LastActivationAt       string
	LastConversionMarkedAt string
	LastMessageAt          string
	LastBatchSourceID      *int64
	LastBatchStatus        string
	LastBatchWindowStart   string
	LastBatchWindowEnd     string
	LastTriggerMessageAt   string
	EnteredAt              *time.Time
	ExitedAt               *time.Time
	ExitReason             string
	StatePayloadDigest     OpaqueDigest
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// MarketingStateHistoryFact preserves a V1 transition record. Batch and
// person values are source references only and cannot select a V2 customer.
type MarketingStateHistoryFact struct {
	SourceID               int64
	Source                 SourceEnvelope
	CustomerID             *int64
	PersonSourceID         *int64
	ExternalUserIDDigest   OpaqueDigest
	AutomationKey          string
	MainStage              string
	SubStage               string
	Activated              bool
	Converted              bool
	EligibleForConversion  bool
	BatchSourceID          *int64
	LifecycleStatus        string
	ExitReason             string
	LastActivationAt       string
	LastConversionMarkedAt string
	LastMessageAt          string
	ChangeReason           string
	StatePayloadDigest     OpaqueDigest
	RecordedAt             time.Time
	CreatedAt              time.Time
}

// ValueSegmentCurrentFact is a historical scoring snapshot. Its rank, score,
// and status-like fields are preserved as source scalars only; they do not run
// a V2 score or change current segment membership.
type ValueSegmentCurrentFact struct {
	SourceID                 int64
	Source                   SourceEnvelope
	CustomerID               *int64
	ExternalUserIDDigest     OpaqueDigest
	Segment                  string
	SegmentRank              int64
	Score                    int64
	ScoringVersion           string
	ComputedReason           string
	SubmissionSourceID       *int64
	MatchedQuestionIDsDigest OpaqueDigest
	SourcePayloadDigest      OpaqueDigest
	EvaluatedAt              time.Time
	ComputedAt               time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// ValueSegmentHistoryFact is an inert V1 history record. SubmissionSourceID
// remains a source reference and never becomes a V2 questionnaire FK.
type ValueSegmentHistoryFact struct {
	SourceID                 int64
	Source                   SourceEnvelope
	CustomerID               *int64
	ExternalUserIDDigest     OpaqueDigest
	Segment                  string
	SegmentRank              int64
	Score                    int64
	ScoringVersion           string
	ChangeReason             string
	SubmissionSourceID       *int64
	MatchedQuestionIDsDigest OpaqueDigest
	SourcePayloadDigest      OpaqueDigest
	EvaluatedAt              time.Time
	RecordedAt               time.Time
	CreatedAt                time.Time
}

type Result[T any] struct {
	SourceID    int64
	Disposition Disposition
	Reason      string
	Fact        *T
}

// History preserves table separation, source order, and row count. It is not
// an import command and cannot activate historical marketing behaviour.
type History struct {
	MarketingStateCurrent []Result[MarketingStateCurrentFact]
	MarketingStateHistory []Result[MarketingStateHistoryFact]
	ValueSegmentCurrent   []Result[ValueSegmentCurrentFact]
	ValueSegmentHistory   []Result[ValueSegmentHistoryFact]
}

func (h History) SourceCount() int {
	return len(h.MarketingStateCurrent) + len(h.MarketingStateHistory) + len(h.ValueSegmentCurrent) + len(h.ValueSegmentHistory)
}

func (h History) TerminalCount() int {
	return terminalCount(h.MarketingStateCurrent) + terminalCount(h.MarketingStateHistory) + terminalCount(h.ValueSegmentCurrent) + terminalCount(h.ValueSegmentHistory)
}

func terminalCount[T any](rows []Result[T]) int {
	count := 0
	for _, row := range rows {
		if row.Disposition == DispositionCandidate || row.Disposition == DispositionQuarantine {
			count++
		}
	}
	return count
}

type marketingStateCurrentJSON struct {
	ID                     int64           `json:"id"`
	PersonID               *int64          `json:"person_id"`
	ExternalUserID         string          `json:"external_userid"`
	AutomationKey          string          `json:"automation_key"`
	MainStage              string          `json:"main_stage"`
	SubStage               string          `json:"sub_stage"`
	Activated              bool            `json:"activated"`
	Converted              bool            `json:"converted"`
	EligibleForConversion  bool            `json:"eligible_for_conversion"`
	LifecycleStatus        string          `json:"lifecycle_status"`
	LastActivationAt       string          `json:"last_activation_at"`
	LastConversionMarkedAt string          `json:"last_conversion_marked_at"`
	LastMessageAt          string          `json:"last_message_at"`
	LastBatchID            *int64          `json:"last_batch_id"`
	LastBatchStatus        string          `json:"last_batch_status"`
	LastBatchWindowStart   string          `json:"last_batch_window_start"`
	LastBatchWindowEnd     string          `json:"last_batch_window_end"`
	LastTriggerMessageAt   string          `json:"last_trigger_message_at"`
	EnteredAt              *time.Time      `json:"entered_at"`
	ExitedAt               *time.Time      `json:"exited_at"`
	ExitReason             string          `json:"exit_reason"`
	StatePayload           json.RawMessage `json:"state_payload_json"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

type marketingStateHistoryJSON struct {
	ID                     int64           `json:"id"`
	PersonID               *int64          `json:"person_id"`
	ExternalUserID         string          `json:"external_userid"`
	AutomationKey          string          `json:"automation_key"`
	MainStage              string          `json:"main_stage"`
	SubStage               string          `json:"sub_stage"`
	Activated              bool            `json:"activated"`
	Converted              bool            `json:"converted"`
	EligibleForConversion  bool            `json:"eligible_for_conversion"`
	BatchID                *int64          `json:"batch_id"`
	LifecycleStatus        string          `json:"lifecycle_status"`
	ExitReason             string          `json:"exit_reason"`
	LastActivationAt       string          `json:"last_activation_at"`
	LastConversionMarkedAt string          `json:"last_conversion_marked_at"`
	LastMessageAt          string          `json:"last_message_at"`
	ChangeReason           string          `json:"change_reason"`
	StatePayload           json.RawMessage `json:"state_payload_json"`
	RecordedAt             time.Time       `json:"recorded_at"`
	CreatedAt              time.Time       `json:"created_at"`
}

type valueSegmentCurrentJSON struct {
	ID                 int64           `json:"id"`
	ExternalUserID     string          `json:"external_userid"`
	Segment            string          `json:"segment"`
	SegmentRank        int64           `json:"segment_rank"`
	Score              int64           `json:"score"`
	ScoringVersion     string          `json:"scoring_version"`
	ComputedReason     string          `json:"computed_reason"`
	SubmissionID       *int64          `json:"submission_id"`
	MatchedQuestionIDs json.RawMessage `json:"matched_question_ids_json"`
	SourcePayload      json.RawMessage `json:"source_payload_json"`
	EvaluatedAt        time.Time       `json:"evaluated_at"`
	ComputedAt         time.Time       `json:"computed_at"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type valueSegmentHistoryJSON struct {
	ID                 int64           `json:"id"`
	ExternalUserID     string          `json:"external_userid"`
	Segment            string          `json:"segment"`
	SegmentRank        int64           `json:"segment_rank"`
	Score              int64           `json:"score"`
	ScoringVersion     string          `json:"scoring_version"`
	ChangeReason       string          `json:"change_reason"`
	SubmissionID       *int64          `json:"submission_id"`
	MatchedQuestionIDs json.RawMessage `json:"matched_question_ids_json"`
	SourcePayload      json.RawMessage `json:"source_payload_json"`
	EvaluatedAt        time.Time       `json:"evaluated_at"`
	RecordedAt         time.Time       `json:"recorded_at"`
	CreatedAt          time.Time       `json:"created_at"`
}

// AdaptHistory accepts only authenticated archive envelopes. An archive row
// with a redacted path is quarantined rather than treating a placeholder as a
// V1 NULL or former source value; nested JSON array paths are included.
func AdaptHistory(marketingCurrent, marketingHistory, valueCurrent, valueHistory []v1archive.ArchivedRow) History {
	history := History{
		MarketingStateCurrent: make([]Result[MarketingStateCurrentFact], len(marketingCurrent)),
		MarketingStateHistory: make([]Result[MarketingStateHistoryFact], len(marketingHistory)),
		ValueSegmentCurrent:   make([]Result[ValueSegmentCurrentFact], len(valueCurrent)),
		ValueSegmentHistory:   make([]Result[ValueSegmentHistoryFact], len(valueHistory)),
	}
	for i, row := range marketingCurrent {
		history.MarketingStateCurrent[i] = adaptMarketingStateCurrent(row, int64(i+1))
	}
	for i, row := range marketingHistory {
		history.MarketingStateHistory[i] = adaptMarketingStateHistory(row, int64(i+1))
	}
	for i, row := range valueCurrent {
		history.ValueSegmentCurrent[i] = adaptValueSegmentCurrent(row, int64(i+1))
	}
	for i, row := range valueHistory {
		history.ValueSegmentHistory[i] = adaptValueSegmentHistory(row, int64(i+1))
	}
	quarantineDuplicateIDs(history.MarketingStateCurrent, "customer_marketing_state_current_source_ambiguous")
	quarantineDuplicateIDs(history.MarketingStateHistory, "customer_marketing_state_history_source_ambiguous")
	quarantineDuplicateIDs(history.ValueSegmentCurrent, "customer_value_segment_current_source_ambiguous")
	quarantineDuplicateIDs(history.ValueSegmentHistory, "customer_value_segment_history_source_ambiguous")
	return history
}

func adaptMarketingStateCurrent(row v1archive.ArchivedRow, ordinal int64) Result[MarketingStateCurrentFact] {
	fields, envelope, reason := archiveFields(row, MarketingStateCurrentTableID, ordinal)
	if reason != "" {
		return quarantine[MarketingStateCurrentFact](0, reason)
	}
	var value marketingStateCurrentJSON
	if !decodeExact(fields, row.Payload, &value, marketingStateCurrentFields, []string{"person_id", "last_batch_id", "entered_at", "exited_at"}) {
		return quarantine[MarketingStateCurrentFact](sourceID(fields), "customer_marketing_state_current_shape_invalid")
	}
	return candidate(MarketingStateCurrentFact{SourceID: value.ID, Source: envelope, PersonSourceID: value.PersonID, ExternalUserIDDigest: fieldDigest(MarketingStateCurrentTableID, "external_userid", fields["external_userid"]), AutomationKey: value.AutomationKey, MainStage: value.MainStage, SubStage: value.SubStage, Activated: value.Activated, Converted: value.Converted, EligibleForConversion: value.EligibleForConversion, LifecycleStatus: value.LifecycleStatus, LastActivationAt: value.LastActivationAt, LastConversionMarkedAt: value.LastConversionMarkedAt, LastMessageAt: value.LastMessageAt, LastBatchSourceID: value.LastBatchID, LastBatchStatus: value.LastBatchStatus, LastBatchWindowStart: value.LastBatchWindowStart, LastBatchWindowEnd: value.LastBatchWindowEnd, LastTriggerMessageAt: value.LastTriggerMessageAt, EnteredAt: value.EnteredAt, ExitedAt: value.ExitedAt, ExitReason: value.ExitReason, StatePayloadDigest: fieldDigest(MarketingStateCurrentTableID, "state_payload_json", fields["state_payload_json"]), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt})
}

func adaptMarketingStateHistory(row v1archive.ArchivedRow, ordinal int64) Result[MarketingStateHistoryFact] {
	fields, envelope, reason := archiveFields(row, MarketingStateHistoryTableID, ordinal)
	if reason != "" {
		return quarantine[MarketingStateHistoryFact](0, reason)
	}
	var value marketingStateHistoryJSON
	if !decodeExact(fields, row.Payload, &value, marketingStateHistoryFields, []string{"person_id", "batch_id"}) {
		return quarantine[MarketingStateHistoryFact](sourceID(fields), "customer_marketing_state_history_shape_invalid")
	}
	return candidate(MarketingStateHistoryFact{SourceID: value.ID, Source: envelope, PersonSourceID: value.PersonID, ExternalUserIDDigest: fieldDigest(MarketingStateHistoryTableID, "external_userid", fields["external_userid"]), AutomationKey: value.AutomationKey, MainStage: value.MainStage, SubStage: value.SubStage, Activated: value.Activated, Converted: value.Converted, EligibleForConversion: value.EligibleForConversion, BatchSourceID: value.BatchID, LifecycleStatus: value.LifecycleStatus, ExitReason: value.ExitReason, LastActivationAt: value.LastActivationAt, LastConversionMarkedAt: value.LastConversionMarkedAt, LastMessageAt: value.LastMessageAt, ChangeReason: value.ChangeReason, StatePayloadDigest: fieldDigest(MarketingStateHistoryTableID, "state_payload_json", fields["state_payload_json"]), RecordedAt: value.RecordedAt, CreatedAt: value.CreatedAt})
}

func adaptValueSegmentCurrent(row v1archive.ArchivedRow, ordinal int64) Result[ValueSegmentCurrentFact] {
	fields, envelope, reason := archiveFields(row, ValueSegmentCurrentTableID, ordinal)
	if reason != "" {
		return quarantine[ValueSegmentCurrentFact](0, reason)
	}
	var value valueSegmentCurrentJSON
	if !decodeExact(fields, row.Payload, &value, valueSegmentCurrentFields, []string{"submission_id"}) {
		return quarantine[ValueSegmentCurrentFact](sourceID(fields), "customer_value_segment_current_shape_invalid")
	}
	return candidate(ValueSegmentCurrentFact{SourceID: value.ID, Source: envelope, ExternalUserIDDigest: fieldDigest(ValueSegmentCurrentTableID, "external_userid", fields["external_userid"]), Segment: value.Segment, SegmentRank: value.SegmentRank, Score: value.Score, ScoringVersion: value.ScoringVersion, ComputedReason: value.ComputedReason, SubmissionSourceID: value.SubmissionID, MatchedQuestionIDsDigest: fieldDigest(ValueSegmentCurrentTableID, "matched_question_ids_json", fields["matched_question_ids_json"]), SourcePayloadDigest: fieldDigest(ValueSegmentCurrentTableID, "source_payload_json", fields["source_payload_json"]), EvaluatedAt: value.EvaluatedAt, ComputedAt: value.ComputedAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt})
}

func adaptValueSegmentHistory(row v1archive.ArchivedRow, ordinal int64) Result[ValueSegmentHistoryFact] {
	fields, envelope, reason := archiveFields(row, ValueSegmentHistoryTableID, ordinal)
	if reason != "" {
		return quarantine[ValueSegmentHistoryFact](0, reason)
	}
	var value valueSegmentHistoryJSON
	if !decodeExact(fields, row.Payload, &value, valueSegmentHistoryFields, []string{"submission_id"}) {
		return quarantine[ValueSegmentHistoryFact](sourceID(fields), "customer_value_segment_history_shape_invalid")
	}
	return candidate(ValueSegmentHistoryFact{SourceID: value.ID, Source: envelope, ExternalUserIDDigest: fieldDigest(ValueSegmentHistoryTableID, "external_userid", fields["external_userid"]), Segment: value.Segment, SegmentRank: value.SegmentRank, Score: value.Score, ScoringVersion: value.ScoringVersion, ChangeReason: value.ChangeReason, SubmissionSourceID: value.SubmissionID, MatchedQuestionIDsDigest: fieldDigest(ValueSegmentHistoryTableID, "matched_question_ids_json", fields["matched_question_ids_json"]), SourcePayloadDigest: fieldDigest(ValueSegmentHistoryTableID, "source_payload_json", fields["source_payload_json"]), EvaluatedAt: value.EvaluatedAt, RecordedAt: value.RecordedAt, CreatedAt: value.CreatedAt})
}

var marketingStateCurrentFields = []string{"id", "person_id", "external_userid", "automation_key", "main_stage", "sub_stage", "activated", "converted", "eligible_for_conversion", "lifecycle_status", "last_activation_at", "last_conversion_marked_at", "last_message_at", "last_batch_id", "last_batch_status", "last_batch_window_start", "last_batch_window_end", "last_trigger_message_at", "entered_at", "exited_at", "exit_reason", "state_payload_json", "created_at", "updated_at"}
var marketingStateHistoryFields = []string{"id", "person_id", "external_userid", "automation_key", "main_stage", "sub_stage", "activated", "converted", "eligible_for_conversion", "batch_id", "lifecycle_status", "exit_reason", "last_activation_at", "last_conversion_marked_at", "last_message_at", "change_reason", "state_payload_json", "recorded_at", "created_at"}
var valueSegmentCurrentFields = []string{"id", "external_userid", "segment", "segment_rank", "score", "scoring_version", "computed_reason", "submission_id", "matched_question_ids_json", "source_payload_json", "evaluated_at", "computed_at", "created_at", "updated_at"}
var valueSegmentHistoryFields = []string{"id", "external_userid", "segment", "segment_rank", "score", "scoring_version", "change_reason", "submission_id", "matched_question_ids_json", "source_payload_json", "evaluated_at", "recorded_at", "created_at"}

func archiveFields(row v1archive.ArchivedRow, table string, ordinal int64) (fields, SourceEnvelope, string) {
	zero := [sha256.Size]byte{}
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, table + "_archive_invalid"
	}
	if len(row.RedactedFields) != 0 {
		for _, path := range row.RedactedFields {
			if redactionPathMatches("state_payload_json", path) || redactionPathMatches("matched_question_ids_json", path) || redactionPathMatches("source_payload_json", path) {
				return nil, SourceEnvelope{}, table + "_source_redacted"
			}
		}
		return nil, SourceEnvelope{}, table + "_source_redacted"
	}
	parsed, ok := object(row.Payload)
	if !ok {
		return nil, SourceEnvelope{}, table + "_shape_invalid"
	}
	return parsed, SourceEnvelope{SourceKeyDigest: OpaqueDigest(row.SourceKeyHMAC), PayloadDigest: OpaqueDigest(row.PayloadHMAC), FieldDigest: OpaqueDigest(row.FieldHMAC)}, ""
}

// redactionPathMatches includes both object and array nesting. It is kept
// separate so a future archive preflight cannot accidentally accept
// state_payload_json[0].secret as an unredacted source value.
func redactionPathMatches(field, path string) bool {
	return path == field || strings.HasPrefix(path, field+".") || strings.HasPrefix(path, field+"[")
}

func decodeExact(source fields, payload []byte, target any, names, optional []string) bool {
	if len(source) != len(names) {
		return false
	}
	optionalSet := make(map[string]bool, len(optional))
	for _, name := range optional {
		optionalSet[name] = true
	}
	for _, name := range names {
		raw, found := source[name]
		if !found || (!optionalSet[name] && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

type fields map[string]json.RawMessage

func object(value []byte) (fields, bool) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	result := make(fields)
	if decoder.Decode(&result) != nil || result == nil {
		return nil, false
	}
	var extra any
	return result, errors.Is(decoder.Decode(&extra), io.EOF)
}

func sourceID(source fields) int64 {
	raw, found := source["id"]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0
	}
	var id int64
	if json.Unmarshal(raw, &id) != nil || id < 1 {
		return 0
	}
	return id
}

func fieldDigest(table, field string, value []byte) OpaqueDigest {
	data := append([]byte("v1-marketing-state-history-field-v1\x00"+table+"\x00"+field+"\x00"), value...)
	return OpaqueDigest(sha256.Sum256(data))
}

func candidate[T any](fact T) Result[T] {
	return Result[T]{SourceID: factSourceID(fact), Disposition: DispositionCandidate, Fact: &fact}
}

func quarantine[T any](sourceID int64, reason string) Result[T] {
	return Result[T]{SourceID: sourceID, Disposition: DispositionQuarantine, Reason: reason}
}

func factSourceID(value any) int64 {
	switch value := value.(type) {
	case MarketingStateCurrentFact:
		return value.SourceID
	case MarketingStateHistoryFact:
		return value.SourceID
	case ValueSegmentCurrentFact:
		return value.SourceID
	case ValueSegmentHistoryFact:
		return value.SourceID
	default:
		return 0
	}
}

func quarantineDuplicateIDs[T any](values []Result[T], reason string) {
	counts := make(map[int64]int, len(values))
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			counts[value.SourceID]++
		}
	}
	for i := range values {
		if values[i].Disposition == DispositionCandidate && values[i].Fact != nil && counts[values[i].SourceID] > 1 {
			values[i] = quarantine[T](values[i].SourceID, reason)
		}
	}
}
