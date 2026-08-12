package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestTagCatalogRepositoryRequiresTransactionBoundContext(t *testing.T) {
	items, err := NewTagCatalogRepository().ListTags(context.Background())
	if !errors.Is(err, platformport.ErrTransactionRequired) || items != nil {
		t.Fatalf("ListTags() items/error = %#v/%v, want transaction requirement", items, err)
	}
}

func TestTagCatalogRepositoryListsGroupedAndUngroupedTags(t *testing.T) {
	tx := &tagCatalogTx{rows: []contactdb.ListTagsRow{
		{
			ID: 11, GroupID: pgtype.Int8{Int64: 3, Valid: true},
			GroupName:      pgtype.Text{String: "客户层级", Valid: true},
			GroupSortOrder: pgtype.Int4{Int32: 2, Valid: true}, Name: "重点", SortOrder: -1,
		},
		{ID: 17, Name: "未分组", SortOrder: 4},
	}}
	uow := platformstore.NewUnitOfWork(&tagCatalogBeginner{tx: tx})

	var items []contactapp.TagCatalogRecord
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var readErr error
		items, readErr = NewTagCatalogRepository().ListTags(txCtx)
		return readErr
	})
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	if tx.queryCalls != 1 || tx.commits != 1 || tx.rollbacks != 0 || len(tx.queryArgs) != 0 {
		t.Fatalf("query/commit/rollback/args = %d/%d/%d/%#v", tx.queryCalls, tx.commits, tx.rollbacks, tx.queryArgs)
	}
	if len(items) != 2 || items[0].ID != 11 || items[0].GroupID == nil || *items[0].GroupID != 3 ||
		items[0].GroupName == nil || *items[0].GroupName != "客户层级" ||
		items[0].GroupSortOrder == nil || *items[0].GroupSortOrder != 2 || items[0].SortOrder != -1 {
		t.Fatalf("grouped item = %#v", items)
	}
	if items[1].ID != 17 || items[1].GroupID != nil || items[1].GroupName != nil ||
		items[1].GroupSortOrder != nil || items[1].Name != "未分组" || items[1].SortOrder != 4 {
		t.Fatalf("ungrouped item = %#v", items[1])
	}
	for _, required := range []string{
		"-- name: ListTags :many",
		"FROM tags AS t",
		"LEFT JOIN tag_groups AS g ON g.id = t.group_id",
		"(t.group_id IS NULL)",
		"g.sort_order",
		"g.id",
		"t.sort_order",
		"t.id",
	} {
		if !strings.Contains(tx.statement, required) {
			t.Fatalf("tag catalog SQL missing %q:\n%s", required, tx.statement)
		}
	}
	if strings.Contains(strings.ToLower(tx.statement), "wecom_tag_id") {
		t.Fatalf("tag catalog SQL exposed WeCom tag id:\n%s", tx.statement)
	}
}

func TestTagCatalogRepositoryReturnsNonNilEmptySlice(t *testing.T) {
	tx := &tagCatalogTx{}
	uow := platformstore.NewUnitOfWork(&tagCatalogBeginner{tx: tx})
	var items []contactapp.TagCatalogRecord
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var readErr error
		items, readErr = NewTagCatalogRepository().ListTags(txCtx)
		return readErr
	})
	if err != nil || items == nil || len(items) != 0 || tx.commits != 1 || tx.rollbacks != 0 {
		t.Fatalf("empty result/error/commit/rollback = %#v/%v/%d/%d", items, err, tx.commits, tx.rollbacks)
	}
}

