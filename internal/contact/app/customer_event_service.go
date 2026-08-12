package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	customerEventCursorVersion   = 1
	customerEventCursorOperation = "listCustomerEvents"
	customerEventCursorSort      = "occurred_at_desc,id_desc"
	customerEventMaximumCursor   = 512
)

var (
	ErrInvalidCustomerEventQuery = errors.New("invalid customer event query")
	ErrCustomerEventsUnavailable = errors.New("customer events unavailable")
)

type CustomerEventInput struct {
	CustomerID   contactport.CustomerID
	OwnerStaffID *int64
	Cursor       string
	Limit        int32
}

type CustomerEventQuery struct {
	CustomerID      contactport.CustomerID
	OwnerStaffID    *int64
	AfterOccurredAt *time.Time
	AfterID         *int64
	Limit           int32
}

type CustomerEventRecord struct {
	ID         int64
	CustomerID contactport.CustomerID
	EventType  string
	Payload    json.RawMessage
	Actor      string
	OccurredAt time.Time
}

type CustomerEventStoreResult struct {
	Items   []CustomerEventRecord
	HasMore bool
}

type CustomerEventResult struct {
	Items      []CustomerEventRecord
	NextCursor *string
}

type CustomerEventStore interface {
	ListCustomerEvents(context.Context, CustomerEventQuery) (CustomerEventStoreResult, error)
}

type CustomerEventService struct {
	uow   platformport.UnitOfWork
	store CustomerEventStore
}

func NewCustomerEventService(uow platformport.UnitOfWork, store CustomerEventStore) *CustomerEventService {
	return &CustomerEventService{uow: uow, store: store}
}

func (service *CustomerEventService) List(
	ctx context.Context,
	input CustomerEventInput,
) (CustomerEventResult, error) {
	if ctx == nil || input.CustomerID <= 0 || (input.OwnerStaffID != nil && *input.OwnerStaffID <= 0) ||
		input.Limit < 0 || input.Limit > CustomerListMaximumLimit || len(input.Cursor) > customerEventMaximumCursor {
		return CustomerEventResult{}, ErrInvalidCustomerEventQuery
	}
	if service == nil || nilCustomerDetailDependency(service.uow) || nilCustomerDetailDependency(service.store) {
		return CustomerEventResult{}, ErrCustomerEventsUnavailable
	}
	if err := ctx.Err(); err != nil {
		return CustomerEventResult{}, errors.Join(ErrCustomerEventsUnavailable, err)
	}

	query := CustomerEventQuery{
		CustomerID:   input.CustomerID,
		OwnerStaffID: cloneInt64(input.OwnerStaffID),
		Limit:        input.Limit,
	}
	if query.Limit == 0 {
		query.Limit = CustomerListDefaultLimit
	}
	fingerprint := customerEventFilterHash(input.CustomerID, input.OwnerStaffID)
	if input.Cursor != "" {
		if err := applyCustomerEventCursor(&query, input.Cursor, fingerprint); err != nil {
			return CustomerEventResult{}, errors.Join(ErrInvalidCustomerEventQuery, err)
		}
	}

	var stored CustomerEventStoreResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		stored, storeErr = service.store.ListCustomerEvents(txCtx, query)
		return storeErr
	})
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			return CustomerEventResult{}, ErrCustomerNotFound
		}
		return CustomerEventResult{}, errors.Join(ErrCustomerEventsUnavailable, err)
	}
	if err := validateCustomerEventStoreResult(stored, query); err != nil {
		return CustomerEventResult{}, errors.Join(ErrCustomerEventsUnavailable, err)
	}

	result := CustomerEventResult{Items: cloneCustomerEvents(stored.Items)}
	if stored.HasMore {
		last := stored.Items[len(stored.Items)-1]
		next, encodeErr := encodeCustomerEventCursor(fingerprint, last.OccurredAt, last.ID)
		if encodeErr != nil {
			return CustomerEventResult{}, errors.Join(ErrCustomerEventsUnavailable, encodeErr)
		}
		result.NextCursor = &next
	}
	return result, nil
}

