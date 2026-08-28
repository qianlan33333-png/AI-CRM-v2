// This file adapts the frozen legacy browser transport at the aicrm
// composition root. It owns no business rules, storage, or provider calls.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	operationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	wecomtag "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
)

const (
	LegacySessionCookieName = "aicrm_next_admin_session"
	LegacyCSRFCookieName    = "aicrm_next_csrf"
	legacySessionMaxAge     = 8 * time.Hour
	legacyImageFacetsPath   = "/api/admin/image-library/facets"
)

var (
	errInvalidLegacyQuery   = errors.New("legacy customer list query cannot be mapped safely")
	errInvalidLegacyProduct = errors.New("legacy product request cannot be mapped safely")
)

type customerListApplication interface {
	List(context.Context, contactapp.CustomerListInput) (contactapp.CustomerListResult, error)
}

type customerDetailApplication interface {
	Get(context.Context, contactapp.CustomerDetailInput) (contactapp.CustomerDetailStoreResult, error)
}

type identityResolveApplication interface {
	Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error)
}

type legacyProductApplication interface {
	ListLegacy(context.Context, int32, int32) (productport.LegacyPage, error)
	Get(context.Context, productport.ID) (productport.Product, error)
	Create(context.Context, productport.CreateCommand) (productport.Product, error)
	Update(context.Context, productport.UpdateCommand) (productport.Product, error)
}

type legacyMediaApplication interface {
	Upload(context.Context, mediaport.UploadCommand) (mediaport.Image, error)
	Facets(context.Context) (mediaport.ImageFacets, error)
}

type legacyImageFacetsSuccess struct {
	OK                       bool     `json:"ok"`
	Categories               []string `json:"categories"`
	Tags                     []string `json:"tags"`
	SourceStatus             string   `json:"source_status"`
	RouteOwner               string   `json:"route_owner"`
	FallbackUsed             bool     `json:"fallback_used"`
	RealExternalCallExecuted bool     `json:"real_external_call_executed"`
	StorageAdapterMode       string   `json:"storage_adapter_mode"`
	AdapterMode              string   `json:"adapter_mode"`
}

type miniProgramApplication interface {
	List(context.Context, mediaport.MiniProgramListQuery) (mediaport.MiniProgramPage, error)
	Get(context.Context, int64) (mediaport.MiniProgram, error)
	Create(context.Context, mediaport.MiniProgramCreateCommand) (mediaport.MiniProgramMutationResult, error)
	Update(context.Context, mediaport.MiniProgramUpdateCommand) (mediaport.MiniProgramMutationResult, error)
	Delete(context.Context, mediaport.MiniProgramDeleteCommand) (mediaport.MiniProgramDeleteResult, error)
	ResolveThumbnail(context.Context, mediaport.MiniProgramResolveThumbnailCommand) (mediaport.MiniProgramThumbnailResolutionResult, error)
}

type legacySurveyApplication interface {
	ListLegacy(context.Context, int32, int32) (surveyport.LegacyPage, error)
	Get(context.Context, surveyport.ID) (surveyport.Questionnaire, error)
	Create(context.Context, surveyport.CreateCommand) (surveyport.Questionnaire, error)
	Update(context.Context, surveyport.ID, surveyport.UpdateCommand) (surveyport.Questionnaire, error)
	SetDisabled(context.Context, surveyport.ID, bool, int64, string) (surveyport.Questionnaire, error)
	Delete(context.Context, surveyport.ID, int64, string) (surveyport.DeleteResult, error)
	Duplicate(context.Context, surveyport.ID, int64, string, string, string) (surveyport.Questionnaire, error)
}

// legacySurveySubmissionApplication is the frozen read-only submission
// analysis surface: aggregate results, paged detail, and the PII CSV export.
type legacySurveySubmissionApplication interface {
	Results(context.Context, surveyport.ID) (surveyport.SubmissionResult, error)
	List(context.Context, surveyport.ID, int32, int32) (surveyport.SubmissionPage, error)
	Export(context.Context, surveyport.ID) (surveyport.SubmissionCSVDownload, error)
}

type legacyChannelApplication interface {
	ListChannels(context.Context, int32, string, bool) ([]contactapp.Channel, error)
	GetChannel(context.Context, int64) (contactapp.Channel, error)
	CreateChannel(context.Context, contactapp.CreateChannelCommand) (contactapp.Channel, error)
	UpdateChannel(context.Context, contactapp.UpdateChannelCommand) (contactapp.Channel, error)
}

