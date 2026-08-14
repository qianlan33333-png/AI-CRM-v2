package outbound_acceptance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestO7QueriesRealO6AttemptAndControlFactsWithOwnerScope(t *testing.T) {
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6B1CancelFixture(t, ctx, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerTx := tx
	defer func() { _ = ownerTx.Rollback(context.Background()) }()
	owner, err := contactfixture.CreateStaff(ctx, tx, fmt.Sprintf("outbound-o7-owner-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	customers := createOutboundCustomers(t, ctx, pool, 2)
	for _, customerID := range customers {
		if err = contactfixture.AssignCustomerOwner(ctx, tx, customerID, owner); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	batch, err := newBatchService(t, pool).Enqueue(ctx, outboundapp.EnqueueBatchCommand{
		IdempotencyScope: "legacy-admin:1", IdempotencyKey: "outbound-o7-query-batch-0001",
		Tier: outboundapp.BatchTierS, CustomerIDs: customers, TemplateKey: outboundapp.TemplateTextNoticeV1,
		Payload: []byte(`{"text":"O7 query batch"}`),
	})
	if err != nil || batch.TaskCount != 2 {
		t.Fatalf("Enqueue()=%+v err=%v", batch, err)
	}
	query := outboundapp.NewTaskQueryService(platformstore.NewUnitOfWork(pool), outboundstore.NewTaskQueryRepository())
	listed, err := query.List(ctx, outboundapp.TaskListQuery{BatchID: &batch.BatchID, OwnerStaffID: &owner, Limit: 100})
	if err != nil || len(listed.Items) != 2 || listed.HasMore {
		t.Fatalf("List()=%+v err=%v", listed, err)
	}
	task := listed.Items[0]
	wrongOwner := int64(43)
	if _, err = query.Get(ctx, outboundapp.TaskGetQuery{TaskID: task.TaskID, OwnerStaffID: &wrongOwner}); err != outboundapp.ErrTaskNotFound {
		t.Fatalf("cross-owner Get() err=%v", err)
	}

	control, err := outboundstore.NewControlRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := outboundapp.NewCancelService(platformstore.NewUnitOfWork(pool), control, eventstore.NewAppender()).Cancel(ctx, outboundapp.CancelCommand{
		TaskID: task.TaskID, IdempotencyScope: "legacy-admin:1", IdempotencyKey: "test-cancel-00000000",
	})
	if err != nil || cancelled.Status != outboundapp.TaskStatusCancelled {
		t.Fatalf("Cancel()=%+v err=%v", cancelled, err)
	}
	retried, err := outboundapp.NewManualRetryService(platformstore.NewUnitOfWork(pool), control, eventstore.NewAppender()).Retry(ctx, outboundapp.ManualRetryCommand{
		TaskID: task.TaskID, IdempotencyScope: "legacy-admin:1", IdempotencyKey: "outbound-o7-manual-retry-0001",
	})
	if err != nil || retried.Status != outboundapp.TaskStatusPending || retried.Job.Generation != 2 {
		t.Fatalf("Retry()=%+v err=%v", retried, err)
	}
	reconciled, err := query.Reconcile(ctx, outboundapp.TaskGetQuery{TaskID: task.TaskID, OwnerStaffID: &owner})
	if err != nil || reconciled.Task.Status != outboundapp.TaskStatusPending || reconciled.Task.Job.Generation != 2 ||
		len(reconciled.ControlReceipts) != 2 || reconciled.ControlReceipts[0].Operation != "cancel" ||
		reconciled.ControlReceipts[1].Operation != "manual_retry" {
		t.Fatalf("Reconcile controls=%+v err=%v", reconciled, err)
	}

	providerCustomer := createOutboundCustomer(t, ctx, pool)
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assignmentTx := tx
	defer func() { _ = assignmentTx.Rollback(context.Background()) }()
	if err = contactfixture.AssignCustomerOwner(ctx, tx, providerCustomer, owner); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	enqueued := enqueueOneFixture(t, ctx, pool, providerCustomer, "outbound-o7-attempt-query-0001", "success")
	provider := &fixtureProvider{}
	attempt, err := outboundapp.NewSenderService(
		platformstore.NewUnitOfWork(pool), outboundstore.NewSenderRepository(), eventstore.NewAppender(), provider, fixtureRateGate{},
	).Execute(ctx, outboundapp.SendCommand{RiverJobID: enqueued.RiverJobID, TaskID: enqueued.TaskID, JobKind: outboundapp.OutboundEnqueueOneJobKind})
	if err != nil || attempt.State != outboundapp.SendAttemptSucceeded {
		t.Fatalf("Execute()=%+v err=%v", attempt, err)
	}
	reconciled, err = query.Reconcile(ctx, outboundapp.TaskGetQuery{TaskID: enqueued.TaskID, OwnerStaffID: &owner})
	if err != nil || reconciled.Task.Status != outboundapp.TaskStatusSent || reconciled.Task.ProviderMessageID == "" ||
		len(reconciled.Attempts) != 1 || reconciled.Attempts[0].State != outboundapp.SendAttemptSucceeded ||
		reconciled.Attempts[0].ProviderMessageID == "" || len(reconciled.ControlReceipts) != 0 {
		t.Fatalf("Reconcile provider facts=%+v err=%v", reconciled, err)
	}
}
