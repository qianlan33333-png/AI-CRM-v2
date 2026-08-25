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
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

type legacyOrderBoardStub struct {
	filter         orderport.BoardFilter
	listCalls      int
	refundCommand  orderport.RefundCommand
	previewCommand orderport.ExportCommand
	exportCommand  orderport.ExportCommand
	getActor       int64
	retryID        int64
	page           orderport.Page
	refund         orderport.Refund
}

func (s *legacyOrderBoardStub) ListOrders(_ context.Context, filter orderport.BoardFilter) (orderport.Page, error) {
	s.listCalls++
	s.filter = filter
	return s.page, nil
}
func (*legacyOrderBoardStub) GetOrder(context.Context, string, string) (orderport.Detail, error) {
	return orderport.Detail{}, nil
}
func (*legacyOrderBoardStub) ListRefunds(context.Context, orderport.RefundFilter) (orderport.RefundPage, error) {
	return orderport.RefundPage{}, nil
}
func (s *legacyOrderBoardStub) PreviewExport(_ context.Context, command orderport.ExportCommand) (orderport.ExportPreview, error) {
	s.previewCommand = command
	return orderport.ExportPreview{}, nil
}
func (s *legacyOrderBoardStub) CreateExport(_ context.Context, command orderport.ExportCommand) (orderport.ExportJob, error) {
	s.exportCommand = command
	return orderport.ExportJob{}, nil
}
func (s *legacyOrderBoardStub) GetExport(_ context.Context, _ string, actor int64) (orderport.ExportJob, error) {
	s.getActor = actor
	return orderport.ExportJob{}, nil
}
func (s *legacyOrderBoardStub) RequestRefund(_ context.Context, command orderport.RefundCommand) (orderport.Refund, error) {
	s.refundCommand = command
	return s.refund, nil
}
func (*legacyOrderBoardStub) ListExternalEffects(context.Context, string, string) (orderport.ExternalEffectPage, error) {
	return orderport.ExternalEffectPage{}, nil
}
func (s *legacyOrderBoardStub) RequestExternalEffectRetry(_ context.Context, effectID, _ int64, _ string) (orderport.ExternalEffect, error) {
	s.retryID = effectID
	return orderport.ExternalEffect{}, nil
}

func TestOrderABRootFiltersAliasesAndUsesOrderRead(t *testing.T) {
	now := time.Date(2026, 8, 15, 17, 0, 0, 0, time.UTC)
	stub := &legacyOrderBoardStub{page: orderport.Page{Items: []orderport.Item{{OrderNo: "M-1", MerchantOrderNo: "M-1", OutTradeNo: "M-1", TransactionID: "WX-1", PlatformTransactionNo: "WX-1", PayerName: "张三", Mobile: "13800000000", ProductCode: "SKU-1", ProductName: "商品", AmountYuan: "19.90", Currency: "CNY", Status: "paid", StatusLabel: "已支付", Provider: "wechat", ProviderLabel: "微信支付", DetailURL: "/api/admin/orders/1", CreatedAt: now}}, Total: 1, Limit: 1}}
	router, auth := legacyOrderBoardRouter(t, stub)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/orders?status=paid&payment_status=paid&external_userid=wmid-1&identity=wmid-1&transaction_id=WX-1&platform_transaction_no=WX-1&order_no=M-1&out_trade_no=M-1&date_from=2026-08-01&date_to=2026-08-31&limit=1", legacyToken(99)))
	if response.Code != http.StatusOK || stub.filter.Status != "paid" || stub.filter.Identity != "wmid-1" || stub.filter.TransactionID != "WX-1" || stub.filter.OrderNo != "M-1" || stub.filter.Limit != 1 || stub.filter.CreatedFrom == nil || stub.filter.CreatedTo == nil {
		t.Fatalf("status=%d filter=%+v body=%s", response.Code, stub.filter, response.Body.String())
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityOrderRead {
		t.Fatalf("capabilities=%v", got)
	}
}

func TestOrderBoardProviderListsUseReadRBAC(t *testing.T) {
	for _, test := range []struct {
		name     string
		path     string
		provider string
		limit    int32
	}{
		{name: "wechat pay", path: "/api/admin/wechat-pay/orders?status=paid&limit=20", provider: "wechat", limit: 20},
		{name: "wechat shop", path: "/api/admin/orders?provider=wechat_shop&status=paid&limit=50", provider: "wechat_shop", limit: 50},
		{name: "alipay", path: "/api/admin/alipay/transactions?payment_status=paid&limit=50", provider: "alipay", limit: 50},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyOrderBoardStub{}
			router, auth := legacyOrderBoardRouter(t, stub)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(99)))
			if response.Code != http.StatusOK || stub.filter.Provider != test.provider || stub.filter.Status != "paid" || stub.filter.Limit != test.limit {
				t.Fatalf("status=%d filter=%+v body=%s", response.Code, stub.filter, response.Body.String())
			}
			if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityOrderRead {
				t.Fatalf("capabilities=%v", got)
			}
		})
	}
}

