package membergrid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const maximumManagementBodyBytes int64 = 16 << 10

const (
	CapabilityProductsRead  = "products.read"
	CapabilityProductsWrite = "products.write"
)

type ManagementActor struct {
	ID int64
}

// ManagementAuthorizer is intentionally supplied by Lane E. This leaf package
// does not modify or bypass central authentication and authorization.
type ManagementAuthorizer interface {
	Authorize(context.Context, string) (ManagementActor, error)
}

// ManagementCSRFVerifier is intentionally supplied by Lane E and is required
// for every mutation handled by this fragment.
type ManagementCSRFVerifier interface {
	Verify(*http.Request) error
}

type ManagementHandler struct {
	application ManagementApplication
	authorizer  ManagementAuthorizer
	csrf        ManagementCSRFVerifier
}

func NewManagementHandler(application ManagementApplication, authorizer ManagementAuthorizer, csrf ManagementCSRFVerifier) (*ManagementHandler, error) {
	if nilDependency(application) || nilDependency(authorizer) || nilDependency(csrf) {
		return nil, errors.New("member grid management HTTP dependencies are required")
	}
	return &ManagementHandler{application: application, authorizer: authorizer, csrf: csrf}, nil
}

type CreateSavedViewRequest struct {
	ExpectedVersion *int64    `json:"expected_version"`
	Name            *string   `json:"name"`
	State           *string   `json:"state"`
	Sort            *string   `json:"sort"`
	Columns         *[]string `json:"columns"`
	SourceViewID    *int64    `json:"source_view_id"`
}

type UpdateSavedViewRequest struct {
	ExpectedVersion *int64    `json:"expected_version"`
	Name            *string   `json:"name"`
	State           *string   `json:"state"`
	Sort            *string   `json:"sort"`
	Columns         *[]string `json:"columns"`
}

type DeleteVersionedRequest struct {
	ExpectedVersion *int64 `json:"expected_version"`
}

type CreateCollaboratorRequest struct {
	ExpectedVersion *int64  `json:"expected_version"`
	StaffID         *int64  `json:"staff_id"`
	Permission      *string `json:"permission"`
}

type UpdateCollaboratorRequest struct {
	ExpectedVersion *int64  `json:"expected_version"`
	Permission      *string `json:"permission"`
}

