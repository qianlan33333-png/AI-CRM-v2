// Package externaleffects defines the closed runtime boundary for provider
// side effects. It contains no provider implementation and no persistence
// adapter: those are integrated by the owning application and platform.
package externaleffects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidCommand       = errors.New("invalid external effect command")
	ErrPayloadMismatch      = errors.New("external effect payload mismatch")
	ErrInvalidTransition    = errors.New("invalid external effect transition")
	ErrLeaseExpired         = errors.New("external effect lease expired")
	ErrLeaseFence           = errors.New("external effect lease fence rejected")
	ErrRetryForbidden       = errors.New("external effect retry forbidden")
	ErrCancelForbidden      = errors.New("external effect cancel forbidden")
	ErrReconcileRequired    = errors.New("external effect reconciliation required")
	ErrRecoveryForbidden    = errors.New("external effect attempted recovery forbidden")
	ErrInvalidAdapterResult = errors.New("invalid external effect adapter result")
	ErrAdapterFailure       = errors.New("external effect adapter failure")
	ErrNotFound             = errors.New("external effect not found")
	ErrUnavailable          = errors.New("external effect runtime unavailable")
)

// Owner and Kind are deliberately closed. Callers cannot supply arbitrary
// provider names, job kinds, payload schemas, or routing instructions.
type Owner string

const (
	OwnerCampaign Owner = "campaign"
	OwnerContact  Owner = "contact"
	OwnerOutbound Owner = "outbound"
	OwnerWeCom    Owner = "wecom"
	OwnerSurvey   Owner = "survey"
	OwnerAudience Owner = "audience"
	OwnerOrder    Owner = "order"
)

type Kind string

const (
	KindCampaignDispatch          Kind = "campaign_dispatch"
	KindCampaignGroupAnnouncement Kind = "campaign_group_announcement"
	KindContactTouch              Kind = "contact_touch"
	KindOutboundMessage           Kind = "outbound_message"
	KindOutboundMedia             Kind = "outbound_media"
	KindWeComTagSync              Kind = "wecom_tag_sync"
	KindWeComProfileSync          Kind = "wecom_profile_sync"
	KindSurveyWebhook             Kind = "survey_webhook"
	KindAudienceWebhook           Kind = "audience_webhook"
	KindOrderPaymentCapture       Kind = "order_payment_capture"
	KindOrderRefund               Kind = "order_refund"
)

func validOwnerKind(owner Owner, kind Kind) bool {
	switch owner {
	case OwnerCampaign:
		return kind == KindCampaignDispatch || kind == KindCampaignGroupAnnouncement
	case OwnerContact:
		return kind == KindContactTouch
	case OwnerOutbound:
		return kind == KindOutboundMessage || kind == KindOutboundMedia
	case OwnerWeCom:
		return kind == KindWeComTagSync || kind == KindWeComProfileSync
	case OwnerSurvey:
		return kind == KindSurveyWebhook
	case OwnerAudience:
		return kind == KindAudienceWebhook
	case OwnerOrder:
		return kind == KindOrderPaymentCapture || kind == KindOrderRefund
	default:
		return false
	}
}

type State string

const (
	StateAccepted        State = "accepted"
	StateQueued          State = "queued"
	StateAttempted       State = "attempted"
	StateExecuted        State = "executed"
	StateOutcomeUnknown  State = "outcome_unknown"
	StateReconciled      State = "reconciled"
	StateRetryableFailed State = "retryable_failed"
	StateFinalFailed     State = "final_failed"
	StateCancelled       State = "cancelled"
)

func validState(state State) bool {
	switch state {
	case StateAccepted, StateQueued, StateAttempted, StateExecuted, StateOutcomeUnknown,
		StateReconciled, StateRetryableFailed, StateFinalFailed, StateCancelled:
		return true
	default:
		return false
	}
}

