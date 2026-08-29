package http

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const CustomerAcquisitionLinksPath = "/api/admin/wecom-customer-acquisition-links"

type customerAcquisitionLinksApplication interface {
	List(context.Context, string, int) (wecomport.CustomerAcquisitionLinkPage, error)
	Get(context.Context, string) (wecomport.CustomerAcquisitionLink, error)
	Create(context.Context, wecomapp.CustomerAcquisitionLinkCommand) (wecomapp.CustomerAcquisitionLinkReceipt, error)
	Update(context.Context, wecomapp.CustomerAcquisitionLinkCommand) (wecomapp.CustomerAcquisitionLinkReceipt, error)
	Delete(context.Context, wecomapp.CustomerAcquisitionLinkCommand) (wecomapp.CustomerAcquisitionLinkReceipt, error)
	SetEnabled(context.Context, string, bool) error
	Reconcile(context.Context, wecomapp.ReconcileCustomerAcquisitionLinkCommand) (wecomapp.CustomerAcquisitionLinkReceipt, error)
}

// CustomerAcquisitionLinkHandler is a narrow legacy transport adapter. Route
// registration supplies its capability and CSRF gate; this handler repeats the
// principal check so direct mounting fails closed too.
type CustomerAcquisitionLinkHandler struct {
	application customerAcquisitionLinksApplication
}

func NewCustomerAcquisitionLinkHandler(application customerAcquisitionLinksApplication) *CustomerAcquisitionLinkHandler {
	return &CustomerAcquisitionLinkHandler{application: application}
}

func NewDisabledCustomerAcquisitionLinkHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeCustomerAcquisitionLinkError(writer, http.StatusServiceUnavailable, "customer_acquisition_link_unavailable")
	})
}

func (handler *CustomerAcquisitionLinkHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setCustomerAcquisitionLinkHeaders(writer)
	if handler == nil || handler.application == nil || request == nil || request.URL == nil || request.URL.RawPath != "" || strings.HasSuffix(request.URL.Path, "/") || strings.Contains(request.URL.Path, "\\") {
		writeCustomerAcquisitionLinkError(writer, http.StatusServiceUnavailable, "customer_acquisition_link_unavailable")
		return
	}
	parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(parts) < 3 || "/"+strings.Join(parts[:3], "/") != CustomerAcquisitionLinksPath {
		writeCustomerAcquisitionLinkError(writer, http.StatusNotFound, "not_found")
		return
	}
	switch {
	case len(parts) == 3 && request.Method == http.MethodGet:
		handler.list(writer, request)
	case len(parts) == 3 && request.Method == http.MethodPost:
		handler.create(writer, request)
	case len(parts) == 4 && request.Method == http.MethodGet:
		handler.get(writer, request, parts[3])
	case len(parts) == 4 && request.Method == http.MethodPatch:
		handler.update(writer, request, parts[3])
	case len(parts) == 4 && request.Method == http.MethodDelete:
		handler.delete(writer, request, parts[3])
	case len(parts) == 5 && request.Method == http.MethodPost && parts[4] == "reconcile":
		handler.reconcile(writer, request, parts[3])
	case len(parts) == 5 && request.Method == http.MethodPost && (parts[4] == "enable" || parts[4] == "disable"):
		handler.setEnabled(writer, request, parts[3], parts[4] == "enable")
	default:
		writeCustomerAcquisitionLinkError(writer, http.StatusNotFound, "not_found")
	}
}

func (handler *CustomerAcquisitionLinkHandler) list(writer http.ResponseWriter, request *http.Request) {
	if !customerAcquisitionLinkAuthorized(request, authport.CapabilityChannelsRead) {
		writeCustomerAcquisitionLinkError(writer, http.StatusForbidden, "permission_denied")
		return
	}
	query := request.URL.Query()
	if len(query) > 2 || len(query["cursor"]) > 1 || len(query["limit"]) > 1 {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_query")
		return
	}
	limit := 100
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_query")
			return
		}
		limit = parsed
	}
	page, err := handler.application.List(request.Context(), query.Get("cursor"), limit)
	if err != nil {
		writeCustomerAcquisitionLinkApplicationError(writer, err)
		return
	}
	writeCustomerAcquisitionLinkJSON(writer, http.StatusOK, map[string]any{"items": customerAcquisitionLinkSummaries(page.Links), "next_cursor": page.NextCursor})
}

func (handler *CustomerAcquisitionLinkHandler) get(writer http.ResponseWriter, request *http.Request, linkID string) {
	if !customerAcquisitionLinkID(linkID) {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_link_id")
		return
	}
	if !customerAcquisitionLinkAuthorized(request, authport.CapabilityChannelsRead) {
		writeCustomerAcquisitionLinkError(writer, http.StatusForbidden, "permission_denied")
		return
	}
	link, err := handler.application.Get(request.Context(), linkID)
	if err != nil {
		writeCustomerAcquisitionLinkApplicationError(writer, err)
		return
	}
	writeCustomerAcquisitionLinkJSON(writer, http.StatusOK, customerAcquisitionLinkValue(link))
}

