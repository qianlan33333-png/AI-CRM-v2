package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

type txKey struct{}

type testUoW struct{ calls int }

func (uow *testUoW) Within(ctx context.Context, fn func(context.Context) error) error {
	uow.calls++
	return fn(context.WithValue(ctx, txKey{}, true))
}

type testEvents struct {
	events []eventport.Event
	err    error
}

func (events *testEvents) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	if ctx.Value(txKey{}) != true {
		return 0, errors.New("event outside transaction")
	}
	if events.err != nil {
		return 0, events.err
	}
	events.events = append(events.events, event)
	return eventport.EventID(len(events.events)), nil
}

type testStore struct {
	members       map[string]memberdomain.Member
	receipts      map[string]memberport.Receipt
	nextReceiptID int64
	createCalls   int
	readbackCalls int
	listOverride  []memberdomain.Member
}

func newTestStore() *testStore {
	return &testStore{members: map[string]memberdomain.Member{}, receipts: map[string]memberport.Receipt{}, nextReceiptID: 1}
}

func (*testStore) ServiceProductExists(ctx context.Context, id int64) (bool, error) {
	return ctx.Value(txKey{}) == true && id == 7, nil
}
func (*testStore) CustomerExists(ctx context.Context, id int64) (bool, error) {
	return ctx.Value(txKey{}) == true && id == 9, nil
}
func (store *testStore) Get(_ context.Context, productID int64, memberRef string) (memberdomain.Member, error) {
	store.readbackCalls++
	member, ok := store.members[memberRef]
	if !ok || member.ServiceProductID != productID {
		return memberdomain.Member{}, memberport.ErrNotFound
	}
	return cloneMember(member), nil
}
func (store *testStore) GetForUpdate(ctx context.Context, productID int64, memberRef string) (memberdomain.Member, error) {
	return store.Get(ctx, productID, memberRef)
}
func (store *testStore) Create(_ context.Context, record memberport.CreateRecord) (memberdomain.Member, error) {
	store.createCalls++
	member := memberdomain.Member{MemberRef: record.MemberRef, ServiceProductID: record.ServiceProductID, CustomerID: record.CustomerID, State: memberdomain.StateActive, Source: record.Source, StartsAt: record.StartsAt, ExpiresAt: cloneTime(record.ExpiresAt), Remark: cloneString(record.Remark), Alliance: cloneString(record.Alliance), Version: 1, CreatedAt: record.CreatedAt, UpdatedAt: record.CreatedAt}
	store.members[member.MemberRef] = cloneMember(member)
	return member, nil
}
func (store *testStore) Transition(_ context.Context, record memberport.TransitionRecord) (memberdomain.Member, error) {
	member, ok := store.members[record.MemberRef]
	if !ok || member.ServiceProductID != record.ServiceProductID {
		return memberdomain.Member{}, memberport.ErrNotFound
	}
	if member.Version != record.ExpectedVersion {
		return memberdomain.Member{}, memberport.ErrConflict
	}
	member.State, member.Version, member.UpdatedAt = record.Target, member.Version+1, record.TransitionedAt
	if record.Target == memberdomain.StateExpired {
		member.ExpiredAt = cloneTime(&record.TransitionedAt)
	}
	if record.Target == memberdomain.StateRemoved {
		member.RemovedAt = cloneTime(&record.TransitionedAt)
	}
	store.members[record.MemberRef] = cloneMember(member)
	return member, nil
}
func (store *testStore) UpdateFields(_ context.Context, record memberport.UpdateFieldsRecord) (memberdomain.Member, error) {
	member, ok := store.members[record.MemberRef]
	if !ok || member.ServiceProductID != record.ServiceProductID {
		return memberdomain.Member{}, memberport.ErrNotFound
	}
	if member.Version != record.ExpectedVersion {
		return memberdomain.Member{}, memberport.ErrConflict
	}
	member.Remark, member.Alliance, member.Version, member.UpdatedAt = cloneString(record.Remark), cloneString(record.Alliance), member.Version+1, record.UpdatedAt
	store.members[record.MemberRef] = cloneMember(member)
	return member, nil
}
func (store *testStore) List(_ context.Context, query memberport.StoreListQuery) ([]memberdomain.Member, error) {
	if store.listOverride != nil {
		return cloneMembers(store.listOverride), nil
	}
	rows := make([]memberdomain.Member, 0, len(store.members))
	for _, member := range store.members {
		if member.ServiceProductID != query.ServiceProductID || query.State != nil && member.State != *query.State || query.Source != nil && member.Source != *query.Source {
			continue
		}
		if query.After != nil && !before(member, query.After.UpdatedAt, query.After.MemberRef) {
			continue
		}
		rows = append(rows, cloneMember(member))
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt) || rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) && rows[i].MemberRef > rows[j].MemberRef
	})
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
	}
	return rows, nil
}
func (store *testStore) ReserveReceipt(_ context.Context, reservation memberport.ReceiptReservation) (memberport.Receipt, bool, error) {
	key := reservation.Operation + ":" + string(reservation.KeyDigest[:])
	if receipt, ok := store.receipts[key]; ok {
		return receipt, false, nil
	}
	receipt := memberport.Receipt{ID: store.nextReceiptID, ReceiptReservation: reservation, State: "reserved"}
	store.nextReceiptID++
	store.receipts[key] = receipt
	return receipt, true, nil
}
func (store *testStore) CompleteReceipt(_ context.Context, id int64, snapshot json.RawMessage, completedAt time.Time) (memberport.Receipt, error) {
	for key, receipt := range store.receipts {
		if receipt.ID != id {
			continue
		}
		receipt.State, receipt.ResultSnapshot = "completed", append([]byte(nil), snapshot...)
		store.receipts[key] = receipt
		return receipt, nil
	}
	return memberport.Receipt{}, memberport.ErrUnavailable
}

