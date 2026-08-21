package outboundhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	ExternalEffectsJobsPath        = "/api/admin/external-effects/jobs"
	ExternalEffectsDiagnosticsPath = "/api/admin/external-effects/diagnostics"

	maximumSafeJSONInteger int64 = 9_007_199_254_740_991
)

var (
	canonicalPositiveInteger = regexp.MustCompile(`^[1-9][0-9]{0,2}$`)
	jobIDPattern             = regexp.MustCompile(`^eej_v1_[A-Za-z0-9_-]{22}$`)
	cursorPattern            = regexp.MustCompile(`^eec_v1_[A-Za-z0-9_-]{24,1000}$`)
)

type ExternalEffectsApplication interface {
	ListJobs(context.Context, outboundapp.ExternalEffectJobQuery) (outboundapp.ExternalEffectJobPage, error)
	Diagnostics(context.Context) (outboundapp.ExternalEffectsDiagnostics, error)
}

type ExternalEffectsHandler struct {
	application ExternalEffectsApplication
}

func NewExternalEffectsHandler(application ExternalEffectsApplication) (*ExternalEffectsHandler, error) {
	if nilExternalEffectsApplication(application) {
		return nil, errors.New("external effects application is required")
	}
	return &ExternalEffectsHandler{application: application}, nil
}

// ServeHTTP is a leaf-only dispatcher. Central route registration remains a
// Lane E responsibility and is intentionally absent from this package.
func (handler *ExternalEffectsHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
		return
	}
	switch request.URL.Path {
	case ExternalEffectsJobsPath:
		handler.Jobs(writer, request)
	case ExternalEffectsDiagnosticsPath:
		handler.Diagnostics(writer, request)
	default:
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, nil))
	}
}

func (handler *ExternalEffectsHandler) Jobs(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
		return
	}
	if request.Method != http.MethodGet {
		writeExternalEffectsMethodNotAllowed(writer)
		return
	}
	if err := authorizeExternalEffectsRead(request.Context()); err != nil {
		writeExternalEffectsError(writer, request, err)
		return
	}
	if handler == nil || nilExternalEffectsApplication(handler.application) {
		writeExternalEffectsError(writer, request, outboundapp.ErrExternalEffectsUnavailable)
		return
	}
	query, err := parseExternalEffectsJobQuery(request.URL.RawQuery)
	if err != nil {
		writeExternalEffectsError(writer, request, err)
		return
	}
	page, err := handler.application.ListJobs(request.Context(), query)
	if err != nil {
		writeExternalEffectsError(writer, request, err)
		return
	}
	response, err := mapExternalEffectsJobPage(page, query)
	if err != nil {
		writeExternalEffectsError(writer, request, errors.Join(outboundapp.ErrExternalEffectsUnavailable, err))
		return
	}
	writeExternalEffectsJSON(writer, http.StatusOK, response)
}

func (handler *ExternalEffectsHandler) Diagnostics(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
		return
	}
	if request.Method != http.MethodGet {
		writeExternalEffectsMethodNotAllowed(writer)
		return
	}
	if err := authorizeExternalEffectsRead(request.Context()); err != nil {
		writeExternalEffectsError(writer, request, err)
		return
	}
	if request.URL.RawQuery != "" {
		writeExternalEffectsError(writer, request, outboundapp.ErrInvalidExternalEffectsQuery)
		return
	}
	if handler == nil || nilExternalEffectsApplication(handler.application) {
		writeExternalEffectsError(writer, request, outboundapp.ErrExternalEffectsUnavailable)
		return
	}
	diagnostics, err := handler.application.Diagnostics(request.Context())
	if err != nil {
		writeExternalEffectsError(writer, request, err)
		return
	}
	response, err := mapExternalEffectsDiagnostics(diagnostics)
	if err != nil {
		writeExternalEffectsError(writer, request, errors.Join(outboundapp.ErrExternalEffectsUnavailable, err))
		return
	}
	writeExternalEffectsJSON(writer, http.StatusOK, response)
}

