package main

import (
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const specPath = "../../api/openapi.yaml"
const mappingPath = "../../docs/api-mapping.jsonl"

var (
	freshOnce      sync.Once
	freshDocument  *openapi3.T
	freshInventory mappingInventory
	freshErr       error
)

func TestFrozenOpenAPI(t *testing.T) {
	doc, ids, err := load(specPath, mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(doc, ids); err != nil {
		t.Fatal(err)
	}
}

func TestInternalEventRegistryContractRemainsClosed(t *testing.T) {
	doc, inventory := fresh(t)
	if err := validateInternalEventRegistryContract(doc); err != nil {
		t.Fatal(err)
	}

	registry := doc.Components.Schemas["LegacyInternalEventDiagnosticsResponse"].Value.Properties["consumer_registry"].Value
	registry.MinItems = 4
	registry.MaxItems = uint64Pointer(4)
	reject(t, doc, inventory)
}

func uint64Pointer(value uint64) *uint64 { return &value }

func TestAdminOpsSafeProjectionContractRemainsClosed(t *testing.T) {
	doc, inventory := fresh(t)
	job := doc.Components.Schemas["AdminOpsJob"].Value
	for _, forbidden := range []string{"target_ref", "failure_code", "provider_receipt", "raw_response"} {
		if _, present := job.Properties[forbidden]; present {
			t.Fatalf("AdminOpsJob exposes forbidden property %q", forbidden)
		}
	}
	for _, required := range []string{"target_kind", "target_present", "target_mask", "failure_present", "failure_class", "local_only", "real_external_call_executed"} {
		if _, present := job.Properties[required]; !present || !containsString(job.Required, required) {
			t.Fatalf("AdminOpsJob missing required closed property %q", required)
		}
	}
	if got := job.Properties["failure_class"].Value.Enum; !reflect.DeepEqual(got, []any{"none", "local_failure", "outcome_unknown"}) {
		t.Fatalf("failure_class enum=%v", got)
	}
	releaseChanges := doc.Components.Schemas["AdminOpsReleaseChangesRead"].Value
	if releaseChanges.AdditionalProperties.Has == nil || *releaseChanges.AdditionalProperties.Has || !reflect.DeepEqual(releaseChanges.Properties["wecom.webhook_ref"].Value.Enum, []any{"masked"}) {
		t.Fatal("AdminOps release changes must remain closed and mask webhook_ref")
	}
	categoryKey := doc.Components.Parameters["AdminOpsCategoryKey"].Value.Schema.Value
	if categoryKey.Pattern != "^[a-z][a-z0-9_]{1,79}$" || categoryKey.MinLength != 2 {
		t.Fatalf("AdminOps category key contract drifted: pattern=%q min=%d", categoryKey.Pattern, categoryKey.MinLength)
	}
	validationStatus := doc.Components.Schemas["AdminOpsLocalResponse"].Value.Properties["validationStatus"].Value
	if !reflect.DeepEqual(validationStatus.Enum, []any{"unconfigured", "unverified", "queued", "valid", "invalid"}) {
		t.Fatalf("notification validation enum=%v", validationStatus.Enum)
	}
	for _, operationID := range []string{"listAdminOpsCallbackJobs", "listAdminOpsDeferredJobs", "listAdminOpsWebhookDeliveryJobs", "listAdminOpsBroadcastJobs", "getAdminOpsBroadcastJob", "approveAdminOpsBroadcastJob", "cancelAdminOpsBroadcastJob", "getAdminOpsMessageBatch"} {
		contract := p4AdminOpsSafeOperations[operationID]
		operation := operationForMethod(doc.Paths.Value(contract.path), contract.method)
		if operation == nil || operation.Responses.Value("409") == nil || operation.Responses.Value("200") != nil || operation.Responses.Value("202") != nil || operation.Extensions["x-p4-status"] != "BLOCKED_REDLINE" {
			t.Fatalf("%s does not fail closed", operationID)
		}
	}
	doc.Paths.Value("/api/admin/config/categories").Get.Extensions["x-aicrm-local-only"] = false
	reject(t, doc, inventory)
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestCanonicalCandidateDeclarationDoesNotRequireRunnerRegistryChanges(t *testing.T) {
	build := func(t *testing.T) (*openapi3.T, mappingInventory) {
		t.Helper()
		doc, inventory := fresh(t)
		template := doc.Paths.Value("/api/admin/execution-runtime").Get
		if template == nil {
			t.Fatal("canonical fixture template is missing")
		}
		operation := *template
		operation.OperationID = "getCanonicalFixture"
		operation.Extensions = make(map[string]any, len(template.Extensions))
		for key, value := range template.Extensions {
			operation.Extensions[key] = value
		}
		operation.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-9998"}
		doc.Paths.Set("/api/admin/canonical-fixture", &openapi3.PathItem{Get: &operation})
		inventory.Known["LEGACY-API-9998"] = true
		inventory.Candidates[operation.OperationID] = canonicalCandidateOperation{
			Path:       "/api/admin/canonical-fixture",
			Method:     "GET",
			MappingIDs: []string{"LEGACY-API-9998"},
		}
		return doc, inventory
	}

	t.Run("canonical declaration accepted", func(t *testing.T) {
		doc, inventory := build(t)
		if err := validate(doc, inventory); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missing canonical declaration rejected", func(t *testing.T) {
		doc, inventory := build(t)
		delete(inventory.Candidates, "getCanonicalFixture")
		reject(t, doc, inventory)
	})
	t.Run("canonical path mismatch rejected", func(t *testing.T) {
		doc, inventory := build(t)
		contract := inventory.Candidates["getCanonicalFixture"]
		contract.Path = "/api/admin/forged"
		inventory.Candidates["getCanonicalFixture"] = contract
		reject(t, doc, inventory)
	})
	t.Run("canonical link mismatch rejected", func(t *testing.T) {
		doc, inventory := build(t)
		doc.Paths.Value("/api/admin/canonical-fixture").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0314"}
		reject(t, doc, inventory)
	})
}

func TestOwnerApprovedNativePackageRegistry(t *testing.T) {
	operation := func(contract nativePackageOperation) (*openapi3.PathItem, *openapi3.Operation) {
		op := &openapi3.Operation{
			OperationID: "updateProduct",
			Extensions: map[string]any{
				"x-p4-decision-evidence":      contract.evidence,
				"x-aicrm-capability":          contract.capability,
				"x-aicrm-auth-scheme":         contract.authScheme,
				"x-aicrm-data-classification": contract.classification,
				"x-aicrm-data-source":         contract.dataSource,
				"x-aicrm-external-effect":     "none",
				"x-aicrm-session-bound-csrf":  contract.csrf,
				"x-aicrm-rbac-scopes":         map[string]any{"admin": "global", "ops": "global"},
			},
			Responses: openapi3.NewResponses(openapi3.WithStatus(403, &openapi3.ResponseRef{Value: openapi3.NewResponse()})),
		}
		item := &openapi3.PathItem{Put: op}
		return item, op
	}

	t.Run("authenticated operation accepted", func(t *testing.T) {
		contract := nativePackageOperations["updateProduct"]
		item, op := operation(contract)
		if err := validateNativePackageOperation(contract.path, item, op, contract); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("legacy mapping rejected", func(t *testing.T) {
		contract := nativePackageOperations["updateProduct"]
		item, op := operation(contract)
		op.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0530"}
		if err := validateNativePackageOperation(contract.path, item, op, contract); err == nil {
			t.Fatal("expected native operation with legacy mapping to be rejected")
		}
	})
	t.Run("public POST accepted without broadening generic public routes", func(t *testing.T) {
		contract := nativePackageOperations["submitPublicSurvey"]
		security := openapi3.SecurityRequirements{}
		op := &openapi3.Operation{
			OperationID: "submitPublicSurvey",
			Security:    &security,
			Extensions: map[string]any{
				"x-p4-decision-evidence":      contract.evidence,
				"x-aicrm-capability":          contract.capability,
				"x-aicrm-auth-scheme":         contract.authScheme,
				"x-aicrm-data-classification": contract.classification,
				"x-aicrm-data-source":         contract.dataSource,
				"x-aicrm-external-effect":     "none",
				"x-aicrm-csrf":                contract.csrf,
			},
		}
		item := &openapi3.PathItem{Post: op}
		if err := validateNativePackageOperation(contract.path, item, op, contract); err != nil {
			t.Fatal(err)
		}
		op.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global"}
		if err := validateNativePackageOperation(contract.path, item, op, contract); err == nil {
			t.Fatal("expected public native operation with RBAC scopes to be rejected")
		}
	})
}

func TestReleasePlaneNativePackageRegistryRemainsClosed(t *testing.T) {
	doc, _ := fresh(t)
	operations := map[string]struct {
		path   string
		method string
	}{
		"listReleaseCandidates":      {"/api/v1/admin/release-candidates", "GET"},
		"registerReleaseCandidate":   {"/api/v1/admin/release-candidates", "POST"},
		"getReleaseCandidate":        {"/api/v1/admin/release-candidates/{candidate_id}", "GET"},
		"recordReleasePrerequisite":  {"/api/v1/admin/release-candidates/{candidate_id}/prerequisites", "POST"},
		"prepareReleaseCandidate":    {"/api/v1/admin/release-candidates/{candidate_id}/prepare", "POST"},
		"startReleaseCutover":        {"/api/v1/admin/release-candidates/{candidate_id}/cutover/start", "POST"},
		"restartReleaseCutover":      {"/api/v1/admin/release-candidates/{candidate_id}/cutover/restart", "POST"},
		"completeReleaseCutoverStep": {"/api/v1/admin/release-candidates/{candidate_id}/cutover/steps/{step}/complete", "POST"},
		"activateReleaseCandidate":   {"/api/v1/admin/release-candidates/{candidate_id}/activate", "POST"},
		"recordReleaseRollbackCheck": {"/api/v1/admin/release-candidates/{candidate_id}/rollback-checks", "POST"},
		"requestReleaseRollback":     {"/api/v1/admin/release-candidates/{candidate_id}/rollback/request", "POST"},
		"completeReleaseRollback":    {"/api/v1/admin/release-candidates/{candidate_id}/rollback/complete", "POST"},
	}
	for operationID, want := range operations {
		item := doc.Paths.Value(want.path)
		op := operationForMethod(item, want.method)
		contract, registered := nativePackageOperations[operationID]
		if op == nil || op.OperationID != operationID || !registered {
			t.Fatalf("%s native operation is missing", operationID)
		}
		if contract.evidence != p4ReleasePlaneEvidence {
			t.Fatalf("%s evidence = %q", operationID, contract.evidence)
		}
		if err := validateNativePackageOperation(want.path, item, op, contract); err != nil {
			t.Fatalf("%s: %v", operationID, err)
		}
	}
}

func TestCampaignInitiationTouchPlanContractRemainsClosed(t *testing.T) {
	doc, inventory := fresh(t)
	operations := map[string]struct {
		path   string
		method string
	}{
		"listCloudCampaignTouchPlans":  {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans", "GET"},
		"createCloudCampaignTouchPlan": {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans", "POST"},
		"getCloudCampaignTouchPlan":    {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}", "GET"},
	}
	for operationID, want := range operations {
		item := doc.Paths.Value(want.path)
		op := operationForMethod(item, want.method)
		if op == nil || op.OperationID != operationID {
			t.Fatalf("%s operation is missing", operationID)
		}
		if err := validateNativePackageOperation(want.path, item, op, nativePackageOperations[operationID]); err != nil {
			t.Fatal(err)
		}
	}
	for _, forbidden := range []string{
		"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/approve",
		"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/reject",
		"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/handoff",
	} {
		if doc.Paths.Value(forbidden) != nil {
			t.Fatalf("00067 route leaked into 00066: %s", forbidden)
		}
	}
	requestSource := doc.Components.Schemas["CloudCampaignTouchPlanCreateSource"].Value
	if requestSource == nil || requestSource.Discriminator == nil || requestSource.Discriminator.PropertyName != "kind" || len(requestSource.OneOf) != 3 {
		t.Fatal("touch-plan request source must remain a closed three-way discriminator")
	}
	for _, ref := range requestSource.OneOf {
		if ref == nil || ref.Value == nil || strings.Contains(ref.Ref, "CustomerFilter") {
			t.Fatalf("customer_filter must not be a callable touch-plan source: %#v", ref)
		}
	}
	if _, present := doc.Components.Schemas["CloudCampaignTouchPlanCustomerFilterRequest"]; present {
		t.Fatal("customer_filter request schema must not be exposed")
	}
	detail := doc.Components.Schemas["CloudCampaignTouchPlanDetailResponse"].Value
	if detail == nil {
		t.Fatal("touch-plan detail schema is missing")
	}
	for _, forbidden := range []string{"customer_ids", "recipients", "review_status", "handoff_created"} {
		if _, present := detail.Properties[forbidden]; present {
			t.Fatalf("touch-plan detail leaks deferred field %q", forbidden)
		}
	}
	for _, name := range []string{"CloudCampaignTouchPlanSummary", "CloudCampaignTouchPlanDetailResponse", "CloudCampaignTouchPlanListResponse"} {
		schema := doc.Components.Schemas[name].Value
		if schema == nil || schema.AdditionalProperties.Has == nil || *schema.AdditionalProperties.Has {
			t.Fatalf("%s must remain a closed response", name)
		}
		for field, want := range map[string][]any{
			"local_only":                  {true},
			"provider_execution_eligible": {false},
			"runtime_executed":            {false},
			"real_external_call_executed": {false},
			"delivery_proven":             {false},
		} {
			property := schema.Properties[field]
			if property == nil || property.Value == nil || !containsString(schema.Required, field) || !reflect.DeepEqual(property.Value.Enum, want) {
				t.Fatalf("%s safety field %s drifted", name, field)
			}
		}
	}
	create := doc.Paths.Value(operations["createCloudCampaignTouchPlan"].path).Post
	if create == nil || create.Parameters.GetByInAndName("header", "X-CSRF-Token") == nil || create.Parameters.GetByInAndName("header", "Idempotency-Key") == nil {
		t.Fatal("touch-plan create must retain root CSRF and idempotency headers")
	}
	if err := validateContracts(doc, inventory, false); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledUserOpsPageCarriersRemainAbsent(t *testing.T) {
	doc, _ := fresh(t)
	for _, path := range []string{"/admin/user-ops", "/admin/user-ops/ui"} {
		if doc.Paths.Value(path) != nil {
			t.Fatalf("cancelled User Ops page carrier remains registered: %s", path)
		}
	}
	for _, operationID := range []string{"getUserOpsReviewWorkspace", "getUserOpsReviewWorkspaceAlias"} {
		if _, present := nativePackageOperations[operationID]; present {
			t.Fatalf("cancelled User Ops operation remains registered: %s", operationID)
		}
	}
}

func TestOperationsWorkspaceCarrierRBACRemainsNarrow(t *testing.T) {
	wantOperationsRead := map[string]string{
		"getCloudOrchestratorCampaignsWorkspace": "operations.read",
		"getAudiencePackagesWorkspace":           "operations.read",
		"getAudiencePackageDetailWorkspace":      "operations.read",
	}
	for operationID, capability := range wantOperationsRead {
		contract := nativePackageOperations[operationID]
		if contract.capability != capability || !reflect.DeepEqual(contract.scopes, map[string]string{"admin": "global", "ops": "global"}) {
			t.Fatalf("%s capability/scopes=%q/%v", operationID, contract.capability, contract.scopes)
		}
	}
	for _, operationID := range []string{
		"getCloudOrchestratorWorkspace",
		"getCloudOrchestratorPlansWorkspace",
		"getCloudOrchestratorPlanDetailWorkspace",
		"getCloudOrchestratorObservabilityWorkspace",
		"getGroupOpsPlansWorkspace",
		"getGroupOpsPlanDetailWorkspace",
	} {
		contract := nativePackageOperations[operationID]
		if contract.capability != "admin.read" || !reflect.DeepEqual(contract.scopes, map[string]string{"admin": "global"}) {
			t.Fatalf("%s capability/scopes=%q/%v", operationID, contract.capability, contract.scopes)
		}
	}
}

func TestCloudCampaignDetailRBACMatchesOperationsWorkspace(t *testing.T) {
	contract := authorizationContracts["getCloudCampaign"]
	operationsRead := map[string]string{"admin": "global", "ops": "global"}
	if contract.capability != "operations.read" || !reflect.DeepEqual(contract.scopes, operationsRead) {
		t.Fatalf("getCloudCampaign capability/scopes=%q/%v", contract.capability, contract.scopes)
	}

	doc, inventory := fresh(t)
	operation := doc.Paths.Value("/api/admin/cloud-orchestrator/campaigns/{campaign_code}").Get
	if operation == nil || operation.OperationID != "getCloudCampaign" {
		t.Fatal("getCloudCampaign operation is missing")
	}
	if capability, _ := operation.Extensions["x-aicrm-capability"].(string); capability != "operations.read" {
		t.Fatalf("getCloudCampaign OAS capability=%q", capability)
	}
	if scopes, err := stringMap(operation.Extensions["x-aicrm-rbac-scopes"]); err != nil || !reflect.DeepEqual(scopes, operationsRead) {
		t.Fatalf("getCloudCampaign OAS scopes=%v err=%v", scopes, err)
	}
	if err := validateContracts(doc, inventory, false); err != nil {
		t.Fatal(err)
	}

	operation.Extensions["x-aicrm-capability"] = "admin.read"
	operation.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global"}
	reject(t, doc, inventory)
}

func TestCloudCampaignWorkspaceLaunchQueryRemainsLosslessAndClosed(t *testing.T) {
	doc, _ := fresh(t)
	operation := doc.Paths.Value("/admin/cloud-orchestrator/campaigns").Get
	if operation == nil || len(operation.Parameters) != 2 {
		t.Fatalf("campaign carrier parameters=%v", operation)
	}
	kind := operation.Parameters.GetByInAndName("query", "source_kind")
	id := operation.Parameters.GetByInAndName("query", "source_id")
	if kind == nil || kind.Required || kind.Schema == nil || kind.Schema.Value == nil ||
		kind.Schema.Value.Type == nil || !kind.Schema.Value.Type.Is("string") ||
		!reflect.DeepEqual(kind.Schema.Value.Enum, []any{"customer_selection", "segment_members", "ai_audience_package_members"}) {
		t.Fatalf("source_kind=%#v", kind)
	}
	if id == nil || id.Required || id.Schema == nil || id.Schema.Value == nil ||
		id.Schema.Value.Type == nil || !id.Schema.Value.Type.Is("string") || id.Schema.Value.Format != "" ||
		id.Schema.Value.Pattern != "^[1-9][0-9]{0,18}$" || id.Schema.Value.MaxLength == nil || *id.Schema.Value.MaxLength != 19 ||
		id.Schema.Value.Min != nil || id.Schema.Value.Max != nil ||
		id.Schema.Value.Extensions["x-aicrm-decimal-maximum"] != "9223372036854775807" ||
		operation.Extensions["x-aicrm-query-contract"] != "none_or_exact_source_pair" {
		t.Fatalf("source_id=%#v", id)
	}
	redirect := operation.Responses.Value("302")
	if redirect == nil || redirect.Value == nil {
		t.Fatal("campaign carrier redirect response is missing")
	}
	location := redirect.Value.Headers["Location"]
	if location == nil || location.Value == nil || location.Value.Schema == nil || location.Value.Schema.Value == nil || len(location.Value.Schema.Value.OneOf) != 2 {
		t.Fatalf("Location=%#v", location)
	}
	pattern := regexp.MustCompile(location.Value.Schema.Value.OneOf[1].Value.Pattern)
	for _, sourceID := range []string{"1", "999999999999999999", "1000000000000000000", "9223372036854775807"} {
		value := "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns%3Fsource_kind%3Dsegment_members%26source_id%3D" + sourceID
		if !pattern.MatchString(value) {
			t.Fatalf("Location pattern rejected source_id=%s", sourceID)
		}
	}
	for _, sourceID := range []string{"0", "01", "9223372036854775808", "9999999999999999999", "10000000000000000000"} {
		value := "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns%3Fsource_kind%3Dsegment_members%26source_id%3D" + sourceID
		if pattern.MatchString(value) {
			t.Fatalf("Location pattern accepted source_id=%s", sourceID)
		}
	}
	if operation.Responses.Value("400") == nil {
		t.Fatal("campaign carrier lacks malformed-query response")
	}
}

func TestCloudCampaignWorkspaceLaunchQueryRegistryRejectsDrift(t *testing.T) {
	for name, mutate := range map[string]func(*openapi3.Operation){
		"source id becomes number": func(operation *openapi3.Operation) {
			parameter := operation.Parameters.GetByInAndName("query", "source_id")
			parameter.Schema.Value.Type = &openapi3.Types{"integer"}
			parameter.Schema.Value.Format = "int64"
		},
		"source kind broadens": func(operation *openapi3.Operation) {
			operation.Parameters.GetByInAndName("query", "source_kind").Schema.Value.Enum = append(
				operation.Parameters.GetByInAndName("query", "source_kind").Schema.Value.Enum,
				"customer_filter",
			)
		},
		"extra query appears": func(operation *openapi3.Operation) {
			operation.Parameters = append(operation.Parameters, &openapi3.ParameterRef{Value: &openapi3.Parameter{
				Name: "return_to", In: "query", Schema: &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
			}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc, ids := fresh(t)
			mutate(doc.Paths.Value("/admin/cloud-orchestrator/campaigns").Get)
			reject(t, doc, ids)
		})
	}
}

func TestCampaignReviewHandoffContractRemainsSeparateFromInitiation(t *testing.T) {
	doc, inventory := fresh(t)
	operations := map[string]struct{ path, method string }{
		"listCloudCampaignTouchPlanRecipients": {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/recipients", "GET"},
		"getCloudCampaignTouchPlanRecipient":   {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/recipients/{customer_id}", "GET"},
		"getCloudCampaignTouchPlanReview":      {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/review", "GET"},
		"mutateCloudCampaignTouchPlanReview":   {"/api/admin/cloud-orchestrator/campaigns/{campaign_code}/touch-plans/{plan_id}/review/{operation}", "POST"},
	}
	for operationID, want := range operations {
		item := doc.Paths.Value(want.path)
		op := operationForMethod(item, want.method)
		if op == nil || op.OperationID != operationID {
			t.Fatalf("%s operation is missing", operationID)
		}
		if err := validateNativePackageOperation(want.path, item, op, nativePackageOperations[operationID]); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateContracts(doc, inventory, false); err != nil {
		t.Fatal(err)
	}
}

func TestGroupOpsPlanIDRemainsALosslessDecimalString(t *testing.T) {
	for name, mutate := range map[string]func(*openapi3.Schema){
		"integer type": func(schema *openapi3.Schema) {
			schema.Type = &openapi3.Types{"integer"}
			schema.Format = "int64"
		},
		"broader pattern": func(schema *openapi3.Schema) {
			schema.Pattern = "^[0-9]+$"
		},
		"longer value": func(schema *openapi3.Schema) {
			maximum := uint64(20)
			schema.MaxLength = &maximum
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc, ids := fresh(t)
			item := doc.Paths.Value("/admin/automation-conversion/group-ops/plans/{plan_id}")
			parameter := item.Parameters.GetByInAndName("path", "plan_id")
			if parameter == nil || parameter.Schema == nil || parameter.Schema.Value == nil {
				t.Fatal("group operations plan_id parameter is missing")
			}
			mutate(parameter.Schema.Value)
			reject(t, doc, ids)
		})
	}
}

func TestRejectsUnsafeContractMutations(t *testing.T) {
	tests := map[string]func(*testing.T){
		"canonical public route becomes authenticated": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/system/health").Get.Security = &openapi3.SecurityRequirements{{"AdminSession": []string{}}}
			reject(t, doc, ids)
		},
		"canonical public route forges RBAC scopes": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/system/health").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global"}
			reject(t, doc, ids)
		},
		"canonical public route loses non PII classification": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/system/health").Get.Extensions, "x-aicrm-data-classification")
			reject(t, doc, ids)
		},
		"signoff regression": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Extensions["x-p1-signoff-status"] = "PENDING_HUMAN_SIGNOFF"
			reject(t, doc, ids)
		},
		"forged decision evidence": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Extensions["x-p1-decision-evidence"] = "G1-D01-FORGED"
			reject(t, doc, ids)
		},
		"unknown legacy link": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-9999"}
			reject(t, doc, ids)
		},
		"identity in customer": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["Customer"].Value.Properties["external_userid"] = doc.Components.Schemas["HealthResponse"]
			reject(t, doc, ids)
		},
		"unscoped identity": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["IdentityRef"].Value.Required = []string{"type", "value", "assurance", "source"}
			reject(t, doc, ids)
		},
		"missing operation": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Delete("/api/v1/admin/config/overview")
			reject(t, doc, ids)
		},
		"domain verification becomes authenticated": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/{filename}").Get.Security = &openapi3.SecurityRequirements{{"AdminSession": []string{}}}
			reject(t, doc, ids)
		},
		"domain verification filename widens": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/{filename}").Get.Parameters[0].Value.Schema.Value.Pattern = ".*"
			reject(t, doc, ids)
		},
		"domain verification loses no-store": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/{filename}").Get.Responses.Value("200").Value.Headers, "Cache-Control")
			reject(t, doc, ids)
		},
		"legacy health capability swapped to dotted form": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/health").Get.Extensions["x-aicrm-capability"] = "health.read"
			reject(t, doc, ids)
		},
		"legacy health becomes authenticated": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/health").Get.Security = &openapi3.SecurityRequirements{{"AdminSession": []string{}}}
			reject(t, doc, ids)
		},
		"legacy health forges RBAC scopes": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/health").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global"}
			reject(t, doc, ids)
		},
		"legacy health evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/health").Get.Extensions["x-p4-decision-evidence"] = "P4-S04-LEGACY-HEALTH-FORGED"
			reject(t, doc, ids)
		},
		"legacy health linked to the wrong mapping": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/health").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0741"}
			reject(t, doc, ids)
		},
		"legacy health claims an external effect": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/health").Get.Extensions["x-aicrm-external-effect"] = "provider"
			reject(t, doc, ids)
		},
		"legacy health loses the exact method error": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/health").Get.Responses.Delete("405")
			reject(t, doc, ids)
		},
		"legacy health snapshot widens a field": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyRuntimeHealthSnapshot"].Value.Properties["debug"] = doc.Components.Schemas["HealthResponse"]
			reject(t, doc, ids)
		},
		"legacy health warning enum widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyRuntimeHealthSnapshot"].Value.Properties["warning"].Value.Enum = []any{"", "fixture data mode", "production runtime is using fixture data; production data is not ready", "everything is fine"}
			reject(t, doc, ids)
		},
		"push center evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/push-center/stats").Get.Extensions["x-p4-decision-evidence"] = "P4-PUSH-CENTER-FORGED"
			reject(t, doc, ids)
		},
		"push center scope widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/push-center/sections").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global", "ops": "global"}
			reject(t, doc, ids)
		},
		"push center external effect injected": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/push-center/sections").Get.Extensions["x-aicrm-external-effect"] = "provider"
			reject(t, doc, ids)
		},
		"outbound cancellation loses its closed local receipt": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/push-center/jobs/{job_id}/cancel").Post.Responses.Delete("202")
			reject(t, doc, ids)
		},
		"outbound cancellation forges a legacy mapping": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/push-center/jobs/{job_id}/cancel").Post.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0416"}
			reject(t, doc, ids)
		},
		"outbound job exposes a provider receipt object": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyOutboundJob"].Value.Properties["provider_receipt"] = doc.Components.Schemas["LegacyOutboundQueueJob"]
			reject(t, doc, ids)
		},
		"outbound job claims delivery proof": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyOutboundJob"].Value.Properties["delivery_proven"].Value.Enum = []any{true}
			reject(t, doc, ids)
		},
		"outbound failure class widens to provider text": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyOutboundAttempt"].Value.Properties["failure_class"].Value.Enum = append(doc.Components.Schemas["LegacyOutboundAttempt"].Value.Properties["failure_class"].Value.Enum, "provider_raw")
			reject(t, doc, ids)
		},
		"outbound retry loses its closed local receipt": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/push-center/jobs/{job_id}/retry").Post.Responses.Delete("202")
			reject(t, doc, ids)
		},
		"outbound detail widens sales scope": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/push-center/jobs/{job_id}").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global", "ops": "global", "sales": "global"}
			reject(t, doc, ids)
		},
		"browser JWT substitution": func(t *testing.T) {
			doc, ids := fresh(t)
			scheme := doc.Components.SecuritySchemes["AdminSession"].Value
			scheme.Type, scheme.In, scheme.Name, scheme.Scheme, scheme.BearerFormat = "http", "", "", "bearer", "JWT"
			reject(t, doc, ids)
		},
		"logout without CSRF": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/auth/logout").Post.Parameters = nil
			reject(t, doc, ids)
		},
		"logout without CSRF failure response": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/auth/logout").Post.Responses.Delete("403")
			reject(t, doc, ids)
		},
		"missing operation capability": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/v1/customers").Get.Extensions, "x-aicrm-capability")
			reject(t, doc, ids)
		},
		"sales customer scope widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
				"admin": "global", "ops": "global", "sales": "global",
			}
			reject(t, doc, ids)
		},
		"config granted to ops": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/admin/config/overview").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
				"admin": "global", "ops": "global",
			}
			reject(t, doc, ids)
		},
		"settings manage granted to ops": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/config/app-settings").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global", "ops": "global"}
			reject(t, doc, ids)
		},
		"admin shell evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/admin").Get.Extensions["x-p4-decision-evidence"] = "P4-ADMIN-SHELL-FORGED"
			reject(t, doc, ids)
		},
		"admin shell grants sales": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/admin").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global", "ops": "global", "sales": "global"}
			reject(t, doc, ids)
		},
		"admin shell logout loses redirect": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/admin/logout").Get.Responses.Delete("302")
			reject(t, doc, ids)
		},
		"admin shell denial claims external success": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["AdminShellAccessDenied"].Value.Properties["real_external_call_executed"].Value.Enum = []any{true}
			reject(t, doc, ids)
		},
		"JSON settings resource write missing": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/config/app-settings").Put = nil
			reject(t, doc, ids)
		},
		"JSON settings resource action token missing": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/admin/config/app-settings").Put.Extensions, "x-aicrm-route-bound-action-token")
			reject(t, doc, ids)
		},
		"secret value exposed": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyMaskedAppSetting"].Value.Properties["value"] = doc.Components.Schemas["AdminConfigEntry"]
			reject(t, doc, ids)
		},
		"secret form accepts replacement": func(t *testing.T) {
			doc, ids := fresh(t)
			maximum := uint64(100)
			doc.Components.Schemas["LegacyAppSettingsSaveForm"].Value.Properties["setting__database.url"].Value.MaxLength = &maximum
			reject(t, doc, ids)
		},
		"settings version derives audit state": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyEditableAppSetting"].Value.Properties["version"].Value.Enum = []any{"1"}
			reject(t, doc, ids)
		},
		"role denial without forbidden response": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/identity/resolve").Post.Responses.Delete("403")
			reject(t, doc, ids)
		},
		"missing stage evidence": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/v1/stages").Get.Extensions, "x-p2-decision-evidence")
			reject(t, doc, ids)
		},
		"sales stage write widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/stages").Post.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
				"admin": "global", "ops": "global", "sales": "global",
			}
			reject(t, doc, ids)
		},
		"stage write without csrf": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/stages/{stage_id}").Patch.Parameters = nil
			reject(t, doc, ids)
		},
		"missing contact evidence": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/v1/customers").Get.Extensions, "x-p3-decision-evidence")
			reject(t, doc, ids)
		},
		"unknown contact legacy link": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/tags").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-9999"}
			reject(t, doc, ids)
		},
		"sales tag scope widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/tags").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
				"admin": "global", "ops": "global", "sales": "global",
			}
			reject(t, doc, ids)
		},
		"tag dependency response removed": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/tags").Get.Responses.Delete("503")
			reject(t, doc, ids)
		},
		"automation capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/automation-conversion/agent-runs").Get.Extensions["x-aicrm-capability"] = "automation.write"
			reject(t, doc, ids)
		},
		"automation evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/automation-conversion/agent-runs").Get.Extensions["x-p4-decision-evidence"] = "P4-W0-D01-FORGED"
			reject(t, doc, ids)
		},
		"automation identity filter removed": func(t *testing.T) {
			doc, ids := fresh(t)
			operation := doc.Paths.Value("/api/admin/automation-conversion/agent-runs").Get
			operation.Parameters = operation.Parameters[:7]
			reject(t, doc, ids)
		},
		"product evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/products").Get.Extensions["x-p4-decision-evidence"] = "P4-I01A-FORGED"
			reject(t, doc, ids)
		},
		"product legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/products/{product_id}").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0526"}
			reject(t, doc, ids)
		},
		"product capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/products").Get.Extensions["x-aicrm-capability"] = "products.write"
			reject(t, doc, ids)
		},
		"media evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/image-library/upload").Post.Extensions["x-p4-decision-evidence"] = "P4-H01A1-FORGED"
			reject(t, doc, ids)
		},
		"media legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/image-library/upload").Post.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0362"}
			reject(t, doc, ids)
		},
		"media capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/image-library/upload").Post.Extensions["x-aicrm-capability"] = "media.images.read"
			reject(t, doc, ids)
		},
		"group invite evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/group-invite-library").Get.Extensions["x-p4-decision-evidence"] = "P4-H03-FORGED"
			reject(t, doc, ids)
		},
		"group invite legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/group-invite-library/{item_id}").Delete.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0338"}
			reject(t, doc, ids)
		},
		"group invite capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/group-invite-library").Get.Extensions["x-aicrm-capability"] = "media.library.write"
			reject(t, doc, ids)
		},
		"order evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/orders").Get.Extensions["x-p4-decision-evidence"] = "P4-I03-FORGED"
			reject(t, doc, ids)
		},
		"order legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/orders").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0406"}
			reject(t, doc, ids)
		},
		"order capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/orders").Get.Extensions["x-aicrm-capability"] = "order.write"
			reject(t, doc, ids)
		},
		"survey evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires").Get.Extensions["x-p4-decision-evidence"] = "P4-F01A-FORGED"
			reject(t, doc, ids)
		},
		"survey legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0424"}
			reject(t, doc, ids)
		},
		"survey capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires").Get.Extensions["x-aicrm-capability"] = "questionnaires.write"
			reject(t, doc, ids)
		},
		"survey assessment enabled": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyQuestionnaireCreateRequest"].Value.Properties["assessment_enabled"].Value.Enum = nil
			reject(t, doc, ids)
		},
		"survey F01B evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}").Patch.Extensions["x-p4-decision-evidence"] = "P4-F01AB-FORGED"
			reject(t, doc, ids)
		},
		"survey F01B legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/duplicate").Post.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0430"}
			reject(t, doc, ids)
		},
		"survey F01B CSRF removed": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/enable").Post.Parameters = nil
			reject(t, doc, ids)
		},
		"channel evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/channels").Get.Extensions["x-p4-decision-evidence"] = "P4-C01-FORGED"
			reject(t, doc, ids)
		},
		"channel legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/channels/{channel_id}").Patch.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0195"}
			reject(t, doc, ids)
		},
		"channel capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/channels").Get.Extensions["x-aicrm-capability"] = "channels.write"
			reject(t, doc, ids)
		},
		"channel request opened": func(t *testing.T) {
			doc, ids := fresh(t)
			opened := true
			doc.Components.Schemas["LegacyChannelWriteRequest"].Value.AdditionalProperties.Has = &opened
			reject(t, doc, ids)
		},
		"coupon A+B evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/h5/coupons/available").Get.Extensions["x-p4-decision-evidence"] = "P4-COUPON-AB-FORGED"
			reject(t, doc, ids)
		},
		"coupon H5 claim lacks idempotency": func(t *testing.T) {
			doc, ids := fresh(t)
			removeRequiredHeader(t, doc.Paths.Value("/api/h5/coupons/{public_slug}/claim").Post, "Idempotency-Key")
			reject(t, doc, ids)
		},
		"coupon H5 claim accepts public auth": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/h5/coupons/{public_slug}/claim").Post.Extensions["x-aicrm-auth-scheme"] = "public"
			reject(t, doc, ids)
		},
		"coupon claim leaks customer ID": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyCouponClaim"].Value.Properties["customer_id"] = doc.Components.Schemas["HealthResponse"]
			reject(t, doc, ids)
		},
		"offset pagination reintroduced": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Parameters[0].Value.Name = "offset"
			reject(t, doc, ids)
		},
		"estimated total removed": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["CustomerListResponse"].Value.Required = []string{"items", "next_cursor", "total", "watermark"}
			reject(t, doc, ids)
		},
		"stage smuggled into profile update": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["CustomerUpdateRequest"].Value.Properties["stage_id"] = doc.Components.Schemas["HealthResponse"]
			reject(t, doc, ids)
		},
		"event actor became optional": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["CustomerEvent"].Value.Required = []string{"id", "customer_id", "event_type", "payload", "occurred_at"}
			reject(t, doc, ids)
		},
		"customer tag write without csrf": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers/{customer_id}/tags/{tag_id}").Put.Parameters = nil
			reject(t, doc, ids)
		},
		"tag A+B evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/wecom/tags/sync").Post.Extensions["x-p4-decision-evidence"] = "P4-B02AB-FORGED"
			reject(t, doc, ids)
		},
		"tag A+B mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/wecom/tags/live/mark").Post.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0559"}
			reject(t, doc, ids)
		},
		"tag A+B queue capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/wecom/tags/live/unmark").Post.Extensions["x-aicrm-capability"] = "customers.read"
			reject(t, doc, ids)
		},
		"tag A+B queue without csrf": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/wecom/tags/sync-due").Post.Parameters = nil
			reject(t, doc, ids)
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			test(t)
		})
	}
}

