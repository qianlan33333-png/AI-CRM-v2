package main

import (
	"context"
	"encoding/json"
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
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type legacyCouponBoardStub struct {
	legacyCouponStub
	payment map[string]int64
	sidebar map[string]int64
	claims  couponport.ClaimPage
	items   []couponport.Coupon
	rows    []couponport.SidebarCoupon
	claim   couponport.ClaimCommand
	copyID  couponport.ID
	copyKey string
	writes  int
	limit   int32
	offset  int32
	status  string
	query   string
}

func (s *legacyCouponBoardStub) List(_ context.Context, limit, offset int32, status, query string) (couponport.Page, error) {
	s.limit, s.offset, s.status, s.query = limit, offset, status, query
	return s.page, nil
}

func (s *legacyCouponBoardStub) Archive(_ context.Context, _ couponport.ID, _ int64, _ string) (couponport.Coupon, error) {
	s.writes++
	return s.item, nil
}
func (s *legacyCouponBoardStub) Delete(_ context.Context, _ couponport.ID, _ int64, _ string) (couponport.Coupon, error) {
	s.writes++
	return s.item, nil
}
func (s *legacyCouponBoardStub) Copy(_ context.Context, id couponport.ID, _ int64, key string) (couponport.Coupon, error) {
	s.copyID, s.copyKey = id, key
	s.writes++
	return s.item, nil
}
func (s *legacyCouponBoardStub) Claim(_ context.Context, command couponport.ClaimCommand) (couponport.Claim, error) {
	s.claim, s.writes = command, s.writes+1
	return couponport.Claim{ID: 1, CouponID: int64(command.CouponID), CustomerID: command.CustomerID, ClaimNumber: 1, ClaimRef: "cp_abcdefghijklmnop", Status: "claimed", ClaimedAt: time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)}, nil
}
func (s *legacyCouponBoardStub) ListClaims(context.Context, couponport.ID, int32, int32) (couponport.ClaimPage, error) {
	return s.claims, nil
}
func (s *legacyCouponBoardStub) ListAvailable(context.Context, string, int64) ([]couponport.Coupon, error) {
	return s.items, nil
}
func (s *legacyCouponBoardStub) ResolvePaymentIdentitySession(_ context.Context, token string) (int64, error) {
	id, ok := s.payment[token]
	if !ok {
		return 0, couponapp.ErrNotClaimable
	}
	return id, nil
}
func (s *legacyCouponBoardStub) ResolveSidebarGrant(_ context.Context, token string) (int64, error) {
	id, ok := s.sidebar[token]
	if !ok {
		return 0, couponapp.ErrNotClaimable
	}
	return id, nil
}
func (s *legacyCouponBoardStub) ListSidebarCoupons(context.Context, int64) ([]couponport.SidebarCoupon, error) {
	return s.rows, nil
}

func legacyCouponBoardRouter(t *testing.T, stub *legacyCouponBoardStub) http.Handler {
	t.Helper()
	router, _ := legacyCouponBoardRouterWithAuth(t, stub)
	return router
}

func legacyCouponBoardRouterWithAuth(t *testing.T, stub *legacyCouponBoardStub) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	products := &legacyProductStub{page: productport.LegacyPage{Items: []productport.Product{{ID: 7, Name: "商品", PriceMinor: 999, Currency: "CNY"}}, Total: 1, Limit: 20}}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, products, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.coupons, legacy.couponBoard = stub, stub
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

func couponPublicRequest(method, path string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}
func couponPublicWrite(path, body string, cookie *http.Cookie, key string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Origin", "http://example.com")
	request.Header.Set("Idempotency-Key", key)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}
func couponBoardAdminWrite(method, path string) *http.Request {
	request := legacyChannelWriteRequest(method, path, "")
	request.Header.Set("Idempotency-Key", "coupon-board-key-0001")
	return request
}

