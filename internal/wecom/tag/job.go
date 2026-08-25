package tag

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type RiverJobInserter struct {
	client *platformjobqueue.InsertOnlyClient
}

func NewRiverJobInserter(pool *pgxpool.Pool) (*RiverJobInserter, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(ErrInvalidConfiguration, err)
	}
	return &RiverJobInserter{client: client}, nil
}

func (inserter *RiverJobInserter) Insert(ctx context.Context, args JobArgs, generation int64, scheduledAt time.Time) (eer.RiverJobLink, error) {
	if inserter == nil || inserter.client == nil || args.EffectID == "" || generation < 1 || scheduledAt.IsZero() {
		return eer.RiverJobLink{}, ErrInvalidCommand
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return eer.RiverJobLink{}, err
	}
	jobID, err := inserter.client.InsertTx(ctx, tx, args, string(platformjobqueue.QueueSync))
	if err != nil || jobID < 1 {
		return eer.RiverJobLink{}, errors.Join(ErrEffectUnavailable, err)
	}
	return eer.RiverJobLink{
		JobID: jobID, Generation: generation, Queue: string(platformjobqueue.QueueSync),
		ArgsDigest: digest("river-args", args.EffectID), ScheduledAt: scheduledAt.UTC(),
	}, nil
}

var _ JobInserter = (*RiverJobInserter)(nil)
