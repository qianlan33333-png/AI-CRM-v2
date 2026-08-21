package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const maximumMergeReviewBodyBytes = 1 << 20

type reviewApplication interface {
	ListMergeReviewsByStatus(context.Context, identityport.MergeReviewStatus, string, int32) (identityport.MergeReviewPage, error)
	ApproveMergeReview(context.Context, identityport.ApproveMergeReviewCommand) (identityport.MergeReview, error)
	RejectMergeReview(context.Context, identityport.RejectMergeReviewCommand) (identityport.MergeReview, error)
}

type ReviewHandler struct {
	application reviewApplication
}

func NewReviewHandler(application reviewApplication) (*ReviewHandler, error) {
	if nilReviewApplication(application) {
		return nil, identityapp.ErrMergeReviewUnavailable
	}
	return &ReviewHandler{application: application}, nil
}

func (handler *ReviewHandler) ListIdentityMergeReviews(
	writer http.ResponseWriter,
	request *http.Request,
	params generated.ListIdentityMergeReviewsParams,
) {
	if _, err := handler.operation(request, authport.CapabilityIdentityReviewRead); err != nil {
		writeReviewError(writer, request, err, params.Cursor != nil)
		return
	}
	status := identityport.MergeReviewPending
	if params.Status != nil {
		generatedStatus := *params.Status
		if !generatedStatus.Valid() {
			writeReviewError(writer, request, identityapp.ErrMergeReviewInvalid, false)
			return
		}
		status = identityport.MergeReviewStatus(generatedStatus)
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = string(*params.Cursor)
	}
	limit := int32(0)
	if params.Limit != nil {
		limit = int32(*params.Limit)
	}
	page, err := handler.application.ListMergeReviewsByStatus(request.Context(), status, cursor, limit)
	if err != nil {
		writeReviewError(writer, request, err, params.Cursor != nil)
		return
	}
	items := make([]generated.IdentityMergeReview, 0, len(page.Items))
	for _, review := range page.Items {
		converted, convertErr := mergeReviewResponse(review)
		if convertErr != nil || identityport.MergeReviewStatus(converted.Status) != status {
			writeReviewError(writer, request, identityapp.ErrMergeReviewUnavailable, false)
			return
		}
		items = append(items, converted)
	}
	var next *string
	if page.NextCursor != "" {
		value := page.NextCursor
		next = &value
	}
	writeReviewJSON(writer, http.StatusOK, generated.IdentityMergeReviewPage{Items: items, NextCursor: next})
}

func (handler *ReviewHandler) ApproveIdentityMergeReview(
	writer http.ResponseWriter,
	request *http.Request,
	reviewID generated.MergeReviewID,
	params generated.ApproveIdentityMergeReviewParams,
) {
	principal, err := handler.operation(request, authport.CapabilityIdentityReviewWrite)
	if err != nil {
		writeReviewError(writer, request, err, false)
		return
	}
	body := generated.ApproveIdentityMergeReviewRequest{}
	if err = decodeReviewBody(writer, request, &body); err != nil {
		writeReviewError(writer, request, err, false)
		return
	}
	review, err := handler.application.ApproveMergeReview(request.Context(), identityport.ApproveMergeReviewCommand{
		ReviewID: int64(reviewID), ExpectedVersion: body.ExpectedVersion,
		PrimaryCustomerID: contactport.CustomerID(body.PrimaryCustomerId), Reason: body.Reason,
		Actor: reviewActor(principal), IdempotencyKey: string(params.IdempotencyKey),
	})
	if err != nil {
		writeReviewError(writer, request, err, false)
		return
	}
	response, err := mergeReviewResponse(review)
	if err != nil || response.Status != generated.IdentityMergeReviewStatusApproved {
		writeReviewError(writer, request, identityapp.ErrMergeReviewUnavailable, false)
		return
	}
	writeReviewJSON(writer, http.StatusOK, response)
}

