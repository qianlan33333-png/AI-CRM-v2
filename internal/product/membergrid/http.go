package membergrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const maximumQueryBodyBytes int64 = 8 << 10

// RoutePrefix is the exact central mount point for the four member-grid routes.
const RoutePrefix = "/api/admin/service-period-products"

var canonicalProductID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
var canonicalInteger = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

type Application interface {
	Access(context.Context, int64) (AccessResponse, error)
	Schema(context.Context, int64) (SchemaResponse, error)
	MemberViews(context.Context, int64) (MemberViewsResponse, error)
	Query(context.Context, QueryInput) (QueryResponse, error)
}

type Handler struct {
	application Application
}

func NewHandler(application Application) (*Handler, error) {
	if nilDependency(application) {
		return nil, errors.New("member grid application is required")
	}
	return &Handler{application: application}, nil
}

func (handler *Handler) Access(writer http.ResponseWriter, request *http.Request, rawProductID string) {
	if !requireMethod(writer, request, http.MethodGet) || !authorize(writer, request, authport.CapabilityProductsRead) {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	response, err := handler.application.Access(request.Context(), productID)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *Handler) Schema(writer http.ResponseWriter, request *http.Request, rawProductID string) {
	if !requireMethod(writer, request, http.MethodGet) || !authorize(writer, request, authport.CapabilityProductsRead) {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	response, err := handler.application.Schema(request.Context(), productID)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *Handler) MemberViews(writer http.ResponseWriter, request *http.Request, rawProductID string) {
	if !requireMethod(writer, request, http.MethodGet) || !authorize(writer, request, authport.CapabilityProductsRead) {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	response, err := handler.application.MemberViews(request.Context(), productID)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *Handler) Query(writer http.ResponseWriter, request *http.Request, rawProductID string) {
	if !requireMethod(writer, request, http.MethodPost) || !authorize(writer, request, authport.CapabilityEntitlementsRead) {
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	input, problem := decodeQueryBody(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	input.ProductID = productID
	response, err := handler.application.Query(request.Context(), input)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

type routeFragment struct {
	handler *Handler
}

func NewRouteFragment(handler *Handler) (http.Handler, error) {
	if handler == nil || nilDependency(handler.application) {
		return nil, errors.New("member grid handler is required")
	}
	return &routeFragment{handler: handler}, nil
}

// ServeHTTP accepts the exact full prefix or the relative path supplied by a
// stripping mount. It owns only the four routes documented in ROUTE_FRAGMENT.md.
func (fragment *routeFragment) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setSecurityHeaders(writer)
	if fragment == nil || fragment.handler == nil || request == nil || request.URL == nil {
		writeFailure(writer, request, ErrUnavailable)
		return
	}
	if request.URL.RawQuery != "" || request.URL.RawPath != "" || !strings.HasPrefix(request.URL.Path, "/") ||
		strings.HasSuffix(request.URL.Path, "/") {
		writeProblem(writer, request, malformed("path", "invalid", ErrInvalidQuery))
		return
	}
	path := request.URL.Path
	if strings.HasPrefix(path, RoutePrefix+"/") {
		path = strings.TrimPrefix(path, RoutePrefix)
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	switch {
	case len(segments) == 3 && segments[1] == "member-grid" && segments[2] == "access":
		fragment.handler.Access(writer, request, segments[0])
	case len(segments) == 3 && segments[1] == "member-grid" && segments[2] == "schema":
		fragment.handler.Schema(writer, request, segments[0])
	case len(segments) == 2 && segments[1] == "member-views":
		fragment.handler.MemberViews(writer, request, segments[0])
	case len(segments) == 3 && segments[1] == "member-grid" && segments[2] == "query":
		fragment.handler.Query(writer, request, segments[0])
	default:
		writeCode(writer, request, platformhttp.CodeNotFound, errors.New("member grid route not found"))
	}
}

func parseProductID(raw string) (int64, error) {
	if !canonicalProductID.MatchString(raw) {
		return 0, ErrInvalidProductID
	}
	productID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || productID < 1 {
		return 0, ErrInvalidProductID
	}
	return productID, nil
}

type requestProblem struct {
	code   platformhttp.ErrorCode
	cause  error
	field  string
	reason string
}

func decodeQueryBody(writer http.ResponseWriter, request *http.Request) (QueryInput, *requestProblem) {
	input := QueryInput{State: StateAll, Source: SourceAny, Limit: DefaultLimit}
	if request == nil || request.Body == nil {
		return QueryInput{}, malformed("body", "required", errors.New("query body is required"))
	}
	if contentType := strings.TrimSpace(request.Header.Get("Content-Type")); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			return QueryInput{}, malformed("body", "invalid_content_type", errors.New("application/json is required"))
		}
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maximumQueryBodyBytes))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil {
		return QueryInput{}, malformed("body", "invalid_json", err)
	}
	openingDelimiter, ok := opening.(json.Delim)
	if !ok || openingDelimiter != '{' {
		return QueryInput{}, malformed("body", "object_required", errors.New("query body must be an object"))
	}

	seen := make(map[string]struct{}, 7)
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		key, ok := keyToken.(string)
		if tokenErr != nil || !ok {
			return QueryInput{}, malformed("body", "invalid_json", tokenErr)
		}
		if _, duplicate := seen[key]; duplicate {
			return QueryInput{}, malformed(key, "duplicate", errors.New("duplicate query field"))
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return QueryInput{}, malformed(key, "invalid", err)
		}
		switch key {
		case "state":
			var state string
			if err = json.Unmarshal(raw, &state); err != nil {
				return QueryInput{}, validation("state", "invalid", err)
			}
			input.State = StateFilter(state)
			if !input.State.validCanonicalGridState() {
				return QueryInput{}, validation("state", "unsupported", ErrInvalidQuery)
			}
		case "source":
			var source string
			if err = json.Unmarshal(raw, &source); err != nil {
				return QueryInput{}, validation("source", "invalid", err)
			}
			input.Source = SourceFilter(source)
			if input.Source != SourceManual && input.Source != SourcePaidOrder {
				return QueryInput{}, validation("source", "unsupported", ErrInvalidQuery)
			}
		case "limit":
			text := string(raw)
			if !canonicalInteger.MatchString(text) {
				return QueryInput{}, validation("limit", "invalid", ErrInvalidQuery)
			}
			limit, parseErr := strconv.ParseInt(text, 10, 32)
			if parseErr != nil || limit < 1 || limit > MaximumLimit {
				return QueryInput{}, validation("limit", "out_of_range", ErrInvalidQuery)
			}
			input.Limit = int(limit)
		case "cursor":
			if string(raw) == "null" {
				input.Cursor = ""
				continue
			}
			if err = json.Unmarshal(raw, &input.Cursor); err != nil || len(input.Cursor) > 256 {
				return QueryInput{}, &requestProblem{
					code: platformhttp.CodeCursorInvalid, cause: ErrInvalidCursor,
					field: "cursor", reason: "invalid",
				}
			}
		case "sort":
			if err = json.Unmarshal(raw, &input.Sort); err != nil || (input.Sort != string(querySortUpdatedAtDesc) && input.Sort != string(querySortStartsAtDesc)) {
				return QueryInput{}, validation("sort", "unsupported", ErrInvalidQuery)
			}
		case "group_by":
			if err = json.Unmarshal(raw, &input.GroupBy); err != nil || input.GroupBy != string(queryGroupState) {
				return QueryInput{}, validation("group_by", "unsupported", ErrInvalidQuery)
			}
		case "view_id":
			if err = json.Unmarshal(raw, &input.ViewID); err != nil || input.ViewID != "default" {
				return QueryInput{}, validation("view_id", "unsupported", ErrInvalidQuery)
			}
		default:
			return QueryInput{}, malformed("body", "unknown_field", errors.New("unknown query field"))
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return QueryInput{}, malformed("body", "invalid_json", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return QueryInput{}, malformed("body", "trailing_data", err)
	}
	return input, nil
}

func malformed(field, reason string, cause error) *requestProblem {
	return &requestProblem{code: platformhttp.CodeMalformedRequest, cause: cause, field: field, reason: reason}
}

func validation(field, reason string, cause error) *requestProblem {
	return &requestProblem{code: platformhttp.CodeValidationFailed, cause: cause, field: field, reason: reason}
}

func authorize(writer http.ResponseWriter, request *http.Request, capability authport.Capability) bool {
	if request == nil {
		writeCode(writer, request, platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
		return false
	}
	principal, authenticated := authport.PrincipalFromContext(request.Context())
	if !authenticated {
		writeCode(writer, request, platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
		return false
	}
	if principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps {
		writeCode(writer, request, platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		return false
	}
	authorization, allowed := authport.AuthorizationFromContext(request.Context())
	if !allowed || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal ||
		authorization.OwnerStaffID != 0 {
		writeCode(writer, request, platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		return false
	}
	return true
}

func requireMethod(writer http.ResponseWriter, request *http.Request, method string) bool {
	if request != nil && request.Method == method {
		return true
	}
	setSecurityHeaders(writer)
	writer.Header().Set("Allow", method)
	writer.WriteHeader(http.StatusMethodNotAllowed)
	return false
}

func writeFailure(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, authport.ErrUnauthenticated):
		writeCode(writer, request, platformhttp.CodeUnauthenticated, err)
	case errors.Is(err, authport.ErrUnauthorized):
		writeCode(writer, request, platformhttp.CodeUnauthorized, err)
	case errors.Is(err, ErrInvalidProductID):
		writeProblem(writer, request, malformed("service_product_id", "invalid", err))
	case errors.Is(err, ErrInvalidCursor):
		writeProblem(writer, request, &requestProblem{
			code: platformhttp.CodeCursorInvalid, cause: err, field: "cursor", reason: "invalid",
		})
	case errors.Is(err, ErrInvalidQuery):
		writeProblem(writer, request, validation("query", "invalid", err))
	case errors.Is(err, ErrNotFound):
		writeCode(writer, request, platformhttp.CodeNotFound, err)
	default:
		writeCode(writer, request, platformhttp.CodeDependencyUnavailable, err)
	}
}

func writeProblem(writer http.ResponseWriter, request *http.Request, problem *requestProblem) {
	if problem == nil {
		writeCode(writer, request, platformhttp.CodeDependencyUnavailable, ErrUnavailable)
		return
	}
	detail := platformhttp.FieldError{Field: problem.field, Reason: problem.reason}
	writeCode(writer, request, problem.code, platformhttp.NewError(problem.code, problem.cause, detail))
}

func writeCode(writer http.ResponseWriter, request *http.Request, code platformhttp.ErrorCode, err error) {
	if writer == nil || request == nil {
		return
	}
	platformhttp.MarkCompatibilityError(writer, code)
	httpError := err
	if platformhttp.ErrorCodeOf(err) != code {
		httpError = platformhttp.NewError(code, err)
	}
	platformhttp.WriteError(&privateResponseWriter{ResponseWriter: writer}, request, httpError)
}

type privateResponseWriter struct {
	http.ResponseWriter
}

func (writer *privateResponseWriter) WriteHeader(status int) {
	setSecurityHeaders(writer.ResponseWriter)
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *privateResponseWriter) Write(data []byte) (int, error) {
	setSecurityHeaders(writer.ResponseWriter)
	return writer.ResponseWriter.Write(data)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	setSecurityHeaders(writer)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func setSecurityHeaders(writer http.ResponseWriter) {
	if writer == nil {
		return
	}
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func (problem *requestProblem) Error() string {
	if problem == nil {
		return "member grid request problem"
	}
	return fmt.Sprintf("%s: %s", problem.field, problem.reason)
}
