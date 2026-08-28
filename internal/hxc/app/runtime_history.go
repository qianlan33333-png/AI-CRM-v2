package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

// HXCRuntimeHistoryWriter records immutable V1 sender/runtime observations only.
type HXCRuntimeHistoryWriter struct {
	store   hxc.HXCRuntimeHistoryStore
	journal hxc.HXCHistoryJournal
}

func NewHXCRuntimeHistoryWriter(store hxc.HXCRuntimeHistoryStore, journal hxc.HXCHistoryJournal) (*HXCRuntimeHistoryWriter, error) {
	if nilHXC(store) || nilHXC(journal) {
		return nil, hxc.ErrHXCHistoryUnavailable
	}
	return &HXCRuntimeHistoryWriter{store: store, journal: journal}, nil
}

func (w *HXCRuntimeHistoryWriter) ImportSenderConfig(ctx context.Context, source string, value hxc.HistoricalHXCSenderConfig) (hxc.HXCHistoryReceipt, error) {
	value = normalizeRuntimeSenderConfig(value)
	return importHXCRuntime(w, ctx, hxc.HXCHistorySenderConfig, source, value, HistoricalHXCSenderConfigDigest,
		func(v hxc.HistoricalHXCSenderConfig, id int64) hxc.HistoricalHXCSenderConfig { v.ID = id; return v },
		func(v hxc.HistoricalHXCSenderConfig) bool { return validRuntimeSenderConfig(v, false) },
		func() (hxc.HistoricalHXCSenderConfig, error) {
			return w.store.CreateHistoricalHXCSenderConfig(ctx, value)
		},
		func(id int64) (hxc.HistoricalHXCSenderConfig, error) {
			return w.store.GetHistoricalHXCSenderConfig(ctx, id)
		})
}

func (w *HXCRuntimeHistoryWriter) ImportSendRecord(ctx context.Context, source string, value hxc.HistoricalHXCSendRecord) (hxc.HXCHistoryReceipt, error) {
	value = normalizeRuntimeSendRecord(value)
	return importHXCRuntime(w, ctx, hxc.HXCHistorySendRecord, source, value, HistoricalHXCSendRecordDigest,
		func(v hxc.HistoricalHXCSendRecord, id int64) hxc.HistoricalHXCSendRecord { v.ID = id; return v },
		func(v hxc.HistoricalHXCSendRecord) bool { return validRuntimeSendRecord(v, false) },
		func() (hxc.HistoricalHXCSendRecord, error) { return w.store.CreateHistoricalHXCSendRecord(ctx, value) },
		func(id int64) (hxc.HistoricalHXCSendRecord, error) {
			return w.store.GetHistoricalHXCSendRecord(ctx, id)
		})
}

func importHXCRuntime[T any](w *HXCRuntimeHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, valid func(T) bool, create func() (T, error), get func(int64) (T, error)) (hxc.HXCHistoryReceipt, error) {
	var empty hxc.HXCHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilHXC(w.store) || nilHXC(w.journal) {
		return empty, hxc.ErrHXCHistoryUnavailable
	}
	key, payload, id, ok := runtimeIdentity(value)
	if !ok || !valid(value) || source != hex.EncodeToString(key[:]) || id != 0 {
		return empty, hxc.ErrHXCHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, hxc.ErrHXCHistoryInvalid
	}
	receipt, found, err := w.journal.LoadHXCHistory(ctx, kind, source)
	if err != nil {
		return empty, hxcHistoryError(err)
	}
	if found {
		if !validReceipt(receipt, kind, source, payload) {
			return empty, hxc.ErrHXCHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, hxcHistoryError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, hxc.ErrHXCHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, hxcHistoryError(err)
	}
	_, _, targetID, ok := runtimeIdentity(actual)
	if !ok || targetID < 1 {
		return empty, hxc.ErrHXCHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, hxc.ErrHXCHistoryConflict
	}
	receipt = hxc.HXCHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: targetID, TargetDigest: actualDigest}
	if err := w.journal.RecordHXCHistory(ctx, receipt); err != nil {
		return empty, hxcHistoryError(err)
	}
	return receipt, nil
}