func (handler *ReviewHandler) RejectIdentityMergeReview(
	writer http.ResponseWriter,
	request *http.Request,
	reviewID generated.MergeReviewID,
	params generated.RejectIdentityMergeReviewParams,
) {
	principal, err := handler.operation(request, authport.CapabilityIdentityReviewWrite)
	if err != nil {
		writeReviewError(writer, request, err, false)
		return
	}
	body := generated.RejectIdentityMergeReviewRequest{}
	if err = decodeReviewBody(writer, request, &body); err != nil {
		writeReviewError(writer, request, err, false)
		return
	}
	review, err := handler.application.RejectMergeReview(request.Context(), identityport.RejectMergeReviewCommand{
		ReviewID: int64(reviewID), ExpectedVersion: body.ExpectedVersion, Reason: body.Reason,
		Actor: reviewActor(principal), IdempotencyKey: string(params.IdempotencyKey),
	})
	if err != nil {
		writeReviewError(writer, request, err, false)
		return
	}
	response, err := mergeReviewResponse(review)
	if err != nil || response.Status != generated.IdentityMergeReviewStatusRejected {
		writeReviewError(writer, request, identityapp.ErrMergeReviewUnavailable, false)
		return
	}
	writeReviewJSON(writer, http.StatusOK, response)
}

func (handler *ReviewHandler) operation(request *http.Request, capability authport.Capability) (authport.Principal, error) {
	if handler == nil || nilReviewApplication(handler.application) || request == nil {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrMergeReviewUnavailable)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID <= 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return principal, nil
}

func decodeReviewBody(writer http.ResponseWriter, request *http.Request, target any) error {
	if request == nil || request.Body == nil || target == nil {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, identityapp.ErrMergeReviewInvalid)
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maximumMergeReviewBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, err)
	}
	return nil
}

func mergeReviewResponse(review identityport.MergeReview) (generated.IdentityMergeReview, error) {
	if review.ReviewID <= 0 || review.Version <= 0 || review.CreatedAt.IsZero() ||
		len(review.CustomerIDs) != 2 || review.CustomerIDs[0] <= 0 || review.CustomerIDs[0] >= review.CustomerIDs[1] ||
		strings.TrimSpace(review.Scope) == "" {
		return generated.IdentityMergeReview{}, identityapp.ErrMergeReviewUnavailable
	}
	status := generated.IdentityMergeReviewStatus(review.Status)
	kind := generated.IdentityMergeReviewType(review.Kind)
	if !status.Valid() || !kind.Valid() ||
		(status == generated.IdentityMergeReviewStatusPending) != (review.ResolvedAt == nil) {
		return generated.IdentityMergeReview{}, identityapp.ErrMergeReviewUnavailable
	}
	if review.ResolvedAt != nil && (review.ResolvedAt.IsZero() || review.ResolvedAt.Before(review.CreatedAt)) {
		return generated.IdentityMergeReview{}, identityapp.ErrMergeReviewUnavailable
	}
	return generated.IdentityMergeReview{
		ReviewId: review.ReviewID, Status: status, Type: kind, Scope: review.Scope,
		CustomerIds: []int64{int64(review.CustomerIDs[0]), int64(review.CustomerIDs[1])},
		Version:     review.Version, CreatedAt: review.CreatedAt.UTC(), ResolvedAt: cloneReviewTime(review.ResolvedAt),
	}, nil
}

func reviewActor(principal authport.Principal) contactport.Actor {
	return contactport.Actor(fmt.Sprintf("admin:%d", principal.AdminUserID))
}

func writeReviewError(writer http.ResponseWriter, request *http.Request, err error, cursor bool) {
	if request == nil {
		return
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, identityapp.ErrMergeReviewInvalid):
		code = platformhttp.CodeValidationFailed
		if cursor {
			code = platformhttp.CodeCursorInvalid
		}
	case errors.Is(err, identityapp.ErrMergeReviewNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, identityapp.ErrMergeReviewConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func writeReviewJSON(writer http.ResponseWriter, status int, value any) {
	if writer == nil {
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func nilReviewApplication(application reviewApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func cloneReviewTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