type legacyTagApplication interface {
	List(context.Context) (contactapp.LegacyTagCatalog, error)
	GetGroup(context.Context, int64) (contactapp.LegacyTagGroup, error)
	GetTag(context.Context, int64) (contactapp.LegacyTag, error)
	CreateGroup(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, contactapp.LegacyTag, error)
	UpdateGroup(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error)
	ArchiveGroup(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error)
	CreateTag(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
	UpdateTag(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
	ArchiveTag(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
}

type legacyTagSyncApplication interface {
	Request(context.Context, contactapp.LegacyTagSyncCommand) (contactapp.LegacyTagSyncAcceptance, wecomtag.Acceptance, error)
}

type legacyTagLiveMutationApplication interface {
	Request(context.Context, contactapp.LegacyTagLiveMutationCommand, string, []string) (contactapp.LegacyTagLiveMutationAcceptance, wecomtag.Acceptance, error)
}

type legacyTagExecutionStatusApplication interface {
	Get(context.Context) (contactapp.LegacyTagExecutionGate, error)
}

type wecomTagEffectApplication interface {
	Reconcile(context.Context, wecomtag.ReconcileCommand) (wecomtag.Reconciliation, error)
}

type legacyOrderApplication interface {
	List(context.Context, orderport.Filter) (orderport.Page, error)
}

type legacyExecutionRuntimeApplication interface {
	Runtime(context.Context) (adminopsapp.ExecutionRuntime, error)
	Timeline(context.Context, string) (adminopsport.ExecutionTimeline, error)
}

// runtimeConfigDeclaration is an immutable, local-only rendering projection.
// It contains no connection, secret, provider, or persisted-state material.
type runtimeConfigDeclaration struct {
	DatabaseMode        string
	ProductionDataReady string
	ReleaseSHA          string
	WeChatCallbackToken string
	WeChatPayConfig     string
	OAuthConfig         string
}

// Handler is deliberately a thin transport adapter over existing v2 services.
type Handler struct {
	auth                    authport.Service
	customers               customerListApplication
	customerDetail          customerDetailApplication
	identityResolve         identityResolveApplication
	weComCorpID             string
	outbound                legacyOutboundQueryApplication
	cancel                  legacyCancelApplication
	manualRetry             legacyRetryApplication
	products                legacyProductApplication
	servicePeriod           http.Handler
	servicePeriodHistory    productport.ServicePeriodHistoryReader
	memberGrid              http.Handler
	memberGridManagement    http.Handler
	memberGridExternalShare http.Handler
	radar                   http.Handler
	campaign                http.Handler
	aiAudience              http.Handler
	audienceHistory         segmentport.AudienceHistoryReader
	aiAudienceInbound       *aiAudienceInboundRoutes
	aiAudienceMembers       http.Handler
	aiAudienceConfiguration http.Handler
	aiAudienceSendRecords   http.Handler
	channelEntrants         http.Handler
	channelAcquisition      http.Handler
	channelAcquisitionAsset http.Handler
	entrantReceipts         http.Handler
	media                   legacyMediaApplication
	imageDeletes            legacyImageDeleteApplication
	attachments             legacyAttachmentApplication
	contentDelivery         mediaport.ContentDeliveryService
	outboundMediaAccepted   outboundMediaAcceptedApplication
	outboundMediaDetail     outboundMediaEffectDetailApplication
	outboundMediaReconcile  outboundMediaReconcileApplication
	groupInvites            groupInviteApplication
	miniPrograms            miniProgramApplication
	surveys                 legacySurveyApplication
	surveySubmissions       legacySurveySubmissionApplication
	surveySafeAdmin         surveySafeAdminHTTP
	surveyOperations        surveyOperationsHTTP
	groupOps                groupOpsHTTP
	groupOpsHistory         *adminGroupOpsHistory
	channels                legacyChannelApplication
	channelHistory          contactport.HistoricalChannelHistoryReader
	legacyTags              legacyTagApplication
	automationAgents        automationport.AgentService
	automationRules         automationport.RuleService
	automationRuleRuns      automationport.RuntimeReader
	automationRuleReconcile automationport.RuntimeReconciler
	legacyTagSync           legacyTagSyncApplication
	legacyTagLive           legacyTagLiveMutationApplication
	legacyTagStatus         legacyTagExecutionStatusApplication
	wecomTagEffects         wecomTagEffectApplication
	coupons                 legacyCouponApplication
	couponBoard             couponBoardApplication
	couponHistory           couponport.HistoricalReader
	contactHistory          contactport.ContactHistoryReader
	settings                legacySettingsApplication
	setupWizard             http.Handler
	adminAccess             http.Handler
	orders                  legacyOrderApplication
	orderBoard              legacyOrderBoardApplication
	messageArchive          legacyMessageArchiveApplication
	messageHistory          wecomport.MessageHistoryReader
	messageArchiveUnionID   legacyMessageArchiveUnionResolver
	customerQuestionnaires  *legacyCustomerProfileQuestionnaireAnswersHandler
	adminOps                legacyAdminOps
	runtimeConfig           runtimeConfigDeclaration
	operationCycles         legacyOperationCycleApplication
	pushCenter              legacyPushCenterApplication
	externalEffects         externalEffectsHTTP
	executionRuntime        legacyExecutionRuntimeApplication
	operationAuth           operationServiceAuthenticator
	systemHealth            http.Handler
	hxcSender               *hxcSenderHandler
	deliveryLineage         legacyDeliveryLineageReaders
	externalCustomerRead    *legacyExternalCustomerReadHandler
}

type aiAudienceInboundRoutes struct {
	webhook              http.Handler
	retiredSubscriptions http.Handler
}

// legacyOperationCycleApplication is the frozen A+B operation surface. Its
// commands only create local facts or durable queue acceptance; it exposes no
// provider or generic agent operation.
type legacyOperationCycleApplication interface {
	Report(context.Context, operationapp.ReportCommand) (map[string]any, error)
	ListStrategies(context.Context, int32, int32) (map[string]any, error)
	GetStrategy(context.Context, string) (map[string]any, error)
	ListRuns(context.Context, string, int32, int32) (map[string]any, error)
	GetRun(context.Context, string) (map[string]any, error)
	Start(context.Context, operationapp.StartCommand) (map[string]any, error)
	CurrentAction(context.Context, string) (map[string]any, error)
	GetActionResult(context.Context, string) (map[string]any, error)
	Claim(context.Context, string, string) (map[string]any, error)
	RecordActionEvent(context.Context, operationapp.ActionEventCommand) (map[string]any, error)
	Heartbeat(context.Context, operationapp.RunnerHeartbeatCommand) (map[string]any, error)
	ContextIndex(context.Context, int32, int32) (map[string]any, error)
	StrategyContext(context.Context, string, string, int32, int32, map[string]string) (map[string]any, error)
	CreateProposal(context.Context, operationapp.ProposalCommand) (map[string]any, error)
	ListProposals(context.Context, string, int32, int32) (map[string]any, error)
	DecideProposal(context.Context, string, string, string) (map[string]any, error)
}

// operationServiceAuthenticator intentionally has no production wiring in
// this slice. B routes therefore fail closed until the separately-owned
// client credential/JWT lifecycle is supplied; they never fall back to a
// browser cookie, anonymous request, or ad-hoc header.
type operationServiceAuthenticator interface {
	AuthenticateOperation(context.Context, *http.Request, string) (operationServicePrincipal, error)
}

type operationServicePrincipal struct {
	ClientID    string
	PrincipalID string
}

type legacySettingsApplication interface {
	List(context.Context, configapp.SettingsListInput) (configapp.SettingsProjection, error)
	Save(context.Context, configapp.SaveSettingsInput) error
}

type legacyCouponApplication interface {
	List(context.Context, int32, int32, string, string) (couponport.Page, error)
	Get(context.Context, couponport.ID) (couponport.Coupon, error)
	Create(context.Context, couponport.UpsertCommand) (couponport.Coupon, error)
	Update(context.Context, couponport.UpsertCommand) (couponport.Coupon, error)
	UpdateDraft(context.Context, couponport.UpsertCommand) (couponport.Coupon, error)
	Publish(context.Context, couponport.ID, int64, string) (couponport.Coupon, error)
	Stop(context.Context, couponport.ID, int64, string) (couponport.Coupon, error)
}

type couponBoardApplication interface {
	Archive(context.Context, couponport.ID, int64, string) (couponport.Coupon, error)
	Delete(context.Context, couponport.ID, int64, string) (couponport.Coupon, error)
	Copy(context.Context, couponport.ID, int64, string) (couponport.Coupon, error)
	Claim(context.Context, couponport.ClaimCommand) (couponport.Claim, error)
	ListClaims(context.Context, couponport.ID, int32, int32) (couponport.ClaimPage, error)
	ListAvailable(context.Context, string, int64) ([]couponport.Coupon, error)
	ResolvePaymentIdentitySession(context.Context, string) (int64, error)
	ResolveSidebarGrant(context.Context, string) (int64, error)
	ListSidebarCoupons(context.Context, int64) ([]couponport.SidebarCoupon, error)
}

func NewHandlerWithAll(
	auth authport.Service,
	customers customerListApplication,
	customerDetail customerDetailApplication,
	identityResolve identityResolveApplication,
	weComCorpID string,
	outbound legacyOutboundQueryApplication,
	cancel legacyCancelApplication,
	manualRetry legacyRetryApplication,
	products legacyProductApplication,
	media legacyMediaApplication,
	groupInvites groupInviteApplication,
	miniPrograms miniProgramApplication,
	surveys legacySurveyApplication,
	channels legacyChannelApplication,
	coupons legacyCouponApplication,
	legacyTags legacyTagApplication,
	settings legacySettingsApplication,
) (*Handler, error) {
	handler, err := NewHandlerWithOutboundProductsMediaAndSurvey(
		auth, customers, outbound, cancel, manualRetry, products, media, surveys,
	)
	if err != nil || nilLegacyDependency(customerDetail) || nilLegacyDependency(identityResolve) || nilLegacyDependency(channels) || nilLegacyDependency(coupons) || nilLegacyDependency(legacyTags) || nilLegacyDependency(groupInvites) || nilLegacyDependency(miniPrograms) || nilLegacyDependency(settings) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	handler.customerDetail = customerDetail
	handler.identityResolve = identityResolve
	handler.weComCorpID = strings.TrimSpace(weComCorpID)
	handler.channels = channels
	handler.groupInvites = groupInvites
	handler.miniPrograms = miniPrograms
	handler.coupons = coupons
	if board, ok := coupons.(couponBoardApplication); ok {
		handler.couponBoard = board
	}
	handler.legacyTags = legacyTags
	handler.settings = settings
	return handler, nil
}

func NewHandlerWithOutboundAndProducts(
	auth authport.Service,
	customers customerListApplication,
	outbound legacyOutboundQueryApplication,
	cancel legacyCancelApplication,
	manualRetry legacyRetryApplication,
	products legacyProductApplication,
) (*Handler, error) {
	handler, err := NewHandlerWithOutbound(auth, customers, outbound, cancel, manualRetry)
	if err != nil || nilLegacyDependency(products) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	handler.products = products
	return handler, nil
}

func NewHandlerWithOutboundProductsAndMedia(
	auth authport.Service,
	customers customerListApplication,
	outbound legacyOutboundQueryApplication,
	cancel legacyCancelApplication,
	manualRetry legacyRetryApplication,
	products legacyProductApplication,
	media legacyMediaApplication,
) (*Handler, error) {
	handler, err := NewHandlerWithOutboundAndProducts(auth, customers, outbound, cancel, manualRetry, products)
	if err != nil || nilLegacyDependency(media) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	handler.media = media
	return handler, nil
}

func NewHandlerWithOutboundProductsMediaAndSurvey(
	auth authport.Service,
	customers customerListApplication,
	outbound legacyOutboundQueryApplication,
	cancel legacyCancelApplication,
	manualRetry legacyRetryApplication,
	products legacyProductApplication,
	media legacyMediaApplication,
	surveys legacySurveyApplication,
) (*Handler, error) {
	handler, err := NewHandlerWithOutboundProductsAndMedia(auth, customers, outbound, cancel, manualRetry, products, media)
	if err != nil || nilLegacyDependency(surveys) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	handler.surveys = surveys
	return handler, nil
}

func (handler *Handler) GetImageFacets(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.media) || request == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, mediaapp.ErrFacetsUnavailable))
		return
	}
	facets, err := handler.media.Facets(request.Context())
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, err))
		return
	}
	if facets.Categories == nil {
		facets.Categories = []string{}
	}
	if facets.Tags == nil {
		facets.Tags = []string{}
	}
	writeJSON(writer, http.StatusOK, legacyImageFacetsSuccess{
		OK: true, Categories: facets.Categories, Tags: facets.Tags, SourceStatus: "next_media_library",
		RouteOwner: "ai_crm_next", FallbackUsed: false, RealExternalCallExecuted: false,
		StorageAdapterMode: "postgresql", AdapterMode: "postgresql",
	})
}