func TestOrderBoardProviderFilteredExportsRequireCSRFAndUseServerActor(t *testing.T) {
	csrf := legacyToken(0x31)
	for _, test := range []struct {
		name     string
		provider string
	}{
		{name: "wechat", provider: "wechat"},
		{name: "wechat shop", provider: "wechat_shop"},
		{name: "alipay", provider: "alipay"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyOrderBoardStub{}
			auth := &orderBoardExportAuth{expectedCSRF: csrf}
			router := legacyOrderBoardRouterWithService(t, stub, auth)
			body := `{"resource":"orders","format":"csv","filter":{"provider":"` + test.provider + `","status":"paid"}}`

			missing := legacyChannelWriteRequest(http.MethodPost, "/api/admin/exports", body)
			missing.Header.Set("Idempotency-Key", "commerce-export-key-0001")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, missing)
			if response.Code != http.StatusForbidden || auth.csrfCalls != 1 || stub.exportCommand.Actor != 0 {
				t.Fatalf("missing csrf status=%d csrf=%d command=%+v body=%s", response.Code, auth.csrfCalls, stub.exportCommand, response.Body.String())
			}

			auth.csrfCalls, auth.capabilities = 0, nil
			request := legacyChannelWriteRequest(http.MethodPost, "/api/admin/exports", body)
			request.Header.Set("Idempotency-Key", "commerce-export-key-0001")
			request.Header.Set("X-CSRF-Token", csrf)
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || auth.csrfCalls != 1 || stub.exportCommand.Actor != 41 || stub.exportCommand.IdempotencyKey != "commerce-export-key-0001" || stub.exportCommand.Resource != "orders" || stub.exportCommand.Format != "csv" || stub.exportCommand.Filter.Provider != test.provider || stub.exportCommand.Filter.Status != "paid" {
				t.Fatalf("status=%d csrf=%d command=%+v body=%s", response.Code, auth.csrfCalls, stub.exportCommand, response.Body.String())
			}
			if len(auth.capabilities) != 1 || auth.capabilities[0] != authport.CapabilityOrderWrite {
				t.Fatalf("capabilities=%v", auth.capabilities)
			}
		})
	}
}

func TestOrderListPageIsAnAuthorizedCarrierOnly(t *testing.T) {
	for _, test := range []struct {
		name      string
		principal authport.Principal
		token     bool
		want      int
	}{
		{name: "admin", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, token: true, want: http.StatusFound},
		{name: "ops", principal: authport.Principal{AdminUserID: 8, Role: authport.RoleOps}, token: true, want: http.StatusFound},
		{name: "sales", principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales}, token: true, want: http.StatusForbidden},
		{name: "anonymous", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, want: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			board := &legacyOrderBoardStub{}
			auth := &orderPageAuthSpy{principal: test.principal}
			router := orderPageRouter(t, auth, board)
			request := httptest.NewRequest(http.MethodGet, legacyOrderPagePath, nil)
			if test.token {
				request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(171)})
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			wantCache := "no-store"
			if test.want == http.StatusFound {
				wantCache = "private, no-store"
			}
			if response.Code != test.want || response.Header().Get("Cache-Control") != wantCache || response.Header().Get("X-Content-Type-Options") != "nosniff" || auth.csrfCalls != 0 || board.listCalls != 0 {
				t.Fatalf("status/headers/csrf/list=%d/%q/%q/%d/%d", response.Code, response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"), auth.csrfCalls, board.listCalls)
			}
			if test.want == http.StatusFound && (response.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Forders" || auth.authenticateCalls != 1 || auth.authorizeCalls != 1 || len(auth.capabilities) != 1 || auth.capabilities[0] != authport.CapabilityOrderRead) {
				t.Fatalf("location/auth/capability=%q/%d/%d/%v", response.Header().Get("Location"), auth.authenticateCalls, auth.authorizeCalls, auth.capabilities)
			}
		})
	}
}

func TestOrderListPageRejectsOtherMethodsBeforeAuthentication(t *testing.T) {
	auth := &orderPageAuthSpy{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
	router := orderPageRouter(t, auth, &legacyOrderBoardStub{})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, legacyOrderPagePath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("method/status/headers=%s/%d/%q/%q/%q", method, response.Code, response.Header().Get("Allow"), response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
		}
	}
	if auth.authenticateCalls != 0 || auth.authorizeCalls != 0 || auth.csrfCalls != 0 {
		t.Fatalf("authenticate/authorize/csrf=%d/%d/%d", auth.authenticateCalls, auth.authorizeCalls, auth.csrfCalls)
	}
}

