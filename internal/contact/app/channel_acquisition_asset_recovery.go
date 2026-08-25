package app

import (
	"context"
	"errors"
	"reflect"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	ChannelAcquisitionAssetRecoveryJobKind = "contact_acquisition_asset_recovery"
	ChannelAcquisitionAssetRecoveryLimit   = int32(100)
	ChannelAcquisitionAssetRecoveryPeriod  = time.Minute
)

type ChannelAcquisitionAssetRecoveryJobArgs struct{}

func (ChannelAcquisitionAssetRecoveryJobArgs) Kind() string {
	return ChannelAcquisitionAssetRecoveryJobKind
}

type ChannelAcquisitionAssetRecoveryCandidate struct {
	EffectID   string
	Generation int64
}

type ChannelAcquisitionAssetRecoveryStore interface {
	ListExpiredAttempts(context.Context, time.Time, int32) ([]ChannelAcquisitionAssetRecoveryCandidate, error)
}

type ChannelAcquisitionAssetRecoveryService struct {
	uow   platformport.UnitOfWork
	store ChannelAcquisitionAssetRecoveryStore
	jobs  ChannelAcquisitionAssetJobInserter
	now   func() time.Time
}

func NewChannelAcquisitionAssetRecoveryService(uow platformport.UnitOfWork, store ChannelAcquisitionAssetRecoveryStore, jobs ChannelAcquisitionAssetJobInserter, now func() time.Time) (*ChannelAcquisitionAssetRecoveryService, error) {
	if channelAcquisitionAssetRecoveryNil(uow) || channelAcquisitionAssetRecoveryNil(store) || channelAcquisitionAssetRecoveryNil(jobs) || now == nil {
		return nil, ErrChannelAcquisitionAssetUnavailable
	}
	return &ChannelAcquisitionAssetRecoveryService{uow: uow, store: store, jobs: jobs, now: now}, nil
}

// EnqueueExpired inserts only effect_id jobs. Execute reloads the immutable
// Contact binding and converges an expired attempted lease without Provider I/O.
func (service *ChannelAcquisitionAssetRecoveryService) EnqueueExpired(ctx context.Context) (int, error) {
	if service == nil || ctx == nil || ctx.Err() != nil || channelAcquisitionAssetRecoveryNil(service.uow) ||
		channelAcquisitionAssetRecoveryNil(service.store) || channelAcquisitionAssetRecoveryNil(service.jobs) || service.now == nil {
		return 0, ErrChannelAcquisitionAssetUnavailable
	}
	now := service.now().UTC()
	enqueued := 0
	err := service.uow.Within(ctx, func(tx context.Context) error {
		candidates, err := service.store.ListExpiredAttempts(tx, now, ChannelAcquisitionAssetRecoveryLimit)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			if candidate.EffectID == "" || candidate.Generation < 1 {
				return ErrChannelAcquisitionAssetUnavailable
			}
			link, insertErr := service.jobs.Insert(tx, ChannelAcquisitionAssetJobArgs{EffectID: candidate.EffectID}, candidate.Generation, now)
			if insertErr != nil || link.JobID < 1 || link.Generation != candidate.Generation || link.Queue == "" || link.ArgsDigest == eer.Digest("") {
				return errors.Join(ErrChannelAcquisitionAssetUnavailable, insertErr)
			}
			enqueued++
		}
		return nil
	})
	if err != nil {
		return 0, classifyChannelAcquisitionAssetError(err)
	}
	return enqueued, nil
}

func channelAcquisitionAssetRecoveryNil(value any) bool {
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
