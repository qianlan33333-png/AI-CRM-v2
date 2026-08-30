package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const sidebarTestSession authport.SessionRef = "sidebar-test-session"

type corpFake struct{ value string }

func (fake corpFake) CorpID(context.Context) (string, error) { return fake.value, nil }

type identityFake struct{ result identityport.ResolveResult }

func (fake identityFake) Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error) {
	return fake.result, nil
}

type phoneFake struct{ status string }

func (fake phoneFake) BindPhone(context.Context, PhoneBindingCommand) (string, error) {
	if fake.status == "" {
		return "already_bound", nil
	}
	return fake.status, nil
}

type recordingPhoneFake struct {
	command PhoneBindingCommand
	status  string
}

func (fake *recordingPhoneFake) BindPhone(_ context.Context, command PhoneBindingCommand) (string, error) {
	fake.command = command
	return fake.status, nil
}

type profileFake struct {
	profile                 contactport.SidebarProfile
	resolveErr, readErr     error
	resolveCalls, readCalls int
	updateErr               error
}

type profileEffectFake struct {
	*profileFake
	result  contactport.SidebarProfileUpdateResult
	command contactport.SidebarProfileUpdateCommand
	err     error
}

func (fake *profileEffectFake) UpdateSidebarProfileWithEffect(_ context.Context, command contactport.SidebarProfileUpdateCommand) (contactport.SidebarProfileUpdateResult, error) {
	fake.command = command
	return fake.result, fake.err
}

func (fake *profileFake) ResolveSidebarProfile(_ context.Context, customerID contactport.CustomerID) (contactport.SidebarProfile, error) {
	fake.resolveCalls++
	if fake.resolveErr != nil {
		return contactport.SidebarProfile{}, fake.resolveErr
	}
	if fake.profile.CustomerID != customerID {
		return contactport.SidebarProfile{}, contactport.ErrSidebarProfileNotFound
	}
	return fake.profile, nil
}
func (fake *profileFake) ReadSidebarProfile(_ context.Context, customerID contactport.CustomerID, ownerStaffID int64) (contactport.SidebarProfile, error) {
	fake.readCalls++
	if fake.readErr != nil {
		return contactport.SidebarProfile{}, fake.readErr
	}
	if fake.profile.CustomerID != customerID || fake.profile.OwnerStaffID != ownerStaffID {
		return contactport.SidebarProfile{}, contactport.ErrSidebarProfileNotFound
	}
	return fake.profile, nil
}
func (fake *profileFake) UpdateSidebarProfile(_ context.Context, command contactport.SidebarProfileUpdateCommand) (contactport.SidebarProfile, error) {
	if fake.updateErr != nil {
		return contactport.SidebarProfile{}, fake.updateErr
	}
	if command.Patch.Needs != nil {
		fake.profile.Needs = *command.Patch.Needs
	}
	fake.profile.UpdatedAt = fake.profile.UpdatedAt.Add(time.Second)
	return fake.profile, nil
}

type surveyFake struct {
	page       surveyport.CustomerSurveyAnswerPage
	err        error
	customerID contactport.CustomerID
	limit      int32
	calls      int
	count      int
	countCalls int
	countErr   error
}

func (fake *surveyFake) CountCustomerSurveyAnswers(_ context.Context, customerID contactport.CustomerID) (int, error) {
	fake.customerID, fake.countCalls = customerID, fake.countCalls+1
	return fake.count, fake.countErr
}

func (fake *surveyFake) ListCustomerSurveyAnswers(_ context.Context, customerID contactport.CustomerID, limit int32) (surveyport.CustomerSurveyAnswerPage, error) {
	fake.customerID, fake.limit, fake.calls = customerID, limit, fake.calls+1
	return fake.page, fake.err
}

type orderFake struct {
	page       orderport.Page
	err        error
	calls      int
	count      int64
	countCalls int
	countErr   error
}

func (fake *orderFake) CountCustomer(_ context.Context, customerID int64) (int64, error) {
	fake.countCalls++
	if customerID != 41 {
		return 0, errors.New("unscoped order count")
	}
	return fake.count, fake.countErr
}

func (fake *orderFake) List(_ context.Context, filter orderport.Filter) (orderport.Page, error) {
	fake.calls++
	if filter.CustomerID == nil || *filter.CustomerID != 41 {
		return orderport.Page{}, errors.New("unscoped order read")
	}
	return fake.page, fake.err
}

