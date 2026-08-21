package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
)

var errExternalEffectsTaskStoreFixture = errors.New("external effects task-store fixture failure")

type externalEffectsTaskStoreFixture struct {
	list    func(outboundapp.TaskListQuery) ([]outboundapp.TaskReadModel, error)
	queries []outboundapp.TaskListQuery
}

func (*externalEffectsTaskStoreFixture) GetTask(context.Context, outboundapp.TaskGetQuery) (outboundapp.TaskReadModel, error) {
	return outboundapp.TaskReadModel{}, errExternalEffectsTaskStoreFixture
}

func (store *externalEffectsTaskStoreFixture) ListTasks(
	_ context.Context,
	query outboundapp.TaskListQuery,
) ([]outboundapp.TaskReadModel, error) {
	store.queries = append(store.queries, query)
	if store.list == nil {
		return nil, nil
	}
	return store.list(query)
}

func (*externalEffectsTaskStoreFixture) ListAttempts(context.Context, outboundapp.TaskID) ([]outboundapp.AttemptReadModel, error) {
	return nil, errExternalEffectsTaskStoreFixture
}

func (*externalEffectsTaskStoreFixture) ListControlReceipts(context.Context, outboundapp.TaskID) ([]outboundapp.ControlReceiptReadModel, error) {
	return nil, errExternalEffectsTaskStoreFixture
}

func TestExternalEffectsRepositoryProjectsOnlyApprovedLocalFacts(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, time.August, 21, 4, 0, 0, 0, time.FixedZone("fixture", 8*60*60))
	ownerID := int64(771)
	batchID := int64(772)
	store := &externalEffectsTaskStoreFixture{list: func(query outboundapp.TaskListQuery) ([]outboundapp.TaskReadModel, error) {
		return []outboundapp.TaskReadModel{{
			TaskID: 9, CustomerID: 991, OwnerStaffID: &ownerID, BatchID: &batchID,
			Status: outboundapp.TaskStatusOutcomeUnknown, AttemptCount: 2,
			LastFailureKind: "provider_failure_secret", LastError: "raw provider error secret",
			ProviderMessageID: "provider-message-id-secret",
			Job:               outboundapp.TaskJob{TaskID: 9, Generation: 3, RiverJobID: 8844, JobKind: "outbound_enqueue_one"},
			CreatedAt:         created, StatusUpdatedAt: created.Add(time.Minute),
		}}, nil
	}}
	repository, err := NewExternalEffectsRepository(store)
	if err != nil {
		t.Fatalf("NewExternalEffectsRepository() error = %v", err)
	}

	got, err := repository.ListExternalEffectSources(context.Background(), outboundapp.ExternalEffectStoreQuery{
		Status: outboundapp.TaskStatusOutcomeUnknown, Offset: 4, Limit: 7,
	})
	if err != nil {
		t.Fatalf("ListExternalEffectSources() error = %v", err)
	}
	if len(got) != 1 || got[0].TaskID != 9 || got[0].Status != outboundapp.TaskStatusOutcomeUnknown ||
		got[0].AttemptCount != 2 || got[0].CreatedAt.Location() != time.UTC ||
		got[0].StatusUpdatedAt.Location() != time.UTC {
		t.Fatalf("sources = %+v", got)
	}
	if len(store.queries) != 1 || store.queries[0].Status != outboundapp.TaskStatusOutcomeUnknown ||
		store.queries[0].Offset != 4 || store.queries[0].Limit != 7 ||
		store.queries[0].OwnerStaffID != nil || store.queries[0].BatchID != nil {
		t.Fatalf("delegated query = %+v", store.queries)
	}
	projected := fmt.Sprintf("%+v", got[0])
	for _, secret := range []string{
		"991", "771", "772", "provider_failure_secret", "raw provider error secret",
		"provider-message-id-secret", "8844", "outbound_enqueue_one",
	} {
		if strings.Contains(projected, secret) {
			t.Fatalf("safe source leaked %q: %s", secret, projected)
		}
	}
	fields := reflect.VisibleFields(reflect.TypeOf(outboundapp.ExternalEffectSource{}))
	if len(fields) != 5 {
		t.Fatalf("ExternalEffectSource field count = %d, want 5", len(fields))
	}
}

