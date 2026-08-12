package app

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type tagCatalogAttemptKey struct{}
type tagCatalogParentKey struct{}

type tagCatalogTestUoW struct {
	calls     int
	callbacks int
	attempts  int
	err       error
}

func (uow *tagCatalogTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	attempts := uow.attempts
	if attempts == 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		uow.callbacks++
		if err := callback(context.WithValue(ctx, tagCatalogAttemptKey{}, attempt)); err != nil {
			return err
		}
	}
	return nil
}

type tagCatalogTestStore struct {
	result []TagCatalogRecord
	err    error

	calls        int
	attempts     []int
	parentValues []string
}

func (store *tagCatalogTestStore) ListTags(ctx context.Context) ([]TagCatalogRecord, error) {
	store.calls++
	attempt, _ := ctx.Value(tagCatalogAttemptKey{}).(int)
	store.attempts = append(store.attempts, attempt)
	parent, _ := ctx.Value(tagCatalogParentKey{}).(string)
	store.parentValues = append(store.parentValues, parent)
	return store.result, store.err
}

func tagCatalogInt64(value int64) *int64    { return &value }
func tagCatalogInt32(value int32) *int32    { return &value }
func tagCatalogString(value string) *string { return &value }

func tagCatalogGrouped(id, groupID int64, groupName string, groupSort, sort int32, name string) TagCatalogRecord {
	return TagCatalogRecord{
		ID: id, GroupID: tagCatalogInt64(groupID), GroupName: tagCatalogString(groupName),
		GroupSortOrder: tagCatalogInt32(groupSort), Name: name, SortOrder: sort,
	}
}

func tagCatalogUngrouped(id int64, sort int32, name string) TagCatalogRecord {
	return TagCatalogRecord{ID: id, Name: name, SortOrder: sort}
}

func tagCatalogRequireUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrTagCatalogUnavailable) {
		t.Fatalf("List() error = %v, expected tag catalog unavailable", err)
	}
}

func tagCatalogRequireNoCalls(t *testing.T, uow *tagCatalogTestUoW, store *tagCatalogTestStore) {
	t.Helper()
	if uow.calls != 0 || uow.callbacks != 0 || store.calls != 0 {
		t.Fatalf("unexpected collaborator calls: unit=%d callbacks=%d store=%d", uow.calls, uow.callbacks, store.calls)
	}
}

func TestP3C02ETagCatalogServiceReadsStableCatalog(t *testing.T) {
	records := []TagCatalogRecord{
		tagCatalogGrouped(11, 7, "lifecycle", -2, -1, strings.Repeat("界", 200)),
		tagCatalogGrouped(12, 7, "lifecycle", -2, 0, "new"),
		tagCatalogGrouped(13, 8, "source", -2, math.MaxInt32, "referral"),
		tagCatalogGrouped(14, 9, "status", math.MaxInt32, math.MinInt32, "active"),
		tagCatalogUngrouped(15, math.MinInt32, "priority"),
		tagCatalogUngrouped(16, math.MaxInt32, "vip"),
	}
	uow := &tagCatalogTestUoW{}
	store := &tagCatalogTestStore{result: records}
	ctx := context.WithValue(context.Background(), tagCatalogParentKey{}, "request-marker")

	got, err := NewTagCatalogService(uow, store).List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !reflect.DeepEqual(got, records) {
		t.Fatalf("List() = %#v, expected %#v", got, records)
	}
	if uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
		t.Fatalf("calls = unit:%d callbacks:%d store:%d, expected one transaction read", uow.calls, uow.callbacks, store.calls)
	}
	if !reflect.DeepEqual(store.attempts, []int{1}) || !reflect.DeepEqual(store.parentValues, []string{"request-marker"}) {
		t.Fatalf("transaction context = attempts:%v parent:%v", store.attempts, store.parentValues)
	}
}

func TestP3C02ETagCatalogServiceReturnsNonNilEmptyCatalog(t *testing.T) {
	uow := &tagCatalogTestUoW{}
	store := &tagCatalogTestStore{result: []TagCatalogRecord{}}

	got, err := NewTagCatalogService(uow, store).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("List() = %#v, expected a non-nil empty catalog", got)
	}
	if uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
		t.Fatalf("calls = unit:%d callbacks:%d store:%d", uow.calls, uow.callbacks, store.calls)
	}
}

