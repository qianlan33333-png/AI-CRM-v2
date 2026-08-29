package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const ChannelAcquisitionAssetsRoutePrefix = "/api/admin/channels"

var channelAcquisitionAssetEffectID = regexp.MustCompile(`^eer_[1-9][0-9]{0,18}$`)

type channelAcquisitionAssetCommands interface {
	Publish(context.Context, contactapp.PublishChannelAcquisitionAssetCommand) (contactapp.ChannelAcquisitionAssetAcceptance, error)
	ReconcileCurrent(context.Context, contactapp.ReconcileCurrentChannelAcquisitionAssetCommand) (contactapp.ChannelAcquisitionAssetReconciliation, error)
}

type channelAcquisitionAssetQueries interface {
	Get(context.Context, int64, string) (contactapp.ChannelAcquisitionAssetItem, error)
	List(context.Context, contactapp.ChannelAcquisitionAssetListInput) (contactapp.ChannelAcquisitionAssetPage, error)
}

type ChannelAcquisitionAssetHandler struct {
	commands channelAcquisitionAssetCommands
	queries  channelAcquisitionAssetQueries
	csrf     channelAcquisitionCSRFValidator
}

func NewChannelAcquisitionAssetHandler(commands channelAcquisitionAssetCommands, queries channelAcquisitionAssetQueries, csrf channelAcquisitionCSRFValidator) (*ChannelAcquisitionAssetHandler, error) {
	if channelAcquisitionNil(commands) || channelAcquisitionNil(queries) || channelAcquisitionNil(csrf) {
		return nil, contactapp.ErrChannelAcquisitionAssetUnavailable
	}
	return &ChannelAcquisitionAssetHandler{commands: commands, queries: queries, csrf: csrf}, nil
}

func NewChannelAcquisitionAssetRouteFragment(handler *ChannelAcquisitionAssetHandler) (http.Handler, error) {
	if handler == nil || channelAcquisitionNil(handler.commands) || channelAcquisitionNil(handler.queries) || channelAcquisitionNil(handler.csrf) {
		return nil, contactapp.ErrChannelAcquisitionAssetUnavailable
	}
	return http.HandlerFunc(handler.route), nil
}

func NewDisabledChannelAcquisitionAssetRouteFragment() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		channelAcquisitionWriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelAcquisitionAssetUnavailable))
	})
}

func (handler *ChannelAcquisitionAssetHandler) route(writer http.ResponseWriter, request *http.Request) {
	channelAcquisitionSecurityHeaders(writer)
	if request == nil || request.URL == nil || request.URL.RawPath != "" || strings.HasSuffix(request.URL.Path, "/") || strings.Contains(request.URL.Path, "\\") {
		channelAcquisitionAssetWriteError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return
	}
	segments := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(segments) < 5 || segments[0] != "api" || segments[1] != "admin" || segments[2] != "channels" {
		channelAcquisitionAssetWriteError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return
	}
	switch {
	case len(segments) == 6 && segments[4] == "qrcode" && segments[5] == "generate" && request.Method == http.MethodPost:
		handler.publishQRCode(writer, request, segments[3])
	case segments[4] != "acquisition-assets":
		channelAcquisitionAssetWriteError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
	case len(segments) == 5 && request.Method == http.MethodPost:
		handler.publish(writer, request, segments[3])
	case len(segments) == 5 && request.Method == http.MethodGet:
		handler.list(writer, request, segments[3])
	case len(segments) == 6 && request.Method == http.MethodGet:
		handler.get(writer, request, segments[3], segments[5])
	case len(segments) == 7 && segments[6] == "reconcile" && request.Method == http.MethodPost:
		handler.reconcile(writer, request, segments[3], segments[5])
	default:
		channelAcquisitionAssetWriteError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
	}
}