func TestTagCatalogRepositoryFailsClosedOnNullableGroupMismatch(t *testing.T) {
	for _, row := range []contactdb.ListTagsRow{
		{ID: 1, GroupID: pgtype.Int8{Int64: 2, Valid: true}, Name: "tag"},
		{ID: 1, GroupName: pgtype.Text{String: "group", Valid: true}, Name: "tag"},
		{ID: 1, GroupSortOrder: pgtype.Int4{Int32: 2, Valid: true}, Name: "tag"},
		{ID: 1, GroupID: pgtype.Int8{Int64: 2, Valid: true}, GroupName: pgtype.Text{String: "group", Valid: true}, Name: "tag"},
	} {
		tx := &tagCatalogTx{rows: []contactdb.ListTagsRow{row}}
		uow := platformstore.NewUnitOfWork(&tagCatalogBeginner{tx: tx})
		err := uow.Within(context.Background(), func(txCtx context.Context) error {
			_, readErr := NewTagCatalogRepository().ListTags(txCtx)
			return readErr
		})
		if !errors.Is(err, errInvalidTagCatalogRow) || tx.commits != 0 || tx.rollbacks != 1 {
			t.Fatalf("row/error/commit/rollback = %#v/%v/%d/%d", row, err, tx.commits, tx.rollbacks)
		}
	}
}

func TestTagCatalogRepositoryPropagatesDatabaseError(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	tx := &tagCatalogTx{queryErr: databaseErr}
	uow := platformstore.NewUnitOfWork(&tagCatalogBeginner{tx: tx})
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		_, readErr := NewTagCatalogRepository().ListTags(txCtx)
		return readErr
	})
	if !errors.Is(err, databaseErr) || tx.commits != 0 || tx.rollbacks != 1 {
		t.Fatalf("error/commit/rollback = %v/%d/%d", err, tx.commits, tx.rollbacks)
	}
}

type tagCatalogBeginner struct{ tx *tagCatalogTx }

func (beginner *tagCatalogBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return beginner.tx, nil
}

type tagCatalogTx struct {
	rows       []contactdb.ListTagsRow
	queryErr   error
	statement  string
	queryArgs  []any
	queryCalls int
	commits    int
	rollbacks  int
}

func (*tagCatalogTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}
func (tx *tagCatalogTx) Commit(context.Context) error   { tx.commits++; return nil }
func (tx *tagCatalogTx) Rollback(context.Context) error { tx.rollbacks++; return nil }
func (*tagCatalogTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}
func (*tagCatalogTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (*tagCatalogTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (*tagCatalogTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}
func (*tagCatalogTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}
func (tx *tagCatalogTx) Query(_ context.Context, statement string, args ...any) (pgx.Rows, error) {
	tx.queryCalls++
	tx.statement = statement
	tx.queryArgs = append([]any(nil), args...)
	if tx.queryErr != nil {
		return nil, tx.queryErr
	}
	return &tagCatalogRows{rows: tx.rows}, nil
}
func (*tagCatalogTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (*tagCatalogTx) Conn() *pgx.Conn                                  { return nil }

type tagCatalogRows struct {
	rows    []contactdb.ListTagsRow
	index   int
	current contactdb.ListTagsRow
}

func (*tagCatalogRows) Close()                                       {}
func (*tagCatalogRows) Err() error                                   { return nil }
func (*tagCatalogRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (*tagCatalogRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (rows *tagCatalogRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.current = rows.rows[rows.index]
	rows.index++
	return true
}
func (rows *tagCatalogRows) Scan(dest ...any) error {
	if len(dest) != 6 {
		return errors.New("unexpected tag catalog scan destination count")
	}
	*dest[0].(*int64) = rows.current.ID
	*dest[1].(*pgtype.Int8) = rows.current.GroupID
	*dest[2].(*pgtype.Text) = rows.current.GroupName
	*dest[3].(*pgtype.Int4) = rows.current.GroupSortOrder
	*dest[4].(*string) = rows.current.Name
	*dest[5].(*int32) = rows.current.SortOrder
	return nil
}
func (*tagCatalogRows) Values() ([]any, error) { return nil, errors.New("not implemented") }
func (*tagCatalogRows) RawValues() [][]byte    { return nil }
func (*tagCatalogRows) Conn() *pgx.Conn        { return nil }
