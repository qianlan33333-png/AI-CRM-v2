package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerEventRepositoryRejectsInvalidQueriesBeforeDatabaseAccess(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	afterID := int64(3)
	invalidOwner := int64(0)
	invalidAfterID := int64(0)
	for _, query := range []contactapp.CustomerEventQuery{
		{},
		{CustomerID: 42, Limit: 0},
		{CustomerID: 42, Limit: contactapp.CustomerListMaximumLimit + 1},
		{CustomerID: 42, Limit: 1, OwnerStaffID: &invalidOwner},
		{CustomerID: 42, Limit: 1, AfterOccurredAt: &at},
		{CustomerID: 42, Limit: 1, AfterID: &afterID},
		{CustomerID: 42, Limit: 1, AfterOccurredAt: &time.Time{}, AfterID: &afterID},
		{CustomerID: 42, Limit: 1, AfterOccurredAt: &at, AfterID: &invalidAfterID},
	} {
		_, err := NewCustomerEventRepository().ListCustomerEvents(context.Background(), query)
		if !errors.Is(err, contactapp.ErrInvalidCustomerEventQuery) {
			t.Fatalf("ListCustomerEvents(%#v) error = %v, want invalid query", query, err)
		}
	}
}

func TestCustomerEventRepositoryRequiresTransactionBoundContext(t *testing.T) {
	query := validCustomerEventQuery()
	if _, err := NewCustomerEventRepository().ListCustomerEvents(context.Background(), query); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("background context error = %v, want transaction requirement", err)
	}
	var typedNilContext *typedNilCustomerEventContext
	if _, err := NewCustomerEventRepository().ListCustomerEvents(typedNilContext, query); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("typed nil context error = %v, want transaction requirement", err)
	}
}

func TestCustomerEventRepositoryListsTimelineWithOwnerScopedKeyset(t *testing.T) {
	local := time.FixedZone("UTC+8", 8*60*60)
	occurredAt := time.Date(2026, time.August, 12, 10, 0, 0, 0, local)
	afterOccurredAt := occurredAt.Add(-time.Minute)
	afterID := int64(12)
	ownerStaffID := int64(8)
	payload := []byte(`{"source":"test"}`)
	tx := &customerEventTx{rows: []contactdb.ListCustomerEventsRow{
		customerEventRow(42, 15, occurredAt, payload),
		customerEventRow(84, 14, occurredAt.Add(-time.Minute), []byte(`{"source":"older"}`)),
		customerEventRow(42, 13, occurredAt.Add(-2*time.Minute), []byte(`{"source":"more"}`)),
	}}
	uow := platformstore.NewUnitOfWork(&customerEventBeginner{tx: tx})
	query := contactapp.CustomerEventQuery{
		CustomerID: 42, OwnerStaffID: &ownerStaffID, AfterOccurredAt: &afterOccurredAt, AfterID: &afterID, Limit: 2,
	}

	var result contactapp.CustomerEventStoreResult
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var readErr error
		result, readErr = NewCustomerEventRepository().ListCustomerEvents(txCtx, query)
		return readErr
	})
	if err != nil {
		t.Fatalf("ListCustomerEvents() error = %v", err)
	}
	if !result.HasMore || len(result.Items) != 2 || result.Items[0].ID != 15 || result.Items[1].ID != 14 {
		t.Fatalf("result = %#v, want first two events and HasMore", result)
	}
	if result.Items[0].CustomerID != 42 || result.Items[1].CustomerID != 84 {
		t.Fatalf("event customer ids = %d/%d, want original root/descendant ids", result.Items[0].CustomerID, result.Items[1].CustomerID)
	}
	if result.Items[0].OccurredAt.Location() != time.UTC || !result.Items[0].OccurredAt.Equal(occurredAt) {
		t.Fatalf("OccurredAt = %s (%s), want UTC representation of %s", result.Items[0].OccurredAt, result.Items[0].OccurredAt.Location(), occurredAt)
	}
	payload[2] = 'X'
	if string(result.Items[0].Payload) != `{"source":"test"}` {
		t.Fatalf("Payload = %s, want independent JSON copy", result.Items[0].Payload)
	}
	if tx.commits != 1 || tx.rollbacks != 0 || tx.queryCalls != 1 {
		t.Fatalf("transaction/query calls = %d/%d/%d, want 1/0/1", tx.commits, tx.rollbacks, tx.queryCalls)
	}
	if len(tx.queryArgs) != 5 || tx.queryArgs[2].(int32) != 3 || tx.queryArgs[3].(int64) != 42 {
		t.Fatalf("query arguments = %#v, want limit+1 and customer id", tx.queryArgs)
	}
	if after := tx.queryArgs[0].(pgtype.Timestamptz); !after.Valid || after.Time.Location() != time.UTC || !after.Time.Equal(afterOccurredAt) {
		t.Fatalf("after timestamp = %#v, want UTC cursor position", after)
	}
	if after := tx.queryArgs[1].(pgtype.Int8); !after.Valid || after.Int64 != afterID {
		t.Fatalf("after id = %#v", after)
	}
	if owner := tx.queryArgs[4].(pgtype.Int8); !owner.Valid || owner.Int64 != ownerStaffID {
		t.Fatalf("owner argument = %#v", owner)
	}
	for _, required := range []string{
		"-- name: ListCustomerEvents :many",
		"WITH RECURSIVE root_customer AS",
		"FROM customer_merge_lineage AS lineage",
		"JOIN lineage_ids AS parent",
		"LEFT JOIN LATERAL",
		"CROSS JOIN LATERAL",
		"AND NOT c.is_deleted",
		"c.owner_staff_id = $5::bigint",
		"AND (ce.occurred_at, ce.id) <",
		"ORDER BY ce.occurred_at DESC, ce.id DESC",
		"ORDER BY candidate.occurred_at DESC, candidate.id DESC",
		"LIMIT $3::integer",
	} {
		if !strings.Contains(tx.statement, required) {
			t.Fatalf("timeline SQL missing %q:\n%s", required, tx.statement)
		}
	}
	upperSQL := strings.ToUpper(tx.statement)
	if strings.Contains(upperSQL, "OFFSET") || strings.Contains(upperSQL, "COUNT(") {
		t.Fatalf("timeline SQL has forbidden pagination/count shape:\n%s", tx.statement)
	}
}

