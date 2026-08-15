// This adapter preserves the frozen archive routes over the local, redacted
// v2 projection. It never creates a WeCom provider client or dispatches work.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
)

const (
	archiveRouteOwner        = "ai_crm_v2"
	archiveProjectionStatus  = "local_projection"
	archiveExternalPageLimit = 20
	archiveDefaultStartTime  = "2000-01-01 00:00:00"
	archiveDefaultEndTime    = "2099-12-31 23:59:59"
	archiveDefaultSyncLimit  = 100
	archiveDefaultMaxPages   = 1000
)

var errInvalidLegacyMessageArchive = errors.New("legacy message archive request cannot be mapped safely")

type legacyMessageArchiveApplication interface {
	Health(context.Context) (wecomapp.ArchiveHealth, error)
	List(context.Context, wecomapp.ArchiveQuery) ([]wecomapp.ArchiveMessage, int64, error)
	RequestSync(context.Context, wecomapp.ArchiveSyncCommand) (wecomapp.ArchiveSyncReceipt, error)
}

// legacyMessageArchiveUnionResolver deliberately looks across no guessed
// UnionID scope. Identity owns the ambiguity policy and fails closed.
type legacyMessageArchiveUnionResolver interface {
	ResolveUnionID(context.Context, string) (identityport.ResolveResult, error)
}

type legacyArchiveSyncBody struct {
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	OwnerUserID string `json:"owner_userid"`
	Cursor      string `json:"cursor"`
	Limit       *int64 `json:"limit"`
	MaxPages    *int64 `json:"max_pages"`
}

func (handler *Handler) ArchiveHealth(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.messageArchive) || request == nil || !legacyArchiveAuthorized(request.Context(), authport.CapabilityMessageArchiveRead) {
		legacyArchiveError(writer, http.StatusServiceUnavailable, "archive_health_unavailable", "message archive health is unavailable", "archive_health")
		return
	}
	health, err := handler.messageArchive.Health(request.Context())
	if err != nil {
		legacyArchiveError(writer, http.StatusServiceUnavailable, "archive_health_failed", "message archive health is unavailable", "archive_health")
		return
	}
	payload := map[string]any{
		"ok": true, "record_count": health.RecordCount, "accepted_sync_count": health.AcceptedSyncCount,
		"route_owner": archiveRouteOwner, "source_status": "archive_health", "read_model_status": archiveProjectionStatus,
		"fallback_used": false, "real_external_call_executed": false,
	}
	if health.LastAcceptedAt != nil {
		payload["last_accepted_at"] = health.LastAcceptedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(writer, http.StatusOK, payload)
}

func (handler *Handler) RequestArchiveSync(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.messageArchive) || request == nil {
		legacyArchiveError(writer, http.StatusServiceUnavailable, "archive_sync_unavailable", "message archive sync is unavailable", "archive_sync")
		return
	}
	authorization, ok := legacyArchiveAuthorization(request.Context(), authport.CapabilityMessageArchiveExecute)
	if !ok {
		legacyArchiveError(writer, http.StatusForbidden, "archive_sync_forbidden", "message archive sync is forbidden", "archive_sync")
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 || authorization.Scope != authport.ScopeGlobal {
		legacyArchiveError(writer, http.StatusForbidden, "archive_sync_forbidden", "message archive sync is forbidden", "archive_sync")
		return
	}
	command, err := legacyArchiveSyncCommand(writer, request, principal.AdminUserID)
	if err != nil {
		legacyArchiveError(writer, http.StatusBadRequest, "invalid_request", "message archive sync request is invalid", "archive_sync")
		return
	}
	receipt, err := handler.messageArchive.RequestSync(request.Context(), command)
	if err != nil {
		status, code := http.StatusServiceUnavailable, "archive_sync_failed"
		if errors.Is(err, wecomapp.ErrInvalidArchiveSyncCommand) {
			status, code = http.StatusBadRequest, "invalid_request"
		} else if errors.Is(err, wecomapp.ErrArchiveSyncConflict) {
			status, code = http.StatusConflict, "archive_sync_idempotency_conflict"
		}
		legacyArchiveError(writer, status, code, "message archive sync was not accepted", "archive_sync")
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"ok": true, "status": string(receipt.State), "sync_receipt": map[string]any{
			"id": receipt.ID, "state": string(receipt.State), "accepted_event_id": int64(receipt.EventID),
		},
		"side_effect_executed": false, "provider_receipt": nil, "route_owner": archiveRouteOwner,
		"source_status": "archive_sync_accepted", "read_model_status": "not_applicable", "fallback_used": false,
		"real_external_call_executed": false,
	})
}

