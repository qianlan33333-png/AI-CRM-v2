package jobqueue

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	queueriver "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// InsertOnlyClient permits a command boundary to persist a known River job in
// its caller's PostgreSQL transaction without registering or running workers.
type InsertOnlyClient struct {
	client *queueriver.Client[pgx.Tx]
}

func NewInsertOnlyClient(pool *pgxpool.Pool) (*InsertOnlyClient, error) {
	if pool == nil {
		return nil, ErrClientUnavailable
	}
	client, err := queueriver.NewClient(riverpgxv5.New(pool), &queueriver.Config{SkipUnknownJobCheck: true})
	if err != nil {
		return nil, err
	}
	return &InsertOnlyClient{client: client}, nil
}

func (client *InsertOnlyClient) InsertTx(ctx context.Context, tx pgx.Tx, args queueriver.JobArgs, queue string) (int64, error) {
	return client.InsertTxScheduled(ctx, tx, args, queue, time.Time{})
}

// InsertTxScheduled is the same transaction-bound insert with an explicit
// future schedule. A zero time preserves River's immediate scheduling.
func (client *InsertOnlyClient) InsertTxScheduled(ctx context.Context, tx pgx.Tx, args queueriver.JobArgs, queue string, scheduledAt time.Time) (int64, error) {
	if client == nil || client.client == nil || tx == nil || args == nil || queue == "" {
		return 0, ErrClientUnavailable
	}
	options := &queueriver.InsertOpts{Queue: queue}
	if !scheduledAt.IsZero() {
		options.ScheduledAt = scheduledAt.UTC()
	}
	result, err := client.client.InsertTx(ctx, tx, args, options)
	if err != nil || result == nil || result.Job == nil || result.Job.ID <= 0 || result.Job.Queue != queue || result.Job.Kind != args.Kind() {
		return 0, errors.Join(ErrClientUnavailable, err)
	}
	return result.Job.ID, nil
}