func authorizeExternalEffectsRead(ctx context.Context) error {
	principal, principalOK := authport.PrincipalFromContext(ctx)
	if !principalOK || principal.AdminUserID < 1 {
		return platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	authorization, authorizationOK := authport.AuthorizationFromContext(ctx)
	if !authorizationOK || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) ||
		authorization.Capability != authport.CapabilityOperationsRead || authorization.Scope != authport.ScopeGlobal ||
		authorization.OwnerStaffID != 0 {
		return platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return nil
}

func parseExternalEffectsJobQuery(rawQuery string) (outboundapp.ExternalEffectJobQuery, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return outboundapp.ExternalEffectJobQuery{}, outboundapp.ErrInvalidExternalEffectsQuery
	}
	allowed := map[string]bool{"cursor": true, "status": true, "classification": true, "limit": true}
	for key, entries := range values {
		if !utf8.ValidString(key) || !allowed[key] || len(entries) != 1 || entries[0] == "" ||
			!utf8.ValidString(entries[0]) || strings.TrimSpace(entries[0]) != entries[0] {
			return outboundapp.ExternalEffectJobQuery{}, outboundapp.ErrInvalidExternalEffectsQuery
		}
	}
	query := outboundapp.ExternalEffectJobQuery{Limit: outboundapp.ExternalEffectsDefaultLimit}
	if value, ok := singleExternalEffectsValue(values, "cursor"); ok {
		if !cursorPattern.MatchString(value) {
			return outboundapp.ExternalEffectJobQuery{}, outboundapp.ErrInvalidExternalEffectsCursor
		}
		query.Cursor = value
	}
	if value, ok := singleExternalEffectsValue(values, "status"); ok {
		query.Status = outboundapp.TaskStatus(value)
		if !outboundapp.ExternalEffectStatusKnown(query.Status) {
			return outboundapp.ExternalEffectJobQuery{}, outboundapp.ErrInvalidExternalEffectsQuery
		}
	}
	if value, ok := singleExternalEffectsValue(values, "classification"); ok {
		query.Handling = outboundapp.ExternalEffectHandling(value)
		if !outboundapp.ExternalEffectHandlingKnown(query.Handling) {
			return outboundapp.ExternalEffectJobQuery{}, outboundapp.ErrInvalidExternalEffectsQuery
		}
	}
	if value, ok := singleExternalEffectsValue(values, "limit"); ok {
		if !canonicalPositiveInteger.MatchString(value) {
			return outboundapp.ExternalEffectJobQuery{}, outboundapp.ErrInvalidExternalEffectsQuery
		}
		parsed, parseErr := strconv.ParseInt(value, 10, 32)
		if parseErr != nil || parsed > int64(outboundapp.ExternalEffectsMaximumLimit) {
			return outboundapp.ExternalEffectJobQuery{}, outboundapp.ErrInvalidExternalEffectsQuery
		}
		query.Limit = int32(parsed)
	}
	if query.Status != "" && query.Handling != "" &&
		outboundapp.ExternalEffectHandlingForStatus(query.Status) != query.Handling {
		return outboundapp.ExternalEffectJobQuery{}, outboundapp.ErrInvalidExternalEffectsQuery
	}
	return query, nil
}

func singleExternalEffectsValue(values url.Values, key string) (string, bool) {
	entries, ok := values[key]
	if !ok || len(entries) != 1 {
		return "", false
	}
	return entries[0], true
}

type externalEffectJobResponse struct {
	ID              string                             `json:"id"`
	Status          outboundapp.TaskStatus             `json:"status"`
	Classification  outboundapp.ExternalEffectHandling `json:"classification"`
	AttemptCount    int32                              `json:"attempt_count"`
	CreatedAt       time.Time                          `json:"created_at"`
	StatusUpdatedAt time.Time                          `json:"status_updated_at"`
}

type externalEffectFiltersResponse struct {
	Status         *outboundapp.TaskStatus             `json:"status"`
	Classification *outboundapp.ExternalEffectHandling `json:"classification"`
}

