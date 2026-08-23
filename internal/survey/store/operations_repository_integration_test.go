package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	operationsCompletionSaveOperation = "operations_completion_save"
	operationsQueueTestOperation      = "operations_external_push_test_queue"
)

// Uses an already-migrated dedicated local PostgreSQL database. The test
// deliberately rolls its transaction back so immutable operation receipts and
// event-log rows never escape the scenario.
func TestOperationsRepositoryPostgreSQLAtomicLocalOnlyRoundTrip(t *testing.T) {
	databaseURL := os.Getenv("AICRM_SURVEY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AICRM_SURVEY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}

	repository := NewOperationsRepository()
	events := &surveyOperationsIntegrationEvents{}
	uow := platformstore.NewUnitOfWork(pool)
	now := time.Date(2026, time.August, 22, 11, 0, 0, 0, time.UTC)
	err = uow.Within(ctx, func(tx context.Context) error {
		db, txErr := platformstore.TxFromContext(tx)
		if txErr != nil {
			return txErr
		}
		var questionnaireID int64
		txErr = db.QueryRow(tx, `INSERT INTO questionnaires (slug, name, title, description, answer_display_mode, assessment_enabled, assessment_config, is_disabled, created_by, version, submission_count, created_at, updated_at)
VALUES ('operations-it', 'operations-it', 'operations-it', '', 'all_in_one', FALSE, '{}'::jsonb, FALSE, 41, 1, 0, $1, $1)
RETURNING id`, now).Scan(&questionnaireID)
		if txErr != nil {
			return txErr
		}
		id := surveyport.ID(questionnaireID)
		completion := surveyport.CompletionOperations{NavigationTargetID: "target-operations", ChannelID: 9}
		completionReceipt, owned, txErr := repository.ReserveOperations(tx, operationsCompletionSaveOperation, operationsReservation("completion", now))
		if txErr != nil || !owned {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		if txErr = repository.SaveCompletionOperations(tx, id, completion, now); txErr != nil {
			return txErr
		}
		projection, txErr := repository.ReadOperations(tx, id)
		if txErr != nil || projection.QuestionnaireID != id || projection.Completion != completion || !projection.LocalOnly {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		if _, txErr = events.Append(tx, eventport.Event{Type: eventport.EvSurveyUpdated, Payload: json.RawMessage(`{"questionnaire_id":1}`), OccurredAt: now, IdempotencyKey: "survey.operations.integration.completion"}); txErr != nil {
			return txErr
		}
		completionSnapshot, txErr := json.Marshal(projection)
		if txErr != nil {
			return txErr
		}
		completedReceipt, txErr := repository.CompleteOperations(tx, completionReceipt.ID, completionSnapshot, now)
		var storedProjection surveyport.OperationsProjection
		if json.Unmarshal(completedReceipt.ResultSnapshot, &storedProjection) != nil || txErr != nil || completedReceipt.State != "completed" || storedProjection != projection {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}

		external := surveyport.ExternalPushOperations{Enabled: true, ConfigurationReference: "config-operations"}
		if txErr = repository.SaveExternalPushOperations(tx, id, external, now); txErr != nil {
			return txErr
		}
		projection, txErr = repository.ReadOperations(tx, id)
		if txErr != nil || projection.ExternalPush != external {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		queueReceipt, owned, txErr := repository.ReserveOperations(tx, operationsQueueTestOperation, operationsReservation("queue", now))
		if txErr != nil || !owned {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		testRunID, txErr := repository.CreateQueuedExternalPushTest(tx, id, queueReceipt.ID, now)
		if txErr != nil {
			return txErr
		}
		testRun, txErr := repository.ReadExternalPushTest(tx, id, testRunID)
		if txErr != nil || testRun.Status != surveyapp.ExternalPushTestQueued || testRun.AttemptCount != 0 || testRun.SideEffectExecuted || testRun.ProviderResultReceived || testRun.UnknownAfterDispatch || testRun.AutoRetryAllowed {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		if _, txErr = events.Append(tx, eventport.Event{Type: eventport.EvSurveyUpdated, Payload: json.RawMessage(`{"questionnaire_id":1}`), OccurredAt: now, IdempotencyKey: "survey.operations.integration.queue"}); txErr != nil {
			return txErr
		}
		queueSnapshot, txErr := json.Marshal(testRun)
		if txErr != nil {
			return txErr
		}
		if completedReceipt, txErr = repository.CompleteOperations(tx, queueReceipt.ID, queueSnapshot, now); txErr != nil || completedReceipt.State != "completed" {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		globalTotal, txErr := repository.CountExternalPushTests(tx, nil)
		if txErr != nil || globalTotal < 1 {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		page, txErr := repository.ListExternalPushTests(tx, &id, 50, 0)
		if txErr != nil || len(page) != 1 || page[0] != testRun {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		var operationRows, receiptRows int
		if txErr = db.QueryRow(tx, `SELECT count(*) FROM questionnaire_operations WHERE questionnaire_id = $1`, questionnaireID).Scan(&operationRows); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, `SELECT count(*) FROM questionnaire_operations_receipts WHERE id IN ($1, $2) AND state = 'completed'`, completionReceipt.ID, queueReceipt.ID).Scan(&receiptRows); txErr != nil {
			return txErr
		}
		if operationRows != 1 || receiptRows != 2 || events.count != 2 {
			return surveyapp.ErrUnavailable
		}
		return errOperationsRepositoryRollback
	})
	if !errors.Is(err, errOperationsRepositoryRollback) {
		t.Fatalf("round trip=%v", err)
	}

	err = uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, txErr := repository.ReserveOperations(tx, operationsCompletionSaveOperation, operationsReservation("incomplete", now))
		if txErr != nil || !owned || receipt.ID < 1 {
			return errors.Join(surveyapp.ErrUnavailable, txErr)
		}
		return nil
	})
	if err == nil {
		t.Fatal("incomplete receipt transaction committed")
	}
}

func operationsReservation(label string, now time.Time) surveyapp.OperationsReservation {
	key := sha256.Sum256([]byte("operations-integration-key-" + label))
	payload := sha256.Sum256([]byte("operations-integration-payload-" + label))
	return surveyapp.OperationsReservation{ActorScope: "admin:41", KeyDigest: key, PayloadDigest: payload, CreatedAt: now}
}

type surveyOperationsIntegrationEvents struct{ count int }

func (events *surveyOperationsIntegrationEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	events.count++
	return eventport.EventID(events.count), nil
}

var errOperationsRepositoryRollback = errors.New("rollback survey operations repository integration")
