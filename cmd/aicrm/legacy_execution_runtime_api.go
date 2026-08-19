package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const executionRuntimeRequiredCapability = "admin_read"

// emptyExecutionRuntimeReader records the explicitly supported absence of the
// control plane. It is a local read adapter: it never calls a worker, provider,
// or another domain, and an absent control remains a successful ok:false read.
type emptyExecutionRuntimeReader struct{}

func (emptyExecutionRuntimeReader) ReadExecutionRuntime(context.Context) (adminopsport.RuntimeSnapshot, error) {
	return adminopsport.RuntimeSnapshot{ObservedAt: time.Now().UTC()}, nil
}

func (emptyExecutionRuntimeReader) ReadExecutionTimeline(context.Context, string) (adminopsport.ExecutionTimeline, bool, error) {
	return adminopsport.ExecutionTimeline{}, false, nil
}

func (handler *Handler) ExecutionRuntime(writer http.ResponseWriter, request *http.Request) {
	if !executionRuntimeAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if handler == nil || handler.executionRuntime == nil {
		writeExecutionRuntimeError(writer, http.StatusServiceUnavailable, adminopsapp.ErrExecutionRuntimeUnavailable)
		return
	}
	runtime, err := handler.executionRuntime.Runtime(request.Context())
	if err != nil {
		writeExecutionRuntimeError(writer, http.StatusServiceUnavailable, adminopsapp.ErrExecutionRuntimeUnavailable)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": runtime.OK, "control": executionRuntimeControl(runtime.Control),
		"observations": executionRuntimeObservations(runtime.Observations), "truncated": runtime.Truncated,
		"observed_at": runtime.ObservedAt.UTC(), "observed_only": true,
		"real_external_call_executed": false,
	})
}

func (handler *Handler) ExecutionTimeline(writer http.ResponseWriter, request *http.Request) {
	if !executionRuntimeAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if handler == nil || handler.executionRuntime == nil {
		writeExecutionRuntimeError(writer, http.StatusServiceUnavailable, adminopsapp.ErrExecutionTimelineUnavailable)
		return
	}
	timeline, err := handler.executionRuntime.Timeline(request.Context(), chi.URLParam(request, "execution_id"))
	switch {
	case errors.Is(err, adminopsapp.ErrExecutionNotFound):
		writeExecutionRuntimeError(writer, http.StatusNotFound, adminopsapp.ErrExecutionNotFound)
		return
	case err != nil:
		writeExecutionRuntimeError(writer, http.StatusServiceUnavailable, adminopsapp.ErrExecutionTimelineUnavailable)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"execution_id": timeline.ExecutionID, "graph": executionRuntimeGraph(timeline.Graph),
		"observed_at": timeline.ObservedAt.UTC(), "observed_only": true,
		"real_external_call_executed": false,
	})
}

func executionRuntimeAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	return ok && authorization.Capability == authport.CapabilityAdminRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func writeExecutionRuntimeError(writer http.ResponseWriter, status int, err error) {
	code := platformhttp.CodeDependencyUnavailable
	if status == http.StatusNotFound {
		code = platformhttp.CodeNotFound
	}
	platformhttp.MarkCompatibilityError(writer, code)
	writeJSON(writer, status, map[string]any{"ok": false, "error": err.Error(), "real_external_call_executed": false})
}

func executionRuntimeControl(control *adminopsport.RuntimeControl) any {
	if control == nil {
		return nil
	}
	return map[string]any{"name": control.Name, "state": control.State, "details": executionRuntimeDetails(control.Details), "observed_at": control.ObservedAt.UTC()}
}

func executionRuntimeObservations(observations []adminopsport.RuntimeObservation) []map[string]any {
	result := make([]map[string]any, 0, len(observations))
	for _, observation := range observations {
		result = append(result, map[string]any{"source": observation.Source, "queue": observation.Queue, "status": observation.Status, "attempt": observation.Attempt, "status_url": observation.StatusURL, "details": executionRuntimeDetails(observation.Details), "observed_at": observation.ObservedAt.UTC()})
	}
	return result
}

func executionRuntimeGraph(graph adminopsport.ExecutionGraph) map[string]any {
	roots := make([]map[string]any, 0, len(graph.Roots))
	for _, root := range graph.Roots {
		roots = append(roots, executionRuntimeNode(root))
	}
	return map[string]any{"roots": roots, "items": executionRuntimeObservations(graph.Items), "truncated": graph.Truncated}
}

func executionRuntimeNode(node adminopsport.ExecutionGraphNode) map[string]any {
	children := make([]map[string]any, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, executionRuntimeNode(child))
	}
	return map[string]any{"id": node.ID, "kind": node.Kind, "status": node.Status, "message": node.Message, "details": executionRuntimeDetails(node.Details), "observed_at": node.ObservedAt.UTC(), "children": children}
}

func executionRuntimeDetails(details map[string]string) map[string]string {
	if details == nil {
		return map[string]string{}
	}
	return details
}
