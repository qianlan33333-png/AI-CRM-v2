// Package v1outboundtaskhistory classifies V1 outbound task rows as inert
// historical candidates. It cannot write, enqueue, retry, or execute V2 work.
package v1outboundtaskhistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const OutboundTasksTableID = "public/outbound_tasks"

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

const (
	ReasonInvalidArchiveRow    = "outbound_tasks_archive_invalid"
	ReasonInvalidSourcePayload = "outbound_tasks_shape_invalid"
	ReasonUnknownRedactedField = "outbound_tasks_redaction_unknown"
	ReasonDuplicateSourceID    = "outbound_tasks_source_ambiguous"
)

// OpaqueDigest is a private comparison value. It is calculated from the
// canonical archived field bytes, which are redacted bytes when RedactedRoots
// names that field; it never claims to summarize unavailable source plaintext.
type OpaqueDigest [sha256.Size]byte

// SourceEnvelope binds a fact to one immutable archive row. These HMACs are
// not V2 identities and are deliberately not serialized.
type SourceEnvelope struct {
	SourceKeyDigest OpaqueDigest
	PayloadDigest   OpaqueDigest
	FieldDigest     OpaqueDigest
}

// OutboundTaskHistoryFact retains a legacy task as an observation only. The
// private legacy payloads, WeCom identifier, trace, and broadcast-job pointer
// cannot create a V2 task, provider call, or source relation.
type OutboundTaskHistoryFact struct {
	SourceID  int64     `json:"source_id"`
	TaskType  string    `json:"task_type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`

	RequestPayloadDigest  OpaqueDigest   `json:"-"`
	ResponsePayloadDigest OpaqueDigest   `json:"-"`
	WeComTaskIDDigest     *OpaqueDigest  `json:"-"`
	TraceIDDigest         OpaqueDigest   `json:"-"`
	LegacyBroadcastJobID  *int64         `json:"-"`
	Source                SourceEnvelope `json:"-"`
	RedactedRoots         []string       `json:"-"`
}

type Result struct {
	SourceID    int64
	Disposition Disposition
	Reason      string
	Fact        *OutboundTaskHistoryFact
}

// History preserves source order and count. It is not an import command.
type History struct{ Tasks []Result }

func (h History) SourceCount() int { return len(h.Tasks) }

func (h History) TerminalCount() int {
	count := 0
	for _, task := range h.Tasks {
		if task.Disposition == DispositionCandidate || task.Disposition == DispositionQuarantine {
			count++
		}
	}
	return count
}

