package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	ChannelEntrantsRoutePrefix       = "/api/admin/channels"
	channelEntrantsMaximumRawQuery   = 1024
	channelEntrantsMaximumCursorSize = 256
)

var (
	channelEntrantsCanonicalID      = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
	channelEntrantsCanonicalInteger = regexp.MustCompile(`^[1-9][0-9]*$`)
)

type channelEntrantsApplication interface {
	List(context.Context, contactapp.ChannelEntrantsInput) (contactapp.ChannelEntrantsResponse, error)
}

type ChannelEntrantsHandler struct {
	application channelEntrantsApplication
}

func NewChannelEntrantsHandler(application channelEntrantsApplication) (*ChannelEntrantsHandler, error) {
	if channelEntrantsNilApplication(application) {
		return nil, errors.New("channel entrants application is required")
	}
	return &ChannelEntrantsHandler{application: application}, nil
}

// ListChannelContacts is the leaf handler for the legacy-compatible local read.
func (handler *ChannelEntrantsHandler) ListChannelContacts(
	writer http.ResponseWriter,
	request *http.Request,
	rawChannelID string,
) {
	channelEntrantsSetSecurityHeaders(writer)
	if handler == nil || channelEntrantsNilApplication(handler.application) || request == nil {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelEntrantsUnavailable)
		return
	}
	if !channelEntrantsRequireMethod(writer, request, http.MethodGet) ||
		!channelEntrantsAuthorize(writer, request) {
		return
	}
	channelID, err := channelEntrantsParseID(rawChannelID)
	if err != nil {
		channelEntrantsWriteProblem(writer, request, channelEntrantsValidationProblem("channel_id", "invalid", err))
		return
	}
	input, problem := channelEntrantsParseQuery(request.URL)
	if problem != nil {
		channelEntrantsWriteProblem(writer, request, problem)
		return
	}
	input.ChannelID = channelID
	response, err := handler.application.List(request.Context(), input)
	if err != nil {
		channelEntrantsWriteFailure(writer, request, err)
		return
	}
	if !channelEntrantsValidResponse(response, input) {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelEntrantsUnavailable)
		return
	}
	channelEntrantsWriteJSON(writer, http.StatusOK, response)
}

type channelEntrantsRouteFragment struct {
	handler *ChannelEntrantsHandler
}

func NewChannelEntrantsRouteFragment(handler *ChannelEntrantsHandler) (http.Handler, error) {
	if handler == nil || channelEntrantsNilApplication(handler.application) {
		return nil, errors.New("channel entrants handler is required")
	}
	return &channelEntrantsRouteFragment{handler: handler}, nil
}

// ServeHTTP owns only GET /api/admin/channels/{channel_id}/contacts. It accepts
// either the full path or the relative path supplied by a stripping mount.
func (fragment *channelEntrantsRouteFragment) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	channelEntrantsSetSecurityHeaders(writer)
	if fragment == nil || fragment.handler == nil || request == nil || request.URL == nil {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelEntrantsUnavailable)
		return
	}
	if request.URL.RawPath != "" || !strings.HasPrefix(request.URL.Path, "/") ||
		strings.HasSuffix(request.URL.Path, "/") || strings.Contains(request.URL.Path, "\\") {
		channelEntrantsWriteProblem(writer, request, channelEntrantsValidationProblem("path", "invalid", errors.New("invalid channel entrants path")))
		return
	}
	path := request.URL.Path
	if strings.HasPrefix(path, ChannelEntrantsRoutePrefix+"/") {
		path = strings.TrimPrefix(path, ChannelEntrantsRoutePrefix)
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) != 2 || segments[1] != "contacts" || segments[0] == "" {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeNotFound, errors.New("channel entrants route not found"))
		return
	}
	fragment.handler.ListChannelContacts(writer, request, segments[0])
}

