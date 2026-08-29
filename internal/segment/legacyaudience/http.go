package legacyaudience

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

var (
	canonicalID      = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
	canonicalInteger = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
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
	path, problem := ownedPath(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	switch {
	case path == "/package-groups":
		fragment.handler.packageGroups(writer, request)
	case len(segments) == 2 && segments[0] == "package-groups":
		fragment.handler.packageGroup(writer, request, segments[1])
	case path == "/packages":
		fragment.handler.packages(writer, request)
	case path == "/templates":
		fragment.handler.templates(writer, request)
	case len(segments) == 2 && segments[0] == "packages":
		fragment.handler.packageItem(writer, request, segments[1])
	case len(segments) == 3 && segments[0] == "packages" && (segments[2] == "copy" || segments[2] == "pause" || segments[2] == "activate"):
		fragment.handler.packageAction(writer, request, segments[1], segments[2])
	default:
		writeHTTPError(writer, request, http.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
	}
}

func (handler *Handler) templates(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, request, http.MethodGet)
		return
	}
	if !requireNoQuery(writer, request) || !handler.authorize(writer, request, false, nil) {
		return
	}
	writeJSON(writer, http.StatusOK, AudienceTemplateCatalogResponse{Items: ListAudienceTemplates(), Projection: localProjection()})
}

func ownedPath(request *http.Request) (string, *requestProblem) {
	if request.URL.RawPath != "" || request.URL.Path == "" || !strings.HasPrefix(request.URL.Path, "/") ||
		strings.HasSuffix(request.URL.Path, "/") || strings.Contains(request.URL.Path, "\\") || strings.Contains(request.URL.Path, "//") {
		return "", malformed("path", "invalid")
	}
	rawTarget := request.RequestURI
	if separator := strings.IndexByte(rawTarget, '?'); separator >= 0 {
		rawTarget = rawTarget[:separator]
	}
	if strings.Contains(rawTarget, "%") {
		return "", malformed("path", "encoded_path_forbidden")
	}
	path := request.URL.Path
	if path == RoutePrefix {
		return "", malformed("path", "incomplete")
	}
	if strings.HasPrefix(path, RoutePrefix+"/") {
		path = strings.TrimPrefix(path, RoutePrefix)
	}
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", malformed("path", "invalid")
		}
	}
	return path, nil
}

func (handler *Handler) packageGroups(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		if !requireNoQuery(writer, request) || !handler.authorize(writer, request, false, nil) {
			return
		}
		response, err := handler.application.ListGroups(request.Context())
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	case http.MethodPost:
		if !requireNoQuery(writer, request) {
			return
		}
		var actor Actor
		if !handler.authorize(writer, request, true, &actor) {
			return
		}
		key, problem := idempotencyKey(request)
		if problem != nil {
			writeProblem(writer, request, problem)
			return
		}
		input, problem := decodeCreateGroup(writer, request)
		if problem != nil {
			writeProblem(writer, request, problem)
			return
		}
		input.Actor, input.IdempotencyKey = actor, key
		response, err := handler.application.CreateGroup(request.Context(), input)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusCreated, response)
	default:
		writeMethodNotAllowed(writer, request, http.MethodGet+", "+http.MethodPost)
	}
}

func (handler *Handler) packageGroup(writer http.ResponseWriter, request *http.Request, rawID string) {
	if !requireNoQuery(writer, request) {
		return
	}
	switch request.Method {
	case http.MethodPatch, http.MethodDelete:
		var actor Actor
		if !handler.authorize(writer, request, true, &actor) {
			return
		}
		groupID, problem := parseID(rawID, "group_id")
		if problem != nil {
			writeProblem(writer, request, problem)
			return
		}
		key, problem := idempotencyKey(request)
		if problem != nil {
			writeProblem(writer, request, problem)
			return
		}
		if request.Method == http.MethodPatch {
			input, decodeProblem := decodeUpdateGroup(writer, request)
			if decodeProblem != nil {
				writeProblem(writer, request, decodeProblem)
				return
			}
			input.GroupID, input.Actor, input.IdempotencyKey = groupID, actor, key
			response, err := handler.application.UpdateGroup(request.Context(), input)
			if err != nil {
				writeFailure(writer, request, err)
				return
			}
			writeJSON(writer, http.StatusOK, response)
			return
		}
		input, decodeProblem := decodeDeleteGroup(writer, request)
		if decodeProblem != nil {
			writeProblem(writer, request, decodeProblem)
			return
		}
		input.GroupID, input.Actor, input.IdempotencyKey = groupID, actor, key
		response, err := handler.application.DeleteGroup(request.Context(), input)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	default:
		writeMethodNotAllowed(writer, request, http.MethodPatch+", "+http.MethodDelete)
	}
}

