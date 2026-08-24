package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	InternalEventSafeExportMaximumRows   = 10000
	InternalEventSafeExportDigestVersion = int16(1)
)

var (
	ErrInternalEventSafeExportInvalid     = errors.New("invalid internal event safe export")
	ErrInternalEventSafeExportConflict    = errors.New("internal event safe export conflict")
	ErrInternalEventSafeExportNotFound    = errors.New("internal event safe export not found")
	ErrInternalEventSafeExportUnavailable = errors.New("internal event safe export unavailable")
)

type InternalEventSafeExportFilter struct{ EventType, Consumer, Status string }
type InternalEventSafeExportCreate struct {
	ActorID        int64
	IdempotencyKey string
	Filter         InternalEventSafeExportFilter
}
type InternalEventSafeExport struct {
	ID          string    `json:"id"`
	RecordCount int       `json:"record_count"`
	Watermark   time.Time `json:"watermark"`
	CreatedAt   time.Time `json:"created_at"`
}
type InternalEventSafeExportRow struct {
	EventID          eventport.EventID
	EventType        string
	OccurredAt       time.Time
	Dispatched       bool
	Consumer, Status string
	AttemptCount     *int32
	CompletedAt      *time.Time
}
type InternalEventSafeExportReceipt struct {
	ID             int64
	PayloadDigest  [32]byte
	ResultDigest   [32]byte
	ResultSnapshot json.RawMessage
	Completed      bool
}
type InternalEventSafeExportSourceSnapshot struct {
	Watermark    time.Time
	UpperEventID int64
	Rows         []InternalEventSafeExportRow
}
type InternalEventSafeExportStoredSnapshot struct {
	Export                InternalEventSafeExport
	ActorID               int64
	FilterDigest          [32]byte
	UpperEventID          int64
	DigestVersion         int16
	RowsDigest            [32]byte
	ResultDigest          [32]byte
	Rows                  []InternalEventSafeExportRow
	ReceiptID             int64
	ReceiptPayloadDigest  [32]byte
	ReceiptResultDigest   [32]byte
	ReceiptResultSnapshot json.RawMessage
	AuditEventType        string
	AuditIdempotencyKey   string
	AuditOccurredAt       time.Time
	AuditPayload          json.RawMessage
}

type InternalEventSafeExportStore interface {
	ReserveInternalEventSafeExportReceipt(context.Context, int64, [32]byte, [32]byte, time.Time) (InternalEventSafeExportReceipt, bool, error)
	ReadInternalEventSafeExportSourceSnapshot(context.Context, InternalEventSafeExportFilter, int) (InternalEventSafeExportSourceSnapshot, error)
	CreateInternalEventSafeExport(context.Context, InternalEventSafeExport, int64, [32]byte, int64, [32]byte, [32]byte, []InternalEventSafeExportRow) error
	CompleteInternalEventSafeExportReceipt(context.Context, int64, string, [32]byte, json.RawMessage, time.Time) (InternalEventSafeExportReceipt, error)
	ReadInternalEventSafeExportSnapshot(context.Context, string, int64) (InternalEventSafeExportStoredSnapshot, error)
}

type InternalEventSafeExportService struct {
	uow    platformport.UnitOfWork
	store  InternalEventSafeExportStore
	events eventport.Appender
	now    func() time.Time
}

func NewInternalEventSafeExportService(uow platformport.UnitOfWork, store InternalEventSafeExportStore, events eventport.Appender) *InternalEventSafeExportService {
	return &InternalEventSafeExportService{uow: uow, store: store, events: events, now: time.Now}
}

