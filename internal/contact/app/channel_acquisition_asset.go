package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	ChannelAcquisitionAssetJobKind                  = "contact_acquisition_asset_publish"
	channelAcquisitionAssetPolicy                   = "ch02-contact-acquisition-asset-v1"
	channelAcquisitionAssetStateAttempted eer.State = "attempted"
)

var (
	ErrInvalidChannelAcquisitionAsset           = errors.New("invalid channel acquisition asset command")
	ErrChannelAcquisitionAssetConflict          = errors.New("channel acquisition asset command conflict")
	ErrChannelAcquisitionAssetNotFound          = errors.New("channel acquisition asset not found")
	ErrChannelAcquisitionAssetUnavailable       = errors.New("channel acquisition asset unavailable")
	ErrChannelAcquisitionAssetReconcileRequired = errors.New("channel acquisition asset reconciliation required")
)

type PublishChannelAcquisitionAssetCommand struct {
	ChannelID      int64
	Actor          int64
	IdempotencyKey string
	Kind           contactport.AcquisitionAssetKind
}

type ChannelAcquisitionAssetReconcileResolution string

const (
	ChannelAcquisitionAssetProviderApplied    ChannelAcquisitionAssetReconcileResolution = "provider_applied"
	ChannelAcquisitionAssetProviderNotApplied ChannelAcquisitionAssetReconcileResolution = "provider_not_applied"
)

type ReconcileChannelAcquisitionAssetCommand struct {
	EffectID       string
	Actor          int64
	IdempotencyKey string
	Generation     int64
	Fence          int64
	LeaseExpiresAt time.Time
	EvidenceDigest eer.Digest
	Resolution     ChannelAcquisitionAssetReconcileResolution
}

type ReconcileCurrentChannelAcquisitionAssetCommand struct {
	EffectID       string
	ChannelID      int64
	Actor          int64
	IdempotencyKey string
	EvidenceDigest eer.Digest
	Resolution     ChannelAcquisitionAssetReconcileResolution
}

type ChannelAcquisitionAssetAcceptance struct {
	EffectID                 string
	ChannelID                int64
	Kind                     contactport.AcquisitionAssetKind
	AssetVersion             int64
	SupersedesVersion        int64
	State                    eer.State
	RiverJobID               int64
	AcceptReceiptID          string
	QueueReceiptID           string
	EntrantReady             bool
	RealExternalCallExecuted bool
}

type ChannelAcquisitionAssetExecution struct {
	EffectID                 string
	ChannelID                int64
	Kind                     contactport.AcquisitionAssetKind
	AssetVersion             int64
	State                    eer.State
	AttemptReceiptDigest     eer.Digest
	ProviderCallAttempted    bool
	ManualReconcileRequired  bool
	EntrantReady             bool
	RealExternalCallExecuted bool
}

type ChannelAcquisitionAssetReconciliation struct {
	EffectID               string
	State                  eer.State
	Resolution             ChannelAcquisitionAssetReconcileResolution
	ReceiptID              string
	Replacement            *ChannelAcquisitionAssetAcceptance
	ProviderSuccessClaimed bool
	EntrantReady           bool
}

type ChannelAcquisitionAssetJobArgs struct {
	EffectID string `json:"effect_id"`
}

func (ChannelAcquisitionAssetJobArgs) Kind() string { return ChannelAcquisitionAssetJobKind }

// ChannelAcquisitionAssetEffectSpec is fixed to the CH02 effect by its runtime
// interface. The central adapter will add the closed EER owner/kind; this leaf
// does not weaken the shared EER registry to accept arbitrary strings.
type ChannelAcquisitionAssetEffectSpec struct {
	SourceRefDigest   eer.Digest
	TargetRefDigest   eer.Digest
	PayloadDigest     eer.Digest
	PolicyVersionHash eer.Digest
}

func (spec ChannelAcquisitionAssetEffectSpec) Fingerprint() eer.Digest {
	if !validChannelAcquisitionAssetDigest(spec.SourceRefDigest) || !validChannelAcquisitionAssetDigest(spec.TargetRefDigest) ||
		!validChannelAcquisitionAssetDigest(spec.PayloadDigest) || !validChannelAcquisitionAssetDigest(spec.PolicyVersionHash) {
		return ""
	}
	return channelAcquisitionAssetDigest("effect-spec", string(spec.SourceRefDigest), string(spec.TargetRefDigest), string(spec.PayloadDigest), string(spec.PolicyVersionHash))
}

type ChannelAcquisitionAssetEffectAcceptCommand struct {
	ReceiptKeyDigest eer.Digest
	Spec             ChannelAcquisitionAssetEffectSpec
}

type ChannelAcquisitionAssetEffectQueueCommand struct {
	EffectID         string
	Job              eer.RiverJobLink
	ReceiptKeyDigest eer.Digest
}

type ChannelAcquisitionAssetEffectClaimCommand struct {
	EffectID     string
	WorkerDigest eer.Digest
}

type ChannelAcquisitionAssetEffectReconcileCommand struct {
	Lease            eer.Lease
	ReceiptKeyDigest eer.Digest
	EvidenceDigest   eer.Digest
}

type ChannelAcquisitionAssetEffectProjection struct {
	ID                  string
	State               eer.State
	Generation          int64
	UpdatedAt           time.Time
	EnvelopeFingerprint eer.Digest
}

type ChannelAcquisitionAssetEffectTerminal struct {
	EffectID                 string
	State                    eer.State
	Receipt                  eer.OperationReceipt
	Lease                    eer.Lease
	ResultReferenceDigest    eer.Digest
	BusinessCallDispatched   bool
	RealExternalCallExecuted bool
}

type ChannelAcquisitionAssetEffectRuntime interface {
	Accept(context.Context, ChannelAcquisitionAssetEffectAcceptCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error)
	Queue(context.Context, ChannelAcquisitionAssetEffectQueueCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error)
	Claim(context.Context, ChannelAcquisitionAssetEffectClaimCommand) (eer.Lease, ChannelAcquisitionAssetEffectProjection, error)
	RunAttempt(context.Context, eer.Lease, func(context.Context) (eer.AdapterResult, error)) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error)
	Reconcile(context.Context, ChannelAcquisitionAssetEffectReconcileCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error)
	RecoverAttempted(context.Context, eer.Lease) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error)
	Terminal(context.Context, string) (ChannelAcquisitionAssetEffectTerminal, bool, error)
}

type ChannelAcquisitionAssetJobInserter interface {
	Insert(context.Context, ChannelAcquisitionAssetJobArgs, int64, time.Time) (eer.RiverJobLink, error)
}

type ChannelAcquisitionAssetReceiptState string

const (
	ChannelAcquisitionAssetReceiptInProgress ChannelAcquisitionAssetReceiptState = "in_progress"
	ChannelAcquisitionAssetReceiptCompleted  ChannelAcquisitionAssetReceiptState = "completed"
)

type ChannelAcquisitionAssetOperation string

const (
	ChannelAcquisitionAssetOperationPublish   ChannelAcquisitionAssetOperation = "publish"
	ChannelAcquisitionAssetOperationReconcile ChannelAcquisitionAssetOperation = "reconcile"
)

type ChannelAcquisitionAssetActorReceipt struct {
	ID                  int64
	Operation           ChannelAcquisitionAssetOperation
	Actor               int64
	KeyDigest           eer.Digest
	PayloadDigest       eer.Digest
	State               ChannelAcquisitionAssetReceiptState
	ResultEffectID      string
	ReplacementEffectID string
	CreatedAt           time.Time
	CompletedAt         time.Time
}

