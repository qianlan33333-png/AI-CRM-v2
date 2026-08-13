package outbound_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	outboundworker "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/worker"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestSenderConsumesEnqueueOneAndBatchJobsWithFixtureProvider(t *testing.T) {
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundFixture(t, pool)

	provider := &fixtureProvider{}
	repository := outboundstore.NewSenderRepository()
	sender := outboundapp.NewSenderService(platformstore.NewUnitOfWork(pool), repository, provider, fixtureRateGate{})
	oneWorker, err := outboundworker.NewEnqueueOneSender(sender)
	if err != nil {
		t.Fatal(err)
	}
	batchWorker, err := outboundworker.NewEnqueueBatchTaskSender(sender)
	if err != nil {
		t.Fatal(err)
	}

	expectedByJob := make(map[int64]sendExpected)
	for index, outcome := range []struct {
		name    string
		state   outboundapp.SendAttemptState
		failure outboundapp.ProviderFailureKind
	}{
		{"success", outboundapp.SendAttemptSucceeded, ""},
		{"rate_limited", outboundapp.SendAttemptRetryableFailed, outboundapp.ProviderFailureRateLimited},
		{"invalid_argument", outboundapp.SendAttemptFinalFailed, outboundapp.ProviderFailureInvalidArgument},
		{"timeout", outboundapp.SendAttemptOutcomeUnknown, outboundapp.ProviderFailureTimeout},
	} {
		customerID := createOutboundCustomer(t, ctx, pool)
		enqueued := enqueueOneFixture(t, ctx, pool, customerID, fmt.Sprintf("outbound-o4-fixture-%02d", index), outcome.name)
		expectedByJob[enqueued.RiverJobID] = sendExpected{
			command: outboundapp.SendCommand{RiverJobID: enqueued.RiverJobID, TaskID: enqueued.TaskID, JobKind: outboundapp.OutboundEnqueueOneJobKind},
			state:   outcome.state, failure: outcome.failure,
		}
	}

	batch, err := newBatchService(t, pool).Enqueue(ctx, outboundapp.EnqueueBatchCommand{
		IdempotencyScope: "operator:7", IdempotencyKey: "outbound-o4-batch-fixture",
		Tier: outboundapp.BatchTierS, CustomerIDs: createOutboundCustomers(t, ctx, pool, 2),
		TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: json.RawMessage(`{"fixture_outcome":"temporary"}`),
	})
	if err != nil || batch.TaskCount != 2 {
		t.Fatalf("batch Enqueue()=%+v err=%v", batch, err)
	}
	rows, err := pool.Query(ctx, `SELECT id, (args->>'task_id')::bigint FROM river_job WHERE kind=$1 AND (args->>'batch_id')::bigint=$2`, outboundapp.OutboundEnqueueBatchJobKind, batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var jobID, taskID int64
		if err = rows.Scan(&jobID, &taskID); err != nil {
			t.Fatal(err)
		}
		expectedByJob[jobID] = sendExpected{
			command: outboundapp.SendCommand{RiverJobID: jobID, TaskID: outboundapp.TaskID(taskID), JobKind: outboundapp.OutboundEnqueueBatchJobKind},
			state:   outboundapp.SendAttemptRetryableFailed, failure: outboundapp.ProviderFailureTemporary,
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil || len(expectedByJob) != 6 {
		t.Fatalf("fixture jobs=%d rows error=%v, want 6", len(expectedByJob), err)
	}

	var eventsBefore int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	for jobID, want := range expectedByJob {
		var kind string
		var encoded []byte
		if err = pool.QueryRow(ctx, `SELECT kind, args FROM river_job WHERE id=$1 AND queue='outbound'`, jobID).Scan(&kind, &encoded); err != nil {
			t.Fatal(err)
		}
		switch kind {
		case outboundapp.OutboundEnqueueOneJobKind:
			var args outboundapp.EnqueueOneArgs
			if err = json.Unmarshal(encoded, &args); err != nil {
				t.Fatal(err)
			}
			if err = oneWorker.Work(ctx, &river.Job[outboundapp.EnqueueOneArgs]{JobRow: &rivertype.JobRow{ID: jobID}, Args: args}); err != nil {
				t.Fatalf("consume one job %d: %v", jobID, err)
			}
		case outboundapp.OutboundEnqueueBatchJobKind:
			var args outboundapp.EnqueueBatchTaskArgs
			if err = json.Unmarshal(encoded, &args); err != nil {
				t.Fatal(err)
			}
			if err = batchWorker.Work(ctx, &river.Job[outboundapp.EnqueueBatchTaskArgs]{JobRow: &rivertype.JobRow{ID: jobID}, Args: args}); err != nil {
				t.Fatalf("consume batch job %d: %v", jobID, err)
			}
		default:
			t.Fatalf("job %d kind=%q, want existing O2/O3 kind for %+v", jobID, kind, want.command)
		}
	}

	for jobID, want := range expectedByJob {
		var state string
		var failure, messageID *string
		if err = pool.QueryRow(ctx, `SELECT state, failure_kind, provider_message_id FROM outbound_send_attempts WHERE river_job_id=$1`, jobID).Scan(&state, &failure, &messageID); err != nil {
			t.Fatal(err)
		}
		if state != string(want.state) || nullableString(failure) != string(want.failure) {
			t.Fatalf("job %d receipt state/failure=%q/%q, want %q/%q", jobID, state, nullableString(failure), want.state, want.failure)
		}
		if want.state == outboundapp.SendAttemptSucceeded && (messageID == nil || *messageID == "") {
			t.Fatalf("job %d success has no fixture provider message", jobID)
		}
		if replay, replayErr := sender.Execute(ctx, want.command); replayErr != nil || replay.State != want.state {
			t.Fatalf("job %d replay=%+v err=%v", jobID, replay, replayErr)
		}
		if calls := provider.Calls(want.command.TaskID); calls != 1 {
			t.Fatalf("job %d provider calls=%d, want exactly 1", jobID, calls)
		}
	}
	var eventsAfter int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log`).Scan(&eventsAfter); err != nil || eventsAfter != eventsBefore {
		t.Fatalf("sender event count=%d err=%v, want unchanged %d because O5 is excluded", eventsAfter, err, eventsBefore)
	}
}

func TestSenderResultRollbackReplaysUnknownWithoutProviderDuplicate(t *testing.T) {
	pool := openOutboundPool(t)
	ctx := context.Background()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundFixture(t, pool)
	enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o4-rollback-fixture", "success")
	command := outboundapp.SendCommand{RiverJobID: enqueued.RiverJobID, TaskID: enqueued.TaskID, JobKind: outboundapp.OutboundEnqueueOneJobKind}
	provider := &fixtureProvider{}
	base := outboundstore.NewSenderRepository()
	lossy := &rollbackCompleteRepository{SendAttemptRepository: base}
	first := outboundapp.NewSenderService(platformstore.NewUnitOfWork(pool), lossy, provider, fixtureRateGate{})
	if _, err := first.Execute(ctx, command); !errors.Is(err, errFixtureResultRollback) {
		t.Fatalf("first Execute() error=%v, want %v", err, errFixtureResultRollback)
	}
	var state string
	var completedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT state, completed_at FROM outbound_send_attempts WHERE river_job_id=$1`, enqueued.RiverJobID).Scan(&state, &completedAt); err != nil || state != "dispatching" || completedAt != nil {
		t.Fatalf("rolled back receipt state/completed=%q/%v err=%v, want dispatching/nil", state, completedAt, err)
	}

	replay := outboundapp.NewSenderService(platformstore.NewUnitOfWork(pool), base, provider, fixtureRateGate{})
	got, err := replay.Execute(ctx, command)
	if err != nil || got.State != outboundapp.SendAttemptOutcomeUnknown || got.FailureKind != outboundapp.ProviderFailureInterruptedDispatch {
		t.Fatalf("replay Execute()=%+v err=%v", got, err)
	}
	if calls := provider.Calls(enqueued.TaskID); calls != 1 {
		t.Fatalf("provider calls=%d, want exactly 1 after result rollback", calls)
	}
}

func enqueueOneFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, customerID int64, key, outcome string) outboundapp.EnqueuedTask {
	t.Helper()
	repository, err := outboundstore.NewEnqueueOneRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := outboundapp.NewEnqueueOneService(platformstore.NewUnitOfWork(pool), outboundstore.NewRepository(), eventstore.NewAppender(), repository, repository)
	result, err := service.Enqueue(ctx, outboundapp.EnqueueOneCommand{
		OneCommand:       outboundapp.OneCommand{CustomerID: customerID, TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: json.RawMessage(fmt.Sprintf(`{"fixture_outcome":%q}`, outcome))},
		IdempotencyScope: "operator:7", IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type sendExpected struct {
	command outboundapp.SendCommand
	state   outboundapp.SendAttemptState
	failure outboundapp.ProviderFailureKind
}

type fixtureRateGate struct{}

func (fixtureRateGate) Wait(context.Context) error { return nil }

type fixtureProvider struct {
	mu    sync.Mutex
	calls map[outboundapp.TaskID]int
}

func (provider *fixtureProvider) Send(_ context.Context, request outboundapp.SendRequest) (outboundapp.ProviderResult, error) {
	provider.mu.Lock()
	if provider.calls == nil {
		provider.calls = make(map[outboundapp.TaskID]int)
	}
	provider.calls[request.TaskID]++
	provider.mu.Unlock()
	var payload struct {
		Outcome string `json:"fixture_outcome"`
	}
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return outboundapp.ProviderResult{}, err
	}
	switch payload.Outcome {
	case "success":
		return outboundapp.ProviderResult{MessageID: fmt.Sprintf("fixture-message-%d", request.TaskID)}, nil
	case "rate_limited":
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureRateLimited, Code: "fixture-429"}, nil
	case "invalid_argument":
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureInvalidArgument, Code: "fixture-400"}, nil
	case "timeout":
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureTimeout, Code: "fixture-timeout"}, nil
	case "temporary":
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureTemporary, Code: "fixture-temporary"}, nil
	default:
		return outboundapp.ProviderResult{}, errors.New("fixture provider received unknown outcome")
	}
}

func (provider *fixtureProvider) Calls(taskID outboundapp.TaskID) int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls[taskID]
}

var errFixtureResultRollback = errors.New("fixture result transaction rollback")

type rollbackCompleteRepository struct {
	outboundapp.SendAttemptRepository
	once sync.Once
}

func (repository *rollbackCompleteRepository) CompleteSendAttempt(ctx context.Context, command outboundapp.CompleteSendAttempt) (outboundapp.SendAttempt, error) {
	attempt, err := repository.SendAttemptRepository.CompleteSendAttempt(ctx, command)
	if err != nil {
		return outboundapp.SendAttempt{}, err
	}
	rolledBack := false
	repository.once.Do(func() { rolledBack = true })
	if rolledBack {
		return attempt, errFixtureResultRollback
	}
	return attempt, nil
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
