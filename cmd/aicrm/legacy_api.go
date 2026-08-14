// This file adapts the frozen legacy browser transport at the aicrm
// composition root. It owns no business rules, storage, or provider calls.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	LegacySessionCookieName = "aicrm_next_admin_session"
	LegacyCSRFCookieName    = "aicrm_next_csrf"
	legacySessionMaxAge     = 8 * time.Hour
)

var errInvalidLegacyQuery = errors.New("legacy customer list query cannot be mapped safely")

type customerListApplication interface {
	List(context.Context, contactapp.CustomerListInput) (contactapp.CustomerListResult, error)
}

// Handler is deliberately a thin transport adapter over existing v2 services.
type Handler struct {
	auth        authport.Service
	customers   customerListApplication
	outbound    legacyOutboundQueryApplication
	cancel      legacyCancelApplication
	manualRetry legacyRetryApplication
}

func NewHandler(auth authport.Service, customers customerListApplication) (*Handler, error) {
	if nilAuth(auth) || nilCustomers(customers) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	return &Handler{auth: auth, customers: customers}, nil
}

func NewHandlerWithOutbound(
	auth authport.Service,
	customers customerListApplication,
	outbound legacyOutboundQueryApplication,
	cancel legacyCancelApplication,
	manualRetry legacyRetryApplication,
) (*Handler, error) {
	handler, err := NewHandler(auth, customers)
	if err != nil || nilLegacyDependency(outbound) || nilLegacyDependency(cancel) || nilLegacyDependency(manualRetry) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	handler.outbound = outbound
	handler.cancel = cancel
	handler.manualRetry = manualRetry
	return handler, nil
}

// Authenticate accepts the current v2 cookie and the frozen legacy name. The
// opaque value remains exclusively owned by the v2 auth service.
func (handler *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if handler == nil || nilAuth(handler.auth) || next == nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, authport.ErrAuthenticationUnavailable))
			return
		}
		session, err := browserSession(request)
		if err != nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
			return
		}
		principal, err := handler.auth.Authenticate(request.Context(), session)
		if err != nil {
			code := platformhttp.CodeUnauthenticated
			if errors.Is(err, authport.ErrAuthenticationUnavailable) {
				code = platformhttp.CodeDependencyUnavailable
			}
			platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
			return
		}
		if principal.AdminUserID < 1 {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, authport.ErrUnauthenticated))
			return
		}
		ctx, err := platformhttp.ContextWithAccountID(request.Context(), "admin:"+strconv.FormatInt(principal.AdminUserID, 10))
		if err != nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, err))
			return
		}
		next.ServeHTTP(writer, request.WithContext(authport.WithAuthenticatedSession(ctx, principal, session)))
	})
}

func (handler *Handler) Authorize(capability authport.Capability, next http.Handler) (http.Handler, error) {
	if handler == nil || nilAuth(handler.auth) || !capability.Known() || next == nil {
		return nil, authport.ErrUnauthorized
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := authport.PrincipalFromContext(request.Context())
		if !ok {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
			return
		}
		authorization, err := handler.auth.Authorize(request.Context(), principal, capability)
		if err != nil {
			code := platformhttp.CodeUnauthorized
			if !errors.Is(err, authport.ErrUnauthorized) {
				code = platformhttp.CodeDependencyUnavailable
			}
			platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
			return
		}
		ctx, err := authport.WithAuthorization(request.Context(), authorization)
		if err != nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, err))
			return
		}
		next.ServeHTTP(writer, request.WithContext(ctx))
	}), nil
}

// RequireCSRF is kept in the adapter so a later legacy state-changing route
// cannot accidentally accept an unbound old cookie name.
func (handler *Handler) RequireCSRF(next http.Handler) (http.Handler, error) {
	if handler == nil || nilAuth(handler.auth) || next == nil {
		return nil, authport.ErrUnauthorized
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session, ok := authport.SessionFromContext(request.Context())
		if !ok {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
			return
		}
		values := request.Header.Values("X-CSRF-Token")
		if len(values) != 1 || !validToken(values[0]) {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrCSRFInvalid))
			return
		}
		if err := handler.auth.ValidateCSRF(request.Context(), session, authport.CSRFToken(values[0])); err != nil {
			code := platformhttp.CodeUnauthorized
			if errors.Is(err, authport.ErrUnauthenticated) {
				code = platformhttp.CodeUnauthenticated
			} else if errors.Is(err, authport.ErrAuthenticationUnavailable) {
				code = platformhttp.CodeDependencyUnavailable
			}
			platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
			return
		}
		next.ServeHTTP(writer, request)
	}), nil
}

