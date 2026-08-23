package main

import (
	"context"
	"errors"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// campaignContactEligibilityAdapter is the composition-root mapping from the
// Contact-owned closed preview result into Campaign's narrow initiation port.
// It neither reads Contact tables nor invents an exclusion reason.
type campaignContactEligibilityAdapter struct {
	checker contactport.EligibilityChecker
}

var _ campaignport.EligibilityChecker = (*campaignContactEligibilityAdapter)(nil)

func newCampaignContactEligibilityAdapter(checker contactport.EligibilityChecker) (*campaignContactEligibilityAdapter, error) {
	if checker == nil {
		return nil, errors.New("campaign contact eligibility checker is required")
	}
	return &campaignContactEligibilityAdapter{checker: checker}, nil
}

func (adapter *campaignContactEligibilityAdapter) CheckCampaignEligibility(ctx context.Context, request campaignport.EligibilityRequest) ([]campaignport.EligibilityDecision, error) {
	if adapter == nil || adapter.checker == nil || ctx == nil || ctx.Err() != nil || request.Checkpoint != campaignport.EligibilityCheckpointPreview ||
		request.MaximumTargets != campaignport.MaximumEligibilityTargets || request.EvaluatedAt.IsZero() ||
		len(request.CustomerIDs) < 1 || len(request.CustomerIDs) > campaignport.MaximumEligibilityTargets {
		return nil, contactport.ErrInvalidContactEligibility
	}
	customerIDs := make([]contactport.CustomerID, len(request.CustomerIDs))
	for index, customerID := range request.CustomerIDs {
		if customerID < 1 || index > 0 && request.CustomerIDs[index-1] >= customerID {
			return nil, contactport.ErrInvalidContactEligibility
		}
		customerIDs[index] = contactport.CustomerID(customerID)
	}
	decisions, err := adapter.checker.CheckContactEligibility(ctx, contactport.ContactEligibilityCheck{
		Checkpoint: contactport.ContactEligibilityPreview, CustomerIDs: customerIDs, EvaluatedAt: request.EvaluatedAt.UTC(),
	})
	if err != nil || len(decisions) != len(customerIDs) {
		if err != nil {
			return nil, err
		}
		return nil, contactport.ErrContactEligibilityUnavailable
	}
	result := make([]campaignport.EligibilityDecision, len(decisions))
	for index, decision := range decisions {
		if decision.CustomerID != customerIDs[index] {
			return nil, contactport.ErrContactEligibilityUnavailable
		}
		exclusion, valid := mapCampaignEligibilityExclusion(decision.Exclusion)
		if !valid {
			return nil, contactport.ErrContactEligibilityUnavailable
		}
		result[index] = campaignport.EligibilityDecision{CustomerID: int64(decision.CustomerID), CustomerActive: decision.CustomerActive,
			Eligible: decision.Eligible, Exclusion: exclusion}
	}
	return result, nil
}

func mapCampaignEligibilityExclusion(value contactport.ContactEligibilityExclusion) (campaignport.EligibilityExclusion, bool) {
	switch value {
	case contactport.ContactEligibilityExclusionNone:
		return campaignport.EligibilityExclusionNone, true
	case contactport.ContactEligibilityExclusionInactiveCustomer:
		return campaignport.EligibilityExclusionInactiveCustomer, true
	case contactport.ContactEligibilityExclusionContactPolicy:
		return campaignport.EligibilityExclusionContactPolicy, true
	default:
		return "", false
	}
}
