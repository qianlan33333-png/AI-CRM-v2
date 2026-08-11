package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const DefaultSessionLifetime = 8 * time.Hour

var safeProviderID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,127}$`)

type Options struct {
	Clock    func() time.Time
	Random   io.Reader
	Lifetime time.Duration
}

type repository interface {
	FindVerifiedLogin(context.Context, authport.VerifiedLogin) (authstore.LoginUser, error)
	InsertSession(context.Context, []byte, []byte, authstore.LoginUser, time.Time, time.Time) error
	GetActive(context.Context, []byte, time.Time) (authport.Principal, error)
	ValidateCSRF(context.Context, []byte, []byte, time.Time) (bool, error)
	Revoke(context.Context, []byte, []byte, time.Time) error
}

type Service struct {
	uow      platformport.UnitOfWork
	repo     repository
	clock    func() time.Time
	random   io.Reader
	lifetime time.Duration
}

var (
	_ authport.Service = (*Service)(nil)
	_ authport.Issuer  = (*Service)(nil)
)

func NewService(uow platformport.UnitOfWork, repo repository, options Options) (*Service, error) {
	if nilInterface(uow) || nilInterface(repo) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if options.Lifetime == 0 {
		options.Lifetime = DefaultSessionLifetime
	}
	if options.Lifetime < time.Minute || options.Lifetime > 24*time.Hour {
		return nil, authport.ErrAuthenticationUnavailable
	}
	return &Service{uow: uow, repo: repo, clock: options.Clock, random: options.Random, lifetime: options.Lifetime}, nil
}

func (service *Service) IssueVerified(ctx context.Context, login authport.VerifiedLogin) (authport.BrowserSession, error) {
	if service == nil || service.uow == nil || service.repo == nil || service.clock == nil || service.random == nil ||
		login.Provider != authport.ProviderWeCom || !safeProviderID.MatchString(login.TenantID) || !safeProviderID.MatchString(login.SubjectID) {
		return authport.BrowserSession{}, authport.ErrInvalidVerifiedLogin
	}
	session, sessionHash, err := service.newToken()
	if err != nil {
		return authport.BrowserSession{}, err
	}
	csrf, csrfHash, err := service.newToken()
	if err != nil {
		return authport.BrowserSession{}, err
	}
	authTime := service.clock().UTC()
	if authTime.IsZero() {
		return authport.BrowserSession{}, authport.ErrAuthenticationUnavailable
	}
	expiresAt := authTime.Add(service.lifetime)
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		user, findErr := service.repo.FindVerifiedLogin(txCtx, login)
		if errors.Is(findErr, pgx.ErrNoRows) {
			return authport.ErrUnauthenticated
		}
		if findErr != nil {
			return errors.Join(authport.ErrAuthenticationUnavailable, findErr)
		}
		if !validPrincipal(user.Principal) || user.SessionVersion < 1 {
			return authport.ErrUnauthenticated
		}
		if insertErr := service.repo.InsertSession(txCtx, sessionHash, csrfHash, user, authTime, expiresAt); insertErr != nil {
			return errors.Join(authport.ErrAuthenticationUnavailable, insertErr)
		}
		return nil
	})
	if err != nil {
		return authport.BrowserSession{}, err
	}
	return authport.BrowserSession{Session: authport.SessionRef(session), CSRF: authport.CSRFToken(csrf), ExpiresAt: expiresAt}, nil
}

func (service *Service) Authenticate(ctx context.Context, session authport.SessionRef) (principal authport.Principal, err error) {
	tokenHash, err := hashToken(string(session))
	if err != nil || service == nil || service.uow == nil || service.repo == nil || service.clock == nil {
		return authport.Principal{}, authport.ErrUnauthenticated
	}
	now := service.clock().UTC()
	if now.IsZero() {
		return authport.Principal{}, authport.ErrAuthenticationUnavailable
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		principal, err = service.repo.GetActive(txCtx, tokenHash, now)
		if errors.Is(err, pgx.ErrNoRows) {
			return authport.ErrUnauthenticated
		}
		if err != nil {
			return errors.Join(authport.ErrAuthenticationUnavailable, err)
		}
		if !validPrincipal(principal) {
			return authport.ErrUnauthenticated
		}
		return nil
	})
	if err != nil {
		return authport.Principal{}, err
	}
	return principal, nil
}

func (service *Service) Authorize(ctx context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if service == nil || ctx == nil || ctx.Err() != nil {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authorize(principal, capability)
}

func (service *Service) ValidateCSRF(ctx context.Context, session authport.SessionRef, csrf authport.CSRFToken) error {
	tokenHash, err := hashToken(string(session))
	if err != nil {
		return authport.ErrUnauthenticated
	}
	csrfHash, err := hashToken(string(csrf))
	if err != nil {
		return authport.ErrCSRFInvalid
	}
	if service == nil || service.uow == nil || service.repo == nil || service.clock == nil {
		return authport.ErrAuthenticationUnavailable
	}
	now := service.clock().UTC()
	if now.IsZero() {
		return authport.ErrAuthenticationUnavailable
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		valid, validateErr := service.repo.ValidateCSRF(txCtx, tokenHash, csrfHash, now)
		if validateErr != nil {
			return errors.Join(authport.ErrAuthenticationUnavailable, validateErr)
		}
		if !valid {
			return authport.ErrCSRFInvalid
		}
		return nil
	})
	return err
}

func (service *Service) Invalidate(ctx context.Context, session authport.SessionRef, csrf authport.CSRFToken) error {
	tokenHash, err := hashToken(string(session))
	if err != nil {
		return authport.ErrUnauthenticated
	}
	csrfHash, err := hashToken(string(csrf))
	if err != nil {
		return authport.ErrCSRFInvalid
	}
	if service == nil || service.uow == nil || service.repo == nil || service.clock == nil {
		return authport.ErrAuthenticationUnavailable
	}
	revokedAt := service.clock().UTC()
	if revokedAt.IsZero() {
		return authport.ErrAuthenticationUnavailable
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		revokeErr := service.repo.Revoke(txCtx, tokenHash, csrfHash, revokedAt)
		if errors.Is(revokeErr, pgx.ErrNoRows) {
			return authport.ErrCSRFInvalid
		}
		if revokeErr != nil {
			return errors.Join(authport.ErrAuthenticationUnavailable, revokeErr)
		}
		return nil
	})
	return err
}

func (service *Service) newToken() (string, []byte, error) {
	var raw [32]byte
	if _, err := io.ReadFull(service.random, raw[:]); err != nil {
		return "", nil, errors.Join(authport.ErrAuthenticationUnavailable, fmt.Errorf("generate browser token: %w", err))
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(encoded))
	return encoded, hash[:], nil
}

func hashToken(token string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 || len(token) != 43 {
		return nil, authport.ErrUnauthenticated
	}
	hash := sha256.Sum256([]byte(token))
	return hash[:], nil
}

func validPrincipal(principal authport.Principal) bool {
	if principal.AdminUserID < 1 {
		return false
	}
	switch principal.Role {
	case authport.RoleAdmin, authport.RoleOps:
		return principal.StaffID == nil || *principal.StaffID > 0
	case authport.RoleSales:
		return principal.StaffID != nil && *principal.StaffID > 0
	default:
		return false
	}
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