type memberFake struct {
	member     PeriodicMember
	command    PeriodicRemarkCommand
	listResult PeriodicListResult
	listQuery  PeriodicListQuery
	getErr     error
	listErr    error
	count      int
	countCalls int
	countErr   error
}

func (fake *memberFake) CountCustomer(_ context.Context, customerID int64) (int, error) {
	fake.countCalls++
	if customerID != 41 {
		return 0, ErrForbidden
	}
	return fake.count, fake.countErr
}

func (fake *memberFake) Get(context.Context, int64, string) (PeriodicMember, error) {
	return fake.member, fake.getErr
}
func (fake *memberFake) UpdateRemark(_ context.Context, command PeriodicRemarkCommand) (PeriodicMember, error) {
	fake.command = command
	fake.member.Remark = command.Remark
	fake.member.Version++
	return fake.member, nil
}
func (fake *memberFake) ListCustomer(_ context.Context, query PeriodicListQuery) (PeriodicListResult, error) {
	fake.listQuery = query
	if fake.listResult.Items == nil {
		fake.listResult = PeriodicListResult{Items: []PeriodicMember{fake.member}, Limit: query.Limit, Offset: query.Offset}
	}
	return fake.listResult, fake.listErr
}

type mediaFake struct {
	page                  mediaport.ImageListPage
	facets                mediaport.ImageFacets
	listErr, facetsErr    error
	existsErr             error
	exists                bool
	listCalls, facetCalls int
	query                 mediaport.ImageListQuery
	count                 int64
	countCalls            int
	countErr              error
}

func (fake *mediaFake) CountEnabledImages(context.Context) (int64, error) {
	fake.countCalls++
	return fake.count, fake.countErr
}

func (fake *mediaFake) ListImages(_ context.Context, query mediaport.ImageListQuery) (mediaport.ImageListPage, error) {
	fake.listCalls++
	fake.query = query
	return fake.page, fake.listErr
}
func (fake *mediaFake) Facets(context.Context) (mediaport.ImageFacets, error) {
	fake.facetCalls++
	return fake.facets, fake.facetsErr
}
func (fake *mediaFake) LocalImageExists(context.Context, int64) (bool, error) {
	return fake.exists, fake.existsErr
}

func TestContextTokenBindsViewerOwnerCorpAndExpiry(t *testing.T) {
	service, staff := sidebarTestService(t)
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staff}
	result, err := service.MintContext(context.Background(), principal, sidebarTestSession, true, "wm_external_41")
	if err != nil || result.State != "ready" || result.Token == "" || result.CustomerID != 41 || result.OwnerStaffID != staff || !result.Safety.LocalOnly || result.Safety.ProviderExecutionEligible || result.Safety.RealExternalCallExecuted {
		t.Fatalf("MintContext() result=%+v err=%v", result, err)
	}
	scope, err := service.VerifyContext(context.Background(), principal, sidebarTestSession, result.Token)
	if err != nil || scope.CustomerID != 41 || scope.OwnerStaffID != staff {
		t.Fatalf("VerifyContext() scope=%+v err=%v", scope, err)
	}
	other := principal
	other.AdminUserID = 10
	if _, err = service.VerifyContext(context.Background(), other, sidebarTestSession, result.Token); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer replay error=%v, want forbidden", err)
	}
	if _, err = service.VerifyContext(context.Background(), principal, sidebarTestSession, result.Token+"x"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("tamper error=%v, want invalid token", err)
	}
	service.now = func() time.Time { return result.ExpiresAt }
	if _, err = service.VerifyContext(context.Background(), principal, sidebarTestSession, result.Token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expiry error=%v, want expired token", err)
	}
}

