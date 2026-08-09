package platformriver

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

type invalidDirectionError Direction

func (direction invalidDirectionError) Error() string {
	return `platform river migration: invalid direction "` + string(direction) + `"`
}

func (direction invalidDirectionError) Unwrap() error {
	return ErrInvalidDirection
}

func Migrate(ctx context.Context, pool *pgxpool.Pool, direction Direction, options *MigrateOptions) error {
	if direction != DirectionUp && direction != DirectionDown {
		return invalidDirectionError(direction)
	}

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}

	riverDirection := rivermigrate.DirectionUp
	if direction == DirectionDown {
		riverDirection = rivermigrate.DirectionDown
	}

	var riverOptions *rivermigrate.MigrateOpts
	if options != nil {
		riverOptions = &rivermigrate.MigrateOpts{TargetVersion: options.TargetVersion}
	}

	_, err = migrator.Migrate(ctx, riverDirection, riverOptions)
	return err
}