func (handler *Handler) UploadImage(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.media) || request == nil {
		writeLegacyImageError(writer, mediaapp.ErrUnavailable)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyImageError(writer, mediaapp.ErrUnavailable)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, domain.MaxImageBytes+(1<<20))
	if err := request.ParseMultipartForm(domain.MaxImageBytes + (64 << 10)); err != nil {
		writeLegacyImageError(writer, mediaapp.ErrInvalidUpload)
		return
	}
	file, header, err := request.FormFile("image")
	if err != nil {
		writeLegacyImageError(writer, mediaapp.ErrInvalidUpload)
		return
	}
	defer file.Close()
	content, err := domain.ReadBounded(file)
	if err != nil {
		writeLegacyImageError(writer, mediaapp.ErrInvalidUpload)
		return
	}
	key := request.Header.Get("Idempotency-Key")
	if key == "" {
		var value [16]byte
		if _, err = rand.Read(value[:]); err != nil {
			writeLegacyImageError(writer, mediaapp.ErrUnavailable)
			return
		}
		key = "legacy-upload:" + hex.EncodeToString(value[:])
	}
	result, err := handler.media.Upload(request.Context(), mediaport.UploadCommand{
		Actor: principal.AdminUserID, IdempotencyKey: key, FileName: header.Filename,
		DeclaredType: header.Header.Get("Content-Type"), Content: content,
		Name: request.FormValue("name"), Description: request.FormValue("description"),
		Tags: request.FormValue("tags"), Category: request.FormValue("category"),
	})
	if err != nil {
		writeLegacyImageError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "item": projectLegacyImageUploadItem(result), "source_status": "local_upload", "route_owner": "ai_crm_next",
		"fallback_used": false, "real_external_call_executed": false,
		"storage_adapter_mode": "postgresql", "adapter_mode": "postgresql",
	})
}

