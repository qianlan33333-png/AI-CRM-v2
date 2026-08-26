package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

var ErrGroupOpsMaterialPreparation = errors.New("group ops material preparation unavailable")

// GroupOpsMaterialPreparationJobArgs contains only the opaque EER ID. Source
// bytes and provider tokens remain in Media's private store/provider boundary.
type GroupOpsMaterialPreparationJobArgs struct {
	EffectID string `json:"effect_id"`
}

func (GroupOpsMaterialPreparationJobArgs) Kind() string { return "media_wecom_upload" }

type GroupOpsMaterialPreparation struct {
	EffectID, SourceKind, SourceDigest, UploadKind string
	SourceID, Generation                           int64
	CreatedAt                                      time.Time
}

type GroupOpsMaterialPreparationStore interface {
	HasSufficientGroupOpsUploadLease(context.Context, string, int64, string, string, string, time.Time) (bool, error)
	NextGroupOpsUploadPreparationGeneration(context.Context, string, int64, string, string, string) (int64, error)
	BindGroupOpsUploadPreparation(context.Context, GroupOpsMaterialPreparation) (bool, error)
}

type GroupOpsMaterialPreparationEffects interface {
	Accept(context.Context, eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error)
	Queue(context.Context, eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error)
	Claim(context.Context, eer.ClaimCommand) (eer.Lease, eer.Projection, error)
	RunAttempt(context.Context, eer.Lease, eer.Adapter) (eer.Projection, eer.OperationReceipt, error)
}

type GroupOpsMaterialPreparationJobs interface {
	Insert(context.Context, GroupOpsMaterialPreparationJobArgs, time.Time) (eer.RiverJobLink, error)
}

type GroupOpsMaterialUploadInput struct {
	EffectID, SourceDigest, Filename, MIME, Checksum, Kind string
	Bytes                                                  []byte
}

type GroupOpsMaterialUploadResult struct {
	MediaID                                             string
	CreatedAt, ExpiresAt                                time.Time
	BusinessCallDispatched, OutcomeUnknown, FinalFailed bool
}

// GroupOpsMaterialUploadAttemptStore owns the short persistence transactions
// on either side of provider I/O. Ready recording must append the receipt
// before setting preparation ready; outcome_unknown is terminal/no-retry.
type GroupOpsMaterialUploadAttemptStore interface {
	LoadGroupOpsMaterialUpload(context.Context, string) (GroupOpsMaterialUploadInput, error)
	RecordGroupOpsMaterialUploadReady(context.Context, string, GroupOpsMaterialUploadResult, eer.Digest) error
	MarkGroupOpsMaterialUploadOutcomeUnknown(context.Context, string, time.Time) error
	MarkGroupOpsMaterialUploadFinalFailed(context.Context, string, time.Time) error
}

type GroupOpsMaterialUploadProvider interface {
	Upload(context.Context, GroupOpsMaterialUploadInput) (GroupOpsMaterialUploadResult, error)
}

// GroupOpsMaterialPreparationService deliberately has no UnitOfWork wrapper:
// Group Ops calls Ensure from its already-open AcceptPlan transaction, so the
// source snapshot, EER acceptance, preparation binding, River insert, and EER
// queue transition commit or roll back together.
type GroupOpsMaterialPreparationService struct {
	store    GroupOpsMaterialPreparationStore
	effects  GroupOpsMaterialPreparationEffects
	jobs     GroupOpsMaterialPreparationJobs
	scope    eer.Digest
	now      func() time.Time
	attempts GroupOpsMaterialUploadAttemptStore
	uploader GroupOpsMaterialUploadProvider
}

func (service *GroupOpsMaterialPreparationService) SetUploadAttemptDependencies(store GroupOpsMaterialUploadAttemptStore, uploader GroupOpsMaterialUploadProvider) error {
	if service == nil || store == nil || uploader == nil {
		return ErrGroupOpsMaterialPreparation
	}
	service.attempts, service.uploader = store, uploader
	return nil
}

func NewGroupOpsMaterialPreparationService(store GroupOpsMaterialPreparationStore, effects GroupOpsMaterialPreparationEffects, jobs GroupOpsMaterialPreparationJobs, providerScopeDigest eer.Digest) (*GroupOpsMaterialPreparationService, error) {
	if store == nil || effects == nil || jobs == nil || !validPreparationDigest(providerScopeDigest) {
		return nil, ErrGroupOpsMaterialPreparation
	}
	return &GroupOpsMaterialPreparationService{store: store, effects: effects, jobs: jobs, scope: providerScopeDigest, now: time.Now}, nil
}

