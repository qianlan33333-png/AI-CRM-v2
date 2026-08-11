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
	"io"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	customerListCursorVersion   = 1
	customerListCursorOperation = "listCustomers"
	customerListCursorSort      = "updated_at_desc,id_desc"
	customerListMaximumCursor   = 512
	customerListMaximumKeyword  = 200
)

var (
	ErrInvalidCustomerListQuery = errors.New("invalid customer list query")
	ErrCustomerListUnavailable  = errors.New("customer list unavailable")
)

type CustomerListInput struct {
	Keyword            string
	OwnerStaffID       *int64
	StageID            *int64
	ChannelID          *int64
	TagID              *int64
	IsDeleted          bool
	AddedAfter         *time.Time
	AddedBefore        *time.Time
	LastInteractAfter  *time.Time
	LastInteractBefore *time.Time
	Cursor             string
	Limit              int32
}

type CustomerListResult struct {
	Items           []CustomerRecord
	NextCursor      *string
	Total           int64
	TotalIsEstimate bool
	Watermark       time.Time
}

type CustomerListService struct {
	uow   platformport.UnitOfWork
	store CustomerQueryStore
	now   func() time.Time
}

func NewCustomerListService(uow platformport.UnitOfWork, store CustomerQueryStore) *CustomerListService {
	return &CustomerListService{uow: uow, store: store, now: time.Now}
}

func (service *CustomerListService) List(
	ctx context.Context,
	input CustomerListInput,
) (CustomerListResult, error) {
	if ctx == nil || service == nil || service.uow == nil || service.store == nil || service.now == nil {
		return CustomerListResult{}, ErrCustomerListUnavailable
	}
	if err := ctx.Err(); err != nil {
		return CustomerListResult{}, errors.Join(ErrCustomerListUnavailable, err)
	}

	query, filterHash, err := service.normalize(input)
	if err != nil {
		return CustomerListResult{}, err
	}
	if input.Cursor == "" {
		query.Watermark = service.now().UTC()
		if query.Watermark.IsZero() {
			return CustomerListResult{}, ErrCustomerListUnavailable
		}
	} else if err := applyCustomerListCursor(&query, input.Cursor, filterHash); err != nil {
		return CustomerListResult{}, errors.Join(ErrInvalidCustomerListQuery, err)
	}

	var stored CustomerListStoreResult
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		stored, storeErr = service.store.ListCustomers(txCtx, query)
		return storeErr
	})
	if err != nil {
		return CustomerListResult{}, errors.Join(ErrCustomerListUnavailable, err)
	}
	if err := validateCustomerListStoreResult(stored, query); err != nil {
		return CustomerListResult{}, errors.Join(ErrCustomerListUnavailable, err)
	}

	result := CustomerListResult{
		Items:     stored.Items,
		Total:     stored.BoundedTotal,
		Watermark: query.Watermark,
	}
	if stored.BoundedTotal > CustomerListExactTotalCap {
		result.Total = CustomerListExactTotalCap
		result.TotalIsEstimate = true
	}
	if stored.HasMore {
		last := stored.Items[len(stored.Items)-1]
		next, encodeErr := encodeCustomerListCursor(filterHash, query.Watermark, last.UpdatedAt, last.ID)
		if encodeErr != nil {
			return CustomerListResult{}, errors.Join(ErrCustomerListUnavailable, encodeErr)
		}
		result.NextCursor = &next
	}
	return result, nil
}

func (service *CustomerListService) normalize(input CustomerListInput) (CustomerListQuery, string, error) {
	limit := input.Limit
	if limit == 0 {
		limit = CustomerListDefaultLimit
	}
	if limit < 1 || limit > CustomerListMaximumLimit || len(input.Cursor) > customerListMaximumCursor {
		return CustomerListQuery{}, "", ErrInvalidCustomerListQuery
	}

	keyword := strings.TrimSpace(input.Keyword)
	if !utf8.ValidString(keyword) || utf8.RuneCountInString(keyword) > customerListMaximumKeyword {
		return CustomerListQuery{}, "", ErrInvalidCustomerListQuery
	}
	for _, id := range []*int64{input.OwnerStaffID, input.StageID, input.ChannelID, input.TagID} {
		if id != nil && *id <= 0 {
			return CustomerListQuery{}, "", ErrInvalidCustomerListQuery
		}
	}

	addedAfter, err := normalizedOptionalTime(input.AddedAfter)
	if err != nil {
		return CustomerListQuery{}, "", err
	}
	addedBefore, err := normalizedOptionalTime(input.AddedBefore)
	if err != nil {
		return CustomerListQuery{}, "", err
	}
	interactAfter, err := normalizedOptionalTime(input.LastInteractAfter)
	if err != nil {
		return CustomerListQuery{}, "", err
	}
	interactBefore, err := normalizedOptionalTime(input.LastInteractBefore)
	if err != nil {
		return CustomerListQuery{}, "", err
	}
	if invalidTimeRange(addedAfter, addedBefore) || invalidTimeRange(interactAfter, interactBefore) {
		return CustomerListQuery{}, "", ErrInvalidCustomerListQuery
	}

	query := CustomerListQuery{
		Keyword:            keyword,
		OwnerStaffID:       cloneInt64(input.OwnerStaffID),
		StageID:            cloneInt64(input.StageID),
		ChannelID:          cloneInt64(input.ChannelID),
		TagID:              cloneInt64(input.TagID),
		IsDeleted:          input.IsDeleted,
		AddedAfter:         addedAfter,
		AddedBefore:        addedBefore,
		LastInteractAfter:  interactAfter,
		LastInteractBefore: interactBefore,
		Limit:              limit,
	}
	filterHash, err := customerListFilterHash(query)
	if err != nil {
		return CustomerListQuery{}, "", errors.Join(ErrCustomerListUnavailable, err)
	}
	return query, filterHash, nil
}

