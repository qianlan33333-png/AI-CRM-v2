package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type legacyTagUOW struct {
	in    bool
	calls int
}

func (u *legacyTagUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	u.calls++
	u.in = true
	defer func() { u.in = false }()
	return fn(ctx)
}

type legacyTagEvents struct {
	u     *legacyTagUOW
	items []eventport.Event
}

func (e *legacyTagEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	if !e.u.in {
		return 0, errors.New("outside uow")
	}
	e.items = append(e.items, event)
	return eventport.EventID(len(e.items)), nil
}

type legacyTagStore struct {
	u               *legacyTagUOW
	groups          []LegacyTagGroup
	tags            []LegacyTag
	writes          int
	tagReferences   map[int64]int64
	groupReferences map[int64]int64
	receipt         LocalTagReceipt
	receiptOwned    bool
}

func (s *legacyTagStore) check() error {
	if !s.u.in {
		return errors.New("outside uow")
	}
	return nil
}
func (s *legacyTagStore) ListLegacyTagGroups(context.Context) ([]LegacyTagGroup, error) {
	return append([]LegacyTagGroup(nil), s.groups...), s.check()
}
func (s *legacyTagStore) ListLegacyTags(context.Context) ([]LegacyTag, error) {
	return append([]LegacyTag(nil), s.tags...), s.check()
}
func (s *legacyTagStore) CreateLegacyTagGroup(_ context.Context, n string) (LegacyTagGroup, error) {
	if e := s.check(); e != nil {
		return LegacyTagGroup{}, e
	}
	s.writes++
	g := LegacyTagGroup{ID: int64(len(s.groups) + 1), Name: n}
	s.groups = append(s.groups, g)
	return g, nil
}
func (s *legacyTagStore) CreateLegacyTag(_ context.Context, g int64, n string) (LegacyTag, error) {
	if e := s.check(); e != nil {
		return LegacyTag{}, e
	}
	s.writes++
	for _, x := range s.groups {
		if x.ID == g {
			v := LegacyTag{ID: int64(len(s.tags) + 1), GroupID: g, GroupName: x.Name, Name: n}
			s.tags = append(s.tags, v)
			return v, nil
		}
	}
	return LegacyTag{}, ErrLegacyTagNotFound
}
func (s *legacyTagStore) UpdateLegacyTagGroup(context.Context, int64, string) (LegacyTagGroup, error) {
	return LegacyTagGroup{}, ErrLegacyTagNotFound
}
func (s *legacyTagStore) ArchiveLegacyTagGroup(context.Context, int64) (LegacyTagGroup, error) {
	return LegacyTagGroup{}, ErrLegacyTagNotFound
}
func (s *legacyTagStore) UpdateLegacyTag(context.Context, int64, string) (LegacyTag, error) {
	return LegacyTag{}, ErrLegacyTagNotFound
}
func (s *legacyTagStore) ArchiveLegacyTag(context.Context, int64) (LegacyTag, error) {
	return LegacyTag{}, ErrLegacyTagNotFound
}
func (s *legacyTagStore) GetLegacyTagGroup(_ context.Context, id int64) (LegacyTagGroup, error) {
	for _, group := range s.groups {
		if group.ID == id {
			return group, nil
		}
	}
	return LegacyTagGroup{}, ErrLegacyTagNotFound
}
func (s *legacyTagStore) GetLegacyTag(_ context.Context, id int64) (LegacyTag, error) {
	for _, tag := range s.tags {
		if tag.ID == id {
			return tag, nil
		}
	}
	return LegacyTag{}, ErrLegacyTagNotFound
}
func (s *legacyTagStore) ReorderLegacyTagGroups(_ context.Context, ids []int64) ([]LegacyTagGroup, error) {
	if e := s.check(); e != nil {
		return nil, e
	}
	if !sameLegacyIDs(groupIDs(s.groups), ids) {
		return nil, ErrLegacyTagConflict
	}
	for index := range s.groups {
		s.groups[index].SortOrder = int32(index)
	}
	return append([]LegacyTagGroup(nil), s.groups...), nil
}
func (s *legacyTagStore) ReorderLegacyTags(_ context.Context, ids []int64) ([]LegacyTag, error) {
	if e := s.check(); e != nil {
		return nil, e
	}
	if !sameLegacyIDs(tagIDs(s.tags), ids) {
		return nil, ErrLegacyTagConflict
	}
	for index := range s.tags {
		s.tags[index].SortOrder = int32(index)
	}
	return append([]LegacyTag(nil), s.tags...), nil
}
func (s *legacyTagStore) CountLegacyTagReferences(_ context.Context, id int64) (int64, error) {
	if e := s.check(); e != nil {
		return 0, e
	}
	return s.tagReferences[id], nil
}
func (s *legacyTagStore) CountLegacyTagGroupReferences(_ context.Context, id int64) (int64, error) {
	if e := s.check(); e != nil {
		return 0, e
	}
	return s.groupReferences[id], nil
}
func (s *legacyTagStore) ReserveLocalTagReceipt(_ context.Context, reservation LocalTagReceiptReservation) (LocalTagReceipt, bool, error) {
	if e := s.check(); e != nil {
		return LocalTagReceipt{}, false, e
	}
	if s.receipt.ID == 0 {
		s.receipt = LocalTagReceipt{ID: 1, Operation: reservation.Operation, Actor: reservation.Actor, IdempotencyKey: reservation.IdempotencyKey, PayloadDigest: append([]byte(nil), reservation.PayloadDigest...), State: localTagReceiptInProgress}
		s.receiptOwned = true
	}
	return s.receipt, s.receiptOwned, nil
}
func (s *legacyTagStore) CompleteLocalTagReceipt(_ context.Context, id int64, resultIDs []int64, _ time.Time) (LocalTagReceipt, error) {
	if e := s.check(); e != nil {
		return LocalTagReceipt{}, e
	}
	if id != s.receipt.ID || !s.receiptOwned {
		return LocalTagReceipt{}, ErrLegacyTagConflict
	}
	s.receipt.State = localTagReceiptCompleted
	s.receipt.ResultIDs = append([]int64(nil), resultIDs...)
	s.receiptOwned = false
	return s.receipt, nil
}

