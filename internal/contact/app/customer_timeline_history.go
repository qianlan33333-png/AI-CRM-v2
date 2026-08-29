package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

const customerTimelineHistoryKind = "customer_timeline_event"

// CustomerTimelineHistoryWriter persists only immutable timeline observations
// and caller-transaction journal receipts. It has no dependency on the current
// customer event writer, an outbox, a queue, or a Provider.
type CustomerTimelineHistoryWriter struct {
	store   contact.CustomerTimelineHistoryStore
	journal contact.CustomerTimelineHistoryJournal
}

func NewCustomerTimelineHistoryWriter(store contact.CustomerTimelineHistoryStore, journal contact.CustomerTimelineHistoryJournal) (*CustomerTimelineHistoryWriter, error) {
	if nilCustomerTimelineHistory(store) || nilCustomerTimelineHistory(journal) {
		return nil, contact.ErrCustomerTimelineHistoryUnavailable
	}
	return &CustomerTimelineHistoryWriter{store: store, journal: journal}, nil
}

func (writer *CustomerTimelineHistoryWriter) ImportHistoricalCustomerTimelineEvent(ctx context.Context, source string, value contact.HistoricalCustomerTimelineEvent) (contact.CustomerTimelineHistoryReceipt, error) {
	var empty contact.CustomerTimelineHistoryReceipt
	if writer == nil || ctx == nil || ctx.Err() != nil || nilCustomerTimelineHistory(writer.store) || nilCustomerTimelineHistory(writer.journal) {
		return empty, contact.ErrCustomerTimelineHistoryUnavailable
	}
	value = normalizeCustomerTimelineHistory(value)
	if !validCustomerTimelineHistory(value, false) || source != hex.EncodeToString(value.SourceKeyDigest[:]) {
		return empty, contact.ErrCustomerTimelineHistoryInvalid
	}
	if _, err := HistoricalCustomerTimelineEventDigest(withCustomerTimelineHistoryID(value, 1)); err != nil {
		return empty, contact.ErrCustomerTimelineHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadCustomerTimelineHistory(ctx, customerTimelineHistoryKind, source)
	if err != nil {
		return empty, customerTimelineHistoryError(err)
	}
	if found {
		if !validCustomerTimelineReceipt(receipt, source, value.SourcePayloadDigest) {
			return empty, contact.ErrCustomerTimelineHistoryConflict
		}
		actual, getErr := writer.store.GetHistoricalCustomerTimelineEvent(ctx, receipt.TargetID)
		if getErr != nil {
			return empty, customerTimelineHistoryError(getErr)
		}
		actualDigest, actualErr := HistoricalCustomerTimelineEventDigest(actual)
		expectedDigest, expectedErr := HistoricalCustomerTimelineEventDigest(withCustomerTimelineHistoryID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contact.ErrCustomerTimelineHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, createErr := writer.store.CreateHistoricalCustomerTimelineEvent(ctx, value)
	if createErr != nil {
		return empty, customerTimelineHistoryError(createErr)
	}
	if !validCustomerTimelineHistory(actual, true) || actual.ID < 1 {
		return empty, contact.ErrCustomerTimelineHistoryConflict
	}
	actualDigest, actualErr := HistoricalCustomerTimelineEventDigest(actual)
	expectedDigest, expectedErr := HistoricalCustomerTimelineEventDigest(withCustomerTimelineHistoryID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, contact.ErrCustomerTimelineHistoryConflict
	}
	receipt = contact.CustomerTimelineHistoryReceipt{
		Kind: customerTimelineHistoryKind, SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest,
		TargetDigest: actualDigest, TargetID: actual.ID,
	}
	if err := writer.journal.RecordCustomerTimelineHistory(ctx, receipt); err != nil {
		return empty, customerTimelineHistoryError(err)
	}
	return receipt, nil
}

// HistoricalCustomerTimelineEventDigest includes all private source evidence;
// directly marshaling the Port value would omit json:"-" fields.
func HistoricalCustomerTimelineEventDigest(value contact.HistoricalCustomerTimelineEvent) ([sha256.Size]byte, error) {
	if !validCustomerTimelineHistory(value, true) {
		return [sha256.Size]byte{}, contact.ErrCustomerTimelineHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		ID                                    int64
		SourceKey, SourcePayload, SourceField [sha256.Size]byte
		SourceID                              int64
		EventID, EventType                    string
		EventTime                             time.Time
		Title, Summary                        string
		SourceTable, SourceValue              string
		MetadataJSON                          []byte
		CreatedAt                             time.Time
		UnionID                               string
		CustomerID                            *int64
	}{
		value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.SourceID,
		value.EventID, value.EventType, value.EventTime, value.Title, value.Summary, value.SourceTable, value.SourceValue,
		value.MetadataJSON, value.CreatedAt, value.UnionID, value.CustomerID,
	})
	if err != nil {
		return [sha256.Size]byte{}, contact.ErrCustomerTimelineHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validCustomerTimelineHistory(value contact.HistoricalCustomerTimelineEvent, stored bool) bool {
	if stored && value.ID < 1 || !stored && value.ID != 0 || value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.SourceFieldDigest == ([32]byte{}) || !validCustomerTimelineText(value.EventID) || !validCustomerTimelineText(value.EventType) || !validCustomerTimelineText(value.Title) || !validCustomerTimelineText(value.Summary) || !validCustomerTimelineText(value.SourceTable) || !validCustomerTimelineText(value.SourceValue) || !validCustomerTimelineText(value.UnionID) || !json.Valid(value.MetadataJSON) || value.EventTime.IsZero() || value.CreatedAt.IsZero() {
		return false
	}
	if value.CustomerID != nil && *value.CustomerID < 1 {
		return false
	}
	return !stored || (value.EventTime.Location() == time.UTC && value.EventTime.Equal(value.EventTime.UTC().Truncate(time.Microsecond)) && value.CreatedAt.Location() == time.UTC && value.CreatedAt.Equal(value.CreatedAt.UTC().Truncate(time.Microsecond)))
}

func normalizeCustomerTimelineHistory(value contact.HistoricalCustomerTimelineEvent) contact.HistoricalCustomerTimelineEvent {
	value.EventTime = value.EventTime.UTC().Truncate(time.Microsecond)
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.MetadataJSON = append([]byte(nil), value.MetadataJSON...)
	if value.CustomerID != nil {
		copied := *value.CustomerID
		value.CustomerID = &copied
	}
	return value
}

func withCustomerTimelineHistoryID(value contact.HistoricalCustomerTimelineEvent, id int64) contact.HistoricalCustomerTimelineEvent {
	value.ID = id
	return value
}

func validCustomerTimelineText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validCustomerTimelineReceipt(value contact.CustomerTimelineHistoryReceipt, source string, payload [32]byte) bool {
	return value.Kind == customerTimelineHistoryKind && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([32]byte{})
}

func customerTimelineHistoryError(err error) error {
	if errors.Is(err, contact.ErrCustomerTimelineHistoryInvalid) {
		return contact.ErrCustomerTimelineHistoryInvalid
	}
	if errors.Is(err, contact.ErrCustomerTimelineHistoryConflict) {
		return contact.ErrCustomerTimelineHistoryConflict
	}
	return contact.ErrCustomerTimelineHistoryUnavailable
}

func nilCustomerTimelineHistory(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	return (ref.Kind() == reflect.Ptr || ref.Kind() == reflect.Interface) && ref.IsNil()
}
