package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrAdminAccessUnavailable   = errors.New("admin access unavailable")
	ErrInvalidAdminAccessInput  = errors.New("invalid admin access input")
	ErrAdminAccessMemberMissing = errors.New("admin access member missing")
)

type AdminAccessMember struct {
	AdminUserID      int64
	DisplayName      string
	Role             authport.Role
	StaffID          *int64
	StaffWeComUserID string
	StaffName        string
	IsActive         bool
	LoginEnabled     bool
}

type AdminAccessSaveInput struct {
	AdminUserID  int64 `json:"admin_user_id"`
	LoginEnabled bool  `json:"login_enabled"`
}

type adminAccessRepository interface {
	ListAdminAccessMembers(context.Context) ([]authstore.AdminAccessMember, error)
	SaveAdminAccessMember(context.Context, int64, bool, time.Time) (authstore.AdminAccessSaveResult, error)
}

type AdminAccessService struct {
	uow   platformport.UnitOfWork
	repo  adminAccessRepository
	clock func() time.Time
}

func NewAdminAccessService(uow platformport.UnitOfWork, repo adminAccessRepository, clock func() time.Time) (*AdminAccessService, error) {
	if nilInterface(uow) || nilInterface(repo) {
		return nil, ErrAdminAccessUnavailable
	}
	if clock == nil {
		clock = time.Now
	}
	return &AdminAccessService{uow: uow, repo: repo, clock: clock}, nil
}

func (service *AdminAccessService) List(ctx context.Context) ([]AdminAccessMember, error) {
	if service == nil || service.uow == nil || service.repo == nil {
		return nil, ErrAdminAccessUnavailable
	}
	var members []authstore.AdminAccessMember
	if err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var err error
		members, err = service.repo.ListAdminAccessMembers(txCtx)
		return err
	}); err != nil {
		return nil, errors.Join(ErrAdminAccessUnavailable, err)
	}
	result := make([]AdminAccessMember, len(members))
	for index, member := range members {
		result[index] = AdminAccessMember{
			AdminUserID: member.AdminUserID, DisplayName: member.DisplayName, Role: authport.Role(member.Role),
			StaffID: cloneAdminAccessID(member.StaffID), StaffWeComUserID: member.StaffWeComUserID, StaffName: member.StaffName,
			IsActive: member.IsActive, LoginEnabled: member.LoginEnabled,
		}
	}
	return result, nil
}

func (service *AdminAccessService) Save(ctx context.Context, inputs []AdminAccessSaveInput) ([]AdminAccessMember, error) {
	if service == nil || service.uow == nil || service.repo == nil || !validAdminAccessInputs(inputs) {
		return nil, ErrInvalidAdminAccessInput
	}
	now := service.clock().UTC()
	if now.IsZero() {
		return nil, ErrAdminAccessUnavailable
	}
	result := make([]AdminAccessMember, len(inputs))
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		for index, input := range inputs {
			member, err := service.repo.SaveAdminAccessMember(txCtx, input.AdminUserID, input.LoginEnabled, now)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrAdminAccessMemberMissing
			}
			if err != nil {
				return err
			}
			result[index] = AdminAccessMember{AdminUserID: member.AdminUserID, LoginEnabled: member.LoginEnabled}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrAdminAccessMemberMissing) {
			return nil, err
		}
		return nil, errors.Join(ErrAdminAccessUnavailable, err)
	}
	return result, nil
}

func validAdminAccessInputs(inputs []AdminAccessSaveInput) bool {
	if len(inputs) < 1 || len(inputs) > 200 {
		return false
	}
	seen := make(map[int64]struct{}, len(inputs))
	for _, input := range inputs {
		if input.AdminUserID < 1 {
			return false
		}
		if _, duplicate := seen[input.AdminUserID]; duplicate {
			return false
		}
		seen[input.AdminUserID] = struct{}{}
	}
	return true
}

func cloneAdminAccessID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