func (handler *Handler) packages(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, request, http.MethodGet)
		return
	}
	if !handler.authorize(writer, request, false, nil) {
		return
	}
	input, problem := parsePackageQuery(request.URL.RawQuery)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	response, err := handler.application.ListPackages(request.Context(), input)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *Handler) packageItem(writer http.ResponseWriter, request *http.Request, rawID string) {
	switch request.Method {
	case http.MethodGet:
		if !requireNoQuery(writer, request) || !handler.authorize(writer, request, false, nil) {
			return
		}
		packageID, problem := parseID(rawID, "package_id")
		if problem != nil {
			writeProblem(writer, request, problem)
			return
		}
		response, err := handler.application.GetPackage(request.Context(), packageID)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	case http.MethodPatch, http.MethodDelete:
		if !requireNoQuery(writer, request) {
			return
		}
		var actor Actor
		if !handler.authorize(writer, request, true, &actor) {
			return
		}
		packageID, problem := parseID(rawID, "package_id")
		if problem != nil {
			writeProblem(writer, request, problem)
			return
		}
		key, problem := idempotencyKey(request)
		if problem != nil {
			writeProblem(writer, request, problem)
			return
		}
		if request.Method == http.MethodPatch {
			input, decodeProblem := decodeUpdatePackage(writer, request)
			if decodeProblem != nil {
				writeProblem(writer, request, decodeProblem)
				return
			}
			input.PackageID, input.Actor, input.IdempotencyKey = packageID, actor, key
			response, err := handler.application.UpdatePackage(request.Context(), input)
			if err != nil {
				writeFailure(writer, request, err)
				return
			}
			writeJSON(writer, http.StatusOK, response)
			return
		}
		command, decodeProblem := decodePackageCommand(writer, request)
		if decodeProblem != nil {
			writeProblem(writer, request, decodeProblem)
			return
		}
		command.PackageID, command.Actor, command.IdempotencyKey = packageID, actor, key
		response, err := handler.application.ArchivePackage(request.Context(), command)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	default:
		writeMethodNotAllowed(writer, request, http.MethodGet+", "+http.MethodPatch+", "+http.MethodDelete)
	}
}

