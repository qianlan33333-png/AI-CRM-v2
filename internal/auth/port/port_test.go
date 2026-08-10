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
		CapabilityConfigOverviewRead,
	} {
		if !capability.Known() {
			t.Fatalf("capability %q is not known", capability)
		}
	}
	if Capability("customers.delete").Known() {
		t.Fatal("unknown capability became known")
	}
}
