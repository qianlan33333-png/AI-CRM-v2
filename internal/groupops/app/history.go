package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

// HistoricalWriter shares the caller transaction enforced by store and journal.
// Historical nodes and directory rows never enter current execution or Provider paths.
type HistoricalWriter struct {
	store   groupopsport.HistoricalStore
	journal groupopsport.HistoricalJournal
}

func NewHistoricalWriter(store groupopsport.HistoricalStore, journal groupopsport.HistoricalJournal) (*HistoricalWriter, error) {
	if nilHistoricalDependency(store) || nilHistoricalDependency(journal) {
		return nil, groupopsport.ErrHistoryUnavailable
	}
	return &HistoricalWriter{store: store, journal: journal}, nil
}

func (w *HistoricalWriter) ImportPlan(ctx context.Context, source string, payload [32]byte, r groupopsport.HistoricalPlan) (groupopsport.HistoricalReceipt, error) {
	r = normalizeHistoricalPlan(r)
	if r.ID != 0 || r.SourcePlanID < 1 || r.Status != groupopsport.PlanArchived || r.Revision != 1 || r.CreatedBy < 1 || r.UpdatedBy != r.CreatedBy ||
		!validName(r.Name) || !historicalText(r.Name, r.SourceCode, r.PlanType, r.OriginalStatus) || !historicalOptionalID(r.OwnerStaffID) ||
		!historicalTimes(r.CreatedAt, r.UpdatedAt) || r.UpdatedAt.Before(r.CreatedAt) || !historicalOptionalTimes(r.ArchivedAt) {
		return groupopsport.HistoricalReceipt{}, groupopsport.ErrHistoryInvalid
	}
	return w.importHistory(ctx, "plans", source, payload, func(id int64) [32]byte {
		expected := r
		expected.ID = id
		return HistoricalPlanTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual groupopsport.HistoricalPlan
		var err error
		if id == 0 {
			actual, err = w.store.CreateHistoricalPlan(ctx, r)
		} else {
			actual, err = w.store.GetHistoricalPlan(ctx, id)
		}
		return actual.ID, HistoricalPlanTargetDigest(actual), err
	})
}

func (w *HistoricalWriter) ImportDirectory(ctx context.Context, source string, payload [32]byte, r groupopsport.HistoricalDirectory) (groupopsport.HistoricalReceipt, error) {
	r.RecordedAt = historicalTime(r.RecordedAt)
	kind := "group_chats"
	validShape := r.SourceID != nil && *r.SourceID > 0 && r.MemberCount != nil && *r.MemberCount >= 0 && r.InternalMemberCount == nil && r.ExternalMemberCount == nil && r.OwnerName == nil
	if r.SourceKind == "wecom_group_chat_snapshots" {
		kind = "snapshots"
		validShape = r.SourceID == nil && r.MemberCount == nil && r.InternalMemberCount != nil && *r.InternalMemberCount >= 0 && r.ExternalMemberCount != nil && *r.ExternalMemberCount >= 0
	} else if r.SourceKind != "group_chats" {
		validShape = false
	}
	if r.ID != 0 || !validShape || r.ChatReference == "" || !historicalText(r.ChatReference, r.OriginalStatus) ||
		!historicalOptionalText(r.DisplayName, r.OwnerName) || !historicalOptionalID(r.OwnerStaffID) || !historicalTimes(r.RecordedAt) {
		return groupopsport.HistoricalReceipt{}, groupopsport.ErrHistoryInvalid
	}
	return w.importHistory(ctx, kind, source, payload, func(id int64) [32]byte {
		expected := r
		expected.ID = id
		return HistoricalDirectoryTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual groupopsport.HistoricalDirectory
		var err error
		if id == 0 {
			actual, err = w.store.CreateHistoricalDirectory(ctx, r)
		} else {
			actual, err = w.store.GetHistoricalDirectory(ctx, id)
		}
		return actual.ID, HistoricalDirectoryTargetDigest(actual), err
	})
}

