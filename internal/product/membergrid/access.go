package membergrid

import (
	"context"
	"errors"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

// AccessStore resolves only the current product-local collaborator grant. It
// must not cache its result: a collaborator removal applies to the next call.
type AccessStore interface {
	CollaboratorPermission(context.Context, int64, int64) (CollaboratorPermission, bool, error)
}

// AccessService narrows Member Grid access without changing Products or
// entitlement permissions. Admin and ops are global; sales remains scoped to
// an active collaborator row for the addressed product.
type AccessService struct {
	grid   *Service
	uow    platformport.UnitOfWork
	store  AccessStore
	editor MemberFieldEditor
}

var _ Application = (*AccessService)(nil)

func NewAccessService(grid *Service, uow platformport.UnitOfWork, store AccessStore, editor MemberFieldEditor) (*AccessService, error) {
	if grid == nil || nilDependency(uow) || nilDependency(store) || nilDependency(editor) {
		return nil, errors.New("member grid access dependencies are required")
	}
	return &AccessService{grid: grid, uow: uow, store: store, editor: editor}, nil
}

func (service *AccessService) Access(ctx context.Context, productID int64) (AccessResponse, error) {
	canEdit, err := service.authorize(ctx, productID, false)
	if err != nil {
		return AccessResponse{}, err
	}
	response, err := service.grid.Access(ctx, productID)
	if err != nil {
		return AccessResponse{}, err
	}
	response.CanEdit = canEdit
	return response, nil
}

func (service *AccessService) Schema(ctx context.Context, productID int64) (SchemaResponse, error) {
	if _, err := service.authorize(ctx, productID, false); err != nil {
		return SchemaResponse{}, err
	}
	return service.grid.Schema(ctx, productID)
}

func (service *AccessService) MemberViews(ctx context.Context, productID int64) (MemberViewsResponse, error) {
	if _, err := service.authorize(ctx, productID, false); err != nil {
		return MemberViewsResponse{}, err
	}
	return service.grid.MemberViews(ctx, productID)
}

func (service *AccessService) Query(ctx context.Context, input QueryInput) (QueryResponse, error) {
	if _, err := service.authorize(ctx, input.ProductID, false); err != nil {
		return QueryResponse{}, err
	}
	return service.grid.Query(ctx, input)
}

func (service *AccessService) UpdateFields(ctx context.Context, command UpdateFieldsCommand) (memberdomain.Member, error) {
	if _, err := service.authorize(ctx, command.ProductID, true); err != nil {
		return memberdomain.Member{}, err
	}
	member, err := service.editor.UpdateFields(ctx, memberport.UpdateFieldsCommand{
		ServiceProductID: command.ProductID,
		MemberRef:        command.MemberRef,
		ExpectedVersion:  command.ExpectedVersion,
		Remark:           command.Remark,
		Alliance:         command.Alliance,
		ActorID:          principalID(ctx),
		IdempotencyKey:   command.IdempotencyKey,
	})
	return member, classifyMemberError(err)
}

func (service *AccessService) authorize(ctx context.Context, productID int64, write bool) (bool, error) {
	if service == nil || service.grid == nil || nilDependency(service.uow) || nilDependency(service.store) || nilDependency(service.editor) ||
		ctx == nil || productID < 1 {
		return false, ErrUnavailable
	}
	principal, ok := authport.PrincipalFromContext(ctx)
	if !ok || principal.AdminUserID < 1 {
		return false, authport.ErrUnauthenticated
	}
	capability := authport.CapabilityMemberGridRead
	if write {
		capability = authport.CapabilityMemberGridWrite
	}
	authorization, ok := authport.AuthorizationFromContext(ctx)
	if !ok || authorization.Capability != capability {
		return false, authport.ErrUnauthorized
	}
	if authorization.Scope == authport.ScopeGlobal {
		if authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			return false, authport.ErrUnauthorized
		}
		return true, nil
	}
	if authorization.Scope != authport.ScopeOwnerStaff || principal.Role != authport.RoleSales || principal.StaffID == nil ||
		*principal.StaffID < 1 || authorization.OwnerStaffID != *principal.StaffID {
		return false, authport.ErrUnauthorized
	}
	var permission CollaboratorPermission
	var found bool
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		permission, found, storeErr = service.store.CollaboratorPermission(txCtx, productID, *principal.StaffID)
		return storeErr
	})
	if err != nil {
		return false, errors.Join(ErrUnavailable, err)
	}
	if !found || !permission.valid() || (write && permission != CollaboratorPermissionEdit) {
		return false, authport.ErrUnauthorized
	}
	return permission == CollaboratorPermissionEdit, nil
}

func principalID(ctx context.Context) int64 {
	principal, _ := authport.PrincipalFromContext(ctx)
	return principal.AdminUserID
}

func classifyMemberError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, memberport.ErrInvalidInput):
		return ErrInvalidQuery
	case errors.Is(err, memberport.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, memberport.ErrConflict):
		return ErrConflict
	case errors.Is(err, memberport.ErrUnavailable):
		return ErrUnavailable
	default:
		return errors.Join(ErrUnavailable, err)
	}
}
