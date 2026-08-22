package legacyaudiencemembers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	canonicalPackageID = regexp.MustCompile(`^[1-9][0-9]*$`)
	canonicalLimit     = regexp.MustCompile(`^[1-9][0-9]*$`)
	canonicalOffset    = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	requestIDPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
)

type Handler struct {
	application Application
	security    Security
}

func NewHandler(application Application, security Security) (*Handler, error) {
	if nilInterface(application) || nilInterface(security) {
		return nil, ErrUnavailable
	}
	return &Handler{application: application, security: security}, nil
}

var _ http.Handler = (*routeFragment)(nil)

type routeFragment struct {
	handler *Handler
}

func NewRouteFragment(handler *Handler) (http.Handler, error) {
	if handler == nil || nilInterface(handler.application) || nilInterface(handler.security) {
		return nil, ErrUnavailable
	}
	return &routeFragment{handler: handler}, nil
}

func (fragment *routeFragment) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setSecurityHeaders(writer)
	if fragment == nil || fragment.handler == nil || request == nil || request.URL == nil {
		writeFailure(writer, request, ErrUnavailable)
		return
	}

	rawPackageID, matched := matchOwnedRoute(request)
	if !matched {
		writeHTTPError(writer, request, http.StatusNotFound, "NOT_FOUND", "The resource was not found.")
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeHTTPError(writer, request, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The method is not allowed.")
		return
	}
	if err := fragment.handler.security.Authorize(request, AccessRequirement{
		Capability:  CapabilitySegmentsRead,
		RequireCSRF: false,
	}); err != nil {
		writeFailure(writer, request, err)
		return
	}

	packageID, ok := parsePackageID(rawPackageID)
	if !ok {
		writeHTTPError(writer, request, http.StatusNotFound, "NOT_FOUND", "The resource was not found.")
		return
	}
	limit, offset, ok := parsePageQuery(request.URL.RawQuery)
	if !ok {
		writeHTTPError(writer, request, http.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.")
		return
	}

	response, err := fragment.handler.application.ListMembers(request.Context(), ListInput{
		PackageID: packageID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func matchOwnedRoute(request *http.Request) (string, bool) {
	if request == nil || request.URL == nil || request.URL.Path == "" || request.URL.RawPath != "" {
		return "", false
	}
	path := request.URL.Path
	if !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") ||
		strings.Contains(path, "\\") || strings.Contains(path, "//") {
		return "", false
	}
	rawTarget := request.RequestURI
	if separator := strings.IndexByte(rawTarget, '?'); separator >= 0 {
		rawTarget = rawTarget[:separator]
	}
	if strings.Contains(rawTarget, "%") {
		return "", false
	}
	if strings.HasPrefix(path, RoutePrefix+"/") {
		path = strings.TrimPrefix(path, RoutePrefix)
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "packages" || parts[2] != "members" ||
		parts[1] == "" || parts[1] == "." || parts[1] == ".." {
		return "", false
	}
	return parts[1], true
}

func parsePackageID(raw string) (int64, bool) {
	if !canonicalPackageID.MatchString(raw) || len(raw) > 19 {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	return value, err == nil && value > 0
}

func parsePageQuery(raw string) (int, int64, bool) {
	limit, offset := DefaultLimit, int64(0)
	if raw == "" {
		return limit, offset, true
	}
	if strings.HasPrefix(raw, "&") || strings.HasSuffix(raw, "&") || strings.Contains(raw, "&&") {
		return 0, 0, false
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return 0, 0, false
	}
	for key, entries := range values {
		if !utf8.ValidString(key) || key == "" || (key != "limit" && key != "offset") {
			return 0, 0, false
		}
		if len(entries) != 1 || entries[0] == "" || !utf8.ValidString(entries[0]) {
			return 0, 0, false
		}
	}
	if entries, exists := values["limit"]; exists {
		if !canonicalLimit.MatchString(entries[0]) {
			return 0, 0, false
		}
		parsed, parseErr := strconv.ParseInt(entries[0], 10, 32)
		if parseErr != nil || parsed < 1 || parsed > MaximumLimit {
			return 0, 0, false
		}
		limit = int(parsed)
	}
	if entries, exists := values["offset"]; exists {
		if !canonicalOffset.MatchString(entries[0]) {
			return 0, 0, false
		}
		parsed, parseErr := strconv.ParseUint(entries[0], 10, 63)
		if parseErr != nil || parsed > math.MaxInt64 {
			return 0, 0, false
		}
		offset = int64(parsed)
	}
	return limit, offset, true
}

type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeFailure(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ErrUnauthenticated):
		writeHTTPError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
	case errors.Is(err, ErrForbidden):
		writeHTTPError(writer, request, http.StatusForbidden, "UNAUTHORIZED", "Permission is denied.")
	case errors.Is(err, ErrInvalidInput):
		writeHTTPError(writer, request, http.StatusBadRequest, "MALFORMED_REQUEST", "The request is malformed.")
	case errors.Is(err, ErrNotFound):
		writeHTTPError(writer, request, http.StatusNotFound, "NOT_FOUND", "The resource was not found.")
	default:
		writeHTTPError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required dependency is unavailable.")
	}
}

func writeHTTPError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	if writer == nil {
		return
	}
	setSecurityHeaders(writer)
	requestID := requestIdentifier(request)
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Request-ID", requestID)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(errorResponse{Code: code, Message: message, RequestID: requestID})
}

func writeJSON(writer http.ResponseWriter, status int, value ListResponse) {
	if writer == nil {
		return
	}
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

// WriteMethodNotAllowed preserves the owned route's closed error envelope when
// the central Chi router rejects a non-GET method before authentication.
func WriteMethodNotAllowed(writer http.ResponseWriter, request *http.Request) {
	if writer == nil {
		return
	}
	writer.Header().Set("Allow", http.MethodGet)
	writeHTTPError(writer, request, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The method is not allowed.")
}

func requestIdentifier(request *http.Request) string {
	if request != nil {
		candidate := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestIDPattern.MatchString(candidate) {
			return candidate
		}
	}
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("local-%x", buffer)
}
