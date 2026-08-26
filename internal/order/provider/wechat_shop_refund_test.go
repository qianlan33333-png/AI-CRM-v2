package provider

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestWeChatShopRefundCreateAndQueryUseOfficialContracts(t *testing.T) {
	credential, err := NewWeChatShopCredential("wx-shop-app", "shop-secret")
	if err != nil {
		t.Fatal(err)
	}
	requestCount := 0
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		switch requestCount {
		case 1:
			return shopJSONResponse(http.StatusOK, `{"access_token":"token","expires_in":7200}`), nil
		case 2:
			if request.URL.Path != weChatShopAfterSaleCreatePath || request.URL.Query().Get("access_token") != "token" {
				t.Fatalf("create target=%s", request.URL.String())
			}
			var payload map[string]any
			if json.Unmarshal(body, &payload) != nil || payload["request_id"] != "wsr_1234567890abcdef1234567890abcdef" || payload["order_id"] != "order-1" || payload["product_id"] != "product-1" || payload["sku_id"] != "sku-1" || payload["count"] != float64(2) || payload["amount"] != float64(880) || payload["reason"] != "10000014" || payload["type"] != "REFUND" {
				t.Fatalf("create payload=%s", body)
			}
			return shopJSONResponse(http.StatusOK, `{"errcode":0,"aftersale_id":1234567,"after_sale_order_id":"1234567"}`), nil
		case 3:
			if request.URL.Path != weChatShopAfterSaleGetPath || request.URL.Query().Get("access_token") != "token" || string(body) != `{"after_sale_order_id":"1234567"}` {
				t.Fatalf("query request=%s body=%s", request.URL.String(), body)
			}
			return shopJSONResponse(http.StatusOK, `{"errcode":0,"after_sale_order":{"after_sale_order_id":"1234567","status":"MERCHANT_REFUND_SUCCESS","order_id":"order-1","type":"REFUND","update_time":1700000100,"product_info":{"product_id":"product-1","sku_id":"sku-1","count":2},"refund_info":{"amount":880,"refund_reason":1},"openid":"sensitive-openid"}}`), nil
		default:
			t.Fatalf("unexpected request=%d", requestCount)
			return nil, nil
		}
	})
	provider, err := NewWeChatShopOrder(WeChatShopOrderConfig{APIBaseURL: WeChatShopProductionBaseURL, Credential: credential}, client)
	if err != nil {
		t.Fatal(err)
	}
	reasonDigest := sha1.Sum([]byte("audit reason"))
	var reason [32]byte
	copy(reason[:], reasonDigest[:])
	result, err := provider.RequestRefund(context.Background(), orderport.WeChatShopRefundRequest{
		ProviderOrderID: "order-1", ProductID: "product-1", SKUID: "sku-1", Count: 2,
		OutRefundNo: "wsr_1234567890abcdef1234567890abcdef", AmountMinor: 880,
		Currency: "CNY", ReasonCode: "10000014", ReasonDigest: reason,
	})
	if err != nil || result.Completion != orderport.WeChatShopProviderAccepted || result.AfterSaleID != "1234567" || result.EvidenceDigest == ([32]byte{}) {
		t.Fatalf("create result=%+v err=%v", result, err)
	}
	query, err := provider.QueryRefund(context.Background(), result.AfterSaleID)
	if err != nil || query.AfterSaleID != result.AfterSaleID || query.ProviderOrderID != "order-1" || query.ProductID != "product-1" || query.SKUID != "sku-1" || query.Count != 2 || query.AmountMinor != 880 || query.Currency != "CNY" || query.Type != "REFUND" || query.Status != "MERCHANT_REFUND_SUCCESS" || query.EvidenceDigest == ([32]byte{}) || query.ProviderRefundDigest == ([32]byte{}) || !query.OccurredAt.Equal(time.Unix(1700000100, 0).UTC()) {
		t.Fatalf("query=%+v err=%v", query, err)
	}
	if formatted := fmt.Sprintf("%+v %+v", credential, provider); strings.Contains(formatted, "shop-secret") || strings.Contains(formatted, "token") {
		t.Fatalf("secret leaked: %s", formatted)
	}
}

