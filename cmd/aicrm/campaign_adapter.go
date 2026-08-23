package main

import (
	"errors"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	"net/http"
)

type legacyCampaignAuthorizer struct{}

func (legacyCampaignAuthorizer) Authorize(request *http.Request, requirement campaign.AccessRequirement) (campaign.Actor, error) {
	if request == nil {
		return campaign.Actor{}, campaign.ErrUnauthenticated
	}
	principal, pok := authport.PrincipalFromContext(request.Context())
	authorization, aok := authport.AuthorizationFromContext(request.Context())
	if !pok || principal.AdminUserID < 1 {
		return campaign.Actor{}, campaign.ErrUnauthenticated
	}
	if !aok || authorization.Scope != authport.ScopeGlobal {
		return campaign.Actor{}, campaign.ErrForbidden
	}
	expected := authport.CapabilityAdminRead
	if requirement.Capability == campaign.CapabilityOperationsRead {
		expected = authport.CapabilityOperationsRead
	} else if requirement.Capability == campaign.CapabilityManageAutomation {
		expected = authport.CapabilityOperationsManage
	}
	if requirement.Capability != campaign.CapabilityAdminRead && requirement.Capability != campaign.CapabilityOperationsRead && requirement.Capability != campaign.CapabilityManageAutomation || authorization.Capability != expected {
		return campaign.Actor{}, campaign.ErrForbidden
	}
	return campaign.Actor{ID: principal.AdminUserID}, nil
}

type legacyCampaignCSRF struct{}

func (legacyCampaignCSRF) Verify(request *http.Request, actor campaign.Actor) error {
	if request == nil || actor.ID < 1 {
		return campaign.ErrCSRFInvalid
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Scope != authport.ScopeGlobal || authorization.Capability != authport.CapabilityOperationsManage {
		return errors.Join(campaign.ErrCSRFInvalid, campaign.ErrForbidden)
	}
	return nil
}