// CanTransition is the complete runtime state machine. Store adapters enforce
// it atomically with their generation/fence compare-and-swap.
func CanTransition(from, to State) bool {
	switch from {
	case StateAccepted:
		return to == StateQueued || to == StateCancelled
	case StateQueued:
		return to == StateAttempted || to == StateCancelled
	case StateAttempted:
		return to == StateExecuted || to == StateRetryableFailed || to == StateFinalFailed || to == StateOutcomeUnknown
	case StateRetryableFailed:
		return to == StateQueued
	case StateOutcomeUnknown:
		return to == StateReconciled
	default:
		return false
	}
}

// Digest must be a SHA-256 digest, not an identifier, body, secret, or
// provider receipt. This makes the package boundary unsuitable for PII.
type Digest string

func validDigest(value Digest) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(string(value), prefix) || len(value) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(string(value[len(prefix):]))
	return err == nil
}

// EnvelopeInput holds only opaque digests. Its immutable result has no
// exported fields, so no caller can mutate accepted execution facts in place.
type EnvelopeInput struct {
	Owner             Owner
	Kind              Kind
	SourceRefDigest   Digest
	TargetRefDigest   Digest
	PayloadDigest     Digest
	PolicyVersionHash Digest
}

type EffectEnvelope struct {
	owner             Owner
	kind              Kind
	sourceRefDigest   Digest
	targetRefDigest   Digest
	payloadDigest     Digest
	policyVersionHash Digest
}

func NewEnvelope(input EnvelopeInput) (EffectEnvelope, error) {
	if !validOwnerKind(input.Owner, input.Kind) || !validDigest(input.SourceRefDigest) ||
		!validDigest(input.TargetRefDigest) || !validDigest(input.PayloadDigest) ||
		!validDigest(input.PolicyVersionHash) {
		return EffectEnvelope{}, ErrInvalidCommand
	}
	return EffectEnvelope{
		owner: input.Owner, kind: input.Kind, sourceRefDigest: input.SourceRefDigest,
		targetRefDigest: input.TargetRefDigest, payloadDigest: input.PayloadDigest,
		policyVersionHash: input.PolicyVersionHash,
	}, nil
}

func (envelope EffectEnvelope) Owner() Owner              { return envelope.owner }
func (envelope EffectEnvelope) Kind() Kind                { return envelope.kind }
func (envelope EffectEnvelope) SourceRefDigest() Digest   { return envelope.sourceRefDigest }
func (envelope EffectEnvelope) TargetRefDigest() Digest   { return envelope.targetRefDigest }
func (envelope EffectEnvelope) PayloadDigest() Digest     { return envelope.payloadDigest }
func (envelope EffectEnvelope) PolicyVersionHash() Digest { return envelope.policyVersionHash }

func (envelope EffectEnvelope) valid() bool {
	return validOwnerKind(envelope.owner, envelope.kind) && validDigest(envelope.sourceRefDigest) &&
		validDigest(envelope.targetRefDigest) && validDigest(envelope.payloadDigest) && validDigest(envelope.policyVersionHash)
}

// Fingerprint is safe to persist and compare for exact idempotency replay.
func (envelope EffectEnvelope) Fingerprint() Digest {
	if !envelope.valid() {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		string(envelope.owner), string(envelope.kind), string(envelope.sourceRefDigest),
		string(envelope.targetRefDigest), string(envelope.payloadDigest), string(envelope.policyVersionHash),
	}, "\x00")))
	return Digest("sha256:" + hex.EncodeToString(sum[:]))
}

type RiverJobLink struct {
	JobID       int64
	Generation  int64
	Queue       string
	ArgsDigest  Digest
	ScheduledAt time.Time
}

func (link RiverJobLink) valid() bool {
	return link.JobID > 0 && link.Generation > 0 && link.Queue != "" && strings.TrimSpace(link.Queue) == link.Queue &&
		validDigest(link.ArgsDigest) && !link.ScheduledAt.IsZero()
}

func (link RiverJobLink) fingerprint() Digest {
	if !link.valid() {
		return ""
	}
	return digestParts("river-job", strconv.FormatInt(link.JobID, 10), strconv.FormatInt(link.Generation, 10), link.Queue,
		string(link.ArgsDigest), link.ScheduledAt.UTC().Format(time.RFC3339Nano))
}

