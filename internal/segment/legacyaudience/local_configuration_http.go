package legacyaudience

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LocalConfigurationHandler struct {
	application LocalConfigurationApplication
	security    Security
}

func NewLocalConfigurationHandler(application LocalConfigurationApplication, security Security) (*LocalConfigurationHandler, error) {
	if nilInterface(application) || nilInterface(security) {
		return nil, ErrUnavailable
	}
	return &LocalConfigurationHandler{application: application, security: security}, nil
}

type localConfigurationRouteFragment struct {
	handler *LocalConfigurationHandler
}

func NewLocalConfigurationRouteFragment(handler *LocalConfigurationHandler) (http.Handler, error) {
	if handler == nil || nilInterface(handler.application) || nilInterface(handler.security) {
		return nil, ErrUnavailable
	}
	return &localConfigurationRouteFragment{handler: handler}, nil
}

func (fragment *localConfigurationRouteFragment) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
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
	if path == OperationMembersRoute {
		fragment.handler.operationMembers(writer, request)
		return
	}
	if path == OperationMembersSyncRoute {
		fragment.handler.syncOperationMembers(writer, request)
		return
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) != 3 || segments[0] != "packages" || (segments[2] != "automation-binding" && segments[2] != "senders" &&
		segments[2] != "configuration" && segments[2] != "configuration-preview" && segments[2] != "configuration-materialize") {
		writeHTTPError(writer, request, http.StatusNotFound, "NOT_FOUND", "The resource was not found.", nil)
		return
	}
	if segments[2] == "automation-binding" {
		fragment.handler.automationBinding(writer, request, segments[1])
		return
	}
	switch segments[2] {
	case "senders":
		fragment.handler.senders(writer, request, segments[1])
	case "configuration":
		fragment.handler.configuration(writer, request, segments[1])
	case "configuration-preview":
		fragment.handler.previewConfiguration(writer, request, segments[1])
	case "configuration-materialize":
		fragment.handler.materializeConfiguration(writer, request, segments[1])
	}
}

func (handler *LocalConfigurationHandler) operationMembers(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, request, http.MethodGet)
		return
	}
	if !handler.authorize(writer, request, false, nil) {
		return
	}
	scope, pageSize, problem := parseOperationMembersQuery(request.URL.RawQuery)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	var response any
	var err error
	if scope == OperationMemberScope {
		response, err = handler.application.ListOperationMembers(request.Context(), pageSize)
	} else if extension, ok := handler.application.(GroupOpsOperationMemberApplication); ok {
		response, err = extension.ListGroupOpsOperationMembers(request.Context(), pageSize)
	} else {
		writeProblem(writer, request, validation("scope", "unsupported"))
		return
	}
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *LocalConfigurationHandler) syncOperationMembers(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeMethodNotAllowed(writer, request, http.MethodPost)
		return
	}
	actor, err := handler.security.Authorize(request, AccessRequirement{Capability: CapabilityOperationsManage, RequireCSRF: true})
	if err != nil || actor.AdminUserID < 1 {
		if err == nil {
			err = ErrForbidden
		}
		writeFailure(writer, request, err)
		return
	}
	key, problem := idempotencyKey(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	var body struct {
		Scope    string `json:"scope"`
		PageSize int    `json:"page_size"`
	}
	fields, problem := decodeObject(writer, request, map[string]bool{"scope": true, "page_size": true})
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if raw, ok := fields["scope"]; !ok || json.Unmarshal(raw, &body.Scope) != nil ||
		(body.Scope != OperationMemberScope && body.Scope != GroupOpsOperationMemberScope) {
		writeProblem(writer, request, validation("scope", "unsupported"))
		return
	}
	body.PageSize = MaximumOperationMemberPageSize
	if raw, ok := fields["page_size"]; ok {
		var value int64
		if json.Unmarshal(raw, &value) != nil || value < 1 || value > MaximumOperationMemberPageSize {
			writeProblem(writer, request, validation("page_size", "invalid"))
			return
		}
		body.PageSize = int(value)
	}
	var response any
	if body.Scope == OperationMemberScope {
		response, err = handler.application.SyncOperationMembers(request.Context(), OperationMemberSyncInput{
			Actor: actor, IdempotencyKey: key, PageSize: body.PageSize,
		})
	} else {
		extension, ok := handler.application.(GroupOpsOperationMemberApplication)
		if !ok {
			writeFailure(writer, request, ErrUnavailable)
			return
		}
		response, err = extension.RefreshGroupOpsOperationMembers(request.Context(), actor.AdminUserID, key, body.PageSize)
	}
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *LocalConfigurationHandler) automationBinding(writer http.ResponseWriter, request *http.Request, rawID string) {
	packageID, problem := parseID(rawID, "package_id")
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	switch request.Method {
	case http.MethodGet:
		if !requireNoQuery(writer, request) || !handler.authorize(writer, request, false, nil) {
			return
		}
		response, err := handler.application.GetAutomationBinding(request.Context(), packageID)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	case http.MethodPut:
		if !requireNoQuery(writer, request) {
			return
		}
		var actor Actor
		if !handler.authorize(writer, request, true, &actor) {
			return
		}
		key, keyProblem := idempotencyKey(request)
		if keyProblem != nil {
			writeProblem(writer, request, keyProblem)
			return
		}
		input, decodeProblem := decodePutAutomationBinding(writer, request)
		if decodeProblem != nil {
			writeProblem(writer, request, decodeProblem)
			return
		}
		input.PackageID, input.Actor, input.IdempotencyKey = packageID, actor, key
		response, err := handler.application.PutAutomationBinding(request.Context(), input)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	case http.MethodDelete:
		if !requireNoQuery(writer, request) {
			return
		}
		var actor Actor
		if !handler.authorize(writer, request, true, &actor) {
			return
		}
		key, keyProblem := idempotencyKey(request)
		if keyProblem != nil {
			writeProblem(writer, request, keyProblem)
			return
		}
		input, decodeProblem := decodeDeleteAutomationBinding(writer, request)
		if decodeProblem != nil {
			writeProblem(writer, request, decodeProblem)
			return
		}
		response, err := handler.application.DeleteAutomationBinding(request.Context(), DeleteAutomationBindingInput{
			PackageID: packageID, ExpectedVersion: input.ExpectedVersion, Actor: actor, IdempotencyKey: key,
		})
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	default:
		writeMethodNotAllowed(writer, request, http.MethodGet+", "+http.MethodPut+", "+http.MethodDelete)
	}
}

