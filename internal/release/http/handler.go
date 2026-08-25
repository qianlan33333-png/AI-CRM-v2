// Package releasehttp adapts the local release-plane app boundary to HTTP.
package releasehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	releaseapp "github.com/qianlan33333-png/AI-CRM-v2/internal/release/app"
	releaseport "github.com/qianlan33333-png/AI-CRM-v2/internal/release/port"
)

const maxRequestBytes int64 = 64 << 10

type Application interface {
	List(context.Context, int32) ([]releaseport.Candidate, error)
	Register(context.Context, releaseapp.RegisterCommand) (releaseport.Candidate, error)
	Detail(context.Context, int64) (releaseport.Detail, error)
	RecordPrerequisite(context.Context, releaseapp.ReceiptCommand) (releaseport.PrerequisiteReceipt, error)
	Prepare(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error)
	StartCutover(context.Context, releaseapp.CandidateCommand) (releaseport.WorkerLease, error)
	RestartCutover(context.Context, releaseapp.WorkerCommand) (releaseport.WorkerLease, error)
	CompleteStep(context.Context, releaseapp.StepCommand) (releaseport.CutoverJournalEntry, error)
	Activate(context.Context, releaseapp.WorkerCommand) (releaseport.Candidate, error)
	RecordRollbackCheck(context.Context, releaseapp.RollbackCheckCommand) (releaseport.RollbackCheck, error)
	RequestRollback(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error)
	CompleteRollback(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error)
}

type Handler struct{ application Application }

func NewHandler(application Application) (*Handler, error) {
	if application == nil {
		return nil, releaseapp.ErrUnavailable
	}
	return &Handler{application: application}, nil
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request, limit int32) {
	if !h.valid(w, r) || !h.readActor(w, r) {
		return
	}
	if limit == 0 {
		limit = 50
	}
	values, err := h.application.List(r.Context(), limit)
	if err != nil {
		h.error(w, r, err)
		return
	}
	items := make([]candidateResponse, len(values))
	for i := range values {
		items[i] = candidateOf(values[i])
	}
	h.write(w, http.StatusOK, candidateListResponse{Items: items})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.manageActor(w, r)
	if !ok {
		return
	}
	var body registerRequest
	if !decode(w, r, &body) {
		h.error(w, r, releaseapp.ErrInvalidCommand)
		return
	}
	value, err := h.application.Register(r.Context(), releaseapp.RegisterCommand{CommitSHA: body.CommitSHA, ArtifactDigest: body.ArtifactDigest, ManifestDigest: body.ManifestDigest, ConfigDigest: body.ConfigDigest, TargetSchemaVersion: body.TargetSchemaVersion, ActorID: actorID, IdempotencyKey: key(r)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusCreated, candidateOf(value))
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request, candidateID int64) {
	if !h.valid(w, r) || !h.readActor(w, r) {
		return
	}
	value, err := h.application.Detail(r.Context(), candidateID)
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, detailOf(value))
}

