package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var errExternalEffectsFixture = errors.New("external effects fixture failure")

type externalEffectsTestUOW struct {
	err   error
	calls int
}

func (uow *externalEffectsTestUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	return callback(ctx)
}

type externalEffectsTestStore struct {
	list    func(ExternalEffectStoreQuery) ([]ExternalEffectSource, error)
	counts  ExternalEffectStatusCounts
	err     error
	queries []ExternalEffectStoreQuery
}

func (store *externalEffectsTestStore) ListExternalEffectSources(
	_ context.Context,
	query ExternalEffectStoreQuery,
) ([]ExternalEffectSource, error) {
	store.queries = append(store.queries, query)
	if store.err != nil {
		return nil, store.err
	}
	if store.list != nil {
		return store.list(query)
	}
	return nil, nil
}

func (store *externalEffectsTestStore) CountExternalEffectStatuses(context.Context) (ExternalEffectStatusCounts, error) {
	if store.err != nil {
		return ExternalEffectStatusCounts{}, store.err
	}
	return store.counts, nil
}

func TestExternalEffectsListUsesIrreversibleIDsAndEncryptedFilterBoundCursor(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, time.August, 21, 1, 0, 0, 0, time.UTC)
	rows := []ExternalEffectSource{
		{TaskID: 103, Status: TaskStatusPending, AttemptCount: 0, CreatedAt: created, StatusUpdatedAt: created},
		{TaskID: 102, Status: TaskStatusSent, AttemptCount: 1, CreatedAt: created.Add(time.Minute), StatusUpdatedAt: created.Add(2 * time.Minute)},
		{TaskID: 101, Status: TaskStatusOutcomeUnknown, AttemptCount: 2, CreatedAt: created.Add(3 * time.Minute), StatusUpdatedAt: created.Add(4 * time.Minute)},
	}
	store := &externalEffectsTestStore{list: func(query ExternalEffectStoreQuery) ([]ExternalEffectSource, error) {
		if query.Status != "" || query.Offset < 0 || query.Limit < 1 {
			t.Fatalf("unsafe store query = %+v", query)
		}
		start := int(query.Offset)
		if start >= len(rows) {
			return nil, nil
		}
		end := start + int(query.Limit)
		if end > len(rows) {
			end = len(rows)
		}
		return append([]ExternalEffectSource(nil), rows[start:end]...), nil
	}}
	uow := &externalEffectsTestUOW{}
	service := mustExternalEffectsService(t, uow, store, bytes.Repeat([]byte{0x31}, 32))
	service.entropy = bytes.NewReader(bytes.Repeat([]byte{0x42}, 256))

	first, err := service.ListJobs(context.Background(), ExternalEffectJobQuery{Limit: 2})
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	if len(first.Items) != 2 || first.NextCursor == nil || first.PageSize != 2 {
		t.Fatalf("first page = %+v", first)
	}
	if len(store.queries) < 2 || store.queries[0] != (ExternalEffectStoreQuery{Offset: 0, Limit: 2}) ||
		store.queries[1] != (ExternalEffectStoreQuery{Offset: 2, Limit: 1}) {
		t.Fatalf("first page/probe queries = %+v", store.queries)
	}
	assertExternalEffectsSafety(t, first.ProviderExecutionEligible, first.RealExternalCallExecuted, first.DeliveryProven, first.LocalFactOnly, first.DeliverySemantics)
	if first.Items[0].ID == first.Items[1].ID || first.Items[0].ID == "103" ||
		!strings.HasPrefix(first.Items[0].ID, ExternalEffectJobIDPrefix) {
		t.Fatalf("unsafe or unstable IDs = %#v", first.Items)
	}
	if first.Items[0].Handling != ExternalEffectSafeLocalHandling || first.Items[1].Handling != ExternalEffectFrozen {
		t.Fatalf("classifications = %q/%q", first.Items[0].Handling, first.Items[1].Handling)
	}
	if !strings.HasPrefix(*first.NextCursor, ExternalEffectsCursorPrefix) ||
		strings.Contains(*first.NextCursor, "101") || strings.Contains(*first.NextCursor, "102") {
		t.Fatalf("cursor is not opaque: %q", *first.NextCursor)
	}

	second, err := service.ListJobs(context.Background(), ExternalEffectJobQuery{Cursor: *first.NextCursor, Limit: 2})
	if err != nil {
		t.Fatalf("second ListJobs() error = %v", err)
	}
	if len(second.Items) != 1 || second.NextCursor != nil || second.Items[0].Handling != ExternalEffectManualReview {
		t.Fatalf("second page = %+v", second)
	}
	if got := store.queries[2]; got != (ExternalEffectStoreQuery{Offset: 2, Limit: 2}) {
		t.Fatalf("second page query = %+v", got)
	}

	repeated, err := service.ListJobs(context.Background(), ExternalEffectJobQuery{Limit: 2})
	if err != nil || repeated.Items[0].ID != first.Items[0].ID {
		t.Fatalf("stable pseudonym = %q/%q err=%v", first.Items[0].ID, repeated.Items[0].ID, err)
	}
	other := mustExternalEffectsService(t, &externalEffectsTestUOW{}, store, bytes.Repeat([]byte{0x32}, 32))
	other.entropy = bytes.NewReader(bytes.Repeat([]byte{0x43}, 256))
	otherPage, err := other.ListJobs(context.Background(), ExternalEffectJobQuery{Limit: 2})
	if err != nil || otherPage.Items[0].ID == first.Items[0].ID {
		t.Fatalf("secret-scoped pseudonym = %q/%q err=%v", first.Items[0].ID, otherPage.Items[0].ID, err)
	}

	tampered := *first.NextCursor
	if tampered[len(tampered)-1] == 'A' {
		tampered = tampered[:len(tampered)-1] + "B"
	} else {
		tampered = tampered[:len(tampered)-1] + "A"
	}
	if _, err = service.ListJobs(context.Background(), ExternalEffectJobQuery{Cursor: tampered, Limit: 2}); !errors.Is(err, ErrInvalidExternalEffectsCursor) {
		t.Fatalf("tampered cursor error = %v", err)
	}
	if _, err = service.ListJobs(context.Background(), ExternalEffectJobQuery{
		Cursor: *first.NextCursor, Limit: 2, Status: TaskStatusPending,
	}); !errors.Is(err, ErrInvalidExternalEffectsCursor) {
		t.Fatalf("filter-switched cursor error = %v", err)
	}
}

