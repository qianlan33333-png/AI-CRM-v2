package provider

import (
	"context"
	"crypto/sha256"
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

func TestWeChatShopOrderStableTokenRefreshAndTypedMaterial(t *testing.T) {
	const orderID = "370511505847120892812345678901"
	credential, err := NewWeChatShopCredential("wx-shop-app", "shop-secret")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		switch requests {
		case 1:
			assertShopTokenRequest(t, request, body, false)
			return shopJSONResponse(http.StatusOK, `{"access_token":"expired-token","expires_in":7200}`), nil
		case 2:
			assertShopOrderRequest(t, request, body, orderID, "expired-token")
			return shopJSONResponse(http.StatusOK, `{"errcode":42001,"errmsg":"expired access token"}`), nil
		case 3:
			assertShopTokenRequest(t, request, body, true)
			return shopJSONResponse(http.StatusOK, `{"access_token":"fresh-token","expires_in":7200}`), nil
		case 4, 5:
			assertShopOrderRequest(t, request, body, orderID, "fresh-token")
			return shopJSONResponse(http.StatusOK, shopOrderResponse(orderID)), nil
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil, nil
		}
	})
	provider, err := NewWeChatShopOrder(WeChatShopOrderConfig{APIBaseURL: "https://api.weixin.qq.com", Credential: credential}, client)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	provider.now = func() time.Time { return now }

	material, err := provider.GetOrder(context.Background(), orderID)
	if err != nil {
		t.Fatal(err)
	}
	if material.ProviderOrderID != orderID || material.StatusCode != 20 || !material.DealRecorded || material.AmountMinor != 12900 || material.Currency != "CNY" || material.Readiness != orderport.WeChatShopMaterialReady || !material.ProviderVerified || material.Source != orderport.WeChatShopMaterialProvider {
		t.Fatalf("material=%+v", material)
	}
	if material.TransactionDigest == ([32]byte{}) || material.EvidenceDigest == ([32]byte{}) || !material.SyncedAt.Equal(now) {
		t.Fatalf("material digests/time=%+v", material)
	}
	if len(material.Lines) != 2 || material.Lines[0].ProductID != "product-1" || material.Lines[0].SKUID != "sku-1" || material.Lines[0].SKUCount != 2 || material.Lines[0].RemainingSKUCount != 1 || material.Lines[0].Readiness != orderport.WeChatShopLineReady || material.Lines[1].Readiness != orderport.WeChatShopLineNoRemainingCount {
		t.Fatalf("lines=%+v", material.Lines)
	}
	firstDigest := material.EvidenceDigest
	second, err := provider.GetOrder(context.Background(), orderID)
	if err != nil || second.EvidenceDigest != firstDigest || requests != 5 {
		t.Fatalf("cached GetOrder=%+v err=%v requests=%d", second, err, requests)
	}
	formatted := fmt.Sprintf("%+v %+v %#v %#v", credential, provider, credential, provider)
	if strings.Contains(formatted, "shop-secret") || strings.Contains(formatted, "fresh-token") {
		t.Fatalf("credential/token leaked through formatting: %s", formatted)
	}
}

func TestNormalizeWeChatShopOrderMaterialDoesNotGuessLineEvidence(t *testing.T) {
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	missingAfterSale := []byte(`{"errcode":0,"order":{"order_id":"order-1","status":20,"order_detail":{"pay_info":{"pay_time":1700000000},"price_info":{"order_price":100},"product_infos":[{"product_id":234245,"sku_id":23424,"sku_cnt":1,"real_price":100}]}}}`)
	material, err := NormalizeWeChatShopOrderMaterial(missingAfterSale, "order-1", orderport.WeChatShopMaterialProvider, now)
	if err != nil || material.Readiness != orderport.WeChatShopMaterialAfterSaleEvidenceMiss || len(material.Lines) != 1 || material.Lines[0].ProductID != "234245" || material.Lines[0].SKUID != "23424" || material.Lines[0].AfterSaleEvidenceExact || material.Lines[0].RemainingSKUCount != 0 {
		t.Fatalf("missing aftersale material=%+v err=%v", material, err)
	}
	legacy, err := NormalizeWeChatShopOrderMaterial([]byte(`{"order_id":"order-1","status":20,"order_detail":{"pay_info":{"pay_time":1700000000},"price_info":{"order_price":100},"product_infos":[{"product_id":"product-1","sku_id":"sku-1","sku_cnt":1,"on_aftersale_sku_cnt":0,"finish_aftersale_sku_cnt":0,"real_price":100}]}}`), "order-1", orderport.WeChatShopMaterialLegacyRaw, now)
	if err != nil || legacy.ProviderVerified || legacy.Readiness != orderport.WeChatShopMaterialProviderSyncRequired {
		t.Fatalf("legacy material=%+v err=%v", legacy, err)
	}

	invalidCases := map[string]string{
		"missing product":    `{"errcode":0,"order":{"order_id":"order-1","status":20,"order_detail":{"price_info":{"order_price":100},"product_infos":[{"sku_id":"sku-1","sku_cnt":1,"real_price":100}]}}}`,
		"missing count":      `{"errcode":0,"order":{"order_id":"order-1","status":20,"order_detail":{"price_info":{"order_price":100},"product_infos":[{"product_id":"product-1","sku_id":"sku-1","real_price":100}]}}}`,
		"duplicate line":     `{"errcode":0,"order":{"order_id":"order-1","status":20,"order_detail":{"price_info":{"order_price":100},"product_infos":[{"product_id":"product-1","sku_id":"sku-1","sku_cnt":1,"real_price":100},{"product_id":"product-1","sku_id":"sku-1","sku_cnt":1,"real_price":100}]}}}`,
		"cross order":        `{"errcode":0,"order":{"order_id":"order-2","status":20,"order_detail":{"price_info":{"order_price":100},"product_infos":[{"product_id":"product-1","sku_id":"sku-1","sku_cnt":1,"real_price":100}]}}}`,
		"negative aftersale": `{"errcode":0,"order":{"order_id":"order-1","status":20,"order_detail":{"price_info":{"order_price":100},"product_infos":[{"product_id":"product-1","sku_id":"sku-1","sku_cnt":1,"on_aftersale_sku_cnt":-1,"finish_aftersale_sku_cnt":0,"real_price":100}]}}}`,
	}
	for name, raw := range invalidCases {
		t.Run(name, func(t *testing.T) {
			if _, normalizeErr := NormalizeWeChatShopOrderMaterial([]byte(raw), "order-1", orderport.WeChatShopMaterialProvider, now); !errors.Is(normalizeErr, ErrInvalidProviderResponse) {
				t.Fatalf("err=%v", normalizeErr)
			}
		})
	}
	if _, normalizeErr := NormalizeWeChatShopOrderMaterial([]byte(`{"order":{"order_id":"order-1","status":20,"order_detail":{"price_info":{"order_price":100},"product_infos":[{"product_id":"product-1","sku_id":"sku-1","sku_cnt":1,"real_price":100}]}},"order_id":"order-1"}`), "order-1", orderport.WeChatShopMaterialLegacyRaw, now); !errors.Is(normalizeErr, ErrInvalidProviderResponse) {
		t.Fatalf("ambiguous legacy shape err=%v", normalizeErr)
	}
}

