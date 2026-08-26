package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	groupopsmaterial "github.com/qianlan33333-png/AI-CRM-v2/internal/media/groupopsmaterial"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	outboundprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/provider"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type groupOpsMaterialBoundary struct {
	uow          platformport.UnitOfWork
	repository   *mediastore.GroupOpsMaterialRepository
	preparations *mediaapp.GroupOpsMaterialPreparationService
	freezer      *groupopsmaterial.Freezer
	now          func() time.Time
}

func newGroupOpsMaterialRuntime(pool *pgxpool.Pool, uow platformport.UnitOfWork, effects mediaapp.GroupOpsMaterialPreparationEffects, corpID string) (*groupOpsMaterialBoundary, *mediaapp.GroupOpsMaterialPreparationService, error) {
	if pool == nil || uow == nil || effects == nil || corpID == "" {
		return nil, nil, groupopsapp.ErrUnavailable
	}
	scope := groupOpsMaterialDigest("wecom-provider-scope", corpID)
	repository, err := mediastore.NewGroupOpsMaterialRepository(string(scope))
	if err != nil {
		return nil, nil, err
	}
	jobs, err := mediastore.NewGroupOpsMaterialPreparationJobInserter(pool)
	if err != nil {
		return nil, nil, err
	}
	preparations, err := mediaapp.NewGroupOpsMaterialPreparationService(repository, effects, jobs, scope)
	if err != nil {
		return nil, nil, err
	}
	freezer, err := groupopsmaterial.NewFreezer(repository)
	if err != nil {
		return nil, nil, err
	}
	return &groupOpsMaterialBoundary{uow: uow, repository: repository, preparations: preparations, freezer: freezer, now: time.Now}, preparations, nil
}

func bindGroupOpsMaterialUploader(service *mediaapp.GroupOpsMaterialPreparationService, uow platformport.UnitOfWork, repository *mediastore.GroupOpsMaterialRepository, config appconfig.WeComOutbound) error {
	if service == nil || uow == nil || repository == nil || !config.Enabled || !config.PermissionConfirmed {
		return groupopsapp.ErrUnavailable
	}
	providerHTTP := &http.Client{Timeout: 30 * time.Second}
	tokens, err := newGroupOpsTokens(config, providerHTTP, time.Now)
	if err != nil {
		return err
	}
	uploader, err := outboundprovider.NewTemporaryMediaUploader("https://qyapi.weixin.qq.com", providerHTTP, groupOpsTokenAdapter{provider: tokens}, time.Now)
	if err != nil {
		return err
	}
	return service.SetUploadAttemptDependencies(&groupOpsMaterialAttemptStore{uow: uow, store: repository}, groupOpsMaterialUploadProvider{uploader: uploader})
}

func (boundary *groupOpsMaterialBoundary) CaptureAndPrepare(ctx context.Context, plan groupopsport.MaterialPlan, scheduledFor time.Time) (groupopsport.MaterialSourceSnapshot, error) {
	if boundary == nil || boundary.repository == nil || boundary.preparations == nil || boundary.now == nil || ctx == nil || scheduledFor.IsZero() {
		return groupopsport.MaterialSourceSnapshot{}, groupopsapp.ErrUnavailable
	}
	mediaPlan := mediaport.GroupOpsMaterialPlan{References: make([]mediaport.GroupOpsMaterialReference, len(plan.References))}
	for index, reference := range plan.References {
		mediaPlan.References[index] = mediaport.GroupOpsMaterialReference{Kind: reference.Kind, ID: reference.ID}
	}
	sources, err := boundary.repository.CaptureGroupOpsMaterialSources(ctx, mediaPlan)
	if err != nil {
		return groupopsport.MaterialSourceSnapshot{}, err
	}
	requiredThrough := scheduledFor.Add(time.Hour)
	if floor := boundary.now().UTC().Add(time.Hour); requiredThrough.Before(floor) {
		requiredThrough = floor
	}
	if _, err = boundary.preparations.Ensure(ctx, sources, scheduledFor, requiredThrough); err != nil {
		return groupopsport.MaterialSourceSnapshot{}, err
	}
	raw, err := json.Marshal(sources)
	if err != nil {
		return groupopsport.MaterialSourceSnapshot{}, err
	}
	result := groupopsport.MaterialSourceSnapshot{References: make([]groupopsport.CapturedMaterialReference, len(sources.References)), Snapshot: raw}
	for index, source := range sources.References {
		result.References[index] = groupopsport.CapturedMaterialReference{Kind: source.Reference.Kind, ID: source.Reference.ID, SourceDigest: source.SourceDigest}
	}
	return result, nil
}

