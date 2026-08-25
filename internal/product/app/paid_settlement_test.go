package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestPaidSettlementServicePeriodGrantAndCompensationAreClosed(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	digest := [32]byte{1, 2, 3}
	store := &paidSettlementFakeStore{product: PaidProductSnapshot{ID: 7, Version: 3, Kind: productport.PaidSettlementServicePeriod}}
	events := &paidSettlementEvents{}
	service, err := NewPaidSettlementService(store, events)
	if err != nil {
		t.Fatal(err)
	}
	command := productport.PaidSettlementCommand{Action: productport.PaidSettlementGrant, ProductKind: productport.PaidSettlementServicePeriod, ProductID: 7, ProductVersion: 3, OrderID: 11, CustomerID: 13, SettlementReceiptDigest: digest, OccurredAt: now}
	granted, err := service.ApplyPaidSettlement(context.Background(), command)
	if err != nil || granted.EntitlementID != 17 || granted.MemberRef == "" || len(events.rows) != 2 {
		t.Fatalf("grant=%+v events=%+v err=%v", granted, events.rows, err)
	}
	command.Action = productport.PaidSettlementCompensate
	compensated, err := service.ApplyPaidSettlement(context.Background(), command)
	if err != nil || compensated.State != "revoked" || store.entitlement.State != "revoked" || store.member.State != "removed" || len(events.rows) != 4 {
		t.Fatalf("compensate=%+v entitlement=%+v member=%+v events=%d err=%v", compensated, store.entitlement, store.member, len(events.rows), err)
	}
}

func TestPaidSettlementRejectsDigestDriftAndEventFailure(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	digest := [32]byte{9}
	store := &paidSettlementFakeStore{product: PaidProductSnapshot{ID: 7, Version: 3, Kind: productport.PaidSettlementOrdinary}, entitlement: PaidEntitlementRecord{ID: 17, ProductID: 7, OrderID: 11, CustomerID: 13, State: "active", Version: 1, Digest: [32]byte{8}}}
	service, _ := NewPaidSettlementService(store, &paidSettlementEvents{})
	command := productport.PaidSettlementCommand{Action: productport.PaidSettlementGrant, ProductKind: productport.PaidSettlementOrdinary, ProductID: 7, ProductVersion: 3, OrderID: 11, CustomerID: 13, SettlementReceiptDigest: digest, OccurredAt: now}
	if _, err := service.ApplyPaidSettlement(context.Background(), command); !errors.Is(err, productport.ErrPaidSettlementConflict) {
		t.Fatalf("digest drift err=%v", err)
	}
	store.entitlement = PaidEntitlementRecord{}
	eventErr := errors.New("event failed")
	service, _ = NewPaidSettlementService(store, &paidSettlementEvents{err: eventErr})
	if _, err := service.ApplyPaidSettlement(context.Background(), command); !errors.Is(err, eventErr) {
		t.Fatalf("event failure err=%v", err)
	}
}

type paidSettlementFakeStore struct {
	product     PaidProductSnapshot
	entitlement PaidEntitlementRecord
	member      PaidMemberRecord
}

func (store *paidSettlementFakeStore) LockPaidProduct(context.Context, productport.ID, int64, productport.PaidSettlementProductKind) (PaidProductSnapshot, error) {
	return store.product, nil
}
func (store *paidSettlementFakeStore) CreatePaidEntitlement(_ context.Context, command productport.PaidSettlementCommand) (PaidEntitlementRecord, error) {
	store.entitlement = PaidEntitlementRecord{ID: 17, ProductID: command.ProductID, OrderID: command.OrderID, CustomerID: command.CustomerID, State: "active", Version: 1, Digest: command.SettlementReceiptDigest, GrantedAt: command.OccurredAt}
	return store.entitlement, nil
}
func (store *paidSettlementFakeStore) LockPaidEntitlement(context.Context, int64) (PaidEntitlementRecord, error) {
	if store.entitlement.ID == 0 {
		return PaidEntitlementRecord{}, ErrPaidSettlementRowNotFound()
	}
	return store.entitlement, nil
}
func (store *paidSettlementFakeStore) RevokePaidEntitlement(_ context.Context, _ int64, _ int64, _ [32]byte, at time.Time) (PaidEntitlementRecord, error) {
	store.entitlement.State, store.entitlement.Version, store.entitlement.RevokedAt = "revoked", store.entitlement.Version+1, &at
	return store.entitlement, nil
}
func (store *paidSettlementFakeStore) CreatePaidMember(_ context.Context, ref string, command productport.PaidSettlementCommand, _ productport.EntitlementID) (PaidMemberRecord, error) {
	store.member = PaidMemberRecord{MemberRef: ref, ProductID: int64(command.ProductID), CustomerID: command.CustomerID, State: "active", Version: 1, CreatedAt: command.OccurredAt, UpdatedAt: command.OccurredAt}
	return store.member, nil
}
func (store *paidSettlementFakeStore) LockPaidMember(context.Context, int64) (PaidMemberRecord, error) {
	if store.member.MemberRef == "" {
		return PaidMemberRecord{}, ErrPaidSettlementRowNotFound()
	}
	return store.member, nil
}
func (store *paidSettlementFakeStore) RemovePaidMember(_ context.Context, _ int64, _ int64, _ [32]byte, at time.Time) (PaidMemberRecord, error) {
	store.member.State, store.member.Version, store.member.UpdatedAt = "removed", store.member.Version+1, at
	return store.member, nil
}

type paidSettlementEvents struct {
	rows []eventport.Event
	err  error
}

func (events *paidSettlementEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	if events.err != nil {
		return 0, events.err
	}
	events.rows = append(events.rows, event)
	return eventport.EventID(len(events.rows)), nil
}