func (handler *Handler) ListExternalChatRecords(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.messageArchive) || request == nil || !legacyArchiveAuthorized(request.Context(), authport.CapabilityMessageArchiveExternalRead) {
		legacyArchiveError(writer, http.StatusServiceUnavailable, "external_chat_records_unavailable", "external chat records are unavailable", "external_chat_records")
		return
	}
	query, matchedBy, requestedExternalID, err := legacyExternalArchiveQuery(request)
	if err != nil {
		legacyArchiveError(writer, http.StatusBadRequest, "invalid_request", "external chat records request is invalid", "external_chat_records")
		return
	}
	customerID, err := handler.resolveArchiveCustomer(request.Context(), matchedBy, requestedExternalID, request.URL.Query().Get("unionid"), request.URL.Query().Get("mobile"))
	if err != nil {
		legacyArchiveReadError(writer, err, "external_chat_records")
		return
	}
	query.CustomerID = customerID
	records, total, err := handler.messageArchive.List(request.Context(), query)
	if err != nil {
		legacyArchiveReadError(writer, err, "external_chat_records")
		return
	}
	externalUserID := requestedExternalID
	if externalUserID == "" && len(records) > 0 {
		externalUserID = records[0].ExternalUserID
	}
	nextOffset := query.Offset + int32(len(records))
	var nextCursor string
	if int64(nextOffset) < total {
		nextCursor = legacyArchiveCursor(nextOffset)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "items": legacyArchiveMessages(records), "messages": legacyArchiveMessages(records), "total": total,
		"count": len(records), "limit": archiveExternalPageLimit, "next_cursor": nextCursor, "has_more": nextCursor != "",
		"external_userid": externalUserID, "matched_by": matchedBy,
		"filters": map[string]any{
			"chat_scene": query.ChatType, "start_time": query.StartedAt.UTC().Format("2006-01-02 15:04:05"), "with_userid": archiveMaskedIdentifier(query.WithUserID),
		},
		"route_owner": archiveRouteOwner, "source_status": "external_chat_records", "read_model_status": archiveProjectionStatus,
		"fallback_used": false,
	})
}

func (handler *Handler) SearchArchivedMessages(writer http.ResponseWriter, request *http.Request) {
	handler.listArchivedMessages(writer, request, true)
}

func (handler *Handler) ListArchivedMessages(writer http.ResponseWriter, request *http.Request) {
	handler.listArchivedMessages(writer, request, false)
}

