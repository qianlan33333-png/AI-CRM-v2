// Package http exposes the closed Segment refresh-request HTTP boundary.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type refreshRequester interface {
	RequestRefresh(context.Context, segmentport.RefreshCommand) (segmentport.Segment, error)
}

type RefreshHandler struct {
	requester refreshRequester
}

func NewRefreshHandler(requester refreshRequester) (*RefreshHandler, error) {
	if nilRefreshRequester(requester) {
		return nil, errors.New("segment refresh requester is required")
	}
	return &RefreshHandler{requester: requester}, nil
}

func (handler *RefreshHandler) RequestSegmentRefresh(
	writer http.ResponseWriter,
	request *http.Request,
	segmentID generated.SegmentID,
	params generated.RequestSegmentRefreshParams,
) {
	principal, err := handler.operation(request)
	if err != nil {
		writeRefreshError(writer, request, err)
		return
	}
	if segmentID <= 0 {
		writeRefreshError(writer, request, segmentapp.ErrSegmentNotFound)
		return
	}
	accepted, err := handler.requester.RequestRefresh(request.Context(), segmentport.RefreshCommand{
		SegmentID:      segmentport.SegmentID(segmentID),
		Actor:          segmentport.Actor("admin:" + strconv.FormatInt(principal.AdminUserID, 10)),
		IdempotencyKey: string(params.IdempotencyKey),
	})
	if err != nil {
		writeRefreshError(writer, request, err)
		return
	}
	if accepted.ID != segmentport.SegmentID(segmentID) {
		writeRefreshError(writer, request, segmentapp.ErrRefreshRequestFailed)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(writer).Encode(generated.SegmentRefreshAccepted{
		SegmentId: int64(segmentID), Status: generated.Accepted,
	})
}

func (handler *RefreshHandler) operation(request *http.Request) (authport.Principal, error) {
	if handler == nil || nilRefreshRequester(handler.requester) || request == nil {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilitySegmentsWrite || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	return principal, nil
}

func writeRefreshError(writer http.ResponseWriter, request *http.Request, err error) {
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, segmentapp.ErrInvalidRefreshRequest):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, segmentapp.ErrSegmentNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, segmentapp.ErrRefreshCommandConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilRefreshRequester(requester refreshRequester) bool {
	if requester == nil {
		return true
	}
	value := reflect.ValueOf(requester)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
