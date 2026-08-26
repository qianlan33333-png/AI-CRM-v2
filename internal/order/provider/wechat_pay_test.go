package provider

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (function httpDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

type payMaterializerStub struct {
	prepay WeChatPayPrepayMaterial
	refund WeChatPayRefundMaterial
}

func (stub payMaterializerStub) ResolvePrepay(context.Context, orderport.PrepayRequest) (WeChatPayPrepayMaterial, error) {
	return stub.prepay, nil
}

func (stub payMaterializerStub) ResolveRefund(context.Context, orderport.RefundRequest) (WeChatPayRefundMaterial, error) {
	return stub.refund, nil
}

func TestWeChatPaySignedRequestsAndQueries(t *testing.T) {
	merchantKey := testRSAKey(t)
	platformKey := testRSAKey(t)
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	identityDigest := sha256.Sum256([]byte("identity"))
	reasonDigest := sha256.Sum256([]byte("reason"))
	credential, err := NewWeChatPayCredential("merchant-1", "merchant-serial", merchantKey, nil, map[string]*rsa.PublicKey{"platform-serial": &platformKey.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	doer := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		verifyMerchantRequest(t, request, body, &merchantKey.PublicKey)
		var responseBody string
		switch request.URL.RequestURI() {
		case "/v3/pay/transactions/jsapi":
			var payload struct {
				AppID       string `json:"appid"`
				MerchantID  string `json:"mchid"`
				Description string `json:"description"`
				OutTradeNo  string `json:"out_trade_no"`
				NotifyURL   string `json:"notify_url"`
				TimeExpire  string `json:"time_expire"`
				Amount      struct {
					Total int64 `json:"total"`
				} `json:"amount"`
				Payer struct {
					OpenID string `json:"openid"`
				} `json:"payer"`
			}
			if json.Unmarshal(body, &payload) != nil || payload.AppID != "app-1" || payload.MerchantID != "merchant-1" || payload.Description != "Order description" || payload.OutTradeNo != "order-1" || payload.NotifyURL != "https://callback.example/pay" || payload.TimeExpire != now.Add(2*time.Hour).Format(time.RFC3339) || payload.Amount.Total != 9900 || payload.Payer.OpenID != "openid-1" {
				t.Fatalf("unexpected prepay payload: %s", body)
			}
			responseBody = `{"prepay_id":"wx-prepay-1"}`
		case "/v3/refund/domestic/refunds":
			var payload struct {
				OutTradeNo  string `json:"out_trade_no"`
				OutRefundNo string `json:"out_refund_no"`
				Reason      string `json:"reason"`
				NotifyURL   string `json:"notify_url"`
				Amount      struct {
					Refund, Total int64
					Currency      string
				}
			}
			if json.Unmarshal(body, &payload) != nil || payload.OutTradeNo != "order-1" || payload.OutRefundNo != "refund-1" || payload.Reason != "customer requested" || payload.NotifyURL != "https://callback.example/refund" || payload.Amount.Refund != 9900 || payload.Amount.Total != 19900 || payload.Amount.Currency != "CNY" {
				t.Fatalf("unexpected refund payload: %s", body)
			}
			responseBody = `{"refund_id":"wx-refund-1","out_refund_no":"refund-1","status":"PROCESSING"}`
		case "/v3/pay/transactions/out-trade-no/order-1?mchid=merchant-1":
			responseBody = `{"out_trade_no":"order-1","trade_state":"SUCCESS","transaction_id":"wx-transaction-1","success_time":"2026-08-26T07:59:00Z","amount":{"total":9900,"currency":"CNY"}}`
		case "/v3/pay/transactions/out-trade-no/order-2?mchid=merchant-1":
			responseBody = `{"out_trade_no":"order-1","trade_state":"SUCCESS","transaction_id":"wx-transaction-1","success_time":"2026-08-26T07:59:00Z","amount":{"total":9900,"currency":"CNY"}}`
		case "/v3/refund/domestic/refunds/refund-1":
			responseBody = `{"refund_id":"wx-refund-1","out_refund_no":"refund-1","status":"SUCCESS","success_time":"2026-08-26T07:59:30Z","amount":{"refund":9900,"currency":"CNY"}}`
		default:
			t.Fatalf("unexpected request URI %s", request.URL.RequestURI())
		}
		return signedPayResponse(t, platformKey, "platform-serial", now, http.StatusOK, responseBody), nil
	})
	provider, err := NewWeChatPay(WeChatPayConfig{Enabled: true, AppID: "app-1", APIBaseURL: "https://api.mch.weixin.qq.com", PaymentNotifyURL: "https://callback.example/pay", RefundNotifyURL: "https://callback.example/refund", Credential: credential}, payMaterializerStub{prepay: WeChatPayPrepayMaterial{Description: "Order description", PayerOpenID: "openid-1", PayerIdentityDigest: identityDigest}, refund: WeChatPayRefundMaterial{OriginalAmountMinor: 19900, Reason: "customer requested", ReasonDigest: reasonDigest}}, doer)
	if err != nil {
		t.Fatal(err)
	}
	provider.now = func() time.Time { return now }
	provider.nonce = func() (string, error) { return "fixed-nonce", nil }

	prepay, err := provider.CreatePrepay(context.Background(), orderport.PrepayRequest{MerchantOrderNo: "order-1", AmountMinor: 9900, Currency: "CNY", ProductSnapshot: "product-digest", PayerIdentityDigest: identityDigest, ProviderNotifyTarget: sha256.Sum256([]byte("notify"))})
	if err != nil || prepay.Completion != orderport.ProviderExecuted || zeroDigest(prepay.ReceiptDigest) || prepay.JSAPIHandoff == nil || !prepay.BusinessCallDispatched || !prepay.RealExternalCallExecuted {
		t.Fatalf("CreatePrepay() = %+v err=%v", prepay, err)
	}
	handoff := prepay.JSAPIHandoff
	if handoff.AppID != "app-1" || handoff.TimeStamp != fmt.Sprint(now.Unix()) || handoff.NonceStr != "fixed-nonce" || handoff.Package != "prepay_id=wx-prepay-1" || handoff.SignType != "RSA" || !handoff.ExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("JSAPI handoff = %+v", handoff)
	}
	paySignature, decodeErr := base64.StdEncoding.Strict().DecodeString(handoff.PaySign)
	payDigest := sha256.Sum256([]byte(handoff.AppID + "\n" + handoff.TimeStamp + "\n" + handoff.NonceStr + "\n" + handoff.Package + "\n"))
	if decodeErr != nil || rsa.VerifyPKCS1v15(&merchantKey.PublicKey, crypto.SHA256, payDigest[:], paySignature) != nil {
		t.Fatalf("invalid JSAPI pay signature: %v", decodeErr)
	}
	refund, err := provider.RequestRefund(context.Background(), orderport.RefundRequest{MerchantOrderNo: "order-1", OutRefundNo: "refund-1", AmountMinor: 9900, Currency: "CNY", ReasonDigest: reasonDigest})
	if err != nil || refund.Completion != orderport.ProviderExecuted || zeroDigest(refund.ReceiptDigest) || !refund.BusinessCallDispatched || !refund.RealExternalCallExecuted {
		t.Fatalf("RequestRefund() = %+v err=%v", refund, err)
	}
	paymentQuery, err := provider.QueryPayment(context.Background(), "order-1")
	if err != nil || !paymentQuery.Confirmed || paymentQuery.AmountMinor != 9900 || paymentQuery.Currency != "CNY" || zeroDigest(paymentQuery.ProviderTransactionDigest) {
		t.Fatalf("QueryPayment() = %+v err=%v", paymentQuery, err)
	}
	refundQuery, err := provider.QueryRefund(context.Background(), "refund-1")
	if err != nil || !refundQuery.Confirmed || refundQuery.AmountMinor != 9900 || refundQuery.Currency != "CNY" || zeroDigest(refundQuery.ProviderRefundDigest) {
		t.Fatalf("QueryRefund() = %+v err=%v", refundQuery, err)
	}
	if _, err = provider.QueryPayment(context.Background(), "order-2"); !errors.Is(err, ErrInvalidProviderResponse) {
		t.Fatalf("cross-order query err=%v", err)
	}
	if requests != 5 {
		t.Fatalf("requests=%d want 5", requests)
	}
}

