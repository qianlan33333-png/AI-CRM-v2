package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// AudienceHistoryStore only resolves the caller-bound transaction. Historical
// imports must compose this store with their journal in that same transaction.
type AudienceHistoryStore struct {
	tx func(context.Context) (pgx.Tx, error)
}

// AudienceHistoryReader is intentionally separate from the write store. It
// accepts a pool for API reads or a caller transaction for reconciliation.
type AudienceHistoryReader struct {
	db segmentdb.DBTX
}

var _ segmentport.AudienceHistoryStore = (*AudienceHistoryStore)(nil)
var _ segmentport.AudienceHistoryReader = (*AudienceHistoryReader)(nil)

func NewAudienceHistoryStore() *AudienceHistoryStore {
	return &AudienceHistoryStore{tx: platformstore.TxFromContext}
}

func NewAudienceHistoryReader(db segmentdb.DBTX) *AudienceHistoryReader {
	return &AudienceHistoryReader{db: db}
}

func (store *AudienceHistoryStore) queries(ctx context.Context) (*segmentdb.Queries, error) {
	if store == nil || store.tx == nil || ctx == nil || ctx.Err() != nil {
		return nil, segmentport.ErrAudienceHistoryUnavailable
	}
	tx, err := store.tx(ctx)
	if err != nil || tx == nil {
		return nil, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentdb.New(tx), nil
}

func (reader *AudienceHistoryReader) queries(ctx context.Context) (*segmentdb.Queries, error) {
	if reader == nil || audienceHistoryNilDependency(reader.db) || ctx == nil || ctx.Err() != nil {
		return nil, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentdb.New(reader.db), nil
}

func audienceHistoryNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (store *AudienceHistoryStore) CreateHistoricalAudienceGroup(ctx context.Context, value segmentport.HistoricalAudienceGroup) (segmentport.HistoricalAudienceGroup, error) {
	if !validAudienceHistoryGroupCreate(value) {
		return segmentport.HistoricalAudienceGroup{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceGroup{}, err
	}
	row, err := queries.CreateHistoricalAudienceGroup(ctx, segmentdb.CreateHistoricalAudienceGroupParams{SourceID: value.SourceID, Name: value.Name, CreatedAt: audienceHistoryTimestamp(value.CreatedAt), UpdatedAt: audienceHistoryTimestamp(value.UpdatedAt)})
	if err != nil {
		return segmentport.HistoricalAudienceGroup{}, audienceHistoryError(err)
	}
	return audienceHistoryGroup(row)
}

func (store *AudienceHistoryStore) GetHistoricalAudienceGroup(ctx context.Context, id int64) (segmentport.HistoricalAudienceGroup, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceGroup{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceGroup{}, err
	}
	row, err := queries.GetHistoricalAudienceGroup(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceGroup{}, audienceHistoryError(err)
	}
	return audienceHistoryGroup(row)
}

func (store *AudienceHistoryStore) CreateHistoricalAudiencePackage(ctx context.Context, value segmentport.HistoricalAudiencePackage) (segmentport.HistoricalAudiencePackage, error) {
	if !validAudienceHistoryPackageCreate(value) {
		return segmentport.HistoricalAudiencePackage{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	row, err := queries.CreateHistoricalAudiencePackage(ctx, segmentdb.CreateHistoricalAudiencePackageParams{
		SourceID: value.SourceID, GroupHistoryID: audienceHistoryNullableInt64(value.GroupHistoryID), CurrentVersionSourceID: audienceHistoryNullableInt64(value.CurrentVersionSourceID),
		PackageKey: value.PackageKey, Name: value.Name, NaturalLanguageDefinition: value.NaturalLanguageDefinition, OriginalStatus: value.OriginalStatus,
		QueryMode: value.QueryMode, IdentityPolicy: value.IdentityPolicy, IncrementalEnabled: value.IncrementalEnabled, DailyEnabled: value.DailyEnabled,
		IncrementalIntervalSeconds: value.IncrementalIntervalSecs, DailyRefreshTime: value.DailyRefreshTime, Timezone: value.Timezone, LookbackSeconds: value.LookbackSecs,
		LastIncrementalAt: audienceHistoryNullableTimestamp(value.LastIncrementalAt), LastDailyRefreshedAt: audienceHistoryNullableTimestamp(value.LastDailyRefreshedAt),
		NextIncrementalAt: audienceHistoryNullableTimestamp(value.NextIncrementalAt), NextDailyAt: audienceHistoryNullableTimestamp(value.NextDailyAt), PausedReason: value.PausedReason,
		CreatedAt: audienceHistoryTimestamp(value.CreatedAt), UpdatedAt: audienceHistoryTimestamp(value.UpdatedAt), RuntimeDigest: value.RuntimeDigest[:],
	})
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, audienceHistoryError(err)
	}
	return audienceHistoryPackage(row)
}

func (store *AudienceHistoryStore) GetHistoricalAudiencePackage(ctx context.Context, id int64) (segmentport.HistoricalAudiencePackage, error) {
	if id < 1 {
		return segmentport.HistoricalAudiencePackage{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	row, err := queries.GetHistoricalAudiencePackage(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, audienceHistoryError(err)
	}
	return audienceHistoryPackage(row)
}

func (store *AudienceHistoryStore) CreateHistoricalAudienceVersion(ctx context.Context, value segmentport.HistoricalAudienceVersion) (segmentport.HistoricalAudienceVersion, error) {
	if !validAudienceHistoryVersionCreate(value) {
		return segmentport.HistoricalAudienceVersion{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, err
	}
	row, err := queries.CreateHistoricalAudienceVersion(ctx, segmentdb.CreateHistoricalAudienceVersionParams{
		SourceID: value.SourceID, PackageHistoryID: value.PackageHistoryID, VersionNumber: value.VersionNumber, OriginalStatus: value.OriginalStatus,
		AiPrompt: value.AIPrompt, AiRationale: value.AIRationale, NaturalLanguageExplanation: value.NaturalLanguageExplanation,
		CreatedAt: audienceHistoryTimestamp(value.CreatedAt), PublishedAt: audienceHistoryNullableTimestamp(value.PublishedAt), TemplateKey: value.TemplateKey,
		TemplateVersion: audienceHistoryNullableInt64(value.TemplateVersion), TemplateFingerprint: value.TemplateFingerprint, DefinitionDigest: value.DefinitionDigest[:],
	})
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, audienceHistoryError(err)
	}
	return audienceHistoryVersion(row)
}

func (store *AudienceHistoryStore) GetHistoricalAudienceVersion(ctx context.Context, id int64) (segmentport.HistoricalAudienceVersion, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceVersion{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, err
	}
	row, err := queries.GetHistoricalAudienceVersion(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, audienceHistoryError(err)
	}
	return audienceHistoryVersion(row)
}

func (store *AudienceHistoryStore) CreateHistoricalAudienceSender(ctx context.Context, value segmentport.HistoricalAudienceSender) (segmentport.HistoricalAudienceSender, error) {
	if !validAudienceHistorySenderCreate(value) {
		return segmentport.HistoricalAudienceSender{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, err
	}
	row, err := queries.CreateHistoricalAudienceSender(ctx, segmentdb.CreateHistoricalAudienceSenderParams{SourceID: value.SourceID, PackageHistoryID: value.PackageHistoryID, StaffID: audienceHistoryNullableInt64(value.StaffID), DisplayName: value.DisplayName, Priority: value.Priority, OriginalStatus: value.OriginalStatus, CreatedAt: audienceHistoryTimestamp(value.CreatedAt), UpdatedAt: audienceHistoryTimestamp(value.UpdatedAt)})
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, audienceHistoryError(err)
	}
	return audienceHistorySender(row)
}

func (store *AudienceHistoryStore) GetHistoricalAudienceSender(ctx context.Context, id int64) (segmentport.HistoricalAudienceSender, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceSender{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, err
	}
	row, err := queries.GetHistoricalAudienceSender(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, audienceHistoryError(err)
	}
	return audienceHistorySender(row)
}

func (store *AudienceHistoryStore) CreateHistoricalAudienceRule(ctx context.Context, value segmentport.HistoricalAudienceRule) (segmentport.HistoricalAudienceRule, error) {
	if !validAudienceHistoryRuleCreate(value) {
		return segmentport.HistoricalAudienceRule{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, err
	}
	row, err := queries.CreateHistoricalAudienceRule(ctx, segmentdb.CreateHistoricalAudienceRuleParams{SourceID: value.SourceID, RuleKey: value.RuleKey, DisplayName: value.DisplayName, Description: value.Description, RuleType: value.RuleType, OwnerStaffID: audienceHistoryNullableInt64(value.OwnerStaffID), OriginalStatus: value.OriginalStatus, CreatedAt: audienceHistoryTimestamp(value.CreatedAt), UpdatedAt: audienceHistoryTimestamp(value.UpdatedAt)})
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, audienceHistoryError(err)
	}
	return audienceHistoryRule(row)
}

func (store *AudienceHistoryStore) GetHistoricalAudienceRule(ctx context.Context, id int64) (segmentport.HistoricalAudienceRule, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceRule{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, err
	}
	row, err := queries.GetHistoricalAudienceRule(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, audienceHistoryError(err)
	}
	return audienceHistoryRule(row)
}

func (store *AudienceHistoryStore) CreateHistoricalAudienceRuleVersion(ctx context.Context, value segmentport.HistoricalAudienceRuleVersion) (segmentport.HistoricalAudienceRuleVersion, error) {
	if !validAudienceHistoryRuleVersionCreate(value) {
		return segmentport.HistoricalAudienceRuleVersion{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, err
	}
	row, err := queries.CreateHistoricalAudienceRuleVersion(ctx, segmentdb.CreateHistoricalAudienceRuleVersionParams{SourceID: value.SourceID, RuleHistoryID: value.RuleHistoryID, Version: value.Version, ExecutorType: value.ExecutorType, OriginalStatus: value.OriginalStatus, PublishedAt: audienceHistoryNullableTimestamp(value.PublishedAt), CreatedAt: audienceHistoryTimestamp(value.CreatedAt), DefinitionDigest: value.DefinitionDigest[:]})
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, audienceHistoryError(err)
	}
	return audienceHistoryRuleVersion(row)
}

func (store *AudienceHistoryStore) GetHistoricalAudienceRuleVersion(ctx context.Context, id int64) (segmentport.HistoricalAudienceRuleVersion, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceRuleVersion{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, err
	}
	row, err := queries.GetHistoricalAudienceRuleVersion(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, audienceHistoryError(err)
	}
	return audienceHistoryRuleVersion(row)
}

func (store *AudienceHistoryStore) CreateHistoricalAudienceDefinition(ctx context.Context, value segmentport.HistoricalAudienceDefinition) (segmentport.HistoricalAudienceDefinition, error) {
	if !validAudienceHistoryDefinitionCreate(value) {
		return segmentport.HistoricalAudienceDefinition{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, err
	}
	row, err := queries.CreateHistoricalAudienceDefinition(ctx, segmentdb.CreateHistoricalAudienceDefinitionParams{SourceID: value.SourceID, Code: value.Code, DisplayName: value.DisplayName, Description: value.Description, SourceType: value.SourceType, SqlDialect: value.SQLDialect, OriginalStatus: value.OriginalStatus, Version: value.Version, CachedHeadcount: value.CachedHeadcount, LastRefreshedAt: audienceHistoryNullableTimestamp(value.LastRefreshedAt), UsageCount: value.UsageCount, CreatedAt: audienceHistoryTimestamp(value.CreatedAt), UpdatedAt: audienceHistoryTimestamp(value.UpdatedAt), DefinitionDigest: value.DefinitionDigest[:]})
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, audienceHistoryError(err)
	}
	return audienceHistoryDefinition(row)
}

func (store *AudienceHistoryStore) GetHistoricalAudienceDefinition(ctx context.Context, id int64) (segmentport.HistoricalAudienceDefinition, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceDefinition{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, err
	}
	row, err := queries.GetHistoricalAudienceDefinition(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, audienceHistoryError(err)
	}
	return audienceHistoryDefinition(row)
}

func (store *AudienceHistoryStore) CreateHistoricalAudienceMember(ctx context.Context, value segmentport.HistoricalAudienceMember) (segmentport.HistoricalAudienceMember, error) {
	if !validAudienceHistoryMemberCreate(value) {
		return segmentport.HistoricalAudienceMember{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	row, err := queries.CreateHistoricalAudienceMember(ctx, segmentdb.CreateHistoricalAudienceMemberParams{SourceID: value.SourceID, PackageHistoryID: value.PackageHistoryID, CustomerID: audienceHistoryNullableInt64(value.CustomerID), IdentityKind: value.IdentityKind, OriginalStatus: value.OriginalStatus, FirstEnteredAt: audienceHistoryTimestamp(value.FirstEnteredAt), LastSeenAt: audienceHistoryTimestamp(value.LastSeenAt), LastUpdatedAt: audienceHistoryTimestamp(value.LastUpdatedAt), ExitedAt: audienceHistoryNullableTimestamp(value.ExitedAt), CreatedAt: audienceHistoryTimestamp(value.CreatedAt), UpdatedAt: audienceHistoryTimestamp(value.UpdatedAt), PayloadDigest: value.PayloadDigest[:]})
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, audienceHistoryError(err)
	}
	return audienceHistoryMember(row)
}

func (store *AudienceHistoryStore) GetHistoricalAudienceMember(ctx context.Context, id int64) (segmentport.HistoricalAudienceMember, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceMember{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	row, err := queries.GetHistoricalAudienceMember(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, audienceHistoryError(err)
	}
	return audienceHistoryMember(row)
}

func (reader *AudienceHistoryReader) GetHistoricalAudiencePackage(ctx context.Context, id int64) (segmentport.HistoricalAudiencePackage, error) {
	if id < 1 {
		return segmentport.HistoricalAudiencePackage{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	row, err := queries.GetHistoricalAudiencePackage(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, audienceHistoryError(err)
	}
	return audienceHistoryPackage(row)
}

func (reader *AudienceHistoryReader) GetHistoricalAudienceGroup(ctx context.Context, id int64) (segmentport.HistoricalAudienceGroup, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceGroup{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceGroup{}, err
	}
	row, err := queries.GetHistoricalAudienceGroup(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceGroup{}, audienceHistoryError(err)
	}
	return audienceHistoryGroup(row)
}

func (reader *AudienceHistoryReader) GetHistoricalAudienceVersion(ctx context.Context, id int64) (segmentport.HistoricalAudienceVersion, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceVersion{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, err
	}
	row, err := queries.GetHistoricalAudienceVersion(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, audienceHistoryError(err)
	}
	return audienceHistoryVersion(row)
}

func (reader *AudienceHistoryReader) GetHistoricalAudienceSender(ctx context.Context, id int64) (segmentport.HistoricalAudienceSender, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceSender{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, err
	}
	row, err := queries.GetHistoricalAudienceSender(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, audienceHistoryError(err)
	}
	return audienceHistorySender(row)
}

func (reader *AudienceHistoryReader) GetHistoricalAudienceRule(ctx context.Context, id int64) (segmentport.HistoricalAudienceRule, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceRule{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, err
	}
	row, err := queries.GetHistoricalAudienceRule(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, audienceHistoryError(err)
	}
	return audienceHistoryRule(row)
}

func (reader *AudienceHistoryReader) GetHistoricalAudienceRuleVersion(ctx context.Context, id int64) (segmentport.HistoricalAudienceRuleVersion, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceRuleVersion{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, err
	}
	row, err := queries.GetHistoricalAudienceRuleVersion(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, audienceHistoryError(err)
	}
	return audienceHistoryRuleVersion(row)
}

func (reader *AudienceHistoryReader) GetHistoricalAudienceDefinition(ctx context.Context, id int64) (segmentport.HistoricalAudienceDefinition, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceDefinition{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, err
	}
	row, err := queries.GetHistoricalAudienceDefinition(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, audienceHistoryError(err)
	}
	return audienceHistoryDefinition(row)
}

func (reader *AudienceHistoryReader) GetHistoricalAudienceMember(ctx context.Context, id int64) (segmentport.HistoricalAudienceMember, error) {
	if id < 1 {
		return segmentport.HistoricalAudienceMember{}, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	row, err := queries.GetHistoricalAudienceMember(ctx, id)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, audienceHistoryError(err)
	}
	return audienceHistoryMember(row)
}

func (reader *AudienceHistoryReader) ListHistoricalAudienceGroups(ctx context.Context, limit, offset int32) ([]segmentport.HistoricalAudienceGroup, int64, error) {
	queries, err := reader.queriesForPage(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := audienceHistoryCount(queries.CountHistoricalAudienceGroups(ctx))
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListHistoricalAudienceGroups(ctx, segmentdb.ListHistoricalAudienceGroupsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceHistoryError(err)
	}
	result := make([]segmentport.HistoricalAudienceGroup, 0, len(rows))
	for _, row := range rows {
		value, err := audienceHistoryGroup(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *AudienceHistoryReader) ListHistoricalAudiencePackages(ctx context.Context, limit, offset int32) ([]segmentport.HistoricalAudiencePackage, int64, error) {
	queries, err := reader.queriesForPage(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := audienceHistoryCount(queries.CountHistoricalAudiencePackages(ctx))
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListHistoricalAudiencePackages(ctx, segmentdb.ListHistoricalAudiencePackagesParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceHistoryError(err)
	}
	result := make([]segmentport.HistoricalAudiencePackage, 0, len(rows))
	for _, row := range rows {
		value, err := audienceHistoryPackage(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *AudienceHistoryReader) ListHistoricalAudienceVersions(ctx context.Context, packageID int64, limit, offset int32) ([]segmentport.HistoricalAudienceVersion, int64, error) {
	if packageID < 1 {
		return nil, 0, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queriesForPage(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := audienceHistoryCount(queries.CountHistoricalAudienceVersions(ctx, packageID))
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListHistoricalAudienceVersions(ctx, segmentdb.ListHistoricalAudienceVersionsParams{PackageHistoryID: packageID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceHistoryError(err)
	}
	result := make([]segmentport.HistoricalAudienceVersion, 0, len(rows))
	for _, row := range rows {
		value, err := audienceHistoryVersion(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *AudienceHistoryReader) ListHistoricalAudienceSenders(ctx context.Context, packageID int64, limit, offset int32) ([]segmentport.HistoricalAudienceSender, int64, error) {
	if packageID < 1 {
		return nil, 0, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queriesForPage(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := audienceHistoryCount(queries.CountHistoricalAudienceSenders(ctx, packageID))
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListHistoricalAudienceSenders(ctx, segmentdb.ListHistoricalAudienceSendersParams{PackageHistoryID: packageID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceHistoryError(err)
	}
	result := make([]segmentport.HistoricalAudienceSender, 0, len(rows))
	for _, row := range rows {
		value, err := audienceHistorySender(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *AudienceHistoryReader) ListHistoricalAudienceRules(ctx context.Context, limit, offset int32) ([]segmentport.HistoricalAudienceRule, int64, error) {
	queries, err := reader.queriesForPage(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := audienceHistoryCount(queries.CountHistoricalAudienceRules(ctx))
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListHistoricalAudienceRules(ctx, segmentdb.ListHistoricalAudienceRulesParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceHistoryError(err)
	}
	result := make([]segmentport.HistoricalAudienceRule, 0, len(rows))
	for _, row := range rows {
		value, err := audienceHistoryRule(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *AudienceHistoryReader) ListHistoricalAudienceRuleVersions(ctx context.Context, ruleID int64, limit, offset int32) ([]segmentport.HistoricalAudienceRuleVersion, int64, error) {
	if ruleID < 1 {
		return nil, 0, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queriesForPage(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := audienceHistoryCount(queries.CountHistoricalAudienceRuleVersions(ctx, ruleID))
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListHistoricalAudienceRuleVersions(ctx, segmentdb.ListHistoricalAudienceRuleVersionsParams{RuleHistoryID: ruleID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceHistoryError(err)
	}
	result := make([]segmentport.HistoricalAudienceRuleVersion, 0, len(rows))
	for _, row := range rows {
		value, err := audienceHistoryRuleVersion(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *AudienceHistoryReader) ListHistoricalAudienceDefinitions(ctx context.Context, limit, offset int32) ([]segmentport.HistoricalAudienceDefinition, int64, error) {
	queries, err := reader.queriesForPage(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := audienceHistoryCount(queries.CountHistoricalAudienceDefinitions(ctx))
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListHistoricalAudienceDefinitions(ctx, segmentdb.ListHistoricalAudienceDefinitionsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceHistoryError(err)
	}
	result := make([]segmentport.HistoricalAudienceDefinition, 0, len(rows))
	for _, row := range rows {
		value, err := audienceHistoryDefinition(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *AudienceHistoryReader) ListHistoricalAudienceMembers(ctx context.Context, packageID int64, limit, offset int32) ([]segmentport.HistoricalAudienceMember, int64, error) {
	if packageID < 1 {
		return nil, 0, segmentport.ErrAudienceHistoryInvalid
	}
	queries, err := reader.queriesForPage(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := audienceHistoryCount(queries.CountHistoricalAudienceMembers(ctx, packageID))
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListHistoricalAudienceMembers(ctx, segmentdb.ListHistoricalAudienceMembersParams{PackageHistoryID: packageID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceHistoryError(err)
	}
	result := make([]segmentport.HistoricalAudienceMember, 0, len(rows))
	for _, row := range rows {
		value, err := audienceHistoryMember(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *AudienceHistoryReader) queriesForPage(ctx context.Context, limit, offset int32) (*segmentdb.Queries, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, segmentport.ErrAudienceHistoryInvalid
	}
	return reader.queries(ctx)
}

func audienceHistoryCount(count int64, err error) (int64, error) {
	if err != nil {
		return 0, audienceHistoryError(err)
	}
	if count < 0 {
		return 0, segmentport.ErrAudienceHistoryUnavailable
	}
	return count, nil
}

func audienceHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func audienceHistoryNullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return audienceHistoryTimestamp(*value)
}
func audienceHistoryNullableInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func audienceHistoryRequiredTime(value pgtype.Timestamptz) (time.Time, error) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite || value.Time.IsZero() {
		return time.Time{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return value.Time, nil
}
func audienceHistoryOptionalTime(value pgtype.Timestamptz) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	if value.InfinityModifier != pgtype.Finite || value.Time.IsZero() {
		return nil, segmentport.ErrAudienceHistoryUnavailable
	}
	copy := value.Time
	return &copy, nil
}
func audienceHistoryOptionalPositive(value pgtype.Int8) (*int64, error) {
	if !value.Valid {
		return nil, nil
	}
	if value.Int64 < 1 {
		return nil, segmentport.ErrAudienceHistoryUnavailable
	}
	copy := value.Int64
	return &copy, nil
}
func audienceHistoryOptionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}
func audienceHistoryDigest(value []byte) ([32]byte, error) {
	var digest [32]byte
	if len(value) != len(digest) {
		return digest, segmentport.ErrAudienceHistoryUnavailable
	}
	copy(digest[:], value)
	return digest, nil
}
func audienceHistoryError(err error) error {
	var postgresError *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || errors.As(err, &postgresError) && strings.HasPrefix(postgresError.Code, "23") {
		return segmentport.ErrAudienceHistoryConflict
	}
	return segmentport.ErrAudienceHistoryUnavailable
}

func validAudienceHistoryBase(id, sourceID int64, times ...time.Time) bool {
	if id != 0 || sourceID < 1 {
		return false
	}
	for _, stamp := range times {
		if stamp.IsZero() {
			return false
		}
	}
	return true
}
func validAudienceHistoryOptionalID(value *int64) bool       { return value == nil || *value > 0 }
func validAudienceHistoryOptionalTime(value *time.Time) bool { return value == nil || !value.IsZero() }
func validAudienceHistoryGroupCreate(value segmentport.HistoricalAudienceGroup) bool {
	return validAudienceHistoryBase(value.ID, value.SourceID, value.CreatedAt, value.UpdatedAt)
}
func validAudienceHistoryPackageCreate(value segmentport.HistoricalAudiencePackage) bool {
	return validAudienceHistoryBase(value.ID, value.SourceID, value.CreatedAt, value.UpdatedAt) && validAudienceHistoryOptionalID(value.GroupHistoryID) && validAudienceHistoryOptionalID(value.CurrentVersionSourceID) && validAudienceHistoryOptionalTime(value.LastIncrementalAt) && validAudienceHistoryOptionalTime(value.LastDailyRefreshedAt) && validAudienceHistoryOptionalTime(value.NextIncrementalAt) && validAudienceHistoryOptionalTime(value.NextDailyAt)
}
func validAudienceHistoryVersionCreate(value segmentport.HistoricalAudienceVersion) bool {
	return validAudienceHistoryBase(value.ID, value.SourceID, value.CreatedAt) && value.PackageHistoryID > 0 && validAudienceHistoryOptionalTime(value.PublishedAt)
}
func validAudienceHistorySenderCreate(value segmentport.HistoricalAudienceSender) bool {
	return validAudienceHistoryBase(value.ID, value.SourceID, value.CreatedAt, value.UpdatedAt) && value.PackageHistoryID > 0 && validAudienceHistoryOptionalID(value.StaffID)
}
func validAudienceHistoryRuleCreate(value segmentport.HistoricalAudienceRule) bool {
	return validAudienceHistoryBase(value.ID, value.SourceID, value.CreatedAt, value.UpdatedAt) && validAudienceHistoryOptionalID(value.OwnerStaffID)
}
func validAudienceHistoryRuleVersionCreate(value segmentport.HistoricalAudienceRuleVersion) bool {
	return validAudienceHistoryBase(value.ID, value.SourceID, value.CreatedAt) && value.RuleHistoryID > 0 && validAudienceHistoryOptionalTime(value.PublishedAt)
}
func validAudienceHistoryDefinitionCreate(value segmentport.HistoricalAudienceDefinition) bool {
	return validAudienceHistoryBase(value.ID, value.SourceID, value.CreatedAt, value.UpdatedAt) && validAudienceHistoryOptionalTime(value.LastRefreshedAt)
}
func validAudienceHistoryMemberCreate(value segmentport.HistoricalAudienceMember) bool {
	return validAudienceHistoryBase(value.ID, value.SourceID, value.FirstEnteredAt, value.LastSeenAt, value.LastUpdatedAt, value.CreatedAt, value.UpdatedAt) && value.PackageHistoryID > 0 && validAudienceHistoryOptionalID(value.CustomerID) && validAudienceHistoryOptionalTime(value.ExitedAt)
}

func audienceHistoryGroup(row segmentdb.SegmentV1AudienceGroup) (segmentport.HistoricalAudienceGroup, error) {
	created, err := audienceHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceGroup{}, err
	}
	updated, err := audienceHistoryRequiredTime(row.UpdatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceGroup{}, err
	}
	if row.ID < 1 || row.SourceID < 1 {
		return segmentport.HistoricalAudienceGroup{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentport.HistoricalAudienceGroup{ID: row.ID, SourceID: row.SourceID, Name: row.Name, CreatedAt: created, UpdatedAt: updated}, nil
}
func audienceHistoryPackage(row segmentdb.SegmentV1AudiencePackage) (segmentport.HistoricalAudiencePackage, error) {
	groupID, err := audienceHistoryOptionalPositive(row.GroupHistoryID)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	versionSourceID, err := audienceHistoryOptionalPositive(row.CurrentVersionSourceID)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	lastIncremental, err := audienceHistoryOptionalTime(row.LastIncrementalAt)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	lastDaily, err := audienceHistoryOptionalTime(row.LastDailyRefreshedAt)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	nextIncremental, err := audienceHistoryOptionalTime(row.NextIncrementalAt)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	nextDaily, err := audienceHistoryOptionalTime(row.NextDailyAt)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	created, err := audienceHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	updated, err := audienceHistoryRequiredTime(row.UpdatedAt)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	digest, err := audienceHistoryDigest(row.RuntimeDigest)
	if err != nil {
		return segmentport.HistoricalAudiencePackage{}, err
	}
	if row.ID < 1 || row.SourceID < 1 {
		return segmentport.HistoricalAudiencePackage{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentport.HistoricalAudiencePackage{ID: row.ID, SourceID: row.SourceID, GroupHistoryID: groupID, CurrentVersionSourceID: versionSourceID, PackageKey: row.PackageKey, Name: row.Name, NaturalLanguageDefinition: row.NaturalLanguageDefinition, OriginalStatus: row.OriginalStatus, QueryMode: row.QueryMode, IdentityPolicy: row.IdentityPolicy, IncrementalEnabled: row.IncrementalEnabled, DailyEnabled: row.DailyEnabled, IncrementalIntervalSecs: row.IncrementalIntervalSeconds, DailyRefreshTime: row.DailyRefreshTime, Timezone: row.Timezone, LookbackSecs: row.LookbackSeconds, LastIncrementalAt: lastIncremental, LastDailyRefreshedAt: lastDaily, NextIncrementalAt: nextIncremental, NextDailyAt: nextDaily, PausedReason: row.PausedReason, CreatedAt: created, UpdatedAt: updated, RuntimeDigest: digest}, nil
}
func audienceHistoryVersion(row segmentdb.SegmentV1AudienceVersion) (segmentport.HistoricalAudienceVersion, error) {
	published, err := audienceHistoryOptionalTime(row.PublishedAt)
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, err
	}
	templateVersion := audienceHistoryOptionalInt64(row.TemplateVersion)
	created, err := audienceHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, err
	}
	digest, err := audienceHistoryDigest(row.DefinitionDigest)
	if err != nil {
		return segmentport.HistoricalAudienceVersion{}, err
	}
	if row.ID < 1 || row.SourceID < 1 || row.PackageHistoryID < 1 {
		return segmentport.HistoricalAudienceVersion{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentport.HistoricalAudienceVersion{ID: row.ID, SourceID: row.SourceID, PackageHistoryID: row.PackageHistoryID, VersionNumber: row.VersionNumber, OriginalStatus: row.OriginalStatus, AIPrompt: row.AiPrompt, AIRationale: row.AiRationale, NaturalLanguageExplanation: row.NaturalLanguageExplanation, CreatedAt: created, PublishedAt: published, TemplateKey: row.TemplateKey, TemplateVersion: templateVersion, TemplateFingerprint: row.TemplateFingerprint, DefinitionDigest: digest}, nil
}
func audienceHistorySender(row segmentdb.SegmentV1AudienceSender) (segmentport.HistoricalAudienceSender, error) {
	staffID, err := audienceHistoryOptionalPositive(row.StaffID)
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, err
	}
	created, err := audienceHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, err
	}
	updated, err := audienceHistoryRequiredTime(row.UpdatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceSender{}, err
	}
	if row.ID < 1 || row.SourceID < 1 || row.PackageHistoryID < 1 {
		return segmentport.HistoricalAudienceSender{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentport.HistoricalAudienceSender{ID: row.ID, SourceID: row.SourceID, PackageHistoryID: row.PackageHistoryID, StaffID: staffID, DisplayName: row.DisplayName, Priority: row.Priority, OriginalStatus: row.OriginalStatus, CreatedAt: created, UpdatedAt: updated}, nil
}
func audienceHistoryRule(row segmentdb.SegmentV1AudienceRule) (segmentport.HistoricalAudienceRule, error) {
	ownerID, err := audienceHistoryOptionalPositive(row.OwnerStaffID)
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, err
	}
	created, err := audienceHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, err
	}
	updated, err := audienceHistoryRequiredTime(row.UpdatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceRule{}, err
	}
	if row.ID < 1 || row.SourceID < 1 {
		return segmentport.HistoricalAudienceRule{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentport.HistoricalAudienceRule{ID: row.ID, SourceID: row.SourceID, RuleKey: row.RuleKey, DisplayName: row.DisplayName, Description: row.Description, RuleType: row.RuleType, OwnerStaffID: ownerID, OriginalStatus: row.OriginalStatus, CreatedAt: created, UpdatedAt: updated}, nil
}
func audienceHistoryRuleVersion(row segmentdb.SegmentV1AudienceRuleVersion) (segmentport.HistoricalAudienceRuleVersion, error) {
	published, err := audienceHistoryOptionalTime(row.PublishedAt)
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, err
	}
	created, err := audienceHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, err
	}
	digest, err := audienceHistoryDigest(row.DefinitionDigest)
	if err != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, err
	}
	if row.ID < 1 || row.SourceID < 1 || row.RuleHistoryID < 1 {
		return segmentport.HistoricalAudienceRuleVersion{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentport.HistoricalAudienceRuleVersion{ID: row.ID, SourceID: row.SourceID, RuleHistoryID: row.RuleHistoryID, Version: row.Version, ExecutorType: row.ExecutorType, OriginalStatus: row.OriginalStatus, PublishedAt: published, CreatedAt: created, DefinitionDigest: digest}, nil
}
func audienceHistoryDefinition(row segmentdb.SegmentV1Definition) (segmentport.HistoricalAudienceDefinition, error) {
	lastRefreshed, err := audienceHistoryOptionalTime(row.LastRefreshedAt)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, err
	}
	created, err := audienceHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, err
	}
	updated, err := audienceHistoryRequiredTime(row.UpdatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, err
	}
	digest, err := audienceHistoryDigest(row.DefinitionDigest)
	if err != nil {
		return segmentport.HistoricalAudienceDefinition{}, err
	}
	if row.ID < 1 || row.SourceID < 1 {
		return segmentport.HistoricalAudienceDefinition{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentport.HistoricalAudienceDefinition{ID: row.ID, SourceID: row.SourceID, Code: row.Code, DisplayName: row.DisplayName, Description: row.Description, SourceType: row.SourceType, SQLDialect: row.SqlDialect, OriginalStatus: row.OriginalStatus, Version: row.Version, CachedHeadcount: row.CachedHeadcount, LastRefreshedAt: lastRefreshed, UsageCount: row.UsageCount, CreatedAt: created, UpdatedAt: updated, DefinitionDigest: digest}, nil
}
func audienceHistoryMember(row segmentdb.SegmentV1AudienceMember) (segmentport.HistoricalAudienceMember, error) {
	customerID, err := audienceHistoryOptionalPositive(row.CustomerID)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	first, err := audienceHistoryRequiredTime(row.FirstEnteredAt)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	lastSeen, err := audienceHistoryRequiredTime(row.LastSeenAt)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	lastUpdated, err := audienceHistoryRequiredTime(row.LastUpdatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	exited, err := audienceHistoryOptionalTime(row.ExitedAt)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	created, err := audienceHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	updated, err := audienceHistoryRequiredTime(row.UpdatedAt)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	digest, err := audienceHistoryDigest(row.PayloadDigest)
	if err != nil {
		return segmentport.HistoricalAudienceMember{}, err
	}
	if row.ID < 1 || row.SourceID < 1 || row.PackageHistoryID < 1 {
		return segmentport.HistoricalAudienceMember{}, segmentport.ErrAudienceHistoryUnavailable
	}
	return segmentport.HistoricalAudienceMember{ID: row.ID, SourceID: row.SourceID, PackageHistoryID: row.PackageHistoryID, CustomerID: customerID, IdentityKind: row.IdentityKind, OriginalStatus: row.OriginalStatus, FirstEnteredAt: first, LastSeenAt: lastSeen, LastUpdatedAt: lastUpdated, ExitedAt: exited, CreatedAt: created, UpdatedAt: updated, PayloadDigest: digest}, nil
}
