package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrServicePeriodHistoryInvalid     = errors.New("invalid service-period history")
	ErrServicePeriodHistoryConflict    = errors.New("service-period history conflict")
	ErrServicePeriodHistoryUnavailable = errors.New("service-period history unavailable")
)

// These records preserve source facts, never current membership or authority.
// ProductID references the existing verified Product, not a second catalog.
type ServicePeriodHistoryDefinition struct {
	ID                   int64     `json:"id"`
	SourceDefinitionID   int64     `json:"source_definition_id"`
	ProductID            int64     `json:"product_id"`
	MembershipConfigID   string    `json:"membership_config_id"`
	MembershipConfigName string    `json:"membership_config_name"`
	DurationDays         int32     `json:"duration_days"`
	Deleted              bool      `json:"deleted"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ServicePeriodHistoryEntitlement struct {
	ID                  int64     `json:"id"`
	SourceEntitlementID int64     `json:"source_entitlement_id"`
	DefinitionID        int64     `json:"definition_id"`
	CustomerID          *int64    `json:"customer_id"`
	MembershipConfigID  string    `json:"membership_config_id"`
	Status              string    `json:"status"`
	StartAt             time.Time `json:"start_at"`
	EndAt               time.Time `json:"end_at"`
	LastOrderID         *int64    `json:"last_order_id"`
	LastOutTradeNo      string    `json:"last_out_trade_no"`
	RenewalCount        int32     `json:"renewal_count"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ServicePeriodHistoryEvent struct {
	ID            int64      `json:"id"`
	SourceEventID int64      `json:"source_event_id"`
	DefinitionID  int64      `json:"definition_id"`
	EntitlementID *int64     `json:"entitlement_id"`
	CustomerID    *int64     `json:"customer_id"`
	OrderID       *int64     `json:"order_id"`
	EventID       string     `json:"event_id"`
	EventType     string     `json:"event_type"`
	DurationDays  int32      `json:"duration_days"`
	OutTradeNo    string     `json:"out_trade_no"`
	BeforeStartAt *time.Time `json:"before_start_at"`
	BeforeEndAt   *time.Time `json:"before_end_at"`
	AfterStartAt  *time.Time `json:"after_start_at"`
	AfterEndAt    *time.Time `json:"after_end_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type ServicePeriodHistoryReceipt struct {
	SourceIdentifier string
	PayloadDigest    [32]byte
	TargetID         int64
	TargetDigest     [32]byte
	Replayed         bool
}

// Both adapters must use the caller's transaction; no runtime event is emitted.
type ServicePeriodHistoryStore interface {
	CreateServicePeriodHistoryDefinition(context.Context, ServicePeriodHistoryDefinition) (ServicePeriodHistoryDefinition, error)
	GetServicePeriodHistoryDefinition(context.Context, int64) (ServicePeriodHistoryDefinition, error)
	CreateServicePeriodHistoryEntitlement(context.Context, ServicePeriodHistoryEntitlement) (ServicePeriodHistoryEntitlement, error)
	GetServicePeriodHistoryEntitlement(context.Context, int64) (ServicePeriodHistoryEntitlement, error)
	CreateServicePeriodHistoryEvent(context.Context, ServicePeriodHistoryEvent) (ServicePeriodHistoryEvent, error)
	GetServicePeriodHistoryEvent(context.Context, int64) (ServicePeriodHistoryEvent, error)
}

// Kind is exactly definitions, entitlements or events, each with one scoped journal.
type ServicePeriodHistoryJournal interface {
	LoadServicePeriodHistory(context.Context, string, string) (ServicePeriodHistoryReceipt, bool, error)
	RecordServicePeriodHistory(context.Context, string, ServicePeriodHistoryReceipt) error
}

type ServicePeriodHistoryProduct struct {
	ServicePeriodHistoryDefinition
	ProductCode string `json:"product_code"`
	ProductName string `json:"product_name"`
	PriceMinor  int64  `json:"price_minor"`
	Currency    string `json:"currency"`
}

type ServicePeriodHistoryReader interface {
	ListServicePeriodHistoryDefinitions(context.Context, int32, int32) ([]ServicePeriodHistoryProduct, int64, error)
	ListServicePeriodHistoryEntitlements(context.Context, int64, int32, int32) ([]ServicePeriodHistoryEntitlement, int64, error)
	ListServicePeriodHistoryEvents(context.Context, int64, int32, int32) ([]ServicePeriodHistoryEvent, int64, error)
}