func TestP3C02ETagCatalogServiceRejectsContextBeforeTransaction(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name      string
		ctx       context.Context
		wantCause error
	}{
		{name: "nil context"},
		{name: "cancelled context", ctx: cancelled, wantCause: context.Canceled},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store := &tagCatalogTestUoW{}, &tagCatalogTestStore{}
			got, err := NewTagCatalogService(uow, store).List(testCase.ctx)
			tagCatalogRequireUnavailable(t, err)
			if testCase.wantCause != nil && !errors.Is(err, testCase.wantCause) {
				t.Fatalf("List() error = %v, expected cause %v", err, testCase.wantCause)
			}
			if got != nil {
				t.Fatalf("List() = %#v, expected no partial catalog", got)
			}
			tagCatalogRequireNoCalls(t, uow, store)
		})
	}
}

func TestP3C02ETagCatalogServiceFailsClosedWithoutDependencies(t *testing.T) {
	tests := []struct {
		name    string
		service func(*tagCatalogTestUoW, *tagCatalogTestStore) *TagCatalogService
	}{
		{name: "nil receiver", service: func(*tagCatalogTestUoW, *tagCatalogTestStore) *TagCatalogService { return nil }},
		{name: "missing unit of work", service: func(_ *tagCatalogTestUoW, store *tagCatalogTestStore) *TagCatalogService {
			return &TagCatalogService{store: store}
		}},
		{name: "typed nil unit of work", service: func(_ *tagCatalogTestUoW, store *tagCatalogTestStore) *TagCatalogService {
			var typedNil *tagCatalogTestUoW
			var dependency platformport.UnitOfWork = typedNil
			return &TagCatalogService{uow: dependency, store: store}
		}},
		{name: "missing store", service: func(uow *tagCatalogTestUoW, _ *tagCatalogTestStore) *TagCatalogService {
			return &TagCatalogService{uow: uow}
		}},
		{name: "typed nil store", service: func(uow *tagCatalogTestUoW, _ *tagCatalogTestStore) *TagCatalogService {
			var typedNil *tagCatalogTestStore
			var dependency TagCatalogStore = typedNil
			return &TagCatalogService{uow: uow, store: dependency}
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store := &tagCatalogTestUoW{}, &tagCatalogTestStore{}
			got, err := testCase.service(uow, store).List(context.Background())
			tagCatalogRequireUnavailable(t, err)
			if got != nil {
				t.Fatalf("List() = %#v, expected no partial catalog", got)
			}
			tagCatalogRequireNoCalls(t, uow, store)
		})
	}
}

func TestP3C02ETagCatalogServiceMapsDependencyFailures(t *testing.T) {
	uowCause := errors.New("unit of work failed")
	uow := &tagCatalogTestUoW{err: uowCause}
	store := &tagCatalogTestStore{result: []TagCatalogRecord{}}
	got, err := NewTagCatalogService(uow, store).List(context.Background())
	tagCatalogRequireUnavailable(t, err)
	if !errors.Is(err, uowCause) {
		t.Fatalf("List() error = %v, expected unit of work cause", err)
	}
	if got != nil || uow.calls != 1 || uow.callbacks != 0 || store.calls != 0 {
		t.Fatalf("unit failure result/calls = %#v unit:%d callbacks:%d store:%d", got, uow.calls, uow.callbacks, store.calls)
	}

	storeCause := errors.New("store failed")
	uow = &tagCatalogTestUoW{}
	store = &tagCatalogTestStore{err: storeCause}
	got, err = NewTagCatalogService(uow, store).List(context.Background())
	tagCatalogRequireUnavailable(t, err)
	if !errors.Is(err, storeCause) {
		t.Fatalf("List() error = %v, expected store cause", err)
	}
	if got != nil || uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
		t.Fatalf("store failure result/calls = %#v unit:%d callbacks:%d store:%d", got, uow.calls, uow.callbacks, store.calls)
	}
}

