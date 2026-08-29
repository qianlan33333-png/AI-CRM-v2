package port

import (
	"context"
	"errors"
	"testing"
)

func TestAuthorizationContextRejectsCapabilityScopeMismatch(t *testing.T) {
	tests := []Authorization{
		{Capability: CapabilityAuthSessionRead, Scope: ScopeGlobal},
		{Capability: CapabilityCustomersRead, Scope: ScopeSelf},
		{Capability: CapabilityIdentityResolve, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		{Capability: CapabilityConfigOverviewRead, Scope: ScopeSelf},
		{Capability: CapabilityStagesRead, Scope: ScopeSelf},
		{Capability: CapabilityStagesWrite, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		{Capability: CapabilitySegmentsRead, Scope: ScopeSelf},
		{Capability: CapabilitySegmentsWrite, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		{Capability: CapabilityMediaImagesWrite, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		{Capability: CapabilityQuestionnairesRead, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		{Capability: CapabilityQuestionnairesWrite, Scope: ScopeSelf},
		{Capability: CapabilityChannelsRead, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		{Capability: CapabilityChannelsWrite, Scope: ScopeSelf},
		{Capability: CapabilityCouponsRead, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		{Capability: CapabilityCouponsWrite, Scope: ScopeSelf},
		{Capability: CapabilityEntitlementsRead, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		{Capability: CapabilityEntitlementsWrite, Scope: ScopeSelf},
	}
	for _, authorization := range tests {
		ctx, err := WithAuthorization(context.Background(), authorization)
		if ctx != nil || !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("WithAuthorization(%#v) = %v, %v; want nil, ErrUnauthorized", authorization, ctx, err)
		}
	}
}

func TestFrozenCapabilitiesAreKnown(t *testing.T) {
	for _, capability := range []Capability{
		CapabilityAuthSessionRead, CapabilityAuthSessionLogout,
		CapabilityCustomersRead, CapabilityCustomersWrite, CapabilityCustomerEventsRead,
		CapabilityIdentityResolve, CapabilityIdentityBind, CapabilityIdentityIngest,
		CapabilityIdentityReviewRead, CapabilityIdentityReviewWrite,
		CapabilityConfigOverviewRead,
		CapabilityConfigSettingsManage,
		CapabilityStagesRead, CapabilityStagesWrite,
		CapabilitySegmentsRead, CapabilitySegmentsWrite,
		CapabilityProductsRead, CapabilityProductsWrite,
		CapabilityMemberGridRead, CapabilityMemberGridWrite,
		CapabilityEntitlementsRead, CapabilityEntitlementsWrite,
		CapabilityMediaImagesWrite,
		CapabilityQuestionnairesRead, CapabilityQuestionnairesWrite,
		CapabilityChannelsRead, CapabilityChannelsWrite,
		CapabilityCouponsRead, CapabilityCouponsWrite,
		CapabilityOrderRead,
		CapabilityMessageArchiveRead, CapabilityMessageArchiveExecute, CapabilityMessageArchiveExternalRead,
		CapabilityOperationsRead, CapabilityOperationsManage,
		CapabilityAdminShellRead,
		CapabilityContactOwnerReassignment,
		CapabilityReleaseRead, CapabilityReleaseManage,
	} {
		if !capability.Known() {
			t.Fatalf("capability %q is not known", capability)
		}
	}
	if Capability("customers.delete").Known() {
		t.Fatal("unknown capability became known")
	}
}

func TestMemberGridCapabilitiesAllowOnlyGlobalOrOwnerStaff(t *testing.T) {
	for _, capability := range []Capability{CapabilityMemberGridRead, CapabilityMemberGridWrite} {
		for _, authorization := range []Authorization{
			{Capability: capability, Scope: ScopeGlobal},
			{Capability: capability, Scope: ScopeOwnerStaff, OwnerStaffID: 42},
		} {
			if _, err := WithAuthorization(context.Background(), authorization); err != nil {
				t.Fatalf("capability=%q authorization=%+v error=%v", capability, authorization, err)
			}
		}
	}
}

func TestOrderReadCapabilityStaysGlobal(t *testing.T) {
	if string(CapabilityOrderRead) != "order.read" || !CapabilityOrderRead.Known() {
		t.Fatal("order read capability drifted")
	}
	ctx, err := WithAuthorization(context.Background(), Authorization{Capability: CapabilityOrderRead, Scope: ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := AuthorizationFromContext(ctx); !ok || got != (Authorization{Capability: CapabilityOrderRead, Scope: ScopeGlobal}) {
		t.Fatalf("authorization=%#v/%v", got, ok)
	}
}

func TestCouponCapabilitiesStayGlobal(t *testing.T) {
	for capability, want := range map[Capability]string{
		CapabilityCouponsRead: "coupons.read", CapabilityCouponsWrite: "coupons.write",
	} {
		if string(capability) != want || !capability.Known() {
			t.Fatalf("capability %q drifted", capability)
		}
		ctx, err := WithAuthorization(context.Background(), Authorization{Capability: capability, Scope: ScopeGlobal})
		if value, ok := AuthorizationFromContext(ctx); err != nil || !ok || value.Capability != capability || value.Scope != ScopeGlobal {
			t.Fatalf("global authorization=%#v/%v err=%v", value, ok, err)
		}
	}
}

func TestChannelCapabilitiesStayGlobal(t *testing.T) {
	for capability, want := range map[Capability]string{
		CapabilityChannelsRead: "channels.read", CapabilityChannelsWrite: "channels.write",
	} {
		if string(capability) != want || !capability.Known() {
			t.Fatalf("capability %q drifted", capability)
		}
		ctx, err := WithAuthorization(context.Background(), Authorization{Capability: capability, Scope: ScopeGlobal})
		if value, ok := AuthorizationFromContext(ctx); err != nil || !ok || value.Capability != capability || value.Scope != ScopeGlobal {
			t.Fatalf("global authorization=%#v/%v err=%v", value, ok, err)
		}
	}
}

func TestQuestionnaireCapabilitiesStayGlobal(t *testing.T) {
	for capability, want := range map[Capability]string{
		CapabilityQuestionnairesRead: "questionnaires.read", CapabilityQuestionnairesWrite: "questionnaires.write",
	} {
		if string(capability) != want || !capability.Known() {
			t.Fatalf("capability %q drifted", capability)
		}
		ctx, err := WithAuthorization(context.Background(), Authorization{Capability: capability, Scope: ScopeGlobal})
		if value, ok := AuthorizationFromContext(ctx); err != nil || !ok || value.Capability != capability || value.Scope != ScopeGlobal {
			t.Fatalf("global authorization=%#v/%v err=%v", value, ok, err)
		}
	}
}

func TestMediaImageWriteCapabilityStaysGlobal(t *testing.T) {
	if string(CapabilityMediaImagesWrite) != "media.images.write" || !CapabilityMediaImagesWrite.Known() {
		t.Fatal("media image write capability drifted")
	}
	ctx, err := WithAuthorization(context.Background(), Authorization{Capability: CapabilityMediaImagesWrite, Scope: ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := AuthorizationFromContext(ctx); !ok || value != (Authorization{Capability: CapabilityMediaImagesWrite, Scope: ScopeGlobal}) {
		t.Fatalf("authorization=%#v/%v", value, ok)
	}
}

func TestSegmentCapabilitiesStayGlobal(t *testing.T) {
	for capability, want := range map[Capability]string{
		CapabilitySegmentsRead:  "segments.read",
		CapabilitySegmentsWrite: "segments.write",
	} {
		if string(capability) != want || !capability.Known() {
			t.Fatalf("capability %q drifted", capability)
		}
		ctx, err := WithAuthorization(context.Background(), Authorization{
			Capability: capability,
			Scope:      ScopeGlobal,
		})
		if err != nil {
			t.Fatalf("WithAuthorization(%q, global) = %v; want accepted", capability, err)
		}
		got, ok := AuthorizationFromContext(ctx)
		if !ok || got != (Authorization{Capability: capability, Scope: ScopeGlobal}) {
			t.Fatalf("AuthorizationFromContext() = %#v, %v; want global %q", got, ok, capability)
		}
	}
}

func TestIdentityReviewCapabilitiesStayGlobal(t *testing.T) {
	for capability, want := range map[Capability]string{
		CapabilityIdentityReviewRead:  "identity.review.read",
		CapabilityIdentityReviewWrite: "identity.review.write",
	} {
		if string(capability) != want || !capability.Known() {
			t.Fatalf("capability %q drifted", capability)
		}
		ctx, err := WithAuthorization(context.Background(), Authorization{
			Capability: capability,
			Scope:      ScopeGlobal,
		})
		if err != nil {
			t.Fatalf("WithAuthorization(%q, global) = %v; want accepted", capability, err)
		}
		got, ok := AuthorizationFromContext(ctx)
		if !ok || got != (Authorization{Capability: capability, Scope: ScopeGlobal}) {
			t.Fatalf("AuthorizationFromContext() = %#v, %v; want global %q", got, ok, capability)
		}
	}
}
