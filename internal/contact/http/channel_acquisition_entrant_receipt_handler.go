package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type channelAcquisitionEntrantReceiptService interface {
	List(context.Context, contactapp.ChannelAcquisitionEntrantReceiptListInput) (contactapp.ChannelAcquisitionEntrantReceiptPage, error)
	Get(context.Context, int64, int64, int64) (contactapp.ChannelAcquisitionEntrantReceiptItem, error)
	Reconcile(context.Context, contactapp.ReconcileChannelAcquisitionEntrantReceiptCommand) (contactapp.ChannelAcquisitionEntrantReceiptItem, error)
	ListUnassigned(context.Context, contactapp.UnassignedChannelAcquisitionEntrantReceiptListInput) (contactapp.ChannelAcquisitionEntrantReceiptPage, error)
	GetUnassigned(context.Context, int64, int64) (contactapp.ChannelAcquisitionEntrantReceiptItem, error)
	ReconcileUnassigned(context.Context, contactapp.ReconcileChannelAcquisitionEntrantReceiptCommand) (contactapp.ChannelAcquisitionEntrantReceiptItem, error)
}

type ChannelAcquisitionEntrantReceiptHandler struct {
	service channelAcquisitionEntrantReceiptService
	csrf    channelAcquisitionCSRFValidator
}

func NewChannelAcquisitionEntrantReceiptHandler(service channelAcquisitionEntrantReceiptService, csrf channelAcquisitionCSRFValidator) (*ChannelAcquisitionEntrantReceiptHandler, error) {
	if channelAcquisitionNil(service) || channelAcquisitionNil(csrf) {
		return nil, contactapp.ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	return &ChannelAcquisitionEntrantReceiptHandler{service: service, csrf: csrf}, nil
}

func NewChannelAcquisitionEntrantReceiptRouteFragment(handler *ChannelAcquisitionEntrantReceiptHandler) (http.Handler, error) {
	if handler == nil {
		return nil, contactapp.ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	return http.HandlerFunc(handler.route), nil
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) route(writer http.ResponseWriter, request *http.Request) {
	channelAcquisitionSecurityHeaders(writer)
	if request == nil || request.URL == nil || request.URL.RawPath != "" || strings.HasSuffix(request.URL.Path, "/") || strings.Contains(request.URL.Path, "\\") {
		handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
		return
	}
	parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "admin" && parts[2] == "channel-acquisition-entrant-receipts" && parts[3] == "unassigned" {
		switch {
		case len(parts) == 4 && request.Method == http.MethodGet:
			handler.listUnassigned(writer, request)
		case len(parts) == 5 && request.Method == http.MethodGet:
			handler.getUnassigned(writer, request, parts[4])
		case len(parts) == 6 && parts[5] == "reconcile" && request.Method == http.MethodPost:
			handler.reconcileUnassigned(writer, request, parts[4])
		default:
			handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
		}
		return
	}
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "admin" || parts[2] != "channels" || parts[4] != "acquisition-entrant-receipts" {
		handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
		return
	}
	switch {
	case len(parts) == 5 && request.Method == http.MethodGet:
		handler.list(writer, request, parts[3])
	case len(parts) == 6 && request.Method == http.MethodGet:
		handler.get(writer, request, parts[3], parts[5])
	case len(parts) == 7 && parts[6] == "reconcile" && request.Method == http.MethodPost:
		handler.reconcile(writer, request, parts[3], parts[5])
	default:
		handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
	}
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) listUnassigned(writer http.ResponseWriter, request *http.Request) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsRead, false)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	limit, cursor, err := entrantReceiptListQuery(request.URL)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "query", err)
		return
	}
	page, err := handler.service.ListUnassigned(request.Context(), contactapp.UnassignedChannelAcquisitionEntrantReceiptListInput{ActorID: actor, Limit: limit, Cursor: cursor})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, page)
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) getUnassigned(writer http.ResponseWriter, request *http.Request, rawReceiptID string) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsRead, false)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	receiptID, err := channelAcquisitionID(rawReceiptID)
	if err != nil {
		handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
		return
	}
	item, err := handler.service.GetUnassigned(request.Context(), actor, receiptID)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, item)
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) list(writer http.ResponseWriter, request *http.Request, rawChannelID string) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsRead, false)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelID, err := channelAcquisitionID(rawChannelID)
	if err != nil {
		handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
		return
	}
	limit, cursor, err := entrantReceiptListQuery(request.URL)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "query", err)
		return
	}
	page, err := handler.service.List(request.Context(), contactapp.ChannelAcquisitionEntrantReceiptListInput{ActorID: actor, ChannelID: channelID, Limit: limit, Cursor: cursor})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, page)
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) get(writer http.ResponseWriter, request *http.Request, rawChannelID, rawReceiptID string) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsRead, false)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelID, err := channelAcquisitionID(rawChannelID)
	receiptID, receiptErr := channelAcquisitionID(rawReceiptID)
	if err != nil || receiptErr != nil {
		handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
		return
	}
	item, err := handler.service.Get(request.Context(), actor, channelID, receiptID)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, item)
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) reconcile(writer http.ResponseWriter, request *http.Request, rawChannelID, rawReceiptID string) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsWrite, true)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelID, err := channelAcquisitionID(rawChannelID)
	receiptID, receiptErr := channelAcquisitionID(rawReceiptID)
	if err != nil || receiptErr != nil {
		handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
		return
	}
	key, err := channelAcquisitionIdempotencyKey(request)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "Idempotency-Key", err)
		return
	}
	effectID, customerID, reason, err := entrantReceiptReconcileBody(writer, request)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "body", contactapp.ErrInvalidChannelAcquisitionEntrantReceipt)
		return
	}
	item, err := handler.service.Reconcile(request.Context(), contactapp.ReconcileChannelAcquisitionEntrantReceiptCommand{ActorID: actor, ChannelID: channelID, ReceiptID: receiptID, EffectID: effectID, CustomerID: customerID, Reason: reason, IdempotencyKey: key})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, item)
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) reconcileUnassigned(writer http.ResponseWriter, request *http.Request, rawReceiptID string) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsWrite, true)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	receiptID, err := channelAcquisitionID(rawReceiptID)
	if err != nil {
		handler.writeError(writer, request, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound)
		return
	}
	key, err := channelAcquisitionIdempotencyKey(request)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "Idempotency-Key", err)
		return
	}
	effectID, customerID, reason, err := entrantReceiptReconcileBody(writer, request)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "body", contactapp.ErrInvalidChannelAcquisitionEntrantReceipt)
		return
	}
	item, err := handler.service.ReconcileUnassigned(request.Context(), contactapp.ReconcileChannelAcquisitionEntrantReceiptCommand{ActorID: actor, ReceiptID: receiptID, EffectID: effectID, CustomerID: customerID, Reason: reason, IdempotencyKey: key})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, item)
}

