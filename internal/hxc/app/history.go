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

	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

type HXCHistoryWriter struct {
	store   hxc.HXCHistoryStore
	journal hxc.HXCHistoryJournal
}

func NewHXCHistoryWriter(store hxc.HXCHistoryStore, journal hxc.HXCHistoryJournal) (*HXCHistoryWriter, error) {
	if nilHXC(store) || nilHXC(journal) {
		return nil, hxc.ErrHXCHistoryUnavailable
	}
	return &HXCHistoryWriter{store: store, journal: journal}, nil
}

func (w *HXCHistoryWriter) ImportMeta(ctx context.Context, source string, v hxc.HistoricalHXCMeta) (hxc.HXCHistoryReceipt, error) {
	v = normalizeMeta(v)
	return importHXC(w, ctx, hxc.HXCHistoryMeta, source, v, HistoricalHXCMetaDigest, func(x hxc.HistoricalHXCMeta, id int64) hxc.HistoricalHXCMeta { x.ID = id; return x }, func(x hxc.HistoricalHXCMeta) bool { return validMeta(x, false) }, func() (hxc.HistoricalHXCMeta, error) { return w.store.CreateHistoricalHXCMeta(ctx, v) }, func(id int64) (hxc.HistoricalHXCMeta, error) { return w.store.GetHistoricalHXCMeta(ctx, id) })
}
func (w *HXCHistoryWriter) ImportSnapshot(ctx context.Context, source string, v hxc.HistoricalHXCSnapshot) (hxc.HXCHistoryReceipt, error) {
	v = normalizeSnapshot(v)
	return importHXC(w, ctx, hxc.HXCHistorySnapshot, source, v, HistoricalHXCSnapshotDigest, func(x hxc.HistoricalHXCSnapshot, id int64) hxc.HistoricalHXCSnapshot { x.ID = id; return x }, func(x hxc.HistoricalHXCSnapshot) bool { return validSnapshot(x, false) }, func() (hxc.HistoricalHXCSnapshot, error) { return w.store.CreateHistoricalHXCSnapshot(ctx, v) }, func(id int64) (hxc.HistoricalHXCSnapshot, error) { return w.store.GetHistoricalHXCSnapshot(ctx, id) })
}
func (w *HXCHistoryWriter) ImportActivation(ctx context.Context, source string, v hxc.HistoricalHXCActivation) (hxc.HXCHistoryReceipt, error) {
	v = normalizeActivation(v)
	kind := activationKind(v.SourceTable)
	if kind == "" {
		return hxc.HXCHistoryReceipt{}, hxc.ErrHXCHistoryInvalid
	}
	return importHXC(w, ctx, kind, source, v, HistoricalHXCActivationDigest, func(x hxc.HistoricalHXCActivation, id int64) hxc.HistoricalHXCActivation { x.ID = id; return x }, func(x hxc.HistoricalHXCActivation) bool { return validActivation(x, false) }, func() (hxc.HistoricalHXCActivation, error) { return w.store.CreateHistoricalHXCActivation(ctx, v) }, func(id int64) (hxc.HistoricalHXCActivation, error) {
		return w.store.GetHistoricalHXCActivation(ctx, id)
	})
}
func (w *HXCHistoryWriter) ImportLead(ctx context.Context, source string, v hxc.HistoricalHXCLead) (hxc.HXCHistoryReceipt, error) {
	v = normalizeLead(v)
	return importHXC(w, ctx, hxc.HXCHistoryLead, source, v, HistoricalHXCLeadDigest, func(x hxc.HistoricalHXCLead, id int64) hxc.HistoricalHXCLead { x.ID = id; return x }, func(x hxc.HistoricalHXCLead) bool { return validLead(x, false) }, func() (hxc.HistoricalHXCLead, error) { return w.store.CreateHistoricalHXCLead(ctx, v) }, func(id int64) (hxc.HistoricalHXCLead, error) { return w.store.GetHistoricalHXCLead(ctx, id) })
}
func (w *HXCHistoryWriter) ImportBatch(ctx context.Context, source string, v hxc.HistoricalHXCBatch) (hxc.HXCHistoryReceipt, error) {
	v = normalizeBatch(v)
	return importHXC(w, ctx, hxc.HXCHistoryBatch, source, v, HistoricalHXCBatchDigest, func(x hxc.HistoricalHXCBatch, id int64) hxc.HistoricalHXCBatch { x.ID = id; return x }, func(x hxc.HistoricalHXCBatch) bool { return validBatch(x, false) }, func() (hxc.HistoricalHXCBatch, error) { return w.store.CreateHistoricalHXCBatch(ctx, v) }, func(id int64) (hxc.HistoricalHXCBatch, error) { return w.store.GetHistoricalHXCBatch(ctx, id) })
}

