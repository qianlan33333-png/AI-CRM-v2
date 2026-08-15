package app

import (
	"context"
	"errors"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestAuthorizeFrozenCapabilityMatrix(t *testing.T) {
	salesStaffID := int64(42)
	capabilities := []struct {
		capability authport.Capability
		admin      authport.ScopeKind
		ops        authport.ScopeKind
		sales      authport.ScopeKind
	}{
		{authport.CapabilityAuthSessionRead, authport.ScopeSelf, authport.ScopeSelf, authport.ScopeSelf},
		{authport.CapabilityAuthSessionLogout, authport.ScopeSelf, authport.ScopeSelf, authport.ScopeSelf},
		{authport.CapabilityCustomersRead, authport.ScopeGlobal, authport.ScopeGlobal, authport.ScopeOwnerStaff},
		{authport.CapabilityCustomersWrite, authport.ScopeGlobal, authport.ScopeGlobal, authport.ScopeOwnerStaff},
		{authport.CapabilityCustomerEventsRead, authport.ScopeGlobal, authport.ScopeGlobal, authport.ScopeOwnerStaff},
		{authport.CapabilityIdentityResolve, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityIdentityBind, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityIdentityIngest, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityIdentityReviewRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityIdentityReviewWrite, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityConfigOverviewRead, authport.ScopeGlobal, "", ""},
		{authport.CapabilityConfigSettingsManage, authport.ScopeGlobal, "", ""},
		{authport.CapabilityStagesRead, authport.ScopeGlobal, authport.ScopeGlobal, authport.ScopeGlobal},
		{authport.CapabilityStagesWrite, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilitySegmentsRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilitySegmentsWrite, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityOutboundRead, authport.ScopeGlobal, authport.ScopeGlobal, authport.ScopeOwnerStaff},
		{authport.CapabilityOutboundControl, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityMediaImagesWrite, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityQuestionnairesRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityQuestionnairesWrite, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityChannelsRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityChannelsWrite, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityCouponsRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityOrderRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityOrderWrite, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityMessageArchiveRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityMessageArchiveExecute, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityMessageArchiveExternalRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityCouponsWrite, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityOperationsRead, authport.ScopeGlobal, authport.ScopeGlobal, ""},
		{authport.CapabilityOperationsManage, authport.ScopeGlobal, authport.ScopeGlobal, ""},
	}
	principals := []struct {
		name      string
		principal authport.Principal
		scope     func(authport.ScopeKind, authport.ScopeKind, authport.ScopeKind) authport.ScopeKind
	}{
		{
			name:      "admin",
			principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin},
			scope: func(admin, _, _ authport.ScopeKind) authport.ScopeKind {
				return admin
			},
		},
		{
			name:      "ops",
			principal: authport.Principal{AdminUserID: 2, Role: authport.RoleOps},
			scope: func(_, ops, _ authport.ScopeKind) authport.ScopeKind {
				return ops
			},
		},
		{
			name:      "sales",
			principal: authport.Principal{AdminUserID: 3, Role: authport.RoleSales, StaffID: &salesStaffID},
			scope: func(_, _, sales authport.ScopeKind) authport.ScopeKind {
				return sales
			},
		},
	}

	for _, principalCase := range principals {
		for _, capabilityCase := range capabilities {
			t.Run(principalCase.name+"/"+string(capabilityCase.capability), func(t *testing.T) {
				wantScope := principalCase.scope(capabilityCase.admin, capabilityCase.ops, capabilityCase.sales)
				got, err := authorize(principalCase.principal, capabilityCase.capability)
				if wantScope == "" {
					assertPolicyDenied(t, got, err)
					return
				}
				if err != nil {
					t.Fatalf("authorize() error = %v", err)
				}

				want := authport.Authorization{
					Capability: capabilityCase.capability,
					Scope:      wantScope,
				}
				if wantScope == authport.ScopeOwnerStaff {
					want.OwnerStaffID = salesStaffID
				}
				if got != want {
					t.Fatalf("authorize() = %#v, want %#v", got, want)
				}
			})
		}
	}
}

func TestAuthorizeFailsClosedForUnknownCapabilitiesAndInvalidPrincipals(t *testing.T) {
	zeroStaffID := int64(0)
	negativeStaffID := int64(-1)
	tests := []struct {
		name       string
		principal  authport.Principal
		capability authport.Capability
	}{
		{
			name:       "empty capability",
			principal:  authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin},
			capability: "",
		},
		{
			name:       "future capability",
			principal:  authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin},
			capability: authport.Capability("customers.delete"),
		},
		{
			name:       "zero admin user id",
			principal:  authport.Principal{Role: authport.RoleAdmin},
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "negative admin user id",
			principal:  authport.Principal{AdminUserID: -1, Role: authport.RoleAdmin},
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "unknown role",
			principal:  authport.Principal{AdminUserID: 1, Role: authport.Role("superuser")},
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "admin zero staff id",
			principal:  authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin, StaffID: &zeroStaffID},
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "ops negative staff id",
			principal:  authport.Principal{AdminUserID: 1, Role: authport.RoleOps, StaffID: &negativeStaffID},
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "sales missing staff id",
			principal:  authport.Principal{AdminUserID: 1, Role: authport.RoleSales},
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "sales zero staff id",
			principal:  authport.Principal{AdminUserID: 1, Role: authport.RoleSales, StaffID: &zeroStaffID},
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "sales negative staff id",
			principal:  authport.Principal{AdminUserID: 1, Role: authport.RoleSales, StaffID: &negativeStaffID},
			capability: authport.CapabilityCustomersRead,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := authorize(testCase.principal, testCase.capability)
			assertPolicyDenied(t, got, err)
		})
	}
}

