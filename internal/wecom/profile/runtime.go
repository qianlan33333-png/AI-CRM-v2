// Package profile owns the typed WeCom external-contact profile writeback
// state machine. It records local/EER facts; it never makes provider I/O.
package profile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const (
	JobKind       = "wecom_contact_profile_effect"
	policyV1      = "p4-wecom-contact-profile-effect-v1"
	ownerWeCom    = "wecom"
	kindProfile   = "wecom_profile_sync"
	maxStaffID    = 128
	maxExternalID = 1024
	maxRemark     = 400
	maxDesc       = 1500
)

var (
	ErrInvalidConfiguration = errors.New("invalid WeCom contact profile effect configuration")
	ErrInvalidCommand       = errors.New("invalid WeCom contact profile effect command")
	ErrEffectConflict       = errors.New("WeCom contact profile effect idempotency conflict")
	ErrEffectUnavailable    = errors.New("WeCom contact profile effect unavailable")
	ErrReconcileRequired    = errors.New("WeCom contact profile effect manual reconciliation required")
)

type QueueCommand struct {
	LegacyReceiptID int64
	Actor           int64
	IdempotencyKey  string
	StaffUserID     string
	ExternalUserID  string
	Remark          string
	Description     string
}

type JobArgs struct {
	EffectID string `json:"effect_id"`
}

func (JobArgs) Kind() string { return JobKind }

type ReconcileResolution string

const (
	ResolutionProviderApplied    ReconcileResolution = "provider_applied"
	ResolutionProviderNotApplied ReconcileResolution = "provider_not_applied"
)

type Effect struct {
	EffectID                                                 string
	LegacyReceiptID, Actor                                   int64
	CorpID, StaffUserID, ExternalUserID, Remark, Description string
	IdempotencyDigest, EnvelopeFingerprint                   eer.Digest
	State                                                    eer.State
	AcceptReceiptID, QueueReceiptID                          string
	RiverJobID, Generation, Fence                            int64
	LeaseExpiresAt                                           time.Time
	AttemptReceiptID                                         string
	AttemptReceiptDigest                                     eer.Digest
	AttemptCompletedAt                                       time.Time
	ProviderCallAttempted, RealExternalCallExecuted          bool
	ReconcileReceiptID                                       string
	ReconcileReceiptDigest, ReconcileEvidenceDigest          eer.Digest
	ReconcileResolution                                      ReconcileResolution
	ReconciledAt, UpdatedAt                                  time.Time
}

type Acceptance struct {
	EffectID, AcceptReceiptID, QueueReceiptID string
	State                                     eer.State
	RiverJobID                                int64
	RealExternalCallExecuted                  bool
}

type Execution struct {
	EffectID                                                                 string
	State                                                                    eer.State
	AttemptReceiptDigest                                                     eer.Digest
	ProviderCallAttempted, RealExternalCallExecuted, ManualReconcileRequired bool
}

type ReconcileCommand struct {
	EffectID          string
	Actor             int64
	IdempotencyKey    string
	Generation, Fence int64
	LeaseExpiresAt    time.Time
	EvidenceDigest    eer.Digest
	Resolution        ReconcileResolution
}

