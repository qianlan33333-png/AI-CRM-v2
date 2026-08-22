package http

import (
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

func (fragment *RouteFragment) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if fragment == nil || nilInterface(fragment.application) || nilInterface(fragment.authorizer) || nilInterface(fragment.csrf) || request == nil || request.URL == nil {
		writeError(writer, stdhttp.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required dependency is unavailable.", nil)
		return
	}
	if request.URL.EscapedPath() != request.URL.Path || strings.Contains(request.URL.Path, `\`) {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}

	path := request.URL.Path
	switch path {
	case BasePath:
		fragment.serveCollection(writer, request)
		return
	case BasePath + "/new/options":
		fragment.serveOptions(writer, request)
		return
	}
	if !strings.HasPrefix(path, BasePath+"/") {
		writeError(writer, stdhttp.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
		return
	}
	remainder := strings.TrimPrefix(path, BasePath+"/")
	segments := strings.Split(remainder, "/")
	if len(segments) < 1 || len(segments) > 2 || segments[0] == "" {
		writeError(writer, stdhttp.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
		return
	}
	id, ok := canonicalPositiveID(segments[0])
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	if len(segments) == 1 {
		fragment.serveItem(writer, request, radarport.LinkID(id))
		return
	}
	if segments[1] == "" {
		writeError(writer, stdhttp.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
		return
	}
	fragment.serveAction(writer, request, radarport.LinkID(id), segments[1])
}

func (fragment *RouteFragment) serveCollection(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	switch request.Method {
	case stdhttp.MethodGet:
		fragment.list(writer, request)
	case stdhttp.MethodPost:
		fragment.create(writer, request)
	default:
		writeMethodNotAllowed(writer, "GET, POST")
	}
}

func (fragment *RouteFragment) serveItem(writer stdhttp.ResponseWriter, request *stdhttp.Request, id radarport.LinkID) {
	switch request.Method {
	case stdhttp.MethodGet:
		fragment.get(writer, request, id)
	case stdhttp.MethodPatch:
		fragment.update(writer, request, id)
	default:
		writeMethodNotAllowed(writer, "GET, PATCH")
	}
}

func (fragment *RouteFragment) serveAction(writer stdhttp.ResponseWriter, request *stdhttp.Request, id radarport.LinkID, action string) {
	switch action {
	case "share":
		if request.Method != stdhttp.MethodGet {
			writeMethodNotAllowed(writer, "GET")
			return
		}
		fragment.share(writer, request, id)
	case "enable", "disable":
		if request.Method != stdhttp.MethodPost {
			writeMethodNotAllowed(writer, "POST")
			return
		}
		fragment.setStatus(writer, request, id, action)
	default:
		writeError(writer, stdhttp.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
	}
}

func (fragment *RouteFragment) list(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if _, ok := fragment.authorize(writer, request, PermissionAdminRead, false); !ok {
		return
	}
	if requireEmptyBody(request) != nil {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	input, err := parseListQuery(request.URL.RawQuery)
	if err != nil {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	page, err := fragment.application.List(request.Context(), input)
	if err != nil {
		writeApplicationError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, page)
}

func (fragment *RouteFragment) get(writer stdhttp.ResponseWriter, request *stdhttp.Request, id radarport.LinkID) {
	if _, ok := fragment.authorize(writer, request, PermissionAdminRead, false); !ok {
		return
	}
	if request.URL.RawQuery != "" || requireEmptyBody(request) != nil {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	result, err := fragment.application.Get(request.Context(), id)
	if err != nil {
		writeApplicationError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, result)
}

func (fragment *RouteFragment) create(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	actor, ok := fragment.authorize(writer, request, PermissionAdminWrite, true)
	if !ok {
		return
	}
	if request.URL.RawQuery != "" {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	var body createRequest
	if err := decodeStrictJSON(request, &body); err != nil {
		writeDecodeError(writer, err)
		return
	}
	if body.ExpectedVersion == nil || body.Name == nil || body.Title == nil || body.DestinationURL == nil {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	result, err := fragment.application.Create(request.Context(), radarport.CreateCommand{
		ExpectedVersion: *body.ExpectedVersion,
		Name:            *body.Name,
		Title:           *body.Title,
		DestinationURL:  *body.DestinationURL,
		CoverImageID:    body.CoverImageID,
		AttachmentID:    body.AttachmentID,
		ActorID:         actor.ID,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeApplicationError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusCreated, result)
}

func (fragment *RouteFragment) update(writer stdhttp.ResponseWriter, request *stdhttp.Request, id radarport.LinkID) {
	actor, ok := fragment.authorize(writer, request, PermissionAdminWrite, true)
	if !ok {
		return
	}
	if request.URL.RawQuery != "" {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	var body updateRequest
	if err := decodeStrictJSON(request, &body); err != nil {
		writeDecodeError(writer, err)
		return
	}
	if body.ExpectedVersion == nil {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	result, err := fragment.application.Update(request.Context(), radarport.UpdateCommand{
		LinkID:          id,
		ExpectedVersion: *body.ExpectedVersion,
		Name:            radarport.OptionalString{Set: body.Name.Set, Value: body.Name.Value},
		Title:           radarport.OptionalString{Set: body.Title.Set, Value: body.Title.Value},
		DestinationURL:  radarport.OptionalString{Set: body.DestinationURL.Set, Value: body.DestinationURL.Value},
		CoverImageID:    radarport.OptionalNullableID{Set: body.CoverImageID.Set, Value: body.CoverImageID.Value},
		AttachmentID:    radarport.OptionalNullableID{Set: body.AttachmentID.Set, Value: body.AttachmentID.Value},
		ActorID:         actor.ID,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeApplicationError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, result)
}

func (fragment *RouteFragment) setStatus(writer stdhttp.ResponseWriter, request *stdhttp.Request, id radarport.LinkID, action string) {
	actor, ok := fragment.authorize(writer, request, PermissionAdminWrite, true)
	if !ok {
		return
	}
	if request.URL.RawQuery != "" {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	var body versionRequest
	if err := decodeStrictJSON(request, &body); err != nil {
		writeDecodeError(writer, err)
		return
	}
	if body.ExpectedVersion == nil {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	target := radarport.StatusDisabled
	if action == "enable" {
		target = radarport.StatusEnabled
	}
	result, err := fragment.application.SetStatus(request.Context(), radarport.SetStatusCommand{
		LinkID:          id,
		ExpectedVersion: *body.ExpectedVersion,
		Target:          target,
		ActorID:         actor.ID,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeApplicationError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, result)
}

func (fragment *RouteFragment) share(writer stdhttp.ResponseWriter, request *stdhttp.Request, id radarport.LinkID) {
	if _, ok := fragment.authorize(writer, request, PermissionAdminRead, false); !ok {
		return
	}
	if request.URL.RawQuery != "" || requireEmptyBody(request) != nil {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	projection, err := fragment.application.Share(request.Context(), id)
	if err != nil {
		writeApplicationError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, projection)
}

func (fragment *RouteFragment) serveOptions(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if request.Method != stdhttp.MethodGet {
		writeMethodNotAllowed(writer, "GET")
		return
	}
	if _, ok := fragment.authorize(writer, request, PermissionAdminRead, false); !ok {
		return
	}
	if request.URL.RawQuery != "" || requireEmptyBody(request) != nil {
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, fragment.application.Options(request.Context()))
}

type createRequest struct {
	ExpectedVersion *int64  `json:"expected_version"`
	Name            *string `json:"name"`
	Title           *string `json:"title"`
	DestinationURL  *string `json:"destination_url"`
	CoverImageID    *int64  `json:"cover_image_id"`
	AttachmentID    *int64  `json:"attachment_id"`
}

type updateRequest struct {
	ExpectedVersion *int64             `json:"expected_version"`
	Name            optionalString     `json:"name"`
	Title           optionalString     `json:"title"`
	DestinationURL  optionalString     `json:"destination_url"`
	CoverImageID    optionalNullableID `json:"cover_image_id"`
	AttachmentID    optionalNullableID `json:"attachment_id"`
}

type versionRequest struct {
	ExpectedVersion *int64 `json:"expected_version"`
}

func (fragment *RouteFragment) authorize(writer stdhttp.ResponseWriter, request *stdhttp.Request, permission Permission, csrfRequired bool) (Actor, bool) {
	actor, err := fragment.authorizer.Authorize(request.Context(), permission)
	if err != nil || actor.ID < 1 {
		switch {
		case errors.Is(err, ErrUnauthenticated):
			writeError(writer, stdhttp.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.", nil)
		case errors.Is(err, ErrForbidden), err == nil:
			writeError(writer, stdhttp.StatusForbidden, "UNAUTHORIZED", "Permission is denied.", nil)
		default:
			writeError(writer, stdhttp.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required dependency is unavailable.", nil)
		}
		return Actor{}, false
	}
	if csrfRequired {
		if err = fragment.csrf.Verify(request); err != nil {
			if errors.Is(err, ErrCSRFInvalid) {
				writeError(writer, stdhttp.StatusForbidden, "CSRF_INVALID", "CSRF validation failed.", nil)
			} else {
				writeError(writer, stdhttp.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required dependency is unavailable.", nil)
			}
			return Actor{}, false
		}
	}
	return actor, true
}

func parseListQuery(raw string) (radarport.ListInput, error) {
	input := radarport.ListInput{Status: radarport.StatusFilterAll, Sort: radarport.SortUpdatedDesc, Limit: radarport.DefaultLimit}
	if raw == "" {
		return input, nil
	}
	seen := make(map[string]struct{})
	for _, part := range strings.Split(raw, "&") {
		if part == "" || strings.Count(part, "=") != 1 || strings.ContainsAny(part, "+%") {
			return radarport.ListInput{}, errors.New("invalid query")
		}
		key, value, _ := strings.Cut(part, "=")
		if key == "" || value == "" {
			return radarport.ListInput{}, errors.New("invalid query")
		}
		if _, duplicate := seen[key]; duplicate {
			return radarport.ListInput{}, errors.New("invalid query")
		}
		seen[key] = struct{}{}
		switch key {
		case "status":
			input.Status = radarport.StatusFilter(value)
			if !input.Status.Valid() {
				return radarport.ListInput{}, errors.New("invalid query")
			}
		case "sort":
			input.Sort = radarport.Sort(value)
			if !input.Sort.Valid() {
				return radarport.ListInput{}, errors.New("invalid query")
			}
		case "limit":
			parsed, ok := canonicalNonNegativeInt32(value)
			if !ok || parsed < 1 || parsed > radarport.MaximumLimit {
				return radarport.ListInput{}, errors.New("invalid query")
			}
			input.Limit = parsed
		case "offset":
			parsed, ok := canonicalNonNegativeInt32(value)
			if !ok || parsed > radarport.MaximumOffset {
				return radarport.ListInput{}, errors.New("invalid query")
			}
			input.Offset = parsed
		default:
			return radarport.ListInput{}, errors.New("invalid query")
		}
	}
	return input, nil
}

func canonicalPositiveID(value string) (int64, bool) {
	if value == "" || value == "0" || value[0] == '0' {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0
}

func canonicalNonNegativeInt32(value string) (int32, bool) {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int32(parsed), err == nil && parsed >= 0
}

func idempotencyKey(request *stdhttp.Request) (string, bool) {
	if request == nil {
		return "", false
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || len(values[0]) < radarport.MinimumIdempotencyKeyBytes || len(values[0]) > radarport.MaximumIdempotencyKeyBytes || strings.TrimSpace(values[0]) != values[0] {
		return "", false
	}
	for _, character := range []byte(values[0]) {
		if character < 0x21 || character > 0x7e || character == ',' {
			return "", false
		}
	}
	return values[0], true
}

func requireEmptyBody(request *stdhttp.Request) error {
	if request == nil || request.Body == nil || request.Body == stdhttp.NoBody {
		return nil
	}
	if request.ContentLength > 0 {
		return errors.New("body not allowed")
	}
	var one [1]byte
	count, err := request.Body.Read(one[:])
	if count != 0 || err != nil && err != io.EOF {
		return errors.New("body not allowed")
	}
	return nil
}

func writeDecodeError(writer stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errRequestBodyTooLarge):
		writeError(writer, stdhttp.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "The request body is too large.", nil)
	case errors.Is(err, errUnsupportedMedia):
		writeError(writer, stdhttp.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json.", nil)
	default:
		writeError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.", nil)
	}
}

func writeApplicationError(writer stdhttp.ResponseWriter, err error) {
	var validation *radarport.ValidationError
	switch {
	case errors.As(err, &validation):
		writeError(writer, stdhttp.StatusUnprocessableEntity, "VALIDATION_FAILED", "Validation failed.", []fieldError{{Field: validation.Field, Reason: validation.Reason}})
	case errors.Is(err, radarport.ErrNotFound):
		writeError(writer, stdhttp.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
	case errors.Is(err, radarport.ErrConflict), errors.Is(err, radarport.ErrStateConflict), errors.Is(err, radarport.ErrIdempotencyConflict):
		writeError(writer, stdhttp.StatusConflict, "CONFLICT", "The request conflicts with the current state.", nil)
	case errors.Is(err, radarport.ErrUnavailable), errors.Is(err, radarport.ErrIdempotencyStateInvalid):
		writeError(writer, stdhttp.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required dependency is unavailable.", nil)
	default:
		writeError(writer, stdhttp.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred.", nil)
	}
}

type fieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type errorResponse struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []fieldError `json:"details,omitempty"`
}

func writeError(writer stdhttp.ResponseWriter, status int, code, message string, details []fieldError) {
	if writer == nil {
		return
	}
	setResponseHeaders(writer)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(errorResponse{Code: code, Message: message, Details: details})
}

func writeMethodNotAllowed(writer stdhttp.ResponseWriter, allow string) {
	writer.Header().Set("Allow", allow)
	writeError(writer, stdhttp.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The method is not allowed.", nil)
}

func writeJSON(writer stdhttp.ResponseWriter, status int, value any) {
	setResponseHeaders(writer)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func setResponseHeaders(writer stdhttp.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}
