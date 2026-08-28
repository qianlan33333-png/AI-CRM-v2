package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	historicalDirectoryGroupChats = "group_chats"
	historicalDirectorySnapshots  = "wecom_group_chat_snapshots"
)

var _ groupopsport.HistoricalStore = (*Repository)(nil)

// HistoricalReader reads only the immutable V1 history tables through the
// caller-provided UnitOfWork. It never reads or reconstructs execution state.
type HistoricalReader struct {
	uow platformport.UnitOfWork
}

var _ groupopsport.HistoricalReader = (*HistoricalReader)(nil)

func NewHistoricalReader(uow platformport.UnitOfWork) *HistoricalReader {
	return &HistoricalReader{uow: uow}
}

func (r *Repository) CreateHistoricalPlan(ctx context.Context, value groupopsport.HistoricalPlan) (groupopsport.HistoricalPlan, error) {
	if r == nil || !validHistoricalPlan(value) {
		return groupopsport.HistoricalPlan{}, groupopsport.ErrHistoryInvalid
	}
	id, err := r.Create(ctx, value.Plan)
	if err != nil {
		return groupopsport.HistoricalPlan{}, groupOpsHistoryQueryError(err)
	}
	q, err := queries(ctx)
	if err != nil {
		return groupopsport.HistoricalPlan{}, groupOpsHistoryQueryError(err)
	}
	if _, err = q.CreateGroupOpsHistoricalPlanMarker(ctx, groupopsdb.CreateGroupOpsHistoricalPlanMarkerParams{
		PlanID: id, SourcePlanID: value.SourcePlanID, SourceCode: value.SourceCode, PlanType: value.PlanType,
		OriginalStatus: value.OriginalStatus, OwnerStaffID: groupOpsHistoryInt64(value.OwnerStaffID), ArchivedAt: groupOpsHistoryTime(value.ArchivedAt),
	}); err != nil {
		return groupopsport.HistoricalPlan{}, groupOpsHistoryQueryError(err)
	}
	return r.GetHistoricalPlan(ctx, id)
}

func (r *Repository) GetHistoricalPlan(ctx context.Context, id int64) (groupopsport.HistoricalPlan, error) {
	q, err := groupOpsHistoryQueries(r, ctx, id)
	if err != nil {
		return groupopsport.HistoricalPlan{}, err
	}
	row, err := q.GetGroupOpsHistoricalPlan(ctx, id)
	if err != nil {
		return groupopsport.HistoricalPlan{}, groupOpsHistoryQueryError(err)
	}
	return groupOpsHistoricalPlan(row.ID, row.Name, row.Status, row.Revision, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt,
		row.SourcePlanID, row.SourceCode, row.PlanType, row.OriginalStatus, row.OwnerStaffID, row.ArchivedAt)
}

func (r *Repository) CreateHistoricalDirectory(ctx context.Context, value groupopsport.HistoricalDirectory) (groupopsport.HistoricalDirectory, error) {
	if r == nil || !validHistoricalDirectory(value) {
		return groupopsport.HistoricalDirectory{}, groupopsport.ErrHistoryInvalid
	}
	q, err := queries(ctx)
	if err != nil {
		return groupopsport.HistoricalDirectory{}, groupOpsHistoryQueryError(err)
	}
	row, err := q.CreateGroupOpsHistoricalDirectory(ctx, groupopsdb.CreateGroupOpsHistoricalDirectoryParams{
		SourceKind: value.SourceKind, SourceID: groupOpsHistoryInt64(value.SourceID), ChatReference: value.ChatReference,
		DisplayName: groupOpsHistoryText(value.DisplayName), OwnerStaffID: groupOpsHistoryInt64(value.OwnerStaffID), OwnerName: groupOpsHistoryText(value.OwnerName),
		MemberCount: groupOpsHistoryInt32(value.MemberCount), InternalMemberCount: groupOpsHistoryInt32(value.InternalMemberCount), ExternalMemberCount: groupOpsHistoryInt32(value.ExternalMemberCount),
		OriginalStatus: value.OriginalStatus, RecordedAt: timestamp(value.RecordedAt),
	})
	if err != nil {
		return groupopsport.HistoricalDirectory{}, groupOpsHistoryQueryError(err)
	}
	return groupOpsHistoricalDirectory(row)
}

