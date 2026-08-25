package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	channelAcquisitionAssetCursorPrefix             = "caa1."
	channelAcquisitionAssetCursorVersion       byte = 1
	channelAcquisitionAssetCursorPayloadSize        = 25
	channelAcquisitionAssetMaximumCursorLength      = 256
	channelAcquisitionAssetDefaultCursorTTL         = 15 * time.Minute
)

type ChannelAcquisitionAssetCursorCodec struct {
	aead cipher.AEAD
	now  func() time.Time
}

func NewChannelAcquisitionAssetCursorCodec(secret []byte) (*ChannelAcquisitionAssetCursorCodec, error) {
	if len(secret) < 32 {
		return nil, ErrChannelAcquisitionAssetUnavailable
	}
	key := sha256.Sum256(append([]byte("AI-CRM-v2/contact/channel-acquisition-assets/cursor/v1\x00"), secret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &ChannelAcquisitionAssetCursorCodec{aead: aead, now: time.Now}, nil
}

func (codec *ChannelAcquisitionAssetCursorCodec) Encode(channelID int64, effectID string) (string, error) {
	id, err := channelAcquisitionAssetNumericEffectID(effectID)
	if !channelAcquisitionAssetCursorReady(codec) || channelID < 1 || err != nil {
		return "", ErrInvalidChannelAcquisitionAsset
	}
	payload := make([]byte, channelAcquisitionAssetCursorPayloadSize)
	payload[0] = channelAcquisitionAssetCursorVersion
	binary.BigEndian.PutUint64(payload[1:9], uint64(channelID))
	binary.BigEndian.PutUint64(payload[9:17], uint64(id))
	binary.BigEndian.PutUint64(payload[17:25], uint64(codec.now().UTC().Add(channelAcquisitionAssetDefaultCursorTTL).Unix()))
	nonce := make([]byte, codec.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	sealed := codec.aead.Seal(nil, nonce, payload, []byte(channelAcquisitionAssetCursorPrefix))
	return channelAcquisitionAssetCursorPrefix + base64.RawURLEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func (codec *ChannelAcquisitionAssetCursorCodec) Decode(token string, channelID int64) (int64, error) {
	if !channelAcquisitionAssetCursorReady(codec) || channelID < 1 || len(token) <= len(channelAcquisitionAssetCursorPrefix) || len(token) > channelAcquisitionAssetMaximumCursorLength || !strings.HasPrefix(token, channelAcquisitionAssetCursorPrefix) {
		return 0, ErrInvalidChannelAcquisitionAsset
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(token, channelAcquisitionAssetCursorPrefix))
	if err != nil || len(raw) != codec.aead.NonceSize()+channelAcquisitionAssetCursorPayloadSize+codec.aead.Overhead() {
		return 0, ErrInvalidChannelAcquisitionAsset
	}
	payload, err := codec.aead.Open(nil, raw[:codec.aead.NonceSize()], raw[codec.aead.NonceSize():], []byte(channelAcquisitionAssetCursorPrefix))
	if err != nil || len(payload) != channelAcquisitionAssetCursorPayloadSize || payload[0] != channelAcquisitionAssetCursorVersion || int64(binary.BigEndian.Uint64(payload[1:9])) != channelID {
		return 0, ErrInvalidChannelAcquisitionAsset
	}
	id := int64(binary.BigEndian.Uint64(payload[9:17]))
	expires := int64(binary.BigEndian.Uint64(payload[17:25]))
	if id < 1 || !time.Unix(expires, 0).UTC().After(codec.now().UTC()) {
		return 0, ErrInvalidChannelAcquisitionAsset
	}
	return id, nil
}

func channelAcquisitionAssetNumericEffectID(value string) (int64, error) {
	if !strings.HasPrefix(value, "eer_") {
		return 0, ErrInvalidChannelAcquisitionAsset
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "eer_"), 10, 64)
	if err != nil || id < 1 {
		return 0, ErrInvalidChannelAcquisitionAsset
	}
	return id, nil
}

func channelAcquisitionAssetCursorReady(codec *ChannelAcquisitionAssetCursorCodec) bool {
	return codec != nil && codec.aead != nil && codec.now != nil
}