func (handler *LocalConfigurationHandler) configuration(writer http.ResponseWriter, request *http.Request, rawID string) {
	packageID, problem := parseID(rawID, "package_id")
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	switch request.Method {
	case http.MethodGet:
		if !requireNoQuery(writer, request) || !handler.authorize(writer, request, false, nil) {
			return
		}
		response, err := handler.application.GetConfiguration(request.Context(), packageID)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	case http.MethodPut:
		if !requireNoQuery(writer, request) {
			return
		}
		var actor Actor
		if !handler.authorize(writer, request, true, &actor) {
			return
		}
		key, keyProblem := idempotencyKey(request)
		if keyProblem != nil {
			writeProblem(writer, request, keyProblem)
			return
		}
		input, decodeProblem := decodePutConfiguration(writer, request)
		if decodeProblem != nil {
			writeProblem(writer, request, decodeProblem)
			return
		}
		input.PackageID, input.Actor, input.IdempotencyKey = packageID, actor, key
		response, err := handler.application.PutConfiguration(request.Context(), input)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	default:
		writeMethodNotAllowed(writer, request, http.MethodGet+", "+http.MethodPut)
	}
}

func (handler *LocalConfigurationHandler) previewConfiguration(writer http.ResponseWriter, request *http.Request, rawID string) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, request, http.MethodGet)
		return
	}
	if !handler.authorize(writer, request, false, nil) {
		return
	}
	packageID, problem := parseID(rawID, "package_id")
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	input, queryProblem := parsePreviewConfigurationQuery(request.URL.RawQuery)
	if queryProblem != nil {
		writeProblem(writer, request, queryProblem)
		return
	}
	input.PackageID = packageID
	response, err := handler.application.PreviewConfiguration(request.Context(), input)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *LocalConfigurationHandler) materializeConfiguration(writer http.ResponseWriter, request *http.Request, rawID string) {
	if request.Method != http.MethodPost {
		writeMethodNotAllowed(writer, request, http.MethodPost)
		return
	}
	if !requireNoQuery(writer, request) {
		return
	}
	packageID, problem := parseID(rawID, "package_id")
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	var actor Actor
	if !handler.authorize(writer, request, true, &actor) {
		return
	}
	key, keyProblem := idempotencyKey(request)
	if keyProblem != nil {
		writeProblem(writer, request, keyProblem)
		return
	}
	input, decodeProblem := decodeMaterializeConfiguration(writer, request)
	if decodeProblem != nil {
		writeProblem(writer, request, decodeProblem)
		return
	}
	input.PackageID, input.Actor, input.IdempotencyKey = packageID, actor, key
	response, err := handler.application.MaterializeConfiguration(request.Context(), input)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *LocalConfigurationHandler) senders(writer http.ResponseWriter, request *http.Request, rawID string) {
	packageID, problem := parseID(rawID, "package_id")
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	switch request.Method {
	case http.MethodGet:
		if !requireNoQuery(writer, request) || !handler.authorize(writer, request, false, nil) {
			return
		}
		response, err := handler.application.GetSenders(request.Context(), packageID)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	case http.MethodPut:
		if !requireNoQuery(writer, request) {
			return
		}
		var actor Actor
		if !handler.authorize(writer, request, true, &actor) {
			return
		}
		key, keyProblem := idempotencyKey(request)
		if keyProblem != nil {
			writeProblem(writer, request, keyProblem)
			return
		}
		input, decodeProblem := decodeReplaceSenders(writer, request)
		if decodeProblem != nil {
			writeProblem(writer, request, decodeProblem)
			return
		}
		input.PackageID, input.Actor, input.IdempotencyKey = packageID, actor, key
		response, err := handler.application.ReplaceSenders(request.Context(), input)
		if err != nil {
			writeFailure(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, response)
	default:
		writeMethodNotAllowed(writer, request, http.MethodGet+", "+http.MethodPut)
	}
}

func (handler *LocalConfigurationHandler) authorize(writer http.ResponseWriter, request *http.Request, write bool, actor *Actor) bool {
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

func parseOperationMembersQuery(raw string) (string, int, *requestProblem) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "", 0, malformed("query", "invalid_encoding")
	}
	for key, entries := range values {
		if key != "scope" && key != "page_size" {
			return "", 0, malformed("query", "unknown_parameter")
		}
		if len(entries) != 1 || entries[0] == "" {
			return "", 0, malformed(key, "duplicate_or_empty")
		}
	}
	scope, exists := values["scope"]
	if !exists {
		return "", 0, validation("scope", "required")
	}
	if scope[0] != OperationMemberScope && scope[0] != GroupOpsOperationMemberScope {
		return "", 0, validation("scope", "unsupported")
	}
	pageSize := MaximumOperationMemberPageSize
	if pageSizeValues, pageSizeExists := values["page_size"]; pageSizeExists {
		value, problem := parseQueryInteger(pageSizeValues[0], "page_size", 1, MaximumOperationMemberPageSize)
		if problem != nil {
			return "", 0, problem
		}
		pageSize = int(value)
	}
	return scope[0], pageSize, nil
}

