package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrProviderDisabled = errors.New("group ops provider disabled")

type ExecutionKey struct {
	NodeID          int64
	TargetReference string
}

type RunReservation struct {
	PlanID          int64
	Trigger         groupopsport.RunTrigger
	SourceKeyDigest [sha256.Size]byte
	PlanRevision    int64
	ScheduledFor    time.Time
	AcceptedAt      time.Time
	AcceptedBy      string
}

type ExecutionDraft struct {
	RunID              int64
	PlanID             int64
	PlanRevision       int64
	NodeID             int64
	NodePosition       int32
	TargetReference    string
	SenderUserID       string
	TargetDigest       string
	ContentSnapshot    json.RawMessage
	ContentDigest      string
	MaterialSnapshot   json.RawMessage
	MaterialDigest     string
	ExecutionKeyDigest [sha256.Size]byte
	ExternalEffectID   string
	CreatedAt          time.Time
}

type RuntimeStore interface {
	ListExecutionKeys(context.Context, int64, int64) ([]ExecutionKey, error)
	ReserveRun(context.Context, RunReservation) (groupopsport.Run, error)
	InsertExecution(context.Context, ExecutionDraft) (groupopsport.Execution, error)
	ReadRunSummary(context.Context, int64) (groupopsport.RunSummary, error)
	ListExecutions(context.Context, int64, int32, int32) ([]groupopsport.Execution, int64, error)
	GetExecution(context.Context, int64) (groupopsport.Execution, error)
	ReconcileExecution(context.Context, int64, string, bool, time.Time) (groupopsport.Execution, error)
	RecordExecutionOutcome(context.Context, int64, groupopsport.ExecutionState, bool, bool, string, int32, time.Time) (groupopsport.Execution, error)
	FindPlanByWebhookReference(context.Context, string) (int64, error)
	ListDirectoryGroups(context.Context, int64, int32, int32) ([]groupopsport.GroupDirectoryItem, int64, error)
	ReplaceDirectoryGroups(context.Context, int64, []groupopsport.GroupDirectoryItem, time.Time) error
	RecordDirectoryRefresh(context.Context, string, int64, int64, [sha256.Size]byte, string, int32, bool, time.Time) error
}

type RuntimeEffects interface {
	Accept(context.Context, eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error)
	Queue(context.Context, eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error)
	Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error)
}

type DispatchJobInserter interface {
	Insert(context.Context, GroupOpsDispatchJobArgs, int64, time.Time) (eer.RiverJobLink, error)
}

type GroupOpsDispatchJobArgs struct {
	EffectID string `json:"effect_id"`
}

func (GroupOpsDispatchJobArgs) Kind() string { return "group_ops_dispatch" }

type RuntimeService struct {
	uow             platformport.UnitOfWork
	plans           Store
	runtime         RuntimeStore
	effects         RuntimeEffects
	staff           contactport.StaffDirectoryReader
	groups          groupopsport.GroupDirectorySource
	senders         groupopsport.ExecutionSenderResolver
	jobs            DispatchJobInserter
	evidence        groupopsport.ReconciliationEvidenceVerifier
	dispatchEnabled bool
	now             func() time.Time
}

func NewRuntimeService(uow platformport.UnitOfWork, plans Store, runtime RuntimeStore, effects RuntimeEffects, staff contactport.StaffDirectoryReader, groups groupopsport.GroupDirectorySource, senders groupopsport.ExecutionSenderResolver, jobs DispatchJobInserter, evidence ...groupopsport.ReconciliationEvidenceVerifier) (*RuntimeService, error) {
	if nilRuntimeDependency(uow) || nilRuntimeDependency(plans) || nilRuntimeDependency(runtime) || nilRuntimeDependency(effects) || nilRuntimeDependency(staff) || nilRuntimeDependency(senders) || nilRuntimeDependency(jobs) {
		return nil, ErrUnavailable
	}
	if len(evidence) > 1 {
		return nil, ErrUnavailable
	}
	service := &RuntimeService{uow: uow, plans: plans, runtime: runtime, effects: effects, staff: staff, groups: groups, senders: senders, jobs: jobs, dispatchEnabled: true, now: time.Now}
	if len(evidence) == 1 && !nilRuntimeDependency(evidence[0]) {
		service.evidence = evidence[0]
	}
	return service, nil
}

