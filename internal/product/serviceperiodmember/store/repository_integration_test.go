package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	memberapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/app"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

func TestRepositoryPostgreSQL16AtomicLifecycleIdempotencyAndRollback(t *testing.T) {
	databaseURL := os.Getenv("AICRM_SERVICE_PERIOD_MEMBER_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AICRM_SERVICE_PERIOD_MEMBER_TEST_DATABASE_URL is not set")
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
	assertDedicatedSchemaHasNoExternalIdentityColumns(t, ctx, pool)

	key := fmt.Sprintf("service-member-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	var productID, archivedProductID, ordinaryProductID, customerID int64
	insertProduct := func(code, projection string) int64 {
		var id int64
		err := pool.QueryRow(ctx, `INSERT INTO products
(product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at,legacy_admin_projection)
VALUES ($1,'service member integration','local only',0,'CNY',0,7001,$2,$2,$3::jsonb) RETURNING id`, code, now, projection).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	productID = insertProduct(key, `{"schema_version":1,"status":"service_period_enabled","enabled":true}`)
	archivedProductID = insertProduct(key+"-archived", `{"schema_version":1,"status":"service_period_archived","enabled":false}`)
	ordinaryProductID = insertProduct(key+"-ordinary", `{"schema_version":1}`)
	customerID, err = contactfixture.CreateCustomerWithDetails(ctx, pool, "service member OneID", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM products WHERE id=ANY($1::bigint[])`, []int64{productID, archivedProductID, ordinaryProductID})
		_ = contactfixture.DeleteCustomers(context.Background(), pool, []int64{customerID})
	})

	codec, err := memberapp.NewCursorCodec(bytes.Repeat([]byte("service-member-pg16-secret-"), 2))
	if err != nil {
		t.Fatal(err)
	}
	events := &memberIntegrationEvents{}
	service, err := memberapp.NewService(inlineUoW{}, NewRepository(), events, codec)
	if err != nil {
		t.Fatal(err)
	}
	uow := platformstore.NewUnitOfWork(pool)
	err = uow.Within(ctx, func(tx context.Context) error {
		db, txErr := platformstore.TxFromContext(tx)
		if txErr != nil {
			return txErr
		}
		command := memberport.AddCommand{ServiceProductID: productID, CustomerID: customerID, Source: memberdomain.SourceManual, ActorID: 7001, IdempotencyKey: key + "-add"}
		created, addErr := service.Add(tx, command)
		if addErr != nil {
			return fmt.Errorf("add: %w", addErr)
		}
		lockCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, lockErr := pool.Exec(lockCtx, `UPDATE products SET updated_at=updated_at WHERE id=$1`, productID)
		cancel()
		if !isLockTimeout(lockErr) {
			return fmt.Errorf("member add did not retain product share lock: %w", lockErr)
		}
		replayed, addErr := service.Add(tx, command)
		if addErr != nil || replayed.MemberRef != created.MemberRef {
			return fmt.Errorf("idempotent replay: %w", addErr)
		}
		if _, txErr = db.Exec(tx, "SAVEPOINT ordinary_isolation"); txErr != nil {
			return txErr
		}
		_, addErr = service.Add(tx, memberport.AddCommand{ServiceProductID: ordinaryProductID, CustomerID: customerID, Source: memberdomain.SourceManual, ActorID: 7001, IdempotencyKey: key + "-ordinary"})
		if _, txErr = db.Exec(tx, "ROLLBACK TO SAVEPOINT ordinary_isolation"); txErr != nil {
			return txErr
		}
		if !errors.Is(addErr, memberport.ErrNotFound) {
			return fmt.Errorf("ordinary product isolation: %w", addErr)
		}
		if _, txErr = db.Exec(tx, "SAVEPOINT archived_isolation"); txErr != nil {
			return txErr
		}
		_, addErr = service.Add(tx, memberport.AddCommand{ServiceProductID: archivedProductID, CustomerID: customerID, Source: memberdomain.SourceManual, ActorID: 7001, IdempotencyKey: key + "-archived"})
		if _, txErr = db.Exec(tx, "ROLLBACK TO SAVEPOINT archived_isolation"); txErr != nil {
			return txErr
		}
		if !errors.Is(addErr, memberport.ErrNotFound) {
			return fmt.Errorf("archived product add fail-closed: %w", addErr)
		}
		if _, txErr = db.Exec(tx, "SAVEPOINT bad_cas"); txErr != nil {
			return txErr
		}
		_, addErr = service.Expire(tx, memberport.TransitionCommand{ServiceProductID: productID, MemberRef: created.MemberRef, ExpectedVersion: 99, ActorID: 7001, IdempotencyKey: key + "-bad-cas"})
		if _, txErr = db.Exec(tx, "ROLLBACK TO SAVEPOINT bad_cas"); txErr != nil {
			return txErr
		}
		if !errors.Is(addErr, memberport.ErrConflict) {
			return fmt.Errorf("CAS: %w", addErr)
		}
		expired, addErr := service.Expire(tx, memberport.TransitionCommand{ServiceProductID: productID, MemberRef: created.MemberRef, ExpectedVersion: 1, ActorID: 7001, IdempotencyKey: key + "-expire"})
		if addErr != nil || expired.State != memberdomain.StateExpired || expired.Version != 2 {
			return fmt.Errorf("expire: %w", addErr)
		}
		remark, alliance := "renewed locally", "partner A"
		updated, addErr := service.UpdateFields(tx, memberport.UpdateFieldsCommand{ServiceProductID: productID, MemberRef: created.MemberRef, ExpectedVersion: 2, Remark: &remark, Alliance: &alliance, ActorID: 7001, IdempotencyKey: key + "-fields"})
		if addErr != nil || updated.Version != 3 {
			return fmt.Errorf("fields: %w", addErr)
		}
		removed, addErr := service.Remove(tx, memberport.TransitionCommand{ServiceProductID: productID, MemberRef: created.MemberRef, ExpectedVersion: 3, ActorID: 7001, IdempotencyKey: key + "-remove"})
		if addErr != nil || removed.State != memberdomain.StateRemoved || removed.Version != 4 {
			return fmt.Errorf("remove: %w", addErr)
		}
		var members, receipts int
		_ = db.QueryRow(tx, `SELECT count(*) FROM service_period_members WHERE service_product_id=$1`, productID).Scan(&members)
		_ = db.QueryRow(tx, `SELECT count(*) FROM service_period_member_operation_receipts WHERE actor_scope='service_period_members:actor:7001'`).Scan(&receipts)
		if members != 1 || receipts != 4 || events.count != 4 {
			return fmt.Errorf("facts=%d/%d/%d", members, receipts, events.count)
		}
		return errIntegrationRollback
	})
	if !errors.Is(err, errIntegrationRollback) {
		t.Fatalf("round trip=%v", err)
	}
	for table := range map[string]struct{}{`service_period_members`: {}, `service_period_member_operation_receipts`: {}} {
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback %s=%d err=%v", table, count, err)
		}
	}
	if err = uow.Within(ctx, func(tx context.Context) error {
		db, _ := platformstore.TxFromContext(tx)
		_, insertErr := db.Exec(tx, `INSERT INTO service_period_member_operation_receipts
(operation,actor_scope,key_digest,payload_digest,state,created_at) VALUES
('service_period_member.add','service_period_members:actor:7001',decode(repeat('00',32),'hex'),decode(repeat('01',32),'hex'),'reserved',now())`)
		return insertErr
	}); err == nil {
		t.Fatal("incomplete receipt unexpectedly committed")
	}
}

func assertDedicatedSchemaHasNoExternalIdentityColumns(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name='service_period_members' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err = rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"phone", "mobile", "unionid", "openid", "external"} {
			if contains(column, forbidden) {
				t.Fatalf("forbidden service member column=%q", column)
			}
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

func isLockTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "57014"
}

type inlineUoW struct{}

func (inlineUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type memberIntegrationEvents struct{ count int }

func (events *memberIntegrationEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	events.count++
	return eventport.EventID(events.count), nil
}

var errIntegrationRollback = errors.New("rollback service member integration")
