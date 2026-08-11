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

func TestCustomerDetailRepositoryRejectsInvalidQueryBeforeDatabaseAccess(t *testing.T) {
	invalidOwner := int64(0)
	for _, input := range []contactapp.CustomerDetailInput{
		{},
		{ID: 1, OwnerStaffID: &invalidOwner},
	} {
		_, err := NewCustomerDetailRepository().GetCustomerDetail(context.Background(), input)
		if !errors.Is(err, contactapp.ErrInvalidCustomerDetailQuery) {
			t.Fatalf("GetCustomerDetail(%#v) error = %v, want invalid customer detail query", input, err)
		}
	}
}

func TestCustomerDetailRepositoryRequiresTransactionBoundContext(t *testing.T) {
	_, err := NewCustomerDetailRepository().GetCustomerDetail(context.Background(), validCustomerDetailInput())
	if !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("GetCustomerDetail() error = %v, want transaction requirement", err)
	}
}

func TestCustomerDetailRepositoryReadsCustomerAndLocalTagsOnOneTransaction(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	ownerStaffID := int64(8)
	tx := &customerDetailTx{
		customer: detailCustomerRow(42, at),
		tags: []contactdb.ListCustomerDetailTagsRow{
			{
				ID: 13, GroupSortOrder: 0, Name: "未分组", SortOrder: 2,
			},
			{
				ID:             11,
				GroupID:        pgtype.Int8{Int64: 5, Valid: true},
				GroupName:      pgtype.Text{String: "客户分组", Valid: true},
				GroupSortOrder: 1,
				Name:           "重点",
				SortOrder:      -1,
			},
		},
	}
	uow := platformstore.NewUnitOfWork(&customerDetailBeginner{tx: tx})
	input := contactapp.CustomerDetailInput{ID: 42, OwnerStaffID: &ownerStaffID}

	var result contactapp.CustomerDetailStoreResult
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var err error
		result, err = NewCustomerDetailRepository().GetCustomerDetail(txCtx, input)
		return err
	})
	if err != nil {
		t.Fatalf("GetCustomerDetail() error = %v", err)
	}
	if tx.commits != 1 || tx.rollbacks != 0 || tx.customerCalls != 1 || tx.tagCalls != 1 {
		t.Fatalf("transaction/query calls = commit:%d rollback:%d customer:%d tags:%d, want 1/0/1/1", tx.commits, tx.rollbacks, tx.customerCalls, tx.tagCalls)
	}
	if result.Customer.ID != 42 || result.Customer.OwnerStaffID == nil || *result.Customer.OwnerStaffID != ownerStaffID {
		t.Fatalf("customer = %#v, want mapped scoped customer", result.Customer)
	}
	if len(result.Tags) != 2 || result.Tags[0].ID != 13 || result.Tags[0].GroupID != nil || result.Tags[0].GroupName != nil {
		t.Fatalf("tags = %#v, want mapped local tag rows", result.Tags)
	}
	if result.Tags[1].GroupID == nil || *result.Tags[1].GroupID != 5 || result.Tags[1].GroupName == nil || *result.Tags[1].GroupName != "客户分组" {
		t.Fatalf("grouped tag = %#v, want group pair", result.Tags[1])
	}
	if len(tx.customerArgs) != 2 {
		t.Fatalf("customer args = %#v, want id and owner predicate", tx.customerArgs)
	}
	if got := tx.customerArgs[0].(int64); got != int64(input.ID) {
		t.Fatalf("customer id argument = %d, want %d", got, input.ID)
	}
	ownerArgument := tx.customerArgs[1].(pgtype.Int8)
	if !ownerArgument.Valid || ownerArgument.Int64 != ownerStaffID {
		t.Fatalf("owner SQL argument = %#v, want valid owner %d", ownerArgument, ownerStaffID)
	}
	if len(tx.tagArgs) != 1 || tx.tagArgs[0].(int64) != int64(input.ID) {
		t.Fatalf("tag args = %#v, want customer id %d", tx.tagArgs, input.ID)
	}
	if !strings.Contains(tx.customerStatement, "c.owner_staff_id = $2::bigint") {
		t.Fatalf("customer SQL must enforce the owner predicate, got:\n%s", tx.customerStatement)
	}
	if strings.Contains(strings.ToLower(tx.tagStatement), "wecom_tag_id") {
		t.Fatalf("tag SQL must not select wecom tag identifiers, got:\n%s", tx.tagStatement)
	}
	if !strings.Contains(tx.tagStatement, "ORDER BY COALESCE(g.sort_order, 0), t.sort_order, t.id") {
		t.Fatalf("tag SQL has wrong ordering, got:\n%s", tx.tagStatement)
	}
}