func (s *InternalEventSafeExportService) Create(ctx context.Context, command InternalEventSafeExportCreate) (InternalEventSafeExport, error) {
	if s == nil || s.uow == nil || s.store == nil || s.events == nil || s.now == nil || ctx == nil || command.ActorID < 1 || !validInternalEventSafeExportKey(command.IdempotencyKey) || !validInternalEventSafeExportFilter(command.Filter) {
		return InternalEventSafeExport{}, ErrInternalEventSafeExportInvalid
	}
	createdAt := s.now().UTC().Truncate(time.Microsecond)
	if createdAt.IsZero() {
		return InternalEventSafeExport{}, ErrInternalEventSafeExportUnavailable
	}
	filterDigest, err := internalEventSafeExportFilterDigest(command.Filter)
	if err != nil {
		return InternalEventSafeExport{}, ErrInternalEventSafeExportUnavailable
	}
	keyDigest := sha256.Sum256([]byte(command.IdempotencyKey))
	id, err := newInternalEventSafeExportID()
	if err != nil {
		return InternalEventSafeExport{}, ErrInternalEventSafeExportUnavailable
	}
	result := InternalEventSafeExport{ID: id, CreatedAt: createdAt}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveInternalEventSafeExportReceipt(tx, command.ActorID, keyDigest, filterDigest, createdAt)
		if reserveErr != nil {
			return reserveErr
		}
		if receipt.ID < 1 || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], filterDigest[:]) != 1 {
			return ErrInternalEventSafeExportConflict
		}
		if !owned {
			if !receipt.Completed || !decodeInternalEventSafeExport(receipt.ResultSnapshot, &result) {
				return ErrInternalEventSafeExportUnavailable
			}
			stored, readErr := s.store.ReadInternalEventSafeExportSnapshot(tx, result.ID, command.ActorID)
			if readErr != nil || !verifyInternalEventSafeExportSnapshot(stored) || stored.Export != result || subtle.ConstantTimeCompare(stored.ResultDigest[:], receipt.ResultDigest[:]) != 1 {
				return errors.Join(ErrInternalEventSafeExportUnavailable, readErr)
			}
			return nil
		}

		source, sourceErr := s.store.ReadInternalEventSafeExportSourceSnapshot(tx, command.Filter, InternalEventSafeExportMaximumRows+1)
		if sourceErr != nil {
			return sourceErr
		}
		if len(source.Rows) > InternalEventSafeExportMaximumRows {
			return ErrInternalEventSafeExportConflict
		}
		if source.Watermark.IsZero() || source.UpperEventID < 0 {
			return ErrInternalEventSafeExportUnavailable
		}
		rowsDigest, digestErr := internalEventSafeExportRowsDigest(source.Rows)
		if digestErr != nil {
			return digestErr
		}
		result.Watermark = source.Watermark.UTC()
		result.RecordCount = len(source.Rows)
		resultDigest, digestErr := internalEventSafeExportResultDigest(result, command.ActorID, filterDigest, source.UpperEventID, rowsDigest)
		if digestErr != nil {
			return digestErr
		}
		if createErr := s.store.CreateInternalEventSafeExport(tx, result, command.ActorID, filterDigest, source.UpperEventID, rowsDigest, resultDigest, source.Rows); createErr != nil {
			return createErr
		}
		eventPayload, marshalErr := json.Marshal(internalEventSafeExportAuditPayload(result, command.ActorID, filterDigest, rowsDigest, resultDigest))
		if marshalErr != nil {
			return marshalErr
		}
		if _, appendErr := s.events.Append(tx, eventport.Event{Type: eventport.EvInternalEventSafeExportCreated, Payload: eventPayload, OccurredAt: createdAt, IdempotencyKey: internalEventSafeExportAuditKey(receipt.ID)}); appendErr != nil {
			return appendErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteInternalEventSafeExportReceipt(tx, receipt.ID, result.ID, resultDigest, snapshot, createdAt)
		var decoded InternalEventSafeExport
		if completeErr != nil || !completed.Completed || !decodeInternalEventSafeExport(completed.ResultSnapshot, &decoded) || decoded != result || subtle.ConstantTimeCompare(completed.ResultDigest[:], resultDigest[:]) != 1 {
			return errors.Join(ErrInternalEventSafeExportUnavailable, completeErr)
		}
		return nil
	})
	if err != nil {
		return InternalEventSafeExport{}, classifyInternalEventSafeExport(err)
	}
	return result, nil
}