// projectLegacyImageUploadItem freezes the pre-0357 multipart contract. The
// new enabled field is intentionally not visible through /upload.
func projectLegacyImageUploadItem(image mediaport.Image) struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	FileName    string    `json:"file_name"`
	FileSize    int32     `json:"file_size"`
	MimeType    string    `json:"mime_type"`
	Width       int32     `json:"width"`
	Height      int32     `json:"height"`
	Description string    `json:"description"`
	Tags        string    `json:"tags"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
} {
	return struct {
		ID          int64     `json:"id"`
		Name        string    `json:"name"`
		FileName    string    `json:"file_name"`
		FileSize    int32     `json:"file_size"`
		MimeType    string    `json:"mime_type"`
		Width       int32     `json:"width"`
		Height      int32     `json:"height"`
		Description string    `json:"description"`
		Tags        string    `json:"tags"`
		Category    string    `json:"category"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}{image.ID, image.Name, image.FileName, image.FileSize, image.MimeType, image.Width, image.Height, image.Description, image.Tags, image.Category, image.CreatedAt, image.UpdatedAt}
}

func writeLegacyImageError(writer http.ResponseWriter, err error) {
	platformhttp.MarkCompatibilityError(writer, platformhttp.CodeMalformedRequest)
	writeJSON(writer, http.StatusBadRequest, map[string]any{
		"ok": false, "error": err.Error(), "source_status": "next_media_library_error", "route_owner": "ai_crm_next",
		"fallback_used": false, "real_external_call_executed": false,
	})
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
		var token string
		switch len(values) {
		case 0:
			if !sameOriginBrowserRequest(request) {
				platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrCSRFInvalid))
				return
			}
			for _, name := range []string{authhttp.CSRFCookieName, LegacyCSRFCookieName} {
				cookie, err := request.Cookie(name)
				if err == nil && validToken(cookie.Value) {
					token = cookie.Value
					break
				}
			}
		case 1:
			token = values[0]
		}
		if !validToken(token) {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrCSRFInvalid))
			return
		}
		if err := handler.auth.ValidateCSRF(request.Context(), session, authport.CSRFToken(token)); err != nil {
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

func sameOriginBrowserRequest(request *http.Request) bool {
	if request == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(request.Header.Get("Sec-Fetch-Site"))) {
	case "same-origin":
		return true
	case "cross-site", "same-site", "none":
		return false
	}
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	prefix := "https://"
	if request.TLS == nil {
		prefix = "http://"
	}
	return strings.EqualFold(origin, prefix+request.Host)
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

// GetCustomer resolves the frozen external_userid through Identity before it
// reads Contact's channel-neutral OneID projection. The raw identifier is
// never passed to Contact or persisted by this adapter.
func (handler *Handler) GetCustomer(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.customerDetail) || nilLegacyDependency(handler.identityResolve) || request == nil || handler.weComCorpID == "" {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, contactapp.ErrCustomerDetailUnavailable))
		return
	}
	externalUserID := strings.TrimSpace(chi.URLParam(request, "external_userid"))
	if externalUserID == "" {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, contactapp.ErrCustomerNotFound))
		return
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityCustomersRead {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	ownerStaffID, err := legacyOwnerScope(authorization, nil)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, err))
		return
	}
	resolved, err := handler.identityResolve.Resolve(request.Context(), identityport.IDRef{
		Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + handler.weComCorpID,
		Value: externalUserID, Assurance: identityport.AssuranceVerified, Source: "legacy-customer-read",
	})
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, err))
		return
	}
	if resolved.Status != identityport.ResolveFound || resolved.CustomerID <= 0 {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, contactapp.ErrCustomerNotFound))
		return
	}
	result, err := handler.customerDetail.Get(request.Context(), contactapp.CustomerDetailInput{
		ID: contactport.CustomerID(resolved.CustomerID), OwnerStaffID: ownerStaffID,
	})
	if err != nil {
		code := platformhttp.CodeDependencyUnavailable
		if errors.Is(err, contactapp.ErrCustomerNotFound) || errors.Is(err, contactapp.ErrInvalidCustomerDetailQuery) {
			code = platformhttp.CodeNotFound
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
		return
	}
	tags := make([]string, 0, len(result.Tags))
	for _, tag := range result.Tags {
		tags = append(tags, tag.Name)
	}
	customer := map[string]any{
		"external_userid": externalUserID, "customer_id": int64(result.Customer.ID),
		"customer_name": result.Customer.Name, "avatar_url": result.Customer.AvatarURL,
		"gender": result.Customer.Gender, "stage_id": result.Customer.StageID,
		"owner_staff_id": result.Customer.OwnerStaffID, "channel_id": result.Customer.ChannelID,
		"added_at": result.Customer.AddedAt, "last_interact_at": result.Customer.LastInteractAt,
		"is_deleted": result.Customer.IsDeleted, "tags": tags,
		"created_at": result.Customer.CreatedAt.UTC(), "updated_at": result.Customer.UpdatedAt.UTC(),
	}
	mirrorLegacyCSRFCookie(writer, request)
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "customer": customer, "source_status": "v2_identity_contact_read",
		"fallback_used": false, "real_external_call_executed": false,
	})
}

