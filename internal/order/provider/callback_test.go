package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"
)

type callbackSignatureStub struct{ err error }

func (stub callbackSignatureStub) Verify(context.Context, string, string, string) error {
	return stub.err
}

type callbackDecryptStub struct {
	payload []byte
	err     error
}

func (stub callbackDecryptStub) Decrypt(context.Context, string, string, string) ([]byte, error) {
	return stub.payload, stub.err
}

func TestClosedCallbackVerifierPaymentAndSignatureFailure(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	plain := []byte(`{"out_trade_no":"pe01_0123456789abcdef0123456789abcdef","transaction_id":"wx-transaction-1","trade_state":"SUCCESS","success_time":"2026-08-25T09:59:00Z","amount":{"total":9900,"currency":"CNY"}}`)
	verifier, err := NewClosedCallbackVerifier(callbackSignatureStub{}, callbackDecryptStub{payload: plain})
	if err != nil {
		t.Fatal(err)
	}
	verifier.now = func() time.Time { return now }
	body := []byte(`{"id":"event-1","event_type":"TRANSACTION.SUCCESS","resource_type":"encrypt-resource","resource":{"algorithm":"AEAD_AES_256_GCM","ciphertext":"cipher","nonce":"nonce","associated_data":"transaction"}}`)
	headers := callbackHeaders(now)
	command, err := verifier.VerifyPayment(context.Background(), body, headers)
	if err != nil || command.MerchantOrderNo == "" || command.AmountMinor != 9900 || command.Currency != "CNY" || !command.Succeeded || command.ProviderEventDigest == ([32]byte{}) || command.ProviderTransactionDigest == ([32]byte{}) {
		t.Fatalf("VerifyPayment() = %+v err=%v", command, err)
	}

	failed, _ := NewClosedCallbackVerifier(callbackSignatureStub{err: errors.New("bad signature")}, callbackDecryptStub{payload: plain})
	failed.now = verifier.now
	if _, err = failed.VerifyPayment(context.Background(), body, headers); !errors.Is(err, ErrInvalidWeChatPayCallback) {
		t.Fatalf("signature failure err=%v", err)
	}
	headers["Wechatpay-Timestamp"] = strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10)
	if _, err = verifier.VerifyPayment(context.Background(), body, headers); !errors.Is(err, ErrInvalidWeChatPayCallback) {
		t.Fatalf("stale callback err=%v", err)
	}
}

func callbackHeaders(now time.Time) map[string]string {
	return map[string]string{"Wechatpay-Timestamp": fmt.Sprint(now.Unix()), "Wechatpay-Nonce": "nonce", "Wechatpay-Serial": "serial", "Wechatpay-Signature": "signature"}
}
