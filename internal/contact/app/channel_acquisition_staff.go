package app

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type ChannelAcquisitionStaff struct {
	WeComUserID  string `json:"wecom_userid"`
	DisplayName  string `json:"display_name"`
	Assigned     bool   `json:"assigned"`
	Priority     int32  `json:"priority,omitempty"`
	RatioPercent *int32 `json:"ratio_percent,omitempty"`
	MaxScans24h  *int32 `json:"max_scans_24h,omitempty"`
}

type ChannelAcquisitionStaffDirectory struct {
	ChannelID                int64                     `json:"channel_id"`
	Items                    []ChannelAcquisitionStaff `json:"items"`
	ProviderSource           string                    `json:"provider_source"`
	ProviderReadSucceeded    bool                      `json:"provider_read_succeeded"`
	RealExternalCallExecuted bool                      `json:"real_external_call_executed"`
}

type channelAcquisitionFollowUserReader interface {
	ListFollowUsers(context.Context) ([]string, error)
}

type ChannelAcquisitionStaffService struct {
	channels channelAcquisitionReader
	local    contactport.StaffDirectoryReader
	provider channelAcquisitionFollowUserReader
}

func NewChannelAcquisitionStaffService(channels channelAcquisitionReader, local contactport.StaffDirectoryReader, provider channelAcquisitionFollowUserReader) *ChannelAcquisitionStaffService {
	return &ChannelAcquisitionStaffService{channels: channels, local: local, provider: provider}
}

// List performs one read-only Provider refresh and returns only the intersection
// with active local staff. It never creates staff or mutates channel assignment.
func (service *ChannelAcquisitionStaffService) List(ctx context.Context, channelID int64) (ChannelAcquisitionStaffDirectory, error) {
	if service == nil || service.channels == nil || service.local == nil || service.provider == nil || ctx == nil || channelID < 1 {
		return ChannelAcquisitionStaffDirectory{}, ErrChannelUnavailable
	}
	channel, err := service.channels.GetChannel(ctx, channelID)
	if err != nil {
		return ChannelAcquisitionStaffDirectory{}, err
	}
	followUsers, err := service.provider.ListFollowUsers(ctx)
	if err != nil {
		return ChannelAcquisitionStaffDirectory{}, ErrChannelUnavailable
	}
	following := make(map[string]struct{}, len(followUsers))
	for _, userID := range followUsers {
		if !validChannelAcquisitionStaffText(userID) {
			return ChannelAcquisitionStaffDirectory{}, ErrChannelUnavailable
		}
		if _, duplicate := following[userID]; duplicate {
			return ChannelAcquisitionStaffDirectory{}, ErrChannelUnavailable
		}
		following[userID] = struct{}{}
	}
	local, err := service.local.ListEligibleStaff(ctx)
	if err != nil {
		return ChannelAcquisitionStaffDirectory{}, ErrChannelUnavailable
	}
	assigned := make(map[string]ChannelAssignee, len(channel.Assignees))
	for _, value := range channel.Assignees {
		assigned[value.WeComUserID] = value
	}
	items := make([]ChannelAcquisitionStaff, 0, len(local))
	seen := make(map[string]struct{}, len(local))
	for _, value := range local {
		if !validChannelAcquisitionStaffText(value.WeComUserID) || !validChannelAcquisitionStaffText(value.DisplayName) {
			return ChannelAcquisitionStaffDirectory{}, ErrChannelUnavailable
		}
		if _, duplicate := seen[value.WeComUserID]; duplicate {
			return ChannelAcquisitionStaffDirectory{}, ErrChannelUnavailable
		}
		seen[value.WeComUserID] = struct{}{}
		if _, available := following[value.WeComUserID]; !available {
			continue
		}
		item := ChannelAcquisitionStaff{WeComUserID: value.WeComUserID, DisplayName: value.DisplayName}
		if current, ok := assigned[value.WeComUserID]; ok && current.Status == "active" {
			item.Assigned = true
			item.Priority = current.Priority
			item.RatioPercent = current.RatioPercent
			item.MaxScans24h = current.MaxScans24h
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].WeComUserID < items[j].WeComUserID })
	return ChannelAcquisitionStaffDirectory{
		ChannelID: channelID, Items: items, ProviderSource: "wecom_follow_user_list",
		ProviderReadSucceeded: true, RealExternalCallExecuted: false,
	}, nil
}

func validChannelAcquisitionStaffText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 128 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
