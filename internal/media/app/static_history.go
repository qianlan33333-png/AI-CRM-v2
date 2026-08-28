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

// StaticMediaHistoryWriter writes immutable V1 invitation metadata only. It
// does not touch current group invitations, queues, events, or Provider state.
type StaticMediaHistoryWriter struct {
	store   mediaport.StaticMediaHistoryStore
	journal mediaport.StaticMediaHistoryJournal
}

func NewStaticMediaHistoryWriter(store mediaport.StaticMediaHistoryStore, journal mediaport.StaticMediaHistoryJournal) (*StaticMediaHistoryWriter, error) {
	if staticMediaHistoryNil(store) || staticMediaHistoryNil(journal) {
		return nil, mediaport.ErrStaticMediaHistoryUnavailable
	}
	return &StaticMediaHistoryWriter{store: store, journal: journal}, nil
}

func (writer *StaticMediaHistoryWriter) ImportGroupInvite(ctx context.Context, sourceHex string, value mediaport.HistoricalGroupInvite) (mediaport.StaticMediaHistoryReceipt, error) {
	empty := mediaport.StaticMediaHistoryReceipt{}
	if writer == nil || ctx == nil || ctx.Err() != nil || staticMediaHistoryNil(writer.store) || staticMediaHistoryNil(writer.journal) {
		return empty, mediaport.ErrStaticMediaHistoryUnavailable
	}
	source, ok := staticMediaHistorySource(sourceHex)
	if !ok || value.ID != 0 || value.SourceKeyDigest != source || value.SourcePayloadDigest == ([sha256.Size]byte{}) || !validHistoricalGroupInvite(value, false) {
		return empty, mediaport.ErrStaticMediaHistoryInvalid
	}
	value = normalizeHistoricalGroupInvite(value)
	if _, err := HistoricalGroupInviteDigest(withHistoricalGroupInviteID(value, 1)); err != nil {
		return empty, mediaport.ErrStaticMediaHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadStaticMediaHistory(ctx, "group_invite", sourceHex)
	if err != nil {
		return empty, staticMediaHistoryError(err)
	}
	if found {
		if !validStaticMediaHistoryReceipt(receipt, sourceHex, value.SourcePayloadDigest) {
			return empty, mediaport.ErrStaticMediaHistoryConflict
		}
		actual, err := writer.store.GetHistoricalGroupInvite(ctx, receipt.TargetID)
		if err != nil {
			return empty, staticMediaHistoryError(err)
		}
		actualDigest, actualErr := HistoricalGroupInviteDigest(actual)
		expectedDigest, expectedErr := HistoricalGroupInviteDigest(withHistoricalGroupInviteID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, mediaport.ErrStaticMediaHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := writer.store.CreateHistoricalGroupInvite(ctx, value)
	if err != nil {
		return empty, staticMediaHistoryError(err)
	}
	actualDigest, actualErr := HistoricalGroupInviteDigest(actual)
	expectedDigest, expectedErr := HistoricalGroupInviteDigest(withHistoricalGroupInviteID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, mediaport.ErrStaticMediaHistoryConflict
	}
	receipt = mediaport.StaticMediaHistoryReceipt{Kind: "group_invite", SourceIdentifier: sourceHex, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err := writer.journal.RecordStaticMediaHistory(ctx, receipt); err != nil {
		return empty, staticMediaHistoryError(err)
	}
	return receipt, nil
}

// HistoricalGroupInviteDigest covers every stored field, including the target
// ID, so a replay cannot treat a changed immutable row as successful.
func HistoricalGroupInviteDigest(value mediaport.HistoricalGroupInvite) ([sha256.Size]byte, error) {
	if !validHistoricalGroupInvite(value, true) {
		return [sha256.Size]byte{}, mediaport.ErrStaticMediaHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                 string            `json:"kind"`
		ID                   int64             `json:"id"`
		SourceID             int64             `json:"source_id"`
		SourceKeyDigest      [sha256.Size]byte `json:"source_key_digest"`
		SourcePayloadDigest  [sha256.Size]byte `json:"source_payload_digest"`
		Name                 string            `json:"name"`
		Title                string            `json:"title"`
		Description          string            `json:"description"`
		OriginalState        string            `json:"original_state"`
		OriginalAutoCreate   bool              `json:"original_auto_create"`
		RoomBaseName         string            `json:"room_base_name"`
		RoomBaseSourceID     *int64            `json:"room_base_source_id"`
		OriginalEnabled      bool              `json:"original_enabled"`
		OriginalBindingState string            `json:"original_binding_state"`
		CreatedAt            string            `json:"created_at"`
		UpdatedAt            string            `json:"updated_at"`
	}{
		Kind: "v1.group_invite", ID: value.ID, SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest,
		SourcePayloadDigest: value.SourcePayloadDigest, Name: value.Name, Title: value.Title, Description: value.Description,
		OriginalState: value.OriginalState, OriginalAutoCreate: value.OriginalAutoCreate, RoomBaseName: value.RoomBaseName,
		RoomBaseSourceID: value.RoomBaseSourceID, OriginalEnabled: value.OriginalEnabled, OriginalBindingState: value.OriginalBindingState,
		CreatedAt: staticMediaHistoryTime(value.CreatedAt), UpdatedAt: staticMediaHistoryTime(value.UpdatedAt),
	})
	if err != nil {
		return [sha256.Size]byte{}, mediaport.ErrStaticMediaHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validHistoricalGroupInvite(value mediaport.HistoricalGroupInvite, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && staticMediaHistoryText(value.Name) && staticMediaHistoryText(value.Title) &&
		staticMediaHistoryText(value.Description) && staticMediaHistoryText(value.OriginalState) && staticMediaHistoryText(value.RoomBaseName) &&
		staticMediaHistoryText(value.OriginalBindingState) && staticMediaHistoryTimeValid(value.CreatedAt, stored) && staticMediaHistoryTimeValid(value.UpdatedAt, stored)
}

func staticMediaHistorySource(value string) ([sha256.Size]byte, bool) {
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

func normalizeHistoricalGroupInvite(value mediaport.HistoricalGroupInvite) mediaport.HistoricalGroupInvite {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	if value.RoomBaseSourceID != nil {
		id := *value.RoomBaseSourceID
		value.RoomBaseSourceID = &id
	}
	return value
}

func withHistoricalGroupInviteID(value mediaport.HistoricalGroupInvite, id int64) mediaport.HistoricalGroupInvite {
	value.ID = id
	return normalizeHistoricalGroupInvite(value)
}

func staticMediaHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
func staticMediaHistoryTimeValid(value time.Time, canonical bool) bool {
	return !value.IsZero() && (!canonical || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func staticMediaHistoryTime(value time.Time) string { return value.Format(time.RFC3339Nano) }
func validStaticMediaHistoryReceipt(value mediaport.StaticMediaHistoryReceipt, source string, payload [sha256.Size]byte) bool {
	return value.Kind == "group_invite" && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([sha256.Size]byte{})
}
func staticMediaHistoryError(err error) error {
	switch {
	case errors.Is(err, mediaport.ErrStaticMediaHistoryInvalid):
		return mediaport.ErrStaticMediaHistoryInvalid
	case errors.Is(err, mediaport.ErrStaticMediaHistoryConflict):
		return mediaport.ErrStaticMediaHistoryConflict
	default:
		return mediaport.ErrStaticMediaHistoryUnavailable
	}
}
func staticMediaHistoryNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Ptr && v.IsNil()
}
