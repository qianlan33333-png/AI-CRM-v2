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

const CustomerSafeExportMaximumRows = 10000

var (
	ErrCustomerSafeExportInvalid     = errors.New("invalid customer safe export")
	ErrCustomerSafeExportConflict    = errors.New("customer safe export conflict")
	ErrCustomerSafeExportNotFound    = errors.New("customer safe export not found")
	ErrCustomerSafeExportUnavailable = errors.New("customer safe export unavailable")
)

// CustomerSafeExportCreate freezes the existing customer-list filters. A nil
// OwnerScopeStaffID means a global admin/ops export; sales gets its authorized
// staff id and cannot widen that scope in HTTP.
type CustomerSafeExportCreate struct {
	ActorID           int64
	OwnerScopeStaffID *int64
	IdempotencyKey    string
	Filter            CustomerListInput
}

type CustomerSafeExport struct {
	ID          string    `json:"id"`
	RecordCount int       `json:"record_count"`
	Watermark   time.Time `json:"watermark"`
	CreatedAt   time.Time `json:"created_at"`
}

// CustomerSafeExportRow is deliberately a closed, local-only CSV whitelist.
type CustomerSafeExportRow struct {
	CustomerID     int64
	DisplayName    string
	OwnerStaffID   *int64
	StageID        *int64
	ChannelID      *int64
	AddedAt        *time.Time
	LastInteractAt *time.Time
}

type CustomerSafeExportReceipt struct {
	ID             int64
	PayloadDigest  [32]byte
	ResultSnapshot json.RawMessage
	Completed      bool
}

type CustomerSafeExportStore interface {
	ReserveCustomerSafeExportReceipt(context.Context, int64, [32]byte, [32]byte, time.Time) (CustomerSafeExportReceipt, bool, error)
	CreateCustomerSafeExport(context.Context, CustomerSafeExport, int64, *int64, [32]byte, CustomerListQuery) ([]CustomerSafeExportRow, error)
	CompleteCustomerSafeExportReceipt(context.Context, int64, string, json.RawMessage, time.Time) (CustomerSafeExportReceipt, error)
	ReadCustomerSafeExport(context.Context, string, int64) (CustomerSafeExport, error)
	ReadCustomerSafeExportRows(context.Context, string, int64, *int64) ([]CustomerSafeExportRow, error)
}

type CustomerSafeExportService struct {
	uow    platformport.UnitOfWork
	store  CustomerSafeExportStore
	events eventport.Appender
	now    func() time.Time
}

func NewCustomerSafeExportService(uow platformport.UnitOfWork, store CustomerSafeExportStore, events eventport.Appender) *CustomerSafeExportService {
	return &CustomerSafeExportService{uow: uow, store: store, events: events, now: time.Now}
}

func (service *CustomerSafeExportService) Create(ctx context.Context, command CustomerSafeExportCreate) (CustomerSafeExport, error) {
	if !customerSafeExportReady(service) || ctx == nil || ctx.Err() != nil || command.ActorID < 1 || !validCustomerSafeExportKey(command.IdempotencyKey) {
		return CustomerSafeExport{}, ErrCustomerSafeExportInvalid
	}
	if command.OwnerScopeStaffID != nil && *command.OwnerScopeStaffID < 1 {
		return CustomerSafeExport{}, ErrCustomerSafeExportInvalid
	}
	if command.OwnerScopeStaffID != nil {
		if command.Filter.OwnerStaffID != nil && *command.Filter.OwnerStaffID != *command.OwnerScopeStaffID {
			return CustomerSafeExport{}, ErrCustomerSafeExportConflict
		}
		command.Filter.OwnerStaffID = cloneInt64(command.OwnerScopeStaffID)
	}
	command.Filter.Cursor = ""
	// Reuse the public-list filter normalizer, whose request-page limit is
	// intentionally smaller than this server-side frozen-export cap.
	command.Filter.Limit = CustomerListMaximumLimit
	command.Filter.IsDeleted = false
	query, filterHash, err := (&CustomerListService{}).normalize(command.Filter)
	if err != nil || query.IsDeleted || query.MatchNone || query.CustomerID != nil {
		return CustomerSafeExport{}, ErrCustomerSafeExportInvalid
	}
	query.Limit = CustomerSafeExportMaximumRows
	now := service.now().UTC()
	if now.IsZero() {
		return CustomerSafeExport{}, ErrCustomerSafeExportUnavailable
	}
	query.Watermark = now
	payload, err := json.Marshal(struct {
		FilterHash string `json:"filter_hash"`
		Scope      *int64 `json:"owner_scope_staff_id"`
	}{filterHash, command.OwnerScopeStaffID})
	if err != nil {
		return CustomerSafeExport{}, ErrCustomerSafeExportUnavailable
	}
	keyDigest := sha256.Sum256([]byte(command.IdempotencyKey))
	payloadDigest := sha256.Sum256(payload)
	id, err := newCustomerSafeExportID()
	if err != nil {
		return CustomerSafeExport{}, ErrCustomerSafeExportUnavailable
	}
	result := CustomerSafeExport{ID: id, Watermark: now, CreatedAt: now}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveCustomerSafeExportReceipt(txCtx, command.ActorID, keyDigest, payloadDigest, now)
		if reserveErr != nil {
			return reserveErr
		}
		if receipt.ID < 1 || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return ErrCustomerSafeExportConflict
		}
		if !owned {
			if !receipt.Completed || !decodeCustomerSafeExport(receipt.ResultSnapshot, &result) {
				return ErrCustomerSafeExportUnavailable
			}
			return nil
		}
		rows, createErr := service.store.CreateCustomerSafeExport(txCtx, result, command.ActorID, command.OwnerScopeStaffID, payloadDigest, query)
		if createErr != nil {
			return createErr
		}
		result.RecordCount = len(rows)
		if result.RecordCount > CustomerSafeExportMaximumRows {
			return ErrCustomerSafeExportConflict
		}
		eventPayload, marshalErr := json.Marshal(struct {
			ExportID     string `json:"export_id"`
			RecordCount  int    `json:"record_count"`
			FilterDigest string `json:"filter_digest"`
		}{result.ID, result.RecordCount, filterHash})
		if marshalErr != nil {
			return marshalErr
		}
		if _, appendErr := service.events.Append(txCtx, eventport.Event{
			Type: eventport.EvCustomerSafeExportCreated, Payload: eventPayload, OccurredAt: now,
			IdempotencyKey: "customer-safe-export:" + strconv.FormatInt(receipt.ID, 10),
		}); appendErr != nil {
			return appendErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := service.store.CompleteCustomerSafeExportReceipt(txCtx, receipt.ID, result.ID, snapshot, now)
		var completedResult CustomerSafeExport
		if completeErr != nil || !completed.Completed || !decodeCustomerSafeExport(completed.ResultSnapshot, &completedResult) || completedResult != result {
			return errors.Join(ErrCustomerSafeExportUnavailable, completeErr)
		}
		return nil
	})
	if err != nil {
		return CustomerSafeExport{}, classifyCustomerSafeExportError(err)
	}
	return result, nil
}