func channelEntrantsParseID(raw string) (int64, error) {
	if !channelEntrantsCanonicalID.MatchString(raw) {
		return 0, contactapp.ErrInvalidChannelEntrantsQuery
	}
	channelID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || channelID < 1 {
		return 0, contactapp.ErrInvalidChannelEntrantsQuery
	}
	return channelID, nil
}

type channelEntrantsRequestProblem struct {
	code   platformhttp.ErrorCode
	cause  error
	field  string
	reason string
}

func channelEntrantsParseQuery(target *url.URL) (contactapp.ChannelEntrantsInput, *channelEntrantsRequestProblem) {
	input := contactapp.ChannelEntrantsInput{Limit: contactapp.ChannelEntrantsDefaultLimit}
	if target == nil {
		return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem("query", "invalid", errors.New("request URL is required"))
	}
	if len(target.RawQuery) > channelEntrantsMaximumRawQuery {
		return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem("query", "too_long", errors.New("query is too long"))
	}
	values, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem("query", "invalid_encoding", err)
	}
	for key, entries := range values {
		if !utf8.ValidString(key) {
			return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem("query", "invalid_encoding", errors.New("query key is not UTF-8"))
		}
		if key != "limit" && key != "cursor" {
			return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem("query", "unknown_field", errors.New("unknown channel entrants query field"))
		}
		if len(entries) != 1 {
			return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem(key, "duplicate", errors.New("duplicate channel entrants query field"))
		}
		value := entries[0]
		if !utf8.ValidString(value) {
			return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem(key, "invalid_encoding", errors.New("query value is not UTF-8"))
		}
		switch key {
		case "limit":
			if !channelEntrantsCanonicalInteger.MatchString(value) {
				return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem("limit", "invalid", contactapp.ErrInvalidChannelEntrantsQuery)
			}
			limit, parseErr := strconv.ParseInt(value, 10, 32)
			if parseErr != nil || limit < 1 || limit > contactapp.ChannelEntrantsMaximumLimit {
				return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem("limit", "out_of_range", contactapp.ErrInvalidChannelEntrantsQuery)
			}
			input.Limit = int(limit)
		case "cursor":
			if value == "" || len(value) > channelEntrantsMaximumCursorSize {
				return contactapp.ChannelEntrantsInput{}, channelEntrantsValidationProblem("cursor", "invalid", contactapp.ErrInvalidChannelEntrantsCursor)
			}
			input.Cursor = value
		}
	}
	return input, nil
}

func channelEntrantsAuthorize(writer http.ResponseWriter, request *http.Request) bool {
	if request == nil {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
		return false
	}
	principal, authenticated := authport.PrincipalFromContext(request.Context())
	if !authenticated || principal.AdminUserID < 1 {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
		return false
	}
	if principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		return false
	}
	authorization, authorized := authport.AuthorizationFromContext(request.Context())
	if !authorized || authorization.Capability != authport.CapabilityCustomersRead ||
		authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		return false
	}
	return true
}

