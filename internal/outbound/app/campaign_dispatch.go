package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type CampaignDispatchCommand struct {
	CampaignCode   string
	PlanID         string
	ActorID        int64
	IdempotencyKey string
	ExternalGate   bool
}

type CampaignDispatchReconcileCommand struct {
	CampaignCode   string
	PlanID         string
	EffectID       string
	ActorID        int64
	IdempotencyKey string
	Generation     int64
	Fence          int64
	LeaseExpiresAt time.Time
	EvidenceDigest string
}

type CampaignDispatchRuntime interface {
	Accept(context.Context, eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error)
	Queue(context.Context, eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error)
	Claim(context.Context, eer.ClaimCommand) (eer.Lease, eer.Projection, error)
	RunAttempt(context.Context, eer.Lease, eer.Adapter) (eer.Projection, eer.OperationReceipt, error)
	Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error)
}

type CampaignDispatchService struct {
	uow      platformport.UnitOfWork
	repo     outboundport.CampaignDispatchRepository
	runtime  CampaignDispatchRuntime
	enqueuer outboundport.CampaignDispatchEnqueuer
	contact  contactport.EligibilityChecker
	audience outboundport.AudienceDispatchTargetQualifier
	evidence outboundport.CampaignDispatchReconciliationEvidenceVerifier
	now      func() time.Time
}

type campaignDispatchEvidenceAdapter struct {
	adapter                 eer.Adapter
	result                  eer.AdapterResult
	completed               bool
	providerEvidence        outboundport.CampaignDispatchProviderAttemptReceipt
	providerEvidencePresent bool
}

type campaignDispatchProviderEvidenceAdapter interface {
	ExecuteWithCampaignDispatchProviderEvidence(context.Context, eer.EffectEnvelope, eer.Attempt, func(outboundport.CampaignDispatchProviderAttemptReceipt)) (eer.AdapterResult, error)
}

func (adapter *campaignDispatchEvidenceAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	var result eer.AdapterResult
	var err error
	if provider, ok := adapter.adapter.(campaignDispatchProviderEvidenceAdapter); ok {
		result, err = provider.ExecuteWithCampaignDispatchProviderEvidence(ctx, envelope, attempt, func(evidence outboundport.CampaignDispatchProviderAttemptReceipt) {
			adapter.providerEvidence, adapter.providerEvidencePresent = evidence, true
		})
	} else {
		result, err = adapter.adapter.Execute(ctx, envelope, attempt)
	}
	if err == nil {
		adapter.result, adapter.completed = result, true
	}
	return result, err
}

