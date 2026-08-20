package app

import authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"

type capabilityPolicy struct {
	admin authport.ScopeKind
	ops   authport.ScopeKind
	sales authport.ScopeKind
}

// capabilityPolicies is deliberately closed. A new operation remains denied
// until its OpenAPI contract and this table are frozen in the same slice.
var capabilityPolicies = map[authport.Capability]capabilityPolicy{
	authport.CapabilityAuthSessionRead: {
		admin: authport.ScopeSelf, ops: authport.ScopeSelf, sales: authport.ScopeSelf,
	},
	authport.CapabilityAuthSessionLogout: {
		admin: authport.ScopeSelf, ops: authport.ScopeSelf, sales: authport.ScopeSelf,
	},
	authport.CapabilityCustomersRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal, sales: authport.ScopeOwnerStaff,
	},
	authport.CapabilityCustomersWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal, sales: authport.ScopeOwnerStaff,
	},
	authport.CapabilityCustomerEventsRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal, sales: authport.ScopeOwnerStaff,
	},
	authport.CapabilityIdentityResolve: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityIdentityBind: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityIdentityIngest: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityIdentityReviewRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityIdentityReviewWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityConfigOverviewRead: {
		admin: authport.ScopeGlobal,
	},
	authport.CapabilityConfigSettingsManage: {
		admin: authport.ScopeGlobal,
	},
	authport.CapabilityStagesRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal, sales: authport.ScopeGlobal,
	},
	authport.CapabilityStagesWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilitySegmentsRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilitySegmentsWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityOutboundRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal, sales: authport.ScopeOwnerStaff,
	},
	authport.CapabilityOutboundControl: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityProductsRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityProductsWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityEntitlementsRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityEntitlementsWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityMediaImagesWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityMediaLibraryRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityMediaLibraryWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityQuestionnairesRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityQuestionnairesWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityChannelsRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityChannelsWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityCouponsRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityCouponsWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityOrderRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityOrderWrite: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityMessageArchiveRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityMessageArchiveExecute: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityMessageArchiveExternalRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityOperationsRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityOperationsManage: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
	authport.CapabilityAdminRead: {
		admin: authport.ScopeGlobal,
	},
	authport.CapabilityAdminShellRead: {
		admin: authport.ScopeGlobal, ops: authport.ScopeGlobal,
	},
}

func authorize(principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if !validPrincipal(principal) {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	policy, ok := capabilityPolicies[capability]
	if !ok {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	var scope authport.ScopeKind
	switch principal.Role {
	case authport.RoleAdmin:
		scope = policy.admin
	case authport.RoleOps:
		scope = policy.ops
	case authport.RoleSales:
		scope = policy.sales
	}
	if scope == "" {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	authorization := authport.Authorization{Capability: capability, Scope: scope}
	if scope == authport.ScopeOwnerStaff {
		if principal.StaffID == nil || *principal.StaffID < 1 {
			return authport.Authorization{}, authport.ErrUnauthorized
		}
		authorization.OwnerStaffID = *principal.StaffID
	}
	return authorization, nil
}