// publishQRCode keeps the old generate endpoint as a thin CH02 adapter. It
// creates the same typed asset EER acceptance as the current asset endpoint;
// it never invents a second QR-code state machine.
func (handler *ChannelAcquisitionAssetHandler) publishQRCode(writer http.ResponseWriter, request *http.Request, rawChannelID string) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsWrite, true)
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelID, key, ok := channelAcquisitionAssetMutationIdentity(writer, request, rawChannelID)
	if !ok {
		return
	}
	object, err := channelAcquisitionDecodeObject(writer, request)
	if err != nil || len(object) != 0 {
		channelAcquisitionWriteValidation(writer, request, "body", contactapp.ErrInvalidChannelAcquisitionAsset)
		return
	}
	accepted, err := handler.commands.Publish(request.Context(), contactapp.PublishChannelAcquisitionAssetCommand{ChannelID: channelID, Actor: actor, IdempotencyKey: key, Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusAccepted, channelAcquisitionAssetAcceptanceResponse(accepted))
}

func (handler *ChannelAcquisitionAssetHandler) publish(writer http.ResponseWriter, request *http.Request, rawChannelID string) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsWrite, true)
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelID, key, ok := channelAcquisitionAssetMutationIdentity(writer, request, rawChannelID)
	if !ok {
		return
	}
	object, err := channelAcquisitionDecodeObject(writer, request)
	if err != nil || len(object) != 1 || object["kind"] == nil {
		channelAcquisitionWriteValidation(writer, request, "body", contactapp.ErrInvalidChannelAcquisitionAsset)
		return
	}
	var kind contactport.AcquisitionAssetKind
	if json.Unmarshal(object["kind"], &kind) != nil || kind != contactport.AcquisitionAssetQRCode && kind != contactport.AcquisitionAssetLink {
		channelAcquisitionWriteValidation(writer, request, "kind", contactapp.ErrInvalidChannelAcquisitionAsset)
		return
	}
	accepted, err := handler.commands.Publish(request.Context(), contactapp.PublishChannelAcquisitionAssetCommand{ChannelID: channelID, Actor: actor, IdempotencyKey: key, Kind: kind})
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusAccepted, channelAcquisitionAssetAcceptanceResponse(accepted))
}

func (handler *ChannelAcquisitionAssetHandler) list(writer http.ResponseWriter, request *http.Request, rawChannelID string) {
	if _, err := handler.authorize(request, authport.CapabilityChannelsRead, false); err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelID, err := channelAcquisitionID(rawChannelID)
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return
	}
	input, err := channelAcquisitionAssetListInput(request.URL, channelID)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "query", err)
		return
	}
	page, err := handler.queries.List(request.Context(), input)
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	response := channelAcquisitionAssetListResponse{Items: make([]channelAcquisitionAssetResponse, len(page.Items)), Limit: page.Limit, HasMore: page.HasMore, NextCursor: page.NextCursor}
	for index, item := range page.Items {
		response.Items[index] = channelAcquisitionAssetItemResponse(item)
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, response)
}

func (handler *ChannelAcquisitionAssetHandler) get(writer http.ResponseWriter, request *http.Request, rawChannelID, effectID string) {
	if _, err := handler.authorize(request, authport.CapabilityChannelsRead, false); err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelID, err := channelAcquisitionID(rawChannelID)
	if err != nil || !channelAcquisitionAssetValidEffectID(effectID) {
		channelAcquisitionAssetWriteError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return
	}
	item, err := handler.queries.Get(request.Context(), channelID, effectID)
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, channelAcquisitionAssetItemResponse(item))
}

func (handler *ChannelAcquisitionAssetHandler) reconcile(writer http.ResponseWriter, request *http.Request, rawChannelID, effectID string) {
	actor, err := handler.authorize(request, authport.CapabilityChannelsWrite, true)
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelID, key, ok := channelAcquisitionAssetMutationIdentity(writer, request, rawChannelID)
	if !ok || !channelAcquisitionAssetValidEffectID(effectID) {
		if ok {
			channelAcquisitionAssetWriteError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		}
		return
	}
	object, err := channelAcquisitionDecodeObject(writer, request)
	if err != nil || len(object) != 2 || object["resolution"] == nil || object["evidence_digest"] == nil {
		channelAcquisitionWriteValidation(writer, request, "body", contactapp.ErrInvalidChannelAcquisitionAsset)
		return
	}
	var resolution contactapp.ChannelAcquisitionAssetReconcileResolution
	var evidence eer.Digest
	if json.Unmarshal(object["resolution"], &resolution) != nil || json.Unmarshal(object["evidence_digest"], &evidence) != nil ||
		(resolution != contactapp.ChannelAcquisitionAssetProviderApplied && resolution != contactapp.ChannelAcquisitionAssetProviderNotApplied) {
		channelAcquisitionWriteValidation(writer, request, "body", contactapp.ErrInvalidChannelAcquisitionAsset)
		return
	}
	result, err := handler.commands.ReconcileCurrent(request.Context(), contactapp.ReconcileCurrentChannelAcquisitionAssetCommand{
		EffectID: effectID, ChannelID: channelID, Actor: actor, IdempotencyKey: key, EvidenceDigest: evidence, Resolution: resolution,
	})
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, err)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, channelAcquisitionAssetReconciliationResponse(result))
}

