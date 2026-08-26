package provider

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

const weChatShopCallbackEvent = "channels_ec_aftersale_update"

var ErrInvalidWeChatShopCallback = errors.New("invalid wechat shop callback")

type WeChatShopCallbackCredential struct {
	appID string
	token string
	key   []byte
}

func NewWeChatShopCallbackCredential(appID, token, encodingAESKey string) (*WeChatShopCallbackCredential, error) {
	key, err := decodeWeChatShopAESKey(encodingAESKey)
	if !validShopCredentialPart(appID, 128) || !validShopCredentialPart(token, 256) || err != nil {
		return nil, ErrInvalidProviderConfig
	}
	return &WeChatShopCallbackCredential{appID: appID, token: token, key: key}, nil
}

func (*WeChatShopCallbackCredential) String() string {
	return "wechat-shop-callback-credential[redacted]"
}
func (*WeChatShopCallbackCredential) GoString() string {
	return "wechat-shop-callback-credential[redacted]"
}

type WeChatShopCallbackVerifier struct {
	credential *WeChatShopCallbackCredential
	now        func() time.Time
}

var _ orderport.WeChatShopRefundCallbackVerifier = (*WeChatShopCallbackVerifier)(nil)

func NewWeChatShopCallbackVerifier(credential *WeChatShopCallbackCredential) (*WeChatShopCallbackVerifier, error) {
	if credential == nil || !validShopCredentialPart(credential.appID, 128) || !validShopCredentialPart(credential.token, 256) || len(credential.key) != 32 {
		return nil, ErrInvalidProviderConfig
	}
	return &WeChatShopCallbackVerifier{credential: credential, now: time.Now}, nil
}

func (*WeChatShopCallbackVerifier) String() string { return "wechat-shop-callback-verifier[redacted]" }
func (*WeChatShopCallbackVerifier) GoString() string {
	return "wechat-shop-callback-verifier[redacted]"
}

func (verifier *WeChatShopCallbackVerifier) VerifyURL(ctx context.Context, query map[string]string) (string, error) {
	if !verifier.ready(ctx) {
		return "", ErrInvalidWeChatShopCallback
	}
	signature, timestamp, nonce, echo := query["signature"], query["timestamp"], query["nonce"], query["echostr"]
	if echo == "" || len(echo) > 64<<10 || !verifier.validTimestamp(timestamp) || !validSHA1Signature(signature) || !validCallbackPart(nonce, 128) || !wechatShopSHA1(signature, verifier.credential.token, timestamp, nonce) {
		return "", ErrInvalidWeChatShopCallback
	}
	return echo, nil
}

