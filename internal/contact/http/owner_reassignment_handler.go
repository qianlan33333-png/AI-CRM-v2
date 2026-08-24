package http

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type ownerReassignmentApplication interface {
	CreatePreview(context.Context, int64, []byte, string) (contactapp.OwnerReassignmentPreview, error)
	Preview(context.Context, int64, string) (contactapp.OwnerReassignmentPreview, error)
	Execute(context.Context, int64, string, string, string, string) (contactapp.OwnerReassignmentPreview, error)
}
type OwnerReassignmentHandler struct{ application ownerReassignmentApplication }

func NewOwnerReassignmentHandler(a ownerReassignmentApplication) (*OwnerReassignmentHandler, error) {
	if a == nil {
		return nil, errors.New("owner reassignment application is required")
	}
	return &OwnerReassignmentHandler{a}, nil
}
func (h *OwnerReassignmentHandler) Template(w http.ResponseWriter, r *http.Request) {
	if _, e := h.actor(r); e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	ownerReassignmentCSV(w, "owner-reassignment-template.csv", contactapp.OwnerReassignmentTemplate())
}
func (h *OwnerReassignmentHandler) CreatePreview(w http.ResponseWriter, r *http.Request) {
	actor, e := h.actor(r)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	mediaType, params, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if parseErr != nil || mediaType != "text/csv" || len(params) > 1 || (len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) {
		writeOwnerReassignmentError(w, r, contactapp.ErrOwnerReassignmentInvalid)
		return
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		writeOwnerReassignmentError(w, r, contactapp.ErrOwnerReassignmentInvalid)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	p, e := h.application.CreatePreview(r.Context(), actor, body, key)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	ownerReassignmentJSON(w, http.StatusCreated, p)
}
func (h *OwnerReassignmentHandler) Preview(w http.ResponseWriter, r *http.Request, id string) {
	actor, e := h.actor(r)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	p, e := h.application.Preview(r.Context(), actor, id)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	ownerReassignmentJSON(w, http.StatusOK, p)
}
func (h *OwnerReassignmentHandler) Execute(w http.ResponseWriter, r *http.Request, id string) {
	actor, e := h.actor(r)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	var body struct {
		PreviewHash  string `json:"preview_hash"`
		Confirmation string `json:"confirmation"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeOwnerReassignmentError(w, r, contactapp.ErrOwnerReassignmentInvalid)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	p, e := h.application.Execute(r.Context(), actor, id, body.PreviewHash, body.Confirmation, key)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	ownerReassignmentJSON(w, http.StatusOK, p)
}
func (h *OwnerReassignmentHandler) ErrorsCSV(w http.ResponseWriter, r *http.Request, id string) {
	actor, e := h.actor(r)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	p, e := h.application.Preview(r.Context(), actor, id)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	var b strings.Builder
	c := csv.NewWriter(&b)
	_ = c.Write([]string{"line", "code"})
	for _, x := range p.Issues {
		_ = c.Write([]string{strconv.Itoa(x.Line), x.Code})
	}
	c.Flush()
	ownerReassignmentCSV(w, "owner-reassignment-errors.csv", []byte(b.String()))
}
func (h *OwnerReassignmentHandler) ResultsCSV(w http.ResponseWriter, r *http.Request, id string) {
	actor, e := h.actor(r)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	p, e := h.application.Preview(r.Context(), actor, id)
	if e != nil {
		writeOwnerReassignmentError(w, r, e)
		return
	}
	if !p.Executed {
		writeOwnerReassignmentError(w, r, contactapp.ErrOwnerReassignmentConflict)
		return
	}
	var b strings.Builder
	c := csv.NewWriter(&b)
	_ = c.Write([]string{"customer_id", "previous_owner_staff_id", "target_owner_staff_id", "updated_at"})
	for _, x := range p.Result {
		_ = c.Write([]string{strconv.FormatInt(x.CustomerID, 10), strconv.FormatInt(x.PreviousOwnerStaffID, 10), strconv.FormatInt(x.TargetOwnerStaffID, 10), x.UpdatedAt.UTC().Format(time.RFC3339Nano)})
	}
	c.Flush()
	ownerReassignmentCSV(w, "owner-reassignment-results.csv", []byte(b.String()))
}

func (h *OwnerReassignmentHandler) actor(r *http.Request) (int64, error) {
	if h == nil || r == nil {
		return 0, contactapp.ErrOwnerReassignmentUnavailable
	}
	p, ok := authport.PrincipalFromContext(r.Context())
	a, ok2 := authport.AuthorizationFromContext(r.Context())
	if !ok || !ok2 || p.Role != authport.RoleAdmin || p.AdminUserID < 1 || a.Capability != authport.CapabilityContactOwnerReassignment || a.Scope != authport.ScopeGlobal {
		return 0, authport.ErrUnauthorized
	}
	return p.AdminUserID, nil
}
func ownerReassignmentCSV(w http.ResponseWriter, name string, data []byte) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
func ownerReassignmentJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeOwnerReassignmentError(w http.ResponseWriter, r *http.Request, e error) {
	code := platformhttp.CodeInternal
	switch {
	case errors.Is(e, authport.ErrUnauthorized):
		code = platformhttp.CodeUnauthorized
	case errors.Is(e, contactapp.ErrOwnerReassignmentInvalid):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(e, contactapp.ErrOwnerReassignmentNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(e, contactapp.ErrOwnerReassignmentConflict), errors.Is(e, contactapp.ErrOwnerReassignmentExpired):
		code = platformhttp.CodeConflict
	case errors.Is(e, contactapp.ErrOwnerReassignmentUnavailable):
		code = platformhttp.CodeDependencyUnavailable
	}
	platformhttp.WriteError(w, r, platformhttp.NewError(code, e))
}
