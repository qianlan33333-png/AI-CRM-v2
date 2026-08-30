package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

const oauthBindingVersion = 1

var (
	ErrOAuthAttemptInvalid = errors.New("sidebar oauth attempt invalid")
	ErrOAuthUnavailable    = errors.New("sidebar oauth unavailable")
)

// OAuthProvider is implemented by the composition root around the trusted
// WeCom adapter. The sidebar domain does not accept provider identity from an
// HTTP request.
type OAuthProvider interface {
	CorpID() string
	AuthorizationURL(string) (string, error)
	Exchange(context.Context, string) (OAuthIdentity, error)
}

type OAuthIdentity struct {
	CorpID string
	UserID string
}

type OAuthGrantOptions struct {
	Clock  func() time.Time
	Random io.Reader
}

// OAuthStart carries two browser-only values. State is still owned and claimed
// exactly once by auth; Binding is an encrypted sidebar cookie payload that
// binds that state to one corp and external contact.
type OAuthStart struct {
	AuthorizationURL string
	State            authport.OAuthState
	Binding          string
	ExpiresAt        time.Time
}

type OAuthCompletion struct {
	Session  authport.BrowserSession
	NextPath string
}

// OAuthGrantService closes the sidebar-specific security chain around the
// existing auth and identity primitives. It creates no customer or identity:
// an OAuth employee userid does not prove which OneID owns an external contact.
type OAuthGrantService struct {
	states   authport.OAuthStateManager
	provider OAuthProvider
	issuer   authport.Issuer
	auth     authport.Service
	contexts *Service
	binding  *oauthBindingCodec
	now      func() time.Time
	random   io.Reader
}

func NewOAuthGrantService(states authport.OAuthStateManager, provider OAuthProvider, issuer authport.Issuer, auth authport.Service, contexts *Service, rootKey []byte, options OAuthGrantOptions) (*OAuthGrantService, error) {
	if states == nil || provider == nil || issuer == nil || auth == nil || contexts == nil {
		return nil, ErrOAuthUnavailable
	}
	codec, err := newOAuthBindingCodec(rootKey)
	if err != nil {
		return nil, err
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	return &OAuthGrantService{states: states, provider: provider, issuer: issuer, auth: auth, contexts: contexts, binding: codec, now: options.Clock, random: options.Random}, nil
}

func (service *OAuthGrantService) Begin(ctx context.Context, externalUserID, nextPath string) (OAuthStart, error) {
	if service == nil || ctx == nil || !validExternalUserID(externalUserID) {
		return OAuthStart{}, ErrOAuthAttemptInvalid
	}
	now := service.now().UTC().Truncate(time.Second)
	corpID, err := service.currentCorp(ctx)
	if err != nil {
		return OAuthStart{}, err
	}
	attempt, err := service.states.Begin(ctx, authport.ProviderWeCom, nextPath)
	if err != nil {
		return OAuthStart{}, mapOAuthError(err)
	}
	if !attempt.ExpiresAt.After(now) || attempt.ExpiresAt.After(now.Add(15*time.Minute)) {
		return OAuthStart{}, ErrOAuthUnavailable
	}
	binding, err := service.binding.encode(oauthBindingClaims{
		Version: oauthBindingVersion, StateDigest: oauthStateDigest(attempt.State), CorpID: corpID,
		ExternalUserID: externalUserID, IssuedAt: now, ExpiresAt: attempt.ExpiresAt.UTC(),
	}, service.random)
	if err != nil {
		return OAuthStart{}, err
	}
	authorizationURL, err := service.provider.AuthorizationURL(string(attempt.State))
	if err != nil || !validAuthorizationURL(authorizationURL) {
		return OAuthStart{}, ErrOAuthUnavailable
	}
	return OAuthStart{AuthorizationURL: authorizationURL, State: attempt.State, Binding: binding, ExpiresAt: attempt.ExpiresAt.UTC()}, nil
}

func (service *OAuthGrantService) Complete(ctx context.Context, code string, state authport.OAuthState, binding string) (OAuthCompletion, error) {
	if service == nil || ctx == nil || !validOAuthCode(code) {
		return OAuthCompletion{}, ErrOAuthAttemptInvalid
	}
	claims, err := service.binding.decode(binding)
	if err != nil || !hmac.Equal([]byte(claims.StateDigest), []byte(oauthStateDigest(state))) {
		return OAuthCompletion{}, ErrOAuthAttemptInvalid
	}
	now := service.now().UTC()
	if now.Before(claims.IssuedAt.Add(-time.Minute)) || !now.Before(claims.ExpiresAt) {
		return OAuthCompletion{}, ErrOAuthAttemptInvalid
	}
	corpID, err := service.currentCorp(ctx)
	if err != nil {
		return OAuthCompletion{}, err
	}
	if claims.CorpID != corpID {
		return OAuthCompletion{}, ErrOAuthAttemptInvalid
	}
	claim, err := service.states.Claim(ctx, authport.ProviderWeCom, state)
	if err != nil {
		return OAuthCompletion{}, mapOAuthError(err)
	}
	identity, err := service.provider.Exchange(ctx, code)
	if err != nil {
		return OAuthCompletion{}, ErrOAuthUnavailable
	}
	if identity.CorpID != corpID || !validOAuthSubject(identity.UserID) {
		return OAuthCompletion{}, ErrOAuthAttemptInvalid
	}
	session, err := service.issuer.IssueVerified(ctx, authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: corpID, SubjectID: identity.UserID})
	if err != nil {
		return OAuthCompletion{}, mapOAuthError(err)
	}
	principal, err := service.auth.Authenticate(ctx, session.Session)
	if err != nil {
		return service.rejectIssuedSession(ctx, session, mapOAuthError(err))
	}
	_, err = service.auth.Authorize(ctx, principal, authport.CapabilityCustomersRead)
	if err != nil {
		return service.rejectIssuedSession(ctx, session, mapOAuthError(err))
	}
	return OAuthCompletion{Session: session, NextPath: claim.NextPath}, nil
}

