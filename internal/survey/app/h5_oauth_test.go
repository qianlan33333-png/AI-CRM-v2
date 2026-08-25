package app

import (
	"context"
	"errors"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestH5OAuthCallbackClaimsOnceAndResolvesOnlyTrustedIdentity(t *testing.T) {
	states := &h5States{claim: authport.OAuthClaim{Provider: authport.ProviderWeCom, NextPath: "/s/survey-1"}}
	provider := &h5Provider{identity: H5ProviderIdentity{CorpID: "ww123", ExternalUserID: "external-1"}}
	identities := &h5IdentityService{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 42}}
	service, err := NewH5OAuthService(states, provider, identities)
	if err != nil {
		t.Fatal(err)
	}
	service.clock = func() time.Time { return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) }
	identity, next, err := service.Callback(context.Background(), "state-abcdefghijklmnopqrstuvwxyz", "code")
	if err != nil || identity.CustomerID != 42 || next != "/s/survey-1" || states.claims != 1 || provider.calls != 1 {
		t.Fatalf("Callback() identity=%+v next=%q err=%v claims=%d calls=%d", identity, next, err, states.claims, provider.calls)
	}
	if identities.ref.Kind != identityport.KindWeComExternalUserID || identities.ref.Value != "external-1" || identities.ref.Assurance != identityport.AssuranceVerified {
		t.Fatalf("Resolve ref=%+v", identities.ref)
	}
	states.err = authport.ErrOAuthStateInvalid
	if _, _, err = service.Callback(context.Background(), "state-abcdefghijklmnopqrstuvwxyz", "code"); !errors.Is(err, ErrH5IdentityRequired) || provider.calls != 1 {
		t.Fatalf("replay err=%v calls=%d", err, provider.calls)
	}
}

func TestH5OAuthRejectsUnsafeNextAndUnresolvedIdentity(t *testing.T) {
	if safeH5Next("//evil") || safeH5Next("/s/ok?next=x") || !safeH5Next("/s/survey-1") {
		t.Fatal("safeH5Next mismatch")
	}
	states := &h5States{claim: authport.OAuthClaim{Provider: authport.ProviderWeCom, NextPath: "/s/survey-1"}}
	service, _ := NewH5OAuthService(states, &h5Provider{identity: H5ProviderIdentity{CorpID: "ww", ExternalUserID: "external"}}, &h5IdentityService{result: identityport.ResolveResult{Status: identityport.ResolveNotFound}})
	if _, _, err := service.Callback(context.Background(), "state-abcdefghijklmnopqrstuvwxyz", "code"); !errors.Is(err, ErrH5IdentityRequired) {
		t.Fatalf("err=%v", err)
	}
}

type h5States struct {
	claim  authport.OAuthClaim
	err    error
	claims int
}

func (s *h5States) Begin(context.Context, authport.Provider, string) (authport.OAuthAttempt, error) {
	return authport.OAuthAttempt{State: "state-abcdefghijklmnopqrstuvwxyz", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (s *h5States) Claim(context.Context, authport.Provider, authport.OAuthState) (authport.OAuthClaim, error) {
	s.claims++
	return s.claim, s.err
}

type h5Provider struct {
	identity H5ProviderIdentity
	calls    int
}

func (p *h5Provider) AuthorizationURL(string) (string, error) {
	return "https://provider.example/oauth", nil
}
func (p *h5Provider) ExchangeExternalIdentity(context.Context, string) (H5ProviderIdentity, error) {
	p.calls++
	return p.identity, nil
}

type h5IdentityService struct {
	result identityport.ResolveResult
	ref    identityport.IDRef
}

func (s *h5IdentityService) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	s.ref = ref
	return s.result, nil
}
func (*h5IdentityService) Bind(context.Context, identityport.BindCommand) (identityport.BindResult, error) {
	return identityport.BindResult{}, errors.New("unexpected")
}
func (*h5IdentityService) Ingest(context.Context, identityport.IngestCommand) (identityport.IngestResult, error) {
	return identityport.IngestResult{}, errors.New("unexpected")
}
