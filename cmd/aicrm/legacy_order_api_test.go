package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

type legacyOrderStub struct {
	page   orderport.Page
	err    error
	filter orderport.Filter
	calls  int
}

func (stub *legacyOrderStub) List(_ context.Context, filter orderport.Filter) (orderport.Page, error) {
	stub.calls++
	stub.filter = filter
	return stub.page, stub.err
}

func TestI03LegacyOrderRootRouteA01RBACEnvelopeAndAliases(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	stub := &legacyOrderStub{page: orderport.Page{Items: []orderport.Item{{
		CreatedAt: now, MerchantOrderNo: "M-1", OutTradeNo: "M-1", OrderNo: "M-1",
		PlatformTransactionNo: "WX-1", TransactionID: "WX-1", PayerName: "张三", Mobile: "13800000000",
		ExternalUserID: "wmid-1", ProductCode: "SKU-1", ProductName: "商品", AmountYuan: "19.90", Currency: "CNY",
		Status: "paid", StatusLabel: "已支付", Provider: "wechat", ProviderLabel: "微信支付", DetailURL: "/api/admin/orders/1",
	}}, Total: 2, Limit: 1, HasMore: true}}
	router, auth := legacyOrderRouter(t, stub)
	request := legacyRequest(http.MethodGet, "/api/admin/orders?provider=wechat&order_no=M-1&mobile=138&product_code=SKU-1&created_from=2026-08-01T00:00:00%2B08:00&created_to=2026-08-31&status=paid&limit=1&offset=0", legacyToken(91))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.calls != 1 || stub.filter.Provider != "wechat" || stub.filter.Limit != 1 || stub.filter.CreatedFrom == nil || stub.filter.CreatedFrom.Location() != time.UTC {
		t.Fatalf("calls/filter=%d/%+v", stub.calls, stub.filter)
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityOrderRead {
		t.Fatalf("capabilities=%v", got)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope) != 4 || envelope["items"] == nil || envelope["total"] == nil || envelope["limit"] == nil || envelope["has_more"] == nil {
		t.Fatalf("envelope=%s", response.Body.String())
	}
	if body := response.Body.String(); !containsAll(body, `"merchant_order_no":"M-1"`, `"out_trade_no":"M-1"`, `"order_no":"M-1"`, `"platform_transaction_no":"WX-1"`, `"transaction_id":"WX-1"`, `"amount_yuan":"19.90"`, `"external_userid":"wmid-1"`) {
		t.Fatalf("aliases=%s", body)
	}
}

func TestI03LegacyOrderRejectsUnknownRepeatedMalformedAndMissingSession(t *testing.T) {
	stub := &legacyOrderStub{}
	router, _ := legacyOrderRouter(t, stub)
	for _, path := range []string{
		"/api/admin/orders?unknown=1", "/api/admin/orders?status=paid&status=pending",
		"/api/admin/orders?limit=101", "/api/admin/orders?created_from=2026-08-31&created_to=2026-08-01",
		"/api/admin/orders?status=paid;provider=wechat",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(92)))
		if response.Code != http.StatusBadRequest || stub.calls != 0 || !containsAll(response.Body.String(), `"message"`, `"error_code":"invalid_argument"`) {
			t.Fatalf("path=%s status=%d calls=%d body=%s", path, response.Code, stub.calls, response.Body.String())
		}
	}
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/admin/orders", nil))
	if missing.Code != http.StatusUnauthorized || stub.calls != 0 {
		t.Fatalf("missing session status=%d calls=%d body=%s", missing.Code, stub.calls, missing.Body.String())
	}
}

func TestI03LegacyOrderSanitizesUnavailable(t *testing.T) {
	stub := &legacyOrderStub{err: orderapp.ErrUnavailable}
	router, _ := legacyOrderRouter(t, stub)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/orders", legacyToken(93)))
	if response.Code != http.StatusServiceUnavailable || !containsAll(response.Body.String(), `"message":"order list unavailable"`, `"error_code":"unavailable"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func legacyOrderRouter(t *testing.T, orders legacyOrderApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.orders = orders
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
