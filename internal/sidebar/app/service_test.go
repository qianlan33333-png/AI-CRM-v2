package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type corpFake struct{ value string }

func (fake corpFake) CorpID(context.Context) (string, error) { return fake.value, nil }

type identityFake struct{ result identityport.ResolveResult }

func (fake identityFake) Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error) {
	return fake.result, nil
}

type profileFake struct{ profile contactport.SidebarProfile }

func (fake *profileFake) ResolveSidebarProfile(context.Context, contactport.CustomerID) (contactport.SidebarProfile, error) {
	return fake.profile, nil
}
func (fake *profileFake) ReadSidebarProfile(context.Context, contactport.CustomerID, int64) (contactport.SidebarProfile, error) {
	return fake.profile, nil
}
func (fake *profileFake) UpdateSidebarProfile(_ context.Context, command contactport.SidebarProfileUpdateCommand) (contactport.SidebarProfile, error) {
	if command.Patch.Needs != nil {
		fake.profile.Needs = *command.Patch.Needs
	}
	fake.profile.UpdatedAt = fake.profile.UpdatedAt.Add(time.Second)
	return fake.profile, nil
}

type surveyFake struct{}

func (surveyFake) ListCustomerSurveyAnswers(context.Context, contactport.CustomerID, int32) (surveyport.CustomerSurveyAnswerPage, error) {
	return surveyport.CustomerSurveyAnswerPage{}, nil
}

type orderFake struct{ page orderport.Page }

func (fake orderFake) List(_ context.Context, filter orderport.Filter) (orderport.Page, error) {
	if filter.CustomerID == nil || *filter.CustomerID != 41 {
		return orderport.Page{}, errors.New("unscoped order read")
	}
	return fake.page, nil
}

type memberFake struct {
	member  PeriodicMember
	command PeriodicRemarkCommand
}

func (fake *memberFake) Get(context.Context, int64, string) (PeriodicMember, error) {
	return fake.member, nil
}
func (fake *memberFake) UpdateRemark(_ context.Context, command PeriodicRemarkCommand) (PeriodicMember, error) {
	fake.command = command
	fake.member.Remark = command.Remark
	fake.member.Version++
	return fake.member, nil
}
func (fake *memberFake) ListCustomer(context.Context, PeriodicListQuery) (PeriodicListResult, error) {
	return PeriodicListResult{Items: []PeriodicMember{fake.member}, Limit: 20}, nil
}

type mediaFake struct{ exists bool }

func (mediaFake) ListImages(context.Context, mediaport.ImageListQuery) (mediaport.ImageListPage, error) {
	return mediaport.ImageListPage{}, nil
}
func (mediaFake) Facets(context.Context) (mediaport.ImageFacets, error) {
	return mediaport.ImageFacets{}, nil
}
func (fake mediaFake) LocalImageExists(context.Context, int64) (bool, error) { return fake.exists, nil }

func TestContextTokenBindsViewerOwnerCorpAndExpiry(t *testing.T) {
	service, staff := sidebarTestService(t)
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staff}
	result, err := service.MintContext(context.Background(), principal, true, "wm_external_41")
	if err != nil || result.State != "ready" || result.Token == "" || result.CustomerID != 41 || result.OwnerStaffID != staff || !result.Safety.LocalOnly || result.Safety.ProviderExecutionEligible || result.Safety.RealExternalCallExecuted {
		t.Fatalf("MintContext() result=%+v err=%v", result, err)
	}
	scope, err := service.VerifyContext(context.Background(), principal, result.Token)
	if err != nil || scope.CustomerID != 41 || scope.OwnerStaffID != staff {
		t.Fatalf("VerifyContext() scope=%+v err=%v", scope, err)
	}
	other := principal
	other.AdminUserID = 10
	if _, err = service.VerifyContext(context.Background(), other, result.Token); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer replay error=%v, want forbidden", err)
	}
	if _, err = service.VerifyContext(context.Background(), principal, result.Token+"x"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("tamper error=%v, want invalid token", err)
	}
	service.now = func() time.Time { return result.ExpiresAt }
	if _, err = service.VerifyContext(context.Background(), principal, result.Token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expiry error=%v, want expired token", err)
	}
}

func TestMintContextReturnsExplicitViewerStateAndRejectsWrongOwner(t *testing.T) {
	service, staff := sidebarTestService(t)
	result, err := service.MintContext(context.Background(), authport.Principal{}, false, "wm_external_41")
	if err != nil || result.State != "viewer_session_required" || !result.Safety.LocalOnly {
		t.Fatalf("viewer state=%+v err=%v", result, err)
	}
	other := staff + 1
	_, err = service.MintContext(context.Background(), authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &other}, true, "wm_external_41")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong owner error=%v", err)
	}
}

func TestOrdersRedactPayerAndIdentityFields(t *testing.T) {
	service, staff := sidebarTestService(t)
	service.orders = orderFake{page: orderport.Page{Items: []orderport.Item{{MerchantOrderNo: "order-41", PayerName: "secret-name", Mobile: "13800000000", UnionID: "secret-union", ProductName: "local product", AmountYuan: "9.90", Currency: "CNY", Status: "paid", Provider: "wechat_pay"}}, Total: 1, Limit: 20}}
	result, err := service.Orders(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	for _, forbidden := range []string{"secret-name", "13800000000", "secret-union", "payer_name", "unionid"} {
		if stringContains(string(raw), forbidden) {
			t.Fatalf("unsafe order projection %s", raw)
		}
	}
}

func TestPeriodicRemarkUsesCanonicalKeyCASAndCustomerScope(t *testing.T) {
	service, staff := sidebarTestService(t)
	members := service.members.(*memberFake)
	remark := "renew next week"
	updated, err := service.UpdatePeriodicRemark(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}}, 71, members.member.MemberRef, 3, &remark, "sidebar-remark-0001")
	if err != nil || updated.Member.Version != 4 || !updated.Safety.LocalOnly || members.command.ServiceProductID != 71 || members.command.MemberRef != members.member.MemberRef || members.command.ExpectedVersion != 3 {
		t.Fatalf("UpdatePeriodicRemark() member=%+v command=%+v err=%v", updated, members.command, err)
	}
	members.member.CustomerID = 42
	if _, err = service.UpdatePeriodicRemark(context.Background(), Scope{CustomerID: 41, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}}, 71, members.member.MemberRef, 4, &remark, "sidebar-remark-0002"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-customer error=%v", err)
	}
}

func sidebarTestService(t *testing.T) (*Service, int64) {
	t.Helper()
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	staff := int64(7)
	profiles := &profileFake{profile: contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: staff, Name: "customer", UpdatedAt: now}}
	members := &memberFake{member: PeriodicMember{MemberRef: "spm_abcdefghijklmnopqrstuv", ServiceProductID: 71, CustomerID: 41, State: "active", Source: "manual", StartsAt: now.Add(-time.Hour), Version: 3, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}}
	service, err := NewService(corpFake{"corp-1"}, identityFake{identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 41}}, profiles, surveyFake{}, orderFake{}, members, mediaFake{exists: true}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	return service, staff
}

func stringContains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