func TestCustomerEventRepositoryReturnsNonNilEmptyItemsForExistingCustomerWithoutEvents(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	tx := &customerEventTx{rows: []contactdb.ListCustomerEventsRow{{
		CustomerID: 42, EventID: 0, Payload: []byte(`{}`), OccurredAt: pgtype.Timestamptz{Time: at, Valid: true},
	}}}
	uow := platformstore.NewUnitOfWork(&customerEventBeginner{tx: tx})

	var result contactapp.CustomerEventStoreResult
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var readErr error
		result, readErr = NewCustomerEventRepository().ListCustomerEvents(txCtx, validCustomerEventQuery())
		return readErr
	})
	if err != nil || result.Items == nil || len(result.Items) != 0 || result.HasMore {
		t.Fatalf("result/error = %#v / %v, want non-nil empty items", result, err)
	}
}

func TestCustomerEventRepositoryMapsMissingCustomerAndOwnerMismatchToNotFound(t *testing.T) {
	ownerStaffID := int64(8)
	for _, query := range []contactapp.CustomerEventQuery{
		{CustomerID: 42, Limit: 1},
		{CustomerID: 42, OwnerStaffID: &ownerStaffID, Limit: 1},
	} {
		tx := &customerEventTx{}
		uow := platformstore.NewUnitOfWork(&customerEventBeginner{tx: tx})
		err := uow.Within(context.Background(), func(txCtx context.Context) error {
			_, readErr := NewCustomerEventRepository().ListCustomerEvents(txCtx, query)
			return readErr
		})
		if !errors.Is(err, contactapp.ErrCustomerNotFound) || tx.queryCalls != 1 || tx.commits != 0 || tx.rollbacks != 1 {
			t.Fatalf("missing/owner result = %v; calls commit rollback = %d %d %d", err, tx.queryCalls, tx.commits, tx.rollbacks)
		}
	}
}

