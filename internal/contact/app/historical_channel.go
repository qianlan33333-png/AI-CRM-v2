package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// HistoricalChannelWriter writes only a disabled local definition and its
// migration-owned provenance. The Store and Journal use the caller's one
// transaction; this writer intentionally has no UoW, event, or Provider seam.
type HistoricalChannelWriter struct {
	store   contactport.HistoricalChannelStore
	journal contactport.HistoricalChannelJournal
}

func NewHistoricalChannelWriter(store contactport.HistoricalChannelStore, journal contactport.HistoricalChannelJournal) (*HistoricalChannelWriter, error) {
	if historicalChannelNil(store) || historicalChannelNil(journal) {
		return nil, contactport.ErrHistoricalChannelUnavailable
	}
	return &HistoricalChannelWriter{store: store, journal: journal}, nil
}

func (writer *HistoricalChannelWriter) Import(ctx context.Context, definition contactport.HistoricalChannelDefinition) (contactport.HistoricalChannelReceipt, error) {
	if writer == nil || historicalChannelNil(writer.store) || historicalChannelNil(writer.journal) || ctx == nil || ctx.Err() != nil {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelUnavailable
	}
	expected, err := historicalChannelRecord(definition)
	if err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}

	existing, found, err := writer.journal.LoadHistoricalChannel(ctx, definition.SourceIdentifier)
	if err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	if found {
		if !sameHistoricalChannelFact(existing, definition) || existing.TargetID < 1 || existing.TargetDigest == ([sha256.Size]byte{}) {
			return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
		}
		stored, getErr := writer.store.GetHistoricalChannel(ctx, existing.TargetID)
		if getErr != nil {
			return contactport.HistoricalChannelReceipt{}, getErr
		}
		if !sameHistoricalChannelRecord(stored, expected) {
			return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
		}
		digest, digestErr := HistoricalChannelTargetDigest(stored)
		if digestErr != nil || digest != existing.TargetDigest {
			return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
		}
		existing.Replayed = true
		return existing, nil
	}

	stored, err := writer.store.CreateHistoricalChannel(ctx, expected)
	if err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	if !sameHistoricalChannelRecord(stored, expected) {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
	}
	digest, err := HistoricalChannelTargetDigest(stored)
	if err != nil {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
	}
	receipt := contactport.HistoricalChannelReceipt{
		SourceIdentifier: definition.SourceIdentifier,
		PayloadDigest:    definition.PayloadDigest,
		TargetID:         stored.ID,
		TargetDigest:     digest,
	}
	if err = writer.journal.RecordHistoricalChannel(ctx, receipt); err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	return receipt, nil
}

// HistoricalChannelTargetDigest is stable across PostgreSQL jsonb formatting.
// It includes every static target field so a replay can reject target drift.
func HistoricalChannelTargetDigest(record contactport.HistoricalChannelRecord) ([sha256.Size]byte, error) {
	if !validHistoricalChannelStored(record) {
		return [sha256.Size]byte{}, contactport.ErrHistoricalChannelInvalid
	}
	projection, err := canonicalJSON(record.Projection)
	if err != nil {
		return [sha256.Size]byte{}, contactport.ErrHistoricalChannelInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                                   string `json:"kind"`
		ID                                     int64  `json:"id"`
		Code, Name, Status, LegacyConfigDigest string
		Projection                             json.RawMessage
		CreatedBy, UpdatedBy                   int64
		CreatedAt, UpdatedAt                   string
	}{
		Kind: "v1.inactive_channel", ID: record.ID, Code: record.Code, Name: record.Name, Status: record.Status,
		LegacyConfigDigest: record.LegacyConfigDigest, Projection: projection,
		CreatedBy: record.CreatedBy, UpdatedBy: record.UpdatedBy,
		CreatedAt: historicalChannelTime(record.CreatedAt), UpdatedAt: historicalChannelTime(record.UpdatedAt),
	})
	if err != nil {
		return [sha256.Size]byte{}, contactport.ErrHistoricalChannelInvalid
	}
	return sha256.Sum256(encoded), nil
}