// SetDispatchEnabled is composition-only. It prevents request intake from
// accepting EER work when the independently configured write provider is off.
func (service *RuntimeService) SetDispatchEnabled(enabled bool) {
	if service != nil {
		service.dispatchEnabled = enabled
	}
}

func (service *RuntimeService) PreviewRunDue(ctx context.Context, planID int64) (groupopsport.RunDuePreview, error) {
	if ctx == nil || service == nil || service.now == nil || planID < 1 {
		return groupopsport.RunDuePreview{}, ErrInvalid
	}
	now := service.nowUTC()
	var result groupopsport.RunDuePreview
	err := service.uow.Within(ctx, func(tx context.Context) error {
		detail, err := service.plans.Lock(tx, planID)
		if err != nil {
			return err
		}
		if !validDetail(detail, planID) {
			return ErrUnavailable
		}
		result = groupopsport.RunDuePreview{PlanID: planID, PlanStatus: detail.Plan.Status, SnapshotRevision: detail.Plan.Revision, EvaluatedAt: now, Blockers: []string{}, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}
		if detail.Plan.Status != groupopsport.PlanActive {
			result.Blockers = append(result.Blockers, "plan_not_active")
			return nil
		}
		validation := contentValidation(detail)
		if !validation.Valid {
			result.Blockers = append(result.Blockers, validation.IssueCodes...)
			return nil
		}
		existing, err := service.runtime.ListExecutionKeys(tx, planID, detail.Plan.Revision)
		if err != nil {
			return err
		}
		due, next := scheduledExecutions(detail, now, executionKeySet(existing), true)
		result.DueExecutionCount = int32(len(due))
		result.NextDueAt = next
		return nil
	})
	if err != nil {
		return groupopsport.RunDuePreview{}, classify(err)
	}
	return result, nil
}

func (service *RuntimeService) RunDue(ctx context.Context, command groupopsport.RunDueCommand) (groupopsport.RunSummary, error) {
	if command.ActorID < 1 {
		return groupopsport.RunSummary{}, ErrInvalid
	}
	return service.acceptPlan(ctx, groupopsport.AcceptPlanCommand{PlanID: command.PlanID, Trigger: groupopsport.RunTriggerDue, AcceptedBy: "admin:" + strconv.FormatInt(command.ActorID, 10), IdempotencyKey: command.IdempotencyKey})
}

func (service *RuntimeService) AcceptPlan(ctx context.Context, command groupopsport.AcceptPlanCommand) (groupopsport.RunSummary, error) {
	return service.acceptPlan(ctx, command)
}

func (service *RuntimeService) AcceptWebhook(ctx context.Context, reference, idempotencyKey string) (groupopsport.RunSummary, error) {
	if ctx == nil || !opaque(reference) || !validKey(idempotencyKey) {
		return groupopsport.RunSummary{}, ErrInvalid
	}
	var planID int64
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		planID, err = service.runtime.FindPlanByWebhookReference(tx, reference)
		return err
	})
	if err != nil {
		return groupopsport.RunSummary{}, classify(err)
	}
	return service.acceptPlan(ctx, groupopsport.AcceptPlanCommand{PlanID: planID, Trigger: groupopsport.RunTriggerWebhook, AcceptedBy: "webhook:" + reference, IdempotencyKey: idempotencyKey})
}