func TestExternalEffectsClassificationCursorAdvancesEachClosedStatusStream(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 21, 1, 30, 0, 0, time.UTC)
	byStatus := map[TaskStatus][]ExternalEffectSource{
		TaskStatusFinalFailed: {
			{TaskID: 10, Status: TaskStatusFinalFailed, AttemptCount: 1, CreatedAt: now, StatusUpdatedAt: now},
			{TaskID: 7, Status: TaskStatusFinalFailed, AttemptCount: 2, CreatedAt: now, StatusUpdatedAt: now},
		},
		TaskStatusOutcomeUnknown: {
			{TaskID: 9, Status: TaskStatusOutcomeUnknown, AttemptCount: 1, CreatedAt: now, StatusUpdatedAt: now},
			{TaskID: 8, Status: TaskStatusOutcomeUnknown, AttemptCount: 2, CreatedAt: now, StatusUpdatedAt: now},
			{TaskID: 6, Status: TaskStatusOutcomeUnknown, AttemptCount: 3, CreatedAt: now, StatusUpdatedAt: now},
		},
	}
	store := &externalEffectsTestStore{list: func(query ExternalEffectStoreQuery) ([]ExternalEffectSource, error) {
		rows := byStatus[query.Status]
		start := int(query.Offset)
		if start >= len(rows) {
			return nil, nil
		}
		end := start + int(query.Limit)
		if end > len(rows) {
			end = len(rows)
		}
		return append([]ExternalEffectSource(nil), rows[start:end]...), nil
	}}
	service := mustExternalEffectsService(t, &externalEffectsTestUOW{}, store, bytes.Repeat([]byte{0x33}, 32))
	service.entropy = bytes.NewReader(bytes.Repeat([]byte{0x55}, 256))

	first, err := service.ListJobs(context.Background(), ExternalEffectJobQuery{
		Handling: ExternalEffectManualReview, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].Status != TaskStatusFinalFailed ||
		first.Items[1].Status != TaskStatusOutcomeUnknown || first.NextCursor == nil {
		t.Fatalf("first merged page = %+v", first)
	}
	if len(store.queries) != 2 || store.queries[0] != (ExternalEffectStoreQuery{Status: TaskStatusFinalFailed, Limit: 2}) ||
		store.queries[1] != (ExternalEffectStoreQuery{Status: TaskStatusOutcomeUnknown, Limit: 2}) {
		t.Fatalf("first stream queries = %+v", store.queries)
	}

	second, err := service.ListJobs(context.Background(), ExternalEffectJobQuery{
		Handling: ExternalEffectManualReview, Limit: 2, Cursor: *first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.Items[0].Status != TaskStatusOutcomeUnknown ||
		second.Items[1].Status != TaskStatusFinalFailed || second.NextCursor == nil {
		t.Fatalf("second merged page = %+v", second)
	}
	if store.queries[2] != (ExternalEffectStoreQuery{Status: TaskStatusFinalFailed, Offset: 1, Limit: 2}) ||
		store.queries[3] != (ExternalEffectStoreQuery{Status: TaskStatusOutcomeUnknown, Offset: 1, Limit: 2}) {
		t.Fatalf("second stream queries = %+v", store.queries[2:4])
	}

	third, err := service.ListJobs(context.Background(), ExternalEffectJobQuery{
		Handling: ExternalEffectManualReview, Limit: 2, Cursor: *second.NextCursor,
	})
	if err != nil || len(third.Items) != 1 || third.Items[0].Status != TaskStatusOutcomeUnknown || third.NextCursor != nil {
		t.Fatalf("third merged page = %+v, %v", third, err)
	}
}

func TestExternalEffectsListFiltersAndFailsClosedForInvalidStoreFacts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	valid := ExternalEffectSource{TaskID: 9, Status: TaskStatusFinalFailed, AttemptCount: 1, CreatedAt: now, StatusUpdatedAt: now}
	tests := []struct {
		name  string
		query ExternalEffectJobQuery
		rows  []ExternalEffectSource
		want  error
	}{
		{name: "matching status and classification", query: ExternalEffectJobQuery{Status: TaskStatusFinalFailed, Handling: ExternalEffectManualReview}, rows: []ExternalEffectSource{valid}},
		{name: "incompatible filter", query: ExternalEffectJobQuery{Status: TaskStatusPending, Handling: ExternalEffectFrozen}, want: ErrInvalidExternalEffectsQuery},
		{name: "queued is not an approved status", query: ExternalEffectJobQuery{Status: TaskStatus("queued")}, want: ErrInvalidExternalEffectsQuery},
		{name: "accepted is not an approved status", query: ExternalEffectJobQuery{Status: TaskStatus("accepted")}, want: ErrInvalidExternalEffectsQuery},
		{name: "completed is not an approved status", query: ExternalEffectJobQuery{Status: TaskStatus("completed")}, want: ErrInvalidExternalEffectsQuery},
		{name: "ascending store order", rows: []ExternalEffectSource{valid, {TaskID: 10, Status: TaskStatusPending, CreatedAt: now, StatusUpdatedAt: now}}, want: ErrExternalEffectsUnavailable},
		{name: "sensitive lifecycle inconsistency", rows: []ExternalEffectSource{{TaskID: 9, Status: TaskStatusSent, AttemptCount: 0, CreatedAt: now, StatusUpdatedAt: now}}, want: ErrExternalEffectsUnavailable},
		{name: "unknown store enum", rows: []ExternalEffectSource{{TaskID: 9, Status: TaskStatus("accepted"), AttemptCount: 1, CreatedAt: now, StatusUpdatedAt: now}}, want: ErrExternalEffectsUnavailable},
		{name: "invalid timestamps", rows: []ExternalEffectSource{{TaskID: 9, Status: TaskStatusPending, CreatedAt: now, StatusUpdatedAt: now.Add(-time.Second)}}, want: ErrExternalEffectsUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &externalEffectsTestStore{list: func(ExternalEffectStoreQuery) ([]ExternalEffectSource, error) {
				return append([]ExternalEffectSource(nil), test.rows...), nil
			}}
			service := mustExternalEffectsService(t, &externalEffectsTestUOW{}, store, bytes.Repeat([]byte{0x44}, 32))
			got, err := service.ListJobs(context.Background(), test.query)
			if test.want != nil {
				if !errors.Is(err, test.want) {
					t.Fatalf("ListJobs() error = %v, want %v", err, test.want)
				}
				return
			}
			if err != nil || len(got.Items) != 1 || got.Items[0].Status != TaskStatusFinalFailed {
				t.Fatalf("ListJobs() = %+v, %v", got, err)
			}
			captured := store.queries[0]
			if captured.Status != test.query.Status || captured.Offset != 0 || captured.Limit != ExternalEffectsDefaultLimit {
				t.Fatalf("store query = %+v", captured)
			}
		})
	}
}

