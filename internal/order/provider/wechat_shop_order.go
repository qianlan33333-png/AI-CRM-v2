package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

const (
	weChatShopStableTokenPath = "/cgi-bin/stable_token"
	weChatShopOrderGetPath    = "/channels/ec/order/get"
)

// WeChatShopCredential intentionally keeps the secret fields unexported so
// accidental structured logging cannot serialize them.
type WeChatShopCredential struct {
	appID     string
	appSecret string
}

func NewWeChatShopCredential(appID, appSecret string) (*WeChatShopCredential, error) {
	if !validShopCredentialPart(appID, 128) || !validShopCredentialPart(appSecret, 256) {
		return nil, ErrInvalidProviderConfig
	}
	return &WeChatShopCredential{appID: appID, appSecret: appSecret}, nil
}

func (*WeChatShopCredential) String() string   { return "wechat-shop-credential[redacted]" }
func (*WeChatShopCredential) GoString() string { return "wechat-shop-credential[redacted]" }

type WeChatShopOrderConfig struct {
	APIBaseURL string
	Credential *WeChatShopCredential
}

type WeChatShopOrder struct {
	config     WeChatShopOrderConfig
	baseURL    *url.URL
	httpClient HTTPDoer
	now        func() time.Time

	tokenMu        sync.Mutex
	accessToken    string
	tokenExpiresAt time.Time
}

var _ orderport.WeChatShopOrderMaterialProvider = (*WeChatShopOrder)(nil)

func (*WeChatShopOrder) String() string   { return "wechat-shop-order-provider[redacted]" }
func (*WeChatShopOrder) GoString() string { return "wechat-shop-order-provider[redacted]" }

func NewWeChatShopOrder(config WeChatShopOrderConfig, client HTTPDoer) (*WeChatShopOrder, error) {
	baseURL, valid := validHTTPSBase(config.APIBaseURL)
	if !valid || config.Credential == nil || !validShopCredentialPart(config.Credential.appID, 128) || !validShopCredentialPart(config.Credential.appSecret, 256) || client == nil {
		return nil, ErrInvalidProviderConfig
	}
	return &WeChatShopOrder{config: config, baseURL: baseURL, httpClient: client, now: time.Now}, nil
}

func (provider *WeChatShopOrder) GetOrder(ctx context.Context, orderID string) (orderport.WeChatShopOrderMaterial, error) {
	if provider == nil || ctx == nil || ctx.Err() != nil || !validShopReference(orderID) {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialInvalid
	}
	token, err := provider.stableToken(ctx, false)
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, err
	}
	body, code, err := provider.getOrder(ctx, orderID, token)
	if err == nil && isWeChatTokenInvalid(code) {
		token, err = provider.stableToken(ctx, true)
		if err != nil {
			return orderport.WeChatShopOrderMaterial{}, err
		}
		body, code, err = provider.getOrder(ctx, orderID, token)
	}
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, err
	}
	if code != 0 {
		return orderport.WeChatShopOrderMaterial{}, ErrInvalidProviderResponse
	}
	return NormalizeWeChatShopOrderMaterial(body, orderID, orderport.WeChatShopMaterialProvider, provider.now().UTC())
}

func (provider *WeChatShopOrder) stableToken(ctx context.Context, force bool) (string, error) {
	provider.tokenMu.Lock()
	defer provider.tokenMu.Unlock()
	now := provider.now().UTC()
	if !force && provider.accessToken != "" && provider.tokenExpiresAt.After(now.Add(5*time.Minute)) {
		return provider.accessToken, nil
	}
	payload := struct {
		GrantType    string `json:"grant_type"`
		AppID        string `json:"appid"`
		Secret       string `json:"secret"`
		ForceRefresh bool   `json:"force_refresh"`
	}{GrantType: "client_credential", AppID: provider.config.Credential.appID, Secret: provider.config.Credential.appSecret, ForceRefresh: force}
	body, err := provider.post(ctx, weChatShopStableTokenPath, nil, payload)
	if err != nil {
		return "", err
	}
	var response struct {
		ErrCode     exactInteger `json:"errcode"`
		AccessToken string       `json:"access_token"`
		ExpiresIn   exactInteger `json:"expires_in"`
	}
	if !decodeJSONObject(body, &response) || (response.ErrCode.set && response.ErrCode.value != 0) || !validShopCredentialPart(response.AccessToken, 2048) || !response.ExpiresIn.set || response.ExpiresIn.value < 60 || response.ExpiresIn.value > 86_400 {
		return "", ErrInvalidProviderResponse
	}
	provider.accessToken = response.AccessToken
	provider.tokenExpiresAt = now.Add(time.Duration(response.ExpiresIn.value) * time.Second)
	return provider.accessToken, nil
}

