package legacyaudience

import (
	"context"
	"reflect"
	"testing"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

type groupDirectorySourceStub struct {
	limit int32
	items []groupopsport.OperationMember
}

func (*groupDirectorySourceStub) ListOwnedGroups(context.Context, int64, int32) (groupopsport.GroupDirectorySnapshot, error) {
	return groupopsport.GroupDirectorySnapshot{}, nil
}
func (source *groupDirectorySourceStub) RefreshOperationMembers(_ context.Context, limit int32) ([]groupopsport.OperationMember, error) {
	source.limit = limit
	return append([]groupopsport.OperationMember(nil), source.items...), nil
}

func TestGroupOpsDirectoryOperationMemberSourceUsesFixedProviderLimit(t *testing.T) {
	directory := &groupDirectorySourceStub{items: []groupopsport.OperationMember{
		{SenderUserID: "beta", DisplayName: "Beta"}, {SenderUserID: "alpha", DisplayName: "Alpha"},
	}}
	source, err := NewGroupOpsDirectoryOperationMemberSource(directory)
	if err != nil {
		t.Fatal(err)
	}
	items, err := source.ReadOperationMembers(context.Background())
	want := []OperationMember{{SenderUserID: "alpha", DisplayName: "Alpha"}, {SenderUserID: "beta", DisplayName: "Beta"}}
	if err != nil || directory.limit != MaximumOperationMemberPageSize || !reflect.DeepEqual(items, want) {
		t.Fatalf("items=%+v provider_limit=%d err=%v", items, directory.limit, err)
	}
}
