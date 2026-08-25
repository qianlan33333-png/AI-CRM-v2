package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrCampaignDispatchWorker = errors.New("invalid outbound campaign dispatch worker")

// ProviderShapedAdapter deliberately exposes only digest-only envelopes. The
// production implementation is disabled by default and must be explicitly
// composed with an authorised provider client; no startup configuration can
// accidentally turn a fake/local receipt into a network call.
type ProviderShapedAdapter struct {
	Enabled  bool
	Provider interface {
		Execute(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error)
	}
}

func (adapter ProviderShapedAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	if adapter.Enabled && adapter.Provider != nil {
		return adapter.Provider.Execute(ctx, envelope, attempt)
	}
	sum := sha256.Sum256([]byte("outbound.campaign_dispatch.gate_disabled.v1\x00" + string(envelope.Fingerprint()) + "\x00" + strconv.FormatInt(int64(attempt.Number), 10)))
	return eer.AdapterResult{Completion: eer.CompletionFinalFailed, ReceiptDigest: eer.Digest("sha256:" + hex.EncodeToString(sum[:]))}, nil
}

type CampaignDispatchWorker struct {
	river.WorkerDefaults[outboundstore.CampaignDispatchArgs]
	service *outboundapp.CampaignDispatchService
	adapter eer.Adapter
}

func NewCampaignDispatchWorker(service *outboundapp.CampaignDispatchService, adapter eer.Adapter) (*CampaignDispatchWorker, error) {
	if service == nil || adapter == nil {
		return nil, ErrCampaignDispatchWorker
	}
	return &CampaignDispatchWorker{service: service, adapter: adapter}, nil
}

func RegisterCampaignDispatchWorker(registry *platformjobqueue.WorkerRegistry, service *outboundapp.CampaignDispatchService, adapter eer.Adapter) error {
	worker, err := NewCampaignDispatchWorker(service, adapter)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueOutbound, worker)
}

func (worker *CampaignDispatchWorker) Work(ctx context.Context, job *river.Job[outboundstore.CampaignDispatchArgs]) error {
	if worker == nil || worker.service == nil || worker.adapter == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 || job.Args.EffectID == "" {
		return ErrCampaignDispatchWorker
	}
	sum := sha256.Sum256([]byte("outbound.campaign_dispatch.worker.v1\x00" + job.Args.EffectID))
	return worker.service.RunEffect(ctx, job.Args.EffectID, eer.Digest("sha256:"+hex.EncodeToString(sum[:])), worker.adapter)
}

func (*CampaignDispatchWorker) Timeout(*river.Job[outboundstore.CampaignDispatchArgs]) time.Duration {
	return 30 * time.Second
}
