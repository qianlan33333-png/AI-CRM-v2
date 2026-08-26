package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const (
	legacyCustomerProfileMessagesPath          = "/api/admin/customers/profile/messages"
	legacyCustomerProfileMessagesDefaultLimit  = int32(30)
	legacyCustomerProfileMessagesFetchAllLimit = int32(100)
)

var (
	errInvalidLegacyCustomerProfileMessagesQuery    = errors.New("invalid customer profile messages query")
	errUnsupportedLegacyCustomerProfileMessagesHint = errors.New("unsupported customer profile messages identity hint")
	errInvalidLegacyCustomerProfileMessagesFetchAll = errors.New("invalid customer profile messages fetch_all")
)

type legacyCustomerProfileMessagesQuery struct {
	UnionID        string
	ExternalUserID string
	FetchAll       bool
}

type legacyCustomerProfileMessage struct {
	ChatType    string `json:"chat_type"`
	MessageType string `json:"msgtype"`
	SendTime    string `json:"send_time"`
}

type legacyCustomerProfileMessagesSuccess struct {
	OK                       bool                           `json:"ok"`
	Messages                 []legacyCustomerProfileMessage `json:"messages"`
	Count                    int                            `json:"count"`
	Limit                    int32                          `json:"limit"`
	SourceStatus             string                         `json:"source_status"`
	RouteOwner               string                         `json:"route_owner"`
	RealExternalCallExecuted bool                           `json:"real_external_call_executed"`
}

type legacyCustomerProfileMessagesError struct {
	OK                       bool   `json:"ok"`
	StatusCode               int    `json:"status_code"`
	ErrorCode                string `json:"error_code"`
	RealExternalCallExecuted bool   `json:"real_external_call_executed"`
}

func (handler *Handler) GetCustomerProfileMessages(writer http.ResponseWriter, request *http.Request) {
	setLegacyCustomerProfileMessagesSecurityHeaders(writer)
	if request != nil && request.Method != http.MethodGet {
		writeLegacyCustomerProfileMessagesMethodNotAllowed(writer, request)
		return
	}
	if !legacyCustomerProfileMessagesAuthorized(request) {
		writeLegacyCustomerProfileMessagesError(writer, http.StatusForbidden, "customer_profile_messages_forbidden")
		return
	}
	if request == nil || request.URL == nil {
		writeLegacyCustomerProfileMessagesUnavailable(writer)
		return
	}

	query, err := parseLegacyCustomerProfileMessagesQuery(request.URL.RawQuery)
	switch {
	case errors.Is(err, errUnsupportedLegacyCustomerProfileMessagesHint):
		writeLegacyCustomerProfileMessagesError(writer, http.StatusUnprocessableEntity, "unsupported_identity_hint")
		return
	case errors.Is(err, errInvalidLegacyCustomerProfileMessagesFetchAll):
		writeLegacyCustomerProfileMessagesError(writer, http.StatusUnprocessableEntity, "invalid_fetch_all")
		return
	case err != nil:
		writeLegacyCustomerProfileMessagesError(writer, http.StatusUnprocessableEntity, "invalid_identity_hint")
		return
	}
	if handler == nil || nilLegacyDependency(handler.customerDetail) {
		writeLegacyCustomerProfileMessagesUnavailable(writer)
		return
	}

	customerID, status := handler.resolveLegacyCustomerProfileMessagesCustomerID(request, query)
	if status != 0 {
		switch status {
		case http.StatusNotFound:
			writeLegacyCustomerProfileMessagesNotFound(writer)
		case http.StatusConflict:
			writeLegacyCustomerProfileMessagesError(writer, http.StatusConflict, "identity_hint_conflict")
		default:
			writeLegacyCustomerProfileMessagesUnavailable(writer)
		}
		return
	}

	if _, err = handler.customerDetail.Get(request.Context(), contactapp.CustomerDetailInput{ID: customerID}); errors.Is(err, contactapp.ErrCustomerNotFound) {
		writeLegacyCustomerProfileMessagesNotFound(writer)
		return
	} else if err != nil {
		writeLegacyCustomerProfileMessagesUnavailable(writer)
		return
	}

	archiveReader, ok := handler.messageArchive.(wecomport.CustomerChatSummaryReader)
	if !ok || nilLegacyDependency(archiveReader) {
		writeLegacyCustomerProfileMessagesUnavailable(writer)
		return
	}
	limit := legacyCustomerProfileMessagesDefaultLimit
	if query.FetchAll {
		limit = legacyCustomerProfileMessagesFetchAllLimit
	}
	page, err := archiveReader.ListCustomerChatSummaries(request.Context(), wecomport.CustomerChatSummaryQuery{
		CustomerID: customerID,
		Limit:      limit,
		Offset:     0,
	})
	if err != nil {
		writeLegacyCustomerProfileMessagesUnavailable(writer)
		return
	}
	messages, valid := legacyCustomerProfileMessages(page, limit)
	if !valid {
		writeLegacyCustomerProfileMessagesUnavailable(writer)
		return
	}
	writeLegacyCustomerProfileMessagesJSON(writer, http.StatusOK, legacyCustomerProfileMessagesSuccess{
		OK:                       true,
		Messages:                 messages,
		Count:                    len(messages),
		Limit:                    limit,
		SourceStatus:             "message_archive_read_model",
		RouteOwner:               archiveRouteOwner,
		RealExternalCallExecuted: false,
	})
}

