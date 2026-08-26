package legacyaudience

import (
	"context"
	"encoding/json"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const (
	OperationMembersRoute          = "/api/admin/common/operation-members"
	OperationMembersSyncRoute      = OperationMembersRoute + "/sync"
	OperationMemberScope           = "ai_audience"
	GroupOpsOperationMemberScope   = "group_ops"
	ConfigurationSchemaVersion     = "ai_audience_local_configuration.v1"
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
	Scope                string            `json:"scope"`
	Items                []OperationMember `json:"items"`
	PageSize             int               `json:"page_size"`
	ProviderReadExecuted bool              `json:"provider_read_executed"`
	Projection
}

// OperationMemberSyncInput deliberately contains no provider payload. The
// service fetches that payload through its narrow source, persists only its
// normalized Audience projection, and emits a redacted event.
type OperationMemberSyncInput struct {
	Actor          Actor
	IdempotencyKey string
	PageSize       int
}

// OperationMemberSource is the only Provider read boundary used by AI
// Audience operation-member synchronization. Implementations must return a
// complete bounded snapshot or an error; a partial page is not deletion
// authority for the local projection.
type OperationMemberSource interface {
	ReadOperationMembers(context.Context) ([]OperationMember, error)
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

// ConfigurationVersion is an append-only typed snapshot of the canonical
// Segment definition that owns this Audience package. It contains no prompt,
// free-form configuration, identity, credential or provider payload.
type ConfigurationVersion struct {
	PackageID        int64                   `json:"package_id"`
	Version          int64                   `json:"version"`
	SchemaVersion    string                  `json:"schema_version"`
	PackageVersion   int64                   `json:"package_version"`
	Definition       segmentport.Definition  `json:"definition"`
	DefinitionDigest string                  `json:"definition_digest"`
	RefreshMode      segmentport.RefreshMode `json:"refresh_mode"`
	RefreshCron      *string                 `json:"refresh_cron"`
	CreatedBy        int64                   `json:"created_by"`
	CreatedAt        time.Time               `json:"created_at"`
}

type ConfigurationResponse struct {
	Configuration *ConfigurationVersion `json:"configuration"`
	Projection
}

// SendRecordProjection intentionally excludes message content, recipient
// identifiers, sender identifiers, provider response data and credentials.
type ConfigurationEvaluationResponse struct {
	PackageID            int64     `json:"package_id"`
	ConfigurationVersion int64     `json:"configuration_version"`
	PackageVersion       int64     `json:"package_version"`
	DefinitionDigest     string    `json:"definition_digest"`
	MemberCount          int64     `json:"member_count"`
	MemberDigest         string    `json:"member_digest"`
	EvaluatedAt          time.Time `json:"evaluated_at"`
	Materialized         bool      `json:"materialized"`
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
	PackageID              int64
	ExpectedVersion        int64
	ExpectedPackageVersion int64
	Actor                  Actor
	IdempotencyKey         string
}

type PreviewConfigurationInput struct {
	PackageID            int64
	ConfigurationVersion int64
	EvaluatedAt          time.Time
}

type MaterializeConfigurationInput struct {
	PackageID              int64
	ConfigurationVersion   int64
	ExpectedPackageVersion int64
	Actor                  Actor
	IdempotencyKey         string
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
	ReserveConfigurationReceipt(context.Context, ReceiptReservation) (Receipt, bool, error)
	CompleteConfigurationReceipt(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
	GetCurrentConfiguration(context.Context, int64) (*ConfigurationVersion, error)
	GetConfigurationVersion(context.Context, int64, int64) (*ConfigurationVersion, error)
	InsertConfigurationVersion(context.Context, ConfigurationVersion) (ConfigurationVersion, error)
	ListOperationMembers(context.Context) ([]OperationMember, error)
	ReplaceOperationMembers(context.Context, []OperationMember, time.Time) ([]OperationMember, error)
}

type LocalConfigurationApplication interface {
	ListOperationMembers(context.Context, int) (OperationMemberListResponse, error)
	SyncOperationMembers(context.Context, OperationMemberSyncInput) (OperationMemberListResponse, error)
	GetAutomationBinding(context.Context, int64) (AutomationBindingResponse, error)
	PutAutomationBinding(context.Context, PutAutomationBindingInput) (AutomationBindingResponse, error)
	DeleteAutomationBinding(context.Context, DeleteAutomationBindingInput) (AutomationBindingDeleteResponse, error)
	GetSenders(context.Context, int64) (PackageSendersResponse, error)
	ReplaceSenders(context.Context, ReplaceSendersInput) (PackageSendersResponse, error)
	GetConfiguration(context.Context, int64) (ConfigurationResponse, error)
	PutConfiguration(context.Context, PutConfigurationInput) (ConfigurationResponse, error)
	PreviewConfiguration(context.Context, PreviewConfigurationInput) (ConfigurationEvaluationResponse, error)
	MaterializeConfiguration(context.Context, MaterializeConfigurationInput) (ConfigurationEvaluationResponse, error)
}

type GroupOpsOperationMemberApplication interface {
	ListGroupOpsOperationMembers(context.Context, int) (any, error)
	RefreshGroupOpsOperationMembers(context.Context, int64, string, int) (any, error)
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
		{Method: "POST", Pattern: OperationMembersSyncRoute, Capability: CapabilityOperationsManage, RequiresCSRF: true},
		{Method: "GET", Pattern: RoutePrefix + "/packages/{package_id}/automation-binding", Capability: CapabilitySegmentsRead},
		{Method: "PUT", Pattern: RoutePrefix + "/packages/{package_id}/automation-binding", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
		{Method: "DELETE", Pattern: RoutePrefix + "/packages/{package_id}/automation-binding", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: RoutePrefix + "/packages/{package_id}/senders", Capability: CapabilitySegmentsRead},
		{Method: "PUT", Pattern: RoutePrefix + "/packages/{package_id}/senders", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: RoutePrefix + "/packages/{package_id}/configuration", Capability: CapabilitySegmentsRead},
		{Method: "PUT", Pattern: RoutePrefix + "/packages/{package_id}/configuration", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: RoutePrefix + "/packages/{package_id}/configuration-preview", Capability: CapabilitySegmentsRead},
		{Method: "POST", Pattern: RoutePrefix + "/packages/{package_id}/configuration-materialize", Capability: CapabilitySegmentsWrite, RequiresCSRF: true},
	}
}
