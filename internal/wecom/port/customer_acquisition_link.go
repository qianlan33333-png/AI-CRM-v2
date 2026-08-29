package port

import (
	"context"
	"errors"
)

var (
	ErrCustomerAcquisitionLinkNotDispatched = errors.New("customer acquisition link provider call not dispatched")
	// ErrCustomerAcquisitionLinkNotFound is returned only when the Provider
	// explicitly reports that a link is absent. Transport and malformed
	// responses deliberately remain unknown outcomes.
	ErrCustomerAcquisitionLinkNotFound = errors.New("customer acquisition link provider link not found")
)

type CustomerAcquisitionLinkInput struct {
	LinkName      string
	UserIDs       []string
	DepartmentIDs []int64
	SkipVerify    bool
}

type CustomerAcquisitionLink struct {
	LinkID        string
	LinkName      string
	URL           string
	UserIDs       []string
	DepartmentIDs []int64
	SkipVerify    bool
}

type CustomerAcquisitionLinkPage struct {
	Links      []CustomerAcquisitionLink
	NextCursor string
}

type CustomerAcquisitionLinkWriteOutcome string

const (
	CustomerAcquisitionLinkExecuted       CustomerAcquisitionLinkWriteOutcome = "executed"
	CustomerAcquisitionLinkFinalFailed    CustomerAcquisitionLinkWriteOutcome = "final_failed"
	CustomerAcquisitionLinkOutcomeUnknown CustomerAcquisitionLinkWriteOutcome = "outcome_unknown"
)

// CustomerAcquisitionLinkWriteResult records a bounded command outcome. Its
// digest is a local correlation value, never a Provider receipt or delivery
// claim. OutcomeUnknown must be reconciled and must never be retried
// automatically with the same business command.
type CustomerAcquisitionLinkWriteResult struct {
	Outcome                    CustomerAcquisitionLinkWriteOutcome
	Link                       *CustomerAcquisitionLink
	OutcomeDigest              [32]byte
	BusinessEndpointDispatched bool
	RealExternalCallExecuted   bool
}

// CustomerAcquisitionLinkProvider is the narrow official WeCom boundary. It
// intentionally has no enable/disable methods because the provider contract
// does not expose those legacy transitions.
type CustomerAcquisitionLinkProvider interface {
	ListCustomerAcquisitionLinks(context.Context, string, int) (CustomerAcquisitionLinkPage, error)
	GetCustomerAcquisitionLink(context.Context, string) (CustomerAcquisitionLink, error)
	CreateCustomerAcquisitionLink(context.Context, CustomerAcquisitionLinkInput) (CustomerAcquisitionLinkWriteResult, error)
	UpdateCustomerAcquisitionLink(context.Context, string, CustomerAcquisitionLinkInput) (CustomerAcquisitionLinkWriteResult, error)
	DeleteCustomerAcquisitionLink(context.Context, string) (CustomerAcquisitionLinkWriteResult, error)
}
