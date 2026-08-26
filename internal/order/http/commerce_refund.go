package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

const (
	WeChatShopCallbackPath  = "/api/public/wechat-shop/callbacks/refund"
	WeChatShopReconcilePath = "/api/admin/wechat-shop/refunds/{refund_id}/reconcile"
)

type CommerceRefundHandler struct {
	wechatPay orderport.WeChatPayRefundCompatibilityApplication
	shop      orderport.WeChatShopRefundApplication
	callbacks orderport.WeChatShopRefundCallbackVerifier
}

func NewCommerceRefundHandler(wechatPay orderport.WeChatPayRefundCompatibilityApplication, shop orderport.WeChatShopRefundApplication, callbacks orderport.WeChatShopRefundCallbackVerifier) (*CommerceRefundHandler, error) {
	if wechatPay == nil || shop == nil || callbacks == nil {
		return nil, orderport.ErrCommerceRefundUnavailable
	}
	return &CommerceRefundHandler{wechatPay: wechatPay, shop: shop, callbacks: callbacks}, nil
}

type commerceRefundInput struct {
	Provider                  string `json:"provider"`
	OrderNo                   string `json:"order_no"`
	ProductID                 string `json:"product_id"`
	SKUID                     string `json:"sku_id"`
	RefundCount               *int64 `json:"refund_count"`
	RefundAmountTotal         *int64 `json:"refund_amount_total"`
	ReasonCode                string `json:"reason_code"`
	Reason                    string `json:"reason"`
	TransactionIDConfirmation string `json:"transaction_id_confirmation"`
	Checked                   *bool  `json:"checked"`
	Operator                  string `json:"operator"`
}

func (handler *CommerceRefundHandler) WeChatPayCompatibility(writer http.ResponseWriter, request *http.Request, orderReference string) {
	principal, input, key, err := commerceRefundRequest(writer, request)
	if err == nil && strings.TrimSpace(input.Provider) != "" && strings.TrimSpace(input.Provider) != "wechat" {
		err = orderport.ErrCommerceRefundInvalid
	}
	if err == nil && strings.TrimSpace(input.OrderNo) != "" && strings.TrimSpace(input.OrderNo) != orderReference {
		err = orderport.ErrCommerceRefundInvalid
	}
	if err == nil {
		result, callErr := handler.wechatPay.RequestWeChatPayRefundV2(request.Context(), orderport.WeChatPayRefundCompatibilityCommand{
			OrderReference: orderReference, TransactionIDConfirmation: strings.TrimSpace(input.TransactionIDConfirmation),
			AmountMinor: *input.RefundAmountTotal, Reason: strings.TrimSpace(input.Reason), Checked: *input.Checked,
			Actor: principal.AdminUserID, IdempotencyKey: key,
		})
		if callErr == nil {
			writeJSON(writer, http.StatusAccepted, mapRefundV2(result))
			return
		}
		err = callErr
	}
	writeError(writer, request, err)
}

func (handler *CommerceRefundHandler) WeChatShopCompatibility(writer http.ResponseWriter, request *http.Request) {
	principal, input, key, err := commerceRefundRequest(writer, request)
	if err == nil && strings.TrimSpace(input.Provider) != "wechat_shop" {
		err = orderport.ErrCommerceRefundInvalid
	}
	if err == nil {
		result, callErr := handler.shop.RequestRefund(request.Context(), orderport.WeChatShopRefundCommand{
			OrderReference: strings.TrimSpace(input.OrderNo), TransactionIDConfirmation: strings.TrimSpace(input.TransactionIDConfirmation),
			ProductID: strings.TrimSpace(input.ProductID), SKUID: strings.TrimSpace(input.SKUID), Count: valueOrZero(input.RefundCount),
			AmountMinor: *input.RefundAmountTotal, ReasonCode: strings.TrimSpace(input.ReasonCode), Reason: strings.TrimSpace(input.Reason), Checked: *input.Checked,
			Actor: principal.AdminUserID, IdempotencyKey: key,
		})
		if callErr == nil {
			writeJSON(writer, http.StatusAccepted, mapWeChatShopRefund(result))
			return
		}
		err = callErr
	}
	writeError(writer, request, err)
}