func decodePutAutomationBinding(writer http.ResponseWriter, request *http.Request) (PutAutomationBindingInput, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"automation_agent_id": true, "expected_version": true})
	if problem != nil {
		return PutAutomationBindingInput{}, problem
	}
	agentID, problem := requiredInteger(fields, "automation_agent_id", 1, 1<<62)
	if problem != nil {
		return PutAutomationBindingInput{}, problem
	}
	expected, problem := requiredInteger(fields, "expected_version", 0, 1<<62)
	if problem != nil {
		return PutAutomationBindingInput{}, problem
	}
	return PutAutomationBindingInput{AutomationAgentID: agentID, ExpectedVersion: expected}, nil
}

func decodeDeleteAutomationBinding(writer http.ResponseWriter, request *http.Request) (DeleteAutomationBindingInput, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"expected_version": true})
	if problem != nil {
		return DeleteAutomationBindingInput{}, problem
	}
	expected, problem := requiredInteger(fields, "expected_version", 0, 1<<62)
	if problem != nil {
		return DeleteAutomationBindingInput{}, problem
	}
	return DeleteAutomationBindingInput{ExpectedVersion: expected}, nil
}

func decodePutConfiguration(writer http.ResponseWriter, request *http.Request) (PutConfigurationInput, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"expected_version": true, "expected_package_version": true})
	if problem != nil {
		return PutConfigurationInput{}, problem
	}
	expected, problem := requiredInteger(fields, "expected_version", 0, 1<<62)
	if problem != nil {
		return PutConfigurationInput{}, problem
	}
	packageVersion, problem := requiredInteger(fields, "expected_package_version", 1, 1<<62)
	if problem != nil {
		return PutConfigurationInput{}, problem
	}
	return PutConfigurationInput{ExpectedVersion: expected, ExpectedPackageVersion: packageVersion}, nil
}

func decodeMaterializeConfiguration(writer http.ResponseWriter, request *http.Request) (MaterializeConfigurationInput, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"configuration_version": true, "expected_package_version": true})
	if problem != nil {
		return MaterializeConfigurationInput{}, problem
	}
	version, problem := requiredInteger(fields, "configuration_version", 1, 1<<62)
	if problem != nil {
		return MaterializeConfigurationInput{}, problem
	}
	packageVersion, problem := requiredInteger(fields, "expected_package_version", 1, 1<<62)
	if problem != nil {
		return MaterializeConfigurationInput{}, problem
	}
	return MaterializeConfigurationInput{ConfigurationVersion: version, ExpectedPackageVersion: packageVersion}, nil
}

