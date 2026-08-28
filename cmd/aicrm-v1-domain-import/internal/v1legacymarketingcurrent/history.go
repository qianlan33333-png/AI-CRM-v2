// Package v1legacymarketingcurrent parses two V1 marketing snapshots as
// private, inert source facts. It has no target-store or identity dependency.
package v1legacymarketingcurrent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	MarketingStateCurrentTableID        = "public/marketing_state_current"
	MarketingValueSegmentCurrentTableID = "public/marketing_value_segment_current"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// SourceRecord is immutable archive evidence, not a V2 identifier.
type SourceRecord struct {
	SourceKeyHMAC [sha256.Size]byte
	PayloadHMAC   [sha256.Size]byte
	FieldHMAC     [sha256.Size]byte
	SourceOrdinal int64
}

// MarketingStateCurrentFact preserves the original V1 snapshot. External user
// IDs, payload JSON, and the batch source reference must not select a V2
// customer, automation, or batch.
type MarketingStateCurrentFact struct {
	SourceID             int64
	Source               SourceRecord
	ScenarioKey          string
	ExternalUserID       string
	MarketingPhase       string
	PhaseLabel           string
	PhaseReason          string
	LifecycleStatus      string
	LastBatchSourceID    *int64
	LastBatchStatus      string
	LastBatchWindowStart string
	LastBatchWindowEnd   string
	LastTriggerMessageAt string
	EnteredAt            *time.Time
	ExitedAt             *time.Time
	ExitReason           string
	SourcePayload        json.RawMessage
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// MarketingValueSegmentCurrentFact is a V1 score snapshot only. Score and
// breakdown JSON do not run a V2 scoring engine or alter current membership.
type MarketingValueSegmentCurrentFact struct {
	SourceID       int64
	Source         SourceRecord
	ScenarioKey    string
	ExternalUserID string
	ValueSegment   string
	SegmentLabel   string
	Score          int64
	ScoreBreakdown json.RawMessage
	SourcePayload  json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Result[T any] struct {
	SourceID    int64
	Disposition Disposition
	Reason      string
	Fact        *T
}

type History struct {
	MarketingStateCurrent        []Result[MarketingStateCurrentFact]
	MarketingValueSegmentCurrent []Result[MarketingValueSegmentCurrentFact]
}

func (history History) SourceCount() int {
	return len(history.MarketingStateCurrent) + len(history.MarketingValueSegmentCurrent)
}

func (history History) TerminalCount() int {
	return terminalCount(history.MarketingStateCurrent) + terminalCount(history.MarketingValueSegmentCurrent)
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

// AdaptHistory preserves source-row order and table separation. It neither
// creates V2 rows nor turns a legacy lifecycle state into an active state.
func AdaptHistory(marketingStateCurrent, marketingValueSegmentCurrent []v1archive.ArchivedRow) History {
	history := History{
		MarketingStateCurrent:        make([]Result[MarketingStateCurrentFact], len(marketingStateCurrent)),
		MarketingValueSegmentCurrent: make([]Result[MarketingValueSegmentCurrentFact], len(marketingValueSegmentCurrent)),
	}
	for index, row := range marketingStateCurrent {
		history.MarketingStateCurrent[index] = adaptMarketingStateCurrent(row, int64(index+1))
	}
	for index, row := range marketingValueSegmentCurrent {
		history.MarketingValueSegmentCurrent[index] = adaptMarketingValueSegmentCurrent(row, int64(index+1))
	}
	quarantineDuplicateSourceIDs(history.MarketingStateCurrent, "marketing_state_current_source_id_ambiguous")
	quarantineDuplicateSourceIDs(history.MarketingValueSegmentCurrent, "marketing_value_segment_current_source_id_ambiguous")
	return history
}

func adaptMarketingStateCurrent(row v1archive.ArchivedRow, ordinal int64) Result[MarketingStateCurrentFact] {
	fields, source, reason := archiveFields(row, MarketingStateCurrentTableID, ordinal, marketingStateCurrentFields)
	if reason != "" {
		return quarantine[MarketingStateCurrentFact](sourceID(fields), reason)
	}
	var value marketingStateCurrentJSON
	if json.Unmarshal(row.Payload, &value) != nil {
		return quarantine[MarketingStateCurrentFact](sourceID(fields), "marketing_state_current_shape_invalid")
	}
	return candidate(MarketingStateCurrentFact{
		SourceID: value.ID, Source: source, ScenarioKey: value.ScenarioKey, ExternalUserID: value.ExternalUserID,
		MarketingPhase: value.MarketingPhase, PhaseLabel: value.PhaseLabel, PhaseReason: value.PhaseReason,
		LifecycleStatus: value.LifecycleStatus, LastBatchSourceID: copyID(value.LastBatchID), LastBatchStatus: value.LastBatchStatus,
		LastBatchWindowStart: value.LastBatchWindowStart, LastBatchWindowEnd: value.LastBatchWindowEnd,
		LastTriggerMessageAt: value.LastTriggerMessageAt, EnteredAt: copyTime(value.EnteredAt), ExitedAt: copyTime(value.ExitedAt),
		ExitReason: value.ExitReason, SourcePayload: cloneRaw(value.SourcePayload), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	})
}

func adaptMarketingValueSegmentCurrent(row v1archive.ArchivedRow, ordinal int64) Result[MarketingValueSegmentCurrentFact] {
	fields, source, reason := archiveFields(row, MarketingValueSegmentCurrentTableID, ordinal, marketingValueSegmentCurrentFields)
	if reason != "" {
		return quarantine[MarketingValueSegmentCurrentFact](sourceID(fields), reason)
	}
	var value marketingValueSegmentCurrentJSON
	if json.Unmarshal(row.Payload, &value) != nil {
		return quarantine[MarketingValueSegmentCurrentFact](sourceID(fields), "marketing_value_segment_current_shape_invalid")
	}
	return candidate(MarketingValueSegmentCurrentFact{
		SourceID: value.ID, Source: source, ScenarioKey: value.ScenarioKey, ExternalUserID: value.ExternalUserID,
		ValueSegment: value.ValueSegment, SegmentLabel: value.SegmentLabel, Score: value.Score,
		ScoreBreakdown: cloneRaw(value.ScoreBreakdown), SourcePayload: cloneRaw(value.SourcePayload), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	})
}

type marketingStateCurrentJSON struct {
	ID                   int64           `json:"id"`
	ScenarioKey          string          `json:"scenario_key"`
	ExternalUserID       string          `json:"external_userid"`
	MarketingPhase       string          `json:"marketing_phase"`
	PhaseLabel           string          `json:"phase_label"`
	PhaseReason          string          `json:"phase_reason"`
	LifecycleStatus      string          `json:"lifecycle_status"`
	LastBatchID          *int64          `json:"last_batch_id"`
	LastBatchStatus      string          `json:"last_batch_status"`
	LastBatchWindowStart string          `json:"last_batch_window_start"`
	LastBatchWindowEnd   string          `json:"last_batch_window_end"`
	LastTriggerMessageAt string          `json:"last_trigger_message_at"`
	EnteredAt            *time.Time      `json:"entered_at"`
	ExitedAt             *time.Time      `json:"exited_at"`
	ExitReason           string          `json:"exit_reason"`
	SourcePayload        json.RawMessage `json:"source_payload_json"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type marketingValueSegmentCurrentJSON struct {
	ID             int64           `json:"id"`
	ScenarioKey    string          `json:"scenario_key"`
	ExternalUserID string          `json:"external_userid"`
	ValueSegment   string          `json:"value_segment"`
	SegmentLabel   string          `json:"segment_label"`
	Score          int64           `json:"score"`
	ScoreBreakdown json.RawMessage `json:"score_breakdown_json"`
	SourcePayload  json.RawMessage `json:"source_payload_json"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

var marketingStateCurrentFields = []string{
	"id", "scenario_key", "external_userid", "marketing_phase", "phase_label", "phase_reason", "lifecycle_status",
	"last_batch_id", "last_batch_status", "last_batch_window_start", "last_batch_window_end", "last_trigger_message_at",
	"entered_at", "exited_at", "exit_reason", "source_payload_json", "created_at", "updated_at",
}

var marketingValueSegmentCurrentFields = []string{
	"id", "scenario_key", "external_userid", "value_segment", "segment_label", "score", "score_breakdown_json", "source_payload_json", "created_at", "updated_at",
}

func archiveFields(row v1archive.ArchivedRow, tableID string, ordinal int64, names []string) (map[string]json.RawMessage, SourceRecord, string) {
	zero := [sha256.Size]byte{}
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal != ordinal ||
		row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || len(row.RedactedFields) != 0 || !json.Valid(row.Payload) {
		return nil, SourceRecord{}, tableID[7:] + "_archive_invalid"
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(row.Payload, &fields) != nil || len(fields) != len(names) {
		return fields, SourceRecord{}, tableID[7:] + "_shape_invalid"
	}
	for _, name := range names {
		value, found := fields[name]
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) && name != "last_batch_id" && name != "entered_at" && name != "exited_at" {
			return fields, SourceRecord{}, tableID[7:] + "_shape_invalid"
		}
	}
	return fields, SourceRecord{SourceKeyHMAC: row.SourceKeyHMAC, PayloadHMAC: row.PayloadHMAC, FieldHMAC: row.FieldHMAC, SourceOrdinal: row.SourceOrdinal}, ""
}

func sourceID(fields map[string]json.RawMessage) int64 {
	if fields == nil {
		return 0
	}
	var value int64
	if json.Unmarshal(fields["id"], &value) != nil {
		return 0
	}
	return value
}

func candidate[T any](fact T) Result[T] {
	return Result[T]{SourceID: factSourceID(fact), Disposition: DispositionCandidate, Fact: &fact}
}

func quarantine[T any](id int64, reason string) Result[T] {
	return Result[T]{SourceID: id, Disposition: DispositionQuarantine, Reason: reason}
}

func factSourceID(value any) int64 {
	switch value := value.(type) {
	case MarketingStateCurrentFact:
		return value.SourceID
	case MarketingValueSegmentCurrentFact:
		return value.SourceID
	default:
		return 0
	}
}

func quarantineDuplicateSourceIDs[T any](rows []Result[T], reason string) {
	counts := make(map[int64]int, len(rows))
	for _, row := range rows {
		if row.Disposition == DispositionCandidate && row.Fact != nil {
			counts[row.SourceID]++
		}
	}
	for index, row := range rows {
		if row.Disposition == DispositionCandidate && row.Fact != nil && counts[row.SourceID] > 1 {
			rows[index] = quarantine[T](row.SourceID, reason)
		}
	}
}

func copyID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
