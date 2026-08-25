package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360app "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/app"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

// legacyExternalIdentityResolvePurpose is frozen by the legacy API-client
// contract. Route composition supplies the production JWT authenticator.
const legacyExternalIdentityResolvePurpose = "identity"

var (
	errLegacyExternalCustomerReadInvalid     = errors.New("invalid legacy external customer read request")
	errLegacyExternalCustomerReadUnavailable = errors.New("legacy external customer read unavailable")
	errLegacyExternalCustomerReadNotFound    = errors.New("legacy external customer read not found")
	errLegacyExternalCustomerReadConflict    = errors.New("legacy external customer read identity conflict")
)

// legacyExternalCustomerEventReader is intentionally local to this adapter.
// It reuses the Contact event service and does not introduce a second read
// model or an external-identity field in Contact.
type legacyExternalCustomerEventReader interface {
	List(context.Context, contactapp.CustomerEventInput) (contactapp.CustomerEventResult, error)
}

// legacyExternalVerifiedUnionIDResolver resolves only verified unionid
// references. The legacy adapter cannot derive verification from an unscoped
// value, so route composition must not inject a broader resolver here.
type legacyExternalVerifiedUnionIDResolver interface {
	ResolveUnionID(context.Context, string) (identityport.ResolveResult, error)
}

// legacyExternalCustomerReadHandler is a narrow legacy transport adapter.
// Human-session routing and service/JWT routing are deliberately composed by
// the root. The identity endpoint additionally verifies the injected service
// principal itself so it cannot fall back to a browser session.
type legacyExternalCustomerReadHandler struct {
	customers customerListApplication
	detail    customerDetailApplication
	events    legacyExternalCustomerEventReader
	chats     customer360port.CustomerChatActivityReader
	identity  identityResolveApplication
	unionIDs  legacyExternalVerifiedUnionIDResolver
	corpID    string
	service   operationServiceAuthenticator
}

func newLegacyExternalCustomerReadHandler(
	customers customerListApplication,
	detail customerDetailApplication,
	events legacyExternalCustomerEventReader,
	chats customer360port.CustomerChatActivityReader,
	identity identityResolveApplication,
	unionIDs legacyExternalVerifiedUnionIDResolver,
	corpID string,
	service operationServiceAuthenticator,
) (*legacyExternalCustomerReadHandler, error) {
	if nilLegacyDependency(customers) || nilLegacyDependency(detail) || nilLegacyDependency(events) ||
		nilLegacyDependency(chats) || nilLegacyDependency(identity) || nilLegacyDependency(unionIDs) {
		return nil, errLegacyExternalCustomerReadUnavailable
	}
	return &legacyExternalCustomerReadHandler{
		customers: customers, detail: detail, events: events, chats: chats, identity: identity,
		unionIDs: unionIDs, corpID: strings.TrimSpace(corpID), service: service,
	}, nil
}

func (handler *Handler) ListExternalCustomers(writer http.ResponseWriter, request *http.Request) {
	if handler == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	if handler.externalCustomerRead == nil {
		handler.ListCustomers(writer, request)
		return
	}
	handler.externalCustomerRead.ListCustomers(writer, request)
}

func (handler *Handler) GetExternalCustomer(writer http.ResponseWriter, request *http.Request) {
	if handler == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	if handler.externalCustomerRead == nil {
		handler.GetCustomer(writer, request)
		return
	}
	handler.externalCustomerRead.GetCustomer(writer, request, chi.URLParam(request, "external_userid"))
}

func (handler *Handler) GetExternalCustomerTimeline(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.externalCustomerRead == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	handler.externalCustomerRead.GetCustomerTimeline(writer, request, chi.URLParam(request, "external_userid"))
}

func (handler *Handler) GetExternalUser(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.externalCustomerRead == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	handler.externalCustomerRead.GetUser(writer, request, chi.URLParam(request, "unionid"))
}

func (handler *Handler) GetExternalUserTimeline(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.externalCustomerRead == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	handler.externalCustomerRead.GetUserTimeline(writer, request, chi.URLParam(request, "unionid"))
}

func (handler *Handler) GetExternalRecentMessages(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.externalCustomerRead == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	handler.externalCustomerRead.GetRecentMessages(writer, request, chi.URLParam(request, "external_userid"))
}

