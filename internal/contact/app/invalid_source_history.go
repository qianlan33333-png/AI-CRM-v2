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

const (
	invalidSourceHistoryUnboundTagKind     = "unbound_tag"
	invalidSourceHistoryInvalidChannelKind = "invalid_channel"
)

// InvalidSourceHistoryWriter stores only sealed invalid V1 source facts. It
// deliberately has no dependency on current tags, channels, events, or a Provider.
type InvalidSourceHistoryWriter struct {
	store   contact.InvalidSourceHistoryStore
	journal contact.InvalidSourceHistoryJournal
}

func NewInvalidSourceHistoryWriter(store contact.InvalidSourceHistoryStore, journal contact.InvalidSourceHistoryJournal) *InvalidSourceHistoryWriter {
	return &InvalidSourceHistoryWriter{store: store, journal: journal}
}

func (writer *InvalidSourceHistoryWriter) ImportHistoricalUnboundTag(ctx context.Context, sourceIdentifier string, value contact.HistoricalUnboundTag) (contact.InvalidSourceHistoryReceipt, error) {
	empty := contact.InvalidSourceHistoryReceipt{}
	if !invalidSourceHistoryReady(writer, ctx) {
		return empty, contact.ErrInvalidSourceHistoryUnavailable
	}
	source, ok := invalidSourceHistorySource(sourceIdentifier)
	if !ok || value.ID != 0 || value.SourceKeyDigest != source || !validHistoricalUnboundTag(value, false) {
		return empty, contact.ErrInvalidSourceHistoryInvalid
	}
	value = normalizeHistoricalUnboundTag(value)
	if _, err := DigestHistoricalUnboundTag(withHistoricalUnboundTagID(value, 1)); err != nil {
		return empty, contact.ErrInvalidSourceHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadInvalidSourceHistory(ctx, invalidSourceHistoryUnboundTagKind, sourceIdentifier)
	if err != nil {
		return empty, invalidSourceHistoryError(err)
	}
	if found {
		if !validInvalidSourceHistoryReceipt(receipt, invalidSourceHistoryUnboundTagKind, sourceIdentifier, value.SourcePayloadDigest) {
			return empty, contact.ErrInvalidSourceHistoryConflict
		}
		actual, getErr := writer.store.GetHistoricalUnboundTag(ctx, receipt.TargetID)
		expected := withHistoricalUnboundTagID(value, receipt.TargetID)
		actualDigest, actualErr := DigestHistoricalUnboundTag(actual)
		expectedDigest, expectedErr := DigestHistoricalUnboundTag(expected)
		if getErr != nil || actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contact.ErrInvalidSourceHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := writer.store.CreateHistoricalUnboundTag(ctx, value)
	if err != nil {
		return empty, invalidSourceHistoryError(err)
	}
	actualDigest, actualErr := DigestHistoricalUnboundTag(actual)
	expectedDigest, expectedErr := DigestHistoricalUnboundTag(withHistoricalUnboundTagID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, contact.ErrInvalidSourceHistoryConflict
	}
	receipt = contact.InvalidSourceHistoryReceipt{Kind: invalidSourceHistoryUnboundTagKind, SourceIdentifier: sourceIdentifier, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err := writer.journal.RecordInvalidSourceHistory(ctx, receipt); err != nil {
		return empty, invalidSourceHistoryError(err)
	}
	return receipt, nil
}

func (writer *InvalidSourceHistoryWriter) ImportHistoricalInvalidChannel(ctx context.Context, sourceIdentifier string, value contact.HistoricalInvalidChannel) (contact.InvalidSourceHistoryReceipt, error) {
	empty := contact.InvalidSourceHistoryReceipt{}
	if !invalidSourceHistoryReady(writer, ctx) {
		return empty, contact.ErrInvalidSourceHistoryUnavailable
	}
	source, ok := invalidSourceHistorySource(sourceIdentifier)
	if !ok || value.ID != 0 || value.SourceKeyDigest != source || !validHistoricalInvalidChannel(value, false) {
		return empty, contact.ErrInvalidSourceHistoryInvalid
	}
	value = normalizeHistoricalInvalidChannel(value)
	if _, err := DigestHistoricalInvalidChannel(withHistoricalInvalidChannelID(value, 1)); err != nil {
		return empty, contact.ErrInvalidSourceHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadInvalidSourceHistory(ctx, invalidSourceHistoryInvalidChannelKind, sourceIdentifier)
	if err != nil {
		return empty, invalidSourceHistoryError(err)
	}
	if found {
		if !validInvalidSourceHistoryReceipt(receipt, invalidSourceHistoryInvalidChannelKind, sourceIdentifier, value.SourcePayloadDigest) {
			return empty, contact.ErrInvalidSourceHistoryConflict
		}
		actual, getErr := writer.store.GetHistoricalInvalidChannel(ctx, receipt.TargetID)
		expected := withHistoricalInvalidChannelID(value, receipt.TargetID)
		actualDigest, actualErr := DigestHistoricalInvalidChannel(actual)
		expectedDigest, expectedErr := DigestHistoricalInvalidChannel(expected)
		if getErr != nil || actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contact.ErrInvalidSourceHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := writer.store.CreateHistoricalInvalidChannel(ctx, value)
	if err != nil {
		return empty, invalidSourceHistoryError(err)
	}
	actualDigest, actualErr := DigestHistoricalInvalidChannel(actual)
	expectedDigest, expectedErr := DigestHistoricalInvalidChannel(withHistoricalInvalidChannelID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, contact.ErrInvalidSourceHistoryConflict
	}
	receipt = contact.InvalidSourceHistoryReceipt{Kind: invalidSourceHistoryInvalidChannelKind, SourceIdentifier: sourceIdentifier, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err := writer.journal.RecordInvalidSourceHistory(ctx, receipt); err != nil {
		return empty, invalidSourceHistoryError(err)
	}
	return receipt, nil
}

func DigestHistoricalUnboundTag(value contact.HistoricalUnboundTag) ([sha256.Size]byte, error) {
	value = normalizeHistoricalUnboundTag(value)
	if !validHistoricalUnboundTag(value, true) {
		return [sha256.Size]byte{}, contact.ErrInvalidSourceHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                                                    string
		ID                                                      int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest [32]byte
		PrivateDigest                                           [32]byte
		RedactedRoots                                           []string
		TagSourceID                                             string
		UnionIDDigest                                           [32]byte
		CreatedAt, QuarantineReason                             string
	}{"v1.unbound_tag", value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.PrivateDigest, value.RedactedRoots, value.TagSourceID, value.UnionIDDigest, invalidSourceHistoryTime(value.CreatedAt), value.QuarantineReason})
	if err != nil {
		return [sha256.Size]byte{}, contact.ErrInvalidSourceHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func DigestHistoricalInvalidChannel(value contact.HistoricalInvalidChannel) ([sha256.Size]byte, error) {
	value = normalizeHistoricalInvalidChannel(value)
	if !validHistoricalInvalidChannel(value, true) {
		return [sha256.Size]byte{}, contact.ErrInvalidSourceHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                                                    string
		ID                                                      int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest [32]byte
		PrivateDigest                                           [32]byte
		RedactedRoots                                           []string
		SourceID                                                int64
		Code, Name, ChannelType, CarrierType                    string
		CreatedAt, UpdatedAt, QuarantineReason                  string
	}{"v1.invalid_channel", value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.PrivateDigest, value.RedactedRoots, value.SourceID, value.Code, value.Name, value.ChannelType, value.CarrierType, invalidSourceHistoryTime(value.CreatedAt), invalidSourceHistoryTime(value.UpdatedAt), value.QuarantineReason})
	if err != nil {
		return [sha256.Size]byte{}, contact.ErrInvalidSourceHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func invalidSourceHistoryReady(writer *InvalidSourceHistoryWriter, ctx context.Context) bool {
	return writer != nil && ctx != nil && ctx.Err() == nil && !invalidSourceHistoryNil(writer.store) && !invalidSourceHistoryNil(writer.journal)
}

func invalidSourceHistorySource(value string) ([sha256.Size]byte, bool) {
	if len(value) != hex.EncodedLen(sha256.Size) || value != strings.ToLower(value) {
		return [sha256.Size]byte{}, false
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, false
	}
	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	return digest, digest != ([sha256.Size]byte{})
}

func validHistoricalUnboundTag(value contact.HistoricalUnboundTag, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && invalidSourceHistoryDigest(value.SourceKeyDigest) && invalidSourceHistoryDigest(value.SourcePayloadDigest) && invalidSourceHistoryDigest(value.SourceFieldDigest) && invalidSourceHistoryDigest(value.PrivateDigest) && invalidSourceHistoryDigest(value.UnionIDDigest) && invalidSourceHistoryText(value.TagSourceID) && invalidSourceHistoryRoots(value.RedactedRoots) && invalidSourceHistoryTimeValid(value.CreatedAt, stored) && value.QuarantineReason == "invalid_contact_tag"
}

func validHistoricalInvalidChannel(value contact.HistoricalInvalidChannel, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && invalidSourceHistoryDigest(value.SourceKeyDigest) && invalidSourceHistoryDigest(value.SourcePayloadDigest) && invalidSourceHistoryDigest(value.SourceFieldDigest) && invalidSourceHistoryDigest(value.PrivateDigest) && invalidSourceHistoryRoots(value.RedactedRoots) && invalidSourceHistoryText(value.Code) && invalidSourceHistoryText(value.Name) && invalidSourceHistoryText(value.ChannelType) && invalidSourceHistoryText(value.CarrierType) && invalidSourceHistoryTimeValid(value.CreatedAt, stored) && invalidSourceHistoryTimeValid(value.UpdatedAt, stored) && value.QuarantineReason == "invalid_channel_definition"
}

func invalidSourceHistoryDigest(value [32]byte) bool { return value != ([32]byte{}) }
func invalidSourceHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
func invalidSourceHistoryRoots(values []string) bool {
	for _, value := range values {
		if !invalidSourceHistoryText(value) {
			return false
		}
	}
	return true
}
func invalidSourceHistoryTimeValid(value time.Time, canonical bool) bool {
	return !value.IsZero() && (!canonical || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func invalidSourceHistoryTime(value time.Time) string { return value.Format(time.RFC3339Nano) }

func normalizeHistoricalUnboundTag(value contact.HistoricalUnboundTag) contact.HistoricalUnboundTag {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.RedactedRoots = invalidSourceHistoryCloneRoots(value.RedactedRoots)
	return value
}
func normalizeHistoricalInvalidChannel(value contact.HistoricalInvalidChannel) contact.HistoricalInvalidChannel {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	value.RedactedRoots = invalidSourceHistoryCloneRoots(value.RedactedRoots)
	return value
}
func invalidSourceHistoryCloneRoots(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}
func withHistoricalUnboundTagID(value contact.HistoricalUnboundTag, id int64) contact.HistoricalUnboundTag {
	value.ID = id
	return normalizeHistoricalUnboundTag(value)
}
func withHistoricalInvalidChannelID(value contact.HistoricalInvalidChannel, id int64) contact.HistoricalInvalidChannel {
	value.ID = id
	return normalizeHistoricalInvalidChannel(value)
}

func validInvalidSourceHistoryReceipt(receipt contact.InvalidSourceHistoryReceipt, kind, source string, payload [32]byte) bool {
	return receipt.Kind == kind && receipt.SourceIdentifier == source && receipt.PayloadDigest == payload && receipt.TargetID > 0 && receipt.TargetDigest != ([32]byte{})
}
func invalidSourceHistoryError(err error) error {
	if errors.Is(err, contact.ErrInvalidSourceHistoryInvalid) {
		return contact.ErrInvalidSourceHistoryInvalid
	}
	if errors.Is(err, contact.ErrInvalidSourceHistoryConflict) {
		return contact.ErrInvalidSourceHistoryConflict
	}
	return contact.ErrInvalidSourceHistoryUnavailable
}
func invalidSourceHistoryNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
