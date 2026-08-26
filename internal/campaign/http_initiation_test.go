package campaign

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type initiationHTTPApplicationStub struct {
	createCommand CreateDraftTouchPlanCommand
	indexStatus   TouchPlanReviewStatus
	indexCursor   string
	indexLimit    int32
	indexPage     TouchPlanIndexPage
	listCode      string
	listCursor    string
	listLimit     int32
	detailCode    string
	detailID      string
	plan          DraftTouchPlan
	page          DraftTouchPlanPage
	err           error
}

func (stub *initiationHTTPApplicationStub) ListTouchPlanIndex(_ context.Context, status TouchPlanReviewStatus, cursor string, limit int32) (TouchPlanIndexPage, error) {
	stub.indexStatus, stub.indexCursor, stub.indexLimit = status, cursor, limit
	return TouchPlanIndexPage{Items: append([]TouchPlanIndexItem(nil), stub.indexPage.Items...), NextCursor: stub.indexPage.NextCursor}, stub.err
}

func (stub *initiationHTTPApplicationStub) CreateDraftTouchPlan(_ context.Context, command CreateDraftTouchPlanCommand) (DraftTouchPlan, error) {
	stub.createCommand = command
	return CloneDraftTouchPlan(stub.plan), stub.err
}

func (stub *initiationHTTPApplicationStub) ListDraftTouchPlans(_ context.Context, code, cursor string, limit int32) (DraftTouchPlanPage, error) {
	stub.listCode, stub.listCursor, stub.listLimit = code, cursor, limit
	return DraftTouchPlanPage{Items: CloneDraftTouchPlanSummaries(stub.page.Items), NextCursor: stub.page.NextCursor}, stub.err
}

func (stub *initiationHTTPApplicationStub) GetDraftTouchPlan(_ context.Context, code, id string) (DraftTouchPlan, error) {
	stub.detailCode, stub.detailID = code, id
	return CloneDraftTouchPlan(stub.plan), stub.err
}

type initiationHTTPAuthorizerStub struct {
	actor        Actor
	err          error
	requirements []AccessRequirement
}

func (stub *initiationHTTPAuthorizerStub) Authorize(_ *http.Request, requirement AccessRequirement) (Actor, error) {
	stub.requirements = append(stub.requirements, requirement)
	return stub.actor, stub.err
}

func TestInitiationRouteFragmentCreatesSafeDraftProjection(t *testing.T) {
	plan := testHTTPDraftTouchPlan(t)
	application := &initiationHTTPApplicationStub{plan: plan}
	authorizer := &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}}
	handler, err := NewInitiationRouteFragment(application, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"expected_campaign_version":4,"source":{"kind":"customer_selection","customer_ids":[9]}}`)
	request := httptest.NewRequest(http.MethodPost, RoutePrefix+"/spring-campaign/touch-plans", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "draft-touch-key-0001")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || len(authorizer.requirements) != 1 ||
		authorizer.requirements[0] != (AccessRequirement{Capability: CapabilityManageAutomation, RequireCSRF: true}) ||
		application.createCommand.CampaignCode != "spring-campaign" || application.createCommand.Owner.ID != 7 ||
		application.createCommand.ExpectedCampaignVersion != 4 || application.createCommand.Source.Kind != InitiationSourceCustomerSelection ||
		!strings.Contains(response.Body.String(), `"local_only":true`) || !strings.Contains(response.Body.String(), `"content_digest"`) ||
		strings.Contains(response.Body.String(), "customer_ids") {
		t.Fatalf("status=%d requirements=%+v command=%+v body=%s", response.Code, authorizer.requirements, application.createCommand, response.Body.String())
	}
}

func TestInitiationRouteFragmentListsAndGetsRecipientSafeProjection(t *testing.T) {
	plan := testHTTPDraftTouchPlan(t)
	application := &initiationHTTPApplicationStub{plan: plan, page: DraftTouchPlanPage{Items: []DraftTouchPlanSummary{DraftTouchPlanSummaryOf(plan)}, NextCursor: "opaque"}}
	authorizer := &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}}
	handler, err := NewInitiationRouteFragment(application, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, path string
		wantCode   int
	}{
		{"list", RoutePrefix + "/spring-campaign/touch-plans?limit=10", http.StatusOK},
		{"detail", RoutePrefix + "/spring-campaign/touch-plans/" + plan.ID, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.wantCode || strings.Contains(response.Body.String(), "customer_ids") || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if len(authorizer.requirements) != 2 || authorizer.requirements[0].Capability != CapabilityOperationsRead ||
		authorizer.requirements[1].Capability != CapabilityOperationsRead || application.listCode != "spring-campaign" || application.listLimit != 10 ||
		application.detailCode != "spring-campaign" || application.detailID != plan.ID {
		t.Fatalf("requirements=%+v list=%q/%d detail=%q/%q", authorizer.requirements, application.listCode, application.listLimit, application.detailCode, application.detailID)
	}
}

func TestInitiationRouteFragmentListsGlobalReviewIndexWithoutRecipients(t *testing.T) {
	plan := testHTTPDraftTouchPlan(t)
	item := TouchPlanIndexItem{Plan: DraftTouchPlanSummaryOf(plan), ReviewStatus: TouchPlanReviewPending, ReviewVersion: 2}
	application := &initiationHTTPApplicationStub{indexPage: TouchPlanIndexPage{Items: []TouchPlanIndexItem{item}, NextCursor: "opaque"}}
	authorizer := &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}}
	handler, err := NewInitiationRouteFragment(application, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, TouchPlanIndexPath+"?review_status=pending_review&limit=10", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || application.indexStatus != TouchPlanReviewPending || application.indexLimit != 10 ||
		len(authorizer.requirements) != 1 || authorizer.requirements[0] != (AccessRequirement{Capability: CapabilityOperationsRead}) ||
		!strings.Contains(body, `"campaign_code":"spring-campaign"`) || !strings.Contains(body, `"review_status":"pending_review"`) ||
		!strings.Contains(body, `"review_version":2`) || !strings.Contains(body, `"local_only":true`) ||
		strings.Contains(body, "customer_ids") || strings.Contains(body, `"content":"`) {
		t.Fatalf("status=%d requirements=%+v input=%q/%d body=%s", response.Code, authorizer.requirements, application.indexStatus, application.indexLimit, body)
	}
}

