package membergrid

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestExternalShareRepositoryUsesOneScopedCASRow(t *testing.T) {
	for name, query := range map[string]string{
		"current": currentExternalShareSQL,
		"set":     setExternalShareSQL,
		"lookup":  lookupEnabledExternalShareSQL,
	} {
		t.Run(name, func(t *testing.T) {
			lower := strings.ToLower(query)
			if !strings.Contains(lower, "public.service_period_member_grid_external_shares") || strings.Contains(lower, "tenant") ||
				strings.Contains(lower, "customer_id") || strings.Contains(lower, "unionid") || strings.Contains(lower, "external_userid") ||
				strings.Contains(lower, "mobile") || strings.Contains(lower, "provider") || strings.Contains(lower, "json") {
				t.Fatalf("unsafe external share SQL: %s", query)
			}
		})
	}
	set := strings.ToLower(setExternalShareSQL)
	if !strings.Contains(set, "on conflict (service_product_id)") || !strings.Contains(set, "where s.version = $5") ||
		!strings.Contains(set, "nullif($2, '')") || !strings.Contains(set, "version = s.version + 1") {
		t.Fatalf("set external share is not CAS/clear-id bound: %s", setExternalShareSQL)
	}
	lookup := strings.ToLower(lookupEnabledExternalShareSQL)
	if !strings.Contains(lookup, "s.share_id = $1") || !strings.Contains(lookup, "s.enabled = true") {
		t.Fatalf("public lookup is not live-enabled scoped: %s", lookupEnabledExternalShareSQL)
	}
}

func TestExternalShareRepositoryReadsWritesAndFailsClosed(t *testing.T) {
	shareID := "share_abcdefghijklmnopqrstuv"
	executor := &fakeSQLExecutor{row: fakeSQLRow{err: pgx.ErrNoRows}}
	repository := repositoryForExecutor(executor)
	current, err := repository.CurrentExternalShare(context.Background(), 8)
	if err != nil || current != (ExternalShare{ServiceProductID: 8, Version: 0}) || executor.queryRowSQL != currentExternalShareSQL || !reflect.DeepEqual(executor.queryRowArgs, []any{int64(8)}) {
		t.Fatalf("current/err/sql/args=%+v/%v/%q/%v", current, err, executor.queryRowSQL, executor.queryRowArgs)
	}

	executor = &fakeSQLExecutor{row: fakeSQLRow{values: []any{int64(8), shareID, true, int64(1)}}}
	repository = repositoryForExecutor(executor)
	updated, err := repository.SetExternalShare(context.Background(), SetExternalShareRecord{
		ServiceProductID: 8, Enabled: true, ShareID: shareID, ExpectedVersion: 0, ActorID: 3, IdempotencyKey: "external-share-enable-0001",
	})
	if err != nil || updated != (ExternalShare{ServiceProductID: 8, ShareID: shareID, Enabled: true, Version: 1}) {
		t.Fatalf("updated/err=%+v/%v", updated, err)
	}
	wantSetArgs := []any{int64(8), shareID, true, int64(3), int64(0)}
	if executor.queryRowSQL != setExternalShareSQL || !reflect.DeepEqual(executor.queryRowArgs, wantSetArgs) {
		t.Fatalf("set sql/args=%q/%#v want=%q/%#v", executor.queryRowSQL, executor.queryRowArgs, setExternalShareSQL, wantSetArgs)
	}

	executor = &fakeSQLExecutor{row: fakeSQLRow{values: []any{int64(8), shareID, true, int64(1)}}}
	repository = repositoryForExecutor(executor)
	resolved, err := repository.LookupEnabledExternalShare(context.Background(), shareID)
	if err != nil || resolved != updated || executor.queryRowSQL != lookupEnabledExternalShareSQL || !reflect.DeepEqual(executor.queryRowArgs, []any{shareID}) {
		t.Fatalf("resolved/err/sql/args=%+v/%v/%q/%v", resolved, err, executor.queryRowSQL, executor.queryRowArgs)
	}

	for name, record := range map[string]SetExternalShareRecord{
		"disabled retains id": {ServiceProductID: 8, Enabled: false, ShareID: shareID, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-disable-001"},
		"enabled empty id":    {ServiceProductID: 8, Enabled: true, ShareID: "", ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-enable-0002"},
		"invalid key":         {ServiceProductID: 8, Enabled: false, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "short"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := repository.SetExternalShare(context.Background(), record); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("record=%+v err=%v", record, err)
			}
		})
	}
}

func TestExternalShareRepositoryMapsCASLookupAndMalformedRows(t *testing.T) {
	shareID := "share_abcdefghijklmnopqrstuv"
	repository := repositoryForExecutor(&fakeSQLExecutor{row: fakeSQLRow{err: pgx.ErrNoRows}})
	if _, err := repository.SetExternalShare(context.Background(), SetExternalShareRecord{
		ServiceProductID: 8, Enabled: false, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-disable-001",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("CAS err=%v", err)
	}
	if _, err := repository.LookupEnabledExternalShare(context.Background(), shareID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lookup err=%v", err)
	}

	repository = repositoryForExecutor(&fakeSQLExecutor{row: fakeSQLRow{values: []any{int64(8), shareID, false, int64(2)}}})
	if _, err := repository.LookupEnabledExternalShare(context.Background(), shareID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled row leaked through public lookup: %v", err)
	}
	repository = repositoryForExecutor(&fakeSQLExecutor{row: fakeSQLRow{values: []any{int64(8), "", true, int64(2)}}})
	if _, err := repository.CurrentExternalShare(context.Background(), 8); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("malformed row err=%v", err)
	}
}
