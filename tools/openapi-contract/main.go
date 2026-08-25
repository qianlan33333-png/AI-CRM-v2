package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type canonicalCandidateOperation struct {
	Path       string
	Method     string
	MappingIDs []string
}

type mappingInventory struct {
	Known      map[string]bool
	Candidates map[string]canonicalCandidateOperation
}

// nativePackageOperation is the explicit owner-approved registry for P4
// business-package operations which have no legacy route to claim. Keeping
// these declarations here avoids manufacturing ledger mappings while still
// making every new route fail closed on its exact security and data boundary.
type nativePackageOperation struct {
	path           string
	method         string
	evidence       string
	capability     string
	authScheme     string
	classification string
	dataSource     string
	csrf           string
	scopes         map[string]string
}

type nativePackagePathParameter struct {
	name      string
	typeName  string
	pattern   string
	maxLength uint64
}

type nativePackageLaunchQueryContract struct {
	kinds           []any
	idPattern       string
	idMaximumLength uint64
	idMaximumValue  string
	location        string
	locationPattern string
}

const (
	p4ClassificationPackageEvidence            = "P4-CLASSIFICATION-SEGMENT-PACKAGE-2026-08-20"
	p4ProductEntitlementEvidence               = "P4-PRODUCT-ENTITLEMENT-PACKAGE-2026-08-20"
	p4SurveyPublicEvidence                     = "P4-SURVEY-PUBLIC-ANONYMOUS-2026-08-20"
	p4SurveySafeAdminEvidence                  = "P4-SURVEY-SAFE-ADMIN-2026-08-21"
	p4CloudOrchestratorEvidence                = "P4-CLOUD-ORCHESTRATOR-CARRIERS-2026-08-20"
	p4GroupOpsWorkspaceEvidence                = "P4-GROUP-OPS-WORKSPACE-CARRIERS-2026-08-20"
	p4AudienceWorkspaceEvidence                = "P4-AI-AUDIENCE-WORKSPACE-CARRIERS-2026-08-20"
	p4OutboundOperationsEvidence               = "P4-OUTBOUND-OPERATIONS-2026-08-20"
	p4CommerceWorkspaceEvidence                = "P4-COMMERCE-WORKSPACE-CARRIERS-2026-08-20"
	p4HXCSenderManagementEvidence              = "P4-HXC-SENDER-MANAGEMENT-2026-08-20"
	p4ExternalEffectsReadonlyEvidence          = "P4-EXTERNAL-EFFECTS-READONLY-2026-08-21"
	p4AIAudienceConfigurationEvidence          = "P4-AI-AUDIENCE-LOCAL-CONFIGURATION-2026-08-22"
	p4GroupOpsLocalEvidence                    = "P4-GROUP-OPS-LOCAL-ONLY-2026-08-23"
	p4ServicePeriodMembersEvidence             = "P4-SERVICE-PERIOD-MEMBERS-LOCAL-2026-08-23"
	p4OrderSafeExportEvidence                  = "P4-ORDER-SAFE-EXPORT-2026-08-23"
	p4ContactPolicyEvidence                    = "P4-CONTACT-POLICY-2026-08-23"
	p4CampaignInitiationEvidence               = "P4-CAMPAIGN-INITIATION-SNAPSHOTS-00066-2026-08-23"
	p4CampaignReviewHandoffEvidence            = "P4-CAMPAIGN-REVIEW-HANDOFF-00067-2026-08-23"
	p4OutboundCampaignHandoffEvidence          = "P4-OUTBOUND-CAMPAIGN-HANDOFF-2026-08-23"
	p4SidebarLocalCoreEvidence                 = "P4-S05-SIDEBAR-LOCAL-CORE-2026-08-24"
	p4ContactOwnerReassignmentEvidence         = "P4-CONTACT-OWNER-REASSIGNMENT-LOCAL-CORE-2026-08-24"
	p4CustomerSafeExportEvidence               = "P4-CUSTOMER-SAFE-EXPORT-LOCAL-CORE-2026-08-24"
	p4InternalEventSafeExportEvidence          = "P4-EE01-INTERNAL-EVENT-SAFE-EXPORT-2026-08-25"
	p4ReleasePlaneEvidence                     = "P4-RP01-RELEASE-PLANE-2026-08-25"
	p4ExternalEffectsRuntimeEvidence           = "P4-EXTERNAL-EFFECTS-RUNTIME-2026-08-25"
	p4OutboundCampaignDispatchEvidence         = "P4-C01-OUTBOUND-DISPATCH-2026-08-25"
	p4PE01WeChatPaySettlementEvidence          = "PE01-WECHAT-PAY-SETTLEMENT-2026-08-25"
	p4AutomationRulesRuntimeEvidence           = "P4-A01-AUTOMATION-RULES-RUNTIME-2026-08-25"
	p4ServicePeriodMemberGridCanonicalEvidence = "P4-SERVICE-PERIOD-MEMBER-GRID-CANONICAL-LOCAL-CORE-2026-08-24"
	c01DispatchOperationID                     = "dispatchOutboundCampaignHandoff"
	c01DispatchReadOperationID                 = "getOutboundCampaignDispatchReconciliation"
	c01DispatchReconcileOperationID            = "reconcileOutboundCampaignDispatch"
)

