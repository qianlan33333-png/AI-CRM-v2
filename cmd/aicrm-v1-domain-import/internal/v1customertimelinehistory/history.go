// Package v1customertimelinehistory decodes frozen V1 customer timeline facts.
// It does not create a current customer event, resolve unionid, or run an
// outbox, queue, or Provider effect.
package v1customertimelinehistory

import (
	"bytes"
	"context"
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
	TableID        = "public/customer_timeline_event_next"
	FixedBatchSize = 250

	DispositionCandidate  = "candidate"
	DispositionQuarantine = "quarantine"
	ReasonFieldRedacted   = "customer_timeline_field_redacted"
)

var (
	ErrArchiveRow            = errors.New("customer timeline archive row invalid")
	ErrFact                  = errors.New("customer timeline source fact invalid")
	ErrRequiredFieldRedacted = errors.New("customer timeline required source field redacted")
	ErrInvalidInput          = errors.New("customer timeline stream input invalid")
)

// ArchiveSource is the immutable, read-only V2 archive surface used by the
// later importer. One table is streamed in source-ordinal order.
type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

// TerminalVerifier lets the later journal owner validate each preserved source
// terminal without putting storage dependencies in this source package.
type TerminalVerifier interface {
	VerifyCustomerTimelineTerminal(context.Context, SourceEnvelope, string, string) error
}

// BatchConsumer receives no more than FixedBatchSize outcomes. Nil means a
// complete read-only preflight without retaining a table-sized result.
type BatchConsumer interface {
	ConsumeCustomerTimelineBatch(context.Context, Batch) error
}

type SourceEnvelope struct {
	SourceOrdinal int64
	SourceKeyHMAC [sha256.Size]byte `json:"-"`
	PayloadHMAC   [sha256.Size]byte `json:"-"`
	FieldHMAC     [sha256.Size]byte `json:"-"`
}

// TimelineEventFact preserves all eleven archived source columns. Customer
// association and presentation classification intentionally remain for the
// later formal owner; no current timeline projection is inferred here.
type TimelineEventFact struct {
	Source       SourceEnvelope  `json:"-"`
	SourceID     int64           `json:"source_id"`
	EventID      string          `json:"-"`
	EventType    string          `json:"event_type"`
	EventTime    time.Time       `json:"event_time"`
	Title        string          `json:"-"`
	Summary      string          `json:"-"`
	SourceTable  string          `json:"source_table"`
	SourceValue  string          `json:"-"`
	MetadataJSON json.RawMessage `json:"-"`
	CreatedAt    time.Time       `json:"created_at"`
	UnionID      string          `json:"-"`
}

type Result struct {
	Source      SourceEnvelope
	Fact        *TimelineEventFact
	Disposition string
	Reason      string
}

type Batch struct {
	Rows []Result
}

type Summary struct {
	Rows        int64
	Candidates  int64
	Quarantined int64
}

