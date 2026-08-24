package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type sidebarProfileUOW struct{}

func (sidebarProfileUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type sidebarProfileEvents struct{ values []eventport.Event }

func (events *sidebarProfileEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	events.values = append(events.values, event)
	return eventport.EventID(len(events.values)), nil
}

type sidebarProfileStoreFake struct {
	record  SidebarProfileRecord
	receipt SidebarProfileReceipt
	updates int
}

func (store *sidebarProfileStoreFake) ReadSidebarProfile(context.Context, int64, int64) (SidebarProfileRecord, error) {
	return store.record, nil
}
func (store *sidebarProfileStoreFake) UpdateSidebarProfile(_ context.Context, record SidebarProfileRecord, expected time.Time) (SidebarProfileRecord, error) {
	if !expected.Equal(store.record.UpdatedAt) {
		return SidebarProfileRecord{}, contactport.ErrSidebarProfileConflict
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(store.record.Extra, &root)
	root["sidebar_profile"] = append([]byte(nil), record.Extra...)
	record.Extra, _ = json.Marshal(root)
	store.record, store.updates = record, store.updates+1
	return record, nil
}
func (store *sidebarProfileStoreFake) ReserveSidebarProfileReceipt(_ context.Context, reservation SidebarProfileReceiptReservation) (SidebarProfileReceipt, bool, error) {
	if store.receipt.ID > 0 {
		return store.receipt, false, nil
	}
	store.receipt = SidebarProfileReceipt{ID: 1, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, State: "reserved"}
	return store.receipt, true, nil
}
func (store *sidebarProfileStoreFake) CompleteSidebarProfileReceipt(_ context.Context, id int64, snapshot json.RawMessage, _ time.Time) (SidebarProfileReceipt, error) {
	if id != store.receipt.ID {
		return SidebarProfileReceipt{}, errors.New("wrong receipt")
	}
	store.receipt.State, store.receipt.ResultSnapshot = "completed", append([]byte(nil), snapshot...)
	return store.receipt, nil
}

func TestSidebarProfileUpdateIsCASReceiptAndReplaySafe(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	store := &sidebarProfileStoreFake{record: SidebarProfileRecord{CustomerID: 41, OwnerStaffID: 7, Name: "customer", Extra: json.RawMessage(`{"kept":{"flag":true}}`), UpdatedAt: now.Add(-time.Minute)}}
	events := &sidebarProfileEvents{}
	service := NewSidebarProfileService(sidebarProfileUOW{}, store, events)
	service.now = func() time.Time { return now }
	needs := "renewal"
	command := contactport.SidebarProfileUpdateCommand{CustomerID: 41, OwnerStaffID: 7, ExpectedUpdatedAt: store.record.UpdatedAt, Patch: contactport.SidebarProfilePatch{Needs: &needs}, Actor: "admin:9", IdempotencyKey: "sidebar-profile-0001"}

	first, err := service.UpdateSidebarProfile(context.Background(), command)
	if err != nil || first.Needs != needs || store.updates != 1 || len(events.values) != 1 || events.values[0].Type != eventport.EvCustomerUpdated {
		t.Fatalf("first=%+v updates=%d events=%+v err=%v", first, store.updates, events.values, err)
	}
	var kept struct {
		Flag bool `json:"flag"`
	}
	var extra map[string]json.RawMessage
	if err = json.Unmarshal(store.record.Extra, &extra); err != nil || json.Unmarshal(extra["kept"], &kept) != nil || !kept.Flag {
		t.Fatalf("unrelated customer extra was not preserved: %s", store.record.Extra)
	}
	replayed, err := service.UpdateSidebarProfile(context.Background(), command)
	if err != nil || replayed != first || store.updates != 1 || len(events.values) != 1 {
		t.Fatalf("replay=%+v updates=%d events=%d err=%v", replayed, store.updates, len(events.values), err)
	}
	changed := command
	different := "different"
	changed.Patch.Needs = &different
	if _, err = service.UpdateSidebarProfile(context.Background(), changed); !errors.Is(err, contactport.ErrSidebarProfileConflict) {
		t.Fatalf("payload reuse error=%v", err)
	}
	if store.updates != 1 || len(events.values) != 1 {
		t.Fatal("conflict created side effects")
	}
	if store.receipt.KeyDigest != sha256.Sum256([]byte(command.IdempotencyKey)) {
		t.Fatal("receipt key digest mismatch")
	}
}

func TestSidebarProfileUpdateRejectsStaleVersionBeforeSideEffects(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	store := &sidebarProfileStoreFake{record: SidebarProfileRecord{CustomerID: 41, OwnerStaffID: 7, Name: "customer", Extra: json.RawMessage(`{}`), UpdatedAt: now}}
	events := &sidebarProfileEvents{}
	service := NewSidebarProfileService(sidebarProfileUOW{}, store, events)
	service.now = func() time.Time { return now.Add(time.Second) }
	needs := "renewal"
	_, err := service.UpdateSidebarProfile(context.Background(), contactport.SidebarProfileUpdateCommand{CustomerID: 41, OwnerStaffID: 7, ExpectedUpdatedAt: now.Add(-time.Second), Patch: contactport.SidebarProfilePatch{Needs: &needs}, Actor: "admin:9", IdempotencyKey: "sidebar-profile-0002"})
	if !errors.Is(err, contactport.ErrSidebarProfileConflict) || store.updates != 0 || len(events.values) != 0 {
		t.Fatalf("stale update err=%v updates=%d events=%d", err, store.updates, len(events.values))
	}
}