func TestExternalEffectsListRejectsProbeThatDoesNotAdvance(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 21, 2, 30, 0, 0, time.UTC)
	row := ExternalEffectSource{TaskID: 9, Status: TaskStatusPending, CreatedAt: now, StatusUpdatedAt: now}
	store := &externalEffectsTestStore{list: func(ExternalEffectStoreQuery) ([]ExternalEffectSource, error) {
		return []ExternalEffectSource{row}, nil
	}}
	service := mustExternalEffectsService(t, &externalEffectsTestUOW{}, store, bytes.Repeat([]byte{0x45}, 32))
	if _, err := service.ListJobs(context.Background(), ExternalEffectJobQuery{Limit: 1}); !errors.Is(err, ErrExternalEffectsUnavailable) {
		t.Fatalf("non-advancing probe error = %v", err)
	}
}

func TestExternalEffectsDiagnosticsClassifiesRiskWithoutDeliveryClaims(t *testing.T) {
	t.Parallel()

	store := &externalEffectsTestStore{counts: ExternalEffectStatusCounts{
		Pending: 2, Sending: 3, Sent: 5, RetryableFailed: 7, FinalFailed: 11, OutcomeUnknown: 13, Cancelled: 17,
	}}
	service := mustExternalEffectsService(t, &externalEffectsTestUOW{}, store, bytes.Repeat([]byte{0x55}, 32))
	generatedAt := time.Date(2026, time.August, 21, 3, 0, 0, 0, time.FixedZone("fixture", 8*60*60))
	service.clock = func() time.Time { return generatedAt }

	got, err := service.Diagnostics(context.Background())
	if err != nil {
		t.Fatalf("Diagnostics() error = %v", err)
	}
	if got.Total != 58 || got.ByClassification.SafeLocalHandling != 9 ||
		got.ByClassification.Frozen != 25 || got.ByClassification.ManualReview != 24 {
		t.Fatalf("diagnostic counts = %+v", got)
	}
	if got.Risk.Level != ExternalEffectRiskOutcomeUnknownPresent || got.Risk.OutcomeUnknownCount != 13 ||
		got.Risk.ManualReviewCount != 24 || !got.Risk.ManualReviewRequired {
		t.Fatalf("risk = %+v", got.Risk)
	}
	if !got.GeneratedAt.Equal(generatedAt.UTC()) {
		t.Fatalf("GeneratedAt = %s, want %s", got.GeneratedAt, generatedAt.UTC())
	}
	assertExternalEffectsSafety(t, got.ProviderExecutionEligible, got.RealExternalCallExecuted, got.DeliveryProven, got.LocalFactOnly, got.DeliverySemantics)
}