type ChannelAcquisitionAssetBinding struct {
	EffectID                     string
	CorpID                       string
	CorrelationKey               string
	ChannelID                    int64
	Kind                         contactport.AcquisitionAssetKind
	AssetVersion                 int64
	SupersedesVersion            int64
	Snapshot                     contactport.AcquisitionAssetSnapshot
	SnapshotDigest               [32]byte
	IdempotencyDigest            eer.Digest
	EnvelopeFingerprint          eer.Digest
	State                        eer.State
	AcceptReceiptID              string
	AcceptReceiptDigest          eer.Digest
	QueueReceiptID               string
	QueueReceiptDigest           eer.Digest
	RiverJobID                   int64
	Generation                   int64
	Fence                        int64
	LeaseExpiresAt               time.Time
	AttemptReceiptID             string
	AttemptReceiptDigest         eer.Digest
	ProviderAssetReferenceDigest [32]byte
	ProviderCallAttempted        bool
	RealExternalCallExecuted     bool
	ReconcileReceiptID           string
	ReconcileReceiptDigest       eer.Digest
	ReconcileEvidenceDigest      eer.Digest
	ReconcileResolution          ChannelAcquisitionAssetReconcileResolution
	ReconciledAt                 time.Time
	EntrantReady                 bool
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

type ChannelAcquisitionAssetAttemptCompletion struct {
	EffectID                     string
	Lease                        eer.Lease
	State                        eer.State
	Receipt                      eer.OperationReceipt
	ProviderAssetReferenceDigest [32]byte
	ProviderCallAttempted        bool
	RealExternalCallExecuted     bool
	CompletedAt                  time.Time
}

type ChannelAcquisitionAssetReconcileCompletion struct {
	EffectID       string
	Lease          eer.Lease
	Receipt        eer.OperationReceipt
	EvidenceDigest eer.Digest
	Resolution     ChannelAcquisitionAssetReconcileResolution
	CompletedAt    time.Time
}

type ChannelAcquisitionAssetStore interface {
	LockSnapshot(context.Context, int64) (contactport.AcquisitionAssetSnapshot, error)
	ReserveActorReceipt(context.Context, ChannelAcquisitionAssetActorReceipt) (ChannelAcquisitionAssetActorReceipt, bool, error)
	CompleteActorReceipt(context.Context, int64, string, string, time.Time) (ChannelAcquisitionAssetActorReceipt, error)
	NextAssetVersion(context.Context, int64, int64, contactport.AcquisitionAssetKind) (int64, error)
	InsertAccepted(context.Context, ChannelAcquisitionAssetBinding) (ChannelAcquisitionAssetBinding, error)
	MarkQueued(context.Context, string, eer.RiverJobLink, eer.OperationReceipt, time.Time) (ChannelAcquisitionAssetBinding, error)
	LockBinding(context.Context, string) (ChannelAcquisitionAssetBinding, error)
	MarkAttempted(context.Context, string, eer.Lease, time.Time) (ChannelAcquisitionAssetBinding, error)
	CompleteAttempt(context.Context, ChannelAcquisitionAssetAttemptCompletion) (ChannelAcquisitionAssetBinding, error)
	CompleteReconcile(context.Context, ChannelAcquisitionAssetReconcileCompletion) (ChannelAcquisitionAssetBinding, error)
}

type channelAcquisitionAssetScopedLockStore interface {
	LockBindingForChannel(context.Context, int64, string) (ChannelAcquisitionAssetBinding, error)
}

type ChannelAcquisitionAssetService struct {
	uow      platformport.UnitOfWork
	store    ChannelAcquisitionAssetStore
	effects  ChannelAcquisitionAssetEffectRuntime
	jobs     ChannelAcquisitionAssetJobInserter
	provider contactport.AcquisitionAssetProvider
	now      func() time.Time
	corpID   string
}

func NewChannelAcquisitionAssetService(uow platformport.UnitOfWork, store ChannelAcquisitionAssetStore, effects ChannelAcquisitionAssetEffectRuntime, jobs ChannelAcquisitionAssetJobInserter, provider contactport.AcquisitionAssetProvider, corpID string) (*ChannelAcquisitionAssetService, error) {
	service, err := NewChannelAcquisitionAssetCommandService(uow, store, effects, jobs, corpID)
	if err != nil || channelAcquisitionAssetNil(provider) {
		return nil, ErrChannelAcquisitionAssetUnavailable
	}
	service.provider = provider
	return service, nil
}

// NewChannelAcquisitionAssetCommandService constructs the API-side command
// service without a Provider credential or HTTP client. Execute remains closed.
func NewChannelAcquisitionAssetCommandService(uow platformport.UnitOfWork, store ChannelAcquisitionAssetStore, effects ChannelAcquisitionAssetEffectRuntime, jobs ChannelAcquisitionAssetJobInserter, corpID string) (*ChannelAcquisitionAssetService, error) {
	if channelAcquisitionAssetNil(uow) || channelAcquisitionAssetNil(store) || channelAcquisitionAssetNil(effects) || channelAcquisitionAssetNil(jobs) || !validChannelAcquisitionAssetText(corpID, 1, 128) {
		return nil, ErrChannelAcquisitionAssetUnavailable
	}
	return &ChannelAcquisitionAssetService{uow: uow, store: store, effects: effects, jobs: jobs, now: time.Now, corpID: corpID}, nil
}

// Publish freezes the Contact-owned channel snapshot, actor receipt, typed
// binding, EER acceptance, and River link in one caller UnitOfWork.
func (service *ChannelAcquisitionAssetService) Publish(ctx context.Context, command PublishChannelAcquisitionAssetCommand) (ChannelAcquisitionAssetAcceptance, error) {
	if !service.commandReady(ctx) || !validPublishChannelAcquisitionAssetCommand(command) {
		return ChannelAcquisitionAssetAcceptance{}, ErrInvalidChannelAcquisitionAsset
	}
	var result ChannelAcquisitionAssetAcceptance
	err := service.uow.Within(ctx, func(tx context.Context) error {
		snapshot, err := service.store.LockSnapshot(tx, command.ChannelID)
		if err != nil {
			return err
		}
		snapshot, snapshotDigest, ok := canonicalChannelAcquisitionAssetSnapshot(snapshot, command.Kind)
		if !ok || snapshot.ChannelID != command.ChannelID {
			return ErrInvalidChannelAcquisitionAsset
		}
		now := service.now().UTC()
		candidate := ChannelAcquisitionAssetActorReceipt{
			Operation: ChannelAcquisitionAssetOperationPublish, Actor: command.Actor,
			KeyDigest:     channelAcquisitionAssetDigest("actor-key", strconv.FormatInt(command.Actor, 10), command.IdempotencyKey),
			PayloadDigest: channelAcquisitionAssetDigest("publish-payload", strconv.FormatInt(command.ChannelID, 10), string(command.Kind), hex.EncodeToString(snapshotDigest[:])),
			State:         ChannelAcquisitionAssetReceiptInProgress, CreatedAt: now,
		}
		receipt, owned, err := service.store.ReserveActorReceipt(tx, candidate)
		if err != nil {
			return err
		}
		if !sameChannelAcquisitionAssetReceiptCommand(receipt, candidate) {
			return ErrChannelAcquisitionAssetConflict
		}
		if !owned {
			result, err = service.replayPublish(tx, receipt)
			return err
		}
		binding, err := service.queueNewBinding(tx, command.Actor, command.IdempotencyKey, snapshot, snapshotDigest, command.Kind, 0, now)
		if err != nil {
			return err
		}
		completed, err := service.store.CompleteActorReceipt(tx, receipt.ID, binding.EffectID, "", now)
		if err != nil || !validCompletedChannelAcquisitionAssetReceipt(completed, receipt, binding.EffectID, "") {
			return errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
		}
		result, err = channelAcquisitionAssetAcceptance(binding)
		return err
	})
	if err != nil {
		return ChannelAcquisitionAssetAcceptance{}, classifyChannelAcquisitionAssetError(err)
	}
	return result, nil
}

// Execute is the only Provider-call path. A live attempted lease never calls
// the Provider again. Once that lease expires, the same River delivery closes
// both EER and the Contact projection as outcome_unknown without Provider I/O.
func (service *ChannelAcquisitionAssetService) Execute(ctx context.Context, effectID string, workerDigest eer.Digest) (ChannelAcquisitionAssetExecution, error) {
	if !service.executeReady(ctx) || effectID == "" || !validChannelAcquisitionAssetDigest(workerDigest) {
		return ChannelAcquisitionAssetExecution{}, ErrInvalidChannelAcquisitionAsset
	}
	var binding ChannelAcquisitionAssetBinding
	if err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		binding, err = service.store.LockBinding(tx, effectID)
		return err
	}); err != nil || !validChannelAcquisitionAssetBinding(binding) {
		return ChannelAcquisitionAssetExecution{}, classifyChannelAcquisitionAssetError(err)
	}
	if channelAcquisitionAssetTerminal(binding.State) {
		return channelAcquisitionAssetExecution(binding, false), nil
	}
	if terminal, found, terminalErr := service.effects.Terminal(ctx, effectID); terminalErr != nil {
		return ChannelAcquisitionAssetExecution{}, classifyChannelAcquisitionAssetError(terminalErr)
	} else if found {
		return service.convergeTerminal(ctx, binding, terminal)
	}
	if binding.State == channelAcquisitionAssetStateAttempted {
		if binding.LeaseExpiresAt.After(service.now().UTC()) {
			return channelAcquisitionAssetExecution(binding, false), nil
		}
		return service.recoverAttempted(ctx, binding)
	}
	if binding.State != eer.StateQueued {
		return ChannelAcquisitionAssetExecution{}, ErrChannelAcquisitionAssetUnavailable
	}
	lease, projection, err := service.effects.Claim(ctx, ChannelAcquisitionAssetEffectClaimCommand{EffectID: effectID, WorkerDigest: workerDigest})
	if err != nil || !validChannelAcquisitionAssetProjection(projection, effectID, eer.StateQueued) || lease.EffectID != effectID || lease.Generation != projection.Generation || lease.Fence < 1 || lease.ExpiresAt.IsZero() {
		return ChannelAcquisitionAssetExecution{}, classifyChannelAcquisitionAssetError(err)
	}
	request := contactport.AcquisitionAssetPublishRequest{
		EffectID: effectID, CorpID: binding.CorpID, CorrelationKey: binding.CorrelationKey, AssetVersion: binding.AssetVersion, Supersedes: binding.SupersedesVersion,
		Kind: binding.Kind, Snapshot: cloneChannelAcquisitionAssetSnapshot(binding.Snapshot), SnapshotDigest: binding.SnapshotDigest,
	}
	var providerResult contactport.AcquisitionAssetProviderResult
	providerCalled, providerResultValid := false, false
	projection, attemptReceipt, runErr := service.effects.RunAttempt(ctx, lease, func(providerCtx context.Context) (eer.AdapterResult, error) {
		if markErr := service.uow.Within(providerCtx, func(tx context.Context) error {
			current, lockErr := service.store.LockBinding(tx, effectID)
			if lockErr != nil {
				return lockErr
			}
			if current.State != eer.StateQueued || !channelAcquisitionAssetCanTransition(current.State, channelAcquisitionAssetStateAttempted) {
				return ErrChannelAcquisitionAssetUnavailable
			}
			binding, lockErr = service.store.MarkAttempted(tx, effectID, lease, service.now().UTC())
			return lockErr
		}); markErr != nil || binding.State != channelAcquisitionAssetStateAttempted {
			return eer.AdapterResult{}, classifyChannelAcquisitionAssetError(markErr)
		}
		var providerErr error
		providerResult, providerErr = service.provider.PublishAcquisitionAsset(providerCtx, request)
		providerCalled = providerResult.BusinessEndpointDispatched
		if providerErr != nil {
			return eer.AdapterResult{BusinessCallDispatched: providerResult.BusinessEndpointDispatched, RealExternalCallExecuted: providerResult.RealExternalCallExecuted}, providerErr
		}
		completion, valid := channelAcquisitionAssetProviderCompletion(providerResult)
		if !valid {
			return eer.AdapterResult{BusinessCallDispatched: providerResult.BusinessEndpointDispatched, RealExternalCallExecuted: providerResult.RealExternalCallExecuted}, nil
		}
		providerResultValid = true
		return eer.AdapterResult{Completion: completion, ReceiptDigest: channelAcquisitionAssetProviderDigest(providerResult.ReceiptDigest),
			ResultReferenceDigest:    channelAcquisitionAssetOptionalProviderDigest(providerResult.AssetReferenceDigest),
			BusinessCallDispatched:   providerResult.BusinessEndpointDispatched,
			RealExternalCallExecuted: providerResult.RealExternalCallExecuted}, nil
	})
	if !validChannelAcquisitionAssetTerminalAttemptProjection(projection, effectID) || !validChannelAcquisitionAssetEffectReceipt(attemptReceipt, effectID, projection.State) || !channelAcquisitionAssetCanTransition(channelAcquisitionAssetStateAttempted, projection.State) {
		return ChannelAcquisitionAssetExecution{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, runErr)
	}
	var assetReference [32]byte
	if providerResultValid && projection.State == eer.StateExecuted {
		assetReference = providerResult.AssetReferenceDigest
	}
	err = service.uow.Within(ctx, func(tx context.Context) error {
		current, lockErr := service.store.LockBinding(tx, effectID)
		if lockErr != nil {
			return lockErr
		}
		if current.State != channelAcquisitionAssetStateAttempted || current.Generation != lease.Generation || current.Fence != lease.Fence {
			return ErrChannelAcquisitionAssetUnavailable
		}
		binding, lockErr = service.store.CompleteAttempt(tx, ChannelAcquisitionAssetAttemptCompletion{
			EffectID: effectID, Lease: lease, State: projection.State, Receipt: attemptReceipt,
			ProviderAssetReferenceDigest: assetReference, ProviderCallAttempted: providerCalled,
			RealExternalCallExecuted: providerResult.RealExternalCallExecuted && providerCalled, CompletedAt: projection.UpdatedAt,
		})
		return lockErr
	})
	if err != nil || !validChannelAcquisitionAssetBinding(binding) {
		return ChannelAcquisitionAssetExecution{}, classifyChannelAcquisitionAssetError(err)
	}
	if binding.State == eer.StateOutcomeUnknown {
		return channelAcquisitionAssetExecution(binding, providerCalled), nil
	}
	if runErr != nil {
		return ChannelAcquisitionAssetExecution{}, classifyChannelAcquisitionAssetError(runErr)
	}
	return channelAcquisitionAssetExecution(binding, providerCalled), nil
}

