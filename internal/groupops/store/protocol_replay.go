package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	groupopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store/generated"
)

const groupOpsProtocolClientID = "aicrm-webhook-group-ops"

type ProtocolReplayStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewProtocolReplayStore(pool *pgxpool.Pool) (*ProtocolReplayStore, error) {
	if pool == nil {
		return nil, errors.New("invalid Group Ops protocol replay store")
	}
	return &ProtocolReplayStore{pool: pool, now: time.Now}, nil
}

func (store *ProtocolReplayStore) Reserve(ctx context.Context, resource, event string, payload [32]byte) (bool, error) {
	if store == nil || store.pool == nil || store.now == nil || ctx == nil || resource == "" || event == "" {
		return false, errors.New("Group Ops protocol replay store unavailable")
	}
	eventDigest := sha256.Sum256([]byte(event))
	stored, err := groupopsdb.New(store.pool).ReserveGroupOpsProtocolReplay(ctx, groupopsdb.ReserveGroupOpsProtocolReplayParams{
		ClientID: groupOpsProtocolClientID, ResourceReference: resource, EventID: event,
		EventIDDigest: eventDigest[:], PayloadDigest: payload[:], CreatedAt: timestamp(store.now().UTC()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil || len(stored) != sha256.Size {
		return false, unavailable(err)
	}
	return true, nil
}
