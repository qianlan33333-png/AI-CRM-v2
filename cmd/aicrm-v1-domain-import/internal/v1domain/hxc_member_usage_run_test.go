package v1domain

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
)

type hxcUsageCountRow struct {
	values []any
	err    error
}

func (row hxcUsageCountRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return errors.New("scan arity")
	}
	for i, value := range row.values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

type hxcUsageCountDB struct {
	rows  []hxcUsageCountRow
	calls int
}

func (db *hxcUsageCountDB) QueryRow(context.Context, string, ...any) pgx.Row {
	db.calls++
	if len(db.rows) == 0 {
		return hxcUsageCountRow{err: errors.New("unexpected query")}
	}
	row := db.rows[0]
	db.rows = db.rows[1:]
	return row
}

func TestHXCMemberUsageSourceRequiresSealAndPositiveManifest(t *testing.T) {
	for _, tc := range []struct {
		ready bool
		count int64
		want  bool
	}{{true, 810554, true}, {false, 810554, false}, {true, 0, false}, {true, -1, false}} {
		db := &hxcUsageCountDB{rows: []hxcUsageCountRow{{values: []any{tc.ready}}, {values: []any{tc.count}}}}
		count, err := hxcMemberUsageSourceCount(context.Background(), db, "archive")
		if (err == nil) != tc.want || (tc.want && count != tc.count) {
			t.Fatalf("%+v: %d %v", tc, count, err)
		}
		if !tc.ready && db.calls != 1 {
			t.Fatal("unsealed source reached manifest")
		}
	}
	for _, rows := range [][]hxcUsageCountRow{{{err: pgx.ErrNoRows}}, {{values: []any{true}}, {err: pgx.ErrNoRows}}} {
		if _, err := hxcMemberUsageSourceCount(context.Background(), &hxcUsageCountDB{rows: rows}, "archive"); err == nil {
			t.Fatal("missing source evidence accepted")
		}
	}
}

func TestHXCMemberUsageTargetCountRejectsMissingExtraOrAliasedTargets(t *testing.T) {
	for _, counts := range [][3]int64{{810554, 810554, 810554}, {810553, 810554, 810554}, {810555, 810554, 810554}, {810554, 810553, 810553}, {810554, 810554, 810553}} {
		db := &hxcUsageCountDB{rows: []hxcUsageCountRow{{values: []any{counts[0], counts[1], counts[2]}}}}
		err := hxcMemberUsageTargetCount(context.Background(), db, "archive", 810554)
		want := counts == [3]int64{810554, 810554, 810554}
		if (err == nil) != want {
			t.Fatalf("%v: %v", counts, err)
		}
	}
	if err := hxcMemberUsageTargetCount(context.Background(), &hxcUsageCountDB{}, "archive", 810554); err == nil {
		t.Fatal("query error accepted")
	}
}

func TestHXCMemberUsageRunRejectsMissingDependencies(t *testing.T) {
	if _, err := RunHXCMemberUsageHistory(context.Background(), nil, nil, "archive", make([]byte, 32), false); err == nil {
		t.Fatal("missing dependencies accepted")
	}
}
