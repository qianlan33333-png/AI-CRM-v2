package membergrid

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

func TestExternalShareRepositoryReadsWritesAndFailsClosed(t *testing.T) {
	shareID := "share_abcdefghijklmnopqrstuv"
	queries := &fakeShareQueries{currentErr: pgx.ErrNoRows}
	repository := repositoryForShareQueries(queries)
	current, err := repository.CurrentExternalShare(context.Background(), 8)
	if err != nil || current != (ExternalShare{ServiceProductID: 8, Version: 0}) || queries.currentID != 8 {
		t.Fatalf("current/err/id=%+v/%v/%d", current, err, queries.currentID)
	}

	queries = &fakeShareQueries{setRow: productdb.SetMemberGridExternalShareRow{ServiceProductID: 8, ShareID: shareID, Enabled: true, Version: 1}}
	repository = repositoryForShareQueries(queries)
	updated, err := repository.SetExternalShare(context.Background(), SetExternalShareRecord{
		ServiceProductID: 8, Enabled: true, ShareID: shareID, ExpectedVersion: 0, ActorID: 3, IdempotencyKey: "external-share-enable-0001",
	})
	if err != nil || updated != (ExternalShare{ServiceProductID: 8, ShareID: shareID, Enabled: true, Version: 1}) {
		t.Fatalf("updated/err=%+v/%v", updated, err)
	}
	wantSetArgs := productdb.SetMemberGridExternalShareParams{ServiceProductID: 8, ShareID: shareID, Enabled: true, UpdatedBy: 3, ExpectedVersion: 0}
	if !reflect.DeepEqual(queries.setParams, wantSetArgs) {
		t.Fatalf("set args=%#v want=%#v", queries.setParams, wantSetArgs)
	}

	queries = &fakeShareQueries{lookupRow: productdb.LookupEnabledMemberGridExternalShareRow{ServiceProductID: 8, ShareID: shareID, Enabled: true, Version: 1}}
	repository = repositoryForShareQueries(queries)
	resolved, err := repository.LookupEnabledExternalShare(context.Background(), shareID)
	if err != nil || resolved != updated || queries.lookupID != shareID {
		t.Fatalf("resolved/err/id=%+v/%v/%q", resolved, err, queries.lookupID)
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
	repository := repositoryForShareQueries(&fakeShareQueries{setErr: pgx.ErrNoRows, lookupErr: pgx.ErrNoRows})
	if _, err := repository.SetExternalShare(context.Background(), SetExternalShareRecord{
		ServiceProductID: 8, Enabled: false, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-disable-001",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("CAS err=%v", err)
	}
	if _, err := repository.LookupEnabledExternalShare(context.Background(), shareID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lookup err=%v", err)
	}

	repository = repositoryForShareQueries(&fakeShareQueries{lookupRow: productdb.LookupEnabledMemberGridExternalShareRow{ServiceProductID: 8, ShareID: shareID, Enabled: false, Version: 2}})
	if _, err := repository.LookupEnabledExternalShare(context.Background(), shareID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled row leaked through public lookup: %v", err)
	}
	repository = repositoryForShareQueries(&fakeShareQueries{currentRow: productdb.CurrentMemberGridExternalShareRow{ServiceProductID: 8, ShareID: "", Enabled: true, Version: 2}})
	if _, err := repository.CurrentExternalShare(context.Background(), 8); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("malformed row err=%v", err)
	}
}