type customerListFilterFingerprint struct {
	Keyword            string  `json:"keyword"`
	OwnerStaffID       *int64  `json:"owner_staff_id"`
	StageID            *int64  `json:"stage_id"`
	ChannelID          *int64  `json:"channel_id"`
	TagID              *int64  `json:"tag_id"`
	IsDeleted          bool    `json:"is_deleted"`
	AddedAfter         *string `json:"added_after"`
	AddedBefore        *string `json:"added_before"`
	LastInteractAfter  *string `json:"last_interact_after"`
	LastInteractBefore *string `json:"last_interact_before"`
}

func customerListFilterHash(query CustomerListQuery) (string, error) {
	fingerprint := customerListFilterFingerprint{
		Keyword: query.Keyword, OwnerStaffID: query.OwnerStaffID, StageID: query.StageID,
		ChannelID: query.ChannelID, TagID: query.TagID, IsDeleted: query.IsDeleted,
		AddedAfter: formatOptionalTime(query.AddedAfter), AddedBefore: formatOptionalTime(query.AddedBefore),
		LastInteractAfter:  formatOptionalTime(query.LastInteractAfter),
		LastInteractBefore: formatOptionalTime(query.LastInteractBefore),
	}
	encoded, err := json.Marshal(fingerprint)
	if err != nil {
		return "", fmt.Errorf("encode customer list filter: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

type customerListCursor struct {
	Version    int    `json:"v"`
	Operation  string `json:"operation"`
	Sort       string `json:"sort"`
	FilterHash string `json:"filter"`
	Watermark  string `json:"watermark"`
	UpdatedAt  string `json:"updated_at"`
	ID         int64  `json:"id"`
}

func encodeCustomerListCursor(
	filterHash string,
	watermark time.Time,
	updatedAt time.Time,
	id contactport.CustomerID,
) (string, error) {
	payload := customerListCursor{
		Version: customerListCursorVersion, Operation: customerListCursorOperation,
		Sort: customerListCursorSort, FilterHash: filterHash,
		Watermark: watermark.UTC().Format(time.RFC3339Nano),
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339Nano), ID: int64(id),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode customer list cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func applyCustomerListCursor(query *CustomerListQuery, raw string, expectedFilterHash string) error {
	if query == nil || raw == "" || len(raw) > customerListMaximumCursor || strings.Contains(raw, "=") {
		return errors.New("customer list cursor shape is invalid")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return errors.New("customer list cursor encoding is invalid")
	}

	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var cursor customerListCursor
	if err := decoder.Decode(&cursor); err != nil {
		return errors.New("customer list cursor payload is invalid")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return errors.New("customer list cursor payload is invalid")
	}
	if cursor.Version != customerListCursorVersion || cursor.Operation != customerListCursorOperation ||
		cursor.Sort != customerListCursorSort || cursor.FilterHash != expectedFilterHash || cursor.ID <= 0 {
		return errors.New("customer list cursor contract does not match")
	}
	watermark, err := parseCursorTime(cursor.Watermark)
	if err != nil {
		return errors.New("customer list cursor watermark is invalid")
	}
	updatedAt, err := parseCursorTime(cursor.UpdatedAt)
	if err != nil || updatedAt.After(watermark) {
		return errors.New("customer list cursor position is invalid")
	}
	id := contactport.CustomerID(cursor.ID)
	query.Watermark = watermark
	query.AfterUpdatedAt = &updatedAt
	query.AfterID = &id
	return nil
}

func validateCustomerListStoreResult(result CustomerListStoreResult, query CustomerListQuery) error {
	if result.BoundedTotal < 0 || result.BoundedTotal > CustomerListExactTotalCap+1 ||
		result.BoundedTotal < int64(len(result.Items)) || len(result.Items) > int(query.Limit) ||
		(result.HasMore && len(result.Items) == 0) {
		return errors.New("customer list store result shape is invalid")
	}
	for index, item := range result.Items {
		if item.ID <= 0 || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.UpdatedAt.After(query.Watermark) ||
			!validJSONObject(item.Extra) {
			return errors.New("customer list store item is invalid")
		}
		if index > 0 {
			previous := result.Items[index-1]
			if item.UpdatedAt.After(previous.UpdatedAt) ||
				(item.UpdatedAt.Equal(previous.UpdatedAt) && item.ID >= previous.ID) {
				return errors.New("customer list store order is invalid")
			}
		}
	}
	return nil
}

func validJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid(trimmed)
}

func normalizedOptionalTime(value *time.Time) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	if value.IsZero() {
		return nil, ErrInvalidCustomerListQuery
	}
	normalized := value.UTC()
	return &normalized, nil
}

func invalidTimeRange(after, before *time.Time) bool {
	return after != nil && before != nil && after.After(*before)
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func parseCursorTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.IsZero() || parsed.Format(time.RFC3339Nano) != value || parsed.Location() != time.UTC {
		return time.Time{}, errors.New("invalid cursor time")
	}
	return parsed, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing cursor payload")
	}
	return nil
}