func TestCustomerEventRepositoryFailsClosedOnMalformedRowsAndDatabaseErrors(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		tx   *customerEventTx
		want error
	}{
		{name: "zero event customer", tx: &customerEventTx{rows: []contactdb.ListCustomerEventsRow{customerEventRow(0, 15, at, []byte(`{}`))}}},
		{name: "mixed sentinel row", tx: &customerEventTx{rows: []contactdb.ListCustomerEventsRow{customerEventRow(42, 15, at, []byte(`{}`)), {CustomerID: 42}}}},
		{name: "database error", tx: &customerEventTx{queryErr: errors.New("database unavailable")}},
		{name: "no rows error", tx: &customerEventTx{queryErr: pgx.ErrNoRows}, want: contactapp.ErrCustomerNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			uow := platformstore.NewUnitOfWork(&customerEventBeginner{tx: test.tx})
			err := uow.Within(context.Background(), func(txCtx context.Context) error {
				_, readErr := NewCustomerEventRepository().ListCustomerEvents(txCtx, validCustomerEventQuery())
				return readErr
			})
			if err == nil || test.tx.commits != 0 || test.tx.rollbacks != 1 {
				t.Fatalf("malformed/database error = %v; commit/rollback = %d/%d", err, test.tx.commits, test.tx.rollbacks)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func validCustomerEventQuery() contactapp.CustomerEventQuery {
	return contactapp.CustomerEventQuery{CustomerID: contactport.CustomerID(42), Limit: contactapp.CustomerListDefaultLimit}
}

func customerEventRow(customerID, eventID int64, occurredAt time.Time, payload []byte) contactdb.ListCustomerEventsRow {
	return contactdb.ListCustomerEventsRow{
		CustomerID: customerID,
		EventID:    eventID,
		EventType:  "customer.updated",
		Payload:    payload,
		Actor:      "operator",
		OccurredAt: pgtype.Timestamptz{Time: occurredAt, Valid: true},
	}
}

type typedNilCustomerEventContext struct{}

func (*typedNilCustomerEventContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*typedNilCustomerEventContext) Done() <-chan struct{}       { return nil }
func (*typedNilCustomerEventContext) Err() error                  { return nil }
func (*typedNilCustomerEventContext) Value(any) any               { return nil }

type customerEventBeginner struct{ tx *customerEventTx }

func (beginner *customerEventBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return beginner.tx, nil
}

type customerEventTx struct {
	rows       []contactdb.ListCustomerEventsRow
	queryErr   error
	statement  string
	queryArgs  []any
	queryCalls int
	commits    int
	rollbacks  int
}

func (*customerEventTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}
func (tx *customerEventTx) Commit(context.Context) error {
	tx.commits++
	return nil
}
func (tx *customerEventTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}
func (*customerEventTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}
func (*customerEventTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (*customerEventTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (*customerEventTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}
func (*customerEventTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}
func (tx *customerEventTx) Query(_ context.Context, statement string, args ...any) (pgx.Rows, error) {
	tx.queryCalls++
	tx.statement = statement
	tx.queryArgs = append([]any(nil), args...)
	if tx.queryErr != nil {
		return nil, tx.queryErr
	}
	return &customerEventRows{rows: tx.rows}, nil
}
func (*customerEventTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (*customerEventTx) Conn() *pgx.Conn                                  { return nil }

type customerEventRows struct {
	rows    []contactdb.ListCustomerEventsRow
	index   int
	current contactdb.ListCustomerEventsRow
}

func (*customerEventRows) Close()                        {}
func (*customerEventRows) Err() error                    { return nil }
func (*customerEventRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (*customerEventRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}
func (rows *customerEventRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.current = rows.rows[rows.index]
	rows.index++
	return true
}
func (rows *customerEventRows) Scan(dest ...any) error {
	if len(dest) != 6 {
		return errors.New("unexpected event scan destination count")
	}
	*dest[0].(*int64) = rows.current.CustomerID
	*dest[1].(*int64) = rows.current.EventID
	*dest[2].(*string) = rows.current.EventType
	*dest[3].(*[]byte) = append([]byte(nil), rows.current.Payload...)
	*dest[4].(*string) = rows.current.Actor
	*dest[5].(*pgtype.Timestamptz) = rows.current.OccurredAt
	return nil
}
func (*customerEventRows) Values() ([]any, error) { return nil, errors.New("not implemented") }
func (*customerEventRows) RawValues() [][]byte    { return nil }
func (*customerEventRows) Conn() *pgx.Conn        { return nil }
