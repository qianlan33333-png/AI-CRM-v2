package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

var ErrH5IdentityRequired = errors.New("survey h5 identity is required")

// H5OAuthProvider is deliberately narrower than the admin OAuth client. The
// provider result is trusted adapter output; HTTP request fields never become
// an identity hint or an identity binding.
type H5OAuthProvider interface {
	AuthorizationURL(string) (string, error)
	ExchangeExternalIdentity(context.Context, string) (H5ProviderIdentity, error)
}

// DisabledH5OAuthProvider is the production default until a provider adapter
// can prove an external contact identifier under a reviewed V2 contract.
// It intentionally cannot turn an employee userid into an external identity.
type DisabledH5OAuthProvider struct{}

func (DisabledH5OAuthProvider) AuthorizationURL(string) (string, error) {
	return "", ErrH5IdentityRequired
}

func (DisabledH5OAuthProvider) ExchangeExternalIdentity(context.Context, string) (H5ProviderIdentity, error) {
	return H5ProviderIdentity{}, ErrH5IdentityRequired
}

type H5ProviderIdentity struct {
	CorpID         string
	ExternalUserID string
}

type H5CanonicalIdentity struct {
	CustomerID int64
	ExpiresAt  time.Time
}

type H5IdentityResolver interface {
	Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error)
}

type H5OAuthService struct {
	states     authport.OAuthStateManager
	provider   H5OAuthProvider
	identities H5IdentityResolver
	clock      func() time.Time
}

func NewH5OAuthService(states authport.OAuthStateManager, provider H5OAuthProvider, identities H5IdentityResolver) (*H5OAuthService, error) {
	if nilH5Dependency(states) || nilH5Dependency(provider) || nilH5Dependency(identities) {
		return nil, ErrH5IdentityRequired
	}
	return &H5OAuthService{states: states, provider: provider, identities: identities, clock: time.Now}, nil
}

func (service *H5OAuthService) Start(ctx context.Context, next string) (string, time.Time, error) {
	if service == nil || service.clock == nil || !safeH5Next(next) {
		return "", time.Time{}, ErrH5IdentityRequired
	}
	attempt, err := service.states.Begin(ctx, authport.ProviderWeCom, next)
	if err != nil {
		return "", time.Time{}, classifyH5OAuth(err)
	}
	url, err := service.provider.AuthorizationURL(string(attempt.State))
	if err != nil || strings.TrimSpace(url) != url || url == "" {
		return "", time.Time{}, ErrH5IdentityRequired
	}
	return url, attempt.ExpiresAt.UTC(), nil
}

func (service *H5OAuthService) Callback(ctx context.Context, state, code string) (H5CanonicalIdentity, string, error) {
	if service == nil || !validH5Token(state) || !validH5Code(code) {
		return H5CanonicalIdentity{}, "", ErrH5IdentityRequired
	}
	claim, err := service.states.Claim(ctx, authport.ProviderWeCom, authport.OAuthState(state))
	if err != nil || claim.Provider != authport.ProviderWeCom || !safeH5Next(claim.NextPath) {
		return H5CanonicalIdentity{}, "", classifyH5OAuth(err)
	}
	trusted, err := service.provider.ExchangeExternalIdentity(ctx, code)
	if err != nil || !validProviderIdentity(trusted) {
		return H5CanonicalIdentity{}, "", ErrH5IdentityRequired
	}
	resolved, err := service.identities.Resolve(ctx, identityport.IDRef{
		Kind: identityport.KindWeComExternalUserID, Scope: trusted.CorpID,
		Value: trusted.ExternalUserID, Assurance: identityport.AssuranceVerified, Source: "wecom_h5_oauth",
	})
	if err != nil || resolved.Status != identityport.ResolveFound || resolved.CustomerID < 1 {
		return H5CanonicalIdentity{}, "", ErrH5IdentityRequired
	}
	now := service.clock().UTC()
	if now.IsZero() {
		return H5CanonicalIdentity{}, "", ErrH5IdentityRequired
	}
	return H5CanonicalIdentity{CustomerID: int64(resolved.CustomerID), ExpiresAt: now.Add(5 * time.Minute)}, claim.NextPath, nil
}

func safeH5Next(value string) bool {
	if len(value) < 1 || len(value) > 2048 || !strings.HasPrefix(value, "/s/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "\\#?") || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return ValidPublicSlug(strings.TrimPrefix(value, "/s/"))
}

func validProviderIdentity(value H5ProviderIdentity) bool {
	return validH5IdentityPart(value.CorpID, 128) && validH5IdentityPart(value.ExternalUserID, 256)
}

func validH5IdentityPart(value string, maximum int) bool {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validH5Token(value string) bool {
	return len(value) >= 32 && len(value) <= 128 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\\/?#")
}

func validH5Code(value string) bool { return validH5IdentityPart(value, 512) }

func classifyH5OAuth(err error) error {
	if errors.Is(err, authport.ErrOAuthStateUnavailable) {
		return ErrUnavailable
	}
	return ErrH5IdentityRequired
}

func nilH5Dependency(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	return ref.Kind() == reflect.Pointer && ref.IsNil()
}
