package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const DefaultOAuthStateLifetime = 5 * time.Minute

type OAuthStateOptions struct {
	Clock    func() time.Time
	Random   io.Reader
	Lifetime time.Duration
}

type oauthStateRepository interface {
	InsertOAuthState(context.Context, []byte, authport.Provider, string, time.Time, time.Time) error
	ClaimOAuthState(context.Context, []byte, authport.Provider, time.Time) (authstore.OAuthStateClaim, error)
}

type OAuthStateService struct {
	uow      platformport.UnitOfWork
	repo     oauthStateRepository
	clock    func() time.Time
	random   io.Reader
	lifetime time.Duration
}

var _ authport.OAuthStateManager = (*OAuthStateService)(nil)

func NewOAuthStateService(uow platformport.UnitOfWork, repo oauthStateRepository, options OAuthStateOptions) (*OAuthStateService, error) {
	if nilOAuthDependency(uow) || nilOAuthDependency(repo) {
		return nil, authport.ErrOAuthStateUnavailable
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if options.Lifetime == 0 {
		options.Lifetime = DefaultOAuthStateLifetime
	}
	if options.Lifetime < time.Minute || options.Lifetime > 15*time.Minute {
		return nil, authport.ErrOAuthStateUnavailable
	}
	return &OAuthStateService{uow: uow, repo: repo, clock: options.Clock, random: options.Random, lifetime: options.Lifetime}, nil
}

func (service *OAuthStateService) Begin(ctx context.Context, provider authport.Provider, nextPath string) (authport.OAuthAttempt, error) {
	if service == nil || service.uow == nil || service.repo == nil || service.clock == nil || service.random == nil ||
		provider != authport.ProviderWeCom || !validStoredNext(nextPath) {
		return authport.OAuthAttempt{}, authport.ErrOAuthStateInvalid
	}
	var raw [32]byte
	if _, err := io.ReadFull(service.random, raw[:]); err != nil {
		return authport.OAuthAttempt{}, errors.Join(authport.ErrOAuthStateUnavailable, err)
	}
	state := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(state))
	createdAt := service.clock().UTC()
	if createdAt.IsZero() {
		return authport.OAuthAttempt{}, authport.ErrOAuthStateUnavailable
	}
	expiresAt := createdAt.Add(service.lifetime)
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		if insertErr := service.repo.InsertOAuthState(txCtx, hash[:], provider, nextPath, createdAt, expiresAt); insertErr != nil {
			return errors.Join(authport.ErrOAuthStateUnavailable, insertErr)
		}
		return nil
	})
	if err != nil {
		return authport.OAuthAttempt{}, err
	}
	return authport.OAuthAttempt{State: authport.OAuthState(state), ExpiresAt: expiresAt}, nil
}

func (service *OAuthStateService) Claim(ctx context.Context, provider authport.Provider, state authport.OAuthState) (claim authport.OAuthClaim, err error) {
	hash, hashErr := hashToken(string(state))
	if hashErr != nil || provider != authport.ProviderWeCom {
		return authport.OAuthClaim{}, authport.ErrOAuthStateInvalid
	}
	if service == nil || service.uow == nil || service.repo == nil || service.clock == nil {
		return authport.OAuthClaim{}, authport.ErrOAuthStateUnavailable
	}
	now := service.clock().UTC()
	if now.IsZero() {
		return authport.OAuthClaim{}, authport.ErrOAuthStateUnavailable
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		stored, claimErr := service.repo.ClaimOAuthState(txCtx, hash, provider, now)
		if errors.Is(claimErr, pgx.ErrNoRows) {
			return authport.ErrOAuthStateInvalid
		}
		if claimErr != nil {
			return errors.Join(authport.ErrOAuthStateUnavailable, claimErr)
		}
		if !validStoredNext(stored.NextPath) {
			return authport.ErrOAuthStateInvalid
		}
		claim = authport.OAuthClaim{Provider: provider, NextPath: stored.NextPath}
		return nil
	})
	if err != nil {
		return authport.OAuthClaim{}, err
	}
	return claim, nil
}

func validStoredNext(value string) bool {
	if len(value) < 1 || len(value) > 2048 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.ContainsRune(value, '\\') || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func nilOAuthDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
