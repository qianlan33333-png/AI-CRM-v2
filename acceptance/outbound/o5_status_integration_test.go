package outbound_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestSenderProjectsStableTaskStatusAndResultEvent(t *testing.T) {
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundFixture(t, pool)

	provider := &fixtureProvider{}
	sender := outboundapp.NewSenderService(
		platformstore.NewUnitOfWork(pool), outboundstore.NewSenderRepository(), eventstore.NewAppender(), provider, fixtureRateGate{},
	)
	for index, test := range []struct {
		outcome   string
		status    outboundapp.TaskStatus
		eventType string
	}{
		{"success", outboundapp.TaskStatusSent, eventport.EvOutboundSent},
		{"rate_limited", outboundapp.TaskStatusRetryableFailed, eventport.EvOutboundFailed},
		{"invalid_argument", outboundapp.TaskStatusFinalFailed, eventport.EvOutboundFailed},
		{"timeout", outboundapp.TaskStatusOutcomeUnknown, eventport.EvOutboundFailed},
	} {
		enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), fmt.Sprintf("outbound-o5-result-%02d", index), test.outcome)
		command := outboundapp.SendCommand{RiverJobID: enqueued.RiverJobID, TaskID: enqueued.TaskID, JobKind: outboundapp.OutboundEnqueueOneJobKind}
		attempt, err := sender.Execute(ctx, command)
		if err != nil {
			t.Fatalf("%s Execute(): %v", test.outcome, err)
		}

		var status string
		var attemptCount int
		var currentAttemptID int64
		var failureKind, lastError, messageID *string
		var sentAt *time.Time
		if err = pool.QueryRow(ctx, `
SELECT status, attempt_count, current_attempt_id, last_failure_kind, last_error, provider_message_id, sent_at
FROM outbound_tasks WHERE id=$1`, enqueued.TaskID).Scan(
			&status, &attemptCount, &currentAttemptID, &failureKind, &lastError, &messageID, &sentAt,
		); err != nil {
			t.Fatal(err)
		}
		if status != string(test.status) || attemptCount != 1 || currentAttemptID != attempt.ID {
			t.Fatalf("%s task status/count/attempt=%s/%d/%d, want %s/1/%d", test.outcome, status, attemptCount, currentAttemptID, test.status, attempt.ID)
		}
		if test.status == outboundapp.TaskStatusSent {
			if messageID == nil || *messageID == "" || sentAt == nil || failureKind != nil || lastError != nil {
				t.Fatalf("sent projection message/sent/failure/error=%v/%v/%v/%v", messageID, sentAt, failureKind, lastError)
			}
		} else if failureKind == nil || *failureKind == "" || lastError == nil || *lastError == "" || messageID != nil || sentAt != nil {
			t.Fatalf("failure projection kind/error/message/sent=%v/%v/%v/%v", failureKind, lastError, messageID, sentAt)
		}

		var eventID int64
		var eventType, eventKey string
		var payload []byte
		var occurredAt time.Time
		if err = pool.QueryRow(ctx, `
SELECT id, event_type, payload, occurred_at, idempotency_key
FROM event_log WHERE idempotency_key=$1`, fmt.Sprintf("outbound.send-result:%d", attempt.ID)).Scan(
			&eventID, &eventType, &payload, &occurredAt, &eventKey,
		); err != nil {
			t.Fatal(err)
		}
		var eventPayload struct {
			TaskID       int64  `json:"task_id"`
			AttemptID    int64  `json:"attempt_id"`
			Status       string `json:"status"`
			AttemptCount int    `json:"attempt_count"`
		}
		if err = json.Unmarshal(payload, &eventPayload); err != nil || eventType != test.eventType || eventPayload.TaskID != int64(enqueued.TaskID) || eventPayload.AttemptID != attempt.ID || eventPayload.Status != string(test.status) || eventPayload.AttemptCount != 1 || !occurredAt.Equal(attempt.CompletedAt) {
			t.Fatalf("%s event id/type/key/payload/time=%d/%s/%s/%s/%s err=%v", test.outcome, eventID, eventType, eventKey, payload, occurredAt, err)
		}

		var jobsBefore, jobsAfter, eventsAfter int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job`).Scan(&jobsBefore); err != nil {
			t.Fatal(err)
		}
		const replays = 2
		var wait sync.WaitGroup
		errorsSeen := make(chan error, replays)
		for range replays {
			wait.Add(1)
			go func() {
				defer wait.Done()
				_, replayErr := sender.Execute(ctx, command)
				errorsSeen <- replayErr
			}()
		}
		wait.Wait()
		close(errorsSeen)
		for replayErr := range errorsSeen {
			if replayErr != nil {
				t.Fatalf("%s concurrent replay: %v", test.outcome, replayErr)
			}
		}
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job`).Scan(&jobsAfter); err != nil || jobsAfter != jobsBefore {
			t.Fatalf("%s replay River jobs=%d err=%v, want unchanged %d", test.outcome, jobsAfter, err, jobsBefore)
		}
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE idempotency_key=$1`, eventKey).Scan(&eventsAfter); err != nil || eventsAfter != 1 {
			t.Fatalf("%s replay result events=%d err=%v, want 1", test.outcome, eventsAfter, err)
		}
		if calls := provider.Calls(enqueued.TaskID); calls != 1 {
			t.Fatalf("%s provider calls=%d, want 1", test.outcome, calls)
		}
	}
}

func TestSenderResultEventRollbackIsAtomicAndReplayStaysUnknown(t *testing.T) {
	pool := openOutboundPool(t)
	ctx := context.Background()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundFixture(t, pool)
	enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o5-event-rollback", "success")
	command := outboundapp.SendCommand{RiverJobID: enqueued.RiverJobID, TaskID: enqueued.TaskID, JobKind: outboundapp.OutboundEnqueueOneJobKind}
	provider := &fixtureProvider{}
	first := outboundapp.NewSenderService(
		platformstore.NewUnitOfWork(pool), outboundstore.NewSenderRepository(), o5RollbackAppender{}, provider, fixtureRateGate{},
	)
	if _, err := first.Execute(ctx, command); !errors.Is(err, errO5EventRollback) {
		t.Fatalf("first Execute() error=%v, want %v", err, errO5EventRollback)
	}
	var receiptState, taskStatus string
	var resultEvents int
	if err := pool.QueryRow(ctx, `
