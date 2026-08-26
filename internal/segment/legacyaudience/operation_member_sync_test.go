package legacyaudience

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type operationMemberSourceStub struct {
	items []OperationMember
	err   error
	calls int
}

func (source *operationMemberSourceStub) ReadOperationMembers(context.Context) ([]OperationMember, error) {
	source.calls++
	if source.err != nil {
		return nil, source.err
	}
	return append([]OperationMember(nil), source.items...), nil
}

func TestSyncOperationMembersReplacesProjectionAndRecordsOnlyRedactedEvent(t *testing.T) {
	world := newLocalConfigurationWorld()
	world.operationMembers = []OperationMember{{SenderUserID: "previous", DisplayName: "Previous"}}
	service := newLocalConfigurationService(t, world)
	source := &operationMemberSourceStub{items: []OperationMember{
		{SenderUserID: "beta", DisplayName: "Beta"}, {SenderUserID: "alpha", DisplayName: "Alpha"},
	}}
	if err := service.SetOperationMemberSource(source); err != nil {
		t.Fatal(err)
	}
	response, err := service.SyncOperationMembers(context.Background(), OperationMemberSyncInput{
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "audience-member-sync-key-1", PageSize: 1,
	})
	if err != nil || !reflect.DeepEqual(response.Items, []OperationMember{{SenderUserID: "alpha", DisplayName: "Alpha"}}) ||
		response.PageSize != 1 || response.Scope != OperationMemberScope || !response.LocalProjection || response.RealExternalCallExecuted {
		t.Fatalf("SyncOperationMembers response=%+v err=%v", response, err)
	}
	want := []OperationMember{{SenderUserID: "alpha", DisplayName: "Alpha"}, {SenderUserID: "beta", DisplayName: "Beta"}}
	if !reflect.DeepEqual(world.operationMembers, want) || len(world.receipts) != 1 || len(world.base.events) != 1 {
		t.Fatalf("projection=%+v receipts=%d events=%+v", world.operationMembers, len(world.receipts), world.base.events)
	}
	payload := string(world.base.events[0].Payload)
	if strings.Contains(payload, "alpha") || strings.Contains(payload, "Beta") || !strings.Contains(payload, "member_count") || !strings.Contains(payload, "members_digest") {
		t.Fatalf("event payload is not redacted: %s", payload)
	}
	// page_size must only trim the response, never the stored replacement.
	stored, err := service.ListOperationMembers(context.Background(), MaximumOperationMemberPageSize)
	if err != nil || !reflect.DeepEqual(stored.Items, want) {
		t.Fatalf("stored projection=%+v err=%v", stored, err)
	}
}

func TestSyncOperationMembersProviderFailureRetainsLastSuccessfulProjection(t *testing.T) {
	world := newLocalConfigurationWorld()
	world.operationMembers = []OperationMember{{SenderUserID: "previous", DisplayName: "Previous"}}
	service := newLocalConfigurationService(t, world)
	if err := service.SetOperationMemberSource(&operationMemberSourceStub{err: errors.New("provider unavailable")}); err != nil {
		t.Fatal(err)
	}
	_, err := service.SyncOperationMembers(context.Background(), OperationMemberSyncInput{
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "audience-member-sync-failure", PageSize: 100,
	})
	if !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(world.operationMembers, []OperationMember{{SenderUserID: "previous", DisplayName: "Previous"}}) ||
		len(world.receipts) != 0 || len(world.base.events) != 0 {
		t.Fatalf("err=%v projection=%+v receipts=%d events=%d", err, world.operationMembers, len(world.receipts), len(world.base.events))
	}
}

func TestSyncOperationMembersEmptyProviderSnapshotClearsProjection(t *testing.T) {
	world := newLocalConfigurationWorld()
	world.operationMembers = []OperationMember{{SenderUserID: "previous", DisplayName: "Previous"}}
	service := newLocalConfigurationService(t, world)
	if err := service.SetOperationMemberSource(&operationMemberSourceStub{items: []OperationMember{}}); err != nil {
		t.Fatal(err)
	}
	response, err := service.SyncOperationMembers(context.Background(), OperationMemberSyncInput{
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "audience-member-sync-empty", PageSize: 100,
	})
	if err != nil || len(response.Items) != 0 || len(world.operationMembers) != 0 || len(world.base.events) != 1 {
		t.Fatalf("response=%+v projection=%+v events=%d err=%v", response, world.operationMembers, len(world.base.events), err)
	}
}
