package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	contactworker "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/worker"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	operationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
	segmentworker "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/worker"
	statsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/stats/store"
	wecomcallback "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/callback"
)

var errInvalidWorkerDatabaseConfig = errors.New("invalid worker database configuration")

type componentBuilders struct {
	api    func(appconfig.Root) (appruntime.Component, error)
	worker func(appconfig.Root) (appruntime.Component, error)
}

func components(role appruntime.Role, config appconfig.Root) (appruntime.Components, error) {
	return buildComponents(role, config, componentBuilders{api: newAPIComponent, worker: newWorkerComponent})
}

func buildComponents(role appruntime.Role, config appconfig.Root, builders componentBuilders) (appruntime.Components, error) {
	var selected appruntime.Components
	if role == appruntime.RoleAPI || role == appruntime.RoleAll {
		api, err := builders.api(config)
		if err != nil {
			return appruntime.Components{}, err
		}
		selected.API = api
	}
	if role == appruntime.RoleWorker || role == appruntime.RoleAll {
		worker, err := builders.worker(config)
		if err != nil {
			return appruntime.Components{}, err
		}
		selected.Worker = worker
	}
	if role != appruntime.RoleAPI && role != appruntime.RoleWorker && role != appruntime.RoleAll {
		return appruntime.Components{}, appruntime.ErrInvalidRole
	}
	return selected, nil
}

func newWorkerComponent(config appconfig.Root) (appruntime.Component, error) {
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
	partitionMaintainer, err := contactstore.NewEventPartitionMaintainer(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	partitionWorker, err := contactworker.NewEventPartitionMaintenanceWorker(partitionMaintainer, time.Now)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = platformjobqueue.AddWorker(workers, platformjobqueue.QueueHeavy, partitionWorker); err != nil {
		pool.Close()
		return nil, err
	}
	scheduledRefreshes, err := segmentstore.NewScheduledRefreshRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	refreshWorker, err := segmentworker.NewScheduledRefreshWorker(
		scheduledRefreshes,
		segmentapp.NewRefreshService(platformstore.NewUnitOfWork(pool), segmentstore.NewRefreshRepository(), eventstore.NewAppender()),
		time.Now,
	)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = platformjobqueue.AddWorker(workers, platformjobqueue.QueueHeavy, refreshWorker); err != nil {
		pool.Close()
		return nil, err
	}
	enqueuer := eventdispatcher.NewDeferredEnqueuer()
	deliveries, err := eventstore.NewRuntimeDeliveryRepository(pool, enqueuer, eventdispatcher.DefaultBatchSize, []eventport.DeliveryBinding{
		{EventType: eventport.EvTagApplied, Consumer: eventport.ConsumerAutomationTagTrigger},
		{EventType: eventport.EvTagApplied, Consumer: eventport.ConsumerStatsTagApplied},
		{EventType: eventport.EvOperationCycleFact, Consumer: eventport.ConsumerOperationCycleFact},
		{EventType: eventport.EvCloudCampaignFact, Consumer: eventport.ConsumerCloudCampaignFact},
		{EventType: eventport.EvOutboundCampaignHandoffFact, Consumer: eventport.ConsumerOutboundCampaignHandoffFact},
	})
	if err != nil {
		pool.Close()
		return nil, err
	}
	router, err := eventdispatcher.NewRouter(wecomcallback.NewAuditSubscriber())
	if err != nil {
		pool.Close()
		return nil, err
	}
	automationConsumer, err := automationstore.NewTagTriggerConsumer(
		platformstore.NewUnitOfWork(pool), automationstore.NewRepository(pool), eventstore.NewAppender(), deliveries,
	)
	if err == nil {
		err = router.RegisterDelivery(automationConsumer)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	statsConsumer, err := statsstore.NewTagAppliedConsumer(
		platformstore.NewUnitOfWork(pool), statsstore.NewRepository(pool), deliveries,
	)
	if err == nil {
		err = router.RegisterDelivery(statsConsumer)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	operationCycleConsumer, err := operationstore.NewFactDeliveryConsumer(platformstore.NewUnitOfWork(pool), deliveries)
	if err == nil {
		err = router.RegisterDelivery(operationCycleConsumer)
	}
	campaignConsumer, err := campaign.NewFactDeliveryConsumer(platformstore.NewUnitOfWork(pool), deliveries)
	if err == nil {
		err = router.RegisterDelivery(campaignConsumer)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	outboundCampaignHandoffConsumer, err := outbound.NewCampaignHandoffFactDeliveryConsumer(platformstore.NewUnitOfWork(pool), deliveries)
	if err == nil {
		err = router.RegisterDelivery(outboundCampaignHandoffConsumer)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	deliveryWorker, err := eventdispatcher.NewDeliveryWorker(deliveries, router)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = platformjobqueue.AddWorker(workers, platformjobqueue.QueueEvent, deliveryWorker); err != nil {
		pool.Close()
		return nil, err
	}
	dispatcher, err := eventdispatcher.New(deliveries)
	if err != nil {
		pool.Close()
		return nil, err
	}
	dispatchWorker, err := eventdispatcher.NewDispatchWorker(dispatcher)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = platformjobqueue.AddWorker(workers, platformjobqueue.QueueEvent, dispatchWorker); err != nil {
		pool.Close()
		return nil, err
	}
	periodicPlan, err := schedulerPlan(workers)
	if err != nil {
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
	}, workers, periodicPlan.Jobs()...)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = enqueuer.Bind(client); err != nil {
		pool.Close()
		return nil, err
	}
	return &workerComponent{runtime: platformriver.NewRuntime(client), pool: pool}, nil
}

type workerComponent struct {
	runtime appruntime.Component
	pool    *pgxpool.Pool
}

func (worker *workerComponent) Run(ctx context.Context) error {
	defer worker.pool.Close()
	return worker.runtime.Run(ctx)
}
