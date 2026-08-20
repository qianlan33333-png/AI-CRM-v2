package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	CustomerMergeHistoryDefaultLimit = int32(50)
	CustomerMergeHistoryMaximumLimit = int32(100)
	customerMergeHistoryCursorLimit  = 512
)

var (
	ErrCustomerMergeHistoryInvalid     = errors.New("invalid customer merge history query")
	ErrCustomerMergeHistoryUnavailable = errors.New("customer merge history unavailable")
)

type CustomerMergeHistoryStore interface {
	ListCustomerMergeHistory(context.Context, contactport.CustomerID, int64, int32) ([]identityport.CustomerMergeHistory, error)
}

type CustomerMergeHistoryService struct {
	uow   platformport.UnitOfWork
	store CustomerMergeHistoryStore
}

func NewCustomerMergeHistoryService(uow platformport.UnitOfWork, store CustomerMergeHistoryStore) *CustomerMergeHistoryService {
	return &CustomerMergeHistoryService{uow: uow, store: store}
}

func (service *CustomerMergeHistoryService) ListCustomerMergeHistory(
	ctx context.Context,
	customerID contactport.CustomerID,
	cursor string,
	limit int32,
) (identityport.CustomerMergeHistoryPage, error) {
	if ctx == nil || customerID <= 0 || len(cursor) > customerMergeHistoryCursorLimit {
		return identityport.CustomerMergeHistoryPage{}, ErrCustomerMergeHistoryInvalid
	}
	if service == nil || nilMergeHistoryDependency(service.uow) || nilMergeHistoryDependency(service.store) {
		return identityport.CustomerMergeHistoryPage{}, ErrCustomerMergeHistoryUnavailable
	}
	if limit == 0 {
		limit = CustomerMergeHistoryDefaultLimit
	}
	if limit < 1 || limit > CustomerMergeHistoryMaximumLimit {
		return identityport.CustomerMergeHistoryPage{}, ErrCustomerMergeHistoryInvalid
	}
	afterID, err := decodeCustomerMergeHistoryCursor(cursor, customerID)
	if err != nil {
		return identityport.CustomerMergeHistoryPage{}, err
	}
	var rows []identityport.CustomerMergeHistory
	err = service.uow.Within(ctx, func(tx context.Context) error {
		var storeErr error
		rows, storeErr = service.store.ListCustomerMergeHistory(tx, customerID, afterID, limit+1)
		return storeErr
	})
	if err != nil || len(rows) > int(limit)+1 {
		return identityport.CustomerMergeHistoryPage{}, errors.Join(ErrCustomerMergeHistoryUnavailable, err)
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	page := identityport.CustomerMergeHistoryPage{CustomerID: customerID, Items: make([]identityport.CustomerMergeHistory, 0, len(rows))}
	for index, row := range rows {
		if !validCustomerMergeHistory(row) || (index > 0 && rows[index-1].MergeAuditID <= row.MergeAuditID) {
			return identityport.CustomerMergeHistoryPage{}, ErrCustomerMergeHistoryUnavailable
		}
		page.Items = append(page.Items, row)
	}
	if hasMore {
		page.NextCursor, err = encodeCustomerMergeHistoryCursor(customerID, rows[len(rows)-1].MergeAuditID)
		if err != nil {
			return identityport.CustomerMergeHistoryPage{}, errors.Join(ErrCustomerMergeHistoryUnavailable, err)
		}
	}
	return page, nil
}

func validCustomerMergeHistory(value identityport.CustomerMergeHistory) bool {
	return value.MergeAuditID > 0 && value.PrimaryCustomerID > 0 && value.MergedCustomerID > 0 &&
		value.PrimaryCustomerID != value.MergedCustomerID && (value.Mode == "auto" || value.Mode == "manual") &&
		validMergeHistoryText(value.PolicyVersion, 200) && !value.MergedAt.IsZero()
}

func validMergeHistoryText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maximum && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

type customerMergeHistoryCursor struct {
	Version    int    `json:"v"`
	Operation  string `json:"operation"`
	Sort       string `json:"sort"`
	CustomerID int64  `json:"customer_id"`
	AfterID    int64  `json:"after_id"`
}

func encodeCustomerMergeHistoryCursor(customerID contactport.CustomerID, afterID int64) (string, error) {
	if customerID <= 0 || afterID <= 0 {
		return "", ErrCustomerMergeHistoryInvalid
	}
	encoded, err := json.Marshal(customerMergeHistoryCursor{1, "listCustomerMergeHistory", "audit_id_desc", int64(customerID), afterID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCustomerMergeHistoryCursor(raw string, customerID contactport.CustomerID) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	if len(raw) > customerMergeHistoryCursorLimit || strings.Contains(raw, "=") {
		return 0, ErrCustomerMergeHistoryInvalid
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return 0, ErrCustomerMergeHistoryInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var cursor customerMergeHistoryCursor
	if err = decoder.Decode(&cursor); err != nil {
		return 0, ErrCustomerMergeHistoryInvalid
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return 0, ErrCustomerMergeHistoryInvalid
	}
	if cursor.Version != 1 || cursor.Operation != "listCustomerMergeHistory" || cursor.Sort != "audit_id_desc" ||
		cursor.CustomerID != int64(customerID) || cursor.AfterID <= 0 {
		return 0, ErrCustomerMergeHistoryInvalid
	}
	return cursor.AfterID, nil
}

func nilMergeHistoryDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}

var _ identityport.CustomerMergeHistoryReader = (*CustomerMergeHistoryService)(nil)
