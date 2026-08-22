package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

func TestChannelEntrantsServicePaginatesWithStableTimestampTieBreak(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 22, 3, 0, 0, 0, time.UTC)
	codec := mustChannelEntrantsTestCodec(t, &now)
	uow := &channelEntrantsTestUOW{}
	store := &channelEntrantsTestStore{state: ChannelEntrantsChannelActive}
	service := mustChannelEntrantsTestService(t, uow, store, codec)

	tied := now.Add(-time.Hour)
	older := tied.Add(-time.Second)
	lastInteract := tied.Add(5 * time.Minute)
	store.records = []ChannelEntrantsRecord{
		{CustomerID: 33, ChannelID: 7, DisplayName: "三号", AddedAt: tied, LastInteractAt: &lastInteract},
		{CustomerID: 22, ChannelID: 7, DisplayName: "二号", AddedAt: tied},
		{CustomerID: 99, ChannelID: 7, DisplayName: "更早", AddedAt: older},
	}

	first, err := service.List(context.Background(), ChannelEntrantsInput{ChannelID: 7, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if first.ChannelID != 7 || first.Limit != 2 || !first.HasMore || first.NextCursor == "" ||
		!first.LocalProjection || first.ProviderExecutionEligible || first.RealExternalCallExecuted || len(first.Items) != 2 {
		t.Fatalf("first=%#v", first)
	}
	if first.Items[0].CustomerID != 33 || first.Items[1].CustomerID != 22 ||
		first.Items[0].LastInteractAt == nil || !first.Items[0].LastInteractAt.Equal(lastInteract) {
		t.Fatalf("first items=%#v", first.Items)
	}
	position, err := codec.Decode(first.NextCursor, 7)
	if err != nil || position.CustomerID != 22 || !position.AddedAt.Equal(tied) {
		t.Fatalf("position=%#v err=%v", position, err)
	}
	if store.listCalls != 1 || store.lastQuery.Limit != 3 || store.lastQuery.After != nil {
		t.Fatalf("first store query=%#v calls=%d", store.lastQuery, store.listCalls)
	}

	store.records = []ChannelEntrantsRecord{
		{CustomerID: 99, ChannelID: 7, DisplayName: "更早", AddedAt: older},
	}
	second, err := service.List(context.Background(), ChannelEntrantsInput{
		ChannelID: 7, Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.HasMore || second.NextCursor != "" || len(second.Items) != 1 || second.Items[0].CustomerID != 99 {
		t.Fatalf("second=%#v", second)
	}
	if store.lastQuery.After == nil || store.lastQuery.After.CustomerID != 22 ||
		!store.lastQuery.After.AddedAt.Equal(tied) {
		t.Fatalf("second store query=%#v", store.lastQuery)
	}
	if uow.calls != 2 || store.stateCalls != 2 || store.listCalls != 2 || store.externalOps != 0 {
		t.Fatalf("calls uow/state/list/external=%d/%d/%d/%d", uow.calls, store.stateCalls, store.listCalls, store.externalOps)
	}
}

func TestChannelEntrantsServiceDefaultsLimitAndReturnsNonNilEmptyItems(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 22, 4, 0, 0, 0, time.UTC)
	store := &channelEntrantsTestStore{state: ChannelEntrantsChannelInactive, records: nil}
	service := mustChannelEntrantsTestService(t, &channelEntrantsTestUOW{}, store, mustChannelEntrantsTestCodec(t, &now))

	response, err := service.List(context.Background(), ChannelEntrantsInput{ChannelID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if response.Limit != ChannelEntrantsDefaultLimit || response.Items == nil || len(response.Items) != 0 ||
		response.HasMore || response.NextCursor != "" || !response.LocalProjection || response.ProviderExecutionEligible || response.RealExternalCallExecuted {
		t.Fatalf("response=%#v", response)
	}
	if store.lastQuery.Limit != ChannelEntrantsDefaultLimit+1 {
		t.Fatalf("store limit=%d", store.lastQuery.Limit)
	}
}

func TestChannelEntrantsServiceReturnsNotFoundForMissingOrArchivedChannel(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 22, 5, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		state    ChannelEntrantsChannelState
		stateErr error
	}{
		{name: "missing", stateErr: ErrChannelEntrantsNotFound},
		{name: "archived", state: ChannelEntrantsChannelArchived},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			store := &channelEntrantsTestStore{state: testCase.state, stateErr: testCase.stateErr}
			service := mustChannelEntrantsTestService(t, &channelEntrantsTestUOW{}, store, mustChannelEntrantsTestCodec(t, &now))
			_, err := service.List(context.Background(), ChannelEntrantsInput{ChannelID: 9})
			if !errors.Is(err, ErrChannelEntrantsNotFound) {
				t.Fatalf("error=%v", err)
			}
			if store.listCalls != 0 {
				t.Fatalf("list called %d times", store.listCalls)
			}
		})
	}
}

func TestChannelEntrantsServiceRejectsCursorBeforeOpeningTransaction(t *testing.T) {
	t.Parallel()

	clock := time.Date(2026, 8, 22, 6, 0, 0, 0, time.UTC)
	codec := mustChannelEntrantsTestCodec(t, &clock)
	token, err := codec.Encode(10, ChannelEntrantsPosition{AddedAt: clock.Add(-time.Minute), CustomerID: 101})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		channelID int64
		cursor    func() string
	}{
		{name: "cross channel", channelID: 11, cursor: func() string { return token }},
		{name: "tampered", channelID: 10, cursor: func() string {
			return token[:len(token)-1] + alternateChannelEntrantsCursorCharacter(token[len(token)-1])
		}},
		{name: "malformed", channelID: 10, cursor: func() string { return "not-a-cursor" }},
		{name: "expired", channelID: 10, cursor: func() string {
			clock = clock.Add(channelEntrantsDefaultCursorTTL)
			return token
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			originalClock := clock
			defer func() { clock = originalClock }()
			uow := &channelEntrantsTestUOW{}
			store := &channelEntrantsTestStore{state: ChannelEntrantsChannelActive}
			service := mustChannelEntrantsTestService(t, uow, store, codec)
			_, listErr := service.List(context.Background(), ChannelEntrantsInput{
				ChannelID: testCase.channelID, Limit: 2, Cursor: testCase.cursor(),
			})
			if !errors.Is(listErr, ErrInvalidChannelEntrantsCursor) {
				t.Fatalf("error=%v", listErr)
			}
			if uow.calls != 0 || store.stateCalls != 0 || store.listCalls != 0 {
				t.Fatalf("calls uow/state/list=%d/%d/%d", uow.calls, store.stateCalls, store.listCalls)
			}
		})
	}
}