func (service *CustomerSafeExportService) Get(ctx context.Context, id string, actorID int64) (CustomerSafeExport, error) {
	if !customerSafeExportReady(service) || ctx == nil || ctx.Err() != nil || actorID < 1 || !validCustomerSafeExportID(id) {
		return CustomerSafeExport{}, ErrCustomerSafeExportInvalid
	}
	var result CustomerSafeExport
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var readErr error
		result, readErr = service.store.ReadCustomerSafeExport(txCtx, id, actorID)
		return readErr
	})
	if err != nil {
		return CustomerSafeExport{}, classifyCustomerSafeExportError(err)
	}
	return result, nil
}

func (service *CustomerSafeExportService) Download(ctx context.Context, id string, actorID int64, ownerScopeStaffID *int64) (CustomerSafeExport, []CustomerSafeExportRow, error) {
	if !customerSafeExportReady(service) || ctx == nil || ctx.Err() != nil || actorID < 1 || !validCustomerSafeExportID(id) {
		return CustomerSafeExport{}, nil, ErrCustomerSafeExportInvalid
	}
	var result CustomerSafeExport
	var rows []CustomerSafeExportRow
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var readErr error
		result, readErr = service.store.ReadCustomerSafeExport(txCtx, id, actorID)
		if readErr != nil {
			return readErr
		}
		rows, readErr = service.store.ReadCustomerSafeExportRows(txCtx, id, actorID, ownerScopeStaffID)
		return readErr
	})
	if err != nil {
		return CustomerSafeExport{}, nil, classifyCustomerSafeExportError(err)
	}
	return result, rows, nil
}

func customerSafeExportReady(service *CustomerSafeExportService) bool {
	return service != nil && service.uow != nil && service.store != nil && service.events != nil && service.now != nil
}

func validCustomerSafeExportKey(key string) bool {
	return utf8.ValidString(key) && strings.TrimSpace(key) == key && utf8.RuneCountInString(key) >= 16 && utf8.RuneCountInString(key) <= 128
}

func validCustomerSafeExportID(value string) bool {
	if len(value) != 36 || !strings.HasPrefix(value, "cse_") {
		return false
	}
	_, err := hex.DecodeString(value[4:])
	return err == nil
}

func newCustomerSafeExportID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "cse_" + hex.EncodeToString(raw), nil
}

func decodeCustomerSafeExport(raw json.RawMessage, result *CustomerSafeExport) bool {
	return result != nil && json.Unmarshal(raw, result) == nil && validCustomerSafeExportID(result.ID) && result.RecordCount >= 0 && result.RecordCount <= CustomerSafeExportMaximumRows && !result.Watermark.IsZero() && !result.CreatedAt.IsZero()
}

func classifyCustomerSafeExportError(err error) error {
	switch {
	case errors.Is(err, ErrCustomerSafeExportInvalid), errors.Is(err, ErrCustomerSafeExportConflict), errors.Is(err, ErrCustomerSafeExportNotFound), errors.Is(err, ErrCustomerSafeExportUnavailable):
		return err
	default:
		return errors.Join(ErrCustomerSafeExportUnavailable, err)
	}
}