func TestWeChatPayDistinguishesPreDispatchAndAmbiguousCalls(t *testing.T) {
	merchantKey, platformKey := testRSAKey(t), testRSAKey(t)
	credential, err := NewWeChatPayCredential("merchant", "serial", merchantKey, nil, map[string]*rsa.PublicKey{"platform": &platformKey.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("identity"))
	provider, err := NewWeChatPay(WeChatPayConfig{Enabled: true, AppID: "app", APIBaseURL: "https://api.mch.weixin.qq.com", PaymentNotifyURL: "https://callback.example/pay", RefundNotifyURL: "https://callback.example/refund", Credential: credential}, payMaterializerStub{prepay: WeChatPayPrepayMaterial{Description: "desc", PayerOpenID: "openid", PayerIdentityDigest: digest}}, httpDoerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("timeout") }))
	if err != nil {
		t.Fatal(err)
	}
	request := orderport.PrepayRequest{MerchantOrderNo: "order", AmountMinor: 1, Currency: "CNY", ProductSnapshot: "digest", PayerIdentityDigest: digest, ProviderNotifyTarget: sha256.Sum256([]byte("notify"))}
	result, err := provider.CreatePrepay(context.Background(), request)
	if err != nil || result.Completion != orderport.ProviderOutcomeUnknown || result.JSAPIHandoff != nil || !result.BusinessCallDispatched || !result.RealExternalCallExecuted {
		t.Fatalf("transport result=%+v err=%v", result, err)
	}
	request.ProviderNotifyTarget = [32]byte{}
	result, err = provider.CreatePrepay(context.Background(), request)
	if err != nil || result.Completion != orderport.ProviderFinalFailed || result.BusinessCallDispatched || result.RealExternalCallExecuted {
		t.Fatalf("invalid request result=%+v err=%v", result, err)
	}
	if _, err = NewWeChatPay(WeChatPayConfig{Enabled: true, AppID: "app", APIBaseURL: "http://api.example", PaymentNotifyURL: "https://callback.example/pay", RefundNotifyURL: "https://callback.example/refund", Credential: credential}, payMaterializerStub{}, httpDoerFunc(func(*http.Request) (*http.Response, error) { return nil, nil })); !errors.Is(err, ErrInvalidProviderConfig) {
		t.Fatalf("HTTP base err=%v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", credential), "merchant") {
		t.Fatal("credential formatting leaked merchant material")
	}
}

