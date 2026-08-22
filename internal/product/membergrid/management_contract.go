package membergrid

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaximumViewNameRunes        = 200
	MinimumIdempotencyKeyLength = 16
	MaximumIdempotencyKeyLength = 128
)

var (
	ErrInvalidManagementInput = errors.New("invalid member grid management input")
	ErrConflict               = errors.New("member grid management conflict")
	ErrBuiltInView            = errors.New("built-in member view is immutable")
	ErrInactiveStaff          = errors.New("collaborator staff is not active")
	ErrAuthenticationRequired = errors.New("authentication is required")
	ErrPermissionDenied       = errors.New("permission is denied")
	ErrCSRFRejected           = errors.New("csrf verification failed")
)

type ViewSort string

const ViewSortGrantedAtDesc ViewSort = "granted_at_desc"

func (sort ViewSort) valid() bool { return sort == ViewSortGrantedAtDesc }

type CollaboratorPermission string

const (
	CollaboratorPermissionView CollaboratorPermission = "view"
	CollaboratorPermissionEdit CollaboratorPermission = "edit"
)

func (permission CollaboratorPermission) valid() bool {
	return permission == CollaboratorPermissionView || permission == CollaboratorPermissionEdit
}

// SavedView stores only the closed member-grid configuration. It deliberately
// contains no arbitrary JSON, SQL expression, customer identity, provider
// identity, or externally shareable token.
type SavedView struct {
	ID               int64       `json:"view_id"`
	ServiceProductID int64       `json:"service_product_id"`
	Name             string      `json:"name"`
	State            StateFilter `json:"state"`
	Sort             ViewSort    `json:"sort"`
	Columns          []string    `json:"columns"`
	SourceViewID     *int64      `json:"source_view_id,omitempty"`
	Version          int64       `json:"version"`
	CreatedBy        int64       `json:"created_by"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// Collaborator is local staff permission metadata for this member grid. The
// edit value is not an authorization grant for Product or any central service.
type Collaborator struct {
	ID               int64                  `json:"collaborator_id"`
	ServiceProductID int64                  `json:"service_product_id"`
	StaffID          int64                  `json:"staff_id"`
	Permission       CollaboratorPermission `json:"permission"`
	Version          int64                  `json:"version"`
	InvitedBy        int64                  `json:"invited_by"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type ShareSettingsResponse struct {
	ServiceProductID                        int64          `json:"service_product_id"`
	SavedViews                              []SavedView    `json:"saved_views"`
	Collaborators                           []Collaborator `json:"collaborators"`
	ExternalShareSupported                  bool           `json:"external_share_supported"`
	ExternalShareEnabled                    bool           `json:"external_share_enabled"`
	RealExternalCallExecuted                bool           `json:"real_external_call_executed"`
	CollaboratorEditIsLocalMetadataOnly     bool           `json:"collaborator_edit_is_local_metadata_only"`
	CollaboratorEditGrantsCentralPermission bool           `json:"collaborator_edit_grants_central_permission"`
}

type SavedViewResponse struct {
	OK   bool      `json:"ok"`
	View SavedView `json:"view"`
}

type DeleteSavedViewResponse struct {
	OK      bool      `json:"ok"`
	Deleted bool      `json:"deleted"`
	View    SavedView `json:"view"`
}

type CollaboratorResponse struct {
	OK                                bool         `json:"ok"`
	Collaborator                      Collaborator `json:"collaborator"`
	EditPermissionIsLocalMetadataOnly bool         `json:"edit_permission_is_local_metadata_only"`
	GrantsCentralProductsPermission   bool         `json:"grants_central_products_permission"`
}

type DeleteCollaboratorResponse struct {
	OK                                bool         `json:"ok"`
	Deleted                           bool         `json:"deleted"`
	Collaborator                      Collaborator `json:"collaborator"`
	EditPermissionIsLocalMetadataOnly bool         `json:"edit_permission_is_local_metadata_only"`
	GrantsCentralProductsPermission   bool         `json:"grants_central_products_permission"`
}

type CreateSavedViewCommand struct {
	ServiceProductID int64
	ExpectedVersion  int64
	Name             string
	State            StateFilter
	Sort             ViewSort
	Columns          []string
	SourceViewID     *int64
	ActorID          int64
	IdempotencyKey   string
}

type UpdateSavedViewCommand struct {
	ServiceProductID int64
	ViewID           int64
	ExpectedVersion  int64
	Name             string
	State            StateFilter
	Sort             ViewSort
	Columns          []string
	ActorID          int64
	IdempotencyKey   string
}

type DeleteSavedViewCommand struct {
	ServiceProductID int64
	ViewID           int64
	ExpectedVersion  int64
	ActorID          int64
	IdempotencyKey   string
}

type CreateCollaboratorCommand struct {
	ServiceProductID int64
	ExpectedVersion  int64
	StaffID          int64
	Permission       CollaboratorPermission
	ActorID          int64
	IdempotencyKey   string
}

type UpdateCollaboratorCommand struct {
	ServiceProductID int64
	CollaboratorID   int64
	ExpectedVersion  int64
	Permission       CollaboratorPermission
	ActorID          int64
	IdempotencyKey   string
}

type DeleteCollaboratorCommand struct {
	ServiceProductID int64
	CollaboratorID   int64
	ExpectedVersion  int64
	ActorID          int64
	IdempotencyKey   string
}

type CreateSavedViewRecord struct {
	ServiceProductID int64
	Name             string
	State            StateFilter
	Sort             ViewSort
	Columns          []string
	SourceViewID     *int64
	CreatedBy        int64
	CreatedAt        time.Time
}

type UpdateSavedViewRecord struct {
	ServiceProductID int64
	ViewID           int64
	ExpectedVersion  int64
	Name             string
	State            StateFilter
	Sort             ViewSort
	Columns          []string
	UpdatedAt        time.Time
}

type CreateCollaboratorRecord struct {
	ServiceProductID int64
	StaffID          int64
	Permission       CollaboratorPermission
	InvitedBy        int64
	CreatedAt        time.Time
}

type UpdateCollaboratorRecord struct {
	ServiceProductID int64
	CollaboratorID   int64
	ExpectedVersion  int64
	Permission       CollaboratorPermission
	UpdatedAt        time.Time
}

type MutationReceiptReservation struct {
	Operation     string
	ActorScope    string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type MutationReceipt struct {
	ID             int64
	Operation      string
	ActorScope     string
	KeyDigest      [32]byte
	PayloadDigest  [32]byte
	State          string
	ResultSnapshot json.RawMessage
}

type mutationSnapshot struct {
	Kind         string        `json:"kind"`
	Status       int           `json:"status"`
	View         *SavedView    `json:"view,omitempty"`
	Collaborator *Collaborator `json:"collaborator,omitempty"`
	Deleted      bool          `json:"deleted,omitempty"`
}

type ManagementStore interface {
	ProductExists(context.Context, int64) (bool, error)
	ActiveStaffExists(context.Context, int64) (bool, error)
	ListSavedViews(context.Context, int64) ([]SavedView, error)
	GetSavedViewForUpdate(context.Context, int64, int64) (SavedView, error)
	CreateSavedView(context.Context, CreateSavedViewRecord) (SavedView, error)
	UpdateSavedView(context.Context, UpdateSavedViewRecord) (SavedView, error)
	DeleteSavedView(context.Context, int64, int64, int64) (SavedView, error)
	ListCollaborators(context.Context, int64) ([]Collaborator, error)
	GetCollaboratorForUpdate(context.Context, int64, int64) (Collaborator, error)
	CreateCollaborator(context.Context, CreateCollaboratorRecord) (Collaborator, error)
	UpdateCollaborator(context.Context, UpdateCollaboratorRecord) (Collaborator, error)
	DeleteCollaborator(context.Context, int64, int64, int64) (Collaborator, error)
	ReserveMutationReceipt(context.Context, MutationReceiptReservation) (MutationReceipt, bool, error)
	CompleteMutationReceipt(context.Context, int64, json.RawMessage, time.Time) (MutationReceipt, error)
}

type ManagementApplication interface {
	ShareSettings(context.Context, int64) (ShareSettingsResponse, error)
	CreateSavedView(context.Context, CreateSavedViewCommand) (SavedViewResponse, error)
	UpdateSavedView(context.Context, UpdateSavedViewCommand) (SavedViewResponse, error)
	DeleteSavedView(context.Context, DeleteSavedViewCommand) (DeleteSavedViewResponse, error)
	CreateCollaborator(context.Context, CreateCollaboratorCommand) (CollaboratorResponse, error)
	UpdateCollaborator(context.Context, UpdateCollaboratorCommand) (CollaboratorResponse, error)
	DeleteCollaborator(context.Context, DeleteCollaboratorCommand) (DeleteCollaboratorResponse, error)
}

func validViewName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && utf8.ValidString(name) &&
		utf8.RuneCountInString(name) <= MaximumViewNameRunes
}

func validIdempotencyKey(key string) bool {
	return len(key) >= MinimumIdempotencyKeyLength && len(key) <= MaximumIdempotencyKeyLength &&
		strings.TrimSpace(key) == key && utf8.ValidString(key)
}

func validColumnSelection(columns []string) bool {
	if len(columns) == 0 || len(columns) > len(safeColumns) {
		return false
	}
	allowed := make(map[string]struct{}, len(safeColumns))
	for _, column := range safeColumns {
		allowed[column.Key] = struct{}{}
	}
	seen := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		if column == "" || strings.TrimSpace(column) != column {
			return false
		}
		if _, ok := allowed[column]; !ok {
			return false
		}
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}

func cloneColumnsSelection(columns []string) []string {
	return append([]string(nil), columns...)
}

func cloneOptionalID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneSavedView(view SavedView) SavedView {
	view.Columns = cloneColumnsSelection(view.Columns)
	view.SourceViewID = cloneOptionalID(view.SourceViewID)
	view.CreatedAt = view.CreatedAt.UTC()
	view.UpdatedAt = view.UpdatedAt.UTC()
	return view
}

func cloneCollaborator(collaborator Collaborator) Collaborator {
	collaborator.CreatedAt = collaborator.CreatedAt.UTC()
	collaborator.UpdatedAt = collaborator.UpdatedAt.UTC()
	return collaborator
}
