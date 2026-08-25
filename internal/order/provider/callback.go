package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

var ErrInvalidWeChatPayCallback = errors.New("invalid wechat pay callback")

type SignatureVerifier interface {
	Verify(context.Context, string, string, string) error
}

type ResourceDecryptor interface {
	Decrypt(context.Context, string, string, string) ([]byte, error)
}

type ClosedCallbackVerifier struct {
	signatures SignatureVerifier
	resources  ResourceDecryptor
	now        func() time.Time
}

func NewClosedCallbackVerifier(signatures SignatureVerifier, resources ResourceDecryptor) (*ClosedCallbackVerifier, error) {
	if signatures == nil || resources == nil {
		return nil, ErrInvalidWeChatPayCallback
	}
	return &ClosedCallbackVerifier{signatures: signatures, resources: resources, now: time.Now}, nil
}

type callbackEnvelope struct {
	ID           string `json:"id"`
	EventType    string `json:"event_type"`
	ResourceType string `json:"resource_type"`
	Resource     struct {
		Algorithm      string `json:"algorithm"`
		Ciphertext     string `json:"ciphertext"`
		Nonce          string `json:"nonce"`
		AssociatedData string `json:"associated_data"`
	} `json:"resource"`
}

func (verifier *ClosedCallbackVerifier) VerifyPayment(ctx context.Context, body []byte, headers map[string]string) (orderport.PaymentCallbackCommand, error) {
	plain, envelope, received, err := verifier.verify(ctx, body, headers, "TRANSACTION.SUCCESS")
	if err != nil {
		return orderport.PaymentCallbackCommand{}, err
	}
	var payload struct {
		MerchantOrderNo string `json:"out_trade_no"`
		TransactionID   string `json:"transaction_id"`
		TradeState      string `json:"trade_state"`
		SuccessTime     string `json:"success_time"`
		Amount          struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	if json.Unmarshal(plain, &payload) != nil || payload.TradeState != "SUCCESS" || payload.MerchantOrderNo == "" || payload.TransactionID == "" || payload.Amount.Total < 1 || payload.Amount.Currency != "CNY" {
		return orderport.PaymentCallbackCommand{}, ErrInvalidWeChatPayCallback
	}
	occurred, err := callbackTime(payload.SuccessTime, received)
	if err != nil {
		return orderport.PaymentCallbackCommand{}, err
	}
	return orderport.PaymentCallbackCommand{MerchantOrderNo: payload.MerchantOrderNo, ProviderEventDigest: callbackDigest("pe01/wechat-event/v1", envelope.ID), PayloadDigest: sha256.Sum256(body), ProviderTransactionDigest: callbackDigest("pe01/wechat-transaction/v1", payload.TransactionID), AmountMinor: payload.Amount.Total, Currency: payload.Amount.Currency, Succeeded: true, OccurredAt: occurred}, nil
}

func (verifier *ClosedCallbackVerifier) VerifyRefund(ctx context.Context, body []byte, headers map[string]string) (orderport.RefundCallbackCommand, error) {
	plain, envelope, received, err := verifier.verify(ctx, body, headers, "REFUND.SUCCESS")
	if err != nil {
		return orderport.RefundCallbackCommand{}, err
	}
	var payload struct {
		OutRefundNo  string `json:"out_refund_no"`
		RefundID     string `json:"refund_id"`
		RefundStatus string `json:"refund_status"`
		SuccessTime  string `json:"success_time"`
		Amount       struct {
			Refund   int64  `json:"refund"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	if json.Unmarshal(plain, &payload) != nil || payload.RefundStatus != "SUCCESS" || payload.OutRefundNo == "" || payload.RefundID == "" || payload.Amount.Refund < 1 || payload.Amount.Currency != "CNY" {
		return orderport.RefundCallbackCommand{}, ErrInvalidWeChatPayCallback
	}
	occurred, err := callbackTime(payload.SuccessTime, received)
	if err != nil {
		return orderport.RefundCallbackCommand{}, err
	}
	return orderport.RefundCallbackCommand{OutRefundNo: payload.OutRefundNo, ProviderEventDigest: callbackDigest("pe01/wechat-event/v1", envelope.ID), PayloadDigest: sha256.Sum256(body), ProviderRefundDigest: callbackDigest("pe01/wechat-refund/v1", payload.RefundID), AmountMinor: payload.Amount.Refund, Currency: payload.Amount.Currency, Succeeded: true, OccurredAt: occurred}, nil
}

func (verifier *ClosedCallbackVerifier) verify(ctx context.Context, body []byte, headers map[string]string, eventType string) ([]byte, callbackEnvelope, time.Time, error) {
	if verifier == nil || verifier.signatures == nil || verifier.resources == nil || verifier.now == nil || len(body) == 0 || len(body) > 128<<10 {
		return nil, callbackEnvelope{}, time.Time{}, ErrInvalidWeChatPayCallback
	}
	timestamp, nonce, serial, signature := headers["Wechatpay-Timestamp"], headers["Wechatpay-Nonce"], headers["Wechatpay-Serial"], headers["Wechatpay-Signature"]
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	received := verifier.now().UTC()
	if err != nil || nonce == "" || serial == "" || signature == "" || received.Sub(time.Unix(seconds, 0).UTC()) > 5*time.Minute || received.Sub(time.Unix(seconds, 0).UTC()) < -5*time.Minute {
		return nil, callbackEnvelope{}, time.Time{}, ErrInvalidWeChatPayCallback
	}
	message := timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	if err = verifier.signatures.Verify(ctx, message, signature, serial); err != nil {
		return nil, callbackEnvelope{}, time.Time{}, ErrInvalidWeChatPayCallback
	}
	var envelope callbackEnvelope
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil || decoder.Decode(&struct{}{}) != io.EOF || envelope.ID == "" || envelope.EventType != eventType || envelope.ResourceType != "encrypt-resource" || envelope.Resource.Algorithm != "AEAD_AES_256_GCM" || envelope.Resource.Ciphertext == "" || envelope.Resource.Nonce == "" {
		return nil, callbackEnvelope{}, time.Time{}, ErrInvalidWeChatPayCallback
	}
	plain, err := verifier.resources.Decrypt(ctx, envelope.Resource.Ciphertext, envelope.Resource.Nonce, envelope.Resource.AssociatedData)
	if err != nil || len(plain) == 0 || len(plain) > 64<<10 || !json.Valid(plain) {
		return nil, callbackEnvelope{}, time.Time{}, ErrInvalidWeChatPayCallback
	}
	return plain, envelope, received, nil
}

func callbackTime(value string, fallback time.Time) (time.Time, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, ErrInvalidWeChatPayCallback
	}
	return parsed.UTC(), nil
}

func callbackDigest(domain, value string) [32]byte {
	return sha256.Sum256([]byte(domain + "\x00" + value))
}