func (verifier *WeChatShopCallbackVerifier) VerifyRefund(ctx context.Context, body []byte, query map[string]string) (orderport.WeChatShopRefundCallbackCommand, error) {
	if !verifier.ready(ctx) || len(body) == 0 || len(body) > 128<<10 {
		return orderport.WeChatShopRefundCallbackCommand{}, ErrInvalidWeChatShopCallback
	}
	signature, timestamp, nonce := query["msg_signature"], query["timestamp"], query["nonce"]
	if !verifier.validTimestamp(timestamp) || !validSHA1Signature(signature) || !validCallbackPart(nonce, 128) {
		return orderport.WeChatShopRefundCallbackCommand{}, ErrInvalidWeChatShopCallback
	}
	var envelope struct {
		ToUserName string `json:"ToUserName"`
		Encrypt    string `json:"Encrypt"`
	}
	if !decodeJSONObject(body, &envelope) || !validCallbackPart(envelope.ToUserName, 128) || envelope.Encrypt == "" || len(envelope.Encrypt) > 128<<10 || !wechatShopSHA1(signature, verifier.credential.token, timestamp, nonce, envelope.Encrypt) {
		return orderport.WeChatShopRefundCallbackCommand{}, ErrInvalidWeChatShopCallback
	}
	plain, err := decryptWeChatShopCallback(envelope.Encrypt, verifier.credential.key, verifier.credential.appID)
	if err != nil || len(plain) == 0 || len(plain) > 64<<10 {
		return orderport.WeChatShopRefundCallbackCommand{}, ErrInvalidWeChatShopCallback
	}
	var message struct {
		CreateTime exactInteger `json:"CreateTime"`
		MsgType    string       `json:"MsgType"`
		Event      string       `json:"Event"`
		Update     struct {
			Status           string         `json:"status"`
			AfterSaleOrderID exactReference `json:"after_sale_order_id"`
			OrderID          exactReference `json:"order_id"`
		} `json:"finder_shop_aftersale_status_update"`
	}
	if !decodeJSONObject(plain, &message) || !message.CreateTime.set || message.CreateTime.value < 1 || message.MsgType != "event" || message.Event != weChatShopCallbackEvent || !message.Update.AfterSaleOrderID.set || !message.Update.OrderID.set || !validShopStatus(message.Update.Status) {
		return orderport.WeChatShopRefundCallbackCommand{}, ErrInvalidWeChatShopCallback
	}
	occurred := time.Unix(message.CreateTime.value, 0).UTC()
	if occurred.After(verifier.now().UTC().Add(5 * time.Minute)) {
		return orderport.WeChatShopRefundCallbackCommand{}, ErrInvalidWeChatShopCallback
	}
	payloadDigest := sha256.Sum256(body)
	return orderport.WeChatShopRefundCallbackCommand{
		AfterSaleID:         message.Update.AfterSaleOrderID.value,
		ProviderOrderID:     message.Update.OrderID.value,
		ProviderStatus:      message.Update.Status,
		ProviderEventDigest: providerDigest("wechat-shop/aftersale-callback-event/v1", message.Update.AfterSaleOrderID.value, message.Update.OrderID.value, message.Update.Status, strconv.FormatInt(message.CreateTime.value, 10), digestHex(sha256.Sum256(plain))),
		PayloadDigest:       payloadDigest,
		OccurredAt:          occurred,
	}, nil
}

func (verifier *WeChatShopCallbackVerifier) ready(ctx context.Context) bool {
	return verifier != nil && verifier.credential != nil && verifier.now != nil && ctx != nil && ctx.Err() == nil
}

func (verifier *WeChatShopCallbackVerifier) validTimestamp(value string) bool {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return false
	}
	delta := verifier.now().UTC().Sub(time.Unix(seconds, 0).UTC())
	return delta >= -5*time.Minute && delta <= 5*time.Minute
}

func wechatShopSHA1(signature string, parts ...string) bool {
	values := append([]string(nil), parts...)
	sort.Strings(values)
	digest := sha1.Sum([]byte(strings.Join(values, "")))
	expected := hex.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

func validSHA1Signature(value string) bool {
	if len(value) != sha1.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validCallbackPart(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}

func decryptWeChatShopCallback(encoded string, key []byte, appID string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 || len(key) != 32 {
		return nil, ErrInvalidWeChatShopCallback
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidWeChatShopCallback
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(plain, ciphertext)
	plain, err = unpadWeChatShopPKCS7(plain)
	if err != nil || len(plain) < 21 {
		return nil, ErrInvalidWeChatShopCallback
	}
	messageLength := int(binary.BigEndian.Uint32(plain[16:20]))
	messageEnd := 20 + messageLength
	if messageLength < 1 || messageEnd < 21 || messageEnd >= len(plain) || string(plain[messageEnd:]) != appID {
		return nil, ErrInvalidWeChatShopCallback
	}
	return append([]byte(nil), plain[20:messageEnd]...), nil
}

func unpadWeChatShopPKCS7(value []byte) ([]byte, error) {
	if len(value) == 0 || len(value)%aes.BlockSize != 0 {
		return nil, ErrInvalidWeChatShopCallback
	}
	padding := int(value[len(value)-1])
	if padding < 1 || padding > 32 || padding > len(value) {
		return nil, ErrInvalidWeChatShopCallback
	}
	for _, character := range value[len(value)-padding:] {
		if int(character) != padding {
			return nil, ErrInvalidWeChatShopCallback
		}
	}
	return value[:len(value)-padding], nil
}

func decodeWeChatShopAESKey(value string) ([]byte, error) {
	if len(value) != 43 {
		return nil, ErrInvalidProviderConfig
	}
	decoded, err := base64.StdEncoding.DecodeString(value + "=")
	if err != nil || len(decoded) != 32 {
		return nil, ErrInvalidProviderConfig
	}
	return decoded, nil
}
