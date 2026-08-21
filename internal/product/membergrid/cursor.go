package membergrid

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
	cursorPrefix           = "mg1."
	cursorPayloadSize      = 26
	minimumCursorKey       = 32
	cursorVersion     byte = 1
)

var cursorKeyDomain = []byte("AI-CRM-v2/membergrid/cursor/v1\x00")

type CursorCodec struct {
	aead   cipher.AEAD
	random io.Reader
}

func NewCursorCodec(secret []byte) (*CursorCodec, error) {
	return newCursorCodec(secret, rand.Reader)
}

func newCursorCodec(secret []byte, random io.Reader) (*CursorCodec, error) {
	if len(secret) < minimumCursorKey || random == nil {
		return nil, errors.New("member grid cursor secret must contain at least 32 bytes")
	}
	material := make([]byte, 0, len(cursorKeyDomain)+len(secret))
	material = append(material, cursorKeyDomain...)
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
	return &CursorCodec{aead: aead, random: random}, nil
}

func (codec *CursorCodec) Encode(productID int64, state StateFilter, position Position) (string, error) {
	if codec == nil || codec.aead == nil || codec.random == nil || productID < 1 ||
		!state.valid() || position.EntitlementID < 1 || position.GrantedAt.IsZero() {
		return "", ErrInvalidCursor
	}
	payload := make([]byte, cursorPayloadSize)
	payload[0] = cursorVersion
	payload[1] = encodeCursorState(state)
	binary.BigEndian.PutUint64(payload[2:10], uint64(productID))
	binary.BigEndian.PutUint64(payload[10:18], uint64(position.GrantedAt.UTC().UnixMicro()))
	binary.BigEndian.PutUint64(payload[18:26], uint64(position.EntitlementID))

	nonce := make([]byte, codec.aead.NonceSize())
	if _, err := io.ReadFull(codec.random, nonce); err != nil {
		return "", errors.Join(ErrUnavailable, err)
	}
	sealed := codec.aead.Seal(nil, nonce, payload, []byte(cursorPrefix))
	token := append(nonce, sealed...)
	return cursorPrefix + base64.RawURLEncoding.EncodeToString(token), nil
}

func (codec *CursorCodec) Decode(token string, productID int64, state StateFilter) (Position, error) {
	if codec == nil || codec.aead == nil || productID < 1 || !state.valid() ||
		len(token) <= len(cursorPrefix) || len(token) > 256 || token[:len(cursorPrefix)] != cursorPrefix {
		return Position{}, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token[len(cursorPrefix):])
	if err != nil || len(raw) != codec.aead.NonceSize()+cursorPayloadSize+codec.aead.Overhead() {
		return Position{}, ErrInvalidCursor
	}
	nonce := raw[:codec.aead.NonceSize()]
	payload, err := codec.aead.Open(nil, nonce, raw[codec.aead.NonceSize():], []byte(cursorPrefix))
	if err != nil || len(payload) != cursorPayloadSize || payload[0] != cursorVersion {
		return Position{}, ErrInvalidCursor
	}
	decodedState, ok := decodeCursorState(payload[1])
	decodedProductID := int64(binary.BigEndian.Uint64(payload[2:10]))
	grantedAtMicros := int64(binary.BigEndian.Uint64(payload[10:18]))
	entitlementID := int64(binary.BigEndian.Uint64(payload[18:26]))
	if !ok || decodedState != state || decodedProductID != productID || decodedProductID < 1 ||
		entitlementID < 1 {
		return Position{}, ErrInvalidCursor
	}
	grantedAt := time.UnixMicro(grantedAtMicros).UTC()
	if grantedAt.IsZero() {
		return Position{}, ErrInvalidCursor
	}
	return Position{GrantedAt: grantedAt, EntitlementID: entitlementID}, nil
}

func encodeCursorState(state StateFilter) byte {
	switch state {
	case StateActive:
		return 1
	case StateRevoked:
		return 2
	case StateAll:
		return 3
	default:
		return 0
	}
}

func decodeCursorState(value byte) (StateFilter, bool) {
	switch value {
	case 1:
		return StateActive, true
	case 2:
		return StateRevoked, true
	case 3:
		return StateAll, true
	default:
		return "", false
	}
}