func parseLegacyCustomerProfileMessagesQuery(rawQuery string) (legacyCustomerProfileMessagesQuery, error) {
	if !utf8.ValidString(rawQuery) {
		return legacyCustomerProfileMessagesQuery{}, errInvalidLegacyCustomerProfileMessagesQuery
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return legacyCustomerProfileMessagesQuery{}, errInvalidLegacyCustomerProfileMessagesQuery
	}
	if _, exists := values["user_id"]; exists {
		return legacyCustomerProfileMessagesQuery{}, errUnsupportedLegacyCustomerProfileMessagesHint
	}
	for key, entries := range values {
		switch key {
		case "unionid", "external_userid":
			if len(entries) != 1 || !validLegacyCustomerProfileTagsHint(entries[0]) {
				return legacyCustomerProfileMessagesQuery{}, errInvalidLegacyCustomerProfileMessagesQuery
			}
		case "fetch_all":
			if len(entries) != 1 {
				return legacyCustomerProfileMessagesQuery{}, errInvalidLegacyCustomerProfileMessagesFetchAll
			}
		default:
			return legacyCustomerProfileMessagesQuery{}, errInvalidLegacyCustomerProfileMessagesQuery
		}
	}

	unionID, unionOK := values["unionid"]
	externalUserID, externalUserIDOK := values["external_userid"]
	if !unionOK && !externalUserIDOK {
		return legacyCustomerProfileMessagesQuery{}, errInvalidLegacyCustomerProfileMessagesQuery
	}
	query := legacyCustomerProfileMessagesQuery{}
	if unionOK {
		query.UnionID = strings.TrimSpace(unionID[0])
	}
	if externalUserIDOK {
		query.ExternalUserID = strings.TrimSpace(externalUserID[0])
	}
	if fetchAll, exists := values["fetch_all"]; exists {
		switch fetchAll[0] {
		case "false":
			query.FetchAll = false
		case "true":
			query.FetchAll = true
		default:
			return legacyCustomerProfileMessagesQuery{}, errInvalidLegacyCustomerProfileMessagesFetchAll
		}
	}
	return query, nil
}

func (handler *Handler) resolveLegacyCustomerProfileMessagesCustomerID(request *http.Request, query legacyCustomerProfileMessagesQuery) (contactport.CustomerID, int) {
	if handler == nil || request == nil {
		return 0, http.StatusServiceUnavailable
	}
	if query.UnionID != "" && query.ExternalUserID != "" {
		if nilLegacyDependency(handler.messageArchiveUnionID) || nilLegacyDependency(handler.identityResolve) || handler.weComCorpID == "" {
			return 0, http.StatusServiceUnavailable
		}
		unionResult, unionErr := handler.messageArchiveUnionID.ResolveUnionID(request.Context(), query.UnionID)
		externalResult, externalErr := handler.identityResolve.Resolve(request.Context(), identityport.IDRef{
			Kind:      identityport.KindWeComExternalUserID,
			Scope:     "wecom-corp:" + handler.weComCorpID,
			Value:     query.ExternalUserID,
			Assurance: identityport.AssuranceVerified,
			Source:    "legacy-customer-profile-messages",
		})
		if unionErr != nil || externalErr != nil || !validLegacyCustomerProfileTagsResolution(unionResult) || !validLegacyCustomerProfileTagsResolution(externalResult) {
			return 0, http.StatusServiceUnavailable
		}
		if unionResult.Status != identityport.ResolveFound || externalResult.Status != identityport.ResolveFound || unionResult.CustomerID != externalResult.CustomerID {
			return 0, http.StatusConflict
		}
		return unionResult.CustomerID, 0
	}
	if query.UnionID != "" {
		if nilLegacyDependency(handler.messageArchiveUnionID) {
			return 0, http.StatusServiceUnavailable
		}
		result, err := handler.messageArchiveUnionID.ResolveUnionID(request.Context(), query.UnionID)
		if err != nil || !validLegacyCustomerProfileTagsResolution(result) {
			return 0, http.StatusServiceUnavailable
		}
		switch result.Status {
		case identityport.ResolveFound:
			return result.CustomerID, 0
		case identityport.ResolveNotFound:
			return 0, http.StatusNotFound
		default:
			return 0, http.StatusConflict
		}
	}
	if nilLegacyDependency(handler.identityResolve) || handler.weComCorpID == "" {
		return 0, http.StatusServiceUnavailable
	}
	result, err := handler.identityResolve.Resolve(request.Context(), identityport.IDRef{
		Kind:      identityport.KindWeComExternalUserID,
		Scope:     "wecom-corp:" + handler.weComCorpID,
		Value:     query.ExternalUserID,
		Assurance: identityport.AssuranceVerified,
		Source:    "legacy-customer-profile-messages",
	})
	if err != nil || !validLegacyCustomerProfileTagsResolution(result) {
		return 0, http.StatusServiceUnavailable
	}
	switch result.Status {
	case identityport.ResolveFound:
		return result.CustomerID, 0
	case identityport.ResolveNotFound:
		return 0, http.StatusNotFound
	default:
		return 0, http.StatusConflict
	}
}

