package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestOAuthGrantClosesStateProviderIdentityAndEmployeeSessionOnly(t *testing.T) {
	service, _, states, provider, auth, resolver, now := oauthGrantFixture(t)
	start, err := service.Begin(context.Background(), "wm_external_41", "/sidebar/bind-mobile?tab=profile")
	if err != nil {
		t.Fatal(err)
	}
	if start.AuthorizationURL != "https://open.weixin.test/authorize?state="+string(start.State) || start.Binding == "" || !start.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("Begin() = %+v", start)
	}
	for _, secret := range []string{"wm_external_41", "corp-1", string(start.State)} {
		if strings.Contains(start.Binding, secret) {
			t.Fatalf("encrypted binding exposed %q", secret)
		}
	}

	completed, err := service.Complete(context.Background(), "one-time-code", start.State, start.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if completed.NextPath != "/sidebar/bind-mobile?tab=profile" || completed.Session.Session != auth.session.Session {
		t.Fatalf("Complete() = %+v", completed)
	}
	if states.claimCalls != 1 || provider.exchangeCalls != 1 || auth.issueCalls != 1 || auth.authenticateCalls != 1 || auth.authorizeCalls != 1 {
		t.Fatalf("state/provider/issue/auth/authz calls = %d/%d/%d/%d/%d", states.claimCalls, provider.exchangeCalls, auth.issueCalls, auth.authenticateCalls, auth.authorizeCalls)
	}
	if resolver.calls != 0 {
		t.Fatalf("OAuth callback resolved customer context %d times, want 0", resolver.calls)
	}

	if _, err = service.Complete(context.Background(), "one-time-code", start.State, start.Binding); !errors.Is(err, ErrOAuthAttemptInvalid) {
		t.Fatalf("OAuth replay error = %v", err)
	}
	if states.claimCalls != 2 || provider.exchangeCalls != 1 || auth.issueCalls != 1 {
		t.Fatalf("replay calls state/provider/issue = %d/%d/%d", states.claimCalls, provider.exchangeCalls, auth.issueCalls)
	}
}

func TestOAuthGrantDoesNotResolveCustomerDuringCallback(t *testing.T) {
	service, _, states, provider, auth, resolver, _ := oauthGrantFixture(t)
	start, err := service.Begin(context.Background(), "wm_external_41", "/sidebar/bind-mobile")
	if err != nil {
		t.Fatal(err)
	}

	wrongState := authport.OAuthState(oauthTestToken(9))
	if _, err = service.Complete(context.Background(), "code", wrongState, start.Binding); !errors.Is(err, ErrOAuthAttemptInvalid) {
		t.Fatalf("cross-state binding error = %v", err)
	}
	if states.claimCalls != 0 || provider.exchangeCalls != 0 || auth.issueCalls != 0 || resolver.calls != 0 {
		t.Fatalf("cross-state calls = %d/%d/%d/%d", states.claimCalls, provider.exchangeCalls, auth.issueCalls, resolver.calls)
	}
	tampered := start.Binding[:len(start.Binding)-1] + "A"
	if _, err = service.Complete(context.Background(), "code", start.State, tampered); !errors.Is(err, ErrOAuthAttemptInvalid) {
		t.Fatalf("tampered binding error = %v", err)
	}
	if states.claimCalls != 0 || provider.exchangeCalls != 0 {
		t.Fatalf("tampered binding reached state/provider = %d/%d", states.claimCalls, provider.exchangeCalls)
	}

	resolver.err = ErrUnavailable
	completed, err := service.Complete(context.Background(), "code", start.State, start.Binding)
	if err != nil || completed.Session.Session == "" {
		t.Fatalf("unbound Complete() = %+v err=%v", completed, err)
	}
	if resolver.calls != 0 {
		t.Fatalf("identity resolve calls = %d, want 0", resolver.calls)
	}
}

func TestOAuthGrantRejectsExpiredBindingAndWrongProviderCorpBeforeUnsafeProgress(t *testing.T) {
	service, _, states, provider, auth, _, now := oauthGrantFixture(t)
	start, err := service.Begin(context.Background(), "wm_external_41", "/sidebar/bind-mobile")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now.Add(5 * time.Minute) }
	if _, err = service.Complete(context.Background(), "code", start.State, start.Binding); !errors.Is(err, ErrOAuthAttemptInvalid) {
		t.Fatalf("expired binding error = %v", err)
	}
	if states.claimCalls != 0 || provider.exchangeCalls != 0 || auth.issueCalls != 0 {
		t.Fatalf("expired calls = %d/%d/%d", states.claimCalls, provider.exchangeCalls, auth.issueCalls)
	}

	service, _, states, provider, auth, _, _ = oauthGrantFixture(t)
	start, err = service.Begin(context.Background(), "wm_external_41", "/sidebar/bind-mobile")
	if err != nil {
		t.Fatal(err)
	}
	provider.identity.CorpID = "other-corp"
	if _, err = service.Complete(context.Background(), "code", start.State, start.Binding); !errors.Is(err, ErrOAuthAttemptInvalid) {
		t.Fatalf("provider corp error = %v", err)
	}
	if states.claimCalls != 1 || provider.exchangeCalls != 1 || auth.issueCalls != 0 {
		t.Fatalf("corp mismatch calls = %d/%d/%d", states.claimCalls, provider.exchangeCalls, auth.issueCalls)
	}
	if _, err = service.Complete(context.Background(), "code", start.State, start.Binding); !errors.Is(err, ErrOAuthAttemptInvalid) || provider.exchangeCalls != 1 {
		t.Fatalf("corp mismatch replay error/calls = %v/%d", err, provider.exchangeCalls)
	}
}

