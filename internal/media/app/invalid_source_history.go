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

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

const invalidAssetHistoryKind = "invalid_asset"

type InvalidSourceHistoryWriter struct {
	store   mediaport.InvalidSourceHistoryStore
	journal mediaport.InvalidSourceHistoryJournal
}

func NewInvalidSourceHistoryWriter(store mediaport.InvalidSourceHistoryStore, journal mediaport.InvalidSourceHistoryJournal) (*InvalidSourceHistoryWriter, error) {
	if nilInvalidSourceHistoryDependency(store) || nilInvalidSourceHistoryDependency(journal) {
		return nil, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	return &InvalidSourceHistoryWriter{store: store, journal: journal}, nil
}

func (writer *InvalidSourceHistoryWriter) ImportHistoricalInvalidAsset(ctx context.Context, sourceIdentifier string, value mediaport.HistoricalInvalidAsset) (mediaport.InvalidSourceHistoryReceipt, error) {
	empty := mediaport.InvalidSourceHistoryReceipt{}
	if writer == nil || ctx == nil || ctx.Err() != nil || nilInvalidSourceHistoryDependency(writer.store) || nilInvalidSourceHistoryDependency(writer.journal) {
		return empty, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	value = normalizeHistoricalInvalidAsset(value)
	if value.ID != 0 || !validHistoricalInvalidAsset(value, false) || !invalidAssetHistorySourceMatches(sourceIdentifier, value.SourceKeyDigest) {
		return empty, mediaport.ErrInvalidSourceHistoryInvalid
	}
	if _, err := DigestHistoricalInvalidAsset(withHistoricalInvalidAssetID(value, 1)); err != nil {
		return empty, mediaport.ErrInvalidSourceHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadInvalidSourceHistory(ctx, invalidAssetHistoryKind, sourceIdentifier)
	if err != nil {
		return empty, invalidAssetHistoryError(err)
	}
	if found {
		if !validInvalidAssetHistoryReceipt(receipt, sourceIdentifier, value.SourcePayloadDigest) {
			return empty, mediaport.ErrInvalidSourceHistoryConflict
		}
		actual, err := writer.store.GetHistoricalInvalidAsset(ctx, receipt.TargetID)
		if err != nil {
			return empty, invalidAssetHistoryError(err)
		}
		actualDigest, actualErr := DigestHistoricalInvalidAsset(actual)
		expectedDigest, expectedErr := DigestHistoricalInvalidAsset(withHistoricalInvalidAssetID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, mediaport.ErrInvalidSourceHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := writer.store.CreateHistoricalInvalidAsset(ctx, value)
	if err != nil {
		return empty, invalidAssetHistoryError(err)
	}
	actualDigest, actualErr := DigestHistoricalInvalidAsset(actual)
	expectedDigest, expectedErr := DigestHistoricalInvalidAsset(withHistoricalInvalidAssetID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, mediaport.ErrInvalidSourceHistoryConflict
	}
	receipt = mediaport.InvalidSourceHistoryReceipt{Kind: invalidAssetHistoryKind, SourceIdentifier: sourceIdentifier, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err := writer.journal.RecordInvalidSourceHistory(ctx, receipt); err != nil {
		return empty, invalidAssetHistoryError(err)
	}
	return receipt, nil
}

func DigestHistoricalInvalidAsset(value mediaport.HistoricalInvalidAsset) ([sha256.Size]byte, error) {
	value = normalizeHistoricalInvalidAsset(value)
	if !validHistoricalInvalidAsset(value, true) {
		return [sha256.Size]byte{}, mediaport.ErrInvalidSourceHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		ID, SourceID                                                           int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest, PrivateDigest [sha256.Size]byte
		RedactedRoots                                                          []string
		Kind, Name, FileName, MIMEType, QuarantineReason                       string
		FileSize                                                               int64
		OriginalEnabled                                                        bool
		ContentDigest                                                          [sha256.Size]byte
		CreatedAt, UpdatedAt                                                   time.Time
	}{value.ID, value.SourceID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.PrivateDigest, value.RedactedRoots, value.Kind, value.Name, value.FileName, value.MIMEType, value.QuarantineReason, value.FileSize, value.OriginalEnabled, value.ContentDigest, value.CreatedAt, value.UpdatedAt})
	if err != nil {
		return [sha256.Size]byte{}, mediaport.ErrInvalidSourceHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validHistoricalInvalidAsset(value mediaport.HistoricalInvalidAsset, stored bool) bool {
	if (stored && value.ID < 1) || (!stored && value.ID != 0) || value.SourceKeyDigest == ([sha256.Size]byte{}) || value.SourcePayloadDigest == ([sha256.Size]byte{}) || value.SourceFieldDigest == ([sha256.Size]byte{}) || value.PrivateDigest == ([sha256.Size]byte{}) || value.ContentDigest == ([sha256.Size]byte{}) || value.RedactedRoots == nil || (value.Kind != "image" && value.Kind != "attachment") || value.QuarantineReason != "invalid_static_media_definition" || !invalidAssetHistoryTime(value.CreatedAt, stored) || !invalidAssetHistoryTime(value.UpdatedAt, stored) {
		return false
	}
	for _, text := range append(value.RedactedRoots, value.Kind, value.Name, value.FileName, value.MIMEType, value.QuarantineReason) {
		if !invalidAssetHistoryText(text) {
			return false
		}
	}
	return true
}

func normalizeHistoricalInvalidAsset(value mediaport.HistoricalInvalidAsset) mediaport.HistoricalInvalidAsset {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	value.RedactedRoots = append([]string{}, value.RedactedRoots...)
	return value
}

func withHistoricalInvalidAssetID(value mediaport.HistoricalInvalidAsset, id int64) mediaport.HistoricalInvalidAsset {
	value.ID = id
	return normalizeHistoricalInvalidAsset(value)
}

func invalidAssetHistorySourceMatches(source string, digest [sha256.Size]byte) bool {
	return digest != ([sha256.Size]byte{}) && len(source) == hex.EncodedLen(sha256.Size) && source == strings.ToLower(source) && source == hex.EncodeToString(digest[:])
}

func invalidAssetHistoryTime(value time.Time, canonical bool) bool {
	return !value.IsZero() && (!canonical || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func invalidAssetHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
func validInvalidAssetHistoryReceipt(value mediaport.InvalidSourceHistoryReceipt, source string, payload [sha256.Size]byte) bool {
	return value.Kind == invalidAssetHistoryKind && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([sha256.Size]byte{})
}
func invalidAssetHistoryError(err error) error {
	switch {
	case errors.Is(err, mediaport.ErrInvalidSourceHistoryInvalid):
		return mediaport.ErrInvalidSourceHistoryInvalid
	case errors.Is(err, mediaport.ErrInvalidSourceHistoryConflict):
		return mediaport.ErrInvalidSourceHistoryConflict
	default:
		return mediaport.ErrInvalidSourceHistoryUnavailable
	}
}
func nilInvalidSourceHistoryDependency(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil()
}
