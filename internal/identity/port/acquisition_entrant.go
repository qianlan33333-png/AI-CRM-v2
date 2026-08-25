package port

import (
	"context"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// AcquisitionEntrantIdentityResolution is the narrow, transaction-bound
// answer CH03 needs before Contact can write an entrant receipt. It never
// creates a Customer or exposes an arbitrary identity projection.
type AcquisitionEntrantIdentityResolution struct {
	Status     AcquisitionEntrantIdentityStatus
	CustomerID contactport.CustomerID
}

type AcquisitionEntrantIdentityStatus string

const (
	AcquisitionEntrantIdentityFound    AcquisitionEntrantIdentityStatus = "found"
	AcquisitionEntrantIdentityNotFound AcquisitionEntrantIdentityStatus = "not_found"
	AcquisitionEntrantIdentityConflict AcquisitionEntrantIdentityStatus = "conflict"
)

// AcquisitionEntrantIdentityResolver must run in the caller's active
// UnitOfWork. Implementations lock the unique verified identity and its
// effective active Customer before returning Found.
type AcquisitionEntrantIdentityResolver interface {
	ResolveAcquisitionEntrantIdentity(context.Context, IDRef) (AcquisitionEntrantIdentityResolution, error)
}
