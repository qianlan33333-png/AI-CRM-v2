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
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type legacyProductStub struct {
	page        productport.LegacyPage
	product     productport.Product
	err         error
	lastLimit   int32
	lastOffset  int32
	lastID      productport.ID
	lastCommand productport.CreateCommand
}

func (stub *legacyProductStub) ListLegacy(_ context.Context, limit, offset int32) (productport.LegacyPage, error) {
	stub.lastLimit, stub.lastOffset = limit, offset
	return stub.page, stub.err
}
func (stub *legacyProductStub) Get(_ context.Context, id productport.ID) (productport.Product, error) {
	stub.lastID = id
	return stub.product, stub.err
}
func (stub *legacyProductStub) Create(_ context.Context, command productport.CreateCommand) (productport.Product, error) {
	stub.lastCommand = command
	if stub.err != nil {
		return productport.Product{}, stub.err
	}
	result := stub.product
	result.ProductCode, result.Name, result.Description = command.ProductCode, command.Name, command.Description
	result.PriceMinor, result.Currency, result.StockQuantity = command.PriceMinor, strings.ToUpper(command.Currency), command.StockQuantity
	result.Images = append([]string(nil), command.Images...)
	result.CreatedBy, result.LegacyAdminProjection = command.Actor, append([]byte(nil), command.LegacyAdminProjection...)
	return result, nil
}

func TestI01ALegacyProductRoutesRoundTripProjectionWithoutEffects(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 30, 0, 0, time.UTC)
	base := productport.Product{ID: 81, ProductCode: "sku-legacy-81", Name: "旧 UI 商品", Description: "说明", PriceMinor: 9900,
		Currency: "CNY", StockQuantity: 0, Images: []string{}, CreatedBy: 1, CreatedAt: now, UpdatedAt: now,
		LegacyAdminProjection: productapp.DefaultLegacyAdminProjection()}
	stub := &legacyProductStub{product: base, page: productport.LegacyPage{Items: []productport.Product{base}, Total: 17, Limit: 5, Offset: 10}}
	router := legacyProductRouter(t, &legacyAuthStub{}, stub)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, legacyRequest(http.MethodGet, "/api/admin/wechat-pay/products?limit=5&offset=10", legacyToken(31)))
	if list.Code != http.StatusOK || stub.lastLimit != 5 || stub.lastOffset != 10 {
		t.Fatalf("list status/page=%d/%d/%d body=%s", list.Code, stub.lastLimit, stub.lastOffset, list.Body.String())
	}
	var listBody map[string]any
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil || listBody["ok"] != true || listBody["total"] != float64(17) || listBody["limit"] != float64(5) || listBody["offset"] != float64(10) {
		t.Fatalf("list body=%#v err=%v", listBody, err)
	}
	assertNoLegacyProductEffects(t, listBody)
	items, ok := listBody["items"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["product_code"] != "sku-legacy-81" {
		t.Fatalf("list items=%#v", listBody["items"])
	}

	detail := httptest.NewRecorder()
	router.ServeHTTP(detail, legacyRequest(http.MethodGet, "/api/admin/wechat-pay/products/81", legacyToken(32)))
	if detail.Code != http.StatusOK || stub.lastID != 81 {
		t.Fatalf("detail status/id=%d/%d body=%s", detail.Code, stub.lastID, detail.Body.String())
	}
	var detailBody map[string]any
	if err := json.NewDecoder(detail.Body).Decode(&detailBody); err != nil || detailBody["ok"] != true {
		t.Fatalf("detail body=%#v err=%v", detailBody, err)
	}
	assertNoLegacyProductEffects(t, detailBody)
	detailProduct, ok := detailBody["product"].(map[string]any)
	if !ok || detailProduct["product_code"] != "sku-legacy-81" || detailProduct["stock_quantity"] != float64(0) {
		t.Fatalf("detail product=%#v", detailBody["product"])
	}

	body := `{"product_code":"sku-legacy-81","title":"旧 UI 商品","description":"说明","price_cents":9900,"amount_total":9900,"currency":"cny","status":"active","enabled":true,"buy_button_text":"立即购买","require_mobile":true,"lead_program_id":12,"lead_channel_id":13,"lead_qr_title":"标题","lead_qr_subtitle":"副标题","completion_redirect_enabled":true,"completion_redirect_url":"/done","completion_target":{"kind":"page","id":9},"wecom_tagging":{"tag_ids":[1,2]},"slices":[{"image_id":71,"kind":"image"}]}`
	createRequest := httptest.NewRequest(http.MethodPost, "/api/admin/wechat-pay/products", strings.NewReader(body))
	createRequest.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(33)})
	createRequest.Header.Set("X-CSRF-Token", legacyToken(34))
	create := httptest.NewRecorder()
	router.ServeHTTP(create, createRequest)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	if stub.lastCommand.ProductCode != "sku-legacy-81" || stub.lastCommand.Name != "旧 UI 商品" || stub.lastCommand.PriceMinor != 9900 || stub.lastCommand.StockQuantity != 0 || len(stub.lastCommand.Images) != 0 || !strings.HasPrefix(stub.lastCommand.IdempotencyKey, "legacy-product-code:") {
		t.Fatalf("legacy command=%+v", stub.lastCommand)
	}
	var createBody map[string]any
	if err := json.NewDecoder(create.Body).Decode(&createBody); err != nil {
		t.Fatal(err)
	}
	assertNoLegacyProductEffects(t, createBody)
	product := createBody["product"].(map[string]any)
	if product["title"] != "旧 UI 商品" || product["status"] != "active" || product["enabled"] != true || product["stock_quantity"] != float64(0) || product["sold_count"] != float64(0) {
		t.Fatalf("created product=%#v", product)
	}
	if slices, ok := product["slices"].([]any); !ok || len(slices) != 1 || slices[0].(map[string]any)["image_id"] != float64(71) {
		t.Fatalf("passive slices=%#v", product["slices"])
	}
	if tagging := product["wecom_tagging"].(map[string]any); len(tagging["tag_ids"].([]any)) != 2 {
		t.Fatalf("tagging=%#v", tagging)
	}
}