func (boundary *groupOpsMaterialBoundary) FreezePrepared(ctx context.Context, sourceRaw json.RawMessage, requiredThrough time.Time) (groupopsport.PreparedMaterial, error) {
	if boundary == nil || boundary.repository == nil || boundary.freezer == nil || ctx == nil || !json.Valid(sourceRaw) || requiredThrough.IsZero() {
		return groupopsport.PreparedMaterial{}, groupopsapp.ErrUnavailable
	}
	var sources mediaport.GroupOpsMaterialSourceSnapshot
	if err := json.Unmarshal(sourceRaw, &sources); err != nil || mediaport.ValidateGroupOpsMaterialSourceSnapshot(sources) != nil {
		return groupopsport.PreparedMaterial{}, groupopsapp.ErrUnavailable
	}
	prepared, err := boundary.repository.ReadPreparedGroupOpsPlan(ctx, sources, requiredThrough)
	if err != nil {
		state, stateErr := boundary.repository.ReadGroupOpsMaterialPreparationState(ctx, sources)
		if stateErr != nil {
			return groupopsport.PreparedMaterial{}, errors.Join(groupopsapp.ErrUnavailable, err, stateErr)
		}
		switch state {
		case "outcome_unknown":
			return groupopsport.PreparedMaterial{}, groupopsapp.ErrMaterialPreparationOutcomeUnknown
		case "preparing", "ready":
			return groupopsport.PreparedMaterial{}, groupopsapp.ErrMaterialPreparationPending
		default:
			return groupopsport.PreparedMaterial{}, errors.Join(groupopsapp.ErrUnavailable, err)
		}
	}
	snapshot, err := boundary.freezer.FreezeGroupOpsMaterial(ctx, sources, requiredThrough)
	if err != nil {
		return groupopsport.PreparedMaterial{}, err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return groupopsport.PreparedMaterial{}, err
	}
	readyUntil := time.Time{}
	for _, item := range prepared.Items {
		if item.ReadyUntil.IsZero() {
			continue
		}
		if readyUntil.IsZero() || item.ReadyUntil.Before(readyUntil) {
			readyUntil = item.ReadyUntil
		}
	}
	if readyUntil.IsZero() {
		readyUntil = requiredThrough.Add(time.Hour)
	}
	return groupopsport.PreparedMaterial{Snapshot: raw, Digest: string(groupOpsMaterialDigest("group-ops-material", string(raw))), ReadyUntil: readyUntil}, nil
}

func (boundary *groupOpsMaterialBoundary) VerifyMaterialReady(ctx context.Context, sourceRaw, materialRaw json.RawMessage, digest string, now time.Time) error {
	if boundary == nil || boundary.uow == nil || ctx == nil || !json.Valid(materialRaw) || now.IsZero() {
		return groupopsapp.ErrUnavailable
	}
	return boundary.uow.Within(ctx, func(tx context.Context) error {
		prepared, err := boundary.FreezePrepared(tx, sourceRaw, now.UTC().Add(time.Hour))
		if err != nil || !bytes.Equal(prepared.Snapshot, materialRaw) || prepared.Digest != digest {
			return errors.Join(groupopsapp.ErrUnavailable, err)
		}
		return nil
	})
}

type groupOpsMaterialAttemptStore struct {
	uow   platformport.UnitOfWork
	store mediaapp.GroupOpsMaterialUploadAttemptStore
}

func (store *groupOpsMaterialAttemptStore) LoadGroupOpsMaterialUpload(ctx context.Context, effectID string) (result mediaapp.GroupOpsMaterialUploadInput, err error) {
	err = store.uow.Within(ctx, func(tx context.Context) error {
		result, err = store.store.LoadGroupOpsMaterialUpload(tx, effectID)
		return err
	})
	return result, err
}

func (store *groupOpsMaterialAttemptStore) RecordGroupOpsMaterialUploadReady(ctx context.Context, effectID string, result mediaapp.GroupOpsMaterialUploadResult, receipt eer.Digest) error {
	return store.uow.Within(ctx, func(tx context.Context) error {
		return store.store.RecordGroupOpsMaterialUploadReady(tx, effectID, result, receipt)
	})
}

func (store *groupOpsMaterialAttemptStore) MarkGroupOpsMaterialUploadOutcomeUnknown(ctx context.Context, effectID string, now time.Time) error {
	return store.uow.Within(ctx, func(tx context.Context) error {
		return store.store.MarkGroupOpsMaterialUploadOutcomeUnknown(tx, effectID, now)
	})
}

func (store *groupOpsMaterialAttemptStore) MarkGroupOpsMaterialUploadFinalFailed(ctx context.Context, effectID string, now time.Time) error {
	return store.uow.Within(ctx, func(tx context.Context) error {
		return store.store.MarkGroupOpsMaterialUploadFinalFailed(tx, effectID, now)
	})
}

type groupOpsMaterialUploadProvider struct {
	uploader *outboundprovider.TemporaryMediaUploader
}

func (provider groupOpsMaterialUploadProvider) Upload(ctx context.Context, input mediaapp.GroupOpsMaterialUploadInput) (mediaapp.GroupOpsMaterialUploadResult, error) {
	if provider.uploader == nil {
		return mediaapp.GroupOpsMaterialUploadResult{}, groupopsapp.ErrUnavailable
	}
	result, err := provider.uploader.Upload(ctx, outboundprovider.TemporaryMediaUpload{Kind: input.Kind, Filename: input.Filename, MIME: input.MIME, Bytes: input.Bytes, Checksum: input.Checksum})
	return mediaapp.GroupOpsMaterialUploadResult{
		MediaID: result.MediaID, CreatedAt: result.CreatedAt, ExpiresAt: result.ExpiresAt,
		BusinessCallDispatched: result.BusinessCallDispatched, OutcomeUnknown: result.OutcomeUnknown, FinalFailed: result.FinalFailed,
	}, err
}

func groupOpsMaterialDigest(label string, values ...string) eer.Digest {
	joined := label
	for _, value := range values {
		joined += "\x00" + value
	}
	sum := sha256.Sum256([]byte(joined))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
