package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store/generated"
)

var _ groupopsapp.RuntimeStore = (*Repository)(nil)

func (repository *Repository) ListExecutionKeys(ctx context.Context, planID, revision int64) ([]groupopsapp.ExecutionKey, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || planID < 1 || revision < 1 {
		return nil, unavailable(err)
	}
	rows, err := q.ListGroupOpsExecutionKeys(ctx, groupopsdb.ListGroupOpsExecutionKeysParams{PlanID: planID, PlanRevision: revision})
	if err != nil {
		return nil, unavailable(err)
	}
	result := make([]groupopsapp.ExecutionKey, len(rows))
	for index, row := range rows {
		result[index] = groupopsapp.ExecutionKey{NodeID: row.NodeID, TargetReference: row.TargetReference}
	}
	return result, nil
}

func (repository *Repository) ReserveRun(ctx context.Context, reservation groupopsapp.RunReservation) (groupopsport.Run, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil {
		return groupopsport.Run{}, unavailable(err)
	}
	row, err := q.ReserveGroupOpsRun(ctx, groupopsdb.ReserveGroupOpsRunParams{
		PlanID: reservation.PlanID, TriggerKind: string(reservation.Trigger), SourceKeyDigest: reservation.SourceKeyDigest[:], PlanRevision: reservation.PlanRevision,
		ScheduledFor: timestamp(reservation.ScheduledFor), AcceptedAt: timestamp(reservation.AcceptedAt), AcceptedBy: reservation.AcceptedBy,
	})
	if err != nil {
		return groupopsport.Run{}, unavailable(err)
	}
	if subtle.ConstantTimeCompare(row.SourceKeyDigest, reservation.SourceKeyDigest[:]) != 1 || row.PlanID != reservation.PlanID || row.PlanRevision != reservation.PlanRevision || row.TriggerKind != string(reservation.Trigger) || row.AcceptedBy != reservation.AcceptedBy {
		return groupopsport.Run{}, groupopsapp.ErrConflict
	}
	return run(row.ID, row.PlanID, row.TriggerKind, row.PlanRevision, row.ScheduledFor, row.AcceptedAt, row.AcceptedBy)
}

func (repository *Repository) InsertExecution(ctx context.Context, draft groupopsapp.ExecutionDraft) (groupopsport.Execution, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || !json.Valid(draft.ContentSnapshot) || !json.Valid(draft.MaterialSnapshot) || strings.TrimSpace(draft.SenderUserID) != draft.SenderUserID || draft.SenderUserID == "" {
		return groupopsport.Execution{}, unavailable(err)
	}
	effectID, err := parseExternalEffectID(draft.ExternalEffectID)
	if err != nil {
		return groupopsport.Execution{}, groupopsapp.ErrInvalid
	}
	row, err := q.InsertGroupOpsExecution(ctx, groupopsdb.InsertGroupOpsExecutionParams{
		RunID: draft.RunID, PlanID: draft.PlanID, NodeID: draft.NodeID, PlanRevision: draft.PlanRevision, NodePosition: draft.NodePosition,
		TargetReference: draft.TargetReference, SenderUseridSnapshot: text(draft.SenderUserID), TargetDigest: draft.TargetDigest, ContentSnapshot: draft.ContentSnapshot, ContentDigest: draft.ContentDigest,
		MaterialSnapshot: draft.MaterialSnapshot, MaterialDigest: draft.MaterialDigest, ExecutionKeyDigest: draft.ExecutionKeyDigest[:], ExternalEffectID: effectID, CreatedAt: timestamp(draft.CreatedAt),
	})
	if err != nil {
		return groupopsport.Execution{}, unavailable(err)
	}
	if row.RunID != draft.RunID || row.PlanID != draft.PlanID || row.PlanRevision != draft.PlanRevision || row.NodeID != draft.NodeID || row.NodePosition != draft.NodePosition || row.TargetReference != draft.TargetReference || row.SenderUseridSnapshot.String != draft.SenderUserID || row.SenderUseridSnapshot.Valid != (draft.SenderUserID != "") || row.TargetDigest != draft.TargetDigest || row.ContentDigest != draft.ContentDigest || row.MaterialDigest != draft.MaterialDigest || subtle.ConstantTimeCompare(row.ExecutionKeyDigest, draft.ExecutionKeyDigest[:]) != 1 || row.ExternalEffectID != effectID || !jsonBytesEqual(row.ContentSnapshot, draft.ContentSnapshot) || !jsonBytesEqual(row.MaterialSnapshot, draft.MaterialSnapshot) {
		return groupopsport.Execution{}, groupopsapp.ErrConflict
	}
	return executionFromInsert(row)
}

