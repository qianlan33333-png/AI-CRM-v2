package provider

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

const weChatPayAuthorizationScheme = "WECHATPAY2-SHA256-RSA2048"

const jsapiPrepayTTL = 2 * time.Hour

type WeChatPayCredential struct {
	merchantID      string
	merchantSerial  string
	merchantSigner  crypto.Signer
	apiV3Key        [32]byte
	hasAPIV3Key     bool
	platformRSAKeys map[string]*rsa.PublicKey
}

func NewWeChatPayCredential(merchantID, merchantSerial string, signer crypto.Signer, apiV3Key []byte, platformKeys map[string]*rsa.PublicKey) (WeChatPayCredential, error) {
	merchantID, merchantSerial = strings.TrimSpace(merchantID), strings.TrimSpace(merchantSerial)
	publicKey, rsaSigner := signerPublicRSA(signer)
	if merchantID == "" || merchantSerial == "" || !rsaSigner || publicKey.Size() < 256 || len(platformKeys) == 0 {
		return WeChatPayCredential{}, ErrInvalidProviderConfig
	}
	keys := make(map[string]*rsa.PublicKey, len(platformKeys))
	for serial, key := range platformKeys {
		serial = strings.TrimSpace(serial)
		if serial == "" || key == nil || key.Size() < 256 {
			return WeChatPayCredential{}, ErrInvalidProviderConfig
		}
		keys[serial] = key
	}
	credential := WeChatPayCredential{merchantID: merchantID, merchantSerial: merchantSerial, merchantSigner: signer, platformRSAKeys: keys}
	if len(apiV3Key) != 0 {
		if len(apiV3Key) != len(credential.apiV3Key) {
			return WeChatPayCredential{}, ErrInvalidProviderConfig
		}
		copy(credential.apiV3Key[:], apiV3Key)
		credential.hasAPIV3Key = true
	}
	return credential, nil
}

// NewWeChatPayCallbackCredential builds the verifier-only credential used by
// the API process. It deliberately has no merchant signer, so payment write
// credentials do not need to enter the callback process.
func NewWeChatPayCallbackCredential(merchantID string, apiV3Key []byte, platformKeys map[string]*rsa.PublicKey) (WeChatPayCredential, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" || len(apiV3Key) != 32 || len(platformKeys) == 0 {
		return WeChatPayCredential{}, ErrInvalidProviderConfig
	}
	keys := make(map[string]*rsa.PublicKey, len(platformKeys))
	for serial, key := range platformKeys {
		serial = strings.TrimSpace(serial)
		if serial == "" || key == nil || key.Size() < 256 {
			return WeChatPayCredential{}, ErrInvalidProviderConfig
		}
		keys[serial] = key
	}
	credential := WeChatPayCredential{merchantID: merchantID, platformRSAKeys: keys, hasAPIV3Key: true}
	copy(credential.apiV3Key[:], apiV3Key)
	return credential, nil
}

func (WeChatPayCredential) String() string   { return "[REDACTED]" }
func (WeChatPayCredential) GoString() string { return "[REDACTED]" }

type WeChatPayConfig struct {
	Enabled          bool
	AppID            string
	APIBaseURL       string
	PaymentNotifyURL string
	RefundNotifyURL  string
	Credential       WeChatPayCredential
}

func (WeChatPayConfig) String() string   { return "WeChatPayConfig{credentials:[REDACTED]}" }
func (WeChatPayConfig) GoString() string { return "WeChatPayConfig{credentials:[REDACTED]}" }

type WeChatPayPrepayMaterial struct {
	Description         string
	PayerOpenID         string
	PayerIdentityDigest [32]byte
}

type WeChatPayRefundMaterial struct {
	OriginalAmountMinor int64
	Reason              string
	ReasonDigest        [32]byte
}

// WeChatPayMaterializer resolves the provider values intentionally absent from
// the digest-only Order port. It must verify the request digest against its
// canonical identity/order facts before returning material.
type WeChatPayMaterializer interface {
	ResolvePrepay(context.Context, orderport.PrepayRequest) (WeChatPayPrepayMaterial, error)
	ResolveRefund(context.Context, orderport.RefundRequest) (WeChatPayRefundMaterial, error)
}

type WeChatPay struct {
	config       WeChatPayConfig
	baseURL      *url.URL
	materializer WeChatPayMaterializer
	httpClient   HTTPDoer
	now          func() time.Time
	nonce        func() (string, error)
}

