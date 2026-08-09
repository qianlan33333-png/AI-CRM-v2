package platformstore

import (
	"context"
	"errors"
	"testing"
)

type pingQuerier struct {
	value int64
	err   error
}

func (querier pingQuerier) Ping(context.Context) (int64, error) {
	return querier.value, querier.err
}

func TestPingStore(t *testing.T) {
	if store := NewPingStore(nil); store == nil {
		t.Fatal("NewPingStore(nil) = nil")
	}

	t.Run("generated one succeeds", func(t *testing.T) {
		store := &PingStore{querier: pingQuerier{value: 1}}
		if err := store.Ping(context.Background()); err != nil {
			t.Fatalf("Ping() error = %v", err)
		}
	})

	t.Run("generated error remains errors Is visible", func(t *testing.T) {
		sentinel := errors.New("generated query failed")
		store := &PingStore{querier: pingQuerier{err: sentinel}}
		if err := store.Ping(context.Background()); !errors.Is(err, sentinel) {
			t.Fatalf("errors.Is(Ping() error, sentinel) = false; error = %v", err)
		}
	})

	t.Run("unexpected generated value has exact error", func(t *testing.T) {
		store := &PingStore{querier: pingQuerier{value: 0}}
		if err := store.Ping(context.Background()); err == nil {
			t.Fatal("Ping() error = nil, want non-nil")
		} else if got, want := err.Error(), "platform store ping: unexpected value 0"; got != want {
			t.Fatalf("Ping() error = %q, want %q", got, want)
		}
	})
}