func (handler *Handler) packageAction(writer http.ResponseWriter, request *http.Request, rawID, action string) {
	if request.Method != http.MethodPost {
		writeMethodNotAllowed(writer, request, http.MethodPost)
		return
	}
	if !requireNoQuery(writer, request) {
		return
	}
	var actor Actor
	if !handler.authorize(writer, request, true, &actor) {
		return
	}
	packageID, problem := parseID(rawID, "package_id")
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	key, problem := idempotencyKey(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	command, problem := decodePackageCommand(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	command.PackageID, command.Actor, command.IdempotencyKey = packageID, actor, key
	var response PackageMutationResponse
	var err error
	switch action {
	case "copy":
		response, err = handler.application.CopyPackage(request.Context(), command)
	case "pause":
		response, err = handler.application.PausePackage(request.Context(), command)
	case "activate":
		response, err = handler.application.ActivatePackage(request.Context(), command)
	default:
		writeHTTPError(writer, request, http.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
		return
	}
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	status := http.StatusOK
	if action == "copy" {
		status = http.StatusCreated
	}
	writeJSON(writer, status, response)
}

func (handler *Handler) authorize(writer http.ResponseWriter, request *http.Request, write bool, actor *Actor) bool {
	requirement := AccessRequirement{Capability: CapabilitySegmentsRead}
	if write {
		requirement = AccessRequirement{Capability: CapabilitySegmentsWrite, RequireCSRF: true}
	}
	resolved, err := handler.security.Authorize(request, requirement)
	if err != nil {
		writeFailure(writer, request, err)
		return false
	}
	if write && resolved.AdminUserID <= 0 {
		writeFailure(writer, request, ErrForbidden)
		return false
	}
	if actor != nil {
		*actor = resolved
	}
	return true
}

type requestProblem struct {
	status int
	code   string
	field  string
	reason string
}

func malformed(field, reason string) *requestProblem {
	return &requestProblem{status: http.StatusBadRequest, code: "MALFORMED_REQUEST", field: field, reason: reason}
}

func validation(field, reason string) *requestProblem {
	return &requestProblem{status: http.StatusUnprocessableEntity, code: "VALIDATION_FAILED", field: field, reason: reason}
}

func decodeCreateGroup(writer http.ResponseWriter, request *http.Request) (CreateGroupInput, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"name": true, "sort_order": true, "expected_version": true})
	if problem != nil {
		return CreateGroupInput{}, problem
	}
	name, problem := requiredString(fields, "name")
	if problem != nil {
		return CreateGroupInput{}, problem
	}
	expected, problem := requiredInteger(fields, "expected_version", 0, 0)
	if problem != nil {
		return CreateGroupInput{}, problem
	}
	var sortOrder int64
	if raw, exists := fields["sort_order"]; exists {
		sortOrder, problem = integerValue(raw, "sort_order", 0, int64(maximumSortOrder))
		if problem != nil {
			return CreateGroupInput{}, problem
		}
	}
	return CreateGroupInput{Name: name, SortOrder: int32(sortOrder), ExpectedVersion: expected}, nil
}

func decodeUpdateGroup(writer http.ResponseWriter, request *http.Request) (UpdateGroupInput, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"name": true, "sort_order": true, "expected_version": true})
	if problem != nil {
		return UpdateGroupInput{}, problem
	}
	expected, problem := requiredInteger(fields, "expected_version", 1, 1<<62)
	if problem != nil {
		return UpdateGroupInput{}, problem
	}
	input := UpdateGroupInput{ExpectedVersion: expected}
	if raw, exists := fields["name"]; exists {
		value, parseProblem := stringValue(raw, "name")
		if parseProblem != nil {
			return UpdateGroupInput{}, parseProblem
		}
		input.Name = &value
	}
	if raw, exists := fields["sort_order"]; exists {
		value, parseProblem := integerValue(raw, "sort_order", 0, int64(maximumSortOrder))
		if parseProblem != nil {
			return UpdateGroupInput{}, parseProblem
		}
		converted := int32(value)
		input.SortOrder = &converted
	}
	if input.Name == nil && input.SortOrder == nil {
		return UpdateGroupInput{}, validation("body", "mutation_required")
	}
	return input, nil
}

func decodeDeleteGroup(writer http.ResponseWriter, request *http.Request) (DeleteGroupInput, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"expected_version": true})
	if problem != nil {
		return DeleteGroupInput{}, problem
	}
	expected, problem := requiredInteger(fields, "expected_version", 1, 1<<62)
	if problem != nil {
		return DeleteGroupInput{}, problem
	}
	return DeleteGroupInput{ExpectedVersion: expected}, nil
}

func decodePackageCommand(writer http.ResponseWriter, request *http.Request) (PackageCommand, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"expected_version": true})
	if problem != nil {
		return PackageCommand{}, problem
	}
	expected, problem := requiredInteger(fields, "expected_version", 1, 1<<62)
	if problem != nil {
		return PackageCommand{}, problem
	}
	return PackageCommand{ExpectedVersion: expected}, nil
}