func (handler *Handler) listArchivedMessages(writer http.ResponseWriter, request *http.Request, search bool) {
	if handler == nil || nilLegacyDependency(handler.messageArchive) || nilLegacyDependency(handler.customerDetail) || nilLegacyDependency(handler.identityResolve) || request == nil {
		legacyArchiveError(writer, http.StatusServiceUnavailable, "message_archive_unavailable", "message archive is unavailable", "message_archive")
		return
	}
	authorization, ok := legacyArchiveAuthorization(request.Context(), authport.CapabilityCustomersRead)
	if !ok {
		legacyArchiveError(writer, http.StatusForbidden, "message_archive_forbidden", "message archive is forbidden", "message_archive")
		return
	}
	query, externalUserID, err := legacyMessageArchiveQuery(request, search)
	if err != nil {
		legacyArchiveError(writer, http.StatusBadRequest, "invalid_request", "message archive request is invalid", "message_archive")
		return
	}
	resolved, err := handler.resolveArchiveExternalUserID(request.Context(), externalUserID)
	if err != nil {
		legacyArchiveReadError(writer, err, "message_archive")
		return
	}
	ownerStaffID, err := legacyOwnerScope(authorization, nil)
	if err != nil {
		legacyArchiveError(writer, http.StatusForbidden, "message_archive_forbidden", "message archive is forbidden", "message_archive")
		return
	}
	if _, err = handler.customerDetail.Get(request.Context(), contactapp.CustomerDetailInput{ID: resolved, OwnerStaffID: ownerStaffID}); err != nil {
		legacyArchiveReadError(writer, err, "message_archive")
		return
	}
	query.CustomerID = resolved
	records, _, err := handler.messageArchive.List(request.Context(), query)
	if err != nil {
		legacyArchiveReadError(writer, err, "message_archive")
		return
	}
	payload := map[string]any{
		"ok": true, "messages": legacyArchiveMessages(records), "items": legacyArchiveMessages(records), "count": len(records),
		"external_userid": externalUserID, "limit": query.Limit, "offset": query.Offset,
		"filters": map[string]any{"chat_type": query.ChatType}, "source_status": "message_archive_read_model",
		"read_model_status": archiveProjectionStatus, "fallback_used": false, "route_owner": archiveRouteOwner,
	}
	if search {
		payload["keyword"] = query.Keyword
	}
	writeJSON(writer, http.StatusOK, payload)
}

func (handler *Handler) DeprecatedMessageArchive(writer http.ResponseWriter, request *http.Request) {
	legacyDeprecatedArchiveRoute(writer, "/api/messages/search")
}

func (handler *Handler) DeprecatedExternalMessageArchive(writer http.ResponseWriter, request *http.Request) {
	legacyDeprecatedArchiveRoute(writer, "/api/messages/"+strings.TrimSpace(requestPathParameter(request, "external_userid")))
}

func (handler *Handler) DeprecatedExternalMessageHistory(writer http.ResponseWriter, request *http.Request) {
	legacyDeprecatedArchiveRoute(writer, "/api/messages/"+strings.TrimSpace(requestPathParameter(request, "external_userid")))
}

func legacyArchiveSyncCommand(writer http.ResponseWriter, request *http.Request, adminUserID int64) (wecomapp.ArchiveSyncCommand, error) {
	if request == nil || adminUserID < 1 {
		return wecomapp.ArchiveSyncCommand{}, errInvalidLegacyMessageArchive
	}
	key := request.Header.Get("Idempotency-Key")
	if key == "" || strings.TrimSpace(key) != key {
		return wecomapp.ArchiveSyncCommand{}, errInvalidLegacyMessageArchive
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	var body legacyArchiveSyncBody
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return wecomapp.ArchiveSyncCommand{}, errInvalidLegacyMessageArchive
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return wecomapp.ArchiveSyncCommand{}, errInvalidLegacyMessageArchive
	}
	limit, maxPages := int64(archiveDefaultSyncLimit), int64(archiveDefaultMaxPages)
	if body.Limit != nil {
		limit = *body.Limit
	}
	if body.MaxPages != nil {
		maxPages = *body.MaxPages
	}
	return wecomapp.ArchiveSyncCommand{
		Actor: "admin:" + strconv.FormatInt(adminUserID, 10), IdempotencyKey: key,
		StartTime: archiveTextDefault(body.StartTime, archiveDefaultStartTime), EndTime: archiveTextDefault(body.EndTime, archiveDefaultEndTime),
		OwnerUserID: strings.TrimSpace(body.OwnerUserID), Cursor: strings.TrimSpace(body.Cursor), Limit: limit, MaxPages: maxPages,
	}, nil
}

