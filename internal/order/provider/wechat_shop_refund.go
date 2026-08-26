package provider

import (
	"context"
	"crypto/sha256"
	"net/url"
	"strconv"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

const (
	weChatShopAfterSaleCreatePath = "/channels/ec/aftersale/genaftersaleorder"
	weChatShopAfterSaleGetPath    = "/channels/ec/aftersale/getaftersaleorder"
)

var weChatShopRefundReasonCodes = map[string]struct{}{
	"10000000": {},
	"10000001": {},
	"10000002": {},
	"10000006": {},
	"10000007": {},
	"10000008": {},
	"10000014": {},
	"10000015": {},
	"10000017": {},
	"10000021": {},
}

var _ orderport.WeChatShopRefundProvider = (*WeChatShopOrder)(nil)

func (*WeChatShopOrder) Enabled() bool { return true }

func (provider *WeChatShopOrder) RequestRefund(ctx context.Context, request orderport.WeChatShopRefundRequest) (orderport.WeChatShopProviderResult, error) {
	if provider == nil || ctx == nil || ctx.Err() != nil || !validShopReference(request.ProviderOrderID) || !validShopReference(request.ProductID) || !validShopReference(request.SKUID) || !validShopReference(request.OutRefundNo) || request.Count < 1 || request.Count > 1_000_000 || request.AmountMinor < 1 || request.AmountMinor > 1_000_000_000 || request.Currency != "CNY" || !validShopRefundReasonCode(request.ReasonCode) || request.ReasonDigest == ([32]byte{}) {
		return orderport.WeChatShopProviderResult{}, ErrInvalidProviderMaterial
	}
	payload := struct {
		RequestID string `json:"request_id"`
		OrderID   string `json:"order_id"`
		ProductID string `json:"product_id"`
		SKUID     string `json:"sku_id"`
		Count     int64  `json:"count"`
		Amount    int64  `json:"amount"`
		Reason    string `json:"reason"`
		Type      string `json:"type"`
	}{
		RequestID: request.OutRefundNo, OrderID: request.ProviderOrderID,
		ProductID: request.ProductID, SKUID: request.SKUID, Count: request.Count,
		Amount: request.AmountMinor, Reason: request.ReasonCode, Type: "REFUND",
	}
	body, err := provider.postWeChatShopWithStableToken(ctx, weChatShopAfterSaleCreatePath, payload)
	if err != nil {
		return orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderOutcomeUnknown}, err
	}
	var response struct {
		ErrCode          exactInteger   `json:"errcode"`
		AfterSaleID      exactReference `json:"aftersale_id"`
		AfterSaleOrderID exactReference `json:"after_sale_order_id"`
	}
	if !decodeJSONObject(body, &response) || !response.ErrCode.set {
		return orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderOutcomeUnknown}, ErrInvalidProviderResponse
	}
	evidence := providerDigest("wechat-shop/aftersale-create-response/v1", request.OutRefundNo, strconv.FormatInt(response.ErrCode.value, 10), digestHex(sha256.Sum256(body)))
	if response.ErrCode.value != 0 {
		completion := orderport.WeChatShopProviderOutcomeUnknown
		if finalWeChatShopRefundRejection(response.ErrCode.value) {
			completion = orderport.WeChatShopProviderFinalFailed
		}
		return orderport.WeChatShopProviderResult{Completion: completion, EvidenceDigest: evidence}, nil
	}
	afterSaleID, valid := canonicalAfterSaleID(response.AfterSaleID, response.AfterSaleOrderID)
	if !valid {
		return orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderOutcomeUnknown}, ErrInvalidProviderResponse
	}
	return orderport.WeChatShopProviderResult{
		Completion: orderport.WeChatShopProviderAccepted, EvidenceDigest: evidence,
		AfterSaleID: afterSaleID,
	}, nil
}

