package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
)

const (
	DefaultLimit      = 50
	MaximumLimit      = 100
	MaximumExportRows = 1000
)

var (
	ErrInvalidInput           = errors.New("invalid service period member input")
	ErrNotFound               = errors.New("service period member not found")
	ErrConflict               = errors.New("service period member conflict")
	ErrUnavailable            = errors.New("service period members unavailable")
	ErrPaidOrderSourceBlocked = errors.New("paid order source requires an authoritative internal owner")
	ErrExportTooLarge         = errors.New("service period member export exceeds limit")
)

type AddCommand struct {
	ServiceProductID int64
	CustomerID       int64
	Source           domain.Source
	ExpiresAt        *time.Time
	Remark           *string
	Alliance         *string
	ActorID          int64
	IdempotencyKey   string
}

type TransitionCommand struct {
	ServiceProductID int64
	MemberRef        string
	ExpectedVersion  int64
	ActorID          int64
	IdempotencyKey   string
}

type UpdateFieldsCommand struct {
	ServiceProductID int64
	MemberRef        string
	ExpectedVersion  int64
	Remark           *string
	Alliance         *string
	ActorID          int64
	IdempotencyKey   string
}

type Filter struct {
	ServiceProductID int64
	State            *domain.State
	Source           *domain.Source
}

type ListQuery struct {
	Filter
	Limit  int
	Cursor string
}

type ListResult struct {
	Items      []domain.Member `json:"items"`
	Limit      int             `json:"limit"`
	NextCursor string          `json:"next_cursor"`
	HasMore    bool            `json:"has_more"`
}

type ExportColumn string

const (
	ExportMemberRef  ExportColumn = "member_ref"
	ExportCustomerID ExportColumn = "customer_id"
	ExportState      ExportColumn = "state"
	ExportSource     ExportColumn = "source"
	ExportStartsAt   ExportColumn = "starts_at"
	ExportExpiresAt  ExportColumn = "expires_at"
	ExportExpiredAt  ExportColumn = "expired_at"
	ExportRemovedAt  ExportColumn = "removed_at"
	ExportVersion    ExportColumn = "version"
)

type ExportQuery struct {
	Filter
	Columns []ExportColumn
}

type ExportResult struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"-"`
}

type Application interface {
	Add(context.Context, AddCommand) (domain.Member, error)
	Expire(context.Context, TransitionCommand) (domain.Member, error)
	Remove(context.Context, TransitionCommand) (domain.Member, error)
	UpdateFields(context.Context, UpdateFieldsCommand) (domain.Member, error)
	Get(context.Context, int64, string) (domain.Member, error)
	List(context.Context, ListQuery) (ListResult, error)
	Export(context.Context, ExportQuery) (ExportResult, error)
}

type Position struct {
	UpdatedAt time.Time
	MemberRef string
}

type StoreListQuery struct {
	Filter
	Limit int
	After *Position
}

type CreateRecord struct {
	MemberRef        string
	ServiceProductID int64
	CustomerID       int64
	Source           domain.Source
	StartsAt         time.Time
	ExpiresAt        *time.Time
	Remark           *string
	Alliance         *string
	CreatedAt        time.Time
}

type TransitionRecord struct {
	ServiceProductID int64
	MemberRef        string
	ExpectedVersion  int64
	Target           domain.State
	TransitionedAt   time.Time
}

type UpdateFieldsRecord struct {
	ServiceProductID int64
	MemberRef        string
	ExpectedVersion  int64
	Remark           *string
	Alliance         *string
	UpdatedAt        time.Time
}

type ReceiptReservation struct {
	Operation     string
	ActorScope    string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type Receipt struct {
	ID int64
	ReceiptReservation
	State          string
	ResultSnapshot json.RawMessage
}

type Store interface {
	ServiceProductExists(context.Context, int64) (bool, error)
	CustomerExists(context.Context, int64) (bool, error)
	Get(context.Context, int64, string) (domain.Member, error)
	GetForUpdate(context.Context, int64, string) (domain.Member, error)
	Create(context.Context, CreateRecord) (domain.Member, error)
	Transition(context.Context, TransitionRecord) (domain.Member, error)
	UpdateFields(context.Context, UpdateFieldsRecord) (domain.Member, error)
	List(context.Context, StoreListQuery) ([]domain.Member, error)
	ReserveReceipt(context.Context, ReceiptReservation) (Receipt, bool, error)
	CompleteReceipt(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
}