func legacyCustomerProfileMessages(page wecomport.CustomerChatSummaryPage, requestedLimit int32) ([]legacyCustomerProfileMessage, bool) {
	if page.Limit != requestedLimit || page.Offset != 0 || page.Total < 0 || page.Total < int64(len(page.Items)) || len(page.Items) > int(requestedLimit) {
		return nil, false
	}
	messages := make([]legacyCustomerProfileMessage, 0, len(page.Items))
	var previous time.Time
	for _, item := range page.Items {
		if (item.ChatType != "private" && item.ChatType != "group") || !validLegacyCustomerProfileMessageType(item.MessageType) || item.SentAt.IsZero() {
			return nil, false
		}
		sentAt := item.SentAt.UTC()
		if !previous.IsZero() && previous.Before(sentAt) {
			return nil, false
		}
		previous = sentAt
		messages = append(messages, legacyCustomerProfileMessage{
			ChatType:    item.ChatType,
			MessageType: item.MessageType,
			SendTime:    sentAt.Format(time.RFC3339),
		})
	}
	return messages, true
}

func validLegacyCustomerProfileMessageType(value string) bool {
	if value == "" || len(value) > 64 || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func legacyCustomerProfileMessagesAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin &&
		authorizationOK && authorization.Capability == authport.CapabilityAdminRead &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func writeLegacyCustomerProfileMessagesError(writer http.ResponseWriter, status int, code string) {
	switch status {
	case http.StatusForbidden:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeUnauthorized)
	case http.StatusNotFound:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeNotFound)
	case http.StatusConflict:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeConflict)
	case http.StatusUnprocessableEntity:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeValidationFailed)
	default:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	}
	writeLegacyCustomerProfileMessagesJSON(writer, status, legacyCustomerProfileMessagesError{
		OK:                       false,
		StatusCode:               status,
		ErrorCode:                code,
		RealExternalCallExecuted: false,
	})
}

func writeLegacyCustomerProfileMessagesNotFound(writer http.ResponseWriter) {
	writeLegacyCustomerProfileMessagesError(writer, http.StatusNotFound, "customer_not_found")
}

func writeLegacyCustomerProfileMessagesUnavailable(writer http.ResponseWriter) {
	writeLegacyCustomerProfileMessagesError(writer, http.StatusServiceUnavailable, "customer_profile_messages_unavailable")
}

func writeLegacyCustomerProfileMessagesMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	setLegacyCustomerProfileMessagesSecurityHeaders(writer)
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func legacyCustomerProfileMessagesSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(legacyCustomerProfileMessagesHeaderWriter{ResponseWriter: writer}, request)
	})
}

type legacyCustomerProfileMessagesHeaderWriter struct {
	http.ResponseWriter
}

func (writer legacyCustomerProfileMessagesHeaderWriter) WriteHeader(status int) {
	setLegacyCustomerProfileMessagesSecurityHeaders(writer.ResponseWriter)
	writer.ResponseWriter.WriteHeader(status)
}

func (writer legacyCustomerProfileMessagesHeaderWriter) Write(payload []byte) (int, error) {
	setLegacyCustomerProfileMessagesSecurityHeaders(writer.ResponseWriter)
	return writer.ResponseWriter.Write(payload)
}

func setLegacyCustomerProfileMessagesSecurityHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeLegacyCustomerProfileMessagesJSON(writer http.ResponseWriter, status int, value any) {
	writeJSON(legacyCustomerProfileMessagesHeaderWriter{ResponseWriter: writer}, status, value)
}