func (handler *Handler) ListProducts(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.products) || request == nil {
		writeLegacyProductError(writer, request, productapp.ErrUnavailable)
		return
	}
	limit, offset, err := legacyProductPage(request)
	if err != nil {
		writeLegacyProductError(writer, request, err)
		return
	}
	page, err := handler.products.ListLegacy(request.Context(), limit, offset)
	if err != nil {
		writeLegacyProductError(writer, request, err)
		return
	}
	items := make([]map[string]any, len(page.Items))
	for i, item := range page.Items {
		items[i], err = legacyProduct(item)
		if err != nil {
			writeLegacyProductError(writer, request, err)
			return
		}
	}
	writeJSON(writer, http.StatusOK, legacyProductEnvelope(map[string]any{
		"ok": true, "items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset,
	}))
}

func (handler *Handler) GetProduct(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.products) || request == nil {
		writeLegacyProductError(writer, request, productapp.ErrUnavailable)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(request, "product_id")), 10, 64)
	if err != nil || id < 1 {
		writeLegacyProductError(writer, request, errInvalidLegacyProduct)
		return
	}
	item, err := handler.products.Get(request.Context(), productport.ID(id))
	if err != nil {
		writeLegacyProductError(writer, request, err)
		return
	}
	mapped, err := legacyProduct(item)
	if err != nil {
		writeLegacyProductError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, legacyProductEnvelope(map[string]any{"ok": true, "product": mapped}))
}

