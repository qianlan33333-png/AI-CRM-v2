package main

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const legacyCustomerProfileTagsPath = "/api/admin/customers/profile/tags"

var (
	errInvalidLegacyCustomerProfileTagsQuery    = errors.New("invalid customer profile tags query")
	errUnsupportedLegacyCustomerProfileTagsHint = errors.New("unsupported customer profile tags identity hint")
)

// legacyCustomerProfileTagsQuery intentionally contains only the two frozen,
// unambiguous read hints. user_id is rejected rather than guessed because it
// has no owner-stable identity meaning in the current schema.
type legacyCustomerProfileTagsQuery struct {
	UnionID        string
	ExternalUserID string
}

type legacyCustomerProfileTag struct {
	Name string `json:"name"`
}

type legacyCustomerProfileTagsSuccess struct {
	OK   bool                       `json:"ok"`
	Tags []legacyCustomerProfileTag `json:"tags"`
}

type legacyCustomerProfileTagsError struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	ErrorCode  string `json:"error_code"`
}

func (handler *Handler) GetCustomerProfileTags(writer http.ResponseWriter, request *http.Request) {
	if !legacyCustomerProfileTagsAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if request == nil || request.URL == nil {
		writeLegacyCustomerProfileTagsUnavailable(writer)
		return
	}
	query, err := parseLegacyCustomerProfileTagsQuery(request.URL.RawQuery)
	if errors.Is(err, errUnsupportedLegacyCustomerProfileTagsHint) {
		writeLegacyCustomerProfileTagsError(writer, http.StatusUnprocessableEntity, "unsupported_identity_hint")
		return
	}
	if err != nil {
		writeLegacyCustomerProfileTagsError(writer, http.StatusUnprocessableEntity, "invalid_identity_hint")
		return
	}
	if handler == nil || nilLegacyDependency(handler.customerDetail) {
		writeLegacyCustomerProfileTagsUnavailable(writer)
		return
	}

	var customerID contactport.CustomerID
	if query.UnionID != "" && query.ExternalUserID != "" {
		// The owner freeze requires both hints to be resolved before deciding.
		// Do not let a first not-found or conflict hide the other identity fact.
		if nilLegacyDependency(handler.messageArchiveUnionID) || nilLegacyDependency(handler.identityResolve) || handler.weComCorpID == "" {
			writeLegacyCustomerProfileTagsUnavailable(writer)
			return
		}
		unionResult, unionErr := handler.messageArchiveUnionID.ResolveUnionID(request.Context(), query.UnionID)
		externalResult, externalErr := handler.identityResolve.Resolve(request.Context(), identityport.IDRef{
			Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + handler.weComCorpID,
			Value: query.ExternalUserID, Assurance: identityport.AssuranceVerified, Source: "legacy-customer-profile-tags",
		})
		if unionErr != nil || externalErr != nil || !validLegacyCustomerProfileTagsResolution(unionResult) || !validLegacyCustomerProfileTagsResolution(externalResult) {
			writeLegacyCustomerProfileTagsUnavailable(writer)
			return
		}
		if unionResult.Status != identityport.ResolveFound || externalResult.Status != identityport.ResolveFound || unionResult.CustomerID != externalResult.CustomerID {
			writeLegacyCustomerProfileTagsError(writer, http.StatusConflict, "identity_hint_conflict")
			return
		}
		customerID = unionResult.CustomerID
	} else if query.UnionID != "" {
		if nilLegacyDependency(handler.messageArchiveUnionID) {
			writeLegacyCustomerProfileTagsUnavailable(writer)
			return
		}
		result, resolveErr := handler.messageArchiveUnionID.ResolveUnionID(request.Context(), query.UnionID)
		if resolveErr != nil || !validLegacyCustomerProfileTagsResolution(result) {
			writeLegacyCustomerProfileTagsUnavailable(writer)
			return
		}
		switch result.Status {
		case identityport.ResolveFound:
			customerID = result.CustomerID
		case identityport.ResolveNotFound:
			writeLegacyCustomerProfileTagsNotFound(writer)
			return
		default:
			writeLegacyCustomerProfileTagsError(writer, http.StatusConflict, "identity_hint_conflict")
			return
		}
	} else {
		if nilLegacyDependency(handler.identityResolve) || handler.weComCorpID == "" {
			writeLegacyCustomerProfileTagsUnavailable(writer)
			return
		}
		result, resolveErr := handler.identityResolve.Resolve(request.Context(), identityport.IDRef{
			Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + handler.weComCorpID,
			Value: query.ExternalUserID, Assurance: identityport.AssuranceVerified, Source: "legacy-customer-profile-tags",
		})
		if resolveErr != nil || !validLegacyCustomerProfileTagsResolution(result) {
			writeLegacyCustomerProfileTagsUnavailable(writer)
			return
		}
		switch result.Status {
		case identityport.ResolveFound:
			customerID = result.CustomerID
		case identityport.ResolveNotFound:
			writeLegacyCustomerProfileTagsNotFound(writer)
			return
		default:
			writeLegacyCustomerProfileTagsError(writer, http.StatusConflict, "identity_hint_conflict")
			return
		}
	}

	profile, err := handler.customerDetail.Get(request.Context(), contactapp.CustomerDetailInput{ID: customerID})
	if errors.Is(err, contactapp.ErrCustomerNotFound) {
		writeLegacyCustomerProfileTagsNotFound(writer)
		return
	}
	if err != nil {
		writeLegacyCustomerProfileTagsUnavailable(writer)
		return
	}
	tags, valid := legacyCustomerProfileTagNames(profile.Tags)
	if !valid {
		writeLegacyCustomerProfileTagsUnavailable(writer)
		return
	}
	writeJSON(writer, http.StatusOK, legacyCustomerProfileTagsSuccess{OK: true, Tags: tags})
}

