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
	if repository == nil || command.RiverJobID <= 0 || command.TaskID <= 0 || command.RiverAttempt <= 0 || command.RiverMaxAttempts < command.RiverAttempt || err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrInvalidSendCommand, err)
	}
	marker, err := queries.ReserveOutboundSendAttempt(ctx, outbounddb.ReserveOutboundSendAttemptParams{
		RiverJobID: command.RiverJobID, TaskID: int64(command.TaskID), JobKind: command.JobKind,
	})
	if err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	if _, err = queries.BackfillOutboundFirstAttemptHistory(ctx, outbounddb.BackfillOutboundFirstAttemptHistoryParams{
		RiverMaxAttempts: command.RiverMaxAttempts, SendAttemptID: marker.ID, RiverJobID: command.RiverJobID,
		TaskID: int64(command.TaskID), JobKind: command.JobKind,
	}); err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	history, err := queries.ReserveOutboundAttemptHistory(ctx, outbounddb.ReserveOutboundAttemptHistoryParams{
		RiverAttempt: command.RiverAttempt, RiverMaxAttempts: command.RiverMaxAttempts, SendAttemptID: marker.ID,
		RiverJobID: command.RiverJobID, TaskID: int64(command.TaskID), JobKind: command.JobKind,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.SendAttempt{}, outboundapp.ErrSendAttemptConflict
	}
	if err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	return storedAttemptWithHistory(
		marker.ID, history.ID, marker.RiverJobID, marker.TaskID, marker.JobKind,
		history.RiverAttempt, history.RiverMaxAttempts, history.State, history.FailureKind,
		history.ProviderCode, history.ProviderMessageID, history.CompletedAt,
	)
}