var pe01Operations = map[string]nativePackageOperation{
	"createWechatPayCheckout":         {"/api/v1/wechat-pay/checkouts", "POST", p4PE01WeChatPaySettlementEvidence, "order.write", "human_session", "financial", "order.pe01_checkout_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getWechatPayCheckout":            {"/api/v1/wechat-pay/checkouts/{merchant_order_no}", "GET", p4PE01WeChatPaySettlementEvidence, "order.read", "human_session", "financial", "order.pe01_actor_bound_projection", "none", map[string]string{"admin": "global", "ops": "global"}},
	"createWechatPaySettlementRefund": {"/api/v1/wechat-pay/orders/{order_id}/refunds", "POST", p4PE01WeChatPaySettlementEvidence, "order.write", "human_session", "financial", "order.pe01_refund_transaction", "required", map[string]string{"admin": "global"}},
	"receiveWechatPayPaymentCallback": {"/api/public/wechat-pay/callbacks/payment", "POST", p4PE01WeChatPaySettlementEvidence, "", "wechat_pay_signature", "financial", "order.pe01_verified_callback", "none", nil},
	"receiveWechatPayRefundCallback":  {"/api/public/wechat-pay/callbacks/refund", "POST", p4PE01WeChatPaySettlementEvidence, "", "wechat_pay_signature", "financial", "order.pe01_verified_callback", "none", nil},
}

var nativePackageOperations = map[string]nativePackageOperation{
	"createCustomerSafeExport":                 {"/api/v1/customer-exports", "POST", p4CustomerSafeExportEvidence, "customers.read", "human_session", "internal_pii", "contact.local_frozen_snapshot", "required", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getCustomerSafeExport":                    {"/api/v1/customer-exports/{export_id}", "GET", p4CustomerSafeExportEvidence, "customers.read", "human_session", "internal_pii", "contact.local_frozen_snapshot", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"downloadCustomerSafeExport":               {"/api/v1/customer-exports/{export_id}/download", "GET", p4CustomerSafeExportEvidence, "customers.read", "human_session", "internal_pii", "contact.local_frozen_snapshot", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"createInternalEventSafeExport":            {"/api/admin/internal-events/exports", "POST", p4InternalEventSafeExportEvidence, "admin.read", "human_session", "internal", "event_log_and_event_deliveries.local_frozen_snapshot", "required", map[string]string{"admin": "global"}},
	"getInternalEventSafeExport":               {"/api/admin/internal-events/exports/{export_id}", "GET", p4InternalEventSafeExportEvidence, "admin.read", "human_session", "internal", "event_log_and_event_deliveries.local_frozen_snapshot", "none", map[string]string{"admin": "global"}},
	"downloadInternalEventSafeExport":          {"/api/admin/internal-events/exports/{export_id}/download", "GET", p4InternalEventSafeExportEvidence, "admin.read", "human_session", "internal", "event_log_and_event_deliveries.local_frozen_snapshot", "none", map[string]string{"admin": "global"}},
	"listReleaseCandidates":                    {"/api/v1/admin/release-candidates", "GET", p4ReleasePlaneEvidence, "release.read", "human_session", "internal", "local_release_attestation", "none", map[string]string{"admin": "global", "ops": "global"}},
	"registerReleaseCandidate":                 {"/api/v1/admin/release-candidates", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"getReleaseCandidate":                      {"/api/v1/admin/release-candidates/{candidate_id}", "GET", p4ReleasePlaneEvidence, "release.read", "human_session", "internal", "local_release_attestation", "none", map[string]string{"admin": "global", "ops": "global"}},
	"recordReleasePrerequisite":                {"/api/v1/admin/release-candidates/{candidate_id}/prerequisites", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"prepareReleaseCandidate":                  {"/api/v1/admin/release-candidates/{candidate_id}/prepare", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"startReleaseCutover":                      {"/api/v1/admin/release-candidates/{candidate_id}/cutover/start", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"restartReleaseCutover":                    {"/api/v1/admin/release-candidates/{candidate_id}/cutover/restart", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"completeReleaseCutoverStep":               {"/api/v1/admin/release-candidates/{candidate_id}/cutover/steps/{step}/complete", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"activateReleaseCandidate":                 {"/api/v1/admin/release-candidates/{candidate_id}/activate", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"recordReleaseRollbackCheck":               {"/api/v1/admin/release-candidates/{candidate_id}/rollback-checks", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"requestReleaseRollback":                   {"/api/v1/admin/release-candidates/{candidate_id}/rollback/request", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"completeReleaseRollback":                  {"/api/v1/admin/release-candidates/{candidate_id}/rollback/complete", "POST", p4ReleasePlaneEvidence, "release.manage", "human_session", "internal", "local_release_attestation", "required", map[string]string{"admin": "global"}},
	"listExternalEffectsRuntime":               {"/api/admin/external-effects", "GET", p4ExternalEffectsRuntimeEvidence, "operations.read", "human_session", "internal", "external_effects.local_safe_projection", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getExternalEffectRuntime":                 {"/api/admin/external-effects/{effect_id}", "GET", p4ExternalEffectsRuntimeEvidence, "operations.read", "human_session", "internal", "external_effects.local_safe_projection", "none", map[string]string{"admin": "global", "ops": "global"}},
	"cancelExternalEffectRuntime":              {"/api/admin/external-effects/{effect_id}/cancel", "POST", p4ExternalEffectsRuntimeEvidence, "operations.manage", "human_session", "internal", "external_effects.local_control_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"retryExternalEffectRuntime":               {"/api/admin/external-effects/{effect_id}/retry", "POST", p4ExternalEffectsRuntimeEvidence, "operations.manage", "human_session", "internal", "external_effects.local_control_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"reconcileExternalEffectRuntime":           {"/api/admin/external-effects/{effect_id}/reconcile", "POST", p4ExternalEffectsRuntimeEvidence, "operations.manage", "human_session", "internal", "external_effects.local_reconciliation_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"listAutomationRulesRuntime":               {"/api/admin/automations", "GET", p4AutomationRulesRuntimeEvidence, "config.overview.read", "human_session", "internal", "automation.local_rule_configuration", "none", map[string]string{"admin": "global", "ops": "global"}},
	"createAutomationRuleRuntime":              {"/api/admin/automations", "POST", p4AutomationRulesRuntimeEvidence, "config.settings.manage", "human_session", "internal", "automation.local_rule_configuration", "required", map[string]string{"admin": "global"}},
	"getAutomationRuleRuntime":                 {"/api/admin/automations/{rule_id}", "GET", p4AutomationRulesRuntimeEvidence, "config.overview.read", "human_session", "internal", "automation.local_rule_configuration", "none", map[string]string{"admin": "global", "ops": "global"}},
	"updateAutomationRuleRuntime":              {"/api/admin/automations/{rule_id}", "PATCH", p4AutomationRulesRuntimeEvidence, "config.settings.manage", "human_session", "internal", "automation.local_rule_configuration", "required", map[string]string{"admin": "global"}},
	"setAutomationRuleRuntimeStatus":           {"/api/admin/automations/{rule_id}/{status}", "POST", p4AutomationRulesRuntimeEvidence, "config.settings.manage", "human_session", "internal", "automation.local_rule_configuration", "required", map[string]string{"admin": "global"}},
	"listAutomationRuleRuntimeExecutions":      {"/api/admin/automations/executions", "GET", p4AutomationRulesRuntimeEvidence, "config.overview.read", "human_session", "internal", "automation.local_execution_ledger", "none", map[string]string{"admin": "global", "ops": "global"}},
	"reconcileAutomationRuleRuntimeExecution":  {"/api/admin/automations/executions/{action_id}/reconcile", "POST", p4AutomationRulesRuntimeEvidence, "config.settings.manage", "human_session", "internal", "automation.local_eer_reconciliation_transaction", "required", map[string]string{"admin": "global"}},
	"getServicePeriodMemberGridSchema":         {"/api/admin/service-period-products/{service_product_id}/member-grid/schema", "GET", p4ServicePeriodMemberGridCanonicalEvidence, "products.read", "human_session", "internal_pii", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"queryServicePeriodMemberGrid":             {"/api/admin/service-period-products/{service_product_id}/member-grid/query", "POST", p4ServicePeriodMemberGridCanonicalEvidence, "entitlements.read", "human_session", "internal_pii", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"downloadContactOwnerReassignmentTemplate": {"/api/v1/contact-owner-reassignments/template", "GET", p4ContactOwnerReassignmentEvidence, "contact.owner_reassignment", "human_session", "internal", "contact.owner_reassignment.local_template", "none", map[string]string{"admin": "global"}},
	"createContactOwnerReassignmentPreview":    {"/api/v1/contact-owner-reassignments/previews", "POST", p4ContactOwnerReassignmentEvidence, "contact.owner_reassignment", "human_session", "internal", "contact.owner_reassignment.local_preview", "required", map[string]string{"admin": "global"}},
	"getContactOwnerReassignmentPreview":       {"/api/v1/contact-owner-reassignments/previews/{preview_id}", "GET", p4ContactOwnerReassignmentEvidence, "contact.owner_reassignment", "human_session", "internal", "contact.owner_reassignment.local_preview", "none", map[string]string{"admin": "global"}},
	"executeContactOwnerReassignmentPreview":   {"/api/v1/contact-owner-reassignments/previews/{preview_id}/execute", "POST", p4ContactOwnerReassignmentEvidence, "contact.owner_reassignment", "human_session", "internal", "contact.owner_reassignment.local_transaction", "required", map[string]string{"admin": "global"}},
	"downloadContactOwnerReassignmentErrors":   {"/api/v1/contact-owner-reassignments/previews/{preview_id}/errors.csv", "GET", p4ContactOwnerReassignmentEvidence, "contact.owner_reassignment", "human_session", "internal", "contact.owner_reassignment.local_preview", "none", map[string]string{"admin": "global"}},
	"downloadContactOwnerReassignmentResults":  {"/api/v1/contact-owner-reassignments/previews/{preview_id}/results.csv", "GET", p4ContactOwnerReassignmentEvidence, "contact.owner_reassignment", "human_session", "internal", "contact.owner_reassignment.local_preview", "none", map[string]string{"admin": "global"}},
	"mintSidebarContext":                       {"/api/sidebar/context-token", "POST", p4SidebarLocalCoreEvidence, "customers.read", "optional_human_session", "internal_pii", "identity.local_read_model", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getSidebarWorkbench":                      {"/api/sidebar/v2/workbench", "GET", p4SidebarLocalCoreEvidence, "customers.read", "human_session_and_sidebar_context", "internal_pii", "sidebar.local_read_model", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"updateSidebarProfile":                     {"/api/sidebar/v2/profile", "PUT", p4SidebarLocalCoreEvidence, "customers.write", "human_session_and_sidebar_context", "internal_pii", "contact.local_command", "required", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listSidebarQuestionnaires":                {"/api/sidebar/v2/questionnaires", "GET", p4SidebarLocalCoreEvidence, "customers.read", "human_session_and_sidebar_context", "internal_deidentified", "survey.local_safe_read_model", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listSidebarOrders":                        {"/api/sidebar/v2/orders", "GET", p4SidebarLocalCoreEvidence, "customers.read", "human_session_and_sidebar_context", "financial", "order.local_safe_read_model", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listSidebarPeriodicOrders":                {"/api/sidebar/v2/periodic-orders", "GET", p4SidebarLocalCoreEvidence, "customers.read", "human_session_and_sidebar_context", "internal_pii", "service_period_members.local_read_model", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"updateSidebarPeriodicRemark":              {"/api/sidebar/v2/periodic-orders/{service_product_id}/members/{member_ref}/remark", "PUT", p4SidebarLocalCoreEvidence, "customers.write", "human_session_and_sidebar_context", "internal_pii", "service_period_members.local_command", "required", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listSidebarMaterials":                     {"/api/sidebar/v2/materials", "GET", p4SidebarLocalCoreEvidence, "customers.read", "human_session_and_sidebar_context", "internal", "media.local_read_model", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getSidebarMaterialThumbnailStatus":        {"/api/sidebar/v2/materials/image/{image_id}/thumbnail", "GET", p4SidebarLocalCoreEvidence, "customers.read", "human_session_and_sidebar_context", "internal", "media.local_existence_read", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"previewLegacyOrderExport":                 {"/api/admin/exports/preview", "POST", p4OrderSafeExportEvidence, "order.read", "human_session", "internal", "order.local_safe_projection", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getCustomerContactPolicy":                 {"/api/v1/customers/{customer_id}/contact-policy", "GET", p4ContactPolicyEvidence, "operations.read", "human_session", "internal", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"putCustomerContactPolicy":                 {"/api/v1/customers/{customer_id}/contact-policy", "PUT", p4ContactPolicyEvidence, "operations.manage", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"deleteCustomerContactPolicy":              {"/api/v1/customers/{customer_id}/contact-policy", "DELETE", p4ContactPolicyEvidence, "operations.manage", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"listCloudCampaignTouchPlans":              {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans", "GET", p4CampaignInitiationEvidence, "operations.read", "human_session", "internal", "campaign.touch_plan.local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"createCloudCampaignTouchPlan":             {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans", "POST", p4CampaignInitiationEvidence, "operations.manage", "human_session", "internal", "campaign.touch_plan.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getCloudCampaignTouchPlan":                {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}", "GET", p4CampaignInitiationEvidence, "operations.read", "human_session", "internal", "campaign.touch_plan.local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"listCloudCampaignTouchPlanRecipients":     {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/recipients", "GET", p4CampaignReviewHandoffEvidence, "operations.read", "human_session", "internal", "campaign.touch_plan.targets", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getCloudCampaignTouchPlanRecipient":       {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/recipients/{customer_id}", "GET", p4CampaignReviewHandoffEvidence, "operations.read", "human_session", "internal", "campaign.touch_plan.targets", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getCloudCampaignTouchPlanReview":          {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/review", "GET", p4CampaignReviewHandoffEvidence, "operations.read", "human_session", "internal", "campaign.touch_plan.review_local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"mutateCloudCampaignTouchPlanReview":       {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/review/{operation}", "POST", p4CampaignReviewHandoffEvidence, "operations.manage", "human_session", "internal", "campaign.touch_plan.review_local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getOutboundCampaignHandoffSummary":        {"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}", "GET", p4OutboundCampaignHandoffEvidence, "operations.read", "human_session", "internal", "outbound_campaign_handoffs.local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"acceptOutboundCampaignHandoff":            {"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/accept", "POST", p4OutboundCampaignHandoffEvidence, "operations.manage", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"reconcileOutboundCampaignHandoff":         {"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/reconciliation", "GET", p4OutboundCampaignHandoffEvidence, "operations.read", "human_session", "internal", "outbound_campaign_handoffs.local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	c01DispatchOperationID:                     {"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch", "POST", p4OutboundCampaignDispatchEvidence, "operations.manage", "human_session", "internal", "outbound_campaign_dispatches.local_control_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	c01DispatchReadOperationID:                 {"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch-reconciliation", "GET", p4OutboundCampaignDispatchEvidence, "operations.read", "human_session", "internal", "outbound_campaign_dispatches.local_projection", "none", map[string]string{"admin": "global", "ops": "global"}},
	c01DispatchReconcileOperationID:            {"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch-reconciliation/{effect_id}", "POST", p4OutboundCampaignDispatchEvidence, "operations.manage", "human_session", "internal", "outbound_campaign_dispatches.manual_reconcile_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"reorderStages":                            {"/api/v1/stages/reorder", "PUT", p4ClassificationPackageEvidence, "stages.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"archiveStage":                             {"/api/v1/stages/{stage_id}", "DELETE", p4ClassificationPackageEvidence, "stages.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"listTagGroups":                            {"/api/v1/tag-groups", "GET", p4ClassificationPackageEvidence, "customers.read", "human_session", "internal", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"createTagGroup":                           {"/api/v1/tag-groups", "POST", p4ClassificationPackageEvidence, "customers.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"updateTagGroup":                           {"/api/v1/tag-groups/{group_id}", "PATCH", p4ClassificationPackageEvidence, "customers.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"archiveTagGroup":                          {"/api/v1/tag-groups/{group_id}", "DELETE", p4ClassificationPackageEvidence, "customers.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"reorderTagGroups":                         {"/api/v1/tag-groups/reorder", "PUT", p4ClassificationPackageEvidence, "customers.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"createTag":                                {"/api/v1/tags", "POST", p4ClassificationPackageEvidence, "customers.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"updateTag":                                {"/api/v1/tags/{tag_id}", "PATCH", p4ClassificationPackageEvidence, "customers.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"archiveTag":                               {"/api/v1/tags/{tag_id}", "DELETE", p4ClassificationPackageEvidence, "customers.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"reorderTags":                              {"/api/v1/tags/reorder", "PUT", p4ClassificationPackageEvidence, "customers.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"archiveSegment":                           {"/api/v1/segments/{segment_id}", "DELETE", p4ClassificationPackageEvidence, "segments.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},

	"updateProduct":                   {"/api/v1/products/{product_id}", "PUT", p4ProductEntitlementEvidence, "products.write", "human_session", "financial", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"listProductLocalEntitlements":    {"/api/v1/products/{product_id}/local-entitlements", "GET", p4ProductEntitlementEvidence, "entitlements.read", "human_session", "internal_pii", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"grantProductLocalEntitlement":    {"/api/v1/products/{product_id}/local-entitlements", "POST", p4ProductEntitlementEvidence, "entitlements.write", "human_session", "internal_pii", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getProductLocalEntitlement":      {"/api/v1/product-entitlements/{entitlement_id}", "GET", p4ProductEntitlementEvidence, "entitlements.read", "human_session", "internal_pii", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"revokeProductLocalEntitlement":   {"/api/v1/product-entitlements/{entitlement_id}/revoke", "POST", p4ProductEntitlementEvidence, "entitlements.write", "human_session", "internal_pii", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"listServicePeriodMembers":        {"/api/admin/service-period-products/{service_product_id}/members", "GET", p4ServicePeriodMembersEvidence, "entitlements.read", "human_session", "internal_pii", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"addServicePeriodMember":          {"/api/admin/service-period-products/{service_product_id}/members", "POST", p4ServicePeriodMembersEvidence, "entitlements.write", "human_session", "internal_pii", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"exportServicePeriodMembers":      {"/api/admin/service-period-products/{service_product_id}/members/export", "POST", p4ServicePeriodMembersEvidence, "entitlements.read", "human_session", "internal_pii", "local_read_model", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getServicePeriodMember":          {"/api/admin/service-period-products/{service_product_id}/members/{member_ref}", "GET", p4ServicePeriodMembersEvidence, "entitlements.read", "human_session", "internal_pii", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"updateServicePeriodMemberFields": {"/api/admin/service-period-products/{service_product_id}/members/{member_ref}/fields", "PUT", p4ServicePeriodMembersEvidence, "entitlements.write", "human_session", "internal_pii", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"expireServicePeriodMember":       {"/api/admin/service-period-products/{service_product_id}/members/{member_ref}/expire", "POST", p4ServicePeriodMembersEvidence, "entitlements.write", "human_session", "internal_pii", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"removeServicePeriodMember":       {"/api/admin/service-period-products/{service_product_id}/members/{member_ref}/remove", "POST", p4ServicePeriodMembersEvidence, "entitlements.write", "human_session", "internal_pii", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},

	"getPublicSurveyDefinition":            {"/api/public/questionnaires/{slug}", "GET", p4SurveyPublicEvidence, "survey.public.read", "public", "public_non_pii", "local_read_model", "none", nil},
	"submitPublicSurvey":                   {"/api/public/questionnaires/{slug}/submissions", "POST", p4SurveyPublicEvidence, "survey.public.submit", "public", "public_non_pii", "local_command", "none", nil},
	"queryPublicSurveySubmissionResult":    {"/api/public/survey-submission-results/query", "POST", p4SurveyPublicEvidence, "survey.public.result", "public", "public_non_pii", "local_read_model", "none", nil},
	"publishQuestionnairePublicDefinition": {"/api/admin/questionnaires/{questionnaire_id}/public-publish", "POST", p4SurveyPublicEvidence, "questionnaires.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"disableQuestionnairePublicDefinition": {"/api/admin/questionnaires/{questionnaire_id}/public-disable", "POST", p4SurveyPublicEvidence, "questionnaires.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getQuestionnairePublicAnalytics":      {"/api/admin/questionnaires/{questionnaire_id}/public-analytics", "GET", p4SurveyPublicEvidence, "questionnaires.read", "human_session", "internal", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getPublicSurveyPage":                  {"/q/{slug}", "GET", p4SurveyPublicEvidence, "survey.public.page", "public", "public_non_pii", "static", "none", nil},
	"getSurveySafeSubmissionAnalysis":      {"/api/admin/questionnaires/{questionnaire_id}/analysis", "GET", p4SurveySafeAdminEvidence, "questionnaires.read", "human_session", "internal_deidentified", "survey_submission_snapshots.local_read", "none", map[string]string{"admin": "global", "ops": "global"}},
	"previewSurveySafeSubmissionExport":    {"/api/admin/questionnaires/{questionnaire_id}/export/preview", "POST", p4SurveySafeAdminEvidence, "questionnaires.read", "human_session", "internal_deidentified", "survey_submission_snapshots.local_read", "required", map[string]string{"admin": "global", "ops": "global"}},

	"listGroupOpsPlans":            {"/api/admin/automation-conversion/group-ops/plans", "GET", p4GroupOpsLocalEvidence, "admin.read", "human_session", "internal", "group_ops.local_read_model", "none", map[string]string{"admin": "global"}},
	"createGroupOpsPlan":           {"/api/admin/automation-conversion/group-ops/plans", "POST", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getGroupOpsPlan":              {"/api/admin/automation-conversion/group-ops/plans/{plan_id}", "GET", p4GroupOpsLocalEvidence, "admin.read", "human_session", "internal", "group_ops.local_read_model", "none", map[string]string{"admin": "global"}},
	"updateGroupOpsPlan":           {"/api/admin/automation-conversion/group-ops/plans/{plan_id}", "PATCH", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"activateGroupOpsPlan":         {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/activate", "POST", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"pauseGroupOpsPlan":            {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/pause", "POST", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"archiveGroupOpsPlan":          {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/archive", "POST", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"listGroupOpsPlanMembers":      {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/members", "GET", p4GroupOpsLocalEvidence, "admin.read", "human_session", "internal", "group_ops.local_read_model", "none", map[string]string{"admin": "global"}},
	"addGroupOpsPlanMember":        {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/members", "POST", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"removeGroupOpsPlanMember":     {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/members/{staff_id}", "DELETE", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"listGroupOpsPlanGroupAssets":  {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/group-assets", "GET", p4GroupOpsLocalEvidence, "admin.read", "human_session", "internal", "group_ops.local_read_model", "none", map[string]string{"admin": "global"}},
	"addGroupOpsPlanGroupAsset":    {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/group-assets", "POST", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"removeGroupOpsPlanGroupAsset": {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/group-assets/{asset_reference}", "DELETE", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"listGroupOpsPlanNodes":        {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/nodes", "GET", p4GroupOpsLocalEvidence, "admin.read", "human_session", "internal", "group_ops.local_read_model", "none", map[string]string{"admin": "global"}},
	"addGroupOpsPlanNode":          {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/nodes", "POST", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"updateGroupOpsPlanNode":       {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/nodes/{node_id}", "PATCH", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"removeGroupOpsPlanNode":       {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/nodes/{node_id}", "DELETE", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getGroupOpsWebhookDescriptor": {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/webhook-descriptor", "GET", p4GroupOpsLocalEvidence, "admin.read", "human_session", "internal", "group_ops.local_read_model", "none", map[string]string{"admin": "global"}},
	"putGroupOpsWebhookDescriptor": {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/webhook-descriptor", "PUT", p4GroupOpsLocalEvidence, "operations.manage", "human_session", "internal", "group_ops.local_transaction", "required", map[string]string{"admin": "global", "ops": "global"}},
	"previewGroupOpsPlanContent":   {"/api/admin/automation-conversion/group-ops/plans/{plan_id}/content/preview", "POST", p4GroupOpsLocalEvidence, "admin.read", "human_session", "internal", "group_ops.local_read_model", "required", map[string]string{"admin": "global"}},

	"getCloudOrchestratorWorkspace":              {"/admin/cloud-orchestrator", "GET", p4CloudOrchestratorEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getCloudOrchestratorPlansWorkspace":         {"/admin/cloud-orchestrator/plans", "GET", p4CloudOrchestratorEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getCloudOrchestratorPlanDetailWorkspace":    {"/admin/cloud-orchestrator/plans/{plan_id}", "GET", p4CloudOrchestratorEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getCloudOrchestratorCampaignsWorkspace":     {"/admin/cloud-orchestrator/campaigns", "GET", p4CloudOrchestratorEvidence, "operations.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getCloudOrchestratorObservabilityWorkspace": {"/admin/cloud-orchestrator/observability", "GET", p4CloudOrchestratorEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getGroupOpsPlansWorkspace":                  {"/admin/automation-conversion/group-ops/ui", "GET", p4GroupOpsWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getGroupOpsPlanDetailWorkspace":             {"/admin/automation-conversion/group-ops/plans/{plan_id}", "GET", p4GroupOpsWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getAudiencePackagesWorkspace":               {"/admin/automation-conversion", "GET", p4AudienceWorkspaceEvidence, "operations.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getAudiencePackageDetailWorkspace":          {"/admin/automation-conversion/packages/{package_id}", "GET", p4AudienceWorkspaceEvidence, "operations.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyOutboundJob":                       {"/api/admin/push-center/jobs/{job_id}", "GET", p4OutboundOperationsEvidence, "outbound.read", "human_session", "internal_pii", "outbound_tasks.local_read_model", "none", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"cancelLegacyOutboundJob":                    {"/api/admin/push-center/jobs/{job_id}/cancel", "POST", p4OutboundOperationsEvidence, "outbound.control", "human_session", "internal_pii", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"retryLegacyOutboundJob":                     {"/api/admin/push-center/jobs/{job_id}/retry", "POST", p4OutboundOperationsEvidence, "outbound.control", "human_session", "internal_pii", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getAlipayTransactionsWorkspace":             {"/admin/alipay/transactions", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getServiceProductsWorkspace":                {"/admin/service-period-products", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getServiceProductCreateWorkspace":           {"/admin/service-period-products/new", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getServiceProductEditWorkspace":             {"/admin/service-period-products/{service_product_id}/edit", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getServiceProductDataWorkspace":             {"/admin/service-period-products/{service_product_id}/data", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getWeChatPayProductCreateWorkspace":         {"/admin/wechat-pay/products/new", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getWeChatPayProductEditWorkspace":           {"/admin/wechat-pay/products/{product_id}/edit", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getWeChatPayTransactionsWorkspace":          {"/admin/wechat-pay/transactions", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getWeChatPayTransactionWorkspace":           {"/admin/wechat-pay/transactions/{order_id}", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getWeChatShopTransactionsWorkspace":         {"/admin/wechat-shop/transactions", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"getWeChatShopTransactionWorkspace":          {"/admin/wechat-shop/transactions/{order_id}", "GET", p4CommerceWorkspaceEvidence, "admin.read", "human_session", "internal", "static", "none", map[string]string{"admin": "global"}},
	"reorderLegacyHXCSendConfigs":                {"/api/admin/hxc-dashboard/send-config/reorder", "PUT", p4HXCSenderManagementEvidence, "operations.manage", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global"}},
	"listExternalEffectJobs":                     {"/api/admin/external-effects/jobs", "GET", p4ExternalEffectsReadonlyEvidence, "operations.read", "human_session", "internal", "outbound_tasks.local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getExternalEffectsDiagnostics":              {"/api/admin/external-effects/diagnostics", "GET", p4ExternalEffectsRuntimeEvidence, "operations.read", "human_session", "internal", "external_effects.local_safe_projection", "none", map[string]string{"admin": "global", "ops": "global"}},
	"listAIAudienceOperationMembers":             {"/api/admin/common/operation-members", "GET", p4AIAudienceConfigurationEvidence, "segments.read", "human_session", "internal", "staff.local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"getAIAudienceAutomationBinding":             {"/api/admin/ai-audience/packages/{package_id}/automation-binding", "GET", p4AIAudienceConfigurationEvidence, "segments.read", "human_session", "internal", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"putAIAudienceAutomationBinding":             {"/api/admin/ai-audience/packages/{package_id}/automation-binding", "PUT", p4AIAudienceConfigurationEvidence, "segments.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"deleteAIAudienceAutomationBinding":          {"/api/admin/ai-audience/packages/{package_id}/automation-binding", "DELETE", p4AIAudienceConfigurationEvidence, "segments.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
	"getAIAudiencePackageSenders":                {"/api/admin/ai-audience/packages/{package_id}/senders", "GET", p4AIAudienceConfigurationEvidence, "segments.read", "human_session", "internal", "local_read_model", "none", map[string]string{"admin": "global", "ops": "global"}},
	"replaceAIAudiencePackageSenders":            {"/api/admin/ai-audience/packages/{package_id}/senders", "PUT", p4AIAudienceConfigurationEvidence, "segments.write", "human_session", "internal", "local_command", "required", map[string]string{"admin": "global", "ops": "global"}},
}

// nativePackagePathParameters freezes identifiers which must cross generated
// JavaScript clients without IEEE-754 rounding. The runtime remains responsible
// for the stricter domain range check after receiving the lossless text value.
var nativePackagePathParameters = map[string]nativePackagePathParameter{
	"getInternalEventSafeExport": {
		name:      "export_id",
		typeName:  "string",
		pattern:   "^ese_[0-9a-f]{32}$",
		maxLength: 36,
	},
	"downloadInternalEventSafeExport": {
		name:      "export_id",
		typeName:  "string",
		pattern:   "^ese_[0-9a-f]{32}$",
		maxLength: 36,
	},
	"getGroupOpsPlanDetailWorkspace": {
		name:      "plan_id",
		typeName:  "string",
		pattern:   "^[1-9][0-9]{0,18}$",
		maxLength: 19,
	},
	"getAudiencePackageDetailWorkspace": {
		name:      "package_id",
		typeName:  "string",
		pattern:   "^[1-9][0-9]{0,18}$",
		maxLength: 19,
	},
	"getCloudCampaignTouchPlan": {
		name:      "plan_id",
		typeName:  "string",
		pattern:   "^ctp_[0-9a-f]{64}$",
		maxLength: 68,
	},
	"getOutboundCampaignHandoffSummary": {
		name:      "plan_id",
		typeName:  "string",
		pattern:   "^ctp_[0-9a-f]{64}$",
		maxLength: 68,
	},
	"acceptOutboundCampaignHandoff": {
		name:      "plan_id",
		typeName:  "string",
		pattern:   "^ctp_[0-9a-f]{64}$",
		maxLength: 68,
	},
	"reconcileOutboundCampaignHandoff": {
		name:      "plan_id",
		typeName:  "string",
		pattern:   "^ctp_[0-9a-f]{64}$",
		maxLength: 68,
	},
}

var nativePackageLaunchQueryContracts = map[string]nativePackageLaunchQueryContract{
	"getCloudOrchestratorCampaignsWorkspace": {
		kinds:           []any{"customer_selection", "segment_members", "ai_audience_package_members"},
		idPattern:       "^[1-9][0-9]{0,18}$",
		idMaximumLength: 19,
		idMaximumValue:  "9223372036854775807",
		location:        "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns",
		locationPattern: `^/[?]legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns%3Fsource_kind%3D(customer_selection|segment_members|ai_audience_package_members)%26source_id%3D([1-9][0-9]{0,17}|[1-8][0-9]{18}|9[01][0-9]{17}|92[01][0-9]{16}|922[0-2][0-9]{15}|9223[0-2][0-9]{14}|92233[0-6][0-9]{13}|922337[01][0-9]{12}|92233720[0-2][0-9]{10}|922337203[0-5][0-9]{9}|9223372036[0-7][0-9]{8}|92233720368[0-4][0-9]{7}|922337203685[0-3][0-9]{6}|9223372036854[0-6][0-9]{5}|92233720368547[0-6][0-9]{4}|922337203685477[0-4][0-9]{3}|9223372036854775[0-7][0-9]{2}|922337203685477580[0-7])$`,
	},
}

var p1CandidateOperations = map[string]bool{
	"listCustomers": true, "getCustomer": true, "updateCustomer": true,
	"listCustomerEvents": true, "resolveIdentity": true, "bindIdentity": true,
	"ingestIdentityEvent": true, "getAuthSession": true, "logoutAdmin": true,
	"getAdminConfigOverview": true,
}

var p2StageOperations = map[string]bool{
	"listStages": true, "createStage": true, "renameStage": true,
}

var p3ContactOperations = map[string]bool{
	"listTags": true, "setCustomerStage": true,
	"addCustomerTag": true, "removeCustomerTag": true,
}

var p3IdentityOperations = map[string]bool{
	"listIdentityMergeReviews": true, "approveIdentityMergeReview": true,
	"rejectIdentityMergeReview": true,
}

var p3SegmentOperations = map[string]bool{
	"listSegments": true, "createSegment": true, "getSegment": true,
	"updateSegment": true, "listSegmentMembers": true, "requestSegmentRefresh": true,
}

var p4AutomationOperations = map[string]bool{
	"listAutomationTriggerRuns": true,
}

var p4HXCSenderManagementOperations = map[string]bool{
	"upsertLegacyHXCSendConfig":  true,
	"archiveLegacyHXCSendConfig": true,
}

var p4HXCSenderManagementLegacyMappings = map[string][]string{
	"upsertLegacyHXCSendConfig":  {"LEGACY-API-0350"},
	"archiveLegacyHXCSendConfig": {"LEGACY-API-0351"},
}

var p4AutomationAgentOperations = map[string]bool{
	"getLegacyAutomationAgentListPage": true,
}

var p4AutomationAgentManagementOperations = map[string]bool{
	"createLegacyAutomationAgent": true, "getLegacyAutomationAgent": true, "updateLegacyAutomationAgent": true,
	"archiveLegacyAutomationAgent": true, "activateLegacyAutomationAgent": true, "copyLegacyAutomationAgent": true,
	"saveLegacyAutomationAgentFixedContent": true, "pauseLegacyAutomationAgent": true, "publishLegacyAutomationAgent": true,
}

var p4Customer360Operations = map[string]bool{
	"getCustomerContext": true, "listCustomerMergeHistory": true,
	"listCustomerChatActivity": true, "getCustomerActivityAnalytics": true,
	"listCustomerSurveyAnswers": true,
}

var p4Customer360ResponseSchemas = map[string]string{
	"getCustomerContext":           "CustomerContextResponse",
	"listCustomerMergeHistory":     "CustomerMergeHistoryResponse",
	"listCustomerChatActivity":     "CustomerChatActivityResponse",
	"getCustomerActivityAnalytics": "CustomerActivityAnalyticsResponse",
	"listCustomerSurveyAnswers":    "CustomerSurveyAnswerResponse",
}

var p4ProductOperations = map[string]bool{
	"listProducts": true, "createProduct": true, "getProduct": true, "getLegacyProductListPage": true,
	"enableLegacyWechatPayProduct": true, "disableLegacyWechatPayProduct": true,
	"copyLegacyWechatPayProduct": true, "deleteLegacyWechatPayProduct": true,
	"getLegacyWechatPayProductShare": true,
}

var p4ProductLegacyMappings = map[string][]string{
	"listProducts":                  {"LEGACY-API-0525"},
	"createProduct":                 {"LEGACY-API-0526"},
	"getProduct":                    {"LEGACY-API-0530"},
	"getLegacyProductListPage":      {"LEGACY-API-0079"},
	"enableLegacyWechatPayProduct":  {"LEGACY-API-0534"},
	"disableLegacyWechatPayProduct": {"LEGACY-API-0533"},
	"copyLegacyWechatPayProduct":    {"LEGACY-API-0532"},
	"deleteLegacyWechatPayProduct":  {"LEGACY-API-0529"},
}

const p4ServicePeriodLifecycleEvidence = "P4-SERVICE-PERIOD-LIFECYCLE-2026-08-22"
const p4ServicePeriodMemberGridReadEvidence = "P4-SERVICE-PERIOD-MEMBER-GRID-READ-2026-08-22"
const p4MemberGridManagementEvidence = "P4-SERVICE-PERIOD-MEMBER-GRID-MANAGEMENT-2026-08-22"
const p4RadarEvidence = "P4-RADAR-LOCAL-LIFECYCLE-2026-08-22"
const p4CloudCampaignEvidence = "P4-CLOUD-CAMPAIGN-LOCAL-LIFECYCLE-2026-08-22"
const p4AIAudienceEvidence = "P4-AI-AUDIENCE-LOCAL-LIFECYCLE-2026-08-22"

var p4MemberGridManagementOperations = map[string]bool{
	"createServicePeriodMemberView":             true,
	"updateServicePeriodMemberView":             true,
	"deleteServicePeriodMemberView":             true,
	"getServicePeriodMemberGridShareSettings":   true,
	"createServicePeriodMemberGridCollaborator": true,
	"updateServicePeriodMemberGridCollaborator": true,
	"deleteServicePeriodMemberGridCollaborator": true,
}

var p4ServicePeriodLifecycleOperations = map[string]bool{
	"listServicePeriodProducts": true, "createServicePeriodProduct": true,
	"getServicePeriodProduct": true, "updateServicePeriodProduct": true,
	"archiveServicePeriodProduct": true, "enableServicePeriodProduct": true,
	"disableServicePeriodProduct": true, "copyServicePeriodProduct": true,
}

var p4ServicePeriodLifecycleLegacyMappings = map[string][]string{
	"listServicePeriodProducts": {"LEGACY-API-0467"}, "createServicePeriodProduct": {"LEGACY-API-0468"},
	"getServicePeriodProduct": {"LEGACY-API-0471"}, "updateServicePeriodProduct": {"LEGACY-API-0472"},
	"archiveServicePeriodProduct": {"LEGACY-API-0470"}, "enableServicePeriodProduct": {"LEGACY-API-0475"},
	"disableServicePeriodProduct": {"LEGACY-API-0474"}, "copyServicePeriodProduct": {"LEGACY-API-0473"},
}

var p4ServicePeriodMemberGridReadOperations = map[string]bool{
	"getServicePeriodMemberGridAccess": true,
	"listServicePeriodMemberViews":     true,
}

var p4ServicePeriodMemberGridReadLegacyMappings = map[string][]string{
	"getServicePeriodMemberGridAccess": {"LEGACY-API-0476"},
	"listServicePeriodMemberViews":     {"LEGACY-API-0484"},
}

var p4MemberGridManagementLegacyMappings = map[string][]string{
	"createServicePeriodMemberView":             {"LEGACY-API-0485"},
	"updateServicePeriodMemberView":             {"LEGACY-API-0487"},
	"deleteServicePeriodMemberView":             {"LEGACY-API-0486"},
	"getServicePeriodMemberGridShareSettings":   {"LEGACY-API-0483"},
	"createServicePeriodMemberGridCollaborator": {"LEGACY-API-0477"},
	"updateServicePeriodMemberGridCollaborator": {"LEGACY-API-0479"},
	"deleteServicePeriodMemberGridCollaborator": {"LEGACY-API-0478"},
}

var p4RadarOperations = map[string]bool{
	"listRadarLinks": true, "createRadarLink": true, "getRadarLinkOptions": true,
	"getRadarLink": true, "updateRadarLink": true, "enableRadarLink": true,
	"disableRadarLink": true, "getRadarLinkShareProjection": true,
}

var p4RadarLegacyMappings = map[string][]string{
	"listRadarLinks": {"LEGACY-API-0445"}, "createRadarLink": {"LEGACY-API-0446"},
	"getRadarLinkOptions": {"LEGACY-API-0447"}, "getRadarLink": {"LEGACY-API-0453"},
	"updateRadarLink": {"LEGACY-API-0454"}, "disableRadarLink": {"LEGACY-API-0455"},
	"enableRadarLink": {"LEGACY-API-0456"}, "getRadarLinkShareProjection": {"LEGACY-API-0461"},
}

var p4CloudCampaignOperations = map[string]bool{
	"listCloudCampaigns": true, "batchStartCloudCampaigns": true,
	"getCloudCampaign": true, "deleteCloudCampaign": true,
	"addCloudCampaignStep": true, "updateCloudCampaignStep": true,
	"deleteCloudCampaignStep": true, "approveCloudCampaign": true,
	"startCloudCampaign": true, "rejectCloudCampaign": true,
	"pauseCloudCampaign": true,
}

var p4CloudCampaignLegacyMappings = map[string][]string{
	"listCloudCampaigns": {"LEGACY-API-0209"}, "batchStartCloudCampaigns": {"LEGACY-API-0211"},
	"getCloudCampaign": {"LEGACY-API-0217"}, "deleteCloudCampaign": {"LEGACY-API-0216"},
	"addCloudCampaignStep": {"LEGACY-API-0224"}, "updateCloudCampaignStep": {"LEGACY-API-0226"},
	"deleteCloudCampaignStep": {"LEGACY-API-0225"}, "approveCloudCampaign": {"LEGACY-API-0218"},
	"startCloudCampaign": {"LEGACY-API-0222"}, "rejectCloudCampaign": {"LEGACY-API-0221"},
	"pauseCloudCampaign": {"LEGACY-API-0220"},
}

var p4AIAudienceOperations = map[string]bool{
	"listAIAudiencePackageGroups": true, "createAIAudiencePackageGroup": true,
	"updateAIAudiencePackageGroup": true, "deleteAIAudiencePackageGroup": true,
	"listAIAudiencePackages": true, "getAIAudiencePackage": true,
	"updateAIAudiencePackage": true, "copyAIAudiencePackage": true,
	"pauseAIAudiencePackage": true, "activateAIAudiencePackage": true,
	"archiveAIAudiencePackage": true, "listAIAudiencePackageMembers": true,
}

var p4AIAudienceLegacyMappings = map[string][]string{
	"listAIAudiencePackageGroups": {"LEGACY-API-0089"}, "createAIAudiencePackageGroup": {"LEGACY-API-0090"},
	"deleteAIAudiencePackageGroup": {"LEGACY-API-0091"}, "updateAIAudiencePackageGroup": {"LEGACY-API-0092"},
	"listAIAudiencePackages": {"LEGACY-API-0093"}, "archiveAIAudiencePackage": {"LEGACY-API-0095"},
	"getAIAudiencePackage": {"LEGACY-API-0096"}, "updateAIAudiencePackage": {"LEGACY-API-0097"},
	"activateAIAudiencePackage": {"LEGACY-API-0098"}, "copyAIAudiencePackage": {"LEGACY-API-0102"},
	"pauseAIAudiencePackage":       {"LEGACY-API-0104"},
	"listAIAudiencePackageMembers": {"LEGACY-API-0103"},
}

var p4MediaOperations = map[string]bool{
	"uploadLegacyImage":     true,
	"listLegacyAttachments": true, "createLegacyAttachment": true, "uploadLegacyAttachment": true,
	"getLegacyAttachment": true, "updateLegacyAttachment": true, "deleteLegacyAttachment": true,
	"downloadLegacyAttachment": true,
}

var p4MediaLegacyMappings = map[string][]string{
	"uploadLegacyImage":     {"LEGACY-API-0361"},
	"listLegacyAttachments": {"LEGACY-API-0123"}, "createLegacyAttachment": {"LEGACY-API-0124"},
	"uploadLegacyAttachment": {"LEGACY-API-0125"}, "getLegacyAttachment": {"LEGACY-API-0127"},
	"updateLegacyAttachment": {"LEGACY-API-0128"}, "deleteLegacyAttachment": {"LEGACY-API-0126"},
	// The private download is the v2 safety projection of the legacy Attachment
	// GET mapping; it stays separately authenticated and metadata-free.
	"downloadLegacyAttachment": {"LEGACY-API-0127"},
}

var p4MediaEvidence = map[string]string{
	"uploadLegacyImage":     p4MediaDecisionEvidence,
	"listLegacyAttachments": p4AttachmentLibraryDecisionEvidence, "createLegacyAttachment": p4AttachmentLibraryDecisionEvidence,
	"uploadLegacyAttachment": p4AttachmentLibraryDecisionEvidence, "getLegacyAttachment": p4AttachmentLibraryDecisionEvidence,
	"updateLegacyAttachment": p4AttachmentLibraryDecisionEvidence, "deleteLegacyAttachment": p4AttachmentLibraryDecisionEvidence,
	"downloadLegacyAttachment": p4AttachmentLibraryDecisionEvidence,
}

var p4GroupInviteOperations = map[string]bool{
	"listLegacyGroupInvites": true, "createLegacyGroupInvite": true, "getLegacyGroupInvite": true,
	"updateLegacyGroupInvite": true, "archiveLegacyGroupInvite": true,
}

var p4GroupInviteLegacyMappings = map[string][]string{
	"listLegacyGroupInvites": {"LEGACY-API-0335"}, "createLegacyGroupInvite": {"LEGACY-API-0336"},
	"getLegacyGroupInvite": {"LEGACY-API-0338"}, "updateLegacyGroupInvite": {"LEGACY-API-0339"},
	"archiveLegacyGroupInvite": {"LEGACY-API-0337"},
}

var p4SurveyOperations = map[string]bool{
	"listLegacyQuestionnaires": true, "createLegacyQuestionnaire": true, "getLegacyQuestionnaire": true,
	"getLegacyQuestionnairePreflight": true,
	"replaceLegacyQuestionnaire":      true, "updateLegacyQuestionnaire": true,
	"deleteLegacyQuestionnaire": true, "duplicateLegacyQuestionnaire": true,
	"disableLegacyQuestionnaire": true, "enableLegacyQuestionnaire": true,
	"getLegacyQuestionnaireResults": true, "listLegacyQuestionnaireSubmissions": true,
	"exportLegacyQuestionnaireSubmissions": true,
	"listSurveyExternalPushLogs":           true, "listSurveyQuestionnaireExternalPushLogs": true,
	"getSurveyOperationsPageData": true, "getSurveyOperations": true,
	"saveSurveyCompletionOperations": true, "saveSurveyExternalPushOperations": true,
	"queueSurveyExternalPushTest": true,
}

var p4SurveyLegacyMappings = map[string][]string{
	"listLegacyQuestionnaires":                {"LEGACY-API-0423"},
	"createLegacyQuestionnaire":               {"LEGACY-API-0424"},
	"getLegacyQuestionnaire":                  {"LEGACY-API-0427"},
	"getLegacyQuestionnairePreflight":         {"LEGACY-API-0425"},
	"replaceLegacyQuestionnaire":              {"LEGACY-API-0429"},
	"updateLegacyQuestionnaire":               {"LEGACY-API-0428"},
	"deleteLegacyQuestionnaire":               {"LEGACY-API-0426"},
	"duplicateLegacyQuestionnaire":            {"LEGACY-API-0431"},
	"disableLegacyQuestionnaire":              {"LEGACY-API-0430"},
	"enableLegacyQuestionnaire":               {"LEGACY-API-0432"},
	"getLegacyQuestionnaireResults":           {"LEGACY-API-0442"},
	"listLegacyQuestionnaireSubmissions":      {"LEGACY-API-0444"},
	"exportLegacyQuestionnaireSubmissions":    {"LEGACY-API-0433"},
	"listSurveyExternalPushLogs":              {"LEGACY-API-0062"},
	"listSurveyQuestionnaireExternalPushLogs": {"LEGACY-API-0066"},
	"getSurveyOperationsPageData":             {"LEGACY-API-0067"},
	"getSurveyOperations":                     {"LEGACY-API-0436"},
	"saveSurveyCompletionOperations":          {"LEGACY-API-0437"},
	"saveSurveyExternalPushOperations":        {"LEGACY-API-0438"},
	"queueSurveyExternalPushTest":             {"LEGACY-API-0439"},
}

var p4SurveyEvidence = map[string]string{
	"listLegacyQuestionnaires":                "P4-F01A-2026-08-15",
	"createLegacyQuestionnaire":               "P4-F01A-2026-08-15",
	"getLegacyQuestionnaire":                  "P4-F01A-2026-08-15",
	"getLegacyQuestionnairePreflight":         "P4-F01A-2026-08-15",
	"replaceLegacyQuestionnaire":              "P4-F01AB-2026-08-15",
	"updateLegacyQuestionnaire":               "P4-F01AB-2026-08-15",
	"deleteLegacyQuestionnaire":               "P4-F01AB-2026-08-15",
	"duplicateLegacyQuestionnaire":            "P4-F01AB-2026-08-15",
	"disableLegacyQuestionnaire":              "P4-F01AB-2026-08-15",
	"enableLegacyQuestionnaire":               "P4-F01AB-2026-08-15",
	"getLegacyQuestionnaireResults":           "P4-F03-2026-08-18",
	"listLegacyQuestionnaireSubmissions":      "P4-F03-2026-08-18",
	"exportLegacyQuestionnaireSubmissions":    "P4-F03-2026-08-18",
	"listSurveyExternalPushLogs":              "P4-SURVEY-OPERATIONS-LOCAL-2026-08-22",
	"listSurveyQuestionnaireExternalPushLogs": "P4-SURVEY-OPERATIONS-LOCAL-2026-08-22",
	"getSurveyOperationsPageData":             "P4-SURVEY-OPERATIONS-LOCAL-2026-08-22",
	"getSurveyOperations":                     "P4-SURVEY-OPERATIONS-LOCAL-2026-08-22",
	"saveSurveyCompletionOperations":          "P4-SURVEY-OPERATIONS-LOCAL-2026-08-22",
	"saveSurveyExternalPushOperations":        "P4-SURVEY-OPERATIONS-LOCAL-2026-08-22",
	"queueSurveyExternalPushTest":             "P4-SURVEY-OPERATIONS-LOCAL-2026-08-22",
}

var p4ChannelOperations = map[string]bool{
	"listLegacyChannels": true, "createLegacyChannel": true, "getLegacyChannel": true, "updateLegacyChannel": true,
	"listLegacyChannelEntrants": true,
}

var p4ChannelLegacyMappings = map[string][]string{
	"listLegacyChannels":        {"LEGACY-API-0190"},
	"createLegacyChannel":       {"LEGACY-API-0191"},
	"getLegacyChannel":          {"LEGACY-API-0195"},
	"updateLegacyChannel":       {"LEGACY-API-0196"},
	"listLegacyChannelEntrants": {"LEGACY-API-0201"},
}

var p4ChannelEvidence = map[string]string{
	"listLegacyChannels":        p4ChannelDecisionEvidence,
	"createLegacyChannel":       p4ChannelDecisionEvidence,
	"getLegacyChannel":          p4ChannelDecisionEvidence,
	"updateLegacyChannel":       p4ChannelDecisionEvidence,
	"listLegacyChannelEntrants": "P4-S06-002-LOCAL-READ-2026-08-22",
}

var p4TagOperations = map[string]bool{
	"listLegacyWecomTags": true, "createLegacyWecomTagGroup": true,
	"updateLegacyWecomTagGroupPut": true, "updateLegacyWecomTagGroupPatch": true, "archiveLegacyWecomTagGroup": true,
	"createLegacyWecomTag": true, "updateLegacyWecomTagPut": true, "updateLegacyWecomTagPatch": true, "archiveLegacyWecomTag": true,
}

var p4TagLegacyMappings = map[string][]string{
	"listLegacyWecomTags": {"LEGACY-API-0555"}, "createLegacyWecomTagGroup": {"LEGACY-API-0552"},
	"updateLegacyWecomTagGroupPut": {"LEGACY-API-0553"}, "updateLegacyWecomTagGroupPatch": {"LEGACY-API-0553"}, "archiveLegacyWecomTagGroup": {"LEGACY-API-0553"},
	"createLegacyWecomTag": {"LEGACY-API-0556"}, "updateLegacyWecomTagPut": {"LEGACY-API-0562"}, "updateLegacyWecomTagPatch": {"LEGACY-API-0562"}, "archiveLegacyWecomTag": {"LEGACY-API-0562"},
}

var p4TagABOperations = map[string]bool{
	"legacyWecomTagsAdminShell": true, "listLegacyWecomTagGroups": true, "getLegacyWecomTagGroup": true,
	"getLegacyWecomTagExecutionGate": true, "queueLegacyWecomTagLiveMark": true, "queueLegacyWecomTagLiveUnmark": true,
	"queueLegacyWecomTagSync": true, "queueLegacyWecomTagSyncDue": true, "getLegacyWecomTag": true,
}

var p4TagABLegacyMappings = map[string][]string{
	"legacyWecomTagsAdminShell":      {"LEGACY-API-0086"},
	"listLegacyWecomTagGroups":       {"LEGACY-API-0551"},
	"getLegacyWecomTagGroup":         {"LEGACY-API-0554"},
	"getLegacyWecomTagExecutionGate": {"LEGACY-API-0557"},
	"queueLegacyWecomTagLiveMark":    {"LEGACY-API-0558"},
	"queueLegacyWecomTagLiveUnmark":  {"LEGACY-API-0559"},
	"queueLegacyWecomTagSync":        {"LEGACY-API-0560"},
	"queueLegacyWecomTagSyncDue":     {"LEGACY-API-0561"},
	"getLegacyWecomTag":              {"LEGACY-API-0563"},
}

var p4CouponOperations = map[string]bool{
	"listLegacyCoupons": true, "createLegacyCoupon": true, "getLegacyCoupon": true,
	"updateLegacyCoupon": true, "publishLegacyCoupon": true, "stopLegacyCoupon": true,
	"getLegacyCouponListPage": true, "getLegacyCouponNewPage": true,
	"getLegacyCouponDataPage": true, "getLegacyCouponEditPage": true,
	"listLegacyCouponProductOptions": true, "deleteLegacyCoupon": true,
	"archiveLegacyCoupon": true, "listLegacyCouponClaims": true,
	"copyLegacyCoupon": true, "getLegacyCouponShare": true,
	"listH5AvailableCoupons": true, "getH5CouponState": true,
	"claimH5Coupon": true, "listSidebarCoupons": true, "getPublicCouponPage": true,
}

var p4CouponLegacyMappings = map[string][]string{
	"listLegacyCoupons": {"LEGACY-API-0285"}, "createLegacyCoupon": {"LEGACY-API-0286"},
	"getLegacyCoupon": {"LEGACY-API-0289"}, "updateLegacyCoupon": {"LEGACY-API-0290"},
	"publishLegacyCoupon": {"LEGACY-API-0294"}, "stopLegacyCoupon": {"LEGACY-API-0296"},
	"getLegacyCouponListPage": {"LEGACY-API-0043"}, "getLegacyCouponNewPage": {"LEGACY-API-0044"},
	"getLegacyCouponDataPage": {"LEGACY-API-0045"}, "getLegacyCouponEditPage": {"LEGACY-API-0046"},
	"listLegacyCouponProductOptions": {"LEGACY-API-0287"}, "deleteLegacyCoupon": {"LEGACY-API-0288"},
	"archiveLegacyCoupon": {"LEGACY-API-0291"}, "listLegacyCouponClaims": {"LEGACY-API-0292"},
	"copyLegacyCoupon": {"LEGACY-API-0293"}, "getLegacyCouponShare": {"LEGACY-API-0295"},
	"listH5AvailableCoupons": {"LEGACY-API-0642"}, "getH5CouponState": {"LEGACY-API-0643"},
	"claimH5Coupon": {"LEGACY-API-0644"}, "listSidebarCoupons": {"LEGACY-API-0727"},
	"getPublicCouponPage": {"LEGACY-API-0756"},
}

var p4CouponEvidence = map[string]string{
	"listLegacyCoupons": p4CouponJ01DecisionEvidence, "createLegacyCoupon": p4CouponJ01DecisionEvidence,
	"getLegacyCoupon": p4CouponJ01DecisionEvidence, "updateLegacyCoupon": p4CouponJ01DecisionEvidence,
	"publishLegacyCoupon": p4CouponJ01DecisionEvidence, "stopLegacyCoupon": p4CouponJ01DecisionEvidence,
	"getLegacyCouponListPage": p4CouponABDecisionEvidence, "getLegacyCouponNewPage": p4CouponABDecisionEvidence,
	"getLegacyCouponDataPage": p4CouponABDecisionEvidence, "getLegacyCouponEditPage": p4CouponABDecisionEvidence,
	"listLegacyCouponProductOptions": p4CouponABDecisionEvidence, "deleteLegacyCoupon": p4CouponABDecisionEvidence,
	"archiveLegacyCoupon": p4CouponABDecisionEvidence, "listLegacyCouponClaims": p4CouponABDecisionEvidence,
	"copyLegacyCoupon": p4CouponABDecisionEvidence, "getLegacyCouponShare": p4CouponABDecisionEvidence,
	"listH5AvailableCoupons": p4CouponABDecisionEvidence, "getH5CouponState": p4CouponABDecisionEvidence,
	"claimH5Coupon": p4CouponABDecisionEvidence, "listSidebarCoupons": p4CouponABDecisionEvidence,
	"getPublicCouponPage": p4CouponABDecisionEvidence,
}

type couponPublicAccessContract struct {
	authScheme  string
	accessScope string
}

var couponPublicAccessContracts = map[string]couponPublicAccessContract{
	"listH5AvailableCoupons": {authScheme: "payment_identity_session", accessScope: "self"},
	"getH5CouponState":       {authScheme: "public"},
	"claimH5Coupon":          {authScheme: "payment_identity_session", accessScope: "self"},
	"listSidebarCoupons":     {authScheme: "sidebar_grant", accessScope: "owner"},
	"getPublicCouponPage":    {authScheme: "public"},
}

var p4OrderOperations = map[string]bool{
	"getLegacyOrderListPage": true, "listLegacyOrders": true, "getLegacyOrder": true, "getLegacyOrderItems": true,
	"listLegacyAlipayTransactions": true, "getLegacyAlipayTransaction": true,
	"listLegacyRefunds": true, "createLegacyRefundIntent": true,
	"createLegacyOrderExport": true, "getLegacyOrderExport": true,
	"createLegacyWechatOrderExport": true, "getDeprecatedLegacyWechatOrderExport": true,
	"downloadDeprecatedLegacyWechatOrderExport": true, "listLegacyWechatTransactions": true,
	"listLegacyWechatOrderExternalEffects": true, "reviewLegacyWechatOrderExternalEffect": true,
	"createLegacyWechatRefundIntent": true,
}

var p4OrderLegacyMappings = map[string][]string{
	"getLegacyOrderListPage": {"LEGACY-API-0058"}, "listLegacyOrders": {"LEGACY-API-0405"}, "getLegacyOrder": {"LEGACY-API-0406"},
	"getLegacyOrderItems": {"LEGACY-API-0407"}, "listLegacyAlipayTransactions": {"LEGACY-API-0119"},
	"getLegacyAlipayTransaction": {"LEGACY-API-0120"}, "listLegacyRefunds": {"LEGACY-API-0463"},
	"createLegacyRefundIntent": {"LEGACY-API-0464"}, "createLegacyOrderExport": {"LEGACY-API-0316"},
	"getLegacyOrderExport": {"LEGACY-API-0317"}, "createLegacyWechatOrderExport": {"LEGACY-API-0518"},
	"getDeprecatedLegacyWechatOrderExport": {"LEGACY-API-0519"}, "downloadDeprecatedLegacyWechatOrderExport": {"LEGACY-API-0520"},
	"listLegacyWechatTransactions": {"LEGACY-API-0521"}, "listLegacyWechatOrderExternalEffects": {"LEGACY-API-0522"},
	"reviewLegacyWechatOrderExternalEffect": {"LEGACY-API-0523"}, "createLegacyWechatRefundIntent": {"LEGACY-API-0524"},
}

var p4CustomerCompatOperations = map[string]bool{
	"listLegacyCustomers": true, "getLegacyCustomer": true,
}

var p4CustomerCompatLegacyMappings = map[string][]string{
	"listLegacyCustomers": {"LEGACY-API-0609"},
	"getLegacyCustomer":   {"LEGACY-API-0619"},
}

var p4ConfigSettingsOperations = map[string]bool{
	"getLegacyAppSettingsPage": true, "saveLegacyAppSettingsPage": true, "getLegacyAppSettingsResource": true,
	"saveLegacyAppSettingsResource": true,
}

var p4SetupWizardOperations = map[string]bool{
	"getSetupWizard": true, "saveSetupWizard": true,
}

var p4DomainVerificationOperations = map[string]bool{
	"getDomainVerificationFile": true,
}

var p4LegacyHealthOperations = map[string]bool{
	"getLegacyHealth": true,
}

var p4PushCenterOperations = map[string]bool{
	"getLegacyPushCenterSections": true, "getLegacyPushCenterStats": true,
}

var p4ExecutionRuntimeOperations = map[string]bool{
	"getLegacyExecutionRuntimePage": true, "getLegacyExecutionRuntime": true, "getLegacyExecutionTimeline": true,
}

var p4AdminShellOperations = map[string]bool{
	"getLegacyAdminShell": true, "getLegacyAdminLogoutCompat": true,
}

var p4PushCenterLegacyMappings = map[string][]string{
	"getLegacyPushCenterSections": {"LEGACY-API-0421"},
	"getLegacyPushCenterStats":    {"LEGACY-API-0422"},
}

var p4ExecutionRuntimeLegacyMappings = map[string][]string{
	"getLegacyExecutionRuntime": {"LEGACY-API-0314"}, "getLegacyExecutionTimeline": {"LEGACY-API-0315"},
}

var p4AdminShellLegacyMappings = map[string][]string{
	"getLegacyAdminShell": {"LEGACY-API-0001"}, "getLegacyAdminLogoutCompat": {"LEGACY-API-0053"},
}

var p4ConfigSettingsLegacyMappings = map[string][]string{
	"getLegacyAppSettingsPage": {"LEGACY-API-0026"}, "saveLegacyAppSettingsPage": {"LEGACY-API-0027"},
	"getLegacyAppSettingsResource": {"LEGACY-API-0253"}, "saveLegacyAppSettingsResource": {"LEGACY-API-0254"},
}

var p4ConfigSettingsEvidence = map[string]string{
	"getLegacyAppSettingsPage": "P4-A02-2026-08-15", "saveLegacyAppSettingsPage": "P4-A02-2026-08-15",
	"getLegacyAppSettingsResource": "P4-A02-2026-08-15", "saveLegacyAppSettingsResource": "P4-ADMINOPS-JOBS-AB-2026-08-16",
}

type p4AdminOpsSafeContract struct {
	path, method, mapping string
	write, blocked        bool
}

var p4AdminOpsSafeOperations = map[string]p4AdminOpsSafeContract{
	"getAdminOpsConfigPage":                  {"/admin/config", "GET", "LEGACY-API-0021", false, false},
	"getAdminOpsReleasesPage":                {"/admin/config/releases", "GET", "LEGACY-API-0035", false, false},
	"getAdminOpsNewReleasePage":              {"/admin/config/releases/new", "GET", "LEGACY-API-0037", false, false},
	"getAdminOpsReleasePage":                 {"/admin/config/releases/{release_id}", "GET", "LEGACY-API-0038", false, false},
	"listAdminOpsCategories":                 {"/api/admin/config/categories", "GET", "LEGACY-API-0256", false, false},
	"getAdminOpsCategory":                    {"/api/admin/config/categories/{category_key}", "GET", "LEGACY-API-0257", false, false},
	"checkAdminOpsCategory":                  {"/api/admin/config/categories/{category_key}/check", "POST", "LEGACY-API-0258", true, false},
	"setAdminOpsCategoryEnabled":             {"/api/admin/config/categories/{category_key}/enabled", "PUT", "LEGACY-API-0259", true, false},
	"setAdminOpsCategorySettings":            {"/api/admin/config/categories/{category_key}/settings", "PUT", "LEGACY-API-0260", true, false},
	"getAdminOpsPushCapabilities":            {"/api/admin/config/push-capabilities", "GET", "LEGACY-API-0270", false, false},
	"setAdminOpsPushScheduler":               {"/api/admin/config/push-capabilities/scheduler", "PATCH", "LEGACY-API-0271", true, false},
	"setAdminOpsPushCapability":              {"/api/admin/config/push-capabilities/{capability_key}", "PATCH", "LEGACY-API-0272", true, false},
	"listAdminOpsReleases":                   {"/api/admin/config/releases", "GET", "LEGACY-API-0273", false, false},
	"createAdminOpsRelease":                  {"/api/admin/config/releases", "POST", "LEGACY-API-0274", true, false},
	"getAdminOpsRelease":                     {"/api/admin/config/releases/{release_id}", "GET", "LEGACY-API-0275", false, false},
	"compareAdminOpsReleaseShadow":           {"/api/admin/config/releases/{release_id}/shadow-compare", "GET", "LEGACY-API-0278", false, false},
	"validateAdminOpsRelease":                {"/api/admin/config/releases/{release_id}/validate", "POST", "LEGACY-API-0279", true, false},
	"publishAdminOpsRelease":                 {"/api/admin/config/releases/{release_id}/publish", "POST", "LEGACY-API-0276", true, false},
	"rollbackAdminOpsRelease":                {"/api/admin/config/releases/{release_id}/rollback", "POST", "LEGACY-API-0277", true, false},
	"getAdminOpsJobsSummary":                 {"/api/admin/jobs/summary", "GET", "LEGACY-API-0384", false, false},
	"listAdminOpsArchiveSyncJobs":            {"/api/admin/jobs/archive-sync", "GET", "LEGACY-API-0376", false, false},
	"runAdminOpsArchiveSyncPlan":             {"/api/admin/jobs/archive-sync/run", "POST", "LEGACY-API-0377", true, false},
	"listAdminOpsCallbackJobs":               {"/api/admin/jobs/callbacks", "GET", "LEGACY-API-0378", false, true},
	"listAdminOpsDeferredJobs":               {"/api/admin/jobs/deferred-jobs", "GET", "LEGACY-API-0379", false, true},
	"listAdminOpsWebhookDeliveryJobs":        {"/api/admin/jobs/webhook-deliveries", "GET", "LEGACY-API-0385", false, true},
	"listAdminOpsMessageBatchJobs":           {"/api/admin/jobs/message-batches", "GET", "LEGACY-API-0380", false, false},
	"getAdminOpsMessageBatch":                {"/api/admin/jobs/message-batches/{batch_id}", "GET", "LEGACY-API-0381", false, true},
	"acknowledgeAdminOpsMessageBatch":        {"/api/admin/jobs/message-batches/{batch_id}/ack", "POST", "LEGACY-API-0382", true, false},
	"listAdminOpsBroadcastJobs":              {"/api/admin/broadcast-jobs", "GET", "LEGACY-API-0181", false, true},
	"runAdminOpsFeishuHourlyReportPlan":      {"/api/admin/broadcast-jobs/feishu-hourly-report/run", "POST", "LEGACY-API-0182", true, false},
	"getAdminOpsFeishuNotificationSetting":   {"/api/admin/broadcast-jobs/notification-settings/feishu", "GET", "LEGACY-API-0183", false, false},
	"saveAdminOpsFeishuNotificationSetting":  {"/api/admin/broadcast-jobs/notification-settings/feishu", "PUT", "LEGACY-API-0184", true, false},
	"validateAdminOpsFeishuNotificationPlan": {"/api/admin/broadcast-jobs/notification-settings/feishu/validate", "POST", "LEGACY-API-0185", true, false},
	"getAdminOpsBroadcastJob":                {"/api/admin/broadcast-jobs/{job_id}", "GET", "LEGACY-API-0186", false, true},
	"approveAdminOpsBroadcastJob":            {"/api/admin/broadcast-jobs/{job_id}/approve", "POST", "LEGACY-API-0187", true, true},
	"cancelAdminOpsBroadcastJob":             {"/api/admin/broadcast-jobs/{job_id}/cancel", "POST", "LEGACY-API-0188", true, true},
}

var identityOperations = map[string]bool{
	"resolveIdentity": true, "bindIdentity": true, "ingestIdentityEvent": true,
	"listIdentityMergeReviews": true, "approveIdentityMergeReview": true,
	"rejectIdentityMergeReview": true,
}

var contactOperations = map[string]bool{
	"listCustomers": true, "getCustomer": true, "updateCustomer": true,
	"listCustomerEvents": true, "listTags": true, "setCustomerStage": true,
	"addCustomerTag": true, "removeCustomerTag": true,
}

var segmentOperations = map[string]bool{
	"listSegments": true, "createSegment": true, "getSegment": true,
	"updateSegment": true, "listSegmentMembers": true, "requestSegmentRefresh": true,
}

type authorizationContract struct {
	capability string
	scopes     map[string]string
}

var authorizationContracts = map[string]authorizationContract{
	"getAdminOpsConfigPage":                      {"config.overview.read", map[string]string{"admin": "global"}},
	"getAdminOpsReleasesPage":                    {"config.overview.read", map[string]string{"admin": "global"}},
	"getAdminOpsNewReleasePage":                  {"config.overview.read", map[string]string{"admin": "global"}},
	"getAdminOpsReleasePage":                     {"config.overview.read", map[string]string{"admin": "global"}},
	"listAdminOpsCategories":                     {"config.overview.read", map[string]string{"admin": "global"}},
	"getAdminOpsCategory":                        {"config.overview.read", map[string]string{"admin": "global"}},
	"checkAdminOpsCategory":                      {"config.settings.manage", map[string]string{"admin": "global"}},
	"setAdminOpsCategoryEnabled":                 {"config.settings.manage", map[string]string{"admin": "global"}},
	"setAdminOpsCategorySettings":                {"config.settings.manage", map[string]string{"admin": "global"}},
	"getAdminOpsPushCapabilities":                {"config.overview.read", map[string]string{"admin": "global"}},
	"setAdminOpsPushScheduler":                   {"config.settings.manage", map[string]string{"admin": "global"}},
	"setAdminOpsPushCapability":                  {"config.settings.manage", map[string]string{"admin": "global"}},
	"listAdminOpsReleases":                       {"config.overview.read", map[string]string{"admin": "global"}},
	"createAdminOpsRelease":                      {"config.settings.manage", map[string]string{"admin": "global"}},
	"getAdminOpsRelease":                         {"config.overview.read", map[string]string{"admin": "global"}},
	"compareAdminOpsReleaseShadow":               {"config.overview.read", map[string]string{"admin": "global"}},
	"validateAdminOpsRelease":                    {"config.settings.manage", map[string]string{"admin": "global"}},
	"publishAdminOpsRelease":                     {"config.settings.manage", map[string]string{"admin": "global"}},
	"rollbackAdminOpsRelease":                    {"config.settings.manage", map[string]string{"admin": "global"}},
	"getAdminOpsJobsSummary":                     {"config.overview.read", map[string]string{"admin": "global"}},
	"listAdminOpsArchiveSyncJobs":                {"config.overview.read", map[string]string{"admin": "global"}},
	"runAdminOpsArchiveSyncPlan":                 {"config.settings.manage", map[string]string{"admin": "global"}},
	"listAdminOpsCallbackJobs":                   {"config.overview.read", map[string]string{"admin": "global"}},
	"listAdminOpsDeferredJobs":                   {"config.overview.read", map[string]string{"admin": "global"}},
	"listAdminOpsWebhookDeliveryJobs":            {"config.overview.read", map[string]string{"admin": "global"}},
	"listAdminOpsMessageBatchJobs":               {"config.overview.read", map[string]string{"admin": "global"}},
	"getAdminOpsMessageBatch":                    {"config.overview.read", map[string]string{"admin": "global"}},
	"acknowledgeAdminOpsMessageBatch":            {"config.settings.manage", map[string]string{"admin": "global"}},
	"listAdminOpsBroadcastJobs":                  {"config.overview.read", map[string]string{"admin": "global"}},
	"runAdminOpsFeishuHourlyReportPlan":          {"config.settings.manage", map[string]string{"admin": "global"}},
	"getAdminOpsFeishuNotificationSetting":       {"config.overview.read", map[string]string{"admin": "global"}},
	"saveAdminOpsFeishuNotificationSetting":      {"config.settings.manage", map[string]string{"admin": "global"}},
	"validateAdminOpsFeishuNotificationPlan":     {"config.settings.manage", map[string]string{"admin": "global"}},
	"getAdminOpsBroadcastJob":                    {"config.overview.read", map[string]string{"admin": "global"}},
	"approveAdminOpsBroadcastJob":                {"config.settings.manage", map[string]string{"admin": "global"}},
	"cancelAdminOpsBroadcastJob":                 {"config.settings.manage", map[string]string{"admin": "global"}},
	"listCustomers":                              {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getCustomer":                                {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"updateCustomer":                             {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listCustomerEvents":                         {"customer.events.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getCustomerContext":                         {"customer.events.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listCustomerMergeHistory":                   {"identity.review.read", map[string]string{"admin": "global", "ops": "global"}},
	"listCustomerChatActivity":                   {"customer.events.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getCustomerActivityAnalytics":               {"customer.events.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listCustomerSurveyAnswers":                  {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"resolveIdentity":                            {"identity.resolve", map[string]string{"admin": "global", "ops": "global"}},
	"bindIdentity":                               {"identity.bind", map[string]string{"admin": "global", "ops": "global"}},
	"ingestIdentityEvent":                        {"identity.ingest", map[string]string{"admin": "global", "ops": "global"}},
	"listIdentityMergeReviews":                   {"identity.review.read", map[string]string{"admin": "global", "ops": "global"}},
	"approveIdentityMergeReview":                 {"identity.review.write", map[string]string{"admin": "global", "ops": "global"}},
	"rejectIdentityMergeReview":                  {"identity.review.write", map[string]string{"admin": "global", "ops": "global"}},
	"getAuthSession":                             {"auth.session.read", map[string]string{"admin": "self", "ops": "self", "sales": "self"}},
	"logoutAdmin":                                {"auth.session.logout", map[string]string{"admin": "self", "ops": "self", "sales": "self"}},
	"getAdminConfigOverview":                     {"config.overview.read", map[string]string{"admin": "global"}},
	"createLegacyAutomationAgent":                {"config.settings.manage", map[string]string{"admin": "global"}},
	"getLegacyAutomationAgent":                   {"config.overview.read", map[string]string{"admin": "global"}},
	"updateLegacyAutomationAgent":                {"config.settings.manage", map[string]string{"admin": "global"}},
	"archiveLegacyAutomationAgent":               {"config.settings.manage", map[string]string{"admin": "global"}},
	"activateLegacyAutomationAgent":              {"config.settings.manage", map[string]string{"admin": "global"}},
	"copyLegacyAutomationAgent":                  {"config.settings.manage", map[string]string{"admin": "global"}},
	"saveLegacyAutomationAgentFixedContent":      {"config.settings.manage", map[string]string{"admin": "global"}},
	"pauseLegacyAutomationAgent":                 {"config.settings.manage", map[string]string{"admin": "global"}},
	"publishLegacyAutomationAgent":               {"config.settings.manage", map[string]string{"admin": "global"}},
	"listStages":                                 {"stages.read", map[string]string{"admin": "global", "ops": "global", "sales": "global"}},
	"createStage":                                {"stages.write", map[string]string{"admin": "global", "ops": "global"}},
	"renameStage":                                {"stages.write", map[string]string{"admin": "global", "ops": "global"}},
	"listTags":                                   {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"setCustomerStage":                           {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"addCustomerTag":                             {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"removeCustomerTag":                          {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listSegments":                               {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"getSegment":                                 {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"listSegmentMembers":                         {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"createSegment":                              {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateSegment":                              {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"requestSegmentRefresh":                      {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"listAutomationTriggerRuns":                  {"config.overview.read", map[string]string{"admin": "global"}},
	"getLegacyAutomationAgentListPage":           {"config.overview.read", map[string]string{"admin": "global"}},
	"getCloudOrchestratorWorkspace":              {"admin.read", map[string]string{"admin": "global"}},
	"getCloudOrchestratorPlansWorkspace":         {"admin.read", map[string]string{"admin": "global"}},
	"getCloudOrchestratorPlanDetailWorkspace":    {"admin.read", map[string]string{"admin": "global"}},
	"getCloudOrchestratorCampaignsWorkspace":     {"operations.read", map[string]string{"admin": "global", "ops": "global"}},
	"getCloudOrchestratorObservabilityWorkspace": {"admin.read", map[string]string{"admin": "global"}},
	"getAlipayTransactionsWorkspace":             {"admin.read", map[string]string{"admin": "global"}},
	"getServiceProductsWorkspace":                {"admin.read", map[string]string{"admin": "global"}},
	"getServiceProductCreateWorkspace":           {"admin.read", map[string]string{"admin": "global"}},
	"getServiceProductEditWorkspace":             {"admin.read", map[string]string{"admin": "global"}},
	"getServiceProductDataWorkspace":             {"admin.read", map[string]string{"admin": "global"}},
	"getWeChatPayProductCreateWorkspace":         {"admin.read", map[string]string{"admin": "global"}},
	"getWeChatPayProductEditWorkspace":           {"admin.read", map[string]string{"admin": "global"}},
	"getWeChatPayTransactionsWorkspace":          {"admin.read", map[string]string{"admin": "global"}},
	"getWeChatPayTransactionWorkspace":           {"admin.read", map[string]string{"admin": "global"}},
	"getWeChatShopTransactionsWorkspace":         {"admin.read", map[string]string{"admin": "global"}},
	"getWeChatShopTransactionWorkspace":          {"admin.read", map[string]string{"admin": "global"}},
	"listProducts":                               {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"createProduct":                              {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"getProduct":                                 {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyProductListPage":                   {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"enableLegacyWechatPayProduct":               {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"disableLegacyWechatPayProduct":              {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"copyLegacyWechatPayProduct":                 {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"deleteLegacyWechatPayProduct":               {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyWechatPayProductShare":             {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"listServicePeriodProducts":                  {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"createServicePeriodProduct":                 {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"getServicePeriodProduct":                    {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateServicePeriodProduct":                 {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveServicePeriodProduct":                {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"enableServicePeriodProduct":                 {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"disableServicePeriodProduct":                {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"copyServicePeriodProduct":                   {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"getServicePeriodMemberGridAccess":           {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"getServicePeriodMemberGridSchema":           {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"listServicePeriodMemberViews":               {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"queryServicePeriodMemberGrid":               {"entitlements.read", map[string]string{"admin": "global", "ops": "global"}},
	"createServicePeriodMemberView":              {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateServicePeriodMemberView":              {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"deleteServicePeriodMemberView":              {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"getServicePeriodMemberGridShareSettings":    {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"listRadarLinks":                             {"admin.read", map[string]string{"admin": "global", "ops": "global"}},
	"createRadarLink":                            {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"getRadarLinkOptions":                        {"admin.read", map[string]string{"admin": "global", "ops": "global"}},
	"getRadarLink":                               {"admin.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateRadarLink":                            {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"enableRadarLink":                            {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"disableRadarLink":                           {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"getRadarLinkShareProjection":                {"admin.read", map[string]string{"admin": "global", "ops": "global"}},
	"listCloudCampaigns":                         {"operations.read", map[string]string{"admin": "global", "ops": "global"}},
	"getCloudCampaign":                           {"operations.read", map[string]string{"admin": "global", "ops": "global"}},
	"batchStartCloudCampaigns":                   {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"deleteCloudCampaign":                        {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"addCloudCampaignStep":                       {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"updateCloudCampaignStep":                    {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"deleteCloudCampaignStep":                    {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"approveCloudCampaign":                       {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"startCloudCampaign":                         {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"rejectCloudCampaign":                        {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"pauseCloudCampaign":                         {"operations.manage", map[string]string{"admin": "global", "ops": "global"}},
	"listAIAudiencePackageGroups":                {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"createAIAudiencePackageGroup":               {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateAIAudiencePackageGroup":               {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"deleteAIAudiencePackageGroup":               {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"listAIAudiencePackages":                     {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"getAIAudiencePackage":                       {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateAIAudiencePackage":                    {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"copyAIAudiencePackage":                      {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"pauseAIAudiencePackage":                     {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"activateAIAudiencePackage":                  {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveAIAudiencePackage":                   {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"listAIAudiencePackageMembers":               {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"createServicePeriodMemberGridCollaborator":  {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateServicePeriodMemberGridCollaborator":  {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"deleteServicePeriodMemberGridCollaborator":  {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"uploadLegacyImage":                          {"media.images.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyAttachments":                      {"media.library.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyAttachment":                     {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"uploadLegacyAttachment":                     {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyAttachment":                        {"media.library.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyAttachment":                     {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"deleteLegacyAttachment":                     {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"downloadLegacyAttachment":                   {"media.library.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyGroupInvites":                     {"media.library.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyGroupInvite":                    {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyGroupInvite":                       {"media.library.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyGroupInvite":                    {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveLegacyGroupInvite":                   {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyQuestionnaires":                   {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyQuestionnaire":                  {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyQuestionnaire":                     {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"replaceLegacyQuestionnaire":                 {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyQuestionnaire":                  {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"deleteLegacyQuestionnaire":                  {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"duplicateLegacyQuestionnaire":               {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"disableLegacyQuestionnaire":                 {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"enableLegacyQuestionnaire":                  {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyQuestionnairePreflight":            {"admin.read", map[string]string{"admin": "global"}},
	"getLegacyQuestionnaireResults":              {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyQuestionnaireSubmissions":         {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"exportLegacyQuestionnaireSubmissions":       {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"listSurveyExternalPushLogs":                 {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"listSurveyQuestionnaireExternalPushLogs":    {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"getSurveyOperationsPageData":                {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"getSurveyOperations":                        {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"saveSurveyCompletionOperations":             {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"saveSurveyExternalPushOperations":           {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"queueSurveyExternalPushTest":                {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyChannels":                         {"channels.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyChannel":                        {"channels.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyChannel":                           {"channels.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyChannel":                        {"channels.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyChannelEntrants":                  {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyWecomTags":                        {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyPushCenterSections":                {"operations.read", map[string]string{"admin": "global"}},
	"getLegacyPushCenterStats":                   {"operations.read", map[string]string{"admin": "global"}},
	"getLegacyExecutionRuntimePage":              {"admin.read", map[string]string{"admin": "global"}},
	"getLegacyExecutionRuntime":                  {"admin.read", map[string]string{"admin": "global"}},
	"getLegacyExecutionTimeline":                 {"admin.read", map[string]string{"admin": "global"}},
	"createLegacyWecomTagGroup":                  {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyWecomTagGroupPut":               {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyWecomTagGroupPatch":             {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveLegacyWecomTagGroup":                 {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyWecomTag":                       {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyWecomTagPut":                    {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyWecomTagPatch":                  {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveLegacyWecomTag":                      {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"legacyWecomTagsAdminShell":                  {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyWecomTagGroups":                   {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyWecomTagGroup":                     {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyWecomTagExecutionGate":             {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"queueLegacyWecomTagLiveMark":                {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"queueLegacyWecomTagLiveUnmark":              {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"queueLegacyWecomTagSync":                    {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"queueLegacyWecomTagSyncDue":                 {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyWecomTag":                          {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyCoupons":                          {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyCoupon":                         {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCoupon":                            {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyCoupon":                         {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"publishLegacyCoupon":                        {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"stopLegacyCoupon":                           {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponListPage":                    {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponNewPage":                     {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponDataPage":                    {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponEditPage":                    {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyCouponProductOptions":             {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"deleteLegacyCoupon":                         {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveLegacyCoupon":                        {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyCouponClaims":                     {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"copyLegacyCoupon":                           {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponShare":                       {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyOrderListPage":                     {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyOrders":                           {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyOrder":                             {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyOrderItems":                        {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyAlipayTransactions":               {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyAlipayTransaction":                 {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyRefunds":                          {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyRefundIntent":                   {"order.write", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyOrderExport":                    {"order.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyOrderExport":                       {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyWechatOrderExport":              {"order.write", map[string]string{"admin": "global", "ops": "global"}},
	"getDeprecatedLegacyWechatOrderExport":       {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"downloadDeprecatedLegacyWechatOrderExport":  {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyWechatTransactions":               {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyWechatOrderExternalEffects":       {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"reviewLegacyWechatOrderExternalEffect":      {"order.write", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyWechatRefundIntent":             {"order.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyCustomers":                        {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getLegacyCustomer":                          {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getLegacyAppSettingsPage":                   {"config.settings.manage", map[string]string{"admin": "global"}},
	"saveLegacyAppSettingsPage":                  {"config.settings.manage", map[string]string{"admin": "global"}},
	"getLegacyAppSettingsResource":               {"config.settings.manage", map[string]string{"admin": "global"}},
	"saveLegacyAppSettingsResource":              {"config.settings.manage", map[string]string{"admin": "global"}},
	"getSetupWizard":                             {"config.overview.read", map[string]string{"admin": "global"}},
	"saveSetupWizard":                            {"config.settings.manage", map[string]string{"admin": "global"}},
	"getLegacyAdminShell":                        {"admin.shell.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyAdminLogoutCompat":                 {"admin.shell.read", map[string]string{"admin": "global", "ops": "global"}},
	"upsertLegacyHXCSendConfig":                  {"operations.manage", map[string]string{"admin": "global"}},
	"archiveLegacyHXCSendConfig":                 {"operations.manage", map[string]string{"admin": "global"}},
}

const g1DecisionEvidence = "G1-D01-2026-08-10"
const p2StageDecisionEvidence = "P2-16-2026-08-11"
const p3ContactDecisionEvidence = "P3-C00-2026-08-12"
const p3IdentityDecisionEvidence = "P3-I00-2026-08-12"
const p3SegmentDecisionEvidence = "P3-S00-2026-08-12"
const p4AutomationDecisionEvidence = "P4-W0-D01-2026-08-14"
const p4AutomationAgentDecisionEvidence = "P4-AUTOMATION-AGENT-BROWSER-061-2026-08-20"
const p4AutomationAgentManagementDecisionEvidence = "P4-AUTOMATION-AGENT-MANAGEMENT-2026-08-20"
const p4Customer360DecisionEvidence = "P4-CUSTOMER-360-READ-2026-08-20"
const p4ProductDecisionEvidence = "P4-I01A-2026-08-14"
const (
	p4MediaDecisionEvidence             = "P4-H01A1-2026-08-14"
	p4AttachmentLibraryDecisionEvidence = "P4-ATTACHMENT-LIBRARY-00062-2026-08-23"
)
const p4GroupInviteDecisionEvidence = "P4-H03-2026-08-15"
const p4ChannelDecisionEvidence = "P4-C01-2026-08-15"
const p4TagDecisionEvidence = "P4-B02-2026-08-15"
const p4TagABDecisionEvidence = "P4-B02AB-2026-08-15"
const p4CouponJ01DecisionEvidence = "P4-J01-2026-08-15"
const p4CouponABDecisionEvidence = "P4-COUPON-AB-2026-08-15"
const p4OrderDecisionEvidence = "P4-ORDER-AB-2026-08-15"
const p4CustomerCompatDecisionEvidence = "P4-B01-2026-08-15"
const p4DomainVerificationDecisionEvidence = "P4-S04-DOMAIN-VERIFICATION-2026-08-16"
const p4LegacyHealthDecisionEvidence = "P4-S04-LEGACY-HEALTH-2026-08-18"
const p4PushCenterDecisionEvidence = "P4-PUSH-CENTER-0421-0422-2026-08-16"
const p4ExecutionRuntimeDecisionEvidence = "P4-EXECUTION-RUNTIME-AB-2026-08-16"
const p4AdminShellDecisionEvidence = "P4-ADMIN-SHELL-AB-2026-08-16"

func main() {
	spec := flag.String("spec", "../api/openapi.yaml", "OpenAPI document")
	mapping := flag.String("mapping", "../docs/api-mapping.jsonl", "legacy API mapping")
	flag.Parse()
	doc, inventory, err := load(*spec, *mapping)
	if err == nil {
		err = validate(doc, inventory)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "openapi-contract:", err)
		os.Exit(1)
	}
	fmt.Println("openapi-contract: PASS")
}

func load(spec, mapping string) (*openapi3.T, mappingInventory, error) {
	empty := mappingInventory{}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(spec)
	if err != nil {
		return nil, empty, err
	}
	file, err := os.Open(mapping)
	if err != nil {
		return nil, empty, err
	}
	defer file.Close()
	inventory := mappingInventory{Known: map[string]bool{}, Candidates: map[string]canonicalCandidateOperation{}}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var row struct {
			MappingID            string `json:"mapping_id"`
			CandidateOperationID string `json:"candidate_v2_operation_id"`
			CandidateMethod      string `json:"candidate_v2_method"`
			CandidatePath        string `json:"candidate_v2_path"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, empty, err
		}
		if row.MappingID == "" || inventory.Known[row.MappingID] {
			return nil, empty, errors.New("invalid legacy mapping IDs")
		}
		inventory.Known[row.MappingID] = true
		if row.CandidateOperationID == "" || row.CandidateOperationID == "PENDING_HUMAN_DESIGN" || row.CandidateOperationID == "NOT_APPLICABLE" {
			continue
		}
		if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`).MatchString(row.CandidateOperationID) ||
			!regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE)$`).MatchString(row.CandidateMethod) ||
			!strings.HasPrefix(row.CandidatePath, "/") || strings.Contains(row.CandidatePath, "//") {
			return nil, empty, fmt.Errorf("invalid canonical candidate declaration: %s", row.MappingID)
		}
		candidate := inventory.Candidates[row.CandidateOperationID]
		if candidate.Path != "" && (candidate.Path != row.CandidatePath || candidate.Method != row.CandidateMethod) {
			return nil, empty, fmt.Errorf("inconsistent canonical candidate declaration: %s", row.CandidateOperationID)
		}
		candidate.Path = row.CandidatePath
		candidate.Method = row.CandidateMethod
		candidate.MappingIDs = append(candidate.MappingIDs, row.MappingID)
		inventory.Candidates[row.CandidateOperationID] = candidate
	}
	if err := scanner.Err(); err != nil {
		return nil, empty, err
	}
	if len(inventory.Known) == 0 {
		return nil, empty, errors.New("legacy mapping inventory is empty")
	}
	return doc, inventory, nil
}

func isRunnerDeclaredOperation(operationID string) bool {
	return p1CandidateOperations[operationID] || p2StageOperations[operationID] ||
		p3ContactOperations[operationID] || p3IdentityOperations[operationID] || p3SegmentOperations[operationID] ||
		p4AutomationOperations[operationID] || p4HXCSenderManagementOperations[operationID] || p4AutomationAgentOperations[operationID] || p4AutomationAgentManagementOperations[operationID] || p4Customer360Operations[operationID] || p4ProductOperations[operationID] || p4ServicePeriodLifecycleOperations[operationID] || p4ServicePeriodMemberGridReadOperations[operationID] || p4MemberGridManagementOperations[operationID] || p4RadarOperations[operationID] || p4CloudCampaignOperations[operationID] || p4AIAudienceOperations[operationID] || p4MediaOperations[operationID] ||
		p4GroupInviteOperations[operationID] || p4SurveyOperations[operationID] || p4ChannelOperations[operationID] ||
		p4TagOperations[operationID] || p4TagABOperations[operationID] || p4CouponOperations[operationID] ||
		p4OrderOperations[operationID] || p4CustomerCompatOperations[operationID] || p4ConfigSettingsOperations[operationID] || p4AdminOpsSafeOperations[operationID].path != "" || p4SetupWizardOperations[operationID] ||
		p4DomainVerificationOperations[operationID] || p4PushCenterOperations[operationID] ||
		p4ExecutionRuntimeOperations[operationID] || p4AdminShellOperations[operationID] ||
		p4LegacyHealthOperations[operationID] || nativePackageOperationDeclared(operationID) || pe01OperationDeclared(operationID)
}

func pe01OperationDeclared(operationID string) bool { _, ok := pe01Operations[operationID]; return ok }

func validatePE01Operation(path string, item *openapi3.PathItem, op *openapi3.Operation, contract nativePackageOperation) error {
	if path != contract.path || operationForMethod(item, contract.method) != op || op.Extensions["x-p4-decision-evidence"] != contract.evidence || op.Extensions["x-aicrm-auth-scheme"] != contract.authScheme || op.Extensions["x-aicrm-data-classification"] != contract.classification || op.Extensions["x-aicrm-data-source"] != contract.dataSource || op.Extensions["x-aicrm-session-bound-csrf"] != contract.csrf {
		return fmt.Errorf("%s PE01 route boundary drifted", op.OperationID)
	}
	if contract.authScheme == "wechat_pay_signature" {
		if op.Security == nil || len(*op.Security) != 0 || op.Extensions["x-aicrm-external-effect"] != "provider_callback" {
			return fmt.Errorf("%s PE01 callback boundary drifted", op.OperationID)
		}
		if _, ok := op.Extensions["x-aicrm-capability"]; ok {
			return fmt.Errorf("%s PE01 callback must not declare human capability", op.OperationID)
		}
		return nil
	}
	if op.Extensions["x-aicrm-capability"] != contract.capability {
		return fmt.Errorf("%s PE01 capability drifted", op.OperationID)
	}
	scopes, err := stringMap(op.Extensions["x-aicrm-rbac-scopes"])
	if err != nil || !reflect.DeepEqual(scopes, contract.scopes) {
		return fmt.Errorf("%s PE01 RBAC scopes=%v", op.OperationID, scopes)
	}
	wantEffect := "none"
	if contract.method == "POST" {
		wantEffect = "accepted_only"
	}
	if op.Extensions["x-aicrm-external-effect"] != wantEffect {
		return fmt.Errorf("%s PE01 external effect drifted", op.OperationID)
	}
	return nil
}

func validateP4AdminOpsSafeOperation(path string, item *openapi3.PathItem, op *openapi3.Operation, contract p4AdminOpsSafeContract, known map[string]bool) error {
	if path != contract.path || operationForMethod(item, contract.method) != op || !known[contract.mapping] {
		return fmt.Errorf("%s AdminOps path, method, or mapping drifted", op.OperationID)
	}
	ids, err := stringList(op.Extensions["x-legacy-mapping-ids"])
	if err != nil || !reflect.DeepEqual(ids, []string{contract.mapping}) {
		return fmt.Errorf("%s AdminOps legacy mapping=%v", op.OperationID, ids)
	}
	capability, csrf, source := "config.overview.read", "none", "local_read_model"
	if contract.write {
		capability, csrf, source = "config.settings.manage", "required", "local_command"
	}
	if op.OperationID == "checkAdminOpsCategory" {
		source = "local_read_model"
	}
	if contract.blocked {
		switch op.OperationID {
		case "getAdminOpsMessageBatch":
			source = "unavailable_owner_mapping"
		case "approveAdminOpsBroadcastJob":
			source = "unavailable_review_state"
		case "listAdminOpsCallbackJobs", "listAdminOpsDeferredJobs", "listAdminOpsWebhookDeliveryJobs":
			source = "unavailable_job_kind"
		default:
			source = "unavailable_broadcast_fact"
		}
	}
	if op.Extensions["x-p4-decision-evidence"] != "P4-ADMINOPS-SAFE-PROJECTION-2026-08-23" ||
		op.Extensions["x-aicrm-capability"] != capability || op.Extensions["x-aicrm-auth-scheme"] != "human_session" ||
		op.Extensions["x-aicrm-session-bound-csrf"] != csrf || op.Extensions["x-aicrm-data-classification"] != "internal" ||
		op.Extensions["x-aicrm-data-source"] != source || op.Extensions["x-aicrm-external-effect"] != "none" ||
		op.Extensions["x-aicrm-local-only"] != true || op.Responses.Value("401") == nil ||
		op.Responses.Value("403") == nil || op.Responses.Value("503") == nil {
		return fmt.Errorf("%s AdminOps security or local-only boundary drifted", op.OperationID)
	}
	if contract.write && op.Extensions["x-aicrm-route-bound-action-token"] != "required" {
		return fmt.Errorf("%s AdminOps action-token boundary drifted", op.OperationID)
	}
	if contract.blocked {
		if op.Extensions["x-p4-status"] != "BLOCKED_REDLINE" || op.Responses.Value("409") == nil || op.Responses.Value("200") != nil || op.Responses.Value("202") != nil {
			return fmt.Errorf("%s must remain an explicit BLOCKED_REDLINE", op.OperationID)
		}
	} else if op.Responses.Value("200") == nil && op.Responses.Value("202") == nil {
		return fmt.Errorf("%s AdminOps success response is missing", op.OperationID)
	}
	return nil
}

func nativePackageOperationDeclared(operationID string) bool {
	_, ok := nativePackageOperations[operationID]
	return ok
}

func operationForMethod(item *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case "GET":
		return item.Get
	case "POST":
		return item.Post
	case "PUT":
		return item.Put
	case "PATCH":
		return item.Patch
	case "DELETE":
		return item.Delete
	default:
		return nil
	}
}

func validateCanonicalCandidate(path string, item *openapi3.PathItem, op *openapi3.Operation, contract canonicalCandidateOperation, known map[string]bool) error {
	if path != contract.Path || operationForMethod(item, contract.Method) != op {
		return fmt.Errorf("%s path or method differs from canonical mapping", op.OperationID)
	}
	ids, err := stringList(op.Extensions["x-legacy-mapping-ids"])
	if err != nil || !reflect.DeepEqual(ids, contract.MappingIDs) {
		return fmt.Errorf("%s canonical legacy mapping=%v", op.OperationID, ids)
	}
	for _, id := range ids {
		if !known[id] {
			return fmt.Errorf("%s links unknown mapping %s", op.OperationID, id)
		}
	}
	return nil
}

func validateGenericCanonicalAuthorization(op *openapi3.Operation, method string) error {
	decisionEvidence := ""
	for _, key := range []string{"x-p1-decision-evidence", "x-p2-decision-evidence", "x-p3-decision-evidence", "x-p4-decision-evidence"} {
		if value, ok := op.Extensions[key].(string); ok && value != "" {
			decisionEvidence = value
			break
		}
	}
	capability, capabilityOK := op.Extensions["x-aicrm-capability"].(string)
	authScheme, authSchemeOK := op.Extensions["x-aicrm-auth-scheme"].(string)
	classification, classificationOK := op.Extensions["x-aicrm-data-classification"].(string)
	_, scopesDeclared := op.Extensions["x-aicrm-rbac-scopes"]
	scopes, scopeErr := stringMap(op.Extensions["x-aicrm-rbac-scopes"])
	if authSchemeOK && authScheme == "public" {
		csrf, csrfOK := op.Extensions["x-aicrm-csrf"].(string)
		if decisionEvidence == "" || !capabilityOK || !regexp.MustCompile(`^[a-z][a-z0-9.]*$`).MatchString(capability) ||
			!classificationOK || classification != "public_non_pii" || scopesDeclared || scopeErr == nil || len(scopes) != 0 ||
			op.Security == nil || len(*op.Security) != 0 || !csrfOK || csrf != "none" || method != "GET" ||
			op.Extensions["x-aicrm-external-effect"] != "none" {
			return fmt.Errorf("%s canonical public declaration is incomplete", op.OperationID)
		}
		return nil
	}
	if decisionEvidence == "" || !capabilityOK || !regexp.MustCompile(`^[a-z][a-z0-9.]*$`).MatchString(capability) ||
		!authSchemeOK || authScheme == "" || !classificationOK || classification == "" ||
		scopeErr != nil || len(scopes) == 0 || op.Extensions["x-aicrm-external-effect"] != "none" {
		return fmt.Errorf("%s canonical authorization declaration is incomplete", op.OperationID)
	}
	for role, scope := range scopes {
		if (role != "admin" && role != "ops" && role != "sales") ||
			(scope != "global" && scope != "owner_staff" && scope != "self") {
			return fmt.Errorf("%s canonical RBAC scopes=%v", op.OperationID, scopes)
		}
	}
	if len(scopes) < 3 && op.Responses.Value("403") == nil {
		return fmt.Errorf("%s denies a role but lacks 403", op.OperationID)
	}
	csrf, csrfOK := op.Extensions["x-aicrm-session-bound-csrf"].(string)
	if !csrfOK || (method == "GET" && csrf != "none") || (method != "GET" && csrf != "required") {
		return fmt.Errorf("%s canonical CSRF declaration is incomplete", op.OperationID)
	}
	return nil
}

func validateNativePackageOperation(path string, item *openapi3.PathItem, op *openapi3.Operation, contract nativePackageOperation) error {
	if path != contract.path || operationForMethod(item, contract.method) != op {
		return fmt.Errorf("%s path or method differs from owner-approved native package", op.OperationID)
	}
	if _, linked := op.Extensions["x-legacy-mapping-ids"]; linked {
		return fmt.Errorf("%s native package operation must not claim a legacy mapping", op.OperationID)
	}
	if op.Extensions["x-p4-decision-evidence"] != contract.evidence ||
		op.Extensions["x-aicrm-capability"] != contract.capability ||
		op.Extensions["x-aicrm-auth-scheme"] != contract.authScheme ||
		op.Extensions["x-aicrm-data-classification"] != contract.classification ||
		op.Extensions["x-aicrm-data-source"] != contract.dataSource ||
		op.Extensions["x-aicrm-external-effect"] != "none" {
		return fmt.Errorf("%s native package security or data boundary drifted", op.OperationID)
	}
	if contract.authScheme == "public" {
		if op.Security == nil || len(*op.Security) != 0 || op.Extensions["x-aicrm-csrf"] != contract.csrf {
			return fmt.Errorf("%s public native package authorization drifted", op.OperationID)
		}
		if _, declared := op.Extensions["x-aicrm-rbac-scopes"]; declared {
			return fmt.Errorf("%s public native package operation must not declare RBAC scopes", op.OperationID)
		}
		return nil
	}
	if op.Extensions["x-aicrm-session-bound-csrf"] != contract.csrf {
		return fmt.Errorf("%s native package CSRF declaration drifted", op.OperationID)
	}
	scopes, err := stringMap(op.Extensions["x-aicrm-rbac-scopes"])
	if err != nil || !reflect.DeepEqual(scopes, contract.scopes) {
		return fmt.Errorf("%s native package RBAC scopes=%v", op.OperationID, scopes)
	}
	if len(contract.scopes) < 3 && op.Responses.Value("403") == nil {
		return fmt.Errorf("%s native package operation denies a role but lacks 403", op.OperationID)
	}
	if parameterContract, frozen := nativePackagePathParameters[op.OperationID]; frozen {
		parameter := item.Parameters.GetByInAndName("path", parameterContract.name)
		if parameter == nil || !parameter.Required || parameter.Schema == nil || parameter.Schema.Value == nil {
			return fmt.Errorf("%s lossless path parameter contract is missing", op.OperationID)
		}
		schema := parameter.Schema.Value
		if schema.Type == nil || !schema.Type.Is(parameterContract.typeName) || schema.Format != "" ||
			schema.Pattern != parameterContract.pattern || schema.MaxLength == nil || *schema.MaxLength != parameterContract.maxLength ||
			schema.Min != nil || schema.Max != nil {
			return fmt.Errorf("%s lossless path parameter contract drifted", op.OperationID)
		}
	}
	if queryContract, frozen := nativePackageLaunchQueryContracts[op.OperationID]; frozen {
		if err := validateNativePackageLaunchQuery(op, queryContract); err != nil {
			return fmt.Errorf("%s %w", op.OperationID, err)
		}
	}
	return nil
}

func validateNativePackageLaunchQuery(op *openapi3.Operation, contract nativePackageLaunchQueryContract) error {
	if op.Extensions["x-aicrm-query-contract"] != "none_or_exact_source_pair" || len(op.Parameters) != 2 {
		return errors.New("launch query parameter count drifted")
	}
	kind := op.Parameters.GetByInAndName("query", "source_kind")
	id := op.Parameters.GetByInAndName("query", "source_id")
	if kind == nil || kind.Required || kind.Schema == nil || kind.Schema.Value == nil ||
		kind.Schema.Value.Type == nil || !kind.Schema.Value.Type.Is("string") || kind.Schema.Value.Format != "" ||
		!reflect.DeepEqual(kind.Schema.Value.Enum, contract.kinds) {
		return errors.New("source_kind launch query drifted")
	}
	if id == nil || id.Required || id.Schema == nil || id.Schema.Value == nil ||
		id.Schema.Value.Type == nil || !id.Schema.Value.Type.Is("string") || id.Schema.Value.Format != "" ||
		id.Schema.Value.Pattern != contract.idPattern || id.Schema.Value.MinLength != 1 ||
		id.Schema.Value.MaxLength == nil || *id.Schema.Value.MaxLength != contract.idMaximumLength ||
		id.Schema.Value.Min != nil || id.Schema.Value.Max != nil ||
		id.Schema.Value.Extensions["x-aicrm-decimal-maximum"] != contract.idMaximumValue {
		return errors.New("source_id launch query drifted")
	}
	malformed := op.Responses.Value("400")
	redirect := op.Responses.Value("302")
	if malformed == nil || redirect == nil || redirect.Value == nil {
		return errors.New("launch query responses drifted")
	}
	location := redirect.Value.Headers["Location"]
	if location == nil || location.Value == nil || location.Value.Schema == nil || location.Value.Schema.Value == nil ||
		len(location.Value.Schema.Value.OneOf) != 2 {
		return errors.New("launch query Location drifted")
	}
	withoutSource := location.Value.Schema.Value.OneOf[0]
	withSource := location.Value.Schema.Value.OneOf[1]
	if withoutSource == nil || withoutSource.Value == nil || withoutSource.Value.Type == nil || !withoutSource.Value.Type.Is("string") ||
		!reflect.DeepEqual(withoutSource.Value.Enum, []any{contract.location}) ||
		withSource == nil || withSource.Value == nil || withSource.Value.Type == nil || !withSource.Value.Type.Is("string") ||
		withSource.Value.Pattern != contract.locationPattern {
		return errors.New("launch query Location alternatives drifted")
	}
	return nil
}

func validateOutboundCancelOperation(op *openapi3.Operation) error {
	if op == nil || op.RequestBody != nil || !operationResponseUsesStatusLocalSchema(op, "202", "LegacyOutboundCancelResponse") ||
		op.Responses.Value("400") == nil || op.Responses.Value("401") == nil || op.Responses.Value("403") == nil ||
		op.Responses.Value("404") == nil || op.Responses.Value("409") == nil || op.Responses.Value("503") == nil {
		return errors.New("cancelLegacyOutboundJob closed response contract drifted")
	}
	if len(op.Parameters) != 3 {
		return errors.New("cancelLegacyOutboundJob parameter contract drifted")
	}
	want := map[string]string{"job_id": "path", "X-CSRF-Token": "header", "Idempotency-Key": "header"}
	for _, reference := range op.Parameters {
		if reference == nil || reference.Value == nil || !reference.Value.Required || want[reference.Value.Name] != reference.Value.In {
			return errors.New("cancelLegacyOutboundJob parameter contract drifted")
		}
		delete(want, reference.Value.Name)
	}
	if len(want) != 0 {
		return errors.New("cancelLegacyOutboundJob parameter contract drifted")
	}
	return nil
}

func validateOutboundSafeProjectionContract(doc *openapi3.T) error {
	operations := []struct {
		path, method, schema, status string
	}{
		{"/api/admin/push-center/jobs", "GET", "LegacyOutboundJobListResponse", "200"},
		{"/api/admin/push-center/jobs/{job_id}", "GET", "LegacyOutboundJobDetailResponse", "200"},
		{"/api/admin/push-center/jobs/{job_id}/reconciliation", "GET", "LegacyOutboundJobReconciliationResponse", "200"},
		{"/api/admin/push-center/jobs/{job_id}/cancel", "POST", "LegacyOutboundCancelResponse", "202"},
		{"/api/admin/push-center/jobs/{job_id}/retry", "POST", "LegacyOutboundRetryResponse", "202"},
	}
	for _, contract := range operations {
		item := doc.Paths.Value(contract.path)
		if item == nil {
			return fmt.Errorf("%s %s outbound safe projection contract is missing", contract.method, contract.path)
		}
		operation := operationForMethod(item, contract.method)
		if operation == nil || operation.Extensions["x-aicrm-local-only"] != true ||
			!operationResponseUsesStatusLocalSchema(operation, contract.status, contract.schema) {
			return fmt.Errorf("%s %s outbound safe projection contract drifted", contract.method, contract.path)
		}
	}
	for _, name := range []string{"LegacyOutboundJob", "LegacyOutboundAttempt"} {
		schema := doc.Components.Schemas[name]
		if !closedOutboundProjectionSchema(schema, true) {
			return fmt.Errorf("%s must remain a closed provider-safe projection", name)
		}
	}
	for _, name := range []string{"LegacyOutboundControlReceipt", "LegacyOutboundCancelReceipt", "LegacyOutboundRetryReceipt"} {
		schema := doc.Components.Schemas[name]
		if !closedOutboundProjectionSchema(schema, false) || !legacyTagBooleanEnum(schema.Value, "provider_receipt_present", false) {
			return fmt.Errorf("%s must remain a closed local receipt projection", name)
		}
	}
	for _, name := range []string{"LegacyOutboundJobListResponse", "LegacyOutboundJobDetailResponse", "LegacyOutboundJobReconciliationResponse", "LegacyOutboundCancelResponse", "LegacyOutboundRetryResponse"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has ||
			!legacyTagBooleanEnum(schema.Value, "local_fact_only", true) ||
			!legacyTagBooleanEnum(schema.Value, "real_external_call_executed", false) ||
			!legacyTagStringEnum(schema.Value, "delivery_semantics", "local_state_not_delivery_proof") {
			return fmt.Errorf("%s must keep the local-only delivery boundary", name)
		}
	}
	return nil
}

func validateOutboundCampaignHandoffContract(doc *openapi3.T) error {
	operations := map[string]struct {
		method, schema, status string
	}{
		"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}":                {"GET", "OutboundCampaignHandoffSummary", "200"},
		"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/accept":         {"POST", "OutboundCampaignHandoffReconciliation", "200"},
		"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/reconciliation": {"GET", "OutboundCampaignHandoffReconciliation", "200"},
		"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch":       {"POST", "OutboundCampaignDispatchReconciliation", "200"},
		"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch-reconciliation": {
			"GET", "OutboundCampaignDispatchReconciliation", "200"},
		"/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/dispatch-reconciliation/{effect_id}": {
			"POST", "OutboundCampaignDispatchReconciliation", "200"},
	}
	for path := range doc.Paths.Map() {
		if strings.HasPrefix(path, "/api/admin/outbound/campaign-handoffs/") {
			if _, allowed := operations[path]; !allowed {
				return fmt.Errorf("Outbound Campaign dispatch/release route must remain EXTERNAL_GATE: %s", path)
			}
		}
	}
	for path, contract := range operations {
		item := doc.Paths.Value(path)
		if item == nil {
			return fmt.Errorf("%s Outbound Campaign handoff path is missing", path)
		}
		operation := operationForMethod(item, contract.method)
		if operation == nil || operation.Extensions["x-aicrm-local-only"] != true ||
			!operationResponseUsesStatusLocalSchema(operation, contract.status, contract.schema) {
			return fmt.Errorf("%s %s Outbound Campaign handoff contract drifted", contract.method, path)
		}
		if path == "/api/admin/outbound/campaign-handoffs/{campaign_code}/{plan_id}/accept" {
			if !strings.Contains(operation.Description, "internal Events delivery River job") ||
				!strings.Contains(operation.Description, "no Outbound send job") || !strings.Contains(operation.Description, "no Provider") {
				return errors.New("Outbound Campaign accept must disclose its exact local Events job boundary")
			}
			want := map[string]string{"X-CSRF-Token": "header", "Idempotency-Key": "header"}
			for _, reference := range operation.Parameters {
				if reference == nil || reference.Value == nil || !reference.Value.Required || want[reference.Value.Name] != reference.Value.In {
					return errors.New("Outbound Campaign accept header contract drifted")
				}
				delete(want, reference.Value.Name)
			}
			if len(want) != 0 {
				return errors.New("Outbound Campaign accept header contract drifted")
			}
		}
	}
	for _, name := range []string{"OutboundCampaignHandoffSummary", "OutboundCampaignHandoffReconciliation"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has {
			return fmt.Errorf("%s must remain a closed safe projection", name)
		}
		for _, forbidden := range []string{"customer_id", "customer_ids", "outbound_task_id", "provider_message_id", "provider_code", "last_error", "raw_identity", "event_id"} {
			if schema.Value.Properties[forbidden] != nil {
				return fmt.Errorf("%s exposes forbidden field %s", name, forbidden)
			}
		}
	}
	safety := doc.Components.Schemas["OutboundCampaignHandoffSafety"]
	if safety == nil || safety.Value == nil || safety.Value.AdditionalProperties.Has == nil || *safety.Value.AdditionalProperties.Has ||
		!legacyTagBooleanEnum(safety.Value, "local_only", true) ||
		!legacyTagBooleanEnum(safety.Value, "provider_execution_eligible", false) ||
		!legacyTagBooleanEnum(safety.Value, "real_external_call_executed", false) ||
		!legacyTagBooleanEnum(safety.Value, "delivery_proven", false) {
		return errors.New("Outbound Campaign handoff safety contract drifted")
	}
	return nil
}

func validateInternalEventRegistryContract(doc *openapi3.T) error {
	wantConsumers := []string{
		"automation.tag-trigger.v1",
		"stats.tag-applied.v1",
		"operation-cycle.fact.v1",
		"cloud-campaign.fact.v1",
		"outbound-campaign-handoff.fact.v1",
	}
	for _, path := range []string{"/api/admin/internal-events", "/api/admin/internal-events/diagnostics"} {
		item := doc.Paths.Value(path)
		if item == nil || item.Get == nil {
			return fmt.Errorf("internal Events registry path is missing: %s", path)
		}
		parameter := item.Get.Parameters.GetByInAndName("query", "consumer")
		if parameter == nil || parameter.Schema == nil || parameter.Schema.Value == nil || !reflect.DeepEqual(parameter.Schema.Value.Enum, stringsToAny(wantConsumers)) {
			return fmt.Errorf("internal Events consumer query registry drifted: %s", path)
		}
	}

	delivery := doc.Components.Schemas["LegacyInternalEventDelivery"]
	filters := doc.Components.Schemas["LegacyInternalEventFilters"]
	binding := doc.Components.Schemas["LegacyInternalEventConsumerBinding"]
	diagnostics := doc.Components.Schemas["LegacyInternalEventDiagnosticsResponse"]
	if delivery == nil || delivery.Value == nil || filters == nil || filters.Value == nil || binding == nil || binding.Value == nil || diagnostics == nil || diagnostics.Value == nil {
		return errors.New("internal Events registry schema is missing")
	}
	if !reflect.DeepEqual(delivery.Value.Properties["consumer"].Value.Enum, stringsToAny(wantConsumers)) ||
		!reflect.DeepEqual(filters.Value.Properties["consumer"].Value.Enum, stringsToAny(append([]string{""}, wantConsumers...))) ||
		!reflect.DeepEqual(binding.Value.Properties["consumer"].Value.Enum, stringsToAny(wantConsumers)) {
		return errors.New("internal Events consumer schema registry drifted")
	}
	wantEventTypes := []string{"customer.tag_applied", "operation_cycle.fact_recorded", "cloud_campaign.fact_recorded", "outbound.campaign_handoff_fact_recorded"}
	eventTypes := binding.Value.Properties["event_types"]
	if eventTypes == nil || eventTypes.Value == nil || eventTypes.Value.Items == nil || eventTypes.Value.Items.Value == nil ||
		!reflect.DeepEqual(eventTypes.Value.Items.Value.Enum, stringsToAny(wantEventTypes)) {
		return errors.New("internal Events event type registry drifted")
	}
	registry := diagnostics.Value.Properties["consumer_registry"]
	if registry == nil || registry.Value == nil || registry.Value.MinItems != uint64(len(wantConsumers)) || registry.Value.MaxItems == nil || *registry.Value.MaxItems != uint64(len(wantConsumers)) {
		return errors.New("internal Events diagnostics registry count drifted")
	}
	return nil
}

func stringsToAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func closedOutboundProjectionSchema(reference *openapi3.SchemaRef, includeFailure bool) bool {
	if reference == nil || reference.Value == nil || reference.Value.AdditionalProperties.Has == nil || *reference.Value.AdditionalProperties.Has {
		return false
	}
	schema := reference.Value
	for _, forbidden := range []string{"failure", "code", "last_error", "provider_code", "provider_message_id", "message_id", "provider_receipt"} {
		if schema.Properties[forbidden] != nil {
			return false
		}
	}
	if !legacyTagBooleanEnum(schema, "delivery_proven", false) || !legacyTagBooleanEnum(schema, "local_fact_only", true) ||
		!legacyTagBooleanEnum(schema, "real_external_call_executed", false) ||
		!legacyTagStringEnum(schema, "delivery_semantics", "local_state_not_delivery_proof") ||
		schema.Properties["provider_receipt_present"] == nil {
		return false
	}
	if !includeFailure {
		return true
	}
	failurePresent := schema.Properties["failure_present"]
	failureClass := schema.Properties["failure_class"]
	if failurePresent == nil || failurePresent.Value == nil || failureClass == nil || failureClass.Value == nil {
		return false
	}
	values, err := stringList(failureClass.Value.Enum)
	sort.Strings(values)
	return err == nil && reflect.DeepEqual(values, []string{"local_failure", "none", "outcome_unknown"})
}

func validate(doc *openapi3.T, inventory mappingInventory) error {
	return validateContracts(doc, inventory, true)
}

func validateContracts(doc *openapi3.T, inventory mappingInventory, validateOpenAPI bool) error {
	known := inventory.Known
	if validateOpenAPI {
		if err := doc.Validate(context.Background()); err != nil {
			return err
		}
	}
	if len(doc.Security) == 0 {
		return errors.New("business API lacks default security")
	}
	seenP1, seenP2 := map[string]bool{}, map[string]bool{}
	seenP3Contact, seenP3Identity, seenP3Segment := map[string]bool{}, map[string]bool{}, map[string]bool{}
	seenP4Automation, seenP4HXCSenderManagement, seenP4AutomationAgent, seenP4AutomationAgentManagement, seenP4Customer360, seenP4Product, seenP4ServicePeriodLifecycle, seenP4ServicePeriodMemberGridRead, seenP4MemberGridManagement, seenP4Radar, seenP4CloudCampaign, seenP4AIAudience, seenP4Media, seenP4GroupInvite, seenP4Survey, seenP4Channel, seenP4Tag, seenP4TagAB, seenP4Coupon, seenP4Order, seenP4CustomerCompat, seenP4ConfigSettings, seenP4SetupWizard, seenP4DomainVerification, seenP4PushCenter, seenP4ExecutionRuntime, seenP4AdminShell, seenP4LegacyHealth := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	seenP4AdminOpsSafe := map[string]bool{}
	seenOperationIDs, seenCanonical := map[string]bool{}, map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for _, op := range item.Operations() {
			if path == "/healthz" {
				continue
			}
			runnerDeclared := isRunnerDeclaredOperation(op.OperationID)
			canonicalContract, canonicalDeclared := inventory.Candidates[op.OperationID]
			if seenOperationIDs[op.OperationID] || (!runnerDeclared && !canonicalDeclared) {
				return fmt.Errorf("unexpected or duplicate candidate operation: %s", op.OperationID)
			}
			seenOperationIDs[op.OperationID] = true
			if canonicalDeclared {
				if err := validateCanonicalCandidate(path, item, op, canonicalContract, known); err != nil {
					return err
				}
				seenCanonical[op.OperationID] = true
			}
			canonicalFallback := canonicalDeclared && !runnerDeclared
			if p1CandidateOperations[op.OperationID] {
				seenP1[op.OperationID] = true
				status, ok := op.Extensions["x-p1-signoff-status"].(string)
				if !ok || status != "APPROVED" {
					return fmt.Errorf("%s lacks approved G1 signoff", op.OperationID)
				}
				evidence, ok := op.Extensions["x-p1-decision-evidence"].(string)
				if !ok || evidence != g1DecisionEvidence {
					return fmt.Errorf("%s has missing or forged G1 evidence", op.OperationID)
				}
				ids, err := stringList(op.Extensions["x-legacy-mapping-ids"])
				if err != nil || len(ids) == 0 {
					return fmt.Errorf("%s lacks legacy links", op.OperationID)
				}
				for _, id := range ids {
					if !known[id] {
						return fmt.Errorf("%s links unknown mapping %s", op.OperationID, id)
					}
				}
			} else if p2StageOperations[op.OperationID] {
				seenP2[op.OperationID] = true
				evidence, ok := op.Extensions["x-p2-decision-evidence"].(string)
				if !ok || evidence != p2StageDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P2 stage evidence", op.OperationID)
				}
			} else if p3ContactOperations[op.OperationID] {
				seenP3Contact[op.OperationID] = true
				if ids, ok := op.Extensions["x-legacy-mapping-ids"]; ok {
					legacyIDs, err := stringList(ids)
					if err != nil || len(legacyIDs) == 0 {
						return fmt.Errorf("%s has invalid legacy links", op.OperationID)
					}
					for _, id := range legacyIDs {
						if !known[id] {
							return fmt.Errorf("%s links unknown mapping %s", op.OperationID, id)
						}
					}
				}
			} else if p3IdentityOperations[op.OperationID] {
				seenP3Identity[op.OperationID] = true
			} else if p3SegmentOperations[op.OperationID] {
				seenP3Segment[op.OperationID] = true
			} else if p4AutomationOperations[op.OperationID] {
				seenP4Automation[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4AutomationDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Automation evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, []string{"LEGACY-API-0141"}) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4HXCSenderManagementOperations[op.OperationID] {
				seenP4HXCSenderManagement[op.OperationID] = true
				if op.Extensions["x-p4-decision-evidence"] != p4HXCSenderManagementEvidence ||
					op.Extensions["x-aicrm-auth-scheme"] != "human_session" ||
					op.Extensions["x-aicrm-session-bound-csrf"] != "required" ||
					op.Extensions["x-aicrm-data-source"] != "local_command" ||
					op.Extensions["x-aicrm-external-effect"] != "none" {
					return fmt.Errorf("%s HXC local management boundary drifted", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4HXCSenderManagementLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4AutomationAgentOperations[op.OperationID] {
				seenP4AutomationAgent[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4AutomationAgentDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Automation Agent evidence", op.OperationID)
				}
				if _, linked := op.Extensions["x-legacy-mapping-ids"]; linked {
					return fmt.Errorf("%s must not duplicate the LEGACY-API-0129 API mapping", op.OperationID)
				}
				if op.Extensions["x-aicrm-capability"] != "config.overview.read" || op.Extensions["x-aicrm-auth-scheme"] != "human_session" || op.Extensions["x-aicrm-session-bound-csrf"] != "none" || op.Extensions["x-aicrm-data-classification"] != "internal" || op.Extensions["x-aicrm-external-effect"] != "none" {
					return fmt.Errorf("%s Automation Agent carrier contract drifted", op.OperationID)
				}
				scopes, scopeErr := stringMap(op.Extensions["x-aicrm-rbac-scopes"])
				if scopeErr != nil || !reflect.DeepEqual(scopes, map[string]string{"admin": "global"}) || op.Responses.Value("403") == nil {
					return fmt.Errorf("%s Automation Agent carrier must stay admin/global and fail closed", op.OperationID)
				}
			} else if p4AutomationAgentManagementOperations[op.OperationID] {
				seenP4AutomationAgentManagement[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4AutomationAgentManagementDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Automation Agent management evidence", op.OperationID)
				}
				if _, linked := op.Extensions["x-legacy-mapping-ids"]; linked {
					return fmt.Errorf("%s must not close the independently automated legacy ledger", op.OperationID)
				}
				if op.Extensions["x-aicrm-auth-scheme"] != "human_session" || op.Extensions["x-aicrm-data-classification"] != "internal" ||
					op.Extensions["x-aicrm-external-effect"] != "none" || op.Responses.Value("401") == nil || op.Responses.Value("403") == nil || op.Responses.Value("503") == nil {
					return fmt.Errorf("%s local Automation Agent management boundary drifted", op.OperationID)
				}
				if op.OperationID == "getLegacyAutomationAgent" {
					if op.Extensions["x-aicrm-session-bound-csrf"] != "none" {
						return fmt.Errorf("%s read must not require CSRF", op.OperationID)
					}
				} else if op.Extensions["x-aicrm-session-bound-csrf"] != "required" {
					return fmt.Errorf("%s write must require CSRF", op.OperationID)
				}
			} else if p4Customer360Operations[op.OperationID] {
				seenP4Customer360[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4Customer360DecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Customer 360 evidence", op.OperationID)
				}
				if _, linked := op.Extensions["x-legacy-mapping-ids"]; linked {
					return fmt.Errorf("%s must not claim a legacy Customer mapping", op.OperationID)
				}
				if op.Extensions["x-aicrm-auth-scheme"] != "human_session" || op.Extensions["x-aicrm-session-bound-csrf"] != "none" ||
					op.Extensions["x-aicrm-data-classification"] != "internal_pii" || op.Extensions["x-aicrm-data-source"] != "local_read_model" ||
					op.Extensions["x-aicrm-external-effect"] != "none" || !operationResponseUsesLocalSchema(op, p4Customer360ResponseSchemas[op.OperationID]) ||
					op.Responses.Value("400") == nil || op.Responses.Value("401") == nil || op.Responses.Value("403") == nil ||
					op.Responses.Value("503") == nil || (op.OperationID != "listCustomerMergeHistory" && op.Responses.Value("404") == nil) {
					return fmt.Errorf("%s safe local Customer 360 contract drifted", op.OperationID)
				}
			} else if p4ProductOperations[op.OperationID] {
				seenP4Product[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4ProductDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Product evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4ProductLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4ServicePeriodLifecycleOperations[op.OperationID] {
				seenP4ServicePeriodLifecycle[op.OperationID] = true
				read := op.OperationID == "listServicePeriodProducts" || op.OperationID == "getServicePeriodProduct"
				capability, csrf, source := "products.write", "required", "local_command"
				if read {
					capability, csrf, source = "products.read", "none", "local_read_model"
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4ServicePeriodLifecycleLegacyMappings[op.OperationID]) ||
					op.Extensions["x-p4-decision-evidence"] != p4ServicePeriodLifecycleEvidence ||
					op.Extensions["x-aicrm-capability"] != capability || op.Extensions["x-aicrm-auth-scheme"] != "human_session" ||
					op.Extensions["x-aicrm-session-bound-csrf"] != csrf || op.Extensions["x-aicrm-data-classification"] != "financial" ||
					op.Extensions["x-aicrm-data-source"] != source || op.Extensions["x-aicrm-external-effect"] != "none" ||
					op.Responses.Value("401") == nil || op.Responses.Value("403") == nil || op.Responses.Value("503") == nil {
					return fmt.Errorf("%s service-period lifecycle contract drifted", op.OperationID)
				}
			} else if p4ServicePeriodMemberGridReadOperations[op.OperationID] {
				seenP4ServicePeriodMemberGridRead[op.OperationID] = true
				capability := "products.read"
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4ServicePeriodMemberGridReadLegacyMappings[op.OperationID]) ||
					op.Extensions["x-p4-decision-evidence"] != p4ServicePeriodMemberGridReadEvidence ||
					op.Extensions["x-aicrm-capability"] != capability || op.Extensions["x-aicrm-auth-scheme"] != "human_session" ||
					op.Extensions["x-aicrm-session-bound-csrf"] != "none" || op.Extensions["x-aicrm-data-classification"] != "internal_pii" ||
					op.Extensions["x-aicrm-data-source"] != "local_read_model" || op.Extensions["x-aicrm-external-effect"] != "none" ||
					op.Responses.Value("401") == nil || op.Responses.Value("403") == nil || op.Responses.Value("404") == nil || op.Responses.Value("503") == nil {
					return fmt.Errorf("%s service-period member-grid read contract drifted", op.OperationID)
				}
			} else if p4MemberGridManagementOperations[op.OperationID] {
				seenP4MemberGridManagement[op.OperationID] = true
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4MemberGridManagementLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				wantCSRF, wantDataSource, wantCapability := "required", "local_command", "products.write"
				if op.OperationID == "getServicePeriodMemberGridShareSettings" {
					wantCSRF, wantDataSource, wantCapability = "none", "local_read_model", "products.read"
				}
				if op.Extensions["x-p4-decision-evidence"] != p4MemberGridManagementEvidence ||
					op.Extensions["x-aicrm-capability"] != wantCapability ||
					op.Extensions["x-aicrm-auth-scheme"] != "human_session" ||
					op.Extensions["x-aicrm-session-bound-csrf"] != wantCSRF ||
					op.Extensions["x-aicrm-data-classification"] != "internal_pii" ||
					op.Extensions["x-aicrm-data-source"] != wantDataSource ||
					op.Extensions["x-aicrm-external-effect"] != "none" ||
					op.Responses.Value("401") == nil || op.Responses.Value("403") == nil || op.Responses.Value("503") == nil {
					return fmt.Errorf("%s local member-grid boundary drifted", op.OperationID)
				}
			} else if p4RadarOperations[op.OperationID] {
				seenP4Radar[op.OperationID] = true
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4RadarLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				read := op.OperationID == "listRadarLinks" || op.OperationID == "getRadarLinkOptions" || op.OperationID == "getRadarLink" || op.OperationID == "getRadarLinkShareProjection"
				wantCapability, wantCSRF, wantSource := "operations.manage", "required", "local_command"
				if read {
					wantCapability, wantCSRF, wantSource = "admin.read", "none", "local_read_model"
				}
				if op.Extensions["x-p4-decision-evidence"] != p4RadarEvidence || op.Extensions["x-aicrm-capability"] != wantCapability ||
					op.Extensions["x-aicrm-auth-scheme"] != "human_session" || op.Extensions["x-aicrm-session-bound-csrf"] != wantCSRF ||
					op.Extensions["x-aicrm-data-classification"] != "internal" || op.Extensions["x-aicrm-data-source"] != wantSource ||
					op.Extensions["x-aicrm-external-effect"] != "none" || op.Responses.Value("401") == nil ||
					op.Responses.Value("403") == nil || op.Responses.Value("503") == nil {
					return fmt.Errorf("%s local Radar boundary drifted", op.OperationID)
				}
			} else if p4CloudCampaignOperations[op.OperationID] {
				seenP4CloudCampaign[op.OperationID] = true
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4CloudCampaignLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				read := op.OperationID == "listCloudCampaigns" || op.OperationID == "getCloudCampaign"
				wantCapability, wantCSRF, wantSource := "operations.manage", "required", "local_command"
				if read {
					wantCapability, wantCSRF, wantSource = "operations.read", "none", "local_read_model"
				}
				if op.Extensions["x-p4-decision-evidence"] != p4CloudCampaignEvidence || op.Extensions["x-aicrm-capability"] != wantCapability ||
					op.Extensions["x-aicrm-auth-scheme"] != "human_session" || op.Extensions["x-aicrm-session-bound-csrf"] != wantCSRF ||
					op.Extensions["x-aicrm-data-classification"] != "internal" || op.Extensions["x-aicrm-data-source"] != wantSource ||
					op.Extensions["x-aicrm-external-effect"] != "none" || op.Responses.Value("401") == nil ||
					op.Responses.Value("403") == nil || op.Responses.Value("503") == nil {
					return fmt.Errorf("%s local Cloud Campaign boundary drifted", op.OperationID)
				}
			} else if p4AIAudienceOperations[op.OperationID] {
				seenP4AIAudience[op.OperationID] = true
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4AIAudienceLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				read := op.OperationID == "listAIAudiencePackageGroups" || op.OperationID == "listAIAudiencePackages" || op.OperationID == "getAIAudiencePackage" || op.OperationID == "listAIAudiencePackageMembers"
				wantCapability, wantCSRF, wantSource := "segments.write", "required", "local_command"
				if read {
					wantCapability, wantCSRF = "segments.read", "none"
					wantSource = "local_read_model"
					if op.OperationID != "listAIAudiencePackageGroups" {
						wantSource = "segments.local_read_model"
					}
				}
				if op.Extensions["x-p4-decision-evidence"] != p4AIAudienceEvidence || op.Extensions["x-aicrm-capability"] != wantCapability ||
					op.Extensions["x-aicrm-auth-scheme"] != "human_session" || op.Extensions["x-aicrm-session-bound-csrf"] != wantCSRF ||
					op.Extensions["x-aicrm-data-classification"] != "internal" || op.Extensions["x-aicrm-data-source"] != wantSource ||
					op.Extensions["x-aicrm-external-effect"] != "none" || op.Responses.Value("401") == nil ||
					op.Responses.Value("403") == nil || op.Responses.Value("503") == nil {
					return fmt.Errorf("%s local AI Audience boundary drifted", op.OperationID)
				}
			} else if p4MediaOperations[op.OperationID] {
				seenP4Media[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4MediaEvidence[op.OperationID] {
					return fmt.Errorf("%s has missing or forged P4 Media evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4MediaLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4GroupInviteOperations[op.OperationID] {
				seenP4GroupInvite[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4GroupInviteDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Group Invite evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4GroupInviteLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4SurveyOperations[op.OperationID] {
				seenP4Survey[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4SurveyEvidence[op.OperationID] {
					return fmt.Errorf("%s has missing or forged P4 Survey evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4SurveyLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4ChannelOperations[op.OperationID] {
				seenP4Channel[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4ChannelEvidence[op.OperationID] {
					return fmt.Errorf("%s has missing or forged P4 Channel evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4ChannelLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4TagABOperations[op.OperationID] {
				seenP4TagAB[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4TagABDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Tag A+B evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4TagABLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4TagOperations[op.OperationID] {
				seenP4Tag[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4TagDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Tag evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4TagLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4CouponOperations[op.OperationID] {
				seenP4Coupon[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4CouponEvidence[op.OperationID] {
					return fmt.Errorf("%s has missing or forged P4 Coupon evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4CouponLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4OrderOperations[op.OperationID] {
				seenP4Order[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4OrderDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Order evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4OrderLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4CustomerCompatOperations[op.OperationID] {
				seenP4CustomerCompat[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4CustomerCompatDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Customer evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4CustomerCompatLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4ConfigSettingsOperations[op.OperationID] {
				seenP4ConfigSettings[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4ConfigSettingsEvidence[op.OperationID] {
					return fmt.Errorf("%s has missing or forged P4 Config Settings evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4ConfigSettingsLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if contract, ok := p4AdminOpsSafeOperations[op.OperationID]; ok {
				seenP4AdminOpsSafe[op.OperationID] = true
				if err := validateP4AdminOpsSafeOperation(path, item, op, contract, known); err != nil {
					return err
				}
			} else if p4SetupWizardOperations[op.OperationID] {
				seenP4SetupWizard[op.OperationID] = true
				read := op.OperationID == "getSetupWizard"
				capability, csrf, source := "config.settings.manage", "required", "local_command"
				if read {
					capability, csrf, source = "config.overview.read", "none", "local_read_model"
				}
				if op.Extensions["x-p4-decision-evidence"] != "P4-SETUP-WIZARD-LOCAL-2026-08-23" ||
					op.Extensions["x-aicrm-capability"] != capability || op.Extensions["x-aicrm-auth-scheme"] != "human_session" ||
					op.Extensions["x-aicrm-session-bound-csrf"] != csrf || op.Extensions["x-aicrm-data-classification"] != "internal" ||
					op.Extensions["x-aicrm-data-source"] != source || op.Extensions["x-aicrm-external-effect"] != "none" ||
					op.Responses.Value("401") == nil || op.Responses.Value("403") == nil || op.Responses.Value("503") == nil {
					return fmt.Errorf("%s local setup-wizard contract drifted", op.OperationID)
				}
				if _, linked := op.Extensions["x-legacy-mapping-ids"]; linked {
					return fmt.Errorf("%s must not replace the frozen legacy HTML carrier", op.OperationID)
				}
			} else if p4PushCenterOperations[op.OperationID] {
				seenP4PushCenter[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4PushCenterDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Push Center evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4PushCenterLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				if op.Extensions["x-aicrm-session-bound-csrf"] != "none" || op.Extensions["x-aicrm-data-classification"] != "internal_pii" || op.Extensions["x-aicrm-external-effect"] != "none" {
					return fmt.Errorf("%s Push Center read contract drifted", op.OperationID)
				}
			} else if p4ExecutionRuntimeOperations[op.OperationID] {
				seenP4ExecutionRuntime[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4ExecutionRuntimeDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Execution Runtime evidence", op.OperationID)
				}
				if op.OperationID == "getLegacyExecutionRuntimePage" {
					if _, linked := op.Extensions["x-legacy-mapping-ids"]; linked {
						return fmt.Errorf("%s must not duplicate the LEGACY-API-0314 API mapping", op.OperationID)
					}
				} else {
					ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
					if linkErr != nil || !reflect.DeepEqual(ids, p4ExecutionRuntimeLegacyMappings[op.OperationID]) {
						return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
					}
				}
				if op.Extensions["x-aicrm-capability"] != "admin.read" || op.Extensions["x-aicrm-auth-scheme"] != "human_session" || op.Extensions["x-aicrm-session-bound-csrf"] != "none" || op.Extensions["x-aicrm-data-classification"] != "internal_pii" || op.Extensions["x-aicrm-external-effect"] != "none" {
					return fmt.Errorf("%s Execution Runtime authorization or observation contract drifted", op.OperationID)
				}
				scopes, scopeErr := stringMap(op.Extensions["x-aicrm-rbac-scopes"])
				if scopeErr != nil || !reflect.DeepEqual(scopes, map[string]string{"admin": "global"}) || op.Responses.Value("403") == nil {
					return fmt.Errorf("%s Execution Runtime must stay admin/global and fail closed", op.OperationID)
				}
			} else if p4AdminShellOperations[op.OperationID] {
				seenP4AdminShell[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4AdminShellDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Admin Shell evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4AdminShellLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				if op.Extensions["x-aicrm-auth-scheme"] != "human_session" || op.Extensions["x-aicrm-session-bound-csrf"] != "none" || op.Extensions["x-aicrm-data-source"] != "static" || op.Extensions["x-aicrm-external-effect"] != "none" {
					return fmt.Errorf("%s Admin Shell security or effect contract drifted", op.OperationID)
				}
			} else if p4DomainVerificationOperations[op.OperationID] {
				seenP4DomainVerification[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4DomainVerificationDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 domain verification evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, []string{"LEGACY-API-0781"}) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				if op.Security == nil || len(*op.Security) != 0 || op.Extensions["x-aicrm-auth-scheme"] != "public" || op.Extensions["x-aicrm-csrf"] != "none" || op.Extensions["x-aicrm-data-source"] != "static" || op.Extensions["x-aicrm-external-effect"] != "none" {
					return fmt.Errorf("%s public static contract drifted", op.OperationID)
				}
				parameter := op.Parameters.GetByInAndName("path", "filename")
				if parameter == nil || !parameter.Required || parameter.Schema == nil || parameter.Schema.Value == nil || parameter.Schema.Value.Pattern != "^(?:WW|MP)_verify_[A-Za-z0-9_-]+\\.txt$" {
					return fmt.Errorf("%s filename contract drifted", op.OperationID)
				}
				response := op.Responses.Value("200")
				if response == nil || response.Value == nil || response.Value.Headers["Cache-Control"] == nil || response.Value.Content["text/plain"] == nil {
					return fmt.Errorf("%s plaintext no-store response drifted", op.OperationID)
				}
			} else if p4LegacyHealthOperations[op.OperationID] {
				seenP4LegacyHealth[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4LegacyHealthDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 legacy health evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, []string{"LEGACY-API-0757"}) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				// The immutable LEGACY-API-0757 capability keeps its exact
				// underscored manifest name; the generic dotted-capability
				// fallback deliberately does not apply to this operation.
				if op.Security == nil || len(*op.Security) != 0 ||
					op.Extensions["x-aicrm-capability"] != "health_read" ||
					op.Extensions["x-aicrm-auth-scheme"] != "public" ||
					op.Extensions["x-aicrm-csrf"] != "none" ||
					op.Extensions["x-aicrm-data-classification"] != "public_non_pii" ||
					op.Extensions["x-aicrm-data-source"] != "read_model" ||
					op.Extensions["x-aicrm-rate-limit"] != "health" ||
					op.Extensions["x-aicrm-external-effect"] != "none" {
					return fmt.Errorf("%s public runtime-mode snapshot contract drifted", op.OperationID)
				}
				if _, scopesDeclared := op.Extensions["x-aicrm-rbac-scopes"]; scopesDeclared {
					return fmt.Errorf("%s must not declare RBAC scopes", op.OperationID)
				}
				if op.RequestBody != nil || len(op.Parameters) != 0 {
					return fmt.Errorf("%s must remain a parameterless read", op.OperationID)
				}
				methodNotAllowed := op.Responses.Value("405")
				if !operationResponseUsesLocalSchema(op, "LegacyRuntimeHealthSnapshot") ||
					methodNotAllowed == nil || methodNotAllowed.Value == nil ||
					methodNotAllowed.Value.Headers["Allow"] == nil ||
					!operationResponseUsesStatusLocalSchema(op, "405", "LegacyHealthMethodNotAllowed") ||
					len(op.Responses.Map()) != 2 {
					return fmt.Errorf("%s response envelope drifted", op.OperationID)
				}
			} else if pe01Contract, pe01 := pe01Operations[op.OperationID]; pe01 {
				if err := validatePE01Operation(path, item, op, pe01Contract); err != nil {
					return err
				}
			} else if nativeContract, native := nativePackageOperations[op.OperationID]; native {
				if err := validateNativePackageOperation(path, item, op, nativeContract); err != nil {
					return err
				}
				if op.OperationID == "cancelLegacyOutboundJob" {
					if err := validateOutboundCancelOperation(op); err != nil {
						return err
					}
				}
			} else if canonicalFallback {
				if err := validateGenericCanonicalAuthorization(op, canonicalContract.Method); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("unexpected candidate operation branch: %s", op.OperationID)
			}
			if contactOperations[op.OperationID] {
				evidence, ok := op.Extensions["x-p3-decision-evidence"].(string)
				if !ok || evidence != p3ContactDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P3 contact evidence", op.OperationID)
				}
			}
			if identityOperations[op.OperationID] {
				evidence, ok := op.Extensions["x-p3-decision-evidence"].(string)
				if !ok || evidence != p3IdentityDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P3 identity evidence", op.OperationID)
				}
			}
			if segmentOperations[op.OperationID] {
				evidence, ok := op.Extensions["x-p3-decision-evidence"].(string)
				if !ok || evidence != p3SegmentDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P3 segment evidence", op.OperationID)
				}
			}
			if p4DomainVerificationOperations[op.OperationID] || p4LegacyHealthOperations[op.OperationID] || nativePackageOperationDeclared(op.OperationID) || pe01OperationDeclared(op.OperationID) {
				// The public static route and the public runtime-mode snapshot are
				// fully constrained in their dedicated branches above. Native
				// package operations are likewise validated against their exact
				// owner-approved registry rather than the legacy authorization map.
			} else if public, publicOperation := couponPublicAccessContracts[op.OperationID]; publicOperation {
				authScheme, ok := op.Extensions["x-aicrm-auth-scheme"].(string)
				if !ok || authScheme != public.authScheme {
					return fmt.Errorf("%s public auth scheme=%q", op.OperationID, authScheme)
				}
				if public.accessScope != "" {
					accessScope, ok := op.Extensions["x-aicrm-access-scope"].(string)
					if !ok || accessScope != public.accessScope {
						return fmt.Errorf("%s public access scope=%q", op.OperationID, accessScope)
					}
				}
			} else {
				contract, found := authorizationContracts[op.OperationID]
				if !found {
					if canonicalFallback {
						continue
					}
					return fmt.Errorf("%s lacks authorization contract", op.OperationID)
				}
				capability, ok := op.Extensions["x-aicrm-capability"].(string)
				if !ok || capability != contract.capability {
					return fmt.Errorf("%s capability=%q", op.OperationID, capability)
				}
				scopes, err := stringMap(op.Extensions["x-aicrm-rbac-scopes"])
				if err != nil || !reflect.DeepEqual(scopes, contract.scopes) {
					return fmt.Errorf("%s RBAC scopes=%v", op.OperationID, scopes)
				}
				if len(contract.scopes) < 3 && op.Responses.Value("403") == nil {
					return fmt.Errorf("%s denies a role but lacks 403", op.OperationID)
				}
			}
			if op.OperationID == "listTags" && op.Responses.Value("503") == nil {
				return fmt.Errorf("listTags lacks dependency unavailable response")
			}
		}
	}
	if len(seenP1) != len(p1CandidateOperations) || len(seenP2) != len(p2StageOperations) ||
		len(seenP3Contact) != len(p3ContactOperations) || len(seenP3Identity) != len(p3IdentityOperations) || len(seenP3Segment) != len(p3SegmentOperations) ||
		len(seenP4Automation) != len(p4AutomationOperations) || len(seenP4HXCSenderManagement) != len(p4HXCSenderManagementOperations) || len(seenP4AutomationAgent) != len(p4AutomationAgentOperations) || len(seenP4AutomationAgentManagement) != len(p4AutomationAgentManagementOperations) || len(seenP4Customer360) != len(p4Customer360Operations) || len(seenP4Product) != len(p4ProductOperations) || len(seenP4ServicePeriodLifecycle) != len(p4ServicePeriodLifecycleOperations) || len(seenP4ServicePeriodMemberGridRead) != len(p4ServicePeriodMemberGridReadOperations) || len(seenP4MemberGridManagement) != len(p4MemberGridManagementOperations) || len(seenP4Radar) != len(p4RadarOperations) || len(seenP4CloudCampaign) != len(p4CloudCampaignOperations) || len(seenP4AIAudience) != len(p4AIAudienceOperations) || len(seenP4Media) != len(p4MediaOperations) ||
		len(seenP4GroupInvite) != len(p4GroupInviteOperations) || len(seenP4Survey) != len(p4SurveyOperations) || len(seenP4Channel) != len(p4ChannelOperations) ||
		len(seenP4Tag) != len(p4TagOperations) || len(seenP4TagAB) != len(p4TagABOperations) || len(seenP4Coupon) != len(p4CouponOperations) ||
		len(seenP4Order) != len(p4OrderOperations) || len(seenP4CustomerCompat) != len(p4CustomerCompatOperations) ||
		len(seenP4ConfigSettings) != len(p4ConfigSettingsOperations) || len(seenP4AdminOpsSafe) != len(p4AdminOpsSafeOperations) || len(seenP4SetupWizard) != len(p4SetupWizardOperations) || len(seenP4DomainVerification) != len(p4DomainVerificationOperations) ||
		len(seenP4PushCenter) != len(p4PushCenterOperations) || len(seenP4ExecutionRuntime) != len(p4ExecutionRuntimeOperations) ||
		len(seenP4AdminShell) != len(p4AdminShellOperations) || len(seenP4LegacyHealth) != len(p4LegacyHealthOperations) || len(seenCanonical) != len(inventory.Candidates) {
		return errors.New("candidate inventory differs from canonical declarations")
	}
	for id := range p1CandidateOperations {
		if !seenP1[id] {
			return fmt.Errorf("missing candidate operation: %s", id)
		}
	}
	for id := range p2StageOperations {
		if !seenP2[id] {
			return fmt.Errorf("missing P2 stage operation: %s", id)
		}
	}
	for id := range p3ContactOperations {
		if !seenP3Contact[id] {
			return fmt.Errorf("missing P3 contact operation: %s", id)
		}
	}
	for id := range p3IdentityOperations {
		if !seenP3Identity[id] {
			return fmt.Errorf("missing P3 identity operation: %s", id)
		}
	}
	for id := range p3SegmentOperations {
		if !seenP3Segment[id] {
			return fmt.Errorf("missing P3 segment operation: %s", id)
		}
	}
	for id := range p4AutomationOperations {
		if !seenP4Automation[id] {
			return fmt.Errorf("missing P4 Automation operation: %s", id)
		}
	}
	for id := range p4AutomationAgentOperations {
		if !seenP4AutomationAgent[id] {
			return fmt.Errorf("missing P4 Automation Agent operation: %s", id)
		}
	}
	for id := range p4AutomationAgentManagementOperations {
		if !seenP4AutomationAgentManagement[id] {
			return fmt.Errorf("missing P4 Automation Agent management operation: %s", id)
		}
	}
	for id := range p4Customer360Operations {
		if !seenP4Customer360[id] {
			return fmt.Errorf("missing P4 Customer 360 operation: %s", id)
		}
	}
	for id := range p4ProductOperations {
		if !seenP4Product[id] {
			return fmt.Errorf("missing P4 Product operation: %s", id)
		}
	}
	for id := range p4ServicePeriodLifecycleOperations {
		if !seenP4ServicePeriodLifecycle[id] {
			return fmt.Errorf("missing P4 service-period lifecycle operation: %s", id)
		}
	}
	for id := range p4ServicePeriodMemberGridReadOperations {
		if !seenP4ServicePeriodMemberGridRead[id] {
			return fmt.Errorf("missing P4 service-period member-grid read operation: %s", id)
		}
	}
	for id := range p4MemberGridManagementOperations {
		if !seenP4MemberGridManagement[id] {
			return fmt.Errorf("missing P4 member-grid management operation: %s", id)
		}
	}
	for id := range p4RadarOperations {
		if !seenP4Radar[id] {
			return fmt.Errorf("missing P4 Radar operation: %s", id)
		}
	}
	for id := range p4CloudCampaignOperations {
		if !seenP4CloudCampaign[id] {
			return fmt.Errorf("missing P4 Cloud Campaign operation: %s", id)
		}
	}
	for id := range p4AIAudienceOperations {
		if !seenP4AIAudience[id] {
			return fmt.Errorf("missing P4 AI Audience operation: %s", id)
		}
	}
	for id := range p4MediaOperations {
		if !seenP4Media[id] {
			return fmt.Errorf("missing P4 Media operation: %s", id)
		}
	}
	for id := range p4GroupInviteOperations {
		if !seenP4GroupInvite[id] {
			return fmt.Errorf("missing P4 Group Invite operation: %s", id)
		}
	}
	for id := range p4SurveyOperations {
		if !seenP4Survey[id] {
			return fmt.Errorf("missing P4 Survey operation: %s", id)
		}
	}
	for id := range p4ChannelOperations {
		if !seenP4Channel[id] {
			return fmt.Errorf("missing P4 Channel operation: %s", id)
		}
	}
	for id := range p4TagOperations {
		if !seenP4Tag[id] {
			return fmt.Errorf("missing P4 Tag operation: %s", id)
		}
	}
	for id := range p4TagABOperations {
		if !seenP4TagAB[id] {
			return fmt.Errorf("missing P4 Tag A+B operation: %s", id)
		}
	}
	for id := range p4CouponOperations {
		if !seenP4Coupon[id] {
			return fmt.Errorf("missing P4 Coupon operation: %s", id)
		}
	}
	for id := range p4OrderOperations {
		if !seenP4Order[id] {
			return fmt.Errorf("missing P4 Order operation: %s", id)
		}
	}
	for id := range p4CustomerCompatOperations {
		if !seenP4CustomerCompat[id] {
			return fmt.Errorf("missing P4 Customer compatibility operation: %s", id)
		}
	}
	for id := range p4ConfigSettingsOperations {
		if !seenP4ConfigSettings[id] {
			return fmt.Errorf("missing P4 Config Settings compatibility operation: %s", id)
		}
	}
	for id := range p4AdminOpsSafeOperations {
		if !seenP4AdminOpsSafe[id] {
			return fmt.Errorf("missing P4 AdminOps safe projection operation: %s", id)
		}
	}
	for id := range p4SetupWizardOperations {
		if !seenP4SetupWizard[id] {
			return fmt.Errorf("missing P4 setup-wizard operation: %s", id)
		}
	}
	for id := range p4DomainVerificationOperations {
		if !seenP4DomainVerification[id] {
			return fmt.Errorf("missing P4 domain verification operation: %s", id)
		}
	}
	for id := range p4PushCenterOperations {
		if !seenP4PushCenter[id] {
			return fmt.Errorf("missing P4 Push Center operation: %s", id)
		}
	}
	for id := range p4ExecutionRuntimeOperations {
		if !seenP4ExecutionRuntime[id] {
			return fmt.Errorf("missing P4 Execution Runtime operation: %s", id)
		}
	}
	for id := range p4AdminShellOperations {
		if !seenP4AdminShell[id] {
			return fmt.Errorf("missing P4 Admin Shell operation: %s", id)
		}
	}
	for id := range p4LegacyHealthOperations {
		if !seenP4LegacyHealth[id] {
			return fmt.Errorf("missing P4 legacy health operation: %s", id)
		}
	}
	for id := range inventory.Candidates {
		if !seenCanonical[id] {
			return fmt.Errorf("missing canonical candidate operation: %s", id)
		}
	}
	customer := doc.Components.Schemas["Customer"]
	if customer == nil || customer.Value == nil {
		return errors.New("Customer schema missing")
	}
	for _, name := range []string{"external_userid", "unionid", "openid", "phone", "mobile"} {
		if _, ok := customer.Value.Properties[name]; ok {
			return fmt.Errorf("Customer contains external identity: %s", name)
		}
	}
	identity := doc.Components.Schemas["IdentityRef"]
	if identity == nil || identity.Value == nil {
		return errors.New("IdentityRef schema missing")
	}
	required := append([]string(nil), identity.Value.Required...)
	sort.Strings(required)
	want := []string{"scope", "type", "value"}
	if fmt.Sprint(required) != fmt.Sprint(want) {
		return fmt.Errorf("IdentityRef required fields=%v", required)
	}
	if len(identity.Value.Properties) != 3 || identity.Value.AdditionalProperties.Has == nil ||
		*identity.Value.AdditionalProperties.Has {
		return errors.New("IdentityRef must be a closed type/scope/value admin shape")
	}
	kinds, err := stringList(identity.Value.Properties["type"].Value.Enum)
	if err != nil {
		return errors.New("IdentityRef type enum is invalid")
	}
	sort.Strings(kinds)
	wantKinds := []string{"alipay_user_id", "ext", "mp_openid", "oa_openid", "phone", "unionid", "wecom_external_userid"}
	if !reflect.DeepEqual(kinds, wantKinds) {
		return fmt.Errorf("IdentityRef type enum=%v", kinds)
	}
	if doc.Components.Schemas["ErrorResponse"] == nil {
		return errors.New("ErrorResponse schema missing")
	}
	if err := validateBrowserSessionContract(doc); err != nil {
		return err
	}
	if err := validateStageContract(doc); err != nil {
		return err
	}
	if err := validateContactContract(doc); err != nil {
		return err
	}
	if err := validateIdentityContract(doc); err != nil {
		return err
	}
	if err := validateSegmentContract(doc); err != nil {
		return err
	}
	if err := validateAutomationContract(doc); err != nil {
		return err
	}
	if err := validateGroupInviteContract(doc); err != nil {
		return err
	}
	if err := validateSurveyContract(doc); err != nil {
		return err
	}
	if err := validateChannelContract(doc); err != nil {
		return err
	}
	if err := validateTagABContract(doc); err != nil {
		return err
	}
	if err := validateCouponContract(doc); err != nil {
		return err
	}
	if err := validateCustomerCompatContract(doc); err != nil {
		return err
	}
	if err := validateOutboundSafeProjectionContract(doc); err != nil {
		return err
	}
	if err := validateOutboundCampaignHandoffContract(doc); err != nil {
		return err
	}
	if err := validateInternalEventRegistryContract(doc); err != nil {
		return err
	}
	if err := validateConfigSettingsContract(doc); err != nil {
		return err
	}
	if err := validateRadarShareContract(doc); err != nil {
		return err
	}
	if err := validateAdminShellContract(doc); err != nil {
		return err
	}
	if err := validateLegacyHealthContract(doc); err != nil {
		return err
	}
	return nil
}

func validateRadarShareContract(doc *openapi3.T) error {
	operation := doc.Paths.Value("/api/admin/radar-links/{link_id}/share")
	schema := doc.Components.Schemas["RadarShareProjection"]
	if operation == nil || operation.Get == nil || !operationResponseUsesLocalSchema(operation.Get, "RadarShareProjection") ||
		schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has ||
		len(schema.Value.Properties) != 9 {
		return errors.New("Radar share projection must remain a closed local-only descriptor")
	}
	required := append([]string(nil), schema.Value.Required...)
	sort.Strings(required)
	if !reflect.DeepEqual(required, []string{"available", "link_id", "local_projection", "public_code", "public_route_ready", "qr_payload", "real_external_call_executed", "share_path", "status"}) ||
		!legacyTagBooleanEnum(schema.Value, "available", false) ||
		!legacyTagBooleanEnum(schema.Value, "public_route_ready", false) ||
		!legacyTagBooleanEnum(schema.Value, "real_external_call_executed", false) ||
		!legacyTagStringEnum(schema.Value, "share_path", "") ||
		!legacyTagStringEnum(schema.Value, "qr_payload", "") {
		return errors.New("Radar share projection must not expose an unavailable public route")
	}
	for path := range doc.Paths.Map() {
		if strings.HasPrefix(path, "/r/") {
			return errors.New("Radar public route is not available")
		}
	}
	return nil
}

// validateLegacyHealthContract freezes the exact 15-field LEGACY-API-0757
// snapshot and the legacy 405 detail payload at immutable legacy SHA 6cb989c.
func validateLegacyHealthContract(doc *openapi3.T) error {
	snapshot := doc.Components.Schemas["LegacyRuntimeHealthSnapshot"]
	if snapshot == nil || snapshot.Value == nil || snapshot.Value.AdditionalProperties.Has == nil ||
		*snapshot.Value.AdditionalProperties.Has || len(snapshot.Value.Properties) != 15 {
		return errors.New("LegacyRuntimeHealthSnapshot must remain a closed 15-field snapshot")
	}
	required := append([]string(nil), snapshot.Value.Required...)
	sort.Strings(required)
	want := []string{"database", "database_mode", "fixture_mode", "legacy_runtime_enabled", "ok",
		"production_data_mode", "production_data_ready", "repository_policy", "runtime_owner",
		"secret_key_present", "service", "status", "warning",
		"wechat_shop_callback_token_present", "wechat_shop_callback_token_required"}
	if !reflect.DeepEqual(required, want) {
		return fmt.Errorf("LegacyRuntimeHealthSnapshot required=%v", required)
	}
	stringEnums := map[string][]any{
		"status":            {"ok", "degraded"},
		"service":           {"aicrm-next"},
		"database":          {"postgres", "fixture"},
		"database_mode":     {"postgres", "fixture"},
		"repository_policy": {"production_repositories_required", "fixture_repositories_allowed"},
		"runtime_owner":     {"ai_crm_next"},
		"warning":           {"", "fixture data mode", "production runtime is using fixture data; production data is not ready"},
	}
	for name, wantEnum := range stringEnums {
		property := snapshot.Value.Properties[name]
		if property == nil || property.Value == nil || !reflect.DeepEqual(property.Value.Enum, wantEnum) {
			return fmt.Errorf("LegacyRuntimeHealthSnapshot %s enum drifted", name)
		}
	}
	legacyEnabled := snapshot.Value.Properties["legacy_runtime_enabled"]
	if legacyEnabled == nil || legacyEnabled.Value == nil || !reflect.DeepEqual(legacyEnabled.Value.Enum, []any{false}) {
		return errors.New("LegacyRuntimeHealthSnapshot legacy_runtime_enabled must remain fixed false")
	}
	methodNotAllowed := doc.Components.Schemas["LegacyHealthMethodNotAllowed"]
	if methodNotAllowed == nil || methodNotAllowed.Value == nil || methodNotAllowed.Value.AdditionalProperties.Has == nil ||
		*methodNotAllowed.Value.AdditionalProperties.Has || len(methodNotAllowed.Value.Properties) != 1 ||
		!reflect.DeepEqual(methodNotAllowed.Value.Required, []string{"detail"}) {
		return errors.New("LegacyHealthMethodNotAllowed must remain a closed detail-only payload")
	}
	detail := methodNotAllowed.Value.Properties["detail"]
	if detail == nil || detail.Value == nil || !reflect.DeepEqual(detail.Value.Enum, []any{"Method Not Allowed"}) {
		return errors.New("LegacyHealthMethodNotAllowed detail must remain the exact legacy message")
	}
	return nil
}

func validateAdminShellContract(doc *openapi3.T) error {
	page := doc.Paths.Value("/admin")
	logout := doc.Paths.Value("/admin/logout")
	if page == nil || page.Get == nil || logout == nil || logout.Get == nil {
		return errors.New("P4 Admin Shell compatibility operations are incomplete")
	}
	if page.Get.Responses.Value("200") == nil || page.Get.Responses.Value("200").Value.Content["text/html"] == nil ||
		page.Get.Responses.Value("302") == nil || page.Get.Responses.Value("403") == nil || page.Get.Responses.Value("503") == nil ||
		logout.Get.Responses.Value("302") == nil || logout.Get.Responses.Value("403") == nil || logout.Get.Responses.Value("503") == nil {
		return errors.New("P4 Admin Shell response boundaries are incomplete")
	}
	for name, operation := range map[string]*openapi3.Operation{"page": page.Get, "logout": logout.Get} {
		if operation.Security != nil || operation.RequestBody != nil || operation.Responses.Value("302").Value.Headers["Location"] == nil {
			return fmt.Errorf("P4 Admin Shell %s transport contract drifted", name)
		}
	}
	denied := doc.Components.Schemas["AdminShellAccessDenied"]
	denialPropertyEnum := func(name string, want []any) bool {
		if denied == nil || denied.Value == nil {
			return false
		}
		property := denied.Value.Properties[name]
		return property != nil && property.Value != nil && reflect.DeepEqual(property.Value.Enum, want)
	}
	if denied == nil || denied.Value == nil || denied.Value.AdditionalProperties.Has == nil || *denied.Value.AdditionalProperties.Has ||
		!reflect.DeepEqual(denied.Value.Required, []string{"ok", "error", "required_capability", "route_owner", "real_external_call_executed"}) ||
		!denialPropertyEnum("ok", []any{false}) ||
		!denialPropertyEnum("error", []any{"admin_capability_required", "principal_type_forbidden"}) ||
		!denialPropertyEnum("required_capability", []any{"admin_read"}) ||
		!denialPropertyEnum("route_owner", []any{"ai_crm_next"}) ||
		!denialPropertyEnum("real_external_call_executed", []any{false}) {
		return errors.New("P4 Admin Shell denial payload must remain fail-closed")
	}
	return nil
}

func validateTagABContract(doc *openapi3.T) error {
	shell := doc.Paths.Value("/admin/wecom-tags")
	groups := doc.Paths.Value("/api/admin/wecom/tag-groups")
	group := doc.Paths.Value("/api/admin/wecom/tag-groups/{group_id}")
	tag := doc.Paths.Value("/api/admin/wecom/tags/{tag_id}")
	gate := doc.Paths.Value("/api/admin/wecom/tags/live/gate")
	mark := doc.Paths.Value("/api/admin/wecom/tags/live/mark")
	unmark := doc.Paths.Value("/api/admin/wecom/tags/live/unmark")
	sync := doc.Paths.Value("/api/admin/wecom/tags/sync")
	syncDue := doc.Paths.Value("/api/admin/wecom/tags/sync-due")
	if shell == nil || shell.Get == nil || shell.Get.Responses.Value("302") == nil || shell.Get.Responses.Value("200") != nil ||
		groups == nil || groups.Get == nil || group == nil || group.Get == nil || tag == nil || tag.Get == nil ||
		gate == nil || gate.Get == nil || mark == nil || mark.Post == nil || unmark == nil || unmark.Post == nil ||
		sync == nil || sync.Post == nil || syncDue == nil || syncDue.Post == nil {
		return errors.New("P4-B02AB Tag A+B compatibility operations are incomplete")
	}
	for _, contract := range []struct {
		operation *openapi3.Operation
		schema    string
	}{
		{groups.Get, "LegacyTagGroupsResponse"}, {group.Get, "LegacyTagGroupResponse"},
		{tag.Get, "LegacyTagResponse"}, {gate.Get, "LegacyTagExecutionGate"},
	} {
		if !operationResponseUsesLocalSchema(contract.operation, contract.schema) {
			return fmt.Errorf("%s response schema ref drifted", contract.operation.OperationID)
		}
	}
	for name, operation := range map[string]*openapi3.Operation{
		"queueLegacyWecomTagLiveMark": mark.Post, "queueLegacyWecomTagLiveUnmark": unmark.Post,
		"queueLegacyWecomTagSync": sync.Post, "queueLegacyWecomTagSyncDue": syncDue.Post,
	} {
		if !operationResponseUsesStatusLocalSchema(operation, "202", "LegacyTagQueuedAcceptance") {
			return fmt.Errorf("%s must remain accepted-only local queueing", name)
		}
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !hasRequiredHeader(operation, "Idempotency-Key") || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks replay, conflict, or unavailable contract", name)
		}
	}
	for name, operation := range map[string]*openapi3.Operation{
		"queueLegacyWecomTagLiveMark": mark.Post, "queueLegacyWecomTagLiveUnmark": unmark.Post,
	} {
		if !operationRequestUsesLocalSchema(operation, "LegacyTagOpaqueLiveMutationRequest") {
			return fmt.Errorf("%s opaque legacy request drifted", name)
		}
	}
	for name, operation := range map[string]*openapi3.Operation{
		"queueLegacyWecomTagSync": sync.Post, "queueLegacyWecomTagSyncDue": syncDue.Post,
	} {
		if !operationRequestUsesLocalSchema(operation, "LegacyTagSyncRequest") {
			return fmt.Errorf("%s sync request drifted", name)
		}
	}
	acceptance := doc.Components.Schemas["LegacyTagQueuedAcceptance"]
	gateSchema := doc.Components.Schemas["LegacyTagExecutionGate"]
	if acceptance == nil || acceptance.Value == nil || gateSchema == nil || gateSchema.Value == nil ||
		!legacyTagBooleanEnum(acceptance.Value, "accepted", true) ||
		!legacyTagBooleanEnum(acceptance.Value, "real_external_call_executed", false) ||
		!legacyTagBooleanEnum(acceptance.Value, "sync_executed", false) ||
		!legacyTagStringEnum(acceptance.Value, "state", "queued") ||
		gateSchema.Value.AdditionalProperties.Has == nil || *gateSchema.Value.AdditionalProperties.Has ||
		!reflect.DeepEqual(gateSchema.Value.Required, []string{"provider_execution_eligible", "local_command_acceptance_available", "local_queue_available", "sync_executed", "observed_at", "real_external_call_executed"}) ||
		!legacyTagBooleanEnum(gateSchema.Value, "provider_execution_eligible", false) ||
		!legacyTagBooleanEnum(gateSchema.Value, "local_command_acceptance_available", true) ||
		!legacyTagBooleanEnum(gateSchema.Value, "local_queue_available", true) ||
		!legacyTagBooleanEnum(gateSchema.Value, "real_external_call_executed", false) ||
		!legacyTagBooleanEnum(gateSchema.Value, "sync_executed", false) ||
		gateSchema.Value.Properties["observed_at"] == nil || gateSchema.Value.Properties["observed_at"].Value == nil || gateSchema.Value.Properties["observed_at"].Value.Format != "date-time" {
		return errors.New("P4-B02AB Tag A+B execution state must remain fail-closed")
	}
	return nil
}

func legacyTagBooleanEnum(schema *openapi3.Schema, name string, want bool) bool {
	property := schema.Properties[name]
	return property != nil && property.Value != nil && reflect.DeepEqual(property.Value.Enum, []any{want})
}

func legacyTagStringEnum(schema *openapi3.Schema, name, want string) bool {
	property := schema.Properties[name]
	return property != nil && property.Value != nil && reflect.DeepEqual(property.Value.Enum, []any{want})
}

func validateConfigSettingsContract(doc *openapi3.T) error {
	page := doc.Paths.Value("/admin/config/app-settings")
	save := doc.Paths.Value("/admin/config/app-settings/save")
	resource := doc.Paths.Value("/api/admin/config/app-settings")
	if page == nil || page.Get == nil || save == nil || save.Post == nil || resource == nil || resource.Get == nil || resource.Put == nil {
		return errors.New("P4-A02 Config Settings compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(resource.Get, "LegacyAppSettingsResponse") || !operationFormUsesLocalSchema(save.Post, "LegacyAppSettingsSaveForm") ||
		!operationRequestUsesLocalSchema(resource.Put, "LegacyAppSettingsResourceSaveRequest") || !operationResponseUsesLocalSchema(resource.Put, "LegacyAppSettingsResourceSaveResponse") {
		return errors.New("P4-A02 Config Settings form or resource schema drifted")
	}
	form := doc.Components.Schemas["LegacyAppSettingsSaveForm"]
	masked := doc.Components.Schemas["LegacyMaskedAppSetting"]
	editable := doc.Components.Schemas["LegacyEditableAppSetting"]
	if form == nil || form.Value == nil || form.Value.AdditionalProperties.Has == nil || *form.Value.AdditionalProperties.Has || len(form.Value.Properties) != 16 {
		return errors.New("LegacyAppSettingsSaveForm must remain closed with four transport fields plus twelve settings")
	}
	if masked == nil || masked.Value == nil || masked.Value.AdditionalProperties.Has == nil || *masked.Value.AdditionalProperties.Has || len(masked.Value.Properties) != 7 {
		return errors.New("LegacyMaskedAppSetting must remain boolean-only")
	}
	if editable == nil || editable.Value == nil || editable.Value.Properties["version"] == nil || editable.Value.Properties["version"].Value == nil || len(editable.Value.Properties["version"].Value.Enum) != 1 || editable.Value.Properties["version"].Value.Enum[0] != "" {
		return errors.New("LegacyEditableAppSetting version must remain fixed empty")
	}
	for _, key := range []string{"database.url", "wecom.secret", "wecom.callback_token", "wecom.callback_aes_key", "ai.api_key", "auth.jwt_secret", "extension.api_key_pepper", "gateway.webhook_master_key"} {
		property := form.Value.Properties["setting__"+key]
		if property == nil || property.Value == nil || property.Value.MaxLength == nil || *property.Value.MaxLength != 0 {
			return fmt.Errorf("secret form key %s must only accept blank preservation", key)
		}
	}
	if save.Post.Responses.Value("302") == nil || save.Post.Responses.Value("200") != nil {
		return errors.New("P4-A02 save redirect contract drifted")
	}
	if value, ok := save.Post.Extensions["x-aicrm-session-bound-csrf"].(string); !ok || value != "required" {
		return errors.New("P4-A02 session-bound CSRF requirement drifted")
	}
	if value, ok := resource.Put.Extensions["x-aicrm-session-bound-csrf"].(string); !ok || value != "required" {
		return errors.New("P4 Admin Config JSON write lost session-bound CSRF")
	}
	if value, ok := resource.Put.Extensions["x-aicrm-route-bound-action-token"].(string); !ok || value != "required" {
		return errors.New("P4 Admin Config JSON write lost exact action-token requirement")
	}
	if resource.Put.Responses.Value("400") == nil || resource.Put.Responses.Value("409") == nil || resource.Put.Responses.Value("503") == nil {
		return errors.New("P4 Admin Config JSON write lost finite error responses")
	}
	wizard := doc.Paths.Value("/api/admin/setup-wizard")
	if wizard == nil || wizard.Get == nil || wizard.Post == nil || !operationResponseUsesLocalSchema(wizard.Get, "SetupWizardReadResponse") ||
		!operationRequestUsesLocalSchema(wizard.Post, "SetupWizardSaveRequest") || !operationResponseUsesLocalSchema(wizard.Post, "SetupWizardSaveResponse") {
		return errors.New("P4 setup-wizard canonical JSON contract is incomplete")
	}
	if err := validateRequiredCSRF(wizard.Post); err != nil {
		return fmt.Errorf("setup-wizard: %w", err)
	}
	if wizard.Post.Extensions["x-aicrm-route-bound-action-token"] != "required" || wizard.Post.Responses.Value("400") == nil || wizard.Post.Responses.Value("409") == nil || wizard.Post.Responses.Value("503") == nil {
		return errors.New("P4 setup-wizard write safety contract drifted")
	}
	request := doc.Components.Schemas["SetupWizardSaveRequest"]
	wizardMasked := doc.Components.Schemas["SetupWizardMaskedSetting"]
	aiMasked := doc.Components.Schemas["SetupWizardUnavailableMaskedSetting"]
	snapshot := doc.Components.Schemas["SetupWizardSnapshot"]
	readResponse := doc.Components.Schemas["SetupWizardReadResponse"]
	saveResponse := doc.Components.Schemas["SetupWizardSaveResponse"]
	if request == nil || request.Value == nil || request.Value.AdditionalProperties.Has == nil || *request.Value.AdditionalProperties.Has ||
		wizardMasked == nil || wizardMasked.Value == nil || wizardMasked.Value.AdditionalProperties.Has == nil || *wizardMasked.Value.AdditionalProperties.Has || !legacyTagBooleanEnum(wizardMasked.Value, "masked", true) {
		return errors.New("P4 setup-wizard request or masked projection must remain closed")
	}
	if snapshot == nil || snapshot.Value == nil || snapshot.Value.Properties["editable_configured"] == nil || snapshot.Value.Properties["editable_configured"].Ref != "#/components/schemas/SetupWizardEditableConfigured" ||
		readResponse == nil || readResponse.Value == nil || saveResponse == nil || saveResponse.Value == nil ||
		!legacyTagBooleanEnum(readResponse.Value, "local_only", true) || !legacyTagBooleanEnum(readResponse.Value, "runtime_applied", false) ||
		!legacyTagBooleanEnum(saveResponse.Value, "local_only", true) || !legacyTagBooleanEnum(saveResponse.Value, "runtime_applied", false) {
		return errors.New("P4 setup-wizard must expose unconfigured state and fail-closed runtime application")
	}
	if aiMasked == nil || aiMasked.Value == nil || aiMasked.Value.AdditionalProperties.Has == nil || *aiMasked.Value.AdditionalProperties.Has ||
		!legacyTagBooleanEnum(aiMasked.Value, "configured", false) || !legacyTagBooleanEnum(aiMasked.Value, "masked", true) {
		return errors.New("P4 setup-wizard AI key must remain unavailable and masked")
	}
	for _, key := range []string{"wecom.secret", "wecom.callback_token", "wecom.callback_aes_key", "ai.api_key"} {
		property := request.Value.Properties[key]
		if property == nil || property.Value == nil || property.Value.MaxLength == nil || *property.Value.MaxLength != 0 {
			return fmt.Errorf("setup-wizard secret key %s must only accept empty preservation", key)
		}
	}
	return nil
}

func operationFormUsesLocalSchema(operation *openapi3.Operation, schemaName string) bool {
	if operation == nil || operation.RequestBody == nil || operation.RequestBody.Value == nil {
		return false
	}
	media := operation.RequestBody.Value.Content["application/x-www-form-urlencoded"]
	return media != nil && media.Schema != nil && media.Schema.Ref == "#/components/schemas/"+schemaName
}

func validateGroupInviteContract(doc *openapi3.T) error {
	collection := doc.Paths.Value("/api/admin/group-invite-library")
	detail := doc.Paths.Value("/api/admin/group-invite-library/{item_id}")
	if collection == nil || collection.Get == nil || collection.Post == nil || detail == nil || detail.Get == nil || detail.Put == nil || detail.Delete == nil {
		return errors.New("P4-H03 Group Invite compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(collection.Get, "LegacyGroupInviteListResponse") ||
		!operationRequestUsesLocalSchema(collection.Post, "LegacyGroupInviteCreateRequest") ||
		!operationResponseUsesLocalSchema(collection.Post, "LegacyGroupInviteMutationResponse") ||
		!operationResponseUsesLocalSchema(detail.Get, "LegacyGroupInviteDetailResponse") ||
		!operationRequestUsesLocalSchema(detail.Put, "LegacyGroupInviteUpdateRequest") ||
		!operationResponseUsesLocalSchema(detail.Put, "LegacyGroupInviteMutationResponse") ||
		!operationResponseUsesLocalSchema(detail.Delete, "LegacyGroupInviteArchiveResponse") {
		return errors.New("P4-H03 Group Invite request or response schema drifted")
	}
	for name, operation := range map[string]*openapi3.Operation{"createLegacyGroupInvite": collection.Post, "updateLegacyGroupInvite": detail.Put, "archiveLegacyGroupInvite": detail.Delete} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s conflict or dependency response drifted", name)
		}
	}
	return nil
}

func validateChannelContract(doc *openapi3.T) error {
	collection := doc.Paths.Value("/api/admin/channels")
	detail := doc.Paths.Value("/api/admin/channels/{channel_id}")
	entrants := doc.Paths.Value("/api/admin/channels/{channel_id}/contacts")
	if collection == nil || collection.Get == nil || collection.Post == nil || detail == nil || detail.Get == nil || detail.Patch == nil || entrants == nil || entrants.Get == nil {
		return errors.New("P4-C01 Channel compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(collection.Get, "LegacyChannelListResponse") ||
		!operationRequestUsesLocalSchema(collection.Post, "LegacyChannelWriteRequest") ||
		!operationResponseUsesStatusLocalSchema(collection.Post, "201", "LegacyChannelMutationResponse") ||
		!operationResponseUsesLocalSchema(detail.Get, "LegacyChannelDetailResponse") ||
		!operationRequestUsesLocalSchema(detail.Patch, "LegacyChannelWriteRequest") ||
		!operationResponseUsesLocalSchema(detail.Patch, "LegacyChannelMutationResponse") ||
		!operationResponseUsesLocalSchema(entrants.Get, "LegacyChannelEntrantsResponse") {
		return errors.New("P4-C01 Channel request or response schema drifted")
	}
	for name, operation := range map[string]*openapi3.Operation{"createLegacyChannel": collection.Post, "updateLegacyChannel": detail.Patch} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if operation.Responses.Value("400") == nil || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s boundary or failure responses drifted", name)
		}
	}
	request := doc.Components.Schemas["LegacyChannelWriteRequest"]
	if request == nil || request.Value == nil || request.Value.AdditionalProperties.Has == nil || *request.Value.AdditionalProperties.Has {
		return errors.New("LegacyChannelWriteRequest must remain closed")
	}
	for name, required := range map[string][]string{
		"LegacyChannelMutationResponse": {"ok", "channel", "reason", "source", "fallback_used", "provider_execution_eligible", "real_external_call_executed"},
		"LegacyChannelEntrantsResponse": {"channel_id", "items", "limit", "has_more", "next_cursor", "local_projection", "provider_execution_eligible", "real_external_call_executed"},
	} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || !reflect.DeepEqual(schema.Value.Required, required) || !legacyTagBooleanEnum(schema.Value, "provider_execution_eligible", false) || !legacyTagBooleanEnum(schema.Value, "real_external_call_executed", false) {
			return fmt.Errorf("%s provider gate drifted", name)
		}
	}
	return nil
}

func validateCouponContract(doc *openapi3.T) error {
	collection := doc.Paths.Value("/api/admin/coupons")
	detail := doc.Paths.Value("/api/admin/coupons/{coupon_id}")
	publish := doc.Paths.Value("/api/admin/coupons/{coupon_id}/publish")
	stop := doc.Paths.Value("/api/admin/coupons/{coupon_id}/stop")
	listPage := doc.Paths.Value("/admin/coupons")
	newPage := doc.Paths.Value("/admin/coupons/new")
	dataPage := doc.Paths.Value("/admin/coupons/{coupon_id}/data")
	editPage := doc.Paths.Value("/admin/coupons/{coupon_id}/edit")
	options := doc.Paths.Value("/api/admin/coupons/product-options")
	archive := doc.Paths.Value("/api/admin/coupons/{coupon_id}/archive")
	claims := doc.Paths.Value("/api/admin/coupons/{coupon_id}/claims")
	copyPage := doc.Paths.Value("/api/admin/coupons/{coupon_id}/copy")
	share := doc.Paths.Value("/api/admin/coupons/{coupon_id}/share")
	h5Available := doc.Paths.Value("/api/h5/coupons/available")
	h5Coupon := doc.Paths.Value("/api/h5/coupons/{public_slug}")
	h5Claim := doc.Paths.Value("/api/h5/coupons/{public_slug}/claim")
	sidebar := doc.Paths.Value("/api/sidebar/v2/coupons")
	publicPage := doc.Paths.Value("/c/{public_slug}")
	if collection == nil || collection.Get == nil || collection.Post == nil || detail == nil || detail.Get == nil || detail.Put == nil || detail.Delete == nil || publish == nil || publish.Post == nil || stop == nil || stop.Post == nil ||
		listPage == nil || listPage.Get == nil || newPage == nil || newPage.Get == nil || dataPage == nil || dataPage.Get == nil || editPage == nil || editPage.Get == nil || options == nil || options.Get == nil || archive == nil || archive.Post == nil || claims == nil || claims.Get == nil || copyPage == nil || copyPage.Post == nil || share == nil || share.Get == nil || h5Available == nil || h5Available.Get == nil || h5Coupon == nil || h5Coupon.Get == nil || h5Claim == nil || h5Claim.Post == nil || sidebar == nil || sidebar.Get == nil || publicPage == nil || publicPage.Get == nil {
		return errors.New("P4 Coupon A+B compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(collection.Get, "LegacyCouponListResponse") || !operationRequestUsesLocalSchema(collection.Post, "CouponUpsertRequest") || !operationResponseUsesLocalSchema(collection.Post, "LegacyCouponCreateResponse") || !operationResponseUsesLocalSchema(detail.Get, "LegacyCouponDetailResponse") || !operationRequestUsesLocalSchema(detail.Put, "CouponUpsertRequest") || !operationResponseUsesLocalSchema(detail.Put, "LegacyCouponUpdateResponse") || !operationResponseUsesLocalSchema(publish.Post, "LegacyCouponMutationResponse") || !operationResponseUsesLocalSchema(stop.Post, "LegacyCouponMutationResponse") ||
		!operationResponseUsesLocalSchema(detail.Delete, "LegacyCouponBoardMutationResponse") || !operationResponseUsesLocalSchema(options.Get, "LegacyCouponProductOptionsResponse") || !operationResponseUsesLocalSchema(archive.Post, "LegacyCouponBoardMutationResponse") || !operationResponseUsesLocalSchema(claims.Get, "LegacyCouponClaimListResponse") || !operationResponseUsesLocalSchema(copyPage.Post, "LegacyCouponBoardMutationResponse") || !operationResponseUsesLocalSchema(share.Get, "LegacyCouponShareResponse") || !operationResponseUsesLocalSchema(h5Available.Get, "H5CouponAvailableResponse") || !operationResponseUsesLocalSchema(h5Coupon.Get, "H5CouponDetailResponse") || !operationResponseUsesLocalSchema(h5Claim.Post, "H5CouponClaimResponse") || !operationResponseUsesLocalSchema(sidebar.Get, "SidebarCouponListResponse") {
		return errors.New("P4 Coupon A+B request or response schema drifted")
	}
	for name, operation := range map[string]*openapi3.Operation{"createLegacyCoupon": collection.Post, "updateLegacyCoupon": detail.Put, "publishLegacyCoupon": publish.Post, "stopLegacyCoupon": stop.Post} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if operation.Responses.Value("400") == nil || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s boundary or failure responses drifted", name)
		}
	}
	for name, operation := range map[string]*openapi3.Operation{"deleteLegacyCoupon": detail.Delete, "archiveLegacyCoupon": archive.Post, "copyLegacyCoupon": copyPage.Post} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !hasRequiredHeader(operation, "Idempotency-Key") || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks required idempotency, conflict, or unavailable response", name)
		}
	}
	if !hasRequiredHeader(h5Claim.Post, "Idempotency-Key") || h5Claim.Post.Responses.Value("401") == nil || h5Claim.Post.Responses.Value("409") == nil || h5Claim.Post.Responses.Value("503") == nil {
		return errors.New("claimH5Coupon lacks frozen auth, idempotency, conflict, or unavailable response")
	}
	if csrf, ok := h5Claim.Post.Extensions["x-aicrm-csrf"].(string); !ok || csrf != "same_origin_empty_body" {
		return errors.New("claimH5Coupon CSRF applicability drifted")
	}
	if h5Available.Get.Responses.Value("401") == nil || sidebar.Get.Responses.Value("401") == nil || h5Coupon.Get.Responses.Value("404") == nil || publicPage.Get.Responses.Value("404") == nil {
		return errors.New("P4 Coupon public access failures drifted")
	}
	for _, parameter := range collection.Post.Parameters {
		if parameter != nil && parameter.Value != nil && parameter.Value.Name == "Idempotency-Key" {
			return errors.New("legacy coupon create must not claim replay safety")
		}
	}
	request := doc.Components.Schemas["CouponUpsertRequest"]
	if request == nil || request.Value == nil || request.Value.AdditionalProperties.Has == nil || *request.Value.AdditionalProperties.Has || len(request.Value.Properties) != 12 {
		return errors.New("CouponUpsertRequest must remain closed with 11 rule fields plus target_refs")
	}
	required := append([]string(nil), request.Value.Required...)
	sort.Strings(required)
	want := []string{"claim_ends_at", "claim_starts_at", "discount_amount_total", "name", "target_refs", "total_issue_limit", "validity_mode"}
	if !reflect.DeepEqual(required, want) {
		return fmt.Errorf("CouponUpsertRequest required=%v", required)
	}
	perUser, instructions := request.Value.Properties["per_user_issue_limit"], request.Value.Properties["instructions"]
	if perUser == nil || perUser.Value == nil || fmt.Sprint(perUser.Value.Default) != "1" || instructions == nil || instructions.Value == nil || fmt.Sprint(instructions.Value.Default) != "" {
		return errors.New("CouponUpsertRequest defaults drifted")
	}
	for name, required := range map[string][]string{
		"LegacyCouponCreateResponse": {"ok", "coupon", "coupon_id", "fallback_used", "create_replay_safe", "real_external_call_executed"},
		"LegacyCouponUpdateResponse": {"ok", "coupon", "fallback_used", "real_external_call_executed"},
	} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has || len(schema.Value.Properties) != len(required) {
			return fmt.Errorf("%s must be closed with exact fields", name)
		}
		actual := append([]string(nil), schema.Value.Required...)
		sort.Strings(actual)
		sort.Strings(required)
		if !reflect.DeepEqual(actual, required) {
			return fmt.Errorf("%s required=%v", name, actual)
		}
	}
	for _, name := range []string{"LegacyCouponProductOptionsResponse", "LegacyCouponClaimListResponse", "LegacyCouponShareResponse", "H5CouponAvailableResponse", "H5CouponDetailResponse", "H5CouponClaimResponse", "SidebarCouponListResponse"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has {
			return fmt.Errorf("%s must remain closed", name)
		}
	}
	claim := doc.Components.Schemas["LegacyCouponClaim"]
	sidebarCoupon := doc.Components.Schemas["SidebarCoupon"]
	if claim == nil || claim.Value == nil || claim.Value.Properties["customer_id"] != nil || sidebarCoupon == nil || sidebarCoupon.Value == nil || sidebarCoupon.Value.Properties["customer_id"] != nil {
		return errors.New("Coupon claims and sidebar projections must not disclose customer IDs")
	}
	return nil
}

func validateCustomerCompatContract(doc *openapi3.T) error {
	list := doc.Paths.Value("/api/customers")
	detail := doc.Paths.Value("/api/customers/{external_userid}")
	if list == nil || list.Get == nil || detail == nil || detail.Get == nil ||
		!operationResponseUsesLocalSchema(list.Get, "LegacyCustomerListResponse") ||
		!operationResponseUsesLocalSchema(detail.Get, "LegacyCustomerDetailResponse") {
		return errors.New("P4-B01 Customer compatibility operations are incomplete")
	}
	if list.Get.Responses.Value("400") == nil || list.Get.Responses.Value("503") == nil ||
		detail.Get.Responses.Value("404") == nil || detail.Get.Responses.Value("503") == nil {
		return errors.New("P4-B01 Customer boundary or failure responses drifted")
	}
	return nil
}

func validateSurveyContract(doc *openapi3.T) error {
	collection := doc.Paths.Value("/api/admin/questionnaires")
	preflight := doc.Paths.Value("/api/admin/questionnaires/preflight")
	detail := doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}")
	duplicate := doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/duplicate")
	disable := doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/disable")
	enable := doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/enable")
	if collection == nil || collection.Get == nil || collection.Post == nil || preflight == nil || preflight.Get == nil || detail == nil || detail.Get == nil ||
		detail.Put == nil || detail.Patch == nil || detail.Delete == nil ||
		duplicate == nil || duplicate.Post == nil || disable == nil || disable.Post == nil || enable == nil || enable.Post == nil {
		return errors.New("P4-F01A Survey compatibility operations are incomplete")
	}
	if preflight.Get.OperationID != "getLegacyQuestionnairePreflight" || preflight.Get.RequestBody != nil || len(preflight.Get.Parameters) != 0 ||
		!operationResponseUsesLocalSchema(preflight.Get, "LegacyQuestionnairePreflightResponse") ||
		preflight.Get.Responses.Value("401") == nil || preflight.Get.Responses.Value("403") == nil ||
		preflight.Get.Responses.Value("405") == nil || preflight.Get.Responses.Value("503") == nil {
		return errors.New("P4-F01A Questionnaire preflight contract drifted")
	}
	if !operationResponseUsesLocalSchema(collection.Get, "LegacyQuestionnaireListResponse") ||
		!operationRequestUsesLocalSchema(collection.Post, "LegacyQuestionnaireCreateRequest") ||
		!operationResponseUsesLocalSchema(collection.Post, "LegacyQuestionnaireCreateResponse") ||
		!operationResponseUsesLocalSchema(detail.Get, "LegacyQuestionnaireDetailResponse") {
		return errors.New("P4-F01A Survey request or response schema drifted")
	}
	if err := validateRequiredCSRF(collection.Post); err != nil {
		return fmt.Errorf("createLegacyQuestionnaire: %w", err)
	}
	if collection.Post.Responses.Value("400") == nil || collection.Post.Responses.Value("409") == nil ||
		collection.Post.Responses.Value("503") == nil || detail.Get.Responses.Value("404") == nil || detail.Get.Responses.Value("503") == nil {
		return errors.New("P4-F01A Survey boundary or failure responses drifted")
	}
	for _, contract := range []struct {
		name     string
		op       *openapi3.Operation
		request  string
		response string
	}{
		{"replaceLegacyQuestionnaire", detail.Put, "LegacyQuestionnaireCreateRequest", "LegacyQuestionnaireMutationResponse"},
		{"updateLegacyQuestionnaire", detail.Patch, "LegacyQuestionnaireCreateRequest", "LegacyQuestionnaireMutationResponse"},
		{"deleteLegacyQuestionnaire", detail.Delete, "", "LegacyQuestionnaireDeleteResponse"},
		{"duplicateLegacyQuestionnaire", duplicate.Post, "LegacyQuestionnaireDuplicateRequest", "LegacyQuestionnaireMutationResponse"},
		{"disableLegacyQuestionnaire", disable.Post, "LegacyQuestionnaireDisableRequest", "LegacyQuestionnaireMutationResponse"},
		{"enableLegacyQuestionnaire", enable.Post, "", "LegacyQuestionnaireMutationResponse"},
	} {
		if contract.op.OperationID != contract.name ||
			(contract.request == "" && contract.op.RequestBody != nil) ||
			(contract.request != "" && !operationRequestUsesLocalSchema(contract.op, contract.request)) ||
			!operationResponseUsesLocalSchema(contract.op, contract.response) ||
			!hasHeader(contract.op, "Idempotency-Key") ||
			contract.op.Responses.Value("400") == nil || contract.op.Responses.Value("404") == nil ||
			contract.op.Responses.Value("409") == nil || contract.op.Responses.Value("503") == nil {
			return fmt.Errorf("%s F01B boundary, schema, or idempotency contract drifted", contract.name)
		}
		if err := validateRequiredCSRF(contract.op); err != nil {
			return fmt.Errorf("%s: %w", contract.name, err)
		}
	}
	request := doc.Components.Schemas["LegacyQuestionnaireCreateRequest"]
	if request == nil || request.Value == nil || request.Value.AdditionalProperties.Has == nil || *request.Value.AdditionalProperties.Has {
		return errors.New("LegacyQuestionnaireCreateRequest must remain closed")
	}
	assessment := request.Value.Properties["assessment_enabled"]
	config := request.Value.Properties["assessment_config"]
	rules := request.Value.Properties["score_rules"]
	if assessment == nil || assessment.Value == nil || !reflect.DeepEqual(assessment.Value.Enum, []any{false}) ||
		config == nil || config.Value == nil || config.Value.MaxProps == nil || *config.Value.MaxProps != 0 ||
		rules == nil || rules.Value == nil || rules.Value.MaxItems == nil || *rules.Value.MaxItems != 0 {
		return errors.New("P4-F01A must keep F02 assessment unavailable")
	}
	return nil
}

func validateAutomationContract(doc *openapi3.T) error {
	item := doc.Paths.Value("/api/admin/automation-conversion/agent-runs")
	if item == nil || item.Get == nil || item.Post != nil || item.Put != nil || item.Patch != nil || item.Delete != nil {
		return errors.New("P4-W0-D01 Automation compatibility operation is incomplete")
	}
	operation := item.Get
	if operation.OperationID != "listAutomationTriggerRuns" || operation.RequestBody != nil ||
		!operationResponseUsesLocalSchema(operation, "AutomationTriggerRunListResponse") ||
		operation.Responses.Value("400") == nil || operation.Responses.Value("503") == nil {
		return errors.New("P4-W0-D01 Automation response or failure contract drifted")
	}
	parameters := map[string]bool{}
	for _, parameter := range operation.Parameters {
		if parameter == nil || parameter.Value == nil || parameter.Value.In != "query" || parameters[parameter.Value.Name] {
			return errors.New("P4-W0-D01 Automation query parameters are invalid")
		}
		parameters[parameter.Value.Name] = true
	}
	for _, name := range []string{"page", "page_size", "request_id", "run_id", "agent_code", "run_status", "trigger_source", "unionid", "userid", "started_after", "started_before", "has_error", "visibility"} {
		if !parameters[name] {
			return fmt.Errorf("P4-W0-D01 Automation query is missing %s", name)
		}
	}
	if len(parameters) != 13 {
		return fmt.Errorf("P4-W0-D01 Automation query parameter count=%d", len(parameters))
	}
	for _, name := range []string{"AutomationTriggerRun", "AutomationTriggerRunListResponse"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has {
			return fmt.Errorf("%s must remain a closed response", name)
		}
	}
	return nil
}

func validateSegmentContract(doc *openapi3.T) error {
	segments := doc.Paths.Value("/api/v1/segments")
	segment := doc.Paths.Value("/api/v1/segments/{segment_id}")
	members := doc.Paths.Value("/api/v1/segments/{segment_id}/members")
	refresh := doc.Paths.Value("/api/v1/segments/{segment_id}/refresh")
	if segments == nil || segments.Get == nil || segments.Post == nil || segment == nil || segment.Get == nil || segment.Patch == nil ||
		members == nil || members.Get == nil || refresh == nil || refresh.Post == nil {
		return errors.New("P3 segment operations are incomplete")
	}
	for _, contract := range []struct {
		operation *openapi3.Operation
		status    string
		schema    string
	}{
		{segments.Get, "200", "SegmentPage"}, {segments.Post, "201", "Segment"},
		{segment.Get, "200", "Segment"}, {segment.Patch, "200", "Segment"},
		{members.Get, "200", "SegmentMemberPage"}, {refresh.Post, "202", "SegmentRefreshAccepted"},
	} {
		if !operationResponseUsesStatusLocalSchema(contract.operation, contract.status, contract.schema) {
			return fmt.Errorf("%s response schema ref drifted", contract.operation.OperationID)
		}
	}
	for _, contract := range []struct {
		operation *openapi3.Operation
		schema    string
	}{
		{segments.Post, "CreateSegmentRequest"}, {segment.Patch, "UpdateSegmentRequest"},
	} {
		if !operationRequestUsesLocalSchema(contract.operation, contract.schema) {
			return fmt.Errorf("%s request schema ref drifted", contract.operation.OperationID)
		}
	}
	for _, operation := range []*openapi3.Operation{segments.Post, segment.Patch, refresh.Post} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", operation.OperationID, err)
		}
		if !hasRequiredHeader(operation, "Idempotency-Key") || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks idempotency, conflict, or unavailable response", operation.OperationID)
		}
	}
	for _, operation := range []*openapi3.Operation{segments.Get, segment.Get, members.Get} {
		if operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks unavailable response", operation.OperationID)
		}
	}
	definition := doc.Components.Schemas["SegmentDefinition"]
	if definition == nil || definition.Value == nil || len(definition.Value.OneOf) != 3 {
		return errors.New("SegmentDefinition must remain a three-node closed AST")
	}
	for _, name := range []string{"SegmentDefinitionAnd", "SegmentDefinitionOr", "SegmentDefinitionPredicate"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has {
			return fmt.Errorf("%s must remain a closed AST node", name)
		}
	}
	predicate := doc.Components.Schemas["SegmentDefinitionPredicate"].Value
	for field, want := range map[string][]string{
		"field": {"added_at", "channel_id", "is_deleted", "last_interact_at", "owner_staff_id", "stage_id", "tag_id"},
		"op":    {"after", "before", "eq", "has_any", "in"},
	} {
		property := predicate.Properties[field]
		if property == nil || property.Value == nil {
			return fmt.Errorf("SegmentDefinitionPredicate missing %s", field)
		}
		got, err := stringList(property.Value.Enum)
		if err != nil {
			return err
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("SegmentDefinitionPredicate.%s=%v", field, got)
		}
	}
	return nil
}

func validateIdentityContract(doc *openapi3.T) error {
	resolve := doc.Paths.Value("/api/v1/identity/resolve")
	bind := doc.Paths.Value("/api/v1/identity/bind")
	ingest := doc.Paths.Value("/api/v1/identity/ingest")
	reviews := doc.Paths.Value("/api/v1/identity/merge-reviews")
	approve := doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve")
	reject := doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject")
	if resolve == nil || resolve.Post == nil || bind == nil || bind.Post == nil || ingest == nil || ingest.Post == nil ||
		reviews == nil || reviews.Get == nil || approve == nil || approve.Post == nil || reject == nil || reject.Post == nil {
		return errors.New("P3 identity operations are incomplete")
	}
	for _, contract := range []struct {
		operation *openapi3.Operation
		request   string
		response  string
	}{
		{resolve.Post, "ResolveIdentityRequest", "ResolveIdentityResponse"},
		{bind.Post, "BindIdentityRequest", "BindIdentityResponse"},
		{ingest.Post, "IngestIdentityEventRequest", "IngestIdentityEventResponse"},
		{reviews.Get, "", "IdentityMergeReviewPage"},
		{approve.Post, "ApproveIdentityMergeReviewRequest", "IdentityMergeReview"},
		{reject.Post, "RejectIdentityMergeReviewRequest", "IdentityMergeReview"},
	} {
		if contract.request != "" && !operationRequestUsesLocalSchema(contract.operation, contract.request) {
			return fmt.Errorf("%s request schema ref drifted", contract.operation.OperationID)
		}
		if !operationResponseUsesLocalSchema(contract.operation, contract.response) {
			return fmt.Errorf("%s response schema ref drifted", contract.operation.OperationID)
		}
	}
	for name, operation := range map[string]*openapi3.Operation{
		"bindIdentity": bind.Post, "ingestIdentityEvent": ingest.Post,
		"approveIdentityMergeReview": approve.Post, "rejectIdentityMergeReview": reject.Post,
	} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !hasRequiredHeader(operation, "Idempotency-Key") {
			return fmt.Errorf("%s lacks required Idempotency-Key", name)
		}
		if operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks conflict or unavailable response", name)
		}
	}
	for _, operation := range []*openapi3.Operation{resolve.Post, reviews.Get} {
		if operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks unavailable response", operation.OperationID)
		}
	}
	for _, operation := range []*openapi3.Operation{resolve.Post, bind.Post, ingest.Post, approve.Post, reject.Post} {
		if operation.Responses.Value("422") == nil {
			return fmt.Errorf("%s lacks semantic validation response", operation.OperationID)
		}
	}
	ref := doc.Components.Schemas["IdentityRef"]
	if ref == nil || ref.Value == nil || ref.Value.Properties["assurance"] != nil || ref.Value.Properties["source"] != nil {
		return errors.New("admin IdentityRef must not accept assurance or source")
	}
	for _, contract := range []struct {
		name     string
		variants map[string][]string
	}{
		{"ResolveIdentityResponse", map[string][]string{
			"ResolveIdentityFound": {"customer_id", "status"}, "ResolveIdentityNotFound": {"status"},
			"ResolveIdentityConflict": {"status"},
		}},
		{"BindIdentityResponse", map[string][]string{
			"BindIdentityBound": {"customer_id", "status"}, "BindIdentityAlreadyBound": {"customer_id", "status"},
			"BindIdentityMerged":       {"customer_id", "merge_audit_id", "primary_customer_id", "status"},
			"BindIdentityManualReview": {"review_id", "status"}, "BindIdentityRejected": {"status"},
		}},
		{"IngestIdentityEventResponse", map[string][]string{
			"IngestIdentityEventAttributed": {"customer_id", "event_id", "status"},
			"IngestIdentityEventPending":    {"pending_event_id", "status"},
			"IngestIdentityEventConflict":   {"pending_event_id", "status"},
		}},
	} {
		if err := validateStrictStatusUnion(doc, contract.name, contract.variants); err != nil {
			return err
		}
	}
	review := doc.Components.Schemas["IdentityMergeReview"]
	if review == nil || review.Value == nil || review.Value.Properties["identity_fingerprint"] == nil ||
		review.Value.Properties["value"] != nil || review.Value.Properties["normalized_value"] != nil {
		return errors.New("merge review identity redaction drifted")
	}
	wantReviewFields := []string{"created_at", "customer_ids", "identity_fingerprint", "resolved_at", "review_id", "scope", "status", "type", "version"}
	gotReviewFields := make([]string, 0, len(review.Value.Properties))
	for field := range review.Value.Properties {
		gotReviewFields = append(gotReviewFields, field)
	}
	sort.Strings(gotReviewFields)
	if !reflect.DeepEqual(gotReviewFields, wantReviewFields) || review.Value.AdditionalProperties.Has == nil ||
		*review.Value.AdditionalProperties.Has {
		return fmt.Errorf("merge review response fields=%v", gotReviewFields)
	}
	reviewRequired := append([]string(nil), review.Value.Required...)
	sort.Strings(reviewRequired)
	if !reflect.DeepEqual(reviewRequired, wantReviewFields) {
		return fmt.Errorf("merge review response required=%v", reviewRequired)
	}
	customerIDs := review.Value.Properties["customer_ids"].Value
	if customerIDs == nil || customerIDs.MinItems != 2 || customerIDs.MaxItems == nil ||
		*customerIDs.MaxItems != 2 || !customerIDs.UniqueItems {
		return errors.New("merge review must contain exactly two unique current roots")
	}
	fingerprint := review.Value.Properties["identity_fingerprint"].Value
	if fingerprint == nil || fingerprint.Pattern != `^hmac-sha256-v[1-9][0-9]*:[A-Za-z0-9_-]{21}[AQgw]$` {
		return errors.New("merge review fingerprint is not a versioned secret-backed HMAC")
	}
	page := doc.Components.Schemas["IdentityMergeReviewPage"]
	if page == nil || page.Value == nil {
		return errors.New("IdentityMergeReviewPage is missing")
	}
	pageFields := make([]string, 0, len(page.Value.Properties))
	for field := range page.Value.Properties {
		pageFields = append(pageFields, field)
	}
	sort.Strings(pageFields)
	if page.Value.Type == nil || !page.Value.Type.Is("object") ||
		!reflect.DeepEqual(pageFields, []string{"items", "next_cursor"}) ||
		page.Value.AdditionalProperties.Has == nil || *page.Value.AdditionalProperties.Has {
		return fmt.Errorf("IdentityMergeReviewPage fields=%v", pageFields)
	}
	pageRequired := append([]string(nil), page.Value.Required...)
	sort.Strings(pageRequired)
	if !reflect.DeepEqual(pageRequired, []string{"items", "next_cursor"}) {
		return fmt.Errorf("IdentityMergeReviewPage required=%v", pageRequired)
	}
	items := page.Value.Properties["items"]
	nextCursor := page.Value.Properties["next_cursor"]
	if items == nil || items.Value == nil || items.Value.Type == nil || !items.Value.Type.Is("array") ||
		items.Value.Items == nil || items.Value.Items.Ref != "#/components/schemas/IdentityMergeReview" ||
		nextCursor == nil || nextCursor.Value == nil || nextCursor.Value.Type == nil ||
		!nextCursor.Value.Type.Is("string") || !nextCursor.Value.Nullable {
		return errors.New("IdentityMergeReviewPage item or cursor shape drifted")
	}
	return nil
}

func operationRequestUsesLocalSchema(operation *openapi3.Operation, schemaName string) bool {
	if operation == nil || operation.RequestBody == nil || operation.RequestBody.Value == nil {
		return false
	}
	media := operation.RequestBody.Value.Content["application/json"]
	return media != nil && media.Schema != nil && media.Schema.Ref == "#/components/schemas/"+schemaName
}

func operationResponseUsesLocalSchema(operation *openapi3.Operation, schemaName string) bool {
	return operationResponseUsesStatusLocalSchema(operation, "200", schemaName)
}

func operationResponseUsesStatusLocalSchema(operation *openapi3.Operation, status, schemaName string) bool {
	if operation == nil {
		return false
	}
	response := operation.Responses.Value(status)
	if response == nil || response.Value == nil {
		return false
	}
	media := response.Value.Content["application/json"]
	return media != nil && media.Schema != nil && media.Schema.Ref == "#/components/schemas/"+schemaName
}

func validateStrictStatusUnion(doc *openapi3.T, name string, variants map[string][]string) error {
	union := doc.Components.Schemas[name]
	if union == nil || union.Value == nil || len(union.Value.OneOf) != len(variants) ||
		union.Value.Discriminator == nil || union.Value.Discriminator.PropertyName != "status" ||
		len(union.Value.Discriminator.Mapping) != len(variants) {
		return fmt.Errorf("%s status union drifted", name)
	}
	seen := map[string]bool{}
	for _, variantRef := range union.Value.OneOf {
		const prefix = "#/components/schemas/"
		if variantRef == nil || !strings.HasPrefix(variantRef.Ref, prefix) {
			return fmt.Errorf("%s has an inline or external variant", name)
		}
		variantName := strings.TrimPrefix(variantRef.Ref, prefix)
		wantRequired, ok := variants[variantName]
		if !ok || seen[variantName] {
			return fmt.Errorf("%s has unexpected variant %s", name, variantName)
		}
		seen[variantName] = true
		variant := doc.Components.Schemas[variantName]
		if variant == nil || variant.Value == nil || variant.Value.Type == nil || !variant.Value.Type.Is("object") ||
			variant.Value.AdditionalProperties.Has == nil || *variant.Value.AdditionalProperties.Has ||
			len(variant.Value.Properties) != len(wantRequired) {
			return fmt.Errorf("%s variant %s permits ambiguous fields", name, variantName)
		}
		gotRequired := append([]string(nil), variant.Value.Required...)
		sort.Strings(gotRequired)
		if !reflect.DeepEqual(gotRequired, wantRequired) {
			return fmt.Errorf("%s variant %s required=%v", name, variantName, gotRequired)
		}
		status := variant.Value.Properties["status"]
		if status == nil || status.Value == nil || len(status.Value.Enum) != 1 {
			return fmt.Errorf("%s variant %s lacks a single status", name, variantName)
		}
		statusValue, ok := status.Value.Enum[0].(string)
		if !ok || union.Value.Discriminator.Mapping[statusValue] != prefix+variantName {
			return fmt.Errorf("%s variant %s discriminator mapping drifted", name, variantName)
		}
	}
	return nil
}

func hasRequiredHeader(operation *openapi3.Operation, name string) bool {
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == "header" && ref.Value.Name == name && ref.Value.Required {
			return true
		}
	}
	return false
}

func hasHeader(operation *openapi3.Operation, name string) bool {
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == "header" && ref.Value.Name == name {
			return true
		}
	}
	return false
}

func validateBrowserSessionContract(doc *openapi3.T) error {
	scheme := doc.Components.SecuritySchemes["AdminSession"]
	if scheme == nil || scheme.Value == nil || scheme.Value.Type != "apiKey" ||
		scheme.Value.In != "cookie" || scheme.Value.Name != "aicrm_session" {
		return errors.New("AdminSession must remain an opaque aicrm_session cookie")
	}
	logout := doc.Paths.Value("/api/v1/auth/logout")
	if logout == nil || logout.Post == nil {
		return errors.New("logout operation missing")
	}
	var csrf *openapi3.Parameter
	for _, ref := range logout.Post.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.Name == "X-CSRF-Token" {
			csrf = ref.Value
			break
		}
	}
	if csrf == nil || csrf.In != "header" || !csrf.Required || csrf.Schema == nil || csrf.Schema.Value == nil {
		return errors.New("logout lacks required X-CSRF-Token header")
	}
	schema := csrf.Schema.Value
	if schema.MinLength != 43 || schema.MaxLength == nil || *schema.MaxLength != 43 || schema.Pattern != "^[A-Za-z0-9_-]{43}$" {
		return errors.New("logout CSRF token shape is not frozen")
	}
	for _, status := range []string{"204", "401", "403"} {
		if logout.Post.Responses.Value(status) == nil {
			return fmt.Errorf("logout response missing: %s", status)
		}
	}
	return nil
}

func validateStageContract(doc *openapi3.T) error {
	stages := doc.Paths.Value("/api/v1/stages")
	stage := doc.Paths.Value("/api/v1/stages/{stage_id}")
	if stages == nil || stages.Get == nil || stages.Post == nil || stage == nil || stage.Patch == nil {
		return errors.New("P2 stage operations are incomplete")
	}
	for name, operation := range map[string]*openapi3.Operation{
		"createStage": stages.Post,
		"renameStage": stage.Patch,
	} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, status := range []string{"401", "403", "422", "503"} {
			if operation.Responses.Value(status) == nil {
				return fmt.Errorf("%s response missing: %s", name, status)
			}
		}
	}
	if stages.Post.Responses.Value("201") == nil || stage.Patch.Responses.Value("200") == nil ||
		stage.Patch.Responses.Value("404") == nil {
		return errors.New("P2 stage success or not-found responses drifted")
	}
	return nil
}

func validateContactContract(doc *openapi3.T) error {
	customers := doc.Paths.Value("/api/v1/customers")
	detail := doc.Paths.Value("/api/v1/customers/{customer_id}")
	events := doc.Paths.Value("/api/v1/customers/{customer_id}/events")
	stage := doc.Paths.Value("/api/v1/customers/{customer_id}/stage")
	tags := doc.Paths.Value("/api/v1/customers/{customer_id}/tags/{tag_id}")
	catalog := doc.Paths.Value("/api/v1/tags")
	if customers == nil || customers.Get == nil || detail == nil || detail.Get == nil || detail.Patch == nil ||
		events == nil || events.Get == nil || stage == nil || stage.Put == nil ||
		tags == nil || tags.Put == nil || tags.Delete == nil || catalog == nil || catalog.Get == nil {
		return errors.New("P3 contact operations are incomplete")
	}

	wantFilters := []string{
		"added_after", "added_before", "channel_id", "cursor", "is_deleted", "keyword", "last_interact_after",
		"last_interact_before", "limit", "mobile", "owner_staff_id", "stage_id", "tag_id",
	}
	gotFilters := make([]string, 0, len(customers.Get.Parameters))
	for _, ref := range customers.Get.Parameters {
		if ref == nil || ref.Value == nil || ref.Value.In != "query" {
			return errors.New("listCustomers has invalid query parameter")
		}
		if ref.Value.Name == "offset" {
			return errors.New("listCustomers must not expose offset pagination")
		}
		gotFilters = append(gotFilters, ref.Value.Name)
	}
	sort.Strings(gotFilters)
	if fmt.Sprint(gotFilters) != fmt.Sprint(wantFilters) {
		return fmt.Errorf("listCustomers filters=%v", gotFilters)
	}
	mobile := customers.Get.Parameters.GetByInAndName("query", "mobile")
	if mobile == nil || mobile.Schema == nil || mobile.Schema.Value == nil || mobile.Schema.Value.Type == nil || !mobile.Schema.Value.Type.Is("string") ||
		mobile.Schema.Value.Pattern != `^\+[1-9][0-9]{1,14}$` || mobile.Schema.Value.MinLength != 3 || mobile.Schema.Value.MaxLength == nil || *mobile.Schema.Value.MaxLength != 32 {
		return errors.New("listCustomers mobile filter must remain exact E.164")
	}

	listResponse := doc.Components.Schemas["CustomerListResponse"]
	if listResponse == nil || listResponse.Value == nil {
		return errors.New("CustomerListResponse schema missing")
	}
	required := append([]string(nil), listResponse.Value.Required...)
	sort.Strings(required)
	wantRequired := []string{"items", "next_cursor", "total", "total_is_estimate", "watermark"}
	if fmt.Sprint(required) != fmt.Sprint(wantRequired) {
		return fmt.Errorf("CustomerListResponse required=%v", required)
	}

	update := doc.Components.Schemas["CustomerUpdateRequest"]
	if update == nil || update.Value == nil {
		return errors.New("CustomerUpdateRequest schema missing")
	}
	for _, name := range []string{"stage_id", "external_userid", "unionid", "openid", "phone", "mobile"} {
		if _, ok := update.Value.Properties[name]; ok {
			return fmt.Errorf("CustomerUpdateRequest contains forbidden field: %s", name)
		}
	}

	event := doc.Components.Schemas["CustomerEvent"]
	if event == nil || event.Value == nil || event.Value.Properties["actor"] == nil {
		return errors.New("CustomerEvent actor is not frozen")
	}
	actorRequired := false
	for _, name := range event.Value.Required {
		actorRequired = actorRequired || name == "actor"
	}
	if !actorRequired {
		return errors.New("CustomerEvent actor became optional")
	}

	for name, operation := range map[string]*openapi3.Operation{
		"updateCustomer": detail.Patch, "setCustomerStage": stage.Put,
		"addCustomerTag": tags.Put, "removeCustomerTag": tags.Delete,
	} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, status := range []string{"401", "403"} {
			if operation.Responses.Value(status) == nil {
				return fmt.Errorf("%s response missing: %s", name, status)
			}
		}
	}
	return nil
}

func validateRequiredCSRF(operation *openapi3.Operation) error {
	var csrf *openapi3.Parameter
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.Name == "X-CSRF-Token" {
			csrf = ref.Value
			break
		}
	}
	if csrf == nil || csrf.In != "header" || !csrf.Required || csrf.Schema == nil || csrf.Schema.Value == nil {
		return errors.New("required X-CSRF-Token header is missing")
	}
	schema := csrf.Schema.Value
	if schema.MinLength != 43 || schema.MaxLength == nil || *schema.MaxLength != 43 || schema.Pattern != "^[A-Za-z0-9_-]{43}$" {
		return errors.New("CSRF token shape is not frozen")
	}
	return nil
}

func stringList(value any) ([]string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result []string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	for _, item := range result {
		if item == "" {
			return nil, errors.New("blank list item")
		}
	}
	return result, nil
}

func stringMap(value any) (map[string]string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, errors.New("empty string map")
	}
	for key, item := range result {
		if key == "" || item == "" {
			return nil, errors.New("blank string map entry")
		}
	}
	return result, nil
}
