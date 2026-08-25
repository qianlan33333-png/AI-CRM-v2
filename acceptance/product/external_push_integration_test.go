package product_acceptance

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

func TestCommerceExternalPushPG16RoundTrip(t *testing.T) {
	pool, ctx := openExternalPushPool(t)
	uow := platformstore.NewUnitOfWork(pool)
	runtime, err := eer.NewService(eerstore.NewRepository(pool, uow))
	if err != nil {
		t.Fatal(err)
	}
	accepter := productstore.NewCommerceExternalPushEERAccepter(runtime)
	service := productapp.NewCommerceExternalPushService(uow, productstore.NewCatalogRepository(), accepter)
	product := mustCreateExternalPushProduct(t, ctx, pool)
	actor, saveKey, testKey := int64(8817), "external-push-save-"+uniqueCode("key"), "external-push-test-"+uniqueCode("key")

	saved, err := service.SaveExternalPushConfiguration(ctx, productport.SaveExternalPushConfigurationCommand{
		ProductID: product.ID, ProductKind: productport.ExternalPushWeChatPay, Enabled: true,
		ConfigurationReference: "local-payment-product-" + fmt.Sprint(product.ID), Actor: actor, IdempotencyKey: saveKey,
	})
	if err != nil || !saved.Enabled || saved.ProductID != product.ID || saved.ProductKind != productport.ExternalPushWeChatPay || saved.ConfigurationReference == "" {
		t.Fatalf("save=%+v err=%v", saved, err)
	}
	replayedSave, err := service.SaveExternalPushConfiguration(ctx, productport.SaveExternalPushConfigurationCommand{
		ProductID: product.ID, ProductKind: productport.ExternalPushWeChatPay, Enabled: true,
		ConfigurationReference: saved.ConfigurationReference, Actor: actor, IdempotencyKey: saveKey,
	})
	if err != nil || !sameExternalPushConfiguration(replayedSave, saved) {
		t.Fatalf("save replay=%+v err=%v want=%+v", replayedSave, err, saved)
	}
	read, err := service.GetExternalPushConfiguration(ctx, product.ID, productport.ExternalPushWeChatPay)
	if err != nil || !sameExternalPushConfiguration(read, saved) {
		t.Fatalf("read=%+v err=%v want=%+v", read, err, saved)
	}

	accepted, err := service.QueueExternalPushTest(ctx, productport.QueueExternalPushTestCommand{
		ProductID: product.ID, ProductKind: productport.ExternalPushWeChatPay, Actor: actor, IdempotencyKey: testKey,
	})
	if err != nil || accepted.EffectID == "" || accepted.State != "accepted" || accepted.ProviderAccepted || accepted.DeliveryProven || accepted.RealExternalCallExecuted || accepted.AutoRetryAllowed {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	replayedTest, err := service.QueueExternalPushTest(ctx, productport.QueueExternalPushTestCommand{
		ProductID: product.ID, ProductKind: productport.ExternalPushWeChatPay, Actor: actor, IdempotencyKey: testKey,
	})
	if err != nil || !sameExternalPushTest(replayedTest, accepted) {
		t.Fatalf("test replay=%+v err=%v want=%+v", replayedTest, err, accepted)
	}

	periodService := productapp.NewServicePeriodService(uow, productstore.NewCatalogRepository(), eventstore.NewAppender())
	period, err := periodService.CreateServicePeriodProduct(ctx, productport.CreateServicePeriodProductCommand{
		ProductCode: uniqueCode("external-push-period"), Name: "周期商品外推测试", Description: "local only", PriceMinor: 1,
		Currency: "CNY", StockQuantity: 0, Actor: actor, IdempotencyKey: "external-push-period-create-" + uniqueCode("key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	periodSaved, err := service.SaveExternalPushConfiguration(ctx, productport.SaveExternalPushConfigurationCommand{
		ProductID: period.ServiceProductID, ProductKind: productport.ExternalPushServicePeriod, Enabled: true,
		ConfigurationReference: "local-period-product-" + fmt.Sprint(period.ServiceProductID), Actor: actor, IdempotencyKey: "external-push-period-save-" + uniqueCode("key"),
	})
	if err != nil || !periodSaved.Enabled || periodSaved.ProductID != period.ServiceProductID || periodSaved.ProductKind != productport.ExternalPushServicePeriod {
		t.Fatalf("period save=%+v err=%v", periodSaved, err)
	}
	periodAccepted, err := service.QueueExternalPushTest(ctx, productport.QueueExternalPushTestCommand{
		ProductID: period.ServiceProductID, ProductKind: productport.ExternalPushServicePeriod, Actor: actor, IdempotencyKey: "external-push-period-test-" + uniqueCode("key"),
	})
	if err != nil || periodAccepted.State != "accepted" || periodAccepted.ProviderAccepted || periodAccepted.DeliveryProven || periodAccepted.RealExternalCallExecuted || periodAccepted.AutoRetryAllowed {
		t.Fatalf("period accepted=%+v err=%v", periodAccepted, err)
	}
	var configurations, bindings, effects, attempts, queuedJobs, acceptedFacts, deliveredFacts, providerFacts, executedFacts, retryFacts int
	err = pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.product_external_push_configurations WHERE product_id=$1 AND product_kind='wechat_pay' AND enabled AND configuration_reference=$2),
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=$1 AND product_kind='wechat_pay' AND state='accepted' AND NOT provider_accepted AND NOT delivery_proven AND NOT real_external_call_executed AND NOT auto_retry_allowed),
  (SELECT count(*) FROM public.external_effects WHERE owner='product' AND kind='product_external_push_test' AND state='accepted'),
  (SELECT count(*) FROM public.external_effect_attempts),
  (SELECT count(*) FROM public.external_effects WHERE river_job_id IS NOT NULL),
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=$1 AND state='accepted'),
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=$1 AND delivery_proven),
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=$1 AND provider_accepted),
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=$1 AND real_external_call_executed),
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=$1 AND auto_retry_allowed)`,
		int64(product.ID), saved.ConfigurationReference,
	).Scan(&configurations, &bindings, &effects, &attempts, &queuedJobs, &acceptedFacts, &deliveredFacts, &providerFacts, &executedFacts, &retryFacts)
	if err != nil || configurations != 1 || bindings != 1 || effects < 1 || attempts != 0 || queuedJobs != 0 || acceptedFacts != 1 || deliveredFacts != 0 || providerFacts != 0 || executedFacts != 0 || retryFacts != 0 {
		t.Fatalf("facts config/binding/effects/attempts/jobs/accepted/delivered/provider/executed/retry=%d/%d/%d/%d/%d/%d/%d/%d/%d/%d err=%v", configurations, bindings, effects, attempts, queuedJobs, acceptedFacts, deliveredFacts, providerFacts, executedFacts, retryFacts, err)
	}
	var periodConfigurations, periodBindings int
	err = pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.product_external_push_configurations WHERE product_id=$1 AND product_kind='service_period' AND enabled AND configuration_reference=$2),
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=$1 AND product_kind='service_period' AND state='accepted' AND NOT provider_accepted AND NOT delivery_proven AND NOT real_external_call_executed AND NOT auto_retry_allowed)`,
		int64(period.ServiceProductID), periodSaved.ConfigurationReference,
	).Scan(&periodConfigurations, &periodBindings)
	if err != nil || periodConfigurations != 1 || periodBindings != 1 {
		t.Fatalf("period facts config/binding=%d/%d err=%v", periodConfigurations, periodBindings, err)
	}
}

