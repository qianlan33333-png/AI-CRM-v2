package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errContactPolicyIntegrationRollback = errors.New("rollback contact policy integration")

type inlineContactPolicyUoW struct{}

func (inlineContactPolicyUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type contactPolicyIntegrationEvents struct{ count int }

func (events *contactPolicyIntegrationEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	events.count++
	return eventport.EventID(events.count), nil
}

func TestContactPolicyRepositoryPostgreSQL16AtomicCASReceiptAndChecks(t *testing.T) {
	pool := contactPolicyIntegrationPool(t)
	ctx := context.Background()
	repository := NewContactPolicyRepository()
	uow := platformstore.NewUnitOfWork(pool)
	prefix := fmt.Sprintf("contact-policy-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	events := &contactPolicyIntegrationEvents{}

	err := uow.Within(ctx, func(txCtx context.Context) error {
		db, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		ids := make([]contactport.CustomerID, 4)
		for index := range ids {
			if txErr = db.QueryRow(txCtx, `INSERT INTO customers(name) VALUES($1) RETURNING id`, fmt.Sprintf("%s-%d", prefix, index)).Scan(&ids[index]); txErr != nil {
				return txErr
			}
		}
		service := contactapp.NewContactPolicyService(inlineContactPolicyUoW{}, repository, events)
		initial, getErr := service.Get(txCtx, ids[0])
		if getErr != nil || !initial.Eligible || initial.PolicyPresent || initial.Version != 0 {
			return fmt.Errorf("default eligible projection=%#v: %w", initial, getErr)
		}
		until := now.Add(time.Hour)
		setCommand := contactapp.SetContactPolicyCommand{
			CustomerID: ids[0], ExpectedVersion: 0, ReasonCode: contactapp.ContactPolicyReasonManualOptOut,
			SuppressedUntil: &until, ActorID: 701, IdempotencyKey: prefix + "-set-0000000000000001",
		}
		set, setErr := service.Set(txCtx, setCommand)
		if setErr != nil || set.Version != 1 || set.Eligible || !set.SuppressionActive {
			return fmt.Errorf("set=%#v: %w", set, setErr)
		}
		replay, replayErr := service.Set(txCtx, setCommand)
		if replayErr != nil || replay.Version != set.Version {
			return fmt.Errorf("replay=%#v: %w", replay, replayErr)
		}
		setCommand.ExpectedVersion = 1
		setCommand.ReasonCode = contactapp.ContactPolicyReasonCompliance
		setCommand.IdempotencyKey = prefix + "-update-0000000000001"
		updated, updateErr := service.Set(txCtx, setCommand)
		if updateErr != nil || updated.Version != 2 || updated.ReasonCode == nil || *updated.ReasonCode != contactapp.ContactPolicyReasonCompliance {
			return fmt.Errorf("update=%#v: %w", updated, updateErr)
		}
		cleared, clearErr := service.Clear(txCtx, contactapp.ClearContactPolicyCommand{
			CustomerID: ids[0], ExpectedVersion: 2, ActorID: 701,
			IdempotencyKey: prefix + "-clear-00000000000001",
		})
		if clearErr != nil || !cleared.Eligible || cleared.PolicyPresent || cleared.Version != 0 {
			return fmt.Errorf("clear=%#v: %w", cleared, clearErr)
		}
		expired := now.Add(-time.Minute)
		if _, txErr = db.Exec(txCtx, `INSERT INTO customer_contact_policies(customer_id,reason_code,suppressed_until,created_at,updated_at) VALUES($1,'operator_hold',$2,$3,$3)`, ids[1], expired, now); txErr != nil {
			return txErr
		}
		expiredProjection, getErr := service.Get(txCtx, ids[1])
		if getErr != nil || !expiredProjection.PolicyPresent || !expiredProjection.Eligible || expiredProjection.SuppressionActive {
			return fmt.Errorf("expired=%#v: %w", expiredProjection, getErr)
		}
		if _, txErr = db.Exec(txCtx, `INSERT INTO customer_contact_policies(customer_id,reason_code,created_at,updated_at) VALUES($1,'operator_hold',$2,$2)`, ids[2], now); txErr != nil {
			return txErr
		}
		if _, txErr = db.Exec(txCtx, `UPDATE customers SET is_deleted=TRUE, updated_at=$2 WHERE id=$1`, ids[3], now); txErr != nil {
			return txErr
		}
		missingID := contactport.CustomerID(int64(ids[3]) + 1000000)
		input := []contactport.CustomerID{ids[2], missingID, ids[0], ids[3], ids[1]}
		for _, checkpoint := range []contactport.ContactEligibilityCheckpoint{contactport.ContactEligibilityPreview, contactport.ContactEligibilityDispatch} {
			decisions, checkErr := repository.CheckContactEligibility(txCtx, contactport.ContactEligibilityCheck{
				Checkpoint: checkpoint, CustomerIDs: input, EvaluatedAt: now,
			})
			if checkErr != nil || len(decisions) != len(input) || !sort.SliceIsSorted(decisions, func(i, j int) bool { return decisions[i].CustomerID < decisions[j].CustomerID }) {
				return fmt.Errorf("%s decisions=%#v: %w", checkpoint, decisions, checkErr)
			}
			byID := make(map[contactport.CustomerID]contactport.ContactEligibility, len(decisions))
			for _, decision := range decisions {
				byID[decision.CustomerID] = decision
			}
			for _, eligibleID := range []contactport.CustomerID{ids[0], ids[1]} {
				decision := byID[eligibleID]
				if !decision.CustomerActive || !decision.Eligible || decision.Exclusion != contactport.ContactEligibilityExclusionNone {
					return fmt.Errorf("%s eligible decision=%#v", checkpoint, decision)
				}
			}
			suppressed := byID[ids[2]]
			if !suppressed.CustomerActive || suppressed.Eligible || suppressed.Exclusion != contactport.ContactEligibilityExclusionContactPolicy {
				return fmt.Errorf("%s suppressed decision=%#v", checkpoint, suppressed)
			}
			for _, inactiveID := range []contactport.CustomerID{ids[3], missingID} {
				decision := byID[inactiveID]
				if decision.CustomerActive || decision.Eligible || decision.Exclusion != contactport.ContactEligibilityExclusionInactiveCustomer {
					return fmt.Errorf("%s inactive decision=%#v", checkpoint, decision)
				}
			}
		}
		var policies, receipts int
		if txErr = db.QueryRow(txCtx, `SELECT count(*) FROM customer_contact_policies`).Scan(&policies); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(txCtx, `SELECT count(*) FROM customer_contact_policy_operation_receipts WHERE actor_scope='customer_contact_policy:actor:701' AND state='completed'`).Scan(&receipts); txErr != nil {
			return txErr
		}
		if policies != 2 || receipts != 3 || events.count != 3 {
			return fmt.Errorf("facts policies/receipts/events=%d/%d/%d", policies, receipts, events.count)
		}
		return errContactPolicyIntegrationRollback
	})
	if !errors.Is(err, errContactPolicyIntegrationRollback) {
		t.Fatalf("round trip: %v", err)
	}
	for _, table := range []string{"customer_contact_policies", "customer_contact_policy_operation_receipts"} {
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback %s=%d err=%v", table, count, err)
		}
	}

	err = uow.Within(ctx, func(txCtx context.Context) error {
		db, _ := platformstore.TxFromContext(txCtx)
		_, insertErr := db.Exec(txCtx, `INSERT INTO customer_contact_policy_operation_receipts(operation,actor_scope,key_digest,payload_digest,created_at) VALUES('customer_contact_policy.set','customer_contact_policy:actor:701',decode(repeat('01',32),'hex'),decode(repeat('02',32),'hex'),now())`)
		return insertErr
	})
	if err == nil {
		t.Fatal("incomplete receipt unexpectedly committed")
	}
}

func TestContactPolicyCheckerSerializesEmptyRowSetAndReplaySurvivesSoftDelete(t *testing.T) {
	pool := contactPolicyIntegrationPool(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("contact-policy-lock-%d", time.Now().UnixNano())
	var customerID contactport.CustomerID
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES($1) RETURNING id`, prefix).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	repository := NewContactPolicyRepository()
	locked := make(chan struct{})
	release := make(chan struct{})
	checkDone := make(chan error, 1)
	go func() {
		checkDone <- platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
			decisions, err := repository.CheckContactEligibility(txCtx, contactport.ContactEligibilityCheck{
				Checkpoint:  contactport.ContactEligibilityDispatch,
				CustomerIDs: []contactport.CustomerID{customerID}, EvaluatedAt: time.Now().UTC(),
			})
			if err != nil || len(decisions) != 1 || !decisions[0].CustomerActive || !decisions[0].Eligible || decisions[0].Exclusion != contactport.ContactEligibilityExclusionNone {
				return fmt.Errorf("initial empty-row check=%#v: %w", decisions, err)
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("checker did not acquire policy lock")
	}
	service := contactapp.NewContactPolicyService(platformstore.NewUnitOfWork(pool), repository, &contactPolicyIntegrationEvents{})
	command := contactapp.SetContactPolicyCommand{
		CustomerID: customerID, ExpectedVersion: 0, ReasonCode: contactapp.ContactPolicyReasonOperatorHold,
		ActorID: 702, IdempotencyKey: prefix + "-set-0000000000000001",
	}
	setDone := make(chan error, 1)
	go func() {
		_, err := service.Set(ctx, command)
		setDone <- err
	}()
	select {
	case err := <-setDone:
		t.Fatalf("set bypassed empty-row checker lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if err := <-checkDone; err != nil {
		t.Fatal(err)
	}
	if err := <-setDone; err != nil {
		t.Fatalf("set after checker release: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE customers SET is_deleted=TRUE, updated_at=now() WHERE id=$1`, customerID); err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Set(ctx, command)
	if err != nil || replayed.Version != 1 || replayed.Eligible {
		t.Fatalf("soft-deleted durable replay=%#v err=%v", replayed, err)
	}
}

func TestContactPolicyRepositoryCASAndIdempotencyConflictsRollback(t *testing.T) {
	pool := contactPolicyIntegrationPool(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("contact-policy-conflict-%d", time.Now().UnixNano())
	var customerID contactport.CustomerID
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES($1) RETURNING id`, prefix).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	service := contactapp.NewContactPolicyService(platformstore.NewUnitOfWork(pool), NewContactPolicyRepository(), &contactPolicyIntegrationEvents{})
	initial := contactapp.SetContactPolicyCommand{
		CustomerID: customerID, ExpectedVersion: 0, ReasonCode: contactapp.ContactPolicyReasonOperatorHold,
		ActorID: 703, IdempotencyKey: prefix + "-initial-0000000000001",
	}
	created, err := service.Set(ctx, initial)
	if err != nil || created.Version != 1 {
		t.Fatalf("initial=%#v err=%v", created, err)
	}
	stale := initial
	stale.IdempotencyKey = prefix + "-stale-000000000000001"
	if _, err = service.Set(ctx, stale); !errors.Is(err, contactapp.ErrContactPolicyConflict) {
		t.Fatalf("stale CAS error=%v", err)
	}
	mismatched := initial
	mismatched.ReasonCode = contactapp.ContactPolicyReasonCompliance
	if _, err = service.Set(ctx, mismatched); !errors.Is(err, contactapp.ErrContactPolicyConflict) {
		t.Fatalf("idempotency payload mismatch error=%v", err)
	}
	var version, completedReceipts, reservedReceipts int
	if err = pool.QueryRow(ctx, `SELECT version FROM customer_contact_policies WHERE customer_id=$1`, customerID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state='completed'), count(*) FILTER (WHERE state='reserved') FROM customer_contact_policy_operation_receipts WHERE actor_scope='customer_contact_policy:actor:703'`).Scan(&completedReceipts, &reservedReceipts); err != nil {
		t.Fatal(err)
	}
	if version != 1 || completedReceipts != 1 || reservedReceipts != 0 {
		t.Fatalf("post-conflict version/completed/reserved=%d/%d/%d", version, completedReceipts, reservedReceipts)
	}
}

func TestContactPolicyCheckerAcceptsFirstCampaignThousandAndRejectsMore(t *testing.T) {
	pool := contactPolicyIntegrationPool(t)
	ctx := context.Background()
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		db, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		rows, txErr := db.Query(txCtx, `INSERT INTO customers(name) SELECT 'contact-policy-1000-' || value::text FROM generate_series(1, $1) AS value RETURNING id`, contactport.ContactEligibilityMaximumCustomers)
		if txErr != nil {
			return txErr
		}
		ids := make([]contactport.CustomerID, 0, contactport.ContactEligibilityMaximumCustomers)
		for rows.Next() {
			var id contactport.CustomerID
			if txErr = rows.Scan(&id); txErr != nil {
				rows.Close()
				return txErr
			}
			ids = append(ids, id)
		}
		rows.Close()
		if txErr = rows.Err(); txErr != nil {
			return txErr
		}
		decisions, txErr := NewContactPolicyRepository().CheckContactEligibility(txCtx, contactport.ContactEligibilityCheck{
			Checkpoint: contactport.ContactEligibilityPreview, CustomerIDs: ids, EvaluatedAt: time.Now().UTC(),
		})
		if txErr != nil || len(decisions) != contactport.ContactEligibilityMaximumCustomers {
			return fmt.Errorf("1000 decisions=%d: %w", len(decisions), txErr)
		}
		_, txErr = NewContactPolicyRepository().CheckContactEligibility(txCtx, contactport.ContactEligibilityCheck{
			Checkpoint:  contactport.ContactEligibilityPreview,
			CustomerIDs: append(ids, contactport.CustomerID(ids[len(ids)-1]+1)), EvaluatedAt: time.Now().UTC(),
		})
		if !errors.Is(txErr, contactport.ErrInvalidContactEligibility) {
			return fmt.Errorf("1001 check error=%v", txErr)
		}
		return errContactPolicyIntegrationRollback
	})
	if !errors.Is(err, errContactPolicyIntegrationRollback) {
		t.Fatalf("1000 bound: %v", err)
	}
}

func contactPolicyIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("P4CONTACTPOLICY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("P4CONTACTPOLICY_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(context.Background(), "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	rows, err := pool.Query(context.Background(), `SELECT table_name,column_name FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('customer_contact_policies','customer_contact_policy_operation_receipts') ORDER BY table_name,ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, column string
		if err = rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"phone", "mobile", "unionid", "openid", "provider", "outbound", "delivery"} {
			if strings.Contains(column, forbidden) {
				t.Fatalf("forbidden %s column=%q", table, column)
			}
		}
	}
	return pool
}
