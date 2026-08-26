// Package port freezes the cross-domain contact contract. It contains no store
// or HTTP implementation and never carries external identity values.
package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type CustomerID int64
type EventID int64
type Actor string
type StageID int64

var (
	ErrInvalidStage              = errors.New("invalid stage")
	ErrStageNotFound             = errors.New("stage not found")
	ErrStageReferenced           = errors.New("stage is still referenced by customers")
	ErrStageConflict             = errors.New("stage command conflict")
	ErrInvalidMergeCommand       = errors.New("invalid contact merge command")
	ErrMergeCustomerNotFound     = errors.New("contact merge customer not found")
	ErrMergeConflict             = errors.New("contact merge conflict")
	ErrMergeStoreFailed          = errors.New("contact merge store failed")
	ErrExternalEventConflict     = errors.New("external customer event conflict")
	ErrTagReferenceNotFound      = errors.New("contact tag reference not found")
	ErrTagReferenceUnavailable   = errors.New("contact tag reference unavailable")
	ErrStaffReferenceNotFound    = errors.New("contact staff reference not found")
	ErrStaffReferenceUnavailable = errors.New("contact staff reference unavailable")
)

var (
	ErrCustomerReadNotFound      = errors.New("customer read projection not found")
	ErrCustomerReadUnavailable   = errors.New("customer read projection unavailable")
	ErrSidebarProfileInvalid     = errors.New("sidebar customer profile invalid")
	ErrSidebarProfileNotFound    = errors.New("sidebar customer profile not found")
	ErrSidebarProfileConflict    = errors.New("sidebar customer profile conflict")
	ErrSidebarProfileUnavailable = errors.New("sidebar customer profile unavailable")
)

// CustomerProjection is deliberately channel neutral. Phone numbers and
// external identifiers remain snapshots owned by the consuming business fact.
type CustomerProjection struct {
	ID   CustomerID
	Name string
}

type CustomerReader interface {
	ReadCustomer(context.Context, CustomerID) (CustomerProjection, error)
}

