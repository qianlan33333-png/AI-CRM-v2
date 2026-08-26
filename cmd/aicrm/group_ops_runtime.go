package main

import (
	"context"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
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
