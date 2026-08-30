package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	"github.com/riverqueue/river"
)

var whitelistRoutePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^/api/v1/products(?:/[1-9][0-9]*(?:/local-entitlements)?)?$`),
	regexp.MustCompile(`^/api/v1/product-entitlements/[1-9][0-9]*$`),
	regexp.MustCompile(`^/api/v1/segments(?:/[1-9][0-9]*(?:/members)?)?$`),
	regexp.MustCompile(`^/api/admin/wechat-pay/products(?:/[1-9][0-9]*(?:/(?:enable|disable|copy))?)?$`),
	regexp.MustCompile(`^/api/admin/service-period-products(?:/[1-9][0-9]*(?:/(?:enable|disable|copy|members(?:/[^/]+(?:/fields)?)?|member-views(?:/[1-9][0-9]*)?))?)?$`),
	regexp.MustCompile(`^/api/admin/orders$`),
	regexp.MustCompile(`^/api/admin/questionnaires(?:/preflight|/[1-9][0-9]*(?:/(?:duplicate|disable|enable|results|submissions))?)?$`),
	regexp.MustCompile(`^/api/admin/radar-links(?:/new/options|/[1-9][0-9]*(?:/(?:enable|disable))?)?$`),
	regexp.MustCompile(`^/api/admin/channels(?:/[1-9][0-9]*)?$`),
	regexp.MustCompile(`^/api/admin/ai-audience/(?:package-groups(?:/[1-9][0-9]*)?|packages(?:/[1-9][0-9]*(?:/(?:copy|pause|configuration|configuration-preview|configuration-materialize|members))?)?)$`),
	regexp.MustCompile(`^/api/admin/automation-agents(?:/[1-9][0-9]*(?:/(?:fixed-content|precheck|activate|copy|pause|publish))?)?$`),
	regexp.MustCompile(`^/api/admin/hxc-current$`),
	regexp.MustCompile(`^/api/sidebar/v2/(?:oauth/(?:start|callback)|questionnaires|orders|periodic-orders(?:/[1-9][0-9]*/members/[^/]+/remark)?|shareable-products)$`),
}

func newWhitelistAPIComponent(config appconfig.Root) (appruntime.Component, error) {
	component, err := newAPIComponent(config)
	if err != nil {
		return nil, err
	}
	api, ok := component.(*apiComponent)
	if !ok || api.server == nil || api.server.Handler == nil || api.pool == nil {
		return nil, errInvalidAPIComponent
	}
	api.server.Handler = whitelistGateway(api.server.Handler, func(ctx context.Context) error {
		return checkWhitelistReadiness(ctx, api.pool)
	})
	return api, nil
}

func newWhitelistWorkerComponent(config appconfig.Root) (appruntime.Component, error) {
	poolConfig, err := pgxpool.ParseConfig(config.Database.URL.Value())
	if err != nil || poolConfig.ConnConfig.DescriptionCacheCapacity < 1 {
		return nil, errInvalidWorkerDatabaseConfig
	}
	poolConfig.MaxConns = config.Worker.PoolMaxConns
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, errInvalidWorkerDatabaseConfig
	}
	queues := config.Worker.Queues
	workers := platformjobqueue.NewWorkerRegistry()
	if err = platformjobqueue.AddWorker(workers, platformjobqueue.QueueCritical, &whitelistInertWorker{}); err != nil {
		pool.Close()
		return nil, err
	}
	client, err := platformjobqueue.NewClient(pool, platformjobqueue.QueueConcurrency{
		Critical: queues.Critical,
		Event:    queues.Event,
		Outbound: queues.Outbound,
		Sync:     queues.Sync,
		Heavy:    queues.Heavy,
		AI:       queues.AI,
	}, workers)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &workerComponent{runtime: platformriver.NewRuntime(client), pool: pool}, nil
}

// River requires at least one registered kind before it can start. This kind
// has no producer and cancels any manually forged job without side effects.
type whitelistInertJobArgs struct{}

func (whitelistInertJobArgs) Kind() string { return "aicrm_whitelist_inert_v1" }

type whitelistInertWorker struct {
	river.WorkerDefaults[whitelistInertJobArgs]
}

func (*whitelistInertWorker) Work(context.Context, *river.Job[whitelistInertJobArgs]) error {
	return river.JobCancel(errors.New("whitelist inert job cannot execute"))
}

func whitelistGateway(next http.Handler, readinessCheck func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/readyz" {
			ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
			defer cancel()
			if readinessCheck == nil || readinessCheck(ctx) != nil {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusServiceUnavailable)
				_, _ = writer.Write([]byte("{\"status\":\"not_ready\"}\n"))
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("{\"status\":\"ready\",\"schema\":\"aicrm-v2-core/v1\"}\n"))
			return
		}
		if whitelistRouteAllowed(request.Method, request.URL.Path) {
			next.ServeHTTP(writer, request)
			return
		}
		http.NotFound(writer, request)
	})
}

func whitelistRouteAllowed(method, path string) bool {
	if method == http.MethodGet && (path == "/healthz" || path == "/api/v1/auth/session" || path == "/login" || path == "/logout" || path == "/auth/wecom/start" || path == "/auth/wecom/callback") {
		return true
	}
	if method == http.MethodOptions && (path == "/login" || path == "/logout" || path == "/auth/wecom/start" || path == "/auth/wecom/callback") {
		return true
	}
	for _, pattern := range whitelistRoutePatterns {
		if pattern.MatchString(path) {
			return whitelistMethodAllowed(method, path)
		}
	}
	return false
}

func whitelistMethodAllowed(method, path string) bool {
	if method == http.MethodOptions {
		return true
	}
	if path == "/api/admin/orders" || path == "/api/admin/hxc-current" || regexp.MustCompile(`^/api/admin/orders/`).MatchString(path) ||
		path == "/api/sidebar/v2/orders" || path == "/api/sidebar/v2/questionnaires" || path == "/api/sidebar/v2/shareable-products" {
		return method == http.MethodGet
	}
	if regexp.MustCompile(`^/api/v1/(?:product-entitlements/|products/[1-9][0-9]*/local-entitlements|segments/[1-9][0-9]*/members)`).MatchString(path) {
		return method == http.MethodGet
	}
	if path == "/api/admin/automation-agents" || regexp.MustCompile(`^/api/admin/automation-agents/`).MatchString(path) {
		return method == http.MethodGet || method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
	}
	return method == http.MethodGet || method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func checkWhitelistReadiness(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("database unavailable")
	}
	if err := pool.Ping(ctx); err != nil {
		return errors.New("database unavailable")
	}
	var database string
	var schemaVersion int
	var riverVersion int
	var requiredTables bool
	err := pool.QueryRow(ctx, `SELECT current_database(),
  (SELECT version FROM public.whitelist_schema_version WHERE singleton),
  (SELECT COALESCE(max(version),0) FROM public.river_migration WHERE line='main'),
  to_regclass('public.admin_users') IS NOT NULL AND
  to_regclass('public.event_log') IS NOT NULL AND
  to_regclass('public.river_job') IS NOT NULL AND
  to_regclass('public.customers') IS NOT NULL AND
  to_regclass('public.products') IS NOT NULL AND
  to_regclass('public.order_list_projections') IS NOT NULL AND
  to_regclass('public.questionnaires') IS NOT NULL AND
  to_regclass('public.product_local_entitlements') IS NOT NULL AND
  to_regclass('public.radar_links') IS NOT NULL AND
  to_regclass('public.channels') IS NOT NULL AND
  to_regclass('public.segments') IS NOT NULL AND
  to_regclass('public.automation_agent_configurations') IS NOT NULL AND
  to_regclass('public.hxc_user_current') IS NOT NULL`).Scan(&database, &schemaVersion, &riverVersion, &requiredTables)
	if err != nil {
		return errors.New("whitelist readiness query failed")
	}
	if database != "aicrm_v2_core" || schemaVersion != 1 || riverVersion < 6 || !requiredTables {
		return fmt.Errorf("whitelist runtime is incompatible")
	}
	return nil
}