func importHXC[T any](w *HXCHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, valid func(T) bool, create func() (T, error), get func(int64) (T, error)) (hxc.HXCHistoryReceipt, error) {
	var empty hxc.HXCHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilHXC(w.store) || nilHXC(w.journal) {
		return empty, hxc.ErrHXCHistoryUnavailable
	}
	key, payload, id, ok := hxcIdentity(value)
	if !ok || !valid(value) || kind == "" || source != hex.EncodeToString(key[:]) || payload == ([32]byte{}) || id != 0 {
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
		a, ae := digest(actual)
		e, ee := digest(withID(value, receipt.TargetID))
		if ae != nil || ee != nil || a != e || a != receipt.TargetDigest {
			return empty, hxc.ErrHXCHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, hxcHistoryError(err)
	}
	_, _, target, ok := hxcIdentity(actual)
	if !ok || target < 1 {
		return empty, hxc.ErrHXCHistoryConflict
	}
	a, ae := digest(actual)
	e, ee := digest(withID(value, target))
	if ae != nil || ee != nil || a != e {
		return empty, hxc.ErrHXCHistoryConflict
	}
	receipt = hxc.HXCHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: target, TargetDigest: a}
	if err := w.journal.RecordHXCHistory(ctx, receipt); err != nil {
		return empty, hxcHistoryError(err)
	}
	return receipt, nil
}

