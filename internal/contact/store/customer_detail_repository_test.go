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
	for _, input := range []contactapp.CustomerDetailInput{{}, {ID: 1, OwnerStaffID: &invalidOwner}} {
		_, err := NewCustomerDetailRepository().GetCustomerDetail(context.Background(), input)
		if !errors.Is(err, contactapp.ErrInvalidCustomerDetailQuery) {
			t.Fatalf("GetCustomerDetail(%#v) error = %v, want invalid query", input, err)
		}
	}
}

func TestCustomerDetailRepositoryRequiresTransactionBoundContext(t *testing.T) {
	_, err := NewCustomerDetailRepository().GetCustomerDetail(context.Background(), validCustomerDetailInput())
	if !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("GetCustomerDetail() error = %v, want transaction requirement", err)
	}
}

func TestCustomerDetailRepositoryReadsCustomerAndTagsWithOneSnapshotQuery(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	ownerStaffID := int64(8)
	first := detailSnapshotRow(42, at)
	first.TagID = pgtype.Int8{Int64: 13, Valid: true}
	first.TagName = pgtype.Text{String: "未分组", Valid: true}
	first.TagSortOrder = pgtype.Int4{Int32: 2, Valid: true}
	second := detailSnapshotRow(42, at)
	second.TagID = pgtype.Int8{Int64: 11, Valid: true}
	second.GroupID = pgtype.Int8{Int64: 5, Valid: true}
	second.GroupName = pgtype.Text{String: "客户分组", Valid: true}
	second.GroupSortOrder = 1
	second.TagName = pgtype.Text{String: "重点", Valid: true}
	second.TagSortOrder = pgtype.Int4{Int32: -1, Valid: true}
	tx := &customerDetailTx{rows: []contactdb.GetCustomerDetailSnapshotRow{first, second}}
	uow := platformstore.NewUnitOfWork(&customerDetailBeginner{tx: tx})
	input := contactapp.CustomerDetailInput{ID: 42, OwnerStaffID: &ownerStaffID}

	var result contactapp.CustomerDetailStoreResult
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var readErr error
		result, readErr = NewCustomerDetailRepository().GetCustomerDetail(txCtx, input)
		return readErr
	})
	if err != nil {
		t.Fatalf("GetCustomerDetail() error = %v", err)
	}
	if tx.commits != 1 || tx.rollbacks != 0 || tx.queryCalls != 1 {
		t.Fatalf("transaction/query calls = %d/%d/%d, want 1/0/1", tx.commits, tx.rollbacks, tx.queryCalls)
	}
	if result.Customer.ID != 42 || result.Customer.OwnerStaffID == nil || *result.Customer.OwnerStaffID != ownerStaffID {
		t.Fatalf("customer = %#v", result.Customer)
	}
	if len(result.Tags) != 2 || result.Tags[0].ID != 13 || result.Tags[0].GroupID != nil ||
		result.Tags[1].GroupID == nil || *result.Tags[1].GroupID != 5 {
		t.Fatalf("tags = %#v", result.Tags)
	}
	if len(tx.queryArgs) != 2 || tx.queryArgs[0].(int64) != int64(input.ID) {
		t.Fatalf("query args = %#v", tx.queryArgs)
	}
	ownerArgument := tx.queryArgs[1].(pgtype.Int8)
	if !ownerArgument.Valid || ownerArgument.Int64 != ownerStaffID {
		t.Fatalf("owner SQL argument = %#v", ownerArgument)
	}
	for _, required := range []string{
		"-- name: GetCustomerDetailSnapshot :many",
		"c.owner_staff_id = $2::bigint",
		"LEFT JOIN customer_tags",
		"ORDER BY COALESCE(g.sort_order, 0), t.sort_order, t.id",
	} {
		if !strings.Contains(tx.statement, required) {
			t.Fatalf("snapshot SQL missing %q:\n%s", required, tx.statement)
		}
	}
	if strings.Contains(strings.ToLower(tx.statement), "wecom_tag_id") {
		t.Fatalf("snapshot SQL exposed WeCom tag id:\n%s", tx.statement)
	}
}

func TestCustomerDetailRepositoryMapsMissingAndOwnerMismatchToNotFound(t *testing.T) {
	ownerStaffID := int64(8)
	for _, input := range []contactapp.CustomerDetailInput{{ID: 42}, {ID: 42, OwnerStaffID: &ownerStaffID}} {
		tx := &customerDetailTx{}
		uow := platformstore.NewUnitOfWork(&customerDetailBeginner{tx: tx})
		err := uow.Within(context.Background(), func(txCtx context.Context) error {
			_, readErr := NewCustomerDetailRepository().GetCustomerDetail(txCtx, input)
			return readErr
		})
		if !errors.Is(err, contactapp.ErrCustomerNotFound) || tx.queryCalls != 1 || tx.commits != 0 || tx.rollbacks != 1 {
			t.Fatalf("missing result error/calls = %v/%d/%d/%d", err, tx.queryCalls, tx.commits, tx.rollbacks)
		}
	}
}

