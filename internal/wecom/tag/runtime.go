// Package tag owns WeCom provider-tag execution. Contact may accept legacy
// commands, but only this package turns a typed tag command into an EER
// attempt. An EER queue/receipt is never treated as provider success.
package tag

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
	"unicode"
	"unicode/utf8"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const (
	JobKind      = "wecom_tag_effect"
	policyV1     = "b1-wc01-wecom-tag-effect-v1"
	ownerWeCom   = "wecom"
	kindTagSync  = "wecom_tag_sync"
	maximumTagID = 128
)

var (
	ErrInvalidConfiguration = errors.New("invalid WeCom tag runtime configuration")
	ErrInvalidCommand       = errors.New("invalid WeCom tag effect command")
	ErrEffectConflict       = errors.New("WeCom tag effect idempotency conflict")
	ErrEffectUnavailable    = errors.New("WeCom tag effect unavailable")
	ErrReconcileRequired    = errors.New("WeCom tag effect manual reconciliation required")
)

type Operation string

const (
	OperationCatalogSync Operation = "catalog_sync"
	OperationMark        Operation = "mark"
	OperationUnmark      Operation = "unmark"
)

type SyncTrigger string

const (
	SyncTriggerManual SyncTrigger = "manual"
	SyncTriggerDue    SyncTrigger = "due"
)

type ReconcileResolution string

const (
	ResolutionProviderApplied    ReconcileResolution = "provider_applied"
	ResolutionProviderNotApplied ReconcileResolution = "provider_not_applied"
)

// QueueCommand contains the typed facts missing from the old opaque 00038
// payload. Actor is supplied by the authenticated server-side principal and
// CorpID is supplied by Service configuration, never by the request body.
type QueueCommand struct {
	LegacyReceiptID int64
	Actor           int64
	IdempotencyKey  string
	Operation       Operation
	SyncTrigger     SyncTrigger
	ExternalUserID  string
	ProviderTagIDs  []string
}

type JobArgs struct {
	EffectID string `json:"effect_id"`
}

func (JobArgs) Kind() string { return JobKind }

type Effect struct {
	EffectID               string
	LegacyReceiptID        int64
	Actor                  int64
	CorpID                 string
	Operation              Operation
	SyncTrigger            SyncTrigger
	ExternalUserID         string
	ProviderTagIDs         []string
	IdempotencyDigest      eer.Digest
	EnvelopeFingerprint    eer.Digest
	State                  eer.State
	AcceptReceiptID        string
	QueueReceiptID         string
	RiverJobID             int64
	Generation             int64
	Fence                  int64
	LeaseExpiresAt         time.Time
	AttemptReceiptID       string
	AttemptReceiptDigest   eer.Digest
	AttemptCompletedAt     time.Time
	ReconcileReceiptID     string
	ReconcileReceiptDigest eer.Digest
	ReconcileResolution    ReconcileResolution
	ReconcileEvidenceHash  eer.Digest
	ReconciledAt           time.Time
	UpdatedAt              time.Time
}

type Acceptance struct {
	EffectID                 string    `json:"effect_id"`
	Operation                Operation `json:"operation"`
	State                    eer.State `json:"state"`
	RiverJobID               int64     `json:"river_job_id"`
	AcceptReceiptID          string    `json:"accept_receipt_id"`
	QueueReceiptID           string    `json:"queue_receipt_id"`
	RealExternalCallExecuted bool      `json:"real_external_call_executed"`
}

type Execution struct {
	EffectID                 string
	Operation                Operation
	State                    eer.State
	AttemptReceiptDigest     eer.Digest
	ProviderCallAttempted    bool
	ManualReconcileRequired  bool
	RealExternalCallExecuted bool
}

type ReconcileCommand struct {
	EffectID       string
	Actor          int64
	IdempotencyKey string
	Generation     int64
	Fence          int64
	LeaseExpiresAt time.Time
	EvidenceDigest eer.Digest
	Resolution     ReconcileResolution
}

type Reconciliation struct {
	EffectID               string              `json:"effect_id"`
	State                  eer.State           `json:"state"`
	Resolution             ReconcileResolution `json:"resolution"`
	ReceiptID              string              `json:"receipt_id"`
	ProviderCallAttempted  bool                `json:"provider_call_attempted"`
	ProviderSuccessClaimed bool                `json:"provider_success_claimed"`
}