type externalEffectsJobsResponse struct {
	OK                        bool                          `json:"ok"`
	Items                     []externalEffectJobResponse   `json:"items"`
	NextCursor                *string                       `json:"next_cursor"`
	PageSize                  int32                         `json:"page_size"`
	AppliedFilters            externalEffectFiltersResponse `json:"applied_filters"`
	ProviderExecutionEligible bool                          `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool                          `json:"real_external_call_executed"`
	DeliveryProven            bool                          `json:"delivery_proven"`
	LocalFactOnly             bool                          `json:"local_fact_only"`
	DeliverySemantics         string                        `json:"delivery_semantics"`
}

type externalEffectStatusCountsResponse struct {
	Pending         int64 `json:"pending"`
	Sending         int64 `json:"sending"`
	Sent            int64 `json:"sent"`
	RetryableFailed int64 `json:"retryable_failed"`
	FinalFailed     int64 `json:"final_failed"`
	OutcomeUnknown  int64 `json:"outcome_unknown"`
	Cancelled       int64 `json:"cancelled"`
}

type externalEffectClassificationCountsResponse struct {
	SafeLocalHandling int64 `json:"safe_local_handling"`
	Frozen            int64 `json:"frozen"`
	ManualReview      int64 `json:"manual_review"`
}

type externalEffectCountsResponse struct {
	Total            int64                                      `json:"total"`
	ByStatus         externalEffectStatusCountsResponse         `json:"by_status"`
	ByClassification externalEffectClassificationCountsResponse `json:"by_classification"`
}

type externalEffectRiskResponse struct {
	Level                outboundapp.ExternalEffectRiskLevel `json:"level"`
	OutcomeUnknownCount  int64                               `json:"outcome_unknown_count"`
	ManualReviewCount    int64                               `json:"manual_review_count"`
	ManualReviewRequired bool                                `json:"manual_review_required"`
}

type externalEffectsDiagnosticsResponse struct {
	OK                        bool                         `json:"ok"`
	Counts                    externalEffectCountsResponse `json:"counts"`
	RiskSummary               externalEffectRiskResponse   `json:"risk_summary"`
	GeneratedAt               time.Time                    `json:"generated_at"`
	ProviderExecutionEligible bool                         `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool                         `json:"real_external_call_executed"`
	DeliveryProven            bool                         `json:"delivery_proven"`
	LocalFactOnly             bool                         `json:"local_fact_only"`
	DeliverySemantics         string                       `json:"delivery_semantics"`
}

func mapExternalEffectsJobPage(
	page outboundapp.ExternalEffectJobPage,
	expected outboundapp.ExternalEffectJobQuery,
) (externalEffectsJobsResponse, error) {
	if page.PageSize < 1 || page.PageSize > outboundapp.ExternalEffectsMaximumLimit || page.PageSize != expected.Limit ||
		page.AppliedFilters.Status != expected.Status || page.AppliedFilters.Handling != expected.Handling ||
		len(page.Items) > int(page.PageSize) ||
		(page.NextCursor != nil && (len(page.Items) == 0 || !cursorPattern.MatchString(*page.NextCursor))) ||
		(page.AppliedFilters.Status != "" && !outboundapp.ExternalEffectStatusKnown(page.AppliedFilters.Status)) ||
		(page.AppliedFilters.Handling != "" && !outboundapp.ExternalEffectHandlingKnown(page.AppliedFilters.Handling)) ||
		(page.AppliedFilters.Status != "" && page.AppliedFilters.Handling != "" &&
			outboundapp.ExternalEffectHandlingForStatus(page.AppliedFilters.Status) != page.AppliedFilters.Handling) ||
		page.ProviderExecutionEligible || page.RealExternalCallExecuted || page.DeliveryProven || !page.LocalFactOnly ||
		page.DeliverySemantics != outboundapp.ExternalEffectsDeliverySemantics {
		return externalEffectsJobsResponse{}, errors.New("invalid external effects job page")
	}
	items := make([]externalEffectJobResponse, len(page.Items))
	seen := make(map[string]struct{}, len(page.Items))
	for index, item := range page.Items {
		if !validExternalEffectJob(item, page.AppliedFilters) {
			return externalEffectsJobsResponse{}, errors.New("invalid external effects job")
		}
		if _, exists := seen[item.ID]; exists {
			return externalEffectsJobsResponse{}, errors.New("duplicate external effects job")
		}
		seen[item.ID] = struct{}{}
		items[index] = externalEffectJobResponse{
			ID: item.ID, Status: item.Status, Classification: item.Handling, AttemptCount: item.AttemptCount,
			CreatedAt: item.CreatedAt.UTC(), StatusUpdatedAt: item.StatusUpdatedAt.UTC(),
		}
	}
	return externalEffectsJobsResponse{
		OK: true, Items: items, NextCursor: cloneExternalEffectsString(page.NextCursor), PageSize: page.PageSize,
		AppliedFilters: externalEffectFiltersResponse{
			Status:         cloneExternalEffectStatus(page.AppliedFilters.Status),
			Classification: cloneExternalEffectHandling(page.AppliedFilters.Handling),
		},
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
		LocalFactOnly: true, DeliverySemantics: outboundapp.ExternalEffectsDeliverySemantics,
	}, nil
}