func (provider *WeChatShopOrder) getOrder(ctx context.Context, orderID, token string) ([]byte, int64, error) {
	query := url.Values{"access_token": []string{token}}
	body, err := provider.post(ctx, weChatShopOrderGetPath, query, struct {
		OrderID string `json:"order_id"`
	}{OrderID: orderID})
	if err != nil {
		return nil, 0, err
	}
	var envelope struct {
		ErrCode exactInteger `json:"errcode"`
	}
	if !decodeJSONObject(body, &envelope) {
		return nil, 0, ErrInvalidProviderResponse
	}
	if envelope.ErrCode.set {
		return body, envelope.ErrCode.value, nil
	}
	return body, 0, nil
}

func (provider *WeChatShopOrder) post(ctx context.Context, path string, query url.Values, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrInvalidProviderMaterial
	}
	target := *provider.baseURL
	target.Path = path
	target.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrInvalidProviderMaterial
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.httpClient.Do(request)
	if err != nil {
		return nil, ErrProviderUnavailable
	}
	responseBody, readErr := readProviderResponse(response)
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, ErrProviderUnavailable
	}
	return responseBody, nil
}

func NormalizeWeChatShopOrderMaterial(raw []byte, requestedOrderID string, source orderport.WeChatShopMaterialSource, syncedAt time.Time) (orderport.WeChatShopOrderMaterial, error) {
	if !validShopReference(requestedOrderID) || syncedAt.IsZero() || (source != orderport.WeChatShopMaterialProvider && source != orderport.WeChatShopMaterialLegacyRaw) {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialInvalid
	}
	order, err := decodeWeChatShopOrder(raw, source)
	if err != nil || !order.OrderID.set || order.OrderID.value != requestedOrderID || !order.Status.set || order.Status.value < 0 || !order.OrderDetail.PriceInfo.OrderPrice.set || order.OrderDetail.PriceInfo.OrderPrice.value < 0 || len(order.OrderDetail.ProductInfos) == 0 {
		return orderport.WeChatShopOrderMaterial{}, ErrInvalidProviderResponse
	}
	material := orderport.WeChatShopOrderMaterial{
		ProviderOrderID:  order.OrderID.value,
		StatusCode:       order.Status.value,
		DealRecorded:     order.OrderDetail.PayInfo.PayTime.value > 0 || paidWeChatShopStatus(order.Status.value),
		AmountMinor:      order.OrderDetail.PriceInfo.OrderPrice.value,
		Currency:         "CNY",
		EvidenceDigest:   providerDigest("wechat-shop/order-material/v1", requestedOrderID, digestHex(sha256.Sum256(raw))),
		Source:           source,
		ProviderVerified: source == orderport.WeChatShopMaterialProvider,
		CreatedAt:        unixSeconds(order.CreateTime),
		PaidAt:           unixSeconds(order.OrderDetail.PayInfo.PayTime),
		UpdatedAt:        unixSeconds(order.UpdateTime),
		SyncedAt:         syncedAt.UTC(),
	}
	if order.OrderDetail.PayInfo.TransactionID.set {
		material.TransactionDigest = providerDigest("wechat-shop/transaction/v1", order.OrderDetail.PayInfo.TransactionID.value)
	}
	seen := make(map[string]struct{}, len(order.OrderDetail.ProductInfos))
	for index, item := range order.OrderDetail.ProductInfos {
		if !item.ProductID.set || !item.SKUID.set || !item.SKUCount.set || item.SKUCount.value < 1 || !item.RealPrice.set || item.RealPrice.value < 0 {
			return orderport.WeChatShopOrderMaterial{}, ErrInvalidProviderResponse
		}
		key := item.ProductID.value + "\x00" + item.SKUID.value
		if _, exists := seen[key]; exists {
			return orderport.WeChatShopOrderMaterial{}, ErrInvalidProviderResponse
		}
		seen[key] = struct{}{}
		line := orderport.WeChatShopOrderLine{Position: index + 1, ProductID: item.ProductID.value, SKUID: item.SKUID.value, SKUCount: item.SKUCount.value, RealPriceMinor: item.RealPrice.value}
		if (item.OnAfterSaleSKUCount.set && item.OnAfterSaleSKUCount.value < 0) || (item.FinishAfterSaleSKUCount.set && item.FinishAfterSaleSKUCount.value < 0) || (item.OnAfterSaleSKUCount.set && item.FinishAfterSaleSKUCount.set && item.OnAfterSaleSKUCount.value+item.FinishAfterSaleSKUCount.value > item.SKUCount.value) {
			return orderport.WeChatShopOrderMaterial{}, ErrInvalidProviderResponse
		}
		if item.OnAfterSaleSKUCount.set && item.FinishAfterSaleSKUCount.set {
			line.AfterSaleEvidenceExact = true
			line.OnAfterSaleSKUCount = item.OnAfterSaleSKUCount.value
			line.FinishAfterSaleSKUCount = item.FinishAfterSaleSKUCount.value
			line.RemainingSKUCount = item.SKUCount.value - item.OnAfterSaleSKUCount.value - item.FinishAfterSaleSKUCount.value
			if line.RemainingSKUCount > 0 {
				line.Readiness = orderport.WeChatShopLineReady
			} else {
				line.Readiness = orderport.WeChatShopLineNoRemainingCount
			}
		} else {
			line.Readiness = orderport.WeChatShopLineAfterSaleEvidenceMiss
		}
		material.Lines = append(material.Lines, line)
	}
	material.Readiness = materialReadiness(material)
	return material, nil
}