func (handler *CustomerAcquisitionLinkHandler) create(writer http.ResponseWriter, request *http.Request) {
	actor, key, input, ok := customerAcquisitionLinkMutation(writer, request)
	if !ok {
		return
	}
	receipt, err := handler.application.Create(request.Context(), wecomapp.CustomerAcquisitionLinkCommand{Actor: actor, IdempotencyKey: key, Input: input})
	if err != nil {
		writeCustomerAcquisitionLinkApplicationError(writer, err)
		return
	}
	writeCustomerAcquisitionLinkJSON(writer, http.StatusAccepted, customerAcquisitionLinkReceiptValue(receipt))
}

func (handler *CustomerAcquisitionLinkHandler) update(writer http.ResponseWriter, request *http.Request, linkID string) {
	if !customerAcquisitionLinkID(linkID) {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_link_id")
		return
	}
	actor, key, input, ok := customerAcquisitionLinkMutation(writer, request)
	if !ok {
		return
	}
	receipt, err := handler.application.Update(request.Context(), wecomapp.CustomerAcquisitionLinkCommand{Actor: actor, IdempotencyKey: key, LinkID: linkID, Input: input})
	if err != nil {
		writeCustomerAcquisitionLinkApplicationError(writer, err)
		return
	}
	writeCustomerAcquisitionLinkJSON(writer, http.StatusOK, customerAcquisitionLinkReceiptValue(receipt))
}

