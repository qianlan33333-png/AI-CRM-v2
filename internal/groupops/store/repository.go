package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

var _ groupopsapp.Store = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (r *Repository) List(ctx context.Context, limit, offset int32) ([]groupopsport.Plan, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return nil, unavailable(err)
	}
	rows, err := q.ListGroupOpsPlans(ctx, groupopsdb.ListGroupOpsPlansParams{RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, unavailable(err)
	}
	plans := make([]groupopsport.Plan, len(rows))
	for index, row := range rows {
		plans[index], err = plan(row)
		if err != nil {
			return nil, err
		}
	}
	return plans, nil
}

func (r *Repository) Count(ctx context.Context) (int64, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return 0, unavailable(err)
	}
	total, err := q.CountGroupOpsPlans(ctx)
	if err != nil || total < 0 {
		return 0, unavailable(err)
	}
	return total, nil
}

func (r *Repository) Get(ctx context.Context, planID int64) (groupopsport.Detail, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || planID < 1 {
		return groupopsport.Detail{}, unavailable(err)
	}
	row, err := q.GetGroupOpsPlan(ctx, planID)
	if err != nil {
		return groupopsport.Detail{}, unavailable(err)
	}
	return detail(ctx, q, row)
}

func (r *Repository) Lock(ctx context.Context, planID int64) (groupopsport.Detail, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || planID < 1 {
		return groupopsport.Detail{}, unavailable(err)
	}
	row, err := q.LockGroupOpsPlan(ctx, planID)
	if err != nil {
		return groupopsport.Detail{}, unavailable(err)
	}
	return detail(ctx, q, row)
}

func (r *Repository) Create(ctx context.Context, value groupopsport.Plan) (int64, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return 0, unavailable(err)
	}
	id, err := q.CreateGroupOpsPlan(ctx, groupopsdb.CreateGroupOpsPlanParams{
		Name: value.Name, Status: string(value.Status), Revision: value.Revision, CreatedBy: value.CreatedBy, UpdatedBy: value.UpdatedBy,
		CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt),
	})
	if err != nil || id < 1 {
		return 0, unavailable(err)
	}
	if err = q.CreateGroupOpsPlanWebhookDescriptor(ctx, id); err != nil {
		return 0, unavailable(err)
	}
	return id, nil
}

func (r *Repository) Save(ctx context.Context, value groupopsport.Detail) error {
	q, err := queries(ctx)
	if r == nil || err != nil || value.Plan.ID < 1 || value.Plan.Revision < 2 {
		return unavailable(err)
	}
	id, err := q.SaveGroupOpsPlan(ctx, groupopsdb.SaveGroupOpsPlanParams{
		Name: value.Plan.Name, Status: string(value.Plan.Status), Revision: value.Plan.Revision, UpdatedBy: value.Plan.UpdatedBy,
		UpdatedAt: timestamp(value.Plan.UpdatedAt), PlanID: value.Plan.ID, PreviousRevision: value.Plan.Revision - 1,
	})
	if errors.Is(err, pgx.ErrNoRows) || id != value.Plan.ID {
		return groupopsapp.ErrConflict
	}
	if err != nil {
		return unavailable(err)
	}
	if err = q.DeleteGroupOpsPlanMembers(ctx, value.Plan.ID); err != nil {
		return unavailable(err)
	}
	for _, member := range value.Members {
		if err = q.CreateGroupOpsPlanMember(ctx, groupopsdb.CreateGroupOpsPlanMemberParams{PlanID: value.Plan.ID, StaffID: member.StaffID}); err != nil {
			return unavailable(err)
		}
	}
	assetIDs := persistedAssetIDs(value.GroupAssets)
	if err = q.DeleteMissingGroupOpsPlanGroupAssets(ctx, groupopsdb.DeleteMissingGroupOpsPlanGroupAssetsParams{PlanID: value.Plan.ID, Ids: assetIDs}); err != nil {
		return unavailable(err)
	}
	for _, asset := range value.GroupAssets {
		if err = q.UpsertGroupOpsPlanGroupAsset(ctx, groupopsdb.UpsertGroupOpsPlanGroupAssetParams{PlanID: value.Plan.ID, AssetReference: asset.AssetRef}); err != nil {
			return unavailable(err)
		}
	}
	nodeIDs := persistedNodeIDs(value.Nodes)
	if err = q.DeleteMissingGroupOpsPlanNodes(ctx, groupopsdb.DeleteMissingGroupOpsPlanNodesParams{PlanID: value.Plan.ID, Ids: nodeIDs}); err != nil {
		return unavailable(err)
	}
	for _, node := range value.Nodes {
		if node.ID == 0 {
			err = q.CreateGroupOpsPlanNode(ctx, groupopsdb.CreateGroupOpsPlanNodeParams{PlanID: value.Plan.ID, Position: node.Position, Kind: string(node.Kind), MessageText: node.MessageText, DelayMinutes: node.DelayMinutes, MaterialReference: node.MaterialRef})
		} else {
			var updated int64
			updated, err = q.UpdateGroupOpsPlanNode(ctx, groupopsdb.UpdateGroupOpsPlanNodeParams{PlanID: value.Plan.ID, NodeID: node.ID, Position: node.Position, Kind: string(node.Kind), MessageText: node.MessageText, DelayMinutes: node.DelayMinutes, MaterialReference: node.MaterialRef})
			if err == nil && updated != 1 {
				return groupopsapp.ErrConflict
			}
		}
		if err != nil {
			return unavailable(err)
		}
	}
	updated, err := q.SaveGroupOpsPlanWebhookDescriptor(ctx, groupopsdb.SaveGroupOpsPlanWebhookDescriptorParams{PlanID: value.Plan.ID, Reference: value.WebhookDescriptor.Reference})
	if err != nil || updated != 1 {
		return unavailable(err)
	}
	return nil
}

