package main

import (
	"context"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	groupopsdirectory "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/groupopsdirectory"
)

// groupOpsSenderResolver composes the two owner-scoped reads inside the
// caller's UoW: Group Ops supplies a group owner; Contact proves that local
// owner remains active and exposes exactly its WeCom userid.
type groupOpsSenderResolver struct {
	groups interface {
		LockDirectoryGroupOwner(context.Context, string) (int64, error)
	}
	staff contactport.ActiveStaffSenderReader
}

type groupOpsDirectoryOwnerResolver struct {
	staff contactport.ActiveStaffWeComUserIDReader
}

func (resolver groupOpsDirectoryOwnerResolver) ResolveActiveWeComUserID(ctx context.Context, staffID int64) (string, error) {
	if resolver.staff == nil {
		return "", contactport.ErrStaffReferenceUnavailable
	}
	return resolver.staff.ReadActiveWeComUserID(ctx, staffID)
}

type groupOpsDirectoryActiveStaff struct {
	staff contactport.StaffDirectoryReader
}

func (directory groupOpsDirectoryActiveStaff) ListActiveWeComStaff(ctx context.Context) ([]groupopsdirectory.ActiveStaff, error) {
	if directory.staff == nil {
		return nil, contactport.ErrStaffReferenceUnavailable
	}
	entries, err := directory.staff.ListEligibleStaff(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]groupopsdirectory.ActiveStaff, len(entries))
	for index, entry := range entries {
		result[index] = groupopsdirectory.ActiveStaff{WeComUserID: entry.WeComUserID, DisplayName: entry.DisplayName}
	}
	return result, nil
}

func (resolver groupOpsSenderResolver) ResolveExecutionSender(ctx context.Context, target string) (string, bool, error) {
	if resolver.groups == nil || resolver.staff == nil || ctx == nil || target == "" {
		return "", false, nil
	}
	owner, err := resolver.groups.LockDirectoryGroupOwner(ctx, target)
	if err != nil {
		return "", false, err
	}
	userID, err := resolver.staff.LockActiveWeComUserID(ctx, owner)
	if err != nil {
		return "", false, err
	}
	return userID, userID != "", nil
}