// ConfigOverview preserves the old envelope while reporting real v2 operation
// permissions for the authenticated principal. It contains no persisted or
// secret configuration because no such v2 read service is wired in this slice.
func (handler *Handler) ConfigOverview(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilAuth(handler.auth) || request == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, authport.ErrAuthenticationUnavailable))
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
		return
	}
	capabilities := handler.allowedCapabilities(request.Context(), principal)
	mirrorLegacyCSRFCookie(writer, request)
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true,
		"overview": map[string]any{
			"categories": []map[string]any{{"key": "v2_auth", "capabilities": capabilities}},
		},
		"source_status": "v2_auth_policy",
		"fallback_used": false,
	})
}

// Capabilities is the frozen legacy capability-read path backed by the v2
// closed authorization policy, rather than a compatibility-only registry.
func (handler *Handler) Capabilities(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilAuth(handler.auth) || request == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, authport.ErrAuthenticationUnavailable))
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
		return
	}
	mirrorLegacyCSRFCookie(writer, request)
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok":            true,
		"registry":      map[string]any{"capabilities": handler.allowedCapabilities(request.Context(), principal)},
		"source_status": "v2_auth_policy",
		"fallback_used": false,
	})
}

// ListCustomers calls the v2 Contact application service. Legacy identity
// filters and OFFSET pagination are intentionally rejected: v2's OneID and
// signed keyset contract must not be weakened or fabricated at this boundary.
func (handler *Handler) ListCustomers(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilCustomers(handler.customers) || request == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, errInvalidLegacyQuery))
		return
	}
	input, filters, err := legacyCustomerListInput(request)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityCustomersRead {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	input.OwnerStaffID, err = legacyOwnerScope(authorization, input.OwnerStaffID)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, err))
		return
	}
	result, err := handler.customers.List(request.Context(), input)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, err))
		return
	}
	items := make([]legacyCustomer, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, mapCustomer(item))
	}
	mirrorLegacyCSRFCookie(writer, request)
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "customers": items, "items": items, "count": len(items),
		"total": result.Total, "total_is_estimate": result.TotalIsEstimate,
		"has_more": result.NextCursor != nil, "limit": input.Limit, "offset": 0,
		"filters": filters, "projection_watermark": result.Watermark.UTC(),
		"source_status": "v2_contact_service", "fallback_used": false,
	})
}