// SidebarProfile is the Contact-owned, channel-neutral subset exposed to the
// customer sidebar. External identity values never fit in this contract.
type SidebarProfile struct {
	CustomerID   CustomerID `json:"customer_id"`
	Name         string     `json:"name"`
	OwnerStaffID int64      `json:"owner_staff_id"`
	Source       string     `json:"source"`
	Industry     string     `json:"industry"`
	Description  string     `json:"description"`
	Needs        string     `json:"needs"`
	PainPoints   string     `json:"pain_points"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type SidebarProfilePatch struct {
	Source      *string `json:"source,omitempty"`
	Industry    *string `json:"industry,omitempty"`
	Description *string `json:"description,omitempty"`
	Needs       *string `json:"needs,omitempty"`
	PainPoints  *string `json:"pain_points,omitempty"`
}

type SidebarProfileUpdateCommand struct {
	CustomerID        CustomerID          `json:"customer_id"`
	OwnerStaffID      int64               `json:"owner_staff_id"`
	ExpectedUpdatedAt time.Time           `json:"expected_updated_at"`
	Patch             SidebarProfilePatch `json:"patch"`
	Actor             Actor               `json:"actor"`
	IdempotencyKey    string              `json:"idempotency_key"`
}

type SidebarProfileService interface {
	ResolveSidebarProfile(context.Context, CustomerID) (SidebarProfile, error)
	ReadSidebarProfile(context.Context, CustomerID, int64) (SidebarProfile, error)
	UpdateSidebarProfile(context.Context, SidebarProfileUpdateCommand) (SidebarProfile, error)
}

// ImageReferenceReader is the Contact-owned read-only answer to whether a
// channel welcome projection references one Media image. It returns only local
// channel IDs in ascending order.
type ImageReferenceReader interface {
	ListImageReferenceChannelIDs(context.Context, int64) ([]int64, error)
}

// AttachmentReferenceReader is the Contact-owned answer to whether a channel
// welcome projection references one private attachment.
type AttachmentReferenceReader interface {
	ListAttachmentReferenceChannelIDs(context.Context, int64) ([]int64, error)
}

// MiniProgramReferenceReader and GroupInviteReferenceReader keep Media's
// destructive lifecycle from stranding Channel-owned welcome references.
type MiniProgramReferenceReader interface {
	ListMiniProgramReferenceChannelIDs(context.Context, int64) ([]int64, error)
}

type GroupInviteReferenceReader interface {
	ListGroupInviteReferenceChannelIDs(context.Context, int64) ([]int64, error)
}

// TagReferenceReader locks one active Contact-owned tag in the caller's
// UnitOfWork. It prevents a channel mutation from committing a stale tag
// reference while that tag or its group is being archived.
type TagReferenceReader interface {
	LockActiveTag(context.Context, int64) (TagReference, error)
}

type TagReference struct {
	ID        int64
	Name      string
	GroupName *string
}

// EligibleStaffReferenceReader locks exactly one active local staff row by its
// WeCom user ID in the caller's UnitOfWork. It never exposes a full directory.
type EligibleStaffReferenceReader interface {
	LockEligibleStaffByWeComUserID(context.Context, string) (StaffDirectoryEntry, error)
}

// StaffDirectoryReader remains the broad safe projection for existing read
// consumers. Channel mutation must use EligibleStaffReferenceReader instead.
type StaffDirectoryReader interface {
	ListEligibleStaff(context.Context) ([]StaffDirectoryEntry, error)
}

// ActiveStaffReader answers only whether one local numeric staff fact remains
// active. The supplied context must carry the caller's UnitOfWork transaction.
// It intentionally exposes neither directory fields nor external identities.
type ActiveStaffReader interface {
	IsActiveStaff(context.Context, int64) (bool, error)
}

// ActiveStaffSenderReader is the one-record, transaction-bound bridge needed
// when an owning Group Ops group already names a local staff owner.
type ActiveStaffSenderReader interface {
	LockActiveWeComUserID(context.Context, int64) (string, error)
}

// ActiveStaffWeComUserIDReader is the non-mutating lookup used before a
// read-only WeCom directory request. Dispatch still uses the locked variant
// above inside its accepting UnitOfWork.
type ActiveStaffWeComUserIDReader interface {
	ReadActiveWeComUserID(context.Context, int64) (string, error)
}

type StaffDirectoryEntry struct {
	WeComUserID string
	DisplayName string
	UpdatedAt   time.Time
}

// HistoricalImportStaffReader is narrower than the ordinary directory read.
// The manifest's single-corp authorization is checked by DM01 before this
// call; staff has no corp_id and legacy roles never cross this boundary.
type HistoricalImportStaffReader interface {
	LockUniqueActiveStaffForHistoricalImport(context.Context, string) (HistoricalImportStaff, error)
}

type HistoricalImportStaff struct {
	ID int64
}

type HistoricalImportSource uint8

const (
	HistoricalImportOwnerRoleMap HistoricalImportSource = iota + 1
	HistoricalImportCustomerIdentity
	HistoricalImportExternalIdentity
)

type HistoricalImportDisposition uint8

const (
	HistoricalImportImported HistoricalImportDisposition = iota + 1
	HistoricalImportQuarantined
	HistoricalImportSkipped
)

// HistoricalImportSourceFact carries only secret-backed digests. Raw legacy
// source keys and payloads never cross into the target ledger boundary.
type HistoricalImportSourceFact struct {
	SourceKeyHMAC []byte
	PayloadHMAC   []byte
	FieldDigest   []byte
}

type HistoricalImportStaffFact struct {
	WeComUserID string
	Name        string
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type HistoricalImportCustomerFact struct {
	Name         string
	AvatarURL    *string
	Gender       *int16
	OwnerStaffID *int64
	FirstSeenAt  time.Time
	LastSeenAt   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type HistoricalImportLineage struct {
	TargetID    int64
	PayloadHMAC []byte
	FieldDigest []byte
	LastRunID   int64
}

type HistoricalImportRowReceipt struct {
	PayloadHMAC []byte
	FieldDigest []byte
	Disposition HistoricalImportDisposition
}

type HistoricalImportQuarantine struct {
	RunID      int64
	Source     HistoricalImportSource
	SourceFact HistoricalImportSourceFact
	ReasonCode string
}

type NonActiveSource uint8

const (
	NonActiveMergeAudit NonActiveSource = iota + 1
	NonActiveResolutionQueue
	NonActiveContacts
	NonActiveIdentityConflicts
	NonActivePeople
	NonActiveFollowUsers
	NonActiveDirectoryMembers
	NonActiveExternalBindings
)

type NonActiveDisposition uint8

const (
	NonActiveArchived NonActiveDisposition = iota + 1
	NonActiveSkipped
	NonActiveQuarantined
)

type NonActiveLeaseFence struct {
	RunID      int64
	Generation int64
	TokenHMAC  []byte
}

type NonActiveRowReceipt struct {
	PayloadHMAC []byte
	FieldDigest []byte
	Disposition NonActiveDisposition
}

type NonActiveArchive struct {
	RunID      int64
	Source     NonActiveSource
	SourceFact HistoricalImportSourceFact
	Nonce      []byte
	Ciphertext []byte
	KeyVersion int16
}

type NonActiveQuarantine struct {
	RunID      int64
	Source     NonActiveSource
	SourceFact HistoricalImportSourceFact
	ReasonCode string
}

// NonActiveTarget is the closed Contact-owned boundary for the eight DM01
// sources that do not materialize active roots. Receipt append is lease-fenced
// and is the final write for every newly processed row.
type NonActiveTarget interface {
	AssertNonActiveLease(context.Context, NonActiveLeaseFence) error
	LockNonActiveSource(context.Context, NonActiveSource, []byte) error
	FindNonActiveReceipt(context.Context, int64, NonActiveSource, []byte) (NonActiveRowReceipt, bool, error)
	FindNonActiveArchive(context.Context, int64, NonActiveSource, []byte) (NonActiveArchive, bool, error)
	FindNonActiveQuarantine(context.Context, int64, NonActiveSource, []byte) (NonActiveQuarantine, bool, error)
	AppendNonActiveArchive(context.Context, NonActiveArchive) error
	AppendNonActiveQuarantine(context.Context, NonActiveQuarantine) error
	AppendNonActiveReceipt(context.Context, NonActiveLeaseFence, NonActiveSource, HistoricalImportSourceFact, NonActiveDisposition) error
}

// HistoricalImportTarget is the closed Contact-owned target boundary for
// DM01. Every method requires the transaction context supplied by UnitOfWork.
// It has no event, merge, Provider, role, or arbitrary SQL capability.
type HistoricalImportTarget interface {
	LockHistoricalImportSource(context.Context, HistoricalImportSource, []byte) error
	FindHistoricalImportRowReceipt(context.Context, int64, HistoricalImportSource, []byte) (HistoricalImportRowReceipt, bool, error)
	LockHistoricalImportLineage(context.Context, HistoricalImportSource, []byte) (HistoricalImportLineage, bool, error)
	EnsureHistoricalImportStaff(context.Context, HistoricalImportStaffFact) (int64, error)
	CreateHistoricalImportCustomer(context.Context, HistoricalImportCustomerFact) (int64, error)
	ValidateHistoricalImportStaff(context.Context, int64, HistoricalImportStaffFact) error
	ValidateHistoricalImportCustomer(context.Context, int64, HistoricalImportCustomerFact) error
	LockHistoricalImportStaffTarget(context.Context, int64) (HistoricalImportStaffFact, error)
	LockHistoricalImportCustomerTarget(context.Context, int64) (HistoricalImportCustomerFact, error)
	UpdateHistoricalImportStaffCAS(context.Context, int64, HistoricalImportStaffFact, HistoricalImportStaffFact) error
	UpdateHistoricalImportCustomerCAS(context.Context, int64, HistoricalImportCustomerFact, HistoricalImportCustomerFact) error
	IsHistoricalImportActiveStaff(context.Context, int64) (bool, error)
	ValidateHistoricalImportCustomerRoot(context.Context, int64) error
	AppendHistoricalImportLineage(context.Context, int64, HistoricalImportSource, HistoricalImportSourceFact, int64) error
	UpdateHistoricalImportLineageCAS(context.Context, int64, HistoricalImportSource, HistoricalImportSourceFact, HistoricalImportLineage) error
	AppendHistoricalImportQuarantine(context.Context, HistoricalImportQuarantine) error
	AppendHistoricalImportRowReceipt(context.Context, NonActiveLeaseFence, HistoricalImportSource, HistoricalImportSourceFact, HistoricalImportDisposition) error
}

type Stage struct {
	ID        StageID
	Name      string
	SortOrder int32
	Config    json.RawMessage
}

type CreateStageCommand struct {
	Name           string
	SortOrder      int32
	Config         json.RawMessage
	Actor          Actor
	IdempotencyKey string
}

type RenameStageCommand struct {
	ID             StageID
	Name           string
	Actor          Actor
	IdempotencyKey string
}

// ReorderStagesCommand is an exact active-stage ordering. Callers must supply
// every active stage exactly once; this prevents a stale client from silently
// dropping a stage from the local lifecycle.
type ReorderStagesCommand struct {
	IDs            []StageID
	Actor          Actor
	IdempotencyKey string
}

type ArchiveStageCommand struct {
	ID             StageID
	Actor          Actor
	IdempotencyKey string
}

// StageService is the only public stage mutation boundary. Implementations
// must commit each mutation with its domain event in one UnitOfWork.
type StageService interface {
	ListStages(context.Context) ([]Stage, error)
	CreateStage(context.Context, CreateStageCommand) (Stage, error)
	RenameStage(context.Context, RenameStageCommand) (Stage, error)
	ReorderStages(context.Context, ReorderStagesCommand) ([]Stage, error)
	ArchiveStage(context.Context, ArchiveStageCommand) (Stage, error)
}

type CreateForIdentityCommand struct {
	Name         string
	OwnerStaffID *int64
	ChannelID    *int64
	Actor        Actor
}

type MergeCustomersCommand struct {
	PrimaryID CustomerID
	MergedID  CustomerID
	Actor     Actor
	Reason    string
}

type ExternalEventCommand struct {
	CustomerID     CustomerID
	EventType      string
	Payload        json.RawMessage
	Actor          Actor
	OccurredAt     time.Time
	IdempotencyKey string
}

// MergePort methods require the transaction context supplied by UnitOfWork.
type MergePort interface {
	CreateForIdentity(context.Context, CreateForIdentityCommand) (CustomerID, error)
	MergeCustomers(context.Context, MergeCustomersCommand) error
	AppendExternalEvent(context.Context, ExternalEventCommand) (EventID, error)
}