func TestCustomerDetailRepositoryMapsMissingAndOwnerMismatchToNotFound(t *testing.T) {
	ownerStaffID := int64(8)
	for _, input := range []contactapp.CustomerDetailInput{
		{ID: 42},
		{ID: 42, OwnerStaffID: &ownerStaffID},
	} {
		tx := &customerDetailTx{customerErr: pgx.ErrNoRows}
		uow := platformstore.NewUnitOfWork(&customerDetailBeginner{tx: tx})

		err := uow.Within(context.Background(), func(txCtx context.Context) error {
			_, err := NewCustomerDetailRepository().GetCustomerDetail(txCtx, input)
			return err
		})
		if !errors.Is(err, contactapp.ErrCustomerNotFound) {
			t.Fatalf("GetCustomerDetail(%#v) error = %v, want customer not found", input, err)
		}
		if tx.tagCalls != 0 || tx.commits != 0 || tx.rollbacks != 1 {
			t.Fatalf("missing customer calls = tags:%d commit:%d rollback:%d, want 0/0/1", tx.tagCalls, tx.commits, tx.rollbacks)
		}
	}
}

func TestCustomerDetailRepositoryReturnsNonNilEmptyTags(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	tx := &customerDetailTx{customer: detailCustomerRow(42, at)}
	uow := platformstore.NewUnitOfWork(&customerDetailBeginner{tx: tx})

	var result contactapp.CustomerDetailStoreResult
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var err error
		result, err = NewCustomerDetailRepository().GetCustomerDetail(txCtx, validCustomerDetailInput())
		return err
	})
	if err != nil {
		t.Fatalf("GetCustomerDetail() error = %v", err)
	}
	if result.Tags == nil || len(result.Tags) != 0 {
		t.Fatalf("tags = %#v, want non-nil empty slice", result.Tags)
	}
}

func TestCustomerDetailRepositoryPropagatesTagQueryError(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	want := errors.New("tag query failed")
	tx := &customerDetailTx{customer: detailCustomerRow(42, at), tagErr: want}
	uow := platformstore.NewUnitOfWork(&customerDetailBeginner{tx: tx})

	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		_, err := NewCustomerDetailRepository().GetCustomerDetail(txCtx, validCustomerDetailInput())
		return err
	})
	if !errors.Is(err, want) {
		t.Fatalf("GetCustomerDetail() error = %v, want %v", err, want)
	}
}

func validCustomerDetailInput() contactapp.CustomerDetailInput {
	return contactapp.CustomerDetailInput{ID: contactport.CustomerID(42)}
}

func detailCustomerRow(id int64, at time.Time) contactdb.Customer {
	return contactdb.Customer{
		ID:           id,
		Name:         "customer",
		OwnerStaffID: pgtype.Int8{Int64: 8, Valid: true},
		Extra:        []byte(`{"source":"test"}`),
		CreatedAt:    pgtype.Timestamptz{Time: at.Add(-time.Hour), Valid: true},
		UpdatedAt:    pgtype.Timestamptz{Time: at, Valid: true},
	}
}

type customerDetailBeginner struct {
	tx *customerDetailTx
}

func (beginner *customerDetailBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return beginner.tx, nil
}