func (handler *Handler) GetExternalUserRecentMessages(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.externalCustomerRead == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	handler.externalCustomerRead.GetUserRecentMessages(writer, request, chi.URLParam(request, "unionid"))
}

func (handler *Handler) ResolveExternalIdentity(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.externalCustomerRead == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	handler.externalCustomerRead.ResolveIdentity(writer, request)
}

func (handler *legacyExternalCustomerReadHandler) ListCustomers(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || nilLegacyDependency(handler.customers) {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	owner, query, err := handler.listInput(request)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	result, err := handler.customers.List(request.Context(), contactapp.CustomerListInput{
		Keyword: query.keyword, OwnerStaffID: cloneLegacyExternalInt64(owner), Limit: query.limit,
	})
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	items, err := legacyExternalCustomerListItems(result.Items)
	if err != nil || result.Total < int64(len(items)) || result.TotalIsEstimate || result.Watermark.IsZero() {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	writeLegacyExternalCustomerReadJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "customers": items, "items": items, "count": len(items), "total": result.Total,
		"limit": resultLimit(query.limit, contactapp.CustomerListDefaultLimit), "offset": 0,
		"next_cursor": cloneLegacyExternalString(result.NextCursor), "source_status": "canonical_contact", "route_owner": "ai_crm_next",
		"fallback_used": false, "real_external_call_executed": false,
	})
}

func (handler *legacyExternalCustomerReadHandler) GetCustomer(writer http.ResponseWriter, request *http.Request, externalUserID string) {
	customer, _, err := handler.customerByExternalUserID(request, externalUserID)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	writeLegacyExternalCustomerReadJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "customer": customer, "source_status": "canonical_contact", "route_owner": "ai_crm_next",
		"fallback_used": false, "real_external_call_executed": false,
	})
}

func (handler *legacyExternalCustomerReadHandler) GetUser(writer http.ResponseWriter, request *http.Request, unionID string) {
	customer, _, err := handler.customerByUnionID(request, unionID)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	writeLegacyExternalCustomerReadJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "customer": customer, "source_status": "canonical_contact", "route_owner": "ai_crm_next",
		"fallback_used": false, "real_external_call_executed": false,
	})
}

func (handler *legacyExternalCustomerReadHandler) GetCustomerTimeline(writer http.ResponseWriter, request *http.Request, externalUserID string) {
	_, customerID, err := handler.customerByExternalUserID(request, externalUserID)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	handler.timeline(writer, request, customerID)
}

func (handler *legacyExternalCustomerReadHandler) GetUserTimeline(writer http.ResponseWriter, request *http.Request, unionID string) {
	_, customerID, err := handler.customerByUnionID(request, unionID)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	handler.timeline(writer, request, customerID)
}

func (handler *legacyExternalCustomerReadHandler) GetRecentMessages(writer http.ResponseWriter, request *http.Request, externalUserID string) {
	_, customerID, err := handler.customerByExternalUserID(request, externalUserID)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	handler.messages(writer, request, customerID)
}

func (handler *legacyExternalCustomerReadHandler) GetUserRecentMessages(writer http.ResponseWriter, request *http.Request, unionID string) {
	_, customerID, err := handler.customerByUnionID(request, unionID)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	handler.messages(writer, request, customerID)
}

// ResolveIdentity is for the legacy api_client_jwt route only. It validates
// the injected service principal before reading identity and never considers a
// human-session context as authentication.
func (handler *legacyExternalCustomerReadHandler) ResolveIdentity(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || handler.service == nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	principal, err := handler.service.AuthenticateOperation(request.Context(), request, legacyExternalIdentityResolvePurpose)
	if err != nil || strings.TrimSpace(principal.ClientID) == "" || strings.TrimSpace(principal.PrincipalID) == "" {
		if err == nil {
			err = authport.ErrUnauthorized
		}
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	ref, err := handler.identityResolveRef(request)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	var result contactport.CustomerID
	if ref.Kind == identityport.KindExtension {
		result, err = handler.resolveIdentityUnionID(request.Context(), ref.Value)
	} else {
		result, err = handler.resolveRef(request.Context(), ref)
	}
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	writeLegacyExternalCustomerReadJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "identity": map[string]any{"customer_id": int64(result)},
		"source_status": "canonical_identity", "route_owner": "ai_crm_next",
		"fallback_used": false, "real_external_call_executed": false,
	})
}