func validExternalEffectJob(item outboundapp.ExternalEffectJob, filters outboundapp.ExternalEffectAppliedFilters) bool {
	if !externalEffectJobIDDecodes(item.ID) || !outboundapp.ExternalEffectStatusKnown(item.Status) ||
		item.Handling != outboundapp.ExternalEffectHandlingForStatus(item.Status) || item.AttemptCount < 0 ||
		!validExternalEffectHTTPTime(item.CreatedAt) || !validExternalEffectHTTPTime(item.StatusUpdatedAt) ||
		item.StatusUpdatedAt.Before(item.CreatedAt) ||
		(filters.Status != "" && item.Status != filters.Status) ||
		(filters.Handling != "" && item.Handling != filters.Handling) {
		return false
	}
	switch item.Status {
	case outboundapp.TaskStatusPending, outboundapp.TaskStatusCancelled:
		return item.AttemptCount == 0
	default:
		return item.AttemptCount > 0
	}
}

func mapExternalEffectsDiagnostics(
	diagnostics outboundapp.ExternalEffectsDiagnostics,
) (externalEffectsDiagnosticsResponse, error) {
	statusTotal, statusOK := checkedExternalEffectHTTPCountSum(
		diagnostics.ByStatus.Pending, diagnostics.ByStatus.Sending, diagnostics.ByStatus.Sent,
		diagnostics.ByStatus.RetryableFailed, diagnostics.ByStatus.FinalFailed,
		diagnostics.ByStatus.OutcomeUnknown, diagnostics.ByStatus.Cancelled,
	)
	expectedSafe, safeOK := checkedExternalEffectHTTPCountSum(
		diagnostics.ByStatus.Pending, diagnostics.ByStatus.RetryableFailed,
	)
	expectedFrozen, frozenOK := checkedExternalEffectHTTPCountSum(
		diagnostics.ByStatus.Sending, diagnostics.ByStatus.Sent, diagnostics.ByStatus.Cancelled,
	)
	expectedReview, reviewOK := checkedExternalEffectHTTPCountSum(
		diagnostics.ByStatus.FinalFailed, diagnostics.ByStatus.OutcomeUnknown,
	)
	classificationTotal, classificationOK := checkedExternalEffectHTTPCountSum(
		diagnostics.ByClassification.SafeLocalHandling, diagnostics.ByClassification.Frozen,
		diagnostics.ByClassification.ManualReview,
	)
	if !validExternalEffectCountRange(diagnostics.ByStatus) ||
		diagnostics.Total < 0 || diagnostics.Total > maximumSafeJSONInteger || !statusOK || !safeOK ||
		!frozenOK || !reviewOK || !classificationOK || diagnostics.Total != statusTotal ||
		diagnostics.ByClassification.SafeLocalHandling != expectedSafe ||
		diagnostics.ByClassification.Frozen != expectedFrozen ||
		diagnostics.ByClassification.ManualReview != expectedReview || classificationTotal != diagnostics.Total ||
		!validExternalEffectRisk(diagnostics) || !validExternalEffectHTTPTime(diagnostics.GeneratedAt) ||
		diagnostics.ProviderExecutionEligible || diagnostics.RealExternalCallExecuted ||
		diagnostics.DeliveryProven || !diagnostics.LocalFactOnly ||
		diagnostics.DeliverySemantics != outboundapp.ExternalEffectsDeliverySemantics {
		return externalEffectsDiagnosticsResponse{}, errors.New("invalid external effects diagnostics")
	}
	return externalEffectsDiagnosticsResponse{
		OK: true,
		Counts: externalEffectCountsResponse{
			Total: diagnostics.Total,
			ByStatus: externalEffectStatusCountsResponse{
				Pending: diagnostics.ByStatus.Pending, Sending: diagnostics.ByStatus.Sending, Sent: diagnostics.ByStatus.Sent,
				RetryableFailed: diagnostics.ByStatus.RetryableFailed, FinalFailed: diagnostics.ByStatus.FinalFailed,
				OutcomeUnknown: diagnostics.ByStatus.OutcomeUnknown, Cancelled: diagnostics.ByStatus.Cancelled,
			},
			ByClassification: externalEffectClassificationCountsResponse{
				SafeLocalHandling: diagnostics.ByClassification.SafeLocalHandling,
				Frozen:            diagnostics.ByClassification.Frozen, ManualReview: diagnostics.ByClassification.ManualReview,
			},
		},
		RiskSummary: externalEffectRiskResponse{
			Level: diagnostics.Risk.Level, OutcomeUnknownCount: diagnostics.Risk.OutcomeUnknownCount,
			ManualReviewCount:    diagnostics.Risk.ManualReviewCount,
			ManualReviewRequired: diagnostics.Risk.ManualReviewRequired,
		},
		GeneratedAt: diagnostics.GeneratedAt.UTC(), ProviderExecutionEligible: false,
		RealExternalCallExecuted: false, DeliveryProven: false, LocalFactOnly: true,
		DeliverySemantics: outboundapp.ExternalEffectsDeliverySemantics,
	}, nil
}