func TestBootstrapReusesResolvedProfileAndOmitsPIIForNonReadyStates(t *testing.T) {
	service, staff := sidebarTestService(t)
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staff}
	result, err := service.Bootstrap(context.Background(), principal, sidebarTestSession, true, "wm_external_41")
	profiles := service.profiles.(*profileFake)
	if err != nil || result.State != "ready" || result.Token == "" || result.Workbench == nil || result.Workbench.Profile.CustomerID != 41 || profiles.resolveCalls != 1 || profiles.readCalls != 0 {
		t.Fatalf("Bootstrap() result=%+v profile=%+v err=%v", result, profiles, err)
	}

	viewer, err := service.Bootstrap(context.Background(), authport.Principal{}, "", false, "wm_external_41")
	raw, _ := json.Marshal(viewer)
	if err != nil || viewer.State != "viewer_session_required" || viewer.Workbench != nil || stringContains(string(raw), "customer_id") || stringContains(string(raw), "owner_staff_id") || stringContains(string(raw), "context_token") {
		t.Fatalf("viewer result=%+v json=%s err=%v", viewer, raw, err)
	}
}

func TestBootstrapMatchesContextTokenAndWorkbenchTwoStepResult(t *testing.T) {
	legacy, staff := sidebarTestService(t)
	legacy.surveys.(*surveyFake).count = 27
	legacy.orders.(*orderFake).count = 31
	legacy.members.(*memberFake).count = 8
	legacy.media.(*mediaFake).count = 19
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staff}
	contextResult, err := legacy.MintContext(context.Background(), principal, sidebarTestSession, true, "wm_external_41")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := legacy.VerifyContext(context.Background(), principal, sidebarTestSession, contextResult.Token)
	if err != nil {
		t.Fatal(err)
	}
	workbench, err := legacy.Workbench(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}

	combined, _ := sidebarTestService(t)
	combined.surveys.(*surveyFake).count = 27
	combined.orders.(*orderFake).count = 31
	combined.members.(*memberFake).count = 8
	combined.media.(*mediaFake).count = 19
	bootstrap, err := combined.Bootstrap(context.Background(), principal, sidebarTestSession, true, "wm_external_41")
	if err != nil || bootstrap.State != contextResult.State || bootstrap.Token != contextResult.Token || !bootstrap.ExpiresAt.Equal(contextResult.ExpiresAt) || bootstrap.CustomerID != contextResult.CustomerID || bootstrap.OwnerStaffID != contextResult.OwnerStaffID || bootstrap.Workbench == nil || !reflect.DeepEqual(*bootstrap.Workbench, workbench) {
		t.Fatalf("bootstrap=%+v context=%+v workbench=%+v err=%v", bootstrap, contextResult, workbench, err)
	}
}

func TestBootstrapReportsSanitizedFailureStage(t *testing.T) {
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}

	tests := []struct {
		name         string
		stage        string
		breakService func(*Service)
	}{
		{
			name:  "profile",
			stage: "profile",
			breakService: func(service *Service) {
				service.profiles.(*profileFake).resolveErr = contactport.ErrSidebarProfileUnavailable
			},
		},
		{
			name:  "workbench",
			stage: "workbench",
			breakService: func(service *Service) {
				service.workbench = failingWorkbenchReader{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := sidebarTestService(t)
			test.breakService(service)
			_, err := service.Bootstrap(context.Background(), principal, sidebarTestSession, true, "wm_external_41")
			if !errors.Is(err, ErrUnavailable) || BootstrapFailureStage(err) != test.stage {
				t.Fatalf("Bootstrap() error=%v stage=%q, want unavailable/%q", err, BootstrapFailureStage(err), test.stage)
			}
		})
	}
}

type failingWorkbenchReader struct{}

func (failingWorkbenchReader) Read(context.Context, contactport.CustomerID) (WorkbenchCounts, error) {
	return WorkbenchCounts{}, ErrUnavailable
}

func TestUpdateProfileReportsQueuedProviderEligibleWithoutExecution(t *testing.T) {
	service, staff := sidebarTestService(t)
	profile := service.profiles.(*profileFake).profile
	effects := &profileEffectFake{
		profileFake: &profileFake{profile: profile},
		result: contactport.SidebarProfileUpdateResult{
			Profile: profile, EffectQueued: true, ProviderExecutionEligible: true,
		},
	}
	service.profiles = effects
	description := "follow-up"
	result, err := service.UpdateProfile(context.Background(), Scope{
		CustomerID: 41, OwnerStaffID: staff, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staff},
	}, profile.UpdatedAt, contactport.SidebarProfilePatch{Description: &description}, "sidebar-profile-queued-0001")
	if err != nil || result.Safety.LocalOnly || !result.Safety.EffectQueued || !result.Safety.ProviderExecutionEligible || result.Safety.RealExternalCallExecuted || effects.command.CustomerID != 41 || effects.command.OwnerStaffID != staff || effects.command.Actor != "admin:9" {
		t.Fatalf("result=%+v command=%+v err=%v", result, effects.command, err)
	}
}

