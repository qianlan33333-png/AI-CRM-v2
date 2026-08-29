package membergrid

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestManagementRepositorySQLIsClosedScopedAndCASBound(t *testing.T) {
	itemQueries := map[string]string{
		"get view":            getSavedViewForUpdateSQL,
		"update view":         updateSavedViewSQL,
		"delete view":         deleteSavedViewSQL,
		"get collaborator":    getCollaboratorForUpdateSQL,
		"update collaborator": updateCollaboratorSQL,
		"delete collaborator": deleteCollaboratorSQL,
	}
	for name, query := range itemQueries {
		t.Run(name, func(t *testing.T) {
			lower := strings.ToLower(query)
			if !strings.Contains(lower, "service_product_id = $1") || !strings.Contains(lower, "id = $2") {
				t.Fatalf("item query is not product scoped: %s", query)
			}
			if strings.Contains(name, "update") || strings.Contains(name, "delete") {
				if !strings.Contains(lower, "version = $3") {
					t.Fatalf("mutation is not CAS-bound: %s", query)
				}
			}
			if strings.Contains(lower, "tenant") || strings.Contains(lower, "unionid") || strings.Contains(lower, "external_userid") ||
				strings.Contains(lower, "customer_id") || strings.Contains(lower, "mobile") || strings.Contains(lower, "provider") ||
				strings.Contains(lower, "opaque") {
				t.Fatalf("forbidden identity/tenant/provider source in SQL: %s", query)
			}
		})
	}

	for name, query := range map[string]string{
		"list views": listSavedViewsSQL, "create view": createSavedViewSQL,
		"list collaborators": listCollaboratorsSQL, "create collaborator": createCollaboratorSQL,
	} {
		t.Run(name, func(t *testing.T) {
			lower := strings.ToLower(query)
			if !strings.Contains(lower, "service_product_id") || strings.Contains(lower, "tenant") || strings.Contains(lower, "unionid") ||
				strings.Contains(lower, "external_userid") || strings.Contains(lower, "customer_id") || strings.Contains(lower, "provider") {
				t.Fatalf("unsafe or unscoped SQL: %s", query)
			}
		})
	}

	staffSQL := strings.ToLower(activeStaffExistsSQL)
	if !strings.Contains(staffSQL, "from public.staff") || !strings.Contains(staffSQL, "s.id = $1") ||
		!strings.Contains(staffSQL, "is_active = true") || !strings.Contains(staffSQL, "for share") ||
		strings.Contains(staffSQL, "wecom") || strings.Contains(staffSQL, "provider") {
		t.Fatalf("active staff validation SQL=%s", activeStaffExistsSQL)
	}
	permissionSQL := strings.ToLower(collaboratorPermissionSQL)
	if !strings.Contains(permissionSQL, "service_product_id = $1") || !strings.Contains(permissionSQL, "staff_id = $2") ||
		!strings.Contains(permissionSQL, "is_active = true") || strings.Contains(permissionSQL, "provider") || strings.Contains(permissionSQL, "wecom") {
		t.Fatalf("collaborator permission SQL=%s", collaboratorPermissionSQL)
	}
	for _, query := range []string{reserveManagementReceiptSQL, getManagementReceiptSQL, completeManagementReceiptSQL} {
		if !strings.Contains(query, "public.product_operation_receipts") || strings.Contains(strings.ToLower(query), "external") {
			t.Fatalf("receipt query escaped local Product receipt boundary: %s", query)
		}
	}
	if strings.Contains(strings.ToLower(createSavedViewSQL), "json") || strings.Contains(strings.ToLower(updateSavedViewSQL), "json") {
		t.Fatal("saved view configuration must not use arbitrary JSON storage")
	}
}

