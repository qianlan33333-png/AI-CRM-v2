package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type SenderRepository struct{}

var _ outboundapp.SendAttemptRepository = (*SenderRepository)(nil)

func NewSenderRepository() *SenderRepository { return &SenderRepository{} }

func (repository *SenderRepository) ReserveSendAttempt(ctx context.Context, command outboundapp.SendCommand) (outboundapp.SendAttempt, error) {
	queries, err := senderQueries(ctx)
	if repository == nil || command.RiverJobID <= 0 || command.TaskID <= 0 || err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrInvalidSendCommand, err)
	}
	row, err := queries.ReserveOutboundSendAttempt(ctx, outbounddb.ReserveOutboundSendAttemptParams{
		RiverJobID: command.RiverJobID, TaskID: int64(command.TaskID), JobKind: command.JobKind,
	})
	if err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	return storedAttempt(row.ID, row.RiverJobID, row.TaskID, row.JobKind, row.State, row.FailureKind, row.ProviderCode, row.ProviderMessageID, row.CompletedAt)
}

func (repository *SenderRepository) StartSendAttempt(ctx context.Context, attemptID int64) (outboundapp.SendAttempt, bool, error) {
	queries, err := senderQueries(ctx)
	if repository == nil || attemptID <= 0 || err != nil {
		return outboundapp.SendAttempt{}, false, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	row, err := queries.StartOutboundSendAttempt(ctx, attemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.SendAttempt{}, false, outboundapp.ErrSendAttemptFailed
	}
	if err != nil {
		return outboundapp.SendAttempt{}, false, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	attempt, err := storedAttempt(row.ID, row.RiverJobID, row.TaskID, row.JobKind, row.State, row.FailureKind, row.ProviderCode, row.ProviderMessageID, row.CompletedAt)
	return attempt, row.Started, err
}

func (repository *SenderRepository) LoadSendRequest(ctx context.Context, taskID outboundapp.TaskID) (outboundapp.SendRequest, error) {
	queries, err := senderQueries(ctx)
	if repository == nil || taskID <= 0 || err != nil {
		return outboundapp.SendRequest{}, errors.Join(outboundapp.ErrInvalidSendCommand, err)
	}
	row, err := queries.LoadOutboundSendRequest(ctx, int64(taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.SendRequest{}, outboundapp.ErrInvalidSendCommand
	}
	if err != nil {
		return outboundapp.SendRequest{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	return outboundapp.SendRequest{TaskID: outboundapp.TaskID(row.ID), CustomerID: row.CustomerID, TemplateKey: row.TemplateKey, Payload: row.Payload}, nil
}

func (repository *SenderRepository) CompleteSendAttempt(ctx context.Context, command outboundapp.CompleteSendAttempt) (outboundapp.SendAttempt, error) {
	queries, err := senderQueries(ctx)
	if repository == nil || command.ID <= 0 || err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	row, err := queries.CompleteOutboundSendAttempt(ctx, outbounddb.CompleteOutboundSendAttemptParams{
		ID: command.ID, State: string(command.State), Column3: string(command.FailureKind),
		Column4: command.ProviderCode, Column5: command.ProviderMessageID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.SendAttempt{}, outboundapp.ErrSendAttemptFailed
	}
	if err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	return storedAttempt(row.ID, row.RiverJobID, row.TaskID, row.JobKind, row.State, row.FailureKind, row.ProviderCode, row.ProviderMessageID, row.CompletedAt)
}

func (repository *SenderRepository) MarkTaskSending(ctx context.Context, attempt outboundapp.SendAttempt) error {
	queries, err := senderQueries(ctx)
	if repository == nil || attempt.ID <= 0 || attempt.State != outboundapp.SendAttemptDispatching || err != nil {
		return errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	rows, err := queries.MarkOutboundTaskSending(ctx, attempt.ID)
	if err != nil || rows != 1 {
		return errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	return nil
}

func (repository *SenderRepository) ProjectTaskResult(ctx context.Context, attempt outboundapp.SendAttempt) (outboundapp.TaskResultFact, error) {
	queries, err := senderQueries(ctx)
	status := storedTaskStatus(attempt.State)
	if repository == nil || attempt.ID <= 0 || status == "" || err != nil {
		return outboundapp.TaskResultFact{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	row, err := queries.ProjectOutboundTaskResult(ctx, outbounddb.ProjectOutboundTaskResultParams{
		TaskStatus: string(status), AttemptID: attempt.ID, AttemptState: string(attempt.State),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.TaskResultFact{}, outboundapp.ErrSendAttemptFailed
	}
	if err != nil || !row.CompletedAt.Valid {
		return outboundapp.TaskResultFact{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	fact := outboundapp.TaskResultFact{
		TaskID: outboundapp.TaskID(row.TaskID), CustomerID: row.CustomerID, AttemptID: row.AttemptID,
		RiverJobID: row.RiverJobID, Status: outboundapp.TaskStatus(row.Status), AttemptCount: row.AttemptCount,
		OccurredAt: row.CompletedAt.Time,
	}
	if row.LastFailureKind.Valid {
		fact.FailureKind = outboundapp.ProviderFailureKind(row.LastFailureKind.String)
	}
	if row.LastError.Valid {
		fact.ProviderCode = row.LastError.String
	}
	if row.ProviderMessageID.Valid {
		fact.ProviderMessageID = row.ProviderMessageID.String
	}
	return fact, nil
}

func senderQueries(ctx context.Context) (*outbounddb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return outbounddb.New(tx), nil
}

func storedAttempt(id, riverJobID, taskID int64, jobKind, state string, failureKind, providerCode, providerMessageID pgtype.Text, completedAt pgtype.Timestamptz) (outboundapp.SendAttempt, error) {
	if id <= 0 || riverJobID <= 0 || taskID <= 0 || jobKind == "" || state == "" {
		return outboundapp.SendAttempt{}, outboundapp.ErrSendAttemptFailed
	}
	attempt := outboundapp.SendAttempt{
		ID: id, RiverJobID: riverJobID, TaskID: outboundapp.TaskID(taskID), JobKind: jobKind,
		State: outboundapp.SendAttemptState(state),
	}
	if failureKind.Valid {
		attempt.FailureKind = outboundapp.ProviderFailureKind(failureKind.String)
	}
	if providerCode.Valid {
		attempt.ProviderCode = providerCode.String
	}
	if providerMessageID.Valid {
		attempt.ProviderMessageID = providerMessageID.String
	}
	if completedAt.Valid {
		attempt.CompletedAt = completedAt.Time
	}
	return attempt, nil
}

func storedTaskStatus(state outboundapp.SendAttemptState) outboundapp.TaskStatus {
	switch state {
	case outboundapp.SendAttemptSucceeded:
		return outboundapp.TaskStatusSent
	case outboundapp.SendAttemptRetryableFailed:
		return outboundapp.TaskStatusRetryableFailed
	case outboundapp.SendAttemptFinalFailed:
		return outboundapp.TaskStatusFinalFailed
	case outboundapp.SendAttemptOutcomeUnknown:
		return outboundapp.TaskStatusOutcomeUnknown
	default:
		return ""
	}
}