func (handler *CommerceRefundHandler) WeChatShopCallback(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
		writeError(writer, request, orderport.ErrCommerceRefundInvalid)
		return
	}
	query, err := wechatShopCallbackQuery(request)
	if err == nil && request.Method == http.MethodGet {
		var echo string
		echo, err = handler.callbacks.VerifyURL(request.Context(), query)
		if err == nil {
			writeWeChatShopCallbackText(writer, echo)
			return
		}
	} else if err == nil && request.Method == http.MethodPost {
		var body []byte
		body, err = rawCallbackBody(writer, request)
		if err == nil {
			var command orderport.WeChatShopRefundCallbackCommand
			command, err = handler.callbacks.VerifyRefund(request.Context(), body, query)
			if err == nil {
				_, err = handler.shop.ApplyRefundCallback(request.Context(), command)
				if err == nil {
					writeWeChatShopCallbackText(writer, "success")
					return
				}
			}
		}
	} else if err == nil {
		err = orderport.ErrCommerceRefundInvalid
	}
	if err != nil && !errors.Is(err, orderport.ErrWeChatShopRefundDisabled) {
		err = errors.Join(orderport.ErrCommerceRefundInvalid, err)
	}
	writeError(writer, request, err)
}

func (handler *CommerceRefundHandler) ReconcileWeChatShopRefund(writer http.ResponseWriter, request *http.Request, rawID string) {
	_, err := adminActor(request, authport.CapabilityOrderWrite)
	refundID, parseErr := strconv.ParseInt(rawID, 10, 64)
	if err == nil && (parseErr != nil || refundID < 1 || request.Body == nil) {
		err = orderport.ErrCommerceRefundInvalid
	}
	var empty struct{}
	if err == nil {
		err = decode(writer, request, &empty)
	}
	if err == nil {
		result, callErr := handler.shop.QueueRefundReconciliation(request.Context(), refundID)
		if callErr == nil {
			writeJSON(writer, http.StatusAccepted, mapWeChatShopRefund(result))
			return
		}
		err = callErr
	}
	writeError(writer, request, err)
}

func commerceRefundRequest(writer http.ResponseWriter, request *http.Request) (authport.Principal, commerceRefundInput, string, error) {
	if request == nil {
		return authport.Principal{}, commerceRefundInput{}, "", orderport.ErrCommerceRefundInvalid
	}
	principal, err := adminActor(request, authport.CapabilityOrderWrite)
	var input commerceRefundInput
	if err == nil {
		err = decode(writer, request, &input)
	}
	values := request.Header.Values("Idempotency-Key")
	if err == nil && (input.RefundAmountTotal == nil || input.Checked == nil || len(values) != 1 || strings.TrimSpace(values[0]) != values[0] || len(values[0]) < 16 || len(values[0]) > 128) {
		err = orderport.ErrCommerceRefundInvalid
	}
	return principal, input, request.Header.Get("Idempotency-Key"), err
}

func wechatShopCallbackQuery(request *http.Request) (map[string]string, error) {
	if request == nil || request.URL == nil {
		return nil, orderport.ErrCommerceRefundInvalid
	}
	names := []string{"timestamp", "nonce"}
	if request.Method == http.MethodGet {
		names = append(names, "signature", "echostr")
	} else if request.Method == http.MethodPost {
		names = append(names, "msg_signature")
	}
	query := request.URL.Query()
	result := make(map[string]string, len(names))
	for _, name := range names {
		values := query[name]
		if len(values) != 1 || values[0] == "" || strings.TrimSpace(values[0]) != values[0] {
			return nil, orderport.ErrCommerceRefundInvalid
		}
		result[name] = values[0]
	}
	return result, nil
}

func writeWeChatShopCallbackText(writer http.ResponseWriter, value string) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(value))
}

func mapRefundV2(refund orderport.RefundV2) map[string]any {
	return map[string]any{"id": refund.ID, "order_id": refund.OrderID, "out_refund_no": refund.OutRefundNo, "amount_minor": refund.AmountMinor, "currency": refund.Currency, "external_effect_id": refund.ExternalEffectID, "state": refund.State, "version": refund.Version, "created_at": refund.CreatedAt.UTC(), "updated_at": refund.UpdatedAt.UTC()}
}

func mapWeChatShopRefund(refund orderport.WeChatShopRefund) map[string]any {
	var afterSaleID any
	if refund.ProviderAfterSaleID != "" {
		afterSaleID = refund.ProviderAfterSaleID
	}
	return map[string]any{"id": refund.ID, "order_id": refund.OrderID, "merchant_order_no": refund.MerchantOrderNo, "provider_order_id": refund.ProviderOrderID, "product_id": refund.ProductID, "sku_id": refund.SKUID, "refund_count": refund.RefundCount, "reason_code": refund.ReasonCode, "provider_after_sale_id": afterSaleID, "out_refund_no": refund.OutRefundNo, "amount_minor": refund.AmountMinor, "currency": refund.Currency, "state": refund.State, "provider_accepted": refund.State == orderport.WeChatShopRefundProviderAccepted, "delivery_proven": refund.State == orderport.WeChatShopRefundSucceeded, "attempt_count": refund.AttemptCount, "version": refund.Version, "created_at": refund.CreatedAt.UTC(), "updated_at": refund.UpdatedAt.UTC()}
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