func entrantReceiptReconcileBody(writer http.ResponseWriter, request *http.Request) (string, int64, string, error) {
	object, err := channelAcquisitionDecodeObject(writer, request)
	if err != nil || len(object) != 3 || object["effect_id"] == nil || object["customer_id"] == nil || object["reason"] == nil {
		return "", 0, "", contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
	}
	var effectID, reason string
	var customerID int64
	if json.Unmarshal(object["effect_id"], &effectID) != nil || json.Unmarshal(object["customer_id"], &customerID) != nil || json.Unmarshal(object["reason"], &reason) != nil {
		return "", 0, "", contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
	}
	return effectID, customerID, reason, nil
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) authorize(request *http.Request, capability authport.Capability, csrf bool) (int64, error) {
	if handler == nil || request == nil {
		return 0, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelAcquisitionEntrantReceiptUnavailable)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	if !csrf {
		return principal.AdminUserID, nil
	}
	session, ok := authport.SessionFromContext(request.Context())
	values := request.Header.Values("X-CSRF-Token")
	if !ok {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if len(values) != 1 || !channelAcquisitionValidCSRF(values[0]) {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrCSRFInvalid)
	}
	if err := handler.csrf.ValidateCSRF(request.Context(), session, authport.CSRFToken(values[0])); err != nil {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, err)
	}
	return principal.AdminUserID, nil
}

func entrantReceiptListQuery(target *url.URL) (int, string, error) {
	if target == nil || len(target.RawQuery) > 1024 {
		return 0, "", contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
	}
	values, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return 0, "", err
	}
	limit := contactapp.ChannelAcquisitionEntrantReceiptDefaultLimit
	cursor := ""
	for key, entries := range values {
		if len(entries) != 1 || entries[0] == "" {
			return 0, "", contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
		}
		switch key {
		case "limit":
			limit, err = strconv.Atoi(entries[0])
		case "cursor":
			cursor = entries[0]
		default:
			return 0, "", contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
		}
		if err != nil {
			return 0, "", contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
		}
	}
	return limit, cursor, nil
}

func (handler *ChannelAcquisitionEntrantReceiptHandler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		channelAcquisitionWriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrInvalidChannelAcquisitionEntrantReceipt):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrChannelAcquisitionEntrantReceiptConflict):
		code = platformhttp.CodeConflict
	}
	channelAcquisitionWriteError(writer, request, platformhttp.NewError(code, err))
}
