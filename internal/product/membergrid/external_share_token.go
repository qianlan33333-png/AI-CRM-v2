package membergrid

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
)

var externalShareTokenKeyDomain = []byte("AI-CRM-v2/membergrid/public-share-token/v1\x00")

// ExternalShareTokenCodec signs the opaque share ID with a domain-separated
// deployment secret. It has no persistence and does not make an identifier a
// valid share without ExternalShareService's live store lookup.
type ExternalShareTokenCodec struct{ key [sha256.Size]byte }

func NewExternalShareTokenCodec(secret []byte) (*ExternalShareTokenCodec, error) {
	if len(secret) < minimumCursorKey {
		return nil, errors.New("member grid external share token secret must contain at least 32 bytes")
	}
	material := make([]byte, 0, len(externalShareTokenKeyDomain)+len(secret))
	material = append(material, externalShareTokenKeyDomain...)
	material = append(material, secret...)
	return &ExternalShareTokenCodec{key: sha256.Sum256(material)}, nil
}

func (codec *ExternalShareTokenCodec) Issue(shareID string) (string, error) {
	if codec == nil || !validExternalShareID(shareID) {
		return "", ErrInvalidExternalShareInput
	}
	return externalShareTokenPrefix + "." + shareID + "." + base64.RawURLEncoding.EncodeToString(codec.signature(shareID)), nil
}

func (codec *ExternalShareTokenCodec) Verify(token string) (string, error) {
	if codec == nil || len(token) > 512 {
		return "", ErrInvalidExternalShareToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != externalShareTokenPrefix || !validExternalShareID(parts[1]) {
		return "", ErrInvalidExternalShareToken
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || len(signature) != sha256.Size || subtle.ConstantTimeCompare(signature, codec.signature(parts[1])) != 1 {
		return "", ErrInvalidExternalShareToken
	}
	return parts[1], nil
}

func (codec *ExternalShareTokenCodec) signature(shareID string) []byte {
	mac := hmac.New(sha256.New, codec.key[:])
	_, _ = mac.Write([]byte(externalShareTokenPrefix + "\x00" + shareID))
	return mac.Sum(nil)
}
