package store

import (
	"context"
	"errors"
	"os"
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

func TestCustomerQueryRepositoryRejectsInvalidQueriesBeforeDatabaseAccess(t *testing.T) {
	watermark := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	afterID := contactport.CustomerID(9)
	afterUpdatedAt := watermark.Add(-time.Minute)
	futureAfterUpdatedAt := watermark.Add(time.Minute)
	invalidOwner := int64(0)
	addedAfter := watermark
	addedBefore := watermark.Add(-time.Minute)

	for _, test := range []struct {
		name  string
		query contactapp.CustomerListQuery
	}{
		{name: "zero limit", query: contactapp.CustomerListQuery{Watermark: watermark}},
		{name: "over maximum limit", query: contactapp.CustomerListQuery{Watermark: watermark, Limit: contactapp.CustomerListMaximumLimit + 1}},
		{name: "zero watermark", query: contactapp.CustomerListQuery{Limit: 1}},
		{name: "after timestamp without id", query: contactapp.CustomerListQuery{Watermark: watermark, Limit: 1, AfterUpdatedAt: &afterUpdatedAt}},
		{name: "after id without timestamp", query: contactapp.CustomerListQuery{Watermark: watermark, Limit: 1, AfterID: &afterID}},
		{name: "after timestamp after watermark", query: contactapp.CustomerListQuery{Watermark: watermark, Limit: 1, AfterUpdatedAt: &futureAfterUpdatedAt, AfterID: &afterID}},
		{name: "nonpositive filter", query: contactapp.CustomerListQuery{Watermark: watermark, Limit: 1, OwnerStaffID: &invalidOwner}},
		{name: "inverted added range", query: contactapp.CustomerListQuery{Watermark: watermark, Limit: 1, AddedAfter: &addedAfter, AddedBefore: &addedBefore}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCustomerQueryRepository().ListCustomers(context.Background(), test.query)
			if !errors.Is(err, errInvalidCustomerListQuery) {
				t.Fatalf("ListCustomers() error = %v, want invalid query", err)
			}
		})
	}
}

func TestCustomerQueryRepositoryRequiresTransactionBoundContext(t *testing.T) {
	_, err := NewCustomerQueryRepository().ListCustomers(context.Background(), validCustomerListQuery())
	if !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("ListCustomers() error = %v, want transaction requirement", err)
	}
}

func TestCustomerListParamsUseNullForEmptyKeywordAndBoundedLimits(t *testing.T) {
	query := validCustomerListQuery()
	query.Limit = 2
	listParams := listCustomersParams(query)
	if listParams.Keyword.Valid {
		t.Fatal("empty keyword must become a NULL SQL parameter")
	}
	if !listParams.Watermark.Valid || listParams.RowLimit != query.Limit+1 {
		t.Fatalf("list params = %#v, want watermark and limit+1", listParams)
	}
	if listParams.AfterUpdatedAt.Valid || listParams.AfterID.Valid {
		t.Fatal("empty keyset must keep both SQL parameters NULL")
	}

	boundedParams := listCustomerIDsBoundedParams(query)
	if boundedParams.Keyword.Valid || boundedParams.TotalLimit != int32(contactapp.CustomerListExactTotalCap+1) {
		t.Fatalf("bounded params = %#v, want NULL keyword and cap+1", boundedParams)
	}
}

func TestCustomerQuerySQLUsesBoundedIDsWithoutCount(t *testing.T) {
	contents, err := os.ReadFile("queries/customers.sql")
	if err != nil {
		t.Fatalf("read customer queries: %v", err)
	}
	querySQL := string(contents)
	upper := strings.ToUpper(querySQL)
	if strings.Contains(upper, "COUNT") || strings.Contains(upper, "COUNT(*)") {
		t.Fatal("customer query SQL must not use COUNT or COUNT(*)")
	}
	boundedStart := strings.Index(querySQL, "-- name: ListCustomerIDsBounded :many")
	if boundedStart < 0 {
		t.Fatal("bounded customer-id query is missing")
	}
	boundedSQL := querySQL[boundedStart:]
	for _, required := range []string{
		"SELECT c.id",
		"c.updated_at <= sqlc.arg(watermark)::timestamptz",
		"ORDER BY c.updated_at DESC, c.id DESC",
		"LIMIT sqlc.arg(total_limit)::integer",
	} {
		if !strings.Contains(boundedSQL, required) {
			t.Fatalf("bounded customer-id query missing %q", required)
		}
	}
	for _, forbidden := range []string{"after_updated_at", "after_id"} {
		if strings.Contains(boundedSQL, forbidden) {
			t.Fatalf("bounded customer-id query must not apply cursor %q", forbidden)
		}
	}
}

