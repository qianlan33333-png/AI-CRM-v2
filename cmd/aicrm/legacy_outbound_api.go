package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

var errInvalidLegacyOutboundQuery = errors.New("legacy outbound query cannot be mapped safely")

const legacyOutboundDeliverySemantics = "local_state_not_delivery_proof"

type legacyOutboundQueryApplication interface {
	Get(context.Context, outboundapp.TaskGetQuery) (outboundapp.TaskReadModel, error)
	List(context.Context, outboundapp.TaskListQuery) (outboundapp.TaskListResult, error)
	Reconcile(context.Context, outboundapp.TaskGetQuery) (outboundapp.TaskReconciliation, error)
}

type legacyCancelApplication interface {
	Cancel(context.Context, outboundapp.CancelCommand) (outboundapp.CancelledTask, error)
}

type legacyRetryApplication interface {
	Retry(context.Context, outboundapp.ManualRetryCommand) (outboundapp.ManualRetryResult, error)
}

func (handler *Handler) ListOutboundJobs(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.outbound) || request == nil {
		legacyOutboundError(writer, request, outboundapp.ErrTaskQueryUnavailable)
		return
	}
	query, err := legacyOutboundListQuery(request)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	query.OwnerStaffID, err = legacyOutboundOwner(request.Context(), query.OwnerStaffID)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	result, err := handler.outbound.List(request.Context(), query)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	jobs := make([]legacyOutboundJob, len(result.Items))
	for index, task := range result.Items {
		jobs[index] = mapLegacyOutboundJob(task)
	}
	writeLegacyOutboundJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "jobs": jobs, "items": jobs, "count": len(jobs), "has_more": result.HasMore,
		"limit": query.Limit, "offset": query.Offset, "source_status": "v2_outbound_service", "fallback_used": false,
	})
}

func (handler *Handler) GetOutboundJob(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.outbound) || request == nil {
		legacyOutboundError(writer, request, outboundapp.ErrTaskQueryUnavailable)
		return
	}
	query, err := legacyOutboundGetQuery(request)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	query.OwnerStaffID, err = legacyOutboundOwner(request.Context(), nil)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	task, err := handler.outbound.Get(request.Context(), query)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	writeLegacyOutboundJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "job": mapLegacyOutboundJob(task), "source_status": "v2_outbound_service", "fallback_used": false,
	})
}

func (handler *Handler) ReconcileOutboundJob(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.outbound) || request == nil {
		legacyOutboundError(writer, request, outboundapp.ErrTaskQueryUnavailable)
		return
	}
	query, err := legacyOutboundGetQuery(request)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	query.OwnerStaffID, err = legacyOutboundOwner(request.Context(), nil)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	result, err := handler.outbound.Reconcile(request.Context(), query)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	attempts := make([]legacyOutboundAttempt, len(result.Attempts))
	for index, attempt := range result.Attempts {
		attempts[index] = mapLegacyOutboundAttempt(attempt)
	}
	receipts := make([]legacyOutboundControlReceipt, len(result.ControlReceipts))
	for index, receipt := range result.ControlReceipts {
		receipts[index] = mapLegacyOutboundReceipt(receipt)
	}
	writeLegacyOutboundJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "job": mapLegacyOutboundJob(result.Task), "attempts": attempts,
		"control_receipts": receipts, "source_status": "v2_outbound_service", "fallback_used": false,
	})
}

func (handler *Handler) CancelOutboundJob(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.cancel) || request == nil {
		legacyOutboundError(writer, request, outboundapp.ErrCancelFailed)
		return
	}
	taskID, principal, key, err := legacyOutboundControlInput(request)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	result, err := handler.cancel.Cancel(request.Context(), outboundapp.CancelCommand{
		TaskID: taskID, IdempotencyScope: "legacy-admin:" + strconv.FormatInt(principal.AdminUserID, 10), IdempotencyKey: key,
	})
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	receipt := localLegacyOutboundReceipt(legacyOutboundControlReceipt{ID: result.ReceiptID, TaskID: int64(result.TaskID),
		Operation: "cancel", State: string(outboundapp.ControlReceiptCompleted), Generation: result.Job.Generation,
		RiverJobID: result.Job.RiverJobID, JobKind: result.Job.JobKind, EventID: int64(result.EventID),
		TaskStatus: string(result.Status), CompletedAt: result.CancelledAt.UTC()})
	writeLegacyOutboundJSON(writer, http.StatusAccepted, map[string]any{
		"ok": true, "control_receipt": receipt, "source_status": "v2_outbound_cancel_service", "fallback_used": false,
	})
}