func (handler *Handler) CreateProduct(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.products) || request == nil {
		writeLegacyProductError(writer, request, productapp.ErrUnavailable)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyProductError(writer, request, authport.ErrUnauthorized)
		return
	}
	command, err := legacyProductCommand(writer, request, principal.AdminUserID)
	if err != nil {
		writeLegacyProductError(writer, request, err)
		return
	}
	item, err := handler.products.Create(request.Context(), command)
	if err != nil {
		writeLegacyProductError(writer, request, err)
		return
	}
	mapped, err := legacyProduct(item)
	if err != nil {
		writeLegacyProductError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, legacyProductEnvelope(map[string]any{"ok": true, "product": mapped}))
}

type legacyProductUpsertRequest struct {
	ProductCode               string          `json:"product_code"`
	Title                     string          `json:"title"`
	Name                      string          `json:"name"`
	Description               string          `json:"description"`
	PriceCents                *int64          `json:"price_cents"`
	AmountTotal               *int64          `json:"amount_total"`
	Currency                  string          `json:"currency"`
	Status                    *string         `json:"status"`
	Enabled                   *bool           `json:"enabled"`
	BuyButtonText             string          `json:"buy_button_text"`
	RequireMobile             bool            `json:"require_mobile"`
	LeadProgramID             *int64          `json:"lead_program_id"`
	LeadChannelID             *int64          `json:"lead_channel_id"`
	LeadQRTitle               string          `json:"lead_qr_title"`
	LeadQRSubtitle            string          `json:"lead_qr_subtitle"`
	CompletionRedirectEnabled bool            `json:"completion_redirect_enabled"`
	CompletionRedirectURL     string          `json:"completion_redirect_url"`
	CompletionTarget          json.RawMessage `json:"completion_target"`
	WeComTagging              json.RawMessage `json:"wecom_tagging"`
	Slices                    json.RawMessage `json:"slices"`
}