func TestCustomerRecordFromRowCopiesExtra(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	extra := []byte(`{"source":"test"}`)
	row := contactdb.Customer{
		ID:             42,
		Name:           "Ada",
		AvatarUrl:      pgtype.Text{String: "https://example.test/avatar", Valid: true},
		Gender:         pgtype.Int2{Int16: 2, Valid: true},
		StageID:        pgtype.Int8{Int64: 3, Valid: true},
		OwnerStaffID:   pgtype.Int8{Int64: 4, Valid: true},
		ChannelID:      pgtype.Int8{Int64: 5, Valid: true},
		AddedAt:        pgtype.Timestamptz{Time: at.Add(-time.Hour), Valid: true},
		LastInteractAt: pgtype.Timestamptz{Time: at.Add(-time.Minute), Valid: true},
		Extra:          extra,
		CreatedAt:      pgtype.Timestamptz{Time: at.Add(-time.Hour), Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: at, Valid: true},
	}

	record := customerRecordFromRow(row)
	extra[2] = 'X'
	if string(record.Extra) != `{"source":"test"}` {
		t.Fatalf("Extra = %s, want an independent JSON copy", record.Extra)
	}
	if record.ID != 42 || record.AvatarURL == nil || *record.AvatarURL != row.AvatarUrl.String {
		t.Fatalf("record = %#v, want mapped customer values", record)
	}
	if record.Gender == nil || *record.Gender != 2 || record.StageID == nil || *record.StageID != 3 {
		t.Fatalf("record optional values = %#v, want mapped pointers", record)
	}
}

func TestCustomerQueryRepositoryTrimsLimitPlusOneAndUsesBoundedTotal(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	tx := &customerQueryTx{
		boundedIDs: make([]int64, int(contactapp.CustomerListExactTotalCap+1)),
		rows: []contactdb.Customer{
			customerRow(3, at, `{"position":1}`),
			customerRow(2, at.Add(-time.Minute), `{"position":2}`),
			customerRow(1, at.Add(-2*time.Minute), `{"position":3}`),
		},
	}
	uow := platformstore.NewUnitOfWork(&customerQueryBeginner{tx: tx})
	query := validCustomerListQuery()
	query.Limit = 2

	var result contactapp.CustomerListStoreResult
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var err error
		result, err = NewCustomerQueryRepository().ListCustomers(txCtx, query)
		return err
	})
	if err != nil {
		t.Fatalf("ListCustomers() error = %v", err)
	}
	if !result.HasMore || len(result.Items) != 2 || result.Items[0].ID != 3 || result.Items[1].ID != 2 {
		t.Fatalf("result = %#v, want trimmed first page and HasMore", result)
	}
	if result.BoundedTotal != contactapp.CustomerListExactTotalCap+1 {
		t.Fatalf("BoundedTotal = %d, want cap+1", result.BoundedTotal)
	}
	if tx.commits != 1 || tx.rollbacks != 0 {
		t.Fatalf("transaction calls = commit:%d rollback:%d, want 1/0", tx.commits, tx.rollbacks)
	}
	if got := tx.listArgs[13].(int32); got != query.Limit+1 {
		t.Fatalf("list row limit = %d, want %d", got, query.Limit+1)
	}
	if got := tx.boundedArgs[11].(int32); got != int32(contactapp.CustomerListExactTotalCap+1) {
		t.Fatalf("bounded id limit = %d, want cap+1", got)
	}
	if keyword := tx.listArgs[1].(pgtype.Text); keyword.Valid {
		t.Fatal("empty keyword was not passed as NULL")
	}
	tx.rows[0].Extra[2] = 'X'
	if string(result.Items[0].Extra) != `{"position":1}` {
		t.Fatalf("item Extra = %s, want an independent JSON copy", result.Items[0].Extra)
	}
}

func validCustomerListQuery() contactapp.CustomerListQuery {
	return contactapp.CustomerListQuery{
		Limit:     contactapp.CustomerListDefaultLimit,
		Watermark: time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC),
	}
}

