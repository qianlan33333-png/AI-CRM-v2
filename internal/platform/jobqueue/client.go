package jobqueue

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	queueriver "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

var ErrClientUnavailable = errors.New("River client is unavailable")

type Client struct {
	client   *queueriver.Client[pgx.Tx]
	registry *WorkerRegistry
}

func NewClient(pool *pgxpool.Pool, concurrency QueueConcurrency, registry *WorkerRegistry) (*Client, error) {
	if pool == nil || registry == nil || registry.workers == nil || registry.assignments == nil {
		return nil, ErrClientUnavailable
	}
	if err := concurrency.validate(); err != nil {
		return nil, err
	}
	client, err := queueriver.NewClient(riverpgxv5.New(pool), &queueriver.Config{
		Queues:  concurrency.riverQueues(),
		Workers: registry.workers,
	})
	if err != nil {
		return nil, err
	}
	return &Client{client: client, registry: registry}, nil
}

func (client *Client) Start(ctx context.Context) error {
	if client == nil || client.client == nil {
		return ErrClientUnavailable
	}
	return client.client.Start(ctx)
}

func (client *Client) Stop(ctx context.Context) error {
	if client == nil || client.client == nil {
		return ErrClientUnavailable
	}
	return client.client.Stop(ctx)
}

func (client *Client) Stopped() <-chan struct{} {
	if client == nil || client.client == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return client.client.Stopped()
}

func (client *Client) Enqueue(ctx context.Context, queue Queue, args queueriver.JobArgs, options *queueriver.InsertOpts) (*rivertype.JobInsertResult, error) {
	if client == nil || client.client == nil {
		return nil, ErrClientUnavailable
	}
	insertOptions, err := client.explicitOptions(queue, args, options)
	if err != nil {
		return nil, err
	}
	return client.client.Insert(ctx, args, insertOptions)
}

func (client *Client) EnqueueTx(ctx context.Context, tx pgx.Tx, queue Queue, args queueriver.JobArgs, options *queueriver.InsertOpts) (*rivertype.JobInsertResult, error) {
	if client == nil || client.client == nil || tx == nil {
		return nil, ErrClientUnavailable
	}
	insertOptions, err := client.explicitOptions(queue, args, options)
	if err != nil {
		return nil, err
	}
	return client.client.InsertTx(ctx, tx, args, insertOptions)
}

func (client *Client) explicitOptions(queue Queue, args queueriver.JobArgs, options *queueriver.InsertOpts) (*queueriver.InsertOpts, error) {
	if !validQueue(queue) || args == nil {
		return nil, ErrInvalidQueue
	}
	registeredQueue, ok := client.registry.assignments[args.Kind()]
	if !ok {
		return nil, ErrUnregisteredJob
	}
	if registeredQueue != queue {
		return nil, ErrQueueMismatch
	}
	cloned := queueriver.InsertOpts{}
	if options != nil {
		cloned = *options
	}
	if cloned.Queue != "" && cloned.Queue != string(queue) {
		return nil, ErrQueueMismatch
	}
	cloned.Queue = string(queue)
	return &cloned, nil
}
