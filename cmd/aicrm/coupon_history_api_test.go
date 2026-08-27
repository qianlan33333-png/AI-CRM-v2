package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
)

type couponHistoryAPIStub struct {
	err           error
	empty         bool
	calls         int
	kind          string
	couponID      int64
	limit, offset int32
}

func (s *couponHistoryAPIStub) ListHistoricalDefinitions(_ context.Context, limit, offset int32) ([]couponport.HistoricalDefinition, int64, error) {
	s.calls++
	s.kind, s.limit, s.offset = "definitions", limit, offset
	if s.empty {
		return nil, 0, s.err
	}
	coupon := legacyCouponItem()
	coupon.Status, coupon.HistoryOnly, coupon.AvailabilityStatus = "archived", true, "archived"
	coupon.IssuedCount = 4
	return []couponport.HistoricalDefinition{{Coupon: coupon, SourceCouponID: 81, OriginalStatus: "stopped", FirstClaimAt: &coupon.CreatedAt}}, 21, s.err
}
func (s *couponHistoryAPIStub) ListHistoricalClaims(_ context.Context, id int64, limit, offset int32) ([]couponport.HistoricalClaim, int64, error) {
	s.calls++
	s.kind, s.couponID, s.limit, s.offset = "claims", id, limit, offset
	if s.empty {
		return nil, 0, s.err
	}
	at := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	return []couponport.HistoricalClaim{{ID: 9, SourceClaimID: 82, SourceCouponID: 81, CouponID: id, Status: "expired", ClaimNo: " original claim ", Currency: "CNY", DiscountAmountTotal: 100, ValidFrom: at, ValidUntil: at.Add(-time.Hour), ClaimedAt: at, CreatedAt: at, UpdatedAt: at}}, 21, s.err
}
func (s *couponHistoryAPIStub) ListHistoricalRedemptions(_ context.Context, id int64, limit, offset int32) ([]couponport.HistoricalRedemption, int64, error) {
	s.calls++
	s.kind, s.couponID, s.limit, s.offset = "redemptions", id, limit, offset
	if s.empty {
		return nil, 0, s.err
	}
	return []couponport.HistoricalRedemption{{ID: 10, SourceRedemptionID: 83, SourceClaimID: 82, SourceOrderID: 84, ClaimHistoryID: 9, Status: "released", ReleaseReason: " original reason ", OriginalAmountTotal: 5, DiscountAmountTotal: 9, PayableAmountTotal: 17, Currency: "CNY"}}, 21, s.err
}

func couponHistoryRouter(t *testing.T, history couponport.HistoricalReader, auth authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.couponHistory = history
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

var couponHistoryPaths = []string{"/api/admin/coupon-history", "/api/admin/coupon-history/7/claims", "/api/admin/coupon-history/7/redemptions"}

func TestCouponHistoryReadOnlyRoutesAndOriginalFacts(t *testing.T) {
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		auth := &couponPageAuthSpy{principal: authport.Principal{AdminUserID: 1, Role: role}}
		history := &couponHistoryAPIStub{}
		router := couponHistoryRouter(t, history, auth)
		for index, path := range couponHistoryPaths {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, path+"?limit=20&offset=20", legacyToken(81)))
			var page struct {
				Source                   string            `json:"source"`
				ReadOnly                 bool              `json:"read_only"`
				RealExternalCallExecuted bool              `json:"real_external_call_executed"`
				CouponID                 int64             `json:"coupon_id"`
				Items                    []json.RawMessage `json:"items"`
				Total                    int64             `json:"total"`
				Limit, Offset            int32
			}
			if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Source != "v1_history" || !page.ReadOnly || page.RealExternalCallExecuted || page.Total != 21 || page.Limit != 20 || page.Offset != 20 || len(page.Items) != 1 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s: %d %s", path, response.Code, response.Body)
			}
			if history.limit != 20 || history.offset != 20 || history.kind != []string{"definitions", "claims", "redemptions"}[index] || index > 0 && (history.couponID != 7 || page.CouponID != 7) {
				t.Fatal("reader lost route filter or pagination")
			}
			if auth.csrfCalls != 0 || auth.capabilities[len(auth.capabilities)-1] != authport.CapabilityCouponsRead {
				t.Fatalf("capabilities/CSRF=%v/%d", auth.capabilities, auth.csrfCalls)
			}
			for _, want := range [][]string{
				{`"history_only":true`, `"status":"archived"`, `"original_status":"stopped"`, `"issued_count":4`},
				{`"customer_id":null`, `"claim_no":" original claim "`, `"status":"expired"`, `"valid_until":"2026-08-27T23:00:00Z"`},
				{`"order_id":null`, `"status":"released"`, `"release_reason":" original reason "`, `"original_amount_total":5`, `"discount_amount_total":9`, `"payable_amount_total":17`},
			}[index] {
				if !strings.Contains(string(page.Items[0]), want) {
					t.Fatalf("lost original field %s: %s", want, page.Items[0])
				}
			}
		}
	}
}

