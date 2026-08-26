package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type materialApplicationStub struct {
	orderID string
	calls   int
}

func (stub *materialApplicationStub) SyncOrder(_ context.Context, orderID string) (orderport.WeChatShopOrderMaterial, error) {
	stub.calls++
	stub.orderID = orderID
	return orderport.WeChatShopOrderMaterial{ProviderOrderID: orderID}, nil
}

func (*materialApplicationStub) GetOrderMaterial(context.Context, string) (orderport.WeChatShopOrderMaterial, error) {
	return orderport.WeChatShopOrderMaterial{}, nil
}

func TestWeChatShopMaterialSyncWorkerCallsReadOnlySync(t *testing.T) {
	service := &materialApplicationStub{}
	worker := &WeChatShopMaterialSyncWorker{service: service}
	job := &river.Job[orderapp.WeChatShopMaterialSyncArgs]{JobRow: &rivertype.JobRow{ID: 91, Attempt: 1}, Args: orderapp.WeChatShopMaterialSyncArgs{ProviderOrderID: "370511505847120892812345678901"}}
	if err := worker.Work(context.Background(), job); err != nil || service.calls != 1 || service.orderID != job.Args.ProviderOrderID {
		t.Fatalf("Work err=%v calls=%d order=%q", err, service.calls, service.orderID)
	}
	job.Args.ProviderOrderID = " unsafe"
	if err := worker.Work(context.Background(), job); err != ErrInvalidWeChatShopMaterialWorker || service.calls != 1 {
		t.Fatalf("invalid Work err=%v calls=%d", err, service.calls)
	}
	if timeout := worker.Timeout(job); timeout != 20*time.Second {
		t.Fatalf("timeout=%v", timeout)
	}
}

func TestRegisterWeChatShopMaterialSyncWorkerUsesSyncQueueAndRejectsTypedNil(t *testing.T) {
	registry := platformjobqueue.NewWorkerRegistry()
	service := &materialApplicationStub{}
	if err := RegisterWeChatShopMaterialSyncWorker(registry, service); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ExplicitOptions(platformjobqueue.QueueSync, orderapp.WeChatShopMaterialSyncArgs{}, nil); err != nil {
		t.Fatalf("sync queue registration err=%v", err)
	}
	if _, err := registry.ExplicitOptions(platformjobqueue.QueueCritical, orderapp.WeChatShopMaterialSyncArgs{}, nil); err == nil {
		t.Fatal("critical queue unexpectedly accepted material sync")
	}
	var typedNil *materialApplicationStub
	if err := RegisterWeChatShopMaterialSyncWorker(platformjobqueue.NewWorkerRegistry(), typedNil); !errors.Is(err, ErrInvalidWeChatShopMaterialWorker) {
		t.Fatalf("typed nil registration err=%v", err)
	}
}
