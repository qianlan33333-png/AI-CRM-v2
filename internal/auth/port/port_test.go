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
		CapabilityStagesRead, CapabilityStagesWrite,
		CapabilitySegmentsRead, CapabilitySegmentsWrite,
		CapabilityMediaImagesWrite,
	} {
		if !capability.Known() {
			t.Fatalf("capability %q is not known", capability)
		}
	}
	if Capability("customers.delete").Known() {
		t.Fatal("unknown capability became known")
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