func TestP3C02ETagCatalogServiceRejectsNilAndInvalidRecords(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	base := tagCatalogGrouped(11, 7, "lifecycle", 1, 1, "new")
	tests := []struct {
		name    string
		records []TagCatalogRecord
	}{
		{name: "nil catalog", records: nil},
		{name: "zero tag id", records: []TagCatalogRecord{func() TagCatalogRecord { record := base; record.ID = 0; return record }()}},
		{name: "negative tag id", records: []TagCatalogRecord{func() TagCatalogRecord { record := base; record.ID = -1; return record }()}},
		{name: "empty tag name", records: []TagCatalogRecord{func() TagCatalogRecord { record := base; record.Name = ""; return record }()}},
		{name: "invalid utf8 tag name", records: []TagCatalogRecord{func() TagCatalogRecord { record := base; record.Name = invalidUTF8; return record }()}},
		{name: "tag name above rune limit", records: []TagCatalogRecord{func() TagCatalogRecord { record := base; record.Name = strings.Repeat("界", 201); return record }()}},
		{name: "zero group id", records: []TagCatalogRecord{func() TagCatalogRecord { record := base; record.GroupID = tagCatalogInt64(0); return record }()}},
		{name: "negative group id", records: []TagCatalogRecord{func() TagCatalogRecord { record := base; record.GroupID = tagCatalogInt64(-1); return record }()}},
		{name: "empty group name", records: []TagCatalogRecord{func() TagCatalogRecord { record := base; record.GroupName = tagCatalogString(""); return record }()}},
		{name: "invalid utf8 group name", records: []TagCatalogRecord{func() TagCatalogRecord {
			record := base
			record.GroupName = tagCatalogString(invalidUTF8)
			return record
		}()}},
		{name: "group name above rune limit", records: []TagCatalogRecord{func() TagCatalogRecord {
			record := base
			record.GroupName = tagCatalogString(strings.Repeat("界", 201))
			return record
		}()}},
		{name: "duplicate tag id", records: []TagCatalogRecord{base, tagCatalogGrouped(11, 7, "lifecycle", 1, 2, "active")}},
		{name: "same group has inconsistent name", records: []TagCatalogRecord{base, tagCatalogGrouped(12, 7, "renamed", 1, 2, "active")}},
		{name: "same group has inconsistent sort", records: []TagCatalogRecord{base, tagCatalogGrouped(12, 7, "lifecycle", 2, 2, "active")}},
	}

	for mask := 1; mask < 7; mask++ {
		record := tagCatalogUngrouped(20+int64(mask), 0, "partial")
		if mask&1 != 0 {
			record.GroupID = tagCatalogInt64(7)
		}
		if mask&2 != 0 {
			record.GroupName = tagCatalogString("lifecycle")
		}
		if mask&4 != 0 {
			record.GroupSortOrder = tagCatalogInt32(1)
		}
		tests = append(tests, struct {
			name    string
			records []TagCatalogRecord
		}{name: "partial group pointers " + string(rune('0'+mask)), records: []TagCatalogRecord{record}})
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &tagCatalogTestUoW{}
			store := &tagCatalogTestStore{result: testCase.records}
			got, err := NewTagCatalogService(uow, store).List(context.Background())
			tagCatalogRequireUnavailable(t, err)
			if got != nil {
				t.Fatalf("List() = %#v, expected no partial catalog", got)
			}
			if uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
				t.Fatalf("calls = unit:%d callbacks:%d store:%d", uow.calls, uow.callbacks, store.calls)
			}
		})
	}
}