func TestRejectsP3IdentityContractMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *openapi3.T)
	}{
		{
			name: "resolve discriminator mapping missing",
			mutate: func(t *testing.T, doc *openapi3.T) {
				delete(doc.Components.Schemas["ResolveIdentityResponse"].Value.Discriminator.Mapping, "not_found")
			},
		},
		{
			name: "bind discriminator mapping points to wrong variant",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["BindIdentityResponse"].Value.Discriminator.Mapping["merged"] =
					"#/components/schemas/BindIdentityBound"
			},
		},
		{
			name: "admin identity ref admits assurance",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityRef"].Value.Properties["assurance"] = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "admin identity ref admits source",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityRef"].Value.Properties["source"] = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "admin identity ref admits confidence alias",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityRef"].Value.Properties["confidence"] =
					doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "admin identity kind admits namespaced legacy kind",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityRef"].Value.Properties["type"].Value.Enum =
					append(doc.Components.Schemas["IdentityRef"].Value.Properties["type"].Value.Enum, "ext:legacy")
			},
		},
		{
			name: "review fingerprint permits unkeyed digest",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["identity_fingerprint"].Value.Pattern =
					`^[a-f0-9]{64}$`
			},
		},
		{
			name: "review fingerprint permits non canonical base64url tail",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["identity_fingerprint"].Value.Pattern =
					`^hmac-sha256-v[1-9][0-9]*:[A-Za-z0-9_-]{22}$`
			},
		},
		{
			name: "review exposes raw identity alias",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["raw_identity"] =
					doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review page admits unknown fields",
			mutate: func(t *testing.T, doc *openapi3.T) {
				allowed := true
				doc.Components.Schemas["IdentityMergeReviewPage"].Value.AdditionalProperties.Has = &allowed
			},
		},
		{
			name: "resolve response ref drifts",
			mutate: func(t *testing.T, doc *openapi3.T) {
				response := doc.Paths.Value("/api/v1/identity/resolve").Post.Responses.Value("200")
				response.Value.Content["application/json"].Schema = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "bind request ref drifts",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/bind").Post.RequestBody.Value.Content["application/json"].Schema =
					doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review list response ref drifts",
			mutate: func(t *testing.T, doc *openapi3.T) {
				response := doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Responses.Value("200")
				response.Value.Content["application/json"].Schema = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review page item ref drifts",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReviewPage"].Value.Properties["items"].Value.Items =
					doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review page cursor stops being nullable",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReviewPage"].Value.Properties["next_cursor"].Value.Nullable = false
			},
		},
		{
			name: "review admits more than two roots",
			mutate: func(t *testing.T, doc *openapi3.T) {
				max := uint64(3)
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["customer_ids"].Value.MaxItems = &max
			},
		},
		{
			name: "bind lacks csrf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/bind").Post, "X-CSRF-Token")
			},
		},
		{
			name: "bind lacks idempotency key",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/bind").Post, "Idempotency-Key")
			},
		},
		{
			name: "ingest lacks csrf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/ingest").Post, "X-CSRF-Token")
			},
		},
		{
			name: "ingest lacks idempotency key",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/ingest").Post, "Idempotency-Key")
			},
		},
		{
			name: "approve review lacks csrf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve").Post, "X-CSRF-Token")
			},
		},
		{
			name: "approve review lacks idempotency key",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve").Post, "Idempotency-Key")
			},
		},
		{
			name: "reject review lacks csrf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject").Post, "X-CSRF-Token")
			},
		},
		{
			name: "reject review lacks idempotency key",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject").Post, "Idempotency-Key")
			},
		},
		{
			name: "review list operation missing",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Delete("/api/v1/identity/merge-reviews")
			},
		},
		{
			name: "review approval operation missing",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Delete("/api/v1/identity/merge-reviews/{review_id}/approve")
			},
		},
		{
			name: "review rejection operation missing",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Delete("/api/v1/identity/merge-reviews/{review_id}/reject")
			},
		},
		{
			name: "review list lacks unavailable response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Responses.Delete("503")
			},
		},
		{
			name: "identity resolve lacks semantic validation response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/resolve").Post.Responses.Delete("422")
			},
		},
		{
			name: "review approval lacks unavailable response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve").Post.Responses.Delete("503")
			},
		},
		{
			name: "review rejection lacks unavailable response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject").Post.Responses.Delete("503")
			},
		},
		{
			name: "review approval lacks conflict response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve").Post.Responses.Delete("409")
			},
		},
		{
			name: "review rejection lacks conflict response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject").Post.Responses.Delete("409")
			},
		},
		{
			name: "review leaks raw identity value",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["value"] = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review leaks normalized identity value",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["normalized_value"] = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review omits explicit resolved timestamp",
			mutate: func(t *testing.T, doc *openapi3.T) {
				review := doc.Components.Schemas["IdentityMergeReview"].Value
				required := make([]string, 0, len(review.Required))
				for _, field := range review.Required {
					if field != "resolved_at" {
						required = append(required, field)
					}
				}
				review.Required = required
			},
		},
		{
			name: "forged P3 identity evidence",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Extensions["x-p3-decision-evidence"] = "P3-I00-FORGED"
			},
		},
		{
			name: "review capability drift",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Extensions["x-aicrm-capability"] = "identity.review.write"
			},
		},
		{
			name: "review RBAC widened to sales",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
					"admin": "global", "ops": "global", "sales": "global",
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			doc, ids := fresh(t)
			test.mutate(t, doc)
			reject(t, doc, ids)
		})
	}
}