func (service *RuntimeService) acceptPlan(ctx context.Context, command groupopsport.AcceptPlanCommand) (groupopsport.RunSummary, error) {
	if ctx == nil || service == nil || service.now == nil || command.PlanID < 1 || !validRunTrigger(command.Trigger) || !validAcceptedBy(command.AcceptedBy) || !validKey(command.IdempotencyKey) {
		return groupopsport.RunSummary{}, ErrInvalid
	}
	if !service.dispatchEnabled {
		return groupopsport.RunSummary{}, ErrProviderDisabled
	}
	now := service.nowUTC()
	var summary groupopsport.RunSummary
	err := service.uow.Within(ctx, func(tx context.Context) error {
		detail, err := service.plans.Lock(tx, command.PlanID)
		if err != nil {
			return err
		}
		if !validDetail(detail, command.PlanID) || detail.Plan.Status != groupopsport.PlanActive || !contentValidation(detail).Valid {
			return ErrStateConflict
		}
		dueOnly := command.Trigger == groupopsport.RunTriggerDue
		allDrafts, _ := scheduledExecutions(detail, now, nil, dueOnly)
		if len(allDrafts) == 0 {
			return ErrStateConflict
		}
		operation := runtimeReceiptOperation(command.Trigger)
		payload, digestErr := service.runtimeReceiptPayload(tx, command, detail.Plan.Revision, allDrafts)
		if digestErr != nil {
			return digestErr
		}
		payloadDigest, digestErr := digest(payload)
		if digestErr != nil {
			return ErrInvalid
		}
		reservation := Reservation{ActorScope: command.AcceptedBy, KeyDigest: sha256.Sum256([]byte(command.IdempotencyKey)), PayloadDigest: payloadDigest, CreatedAt: now}
		receipt, owned, reserveErr := service.plans.Reserve(tx, operation, reservation)
		if reserveErr != nil || !receiptMatches(receipt, operation, reservation) {
			return errors.Join(ErrUnavailable, reserveErr)
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decode(receipt.ResultSnapshot, &summary) || !validRuntimeSummary(summary, command, detail.Plan.Revision) {
				return ErrUnavailable
			}
			return nil
		}
		existing, err := service.runtime.ListExecutionKeys(tx, command.PlanID, detail.Plan.Revision)
		if err != nil {
			return err
		}
		drafts := allDrafts
		if dueOnly {
			drafts, _ = scheduledExecutions(detail, now, executionKeySet(existing), true)
		}
		sourceKey := runtimeDigest("group-ops-run", strconv.FormatInt(command.PlanID, 10), strconv.FormatInt(detail.Plan.Revision, 10), string(command.Trigger), command.IdempotencyKey, executionFingerprint(allDrafts))
		if dueOnly {
			sourceKey = runtimeDigest("group-ops-run-due", strconv.FormatInt(command.PlanID, 10), strconv.FormatInt(detail.Plan.Revision, 10), executionFingerprint(allDrafts))
		}
		run, err := service.runtime.ReserveRun(tx, RunReservation{PlanID: command.PlanID, Trigger: command.Trigger, SourceKeyDigest: digestBytes(sourceKey), PlanRevision: detail.Plan.Revision, ScheduledFor: allDrafts[0].scheduledFor, AcceptedAt: now, AcceptedBy: command.AcceptedBy})
		if err != nil {
			return err
		}
		for _, draft := range drafts {
			senderUserID, resolved, senderErr := service.senders.ResolveExecutionSender(tx, draft.target)
			if senderErr != nil || !resolved || !opaque(senderUserID) {
				return errors.Join(ErrUnavailable, senderErr)
			}
			content, contentDigest, material, materialDigest, _, err := snapshots(draft.node)
			if err != nil {
				return err
			}
			executionPayloadDigest := runtimeDigest("group-ops-payload", string(contentDigest), string(materialDigest), senderUserID)
			targetDigest := runtimeDigest("group-ops-target", draft.target)
			envelope, err := eer.NewEnvelope(eer.EnvelopeInput{
				Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast,
				SourceRefDigest: runtimeDigest("group-ops-source", string(sourceKey), strconv.FormatInt(command.PlanID, 10), strconv.FormatInt(detail.Plan.Revision, 10), strconv.FormatInt(draft.node.ID, 10)),
				TargetRefDigest: targetDigest, PayloadDigest: executionPayloadDigest,
				PolicyVersionHash: runtimeDigest("group-ops-policy", "v2", senderUserID),
			})
			if err != nil {
				return err
			}
			projection, _, err := service.effects.Accept(tx, eer.AcceptCommand{ReceiptKeyDigest: runtimeDigest("group-ops-accept", string(sourceKey), strconv.FormatInt(command.PlanID, 10), strconv.FormatInt(detail.Plan.Revision, 10), strconv.FormatInt(draft.node.ID, 10), draft.target), Envelope: envelope})
			if err != nil || projection.State != eer.StateAccepted || projection.Owner != eer.OwnerGroupOps || projection.Kind != eer.KindGroupOpsBroadcast {
				return errors.Join(ErrUnavailable, err)
			}
			executionKey := runtimeDigest("group-ops-execution", string(sourceKey), strconv.FormatInt(draft.node.ID, 10), draft.target, senderUserID)
			if dueOnly {
				executionKey = runtimeDigest("group-ops-run-due-execution", strconv.FormatInt(command.PlanID, 10), strconv.FormatInt(detail.Plan.Revision, 10), strconv.FormatInt(draft.node.ID, 10), draft.target, senderUserID)
			}
			if _, err = service.runtime.InsertExecution(tx, ExecutionDraft{RunID: run.ID, PlanID: command.PlanID, PlanRevision: detail.Plan.Revision, NodeID: draft.node.ID, NodePosition: draft.node.Position, TargetReference: draft.target, SenderUserID: senderUserID, TargetDigest: string(targetDigest), ContentSnapshot: content, ContentDigest: string(contentDigest), MaterialSnapshot: material, MaterialDigest: string(materialDigest), ExecutionKeyDigest: digestBytes(executionKey), ExternalEffectID: projection.ID, CreatedAt: now}); err != nil {
				return err
			}
			link, jobErr := service.jobs.Insert(tx, GroupOpsDispatchJobArgs{EffectID: projection.ID}, projection.Generation+1, draft.scheduledFor)
			if jobErr != nil {
				return jobErr
			}
			queued, _, queueErr := service.effects.Queue(tx, eer.QueueCommand{EffectID: projection.ID, Job: link, ReceiptKeyDigest: runtimeDigest("group-ops-queue", projection.ID, strconv.FormatInt(projection.Generation+1, 10))})
			if queueErr != nil || queued.ID != projection.ID || queued.Owner != eer.OwnerGroupOps || queued.Kind != eer.KindGroupOpsBroadcast || queued.State != eer.StateQueued || queued.Generation != link.Generation {
				return errors.Join(ErrUnavailable, queueErr)
			}
		}
		summary, err = service.runtime.ReadRunSummary(tx, run.ID)
		if err != nil || !validRuntimeSummary(summary, command, detail.Plan.Revision) {
			return errors.Join(ErrUnavailable, err)
		}
		snapshot, marshalErr := json.Marshal(summary)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := service.plans.Complete(tx, receipt.ID, snapshot, now)
		if completeErr != nil || !receiptMatches(completed, operation, reservation) || completed.State != "completed" || !jsonEqual(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return groupopsport.RunSummary{}, classifyRuntime(err)
	}
	return summary, nil
}

type runtimeReceiptSnapshot struct {
	NodeID          int64     `json:"node_id"`
	NodePosition    int32     `json:"node_position"`
	NodeKind        string    `json:"node_kind"`
	TargetReference string    `json:"target_reference"`
	SenderUserID    string    `json:"sender_userid"`
	ScheduledFor    time.Time `json:"scheduled_for"`
	ContentDigest   string    `json:"content_digest"`
	MaterialDigest  string    `json:"material_digest"`
}

type runtimeReceiptCommand struct {
	PlanID       int64                    `json:"plan_id"`
	Trigger      groupopsport.RunTrigger  `json:"trigger"`
	AcceptedBy   string                   `json:"accepted_by"`
	PlanRevision int64                    `json:"plan_revision"`
	Snapshot     []runtimeReceiptSnapshot `json:"snapshot"`
}

func runtimeReceiptOperation(trigger groupopsport.RunTrigger) string {
	return "runtime_" + string(trigger)
}

func (service *RuntimeService) runtimeReceiptPayload(ctx context.Context, command groupopsport.AcceptPlanCommand, revision int64, drafts []scheduledExecution) (runtimeReceiptCommand, error) {
	result := runtimeReceiptCommand{PlanID: command.PlanID, Trigger: command.Trigger, AcceptedBy: command.AcceptedBy, PlanRevision: revision, Snapshot: make([]runtimeReceiptSnapshot, len(drafts))}
	for index, draft := range drafts {
		sender, resolved, err := service.senders.ResolveExecutionSender(ctx, draft.target)
		if err != nil || !resolved || !opaque(sender) {
			return runtimeReceiptCommand{}, ErrUnavailable
		}
		_, content, _, material, _, err := snapshots(draft.node)
		if err != nil {
			return runtimeReceiptCommand{}, err
		}
		result.Snapshot[index] = runtimeReceiptSnapshot{NodeID: draft.node.ID, NodePosition: draft.node.Position, NodeKind: string(draft.node.Kind), TargetReference: draft.target, SenderUserID: sender, ScheduledFor: draft.scheduledFor.UTC(), ContentDigest: string(content), MaterialDigest: string(material)}
	}
	return result, nil
}

func validRuntimeSummary(value groupopsport.RunSummary, command groupopsport.AcceptPlanCommand, revision int64) bool {
	if value.Run.ID < 1 || value.Run.PlanID != command.PlanID || value.Run.Trigger != command.Trigger || value.Run.PlanRevision != revision || value.Run.AcceptedBy != command.AcceptedBy || value.Run.ScheduledFor.IsZero() || value.Run.AcceptedAt.IsZero() || value.ProviderExecutionEligible || value.RealExternalCallExecuted || value.ProviderAccepted != 0 || value.DeliveryProven != 0 || value.OutcomeUnknown != 0 || value.Reconciled != 0 || value.FinalFailed != 0 || value.Accepted != int32(len(value.Executions)) {
		return false
	}
	for _, execution := range value.Executions {
		if execution.ID < 1 || execution.RunID != value.Run.ID || execution.PlanID != command.PlanID || execution.PlanRevision != revision || execution.State != groupopsport.ExecutionAccepted || execution.ProviderAccepted || execution.DeliveryProven || execution.AttemptCount != 0 || execution.ProviderReceiptPresent || execution.ReconciliationEvidencePresent || execution.ExternalEffectID == "" {
			return false
		}
	}
	return true
}

func (service *RuntimeService) ListExecutions(ctx context.Context, planID int64, limit, offset int32) (groupopsport.ExecutionPage, error) {
	if ctx == nil || service == nil || planID < 1 || !validPage(limit, offset) {
		return groupopsport.ExecutionPage{}, ErrInvalid
	}
	result := groupopsport.ExecutionPage{Items: []groupopsport.Execution{}, Limit: limit, Offset: offset, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result.Items, result.Total, err = service.runtime.ListExecutions(tx, planID, limit, offset)
		return err
	})
	if err != nil {
		return groupopsport.ExecutionPage{}, classify(err)
	}
	result.HasMore = int64(offset)+int64(len(result.Items)) < result.Total
	return result, nil
}

// ProjectExecutionOutcome records an adapter-observed outcome only. It never
// invokes a Provider and outcome_unknown has no automatic retry transition.
func (service *RuntimeService) ProjectExecutionOutcome(ctx context.Context, command groupopsport.ExecutionOutcomeCommand) (groupopsport.Execution, error) {
	if ctx == nil || service == nil || service.now == nil || command.ExecutionID < 1 || command.AttemptCount < 1 ||
		!validProjectedOutcome(command.State, command.ProviderAccepted, command.DeliveryProven) ||
		(command.ProviderReceiptDigest != "" && !validRuntimeDigest(command.ProviderReceiptDigest)) ||
		(command.ProviderAccepted && command.ProviderReceiptDigest == "") {
		return groupopsport.Execution{}, ErrInvalid
	}
	var result groupopsport.Execution
	err := service.uow.Within(ctx, func(tx context.Context) error {
		current, err := service.runtime.GetExecution(tx, command.ExecutionID)
		if err != nil {
			return err
		}
		if current.State == groupopsport.ExecutionOutcomeUnknown || current.State == groupopsport.ExecutionDeliveryProven || current.State == groupopsport.ExecutionReconciled || current.State == groupopsport.ExecutionFinalFailed || command.AttemptCount < current.AttemptCount || (current.ProviderAccepted && !command.ProviderAccepted) {
			return ErrStateConflict
		}
		result, err = service.runtime.RecordExecutionOutcome(tx, command.ExecutionID, command.State, command.ProviderAccepted, command.DeliveryProven, command.ProviderReceiptDigest, command.AttemptCount, service.nowUTC())
		return err
	})
	if err != nil {
		return groupopsport.Execution{}, classifyRuntime(err)
	}
	return result, nil
}

func (service *RuntimeService) ManualReconcile(ctx context.Context, command groupopsport.ManualReconcileCommand) (groupopsport.Execution, error) {
	if ctx == nil || service == nil || command.ExecutionID < 1 || command.ActorID < 1 || !validKey(command.IdempotencyKey) || command.Generation < 1 || command.Fence < 1 || command.LeaseExpiresAt.IsZero() || !validRuntimeDigest(command.EvidenceDigest) {
		return groupopsport.Execution{}, ErrInvalid
	}
	var result groupopsport.Execution
	err := service.uow.Within(ctx, func(tx context.Context) error {
		current, err := service.runtime.GetExecution(tx, command.ExecutionID)
		if err != nil {
			return err
		}
		if current.State != groupopsport.ExecutionOutcomeUnknown || current.ExternalEffectID == "" {
			return ErrStateConflict
		}
		deliveryProven := false
		verifiedEvidence := command.EvidenceDigest
		if service.evidence != nil {
			verified, verifyErr := service.evidence.VerifyReconciliationEvidence(tx, groupopsport.ReconciliationEvidence{ExecutionID: current.ID, ExternalEffectID: current.ExternalEffectID, EvidenceDigest: command.EvidenceDigest})
			if verifyErr != nil {
				return errors.Join(ErrUnavailable, verifyErr)
			}
			deliveryProven = verified.DeliveryProven
			verifiedEvidence = verified.EvidenceDigest
		}
		// A documented group-message query can prove only an exact status=1
		// record. Missing evidence leaves the EER outcome_unknown.
		if service.evidence != nil && (!deliveryProven || !validRuntimeDigest(verifiedEvidence)) {
			return ErrStateConflict
		}
		projection, _, err := service.effects.Reconcile(tx, eer.ReconcileCommand{
			Lease:            eer.Lease{EffectID: current.ExternalEffectID, Generation: command.Generation, Fence: command.Fence, ExpiresAt: command.LeaseExpiresAt},
			ReceiptKeyDigest: runtimeDigest("group-ops-manual-reconcile", strconv.FormatInt(command.ExecutionID, 10), strconv.FormatInt(command.ActorID, 10), command.IdempotencyKey),
			EvidenceDigest:   eer.Digest(verifiedEvidence),
		})
		if err != nil || projection.ID != current.ExternalEffectID || projection.Owner != eer.OwnerGroupOps || projection.Kind != eer.KindGroupOpsBroadcast || projection.State != eer.StateReconciled {
			return errors.Join(ErrUnavailable, err)
		}
		result, err = service.runtime.ReconcileExecution(tx, command.ExecutionID, verifiedEvidence, deliveryProven, service.nowUTC())
		if err != nil {
			return err
		}
		if result.State != groupopsport.ExecutionReconciled || !result.ReconciliationEvidencePresent || result.DeliveryProven != deliveryProven || (deliveryProven && !result.ProviderAccepted) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return groupopsport.Execution{}, classifyRuntime(err)
	}
	return result, nil
}

func (service *RuntimeService) ListOperationMembers(ctx context.Context, pageSize int32) (groupopsport.OperationMemberPage, error) {
	if ctx == nil || service == nil || pageSize < 1 || pageSize > 100 {
		return groupopsport.OperationMemberPage{}, ErrInvalid
	}
	entries, err := service.staff.ListEligibleStaff(ctx)
	if err != nil {
		return groupopsport.OperationMemberPage{}, ErrUnavailable
	}
	items := make([]groupopsport.OperationMember, 0, min(len(entries), int(pageSize)))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		id, name := strings.TrimSpace(entry.WeComUserID), strings.TrimSpace(entry.DisplayName)
		if !opaque(id) || name == "" {
			return groupopsport.OperationMemberPage{}, ErrUnavailable
		}
		if _, exists := seen[id]; exists {
			return groupopsport.OperationMemberPage{}, ErrUnavailable
		}
		seen[id] = struct{}{}
		if len(items) < int(pageSize) {
			items = append(items, groupopsport.OperationMember{SenderUserID: id, DisplayName: name})
		}
	}
	return groupopsport.OperationMemberPage{Scope: "group_ops", Items: items, PageSize: pageSize, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}, nil
}

func (service *RuntimeService) RefreshOperationMembers(ctx context.Context, command groupopsport.OperationMemberRefreshCommand) (groupopsport.OperationMemberPage, error) {
	if ctx == nil || service == nil || command.ActorID < 1 || command.PageSize < 1 || command.PageSize > 100 || !validKey(command.IdempotencyKey) {
		return groupopsport.OperationMemberPage{}, ErrInvalid
	}
	if nilRuntimeDependency(service.groups) {
		return groupopsport.OperationMemberPage{}, ErrProviderDisabled
	}
	items, err := service.groups.RefreshOperationMembers(ctx, command.PageSize)
	if err != nil || !validOperationMembers(items, command.PageSize) {
		return groupopsport.OperationMemberPage{}, ErrUnavailable
	}
	raw, _ := json.Marshal(items)
	key := sha256.Sum256([]byte(command.IdempotencyKey))
	now := service.nowUTC()
	err = service.uow.Within(ctx, func(tx context.Context) error {
		return service.runtime.RecordDirectoryRefresh(tx, "members", command.ActorID, 0, key, string(runtimeDigest("group-ops-operation-members", string(raw))), int32(len(items)), true, now)
	})
	if err != nil {
		return groupopsport.OperationMemberPage{}, classify(err)
	}
	return groupopsport.OperationMemberPage{Scope: "group_ops", Items: items, PageSize: command.PageSize, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}, nil
}

func (service *RuntimeService) ListGroups(ctx context.Context, ownerStaffID int64, limit, offset int32) (groupopsport.GroupDirectoryPage, error) {
	if ctx == nil || service == nil || ownerStaffID < 0 || !validPage(limit, offset) {
		return groupopsport.GroupDirectoryPage{}, ErrInvalid
	}
	result := groupopsport.GroupDirectoryPage{Items: []groupopsport.GroupDirectoryItem{}, Limit: limit, Offset: offset, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result.Items, result.Total, err = service.runtime.ListDirectoryGroups(tx, ownerStaffID, limit, offset)
		return err
	})
	if err != nil {
		return groupopsport.GroupDirectoryPage{}, classify(err)
	}
	result.HasMore = int64(offset)+int64(len(result.Items)) < result.Total
	return result, nil
}