func (handler *ChannelAcquisitionAssetHandler) authorize(request *http.Request, capability authport.Capability, requiresCSRF bool) (int64, error) {
	if handler == nil || request == nil || channelAcquisitionNil(handler.csrf) {
		return 0, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelAcquisitionAssetUnavailable)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 || principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	if !requiresCSRF {
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

func channelAcquisitionAssetMutationIdentity(writer http.ResponseWriter, request *http.Request, rawChannelID string) (int64, string, bool) {
	channelID, err := channelAcquisitionID(rawChannelID)
	if err != nil {
		channelAcquisitionAssetWriteError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return 0, "", false
	}
	key, err := channelAcquisitionIdempotencyKey(request)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "Idempotency-Key", err)
		return 0, "", false
	}
	return channelID, key, true
}

func channelAcquisitionAssetValidEffectID(value string) bool {
	if !channelAcquisitionAssetEffectID.MatchString(value) {
		return false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "eer_"), 10, 64)
	return err == nil && id > 0
}

func channelAcquisitionAssetListInput(target *url.URL, channelID int64) (contactapp.ChannelAcquisitionAssetListInput, error) {
	input := contactapp.ChannelAcquisitionAssetListInput{ChannelID: channelID, Limit: contactapp.ChannelAcquisitionAssetDefaultLimit}
	if target == nil || len(target.RawQuery) > 1024 {
		return contactapp.ChannelAcquisitionAssetListInput{}, contactapp.ErrInvalidChannelAcquisitionAsset
	}
	values, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return contactapp.ChannelAcquisitionAssetListInput{}, err
	}
	for key, entries := range values {
		if (key != "limit" && key != "cursor") || len(entries) != 1 || entries[0] == "" {
			return contactapp.ChannelAcquisitionAssetListInput{}, contactapp.ErrInvalidChannelAcquisitionAsset
		}
		switch key {
		case "limit":
			limit, parseErr := strconv.Atoi(entries[0])
			if parseErr != nil || limit < 1 || limit > contactapp.ChannelAcquisitionAssetMaximumLimit {
				return contactapp.ChannelAcquisitionAssetListInput{}, contactapp.ErrInvalidChannelAcquisitionAsset
			}
			input.Limit = limit
		case "cursor":
			input.Cursor = entries[0]
		}
	}
	return input, nil
}

type channelAcquisitionAssetResponse struct {
	EffectID             string                           `json:"effect_id"`
	ChannelID            int64                            `json:"channel_id"`
	Kind                 contactport.AcquisitionAssetKind `json:"kind"`
	AssetVersion         int64                            `json:"asset_version"`
	SupersedesVersion    int64                            `json:"supersedes_version"`
	State                eer.State                        `json:"state"`
	AcceptReceiptID      string                           `json:"accept_receipt_id"`
	QueueReceiptID       string                           `json:"queue_receipt_id,omitempty"`
	AttemptReceiptDigest eer.Digest                       `json:"attempt_receipt_digest,omitempty"`
	ReconcileReceiptID   string                           `json:"reconcile_receipt_id,omitempty"`
	AssetURL             string                           `json:"asset_url,omitempty"`
	DownloadURL          string                           `json:"download_url,omitempty"`
	EntrantReady         bool                             `json:"entrant_ready"`
	CreatedAt            string                           `json:"created_at"`
	UpdatedAt            string                           `json:"updated_at"`
	ReconciledAt         string                           `json:"reconciled_at,omitempty"`
}