func TestCustomerDetailRepositoryReturnsNonNilEmptyTags(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	tx := &customerDetailTx{rows: []contactdb.GetCustomerDetailSnapshotRow{detailSnapshotRow(42, at)}}
	uow := platformstore.NewUnitOfWork(&customerDetailBeginner{tx: tx})
	var result contactapp.CustomerDetailStoreResult
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var readErr error
		result, readErr = NewCustomerDetailRepository().GetCustomerDetail(txCtx, validCustomerDetailInput())
		return readErr
	})
	if err != nil || result.Tags == nil || len(result.Tags) != 0 {
		t.Fatalf("error/tags = %v/%#v, want non-nil empty", err, result.Tags)
	}
}

func TestCustomerDetailRepositoryFailsClosedOnMalformedJoinAndQueryError(t *testing.T) {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	malformed := detailSnapshotRow(42, at)
	malformed.TagID = pgtype.Int8{Int64: 9, Valid: true}
	for _, tx := range []*customerDetailTx{
		{rows: []contactdb.GetCustomerDetailSnapshotRow{malformed}},
		{queryErr: errors.New("snapshot query failed")},
	} {
		uow := platformstore.NewUnitOfWork(&customerDetailBeginner{tx: tx})
		err := uow.Within(context.Background(), func(txCtx context.Context) error {
			_, readErr := NewCustomerDetailRepository().GetCustomerDetail(txCtx, validCustomerDetailInput())
			return readErr
		})
		if err == nil || tx.commits != 0 || tx.rollbacks != 1 {
			t.Fatalf("malformed/query error = %v, commit/rollback=%d/%d", err, tx.commits, tx.rollbacks)
		}
	}
}

func validCustomerDetailInput() contactapp.CustomerDetailInput {
	return contactapp.CustomerDetailInput{ID: contactport.CustomerID(42)}
}

func detailSnapshotRow(id int64, at time.Time) contactdb.GetCustomerDetailSnapshotRow {
	return contactdb.GetCustomerDetailSnapshotRow{
		ID: id, Name: "customer", OwnerStaffID: pgtype.Int8{Int64: 8, Valid: true},
		Extra:     []byte(`{"source":"test"}`),
		CreatedAt: pgtype.Timestamptz{Time: at.Add(-time.Hour), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true},
	}
}

type customerDetailBeginner struct{ tx *customerDetailTx }

func (beginner *customerDetailBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return beginner.tx, nil
}

type customerDetailTx struct {
	rows       []contactdb.GetCustomerDetailSnapshotRow
	queryErr   error
	statement  string
	queryArgs  []any
	queryCalls int
	commits    int
	rollbacks  int
}

func (*customerDetailTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}
func (tx *customerDetailTx) Commit(context.Context) error   { tx.commits++; return nil }
func (tx *customerDetailTx) Rollback(context.Context) error { tx.rollbacks++; return nil }
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
	tx.queryCalls++
	tx.statement = statement
	tx.queryArgs = append([]any(nil), args...)
	if tx.queryErr != nil {
		return nil, tx.queryErr
	}
	return &customerDetailRows{rows: tx.rows}, nil
}
func (*customerDetailTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return customerDetailUnexpectedRow{}
}
func (*customerDetailTx) Conn() *pgx.Conn { return nil }

type customerDetailUnexpectedRow struct{}

func (customerDetailUnexpectedRow) Scan(...any) error { return errors.New("unexpected QueryRow") }

type customerDetailRows struct {
	rows    []contactdb.GetCustomerDetailSnapshotRow
	index   int
	current contactdb.GetCustomerDetailSnapshotRow
}

func (*customerDetailRows) Close()                                       {}
func (*customerDetailRows) Err() error                                   { return nil }
func (*customerDetailRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (*customerDetailRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (rows *customerDetailRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.current = rows.rows[rows.index]
	rows.index++
	return true
}
func (rows *customerDetailRows) Scan(dest ...any) error {
	if len(dest) != 19 {
		return errors.New("unexpected snapshot scan destination count")
	}
	*dest[0].(*int64) = rows.current.ID
	*dest[1].(*string) = rows.current.Name
	*dest[2].(*pgtype.Text) = rows.current.AvatarUrl
	*dest[3].(*pgtype.Int2) = rows.current.Gender
	*dest[4].(*pgtype.Int8) = rows.current.StageID
	*dest[5].(*pgtype.Int8) = rows.current.OwnerStaffID
	*dest[6].(*pgtype.Int8) = rows.current.ChannelID
	*dest[7].(*pgtype.Timestamptz) = rows.current.AddedAt
	*dest[8].(*pgtype.Timestamptz) = rows.current.LastInteractAt
	*dest[9].(*bool) = rows.current.IsDeleted
	*dest[10].(*[]byte) = rows.current.Extra
	*dest[11].(*pgtype.Timestamptz) = rows.current.CreatedAt
	*dest[12].(*pgtype.Timestamptz) = rows.current.UpdatedAt
	*dest[13].(*pgtype.Int8) = rows.current.TagID
	*dest[14].(*pgtype.Int8) = rows.current.GroupID
	*dest[15].(*pgtype.Text) = rows.current.GroupName
	*dest[16].(*int32) = rows.current.GroupSortOrder
	*dest[17].(*pgtype.Text) = rows.current.TagName
	*dest[18].(*pgtype.Int4) = rows.current.TagSortOrder
	return nil
}
func (*customerDetailRows) Values() ([]any, error) { return nil, errors.New("not implemented") }
func (*customerDetailRows) RawValues() [][]byte    { return nil }
func (*customerDetailRows) Conn() *pgx.Conn        { return nil }