func (service *ChannelAcquisitionAssetService) convergeTerminal(ctx context.Context, binding ChannelAcquisitionAssetBinding, terminal ChannelAcquisitionAssetEffectTerminal) (ChannelAcquisitionAssetExecution, error) {
	if !validChannelAcquisitionAssetEffectTerminal(terminal, binding.EffectID) {
		return ChannelAcquisitionAssetExecution{}, ErrChannelAcquisitionAssetUnavailable
	}
	var reference [32]byte
	if terminal.State == eer.StateExecuted {
		reference = channelAcquisitionAssetDigestBytes(string(terminal.ResultReferenceDigest))
		if reference == ([32]byte{}) {
			return ChannelAcquisitionAssetExecution{}, ErrChannelAcquisitionAssetUnavailable
		}
	}
	err := service.uow.Within(ctx, func(tx context.Context) error {
		current, lockErr := service.store.LockBinding(tx, binding.EffectID)
		if lockErr != nil {
			return lockErr
		}
		if channelAcquisitionAssetTerminal(current.State) {
			binding = current
			return nil
		}
		if current.State == eer.StateQueued {
			current, lockErr = service.store.MarkAttempted(tx, current.EffectID, terminal.Lease, terminal.Lease.ExpiresAt)
			if lockErr != nil {
				return lockErr
			}
		}
		if current.State != channelAcquisitionAssetStateAttempted || current.Generation != terminal.Lease.Generation || current.Fence != terminal.Lease.Fence {
			return ErrChannelAcquisitionAssetUnavailable
		}
		binding, lockErr = service.store.CompleteAttempt(tx, ChannelAcquisitionAssetAttemptCompletion{
			EffectID: current.EffectID, Lease: terminal.Lease, State: terminal.State, Receipt: terminal.Receipt,
			ProviderAssetReferenceDigest: reference, ProviderCallAttempted: terminal.BusinessCallDispatched,
			RealExternalCallExecuted: terminal.RealExternalCallExecuted, CompletedAt: terminal.Receipt.CompletedAt,
		})
		return lockErr
	})
	if err != nil || !validChannelAcquisitionAssetBinding(binding) {
		return ChannelAcquisitionAssetExecution{}, classifyChannelAcquisitionAssetError(err)
	}
	return channelAcquisitionAssetExecution(binding, false), nil
}