func historicalChannelRecord(definition contactport.HistoricalChannelDefinition) (contactport.HistoricalChannelRecord, error) {
	if !validHistoricalChannelDefinition(definition) {
		return contactport.HistoricalChannelRecord{}, contactport.ErrHistoricalChannelInvalid
	}
	raw, err := json.Marshal(map[string]string{"channel_type": definition.ChannelType, "carrier_type": definition.CarrierType})
	if err != nil {
		return contactport.HistoricalChannelRecord{}, contactport.ErrHistoricalChannelInvalid
	}
	projection, err := normalizeProjection(raw, definition.Code, definition.Name, "inactive")
	if err != nil {
		return contactport.HistoricalChannelRecord{}, contactport.ErrHistoricalChannelInvalid
	}
	return contactport.HistoricalChannelRecord{
		Code: definition.Code, Name: definition.Name, Status: "inactive", Projection: projection,
		LegacyConfigDigest: definition.LegacyConfigDigest, CreatedBy: definition.Actor, UpdatedBy: definition.Actor,
		CreatedAt: normalizeHistoricalChannelTime(definition.CreatedAt), UpdatedAt: normalizeHistoricalChannelTime(definition.UpdatedAt),
	}, nil
}

func validHistoricalChannelDefinition(definition contactport.HistoricalChannelDefinition) bool {
	return validHistoricalChannelSource(definition.SourceIdentifier) && definition.PayloadDigest != ([sha256.Size]byte{}) &&
		validText(definition.Code, 200) && validText(definition.Name, 200) && validHistoricalChannelKind(definition.ChannelType) &&
		validHistoricalChannelKind(definition.CarrierType) && validHistoricalChannelDigest(definition.LegacyConfigDigest) &&
		definition.Actor > 0 && validHistoricalChannelTimes(definition.CreatedAt, definition.UpdatedAt)
}

func validHistoricalChannelStored(record contactport.HistoricalChannelRecord) bool {
	return record.ID > 0 && validText(record.Code, 200) && validText(record.Name, 200) && record.Status == "inactive" &&
		validHistoricalChannelDigest(record.LegacyConfigDigest) && record.CreatedBy > 0 && record.UpdatedBy == record.CreatedBy &&
		validHistoricalChannelTimes(record.CreatedAt, record.UpdatedAt) && json.Valid(record.Projection)
}

func sameHistoricalChannelFact(receipt contactport.HistoricalChannelReceipt, definition contactport.HistoricalChannelDefinition) bool {
	return receipt.SourceIdentifier == definition.SourceIdentifier && receipt.PayloadDigest == definition.PayloadDigest
}

func sameHistoricalChannelRecord(actual, expected contactport.HistoricalChannelRecord) bool {
	return validHistoricalChannelStored(actual) && actual.Code == expected.Code && actual.Name == expected.Name && actual.Status == expected.Status &&
		actual.LegacyConfigDigest == expected.LegacyConfigDigest && actual.CreatedBy == expected.CreatedBy && actual.UpdatedBy == expected.UpdatedBy &&
		actual.CreatedAt.Equal(expected.CreatedAt) && actual.UpdatedAt.Equal(expected.UpdatedAt) && jsonEquivalent(actual.Projection, expected.Projection)
}

func validHistoricalChannelSource(value string) bool { return validText(value, 512) }
func validHistoricalChannelKind(value string) bool   { return value == "qrcode" || value == "link" }

func validHistoricalChannelDigest(value string) bool {
	if len(value) != len("sha256:")+hex.EncodedLen(sha256.Size) || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func validHistoricalChannelTimes(created, updated time.Time) bool {
	return !created.IsZero() && !updated.IsZero() && !updated.Before(created)
}

func normalizeHistoricalChannelTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
func historicalChannelTime(value time.Time) string {
	return normalizeHistoricalChannelTime(value).Format(time.RFC3339Nano)
}

func historicalChannelNil(value any) bool {
	if value == nil {
		return true
	}
	raw := reflect.ValueOf(value)
	return raw.Kind() == reflect.Ptr && raw.IsNil()
}