func TestCouponProductOptionsUsesLocalPageTotalWithoutQuery(t *testing.T) {
	products := &legacyProductStub{page: productport.LegacyPage{
		Items:  []productport.Product{{ID: 27, Name: "本地普通商品", PriceMinor: 9900, Currency: "CNY"}},
		Total:  21,
		Limit:  20,
		Offset: 20,
	}}
	handler, err := NewHandlerWithOutboundProductsMediaAndSurvey(
		&recordingAuth{},
		&legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{},
		&legacyCancelStub{},
		&legacyRetryStub{},
		products,
		&legacyMediaStub{},
		&legacySurveyStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.CouponProductOptions(response, httptest.NewRequest(
		http.MethodGet,
		"/api/admin/coupons/product-options?product_type=standard_product&limit=20&offset=20",
		nil,
	))
	if response.Code != http.StatusOK || products.lastLimit != 20 || products.lastOffset != 20 {
		t.Fatalf("status/page=%d/%d/%d body=%s", response.Code, products.lastLimit, products.lastOffset, response.Body.String())
	}
	var body struct {
		OK     bool  `json:"ok"`
		Total  int64 `json:"total"`
		Limit  int32 `json:"limit"`
		Offset int32 `json:"offset"`
		Items  []struct {
			ID        int64  `json:"id"`
			TargetRef string `json:"target_ref"`
			Name      string `json:"name"`
			Price     int64  `json:"price_minor"`
			Currency  string `json:"currency"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Total != 21 || body.Limit != 20 || body.Offset != 20 || len(body.Items) != 1 || body.Items[0] != (struct {
		ID        int64  `json:"id"`
		TargetRef string `json:"target_ref"`
		Name      string `json:"name"`
		Price     int64  `json:"price_minor"`
		Currency  string `json:"currency"`
	}{ID: 27, TargetRef: "standard_product:27", Name: "本地普通商品", Price: 9900, Currency: "CNY"}) {
		t.Fatalf("body=%#v", body)
	}
}

func TestCouponBoardPublicIdentityFailsClosedAndCannotBeSpoofed(t *testing.T) {
	item := legacyCouponItem()
	item.Status = "published"
	payment, other, sidebar := legacyToken(101), legacyToken(102), legacyToken(103)
	stub := &legacyCouponBoardStub{legacyCouponStub: legacyCouponStub{item: item}, payment: map[string]int64{payment: 41, other: 42}, sidebar: map[string]int64{sidebar: 41}, items: []couponport.Coupon{item}, rows: []couponport.SidebarCoupon{{CouponID: item.ID, CouponName: item.Name, CouponStatus: item.Status, ClaimRef: "cp_abcdefghijklmnop", ClaimedAt: item.CreatedAt}}}
	router := legacyCouponBoardRouter(t, stub)

	for _, request := range []*http.Request{
		couponPublicRequest(http.MethodGet, "/api/h5/coupons/available?target_ref=standard_product:7", nil),
		couponPublicRequest(http.MethodGet, "/api/h5/coupons/available?target_ref=standard_product:7", &http.Cookie{Name: paymentIdentityCookieName, Value: "customer:41"}),
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("identity rejection status=%d body=%s", response.Code, response.Body.String())
		}
	}

	duplicate := couponPublicRequest(http.MethodGet, "/api/h5/coupons/available?target_ref=standard_product:7", &http.Cookie{Name: paymentIdentityCookieName, Value: payment})
	duplicate.AddCookie(&http.Cookie{Name: paymentIdentityCookieName, Value: other})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, duplicate)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("duplicate identity status=%d body=%s", response.Code, response.Body.String())
	}

	cross := couponPublicWrite("/api/h5/coupons/c-7/claim", "", &http.Cookie{Name: paymentIdentityCookieName, Value: payment}, "h5-claim-key-0001")
	cross.Header.Set("Origin", "https://cross.invalid")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, cross)
	if response.Code != http.StatusForbidden || stub.writes != 0 {
		t.Fatalf("cross origin status=%d writes=%d body=%s", response.Code, stub.writes, response.Body.String())
	}

	spoof := couponPublicWrite("/api/h5/coupons/c-7/claim", `{"customer_id":42,"unionid":"forged"}`, &http.Cookie{Name: paymentIdentityCookieName, Value: payment}, "h5-claim-key-0002")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, spoof)
	if response.Code != http.StatusBadRequest || stub.writes != 0 {
		t.Fatalf("spoof status=%d writes=%d body=%s", response.Code, stub.writes, response.Body.String())
	}

	claim := couponPublicWrite("/api/h5/coupons/c-7/claim", "", &http.Cookie{Name: paymentIdentityCookieName, Value: other}, "h5-claim-key-0003")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, claim)
	if response.Code != http.StatusOK || stub.claim.CustomerID != 42 || stub.claim.IdempotencyKey != "h5-claim-key-0003" {
		t.Fatalf("claim status=%d command=%#v body=%s", response.Code, stub.claim, response.Body.String())
	}

	wrongGrant := couponPublicRequest(http.MethodGet, "/api/sidebar/v2/coupons", &http.Cookie{Name: sidebarGrantCookieName, Value: payment})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, wrongGrant)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("payment used as sidebar grant status=%d body=%s", response.Code, response.Body.String())
	}
	validGrant := couponPublicRequest(http.MethodGet, "/api/sidebar/v2/coupons", &http.Cookie{Name: sidebarGrantCookieName, Value: sidebar})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, validGrant)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "customer_id") || !strings.Contains(response.Body.String(), "claim_ref") {
		t.Fatalf("sidebar status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCouponBoardAddsAllFifteenMissingRoutes(t *testing.T) {
	item := legacyCouponItem()
	item.Status = "published"
	payment, sidebar := legacyToken(111), legacyToken(112)
	stub := &legacyCouponBoardStub{legacyCouponStub: legacyCouponStub{item: item}, payment: map[string]int64{payment: 41}, sidebar: map[string]int64{sidebar: 41}, items: []couponport.Coupon{item}, claims: couponport.ClaimPage{Items: []couponport.Claim{}, Limit: 50}, rows: []couponport.SidebarCoupon{}}
	router := legacyCouponBoardRouter(t, stub)
	adminRead := func(path string) *http.Request { return legacyRequest(http.MethodGet, path, legacyToken(113)) }
	publicRead := func(path string) *http.Request { return couponPublicRequest(http.MethodGet, path, nil) }
	for _, testCase := range []struct {
		name       string
		request    *http.Request
		wantStatus int
	}{
		{"admin list page", adminRead("/admin/coupons"), http.StatusFound},
		{"admin new page", adminRead("/admin/coupons/new"), http.StatusOK},
		{"admin data page", adminRead("/admin/coupons/7/data"), http.StatusOK},
		{"admin edit page", adminRead("/admin/coupons/7/edit"), http.StatusOK},
		{"product options", adminRead("/api/admin/coupons/product-options?q=%E5%95%86%E5%93%81&product_type=standard_product&limit=20&offset=0"), http.StatusOK},
		{"delete", couponBoardAdminWrite(http.MethodDelete, "/api/admin/coupons/7"), http.StatusOK},
		{"archive", couponBoardAdminWrite(http.MethodPost, "/api/admin/coupons/7/archive"), http.StatusOK},
		{"claims", adminRead("/api/admin/coupons/7/claims?limit=50&offset=0"), http.StatusOK},
		{"copy", couponBoardAdminWrite(http.MethodPost, "/api/admin/coupons/7/copy"), http.StatusOK},
		{"share", adminRead("/api/admin/coupons/7/share"), http.StatusOK},
		{"h5 available", couponPublicRequest(http.MethodGet, "/api/h5/coupons/available?target_ref=standard_product:7", &http.Cookie{Name: paymentIdentityCookieName, Value: payment}), http.StatusOK},
		{"h5 coupon", publicRead("/api/h5/coupons/c-7"), http.StatusOK},
		{"h5 claim", couponPublicWrite("/api/h5/coupons/c-7/claim", "", &http.Cookie{Name: paymentIdentityCookieName, Value: payment}, "h5-claim-key-0015"), http.StatusOK},
		{"sidebar", couponPublicRequest(http.MethodGet, "/api/sidebar/v2/coupons", &http.Cookie{Name: sidebarGrantCookieName, Value: sidebar}), http.StatusOK},
		{"public page", publicRead("/c/c-7"), http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, testCase.request)
			if response.Code != testCase.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCouponCopyRejectsCSRFAndInvalidIdempotencyKeysBeforeLocalWrite(t *testing.T) {
	stub := &legacyCouponBoardStub{legacyCouponStub: legacyCouponStub{item: legacyCouponItem()}}
	router := legacyCouponBoardRouter(t, stub)
	for _, request := range []*http.Request{
		func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/api/admin/coupons/7/copy", nil)
			r.Header.Set("Origin", "http://example.com")
			r.Header.Set("Idempotency-Key", "coupon-board-key-0001")
			r.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(116)})
			return r
		}(),
		func() *http.Request {
			r := couponBoardAdminWrite(http.MethodPost, "/api/admin/coupons/7/copy")
			r.Header.Set("Idempotency-Key", "too-short")
			return r
		}(),
		func() *http.Request {
			r := couponBoardAdminWrite(http.MethodPost, "/api/admin/coupons/7/copy")
			r.Header.Add("Idempotency-Key", "coupon-board-key-0002")
			return r
		}(),
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden && response.Code != http.StatusBadRequest || stub.writes != 0 {
			t.Fatalf("status/writes=%d/%d body=%s", response.Code, stub.writes, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, couponBoardAdminWrite(http.MethodPost, "/api/admin/coupons/7/copy"))
	if response.Code != http.StatusOK || stub.writes != 1 || stub.copyID != 7 || stub.copyKey != "coupon-board-key-0001" {
		t.Fatalf("status/writes/copy=%d/%d/%d/%q body=%s", response.Code, stub.writes, stub.copyID, stub.copyKey, response.Body.String())
	}
}

func TestCouponListPageRedirectsToTheFrozenSPACarrier(t *testing.T) {
	stub := &legacyCouponBoardStub{}
	router, auth := legacyCouponBoardRouterWithAuth(t, stub)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/admin/coupons", legacyToken(114)))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fcoupons" || stub.limit != 0 || stub.writes != 0 {
		t.Fatalf("response=%d location=%q list-limit=%d writes=%d", response.Code, response.Header().Get("Location"), stub.limit, stub.writes)
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityCouponsRead {
		t.Fatalf("capabilities=%v", got)
	}
}
