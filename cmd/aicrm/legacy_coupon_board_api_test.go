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
func (s *legacyCouponBoardStub) Copy(_ context.Context, _ couponport.ID, _ int64, _ string) (couponport.Coupon, error) {
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
		name    string
		request *http.Request
	}{
		{"admin list page", adminRead("/admin/coupons")},
		{"admin new page", adminRead("/admin/coupons/new")},
		{"admin data page", adminRead("/admin/coupons/7/data")},
		{"admin edit page", adminRead("/admin/coupons/7/edit")},
		{"product options", adminRead("/api/admin/coupons/product-options?q=%E5%95%86%E5%93%81&product_type=standard_product&limit=20&offset=0")},
		{"delete", couponBoardAdminWrite(http.MethodDelete, "/api/admin/coupons/7")},
		{"archive", couponBoardAdminWrite(http.MethodPost, "/api/admin/coupons/7/archive")},
		{"claims", adminRead("/api/admin/coupons/7/claims?limit=50&offset=0")},
		{"copy", couponBoardAdminWrite(http.MethodPost, "/api/admin/coupons/7/copy")},
		{"share", adminRead("/api/admin/coupons/7/share")},
		{"h5 available", couponPublicRequest(http.MethodGet, "/api/h5/coupons/available?target_ref=standard_product:7", &http.Cookie{Name: paymentIdentityCookieName, Value: payment})},
		{"h5 coupon", publicRead("/api/h5/coupons/c-7")},
		{"h5 claim", couponPublicWrite("/api/h5/coupons/c-7/claim", "", &http.Cookie{Name: paymentIdentityCookieName, Value: payment}, "h5-claim-key-0015")},
		{"sidebar", couponPublicRequest(http.MethodGet, "/api/sidebar/v2/coupons", &http.Cookie{Name: sidebarGrantCookieName, Value: sidebar})},
		{"public page", publicRead("/c/c-7")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, testCase.request)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCouponListPageFiltersLocallyWithoutChangingCouponBoardContracts(t *testing.T) {
	unsafeName := `Alpha <img src=x onerror=alert(1)>`
	items := []couponport.Coupon{
		{Name: unsafeName, Status: "published", AvailabilityStatus: "scheduled"},
		{Name: "active coupon", Status: "published", AvailabilityStatus: "active"},
		{Name: "sold out coupon", Status: "published", AvailabilityStatus: "sold_out"},
		{Name: "ended coupon", Status: "published", AvailabilityStatus: "ended"},
		{Name: "draft coupon", Status: "draft"},
		{Name: "stopped coupon", Status: "stopped"},
		{Name: "archived coupon", Status: "archived"},
	}
	stub := &legacyCouponBoardStub{legacyCouponStub: legacyCouponStub{page: couponport.Page{Items: items, Total: int64(len(items)), Limit: 100}}}
	router, auth := legacyCouponBoardRouterWithAuth(t, stub)
	request := legacyRequest(http.MethodGet, "/admin/coupons", legacyToken(114))
	originalURL := request.URL.String()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	body := response.Body.String()

	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("response=%d headers=%v body=%s", response.Code, response.Header(), body)
	}
	if stub.limit != 100 || stub.offset != 0 || stub.status != "" || stub.query != "" || stub.writes != 0 {
		t.Fatalf("list arguments limit=%d offset=%d status=%q query=%q writes=%d", stub.limit, stub.offset, stub.status, stub.query, stub.writes)
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityCouponsRead {
		t.Fatalf("capabilities=%v", got)
	}
	if request.URL.String() != originalURL || strings.Contains(body, "fetch(") || strings.Contains(body, "XMLHttpRequest") || strings.Contains(body, "history.") {
		t.Fatalf("unexpected browser request mutation or remote call body=%s", body)
	}
	if !strings.Contains(body, `type="search" id="coupon-search"`) || !strings.Contains(body, `id="coupon-status"`) || !strings.Contains(body, "row.hidden=") || !strings.Contains(body, "row.dataset.name.toLowerCase().includes(needle)") || !strings.Contains(body, `row.dataset.status===wanted`) {
		t.Fatalf("missing local filter controls body=%s", body)
	}
	for _, status := range []string{"draft", "scheduled", "active", "sold_out", "ended", "stopped", "archived"} {
		if !strings.Contains(body, `value="`+status+`"`) || !strings.Contains(body, `data-status="`+status+`"`) {
			t.Fatalf("missing status %q body=%s", status, body)
		}
	}
	for _, item := range []struct{ name, status string }{
		{"active coupon", "active"},
		{"sold out coupon", "sold_out"},
		{"ended coupon", "ended"},
		{"draft coupon", "draft"},
		{"stopped coupon", "stopped"},
		{"archived coupon", "archived"},
	} {
		if !strings.Contains(body, item.name+" · "+item.status) {
			t.Fatalf("missing displayed availability state %#v body=%s", item, body)
		}
	}
	if strings.Contains(body, `data-status="published"`) || strings.Contains(body, " · published") {
		t.Fatalf("stored status leaked instead of availability status body=%s", body)
	}
	if !strings.Contains(body, `data-name="Alpha &lt;img src=x onerror=alert(1)&gt;"`) || !strings.Contains(body, `Alpha &lt;img src=x onerror=alert(1)&gt; · scheduled`) || strings.Contains(body, unsafeName) {
		t.Fatalf("unsafe coupon name was not escaped body=%s", body)
	}
	if strings.Contains(renderCouponPageBody(t), `id="coupon-search"`) {
		t.Fatal("coupon filter leaked into a non-list coupon page")
	}
}

func TestCouponListPageFailsClosedForUnexpectedAvailabilityStatus(t *testing.T) {
	for _, item := range []couponport.Coupon{
		{Name: "published without availability", Status: "published"},
		{Name: "deleted availability", Status: "published", AvailabilityStatus: "deleted"},
	} {
		stub := &legacyCouponBoardStub{legacyCouponStub: legacyCouponStub{page: couponport.Page{Items: []couponport.Coupon{item}, Total: 1, Limit: 100}}}
		router := legacyCouponBoardRouter(t, stub)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/admin/coupons", legacyToken(115)))
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), item.Name) {
			t.Fatalf("unexpected availability status item=%#v response=%d body=%s", item, response.Code, response.Body.String())
		}
	}
}

func renderCouponPageBody(t *testing.T) string {
	t.Helper()
	response := httptest.NewRecorder()
	renderCouponPage(response, "新建优惠券", nil, couponport.Coupon{})
	return response.Body.String()
}
