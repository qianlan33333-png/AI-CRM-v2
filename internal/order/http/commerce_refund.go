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
	RefundAmountTotal         *int64 `json:"refund_amount_total"`
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
			AmountMinor: *input.RefundAmountTotal, Reason: strings.TrimSpace(input.Reason), Checked: *input.Checked,
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
	body, headers, err := wechatShopCallbackInput(writer, request)
	if err == nil {
		command, verifyErr := handler.callbacks.VerifyRefund(request.Context(), body, headers)
		if verifyErr == nil {
			_, applyErr := handler.shop.ApplyRefundCallback(request.Context(), command)
			if applyErr == nil {
				writeJSON(writer, http.StatusOK, map[string]string{"code": "SUCCESS"})
				return
			}
			err = applyErr
		} else {
			err = verifyErr
			if !errors.Is(verifyErr, orderport.ErrWeChatShopRefundDisabled) {
				err = errors.Join(orderport.ErrCommerceRefundInvalid, verifyErr)
			}
		}
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
		result, callErr := handler.shop.ReconcileRefund(request.Context(), refundID)
		if callErr == nil {
			writeJSON(writer, http.StatusOK, mapWeChatShopRefund(result))
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

func wechatShopCallbackInput(writer http.ResponseWriter, request *http.Request) ([]byte, map[string]string, error) {
	body, err := rawCallbackBody(writer, request)
	if err != nil {
		return nil, nil, err
	}
	headers := make(map[string]string, 4)
	for _, name := range []string{"Wechatshop-Timestamp", "Wechatshop-Nonce", "Wechatshop-Serial", "Wechatshop-Signature"} {
		if len(request.Header.Values(name)) != 1 || request.Header.Get(name) == "" {
			return nil, nil, orderport.ErrCommerceRefundInvalid
		}
		headers[name] = request.Header.Get(name)
	}
	return body, headers, nil
}

func mapRefundV2(refund orderport.RefundV2) map[string]any {
	return map[string]any{"id": refund.ID, "order_id": refund.OrderID, "out_refund_no": refund.OutRefundNo, "amount_minor": refund.AmountMinor, "currency": refund.Currency, "external_effect_id": refund.ExternalEffectID, "state": refund.State, "version": refund.Version, "created_at": refund.CreatedAt.UTC(), "updated_at": refund.UpdatedAt.UTC()}
}

func mapWeChatShopRefund(refund orderport.WeChatShopRefund) map[string]any {
	return map[string]any{"id": refund.ID, "order_id": refund.OrderID, "merchant_order_no": refund.MerchantOrderNo, "out_refund_no": refund.OutRefundNo, "amount_minor": refund.AmountMinor, "currency": refund.Currency, "state": refund.State, "provider_accepted": refund.State == orderport.WeChatShopRefundProviderAccepted, "delivery_proven": refund.State == orderport.WeChatShopRefundSucceeded, "attempt_count": refund.AttemptCount, "version": refund.Version, "created_at": refund.CreatedAt.UTC(), "updated_at": refund.UpdatedAt.UTC()}
}