type Store interface {
	Reserve(context.Context, Effect) (Effect, bool, error)
	GetByIdempotency(context.Context, int64, eer.Digest) (Effect, error)
	MarkQueued(context.Context, string, eer.RiverJobLink, string, time.Time) (Effect, error)
	Get(context.Context, string) (Effect, error)
	RecordClaim(context.Context, string, eer.Lease, time.Time) (Effect, error)
	CompleteAttempt(context.Context, AttemptCompletion) (Effect, error)
	CompleteReconcile(context.Context, ReconcileCompletion) (Effect, error)
}

type JobInserter interface {
	Insert(context.Context, JobArgs, int64, time.Time) (eer.RiverJobLink, error)
}

type AttemptCompletion struct {
	EffectID    string
	Lease       eer.Lease
	State       eer.State
	ReceiptID   string
	Receipt     eer.Digest
	Catalog     CatalogSnapshot
	CompletedAt time.Time
}

type ReconcileCompletion struct {
	EffectID       string
	Lease          eer.Lease
	ReceiptID      string
	Receipt        eer.Digest
	EvidenceDigest eer.Digest
	Resolution     ReconcileResolution
	CompletedAt    time.Time
}

type Service struct {
	uow     platformport.UnitOfWork
	store   Store
	runtime eer.Runtime
	jobs    JobInserter
	corpID  string
	now     func() time.Time
}

func NewService(uow platformport.UnitOfWork, store Store, runtime eer.Runtime, jobs JobInserter, corpID string) (*Service, error) {
	if nilDependency(uow) || nilDependency(store) || nilDependency(runtime) || nilDependency(jobs) || !validCorpID(corpID) {
		return nil, ErrInvalidConfiguration
	}
	return &Service{uow: uow, store: store, runtime: runtime, jobs: jobs, corpID: corpID, now: time.Now}, nil
}

// Queue accepts and queues one typed effect in a single caller UoW. A replay
// returns the durable local record and never inserts another River job.
func (service *Service) Queue(ctx context.Context, command QueueCommand) (Acceptance, error) {
	if !service.ready(ctx) {
		return Acceptance{}, ErrInvalidConfiguration
	}
	var result Acceptance
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var err error
		result, err = service.queueInTransaction(txCtx, command)
		return err
	})
	if err != nil {
		return Acceptance{}, err
	}
	return result, nil
}

// QueueInTransaction joins an existing business-package UoW. It is used by
// the Contact compatibility bridge so the legacy receipt and typed EER queue
// commit or roll back together.
func (service *Service) QueueInTransaction(ctx context.Context, command QueueCommand) (Acceptance, error) {
	if _, err := platformstore.TxFromContext(ctx); err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	return service.queueInTransaction(ctx, command)
}

