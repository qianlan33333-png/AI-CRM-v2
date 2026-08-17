package opsstore

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type readinessQuerier struct {
	ping       int64
	pingErr    error
	compatible pgtype.Bool
	schemaErr  error
}

func (querier readinessQuerier) Ping(context.Context) (int64, error) {
	return querier.ping, querier.pingErr
}

func (querier readinessQuerier) CurrentSchemaCompatible(context.Context) (pgtype.Bool, error) {
	return querier.compatible, querier.schemaErr
}

func TestReadinessRepositoryObserve(t *testing.T) {
	t.Run("healthy and compatible", func(t *testing.T) {
		repository := &ReadinessRepository{querier: readinessQuerier{
			ping: 1, compatible: pgtype.Bool{Bool: true, Valid: true},
		}}
		snapshot, err := repository.Observe(context.Background())
		if err != nil || !snapshot.CurrentSchemaCompatible {
			t.Fatalf("Observe() = %+v, %v", snapshot, err)
		}
	})

	t.Run("database probe fails closed", func(t *testing.T) {
		repository := &ReadinessRepository{querier: readinessQuerier{ping: 0}}
		if _, err := repository.Observe(context.Background()); err == nil || err.Error() != "ops readiness database probe failed" {
			t.Fatalf("Observe() error = %v", err)
		}
	})

	t.Run("schema probe fails closed", func(t *testing.T) {
		repository := &ReadinessRepository{querier: readinessQuerier{ping: 1, schemaErr: errors.New("unavailable")}}
		if _, err := repository.Observe(context.Background()); err == nil || err.Error() != "ops readiness schema probe failed" {
			t.Fatalf("Observe() error = %v", err)
		}
	})

	t.Run("nil repository fails closed", func(t *testing.T) {
		var repository *ReadinessRepository
		if _, err := repository.Observe(context.Background()); err == nil || err.Error() != "ops readiness repository is required" {
			t.Fatalf("Observe() error = %v", err)
		}
	})
}
