package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
)

type adminAccessRepositoryStub struct {
	members []authstore.AdminAccessMember
	listErr error
	saveErr error
	saves   []adminAccessSaveCall
}

type adminAccessSaveCall struct {
	id      int64
	enabled bool
	at      time.Time
}

func (stub *adminAccessRepositoryStub) ListAdminAccessMembers(context.Context) ([]authstore.AdminAccessMember, error) {
	return stub.members, stub.listErr
}

func (stub *adminAccessRepositoryStub) SaveAdminAccessMember(_ context.Context, id int64, enabled bool, at time.Time) (authstore.AdminAccessSaveResult, error) {
	stub.saves = append(stub.saves, adminAccessSaveCall{id: id, enabled: enabled, at: at})
	if stub.saveErr != nil {
		return authstore.AdminAccessSaveResult{}, stub.saveErr
	}
	return authstore.AdminAccessSaveResult{AdminUserID: id, LoginEnabled: enabled}, nil
}

func TestAdminAccessListsCurrentAdminUsersAndStaffProjection(t *testing.T) {
	staffID := int64(8)
	repository := &adminAccessRepositoryStub{members: []authstore.AdminAccessMember{{
		AdminUserID: 3, DisplayName: "管理员", Role: "admin", StaffID: &staffID,
		StaffWeComUserID: "staff-8", StaffName: "成员", IsActive: true, LoginEnabled: true,
	}}}
	service := newAdminAccessService(t, repository)
	members, err := service.List(context.Background())
	if err != nil || len(members) != 1 || members[0].AdminUserID != 3 || members[0].StaffID == nil || *members[0].StaffID != 8 || members[0].StaffWeComUserID != "staff-8" || !members[0].LoginEnabled {
		t.Fatalf("List() = %#v, %v", members, err)
	}
	*repository.members[0].StaffID = 9
	if *members[0].StaffID != 8 {
		t.Fatalf("staff ID alias leaked: %#v", members[0].StaffID)
	}
}

func TestAdminAccessSaveValidatesWholeBatchAndUsesOneTransaction(t *testing.T) {
	uow := &fakeAuthUoW{}
	repository := &adminAccessRepositoryStub{}
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	service, err := NewAdminAccessService(uow, repository, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Save(context.Background(), []AdminAccessSaveInput{{AdminUserID: 4, LoginEnabled: false}, {AdminUserID: 9, LoginEnabled: true}})
	if err != nil || uow.calls != 1 || len(repository.saves) != 2 || result[0].AdminUserID != 4 || result[1].LoginEnabled != true {
		t.Fatalf("Save() = %#v, %v; uow=%d saves=%#v", result, err, uow.calls, repository.saves)
	}
	for _, save := range repository.saves {
		if !save.at.Equal(now.UTC()) {
			t.Fatalf("save time=%v, want %v", save.at, now.UTC())
		}
	}
	for _, invalid := range [][]AdminAccessSaveInput{nil, {}, {{AdminUserID: 0}}, {{AdminUserID: 7}, {AdminUserID: 7}}} {
		if _, err := service.Save(context.Background(), invalid); !errors.Is(err, ErrInvalidAdminAccessInput) {
			t.Fatalf("invalid Save(%#v) error=%v", invalid, err)
		}
	}
	if uow.calls != 1 || len(repository.saves) != 2 {
		t.Fatalf("invalid inputs reached repository uow=%d saves=%d", uow.calls, len(repository.saves))
	}
}

func TestAdminAccessSaveKeepsMissingMemberAndUnavailableDistinct(t *testing.T) {
	missing := newAdminAccessService(t, &adminAccessRepositoryStub{saveErr: pgx.ErrNoRows})
	if _, err := missing.Save(context.Background(), []AdminAccessSaveInput{{AdminUserID: 2, LoginEnabled: false}}); !errors.Is(err, ErrAdminAccessMemberMissing) {
		t.Fatalf("missing Save error=%v", err)
	}
	unavailable := newAdminAccessService(t, &adminAccessRepositoryStub{saveErr: errors.New("db down")})
	if _, err := unavailable.Save(context.Background(), []AdminAccessSaveInput{{AdminUserID: 2, LoginEnabled: false}}); !errors.Is(err, ErrAdminAccessUnavailable) || errors.Is(err, ErrAdminAccessMemberMissing) {
		t.Fatalf("unavailable Save error=%v", err)
	}
}

func newAdminAccessService(t *testing.T, repository *adminAccessRepositoryStub) *AdminAccessService {
	t.Helper()
	service, err := NewAdminAccessService(&fakeAuthUoW{}, repository, func() time.Time { return time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	return service
}