func TestExternalEffectsDiagnosticsRejectsCountOverflow(t *testing.T) {
	t.Parallel()

	store := &externalEffectsTestStore{counts: ExternalEffectStatusCounts{
		Pending: int64(1<<63 - 1), Sending: 1,
	}}
	service := mustExternalEffectsService(t, &externalEffectsTestUOW{}, store, bytes.Repeat([]byte{0x66}, 32))
	if _, err := service.Diagnostics(context.Background()); !errors.Is(err, ErrExternalEffectsUnavailable) {
		t.Fatalf("Diagnostics overflow error = %v", err)
	}
	if got := store.counts.Total(); got != -1 {
		t.Fatalf("overflow Total() = %d, want -1 sentinel", got)
	}
}

func TestExternalEffectsDependencyAndStoreFailuresAreClosed(t *testing.T) {
	t.Parallel()

	if _, err := NewExternalEffectsService(nil, &externalEffectsTestStore{}, bytes.Repeat([]byte{1}, 32)); !errors.Is(err, ErrInvalidExternalEffectsConfiguration) {
		t.Fatalf("nil UoW constructor error = %v", err)
	}
	var typedNilStore *externalEffectsTestStore
	if _, err := NewExternalEffectsService(&externalEffectsTestUOW{}, typedNilStore, bytes.Repeat([]byte{1}, 32)); !errors.Is(err, ErrInvalidExternalEffectsConfiguration) {
		t.Fatalf("typed nil store constructor error = %v", err)
	}
	if _, err := NewExternalEffectsService(&externalEffectsTestUOW{}, &externalEffectsTestStore{}, []byte("short")); !errors.Is(err, ErrInvalidExternalEffectsConfiguration) {
		t.Fatalf("short secret constructor error = %v", err)
	}
	service := mustExternalEffectsService(t, &externalEffectsTestUOW{}, &externalEffectsTestStore{err: errExternalEffectsFixture}, bytes.Repeat([]byte{2}, 32))
	if _, err := service.ListJobs(context.Background(), ExternalEffectJobQuery{}); !errors.Is(err, ErrExternalEffectsUnavailable) || !errors.Is(err, errExternalEffectsFixture) {
		t.Fatalf("ListJobs store error = %v", err)
	}
	if _, err := service.Diagnostics(context.Background()); !errors.Is(err, ErrExternalEffectsUnavailable) || !errors.Is(err, errExternalEffectsFixture) {
		t.Fatalf("Diagnostics store error = %v", err)
	}
}

func mustExternalEffectsService(
	t *testing.T,
	uow *externalEffectsTestUOW,
	store *externalEffectsTestStore,
	secret []byte,
) *ExternalEffectsService {
	t.Helper()
	service, err := NewExternalEffectsService(uow, store, secret)
	if err != nil {
		t.Fatalf("NewExternalEffectsService() error = %v", err)
	}
	return service
}

func assertExternalEffectsSafety(
	t *testing.T,
	providerEligible, externalCall, deliveryProven, localFactOnly bool,
	semantics string,
) {
	t.Helper()
	if providerEligible || externalCall || deliveryProven || !localFactOnly ||
		semantics != ExternalEffectsDeliverySemantics {
		t.Fatalf("unsafe semantics = eligible:%v external:%v delivery:%v local:%v semantics:%q",
			providerEligible, externalCall, deliveryProven, localFactOnly, semantics)
	}
}