SELECT attempt.state, task.status,
  (SELECT count(*) FROM event_log WHERE idempotency_key='outbound.send-result:' || attempt.id::text)
FROM outbound_send_attempts AS attempt
JOIN outbound_tasks AS task ON task.id=attempt.task_id
WHERE attempt.river_job_id=$1`, enqueued.RiverJobID).Scan(&receiptState, &taskStatus, &resultEvents); err != nil || receiptState != "dispatching" || taskStatus != "sending" || resultEvents != 0 {
		t.Fatalf("rollback receipt/task/events=%s/%s/%d err=%v, want dispatching/sending/0", receiptState, taskStatus, resultEvents, err)
	}

	replay := outboundapp.NewSenderService(
		platformstore.NewUnitOfWork(pool), outboundstore.NewSenderRepository(), eventstore.NewAppender(), provider, fixtureRateGate{},
	)
	got, err := replay.Execute(ctx, command)
	if err != nil || got.State != outboundapp.SendAttemptOutcomeUnknown || got.FailureKind != outboundapp.ProviderFailureInterruptedDispatch {
		t.Fatalf("replay Execute()=%+v err=%v", got, err)
	}
	if err = pool.QueryRow(ctx, `
SELECT task.status, count(event.id)
FROM outbound_tasks AS task
LEFT JOIN event_log AS event ON event.idempotency_key='outbound.send-result:' || task.current_attempt_id::text
WHERE task.id=$1 GROUP BY task.status`, enqueued.TaskID).Scan(&taskStatus, &resultEvents); err != nil || taskStatus != string(outboundapp.TaskStatusOutcomeUnknown) || resultEvents != 1 {
		t.Fatalf("replay task/events=%s/%d err=%v, want outcome_unknown/1", taskStatus, resultEvents, err)
	}
	if calls := provider.Calls(enqueued.TaskID); calls != 1 {
		t.Fatalf("provider calls=%d, want exactly 1", calls)
	}
}

var errO5EventRollback = errors.New("fixture O5 event rollback")

type o5RollbackAppender struct{}

func (o5RollbackAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errO5EventRollback
}