func (service *RuntimeService) RefreshGroups(ctx context.Context, command groupopsport.GroupRefreshCommand) (groupopsport.GroupDirectoryPage, error) {
	if ctx == nil || service == nil || command.OwnerStaffID < 1 || command.ActorID < 1 || command.Limit < 1 || command.Limit > 200 || !validKey(command.IdempotencyKey) {
		return groupopsport.GroupDirectoryPage{}, ErrInvalid
	}
	if nilRuntimeDependency(service.groups) {
		return groupopsport.GroupDirectoryPage{}, ErrProviderDisabled
	}
	items, err := service.groups.ListOwnedGroups(ctx, command.OwnerStaffID, command.Limit)
	if err != nil || !validDirectoryGroups(items, command.OwnerStaffID, command.Limit) {
		return groupopsport.GroupDirectoryPage{}, ErrUnavailable
	}
	now := service.nowUTC()
	raw, _ := json.Marshal(items)
	snapshotDigest := runtimeDigest("group-ops-group-directory", string(raw))
	key := sha256.Sum256([]byte(command.IdempotencyKey))
	err = service.uow.Within(ctx, func(tx context.Context) error {
		if err := service.runtime.ReplaceDirectoryGroups(tx, command.OwnerStaffID, items, now); err != nil {
			return err
		}
		return service.runtime.RecordDirectoryRefresh(tx, "groups", command.ActorID, command.OwnerStaffID, key, string(snapshotDigest), int32(len(items)), true, now)
	})
	if err != nil {
		return groupopsport.GroupDirectoryPage{}, classify(err)
	}
	return service.ListGroups(ctx, command.OwnerStaffID, command.Limit, 0)
}

