package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const InternalEventSafeExportMaximumRows = 10000

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
	ResultSnapshot json.RawMessage
	Completed      bool
}

type InternalEventSafeExportStore interface {
	ReserveInternalEventSafeExportReceipt(context.Context, int64, [32]byte, [32]byte, time.Time) (InternalEventSafeExportReceipt, bool, error)
	CreateInternalEventSafeExport(context.Context, InternalEventSafeExport, int64, [32]byte, InternalEventSafeExportFilter) ([]InternalEventSafeExportRow, error)
	CompleteInternalEventSafeExportReceipt(context.Context, int64, string, json.RawMessage, time.Time) (InternalEventSafeExportReceipt, error)
	ReadInternalEventSafeExport(context.Context, string, int64) (InternalEventSafeExport, error)
	ReadInternalEventSafeExportRows(context.Context, string, int64) ([]InternalEventSafeExportRow, error)
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
	now := s.now().UTC()
	if now.IsZero() {
		return InternalEventSafeExport{}, ErrInternalEventSafeExportUnavailable
	}
	payload, err := json.Marshal(command.Filter)
	if err != nil {
		return InternalEventSafeExport{}, ErrInternalEventSafeExportUnavailable
	}
	key := sha256.Sum256([]byte(command.IdempotencyKey))
	digest := sha256.Sum256(payload)
	id, err := newInternalEventSafeExportID()
	if err != nil {
		return InternalEventSafeExport{}, ErrInternalEventSafeExportUnavailable
	}
	result := InternalEventSafeExport{ID: id, Watermark: now, CreatedAt: now}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveInternalEventSafeExportReceipt(tx, command.ActorID, key, digest, now)
		if reserveErr != nil {
			return reserveErr
		}
		if receipt.ID < 1 || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], digest[:]) != 1 {
			return ErrInternalEventSafeExportConflict
		}
		if !owned {
			if !receipt.Completed || !decodeInternalEventSafeExport(receipt.ResultSnapshot, &result) {
				return ErrInternalEventSafeExportUnavailable
			}
			return nil
		}
		rows, createErr := s.store.CreateInternalEventSafeExport(tx, result, command.ActorID, digest, command.Filter)
		if createErr != nil {
			return createErr
		}
		if len(rows) > InternalEventSafeExportMaximumRows {
			return ErrInternalEventSafeExportConflict
		}
		result.RecordCount = len(rows)
		eventPayload, marshalErr := json.Marshal(struct {
			ExportID     string `json:"export_id"`
			RecordCount  int    `json:"record_count"`
			FilterDigest string `json:"filter_digest"`
		}{result.ID, result.RecordCount, hex.EncodeToString(digest[:])})
		if marshalErr != nil {
			return marshalErr
		}
		if _, appendErr := s.events.Append(tx, eventport.Event{Type: eventport.EvInternalEventSafeExportCreated, Payload: eventPayload, OccurredAt: now, IdempotencyKey: "internal-event-safe-export:" + strconv.FormatInt(receipt.ID, 10)}); appendErr != nil {
			return appendErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteInternalEventSafeExportReceipt(tx, receipt.ID, result.ID, snapshot, now)
		var decoded InternalEventSafeExport
		if completeErr != nil || !completed.Completed || !decodeInternalEventSafeExport(completed.ResultSnapshot, &decoded) || decoded != result {
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
	if s == nil || s.uow == nil || s.store == nil || ctx == nil || actor < 1 || !validInternalEventSafeExportID(id) {
		return InternalEventSafeExport{}, ErrInternalEventSafeExportInvalid
	}
	var result InternalEventSafeExport
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		result, e = s.store.ReadInternalEventSafeExport(tx, id, actor)
		return e
	})
	if err != nil {
		return InternalEventSafeExport{}, classifyInternalEventSafeExport(err)
	}
	return result, nil
}
func (s *InternalEventSafeExportService) Download(ctx context.Context, id string, actor int64) (InternalEventSafeExport, []InternalEventSafeExportRow, error) {
	if s == nil || s.uow == nil || s.store == nil || ctx == nil || actor < 1 || !validInternalEventSafeExportID(id) {
		return InternalEventSafeExport{}, nil, ErrInternalEventSafeExportInvalid
	}
	var result InternalEventSafeExport
	var rows []InternalEventSafeExportRow
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		result, e = s.store.ReadInternalEventSafeExport(tx, id, actor)
		if e == nil {
			rows, e = s.store.ReadInternalEventSafeExportRows(tx, id, actor)
		}
		return e
	})
	if err != nil {
		return InternalEventSafeExport{}, nil, classifyInternalEventSafeExport(err)
	}
	return result, rows, nil
}
func validInternalEventSafeExportFilter(f InternalEventSafeExportFilter) bool {
	if strings.TrimSpace(f.EventType) != f.EventType || strings.TrimSpace(f.Consumer) != f.Consumer || strings.TrimSpace(f.Status) != f.Status || len(f.EventType) > 200 {
		return false
	}
	if f.Consumer != "" {
		if _, ok := eventport.AdminReadBindingForConsumer(f.Consumer); !ok {
			return false
		}
	}
	if f.Status != "" {
		found := false
		for _, status := range eventport.AdminReadStatuses() {
			found = found || status == f.Status
		}
		if !found {
			return false
		}
	}
	return true
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
	return result != nil && json.Unmarshal(raw, result) == nil && validInternalEventSafeExportID(result.ID) && result.RecordCount >= 0 && result.RecordCount <= InternalEventSafeExportMaximumRows && !result.Watermark.IsZero() && !result.CreatedAt.IsZero()
}
func classifyInternalEventSafeExport(err error) error {
	switch {
	case errors.Is(err, ErrInternalEventSafeExportInvalid), errors.Is(err, ErrInternalEventSafeExportConflict), errors.Is(err, ErrInternalEventSafeExportNotFound), errors.Is(err, ErrInternalEventSafeExportUnavailable):
		return err
	default:
		return errors.Join(ErrInternalEventSafeExportUnavailable, err)
	}
}