func TestMintContextReturnsExplicitViewerStateAndRejectsWrongOwner(t *testing.T) {
	service, staff := sidebarTestService(t)
	result, err := service.MintContext(context.Background(), authport.Principal{}, "", false, "wm_external_41")
	if err != nil || result.State != "viewer_session_required" || !result.Safety.LocalOnly {
		t.Fatalf("viewer state=%+v err=%v", result, err)
	}
	other := staff + 1
	wrongOwner, err := service.MintContext(context.Background(), authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &other}, sidebarTestSession, true, "wm_external_41")
	service.identity = identityFake{identityport.ResolveResult{Status: identityport.ResolveNotFound}}
	unbound, unboundErr := service.MintContext(context.Background(), authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &other}, sidebarTestSession, true, "wm_external_41")
	if err != nil || unboundErr != nil || wrongOwner != unbound || wrongOwner.State != "customer_not_bound" || !wrongOwner.Safety.LocalOnly {
		t.Fatalf("wrong-owner/unbound=%+v/%+v errors=%v/%v", wrongOwner, unbound, err, unboundErr)
	}
}

func TestVerifyContextRevalidatesLiveCustomerAndOwnerForEveryRole(t *testing.T) {
	for _, role := range []authport.Role{authport.RoleSales, authport.RoleAdmin, authport.RoleOps} {
		t.Run(string(role), func(t *testing.T) {
			service, staff := sidebarTestService(t)
			principal := authport.Principal{AdminUserID: 9, Role: role}
			if role == authport.RoleSales {
				principal.StaffID = &staff
			}
			minted, err := service.MintContext(context.Background(), principal, sidebarTestSession, true, "wm_external_41")
			if err != nil {
				t.Fatal(err)
			}
			profiles := service.profiles.(*profileFake)
			profiles.profile.OwnerStaffID = staff + 1
			if _, err = service.VerifyContext(context.Background(), principal, sidebarTestSession, minted.Token); !errors.Is(err, ErrTokenInvalid) {
				t.Fatalf("transferred owner error=%v", err)
			}
		})
	}

	service, staff := sidebarTestService(t)
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}
	minted, err := service.MintContext(context.Background(), principal, sidebarTestSession, true, "wm_external_41")
	if err != nil {
		t.Fatal(err)
	}
	profiles := service.profiles.(*profileFake)
	profiles.resolveErr = contactport.ErrSidebarProfileNotFound
	if _, err = service.VerifyContext(context.Background(), principal, sidebarTestSession, minted.Token); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("deleted customer error=%v", err)
	}
	profiles.resolveErr = contactport.ErrSidebarProfileUnavailable
	if _, err = service.VerifyContext(context.Background(), principal, sidebarTestSession, minted.Token); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("live profile dependency error=%v staff=%d", err, staff)
	}
}

