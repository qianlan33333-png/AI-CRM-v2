package app

import (
	"context"
	"errors"
	"testing"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

func TestDirectoryRefresherSyncsThenReadsLocalProjection(t *testing.T) {
	provider := &memberRefresherStub{page: groupopsport.OperationMemberPage{Scope: "group_ops", PageSize: 100, Items: []groupopsport.OperationMember{{StaffID: 7, SenderUserID: "alice", DisplayName: "Alice"}}}}
	reader := &projectionReaderStub{projection: Projection{Directory: []Candidate{{WeComUserID: "alice", DisplayName: "Alice"}}}}
	result, err := NewDirectoryRefresher(provider, reader).Refresh(context.Background(), RefreshDirectoryCommand{ActorID: 9, PageSize: 100, IdempotencyKey: "hxc-directory-refresh-0001"})
	if err != nil || provider.calls != 1 || reader.calls != 1 || result.SyncedCount != 1 || !result.ProviderReadExecuted || len(result.Projection.Directory) != 1 {
		t.Fatalf("result=%#v provider=%d reader=%d err=%v", result, provider.calls, reader.calls, err)
	}
}

func TestDirectoryRefresherFailsClosedBeforeReadback(t *testing.T) {
	reader := &projectionReaderStub{}
	_, err := NewDirectoryRefresher(&memberRefresherStub{err: errors.New("provider")}, reader).Refresh(context.Background(), RefreshDirectoryCommand{ActorID: 9, PageSize: 100, IdempotencyKey: "hxc-directory-refresh-0002"})
	if !errors.Is(err, ErrDirectoryRefreshUnavailable) || reader.calls != 0 {
		t.Fatalf("reader=%d err=%v", reader.calls, err)
	}
	_, err = NewDirectoryRefresher(nil, reader).Refresh(context.Background(), RefreshDirectoryCommand{ActorID: 9, PageSize: 100, IdempotencyKey: "hxc-directory-refresh-0003"})
	if !errors.Is(err, ErrDirectoryRefreshInvalid) {
		t.Fatalf("nil dependency err=%v", err)
	}
}

type memberRefresherStub struct {
	page  groupopsport.OperationMemberPage
	err   error
	calls int
}

func (stub *memberRefresherStub) RefreshOperationMembers(context.Context, groupopsport.OperationMemberRefreshCommand) (groupopsport.OperationMemberPage, error) {
	stub.calls++
	return stub.page, stub.err
}

type projectionReaderStub struct {
	projection Projection
	err        error
	calls      int
}

func (stub *projectionReaderStub) Read(context.Context) (Projection, error) {
	stub.calls++
	return stub.projection, stub.err
}
