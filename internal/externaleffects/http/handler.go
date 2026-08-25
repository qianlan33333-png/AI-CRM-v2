// Package http adapts the narrow operator controls for the external-effects runtime.
package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const maxRequestBytes int64 = 16 << 10

type Application interface {
	List(context.Context, int32) ([]eer.Projection, error)
	Detail(context.Context, string) (eer.Projection, error)
	Diagnostics(context.Context) (eer.Diagnostics, error)
	Cancel(context.Context, eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error)
	Retry(context.Context, eer.RetryCommand) (eer.Projection, eer.OperationReceipt, error)
	Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error)
}

type Handler struct{ app Application }

func NewHandler(app Application) (*Handler, error) {
	if app == nil {
		return nil, eer.ErrInvalidCommand
	}
	return &Handler{app: app}, nil
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request, limit int32) {
	if !h.valid(w, r) {
		return
	}
	if limit == 0 {
		limit = 50
	}
	items, err := h.app.List(r.Context(), limit)
	if err != nil {
		h.error(w, r, err)
		return
	}
	response := make([]projectionResponse, len(items))
	for i := range items {
		response[i] = projectionOf(items[i])
	}
	h.write(w, http.StatusOK, listResponse{Items: response})
}
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request, id string) {
	if !h.valid(w, r) {
		return
	}
	value, err := h.app.Detail(r.Context(), id)
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, projectionOf(value))
}
func (h *Handler) Diagnostics(w http.ResponseWriter, r *http.Request) {
	if !h.valid(w, r) {
		return
	}
	value, err := h.app.Diagnostics(r.Context())
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, diagnosticsOf(value))
}
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request, id string) {
	if !h.valid(w, r) {
		return
	}
	key, ok := idempotency(w, r)
	if !ok {
		return
	}
	value, _, err := h.app.Cancel(r.Context(), eer.CancelCommand{EffectID: id, ReceiptKeyDigest: key})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, projectionOf(value))
}
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request, id string) {
	if !h.valid(w, r) {
		return
	}
	key, ok := idempotency(w, r)
	if !ok {
		return
	}
	var body retryRequest
	if !decode(w, r, &body) {
		h.error(w, r, eer.ErrInvalidCommand)
		return
	}
	scheduled, err := time.Parse(time.RFC3339Nano, body.ScheduledAt)
	if err != nil {
		h.error(w, r, eer.ErrInvalidCommand)
		return
	}
	value, _, err := h.app.Retry(r.Context(), eer.RetryCommand{EffectID: id, ReceiptKeyDigest: key, Job: eer.RiverJobLink{JobID: body.JobID, Generation: body.Generation, Queue: body.Queue, ArgsDigest: eer.Digest(body.ArgsDigest), ScheduledAt: scheduled}})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, projectionOf(value))
}
func (h *Handler) Reconcile(w http.ResponseWriter, r *http.Request, id string) {
	if !h.valid(w, r) {
		return
	}
	key, ok := idempotency(w, r)
	if !ok {
		return
	}
	var body reconcileRequest
	if !decode(w, r, &body) {
		h.error(w, r, eer.ErrInvalidCommand)
		return
	}
	expires, err := time.Parse(time.RFC3339Nano, body.LeaseExpiresAt)
	if err != nil {
		h.error(w, r, eer.ErrInvalidCommand)
		return
	}
	value, _, err := h.app.Reconcile(r.Context(), eer.ReconcileCommand{Lease: eer.Lease{EffectID: id, Generation: body.Generation, Fence: body.Fence, ExpiresAt: expires}, ReceiptKeyDigest: key, EvidenceDigest: eer.Digest(body.EvidenceDigest)})
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.write(w, http.StatusOK, projectionOf(value))
}

func (h *Handler) valid(w http.ResponseWriter, r *http.Request) bool {
	if h != nil && h.app != nil && r != nil {
		return true
	}
	h.error(w, r, eer.ErrUnavailable)
	return false
}
func idempotency(w http.ResponseWriter, r *http.Request) (eer.Digest, bool) {
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 || values[0] == "" {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, eer.ErrInvalidCommand))
		return "", false
	}
	sum := sha256.Sum256([]byte(values[0]))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:])), true
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
	case errors.Is(err, eer.ErrInvalidCommand):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, eer.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, eer.ErrPayloadMismatch), errors.Is(err, eer.ErrInvalidTransition), errors.Is(err, eer.ErrLeaseExpired), errors.Is(err, eer.ErrLeaseFence), errors.Is(err, eer.ErrRetryForbidden), errors.Is(err, eer.ErrCancelForbidden), errors.Is(err, eer.ErrReconcileRequired), errors.Is(err, eer.ErrRecoveryForbidden):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(w, r, platformhttp.NewError(code, err))
}
func (h *Handler) write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type retryRequest struct {
	JobID       int64  `json:"job_id"`
	Generation  int64  `json:"generation"`
	Queue       string `json:"queue"`
	ArgsDigest  string `json:"args_digest"`
	ScheduledAt string `json:"scheduled_at"`
}
type reconcileRequest struct {
	Generation     int64  `json:"generation"`
	Fence          int64  `json:"fence"`
	LeaseExpiresAt string `json:"lease_expires_at"`
	EvidenceDigest string `json:"evidence_digest"`
}
type projectionResponse struct {
	ID           string    `json:"id"`
	Owner        eer.Owner `json:"owner"`
	Kind         eer.Kind  `json:"kind"`
	State        eer.State `json:"state"`
	AttemptCount int32     `json:"attempt_count"`
	Generation   int64     `json:"generation"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func projectionOf(value eer.Projection) projectionResponse {
	return projectionResponse{ID: value.ID, Owner: value.Owner, Kind: value.Kind, State: value.State, AttemptCount: value.AttemptCount, Generation: value.Generation, UpdatedAt: value.UpdatedAt}
}

type listResponse struct {
	Items []projectionResponse `json:"items"`
}
type diagnosticsResponse struct {
	Accepted        int64 `json:"accepted"`
	Queued          int64 `json:"queued"`
	Attempted       int64 `json:"attempted"`
	OutcomeUnknown  int64 `json:"outcome_unknown"`
	RetryableFailed int64 `json:"retryable_failed"`
}

func diagnosticsOf(value eer.Diagnostics) diagnosticsResponse {
	return diagnosticsResponse{Accepted: value.Accepted, Queued: value.Queued, Attempted: value.Attempted, OutcomeUnknown: value.OutcomeUnknown, RetryableFailed: value.RetryableFailed}
}