func (handler *legacyExternalCustomerReadHandler) timeline(writer http.ResponseWriter, request *http.Request, customerID contactport.CustomerID) {
	owner, limit, err := handler.timelineInput(request)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	result, err := handler.events.List(request.Context(), contactapp.CustomerEventInput{CustomerID: customerID, OwnerStaffID: cloneLegacyExternalInt64(owner), Limit: limit})
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	items, err := legacyExternalTimelineItems(customerID, result.Items)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	writeLegacyExternalCustomerReadJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "timeline": map[string]any{"customer_id": int64(customerID), "items": items, "count": len(items),
			"limit": resultLimit(limit, contactapp.CustomerListDefaultLimit), "offset": 0, "next_cursor": cloneLegacyExternalString(result.NextCursor)},
		"source_status": "canonical_contact_events", "route_owner": "ai_crm_next",
		"fallback_used": false, "real_external_call_executed": false,
	})
}

func (handler *legacyExternalCustomerReadHandler) messages(writer http.ResponseWriter, request *http.Request, customerID contactport.CustomerID) {
	owner, limit, err := handler.messagesInput(request)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	page, err := handler.chats.ListCustomerChatActivity(request.Context(), customer360port.CustomerChatActivityQuery{
		CustomerID: customerID, OwnerStaffID: cloneLegacyExternalInt64(owner), Limit: limit,
	})
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, err)
		return
	}
	messages, err := legacyExternalMessageItems(customerID, limit, page)
	if err != nil {
		writeLegacyExternalCustomerReadError(writer, request, errLegacyExternalCustomerReadUnavailable)
		return
	}
	writeLegacyExternalCustomerReadJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "messages": messages, "items": messages, "count": len(messages),
		"limit": resultLimit(limit, customer360app.CustomerChatActivityDefaultLimit), "source_status": "local_archive_summary", "route_owner": "ai_crm_next",
		"message_content_included": false, "participant_identity_included": false, "provider_message_id_included": false,
		"fallback_used": false, "real_external_call_executed": false,
	})
}

func (handler *legacyExternalCustomerReadHandler) customerByExternalUserID(request *http.Request, externalUserID string) (legacyExternalCustomer, contactport.CustomerID, error) {
	if handler == nil || request == nil || handler.corpID == "" || !validLegacyExternalIdentityValue(externalUserID) {
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadInvalid
	}
	result, err := handler.resolveRef(request.Context(), identityport.IDRef{Kind: identityport.KindWeComExternalUserID,
		Scope: "wecom-corp:" + handler.corpID, Value: externalUserID, Assurance: identityport.AssuranceVerified, Source: "legacy-external-customer-read"})
	if err != nil {
		return legacyExternalCustomer{}, 0, err
	}
	return handler.customerByID(request, result)
}

func (handler *legacyExternalCustomerReadHandler) customerByUnionID(request *http.Request, unionID string) (legacyExternalCustomer, contactport.CustomerID, error) {
	if handler == nil || request == nil || !validLegacyExternalIdentityValue(unionID) || nilLegacyDependency(handler.unionIDs) {
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadInvalid
	}
	result, err := handler.unionIDs.ResolveUnionID(request.Context(), unionID)
	if err != nil || !validLegacyExternalResolveResult(result) {
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadUnavailable
	}
	switch result.Status {
	case identityport.ResolveFound:
		return handler.customerByID(request, result.CustomerID)
	case identityport.ResolveNotFound:
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadNotFound
	default:
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadConflict
	}
}

func (handler *legacyExternalCustomerReadHandler) customerByID(request *http.Request, customerID contactport.CustomerID) (legacyExternalCustomer, contactport.CustomerID, error) {
	if handler == nil || request == nil || nilLegacyDependency(handler.detail) || customerID <= 0 {
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadUnavailable
	}
	owner, err := legacyExternalCustomerOwner(request.Context())
	if err != nil {
		return legacyExternalCustomer{}, 0, err
	}
	detail, err := handler.detail.Get(request.Context(), contactapp.CustomerDetailInput{ID: customerID, OwnerStaffID: cloneLegacyExternalInt64(owner)})
	if errors.Is(err, contactapp.ErrCustomerNotFound) {
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadNotFound
	}
	if err != nil {
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadUnavailable
	}
	customer, err := legacyExternalCustomerFromRecord(detail.Customer)
	if err != nil || customer.ID != int64(customerID) {
		return legacyExternalCustomer{}, 0, errLegacyExternalCustomerReadUnavailable
	}
	return customer, customerID, nil
}

