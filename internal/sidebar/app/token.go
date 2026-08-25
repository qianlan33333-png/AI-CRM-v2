package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

const tokenVersion = 2

type tokenClaims struct {
	Version            int           `json:"v"`
	CorpID             string        `json:"corp_id"`
	CustomerID         int64         `json:"customer_id"`
	OwnerStaffID       int64         `json:"owner_staff_id"`
	AdminUserID        int64         `json:"admin_user_id"`
	Role               authport.Role `json:"role"`
	SessionFingerprint string        `json:"session_fingerprint"`
	IssuedAt           time.Time     `json:"issued_at"`
	ExpiresAt          time.Time     `json:"expires_at"`
}

type tokenCodec struct {
	key        [32]byte
	sessionKey [32]byte
}

func newTokenCodec(root []byte) (*tokenCodec, error) {
	if len(root) < 32 {
		return nil, ErrUnavailable
	}
	return &tokenCodec{
		key:        hmacSHA256(root, []byte("aicrm.sidebar.context-token.v2")),
		sessionKey: hmacSHA256(root, []byte("aicrm.sidebar.context-token.session.v1")),
	}, nil
}

func (codec *tokenCodec) encode(claims tokenClaims) (string, error) {
	if codec == nil || !validClaims(claims) {
		return "", ErrInvalidInput
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", ErrUnavailable
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := hmacSHA256(codec.key[:], []byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature[:]), nil
}

func (codec *tokenCodec) decode(value string) (tokenClaims, error) {
	if codec == nil || len(value) > 4096 || strings.TrimSpace(value) != value {
		return tokenClaims{}, ErrTokenInvalid
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return tokenClaims{}, ErrTokenInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	want := hmacSHA256(codec.key[:], []byte(parts[0]))
	if err != nil || !hmac.Equal(signature, want[:]) {
		return tokenClaims{}, ErrTokenInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenClaims{}, ErrTokenInvalid
	}
	var claims tokenClaims
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&claims) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) || !validClaims(claims) {
		return tokenClaims{}, ErrTokenInvalid
	}
	return claims, nil
}

func validClaims(claims tokenClaims) bool {
	return claims.Version == tokenVersion && claims.CorpID != "" && claims.CustomerID > 0 && claims.OwnerStaffID > 0 &&
		claims.AdminUserID > 0 && (claims.Role == authport.RoleAdmin || claims.Role == authport.RoleOps || claims.Role == authport.RoleSales) &&
		validSessionFingerprint(claims.SessionFingerprint) && !claims.IssuedAt.IsZero() && claims.ExpiresAt.After(claims.IssuedAt)
}

func (codec *tokenCodec) sessionFingerprint(session authport.SessionRef) (string, error) {
	value := string(session)
	if codec == nil || value == "" || len(value) > 4096 || strings.TrimSpace(value) != value {
		return "", ErrTokenInvalid
	}
	digest := hmacSHA256(codec.sessionKey[:], []byte(value))
	return base64.RawURLEncoding.EncodeToString(digest[:16]), nil
}

func validSessionFingerprint(value string) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) == 16 && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func hmacSHA256(key, value []byte) [32]byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(value)
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func tokenError(err error) error {
	if errors.Is(err, ErrTokenExpired) || errors.Is(err, ErrTokenInvalid) {
		return err
	}
	return ErrUnavailable
}