func TestP3C02ETagCatalogServiceRejectsEveryUnstableSortDimension(t *testing.T) {
	tests := []struct {
		name    string
		records []TagCatalogRecord
	}{
		{name: "ungrouped before grouped", records: []TagCatalogRecord{tagCatalogUngrouped(1, 0, "loose"), tagCatalogGrouped(2, 7, "group", 0, 0, "grouped")}},
		{name: "group sort order descends", records: []TagCatalogRecord{tagCatalogGrouped(1, 7, "first", 2, 0, "one"), tagCatalogGrouped(2, 8, "second", 1, 0, "two")}},
		{name: "group id descends", records: []TagCatalogRecord{tagCatalogGrouped(1, 8, "first", 1, 0, "one"), tagCatalogGrouped(2, 7, "second", 1, 0, "two")}},
		{name: "grouped tag sort order descends", records: []TagCatalogRecord{tagCatalogGrouped(1, 7, "group", 1, 2, "one"), tagCatalogGrouped(2, 7, "group", 1, 1, "two")}},
		{name: "grouped tag id descends", records: []TagCatalogRecord{tagCatalogGrouped(2, 7, "group", 1, 1, "one"), tagCatalogGrouped(1, 7, "group", 1, 1, "two")}},
		{name: "ungrouped tag sort order descends", records: []TagCatalogRecord{tagCatalogUngrouped(1, 2, "one"), tagCatalogUngrouped(2, 1, "two")}},
		{name: "ungrouped tag id descends", records: []TagCatalogRecord{tagCatalogUngrouped(2, 1, "one"), tagCatalogUngrouped(1, 1, "two")}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &tagCatalogTestUoW{}
			store := &tagCatalogTestStore{result: testCase.records}
			got, err := NewTagCatalogService(uow, store).List(context.Background())
			tagCatalogRequireUnavailable(t, err)
			if got != nil || uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
				t.Fatalf("result/calls = %#v unit:%d callbacks:%d store:%d", got, uow.calls, uow.callbacks, store.calls)
			}
		})
	}
}

func TestP3C02ETagCatalogServiceReturnsDeepCopy(t *testing.T) {
	storeRecords := []TagCatalogRecord{tagCatalogGrouped(11, 7, "lifecycle", 1, 2, "active")}
	got, err := NewTagCatalogService(&tagCatalogTestUoW{}, &tagCatalogTestStore{result: storeRecords}).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got[0].GroupID == storeRecords[0].GroupID || got[0].GroupName == storeRecords[0].GroupName || got[0].GroupSortOrder == storeRecords[0].GroupSortOrder {
		t.Fatal("result retained store-owned group pointers")
	}

	got[0].ID = 99
	*got[0].GroupID = 98
	*got[0].GroupName = "changed-result"
	*got[0].GroupSortOrder = 97
	got[0].Name = "changed-result"
	got[0].SortOrder = 96
	if storeRecords[0].ID != 11 || *storeRecords[0].GroupID != 7 || *storeRecords[0].GroupName != "lifecycle" || *storeRecords[0].GroupSortOrder != 1 || storeRecords[0].Name != "active" || storeRecords[0].SortOrder != 2 {
		t.Fatalf("mutating result changed store records: %#v", storeRecords)
	}

	storeRecords[0].ID = 88
	*storeRecords[0].GroupID = 87
	*storeRecords[0].GroupName = "changed-store"
	*storeRecords[0].GroupSortOrder = 86
	storeRecords[0].Name = "changed-store"
	storeRecords[0].SortOrder = 85
	if got[0].ID != 99 || *got[0].GroupID != 98 || *got[0].GroupName != "changed-result" || *got[0].GroupSortOrder != 97 || got[0].Name != "changed-result" || got[0].SortOrder != 96 {
		t.Fatalf("mutating store changed returned records: %#v", got)
	}
}

func TestP3C02ETagCatalogServiceReadsOnEveryUnitOfWorkRetry(t *testing.T) {
	uow := &tagCatalogTestUoW{attempts: 3}
	store := &tagCatalogTestStore{result: []TagCatalogRecord{tagCatalogUngrouped(11, 1, "vip")}}
	ctx := context.WithValue(context.Background(), tagCatalogParentKey{}, "retry-parent")

	got, err := NewTagCatalogService(uow, store).List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != 11 {
		t.Fatalf("List() = %#v", got)
	}
	if uow.calls != 1 || uow.callbacks != 3 || store.calls != 3 {
		t.Fatalf("calls = unit:%d callbacks:%d store:%d, expected one read per retry", uow.calls, uow.callbacks, store.calls)
	}
	if !reflect.DeepEqual(store.attempts, []int{1, 2, 3}) || !reflect.DeepEqual(store.parentValues, []string{"retry-parent", "retry-parent", "retry-parent"}) {
		t.Fatalf("retry contexts = attempts:%v parents:%v", store.attempts, store.parentValues)
	}
}