func TestWeChatShopOrderRejectsUnsafeProviderResultsWithoutLeakingBody(t *testing.T) {
	credential, err := NewWeChatShopCredential("wx-app", "shop-secret")
	if err != nil {
		t.Fatal(err)
	}
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == weChatShopStableTokenPath {
			return shopJSONResponse(http.StatusOK, `{"access_token":"access-token","expires_in":7200}`), nil
		}
		return shopJSONResponse(http.StatusOK, `{"errcode":0,"order":{"order_id":"wrong-order","buyer_mobile":"13900000000"}}`), nil
	})
	provider, err := NewWeChatShopOrder(WeChatShopOrderConfig{APIBaseURL: "https://api.weixin.qq.com", Credential: credential}, client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.GetOrder(context.Background(), "order-1")
	if !errors.Is(err, ErrInvalidProviderResponse) || strings.Contains(err.Error(), "13900000000") || strings.Contains(err.Error(), "shop-secret") || strings.Contains(err.Error(), "access-token") {
		t.Fatalf("unsafe error=%v", err)
	}
	first := sha256.Sum256([]byte(shopOrderResponse("order-1")))
	second := sha256.Sum256([]byte(shopOrderResponse("order-2")))
	if first == second {
		t.Fatal("raw evidence test fixture did not change")
	}
}

func assertShopTokenRequest(t *testing.T, request *http.Request, body []byte, force bool) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.Path != weChatShopStableTokenPath || request.URL.RawQuery != "" {
		t.Fatalf("token request=%s %s", request.Method, request.URL.String())
	}
	var payload struct {
		GrantType    string `json:"grant_type"`
		AppID        string `json:"appid"`
		Secret       string `json:"secret"`
		ForceRefresh bool   `json:"force_refresh"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.GrantType != "client_credential" || payload.AppID != "wx-shop-app" || payload.Secret != "shop-secret" || payload.ForceRefresh != force {
		t.Fatalf("token payload=%s", body)
	}
}

func assertShopOrderRequest(t *testing.T, request *http.Request, body []byte, orderID, token string) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.Path != weChatShopOrderGetPath || request.URL.Query().Get("access_token") != token {
		t.Fatalf("order request=%s %s", request.Method, request.URL.String())
	}
	var payload struct {
		OrderID string `json:"order_id"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.OrderID != orderID {
		t.Fatalf("order payload=%s", body)
	}
}

func shopJSONResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func shopOrderResponse(orderID string) string {
	return fmt.Sprintf(`{"errcode":0,"order":{"order_id":%q,"status":20,"create_time":1700000000,"update_time":1700000100,"buyer_mobile":"13900000000","order_detail":{"pay_info":{"pay_time":1700000050,"transaction_id":"transaction-1","openid":"sensitive-openid"},"price_info":{"order_price":12900},"delivery_info":{"address_info":{"tel_number":"13900000000"}},"product_infos":[{"product_id":"product-1","sku_id":"sku-1","sku_cnt":2,"on_aftersale_sku_cnt":1,"finish_aftersale_sku_cnt":0,"real_price":6450},{"product_id":"product-2","sku_id":"sku-2","sku_cnt":1,"on_aftersale_sku_cnt":0,"finish_aftersale_sku_cnt":1,"real_price":6450}]}}}`, orderID)
}