func TestCouponHistoryRejectsInvalidQueryBeforeReading(t *testing.T) {
	history := &couponHistoryAPIStub{}
	router := couponHistoryRouter(t, history, &recordingAuth{})
	for _, path := range couponHistoryPaths {
		for _, query := range []string{"limit=0", "limit=101", "limit=-1", "limit=", "limit=1.5", "limit=1&limit=2", "offset=-1", "offset=", "offset=0&offset=1", "offset=2147483648", "execute=true", "coupon_id=2", "limit=%zz", "limit=1;offset=2"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, path+"?"+query, legacyToken(82)))
			if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s?%s: %d %s", path, query, response.Code, response.Body)
			}
		}
	}
	for _, id := range []string{"0", "-1", "x", "9223372036854775808"} {
		for _, child := range []string{"claims", "redemptions"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/coupon-history/"+id+"/"+child, legacyToken(82)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid ID accepted: %d %s", response.Code, response.Body)
			}
		}
	}
	if history.calls != 0 {
		t.Fatal("invalid request reached reader")
	}
}

func TestCouponHistoryEmptyAndUnavailableAreDistinct(t *testing.T) {
	history := &couponHistoryAPIStub{empty: true}
	router := couponHistoryRouter(t, history, &recordingAuth{})
	for _, path := range couponHistoryPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(83)))
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"items":[]`) || history.limit != 50 || history.offset != 0 {
			t.Fatalf("empty/default page: %d %s", response.Code, response.Body)
		}
	}
	history.err = errors.New("private database detail and customer payload")
	for _, path := range couponHistoryPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(83)))
		if response.Code != 503 || strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), `"items"`) || !strings.Contains(response.Body.String(), `"code":"DEPENDENCY_UNAVAILABLE"`) || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("failure not closed: %d %s", response.Code, response.Body)
		}
	}
	var missing *couponHistoryAPIStub
	router = couponHistoryRouter(t, missing, &recordingAuth{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, couponHistoryPaths[0], legacyToken(83)))
	if response.Code != 503 {
		t.Fatalf("missing reader=%d", response.Code)
	}
}

func TestCouponHistoryRoutesRequireReadAuthorization(t *testing.T) {
	for _, anonymous := range []bool{true, false} {
		history := &couponHistoryAPIStub{}
		auth := &couponPageAuthSpy{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleSales}}
		router := couponHistoryRouter(t, history, auth)
		for _, path := range couponHistoryPaths {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			want := http.StatusUnauthorized
			if !anonymous {
				request = legacyRequest(http.MethodGet, path, legacyToken(84))
				want = http.StatusForbidden
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != want {
				t.Fatalf("authorization=%d want=%d", response.Code, want)
			}
		}
		if history.calls != 0 || auth.csrfCalls != 0 {
			t.Fatal("unauthorized history reached reader or required CSRF")
		}
	}
}
