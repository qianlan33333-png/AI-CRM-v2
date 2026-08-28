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

const radarClickHistoryKind = "radar_click"

// RadarClickHistoryWriter writes immutable V1 click observations only. It has
// no dependency on the current Radar tracking, event, queue, or Provider path.
type RadarClickHistoryWriter struct {
	store   radarport.RadarClickHistoryStore
	journal radarport.RadarClickHistoryJournal
}

func NewRadarClickHistoryWriter(store radarport.RadarClickHistoryStore, journal radarport.RadarClickHistoryJournal) (*RadarClickHistoryWriter, error) {
	if nilRadarClickHistoryDependency(store) || nilRadarClickHistoryDependency(journal) {
		return nil, radarport.ErrRadarClickHistoryUnavailable
	}
	return &RadarClickHistoryWriter{store: store, journal: journal}, nil
}

func (writer *RadarClickHistoryWriter) ImportHistoricalRadarClick(ctx context.Context, sourceIdentifier string, value radarport.HistoricalRadarClick) (radarport.RadarClickHistoryReceipt, error) {
	empty := radarport.RadarClickHistoryReceipt{}
	if writer == nil || ctx == nil || ctx.Err() != nil || nilRadarClickHistoryDependency(writer.store) || nilRadarClickHistoryDependency(writer.journal) {
		return empty, radarport.ErrRadarClickHistoryUnavailable
	}
	value = normalizeHistoricalRadarClick(value)
	if !validHistoricalRadarClick(value, false) || sourceIdentifier != hex.EncodeToString(value.SourceKeyDigest[:]) {
		return empty, radarport.ErrRadarClickHistoryInvalid
	}
	if _, err := HistoricalRadarClickDigest(withHistoricalRadarClickID(value, 1)); err != nil {
		return empty, radarport.ErrRadarClickHistoryInvalid
	}

	receipt, found, err := writer.journal.LoadRadarClickHistory(ctx, radarClickHistoryKind, sourceIdentifier)
	if err != nil {
		return empty, radarClickHistoryError(err)
	}
	if found {
		if !validRadarClickHistoryReceipt(receipt, sourceIdentifier, value.SourcePayloadDigest) {
			return empty, radarport.ErrRadarClickHistoryConflict
		}
		actual, err := writer.store.GetHistoricalRadarClick(ctx, receipt.TargetID)
		if err != nil {
			return empty, radarClickHistoryError(err)
		}
		actualDigest, actualErr := HistoricalRadarClickDigest(actual)
		expectedDigest, expectedErr := HistoricalRadarClickDigest(withHistoricalRadarClickID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, radarport.ErrRadarClickHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}

	actual, err := writer.store.CreateHistoricalRadarClick(ctx, value)
	if err != nil {
		return empty, radarClickHistoryError(err)
	}
	actualDigest, actualErr := HistoricalRadarClickDigest(actual)
	expectedDigest, expectedErr := HistoricalRadarClickDigest(withHistoricalRadarClickID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, radarport.ErrRadarClickHistoryConflict
	}
	receipt = radarport.RadarClickHistoryReceipt{Kind: radarClickHistoryKind, SourceIdentifier: sourceIdentifier, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err := writer.journal.RecordRadarClickHistory(ctx, receipt); err != nil {
		return empty, radarClickHistoryError(err)
	}
	return receipt, nil
}

// HistoricalRadarClickDigest covers every persisted typed field, including all
// source and private-field digests, so journal replay cannot hide target drift.
func HistoricalRadarClickDigest(value radarport.HistoricalRadarClick) ([32]byte, error) {
	value = normalizeHistoricalRadarClick(value)
	if !validHistoricalRadarClick(value, true) {
		return [32]byte{}, radarport.ErrRadarClickHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		ID                     int64
		SourceKeyDigest        [32]byte
		SourcePayloadDigest    [32]byte
		SourceFieldDigest      [32]byte
		SourceID               int64
		LinkSourceID           int64
		RadarLinkID            *int64
		CustomerID             *int64
		Code                   string
		RawStage               string
		SourceChannel          string
		TargetTypeSnapshot     string
		SourceChannelSnapshot  string
		ErrorCode              string
		CreatedAt              time.Time
		OpenIDDigest           [32]byte
		UnionIDDigest          [32]byte
		ExternalUserIDDigest   [32]byte
		CampaignIDDigest       [32]byte
		StaffIDDigest          [32]byte
		UserAgentDigest        [32]byte
		IPDigest               [32]byte
		PersonIDDigest         [32]byte
		IPHashDigest           [32]byte
		CampaignSnapshotDigest [32]byte
		StaffSnapshotDigest    [32]byte
		RefererDigest          [32]byte
		QueryParamsDigest      [32]byte
	}{
		value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.SourceID, value.LinkSourceID,
		value.RadarLinkID, value.CustomerID, value.Code, value.RawStage, value.SourceChannel, value.TargetTypeSnapshot,
		value.SourceChannelSnapshot, value.ErrorCode, value.CreatedAt, value.OpenIDDigest, value.UnionIDDigest,
		value.ExternalUserIDDigest, value.CampaignIDDigest, value.StaffIDDigest, value.UserAgentDigest, value.IPDigest,
		value.PersonIDDigest, value.IPHashDigest, value.CampaignSnapshotDigest, value.StaffSnapshotDigest, value.RefererDigest,
		value.QueryParamsDigest,
	})
	if err != nil {
		return [32]byte{}, radarport.ErrRadarClickHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validHistoricalRadarClick(value radarport.HistoricalRadarClick, stored bool) bool {
	if (stored && value.ID < 1) || (!stored && value.ID != 0) || value.SourceID < 1 || value.LinkSourceID < 1 ||
		value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.SourceFieldDigest == ([32]byte{}) || value.CreatedAt.IsZero() ||
		(value.RadarLinkID != nil && *value.RadarLinkID < 1) || (value.CustomerID != nil && *value.CustomerID < 1) {
		return false
	}
	for _, value := range []string{value.Code, value.RawStage, value.SourceChannel, value.TargetTypeSnapshot, value.SourceChannelSnapshot, value.ErrorCode} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return false
		}
	}
	for _, digest := range [][32]byte{value.OpenIDDigest, value.UnionIDDigest, value.ExternalUserIDDigest, value.CampaignIDDigest, value.StaffIDDigest, value.UserAgentDigest, value.IPDigest, value.PersonIDDigest, value.IPHashDigest, value.CampaignSnapshotDigest, value.StaffSnapshotDigest, value.RefererDigest, value.QueryParamsDigest} {
		if digest == ([32]byte{}) {
			return false
		}
	}
	return !stored || value.CreatedAt.Equal(value.CreatedAt.UTC().Truncate(time.Microsecond))
}

func normalizeHistoricalRadarClick(value radarport.HistoricalRadarClick) radarport.HistoricalRadarClick {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	if value.RadarLinkID != nil {
		id := *value.RadarLinkID
		value.RadarLinkID = &id
	}
	if value.CustomerID != nil {
		id := *value.CustomerID
		value.CustomerID = &id
	}
	return value
}

func withHistoricalRadarClickID(value radarport.HistoricalRadarClick, id int64) radarport.HistoricalRadarClick {
	value.ID = id
	return normalizeHistoricalRadarClick(value)
}

func validRadarClickHistoryReceipt(value radarport.RadarClickHistoryReceipt, source string, payload [32]byte) bool {
	return value.Kind == radarClickHistoryKind && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([32]byte{})
}

func radarClickHistoryError(err error) error {
	switch {
	case errors.Is(err, radarport.ErrRadarClickHistoryInvalid):
		return radarport.ErrRadarClickHistoryInvalid
	case errors.Is(err, radarport.ErrRadarClickHistoryConflict):
		return radarport.ErrRadarClickHistoryConflict
	default:
		return radarport.ErrRadarClickHistoryUnavailable
	}
}

func nilRadarClickHistoryDependency(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil()
}