func channelEntrantsRequireMethod(writer http.ResponseWriter, request *http.Request, method string) bool {
	if request != nil && request.Method == method {
		return true
	}
	channelEntrantsSetSecurityHeaders(writer)
	if writer != nil {
		writer.Header().Set("Allow", method)
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
	return false
}

func channelEntrantsValidResponse(
	response contactapp.ChannelEntrantsResponse,
	input contactapp.ChannelEntrantsInput,
) bool {
	if response.ChannelID != input.ChannelID || response.Limit != input.Limit ||
		response.Items == nil || len(response.Items) > response.Limit ||
		!response.LocalProjection || response.RealExternalCallExecuted ||
		len(response.NextCursor) > channelEntrantsMaximumCursorSize || !utf8.ValidString(response.NextCursor) ||
		(response.HasMore && (len(response.Items) != response.Limit || response.NextCursor == "")) ||
		(!response.HasMore && response.NextCursor != "") {
		return false
	}
	seen := make(map[int64]struct{}, len(response.Items))
	var previous *contactapp.ChannelEntrantItem
	for index := range response.Items {
		item := response.Items[index]
		if item.CustomerID < 1 || item.AddedAt.IsZero() || !utf8.ValidString(item.DisplayName) ||
			(item.LastInteractAt != nil && item.LastInteractAt.IsZero()) {
			return false
		}
		if _, duplicate := seen[item.CustomerID]; duplicate {
			return false
		}
		seen[item.CustomerID] = struct{}{}
		if previous != nil && !channelEntrantsHTTPItemBefore(item, *previous) {
			return false
		}
		previous = &response.Items[index]
	}
	return true
}

func channelEntrantsHTTPItemBefore(current, previous contactapp.ChannelEntrantItem) bool {
	if current.AddedAt.Before(previous.AddedAt) {
		return true
	}
	return current.AddedAt.Equal(previous.AddedAt) && current.CustomerID < previous.CustomerID
}

func channelEntrantsWriteFailure(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, contactapp.ErrInvalidChannelEntrantsCursor):
		channelEntrantsWriteProblem(writer, request, channelEntrantsValidationProblem("cursor", "invalid", err))
	case errors.Is(err, contactapp.ErrInvalidChannelEntrantsQuery):
		channelEntrantsWriteProblem(writer, request, channelEntrantsValidationProblem("query", "invalid", err))
	case errors.Is(err, contactapp.ErrChannelEntrantsNotFound):
		channelEntrantsWriteCode(writer, request, platformhttp.CodeNotFound, err)
	default:
		channelEntrantsWriteCode(writer, request, platformhttp.CodeDependencyUnavailable, err)
	}
}

func channelEntrantsValidationProblem(field, reason string, cause error) *channelEntrantsRequestProblem {
	return &channelEntrantsRequestProblem{
		code: platformhttp.CodeValidationFailed, cause: cause, field: field, reason: reason,
	}
}

func channelEntrantsWriteProblem(
	writer http.ResponseWriter,
	request *http.Request,
	problem *channelEntrantsRequestProblem,
) {
	if problem == nil {
		channelEntrantsWriteCode(writer, request, platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelEntrantsUnavailable)
		return
	}
	detail := platformhttp.FieldError{Field: problem.field, Reason: problem.reason}
	channelEntrantsWriteCode(writer, request, problem.code, platformhttp.NewError(problem.code, problem.cause, detail))
}

func channelEntrantsWriteCode(
	writer http.ResponseWriter,
	request *http.Request,
	code platformhttp.ErrorCode,
	err error,
) {
	if writer == nil {
		return
	}
	channelEntrantsSetSecurityHeaders(writer)
	if request == nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	platformhttp.MarkCompatibilityError(writer, code)
	httpError := err
	if platformhttp.ErrorCodeOf(err) != code {
		httpError = platformhttp.NewError(code, err)
	}
	platformhttp.WriteError(&channelEntrantsPrivateResponseWriter{ResponseWriter: writer}, request, httpError)
}

type channelEntrantsPrivateResponseWriter struct {
	http.ResponseWriter
}

func (writer *channelEntrantsPrivateResponseWriter) WriteHeader(status int) {
	channelEntrantsSetSecurityHeaders(writer.ResponseWriter)
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *channelEntrantsPrivateResponseWriter) Write(data []byte) (int, error) {
	channelEntrantsSetSecurityHeaders(writer.ResponseWriter)
	return writer.ResponseWriter.Write(data)
}

func channelEntrantsWriteJSON(writer http.ResponseWriter, status int, value any) {
	if writer == nil {
		return
	}
	channelEntrantsSetSecurityHeaders(writer)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func channelEntrantsSetSecurityHeaders(writer http.ResponseWriter) {
	if writer == nil {
		return
	}
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func channelEntrantsNilApplication(application channelEntrantsApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
