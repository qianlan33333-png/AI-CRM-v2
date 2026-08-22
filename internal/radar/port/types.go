package port

import "time"

type LinkID int64

type Status string

const (
	StatusDraft    Status = "draft"
	StatusEnabled  Status = "enabled"
	StatusDisabled Status = "disabled"
)

func (status Status) Valid() bool {
	switch status {
	case StatusDraft, StatusEnabled, StatusDisabled:
		return true
	default:
		return false
	}
}

type StatusFilter string

const (
	StatusFilterAll      StatusFilter = "all"
	StatusFilterDraft    StatusFilter = "draft"
	StatusFilterEnabled  StatusFilter = "enabled"
	StatusFilterDisabled StatusFilter = "disabled"
)

func (filter StatusFilter) Valid() bool {
	switch filter {
	case StatusFilterAll, StatusFilterDraft, StatusFilterEnabled, StatusFilterDisabled:
		return true
	default:
		return false
	}
}

type Sort string

const (
	SortUpdatedDesc Sort = "updated_desc"
	SortCreatedDesc Sort = "created_desc"
	SortNameAsc     Sort = "name_asc"
)

func (sort Sort) Valid() bool {
	switch sort {
	case SortUpdatedDesc, SortCreatedDesc, SortNameAsc:
		return true
	default:
		return false
	}
}

const (
	DefaultLimit               int32 = 20
	MaximumLimit               int32 = 100
	MaximumOffset              int32 = 1_000_000
	MaximumNameRunes                 = 120
	MaximumTitleRunes                = 200
	MaximumURLBytes                  = 2048
	MaximumRequestBodyBytes    int64 = 64 << 10
	MinimumIdempotencyKeyBytes       = 16
	MaximumIdempotencyKeyBytes       = 128
)

type Link struct {
	LinkID         LinkID    `json:"link_id"`
	PublicCode     string    `json:"public_code"`
	Name           string    `json:"name"`
	Title          string    `json:"title"`
	DestinationURL string    `json:"destination_url"`
	CoverImageID   *int64    `json:"cover_image_id"`
	AttachmentID   *int64    `json:"attachment_id"`
	Status         Status    `json:"status"`
	Version        int64     `json:"version"`
	CreatedBy      int64     `json:"created_by"`
	UpdatedBy      int64     `json:"updated_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ListInput struct {
	Status StatusFilter
	Sort   Sort
	Limit  int32
	Offset int32
}

type Page struct {
	Items                    []Link       `json:"items"`
	Total                    int64        `json:"total"`
	Limit                    int32        `json:"limit"`
	Offset                   int32        `json:"offset"`
	HasMore                  bool         `json:"has_more"`
	StatusFilter             StatusFilter `json:"status_filter"`
	Sort                     Sort         `json:"sort"`
	LocalProjection          bool         `json:"local_projection"`
	RealExternalCallExecuted bool         `json:"real_external_call_executed"`
}

type LinkResponse struct {
	Link                     Link `json:"link"`
	LocalProjection          bool `json:"local_projection"`
	RealExternalCallExecuted bool `json:"real_external_call_executed"`
}

type ShareProjection struct {
	LinkID                   LinkID `json:"link_id"`
	PublicCode               string `json:"public_code"`
	Status                   Status `json:"status"`
	SharePath                string `json:"share_path"`
	QRPayload                string `json:"qr_payload"`
	LocalProjection          bool   `json:"local_projection"`
	PublicRouteReady         bool   `json:"public_route_ready"`
	RealExternalCallExecuted bool   `json:"real_external_call_executed"`
}

type OptionDefaults struct {
	InitialStatus Status       `json:"initial_status"`
	StatusFilter  StatusFilter `json:"status_filter"`
	Sort          Sort         `json:"sort"`
	Limit         int32        `json:"limit"`
}

type OptionLimits struct {
	NameRunes                  int   `json:"name_runes"`
	TitleRunes                 int   `json:"title_runes"`
	DestinationURLBytes        int   `json:"destination_url_bytes"`
	ListLimitMinimum           int32 `json:"list_limit_minimum"`
	ListLimitMaximum           int32 `json:"list_limit_maximum"`
	ListOffsetMaximum          int32 `json:"list_offset_maximum"`
	RequestBodyBytes           int64 `json:"request_body_bytes"`
	IdempotencyKeyBytesMinimum int   `json:"idempotency_key_bytes_minimum"`
	IdempotencyKeyBytesMaximum int   `json:"idempotency_key_bytes_maximum"`
}

type Options struct {
	Statuses                 []Status       `json:"statuses"`
	StatusFilters            []StatusFilter `json:"status_filters"`
	Sorts                    []Sort         `json:"sorts"`
	Defaults                 OptionDefaults `json:"defaults"`
	Limits                   OptionLimits   `json:"limits"`
	DestinationSchemes       []string       `json:"destination_schemes"`
	LocalProjection          bool           `json:"local_projection"`
	PublicRouteReady         bool           `json:"public_route_ready"`
	RealExternalCallExecuted bool           `json:"real_external_call_executed"`
}

type OptionalString struct {
	Set   bool
	Value string
}

type OptionalNullableID struct {
	Set   bool
	Value *int64
}

type CreateCommand struct {
	ExpectedVersion int64
	Name            string
	Title           string
	DestinationURL  string
	CoverImageID    *int64
	AttachmentID    *int64
	ActorID         int64
	IdempotencyKey  string
}

type UpdateCommand struct {
	LinkID          LinkID
	ExpectedVersion int64
	Name            OptionalString
	Title           OptionalString
	DestinationURL  OptionalString
	CoverImageID    OptionalNullableID
	AttachmentID    OptionalNullableID
	ActorID         int64
	IdempotencyKey  string
}

type SetStatusCommand struct {
	LinkID          LinkID
	ExpectedVersion int64
	Target          Status
	ActorID         int64
	IdempotencyKey  string
}

type CreateRecord struct {
	PublicCode     string
	Name           string
	Title          string
	DestinationURL string
	CoverImageID   *int64
	AttachmentID   *int64
	Status         Status
	ActorID        int64
}

type UpdateRecord struct {
	LinkID          LinkID
	ExpectedVersion int64
	Name            string
	Title           string
	DestinationURL  string
	CoverImageID    *int64
	AttachmentID    *int64
	ActorID         int64
}

type StatusRecord struct {
	LinkID          LinkID
	ExpectedVersion int64
	Target          Status
	ActorID         int64
}

type IdempotencyState string

const (
	IdempotencyReserved  IdempotencyState = "reserved"
	IdempotencyCompleted IdempotencyState = "completed"
)

type IdempotencyRecord struct {
	RecordID      int64
	ActorID       int64
	KeyDigest     [32]byte
	Operation     string
	PayloadDigest [32]byte
	State         IdempotencyState
	Result        *Link
	CreatedAt     time.Time
	CompletedAt   *time.Time
}

type ReserveIdempotencyRecord struct {
	ActorID       int64
	KeyDigest     [32]byte
	Operation     string
	PayloadDigest [32]byte
	CreatedAt     time.Time
}