type customerEventCursor struct {
	Version    int    `json:"v"`
	Operation  string `json:"operation"`
	Sort       string `json:"sort"`
	FilterHash string `json:"filter"`
	OccurredAt string `json:"occurred_at"`
	ID         int64  `json:"id"`
}

func customerEventFilterHash(customerID contactport.CustomerID, ownerStaffID *int64) string {
	payload := fmt.Sprintf("customer_id=%d;owner_staff_id=", customerID)
	if ownerStaffID != nil {
		payload += fmt.Sprintf("%d", *ownerStaffID)
	}
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func encodeCustomerEventCursor(filterHash string, occurredAt time.Time, id int64) (string, error) {
	if len(filterHash) != sha256.Size*2 || occurredAt.IsZero() || id <= 0 {
		return "", errors.New("customer event cursor source is invalid")
	}
	payload := customerEventCursor{
		Version: customerEventCursorVersion, Operation: customerEventCursorOperation,
		Sort: customerEventCursorSort, FilterHash: filterHash,
		OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), ID: id,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode customer event cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func applyCustomerEventCursor(query *CustomerEventQuery, raw, expectedFilterHash string) error {
	if query == nil || raw == "" || len(raw) > customerEventMaximumCursor || strings.Contains(raw, "=") {
		return errors.New("customer event cursor shape is invalid")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return errors.New("customer event cursor encoding is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var cursor customerEventCursor
	if decoder.Decode(&cursor) != nil || ensureJSONEOF(decoder) != nil ||
		cursor.Version != customerEventCursorVersion || cursor.Operation != customerEventCursorOperation ||
		cursor.Sort != customerEventCursorSort || cursor.FilterHash != expectedFilterHash || cursor.ID <= 0 {
		return errors.New("customer event cursor contract does not match")
	}
	occurredAt, err := parseCursorTime(cursor.OccurredAt)
	if err != nil {
		return errors.New("customer event cursor position is invalid")
	}
	query.AfterOccurredAt = &occurredAt
	id := cursor.ID
	query.AfterID = &id
	return nil
}

func validateCustomerEventStoreResult(result CustomerEventStoreResult, query CustomerEventQuery) error {
	if len(result.Items) > int(query.Limit) || (result.HasMore && len(result.Items) == 0) {
		return errors.New("customer event store result shape is invalid")
	}
	for index, item := range result.Items {
		_, offset := item.OccurredAt.Zone()
		if item.ID <= 0 || item.CustomerID != query.CustomerID || item.OccurredAt.IsZero() || offset != 0 ||
			!validCustomerEventText(item.EventType, 0) || !validCustomerEventText(item.Actor, 200) ||
			!validCustomerEventPayload(item.Payload) {
			return errors.New("customer event store item is invalid")
		}
		if index > 0 {
			previous := result.Items[index-1]
			if item.OccurredAt.After(previous.OccurredAt) ||
				(item.OccurredAt.Equal(previous.OccurredAt) && item.ID >= previous.ID) {
				return errors.New("customer event store order is invalid")
			}
		}
	}
	return nil
}

func validCustomerEventText(value string, maximum int) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && utf8.ValidString(value) &&
		(maximum == 0 || utf8.RuneCountInString(value) <= maximum)
}

func validCustomerEventPayload(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var payload map[string]any
	return decoder.Decode(&payload) == nil && payload != nil && ensureJSONEOF(decoder) == nil
}

func cloneCustomerEvents(items []CustomerEventRecord) []CustomerEventRecord {
	cloned := make([]CustomerEventRecord, len(items))
	copy(cloned, items)
	for index := range cloned {
		cloned[index].Payload = append(json.RawMessage(nil), items[index].Payload...)
		cloned[index].OccurredAt = items[index].OccurredAt.UTC()
	}
	return cloned
}