func TestWorkbenchFailsClosedWhenAnyAggregateDependencyFails(t *testing.T) {
	for _, dependency := range []string{"profile", "questionnaire", "order", "periodic", "material"} {
		t.Run(dependency, func(t *testing.T) {
			service, staff := sidebarTestService(t)
			switch dependency {
			case "profile":
				service.profiles.(*profileFake).readErr = contactport.ErrSidebarProfileUnavailable
			case "questionnaire":
				service.surveys.(*surveyFake).countErr = errors.New("survey unavailable")
			case "order":
				service.orders.(*orderFake).countErr = errors.New("order unavailable")
			case "periodic":
				service.members.(*memberFake).countErr = ErrUnavailable
			case "material":
				service.media.(*mediaFake).countErr = errors.New("media unavailable")
			}
			result, err := service.Workbench(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff})
			if !errors.Is(err, ErrUnavailable) || result != (WorkbenchResult{}) {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestWorkbenchUsesCountOnlyReadersWithoutLoadingBusinessRows(t *testing.T) {
	service, staff := sidebarTestService(t)
	surveys := service.surveys.(*surveyFake)
	orders := service.orders.(*orderFake)
	members := service.members.(*memberFake)
	media := service.media.(*mediaFake)
	surveys.count, orders.count, members.count, media.count = 27, 31, 8, 19
	result, err := service.Workbench(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff})
	if err != nil || result.QuestionnaireCount != 27 || result.OrderCount != 31 || result.PeriodicOrderCount != 8 || result.MaterialCount != 19 {
		t.Fatalf("Workbench() result=%+v err=%v", result, err)
	}
	if surveys.calls != 0 || orders.calls != 0 || members.listQuery != (PeriodicListQuery{}) || media.listCalls != 0 || surveys.countCalls != 1 || orders.countCalls != 1 || members.countCalls != 1 || media.countCalls != 1 {
		t.Fatalf("unexpected aggregate reads survey=%+v order=%+v member=%+v media=%+v", surveys, orders, members, media)
	}
}

func TestQuestionnairesExposeOnlyCustomerScopedSafeChoices(t *testing.T) {
	service, staff := sidebarTestService(t)
	surveys := service.surveys.(*surveyFake)
	surveys.page = surveyport.CustomerSurveyAnswerPage{Items: []surveyport.CustomerSurveyAnswer{{
		SubmissionID: 91, QuestionnaireID: 12, SubmittedAt: time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC), Score: 8.5,
		ChoiceAnswers: []surveyport.SafeChoiceAnswerPreview{{QuestionID: 3, QuestionType: surveyport.MultiChoice, SortOrder: 2, OptionIDs: []int64{7, 8}}},
	}}, ScanTruncated: true, ResultTruncated: true}
	result, err := service.Questionnaires(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff}, 7)
	if err != nil || surveys.calls != 1 || surveys.customerID != 41 || surveys.limit != 7 || len(result.Items) != 1 || !result.ScanTruncated || !result.ResultTruncated {
		t.Fatalf("result/survey/error=%+v/%+v/%v", result, surveys, err)
	}
	raw, _ := json.Marshal(result)
	for _, forbidden := range []string{"mobile", "phone", "textarea", "answer_text", "external_userid", "unionid"} {
		if stringContains(string(raw), forbidden) {
			t.Fatalf("unsafe questionnaire projection %s", raw)
		}
	}
}

func TestPhoneBindingUsesScopedCustomerAndLocalReceiptKey(t *testing.T) {
	service, staff := sidebarTestService(t)
	phones := &recordingPhoneFake{status: "bound"}
	service.phones = phones
	status, err := service.BindPhone(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}}, "+8613800138000", "sidebar-phone-0001")
	if err != nil || status != "bound" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if phones.command.CustomerID != 41 || phones.command.ActorID != 9 || phones.command.Mobile != "+8613800138000" || phones.command.IdempotencyKey != "sidebar-phone-0001" {
		t.Fatalf("unexpected command %#v", phones.command)
	}
	phones.status = "manual_review"
	if _, err = service.BindPhone(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}}, "+8613800138000", "sidebar-phone-0002"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unexpected status error=%v", err)
	}
}

func TestOrdersRedactPayerAndIdentityFields(t *testing.T) {
	service, staff := sidebarTestService(t)
	service.orders = &orderFake{page: orderport.Page{Items: []orderport.Item{{MerchantOrderNo: "order-41", PayerName: "secret-name", Mobile: "13800000000", UnionID: "secret-union", ProductName: "local product", AmountYuan: "9.90", Currency: "CNY", Status: "paid", Provider: "wechat_pay", DetailURL: "/api/admin/orders/order-41"}}, Total: 1, Limit: 20}}
	result, err := service.Orders(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	for _, forbidden := range []string{"secret-name", "13800000000", "secret-union", "payer_name", "unionid"} {
		if stringContains(string(raw), forbidden) {
			t.Fatalf("unsafe order projection %s", raw)
		}
	}
	if stringContains(string(raw), "detail_url") {
		t.Fatal("sidebar order projection must not expose an admin-only detail URL")
	}
}

func TestPeriodicRemarkUsesCanonicalKeyCASAndCustomerScope(t *testing.T) {
	service, staff := sidebarTestService(t)
	members := service.members.(*memberFake)
	remark := "renew next week"
	updated, err := service.UpdatePeriodicRemark(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}}, 71, members.member.MemberRef, 3, &remark, "sidebar-remark-0001")
	if err != nil || updated.Member.Version != 4 || !updated.Safety.LocalOnly || members.command.ServiceProductID != 71 || members.command.MemberRef != members.member.MemberRef || members.command.ExpectedVersion != 3 {
		t.Fatalf("UpdatePeriodicRemark() member=%+v command=%+v err=%v", updated, members.command, err)
	}
	members.member.CustomerID = 42
	if _, err = service.UpdatePeriodicRemark(context.Background(), Scope{CustomerID: 41, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}}, 71, members.member.MemberRef, 4, &remark, "sidebar-remark-0002"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-customer error=%v", err)
	}
}