type sourceJSON struct {
	ID              int64     `json:"id"`
	TaskType        string    `json:"task_type"`
	RequestPayload  string    `json:"request_payload"`
	ResponsePayload string    `json:"response_payload"`
	WeComTaskID     *string   `json:"wecom_task_id"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	TraceID         string    `json:"trace_id"`
	BroadcastJobID  *int64    `json:"broadcast_job_id"`
}

var manifestFields = []string{
	"id", "task_type", "request_payload", "response_payload", "wecom_task_id", "status", "created_at", "trace_id", "broadcast_job_id",
}

var nullableFields = map[string]bool{
	"wecom_task_id": true, "broadcast_job_id": true,
}

// AdaptHistory accepts only authenticated immutable archive rows. A redacted
// root remains visible in RedactedRoots and its private digest is explicitly a
// digest of the redacted archive representation, never a recovered plaintext.
func AdaptHistory(rows []v1archive.ArchivedRow) History {
	history := History{Tasks: make([]Result, len(rows))}
	for index, row := range rows {
		history.Tasks[index] = adapt(row, int64(index+1))
	}
	quarantineDuplicateIDs(history.Tasks)
	return history
}

func adapt(row v1archive.ArchivedRow, ordinal int64) Result {
	fields, envelope, roots, reason := archiveFields(row, ordinal)
	if reason != "" {
		return quarantine(0, reason)
	}
	var value sourceJSON
	if !decodeExact(fields, row.Payload, &value) || value.CreatedAt.IsZero() {
		return quarantine(sourceID(fields), ReasonInvalidSourcePayload)
	}
	fact := OutboundTaskHistoryFact{
		SourceID: value.ID, TaskType: value.TaskType, Status: value.Status, CreatedAt: utcMicro(value.CreatedAt),
		RequestPayloadDigest:  fieldDigest("request_payload", fields["request_payload"]),
		ResponsePayloadDigest: fieldDigest("response_payload", fields["response_payload"]),
		TraceIDDigest:         fieldDigest("trace_id", fields["trace_id"]),
		LegacyBroadcastJobID:  cloneInt64(value.BroadcastJobID),
		Source:                envelope,
		RedactedRoots:         roots,
	}
	if value.WeComTaskID != nil {
		digest := fieldDigest("wecom_task_id", fields["wecom_task_id"])
		fact.WeComTaskIDDigest = &digest
	}
	return Result{SourceID: value.ID, Disposition: DispositionCandidate, Fact: &fact}
}

func archiveFields(row v1archive.ArchivedRow, ordinal int64) (map[string]json.RawMessage, SourceEnvelope, []string, string) {
	zero := [sha256.Size]byte{}
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != OutboundTasksTableID || row.SourceOrdinal != ordinal ||
		row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, nil, ReasonInvalidArchiveRow
	}
	fields, ok := object(row.Payload)
	if !ok || !hasManifestShape(fields) {
		return nil, SourceEnvelope{}, nil, ReasonInvalidSourcePayload
	}
	roots, ok := redactedRoots(row.RedactedFields)
	if !ok {
		return nil, SourceEnvelope{}, nil, ReasonUnknownRedactedField
	}
	return fields, SourceEnvelope{
		SourceKeyDigest: OpaqueDigest(row.SourceKeyHMAC),
		PayloadDigest:   OpaqueDigest(row.PayloadHMAC),
		FieldDigest:     OpaqueDigest(row.FieldHMAC),
	}, roots, ""
}

func object(payload []byte) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	fields := map[string]json.RawMessage{}
	if decoder.Decode(&fields) != nil || fields == nil {
		return nil, false
	}
	var extra any
	return fields, errors.Is(decoder.Decode(&extra), io.EOF)
}

func hasManifestShape(fields map[string]json.RawMessage) bool {
	if len(fields) != len(manifestFields) {
		return false
	}
	for _, field := range manifestFields {
		value, found := fields[field]
		if !found || (!nullableFields[field] && isNull(value)) {
			return false
		}
	}
	return true
}

func decodeExact(fields map[string]json.RawMessage, payload []byte, target any) bool {
	if !hasManifestShape(fields) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func redactedRoots(paths []string) ([]string, bool) {
	roots := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		root := path
		if index := strings.IndexAny(root, ".["); index >= 0 {
			root = root[:index]
		}
		if !contains(manifestFields, root) {
			return nil, false
		}
		roots[root] = struct{}{}
	}
	result := make([]string, 0, len(roots))
	for root := range roots {
		result = append(result, root)
	}
	sort.Strings(result)
	return result, true
}

func sourceID(fields map[string]json.RawMessage) int64 {
	value, found := fields["id"]
	if !found || isNull(value) {
		return 0
	}
	var id int64
	if json.Unmarshal(value, &id) != nil {
		return 0
	}
	return id
}

func isNull(value json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(value), []byte("null")) }

func fieldDigest(field string, value json.RawMessage) OpaqueDigest {
	prefix := []byte("v1-outbound-task-history-field-v1\x00" + OutboundTasksTableID + "\x00" + field + "\x00")
	return OpaqueDigest(sha256.Sum256(append(prefix, value...)))
}

func utcMicro(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func quarantine(sourceID int64, reason string) Result {
	return Result{SourceID: sourceID, Disposition: DispositionQuarantine, Reason: reason}
}

func quarantineDuplicateIDs(results []Result) {
	counts := make(map[int64]int, len(results))
	for _, result := range results {
		if result.Disposition == DispositionCandidate && result.Fact != nil {
			counts[result.SourceID]++
		}
	}
	for index := range results {
		if results[index].Disposition == DispositionCandidate && results[index].Fact != nil && counts[results[index].SourceID] > 1 {
			results[index] = quarantine(results[index].SourceID, ReasonDuplicateSourceID)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