func TestServiceAuthorizeFailsClosedForNilAndCancelledContext(t *testing.T) {
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	service := &Service{}
	tests := []struct {
		name       string
		ctx        context.Context
		want       authport.Authorization
		wantDenied bool
	}{
		{name: "nil context", wantDenied: true},
		{name: "cancelled context", ctx: cancelledContext, wantDenied: true},
		{
			name: "live context",
			ctx:  context.Background(),
			want: authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
		},
	}

	principal := authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := service.Authorize(testCase.ctx, principal, authport.CapabilityCustomersRead)
			if testCase.wantDenied {
				assertPolicyDenied(t, got, err)
				return
			}
			if err != nil || got != testCase.want {
				t.Fatalf("Authorize() = %#v, %v; want %#v, nil", got, err, testCase.want)
			}
		})
	}
}

func TestAuthorizationAllowsOwner(t *testing.T) {
	tests := []struct {
		name          string
		authorization authport.Authorization
		ownerStaffID  int64
		want          bool
	}{
		{
			name: "global accepts first positive owner",
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			ownerStaffID: 1,
			want:         true,
		},
		{
			name: "global accepts another positive owner",
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			ownerStaffID: 99,
			want:         true,
		},
		{
			name: "owner staff accepts matching owner",
			authorization: authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: 42,
			},
			ownerStaffID: 42,
			want:         true,
		},
		{
			name: "owner staff rejects another owner",
			authorization: authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: 42,
			},
			ownerStaffID: 43,
		},
		{
			name: "self rejects an owner",
			authorization: authport.Authorization{
				Capability: authport.CapabilityAuthSessionRead,
				Scope:      authport.ScopeSelf,
			},
			ownerStaffID: 42,
		},
		{
			name: "global rejects zero owner",
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
		},
		{
			name: "owner staff rejects negative owner",
			authorization: authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: 42,
			},
			ownerStaffID: -1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.authorization.AllowsOwner(testCase.ownerStaffID); got != testCase.want {
				t.Fatalf("AllowsOwner(%d) = %t, want %t", testCase.ownerStaffID, got, testCase.want)
			}
		})
	}
}

func TestAuthorizationContextRejectsInvalidAuthorization(t *testing.T) {
	validOwnerStaffAuthorization := authport.Authorization{
		Capability:   authport.CapabilityCustomersRead,
		Scope:        authport.ScopeOwnerStaff,
		OwnerStaffID: 42,
	}
	tests := []struct {
		name       string
		ctx        context.Context
		authorize  authport.Authorization
		wantDenied bool
	}{
		{
			name:      "valid self authorization",
			ctx:       context.Background(),
			authorize: authport.Authorization{Capability: authport.CapabilityAuthSessionRead, Scope: authport.ScopeSelf},
		},
		{
			name:      "valid global authorization",
			ctx:       context.Background(),
			authorize: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
		},
		{
			name:      "valid owner staff authorization",
			ctx:       context.Background(),
			authorize: validOwnerStaffAuthorization,
		},
		{
			name:       "nil context",
			authorize:  validOwnerStaffAuthorization,
			wantDenied: true,
		},
		{
			name:       "unknown capability",
			ctx:        context.Background(),
			authorize:  authport.Authorization{Capability: authport.Capability("customers.delete"), Scope: authport.ScopeGlobal},
			wantDenied: true,
		},
		{
			name:       "unknown scope",
			ctx:        context.Background(),
			authorize:  authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeKind("team")},
			wantDenied: true,
		},
		{
			name:       "self with owner",
			ctx:        context.Background(),
			authorize:  authport.Authorization{Capability: authport.CapabilityAuthSessionRead, Scope: authport.ScopeSelf, OwnerStaffID: 42},
			wantDenied: true,
		},
		{
			name:       "global with owner",
			ctx:        context.Background(),
			authorize:  authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal, OwnerStaffID: 42},
			wantDenied: true,
		},
		{
			name:       "owner staff without owner",
			ctx:        context.Background(),
			authorize:  authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff},
			wantDenied: true,
		},
		{
			name:       "owner staff with negative owner",
			ctx:        context.Background(),
			authorize:  authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: -1},
			wantDenied: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, err := authport.WithAuthorization(testCase.ctx, testCase.authorize)
			if testCase.wantDenied {
				if ctx != nil || !errors.Is(err, authport.ErrUnauthorized) {
					t.Fatalf("WithAuthorization() = %v, %v; want nil, ErrUnauthorized", ctx, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("WithAuthorization() error = %v", err)
			}
			got, ok := authport.AuthorizationFromContext(ctx)
			if !ok || got != testCase.authorize {
				t.Fatalf("AuthorizationFromContext() = %#v, %t; want %#v, true", got, ok, testCase.authorize)
			}
		})
	}

	if _, ok := authport.AuthorizationFromContext(nil); ok {
		t.Fatal("AuthorizationFromContext(nil) returned an authorization")
	}
}

func assertPolicyDenied(t *testing.T, authorization authport.Authorization, err error) {
	t.Helper()
	if !errors.Is(err, authport.ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
	if authorization != (authport.Authorization{}) {
		t.Fatalf("authorization = %#v, want zero value", authorization)
	}
}