type Lease struct {
	EffectID   string
	Generation int64
	Fence      int64
	ExpiresAt  time.Time
}

func (lease Lease) valid(now time.Time) bool {
	return lease.validFields() && lease.ExpiresAt.After(now)
}

func (lease Lease) validFields() bool {
	return lease.EffectID != "" && lease.Generation > 0 && lease.Fence > 0 && !lease.ExpiresAt.IsZero()
}

type Attempt struct {
	Number     int32
	Generation int64
	Fence      int64
	StartedAt  time.Time
}

func (attempt Attempt) validFor(lease Lease) bool {
	return attempt.Number > 0 && attempt.Generation == lease.Generation && attempt.Fence == lease.Fence && !attempt.StartedAt.IsZero()
}

// OperationReceipt is an opaque, safe projection of an idempotent command.
// It intentionally has no caller key, payload, provider body, or error text.
type OperationReceipt struct {
	ID            string
	EffectID      string
	CommandDigest Digest
	State         State
	CompletedAt   time.Time
}

func (receipt OperationReceipt) validFor(effectID string, command Digest) bool {
	return receipt.ID != "" && receipt.EffectID == effectID && receipt.CommandDigest == command &&
		validState(receipt.State) && !receipt.CompletedAt.IsZero()
}

// Projection is safe to expose across the runtime boundary. IDs are opaque;
// it does not expose envelopes, provider requests/responses, recipients, or
// error bodies.
type Projection struct {
	ID           string
	Owner        Owner
	Kind         Kind
	State        State
	AttemptCount int32
	Generation   int64
	UpdatedAt    time.Time
}

// Diagnostics is the closed aggregate exposed to operators. It carries no
// provider body, recipient, credential, or adapter error material.
type Diagnostics struct {
	Accepted, Queued, Attempted, OutcomeUnknown, RetryableFailed int64
}

func (projection Projection) valid() bool {
	return projection.ID != "" && validOwnerKind(projection.Owner, projection.Kind) && validState(projection.State) &&
		projection.AttemptCount >= 0 && projection.Generation > 0 && !projection.UpdatedAt.IsZero()
}

type AcceptCommand struct {
	ReceiptKeyDigest Digest
	Envelope         EffectEnvelope
}

func (command AcceptCommand) digest() Digest {
	if !validDigest(command.ReceiptKeyDigest) || !command.Envelope.valid() {
		return ""
	}
	return digestParts("accept", string(command.ReceiptKeyDigest), string(command.Envelope.Fingerprint()))
}

func (command AcceptCommand) valid() bool { return command.digest() != "" }

func (command AcceptCommand) CommandDigest() Digest { return command.digest() }

type QueueCommand struct {
	EffectID         string
	Job              RiverJobLink
	ReceiptKeyDigest Digest
}

func (command QueueCommand) digest() Digest {
	if command.EffectID == "" || !command.Job.valid() || !validDigest(command.ReceiptKeyDigest) {
		return ""
	}
	return digestParts("queue", command.EffectID, string(command.Job.fingerprint()), string(command.ReceiptKeyDigest))
}

func (command QueueCommand) CommandDigest() Digest { return command.digest() }

type ClaimCommand struct {
	EffectID     string
	WorkerDigest Digest
}

type RetryCommand struct {
	EffectID         string
	Job              RiverJobLink
	ReceiptKeyDigest Digest
}

func (command RetryCommand) digest() Digest {
	if command.EffectID == "" || !command.Job.valid() || !validDigest(command.ReceiptKeyDigest) {
		return ""
	}
	return digestParts("retry", command.EffectID, string(command.Job.fingerprint()), string(command.ReceiptKeyDigest))
}

func (command RetryCommand) CommandDigest() Digest { return command.digest() }

type CancelCommand struct {
	EffectID         string
	ReceiptKeyDigest Digest
}