func (w *HistoricalWriter) ImportGroup(ctx context.Context, source string, payload [32]byte, r groupopsport.HistoricalGroup) (groupopsport.HistoricalReceipt, error) {
	r = normalizeHistoricalGroup(r)
	if r.ID != 0 || r.SourceGroupID < 1 || r.SourcePlanID < 1 || r.PlanID < 1 || r.ChatReference == "" ||
		!historicalText(r.ChatReference, r.DisplayName, r.OriginalStatus) || !historicalOptionalID(r.OwnerStaffID) ||
		r.InternalMemberCount < 0 || r.ExternalMemberCount < 0 || !historicalTimes(r.CreatedAt) || !historicalOptionalTimes(r.RemovedAt) {
		return groupopsport.HistoricalReceipt{}, groupopsport.ErrHistoryInvalid
	}
	return w.importHistory(ctx, "groups", source, payload, func(id int64) [32]byte {
		expected := r
		expected.ID = id
		return HistoricalGroupTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		if err := w.checkHistoricalPlan(ctx, r.PlanID, r.SourcePlanID); err != nil {
			return 0, [32]byte{}, err
		}
		var actual groupopsport.HistoricalGroup
		var err error
		if id == 0 {
			actual, err = w.store.CreateHistoricalGroup(ctx, r)
		} else {
			actual, err = w.store.GetHistoricalGroup(ctx, id)
		}
		return actual.ID, HistoricalGroupTargetDigest(actual), err
	})
}

func (w *HistoricalWriter) ImportNode(ctx context.Context, source string, payload [32]byte, r groupopsport.HistoricalNode) (groupopsport.HistoricalReceipt, error) {
	r = normalizeHistoricalNode(r)
	if r.ID != 0 || r.SourceNodeID < 1 || r.SourcePlanID < 1 || r.PlanID < 1 || r.DayIndex < 0 || r.SortOrder < 0 ||
		!historicalText(r.TriggerTime, r.OriginalStatus) || r.ContentPackage == nil || !historicalTimes(r.CreatedAt, r.UpdatedAt) {
		return groupopsport.HistoricalReceipt{}, groupopsport.ErrHistoryInvalid
	}
	return w.importHistory(ctx, "nodes", source, payload, func(id int64) [32]byte {
		expected := r
		expected.ID = id
		return HistoricalNodeTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		if err := w.checkHistoricalPlan(ctx, r.PlanID, r.SourcePlanID); err != nil {
			return 0, [32]byte{}, err
		}
		var actual groupopsport.HistoricalNode
		var err error
		if id == 0 {
			actual, err = w.store.CreateHistoricalNode(ctx, r)
		} else {
			actual, err = w.store.GetHistoricalNode(ctx, id)
		}
		return actual.ID, HistoricalNodeTargetDigest(actual), err
	})
}

func (w *HistoricalWriter) checkHistoricalPlan(ctx context.Context, id, sourceID int64) error {
	parent, err := w.store.GetHistoricalPlan(ctx, id)
	if err != nil {
		return err
	}
	if parent.ID != id || parent.SourcePlanID != sourceID || parent.Status != groupopsport.PlanArchived || parent.Revision != 1 {
		return groupopsport.ErrHistoryConflict
	}
	return nil
}

