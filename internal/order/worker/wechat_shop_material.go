package worker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrInvalidWeChatShopMaterialWorker = errors.New("invalid wechat shop material worker")

type WeChatShopMaterialSyncWorker struct {
	river.WorkerDefaults[orderapp.WeChatShopMaterialSyncArgs]
	service orderport.WeChatShopOrderMaterialApplication
}

func RegisterWeChatShopMaterialSyncWorker(registry *platformjobqueue.WorkerRegistry, service orderport.WeChatShopOrderMaterialApplication) error {
	if registry == nil || nilWeChatShopMaterialApplication(service) {
		return ErrInvalidWeChatShopMaterialWorker
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueSync, &WeChatShopMaterialSyncWorker{service: service})
}

func (worker *WeChatShopMaterialSyncWorker) Work(ctx context.Context, job *river.Job[orderapp.WeChatShopMaterialSyncArgs]) error {
	if worker == nil || nilWeChatShopMaterialApplication(worker.service) || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 || !validWeChatShopMaterialOrderID(job.Args.ProviderOrderID) {
		return ErrInvalidWeChatShopMaterialWorker
	}
	_, err := worker.service.SyncOrder(ctx, job.Args.ProviderOrderID)
	return err
}

func (*WeChatShopMaterialSyncWorker) Timeout(*river.Job[orderapp.WeChatShopMaterialSyncArgs]) time.Duration {
	return 20 * time.Second
}

func validWeChatShopMaterialOrderID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character == 0x7f {
			return false
		}
	}
	return true
}

func nilWeChatShopMaterialApplication(value orderport.WeChatShopOrderMaterialApplication) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