var _ orderport.WeChatPayProvider = (*WeChatPay)(nil)

func NewWeChatPay(config WeChatPayConfig, materializer WeChatPayMaterializer, httpClient HTTPDoer) (*WeChatPay, error) {
	baseURL, valid := validHTTPSBase(config.APIBaseURL)
	if !config.Enabled || strings.TrimSpace(config.AppID) == "" || !validHTTPSURL(config.PaymentNotifyURL) || !validHTTPSURL(config.RefundNotifyURL) || !valid || materializer == nil || httpClient == nil || !config.Credential.valid() {
		return nil, ErrInvalidProviderConfig
	}
	config.AppID = strings.TrimSpace(config.AppID)
	config.PaymentNotifyURL = strings.TrimSpace(config.PaymentNotifyURL)
	config.RefundNotifyURL = strings.TrimSpace(config.RefundNotifyURL)
	return &WeChatPay{config: config, baseURL: baseURL, materializer: materializer, httpClient: httpClient, now: time.Now, nonce: randomNonce}, nil
}

func (provider *WeChatPay) CreatePrepay(ctx context.Context, request orderport.PrepayRequest) (orderport.ProviderResult, error) {
	if provider == nil || ctx == nil || ctx.Err() != nil || !validPrepayRequest(request) {
		return finalPaymentFailure("wechat-pay/prepay-invalid/v1", request.MerchantOrderNo), nil
	}
	material, err := provider.materializer.ResolvePrepay(ctx, request)
	if err != nil || !validPrepayMaterial(request, material) {
		return finalPaymentFailure("wechat-pay/prepay-material/v1", request.MerchantOrderNo), nil
	}
	createdAt := provider.now().UTC()
	expiresAt := createdAt.Add(jsapiPrepayTTL)
	payload := struct {
		AppID       string `json:"appid"`
		MerchantID  string `json:"mchid"`
		Description string `json:"description"`
		OutTradeNo  string `json:"out_trade_no"`
		NotifyURL   string `json:"notify_url"`
		TimeExpire  string `json:"time_expire"`
		Amount      struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
		Payer struct {
			OpenID string `json:"openid"`
		} `json:"payer"`
	}{AppID: provider.config.AppID, MerchantID: provider.config.Credential.merchantID, Description: material.Description, OutTradeNo: request.MerchantOrderNo, NotifyURL: provider.config.PaymentNotifyURL, TimeExpire: expiresAt.Format(time.RFC3339)}
	payload.Amount.Total, payload.Amount.Currency, payload.Payer.OpenID = request.AmountMinor, request.Currency, material.PayerOpenID
	response, outcome := provider.signedJSON(ctx, http.MethodPost, "/v3/pay/transactions/jsapi", payload)
	if outcome != paymentCallExecuted {
		return providerResultForCall("wechat-pay/prepay/v1", request.MerchantOrderNo, response, outcome), nil
	}
	var result struct {
		PrepayID string `json:"prepay_id"`
	}
	if json.Unmarshal(response.body, &result) != nil || strings.TrimSpace(result.PrepayID) == "" {
		return unknownPaymentResult("wechat-pay/prepay-response/v1", request.MerchantOrderNo, response.body), nil
	}
	handoff, err := provider.buildJSAPIHandoff(result.PrepayID, createdAt, expiresAt)
	if err != nil {
		return unknownPaymentResult("wechat-pay/prepay-handoff/v1", request.MerchantOrderNo, response.body), nil
	}
	receipt := providerDigest("wechat-pay/prepay-executed/v2", request.MerchantOrderNo, result.PrepayID, digestHex(sha256.Sum256(response.body)), handoff.AppID, handoff.TimeStamp, handoff.NonceStr, handoff.Package, handoff.SignType, handoff.PaySign, handoff.ExpiresAt.Format(time.RFC3339Nano))
	providerResult := dispatchedPaymentResult(orderport.ProviderExecuted, receipt)
	providerResult.JSAPIHandoff = &handoff
	return providerResult, nil
}