func legacyProductCommand(writer http.ResponseWriter, request *http.Request, actor int64) (productport.CreateCommand, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 256<<10))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var body legacyProductUpsertRequest
	if decoder.Decode(&body) != nil {
		return productport.CreateCommand{}, errInvalidLegacyProduct
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return productport.CreateCommand{}, errInvalidLegacyProduct
	}
	name := strings.TrimSpace(body.Title)
	alias := strings.TrimSpace(body.Name)
	if name == "" {
		name = alias
	} else if alias != "" && alias != name {
		return productport.CreateCommand{}, errInvalidLegacyProduct
	}
	if body.PriceCents == nil {
		body.PriceCents = body.AmountTotal
	} else if body.AmountTotal != nil && *body.AmountTotal != *body.PriceCents {
		return productport.CreateCommand{}, errInvalidLegacyProduct
	}
	if body.PriceCents == nil || body.Status == nil || body.Enabled == nil {
		return productport.CreateCommand{}, errInvalidLegacyProduct
	}
	projection, err := json.Marshal(map[string]any{
		"schema_version": 1, "status": *body.Status, "enabled": *body.Enabled,
		"buy_button_text": body.BuyButtonText, "require_mobile": body.RequireMobile,
		"lead_program_id": body.LeadProgramID, "lead_channel_id": body.LeadChannelID,
		"lead_qr_title": body.LeadQRTitle, "lead_qr_subtitle": body.LeadQRSubtitle,
		"completion_redirect_enabled": body.CompletionRedirectEnabled,
		"completion_redirect_url":     body.CompletionRedirectURL,
		"completion_target":           legacyJSON(body.CompletionTarget, json.RawMessage(`null`)),
		"wecom_tagging":               legacyJSON(body.WeComTagging, json.RawMessage(`{}`)),
		"slices":                      legacyJSON(body.Slices, json.RawMessage(`[]`)),
	})
	if err != nil {
		return productport.CreateCommand{}, errInvalidLegacyProduct
	}
	projection, err = productapp.CanonicalLegacyAdminProjection(projection)
	if err != nil {
		return productport.CreateCommand{}, errInvalidLegacyProduct
	}
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		digest := sha256.Sum256([]byte(strings.TrimSpace(body.ProductCode)))
		idempotencyKey = "legacy-product-code:" + hex.EncodeToString(digest[:])
	}
	return productport.CreateCommand{
		ProductCode: strings.TrimSpace(body.ProductCode), Name: name, Description: body.Description,
		PriceMinor: *body.PriceCents, Currency: body.Currency, StockQuantity: 0, Images: []string{},
		LegacyAdminProjection: projection, Actor: actor, IdempotencyKey: idempotencyKey,
	}, nil
}

