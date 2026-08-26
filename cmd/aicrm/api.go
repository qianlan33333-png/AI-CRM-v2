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
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	confighttp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/http"
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
	eventhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/http"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	externaleffectsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/app"
	externaleffectshttp "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/http"
	externaleffectsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopshttp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/http"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store"
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
	orderhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/http"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
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
	memberapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/app"
	memberhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/http"
	memberstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/store"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
	pushcenterapp "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/app"
	pushcenterstore "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/store"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/http"
	radarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
	releaseapp "github.com/qianlan33333-png/AI-CRM-v2/internal/release/app"
	releasehttp "github.com/qianlan33333-png/AI-CRM-v2/internal/release/http"
	releasestore "github.com/qianlan33333-png/AI-CRM-v2/internal/release/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmenthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/http"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudiencemembers"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	sidebarhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/http"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
	surveyoperationshttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http/operations"
	safeadminhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http/safeadmin"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomcallback "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/callback"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
	groupopsdirectory "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/groupopsdirectory"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
	wecomtag "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
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
	customerIdentity          identityResolveApplication
	customerDetail            *contacthttp.CustomerDetailHandler
	customerDetailReader      customerDetailApplication
	customerSurveyAnswers     surveyport.CustomerSurveyAnswerReader
	customerEvents            *contacthttp.CustomerEventHandler
	customerContext           *customer360http.CustomerContextHandler
	customerChatActivity      *customer360http.CustomerChatActivityHandler
	customerActivityAnalytics *contacthttp.CustomerActivityAnalyticsHandler
	customerMergeHistory      *identityhttp.MergeHistoryHandler
	mutations                 *contacthttp.CustomerMutationHandler
	ownerReassignments        *contacthttp.OwnerReassignmentHandler
	contactPolicy             *contacthttp.ContactPolicyHandler
	customerSafeExports       *contacthttp.CustomerSafeExportHandler
	internalEventSafeExports  *eventhttp.SafeExportHandler
	tags                      *contacthttp.TagCatalogHandler
	localTags                 *contacthttp.LocalTagCatalogHandler
	stages                    *contacthttp.Handler
	segments                  *segmenthttp.CRUDHandler
	products                  *producthttp.Handler
	productLocal              *producthttp.LocalMutationHandler
	productLifecycle          *producthttp.LocalProductLifecycleHandler
	productExternalPush       *producthttp.ExternalPushHandler
	servicePeriodMembers      *memberhttp.Handler
	wechatPaySettlement       *orderhttp.Handler
	commerceRefunds           *orderhttp.CommerceRefundHandler
	sidebar                   *sidebarhttp.Handler
	sidebarActivity           *sidebarhttp.ActivityHandler
	sidebarOAuth              *sidebarhttp.OAuthHandler
	sidebarJSSDK              *sidebarhttp.JSSDKHandler
	surveyPublic              *surveyhttp.PublicHandler
	radarPublic               *radarthttp.PublicHandler
	surveyH5OAuth             *surveyhttp.H5OAuthHandler
	surveyExternalPushDetail  *surveyhttp.ExternalPushDetailHandler
	surveyPushReconcile       *surveyhttp.ExternalPushReconcileHandler

	segmentRefresh  *segmenthttp.RefreshHandler
	identityReviews *identityhttp.ReviewHandler
	identityConsole *identityhttp.ConsoleHandler
	identityIngest  *identityhttp.IngestHandler
	automationRuns  interface {
		List(context.Context, automationstore.TriggerListInput) (automationstore.TriggerListResult, error)
	}
	domainVerification interface {
		Read(string) (string, error)
	}
	legacyHealth             *legacyhealth.Handler
	campaignInitiation       http.Handler
	campaignReview           http.Handler
	outboundCampaignHandoff  *outboundhttp.CampaignHandoffHandler
	outboundCampaignDispatch *outboundhttp.CampaignDispatchHandler
	externalEffectsRuntime   *externaleffectshttp.Handler
	release                  *releasehttp.Handler
	adminOps                 http.Handler
	outboundLegacy           *Handler
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

func (handler *candidateHandler) StartSidebarOAuth(writer http.ResponseWriter, request *http.Request, _ api.StartSidebarOAuthParams) {
	if handler == nil || handler.sidebarOAuth == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.sidebarOAuth.Start(writer, request)
}

func (handler *candidateHandler) CompleteSidebarOAuth(writer http.ResponseWriter, request *http.Request, _ api.CompleteSidebarOAuthParams) {
	if handler == nil || handler.sidebarOAuth == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.sidebarOAuth.Callback(writer, request)
}

func (handler *candidateHandler) GetSidebarAgentConfig(writer http.ResponseWriter, request *http.Request, _ api.GetSidebarAgentConfigParams) {
	if handler == nil || handler.sidebarJSSDK == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.sidebarJSSDK.AgentConfig(writer, request)
}