func parsePreviewConfigurationQuery(raw string) (PreviewConfigurationInput, *requestProblem) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return PreviewConfigurationInput{}, malformed("query", "invalid_encoding")
	}
	for key, entries := range values {
		if key != "configuration_version" && key != "evaluated_at" {
			return PreviewConfigurationInput{}, malformed("query", "unknown_parameter")
		}
		if len(entries) != 1 || entries[0] == "" {
			return PreviewConfigurationInput{}, malformed(key, "duplicate_or_empty")
		}
	}
	rawVersion, exists := values["configuration_version"]
	if !exists {
		return PreviewConfigurationInput{}, validation("configuration_version", "required")
	}
	version, problem := parseQueryInteger(rawVersion[0], "configuration_version", 1, 1<<62)
	if problem != nil {
		return PreviewConfigurationInput{}, problem
	}
	input := PreviewConfigurationInput{ConfigurationVersion: version}
	if rawTime, exists := values["evaluated_at"]; exists {
		parsed, parseErr := time.Parse(time.RFC3339, rawTime[0])
		if parseErr != nil || parsed.Location() != time.UTC {
			return PreviewConfigurationInput{}, validation("evaluated_at", "utc_rfc3339_required")
		}
		input.EvaluatedAt = parsed
	}
	return input, nil
}

func decodeReplaceSenders(writer http.ResponseWriter, request *http.Request) (ReplaceSendersInput, *requestProblem) {
	fields, problem := decodeObject(writer, request, map[string]bool{"items": true})
	if problem != nil {
		return ReplaceSendersInput{}, problem
	}
	rawItems, exists := fields["items"]
	if !exists {
		return ReplaceSendersInput{}, validation("items", "required")
	}
	var itemsRaw []json.RawMessage
	if err := json.Unmarshal(rawItems, &itemsRaw); err != nil || itemsRaw == nil {
		return ReplaceSendersInput{}, validation("items", "array_required")
	}
	if len(itemsRaw) > MaximumSenderCount {
		return ReplaceSendersInput{}, validation("items", "too_many")
	}
	items := make([]PackageSender, 0, len(itemsRaw))
	for index, raw := range itemsRaw {
		item, itemProblem := decodeSenderItem(raw, index)
		if itemProblem != nil {
			return ReplaceSendersInput{}, itemProblem
		}
		items = append(items, item)
	}
	if err := validatePackageSenders(items); err != nil {
		return ReplaceSendersInput{}, validation("items", "invalid")
	}
	return ReplaceSendersInput{Items: items}, nil
}

func decodeSenderItem(raw json.RawMessage, index int) (PackageSender, *requestProblem) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil {
		return PackageSender{}, validation("items", "object_required")
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return PackageSender{}, validation("items", "object_required")
	}
	fields := make(map[string]json.RawMessage, 3)
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, ok := token.(string)
		if tokenErr != nil || !ok {
			return PackageSender{}, validation("items", "invalid")
		}
		if key != "sender_userid" && key != "sort_order" && key != "is_enabled" {
			return PackageSender{}, malformed("items", "unknown_field")
		}
		if _, duplicate := fields[key]; duplicate {
			return PackageSender{}, malformed("items", "duplicate")
		}
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return PackageSender{}, validation("items", "invalid")
		}
		fields[key] = append(json.RawMessage(nil), value...)
	}
	if closing, closeErr := decoder.Token(); closeErr != nil || closing != json.Delim('}') {
		return PackageSender{}, validation("items", "invalid")
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return PackageSender{}, validation("items", "invalid")
	}
	sender, problem := requiredString(fields, "sender_userid")
	if problem != nil {
		return PackageSender{}, problem
	}
	order, problem := requiredInteger(fields, "sort_order", 1, MaximumSenderCount)
	if problem != nil {
		return PackageSender{}, problem
	}
	rawEnabled, exists := fields["is_enabled"]
	if !exists {
		return PackageSender{}, validation("is_enabled", "required")
	}
	var enabled bool
	if json.Unmarshal(rawEnabled, &enabled) != nil || (string(rawEnabled) != "true" && string(rawEnabled) != "false") {
		return PackageSender{}, validation("is_enabled", "boolean_required")
	}
	if int(order) != index+1 {
		return PackageSender{}, validation("sort_order", "sequence_required")
	}
	return PackageSender{SenderUserID: sender, SortOrder: int32(order), IsEnabled: enabled}, nil
}
