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

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

const invalidRadarLinkHistoryKind = "invalid_link"

type InvalidSourceHistoryWriter struct {
	store   radarport.InvalidSourceHistoryStore
	journal radarport.InvalidSourceHistoryJournal
}

func NewInvalidSourceHistoryWriter(store radarport.InvalidSourceHistoryStore, journal radarport.InvalidSourceHistoryJournal) (*InvalidSourceHistoryWriter, error) {
	if nilInvalidRadarLinkHistoryDependency(store) || nilInvalidRadarLinkHistoryDependency(journal) {
		return nil, radarport.ErrInvalidSourceHistoryUnavailable
	}
	return &InvalidSourceHistoryWriter{store: store, journal: journal}, nil
}

func (writer *InvalidSourceHistoryWriter) ImportHistoricalInvalidRadarLink(ctx context.Context, sourceIdentifier string, value radarport.HistoricalInvalidRadarLink) (radarport.InvalidSourceHistoryReceipt, error) {
	empty := radarport.InvalidSourceHistoryReceipt{}
	if writer == nil || ctx == nil || ctx.Err() != nil || nilInvalidRadarLinkHistoryDependency(writer.store) || nilInvalidRadarLinkHistoryDependency(writer.journal) {
		return empty, radarport.ErrInvalidSourceHistoryUnavailable
	}
	value = normalizeHistoricalInvalidRadarLink(value)
	if value.ID != 0 || !validHistoricalInvalidRadarLink(value, false) || !invalidRadarLinkHistorySourceMatches(sourceIdentifier, value.SourceKeyDigest) {
		return empty, radarport.ErrInvalidSourceHistoryInvalid
	}
	if _, err := DigestHistoricalInvalidRadarLink(withHistoricalInvalidRadarLinkID(value, 1)); err != nil {
		return empty, radarport.ErrInvalidSourceHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadInvalidSourceHistory(ctx, invalidRadarLinkHistoryKind, sourceIdentifier)
	if err != nil {
		return empty, invalidRadarLinkHistoryError(err)
	}
	if found {
		if !validInvalidRadarLinkHistoryReceipt(receipt, sourceIdentifier, value.SourcePayloadDigest) {
			return empty, radarport.ErrInvalidSourceHistoryConflict
		}
		actual, err := writer.store.GetHistoricalInvalidRadarLink(ctx, receipt.TargetID)
		if err != nil {
			return empty, invalidRadarLinkHistoryError(err)
		}
		actualDigest, actualErr := DigestHistoricalInvalidRadarLink(actual)
		expectedDigest, expectedErr := DigestHistoricalInvalidRadarLink(withHistoricalInvalidRadarLinkID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, radarport.ErrInvalidSourceHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := writer.store.CreateHistoricalInvalidRadarLink(ctx, value)
	if err != nil {
		return empty, invalidRadarLinkHistoryError(err)
	}
	actualDigest, actualErr := DigestHistoricalInvalidRadarLink(actual)
	expectedDigest, expectedErr := DigestHistoricalInvalidRadarLink(withHistoricalInvalidRadarLinkID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, radarport.ErrInvalidSourceHistoryConflict
	}
	receipt = radarport.InvalidSourceHistoryReceipt{Kind: invalidRadarLinkHistoryKind, SourceIdentifier: sourceIdentifier, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err := writer.journal.RecordInvalidSourceHistory(ctx, receipt); err != nil {
		return empty, invalidRadarLinkHistoryError(err)
	}
	return receipt, nil
}

func DigestHistoricalInvalidRadarLink(value radarport.HistoricalInvalidRadarLink) ([sha256.Size]byte, error) {
	value = normalizeHistoricalInvalidRadarLink(value)
	if !validHistoricalInvalidRadarLink(value, true) {
		return [sha256.Size]byte{}, radarport.ErrInvalidSourceHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		ID, SourceID                                                           int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest, PrivateDigest [sha256.Size]byte
		RedactedRoots                                                          []string
		Code, Title, QuarantineReason                                          string
		DestinationURLDigest                                                   [sha256.Size]byte
		CreatedAt, UpdatedAt                                                   time.Time
	}{value.ID, value.SourceID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.PrivateDigest, value.RedactedRoots, value.Code, value.Title, value.QuarantineReason, value.DestinationURLDigest, value.CreatedAt, value.UpdatedAt})
	if err != nil {
		return [sha256.Size]byte{}, radarport.ErrInvalidSourceHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validHistoricalInvalidRadarLink(value radarport.HistoricalInvalidRadarLink, stored bool) bool {
	if (stored && value.ID < 1) || (!stored && value.ID != 0) || value.SourceKeyDigest == ([sha256.Size]byte{}) || value.SourcePayloadDigest == ([sha256.Size]byte{}) || value.SourceFieldDigest == ([sha256.Size]byte{}) || value.PrivateDigest == ([sha256.Size]byte{}) || value.DestinationURLDigest == ([sha256.Size]byte{}) || value.RedactedRoots == nil || value.QuarantineReason != "invalid_radar_definition" || !invalidRadarLinkHistoryTime(value.CreatedAt, stored) || !invalidRadarLinkHistoryTime(value.UpdatedAt, stored) {
		return false
	}
	for _, text := range append(value.RedactedRoots, value.Code, value.Title, value.QuarantineReason) {
		if !invalidRadarLinkHistoryText(text) {
			return false
		}
	}
	return true
}

func normalizeHistoricalInvalidRadarLink(value radarport.HistoricalInvalidRadarLink) radarport.HistoricalInvalidRadarLink {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	value.RedactedRoots = append([]string{}, value.RedactedRoots...)
	return value
}
func withHistoricalInvalidRadarLinkID(value radarport.HistoricalInvalidRadarLink, id int64) radarport.HistoricalInvalidRadarLink {
	value.ID = id
	return normalizeHistoricalInvalidRadarLink(value)
}
func invalidRadarLinkHistorySourceMatches(source string, digest [sha256.Size]byte) bool {
	return digest != ([sha256.Size]byte{}) && len(source) == hex.EncodedLen(sha256.Size) && source == strings.ToLower(source) && source == hex.EncodeToString(digest[:])
}
func invalidRadarLinkHistoryTime(value time.Time, canonical bool) bool {
	return !value.IsZero() && (!canonical || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func invalidRadarLinkHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
func validInvalidRadarLinkHistoryReceipt(value radarport.InvalidSourceHistoryReceipt, source string, payload [sha256.Size]byte) bool {
	return value.Kind == invalidRadarLinkHistoryKind && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([sha256.Size]byte{})
}
func invalidRadarLinkHistoryError(err error) error {
	switch {
	case errors.Is(err, radarport.ErrInvalidSourceHistoryInvalid):
		return radarport.ErrInvalidSourceHistoryInvalid
	case errors.Is(err, radarport.ErrInvalidSourceHistoryConflict):
		return radarport.ErrInvalidSourceHistoryConflict
	default:
		return radarport.ErrInvalidSourceHistoryUnavailable
	}
}
func nilInvalidRadarLinkHistoryDependency(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil()
}
