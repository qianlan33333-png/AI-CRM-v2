package legacyaudience

import (
	"context"
	"errors"
	"sort"
	"strings"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

// GroupOpsDirectoryOperationMemberSource adapts only the existing read-only
// WeCom directory source. It neither records Group Ops state nor invokes a
// Group Ops runtime operation.
type GroupOpsDirectoryOperationMemberSource struct {
	directory groupopsport.GroupDirectorySource
}

func NewGroupOpsDirectoryOperationMemberSource(directory groupopsport.GroupDirectorySource) (*GroupOpsDirectoryOperationMemberSource, error) {
	if nilInterface(directory) {
		return nil, ErrUnavailable
	}
	return &GroupOpsDirectoryOperationMemberSource{directory: directory}, nil
}

func (source *GroupOpsDirectoryOperationMemberSource) ReadOperationMembers(ctx context.Context) ([]OperationMember, error) {
	if source == nil || nilInterface(source.directory) || ctx == nil {
		return nil, ErrUnavailable
	}
	// The Provider boundary is fixed at 100. HTTP page_size trims the stored
	// projection response only; it must never narrow deletion authority.
	items, err := source.directory.RefreshOperationMembers(ctx, MaximumOperationMemberPageSize)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	result := make([]OperationMember, 0, len(items))
	for _, item := range items {
		result = append(result, OperationMember{
			SenderUserID: strings.TrimSpace(item.SenderUserID),
			DisplayName:  strings.TrimSpace(item.DisplayName),
		})
	}
	if err := validateOperationMembers(result); err != nil {
		return nil, err
	}
	sortOperationMembers(result)
	return result, nil
}

var _ OperationMemberSource = (*GroupOpsDirectoryOperationMemberSource)(nil)

func sortOperationMembers(items []OperationMember) {
	sort.Slice(items, func(left, right int) bool {
		return items[left].SenderUserID < items[right].SenderUserID
	})
}

func validateOperationMembers(items []OperationMember) error {
	if len(items) > MaximumOperationMemberPageSize {
		return ErrUnavailable
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.SenderUserID == "" || item.DisplayName == "" || item.SenderUserID != strings.TrimSpace(item.SenderUserID) ||
			item.DisplayName != strings.TrimSpace(item.DisplayName) || len(item.SenderUserID) > 128 || len(item.DisplayName) > 128 {
			return ErrUnavailable
		}
		if _, exists := seen[item.SenderUserID]; exists {
			return ErrUnavailable
		}
		seen[item.SenderUserID] = struct{}{}
	}
	return nil
}