type scheduledExecution struct {
	node         groupopsport.Node
	target       string
	scheduledFor time.Time
}

func scheduledExecutions(detail groupopsport.Detail, now time.Time, existing map[ExecutionKey]struct{}, dueOnly bool) ([]scheduledExecution, *time.Time) {
	cursor := detail.Plan.UpdatedAt.UTC()
	result := []scheduledExecution{}
	var next *time.Time
	for _, node := range detail.Nodes {
		if node.Kind == groupopsport.NodeDelay {
			cursor = cursor.Add(time.Duration(node.DelayMinutes) * time.Minute)
			continue
		}
		if dueOnly && cursor.After(now) {
			value := cursor
			if next == nil || value.Before(*next) {
				next = &value
			}
			continue
		}
		for _, target := range detail.GroupAssets {
			key := ExecutionKey{NodeID: node.ID, TargetReference: target.AssetRef}
			if existing == nil {
				result = append(result, scheduledExecution{node: node, target: target.AssetRef, scheduledFor: cursor})
			} else if _, done := existing[key]; !done {
				result = append(result, scheduledExecution{node: node, target: target.AssetRef, scheduledFor: cursor})
			}
		}
	}
	return result, next
}

func snapshots(node groupopsport.Node) (json.RawMessage, eer.Digest, json.RawMessage, eer.Digest, eer.Digest, error) {
	content, err := json.Marshal(struct {
		SchemaVersion int32  `json:"schema_version"`
		NodeKind      string `json:"node_kind"`
		MessageText   string `json:"message_text"`
	}{SchemaVersion: 1, NodeKind: string(node.Kind), MessageText: node.MessageText})
	if err != nil {
		return nil, "", nil, "", "", err
	}
	material, err := json.Marshal(struct {
		SchemaVersion int32  `json:"schema_version"`
		NodeKind      string `json:"node_kind"`
		Reference     string `json:"reference"`
	}{SchemaVersion: 1, NodeKind: string(node.Kind), Reference: node.MaterialRef})
	if err != nil {
		return nil, "", nil, "", "", err
	}
	contentDigest := runtimeDigest("group-ops-content", string(content))
	materialDigest := runtimeDigest("group-ops-material", string(material))
	return content, contentDigest, material, materialDigest, runtimeDigest("group-ops-payload", string(contentDigest), string(materialDigest)), nil
}

