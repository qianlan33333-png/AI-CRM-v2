package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	stdhttp "net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

const bodyLimit = 64 << 10

// Handler is intentionally independent of generated OpenAPI types and route
// registration. The central integration lane owns both after the contract is
// frozen. Collaborator metadata is never consulted as authorization.
type Handler struct {
	application memberport.Application
}

func NewHandler(application memberport.Application) (*Handler, error) {
	if nilInterface(application) {
		return nil, memberport.ErrUnavailable
	}
	return &Handler{application: application}, nil
}

type addRequest struct {
	CustomerID int64               `json:"customer_id"`
	Source     memberdomain.Source `json:"source"`
	ExpiresAt  *time.Time          `json:"expires_at"`
	Remark     *string             `json:"remark"`
	Alliance   *string             `json:"alliance"`
}

type transitionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

type updateFieldsRequest struct {
	ExpectedVersion int64   `json:"expected_version"`
	Remark          *string `json:"remark"`
	Alliance        *string `json:"alliance"`
}

type exportRequest struct {
	State   *memberdomain.State       `json:"state"`
	Source  *memberdomain.Source      `json:"source"`
	Columns []memberport.ExportColumn `json:"columns"`
}

func (handler *Handler) Add(w stdhttp.ResponseWriter, request *stdhttp.Request, productID int64) {
	principal, key, err := handler.writeOperation(request)
	if err != nil {
		writeError(w, request, err)
		return
	}
	var body addRequest
	if err = decodeBody(w, request, &body); err != nil {
		writeError(w, request, err)
		return
	}
	member, err := handler.application.Add(request.Context(), memberport.AddCommand{
		ServiceProductID: productID, CustomerID: body.CustomerID, Source: body.Source,
		ExpiresAt: body.ExpiresAt, Remark: body.Remark, Alliance: body.Alliance,
		ActorID: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeError(w, request, err)
		return
	}
	writeJSON(w, stdhttp.StatusCreated, member)
}

func (handler *Handler) Expire(w stdhttp.ResponseWriter, request *stdhttp.Request, productID int64, memberRef string) {
	handler.transition(w, request, productID, memberRef, handler.application.Expire)
}

func (handler *Handler) Remove(w stdhttp.ResponseWriter, request *stdhttp.Request, productID int64, memberRef string) {
	handler.transition(w, request, productID, memberRef, handler.application.Remove)
}

func (handler *Handler) transition(w stdhttp.ResponseWriter, request *stdhttp.Request, productID int64, memberRef string, action func(context.Context, memberport.TransitionCommand) (memberdomain.Member, error)) {
	principal, key, err := handler.writeOperation(request)
	if err != nil {
		writeError(w, request, err)
		return
	}
	var body transitionRequest
	if err = decodeBody(w, request, &body); err != nil {
		writeError(w, request, err)
		return
	}
	member, err := action(request.Context(), memberport.TransitionCommand{ServiceProductID: productID, MemberRef: memberRef, ExpectedVersion: body.ExpectedVersion, ActorID: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeError(w, request, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, member)
}

func (handler *Handler) UpdateFields(w stdhttp.ResponseWriter, request *stdhttp.Request, productID int64, memberRef string) {
	principal, key, err := handler.writeOperation(request)
	if err != nil {
		writeError(w, request, err)
		return
	}
	var body updateFieldsRequest
	if err = decodeBody(w, request, &body); err != nil {
		writeError(w, request, err)
		return
	}
	member, err := handler.application.UpdateFields(request.Context(), memberport.UpdateFieldsCommand{ServiceProductID: productID, MemberRef: memberRef, ExpectedVersion: body.ExpectedVersion, Remark: body.Remark, Alliance: body.Alliance, ActorID: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeError(w, request, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, member)
}

func (handler *Handler) Get(w stdhttp.ResponseWriter, request *stdhttp.Request, productID int64, memberRef string) {
	if err := handler.readOperation(request); err != nil {
		writeError(w, request, err)
		return
	}
	member, err := handler.application.Get(request.Context(), productID, memberRef)
	if err != nil {
		writeError(w, request, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, member)
}

func (handler *Handler) List(w stdhttp.ResponseWriter, request *stdhttp.Request, productID int64) {
	if err := handler.readOperation(request); err != nil {
		writeError(w, request, err)
		return
	}
	query, err := parseListQuery(request, productID)
	if err != nil {
		writeError(w, request, err)
		return
	}
	result, err := handler.application.List(request.Context(), query)
	if err != nil {
		writeError(w, request, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, result)
}

func (handler *Handler) Export(w stdhttp.ResponseWriter, request *stdhttp.Request, productID int64) {
	if err := handler.readOperation(request); err != nil {
		writeError(w, request, err)
		return
	}
	var body exportRequest
	if err := decodeBody(w, request, &body); err != nil {
		writeError(w, request, err)
		return
	}
	result, err := handler.application.Export(request.Context(), memberport.ExportQuery{Filter: memberport.Filter{ServiceProductID: productID, State: body.State, Source: body.Source}, Columns: body.Columns})
	if err != nil {
		writeError(w, request, err)
		return
	}
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="service-period-members.csv"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = w.Write(result.Body)
}

func (handler *Handler) writeOperation(request *stdhttp.Request) (authport.Principal, string, error) {
	principal, err := handler.principal(request, authport.CapabilityEntitlementsWrite)
	if err != nil {
		return authport.Principal{}, "", err
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || len(keys[0]) < 16 || len(keys[0]) > 128 || strings.TrimSpace(keys[0]) != keys[0] {
		return authport.Principal{}, "", memberport.ErrInvalidInput
	}
	return principal, keys[0], nil
}

func (handler *Handler) readOperation(request *stdhttp.Request) error {
	_, err := handler.principal(request, authport.CapabilityEntitlementsRead)
	return err
}

func (handler *Handler) principal(request *stdhttp.Request, capability authport.Capability) (authport.Principal, error) {
	if handler == nil || nilInterface(handler.application) || request == nil {
		return authport.Principal{}, memberport.ErrUnavailable
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 || principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return principal, nil
}

func parseListQuery(request *stdhttp.Request, productID int64) (memberport.ListQuery, error) {
	if request == nil || request.URL == nil || productID < 1 {
		return memberport.ListQuery{}, memberport.ErrInvalidInput
	}
	values := request.URL.Query()
	for key, entries := range values {
		if key != "state" && key != "source" && key != "limit" && key != "cursor" || len(entries) != 1 {
			return memberport.ListQuery{}, memberport.ErrInvalidInput
		}
	}
	query := memberport.ListQuery{Filter: memberport.Filter{ServiceProductID: productID}, Limit: memberport.DefaultLimit, Cursor: values.Get("cursor")}
	if value := values.Get("state"); value != "" {
		state := memberdomain.State(value)
		query.State = &state
	}
	if value := values.Get("source"); value != "" {
		source := memberdomain.Source(value)
		query.Source = &source
	}
	if value := values.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return memberport.ListQuery{}, memberport.ErrInvalidInput
		}
		query.Limit = limit
	}
	return query, nil
}

func decodeBody(w stdhttp.ResponseWriter, request *stdhttp.Request, target any) error {
	if request == nil || request.Body == nil || target == nil {
		return memberport.ErrInvalidInput
	}
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return memberport.ErrInvalidInput
	}
	request.Body = stdhttp.MaxBytesReader(w, request.Body, bodyLimit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return memberport.ErrInvalidInput
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return memberport.ErrInvalidInput
	}
	return nil
}

func writeJSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w stdhttp.ResponseWriter, request *stdhttp.Request, err error) {
	var httpError *platformhttp.HTTPError
	if errors.As(err, &httpError) {
		platformhttp.WriteError(w, request, httpError)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, memberport.ErrInvalidInput):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, memberport.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, memberport.ErrConflict), errors.Is(err, memberport.ErrPaidOrderSourceBlocked), errors.Is(err, memberport.ErrExportTooLarge):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(w, request, platformhttp.NewError(code, err))
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}
