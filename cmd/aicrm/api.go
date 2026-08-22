package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/store"
	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	healthapi "github.com/qianlan33333-png/AI-CRM-v2/internal/api/generated"
	authapp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/app"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/http"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configstore "github.com/qianlan33333-png/AI-CRM-v2/internal/config/store"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contacthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/http"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponstore "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/store"
	customer360app "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/app"
	customer360http "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/http"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcstore "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/http"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	operationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	operationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store"
	opsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/ops/store"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/http"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	domainverification "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/domainverification"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/legacyhealth"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	producthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/http"
	serviceperiodhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/http/serviceperiod"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/product/membergrid"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
	pushcenterapp "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/app"
	pushcenterstore "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmenthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/http"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
	safeadminhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http/safeadmin"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomcallback "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/callback"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
)

var errInvalidAPIComponent = errors.New("invalid API component")

func runtimeConfigDeclarationFromConfig(config appconfig.Root) runtimeConfigDeclaration {
	releaseSHA := strings.TrimSpace(config.Release.SHA)
	if releaseSHA == "" {
		releaseSHA = "unknown"
	}
	callbackStatus := "missing"
	if config.WeCom.Callback.Enabled {
		callbackStatus = "configured"
	}
	oauthStatus := "missing"
	if config.WeCom.OAuth.Enabled {
		oauthStatus = "configured"
	}
	return runtimeConfigDeclaration{
		DatabaseMode:        "postgres",
		ProductionDataReady: "unknown",
		ReleaseSHA:          releaseSHA,
		WeChatCallbackToken: callbackStatus,
		WeChatPayConfig:     "unknown",
		OAuthConfig:         oauthStatus,
	}
}

type apiComponent struct {
	server  *http.Server
	pool    *pgxpool.Pool
	listen  func(string, string) (net.Listener, error)
	address string
}

type candidateHandler struct {
	*authhttp.Handler
	customers                 *contacthttp.CustomerListHandler
	customerDetail            *contacthttp.CustomerDetailHandler
	customerEvents            *contacthttp.CustomerEventHandler
	customerContext           *customer360http.CustomerContextHandler
	customerChatActivity      *customer360http.CustomerChatActivityHandler
	customerActivityAnalytics *contacthttp.CustomerActivityAnalyticsHandler
	customerMergeHistory      *identityhttp.MergeHistoryHandler
	mutations                 *contacthttp.CustomerMutationHandler
	tags                      *contacthttp.TagCatalogHandler
	localTags                 *contacthttp.LocalTagCatalogHandler
	stages                    *contacthttp.Handler
	segments                  *segmenthttp.CRUDHandler
	products                  *producthttp.Handler
	productLocal              *producthttp.LocalMutationHandler
	surveyPublic              *surveyhttp.PublicHandler
	segmentRefresh            *segmenthttp.RefreshHandler
	identityReviews           *identityhttp.ReviewHandler
	identityConsole           *identityhttp.ConsoleHandler
	automationRuns            interface {
		List(context.Context, automationstore.TriggerListInput) (automationstore.TriggerListResult, error)
	}
	domainVerification interface {
		Read(string) (string, error)
	}
	legacyHealth *legacyhealth.Handler
}

type identityConsoleApplication struct {
	resolver *identityapp.ResolveService
	binder   *identityapp.BindService
}

func (application identityConsoleApplication) Resolve(ctx context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	return application.resolver.Resolve(ctx, ref)
}

func (application identityConsoleApplication) Bind(ctx context.Context, command identityport.BindCommand) (identityport.BindResult, error) {
	return application.binder.Bind(ctx, command)
}

var _ api.ServerInterface = (*candidateHandler)(nil)

func (handler *candidateHandler) ListCustomers(writer http.ResponseWriter, request *http.Request, params api.ListCustomersParams) {
	handler.customers.ListCustomers(writer, request, params)
}

func (handler *candidateHandler) GetCustomer(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID) {
	handler.customerDetail.GetCustomer(writer, request, customerID)
}

func (handler *candidateHandler) ListCustomerEvents(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.ListCustomerEventsParams) {
	handler.customerEvents.ListCustomerEvents(writer, request, customerID, params)
}

func (handler *candidateHandler) GetCustomerContext(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.GetCustomerContextParams) {
	handler.customerContext.GetCustomerContext(writer, request, customerID, params)
}

func (handler *candidateHandler) ListCustomerMergeHistory(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.ListCustomerMergeHistoryParams) {
	query := identityhttp.CustomerMergeHistoryQuery{}
	if params.Cursor != nil {
		query.Cursor = string(*params.Cursor)
	}
	if params.Limit != nil {
		query.Limit = *params.Limit
	}
	handler.customerMergeHistory.GetCustomerMergeHistory(writer, request, contactport.CustomerID(customerID), query)
}

func (handler *candidateHandler) ListCustomerChatActivity(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.ListCustomerChatActivityParams) {
	query := customer360http.CustomerChatActivityQuery{}
	if params.ChatType != nil {
		query.ChatType = string(*params.ChatType)
	}
	if params.Cursor != nil {
		query.Cursor = string(*params.Cursor)
		query.CursorSupplied = true
	}
	if params.Limit != nil {
		query.Limit = *params.Limit
		query.LimitSupplied = true
	}
	handler.customerChatActivity.GetCustomerChatActivity(writer, request, contactport.CustomerID(customerID), query)
}

func (handler *candidateHandler) GetCustomerActivityAnalytics(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.GetCustomerActivityAnalyticsParams) {
	handlerParams := contacthttp.CustomerActivityAnalyticsParams{}
	if params.WindowDays != nil {
		windowDays := int(*params.WindowDays)
		handlerParams.WindowDays = &windowDays
	}
	handler.customerActivityAnalytics.GetCustomerActivityAnalytics(writer, request, int64(customerID), handlerParams)
}

func (handler *candidateHandler) ListTags(writer http.ResponseWriter, request *http.Request) {
	handler.tags.ListTags(writer, request)
}

func (handler *candidateHandler) ListTagGroups(writer http.ResponseWriter, request *http.Request) {
	handler.localTags.ListTagGroups(writer, request)
}

func (handler *candidateHandler) CreateTagGroup(writer http.ResponseWriter, request *http.Request, params api.CreateTagGroupParams) {
	handler.localTags.CreateTagGroup(writer, request, params)
}

func (handler *candidateHandler) UpdateTagGroup(writer http.ResponseWriter, request *http.Request, groupID api.TagGroupID, params api.UpdateTagGroupParams) {
	handler.localTags.UpdateTagGroup(writer, request, groupID, params)
}

func (handler *candidateHandler) ArchiveTagGroup(writer http.ResponseWriter, request *http.Request, groupID api.TagGroupID, params api.ArchiveTagGroupParams) {
	handler.localTags.ArchiveTagGroup(writer, request, groupID, params)
}

func (handler *candidateHandler) ReorderTagGroups(writer http.ResponseWriter, request *http.Request, params api.ReorderTagGroupsParams) {
	handler.localTags.ReorderTagGroups(writer, request, params)
}

func (handler *candidateHandler) CreateTag(writer http.ResponseWriter, request *http.Request, params api.CreateTagParams) {
	handler.localTags.CreateTag(writer, request, params)
}

func (handler *candidateHandler) UpdateTag(writer http.ResponseWriter, request *http.Request, tagID api.TagID, params api.UpdateTagParams) {
	handler.localTags.UpdateTag(writer, request, tagID, params)
}

func (handler *candidateHandler) ArchiveTag(writer http.ResponseWriter, request *http.Request, tagID api.TagID, params api.ArchiveTagParams) {
	handler.localTags.ArchiveTag(writer, request, tagID, params)
}

func (handler *candidateHandler) ReorderTags(writer http.ResponseWriter, request *http.Request, params api.ReorderTagsParams) {
	handler.localTags.ReorderTags(writer, request, params)
}

func (handler *candidateHandler) UpdateCustomer(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.UpdateCustomerParams) {
	handler.mutations.UpdateCustomer(writer, request, customerID, params)
}

func (handler *candidateHandler) SetCustomerStage(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.SetCustomerStageParams) {
	handler.mutations.SetCustomerStage(writer, request, customerID, params)
}

func (handler *candidateHandler) AddCustomerTag(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, tagID api.TagID, params api.AddCustomerTagParams) {
	handler.mutations.AddCustomerTag(writer, request, customerID, tagID, params)
}

func (handler *candidateHandler) RemoveCustomerTag(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, tagID api.TagID, params api.RemoveCustomerTagParams) {
	handler.mutations.RemoveCustomerTag(writer, request, customerID, tagID, params)
}

func (handler *candidateHandler) ListStages(writer http.ResponseWriter, request *http.Request) {
	handler.stages.ListStages(writer, request)
}

func (handler *candidateHandler) CreateStage(writer http.ResponseWriter, request *http.Request, params api.CreateStageParams) {
	handler.stages.CreateStage(writer, request, params)
}

func (handler *candidateHandler) ReorderStages(writer http.ResponseWriter, request *http.Request, params api.ReorderStagesParams) {
	handler.stages.ReorderStages(writer, request, params)
}

func (handler *candidateHandler) ArchiveStage(writer http.ResponseWriter, request *http.Request, stageID api.StageID, params api.ArchiveStageParams) {
	handler.stages.ArchiveStage(writer, request, stageID, params)
}

func (handler *candidateHandler) RenameStage(writer http.ResponseWriter, request *http.Request, stageID api.StageID, params api.RenameStageParams) {
	handler.stages.RenameStage(writer, request, stageID, params)
}

func (handler *candidateHandler) RequestSegmentRefresh(writer http.ResponseWriter, request *http.Request, segmentID api.SegmentID, params api.RequestSegmentRefreshParams) {
	handler.segmentRefresh.RequestSegmentRefresh(writer, request, segmentID, params)
}

func (handler *candidateHandler) ListSegments(writer http.ResponseWriter, request *http.Request, params api.ListSegmentsParams) {
	handler.segments.ListSegments(writer, request, params)
}

func (handler *candidateHandler) GetSegment(writer http.ResponseWriter, request *http.Request, segmentID api.SegmentID) {
	handler.segments.GetSegment(writer, request, segmentID)
}

func (handler *candidateHandler) ArchiveSegment(writer http.ResponseWriter, request *http.Request, segmentID api.SegmentID, params api.ArchiveSegmentParams) {
	handler.segments.ArchiveSegment(writer, request, segmentID, params)
}

func (handler *candidateHandler) ListProducts(writer http.ResponseWriter, request *http.Request, params api.ListProductsParams) {
	handler.products.ListProducts(writer, request, params)
}

func (handler *candidateHandler) CreateProduct(writer http.ResponseWriter, request *http.Request, params api.CreateProductParams) {
	handler.products.CreateProduct(writer, request, params)
}

func (handler *candidateHandler) GetProduct(writer http.ResponseWriter, request *http.Request, productID api.ProductID) {
	handler.products.GetProduct(writer, request, productID)
}

func (handler *candidateHandler) UpdateProduct(writer http.ResponseWriter, request *http.Request, productID api.ProductID, _ api.UpdateProductParams) {
	handler.productLocal.UpdateProduct(writer, request, int64(productID))
}

func (handler *candidateHandler) ListProductLocalEntitlements(writer http.ResponseWriter, request *http.Request, productID api.ProductID, params api.ListProductLocalEntitlementsParams) {
	var limit int32
	if params.Limit != nil {
		limit = int32(*params.Limit)
	}
	handler.productLocal.ListProductLocalEntitlements(writer, request, int64(productID), limit)
}

func (handler *candidateHandler) GrantProductLocalEntitlement(writer http.ResponseWriter, request *http.Request, productID api.ProductID, _ api.GrantProductLocalEntitlementParams) {
	handler.productLocal.GrantProductLocalEntitlement(writer, request, int64(productID))
}

func (handler *candidateHandler) GetProductLocalEntitlement(writer http.ResponseWriter, request *http.Request, entitlementID api.EntitlementID) {
	handler.productLocal.GetProductLocalEntitlement(writer, request, int64(entitlementID))
}

func (handler *candidateHandler) RevokeProductLocalEntitlement(writer http.ResponseWriter, request *http.Request, entitlementID api.EntitlementID, _ api.RevokeProductLocalEntitlementParams) {
	handler.productLocal.RevokeProductLocalEntitlement(writer, request, int64(entitlementID))
}