func (command CancelCommand) digest() Digest {
	if command.EffectID == "" || !validDigest(command.ReceiptKeyDigest) {
		return ""
	}
	return digestParts("cancel", command.EffectID, string(command.ReceiptKeyDigest))
}

func (command CancelCommand) CommandDigest() Digest { return command.digest() }

type ReconcileCommand struct {
	Lease            Lease
	ReceiptKeyDigest Digest
	EvidenceDigest   Digest
}

func (command ReconcileCommand) digest() Digest {
	if !command.Lease.validFields() || !validDigest(command.ReceiptKeyDigest) || !validDigest(command.EvidenceDigest) {
		return ""
	}
	return digestParts("reconcile", command.Lease.EffectID, strconv.FormatInt(command.Lease.Generation, 10),
		strconv.FormatInt(command.Lease.Fence, 10), command.Lease.ExpiresAt.UTC().Format(time.RFC3339Nano),
		string(command.ReceiptKeyDigest), string(command.EvidenceDigest))
}

func (command ReconcileCommand) CommandDigest() Digest { return command.digest() }

type RecoverAttemptedCommand struct {
	Lease Lease
}

func (command RecoverAttemptedCommand) digest() Digest {
	if !command.Lease.validFields() {
		return ""
	}
	return digestParts("recover-attempted", command.Lease.EffectID, strconv.FormatInt(command.Lease.Generation, 10),
		strconv.FormatInt(command.Lease.Fence, 10), command.Lease.ExpiresAt.UTC().Format(time.RFC3339Nano))
}

func (command RecoverAttemptedCommand) CommandDigest() Digest { return command.digest() }

type Completion string

const (
	CompletionExecuted        Completion = "executed"
	CompletionRetryableFailed Completion = "retryable_failed"
	CompletionFinalFailed     Completion = "final_failed"
	CompletionOutcomeUnknown  Completion = "outcome_unknown"
)

func (completion Completion) state() (State, bool) {
	switch completion {
	case CompletionExecuted:
		return StateExecuted, true
	case CompletionRetryableFailed:
		return StateRetryableFailed, true
	case CompletionFinalFailed:
		return StateFinalFailed, true
	case CompletionOutcomeUnknown:
		return StateOutcomeUnknown, true
	default:
		return "", false
	}
}

type AdapterResult struct {
	Completion    Completion
	ReceiptDigest Digest
}

func (result AdapterResult) valid() bool {
	_, ok := result.Completion.state()
	return ok && validDigest(result.ReceiptDigest)
}

// Adapter is a domain-owned boundary. Its Execute call is deliberately made
// after PersistAttempt returns, so provider I/O cannot be inside that write.
type Adapter interface {
	Execute(context.Context, EffectEnvelope, Attempt) (AdapterResult, error)
}

// Store methods each represent a separate short persistence transaction. Queue
// and Retry must atomically persist StateQueued, the full RiverJobLink, and the
// returned operation receipt in one UoW. The implementing adapter must enforce
// CanTransition and CAS on generation/fence, returning ErrLeaseFence or
// ErrLeaseExpired when that CAS does not match.
type Store interface {
	Accept(context.Context, AcceptCommand) (Projection, OperationReceipt, error)
	Queue(context.Context, QueueCommand) (Projection, OperationReceipt, error)
	Claim(context.Context, ClaimCommand) (Lease, Projection, error)
	Retry(context.Context, RetryCommand) (Projection, OperationReceipt, error)
	Cancel(context.Context, CancelCommand) (Projection, OperationReceipt, error)
	PersistAttempt(context.Context, Lease) (EffectEnvelope, Attempt, Projection, error)
	CompleteAttempt(context.Context, Lease, Attempt, AdapterResult) (Projection, OperationReceipt, error)
	Reconcile(context.Context, ReconcileCommand) (Projection, OperationReceipt, error)
	RecoverAttemptedToUnknown(context.Context, RecoverAttemptedCommand) (Projection, OperationReceipt, error)
}

type Service struct {
	store Store
	clock func() time.Time
}

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidCommand
	}
	return &Service{store: store, clock: time.Now}, nil
}