func (handler *Handler) RetryOutboundJob(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.manualRetry) || request == nil {
		legacyOutboundError(writer, request, outboundapp.ErrManualRetryFailed)
		return
	}
	taskID, principal, key, err := legacyOutboundControlInput(request)
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	result, err := handler.manualRetry.Retry(request.Context(), outboundapp.ManualRetryCommand{
		TaskID: taskID, IdempotencyScope: "legacy-admin:" + strconv.FormatInt(principal.AdminUserID, 10), IdempotencyKey: key,
	})
	if err != nil {
		legacyOutboundError(writer, request, err)
		return
	}
	receipt := localLegacyOutboundReceipt(legacyOutboundControlReceipt{ID: result.ReceiptID, TaskID: int64(result.TaskID),
		Operation: "manual_retry", State: string(outboundapp.ControlReceiptCompleted), Generation: result.Job.Generation,
		RiverJobID: result.Job.RiverJobID, JobKind: result.Job.JobKind, EventID: int64(result.EventID),
		TaskStatus: string(result.Status), CompletedAt: result.RetriedAt.UTC()})
	writeLegacyOutboundJSON(writer, http.StatusAccepted, map[string]any{
		"ok": true, "control_receipt": receipt, "source_status": "v2_outbound_manual_retry_service", "fallback_used": false,
	})
}

func legacyOutboundListQuery(request *http.Request) (outboundapp.TaskListQuery, error) {
	values := request.URL.Query()
	for _, key := range []string{"section", "effect_type", "target_type", "target_id", "external_userid",
		"trace_id", "idempotency_key", "source_module", "source_route", "created_from", "created_to", "cursor"} {
		if strings.TrimSpace(values.Get(key)) != "" {
			return outboundapp.TaskListQuery{}, errInvalidLegacyOutboundQuery
		}
	}
	query := outboundapp.TaskListQuery{Limit: 50}
	if raw := strings.TrimSpace(values.Get("business_id")); raw != "" {
		batchID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || batchID < 1 {
			return outboundapp.TaskListQuery{}, errInvalidLegacyOutboundQuery
		}
		businessType := strings.TrimSpace(values.Get("business_type"))
		if businessType != "" && businessType != "outbound_batch" {
			return outboundapp.TaskListQuery{}, errInvalidLegacyOutboundQuery
		}
		query.BatchID = &batchID
	} else if strings.TrimSpace(values.Get("business_type")) != "" {
		return outboundapp.TaskListQuery{}, errInvalidLegacyOutboundQuery
	}
	if raw := strings.TrimSpace(values.Get("status")); raw != "" {
		query.Status = outboundapp.TaskStatus(raw)
	}
	if raw := strings.TrimSpace(values.Get("owner_userid")); raw != "" {
		owner, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || owner < 1 {
			return outboundapp.TaskListQuery{}, errInvalidLegacyOutboundQuery
		}
		query.OwnerStaffID = &owner
	}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || limit < 1 || limit > int64(outboundapp.TaskQueryMaximumLimit) {
			return outboundapp.TaskListQuery{}, errInvalidLegacyOutboundQuery
		}
		query.Limit = int32(limit)
	}
	if raw := strings.TrimSpace(values.Get("offset")); raw != "" {
		offset, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || offset < 0 {
			return outboundapp.TaskListQuery{}, errInvalidLegacyOutboundQuery
		}
		query.Offset = int32(offset)
	}
	return query, nil
}

func legacyOutboundGetQuery(request *http.Request) (outboundapp.TaskGetQuery, error) {
	taskID, err := strconv.ParseInt(chi.URLParam(request, "job_id"), 10, 64)
	if err != nil || taskID < 1 {
		return outboundapp.TaskGetQuery{}, errInvalidLegacyOutboundQuery
	}
	return outboundapp.TaskGetQuery{TaskID: outboundapp.TaskID(taskID)}, nil
}

func legacyOutboundOwner(ctx context.Context, requested *int64) (*int64, error) {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	if !ok || authorization.Capability != authport.CapabilityOutboundRead {
		return nil, authport.ErrUnauthorized
	}
	return legacyOwnerScope(authorization, requested)
}