func TestP3IdentityStatusFieldMatricesAreFrozen(t *testing.T) {
	doc, _ := fresh(t)
	assertStatusFieldMatrix(t, "resolveIdentity", successResponseSchema(t, doc, "/api/v1/identity/resolve"), map[string][]string{
		"found":     {"customer_id"},
		"not_found": {},
		"conflict":  {},
	})
	assertStatusFieldMatrix(t, "bindIdentity", successResponseSchema(t, doc, "/api/v1/identity/bind"), map[string][]string{
		"bound":         {"customer_id"},
		"already_bound": {"customer_id"},
		"merged":        {"customer_id", "primary_customer_id", "merge_audit_id"},
		"manual_review": {"review_id"},
		"rejected":      {},
	})
	assertStatusFieldMatrix(t, "ingestIdentityEvent", successResponseSchema(t, doc, "/api/v1/identity/ingest"), map[string][]string{
		"attributed": {"customer_id", "event_id"},
		"pending":    {"pending_event_id"},
		"conflict":   {"pending_event_id"},
	})
}

func TestRejectsP3IdentityStatusFieldMatrixMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *openapi3.T)
	}{
		{
			name: "resolve response no longer uses oneOf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				successResponseSchema(t, doc, "/api/v1/identity/resolve").OneOf = nil
			},
		},
		{
			name: "resolve response loses status discriminator",
			mutate: func(t *testing.T, doc *openapi3.T) {
				successResponseSchema(t, doc, "/api/v1/identity/resolve").Discriminator = nil
			},
		},
		{
			name: "bind manual review admits customer ID",
			mutate: func(t *testing.T, doc *openapi3.T) {
				branch := statusBranch(t, successResponseSchema(t, doc, "/api/v1/identity/bind"), "manual_review")
				branch.Properties["customer_id"] = doc.Components.Schemas["HealthResponse"]
				branch.Required = append(branch.Required, "customer_id")
			},
		},
		{
			name: "ingest pending admits customer ID",
			mutate: func(t *testing.T, doc *openapi3.T) {
				branch := statusBranch(t, successResponseSchema(t, doc, "/api/v1/identity/ingest"), "pending")
				branch.Properties["customer_id"] = doc.Components.Schemas["HealthResponse"]
				branch.Required = append(branch.Required, "customer_id")
			},
		},
		{
			name: "resolve branch admits unknown fields",
			mutate: func(t *testing.T, doc *openapi3.T) {
				allowed := true
				branch := statusBranch(t, successResponseSchema(t, doc, "/api/v1/identity/resolve"), "found")
				branch.AdditionalProperties.Has = &allowed
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, ids := fresh(t)
			test.mutate(t, doc)
			reject(t, doc, ids)
		})
	}
}

