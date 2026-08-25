package worker

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"time"

	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrInvalidSettlementWorker = errors.New("invalid PE01 settlement worker")

type PaymentEffectWorker struct {
	river.WorkerDefaults[orderapp.PaymentEffectBridgeArgs]
	service *orderapp.EffectExecutionService
}

type RefundEffectWorker struct {
	river.WorkerDefaults[orderapp.RefundEffectBridgeArgs]
	service *orderapp.EffectExecutionService
}

type PaymentReconcileWorker struct {
	river.WorkerDefaults[orderapp.PaymentReconcileArgs]
	service *orderapp.EffectExecutionService
}

type RefundReconcileWorker struct {
	river.WorkerDefaults[orderapp.RefundReconcileArgs]
	service *orderapp.EffectExecutionService
}

type WeChatShopRefundWorker struct {
	river.WorkerDefaults[orderapp.WeChatShopRefundArgs]
	service orderport.WeChatShopRefundApplication
}

func RegisterSettlementWorkers(registry *platformjobqueue.WorkerRegistry, service *orderapp.EffectExecutionService) error {
	if registry == nil || service == nil {
		return ErrInvalidSettlementWorker
	}
	if err := platformjobqueue.AddWorker(registry, platformjobqueue.QueueCritical, &PaymentEffectWorker{service: service}); err != nil {
		return err
	}
	if err := platformjobqueue.AddWorker(registry, platformjobqueue.QueueCritical, &RefundEffectWorker{service: service}); err != nil {
		return err
	}
	if err := platformjobqueue.AddWorker(registry, platformjobqueue.QueueCritical, &PaymentReconcileWorker{service: service}); err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueCritical, &RefundReconcileWorker{service: service})
}

func RegisterWeChatShopRefundWorker(registry *platformjobqueue.WorkerRegistry, service orderport.WeChatShopRefundApplication) error {
	if registry == nil || service == nil {
		return ErrInvalidSettlementWorker
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueCritical, &WeChatShopRefundWorker{service: service})
}

func (worker *PaymentEffectWorker) Work(ctx context.Context, job *river.Job[orderapp.PaymentEffectBridgeArgs]) error {
	if worker == nil || worker.service == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 || job.Args.CommandID < 1 {
		return ErrInvalidSettlementWorker
	}
	return worker.service.ExecutePayment(ctx, effectJob(job.ID, int64(job.Attempt), orderapp.PaymentEffectBridgeJobKind, job.Args.CommandID, job.ScheduledAt))
}

func (worker *RefundEffectWorker) Work(ctx context.Context, job *river.Job[orderapp.RefundEffectBridgeArgs]) error {
	if worker == nil || worker.service == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 || job.Args.RefundID < 1 {
		return ErrInvalidSettlementWorker
	}
	return worker.service.ExecuteRefund(ctx, effectJob(job.ID, int64(job.Attempt), orderapp.RefundEffectBridgeJobKind, job.Args.RefundID, job.ScheduledAt))
}

func (worker *PaymentReconcileWorker) Work(ctx context.Context, job *river.Job[orderapp.PaymentReconcileArgs]) error {
	if worker == nil || worker.service == nil || job == nil || job.JobRow == nil || job.Args.CommandID < 1 {
		return ErrInvalidSettlementWorker
	}
	return worker.service.ReconcilePayment(ctx, job.Args.CommandID)
}

func (worker *RefundReconcileWorker) Work(ctx context.Context, job *river.Job[orderapp.RefundReconcileArgs]) error {
	if worker == nil || worker.service == nil || job == nil || job.JobRow == nil || job.Args.RefundID < 1 {
		return ErrInvalidSettlementWorker
	}
	return worker.service.ReconcileRefund(ctx, job.Args.RefundID)
}

func (worker *WeChatShopRefundWorker) Work(ctx context.Context, job *river.Job[orderapp.WeChatShopRefundArgs]) error {
	if worker == nil || worker.service == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 || job.Args.RefundID < 1 || job.ScheduledAt.IsZero() {
		return ErrInvalidSettlementWorker
	}
	digest := sha256.Sum256([]byte("order/wechat-shop/river-args/v1\x00" + strconv.FormatInt(job.Args.RefundID, 10)))
	_, err := worker.service.ExecuteRefund(ctx, orderport.WeChatShopExecutionJob{RefundID: job.Args.RefundID, RiverJobID: job.ID, RiverAttempt: int64(job.Attempt), ArgsDigest: digest, ScheduledAt: job.ScheduledAt.UTC()})
	return err
}

func (*PaymentEffectWorker) Timeout(*river.Job[orderapp.PaymentEffectBridgeArgs]) time.Duration {
	return 30 * time.Second
}
func (*RefundEffectWorker) Timeout(*river.Job[orderapp.RefundEffectBridgeArgs]) time.Duration {
	return 30 * time.Second
}
func (*PaymentReconcileWorker) Timeout(*river.Job[orderapp.PaymentReconcileArgs]) time.Duration {
	return 30 * time.Second
}
func (*RefundReconcileWorker) Timeout(*river.Job[orderapp.RefundReconcileArgs]) time.Duration {
	return 30 * time.Second
}
func (*WeChatShopRefundWorker) Timeout(*river.Job[orderapp.WeChatShopRefundArgs]) time.Duration {
	return 30 * time.Second
}

func effectJob(jobID, generation int64, kind string, recordID int64, scheduled time.Time) orderapp.EffectJob {
	digest := sha256.Sum256([]byte("pe01/river-args/v1\x00" + kind + "\x00" + strconv.FormatInt(recordID, 10)))
	return orderapp.EffectJob{RecordID: recordID, RiverJobID: jobID, RiverGeneration: generation, RiverQueue: string(platformjobqueue.QueueCritical), RiverArgsDigest: digest, ScheduledAt: scheduled.UTC()}
}