func (provider *WeChatPay) buildJSAPIHandoff(prepayID string, createdAt, expiresAt time.Time) (orderport.JSAPIHandoff, error) {
	prepayID = strings.TrimSpace(prepayID)
	if provider == nil || !validProviderReference(prepayID) || provider.nonce == nil || !expiresAt.After(createdAt) {
		return orderport.JSAPIHandoff{}, ErrInvalidProviderResponse
	}
	nonce, err := provider.nonce()
	if err != nil || !validProviderReference(nonce) {
		return orderport.JSAPIHandoff{}, ErrInvalidProviderResponse
	}
	timestamp := strconv.FormatInt(createdAt.Unix(), 10)
	packageValue := "prepay_id=" + prepayID
	signature, err := provider.config.Credential.sign(provider.config.AppID + "\n" + timestamp + "\n" + nonce + "\n" + packageValue + "\n")
	if err != nil || signature == "" {
		return orderport.JSAPIHandoff{}, ErrInvalidProviderResponse
	}
	return orderport.JSAPIHandoff{AppID: provider.config.AppID, TimeStamp: timestamp, NonceStr: nonce, Package: packageValue, SignType: "RSA", PaySign: signature, ExpiresAt: expiresAt}, nil
}

func (provider *WeChatPay) RequestRefund(ctx context.Context, request orderport.RefundRequest) (orderport.ProviderResult, error) {
	if provider == nil || ctx == nil || ctx.Err() != nil || !validRefundRequest(request) {
		return finalPaymentFailure("wechat-pay/refund-invalid/v1", request.OutRefundNo), nil
	}
	material, err := provider.materializer.ResolveRefund(ctx, request)
	if err != nil || !validRefundMaterial(request, material) {
		return finalPaymentFailure("wechat-pay/refund-material/v1", request.OutRefundNo), nil
	}
	payload := struct {
		OutTradeNo  string `json:"out_trade_no"`
		OutRefundNo string `json:"out_refund_no"`
		Reason      string `json:"reason,omitempty"`
		NotifyURL   string `json:"notify_url"`
		Amount      struct {
			Refund   int64  `json:"refund"`
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}{OutTradeNo: request.MerchantOrderNo, OutRefundNo: request.OutRefundNo, Reason: material.Reason, NotifyURL: provider.config.RefundNotifyURL}
	payload.Amount.Refund, payload.Amount.Total, payload.Amount.Currency = request.AmountMinor, material.OriginalAmountMinor, request.Currency
	response, outcome := provider.signedJSON(ctx, http.MethodPost, "/v3/refund/domestic/refunds", payload)
	if outcome != paymentCallExecuted {
		return providerResultForCall("wechat-pay/refund/v1", request.OutRefundNo, response, outcome), nil
	}
	var result struct {
		RefundID    string `json:"refund_id"`
		OutRefundNo string `json:"out_refund_no"`
		Status      string `json:"status"`
	}
	if json.Unmarshal(response.body, &result) != nil || strings.TrimSpace(result.RefundID) == "" || result.OutRefundNo != request.OutRefundNo || strings.TrimSpace(result.Status) == "" {
		return unknownPaymentResult("wechat-pay/refund-response/v1", request.OutRefundNo, response.body), nil
	}
	return dispatchedPaymentResult(orderport.ProviderExecuted, providerDigest("wechat-pay/refund-executed/v1", request.OutRefundNo, result.RefundID, result.Status, digestHex(sha256.Sum256(response.body)))), nil
}

func (provider *WeChatPay) QueryPayment(ctx context.Context, merchantOrderNo string) (orderport.PaymentQueryResult, error) {
	merchantOrderNo = strings.TrimSpace(merchantOrderNo)
	if provider == nil || !validProviderReference(merchantOrderNo) {
		return orderport.PaymentQueryResult{}, ErrInvalidProviderMaterial
	}
	path := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(merchantOrderNo) + "?mchid=" + url.QueryEscape(provider.config.Credential.merchantID)
	response, outcome := provider.signedJSON(ctx, http.MethodGet, path, nil)
	if outcome != paymentCallExecuted {
		return orderport.PaymentQueryResult{}, ErrProviderUnavailable
	}
	var payload struct {
		MerchantOrderNo string `json:"out_trade_no"`
		TradeState      string `json:"trade_state"`
		TransactionID   string `json:"transaction_id"`
		SuccessTime     string `json:"success_time"`
		Amount          struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	evidence := providerDigest("wechat-pay/query-payment/v1", merchantOrderNo, digestHex(sha256.Sum256(response.body)))
	if json.Unmarshal(response.body, &payload) != nil || payload.MerchantOrderNo != merchantOrderNo || strings.TrimSpace(payload.TradeState) == "" {
		return orderport.PaymentQueryResult{}, ErrInvalidProviderResponse
	}
	if payload.TradeState != "SUCCESS" {
		return orderport.PaymentQueryResult{EvidenceDigest: evidence}, nil
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, payload.SuccessTime)
	if err != nil || payload.TransactionID == "" || payload.Amount.Total < 1 || payload.Amount.Currency != "CNY" {
		return orderport.PaymentQueryResult{}, ErrInvalidProviderResponse
	}
	return orderport.PaymentQueryResult{Confirmed: true, EvidenceDigest: evidence, ProviderTransactionDigest: providerDigest("wechat-pay/transaction/v1", payload.TransactionID), AmountMinor: payload.Amount.Total, Currency: payload.Amount.Currency, OccurredAt: occurredAt.UTC()}, nil
}

func (provider *WeChatPay) QueryRefund(ctx context.Context, outRefundNo string) (orderport.RefundQueryResult, error) {
	outRefundNo = strings.TrimSpace(outRefundNo)
	if provider == nil || !validProviderReference(outRefundNo) {
		return orderport.RefundQueryResult{}, ErrInvalidProviderMaterial
	}
	response, outcome := provider.signedJSON(ctx, http.MethodGet, "/v3/refund/domestic/refunds/"+url.PathEscape(outRefundNo), nil)
	if outcome != paymentCallExecuted {
		return orderport.RefundQueryResult{}, ErrProviderUnavailable
	}
	var payload struct {
		RefundID    string `json:"refund_id"`
		OutRefundNo string `json:"out_refund_no"`
		Status      string `json:"status"`
		SuccessTime string `json:"success_time"`
		Amount      struct {
			Refund   int64  `json:"refund"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	evidence := providerDigest("wechat-pay/query-refund/v1", outRefundNo, digestHex(sha256.Sum256(response.body)))
	if json.Unmarshal(response.body, &payload) != nil || payload.OutRefundNo != outRefundNo || strings.TrimSpace(payload.Status) == "" {
		return orderport.RefundQueryResult{}, ErrInvalidProviderResponse
	}
	if payload.Status != "SUCCESS" {
		return orderport.RefundQueryResult{EvidenceDigest: evidence}, nil
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, payload.SuccessTime)
	if err != nil || payload.RefundID == "" || payload.Amount.Refund < 1 || payload.Amount.Currency != "CNY" {
		return orderport.RefundQueryResult{}, ErrInvalidProviderResponse
	}
	return orderport.RefundQueryResult{Confirmed: true, EvidenceDigest: evidence, ProviderRefundDigest: providerDigest("wechat-pay/refund-id/v1", payload.RefundID), AmountMinor: payload.Amount.Refund, Currency: payload.Amount.Currency, OccurredAt: occurredAt.UTC()}, nil
}

type paymentCallOutcome uint8

const (
	paymentCallNotDispatched paymentCallOutcome = iota
	paymentCallOutcomeUnknown
	paymentCallFinalFailed
	paymentCallExecuted
)

type paymentResponse struct {
	status int
	body   []byte
}

func (provider *WeChatPay) signedJSON(ctx context.Context, method, requestTarget string, payload any) (paymentResponse, paymentCallOutcome) {
	if provider == nil || provider.now == nil || provider.nonce == nil || ctx == nil || ctx.Err() != nil {
		return paymentResponse{}, paymentCallNotDispatched
	}
	body := []byte(nil)
	var err error
	if method != http.MethodGet {
		body, err = json.Marshal(payload)
		if err != nil {
			return paymentResponse{}, paymentCallNotDispatched
		}
	}
	nonce, err := provider.nonce()
	if err != nil || nonce == "" {
		return paymentResponse{}, paymentCallNotDispatched
	}
	timestamp := strconv.FormatInt(provider.now().UTC().Unix(), 10)
	message := method + "\n" + requestTarget + "\n" + timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	signature, err := provider.config.Credential.sign(message)
	if err != nil {
		return paymentResponse{}, paymentCallNotDispatched
	}
	request, err := http.NewRequestWithContext(ctx, method, provider.baseURL.String()+requestTarget, strings.NewReader(string(body)))
	if err != nil {
		return paymentResponse{}, paymentCallNotDispatched
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf(`%s mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`, weChatPayAuthorizationScheme, provider.config.Credential.merchantID, nonce, timestamp, provider.config.Credential.merchantSerial, signature))
	response, err := provider.httpClient.Do(request)
	if err != nil {
		return paymentResponse{}, paymentCallOutcomeUnknown
	}
	responseBody, err := readProviderResponse(response)
	result := paymentResponse{status: response.StatusCode, body: responseBody}
	if err != nil || provider.config.Credential.verifyPlatformResponse(provider.now().UTC(), response.Header, responseBody) != nil {
		return result, paymentCallOutcomeUnknown
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return result, paymentCallExecuted
	}
	if response.StatusCode >= 400 && response.StatusCode < 500 {
		return result, paymentCallFinalFailed
	}
	return result, paymentCallOutcomeUnknown
}

func (credential WeChatPayCredential) valid() bool {
	_, ok := signerPublicRSA(credential.merchantSigner)
	return credential.merchantID != "" && credential.merchantSerial != "" && ok && len(credential.platformRSAKeys) > 0
}

func signerPublicRSA(signer crypto.Signer) (*rsa.PublicKey, bool) {
	if signer == nil {
		return nil, false
	}
	publicKey, ok := signer.Public().(*rsa.PublicKey)
	return publicKey, ok && publicKey != nil
}

func (credential WeChatPayCredential) sign(message string) (string, error) {
	digest := sha256.Sum256([]byte(message))
	signature, err := credential.merchantSigner.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return "", ErrInvalidProviderConfig
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func (credential WeChatPayCredential) verifyPlatformResponse(now time.Time, headers http.Header, body []byte) error {
	timestamp, nonce := headers.Get("Wechatpay-Timestamp"), headers.Get("Wechatpay-Nonce")
	serial, signature := headers.Get("Wechatpay-Serial"), headers.Get("Wechatpay-Signature")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	key := credential.platformRSAKeys[serial]
	if err != nil || nonce == "" || key == nil || signature == "" || now.Sub(time.Unix(seconds, 0).UTC()) > 5*time.Minute || now.Sub(time.Unix(seconds, 0).UTC()) < -5*time.Minute {
		return ErrInvalidProviderResponse
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(signature)
	messageDigest := sha256.Sum256([]byte(timestamp + "\n" + nonce + "\n" + string(body) + "\n"))
	if err != nil || rsa.VerifyPKCS1v15(key, crypto.SHA256, messageDigest[:], decoded) != nil {
		return ErrInvalidProviderResponse
	}
	return nil
}

type rsaPlatformVerifier struct{ keys map[string]*rsa.PublicKey }

func (verifier rsaPlatformVerifier) Verify(_ context.Context, message, signature, serial string) error {
	key := verifier.keys[strings.TrimSpace(serial)]
	decoded, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(signature))
	digest := sha256.Sum256([]byte(message))
	if key == nil || err != nil || rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], decoded) != nil {
		return ErrInvalidWeChatPayCallback
	}
	return nil
}

type aesGCMResourceDecryptor struct{ key [32]byte }

func (decryptor aesGCMResourceDecryptor) Decrypt(_ context.Context, ciphertext, nonce, associatedData string) ([]byte, error) {
	block, err := aes.NewCipher(decryptor.key[:])
	if err != nil {
		return nil, ErrInvalidWeChatPayCallback
	}
	gcm, err := cipher.NewGCM(block)
	decoded, decodeErr := base64.StdEncoding.Strict().DecodeString(ciphertext)
	if err != nil || decodeErr != nil {
		return nil, ErrInvalidWeChatPayCallback
	}
	plain, err := gcm.Open(nil, []byte(nonce), decoded, []byte(associatedData))
	if err != nil {
		return nil, ErrInvalidWeChatPayCallback
	}
	return plain, nil
}

func NewWeChatPayCallbackVerifier(config WeChatPayConfig) (*ClosedCallbackVerifier, error) {
	config.AppID = strings.TrimSpace(config.AppID)
	credential := config.Credential
	if config.AppID == "" || !credential.callbackValid() {
		return nil, ErrInvalidProviderConfig
	}
	verifier, err := NewClosedCallbackVerifier(rsaPlatformVerifier{keys: credential.platformRSAKeys}, aesGCMResourceDecryptor{key: credential.apiV3Key})
	if err != nil {
		return nil, err
	}
	verifier.expectedAppID, verifier.expectedMerchant = config.AppID, credential.merchantID
	return verifier, nil
}

func (credential WeChatPayCredential) callbackValid() bool {
	return credential.merchantID != "" && credential.hasAPIV3Key && len(credential.platformRSAKeys) > 0
}

func ParseRSAPrivateKeyPEM(value []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(value)
	if block == nil {
		return nil, ErrInvalidProviderConfig
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if signer, ok := key.(crypto.Signer); ok {
			if publicKey, valid := signerPublicRSA(signer); valid && publicKey.N.BitLen() >= 2048 {
				return signer, nil
			}
		}
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil || key.N.BitLen() < 2048 {
		return nil, ErrInvalidProviderConfig
	}
	return key, nil
}

func ParseRSAPublicKeyPEM(value []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(value)
	if block == nil {
		return nil, ErrInvalidProviderConfig
	}
	if certificate, err := x509.ParseCertificate(block.Bytes); err == nil {
		if key, ok := certificate.PublicKey.(*rsa.PublicKey); ok && key.N.BitLen() >= 2048 {
			return key, nil
		}
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	key, ok := parsed.(*rsa.PublicKey)
	if err != nil || !ok || key.N.BitLen() < 2048 {
		return nil, ErrInvalidProviderConfig
	}
	return key, nil
}

func providerResultForCall(domain, reference string, response paymentResponse, outcome paymentCallOutcome) orderport.ProviderResult {
	receipt := providerDigest(domain, reference, strconv.Itoa(response.status), digestHex(sha256.Sum256(response.body)))
	switch outcome {
	case paymentCallNotDispatched:
		return orderport.ProviderResult{Completion: orderport.ProviderFinalFailed, ReceiptDigest: receipt}
	case paymentCallFinalFailed:
		return dispatchedPaymentResult(orderport.ProviderFinalFailed, receipt)
	default:
		return dispatchedPaymentResult(orderport.ProviderOutcomeUnknown, receipt)
	}
}

func finalPaymentFailure(domain, reference string) orderport.ProviderResult {
	return orderport.ProviderResult{Completion: orderport.ProviderFinalFailed, ReceiptDigest: providerDigest(domain, reference)}
}

func unknownPaymentResult(domain, reference string, body []byte) orderport.ProviderResult {
	return dispatchedPaymentResult(orderport.ProviderOutcomeUnknown, providerDigest(domain, reference, digestHex(sha256.Sum256(body))))
}

func dispatchedPaymentResult(completion orderport.ProviderCompletion, receipt [32]byte) orderport.ProviderResult {
	return orderport.ProviderResult{Completion: completion, ReceiptDigest: receipt, BusinessCallDispatched: true, RealExternalCallExecuted: true}
}

func validPrepayRequest(request orderport.PrepayRequest) bool {
	return validProviderReference(request.MerchantOrderNo) && request.AmountMinor > 0 && request.AmountMinor <= 1_000_000_000 && request.Currency == "CNY" && request.ProductSnapshot != "" && !zeroDigest(request.PayerIdentityDigest) && !zeroDigest(request.ProviderNotifyTarget)
}

func validPrepayMaterial(request orderport.PrepayRequest, material WeChatPayPrepayMaterial) bool {
	return material.PayerIdentityDigest == request.PayerIdentityDigest && strings.TrimSpace(material.Description) == material.Description && material.Description != "" && len(material.Description) <= 127 && strings.TrimSpace(material.PayerOpenID) == material.PayerOpenID && validProviderReference(material.PayerOpenID)
}

func validRefundRequest(request orderport.RefundRequest) bool {
	return validProviderReference(request.MerchantOrderNo) && validProviderReference(request.OutRefundNo) && request.AmountMinor > 0 && request.AmountMinor <= 1_000_000_000 && request.Currency == "CNY" && !zeroDigest(request.ReasonDigest)
}

func validRefundMaterial(request orderport.RefundRequest, material WeChatPayRefundMaterial) bool {
	return material.ReasonDigest == request.ReasonDigest && material.OriginalAmountMinor >= request.AmountMinor && material.OriginalAmountMinor <= 1_000_000_000 && strings.TrimSpace(material.Reason) == material.Reason && material.Reason != "" && len(material.Reason) <= 80
}

func validProviderReference(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '_' && char != '-' && char != '.' {
			return false
		}
	}
	return true
}

func randomNonce() (string, error) {
	var value [16]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", value[:]), nil
}
