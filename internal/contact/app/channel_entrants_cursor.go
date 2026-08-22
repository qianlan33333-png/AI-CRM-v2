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
	"time"
)

const (
	channelEntrantsCursorPrefix             = "ce1."
	channelEntrantsCursorVersion       byte = 1
	channelEntrantsCursorPayloadSize        = 33
	channelEntrantsMinimumCursorKey         = 32
	channelEntrantsMaximumCursorLength      = 256
	channelEntrantsDefaultCursorTTL         = 15 * time.Minute
)

var channelEntrantsCursorKeyDomain = []byte("AI-CRM-v2/contact/channel-entrants/cursor/v1\x00")

type ChannelEntrantsCursorCodec struct {
	aead   cipher.AEAD
	random io.Reader
	now    func() time.Time
	ttl    time.Duration
}

func NewChannelEntrantsCursorCodec(secret []byte) (*ChannelEntrantsCursorCodec, error) {
	return newChannelEntrantsCursorCodec(
		secret,
		rand.Reader,
		time.Now,
		channelEntrantsDefaultCursorTTL,
	)
}

func newChannelEntrantsCursorCodec(
	secret []byte,
	random io.Reader,
	now func() time.Time,
	ttl time.Duration,
) (*ChannelEntrantsCursorCodec, error) {
	if len(secret) < channelEntrantsMinimumCursorKey || random == nil || now == nil || ttl <= 0 {
		return nil, errors.New("channel entrants cursor dependencies are invalid")
	}
	material := make([]byte, 0, len(channelEntrantsCursorKeyDomain)+len(secret))
	material = append(material, channelEntrantsCursorKeyDomain...)
	material = append(material, secret...)
	key := sha256.Sum256(material)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &ChannelEntrantsCursorCodec{aead: aead, random: random, now: now, ttl: ttl}, nil
}

func (codec *ChannelEntrantsCursorCodec) Encode(
	channelID int64,
	position ChannelEntrantsPosition,
) (string, error) {
	if !channelEntrantsCursorCodecReady(codec) || channelID < 1 ||
		position.CustomerID < 1 || position.AddedAt.IsZero() {
		return "", ErrInvalidChannelEntrantsCursor
	}
	now := codec.now().UTC()
	if now.IsZero() {
		return "", ErrChannelEntrantsUnavailable
	}
	expiresAt := now.Add(codec.ttl)
	if !expiresAt.After(now) {
		return "", ErrChannelEntrantsUnavailable
	}

	payload := make([]byte, channelEntrantsCursorPayloadSize)
	payload[0] = channelEntrantsCursorVersion
	binary.BigEndian.PutUint64(payload[1:9], uint64(channelID))
	binary.BigEndian.PutUint64(payload[9:17], uint64(position.AddedAt.UTC().UnixMicro()))
	binary.BigEndian.PutUint64(payload[17:25], uint64(position.CustomerID))
	binary.BigEndian.PutUint64(payload[25:33], uint64(expiresAt.Unix()))

	nonce := make([]byte, codec.aead.NonceSize())
	if _, err := io.ReadFull(codec.random, nonce); err != nil {
		return "", errors.Join(ErrChannelEntrantsUnavailable, err)
	}
	sealed := codec.aead.Seal(nil, nonce, payload, []byte(channelEntrantsCursorPrefix))
	token := append(nonce, sealed...)
	return channelEntrantsCursorPrefix + base64.RawURLEncoding.EncodeToString(token), nil
}

func (codec *ChannelEntrantsCursorCodec) Decode(
	token string,
	channelID int64,
) (ChannelEntrantsPosition, error) {
	if !channelEntrantsCursorCodecReady(codec) || channelID < 1 ||
		len(token) <= len(channelEntrantsCursorPrefix) ||
		len(token) > channelEntrantsMaximumCursorLength ||
		token[:len(channelEntrantsCursorPrefix)] != channelEntrantsCursorPrefix {
		return ChannelEntrantsPosition{}, ErrInvalidChannelEntrantsCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token[len(channelEntrantsCursorPrefix):])
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != token[len(channelEntrantsCursorPrefix):] ||
		len(raw) != codec.aead.NonceSize()+channelEntrantsCursorPayloadSize+codec.aead.Overhead() {
		return ChannelEntrantsPosition{}, ErrInvalidChannelEntrantsCursor
	}
	nonce := raw[:codec.aead.NonceSize()]
	payload, err := codec.aead.Open(
		nil,
		nonce,
		raw[codec.aead.NonceSize():],
		[]byte(channelEntrantsCursorPrefix),
	)
	if err != nil || len(payload) != channelEntrantsCursorPayloadSize ||
		payload[0] != channelEntrantsCursorVersion {
		return ChannelEntrantsPosition{}, ErrInvalidChannelEntrantsCursor
	}

	decodedChannelID := int64(binary.BigEndian.Uint64(payload[1:9]))
	addedAtMicros := int64(binary.BigEndian.Uint64(payload[9:17]))
	customerID := int64(binary.BigEndian.Uint64(payload[17:25]))
	expiresAtSeconds := int64(binary.BigEndian.Uint64(payload[25:33]))
	if decodedChannelID != channelID || decodedChannelID < 1 || customerID < 1 || expiresAtSeconds <= 0 {
		return ChannelEntrantsPosition{}, ErrInvalidChannelEntrantsCursor
	}
	addedAt := time.UnixMicro(addedAtMicros).UTC()
	if addedAt.IsZero() {
		return ChannelEntrantsPosition{}, ErrInvalidChannelEntrantsCursor
	}
	now := codec.now().UTC()
	if now.IsZero() {
		return ChannelEntrantsPosition{}, ErrChannelEntrantsUnavailable
	}
	if !time.Unix(expiresAtSeconds, 0).UTC().After(now) {
		return ChannelEntrantsPosition{}, ErrInvalidChannelEntrantsCursor
	}
	return ChannelEntrantsPosition{AddedAt: addedAt, CustomerID: customerID}, nil
}

func channelEntrantsCursorCodecReady(codec *ChannelEntrantsCursorCodec) bool {
	return codec != nil && codec.aead != nil && codec.random != nil && codec.now != nil && codec.ttl > 0
}