func TestB02LegacyTagCatalogCreateUsesOneUOWAndEvent(t *testing.T) {
	u := &legacyTagUOW{}
	s := &legacyTagStore{u: u, groups: []LegacyTagGroup{}, tags: []LegacyTag{}}
	e := &legacyTagEvents{u: u}
	service := NewLegacyTagCatalogService(u, s, e)
	service.now = func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }
	g, tag, err := service.CreateGroup(context.Background(), LegacyTagCommand{Actor: 7, IdempotencyKey: "catalog-key-0001", TraceID: "trace-1", GroupName: "客户阶段", FirstTagName: "新客"})
	if err != nil || g.ID != 1 || tag.ID != 1 || u.calls != 1 || s.writes != 2 || len(e.items) != 1 || e.items[0].Type != "contact.tag_catalog_group_create" {
		t.Fatalf("group=%#v tag=%#v calls=%d writes=%d events=%#v err=%v", g, tag, u.calls, s.writes, e.items, err)
	}
}
func TestB02LegacyTagCatalogBoundaryErrorsDoNotWrite(t *testing.T) {
	u := &legacyTagUOW{}
	s := &legacyTagStore{u: u}
	e := &legacyTagEvents{u: u}
	service := NewLegacyTagCatalogService(u, s, e)
	for _, c := range []LegacyTagCommand{{Actor: 0, IdempotencyKey: "catalog-key-0001", GroupName: "g", FirstTagName: "t"}, {Actor: 1, GroupName: "g", FirstTagName: "t"}, {Actor: 1, IdempotencyKey: "catalog-key-0001", GroupName: "", FirstTagName: "t"}} {
		if _, _, err := service.CreateGroup(context.Background(), c); !errors.Is(err, ErrInvalidLegacyTag) {
			t.Fatalf("command=%#v err=%v", c, err)
		}
	}
	if u.calls != 0 || s.writes != 0 || len(e.items) != 0 {
		t.Fatalf("calls=%d writes=%d events=%d", u.calls, s.writes, len(e.items))
	}
}

