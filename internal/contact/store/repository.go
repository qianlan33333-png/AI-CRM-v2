// Package store implements contact-owned PostgreSQL persistence.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) ListStages(ctx context.Context) ([]contactport.Stage, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, name, sort_order, config
		FROM stages
		WHERE archived_at IS NULL
		ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stages := make([]contactport.Stage, 0)
	for rows.Next() {
		stage, scanErr := scanStage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stages, nil
}

func (*Repository) GetStage(ctx context.Context, id contactport.StageID) (contactport.Stage, error) {
	if id <= 0 {
		return contactport.Stage{}, contactport.ErrStageNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.Stage{}, err
	}
	stage, err := queryStage(ctx, tx, `SELECT id, name, sort_order, config FROM stages WHERE id = $1`, int64(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.Stage{}, contactport.ErrStageNotFound
	}
	return stage, err
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
	return contactport.Stage{
		ID: contactport.StageID(row.ID), Name: row.Name,
		SortOrder: row.SortOrder, Config: row.Config,
	}, nil
}

func (repository *Repository) RenameStage(ctx context.Context, command contactport.RenameStageCommand) (contactport.Stage, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.Stage{}, err
	}
	stage, err := queryStage(ctx, tx, `
		UPDATE stages
		SET name = $2
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id, name, sort_order, config`, int64(command.ID), command.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.Stage{}, contactport.ErrStageNotFound
	}
	if err != nil {
		return contactport.Stage{}, err
	}
	return stage, nil
}

func (repository *Repository) ReorderStages(ctx context.Context, ids []contactport.StageID) ([]contactport.Stage, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	for order, id := range ids {
		if id <= 0 {
			return nil, contactport.ErrInvalidStage
		}
		result, execErr := tx.Exec(ctx, `UPDATE stages SET sort_order = $2 WHERE id = $1 AND archived_at IS NULL`, int64(id), int32(order))
		if execErr != nil {
			return nil, execErr
		}
		if result.RowsAffected() != 1 {
			return nil, contactport.ErrStageNotFound
		}
	}
	return repository.ListStages(ctx)
}

func (repository *Repository) ArchiveStage(ctx context.Context, command contactport.ArchiveStageCommand, at time.Time) (contactport.Stage, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.Stage{}, err
	}
	var referenced bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM customers WHERE stage_id = $1)`, int64(command.ID)).Scan(&referenced); err != nil {
		return contactport.Stage{}, err
	}
	if referenced {
		return contactport.Stage{}, contactport.ErrStageReferenced
	}
	stage, err := queryStage(ctx, tx, `
		UPDATE stages
		SET archived_at = $2, archived_by = $3
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id, name, sort_order, config`, int64(command.ID), at.UTC(), string(command.Actor))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.Stage{}, contactport.ErrStageNotFound
	}
	return stage, err
}

func queriesFromContext(ctx context.Context) (*contactdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return contactdb.New(tx), nil
}

type stageScanner interface{ Scan(...any) error }

func scanStage(row stageScanner) (contactport.Stage, error) {
	var id int64
	var name string
	var sortOrder int32
	var config []byte
	if err := row.Scan(&id, &name, &sortOrder, &config); err != nil {
		return contactport.Stage{}, err
	}
	if id <= 0 || name == "" || len(config) == 0 {
		return contactport.Stage{}, fmt.Errorf("invalid stage row")
	}
	return contactport.Stage{ID: contactport.StageID(id), Name: name, SortOrder: sortOrder, Config: append([]byte(nil), config...)}, nil
}

func queryStage(ctx context.Context, tx pgx.Tx, query string, args ...any) (contactport.Stage, error) {
	return scanStage(tx.QueryRow(ctx, query, args...))
}

func (repository *Repository) ReserveStageReceipt(ctx context.Context, reservation contactapp.StageReceiptReservation) (contactapp.StageReceipt, bool, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.StageReceipt{}, false, err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO stage_operation_receipts(operation, actor, key_digest, payload_digest, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (operation, actor, key_digest) DO NOTHING
		RETURNING id, operation, actor, key_digest, payload_digest, state, result_ids`, string(reservation.Operation), string(reservation.Actor), reservation.KeyDigest[:], reservation.PayloadDigest[:], reservation.CreatedAt.UTC())
	receipt, scanErr := scanStageReceipt(row)
	if scanErr == nil {
		return receipt, true, nil
	}
	if !errors.Is(scanErr, pgx.ErrNoRows) {
		return contactapp.StageReceipt{}, false, scanErr
	}
	receipt, scanErr = scanStageReceipt(tx.QueryRow(ctx, `
		SELECT id, operation, actor, key_digest, payload_digest, state, result_ids
		FROM stage_operation_receipts WHERE operation = $1 AND actor = $2 AND key_digest = $3`, string(reservation.Operation), string(reservation.Actor), reservation.KeyDigest[:]))
	return receipt, false, scanErr
}

func (repository *Repository) CompleteStageReceipt(ctx context.Context, id int64, resultIDs []contactport.StageID, at time.Time) (contactapp.StageReceipt, error) {
	if id <= 0 || len(resultIDs) == 0 {
		return contactapp.StageReceipt{}, contactport.ErrStageConflict
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.StageReceipt{}, err
	}
	ids := make([]int64, len(resultIDs))
	for index, resultID := range resultIDs {
		if resultID <= 0 {
			return contactapp.StageReceipt{}, contactport.ErrStageConflict
		}
		ids[index] = int64(resultID)
	}
	return scanStageReceipt(tx.QueryRow(ctx, `
		UPDATE stage_operation_receipts
		SET state = 'completed', result_ids = $2, completed_at = $3
		WHERE id = $1 AND state = 'in_progress'
		RETURNING id, operation, actor, key_digest, payload_digest, state, result_ids`, id, ids, at.UTC()))
}

func scanStageReceipt(row stageScanner) (contactapp.StageReceipt, error) {
	var receipt contactapp.StageReceipt
	var operation string
	var actor string
	var keyDigest []byte
	var payloadDigest []byte
	var resultIDs []int64
	if err := row.Scan(&receipt.ID, &operation, &actor, &keyDigest, &payloadDigest, &receipt.State, &resultIDs); err != nil {
		return contactapp.StageReceipt{}, err
	}
	if receipt.ID <= 0 || (operation != "reorder" && operation != "archive") || actor == "" || len(keyDigest) != 32 || len(payloadDigest) != 32 || (receipt.State != "in_progress" && receipt.State != "completed") {
		return contactapp.StageReceipt{}, fmt.Errorf("invalid stage receipt")
	}
	receipt.Operation = contactapp.StageOperation(operation)
	receipt.Actor = contactport.Actor(actor)
	copy(receipt.KeyDigest[:], keyDigest)
	copy(receipt.PayloadDigest[:], payloadDigest)
	receipt.ResultIDs = make([]contactport.StageID, len(resultIDs))
	for index, resultID := range resultIDs {
		if resultID <= 0 {
			return contactapp.StageReceipt{}, fmt.Errorf("invalid stage receipt result")
		}
		receipt.ResultIDs[index] = contactport.StageID(resultID)
	}
	return receipt, nil
}
