package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/compiler"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestRefreshServiceReplacesMembersOnceInsideUnitOfWork(t *testing.T) {
	reference := time.Date(2026, 8, 13, 2, 3, 4, 0, time.UTC)
	order := []string{}
	queries := &refreshQuerySet{order: &order, universe: []int64{5, 3, 1}, deleted: []int64{5, 1}}
	store := &refreshStore{
		definition: []byte(`{"field":"is_deleted","op":"eq","value":false}`),
		queries:    queries,
		order:      &order,
	}
	uow := &refreshUnitOfWork{order: &order}
	events := &refreshEvents{order: &order}
	service := NewRefreshService(uow, store, events)

	result, err := service.RefreshOnce(context.Background(), segmentport.SegmentID(42), reference)
	if err != nil {
		t.Fatalf("RefreshOnce() error = %v", err)
	}
	if result.SegmentID != 42 || result.MemberCount != 2 || !result.RefreshedAt.Equal(reference) {
		t.Fatalf("RefreshOnce() result = %#v", result)
	}
	if !reflect.DeepEqual(store.replaced, []int64{1, 5}) {
		t.Fatalf("replaced members = %v, want [1 5]", store.replaced)
	}
	if store.replacedAt != reference {
		t.Fatalf("replacement instant = %v, want %v", store.replacedAt, reference)
	}
	wantOrder := []string{"begin", "lock", "query-set", "universe", "deleted", "replace", "event", "commit"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("order = %v, want %v", order, wantOrder)
	}
	if events.event.Type != "segment.refreshed" || events.event.IdempotencyKey != "segment.refresh:42:2026-08-13T02:03:04Z" || string(events.event.Payload) != `{"segment_id":42,"member_count":2}` {
		t.Fatalf("event = %#v, want stable refresh fact", events.event)
	}
}

func TestRefreshServiceFailsClosedBeforeReplacement(t *testing.T) {
	reference := time.Date(2026, 8, 13, 2, 3, 4, 0, time.UTC)
	tests := []struct {
		name       string
		segmentID  segmentport.SegmentID
		reference  time.Time
		definition []byte
		queryErr   error
		want       error
	}{
		{name: "invalid segment", segmentID: 0, reference: reference, want: ErrInvalidSegmentRefresh},
		{name: "non UTC reference", segmentID: 42, reference: reference.In(time.FixedZone("offset", 3600)), want: ErrInvalidSegmentRefresh},
		{name: "unknown DSL field", segmentID: 42, reference: reference, definition: []byte(`{"field":"stage_id) OR true --","op":"eq","value":1}`), want: ErrSegmentRefreshFailed},
		{name: "query failure", segmentID: 42, reference: reference, definition: []byte(`{"field":"is_deleted","op":"eq","value":false}`), queryErr: errors.New("query failed"), want: ErrSegmentRefreshFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			queries := &refreshQuerySet{order: &order, universe: []int64{1}, queryErr: test.queryErr}
			store := &refreshStore{definition: test.definition, queries: queries, order: &order}
			service := NewRefreshService(&refreshUnitOfWork{order: &order}, store, &refreshEvents{order: &order})

			_, err := service.RefreshOnce(context.Background(), test.segmentID, test.reference)
			if !errors.Is(err, test.want) {
				t.Fatalf("RefreshOnce() error = %v, want %v", err, test.want)
			}
			if store.replaceCalls != 0 {
				t.Fatalf("ReplaceMembers() calls = %d, want 0", store.replaceCalls)
			}
		})
	}
}

func TestRefreshServiceRejectsMissingDependencies(t *testing.T) {
	reference := time.Date(2026, 8, 13, 2, 3, 4, 0, time.UTC)
	store := &refreshStore{}
	for _, service := range []*RefreshService{
		nil,
		NewRefreshService(nil, store, &refreshEvents{}),
		NewRefreshService(&refreshUnitOfWork{}, nil, &refreshEvents{}),
		NewRefreshService(&refreshUnitOfWork{}, store, nil),
	} {
		if _, err := service.RefreshOnce(context.Background(), 42, reference); !errors.Is(err, ErrSegmentRefreshFailed) {
			t.Fatalf("RefreshOnce() error = %v, want dependency failure", err)
		}
	}
}

type refreshUnitOfWork struct{ order *[]string }

func (uow *refreshUnitOfWork) Within(ctx context.Context, callback func(context.Context) error) error {
	if uow.order != nil {
		*uow.order = append(*uow.order, "begin")
	}
	err := callback(ctx)
	if uow.order != nil {
		if err == nil {
			*uow.order = append(*uow.order, "commit")
		} else {
			*uow.order = append(*uow.order, "rollback")
		}
	}
	return err
}

var _ platformport.UnitOfWork = (*refreshUnitOfWork)(nil)

type refreshStore struct {
	definition   []byte
	queries      compiler.QuerySet
	order        *[]string
	replaced     []int64
	replacedAt   time.Time
	replaceCalls int
}

func (store *refreshStore) LockDefinition(_ context.Context, _ segmentport.SegmentID) ([]byte, error) {
	if store.order != nil {
		*store.order = append(*store.order, "lock")
	}
	return append([]byte(nil), store.definition...), nil
}

func (store *refreshStore) QuerySet(_ context.Context) (compiler.QuerySet, error) {
	if store.order != nil {
		*store.order = append(*store.order, "query-set")
	}
	return store.queries, nil
}

func (store *refreshStore) ReplaceMembers(_ context.Context, _ segmentport.SegmentID, ids []int64, refreshedAt time.Time) error {
	store.replaceCalls++
	store.replaced = append([]int64(nil), ids...)
	store.replacedAt = refreshedAt
	if store.order != nil {
		*store.order = append(*store.order, "replace")
	}
	return nil
}

type refreshQuerySet struct {
	compiler.QuerySet
	order    *[]string
	universe []int64
	deleted  []int64
	queryErr error
}

type refreshEvents struct {
	order *[]string
	event eventport.Event
	err   error
}

func (events *refreshEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	if events.order != nil {
		*events.order = append(*events.order, "event")
	}
	events.event = event
	return 1, events.err
}

func (queries *refreshQuerySet) Universe(context.Context) ([]int64, error) {
	if queries.order != nil {
		*queries.order = append(*queries.order, "universe")
	}
	if queries.queryErr != nil {
		return nil, queries.queryErr
	}
	return append([]int64(nil), queries.universe...), nil
}

func (queries *refreshQuerySet) DeletedEqual(context.Context, bool) ([]int64, error) {
	if queries.order != nil {
		*queries.order = append(*queries.order, "deleted")
	}
	if queries.queryErr != nil {
		return nil, queries.queryErr
	}
	return append([]int64(nil), queries.deleted...), nil
}
