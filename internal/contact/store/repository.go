// Package store implements contact-owned PostgreSQL persistence.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) ListStages(ctx context.Context) ([]contactport.Stage, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListStages(ctx)
	if err != nil {
		return nil, err
	}
	stages := make([]contactport.Stage, 0, len(rows))
	for _, row := range rows {
		stages = append(stages, stageFromRow(row))
	}
	return stages, nil
}

func (repository *Repository) InsertStage(ctx context.Context, command contactport.CreateStageCommand) (contactport.Stage, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return contactport.Stage{}, err
	}
	row, err := queries.InsertStage(ctx, contactdb.InsertStageParams{
		Name: command.Name, SortOrder: command.SortOrder, Config: command.Config,
	})
	if err != nil {
		return contactport.Stage{}, err
	}
	return stageFromRow(row), nil
}

func (repository *Repository) RenameStage(ctx context.Context, command contactport.RenameStageCommand) (contactport.Stage, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return contactport.Stage{}, err
	}
	row, err := queries.RenameStage(ctx, contactdb.RenameStageParams{
		ID: int64(command.ID), Name: command.Name,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.Stage{}, contactport.ErrStageNotFound
	}
	if err != nil {
		return contactport.Stage{}, err
	}
	return stageFromRow(row), nil
}

func queriesFromContext(ctx context.Context) (*contactdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return contactdb.New(tx), nil
}

func stageFromRow(row contactdb.Stage) contactport.Stage {
	return contactport.Stage{
		ID: contactport.StageID(row.ID), Name: row.Name,
		SortOrder: row.SortOrder, Config: row.Config,
	}
}