func (repository *Repository) ReadRunSummary(ctx context.Context, runID int64) (groupopsport.RunSummary, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || runID < 1 {
		return groupopsport.RunSummary{}, unavailable(err)
	}
	runRow, err := q.GetGroupOpsRun(ctx, runID)
	if err != nil {
		return groupopsport.RunSummary{}, unavailable(err)
	}
	rows, err := q.ListGroupOpsRunExecutions(ctx, runID)
	if err != nil {
		return groupopsport.RunSummary{}, unavailable(err)
	}
	runValue, err := run(runRow.ID, runRow.PlanID, runRow.TriggerKind, runRow.PlanRevision, runRow.ScheduledFor, runRow.AcceptedAt, runRow.AcceptedBy)
	if err != nil {
		return groupopsport.RunSummary{}, err
	}
	result := groupopsport.RunSummary{Run: runValue, Executions: make([]groupopsport.Execution, len(rows)), RuntimeSafety: groupopsport.DisabledRuntimeSafety()}
	for index, row := range rows {
		result.Executions[index], err = execution(row)
		if err != nil {
			return groupopsport.RunSummary{}, err
		}
		countExecution(&result, result.Executions[index])
	}
	return result, nil
}

func (repository *Repository) ListExecutions(ctx context.Context, planID int64, limit, offset int32) ([]groupopsport.Execution, int64, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || planID < 1 {
		return nil, 0, unavailable(err)
	}
	rows, err := q.ListGroupOpsExecutions(ctx, groupopsdb.ListGroupOpsExecutionsParams{PlanID: planID, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, unavailable(err)
	}
	total, err := q.CountGroupOpsExecutions(ctx, planID)
	if err != nil || total < 0 {
		return nil, 0, unavailable(err)
	}
	result := make([]groupopsport.Execution, len(rows))
	for index, row := range rows {
		result[index], err = execution(row)
		if err != nil {
			return nil, 0, err
		}
	}
	return result, total, nil
}

func (repository *Repository) GetExecution(ctx context.Context, executionID int64) (groupopsport.Execution, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || executionID < 1 {
		return groupopsport.Execution{}, unavailable(err)
	}
	row, err := q.GetGroupOpsExecution(ctx, executionID)
	if err != nil {
		return groupopsport.Execution{}, unavailable(err)
	}
	return execution(row)
}

func (repository *Repository) ReconcileExecution(ctx context.Context, executionID int64, evidence string, deliveryProven bool, now time.Time) (groupopsport.Execution, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || executionID < 1 {
		return groupopsport.Execution{}, unavailable(err)
	}
	current, err := q.GetGroupOpsExecution(ctx, executionID)
	if err != nil {
		return groupopsport.Execution{}, unavailable(err)
	}
	row, err := q.ReconcileGroupOpsExecution(ctx, groupopsdb.ReconcileGroupOpsExecutionParams{ProviderAccepted: current.ProviderAccepted || deliveryProven, DeliveryProven: deliveryProven, ReconciliationEvidenceDigest: text(evidence), UpdatedAt: timestamp(now), ExecutionID: executionID})
	if err != nil {
		return groupopsport.Execution{}, unavailable(err)
	}
	return execution(row)
}

func (repository *Repository) RecordExecutionOutcome(ctx context.Context, executionID int64, state groupopsport.ExecutionState, providerAccepted, deliveryProven bool, receipt string, attempts int32, now time.Time) (groupopsport.Execution, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || executionID < 1 || attempts < 1 || !validExecutionOutcome(state, providerAccepted, deliveryProven) {
		return groupopsport.Execution{}, unavailable(err)
	}
	row, err := q.RecordGroupOpsExecutionOutcome(ctx, groupopsdb.RecordGroupOpsExecutionOutcomeParams{State: string(state), ProviderAccepted: providerAccepted, DeliveryProven: deliveryProven, ProviderReceiptDigest: text(receipt), AttemptCount: attempts, UpdatedAt: timestamp(now), ExecutionID: executionID})
	if err != nil {
		return groupopsport.Execution{}, unavailable(err)
	}
	return execution(row)
}

func (repository *Repository) FindPlanByWebhookReference(ctx context.Context, reference string) (int64, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil {
		return 0, unavailable(err)
	}
	id, err := q.FindGroupOpsPlanByWebhookReference(ctx, reference)
	if err != nil {
		return 0, unavailable(err)
	}
	return id, nil
}

func (repository *Repository) ListDirectoryGroups(ctx context.Context, owner int64, limit, offset int32) ([]groupopsport.GroupDirectoryItem, int64, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || owner < 0 {
		return nil, 0, unavailable(err)
	}
	rows, err := q.ListGroupOpsDirectoryGroups(ctx, groupopsdb.ListGroupOpsDirectoryGroupsParams{OwnerStaffID: owner, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, unavailable(err)
	}
	total, err := q.CountGroupOpsDirectoryGroups(ctx, owner)
	if err != nil {
		return nil, 0, unavailable(err)
	}
	result := make([]groupopsport.GroupDirectoryItem, len(rows))
	for index, row := range rows {
		if !row.RefreshedAt.Valid {
			return nil, 0, groupopsapp.ErrUnavailable
		}
		result[index] = groupopsport.GroupDirectoryItem{ChatReference: row.ChatReference, OwnerStaffID: row.OwnerStaffID, DisplayName: row.DisplayName, MemberCount: row.MemberCount, RefreshedAt: row.RefreshedAt.Time.UTC()}
	}
	return result, total, nil
}

func (repository *Repository) ReplaceDirectoryGroups(ctx context.Context, owner int64, items []groupopsport.GroupDirectoryItem, now time.Time) error {
	q, err := queries(ctx)
	if repository == nil || err != nil || owner < 1 {
		return unavailable(err)
	}
	if err = q.DeleteGroupOpsDirectoryGroups(ctx, owner); err != nil {
		return unavailable(err)
	}
	for _, item := range items {
		digest := sha256.Sum256([]byte(strings.Join([]string{item.ChatReference, strconv.FormatInt(owner, 10), item.DisplayName, strconv.FormatInt(int64(item.MemberCount), 10)}, "\x00")))
		if err = q.InsertGroupOpsDirectoryGroup(ctx, groupopsdb.InsertGroupOpsDirectoryGroupParams{ChatReference: item.ChatReference, OwnerStaffID: owner, DisplayName: item.DisplayName, MemberCount: item.MemberCount, SourceDigest: "sha256:" + hex.EncodeToString(digest[:]), RefreshedAt: timestamp(now)}); err != nil {
			return unavailable(err)
		}
	}
	return nil
}

func (repository *Repository) RecordDirectoryRefresh(ctx context.Context, kind string, actor, owner int64, key [sha256.Size]byte, snapshot string, count int32, providerRead bool, now time.Time) error {
	q, err := queries(ctx)
	if repository == nil || err != nil {
		return unavailable(err)
	}
	params := groupopsdb.ReserveGroupOpsDirectoryRefreshParams{RefreshKind: kind, ActorID: actor, OwnerStaffID: pgtype.Int8{Int64: owner, Valid: owner > 0}, KeyDigest: key[:], SnapshotDigest: snapshot, ItemCount: count, ProviderReadExecuted: providerRead, RefreshedAt: timestamp(now)}
	row, err := q.ReserveGroupOpsDirectoryRefresh(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = q.GetGroupOpsDirectoryRefresh(ctx, groupopsdb.GetGroupOpsDirectoryRefreshParams{RefreshKind: kind, ActorID: actor, KeyDigest: key[:]})
	}
	if err != nil {
		return unavailable(err)
	}
	if subtle.ConstantTimeCompare(row.KeyDigest, key[:]) != 1 || row.SnapshotDigest != snapshot || row.ItemCount != count || row.ProviderReadExecuted != providerRead || row.OwnerStaffID.Valid != (owner > 0) || row.OwnerStaffID.Int64 != owner {
		return groupopsapp.ErrConflict
	}
	return nil
}

func run(id, planID int64, trigger string, revision int64, scheduled, accepted pgtype.Timestamptz, acceptedBy string) (groupopsport.Run, error) {
	if id < 1 || planID < 1 || revision < 1 || !scheduled.Valid || !accepted.Valid {
		return groupopsport.Run{}, groupopsapp.ErrUnavailable
	}
	return groupopsport.Run{ID: id, PlanID: planID, Trigger: groupopsport.RunTrigger(trigger), PlanRevision: revision, ScheduledFor: scheduled.Time.UTC(), AcceptedAt: accepted.Time.UTC(), AcceptedBy: acceptedBy}, nil
}

func execution(row groupopsdb.GroupOpsExecution) (groupopsport.Execution, error) {
	return executionFields(row.ID, row.RunID, row.PlanID, row.PlanRevision, row.NodeID, row.NodePosition, row.TargetReference, row.SenderUseridSnapshot, row.TargetDigest, row.ContentDigest, row.MaterialDigest, row.ExternalEffectID, row.State, row.ProviderAccepted, row.DeliveryProven, row.ProviderReceiptDigest, row.ReconciliationEvidenceDigest, row.AttemptCount, row.CreatedAt, row.UpdatedAt)
}

func executionFromInsert(row groupopsdb.InsertGroupOpsExecutionRow) (groupopsport.Execution, error) {
	return executionFields(row.ID, row.RunID, row.PlanID, row.PlanRevision, row.NodeID, row.NodePosition, row.TargetReference, row.SenderUseridSnapshot, row.TargetDigest, row.ContentDigest, row.MaterialDigest, row.ExternalEffectID, row.State, row.ProviderAccepted, row.DeliveryProven, row.ProviderReceiptDigest, row.ReconciliationEvidenceDigest, row.AttemptCount, row.CreatedAt, row.UpdatedAt)
}

func executionFields(id, runID, planID, revision, nodeID int64, position int32, target string, sender pgtype.Text, targetDigest, contentDigest, materialDigest string, effectID int64, state string, providerAccepted, deliveryProven bool, receipt, evidence pgtype.Text, attempts int32, created, updated pgtype.Timestamptz) (groupopsport.Execution, error) {
	if id < 1 || runID < 1 || planID < 1 || revision < 1 || nodeID < 1 || position < 1 || effectID < 1 || !created.Valid || !updated.Valid {
		return groupopsport.Execution{}, groupopsapp.ErrUnavailable
	}
	return groupopsport.Execution{ID: id, RunID: runID, PlanID: planID, PlanRevision: revision, NodeID: nodeID, NodePosition: position, TargetReference: target, TargetDigest: targetDigest, ContentDigest: contentDigest, MaterialDigest: materialDigest, ExternalEffectID: formatExternalEffectID(effectID), State: groupopsport.ExecutionState(state), ProviderAccepted: providerAccepted, DeliveryProven: deliveryProven, AttemptCount: attempts, ProviderReceiptPresent: receipt.Valid, ReconciliationEvidencePresent: evidence.Valid, CreatedAt: created.Time.UTC(), UpdatedAt: updated.Time.UTC()}, nil
}

func countExecution(summary *groupopsport.RunSummary, execution groupopsport.Execution) {
	switch execution.State {
	case groupopsport.ExecutionAccepted:
		summary.Accepted++
	case groupopsport.ExecutionProviderAccepted:
		summary.ProviderAccepted++
	case groupopsport.ExecutionDeliveryProven:
		summary.DeliveryProven++
	case groupopsport.ExecutionOutcomeUnknown:
		summary.OutcomeUnknown++
	case groupopsport.ExecutionReconciled:
		summary.Reconciled++
	case groupopsport.ExecutionFinalFailed:
		summary.FinalFailed++
	}
}

func validExecutionOutcome(state groupopsport.ExecutionState, providerAccepted, deliveryProven bool) bool {
	switch state {
	case groupopsport.ExecutionProviderAccepted:
		return providerAccepted && !deliveryProven
	case groupopsport.ExecutionDeliveryProven:
		return providerAccepted && deliveryProven
	case groupopsport.ExecutionOutcomeUnknown, groupopsport.ExecutionFinalFailed:
		return !deliveryProven
	default:
		return false
	}
}

func parseExternalEffectID(value string) (int64, error) {
	if !strings.HasPrefix(value, "eer_") {
		return 0, groupopsapp.ErrInvalid
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "eer_"), 10, 64)
	if err != nil || id < 1 {
		return 0, groupopsapp.ErrInvalid
	}
	return id, nil
}

func formatExternalEffectID(id int64) string { return "eer_" + strconv.FormatInt(id, 10) }
func text(value string) pgtype.Text          { return pgtype.Text{String: value, Valid: value != ""} }

func jsonBytesEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && strings.TrimSpace(string(left)) != "" && strings.TrimSpace(string(right)) != "" && stringMustJSON(a) == stringMustJSON(b)
}

func stringMustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