func TestLocalTagCreateReceiptReplaysAndRejectsPayloadDrift(t *testing.T) {
	u := &legacyTagUOW{}
	store := &legacyTagStore{u: u}
	events := &legacyTagEvents{u: u}
	service := NewLegacyTagCatalogService(u, store, events)
	command := LegacyTagCommand{Actor: 7, IdempotencyKey: "catalog-key-0007", GroupName: "来源", FirstTagName: "活动"}
	firstGroup, firstTag, err := service.CreateGroup(context.Background(), command)
	if err != nil || firstGroup.ID <= 0 || firstTag.ID <= 0 {
		t.Fatalf("first CreateGroup() = %#v/%#v, %v", firstGroup, firstTag, err)
	}
	secondGroup, secondTag, err := service.CreateGroup(context.Background(), command)
	if err != nil || secondGroup != firstGroup || secondTag != firstTag {
		t.Fatalf("replay CreateGroup() = %#v/%#v, %v", secondGroup, secondTag, err)
	}
	if store.writes != 2 || len(events.items) != 1 {
		t.Fatalf("replay writes/events = %d/%d, want 2/1", store.writes, len(events.items))
	}
	_, _, err = service.CreateGroup(context.Background(), LegacyTagCommand{Actor: command.Actor, IdempotencyKey: command.IdempotencyKey, GroupName: "其他", FirstTagName: command.FirstTagName})
	if !errors.Is(err, ErrLegacyTagConflict) || store.writes != 2 || len(events.items) != 1 {
		t.Fatalf("payload drift err/writes/events = %v/%d/%d", err, store.writes, len(events.items))
	}
}

func TestP4CustomerTagsReadsOneGroupAndOneTagFromTheLocalCatalog(t *testing.T) {
	u := &legacyTagUOW{}
	store := &legacyTagStore{u: u, groups: []LegacyTagGroup{{ID: 11, Name: "客户阶段", SortOrder: 3}}, tags: []LegacyTag{{ID: 21, GroupID: 11, GroupName: "客户阶段", Name: "新客", SortOrder: 4}}}
	events := &legacyTagEvents{u: u}
	service := NewLegacyTagCatalogService(u, store, events)
	service.now = func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }

	group, err := service.GetGroup(context.Background(), 11)
	if err != nil || group != (LegacyTagGroup{ID: 11, Name: "客户阶段", SortOrder: 3}) {
		t.Fatalf("GetGroup() = %#v, %v", group, err)
	}
	tag, err := service.GetTag(context.Background(), 21)
	if err != nil || tag != (LegacyTag{ID: 21, GroupID: 11, GroupName: "客户阶段", Name: "新客", SortOrder: 4}) {
		t.Fatalf("GetTag() = %#v, %v", tag, err)
	}
	if u.calls != 2 || store.writes != 0 || len(events.items) != 0 {
		t.Fatalf("uow/writes/events = %d/%d/%d, want 2/0/0", u.calls, store.writes, len(events.items))
	}
}

func TestP4CustomerTagsReadsFailClosedForInvalidOrAbsentIDs(t *testing.T) {
	u := &legacyTagUOW{}
	store := &legacyTagStore{u: u, groups: []LegacyTagGroup{{ID: 12, Name: "已有组"}}, tags: []LegacyTag{{ID: 22, GroupID: 12, GroupName: "已有组", Name: "已有标签"}}}
	events := &legacyTagEvents{u: u}
	service := NewLegacyTagCatalogService(u, store, events)

	for name, read := range map[string]func() error{
		"invalid group": func() error { _, err := service.GetGroup(context.Background(), 0); return err },
		"absent group":  func() error { _, err := service.GetGroup(context.Background(), 11); return err },
		"invalid tag":   func() error { _, err := service.GetTag(context.Background(), 0); return err },
		"absent tag":    func() error { _, err := service.GetTag(context.Background(), 21); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := read(); !errors.Is(err, ErrLegacyTagNotFound) {
				t.Fatalf("read error = %v, want not found", err)
			}
		})
	}
	if store.writes != 0 || len(events.items) != 0 {
		t.Fatalf("writes/events = %d/%d, want 0/0", store.writes, len(events.items))
	}
}

func TestLocalTagArchiveRejectsCustomerReferencesBeforeArchiveEvent(t *testing.T) {
	for name, archive := range map[string]func() error{
		"tag": func() error {
			u := &legacyTagUOW{}
			store := &legacyTagStore{u: u, tagReferences: map[int64]int64{21: 1}}
			_, err := NewLegacyTagCatalogService(u, store, &legacyTagEvents{u: u}).ArchiveTag(context.Background(), LegacyTagCommand{Actor: 1, TagID: 21, IdempotencyKey: "catalog-key-0001"})
			return err
		},
		"group": func() error {
			u := &legacyTagUOW{}
			store := &legacyTagStore{u: u, groupReferences: map[int64]int64{11: 2}}
			_, err := NewLegacyTagCatalogService(u, store, &legacyTagEvents{u: u}).ArchiveGroup(context.Background(), LegacyTagCommand{Actor: 1, GroupID: 11, IdempotencyKey: "catalog-key-0002"})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := archive(); !errors.Is(err, ErrLegacyTagReferenced) {
				t.Fatalf("archive error = %v", err)
			}
		})
	}
}
