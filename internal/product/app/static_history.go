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

	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

// StaticProductHistoryWriter writes immutable V1 page-slice metadata only. It
// has no path to current products, storefront pages, events, or Provider calls.
type StaticProductHistoryWriter struct {
	store   productport.StaticProductHistoryStore
	journal productport.StaticProductHistoryJournal
}

func NewStaticProductHistoryWriter(store productport.StaticProductHistoryStore, journal productport.StaticProductHistoryJournal) (*StaticProductHistoryWriter, error) {
	if staticProductHistoryNil(store) || staticProductHistoryNil(journal) {
		return nil, productport.ErrStaticProductHistoryUnavailable
	}
	return &StaticProductHistoryWriter{store: store, journal: journal}, nil
}

func (writer *StaticProductHistoryWriter) ImportProductPageSlice(ctx context.Context, sourceHex string, value productport.HistoricalProductPageSlice) (productport.StaticProductHistoryReceipt, error) {
	empty := productport.StaticProductHistoryReceipt{}
	if writer == nil || ctx == nil || ctx.Err() != nil || staticProductHistoryNil(writer.store) || staticProductHistoryNil(writer.journal) {
		return empty, productport.ErrStaticProductHistoryUnavailable
	}
	source, ok := staticProductHistorySource(sourceHex)
	if !ok || value.ID != 0 || value.SourceKeyDigest != source || value.SourcePayloadDigest == ([sha256.Size]byte{}) || !validHistoricalProductPageSlice(value, false) {
		return empty, productport.ErrStaticProductHistoryInvalid
	}
	value = normalizeHistoricalProductPageSlice(value)
	if _, err := HistoricalProductPageSliceDigest(withHistoricalProductPageSliceID(value, 1)); err != nil {
		return empty, productport.ErrStaticProductHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadStaticProductHistory(ctx, "product_page_slice", sourceHex)
	if err != nil {
		return empty, staticProductHistoryError(err)
	}
	if found {
		if !validStaticProductHistoryReceipt(receipt, sourceHex, value.SourcePayloadDigest) {
			return empty, productport.ErrStaticProductHistoryConflict
		}
		actual, err := writer.store.GetHistoricalProductPageSlice(ctx, receipt.TargetID)
		if err != nil {
			return empty, staticProductHistoryError(err)
		}
		actualDigest, actualErr := HistoricalProductPageSliceDigest(actual)
		expectedDigest, expectedErr := HistoricalProductPageSliceDigest(withHistoricalProductPageSliceID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, productport.ErrStaticProductHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := writer.store.CreateHistoricalProductPageSlice(ctx, value)
	if err != nil {
		return empty, staticProductHistoryError(err)
	}
	actualDigest, actualErr := HistoricalProductPageSliceDigest(actual)
	expectedDigest, expectedErr := HistoricalProductPageSliceDigest(withHistoricalProductPageSliceID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, productport.ErrStaticProductHistoryConflict
	}
	receipt = productport.StaticProductHistoryReceipt{Kind: "product_page_slice", SourceIdentifier: sourceHex, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err := writer.journal.RecordStaticProductHistory(ctx, receipt); err != nil {
		return empty, staticProductHistoryError(err)
	}
	return receipt, nil
}

func HistoricalProductPageSliceDigest(value productport.HistoricalProductPageSlice) ([sha256.Size]byte, error) {
	if !validHistoricalProductPageSlice(value, true) {
		return [sha256.Size]byte{}, productport.ErrStaticProductHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                string            `json:"kind"`
		ID                  int64             `json:"id"`
		SourceID            int64             `json:"source_id"`
		SourceKeyDigest     [sha256.Size]byte `json:"source_key_digest"`
		SourcePayloadDigest [sha256.Size]byte `json:"source_payload_digest"`
		ProductSourceID     int64             `json:"product_source_id"`
		ImageSourceID       int64             `json:"image_source_id"`
		SortOrder           int64             `json:"sort_order"`
		OriginalEnabled     bool              `json:"original_enabled"`
		CreatedAt           string            `json:"created_at"`
		UpdatedAt           string            `json:"updated_at"`
	}{
		Kind: "v1.product_page_slice", ID: value.ID, SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest,
		SourcePayloadDigest: value.SourcePayloadDigest, ProductSourceID: value.ProductSourceID, ImageSourceID: value.ImageSourceID,
		SortOrder: value.SortOrder, OriginalEnabled: value.OriginalEnabled, CreatedAt: staticProductHistoryTime(value.CreatedAt), UpdatedAt: staticProductHistoryTime(value.UpdatedAt),
	})
	if err != nil {
		return [sha256.Size]byte{}, productport.ErrStaticProductHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validHistoricalProductPageSlice(value productport.HistoricalProductPageSlice, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && staticProductHistoryTimeValid(value.CreatedAt, stored) && staticProductHistoryTimeValid(value.UpdatedAt, stored)
}

func staticProductHistorySource(value string) ([sha256.Size]byte, bool) {
	if len(value) != hex.EncodedLen(sha256.Size) || value != strings.ToLower(value) {
		return [sha256.Size]byte{}, false
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, false
	}
	var result [sha256.Size]byte
	copy(result[:], decoded)
	return result, result != ([sha256.Size]byte{})
}

func normalizeHistoricalProductPageSlice(value productport.HistoricalProductPageSlice) productport.HistoricalProductPageSlice {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	return value
}
func withHistoricalProductPageSliceID(value productport.HistoricalProductPageSlice, id int64) productport.HistoricalProductPageSlice {
	value.ID = id
	return normalizeHistoricalProductPageSlice(value)
}
func staticProductHistoryTimeValid(value time.Time, canonical bool) bool {
	return !value.IsZero() && (!canonical || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func staticProductHistoryTime(value time.Time) string { return value.Format(time.RFC3339Nano) }
func validStaticProductHistoryReceipt(value productport.StaticProductHistoryReceipt, source string, payload [sha256.Size]byte) bool {
	return value.Kind == "product_page_slice" && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([sha256.Size]byte{})
}
func staticProductHistoryError(err error) error {
	switch {
	case errors.Is(err, productport.ErrStaticProductHistoryInvalid):
		return productport.ErrStaticProductHistoryInvalid
	case errors.Is(err, productport.ErrStaticProductHistoryConflict):
		return productport.ErrStaticProductHistoryConflict
	default:
		return productport.ErrStaticProductHistoryUnavailable
	}
}
func staticProductHistoryNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Ptr && v.IsNil()
}