func decodeUpdatePackage(writer http.ResponseWriter, request *http.Request) (UpdatePackageInput, *requestProblem) {
	allowed := map[string]bool{
		"name": true, "definition": true, "refresh_mode": true, "refresh_cron": true,
		"group_id": true, "expected_version": true,
	}
	fields, problem := decodeObject(writer, request, allowed)
	if problem != nil {
		return UpdatePackageInput{}, problem
	}
	expected, problem := requiredInteger(fields, "expected_version", 1, 1<<62)
	if problem != nil {
		return UpdatePackageInput{}, problem
	}
	input := UpdatePackageInput{ExpectedVersion: expected}
	if raw, exists := fields["name"]; exists {
		value, parseProblem := stringValue(raw, "name")
		if parseProblem != nil {
			return UpdatePackageInput{}, parseProblem
		}
		input.Name = &value
	}
	if raw, exists := fields["definition"]; exists {
		if len(raw) == 0 || raw[0] != '{' {
			return UpdatePackageInput{}, validation("definition", "object_required")
		}
		value := segmentport.Definition(append([]byte(nil), raw...))
		input.Definition = &value
	}
	if raw, exists := fields["refresh_mode"]; exists {
		value, parseProblem := stringValue(raw, "refresh_mode")
		if parseProblem != nil {
			return UpdatePackageInput{}, parseProblem
		}
		mode := segmentport.RefreshMode(value)
		if mode != segmentport.RefreshModeManual && mode != segmentport.RefreshModeScheduled {
			return UpdatePackageInput{}, validation("refresh_mode", "unsupported")
		}
		input.RefreshMode = &mode
	}
	if raw, exists := fields["refresh_cron"]; exists {
		input.RefreshCron.Set = true
		if string(raw) != "null" {
			value, parseProblem := stringValue(raw, "refresh_cron")
			if parseProblem != nil {
				return UpdatePackageInput{}, parseProblem
			}
			input.RefreshCron.Value = &value
		}
	}
	if raw, exists := fields["group_id"]; exists {
		input.GroupID.Set = true
		if string(raw) != "null" {
			value, parseProblem := integerValue(raw, "group_id", 1, 1<<62)
			if parseProblem != nil {
				return UpdatePackageInput{}, parseProblem
			}
			input.GroupID.Value = &value
		}
	}
	if input.Name == nil && input.Definition == nil && input.RefreshMode == nil && !input.RefreshCron.Set && !input.GroupID.Set {
		return UpdatePackageInput{}, validation("body", "mutation_required")
	}
	return input, nil
}

func decodeObject(writer http.ResponseWriter, request *http.Request, allowed map[string]bool) (map[string]json.RawMessage, *requestProblem) {
	if request == nil || request.Body == nil {
		return nil, malformed("body", "required")
	}
	contentTypes := request.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		return nil, &requestProblem{status: http.StatusUnsupportedMediaType, code: "MALFORMED_REQUEST", field: "content_type", reason: "application_json_required"}
	}
	mediaType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || mediaType != "application/json" {
		return nil, &requestProblem{status: http.StatusUnsupportedMediaType, code: "MALFORMED_REQUEST", field: "content_type", reason: "application_json_required"}
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, MaximumRequestBodyBytes))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, &requestProblem{status: http.StatusRequestEntityTooLarge, code: "MALFORMED_REQUEST", field: "body", reason: "too_large"}
		}
		return nil, malformed("body", "invalid_json")
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return nil, malformed("body", "object_required")
	}
	fields := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		key, ok := keyToken.(string)
		if tokenErr != nil || !ok {
			return nil, malformed("body", "invalid_json")
		}
		if !allowed[key] {
			return nil, malformed("body", "unknown_field")
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, malformed(key, "duplicate")
		}
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				return nil, &requestProblem{status: http.StatusRequestEntityTooLarge, code: "MALFORMED_REQUEST", field: "body", reason: "too_large"}
			}
			return nil, malformed(key, "invalid_json")
		}
		fields[key] = append(json.RawMessage(nil), raw...)
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, malformed("body", "invalid_json")
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return nil, malformed("body", "trailing_data")
	}
	return fields, nil
}

func requiredString(fields map[string]json.RawMessage, name string) (string, *requestProblem) {
	raw, exists := fields[name]
	if !exists {
		return "", validation(name, "required")
	}
	return stringValue(raw, name)
}

func stringValue(raw json.RawMessage, field string) (string, *requestProblem) {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", validation(field, "string_required")
	}
	return value, nil
}

func requiredInteger(fields map[string]json.RawMessage, name string, minimum, maximum int64) (int64, *requestProblem) {
	raw, exists := fields[name]
	if !exists {
		return 0, validation(name, "required")
	}
	return integerValue(raw, name, minimum, maximum)
}

func integerValue(raw json.RawMessage, field string, minimum, maximum int64) (int64, *requestProblem) {
	text := string(raw)
	if !canonicalInteger.MatchString(text) {
		return 0, validation(field, "integer_required")
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, validation(field, "out_of_range")
	}
	return value, nil
}

func parseID(raw, field string) (int64, *requestProblem) {
	if !canonicalID.MatchString(raw) {
		return 0, validation(field, "invalid")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, validation(field, "invalid")
	}
	return value, nil
}

