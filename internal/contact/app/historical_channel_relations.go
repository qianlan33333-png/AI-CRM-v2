package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

const (
	historicalChannelContactsKind  = "contacts"
	historicalChannelAssigneesKind = "assignees"
	historicalChannelCivilLayout   = "2006-01-02T15:04:05.000000"
)

// HistoricalChannelRelationsWriter persists only V1 channel history through
// the caller-bound transaction. It has no current ownership, event, or
// Provider dependency.
type HistoricalChannelRelationsWriter struct {
	store   contactport.HistoricalChannelRelationsStore
	journal contactport.HistoricalChannelRelationsJournal
}

func NewHistoricalChannelRelationsWriter(store contactport.HistoricalChannelRelationsStore, journal contactport.HistoricalChannelRelationsJournal) (*HistoricalChannelRelationsWriter, error) {
	if historicalChannelNil(store) || historicalChannelNil(journal) {
		return nil, contactport.ErrHistoricalChannelUnavailable
	}
	return &HistoricalChannelRelationsWriter{store: store, journal: journal}, nil
}

func (writer *HistoricalChannelRelationsWriter) ImportContact(ctx context.Context, definition contactport.HistoricalChannelContactDefinition) (contactport.HistoricalChannelReceipt, error) {
	if writer == nil || historicalChannelNil(writer.store) || historicalChannelNil(writer.journal) || ctx == nil || ctx.Err() != nil {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelUnavailable
	}
	expected, err := historicalChannelContactRecord(definition.Contact)
	if err != nil || !validHistoricalChannelRelation(definition.SourceIdentifier, definition.PayloadDigest) {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelInvalid
	}
	existing, found, err := writer.journal.LoadHistoricalChannelRelation(ctx, historicalChannelContactsKind, definition.SourceIdentifier)
	if err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	if found {
		if !sameHistoricalChannelRelationFact(existing, definition.SourceIdentifier, definition.PayloadDigest) {
			return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
		}
		stored, getErr := writer.store.GetHistoricalChannelContact(ctx, existing.TargetID)
		if getErr != nil {
			return contactport.HistoricalChannelReceipt{}, getErr
		}
		digest, digestErr := HistoricalChannelContactTargetDigest(stored)
		if !sameHistoricalChannelContact(stored, expected) || digestErr != nil || digest != existing.TargetDigest {
			return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	stored, err := writer.store.CreateHistoricalChannelContact(ctx, expected)
	if err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	digest, digestErr := HistoricalChannelContactTargetDigest(stored)
	if !sameHistoricalChannelContact(stored, expected) || digestErr != nil {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
	}
	receipt := historicalChannelRelationReceipt(definition.SourceIdentifier, definition.PayloadDigest, stored.ID, digest)
	if err = writer.journal.RecordHistoricalChannelRelation(ctx, historicalChannelContactsKind, receipt); err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	return receipt, nil
}

func (writer *HistoricalChannelRelationsWriter) ImportAssignee(ctx context.Context, definition contactport.HistoricalChannelAssigneeDefinition) (contactport.HistoricalChannelReceipt, error) {
	if writer == nil || historicalChannelNil(writer.store) || historicalChannelNil(writer.journal) || ctx == nil || ctx.Err() != nil {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelUnavailable
	}
	expected, err := historicalChannelAssigneeRecord(definition.Assignee)
	if err != nil || !validHistoricalChannelRelation(definition.SourceIdentifier, definition.PayloadDigest) {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelInvalid
	}
	existing, found, err := writer.journal.LoadHistoricalChannelRelation(ctx, historicalChannelAssigneesKind, definition.SourceIdentifier)
	if err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	if found {
		if !sameHistoricalChannelRelationFact(existing, definition.SourceIdentifier, definition.PayloadDigest) {
			return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
		}
		stored, getErr := writer.store.GetHistoricalChannelAssignee(ctx, existing.TargetID)
		if getErr != nil {
			return contactport.HistoricalChannelReceipt{}, getErr
		}
		digest, digestErr := HistoricalChannelAssigneeTargetDigest(stored)
		if !sameHistoricalChannelAssignee(stored, expected) || digestErr != nil || digest != existing.TargetDigest {
			return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	stored, err := writer.store.CreateHistoricalChannelAssignee(ctx, expected)
	if err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	digest, digestErr := HistoricalChannelAssigneeTargetDigest(stored)
	if !sameHistoricalChannelAssignee(stored, expected) || digestErr != nil {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
	}
	receipt := historicalChannelRelationReceipt(definition.SourceIdentifier, definition.PayloadDigest, stored.ID, digest)
	if err = writer.journal.RecordHistoricalChannelRelation(ctx, historicalChannelAssigneesKind, receipt); err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	return receipt, nil
}

func HistoricalChannelContactTargetDigest(record contactport.HistoricalChannelContact) ([sha256.Size]byte, error) {
	if !validHistoricalChannelStoredContact(record) {
		return [sha256.Size]byte{}, contactport.ErrHistoricalChannelInvalid
	}
	return historicalChannelRelationDigest("v1.channel_contact", record), nil
}

func HistoricalChannelAssigneeTargetDigest(record contactport.HistoricalChannelAssignee) ([sha256.Size]byte, error) {
	if !validHistoricalChannelStoredAssignee(record) {
		return [sha256.Size]byte{}, contactport.ErrHistoricalChannelInvalid
	}
	return historicalChannelRelationDigest("v1.channel_assignee", record), nil
}

func historicalChannelRelationDigest(kind string, record any) [sha256.Size]byte {
	encoded, _ := json.Marshal(struct {
		Kind   string `json:"kind"`
		Record any    `json:"record"`
	}{Kind: kind, Record: record})
	return sha256.Sum256(encoded)
}

func historicalChannelContactRecord(record contactport.HistoricalChannelContact) (contactport.HistoricalChannelContact, error) {
	if !validHistoricalChannelContact(record, false) {
		return contactport.HistoricalChannelContact{}, contactport.ErrHistoricalChannelInvalid
	}
	record.FirstEnteredAt = normalizeHistoricalChannelTime(record.FirstEnteredAt)
	record.LastEnteredAt = normalizeHistoricalChannelTime(record.LastEnteredAt)
	record.CreatedAt = normalizeHistoricalChannelTime(record.CreatedAt)
	record.UpdatedAt = normalizeHistoricalChannelTime(record.UpdatedAt)
	return record, nil
}

func historicalChannelAssigneeRecord(record contactport.HistoricalChannelAssignee) (contactport.HistoricalChannelAssignee, error) {
	if !validHistoricalChannelAssignee(record, false) {
		return contactport.HistoricalChannelAssignee{}, contactport.ErrHistoricalChannelInvalid
	}
	return record, nil
}

func validHistoricalChannelRelation(source string, digest [sha256.Size]byte) bool {
	return validHistoricalChannelSource(source) && digest != ([sha256.Size]byte{})
}

func validHistoricalChannelContact(record contactport.HistoricalChannelContact, stored bool) bool {
	if (stored && record.ID < 1) || (!stored && record.ID != 0) || record.ChannelID < 1 || record.SourceContactID < 1 || record.EnterCount < 1 ||
		record.FirstEnteredAt.IsZero() || record.LastEnteredAt.IsZero() || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.LastEnteredAt.Before(record.FirstEnteredAt) || record.UpdatedAt.Before(record.CreatedAt) {
		return false
	}
	if record.CustomerID != nil && *record.CustomerID < 1 {
		return false
	}
	return !stored || historicalChannelMicroUTC(record.FirstEnteredAt) && historicalChannelMicroUTC(record.LastEnteredAt) && historicalChannelMicroUTC(record.CreatedAt) && historicalChannelMicroUTC(record.UpdatedAt)
}

func validHistoricalChannelAssignee(record contactport.HistoricalChannelAssignee, stored bool) bool {
	if (stored && record.ID < 1) || (!stored && record.ID != 0) || record.ChannelID < 1 || record.SourceAssigneeID < 1 || record.Priority < 0 || record.Status == "" {
		return false
	}
	if record.RatioPercent != nil && (*record.RatioPercent < 0 || *record.RatioPercent > 100) || record.MaxScans24h != nil && *record.MaxScans24h < 0 {
		return false
	}
	created, ok := historicalChannelCivilTime(record.SourceCreatedAt)
	if !ok {
		return false
	}
	updated, ok := historicalChannelCivilTime(record.SourceUpdatedAt)
	return ok && !updated.Before(created)
}

func validHistoricalChannelStoredContact(record contactport.HistoricalChannelContact) bool {
	return validHistoricalChannelContact(record, true)
}

func validHistoricalChannelStoredAssignee(record contactport.HistoricalChannelAssignee) bool {
	return validHistoricalChannelAssignee(record, true)
}

func sameHistoricalChannelRelationFact(receipt contactport.HistoricalChannelReceipt, source string, digest [sha256.Size]byte) bool {
	return receipt.SourceIdentifier == source && receipt.PayloadDigest == digest && receipt.TargetID > 0 && receipt.TargetDigest != ([sha256.Size]byte{})
}

func historicalChannelRelationReceipt(source string, payload [sha256.Size]byte, targetID int64, target [sha256.Size]byte) contactport.HistoricalChannelReceipt {
	return contactport.HistoricalChannelReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: targetID, TargetDigest: target}
}

func sameHistoricalChannelContact(actual, expected contactport.HistoricalChannelContact) bool {
	return validHistoricalChannelStoredContact(actual) && actual.ChannelID == expected.ChannelID && actual.SourceContactID == expected.SourceContactID &&
		sameHistoricalChannelCustomer(actual.CustomerID, expected.CustomerID) && actual.OwnerReference == expected.OwnerReference &&
		actual.FirstEnteredAt.Equal(expected.FirstEnteredAt) && actual.LastEnteredAt.Equal(expected.LastEnteredAt) && actual.EnterCount == expected.EnterCount &&
		actual.CreatedAt.Equal(expected.CreatedAt) && actual.UpdatedAt.Equal(expected.UpdatedAt)
}

func sameHistoricalChannelAssignee(actual, expected contactport.HistoricalChannelAssignee) bool {
	return validHistoricalChannelStoredAssignee(actual) && actual.ChannelID == expected.ChannelID && actual.SourceAssigneeID == expected.SourceAssigneeID &&
		actual.StaffReference == expected.StaffReference && actual.DisplayNameSnapshot == expected.DisplayNameSnapshot && actual.Priority == expected.Priority &&
		sameHistoricalChannelInt32(actual.RatioPercent, expected.RatioPercent) && sameHistoricalChannelInt32(actual.MaxScans24h, expected.MaxScans24h) &&
		actual.Status == expected.Status && actual.SourceCreatedAt == expected.SourceCreatedAt && actual.SourceUpdatedAt == expected.SourceUpdatedAt
}

func sameHistoricalChannelCustomer(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func sameHistoricalChannelInt32(left, right *int32) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func historicalChannelMicroUTC(value time.Time) bool {
	return value.Location() == time.UTC && value.Equal(normalizeHistoricalChannelTime(value))
}

func historicalChannelCivilTime(value string) (time.Time, bool) {
	parsed, err := time.Parse(historicalChannelCivilLayout, value)
	return parsed, err == nil && parsed.Format(historicalChannelCivilLayout) == value
}