func NewCampaignDispatchService(uow platformport.UnitOfWork, repo outboundport.CampaignDispatchRepository, runtime CampaignDispatchRuntime, enqueuer outboundport.CampaignDispatchEnqueuer, contact contactport.EligibilityChecker) (*CampaignDispatchService, error) {
	if nilCampaignDispatchDependency(uow) || nilCampaignDispatchDependency(repo) || nilCampaignDispatchDependency(runtime) || nilCampaignDispatchDependency(enqueuer) || nilCampaignDispatchDependency(contact) {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	return &CampaignDispatchService{uow: uow, repo: repo, runtime: runtime, enqueuer: enqueuer, contact: contact, now: time.Now}, nil
}

// WithAudienceQualification adds the only accepted bridge for legacy
// Audience package dispatches. The qualifier must run in Dispatch's UoW and
// return the target's active relationship owner only when that owner is in the
// package whitelist.
func (service *CampaignDispatchService) WithAudienceQualification(qualifier outboundport.AudienceDispatchTargetQualifier, verifier outboundport.CampaignDispatchReconciliationEvidenceVerifier) (*CampaignDispatchService, error) {
	if service == nil || qualifier == nil {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	clone := *service
	clone.audience, clone.evidence = qualifier, verifier
	return &clone, nil
}

func (service *CampaignDispatchService) Dispatch(ctx context.Context, command CampaignDispatchCommand) (outbound.CampaignDispatchSummary, error) {
	if ctx == nil || service == nil || service.now == nil || !validCampaignDispatchCommand(command) {
		return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchInvalid
	}
	key := sha256.Sum256([]byte(command.IdempotencyKey))
	payload := sha256.Sum256([]byte("outbound.campaign_dispatch.command.v1\x00" + command.CampaignCode + "\x00" + command.PlanID + "\x00" + boolText(command.ExternalGate)))
	var summary outbound.CampaignDispatchSummary
	err := service.uow.Within(ctx, func(tx context.Context) error {
		evaluatedAt := service.now().UTC()
		if evaluatedAt.IsZero() {
			return outbound.ErrCampaignDispatchUnavailable
		}
		handoffID, err := service.repo.LockCampaignHandoffForDispatch(tx, command.CampaignCode, command.PlanID)
		if err != nil {
			return err
		}
		replayed, present, err := service.repo.LoadCampaignDispatchReceipt(tx, command.ActorID, key)
		if err != nil {
			return err
		}
		if present {
			if replayed.HandoffID != handoffID || replayed.PayloadDigest != payload {
				return outbound.ErrCampaignDispatchConflict
			}
			summary = replayed.Result
			return nil
		}
		candidates, err := service.repo.ListCampaignDispatchCandidates(tx, handoffID)
		if err != nil {
			return err
		}
		eligibility, err := service.dispatchEligibility(tx, candidates, evaluatedAt)
		if err != nil {
			return err
		}
		audiencePackageID, audience, err := service.audiencePackage(tx, handoffID)
		if err != nil {
			return err
		}
		qualifications := map[int64]outboundport.AudienceDispatchTargetQualification(nil)
		if audience {
			qualifications, err = service.audienceQualification(tx, audiencePackageID, candidates)
			if err != nil {
				return err
			}
		}
		for _, candidate := range candidates {
			if candidate.CustomerID < 1 || candidate.StepIndex < 1 || strings.TrimSpace(candidate.Content) == "" {
				return outbound.ErrCampaignDispatchUnavailable
			}
			binding := outboundport.CampaignDispatchBinding{HandoffID: handoffID, CustomerID: candidate.CustomerID, StepIndex: candidate.StepIndex, RecipientDigest: outbound.CampaignDispatchRecipientDigest(candidate.CustomerID), PayloadDigest: outbound.CampaignDispatchPayloadDigest(handoffID, candidate.CustomerID, candidate.StepIndex, candidate.Content), CreatedAt: evaluatedAt, UpdatedAt: evaluatedAt}
			decision, present := eligibility[candidate.CustomerID]
			if !present {
				return outbound.ErrCampaignDispatchUnavailable
			}
			if !decision.Eligible {
				blockReason, valid := campaignDispatchBlockReason(decision.Exclusion)
				if !valid {
					return outbound.ErrCampaignDispatchUnavailable
				}
				binding.State, binding.BlockReason = outbound.CampaignDispatchBlocked, blockReason
				if _, err = service.repo.InsertCampaignDispatchBinding(tx, binding); err != nil {
					return err
				}
				continue
			}
			if audience {
				qualification, present := qualifications[candidate.CustomerID]
				if !present {
					return outbound.ErrCampaignDispatchUnavailable
				}
				if !qualification.Eligible {
					binding.State, binding.BlockReason = outbound.CampaignDispatchBlocked, qualification.Exclusion
					if _, err = service.repo.InsertCampaignDispatchBinding(tx, binding); err != nil {
						return err
					}
					continue
				}
				binding.SenderUserIDSnapshot, binding.ExternalUserIDSnapshot = qualification.SenderUserID, qualification.ExternalUserID
				binding.RecipientDigest = outbound.AudienceCampaignDispatchRecipientDigest(candidate.CustomerID, qualification.SenderUserID, qualification.ExternalUserID)
				binding.PayloadDigest = outbound.AudienceCampaignDispatchPayloadDigest(handoffID, candidate.CustomerID, candidate.StepIndex, candidate.Content, qualification.SenderUserID, qualification.ExternalUserID)
			}
			if !command.ExternalGate {
				binding.State, binding.BlockReason = outbound.CampaignDispatchBlocked, "external_gate_disabled"
				if _, err = service.repo.InsertCampaignDispatchBinding(tx, binding); err != nil {
					return err
				}
				continue
			}
			policy := digest("policy", "c01-v1")
			if audience {
				policy = digest("policy", "c01-audience-v2", strconv.FormatInt(audiencePackageID, 10), binding.SenderUserIDSnapshot)
			}
			envelope, envelopeErr := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, SourceRefDigest: digest("source", command.CampaignCode, command.PlanID), TargetRefDigest: eer.Digest(binding.RecipientDigest), PayloadDigest: eer.Digest(binding.PayloadDigest), PolicyVersionHash: policy})
			if envelopeErr != nil {
				return envelopeErr
			}
			accepted, _, acceptErr := service.runtime.Accept(tx, eer.AcceptCommand{ReceiptKeyDigest: digest("accept", command.IdempotencyKey, string(binding.PayloadDigest)), Envelope: envelope})
			if acceptErr != nil {
				return acceptErr
			}
			binding.ExternalEffectID, binding.State = accepted.ID, outbound.CampaignDispatchAccepted
			stored, insertErr := service.repo.InsertCampaignDispatchBinding(tx, binding)
			if insertErr != nil {
				return insertErr
			}
			// A lost HTTP response can replay after the first transaction has
			// already queued the effect. The immutable binding must still name the
			// same EER effect; Queue itself supplies the idempotent receipt fence.
			if stored.ExternalEffectID != accepted.ID {
				return outbound.ErrCampaignDispatchConflict
			}
			if stored.State != outbound.CampaignDispatchAccepted {
				continue
			}
			job, jobErr := service.enqueuer.EnqueueCampaignDispatch(tx, accepted.ID)
			if jobErr != nil {
				return jobErr
			}
			queued, _, queueErr := service.runtime.Queue(tx, eer.QueueCommand{EffectID: accepted.ID, Job: job, ReceiptKeyDigest: digest("queue", command.IdempotencyKey, accepted.ID)})
			if queueErr != nil || queued.State != eer.StateQueued {
				return errors.Join(outbound.ErrCampaignDispatchUnavailable, queueErr)
			}
			if err = service.repo.UpdateCampaignDispatchState(tx, accepted.ID, outbound.CampaignDispatchQueued); err != nil {
				return err
			}
		}
		summary, err = service.repo.ReadCampaignDispatchSummary(tx, handoffID)
		if err != nil {
			return err
		}
		receipt, reserveErr := service.repo.ReserveCampaignDispatchReceipt(tx, command.ActorID, handoffID, key, payload, summary)
		if reserveErr == nil {
			summary = receipt.Result
		}
		return reserveErr
	})
	if err != nil {
		return outbound.CampaignDispatchSummary{}, campaignDispatchError(err)
	}
	if !outbound.ValidCampaignDispatchSummary(summary) {
		return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchUnavailable
	}
	return summary, nil
}

