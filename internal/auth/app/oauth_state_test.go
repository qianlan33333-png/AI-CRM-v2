package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
)

func TestOAuthStateIsHashedAndSingleUse(t *testing.T) {
	now := time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC)
	repository := &fakeOAuthStateRepository{}
	service, err := NewOAuthStateService(&fakeAuthUoW{}, repository, OAuthStateOptions{
		Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x44}, 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := service.Begin(context.Background(), authport.ProviderWeCom, "/admin/customers?tab=active")
	if err != nil || len(attempt.State) != 43 || !attempt.ExpiresAt.Equal(now.Add(DefaultOAuthStateLifetime)) {
		t.Fatalf("Begin()=%+v err=%v", attempt, err)
	}
	wantHash := sha256.Sum256([]byte(attempt.State))
	if !bytes.Equal(repository.insertHash, wantHash[:]) || repository.insertNext != "/admin/customers?tab=active" ||
		bytes.Contains(repository.insertHash, []byte(attempt.State)) {
		t.Fatalf("stored state hash/next = %x/%q", repository.insertHash, repository.insertNext)
	}
	repository.claim = authstore.OAuthStateClaim{NextPath: repository.insertNext}
	claim, err := service.Claim(context.Background(), authport.ProviderWeCom, attempt.State)
	if err != nil || claim.Provider != authport.ProviderWeCom || claim.NextPath != repository.insertNext {
		t.Fatalf("Claim()=%+v err=%v", claim, err)
	}
	if _, err = service.Claim(context.Background(), authport.ProviderWeCom, attempt.State); !errors.Is(err, authport.ErrOAuthStateInvalid) {
		t.Fatalf("replayed Claim() error=%v", err)
	}
}

func TestOAuthStateRejectsUnsafeInputsBeforeStorage(t *testing.T) {
	unsafeNext := []string{"", "admin", "//evil.example/path", `/\\evil.example/path`, "/line\nbreak", " /admin", strings.Repeat("x", 2049)}
	for _, next := range unsafeNext {
		t.Run(next, func(t *testing.T) {
			repository := &fakeOAuthStateRepository{}
			service, err := NewOAuthStateService(&fakeAuthUoW{}, repository, OAuthStateOptions{Random: bytes.NewReader(bytes.Repeat([]byte{1}, 32))})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.Begin(context.Background(), authport.ProviderWeCom, next); !errors.Is(err, authport.ErrOAuthStateInvalid) || repository.insertCalls != 0 {
				t.Fatalf("Begin(%q) error/calls=%v/%d", next, err, repository.insertCalls)
			}
		})
	}
	service, err := NewOAuthStateService(&fakeAuthUoW{}, &fakeOAuthStateRepository{}, OAuthStateOptions{Lifetime: time.Second})
	if !errors.Is(err, authport.ErrOAuthStateUnavailable) || service != nil {
		t.Fatalf("invalid lifetime service/error=%v/%v", service, err)
	}
}

type fakeOAuthStateRepository struct {
	insertCalls int
	insertHash  []byte
	insertNext  string
	claimCalls  int
	claim       authstore.OAuthStateClaim
}

func (repository *fakeOAuthStateRepository) InsertOAuthState(_ context.Context, hash []byte, _ authport.Provider, nextPath string, _, _ time.Time) error {
	repository.insertCalls++
	repository.insertHash = append([]byte(nil), hash...)
	repository.insertNext = nextPath
	return nil
}

func (repository *fakeOAuthStateRepository) ClaimOAuthState(_ context.Context, _ []byte, _ authport.Provider, _ time.Time) (authstore.OAuthStateClaim, error) {
	repository.claimCalls++
	if repository.claimCalls > 1 {
		return authstore.OAuthStateClaim{}, pgx.ErrNoRows
	}
	return repository.claim, nil
}
