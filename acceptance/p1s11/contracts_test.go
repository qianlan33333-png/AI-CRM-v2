package p1s11_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	runtimegenerated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestGeneratedIdentityStatusUnionsUseWireDiscriminators(t *testing.T) {
	assert := func(t *testing.T, want string, response any, decoded any, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatal(err)
		}
		if object["status"] != want || decoded == nil {
			t.Fatalf("generated union status/decoded = %v/%T; want %q/non-nil", object["status"], decoded, want)
		}
	}
	resolve := []struct {
		status string
		build  func() (generated.ResolveIdentityResponse, error)
	}{
		{"found", func() (response generated.ResolveIdentityResponse, err error) {
			err = response.FromResolveIdentityFound(generated.ResolveIdentityFound{CustomerId: 1})
			return
		}},
		{"not_found", func() (response generated.ResolveIdentityResponse, err error) {
			err = response.FromResolveIdentityNotFound(generated.ResolveIdentityNotFound{})
			return
		}},
		{"conflict", func() (response generated.ResolveIdentityResponse, err error) {
			err = response.FromResolveIdentityConflict(generated.ResolveIdentityConflict{})
			return
		}},
	}
	for _, test := range resolve {
		response, err := test.build()
		if err != nil {
			t.Fatal(err)
		}
		decoded, decodeErr := response.ValueByDiscriminator()
		assert(t, test.status, response, decoded, decodeErr)
	}
	bind := []struct {
		status string
		build  func() (generated.BindIdentityResponse, error)
	}{
		{"bound", func() (response generated.BindIdentityResponse, err error) {
			err = response.FromBindIdentityBound(generated.BindIdentityBound{CustomerId: 1})
			return
		}},
		{"already_bound", func() (response generated.BindIdentityResponse, err error) {
			err = response.FromBindIdentityAlreadyBound(generated.BindIdentityAlreadyBound{CustomerId: 1})
			return
		}},
		{"merged", func() (response generated.BindIdentityResponse, err error) {
			err = response.FromBindIdentityMerged(generated.BindIdentityMerged{CustomerId: 1, PrimaryCustomerId: 1, MergeAuditId: 2})
			return
		}},
		{"manual_review", func() (response generated.BindIdentityResponse, err error) {
			err = response.FromBindIdentityManualReview(generated.BindIdentityManualReview{ReviewId: 1})
			return
		}},
		{"rejected", func() (response generated.BindIdentityResponse, err error) {
			err = response.FromBindIdentityRejected(generated.BindIdentityRejected{})
			return
		}},
	}
	for _, test := range bind {
		response, err := test.build()
		if err != nil {
			t.Fatal(err)
		}
		decoded, decodeErr := response.ValueByDiscriminator()
		assert(t, test.status, response, decoded, decodeErr)
	}
	ingest := []struct {
		status string
		build  func() (generated.IngestIdentityEventResponse, error)
	}{
		{"attributed", func() (response generated.IngestIdentityEventResponse, err error) {
			err = response.FromIngestIdentityEventAttributed(generated.IngestIdentityEventAttributed{CustomerId: 1, EventId: 2})
			return
		}},
		{"pending", func() (response generated.IngestIdentityEventResponse, err error) {
			err = response.FromIngestIdentityEventPending(generated.IngestIdentityEventPending{PendingEventId: 1})
			return
		}},
		{"conflict", func() (response generated.IngestIdentityEventResponse, err error) {
			err = response.FromIngestIdentityEventConflict(generated.IngestIdentityEventConflict{PendingEventId: 1})
			return
		}},
	}
	for _, test := range ingest {
		response, err := test.build()
		if err != nil {
			t.Fatal(err)
		}
		decoded, decodeErr := response.ValueByDiscriminator()
		assert(t, test.status, response, decoded, decodeErr)
	}
}