func legacyExternalArchiveQuery(request *http.Request) (wecomapp.ArchiveQuery, string, string, error) {
	if request == nil {
		return wecomapp.ArchiveQuery{}, "", "", errInvalidLegacyMessageArchive
	}
	values := request.URL.Query()
	if !legacyArchiveAllowedQuery(values, "mobile", "unionid", "external_userid", "start_time", "chat_scene", "with_userid", "cursor") {
		return wecomapp.ArchiveQuery{}, "", "", errInvalidLegacyMessageArchive
	}
	externalUserID, unionID, mobile := strings.TrimSpace(values.Get("external_userid")), strings.TrimSpace(values.Get("unionid")), strings.TrimSpace(values.Get("mobile"))
	matchedBy := ""
	switch {
	case externalUserID != "":
		matchedBy = "external_userid"
	case unionID != "":
		matchedBy = "unionid"
	case mobile != "":
		matchedBy = "mobile"
	default:
		return wecomapp.ArchiveQuery{}, "", "", errInvalidLegacyMessageArchive
	}
	startedAt, err := legacyArchiveStartTime(values.Get("start_time"))
	if err != nil {
		return wecomapp.ArchiveQuery{}, "", "", errInvalidLegacyMessageArchive
	}
	chatType, err := legacyArchiveChatType(values.Get("chat_scene"), true)
	if err != nil {
		return wecomapp.ArchiveQuery{}, "", "", errInvalidLegacyMessageArchive
	}
	offset, err := legacyArchiveCursorOffset(values.Get("cursor"))
	if err != nil {
		return wecomapp.ArchiveQuery{}, "", "", errInvalidLegacyMessageArchive
	}
	withUserID := strings.TrimSpace(values.Get("with_userid"))
	if withUserID == "" {
		withUserID = "HuangYouCan"
	}
	if chatType != "private" {
		withUserID = ""
	}
	return wecomapp.ArchiveQuery{ChatType: chatType, StartedAt: &startedAt, WithUserID: withUserID,
		Limit: archiveExternalPageLimit, Offset: offset, External: true}, matchedBy, externalUserID, nil
}

func legacyMessageArchiveQuery(request *http.Request, search bool) (wecomapp.ArchiveQuery, string, error) {
	if request == nil {
		return wecomapp.ArchiveQuery{}, "", errInvalidLegacyMessageArchive
	}
	values := request.URL.Query()
	allowed := []string{"external_userid", "chat_type", "limit", "offset"}
	if search {
		allowed = append(allowed, "keyword")
	}
	if !legacyArchiveAllowedQuery(values, allowed...) {
		return wecomapp.ArchiveQuery{}, "", errInvalidLegacyMessageArchive
	}
	externalUserID := strings.TrimSpace(values.Get("external_userid"))
	keyword := strings.TrimSpace(values.Get("keyword"))
	if externalUserID == "" || (search && keyword == "") {
		return wecomapp.ArchiveQuery{}, "", errInvalidLegacyMessageArchive
	}
	chatType, err := legacyArchiveChatType(values.Get("chat_type"), false)
	if err != nil {
		return wecomapp.ArchiveQuery{}, "", errInvalidLegacyMessageArchive
	}
	limit, err := legacyArchiveInt(values.Get("limit"), wecomapp.MessageArchiveDefaultLimit, 1, wecomapp.MessageArchiveMaximumLimit)
	if err != nil {
		return wecomapp.ArchiveQuery{}, "", errInvalidLegacyMessageArchive
	}
	offset, err := legacyArchiveInt(values.Get("offset"), 0, 0, int32(^uint32(0)>>1))
	if err != nil {
		return wecomapp.ArchiveQuery{}, "", errInvalidLegacyMessageArchive
	}
	return wecomapp.ArchiveQuery{ChatType: chatType, Keyword: keyword, Limit: limit, Offset: offset}, externalUserID, nil
}