func (service *CampaignDispatchService) dispatchEligibility(ctx context.Context, candidates []outboundport.CampaignDispatchCandidate, evaluatedAt time.Time) (map[int64]contactport.ContactEligibility, error) {
	if service == nil || service.contact == nil || ctx == nil || ctx.Err() != nil || len(candidates) == 0 || evaluatedAt.IsZero() {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	unique := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.CustomerID < 1 {
			return nil, outbound.ErrCampaignDispatchUnavailable
		}
		unique[candidate.CustomerID] = struct{}{}
	}
	if len(unique) > contactport.ContactEligibilityMaximumCustomers {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	ids := make([]contactport.CustomerID, 0, len(unique))
	for customerID := range unique {
		ids = append(ids, contactport.CustomerID(customerID))
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	decisions, err := service.contact.CheckContactEligibility(ctx, contactport.ContactEligibilityCheck{
		Checkpoint: contactport.ContactEligibilityDispatch, CustomerIDs: ids, EvaluatedAt: evaluatedAt,
	})
	if err != nil || len(decisions) != len(ids) {
		return nil, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	result := make(map[int64]contactport.ContactEligibility, len(decisions))
	for index, decision := range decisions {
		if decision.CustomerID != ids[index] {
			return nil, outbound.ErrCampaignDispatchUnavailable
		}
		if decision.Eligible {
			if !decision.CustomerActive || decision.Exclusion != contactport.ContactEligibilityExclusionNone {
				return nil, outbound.ErrCampaignDispatchUnavailable
			}
		} else {
			switch decision.Exclusion {
			case contactport.ContactEligibilityExclusionContactPolicy:
				if !decision.CustomerActive {
					return nil, outbound.ErrCampaignDispatchUnavailable
				}
			case contactport.ContactEligibilityExclusionInactiveCustomer:
				if decision.CustomerActive {
					return nil, outbound.ErrCampaignDispatchUnavailable
				}
			default:
				return nil, outbound.ErrCampaignDispatchUnavailable
			}
		}
		result[int64(decision.CustomerID)] = decision
	}
	return result, nil
}

func campaignDispatchBlockReason(exclusion contactport.ContactEligibilityExclusion) (string, bool) {
	switch exclusion {
	case contactport.ContactEligibilityExclusionContactPolicy:
		return "contact_policy", true
	case contactport.ContactEligibilityExclusionInactiveCustomer:
		return "inactive_customer", true
	default:
		return "", false
	}
}

// RunEffect is called only by the dedicated River worker. EER persists the
// attempted fence before invoking the adapter; this mirror writes only its
// safe terminal projection and never changes a receipt into delivery proof.
func (service *CampaignDispatchService) RunEffect(ctx context.Context, effectID string, workerDigest eer.Digest, adapter eer.Adapter) error {
	if ctx == nil || service == nil || adapter == nil || effectID == "" {
		return outbound.ErrCampaignDispatchInvalid
	}
	lease, _, err := service.runtime.Claim(ctx, eer.ClaimCommand{EffectID: effectID, WorkerDigest: workerDigest})
	if err != nil {
		return campaignDispatchError(err)
	}
	captured := &campaignDispatchEvidenceAdapter{adapter: adapter}
	projection, receipt, runErr := service.runtime.RunAttempt(ctx, lease, captured)
	state, valid := campaignDispatchState(projection.State)
	if !valid {
		return outbound.ErrCampaignDispatchUnavailable
	}
	writeErr := service.uow.Within(ctx, func(tx context.Context) error {
		if err := service.repo.UpdateCampaignDispatchState(tx, effectID, state); err != nil {
			return err
		}
		evidence := outboundport.CampaignDispatchProviderAttemptReceipt{Completion: string(projection.State), ReceiptDigest: receipt.CommandDigest}
		if captured.completed && outbound.ValidCampaignDispatchDigest(string(captured.result.ReceiptDigest)) {
			evidence.ReceiptDigest = captured.result.ReceiptDigest
			evidence.BusinessCallDispatched = captured.result.BusinessCallDispatched
			evidence.RealExternalCallExecuted = captured.result.RealExternalCallExecuted
		}
		if captured.providerEvidencePresent {
			evidence.ProviderMessageID = captured.providerEvidence.ProviderMessageID
			evidence.ProviderCode = captured.providerEvidence.ProviderCode
			evidence.ProviderResultReceived = captured.providerEvidence.ProviderResultReceived
		}
		return service.repo.RecordCampaignProviderAttemptReceipt(tx, effectID, projection.AttemptCount, evidence)
	})
	if writeErr != nil {
		return campaignDispatchError(writeErr)
	}
	// A transport ambiguity has already become an EER outcome_unknown fact.
	// Returning nil prevents River from invoking the adapter again; only manual
	// reconciliation can advance that state.
	if projection.State == eer.StateOutcomeUnknown && errors.Is(runErr, eer.ErrAdapterFailure) {
		return nil
	}
	if runErr != nil {
		return campaignDispatchError(runErr)
	}
	return nil
}

func (service *CampaignDispatchService) ReconcileEffect(ctx context.Context, command eer.ReconcileCommand) error {
	if ctx == nil || service == nil || command.Lease.EffectID == "" {
		return outbound.ErrCampaignDispatchInvalid
	}
	projection, receipt, err := service.runtime.Reconcile(ctx, command)
	if err != nil {
		return campaignDispatchError(err)
	}
	if projection.State != eer.StateReconciled {
		return outbound.ErrCampaignDispatchUnavailable
	}
	return service.uow.Within(ctx, func(tx context.Context) error {
		if err := service.repo.UpdateCampaignDispatchState(tx, projection.ID, outbound.CampaignDispatchReconciled); err != nil {
			return err
		}
		return service.repo.RecordCampaignProviderAttemptReceipt(tx, projection.ID, projection.AttemptCount, outboundport.CampaignDispatchProviderAttemptReceipt{Completion: string(eer.StateReconciled), ReceiptDigest: receipt.CommandDigest})
	})
}

func (service *CampaignDispatchService) Reconciliation(ctx context.Context, campaignCode, planID string) (outbound.CampaignDispatchSummary, error) {
	if ctx == nil || service == nil || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchInvalid
	}
	var summary outbound.CampaignDispatchSummary
	err := service.uow.Within(ctx, func(tx context.Context) error {
		handoffID, err := service.repo.ReadCampaignHandoffForDispatch(tx, campaignCode, planID)
		if err != nil {
			return err
		}
		summary, err = service.repo.ReadCampaignDispatchSummary(tx, handoffID)
		return err
	})
	if err != nil {
		return outbound.CampaignDispatchSummary{}, campaignDispatchError(err)
	}
	if !outbound.ValidCampaignDispatchSummary(summary) {
		return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchUnavailable
	}
	return summary, nil
}

func (service *CampaignDispatchService) ManualReconcile(ctx context.Context, command CampaignDispatchReconcileCommand) (outbound.CampaignDispatchSummary, error) {
	if ctx == nil || service == nil || !validCampaignDispatchReconcileCommand(command) {
		return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchInvalid
	}
	var summary outbound.CampaignDispatchSummary
	err := service.uow.Within(ctx, func(tx context.Context) error {
		handoffID, err := service.repo.LockCampaignHandoffForDispatch(tx, command.CampaignCode, command.PlanID)
		if err != nil {
			return err
		}
		binding, err := service.repo.LoadCampaignDispatchByEffect(tx, command.EffectID)
		if err != nil || binding.HandoffID != handoffID {
			return outbound.ErrCampaignHandoffNotFound
		}
		evidence := outboundport.CampaignDispatchProviderAttemptReceipt{Completion: string(eer.StateReconciled)}
		reconcileEvidenceDigest := eer.Digest(command.EvidenceDigest)
		if audienceEvidence, audience, evidenceErr := service.audienceReconciliationEvidence(tx, command.EffectID); evidenceErr != nil {
			return evidenceErr
		} else if audience {
			if service.evidence == nil {
				return outbound.ErrCampaignDispatchUnavailable
			}
			deliveryProven, evidenceDigest, verifyErr := service.evidence.VerifyAudienceCampaignDispatch(tx, audienceEvidence)
			if verifyErr != nil || !outbound.ValidCampaignDispatchDigest(string(evidenceDigest)) {
				return errors.Join(outbound.ErrCampaignDispatchUnavailable, verifyErr)
			}
			evidence.DeliveryProven, evidence.ReconciliationEvidenceDigest = deliveryProven, evidenceDigest
			evidence.ProviderMessageID, evidence.ProviderResultReceived = audienceEvidence.ProviderMessageID, true
			evidence.BusinessCallDispatched = audienceEvidence.BusinessCallDispatched
			evidence.RealExternalCallExecuted = audienceEvidence.RealExternalCallExecuted
			reconcileEvidenceDigest = evidenceDigest
		}
		if evidence.ReconciliationEvidenceDigest == "" {
			evidence.ReconciliationEvidenceDigest = eer.Digest(command.EvidenceDigest)
		}
		projection, receipt, err := service.runtime.Reconcile(tx, eer.ReconcileCommand{
			Lease:            eer.Lease{EffectID: command.EffectID, Generation: command.Generation, Fence: command.Fence, ExpiresAt: command.LeaseExpiresAt},
			ReceiptKeyDigest: digest("manual-reconcile", strconv.FormatInt(command.ActorID, 10), command.IdempotencyKey), EvidenceDigest: reconcileEvidenceDigest,
		})
		if err != nil {
			return err
		}
		if projection.State != eer.StateReconciled {
			return outbound.ErrCampaignDispatchUnavailable
		}
		if err = service.repo.UpdateCampaignDispatchState(tx, command.EffectID, outbound.CampaignDispatchReconciled); err != nil {
			return err
		}
		evidence.ReceiptDigest = receipt.CommandDigest
		if err = service.repo.RecordCampaignProviderAttemptReceipt(tx, command.EffectID, projection.AttemptCount, evidence); err != nil {
			return err
		}
		summary, err = service.repo.ReadCampaignDispatchSummary(tx, handoffID)
		return err
	})
	if err != nil {
		return outbound.CampaignDispatchSummary{}, campaignDispatchError(err)
	}
	if !outbound.ValidCampaignDispatchSummary(summary) {
		return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchUnavailable
	}
	return summary, nil
}

func (service *CampaignDispatchService) audiencePackage(ctx context.Context, handoffID int64) (int64, bool, error) {
	reader, ok := service.repo.(outboundport.AudienceCampaignDispatchSourceReader)
	if !ok {
		return 0, false, nil
	}
	packageID, audience, err := reader.AudiencePackageForCampaignHandoff(ctx, handoffID)
	if err != nil || (audience && packageID < 1) {
		return 0, false, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	return packageID, audience, nil
}

func (service *CampaignDispatchService) audienceQualification(ctx context.Context, packageID int64, candidates []outboundport.CampaignDispatchCandidate) (map[int64]outboundport.AudienceDispatchTargetQualification, error) {
	if service == nil || service.audience == nil || packageID < 1 || len(candidates) == 0 {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	ids := make([]int64, 0, len(candidates))
	seen := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.CustomerID < 1 {
			return nil, outbound.ErrCampaignDispatchUnavailable
		}
		if _, duplicate := seen[candidate.CustomerID]; !duplicate {
			seen[candidate.CustomerID] = struct{}{}
			ids = append(ids, candidate.CustomerID)
		}
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	values, err := service.audience.QualifyAudienceDispatchTargets(ctx, packageID, ids)
	if err != nil || len(values) != len(ids) {
		return nil, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	result := make(map[int64]outboundport.AudienceDispatchTargetQualification, len(values))
	for index, value := range values {
		if value.CustomerID != ids[index] || result[value.CustomerID].CustomerID != 0 {
			return nil, outbound.ErrCampaignDispatchUnavailable
		}
		if value.Eligible {
			if !validAudienceTargetText(value.SenderUserID, 128) || !validAudienceTargetText(value.ExternalUserID, 1024) || value.Exclusion != "" {
				return nil, outbound.ErrCampaignDispatchUnavailable
			}
		} else if value.SenderUserID != "" || value.ExternalUserID != "" || (value.Exclusion != "sender_not_allowed" && value.Exclusion != "target_unresolved") {
			return nil, outbound.ErrCampaignDispatchUnavailable
		}
		result[value.CustomerID] = value
	}
	return result, nil
}

func (service *CampaignDispatchService) audienceReconciliationEvidence(ctx context.Context, effectID string) (outboundport.CampaignDispatchReconciliationEvidence, bool, error) {
	reader, ok := service.repo.(outboundport.CampaignDispatchReconciliationEvidenceReader)
	if !ok {
		return outboundport.CampaignDispatchReconciliationEvidence{}, false, nil
	}
	return reader.LoadAudienceCampaignDispatchReconciliationEvidence(ctx, effectID)
}

func validAudienceTargetText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}

func validCampaignDispatchCommand(command CampaignDispatchCommand) bool {
	return outbound.ValidCampaignHandoffIdentity(command.CampaignCode, command.PlanID) && command.ActorID > 0 && len(command.IdempotencyKey) >= 16 && len(command.IdempotencyKey) <= 128 && strings.TrimSpace(command.IdempotencyKey) == command.IdempotencyKey
}
func validCampaignDispatchReconcileCommand(command CampaignDispatchReconcileCommand) bool {
	return outbound.ValidCampaignHandoffIdentity(command.CampaignCode, command.PlanID) && command.EffectID != "" && command.ActorID > 0 && len(command.IdempotencyKey) >= 16 && len(command.IdempotencyKey) <= 128 && strings.TrimSpace(command.IdempotencyKey) == command.IdempotencyKey && command.Generation > 0 && command.Fence > 0 && !command.LeaseExpiresAt.IsZero() && outbound.ValidCampaignDispatchDigest(command.EvidenceDigest)
}
func campaignDispatchState(value eer.State) (outbound.CampaignDispatchState, bool) {
	switch value {
	case eer.StateExecuted:
		return outbound.CampaignDispatchExecuted, true
	case eer.StateOutcomeUnknown:
		return outbound.CampaignDispatchOutcomeUnknown, true
	case eer.StateRetryableFailed:
		return outbound.CampaignDispatchRetryableFailed, true
	case eer.StateFinalFailed:
		return outbound.CampaignDispatchFinalFailed, true
	default:
		return "", false
	}
}
func digest(label string, parts ...string) eer.Digest {
	sum := sha256.Sum256([]byte(label + "\x00" + strings.Join(parts, "\x00")))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
func campaignDispatchError(err error) error {
	if errors.Is(err, outbound.ErrCampaignDispatchInvalid) || errors.Is(err, outbound.ErrCampaignDispatchConflict) {
		return err
	}
	return errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
}
func nilCampaignDispatchDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}