type channelAcquisitionAssetListResponse struct {
	Items      []channelAcquisitionAssetResponse `json:"items"`
	Limit      int                               `json:"limit"`
	HasMore    bool                              `json:"has_more"`
	NextCursor string                            `json:"next_cursor"`
}

type channelAcquisitionAssetAcceptance struct {
	EffectID          string                           `json:"effect_id"`
	ChannelID         int64                            `json:"channel_id"`
	Kind              contactport.AcquisitionAssetKind `json:"kind"`
	AssetVersion      int64                            `json:"asset_version"`
	SupersedesVersion int64                            `json:"supersedes_version"`
	State             eer.State                        `json:"state"`
	AcceptReceiptID   string                           `json:"accept_receipt_id"`
	QueueReceiptID    string                           `json:"queue_receipt_id"`
	EntrantReady      bool                             `json:"entrant_ready"`
}

type channelAcquisitionAssetReconciliation struct {
	EffectID     string                                                `json:"effect_id"`
	State        eer.State                                             `json:"state"`
	Resolution   contactapp.ChannelAcquisitionAssetReconcileResolution `json:"resolution"`
	ReceiptID    string                                                `json:"receipt_id"`
	Replacement  *channelAcquisitionAssetAcceptance                    `json:"replacement,omitempty"`
	EntrantReady bool                                                  `json:"entrant_ready"`
}

func channelAcquisitionAssetAcceptanceResponse(value contactapp.ChannelAcquisitionAssetAcceptance) channelAcquisitionAssetAcceptance {
	return channelAcquisitionAssetAcceptance{EffectID: value.EffectID, ChannelID: value.ChannelID, Kind: value.Kind, AssetVersion: value.AssetVersion, SupersedesVersion: value.SupersedesVersion, State: value.State, AcceptReceiptID: value.AcceptReceiptID, QueueReceiptID: value.QueueReceiptID, EntrantReady: value.EntrantReady}
}

func channelAcquisitionAssetReconciliationResponse(value contactapp.ChannelAcquisitionAssetReconciliation) channelAcquisitionAssetReconciliation {
	response := channelAcquisitionAssetReconciliation{EffectID: value.EffectID, State: value.State, Resolution: value.Resolution, ReceiptID: value.ReceiptID, EntrantReady: value.EntrantReady}
	if value.Replacement != nil {
		replacement := channelAcquisitionAssetAcceptanceResponse(*value.Replacement)
		response.Replacement = &replacement
	}
	return response
}

func channelAcquisitionAssetItemResponse(item contactapp.ChannelAcquisitionAssetItem) channelAcquisitionAssetResponse {
	response := channelAcquisitionAssetResponse{EffectID: item.EffectID, ChannelID: item.ChannelID, Kind: item.Kind, AssetVersion: item.AssetVersion, SupersedesVersion: item.SupersedesVersion, State: item.State, AcceptReceiptID: item.AcceptReceiptID, QueueReceiptID: item.QueueReceiptID, AttemptReceiptDigest: item.AttemptReceiptDigest, ReconcileReceiptID: item.ReconcileReceiptID, AssetURL: item.AssetURL, EntrantReady: item.EntrantReady, CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	if item.Kind == contactport.AcquisitionAssetQRCode && item.State == eer.StateExecuted && item.AssetURL != "" {
		response.DownloadURL = ChannelAcquisitionAssetsRoutePrefix + "/" + strconv.FormatInt(item.ChannelID, 10) + "/qrcode/download"
	}
	if item.ReconciledAt != nil {
		response.ReconciledAt = item.ReconciledAt.UTC().Format(time.RFC3339Nano)
	}
	return response
}

func channelAcquisitionAssetWriteError(writer http.ResponseWriter, request *http.Request, err error) {
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		channelAcquisitionWriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrInvalidChannelAcquisitionAsset):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, contactapp.ErrChannelAcquisitionAssetNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrChannelAcquisitionAssetConflict), errors.Is(err, contactapp.ErrChannelAcquisitionAssetReconcileRequired):
		code = platformhttp.CodeConflict
	}
	channelAcquisitionWriteError(writer, request, platformhttp.NewError(code, err))
}
