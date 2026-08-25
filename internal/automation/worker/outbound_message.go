package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrOutboundMessageWorker = errors.New("invalid automation outbound message worker")

type OutboundMessageWorker struct {
	river.WorkerDefaults[automationstore.OutboundMessageArgs]
	handoff *automationstore.OutboundMessageHandoff
	adapter eer.Adapter
}

func NewOutboundMessageWorker(handoff *automationstore.OutboundMessageHandoff, adapter eer.Adapter) (*OutboundMessageWorker, error) {
	if handoff == nil || adapter == nil {
		return nil, ErrOutboundMessageWorker
	}
	return &OutboundMessageWorker{handoff: handoff, adapter: adapter}, nil
}

func RegisterOutboundMessageWorker(registry *platformjobqueue.WorkerRegistry, handoff *automationstore.OutboundMessageHandoff, adapter eer.Adapter) error {
	worker, err := NewOutboundMessageWorker(handoff, adapter)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueOutbound, worker)
}

func (worker *OutboundMessageWorker) Work(ctx context.Context, job *river.Job[automationstore.OutboundMessageArgs]) error {
	if worker == nil || worker.handoff == nil || worker.adapter == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 || job.Args.EffectID == "" {
		return ErrOutboundMessageWorker
	}
	sum := sha256.Sum256([]byte("automation.outbound_message.worker.v2\x00" + job.Args.EffectID))
	return worker.handoff.RunEffect(ctx, job.Args.EffectID, eer.Digest("sha256:"+hex.EncodeToString(sum[:])), worker.adapter)
}

func (*OutboundMessageWorker) Timeout(*river.Job[automationstore.OutboundMessageArgs]) time.Duration {
	return 30 * time.Second
}
