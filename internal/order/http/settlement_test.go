package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestCheckoutGetUsesSnakeCaseNoStoreAndExactJSAPIFields(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(2 * time.Hour)
	application := &settlementApplicationStub{checkout: orderport.Checkout{
		OrderID: 9, MerchantOrderNo: "pe01_0123456789abcdef0123456789abcdef", State: orderport.FinancialAwaitingPayment,
		ProductKind: orderport.ProductKindOrdinary, CustomerID: 7, ProductID: 8, AmountMinor: 9900, Currency: "CNY", PaymentCommandID: 10,
		PayParams:       &orderport.JSAPIHandoff{AppID: "wx-app", TimeStamp: "1787738400", NonceStr: "nonce", Package: "prepay_id=wx-prepay", SignType: "RSA", PaySign: "signature", ExpiresAt: expiresAt},
		PrepayExpiresAt: &expiresAt, CreatedAt: now,
	}}
	handler, err := NewHandler(application, callbackVerifierStub{}, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, CheckoutPath+"/"+application.checkout.MerchantOrderNo, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 71, Role: authport.RoleAdmin}, authport.SessionRef("session"))
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityOrderRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.Get(response, request.WithContext(ctx), application.checkout.MerchantOrderNo)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var body map[string]any
	if err = json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"order_id", "merchant_order_no", "payment_command_id", "pay_params", "prepay_expires_at", "created_at"} {
		if body[key] == nil {
			t.Fatalf("missing %s in %v", key, body)
		}
	}
	if body["OrderID"] != nil || body["PayParams"] != nil {
		t.Fatalf("camel-case checkout fields leaked: %v", body)
	}
	params, ok := body["pay_params"].(map[string]any)
	if !ok || len(params) != 6 {
		t.Fatalf("pay_params=%v", body["pay_params"])
	}
	for _, key := range []string{"appId", "timeStamp", "nonceStr", "package", "signType", "paySign"} {
		if params[key] == nil {
			t.Fatalf("missing pay_params.%s in %v", key, params)
		}
	}
	if params["ExpiresAt"] != nil || params["expires_at"] != nil {
		t.Fatalf("internal expiry leaked inside pay_params: %v", params)
	}
}

type settlementApplicationStub struct{ checkout orderport.Checkout }

func (stub *settlementApplicationStub) Checkout(context.Context, orderport.CheckoutCommand) (orderport.Checkout, error) {
	return stub.checkout, nil
}
func (stub *settlementApplicationStub) ApplyPaymentCallback(context.Context, orderport.PaymentCallbackCommand) (orderport.Checkout, error) {
	return stub.checkout, nil
}
func (*settlementApplicationStub) RequestRefundV2(context.Context, orderport.RefundCommandV2) (orderport.RefundV2, error) {
	return orderport.RefundV2{}, nil
}
func (*settlementApplicationStub) ApplyRefundCallback(context.Context, orderport.RefundCallbackCommand) (orderport.RefundV2, error) {
	return orderport.RefundV2{}, nil
}
func (stub *settlementApplicationStub) GetSelfScoped(context.Context, string, [32]byte) (orderport.Checkout, error) {
	return stub.checkout, nil
}

type callbackVerifierStub struct{}

func (callbackVerifierStub) VerifyPayment(context.Context, []byte, map[string]string) (orderport.PaymentCallbackCommand, error) {
	return orderport.PaymentCallbackCommand{}, nil
}
func (callbackVerifierStub) VerifyRefund(context.Context, []byte, map[string]string) (orderport.RefundCallbackCommand, error) {
	return orderport.RefundCallbackCommand{}, nil
}
