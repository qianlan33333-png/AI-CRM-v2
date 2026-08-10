package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
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
	if err != nil {
		return nil, errInvalidWorkerDatabaseConfig
	}
	poolConfig.MaxConns = config.Worker.PoolMaxConns
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, errInvalidWorkerDatabaseConfig
	}
	queues := config.Worker.Queues
	workers := platformjobqueue.NewWorkerRegistry()
	router, err := eventdispatcher.NewRouter()
	if err != nil {
		pool.Close()
		return nil, err
	}
	deliveryWorker, err := eventdispatcher.NewDeliveryWorker(pool, router)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = platformjobqueue.AddWorker(workers, platformjobqueue.QueueEvent, deliveryWorker); err != nil {
		pool.Close()
		return nil, err
	}
	enqueuer := eventdispatcher.NewDeferredEnqueuer()
	dispatcher, err := eventdispatcher.New(platformstore.NewUnitOfWork(pool), enqueuer, eventdispatcher.DefaultBatchSize)
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
