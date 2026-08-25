// Package legacyaudience restores the legacy AI Audience package grouping and
// local display lifecycle. Segment definitions, refresh facts and member facts
// remain owned by the existing Segment domain.
package legacyaudience

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const (
	RoutePrefix = "/api/admin/ai-audience"

	CapabilitySegmentsRead     = "segments.read"
	CapabilitySegmentsWrite    = "segments.write"
	CapabilityOperationsManage = "operations.manage"

	DefaultLimit  = 50
	MaximumLimit  = 100
	MaximumOffset = 100000

	MaximumRequestBodyBytes int64 = 64 << 10
	MaximumGroupNameRunes         = 100
	MaximumPackageNameRunes       = 200
)

type PackageLifecycle string

const (
	PackagePaused   PackageLifecycle = "paused"
	PackageActive   PackageLifecycle = "active"
	PackageArchived PackageLifecycle = "archived"
)

type Actor struct {
	AdminUserID int64
}

type AccessRequirement struct {
	Capability  string
	RequireCSRF bool
}

// Security is an injectable adapter over the central session/RBAC/CSRF stack.
// Implementations must return ErrUnauthenticated, ErrForbidden or
// ErrCSRFInvalid for the corresponding closed failure class.
type Security interface {
	Authorize(*http.Request, AccessRequirement) (Actor, error)
}

type RouteSpec struct {
	Method       string
	Pattern      string
	Capability   string
	RequiresCSRF bool
}

func RouteSpecs() []RouteSpec {
	return []RouteSpec{
		{http.MethodGet, RoutePrefix + "/package-groups", CapabilitySegmentsRead, false},
		{http.MethodPost, RoutePrefix + "/package-groups", CapabilitySegmentsWrite, true},
		{http.MethodPatch, RoutePrefix + "/package-groups/{group_id}", CapabilitySegmentsWrite, true},
		{http.MethodDelete, RoutePrefix + "/package-groups/{group_id}", CapabilitySegmentsWrite, true},
		{http.MethodGet, RoutePrefix + "/packages", CapabilitySegmentsRead, false},
		{http.MethodGet, RoutePrefix + "/packages/{package_id}", CapabilitySegmentsRead, false},
		{http.MethodPatch, RoutePrefix + "/packages/{package_id}", CapabilitySegmentsWrite, true},
		{http.MethodPost, RoutePrefix + "/packages/{package_id}/copy", CapabilitySegmentsWrite, true},
		{http.MethodPost, RoutePrefix + "/packages/{package_id}/pause", CapabilitySegmentsWrite, true},
		{http.MethodPost, RoutePrefix + "/packages/{package_id}/activate", CapabilitySegmentsWrite, true},
		{http.MethodDelete, RoutePrefix + "/packages/{package_id}", CapabilitySegmentsWrite, true},
	}
}

// UnitOfWork supplies one transaction-bound context for each business write.
type UnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}

// SegmentReader is the only source used for package definitions, refresh facts
// and member counts on read paths.
type SegmentReader interface {
	Get(context.Context, segmentport.SegmentID) (segmentport.Segment, error)
}

type LocalEvent struct {
	Type           string
	Payload        json.RawMessage
	OccurredAt     time.Time
	IdempotencyKey string
}

// EventAppender must persist the event in the transaction carried by ctx. It
// must not dispatch work or invoke a provider.
type EventAppender interface {
	Append(context.Context, LocalEvent) error
}

type Group struct {
	ID        int64     `json:"group_id"`
	Name      string    `json:"name"`
	SortOrder int32     `json:"sort_order"`
	Version   int64     `json:"version"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PackageMetadata struct {
	SegmentID int64
	GroupID   *int64
	Lifecycle PackageLifecycle
	Version   int64
	CreatedBy int64
	UpdatedBy int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PackageWriteModel struct {
	SegmentID        int64
	Name             string
	Definition       segmentport.Definition
	RefreshMode      segmentport.RefreshMode
	RefreshCron      *string
	SegmentLifecycle segmentport.LifecycleStatus
	Metadata         PackageMetadata
}

type Package struct {
	ID            int64                     `json:"package_id"`
	Name          string                    `json:"name"`
	Definition    segmentport.Definition    `json:"definition,omitempty"`
	GroupID       *int64                    `json:"group_id"`
	Lifecycle     PackageLifecycle          `json:"lifecycle"`
	Version       int64                     `json:"version"`
	RefreshMode   segmentport.RefreshMode   `json:"refresh_mode"`
	RefreshCron   *string                   `json:"refresh_cron"`
	MemberCount   int64                     `json:"member_count"`
	RefreshedAt   *time.Time                `json:"refreshed_at"`
	RefreshStatus segmentport.RefreshStatus `json:"refresh_status"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
}

