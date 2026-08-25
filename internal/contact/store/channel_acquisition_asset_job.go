package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type ChannelAcquisitionAssetRiverJobInserter struct {
	client *platformjobqueue.InsertOnlyClient
}

var _ contactapp.ChannelAcquisitionAssetJobInserter = (*ChannelAcquisitionAssetRiverJobInserter)(nil)

func NewChannelAcquisitionAssetRiverJobInserter(pool *pgxpool.Pool) (*ChannelAcquisitionAssetRiverJobInserter, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(contactapp.ErrChannelAcquisitionAssetUnavailable, err)
	}
	return &ChannelAcquisitionAssetRiverJobInserter{client: client}, nil
}

func (inserter *ChannelAcquisitionAssetRiverJobInserter) Insert(ctx context.Context, args contactapp.ChannelAcquisitionAssetJobArgs, generation int64, scheduledAt time.Time) (eer.RiverJobLink, error) {
	if inserter == nil || inserter.client == nil || ctx == nil || args.EffectID == "" || generation < 1 || scheduledAt.IsZero() {
		return eer.RiverJobLink{}, contactapp.ErrChannelAcquisitionAssetUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return eer.RiverJobLink{}, errors.Join(contactapp.ErrChannelAcquisitionAssetUnavailable, err)
	}
	jobID, err := inserter.client.InsertTx(ctx, tx, args, string(platformjobqueue.QueueCritical))
	if err != nil || jobID < 1 {
		return eer.RiverJobLink{}, errors.Join(contactapp.ErrChannelAcquisitionAssetUnavailable, err)
	}
	return eer.RiverJobLink{
		JobID: jobID, Generation: generation, Queue: string(platformjobqueue.QueueCritical),
		ArgsDigest: channelAcquisitionAssetJobDigest(args), ScheduledAt: scheduledAt.UTC(),
	}, nil
}

func channelAcquisitionAssetJobDigest(args contactapp.ChannelAcquisitionAssetJobArgs) eer.Digest {
	if args.EffectID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("contact.acquisition.asset.job.v1\x00" + args.EffectID))
	return eer.Digest("sha256:" + hex.EncodeToString(digest[:]))
}