func (service *Service) Accept(ctx context.Context, command AcceptCommand) (Projection, OperationReceipt, error) {
	if service == nil || service.store == nil || ctx == nil || !command.valid() {
		return Projection{}, OperationReceipt{}, ErrInvalidCommand
	}
	projection, receipt, err := service.store.Accept(ctx, command)
	if err != nil {
		return Projection{}, OperationReceipt{}, err
	}
	if !projection.valid() || projection.State != StateAccepted || !receipt.validFor(projection.ID, command.digest()) {
		return Projection{}, OperationReceipt{}, ErrInvalidCommand
	}
	return projection, receipt, nil
}

func (service *Service) Queue(ctx context.Context, command QueueCommand) (Projection, OperationReceipt, error) {
	if service == nil || service.store == nil || ctx == nil || command.digest() == "" {
		return Projection{}, OperationReceipt{}, ErrInvalidCommand
	}
	projection, receipt, err := service.store.Queue(ctx, command)
	if err != nil {
		return Projection{}, OperationReceipt{}, err
	}
	if !projection.valid() || projection.ID != command.EffectID || projection.State != StateQueued ||
		!receipt.validFor(command.EffectID, command.digest()) {
		return Projection{}, OperationReceipt{}, ErrInvalidTransition
	}
	return projection, receipt, nil
}

// Claim reserves a live generation/fence for one queued effect. It does not
// start provider I/O; RunAttempt persists StateAttempted before that boundary.
func (service *Service) Claim(ctx context.Context, command ClaimCommand) (Lease, Projection, error) {
	if service == nil || service.store == nil || service.clock == nil || ctx == nil || command.EffectID == "" || !validDigest(command.WorkerDigest) {
		return Lease{}, Projection{}, ErrInvalidCommand
	}
	lease, projection, err := service.store.Claim(ctx, command)
	if err != nil {
		return Lease{}, Projection{}, err
	}
	if !lease.valid(service.clock()) || lease.EffectID != command.EffectID || !projection.valid() ||
		projection.ID != command.EffectID || projection.State != StateQueued || projection.Generation != lease.Generation {
		return Lease{}, Projection{}, ErrLeaseFence
	}
	return lease, projection, nil
}

func (service *Service) Retry(ctx context.Context, command RetryCommand) (Projection, OperationReceipt, error) {
	if service == nil || service.store == nil || ctx == nil || command.digest() == "" {
		return Projection{}, OperationReceipt{}, ErrInvalidCommand
	}
	projection, receipt, err := service.store.Retry(ctx, command)
	if err != nil {
		return Projection{}, OperationReceipt{}, err
	}
	if !projection.valid() || projection.ID != command.EffectID || projection.State != StateQueued ||
		!receipt.validFor(command.EffectID, command.digest()) {
		return Projection{}, OperationReceipt{}, ErrRetryForbidden
	}
	return projection, receipt, nil
}

func (service *Service) Cancel(ctx context.Context, command CancelCommand) (Projection, OperationReceipt, error) {
	if service == nil || service.store == nil || ctx == nil || command.digest() == "" {
		return Projection{}, OperationReceipt{}, ErrInvalidCommand
	}
	projection, receipt, err := service.store.Cancel(ctx, command)
	if err != nil {
		return Projection{}, OperationReceipt{}, err
	}
	if !projection.valid() || projection.ID != command.EffectID || projection.State != StateCancelled ||
		!receipt.validFor(command.EffectID, command.digest()) {
		return Projection{}, OperationReceipt{}, ErrCancelForbidden
	}
	return projection, receipt, nil
}