func fresh(t *testing.T) (*openapi3.T, mappingInventory) {
	t.Helper()
	freshOnce.Do(func() {
		doc, inventory, err := load(specPath, mappingPath)
		if err != nil {
			freshErr = err
			return
		}
		freshDocument = doc
		freshInventory = inventory
	})
	if freshErr != nil {
		t.Fatal(freshErr)
	}
	return cloneOpenAPIDocument(freshDocument), cloneMappingInventory(freshInventory)
}

func cloneMappingInventory(source mappingInventory) mappingInventory {
	clone := mappingInventory{
		Known:      make(map[string]bool, len(source.Known)),
		Candidates: make(map[string]canonicalCandidateOperation, len(source.Candidates)),
	}
	for id, known := range source.Known {
		clone.Known[id] = known
	}
	for operationID, candidate := range source.Candidates {
		candidate.MappingIDs = append([]string(nil), candidate.MappingIDs...)
		clone.Candidates[operationID] = candidate
	}
	return clone
}
func reject(t *testing.T, doc *openapi3.T, ids mappingInventory) {
	t.Helper()
	// TestFrozenOpenAPI performs the full structural OpenAPI validation once.
	// Negative mutation cases exercise the repository's frozen business
	// contracts without repeating that unchanged full traversal per subtest.
	if err := validateContracts(doc, ids, false); err == nil {
		t.Fatal("mutation was accepted")
	}
}

