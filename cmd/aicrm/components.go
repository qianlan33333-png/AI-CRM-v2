package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	automationworker "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/worker"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	contactworker "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/worker"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	externaleffectsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	operationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	orderworker "github.com/qianlan33333-png/AI-CRM-v2/internal/order/worker"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	outboundworker "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/worker"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
	segmentworker "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/worker"
	statsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/stats/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomcallback "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/callback"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
	wecomtag "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
	wecomworker "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/worker"
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
	uow := platformstore.NewUnitOfWork(pool)
	externalEffectsRuntimeRepository := externaleffectsstore.NewRepository(pool, uow)
	externalEffectsRuntime, err := eer.NewService(externalEffectsRuntimeRepository)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if config.WeCom.CustomerAcquisition.Enabled {
		acquisitionEffects, acquisitionErr := contactapp.NewChannelAcquisitionAssetEERRuntime(externalEffectsRuntime, externalEffectsRuntimeRepository)
		if acquisitionErr != nil {
			pool.Close()
			return nil, acquisitionErr
		}
		acquisitionJobs, acquisitionErr := contactstore.NewChannelAcquisitionAssetRiverJobInserter(pool)
		if acquisitionErr != nil {
			pool.Close()
			return nil, acquisitionErr
		}
		providerHTTP := &http.Client{Timeout: 5 * time.Second}
		acquisitionProvider, acquisitionErr := newChannelAcquisitionAssetProvider(config.WeCom.CustomerAcquisition, providerHTTP, time.Now)
		if acquisitionErr != nil {
			pool.Close()
			return nil, acquisitionErr
		}
		acquisitionStore := contactstore.NewChannelAcquisitionAssetRepository()
		acquisitionService, acquisitionErr := contactapp.NewChannelAcquisitionAssetService(
			uow, acquisitionStore, acquisitionEffects, acquisitionJobs, acquisitionProvider, config.WeCom.CustomerAcquisition.CorpID,
		)
		if acquisitionErr != nil {
			pool.Close()
			return nil, acquisitionErr
		}
		if acquisitionErr = contactworker.RegisterChannelAcquisitionAssetWorker(workers, acquisitionService); acquisitionErr != nil {
			pool.Close()
			return nil, acquisitionErr
		}
		acquisitionRecovery, acquisitionErr := contactapp.NewChannelAcquisitionAssetRecoveryService(uow, acquisitionStore, acquisitionJobs, time.Now)
		if acquisitionErr != nil {
			pool.Close()
			return nil, acquisitionErr
		}
		if acquisitionErr = contactworker.RegisterChannelAcquisitionAssetRecoveryWorker(workers, acquisitionRecovery); acquisitionErr != nil {
			pool.Close()
			return nil, acquisitionErr
		}
	}
	weComTagCorpID := config.WeCom.OAuth.CorpID
	if weComTagCorpID == "" {
		weComTagCorpID = config.WeCom.Callback.CorpID
	}
	if weComTagCorpID != "" {
		weComTagJobs, tagErr := wecomtag.NewRiverJobInserter(pool)
		if tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
		weComTagEffects, tagErr := wecomtag.NewService(
			uow, wecomstore.NewTagEffectRepository(pool), externalEffectsRuntime, weComTagJobs, weComTagCorpID,
		)
		if tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
		if tagErr = wecomtag.RegisterDisabledWorker(workers, weComTagEffects); tagErr != nil {
			pool.Close()
			return nil, tagErr
		}
	}
	automationOutboundMessage, err := automationstore.NewOutboundMessageHandoff(pool, uow, externalEffectsRuntime)
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignDispatchRepository, err := outboundstore.NewCampaignDispatchRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	campaignDispatchService, err := outboundapp.NewCampaignDispatchService(uow, campaignDispatchRepository, externalEffectsRuntime, campaignDispatchRepository)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = outboundworker.RegisterCampaignDispatchWorker(workers, campaignDispatchService, outboundworker.ProviderShapedAdapter{}); err != nil {
		pool.Close()
		return nil, err
	}
	financialOrders, err := orderstore.NewFinancialRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	eerRuntime, err := newPE01ExternalEffectRuntime(externalEffectsRuntimeRepository, externalEffectsRuntimeRepository)
	if err != nil {
		pool.Close()
		return nil, err
	}
	paidBenefits, err := productapp.NewPaidSettlementService(productstore.NewPaidSettlementRepository(), eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	settlement, err := orderapp.NewSettlementService(uow, financialOrders, productstore.NewCatalogRepository(), paidBenefits, eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	effectExecution, err := orderapp.NewEffectExecutionService(uow, financialOrders, eerRuntime, orderprovider.DisabledWeChatPay{}, settlement)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = orderworker.RegisterSettlementWorkers(workers, effectExecution); err != nil {
		pool.Close()
		return nil, err
	}
	commerceRefunds, err := orderstore.NewCommerceRefundRepository(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	wechatShopRefunds, err := orderapp.NewWeChatShopRefundService(uow, commerceRefunds, orderprovider.DisabledWeChatShopRefund{}, eventstore.NewAppender())
	if err != nil {
		pool.Close()
		return nil, err
	}
	if err = orderworker.RegisterWeChatShopRefundWorker(workers, wechatShopRefunds); err != nil {
		pool.Close()
		return nil, err
	}
	if err = automationworker.RegisterOutboundMessageWorker(workers, automationOutboundMessage, automationstore.DisabledOutboundMessageAdapter{}); err != nil {
		pool.Close()
		return nil, err
	}
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
	if config.WeCom.Callback.Enabled || config.WeCom.DirectorySync.Enabled {
		inboundJobs, jobErr := wecomstore.NewRiverJobInserter(pool)
		if jobErr != nil {
			pool.Close()
			return nil, jobErr
		}
		identityIngest := identityapp.NewIngestService(
			platformstore.NewUnitOfWork(pool), identitystore.NewRepository(), contactstore.NewMergePortRepository(), eventstore.NewAppender(), config.Identity.HMACKey.Value(),
		)
		processor, processorErr := wecomapp.NewIdentityContactProcessor(identityIngest)
		if processorErr != nil {
			pool.Close()
			return nil, processorErr
		}
		inboundCorpID := config.WeCom.Callback.CorpID
		if inboundCorpID == "" {
			inboundCorpID = config.WeCom.OAuth.CorpID
		}
		inboundStore := wecomstore.NewInboundRepository()
		inboundService, inboundErr := wecomapp.NewInboundService(
			platformstore.NewUnitOfWork(pool), inboundStore, inboundJobs, processor, inboundCorpID, time.Now,
		)
		if inboundErr != nil {
			pool.Close()
			return nil, inboundErr
		}
		if err = wecomworker.RegisterInboundWorker(workers, inboundService); err != nil {
			pool.Close()
			return nil, err
		}
		if config.WeCom.DirectorySync.Enabled {
			credentials, credentialErr := wecomclient.NewCredentials(config.WeCom.OAuth.CorpID, config.WeCom.OAuth.Secret.Value())
			if credentialErr != nil {
				pool.Close()
				return nil, errInvalidWorkerDatabaseConfig
			}
			providerHTTP := &http.Client{Timeout: 5 * time.Second}
			tokenProvider, tokenErr := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
				BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: providerHTTP, Now: time.Now,
			})
			if tokenErr != nil {
				pool.Close()
				return nil, errInvalidWorkerDatabaseConfig
			}
			reader, readerErr := wecomclient.NewExternalContactReader(wecomclient.ReaderConfig{
				BaseURL: wecomclient.ProductionBaseURL, HTTPClient: providerHTTP, TokenProvider: tokenProvider,
			})
			if readerErr != nil {
				pool.Close()
				return nil, errInvalidWorkerDatabaseConfig
			}
			syncService, syncErr := wecomapp.NewExternalContactSyncServiceWithHandoff(
				platformstore.NewUnitOfWork(pool), reader, wecomstore.NewSyncStateRepository(pool), inboundStore, inboundJobs,
				config.WeCom.OAuth.CorpID, time.Now,
			)
			if syncErr != nil {
				pool.Close()
				return nil, syncErr
			}
			if syncErr = wecomworker.RegisterExternalContactSyncWorker(workers, syncService); syncErr != nil {
				pool.Close()
				return nil, syncErr
			}
		}
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
	automationConsumer, err := automationstore.NewTagTriggerConsumerWithRules(
		platformstore.NewUnitOfWork(pool), automationstore.NewRepository(pool), automationstore.NewRuleRuntimeWithOutboundMessage(automationOutboundMessage), eventstore.NewAppender(), deliveries,
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
	periodicPlan, err := schedulerPlan(workers, config.WeCom.DirectorySync, config.WeCom.CustomerAcquisition)
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