func (provider *WeChatShopOrder) QueryRefund(ctx context.Context, afterSaleID string) (orderport.WeChatShopRefundQueryResult, error) {
	if provider == nil || ctx == nil || ctx.Err() != nil || !validShopReference(afterSaleID) {
		return orderport.WeChatShopRefundQueryResult{}, ErrInvalidProviderMaterial
	}
	body, err := provider.postWeChatShopWithStableToken(ctx, weChatShopAfterSaleGetPath, struct {
		AfterSaleOrderID string `json:"after_sale_order_id"`
	}{AfterSaleOrderID: afterSaleID})
	if err != nil {
		return orderport.WeChatShopRefundQueryResult{}, err
	}
	var response struct {
		ErrCode        exactInteger `json:"errcode"`
		AfterSaleOrder struct {
			AfterSaleOrderID exactReference `json:"after_sale_order_id"`
			Status           string         `json:"status"`
			OrderID          exactReference `json:"order_id"`
			Type             string         `json:"type"`
			UpdateTime       exactInteger   `json:"update_time"`
			ProductInfo      struct {
				ProductID exactReference `json:"product_id"`
				SKUID     exactReference `json:"sku_id"`
				Count     exactInteger   `json:"count"`
			} `json:"product_info"`
			RefundInfo struct {
				Amount exactInteger `json:"amount"`
			} `json:"refund_info"`
		} `json:"after_sale_order"`
	}
	if !decodeJSONObject(body, &response) || !response.ErrCode.set || response.ErrCode.value != 0 {
		return orderport.WeChatShopRefundQueryResult{}, ErrInvalidProviderResponse
	}
	order := response.AfterSaleOrder
	if !order.AfterSaleOrderID.set || order.AfterSaleOrderID.value != afterSaleID || !order.OrderID.set || !order.ProductInfo.ProductID.set || !order.ProductInfo.SKUID.set || !order.ProductInfo.Count.set || order.ProductInfo.Count.value < 1 || order.ProductInfo.Count.value > 1_000_000 || !order.RefundInfo.Amount.set || order.RefundInfo.Amount.value < 1 || order.RefundInfo.Amount.value > 1_000_000_000 || !validShopStatus(order.Status) || order.Type != "REFUND" || !order.UpdateTime.set || order.UpdateTime.value < 1 {
		return orderport.WeChatShopRefundQueryResult{}, ErrInvalidProviderResponse
	}
	evidence := providerDigest("wechat-shop/aftersale-query-response/v1", afterSaleID, digestHex(sha256.Sum256(body)))
	return orderport.WeChatShopRefundQueryResult{
		EvidenceDigest:       evidence,
		ProviderRefundDigest: providerDigest("wechat-shop/aftersale-id/v1", afterSaleID),
		AfterSaleID:          afterSaleID, ProviderOrderID: order.OrderID.value,
		ProductID: order.ProductInfo.ProductID.value, SKUID: order.ProductInfo.SKUID.value,
		Count: order.ProductInfo.Count.value, AmountMinor: order.RefundInfo.Amount.value,
		Currency: "CNY", Type: order.Type, Status: order.Status,
		OccurredAt: time.Unix(order.UpdateTime.value, 0).UTC(),
	}, nil
}

func (provider *WeChatShopOrder) postWeChatShopWithStableToken(ctx context.Context, path string, payload any) ([]byte, error) {
	token, err := provider.stableToken(ctx, false)
	if err != nil {
		return nil, err
	}
	body, code, err := provider.postWeChatShop(ctx, path, token, payload)
	if err == nil && isWeChatTokenInvalid(code) {
		token, err = provider.stableToken(ctx, true)
		if err != nil {
			return nil, err
		}
		body, _, err = provider.postWeChatShop(ctx, path, token, payload)
	}
	return body, err
}

func (provider *WeChatShopOrder) postWeChatShop(ctx context.Context, path, token string, payload any) ([]byte, int64, error) {
	body, err := provider.post(ctx, path, url.Values{"access_token": []string{token}}, payload)
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

func canonicalAfterSaleID(left, right exactReference) (string, bool) {
	if left.set && right.set && left.value != right.value {
		return "", false
	}
	if left.set {
		return left.value, true
	}
	if right.set {
		return right.value, true
	}
	return "", false
}

func validShopRefundReasonCode(value string) bool {
	_, valid := weChatShopRefundReasonCodes[value]
	return valid
}

func finalWeChatShopRefundRejection(code int64) bool {
	switch code {
	case 10021083, 10021084, 10021086, 10021088:
		return true
	default:
		return false
	}
}

func validShopStatus(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character != '_' && (character < 'A' || character > 'Z') {
			return false
		}
	}
	return true
}