func TestWeChatShopRefundRejectsConflictingOfficialAfterSaleFields(t *testing.T) {
	credential, _ := NewWeChatShopCredential("wx-shop-app", "shop-secret")
	client := &sequenceShopClient{responses: []string{
		`{"access_token":"token","expires_in":7200}`,
		`{"errcode":0,"aftersale_id":"123","after_sale_order_id":"124"}`,
	}}
	provider, _ := NewWeChatShopOrder(WeChatShopOrderConfig{APIBaseURL: WeChatShopProductionBaseURL, Credential: credential}, client)
	result, err := provider.RequestRefund(context.Background(), validShopRefundProviderRequest())
	if err == nil || result.Completion != orderport.WeChatShopProviderOutcomeUnknown || result.AfterSaleID != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestWeChatShopRefundRejectsValuesOutsidePersistedBounds(t *testing.T) {
	credential, _ := NewWeChatShopCredential("wx-shop-app", "shop-secret")
	client := &sequenceShopClient{}
	provider, _ := NewWeChatShopOrder(WeChatShopOrderConfig{APIBaseURL: WeChatShopProductionBaseURL, Credential: credential}, client)
	for _, mutate := range []func(*orderport.WeChatShopRefundRequest){
		func(request *orderport.WeChatShopRefundRequest) { request.Count = 1_000_001 },
		func(request *orderport.WeChatShopRefundRequest) { request.AmountMinor = 1_000_000_001 },
	} {
		request := validShopRefundProviderRequest()
		mutate(&request)
		if _, err := provider.RequestRefund(context.Background(), request); err == nil {
			t.Fatalf("accepted out-of-bounds request: %+v", request)
		}
	}
	if client.index != 0 {
		t.Fatalf("provider called %d times", client.index)
	}
}

func TestWeChatShopRefundClassifiesOnlyExplicitBusinessRejectionsAsFinal(t *testing.T) {
	tests := []struct {
		name       string
		code       int64
		completion orderport.WeChatShopProviderCompletion
	}{
		{name: "request id invalid", code: 10021083, completion: orderport.WeChatShopProviderFinalFailed},
		{name: "reason invalid", code: 10021084, completion: orderport.WeChatShopProviderFinalFailed},
		{name: "virtual unsupported", code: 10021086, completion: orderport.WeChatShopProviderFinalFailed},
		{name: "order unsupported", code: 10021088, completion: orderport.WeChatShopProviderFinalFailed},
		{name: "get order failed", code: 10021085, completion: orderport.WeChatShopProviderOutcomeUnknown},
		{name: "request id already created", code: 10021089, completion: orderport.WeChatShopProviderOutcomeUnknown},
		{name: "unclassified provider code", code: 90000001, completion: orderport.WeChatShopProviderOutcomeUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credential, _ := NewWeChatShopCredential("wx-shop-app", "shop-secret")
			client := &sequenceShopClient{responses: []string{
				`{"access_token":"token","expires_in":7200}`,
				fmt.Sprintf(`{"errcode":%d}`, test.code),
			}}
			provider, _ := NewWeChatShopOrder(WeChatShopOrderConfig{APIBaseURL: WeChatShopProductionBaseURL, Credential: credential}, client)
			result, err := provider.RequestRefund(context.Background(), validShopRefundProviderRequest())
			if err != nil || result.Completion != test.completion || result.EvidenceDigest == ([32]byte{}) || result.AfterSaleID != "" || client.index != 2 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, client.index)
			}
		})
	}
}

func TestWeChatShopRefundRemainsUnknownWhenRefreshedTokenIsStillInvalid(t *testing.T) {
	credential, _ := NewWeChatShopCredential("wx-shop-app", "shop-secret")
	client := &sequenceShopClient{responses: []string{
		`{"access_token":"token-1","expires_in":7200}`,
		`{"errcode":40014}`,
		`{"access_token":"token-2","expires_in":7200}`,
		`{"errcode":40014}`,
	}}
	provider, _ := NewWeChatShopOrder(WeChatShopOrderConfig{APIBaseURL: WeChatShopProductionBaseURL, Credential: credential}, client)
	result, err := provider.RequestRefund(context.Background(), validShopRefundProviderRequest())
	if err != nil || result.Completion != orderport.WeChatShopProviderOutcomeUnknown || result.EvidenceDigest == ([32]byte{}) || result.AfterSaleID != "" || client.index != 4 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, client.index)
	}
}

