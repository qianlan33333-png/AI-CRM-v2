package legacyaudience

import (
	"context"
	"encoding/json"
	"time"
)

const (
	OperationMembersRoute          = "/api/admin/common/operation-members"
	OperationMemberScope           = "ai_audience"
	MaximumSenderCount             = 5
	MaximumOperationMemberPageSize = 100
)

type AutomationAgent struct {
	ID     int64
	Status string
}

// AutomationAgentReader is the small Automation-owned read contract used to
// validate a local binding. It never starts an agent or invokes a runtime.
type AutomationAgentReader interface {
	GetAutomationAgent(context.Context, int64) (AutomationAgent, error)
}

type AutomationBinding struct {
	PackageID         int64     `json:"package_id"`
	AutomationAgentID int64     `json:"automation_agent_id"`
	Version           int64     `json:"version"`
	CreatedBy         int64     `json:"created_by"`
	UpdatedBy         int64     `json:"updated_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type PackageSender struct {
	SenderUserID string `json:"sender_userid"`
	SortOrder    int32  `json:"sort_order"`
	IsEnabled    bool   `json:"is_enabled"`
}

type OperationMember struct {
	SenderUserID string `json:"sender_userid"`
	DisplayName  string `json:"display_name"`
}

type OperationMemberListResponse struct {
	Scope    string            `json:"scope"`
	Items    []OperationMember `json:"items"`
	PageSize int               `json:"page_size"`
	Projection
}

type AutomationBindingResponse struct {
	Binding *AutomationBinding `json:"binding"`
	Projection
}

type AutomationBindingDeleteResponse struct {
	PackageID int64 `json:"package_id"`
	Deleted   bool  `json:"deleted"`
	Projection
}

type PackageSendersResponse struct {
	PackageID int64           `json:"package_id"`
	Items     []PackageSender `json:"items"`
	Projection
}

// ConfigurationVersion is an append-only local snapshot. It has no runtime
// flag, provider identity, credential, or model prompt execution semantics.
type ConfigurationVersion struct {
	PackageID      int64           `json:"package_id"`
	Version        int64           `json:"version"`
	TemplateConfig json.RawMessage `json:"template_config"`
	FilterConfig   json.RawMessage `json:"filter_config"`
	CreatedBy      int64           `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
}

type ConfigurationResponse struct {
	Configuration *ConfigurationVersion `json:"configuration"`
	Projection
}

// SendRecordProjection intentionally excludes message content, recipient
// identifiers, sender identifiers, provider response data and credentials.
type SendRecordProjection struct {
	RecordID    string    `json:"record_id"`
	State       string    `json:"state"`
	OccurredAt  time.Time `json:"occurred_at"`
	ProjectedAt time.Time `json:"projected_at"`
}

type SendRecordListResponse struct {
	PackageID int64                  `json:"package_id"`
	Items     []SendRecordProjection `json:"items"`
	Projection
}

type PutAutomationBindingInput struct {
	PackageID         int64
	AutomationAgentID int64
	ExpectedVersion   int64
	Actor             Actor
	IdempotencyKey    string
}

type DeleteAutomationBindingInput struct {
	PackageID       int64
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}

type ReplaceSendersInput struct {
	PackageID      int64
	Items          []PackageSender
	Actor          Actor
	IdempotencyKey string
}

type PutConfigurationInput struct {
	PackageID       int64
	ExpectedVersion int64
	TemplateConfig  json.RawMessage
	FilterConfig    json.RawMessage
	Actor           Actor
	IdempotencyKey  string
}

// LocalConfigurationRepository owns only AI Audience-local references and
// receipts. It cannot write Automation or Contact/Staff state.
type LocalConfigurationRepository interface {
	GetPackageMetadata(context.Context, int64) (PackageMetadata, error)
	LockPackage(context.Context, int64) (PackageWriteModel, error)
	GetAutomationBinding(context.Context, int64) (*AutomationBinding, error)
	SaveAutomationBinding(context.Context, AutomationBinding, int64, int64, time.Time) (AutomationBinding, error)
	DeleteAutomationBinding(context.Context, int64, int64) (bool, error)
	ListPackageSenders(context.Context, int64) ([]PackageSender, error)
	ReplacePackageSenders(context.Context, int64, []PackageSender, int64, time.Time) ([]PackageSender, bool, error)
	ListEligibleSenderUserIDs(context.Context, []string) ([]string, error)
	LockEligibleSenderUserIDs(context.Context, []string) ([]string, error)
	ReserveConfigurationReceipt(context.Context, ReceiptReservation) (Receipt, bool, error)
	CompleteConfigurationReceipt(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
	GetCurrentConfiguration(context.Context, int64) (*ConfigurationVersion, error)
	InsertConfigurationVersion(context.Context, ConfigurationVersion) (ConfigurationVersion, error)
	ListSendRecordProjections(context.Context, int64, int) ([]SendRecordProjection, error)
}

type LocalConfigurationApplication interface {
	ListOperationMembers(context.Context, int) (OperationMemberListResponse, error)
	GetAutomationBinding(context.Context, int64) (AutomationBindingResponse, error)
	PutAutomationBinding(context.Context, PutAutomationBindingInput) (AutomationBindingResponse, error)
	DeleteAutomationBinding(context.Context, DeleteAutomationBindingInput) (AutomationBindingDeleteResponse, error)
	GetSenders(context.Context, int64) (PackageSendersResponse, error)
	ReplaceSenders(context.Context, ReplaceSendersInput) (PackageSendersResponse, error)
	GetConfiguration(context.Context, int64) (ConfigurationResponse, error)
	PutConfiguration(context.Context, PutConfigurationInput) (ConfigurationResponse, error)
	ListSendRecords(context.Context, int64) (SendRecordListResponse, error)
}

type LocalConfigurationRouteSpec struct {
	Method       string
	Pattern      string
	Capability   string
	RequiresCSRF bool
}

func LocalConfigurationRouteSpecs() []LocalConfigurationRouteSpec {
	return []LocalConfigurationRouteSpec{
		{Method: "GET", Pattern: OperationMembersRoute, Capability: CapabilitySegmentsRead},
		{Method: "GET", Pattern: RoutePrefix + "/packages/{package_id}/automation-binding", Capability: CapabilitySegmentsRead},
		{Method: "PUT", Pattern: RoutePrefix + "/packages/{package_id}/automation-binding", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
		{Method: "DELETE", Pattern: RoutePrefix + "/packages/{package_id}/automation-binding", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: RoutePrefix + "/packages/{package_id}/senders", Capability: CapabilitySegmentsRead},
		{Method: "PUT", Pattern: RoutePrefix + "/packages/{package_id}/senders", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: RoutePrefix + "/packages/{package_id}/template-config", Capability: CapabilitySegmentsRead},
		{Method: "PUT", Pattern: RoutePrefix + "/packages/{package_id}/template-config", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: RoutePrefix + "/packages/{package_id}/send-records", Capability: CapabilitySegmentsRead},
	}
}