func HistoricalHXCSenderConfigDigest(value hxc.HistoricalHXCSenderConfig) ([32]byte, error) {
	if !validRuntimeSenderConfig(value, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return digestHXCRuntime(hxc.HXCHistorySenderConfig, struct {
		ID, SourceID                                                           int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest, PrivateDigest [32]byte
		Priority                                                               int64
		OriginalIsActive                                                       bool
		CreatedAt, UpdatedAt                                                   time.Time
	}{value.ID, value.SourceID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.PrivateDigest, value.Priority, value.OriginalIsActive, value.CreatedAt, value.UpdatedAt})
}

func HistoricalHXCSendRecordDigest(value hxc.HistoricalHXCSendRecord) ([32]byte, error) {
	if !validRuntimeSendRecord(value, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return digestHXCRuntime(hxc.HXCHistorySendRecord, struct {
		ID, SourceID                                                           int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest, PrivateDigest [32]byte
		TaskType, OriginalStatus, TargetSource                                 string
		SelectedCount, EligibleCount, SentCount, SkippedCount                  int64
		PlannedCount, QueuedCount, DispatchingCount, SucceededCount            int64
		FailedCount, BlockedCount, CancelledCount, ImageCount                  int64
		IncludeDoNotDisturb                                                    bool
		TargetSourceID                                                         *int64
		CreatedAt                                                              time.Time
		LastStatusSyncAt, LastRefreshedAt                                      *time.Time
	}{value.ID, value.SourceID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.PrivateDigest, value.TaskType, value.OriginalStatus, value.TargetSource, value.SelectedCount, value.EligibleCount, value.SentCount, value.SkippedCount, value.PlannedCount, value.QueuedCount, value.DispatchingCount, value.SucceededCount, value.FailedCount, value.BlockedCount, value.CancelledCount, value.ImageCount, value.IncludeDoNotDisturb, value.TargetSourceID, value.CreatedAt, value.LastStatusSyncAt, value.LastRefreshedAt})
}

func digestHXCRuntime(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{Kind: kind, Value: value})
	if err != nil {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func runtimeIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch value := value.(type) {
	case hxc.HistoricalHXCSenderConfig:
		return value.SourceKeyDigest, value.SourcePayloadDigest, value.ID, true
	case hxc.HistoricalHXCSendRecord:
		return value.SourceKeyDigest, value.SourcePayloadDigest, value.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}

func validRuntimeIdentity(value hxc.HistoricalHXCRuntimeIdentity, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && value.SourceKeyDigest != ([32]byte{}) && value.SourcePayloadDigest != ([32]byte{}) && value.SourceFieldDigest != ([32]byte{}) && value.PrivateDigest != ([32]byte{})
}

func validRuntimeSenderConfig(value hxc.HistoricalHXCSenderConfig, stored bool) bool {
	return validRuntimeIdentity(value.HistoricalHXCRuntimeIdentity, stored) && validTime(value.CreatedAt, stored) && validTime(value.UpdatedAt, stored)
}

func validRuntimeSendRecord(value hxc.HistoricalHXCSendRecord, stored bool) bool {
	return validRuntimeIdentity(value.HistoricalHXCRuntimeIdentity, stored) && validTime(value.CreatedAt, stored) && validOptionalTime(value.LastStatusSyncAt, stored) && validOptionalTime(value.LastRefreshedAt, stored)
}

func normalizeRuntimeSenderConfig(value hxc.HistoricalHXCSenderConfig) hxc.HistoricalHXCSenderConfig {
	value.CreatedAt = normalizeTime(value.CreatedAt)
	value.UpdatedAt = normalizeTime(value.UpdatedAt)
	return value
}

func normalizeRuntimeSendRecord(value hxc.HistoricalHXCSendRecord) hxc.HistoricalHXCSendRecord {
	value.CreatedAt = normalizeTime(value.CreatedAt)
	value.LastStatusSyncAt = normalizePTime(value.LastStatusSyncAt)
	value.LastRefreshedAt = normalizePTime(value.LastRefreshedAt)
	if value.TargetSourceID != nil {
		target := *value.TargetSourceID
		value.TargetSourceID = &target
	}
	return value
}