type weChatShopOrder struct {
	OrderID     exactReference `json:"order_id"`
	Status      exactInteger   `json:"status"`
	CreateTime  exactInteger   `json:"create_time"`
	UpdateTime  exactInteger   `json:"update_time"`
	OrderDetail struct {
		PayInfo struct {
			PayTime       exactInteger   `json:"pay_time"`
			TransactionID exactReference `json:"transaction_id"`
		} `json:"pay_info"`
		PriceInfo struct {
			OrderPrice exactInteger `json:"order_price"`
		} `json:"price_info"`
		ProductInfos []struct {
			ProductID               exactReference `json:"product_id"`
			SKUID                   exactReference `json:"sku_id"`
			SKUCount                exactInteger   `json:"sku_cnt"`
			OnAfterSaleSKUCount     exactInteger   `json:"on_aftersale_sku_cnt"`
			FinishAfterSaleSKUCount exactInteger   `json:"finish_aftersale_sku_cnt"`
			RealPrice               exactInteger   `json:"real_price"`
		} `json:"product_infos"`
	} `json:"order_detail"`
}

func decodeWeChatShopOrder(raw []byte, source orderport.WeChatShopMaterialSource) (weChatShopOrder, error) {
	var envelope struct {
		ErrCode     exactInteger    `json:"errcode"`
		Order       json.RawMessage `json:"order"`
		OrderID     json.RawMessage `json:"order_id"`
		OrderDetail json.RawMessage `json:"order_detail"`
	}
	if !decodeJSONObject(raw, &envelope) || (envelope.ErrCode.set && envelope.ErrCode.value != 0) {
		return weChatShopOrder{}, ErrInvalidProviderResponse
	}
	payload := envelope.Order
	if source == orderport.WeChatShopMaterialLegacyRaw && len(payload) > 0 && !bytes.Equal(bytes.TrimSpace(payload), []byte("null")) && (len(envelope.OrderID) > 0 || len(envelope.OrderDetail) > 0) {
		return weChatShopOrder{}, ErrInvalidProviderResponse
	}
	if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		if source != orderport.WeChatShopMaterialLegacyRaw {
			return weChatShopOrder{}, ErrInvalidProviderResponse
		}
		payload = raw
	}
	var order weChatShopOrder
	if !decodeJSONObject(payload, &order) {
		return weChatShopOrder{}, ErrInvalidProviderResponse
	}
	return order, nil
}

type exactInteger struct {
	value int64
	set   bool
}

func (value *exactInteger) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(string(raw), `"`)
	if text == "" || strings.ContainsAny(text, ".eE+ ") {
		return ErrInvalidProviderResponse
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return ErrInvalidProviderResponse
	}
	value.value, value.set = parsed, true
	return nil
}

type exactReference struct {
	value string
	set   bool
}

func (value *exactReference) UnmarshalJSON(raw []byte) error {
	var text string
	if len(raw) > 0 && raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil {
			return ErrInvalidProviderResponse
		}
	} else {
		text = string(raw)
		if text == "" || len(text) > 128 {
			return ErrInvalidProviderResponse
		}
		for _, character := range text {
			if character < '0' || character > '9' {
				return ErrInvalidProviderResponse
			}
		}
	}
	if !validShopReference(text) {
		return ErrInvalidProviderResponse
	}
	value.value, value.set = text, true
	return nil
}

func decodeJSONObject(raw []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

func materialReadiness(material orderport.WeChatShopOrderMaterial) orderport.WeChatShopMaterialReadiness {
	if material.Source != orderport.WeChatShopMaterialProvider || !material.ProviderVerified {
		return orderport.WeChatShopMaterialProviderSyncRequired
	}
	if !material.DealRecorded {
		return orderport.WeChatShopMaterialOrderNotPaid
	}
	ready := false
	for _, line := range material.Lines {
		if line.Readiness == orderport.WeChatShopLineAfterSaleEvidenceMiss {
			return orderport.WeChatShopMaterialAfterSaleEvidenceMiss
		}
		ready = ready || line.Readiness == orderport.WeChatShopLineReady
	}
	if !ready {
		return orderport.WeChatShopMaterialNoRefundableLine
	}
	return orderport.WeChatShopMaterialReady
}

func paidWeChatShopStatus(status int64) bool {
	switch status {
	case 20, 21, 30, 100:
		return true
	default:
		return false
	}
}

func unixSeconds(value exactInteger) time.Time {
	if !value.set || value.value < 1 {
		return time.Time{}
	}
	return time.Unix(value.value, 0).UTC()
}

func isWeChatTokenInvalid(code int64) bool {
	return code == 40001 || code == 40014 || code == 42001
}

func validShopCredentialPart(value string, maximum int) bool {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character == 0x7f {
			return false
		}
	}
	return true
}

func validShopReference(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character == 0x7f {
			return false
		}
	}
	return true
}