func TestI01ALegacyProductRoutesRejectBoundaryAndErrorCases(t *testing.T) {
	base := productport.Product{ID: 1, ProductCode: "sku", Name: "商品", PriceMinor: 1, Currency: "CNY", CreatedBy: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), LegacyAdminProjection: productapp.DefaultLegacyAdminProjection()}
	for _, target := range []string{
		"/api/admin/wechat-pay/products?limit=0",
		"/api/admin/wechat-pay/products?limit=101",
		"/api/admin/wechat-pay/products?offset=-1",
		"/api/admin/wechat-pay/products?cursor=forbidden",
	} {
		response := httptest.NewRecorder()
		legacyProductRouter(t, &legacyAuthStub{}, &legacyProductStub{product: base}).ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(35)))
		assertLegacyProductError(t, response, http.StatusBadRequest, "MALFORMED_REQUEST")
	}

	tests := []struct {
		name, body string
	}{
		{"unknown field", `{"product_code":"sku","title":"商品","price_cents":1,"currency":"CNY","status":"draft","enabled":false,"unknown_field":"forbidden"}`},
		{"missing status", `{"product_code":"sku","title":"商品","price_cents":1,"currency":"CNY","enabled":false}`},
		{"conflicting prices", `{"product_code":"sku","title":"商品","price_cents":1,"amount_total":2,"currency":"CNY","status":"draft","enabled":false}`},
		{"invalid slices", `{"product_code":"sku","title":"商品","price_cents":1,"currency":"CNY","status":"draft","enabled":false,"slices":{}}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/admin/wechat-pay/products", strings.NewReader(testCase.body))
			request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(36)})
			request.Header.Set("X-CSRF-Token", legacyToken(37))
			response := httptest.NewRecorder()
			legacyProductRouter(t, &legacyAuthStub{}, &legacyProductStub{product: base}).ServeHTTP(response, request)
			assertLegacyProductError(t, response, http.StatusBadRequest, "MALFORMED_REQUEST")
		})
	}

	notFound := httptest.NewRecorder()
	legacyProductRouter(t, &legacyAuthStub{}, &legacyProductStub{err: productapp.ErrNotFound}).ServeHTTP(notFound, legacyRequest(http.MethodGet, "/api/admin/wechat-pay/products/999", legacyToken(38)))
	assertLegacyProductError(t, notFound, http.StatusNotFound, "NOT_FOUND")

	conflict := httptest.NewRecorder()
	legacyProductRouter(t, &legacyAuthStub{}, &legacyProductStub{err: productapp.ErrConflict}).ServeHTTP(conflict, legacyRequest(http.MethodGet, "/api/admin/wechat-pay/products/1", legacyToken(39)))
	assertLegacyProductError(t, conflict, http.StatusBadRequest, "MALFORMED_REQUEST")

	internal := httptest.NewRecorder()
	legacyProductRouter(t, &legacyAuthStub{}, &legacyProductStub{err: productapp.ErrUnavailable}).ServeHTTP(internal, legacyRequest(http.MethodGet, "/api/admin/wechat-pay/products/1", legacyToken(40)))
	assertLegacyProductError(t, internal, http.StatusInternalServerError, "INTERNAL_ERROR")
}

func TestLegacyProductListPageIsCarrierOnly(t *testing.T) {
	stub := &legacyProductStub{}
	router := legacyProductRouter(t, &legacyAuthStub{}, stub)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, legacyProductPagePath, legacyToken(53)))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fwechat-pay%2Fproducts" || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || stub.lastLimit != 0 {
		t.Fatalf("status/headers/read=%d/%q/%q/%q/%d", response.Code, response.Header().Get("Location"), response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"), stub.lastLimit)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, legacyProductPagePath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || stub.lastLimit != 0 {
			t.Fatalf("method/status/allow/read=%s/%d/%q/%d", method, response.Code, response.Header().Get("Allow"), stub.lastLimit)
		}
	}
	for _, test := range []struct {
		name    string
		request *http.Request
		want    int
	}{
		{name: "anonymous", request: httptest.NewRequest(http.MethodGet, legacyProductPagePath, nil), want: http.StatusUnauthorized},
		{name: "sales", request: legacyRequest(http.MethodGet, legacyProductPagePath, legacyToken(54)), want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &legacyAuthStub{}
			if test.name == "sales" {
				staff := int64(9)
				service.principal = authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staff}
			}
			result := httptest.NewRecorder()
			legacyProductRouter(t, service, &legacyProductStub{}).ServeHTTP(result, test.request)
			if result.Code != test.want {
				t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
			}
		})
	}
}

func legacyProductRouter(t *testing.T, service authport.Service, products legacyProductApplication) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundAndProducts(service, &legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, products)
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func assertNoLegacyProductEffects(t *testing.T, payload map[string]any) {
	t.Helper()
	for _, key := range []string{"real_external_call_executed", "payment_request_executed", "real_wechat_pay_executed", "real_alipay_executed", "provider_signature_verified", "real_refund_executed"} {
		if payload[key] != false {
			t.Fatalf("%s=%#v, want false; payload=%#v", key, payload[key], payload)
		}
	}
	if payload["source_status"] != "v2_product_catalog" || payload["fallback_used"] != false {
		t.Fatalf("source receipt=%#v", payload)
	}
}

func assertLegacyProductError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload["code"] != code || payload["message"] == "" || payload["request_id"] == "" {
		t.Fatalf("payload=%#v err=%v", payload, err)
	}
	for _, forbidden := range []string{"ok", "error", "error_code", "detail", "fallback_used", "real_external_call_executed", "payment_request_executed", "real_wechat_pay_executed", "real_alipay_executed", "provider_signature_verified", "real_refund_executed", "source_status"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("platform error leaked legacy success field %s: %#v", forbidden, payload)
		}
	}
}