type timelineEventSource struct {
	ID           int64           `json:"id"`
	EventID      string          `json:"event_id"`
	EventType    string          `json:"event_type"`
	EventTime    time.Time       `json:"event_time"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	SourceTable  string          `json:"source_table"`
	SourceID     string          `json:"source_id"`
	MetadataJSON json.RawMessage `json:"metadata_json"`
	CreatedAt    time.Time       `json:"created_at"`
	UnionID      string          `json:"unionid"`
}

var timelineFields = []string{
	"id", "event_id", "event_type", "event_time", "title", "summary", "source_table", "source_id", "metadata_json", "created_at", "unionid",
}

// metadata_json is SQL NOT NULL but JSON literal null is a retained JSON value,
// distinct from a missing field and therefore accepted here.
var timelineJSONLiteralNull = map[string]bool{"metadata_json": true}

// AdaptTimelineEvent authenticates and decodes a single sealed source row. A
// redacted source is explicitly isolated rather than being completed with a
// guessed value.
func AdaptTimelineEvent(row v1archive.ArchivedRow, sourceHMACKey []byte, expectedOrdinal int64) (TimelineEventFact, error) {
	fields, envelope, err := archiveFields(row, expectedOrdinal, sourceHMACKey)
	if err != nil {
		return TimelineEventFact{}, err
	}
	var value timelineEventSource
	if err := decodeExact(fields, row.Payload, &value); err != nil || !jsonValueValid(fields["metadata_json"]) || value.EventTime.IsZero() || value.CreatedAt.IsZero() {
		return TimelineEventFact{}, ErrFact
	}
	if len(row.RedactedFields) != 0 {
		return TimelineEventFact{}, ErrRequiredFieldRedacted
	}
	return TimelineEventFact{
		Source: envelope, SourceID: value.ID, EventID: value.EventID, EventType: value.EventType,
		EventTime: utcMicro(value.EventTime), Title: value.Title, Summary: value.Summary,
		SourceTable: value.SourceTable, SourceValue: value.SourceID, MetadataJSON: cloneJSON(fields["metadata_json"]),
		CreatedAt: utcMicro(value.CreatedAt), UnionID: value.UnionID,
	}, nil
}

// Stream authenticates every source row before it reaches the supplied
// terminal verifier or batch consumer. It has no full-table map or sort.
func Stream(ctx context.Context, archive ArchiveSource, archiveRun string, sourceHMACKey []byte, verifier TerminalVerifier, consumer BatchConsumer) (Summary, error) {
	if ctx == nil || archive == nil || verifier == nil || archiveRun == "" || strings.TrimSpace(archiveRun) != archiveRun || len(sourceHMACKey) < sha256.Size {
		return Summary{}, ErrInvalidInput
	}
	batch := Batch{Rows: make([]Result, 0, FixedBatchSize)}
	var summary Summary
	ordinal := int64(1)
	err := archive.EachTableRow(ctx, archiveRun, TableID, func(row v1archive.ArchivedRow) error {
		fact, adaptErr := AdaptTimelineEvent(row, sourceHMACKey, ordinal)
		result := Result{Source: sourceEnvelope(row), Disposition: DispositionCandidate}
		switch {
		case adaptErr == nil:
			result.Fact = &fact
		case errors.Is(adaptErr, ErrRequiredFieldRedacted):
			result.Disposition = DispositionQuarantine
			result.Reason = ReasonFieldRedacted
		default:
			return adaptErr
		}
		if err := verifier.VerifyCustomerTimelineTerminal(ctx, result.Source, result.Disposition, result.Reason); err != nil {
			return err
		}
		summary.Rows++
		if result.Disposition == DispositionCandidate {
			summary.Candidates++
		} else {
			summary.Quarantined++
		}
		batch.Rows = append(batch.Rows, result)
		ordinal++
		if len(batch.Rows) == FixedBatchSize {
			return flush(ctx, consumer, &batch)
		}
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	if err := flush(ctx, consumer, &batch); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func archiveFields(row v1archive.ArchivedRow, expectedOrdinal int64, key []byte) (map[string]json.RawMessage, SourceEnvelope, error) {
	zero := [sha256.Size]byte{}
	if len(key) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != TableID || row.SourceOrdinal != expectedOrdinal || row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	canonical, roots, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) || !sameStrings(roots, row.RedactedFields) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	payload, err := v1archive.PayloadHMAC(key, archiveTableName(), canonical)
	if err != nil || !hmac.Equal(payload[:], row.PayloadHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	field, err := v1archive.FieldHMAC(key, archiveTableName(), roots)
	if err != nil || !hmac.Equal(field[:], row.FieldHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	fields, err := rawFields(row.Payload)
	if err != nil {
		return nil, SourceEnvelope{}, err
	}
	id, found := fields["id"]
	if !found || !json.Valid(id) || isNull(id) {
		return nil, SourceEnvelope{}, ErrFact
	}
	keyJSON, err := json.Marshal([]json.RawMessage{id})
	if err != nil {
		return nil, SourceEnvelope{}, ErrFact
	}
	source, err := v1archive.SourceKeyHMAC(key, archiveTableName(), keyJSON)
	if err != nil || !hmac.Equal(source[:], row.SourceKeyHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	return fields, sourceEnvelope(row), nil
}

func decodeExact(fields map[string]json.RawMessage, payload []byte, target any) error {
	if len(fields) != len(timelineFields) {
		return ErrFact
	}
	for _, name := range timelineFields {
		value, found := fields[name]
		if !found || !json.Valid(value) || (!timelineJSONLiteralNull[name] && isNull(value)) {
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

func flush(ctx context.Context, consumer BatchConsumer, batch *Batch) error {
	if batch == nil || len(batch.Rows) == 0 {
		return nil
	}
	if consumer != nil {
		if err := consumer.ConsumeCustomerTimelineBatch(ctx, *batch); err != nil {
			return err
		}
	}
	batch.Rows = batch.Rows[:0]
	return nil
}

func sourceEnvelope(row v1archive.ArchivedRow) SourceEnvelope {
	return SourceEnvelope{SourceOrdinal: row.SourceOrdinal, SourceKeyHMAC: row.SourceKeyHMAC, PayloadHMAC: row.PayloadHMAC, FieldHMAC: row.FieldHMAC}
}

func archiveTableName() string                        { return strings.TrimPrefix(TableID, "public/") }
func cloneJSON(value json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), value...) }
func utcMicro(value time.Time) time.Time              { return value.UTC().Truncate(time.Microsecond) }
func isNull(value json.RawMessage) bool               { return bytes.Equal(bytes.TrimSpace(value), []byte("null")) }
func jsonValueValid(value json.RawMessage) bool       { return isNull(value) || json.Valid(value) }

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