func TestWeChatShopCallbackSeparatesLegacyURLSignatureFromEncryptedPOST(t *testing.T) {
	const (
		appID  = "wx-shop-app"
		token  = "callback-token"
		aesKey = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	)
	credential, err := NewWeChatShopCallbackCredential(appID, token, aesKey)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewWeChatShopCallbackVerifier(credential)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0).UTC()
	verifier.now = func() time.Time { return now }
	timestamp, nonce := "1700000000", "nonce"
	getSignature := shopSignature(token, timestamp, nonce)
	echo, err := verifier.VerifyURL(context.Background(), map[string]string{"signature": getSignature, "timestamp": timestamp, "nonce": nonce, "echostr": "plain-echo"})
	if err != nil || echo != "plain-echo" {
		t.Fatalf("GET echo=%q err=%v", echo, err)
	}
	if _, err = verifier.VerifyURL(context.Background(), map[string]string{"signature": shopSignature(token, timestamp, nonce, "plain-echo"), "timestamp": timestamp, "nonce": nonce, "echostr": "plain-echo"}); err == nil {
		t.Fatal("GET accepted encrypted-message signature")
	}
	plain := []byte(`{"ToUserName":"gh_shop","FromUserName":"sensitive-openid","CreateTime":1700000000,"MsgType":"event","Event":"channels_ec_aftersale_update","finder_shop_aftersale_status_update":{"status":"MERCHANT_REFUND_SUCCESS","after_sale_order_id":"1234567","order_id":"order-1","wxa_vip_discounted_price":100}}`)
	encrypted := encryptShopCallbackFixture(t, plain, credential.key, appID)
	body := []byte(`{"ToUserName":"gh_shop","Encrypt":` + strconvQuote(encrypted) + `}`)
	command, err := verifier.VerifyRefund(context.Background(), body, map[string]string{"msg_signature": shopSignature(token, timestamp, nonce, encrypted), "timestamp": timestamp, "nonce": nonce})
	if err != nil || command.AfterSaleID != "1234567" || command.ProviderOrderID != "order-1" || command.ProviderStatus != "MERCHANT_REFUND_SUCCESS" || command.ProviderEventDigest == ([32]byte{}) || command.PayloadDigest == ([32]byte{}) || !command.OccurredAt.Equal(now) {
		t.Fatalf("command=%+v err=%v", command, err)
	}
	if _, err = verifier.VerifyRefund(context.Background(), body, map[string]string{"msg_signature": getSignature, "timestamp": timestamp, "nonce": nonce}); err == nil {
		t.Fatal("POST accepted legacy three-part signature")
	}
	other := encryptShopCallbackFixture(t, plain, credential.key, "other-app")
	otherBody := []byte(`{"ToUserName":"gh_shop","Encrypt":` + strconvQuote(other) + `}`)
	if _, err = verifier.VerifyRefund(context.Background(), otherBody, map[string]string{"msg_signature": shopSignature(token, timestamp, nonce, other), "timestamp": timestamp, "nonce": nonce}); err == nil {
		t.Fatal("POST accepted wrong decrypted AppID")
	}
	formatted := fmt.Sprintf("%+v %+v", credential, verifier)
	if strings.Contains(formatted, token) || strings.Contains(formatted, aesKey) {
		t.Fatalf("callback secret leaked: %s", formatted)
	}
}

func validShopRefundProviderRequest() orderport.WeChatShopRefundRequest {
	return orderport.WeChatShopRefundRequest{
		ProviderOrderID: "order-1", ProductID: "product-1", SKUID: "sku-1", Count: 1,
		OutRefundNo: "wsr_1234567890abcdef1234567890abcdef", AmountMinor: 100,
		Currency: "CNY", ReasonCode: "10000008", ReasonDigest: [32]byte{1},
	}
}

type sequenceShopClient struct {
	responses []string
	index     int
}

func (client *sequenceShopClient) Do(*http.Request) (*http.Response, error) {
	if client.index >= len(client.responses) {
		return nil, fmt.Errorf("unexpected request")
	}
	response := shopJSONResponse(http.StatusOK, client.responses[client.index])
	client.index++
	return response, nil
}

func shopSignature(parts ...string) string {
	values := append([]string(nil), parts...)
	sort.Strings(values)
	digest := sha1.Sum([]byte(strings.Join(values, "")))
	return hex.EncodeToString(digest[:])
}

func encryptShopCallbackFixture(t *testing.T, message, key []byte, appID string) string {
	t.Helper()
	payload := make([]byte, 20+len(message)+len(appID))
	copy(payload[:16], []byte("0123456789abcdef"))
	binary.BigEndian.PutUint32(payload[16:20], uint32(len(message)))
	copy(payload[20:], message)
	copy(payload[20+len(message):], appID)
	padding := 32 - len(payload)%32
	payload = append(payload, bytes.Repeat([]byte{byte(padding)}, padding)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(ciphertext, payload)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

func strconvQuote(value string) string {
	body, _ := json.Marshal(value)
	return string(body)
}
