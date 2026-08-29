// Package membergrid provides the closed local service-period member grid.
package membergrid

import (
	"context"
	"errors"
	"time"

	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

const (
	DefaultLimit = 50
	MaximumLimit = 50
)

var (
	ErrInvalidProductID = errors.New("invalid service product id")
	ErrInvalidQuery     = errors.New("invalid member grid query")
	ErrInvalidCursor    = errors.New("invalid member grid cursor")
	ErrNotFound         = errors.New("service product not found")
	ErrUnavailable      = errors.New("member grid unavailable")
)

type StateFilter string

const (
	StateActive StateFilter = "active"
	// StateRevoked remains the persisted v1 saved-view value. Query never accepts it.
	StateRevoked StateFilter = "revoked"
	StateExpired StateFilter = "expired"
	StateRemoved StateFilter = "removed"
	StateAll     StateFilter = "all"
)

func (state StateFilter) validCanonicalGridState() bool {
	return state == StateActive || state == StateExpired || state == StateRemoved || state == StateAll
}

func (state StateFilter) validLegacySavedViewState() bool {
	return state == StateActive || state == StateRevoked || state == StateAll
}

type SourceFilter string

const (
	SourceAny       SourceFilter = ""
	SourceManual    SourceFilter = "manual"
	SourcePaidOrder SourceFilter = "paid_order"
)

func (source SourceFilter) valid() bool {
	return source == SourceAny || source == SourceManual || source == SourcePaidOrder
}

type AccessResponse struct {
	ProductID      int64 `json:"product_id"`
	CanView        bool  `json:"can_view"`
	CanQuery       bool  `json:"can_query"`
	CanEdit        bool  `json:"can_edit"`
	CanManageViews bool  `json:"can_manage_views"`
	CanShare       bool  `json:"can_share"`
}

type ColumnDefinition struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

type SchemaResponse struct {
	ServiceProductID int64              `json:"service_product_id"`
	Columns          []ColumnDefinition `json:"columns"`
}

type MemberView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	ReadOnly bool   `json:"read_only"`
}

type MemberViewsResponse struct {
	ProductID int64        `json:"product_id"`
	Views     []MemberView `json:"views"`
}

type MemberRow struct {
	MemberRef        string     `json:"member_ref"`
	ServiceProductID int64      `json:"service_product_id"`
	CustomerID       int64      `json:"customer_id"`
	State            string     `json:"state"`
	Source           string     `json:"source"`
	StartsAt         time.Time  `json:"starts_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
	ExpiredAt        *time.Time `json:"expired_at"`
	RemovedAt        *time.Time `json:"removed_at"`
	Version          int64      `json:"version"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DisplayName      string     `json:"display_name"`
}

type QueryInput struct {
	ProductID int64
	State     StateFilter
	Source    SourceFilter
	Limit     int
	Cursor    string
	Sort      string
	GroupBy   string
	ViewID    string
}

type UpdateFieldsCommand struct {
	ProductID       int64
	MemberRef       string
	ExpectedVersion int64
	Remark          *string
	Alliance        *string
	IdempotencyKey  string
}

type MemberFieldEditor interface {
	UpdateFields(context.Context, memberport.UpdateFieldsCommand) (memberdomain.Member, error)
}

type QueryResponse struct {
	Rows       []MemberRow `json:"rows"`
	Limit      int         `json:"limit"`
	NextCursor string      `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

// MemberRecord is the grid projection over canonical service-period facts.
type MemberRecord struct {
	MemberRef        string
	ServiceProductID int64
	CustomerID       int64
	State            StateFilter
	Source           SourceFilter
	StartsAt         time.Time
	ExpiresAt        *time.Time
	ExpiredAt        *time.Time
	RemovedAt        *time.Time
	Version          int64
	UpdatedAt        time.Time
	DisplayName      string
}

type Position struct {
	UpdatedAt time.Time
	MemberRef string
}

type StoreQuery struct {
	ProductID int64
	State     StateFilter
	Source    SourceFilter
	Limit     int
	After     *Position
}

type Store interface {
	ProductExists(context.Context, int64) (bool, error)
	QueryMembers(context.Context, StoreQuery) ([]MemberRecord, error)
}

var safeColumns = []ColumnDefinition{
	{Key: "member_ref", Label: "成员引用", Type: "string", Nullable: false},
	{Key: "service_product_id", Label: "服务期商品编号", Type: "integer", Nullable: false},
	{Key: "customer_id", Label: "客户编号", Type: "integer", Nullable: false},
	{Key: "state", Label: "本地成员状态", Type: "enum", Nullable: false},
	{Key: "source", Label: "本地成员来源", Type: "enum", Nullable: false},
	{Key: "starts_at", Label: "开始时间", Type: "timestamp", Nullable: false},
	{Key: "expires_at", Label: "到期时间", Type: "timestamp", Nullable: true},
	{Key: "expired_at", Label: "过期时间", Type: "timestamp", Nullable: true},
	{Key: "removed_at", Label: "移除时间", Type: "timestamp", Nullable: true},
	{Key: "version", Label: "本地版本", Type: "integer", Nullable: false},
	{Key: "updated_at", Label: "更新时间", Type: "timestamp", Nullable: false},
	{Key: "display_name", Label: "客户显示名", Type: "string", Nullable: false},
}

// legacySavedViewColumns freezes the v1 saved-view field contract. The
// canonical read schema above intentionally does not change saved views.
var legacySavedViewColumns = []ColumnDefinition{
	{Key: "entitlement_id"}, {Key: "product_id"}, {Key: "state"}, {Key: "version"},
	{Key: "granted_at"}, {Key: "revoked_at"}, {Key: "display_name"}, {Key: "masked_mobile"},
}

var builtInViews = []MemberView{
	{ID: "default", Name: "默认成员视图", Source: "built_in", ReadOnly: true},
}
