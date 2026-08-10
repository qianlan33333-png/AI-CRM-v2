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
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errInvalidAPIComponent = errors.New("invalid API component")

type apiComponent struct {
	server  *http.Server
	pool    *pgxpool.Pool
	listen  func(string, string) (net.Listener, error)
	address string
}

func newAPIComponent(config appconfig.Root) (appruntime.Component, error) {
	poolConfig, err := pgxpool.ParseConfig(config.Database.URL.Value())
	if err != nil || config.API.PoolMaxConns < 1 || config.API.ListenAddress == "" {
		return nil, errInvalidAPIComponent
	}
	poolConfig.MaxConns = config.API.PoolMaxConns
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, errInvalidAPIComponent
	}
	service, err := authapp.NewService(platformstore.NewUnitOfWork(pool), authstore.NewRepository(), authapp.Options{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		pool.Close()
		return nil, err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	handler, err := newAPIHandler(logger, authHandler, authHandler)
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
	if logger == nil || authHandler == nil || candidate == nil {
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

	wrapper := &api.ServerInterfaceWrapper{Handler: candidate, ErrorHandlerFunc: platformhttp.RequestErrorHandler}
	register := func(method, pattern string, capability authport.Capability, endpoint http.Handler) error {
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
		endpoint        http.Handler
	}{
		{http.MethodGet, "/api/v1/admin/config/overview", authport.CapabilityConfigOverviewRead, http.HandlerFunc(wrapper.GetAdminConfigOverview)},
		{http.MethodPost, "/api/v1/auth/logout", authport.CapabilityAuthSessionLogout, http.HandlerFunc(wrapper.LogoutAdmin)},
		{http.MethodGet, "/api/v1/auth/session", authport.CapabilityAuthSessionRead, http.HandlerFunc(wrapper.GetAuthSession)},
		{http.MethodGet, "/api/v1/customers", authport.CapabilityCustomersRead, http.HandlerFunc(wrapper.ListCustomers)},
		{http.MethodGet, "/api/v1/customers/{customer_id}", authport.CapabilityCustomersRead, http.HandlerFunc(wrapper.GetCustomer)},
		{http.MethodPatch, "/api/v1/customers/{customer_id}", authport.CapabilityCustomersWrite, http.HandlerFunc(wrapper.UpdateCustomer)},
		{http.MethodGet, "/api/v1/customers/{customer_id}/events", authport.CapabilityCustomerEventsRead, http.HandlerFunc(wrapper.ListCustomerEvents)},
		{http.MethodPost, "/api/v1/identity/bind", authport.CapabilityIdentityBind, http.HandlerFunc(wrapper.BindIdentity)},
		{http.MethodPost, "/api/v1/identity/ingest", authport.CapabilityIdentityIngest, http.HandlerFunc(wrapper.IngestIdentityEvent)},
		{http.MethodPost, "/api/v1/identity/resolve", authport.CapabilityIdentityResolve, http.HandlerFunc(wrapper.ResolveIdentity)},
	}
	for _, route := range routes {
		if err = register(route.method, route.pattern, route.capability, route.endpoint); err != nil {
			return nil, err
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
