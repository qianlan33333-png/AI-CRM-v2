package legacyaudience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type scriptedRow struct {
	scan func(...any) error
}

func (row scriptedRow) Scan(destinations ...any) error {
	if row.scan == nil {
		return errors.New("unexpected row scan")
	}
	return row.scan(destinations...)
}

type emptyRows struct{}

func (emptyRows) Next() bool        { return false }
func (emptyRows) Scan(...any) error { return errors.New("unexpected scan") }
func (emptyRows) Err() error        { return nil }
func (emptyRows) Close()            {}

type scriptedExecutor struct {
	rows         []SQLRow
	queries      []string
	queryArgs    [][]any
	execs        []string
	execArgs     [][]any
	execAffected []int64
}

func (executor *scriptedExecutor) Exec(_ context.Context, query string, args ...any) (int64, error) {
	executor.execs = append(executor.execs, query)
	executor.execArgs = append(executor.execArgs, append([]any(nil), args...))
	if len(executor.execAffected) == 0 {
		return 1, nil
	}
	value := executor.execAffected[0]
	executor.execAffected = executor.execAffected[1:]
	return value, nil
}

func (executor *scriptedExecutor) Query(_ context.Context, query string, args ...any) (SQLRows, error) {
	executor.queries = append(executor.queries, query)
	executor.queryArgs = append(executor.queryArgs, append([]any(nil), args...))
	return emptyRows{}, nil
}

func (executor *scriptedExecutor) QueryRow(_ context.Context, query string, args ...any) SQLRow {
	executor.queries = append(executor.queries, query)
	executor.queryArgs = append(executor.queryArgs, append([]any(nil), args...))
	if len(executor.rows) == 0 {
		return scriptedRow{scan: func(...any) error { return errors.New("no scripted row") }}
	}
	row := executor.rows[0]
	executor.rows = executor.rows[1:]
	return row
}

type scriptedProvider struct {
	executor SQLExecutor
	noRows   error
}

func (provider scriptedProvider) Reader(context.Context) (SQLExecutor, error) {
	return provider.executor, nil
}
func (provider scriptedProvider) Transaction(context.Context) (SQLExecutor, error) {
	return provider.executor, nil
}
func (provider scriptedProvider) IsNoRows(err error) bool { return errors.Is(err, provider.noRows) }

func TestSQLRepositoryCopyNameLockUsesGlobalNamespace(t *testing.T) {
	executor := &scriptedExecutor{}
	repository, err := NewSQLRepository(scriptedProvider{executor: executor})
	if err != nil {
		t.Fatalf("NewSQLRepository: %v", err)
	}
	if err = repository.LockCopyNameNamespace(context.Background(), "任意源名称"); err != nil {
		t.Fatalf("LockCopyNameNamespace: %v", err)
	}
	if len(executor.execs) != 1 || len(executor.execArgs) != 1 || len(executor.execArgs[0]) != 0 {
		t.Fatalf("global lock calls=%d args=%v", len(executor.execs), executor.execArgs)
	}
	query := executor.execs[0]
	if !strings.Contains(query, "pg_advisory_xact_lock") ||
		!strings.Contains(query, "ai_audience.package.copy.name.v1") || strings.Contains(query, "$1") {
		t.Fatalf("copy namespace lock is not global:\n%s", query)
	}
}

func TestSQLRepositoryCopySetsZeroMembersAndNeverWritesMemberSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 22, 3, 0, 0, 0, time.UTC)
	definition := []byte(`{"field":"stage_id","op":"eq","value":1}`)
	executor := &scriptedExecutor{rows: []SQLRow{
		scriptedRow{scan: func(dest ...any) error {
			*dest[0].(*int64) = 103
			*dest[1].(*string) = "高价值客户 副本"
			*dest[2].(*[]byte) = append([]byte(nil), definition...)
			*dest[3].(*string) = "manual"
			*dest[4].(*sql.NullString) = sql.NullString{}
			*dest[5].(*string) = "active"
			return nil
		}},
		scriptedRow{scan: func(dest ...any) error {
			*dest[0].(*int64) = 103
			*dest[1].(*sql.NullInt64) = sql.NullInt64{Int64: 1, Valid: true}
			*dest[2].(*PackageLifecycle) = PackagePaused
			*dest[3].(*int64) = 1
			*dest[4].(*int64) = 9
			*dest[5].(*int64) = 9
			*dest[6].(*time.Time) = now
			*dest[7].(*time.Time) = now
			return nil
		}},
	}}
	repository, err := NewSQLRepository(scriptedProvider{executor: executor})
	if err != nil {
		t.Fatalf("NewSQLRepository: %v", err)
	}
	source := PackageWriteModel{
		SegmentID: 101, Name: "高价值客户", Definition: segmentport.Definition(definition), RefreshMode: segmentport.RefreshModeManual,
		SegmentLifecycle: segmentport.LifecycleStatusActive,
		Metadata:         PackageMetadata{SegmentID: 101, GroupID: int64Pointer(1), Lifecycle: PackageActive, Version: 2},
	}
	copied, err := repository.InsertPackageCopy(context.Background(), source, "高价值客户 副本", 9, now)
	if err != nil {
		t.Fatalf("InsertPackageCopy: %v", err)
	}
	if copied.SegmentID != 103 || copied.Metadata.Lifecycle != PackagePaused || copied.Metadata.Version != 1 {
		t.Fatalf("copy=%+v", copied)
	}
	if len(executor.queries) != 2 {
		t.Fatalf("query count=%d", len(executor.queries))
	}
	segmentInsert := executor.queries[0]
	if !strings.Contains(segmentInsert, "INSERT INTO public.segments") || !strings.Contains(segmentInsert, "member_count") ||
		!strings.Contains(segmentInsert, "0, NULL, 'idle'") {
		t.Fatalf("copy SQL does not reset member facts:\n%s", segmentInsert)
	}
	combined := strings.Join(executor.queries, "\n")
	if strings.Contains(combined, "segment_members") {
		t.Fatalf("copy SQL writes member snapshot:\n%s", combined)
	}
	if !strings.Contains(executor.queries[1], "INSERT INTO public.ai_audience_package_metadata") ||
		!strings.Contains(executor.queries[1], "'paused', 1") {
		t.Fatalf("metadata copy SQL is not paused/version 1:\n%s", executor.queries[1])
	}
}