func (handler *Handler) resolveArchiveCustomer(ctx context.Context, matchedBy, externalUserID, unionID, mobile string) (contactport.CustomerID, error) {
	if handler == nil || nilLegacyDependency(handler.customerDetail) || nilLegacyDependency(handler.identityResolve) {
		return 0, wecomapp.ErrMessageArchiveUnavailable
	}
	var result identityport.ResolveResult
	var err error
	switch matchedBy {
	case "external_userid":
		if handler.weComCorpID == "" {
			return 0, wecomapp.ErrMessageArchiveUnavailable
		}
		result, err = handler.identityResolve.Resolve(ctx, identityport.IDRef{Kind: identityport.KindWeComExternalUserID,
			Scope: "wecom-corp:" + handler.weComCorpID, Value: externalUserID, Assurance: identityport.AssuranceVerified, Source: "legacy-message-archive"})
	case "mobile":
		result, err = handler.identityResolve.Resolve(ctx, identityport.IDRef{Kind: identityport.KindPhone,
			Scope: "phone:e164", Value: mobile, Assurance: identityport.AssuranceVerified, Source: "legacy-message-archive"})
	case "unionid":
		if nilLegacyDependency(handler.messageArchiveUnionID) {
			return 0, wecomapp.ErrMessageArchiveUnavailable
		}
		result, err = handler.messageArchiveUnionID.ResolveUnionID(ctx, unionID)
	default:
		return 0, errInvalidLegacyMessageArchive
	}
	if err != nil {
		return 0, err
	}
	if result.Status != identityport.ResolveFound || result.CustomerID <= 0 {
		return 0, wecomapp.ErrMessageArchiveNotFound
	}
	if _, err = handler.customerDetail.Get(ctx, contactapp.CustomerDetailInput{ID: result.CustomerID}); err != nil {
		return 0, err
	}
	return result.CustomerID, nil
}

func (handler *Handler) resolveArchiveExternalUserID(ctx context.Context, externalUserID string) (contactport.CustomerID, error) {
	if handler == nil || nilLegacyDependency(handler.identityResolve) || handler.weComCorpID == "" || externalUserID == "" {
		return 0, wecomapp.ErrMessageArchiveUnavailable
	}
	result, err := handler.identityResolve.Resolve(ctx, identityport.IDRef{Kind: identityport.KindWeComExternalUserID,
		Scope: "wecom-corp:" + handler.weComCorpID, Value: externalUserID, Assurance: identityport.AssuranceVerified, Source: "legacy-message-archive"})
	if err != nil {
		return 0, err
	}
	if result.Status != identityport.ResolveFound || result.CustomerID <= 0 {
		return 0, wecomapp.ErrMessageArchiveNotFound
	}
	return result.CustomerID, nil
}

func legacyArchiveMessages(records []wecomapp.ArchiveMessage) []map[string]any {
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, map[string]any{
			"msgid": record.SourceMessageID, "chat_scene": record.ChatType, "chat_type": record.ChatType,
			"unionid": "", "external_userid": record.ExternalUserID, "with_userid": archiveMaskedIdentifier(record.WithUserID),
			"sender": archiveMaskedIdentifier(record.Sender), "receiver": archiveMaskedIdentifier(record.Receiver),
			"chat_id": archiveMaskedIdentifier(record.ChatID), "roomid": archiveMaskedIdentifier(record.RoomID),
			"group_name": archiveMaskedText(record.GroupName), "msgtype": record.MessageType, "content": archiveMaskedText(record.Content),
			"send_time": record.SentAt.UTC().Format(time.RFC3339), "source_id": record.ID,
		})
	}
	return items
}

func legacyArchiveAllowedQuery(values map[string][]string, allowed ...string) bool {
	known := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		known[key] = struct{}{}
	}
	for key, entries := range values {
		if _, ok := known[key]; !ok || len(entries) != 1 {
			return false
		}
	}
	return true
}

func legacyArchiveChatType(value string, required bool) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" && !required {
		return "", nil
	}
	switch normalized {
	case "private", "single", "私信":
		return "private", nil
	case "group", "群聊":
		return "group", nil
	default:
		return "", errInvalidLegacyMessageArchive
	}
}