func (r *Repository) Reserve(ctx context.Context, operation string, reservation groupopsapp.Reservation) (groupopsapp.Receipt, bool, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return groupopsapp.Receipt{}, false, unavailable(err)
	}
	row, err := q.ReserveGroupOpsOperationReceipt(ctx, groupopsdb.ReserveGroupOpsOperationReceiptParams{
		Operation: operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], CreatedAt: timestamp(reservation.CreatedAt),
	})
	if err == nil {
		return receipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return groupopsapp.Receipt{}, false, unavailable(err)
	}
	stored, readErr := q.GetGroupOpsOperationReceipt(ctx, groupopsdb.GetGroupOpsOperationReceiptParams{Operation: operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:]})
	if readErr != nil {
		return groupopsapp.Receipt{}, false, unavailable(readErr)
	}
	return receipt(stored.ID, stored.Operation, stored.ActorScope, stored.KeyDigest, stored.PayloadDigest, stored.State, stored.ResultSnapshot), false, nil
}

func (r *Repository) Complete(ctx context.Context, receiptID int64, snapshot json.RawMessage, now time.Time) (groupopsapp.Receipt, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || receiptID < 1 || now.IsZero() || !json.Valid(snapshot) {
		return groupopsapp.Receipt{}, unavailable(err)
	}
	row, err := q.CompleteGroupOpsOperationReceipt(ctx, groupopsdb.CompleteGroupOpsOperationReceiptParams{ID: receiptID, ResultSnapshot: snapshot, CompletedAt: timestamp(now)})
	if err != nil {
		return groupopsapp.Receipt{}, unavailable(err)
	}
	return receipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func queries(ctx context.Context) (*groupopsdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return groupopsdb.New(tx), nil
}

func detail(ctx context.Context, q *groupopsdb.Queries, row groupopsdb.GroupOpsPlan) (groupopsport.Detail, error) {
	value, err := plan(row)
	if err != nil {
		return groupopsport.Detail{}, err
	}
	members, err := q.ListGroupOpsPlanMembers(ctx, value.ID)
	if err != nil {
		return groupopsport.Detail{}, unavailable(err)
	}
	assets, err := q.ListGroupOpsPlanGroupAssets(ctx, value.ID)
	if err != nil {
		return groupopsport.Detail{}, unavailable(err)
	}
	nodes, err := q.ListGroupOpsPlanNodes(ctx, value.ID)
	if err != nil {
		return groupopsport.Detail{}, unavailable(err)
	}
	reference, err := q.GetGroupOpsPlanWebhookDescriptor(ctx, value.ID)
	if err != nil {
		return groupopsport.Detail{}, unavailable(err)
	}
	result := groupopsport.Detail{
		Plan: value, Members: make([]groupopsport.Member, len(members)), GroupAssets: make([]groupopsport.GroupAsset, len(assets)), Nodes: make([]groupopsport.Node, len(nodes)),
		Safety: groupopsport.LocalSafety(),
	}
	for index, staffID := range members {
		result.Members[index] = groupopsport.Member{StaffID: staffID}
	}
	for index, asset := range assets {
		result.GroupAssets[index] = groupopsport.GroupAsset{ID: asset.ID, AssetRef: asset.AssetReference}
	}
	for index, node := range nodes {
		result.Nodes[index] = groupopsport.Node{ID: node.ID, Position: node.Position, Kind: groupopsport.NodeKind(node.Kind), MessageText: node.MessageText, DelayMinutes: node.DelayMinutes, MaterialRef: node.MaterialReference}
	}
	if reference == "" {
		result.WebhookDescriptor = groupopsport.WebhookDescriptor{Description: "not configured"}
	} else {
		result.WebhookDescriptor = groupopsport.WebhookDescriptor{Configured: true, Reference: reference, Description: "local opaque reference only"}
	}
	return result, nil
}

func plan(row groupopsdb.GroupOpsPlan) (groupopsport.Plan, error) {
	if row.ID < 1 || !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return groupopsport.Plan{}, groupopsapp.ErrUnavailable
	}
	return groupopsport.Plan{ID: row.ID, Name: row.Name, Status: groupopsport.PlanStatus(row.Status), Revision: row.Revision, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

func receipt(id int64, operation, actorScope string, keyDigest, payloadDigest []byte, state string, snapshot []byte) groupopsapp.Receipt {
	value := groupopsapp.Receipt{ID: id, Operation: operation, ActorScope: actorScope, State: state, ResultSnapshot: append(json.RawMessage{}, snapshot...)}
	copy(value.KeyDigest[:], keyDigest)
	copy(value.PayloadDigest[:], payloadDigest)
	return value
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func persistedAssetIDs(values []groupopsport.GroupAsset) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value.ID > 0 {
			result = append(result, value.ID)
		}
	}
	return result
}

func persistedNodeIDs(values []groupopsport.Node) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value.ID > 0 {
			result = append(result, value.ID)
		}
	}
	return result
}

func unavailable(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return groupopsapp.ErrNotFound
	}
	if err != nil {
		return err
	}
	return groupopsapp.ErrUnavailable
}