type orderPageAuthSpy struct {
	principal         authport.Principal
	authenticateCalls int
	authorizeCalls    int
	csrfCalls         int
	capabilities      []authport.Capability
}

func (spy *orderPageAuthSpy) Authenticate(_ context.Context, _ authport.SessionRef) (authport.Principal, error) {
	spy.authenticateCalls++
	return spy.principal, nil
}

func (spy *orderPageAuthSpy) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	spy.authorizeCalls++
	spy.capabilities = append(spy.capabilities, capability)
	if principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) || capability != authport.CapabilityOrderRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (spy *orderPageAuthSpy) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	spy.csrfCalls++
	return nil
}

func (*orderPageAuthSpy) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func orderPageRouter(t *testing.T, service authport.Service, board legacyOrderBoardApplication) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.orderBoard = board
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestOrderABRefundFailsClosedWithoutCommerceRefundCapability(t *testing.T) {
	stub := &legacyOrderBoardStub{}
	router, auth := legacyOrderBoardRouter(t, stub)
	request := legacyChannelWriteRequest(http.MethodPost, "/api/admin/refunds", `{"provider":"wechat","order_no":"M-11","refund_amount_total":1990,"reason":"重复支付","transaction_id_confirmation":"WX-11","checked":true,"operator":"spoof"}`)
	request.Header.Set("Idempotency-Key", "dddddddddddddddd")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || stub.refundCommand.Actor != 0 {
		t.Fatalf("status=%d command=%+v body=%s", response.Code, stub.refundCommand, response.Body.String())
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityOrderWrite {
		t.Fatalf("capabilities=%v", got)
	}
}

func TestOrderABRejectsOpaqueCursorInsteadOfGuessingItsCodec(t *testing.T) {
	router, _ := legacyOrderBoardRouter(t, &legacyOrderBoardStub{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/orders?cursor=unproven", legacyToken(98)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"error_code":"invalid_argument"`) {
		t.Fatalf("cursor status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestOrderSafeExportTransportAllowsOnlyTheWhitelistedFilter(t *testing.T) {
	writer := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/exports/preview", strings.NewReader(`{"resource":"orders","format":"csv","filter":{"provider":"wechat","status":"paid","product_code":"sku-1","local_id":11}}`))
	command, err := legacyPreviewExportCommand(writer, request, 7)
	if err != nil || command.Actor != 7 || command.Filter.Provider != "wechat" || command.Filter.Status != "paid" || command.Filter.ProductCode != "sku-1" || command.Filter.LocalID == nil || *command.Filter.LocalID != 11 || command.IdempotencyKey != "" {
		t.Fatalf("command=%+v err=%v", command, err)
	}
	writer = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/exports/preview", strings.NewReader(`{"resource":"orders","format":"csv","filter":{"mobile":"13800000000"}}`))
	if _, err := legacyPreviewExportCommand(writer, request, 7); err == nil {
		t.Fatal("identity filter was accepted")
	}
}

func TestOrderExternalEffectPublicProjectionNeverLeaksProviderReceipt(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	effect := orderport.ExternalEffect{ID: 9, OrderID: 11, Provider: "wechat", EffectKind: "refund", State: "outcome_unknown", AutoRetryAllowed: false, ProviderReceipt: []byte("provider-receipt-must-not-leak"), CreatedAt: now, UpdatedAt: now}
	encoded, err := json.Marshal(mapLegacyOrderExternalEffect(effect))
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"provider-receipt-must-not-leak", `"provider_receipt":`, "base64"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unsafe effect projection leaked %q: %s", forbidden, body)
		}
	}
	for _, required := range []string{`"receipt_state":"present"`, `"provider_receipt_present":true`, `"delivery_proven":false`, `"local_fact_only":true`, `"real_external_call_executed":false`, `"delivery_semantics":"local_state_not_delivery_proof"`} {
		if !strings.Contains(body, required) {
			t.Fatalf("effect projection missing %q: %s", required, body)
		}
	}
}

func legacyOrderBoardRouter(t *testing.T, board legacyOrderBoardApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	return legacyOrderBoardRouterWithService(t, board, service), service
}

func legacyOrderBoardRouterWithService(t *testing.T, board legacyOrderBoardApplication, service authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.orderBoard = board
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

type orderBoardExportAuth struct {
	expectedCSRF string
	csrfCalls    int
	capabilities []authport.Capability
}

var _ authport.Service = (*orderBoardExportAuth)(nil)

func (*orderBoardExportAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, nil
}

func (auth *orderBoardExportAuth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	auth.capabilities = append(auth.capabilities, capability)
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (auth *orderBoardExportAuth) ValidateCSRF(_ context.Context, _ authport.SessionRef, token authport.CSRFToken) error {
	auth.csrfCalls++
	if string(token) != auth.expectedCSRF {
		return authport.ErrCSRFInvalid
	}
	return nil
}

func (*orderBoardExportAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}