func (handler *legacyExternalCustomerReadHandler) resolveRef(ctx context.Context, ref identityport.IDRef) (contactport.CustomerID, error) {
	if handler == nil || nilLegacyDependency(handler.identity) || ctx == nil {
		return 0, errLegacyExternalCustomerReadUnavailable
	}
	result, err := handler.identity.Resolve(ctx, ref)
	if err != nil || !validLegacyExternalResolveResult(result) {
		return 0, errLegacyExternalCustomerReadUnavailable
	}
	switch result.Status {
	case identityport.ResolveFound:
		return result.CustomerID, nil
	case identityport.ResolveNotFound:
		return 0, errLegacyExternalCustomerReadNotFound
	default:
		return 0, errLegacyExternalCustomerReadConflict
	}
}

func (handler *legacyExternalCustomerReadHandler) listInput(request *http.Request) (*int64, legacyExternalListQuery, error) {
	owner, err := legacyExternalCustomerOwner(request.Context())
	if err != nil {
		return nil, legacyExternalListQuery{}, err
	}
	values, err := legacyExternalQuery(request, "keyword", "limit", "offset")
	if err != nil {
		return nil, legacyExternalListQuery{}, err
	}
	query := legacyExternalListQuery{limit: contactapp.CustomerListDefaultLimit}
	if raw := values.Get("keyword"); raw != "" {
		if !validLegacyExternalText(raw, 200) {
			return nil, legacyExternalListQuery{}, errLegacyExternalCustomerReadInvalid
		}
		query.keyword = raw
	}
	if query.limit, err = legacyExternalLimit(values.Get("limit"), contactapp.CustomerListDefaultLimit, contactapp.CustomerListMaximumLimit); err != nil {
		return nil, legacyExternalListQuery{}, err
	}
	if raw := values.Get("offset"); raw != "" && raw != "0" {
		return nil, legacyExternalListQuery{}, errLegacyExternalCustomerReadInvalid
	}
	return owner, query, nil
}

func (handler *legacyExternalCustomerReadHandler) timelineInput(request *http.Request) (*int64, int32, error) {
	owner, err := legacyExternalCustomerOwner(request.Context())
	if err != nil {
		return nil, 0, err
	}
	values, err := legacyExternalQuery(request, "limit", "offset")
	if err != nil {
		return nil, 0, err
	}
	if raw := values.Get("offset"); raw != "" && raw != "0" {
		return nil, 0, errLegacyExternalCustomerReadInvalid
	}
	limit, err := legacyExternalLimit(values.Get("limit"), contactapp.CustomerListDefaultLimit, contactapp.CustomerListMaximumLimit)
	return owner, limit, err
}

func (handler *legacyExternalCustomerReadHandler) messagesInput(request *http.Request) (*int64, int32, error) {
	owner, err := legacyExternalCustomerOwner(request.Context())
	if err != nil {
		return nil, 0, err
	}
	values, err := legacyExternalQuery(request, "limit")
	if err != nil {
		return nil, 0, err
	}
	limit, err := legacyExternalLimit(values.Get("limit"), 20, customer360app.CustomerChatActivityMaximumLimit)
	return owner, limit, err
}