func checkedExternalEffectHTTPCountSum(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || value > maximumSafeJSONInteger || total > maximumSafeJSONInteger-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func validExternalEffectHTTPTime(value time.Time) bool {
	if value.IsZero() || value.Year() < 1 || value.Year() > 9999 {
		return false
	}
	_, err := value.UTC().MarshalJSON()
	return err == nil
}

func validExternalEffectCountRange(counts outboundapp.ExternalEffectStatusCounts) bool {
	for _, count := range []int64{
		counts.Pending, counts.Sending, counts.Sent, counts.RetryableFailed,
		counts.FinalFailed, counts.OutcomeUnknown, counts.Cancelled,
	} {
		if count < 0 || count > maximumSafeJSONInteger {
			return false
		}
	}
	return true
}

func validExternalEffectRisk(diagnostics outboundapp.ExternalEffectsDiagnostics) bool {
	risk := diagnostics.Risk
	if risk.OutcomeUnknownCount != diagnostics.ByStatus.OutcomeUnknown ||
		risk.ManualReviewCount != diagnostics.ByClassification.ManualReview ||
		risk.ManualReviewRequired != (risk.ManualReviewCount > 0) {
		return false
	}
	switch {
	case risk.OutcomeUnknownCount > 0:
		return risk.Level == outboundapp.ExternalEffectRiskOutcomeUnknownPresent
	case risk.ManualReviewCount > 0:
		return risk.Level == outboundapp.ExternalEffectRiskManualReviewRequired
	default:
		return risk.Level == outboundapp.ExternalEffectRiskNone
	}
}

func cloneExternalEffectsString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneExternalEffectStatus(value outboundapp.TaskStatus) *outboundapp.TaskStatus {
	if value == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func cloneExternalEffectHandling(value outboundapp.ExternalEffectHandling) *outboundapp.ExternalEffectHandling {
	if value == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func writeExternalEffectsJSON(writer http.ResponseWriter, status int, value any) {
	if writer == nil {
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeExternalEffectsMethodNotAllowed(writer http.ResponseWriter) {
	if writer == nil {
		return
	}
	writer.Header().Set("Allow", http.MethodGet)
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func writeExternalEffectsError(writer http.ResponseWriter, request *http.Request, err error) {
	if request == nil {
		return
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, outboundapp.ErrInvalidExternalEffectsCursor):
		code = platformhttp.CodeCursorInvalid
	case errors.Is(err, outboundapp.ErrInvalidExternalEffectsQuery):
		code = platformhttp.CodeMalformedRequest
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilExternalEffectsApplication(application ExternalEffectsApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func externalEffectJobIDDecodes(value string) bool {
	if !jobIDPattern.MatchString(value) {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(value, outboundapp.ExternalEffectJobIDPrefix))
	return err == nil && len(decoded) == 16
}