func TestInitiationRouteFragmentRejectsPageLargerThanFrozenLimit(t *testing.T) {
	plan := testHTTPDraftTouchPlan(t)
	items := make([]DraftTouchPlanSummary, MaximumDraftTouchPlanPageLimit+1)
	for index := range items {
		items[index] = DraftTouchPlanSummaryOf(plan)
	}
	handler, err := NewInitiationRouteFragment(
		&initiationHTTPApplicationStub{plan: plan, page: DraftTouchPlanPage{Items: items}},
		&initiationHTTPAuthorizerStub{actor: Actor{ID: 7}},
	)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, RoutePrefix+"/spring-campaign/touch-plans", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestInitiationRouteFragmentRejectsMalformedInputAndMapsRedline(t *testing.T) {
	plan := testHTTPDraftTouchPlan(t)
	application := &initiationHTTPApplicationStub{plan: plan, err: ErrBlockedRedline}
	authorizer := &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}}
	handler, err := NewInitiationRouteFragment(application, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, method, path string
		body               string
		want               int
	}{
		{"repeated cursor", http.MethodGet, RoutePrefix + "/spring-campaign/touch-plans?cursor=a&cursor=b", "", http.StatusBadRequest},
		{"invalid index status", http.MethodGet, TouchPlanIndexPath + "?review_status=sent", "", http.StatusBadRequest},
		{"repeated index status", http.MethodGet, TouchPlanIndexPath + "?review_status=draft&review_status=pending_review", "", http.StatusBadRequest},
		{"unknown index query", http.MethodGet, TouchPlanIndexPath + "?sort=created_at", "", http.StatusBadRequest},
		{"oversized index page", http.MethodGet, TouchPlanIndexPath + "?limit=101", "", http.StatusBadRequest},
		{"customer filter redline", http.MethodPost, RoutePrefix + "/spring-campaign/touch-plans", `{"expected_campaign_version":4,"source":{"kind":"customer_filter"}}`, http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", "draft-touch-key-0001")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestInitiationRouteFragmentDefersReviewAndHandoffTo00067(t *testing.T) {
	plan := testHTTPDraftTouchPlan(t)
	handler, err := NewInitiationRouteFragment(&initiationHTTPApplicationStub{plan: plan}, &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}})
	if err != nil {
		t.Fatal(err)
	}
	routes := handler.Routes()
	if len(routes) != 4 {
		t.Fatalf("routes=%+v", routes)
	}
	for _, route := range routes {
		if strings.Contains(route.Pattern, "approve") || strings.Contains(route.Pattern, "reject") || strings.Contains(route.Pattern, "handoff") || strings.Contains(route.Pattern, "recipients") {
			t.Fatalf("00067 route leaked into 00066: %+v", route)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, RoutePrefix+"/spring-campaign/touch-plans/"+plan.ID, nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"review"`) || strings.Contains(response.Body.String(), `"review_status"`) || strings.Contains(response.Body.String(), `"handoff_created"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func testHTTPDraftTouchPlan(t *testing.T) DraftTouchPlan {
	t.Helper()
	source, valid := NewCustomerSelectionSourceRef([]int64{9})
	if !valid {
		t.Fatal("source is invalid")
	}
	content := CanonicalContentSnapshot([]Step{{Index: 1, DelayMinutes: 0, Content: "hello"}})
	plan := DraftTouchPlan{
		ID: DraftTouchPlanID(7, "spring-campaign", "draft-touch-key-0001"), CampaignCode: "spring-campaign", CampaignVersion: 4,
		Source: source, Targets: CustomerTargetSnapshot{CustomerIDs: []int64{9}, Digest: CanonicalTargetDigest(source, []int64{9})},
		Content: content, OwnerActorID: 7, Exclusions: PreviewExclusionSummary{CandidateCount: 1, ActiveCustomerCount: 1},
		CreatedAt: time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC), Safety: LocalInitiationSafety(),
	}
	if !ValidDraftTouchPlan(plan) {
		raw, _ := json.Marshal(plan)
		t.Fatalf("invalid plan: %s", raw)
	}
	return plan
}