func (r *Repository) GetHistoricalDirectory(ctx context.Context, id int64) (groupopsport.HistoricalDirectory, error) {
	q, err := groupOpsHistoryQueries(r, ctx, id)
	if err != nil {
		return groupopsport.HistoricalDirectory{}, err
	}
	row, err := q.GetGroupOpsHistoricalDirectory(ctx, id)
	if err != nil {
		return groupopsport.HistoricalDirectory{}, groupOpsHistoryQueryError(err)
	}
	return groupOpsHistoricalDirectory(row)
}

func (r *Repository) CreateHistoricalGroup(ctx context.Context, value groupopsport.HistoricalGroup) (groupopsport.HistoricalGroup, error) {
	if r == nil || !validHistoricalGroup(value) {
		return groupopsport.HistoricalGroup{}, groupopsport.ErrHistoryInvalid
	}
	q, err := queries(ctx)
	if err != nil {
		return groupopsport.HistoricalGroup{}, groupOpsHistoryQueryError(err)
	}
	row, err := q.CreateGroupOpsHistoricalGroup(ctx, groupopsdb.CreateGroupOpsHistoricalGroupParams{
		SourceGroupID: value.SourceGroupID, SourcePlanID: value.SourcePlanID, PlanID: value.PlanID, ChatReference: value.ChatReference,
		DisplayName: value.DisplayName, OwnerStaffID: groupOpsHistoryInt64(value.OwnerStaffID), InternalMemberCount: value.InternalMemberCount,
		ExternalMemberCount: value.ExternalMemberCount, OriginalStatus: value.OriginalStatus, CreatedAt: timestamp(value.CreatedAt), RemovedAt: groupOpsHistoryTime(value.RemovedAt),
	})
	if err != nil {
		return groupopsport.HistoricalGroup{}, groupOpsHistoryQueryError(err)
	}
	return groupOpsHistoricalGroup(row)
}

func (r *Repository) GetHistoricalGroup(ctx context.Context, id int64) (groupopsport.HistoricalGroup, error) {
	q, err := groupOpsHistoryQueries(r, ctx, id)
	if err != nil {
		return groupopsport.HistoricalGroup{}, err
	}
	row, err := q.GetGroupOpsHistoricalGroup(ctx, id)
	if err != nil {
		return groupopsport.HistoricalGroup{}, groupOpsHistoryQueryError(err)
	}
	return groupOpsHistoricalGroup(row)
}

func (r *Repository) CreateHistoricalNode(ctx context.Context, value groupopsport.HistoricalNode) (groupopsport.HistoricalNode, error) {
	if r == nil || !validHistoricalNode(value) {
		return groupopsport.HistoricalNode{}, groupopsport.ErrHistoryInvalid
	}
	q, err := queries(ctx)
	if err != nil {
		return groupopsport.HistoricalNode{}, groupOpsHistoryQueryError(err)
	}
	row, err := q.CreateGroupOpsHistoricalNode(ctx, groupopsdb.CreateGroupOpsHistoricalNodeParams{
		SourceNodeID: value.SourceNodeID, SourcePlanID: value.SourcePlanID, PlanID: value.PlanID, DayIndex: value.DayIndex,
		TriggerTime: value.TriggerTime, SortOrder: value.SortOrder, OriginalStatus: value.OriginalStatus,
		ContentPackage: append([]byte(nil), value.ContentPackage...), CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt),
	})
	if err != nil {
		return groupopsport.HistoricalNode{}, groupOpsHistoryQueryError(err)
	}
	return groupOpsHistoricalNode(row)
}

func (r *Repository) GetHistoricalNode(ctx context.Context, id int64) (groupopsport.HistoricalNode, error) {
	q, err := groupOpsHistoryQueries(r, ctx, id)
	if err != nil {
		return groupopsport.HistoricalNode{}, err
	}
	row, err := q.GetGroupOpsHistoricalNode(ctx, id)
	if err != nil {
		return groupopsport.HistoricalNode{}, groupOpsHistoryQueryError(err)
	}
	return groupOpsHistoricalNode(row)
}

