package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
)

type OperationsRepository struct{}

var _ surveyapp.OperationsStore = (*OperationsRepository)(nil)

func NewOperationsRepository() *OperationsRepository { return &OperationsRepository{} }

func (r *OperationsRepository) ReadOperations(ctx context.Context, questionnaireID surveyport.ID) (surveyport.OperationsProjection, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || questionnaireID < 1 {
		return surveyport.OperationsProjection{}, unavailable(err)
	}
	row, err := q.GetQuestionnaireOperations(ctx, int64(questionnaireID))
	if err != nil {
		return surveyport.OperationsProjection{}, unavailable(err)
	}
	return surveyport.OperationsProjection{
		QuestionnaireID: surveyport.ID(row.QuestionnaireID),
		Completion: surveyport.CompletionOperations{
			NavigationTargetID: row.NavigationTargetID,
			ChannelID:          row.ChannelID,
		},
		ExternalPush: surveyport.ExternalPushOperations{
			Enabled:                row.ExternalPushEnabled,
			ConfigurationReference: row.ExternalPushConfigurationReference,
		},
		LocalOnly: true,
	}, nil
}

func (r *OperationsRepository) SaveCompletionOperations(ctx context.Context, questionnaireID surveyport.ID, value surveyport.CompletionOperations, now time.Time) error {
	q, err := queries(ctx)
	if r == nil || err != nil || questionnaireID < 1 || now.IsZero() {
		return unavailable(err)
	}
	writtenID, err := q.SaveQuestionnaireCompletionOperations(ctx, surveydb.SaveQuestionnaireCompletionOperationsParams{
		NavigationTargetID: value.NavigationTargetID,
		ChannelID:          value.ChannelID,
		UpdatedAt:          timestamp(now),
		QuestionnaireID:    int64(questionnaireID),
	})
	if err != nil || writtenID != int64(questionnaireID) {
		return unavailable(err)
	}
	return nil
}

func (r *OperationsRepository) SaveExternalPushOperations(ctx context.Context, questionnaireID surveyport.ID, value surveyport.ExternalPushOperations, now time.Time) error {
	q, err := queries(ctx)
	if r == nil || err != nil || questionnaireID < 1 || now.IsZero() {
		return unavailable(err)
	}
	writtenID, err := q.SaveQuestionnaireExternalPushOperations(ctx, surveydb.SaveQuestionnaireExternalPushOperationsParams{
		ExternalPushEnabled:                value.Enabled,
		ExternalPushConfigurationReference: value.ConfigurationReference,
		UpdatedAt:                          timestamp(now),
		QuestionnaireID:                    int64(questionnaireID),
	})
	if err != nil || writtenID != int64(questionnaireID) {
		return unavailable(err)
	}
	return nil
}

func (r *OperationsRepository) ReserveOperations(ctx context.Context, operation string, reservation surveyapp.OperationsReservation) (surveyapp.OperationsReceipt, bool, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return surveyapp.OperationsReceipt{}, false, unavailable(err)
	}
	row, err := q.ReserveQuestionnaireOperationsReceipt(ctx, surveydb.ReserveQuestionnaireOperationsReceiptParams{
		Operation: operation, ActorScope: reservation.ActorScope,
		KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:],
		CreatedAt: timestamp(reservation.CreatedAt),
	})
	if err == nil {
		return operationsReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return surveyapp.OperationsReceipt{}, false, unavailable(err)
	}
	stored, readErr := q.GetQuestionnaireOperationsReceipt(ctx, surveydb.GetQuestionnaireOperationsReceiptParams{
		Operation: operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:],
	})
	if readErr != nil {
		return surveyapp.OperationsReceipt{}, false, unavailable(readErr)
	}
	return operationsReceipt(stored.ID, stored.Operation, stored.ActorScope, stored.KeyDigest, stored.PayloadDigest, stored.State, stored.ResultSnapshot), false, nil
}

