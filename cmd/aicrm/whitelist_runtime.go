package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	externaleffectsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
	wecomtag "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
	"github.com/riverqueue/river"
)

type whitelistCapabilities struct {
	weComTagCatalogSync bool
}

var whitelistRoutePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^/api/v1/products(?:/[1-9][0-9]*(?:/local-entitlements)?)?$`),
	regexp.MustCompile(`^/api/v1/product-entitlements/[1-9][0-9]*$`),
	regexp.MustCompile(`^/api/v1/segments(?:/[1-9][0-9]*(?:/members)?)?$`),
	regexp.MustCompile(`^/api/v1/customers(?:/[1-9][0-9]*(?:/(?:stage|context|chat-activity|survey-answers|activity-analytics|tags/[1-9][0-9]*))?)?$`),
	regexp.MustCompile(`^/api/v1/stages(?:/[1-9][0-9]*)?$`),
	regexp.MustCompile(`^/api/v1/tag-groups(?:/reorder|/[1-9][0-9]*)?$`),
	regexp.MustCompile(`^/api/v1/tags(?:/reorder|/[1-9][0-9]*)?$`),
	regexp.MustCompile(`^/api/v1/contact-owner-reassignments/(?:template|previews(?:/[1-9][0-9]*(?:/(?:execute|errors\.csv|results\.csv))?)?)$`),
	regexp.MustCompile(`^/api/admin/wechat-pay/products(?:/[1-9][0-9]*(?:/(?:enable|disable|copy))?)?$`),
	regexp.MustCompile(`^/api/admin/service-period-products(?:/[1-9][0-9]*(?:/(?:enable|disable|copy|members(?:/[^/]+(?:/fields)?)?|member-views(?:/[1-9][0-9]*)?))?)?$`),
	regexp.MustCompile(`^/api/admin/orders$`),
	regexp.MustCompile(`^/api/admin/coupons(?:/product-options|/[1-9][0-9]*(?:/(?:publish|stop|archive|claims|copy|share))?)?$`),
	regexp.MustCompile(`^/api/admin/image-library(?:/.*)?$`),
	regexp.MustCompile(`^/api/admin/attachment-library(?:/.*)?$`),
	regexp.MustCompile(`^/api/admin/miniprogram-library(?:/.*)?$`),
	regexp.MustCompile(`^/api/admin/wecom/(?:tag-groups(?:/[1-9][0-9]*)?|tags(?:/[1-9][0-9]*|/live/gate|/sync)?)$`),
	regexp.MustCompile(`^/api/admin/questionnaires(?:/preflight|/[1-9][0-9]*(?:/(?:duplicate|disable|enable|results|submissions))?)?$`),
	regexp.MustCompile(`^/api/admin/radar-links(?:/new/options|/[1-9][0-9]*(?:/(?:enable|disable))?)?$`),
	regexp.MustCompile(`^/api/admin/channels(?:/[1-9][0-9]*)?$`),
	regexp.MustCompile(`^/api/admin/ai-audience/(?:package-groups(?:/[1-9][0-9]*)?|packages(?:/[1-9][0-9]*(?:/(?:copy|pause|configuration|configuration-preview|configuration-materialize|members))?)?)$`),
	regexp.MustCompile(`^/api/admin/ai-audience/operation-members$`),
	regexp.MustCompile(`^/api/admin/common/operation-members$`),
	regexp.MustCompile(`^/api/admin/automation-conversion/group-ops/plans(?:/.*)?$`),
	regexp.MustCompile(`^/api/admin/automation-agents(?:/[1-9][0-9]*(?:/(?:fixed-content|precheck|activate|copy|pause|publish))?)?$`),
	regexp.MustCompile(`^/api/admin/config/(?:app-settings|categories(?:/[^/]+(?:/(?:enabled|settings|check))?)?|push-capabilities|releases)$`),
	regexp.MustCompile(`^/api/admin/hxc-current$`),
	regexp.MustCompile(`^/api/sidebar/context-token$`),
	regexp.MustCompile(`^/api/sidebar/v2/(?:oauth/(?:start|callback)|jssdk/agent-config|bootstrap|timeline|chat-activity|other-staff-chats|workbench|profile|phone-binding|questionnaires|orders|periodic-orders(?:/[1-9][0-9]*/members/[^/]+/remark)?|shareable-products|materials(?:/image/[1-9][0-9]*/(?:thumbnail|preview))?|coupons)$`),
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
	}, whitelistCapabilities{weComTagCatalogSync: config.WeCom.TagCatalog.Enabled})
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
	if config.WeCom.TagCatalog.Enabled {
		uow := platformstore.NewUnitOfWork(pool)
		externalEffectsRuntime, runtimeErr := eer.NewService(externaleffectsstore.NewRepository(pool, uow))
		if runtimeErr != nil {
			pool.Close()
			return nil, runtimeErr
		}
		tagJobs, tagErr := wecomtag.NewRiverJobInserter(pool)
		if tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
		tagEffects, tagErr := wecomtag.NewService(
			uow, wecomstore.NewTagEffectRepository(pool), externalEffectsRuntime, tagJobs, weComTagEffectCorpID(config),
		)
		if tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
		tagProvider, tagErr := newWeComTagCatalogProvider(config.WeCom.TagCatalog, &http.Client{Timeout: 5 * time.Second}, time.Now)
		if tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
		if tagErr = wecomtag.RegisterWorker(workers, tagEffects, tagProvider); tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
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

func whitelistGateway(next http.Handler, readinessCheck func(context.Context) error, capabilities whitelistCapabilities) http.Handler {
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
			_, _ = writer.Write([]byte("{\"status\":\"ready\",\"schema\":\"aicrm-v2-core/v2\"}\n"))
			return
		}
		if whitelistRouteAllowed(request.Method, request.URL.Path, capabilities) {
			next.ServeHTTP(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"code":    "NOT_FOUND",
			"message": "The requested resource is not available in the whitelist runtime.",
		})
	})
}

func whitelistRouteAllowed(method, path string, capabilities whitelistCapabilities) bool {
	if method == http.MethodGet && (path == "/healthz" || path == "/api/v1/auth/session" || path == "/login" || path == "/logout" || path == "/auth/wecom/start" || path == "/auth/wecom/callback") {
		return true
	}
	if method == http.MethodOptions && (path == "/login" || path == "/logout" || path == "/auth/wecom/start" || path == "/auth/wecom/callback") {
		return true
	}
	if path == "/api/admin/wecom/tags/sync" {
		return capabilities.weComTagCatalogSync && method == http.MethodPost
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
	if path == "/api/sidebar/context-token" || path == "/api/sidebar/v2/bootstrap" || path == "/api/sidebar/v2/phone-binding" {
		return method == http.MethodPost
	}
	if path == "/api/sidebar/v2/profile" || strings.Contains(path, "/periodic-orders/") {
		return method == http.MethodPut
	}
	if strings.HasPrefix(path, "/api/sidebar/v2/") {
		return method == http.MethodGet
	}
	if strings.Contains(path, "/automation-conversion/group-ops/plans/") &&
		(strings.Contains(path, "/run-due") || strings.Contains(path, "/executions/") && strings.HasSuffix(path, "/reconcile")) {
		return method == http.MethodGet
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
  to_regclass('public.order_refund_facts') IS NOT NULL AND
  to_regclass('public.questionnaires') IS NOT NULL AND
  EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='questionnaire_submissions' AND column_name='customer_id') AND
  to_regclass('public.product_local_entitlements') IS NOT NULL AND
  to_regclass('public.radar_links') IS NOT NULL AND
  to_regclass('public.channels') IS NOT NULL AND
  to_regclass('public.segments') IS NOT NULL AND
  to_regclass('public.automation_agent_configurations') IS NOT NULL AND
  to_regclass('public.tag_groups') IS NOT NULL AND
  EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='tag_groups' AND column_name='wecom_group_id') AND
  to_regclass('public.tags') IS NOT NULL AND
  to_regclass('public.coupons') IS NOT NULL AND
  to_regclass('public.media_images') IS NOT NULL AND
  to_regclass('public.media_attachments') IS NOT NULL AND
  to_regclass('public.media_miniprograms') IS NOT NULL AND
  to_regclass('public.group_ops_plans') IS NOT NULL AND
  to_regclass('public.admin_ops_config_categories') IS NOT NULL AND
  to_regclass('public.hxc_user_current') IS NOT NULL`).Scan(&database, &schemaVersion, &riverVersion, &requiredTables)
	if err != nil {
		return errors.New("whitelist readiness query failed")
	}
	if database != "aicrm_v2_core" || schemaVersion != 2 || riverVersion < 6 || !requiredTables {
		return fmt.Errorf("whitelist runtime is incompatible")
	}
	return nil
}