func (service *OAuthGrantService) rejectIssuedSession(ctx context.Context, session authport.BrowserSession, cause error) (OAuthCompletion, error) {
	if err := service.auth.Invalidate(ctx, session.Session, session.CSRF); err != nil {
		return OAuthCompletion{}, errors.Join(ErrOAuthUnavailable, err)
	}
	return OAuthCompletion{}, cause
}

// RevokeCompletedSession closes the only remaining failure window after a
// successful provider exchange: the browser session could not be committed to
// its secure cookies by the HTTP adapter.
func (service *OAuthGrantService) RevokeCompletedSession(ctx context.Context, session authport.BrowserSession) error {
	if service == nil || ctx == nil {
		return ErrOAuthUnavailable
	}
	if err := service.auth.Invalidate(ctx, session.Session, session.CSRF); err != nil {
		return errors.Join(ErrOAuthUnavailable, err)
	}
	return nil
}

func (service *OAuthGrantService) currentCorp(ctx context.Context) (string, error) {
	corpID, err := service.contexts.corp.CorpID(ctx)
	if err != nil || !validOAuthSubject(corpID) || service.provider.CorpID() != corpID {
		return "", ErrOAuthUnavailable
	}
	return corpID, nil
}

type oauthBindingClaims struct {
	Version        int       `json:"v"`
	StateDigest    string    `json:"state_digest"`
	CorpID         string    `json:"corp_id"`
	ExternalUserID string    `json:"external_userid"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type oauthBindingCodec struct{ aead cipher.AEAD }

func newOAuthBindingCodec(root []byte) (*oauthBindingCodec, error) {
	if len(root) < 32 {
		return nil, ErrOAuthUnavailable
	}
	key := hmacSHA256(root, []byte("aicrm.sidebar.oauth-binding.v1"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, ErrOAuthUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrOAuthUnavailable
	}
	return &oauthBindingCodec{aead: aead}, nil
}

func (codec *oauthBindingCodec) encode(claims oauthBindingClaims, random io.Reader) (string, error) {
	if codec == nil || codec.aead == nil || random == nil || !validOAuthBindingClaims(claims) {
		return "", ErrOAuthUnavailable
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", ErrOAuthUnavailable
	}
	nonce := make([]byte, codec.aead.NonceSize())
	if _, err = io.ReadFull(random, nonce); err != nil {
		return "", ErrOAuthUnavailable
	}
	sealed := codec.aead.Seal(nonce, nonce, payload, []byte("aicrm.sidebar.oauth-binding.v1"))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (codec *oauthBindingCodec) decode(value string) (oauthBindingClaims, error) {
	if codec == nil || codec.aead == nil || value == "" || len(value) > 4096 || strings.TrimSpace(value) != value {
		return oauthBindingClaims{}, ErrOAuthAttemptInvalid
	}
	sealed, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(sealed) <= codec.aead.NonceSize() {
		return oauthBindingClaims{}, ErrOAuthAttemptInvalid
	}
	nonce, ciphertext := sealed[:codec.aead.NonceSize()], sealed[codec.aead.NonceSize():]
	payload, err := codec.aead.Open(nil, nonce, ciphertext, []byte("aicrm.sidebar.oauth-binding.v1"))
	if err != nil {
		return oauthBindingClaims{}, ErrOAuthAttemptInvalid
	}
	var claims oauthBindingClaims
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&claims) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) || !validOAuthBindingClaims(claims) {
		return oauthBindingClaims{}, ErrOAuthAttemptInvalid
	}
	return claims, nil
}

func validOAuthBindingClaims(claims oauthBindingClaims) bool {
	return claims.Version == oauthBindingVersion && validOAuthStateDigest(claims.StateDigest) && validOAuthSubject(claims.CorpID) &&
		validExternalUserID(claims.ExternalUserID) && !claims.IssuedAt.IsZero() && claims.ExpiresAt.After(claims.IssuedAt) &&
		!claims.ExpiresAt.After(claims.IssuedAt.Add(15*time.Minute))
}

func oauthStateDigest(state authport.OAuthState) string {
	digest := sha256.Sum256([]byte(state))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func validOAuthStateDigest(value string) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func validOAuthSubject(value string) bool {
	if len(value) < 1 || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("_-.:@", character)) {
			return false
		}
	}
	return true
}

func validOAuthCode(value string) bool {
	if len(value) < 1 || len(value) > 512 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validAuthorizationURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func mapOAuthError(err error) error {
	if errors.Is(err, authport.ErrOAuthStateInvalid) || errors.Is(err, authport.ErrInvalidVerifiedLogin) || errors.Is(err, authport.ErrUnauthenticated) {
		return ErrOAuthAttemptInvalid
	}
	if errors.Is(err, authport.ErrUnauthorized) {
		return ErrForbidden
	}
	return ErrOAuthUnavailable
}