func customerRow(id int64, updatedAt time.Time, extra string) contactdb.Customer {
	return contactdb.Customer{
		ID:        id,
		Name:      "customer",
		Extra:     []byte(extra),
		CreatedAt: pgtype.Timestamptz{Time: updatedAt.Add(-time.Hour), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true},
	}
}

type customerQueryBeginner struct {
	tx *customerQueryTx
}

func (beginner *customerQueryBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return beginner.tx, nil
}

type customerQueryTx struct {
	boundedIDs  []int64
	rows        []contactdb.Customer
	boundedArgs []any
	listArgs    []any
	commits     int
	rollbacks   int
}

func (*customerQueryTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}
func (tx *customerQueryTx) Commit(context.Context) error {
	tx.commits++
	return nil
}
func (tx *customerQueryTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}
func (*customerQueryTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}
func (*customerQueryTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (*customerQueryTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (*customerQueryTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}
func (*customerQueryTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}
func (tx *customerQueryTx) Query(_ context.Context, statement string, args ...any) (pgx.Rows, error) {
	switch {
	case strings.Contains(statement, "-- name: ListCustomerIDsBounded :many"):
		tx.boundedArgs = append([]any(nil), args...)
		return &customerIDRows{ids: tx.boundedIDs}, nil
	case strings.Contains(statement, "-- name: ListCustomers :many"):
		tx.listArgs = append([]any(nil), args...)
		return &customerQueryRows{rows: tx.rows}, nil
	default:
		return nil, errors.New("unexpected query")
	}
}
func (*customerQueryTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (*customerQueryTx) Conn() *pgx.Conn                                  { return nil }

type customerIDRows struct {
	ids     []int64
	index   int
	current int64
}

func (*customerIDRows) Close()                        {}
func (*customerIDRows) Err() error                    { return nil }
func (*customerIDRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (*customerIDRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}
func (rows *customerIDRows) Next() bool {
	if rows.index >= len(rows.ids) {
		return false
	}
	rows.current = rows.ids[rows.index]
	rows.index++
	return true
}
func (rows *customerIDRows) Scan(dest ...any) error {
	if len(dest) != 1 {
		return errors.New("unexpected customer id scan destination count")
	}
	target, ok := dest[0].(*int64)
	if !ok {
		return errors.New("unexpected customer id scan type")
	}
	*target = rows.current
	return nil
}
func (*customerIDRows) Values() ([]any, error) { return nil, errors.New("not implemented") }
func (*customerIDRows) RawValues() [][]byte    { return nil }
func (*customerIDRows) Conn() *pgx.Conn        { return nil }

type customerQueryRows struct {
	rows    []contactdb.Customer
	index   int
	current contactdb.Customer
}

func (*customerQueryRows) Close()                        {}
func (*customerQueryRows) Err() error                    { return nil }
func (*customerQueryRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (*customerQueryRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}
func (rows *customerQueryRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.current = rows.rows[rows.index]
	rows.index++
	return true
}
func (rows *customerQueryRows) Scan(dest ...any) error {
	if len(dest) != 13 {
		return errors.New("unexpected customer scan destination count")
	}
	row := rows.current
	*dest[0].(*int64) = row.ID
	*dest[1].(*string) = row.Name
	*dest[2].(*pgtype.Text) = row.AvatarUrl
	*dest[3].(*pgtype.Int2) = row.Gender
	*dest[4].(*pgtype.Int8) = row.StageID
	*dest[5].(*pgtype.Int8) = row.OwnerStaffID
	*dest[6].(*pgtype.Int8) = row.ChannelID
	*dest[7].(*pgtype.Timestamptz) = row.AddedAt
	*dest[8].(*pgtype.Timestamptz) = row.LastInteractAt
	*dest[9].(*bool) = row.IsDeleted
	*dest[10].(*[]byte) = row.Extra
	*dest[11].(*pgtype.Timestamptz) = row.CreatedAt
	*dest[12].(*pgtype.Timestamptz) = row.UpdatedAt
	return nil
}
func (*customerQueryRows) Values() ([]any, error) { return nil, errors.New("not implemented") }
func (*customerQueryRows) RawValues() [][]byte    { return nil }
func (*customerQueryRows) Conn() *pgx.Conn        { return nil }