func (service *ChannelAcquisitionAssetService) recoverAttempted(ctx context.Context, binding ChannelAcquisitionAssetBinding) (ChannelAcquisitionAssetExecution, error) {
	lease := eer.Lease{
		EffectID: binding.EffectID, Generation: binding.Generation, Fence: binding.Fence,
		ExpiresAt: binding.LeaseExpiresAt.UTC(),
	}
	projection, receipt, err := service.effects.RecoverAttempted(ctx, lease)
	if err != nil || !validChannelAcquisitionAssetProjection(projection, binding.EffectID, eer.StateOutcomeUnknown) ||
		!validChannelAcquisitionAssetEffectReceipt(receipt, binding.EffectID, eer.StateOutcomeUnknown) ||
		!channelAcquisitionAssetCanTransition(binding.State, projection.State) {
		return ChannelAcquisitionAssetExecution{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	terminal, found, terminalErr := service.effects.Terminal(ctx, binding.EffectID)
	if terminalErr != nil || !found {
		return ChannelAcquisitionAssetExecution{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, terminalErr)
	}
	return service.convergeTerminal(ctx, binding, terminal)
}

// Reconcile never revives an unknown effect. provider_not_applied atomically
// closes the old effect and queues a new Contact asset version.
func (service *ChannelAcquisitionAssetService) Reconcile(ctx context.Context, command ReconcileChannelAcquisitionAssetCommand) (ChannelAcquisitionAssetReconciliation, error) {
	if !service.commandReady(ctx) || !validReconcileChannelAcquisitionAssetCommand(command) {
		return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetReconcileRequired
	}
	var result ChannelAcquisitionAssetReconciliation
	err := service.uow.Within(ctx, func(tx context.Context) error {
		binding, err := service.store.LockBinding(tx, command.EffectID)
		if err != nil || !validChannelAcquisitionAssetBinding(binding) {
			return errors.Join(ErrChannelAcquisitionAssetReconcileRequired, err)
		}
		result, err = service.reconcileLocked(tx, binding, command)
		return err
	})
	if err != nil {
		return ChannelAcquisitionAssetReconciliation{}, classifyChannelAcquisitionAssetError(err)
	}
	return result, nil
}

// ReconcileCurrent derives the current lease from the locked binding. API
// clients cannot choose generation, fence, or lease expiry.
func (service *ChannelAcquisitionAssetService) ReconcileCurrent(ctx context.Context, command ReconcileCurrentChannelAcquisitionAssetCommand) (ChannelAcquisitionAssetReconciliation, error) {
	if !service.commandReady(ctx) || !validReconcileCurrentChannelAcquisitionAssetCommand(command) {
		return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetReconcileRequired
	}
	var result ChannelAcquisitionAssetReconciliation
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var binding ChannelAcquisitionAssetBinding
		var err error
		if scoped, ok := service.store.(channelAcquisitionAssetScopedLockStore); ok {
			binding, err = scoped.LockBindingForChannel(tx, command.ChannelID, command.EffectID)
		} else {
			binding, err = service.store.LockBinding(tx, command.EffectID)
		}
		if err != nil || !validChannelAcquisitionAssetBinding(binding) {
			if errors.Is(err, ErrChannelAcquisitionAssetNotFound) {
				return ErrChannelAcquisitionAssetNotFound
			}
			return errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
		}
		if binding.ChannelID != command.ChannelID {
			return ErrChannelAcquisitionAssetNotFound
		}
		result, err = service.reconcileLocked(tx, binding, ReconcileChannelAcquisitionAssetCommand{
			EffectID: command.EffectID, Actor: command.Actor, IdempotencyKey: command.IdempotencyKey,
			Generation: binding.Generation, Fence: binding.Fence, LeaseExpiresAt: binding.LeaseExpiresAt,
			EvidenceDigest: command.EvidenceDigest, Resolution: command.Resolution,
		})
		return err
	})
	if err != nil {
		return ChannelAcquisitionAssetReconciliation{}, classifyChannelAcquisitionAssetError(err)
	}
	return result, nil
}

