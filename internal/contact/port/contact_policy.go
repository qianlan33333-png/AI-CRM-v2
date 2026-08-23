package port

import (
	"context"
	"errors"
	"time"
)

type ContactEligibilityCheckpoint string

const (
	ContactEligibilityPreview  ContactEligibilityCheckpoint = "preview"
	ContactEligibilityDispatch ContactEligibilityCheckpoint = "dispatch"
	// ContactEligibilityMaximumCustomers matches the first Campaign target
	// snapshot contract. Larger Outbound batches are not part of this port.
	ContactEligibilityMaximumCustomers = 1000
)

var (
	ErrInvalidContactEligibility     = errors.New("invalid contact eligibility check")
	ErrContactEligibilityUnavailable = errors.New("contact eligibility unavailable")
)

type ContactEligibilityExclusion string

const (
	ContactEligibilityExclusionNone             ContactEligibilityExclusion = "none"
	ContactEligibilityExclusionInactiveCustomer ContactEligibilityExclusion = "inactive_customer"
	ContactEligibilityExclusionContactPolicy    ContactEligibilityExclusion = "contact_policy"
)

type ContactEligibilityCheck struct {
	Checkpoint  ContactEligibilityCheckpoint
	CustomerIDs []CustomerID
	EvaluatedAt time.Time
}

type ContactEligibility struct {
	CustomerID     CustomerID
	CustomerActive bool
	Eligible       bool
	Exclusion      ContactEligibilityExclusion
}

// EligibilityChecker is a transaction-bound Contact read contract. Callers
// must invoke it with their UnitOfWork context at preview and again while they
// atomically reserve the local dispatch attempt. Its locks serialize policy
// writes only for that database transaction; they are never held across a
// provider call. The complete result is sorted by CustomerID; callers decide
// whether exclusions are omitted during preview or fail the dispatch closed.
// This checker never creates a task or calls a provider.
type EligibilityChecker interface {
	CheckContactEligibility(context.Context, ContactEligibilityCheck) ([]ContactEligibility, error)
}