func TestOAuthGrantRevokesIssuedSessionWhenRBACCannotAuthorizeContext(t *testing.T) {
	service, _, _, _, auth, _, _ := oauthGrantFixture(t)
	start, err := service.Begin(context.Background(), "wm_external_41", "/sidebar/bind-mobile")
	if err != nil {
		t.Fatal(err)
	}
	auth.authorizeErr = authport.ErrUnauthorized
	completed, err := service.Complete(context.Background(), "code", start.State, start.Binding)
	if !errors.Is(err, ErrForbidden) || completed.Session.Session != "" || auth.issueCalls != 1 || auth.invalidateCalls != 1 {
		t.Fatalf("Complete()=%+v error=%v issue/invalidate=%d/%d", completed, err, auth.issueCalls, auth.invalidateCalls)
	}
}

type oauthGrantStates struct {
	now        time.Time
	state      authport.OAuthState
	next       string
	claimCalls int
}

func (states *oauthGrantStates) Begin(_ context.Context, provider authport.Provider, nextPath string) (authport.OAuthAttempt, error) {
	if provider != authport.ProviderWeCom {
		return authport.OAuthAttempt{}, authport.ErrOAuthStateInvalid
	}
	states.next = nextPath
	return authport.OAuthAttempt{State: states.state, ExpiresAt: states.now.Add(5 * time.Minute)}, nil
}

func (states *oauthGrantStates) Claim(_ context.Context, provider authport.Provider, state authport.OAuthState) (authport.OAuthClaim, error) {
	states.claimCalls++
	if states.claimCalls > 1 || provider != authport.ProviderWeCom || state != states.state {
		return authport.OAuthClaim{}, authport.ErrOAuthStateInvalid
	}
	return authport.OAuthClaim{Provider: provider, NextPath: states.next}, nil
}

type oauthGrantProvider struct {
	identity      OAuthIdentity
	exchangeCalls int
}

func (*oauthGrantProvider) CorpID() string { return "corp-1" }
func (*oauthGrantProvider) AuthorizationURL(state string) (string, error) {
	return "https://open.weixin.test/authorize?state=" + state, nil
}
func (provider *oauthGrantProvider) Exchange(context.Context, string) (OAuthIdentity, error) {
	provider.exchangeCalls++
	return provider.identity, nil
}

type oauthGrantAuth struct {
	principal                       authport.Principal
	session                         authport.BrowserSession
	authorizeErr                    error
	issueCalls, authenticateCalls   int
	authorizeCalls, invalidateCalls int
}

func (auth *oauthGrantAuth) IssueVerified(_ context.Context, login authport.VerifiedLogin) (authport.BrowserSession, error) {
	auth.issueCalls++
	if login != (authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: "corp-1", SubjectID: "member-7"}) {
		return authport.BrowserSession{}, authport.ErrInvalidVerifiedLogin
	}
	return auth.session, nil
}
func (auth *oauthGrantAuth) Authenticate(_ context.Context, session authport.SessionRef) (authport.Principal, error) {
	auth.authenticateCalls++
	if session != auth.session.Session {
		return authport.Principal{}, authport.ErrUnauthenticated
	}
	return auth.principal, nil
}
func (auth *oauthGrantAuth) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	auth.authorizeCalls++
	if auth.authorizeErr != nil {
		return authport.Authorization{}, auth.authorizeErr
	}
	if principal != auth.principal || capability != authport.CapabilityCustomersRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}
func (*oauthGrantAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (auth *oauthGrantAuth) Invalidate(_ context.Context, session authport.SessionRef, csrf authport.CSRFToken) error {
	auth.invalidateCalls++
	if session != auth.session.Session || csrf != auth.session.CSRF {
		return authport.ErrUnauthenticated
	}
	return nil
}

type oauthGrantResolver struct {
	result identityport.ResolveResult
	err    error
	ref    identityport.IDRef
	calls  int
}

func (resolver *oauthGrantResolver) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	resolver.calls++
	resolver.ref = ref
	return resolver.result, resolver.err
}

func oauthGrantFixture(t *testing.T) (*OAuthGrantService, *Service, *oauthGrantStates, *oauthGrantProvider, *oauthGrantAuth, *oauthGrantResolver, time.Time) {
	t.Helper()
	now := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	contexts, _ := sidebarTestService(t)
	contexts.now = func() time.Time { return now }
	resolver := &oauthGrantResolver{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(41)}}
	contexts.identity = resolver
	states := &oauthGrantStates{now: now, state: authport.OAuthState(oauthTestToken(1))}
	provider := &oauthGrantProvider{identity: OAuthIdentity{CorpID: "corp-1", UserID: "member-7"}}
	auth := &oauthGrantAuth{
		principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin},
		session:   authport.BrowserSession{Session: authport.SessionRef(oauthTestToken(2)), CSRF: authport.CSRFToken(oauthTestToken(3)), ExpiresAt: now.Add(8 * time.Hour)},
	}
	service, err := NewOAuthGrantService(states, provider, auth, auth, contexts, []byte("01234567890123456789012345678901"), OAuthGrantOptions{
		Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{4}, 64)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, contexts, states, provider, auth, resolver, now
}

func oauthTestToken(value byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}