type Reconciliation struct {
	EffectID                                        string
	State                                           eer.State
	Resolution                                      ReconcileResolution
	ReceiptID                                       string
	ProviderCallAttempted, RealExternalCallExecuted bool
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
	EffectID                                        string
	Lease                                           eer.Lease
	State                                           eer.State
	ReceiptID                                       string
	Receipt                                         eer.Digest
	ProviderCallAttempted, RealExternalCallExecuted bool
	CompletedAt                                     time.Time
}
type ReconcileCompletion struct {
	EffectID                string
	Lease                   eer.Lease
	ReceiptID               string
	Receipt, EvidenceDigest eer.Digest
	Resolution              ReconcileResolution
	CompletedAt             time.Time
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

func (s *Service) Queue(ctx context.Context, c QueueCommand) (Acceptance, error) {
	if !s.ready(ctx) {
		return Acceptance{}, ErrInvalidConfiguration
	}
	var result Acceptance
	err := s.uow.Within(ctx, func(tx context.Context) error { var err error; result, err = s.queue(tx, c); return err })
	return result, err
}
func (s *Service) QueueInTransaction(ctx context.Context, c QueueCommand) (Acceptance, error) {
	if _, err := platformstore.TxFromContext(ctx); err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	return s.queue(ctx, c)
}
func (s *Service) queue(ctx context.Context, raw QueueCommand) (Acceptance, error) {
	if !s.ready(ctx) {
		return Acceptance{}, ErrInvalidConfiguration
	}
	c, ok := canonicalCommand(raw)
	if !ok {
		return Acceptance{}, ErrInvalidCommand
	}
	envelope, err := effectEnvelope(s.corpID, c)
	if err != nil {
		return Acceptance{}, err
	}
	p, accept, err := s.runtime.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: digest("accept", strconv.FormatInt(c.Actor, 10), c.IdempotencyKey), Envelope: envelope})
	if err != nil {
		if errors.Is(err, eer.ErrPayloadMismatch) {
			return Acceptance{}, ErrEffectConflict
		}
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	if !validProjection(p) || accept.ID == "" || accept.EffectID != p.ID {
		return Acceptance{}, ErrEffectUnavailable
	}
	candidate := effectFromCommand(s.corpID, c, p, accept.ID, envelope.Fingerprint(), s.now().UTC())
	stored, inserted, err := s.store.Reserve(ctx, candidate)
	if err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	if !sameCommand(stored, candidate) || stored.EffectID != p.ID {
		return Acceptance{}, ErrEffectConflict
	}
	if !inserted {
		return acceptance(stored)
	}
	link, err := s.jobs.Insert(ctx, JobArgs{EffectID: p.ID}, p.Generation+1, s.now().UTC())
	if err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	queued, receipt, err := s.runtime.Queue(ctx, eer.QueueCommand{EffectID: p.ID, Job: link, ReceiptKeyDigest: digest("queue", strconv.FormatInt(c.Actor, 10), c.IdempotencyKey)})
	if err != nil || !validProjection(queued) || queued.State != eer.StateQueued || queued.Generation != link.Generation || receipt.ID == "" || receipt.EffectID != p.ID {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	stored, err = s.store.MarkQueued(ctx, p.ID, link, receipt.ID, queued.UpdatedAt)
	if err != nil {
		return Acceptance{}, errors.Join(ErrEffectUnavailable, err)
	}
	return acceptance(stored)
}

func (s *Service) Execute(ctx context.Context, effectID string, workerDigest eer.Digest, writer wecomport.ContactProfileWriter) (Execution, error) {
	if !s.ready(ctx) || effectID == "" || !validDigest(workerDigest) || nilDependency(writer) {
		return Execution{}, ErrInvalidCommand
	}
	record, err := s.store.Get(ctx, effectID)
	if err != nil {
		return Execution{}, errors.Join(ErrEffectUnavailable, err)
	}
	if terminal(record.State) {
		return execution(record, false), nil
	}
	if record.State != eer.StateQueued {
		return Execution{}, ErrEffectUnavailable
	}
	lease, p, err := s.runtime.Claim(ctx, eer.ClaimCommand{EffectID: effectID, WorkerDigest: workerDigest})
	if err != nil || !validProjection(p) || p.ID != effectID || p.State != eer.StateQueued {
		return Execution{}, errors.Join(ErrEffectUnavailable, err)
	}
	if _, err = s.store.RecordClaim(ctx, effectID, lease, s.now().UTC()); err != nil {
		return Execution{}, errors.Join(ErrEffectUnavailable, err)
	}
	adapter := &effectAdapter{record: record, writer: writer}
	p, receipt, runErr := s.runtime.RunAttempt(ctx, lease, adapter)
	if !terminalAttempt(p.State) || p.ID != effectID || receipt.ID == "" || receipt.EffectID != effectID {
		return Execution{}, errors.Join(ErrEffectUnavailable, runErr)
	}
	completed, err := s.store.CompleteAttempt(ctx, AttemptCompletion{EffectID: effectID, Lease: lease, State: p.State, ReceiptID: receipt.ID, Receipt: receipt.CommandDigest, ProviderCallAttempted: adapter.result.BusinessCallDispatched, RealExternalCallExecuted: adapter.result.RealExternalCallExecuted, CompletedAt: p.UpdatedAt})
	if err != nil {
		return Execution{}, errors.Join(ErrEffectUnavailable, err)
	}
	if p.State == eer.StateOutcomeUnknown {
		return execution(completed, true), nil
	}
	if runErr != nil {
		return Execution{}, errors.Join(ErrEffectUnavailable, runErr)
	}
	return execution(completed, true), nil
}

func (s *Service) Reconcile(ctx context.Context, c ReconcileCommand) (Reconciliation, error) {
	if !s.ready(ctx) || !validReconcile(c) {
		return Reconciliation{}, ErrReconcileRequired
	}
	var result Effect
	var id string
	err := s.uow.Within(ctx, func(tx context.Context) error {
		r, e := s.store.Get(tx, c.EffectID)
		if e != nil {
			return e
		}
		if r.State != eer.StateOutcomeUnknown && r.State != eer.StateReconciled {
			return ErrReconcileRequired
		}
		lease := eer.Lease{EffectID: c.EffectID, Generation: c.Generation, Fence: c.Fence, ExpiresAt: c.LeaseExpiresAt.UTC()}
		if r.Generation != lease.Generation || r.Fence != lease.Fence || !r.LeaseExpiresAt.Equal(lease.ExpiresAt) {
			return ErrReconcileRequired
		}
		p, receipt, e := s.runtime.Reconcile(tx, eer.ReconcileCommand{Lease: lease, ReceiptKeyDigest: digest("reconcile", strconv.FormatInt(c.Actor, 10), c.IdempotencyKey), EvidenceDigest: c.EvidenceDigest})
		if e != nil || p.ID != c.EffectID || p.State != eer.StateReconciled || receipt.ID == "" {
			return errors.Join(ErrReconcileRequired, e)
		}
		id = receipt.ID
		result, e = s.store.CompleteReconcile(tx, ReconcileCompletion{EffectID: c.EffectID, Lease: lease, ReceiptID: receipt.ID, Receipt: receipt.CommandDigest, EvidenceDigest: c.EvidenceDigest, Resolution: c.Resolution, CompletedAt: p.UpdatedAt})
		return e
	})
	if err != nil {
		if errors.Is(err, ErrReconcileRequired) || errors.Is(err, eer.ErrReconcileRequired) || errors.Is(err, eer.ErrLeaseFence) || errors.Is(err, eer.ErrPayloadMismatch) {
			return Reconciliation{}, ErrReconcileRequired
		}
		return Reconciliation{}, errors.Join(ErrEffectUnavailable, err)
	}
	return Reconciliation{EffectID: result.EffectID, State: result.State, Resolution: result.ReconcileResolution, ReceiptID: id, ProviderCallAttempted: result.ProviderCallAttempted, RealExternalCallExecuted: result.RealExternalCallExecuted}, nil
}

type effectAdapter struct {
	record Effect
	writer wecomport.ContactProfileWriter
	result eer.AdapterResult
}

func (a *effectAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, _ eer.Attempt) (eer.AdapterResult, error) {
	if a == nil || nilDependency(a.writer) || envelope.Fingerprint() != a.record.EnvelopeFingerprint {
		return eer.AdapterResult{}, ErrEffectConflict
	}
	result, err := a.writer.WriteContactProfile(ctx, wecomport.ContactProfileWriteRequest{CorpID: a.record.CorpID, StaffUserID: a.record.StaffUserID, ExternalUserID: a.record.ExternalUserID, Remark: a.record.Remark, Description: a.record.Description})
	a.result = result
	return result, err
}

func (s *Service) ready(ctx context.Context) bool {
	return s != nil && ctx != nil && !nilDependency(s.uow) && !nilDependency(s.store) && !nilDependency(s.runtime) && !nilDependency(s.jobs) && validCorpID(s.corpID) && s.now != nil
}
func effectEnvelope(corp string, c QueueCommand) (eer.EffectEnvelope, error) {
	return eer.NewEnvelope(eer.EnvelopeInput{Owner: ownerWeCom, Kind: kindProfile, SourceRefDigest: digest("source", strconv.FormatInt(c.LegacyReceiptID, 10), strconv.FormatInt(c.Actor, 10)), TargetRefDigest: digest("target", corp, c.StaffUserID, c.ExternalUserID), PayloadDigest: digest("payload", c.Remark, c.Description), PolicyVersionHash: digest("policy", policyV1)})
}
func effectFromCommand(corp string, c QueueCommand, p eer.Projection, receipt string, fingerprint eer.Digest, now time.Time) Effect {
	return Effect{EffectID: p.ID, LegacyReceiptID: c.LegacyReceiptID, Actor: c.Actor, CorpID: corp, StaffUserID: c.StaffUserID, ExternalUserID: c.ExternalUserID, Remark: c.Remark, Description: c.Description, IdempotencyDigest: digest("idempotency", strconv.FormatInt(c.Actor, 10), c.IdempotencyKey), EnvelopeFingerprint: fingerprint, State: p.State, AcceptReceiptID: receipt, Generation: p.Generation, UpdatedAt: now}
}
func canonicalCommand(c QueueCommand) (QueueCommand, bool) {
	if c.LegacyReceiptID < 1 || c.Actor < 1 || !validText(c.IdempotencyKey, 16, 128) || !validText(c.StaffUserID, 1, maxStaffID) || !validText(c.ExternalUserID, 1, maxExternalID) || !validText(c.Remark, 1, maxRemark) || !validOptionalText(c.Description, maxDesc) {
		return QueueCommand{}, false
	}
	return c, true
}
func sameCommand(a, b Effect) bool {
	return a.EffectID == b.EffectID && a.LegacyReceiptID == b.LegacyReceiptID && a.Actor == b.Actor && a.CorpID == b.CorpID && a.StaffUserID == b.StaffUserID && a.ExternalUserID == b.ExternalUserID && a.Remark == b.Remark && a.Description == b.Description && a.IdempotencyDigest == b.IdempotencyDigest && a.EnvelopeFingerprint == b.EnvelopeFingerprint
}
func acceptance(e Effect) (Acceptance, error) {
	if e.EffectID == "" || e.RiverJobID < 1 || e.AcceptReceiptID == "" || e.QueueReceiptID == "" {
		return Acceptance{}, ErrEffectUnavailable
	}
	return Acceptance{EffectID: e.EffectID, State: e.State, RiverJobID: e.RiverJobID, AcceptReceiptID: e.AcceptReceiptID, QueueReceiptID: e.QueueReceiptID}, nil
}
func execution(e Effect, _ bool) Execution {
	return Execution{EffectID: e.EffectID, State: e.State, AttemptReceiptDigest: e.AttemptReceiptDigest, ProviderCallAttempted: e.ProviderCallAttempted, RealExternalCallExecuted: e.RealExternalCallExecuted, ManualReconcileRequired: e.State == eer.StateOutcomeUnknown}
}
func validProjection(p eer.Projection) bool {
	return p.ID != "" && string(p.Owner) == ownerWeCom && string(p.Kind) == kindProfile && p.Generation > 0 && !p.UpdatedAt.IsZero()
}
func terminalAttempt(s eer.State) bool {
	return s == eer.StateExecuted || s == eer.StateOutcomeUnknown || s == eer.StateFinalFailed
}
func terminal(s eer.State) bool { return terminalAttempt(s) || s == eer.StateReconciled }
func validReconcile(c ReconcileCommand) bool {
	return c.EffectID != "" && c.Actor > 0 && validText(c.IdempotencyKey, 16, 128) && c.Generation > 0 && c.Fence > 0 && !c.LeaseExpiresAt.IsZero() && validDigest(c.EvidenceDigest) && (c.Resolution == ResolutionProviderApplied || c.Resolution == ResolutionProviderNotApplied)
}
func digest(label string, parts ...string) eer.Digest {
	sum := sha256.Sum256([]byte("wecom.profile." + policyV1 + "\x00" + label + "\x00" + strings.Join(parts, "\x00")))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
func validDigest(v eer.Digest) bool {
	if !strings.HasPrefix(string(v), "sha256:") || len(v) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, e := hex.DecodeString(strings.TrimPrefix(string(v), "sha256:"))
	return e == nil
}
func validCorpID(v string) bool {
	return validText(v, 1, 256) && strings.IndexFunc(v, unicode.IsSpace) < 0
}
func validText(v string, min, max int) bool {
	return len(v) >= min && len(v) <= max && strings.TrimSpace(v) == v && utf8.ValidString(v) && strings.IndexFunc(v, unicode.IsControl) < 0
}
func validOptionalText(v string, max int) bool {
	return len(v) <= max && strings.TrimSpace(v) == v && utf8.ValidString(v) && strings.IndexFunc(v, unicode.IsControl) < 0
}
func nilDependency(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	default:
		return false
	}
}