func (handler *ManagementHandler) CreateSavedView(writer http.ResponseWriter, request *http.Request, rawProductID string) {
	actor, key, ok := handler.prepareWrite(writer, request, http.MethodPost)
	if !ok {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	var body CreateSavedViewRequest
	seen, problem := decodeManagementObject(writer, request, map[string]struct{}{
		"expected_version": {}, "name": {}, "state": {}, "sort": {}, "columns": {}, "source_view_id": {},
	}, &body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if body.ExpectedVersion == nil || body.Name == nil {
		writeProblem(writer, request, validation("body", "required_fields", ErrInvalidManagementInput))
		return
	}
	command := CreateSavedViewCommand{
		ServiceProductID: productID,
		ExpectedVersion:  *body.ExpectedVersion,
		Name:             *body.Name,
		ActorID:          actor.ID,
		IdempotencyKey:   key,
	}
	_, sourceSupplied := seen["source_view_id"]
	if sourceSupplied {
		if body.SourceViewID == nil || fieldSeen(seen, "state") || fieldSeen(seen, "sort") || fieldSeen(seen, "columns") {
			writeProblem(writer, request, validation("source_view_id", "invalid_clone", ErrInvalidManagementInput))
			return
		}
		command.SourceViewID = cloneOptionalID(body.SourceViewID)
	} else {
		if body.State == nil || body.Sort == nil || body.Columns == nil {
			writeProblem(writer, request, validation("body", "required_fields", ErrInvalidManagementInput))
			return
		}
		command.State = StateFilter(*body.State)
		command.Sort = ViewSort(*body.Sort)
		command.Columns = cloneColumnsSelection(*body.Columns)
	}
	response, err := handler.application.CreateSavedView(request.Context(), command)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, response)
}

func (handler *ManagementHandler) UpdateSavedView(writer http.ResponseWriter, request *http.Request, rawProductID, rawViewID string) {
	actor, key, ok := handler.prepareWrite(writer, request, http.MethodPut)
	if !ok {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	var body UpdateSavedViewRequest
	_, problem := decodeManagementObject(writer, request, map[string]struct{}{
		"expected_version": {}, "name": {}, "state": {}, "sort": {}, "columns": {},
	}, &body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if body.ExpectedVersion == nil || body.Name == nil || body.State == nil || body.Sort == nil || body.Columns == nil {
		writeProblem(writer, request, validation("body", "required_fields", ErrInvalidManagementInput))
		return
	}
	if rawViewID == builtInViews[0].ID {
		writeManagementFailure(writer, request, ErrBuiltInView)
		return
	}
	viewID, err := parseEntityID(rawViewID)
	if err != nil {
		writeProblem(writer, request, malformed("view_id", "invalid", err))
		return
	}
	response, err := handler.application.UpdateSavedView(request.Context(), UpdateSavedViewCommand{
		ServiceProductID: productID,
		ViewID:           viewID,
		ExpectedVersion:  *body.ExpectedVersion,
		Name:             *body.Name,
		State:            StateFilter(*body.State),
		Sort:             ViewSort(*body.Sort),
		Columns:          cloneColumnsSelection(*body.Columns),
		ActorID:          actor.ID,
		IdempotencyKey:   key,
	})
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *ManagementHandler) DeleteSavedView(writer http.ResponseWriter, request *http.Request, rawProductID, rawViewID string) {
	actor, key, ok := handler.prepareWrite(writer, request, http.MethodDelete)
	if !ok {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	var body DeleteVersionedRequest
	_, problem := decodeManagementObject(writer, request, map[string]struct{}{"expected_version": {}}, &body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if body.ExpectedVersion == nil {
		writeProblem(writer, request, validation("expected_version", "required", ErrInvalidManagementInput))
		return
	}
	if rawViewID == builtInViews[0].ID {
		writeManagementFailure(writer, request, ErrBuiltInView)
		return
	}
	viewID, err := parseEntityID(rawViewID)
	if err != nil {
		writeProblem(writer, request, malformed("view_id", "invalid", err))
		return
	}
	response, err := handler.application.DeleteSavedView(request.Context(), DeleteSavedViewCommand{
		ServiceProductID: productID, ViewID: viewID, ExpectedVersion: *body.ExpectedVersion,
		ActorID: actor.ID, IdempotencyKey: key,
	})
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *ManagementHandler) ShareSettings(writer http.ResponseWriter, request *http.Request, rawProductID string) {
	if !requireManagementMethod(writer, request, http.MethodGet) {
		return
	}
	if _, ok := handler.authorize(writer, request, CapabilityProductsRead); !ok {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	response, err := handler.application.ShareSettings(request.Context(), productID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *ManagementHandler) CreateCollaborator(writer http.ResponseWriter, request *http.Request, rawProductID string) {
	actor, key, ok := handler.prepareWrite(writer, request, http.MethodPost)
	if !ok {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	var body CreateCollaboratorRequest
	_, problem := decodeManagementObject(writer, request, map[string]struct{}{
		"expected_version": {}, "staff_id": {}, "permission": {},
	}, &body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if body.ExpectedVersion == nil || body.StaffID == nil || body.Permission == nil {
		writeProblem(writer, request, validation("body", "required_fields", ErrInvalidManagementInput))
		return
	}
	response, err := handler.application.CreateCollaborator(request.Context(), CreateCollaboratorCommand{
		ServiceProductID: productID, ExpectedVersion: *body.ExpectedVersion, StaffID: *body.StaffID,
		Permission: CollaboratorPermission(*body.Permission), ActorID: actor.ID, IdempotencyKey: key,
	})
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, response)
}

func (handler *ManagementHandler) UpdateCollaborator(writer http.ResponseWriter, request *http.Request, rawProductID, rawCollaboratorID string) {
	actor, key, ok := handler.prepareWrite(writer, request, http.MethodPut)
	if !ok {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	collaboratorID, err := parseEntityID(rawCollaboratorID)
	if err != nil {
		writeProblem(writer, request, malformed("collaborator_id", "invalid", err))
		return
	}
	var body UpdateCollaboratorRequest
	_, problem := decodeManagementObject(writer, request, map[string]struct{}{
		"expected_version": {}, "permission": {},
	}, &body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if body.ExpectedVersion == nil || body.Permission == nil {
		writeProblem(writer, request, validation("body", "required_fields", ErrInvalidManagementInput))
		return
	}
	response, err := handler.application.UpdateCollaborator(request.Context(), UpdateCollaboratorCommand{
		ServiceProductID: productID, CollaboratorID: collaboratorID, ExpectedVersion: *body.ExpectedVersion,
		Permission: CollaboratorPermission(*body.Permission), ActorID: actor.ID, IdempotencyKey: key,
	})
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *ManagementHandler) DeleteCollaborator(writer http.ResponseWriter, request *http.Request, rawProductID, rawCollaboratorID string) {
	actor, key, ok := handler.prepareWrite(writer, request, http.MethodDelete)
	if !ok {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	collaboratorID, err := parseEntityID(rawCollaboratorID)
	if err != nil {
		writeProblem(writer, request, malformed("collaborator_id", "invalid", err))
		return
	}
	var body DeleteVersionedRequest
	_, problem := decodeManagementObject(writer, request, map[string]struct{}{"expected_version": {}}, &body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if body.ExpectedVersion == nil {
		writeProblem(writer, request, validation("expected_version", "required", ErrInvalidManagementInput))
		return
	}
	response, err := handler.application.DeleteCollaborator(request.Context(), DeleteCollaboratorCommand{
		ServiceProductID: productID, CollaboratorID: collaboratorID, ExpectedVersion: *body.ExpectedVersion,
		ActorID: actor.ID, IdempotencyKey: key,
	})
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *ManagementHandler) prepareWrite(writer http.ResponseWriter, request *http.Request, method string) (ManagementActor, string, bool) {
	if !requireManagementMethod(writer, request, method) {
		return ManagementActor{}, "", false
	}
	actor, ok := handler.authorize(writer, request, CapabilityProductsWrite)
	if !ok {
		return ManagementActor{}, "", false
	}
	if handler == nil || nilDependency(handler.csrf) {
		writeManagementFailure(writer, request, ErrPermissionDenied)
		return ManagementActor{}, "", false
	}
	if err := handler.csrf.Verify(request); err != nil {
		writeManagementFailure(writer, request, errors.Join(ErrCSRFRejected, err))
		return ManagementActor{}, "", false
	}
	key, problem := managementIdempotencyKey(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return ManagementActor{}, "", false
	}
	return actor, key, true
}

func (handler *ManagementHandler) authorize(writer http.ResponseWriter, request *http.Request, capability string) (ManagementActor, bool) {
	if handler == nil || nilDependency(handler.application) || nilDependency(handler.authorizer) || request == nil {
		writeManagementFailure(writer, request, ErrAuthenticationRequired)
		return ManagementActor{}, false
	}
	actor, err := handler.authorizer.Authorize(request.Context(), capability)
	if err != nil {
		if errors.Is(err, ErrAuthenticationRequired) {
			writeManagementFailure(writer, request, err)
		} else {
			writeManagementFailure(writer, request, errors.Join(ErrPermissionDenied, err))
		}
		return ManagementActor{}, false
	}
	if actor.ID < 1 {
		writeManagementFailure(writer, request, ErrAuthenticationRequired)
		return ManagementActor{}, false
	}
	return actor, true
}

type managementRouteFragment struct {
	handler *ManagementHandler
}

func NewManagementRouteFragment(handler *ManagementHandler) (http.Handler, error) {
	if handler == nil || nilDependency(handler.application) || nilDependency(handler.authorizer) || nilDependency(handler.csrf) {
		return nil, errors.New("member grid management handler is required")
	}
	return &managementRouteFragment{handler: handler}, nil
}

func (fragment *managementRouteFragment) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setSecurityHeaders(writer)
	if fragment == nil || fragment.handler == nil || request == nil || request.URL == nil {
		writeManagementFailure(writer, request, ErrUnavailable)
		return
	}
	path := request.URL.Path
	if request.URL.RawQuery != "" || request.URL.ForceQuery || request.URL.RawPath != "" || request.URL.EscapedPath() != path || !strings.HasPrefix(path, "/") ||
		strings.HasSuffix(path, "/") || strings.Contains(path, "\\") || strings.Contains(path, "//") {
		writeProblem(writer, request, malformed("path", "invalid", ErrInvalidManagementInput))
		return
	}
	if strings.HasPrefix(path, RoutePrefix+"/") {
		path = strings.TrimPrefix(path, RoutePrefix)
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, segment := range segments {
		if segment == "" {
			writeProblem(writer, request, malformed("path", "invalid", ErrInvalidManagementInput))
			return
		}
	}
	switch {
	case len(segments) == 2 && segments[1] == "member-views":
		if request.Method != http.MethodPost {
			writeManagementMethodNotAllowed(writer, http.MethodPost)
			return
		}
		fragment.handler.CreateSavedView(writer, request, segments[0])
	case len(segments) == 3 && segments[1] == "member-views":
		switch request.Method {
		case http.MethodPut:
			fragment.handler.UpdateSavedView(writer, request, segments[0], segments[2])
		case http.MethodDelete:
			fragment.handler.DeleteSavedView(writer, request, segments[0], segments[2])
		default:
			writeManagementMethodNotAllowed(writer, http.MethodPut+", "+http.MethodDelete)
		}
	case len(segments) == 3 && segments[1] == "member-grid" && segments[2] == "share-settings":
		if request.Method != http.MethodGet {
			writeManagementMethodNotAllowed(writer, http.MethodGet)
			return
		}
		fragment.handler.ShareSettings(writer, request, segments[0])
	case len(segments) == 3 && segments[1] == "member-grid" && segments[2] == "collaborators":
		if request.Method != http.MethodPost {
			writeManagementMethodNotAllowed(writer, http.MethodPost)
			return
		}
		fragment.handler.CreateCollaborator(writer, request, segments[0])
	case len(segments) == 4 && segments[1] == "member-grid" && segments[2] == "collaborators":
		switch request.Method {
		case http.MethodPut:
			fragment.handler.UpdateCollaborator(writer, request, segments[0], segments[3])
		case http.MethodDelete:
			fragment.handler.DeleteCollaborator(writer, request, segments[0], segments[3])
		default:
			writeManagementMethodNotAllowed(writer, http.MethodPut+", "+http.MethodDelete)
		}
	default:
		writeCode(writer, request, platformhttp.CodeNotFound, errors.New("member grid management route not found"))
	}
}

func decodeManagementObject(writer http.ResponseWriter, request *http.Request, allowed map[string]struct{}, target any) (map[string]struct{}, *requestProblem) {
	if request == nil || request.Body == nil || target == nil {
		return nil, malformed("body", "required", ErrInvalidManagementInput)
	}
	contentTypes := request.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		return nil, malformed("body", "invalid_content_type", ErrInvalidManagementInput)
	}
	contentType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || contentType != "application/json" {
		return nil, malformed("body", "invalid_content_type", ErrInvalidManagementInput)
	}
	if request.ContentLength > maximumManagementBodyBytes {
		return nil, malformed("body", "too_large", ErrInvalidManagementInput)
	}
	limited := http.MaxBytesReader(writer, request.Body, maximumManagementBodyBytes)
	data, err := io.ReadAll(limited)
	if err != nil || len(bytes.TrimSpace(data)) == 0 || !utf8.Valid(data) {
		return nil, malformed("body", "invalid_json", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, malformed("body", "object_required", err)
	}
	seen := make(map[string]struct{}, len(allowed))
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		key, ok := keyToken.(string)
		if tokenErr != nil || !ok {
			return nil, malformed("body", "invalid_json", tokenErr)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, malformed(key, "duplicate", ErrInvalidManagementInput)
		}
		if _, permitted := allowed[key]; !permitted {
			return nil, malformed("body", "unknown_field", ErrInvalidManagementInput)
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return nil, malformed(key, "invalid", err)
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, malformed("body", "invalid_json", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, malformed("body", "trailing_data", err)
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return nil, validation("body", "invalid_type", err)
	}
	return seen, nil
}

func managementIdempotencyKey(request *http.Request) (string, *requestProblem) {
	if request == nil {
		return "", malformed("Idempotency-Key", "required", ErrInvalidManagementInput)
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || !validIdempotencyKey(values[0]) {
		return "", malformed("Idempotency-Key", "invalid", ErrInvalidManagementInput)
	}
	return values[0], nil
}

func parseEntityID(raw string) (int64, error) {
	if !canonicalProductID.MatchString(raw) {
		return 0, ErrInvalidManagementInput
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, ErrInvalidManagementInput
	}
	return value, nil
}

func fieldSeen(seen map[string]struct{}, field string) bool {
	_, ok := seen[field]
	return ok
}

func requireManagementMethod(writer http.ResponseWriter, request *http.Request, method string) bool {
	if request != nil && request.Method == method {
		return true
	}
	writeManagementMethodNotAllowed(writer, method)
	return false
}

func writeManagementMethodNotAllowed(writer http.ResponseWriter, allow string) {
	setSecurityHeaders(writer)
	writer.Header().Set("Allow", allow)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func writeManagementFailure(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ErrAuthenticationRequired):
		writeCode(writer, request, platformhttp.CodeUnauthenticated, err)
	case errors.Is(err, ErrPermissionDenied), errors.Is(err, ErrCSRFRejected):
		writeCode(writer, request, platformhttp.CodeUnauthorized, err)
	case errors.Is(err, ErrInvalidProductID):
		writeProblem(writer, request, malformed("service_product_id", "invalid", err))
	case errors.Is(err, ErrInvalidManagementInput):
		writeProblem(writer, request, validation("request", "invalid", err))
	case errors.Is(err, ErrInactiveStaff):
		writeProblem(writer, request, validation("staff_id", "inactive", err))
	case errors.Is(err, ErrBuiltInView), errors.Is(err, ErrConflict):
		writeCode(writer, request, platformhttp.CodeConflict, err)
	case errors.Is(err, ErrNotFound):
		writeCode(writer, request, platformhttp.CodeNotFound, err)
	default:
		writeCode(writer, request, platformhttp.CodeDependencyUnavailable, err)
	}
}
