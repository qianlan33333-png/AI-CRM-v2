package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type GroupOpsMaterialPreparationJobInserter struct {
	client *platformjobqueue.InsertOnlyClient
}

func NewGroupOpsMaterialPreparationJobInserter(pool *pgxpool.Pool) (*GroupOpsMaterialPreparationJobInserter, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(mediaapp.ErrGroupOpsMaterialPreparation, err)
	}
	return &GroupOpsMaterialPreparationJobInserter{client: client}, nil
}

func (inserter *GroupOpsMaterialPreparationJobInserter) Insert(ctx context.Context, args mediaapp.GroupOpsMaterialPreparationJobArgs, scheduledAt time.Time) (eer.RiverJobLink, error) {
	if inserter == nil || inserter.client == nil || ctx == nil || args.EffectID == "" || scheduledAt.IsZero() {
		return eer.RiverJobLink{}, mediaapp.ErrGroupOpsMaterialPreparation
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return eer.RiverJobLink{}, errors.Join(mediaapp.ErrGroupOpsMaterialPreparation, err)
	}
	jobID, err := inserter.client.InsertTxScheduled(ctx, tx, args, string(platformjobqueue.QueueOutbound), scheduledAt)
	if err != nil || jobID < 1 {
		return eer.RiverJobLink{}, errors.Join(mediaapp.ErrGroupOpsMaterialPreparation, err)
	}
	sum := sha256.Sum256([]byte("media.group-ops-preparation.job.v1\x00" + args.EffectID))
	return eer.RiverJobLink{
		JobID: jobID, Generation: 1, Queue: string(platformjobqueue.QueueOutbound),
		ArgsDigest: eer.Digest("sha256:" + hex.EncodeToString(sum[:])), ScheduledAt: scheduledAt.UTC(),
	}, nil
}