func (service *Service) queueInTransaction(ctx context.Context, command QueueCommand) (Acceptance, error) {
	if !service.ready(ctx) {
		return Acceptance{}, ErrInvalidConfiguration
	}
	canonical, ok := canonicalCommand(command)
	if !ok {
		return Acceptance{}, ErrInvalidCommand
	}
	envelope, err := effectEnvelope(service.corpID, canonical)
	if err != nil {
		return Acceptance{}, err
	}
	projection, acceptReceipt, err := service.runtime.Accept(ctx, eer.AcceptCommand{
		ReceiptKeyDigest: digest("accept", strconv.FormatInt(canonical.Actor, 10), canonical.IdempotencyKey),
		Envelope:         envelope,
	})
	if err != nil {
		if errors.Is(err, eer.ErrPayloadMismatch) {
			return Acceptance{}, ErrEffectConflict
		}
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	if !validProjection(projection) || acceptReceipt.ID == "" || acceptReceipt.EffectID != projection.ID {
		return Acceptance{}, ErrEffectUnavailable
	}
	candidate := effectFromCommand(service.corpID, canonical, projection, acceptReceipt.ID, envelope.Fingerprint(), service.now().UTC())
	stored, inserted, err := service.store.Reserve(ctx, candidate)
	if err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	if !sameEffectCommand(stored, candidate) || stored.EffectID != projection.ID {
		return Acceptance{}, ErrEffectConflict
	}
	if !inserted {
		return acceptance(stored)
	}
	if projection.State != eer.StateAccepted || projection.Generation < 1 {
		return Acceptance{}, ErrEffectUnavailable
	}
	scheduledAt := service.now().UTC()
	link, err := service.jobs.Insert(ctx, JobArgs{EffectID: projection.ID}, projection.Generation+1, scheduledAt)
	if err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	queued, queueReceipt, err := service.runtime.Queue(ctx, eer.QueueCommand{
		EffectID: projection.ID, Job: link,
		ReceiptKeyDigest: digest("queue", strconv.FormatInt(canonical.Actor, 10), canonical.IdempotencyKey),
	})
	if err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	if !validProjection(queued) || queued.State != eer.StateQueued || queued.Generation != link.Generation ||
		queueReceipt.ID == "" || queueReceipt.EffectID != projection.ID {
		return Acceptance{}, ErrEffectUnavailable
	}
	stored, err = service.store.MarkQueued(ctx, projection.ID, link, queueReceipt.ID, queued.UpdatedAt)
	if err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	return acceptance(stored)
}

// ReplayInTransaction returns only an already-bound typed effect. It never
// creates an EER or River job for a historical 00038 queued receipt.
func (service *Service) ReplayInTransaction(ctx context.Context, command QueueCommand) (Acceptance, error) {
	if _, err := platformstore.TxFromContext(ctx); err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	if !service.ready(ctx) {
		return Acceptance{}, ErrInvalidConfiguration
	}
	canonical, ok := canonicalCommand(command)
	if !ok {
		return Acceptance{}, ErrInvalidCommand
	}
	stored, err := service.store.GetByIdempotency(ctx, canonical.Actor, digest("idempotency", strconv.FormatInt(canonical.Actor, 10), canonical.IdempotencyKey))
	if err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	envelope, err := effectEnvelope(service.corpID, canonical)
	if err != nil {
		return Acceptance{}, err
	}
	expected := effectFromCommand(service.corpID, canonical, eer.Projection{ID: stored.EffectID, State: stored.State, Generation: stored.Generation}, stored.AcceptReceiptID, envelope.Fingerprint(), stored.UpdatedAt)
	if !sameEffectCommand(stored, expected) {
		return Acceptance{}, ErrEffectConflict
	}
	return acceptance(stored)
}

// Execute is called by River. A local outcome_unknown/reconciled/final state
// is checked before EER Claim, so even an accidental River replay cannot call
// Provider twice after an ambiguous result.
func (service *Service) Execute(ctx context.Context, effectID string, workerDigest eer.Digest, provider Provider) (Execution, error) {
	if !service.ready(ctx) || effectID == "" || !validDigest(workerDigest) || nilDependency(provider) {
		return Execution{}, ErrInvalidCommand
	}
	record, err := service.store.Get(ctx, effectID)
	if err != nil {
		return Execution{}, errors.Join(ErrEffectUnavailable, err)
	}
	if terminalState(record.State) {
		return execution(record, false), nil
	}
	if record.State != eer.StateQueued {
		return Execution{}, ErrEffectUnavailable
	}
	lease, projection, err := service.runtime.Claim(ctx, eer.ClaimCommand{EffectID: effectID, WorkerDigest: workerDigest})
	if err != nil || !validProjection(projection) || projection.ID != effectID || projection.State != eer.StateQueued {
		return Execution{}, errors.Join(ErrEffectUnavailable, err)
	}
	if _, err = service.store.RecordClaim(ctx, effectID, lease, service.now().UTC()); err != nil {
		return Execution{}, errors.Join(ErrEffectUnavailable, err)
	}
	adapter := &effectAdapter{record: record, provider: provider}
	projection, receipt, runErr := service.runtime.RunAttempt(ctx, lease, adapter)
	if !terminalAttemptState(projection.State) || projection.ID != effectID || receipt.ID == "" || receipt.EffectID != effectID {
		return Execution{}, errors.Join(ErrEffectUnavailable, runErr)
	}
	completed, err := service.store.CompleteAttempt(ctx, AttemptCompletion{
		EffectID: effectID, Lease: lease, State: projection.State, ReceiptID: receipt.ID,
		Receipt: receipt.CommandDigest, Catalog: adapter.catalog, CompletedAt: projection.UpdatedAt,
	})
	if err != nil {
		return Execution{}, errors.Join(ErrEffectUnavailable, err)
	}
	if projection.State == eer.StateOutcomeUnknown {
		// Unknown is durable manual-review state. Do not return an error that
		// River would interpret as permission to run Provider again.
		return execution(completed, true), nil
	}
	if runErr != nil {
		return Execution{}, errors.Join(ErrEffectUnavailable, runErr)
	}
	return execution(completed, true), nil
}

func (service *Service) Reconcile(ctx context.Context, command ReconcileCommand) (Reconciliation, error) {
	if !service.ready(ctx) || !validReconcile(command) {
		return Reconciliation{}, ErrReconcileRequired
	}
	var result Effect
	var receiptID string
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		record, err := service.store.Get(txCtx, command.EffectID)
		if err != nil {
			return err
		}
		if record.State != eer.StateOutcomeUnknown && record.State != eer.StateReconciled {
			return ErrReconcileRequired
		}
		lease := eer.Lease{EffectID: command.EffectID, Generation: command.Generation, Fence: command.Fence, ExpiresAt: command.LeaseExpiresAt.UTC()}
		if record.Generation != lease.Generation || record.Fence != lease.Fence || !record.LeaseExpiresAt.Equal(lease.ExpiresAt) {
			return ErrReconcileRequired
		}
		projection, receipt, err := service.runtime.Reconcile(txCtx, eer.ReconcileCommand{
			Lease: lease, ReceiptKeyDigest: digest("reconcile", strconv.FormatInt(command.Actor, 10), command.IdempotencyKey),
			EvidenceDigest: command.EvidenceDigest,
		})
		if err != nil || projection.ID != command.EffectID || projection.State != eer.StateReconciled || receipt.ID == "" {
			return errors.Join(ErrReconcileRequired, err)
		}
		receiptID = receipt.ID
		result, err = service.store.CompleteReconcile(txCtx, ReconcileCompletion{
			EffectID: command.EffectID, Lease: lease, ReceiptID: receipt.ID, Receipt: receipt.CommandDigest,
			EvidenceDigest: command.EvidenceDigest, Resolution: command.Resolution, CompletedAt: projection.UpdatedAt,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, ErrReconcileRequired) || errors.Is(err, eer.ErrReconcileRequired) || errors.Is(err, eer.ErrLeaseFence) || errors.Is(err, eer.ErrPayloadMismatch) {
			return Reconciliation{}, ErrReconcileRequired
		}
		return Reconciliation{}, errors.Join(ErrEffectUnavailable, err)
	}
	return Reconciliation{EffectID: result.EffectID, State: result.State, Resolution: result.ReconcileResolution, ReceiptID: receiptID}, nil
}

func (service *Service) ready(ctx context.Context) bool {
	return service != nil && ctx != nil && !nilDependency(service.uow) && !nilDependency(service.store) &&
		!nilDependency(service.runtime) && !nilDependency(service.jobs) && validCorpID(service.corpID) && service.now != nil
}

func effectEnvelope(corpID string, command QueueCommand) (eer.EffectEnvelope, error) {
	target := []string{corpID, command.ExternalUserID}
	tags := append([]string(nil), command.ProviderTagIDs...)
	return eer.NewEnvelope(eer.EnvelopeInput{
		Owner: ownerWeCom, Kind: kindTagSync,
		SourceRefDigest:   digest("source", strconv.FormatInt(command.LegacyReceiptID, 10), strconv.FormatInt(command.Actor, 10)),
		TargetRefDigest:   digest("target", target...),
		PayloadDigest:     digest("payload", string(command.Operation), string(command.SyncTrigger), strings.Join(tags, "\x00")),
		PolicyVersionHash: digest("policy", policyV1),
	})
}

func effectFromCommand(corpID string, command QueueCommand, projection eer.Projection, receiptID string, fingerprint eer.Digest, now time.Time) Effect {
	return Effect{
		EffectID: projection.ID, LegacyReceiptID: command.LegacyReceiptID, Actor: command.Actor, CorpID: corpID,
		Operation: command.Operation, SyncTrigger: command.SyncTrigger, ExternalUserID: command.ExternalUserID,
		ProviderTagIDs: append([]string(nil), command.ProviderTagIDs...), IdempotencyDigest: digest("idempotency", strconv.FormatInt(command.Actor, 10), command.IdempotencyKey),
		EnvelopeFingerprint: fingerprint, State: projection.State, AcceptReceiptID: receiptID, Generation: projection.Generation,
		UpdatedAt: now,
	}
}

func canonicalCommand(command QueueCommand) (QueueCommand, bool) {
	if command.LegacyReceiptID <= 0 || command.Actor <= 0 || !validText(command.IdempotencyKey, 16, 128) {
		return QueueCommand{}, false
	}
	result := command
	result.ProviderTagIDs = append([]string(nil), command.ProviderTagIDs...)
	sort.Strings(result.ProviderTagIDs)
	for index, value := range result.ProviderTagIDs {
		if !validText(value, 1, maximumTagID) || index > 0 && value == result.ProviderTagIDs[index-1] {
			return QueueCommand{}, false
		}
	}
	switch result.Operation {
	case OperationCatalogSync:
		if (result.SyncTrigger != SyncTriggerManual && result.SyncTrigger != SyncTriggerDue) || result.ExternalUserID != "" || len(result.ProviderTagIDs) != 0 {
			return QueueCommand{}, false
		}
	case OperationMark, OperationUnmark:
		if result.SyncTrigger != "" || !validText(result.ExternalUserID, 1, 1024) || len(result.ProviderTagIDs) < 1 || len(result.ProviderTagIDs) > 100 {
			return QueueCommand{}, false
		}
	default:
		return QueueCommand{}, false
	}
	return result, true
}

func sameEffectCommand(left, right Effect) bool {
	return left.EffectID == right.EffectID && left.LegacyReceiptID == right.LegacyReceiptID && left.Actor == right.Actor &&
		left.CorpID == right.CorpID && left.Operation == right.Operation && left.SyncTrigger == right.SyncTrigger &&
		left.ExternalUserID == right.ExternalUserID && reflect.DeepEqual(left.ProviderTagIDs, right.ProviderTagIDs) &&
		left.IdempotencyDigest == right.IdempotencyDigest && left.EnvelopeFingerprint == right.EnvelopeFingerprint
}

func acceptance(effect Effect) (Acceptance, error) {
	if effect.EffectID == "" || effect.RiverJobID <= 0 || effect.AcceptReceiptID == "" || effect.QueueReceiptID == "" {
		return Acceptance{}, ErrEffectUnavailable
	}
	return Acceptance{EffectID: effect.EffectID, Operation: effect.Operation, State: effect.State, RiverJobID: effect.RiverJobID, AcceptReceiptID: effect.AcceptReceiptID, QueueReceiptID: effect.QueueReceiptID}, nil
}

func execution(effect Effect, attempted bool) Execution {
	return Execution{EffectID: effect.EffectID, Operation: effect.Operation, State: effect.State, AttemptReceiptDigest: effect.AttemptReceiptDigest,
		ProviderCallAttempted: attempted, ManualReconcileRequired: effect.State == eer.StateOutcomeUnknown}
}

func validProjection(value eer.Projection) bool {
	return value.ID != "" && string(value.Owner) == ownerWeCom && string(value.Kind) == kindTagSync && value.Generation > 0 && !value.UpdatedAt.IsZero()
}

func terminalAttemptState(state eer.State) bool {
	return state == eer.StateExecuted || state == eer.StateOutcomeUnknown || state == eer.StateFinalFailed
}

func terminalState(state eer.State) bool {
	return terminalAttemptState(state) || state == eer.StateReconciled
}

func validReconcile(command ReconcileCommand) bool {
	return command.EffectID != "" && command.Actor > 0 && validText(command.IdempotencyKey, 16, 128) &&
		command.Generation > 0 && command.Fence > 0 && !command.LeaseExpiresAt.IsZero() && validDigest(command.EvidenceDigest) &&
		(command.Resolution == ResolutionProviderApplied || command.Resolution == ResolutionProviderNotApplied)
}

func digest(label string, parts ...string) eer.Digest {
	sum := sha256.Sum256([]byte("wecom.tag." + policyV1 + "\x00" + label + "\x00" + strings.Join(parts, "\x00")))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func validDigest(value eer.Digest) bool {
	if !strings.HasPrefix(string(value), "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(string(value), "sha256:"))
	return err == nil
}

func validCorpID(value string) bool {
	return validText(value, 1, 256) && strings.IndexFunc(value, unicode.IsSpace) < 0
}

func validText(value string, minimum, maximum int) bool {
	return len(value) >= minimum && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func nilDependency(value any) bool {
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