func TestChannelEntrantsServiceRejectsMalformedStoreProjection(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 22, 7, 0, 0, 0, time.UTC)
	valid := ChannelEntrantsRecord{CustomerID: 40, ChannelID: 12, DisplayName: "有效", AddedAt: now.Add(-time.Minute)}
	zeroInteraction := time.Time{}
	tests := []struct {
		name    string
		records []ChannelEntrantsRecord
	}{
		{name: "zero customer", records: []ChannelEntrantsRecord{{ChannelID: 12, AddedAt: valid.AddedAt}}},
		{name: "wrong channel", records: []ChannelEntrantsRecord{{CustomerID: 40, ChannelID: 13, AddedAt: valid.AddedAt}}},
		{name: "zero added at", records: []ChannelEntrantsRecord{{CustomerID: 40, ChannelID: 12}}},
		{name: "zero last interaction", records: []ChannelEntrantsRecord{{CustomerID: 40, ChannelID: 12, AddedAt: valid.AddedAt, LastInteractAt: &zeroInteraction}}},
		{name: "invalid utf8", records: []ChannelEntrantsRecord{{CustomerID: 40, ChannelID: 12, DisplayName: string([]byte{0xff}), AddedAt: valid.AddedAt}}},
		{name: "duplicate", records: []ChannelEntrantsRecord{valid, valid}},
		{name: "unstable order", records: []ChannelEntrantsRecord{
			valid,
			{CustomerID: 41, ChannelID: 12, AddedAt: valid.AddedAt},
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			store := &channelEntrantsTestStore{state: ChannelEntrantsChannelActive, records: testCase.records}
			service := mustChannelEntrantsTestService(t, &channelEntrantsTestUOW{}, store, mustChannelEntrantsTestCodec(t, &now))
			_, err := service.List(context.Background(), ChannelEntrantsInput{ChannelID: 12, Limit: 2})
			if !errors.Is(err, ErrChannelEntrantsUnavailable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestChannelEntrantsServiceFailsClosedOnInvalidInputAndDependencies(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	codec := mustChannelEntrantsTestCodec(t, &now)
	if _, err := NewChannelEntrantsService(nil, &channelEntrantsTestStore{}, codec); err == nil {
		t.Fatal("nil unit of work was accepted")
	}
	var typedNilStore *channelEntrantsTestStore
	if _, err := NewChannelEntrantsService(&channelEntrantsTestUOW{}, typedNilStore, codec); err == nil {
		t.Fatal("typed nil store was accepted")
	}
	if _, err := NewChannelEntrantsService(&channelEntrantsTestUOW{}, &channelEntrantsTestStore{}, nil); err == nil {
		t.Fatal("nil codec was accepted")
	}

	service := mustChannelEntrantsTestService(t, &channelEntrantsTestUOW{}, &channelEntrantsTestStore{state: ChannelEntrantsChannelActive}, codec)
	for _, input := range []ChannelEntrantsInput{
		{ChannelID: 0},
		{ChannelID: 1, Limit: -1},
		{ChannelID: 1, Limit: ChannelEntrantsMaximumLimit + 1},
		{ChannelID: 1, Cursor: string(bytes.Repeat([]byte("x"), channelEntrantsMaximumCursorLength+1))},
	} {
		if _, err := service.List(context.Background(), input); !errors.Is(err, ErrInvalidChannelEntrantsQuery) {
			t.Fatalf("input=%#v error=%v", input, err)
		}
	}

	uowFailure := errors.New("transaction unavailable")
	service = mustChannelEntrantsTestService(t, &channelEntrantsTestUOW{err: uowFailure}, &channelEntrantsTestStore{}, codec)
	if _, err := service.List(context.Background(), ChannelEntrantsInput{ChannelID: 1}); !errors.Is(err, ErrChannelEntrantsUnavailable) || !errors.Is(err, uowFailure) {
		t.Fatalf("uow error=%v", err)
	}

	storeFailure := errors.New("database unavailable")
	service = mustChannelEntrantsTestService(t, &channelEntrantsTestUOW{}, &channelEntrantsTestStore{state: ChannelEntrantsChannelActive, listErr: storeFailure}, codec)
	if _, err := service.List(context.Background(), ChannelEntrantsInput{ChannelID: 1}); !errors.Is(err, ErrChannelEntrantsUnavailable) || !errors.Is(err, storeFailure) {
		t.Fatalf("store error=%v", err)
	}

	service = mustChannelEntrantsTestService(t, &channelEntrantsTestUOW{}, &channelEntrantsTestStore{state: "future_state"}, codec)
	if _, err := service.List(context.Background(), ChannelEntrantsInput{ChannelID: 1}); !errors.Is(err, ErrChannelEntrantsUnavailable) {
		t.Fatalf("unknown channel state error=%v", err)
	}
}

type channelEntrantsTestUOW struct {
	calls int
	err   error
}

var _ platformport.UnitOfWork = (*channelEntrantsTestUOW)(nil)

func (uow *channelEntrantsTestUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	return callback(ctx)
}

type channelEntrantsTestStore struct {
	state       ChannelEntrantsChannelState
	stateErr    error
	records     []ChannelEntrantsRecord
	listErr     error
	stateCalls  int
	listCalls   int
	lastQuery   ChannelEntrantsStoreQuery
	externalOps int
}

func (store *channelEntrantsTestStore) ReadChannelEntrantsChannelState(context.Context, int64) (ChannelEntrantsChannelState, error) {
	store.stateCalls++
	return store.state, store.stateErr
}

func (store *channelEntrantsTestStore) ListChannelEntrants(_ context.Context, query ChannelEntrantsStoreQuery) ([]ChannelEntrantsRecord, error) {
	store.listCalls++
	store.lastQuery = query
	return append([]ChannelEntrantsRecord(nil), store.records...), store.listErr
}

func mustChannelEntrantsTestCodec(t *testing.T, now *time.Time) *ChannelEntrantsCursorCodec {
	t.Helper()
	codec, err := newChannelEntrantsCursorCodec(
		bytes.Repeat([]byte("service-test-key-"), 3),
		bytes.NewReader(make([]byte, 4096)),
		func() time.Time { return *now },
		channelEntrantsDefaultCursorTTL,
	)
	if err != nil {
		t.Fatal(err)
	}
	return codec
}

func mustChannelEntrantsTestService(
	t *testing.T,
	uow platformport.UnitOfWork,
	store ChannelEntrantsStore,
	codec *ChannelEntrantsCursorCodec,
) *ChannelEntrantsService {
	t.Helper()
	service, err := NewChannelEntrantsService(uow, store, codec)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
