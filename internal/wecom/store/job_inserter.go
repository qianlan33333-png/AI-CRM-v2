package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
)

type RiverJobInserter struct {
	client *platformjobqueue.InsertOnlyClient
}

var _ wecomapp.JobInserter = (*RiverJobInserter)(nil)

func NewRiverJobInserter(pool *pgxpool.Pool) (*RiverJobInserter, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, err
	}
	return &RiverJobInserter{client: client}, nil
}

func (inserter *RiverJobInserter) Insert(ctx context.Context, args wecomapp.InboundJobArgs) (int64, error) {
	if inserter == nil || inserter.client == nil || args.InboxID <= 0 {
		return 0, wecomapp.ErrInboundProcess
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	return inserter.client.InsertTx(ctx, tx, args, string(platformjobqueue.QueueCritical))
}
