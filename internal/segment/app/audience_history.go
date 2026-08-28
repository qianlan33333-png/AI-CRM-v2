package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// AudienceHistoryWriter writes static facts and receipts in the caller's
// transaction. It has no UoW, runtime Segment, event, or Provider dependency.
type AudienceHistoryWriter struct {
	store   segmentport.AudienceHistoryStore
	journal segmentport.AudienceHistoryJournal
}

func NewAudienceHistoryWriter(store segmentport.AudienceHistoryStore, journal segmentport.AudienceHistoryJournal) *AudienceHistoryWriter {
	return &AudienceHistoryWriter{store: store, journal: journal}
}

func (writer *AudienceHistoryWriter) ready(ctx context.Context) bool {
	return writer != nil && ctx != nil && ctx.Err() == nil && !nilCRUD(writer.store) && !nilCRUD(writer.journal)
}

func (writer *AudienceHistoryWriter) WriteGroup(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalAudienceGroup) (segmentport.AudienceHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryInvalid
	}
	value = normalizeAudienceHistoryGroup(value)
	return writeAudienceHistory(ctx, writer.journal, "groups", sourceIdentifier, payloadDigest, value,
		func(v segmentport.HistoricalAudienceGroup) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceGroup, id int64) segmentport.HistoricalAudienceGroup {
			v.ID = id
			return normalizeAudienceHistoryGroup(v)
		},
		HistoricalAudienceGroupDigest, writer.store.CreateHistoricalAudienceGroup, writer.store.GetHistoricalAudienceGroup)
}

// HistoricalAudienceGroupDigest includes every target field, including ID.
func HistoricalAudienceGroupDigest(value segmentport.HistoricalAudienceGroup) ([32]byte, error) {
	if value.ID < 1 || value.SourceID < 1 {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return audienceHistoryDigest("groups", normalizeAudienceHistoryGroup(value))
}

func normalizeAudienceHistoryGroup(value segmentport.HistoricalAudienceGroup) segmentport.HistoricalAudienceGroup {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	return value
}

func (writer *AudienceHistoryWriter) WritePackage(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalAudiencePackage) (segmentport.AudienceHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryInvalid
	}
	value = normalizeAudienceHistoryPackage(value)
	return writeAudienceHistory(ctx, writer.journal, "packages", sourceIdentifier, payloadDigest, value,
		func(v segmentport.HistoricalAudiencePackage) int64 { return v.ID },
		func(v segmentport.HistoricalAudiencePackage, id int64) segmentport.HistoricalAudiencePackage {
			v.ID = id
			return normalizeAudienceHistoryPackage(v)
		},
		HistoricalAudiencePackageDigest, writer.store.CreateHistoricalAudiencePackage, writer.store.GetHistoricalAudiencePackage)
}

// HistoricalAudiencePackageDigest includes every target field, including ID.
func HistoricalAudiencePackageDigest(value segmentport.HistoricalAudiencePackage) ([32]byte, error) {
	if value.ID < 1 || value.SourceID < 1 || (value.GroupHistoryID != nil && *value.GroupHistoryID < 1) || (value.CurrentVersionSourceID != nil && *value.CurrentVersionSourceID < 1) || value.RuntimeDigest == ([32]byte{}) {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return audienceHistoryDigest("packages", normalizeAudienceHistoryPackage(value))
}

func normalizeAudienceHistoryPackage(value segmentport.HistoricalAudiencePackage) segmentport.HistoricalAudiencePackage {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	value.LastIncrementalAt = audienceHistoryTime(value.LastIncrementalAt)
	value.LastDailyRefreshedAt = audienceHistoryTime(value.LastDailyRefreshedAt)
	value.NextIncrementalAt = audienceHistoryTime(value.NextIncrementalAt)
	value.NextDailyAt = audienceHistoryTime(value.NextDailyAt)
	if value.GroupHistoryID != nil {
		id := *value.GroupHistoryID
		value.GroupHistoryID = &id
	}
	if value.CurrentVersionSourceID != nil {
		id := *value.CurrentVersionSourceID
		value.CurrentVersionSourceID = &id
	}
	return value
}

func (writer *AudienceHistoryWriter) WriteVersion(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalAudienceVersion) (segmentport.AudienceHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryInvalid
	}
	value = normalizeAudienceHistoryVersion(value)
	return writeAudienceHistory(ctx, writer.journal, "versions", sourceIdentifier, payloadDigest, value,
		func(v segmentport.HistoricalAudienceVersion) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceVersion, id int64) segmentport.HistoricalAudienceVersion {
			v.ID = id
			return normalizeAudienceHistoryVersion(v)
		},
		HistoricalAudienceVersionDigest, writer.store.CreateHistoricalAudienceVersion, writer.store.GetHistoricalAudienceVersion)
}