func TestSQLRepositorySavePackageUsesSegmentAndMetadataCAS(t *testing.T) {
	now := time.Date(2026, 8, 22, 4, 0, 0, 0, time.UTC)
	executor := &scriptedExecutor{execAffected: []int64{1}, rows: []SQLRow{
		scriptedRow{scan: func(dest ...any) error {
			*dest[0].(*int64) = 101
			*dest[1].(*sql.NullInt64) = sql.NullInt64{Int64: 1, Valid: true}
			*dest[2].(*PackageLifecycle) = PackageArchived
			*dest[3].(*int64) = 2
			*dest[4].(*int64) = 1
			*dest[5].(*int64) = 9
			*dest[6].(*time.Time) = now.Add(-time.Hour)
			*dest[7].(*time.Time) = now
			return nil
		}},
	}}
	repository, _ := NewSQLRepository(scriptedProvider{executor: executor})
	definition := segmentport.Definition(`{"field":"stage_id","op":"eq","value":1}`)
	current := PackageWriteModel{
		SegmentID: 101, Name: "源套餐", Definition: definition, RefreshMode: segmentport.RefreshModeManual,
		SegmentLifecycle: segmentport.LifecycleStatusActive,
		Metadata:         PackageMetadata{SegmentID: 101, GroupID: int64Pointer(1), Lifecycle: PackageActive, Version: 1},
	}
	next := cloneWriteModel(current)
	next.SegmentLifecycle = segmentport.LifecycleStatusArchived
	next.Metadata.Lifecycle = PackageArchived
	next.Metadata.Version = 2
	updated, err := repository.SavePackage(context.Background(), current, next, 1, 9, now)
	if err != nil {
		t.Fatalf("SavePackage: %v", err)
	}
	if updated.Metadata.Version != 2 || updated.Metadata.Lifecycle != PackageArchived {
		t.Fatalf("updated=%+v", updated)
	}
	if len(executor.execs) != 1 || !strings.Contains(executor.execs[0], "UPDATE public.segments") ||
		!strings.Contains(executor.execs[0], "archived_at") || !strings.Contains(executor.execs[0], "lifecycle_status = $5") {
		t.Fatalf("segment CAS SQL:\n%s", strings.Join(executor.execs, "\n"))
	}
	if len(executor.queries) != 1 || !strings.Contains(executor.queries[0], "WHERE segment_id = $5 AND version = $6") ||
		!strings.Contains(executor.queries[0], "version = version + 1") {
		t.Fatalf("metadata CAS SQL:\n%s", strings.Join(executor.queries, "\n"))
	}
	if got := executor.execArgs[0][6]; got != "admin:9" {
		t.Fatalf("archived_by arg=%v", got)
	}
}

func TestSQLRepositoryReceiptReservationIsScopedAndLocked(t *testing.T) {
	wanted := ReceiptReservation{
		Operation: OperationPackagePause, ActorID: 9, KeyDigest: [32]byte{1}, PayloadDigest: [32]byte{2},
		CreatedAt: time.Date(2026, 8, 22, 5, 0, 0, 0, time.UTC),
	}
	executor := &scriptedExecutor{rows: []SQLRow{
		scriptedRow{scan: func(dest ...any) error {
			*dest[0].(*int64) = 1
			*dest[1].(*ReceiptOperation) = wanted.Operation
			*dest[2].(*int64) = wanted.ActorID
			*dest[3].(*[]byte) = append([]byte(nil), wanted.KeyDigest[:]...)
			*dest[4].(*[]byte) = append([]byte(nil), wanted.PayloadDigest[:]...)
			*dest[5].(*string) = "in_progress"
			*dest[6].(*[]byte) = nil
			return nil
		}},
	}}
	repository, _ := NewSQLRepository(scriptedProvider{executor: executor})
	receipt, owned, err := repository.ReserveReceipt(context.Background(), wanted)
	if err != nil || !owned || receipt.ID != 1 {
		t.Fatalf("receipt=%+v owned=%v err=%v", receipt, owned, err)
	}
	if len(executor.queries) != 1 || !strings.Contains(executor.queries[0], "ON CONFLICT (operation, actor_id, key_digest) DO NOTHING") {
		t.Fatalf("receipt SQL:\n%s", strings.Join(executor.queries, "\n"))
	}
}

func TestScriptedExecutorJSONDefinitionIsValid(t *testing.T) {
	if !json.Valid([]byte(`{"field":"stage_id","op":"eq","value":1}`)) {
		t.Fatal("test fixture definition must be valid JSON")
	}
}