func (h *Handler) RecordPrerequisite(w http.ResponseWriter, r *http.Request, candidateID int64) {
	actorID, ok := h.manageActor(w, r)
	if !ok {
		return
	}
	var body prerequisiteRequest
	if !decode(w, r, &body) {
		h.error(w, r, releaseapp.ErrInvalidCommand)
		return
	}
	value, err := h.application.RecordPrerequisite(r.Context(), releaseapp.ReceiptCommand{CandidateID: candidateID, ActorID: actorID, Kind: releaseport.PrerequisiteKind(body.Kind), EvidenceSHA: body.EvidenceSHA, IdempotencyKey: key(r)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusCreated, prerequisiteOf(value))
}

func (h *Handler) Prepare(w http.ResponseWriter, r *http.Request, candidateID int64) {
	h.candidateMutation(w, r, candidateID, h.application.Prepare)
}
func (h *Handler) RequestRollback(w http.ResponseWriter, r *http.Request, candidateID int64) {
	h.candidateMutation(w, r, candidateID, h.application.RequestRollback)
}
func (h *Handler) CompleteRollback(w http.ResponseWriter, r *http.Request, candidateID int64) {
	h.candidateMutation(w, r, candidateID, h.application.CompleteRollback)
}

func (h *Handler) candidateMutation(w http.ResponseWriter, r *http.Request, candidateID int64, operation func(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error)) {
	actorID, ok := h.manageActor(w, r)
	if !ok {
		return
	}
	value, err := operation(r.Context(), releaseapp.CandidateCommand{CandidateID: candidateID, ActorID: actorID, IdempotencyKey: key(r)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, candidateOf(value))
}

func (h *Handler) StartCutover(w http.ResponseWriter, r *http.Request, candidateID int64) {
	actorID, ok := h.manageActor(w, r)
	if !ok {
		return
	}
	value, err := h.application.StartCutover(r.Context(), releaseapp.CandidateCommand{CandidateID: candidateID, ActorID: actorID, IdempotencyKey: key(r)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, leaseOf(value))
}

func (h *Handler) RestartCutover(w http.ResponseWriter, r *http.Request, candidateID int64) {
	actorID, command, ok := h.workerCommand(w, r, candidateID)
	if !ok {
		return
	}
	value, err := h.application.RestartCutover(r.Context(), releaseapp.WorkerCommand{CandidateID: candidateID, ActorID: actorID, Generation: command.Generation, Fence: command.Fence, IdempotencyKey: key(r)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, leaseOf(value))
}

func (h *Handler) CompleteStep(w http.ResponseWriter, r *http.Request, candidateID int64, step string) {
	actorID, command, ok := h.workerCommand(w, r, candidateID)
	if !ok {
		return
	}
	value, err := h.application.CompleteStep(r.Context(), releaseapp.StepCommand{CandidateID: candidateID, ActorID: actorID, Generation: command.Generation, Fence: command.Fence, Step: releaseport.CutoverStep(step), IdempotencyKey: key(r)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, progressOfJournal(value))
}

func (h *Handler) Activate(w http.ResponseWriter, r *http.Request, candidateID int64) {
	actorID, command, ok := h.workerCommand(w, r, candidateID)
	if !ok {
		return
	}
	value, err := h.application.Activate(r.Context(), releaseapp.WorkerCommand{CandidateID: candidateID, ActorID: actorID, Generation: command.Generation, Fence: command.Fence, IdempotencyKey: key(r)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, candidateOf(value))
}

func (h *Handler) RecordRollbackCheck(w http.ResponseWriter, r *http.Request, candidateID int64) {
	actorID, ok := h.manageActor(w, r)
	if !ok {
		return
	}
	var body rollbackCheckRequest
	if !decode(w, r, &body) {
		h.error(w, r, releaseapp.ErrInvalidCommand)
		return
	}
	value, err := h.application.RecordRollbackCheck(r.Context(), releaseapp.RollbackCheckCommand{CandidateID: candidateID, ActorID: actorID, Kind: releaseport.RollbackCheckKind(body.Kind), Passed: body.Passed, EvidenceSHA: body.EvidenceSHA, IdempotencyKey: key(r)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusCreated, rollbackCheckOf(value))
}

func (h *Handler) workerCommand(w http.ResponseWriter, r *http.Request, candidateID int64) (int64, workerCommandRequest, bool) {
	actorID, ok := h.manageActor(w, r)
	if !ok {
		return 0, workerCommandRequest{}, false
	}
	var command workerCommandRequest
	if candidateID < 1 || !decode(w, r, &command) {
		h.error(w, r, releaseapp.ErrInvalidCommand)
		return 0, workerCommandRequest{}, false
	}
	return actorID, command, true
}

func (h *Handler) valid(w http.ResponseWriter, r *http.Request) bool {
	if h != nil && h.application != nil && r != nil && r.URL != nil {
		return true
	}
	h.error(w, r, releaseapp.ErrUnavailable)
	return false
}
func (h *Handler) readActor(w http.ResponseWriter, r *http.Request) bool {
	_, ok := actor(r.Context(), authport.CapabilityReleaseRead, true)
	if !ok {
		h.error(w, r, authport.ErrUnauthorized)
	}
	return ok
}
func (h *Handler) manageActor(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if !h.valid(w, r) {
		return 0, false
	}
	actorID, ok := actor(r.Context(), authport.CapabilityReleaseManage, false)
	if !ok {
		h.error(w, r, authport.ErrUnauthorized)
		return 0, false
	}
	if key(r) == "" {
		h.error(w, r, releaseapp.ErrInvalidCommand)
		return 0, false
	}
	return actorID, true
}
func actor(ctx context.Context, capability authport.Capability, allowOps bool) (int64, bool) {
	principal, principalOK := authport.PrincipalFromContext(ctx)
	authorization, authorizationOK := authport.AuthorizationFromContext(ctx)
	if !principalOK || principal.AdminUserID < 1 || !authorizationOK || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return 0, false
	}
	if principal.Role != authport.RoleAdmin && (!allowOps || principal.Role != authport.RoleOps) {
		return 0, false
	}
	return principal.AdminUserID, true
}
func key(r *http.Request) string {
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 {
		return ""
	}
	return values[0]
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(&struct{}{}) == io.EOF
}
func (h *Handler) error(w http.ResponseWriter, r *http.Request, err error) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, authport.ErrUnauthorized):
		code = platformhttp.CodeUnauthorized
	case errors.Is(err, releaseport.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, releaseport.ErrConflict), errors.Is(err, releaseapp.ErrNotReady), errors.Is(err, releaseapp.ErrInvalidState), errors.Is(err, releaseapp.ErrFence):
		code = platformhttp.CodeConflict
	case errors.Is(err, releaseapp.ErrInvalidCommand):
		code = platformhttp.CodeMalformedRequest
	}
	platformhttp.WriteError(w, r, platformhttp.NewError(code, err))
}
func (h *Handler) write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type registerRequest struct {
	CommitSHA           string `json:"commit_sha"`
	ArtifactDigest      string `json:"artifact_digest"`
	ManifestDigest      string `json:"manifest_digest"`
	ConfigDigest        string `json:"config_digest"`
	TargetSchemaVersion int64  `json:"target_schema_version"`
}
type prerequisiteRequest struct {
	Kind        string `json:"kind"`
	EvidenceSHA string `json:"evidence_sha"`
}
type workerCommandRequest struct {
	Generation int64  `json:"generation"`
	Fence      string `json:"fence"`
}
type rollbackCheckRequest struct {
	Kind        string `json:"kind"`
	Passed      bool   `json:"passed"`
	EvidenceSHA string `json:"evidence_sha"`
}
type candidateResponse struct {
	ID                  int64                      `json:"id"`
	CommitSHA           string                     `json:"commit_sha"`
	ArtifactDigest      string                     `json:"artifact_digest"`
	ManifestDigest      string                     `json:"manifest_digest"`
	ConfigDigest        string                     `json:"config_digest"`
	TargetSchemaVersion int64                      `json:"target_schema_version"`
	State               releaseport.CandidateState `json:"state"`
	CreatedBy           int64                      `json:"created_by"`
	CreatedAt           time.Time                  `json:"created_at"`
	PreparedAt          *time.Time                 `json:"prepared_at,omitempty"`
	ActivatedAt         *time.Time                 `json:"activated_at,omitempty"`
	RollbackRequestedAt *time.Time                 `json:"rollback_requested_at,omitempty"`
	RolledBackAt        *time.Time                 `json:"rolled_back_at,omitempty"`
}

func candidateOf(v releaseport.Candidate) candidateResponse {
	return candidateResponse{ID: v.ID, CommitSHA: v.CommitSHA, ArtifactDigest: v.ArtifactDigest, ManifestDigest: v.ManifestDigest, ConfigDigest: v.ConfigDigest, TargetSchemaVersion: v.TargetSchemaVersion, State: v.State, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt, PreparedAt: v.PreparedAt, ActivatedAt: v.ActivatedAt, RollbackRequestedAt: v.RollbackRequestedAt, RolledBackAt: v.RolledBackAt}
}

type candidateListResponse struct {
	Items []candidateResponse `json:"items"`
}
type subjectResponse struct {
	CandidateID         int64  `json:"candidate_id"`
	CommitSHA           string `json:"commit_sha"`
	ArtifactDigest      string `json:"artifact_digest"`
	ManifestDigest      string `json:"manifest_digest"`
	ConfigDigest        string `json:"config_digest"`
	TargetSchemaVersion int64  `json:"target_schema_version"`
}
type prerequisiteResponse struct {
	ID          int64                        `json:"id"`
	Subject     subjectResponse              `json:"subject"`
	Kind        releaseport.PrerequisiteKind `json:"kind"`
	EvidenceSHA string                       `json:"evidence_sha"`
	RecordedBy  int64                        `json:"recorded_by"`
	RecordedAt  time.Time                    `json:"recorded_at"`
}

func prerequisiteOf(v releaseport.PrerequisiteReceipt) prerequisiteResponse {
	return prerequisiteResponse{ID: v.ID, Subject: subjectResponse{CandidateID: v.CandidateID, CommitSHA: v.CandidateCommitSHA, ArtifactDigest: v.CandidateArtifactDigest, ManifestDigest: v.CandidateManifestDigest, ConfigDigest: v.CandidateConfigDigest, TargetSchemaVersion: v.CandidateSchemaVersion}, Kind: v.Kind, EvidenceSHA: v.EvidenceSHA, RecordedBy: v.RecordedBy, RecordedAt: v.RecordedAt}
}

type leaseResponse struct {
	CandidateID int64     `json:"candidate_id"`
	Generation  int64     `json:"generation"`
	Fence       string    `json:"fence"`
	StartedBy   int64     `json:"started_by"`
	StartedAt   time.Time `json:"started_at"`
}

func leaseOf(v releaseport.WorkerLease) leaseResponse {
	return leaseResponse{CandidateID: v.CandidateID, Generation: v.Generation, Fence: v.Fence, StartedBy: v.StartedBy, StartedAt: v.StartedAt}
}

type progressResponse struct {
	ID          int64                   `json:"id"`
	CandidateID int64                   `json:"candidate_id"`
	Generation  int64                   `json:"generation"`
	Step        releaseport.CutoverStep `json:"step"`
	CompletedBy int64                   `json:"completed_by"`
	CompletedAt time.Time               `json:"completed_at"`
}

func progressOfJournal(v releaseport.CutoverJournalEntry) progressResponse {
	return progressResponse{ID: v.ID, CandidateID: v.CandidateID, Generation: v.Generation, Step: v.Step, CompletedBy: v.CompletedBy, CompletedAt: v.CompletedAt}
}
func progressOf(v releaseport.CutoverProgressEntry) progressResponse {
	return progressResponse{ID: v.ID, CandidateID: v.CandidateID, Generation: v.Generation, Step: v.Step, CompletedBy: v.CompletedBy, CompletedAt: v.CompletedAt}
}

type rollbackCheckResponse struct {
	ID          int64                         `json:"id"`
	CandidateID int64                         `json:"candidate_id"`
	Kind        releaseport.RollbackCheckKind `json:"kind"`
	Passed      bool                          `json:"passed"`
	EvidenceSHA string                        `json:"evidence_sha"`
	RecordedBy  int64                         `json:"recorded_by"`
	RecordedAt  time.Time                     `json:"recorded_at"`
}

func rollbackCheckOf(v releaseport.RollbackCheck) rollbackCheckResponse {
	return rollbackCheckResponse{ID: v.ID, CandidateID: v.CandidateID, Kind: v.Kind, Passed: v.Passed, EvidenceSHA: v.EvidenceSHA, RecordedBy: v.RecordedBy, RecordedAt: v.RecordedAt}
}

type readinessResponse struct {
	CandidateID int64                          `json:"candidate_id"`
	Ready       bool                           `json:"ready"`
	Missing     []releaseport.PrerequisiteKind `json:"missing"`
	Invalid     []releaseport.PrerequisiteKind `json:"invalid"`
	CheckedAt   time.Time                      `json:"checked_at"`
}

func readinessOf(v releaseport.Readiness) readinessResponse {
	return readinessResponse{CandidateID: v.CandidateID, Ready: v.Ready, Missing: v.Missing, Invalid: v.Invalid, CheckedAt: v.CheckedAt}
}

type rollbackEligibilityResponse struct {
	CandidateID int64                           `json:"candidate_id"`
	Eligible    bool                            `json:"eligible"`
	Missing     []releaseport.RollbackCheckKind `json:"missing"`
	Blocked     []releaseport.RollbackCheckKind `json:"blocked"`
	CheckedAt   time.Time                       `json:"checked_at"`
}

func rollbackEligibilityOf(v releaseport.RollbackEligibility) rollbackEligibilityResponse {
	return rollbackEligibilityResponse{CandidateID: v.CandidateID, Eligible: v.Eligible, Missing: v.Missing, Blocked: v.Blocked, CheckedAt: v.CheckedAt}
}

type workerSummaryResponse struct {
	CandidateID int64     `json:"candidate_id"`
	Generation  int64     `json:"generation"`
	StartedBy   int64     `json:"started_by"`
	StartedAt   time.Time `json:"started_at"`
}
type detailResponse struct {
	Candidate           candidateResponse           `json:"candidate"`
	Prerequisites       []prerequisiteResponse      `json:"prerequisites"`
	Readiness           readinessResponse           `json:"readiness"`
	CutoverProgress     []progressResponse          `json:"cutover_progress"`
	RollbackChecks      []rollbackCheckResponse     `json:"rollback_checks"`
	RollbackEligibility rollbackEligibilityResponse `json:"rollback_eligibility"`
	ActiveWorker        *workerSummaryResponse      `json:"active_worker,omitempty"`
}

func detailOf(v releaseport.Detail) detailResponse {
	r := detailResponse{Candidate: candidateOf(v.Candidate), Prerequisites: make([]prerequisiteResponse, len(v.Prerequisites)), Readiness: readinessOf(v.Readiness), CutoverProgress: make([]progressResponse, len(v.CutoverProgress)), RollbackChecks: make([]rollbackCheckResponse, len(v.RollbackChecks)), RollbackEligibility: rollbackEligibilityOf(v.RollbackEligibility)}
	for i := range v.Prerequisites {
		r.Prerequisites[i] = prerequisiteOf(v.Prerequisites[i])
	}
	for i := range v.CutoverProgress {
		r.CutoverProgress[i] = progressOf(v.CutoverProgress[i])
	}
	for i := range v.RollbackChecks {
		r.RollbackChecks[i] = rollbackCheckOf(v.RollbackChecks[i])
	}
	if v.ActiveWorker != nil {
		r.ActiveWorker = &workerSummaryResponse{CandidateID: v.ActiveWorker.CandidateID, Generation: v.ActiveWorker.Generation, StartedBy: v.ActiveWorker.StartedBy, StartedAt: v.ActiveWorker.StartedAt}
	}
	return r
}