// HistoricalAudienceVersionDigest includes every target field, including ID.
func HistoricalAudienceVersionDigest(value segmentport.HistoricalAudienceVersion) ([32]byte, error) {
	if value.ID < 1 || value.SourceID < 1 || value.PackageHistoryID < 1 || value.DefinitionDigest == ([32]byte{}) {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return audienceHistoryDigest("versions", normalizeAudienceHistoryVersion(value))
}

func normalizeAudienceHistoryVersion(value segmentport.HistoricalAudienceVersion) segmentport.HistoricalAudienceVersion {
	if value.TemplateVersion != nil {
		version := *value.TemplateVersion
		value.TemplateVersion = &version
	}
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.PublishedAt = audienceHistoryTime(value.PublishedAt)
	return value
}

func (writer *AudienceHistoryWriter) WriteSender(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalAudienceSender) (segmentport.AudienceHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryInvalid
	}
	value = normalizeAudienceHistorySender(value)
	return writeAudienceHistory(ctx, writer.journal, "senders", sourceIdentifier, payloadDigest, value,
		func(v segmentport.HistoricalAudienceSender) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceSender, id int64) segmentport.HistoricalAudienceSender {
			v.ID = id
			return normalizeAudienceHistorySender(v)
		},
		HistoricalAudienceSenderDigest, writer.store.CreateHistoricalAudienceSender, writer.store.GetHistoricalAudienceSender)
}

// HistoricalAudienceSenderDigest includes every target field, including ID.
func HistoricalAudienceSenderDigest(value segmentport.HistoricalAudienceSender) ([32]byte, error) {
	if value.ID < 1 || value.SourceID < 1 || value.PackageHistoryID < 1 || (value.StaffID != nil && *value.StaffID < 1) {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return audienceHistoryDigest("senders", normalizeAudienceHistorySender(value))
}

func normalizeAudienceHistorySender(value segmentport.HistoricalAudienceSender) segmentport.HistoricalAudienceSender {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	if value.StaffID != nil {
		id := *value.StaffID
		value.StaffID = &id
	}
	return value
}

func (writer *AudienceHistoryWriter) WriteRule(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalAudienceRule) (segmentport.AudienceHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryInvalid
	}
	value = normalizeAudienceHistoryRule(value)
	return writeAudienceHistory(ctx, writer.journal, "rules", sourceIdentifier, payloadDigest, value,
		func(v segmentport.HistoricalAudienceRule) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceRule, id int64) segmentport.HistoricalAudienceRule {
			v.ID = id
			return normalizeAudienceHistoryRule(v)
		},
		HistoricalAudienceRuleDigest, writer.store.CreateHistoricalAudienceRule, writer.store.GetHistoricalAudienceRule)
}

// HistoricalAudienceRuleDigest includes every target field, including ID.
func HistoricalAudienceRuleDigest(value segmentport.HistoricalAudienceRule) ([32]byte, error) {
	if value.ID < 1 || value.SourceID < 1 || (value.OwnerStaffID != nil && *value.OwnerStaffID < 1) {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return audienceHistoryDigest("rules", normalizeAudienceHistoryRule(value))
}

func normalizeAudienceHistoryRule(value segmentport.HistoricalAudienceRule) segmentport.HistoricalAudienceRule {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	if value.OwnerStaffID != nil {
		id := *value.OwnerStaffID
		value.OwnerStaffID = &id
	}
	return value
}

func (writer *AudienceHistoryWriter) WriteRuleVersion(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalAudienceRuleVersion) (segmentport.AudienceHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryInvalid
	}
	value = normalizeAudienceHistoryRuleVersion(value)
	return writeAudienceHistory(ctx, writer.journal, "rule_versions", sourceIdentifier, payloadDigest, value,
		func(v segmentport.HistoricalAudienceRuleVersion) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceRuleVersion, id int64) segmentport.HistoricalAudienceRuleVersion {
			v.ID = id
			return normalizeAudienceHistoryRuleVersion(v)
		},
		HistoricalAudienceRuleVersionDigest, writer.store.CreateHistoricalAudienceRuleVersion, writer.store.GetHistoricalAudienceRuleVersion)
}

// HistoricalAudienceRuleVersionDigest includes every target field, including ID.
func HistoricalAudienceRuleVersionDigest(value segmentport.HistoricalAudienceRuleVersion) ([32]byte, error) {
	if value.ID < 1 || value.SourceID < 1 || value.RuleHistoryID < 1 || value.DefinitionDigest == ([32]byte{}) {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return audienceHistoryDigest("rule_versions", normalizeAudienceHistoryRuleVersion(value))
}

func normalizeAudienceHistoryRuleVersion(value segmentport.HistoricalAudienceRuleVersion) segmentport.HistoricalAudienceRuleVersion {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.PublishedAt = audienceHistoryTime(value.PublishedAt)
	return value
}

func (writer *AudienceHistoryWriter) WriteDefinition(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalAudienceDefinition) (segmentport.AudienceHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryInvalid
	}
	value = normalizeAudienceHistoryDefinition(value)
	return writeAudienceHistory(ctx, writer.journal, "definitions", sourceIdentifier, payloadDigest, value,
		func(v segmentport.HistoricalAudienceDefinition) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceDefinition, id int64) segmentport.HistoricalAudienceDefinition {
			v.ID = id
			return normalizeAudienceHistoryDefinition(v)
		},
		HistoricalAudienceDefinitionDigest, writer.store.CreateHistoricalAudienceDefinition, writer.store.GetHistoricalAudienceDefinition)
}