func (s *InternalEventSafeExportService) Get(ctx context.Context, id string, actor int64) (InternalEventSafeExport, error) {
	stored, err := s.readVerified(ctx, id, actor)
	if err != nil {
		return InternalEventSafeExport{}, err
	}
	return stored.Export, nil
}

func (s *InternalEventSafeExportService) Download(ctx context.Context, id string, actor int64) (InternalEventSafeExport, []InternalEventSafeExportRow, error) {
	stored, err := s.readVerified(ctx, id, actor)
	if err != nil {
		return InternalEventSafeExport{}, nil, err
	}
	return stored.Export, stored.Rows, nil
}

func (s *InternalEventSafeExportService) readVerified(ctx context.Context, id string, actor int64) (InternalEventSafeExportStoredSnapshot, error) {
	if s == nil || s.uow == nil || s.store == nil || ctx == nil || actor < 1 || !validInternalEventSafeExportID(id) {
		return InternalEventSafeExportStoredSnapshot{}, ErrInternalEventSafeExportInvalid
	}
	var stored InternalEventSafeExportStoredSnapshot
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		stored, readErr = s.store.ReadInternalEventSafeExportSnapshot(tx, id, actor)
		if readErr == nil && !verifyInternalEventSafeExportSnapshot(stored) {
			return ErrInternalEventSafeExportUnavailable
		}
		return readErr
	})
	if err != nil {
		return InternalEventSafeExportStoredSnapshot{}, classifyInternalEventSafeExport(err)
	}
	return stored, nil
}

type internalEventSafeExportCanonicalRow struct {
	RowIndex     int     `json:"row_index"`
	EventID      int64   `json:"event_id"`
	EventType    string  `json:"event_type"`
	OccurredAt   string  `json:"occurred_at"`
	Dispatched   bool    `json:"dispatched"`
	Consumer     string  `json:"consumer"`
	Status       string  `json:"status"`
	AttemptCount *int32  `json:"attempt_count"`
	CompletedAt  *string `json:"completed_at"`
}

type internalEventSafeExportCanonicalResult struct {
	DigestVersion int16  `json:"digest_version"`
	ExportID      string `json:"export_id"`
	ActorID       int64  `json:"actor_id"`
	FilterDigest  string `json:"filter_digest"`
	Watermark     string `json:"watermark"`
	UpperEventID  int64  `json:"upper_event_id"`
	RecordCount   int    `json:"record_count"`
	RowsDigest    string `json:"rows_digest"`
	CreatedAt     string `json:"created_at"`
}

type internalEventSafeExportCanonicalFilter struct {
	DigestVersion int16  `json:"digest_version"`
	EventType     string `json:"event_type"`
	Consumer      string `json:"consumer"`
	Status        string `json:"status"`
}

type internalEventSafeExportAudit struct {
	DigestVersion int16  `json:"digest_version"`
	ExportID      string `json:"export_id"`
	ActorID       int64  `json:"actor_id"`
	RecordCount   int    `json:"record_count"`
	FilterDigest  string `json:"filter_digest"`
	RowsDigest    string `json:"rows_digest"`
	ResultDigest  string `json:"result_digest"`
}