func legacyJSON(value, fallback json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return fallback
	}
	return value
}

func legacyProductPage(request *http.Request) (int32, int32, error) {
	query := request.URL.Query()
	for key := range query {
		if key != "limit" && key != "offset" {
			return 0, 0, errInvalidLegacyProduct
		}
	}
	limit, offset := int64(productapp.DefaultLimit), int64(0)
	var err error
	if value := strings.TrimSpace(query.Get("limit")); value != "" {
		limit, err = strconv.ParseInt(value, 10, 32)
		if err != nil {
			return 0, 0, errInvalidLegacyProduct
		}
	}
	if value := strings.TrimSpace(query.Get("offset")); value != "" {
		offset, err = strconv.ParseInt(value, 10, 32)
		if err != nil {
			return 0, 0, errInvalidLegacyProduct
		}
	}
	if limit < 1 || limit > int64(productapp.MaximumLimit) || offset < 0 || offset > int64(productapp.MaximumLegacyOffset) {
		return 0, 0, errInvalidLegacyProduct
	}
	return int32(limit), int32(offset), nil
}

func legacyProduct(product productport.Product) (map[string]any, error) {
	projection, err := productapp.CanonicalLegacyAdminProjection(product.LegacyAdminProjection)
	if err != nil {
		return nil, productapp.ErrUnavailable
	}
	var fields map[string]any
	if json.Unmarshal(projection, &fields) != nil {
		return nil, productapp.ErrUnavailable
	}
	delete(fields, "schema_version")
	fields["id"] = int64(product.ID)
	fields["product_code"] = product.ProductCode
	fields["title"] = product.Name
	fields["name"] = product.Name
	fields["description"] = product.Description
	fields["price_cents"] = product.PriceMinor
	fields["amount_total"] = product.PriceMinor
	fields["currency"] = product.Currency
	fields["stock_quantity"] = product.StockQuantity
	fields["images"] = append([]string(nil), product.Images...)
	fields["sold_count"] = int64(0)
	fields["created_at"] = product.CreatedAt.UTC()
	fields["updated_at"] = product.UpdatedAt.UTC()
	return fields, nil
}

func legacyProductEnvelope(values map[string]any) map[string]any {
	values["source_status"] = "v2_product_catalog"
	values["fallback_used"] = false
	values["real_external_call_executed"] = false
	values["payment_request_executed"] = false
	values["real_wechat_pay_executed"] = false
	values["real_alipay_executed"] = false
	values["provider_signature_verified"] = false
	values["real_refund_executed"] = false
	return values
}

func writeLegacyProductError(writer http.ResponseWriter, request *http.Request, err error) {
	// The immutable legacy helper classifies contract/not-found/internal errors
	// as 400/404/500. The v2 gateway then emits its unified safe error body.
	code := platformhttp.CodeInternal
	switch {
	case errors.Is(err, errInvalidLegacyProduct), errors.Is(err, productapp.ErrInvalidProduct), errors.Is(err, productapp.ErrInvalidCursor):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, productapp.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, productapp.ErrConflict):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, authport.ErrUnauthorized):
		code = platformhttp.CodeUnauthorized
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
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
		authport.CapabilityProductsRead, authport.CapabilityProductsWrite,
		authport.CapabilityQuestionnairesRead, authport.CapabilityQuestionnairesWrite,
		authport.CapabilityChannelsRead, authport.CapabilityChannelsWrite,
		authport.CapabilityMessageArchiveRead, authport.CapabilityMessageArchiveExecute,
		authport.CapabilityMessageArchiveExternalRead,
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
	http.SetCookie(writer, &http.Cookie{Name: LegacyCSRFCookieName, Value: cookie.Value, Path: "/", MaxAge: int(legacySessionMaxAge.Seconds()), Secure: true, HttpOnly: false, SameSite: http.SameSiteStrictMode})
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