func removeRequiredHeader(t *testing.T, operation *openapi3.Operation, name string) {
	t.Helper()
	if operation == nil {
		t.Fatal("operation is missing")
	}
	parameters := make(openapi3.Parameters, 0, len(operation.Parameters))
	removed := false
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == "header" && ref.Value.Name == name {
			removed = true
			continue
		}
		parameters = append(parameters, ref)
	}
	if !removed {
		t.Fatalf("required header %q is missing before mutation", name)
	}
	operation.Parameters = parameters
}

func successResponseSchema(t *testing.T, doc *openapi3.T, path string) *openapi3.Schema {
	t.Helper()
	item := doc.Paths.Value(path)
	if item == nil || item.Post == nil {
		t.Fatalf("%s POST operation is missing", path)
	}
	response := item.Post.Responses.Value("200")
	if response == nil || response.Value == nil {
		t.Fatalf("%s success response is missing", path)
	}
	media := response.Value.Content["application/json"]
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatalf("%s success schema is missing", path)
	}
	return media.Schema.Value
}

func assertStatusFieldMatrix(t *testing.T, operation string, schema *openapi3.Schema, want map[string][]string) {
	t.Helper()
	if len(schema.OneOf) != len(want) {
		t.Fatalf("%s oneOf branches = %d; want %d", operation, len(schema.OneOf), len(want))
	}
	if schema.Discriminator == nil || schema.Discriminator.PropertyName != "status" {
		t.Fatalf("%s response must discriminate oneOf branches by status", operation)
	}
	if len(schema.Properties) != 0 || len(schema.Required) != 0 {
		t.Fatalf("%s response must expose status fields only through oneOf branches", operation)
	}
	seen := make(map[string]bool, len(schema.OneOf))
	for _, ref := range schema.OneOf {
		if ref == nil || ref.Value == nil {
			t.Fatalf("%s has an unresolved oneOf branch", operation)
		}
		if !strings.HasPrefix(ref.Ref, "#/components/schemas/") {
			t.Fatalf("%s has an inline or external oneOf branch", operation)
		}
		branch := ref.Value
		if branch.Type == nil || !branch.Type.Is("object") || branch.AdditionalProperties.Has == nil || *branch.AdditionalProperties.Has {
			t.Fatalf("%s response branch must be a closed object", operation)
		}
		status, ok := branchStatus(branch)
		if !ok {
			t.Fatalf("%s response branch lacks a singleton status enum", operation)
		}
		expectedFields, knownStatus := want[status]
		if !knownStatus || seen[status] {
			t.Fatalf("%s has unexpected or duplicate status branch %q", operation, status)
		}
		seen[status] = true
		wantFields := append([]string{"status"}, expectedFields...)
		if !sameStringSet(schemaFieldNames(branch.Properties), wantFields) || !sameStringSet(branch.Required, wantFields) {
			t.Fatalf("%s status %q fields=%v required=%v; want %v", operation, status, schemaFieldNames(branch.Properties), branch.Required, wantFields)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("%s status branches=%v; want %v", operation, seen, want)
	}
}

func statusBranch(t *testing.T, schema *openapi3.Schema, wanted string) *openapi3.Schema {
	t.Helper()
	for _, ref := range schema.OneOf {
		if ref != nil && ref.Value != nil {
			if status, ok := branchStatus(ref.Value); ok && status == wanted {
				return ref.Value
			}
		}
	}
	t.Fatalf("status branch %q is missing", wanted)
	return nil
}

func branchStatus(schema *openapi3.Schema) (string, bool) {
	status := schema.Properties["status"]
	if status == nil || status.Value == nil || len(status.Value.Enum) != 1 {
		return "", false
	}
	value, ok := status.Value.Enum[0].(string)
	return value, ok && value != ""
}

func schemaFieldNames(properties openapi3.Schemas) []string {
	fields := make([]string, 0, len(properties))
	for field := range properties {
		fields = append(fields, field)
	}
	return fields
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	values := make(map[string]int, len(got))
	for _, value := range got {
		values[value]++
	}
	for _, value := range want {
		values[value]--
	}
	for _, count := range values {
		if count != 0 {
			return false
		}
	}
	return true
}