func TestAddIsOpaqueIdempotentAndAppendsEventInsideTransaction(t *testing.T) {
	service, store, events, uow := fixture(t)
	remark := "重点学员"
	command := memberport.AddCommand{ServiceProductID: 7, CustomerID: 9, Source: memberdomain.SourceManual, Remark: &remark, ActorID: 3, IdempotencyKey: "member-add-key-0001"}
	first, err := service.Add(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Add(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.MemberRef != "spm_AAAAAAAAAAAAAAAAAAAAAA" || !reflect.DeepEqual(first, second) {
		t.Fatalf("opaque/replay mismatch: %#v %#v", first, second)
	}
	if store.createCalls != 1 || len(events.events) != 1 || uow.calls != 2 || events.events[0].Type != eventport.EvProductUpdated || events.events[0].CustomerID != 9 {
		t.Fatalf("create/events/uow=%d/%d/%d event=%#v", store.createCalls, len(events.events), uow.calls, events.events)
	}
	if strings.Contains(string(events.events[0].Payload), "unionid") || strings.Contains(string(events.events[0].Payload), "phone") {
		t.Fatalf("unsafe event payload=%s", events.events[0].Payload)
	}
}

func TestPaidOrderSourceFailsClosedBeforeTransaction(t *testing.T) {
	service, store, events, uow := fixture(t)
	_, err := service.Add(context.Background(), memberport.AddCommand{ServiceProductID: 7, CustomerID: 9, Source: memberdomain.SourcePaidOrder, ActorID: 3, IdempotencyKey: "paid-source-key-001"})
	if !errors.Is(err, memberport.ErrPaidOrderSourceBlocked) || uow.calls != 0 || store.createCalls != 0 || len(events.events) != 0 {
		t.Fatalf("err/uow/create/events=%v/%d/%d/%d", err, uow.calls, store.createCalls, len(events.events))
	}
}

func TestExpireRemoveAndFieldsEnforceCASAndStrictReadback(t *testing.T) {
	service, store, _, _ := fixture(t)
	member := validMember("spm_BBBBBBBBBBBBBBBBBBBBBB", time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC))
	store.members[member.MemberRef] = member
	if _, err := service.Expire(context.Background(), memberport.TransitionCommand{ServiceProductID: 7, MemberRef: member.MemberRef, ExpectedVersion: 2, ActorID: 3, IdempotencyKey: "expire-key-00001"}); !errors.Is(err, memberport.ErrConflict) {
		t.Fatalf("CAS err=%v", err)
	}
	expired, err := service.Expire(context.Background(), memberport.TransitionCommand{ServiceProductID: 7, MemberRef: member.MemberRef, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "expire-key-00002"})
	if err != nil || expired.State != memberdomain.StateExpired || expired.Version != 2 || expired.ExpiredAt == nil {
		t.Fatalf("expired/err=%#v/%v", expired, err)
	}
	remark, alliance := "已续约", "伙伴A"
	updated, err := service.UpdateFields(context.Background(), memberport.UpdateFieldsCommand{ServiceProductID: 7, MemberRef: member.MemberRef, ExpectedVersion: 2, Remark: &remark, Alliance: &alliance, ActorID: 3, IdempotencyKey: "fields-key-00001"})
	if err != nil || updated.Version != 3 || updated.Remark == nil || *updated.Remark != remark {
		t.Fatalf("updated/err=%#v/%v", updated, err)
	}
	removed, err := service.Remove(context.Background(), memberport.TransitionCommand{ServiceProductID: 7, MemberRef: member.MemberRef, ExpectedVersion: 3, ActorID: 3, IdempotencyKey: "remove-key-00001"})
	if err != nil || removed.State != memberdomain.StateRemoved || removed.Version != 4 || removed.RemovedAt == nil || store.readbackCalls < 6 {
		t.Fatalf("removed/reads/err=%#v/%d/%v", removed, store.readbackCalls, err)
	}
}

func TestListCursorIsTamperProofAndFilterBound(t *testing.T) {
	service, store, _, _ := fixture(t)
	now := time.Date(2026, 8, 23, 1, 0, 0, 0, time.UTC)
	store.members["spm_CCCCCCCCCCCCCCCCCCCCCC"] = validMember("spm_CCCCCCCCCCCCCCCCCCCCCC", now)
	store.members["spm_DDDDDDDDDDDDDDDDDDDDDD"] = validMember("spm_DDDDDDDDDDDDDDDDDDDDDD", now.Add(-time.Minute))
	first, err := service.List(context.Background(), memberport.ListQuery{Filter: memberport.Filter{ServiceProductID: 7}, Limit: 1})
	if err != nil || !first.HasMore || first.NextCursor == "" || len(first.Items) != 1 {
		t.Fatalf("first/err=%#v/%v", first, err)
	}
	second, err := service.List(context.Background(), memberport.ListQuery{Filter: memberport.Filter{ServiceProductID: 7}, Limit: 1, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].MemberRef == first.Items[0].MemberRef {
		t.Fatalf("second/err=%#v/%v", second, err)
	}
	active := memberdomain.StateActive
	if _, err = service.List(context.Background(), memberport.ListQuery{Filter: memberport.Filter{ServiceProductID: 7, State: &active}, Limit: 1, Cursor: first.NextCursor}); !errors.Is(err, memberport.ErrInvalidInput) {
		t.Fatalf("filter-bound err=%v", err)
	}
	tampered := first.NextCursor[:len(first.NextCursor)-1] + "A"
	if _, err = service.List(context.Background(), memberport.ListQuery{Filter: memberport.Filter{ServiceProductID: 7}, Limit: 1, Cursor: tampered}); !errors.Is(err, memberport.ErrInvalidInput) {
		t.Fatalf("tamper err=%v", err)
	}
}

func TestExportWhitelistExcludesFreeTextAndExternalIdentifiers(t *testing.T) {
	service, store, _, _ := fixture(t)
	member := validMember("spm_EEEEEEEEEEEEEEEEEEEEEE", time.Date(2026, 8, 23, 1, 0, 0, 0, time.UTC))
	formula := "=HYPERLINK(\"https://example.invalid\")"
	member.Remark = &formula
	store.members[member.MemberRef] = member
	result, err := service.Export(context.Background(), memberport.ExportQuery{Filter: memberport.Filter{ServiceProductID: 7}, Columns: []memberport.ExportColumn{memberport.ExportMemberRef, memberport.ExportCustomerID}})
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(result.Body, []byte("\xef\xbb\xbf"))))
	records, err := reader.ReadAll()
	if err != nil || len(records) != 2 || records[0][0] != "member_ref" || records[0][1] != "customer_id" {
		t.Fatalf("records/err=%#v/%v", records, err)
	}
	for _, column := range []memberport.ExportColumn{"remark", "alliance", "unionid", "phone"} {
		if _, err = service.Export(context.Background(), memberport.ExportQuery{Filter: memberport.Filter{ServiceProductID: 7}, Columns: []memberport.ExportColumn{column}}); !errors.Is(err, memberport.ErrInvalidInput) {
			t.Fatalf("unsafe column %q err=%v", column, err)
		}
	}
	store.listOverride = make([]memberdomain.Member, memberport.MaximumExportRows+1)
	if _, err = service.Export(context.Background(), memberport.ExportQuery{Filter: memberport.Filter{ServiceProductID: 7}, Columns: []memberport.ExportColumn{memberport.ExportMemberRef}}); !errors.Is(err, memberport.ErrExportTooLarge) {
		t.Fatalf("oversize err=%v", err)
	}
}

func fixture(t *testing.T) (*Service, *testStore, *testEvents, *testUoW) {
	t.Helper()
	codec, err := NewCursorCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	store, events, uow := newTestStore(), &testEvents{}, &testUoW{}
	service, err := NewService(uow, store, events, codec)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 23, 2, 0, 0, 0, time.UTC) }
	service.random = bytes.NewReader(make([]byte, 256))
	return service, store, events, uow
}

func validMember(ref string, updated time.Time) memberdomain.Member {
	return memberdomain.Member{MemberRef: ref, ServiceProductID: 7, CustomerID: 9, State: memberdomain.StateActive, Source: memberdomain.SourceManual, StartsAt: updated.Add(-time.Hour), Version: 1, CreatedAt: updated.Add(-time.Hour), UpdatedAt: updated}
}
