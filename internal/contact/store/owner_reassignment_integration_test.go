package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestContactOwnerReassignmentLocalCorePG16(t *testing.T) {
	pool, ctx := openAcceptancePool(t)
	service := newService(pool, eventstore.NewAppender())

	t.Run("preview idempotency", func(t *testing.T) {
		facts := seedFacts(t, ctx, pool, true)
		first := createPreview(t, service, 101, facts, "preview-replay-key")
		replay, err := service.CreatePreview(ctx, 101, facts.csv(facts.target), "preview-replay-key")
		if err != nil || replay.ID != first.ID || replay.Hash != first.Hash {
			t.Fatalf("preview replay=%+v first=%+v err=%v", replay, first, err)
		}
		if _, err = service.CreatePreview(ctx, 101, facts.csv(facts.target+1), "preview-replay-key"); !errors.Is(err, contactapp.ErrOwnerReassignmentConflict) {
			t.Fatalf("different preview payload error=%v", err)
		}
	})

	t.Run("execute commits all local facts and replays", func(t *testing.T) {
		facts := seedFacts(t, ctx, pool, true)
		preview := createPreview(t, service, 102, facts, "preview-success-key")
		result, err := service.Execute(ctx, 102, preview.ID, preview.Hash, contactapp.OwnerReassignmentConfirmation, "execute-success-key")
		if err != nil || !result.Executed || len(result.Result) != 1 || result.Result[0].PreviousOwnerStaffID != facts.owner || result.Result[0].TargetOwnerStaffID != facts.target {
			t.Fatalf("execute result=%+v err=%v", result, err)
		}
		assertCommittedFacts(t, ctx, pool, 102, facts, preview.ID)
		replay, err := service.Execute(ctx, 102, preview.ID, preview.Hash, contactapp.OwnerReassignmentConfirmation, "execute-success-key")
		if err != nil || !replay.Executed || len(replay.Result) != 1 || replay.ID != preview.ID || replay.ExpiresAt.IsZero() {
			t.Fatalf("execute replay=%+v err=%v", replay, err)
		}
		assertCounts(t, ctx, pool, facts.customer, 1, 1)
	})

	t.Run("concurrent execute has one commit and idempotent replay", func(t *testing.T) {
		facts := seedFacts(t, ctx, pool, true)
		preview := createPreview(t, service, 103, facts, "preview-concurrent-key")
		start := make(chan struct{})
		errs := make(chan error, 2)
		var group sync.WaitGroup
		for range 2 {
			group.Add(1)
			go func() {
				defer group.Done()
				<-start
				_, err := service.Execute(context.Background(), 103, preview.ID, preview.Hash, contactapp.OwnerReassignmentConfirmation, "execute-concurrent-key")
				errs <- err
			}()
		}
		close(start)
		group.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent execute error=%v", err)
			}
		}
		assertCommittedFacts(t, ctx, pool, 103, facts, preview.ID)
		assertCounts(t, ctx, pool, facts.customer, 1, 1)
	})

	t.Run("same customer lock and CAS reject reassignment after competing mutation", func(t *testing.T) {
		facts := seedFacts(t, ctx, pool, true)
		preview := createPreview(t, service, 104, facts, "preview-cas-key")
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM public.customers WHERE id=$1 FOR UPDATE`, facts.customer); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			_, executeErr := service.Execute(context.Background(), 104, preview.ID, preview.Hash, contactapp.OwnerReassignmentConfirmation, "execute-cas-key")
			done <- executeErr
		}()
		time.Sleep(50 * time.Millisecond)
		if _, err = tx.Exec(ctx, `UPDATE public.customers SET owner_staff_id=$2,updated_at=now() WHERE id=$1`, facts.customer, facts.competitor); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-done; !errors.Is(err, contactapp.ErrOwnerReassignmentConflict) {
			t.Fatalf("execute after competing customer mutation error=%v", err)
		}
		assertOwner(t, ctx, pool, facts.customer, facts.competitor)
		assertCounts(t, ctx, pool, facts.customer, 0, 0)
		assertUncommitted(t, ctx, pool, 104, preview.ID)
	})

	t.Run("inactive target conflicts without facts", func(t *testing.T) {
		facts := seedFacts(t, ctx, pool, false)
		preview := createPreview(t, service, 105, facts, "preview-inactive-key")
		if _, err := service.Execute(ctx, 105, preview.ID, preview.Hash, contactapp.OwnerReassignmentConfirmation, "execute-inactive-key"); !errors.Is(err, contactapp.ErrOwnerReassignmentConflict) {
			t.Fatalf("inactive target error=%v", err)
		}
		assertOwner(t, ctx, pool, facts.customer, facts.owner)
		assertCounts(t, ctx, pool, facts.customer, 0, 0)
		assertUncommitted(t, ctx, pool, 105, preview.ID)
	})

	t.Run("event failure rolls the whole transaction back", func(t *testing.T) {
		facts := seedFacts(t, ctx, pool, true)
		preview := createPreview(t, service, 106, facts, "preview-rollback-key")
		failing := newService(pool, failingAppender{})
		if _, err := failing.Execute(ctx, 106, preview.ID, preview.Hash, contactapp.OwnerReassignmentConfirmation, "execute-rollback-key"); err == nil {
			t.Fatal("failing event append unexpectedly committed")
		}
		assertOwner(t, ctx, pool, facts.customer, facts.owner)
		assertCounts(t, ctx, pool, facts.customer, 0, 0)
		assertUncommitted(t, ctx, pool, 106, preview.ID)
	})
}

type facts struct {
	customer, owner, target, competitor int64
	updatedAt                           time.Time
}

func (f facts) csv(target int64) []byte {
	return []byte(fmt.Sprintf("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n%d,%d,%s,%d\n", f.customer, f.owner, f.updatedAt.UTC().Format(time.RFC3339Nano), target))
}

func openAcceptancePool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("P4CONTACT_OWNER_REASSIGNMENT_ACCEPTANCE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("P4CONTACT_OWNER_REASSIGNMENT_ACCEPTANCE_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func newService(pool *pgxpool.Pool, events eventport.Appender) *contactapp.OwnerReassignmentService {
	return contactapp.NewOwnerReassignmentService(platformstore.NewUnitOfWork(pool), contactstore.NewOwnerReassignmentRepository(), events)
}

func seedFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetActive bool) facts {
	t.Helper()
	marker := time.Now().UnixNano()
	var f facts
	err := pool.QueryRow(ctx, `
WITH inserted_staff AS (
  INSERT INTO public.staff(wecom_userid,name,is_active) VALUES
    ($1,$1,TRUE),($2,$2,$3),($4,$4,TRUE)
  RETURNING id,wecom_userid
), inserted_customer AS (
  INSERT INTO public.customers(name,owner_staff_id)
  SELECT $5,id FROM inserted_staff WHERE wecom_userid=$1
  RETURNING id,updated_at
)
SELECT
  (SELECT id FROM inserted_customer),
  (SELECT updated_at FROM inserted_customer),
  (SELECT id FROM inserted_staff WHERE wecom_userid=$1),
  (SELECT id FROM inserted_staff WHERE wecom_userid=$2),
  (SELECT id FROM inserted_staff WHERE wecom_userid=$4)
`, fmt.Sprintf("or-owner-%d", marker), fmt.Sprintf("or-target-%d", marker), targetActive, fmt.Sprintf("or-competitor-%d", marker), fmt.Sprintf("or-customer-%d", marker)).Scan(&f.customer, &f.updatedAt, &f.owner, &f.target, &f.competitor)
	if err != nil {
		t.Fatalf("seed owner reassignment facts: %v", err)
	}
	f.updatedAt = f.updatedAt.UTC().Truncate(time.Microsecond)
	return f
}

func createPreview(t *testing.T, service *contactapp.OwnerReassignmentService, actor int64, f facts, key string) contactapp.OwnerReassignmentPreview {
	t.Helper()
	preview, err := service.CreatePreview(context.Background(), actor, f.csv(f.target), key)
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	return preview
}

func assertCommittedFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, actor int64, f facts, previewID string) {
	t.Helper()
	assertOwner(t, ctx, pool, f.customer, f.target)
	assertCounts(t, ctx, pool, f.customer, 1, 1)
	var executed, completed bool
	var previewResults, receiptResults int
	err := pool.QueryRow(ctx, `
SELECT p.executed_at IS NOT NULL, jsonb_array_length(p.result), r.state='completed', jsonb_array_length(r.result)
FROM public.contact_owner_reassignment_previews p
JOIN public.contact_owner_reassignment_operation_receipts r ON r.actor_id=p.actor_id
WHERE p.id=$1 AND p.actor_id=$2
`, previewID, actor).Scan(&executed, &previewResults, &completed, &receiptResults)
	if err != nil || !executed || !completed || previewResults != 1 || receiptResults != 1 {
		t.Fatalf("committed preview/receipt executed=%t preview_results=%d completed=%t receipt_results=%d err=%v", executed, previewResults, completed, receiptResults, err)
	}
}

func assertUncommitted(t *testing.T, ctx context.Context, pool *pgxpool.Pool, actor int64, previewID string) {
	t.Helper()
	var executed bool
	if err := pool.QueryRow(ctx, `SELECT executed_at IS NOT NULL FROM public.contact_owner_reassignment_previews WHERE id=$1 AND actor_id=$2`, previewID, actor).Scan(&executed); err != nil || executed {
		t.Fatalf("preview executed=%t err=%v", executed, err)
	}
	var receipts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.contact_owner_reassignment_operation_receipts WHERE actor_id=$1`, actor).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("receipt count=%d err=%v", receipts, err)
	}
}

func assertOwner(t *testing.T, ctx context.Context, pool *pgxpool.Pool, customerID, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, `SELECT owner_staff_id FROM public.customers WHERE id=$1`, customerID).Scan(&got); err != nil || got != want {
		t.Fatalf("owner=%d err=%v want=%d", got, err, want)
	}
}

func assertCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, customerID int64, wantCustomerEvents, wantEventLog int) {
	t.Helper()
	var customerEvents, eventLog int
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.customer_events WHERE customer_id=$1), (SELECT count(*) FROM public.event_log WHERE customer_id=$1)`, customerID).Scan(&customerEvents, &eventLog)
	if err != nil || customerEvents != wantCustomerEvents || eventLog != wantEventLog {
		t.Fatalf("event counts customer_events=%d event_log=%d err=%v", customerEvents, eventLog, err)
	}
}

type failingAppender struct{}

func (failingAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errors.New("injected event append failure")
}

var _ eventport.Appender = failingAppender{}