func parseLegacyCustomerProfileTagsQuery(rawQuery string) (legacyCustomerProfileTagsQuery, error) {
	if !utf8.ValidString(rawQuery) {
		return legacyCustomerProfileTagsQuery{}, errInvalidLegacyCustomerProfileTagsQuery
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return legacyCustomerProfileTagsQuery{}, errInvalidLegacyCustomerProfileTagsQuery
	}
	if _, exists := values["user_id"]; exists {
		return legacyCustomerProfileTagsQuery{}, errUnsupportedLegacyCustomerProfileTagsHint
	}
	for key, entries := range values {
		if (key != "unionid" && key != "external_userid") || !utf8.ValidString(key) || len(entries) != 1 || !validLegacyCustomerProfileTagsHint(entries[0]) {
			return legacyCustomerProfileTagsQuery{}, errInvalidLegacyCustomerProfileTagsQuery
		}
	}
	unionID, unionOK := values["unionid"]
	externalUserID, externalUserIDOK := values["external_userid"]
	if !unionOK && !externalUserIDOK {
		return legacyCustomerProfileTagsQuery{}, errInvalidLegacyCustomerProfileTagsQuery
	}
	query := legacyCustomerProfileTagsQuery{}
	if unionOK {
		query.UnionID = strings.TrimSpace(unionID[0])
	}
	if externalUserIDOK {
		query.ExternalUserID = strings.TrimSpace(externalUserID[0])
	}
	return query, nil
}

func validLegacyCustomerProfileTagsHint(value string) bool {
	if !utf8.ValidString(value) || len(value) > 1024 || strings.TrimSpace(value) == "" {
		return false
	}
	return !strings.ContainsFunc(value, unicode.IsControl)
}

func validLegacyCustomerProfileTagsResolution(result identityport.ResolveResult) bool {
	if result.Status == identityport.ResolveFound {
		return result.CustomerID > 0
	}
	return result.Status == identityport.ResolveNotFound || result.Status == identityport.ResolveConflict
}

func legacyCustomerProfileTagNames(records []contactapp.CustomerTagRecord) ([]legacyCustomerProfileTag, bool) {
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		name := strings.TrimSpace(record.Name)
		if name == "" || !utf8.ValidString(name) {
			return nil, false
		}
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	tags := make([]legacyCustomerProfileTag, 0, len(names))
	for _, name := range names {
		tags = append(tags, legacyCustomerProfileTag{Name: name})
	}
	return tags, true
}

func writeLegacyCustomerProfileTagsError(writer http.ResponseWriter, status int, code string) {
	switch status {
	case http.StatusConflict:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeConflict)
	case http.StatusUnprocessableEntity:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeValidationFailed)
	default:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	}
	writeJSON(writer, status, legacyCustomerProfileTagsError{OK: false, StatusCode: status, ErrorCode: code})
}

func writeLegacyCustomerProfileTagsNotFound(writer http.ResponseWriter) {
	platformhttp.MarkCompatibilityError(writer, platformhttp.CodeNotFound)
	writeJSON(writer, http.StatusNotFound, legacyCustomerProfileTagsError{OK: false, StatusCode: http.StatusNotFound, ErrorCode: "customer_not_found"})
}

func writeLegacyCustomerProfileTagsUnavailable(writer http.ResponseWriter) {
	platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	writeJSON(writer, http.StatusServiceUnavailable, legacyCustomerProfileTagsError{OK: false, StatusCode: http.StatusServiceUnavailable, ErrorCode: "customer_profile_tags_unavailable"})
}

func writeLegacyCustomerProfileTagsMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	setLegacyCustomerProfileTagsSecurityHeaders(writer)
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func legacyCustomerProfileTagsSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(legacyCustomerProfileTagsHeaderWriter{ResponseWriter: writer}, request)
	})
}

type legacyCustomerProfileTagsHeaderWriter struct {
	http.ResponseWriter
}

func (writer legacyCustomerProfileTagsHeaderWriter) WriteHeader(status int) {
	setLegacyCustomerProfileTagsSecurityHeaders(writer.ResponseWriter)
	writer.ResponseWriter.WriteHeader(status)
}

func (writer legacyCustomerProfileTagsHeaderWriter) Write(payload []byte) (int, error) {
	setLegacyCustomerProfileTagsSecurityHeaders(writer.ResponseWriter)
	return writer.ResponseWriter.Write(payload)
}

func setLegacyCustomerProfileTagsSecurityHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func legacyCustomerProfileTagsAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin && authorizationOK && authorization.Capability == authport.CapabilityAdminRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}