func TestWeChatPayRealCallbackCrypto(t *testing.T) {
	platformKey := testRSAKey(t)
	apiKey := []byte("0123456789abcdef0123456789abcdef")
	credential, err := NewWeChatPayCallbackCredential("merchant", apiKey, map[string]*rsa.PublicKey{"platform": &platformKey.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewWeChatPayCallbackVerifier(WeChatPayConfig{AppID: "app", Credential: credential})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	verifier.now = func() time.Time { return now }
	plain := []byte(`{"appid":"app","mchid":"merchant","out_refund_no":"refund-1","refund_id":"wx-refund-1","refund_status":"SUCCESS","success_time":"2026-08-26T07:59:00Z","amount":{"refund":9900,"currency":"CNY"}}`)
	block, _ := aes.NewCipher(apiKey)
	gcm, _ := cipher.NewGCM(block)
	nonce, associated := "123456789012", "refund"
	ciphertext := base64.StdEncoding.EncodeToString(gcm.Seal(nil, []byte(nonce), plain, []byte(associated)))
	body, _ := json.Marshal(map[string]any{"id": "event-1", "event_type": "REFUND.SUCCESS", "resource_type": "encrypt-resource", "resource": map[string]string{"algorithm": "AEAD_AES_256_GCM", "ciphertext": ciphertext, "nonce": nonce, "associated_data": associated}})
	timestamp := fmt.Sprint(now.Unix())
	message := timestamp + "\ncallback-nonce\n" + string(body) + "\n"
	signature := testRSASign(t, platformKey, message)
	command, err := verifier.VerifyRefund(context.Background(), body, map[string]string{"Wechatpay-Timestamp": timestamp, "Wechatpay-Nonce": "callback-nonce", "Wechatpay-Serial": "platform", "Wechatpay-Signature": signature})
	if err != nil || command.OutRefundNo != "refund-1" || !command.Succeeded || command.AmountMinor != 9900 || zeroDigest(command.ProviderRefundDigest) {
		t.Fatalf("VerifyRefund() = %+v err=%v", command, err)
	}
}

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testRSASign(t *testing.T, key *rsa.PrivateKey, message string) string {
	t.Helper()
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func signedPayResponse(t *testing.T, key *rsa.PrivateKey, serial string, now time.Time, status int, body string) *http.Response {
	t.Helper()
	timestamp, nonce := fmt.Sprint(now.Unix()), "response-nonce"
	header := http.Header{"Wechatpay-Timestamp": []string{timestamp}, "Wechatpay-Nonce": []string{nonce}, "Wechatpay-Serial": []string{serial}, "Wechatpay-Signature": []string{testRSASign(t, key, timestamp+"\n"+nonce+"\n"+body+"\n")}}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func verifyMerchantRequest(t *testing.T, request *http.Request, body []byte, key *rsa.PublicKey) {
	t.Helper()
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, weChatPayAuthorizationScheme+" ") {
		t.Fatalf("Authorization=%q", authorization)
	}
	values := map[string]string{}
	for _, field := range strings.Split(strings.TrimPrefix(authorization, weChatPayAuthorizationScheme+" "), ",") {
		parts := strings.SplitN(strings.TrimSpace(field), "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = strings.Trim(parts[1], `"`)
		}
	}
	message := request.Method + "\n" + request.URL.RequestURI() + "\n" + values["timestamp"] + "\n" + values["nonce_str"] + "\n" + string(body) + "\n"
	signature, err := base64.StdEncoding.Strict().DecodeString(values["signature"])
	digest := sha256.Sum256([]byte(message))
	if err != nil || rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		t.Fatalf("invalid merchant signature: %v", err)
	}
}
