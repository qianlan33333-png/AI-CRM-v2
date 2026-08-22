// Package legacyaudiencemembers exposes the CRM-local, read-only AI Audience
// member snapshot. Segment membership remains owned by public.segment_members;
// this package never refreshes, copies or mutates audience state.
package legacyaudiencemembers

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const (
	RoutePrefix  = "/api/admin/ai-audience"
	RoutePattern = RoutePrefix + "/packages/{package_id}/members"

	CapabilitySegmentsRead = "segments.read"

	DefaultLimit = 50
	MaximumLimit = 200

	UnnamedCustomer = "未命名客户"
)

var (
	ErrInvalidInput    = errors.New("invalid AI Audience member snapshot input")
	ErrNotFound        = errors.New("AI Audience package not found")
	ErrUnavailable     = errors.New("AI Audience member snapshot dependency unavailable")
	ErrUnauthenticated = errors.New("AI Audience member snapshot authentication required")
	ErrForbidden       = errors.New("AI Audience member snapshot permission denied")
)

type AccessRequirement struct {
	Capability  string
	RequireCSRF bool
}

// Security is the narrow adapter Lane E mounts over the existing human-session
// and RBAC stack. Implementations return ErrUnauthenticated or ErrForbidden.
type Security interface {
	Authorize(*http.Request, AccessRequirement) error
}

type RouteSpec struct {
	Method       string
	Pattern      string
	Capability   string
	RequiresCSRF bool
}

func RouteSpecs() []RouteSpec {
	return []RouteSpec{{
		Method:       http.MethodGet,
		Pattern:      RoutePattern,
		Capability:   CapabilitySegmentsRead,
		RequiresCSRF: false,
	}}
}

type ListInput struct {
	PackageID int64
	Limit     int
	Offset    int64
}

// MemberRecord is the closed local row returned by the member repository. It
// intentionally contains no external identity value or raw customer payload.
type MemberRecord struct {
	CustomerID int64
	Nickname   string
	EnteredAt  time.Time
}

type MemberPage struct {
	Items []MemberRecord
	Total int64
}

// TrustedExternalIdentity is an already-resolved safe projection. The adapter
// must return at most one trusted primary WeCom external_userid per customer.
type TrustedExternalIdentity struct {
	CustomerID     int64
	ExternalUserID string
}

// PackageExistenceReader verifies that the authoritative Segment and its local
// AI Audience package metadata both exist.
type PackageExistenceReader interface {
	PackageExists(context.Context, int64) (bool, error)
}

// MemberRepository reads only public.segment_members and public.customers.
type MemberRepository interface {
	ListMembers(context.Context, int64, int, int64) (MemberPage, error)
}

// TrustedIdentityReader is a reverse OneID projection supplied by Lane E. It
// must omit customers with no unique trusted primary external_userid; this
// package then emits an empty string for them.
type TrustedIdentityReader interface {
	ListPrimaryExternalUserIDs(context.Context, []int64) ([]TrustedExternalIdentity, error)
}

type MemberItem struct {
	CustomerID     int64     `json:"customer_id"`
	Nickname       string    `json:"nickname"`
	ExternalUserID string    `json:"external_userid"`
	EnteredAt      time.Time `json:"entered_at"`
}

type ListResponse struct {
	OK                       bool         `json:"ok"`
	Items                    []MemberItem `json:"items"`
	Total                    int64        `json:"total"`
	Count                    int          `json:"count"`
	Limit                    int          `json:"limit"`
	Offset                   int64        `json:"offset"`
	RealExternalCallExecuted bool         `json:"real_external_call_executed"`
}

type Application interface {
	ListMembers(context.Context, ListInput) (ListResponse, error)
}