// RunAttempt has two persistence phases around adapter I/O:
// PersistAttempt (attempted plus fence) -> Adapter.Execute -> CompleteAttempt.
// A transport error is always unknown, never an automatic retry.
func (service *Service) RunAttempt(ctx context.Context, lease Lease, adapter Adapter) (Projection, OperationReceipt, error) {
	if service == nil || service.store == nil || service.clock == nil || ctx == nil || adapter == nil || !lease.validFields() {
		return Projection{}, OperationReceipt{}, ErrInvalidCommand
	}
	if !lease.ExpiresAt.After(service.clock()) {
		return Projection{}, OperationReceipt{}, ErrLeaseExpired
	}
	envelope, attempt, attempted, err := service.store.PersistAttempt(ctx, lease)
	if err != nil {
		return Projection{}, OperationReceipt{}, err
	}
	if !envelope.valid() || !attempt.validFor(lease) || !attempted.valid() || attempted.ID != lease.EffectID || attempted.State != StateAttempted {
		return Projection{}, OperationReceipt{}, ErrLeaseFence
	}

	result, adapterErr := adapter.Execute(ctx, envelope, attempt)
	invalidResult := !result.valid()
	if adapterErr != nil || invalidResult {
		result = AdapterResult{Completion: CompletionOutcomeUnknown, ReceiptDigest: unknownReceiptDigest(lease, attempt)}
	}
	projection, receipt, completionErr := service.store.CompleteAttempt(ctx, lease, attempt, result)
	if completionErr != nil {
		return Projection{}, OperationReceipt{}, completionErr
	}
	state, _ := result.Completion.state()
	if !projection.valid() || projection.ID != lease.EffectID || projection.State != state ||
		!receipt.validFor(lease.EffectID, result.ReceiptDigest) {
		return Projection{}, OperationReceipt{}, ErrInvalidTransition
	}
	if adapterErr != nil {
		return projection, receipt, ErrAdapterFailure
	}
	if invalidResult {
		return projection, receipt, ErrInvalidAdapterResult
	}
	return projection, receipt, nil
}

func (service *Service) Reconcile(ctx context.Context, command ReconcileCommand) (Projection, OperationReceipt, error) {
	if service == nil || service.store == nil || ctx == nil ||
		command.digest() == "" {
		return Projection{}, OperationReceipt{}, ErrInvalidCommand
	}
	projection, receipt, err := service.store.Reconcile(ctx, command)
	if err != nil {
		return Projection{}, OperationReceipt{}, err
	}
	if !projection.valid() || projection.ID != command.Lease.EffectID || projection.State != StateReconciled ||
		!receipt.validFor(command.Lease.EffectID, command.digest()) {
		return Projection{}, OperationReceipt{}, ErrReconcileRequired
	}
	return projection, receipt, nil
}

// RecoverAttemptedToUnknown is crash recovery only. It cannot call an Adapter;
// the Store permits it only when the persisted attempted lease has expired and
// generation/fence still match, then writes a synthetic unknown receipt.
func (service *Service) RecoverAttemptedToUnknown(ctx context.Context, command RecoverAttemptedCommand) (Projection, OperationReceipt, error) {
	if service == nil || service.store == nil || service.clock == nil || ctx == nil || command.digest() == "" {
		return Projection{}, OperationReceipt{}, ErrInvalidCommand
	}
	if command.Lease.ExpiresAt.After(service.clock()) {
		return Projection{}, OperationReceipt{}, ErrRecoveryForbidden
	}
	projection, receipt, err := service.store.RecoverAttemptedToUnknown(ctx, command)
	if err != nil {
		return Projection{}, OperationReceipt{}, err
	}
	if !projection.valid() || projection.ID != command.Lease.EffectID || projection.State != StateOutcomeUnknown ||
		!receipt.validFor(command.Lease.EffectID, command.digest()) {
		return Projection{}, OperationReceipt{}, ErrRecoveryForbidden
	}
	return projection, receipt, nil
}

func unknownReceiptDigest(lease Lease, attempt Attempt) Digest {
	sum := sha256.Sum256([]byte(fmt.Sprintf("unknown\x00%s\x00%d\x00%d\x00%d", lease.EffectID, lease.Generation, lease.Fence, attempt.Number)))
	return Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func digestParts(label string, parts ...string) Digest {
	sum := sha256.Sum256([]byte(label + "\x00" + strings.Join(parts, "\x00")))
	return Digest("sha256:" + hex.EncodeToString(sum[:]))
}