func (handler *legacyExternalCustomerReadHandler) identityResolveRef(request *http.Request) (identityport.IDRef, error) {
	if handler == nil || request == nil {
		return identityport.IDRef{}, errLegacyExternalCustomerReadUnavailable
	}
	values, err := legacyExternalQuery(request, "external_userid", "mobile", "unionid", "openid")
	if err != nil {
		return identityport.IDRef{}, err
	}
	if _, openIDPresent := values["openid"]; openIDPresent {
		return identityport.IDRef{}, errLegacyExternalCustomerReadInvalid
	}
	var ref identityport.IDRef
	for _, candidate := range []identityport.IDRef{
		{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + handler.corpID, Value: values.Get("external_userid")},
		{Kind: identityport.KindPhone, Scope: "phone:e164", Value: values.Get("mobile")},
	} {
		if candidate.Value == "" {
			continue
		}
		if ref.Value != "" || !validLegacyExternalIdentityValue(candidate.Value) {
			return identityport.IDRef{}, errLegacyExternalCustomerReadInvalid
		}
		ref = candidate
	}
	unionID := values.Get("unionid")
	if ref.Value != "" && unionID != "" {
		return identityport.IDRef{}, errLegacyExternalCustomerReadInvalid
	}
	if ref.Value != "" {
		ref.Assurance, ref.Source = identityport.AssuranceVerified, "legacy-external-identity-resolve"
		return ref, nil
	}
	if !validLegacyExternalIdentityValue(unionID) || nilLegacyDependency(handler.unionIDs) {
		return identityport.IDRef{}, errLegacyExternalCustomerReadInvalid
	}
	return identityport.IDRef{Kind: identityport.KindExtension, Scope: "ext:legacy-unionid", Value: unionID, Assurance: identityport.AssuranceVerified, Source: "legacy-external-identity-resolve"}, nil
}

// resolveIdentity accepts legacy unionid through the injected verified
// resolver. The extension ref is a private dispatch marker; it is never
// persisted or returned to the caller.
func (handler *legacyExternalCustomerReadHandler) resolveIdentityUnionID(ctx context.Context, value string) (contactport.CustomerID, error) {
	if handler == nil || nilLegacyDependency(handler.unionIDs) {
		return 0, errLegacyExternalCustomerReadUnavailable
	}
	result, err := handler.unionIDs.ResolveUnionID(ctx, value)
	if err != nil || !validLegacyExternalResolveResult(result) {
		return 0, errLegacyExternalCustomerReadUnavailable
	}
	if result.Status == identityport.ResolveFound {
		return result.CustomerID, nil
	}
	if result.Status == identityport.ResolveNotFound {
		return 0, errLegacyExternalCustomerReadNotFound
	}
	return 0, errLegacyExternalCustomerReadConflict
}