func legacyOutboundControlInput(request *http.Request) (outboundapp.TaskID, authport.Principal, string, error) {
	query, err := legacyOutboundGetQuery(request)
	if err != nil {
		return 0, authport.Principal{}, "", err
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityOutboundControl || authorization.Scope != authport.ScopeGlobal ||
		authorization.OwnerStaffID != 0 || !principalOK || principal.AdminUserID < 1 {
		return 0, authport.Principal{}, "", authport.ErrUnauthorized
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || strings.TrimSpace(keys[0]) != keys[0] || len(keys[0]) < 16 || len(keys[0]) > 128 {
		return 0, authport.Principal{}, "", errInvalidLegacyOutboundQuery
	}
	return query.TaskID, principal, keys[0], nil
}

func legacyOutboundError(writer http.ResponseWriter, request *http.Request, err error) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, errInvalidLegacyOutboundQuery), errors.Is(err, outboundapp.ErrInvalidTaskQuery),
		errors.Is(err, outboundapp.ErrInvalidCancelCommand), errors.Is(err, outboundapp.ErrInvalidManualRetryCommand):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, authport.ErrUnauthorized):
		code = platformhttp.CodeUnauthorized
	case errors.Is(err, outboundapp.ErrTaskNotFound), errors.Is(err, outboundapp.ErrCancelTaskNotFound),
		errors.Is(err, outboundapp.ErrManualRetryTaskNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, outboundapp.ErrCancelCommandConflict), errors.Is(err, outboundapp.ErrCancelTransitionConflict),
		errors.Is(err, outboundapp.ErrCancelWorkerWon), errors.Is(err, outboundapp.ErrManualRetryCommandConflict),
		errors.Is(err, outboundapp.ErrManualRetryTransitionConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

type legacyOutboundJob struct {
	JobID                    int64                  `json:"job_id"`
	TaskID                   int64                  `json:"task_id"`
	CustomerID               int64                  `json:"customer_id"`
	OwnerStaffID             *int64                 `json:"owner_staff_id,omitempty"`
	BusinessID               *int64                 `json:"business_id,omitempty"`
	BatchChunkIndex          *int32                 `json:"batch_chunk_index,omitempty"`
	Status                   string                 `json:"status"`
	AttemptCount             int32                  `json:"attempt_count"`
	FailurePresent           bool                   `json:"failure_present"`
	FailureClass             string                 `json:"failure_class"`
	ProviderReceiptPresent   bool                   `json:"provider_receipt_present"`
	DeliveryProven           bool                   `json:"delivery_proven"`
	LocalFactOnly            bool                   `json:"local_fact_only"`
	RealExternalCallExecuted bool                   `json:"real_external_call_executed"`
	DeliverySemantics        string                 `json:"delivery_semantics"`
	QueueJob                 legacyOutboundQueueJob `json:"queue_job"`
	CreatedAt                time.Time              `json:"created_at"`
	StatusUpdatedAt          time.Time              `json:"status_updated_at"`
}
type legacyOutboundQueueJob struct {
	RiverJobID int64  `json:"river_job_id"`
	Generation int32  `json:"generation"`
	Kind       string `json:"kind"`
}

func mapLegacyOutboundJob(task outboundapp.TaskReadModel) legacyOutboundJob {
	failurePresent, failureClass := legacyOutboundFailureProjection(string(task.Status), task.LastFailureKind != "" || strings.TrimSpace(task.LastError) != "")
	job := legacyOutboundJob{JobID: int64(task.TaskID), TaskID: int64(task.TaskID), CustomerID: task.CustomerID,
		OwnerStaffID: task.OwnerStaffID, BusinessID: task.BatchID, BatchChunkIndex: task.BatchChunkIndex,
		Status: string(task.Status), AttemptCount: task.AttemptCount, FailurePresent: failurePresent, FailureClass: failureClass,
		ProviderReceiptPresent: strings.TrimSpace(task.ProviderMessageID) != "", DeliveryProven: false,
		LocalFactOnly: true, RealExternalCallExecuted: false, DeliverySemantics: legacyOutboundDeliverySemantics,
		QueueJob:  legacyOutboundQueueJob{RiverJobID: task.Job.RiverJobID, Generation: task.Job.Generation, Kind: task.Job.JobKind},
		CreatedAt: task.CreatedAt.UTC(), StatusUpdatedAt: task.StatusUpdatedAt.UTC()}
	return job
}

type legacyOutboundAttempt struct {
	AttemptID                int64      `json:"attempt_id"`
	HistoryID                int64      `json:"history_id"`
	Generation               int32      `json:"generation"`
	RiverJobID               int64      `json:"river_job_id"`
	Attempt                  int32      `json:"attempt"`
	MaxAttempts              int32      `json:"max_attempts"`
	State                    string     `json:"state"`
	FailurePresent           bool       `json:"failure_present"`
	FailureClass             string     `json:"failure_class"`
	ProviderReceiptPresent   bool       `json:"provider_receipt_present"`
	DeliveryProven           bool       `json:"delivery_proven"`
	LocalFactOnly            bool       `json:"local_fact_only"`
	RealExternalCallExecuted bool       `json:"real_external_call_executed"`
	DeliverySemantics        string     `json:"delivery_semantics"`
	DispatchStartedAt        *time.Time `json:"dispatch_started_at,omitempty"`
	CompletedAt              *time.Time `json:"completed_at,omitempty"`
}

func mapLegacyOutboundAttempt(attempt outboundapp.AttemptReadModel) legacyOutboundAttempt {
	failurePresent, failureClass := legacyOutboundFailureProjection(string(attempt.State), attempt.FailureKind != "" || strings.TrimSpace(attempt.ProviderCode) != "")
	result := legacyOutboundAttempt{AttemptID: attempt.ID, HistoryID: attempt.HistoryID, Generation: attempt.Generation,
		RiverJobID: attempt.RiverJobID, Attempt: attempt.RiverAttempt, MaxAttempts: attempt.RiverMaxAttempts,
		State: string(attempt.State), FailurePresent: failurePresent, FailureClass: failureClass,
		ProviderReceiptPresent: strings.TrimSpace(attempt.ProviderMessageID) != "", DeliveryProven: false,
		LocalFactOnly: true, RealExternalCallExecuted: false, DeliverySemantics: legacyOutboundDeliverySemantics,
		DispatchStartedAt: attempt.DispatchStartedAt, CompletedAt: attempt.CompletedAt}
	return result
}

type legacyOutboundControlReceipt struct {
	ID                       int64     `json:"receipt_id"`
	TaskID                   int64     `json:"task_id"`
	Operation                string    `json:"operation"`
	State                    string    `json:"state"`
	Generation               int32     `json:"generation"`
	RiverJobID               int64     `json:"river_job_id"`
	JobKind                  string    `json:"job_kind"`
	EventID                  int64     `json:"event_id"`
	TaskStatus               string    `json:"task_status"`
	CompletedAt              time.Time `json:"completed_at"`
	ProviderReceiptPresent   bool      `json:"provider_receipt_present"`
	DeliveryProven           bool      `json:"delivery_proven"`
	LocalFactOnly            bool      `json:"local_fact_only"`
	RealExternalCallExecuted bool      `json:"real_external_call_executed"`
	DeliverySemantics        string    `json:"delivery_semantics"`
}

func mapLegacyOutboundReceipt(receipt outboundapp.ControlReceiptReadModel) legacyOutboundControlReceipt {
	return localLegacyOutboundReceipt(legacyOutboundControlReceipt{ID: receipt.ID, TaskID: int64(receipt.TaskID), Operation: receipt.Operation,
		State: string(receipt.State), Generation: receipt.Job.Generation, RiverJobID: receipt.Job.RiverJobID,
		JobKind: receipt.Job.JobKind, EventID: int64(receipt.EventID), TaskStatus: string(receipt.TaskStatus),
		CompletedAt: receipt.CompletedAt.UTC()})
}

func localLegacyOutboundReceipt(receipt legacyOutboundControlReceipt) legacyOutboundControlReceipt {
	receipt.ProviderReceiptPresent = false
	receipt.DeliveryProven = false
	receipt.LocalFactOnly = true
	receipt.RealExternalCallExecuted = false
	receipt.DeliverySemantics = legacyOutboundDeliverySemantics
	return receipt
}

func legacyOutboundFailureProjection(state string, rawPresent bool) (bool, string) {
	if state == string(outboundapp.TaskStatusOutcomeUnknown) || state == string(outboundapp.SendAttemptOutcomeUnknown) {
		return true, "outcome_unknown"
	}
	if rawPresent || state == string(outboundapp.TaskStatusRetryableFailed) || state == string(outboundapp.TaskStatusFinalFailed) ||
		state == string(outboundapp.SendAttemptRetryableFailed) || state == string(outboundapp.SendAttemptFinalFailed) {
		return true, "local_failure"
	}
	return false, "none"
}

func writeLegacyOutboundJSON(writer http.ResponseWriter, status int, payload map[string]any) {
	payload["local_fact_only"] = true
	payload["real_external_call_executed"] = false
	payload["delivery_semantics"] = legacyOutboundDeliverySemantics
	writeJSON(writer, status, payload)
}