func internalEventSafeExportRowsDigest(rows []InternalEventSafeExportRow) ([32]byte, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte("aicrm/internal-event-safe-export/rows/v1\x00"))
	seen := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		if !validInternalEventSafeExportRow(row) {
			return [32]byte{}, ErrInternalEventSafeExportUnavailable
		}
		key := strconv.FormatInt(int64(row.EventID), 10) + "\x00" + row.Consumer
		if _, exists := seen[key]; exists {
			return [32]byte{}, ErrInternalEventSafeExportUnavailable
		}
		seen[key] = struct{}{}
		var completedAt *string
		if row.CompletedAt != nil {
			value := row.CompletedAt.UTC().Format(time.RFC3339Nano)
			completedAt = &value
		}
		encoded, err := json.Marshal(internalEventSafeExportCanonicalRow{RowIndex: index + 1, EventID: int64(row.EventID), EventType: row.EventType, OccurredAt: row.OccurredAt.UTC().Format(time.RFC3339Nano), Dispatched: row.Dispatched, Consumer: row.Consumer, Status: row.Status, AttemptCount: row.AttemptCount, CompletedAt: completedAt})
		if err != nil {
			return [32]byte{}, ErrInternalEventSafeExportUnavailable
		}
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(encoded)))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write(encoded)
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func internalEventSafeExportFilterDigest(filter InternalEventSafeExportFilter) ([32]byte, error) {
	encoded, err := json.Marshal(internalEventSafeExportCanonicalFilter{DigestVersion: InternalEventSafeExportDigestVersion, EventType: filter.EventType, Consumer: filter.Consumer, Status: filter.Status})
	if err != nil {
		return [32]byte{}, ErrInternalEventSafeExportUnavailable
	}
	return sha256.Sum256(append([]byte("aicrm/internal-event-safe-export/filter/v1\x00"), encoded...)), nil
}