func TestExternalEffectsRepositoryCountsAllClosedStatusesAcrossPages(t *testing.T) {
	t.Parallel()

	statuses := []outboundapp.TaskStatus{
		outboundapp.TaskStatusPending,
		outboundapp.TaskStatusSending,
		outboundapp.TaskStatusSent,
		outboundapp.TaskStatusRetryableFailed,
		outboundapp.TaskStatusFinalFailed,
		outboundapp.TaskStatusOutcomeUnknown,
		outboundapp.TaskStatusCancelled,
	}
	created := time.Date(2026, time.August, 21, 5, 0, 0, 0, time.UTC)
	models := make([]outboundapp.TaskReadModel, 101)
	var want outboundapp.ExternalEffectStatusCounts
	for index := range models {
		status := statuses[index%len(statuses)]
		models[index] = outboundapp.TaskReadModel{
			TaskID: outboundapp.TaskID(1000 - index), Status: status,
			CreatedAt: created, StatusUpdatedAt: created.Add(time.Minute),
		}
		switch status {
		case outboundapp.TaskStatusPending:
			want.Pending++
		case outboundapp.TaskStatusSending:
			want.Sending++
		case outboundapp.TaskStatusSent:
			want.Sent++
		case outboundapp.TaskStatusRetryableFailed:
			want.RetryableFailed++
		case outboundapp.TaskStatusFinalFailed:
			want.FinalFailed++
		case outboundapp.TaskStatusOutcomeUnknown:
			want.OutcomeUnknown++
		case outboundapp.TaskStatusCancelled:
			want.Cancelled++
		}
	}
	store := &externalEffectsTaskStoreFixture{list: func(query outboundapp.TaskListQuery) ([]outboundapp.TaskReadModel, error) {
		start := int(query.Offset)
		if start >= len(models) {
			return nil, nil
		}
		end := start + int(query.Limit)
		if end > len(models) {
			end = len(models)
		}
		return append([]outboundapp.TaskReadModel(nil), models[start:end]...), nil
	}}
	repository, err := NewExternalEffectsRepository(store)
	if err != nil {
		t.Fatalf("NewExternalEffectsRepository() error = %v", err)
	}

	got, err := repository.CountExternalEffectStatuses(context.Background())
	if err != nil {
		t.Fatalf("CountExternalEffectStatuses() error = %v", err)
	}
	if got != want || got.Total() != 101 {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
	if len(store.queries) != 2 || store.queries[0].Offset != 0 || store.queries[1].Offset != 100 {
		t.Fatalf("pagination queries = %+v", store.queries)
	}
	for _, query := range store.queries {
		if query.Status != "" || query.Limit != outboundapp.TaskQueryMaximumLimit ||
			query.OwnerStaffID != nil || query.BatchID != nil {
			t.Fatalf("unsafe count query = %+v", query)
		}
	}
}

func TestExternalEffectsRepositoryFailsClosedForInvalidPortFacts(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, time.August, 21, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		models []outboundapp.TaskReadModel
	}{
		{name: "unknown status", models: []outboundapp.TaskReadModel{{TaskID: 3, Status: "accepted", CreatedAt: created, StatusUpdatedAt: created}}},
		{name: "ascending ids", models: []outboundapp.TaskReadModel{
			{TaskID: 2, Status: outboundapp.TaskStatusPending, CreatedAt: created, StatusUpdatedAt: created},
			{TaskID: 3, Status: outboundapp.TaskStatusPending, CreatedAt: created, StatusUpdatedAt: created},
		}},
		{name: "invalid timestamps", models: []outboundapp.TaskReadModel{{
			TaskID: 3, Status: outboundapp.TaskStatusPending, CreatedAt: created, StatusUpdatedAt: created.Add(-time.Second),
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &externalEffectsTaskStoreFixture{list: func(outboundapp.TaskListQuery) ([]outboundapp.TaskReadModel, error) {
				return append([]outboundapp.TaskReadModel(nil), test.models...), nil
			}}
			repository, err := NewExternalEffectsRepository(store)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = repository.CountExternalEffectStatuses(context.Background()); err == nil {
				t.Fatal("expected closed count failure")
			}
		})
	}
}

func TestExternalEffectsRepositoryRejectsInvalidQueriesAndPortOverrun(t *testing.T) {
	t.Parallel()

	store := &externalEffectsTaskStoreFixture{list: func(query outboundapp.TaskListQuery) ([]outboundapp.TaskReadModel, error) {
		if query.Offset == 7 {
			return nil, errExternalEffectsTaskStoreFixture
		}
		return make([]outboundapp.TaskReadModel, query.Limit+1), nil
	}}
	repository, err := NewExternalEffectsRepository(store)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []outboundapp.ExternalEffectStoreQuery{
		{Limit: 0},
		{Limit: outboundapp.TaskQueryMaximumLimit + 1},
		{Limit: 1, Offset: -1},
		{Limit: 1, Status: "queued"},
	} {
		if _, err = repository.ListExternalEffectSources(context.Background(), query); !errors.Is(err, outboundapp.ErrInvalidExternalEffectsQuery) {
			t.Fatalf("query %+v error = %v", query, err)
		}
	}
	if _, err = repository.ListExternalEffectSources(context.Background(), outboundapp.ExternalEffectStoreQuery{Limit: 1}); err == nil {
		t.Fatal("expected task port overrun failure")
	}
	if _, err = repository.ListExternalEffectSources(context.Background(), outboundapp.ExternalEffectStoreQuery{Limit: 1, Offset: 7}); !errors.Is(err, errExternalEffectsTaskStoreFixture) {
		t.Fatalf("task port error = %v", err)
	}
}

func TestExternalEffectsRepositoryHasNoWriteSurfaceAndRejectsTypedNil(t *testing.T) {
	t.Parallel()

	var typedNil *externalEffectsTaskStoreFixture
	if _, err := NewExternalEffectsRepository(typedNil); !errors.Is(err, outboundapp.ErrInvalidExternalEffectsConfiguration) {
		t.Fatalf("typed nil constructor error = %v", err)
	}
	methods := reflect.TypeOf(&ExternalEffectsRepository{})
	if methods.NumMethod() != 2 || methods.Method(0).Name != "CountExternalEffectStatuses" ||
		methods.Method(1).Name != "ListExternalEffectSources" {
		t.Fatalf("repository method surface = %v", methods)
	}
}
