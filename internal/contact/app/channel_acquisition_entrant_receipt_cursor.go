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
	channelAcquisitionEntrantReceiptUnassignedPrefix    = "careu1."
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
	return codec.encode(actorID, channelID, receiptID, channelAcquisitionEntrantReceiptCursorPrefix, 1)
}

func (codec *ChannelAcquisitionEntrantReceiptCursorCodec) EncodeUnassigned(actorID, receiptID int64) (string, error) {
	if !channelAcquisitionEntrantReceiptCursorReady(codec) || actorID < 1 || receiptID < 1 {
		return "", ErrInvalidChannelAcquisitionEntrantReceipt
	}
	return codec.encode(actorID, 0, receiptID, channelAcquisitionEntrantReceiptUnassignedPrefix, 2)
}

func (codec *ChannelAcquisitionEntrantReceiptCursorCodec) encode(actorID, channelID, receiptID int64, prefix string, version byte) (string, error) {
	payload := make([]byte, channelAcquisitionEntrantReceiptCursorPayloadSize)
	payload[0] = version
	binary.BigEndian.PutUint64(payload[1:9], uint64(actorID))
	binary.BigEndian.PutUint64(payload[9:17], uint64(channelID))
	binary.BigEndian.PutUint64(payload[17:25], uint64(receiptID))
	binary.BigEndian.PutUint64(payload[25:33], uint64(codec.now().UTC().Add(15*time.Minute).Unix()))
	nonce := make([]byte, codec.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	raw := append(nonce, codec.aead.Seal(nil, nonce, payload, []byte(prefix))...)
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (codec *ChannelAcquisitionEntrantReceiptCursorCodec) Decode(token string, actorID, channelID int64) (int64, error) {
	if !channelAcquisitionEntrantReceiptCursorReady(codec) || actorID < 1 || channelID < 1 || len(token) > channelAcquisitionEntrantReceiptMaximumCursorLength || !strings.HasPrefix(token, channelAcquisitionEntrantReceiptCursorPrefix) {
		return 0, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	return codec.decode(token, actorID, channelID, channelAcquisitionEntrantReceiptCursorPrefix, 1)
}

func (codec *ChannelAcquisitionEntrantReceiptCursorCodec) DecodeUnassigned(token string, actorID int64) (int64, error) {
	if !channelAcquisitionEntrantReceiptCursorReady(codec) || actorID < 1 || len(token) > channelAcquisitionEntrantReceiptMaximumCursorLength || !strings.HasPrefix(token, channelAcquisitionEntrantReceiptUnassignedPrefix) {
		return 0, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	return codec.decode(token, actorID, 0, channelAcquisitionEntrantReceiptUnassignedPrefix, 2)
}

func (codec *ChannelAcquisitionEntrantReceiptCursorCodec) decode(token string, actorID, channelID int64, prefix string, version byte) (int64, error) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(token, prefix))
	if err != nil || len(raw) != codec.aead.NonceSize()+channelAcquisitionEntrantReceiptCursorPayloadSize+codec.aead.Overhead() {
		return 0, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	payload, err := codec.aead.Open(nil, raw[:codec.aead.NonceSize()], raw[codec.aead.NonceSize():], []byte(prefix))
	if err != nil || len(payload) != channelAcquisitionEntrantReceiptCursorPayloadSize || payload[0] != version {
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
