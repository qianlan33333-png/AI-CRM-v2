package provider

import (
	"context"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

// DisabledWeChatPay is the production default until deployment supplies a
// reviewed provider adapter. It cannot produce a local/fake success receipt.
type DisabledWeChatPay struct{}

func (DisabledWeChatPay) CreatePrepay(context.Context, orderport.PrepayRequest) (orderport.ProviderResult, error) {
	return orderport.ProviderResult{}, orderport.ErrWeChatPayDisabled
}

func (DisabledWeChatPay) RequestRefund(context.Context, orderport.RefundRequest) (orderport.ProviderResult, error) {
	return orderport.ProviderResult{}, orderport.ErrWeChatPayDisabled
}

func (DisabledWeChatPay) QueryPayment(context.Context, string) (orderport.PaymentQueryResult, error) {
	return orderport.PaymentQueryResult{}, orderport.ErrWeChatPayDisabled
}

func (DisabledWeChatPay) QueryRefund(context.Context, string) (orderport.RefundQueryResult, error) {
	return orderport.RefundQueryResult{}, orderport.ErrWeChatPayDisabled
}

type DisabledCallbackVerifier struct{}

func (DisabledCallbackVerifier) VerifyPayment(context.Context, []byte, map[string]string) (orderport.PaymentCallbackCommand, error) {
	return orderport.PaymentCallbackCommand{}, orderport.ErrWeChatPayDisabled
}

func (DisabledCallbackVerifier) VerifyRefund(context.Context, []byte, map[string]string) (orderport.RefundCallbackCommand, error) {
	return orderport.RefundCallbackCommand{}, orderport.ErrWeChatPayDisabled
}

// DisabledWeChatShopRefund is intentionally separate from DisabledWeChatPay;
// enabling one provider can never enable or route the other provider.
type DisabledWeChatShopRefund struct{}

func (DisabledWeChatShopRefund) Enabled() bool { return false }

func (DisabledWeChatShopRefund) RequestRefund(context.Context, orderport.WeChatShopRefundRequest) (orderport.WeChatShopProviderResult, error) {
	return orderport.WeChatShopProviderResult{}, orderport.ErrWeChatShopRefundDisabled
}

func (DisabledWeChatShopRefund) QueryRefund(context.Context, string) (orderport.WeChatShopRefundQueryResult, error) {
	return orderport.WeChatShopRefundQueryResult{}, orderport.ErrWeChatShopRefundDisabled
}

type DisabledWeChatShopCallbackVerifier struct{}

func (DisabledWeChatShopCallbackVerifier) VerifyRefund(context.Context, []byte, map[string]string) (orderport.WeChatShopRefundCallbackCommand, error) {
	return orderport.WeChatShopRefundCallbackCommand{}, orderport.ErrWeChatShopRefundDisabled
}