func HistoricalHXCMetaDigest(v hxc.HistoricalHXCMeta) ([32]byte, error) {
	if !validMeta(v, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return digestHXC("meta", v)
}
func HistoricalHXCSnapshotDigest(v hxc.HistoricalHXCSnapshot) ([32]byte, error) {
	if !validSnapshot(v, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return digestHXC("snapshot", v)
}
func HistoricalHXCActivationDigest(v hxc.HistoricalHXCActivation) ([32]byte, error) {
	if !validActivation(v, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return digestHXC("activation", v)
}
func HistoricalHXCLeadDigest(v hxc.HistoricalHXCLead) ([32]byte, error) {
	if !validLead(v, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return digestHXC("lead", v)
}
func HistoricalHXCBatchDigest(v hxc.HistoricalHXCBatch) ([32]byte, error) {
	if !validBatch(v, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return digestHXC("batch", v)
}
func digestHXC(kind string, v any) ([32]byte, error) {
	b, e := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{kind, v})
	if e != nil {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return sha256.Sum256(b), nil
}

func validIdentity(v hxc.HistoricalHXCIdentity, stored bool) bool {
	return (stored && v.ID > 0 || !stored && v.ID == 0) && v.SourceKeyDigest != ([32]byte{}) && v.SourcePayloadDigest != ([32]byte{})
}
func validTime(t time.Time, stored bool) bool {
	return !t.IsZero() && (!stored || t.Location() == time.UTC && t.Equal(t.UTC().Truncate(time.Microsecond)))
}
func validOptionalTime(t *time.Time, stored bool) bool { return t == nil || validTime(*t, stored) }
func validDate(v *string) bool {
	if v == nil {
		return true
	}
	_, e := time.Parse("2006-01-02", *v)
	return e == nil && len(*v) == 10
}
func validCustomer(v *int64) bool { return v == nil || *v > 0 }
func validMeta(v hxc.HistoricalHXCMeta, stored bool) bool {
	return validIdentity(v.HistoricalHXCIdentity, stored) && validTime(v.StartedAt, stored) && validOptionalTime(v.FinishedAt, stored)
}
func validSnapshot(v hxc.HistoricalHXCSnapshot, stored bool) bool {
	return validIdentity(v.HistoricalHXCIdentity, stored) && v.Observation == "observed_snapshot" && validCustomer(v.CustomerID) && validTime(v.ObservedAt, stored) && validDate(v.CRMCreatedAt) && validDate(v.LastQuestionnaireAt) && validDate(v.SubscriptionPeriodStart) && validOptionalTime(v.HXCRegisteredAt, stored) && validOptionalTime(v.HXCLastLoginAt, stored) && validOptionalTime(v.MembershipEndAt, stored) && validOptionalTime(v.LastMessageAt, stored) && validOptionalTime(v.SubscriptionExpires, stored)
}
func validActivation(v hxc.HistoricalHXCActivation, stored bool) bool {
	return validIdentity(v.HistoricalHXCIdentity, stored) && activationKind(v.SourceTable) != "" && validTime(v.CreatedAt, stored) && validTime(v.UpdatedAt, stored)
}
func validLead(v hxc.HistoricalHXCLead, stored bool) bool {
	return validIdentity(v.HistoricalHXCIdentity, stored) && validTime(v.CreatedAt, stored) && validTime(v.UpdatedAt, stored)
}
func validBatch(v hxc.HistoricalHXCBatch, stored bool) bool {
	return validIdentity(v.HistoricalHXCIdentity, stored) && validTime(v.CreatedAt, stored)
}
func activationKind(table string) string {
	if table == "public/user_ops_activation_status_source" {
		return hxc.HXCHistoryActivationStatus
	}
	if table == "public/user_ops_huangxiaocan_activation_source" {
		return hxc.HXCHistoryHuangxiaocanActivation
	}
	return ""
}
func hxcIdentity(v any) ([32]byte, [32]byte, int64, bool) {
	switch x := v.(type) {
	case hxc.HistoricalHXCMeta:
		return x.SourceKeyDigest, x.SourcePayloadDigest, x.ID, true
	case hxc.HistoricalHXCSnapshot:
		return x.SourceKeyDigest, x.SourcePayloadDigest, x.ID, true
	case hxc.HistoricalHXCActivation:
		return x.SourceKeyDigest, x.SourcePayloadDigest, x.ID, true
	case hxc.HistoricalHXCLead:
		return x.SourceKeyDigest, x.SourcePayloadDigest, x.ID, true
	case hxc.HistoricalHXCBatch:
		return x.SourceKeyDigest, x.SourcePayloadDigest, x.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}
func hxcID(v any) (int64, error) {
	_, _, id, ok := hxcIdentity(withHXCID(v, 1))
	if !ok {
		return 0, hxc.ErrHXCHistoryInvalid
	}
	return id, nil
}
func withHXCID(v any, id int64) any {
	switch x := v.(type) {
	case hxc.HistoricalHXCMeta:
		x.ID = id
		return x
	case hxc.HistoricalHXCSnapshot:
		x.ID = id
		return x
	case hxc.HistoricalHXCActivation:
		x.ID = id
		return x
	case hxc.HistoricalHXCLead:
		x.ID = id
		return x
	case hxc.HistoricalHXCBatch:
		x.ID = id
		return x
	}
	return nil
}
func normalizeTime(v time.Time) time.Time { return v.UTC().Truncate(time.Microsecond) }
func normalizePTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	x := normalizeTime(*v)
	return &x
}
func normalizePString(v *string) *string {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
func normalizeMeta(v hxc.HistoricalHXCMeta) hxc.HistoricalHXCMeta {
	v.StartedAt = normalizeTime(v.StartedAt)
	v.FinishedAt = normalizePTime(v.FinishedAt)
	return v
}
func normalizeSnapshot(v hxc.HistoricalHXCSnapshot) hxc.HistoricalHXCSnapshot {
	v.ObservedAt = normalizeTime(v.ObservedAt)
	v.HXCRegisteredAt = normalizePTime(v.HXCRegisteredAt)
	v.HXCLastLoginAt = normalizePTime(v.HXCLastLoginAt)
	v.MembershipEndAt = normalizePTime(v.MembershipEndAt)
	v.LastMessageAt = normalizePTime(v.LastMessageAt)
	v.SubscriptionExpires = normalizePTime(v.SubscriptionExpires)
	v.CRMCreatedAt = normalizePString(v.CRMCreatedAt)
	v.LastQuestionnaireAt = normalizePString(v.LastQuestionnaireAt)
	v.SubscriptionPeriodStart = normalizePString(v.SubscriptionPeriodStart)
	return v
}
func normalizeActivation(v hxc.HistoricalHXCActivation) hxc.HistoricalHXCActivation {
	v.CreatedAt = normalizeTime(v.CreatedAt)
	v.UpdatedAt = normalizeTime(v.UpdatedAt)
	v.LegacyImportBatchRef = normalizePString(v.LegacyImportBatchRef)
	return v
}
func normalizeLead(v hxc.HistoricalHXCLead) hxc.HistoricalHXCLead {
	v.CreatedAt = normalizeTime(v.CreatedAt)
	v.UpdatedAt = normalizeTime(v.UpdatedAt)
	v.LegacyImportBatchRef = normalizePString(v.LegacyImportBatchRef)
	return v
}
func normalizeBatch(v hxc.HistoricalHXCBatch) hxc.HistoricalHXCBatch {
	v.CreatedAt = normalizeTime(v.CreatedAt)
	return v
}
func validReceipt(r hxc.HXCHistoryReceipt, k, s string, p [32]byte) bool {
	return r.Kind == k && r.SourceIdentifier == s && r.PayloadDigest == p && r.TargetID > 0 && r.TargetDigest != ([32]byte{})
}
func hxcHistoryError(e error) error {
	if errors.Is(e, hxc.ErrHXCHistoryInvalid) {
		return hxc.ErrHXCHistoryInvalid
	}
	if errors.Is(e, hxc.ErrHXCHistoryConflict) {
		return hxc.ErrHXCHistoryConflict
	}
	return hxc.ErrHXCHistoryUnavailable
}
func nilHXC(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	return (r.Kind() == reflect.Ptr || r.Kind() == reflect.Interface) && r.IsNil()
}

var _ = strings.TrimSpace