// access creates at ID zero; replay always loads the complete actual target.
func (w *HistoricalWriter) importHistory(ctx context.Context, kind, source string, payload [32]byte, expected func(int64) [32]byte, access func(int64) (int64, [32]byte, error)) (groupopsport.HistoricalReceipt, error) {
	var empty groupopsport.HistoricalReceipt
	if w == nil || nilHistoricalDependency(w.store) || nilHistoricalDependency(w.journal) || ctx == nil || ctx.Err() != nil {
		return empty, groupopsport.ErrHistoryUnavailable
	}
	if source == "" || source != strings.TrimSpace(source) || !historicalText(source) || payload == [32]byte{} {
		return empty, groupopsport.ErrHistoryInvalid
	}
	receipt, found, err := w.journal.LoadGroupOpsHistory(ctx, kind, source)
	if err != nil {
		return empty, historicalError(err)
	}
	if found {
		if receipt.SourceIdentifier != source || receipt.PayloadDigest != payload || receipt.TargetID < 1 || receipt.TargetDigest == [32]byte{} || receipt.TargetDigest != expected(receipt.TargetID) {
			return empty, groupopsport.ErrHistoryConflict
		}
		id, digest, err := access(receipt.TargetID)
		if err != nil {
			return empty, historicalError(err)
		}
		if id != receipt.TargetID || digest != receipt.TargetDigest {
			return empty, groupopsport.ErrHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	id, digest, err := access(0)
	if err != nil {
		return empty, historicalError(err)
	}
	if id < 1 || digest == [32]byte{} || digest != expected(id) {
		return empty, groupopsport.ErrHistoryConflict
	}
	receipt = groupopsport.HistoricalReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: id, TargetDigest: digest}
	if err := w.journal.RecordGroupOpsHistory(ctx, kind, receipt); err != nil {
		return empty, historicalError(err)
	}
	return receipt, nil
}

func HistoricalPlanTargetDigest(r groupopsport.HistoricalPlan) [32]byte {
	return historicalDigest("plans", normalizeHistoricalPlan(r))
}
func HistoricalDirectoryTargetDigest(r groupopsport.HistoricalDirectory) [32]byte {
	r.RecordedAt = historicalTime(r.RecordedAt)
	return historicalDigest("directory", r)
}
func HistoricalGroupTargetDigest(r groupopsport.HistoricalGroup) [32]byte {
	return historicalDigest("groups", normalizeHistoricalGroup(r))
}
func HistoricalNodeTargetDigest(r groupopsport.HistoricalNode) [32]byte {
	r = normalizeHistoricalNode(r)
	if r.ContentPackage == nil {
		return [32]byte{}
	}
	return historicalDigest("nodes", r)
}
func historicalDigest(kind string, value any) [32]byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(append([]byte("group_ops_history\x00"+kind+"\x00"), raw...))
}
func normalizeHistoricalPlan(r groupopsport.HistoricalPlan) groupopsport.HistoricalPlan {
	r.CreatedAt, r.UpdatedAt = historicalTime(r.CreatedAt), historicalTime(r.UpdatedAt)
	r.ArchivedAt = historicalTimePointer(r.ArchivedAt)
	return r
}
func normalizeHistoricalGroup(r groupopsport.HistoricalGroup) groupopsport.HistoricalGroup {
	r.CreatedAt, r.RemovedAt = historicalTime(r.CreatedAt), historicalTimePointer(r.RemovedAt)
	return r
}
func normalizeHistoricalNode(r groupopsport.HistoricalNode) groupopsport.HistoricalNode {
	r.CreatedAt, r.UpdatedAt = historicalTime(r.CreatedAt), historicalTime(r.UpdatedAt)
	r.ContentPackage = canonicalHistoricalContent(r.ContentPackage)
	return r
}
func canonicalHistoricalContent(raw json.RawMessage) json.RawMessage {
	if !json.Valid(raw) {
		return nil
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}
func historicalTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }
func historicalTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := historicalTime(*value)
	return &normalized
}
func historicalTimes(values ...time.Time) bool {
	for _, value := range values {
		if value.IsZero() || value.Year() < 1 || value.Year() > 9999 {
			return false
		}
	}
	return true
}
func historicalOptionalTimes(values ...*time.Time) bool {
	for _, value := range values {
		if value != nil && !historicalTimes(*value) {
			return false
		}
	}
	return true
}
func historicalOptionalID(value *int64) bool { return value == nil || *value > 0 }
func historicalText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}
func historicalOptionalText(values ...*string) bool {
	for _, value := range values {
		if value != nil && !historicalText(*value) {
			return false
		}
	}
	return true
}
func nilHistoricalDependency(value any) bool {
	return value == nil || reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil()
}
func historicalError(err error) error {
	switch {
	case errors.Is(err, groupopsport.ErrHistoryInvalid):
		return groupopsport.ErrHistoryInvalid
	case errors.Is(err, groupopsport.ErrHistoryConflict):
		return groupopsport.ErrHistoryConflict
	default:
		return groupopsport.ErrHistoryUnavailable
	}
}