func TestPeriodicOrdersBindTheCanonicalReaderToCustomerScope(t *testing.T) {
	service, staff := sidebarTestService(t)
	members := service.members.(*memberFake)
	result, err := service.PeriodicOrders(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff}, 9, 4)
	if err != nil || members.listQuery != (PeriodicListQuery{CustomerID: 41, Limit: 9, Offset: 4}) || len(result.Items) != 1 || result.Items[0].CustomerID != 41 {
		t.Fatalf("result/query/error=%+v/%+v/%v", result, members.listQuery, err)
	}
	members.listResult = PeriodicListResult{Items: []PeriodicMember{members.member}}
	members.listResult.Items[0].CustomerID = 42
	if _, err = service.PeriodicOrders(context.Background(), Scope{CustomerID: 41, OwnerStaffID: staff}, 9, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cross-customer list error=%v", err)
	}
}

func TestMaterialsUseLocalMetadataFacetsAndStableQuickKeywords(t *testing.T) {
	service, _ := sidebarTestService(t)
	media := service.media.(*mediaFake)
	media.page = mediaport.ImageListPage{Items: []mediaport.ImageListItem{{
		ID: 31, Name: "hero", FileName: "hero.png", MimeType: "image/png", FileSize: 123,
		Description: "local", Tags: []string{"launch"}, Category: "cover", Width: 640, Height: 480, UpdatedAt: "2026-08-24T09:00:00Z",
	}}, Total: 1, Limit: 5, Offset: 10}
	media.facets = mediaport.ImageFacets{Categories: []string{"cover", "campaign"}, Tags: []string{"launch", "cover", ""}}
	query := mediaport.ImageListQuery{Limit: 5, Offset: 10, EnabledOnly: true, Search: "hero"}
	result, err := service.Materials(context.Background(), query)
	if err != nil || media.listCalls != 1 || media.facetCalls != 1 || media.query.Limit != query.Limit || media.query.Offset != query.Offset || media.query.EnabledOnly != query.EnabledOnly || media.query.Search != query.Search || len(result.Items) != 1 || result.Items[0].Thumbnail != "pending" ||
		!equalStrings(result.QuickKeywords, []string{"campaign", "cover", "launch"}) || !result.Safety.LocalOnly {
		t.Fatalf("result/media/error=%+v/%+v/%v", result, media, err)
	}
}

func TestThumbnailStatusIsPendingOnlyForExistingLocalImage(t *testing.T) {
	service, _ := sidebarTestService(t)
	media := service.media.(*mediaFake)
	status, err := service.ThumbnailStatus(context.Background(), 31)
	if err != nil || status != "pending" {
		t.Fatalf("existing status/error=%q/%v", status, err)
	}
	media.exists = false
	if _, err = service.ThumbnailStatus(context.Background(), 31); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing image error=%v", err)
	}
}

func sidebarTestService(t *testing.T) (*Service, int64) {
	t.Helper()
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	staff := int64(7)
	profiles := &profileFake{profile: contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: staff, Name: "customer", UpdatedAt: now}}
	members := &memberFake{member: PeriodicMember{MemberRef: "spm_abcdefghijklmnopqrstuv", ServiceProductID: 71, CustomerID: 41, State: "active", Source: "manual", StartsAt: now.Add(-time.Hour), Version: 3, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}}
	service, err := NewService(corpFake{"corp-1"}, identityFake{identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 41}}, phoneFake{}, profiles, &surveyFake{}, &orderFake{}, members, &mediaFake{exists: true}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	return service, staff
}

func stringContains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