func TestGeneratedCustomerIsChannelNeutral(t *testing.T) {
	typeOf := reflect.TypeOf(generated.Customer{})
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.Split(typeOf.Field(index).Tag.Get("json"), ",")[0]
		for _, forbidden := range []string{"external_userid", "unionid", "openid", "phone", "mobile"} {
			if name == forbidden {
				t.Fatalf("generated Customer contains %s", forbidden)
			}
		}
	}
}

func TestGeneratedIdentityRefKeepsTrustServerSide(t *testing.T) {
	typeOf := reflect.TypeOf(generated.IdentityRef{})
	want := map[string]bool{"type": true, "scope": true, "value": true}
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "assurance" || name == "source" {
			t.Fatalf("IdentityRef lets an admin request self-assert trust: %s", name)
		}
		delete(want, name)
		if field.Type.Kind() == reflect.Pointer {
			t.Fatalf("IdentityRef.%s became optional", field.Name)
		}
	}
	if len(want) > 0 {
		t.Fatalf("IdentityRef missing required fields: %v", want)
	}
}

func TestPublicPortSurfaceIsFrozen(t *testing.T) {
	contracts := map[string]struct {
		value   any
		methods []string
	}{
		"contact.MergePort": {(*contactport.MergePort)(nil), []string{"AppendExternalEvent", "CreateForIdentity", "MergeCustomers"}},
		"config.Service":    {(*configport.Service)(nil), []string{"Get", "Set"}},
		"events.Appender":   {(*eventport.Appender)(nil), []string{"Append"}},
		"identity.Service":  {(*identityport.Service)(nil), []string{"Bind", "Ingest", "Resolve"}},
		"identity.ReviewService": {(*identityport.ReviewService)(nil), []string{
			"ApproveMergeReview", "ListMergeReviews", "ListMergeReviewsByStatus", "RejectMergeReview",
		}},
		"segment.Service": {(*segmentport.Service)(nil), []string{
			"Archive", "Create", "Get", "List", "ListMembers", "RequestRefresh", "Update",
		}},
		"auth.Service":        {(*authport.Service)(nil), []string{"Authenticate", "Authorize", "Invalidate", "ValidateCSRF"}},
		"platform.UnitOfWork": {(*platformport.UnitOfWork)(nil), []string{"Within"}},
	}
	for name, contract := range contracts {
		assertMethodNames(t, name, reflect.TypeOf(contract.value).Elem(), contract.methods)
	}

	for _, command := range []any{contactport.CreateForIdentityCommand{}, contactport.MergeCustomersCommand{}, contactport.ExternalEventCommand{}} {
		typeOf := reflect.TypeOf(command)
		for index := 0; index < typeOf.NumField(); index++ {
			lower := strings.ToLower(typeOf.Field(index).Name)
			if strings.Contains(lower, "external") || strings.Contains(lower, "union") || strings.Contains(lower, "openid") || strings.Contains(lower, "phone") {
				t.Fatalf("contact command leaks external identity: %s.%s", typeOf.Name(), typeOf.Field(index).Name)
			}
		}
	}
}