func (r *OperationsRepository) CompleteOperations(ctx context.Context, receiptID int64, snapshot json.RawMessage, now time.Time) (surveyapp.OperationsReceipt, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || receiptID < 1 || now.IsZero() || !json.Valid(snapshot) {
		return surveyapp.OperationsReceipt{}, unavailable(err)
	}
	row, err := q.CompleteQuestionnaireOperationsReceipt(ctx, surveydb.CompleteQuestionnaireOperationsReceiptParams{
		ID: receiptID, ResultSnapshot: snapshot, CompletedAt: timestamp(now),
	})
	if err != nil {
		return surveyapp.OperationsReceipt{}, unavailable(err)
	}
	return operationsReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func (r *OperationsRepository) CreateQueuedExternalPushTest(ctx context.Context, questionnaireID surveyport.ID, receiptID int64, now time.Time) (int64, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || questionnaireID < 1 || receiptID < 1 || now.IsZero() {
		return 0, unavailable(err)
	}
	id, err := q.CreateQueuedQuestionnaireExternalPushTest(ctx, surveydb.CreateQueuedQuestionnaireExternalPushTestParams{
		QuestionnaireID: int64(questionnaireID), OperationReceiptID: receiptID, CreatedAt: timestamp(now),
	})
	if err != nil || id < 1 {
		return 0, unavailable(err)
	}
	return id, nil
}

func (r *OperationsRepository) ReadExternalPushTest(ctx context.Context, questionnaireID surveyport.ID, testRunID int64) (surveyport.ExternalPushTest, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || questionnaireID < 1 || testRunID < 1 {
		return surveyport.ExternalPushTest{}, unavailable(err)
	}
	row, err := q.GetQuestionnaireExternalPushTest(ctx, surveydb.GetQuestionnaireExternalPushTestParams{QuestionnaireID: int64(questionnaireID), TestRunID: testRunID})
	if err != nil {
		return surveyport.ExternalPushTest{}, unavailable(err)
	}
	return externalPushTest(row.ID, row.QuestionnaireID, row.Status, row.AttemptCount, row.SideEffectExecuted,
		row.ProviderResultReceived, row.UnknownAfterDispatch, row.AutoRetryAllowed, row.CreatedAt, row.UpdatedAt)
}

func (r *OperationsRepository) CountExternalPushTests(ctx context.Context, questionnaireID *surveyport.ID) (int64, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return 0, unavailable(err)
	}
	var total int64
	if questionnaireID == nil {
		total, err = q.CountGlobalQuestionnaireExternalPushTests(ctx)
	} else if *questionnaireID < 1 {
		return 0, surveyapp.ErrInvalidOperations
	} else {
		total, err = q.CountQuestionnaireExternalPushTests(ctx, int64(*questionnaireID))
	}
	if err != nil || total < 0 {
		return 0, unavailable(err)
	}
	return total, nil
}

func (r *OperationsRepository) ListExternalPushTests(ctx context.Context, questionnaireID *surveyport.ID, limit, offset int32) ([]surveyport.ExternalPushTest, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || limit < 1 || offset < 0 {
		return nil, unavailable(err)
	}
	if questionnaireID == nil {
		rows, listErr := q.ListGlobalQuestionnaireExternalPushTests(ctx, surveydb.ListGlobalQuestionnaireExternalPushTestsParams{RowLimit: limit, RowOffset: offset})
		if listErr != nil {
			return nil, unavailable(listErr)
		}
		result := make([]surveyport.ExternalPushTest, len(rows))
		for index, row := range rows {
			result[index], err = externalPushTest(row.ID, row.QuestionnaireID, row.Status, row.AttemptCount, row.SideEffectExecuted,
				row.ProviderResultReceived, row.UnknownAfterDispatch, row.AutoRetryAllowed, row.CreatedAt, row.UpdatedAt)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	if *questionnaireID < 1 {
		return nil, surveyapp.ErrInvalidOperations
	}
	rows, listErr := q.ListQuestionnaireExternalPushTests(ctx, surveydb.ListQuestionnaireExternalPushTestsParams{
		QuestionnaireID: int64(*questionnaireID), RowLimit: limit, RowOffset: offset,
	})
	if listErr != nil {
		return nil, unavailable(listErr)
	}
	result := make([]surveyport.ExternalPushTest, len(rows))
	for index, row := range rows {
		result[index], err = externalPushTest(row.ID, row.QuestionnaireID, row.Status, row.AttemptCount, row.SideEffectExecuted,
			row.ProviderResultReceived, row.UnknownAfterDispatch, row.AutoRetryAllowed, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func operationsReceipt(id int64, operation, actorScope string, keyDigest, payloadDigest []byte, state string, snapshot []byte) surveyapp.OperationsReceipt {
	value := surveyapp.OperationsReceipt{
		ID: id, Operation: operation, ActorScope: actorScope, State: state,
		ResultSnapshot: append(json.RawMessage{}, snapshot...),
	}
	copy(value.KeyDigest[:], keyDigest)
	copy(value.PayloadDigest[:], payloadDigest)
	return value
}

func externalPushTest(id, questionnaireID int64, status string, attemptCount int32, sideEffectExecuted,
	providerResultReceived, unknownAfterDispatch, autoRetryAllowed bool, createdAt, updatedAt pgtype.Timestamptz,
) (surveyport.ExternalPushTest, error) {
	if id < 1 || questionnaireID < 1 || !createdAt.Valid || !updatedAt.Valid {
		return surveyport.ExternalPushTest{}, surveyapp.ErrUnavailable
	}
	return surveyport.ExternalPushTest{
		TestRunID: id, QuestionnaireID: surveyport.ID(questionnaireID), Status: status,
		AttemptCount: attemptCount, SideEffectExecuted: sideEffectExecuted,
		ProviderResultReceived: providerResultReceived, UnknownAfterDispatch: unknownAfterDispatch,
		AutoRetryAllowed: autoRetryAllowed, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time,
	}, nil
}