func executionFingerprint(values []scheduledExecution) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.FormatInt(value.node.ID, 10) + ":" + value.target
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x00")
}

func executionKeySet(values []ExecutionKey) map[ExecutionKey]struct{} {
	result := make(map[ExecutionKey]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func runtimeDigest(label string, values ...string) eer.Digest {
	sum := sha256.Sum256([]byte(label + "\x00" + strings.Join(values, "\x00")))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func digestBytes(value eer.Digest) [sha256.Size]byte {
	var result [sha256.Size]byte
	decoded, _ := hex.DecodeString(strings.TrimPrefix(string(value), "sha256:"))
	copy(result[:], decoded)
	return result
}

func validRuntimeDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validRunTrigger(value groupopsport.RunTrigger) bool {
	return value == groupopsport.RunTriggerDue || value == groupopsport.RunTriggerBroadcast || value == groupopsport.RunTriggerWebhook
}

func validProjectedOutcome(state groupopsport.ExecutionState, providerAccepted, deliveryProven bool) bool {
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

func validAcceptedBy(value string) bool {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 || parts[1] == "" || !opaque(parts[1]) {
		return false
	}
	if parts[0] == "admin" {
		_, err := strconv.ParseInt(parts[1], 10, 64)
		return err == nil && parts[1][0] != '0'
	}
	return parts[0] == "service" || parts[0] == "webhook"
}

func validDirectoryGroups(items []groupopsport.GroupDirectoryItem, owner int64, limit int32) bool {
	if items == nil || len(items) > int(limit) {
		return false
	}
	seen := map[string]struct{}{}
	for _, item := range items {
		if !opaque(item.ChatReference) || item.OwnerStaffID != owner || strings.TrimSpace(item.DisplayName) != item.DisplayName || item.DisplayName == "" || len([]rune(item.DisplayName)) > 128 || item.MemberCount < 0 {
			return false
		}
		if _, ok := seen[item.ChatReference]; ok {
			return false
		}
		seen[item.ChatReference] = struct{}{}
	}
	return true
}

func validOperationMembers(items []groupopsport.OperationMember, limit int32) bool {
	if items == nil || len(items) > int(limit) {
		return false
	}
	seen := map[string]struct{}{}
	for _, item := range items {
		if !opaque(item.SenderUserID) || strings.TrimSpace(item.DisplayName) != item.DisplayName || item.DisplayName == "" || len([]rune(item.DisplayName)) > 128 {
			return false
		}
		if _, exists := seen[item.SenderUserID]; exists {
			return false
		}
		seen[item.SenderUserID] = struct{}{}
	}
	return true
}

func (service *RuntimeService) nowUTC() time.Time {
	if service == nil || service.now == nil {
		return time.Time{}
	}
	return service.now().UTC().Truncate(time.Microsecond)
}

func classifyRuntime(err error) error {
	if errors.Is(err, ErrProviderDisabled) {
		return err
	}
	return classify(err)
}

func nilRuntimeDependency(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	return ref.Kind() == reflect.Ptr && ref.IsNil()
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