func (repository *SenderRepository) StartSendAttempt(ctx context.Context, attempt outboundapp.SendAttempt) (outboundapp.SendAttempt, bool, error) {
	queries, err := senderQueries(ctx)
	if repository == nil || attempt.ID <= 0 || attempt.HistoryID <= 0 || attempt.RiverAttempt <= 0 || err != nil {
		return outboundapp.SendAttempt{}, false, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	history, err := queries.StartOutboundAttemptHistory(ctx, outbounddb.StartOutboundAttemptHistoryParams{
		HistoryID: attempt.HistoryID, SendAttemptID: attempt.ID, RiverAttempt: attempt.RiverAttempt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		loaded, loadErr := queries.LoadOutboundAttemptHistory(ctx, attempt.HistoryID)
		if loadErr != nil {
			return outboundapp.SendAttempt{}, false, errors.Join(outboundapp.ErrSendAttemptFailed, loadErr)
		}
		stored, storeErr := storedAttemptWithHistory(
			loaded.SendAttemptID, loaded.ID, loaded.RiverJobID, loaded.TaskID, loaded.JobKind,
			loaded.RiverAttempt, loaded.RiverMaxAttempts, loaded.State, loaded.FailureKind,
			loaded.ProviderCode, loaded.ProviderMessageID, loaded.CompletedAt,
		)
		return stored, false, storeErr
	}
	if err != nil {
		return outboundapp.SendAttempt{}, false, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	if attempt.RiverAttempt > 1 {
		if _, err = queries.PrepareOutboundSendAttemptRetry(ctx, outbounddb.PrepareOutboundSendAttemptRetryParams{
			SendAttemptID: attempt.ID, HistoryID: attempt.HistoryID,
		}); err != nil {
			return outboundapp.SendAttempt{}, false, errors.Join(outboundapp.ErrSendAttemptFailed, err)
		}
	}
	marker, err := queries.StartOutboundSendAttempt(ctx, attempt.ID)
	if err != nil || !marker.Started || marker.State != string(outboundapp.SendAttemptDispatching) || marker.ID != attempt.ID ||
		marker.RiverJobID != attempt.RiverJobID || marker.TaskID != int64(attempt.TaskID) || marker.JobKind != attempt.JobKind {
		return outboundapp.SendAttempt{}, false, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	stored, err := storedAttemptWithHistory(
		marker.ID, history.ID, marker.RiverJobID, marker.TaskID, marker.JobKind,
		history.RiverAttempt, history.RiverMaxAttempts, history.State, history.FailureKind,
		history.ProviderCode, history.ProviderMessageID, history.CompletedAt,
	)
	return stored, true, err
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
	if repository == nil || command.ID <= 0 || command.HistoryID <= 0 || err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	history, err := queries.CompleteOutboundAttemptHistory(ctx, outbounddb.CompleteOutboundAttemptHistoryParams{
		AttemptState: string(command.State), FailureKind: string(command.FailureKind), ProviderCode: command.ProviderCode,
		ProviderMessageID: command.ProviderMessageID, HistoryID: command.HistoryID, SendAttemptID: command.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		loaded, loadErr := queries.LoadOutboundAttemptHistory(ctx, command.HistoryID)
		if loadErr != nil {
			return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, loadErr)
		}
		history = outbounddb.CompleteOutboundAttemptHistoryRow{
			ID: loaded.ID, SendAttemptID: loaded.SendAttemptID, RiverAttempt: loaded.RiverAttempt,
			RiverMaxAttempts: loaded.RiverMaxAttempts, State: loaded.State, FailureKind: loaded.FailureKind,
			ProviderCode: loaded.ProviderCode, ProviderMessageID: loaded.ProviderMessageID,
			DispatchStartedAt: loaded.DispatchStartedAt, CompletedAt: loaded.CompletedAt,
		}
	} else if err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	marker, err := queries.CompleteOutboundSendAttempt(ctx, outbounddb.CompleteOutboundSendAttemptParams{
		ID: command.ID, State: string(command.State), Column3: string(command.FailureKind),
		Column4: command.ProviderCode, Column5: command.ProviderMessageID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.SendAttempt{}, outboundapp.ErrSendAttemptFailed
	}
	if err != nil {
		return outboundapp.SendAttempt{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	if marker.State != history.State || marker.ID != history.SendAttemptID {
		return outboundapp.SendAttempt{}, outboundapp.ErrSendAttemptFailed
	}
	return storedAttemptWithHistory(
		marker.ID, history.ID, marker.RiverJobID, marker.TaskID, marker.JobKind,
		history.RiverAttempt, history.RiverMaxAttempts, history.State, history.FailureKind,
		history.ProviderCode, history.ProviderMessageID, history.CompletedAt,
	)
}

func (repository *SenderRepository) MarkTaskSending(ctx context.Context, attempt outboundapp.SendAttempt) error {
	queries, err := senderQueries(ctx)
	if repository == nil || attempt.ID <= 0 || attempt.State != outboundapp.SendAttemptDispatching || err != nil {
		return errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	rows, err := queries.MarkOutboundTaskSending(ctx, outbounddb.MarkOutboundTaskSendingParams{AttemptID: attempt.ID, HistoryID: attempt.HistoryID})
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
	_, err = queries.ProjectOutboundTaskResult(ctx, outbounddb.ProjectOutboundTaskResultParams{
		TaskStatus: string(status), AttemptID: attempt.ID, HistoryID: attempt.HistoryID, AttemptState: string(attempt.State),
	})
	if err != nil {
		return outboundapp.TaskResultFact{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	row, err := queries.LoadOutboundTaskResultFact(ctx, attempt.HistoryID)
	if err != nil || !row.CompletedAt.Valid || row.CurrentAttemptCount < row.RiverAttempt ||
		(row.CurrentAttemptCount == row.RiverAttempt && row.CurrentTaskStatus != string(status)) {
		return outboundapp.TaskResultFact{}, errors.Join(outboundapp.ErrSendAttemptFailed, err)
	}
	fact := outboundapp.TaskResultFact{
		TaskID: outboundapp.TaskID(row.TaskID), CustomerID: row.CustomerID, AttemptID: row.AttemptID,
		HistoryID: row.HistoryID, RiverJobID: row.RiverJobID, RiverAttempt: row.RiverAttempt,
		RiverMaxAttempts: row.RiverMaxAttempts, Status: status, AttemptCount: row.RiverAttempt,
		OccurredAt: row.CompletedAt.Time,
	}
	if row.FailureKind.Valid {
		fact.FailureKind = outboundapp.ProviderFailureKind(row.FailureKind.String)
	}
	if row.ProviderCode.Valid {
		fact.ProviderCode = row.ProviderCode.String
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

func storedAttemptWithHistory(
	id, historyID, riverJobID, taskID int64,
	jobKind string,
	riverAttempt, riverMaxAttempts int32,
	state string,
	failureKind, providerCode, providerMessageID pgtype.Text,
	completedAt pgtype.Timestamptz,
) (outboundapp.SendAttempt, error) {
	if id <= 0 || historyID <= 0 || riverJobID <= 0 || taskID <= 0 || jobKind == "" || riverAttempt <= 0 || riverMaxAttempts < riverAttempt || state == "" {
		return outboundapp.SendAttempt{}, outboundapp.ErrSendAttemptFailed
	}
	attempt := outboundapp.SendAttempt{
		ID: id, HistoryID: historyID, RiverJobID: riverJobID, TaskID: outboundapp.TaskID(taskID), JobKind: jobKind,
		RiverAttempt: riverAttempt, RiverMaxAttempts: riverMaxAttempts, State: outboundapp.SendAttemptState(state),
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
