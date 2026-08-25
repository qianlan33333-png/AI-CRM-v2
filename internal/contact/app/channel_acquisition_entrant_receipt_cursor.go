package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"io"
	"strings"
	"time"
)

const (
	channelAcquisitionEntrantReceiptCursorPrefix        = "care1."
	channelAcquisitionEntrantReceiptMaximumCursorLength = 256
	channelAcquisitionEntrantReceiptCursorPayloadSize   = 33
)

type ChannelAcquisitionEntrantReceiptCursorCodec struct {
	aead cipher.AEAD
	now  func() time.Time
}

func NewChannelAcquisitionEntrantReceiptCursorCodec(secret []byte) (*ChannelAcquisitionEntrantReceiptCursorCodec, error) {
	if len(secret) < 32 {
		return nil, ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	key := sha256.Sum256(append([]byte("AI-CRM-v2/contact/acquisition-entrant-receipt/cursor/v1\x00"), secret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &ChannelAcquisitionEntrantReceiptCursorCodec{aead: aead, now: time.Now}, nil
}

func (codec *ChannelAcquisitionEntrantReceiptCursorCodec) Encode(actorID, channelID, receiptID int64) (string, error) {
	if !channelAcquisitionEntrantReceiptCursorReady(codec) || actorID < 1 || channelID < 1 || receiptID < 1 {
		return "", ErrInvalidChannelAcquisitionEntrantReceipt
	}
	payload := make([]byte, channelAcquisitionEntrantReceiptCursorPayloadSize)
	payload[0] = 1
	binary.BigEndian.PutUint64(payload[1:9], uint64(actorID))
	binary.BigEndian.PutUint64(payload[9:17], uint64(channelID))
	binary.BigEndian.PutUint64(payload[17:25], uint64(receiptID))
	binary.BigEndian.PutUint64(payload[25:33], uint64(codec.now().UTC().Add(15*time.Minute).Unix()))
	nonce := make([]byte, codec.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	raw := append(nonce, codec.aead.Seal(nil, nonce, payload, []byte(channelAcquisitionEntrantReceiptCursorPrefix))...)
	return channelAcquisitionEntrantReceiptCursorPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (codec *ChannelAcquisitionEntrantReceiptCursorCodec) Decode(token string, actorID, channelID int64) (int64, error) {
	if !channelAcquisitionEntrantReceiptCursorReady(codec) || actorID < 1 || channelID < 1 || len(token) > channelAcquisitionEntrantReceiptMaximumCursorLength || !strings.HasPrefix(token, channelAcquisitionEntrantReceiptCursorPrefix) {
		return 0, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(token, channelAcquisitionEntrantReceiptCursorPrefix))
	if err != nil || len(raw) != codec.aead.NonceSize()+channelAcquisitionEntrantReceiptCursorPayloadSize+codec.aead.Overhead() {
		return 0, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	payload, err := codec.aead.Open(nil, raw[:codec.aead.NonceSize()], raw[codec.aead.NonceSize():], []byte(channelAcquisitionEntrantReceiptCursorPrefix))
	if err != nil || len(payload) != channelAcquisitionEntrantReceiptCursorPayloadSize || payload[0] != 1 {
		return 0, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	decodedActor := int64(binary.BigEndian.Uint64(payload[1:9]))
	decodedChannel := int64(binary.BigEndian.Uint64(payload[9:17]))
	receiptID := int64(binary.BigEndian.Uint64(payload[17:25]))
	expiry := int64(binary.BigEndian.Uint64(payload[25:33]))
	if decodedActor != actorID || decodedChannel != channelID || receiptID < 1 || !time.Unix(expiry, 0).After(codec.now().UTC()) {
		return 0, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	return receiptID, nil
}

func channelAcquisitionEntrantReceiptCursorReady(codec *ChannelAcquisitionEntrantReceiptCursorCodec) bool {
	return codec != nil && codec.aead != nil && codec.now != nil
}