type PackageSummary struct {
	ID            int64                     `json:"package_id"`
	Name          string                    `json:"name"`
	GroupID       *int64                    `json:"group_id"`
	Lifecycle     PackageLifecycle          `json:"lifecycle"`
	Version       int64                     `json:"version"`
	RefreshMode   segmentport.RefreshMode   `json:"refresh_mode"`
	RefreshCron   *string                   `json:"refresh_cron"`
	MemberCount   int64                     `json:"member_count"`
	RefreshedAt   *time.Time                `json:"refreshed_at"`
	RefreshStatus segmentport.RefreshStatus `json:"refresh_status"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
}

type Projection struct {
	LocalProjection          bool `json:"local_projection"`
	RealExternalCallExecuted bool `json:"real_external_call_executed"`
}

func localProjection() Projection {
	return Projection{LocalProjection: true, RealExternalCallExecuted: false}
}

type GroupListResponse struct {
	Items []Group `json:"items"`
	Projection
}

type GroupMutationResponse struct {
	Group Group `json:"group"`
	Projection
}

type GroupDeleteResponse struct {
	GroupID int64 `json:"group_id"`
	Version int64 `json:"version"`
	Deleted bool  `json:"deleted"`
	Projection
}

type PackageListResponse struct {
	Items  []PackageSummary `json:"items"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Total  int64            `json:"total"`
	Projection
}

type PackageDetailResponse struct {
	Package Package `json:"package"`
	Projection
}

type PackageMutation struct {
	ID          int64                   `json:"package_id"`
	Name        string                  `json:"name"`
	GroupID     *int64                  `json:"group_id"`
	Lifecycle   PackageLifecycle        `json:"lifecycle"`
	Version     int64                   `json:"version"`
	RefreshMode segmentport.RefreshMode `json:"refresh_mode"`
	RefreshCron *string                 `json:"refresh_cron"`
	MemberCount *int64                  `json:"member_count,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type PackageMutationResponse struct {
	Package PackageMutation `json:"package"`
	Projection
}

type PackageArchiveResponse struct {
	PackageID int64            `json:"package_id"`
	Lifecycle PackageLifecycle `json:"lifecycle"`
	Version   int64            `json:"version"`
	Archived  bool             `json:"archived"`
	Projection
}

type OptionalString struct {
	Set   bool
	Value *string
}

type OptionalInt64 struct {
	Set   bool
	Value *int64
}

type CreateGroupInput struct {
	Name            string
	SortOrder       int32
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}

type UpdateGroupInput struct {
	GroupID         int64
	Name            *string
	SortOrder       *int32
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}

type DeleteGroupInput struct {
	GroupID         int64
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}

type ListPackagesInput struct {
	GroupID *int64
	Limit   int
	Offset  int
}

type UpdatePackageInput struct {
	PackageID       int64
	Name            *string
	Definition      *segmentport.Definition
	RefreshMode     *segmentport.RefreshMode
	RefreshCron     OptionalString
	GroupID         OptionalInt64
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}

type PackageCommand struct {
	PackageID       int64
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}

type ReceiptOperation string

const (
	OperationGroupCreate     ReceiptOperation = "group_create"
	OperationGroupUpdate     ReceiptOperation = "group_update"
	OperationGroupDelete     ReceiptOperation = "group_delete"
	OperationPackageUpdate   ReceiptOperation = "package_update"
	OperationPackageCopy     ReceiptOperation = "package_copy"
	OperationPackagePause    ReceiptOperation = "package_pause"
	OperationPackageActivate ReceiptOperation = "package_activate"
	OperationPackageArchive  ReceiptOperation = "package_archive"
)

type ReceiptReservation struct {
	Operation     ReceiptOperation
	ActorID       int64
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type Receipt struct {
	ID            int64
	Operation     ReceiptOperation
	ActorID       int64
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	State         string
	ResultJSON    json.RawMessage
}

type Repository interface {
	ListGroups(context.Context) ([]Group, error)
	LockGroup(context.Context, int64) (Group, error)
	InsertGroup(context.Context, string, int32, int64, time.Time) (Group, error)
	UpdateGroup(context.Context, Group, string, int32, int64, time.Time) (Group, error)
	CountPackagesInGroup(context.Context, int64) (int64, error)
	DeleteGroup(context.Context, int64, int64) error

	ListPackageMetadata(context.Context, *int64, int, int) ([]PackageMetadata, int64, error)
	GetPackageMetadata(context.Context, int64) (PackageMetadata, error)
	LockPackage(context.Context, int64) (PackageWriteModel, error)
	SavePackage(context.Context, PackageWriteModel, PackageWriteModel, int64, int64, time.Time) (PackageWriteModel, error)
	LockCopyNameNamespace(context.Context, string) error
	PackageNameExists(context.Context, string) (bool, error)
	InsertPackageCopy(context.Context, PackageWriteModel, string, int64, time.Time) (PackageWriteModel, error)

	ReserveReceipt(context.Context, ReceiptReservation) (Receipt, bool, error)
	CompleteReceipt(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
}

type Application interface {
	ListGroups(context.Context) (GroupListResponse, error)
	CreateGroup(context.Context, CreateGroupInput) (GroupMutationResponse, error)
	UpdateGroup(context.Context, UpdateGroupInput) (GroupMutationResponse, error)
	DeleteGroup(context.Context, DeleteGroupInput) (GroupDeleteResponse, error)
	ListPackages(context.Context, ListPackagesInput) (PackageListResponse, error)
	GetPackage(context.Context, int64) (PackageDetailResponse, error)
	UpdatePackage(context.Context, UpdatePackageInput) (PackageMutationResponse, error)
	CopyPackage(context.Context, PackageCommand) (PackageMutationResponse, error)
	PausePackage(context.Context, PackageCommand) (PackageMutationResponse, error)
	ActivatePackage(context.Context, PackageCommand) (PackageMutationResponse, error)
	ArchivePackage(context.Context, PackageCommand) (PackageArchiveResponse, error)
}