// TestGeneratedLegacyHealthSnapshotKeepsTheExactLegacyFieldSet pins the
// LEGACY-API-0757 wire shape: exactly the 15 frozen fields, nothing else.
func TestGeneratedLegacyHealthSnapshotKeepsTheExactLegacyFieldSet(t *testing.T) {
	typeOf := reflect.TypeOf(generated.LegacyRuntimeHealthSnapshot{})
	want := map[string]bool{
		"ok": true, "status": true, "service": true,
		"secret_key_present": true, "wechat_shop_callback_token_present": true,
		"wechat_shop_callback_token_required": true, "database": true, "database_mode": true,
		"fixture_mode": true, "production_data_ready": true, "production_data_mode": true,
		"repository_policy": true, "runtime_owner": true, "legacy_runtime_enabled": true,
		"warning": true,
	}
	if typeOf.NumField() != len(want) {
		t.Fatalf("LegacyRuntimeHealthSnapshot fields=%d want=%d", typeOf.NumField(), len(want))
	}
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.Split(typeOf.Field(index).Tag.Get("json"), ",")[0]
		if !want[name] {
			t.Fatalf("LegacyRuntimeHealthSnapshot carries unexpected field %s", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("LegacyRuntimeHealthSnapshot missing fields: %v", want)
	}
}

func TestCandidateServerIsNotTheRuntimeServer(t *testing.T) {
	assertMethodNames(t, "runtime server", reflect.TypeOf((*runtimegenerated.StrictServerInterface)(nil)).Elem(), []string{"GetHealthz"})
	assertMethodNames(t, "candidate server", reflect.TypeOf((*generated.StrictServerInterface)(nil)).Elem(), []string{
		"ListUnboundTagHistory", "GetUnboundTagHistory", "ListInvalidChannelHistory", "GetInvalidChannelHistory",
		"ListInvalidAssetHistory", "GetInvalidAssetHistory", "ListInvalidRadarLinkHistory", "GetInvalidRadarLinkHistory",
		"ListCampaignHistoryDefinitions", "GetCampaignHistoryDefinition", "ListCampaignHistoryDefinitionSteps",
		"ListMarketingStateHistorySnapshot", "GetMarketingStateHistorySnapshot", "ListMarketingStateHistoryChange", "GetMarketingStateHistoryChange", "ListMarketingStateHistoryValueSnapshot", "GetMarketingStateHistoryValueSnapshot", "ListMarketingStateHistoryValueChange", "GetMarketingStateHistoryValueChange",
		"ListBroadcastJobHistory", "GetBroadcastJobHistory",
		"ListOutboundTaskHistory", "GetOutboundTaskHistory",
		"ListAudienceHistoryGroups", "ListAudienceHistoryPackages", "ListAudienceHistoryVersions", "ListAudienceHistorySenders", "ListAudienceHistoryRules", "ListAudienceHistoryRuleVersions", "ListAudienceHistoryDefinitions", "ListAudienceHistoryMembers", "GetAudienceHistoryPackage", "GetAudienceHistoryDefinition",
		"ListProfileHistoryTemplates", "GetProfileHistoryTemplate", "ListProfileHistoryCategories", "ListProfileHistoryOptionMappings", "ListSignupTagHistoryRules",
		"GetChannelHistory",
		"ListCampaignHistorySegments",
		"ListLegacyMarketingHistoryStates",
		"GetLegacyMarketingHistoryState",
		"ListLegacyMarketingHistoryValues",
		"GetLegacyMarketingHistoryValue",
		"ListWeComContactHistoryEvents",
		"GetWeComContactHistoryEvent",
		"ListWeComContactHistoryRelations",
		"GetWeComContactHistoryRelation",
		"GetCampaignHistorySegment",
		"ListCampaignHistoryMembers",
		"ListCampaignHistoryBroadcastPlans",
		"GetCampaignHistoryBroadcastPlan",
		"ListCampaignHistoryBroadcastRecipients",
		"ListCampaignHistoryBroadcastMessages",
		"ListServicePeriodHistoryDefinitions", "ListServicePeriodHistoryEntitlements", "ListServicePeriodHistoryEvents",
		"ListCouponHistoryDefinitions", "ListCouponHistoryClaims", "ListCouponHistoryRedemptions",
		"ListAutomationHistorySOPs", "GetAutomationHistorySOP", "ListAutomationHistoryConfigs", "GetAutomationHistoryConfig", "ListAutomationHistoryPrompts", "GetAutomationHistoryPrompt", "ListAutomationHistoryAgents", "GetAutomationHistoryAgent",
		"ListSurveyUnresolvedHistorySubmissions", "GetSurveyUnresolvedHistorySubmission", "ListSurveyUnresolvedHistoryAnswers",
		"ListHXCHistoryMeta", "GetHXCHistoryMeta", "ListHXCHistorySnapshot", "GetHXCHistorySnapshot", "ListHXCHistoryActivation", "GetHXCHistoryActivation", "ListHXCHistoryLead", "GetHXCHistoryLead", "ListHXCHistoryBatch", "GetHXCHistoryBatch",
		"ListCustomerStateHistorySnapshot", "GetCustomerStateHistorySnapshot", "ListCustomerStateHistoryChange", "GetCustomerStateHistoryChange", "ListCustomerStateHistoryClassTermTagMapping", "GetCustomerStateHistoryClassTermTagMapping",
		"ListStaticHistoryGroupInvite", "GetStaticHistoryGroupInvite", "ListStaticHistoryProductPageSlice", "GetStaticHistoryProductPageSlice", "ListStaticHistoryCycleStrategy", "GetStaticHistoryCycleStrategy", "ListStaticHistoryCycleVersion", "GetStaticHistoryCycleVersion", "ListStaticHistoryCycleDocument", "GetStaticHistoryCycleDocument",
		"ListRadarClickHistory", "GetRadarClickHistory", "ListMarketingConfigHistoryConfigs", "GetMarketingConfigHistoryConfig", "ListMarketingConfigHistoryRules", "GetMarketingConfigHistoryRule",
		"ListGroupOpsHistoryPlans", "ListGroupOpsHistoryDirectory", "ListGroupOpsHistoryGroups", "ListGroupOpsHistoryNodes",
		"ListMemberViewHistory", "GetMemberViewHistory", "ListMemberUsageHistory", "GetMemberUsageHistory",
		"ListSidebarProfileHistory", "GetSidebarProfileHistory", "ListOwnerMigrationResultHistory", "GetOwnerMigrationResultHistory",
		"ListMessageHistory", "GetMessageHistory",
		"AddCustomerTag", "AddServicePeriodMember", "ApproveIdentityMergeReview", "ArchiveSegment", "ArchiveServicePeriodProduct", "ArchiveStage", "ArchiveTag", "ArchiveTagGroup", "BindIdentity", "CopyLegacyWechatPayProduct", "CopyServicePeriodProduct", "CreateCustomerSafeExport", "CreateInternalEventSafeExport", "CreateLegacyChannel", "CreateProduct", "CreateSegment", "CreateServicePeriodMemberGridCollaborator", "CreateServicePeriodMemberView", "CreateServicePeriodProduct", "CreateStage", "CreateTag", "CreateTagGroup", "DeleteLegacyWechatPayProduct", "DeleteServicePeriodMemberGridCollaborator", "DeleteServicePeriodMemberView", "DisableLegacyWechatPayProduct", "DisableQuestionnairePublicDefinition", "DisableServicePeriodProduct", "DownloadCustomerSafeExport", "DownloadInternalEventSafeExport", "EnableLegacyWechatPayProduct", "EnableServicePeriodProduct", "ExpireServicePeriodMember", "ExportServicePeriodMembers", "GetAdminConfigOverview", "GetAuthSession",
		"DeleteCustomerContactPolicy", "GetCustomer", "GetCustomerActivityAnalytics", "GetCustomerContactPolicy", "GetCustomerContext", "GetCustomerSafeExport", "GetDomainVerificationFile", "GetInternalEventSafeExport", "GetLegacyChannel", "GetLegacyChannelListPage", "GetLegacyExecutionRuntime", "GetLegacyExecutionRuntimePage", "GetLegacyExecutionTimeline", "GetLegacyHealth", "GetLegacyPushCenterSections", "GetLegacyPushCenterStats", "GetLegacyWechatPayProductShare", "GetProduct", "GetProductLocalEntitlement", "GetPublicSurveyDefinition", "GetPublicSurveyPage", "GetQuestionnairePublicAnalytics", "GetSegment", "GetServicePeriodMember", "GetServicePeriodMemberGridAccess", "GetServicePeriodMemberGridSchema", "GetServicePeriodMemberGridShareSettings", "GetServicePeriodProduct", "GetServicePeriodProductShare", "GetSetupWizard", "GrantProductLocalEntitlement", "IngestIdentityEvent", "IssueAPIClientToken", "ListAutomationTriggerRuns", "ListCustomerChatActivity", "ListCustomerEvents", "ListCustomerMergeHistory", "ListCustomerSurveyAnswers", "ListCustomers", "ListIdentityMergeReviews", "PutCustomerContactPolicy", "QueryPublicServicePeriodMemberGridSummary",
		"ListAdminAccessMembers", "ListLegacyChannelEntrants", "ListLegacyChannels", "ListProductLocalEntitlements", "ListProducts", "ListSegmentMembers", "ListSegments", "ListServicePeriodMemberViews", "ListServicePeriodMembers", "ListServicePeriodProducts", "ListStages", "ListTagGroups", "ListTags", "LogoutAdmin", "PublishQuestionnairePublicDefinition", "QueryPublicSurveySubmissionResult", "QueryServicePeriodMemberGrid", "RejectIdentityMergeReview",
		"RemoveCustomerTag", "RemoveServicePeriodMember", "RenameStage", "ReorderStages", "ReorderTagGroups", "ReorderTags", "RequestSegmentRefresh", "ResolveIdentity", "RevokeProductLocalEntitlement", "SaveAdminAccessMembers", "SaveSetupWizard", "SetCustomerStage", "SetServicePeriodMemberGridExternalShare", "SubmitPublicSurvey", "UpdateCustomer", "UpdateLegacyChannel", "UpdateProduct", "UpdateSegment", "UpdateServicePeriodMemberFields", "UpdateServicePeriodMemberGridCollaborator", "UpdateServicePeriodMemberView", "UpdateServicePeriodProduct", "UpdateTag", "UpdateTagGroup",
		"AcknowledgeAdminOpsMessageBatch", "ApproveAdminOpsBroadcastJob", "CancelAdminOpsBroadcastJob", "CheckAdminOpsCategory", "CompareAdminOpsReleaseShadow", "CreateAdminOpsRelease", "GetAdminOpsBroadcastJob", "GetAdminOpsCategory", "GetAdminOpsConfigPage", "GetAdminOpsFeishuNotificationSetting", "GetAdminOpsJobsSummary", "GetAdminOpsMessageBatch", "GetAdminOpsNewReleasePage", "GetAdminOpsPushCapabilities", "GetAdminOpsRelease", "GetAdminOpsReleasePage", "GetAdminOpsReleasesPage", "ListAdminOpsArchiveSyncJobs", "ListAdminOpsBroadcastJobs", "ListAdminOpsCallbackJobs", "ListAdminOpsCategories", "ListAdminOpsDeferredJobs", "ListAdminOpsMessageBatchJobs", "ListAdminOpsReleases", "ListAdminOpsWebhookDeliveryJobs", "PublishAdminOpsRelease", "RollbackAdminOpsRelease", "RunAdminOpsArchiveSyncPlan", "RunAdminOpsFeishuHourlyReportPlan", "SaveAdminOpsFeishuNotificationSetting", "SetAdminOpsCategoryEnabled", "SetAdminOpsCategorySettings", "SetAdminOpsPushCapability", "SetAdminOpsPushScheduler", "ValidateAdminOpsFeishuNotificationPlan", "ValidateAdminOpsRelease",
		"ListLegacyOutboundJobs", "GetLegacyOutboundJob", "GetLegacyOutboundJobReconciliation", "CancelLegacyOutboundJob", "RetryLegacyOutboundJob",
		"ListExternalEffectsRuntime", "GetExternalEffectRuntime", "GetExternalEffectsDiagnostics", "CancelExternalEffectRuntime", "RetryExternalEffectRuntime", "ReconcileExternalEffectRuntime",
		"PreviewMediaContentPackage", "CreateMediaContentPackage", "UpdateMediaContentPackage", "InitiateMediaAttachmentMultipartUpload", "PutMediaAttachmentMultipartPart", "CompleteMediaAttachmentMultipartUpload", "GetMediaContentDeliveryBinding", "CreateMediaContentDeliveryBinding", "UpdateMediaContentDeliveryBinding", "AcceptOutboundMediaContentPackage", "GetOutboundMediaEffectDetail", "ReconcileOutboundMediaEffect",
		"AcceptOutboundCampaignHandoff", "GetOutboundCampaignHandoffSummary", "ReconcileOutboundCampaignHandoff",
		"DispatchOutboundCampaignHandoff", "GetOutboundCampaignDispatchReconciliation", "ReconcileOutboundCampaignDispatch",
		"CreateCloudCampaignTouchPlan", "GetCloudCampaignTouchPlan", "ListCloudCampaignPlans", "ListCloudCampaignTouchPlans", "GetCloudCampaignTouchPlanRecipient", "GetCloudCampaignTouchPlanRecipientReview", "GetCloudCampaignTouchPlanReview", "ListCloudCampaignTouchPlanRecipients", "MutateCloudCampaignTouchPlanRecipientReview", "MutateCloudCampaignTouchPlanReview",
		"MintSidebarContext", "GetSidebarWorkbench", "UpdateSidebarProfile", "BindSidebarPhone", "ListSidebarQuestionnaires", "ListSidebarOrders", "ListSidebarPeriodicOrders", "UpdateSidebarPeriodicRemark", "ListSidebarMaterials", "ListSidebarShareableProducts", "PrepareSidebarImageTemporaryMedia", "GetSidebarMaterialThumbnailStatus", "GetSidebarMaterialThumbnailPreview", "StartSidebarOAuth", "CompleteSidebarOAuth", "GetSidebarAgentConfig", "ListSidebarTimeline", "ListSidebarChatActivity", "ListSidebarOtherStaffChats",
		"CreateContactOwnerReassignmentPreview", "GetContactOwnerReassignmentPreview", "DownloadContactOwnerReassignmentErrors", "ExecuteContactOwnerReassignmentPreview", "DownloadContactOwnerReassignmentResults", "DownloadContactOwnerReassignmentTemplate",
		"ListReleaseCandidates", "RegisterReleaseCandidate", "GetReleaseCandidate", "RecordReleasePrerequisite", "PrepareReleaseCandidate", "StartReleaseCutover", "RestartReleaseCutover", "CompleteReleaseCutoverStep", "ActivateReleaseCandidate", "RecordReleaseRollbackCheck", "RequestReleaseRollback", "CompleteReleaseRollback",
		"CreateWechatPayCheckout", "GetWechatPayCheckout", "CreateWechatPaySettlementRefund", "ReceiveWechatPayPaymentCallback", "ReceiveWechatPayRefundCallback",
		"StartSurveyH5OAuth", "CallbackSurveyH5OAuth", "GetSurveyExternalPushDetail", "ReconcileSurveyExternalPush",
		"ReconcileWechatShopRefund", "ReceiveWechatShopRefundCallback", "VerifyWechatShopRefundCallbackURL",
		"GetWechatPayProductExternalPush", "SaveWechatPayProductExternalPush", "QueueWechatPayProductExternalPushTest", "GetServicePeriodProductExternalPush", "SaveServicePeriodProductExternalPush", "QueueServicePeriodProductExternalPushTest",
	})
}

func assertMethodNames(t *testing.T, name string, contract reflect.Type, want []string) {
	t.Helper()
	want = append([]string(nil), want...)
	sort.Strings(want)
	if contract.NumMethod() != len(want) {
		t.Fatalf("%s methods=%d want=%d", name, contract.NumMethod(), len(want))
	}
	for index, methodName := range want {
		if got := contract.Method(index).Name; got != methodName {
			t.Fatalf("%s method[%d]=%q want=%q", name, index, got, methodName)
		}
	}
}
