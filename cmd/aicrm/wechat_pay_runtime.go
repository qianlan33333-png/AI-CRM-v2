package main

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"net/http"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var errInvalidWeChatPayRuntime = errors.New("invalid wechat pay runtime")

func newWeChatPayProviderRuntime(config appconfig.WeChatPayProvider, uow platformport.UnitOfWork, client *http.Client) (orderport.WeChatPayProvider, error) {
	if !config.Enabled {
		return orderprovider.DisabledWeChatPay{}, nil
	}
	if uow == nil || client == nil || !config.PermissionConfirmed {
		return nil, errInvalidWeChatPayRuntime
	}
	signer, err := orderprovider.ParseRSAPrivateKeyPEM([]byte(config.MerchantPrivateKey.Value()))
	if err != nil {
		return nil, errInvalidWeChatPayRuntime
	}
	platformKey, err := orderprovider.ParseRSAPublicKeyPEM([]byte(config.PlatformCertificate.Value()))
	if err != nil {
		return nil, errInvalidWeChatPayRuntime
	}
	credential, err := orderprovider.NewWeChatPayCredential(config.MerchantID, config.MerchantSerial, signer, nil, map[string]*rsa.PublicKey{config.PlatformSerial: platformKey})
	if err != nil {
		return nil, errInvalidWeChatPayRuntime
	}
	providerConfig := orderprovider.WeChatPayConfig{Enabled: true, AppID: config.AppID, APIBaseURL: "https://api.mch.weixin.qq.com", PaymentNotifyURL: config.PaymentNotifyURL, RefundNotifyURL: config.RefundNotifyURL, Credential: credential}
	materializer := &weChatPayMaterializer{uow: uow, orders: orderstore.NewProviderMaterialReader(), identities: identitystore.NewRepository(), scope: "wechat-app:" + config.AppID}
	provider, err := orderprovider.NewWeChatPay(providerConfig, materializer, client)
	if err != nil {
		return nil, errInvalidWeChatPayRuntime
	}
	return provider, nil
}

func newWeChatPayCallbackRuntime(config appconfig.WeChatPayProvider) (orderport.CallbackVerifier, error) {
	if !config.Enabled {
		return orderprovider.DisabledCallbackVerifier{}, nil
	}
	platformKey, err := orderprovider.ParseRSAPublicKeyPEM([]byte(config.PlatformCertificate.Value()))
	if err != nil {
		return nil, errInvalidWeChatPayRuntime
	}
	credential, err := orderprovider.NewWeChatPayCallbackCredential(config.MerchantID, []byte(config.APIV3Key.Value()), map[string]*rsa.PublicKey{config.PlatformSerial: platformKey})
	if err != nil {
		return nil, errInvalidWeChatPayRuntime
	}
	verifier, err := orderprovider.NewWeChatPayCallbackVerifier(orderprovider.WeChatPayConfig{AppID: config.AppID, Credential: credential})
	if err != nil {
		return nil, errInvalidWeChatPayRuntime
	}
	return verifier, nil
}

type weChatPayMaterializer struct {
	uow        platformport.UnitOfWork
	orders     weChatPayOrderMaterialReader
	identities identityport.VerifiedMPOpenIDReader
	scope      string
}

type weChatPayOrderMaterialReader interface {
	ReadPE01Prepay(context.Context, string) (orderstore.PE01PrepayProviderMaterial, bool, error)
	ReadPE01Refund(context.Context, string) (orderstore.PE01RefundProviderMaterial, bool, error)
}

func (materializer *weChatPayMaterializer) ResolvePrepay(ctx context.Context, request orderport.PrepayRequest) (orderprovider.WeChatPayPrepayMaterial, error) {
	if materializer == nil || materializer.uow == nil || materializer.orders == nil || materializer.identities == nil || materializer.scope == "" {
		return orderprovider.WeChatPayPrepayMaterial{}, errInvalidWeChatPayRuntime
	}
	var result orderprovider.WeChatPayPrepayMaterial
	err := materializer.uow.Within(ctx, func(tx context.Context) error {
		record, found, err := materializer.orders.ReadPE01Prepay(tx, request.MerchantOrderNo)
		if err != nil || !found || record.MerchantOrderNo != request.MerchantOrderNo || record.AmountMinor != request.AmountMinor || record.Currency != request.Currency || record.PaymentIdentityDigest != request.PayerIdentityDigest {
			return errInvalidWeChatPayRuntime
		}
		openid, found, err := materializer.identities.ResolveUniqueVerifiedMPOpenID(tx, identityCustomerID(record.CustomerID), materializer.scope)
		if err != nil || !found || openid.CustomerID != identityCustomerID(record.CustomerID) {
			return errInvalidWeChatPayRuntime
		}
		result = orderprovider.WeChatPayPrepayMaterial{Description: record.ProductName, PayerOpenID: openid.OpenID, PayerIdentityDigest: record.PaymentIdentityDigest}
		return nil
	})
	if err != nil {
		return orderprovider.WeChatPayPrepayMaterial{}, errInvalidWeChatPayRuntime
	}
	return result, nil
}

func (materializer *weChatPayMaterializer) ResolveRefund(ctx context.Context, request orderport.RefundRequest) (orderprovider.WeChatPayRefundMaterial, error) {
	if materializer == nil || materializer.uow == nil || materializer.orders == nil {
		return orderprovider.WeChatPayRefundMaterial{}, errInvalidWeChatPayRuntime
	}
	var result orderprovider.WeChatPayRefundMaterial
	err := materializer.uow.Within(ctx, func(tx context.Context) error {
		record, found, err := materializer.orders.ReadPE01Refund(tx, request.OutRefundNo)
		if err != nil || !found || record.OutRefundNo != request.OutRefundNo || record.MerchantOrderNo != request.MerchantOrderNo || record.RefundAmountMinor != request.AmountMinor || record.Currency != request.Currency {
			return errInvalidWeChatPayRuntime
		}
		reasonDigest := sha256.Sum256([]byte("pe01/refund-reason/v1\x00" + record.Reason))
		if reasonDigest != request.ReasonDigest {
			return errInvalidWeChatPayRuntime
		}
		result = orderprovider.WeChatPayRefundMaterial{OriginalAmountMinor: record.OriginalAmountMinor, Reason: record.Reason, ReasonDigest: reasonDigest}
		return nil
	})
	if err != nil {
		return orderprovider.WeChatPayRefundMaterial{}, errInvalidWeChatPayRuntime
	}
	return result, nil
}

func identityCustomerID(value int64) contactport.CustomerID { return contactport.CustomerID(value) }
