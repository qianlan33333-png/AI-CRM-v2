package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
)

type directUOW struct{}

func (directUOW) Within(ctx context.Context, call func(context.Context) error) error {
	return call(ctx)
}

type payMaterialReaderStub struct {
	prepay orderstore.PE01PrepayProviderMaterial
	refund orderstore.PE01RefundProviderMaterial
}

func (stub payMaterialReaderStub) ReadPE01Prepay(context.Context, string) (orderstore.PE01PrepayProviderMaterial, bool, error) {
	return stub.prepay, true, nil
}
func (stub payMaterialReaderStub) ReadPE01Refund(context.Context, string) (orderstore.PE01RefundProviderMaterial, bool, error) {
	return stub.refund, true, nil
}

type openIDReaderStub struct {
	value identityport.VerifiedMPOpenID
	err   error
}

func (stub openIDReaderStub) ResolveUniqueVerifiedMPOpenID(context.Context, contactport.CustomerID, string) (identityport.VerifiedMPOpenID, bool, error) {
	return stub.value, stub.err == nil, stub.err
}

func TestWeChatPayRuntimeDisabledHasNoProviderConstruction(t *testing.T) {
	provider, err := newWeChatPayProviderRuntime(appconfig.WeChatPayProvider{}, directUOW{}, nil)
	if err != nil || provider == nil {
		t.Fatalf("disabled provider = %#v, %v", provider, err)
	}
	verifier, err := newWeChatPayCallbackRuntime(appconfig.WeChatPayProvider{})
	if err != nil || verifier == nil {
		t.Fatalf("disabled verifier = %#v, %v", verifier, err)
	}
}

func TestWeChatPayMaterializerUsesExactCanonicalFacts(t *testing.T) {
	identityDigest := sha256.Sum256([]byte("identity"))
	reasonDigest := sha256.Sum256([]byte("pe01/refund-reason/v1\x00duplicate"))
	materializer := weChatPayMaterializer{
		uow: directUOW{},
		orders: payMaterialReaderStub{
			prepay: orderstore.PE01PrepayProviderMaterial{MerchantOrderNo: "order-1", CustomerID: 7, ProductName: "CRM 套餐", AmountMinor: 9900, Currency: "CNY", PaymentIdentityDigest: identityDigest},
			refund: orderstore.PE01RefundProviderMaterial{MerchantOrderNo: "order-1", OriginalAmountMinor: 9900, Currency: "CNY", OutRefundNo: "refund-1", RefundAmountMinor: 9900, Reason: "duplicate"},
		},
		identities: openIDReaderStub{value: identityport.VerifiedMPOpenID{CustomerID: 7, OpenID: "openid-7"}}, scope: "wechat-app:app-1",
	}
	prepay, err := materializer.ResolvePrepay(context.Background(), orderport.PrepayRequest{MerchantOrderNo: "order-1", AmountMinor: 9900, Currency: "CNY", PayerIdentityDigest: identityDigest})
	if err != nil || prepay.PayerOpenID != "openid-7" || prepay.Description != "CRM 套餐" || prepay.PayerIdentityDigest != identityDigest {
		t.Fatalf("prepay material = %#v, %v", prepay, err)
	}
	refund, err := materializer.ResolveRefund(context.Background(), orderport.RefundRequest{MerchantOrderNo: "order-1", OutRefundNo: "refund-1", AmountMinor: 9900, Currency: "CNY", ReasonDigest: reasonDigest})
	if err != nil || refund.OriginalAmountMinor != 9900 || refund.Reason != "duplicate" || refund.ReasonDigest != reasonDigest {
		t.Fatalf("refund material = %#v, %v", refund, err)
	}
	_, err = materializer.ResolvePrepay(context.Background(), orderport.PrepayRequest{MerchantOrderNo: "order-1", AmountMinor: 1, Currency: "CNY", PayerIdentityDigest: identityDigest})
	if !errors.Is(err, errInvalidWeChatPayRuntime) {
		t.Fatalf("mismatched material error = %v", err)
	}
}