func TestManagementRepositoryReadsAndWritesClosedRows(t *testing.T) {
	stamp := time.Date(2026, 8, 22, 3, 4, 5, 0, time.UTC)
	sourceID := int64(4)
	columns := []string{"display_name", "state"}
	executor := &fakeSQLExecutor{
		row: fakeSQLRow{values: []any{
			int64(12), int64(7), "复制视图", "active", "granted_at_desc", columns, &sourceID,
			int64(1), int64(17), stamp, stamp,
		}},
	}
	repository := repositoryForExecutor(executor)
	view, err := repository.CreateSavedView(context.Background(), CreateSavedViewRecord{
		ServiceProductID: 7, Name: "复制视图", State: StateActive, Sort: ViewSortGrantedAtDesc,
		Columns: columns, SourceViewID: &sourceID, CreatedBy: 17, CreatedAt: stamp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != 12 || view.ServiceProductID != 7 || view.SourceViewID == nil || *view.SourceViewID != 4 ||
		!reflect.DeepEqual(view.Columns, columns) || view.Version != 1 {
		t.Fatalf("view=%+v", view)
	}
	wantArgs := []any{int64(7), "复制视图", "active", "granted_at_desc", columns, &sourceID, int64(17), stamp}
	if executor.queryRowSQL != createSavedViewSQL || !reflect.DeepEqual(executor.queryRowArgs, wantArgs) {
		t.Fatalf("sql/args=%q/%#v want=%q/%#v", executor.queryRowSQL, executor.queryRowArgs, createSavedViewSQL, wantArgs)
	}

	collaborator := managementSampleCollaborator(7, 13)
	executor = &fakeSQLExecutor{rows: &fakeSQLRows{rows: [][]any{{
		collaborator.ID, collaborator.ServiceProductID, collaborator.StaffID, string(collaborator.Permission),
		collaborator.Version, collaborator.InvitedBy, collaborator.CreatedAt, collaborator.UpdatedAt,
	}}}}
	repository = repositoryForExecutor(executor)
	items, err := repository.ListCollaborators(context.Background(), 7)
	if err != nil || len(items) != 1 || !reflect.DeepEqual(items[0], collaborator) {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if executor.querySQL != listCollaboratorsSQL || !reflect.DeepEqual(executor.queryArgs, []any{int64(7)}) || !executor.rows.closed {
		t.Fatalf("sql/args/closed=%q/%v/%v", executor.querySQL, executor.queryArgs, executor.rows.closed)
	}

	executor = &fakeSQLExecutor{row: fakeSQLRow{values: []any{true}}}
	repository = repositoryForExecutor(executor)
	active, err := repository.ActiveStaffExists(context.Background(), 29)
	if err != nil || !active || executor.queryRowSQL != activeStaffExistsSQL || !reflect.DeepEqual(executor.queryRowArgs, []any{int64(29)}) {
		t.Fatalf("active/err/sql/args=%v/%v/%q/%v", active, err, executor.queryRowSQL, executor.queryRowArgs)
	}

	executor = &fakeSQLExecutor{row: fakeSQLRow{values: []any{"edit"}}}
	repository = repositoryForExecutor(executor)
	permission, found, err := repository.CollaboratorPermission(context.Background(), 7, 29)
	if err != nil || !found || permission != CollaboratorPermissionEdit || executor.queryRowSQL != collaboratorPermissionSQL ||
		!reflect.DeepEqual(executor.queryRowArgs, []any{int64(7), int64(29)}) {
		t.Fatalf("permission/found/error/sql/args=%q/%v/%v/%q/%v", permission, found, err, executor.queryRowSQL, executor.queryRowArgs)
	}
}

func TestManagementRepositoryMapsNotFoundCASAndUniqueConflicts(t *testing.T) {
	repository := repositoryForExecutor(&fakeSQLExecutor{row: fakeSQLRow{err: pgx.ErrNoRows}})
	if _, err := repository.GetSavedViewForUpdate(context.Background(), 1, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get view error=%v", err)
	}
	if _, err := repository.UpdateSavedView(context.Background(), UpdateSavedViewRecord{
		ServiceProductID: 1, ViewID: 2, ExpectedVersion: 1, Name: "视图", State: StateAll,
		Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, UpdatedAt: time.Now().UTC(),
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("view CAS error=%v", err)
	}
	if _, err := repository.DeleteCollaborator(context.Background(), 1, 2, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("collaborator CAS error=%v", err)
	}

	repository = repositoryForExecutor(&fakeSQLExecutor{row: fakeSQLRow{err: &pgconn.PgError{Code: "23505"}}})
	if _, err := repository.CreateCollaborator(context.Background(), CreateCollaboratorRecord{
		ServiceProductID: 1, StaffID: 2, Permission: CollaboratorPermissionView, InvitedBy: 3, CreatedAt: time.Now().UTC(),
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("unique error=%v", err)
	}

	repository = repositoryForExecutor(&fakeSQLExecutor{row: fakeSQLRow{err: &pgconn.PgError{Code: "23503"}}})
	if _, err := repository.CreateSavedView(context.Background(), CreateSavedViewRecord{
		ServiceProductID: 1, Name: "视图", State: StateAll, Sort: ViewSortGrantedAtDesc,
		Columns: []string{"state"}, CreatedBy: 3, CreatedAt: time.Now().UTC(),
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("foreign key error=%v", err)
	}
}

type sequenceSQLExecutor struct {
	rows         []fakeSQLRow
	queryRowSQL  []string
	queryRowArgs [][]any
}

func (executor *sequenceSQLExecutor) QueryRow(_ context.Context, sql string, arguments ...any) sqlRow {
	executor.queryRowSQL = append(executor.queryRowSQL, sql)
	executor.queryRowArgs = append(executor.queryRowArgs, append([]any(nil), arguments...))
	if len(executor.rows) == 0 {
		return fakeSQLRow{err: errors.New("unexpected QueryRow")}
	}
	row := executor.rows[0]
	executor.rows = executor.rows[1:]
	return row
}

func (*sequenceSQLExecutor) Query(context.Context, string, ...any) (sqlRows, error) {
	return nil, errors.New("unexpected Query")
}

func TestManagementRepositoryReceiptReservationReplayAndCompletion(t *testing.T) {
	now := time.Date(2026, 8, 22, 4, 5, 6, 0, time.UTC)
	reservation := MutationReceiptReservation{
		Operation: mutationOperationCreate, ActorScope: "membergrid:member_view.created:actor:17",
		KeyDigest: sha256.Sum256([]byte("repository-key-0001")), PayloadDigest: sha256.Sum256([]byte(`{"name":"x"}`)),
		CreatedAt: now,
	}
	ownedExecutor := &sequenceSQLExecutor{rows: []fakeSQLRow{{values: []any{
		int64(8), reservation.Operation, reservation.ActorScope, reservation.KeyDigest[:], reservation.PayloadDigest[:],
		"in_progress", []byte(nil),
	}}}}
	repository := repositoryForExecutor(ownedExecutor)
	receipt, owned, err := repository.ReserveMutationReceipt(context.Background(), reservation)
	if err != nil || !owned || receipt.ID != 8 || receipt.State != "in_progress" || receipt.KeyDigest != reservation.KeyDigest || receipt.PayloadDigest != reservation.PayloadDigest {
		t.Fatalf("receipt/owned/err=%+v/%v/%v", receipt, owned, err)
	}
	if len(ownedExecutor.queryRowSQL) != 1 || ownedExecutor.queryRowSQL[0] != reserveManagementReceiptSQL {
		t.Fatalf("owned SQL=%v", ownedExecutor.queryRowSQL)
	}
	wantReserveArgs := []any{reservation.Operation, reservation.ActorScope, reservation.KeyDigest[:], reservation.PayloadDigest[:], now}
	if !reflect.DeepEqual(ownedExecutor.queryRowArgs[0], wantReserveArgs) {
		t.Fatalf("reserve args=%#v want=%#v", ownedExecutor.queryRowArgs[0], wantReserveArgs)
	}

	snapshot := json.RawMessage(`{"kind":"member_view.created","status":201}`)
	replayExecutor := &sequenceSQLExecutor{rows: []fakeSQLRow{
		{err: pgx.ErrNoRows},
		{values: []any{int64(8), reservation.Operation, reservation.ActorScope, reservation.KeyDigest[:], reservation.PayloadDigest[:], "completed", []byte(snapshot)}},
	}}
	repository = repositoryForExecutor(replayExecutor)
	receipt, owned, err = repository.ReserveMutationReceipt(context.Background(), reservation)
	if err != nil || owned || receipt.State != "completed" || !reflect.DeepEqual(receipt.ResultSnapshot, snapshot) {
		t.Fatalf("replay receipt/owned/err=%+v/%v/%v", receipt, owned, err)
	}
	if !reflect.DeepEqual(replayExecutor.queryRowSQL, []string{reserveManagementReceiptSQL, getManagementReceiptSQL}) {
		t.Fatalf("replay SQL=%v", replayExecutor.queryRowSQL)
	}

	completionExecutor := &sequenceSQLExecutor{rows: []fakeSQLRow{{values: []any{
		int64(8), reservation.Operation, reservation.ActorScope, reservation.KeyDigest[:], reservation.PayloadDigest[:], "completed", []byte(snapshot),
	}}}}
	repository = repositoryForExecutor(completionExecutor)
	completed, err := repository.CompleteMutationReceipt(context.Background(), 8, snapshot, now.Add(time.Minute))
	if err != nil || completed.State != "completed" || !reflect.DeepEqual(completed.ResultSnapshot, snapshot) {
		t.Fatalf("completed/err=%+v/%v", completed, err)
	}
	if completionExecutor.queryRowSQL[0] != completeManagementReceiptSQL || !reflect.DeepEqual(completionExecutor.queryRowArgs[0], []any{int64(8), snapshot, now.Add(time.Minute)}) {
		t.Fatalf("completion sql/args=%q/%#v", completionExecutor.queryRowSQL[0], completionExecutor.queryRowArgs[0])
	}

	repository = repositoryForExecutor(&fakeSQLExecutor{row: fakeSQLRow{err: pgx.ErrNoRows}})
	if _, err := repository.CompleteMutationReceipt(context.Background(), 8, snapshot, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("completion CAS error=%v", err)
	}
}

func TestManagementRepositoryRejectsMalformedRowsAndInvalidInputs(t *testing.T) {
	stamp := time.Now().UTC()
	executor := &fakeSQLExecutor{row: fakeSQLRow{values: []any{
		int64(1), int64(2), "视图", "active", "granted_at_desc", []string{"customer_id"}, (*int64)(nil),
		int64(1), int64(3), stamp, stamp,
	}}}
	repository := repositoryForExecutor(executor)
	if _, err := repository.GetSavedViewForUpdate(context.Background(), 2, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsafe row error=%v", err)
	}

	badDigestExecutor := &sequenceSQLExecutor{rows: []fakeSQLRow{{values: []any{
		int64(1), mutationOperationCreate, "scope", []byte{1}, make([]byte, 32), "in_progress", []byte(nil),
	}}}}
	repository = repositoryForExecutor(badDigestExecutor)
	reservation := MutationReceiptReservation{
		Operation: mutationOperationCreate, ActorScope: "scope", KeyDigest: sha256.Sum256([]byte("repository-key-0002")),
		PayloadDigest: sha256.Sum256([]byte(`{}`)), CreatedAt: stamp,
	}
	if _, _, err := repository.ReserveMutationReceipt(context.Background(), reservation); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("digest error=%v", err)
	}

	if _, err := repository.ActiveStaffExists(context.Background(), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("invalid staff error=%v", err)
	}
	if _, err := repository.DeleteSavedView(context.Background(), 1, 0, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("invalid view delete error=%v", err)
	}
	if _, err := repository.DeleteCollaborator(context.Background(), 1, 1, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("invalid collaborator delete error=%v", err)
	}
}