func legacyExternalCustomerOwner(ctx context.Context) (*int64, error) {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	if !ok || authorization.Capability != authport.CapabilityCustomersRead {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(ctx)
	if !ok || principal.AdminUserID < 1 {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	switch authorization.Scope {
	case authport.ScopeGlobal:
		if authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		return nil, nil
	case authport.ScopeOwnerStaff:
		if principal.Role != authport.RoleSales || principal.StaffID == nil || *principal.StaffID != authorization.OwnerStaffID || authorization.OwnerStaffID <= 0 {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		owner := authorization.OwnerStaffID
		return &owner, nil
	default:
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}

type legacyExternalListQuery struct {
	keyword string
	limit   int32
}

type legacyExternalCustomer struct {
	ID             int64      `json:"customer_id"`
	Name           string     `json:"name"`
	OwnerStaffID   *int64     `json:"owner_staff_id,omitempty"`
	StageID        *int64     `json:"stage_id,omitempty"`
	ChannelID      *int64     `json:"channel_id,omitempty"`
	AddedAt        *time.Time `json:"added_at,omitempty"`
	LastInteractAt *time.Time `json:"last_interact_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func legacyExternalCustomerListItems(records []contactapp.CustomerRecord) ([]legacyExternalCustomer, error) {
	items := make([]legacyExternalCustomer, 0, len(records))
	for _, record := range records {
		item, err := legacyExternalCustomerFromRecord(record)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func legacyExternalCustomerFromRecord(record contactapp.CustomerRecord) (legacyExternalCustomer, error) {
	if record.ID <= 0 || record.Name != strings.TrimSpace(record.Name) || !utf8.ValidString(record.Name) || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.CreatedAt.After(record.UpdatedAt) {
		return legacyExternalCustomer{}, errLegacyExternalCustomerReadUnavailable
	}
	return legacyExternalCustomer{ID: int64(record.ID), Name: record.Name, OwnerStaffID: cloneLegacyExternalInt64(record.OwnerStaffID),
		StageID: cloneLegacyExternalInt64(record.StageID), ChannelID: cloneLegacyExternalInt64(record.ChannelID), AddedAt: cloneLegacyExternalTime(record.AddedAt),
		LastInteractAt: cloneLegacyExternalTime(record.LastInteractAt), CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC()}, nil
}

func legacyExternalTimelineItems(customerID contactport.CustomerID, records []contactapp.CustomerEventRecord) ([]map[string]any, error) {
	items := make([]map[string]any, 0, len(records))
	for index, record := range records {
		if record.ID <= 0 || record.CustomerID != customerID || !validLegacyExternalText(record.EventType, 200) || record.OccurredAt.IsZero() ||
			(index > 0 && records[index-1].OccurredAt.Before(record.OccurredAt)) {
			return nil, errLegacyExternalCustomerReadUnavailable
		}
		items = append(items, map[string]any{"event_type": record.EventType, "event_time": record.OccurredAt.UTC()})
	}
	return items, nil
}

func legacyExternalMessageItems(customerID contactport.CustomerID, limit int32, page customer360port.CustomerChatActivityPage) ([]map[string]any, error) {
	if customerID <= 0 || page.CustomerID != customerID || page.Total < int64(len(page.Items)) || len(page.Items) > int(resultLimit(limit, customer360app.CustomerChatActivityDefaultLimit)) {
		return nil, errLegacyExternalCustomerReadUnavailable
	}
	items := make([]map[string]any, 0, len(page.Items))
	for index, item := range page.Items {
		if (item.ChatType != "private" && item.ChatType != "group") || !validLegacyExternalText(item.MessageType, 128) || item.SentAt.IsZero() ||
			(index > 0 && page.Items[index-1].SentAt.Before(item.SentAt)) {
			return nil, errLegacyExternalCustomerReadUnavailable
		}
		items = append(items, map[string]any{"chat_type": item.ChatType, "message_type": item.MessageType, "send_time": item.SentAt.UTC()})
	}
	return items, nil
}

func legacyExternalQuery(request *http.Request, allowed ...string) (url.Values, error) {
	if request == nil || request.URL == nil || !utf8.ValidString(request.URL.RawQuery) {
		return nil, errLegacyExternalCustomerReadInvalid
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return nil, errLegacyExternalCustomerReadInvalid
	}
	permitted := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		permitted[key] = struct{}{}
	}
	for key, entries := range values {
		if _, ok := permitted[key]; !ok || len(entries) != 1 || !utf8.ValidString(entries[0]) || strings.IndexFunc(entries[0], unicode.IsControl) >= 0 {
			return nil, errLegacyExternalCustomerReadInvalid
		}
	}
	return values, nil
}

func legacyExternalLimit(raw string, fallback, maximum int32) (int32, error) {
	if raw == "" {
		return fallback, nil
	}
	if raw != strings.TrimSpace(raw) || len(raw) > 10 {
		return 0, errLegacyExternalCustomerReadInvalid
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 1 || value > int64(maximum) {
		return 0, errLegacyExternalCustomerReadInvalid
	}
	return int32(value), nil
}

func validLegacyExternalIdentityValue(value string) bool { return validLegacyExternalText(value, 1024) }

func validLegacyExternalText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validLegacyExternalResolveResult(result identityport.ResolveResult) bool {
	return (result.Status == identityport.ResolveFound && result.CustomerID > 0) || ((result.Status == identityport.ResolveNotFound || result.Status == identityport.ResolveConflict) && result.CustomerID == 0)
}

func resultLimit(value, fallback int32) int32 {
	if value == 0 {
		return fallback
	}
	return value
}

func cloneLegacyExternalInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneLegacyExternalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneLegacyExternalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func writeLegacyExternalCustomerReadJSON(writer http.ResponseWriter, status int, value any) {
	if writer == nil {
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeLegacyExternalCustomerReadError(writer http.ResponseWriter, request *http.Request, err error) {
	if request == nil {
		return
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, errLegacyExternalCustomerReadInvalid), errors.Is(err, contactapp.ErrInvalidCustomerListQuery), errors.Is(err, contactapp.ErrInvalidCustomerEventQuery), errors.Is(err, customer360port.ErrInvalidCustomerChatActivity):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, errLegacyExternalCustomerReadNotFound), errors.Is(err, contactapp.ErrCustomerNotFound), errors.Is(err, contactport.ErrCustomerReadNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, errLegacyExternalCustomerReadConflict):
		code = platformhttp.CodeConflict
	case errors.Is(err, authport.ErrUnauthorized):
		code = platformhttp.CodeUnauthorized
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}