func (handler *candidateHandler) ListCustomers(writer http.ResponseWriter, request *http.Request, params api.ListCustomersParams) {
	if params.Mobile == nil {
		handler.customers.ListCustomers(writer, request, params)
		return
	}
	handler.listCustomersByMobile(writer, request, params)
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

func (handler *candidateHandler) CreateWechatPayCheckout(writer http.ResponseWriter, request *http.Request, _ api.CreateWechatPayCheckoutParams) {
	handler.wechatPaySettlement.Checkout(writer, request)
}

func (handler *candidateHandler) GetWechatPayCheckout(writer http.ResponseWriter, request *http.Request, merchantOrderNo string) {
	handler.wechatPaySettlement.Get(writer, request, merchantOrderNo)
}

func (handler *candidateHandler) CreateWechatPaySettlementRefund(writer http.ResponseWriter, request *http.Request, orderID int64, _ api.CreateWechatPaySettlementRefundParams) {
	handler.wechatPaySettlement.Refund(writer, request, orderID)
}

func (handler *candidateHandler) ReceiveWechatPayPaymentCallback(writer http.ResponseWriter, request *http.Request, _ api.ReceiveWechatPayPaymentCallbackParams) {
	handler.wechatPaySettlement.PaymentCallback(writer, request)
}

func (handler *candidateHandler) ReceiveWechatPayRefundCallback(writer http.ResponseWriter, request *http.Request, _ api.ReceiveWechatPayRefundCallbackParams) {
	handler.wechatPaySettlement.RefundCallback(writer, request)
}

func (handler *candidateHandler) ReconcileWechatShopRefund(writer http.ResponseWriter, request *http.Request, refundID int64, _ api.ReconcileWechatShopRefundParams) {
	if handler == nil || handler.commerceRefunds == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.commerceRefunds.ReconcileWeChatShopRefund(writer, request, strconv.FormatInt(refundID, 10))
}

func (handler *candidateHandler) ReceiveWechatShopRefundCallback(writer http.ResponseWriter, request *http.Request, _ api.ReceiveWechatShopRefundCallbackParams) {
	if handler == nil || handler.commerceRefunds == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.commerceRefunds.WeChatShopCallback(writer, request)
}

func (handler *candidateHandler) VerifyWechatShopRefundCallbackURL(writer http.ResponseWriter, request *http.Request, _ api.VerifyWechatShopRefundCallbackURLParams) {
	if handler == nil || handler.commerceRefunds == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.commerceRefunds.WeChatShopCallback(writer, request)
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

func (handler *candidateHandler) EnableLegacyWechatPayProduct(writer http.ResponseWriter, request *http.Request, productID api.ProductID, _ api.EnableLegacyWechatPayProductParams) {
	handler.productLifecycle.SetLocalProductEnabled(writer, request, int64(productID), true)
}

func (handler *candidateHandler) DisableLegacyWechatPayProduct(writer http.ResponseWriter, request *http.Request, productID api.ProductID, _ api.DisableLegacyWechatPayProductParams) {
	handler.productLifecycle.SetLocalProductEnabled(writer, request, int64(productID), false)
}

func (handler *candidateHandler) CopyLegacyWechatPayProduct(writer http.ResponseWriter, request *http.Request, productID api.ProductID, _ api.CopyLegacyWechatPayProductParams) {
	handler.productLifecycle.CopyLocalProduct(writer, request, int64(productID))
}

func (handler *candidateHandler) DeleteLegacyWechatPayProduct(writer http.ResponseWriter, request *http.Request, productID api.ProductID, _ api.DeleteLegacyWechatPayProductParams) {
	handler.productLifecycle.DeleteLocalProduct(writer, request, int64(productID))
}

func (handler *candidateHandler) GetLegacyWechatPayProductShare(writer http.ResponseWriter, request *http.Request, productID api.ProductID) {
	handler.productLifecycle.ShareLocalProduct(writer, request, int64(productID))
}

func (handler *candidateHandler) GetWechatPayProductExternalPush(writer http.ResponseWriter, request *http.Request, _ api.ProductID) {
	handler.productExternalPush.ServeHTTP(writer, request)
}

func (handler *candidateHandler) SaveWechatPayProductExternalPush(writer http.ResponseWriter, request *http.Request, _ api.ProductID, _ api.SaveWechatPayProductExternalPushParams) {
	handler.productExternalPush.ServeHTTP(writer, request)
}

func (handler *candidateHandler) QueueWechatPayProductExternalPushTest(writer http.ResponseWriter, request *http.Request, _ api.ProductID, _ api.QueueWechatPayProductExternalPushTestParams) {
	handler.productExternalPush.ServeHTTP(writer, request)
}

func (handler *candidateHandler) GetServicePeriodProductExternalPush(writer http.ResponseWriter, request *http.Request, _ int64) {
	handler.productExternalPush.ServeHTTP(writer, request)
}

func (handler *candidateHandler) SaveServicePeriodProductExternalPush(writer http.ResponseWriter, request *http.Request, _ int64, _ api.SaveServicePeriodProductExternalPushParams) {
	handler.productExternalPush.ServeHTTP(writer, request)
}

func (handler *candidateHandler) QueueServicePeriodProductExternalPushTest(writer http.ResponseWriter, request *http.Request, _ int64, _ api.QueueServicePeriodProductExternalPushTestParams) {
	handler.productExternalPush.ServeHTTP(writer, request)
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

func (handler *candidateHandler) ListServicePeriodMembers(writer http.ResponseWriter, request *http.Request, serviceProductID int64, _ api.ListServicePeriodMembersParams) {
	handler.servicePeriodMembers.List(writer, request, serviceProductID)
}

func (handler *candidateHandler) AddServicePeriodMember(writer http.ResponseWriter, request *http.Request, serviceProductID int64, _ api.AddServicePeriodMemberParams) {
	handler.servicePeriodMembers.Add(writer, request, serviceProductID)
}

func (handler *candidateHandler) ExportServicePeriodMembers(writer http.ResponseWriter, request *http.Request, serviceProductID int64, _ api.ExportServicePeriodMembersParams) {
	handler.servicePeriodMembers.Export(writer, request, serviceProductID)
}

func (handler *candidateHandler) CreateCustomerSafeExport(writer http.ResponseWriter, request *http.Request, params api.CreateCustomerSafeExportParams) {
	handler.customerSafeExports.Create(writer, request, params)
}

func (handler *candidateHandler) GetCustomerSafeExport(writer http.ResponseWriter, request *http.Request, exportID api.CustomerSafeExportID) {
	handler.customerSafeExports.Get(writer, request, exportID)
}

func (handler *candidateHandler) DownloadCustomerSafeExport(writer http.ResponseWriter, request *http.Request, exportID api.CustomerSafeExportID) {
	handler.customerSafeExports.Download(writer, request, exportID)
}

func (handler *candidateHandler) GetServicePeriodMember(writer http.ResponseWriter, request *http.Request, serviceProductID int64, memberRef api.ServicePeriodMemberRef) {
	handler.servicePeriodMembers.Get(writer, request, serviceProductID, string(memberRef))
}

func (handler *candidateHandler) ExpireServicePeriodMember(writer http.ResponseWriter, request *http.Request, serviceProductID int64, memberRef api.ServicePeriodMemberRef, _ api.ExpireServicePeriodMemberParams) {
	handler.servicePeriodMembers.Expire(writer, request, serviceProductID, string(memberRef))
}

func (handler *candidateHandler) UpdateServicePeriodMemberFields(writer http.ResponseWriter, request *http.Request, serviceProductID int64, memberRef api.ServicePeriodMemberRef, _ api.UpdateServicePeriodMemberFieldsParams) {
	handler.servicePeriodMembers.UpdateFields(writer, request, serviceProductID, string(memberRef))
}

func (handler *candidateHandler) RemoveServicePeriodMember(writer http.ResponseWriter, request *http.Request, serviceProductID int64, memberRef api.ServicePeriodMemberRef, _ api.RemoveServicePeriodMemberParams) {
	handler.servicePeriodMembers.Remove(writer, request, serviceProductID, string(memberRef))
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

func (handler *candidateHandler) IngestIdentityEvent(writer http.ResponseWriter, request *http.Request, params api.IngestIdentityEventParams) {
	if handler == nil || handler.identityIngest == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrIdentityIngestFailed))
		return
	}
	handler.identityIngest.IngestIdentityEvent(writer, request, params)
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
			AgentCode: api.TagTriggerV1, RunStatus: api.AutomationTriggerRunRunStatusCompleted,
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
		(params.RunStatus != nil && *params.RunStatus != "" && *params.RunStatus != string(api.AutomationTriggerRunRunStatusCompleted)) ||
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

// These generated operations deliberately delegate to Campaign's narrow
// initiation fragment. The composition root owns authorization and the one
// CSRF validation; the fragment only consumes the bound auth context.
func (handler *candidateHandler) ListCloudCampaignTouchPlans(writer http.ResponseWriter, request *http.Request, _ string, _ api.ListCloudCampaignTouchPlansParams) {
	handler.serveCampaignInitiation(writer, request)
}

func (handler *candidateHandler) CreateCloudCampaignTouchPlan(writer http.ResponseWriter, request *http.Request, _ string, _ api.CreateCloudCampaignTouchPlanParams) {
	handler.serveCampaignInitiation(writer, request)
}

func (handler *candidateHandler) GetCloudCampaignTouchPlan(writer http.ResponseWriter, request *http.Request, _ string, _ string) {
	handler.serveCampaignInitiation(writer, request)
}

func (handler *candidateHandler) serveCampaignInitiation(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.campaignInitiation == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.campaignInitiation.ServeHTTP(writer, request)
}

func (handler *candidateHandler) ListCloudCampaignTouchPlanRecipients(writer http.ResponseWriter, request *http.Request, _ string, _ string, _ api.ListCloudCampaignTouchPlanRecipientsParams) {
	handler.serveCampaignReview(writer, request)
}
func (handler *candidateHandler) GetCloudCampaignTouchPlanRecipient(writer http.ResponseWriter, request *http.Request, _ string, _ string, _ int64) {
	handler.serveCampaignReview(writer, request)
}
func (handler *candidateHandler) GetCloudCampaignTouchPlanReview(writer http.ResponseWriter, request *http.Request, _ string, _ string) {
	handler.serveCampaignReview(writer, request)
}
func (handler *candidateHandler) MutateCloudCampaignTouchPlanReview(writer http.ResponseWriter, request *http.Request, _ string, _ string, _ string, _ api.MutateCloudCampaignTouchPlanReviewParams) {
	handler.serveCampaignReview(writer, request)
}
func (handler *candidateHandler) serveCampaignReview(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.campaignReview == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.campaignReview.ServeHTTP(writer, request)
}

func (handler *candidateHandler) GetOutboundCampaignHandoffSummary(writer http.ResponseWriter, request *http.Request, campaignCode string, planID string) {
	if handler == nil || handler.outboundCampaignHandoff == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.outboundCampaignHandoff.Summary(writer, request, campaignCode, planID)
}

func (handler *candidateHandler) AcceptOutboundCampaignHandoff(writer http.ResponseWriter, request *http.Request, campaignCode string, planID string, _ api.AcceptOutboundCampaignHandoffParams) {
	if handler == nil || handler.outboundCampaignHandoff == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.outboundCampaignHandoff.Accept(writer, request, campaignCode, planID)
}

func (handler *candidateHandler) ReconcileOutboundCampaignHandoff(writer http.ResponseWriter, request *http.Request, campaignCode string, planID string) {
	if handler == nil || handler.outboundCampaignHandoff == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.outboundCampaignHandoff.Reconciliation(writer, request, campaignCode, planID)
}

func (handler *candidateHandler) DispatchOutboundCampaignHandoff(writer http.ResponseWriter, request *http.Request, campaignCode string, planID string, _ api.DispatchOutboundCampaignHandoffParams) {
	if handler == nil || handler.outboundCampaignDispatch == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.outboundCampaignDispatch.Dispatch(writer, request, campaignCode, planID)
}

func (handler *candidateHandler) GetOutboundCampaignDispatchReconciliation(writer http.ResponseWriter, request *http.Request, campaignCode string, planID string) {
	if handler == nil || handler.outboundCampaignDispatch == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.outboundCampaignDispatch.Reconciliation(writer, request, campaignCode, planID)
}

func (handler *candidateHandler) ReconcileOutboundCampaignDispatch(writer http.ResponseWriter, request *http.Request, campaignCode string, planID string, effectID string, _ api.ReconcileOutboundCampaignDispatchParams) {
	if handler == nil || handler.outboundCampaignDispatch == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.outboundCampaignDispatch.ManualReconcile(writer, request, campaignCode, planID, effectID)
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
	identityRepository := identitystore.NewRepository()
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
	var sidebarOAuthClient *wecomclient.HumanOAuthClient
	var sidebarAgentConfigClient *wecomclient.AgentConfigTicketClient
	if config.WeCom.Sidebar.Enabled {
		credentials, credentialErr := wecomclient.NewCredentials(config.WeCom.Sidebar.CorpID, config.WeCom.Sidebar.Secret.Value())
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
		sidebarOAuthClient, err = wecomclient.NewHumanOAuthClient(wecomclient.HumanOAuthConfig{
			BaseURL: wecomclient.ProductionBaseURL, AuthorizeURL: wecomclient.ProductionAuthorizeURL,
			CallbackURL: config.WeCom.Sidebar.CallbackURL, CallbackPath: "/api/sidebar/v2/oauth/callback", CorpID: wecomclient.CorpID(config.WeCom.Sidebar.CorpID),
			HTTPClient: providerHTTP, TokenProvider: tokenProvider,
		})
		if err != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
		sidebarAgentConfigClient, err = wecomclient.NewAgentConfigTicketClient(wecomclient.AgentConfigTicketClientConfig{
			BaseURL: wecomclient.ProductionBaseURL, HTTPClient: providerHTTP, TokenProvider: tokenProvider, Now: time.Now,
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
	customerDetailService := contactapp.NewCustomerDetailService(uow, contactstore.NewCustomerDetailRepository())
	customerDetailHandler, err := contacthttp.NewCustomerDetailHandler(customerDetailService)
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
	ownerReassignmentHandler, err := contacthttp.NewOwnerReassignmentHandler(contactapp.NewOwnerReassignmentService(
		uow, contactstore.NewOwnerReassignmentRepository(), eventstore.NewAppender(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	contactPolicyHandler, err := contacthttp.NewContactPolicyHandler(contactapp.NewContactPolicyService(
		uow, contactstore.NewContactPolicyRepository(), eventstore.NewAppender(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	customerSafeExportHandler, err := contacthttp.NewCustomerSafeExportHandler(contactapp.NewCustomerSafeExportService(
		uow, contactstore.NewCustomerSafeExportRepository(), eventstore.NewAppender(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	internalEventSafeExportHandler, err := eventhttp.NewSafeExportHandler(eventapp.NewInternalEventSafeExportService(
		uow, eventstore.NewInternalEventSafeExportRepository(), eventstore.NewAppender(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	customerEventService := contactapp.NewCustomerEventService(uow, contactstore.NewCustomerEventRepository())
	customerEventHandler, err := contacthttp.NewCustomerEventHandler(customerEventService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	messageArchiveService := wecomapp.NewMessageArchiveService(uow, wecomstore.NewMessageArchiveRepository(), eventstore.NewAppender())
	customer360Reader := contactapp.NewCustomer360ReaderService(
		customerDetailService,
		contactapp.NewCustomerEventService(uow, contactstore.NewCustomerEventRepository()),
	)
	customerContextService := customer360app.NewCustomerContextService(customer360Reader, messageArchiveService)
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
	segmentCRUDService := segmentapp.NewCRUDService(uow, segmentstore.NewCRUDRepository(), eventstore.NewAppender())
	segmentCRUDHandler, err := segmenthttp.NewCRUDHandler(segmentCRUDService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyAIAudienceRepository, err := legacyaudience.NewSQLRepository(legacyAIAudienceSQLProvider{pool: pool})
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyAIAudienceService, err := legacyaudience.NewService(
		uow,
		legacyAIAudienceRepository,
		segmentCRUDService,
		legacyAIAudienceEventAppender{appender: eventstore.NewAppender()},
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyAIAudienceHandler, err := legacyaudience.NewHandler(legacyAIAudienceService, legacyAIAudienceSecurity{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyAIAudienceFragment, err := legacyaudience.NewRouteFragment(legacyAIAudienceHandler)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyAIAudienceMembersRepository := legacyaudiencemembers.NewSQLRepository()
	legacyAIAudienceMembersService, err := legacyaudiencemembers.NewService(
		legacyAIAudienceMembersRepository,
		legacyAIAudienceMembersRepository,
		legacyAIAudienceMembersIdentityReader{reader: identityRepository},
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyAIAudienceMembersHandler, err := legacyaudiencemembers.NewHandler(
		legacyAIAudienceMembersApplication{uow: uow, application: legacyAIAudienceMembersService},
		legacyAIAudienceMembersSecurity{},
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyAIAudienceMembersFragment, err := legacyaudiencemembers.NewRouteFragment(legacyAIAudienceMembersHandler)
	if err != nil {
		pool.Close()
		return nil, err
	}
	productService := productapp.NewService(uow, productstore.NewCatalogRepository(), eventstore.NewAppender())
	mediaRepository := mediastore.NewUploadRepository()
	attachmentRepository := mediastore.NewAttachmentRepository()
	automationRepository := automationstore.NewAgentRepository()
	automationRuleRepository := automationstore.NewRuleRepository()
	channelRepository := contactstore.NewChannelRepository()
	channelStaffDirectory := contactstore.NewStaffDirectoryRepository(pool)
	channelTagCatalog := contactstore.NewTagCatalogRepository()
	radarRepository := radarstore.NewPostgresRepository()
	mediaService := mediaapp.NewService(uow, mediaRepository, eventstore.NewAppender())
	imageDeleteService := mediaapp.NewImageDeleteService(uow, mediaRepository, automationRepository, channelRepository, radarRepository, eventstore.NewAppender())
	groupInviteRepository := mediastore.NewGroupInviteRepository()
	groupInviteService := mediaapp.NewGroupInviteServiceWithChannelReferences(uow, groupInviteRepository, groupInviteRepository, eventstore.NewAppender(), channelRepository)
	miniProgramRepository := mediastore.NewMiniProgramRepository()
	miniProgramService := mediaapp.NewMiniProgramServiceWithChannelReferences(uow, miniProgramRepository, miniProgramRepository, eventstore.NewAppender(), miniProgramRepository, channelRepository)
	surveyService := surveyapp.NewService(uow, surveystore.NewQuestionnaireRepository(), eventstore.NewAppender())
	surveySubmissionRepository := surveystore.NewSubmissionRepository()
	surveySubmissionService := surveyapp.NewSubmissionService(uow, surveySubmissionRepository)
	surveyOperationsHandler := surveyoperationshttp.New(surveyapp.NewOperationsService(
		uow,
		surveystore.NewOperationsRepository(),
		eventstore.NewAppender(),
	))
	surveySafeAdminHandler := safeadminhttp.New(surveyapp.NewSafeAdminService(uow, surveySubmissionRepository))
	surveyTokenKey, surveyCookieKey, surveyAbuseKey := deriveSurveyPublicKeys(config.Survey.PublicKey.Value())
	surveyPublicService := surveyapp.NewPublicService(uow, surveystore.NewPublicRepository(), eventstore.NewAppender(), surveyTokenKey)
	surveyPublicHandler := surveyhttp.NewPublicHandler(
		surveyPublicService,
		surveyCookieKey,
		surveyAbuseKey,
	)
	channelService := contactapp.NewChannelServiceWithReferences(uow, channelRepository, mediaRepository, attachmentRepository, miniProgramRepository, groupInviteRepository, channelTagCatalog, channelStaffDirectory, eventstore.NewAppender())
	channelAcquisitionHandler, err := contacthttp.NewChannelAcquisitionHandler(
		channelService,
		contactapp.NewChannelAcquisitionService(channelService),
		service,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	channelAcquisitionFragment, err := contacthttp.NewChannelAcquisitionRouteFragment(channelAcquisitionHandler)
	if err != nil {
		pool.Close()
		return nil, err
	}
	channelEntrantsCursor, err := contactapp.NewChannelEntrantsCursorCodec(config.Identity.HMACKey.Value())
	if err != nil {
		pool.Close()
		return nil, err
	}
	channelEntrantsService, err := contactapp.NewChannelEntrantsService(
		uow,
		contactstore.NewChannelEntrantsRepository(),
		channelEntrantsCursor,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	channelEntrantsHandler, err := contacthttp.NewChannelEntrantsHandler(channelEntrantsService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	channelEntrantsFragment, err := contacthttp.NewChannelEntrantsRouteFragment(channelEntrantsHandler)
	if err != nil {
		pool.Close()
		return nil, err
	}
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
	automationAgentService := automationapp.NewAgentServiceWithMediaReferences(uow, automationRepository, mediaRepository, attachmentRepository, eventstore.NewAppender())
	automationRuleService := automationapp.NewRuleService(uow, automationRuleRepository)
	audienceOperationMembers := channelStaffDirectory
	legacyAIAudienceConfigurationService, err := legacyaudience.NewLocalConfigurationService(
		uow,
		legacyAIAudienceRepository,
		legacyAIAudienceAutomationAgentReader{store: automationstore.NewAgentRepository()},
		audienceOperationMembers,
		channelStaffDirectory,
		segmentapp.NewAudienceDefinitionEngine(segmentstore.NewRefreshRepository()),
		legacyAIAudienceEventAppender{appender: eventstore.NewAppender()},
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	var legacyAIAudienceConfigurationFragment http.Handler
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
	servicePeriodMemberCursor, err := memberapp.NewCursorCodec(config.Identity.HMACKey.Value())
	if err != nil {
		pool.Close()
		return nil, err
	}
	servicePeriodMemberService, err := memberapp.NewService(
		uow,
		memberstore.NewRepository(),
		eventstore.NewAppender(),
		servicePeriodMemberCursor,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	servicePeriodMemberHandler, err := memberhttp.NewHandler(servicePeriodMemberService)
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
	memberGridManagementService, err := membergrid.NewManagementService(uow, membergrid.NewRepository(), eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	memberGridManagementHandler, err := membergrid.NewManagementHandler(
		memberGridManagementService,
		legacyMemberGridManagementAuthorizer{},
		legacyMemberGridManagementCSRF{},
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	radarService, err := radarapp.NewServiceWithMediaReferences(uow, radarRepository, mediaRepository, attachmentRepository, eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	attachmentService := mediaapp.NewAttachmentServiceWithReferences(uow, attachmentRepository, automationRepository, channelRepository, radarRepository, eventstore.NewAppender())
	contentDeliveryService := mediaapp.NewContentDeliveryService(uow, mediastore.NewContentDeliveryRepository())
	radarFragment, err := radarthttp.NewRouteFragment(radarService, legacyRadarAuthorizer{}, legacyRadarCSRF{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	radarPublicHandler, err := radarthttp.NewPublicHandler(radarService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignAudit, err := campaign.NewEventLogAdapter(eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignRepository := campaignstore.NewRepository()
	campaignService, err := campaign.NewService(uow, campaignRepository, campaignAudit)
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignFragment, err := campaign.NewRouteFragment(campaignService, legacyCampaignAuthorizer{}, legacyCampaignCSRF{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignInitiationAudit, err := campaignstore.NewInitiationEventLogAdapter(eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignEligibility, err := newCampaignContactEligibilityAdapter(contactstore.NewContactPolicyRepository())
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignSources, err := newCampaignSegmentSourceAdapter(segmentstore.NewTouchPlanSnapshotRepository())
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignInitiationService, err := campaignapp.NewService(
		uow, campaignRepository, campaignSources, campaignEligibility, campaignRepository, campaignInitiationAudit,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignInitiationFragment, err := campaign.NewInitiationRouteFragment(campaignInitiationService, legacyCampaignAuthorizer{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignReviewAudit, err := campaignstore.NewReviewHandoffEventLogAdapter(eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignReviewService, err := campaignapp.NewReviewHandoffService(uow, campaignRepository, campaignReviewAudit)
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignReviewFragment, err := campaign.NewReviewHandoffRouteFragment(campaignReviewService, legacyCampaignAuthorizer{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	outboundCampaignSource, err := newOutboundCampaignHandoffSourceAdapter(campaignRepository)
	if err != nil {
		pool.Close()
		return nil, err
	}
	outboundCampaignEvents, err := outboundstore.NewCampaignHandoffEventLogAdapter(eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	outboundCampaignService, err := outboundapp.NewCampaignHandoffService(
		uow, outboundCampaignSource, outboundstore.NewCampaignHandoffRepository(), outboundCampaignEvents,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	outboundCampaignHandler, err := outboundhttp.NewCampaignHandoffHandler(outboundCampaignService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	memberGridManagementFragment, err := membergrid.NewManagementRouteFragment(memberGridManagementHandler)
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
	productLifecycleHandler, err := producthttp.NewLocalProductLifecycleHandler(productapp.NewLocalProductLifecycleService(
		uow, productstore.NewCatalogRepository(), eventstore.NewAppender(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	identityResolver := identityapp.NewResolveService(uow, identityRepository)
	surveyH5OAuthService, err := surveyapp.NewH5OAuthService(oauthStates, surveyapp.DisabledH5OAuthProvider{}, identityResolver)
	if err != nil {
		pool.Close()
		return nil, err
	}
	surveyH5OAuthHandler := surveyhttp.NewH5OAuthHandler(surveyH5OAuthService, surveyCookieKey)
	surveyPublicHandler.IdentityReader = surveyH5OAuthHandler
	legacyUnionIDResolver := identityapp.NewMessageArchiveUnionIDResolver(uow, identityRepository)
	customerIdentityMatcher := identityapp.NewCustomerMatcherService(uow, identityRepository)
	customerAnswerService := surveyapp.NewCustomerAnswerService(
		uow, surveySubmissionRepository, customerIdentityMatcher, config.WeCom.OAuth.CorpID,
	)
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
		resolver: identityResolver,
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
	identityIngestHandler, err := identityhttp.NewIngestHandler(identityapp.NewIngestService(
		uow, identityRepository, contactstore.NewMergePortRepository(), eventstore.NewAppender(), config.Identity.HMACKey.Value(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	domainVerification, err := domainverification.New(config.DomainVerification.Directory)
	if err != nil {
		pool.Close()
		return nil, errInvalidAPIComponent
	}
	releaseHandler, err := releasehttp.NewHandler(releaseapp.NewService(uow, releasestore.NewRepository(pool)))
	if err != nil {
		pool.Close()
		return nil, err
	}
	externalEffectsRuntimeRepository := externaleffectsstore.NewRepository(pool, uow)
	externalEffectsRuntimeService, err := externaleffectsapp.NewService(externalEffectsRuntimeRepository, externalEffectsRuntimeRepository)
	if err != nil {
		pool.Close()
		return nil, err
	}
	externalEffectsRuntime, err := eer.NewService(externalEffectsRuntimeRepository)
	if err != nil {
		pool.Close()
		return nil, err
	}
	channelAcquisitionAssetsFragment := contacthttp.NewDisabledChannelAcquisitionAssetRouteFragment()
	if config.WeCom.CustomerAcquisition.Enabled {
		assetEffects, assetErr := contactapp.NewChannelAcquisitionAssetEERRuntime(externalEffectsRuntime, externalEffectsRuntimeRepository)
		if assetErr != nil {
			pool.Close()
			return nil, assetErr
		}
		assetJobs, assetErr := contactstore.NewChannelAcquisitionAssetRiverJobInserter(pool)
		if assetErr != nil {
			pool.Close()
			return nil, assetErr
		}
		assetRepository := contactstore.NewChannelAcquisitionAssetRepository()
		assetCommands, assetErr := contactapp.NewChannelAcquisitionAssetCommandService(
			uow, assetRepository, assetEffects, assetJobs, config.WeCom.CustomerAcquisition.CorpID,
		)
		if assetErr != nil {
			pool.Close()
			return nil, assetErr
		}
		assetCursor, assetErr := contactapp.NewChannelAcquisitionAssetCursorCodec(config.Identity.HMACKey.Value())
		if assetErr != nil {
			pool.Close()
			return nil, assetErr
		}
		assetQueries, assetErr := contactapp.NewChannelAcquisitionAssetQueryService(uow, assetRepository, assetCursor)
		if assetErr != nil {
			pool.Close()
			return nil, assetErr
		}
		assetHandler, assetErr := contacthttp.NewChannelAcquisitionAssetHandler(assetCommands, assetQueries, service)
		if assetErr != nil {
			pool.Close()
			return nil, assetErr
		}
		channelAcquisitionAssetsFragment, assetErr = contacthttp.NewChannelAcquisitionAssetRouteFragment(assetHandler)
		if assetErr != nil {
			pool.Close()
			return nil, assetErr
		}
	}
	entrantReceiptCursor, err := contactapp.NewChannelAcquisitionEntrantReceiptCursorCodec(config.Identity.HMACKey.Value())
	if err != nil {
		pool.Close()
		return nil, err
	}
	entrantReceiptService, err := contactapp.NewChannelAcquisitionEntrantReceiptService(uow, contactstore.NewChannelAcquisitionEntrantReceiptRepository(), entrantReceiptCursor)
	if err != nil {
		pool.Close()
		return nil, err
	}
	entrantReceiptHandler, err := contacthttp.NewChannelAcquisitionEntrantReceiptHandler(entrantReceiptService, service)
	if err != nil {
		pool.Close()
		return nil, err
	}
	channelAcquisitionEntrantReceiptsFragment, err := contacthttp.NewChannelAcquisitionEntrantReceiptRouteFragment(entrantReceiptHandler)
	if err != nil {
		pool.Close()
		return nil, err
	}
	groupOpsRepository := groupopsstore.NewRepository()
	groupOpsStaffDirectory := contactstore.NewStaffDirectoryRepository(pool)
	var groupOpsDirectorySource groupopsport.GroupDirectorySource
	if config.WeCom.DirectorySync.Enabled {
		credentials, credentialErr := wecomclient.NewCredentials(config.WeCom.OAuth.CorpID, config.WeCom.OAuth.Secret.Value())
		if credentialErr != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
		providerHTTP := &http.Client{Timeout: 5 * time.Second}
		tokens, tokenErr := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
			BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: providerHTTP, Now: time.Now,
		})
		if tokenErr != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
		groupOpsDirectorySource, err = groupopsdirectory.New(groupopsdirectory.Config{
			BaseURL: wecomclient.ProductionBaseURL, HTTPClient: providerHTTP, Token: tokens,
			OwnerStaff:  groupOpsDirectoryOwnerResolver{staff: groupOpsStaffDirectory},
			ActiveStaff: groupOpsDirectoryActiveStaff{staff: groupOpsStaffDirectory},
		})
		if err != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
	}
	groupOpsJobs, err := groupopsstore.NewDispatchJobInserter(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	groupOpsReceipts, err := groupopsstore.NewGroupMessageReceiptStore(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	var groupOpsEvidence groupopsport.ReconciliationEvidenceVerifier
	if config.WeCom.Outbound.Enabled {
		groupOpsEvidence, err = newGroupOpsEvidenceVerifier(config.WeCom.Outbound, &http.Client{Timeout: 5 * time.Second}, time.Now, groupOpsReceipts)
		if err != nil {
			pool.Close()
			return nil, err
		}
	}
	groupOpsRuntime, err := groupopsapp.NewRuntimeService(
		uow,
		groupOpsRepository,
		groupOpsRepository,
		externalEffectsRuntime,
		groupOpsStaffDirectory,
		groupOpsDirectorySource,
		groupOpsSenderResolver{groups: groupOpsRepository, staff: groupOpsStaffDirectory},
		groupOpsJobs,
		groupOpsEvidence,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	productExternalPushHandler, err := producthttp.NewExternalPushHandler(
		productapp.NewCommerceExternalPushService(
			uow,
			productstore.NewCatalogRepository(),
			productstore.NewCommerceExternalPushEERAccepter(externalEffectsRuntime),
		),
		productExternalPushAuthorizer{},
		productExternalPushCSRF{},
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	groupOpsHandler := groupopshttp.NewWithRuntime(
		groupopsapp.NewService(uow, groupOpsRepository, channelStaffDirectory, eventstore.NewAppender()),
		groupOpsRuntime,
		nil,
	)
	legacyAIAudienceConfigurationHandler, err := legacyaudience.NewLocalConfigurationHandler(
		groupOpsOperationMemberApplication{LocalConfigurationApplication: legacyAIAudienceConfigurationService, runtime: groupOpsRuntime},
		legacyAIAudienceSecurity{},
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyAIAudienceConfigurationFragment, err = legacyaudience.NewLocalConfigurationRouteFragment(legacyAIAudienceConfigurationHandler)
	if err != nil {
		pool.Close()
		return nil, err
	}
	automationOutboundMessage, err := automationstore.NewOutboundMessageHandoff(pool, uow, externalEffectsRuntime)
	if err != nil {
		pool.Close()
		return nil, err
	}
	surveyExternalPushService, err := surveyapp.NewExternalPushService(uow, surveystore.NewExternalPushRepository(), externalEffectsRuntime)
	if err != nil {
		pool.Close()
		return nil, err
	}
	surveyPublicService.SetBinder(surveyapp.PublicExternalPushBinder{Push: surveyExternalPushService})
	surveyExternalPushDetailHandler := &surveyhttp.ExternalPushDetailHandler{Application: surveyExternalPushService}
	surveyExternalPushReconcileHandler := &surveyhttp.ExternalPushReconcileHandler{Application: surveyExternalPushService}
	publishedOutboundRepository := mediastore.NewPublishedOutboundRepository()
	publishedOutboundService := mediaapp.NewPublishedOutboundService(uow, publishedOutboundRepository, mediaapp.NewOutboundMediaService(externalEffectsRuntime), publishedOutboundRepository)
	outboundMediaEffectDetailService := mediaapp.NewOutboundMediaEffectDetailService(uow, mediastore.NewContentDeliveryRepository())
	outboundMediaReconcileService := mediaapp.NewOutboundMediaReconcileService(uow, mediastore.NewContentDeliveryRepository(), externalEffectsRuntime)
	campaignDispatchRepository, err := outboundstore.NewCampaignDispatchRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignDispatchService, err := outboundapp.NewCampaignDispatchService(uow, campaignDispatchRepository, externalEffectsRuntime, campaignDispatchRepository, contactstore.NewContactPolicyRepository())
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignDispatchHandler, err := outboundhttp.NewCampaignDispatchHandler(campaignDispatchService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	externalEffectsRuntimeHandler, err := externaleffectshttp.NewHandler(externalEffectsRuntimeService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	financialOrders, err := orderstore.NewFinancialRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	paidBenefits, err := productapp.NewPaidSettlementService(productstore.NewPaidSettlementRepository(), eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	settlementService, err := orderapp.NewSettlementService(uow, financialOrders, productstore.NewCatalogRepository(), paidBenefits, eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	weChatPayVerifier, err := newWeChatPayCallbackRuntime(config.Commerce.WeChatPay)
	if err != nil {
		pool.Close()
		return nil, err
	}
	wechatPaySettlementHandler, err := orderhttp.NewHandler(settlementService, weChatPayVerifier, config.Identity.HMACKey.Value())
	if err != nil {
		pool.Close()
		return nil, err
	}
	commerceRefundRepository, err := orderstore.NewCommerceRefundRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	wechatPayRefundCompatibility, err := orderapp.NewWeChatPayRefundCompatibilityService(uow, commerceRefundRepository, settlementService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	wechatShopRefunds, err := orderapp.NewWeChatShopRefundService(uow, commerceRefundRepository, orderprovider.DisabledWeChatShopRefund{}, eventstore.NewAppender(), orderapp.WithWeChatShopRefundDispatch(config.Commerce.WeChatShopRefund.Enabled))
	if err != nil {
		pool.Close()
		return nil, err
	}
	wechatShopRefundCallbacks, err := newWeChatShopRefundCallbackRuntime(config.Commerce.WeChatShopRefund)
	if err != nil {
		pool.Close()
		return nil, err
	}
	commerceRefundHandler, err := orderhttp.NewCommerceRefundHandler(wechatPayRefundCompatibility, wechatShopRefunds, wechatShopRefundCallbacks)
	if err != nil {
		pool.Close()
		return nil, err
	}
	candidate := &candidateHandler{
		Handler: authHandler, customers: customerHandler,
		customerIdentity: identityResolver, customerDetail: customerDetailHandler, customerDetailReader: customerDetailService,
		customerSurveyAnswers: customerAnswerService, customerEvents: customerEventHandler, customerContext: customerContextHandler,
		customerChatActivity: customerChatActivityHandler, customerActivityAnalytics: customerActivityAnalyticsHandler,
		customerMergeHistory: customerMergeHistoryHandler,
		mutations:            mutationHandler, ownerReassignments: ownerReassignmentHandler, contactPolicy: contactPolicyHandler, customerSafeExports: customerSafeExportHandler, internalEventSafeExports: internalEventSafeExportHandler,
		tags: tagCatalogHandler, localTags: localTagCatalogHandler, stages: stageHandler,
		segments:             segmentCRUDHandler,
		products:             productHandler,
		productLocal:         productLocalHandler,
		productLifecycle:     productLifecycleHandler,
		productExternalPush:  productExternalPushHandler,
		servicePeriodMembers: servicePeriodMemberHandler,
		wechatPaySettlement:  wechatPaySettlementHandler,
		commerceRefunds:      commerceRefundHandler,
		surveyPublic:         surveyPublicHandler,
		radarPublic:          radarPublicHandler,
		surveyH5OAuth:        surveyH5OAuthHandler,
		segmentRefresh:       segmentRefreshHandler,
		identityReviews:      identityReviewHandler,
		identityConsole:      identityConsoleHandler,
		identityIngest:       identityIngestHandler,
		automationRuns:       automationstore.NewRepository(pool),
		domainVerification:   domainVerification,
		legacyHealth: legacyhealth.NewHandler(legacyhealth.NewQuery(legacyhealth.RuntimeSnapshot{
			DatabaseIsPostgres:                  config.LegacyHealth.DatabaseIsPostgres,
			ProductionEnvironment:               config.LegacyHealth.ProductionEnvironment,
			SecretKeyPresent:                    config.LegacyHealth.SecretKeyPresent,
			WeChatShopCallbackTokenPresent:      config.LegacyHealth.WeChatShopCallbackTokenPresent,
			AllowMissingWeChatShopCallbackToken: config.LegacyHealth.AllowMissingWeChatShopCallbackToken,
		})),
		campaignInitiation:       campaignInitiationFragment,
		campaignReview:           campaignReviewFragment,
		outboundCampaignHandoff:  outboundCampaignHandler,
		outboundCampaignDispatch: campaignDispatchHandler,
		externalEffectsRuntime:   externalEffectsRuntimeHandler,
		release:                  releaseHandler,
	}
	candidate.surveyExternalPushDetail = surveyExternalPushDetailHandler
	candidate.surveyPushReconcile = surveyExternalPushReconcileHandler
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
	sidebarCorpID := config.WeCom.OAuth.CorpID
	if config.WeCom.Sidebar.Enabled {
		sidebarCorpID = config.WeCom.Sidebar.CorpID
	}
	sidebarService, err := sidebarapp.NewService(
		sidebarCorpReader{settings: configManager, fallback: sidebarCorpID, fallbackAuthoritative: config.WeCom.Sidebar.Enabled},
		identityResolver,
		contactapp.NewSidebarProfileService(uow, contactstore.NewSidebarProfileRepository(), eventstore.NewAppender()),
		customerAnswerService,
		orderapp.NewService(uow, orderstore.NewRepository(), contactstore.NewCustomerDetailRepository(), productstore.NewCatalogRepository()),
		sidebarMemberAdapter{source: servicePeriodMemberService},
		mediaService,
		config.Identity.HMACKey.Value(),
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	candidate.sidebar, err = sidebarhttp.NewHandler(sidebarService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	sidebarActivityService, err := sidebarapp.NewActivityService(customer360Reader, customerContextService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	candidate.sidebarActivity, err = sidebarhttp.NewActivityHandler(candidate.sidebar, sidebarActivityService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	var sidebarOAuthService *sidebarapp.OAuthGrantService
	if sidebarOAuthClient != nil {
		sidebarOAuthService, err = sidebarapp.NewOAuthGrantService(oauthStates, sidebarOAuthProvider{client: sidebarOAuthClient}, service, service, sidebarService, config.Identity.HMACKey.Value(), sidebarapp.OAuthGrantOptions{})
		if err != nil {
			pool.Close()
			return nil, err
		}
	}
	candidate.sidebarOAuth = sidebarhttp.NewOAuthHandler(sidebarOAuthService, authhttp.WriteBrowserSession)
	var sidebarTicketProvider sidebarapp.AgentConfigTicketProvider
	if sidebarAgentConfigClient != nil {
		sidebarTicketProvider = sidebarAgentConfigTicketProvider{client: sidebarAgentConfigClient}
	}
	sidebarJSSDKService, err := sidebarapp.NewJSSDKService(sidebarapp.JSSDKServiceConfig{
		Enabled: config.WeCom.Sidebar.Enabled, CorpID: config.WeCom.Sidebar.CorpID, AgentID: config.WeCom.Sidebar.AgentID, AllowedHosts: config.WeCom.Sidebar.AllowedHosts,
	}, sidebarTicketProvider, sidebarapp.JSSDKOptions{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	candidate.sidebarJSSDK = sidebarhttp.NewJSSDKHandler(sidebarJSSDKService)
	setupWizardService, err := configapp.NewSetupWizardService(configManager, configapp.SetupWizardSecretConfigured{
		WeComSecret:         config.WeCom.OAuth.Enabled,
		WeComCallbackToken:  config.WeCom.Callback.Enabled,
		WeComCallbackAESKey: config.WeCom.Callback.Enabled,
	})
	if err != nil {
		pool.Close()
		return nil, err
	}
	setupWizardHandler, err := confighttp.NewHandler(setupWizardService)
	if err != nil {
		pool.Close()
		return nil, err
	}
	settingsService := configapp.NewSettingsCompatibilityService(uow, configRepository, configManager, configapp.SecretConfiguredSnapshot{
		DatabaseURL: true, WeComSecret: config.WeCom.OAuth.Enabled,
		WeComCallbackToken: config.WeCom.Callback.Enabled, WeComCallbackAESKey: config.WeCom.Callback.Enabled,
		AuthJWTSecret: config.APIClient.JWTSecret.Configured(),
	})
	adminOpsService := adminopsapp.NewService(uow, adminopsstore.NewRepository())
	var externalCustomerRead *legacyExternalCustomerReadHandler
	weComIdentityCorpID := config.WeCom.OAuth.CorpID
	if weComIdentityCorpID == "" {
		weComIdentityCorpID = config.WeCom.Callback.CorpID
	}
	var weComTagEffects *wecomtag.Service
	if weComIdentityCorpID != "" {
		weComTagJobs, tagErr := wecomtag.NewRiverJobInserter(pool)
		if tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
		weComTagEffects, tagErr = wecomtag.NewService(
			uow, wecomstore.NewTagEffectRepository(pool), externalEffectsRuntime, weComTagJobs, weComIdentityCorpID,
		)
		if tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
	}
	var serviceAuthenticator operationServiceAuthenticator
	if config.APIClient.JWTSecret.Configured() {
		serviceAuthenticator = newAPIClientJWTAuthenticator(adminOpsService, config.APIClient.JWTSecret.Value())
	}
	externalCustomerRead, err = newLegacyExternalCustomerReadHandler(
		customerService, customerDetailService, customerEventService, customerContextService,
		identityResolver, legacyUnionIDResolver, weComIdentityCorpID, serviceAuthenticator,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyHandler, err := NewHandlerWithAll(
		service, customerService,
		customerDetailService,
		identityResolver, config.WeCom.OAuth.CorpID,
		outboundQueryService,
		outboundapp.NewCancelService(uow, outboundControlRepository, eventstore.NewAppender()),
		outboundapp.NewManualRetryService(uow, outboundControlRepository, eventstore.NewAppender()),
		productService, mediaService, groupInviteService, miniProgramService, surveyService, channelService, couponService, legacyTagService, settingsService,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	legacyHandler.setupWizard = setupWizardHandler
	if weComTagEffects != nil {
		legacyHandler.legacyTagSync = &legacyTagSyncEffectBridge{legacy: legacyTagSyncService, effects: weComTagEffects}
	}
	legacyHandler.servicePeriod = servicePeriodHandler
	legacyHandler.memberGrid = memberGridFragment
	legacyHandler.memberGridManagement = memberGridManagementFragment
	legacyHandler.radar = radarFragment
	legacyHandler.campaign = campaignFragment
	legacyHandler.aiAudience = legacyAIAudienceFragment
	legacyHandler.aiAudienceMembers = legacyAIAudienceMembersFragment
	legacyHandler.aiAudienceConfiguration = legacyAIAudienceConfigurationFragment
	legacyHandler.channelEntrants = channelEntrantsFragment
	legacyHandler.channelAcquisition = channelAcquisitionFragment
	legacyHandler.channelAcquisitionAsset = channelAcquisitionAssetsFragment
	legacyHandler.entrantReceipts = channelAcquisitionEntrantReceiptsFragment
	legacyHandler.imageDeletes = imageDeleteService
	legacyHandler.attachments = attachmentService
	legacyHandler.contentDelivery = contentDeliveryService
	legacyHandler.outboundMediaAccepted = publishedOutboundService
	legacyHandler.outboundMediaDetail = outboundMediaEffectDetailService
	legacyHandler.outboundMediaReconcile = outboundMediaReconcileService
	if weComTagEffects != nil {
		legacyHandler.legacyTagLive = &legacyTagLiveEffectBridge{legacy: legacyTagLiveService, effects: weComTagEffects}
	}
	legacyHandler.legacyTagStatus = legacyTagStatusService
	legacyHandler.wecomTagEffects = weComTagEffects
	legacyHandler.adminOps = adminOpsService
	legacyHandler.externalCustomerRead = externalCustomerRead
	candidate.adminOps = http.HandlerFunc(legacyHandler.AdminOps)
	candidate.outboundLegacy = legacyHandler
	legacyHandler.runtimeConfig = runtimeConfigDeclarationFromConfig(config)
	legacyHandler.orders = orderapp.NewService(
		uow, orderstore.NewRepository(), contactstore.NewCustomerDetailRepository(), productstore.NewCatalogRepository(),
	)
	legacyHandler.orderBoard = orderapp.NewBoardService(uow, orderstore.NewRepository(), eventstore.NewAppender())
	legacyHandler.couponBoard = couponService
	legacyHandler.automationAgents = automationAgentService
	legacyHandler.automationRules = automationRuleService
	legacyHandler.automationRuleRuns = automationRuleRepository
	legacyHandler.automationRuleReconcile = automationOutboundMessage
	legacyHandler.messageArchive = messageArchiveService
	legacyHandler.messageArchiveUnionID = legacyUnionIDResolver
	legacyHandler.operationCycles = operationapp.NewService(uow, operationstore.NewRepository(), eventstore.NewAppender(), deliveryProducer)
	legacyHandler.pushCenter = pushcenterapp.NewService(uow, pushcenterstore.NewRepository())
	legacyHandler.externalEffects = externalEffectsHandler
	legacyHandler.surveySubmissions = surveySubmissionService
	legacyHandler.surveySafeAdmin = surveySafeAdminHandler
	legacyHandler.surveyOperations = surveyOperationsHandler
	legacyHandler.groupOps = groupOpsHandler
	legacyHandler.executionRuntime = adminopsapp.NewExecutionRuntimeService(emptyExecutionRuntimeReader{})
	hxcStaffDirectory := audienceOperationMembers
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
	var callbackDispatcher wecomcallback.MessageDispatcher
	if config.WeCom.Callback.Enabled {
		inboundJobs, jobErr := wecomstore.NewRiverJobInserter(pool)
		if jobErr != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
		entrantService, entrantErr := wecomapp.NewChannelAcquisitionEntrantService(
			uow,
			contactstore.NewChannelAcquisitionAssetCorrelationRepository(pool),
			identitystore.NewRepository(),
			contactstore.NewChannelAcquisitionEntrantRepository(),
		)
		if entrantErr != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
		inboundService, inboundErr := wecomapp.NewInboundServiceWithEntrants(
			uow, wecomstore.NewInboundRepository(), inboundJobs, nil, entrantService,
			config.WeCom.Callback.CorpID, time.Now,
		)
		if inboundErr != nil {
			pool.Close()
			return nil, errInvalidAPIComponent
		}
		callbackDispatcher = inboundService
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
	externalEffectsRuntimeDiagnostics := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if concrete, ok := candidate.(*candidateHandler); ok && concrete.externalEffectsRuntime != nil {
			wrapper.GetExternalEffectsDiagnostics(writer, request)
			return
		}
		if legacy != nil && legacy.externalEffects != nil {
			legacy.ExternalEffectsDiagnostics(writer, request)
			return
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
	})
	registerPublicProtocol := func(method, pattern string, endpoint http.Handler) error {
		var allowed http.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			defer func() {
				if recover() == nil {
					return
				}
				logger.Error("public protocol handler panic")
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
		methodGuard, wrapErr := gateway.RoutePatternMiddleware(pattern, publicProtocolExactMethod(method, endpoint))
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
		{http.MethodGet, "/api/h5/surveys/oauth/start", http.HandlerFunc(wrapper.StartSurveyH5OAuth)},
		{http.MethodGet, "/api/h5/surveys/oauth/callback", http.HandlerFunc(wrapper.CallbackSurveyH5OAuth)},
		{http.MethodGet, "/api/sidebar/v2/oauth/start", http.HandlerFunc(wrapper.StartSidebarOAuth)},
		{http.MethodGet, "/api/sidebar/v2/oauth/callback", http.HandlerFunc(wrapper.CompleteSidebarOAuth)},
		{http.MethodGet, "/api/sidebar/v2/jssdk/agent-config", http.HandlerFunc(wrapper.GetSidebarAgentConfig)},
	} {
		if err = registerPublicProtocol(route.method, route.pattern, route.endpoint); err != nil {
			return nil, err
		}
	}
	if legacy != nil {
		if err = registerPublicProtocol(http.MethodGet, "/api/identity/resolve", http.HandlerFunc(legacy.ResolveExternalIdentity)); err != nil {
			return nil, err
		}
	}
	if concrete, ok := candidate.(*candidateHandler); ok && concrete.radarPublic != nil {
		for _, route := range []struct {
			method, pattern string
			endpoint        http.Handler
		}{
			{http.MethodGet, radarthttp.PublicRedirectPattern, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				concrete.radarPublic.Redirect(writer, request, chi.URLParam(request, "code"))
			})},
			{http.MethodPost, radarthttp.PublicEventPattern, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				concrete.radarPublic.RecordEvent(writer, request, chi.URLParam(request, "code"))
			})},
		} {
			if err = registerPublicProtocol(route.method, route.pattern, route.endpoint); err != nil {
				return nil, err
			}
		}
	}
	if legacy != nil && legacy.groupOps != nil {
		for _, route := range []struct {
			method, pattern string
			endpoint        http.Handler
		}{
			{http.MethodPost, groupopshttp.BroadcastPath, http.HandlerFunc(legacy.groupOps.Broadcast)},
			{http.MethodPost, groupopshttp.WebhookPath, http.HandlerFunc(legacy.groupOps.Webhook)},
		} {
			if err = registerPublicProtocol(route.method, route.pattern, route.endpoint); err != nil {
				return nil, err
			}
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
	registerSidebarActivityRead := func(pattern string, endpoint http.Handler) error {
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
		tail, wrapErr = authHandler.Authorize(authport.CapabilityCustomersRead, tail)
		if wrapErr != nil {
			return wrapErr
		}
		tail = authHandler.Authenticate(tail)
		tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
		if wrapErr != nil {
			return wrapErr
		}
		methodRouter := chi.NewRouter()
		methodRouter.MethodNotAllowed(http.HandlerFunc(writeSidebarActivityMethodNotAllowed))
		methodRouter.Method(http.MethodGet, pattern, tail)
		router.Handle(pattern, methodRouter)
		return nil
	}
	registerOptionalSidebar := func(pattern string, endpoint http.Handler) error {
		tail, wrapErr := recovery(endpoint)
		if wrapErr != nil {
			return wrapErr
		}
		tail, wrapErr = gateway.TimeoutMiddleware(tail)
		if wrapErr != nil {
			return wrapErr
		}
		tail, wrapErr = authHandler.AuthorizeOptional(authport.CapabilityCustomersRead, tail)
		if wrapErr != nil {
			return wrapErr
		}
		tail = authHandler.AuthenticateOptional(tail)
		tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
		if wrapErr != nil {
			return wrapErr
		}
		router.Method(http.MethodPost, pattern, tail)
		return nil
	}
	registerPublic := func(method, pattern string, endpoint http.Handler) error {
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
	if err = registerOptionalSidebar("/api/sidebar/context-token", http.HandlerFunc(wrapper.MintSidebarContext)); err != nil {
		return nil, err
	}
	if err = registerPublic(http.MethodPost, orderhttp.PaymentCallbackPath, http.HandlerFunc(wrapper.ReceiveWechatPayPaymentCallback)); err != nil {
		return nil, err
	}
	if err = registerPublic(http.MethodPost, orderhttp.RefundCallbackPath, http.HandlerFunc(wrapper.ReceiveWechatPayRefundCallback)); err != nil {
		return nil, err
	}
	if err = registerPublic(http.MethodGet, orderhttp.WeChatShopCallbackPath, http.HandlerFunc(wrapper.VerifyWechatShopRefundCallbackURL)); err != nil {
		return nil, err
	}
	if err = registerPublic(http.MethodPost, orderhttp.WeChatShopCallbackPath, http.HandlerFunc(wrapper.ReceiveWechatShopRefundCallback)); err != nil {
		return nil, err
	}
	if err = registerSidebarActivityRead("/api/sidebar/v2/timeline", http.HandlerFunc(wrapper.ListSidebarTimeline)); err != nil {
		return nil, err
	}
	if err = registerSidebarActivityRead("/api/sidebar/v2/chat-activity", http.HandlerFunc(wrapper.ListSidebarChatActivity)); err != nil {
		return nil, err
	}
	internalEventSafeExportEndpoint := func(method func(http.ResponseWriter, *http.Request)) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			concrete, ok := candidate.(*candidateHandler)
			if !ok || concrete.internalEventSafeExports == nil {
				platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
				return
			}
			method(writer, request)
		})
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
		{http.MethodGet, "/api/v1/contact-owner-reassignments/template", authport.CapabilityContactOwnerReassignment, false, http.HandlerFunc(wrapper.DownloadContactOwnerReassignmentTemplate)},
		{http.MethodPost, "/api/v1/contact-owner-reassignments/previews", authport.CapabilityContactOwnerReassignment, true, http.HandlerFunc(wrapper.CreateContactOwnerReassignmentPreview)},
		{http.MethodGet, "/api/v1/contact-owner-reassignments/previews/{preview_id}", authport.CapabilityContactOwnerReassignment, false, http.HandlerFunc(wrapper.GetContactOwnerReassignmentPreview)},
		{http.MethodPost, "/api/v1/contact-owner-reassignments/previews/{preview_id}/execute", authport.CapabilityContactOwnerReassignment, true, http.HandlerFunc(wrapper.ExecuteContactOwnerReassignmentPreview)},
		{http.MethodGet, "/api/v1/contact-owner-reassignments/previews/{preview_id}/errors.csv", authport.CapabilityContactOwnerReassignment, false, http.HandlerFunc(wrapper.DownloadContactOwnerReassignmentErrors)},
		{http.MethodGet, "/api/v1/contact-owner-reassignments/previews/{preview_id}/results.csv", authport.CapabilityContactOwnerReassignment, false, http.HandlerFunc(wrapper.DownloadContactOwnerReassignmentResults)},
		{http.MethodGet, "/api/sidebar/v2/workbench", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.GetSidebarWorkbench)},
		{http.MethodPut, "/api/sidebar/v2/profile", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.UpdateSidebarProfile)},
		{http.MethodGet, "/api/sidebar/v2/questionnaires", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListSidebarQuestionnaires)},
		{http.MethodGet, "/api/sidebar/v2/orders", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListSidebarOrders)},
		{http.MethodGet, "/api/sidebar/v2/periodic-orders", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListSidebarPeriodicOrders)},
		{http.MethodPut, "/api/sidebar/v2/periodic-orders/{service_product_id}/members/{member_ref}/remark", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.UpdateSidebarPeriodicRemark)},
		{http.MethodGet, "/api/sidebar/v2/materials", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListSidebarMaterials)},
		{http.MethodGet, "/api/sidebar/v2/materials/image/{image_id}/thumbnail", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.GetSidebarMaterialThumbnailStatus)},
		{http.MethodGet, "/api/v1/customers", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListCustomers)},
		{http.MethodGet, "/api/v1/admin/release-candidates", authport.CapabilityReleaseRead, false, http.HandlerFunc(wrapper.ListReleaseCandidates)},
		{http.MethodPost, "/api/v1/admin/release-candidates", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.RegisterReleaseCandidate)},
		{http.MethodGet, "/api/v1/admin/release-candidates/{candidate_id}", authport.CapabilityReleaseRead, false, http.HandlerFunc(wrapper.GetReleaseCandidate)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/prerequisites", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.RecordReleasePrerequisite)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/prepare", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.PrepareReleaseCandidate)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/cutover/start", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.StartReleaseCutover)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/cutover/restart", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.RestartReleaseCutover)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/cutover/steps/{step}/complete", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.CompleteReleaseCutoverStep)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/activate", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.ActivateReleaseCandidate)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/rollback-checks", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.RecordReleaseRollbackCheck)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/rollback/request", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.RequestReleaseRollback)},
		{http.MethodPost, "/api/v1/admin/release-candidates/{candidate_id}/rollback/complete", authport.CapabilityReleaseManage, true, http.HandlerFunc(wrapper.CompleteReleaseRollback)},
		{http.MethodGet, "/api/admin/external-effects", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.ListExternalEffectsRuntime)},
		{http.MethodGet, "/api/admin/external-effects/{effect_id}", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.GetExternalEffectRuntime)},
		{http.MethodPost, "/api/admin/external-effects/{effect_id}/cancel", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.CancelExternalEffectRuntime)},
		{http.MethodPost, "/api/admin/external-effects/{effect_id}/retry", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.RetryExternalEffectRuntime)},
		{http.MethodPost, "/api/admin/external-effects/{effect_id}/reconcile", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.ReconcileExternalEffectRuntime)},
		{http.MethodPost, "/api/v1/customer-exports", authport.CapabilityCustomersRead, true, http.HandlerFunc(wrapper.CreateCustomerSafeExport)},
		{http.MethodGet, "/api/v1/customer-exports/{export_id}", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.GetCustomerSafeExport)},
		{http.MethodGet, "/api/v1/customer-exports/{export_id}/download", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.DownloadCustomerSafeExport)},
		{http.MethodPost, eventhttp.SafeExportPath, authport.CapabilityAdminRead, true, internalEventSafeExportEndpoint(func(writer http.ResponseWriter, request *http.Request) {
			candidate.(*candidateHandler).internalEventSafeExports.Create(writer, request)
		})},
		{http.MethodGet, eventhttp.SafeExportPath + "/{export_id}", authport.CapabilityAdminRead, false, internalEventSafeExportEndpoint(func(writer http.ResponseWriter, request *http.Request) {
			candidate.(*candidateHandler).internalEventSafeExports.Get(writer, request)
		})},
		{http.MethodGet, eventhttp.SafeExportPath + "/{export_id}/download", authport.CapabilityAdminRead, false, internalEventSafeExportEndpoint(func(writer http.ResponseWriter, request *http.Request) {
			candidate.(*candidateHandler).internalEventSafeExports.Download(writer, request)
		})},
		{http.MethodGet, "/api/v1/customers/{customer_id}", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.GetCustomer)},
		{http.MethodPatch, "/api/v1/customers/{customer_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.UpdateCustomer)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/events", authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(wrapper.ListCustomerEvents)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/context", authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(wrapper.GetCustomerContext)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/merge-history", authport.CapabilityIdentityReviewRead, false, http.HandlerFunc(wrapper.ListCustomerMergeHistory)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/chat-activity", authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(wrapper.ListCustomerChatActivity)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/activity-analytics", authport.CapabilityCustomerEventsRead, false, http.HandlerFunc(wrapper.GetCustomerActivityAnalytics)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/contact-policy", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.GetCustomerContactPolicy)},
		{http.MethodPut, "/api/v1/customers/{customer_id}/contact-policy", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.PutCustomerContactPolicy)},
		{http.MethodDelete, "/api/v1/customers/{customer_id}/contact-policy", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.DeleteCustomerContactPolicy)},
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
		{http.MethodPost, orderhttp.CheckoutPath, authport.CapabilityOrderWrite, true, http.HandlerFunc(wrapper.CreateWechatPayCheckout)},
		{http.MethodGet, orderhttp.CheckoutPath + "/{merchant_order_no}", authport.CapabilityOrderRead, false, http.HandlerFunc(wrapper.GetWechatPayCheckout)},
		{http.MethodPost, "/api/v1/wechat-pay/orders/{order_id}/refunds", authport.CapabilityOrderWrite, true, http.HandlerFunc(wrapper.CreateWechatPaySettlementRefund)},
		{http.MethodPut, "/api/v1/products/{product_id}", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.UpdateProduct)},
		{http.MethodPost, "/api/admin/wechat-pay/products/{product_id}/enable", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.EnableLegacyWechatPayProduct)},
		{http.MethodPost, "/api/admin/wechat-pay/products/{product_id}/disable", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.DisableLegacyWechatPayProduct)},
		{http.MethodPost, "/api/admin/wechat-pay/products/{product_id}/copy", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.CopyLegacyWechatPayProduct)},
		{http.MethodDelete, "/api/admin/wechat-pay/products/{product_id}", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.DeleteLegacyWechatPayProduct)},
		{http.MethodGet, "/api/admin/wechat-pay/products/{product_id}/share", authport.CapabilityProductsRead, false, http.HandlerFunc(wrapper.GetLegacyWechatPayProductShare)},
		{http.MethodGet, producthttp.WeChatPayExternalPushPathPattern, authport.CapabilityProductsRead, false, http.HandlerFunc(wrapper.GetWechatPayProductExternalPush)},
		{http.MethodPut, producthttp.WeChatPayExternalPushPathPattern, authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.SaveWechatPayProductExternalPush)},
		{http.MethodPost, producthttp.WeChatPayExternalPushPathPattern + "/test", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.QueueWechatPayProductExternalPushTest)},
		{http.MethodGet, producthttp.ServicePeriodExternalPushPathPattern, authport.CapabilityProductsRead, false, http.HandlerFunc(wrapper.GetServicePeriodProductExternalPush)},
		{http.MethodPut, producthttp.ServicePeriodExternalPushPathPattern, authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.SaveServicePeriodProductExternalPush)},
		{http.MethodPost, producthttp.ServicePeriodExternalPushPathPattern + "/test", authport.CapabilityProductsWrite, true, http.HandlerFunc(wrapper.QueueServicePeriodProductExternalPushTest)},
		{http.MethodGet, "/api/v1/products/{product_id}/local-entitlements", authport.CapabilityEntitlementsRead, false, http.HandlerFunc(wrapper.ListProductLocalEntitlements)},
		{http.MethodPost, "/api/v1/products/{product_id}/local-entitlements", authport.CapabilityEntitlementsWrite, true, http.HandlerFunc(wrapper.GrantProductLocalEntitlement)},
		{http.MethodGet, "/api/v1/product-entitlements/{entitlement_id}", authport.CapabilityEntitlementsRead, false, http.HandlerFunc(wrapper.GetProductLocalEntitlement)},
		{http.MethodPost, "/api/v1/product-entitlements/{entitlement_id}/revoke", authport.CapabilityEntitlementsWrite, true, http.HandlerFunc(wrapper.RevokeProductLocalEntitlement)},
		{http.MethodGet, "/api/admin/service-period-products/{service_product_id}/members", authport.CapabilityEntitlementsRead, false, http.HandlerFunc(wrapper.ListServicePeriodMembers)},
		{http.MethodPost, "/api/admin/service-period-products/{service_product_id}/members", authport.CapabilityEntitlementsWrite, true, http.HandlerFunc(wrapper.AddServicePeriodMember)},
		{http.MethodPost, "/api/admin/service-period-products/{service_product_id}/members/export", authport.CapabilityEntitlementsRead, true, http.HandlerFunc(wrapper.ExportServicePeriodMembers)},
		{http.MethodGet, "/api/admin/service-period-products/{service_product_id}/members/{member_ref}", authport.CapabilityEntitlementsRead, false, http.HandlerFunc(wrapper.GetServicePeriodMember)},
		{http.MethodPut, "/api/admin/service-period-products/{service_product_id}/members/{member_ref}/fields", authport.CapabilityEntitlementsWrite, true, http.HandlerFunc(wrapper.UpdateServicePeriodMemberFields)},
		{http.MethodPost, "/api/admin/service-period-products/{service_product_id}/members/{member_ref}/expire", authport.CapabilityEntitlementsWrite, true, http.HandlerFunc(wrapper.ExpireServicePeriodMember)},
		{http.MethodPost, "/api/admin/service-period-products/{service_product_id}/members/{member_ref}/remove", authport.CapabilityEntitlementsWrite, true, http.HandlerFunc(wrapper.RemoveServicePeriodMember)},
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
		{http.MethodGet, campaign.RoutePrefix + "/{campaign_code}/touch-plans", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.ListCloudCampaignTouchPlans)},
		{http.MethodPost, campaign.RoutePrefix + "/{campaign_code}/touch-plans", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.CreateCloudCampaignTouchPlan)},
		{http.MethodGet, campaign.RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.GetCloudCampaignTouchPlan)},
		{http.MethodGet, campaign.RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/review", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.GetCloudCampaignTouchPlanReview)},
		{http.MethodGet, campaign.RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/recipients", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.ListCloudCampaignTouchPlanRecipients)},
		{http.MethodGet, campaign.RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/recipients/{customer_id}", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.GetCloudCampaignTouchPlanRecipient)},
		{http.MethodPost, campaign.RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/review/{operation}", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.MutateCloudCampaignTouchPlanReview)},
		{http.MethodGet, "/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.GetOutboundCampaignHandoffSummary)},
		{http.MethodPost, "/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/accept", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.AcceptOutboundCampaignHandoff)},
		{http.MethodGet, "/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/reconciliation", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.ReconcileOutboundCampaignHandoff)},
		{http.MethodPost, "/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.DispatchOutboundCampaignHandoff)},
		{http.MethodGet, "/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch-reconciliation", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.GetOutboundCampaignDispatchReconciliation)},
		{http.MethodPost, "/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch-reconciliation/{effect_id}", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.ReconcileOutboundCampaignDispatch)},
		{http.MethodPost, "/api/admin/questionnaires/{questionnaire_id}/public-publish", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(wrapper.PublishQuestionnairePublicDefinition)},
		{http.MethodPost, "/api/admin/questionnaires/{questionnaire_id}/public-disable", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(wrapper.DisableQuestionnairePublicDefinition)},
		{http.MethodGet, "/api/admin/questionnaires/{questionnaire_id}/public-analytics", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(wrapper.GetQuestionnairePublicAnalytics)},
		{http.MethodGet, surveyhttp.ExternalPushDetailPath, authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(wrapper.GetSurveyExternalPushDetail)},
		{http.MethodPost, surveyhttp.ExternalPushReconcilePath, authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(wrapper.ReconcileSurveyExternalPush)},
	}
	routes = append(routes, struct {
		method, pattern string
		capability      authport.Capability
		csrf            bool
		endpoint        http.Handler
	}{http.MethodPost, orderhttp.WeChatShopReconcilePath, authport.CapabilityOrderWrite, true, http.HandlerFunc(wrapper.ReconcileWechatShopRefund)})
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
			if isLegacyAttachmentPattern(pattern) {
				tail = legacyAttachmentSecurityHeaders(tail)
			}
			if pattern == legacyInternalEventsPath || pattern == legacyInternalEventsDiagnosticsPath || pattern == legacyInternalEventDetailPath {
				tail = legacyInternalEventsSecurityHeaders(tail)
			}
			if pattern == legacyImageCollectionPath || pattern == legacyImageFacetsPath || pattern == legacyImageDetailPath || pattern == legacyImageVariantPath || isLegacyAttachmentPattern(pattern) || pattern == legacyApiDocsPath || pattern == legacyMcpToolsPath || pattern == legacyDataHealthChecksPath || pattern == legacyDataHealthCheckPath || pattern == legacyDataHealthSummaryPath || pattern == legacyHXCSenderReadPath || pattern == legacyHXCSenderItemPath || pattern == legacyHXCSenderReorderPath || pattern == legacyDeliveryLineagePath || pattern == legacyInternalEventsPath || pattern == legacyInternalEventsDiagnosticsPath || pattern == legacyInternalEventDetailPath || pattern == legacyCustomerProfileTagsPath || pattern == legacyCustomerProfileMessagesPath || pattern == legacyChannelPagePath || pattern == legacyCouponPagePath || pattern == legacyOrderPagePath || pattern == legacyProductPagePath || pattern == legacyExecutionRuntimePagePath || pattern == legacyAutomationAgentListPagePath || pattern == legacyQuestionnairePagePath || pattern == legacyQuestionnairePagePath+"/ui" || pattern == legacyQuestionnairePreflightPath || pattern == legacyRuntimeConfigPath || pattern == legacyConfigChecklistPath || pattern == legacyaudiencemembers.RoutePattern || isLegacyCustomerPagePattern(pattern) || isCloudOrchestratorPagePattern(pattern) || automationhttp.IsGroupOpsPagePattern(pattern) || automationhttp.IsAudiencePackagePagePattern(pattern) || producthttp.IsWorkspacePagePattern(pattern) {
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
					} else if pattern == legacyAttachmentCollectionPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyAttachmentCollectionMethodNotAllowed))
					} else if pattern == legacyAttachmentUploadPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyAttachmentUploadMethodNotAllowed))
					} else if pattern == legacyAttachmentDetailPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyAttachmentDetailMethodNotAllowed))
					} else if pattern == legacyAttachmentDownloadPath {
						methodRouter.MethodNotAllowed(http.HandlerFunc(writeLegacyAttachmentDownloadMethodNotAllowed))
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
					if pattern == legacyaudiencemembers.RoutePattern {
						methodRouter.MethodNotAllowed(http.HandlerFunc(legacyaudiencemembers.WriteMethodNotAllowed))
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
		if legacy.groupOps != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
				csrf            bool
				endpoint        http.Handler
			}{
				{http.MethodGet, groupopshttp.PlansPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListPlans)},
				{http.MethodPost, groupopshttp.PlansPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.CreatePlan)},
				{http.MethodGet, groupopshttp.PlanPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.GetPlan)},
				{http.MethodPatch, groupopshttp.PlanPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.UpdatePlan)},
				{http.MethodPut, groupopshttp.PlanPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.UpdatePlan)},
				{http.MethodDelete, groupopshttp.PlanPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.DeletePlan)},
				{http.MethodPost, groupopshttp.PlanActivatePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.Activate)},
				{http.MethodPost, groupopshttp.PlanPausePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.Pause)},
				{http.MethodPost, groupopshttp.PlanArchivePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.Archive)},
				{http.MethodPost, groupopshttp.PlanEnablePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.Enable)},
				{http.MethodPost, groupopshttp.PlanDisablePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.Disable)},
				{http.MethodGet, groupopshttp.MembersPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListMembers)},
				{http.MethodPost, groupopshttp.MembersPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.AddMember)},
				{http.MethodDelete, groupopshttp.MemberPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.RemoveMember)},
				{http.MethodGet, groupopshttp.GroupAssetsPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListGroupAssets)},
				{http.MethodPost, groupopshttp.GroupAssetsPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.AddGroupAsset)},
				{http.MethodDelete, groupopshttp.GroupAssetPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.RemoveGroupAsset)},
				{http.MethodGet, groupopshttp.PlanGroupsPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListPlanGroups)},
				{http.MethodPost, groupopshttp.PlanGroupsPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.AddPlanGroup)},
				{http.MethodDelete, groupopshttp.PlanGroupPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.RemovePlanGroup)},
				{http.MethodGet, groupopshttp.NodesPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListNodes)},
				{http.MethodPost, groupopshttp.NodesPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.AddNode)},
				{http.MethodPatch, groupopshttp.NodePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.UpdateNode)},
				{http.MethodPut, groupopshttp.NodePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.UpdateNode)},
				{http.MethodDelete, groupopshttp.NodePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.RemoveNode)},
				{http.MethodGet, groupopshttp.WebhookDescriptorPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.GetWebhookDescriptor)},
				{http.MethodPut, groupopshttp.WebhookDescriptorPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.PutWebhookDescriptor)},
				{http.MethodGet, groupopshttp.PlanWebhookPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.GetWebhook)},
				{http.MethodPost, groupopshttp.ContentPreviewPath, authport.CapabilityAdminRead, true, http.HandlerFunc(legacy.groupOps.Preview)},
				{http.MethodPost, groupopshttp.RunDuePreviewPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.PreviewRunDue)},
				{http.MethodPost, groupopshttp.RunDuePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.RunDue)},
				{http.MethodGet, groupopshttp.ExecutionsPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListExecutions)},
				{http.MethodPost, groupopshttp.ExecutionReconcilePath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.ReconcileExecution)},
				{http.MethodGet, groupopshttp.GroupsPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListGroups)},
				{http.MethodPost, groupopshttp.GroupsSyncPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.SyncGroups)},
				{http.MethodGet, groupopshttp.GroupPickerPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListGroupPicker)},
				{http.MethodPost, groupopshttp.GroupPickerSyncPath, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.SyncGroupPicker)},
				{http.MethodGet, groupopshttp.OperationMembersPath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.groupOps.ListOperationMembers)},
				{http.MethodPost, groupopshttp.OperationMembersSync, authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.groupOps.SyncOperationMembers)},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, route.endpoint); err != nil {
					return nil, err
				}
			}
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
		if legacy.memberGridManagement != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
				csrf            bool
			}{
				{http.MethodPost, membergrid.RoutePrefix + "/{service_product_id}/member-views", authport.CapabilityProductsWrite, true},
				{http.MethodPut, membergrid.RoutePrefix + "/{service_product_id}/member-views/{view_id}", authport.CapabilityProductsWrite, true},
				{http.MethodDelete, membergrid.RoutePrefix + "/{service_product_id}/member-views/{view_id}", authport.CapabilityProductsWrite, true},
				{http.MethodGet, membergrid.RoutePrefix + "/{service_product_id}/member-grid/share-settings", authport.CapabilityProductsRead, false},
				{http.MethodPost, membergrid.RoutePrefix + "/{service_product_id}/member-grid/collaborators", authport.CapabilityProductsWrite, true},
				{http.MethodPut, membergrid.RoutePrefix + "/{service_product_id}/member-grid/collaborators/{collaborator_id}", authport.CapabilityProductsWrite, true},
				{http.MethodDelete, membergrid.RoutePrefix + "/{service_product_id}/member-grid/collaborators/{collaborator_id}", authport.CapabilityProductsWrite, true},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, legacy.memberGridManagement); err != nil {
					return nil, err
				}
			}
		}
		if legacy.radar != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
				csrf            bool
			}{
				{http.MethodGet, radarthttp.BasePath, authport.CapabilityAdminRead, false},
				{http.MethodPost, radarthttp.BasePath, authport.CapabilityOperationsManage, true},
				{http.MethodGet, radarthttp.BasePath + "/new/options", authport.CapabilityAdminRead, false},
				{http.MethodGet, radarthttp.BasePath + "/{link_id}", authport.CapabilityAdminRead, false},
				{http.MethodPatch, radarthttp.BasePath + "/{link_id}", authport.CapabilityOperationsManage, true},
				{http.MethodPost, radarthttp.BasePath + "/{link_id}/enable", authport.CapabilityOperationsManage, true},
				{http.MethodPost, radarthttp.BasePath + "/{link_id}/disable", authport.CapabilityOperationsManage, true},
				{http.MethodGet, radarthttp.BasePath + "/{link_id}/share", authport.CapabilityAdminRead, false},
				{http.MethodGet, radarthttp.BasePath + "/{link_id}/stats", authport.CapabilityAdminRead, false},
				{http.MethodGet, radarthttp.BasePath + "/{link_id}/events", authport.CapabilityAdminRead, false},
				{http.MethodGet, radarthttp.BasePath + "/{link_id}/events/export", authport.CapabilityAdminRead, false},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, legacy.radar); err != nil {
					return nil, err
				}
			}
		}
		if legacy.campaign != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
				csrf            bool
			}{
				{http.MethodGet, campaign.RoutePrefix, authport.CapabilityOperationsRead, false},
				{http.MethodPost, campaign.RoutePrefix + "/batch-start", authport.CapabilityOperationsManage, true},
				{http.MethodGet, campaign.RoutePrefix + "/{campaign_code}", authport.CapabilityOperationsRead, false},
				{http.MethodDelete, campaign.RoutePrefix + "/{campaign_code}", authport.CapabilityOperationsManage, true},
				{http.MethodPost, campaign.RoutePrefix + "/{campaign_code}/approve", authport.CapabilityOperationsManage, true},
				{http.MethodPost, campaign.RoutePrefix + "/{campaign_code}/reject", authport.CapabilityOperationsManage, true},
				{http.MethodPost, campaign.RoutePrefix + "/{campaign_code}/pause", authport.CapabilityOperationsManage, true},
				{http.MethodPost, campaign.RoutePrefix + "/{campaign_code}/start", authport.CapabilityOperationsManage, true},
				{http.MethodPost, campaign.RoutePrefix + "/{campaign_code}/steps", authport.CapabilityOperationsManage, true},
				{http.MethodPatch, campaign.RoutePrefix + "/{campaign_code}/steps/{step_index}", authport.CapabilityOperationsManage, true},
				{http.MethodDelete, campaign.RoutePrefix + "/{campaign_code}/steps/{step_index}", authport.CapabilityOperationsManage, true},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, legacy.campaign); err != nil {
					return nil, err
				}
			}
		}
		if legacy.aiAudience != nil {
			for _, route := range legacyaudience.RouteSpecs() {
				if err = registerLegacy(route.Method, route.Pattern, authport.Capability(route.Capability), route.RequiresCSRF, legacy.aiAudience); err != nil {
					return nil, err
				}
			}
		}
		if legacy.aiAudienceMembers != nil {
			for _, route := range legacyaudiencemembers.RouteSpecs() {
				if err = registerLegacy(route.Method, route.Pattern, authport.Capability(route.Capability), route.RequiresCSRF, legacy.aiAudienceMembers); err != nil {
					return nil, err
				}
			}
		}
		if legacy.aiAudienceConfiguration != nil {
			for _, route := range legacyaudience.LocalConfigurationRouteSpecs() {
				if err = registerLegacy(route.Method, route.Pattern, authport.Capability(route.Capability), route.RequiresCSRF, legacy.aiAudienceConfiguration); err != nil {
					return nil, err
				}
			}
		}
		if legacy.channelEntrants != nil {
			if err = registerLegacy(
				http.MethodGet,
				contacthttp.ChannelEntrantsRoutePrefix+"/{channel_id}/contacts",
				authport.CapabilityCustomersRead,
				false,
				legacy.channelEntrants,
			); err != nil {
				return nil, err
			}
		}
		if legacy.channelAcquisition != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
				csrf            bool
			}{
				{http.MethodGet, "/api/admin/channels/{channel_id}/acquisition-preview", authport.CapabilityChannelsRead, false},
				{http.MethodPut, "/api/admin/channels/{channel_id}/assignees", authport.CapabilityChannelsWrite, true},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, legacy.channelAcquisition); err != nil {
					return nil, err
				}
			}
		}
		if legacy.channelAcquisitionAsset != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
				csrf            bool
			}{
				{http.MethodPost, "/api/admin/channels/{channel_id}/acquisition-assets", authport.CapabilityChannelsWrite, true},
				{http.MethodGet, "/api/admin/channels/{channel_id}/acquisition-assets", authport.CapabilityChannelsRead, false},
				{http.MethodGet, "/api/admin/channels/{channel_id}/acquisition-assets/{effect_id}", authport.CapabilityChannelsRead, false},
				{http.MethodPost, "/api/admin/channels/{channel_id}/acquisition-assets/{effect_id}/reconcile", authport.CapabilityChannelsWrite, true},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, legacy.channelAcquisitionAsset); err != nil {
					return nil, err
				}
			}
		}
		if legacy.entrantReceipts != nil {
			for _, route := range []struct {
				method, pattern string
				capability      authport.Capability
				csrf            bool
			}{
				{http.MethodGet, "/api/admin/channels/{channel_id}/acquisition-entrant-receipts", authport.CapabilityChannelsRead, false},
				{http.MethodGet, "/api/admin/channels/{channel_id}/acquisition-entrant-receipts/{receipt_id}", authport.CapabilityChannelsRead, false},
				{http.MethodPost, "/api/admin/channels/{channel_id}/acquisition-entrant-receipts/{receipt_id}/reconcile", authport.CapabilityChannelsWrite, true},
				{http.MethodGet, "/api/admin/channel-acquisition-entrant-receipts/unassigned", authport.CapabilityChannelsRead, false},
				{http.MethodGet, "/api/admin/channel-acquisition-entrant-receipts/unassigned/{receipt_id}", authport.CapabilityChannelsRead, false},
				{http.MethodPost, "/api/admin/channel-acquisition-entrant-receipts/unassigned/{receipt_id}/reconcile", authport.CapabilityChannelsWrite, true},
			} {
				if err = registerLegacy(route.method, route.pattern, route.capability, route.csrf, legacy.entrantReceipts); err != nil {
					return nil, err
				}
			}
		}
		refundUnavailable := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		})
		wechatShopRefundEndpoint := http.Handler(refundUnavailable)
		wechatPayRefundEndpoint := http.Handler(refundUnavailable)
		if concrete, ok := candidate.(*candidateHandler); ok && concrete.commerceRefunds != nil {
			wechatShopRefundEndpoint = http.HandlerFunc(concrete.commerceRefunds.WeChatShopCompatibility)
			wechatPayRefundEndpoint = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				concrete.commerceRefunds.WeChatPayCompatibility(writer, request, chi.URLParam(request, "order_id"))
			})
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
			{http.MethodGet, confighttp.SetupWizardPath, authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.SetupWizard)},
			{http.MethodPost, confighttp.SetupWizardPath, authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.SetupWizard)},
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
			{http.MethodGet, automationhttp.CloudOrchestratorCampaignsPath, authport.CapabilityOperationsRead, false, cloudOrchestratorPages},
			{http.MethodGet, automationhttp.CloudOrchestratorObservabilityPath, authport.CapabilityAdminRead, false, cloudOrchestratorPages},
			{http.MethodGet, automationhttp.GroupOpsPlansPath, authport.CapabilityAdminRead, false, groupOpsPages},
			{http.MethodGet, automationhttp.GroupOpsPlanDetailPattern, authport.CapabilityAdminRead, false, groupOpsPages},
			{http.MethodGet, automationhttp.AudiencePackagesPath, authport.CapabilityOperationsRead, false, audiencePackagePages},
			{http.MethodGet, automationhttp.AudiencePackageDetailPattern, authport.CapabilityOperationsRead, false, audiencePackagePages},
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
			{http.MethodGet, "/api/admin/automations/executions", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.ListAutomationRuleRuns)},
			{http.MethodPost, "/api/admin/automations/executions/{action_id}/reconcile", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.ReconcileAutomationRuleRun)},
			{http.MethodGet, "/api/admin/automations", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.ListAutomationRules)},
			{http.MethodPost, "/api/admin/automations", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.CreateAutomationRule)},
			{http.MethodGet, "/api/admin/automations/{rule_id}", authport.CapabilityConfigOverviewRead, false, http.HandlerFunc(legacy.GetAutomationRule)},
			{http.MethodPatch, "/api/admin/automations/{rule_id}", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.UpdateAutomationRule)},
			{http.MethodPost, "/api/admin/automations/{rule_id}/{status}", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.SetAutomationRuleStatus)},
			{http.MethodGet, "/api/archive/health", authport.CapabilityMessageArchiveRead, false, http.HandlerFunc(legacy.ArchiveHealth)},
			{http.MethodPost, "/api/archive/sync", authport.CapabilityMessageArchiveExecute, true, http.HandlerFunc(legacy.RequestArchiveSync)},
			{http.MethodGet, "/api/external/chat-records", authport.CapabilityMessageArchiveExternalRead, false, http.HandlerFunc(legacy.ListExternalChatRecords)},
			{http.MethodGet, "/api/messages/search", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.SearchArchivedMessages)},
			{http.MethodGet, "/api/messages/archive", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.DeprecatedMessageArchive)},
			{http.MethodGet, "/api/messages/{external_userid}/archive", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.DeprecatedExternalMessageArchive)},
			{http.MethodGet, "/api/messages/{external_userid}/history", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.DeprecatedExternalMessageHistory)},
			{http.MethodGet, "/api/messages/{external_userid}", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.ListArchivedMessages)},
			{http.MethodGet, "/api/customers", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.ListExternalCustomers)},
			{http.MethodGet, "/api/customers/{external_userid}", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetExternalCustomer)},
			{http.MethodGet, "/api/customers/{external_userid}/timeline", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetExternalCustomerTimeline)},
			{http.MethodGet, "/api/users/{unionid}", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetExternalUser)},
			{http.MethodGet, "/api/users/{unionid}/messages/recent", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetExternalUserRecentMessages)},
			{http.MethodGet, "/api/users/{unionid}/timeline", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetExternalUserTimeline)},
			{http.MethodGet, "/api/messages/{external_userid}/recent", authport.CapabilityCustomersRead, false, http.HandlerFunc(legacy.GetExternalRecentMessages)},
			{http.MethodGet, "/api/admin/push-center/jobs", authport.CapabilityOutboundRead, false, http.HandlerFunc(legacy.ListOutboundJobs)},
			{http.MethodGet, "/api/admin/push-center/jobs/{job_id}", authport.CapabilityOutboundRead, false, http.HandlerFunc(legacy.GetOutboundJob)},
			{http.MethodGet, "/api/admin/push-center/jobs/{job_id}/reconciliation", authport.CapabilityOutboundRead, false, http.HandlerFunc(legacy.ReconcileOutboundJob)},
			{http.MethodPost, "/api/admin/push-center/jobs/{job_id}/cancel", authport.CapabilityOutboundControl, true, http.HandlerFunc(legacy.CancelOutboundJob)},
			{http.MethodPost, "/api/admin/push-center/jobs/{job_id}/retry", authport.CapabilityOutboundControl, true, http.HandlerFunc(legacy.RetryOutboundJob)},
			{http.MethodGet, "/api/admin/push-center/sections", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.PushCenterSections)},
			{http.MethodGet, "/api/admin/push-center/stats", authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.PushCenterStats)},
			{http.MethodGet, outboundhttp.ExternalEffectsJobsPath, authport.CapabilityOperationsRead, false, http.HandlerFunc(legacy.ExternalEffectsJobs)},
			{http.MethodGet, outboundhttp.ExternalEffectsDiagnosticsPath, authport.CapabilityOperationsRead, false, externalEffectsRuntimeDiagnostics},
			{http.MethodGet, legacyExecutionRuntimePagePath, authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.ExecutionRuntimePage)},
			{http.MethodGet, "/api/admin/execution-runtime", authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.ExecutionRuntime)},
			{http.MethodGet, "/api/admin/executions/{execution_id}", authport.CapabilityAdminRead, false, http.HandlerFunc(legacy.ExecutionTimeline)},
			{http.MethodGet, legacyProductPagePath, authport.CapabilityProductsRead, false, http.HandlerFunc(legacy.ProductListPage)},
			{http.MethodGet, "/api/admin/wechat-pay/products", authport.CapabilityProductsRead, false, http.HandlerFunc(legacy.ListProducts)},
			{http.MethodPost, "/api/admin/wechat-pay/products", authport.CapabilityProductsWrite, true, http.HandlerFunc(legacy.CreateProduct)},
			{http.MethodGet, "/api/admin/wechat-pay/products/{product_id}", authport.CapabilityProductsRead, false, http.HandlerFunc(legacy.GetProduct)},
			{http.MethodPut, "/api/admin/wechat-pay/products/{product_id}", authport.CapabilityProductsWrite, true, http.HandlerFunc(legacy.UpdateProduct)},
			{http.MethodGet, legacyOrderPagePath, authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.OrderListPage)},
			{http.MethodGet, "/api/admin/orders", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListOrderBoard)},
			{http.MethodGet, "/api/admin/orders/{order_no}", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.GetOrderBoard)},
			{http.MethodGet, "/api/admin/orders/{order_no}/items", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.GetOrderBoardItems)},
			{http.MethodGet, "/api/admin/alipay/transactions", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListAlipayTransactions)},
			{http.MethodGet, "/api/admin/alipay/transactions/{order_no}", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.GetAlipayTransaction)},
			{http.MethodGet, "/api/admin/wechat-pay/orders", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListWechatTransactions)},
			{http.MethodGet, "/api/admin/refunds", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListOrderBoardRefunds)},
			{http.MethodPost, "/api/admin/exports", authport.CapabilityOrderWrite, true, http.HandlerFunc(legacy.CreateOrderBoardExport)},
			{http.MethodPost, "/api/admin/exports/preview", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.PreviewOrderBoardExport)},
			{http.MethodGet, "/api/admin/exports/{job_id}", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.GetOrderBoardExport)},
			{http.MethodPost, "/api/admin/wechat-pay/order-exports", authport.CapabilityOrderWrite, true, http.HandlerFunc(legacy.CreateWechatOrderBoardExport)},
			{http.MethodGet, "/api/admin/wechat-pay/order-exports/{job_id}", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.DeprecatedWechatOrderBoardExport)},
			{http.MethodGet, "/api/admin/wechat-pay/order-exports/{job_id}/download", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.DeprecatedWechatOrderBoardExport)},
			{http.MethodPost, "/api/admin/refunds", authport.CapabilityOrderWrite, true, wechatShopRefundEndpoint},
			{http.MethodPost, "/api/admin/wechat-pay/orders/{order_id}/refunds", authport.CapabilityOrderWrite, true, wechatPayRefundEndpoint},
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
			{http.MethodGet, legacyAttachmentCollectionPath, authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.ListAttachments)},
			{http.MethodPost, legacyAttachmentCollectionPath, authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.CreateAttachment)},
			{http.MethodPost, legacyAttachmentUploadPath, authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.UploadAttachment)},
			{http.MethodGet, legacyAttachmentDetailPath, authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetAttachment)},
			{http.MethodPut, legacyAttachmentDetailPath, authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.UpdateAttachment)},
			{http.MethodDelete, legacyAttachmentDetailPath, authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.DeleteAttachment)},
			{http.MethodGet, legacyAttachmentDownloadPath, authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.DownloadAttachment)},
			{http.MethodPost, "/api/admin/content-packages/preview", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.ContentPackagePreview)},
			{http.MethodPost, "/api/admin/content-packages", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.ContentPackageCreate)},
			{http.MethodPut, "/api/admin/content-packages/{package_id}", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.ContentPackageUpdate)},
			{http.MethodPost, "/api/admin/attachment-library/uploads", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.PDFMultipartInitiate)},
			{http.MethodPut, "/api/admin/attachment-library/uploads/{upload_id}/parts/{part_number}", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.PDFMultipartPart)},
			{http.MethodPost, "/api/admin/attachment-library/uploads/{upload_id}/complete", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.PDFMultipartComplete)},
			{http.MethodPost, "/api/admin/outbound-media/accept", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.AcceptOutboundMedia)},
			{http.MethodGet, "/api/admin/outbound-media/{content_package_id}/effects/{target_ref}", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetOutboundMediaEffectDetail)},
			{http.MethodPost, "/api/admin/outbound-media/{content_package_id}/effects/{target_ref}/reconcile", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.ReconcileOutboundMedia)},
			{http.MethodGet, "/api/admin/campaigns/{campaign_code}/plans/{plan_id}/content-delivery-binding", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.ContentDeliveryBindingGet)},
			{http.MethodPost, "/api/admin/campaigns/{campaign_code}/plans/{plan_id}/content-delivery-binding", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.ContentDeliveryBindingCreate)},
			{http.MethodPut, "/api/admin/campaigns/{campaign_code}/plans/{plan_id}/content-delivery-binding", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.ContentDeliveryBindingUpdate)},
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
			{http.MethodGet, surveyoperationshttp.GlobalExternalPushLogsPath, authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.ListSurveyExternalPushLogs)},
			{http.MethodGet, surveyoperationshttp.OperationsPagePath, authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.GetSurveyOperationsPage)},
			{http.MethodGet, surveyoperationshttp.QuestionnaireExternalPushLogsPath, authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.ListSurveyQuestionnaireExternalPushLogs)},
			{http.MethodGet, "/api/admin/questionnaires", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.ListQuestionnaires)},
			{http.MethodPost, "/api/admin/questionnaires", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.CreateQuestionnaire)},
			{http.MethodGet, "/api/admin/questionnaires/{questionnaire_id}", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.GetQuestionnaire)},
			{http.MethodPut, "/api/admin/questionnaires/{questionnaire_id}", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.UpdateQuestionnaire)},
			{http.MethodPatch, "/api/admin/questionnaires/{questionnaire_id}", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.UpdateQuestionnaire)},
			{http.MethodGet, surveyoperationshttp.OperationsPath, authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.GetSurveyOperations)},
			{http.MethodPut, surveyoperationshttp.CompletionPath, authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.SaveSurveyCompletionOperations)},
			{http.MethodPut, surveyoperationshttp.ExternalPushPath, authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.SaveSurveyExternalPushOperations)},
			{http.MethodPost, surveyoperationshttp.ExternalPushTestPath, authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.QueueSurveyExternalPushTest)},
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
			{http.MethodPost, "/api/admin/wecom/tag-effects/{effect_id}/reconcile", authport.CapabilityOperationsManage, true, http.HandlerFunc(legacy.ReconcileWeComTagEffect)},
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

func publicProtocolExactMethod(method string, endpoint http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == method {
			endpoint.ServeHTTP(writer, request)
			return
		}
		writer.Header().Set("Allow", method)
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = writer.Write([]byte("{\"code\":\"method_not_allowed\"}\n"))
	})
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