type customerDetailTx struct {
	customer          contactdb.Customer
	customerErr       error
	tags              []contactdb.ListCustomerDetailTagsRow
	tagErr            error
	customerStatement string
	tagStatement      string
	customerArgs      []any
	tagArgs           []any
	customerCalls     int
	tagCalls          int
	commits           int
	rollbacks         int
}

func (*customerDetailTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}
func (tx *customerDetailTx) Commit(context.Context) error {
	tx.commits++
	return nil
}
func (tx *customerDetailTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}
func (*customerDetailTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}
func (*customerDetailTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (*customerDetailTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (*customerDetailTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}
func (*customerDetailTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}
func (tx *customerDetailTx) Query(_ context.Context, statement string, args ...any) (pgx.Rows, error) {
	if !strings.Contains(statement, "-- name: ListCustomerDetailTags :many") {
		return nil, errors.New("unexpected query")
	}
	tx.tagCalls++
	tx.tagStatement = statement
	tx.tagArgs = append([]any(nil), args...)
	if tx.tagErr != nil {
		return nil, tx.tagErr
	}
	return &customerDetailTagRows{rows: tx.tags}, nil
}
func (tx *customerDetailTx) QueryRow(_ context.Context, statement string, args ...any) pgx.Row {
	if !strings.Contains(statement, "-- name: GetCustomerDetailCustomer :one") {
		return customerDetailRow{err: errors.New("unexpected query")}
	}
	tx.customerCalls++
	tx.customerStatement = statement
	tx.customerArgs = append([]any(nil), args...)
	return customerDetailRow{customer: tx.customer, err: tx.customerErr}
}
func (*customerDetailTx) Conn() *pgx.Conn { return nil }

type customerDetailRow struct {
	customer contactdb.Customer
	err      error
}

func (row customerDetailRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != 13 {
		return errors.New("unexpected customer scan destination count")
	}
	*dest[0].(*int64) = row.customer.ID
	*dest[1].(*string) = row.customer.Name
	*dest[2].(*pgtype.Text) = row.customer.AvatarUrl
	*dest[3].(*pgtype.Int2) = row.customer.Gender
	*dest[4].(*pgtype.Int8) = row.customer.StageID
	*dest[5].(*pgtype.Int8) = row.customer.OwnerStaffID
	*dest[6].(*pgtype.Int8) = row.customer.ChannelID
	*dest[7].(*pgtype.Timestamptz) = row.customer.AddedAt
	*dest[8].(*pgtype.Timestamptz) = row.customer.LastInteractAt
	*dest[9].(*bool) = row.customer.IsDeleted
	*dest[10].(*[]byte) = row.customer.Extra
	*dest[11].(*pgtype.Timestamptz) = row.customer.CreatedAt
	*dest[12].(*pgtype.Timestamptz) = row.customer.UpdatedAt
	return nil
}

type customerDetailTagRows struct {
	rows    []contactdb.ListCustomerDetailTagsRow
	index   int
	current contactdb.ListCustomerDetailTagsRow
}

func (*customerDetailTagRows) Close()                        {}
func (*customerDetailTagRows) Err() error                    { return nil }
func (*customerDetailTagRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (*customerDetailTagRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}
func (rows *customerDetailTagRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.current = rows.rows[rows.index]
	rows.index++
	return true
}
func (rows *customerDetailTagRows) Scan(dest ...any) error {
	if len(dest) != 6 {
		return errors.New("unexpected tag scan destination count")
	}
	*dest[0].(*int64) = rows.current.ID
	*dest[1].(*pgtype.Int8) = rows.current.GroupID
	*dest[2].(*pgtype.Text) = rows.current.GroupName
	*dest[3].(*int32) = rows.current.GroupSortOrder
	*dest[4].(*string) = rows.current.Name
	*dest[5].(*int32) = rows.current.SortOrder
	return nil
}
func (*customerDetailTagRows) Values() ([]any, error) { return nil, errors.New("not implemented") }
func (*customerDetailTagRows) RawValues() [][]byte    { return nil }
func (*customerDetailTagRows) Conn() *pgx.Conn        { return nil }
