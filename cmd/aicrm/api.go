package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	healthapi "github.com/qianlan33333-png/AI-CRM-v2/internal/api/generated"
	authapp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/app"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contacthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/http"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/http"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmenthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/http"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
	wecomcallback "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/callback"
)

var errInvalidAPIComponent = errors.New("invalid API component")

type apiComponent struct {
	server  *http.Server
	pool    *pgxpool.Pool
	listen  func(string, string) (net.Listener, error)
	address string
}

type candidateHandler struct {
	*authhttp.Handler
	customers       *contacthttp.CustomerListHandler
	customerDetail  *contacthttp.CustomerDetailHandler
	customerEvents  *contacthttp.CustomerEventHandler
	mutations       *contacthttp.CustomerMutationHandler
	tags            *contacthttp.TagCatalogHandler
	stages          *contacthttp.Handler
	segments        *segmenthttp.CRUDHandler
	segmentRefresh  *segmenthttp.RefreshHandler
	identityReviews *identityhttp.ReviewHandler
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

func (handler *candidateHandler) ListTags(writer http.ResponseWriter, request *http.Request) {
	handler.tags.ListTags(writer, request)
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

func (handler *candidateHandler) RenameStage(writer http.ResponseWriter, request *http.Request, stageID api.StageID, params api.RenameStageParams) {
	handler.stages.RenameStage(writer, request, stageID, params)
}

func (handler *candidateHandler) RequestSegmentRefresh(writer http.ResponseWriter, request *http.Request, segmentID api.SegmentID, params api.RequestSegmentRefreshParams) {
	handler.segmentRefresh.RequestSegmentRefresh(writer, request, segmentID, params)
}

func (handler *candidateHandler) ListSegments(writer http.ResponseWriter, request *http.Request, params api.ListSegmentsParams) {
	handler.segments.ListSegments(writer, request, params)
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
	service, err := authapp.NewService(uow, authstore.NewRepository(), authapp.Options{})
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
		uow, contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(),
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
	identityRepository := identitystore.NewRepository()
	identityReviewHandler, err := identityhttp.NewReviewHandler(identityapp.NewMergeReviewService(
		uow, identityRepository, contactstore.NewMergePortRepository(), eventstore.NewAppender(), config.Identity.HMACKey.Value(),
	))
	if err != nil {
		pool.Close()
		return nil, err
	}
	candidate := &candidateHandler{
		Handler: authHandler, customers: customerHandler,
		customerDetail: customerDetailHandler, customerEvents: customerEventHandler,
		mutations: mutationHandler, tags: tagCatalogHandler, stages: stageHandler,
		segments:        segmentCRUDHandler,
		segmentRefresh:  segmentRefreshHandler,
		identityReviews: identityReviewHandler,
	}
	legacyHandler, err := NewHandler(service, customerService)
	if err != nil {
		pool.Close()
		return nil, err
	}
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
	handler, err := newAPIHandlerWithCallbackAndLegacy(logger, callbackHandler, authHandler, candidate, legacyHandler)
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

	wrapper := &api.ServerInterfaceWrapper{Handler: candidate, ErrorHandlerFunc: platformhttp.RequestErrorHandler}
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
		{http.MethodPut, "/api/v1/customers/{customer_id}/stage", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.SetCustomerStage)},
		{http.MethodPut, "/api/v1/customers/{customer_id}/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.AddCustomerTag)},
		{http.MethodDelete, "/api/v1/customers/{customer_id}/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.RemoveCustomerTag)},
		{http.MethodGet, "/api/v1/tags", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListTags)},
		{http.MethodPost, "/api/v1/identity/bind", authport.CapabilityIdentityBind, true, http.HandlerFunc(wrapper.BindIdentity)},
		{http.MethodPost, "/api/v1/identity/ingest", authport.CapabilityIdentityIngest, true, http.HandlerFunc(wrapper.IngestIdentityEvent)},
		{http.MethodPost, "/api/v1/identity/resolve", authport.CapabilityIdentityResolve, false, http.HandlerFunc(wrapper.ResolveIdentity)},
		{http.MethodGet, "/api/v1/identity/merge-reviews", authport.CapabilityIdentityReviewRead, false, http.HandlerFunc(wrapper.ListIdentityMergeReviews)},
		{http.MethodPost, "/api/v1/identity/merge-reviews/{review_id}/approve", authport.CapabilityIdentityReviewWrite, true, http.HandlerFunc(wrapper.ApproveIdentityMergeReview)},
		{http.MethodPost, "/api/v1/identity/merge-reviews/{review_id}/reject", authport.CapabilityIdentityReviewWrite, true, http.HandlerFunc(wrapper.RejectIdentityMergeReview)},
		{http.MethodGet, "/api/v1/segments", authport.CapabilitySegmentsRead, false, http.HandlerFunc(wrapper.ListSegments)},
		{http.MethodPost, "/api/v1/segments", authport.CapabilitySegmentsWrite, true, http.HandlerFunc(wrapper.CreateSegment)},
		{http.MethodPatch, "/api/v1/segments/{segment_id}", authport.CapabilitySegmentsWrite, true, http.HandlerFunc(wrapper.UpdateSegment)},
		{http.MethodGet, "/api/v1/segments/{segment_id}/members", authport.CapabilitySegmentsRead, false, http.HandlerFunc(wrapper.ListSegmentMembers)},
		{http.MethodPost, "/api/v1/segments/{segment_id}/refresh", authport.CapabilitySegmentsWrite, true, http.HandlerFunc(wrapper.RequestSegmentRefresh)},
		{http.MethodGet, "/api/v1/stages", authport.CapabilityStagesRead, false, http.HandlerFunc(wrapper.ListStages)},
		{http.MethodPost, "/api/v1/stages", authport.CapabilityStagesWrite, true, http.HandlerFunc(wrapper.CreateStage)},
		{http.MethodPatch, "/api/v1/stages/{stage_id}", authport.CapabilityStagesWrite, true, http.HandlerFunc(wrapper.RenameStage)},
	}
	for _, route := range routes {
		if err = register(route.method, route.pattern, route.capability, route.csrf, route.endpoint); err != nil {
			return nil, err
		}
	}
	if legacy != nil {
		registerLegacy := func(method, pattern string, capability authport.Capability, endpoint http.Handler) error {
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
			tail, wrapErr = legacy.Authorize(capability, tail)
			if wrapErr != nil {
				return wrapErr
			}
			tail = legacy.Authenticate(tail)
			tail, wrapErr = gateway.RoutePatternMiddleware(pattern, tail)
			if wrapErr != nil {
				return wrapErr
			}
			router.Method(method, pattern, tail)
			return nil
		}
		for _, route := range []struct {
			method, pattern string
			capability      authport.Capability
			endpoint        http.Handler
		}{
			{http.MethodGet, "/api/admin/config/overview", authport.CapabilityConfigOverviewRead, http.HandlerFunc(legacy.ConfigOverview)},
			{http.MethodGet, "/api/admin/config/capabilities", authport.CapabilityConfigOverviewRead, http.HandlerFunc(legacy.Capabilities)},
			{http.MethodGet, "/api/customers", authport.CapabilityCustomersRead, http.HandlerFunc(legacy.ListCustomers)},
		} {
			if err = registerLegacy(route.method, route.pattern, route.capability, route.endpoint); err != nil {
				return nil, err
			}
		}
	}
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
	router.NotFound(notFound.ServeHTTP)
	router.MethodNotAllowed(methodNotAllowed.ServeHTTP)
	return gateway.RequestIDMiddleware(router)
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
