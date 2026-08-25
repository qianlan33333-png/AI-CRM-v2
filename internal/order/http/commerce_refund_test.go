package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestCommerceRefundHandlersUseTypedCanonicalCommands(t *testing.T) {
	pay := &fakePayCompatibility{}
	shop := &fakeShopRefundApplication{}
	handler, err := NewCommerceRefundHandler(pay, shop, fakeShopCallbackVerifier{})
	if err != nil {
		t.Fatal(err)
	}

	payRequest := authenticatedRefundRequest(t, "/api/admin/wechat-pay/orders/M-1/refunds", `{"refund_amount_total":1990,"reason":"duplicate","transaction_id_confirmation":"TX-1","checked":true}`)
	payResponse := httptest.NewRecorder()
	handler.WeChatPayCompatibility(payResponse, payRequest, "M-1")
	if payResponse.Code != http.StatusAccepted || pay.command.OrderReference != "M-1" || pay.command.TransactionIDConfirmation != "TX-1" || pay.command.Actor != 71 {
		t.Fatalf("status=%d command=%+v body=%s", payResponse.Code, pay.command, payResponse.Body.String())
	}
	var payBody map[string]any
	if err = json.Unmarshal(payResponse.Body.Bytes(), &payBody); err != nil || payBody["order_id"] != float64(11) || payBody["OrderID"] != nil {
		t.Fatalf("body=%v error=%v", payBody, err)
	}

	shopRequest := authenticatedRefundRequest(t, "/api/admin/refunds", `{"provider":"wechat_shop","order_no":"S-1","refund_amount_total":880,"reason":"return","transaction_id_confirmation":"STX-1","checked":true}`)
	shopResponse := httptest.NewRecorder()
	handler.WeChatShopCompatibility(shopResponse, shopRequest)
	if shopResponse.Code != http.StatusAccepted || shop.command.OrderReference != "S-1" || shop.command.TransactionIDConfirmation != "STX-1" {
		t.Fatalf("status=%d command=%+v body=%s", shopResponse.Code, shop.command, shopResponse.Body.String())
	}
	var shopBody map[string]any
	if err = json.Unmarshal(shopResponse.Body.Bytes(), &shopBody); err != nil || shopBody["provider_accepted"] != false || shopBody["delivery_proven"] != false {
		t.Fatalf("body=%v error=%v", shopBody, err)
	}
	for _, field := range []string{"provider_acceptance_digest", "provider_refund_digest", "settlement_digest"} {
		if shopBody[field] != nil {
			t.Fatalf("response leaked %s: %v", field, shopBody)
		}
	}
}

func TestWeChatShopCompatibilityRejectsWeChatPayProvider(t *testing.T) {
	handler, _ := NewCommerceRefundHandler(&fakePayCompatibility{}, &fakeShopRefundApplication{}, fakeShopCallbackVerifier{})
	request := authenticatedRefundRequest(t, "/api/admin/refunds", `{"provider":"wechat","order_no":"M-1","refund_amount_total":1,"reason":"x","transaction_id_confirmation":"TX-1","checked":true}`)
	response := httptest.NewRecorder()
	handler.WeChatShopCompatibility(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCommerceRefundCompatibilityRejectsUnknownFields(t *testing.T) {
	handler, _ := NewCommerceRefundHandler(&fakePayCompatibility{}, &fakeShopRefundApplication{}, fakeShopCallbackVerifier{})
	request := authenticatedRefundRequest(t, "/api/admin/refunds", `{"provider":"wechat_shop","order_no":"M-1","refund_amount_total":1,"reason":"x","transaction_id_confirmation":"TX-1","checked":true,"unapproved_field":"no"}`)
	response := httptest.NewRecorder()
	handler.WeChatShopCompatibility(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWeChatShopDisabledCallbackFailsUnavailable(t *testing.T) {
	handler, _ := NewCommerceRefundHandler(&fakePayCompatibility{}, &fakeShopRefundApplication{}, fakeShopCallbackVerifier{})
	request := httptest.NewRequest(http.MethodPost, WeChatShopCallbackPath, strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Wechatshop-Timestamp", "1756100000")
	request.Header.Set("Wechatshop-Nonce", "nonce")
	request.Header.Set("Wechatshop-Serial", "serial")
	request.Header.Set("Wechatshop-Signature", "signature")
	response := httptest.NewRecorder()
	handler.WeChatShopCallback(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func authenticatedRefundRequest(t *testing.T, path, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "refund-"+strings.Repeat("x", 16))
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 71, Role: authport.RoleAdmin}, authport.SessionRef("session"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityOrderWrite, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}

type fakePayCompatibility struct {
	command orderport.WeChatPayRefundCompatibilityCommand
}

func (fake *fakePayCompatibility) RequestWeChatPayRefundV2(_ context.Context, command orderport.WeChatPayRefundCompatibilityCommand) (orderport.RefundV2, error) {
	fake.command = command
	return orderport.RefundV2{ID: 3, OrderID: 11, OutRefundNo: "pe01r_1234567890abcdef1234567890abcdef", AmountMinor: command.AmountMinor, Currency: "CNY", State: orderport.EffectAccepted, Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

type fakeShopRefundApplication struct {
	command orderport.WeChatShopRefundCommand
}

func (fake *fakeShopRefundApplication) RequestRefund(_ context.Context, command orderport.WeChatShopRefundCommand) (orderport.WeChatShopRefund, error) {
	fake.command = command
	now := time.Now()
	return orderport.WeChatShopRefund{ID: 4, OrderID: 12, MerchantOrderNo: command.OrderReference, OutRefundNo: "wsr_1234567890abcdef1234567890abcdef", AmountMinor: command.AmountMinor, Currency: "CNY", State: orderport.WeChatShopRefundAccepted, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}
func (*fakeShopRefundApplication) ExecuteRefund(context.Context, orderport.WeChatShopExecutionJob) (orderport.WeChatShopRefund, error) {
	return orderport.WeChatShopRefund{}, nil
}
func (*fakeShopRefundApplication) ApplyRefundCallback(context.Context, orderport.WeChatShopRefundCallbackCommand) (orderport.WeChatShopRefund, error) {
	return orderport.WeChatShopRefund{}, nil
}
func (*fakeShopRefundApplication) ReconcileRefund(context.Context, int64) (orderport.WeChatShopRefund, error) {
	return orderport.WeChatShopRefund{}, nil
}

type fakeShopCallbackVerifier struct{}

func (fakeShopCallbackVerifier) VerifyRefund(context.Context, []byte, map[string]string) (orderport.WeChatShopRefundCallbackCommand, error) {
	return orderport.WeChatShopRefundCallbackCommand{}, orderport.ErrWeChatShopRefundDisabled
}
