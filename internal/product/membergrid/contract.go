// Package membergrid restores the legacy service-period product member grid as
// a local, read-only projection over Product and Local Entitlement facts.
package membergrid

import (
	"context"
	"errors"
	"time"
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
	StateActive  StateFilter = "active"
	StateRevoked StateFilter = "revoked"
	StateAll     StateFilter = "all"
)

func (state StateFilter) valid() bool {
	return state == StateActive || state == StateRevoked || state == StateAll
}

type AccessResponse struct {
	ProductID      int64 `json:"product_id"`
	CanView        bool  `json:"can_view"`
	CanQuery       bool  `json:"can_query"`
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
	ProductID int64              `json:"product_id"`
	Columns   []ColumnDefinition `json:"columns"`
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
	EntitlementID int64      `json:"entitlement_id"`
	ProductID     int64      `json:"product_id"`
	State         string     `json:"state"`
	Version       int64      `json:"version"`
	GrantedAt     time.Time  `json:"granted_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	DisplayName   string     `json:"display_name"`
	MaskedMobile  *string    `json:"masked_mobile,omitempty"`
}

type QueryInput struct {
	ProductID int64
	State     StateFilter
	Limit     int
	Cursor    string
}

type QueryResponse struct {
	Rows       []MemberRow `json:"rows"`
	Limit      int         `json:"limit"`
	NextCursor string      `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

// MemberRecord is deliberately closed: it contains no customer identifier,
// order identifier, actor, receipt, provider, payment, or external identity.
type MemberRecord struct {
	EntitlementID int64
	ProductID     int64
	State         StateFilter
	Version       int64
	GrantedAt     time.Time
	RevokedAt     *time.Time
	DisplayName   string
	MaskedMobile  *string
}

type Position struct {
	GrantedAt     time.Time
	EntitlementID int64
}

type StoreQuery struct {
	ProductID int64
	State     StateFilter
	Limit     int
	After     *Position
}

type Store interface {
	ProductExists(context.Context, int64) (bool, error)
	QueryMembers(context.Context, StoreQuery) ([]MemberRecord, error)
}

var safeColumns = []ColumnDefinition{
	{Key: "entitlement_id", Label: "权益编号", Type: "integer", Nullable: false},
	{Key: "product_id", Label: "商品编号", Type: "integer", Nullable: false},
	{Key: "state", Label: "本地权益状态", Type: "enum", Nullable: false},
	{Key: "version", Label: "本地版本", Type: "integer", Nullable: false},
	{Key: "granted_at", Label: "本地授予时间", Type: "timestamp", Nullable: false},
	{Key: "revoked_at", Label: "本地撤销时间", Type: "timestamp", Nullable: true},
	{Key: "display_name", Label: "客户显示名", Type: "string", Nullable: false},
	{Key: "masked_mobile", Label: "脱敏手机号", Type: "string", Nullable: true},
}

var builtInViews = []MemberView{
	{ID: "default", Name: "默认成员视图", Source: "built_in", ReadOnly: true},
}