func internalEventSafeExportResultDigest(export InternalEventSafeExport, actor int64, filterDigest [32]byte, upperEventID int64, rowsDigest [32]byte) ([32]byte, error) {
	if actor < 1 || upperEventID < 0 || !decodeInternalEventSafeExportMust(export) {
		return [32]byte{}, ErrInternalEventSafeExportUnavailable
	}
	encoded, err := json.Marshal(internalEventSafeExportCanonicalResult{DigestVersion: InternalEventSafeExportDigestVersion, ExportID: export.ID, ActorID: actor, FilterDigest: hex.EncodeToString(filterDigest[:]), Watermark: export.Watermark.UTC().Format(time.RFC3339Nano), UpperEventID: upperEventID, RecordCount: export.RecordCount, RowsDigest: hex.EncodeToString(rowsDigest[:]), CreatedAt: export.CreatedAt.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return [32]byte{}, ErrInternalEventSafeExportUnavailable
	}
	return sha256.Sum256(append([]byte("aicrm/internal-event-safe-export/result/v1\x00"), encoded...)), nil
}

func verifyInternalEventSafeExportSnapshot(stored InternalEventSafeExportStoredSnapshot) bool {
	if stored.ActorID < 1 || stored.UpperEventID < 0 || stored.DigestVersion != InternalEventSafeExportDigestVersion || stored.Export.RecordCount != len(stored.Rows) || stored.ReceiptID < 1 || !decodeInternalEventSafeExportMust(stored.Export) {
		return false
	}
	if subtle.ConstantTimeCompare(stored.FilterDigest[:], stored.ReceiptPayloadDigest[:]) != 1 || subtle.ConstantTimeCompare(stored.ResultDigest[:], stored.ReceiptResultDigest[:]) != 1 {
		return false
	}
	rowsDigest, err := internalEventSafeExportRowsDigest(stored.Rows)
	if err != nil || subtle.ConstantTimeCompare(rowsDigest[:], stored.RowsDigest[:]) != 1 {
		return false
	}
	resultDigest, err := internalEventSafeExportResultDigest(stored.Export, stored.ActorID, stored.FilterDigest, stored.UpperEventID, stored.RowsDigest)
	if err != nil || subtle.ConstantTimeCompare(resultDigest[:], stored.ResultDigest[:]) != 1 {
		return false
	}
	var receiptResult InternalEventSafeExport
	if !decodeInternalEventSafeExport(stored.ReceiptResultSnapshot, &receiptResult) || receiptResult != stored.Export {
		return false
	}
	expectedAudit := internalEventSafeExportAuditPayload(stored.Export, stored.ActorID, stored.FilterDigest, stored.RowsDigest, stored.ResultDigest)
	var actualAudit internalEventSafeExportAudit
	if !decodeStrictJSON(stored.AuditPayload, &actualAudit) || actualAudit != expectedAudit {
		return false
	}
	return stored.AuditEventType == eventport.EvInternalEventSafeExportCreated && stored.AuditIdempotencyKey == internalEventSafeExportAuditKey(stored.ReceiptID) && stored.AuditOccurredAt.UTC().Equal(stored.Export.CreatedAt.UTC())
}

func internalEventSafeExportAuditPayload(export InternalEventSafeExport, actor int64, filterDigest, rowsDigest, resultDigest [32]byte) internalEventSafeExportAudit {
	return internalEventSafeExportAudit{DigestVersion: InternalEventSafeExportDigestVersion, ExportID: export.ID, ActorID: actor, RecordCount: export.RecordCount, FilterDigest: hex.EncodeToString(filterDigest[:]), RowsDigest: hex.EncodeToString(rowsDigest[:]), ResultDigest: hex.EncodeToString(resultDigest[:])}
}

func internalEventSafeExportAuditKey(receiptID int64) string {
	return "internal-event-safe-export:" + strconv.FormatInt(receiptID, 10)
}

func validInternalEventSafeExportRow(row InternalEventSafeExportRow) bool {
	if row.EventID < 1 || !validAdminReadText(row.EventType) || row.OccurredAt.IsZero() {
		return false
	}
	if row.Consumer == "" {
		return row.Status == "" && row.AttemptCount == nil && row.CompletedAt == nil
	}
	if !validAdminReadText(row.Consumer) || !adminReadBindingMatches(row.EventType, row.Consumer) || row.AttemptCount == nil {
		return false
	}
	return validAdminReadDelivery(eventport.AdminReadDelivery{EventID: row.EventID, Consumer: row.Consumer, Status: row.Status, AttemptCount: *row.AttemptCount, CompletedAt: row.CompletedAt})
}

func validInternalEventSafeExportFilter(f InternalEventSafeExportFilter) bool {
	if f.EventType != "" && !validAdminReadText(f.EventType) {
		return false
	}
	if f.Consumer != "" {
		if _, ok := eventport.AdminReadBindingForConsumer(f.Consumer); !ok {
			return false
		}
	}
	return f.Status == "" || validAdminReadStatus(f.Status)
}

func validInternalEventSafeExportKey(v string) bool {
	return utf8.ValidString(v) && strings.TrimSpace(v) == v && utf8.RuneCountInString(v) >= 16 && utf8.RuneCountInString(v) <= 128
}
func validInternalEventSafeExportID(v string) bool {
	if len(v) != 36 || !strings.HasPrefix(v, "ese_") {
		return false
	}
	_, err := hex.DecodeString(v[4:])
	return err == nil
}
func newInternalEventSafeExportID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "ese_" + hex.EncodeToString(raw), nil
}
func decodeInternalEventSafeExport(raw json.RawMessage, result *InternalEventSafeExport) bool {
	return result != nil && decodeStrictJSON(raw, result) && decodeInternalEventSafeExportMust(*result)
}
func decodeInternalEventSafeExportMust(result InternalEventSafeExport) bool {
	return validInternalEventSafeExportID(result.ID) && result.RecordCount >= 0 && result.RecordCount <= InternalEventSafeExportMaximumRows && !result.Watermark.IsZero() && !result.CreatedAt.IsZero()
}
func decodeStrictJSON(raw []byte, value any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}
func classifyInternalEventSafeExport(err error) error {
	switch {
	case errors.Is(err, ErrInternalEventSafeExportInvalid), errors.Is(err, ErrInternalEventSafeExportConflict), errors.Is(err, ErrInternalEventSafeExportNotFound), errors.Is(err, ErrInternalEventSafeExportUnavailable):
		return err
	default:
		return errors.Join(ErrInternalEventSafeExportUnavailable, err)
	}
}