func sameExternalPushConfiguration(left, right productport.ExternalPushConfiguration) bool {
	return left.ProductID == right.ProductID &&
		left.ProductKind == right.ProductKind &&
		left.Enabled == right.Enabled &&
		left.ConfigurationReference == right.ConfigurationReference &&
		left.UpdatedAt.Equal(right.UpdatedAt)
}

func sameExternalPushTest(left, right productport.ExternalPushTest) bool {
	return left.ProductID == right.ProductID &&
		left.ProductKind == right.ProductKind &&
		left.EffectID == right.EffectID &&
		left.State == right.State &&
		left.ProviderAccepted == right.ProviderAccepted &&
		left.DeliveryProven == right.DeliveryProven &&
		left.RealExternalCallExecuted == right.RealExternalCallExecuted &&
		left.AutoRetryAllowed == right.AutoRetryAllowed &&
		left.CreatedAt.Equal(right.CreatedAt)
}

func mustCreateExternalPushProduct(t *testing.T, ctx context.Context, pool *pgxpool.Pool) productport.Product {
	t.Helper()
	service := realService(pool)
	product, err := service.Create(ctx, productport.CreateCommand{
		ProductCode: uniqueCode("external-push"), Name: "本地外推测试商品", Description: "local only", PriceMinor: 1,
		Currency: "CNY", StockQuantity: 0, Images: []string{}, LegacyAdminProjection: productapp.DefaultLegacyAdminProjection(),
		Actor: 8817, IdempotencyKey: uniqueCode("external-push-create"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return product
}

func openExternalPushPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *i01aDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*i01aDatabaseURL, acceptancefixtures.CommerceExternalPushDatabaseName); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *i01aDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q error=%v", version, err)
	}
	return pool, ctx
}
