package membergrid

import (
	"context"
	"errors"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

type accessGrantStore struct {
	grants map[[2]int64]CollaboratorPermission
}

func (store *accessGrantStore) CollaboratorPermission(_ context.Context, productID, staffID int64) (CollaboratorPermission, bool, error) {
	permission, ok := store.grants[[2]int64{productID, staffID}]
	return permission, ok, nil
}

type memberEditorSpy struct {
	command memberport.UpdateFieldsCommand
	calls   int
}

func (spy *memberEditorSpy) UpdateFields(_ context.Context, command memberport.UpdateFieldsCommand) (memberdomain.Member, error) {
	spy.calls++
	spy.command = command
	stamp := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	return memberdomain.Member{
		MemberRef: command.MemberRef, ServiceProductID: command.ServiceProductID, CustomerID: 1,
		State: memberdomain.StateActive, Source: memberdomain.SourceManual, StartsAt: stamp,
		Version: command.ExpectedVersion + 1, CreatedAt: stamp, UpdatedAt: stamp,
	}, nil
}

func memberGridContext(t *testing.T, role authport.Role, staffID *int64, capability authport.Capability, scope authport.ScopeKind) context.Context {
	t.Helper()
	ctx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{
		AdminUserID: 91, Role: role, StaffID: staffID,
	}, authport.SessionRef("member-grid-test"))
	authorization := authport.Authorization{Capability: capability, Scope: scope}
	if scope == authport.ScopeOwnerStaff {
		authorization.OwnerStaffID = *staffID
	}
	ctx, err := authport.WithAuthorization(ctx, authorization)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestAccessServiceScopesCollaboratorsToCurrentActiveProduct(t *testing.T) {
	grid, unit := newTestService(t, &memoryStore{exists: true})
	staffID := int64(17)
	grants := &accessGrantStore{grants: map[[2]int64]CollaboratorPermission{
		{71, staffID}: CollaboratorPermissionEdit,
	}}
	editor := &memberEditorSpy{}
	service, err := NewAccessService(grid, unit, grants, editor)
	if err != nil {
		t.Fatal(err)
	}

	readContext := memberGridContext(t, authport.RoleSales, &staffID, authport.CapabilityMemberGridRead, authport.ScopeOwnerStaff)
	access, err := service.Access(readContext, 71)
	if err != nil || !access.CanView || !access.CanQuery || !access.CanEdit {
		t.Fatalf("access=%+v error=%v", access, err)
	}
	if _, err = service.Schema(readContext, 71); err != nil {
		t.Fatalf("schema error=%v", err)
	}
	if _, err = service.MemberViews(readContext, 71); err != nil {
		t.Fatalf("views error=%v", err)
	}
	if _, err = service.query(readContext, QueryInput{ProductID: 71, State: StateAll, Limit: 1}); err != nil {
		t.Fatalf("query error=%v", err)
	}

	writeContext := memberGridContext(t, authport.RoleSales, &staffID, authport.CapabilityMemberGridWrite, authport.ScopeOwnerStaff)
	remark, alliance := "备注", "联盟"
	member, err := service.UpdateFields(writeContext, UpdateFieldsCommand{
		ProductID: 71, MemberRef: "spm_0000000000000000000001", ExpectedVersion: 2,
		Remark: &remark, Alliance: &alliance, IdempotencyKey: "member-grid-fields-0001",
	})
	if err != nil || member.Version != 3 || editor.calls != 1 || editor.command.ActorID != 91 ||
		editor.command.ServiceProductID != 71 || editor.command.MemberRef != "spm_0000000000000000000001" {
		t.Fatalf("member=%+v err=%v calls=%d command=%+v", member, err, editor.calls, editor.command)
	}

	if _, err = service.Access(readContext, 72); !errors.Is(err, authport.ErrUnauthorized) {
		t.Fatalf("cross-product error=%v", err)
	}
	delete(grants.grants, [2]int64{71, staffID})
	if _, err = service.Access(readContext, 71); !errors.Is(err, authport.ErrUnauthorized) {
		t.Fatalf("deleted collaborator error=%v", err)
	}
}

func TestAccessServiceViewCollaboratorCannotEdit(t *testing.T) {
	grid, unit := newTestService(t, &memoryStore{exists: true})
	staffID := int64(18)
	service, err := NewAccessService(grid, unit, &accessGrantStore{grants: map[[2]int64]CollaboratorPermission{
		{71, staffID}: CollaboratorPermissionView,
	}}, &memberEditorSpy{})
	if err != nil {
		t.Fatal(err)
	}
	readContext := memberGridContext(t, authport.RoleSales, &staffID, authport.CapabilityMemberGridRead, authport.ScopeOwnerStaff)
	access, err := service.Access(readContext, 71)
	if err != nil || access.CanEdit {
		t.Fatalf("access=%+v error=%v", access, err)
	}
	writeContext := memberGridContext(t, authport.RoleSales, &staffID, authport.CapabilityMemberGridWrite, authport.ScopeOwnerStaff)
	if _, err = service.UpdateFields(writeContext, UpdateFieldsCommand{ProductID: 71}); !errors.Is(err, authport.ErrUnauthorized) {
		t.Fatalf("view write error=%v", err)
	}
}

func TestAccessServiceKeepsAdminOpsGlobalAndRejectsInactiveSales(t *testing.T) {
	grid, unit := newTestService(t, &memoryStore{exists: true})
	service, err := NewAccessService(grid, unit, &accessGrantStore{grants: map[[2]int64]CollaboratorPermission{}}, &memberEditorSpy{})
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		ctx := memberGridContext(t, role, nil, authport.CapabilityMemberGridRead, authport.ScopeGlobal)
		if access, accessErr := service.Access(ctx, 71); accessErr != nil || !access.CanEdit {
			t.Fatalf("role=%s access=%+v error=%v", role, access, accessErr)
		}
	}
	inactiveID := int64(19)
	ctx := memberGridContext(t, authport.RoleSales, &inactiveID, authport.CapabilityMemberGridRead, authport.ScopeOwnerStaff)
	if _, err = service.Access(ctx, 71); !errors.Is(err, authport.ErrUnauthorized) {
		t.Fatalf("inactive staff error=%v", err)
	}
}