func (service *GroupOpsMaterialPreparationService) Ensure(ctx context.Context, sources mediaport.GroupOpsMaterialSourceSnapshot, scheduledFor, requiredThrough time.Time) ([]GroupOpsMaterialPreparation, error) {
	if service == nil || ctx == nil || service.now == nil || scheduledFor.IsZero() || requiredThrough.IsZero() || !requiredThrough.After(scheduledFor) || mediaport.ValidateGroupOpsMaterialSourceSnapshot(sources) != nil {
		return nil, ErrGroupOpsMaterialPreparation
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return nil, ErrGroupOpsMaterialPreparation
	}
	inputs := uploadInputs(sources)
	if len(inputs) == 0 {
		return []GroupOpsMaterialPreparation{}, nil
	}
	result := make([]GroupOpsMaterialPreparation, 0, len(inputs))
	for _, input := range inputs {
		ready, err := service.store.HasSufficientGroupOpsUploadLease(ctx, input.SourceKind, input.SourceID, input.SourceDigest, string(service.scope), input.UploadKind, requiredThrough)
		if err != nil {
			return nil, errors.Join(ErrGroupOpsMaterialPreparation, err)
		}
		if ready {
			continue
		}
		generation, err := service.store.NextGroupOpsUploadPreparationGeneration(ctx, input.SourceKind, input.SourceID, input.SourceDigest, string(service.scope), input.UploadKind)
		if err != nil || generation < 1 {
			return nil, errors.Join(ErrGroupOpsMaterialPreparation, err)
		}
		generationText := strconv.FormatInt(generation, 10)
		envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerMedia, Kind: eer.KindMediaWeComUpload, SourceRefDigest: eer.Digest(input.SourceDigest), TargetRefDigest: service.scope, PayloadDigest: preparationDigest("payload", input.SourceKind, strconv.FormatInt(input.SourceID, 10), input.SourceDigest, input.UploadKind, string(service.scope), generationText), PolicyVersionHash: preparationDigest("policy", "media-wecom-upload-v1", generationText)})
		if err != nil {
			return nil, ErrGroupOpsMaterialPreparation
		}
		accepted, _, err := service.effects.Accept(ctx, eer.AcceptCommand{Envelope: envelope, ReceiptKeyDigest: preparationDigest("accept", string(envelope.Fingerprint()), generationText)})
		if err != nil || accepted.Owner != eer.OwnerMedia || accepted.Kind != eer.KindMediaWeComUpload || accepted.ID == "" {
			return nil, errors.Join(ErrGroupOpsMaterialPreparation, err)
		}
		prepared := GroupOpsMaterialPreparation{EffectID: accepted.ID, SourceKind: input.SourceKind, SourceID: input.SourceID, SourceDigest: input.SourceDigest, UploadKind: input.UploadKind, Generation: generation, CreatedAt: now}
		if _, err = service.store.BindGroupOpsUploadPreparation(ctx, prepared); err != nil {
			return nil, errors.Join(ErrGroupOpsMaterialPreparation, err)
		}
		scheduledAt := scheduledFor.Add(-12 * time.Hour)
		if scheduledAt.Before(now) {
			scheduledAt = now
		}
		job, err := service.jobs.Insert(ctx, GroupOpsMaterialPreparationJobArgs{EffectID: accepted.ID}, scheduledAt)
		if err != nil {
			return nil, errors.Join(ErrGroupOpsMaterialPreparation, err)
		}
		queued, _, err := service.effects.Queue(ctx, eer.QueueCommand{EffectID: accepted.ID, Job: job, ReceiptKeyDigest: preparationDigest("queue", accepted.ID, string(job.ArgsDigest), job.ScheduledAt.UTC().Format(time.RFC3339Nano))})
		if err != nil || queued.ID != accepted.ID || queued.State != eer.StateQueued {
			return nil, errors.Join(ErrGroupOpsMaterialPreparation, err)
		}
		result = append(result, prepared)
	}
	return result, nil
}

// RunUploadEffect persists the EER attempted fence before calling the upload
// provider. Its adapter is per-effect because EER deliberately exposes only
// digests, never private source bytes.
func (service *GroupOpsMaterialPreparationService) RunUploadEffect(ctx context.Context, effectID string, workerDigest eer.Digest) error {
	if service == nil || ctx == nil || service.attempts == nil || service.uploader == nil || service.now == nil || effectID == "" || !validPreparationDigest(workerDigest) {
		return ErrGroupOpsMaterialPreparation
	}
	lease, projection, err := service.effects.Claim(ctx, eer.ClaimCommand{EffectID: effectID, WorkerDigest: workerDigest})
	if err != nil || projection.ID != effectID || projection.Owner != eer.OwnerMedia || projection.Kind != eer.KindMediaWeComUpload {
		return errors.Join(ErrGroupOpsMaterialPreparation, err)
	}
	_, _, err = service.effects.RunAttempt(ctx, lease, &groupOpsMaterialUploadAdapter{effectID: effectID, store: service.attempts, provider: service.uploader, now: service.now})
	return err
}

type groupOpsMaterialUploadAdapter struct {
	effectID string
	store    GroupOpsMaterialUploadAttemptStore
	provider GroupOpsMaterialUploadProvider
	now      func() time.Time
}

var _ eer.Adapter = (*groupOpsMaterialUploadAdapter)(nil)

func (adapter *groupOpsMaterialUploadAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	if adapter == nil || adapter.store == nil || adapter.provider == nil || adapter.now == nil || ctx == nil || adapter.effectID == "" || attempt.Number < 1 {
		return eer.AdapterResult{}, ErrGroupOpsMaterialPreparation
	}
	input, err := adapter.store.LoadGroupOpsMaterialUpload(ctx, adapter.effectID)
	if err != nil || input.EffectID != adapter.effectID || input.SourceDigest != string(envelope.SourceRefDigest()) || !validPreparationDigest(eer.Digest(input.SourceDigest)) {
		return eer.AdapterResult{}, errors.Join(ErrGroupOpsMaterialPreparation, err)
	}
	result, uploadErr := adapter.provider.Upload(ctx, input)
	receipt := preparationDigest("attempt", adapter.effectID, strconv.FormatInt(int64(attempt.Number), 10), input.SourceDigest, result.MediaID, result.CreatedAt.UTC().Format(time.RFC3339Nano), result.ExpiresAt.UTC().Format(time.RFC3339Nano))
	now := adapter.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return eer.AdapterResult{}, ErrGroupOpsMaterialPreparation
	}
	if uploadErr != nil && !result.BusinessCallDispatched {
		if err := adapter.store.MarkGroupOpsMaterialUploadFinalFailed(ctx, adapter.effectID, now); err != nil {
			return eer.AdapterResult{}, err
		}
		return eer.AdapterResult{Completion: eer.CompletionFinalFailed, ReceiptDigest: receipt}, nil
	}
	if uploadErr != nil || result.OutcomeUnknown {
		if result.BusinessCallDispatched {
			if err := adapter.store.MarkGroupOpsMaterialUploadOutcomeUnknown(ctx, adapter.effectID, now); err != nil {
				return eer.AdapterResult{}, err
			}
		}
		return eer.AdapterResult{Completion: eer.CompletionOutcomeUnknown, ReceiptDigest: receipt, BusinessCallDispatched: result.BusinessCallDispatched, RealExternalCallExecuted: result.BusinessCallDispatched}, uploadErr
	}
	if result.FinalFailed || result.MediaID == "" || result.CreatedAt.IsZero() || !result.ExpiresAt.After(result.CreatedAt) {
		if err := adapter.store.MarkGroupOpsMaterialUploadFinalFailed(ctx, adapter.effectID, now); err != nil {
			return eer.AdapterResult{}, err
		}
		return eer.AdapterResult{Completion: eer.CompletionFinalFailed, ReceiptDigest: receipt, BusinessCallDispatched: result.BusinessCallDispatched, RealExternalCallExecuted: result.BusinessCallDispatched}, nil
	}
	if err := adapter.store.RecordGroupOpsMaterialUploadReady(ctx, adapter.effectID, result, receipt); err != nil {
		return eer.AdapterResult{}, err
	}
	return eer.AdapterResult{Completion: eer.CompletionExecuted, ReceiptDigest: receipt, BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
}

type preparationInput struct {
	SourceKind, SourceDigest, UploadKind string
	SourceID                             int64
}

func uploadInputs(sources mediaport.GroupOpsMaterialSourceSnapshot) []preparationInput {
	seen := map[string]struct{}{}
	inputs := make([]preparationInput, 0, len(sources.References))
	appendInput := func(kind string, id int64, digest, uploadKind string) {
		key := kind + "\x00" + strconv.FormatInt(id, 10) + "\x00" + digest
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		inputs = append(inputs, preparationInput{SourceKind: kind, SourceID: id, SourceDigest: digest, UploadKind: uploadKind})
	}
	for _, source := range sources.References {
		switch source.Reference.Kind {
		case "image":
			appendInput("image", source.Reference.ID, source.SourceDigest, "image")
		case "attachment":
			appendInput("attachment", source.Reference.ID, source.SourceDigest, "file")
		case "miniprogram":
			appendInput("image", source.ThumbnailImageID, source.ThumbnailSourceDigest, "image")
		}
	}
	sort.Slice(inputs, func(left, right int) bool {
		if inputs[left].SourceKind != inputs[right].SourceKind {
			return inputs[left].SourceKind < inputs[right].SourceKind
		}
		if inputs[left].SourceID != inputs[right].SourceID {
			return inputs[left].SourceID < inputs[right].SourceID
		}
		return inputs[left].SourceDigest < inputs[right].SourceDigest
	})
	return inputs
}

func preparationDigest(parts ...string) eer.Digest {
	sum := sha256.Sum256([]byte("media.group_ops_preparation.v1\x00" + joinPreparationParts(parts)))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func joinPreparationParts(parts []string) string {
	value := ""
	for index, part := range parts {
		if index != 0 {
			value += "\x00"
		}
		value += part
	}
	return value
}

func validPreparationDigest(value eer.Digest) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || string(value[:7]) != "sha256:" {
		return false
	}
	decoded, err := hex.DecodeString(string(value[7:]))
	return err == nil && len(decoded) == sha256.Size && string(value) == "sha256:"+hex.EncodeToString(decoded)
}