func idempotencyKey(request *http.Request) (string, *requestProblem) {
	if request == nil {
		return "", validation("idempotency_key", "required")
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || !validIdempotencyKey(values[0]) {
		return "", validation("idempotency_key", "invalid")
	}
	return values[0], nil
}

func parsePackageQuery(raw string) (ListPackagesInput, *requestProblem) {
	input := ListPackagesInput{Limit: DefaultLimit}
	if raw == "" {
		return input, nil
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return ListPackagesInput{}, malformed("query", "invalid_encoding")
	}
	for key, entries := range values {
		if key != "group_id" && key != "limit" && key != "offset" {
			return ListPackagesInput{}, malformed("query", "unknown_parameter")
		}
		if len(entries) != 1 || entries[0] == "" {
			return ListPackagesInput{}, malformed(key, "duplicate_or_empty")
		}
	}
	if entries, exists := values["group_id"]; exists {
		value, problem := parseQueryInteger(entries[0], "group_id", 1, 1<<62)
		if problem != nil {
			return ListPackagesInput{}, problem
		}
		input.GroupID = &value
	}
	if entries, exists := values["limit"]; exists {
		value, problem := parseQueryInteger(entries[0], "limit", 1, MaximumLimit)
		if problem != nil {
			return ListPackagesInput{}, problem
		}
		input.Limit = int(value)
	}
	if entries, exists := values["offset"]; exists {
		value, problem := parseQueryInteger(entries[0], "offset", 0, MaximumOffset)
		if problem != nil {
			return ListPackagesInput{}, problem
		}
		input.Offset = int(value)
	}
	return input, nil
}

func parseQueryInteger(raw, field string, minimum, maximum int64) (int64, *requestProblem) {
	if !canonicalInteger.MatchString(raw) {
		return 0, validation(field, "integer_required")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, validation(field, "out_of_range")
	}
	return value, nil
}

func requireNoQuery(writer http.ResponseWriter, request *http.Request) bool {
	if request != nil && request.URL != nil && request.URL.RawQuery == "" {
		return true
	}
	writeProblem(writer, request, malformed("query", "not_allowed"))
	return false
}

func writeFailure(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ErrUnauthenticated):
		writeHTTPError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.", nil)
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrCSRFInvalid):
		writeHTTPError(writer, request, http.StatusForbidden, "UNAUTHORIZED", "Permission is denied.", nil)
	case errors.Is(err, ErrInvalidInput):
		writeHTTPError(writer, request, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Validation failed.", nil)
	case errors.Is(err, ErrNotFound):
		writeHTTPError(writer, request, http.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
	case errors.Is(err, ErrConflict), errors.Is(err, ErrVersionConflict), errors.Is(err, ErrIdempotencyConflict),
		errors.Is(err, ErrGroupNotEmpty), errors.Is(err, ErrArchived):
		writeHTTPError(writer, request, http.StatusConflict, "CONFLICT", "The request conflicts with the current state.", nil)
	default:
		writeHTTPError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required dependency is unavailable.", nil)
	}
}

func writeProblem(writer http.ResponseWriter, request *http.Request, problem *requestProblem) {
	if problem == nil {
		writeFailure(writer, request, ErrUnavailable)
		return
	}
	details := []fieldError{{Field: problem.field, Reason: problem.reason}}
	message := "The request is malformed."
	if problem.status == http.StatusUnprocessableEntity {
		message = "Validation failed."
	}
	writeHTTPError(writer, request, problem.status, problem.code, message, details)
}

type fieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type errorResponse struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id"`
	Details   []fieldError `json:"details,omitempty"`
}

func writeHTTPError(writer http.ResponseWriter, request *http.Request, status int, code, message string, details []fieldError) {
	if writer == nil {
		return
	}
	setSecurityHeaders(writer)
	writer.Header().Set("Content-Type", "application/json")
	requestID := requestIdentifier(request)
	writer.Header().Set("X-Request-ID", requestID)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(errorResponse{Code: code, Message: message, RequestID: requestID, Details: details})
}

func writeMethodNotAllowed(writer http.ResponseWriter, request *http.Request, allow string) {
	writer.Header().Set("Allow", allow)
	writeHTTPError(writer, request, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The method is not allowed.", nil)
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

func requestIdentifier(request *http.Request) string {
	if request != nil {
		value := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestIDPattern.MatchString(value) {
			return value
		}
	}
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("local-%x", buffer)
}
