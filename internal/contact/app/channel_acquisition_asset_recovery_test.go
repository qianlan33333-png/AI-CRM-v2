package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type acquisitionRecoveryUOW struct{ calls int }

func (uow *acquisitionRecoveryUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

type acquisitionRecoveryStore struct {
	candidates []ChannelAcquisitionAssetRecoveryCandidate
	at         time.Time
	limit      int32
	err        error
}

func (store *acquisitionRecoveryStore) ListExpiredAttempts(_ context.Context, at time.Time, limit int32) ([]ChannelAcquisitionAssetRecoveryCandidate, error) {
	store.at, store.limit = at, limit
	return append([]ChannelAcquisitionAssetRecoveryCandidate(nil), store.candidates...), store.err
}

type acquisitionRecoveryJobs struct {
	effectIDs  []string
	generation []int64
	err        error
}

func (jobs *acquisitionRecoveryJobs) Insert(_ context.Context, args ChannelAcquisitionAssetJobArgs, generation int64, scheduledAt time.Time) (eer.RiverJobLink, error) {
	jobs.effectIDs = append(jobs.effectIDs, args.EffectID)
	jobs.generation = append(jobs.generation, generation)
	if jobs.err != nil {
		return eer.RiverJobLink{}, jobs.err
	}
	digest := sha256.Sum256([]byte(args.EffectID))
	return eer.RiverJobLink{
		JobID: int64(len(jobs.effectIDs)), Generation: generation, Queue: "critical",
		ArgsDigest: eer.Digest("sha256:" + hex.EncodeToString(digest[:])), ScheduledAt: scheduledAt,
	}, nil
}

func TestCH02RecoveryEnqueuesExpiredEffectIDsInOneUOW(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.FixedZone("fixture", 8*60*60))
	uow := &acquisitionRecoveryUOW{}
	store := &acquisitionRecoveryStore{candidates: []ChannelAcquisitionAssetRecoveryCandidate{{EffectID: "eer_41", Generation: 2}, {EffectID: "eer_42", Generation: 3}}}
	jobs := &acquisitionRecoveryJobs{}
	service, err := NewChannelAcquisitionAssetRecoveryService(uow, store, jobs, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	count, err := service.EnqueueExpired(context.Background())
	if err != nil || count != 2 || uow.calls != 1 || store.limit != ChannelAcquisitionAssetRecoveryLimit || !store.at.Equal(now.UTC()) ||
		len(jobs.effectIDs) != 2 || jobs.effectIDs[0] != "eer_41" || jobs.generation[1] != 3 {
		t.Fatalf("count=%d uow=%d at=%v limit=%d jobs=%v generations=%v err=%v", count, uow.calls, store.at, store.limit, jobs.effectIDs, jobs.generation, err)
	}
}

func TestCH02RecoveryFailsClosedWithoutPartialSuccess(t *testing.T) {
	sentinel := errors.New("river unavailable")
	uow := &acquisitionRecoveryUOW{}
	store := &acquisitionRecoveryStore{candidates: []ChannelAcquisitionAssetRecoveryCandidate{{EffectID: "eer_41", Generation: 2}}}
	jobs := &acquisitionRecoveryJobs{err: sentinel}
	service, err := NewChannelAcquisitionAssetRecoveryService(uow, store, jobs, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := service.EnqueueExpired(context.Background()); count != 0 || !errors.Is(err, ErrChannelAcquisitionAssetUnavailable) || !errors.Is(err, sentinel) {
		t.Fatalf("count=%d err=%v", count, err)
	}
	var typedNil *acquisitionRecoveryStore
	if service, err = NewChannelAcquisitionAssetRecoveryService(uow, typedNil, jobs, time.Now); service != nil || !errors.Is(err, ErrChannelAcquisitionAssetUnavailable) {
		t.Fatalf("typed-nil service=%v err=%v", service, err)
	}
}
