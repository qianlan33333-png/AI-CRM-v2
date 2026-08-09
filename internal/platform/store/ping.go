package platformstore

import (
	"context"
	"fmt"

	dbgen "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store/generated"
)

type PingStore struct {
	querier dbgen.Querier
}

func NewPingStore(db dbgen.DBTX) *PingStore {
	return &PingStore{querier: dbgen.New(db)}
}

func (store *PingStore) Ping(ctx context.Context) error {
	value, err := store.querier.Ping(ctx)
	if err != nil {
		return err
	}
	if value == 1 {
		return nil
	}
	return fmt.Errorf("platform store ping: unexpected value %d", value)
}