func (handler *candidateHandler) CreateSegment(writer http.ResponseWriter, request *http.Request, params api.CreateSegmentParams) {
	handler.segments.CreateSegment(writer, request, params)
}

func (handler *candidateHandler) UpdateSegment(writer http.ResponseWriter, request *http.Request, segmentID api.SegmentID, params api.UpdateSegmentParams) {
	handler.segments.UpdateSegment(writer, request, segmentID, params)
}

func (handler *candidateHandler) ListSegmentMembers(writer http.ResponseWriter, request *http.Request, segmentID api.SegmentID, params api.ListSegmentMembersParams) {
	handler.segments.ListSegmentMembers(writer, request, segmentID, params)
}

func (handler *candidateHandler) ListIdentityMergeReviews(writer http.ResponseWriter, request *http.Request, params api.ListIdentityMergeReviewsParams) {
	handler.identityReviews.ListIdentityMergeReviews(writer, request, params)
}

func (handler *candidateHandler) ApproveIdentityMergeReview(writer http.ResponseWriter, request *http.Request, reviewID api.MergeReviewID, params api.ApproveIdentityMergeReviewParams) {
	handler.identityReviews.ApproveIdentityMergeReview(writer, request, reviewID, params)
}

func (handler *candidateHandler) RejectIdentityMergeReview(writer http.ResponseWriter, request *http.Request, reviewID api.MergeReviewID, params api.RejectIdentityMergeReviewParams) {
	handler.identityReviews.RejectIdentityMergeReview(writer, request, reviewID, params)
}

func (handler *candidateHandler) ResolveIdentity(writer http.ResponseWriter, request *http.Request) {
	handler.identityConsole.ResolveIdentity(writer, request)
}

func (handler *candidateHandler) BindIdentity(writer http.ResponseWriter, request *http.Request, params api.BindIdentityParams) {
	handler.identityConsole.BindIdentity(writer, request, params)
}

func (handler *candidateHandler) ListAutomationTriggerRuns(writer http.ResponseWriter, request *http.Request, params api.ListAutomationTriggerRunsParams) {
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityConfigOverviewRead || authorization.Scope != authport.ScopeGlobal || handler.automationRuns == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	input, empty, err := automationTriggerListInput(params)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	result := automationstore.TriggerListResult{}
	if !empty {
		result, err = handler.automationRuns.List(request.Context(), input)
		if err != nil {
			code := platformhttp.CodeDependencyUnavailable
			if errors.Is(err, automationstore.ErrInvalidTagTrigger) {
				code = platformhttp.CodeMalformedRequest
			}
			platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
			return
		}
	}
	items := make([]api.AutomationTriggerRun, 0, len(result.Items))
	for _, receipt := range result.Items {
		items = append(items, api.AutomationTriggerRun{
			RunId:     "automation-trigger:" + strconv.FormatInt(receipt.ID, 10),
			RequestId: "event:" + strconv.FormatInt(int64(receipt.EventID), 10),
			AgentCode: api.TagTriggerV1, RunStatus: api.Completed,
			TriggerSource: api.CustomerTagApplied, CustomerId: int64(receipt.CustomerID), TagId: receipt.TagID,
			SourceEventId: int64(receipt.EventID), TriggeredEventId: int64(receipt.TriggeredEventID),
			StartedAt: receipt.TriggeredAt, CompletedAt: receipt.CompletedAt, HasError: false,
		})
	}
	writeJSON(writer, http.StatusOK, api.AutomationTriggerRunListResponse{
		Items: items, Total: result.Total, Page: input.Page, PageSize: input.PageSize,
		Visibility: api.AutomationTriggerRunListResponseVisibilityMasked,
	})
}

func automationTriggerListInput(params api.ListAutomationTriggerRunsParams) (automationstore.TriggerListInput, bool, error) {
	input := automationstore.TriggerListInput{Page: 1, PageSize: 50, StartedAfter: params.StartedAfter, StartedBefore: params.StartedBefore}
	if params.Page != nil {
		input.Page = *params.Page
	}
	if params.PageSize != nil {
		input.PageSize = *params.PageSize
	}
	if nonempty(params.Unionid) || nonempty(params.Userid) {
		return input, false, automationstore.ErrInvalidTagTrigger
	}
	empty := (params.AgentCode != nil && *params.AgentCode != "" && *params.AgentCode != string(api.TagTriggerV1)) ||
		(params.RunStatus != nil && *params.RunStatus != "" && *params.RunStatus != string(api.Completed)) ||
		(params.TriggerSource != nil && *params.TriggerSource != "" && *params.TriggerSource != string(api.CustomerTagApplied)) ||
		(params.HasError != nil && *params.HasError)
	var err error
	if params.RunId != nil && *params.RunId != "" {
		input.ReceiptID, err = parseLegacyRunID(*params.RunId, "automation-trigger:")
	}
	if err == nil && params.RequestId != nil && *params.RequestId != "" {
		input.EventID, err = parseLegacyRunID(*params.RequestId, "event:")
	}
	return input, empty, err
}

func parseLegacyRunID(value, prefix string) (*int64, error) {
	if !strings.HasPrefix(value, prefix) {
		return nil, automationstore.ErrInvalidTagTrigger
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil || id <= 0 {
		return nil, automationstore.ErrInvalidTagTrigger
	}
	return &id, nil
}

func nonempty(value *string) bool { return value != nil && strings.TrimSpace(*value) != "" }

func (handler *candidateHandler) GetDomainVerificationFile(writer http.ResponseWriter, request *http.Request, filename string) {
	if handler == nil || handler.domainVerification == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, nil))
		return
	}
	content, err := handler.domainVerification.Read(filename)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, nil))
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = writer.Write([]byte(content))
}

// GetLegacyHealth serves the public LEGACY-API-0757 runtime-mode snapshot.
// The frozen legacy handler owns the exact 405 method guard, so the route
// stays mounted for every method outside all authentication middleware.
func (handler *candidateHandler) GetLegacyHealth(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.legacyHealth == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, nil))
		return
	}
	handler.legacyHealth.ServeHTTP(writer, request)
}