func (service *ChannelAcquisitionAssetService) reconcileLocked(tx context.Context, binding ChannelAcquisitionAssetBinding, command ReconcileChannelAcquisitionAssetCommand) (ChannelAcquisitionAssetReconciliation, error) {
	if !validChannelAcquisitionAssetBinding(binding) || binding.EffectID != command.EffectID || !validReconcileChannelAcquisitionAssetCommand(command) {
		return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetReconcileRequired
	}
	var result ChannelAcquisitionAssetReconciliation
	now := service.now().UTC()
	candidate := ChannelAcquisitionAssetActorReceipt{
		Operation: ChannelAcquisitionAssetOperationReconcile, Actor: command.Actor,
		KeyDigest:     channelAcquisitionAssetDigest("actor-key", strconv.FormatInt(command.Actor, 10), command.IdempotencyKey),
		PayloadDigest: channelAcquisitionAssetDigest("reconcile-payload", command.EffectID, strconv.FormatInt(command.Generation, 10), strconv.FormatInt(command.Fence, 10), command.LeaseExpiresAt.UTC().Format(time.RFC3339Nano), string(command.EvidenceDigest), string(command.Resolution)),
		State:         ChannelAcquisitionAssetReceiptInProgress, CreatedAt: now,
	}
	receipt, owned, err := service.store.ReserveActorReceipt(tx, candidate)
	if err != nil {
		return ChannelAcquisitionAssetReconciliation{}, err
	}
	if !sameChannelAcquisitionAssetReceiptCommand(receipt, candidate) {
		return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetConflict
	}
	if !owned {
		result, err = service.replayReconciliation(tx, receipt)
		return result, err
	}
	lease := eer.Lease{EffectID: command.EffectID, Generation: command.Generation, Fence: command.Fence, ExpiresAt: command.LeaseExpiresAt.UTC()}
	if binding.State != eer.StateOutcomeUnknown || binding.Generation != lease.Generation || binding.Fence != lease.Fence || !binding.LeaseExpiresAt.Equal(lease.ExpiresAt) || !channelAcquisitionAssetCanTransition(binding.State, eer.StateReconciled) {
		return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetReconcileRequired
	}
	projection, effectReceipt, err := service.effects.Reconcile(tx, ChannelAcquisitionAssetEffectReconcileCommand{
		Lease: lease, ReceiptKeyDigest: channelAcquisitionAssetDigest("eer-reconcile", strconv.FormatInt(command.Actor, 10), command.IdempotencyKey), EvidenceDigest: command.EvidenceDigest,
	})
	if err != nil || !validChannelAcquisitionAssetProjection(projection, command.EffectID, eer.StateReconciled) || !validChannelAcquisitionAssetEffectReceipt(effectReceipt, command.EffectID, eer.StateReconciled) {
		return ChannelAcquisitionAssetReconciliation{}, errors.Join(ErrChannelAcquisitionAssetReconcileRequired, err)
	}
	binding, err = service.store.CompleteReconcile(tx, ChannelAcquisitionAssetReconcileCompletion{
		EffectID: command.EffectID, Lease: lease, Receipt: effectReceipt, EvidenceDigest: command.EvidenceDigest,
		Resolution: command.Resolution, CompletedAt: projection.UpdatedAt,
	})
	if err != nil || !validChannelAcquisitionAssetBinding(binding) || binding.State != eer.StateReconciled {
		return ChannelAcquisitionAssetReconciliation{}, errors.Join(ErrChannelAcquisitionAssetReconcileRequired, err)
	}
	var replacement *ChannelAcquisitionAssetBinding
	if command.Resolution == ChannelAcquisitionAssetProviderNotApplied {
		snapshot, snapshotErr := service.store.LockSnapshot(tx, binding.ChannelID)
		if snapshotErr != nil {
			return ChannelAcquisitionAssetReconciliation{}, snapshotErr
		}
		snapshot, snapshotDigest, ok := canonicalChannelAcquisitionAssetSnapshot(snapshot, binding.Kind)
		if !ok || snapshot.ChannelID != binding.ChannelID {
			return ChannelAcquisitionAssetReconciliation{}, ErrInvalidChannelAcquisitionAsset
		}
		queued, queueErr := service.queueNewBinding(tx, command.Actor, command.IdempotencyKey+"\x00replacement\x00"+binding.EffectID, snapshot, snapshotDigest, binding.Kind, binding.AssetVersion, now)
		if queueErr != nil {
			return ChannelAcquisitionAssetReconciliation{}, queueErr
		}
		replacement = &queued
	}
	replacementEffectID := ""
	if replacement != nil {
		replacementEffectID = replacement.EffectID
	}
	completed, err := service.store.CompleteActorReceipt(tx, receipt.ID, binding.EffectID, replacementEffectID, now)
	if err != nil || !validCompletedChannelAcquisitionAssetReceipt(completed, receipt, binding.EffectID, replacementEffectID) {
		return ChannelAcquisitionAssetReconciliation{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	result, err = channelAcquisitionAssetReconciliation(binding, replacement)
	return result, err
}

func (service *ChannelAcquisitionAssetService) queueNewBinding(ctx context.Context, actor int64, idempotencySeed string, snapshot contactport.AcquisitionAssetSnapshot, snapshotDigest [32]byte, kind contactport.AcquisitionAssetKind, supersedesVersion int64, now time.Time) (ChannelAcquisitionAssetBinding, error) {
	version, err := service.store.NextAssetVersion(ctx, snapshot.ChannelID, supersedesVersion, kind)
	if err != nil || version < 1 || version <= supersedesVersion {
		return ChannelAcquisitionAssetBinding{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	spec := channelAcquisitionAssetEffectSpec(snapshot, snapshotDigest, kind, version, supersedesVersion)
	if spec.Fingerprint() == "" {
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetUnavailable
	}
	accepted, acceptReceipt, err := service.effects.Accept(ctx, ChannelAcquisitionAssetEffectAcceptCommand{
		ReceiptKeyDigest: channelAcquisitionAssetDigest("eer-accept", strconv.FormatInt(actor, 10), idempotencySeed, strconv.FormatInt(version, 10)), Spec: spec,
	})
	if err != nil || !validAcceptedChannelAcquisitionAssetProjection(accepted) || !validChannelAcquisitionAssetEffectReceipt(acceptReceipt, accepted.ID, eer.StateAccepted) {
		return ChannelAcquisitionAssetBinding{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	correlationKey, err := newChannelAcquisitionAssetCorrelationKey()
	if err != nil {
		return ChannelAcquisitionAssetBinding{}, err
	}
	candidate := ChannelAcquisitionAssetBinding{
		EffectID: accepted.ID, CorpID: service.corpID, CorrelationKey: correlationKey, ChannelID: snapshot.ChannelID, Kind: kind, AssetVersion: version, SupersedesVersion: supersedesVersion,
		Snapshot: cloneChannelAcquisitionAssetSnapshot(snapshot), SnapshotDigest: snapshotDigest,
		IdempotencyDigest: channelAcquisitionAssetDigest("idempotency", strconv.FormatInt(actor, 10), idempotencySeed), EnvelopeFingerprint: accepted.EnvelopeFingerprint,
		State: eer.StateAccepted, AcceptReceiptID: acceptReceipt.ID, AcceptReceiptDigest: acceptReceipt.CommandDigest,
		Generation: accepted.Generation, EntrantReady: false, CreatedAt: now, UpdatedAt: accepted.UpdatedAt,
	}
	stored, err := service.store.InsertAccepted(ctx, candidate)
	if err != nil || !sameChannelAcquisitionAssetBindingCommand(stored, candidate) || !validChannelAcquisitionAssetBinding(stored) {
		return ChannelAcquisitionAssetBinding{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	link, err := service.jobs.Insert(ctx, ChannelAcquisitionAssetJobArgs{EffectID: accepted.ID}, accepted.Generation+1, now)
	if err != nil || !validChannelAcquisitionAssetRiverLink(link, accepted.Generation+1) {
		return ChannelAcquisitionAssetBinding{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	queued, queueReceipt, err := service.effects.Queue(ctx, ChannelAcquisitionAssetEffectQueueCommand{
		EffectID: accepted.ID, Job: link, ReceiptKeyDigest: channelAcquisitionAssetDigest("eer-queue", strconv.FormatInt(actor, 10), idempotencySeed, accepted.ID),
	})
	if err != nil || !validChannelAcquisitionAssetProjection(queued, accepted.ID, eer.StateQueued) || queued.Generation != link.Generation || !validChannelAcquisitionAssetEffectReceipt(queueReceipt, accepted.ID, eer.StateQueued) || !channelAcquisitionAssetCanTransition(stored.State, queued.State) {
		return ChannelAcquisitionAssetBinding{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	stored, err = service.store.MarkQueued(ctx, accepted.ID, link, queueReceipt, queued.UpdatedAt)
	if err != nil || !validChannelAcquisitionAssetBinding(stored) || stored.State != eer.StateQueued || stored.RiverJobID != link.JobID || stored.Generation != link.Generation {
		return ChannelAcquisitionAssetBinding{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	return stored, nil
}

func newChannelAcquisitionAssetCorrelationKey() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	return "ch02_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func channelAcquisitionAssetOptionalProviderDigest(value [32]byte) eer.Digest {
	if value == ([32]byte{}) {
		return ""
	}
	return channelAcquisitionAssetProviderDigest(value)
}

func channelAcquisitionAssetDigestBytes(value string) (result [32]byte) {
	raw, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	if err == nil && len(raw) == len(result) {
		copy(result[:], raw)
	}
	return result
}

func validChannelAcquisitionAssetCorrelationKey(value string) bool {
	if !strings.HasPrefix(value, "ch02_") || len(value) != len("ch02_")+43 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "ch02_"))
	return err == nil && len(raw) == 32
}

func (service *ChannelAcquisitionAssetService) replayPublish(ctx context.Context, receipt ChannelAcquisitionAssetActorReceipt) (ChannelAcquisitionAssetAcceptance, error) {
	if receipt.State != ChannelAcquisitionAssetReceiptCompleted || receipt.ResultEffectID == "" || receipt.ReplacementEffectID != "" || receipt.CompletedAt.IsZero() {
		return ChannelAcquisitionAssetAcceptance{}, ErrChannelAcquisitionAssetUnavailable
	}
	binding, err := service.store.LockBinding(ctx, receipt.ResultEffectID)
	if err != nil || !validChannelAcquisitionAssetBinding(binding) {
		return ChannelAcquisitionAssetAcceptance{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	return channelAcquisitionAssetAcceptance(binding)
}

func (service *ChannelAcquisitionAssetService) replayReconciliation(ctx context.Context, receipt ChannelAcquisitionAssetActorReceipt) (ChannelAcquisitionAssetReconciliation, error) {
	if receipt.State != ChannelAcquisitionAssetReceiptCompleted || receipt.ResultEffectID == "" || receipt.CompletedAt.IsZero() {
		return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetUnavailable
	}
	binding, err := service.store.LockBinding(ctx, receipt.ResultEffectID)
	if err != nil || !validChannelAcquisitionAssetBinding(binding) || binding.State != eer.StateReconciled {
		return ChannelAcquisitionAssetReconciliation{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	var replacement *ChannelAcquisitionAssetBinding
	if receipt.ReplacementEffectID != "" {
		queued, loadErr := service.store.LockBinding(ctx, receipt.ReplacementEffectID)
		if loadErr != nil || !validChannelAcquisitionAssetBinding(queued) {
			return ChannelAcquisitionAssetReconciliation{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, loadErr)
		}
		replacement = &queued
	}
	return channelAcquisitionAssetReconciliation(binding, replacement)
}

func (service *ChannelAcquisitionAssetService) commandReady(ctx context.Context) bool {
	return service != nil && ctx != nil && ctx.Err() == nil && service.now != nil && !channelAcquisitionAssetNil(service.uow) &&
		!channelAcquisitionAssetNil(service.store) && !channelAcquisitionAssetNil(service.effects) && !channelAcquisitionAssetNil(service.jobs)
}

func (service *ChannelAcquisitionAssetService) executeReady(ctx context.Context) bool {
	return service.commandReady(ctx) && !channelAcquisitionAssetNil(service.provider)
}

func channelAcquisitionAssetEffectSpec(snapshot contactport.AcquisitionAssetSnapshot, snapshotDigest [32]byte, kind contactport.AcquisitionAssetKind, version, supersedes int64) ChannelAcquisitionAssetEffectSpec {
	assignees := strings.Join(snapshot.AssigneeWeComUserIDs, "\x00")
	return ChannelAcquisitionAssetEffectSpec{
		SourceRefDigest:   channelAcquisitionAssetDigest("source", strconv.FormatInt(snapshot.ChannelID, 10), strconv.FormatInt(snapshot.ChannelRevision, 10), strconv.FormatInt(version, 10), strconv.FormatInt(supersedes, 10)),
		TargetRefDigest:   channelAcquisitionAssetDigest("target", assignees),
		PayloadDigest:     channelAcquisitionAssetDigest("payload", string(kind), snapshot.ChannelCode, snapshot.ChannelName, snapshot.SceneValue, hex.EncodeToString(snapshotDigest[:])),
		PolicyVersionHash: channelAcquisitionAssetDigest("policy", channelAcquisitionAssetPolicy),
	}
}

func canonicalChannelAcquisitionAssetSnapshot(snapshot contactport.AcquisitionAssetSnapshot, kind contactport.AcquisitionAssetKind) (contactport.AcquisitionAssetSnapshot, [32]byte, bool) {
	result := cloneChannelAcquisitionAssetSnapshot(snapshot)
	sort.Strings(result.AssigneeWeComUserIDs)
	if result.ChannelID < 1 || result.ChannelRevision < 1 || result.ChannelStatus != "active" || !validChannelAcquisitionAssetText(result.ChannelCode, 1, 200) ||
		!validChannelAcquisitionAssetText(result.ChannelName, 1, 200) || len(result.AssigneeWeComUserIDs) == 0 ||
		(kind != contactport.AcquisitionAssetQRCode && kind != contactport.AcquisitionAssetLink) ||
		kind == contactport.AcquisitionAssetQRCode && !validChannelAcquisitionAssetText(result.SceneValue, 1, 512) ||
		kind == contactport.AcquisitionAssetLink && result.SceneValue != "" && !validChannelAcquisitionAssetText(result.SceneValue, 1, 512) {
		return contactport.AcquisitionAssetSnapshot{}, [32]byte{}, false
	}
	for index, assignee := range result.AssigneeWeComUserIDs {
		if !validChannelAcquisitionAssetText(assignee, 1, 200) || index > 0 && assignee == result.AssigneeWeComUserIDs[index-1] {
			return contactport.AcquisitionAssetSnapshot{}, [32]byte{}, false
		}
	}
	value := strings.Join([]string{
		strconv.FormatInt(result.ChannelID, 10), strconv.FormatInt(result.ChannelRevision, 10), result.ChannelCode,
		result.ChannelName, result.ChannelStatus, result.SceneValue, strings.Join(result.AssigneeWeComUserIDs, "\x00"),
	}, "\x00")
	return result, sha256.Sum256([]byte("contact.acquisition.snapshot.v1\x00" + value)), true
}

func validPublishChannelAcquisitionAssetCommand(command PublishChannelAcquisitionAssetCommand) bool {
	return command.ChannelID > 0 && command.Actor > 0 && validKey(command.IdempotencyKey) &&
		(command.Kind == contactport.AcquisitionAssetQRCode || command.Kind == contactport.AcquisitionAssetLink)
}

func validReconcileChannelAcquisitionAssetCommand(command ReconcileChannelAcquisitionAssetCommand) bool {
	return command.EffectID != "" && command.Actor > 0 && validKey(command.IdempotencyKey) && command.Generation > 0 && command.Fence > 0 &&
		!command.LeaseExpiresAt.IsZero() && validChannelAcquisitionAssetDigest(command.EvidenceDigest) &&
		(command.Resolution == ChannelAcquisitionAssetProviderApplied || command.Resolution == ChannelAcquisitionAssetProviderNotApplied)
}

func validReconcileCurrentChannelAcquisitionAssetCommand(command ReconcileCurrentChannelAcquisitionAssetCommand) bool {
	return command.ChannelID > 0 && command.EffectID != "" && command.Actor > 0 && validKey(command.IdempotencyKey) &&
		validChannelAcquisitionAssetDigest(command.EvidenceDigest) &&
		(command.Resolution == ChannelAcquisitionAssetProviderApplied || command.Resolution == ChannelAcquisitionAssetProviderNotApplied)
}

func channelAcquisitionAssetCanTransition(from, to eer.State) bool {
	switch from {
	case eer.StateAccepted:
		return to == eer.StateQueued
	case eer.StateQueued:
		return to == channelAcquisitionAssetStateAttempted
	case channelAcquisitionAssetStateAttempted:
		return to == eer.StateExecuted || to == eer.StateFinalFailed || to == eer.StateOutcomeUnknown
	case eer.StateOutcomeUnknown:
		return to == eer.StateReconciled
	default:
		return false
	}
}

func channelAcquisitionAssetTerminal(state eer.State) bool {
	return state == eer.StateExecuted || state == eer.StateFinalFailed || state == eer.StateOutcomeUnknown || state == eer.StateReconciled
}

func validChannelAcquisitionAssetTerminalAttemptProjection(projection ChannelAcquisitionAssetEffectProjection, effectID string) bool {
	return validChannelAcquisitionAssetProjection(projection, effectID, eer.StateExecuted) || validChannelAcquisitionAssetProjection(projection, effectID, eer.StateFinalFailed) || validChannelAcquisitionAssetProjection(projection, effectID, eer.StateOutcomeUnknown)
}

func validChannelAcquisitionAssetProjection(projection ChannelAcquisitionAssetEffectProjection, effectID string, state eer.State) bool {
	return effectID != "" && projection.ID == effectID && projection.State == state && projection.Generation > 0 && !projection.UpdatedAt.IsZero()
}

func validAcceptedChannelAcquisitionAssetProjection(projection ChannelAcquisitionAssetEffectProjection) bool {
	return validChannelAcquisitionAssetProjection(projection, projection.ID, eer.StateAccepted) && validChannelAcquisitionAssetDigest(projection.EnvelopeFingerprint)
}

func validChannelAcquisitionAssetEffectReceipt(receipt eer.OperationReceipt, effectID string, state eer.State) bool {
	return receipt.ID != "" && receipt.EffectID == effectID && receipt.State == state && validChannelAcquisitionAssetDigest(receipt.CommandDigest) && !receipt.CompletedAt.IsZero()
}

func validChannelAcquisitionAssetEffectTerminal(terminal ChannelAcquisitionAssetEffectTerminal, effectID string) bool {
	if terminal.EffectID != effectID || terminal.Lease.EffectID != effectID || terminal.Lease.Generation < 1 || terminal.Lease.Fence < 1 || terminal.Lease.ExpiresAt.IsZero() ||
		!validChannelAcquisitionAssetEffectReceipt(terminal.Receipt, effectID, terminal.State) ||
		terminal.RealExternalCallExecuted && !terminal.BusinessCallDispatched {
		return false
	}
	switch terminal.State {
	case eer.StateExecuted:
		return terminal.BusinessCallDispatched && terminal.RealExternalCallExecuted && validChannelAcquisitionAssetDigest(terminal.ResultReferenceDigest)
	case eer.StateFinalFailed:
		return terminal.BusinessCallDispatched && terminal.RealExternalCallExecuted && terminal.ResultReferenceDigest == ""
	case eer.StateOutcomeUnknown:
		return terminal.ResultReferenceDigest == ""
	default:
		return false
	}
}

func validChannelAcquisitionAssetRiverLink(link eer.RiverJobLink, generation int64) bool {
	return link.JobID > 0 && link.Generation == generation && validChannelAcquisitionAssetText(link.Queue, 1, 200) && validChannelAcquisitionAssetDigest(link.ArgsDigest) && !link.ScheduledAt.IsZero()
}

func validChannelAcquisitionAssetBinding(binding ChannelAcquisitionAssetBinding) bool {
	_, expectedSnapshotDigest, ok := canonicalChannelAcquisitionAssetSnapshot(binding.Snapshot, binding.Kind)
	if !ok || binding.EffectID == "" || !validChannelAcquisitionAssetText(binding.CorpID, 1, 128) || !validChannelAcquisitionAssetCorrelationKey(binding.CorrelationKey) || binding.ChannelID != binding.Snapshot.ChannelID || binding.AssetVersion < 1 || binding.SupersedesVersion < 0 || binding.AssetVersion <= binding.SupersedesVersion ||
		binding.SnapshotDigest != expectedSnapshotDigest || !validChannelAcquisitionAssetDigest(binding.IdempotencyDigest) || !validChannelAcquisitionAssetDigest(binding.EnvelopeFingerprint) ||
		binding.AcceptReceiptID == "" || !validChannelAcquisitionAssetDigest(binding.AcceptReceiptDigest) || binding.Generation < 1 || binding.EntrantReady ||
		binding.CreatedAt.IsZero() || binding.UpdatedAt.Before(binding.CreatedAt) {
		return false
	}
	switch binding.State {
	case eer.StateAccepted:
		return binding.RiverJobID == 0 && binding.QueueReceiptID == "" && binding.Fence == 0
	case eer.StateQueued:
		return binding.RiverJobID > 0 && binding.QueueReceiptID != "" && validChannelAcquisitionAssetDigest(binding.QueueReceiptDigest) && binding.Fence == 0
	case channelAcquisitionAssetStateAttempted:
		return binding.RiverJobID > 0 && binding.QueueReceiptID != "" && validChannelAcquisitionAssetDigest(binding.QueueReceiptDigest) && binding.Fence > 0 && !binding.LeaseExpiresAt.IsZero() && binding.AttemptReceiptID == ""
	case eer.StateExecuted:
		return validCompletedChannelAcquisitionAssetAttempt(binding) && binding.ProviderCallAttempted && binding.RealExternalCallExecuted && !allZeroChannelAcquisitionAssetDigest(binding.ProviderAssetReferenceDigest)
	case eer.StateFinalFailed:
		return validCompletedChannelAcquisitionAssetAttempt(binding) && binding.ProviderCallAttempted && binding.RealExternalCallExecuted && allZeroChannelAcquisitionAssetDigest(binding.ProviderAssetReferenceDigest)
	case eer.StateOutcomeUnknown:
		return validCompletedChannelAcquisitionAssetAttempt(binding) && allZeroChannelAcquisitionAssetDigest(binding.ProviderAssetReferenceDigest)
	case eer.StateReconciled:
		return validCompletedChannelAcquisitionAssetAttempt(binding) && binding.ReconcileReceiptID != "" && validChannelAcquisitionAssetDigest(binding.ReconcileReceiptDigest) &&
			validChannelAcquisitionAssetDigest(binding.ReconcileEvidenceDigest) && !binding.ReconciledAt.IsZero() &&
			(binding.ReconcileResolution == ChannelAcquisitionAssetProviderApplied || binding.ReconcileResolution == ChannelAcquisitionAssetProviderNotApplied)
	default:
		return false
	}
}

func validCompletedChannelAcquisitionAssetAttempt(binding ChannelAcquisitionAssetBinding) bool {
	return binding.RiverJobID > 0 && binding.QueueReceiptID != "" && validChannelAcquisitionAssetDigest(binding.QueueReceiptDigest) && binding.Fence > 0 &&
		!binding.LeaseExpiresAt.IsZero() && binding.AttemptReceiptID != "" && validChannelAcquisitionAssetDigest(binding.AttemptReceiptDigest)
}

func sameChannelAcquisitionAssetBindingCommand(left, right ChannelAcquisitionAssetBinding) bool {
	return left.EffectID == right.EffectID && left.CorpID == right.CorpID && left.CorrelationKey == right.CorrelationKey && left.ChannelID == right.ChannelID && left.Kind == right.Kind && left.AssetVersion == right.AssetVersion &&
		left.SupersedesVersion == right.SupersedesVersion && left.SnapshotDigest == right.SnapshotDigest && left.IdempotencyDigest == right.IdempotencyDigest &&
		left.EnvelopeFingerprint == right.EnvelopeFingerprint && left.AcceptReceiptID == right.AcceptReceiptID && left.AcceptReceiptDigest == right.AcceptReceiptDigest
}

func sameChannelAcquisitionAssetReceiptCommand(left, right ChannelAcquisitionAssetActorReceipt) bool {
	return left.ID > 0 && left.Operation == right.Operation && left.Actor == right.Actor && left.KeyDigest == right.KeyDigest && left.PayloadDigest == right.PayloadDigest &&
		(left.State == ChannelAcquisitionAssetReceiptInProgress || left.State == ChannelAcquisitionAssetReceiptCompleted)
}

func validCompletedChannelAcquisitionAssetReceipt(completed, original ChannelAcquisitionAssetActorReceipt, effectID, replacementEffectID string) bool {
	return sameChannelAcquisitionAssetReceiptCommand(completed, original) && completed.ID == original.ID && completed.State == ChannelAcquisitionAssetReceiptCompleted &&
		completed.ResultEffectID == effectID && completed.ReplacementEffectID == replacementEffectID && !completed.CompletedAt.IsZero()
}

func channelAcquisitionAssetAcceptance(binding ChannelAcquisitionAssetBinding) (ChannelAcquisitionAssetAcceptance, error) {
	if !validChannelAcquisitionAssetBinding(binding) {
		return ChannelAcquisitionAssetAcceptance{}, ErrChannelAcquisitionAssetUnavailable
	}
	return ChannelAcquisitionAssetAcceptance{
		EffectID: binding.EffectID, ChannelID: binding.ChannelID, Kind: binding.Kind, AssetVersion: binding.AssetVersion,
		SupersedesVersion: binding.SupersedesVersion, State: binding.State, RiverJobID: binding.RiverJobID,
		AcceptReceiptID: binding.AcceptReceiptID, QueueReceiptID: binding.QueueReceiptID, EntrantReady: false,
		RealExternalCallExecuted: binding.RealExternalCallExecuted,
	}, nil
}

func channelAcquisitionAssetExecution(binding ChannelAcquisitionAssetBinding, attempted bool) ChannelAcquisitionAssetExecution {
	return ChannelAcquisitionAssetExecution{
		EffectID: binding.EffectID, ChannelID: binding.ChannelID, Kind: binding.Kind, AssetVersion: binding.AssetVersion, State: binding.State,
		AttemptReceiptDigest: binding.AttemptReceiptDigest, ProviderCallAttempted: attempted,
		ManualReconcileRequired: binding.State == channelAcquisitionAssetStateAttempted || binding.State == eer.StateOutcomeUnknown,
		EntrantReady:            false, RealExternalCallExecuted: binding.RealExternalCallExecuted,
	}
}

func channelAcquisitionAssetReconciliation(binding ChannelAcquisitionAssetBinding, replacement *ChannelAcquisitionAssetBinding) (ChannelAcquisitionAssetReconciliation, error) {
	if !validChannelAcquisitionAssetBinding(binding) || binding.State != eer.StateReconciled {
		return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetUnavailable
	}
	result := ChannelAcquisitionAssetReconciliation{
		EffectID: binding.EffectID, State: binding.State, Resolution: binding.ReconcileResolution, ReceiptID: binding.ReconcileReceiptID,
		ProviderSuccessClaimed: binding.ReconcileResolution == ChannelAcquisitionAssetProviderApplied, EntrantReady: false,
	}
	if replacement != nil {
		accepted, err := channelAcquisitionAssetAcceptance(*replacement)
		if err != nil || replacement.SupersedesVersion != binding.AssetVersion || replacement.EffectID == binding.EffectID {
			return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetUnavailable
		}
		result.Replacement = &accepted
	}
	if binding.ReconcileResolution == ChannelAcquisitionAssetProviderNotApplied && result.Replacement == nil ||
		binding.ReconcileResolution == ChannelAcquisitionAssetProviderApplied && result.Replacement != nil {
		return ChannelAcquisitionAssetReconciliation{}, ErrChannelAcquisitionAssetUnavailable
	}
	return result, nil
}

func channelAcquisitionAssetProviderCompletion(result contactport.AcquisitionAssetProviderResult) (eer.Completion, bool) {
	if allZeroChannelAcquisitionAssetDigest(result.ReceiptDigest) {
		return "", false
	}
	switch result.Outcome {
	case contactport.AcquisitionAssetProviderExecuted:
		return eer.Completion("executed"), result.RealExternalCallExecuted && !allZeroChannelAcquisitionAssetDigest(result.AssetReferenceDigest)
	case contactport.AcquisitionAssetProviderFinalFailed:
		return eer.CompletionFinalFailed, result.RealExternalCallExecuted && allZeroChannelAcquisitionAssetDigest(result.AssetReferenceDigest)
	case contactport.AcquisitionAssetProviderOutcomeUnknown:
		return eer.Completion("outcome_unknown"), allZeroChannelAcquisitionAssetDigest(result.AssetReferenceDigest)
	default:
		return "", false
	}
}

func channelAcquisitionAssetProviderDigest(value [32]byte) eer.Digest {
	return eer.Digest("sha256:" + hex.EncodeToString(value[:]))
}

func channelAcquisitionAssetDigest(label string, parts ...string) eer.Digest {
	value := "contact.acquisition.asset." + channelAcquisitionAssetPolicy + "\x00" + label + "\x00" + strings.Join(parts, "\x00")
	sum := sha256.Sum256([]byte(value))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func validChannelAcquisitionAssetDigest(value eer.Digest) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(string(value), prefix) || len(value) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(string(value), prefix))
	return err == nil
}

func allZeroChannelAcquisitionAssetDigest(value [32]byte) bool { return value == [32]byte{} }

func validChannelAcquisitionAssetText(value string, minimum, maximum int) bool {
	return len(value) >= minimum && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func cloneChannelAcquisitionAssetSnapshot(snapshot contactport.AcquisitionAssetSnapshot) contactport.AcquisitionAssetSnapshot {
	snapshot.AssigneeWeComUserIDs = append([]string(nil), snapshot.AssigneeWeComUserIDs...)
	return snapshot
}

func cloneChannelAcquisitionAssetBinding(binding ChannelAcquisitionAssetBinding) ChannelAcquisitionAssetBinding {
	binding.Snapshot = cloneChannelAcquisitionAssetSnapshot(binding.Snapshot)
	return binding
}

func channelAcquisitionAssetNil(value any) bool {
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

func classifyChannelAcquisitionAssetError(err error) error {
	if err == nil {
		return ErrChannelAcquisitionAssetUnavailable
	}
	switch {
	case errors.Is(err, ErrInvalidChannelAcquisitionAsset), errors.Is(err, ErrChannelAcquisitionAssetConflict), errors.Is(err, ErrChannelAcquisitionAssetNotFound), errors.Is(err, ErrChannelAcquisitionAssetUnavailable), errors.Is(err, ErrChannelAcquisitionAssetReconcileRequired):
		return err
	default:
		return errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
}