// HistoricalAudienceDefinitionDigest includes every target field, including ID.
func HistoricalAudienceDefinitionDigest(value segmentport.HistoricalAudienceDefinition) ([32]byte, error) {
	if value.ID < 1 || value.SourceID < 1 || value.DefinitionDigest == ([32]byte{}) {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return audienceHistoryDigest("definitions", normalizeAudienceHistoryDefinition(value))
}

func normalizeAudienceHistoryDefinition(value segmentport.HistoricalAudienceDefinition) segmentport.HistoricalAudienceDefinition {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	value.LastRefreshedAt = audienceHistoryTime(value.LastRefreshedAt)
	return value
}

func (writer *AudienceHistoryWriter) WriteMember(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalAudienceMember) (segmentport.AudienceHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceHistoryReceipt{}, segmentport.ErrAudienceHistoryInvalid
	}
	value = normalizeAudienceHistoryMember(value)
	return writeAudienceHistory(ctx, writer.journal, "members", sourceIdentifier, payloadDigest, value,
		func(v segmentport.HistoricalAudienceMember) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceMember, id int64) segmentport.HistoricalAudienceMember {
			v.ID = id
			return normalizeAudienceHistoryMember(v)
		},
		HistoricalAudienceMemberDigest, writer.store.CreateHistoricalAudienceMember, writer.store.GetHistoricalAudienceMember)
}

// HistoricalAudienceMemberDigest includes every target field, including ID.
func HistoricalAudienceMemberDigest(value segmentport.HistoricalAudienceMember) ([32]byte, error) {
	if value.ID < 1 || value.SourceID < 1 || value.PackageHistoryID < 1 || (value.CustomerID != nil && *value.CustomerID < 1) || value.PayloadDigest == ([32]byte{}) {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return audienceHistoryDigest("members", normalizeAudienceHistoryMember(value))
}

func normalizeAudienceHistoryMember(value segmentport.HistoricalAudienceMember) segmentport.HistoricalAudienceMember {
	value.FirstEnteredAt = value.FirstEnteredAt.UTC().Truncate(time.Microsecond)
	value.LastSeenAt = value.LastSeenAt.UTC().Truncate(time.Microsecond)
	value.LastUpdatedAt = value.LastUpdatedAt.UTC().Truncate(time.Microsecond)
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	value.ExitedAt = audienceHistoryTime(value.ExitedAt)
	if value.CustomerID != nil {
		id := *value.CustomerID
		value.CustomerID = &id
	}
	return value
}

// This helper only shares the identical receipt/replay flow of these eight
// frozen types. Parents remain enforced by the store's same-transaction FKs.
func writeAudienceHistory[T any](
	ctx context.Context, journal segmentport.AudienceHistoryJournal, kind, source string, payload [32]byte, value T,
	id func(T) int64, withID func(T, int64) T, digest func(T) ([32]byte, error),
	create func(context.Context, T) (T, error), get func(context.Context, int64) (T, error),
) (segmentport.AudienceHistoryReceipt, error) {
	empty := segmentport.AudienceHistoryReceipt{}
	if source == "" || payload == ([32]byte{}) {
		return empty, segmentport.ErrAudienceHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, segmentport.ErrAudienceHistoryInvalid
	}
	receipt, found, err := journal.LoadAudienceHistory(ctx, kind, source)
	if err != nil {
		return empty, audienceHistoryError(err)
	}
	if found {
		if receipt.SourceIdentifier != source || receipt.PayloadDigest != payload || receipt.TargetID < 1 || receipt.TargetDigest == ([32]byte{}) {
			return empty, segmentport.ErrAudienceHistoryConflict
		}
		actual, err := get(ctx, receipt.TargetID)
		if err != nil {
			return empty, audienceHistoryError(err)
		}
		actualDigest, err := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if err != nil || expectedErr != nil || id(actual) != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, segmentport.ErrAudienceHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create(ctx, withID(value, 0))
	if err != nil {
		return empty, audienceHistoryError(err)
	}
	actualDigest, err := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, id(actual)))
	if err != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, segmentport.ErrAudienceHistoryConflict
	}
	receipt = segmentport.AudienceHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: id(actual), TargetDigest: actualDigest}
	if err = journal.RecordAudienceHistory(ctx, kind, receipt); err != nil {
		return empty, audienceHistoryError(err)
	}
	return receipt, nil
}

func audienceHistoryDigest(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string
		Value any
	}{kind, value})
	if err != nil {
		return [32]byte{}, segmentport.ErrAudienceHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func audienceHistoryTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC().Truncate(time.Microsecond)
	return &normalized
}

func audienceHistoryError(err error) error {
	switch {
	case errors.Is(err, segmentport.ErrAudienceHistoryInvalid):
		return segmentport.ErrAudienceHistoryInvalid
	case errors.Is(err, segmentport.ErrAudienceHistoryConflict):
		return segmentport.ErrAudienceHistoryConflict
	default:
		return segmentport.ErrAudienceHistoryUnavailable
	}
}
