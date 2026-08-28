// Package v1cycleobservationhistory decodes frozen V1 operation-cycle
// observations into inert source facts. It does not restore a run, schedule a
// cycle, access a reference href, or create a current V2 relation.
package v1cycleobservationhistory

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	MetricsTableID    = "public/operation_cycle_metrics"
	ReferencesTableID = "public/operation_cycle_references"
)

var (
	ErrArchiveRow            = errors.New("cycle observation archive row invalid")
	ErrFact                  = errors.New("cycle observation fact invalid")
	ErrRequiredFieldRedacted = errors.New("cycle observation required source field redacted")
)

// SourceEnvelope binds a fact to exactly one immutable archive row. It does
// not identify a V2 row or a current operation-cycle run.
type SourceEnvelope struct {
	SourceKeyDigest [sha256.Size]byte `json:"-"`
	PayloadDigest   [sha256.Size]byte `json:"-"`
	FieldDigest     [sha256.Size]byte `json:"-"`
}

// MetricFact is one historical measurement. Its source run and snapshot IDs
// remain signed V1 facts and never become current V2 foreign keys.
type MetricFact struct {
	Source            SourceEnvelope  `json:"-"`
	SourceID          int64           `json:"source_id"`
	RunID             int64           `json:"run_id"`
	MetricKey         string          `json:"metric_key"`
	Label             string          `json:"label"`
	Numerator         *float64        `json:"numerator"`
	Denominator       *float64        `json:"denominator"`
	Value             *float64        `json:"value"`
	Unit              string          `json:"unit"`
	ObservationWindow string          `json:"observation_window"`
	DataSource        string          `json:"data_source"`
	DataQuality       string          `json:"data_quality"`
	LimitationsJSON   json.RawMessage `json:"-"`
	IsCausal          bool            `json:"is_causal"`
	ValueStatus       string          `json:"value_status"`
	LastSnapshotID    int64           `json:"last_snapshot_id"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// ReferenceFact is one historical reference. Href is source-only evidence and
// deliberately never becomes a link or a network request.
type ReferenceFact struct {
	Source            SourceEnvelope `json:"-"`
	SourceID          int64          `json:"source_id"`
	RunID             int64          `json:"run_id"`
	ReferenceKey      string         `json:"reference_key"`
	ReferenceType     string         `json:"reference_type"`
	Label             string         `json:"label"`
	SourceSystem      string         `json:"source_system"`
	ReferenceSourceID string         `json:"source_id_value"`
	Href              string         `json:"-"`
	EvidenceHash      string         `json:"evidence_hash"`
	DataStatus        string         `json:"data_status"`
	LastSnapshotID    int64          `json:"last_snapshot_id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type metricSource struct {
	ID                int64           `json:"id"`
	RunID             int64           `json:"run_id"`
	MetricKey         string          `json:"metric_key"`
	Label             string          `json:"label"`
	Numerator         *float64        `json:"numerator"`
	Denominator       *float64        `json:"denominator"`
	Value             *float64        `json:"value"`
	Unit              string          `json:"unit"`
	ObservationWindow string          `json:"observation_window"`
	DataSource        string          `json:"data_source"`
	DataQuality       string          `json:"data_quality"`
	LimitationsJSON   json.RawMessage `json:"limitations_json"`
	IsCausal          bool            `json:"is_causal"`
	ValueStatus       string          `json:"value_status"`
	LastSnapshotID    int64           `json:"last_snapshot_id"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type referenceSource struct {
	ID             int64     `json:"id"`
	RunID          int64     `json:"run_id"`
	ReferenceKey   string    `json:"reference_key"`
	ReferenceType  string    `json:"reference_type"`
	Label          string    `json:"label"`
	SourceSystem   string    `json:"source_system"`
	SourceID       string    `json:"source_id"`
	Href           string    `json:"href"`
	EvidenceHash   string    `json:"evidence_hash"`
	DataStatus     string    `json:"data_status"`
	LastSnapshotID int64     `json:"last_snapshot_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

var metricFields = []string{
	"id", "run_id", "metric_key", "label", "numerator", "denominator", "value", "unit", "observation_window", "data_source", "data_quality", "limitations_json", "is_causal", "value_status", "last_snapshot_id", "created_at", "updated_at",
}

var referenceFields = []string{
	"id", "run_id", "reference_key", "reference_type", "label", "source_system", "source_id", "href", "evidence_hash", "data_status", "last_snapshot_id", "created_at", "updated_at",
}

var metricNullable = map[string]bool{"numerator": true, "denominator": true, "value": true}

// AdaptMetric authenticates and preserves all seventeen metric columns. It
// intentionally applies no status, run, snapshot, or causal business rules.
func AdaptMetric(row v1archive.ArchivedRow, sourceHMACKey []byte) (MetricFact, error) {
	fields, source, err := archiveFields(row, MetricsTableID, sourceHMACKey)
	if err != nil {
		return MetricFact{}, err
	}
	var value metricSource
	if err = decodeExact(fields, row.Payload, &value, metricFields, metricNullable); err != nil || !validRequiredTimes(value.CreatedAt, value.UpdatedAt) || !json.Valid(value.LimitationsJSON) || isNull(value.LimitationsJSON) {
		return MetricFact{}, ErrFact
	}
	return MetricFact{
		Source: source, SourceID: value.ID, RunID: value.RunID, MetricKey: value.MetricKey, Label: value.Label,
		Numerator: cloneFloat64(value.Numerator), Denominator: cloneFloat64(value.Denominator), Value: cloneFloat64(value.Value), Unit: value.Unit,
		ObservationWindow: value.ObservationWindow, DataSource: value.DataSource, DataQuality: value.DataQuality, LimitationsJSON: cloneJSON(value.LimitationsJSON),
		IsCausal: value.IsCausal, ValueStatus: value.ValueStatus, LastSnapshotID: value.LastSnapshotID,
		CreatedAt: utcMicro(value.CreatedAt), UpdatedAt: utcMicro(value.UpdatedAt),
	}, nil
}

// AdaptReference authenticates and preserves all thirteen reference columns.
// In particular, href remains a private inert source value and is not parsed.
func AdaptReference(row v1archive.ArchivedRow, sourceHMACKey []byte) (ReferenceFact, error) {
	fields, source, err := archiveFields(row, ReferencesTableID, sourceHMACKey)
	if err != nil {
		return ReferenceFact{}, err
	}
	var value referenceSource
	if err = decodeExact(fields, row.Payload, &value, referenceFields, nil); err != nil || !validRequiredTimes(value.CreatedAt, value.UpdatedAt) {
		return ReferenceFact{}, ErrFact
	}
	return ReferenceFact{
		Source: source, SourceID: value.ID, RunID: value.RunID, ReferenceKey: value.ReferenceKey, ReferenceType: value.ReferenceType,
		Label: value.Label, SourceSystem: value.SourceSystem, ReferenceSourceID: value.SourceID, Href: value.Href, EvidenceHash: value.EvidenceHash,
		DataStatus: value.DataStatus, LastSnapshotID: value.LastSnapshotID, CreatedAt: utcMicro(value.CreatedAt), UpdatedAt: utcMicro(value.UpdatedAt),
	}, nil
}

func archiveFields(row v1archive.ArchivedRow, tableID string, key []byte) (map[string]json.RawMessage, SourceEnvelope, error) {
	zero := [sha256.Size]byte{}
	if len(key) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal < 1 || row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	canonical, roots, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) || !sameStrings(roots, row.RedactedFields) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	if len(roots) != 0 {
		return nil, SourceEnvelope{}, ErrRequiredFieldRedacted
	}
	table := strings.TrimPrefix(tableID, "public/")
	payload, err := v1archive.PayloadHMAC(key, table, row.Payload)
	if err != nil || !hmac.Equal(payload[:], row.PayloadHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	field, err := v1archive.FieldHMAC(key, table, row.RedactedFields)
	if err != nil || !hmac.Equal(field[:], row.FieldHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	fields, err := rawFields(row.Payload)
	if err != nil {
		return nil, SourceEnvelope{}, err
	}
	keyJSON, err := json.Marshal([]json.RawMessage{fields["id"]})
	if err != nil {
		return nil, SourceEnvelope{}, ErrFact
	}
	source, err := v1archive.SourceKeyHMAC(key, table, keyJSON)
	if err != nil || !hmac.Equal(source[:], row.SourceKeyHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	return fields, SourceEnvelope{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, FieldDigest: row.FieldHMAC}, nil
}

func decodeExact(fields map[string]json.RawMessage, payload []byte, target any, names []string, nullable map[string]bool) error {
	if len(fields) != len(names) {
		return ErrFact
	}
	for _, name := range names {
		raw, found := fields[name]
		if !found || !json.Valid(raw) || (!nullable[name] && isNull(raw)) {
			return ErrFact
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(target); err != nil {
		return ErrFact
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrFact
	}
	return nil
}

func rawFields(payload []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	fields := map[string]json.RawMessage{}
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, ErrFact
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, ErrFact
	}
	return fields, nil
}

func validRequiredTimes(values ...time.Time) bool {
	for _, value := range values {
		if value.IsZero() {
			return false
		}
	}
	return true
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneJSON(value json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), value...) }
func utcMicro(value time.Time) time.Time              { return value.UTC().Truncate(time.Microsecond) }
func isNull(value json.RawMessage) bool               { return bytes.Equal(bytes.TrimSpace(value), []byte("null")) }

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