func (reader *HistoricalReader) ListHistoricalPlans(ctx context.Context, limit, offset int32) ([]groupopsport.HistoricalPlan, int64, error) {
	values := make([]groupopsport.HistoricalPlan, 0)
	if !validGroupOpsHistoryPage(limit, offset) {
		return values, 0, groupopsport.ErrHistoryInvalid
	}
	var total int64
	err := reader.withQueries(ctx, func(tx context.Context, q *groupopsdb.Queries) error {
		rows, err := q.ListGroupOpsHistoricalPlans(tx, groupopsdb.ListGroupOpsHistoricalPlansParams{RowLimit: limit, RowOffset: offset})
		if err != nil {
			return groupOpsHistoryQueryError(err)
		}
		total, err = q.CountGroupOpsHistoricalPlans(tx)
		if err != nil || total < 0 {
			return groupOpsHistoryQueryError(err)
		}
		values = make([]groupopsport.HistoricalPlan, len(rows))
		for index, row := range rows {
			values[index], err = groupOpsHistoricalPlan(row.ID, row.Name, row.Status, row.Revision, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt,
				row.SourcePlanID, row.SourceCode, row.PlanType, row.OriginalStatus, row.OwnerStaffID, row.ArchivedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return make([]groupopsport.HistoricalPlan, 0), 0, err
	}
	return values, total, nil
}

func (reader *HistoricalReader) ListHistoricalDirectory(ctx context.Context, limit, offset int32) ([]groupopsport.HistoricalDirectory, int64, error) {
	values := make([]groupopsport.HistoricalDirectory, 0)
	if !validGroupOpsHistoryPage(limit, offset) {
		return values, 0, groupopsport.ErrHistoryInvalid
	}
	var total int64
	err := reader.withQueries(ctx, func(tx context.Context, q *groupopsdb.Queries) error {
		rows, err := q.ListGroupOpsHistoricalDirectory(tx, groupopsdb.ListGroupOpsHistoricalDirectoryParams{RowLimit: limit, RowOffset: offset})
		if err != nil {
			return groupOpsHistoryQueryError(err)
		}
		total, err = q.CountGroupOpsHistoricalDirectory(tx)
		if err != nil || total < 0 {
			return groupOpsHistoryQueryError(err)
		}
		values = make([]groupopsport.HistoricalDirectory, len(rows))
		for index, row := range rows {
			values[index], err = groupOpsHistoricalDirectory(row)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return make([]groupopsport.HistoricalDirectory, 0), 0, err
	}
	return values, total, nil
}

func (reader *HistoricalReader) ListHistoricalGroups(ctx context.Context, planID int64, limit, offset int32) ([]groupopsport.HistoricalGroup, int64, error) {
	values := make([]groupopsport.HistoricalGroup, 0)
	if planID < 1 || !validGroupOpsHistoryPage(limit, offset) {
		return values, 0, groupopsport.ErrHistoryInvalid
	}
	var total int64
	err := reader.withQueries(ctx, func(tx context.Context, q *groupopsdb.Queries) error {
		rows, err := q.ListGroupOpsHistoricalGroups(tx, groupopsdb.ListGroupOpsHistoricalGroupsParams{PlanID: planID, RowLimit: limit, RowOffset: offset})
		if err != nil {
			return groupOpsHistoryQueryError(err)
		}
		total, err = q.CountGroupOpsHistoricalGroups(tx, planID)
		if err != nil || total < 0 {
			return groupOpsHistoryQueryError(err)
		}
		values = make([]groupopsport.HistoricalGroup, len(rows))
		for index, row := range rows {
			values[index], err = groupOpsHistoricalGroup(row)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return make([]groupopsport.HistoricalGroup, 0), 0, err
	}
	return values, total, nil
}

func (reader *HistoricalReader) ListHistoricalNodes(ctx context.Context, planID int64, limit, offset int32) ([]groupopsport.HistoricalNode, int64, error) {
	values := make([]groupopsport.HistoricalNode, 0)
	if planID < 1 || !validGroupOpsHistoryPage(limit, offset) {
		return values, 0, groupopsport.ErrHistoryInvalid
	}
	var total int64
	err := reader.withQueries(ctx, func(tx context.Context, q *groupopsdb.Queries) error {
		rows, err := q.ListGroupOpsHistoricalNodes(tx, groupopsdb.ListGroupOpsHistoricalNodesParams{PlanID: planID, RowLimit: limit, RowOffset: offset})
		if err != nil {
			return groupOpsHistoryQueryError(err)
		}
		total, err = q.CountGroupOpsHistoricalNodes(tx, planID)
		if err != nil || total < 0 {
			return groupOpsHistoryQueryError(err)
		}
		values = make([]groupopsport.HistoricalNode, len(rows))
		for index, row := range rows {
			values[index], err = groupOpsHistoricalNode(row)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return make([]groupopsport.HistoricalNode, 0), 0, err
	}
	return values, total, nil
}

func (reader *HistoricalReader) withQueries(ctx context.Context, callback func(context.Context, *groupopsdb.Queries) error) error {
	if reader == nil || reader.uow == nil || ctx == nil || callback == nil {
		return groupopsport.ErrHistoryInvalid
	}
	return reader.uow.Within(ctx, func(tx context.Context) error {
		q, err := queries(tx)
		if err != nil {
			return groupOpsHistoryQueryError(err)
		}
		return callback(tx, q)
	})
}

func groupOpsHistoryQueries(repository *Repository, ctx context.Context, id int64) (*groupopsdb.Queries, error) {
	if repository == nil || id < 1 {
		return nil, groupopsport.ErrHistoryInvalid
	}
	q, err := queries(ctx)
	if err != nil {
		return nil, groupOpsHistoryQueryError(err)
	}
	return q, nil
}

func validHistoricalPlan(value groupopsport.HistoricalPlan) bool {
	return value.Plan.ID == 0 && value.Plan.Status == groupopsport.PlanArchived && value.Plan.Revision == 1 &&
		validGroupOpsHistoryText(value.Plan.Name) && value.Plan.CreatedBy > 0 && value.Plan.UpdatedBy > 0 &&
		!value.Plan.CreatedAt.IsZero() && !value.Plan.UpdatedAt.IsZero() && !value.Plan.UpdatedAt.Before(value.Plan.CreatedAt) &&
		value.SourcePlanID > 0 && validGroupOpsHistoryText(value.SourceCode) && validGroupOpsHistoryText(value.PlanType) &&
		validGroupOpsHistoryText(value.OriginalStatus) && validGroupOpsHistoryOptionalID(value.OwnerStaffID) && validGroupOpsHistoryOptionalTime(value.ArchivedAt)
}

func validHistoricalDirectory(value groupopsport.HistoricalDirectory) bool {
	if value.ID != 0 || !validGroupOpsHistoryText(value.ChatReference) || !validGroupOpsHistoryText(value.OriginalStatus) || value.RecordedAt.IsZero() ||
		!validGroupOpsHistoryOptionalID(value.OwnerStaffID) || !validGroupOpsHistoryOptionalText(value.DisplayName) || !validGroupOpsHistoryOptionalText(value.OwnerName) {
		return false
	}
	switch value.SourceKind {
	case historicalDirectoryGroupChats:
		return validGroupOpsHistoryRequiredID(value.SourceID) && validGroupOpsHistoryNonNegative(value.MemberCount) &&
			value.InternalMemberCount == nil && value.ExternalMemberCount == nil && value.OwnerName == nil
	case historicalDirectorySnapshots:
		return value.SourceID == nil && value.MemberCount == nil && validGroupOpsHistoryNonNegative(value.InternalMemberCount) && validGroupOpsHistoryNonNegative(value.ExternalMemberCount)
	default:
		return false
	}
}

func validHistoricalGroup(value groupopsport.HistoricalGroup) bool {
	return value.ID == 0 && value.SourceGroupID > 0 && value.SourcePlanID > 0 && value.PlanID > 0 &&
		validGroupOpsHistoryText(value.ChatReference) && validGroupOpsHistoryText(value.DisplayName) && validGroupOpsHistoryText(value.OriginalStatus) &&
		value.InternalMemberCount >= 0 && value.ExternalMemberCount >= 0 && validGroupOpsHistoryOptionalID(value.OwnerStaffID) &&
		!value.CreatedAt.IsZero() && validGroupOpsHistoryOptionalTime(value.RemovedAt)
}

func validHistoricalNode(value groupopsport.HistoricalNode) bool {
	return value.ID == 0 && value.SourceNodeID > 0 && value.SourcePlanID > 0 && value.PlanID > 0 && value.DayIndex >= 0 && value.SortOrder >= 0 &&
		validGroupOpsHistoryText(value.TriggerTime) && validGroupOpsHistoryText(value.OriginalStatus) && validGroupOpsHistoryJSON(value.ContentPackage) &&
		!value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() && !value.UpdatedAt.Before(value.CreatedAt)
}

func groupOpsHistoricalPlan(id int64, name, status string, revision, createdBy, updatedBy int64, createdAt, updatedAt pgtype.Timestamptz, sourcePlanID int64, sourceCode, planType, originalStatus string, ownerStaffID pgtype.Int8, archivedAt pgtype.Timestamptz) (groupopsport.HistoricalPlan, error) {
	if id < 1 || status != string(groupopsport.PlanArchived) || revision != 1 || createdBy < 1 || updatedBy < 1 || !createdAt.Valid || !updatedAt.Valid || updatedAt.Time.Before(createdAt.Time) ||
		sourcePlanID < 1 || !validGroupOpsHistoryText(name) || !validGroupOpsHistoryText(sourceCode) || !validGroupOpsHistoryText(planType) || !validGroupOpsHistoryText(originalStatus) ||
		!validGroupOpsHistoryPGOptionalID(ownerStaffID) || !validGroupOpsHistoryPGOptionalTime(archivedAt) {
		return groupopsport.HistoricalPlan{}, groupopsport.ErrHistoryConflict
	}
	return groupopsport.HistoricalPlan{Plan: groupopsport.Plan{ID: id, Name: name, Status: groupopsport.PlanArchived, Revision: revision, CreatedBy: createdBy, UpdatedBy: updatedBy, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time},
		SourcePlanID: sourcePlanID, SourceCode: sourceCode, PlanType: planType, OriginalStatus: originalStatus, OwnerStaffID: groupOpsHistoryInt64Ptr(ownerStaffID), ArchivedAt: groupOpsHistoryTimePtr(archivedAt)}, nil
}

func groupOpsHistoricalDirectory(row groupopsdb.GroupOpsV1HistoryDirectory) (groupopsport.HistoricalDirectory, error) {
	value := groupopsport.HistoricalDirectory{ID: row.ID, SourceKind: row.SourceKind, SourceID: groupOpsHistoryInt64Ptr(row.SourceID), ChatReference: row.ChatReference,
		DisplayName: groupOpsHistoryTextPtr(row.DisplayName), OwnerStaffID: groupOpsHistoryInt64Ptr(row.OwnerStaffID), OwnerName: groupOpsHistoryTextPtr(row.OwnerName),
		MemberCount: groupOpsHistoryInt32Ptr(row.MemberCount), InternalMemberCount: groupOpsHistoryInt32Ptr(row.InternalMemberCount), ExternalMemberCount: groupOpsHistoryInt32Ptr(row.ExternalMemberCount),
		OriginalStatus: row.OriginalStatus}
	if !row.RecordedAt.Valid {
		return groupopsport.HistoricalDirectory{}, groupopsport.ErrHistoryConflict
	}
	value.RecordedAt = row.RecordedAt.Time
	id := value.ID
	value.ID = 0
	if !validHistoricalDirectory(value) {
		return groupopsport.HistoricalDirectory{}, groupopsport.ErrHistoryConflict
	}
	value.ID = id
	return value, nil
}

func groupOpsHistoricalGroup(row groupopsdb.GroupOpsV1HistoryGroup) (groupopsport.HistoricalGroup, error) {
	if !row.CreatedAt.Valid || !validGroupOpsHistoryPGOptionalID(row.OwnerStaffID) || !validGroupOpsHistoryPGOptionalTime(row.RemovedAt) {
		return groupopsport.HistoricalGroup{}, groupopsport.ErrHistoryConflict
	}
	value := groupopsport.HistoricalGroup{ID: row.ID, SourceGroupID: row.SourceGroupID, SourcePlanID: row.SourcePlanID, PlanID: row.PlanID,
		ChatReference: row.ChatReference, DisplayName: row.DisplayName, OwnerStaffID: groupOpsHistoryInt64Ptr(row.OwnerStaffID), InternalMemberCount: row.InternalMemberCount,
		ExternalMemberCount: row.ExternalMemberCount, OriginalStatus: row.OriginalStatus, CreatedAt: row.CreatedAt.Time, RemovedAt: groupOpsHistoryTimePtr(row.RemovedAt)}
	id := value.ID
	value.ID = 0
	if !validHistoricalGroup(value) {
		return groupopsport.HistoricalGroup{}, groupopsport.ErrHistoryConflict
	}
	value.ID = id
	return value, nil
}

func groupOpsHistoricalNode(row groupopsdb.GroupOpsV1HistoryNode) (groupopsport.HistoricalNode, error) {
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return groupopsport.HistoricalNode{}, groupopsport.ErrHistoryConflict
	}
	value := groupopsport.HistoricalNode{ID: row.ID, SourceNodeID: row.SourceNodeID, SourcePlanID: row.SourcePlanID, PlanID: row.PlanID,
		DayIndex: row.DayIndex, TriggerTime: row.TriggerTime, SortOrder: row.SortOrder, OriginalStatus: row.OriginalStatus,
		ContentPackage: append(json.RawMessage(nil), row.ContentPackage...), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
	id := value.ID
	value.ID = 0
	if !validHistoricalNode(value) {
		return groupopsport.HistoricalNode{}, groupopsport.ErrHistoryConflict
	}
	value.ID = id
	return value, nil
}

func validGroupOpsHistoryPage(limit, offset int32) bool {
	return limit > 0 && offset >= 0
}

func validGroupOpsHistoryText(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsRune(value, '\x00')
}

func validGroupOpsHistoryOptionalText(value *string) bool {
	return value == nil || (!strings.ContainsRune(*value, '\x00'))
}

func validGroupOpsHistoryOptionalID(value *int64) bool { return value == nil || *value > 0 }

func validGroupOpsHistoryRequiredID(value *int64) bool { return value != nil && *value > 0 }

func validGroupOpsHistoryNonNegative(value *int32) bool { return value != nil && *value >= 0 }

func validGroupOpsHistoryOptionalTime(value *time.Time) bool { return value == nil || !value.IsZero() }

func validGroupOpsHistoryJSON(value json.RawMessage) bool {
	if !json.Valid(value) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil && object != nil
}

func validGroupOpsHistoryPGOptionalID(value pgtype.Int8) bool { return !value.Valid || value.Int64 > 0 }

func validGroupOpsHistoryPGOptionalTime(value pgtype.Timestamptz) bool {
	return !value.Valid || !value.Time.IsZero()
}

func groupOpsHistoryInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func groupOpsHistoryInt32(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func groupOpsHistoryText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func groupOpsHistoryTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

func groupOpsHistoryInt64Ptr(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func groupOpsHistoryInt32Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	result := value.Int32
	return &result
}

func groupOpsHistoryTextPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func groupOpsHistoryTimePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func groupOpsHistoryQueryError(err error) error {
	if err == nil {
		return groupopsport.ErrHistoryUnavailable
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return groupopsport.ErrHistoryConflict
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23505" || postgresError.Code == "23503" || postgresError.Code == "23514") {
		return groupopsport.ErrHistoryConflict
	}
	return groupopsport.ErrHistoryUnavailable
}