func (handler *CustomerAcquisitionLinkHandler) delete(writer http.ResponseWriter, request *http.Request, linkID string) {
	if !customerAcquisitionLinkID(linkID) || !customerAcquisitionLinkEmptyBody(request) {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	actor, key, ok := customerAcquisitionLinkActorKey(request)
	if !ok {
		writeCustomerAcquisitionLinkError(writer, http.StatusForbidden, "permission_denied")
		return
	}
	receipt, err := handler.application.Delete(request.Context(), wecomapp.CustomerAcquisitionLinkCommand{Actor: actor, IdempotencyKey: key, LinkID: linkID})
	if err != nil {
		writeCustomerAcquisitionLinkApplicationError(writer, err)
		return
	}
	writeCustomerAcquisitionLinkJSON(writer, http.StatusOK, customerAcquisitionLinkReceiptValue(receipt))
}

func (handler *CustomerAcquisitionLinkHandler) setEnabled(writer http.ResponseWriter, request *http.Request, linkID string, enabled bool) {
	if !customerAcquisitionLinkID(linkID) || !customerAcquisitionLinkEmptyBody(request) {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, _, ok := customerAcquisitionLinkActorKey(request); !ok {
		writeCustomerAcquisitionLinkError(writer, http.StatusForbidden, "permission_denied")
		return
	}
	if err := handler.application.SetEnabled(request.Context(), linkID, enabled); err != nil {
		writeCustomerAcquisitionLinkApplicationError(writer, err)
		return
	}
	writeCustomerAcquisitionLinkJSON(writer, http.StatusOK, map[string]any{"state": "unsupported"})
}

func (handler *CustomerAcquisitionLinkHandler) reconcile(writer http.ResponseWriter, request *http.Request, linkID string) {
	if !customerAcquisitionLinkID(linkID) {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_link_id")
		return
	}
	actor, key, ok := customerAcquisitionLinkActorKey(request)
	if !ok {
		writeCustomerAcquisitionLinkError(writer, http.StatusForbidden, "permission_denied")
		return
	}
	var input struct {
		ReceiptID      int64  `json:"receipt_id"`
		Resolution     string `json:"resolution"`
		EvidenceDigest string `json:"evidence_digest"`
	}
	if !customerAcquisitionLinkDecode(request, &input) || input.ReceiptID < 1 || len(input.EvidenceDigest) != 64 {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	evidence, err := hex.DecodeString(input.EvidenceDigest)
	if err != nil || len(evidence) != 32 {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	var digest [32]byte
	copy(digest[:], evidence)
	receipt, err := handler.application.Reconcile(request.Context(), wecomapp.ReconcileCustomerAcquisitionLinkCommand{ReceiptID: input.ReceiptID, Actor: actor, IdempotencyKey: key, LinkID: linkID, Resolution: wecomapp.CustomerAcquisitionLinkResolution(input.Resolution), EvidenceDigest: digest})
	if err != nil {
		writeCustomerAcquisitionLinkApplicationError(writer, err)
		return
	}
	writeCustomerAcquisitionLinkJSON(writer, http.StatusOK, customerAcquisitionLinkReceiptValue(receipt))
}

func customerAcquisitionLinkMutation(writer http.ResponseWriter, request *http.Request) (int64, string, wecomport.CustomerAcquisitionLinkInput, bool) {
	actor, key, ok := customerAcquisitionLinkActorKey(request)
	if !ok {
		writeCustomerAcquisitionLinkError(writer, http.StatusForbidden, "permission_denied")
		return 0, "", wecomport.CustomerAcquisitionLinkInput{}, false
	}
	var payload struct {
		LinkName      string   `json:"link_name"`
		UserIDs       []string `json:"user_ids"`
		DepartmentIDs []int64  `json:"department_ids"`
		SkipVerify    bool     `json:"skip_verify"`
	}
	if !customerAcquisitionLinkDecode(request, &payload) {
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_request")
		return 0, "", wecomport.CustomerAcquisitionLinkInput{}, false
	}
	return actor, key, wecomport.CustomerAcquisitionLinkInput{
		LinkName:      payload.LinkName,
		UserIDs:       payload.UserIDs,
		DepartmentIDs: payload.DepartmentIDs,
		SkipVerify:    payload.SkipVerify,
	}, true
}

func customerAcquisitionLinkActorKey(request *http.Request) (int64, string, bool) {
	if !customerAcquisitionLinkAuthorized(request, authport.CapabilityChannelsWrite) {
		return 0, "", false
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || strings.TrimSpace(keys[0]) != keys[0] || len(keys[0]) < 16 || len(keys[0]) > 128 {
		return 0, "", false
	}
	principal, _ := authport.PrincipalFromContext(request.Context())
	return principal.AdminUserID, keys[0], true
}

func customerAcquisitionLinkAuthorized(request *http.Request, capability authport.Capability) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && (principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps) && authorizationOK && authorization.Capability == capability && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func customerAcquisitionLinkDecode(request *http.Request, target any) bool {
	if request == nil || request.Body == nil {
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

func customerAcquisitionLinkEmptyBody(request *http.Request) bool {
	if request == nil || request.Body == nil {
		return true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, request.Body, 1024))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err == io.EOF {
		return true
	} else if err != nil || len(value) != 0 {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

func customerAcquisitionLinkID(value string) bool {
	return value != "" && len(value) <= 1024 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "/\\\x00\n\r")
}

func customerAcquisitionLinkSummaries(values []wecomport.CustomerAcquisitionLink) []map[string]any {
	result := make([]map[string]any, len(values))
	for index, value := range values {
		result[index] = map[string]any{"link_id": value.LinkID}
	}
	return result
}

func customerAcquisitionLinkValue(value wecomport.CustomerAcquisitionLink) map[string]any {
	return map[string]any{"link_id": value.LinkID, "link_name": value.LinkName, "url": value.URL, "user_ids": value.UserIDs, "department_ids": value.DepartmentIDs, "skip_verify": value.SkipVerify}
}

func customerAcquisitionLinkReceiptValue(value wecomapp.CustomerAcquisitionLinkReceipt) map[string]any {
	response := map[string]any{"receipt_id": value.ID, "state": value.State, "business_endpoint_dispatched": value.BusinessEndpointDispatched, "real_external_call_executed": value.RealExternalCallExecuted}
	if value.OutcomeDigest != ([32]byte{}) {
		response["outcome_digest"] = hex.EncodeToString(value.OutcomeDigest[:])
	}
	if value.Resolution != "" {
		response["resolution"] = value.Resolution
	}
	if value.Link != nil {
		response["link"] = customerAcquisitionLinkValue(*value.Link)
	}
	return response
}

func setCustomerAcquisitionLinkHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeCustomerAcquisitionLinkJSON(writer http.ResponseWriter, status int, value any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeCustomerAcquisitionLinkError(writer http.ResponseWriter, status int, code string) {
	setCustomerAcquisitionLinkHeaders(writer)
	writeCustomerAcquisitionLinkJSON(writer, status, map[string]any{"ok": false, "error": map[string]string{"code": code}})
}

func writeCustomerAcquisitionLinkApplicationError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, wecomapp.ErrCustomerAcquisitionLinkConflict), errors.Is(err, wecomapp.ErrCustomerAcquisitionLinkReconcile):
		writeCustomerAcquisitionLinkError(writer, http.StatusConflict, "customer_acquisition_link_conflict")
	case errors.Is(err, wecomapp.ErrCustomerAcquisitionLinkUnsupported):
		writeCustomerAcquisitionLinkError(writer, http.StatusUnprocessableEntity, "customer_acquisition_link_unsupported")
	case errors.Is(err, wecomapp.ErrInvalidCustomerAcquisitionLinkCommand):
		writeCustomerAcquisitionLinkError(writer, http.StatusBadRequest, "invalid_request")
	default:
		writeCustomerAcquisitionLinkError(writer, http.StatusServiceUnavailable, "customer_acquisition_link_unavailable")
	}
}