func legacyArchiveStartTime(value string) (time.Time, error) {
	seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || seconds < 0 || seconds > 9_999_999_999 {
		return time.Time{}, errInvalidLegacyMessageArchive
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func legacyArchiveCursorOffset(value string) (int32, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	encoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return 0, errInvalidLegacyMessageArchive
	}
	var payload struct {
		Offset int64 `json:"offset"`
	}
	if err = json.Unmarshal(encoded, &payload); err != nil || payload.Offset < 0 || payload.Offset > int64(^uint32(0)>>1) {
		return 0, errInvalidLegacyMessageArchive
	}
	return int32(payload.Offset), nil
}

func legacyArchiveCursor(offset int32) string {
	payload, err := json.Marshal(map[string]int32{"offset": offset})
	if err != nil || offset < 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func legacyArchiveInt(value string, fallback, minimum, maximum int32) (int32, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil || parsed < int64(minimum) || parsed > int64(maximum) {
		return 0, errInvalidLegacyMessageArchive
	}
	return int32(parsed), nil
}

func legacyArchiveAuthorization(ctx context.Context, capability authport.Capability) (authport.Authorization, bool) {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	return authorization, ok && authorization.Capability == capability
}

func legacyArchiveAuthorized(ctx context.Context, capability authport.Capability) bool {
	_, ok := legacyArchiveAuthorization(ctx, capability)
	return ok
}

func legacyArchiveReadError(writer http.ResponseWriter, err error, source string) {
	status, code, message := http.StatusServiceUnavailable, "message_archive_unavailable", "message archive is unavailable"
	if errors.Is(err, wecomapp.ErrMessageArchiveNotFound) || errors.Is(err, contactapp.ErrCustomerNotFound) {
		status, code, message = http.StatusNotFound, "not_found", "message archive customer was not found"
	} else if errors.Is(err, wecomapp.ErrInvalidMessageArchiveQuery) || errors.Is(err, errInvalidLegacyMessageArchive) || errors.Is(err, contactapp.ErrInvalidCustomerDetailQuery) {
		status, code, message = http.StatusBadRequest, "invalid_request", "message archive request is invalid"
	}
	legacyArchiveError(writer, status, code, message, source)
}

func legacyArchiveError(writer http.ResponseWriter, status int, code, message, source string) {
	writeJSON(writer, status, map[string]any{
		"ok": false, "error_code": code, "message": message, "route_owner": archiveRouteOwner,
		"source_status": source, "fallback_used": false,
	})
}

func legacyDeprecatedArchiveRoute(writer http.ResponseWriter, replacementRoute string) {
	writeJSON(writer, http.StatusGone, map[string]any{
		"ok": false, "error_code": "messages_route_deprecated", "message": "This legacy messages route has been replaced by exact Next routes.",
		"replacement_route": replacementRoute, "route_owner": archiveRouteOwner, "source_status": "deprecated",
		"read_model_status": "not_applicable", "fallback_used": false,
	})
}

func archiveTextDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func archiveMaskedIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 2 {
		return "***"
	}
	return string(runes[:1]) + "***" + string(runes[len(runes)-1:])
}

func archiveMaskedText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) {
		return ""
	}
	var builder strings.Builder
	for index := 0; index < len(value); {
		end := index
		if value[index] == '+' {
			end++
		}
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		if digits := value[index:end]; archivePhoneLike(digits) {
			builder.WriteString("[masked-phone]")
			index = end
			continue
		}
		_, width := utf8.DecodeRuneInString(value[index:])
		builder.WriteString(value[index : index+width])
		index += width
	}
	return builder.String()
}

func archivePhoneLike(value string) bool {
	digits := strings.TrimPrefix(value, "+86")
	return len(digits) == 11 && digits[0] == '1' && digits[1] >= '3' && digits[1] <= '9'
}

func requestPathParameter(request *http.Request, key string) string {
	if request == nil {
		return ""
	}
	return chi.URLParam(request, key)
}
