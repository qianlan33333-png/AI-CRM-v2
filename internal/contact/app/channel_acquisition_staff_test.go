package app

import (
	"context"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type acquisitionStaffChannels struct{ channel Channel }

func (stub acquisitionStaffChannels) GetChannel(context.Context, int64) (Channel, error) {
	return stub.channel, nil
}

type acquisitionStaffLocal struct {
	items []contactport.StaffDirectoryEntry
	err   error
	calls *int
}

func (stub acquisitionStaffLocal) ListEligibleStaff(context.Context) ([]contactport.StaffDirectoryEntry, error) {
	if stub.calls != nil {
		*stub.calls++
	}
	return stub.items, stub.err
}

type acquisitionStaffProvider struct {
	items []string
	err   error
}

func (stub acquisitionStaffProvider) ListFollowUsers(context.Context) ([]string, error) {
	return stub.items, stub.err
}

func TestChannelAcquisitionStaffListIntersectsProviderAndLocalStaff(t *testing.T) {
	ratio, cap := int32(100), int32(12)
	service := NewChannelAcquisitionStaffService(
		acquisitionStaffChannels{channel: Channel{ID: 7, ChannelCode: "ch-7", ChannelName: "渠道", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(), Assignees: []ChannelAssignee{{WeComUserID: "staff-2", DisplayName: "客服二", Status: "active", Priority: 1, RatioPercent: &ratio, MaxScans24h: &cap}}}},
		acquisitionStaffLocal{items: []contactport.StaffDirectoryEntry{{WeComUserID: "staff-2", DisplayName: "客服二"}, {WeComUserID: "staff-1", DisplayName: "客服一"}, {WeComUserID: "local-only", DisplayName: "本地"}}},
		acquisitionStaffProvider{items: []string{"staff-2", "provider-only", "staff-1"}},
	)
	result, err := service.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ProviderReadSucceeded || result.RealExternalCallExecuted || result.ProviderSource != "wecom_follow_user_list" || len(result.Items) != 2 {
		t.Fatalf("result=%#v", result)
	}
	if result.Items[0].WeComUserID != "staff-1" || result.Items[0].Assigned || result.Items[1].WeComUserID != "staff-2" || !result.Items[1].Assigned || result.Items[1].RatioPercent == nil || *result.Items[1].RatioPercent != 100 {
		t.Fatalf("items=%#v", result.Items)
	}
}

func TestChannelAcquisitionStaffListFailsClosedWhenProviderReadFails(t *testing.T) {
	service := NewChannelAcquisitionStaffService(
		acquisitionStaffChannels{channel: Channel{ID: 7}}, acquisitionStaffLocal{},
		acquisitionStaffProvider{err: errors.New("provider unavailable")},
	)
	if _, err := service.List(context.Background(), 7); !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestChannelAcquisitionStaffListRejectsMalformedProviderDirectoryBeforeLocalFallback(t *testing.T) {
	localCalls := 0
	service := NewChannelAcquisitionStaffService(
		acquisitionStaffChannels{channel: Channel{ID: 7}},
		acquisitionStaffLocal{items: []contactport.StaffDirectoryEntry{{WeComUserID: "staff-1", DisplayName: "客服一"}}, calls: &localCalls},
		acquisitionStaffProvider{items: []string{"staff-1", "staff-1"}},
	)
	if _, err := service.List(context.Background(), 7); !errors.Is(err, ErrChannelUnavailable) || localCalls != 0 {
		t.Fatalf("err=%v local_calls=%d", err, localCalls)
	}
}