func newAPIComponent(config appconfig.Root) (appruntime.Component, error) {
	poolConfig, err := pgxpool.ParseConfig(config.Database.URL.Value())
	if err != nil || poolConfig.ConnConfig.DescriptionCacheCapacity < 1 || config.API.PoolMaxConns < 1 || config.API.ListenAddress == "" {
		return nil, errInvalidAPIComponent
	}
	poolConfig.MaxConns = config.API.PoolMaxConns
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, errInvalidAPIComponent
	}
	uow := platformstore.NewUnitOfWork(pool)
	deliveryProducer, err := eventstore.NewProducerDeliveryRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	service, err := authapp.NewService(uow, authstore.NewRepository(), authapp.Options{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	oauthStates, err := authapp.NewOAuthStateService(uow, authstore.NewRepository(), authapp.OAuthStateOptions{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	var humanOAuth *wecomclient.HumanOAuthClient
	if config.WeCom.OAuth.Enabled {
		credentials, credentialErr := wecomclient.NewCredentials(config.WeCom.OAuth.CorpID, config.WeCom.OAuth.Secret.Value())
		if credentialErr != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
		providerHTTP := &http.Client{Timeout: 5 * time.Second}
		tokenProvider, tokenErr := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
			BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: providerHTTP, Now: time.Now,
		})
		if tokenErr != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
		humanOAuth, err = wecomclient.NewHumanOAuthClient(wecomclient.HumanOAuthConfig{
			BaseURL: wecomclient.ProductionBaseURL, AuthorizeURL: wecomclient.ProductionAuthorizeURL,
			CallbackURL: config.WeCom.OAuth.CallbackURL, CorpID: wecomclient.CorpID(config.WeCom.OAuth.CorpID),
			HTTPClient: providerHTTP, TokenProvider: tokenProvider,
		})
		if err != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
	}
	humanAuth, err := NewHumanAuthHandler(service, service, oauthStates, humanOAuth, HumanAuthOptions{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		pool.Close()
		return nil, err
	}
	stageHandler, err := contacthttp.NewHandler(contactapp.NewStageService(
		uow, contactstore.NewRepository(), eventstore.NewAppender(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	customerService := contactapp.NewCustomerListService(uow, contactstore.NewCustomerQueryRepository())
	customerHandler, err := contacthttp.NewCustomerListHandler(customerService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	customerDetailHandler, err := contacthttp.NewCustomerDetailHandler(contactapp.NewCustomerDetailService(
		uow, contactstore.NewCustomerDetailRepository(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	mutationHandler, err := contacthttp.NewCustomerMutationHandler(contactapp.NewCustomerMutationService(
		uow, contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(), deliveryProducer,
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	customerEventHandler, err := contacthttp.NewCustomerEventHandler(contactapp.NewCustomerEventService(
		uow, contactstore.NewCustomerEventRepository(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	messageArchiveService := wecomapp.NewMessageArchiveService(uow, wecomstore.NewMessageArchiveRepository(), eventstore.NewAppender())
	customerContextService := customer360app.NewCustomerContextService(
		contactapp.NewCustomer360ReaderService(
			contactapp.NewCustomerDetailService(uow, contactstore.NewCustomerDetailRepository()),
			contactapp.NewCustomerEventService(uow, contactstore.NewCustomerEventRepository()),
		),
		messageArchiveService,
	)
	customerContextHandler, err := customer360http.NewCustomerContextHandler(customerContextService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	customerChatActivityHandler, err := customer360http.NewCustomerChatActivityHandler(customerContextService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	customerActivityAnalyticsHandler, err := contacthttp.NewCustomerActivityAnalyticsHandler(contactapp.NewCustomerActivityAnalyticsService(
		uow, contactstore.NewCustomerActivityAnalyticsRepository(), time.Now,
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	tagCatalogHandler, err := contacthttp.NewTagCatalogHandler(contactapp.NewTagCatalogService(
		uow, contactstore.NewTagCatalogRepository(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	segmentRefreshRepository, err := segmentstore.NewRefreshRequestRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	segmentRefreshHandler, err := segmenthttp.NewRefreshHandler(segmentapp.NewRefreshRequestService(
		uow, segmentRefreshRepository, segmentRefreshRepository,
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	segmentCRUDHandler, err := segmenthttp.NewCRUDHandler(segmentapp.NewCRUDService(
		uow, segmentstore.NewCRUDRepository(), eventstore.NewAppender(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	productService := productapp.NewService(uow, productstore.NewCatalogRepository(), eventstore.NewAppender())
	mediaRepository := mediastore.NewUploadRepository()
	mediaService := mediaapp.NewService(uow, mediaRepository, eventstore.NewAppender())
	imageDeleteService := mediaapp.NewImageDeleteService(uow, mediaRepository, automationstore.NewAgentRepository(), contactstore.NewChannelRepository(), eventstore.NewAppender())
	groupInviteRepository := mediastore.NewGroupInviteRepository()
	groupInviteService := mediaapp.NewGroupInviteService(uow, groupInviteRepository, groupInviteRepository, eventstore.NewAppender())
	miniProgramRepository := mediastore.NewMiniProgramRepository()
	miniProgramService := mediaapp.NewMiniProgramService(uow, miniProgramRepository, miniProgramRepository, eventstore.NewAppender(), miniProgramRepository)
	surveyService := surveyapp.NewService(uow, surveystore.NewQuestionnaireRepository(), eventstore.NewAppender())
	surveySubmissionRepository := surveystore.NewSubmissionRepository()
	surveySubmissionService := surveyapp.NewSubmissionService(uow, surveySubmissionRepository)
	surveySafeAdminHandler := safeadminhttp.New(surveyapp.NewSafeAdminService(uow, surveySubmissionRepository))
	surveyTokenKey, surveyCookieKey, surveyAbuseKey := deriveSurveyPublicKeys(config.Survey.PublicKey.Value())
	surveyPublicHandler := surveyhttp.NewPublicHandler(
		surveyapp.NewPublicService(uow, surveystore.NewPublicRepository(), eventstore.NewAppender(), surveyTokenKey),
		surveyCookieKey,
		surveyAbuseKey,
	)
	channelService := contactapp.NewChannelServiceWithImageReferences(uow, contactstore.NewChannelRepository(), mediaRepository, eventstore.NewAppender())
	legacyTagService := contactapp.NewLegacyTagCatalogService(uow, contactstore.NewLegacyTagCatalogRepository(), eventstore.NewAppender())
	localTagCatalogHandler, err := contacthttp.NewLocalTagCatalogHandler(legacyTagService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyTagExecutionRepository, err := contactstore.NewLegacyTagExecutionRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyTagSyncService := contactapp.NewLegacyTagSyncService(uow, legacyTagExecutionRepository, eventstore.NewAppender(), legacyTagExecutionRepository)
	legacyTagLiveService := contactapp.NewLegacyTagLiveMutationService(uow, legacyTagExecutionRepository, eventstore.NewAppender(), legacyTagExecutionRepository)
	legacyTagStatusService := contactapp.NewLegacyTagExecutionStatusService(uow, legacyTagExecutionRepository)
	couponService := couponapp.NewService(uow, couponstore.NewRepository(), productstore.NewCatalogRepository(), eventstore.NewAppender())
	automationAgentService := automationapp.NewAgentServiceWithImageReferences(uow, automationstore.NewAgentRepository(), mediaRepository, eventstore.NewAppender())
	productHandler, err := producthttp.NewHandler(productService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	servicePeriodHandler, err := serviceperiodhttp.NewHandler(productapp.NewServicePeriodService(
		uow,
		productstore.NewCatalogRepository(),
		eventstore.NewAppender(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	memberGridCursor, err := membergrid.NewCursorCodec(config.Identity.HMACKey.Value())
	if err != nil {
		pool.Close()
		return nil, err
	}
	memberGridService, err := membergrid.NewService(uow, membergrid.NewRepository(), memberGridCursor)
	if err != nil {
		pool.Close()
		return nil, err
	}
	memberGridHandler, err := membergrid.NewHandler(memberGridService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	memberGridFragment, err := membergrid.NewRouteFragment(memberGridHandler)
	if err != nil {
		pool.Close()
		return nil, err
	}
	productLocalHandler, err := producthttp.NewLocalMutationHandler(
		productService,
		productapp.NewEntitlementService(
			uow,
			productstore.NewCatalogRepository(),
			orderstore.NewRepository(),
			eventstore.NewAppender(),
		),
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	identityRepository := identitystore.NewRepository()
	customerMergeHistoryHandler, err := identityhttp.NewMergeHistoryHandler(identityapp.NewCustomerMergeHistoryService(
		uow, identityRepository,
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	identityReviewHandler, err := identityhttp.NewReviewHandler(identityapp.NewMergeReviewService(
		uow, identityRepository, contactstore.NewMergePortRepository(), eventstore.NewAppender(), config.Identity.HMACKey.Value(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	identityConsoleHandler, err := identityhttp.NewConsoleHandler(identityConsoleApplication{
		resolver: identityapp.NewResolveService(uow, identityRepository),
		binder: identityapp.NewBindServiceWithMergePort(
			uow,
			identityRepository,
			contactstore.NewMergePortRepository(),
			eventstore.NewAppender(),
			config.Identity.HMACKey.Value(),
		),
	})
	if err != nil {
		pool.Close()
		return nil, err
	}
	domainVerification, err := domainverification.New(config.DomainVerification.Directory)
	if err != nil {
		pool.Close()
		return nil, errInvalidAPIComponent
	}
	candidate := &candidateHandler{
		Handler: authHandler, customers: customerHandler,
		customerDetail: customerDetailHandler, customerEvents: customerEventHandler, customerContext: customerContextHandler,
		customerChatActivity: customerChatActivityHandler, customerActivityAnalytics: customerActivityAnalyticsHandler,
		customerMergeHistory: customerMergeHistoryHandler,
		mutations:            mutationHandler, tags: tagCatalogHandler, localTags: localTagCatalogHandler, stages: stageHandler,
		segments:           segmentCRUDHandler,
		products:           productHandler,
		productLocal:       productLocalHandler,
		surveyPublic:       surveyPublicHandler,
		segmentRefresh:     segmentRefreshHandler,
		identityReviews:    identityReviewHandler,
		identityConsole:    identityConsoleHandler,
		automationRuns:     automationstore.NewRepository(pool),
		domainVerification: domainVerification,
		legacyHealth: legacyhealth.NewHandler(legacyhealth.NewQuery(legacyhealth.RuntimeSnapshot{
			DatabaseIsPostgres:                  config.LegacyHealth.DatabaseIsPostgres,
			ProductionEnvironment:               config.LegacyHealth.ProductionEnvironment,
			SecretKeyPresent:                    config.LegacyHealth.SecretKeyPresent,
			WeChatShopCallbackTokenPresent:      config.LegacyHealth.WeChatShopCallbackTokenPresent,
			AllowMissingWeChatShopCallbackToken: config.LegacyHealth.AllowMissingWeChatShopCallbackToken,
		})),
	}
	outboundControlRepository, err := outboundstore.NewControlRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	outboundTaskQueryRepository := outboundstore.NewTaskQueryRepository()
	outboundQueryService := outboundapp.NewTaskQueryService(uow, outboundTaskQueryRepository)
	externalEffectsRepository, err := outboundstore.NewExternalEffectsRepository(outboundTaskQueryRepository)
	if err != nil {
		pool.Close()
		return nil, err
	}
	// ExternalEffectsService derives independent job-id and cursor keys with
	// domain-separated labels. Reuse the existing deployment-local HMAC root;
	// never pass a provider, webhook, recipient, or message secret here.
	externalEffectsService, err := outboundapp.NewExternalEffectsService(uow, externalEffectsRepository, config.Identity.HMACKey.Value())
	if err != nil {
		pool.Close()
		return nil, err
	}
	externalEffectsHandler, err := outboundhttp.NewExternalEffectsHandler(externalEffectsService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	configRepository := configstore.NewRepository()
	configManager := configapp.NewManager(uow, configRepository, eventstore.NewAppender())
	settingsService := configapp.NewSettingsCompatibilityService(uow, configRepository, configManager, configapp.SecretConfiguredSnapshot{
		DatabaseURL: true, WeComSecret: config.WeCom.OAuth.Enabled,
		WeComCallbackToken: config.WeCom.Callback.Enabled, WeComCallbackAESKey: config.WeCom.Callback.Enabled,
	})
	adminOpsService := adminopsapp.NewService(uow, adminopsstore.NewRepository())
	legacyHandler, err := NewHandlerWithAll(
		service, customerService,
		contactapp.NewCustomerDetailService(uow, contactstore.NewCustomerDetailRepository()),
		identityapp.NewResolveService(uow, identityRepository), config.WeCom.OAuth.CorpID,
		outboundQueryService,
		outboundapp.NewCancelService(uow, outboundControlRepository, eventstore.NewAppender()),
		outboundapp.NewManualRetryService(uow, outboundControlRepository, eventstore.NewAppender()),
		productService, mediaService, groupInviteService, miniProgramService, surveyService, channelService, couponService, legacyTagService, settingsService,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyHandler.legacyTagSync = legacyTagSyncService
	legacyHandler.servicePeriod = servicePeriodHandler
	legacyHandler.memberGrid = memberGridFragment
	legacyHandler.imageDeletes = imageDeleteService
	legacyHandler.legacyTagLive = legacyTagLiveService
	legacyHandler.legacyTagStatus = legacyTagStatusService
	legacyHandler.adminOps = adminOpsService
	legacyHandler.runtimeConfig = runtimeConfigDeclarationFromConfig(config)
	legacyHandler.orders = orderapp.NewService(
		uow, orderstore.NewRepository(), contactstore.NewCustomerDetailRepository(), productstore.NewCatalogRepository(),
	)
	legacyHandler.orderBoard = orderapp.NewBoardService(uow, orderstore.NewRepository(), eventstore.NewAppender())
	legacyHandler.couponBoard = couponService
	legacyHandler.automationAgents = automationAgentService
	legacyHandler.messageArchive = messageArchiveService
	legacyHandler.messageArchiveUnionID = identityapp.NewMessageArchiveUnionIDResolver(uow, identityRepository)
	legacyHandler.operationCycles = operationapp.NewService(uow, operationstore.NewRepository(), eventstore.NewAppender(), deliveryProducer)
	legacyHandler.pushCenter = pushcenterapp.NewService(uow, pushcenterstore.NewRepository())
	legacyHandler.externalEffects = externalEffectsHandler
	legacyHandler.surveySubmissions = surveySubmissionService
	legacyHandler.surveySafeAdmin = surveySafeAdminHandler
	legacyHandler.executionRuntime = adminopsapp.NewExecutionRuntimeService(emptyExecutionRuntimeReader{})
	hxcStaffDirectory := contactstore.NewStaffDirectoryRepository(pool)
	hxcSenderRepository := hxcstore.NewSenderConfigRepository(pool)
	legacyHandler.hxcSender = &hxcSenderHandler{
		reader:  hxcapp.Reader{Staff: hxcStaffDirectory, Configs: hxcSenderRepository},
		manager: hxcapp.NewManager(uow, hxcSenderRepository, hxcStaffDirectory, eventstore.NewAppender()),
	}
	eventDeliveryLineage, err := eventapp.NewDeliveryLineageReader(uow, eventstore.NewDeliveryLineageRepository(), config.Identity.HMACKey.Value())
	if err != nil {
		pool.Close()
		return nil, errInvalidAPIComponent
	}
	legacyHandler.deliveryLineage = legacyDeliveryLineageReaders{
		outbound: outboundapp.NewDeliveryLineageReader(uow, outboundstore.NewDeliveryLineageRepository()),
		events:   eventDeliveryLineage,
	}
	dataHealthSource := postgresSystemHealthSource{
		platformObserver: opsstore.NewReadinessRepository(pool),
		queueObserver:    outboundstore.NewReadinessRepository(pool),
		production:       strings.EqualFold(strings.TrimSpace(config.Release.Environment), "production"),
		releaseSHA:       config.Release.SHA,
		realCallsEnabled: config.WeCom.OAuth.Enabled,
	}
	systemHealth, err := newSystemHealthHandler(dataHealthSource)
	if err != nil {
		pool.Close()
		return nil, errInvalidAPIComponent
	}
	legacyHandler.systemHealth = systemHealth
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	callbackDispatcher, err := wecomcallback.NewEventDispatcher(uow, eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, errInvalidAPIComponent
	}
	callbackHandler, err := wecomcallback.NewHandler(wecomcallback.Config{
		Enabled:        config.WeCom.Callback.Enabled,
		CorpID:         config.WeCom.Callback.CorpID,
		Token:          config.WeCom.Callback.Token.Value(),
		EncodingAESKey: config.WeCom.Callback.EncodingAESKey.Value(),
	}, wecomcallback.Options{Dispatcher: callbackDispatcher})
	if err != nil {
		pool.Close()
		return nil, errInvalidAPIComponent
	}
	adminReadRepository := eventstore.NewAdminReadRepository(pool)
	adminDetailRepository := eventstore.NewAdminDetailRepository(pool)
	handler, err := newAPIHandlerWithAdminRead(logger, callbackHandler, authHandler, candidate, legacyHandler, humanAuth, adminReadRepository, dataHealthSource, adminDetailRepository)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &apiComponent{
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       time.Minute,
		},
		pool: pool, listen: net.Listen, address: config.API.ListenAddress,
	}, nil
}

func newAPIHandler(logger *slog.Logger, authHandler *authhttp.Handler, candidate api.ServerInterface) (http.Handler, error) {
	callbackHandler, err := wecomcallback.NewHandler(wecomcallback.Config{}, wecomcallback.Options{})
	if err != nil {
		return nil, err
	}
	return newAPIHandlerWithCallback(logger, callbackHandler, authHandler, candidate)
}

func newAPIHandlerWithCallback(logger *slog.Logger, callbackHandler http.Handler, authHandler *authhttp.Handler, candidate api.ServerInterface) (http.Handler, error) {
	return newAPIHandlerWithCallbackAndLegacy(logger, callbackHandler, authHandler, candidate, nil)
}

func newAPIHandlerWithCallbackAndLegacy(logger *slog.Logger, callbackHandler http.Handler, authHandler *authhttp.Handler, candidate api.ServerInterface, legacy *Handler) (http.Handler, error) {
	return newAPIHandlerWithAll(logger, callbackHandler, authHandler, candidate, legacy, nil)
}

func newAPIHandlerWithAll(logger *slog.Logger, callbackHandler http.Handler, authHandler *authhttp.Handler, candidate api.ServerInterface, legacy *Handler, humanAuth *HumanAuthHandler, dataHealthSources ...legacyDataHealthObservationSource) (http.Handler, error) {
	return newAPIHandlerWithAllOptions(logger, callbackHandler, authHandler, candidate, legacy, humanAuth, nil, dataHealthSources...)
}

func newAPIHandlerWithAdminRead(logger *slog.Logger, callbackHandler http.Handler, authHandler *authhttp.Handler, candidate api.ServerInterface, legacy *Handler, humanAuth *HumanAuthHandler, repository eventport.AdminReadRepository, dataHealthSource legacyDataHealthObservationSource, detailRepositories ...eventport.AdminDetailRepository) (http.Handler, error) {
	var detailRepository eventport.AdminDetailRepository
	if len(detailRepositories) > 0 {
		detailRepository = detailRepositories[0]
	}
	return newAPIHandlerWithAllOptionsAndAdminDetail(logger, callbackHandler, authHandler, candidate, legacy, humanAuth, repository, detailRepository, dataHealthSource)
}

func newAPIHandlerWithAllOptions(logger *slog.Logger, callbackHandler http.Handler, authHandler *authhttp.Handler, candidate api.ServerInterface, legacy *Handler, humanAuth *HumanAuthHandler, adminReadRepository eventport.AdminReadRepository, dataHealthSources ...legacyDataHealthObservationSource) (http.Handler, error) {
	return newAPIHandlerWithAllOptionsAndAdminDetail(logger, callbackHandler, authHandler, candidate, legacy, humanAuth, adminReadRepository, nil, dataHealthSources...)
}

func newAPIHandlerWithAllOptionsAndAdminDetail(logger *slog.Logger, callbackHandler http.Handler, authHandler *authhttp.Handler, candidate api.ServerInterface, legacy *Handler, humanAuth *HumanAuthHandler, adminReadRepository eventport.AdminReadRepository, adminDetailRepository eventport.AdminDetailRepository, dataHealthSources ...legacyDataHealthObservationSource) (http.Handler, error) {
	if logger == nil || callbackHandler == nil || authHandler == nil || candidate == nil {
		return nil, errInvalidAPIComponent
	}
	gateway, err := platformhttp.NewGateway(platformhttp.GatewayOptions{Logger: logger})
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	recovery := func(next http.Handler) (http.Handler, error) {
		return gateway.RecoveryErrorLog(next)
	}
	health := healthapi.Handler(healthapi.NewStrictHandler(platformhttp.NewHealthHandler(), nil))
	health, err = recovery(health)
	if err != nil {
		return nil, err
	}
	health, err = gateway.RoutePatternMiddleware("/healthz", health)
	if err != nil {
		return nil, err
	}
	router.Method(http.MethodGet, "/healthz", health)
	// The public LEGACY-API-0757 runtime-mode snapshot mounts outside every
	// authentication middleware, exactly like /healthz. GET keeps the shared
	// recovery boundary; non-GET methods bypass RecoveryErrorLog so the frozen
	// legacy handler answers its exact 405 contract directly (the recovery
	// buffer would normalize the empty-code 405 into a 500). Both branches
	// keep the /health route-pattern record.
	if concrete, ok := candidate.(*candidateHandler); ok && concrete.legacyHealth != nil {
		leaf := http.Handler(concrete.legacyHealth)
		withRecovery, recoverErr := recovery(leaf)
		if recoverErr != nil {
			return nil, recoverErr
		}
		getChain, patternErr := gateway.RoutePatternMiddleware("/health", withRecovery)
		if patternErr != nil {
			return nil, patternErr
		}
		methodGuard, patternErr := gateway.RoutePatternMiddleware("/health", leaf)
		if patternErr != nil {
			return nil, patternErr
		}
		router.Handle("/health", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				getChain.ServeHTTP(writer, request)
				return
			}
			methodGuard.ServeHTTP(writer, request)
		}))
	}
	if legacy != nil && legacy.systemHealth != nil {
		systemHealth := legacy.systemHealth
		systemHealth, err = recovery(systemHealth)
		if err != nil {
			return nil, err
		}
		systemHealth, err = gateway.TimeoutMiddleware(systemHealth)
		if err != nil {
			return nil, err
		}
		systemHealth, err = gateway.RoutePatternMiddleware(systemHealthPath, systemHealth)
		if err != nil {
			return nil, err
		}
		router.Method(http.MethodGet, systemHealthPath, systemHealth)
	}
	registerCallback := func(method, pattern string) error {
		tail, wrapErr := recovery(callbackHandler)
		if wrapErr != nil {
			return wrapErr
		}
		tail, wrapErr = gateway.TimeoutMiddleware(tail)
		if wrapErr != nil {
			return wrapErr
		}
		tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
		if wrapErr != nil {
			return wrapErr
		}
		router.Method(method, pattern, tail)
		return nil
	}
	for _, route := range []struct{ method, pattern string }{
		{http.MethodGet, wecomcallback.EventsPath},
		{http.MethodPost, wecomcallback.EventsPath},
		{http.MethodGet, wecomcallback.ExternalContactCallbackPath},
		{http.MethodPost, wecomcallback.ExternalContactCallbackPath},
	} {
		if err = registerCallback(route.method, route.pattern); err != nil {
			return nil, err
		}
	}
	if humanAuth != nil {
		registerHuman := func(method, pattern string, endpoint http.Handler) error {
			tail, wrapErr := recovery(endpoint)
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = gateway.TimeoutMiddleware(tail)
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
			if wrapErr != nil {
				return wrapErr
			}
			router.Method(method, pattern, tail)
			return nil
		}
		for _, route := range []struct {
			method, pattern string
			endpoint        http.Handler
		}{
			{http.MethodGet, legacyLoginPath, http.HandlerFunc(humanAuth.Login)},
			{http.MethodOptions, legacyLoginPath, http.HandlerFunc(humanAuth.Options)},
			{http.MethodGet, legacyLogoutPath, http.HandlerFunc(humanAuth.Logout)},
			{http.MethodOptions, legacyLogoutPath, http.HandlerFunc(humanAuth.Options)},
			{http.MethodGet, weComOAuthStartPath, http.HandlerFunc(humanAuth.Start)},
			{http.MethodOptions, weComOAuthStartPath, http.HandlerFunc(humanAuth.Options)},
			{http.MethodGet, weComOAuthCallbackPath, http.HandlerFunc(humanAuth.Callback)},
			{http.MethodOptions, weComOAuthCallbackPath, http.HandlerFunc(humanAuth.Options)},
		} {
			if err = registerHuman(route.method, route.pattern, route.endpoint); err != nil {
				return nil, err
			}
		}
	}

	wrapper := &api.ServerInterfaceWrapper{Handler: candidate, ErrorHandlerFunc: platformhttp.RequestErrorHandler}
	registerPublicSurvey := func(method, pattern string, endpoint http.Handler) error {
		var allowed http.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			defer func() {
				if recover() == nil {
					return
				}
				logger.Error("public survey handler panic")
				for key := range writer.Header() {
					writer.Header().Del(key)
				}
				writer.Header().Set("Cache-Control", "no-store")
				writer.Header().Set("Content-Type", "application/json; charset=utf-8")
				writer.Header().Set("Referrer-Policy", "no-referrer")
				writer.Header().Set("X-Content-Type-Options", "nosniff")
				writer.WriteHeader(http.StatusServiceUnavailable)
				_, _ = writer.Write([]byte("{\"code\":\"unavailable\"}\n"))
			}()
			endpoint.ServeHTTP(writer, request)
		})
		var wrapErr error
		allowed, wrapErr = gateway.TimeoutMiddleware(allowed)
		if wrapErr != nil {
			return wrapErr
		}
		allowed, wrapErr = gateway.RoutePatternMiddleware(pattern, allowed)
		if wrapErr != nil {
			return wrapErr
		}
		methodGuard, wrapErr := gateway.RoutePatternMiddleware(pattern, endpoint)
		if wrapErr != nil {
			return wrapErr
		}
		// The recovery buffer expects platform errors for non-success statuses.
		// Deliberate Survey 405 responses therefore bypass it, just like /health.
		router.Handle(pattern, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == method {
				allowed.ServeHTTP(writer, request)
				return
			}
			methodGuard.ServeHTTP(writer, request)
		}))
		return nil
	}
	for _, route := range []struct {
		method, pattern string
		endpoint        http.Handler
	}{
		{http.MethodGet, "/api/public/questionnaires/{slug}", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			candidate.GetPublicSurveyDefinition(writer, request, api.PublicSurveySlug(chi.URLParam(request, "slug")))
		})},
		{http.MethodPost, "/api/public/questionnaires/{slug}/submissions", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			candidate.SubmitPublicSurvey(writer, request, api.PublicSurveySlug(chi.URLParam(request, "slug")))
		})},
		{http.MethodPost, "/api/public/survey-submission-results/query", http.HandlerFunc(candidate.QueryPublicSurveySubmissionResult)},
		{http.MethodGet, "/q/{slug}", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			candidate.GetPublicSurveyPage(writer, request, api.PublicSurveySlug(chi.URLParam(request, "slug")))
		})},
	} {
		if err = registerPublicSurvey(route.method, route.pattern, route.endpoint); err != nil {
			return nil, err
		}
	}
	register := func(method, pattern string, capability authport.Capability, csrf bool, endpoint http.Handler) error {
		tail, wrapErr := recovery(endpoint)
		if wrapErr != nil {
			return wrapErr
		}
		tail, wrapErr = gateway.TimeoutMiddleware(tail)
		if wrapErr != nil {
			return wrapErr
		}
		tail, wrapErr = gateway.AccountBudgetMiddleware(tail)
		if wrapErr != nil {
			return wrapErr
		}
		tail, wrapErr = authHandler.Authorize(capability, tail)
		if wrapErr != nil {
			return wrapErr
		}
		if csrf {
			tail, wrapErr = authHandler.RequireCSRF(tail)
			if wrapErr != nil {
				return wrapErr
			}
		}
		tail = authHandler.Authenticate(tail)
		tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
		if wrapErr != nil {
			return wrapErr
		}
		router.Method(method, pattern, tail)
		return nil
	}
	routes := []struct {
		method, pattern string
		capability      authport.Capability
		csrf            bool
		endpoint        http.Handler
	}{
		{http.MethodGet, "/api/v1/admin/config/overview", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(wrapper.GetAdminConfigOverview)},
		{http.MethodPost, "/api/v1/auth/logout", authport.CapabilityAuthSessionLogout, false, http.HandlerFunc(wrapper.LogoutAdmin)},
		{http.MethodGet, "/api/v1/auth/session", authport.CapabilityAuthSessionRead, false, http.HandlerFunc(wrapper.GetAuthSession)},
		{http.MethodGet, "/api/v1/customers", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListCustomers)},
		{http.MethodGet, "/api/v1/customers/{customer_id}", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.GetCustomer)},
		{http.MethodPatch, "/api/v1/customers/{customer_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.UpdateCustomer)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/events", authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(wrapper.ListCustomerEvents)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/context", authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(wrapper.GetCustomerContext)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/merge-history", authport.CapabilityIdentityReviewRead, false, http.HandlerFunc(wrapper.ListCustomerMergeHistory)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/chat-activity", authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(wrapper.ListCustomerChatActivity)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/activity-analytics", authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(wrapper.GetCustomerActivityAnalytics)},
		{http.MethodPut, "/api/v1/customers/{customer_id}/stage", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.SetCustomerStage)},
		{http.MethodPut, "/api/v1/customers/{customer_id}/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.AddCustomerTag)},
		{http.MethodDelete, "/api/v1/customers/{customer_id}/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.RemoveCustomerTag)},
		{http.MethodGet, "/api/v1/tags", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListTags)},
		{http.MethodPost, "/api/v1/tags", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.CreateTag)},
		{http.MethodPut, "/api/v1/tags/reorder", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.ReorderTags)},
		{http.MethodPatch, "/api/v1/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.UpdateTag)},
		{http.MethodDelete, "/api/v1/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.ArchiveTag)},
		{http.MethodGet, "/api/v1/tag-groups", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListTagGroups)},
		{http.MethodPost, "/api/v1/tag-groups", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.CreateTagGroup)},
		{http.MethodPut, "/api/v1/tag-groups/reorder", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.ReorderTagGroups)},
		{http.MethodPatch, "/api/v1/tag-groups/{group_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.UpdateTagGroup)},
		{http.MethodDelete, "/api/v1/tag-groups/{group_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.ArchiveTagGroup)},
		{http.MethodPost, "/api/v1/identity/bind", authport.CapabilityIdentityBind, true, http.HandlerFunc(wrapper.BindIdentity)},
		{http.MethodPost, "/api/v1/identity/ingest", authport.CapabilityIdentityIngest, true, http.HandlerFunc(wrapper.IngestIdentityEvent)},
		{http.MethodPost, "/api/v1/identity/resolve", authport.CapabilityIdentityResolve, false, http.HandlerFunc(wrapper.ResolveIdentity)},
		{http.MethodGet, "/api/v1/identity/merge-reviews", authport.CapabilityIdentityReviewRead, false, http.HandlerFunc(wrapper.ListIdentityMergeReviews)},
		{http.MethodPost, "/api/v1/identity/merge-reviews/{review_id}/approve", authport.CapabilityIdentityReviewWrite, true, http.HandlerFunc(wrapper.ApproveIdentityMergeReview)},
		{http.MethodPost, "/api/v1/identity/merge-reviews/{review_id}/reject", authport.CapabilityIdentityReviewWrite, true, http.HandlerFunc(wrapper.RejectIdentityMergeReview)},
		{http.MethodGet, "/api/v1/segments", authport.CapabilitySegmentsRead, false, http.HandlerFunc(wrapper.ListSegments)},
		{http.MethodGet, "/api/v1/segments/{segment_id}", authport.CapabilitySegmentsRead, false, http.HandlerFunc(wrapper.GetSegment)},
		{http.MethodGet, "/api/v1/products", authport.CapabilityProductsRead, false, http.HandlerFunc(wrapper.ListProducts)},
		{http.MethodPost, "/api/v1/products", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.CreateProduct)},
		{http.MethodGet, "/api/v1/products/{product_id}", authport.CapabilityProductsRead, false, http.HandlerFunc(wrapper.GetProduct)},
		{http.MethodPut, "/api/v1/products/{product_id}", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.UpdateProduct)},
		{http.MethodGet, "/api/v1/products/{product_id}/local-entitlements", authport.CapabilityEntitlementsRead, false, http.HandlerFunc(wrapper.ListProductLocalEntitlements)},
		{http.MethodPost, "/api/v1/products/{product_id}/local-entitlements", authport.CapabilityEntitlementsWrite, true, http.HandlerFunc(wrapper.GrantProductLocalEntitlement)},
		{http.MethodGet, "/api/v1/product-entitlements/{entitlement_id}", authport.CapabilityEntitlementsRead, false, http.HandlerFunc(wrapper.GetProductLocalEntitlement)},
		{http.MethodPost, "/api/v1/product-entitlements/{entitlement_id}/revoke", authport.CapabilityEntitlementsWrite, true, http.HandlerFunc(wrapper.RevokeProductLocalEntitlement)},
		{http.MethodPost, "/api/v1/segments", authport.CapabilitySegmentsWrite, true, http.HandlerFunc(wrapper.CreateSegment)},
		{http.MethodPatch, "/api/v1/segments/{segment_id}", authport.CapabilitySegmentsWrite, true, http.HandlerFunc(wrapper.UpdateSegment)},
		{http.MethodDelete, "/api/v1/segments/{segment_id}", authport.CapabilitySegmentsWrite, true, http.HandlerFunc(wrapper.ArchiveSegment)},
		{http.MethodGet, "/api/v1/segments/{segment_id}/members", authport.CapabilitySegmentsRead, false, http.HandlerFunc(wrapper.ListSegmentMembers)},
		{http.MethodPost, "/api/v1/segments/{segment_id}/refresh", authport.CapabilitySegmentsWrite, true, http.HandlerFunc(wrapper.RequestSegmentRefresh)},
		{http.MethodGet, "/api/v1/stages", authport.CapabilityStagesRead, false, http.HandlerFunc(wrapper.ListStages)},
		{http.MethodPost, "/api/v1/stages", authport.CapabilityStagesWrite, true, http.HandlerFunc(wrapper.CreateStage)},
		{http.MethodPut, "/api/v1/stages/reorder", authport.CapabilityStagesWrite, true, http.HandlerFunc(wrapper.ReorderStages)},
		{http.MethodDelete, "/api/v1/stages/{stage_id}", authport.CapabilityStagesWrite, true, http.HandlerFunc(wrapper.ArchiveStage)},
		{http.MethodPatch, "/api/v1/stages/{stage_id}", authport.CapabilityStagesWrite, true, http.HandlerFunc(wrapper.RenameStage)},
		{http.MethodPost, "/api/admin/questionnaires/{questionnaire_id}/public-publish", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(wrapper.PublishQuestionnairePublicDefinition)},
		{http.MethodPost, "/api/admin/questionnaires/{questionnaire_id}/public-disable", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(wrapper.DisableQuestionnairePublicDefinition)},
		{http.MethodGet, "/api/admin/questionnaires/{questionnaire_id}/public-analytics", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(wrapper.GetQuestionnairePublicAnalytics)},
	}
	for _, route := range routes {
		if err = register(route.method, route.pattern, route.capability, route.csrf, route.endpoint); err != nil {
			return nil, err
		}
	}
	if legacy != nil {
		var source legacyDataHealthObservationSource
		if len(dataHealthSources) > 0 {
			source = dataHealthSources[0]
		}
		dataHealth := newLegacyDataHealthHandler(source)
		cloudOrchestratorPages := automationhttp.NewCloudOrchestratorPages()
		groupOpsPages := automationhttp.NewGroupOpsPages()
		audiencePackagePages := automationhttp.NewAudiencePackagePages()
		userOpsPages := automationhttp.NewUserOpsPages()
		commerceWorkspacePages := producthttp.NewWorkspacePages()
		isCloudOrchestratorPagePattern := func(pattern string) bool {
			return pattern == automationhttp.CloudOrchestratorRootPath ||
				pattern == automationhttp.CloudOrchestratorPlansPath ||
				pattern == automationhttp.CloudOrchestratorPlanDetailPattern ||
				pattern == automationhttp.CloudOrchestratorCampaignsPath ||
				pattern == automationhttp.CloudOrchestratorObservabilityPath
		}
		strictLegacyMethodRouters := make(map[string]*chi.Mux)
		legacyAPIDocs, docsErr := newLegacyAPIDocsHandler()
		if docsErr != nil {
			return nil, docsErr
		}
		registerLegacy := func(method, pattern string, capability authport.Capability, csrf bool, endpoint http.Handler) error {
			var (
				tail    http.Handler
				wrapErr error
			)
			if method == http.MethodGet && pattern == legacyImageDetailPath {
				tail, wrapErr = gateway.RecoveryErrorLogWithResponseBufferLimit(endpoint, legacyImageDetailMaxJSONLen)
			} else {
				tail, wrapErr = recovery(endpoint)
			}
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = gateway.TimeoutMiddleware(tail)
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = gateway.AccountBudgetMiddleware(tail)
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = legacy.Authorize(capability, tail)
			if wrapErr != nil {
				return wrapErr
			}
			if csrf {
				tail, wrapErr = legacy.RequireCSRF(tail)
				if wrapErr != nil {
					return wrapErr
				}
			}
			tail = legacy.Authenticate(tail)
			tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
			if wrapErr != nil {
				return wrapErr
			}
			if pattern == legacyCustomerProfileTagsPath {
				tail = legacyCustomerProfileTagsSecurityHeaders(tail)
			}
			if pattern == legacyCustomerProfileMessagesPath {
				tail = legacyCustomerProfileMessagesSecurityHeaders(tail)
			}
			if pattern == legacyChannelPagePath {
				tail = legacyChannelPageSecurityHeaders(tail)
			}
			if isLegacyCustomerPagePattern(pattern) {
				tail = legacyCustomerPageRouteGuard(pattern, tail)
				tail = legacyCustomerPageSecurityHeaders(tail)
			}
			if pattern == legacyCouponPagePath {
				tail = legacyCouponPageSecurityHeaders(tail)
			}
			if pattern == legacyOrderPagePath {
				tail = legacyOrderPageSecurityHeaders(tail)
			}
			if pattern == legacyProductPagePath {
				tail = legacyProductPageSecurityHeaders(tail)
			}
			if pattern == legacyExecutionRuntimePagePath {
				tail = legacyExecutionRuntimePageSecurityHeaders(tail)
			}
			if pattern == legacyAutomationAgentListPagePath {
				tail = legacyAutomationAgentListPageSecurityHeaders(tail)
			}
			if isCloudOrchestratorPagePattern(pattern) {
				tail = automationhttp.CloudOrchestratorPageSecurityHeaders(tail)
			}
			if automationhttp.IsGroupOpsPagePattern(pattern) {
				tail = automationhttp.GroupOpsPageSecurityHeaders(tail)
			}
			if automationhttp.IsAudiencePackagePagePattern(pattern) {
				tail = automationhttp.AudiencePackagePageSecurityHeaders(tail)
			}
			if automationhttp.IsUserOpsPagePattern(pattern) {
				tail = automationhttp.UserOpsPageSecurityHeaders(tail)
			}
			if producthttp.IsWorkspacePagePattern(pattern) {
				tail = producthttp.WorkspacePageSecurityHeaders(tail)
			}
			if pattern == legacyQuestionnairePagePath || pattern == legacyQuestionnairePagePath+"/ui" || pattern == legacyQuestionnairePreflightPath ||
				(method == http.MethodGet && pattern == "/api/admin/questionnaires") ||
				(method == http.MethodPost && pattern == "/api/admin/questionnaires/{questionnaire_id}/disable") ||
				(method == http.MethodDelete && pattern == "/api/admin/questionnaires/{questionnaire_id}") {
				tail = legacyQuestionnaireSecurityHeaders(tail)
			}
			if (method == http.MethodPut || method == http.MethodDelete) && pattern == legacyImageDetailPath {
				tail = legacyImageUpdateSecurityHeaders(tail)
			}
			if method == http.MethodPost && pattern == legacyImageCollectionPath {
				tail = legacyImageCreateSecurityHeaders(tail)
			}
			if pattern == legacyInternalEventsPath || pattern == legacyInternalEventsDiagnosticsPath || pattern == legacyInternalEventDetailPath {
				tail = legacyInternalEventsSecurityHeaders(tail)
			}
			if pattern == legacyImageCollectionPath || pattern == legacyImageFacetsPath || pattern == legacyImageDetailPath || pattern == legacyImageVariantPath || pattern == legacyApiDocsPath || pattern == legacyMcpToolsPath || pattern == legacyDataHealthChecksPath || pattern == legacyDataHealthCheckPath || pattern == legacyDataHealthSummaryPath || pattern == legacyHXCSenderReadPath || pattern == legacyHXCSenderItemPath || pattern == legacyHXCSenderReorderPath || pattern == legacyDeliveryLineagePath || pattern == legacyInternalEventsPath || pattern == legacyInternalEventsDiagnosticsPath || pattern == legacyInternalEventDetailPath || pattern == legacyCustomerProfileTagsPath || pattern == legacyCustomerProfileMessagesPath || pattern == legacyChannelPagePath || pattern == legacyCouponPagePath || pattern == legacyOrderPagePath || pattern == legacyProductPagePath || pattern == legacyExecutionRuntimePagePath || pattern == legacyAutomationAgentListPagePath || pattern == legacyQuestionnairePagePath || pattern == legacyQuestionnairePagePath+"/ui" || pattern == legacyQuestionnairePreflightPath || pattern == legacyRuntimeConfigPath || pattern == legacyConfigChecklistPath || isLegacyCustomerPagePattern(pattern) || isCloudOrchestratorPagePattern(pattern) || automationhttp.IsGroupOpsPagePattern(pattern) || automationhttp.IsAudiencePackagePagePattern(pattern) || automationhttp.IsUserOpsPagePattern(pattern) || producthttp.IsWorkspacePagePattern(pattern) {
				// Keep the strict image-library reads out of the compatibility
				// router's legacy 400 method adapter. A per-path method router lets
				// Chi return 405 before authentication and preserves the shared
				// collection path shared by the owned 0356 GET and 0357 POST.
				// The API-docs page and the MCP-tools redirect use the same
				// mechanism so non-GET methods see 405 before authentication.
				methodRouter := strictLegacyMethodRouters[pattern]
				if methodRouter == nil {
					methodRouter = chi.NewRouter()
					if pattern == legacyImageDetailPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyImageDetailMethodNotAllowed))
					} else if pattern == legacyImageCollectionPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyImageCollectionMethodNotAllowed))
					}
					if pattern == legacyDeliveryLineagePath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyDeliveryLineageMethodNotAllowed))
					}
					if pattern == legacyInternalEventsPath || pattern == legacyInternalEventsDiagnosticsPath || pattern == legacyInternalEventDetailPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyInternalEventsMethodNotAllowed))
					}
					if pattern == legacyCustomerProfileTagsPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyCustomerProfileTagsMethodNotAllowed))
					}
					if pattern == legacyCustomerProfileMessagesPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyCustomerProfileMessagesMethodNotAllowed))
					}
					if pattern == legacyChannelPagePath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyChannelPageMethodNotAllowed))
					}
					if isLegacyCustomerPagePattern(pattern) {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyCustomerPageMethodNotAllowed))
					}
					if pattern == legacyCouponPagePath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyCouponPageMethodNotAllowed))
					}
					if pattern == legacyOrderPagePath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyOrderPageMethodNotAllowed))
					}
					if pattern == legacyProductPagePath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyProductPageMethodNotAllowed))
					}
					if pattern == legacyExecutionRuntimePagePath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyExecutionRuntimePageMethodNotAllowed))
					}
					if pattern == legacyAutomationAgentListPagePath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyAutomationAgentListPageMethodNotAllowed))
					}
					if isCloudOrchestratorPagePattern(pattern) {
						methodRouter.MethodNotAllowed(http.HandlerFunc(automationhttp.WriteCloudOrchestratorPageMethodNotAllowed))
					}
					if automationhttp.IsGroupOpsPagePattern(pattern) {
						methodRouter.MethodNotAllowed(http.HandlerFunc(automationhttp.WriteGroupOpsPageMethodNotAllowed))
					}
					if automationhttp.IsAudiencePackagePagePattern(pattern) {
						methodRouter.MethodNotAllowed(http.HandlerFunc(automationhttp.WriteAudiencePackagePageMethodNotAllowed))
					}
					if automationhttp.IsUserOpsPagePattern(pattern) {
						methodRouter.MethodNotAllowed(http.HandlerFunc(automationhttp.WriteUserOpsPageMethodNotAllowed))
					}
					if producthttp.IsWorkspacePagePattern(pattern) {
						methodRouter.MethodNotAllowed(http.HandlerFunc(producthttp.WriteWorkspacePageMethodNotAllowed))
					}
					if pattern == legacyQuestionnairePagePath || pattern == legacyQuestionnairePagePath+"/ui" || pattern == legacyQuestionnairePreflightPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyQuestionnaireMethodNotAllowed))
					}
					if pattern == legacyRuntimeConfigPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyRuntimeConfigMethodNotAllowed))
					}
					if pattern == legacyConfigChecklistPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyConfigChecklistMethodNotAllowed))
					}
					strictLegacyMethodRouters[pattern] = methodRouter
					router.Handle(pattern, methodRouter)
				}
				methodRouter.Method(method, pattern, tail)
				return nil
			}
			router.Method(method, pattern, tail)
			return nil
		}
		if legacy.servicePeriod != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
				csrf            bool
			}{
				{http.MethodGet, serviceperiodhttp.BasePath, authport.CapabilityProductsRead, false},
				{http.MethodPost, serviceperiodhttp.BasePath, authport.CapabilityProductsWrite, true},
				{http.MethodGet, serviceperiodhttp.BasePath + "/{service_product_id}", authport.CapabilityProductsRead, false},
				{http.MethodPut, serviceperiodhttp.BasePath + "/{service_product_id}", authport.CapabilityProductsWrite, true},
				{http.MethodPost, serviceperiodhttp.BasePath + "/{service_product_id}/enable", authport.CapabilityProductsWrite, true},
				{http.MethodPost, serviceperiodhttp.BasePath + "/{service_product_id}/disable", authport.CapabilityProductsWrite, true},
				{http.MethodPost, serviceperiodhttp.BasePath + "/{service_product_id}/copy", authport.CapabilityProductsWrite, true},
				{http.MethodDelete, serviceperiodhttp.BasePath + "/{service_product_id}", authport.CapabilityProductsWrite, true},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, legacy.servicePeriod); err != nil {
					return nil, err
				}
			}
		}
		if legacy.memberGrid != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
			}{
				{http.MethodGet, membergrid.RoutePrefix + "/{service_product_id}/member-grid/access", authport.CapabilityProductsRead},
				{http.MethodGet, membergrid.RoutePrefix + "/{service_product_id}/member-grid/schema", authport.CapabilityProductsRead},
				{http.MethodGet, membergrid.RoutePrefix + "/{service_product_id}/member-views", authport.CapabilityProductsRead},
				{http.MethodPost, membergrid.RoutePrefix + "/{service_product_id}/member-grid/query", authport.CapabilityEntitlementsRead},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, false, legacy.memberGrid); err != nil {
					return nil, err
				}
			}
		}
		for _, route := range []struct {
			method, pattern string
			capability      authport.Capability
			csrf            bool
			endpoint        http.Handler
		}{
			{http.MethodGet, legacyDataHealthChecksPath, authport.CapabilityAdminRead, false, http.HandlerFunc(dataHealth.List)},
			{http.MethodGet, legacyDataHealthCheckPath, authport.CapabilityAdminRead, false, http.HandlerFunc(dataHealth.Detail)},
			{http.MethodGet, legacyDataHealthSummaryPath, authport.CapabilityAdminRead, false, http.HandlerFunc(dataHealth.Summary)},
			{http.MethodGet, legacyHXCSenderPagePath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.hxcSender.Page)},
			{http.MethodGet, legacyHXCSenderReadPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.hxcSender.Read)},
			{http.MethodPost, legacyHXCSenderReadPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.hxcSender.Save)},
			{http.MethodPut, legacyHXCSenderReorderPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.hxcSender.Reorder)},
			{http.MethodDelete, legacyHXCSenderItemPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.hxcSender.Archive)},
			{http.MethodGet, legacyDeliveryLineagePath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.GetDeliveryLineage)},
			{http.MethodGet, legacyInternalEventsPath, authport.CapabilityAdminRead, false, http.HandlerFunc(newLegacyInternalEventsHandler(adminReadRepository).List)},
			{http.MethodGet, legacyInternalEventsDiagnosticsPath, authport.CapabilityAdminRead, false, http.HandlerFunc(newLegacyInternalEventsHandler(adminReadRepository).Diagnostics)},
			{http.MethodGet, legacyInternalEventDetailPath, authport.CapabilityAdminRead, false, http.HandlerFunc(newLegacyInternalEventDetailHandler(adminDetailRepository).Get)},
			{http.MethodGet, legacyCustomerProfileTagsPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.GetCustomerProfileTags)},
			{http.MethodGet, legacyCustomerProfileMessagesPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.GetCustomerProfileMessages)},
			{http.MethodGet, legacyCustomerListPagePath, authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.CustomerListPage)},
			{http.MethodGet, legacyCustomerDetailPagePattern, authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.CustomerDetailPage)},
			{http.MethodGet, legacyCustomerContextPagePattern, authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(legacy.CustomerContextPage)},
			{http.MethodGet, "/api/admin/config/overview", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.ConfigOverview)},
			{http.MethodGet, "/api/admin/config/capabilities", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.Capabilities)},
			{http.MethodGet, "/admin/config/app-settings", authport.CapabilityConfigSettingsManage, false, http.HandlerFunc(legacy.AppSettingsPage)},
			{http.MethodPost, "/admin/config/app-settings/save", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.SaveAppSettings)},
			{http.MethodGet, "/api/admin/config/app-settings", authport.CapabilityConfigSettingsManage, false, http.HandlerFunc(legacy.AppSettingsResource)},
			{http.MethodPut, "/api/admin/config/app-settings", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.SaveAppSettingsResource)},
			{http.MethodGet, "/admin/config", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/api-key", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/api-clients", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/api-clients/new", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/api-clients/{client_id}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/detail/{category_key}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/wecom-tags", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/releases", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/releases/new", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/admin/config/releases", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/admin/config/releases/{release_id}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/admin/config/releases/{release_id}/validate", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/admin/config/releases/{release_id}/publish", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/admin/config/releases/{release_id}/rollback", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, legacyRuntimeConfigPath, authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, legacyApiDocsPath, authport.CapabilityConfigOverviewRead, false, legacyAPIDocs},
			{http.MethodGet, legacyMcpToolsPath, authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacyMcpToolsRedirect)},
			{http.MethodGet, legacyConfigChecklistPath, authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.ConfigChecklist)},
			{http.MethodGet, "/setup/wizard", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/setup/wizard/save", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/api-key", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/api-key/generate", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/api-key/rotate", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPut, "/api/admin/config/api-key/enabled", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/api-clients", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/api-clients", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/api-clients/{client_id}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPut, "/api/admin/config/api-clients/{client_id}", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/api-clients/{client_id}/activate", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/api-clients/{client_id}/rotate-secret", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPut, "/api/admin/config/api-clients/{client_id}/enabled", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/categories", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/categories/{category_key}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPut, "/api/admin/config/categories/{category_key}/enabled", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPut, "/api/admin/config/categories/{category_key}/settings", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/categories/{category_key}/check", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/definitions", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/deployment-profile", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/releases", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/releases", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/releases/{release_id}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/releases/{release_id}/validate", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/releases/{release_id}/publish", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/releases/{release_id}/rollback", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/releases/{release_id}/shadow-compare", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/push-capabilities", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPatch, "/api/admin/config/push-capabilities/scheduler", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPatch, "/api/admin/config/push-capabilities/{capability_key}", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/routing", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/routing/owner-role", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/routing/rule", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/signup-tags", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/signup-tags", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/config/class-term-tags", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/config/class-term-tags", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/jobs/summary", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/jobs/archive-sync", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/jobs/archive-sync/run", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/jobs/callbacks", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/jobs/deferred-jobs", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/jobs/webhook-deliveries", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/jobs/message-batches", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/jobs/message-batches/{batch_id}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/jobs/message-batches/{batch_id}/ack", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/jobs/order-identity-repair/run", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/broadcast-jobs", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/broadcast-jobs/notification-settings/feishu", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPut, "/api/admin/broadcast-jobs/notification-settings/feishu", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/broadcast-jobs/notification-settings/feishu/validate", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/broadcast-jobs/feishu-hourly-report/run", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/broadcast-jobs/{job_id}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/broadcast-jobs/{job_id}/approve", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodPost, "/api/admin/broadcast-jobs/{job_id}/cancel", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.AdminOps)},
			{http.MethodGet, "/api/admin/automation-conversion/agent-runs", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(wrapper.ListAutomationTriggerRuns)},
			{http.MethodGet, automationhttp.CloudOrchestratorRootPath, authport.CapabilityAdminRead, false, cloudOrchestratorPages},
			{http.MethodGet, automationhttp.CloudOrchestratorPlansPath, authport.CapabilityAdminRead, false, cloudOrchestratorPages},
			{http.MethodGet, automationhttp.CloudOrchestratorPlanDetailPattern, authport.CapabilityAdminRead, false, cloudOrchestratorPages},
			{http.MethodGet, automationhttp.CloudOrchestratorCampaignsPath, authport.CapabilityAdminRead, false, cloudOrchestratorPages},
			{http.MethodGet, automationhttp.CloudOrchestratorObservabilityPath, authport.CapabilityAdminRead, false, cloudOrchestratorPages},
			{http.MethodGet, automationhttp.GroupOpsPlansPath, authport.CapabilityAdminRead, false, groupOpsPages},
			{http.MethodGet, automationhttp.GroupOpsPlanDetailPattern, authport.CapabilityAdminRead, false, groupOpsPages},
			{http.MethodGet, automationhttp.AudiencePackagesPath, authport.CapabilityAdminRead, false, audiencePackagePages},
			{http.MethodGet, automationhttp.AudiencePackageDetailPattern, authport.CapabilityAdminRead, false, audiencePackagePages},
			{http.MethodGet, automationhttp.UserOpsPath, authport.CapabilityAdminRead, false, userOpsPages},
			{http.MethodGet, automationhttp.UserOpsUIPath, authport.CapabilityAdminRead, false, userOpsPages},
			{http.MethodGet, producthttp.AlipayTransactionsPath, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.ServiceProductsPath, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.ServiceProductNewPath, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.ServiceProductEditPattern, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.ServiceProductDataPattern, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.WeChatPayProductNewPath, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.WeChatPayProductEditPattern, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.WeChatPayTransactionsPath, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.WeChatPayTransactionPattern, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.WeChatShopTransactionsPath, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, producthttp.WeChatShopTransactionPattern, authport.CapabilityAdminRead, false, commerceWorkspacePages},
			{http.MethodGet, "/admin/automation-agents", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AutomationAgentListPage)},
			{http.MethodGet, "/admin/automation-agents/{agent_id}/edit", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.AutomationAgentEditPage)},
			{http.MethodGet, "/api/admin/automation-agents", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.ListAutomationAgents)},
			{http.MethodPost, "/api/admin/automation-agents", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.CreateAutomationAgent)},
			{http.MethodDelete, "/api/admin/automation-agents/{agent_id}", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.DeleteAutomationAgent)},
			{http.MethodGet, "/api/admin/automation-agents/{agent_id}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.GetAutomationAgent)},
			{http.MethodPatch, "/api/admin/automation-agents/{agent_id}", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.UpdateAutomationAgent)},
			{http.MethodPost, "/api/admin/automation-agents/{agent_id}/activate", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.ActivateAutomationAgent)},
			{http.MethodPost, "/api/admin/automation-agents/{agent_id}/copy", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.CopyAutomationAgent)},
			{http.MethodPut, "/api/admin/automation-agents/{agent_id}/fixed-content", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.SaveAutomationAgentFixedContent)},
			{http.MethodPost, "/api/admin/automation-agents/{agent_id}/pause", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.PauseAutomationAgent)},
			{http.MethodPost, "/api/admin/automation-agents/{agent_id}/publish", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.PublishAutomationAgent)},
			{http.MethodGet, "/api/archive/health", authport.CapabilityMessageArchiveRead, false, http.HandlerFunc(legacy.ArchiveHealth)},
			{http.MethodPost, "/api/archive/sync", authport.CapabilityMessageArchiveExecute, true, http.HandlerFunc(legacy.RequestArchiveSync)},
			{http.MethodGet, "/api/external/chat-records", authport.CapabilityMessageArchiveExternalRead, false, http.HandlerFunc(legacy.ListExternalChatRecords)},
			{http.MethodGet, "/api/messages/search", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.SearchArchivedMessages)},
			{http.MethodGet, "/api/messages/archive", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.DeprecatedMessageArchive)},
			{http.MethodGet, "/api/messages/{external_userid}/archive", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.DeprecatedExternalMessageArchive)},
			{http.MethodGet, "/api/messages/{external_userid}/history", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.DeprecatedExternalMessageHistory)},
			{http.MethodGet, "/api/messages/{external_userid}", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.ListArchivedMessages)},
			{http.MethodGet, "/api/customers", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.ListCustomers)},
			{http.MethodGet, "/api/customers/{external_userid}", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetCustomer)},
			{http.MethodGet, "/api/admin/push-center/jobs", authport.CapabilityOutboundRead, false, http.HandlerFunc(legacy.ListOutboundJobs)},
			{http.MethodGet, "/api/admin/push-center/jobs/{job_id}", authport.CapabilityOutboundRead, false, http.HandlerFunc(legacy.GetOutboundJob)},
			{http.MethodGet, "/api/admin/push-center/jobs/{job_id}/reconciliation", authport.CapabilityOutboundRead, false, http.HandlerFunc(legacy.ReconcileOutboundJob)},
			{http.MethodPost, "/api/admin/push-center/jobs/{job_id}/cancel", authport.CapabilityOutboundControl, true, http.HandlerFunc(legacy.CancelOutboundJob)},
			{http.MethodPost, "/api/admin/push-center/jobs/{job_id}/retry", authport.CapabilityOutboundControl, true, http.HandlerFunc(legacy.RetryOutboundJob)},
			{http.MethodGet, "/api/admin/push-center/sections", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.PushCenterSections)},
			{http.MethodGet, "/api/admin/push-center/stats", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.PushCenterStats)},
			{http.MethodGet, outboundhttp.ExternalEffectsJobsPath, authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.ExternalEffectsJobs)},
			{http.MethodGet, outboundhttp.ExternalEffectsDiagnosticsPath, authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.ExternalEffectsDiagnostics)},
			{http.MethodGet, legacyExecutionRuntimePagePath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.ExecutionRuntimePage)},
			{http.MethodGet, "/api/admin/execution-runtime", authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.ExecutionRuntime)},
			{http.MethodGet, "/api/admin/executions/{execution_id}", authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.ExecutionTimeline)},
			{http.MethodGet, legacyProductPagePath, authport.CapabilityProductsRead, false, http.HandlerFunc(legacy.ProductListPage)},
			{http.MethodGet, "/api/admin/wechat-pay/products", authport.CapabilityProductsRead, false, http.HandlerFunc(legacy.ListProducts)},
			{http.MethodPost, "/api/admin/wechat-pay/products", authport.CapabilityProductsWrite, true, http.HandlerFunc(legacy.CreateProduct)},
			{http.MethodGet, "/api/admin/wechat-pay/products/{product_id}", authport.CapabilityProductsRead, false, http.HandlerFunc(legacy.GetProduct)},
			{http.MethodGet, legacyOrderPagePath, authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.OrderListPage)},
			{http.MethodGet, "/api/admin/orders", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListOrderBoard)},
			{http.MethodGet, "/api/admin/orders/{order_no}", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.GetOrderBoard)},
			{http.MethodGet, "/api/admin/orders/{order_no}/items", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.GetOrderBoardItems)},
			{http.MethodGet, "/api/admin/alipay/transactions", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListAlipayTransactions)},
			{http.MethodGet, "/api/admin/alipay/transactions/{order_no}", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.GetAlipayTransaction)},
			{http.MethodGet, "/api/admin/wechat-pay/orders", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListWechatTransactions)},
			{http.MethodGet, "/api/admin/refunds", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListOrderBoardRefunds)},
			{http.MethodPost, "/api/admin/exports", authport.CapabilityOrderWrite, true, http.HandlerFunc(legacy.CreateOrderBoardExport)},
			{http.MethodGet, "/api/admin/exports/{job_id}", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.GetOrderBoardExport)},
			{http.MethodPost, "/api/admin/wechat-pay/order-exports", authport.CapabilityOrderWrite, true, http.HandlerFunc(legacy.CreateWechatOrderBoardExport)},
			{http.MethodGet, "/api/admin/wechat-pay/order-exports/{job_id}", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.DeprecatedWechatOrderBoardExport)},
			{http.MethodGet, "/api/admin/wechat-pay/order-exports/{job_id}/download", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.DeprecatedWechatOrderBoardExport)},
			{http.MethodPost, "/api/admin/refunds", authport.CapabilityOrderWrite, true, http.HandlerFunc(legacy.CreateOrderBoardRefund)},
			{http.MethodPost, "/api/admin/wechat-pay/orders/{order_id}/refunds", authport.CapabilityOrderWrite, true, http.HandlerFunc(legacy.CreateWechatOrderBoardRefund)},
			{http.MethodGet, "/api/admin/wechat-pay/orders/{order_id}/external-push-deliveries", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListWechatOrderExternalEffects)},
			{http.MethodPost, "/api/admin/wechat-pay/orders/{order_id}/external-push-deliveries/{delivery_id}/retry", authport.CapabilityOrderWrite, true, http.HandlerFunc(legacy.ReviewWechatOrderExternalEffect)},
			{http.MethodGet, legacyImageCollectionPath, authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetImageList)},
			{http.MethodPost, legacyImageCollectionPath, authport.CapabilityMediaImagesWrite, true, http.HandlerFunc(legacy.CreateImage)},
			{http.MethodGet, legacyImageFacetsPath, authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetImageFacets)},
			{http.MethodGet, legacyImageDetailPath, authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetImageDetail)},
			{http.MethodPut, legacyImageDetailPath, authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.UpdateImageMetadata)},
			{http.MethodDelete, legacyImageDetailPath, authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.DeleteImage)},
			{http.MethodGet, legacyImageVariantPath, authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetImageVariant)},
			{http.MethodPost, "/api/admin/image-library/upload", authport.CapabilityMediaImagesWrite, true, http.HandlerFunc(legacy.UploadImage)},
			{http.MethodGet, "/api/admin/group-invite-library", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.ListGroupInvites)},
			{http.MethodPost, "/api/admin/group-invite-library", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.CreateGroupInvite)},
			{http.MethodGet, "/api/admin/group-invite-library/{item_id}", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetGroupInvite)},
			{http.MethodPut, "/api/admin/group-invite-library/{item_id}", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.UpdateGroupInvite)},
			{http.MethodDelete, "/api/admin/group-invite-library/{item_id}", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.ArchiveGroupInvite)},
			{http.MethodGet, "/admin/miniprogram-library", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.MiniProgramLibraryPage)},
			{http.MethodGet, "/api/admin/miniprogram-library", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.ListMiniPrograms)},
			{http.MethodPost, "/api/admin/miniprogram-library", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.CreateMiniProgram)},
			{http.MethodGet, "/api/admin/miniprogram-library/{item_id}", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetMiniProgram)},
			{http.MethodPut, "/api/admin/miniprogram-library/{item_id}", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.UpdateMiniProgram)},
			{http.MethodDelete, "/api/admin/miniprogram-library/{item_id}", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.DeleteMiniProgram)},
			{http.MethodPost, "/api/admin/miniprogram-library/{item_id}/test-resolve", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.TestResolveMiniProgram)},
			{http.MethodGet, legacyQuestionnairePagePath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.QuestionnaireListPage)},
			{http.MethodGet, legacyQuestionnairePagePath + "/ui", authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.QuestionnaireListPage)},
			{http.MethodGet, legacyQuestionnairePreflightPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.QuestionnairePreflight)},
			{http.MethodGet, "/api/admin/questionnaires", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.ListQuestionnaires)},
			{http.MethodPost, "/api/admin/questionnaires", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.CreateQuestionnaire)},
			{http.MethodGet, "/api/admin/questionnaires/{questionnaire_id}", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.GetQuestionnaire)},
			{http.MethodPut, "/api/admin/questionnaires/{questionnaire_id}", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.UpdateQuestionnaire)},
			{http.MethodPatch, "/api/admin/questionnaires/{questionnaire_id}", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.UpdateQuestionnaire)},
			{http.MethodPost, "/api/admin/questionnaires/{questionnaire_id}/duplicate", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.DuplicateQuestionnaire)},
			{http.MethodPost, "/api/admin/questionnaires/{questionnaire_id}/disable", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.SetQuestionnaireDisabled)},
			{http.MethodPost, "/api/admin/questionnaires/{questionnaire_id}/enable", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.SetQuestionnaireDisabled)},
			{http.MethodDelete, "/api/admin/questionnaires/{questionnaire_id}", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.DeleteQuestionnaire)},
			{http.MethodGet, "/api/admin/questionnaires/{questionnaire_id}/results", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.GetQuestionnaireResults)},
			{http.MethodGet, safeadminhttp.ResultsPath, authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.GetQuestionnaireSafeAnalysis)},
			{http.MethodPost, safeadminhttp.ExportPreviewPath, authport.CapabilityQuestionnairesRead, true, http.HandlerFunc(legacy.PreviewQuestionnaireSafeExport)},
			{http.MethodGet, "/api/admin/questionnaires/{questionnaire_id}/submissions", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.ListQuestionnaireSubmissions)},
			{http.MethodGet, "/api/admin/questionnaires/{questionnaire_id}/export", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.ExportQuestionnaireSubmissions)},
			{http.MethodGet, legacyChannelPagePath, authport.CapabilityChannelsRead, false, http.HandlerFunc(legacy.ChannelListPage)},
			{http.MethodGet, "/api/admin/channels", authport.CapabilityChannelsRead, false, http.HandlerFunc(legacy.ListChannels)},
			{http.MethodPost, "/api/admin/channels", authport.CapabilityChannelsWrite, true, http.HandlerFunc(legacy.CreateChannel)},
			{http.MethodGet, "/api/admin/channels/{channel_id}", authport.CapabilityChannelsRead, false, http.HandlerFunc(legacy.GetChannel)},
			{http.MethodPatch, "/api/admin/channels/{channel_id}", authport.CapabilityChannelsWrite, true, http.HandlerFunc(legacy.UpdateChannel)},
			{http.MethodGet, "/api/admin/wecom/tags", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.ListLegacyTags)},
			{http.MethodGet, "/admin/wecom-tags", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.LegacyWecomTagsPage)},
			{http.MethodGet, "/api/admin/wecom/tag-groups", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.ListLegacyTagGroups)},
			{http.MethodPost, "/api/admin/wecom/tag-groups", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.CreateLegacyTagGroup)},
			{http.MethodGet, "/api/admin/wecom/tag-groups/{group_id}", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetLegacyTagGroup)},
			{http.MethodPut, "/api/admin/wecom/tag-groups/{group_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.MutateLegacyTagGroup)},
			{http.MethodPatch, "/api/admin/wecom/tag-groups/{group_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.MutateLegacyTagGroup)},
			{http.MethodDelete, "/api/admin/wecom/tag-groups/{group_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.MutateLegacyTagGroup)},
			{http.MethodPost, "/api/admin/wecom/tags", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.CreateLegacyTag)},
			{http.MethodPut, "/api/admin/wecom/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.MutateLegacyTag)},
			{http.MethodPatch, "/api/admin/wecom/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.MutateLegacyTag)},
			{http.MethodDelete, "/api/admin/wecom/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.MutateLegacyTag)},
			{http.MethodGet, "/api/admin/wecom/tags/live/gate", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetLegacyTagExecutionStatus)},
			{http.MethodPost, "/api/admin/wecom/tags/live/mark", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.MarkLegacyTagLive)},
			{http.MethodPost, "/api/admin/wecom/tags/live/unmark", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.UnmarkLegacyTagLive)},
			{http.MethodPost, "/api/admin/wecom/tags/sync", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.SyncLegacyTags)},
			{http.MethodPost, "/api/admin/wecom/tags/sync-due", authport.CapabilityCustomersWrite, true, http.HandlerFunc(legacy.SyncLegacyTagsDue)},
			{http.MethodGet, "/api/admin/wecom/tags/{tag_id}", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetLegacyTag)},
			{http.MethodGet, "/api/admin/coupons", authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.ListCoupons)},
			{http.MethodPost, "/api/admin/coupons", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.CreateCoupon)},
			{http.MethodGet, "/api/admin/coupons/{coupon_id}", authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.GetCoupon)},
			{http.MethodPut, "/api/admin/coupons/{coupon_id}", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.UpdateCoupon)},
			{http.MethodPost, "/api/admin/coupons/{coupon_id}/publish", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.PublishCoupon)},
			{http.MethodPost, "/api/admin/coupons/{coupon_id}/stop", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.StopCoupon)},
			{http.MethodGet, legacyCouponPagePath, authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.CouponListPage)},
			{http.MethodGet, "/admin/coupons/new", authport.CapabilityCouponsWrite, false, http.HandlerFunc(legacy.CouponNewPage)},
			{http.MethodGet, "/admin/coupons/{coupon_id}/data", authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.CouponDataPage)},
			{http.MethodGet, "/admin/coupons/{coupon_id}/edit", authport.CapabilityCouponsWrite, false, http.HandlerFunc(legacy.CouponEditPage)},
			{http.MethodGet, "/api/admin/coupons/product-options", authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.CouponProductOptions)},
			{http.MethodDelete, "/api/admin/coupons/{coupon_id}", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.CouponDelete)},
			{http.MethodPost, "/api/admin/coupons/{coupon_id}/archive", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.CouponArchive)},
			{http.MethodGet, "/api/admin/coupons/{coupon_id}/claims", authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.CouponClaims)},
			{http.MethodPost, "/api/admin/coupons/{coupon_id}/copy", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.CouponCopy)},
			{http.MethodGet, "/api/admin/coupons/{coupon_id}/share", authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.CouponShare)},
			{http.MethodGet, "/admin/operation-cycles", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.OperationCyclesPage)},
			{http.MethodGet, "/admin/operation-cycles/{strategy_key}", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.OperationCycleStrategyPage)},
			{http.MethodGet, "/admin/operation-cycles/{strategy_key}/runs/{run_key}", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.OperationCycleRunPage)},
			{http.MethodGet, "/api/admin/operation-cycles/action-requests/{request_id}/result", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.GetOperationCycleActionResult)},
			{http.MethodGet, "/api/admin/operation-cycles/runs/{run_key}", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.GetOperationCycleRun)},
			{http.MethodGet, "/api/admin/operation-cycles/strategies", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.ListOperationCycleStrategies)},
			{http.MethodGet, "/api/admin/operation-cycles/strategies/{strategy_key}", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.GetOperationCycleStrategy)},
			{http.MethodPost, "/api/admin/operation-cycles/strategies/{strategy_key}/actions/{action_key}/start", authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.StartOperationCycleAction)},
			{http.MethodGet, "/api/admin/operation-cycles/strategies/{strategy_key}/current-action", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.GetOperationCycleCurrentAction)},
			{http.MethodGet, "/api/admin/operation-cycles/strategies/{strategy_key}/runs", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.ListOperationCycleRuns)},
			{http.MethodGet, "/api/admin/operation-cycles/strategies/{strategy_key}/strategy-change-proposals", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.ListOperationCycleProposals)},
			{http.MethodPost, "/api/admin/operation-cycles/strategy-change-proposals/{proposal_id}/decision", authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.DecideOperationCycleProposal)},
		} {
			if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, route.endpoint); err != nil {
				return nil, err
			}
		}
		adminShell, err := newAdminShellHandler(legacy.auth)
		if err != nil {
			return nil, err
		}
		registerAdminShell := func(pattern string, endpoint http.Handler) error {
			tail, wrapErr := recovery(endpoint)
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = gateway.TimeoutMiddleware(tail)
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = gateway.AccountBudgetMiddleware(tail)
			if wrapErr != nil {
				return wrapErr
			}
			tail = adminShell.Authenticate(tail)
			tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
			if wrapErr != nil {
				return wrapErr
			}
			router.Method(http.MethodGet, pattern, tail)
			return nil
		}
		for _, route := range []struct {
			pattern  string
			endpoint http.Handler
		}{
			{"/admin", http.HandlerFunc(adminShell.Page)},
			{"/admin/logout", http.HandlerFunc(adminShell.LogoutAlias)},
		} {
			if err = registerAdminShell(route.pattern, route.endpoint); err != nil {
				return nil, err
			}
		}
		registerOperation := func(method, pattern string, endpoint http.Handler) error {
			tail, wrapErr := recovery(endpoint)
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = gateway.TimeoutMiddleware(tail)
			if wrapErr != nil {
				return wrapErr
			}
			tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
			if wrapErr != nil {
				return wrapErr
			}
			router.Method(method, pattern, tail)
			return nil
		}
		for _, route := range []struct {
			method, pattern string
			endpoint        http.Handler
		}{
			{http.MethodPost, "/api/operation-cycles/action-requests/claim", http.HandlerFunc(legacy.ClaimOperationCycleAction)},
			{http.MethodPost, "/api/operation-cycles/action-requests/{request_id}/events", http.HandlerFunc(legacy.RecordOperationCycleActionEvent)},
			{http.MethodGet, "/api/operation-cycles/context-index", http.HandlerFunc(legacy.OperationCycleContextIndex)},
			{http.MethodPost, "/api/operation-cycles/reports", http.HandlerFunc(legacy.ReportOperationCycle)},
			{http.MethodPost, "/api/operation-cycles/runner/heartbeat", http.HandlerFunc(legacy.HeartbeatOperationCycleRunner)},
			{http.MethodGet, "/api/operation-cycles/strategies/{strategy_key}/context", http.HandlerFunc(legacy.OperationCycleStrategyContext)},
			{http.MethodPost, "/api/operation-cycles/strategy-change-proposals", http.HandlerFunc(legacy.CreateOperationCycleProposal)},
		} {
			if err = registerOperation(route.method, route.pattern, route.endpoint); err != nil {
				return nil, err
			}
		}
		// Public coupon reads use no human-session middleware. The handlers resolve
		// the opaque payment identity only for the two self-scoped operations.
		for _, route := range []struct {
			method, pattern string
			identity        string
			endpoint        http.Handler
		}{
			{http.MethodGet, "/api/h5/coupons/available", "payment", http.HandlerFunc(legacy.H5AvailableCoupons)},
			{http.MethodGet, "/api/h5/coupons/{public_slug}", "", http.HandlerFunc(legacy.H5Coupon)},
			{http.MethodPost, "/api/h5/coupons/{public_slug}/claim", "payment", http.HandlerFunc(legacy.H5ClaimCoupon)},
			{http.MethodGet, "/api/sidebar/v2/coupons", "sidebar", http.HandlerFunc(legacy.SidebarCoupons)},
			{http.MethodGet, "/c/{public_slug}", "", http.HandlerFunc(legacy.PublicCouponPage)},
		} {
			tail, wrapErr := recovery(route.endpoint)
			if wrapErr != nil {
				return nil, wrapErr
			}
			tail, wrapErr = gateway.TimeoutMiddleware(tail)
			if wrapErr != nil {
				return nil, wrapErr
			}
			if route.identity != "" {
				tail, wrapErr = gateway.AccountBudgetMiddleware(tail)
				if wrapErr != nil {
					return nil, wrapErr
				}
				switch route.identity {
				case "payment":
					tail, wrapErr = legacy.BindCouponPaymentIdentityAccount(tail)
				case "sidebar":
					tail, wrapErr = legacy.BindCouponSidebarGrantAccount(tail)
				default:
					return nil, errInvalidAPIComponent
				}
				if wrapErr != nil {
					return nil, wrapErr
				}
			}
			tail, wrapErr = gateway.RoutePatternMiddleware(route.pattern, tail)
			if wrapErr != nil {
				return nil, wrapErr
			}
			router.Method(route.method, route.pattern, tail)
		}
	}
	domainVerificationRoute, err := recovery(http.HandlerFunc(wrapper.GetDomainVerificationFile))
	if err != nil {
		return nil, err
	}
	domainVerificationRoute, err = gateway.TimeoutMiddleware(domainVerificationRoute)
	if err != nil {
		return nil, err
	}
	domainVerificationRoute, err = gateway.RoutePatternMiddleware("/{filename}", domainVerificationRoute)
	if err != nil {
		return nil, err
	}
	// Register the root-only compatibility route after every concrete route.
	// Chi keeps concrete /healthz, auth, admin, API, OAuth, and callback paths
	// ahead of this one-segment pattern.
	router.Method(http.MethodGet, "/{filename}", domainVerificationRoute)
	notFound, err := recovery(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, nil))
	}))
	if err != nil {
		return nil, err
	}
	methodNotAllowed, err := recovery(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, nil))
	}))
	if err != nil {
		return nil, err
	}
	router.NotFound(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if writeLegacyCustomerPageNotFound(writer, request) {
			return
		}
		notFound.ServeHTTP(writer, request)
	}))
	router.MethodNotAllowed(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if writeIdentityReviewMethodNotAllowed(writer, request) {
			return
		}
		methodNotAllowed.ServeHTTP(writer, request)
	}))
	return gateway.RequestIDMiddleware(legacyCustomerPageNamespaceGuard(router))
}

func (component *apiComponent) Run(ctx context.Context) error {
	if component == nil || component.server == nil || component.pool == nil || component.listen == nil || component.address == "" {
		return errInvalidAPIComponent
	}
	defer component.pool.Close()
	listener, err := component.listen("tcp", component.address)
	if err != nil {
		return err
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- component.server.Serve(listener) }()
	select {
	case err = <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), appruntime.ShutdownGrace-time.Second)
		defer cancel()
		shutdownErr := component.server.Shutdown(shutdownCtx)
		serveErr := <-serveResult
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(shutdownErr, serveErr)
	}
}
