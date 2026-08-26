package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

type campaignInitiationRootAuth struct {
	expectedCSRF string
	csrfCalls    int
}

var _ authport.Service = (*campaignInitiationRootAuth)(nil)

func (*campaignInitiationRootAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, nil
}

func (*campaignInitiationRootAuth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (auth *campaignInitiationRootAuth) ValidateCSRF(_ context.Context, _ authport.SessionRef, token authport.CSRFToken) error {
	auth.csrfCalls++
	if string(token) != auth.expectedCSRF {
		return authport.ErrCSRFInvalid
	}
	return nil
}

func (*campaignInitiationRootAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

type campaignInitiationRootApplication struct {
	plan        campaign.DraftTouchPlan
	createCalls int
	indexCalls  int
	listCalls   int
	detailCalls int
}

func (application *campaignInitiationRootApplication) ListTouchPlanIndex(context.Context, campaign.TouchPlanReviewStatus, string, int32) (campaign.TouchPlanIndexPage, error) {
	application.indexCalls++
	return campaign.TouchPlanIndexPage{}, nil
}

func (application *campaignInitiationRootApplication) CreateDraftTouchPlan(context.Context, campaign.CreateDraftTouchPlanCommand) (campaign.DraftTouchPlan, error) {
	application.createCalls++
	return campaign.CloneDraftTouchPlan(application.plan), nil
}

func (application *campaignInitiationRootApplication) ListDraftTouchPlans(context.Context, string, string, int32) (campaign.DraftTouchPlanPage, error) {
	application.listCalls++
	return campaign.DraftTouchPlanPage{}, nil
}

func (application *campaignInitiationRootApplication) GetDraftTouchPlan(context.Context, string, string) (campaign.DraftTouchPlan, error) {
	application.detailCalls++
	return campaign.CloneDraftTouchPlan(application.plan), nil
}

func TestCampaignInitiationRootCSRFIsValidatedExactlyOnce(t *testing.T) {
	csrf := legacyToken(0x31)
	auth := &campaignInitiationRootAuth{expectedCSRF: csrf}
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	application := &campaignInitiationRootApplication{plan: campaignInitiationRootPlan(t)}
	fragment, err := campaign.NewInitiationRouteFragment(application, legacyCampaignAuthorizer{})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(auth, &legacyCustomerStub{result: legacyCustomerResult()})
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{Handler: authHandler, campaignInitiation: fragment}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, candidate, legacy,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name      string
		token     string
		wantCode  int
		wantCSRF  int
		wantCalls int
	}{
		{name: "missing", wantCode: http.StatusForbidden, wantCSRF: 0, wantCalls: 0},
		{name: "wrong", token: legacyToken(0x32), wantCode: http.StatusForbidden, wantCSRF: 1, wantCalls: 0},
		{name: "correct", token: csrf, wantCode: http.StatusCreated, wantCSRF: 1, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth.csrfCalls = 0
			application.createCalls = 0
			request := httptest.NewRequest(http.MethodPost, campaign.RoutePrefix+"/spring-campaign/touch-plans", bytes.NewBufferString(`{"expected_campaign_version":4,"source":{"kind":"customer_selection","customer_ids":[9]}}`))
			request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(0x30)})
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "root-csrf-touch-plan-key")
			if test.token != "" {
				request.Header.Set("X-CSRF-Token", test.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantCode || auth.csrfCalls != test.wantCSRF || application.createCalls != test.wantCalls {
				t.Fatalf("status=%d csrf=%d creates=%d body=%s", response.Code, auth.csrfCalls, application.createCalls, response.Body.String())
			}
		})
	}

	for _, test := range []struct {
		name      string
		path      string
		wantCalls func() int
	}{
		{"list", campaign.RoutePrefix + "/spring-campaign/touch-plans?limit=1", func() int { return application.listCalls }},
		{"detail", campaign.RoutePrefix + "/spring-campaign/touch-plans/" + string(application.plan.ID), func() int { return application.detailCalls }},
		{"global index", campaign.TouchPlanIndexPath + "?limit=1", func() int { return application.indexCalls }},
	} {
		t.Run(test.name+" delegates through generated candidate adapter", func(t *testing.T) {
			application.indexCalls = 0
			application.listCalls = 0
			application.detailCalls = 0
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(0x30)})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || test.wantCalls() != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, test.wantCalls(), response.Body.String())
			}
		})
	}
}

func campaignInitiationRootPlan(t *testing.T) campaign.DraftTouchPlan {
	t.Helper()
	source, valid := campaign.NewCustomerSelectionSourceRef([]int64{9})
	if !valid {
		t.Fatal("source is invalid")
	}
	content := campaign.CanonicalContentSnapshot([]campaign.Step{{Index: 1, DelayMinutes: 0, Content: "hello"}})
	plan := campaign.DraftTouchPlan{
		ID: campaign.DraftTouchPlanID(7, "spring-campaign", "root-csrf-touch-plan-key"), CampaignCode: "spring-campaign", CampaignVersion: 4,
		Source: source, Targets: campaign.CustomerTargetSnapshot{CustomerIDs: []int64{9}, Digest: campaign.CanonicalTargetDigest(source, []int64{9})},
		Content: content, OwnerActorID: 7, Exclusions: campaign.PreviewExclusionSummary{CandidateCount: 1, ActiveCustomerCount: 1},
		CreatedAt: time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC), Safety: campaign.LocalInitiationSafety(),
	}
	if !campaign.ValidDraftTouchPlan(plan) {
		t.Fatalf("invalid plan: %#v", plan)
	}
	return plan
}
