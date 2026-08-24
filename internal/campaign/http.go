package campaign

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	stdhttp "net/http"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrUnauthenticated = errors.New("campaign unauthenticated")
	ErrForbidden       = errors.New("campaign forbidden")
	ErrCSRFInvalid     = errors.New("campaign csrf invalid")
)

type Authorizer interface {
	Authorize(*stdhttp.Request, AccessRequirement) (Actor, error)
}
type CSRFVerifier interface {
	Verify(*stdhttp.Request, Actor) error
}
type Route struct {
	Method, Pattern, Capability string
	RequiresCSRF                bool
}

// RouteFragment is deliberately unregistered. Lane E owns central route wiring.
type RouteFragment struct {
	application Application
	authorizer  Authorizer
	csrf        CSRFVerifier
}

func NewRouteFragment(application Application, authorizer Authorizer, csrf CSRFVerifier) (*RouteFragment, error) {
	if nilish(application) || nilish(authorizer) || nilish(csrf) {
		return nil, ErrUnavailable
	}
	return &RouteFragment{application, authorizer, csrf}, nil
}
func (h *RouteFragment) Routes() []Route {
	return []Route{{stdhttp.MethodGet, RoutePrefix, CapabilityOperationsRead, false}, {stdhttp.MethodPost, RoutePrefix + "/batch-start", CapabilityManageAutomation, true}, {stdhttp.MethodGet, RoutePrefix + "/{campaign_code}", CapabilityOperationsRead, false}, {stdhttp.MethodDelete, RoutePrefix + "/{campaign_code}", CapabilityManageAutomation, true}, {stdhttp.MethodPost, RoutePrefix + "/{campaign_code}/approve", CapabilityManageAutomation, true}, {stdhttp.MethodPost, RoutePrefix + "/{campaign_code}/reject", CapabilityManageAutomation, true}, {stdhttp.MethodPost, RoutePrefix + "/{campaign_code}/pause", CapabilityManageAutomation, true}, {stdhttp.MethodPost, RoutePrefix + "/{campaign_code}/start", CapabilityManageAutomation, true}, {stdhttp.MethodPost, RoutePrefix + "/{campaign_code}/steps", CapabilityManageAutomation, true}, {stdhttp.MethodPatch, RoutePrefix + "/{campaign_code}/steps/{step_index}", CapabilityManageAutomation, true}, {stdhttp.MethodDelete, RoutePrefix + "/{campaign_code}/steps/{step_index}", CapabilityManageAutomation, true}}
}
func (h *RouteFragment) ServeHTTP(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if h == nil || nilish(h.application) || nilish(h.authorizer) || nilish(h.csrf) || r == nil || r.URL == nil {
		writeHTTPError(w, 503, "DEPENDENCY_UNAVAILABLE")
		return
	}
	if r.URL.EscapedPath() != r.URL.Path || strings.Contains(r.URL.Path, "\\") {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return
	}
	if r.URL.Path == RoutePrefix {
		if r.Method != stdhttp.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		h.list(w, r)
		return
	}
	if r.URL.Path == RoutePrefix+"/batch-start" {
		if r.Method != stdhttp.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.batch(w, r)
		return
	}
	tail := strings.TrimPrefix(r.URL.Path, RoutePrefix+"/")
	parts := strings.Split(tail, "/")
	if len(parts) < 1 || len(parts) > 3 || !validCode(parts[0]) {
		writeHTTPError(w, 404, "NOT_FOUND")
		return
	}
	if len(parts) == 1 {
		if r.Method == stdhttp.MethodGet {
			h.detail(w, r, parts[0])
		} else if r.Method == stdhttp.MethodDelete {
			h.remove(w, r, parts[0])
		} else {
			methodNotAllowed(w, "GET, DELETE")
		}
		return
	}
	if len(parts) == 2 && parts[1] == "steps" {
		if r.Method == stdhttp.MethodPost {
			h.addStep(w, r, parts[0])
		} else {
			methodNotAllowed(w, "POST")
		}
		return
	}
	if len(parts) == 2 && oneOf(parts[1], "approve", "reject", "pause", "start") {
		if r.Method != stdhttp.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.action(w, r, parts[0], parts[1])
		return
	}
	if len(parts) == 3 && parts[1] == "steps" {
		index, ok := positiveIndex(parts[2])
		if !ok {
			writeHTTPError(w, 400, "MALFORMED_REQUEST")
			return
		}
		if r.Method == stdhttp.MethodPatch {
			h.updateStep(w, r, parts[0], index)
		} else if r.Method == stdhttp.MethodDelete {
			h.deleteStep(w, r, parts[0], index)
		} else {
			methodNotAllowed(w, "PATCH, DELETE")
		}
		return
	}
	writeHTTPError(w, 404, "NOT_FOUND")
}
func (h *RouteFragment) list(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !emptyBody(r) {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return
	}
	if _, ok := h.authorize(w, r, CapabilityOperationsRead, false); !ok {
		return
	}
	input, valid := parseListInput(r.URL.RawQuery)
	if !valid {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return
	}
	out, e := h.application.List(r.Context(), input)
	if e != nil {
		mapError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (h *RouteFragment) detail(w stdhttp.ResponseWriter, r *stdhttp.Request, code string) {
	if !emptyBody(r) || r.URL.RawQuery != "" {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return
	}
	if _, ok := h.authorize(w, r, CapabilityOperationsRead, false); !ok {
		return
	}
	out, e := h.application.Detail(r.Context(), code)
	if e != nil {
		mapError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (h *RouteFragment) action(w stdhttp.ResponseWriter, r *stdhttp.Request, code, action string) {
	actor, key, body, ok := h.writeVersion(w, r)
	if !ok {
		return
	}
	cmd := VersionedCommand{CampaignCode: code, ExpectedVersion: body.ExpectedVersion, Actor: actor, IdempotencyKey: key}
	var (
		out MutationResponse
		e   error
	)
	switch action {
	case "approve":
		out, e = h.application.Approve(r.Context(), cmd)
	case "reject":
		out, e = h.application.Reject(r.Context(), cmd)
	case "pause":
		out, e = h.application.Pause(r.Context(), cmd)
	case "start":
		out, e = h.application.Start(r.Context(), cmd)
	}
	if e != nil {
		mapError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (h *RouteFragment) remove(w stdhttp.ResponseWriter, r *stdhttp.Request, code string) {
	actor, key, body, ok := h.writeVersion(w, r)
	if !ok {
		return
	}
	out, e := h.application.Delete(r.Context(), VersionedCommand{CampaignCode: code, ExpectedVersion: body.ExpectedVersion, Actor: actor, IdempotencyKey: key})
	if e != nil {
		mapError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (h *RouteFragment) addStep(w stdhttp.ResponseWriter, r *stdhttp.Request, code string) {
	actor, key, ok := h.writeHeader(w, r)
	if !ok {
		return
	}
	var body stepCreateRequest
	if !decodeJSON(r, &body) || body.ExpectedVersion == 0 {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return
	}
	if !validStepFields(body.DelayMinutes, body.Content) {
		writeHTTPError(w, 422, "VALIDATION_FAILED")
		return
	}
	out, e := h.application.AddStep(r.Context(), StepCreateCommand{CampaignCode: code, ExpectedVersion: body.ExpectedVersion, DelayMinutes: body.DelayMinutes, Content: body.Content, Actor: actor, IdempotencyKey: key})
	if e != nil {
		mapError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (h *RouteFragment) updateStep(w stdhttp.ResponseWriter, r *stdhttp.Request, code string, index int32) {
	actor, key, ok := h.writeHeader(w, r)
	if !ok {
		return
	}
	var body stepUpdateRequest
	if !decodeJSON(r, &body) || body.ExpectedVersion == 0 || body.DelayMinutes == nil && body.Content == nil {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return
	}
	if body.DelayMinutes != nil && !validDelay(*body.DelayMinutes) || body.Content != nil && !validContent(*body.Content) {
		writeHTTPError(w, 422, "VALIDATION_FAILED")
		return
	}
	out, e := h.application.UpdateStep(r.Context(), StepUpdateCommand{CampaignCode: code, StepIndex: index, ExpectedVersion: body.ExpectedVersion, DelayMinutes: body.DelayMinutes, Content: body.Content, Actor: actor, IdempotencyKey: key})
	if e != nil {
		mapError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (h *RouteFragment) deleteStep(w stdhttp.ResponseWriter, r *stdhttp.Request, code string, index int32) {
	actor, key, body, ok := h.writeVersion(w, r)
	if !ok {
		return
	}
	out, e := h.application.DeleteStep(r.Context(), StepDeleteCommand{CampaignCode: code, StepIndex: index, ExpectedVersion: body.ExpectedVersion, Actor: actor, IdempotencyKey: key})
	if e != nil {
		mapError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (h *RouteFragment) batch(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	actor, key, ok := h.writeHeader(w, r)
	if !ok {
		return
	}
	var body batchRequest
	if !decodeJSON(r, &body) {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return
	}
	if !validBatch(BatchStartCommand{Items: body.Campaigns, Actor: actor, IdempotencyKey: key}) {
		writeHTTPError(w, 422, "VALIDATION_FAILED")
		return
	}
	out, e := h.application.BatchStart(r.Context(), BatchStartCommand{Items: body.Campaigns, Actor: actor, IdempotencyKey: key})
	if e != nil {
		mapError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (h *RouteFragment) writeHeader(w stdhttp.ResponseWriter, r *stdhttp.Request) (Actor, string, bool) {
	if r.URL.RawQuery != "" {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return Actor{}, "", false
	}
	actor, ok := h.authorize(w, r, CapabilityManageAutomation, true)
	if !ok {
		return Actor{}, "", false
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return Actor{}, "", false
	}
	key := keys[0]
	if !validKey(key) {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return Actor{}, "", false
	}
	return actor, key, true
}
func (h *RouteFragment) writeVersion(w stdhttp.ResponseWriter, r *stdhttp.Request) (Actor, string, versionRequest, bool) {
	actor, key, ok := h.writeHeader(w, r)
	if !ok {
		return Actor{}, "", versionRequest{}, false
	}
	var body versionRequest
	if !decodeJSON(r, &body) || body.ExpectedVersion < 1 {
		writeHTTPError(w, 400, "MALFORMED_REQUEST")
		return Actor{}, "", versionRequest{}, false
	}
	return actor, key, body, true
}
func (h *RouteFragment) authorize(w stdhttp.ResponseWriter, r *stdhttp.Request, capability string, csrf bool) (Actor, bool) {
	actor, e := h.authorizer.Authorize(r, AccessRequirement{Capability: capability, RequireCSRF: csrf})
	if errors.Is(e, ErrUnauthenticated) {
		writeHTTPError(w, 401, "UNAUTHENTICATED")
		return Actor{}, false
	}
	if e != nil || actor.ID < 1 {
		writeHTTPError(w, 403, "FORBIDDEN")
		return Actor{}, false
	}
	if csrf && h.csrf.Verify(r, actor) != nil {
		writeHTTPError(w, 403, "CSRF_INVALID")
		return Actor{}, false
	}
	return actor, true
}

type versionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}
type stepCreateRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	DelayMinutes    int32  `json:"delay_minutes"`
	Content         string `json:"content"`
}
type stepUpdateRequest struct {
	ExpectedVersion int64   `json:"expected_version"`
	DelayMinutes    *int32  `json:"delay_minutes"`
	Content         *string `json:"content"`
}
type batchRequest struct {
	Campaigns []BatchStartItem `json:"campaigns"`
}

func decodeJSON(r *stdhttp.Request, dst any) bool {
	if r.Body == nil || r.ContentLength > MaximumRequestBodyBytes {
		return false
	}
	ct := r.Header.Get("Content-Type")
	media, params, e := mime.ParseMediaType(ct)
	if e != nil || media != "application/json" || len(params) > 1 || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
		return false
	}
	raw, e := io.ReadAll(io.LimitReader(r.Body, MaximumRequestBodyBytes+1))
	if e != nil || int64(len(raw)) > MaximumRequestBodyBytes || len(bytes.TrimSpace(raw)) == 0 {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(dst) == nil && d.Decode(&struct{}{}) == io.EOF
}
func emptyBody(r *stdhttp.Request) bool {
	if r.Body == nil {
		return true
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1))
	return len(raw) == 0
}
func positiveIndex(value string) (int32, bool) {
	if value == "" || strings.TrimLeft(value, "0123456789") != "" {
		return 0, false
	}
	n, e := strconv.ParseInt(value, 10, 32)
	return int32(n), e == nil && n > 0
}
func parseListInput(raw string) (ListInput, bool) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return ListInput{}, false
	}
	var input ListInput
	for key, values := range values {
		if len(values) != 1 {
			return ListInput{}, false
		}
		switch key {
		case "approval_status":
			value := ApprovalStatus(values[0])
			if !value.Valid() {
				return ListInput{}, false
			}
			input.ApprovalStatus = &value
		case "runtime_status":
			value := RuntimeStatus(values[0])
			if !value.Valid() {
				return ListInput{}, false
			}
			input.RuntimeStatus = &value
		default:
			return ListInput{}, false
		}
	}
	return input, true
}
func oneOf(v string, allowed ...string) bool {
	for _, x := range allowed {
		if v == x {
			return true
		}
	}
	return false
}
func methodNotAllowed(w stdhttp.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeHTTPError(w, 405, "METHOD_NOT_ALLOWED")
}
func mapError(w stdhttp.ResponseWriter, e error) {
	var validation *ValidationError
	switch {
	case errors.As(e, &validation):
		writeHTTPError(w, 422, "VALIDATION_FAILED")
	case errors.Is(e, ErrInvalidArgument):
		writeHTTPError(w, 400, "INVALID_ARGUMENT")
	case errors.Is(e, ErrNotFound):
		writeHTTPError(w, 404, "NOT_FOUND")
	case errors.Is(e, ErrConflict), errors.Is(e, ErrIdempotencyConflict):
		writeHTTPError(w, 409, "CONFLICT")
	case errors.Is(e, ErrStateConflict):
		writeHTTPError(w, 409, "STATE_CONFLICT")
	default:
		writeHTTPError(w, 503, "UNAVAILABLE")
	}
}
func writeHTTPError(w stdhttp.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}
func writeJSON(w stdhttp.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