type legacyCustomer struct {
	CustomerID     int64      `json:"customer_id"`
	CustomerName   string     `json:"customer_name"`
	AvatarURL      *string    `json:"avatar_url,omitempty"`
	StageID        *int64     `json:"stage_id,omitempty"`
	OwnerStaffID   *int64     `json:"owner_staff_id,omitempty"`
	ChannelID      *int64     `json:"channel_id,omitempty"`
	AddedAt        *time.Time `json:"added_at,omitempty"`
	LastInteractAt *time.Time `json:"last_interact_at,omitempty"`
	IsDeleted      bool       `json:"is_deleted"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func mapCustomer(item contactapp.CustomerRecord) legacyCustomer {
	return legacyCustomer{CustomerID: int64(item.ID), CustomerName: item.Name, AvatarURL: item.AvatarURL,
		StageID: item.StageID, OwnerStaffID: item.OwnerStaffID, ChannelID: item.ChannelID,
		AddedAt: item.AddedAt, LastInteractAt: item.LastInteractAt, IsDeleted: item.IsDeleted,
		CreatedAt: item.CreatedAt.UTC(), UpdatedAt: item.UpdatedAt.UTC()}
}

func legacyCustomerListInput(request *http.Request) (contactapp.CustomerListInput, map[string]any, error) {
	if request == nil {
		return contactapp.CustomerListInput{}, nil, errInvalidLegacyQuery
	}
	query := request.URL.Query()
	for _, key := range []string{"tag", "status", "is_bound", "mobile"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			return contactapp.CustomerListInput{}, nil, errInvalidLegacyQuery
		}
	}
	if offset := strings.TrimSpace(query.Get("offset")); offset != "" && offset != "0" {
		return contactapp.CustomerListInput{}, nil, errInvalidLegacyQuery
	}
	limit := contactapp.CustomerListDefaultLimit
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed < 1 || parsed > int64(contactapp.CustomerListMaximumLimit) {
			return contactapp.CustomerListInput{}, nil, errInvalidLegacyQuery
		}
		limit = int32(parsed)
	}
	input := contactapp.CustomerListInput{Keyword: query.Get("keyword"), Limit: limit}
	filters := map[string]any{"keyword": input.Keyword, "owner_userid": "", "tag": "", "status": "", "is_bound": "", "mobile": ""}
	if rawOwner := strings.TrimSpace(query.Get("owner_userid")); rawOwner != "" {
		owner, err := strconv.ParseInt(rawOwner, 10, 64)
		if err != nil || owner < 1 {
			return contactapp.CustomerListInput{}, nil, errInvalidLegacyQuery
		}
		input.OwnerStaffID = &owner
		filters["owner_userid"] = rawOwner
	}
	return input, filters, nil
}

func legacyOwnerScope(authorization authport.Authorization, requested *int64) (*int64, error) {
	switch authorization.Scope {
	case authport.ScopeGlobal:
		if authorization.OwnerStaffID != 0 {
			return nil, authport.ErrUnauthorized
		}
		return requested, nil
	case authport.ScopeOwnerStaff:
		if authorization.OwnerStaffID < 1 || (requested != nil && *requested != authorization.OwnerStaffID) {
			return nil, authport.ErrUnauthorized
		}
		owner := authorization.OwnerStaffID
		return &owner, nil
	default:
		return nil, authport.ErrUnauthorized
	}
}

func (handler *Handler) allowedCapabilities(ctx context.Context, principal authport.Principal) []string {
	if handler == nil || nilAuth(handler.auth) {
		return nil
	}
	all := []authport.Capability{
		authport.CapabilityAuthSessionRead, authport.CapabilityAuthSessionLogout,
		authport.CapabilityCustomersRead, authport.CapabilityCustomersWrite,
		authport.CapabilityCustomerEventsRead, authport.CapabilityIdentityResolve,
		authport.CapabilityIdentityBind, authport.CapabilityIdentityIngest,
		authport.CapabilityIdentityReviewRead, authport.CapabilityIdentityReviewWrite,
		authport.CapabilityConfigOverviewRead, authport.CapabilityStagesRead,
		authport.CapabilityStagesWrite, authport.CapabilitySegmentsRead, authport.CapabilitySegmentsWrite,
		authport.CapabilityOutboundRead, authport.CapabilityOutboundControl,
	}
	allowed := make([]string, 0, len(all))
	for _, capability := range all {
		if _, err := handler.auth.Authorize(ctx, principal, capability); err == nil {
			allowed = append(allowed, string(capability))
		}
	}
	return allowed
}

func browserSession(request *http.Request) (authport.SessionRef, error) {
	if request == nil {
		return "", http.ErrNoCookie
	}
	for _, name := range []string{authhttp.SessionCookieName, LegacySessionCookieName} {
		cookie, err := request.Cookie(name)
		if err == nil && validToken(cookie.Value) {
			return authport.SessionRef(cookie.Value), nil
		}
	}
	return "", http.ErrNoCookie
}

func mirrorLegacyCSRFCookie(writer http.ResponseWriter, request *http.Request) {
	if writer == nil || request == nil {
		return
	}
	if _, err := request.Cookie(LegacyCSRFCookieName); err == nil {
		return
	}
	cookie, err := request.Cookie(authhttp.CSRFCookieName)
	if err != nil || !validToken(cookie.Value) {
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: LegacyCSRFCookieName, Value: cookie.Value, Path: "/", MaxAge: int(legacySessionMaxAge.Seconds()), Secure: true, HttpOnly: false, SameSite: http.SameSiteLaxMode})
}

func validToken(token string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(token) == 43 && len(decoded) == 32
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func nilAuth(service authport.Service) bool {
	if service == nil {
		return true
	}
	value := reflect.ValueOf(service)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func nilCustomers(application customerListApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func nilLegacyDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
