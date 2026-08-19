package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
)

type legacyCouponStub struct {
	item     couponport.Coupon
	page     couponport.Page
	command  couponport.UpsertCommand
	actor    int64
	key      string
	writes   int
	drafts   int
	draftErr error
}

func (s *legacyCouponStub) List(context.Context, int32, int32, string, string) (couponport.Page, error) {
	return s.page, nil
}
func (s *legacyCouponStub) Get(context.Context, couponport.ID) (couponport.Coupon, error) {
	return s.item, nil
}
func (s *legacyCouponStub) Create(_ context.Context, c couponport.UpsertCommand) (couponport.Coupon, error) {
	s.command, s.writes = c, s.writes+1
	return s.item, nil
}
func (s *legacyCouponStub) Update(_ context.Context, c couponport.UpsertCommand) (couponport.Coupon, error) {
	s.command, s.writes = c, s.writes+1
	return s.item, nil
}
func (s *legacyCouponStub) UpdateDraft(_ context.Context, c couponport.UpsertCommand) (couponport.Coupon, error) {
	if s.draftErr != nil {
		return couponport.Coupon{}, s.draftErr
	}
	s.command, s.writes, s.drafts = c, s.writes+1, s.drafts+1
	return s.item, nil
}
func (s *legacyCouponStub) Publish(_ context.Context, _ couponport.ID, actor int64, key string) (couponport.Coupon, error) {
	s.actor, s.key, s.writes = actor, key, s.writes+1
	s.item.Status = "published"
	return s.item, nil
}
func (s *legacyCouponStub) Stop(_ context.Context, _ couponport.ID, actor int64, key string) (couponport.Coupon, error) {
	s.actor, s.key, s.writes = actor, key, s.writes+1
	s.item.Status = "stopped"
	return s.item, nil
}

func TestJ01LegacyCouponSixRoutesDefaultsRBACAndCSRF(t *testing.T) {
	item := legacyCouponItem()
	stub := &legacyCouponStub{item: item, page: couponport.Page{Items: []couponport.Coupon{item}, Total: 1, Limit: 100}}
	router, auth := legacyCouponRouter(t, stub)
	body := `{"name":"满减券","discount_amount_total":100,"total_issue_limit":20,"claim_starts_at":"2026-08-16T00:00:00Z","claim_ends_at":"2026-08-20T00:00:00Z","validity_mode":"relative_days","relative_validity_days":30,"target_refs":["standard_product:7"]}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, legacyChannelWriteRequest(http.MethodPost, "/api/admin/coupons", body))
	if create.Code != http.StatusOK || stub.command.Actor != 1 || stub.command.IdempotencyKey == "" || stub.command.PerUserIssueLimit != 1 || stub.command.Instructions != "" || !strings.Contains(create.Body.String(), `"create_replay_safe":false`) {
		t.Fatalf("create=%d command=%#v body=%s", create.Code, stub.command, create.Body.String())
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityCouponsWrite {
		t.Fatalf("create capabilities=%v", got)
	}
	auth.reset()
	for _, tc := range []struct{ method, path string }{{http.MethodGet, "/api/admin/coupons"}, {http.MethodGet, "/api/admin/coupons/7"}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(tc.method, tc.path, legacyToken(81)))
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s=%d %s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
	if got := auth.capabilities(); len(got) != 2 || got[0] != authport.CapabilityCouponsRead || got[1] != authport.CapabilityCouponsRead {
		t.Fatalf("read capabilities=%v", got)
	}
	for _, tc := range []struct{ method, path, body string }{{http.MethodPut, "/api/admin/coupons/7", body}, {http.MethodPost, "/api/admin/coupons/7/publish", ""}, {http.MethodPost, "/api/admin/coupons/7/stop", ""}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyChannelWriteRequest(tc.method, tc.path, tc.body))
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s=%d %s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
	if stub.actor != 1 || stub.key != "coupon:stop:7" || stub.command.ID != 7 {
		t.Fatalf("transition actor=%d key=%q command=%#v", stub.actor, stub.key, stub.command)
	}
	if stub.drafts != 1 {
		t.Fatalf("browser PUT must use draft-only update, drafts=%d", stub.drafts)
	}
}

func TestJ01LegacyCouponRejectsUnknownCrossOriginAndBadQuery(t *testing.T) {
	stub := &legacyCouponStub{item: legacyCouponItem()}
	router, _ := legacyCouponRouter(t, stub)
	for _, request := range []*http.Request{
		legacyChannelWriteRequest(http.MethodPost, "/api/admin/coupons", `{"name":"x","tenant`+`_id":1}`),
		legacyRequest(http.MethodGet, "/api/admin/coupons?tenant"+"_id=1", legacyToken(82)),
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || stub.writes != 0 {
			t.Fatalf("bad request=%d writes=%d body=%s", response.Code, stub.writes, response.Body.String())
		}
	}
	cross := legacyChannelWriteRequest(http.MethodPost, "/api/admin/coupons", `{}`)
	cross.Header.Set("Origin", "https://cross.invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, cross)
	if response.Code != http.StatusForbidden || stub.writes != 0 {
		t.Fatalf("cross=%d writes=%d", response.Code, stub.writes)
	}
}

func TestJ01LegacyCouponDraftPUTMapsLockedRuleConflictTo409(t *testing.T) {
	stub := &legacyCouponStub{item: legacyCouponItem(), draftErr: couponapp.ErrConflict}
	router, _ := legacyCouponRouter(t, stub)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyChannelWriteRequest(http.MethodPut, "/api/admin/coupons/7", `{"name":"满减券","discount_amount_total":100,"total_issue_limit":20,"claim_starts_at":"2026-08-16T00:00:00Z","claim_ends_at":"2026-08-20T00:00:00Z","validity_mode":"relative_days","relative_validity_days":30,"target_refs":["standard_product:7"]}`))
	if response.Code != http.StatusConflict || stub.writes != 0 || stub.drafts != 0 {
		t.Fatalf("draft conflict=%d writes=%d drafts=%d body=%s", response.Code, stub.writes, stub.drafts, response.Body.String())
	}
}

func legacyCouponRouter(t *testing.T, coupons legacyCouponApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.coupons = coupons
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router, service
}

func legacyCouponItem() couponport.Coupon {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	return couponport.Coupon{ID: 7, Name: "满减券", DiscountAmountTotal: 100, Currency: "CNY", TotalIssueLimit: 20, PerUserIssueLimit: 1, ClaimStartsAt: now, ClaimEndsAt: now.Add(time.Hour), ValidityMode: couponport.ValidityRelativeDays, RelativeValidityDays: func() *int32 { value := int32(30); return &value }(), Instructions: "", TargetRefs: []string{"standard_product:7"}, Status: "draft", CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now}
}
