package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type fakeAuthUoW struct {
	calls int
	err   error
}

func (uow *fakeAuthUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	return callback(ctx)
}

type fakeAuthRepository struct {
	findCalls, insertCalls, getCalls, csrfCalls, revokeCalls int

	loginUser authstore.LoginUser
	findErr   error
	insertErr error
	principal authport.Principal
	getErr    error
	csrfValid bool
	csrfErr   error
	revokeErr error

	findLogin authport.VerifiedLogin

	insertSessionHash []byte
	insertCSRFHash    []byte
	insertUser        authstore.LoginUser
	insertAuthTime    time.Time
	insertExpiresAt   time.Time

	getSessionHash  []byte
	getAt           time.Time
	csrfSessionHash []byte
	csrfTokenHash   []byte
	csrfAt          time.Time

	revokeSessionHash []byte
	revokeCSRFHash    []byte
	revokedAt         time.Time
}

type fakeStaffResolver struct {
	staffID     *int64
	err         error
	calls       int
	adminUserID int64
}

func (resolver *fakeStaffResolver) ResolveStaffID(_ context.Context, adminUserID int64) (*int64, error) {
	resolver.calls++
	resolver.adminUserID = adminUserID
	return resolver.staffID, resolver.err
}

func (repository *fakeAuthRepository) FindVerifiedLogin(_ context.Context, login authport.VerifiedLogin) (authstore.LoginUser, error) {
	repository.findCalls++
	repository.findLogin = login
	return repository.loginUser, repository.findErr
}

func (repository *fakeAuthRepository) InsertSession(_ context.Context, sessionHash, csrfHash []byte, user authstore.LoginUser, authTime, expiresAt time.Time) error {
	repository.insertCalls++
	repository.insertSessionHash = append([]byte(nil), sessionHash...)
	repository.insertCSRFHash = append([]byte(nil), csrfHash...)
	repository.insertUser = user
	repository.insertAuthTime = authTime
	repository.insertExpiresAt = expiresAt
	return repository.insertErr
}

func (repository *fakeAuthRepository) GetActive(_ context.Context, sessionHash []byte, now time.Time) (authport.Principal, error) {
	repository.getCalls++
	repository.getSessionHash = append([]byte(nil), sessionHash...)
	repository.getAt = now
	return repository.principal, repository.getErr
}

func (repository *fakeAuthRepository) ValidateCSRF(_ context.Context, sessionHash, csrfHash []byte, now time.Time) (bool, error) {
	repository.csrfCalls++
	repository.csrfSessionHash = append([]byte(nil), sessionHash...)
	repository.csrfTokenHash = append([]byte(nil), csrfHash...)
	repository.csrfAt = now
	return repository.csrfValid, repository.csrfErr
}

func (repository *fakeAuthRepository) Revoke(_ context.Context, sessionHash, csrfHash []byte, revokedAt time.Time) error {
	repository.revokeCalls++
	repository.revokeSessionHash = append([]byte(nil), sessionHash...)
	repository.revokeCSRFHash = append([]byte(nil), csrfHash...)
	repository.revokedAt = revokedAt
	return repository.revokeErr
}

type recordedReader struct {
	data     []byte
	offset   int
	requests []int
}

func (reader *recordedReader) Read(destination []byte) (int, error) {
	reader.requests = append(reader.requests, len(destination))
	if reader.offset >= len(reader.data) {
		return 0, io.EOF
	}
	count := copy(destination, reader.data[reader.offset:])
	reader.offset += count
	return count, nil
}

type failingReader struct{ err error }

func (reader failingReader) Read([]byte) (int, error) { return 0, reader.err }

func TestIssueVerifiedRejectsUntrustedOrUnsafeIdentityBeforeTransaction(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		login authport.VerifiedLogin
	}{
		{name: "non-WeCom provider", login: authport.VerifiedLogin{Provider: "oidc", CorpID: "corp-1", SubjectID: "user-1"}},
		{name: "empty corp ID", login: authport.VerifiedLogin{Provider: authport.ProviderWeCom, SubjectID: "user-1"}},
		{name: "corp ID begins with punctuation", login: authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: ".corp-1", SubjectID: "user-1"}},
		{name: "corp ID has path separator", login: authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: "corp/1", SubjectID: "user-1"}},
		{name: "empty subject", login: authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: "corp-1"}},
		{name: "subject has whitespace", login: authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: "corp-1", SubjectID: "user 1"}},
		{name: "subject too long", login: authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: "corp-1", SubjectID: strings.Repeat("u", 129)}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeAuthUoW{}
			repository := &fakeAuthRepository{loginUser: usableLoginUser()}
			service := newTestAuthService(t, uow, repository, Options{Random: &recordedReader{data: sequenceBytes(64)}})

			session, err := service.IssueVerified(context.Background(), testCase.login)
			if !errors.Is(err, authport.ErrInvalidVerifiedLogin) {
				t.Fatalf("IssueVerified() error = %v, want ErrInvalidVerifiedLogin", err)
			}
			if session.Session != "" || session.CSRF != "" || !session.ExpiresAt.IsZero() {
				t.Fatalf("IssueVerified() session = %#v, want empty", session)
			}
			if uow.calls != 0 || repository.findCalls != 0 || repository.insertCalls != 0 {
				t.Fatalf("unsafe login reached transaction/repository: uow=%d find=%d insert=%d", uow.calls, repository.findCalls, repository.insertCalls)
			}
		})
	}
}

func TestIssueVerifiedIssuesTwoRawURLTokensAndPersistsOnlyHashes(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	randomBytes := sequenceBytes(64)
	random := &recordedReader{data: randomBytes}
	uow := &fakeAuthUoW{}
	repository := &fakeAuthRepository{loginUser: usableLoginUser()}
	service := newTestAuthService(t, uow, repository, Options{
		Clock:  func() time.Time { return now },
		Random: random,
	})

	session, err := service.IssueVerified(context.Background(), safeVerifiedLogin())
	if err != nil {
		t.Fatalf("IssueVerified() error = %v", err)
	}

	assertRawURLToken(t, string(session.Session), randomBytes[:32])
	assertRawURLToken(t, string(session.CSRF), randomBytes[32:])
	if len(random.requests) != 2 || random.requests[0] != 32 || random.requests[1] != 32 || random.offset != 64 {
		t.Fatalf("random reads = requests:%v offset:%d, want [32 32]/64", random.requests, random.offset)
	}
	assertTokenHash(t, repository.insertSessionHash, string(session.Session))
	assertTokenHash(t, repository.insertCSRFHash, string(session.CSRF))
	if bytes.Equal(repository.insertSessionHash, []byte(session.Session)) || bytes.Equal(repository.insertCSRFHash, []byte(session.CSRF)) {
		t.Fatal("repository received raw browser token material")
	}
	if uow.calls != 1 || repository.findCalls != 1 || repository.insertCalls != 1 {
		t.Fatalf("calls = uow:%d find:%d insert:%d, want 1/1/1", uow.calls, repository.findCalls, repository.insertCalls)
	}
}

func TestIssueVerifiedUsesDefaultAndConfiguredLifetime(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	for _, testCase := range []struct {
		name     string
		lifetime time.Duration
		want     time.Duration
	}{
		{name: "default", want: DefaultSessionLifetime},
		{name: "configured", lifetime: 90 * time.Minute, want: 90 * time.Minute},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &fakeAuthRepository{loginUser: usableLoginUser()}
			service := newTestAuthService(t, &fakeAuthUoW{}, repository, Options{
				Clock:    func() time.Time { return now },
				Random:   &recordedReader{data: sequenceBytes(64)},
				Lifetime: testCase.lifetime,
			})

			session, err := service.IssueVerified(context.Background(), safeVerifiedLogin())
			if err != nil {
				t.Fatalf("IssueVerified() error = %v", err)
			}
			wantAuthTime := now.UTC()
			wantExpiresAt := wantAuthTime.Add(testCase.want)
			if !session.ExpiresAt.Equal(wantExpiresAt) || session.ExpiresAt.Location() != time.UTC {
				t.Fatalf("BrowserSession.ExpiresAt = %v, want %v UTC", session.ExpiresAt, wantExpiresAt)
			}
			if !repository.insertAuthTime.Equal(wantAuthTime) || repository.insertAuthTime.Location() != time.UTC ||
				!repository.insertExpiresAt.Equal(wantExpiresAt) || repository.insertExpiresAt.Location() != time.UTC {
				t.Fatalf("persisted times = auth:%v expires:%v, want %v/%v UTC", repository.insertAuthTime, repository.insertExpiresAt, wantAuthTime, wantExpiresAt)
			}
		})
	}
}

func TestIssueVerifiedPersistsResolvedCurrentStaff(t *testing.T) {
	staffID := int64(42)
	resolver := &fakeStaffResolver{staffID: &staffID}
	repository := &fakeAuthRepository{loginUser: usableLoginUser()}
	service := newTestAuthService(t, &fakeAuthUoW{}, repository, Options{
		Random:        &recordedReader{data: sequenceBytes(64)},
		StaffResolver: resolver,
	})

	if _, err := service.IssueVerified(context.Background(), safeVerifiedLogin()); err != nil {
		t.Fatalf("IssueVerified() error = %v", err)
	}
	if repository.insertUser.Principal.StaffID == nil || *repository.insertUser.Principal.StaffID != staffID {
		t.Fatalf("persisted principal = %#v", repository.insertUser.Principal)
	}
	if resolver.calls != 1 || resolver.adminUserID != 7 {
		t.Fatalf("staff resolver calls/admin=%d/%d, want 1/7", resolver.calls, resolver.adminUserID)
	}
}

func TestIssueVerifiedFailsClosedWhenVerifiedLoginDoesNotResolve(t *testing.T) {
	for _, name := range []string{"unknown user", "disabled login is hidden as no rows"} {
		t.Run(name, func(t *testing.T) {
			uow := &fakeAuthUoW{}
			repository := &fakeAuthRepository{findErr: pgx.ErrNoRows}
			service := newTestAuthService(t, uow, repository, Options{Random: &recordedReader{data: sequenceBytes(64)}})

			session, err := service.IssueVerified(context.Background(), safeVerifiedLogin())
			if !errors.Is(err, authport.ErrUnauthenticated) || errors.Is(err, authport.ErrAuthenticationUnavailable) {
				t.Fatalf("IssueVerified() error = %v, want fail-closed ErrUnauthenticated only", err)
			}
			if session.Session != "" || session.CSRF != "" || !session.ExpiresAt.IsZero() {
				t.Fatalf("IssueVerified() session = %#v, want empty", session)
			}
			if uow.calls != 1 || repository.findCalls != 1 || repository.insertCalls != 0 {
				t.Fatalf("calls = uow:%d find:%d insert:%d, want 1/1/0", uow.calls, repository.findCalls, repository.insertCalls)
			}
		})
	}
}

func TestIssueVerifiedFailsClosedForInvalidRepositoryUser(t *testing.T) {
	zeroStaffID := int64(0)
	for _, testCase := range []struct {
		name string
		user authstore.LoginUser
	}{
		{name: "invalid principal", user: authstore.LoginUser{Principal: authport.Principal{AdminUserID: 0, Role: authport.RoleAdmin}, SessionVersion: 1}},
		{name: "invalid staff", user: authstore.LoginUser{Principal: authport.Principal{AdminUserID: 1, Role: authport.RoleOps, StaffID: &zeroStaffID}, SessionVersion: 1}},
		{name: "missing session version", user: authstore.LoginUser{Principal: usablePrincipal(), SessionVersion: 0}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &fakeAuthRepository{loginUser: testCase.user}
			service := newTestAuthService(t, &fakeAuthUoW{}, repository, Options{Random: &recordedReader{data: sequenceBytes(64)}})

			session, err := service.IssueVerified(context.Background(), safeVerifiedLogin())
			if !errors.Is(err, authport.ErrUnauthenticated) {
				t.Fatalf("IssueVerified() error = %v, want ErrUnauthenticated", err)
			}
			if session.Session != "" || session.CSRF != "" || !session.ExpiresAt.IsZero() || repository.insertCalls != 0 {
				t.Fatalf("IssueVerified() session/insert = %#v/%d, want empty/0", session, repository.insertCalls)
			}
		})
	}
}

func TestIssueVerifiedRandomFailureDoesNotQueryRepository(t *testing.T) {
	sentinel := errors.New("random source unavailable")
	uow := &fakeAuthUoW{}
	repository := &fakeAuthRepository{loginUser: usableLoginUser()}
	service := newTestAuthService(t, uow, repository, Options{Random: failingReader{err: sentinel}})

	session, err := service.IssueVerified(context.Background(), safeVerifiedLogin())
	if !errors.Is(err, authport.ErrAuthenticationUnavailable) || !errors.Is(err, sentinel) {
		t.Fatalf("IssueVerified() error = %v, want joined authentication and random errors", err)
	}
	if session.Session != "" || session.CSRF != "" || !session.ExpiresAt.IsZero() {
		t.Fatalf("IssueVerified() session = %#v, want empty", session)
	}
	if uow.calls != 0 || repository.findCalls != 0 || repository.insertCalls != 0 {
		t.Fatalf("random failure reached transaction/repository: uow=%d find=%d insert=%d", uow.calls, repository.findCalls, repository.insertCalls)
	}
}

func TestAuthenticateRejectsMalformedTokensBeforeRepository(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		token authport.SessionRef
	}{
		{name: "empty", token: ""},
		{name: "not base64url", token: "not/a-token"},
		{name: "wrong decoded size", token: authport.SessionRef(base64.RawURLEncoding.EncodeToString(sequenceBytes(31)))},
		{name: "padded", token: authport.SessionRef(base64.URLEncoding.EncodeToString(sequenceBytes(32)))},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeAuthUoW{}
			repository := &fakeAuthRepository{}
			service := newTestAuthService(t, uow, repository, Options{})

			principal, err := service.Authenticate(context.Background(), testCase.token)
			if !errors.Is(err, authport.ErrUnauthenticated) {
				t.Fatalf("Authenticate() error = %v, want ErrUnauthenticated", err)
			}
			if principal != (authport.Principal{}) {
				t.Fatalf("Authenticate() principal = %#v, want empty", principal)
			}
			if uow.calls != 0 || repository.getCalls != 0 {
				t.Fatalf("malformed token reached transaction/repository: uow=%d get=%d", uow.calls, repository.getCalls)
			}
		})
	}
}

func TestAuthenticateReturnsValidPrincipal(t *testing.T) {
	staffID := int64(42)
	wantPrincipal := authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &staffID}
	now := time.Date(2026, 8, 11, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	token := authport.SessionRef(rawToken(sequenceBytes(32)))
	uow := &fakeAuthUoW{}
	repository := &fakeAuthRepository{principal: wantPrincipal}
	service := newTestAuthService(t, uow, repository, Options{Clock: func() time.Time { return now }})

	principal, err := service.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal != wantPrincipal {
		t.Fatalf("Authenticate() principal = %#v, want %#v", principal, wantPrincipal)
	}
	assertTokenHash(t, repository.getSessionHash, string(token))
	if !repository.getAt.Equal(now.UTC()) || repository.getAt.Location() != time.UTC {
		t.Fatalf("GetActive() time = %v, want %v UTC", repository.getAt, now.UTC())
	}
	if uow.calls != 1 || repository.getCalls != 1 {
		t.Fatalf("calls = uow:%d get:%d, want 1/1", uow.calls, repository.getCalls)
	}
}

func TestAuthenticateResolvesCurrentStaffOutsideMinimalAuthQueries(t *testing.T) {
	staffID := int64(42)
	resolver := &fakeStaffResolver{staffID: &staffID}
	token := authport.SessionRef(rawToken(sequenceBytes(32)))
	repository := &fakeAuthRepository{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales}}
	service := newTestAuthService(t, &fakeAuthUoW{}, repository, Options{StaffResolver: resolver})

	principal, err := service.Authenticate(context.Background(), token)
	if err != nil || principal.StaffID == nil || *principal.StaffID != staffID {
		t.Fatalf("Authenticate() principal=%#v error=%v", principal, err)
	}
	if resolver.calls != 1 || resolver.adminUserID != 7 {
		t.Fatalf("staff resolver calls/admin=%d/%d, want 1/7", resolver.calls, resolver.adminUserID)
	}
}

func TestAuthenticateFailsClosedWhenCurrentStaffResolutionFails(t *testing.T) {
	sentinel := errors.New("staff directory unavailable")
	resolver := &fakeStaffResolver{err: sentinel}
	repository := &fakeAuthRepository{principal: usablePrincipal()}
	service := newTestAuthService(t, &fakeAuthUoW{}, repository, Options{StaffResolver: resolver})

	principal, err := service.Authenticate(context.Background(), authport.SessionRef(rawToken(sequenceBytes(32))))
	if principal != (authport.Principal{}) || !errors.Is(err, authport.ErrAuthenticationUnavailable) || !errors.Is(err, sentinel) {
		t.Fatalf("Authenticate() principal=%#v error=%v", principal, err)
	}
}

func TestAuthenticateFailsClosedForInvalidPrincipal(t *testing.T) {
	zeroStaffID := int64(0)
	for _, testCase := range []struct {
		name      string
		principal authport.Principal
	}{
		{name: "missing admin id", principal: authport.Principal{Role: authport.RoleAdmin}},
		{name: "unknown role", principal: authport.Principal{AdminUserID: 1, Role: authport.Role("superuser")}},
		{name: "nonpositive staff id", principal: authport.Principal{AdminUserID: 1, Role: authport.RoleOps, StaffID: &zeroStaffID}},
		{name: "sales missing staff id", principal: authport.Principal{AdminUserID: 1, Role: authport.RoleSales}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeAuthUoW{}
			repository := &fakeAuthRepository{principal: testCase.principal}
			service := newTestAuthService(t, uow, repository, Options{})

			principal, err := service.Authenticate(context.Background(), authport.SessionRef(rawToken(sequenceBytes(32))))
			if !errors.Is(err, authport.ErrUnauthenticated) {
				t.Fatalf("Authenticate() error = %v, want ErrUnauthenticated", err)
			}
			if principal != (authport.Principal{}) {
				t.Fatalf("Authenticate() principal = %#v, want empty", principal)
			}
			if uow.calls != 1 || repository.getCalls != 1 {
				t.Fatalf("calls = uow:%d get:%d, want 1/1", uow.calls, repository.getCalls)
			}
		})
	}
}

func TestAuthenticateFailsClosedForMissingOrUnavailableSession(t *testing.T) {
	sentinel := errors.New("session store unavailable")
	for _, testCase := range []struct {
		name            string
		repositoryError error
		wantUnavailable bool
		wantUnderlying  error
	}{
		{name: "no rows", repositoryError: pgx.ErrNoRows},
		{name: "repository failure", repositoryError: sentinel, wantUnavailable: true, wantUnderlying: sentinel},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &fakeAuthRepository{getErr: testCase.repositoryError}
			service := newTestAuthService(t, &fakeAuthUoW{}, repository, Options{})

			principal, err := service.Authenticate(context.Background(), authport.SessionRef(rawToken(sequenceBytes(32))))
			if principal != (authport.Principal{}) {
				t.Fatalf("Authenticate() principal = %#v, want empty", principal)
			}
			if testCase.wantUnavailable {
				if !errors.Is(err, authport.ErrAuthenticationUnavailable) || !errors.Is(err, testCase.wantUnderlying) {
					t.Fatalf("Authenticate() error = %v, want joined authentication/repository errors", err)
				}
				return
			}
			if !errors.Is(err, authport.ErrUnauthenticated) || errors.Is(err, authport.ErrAuthenticationUnavailable) {
				t.Fatalf("Authenticate() error = %v, want fail-closed ErrUnauthenticated only", err)
			}
		})
	}
}

func TestInvalidateRejectsMalformedSessionOrCSRFBeforeRepository(t *testing.T) {
	validSession := authport.SessionRef(rawToken(sequenceBytes(32)))
	validCSRF := authport.CSRFToken(rawToken(sequenceBytes(32)))
	for _, testCase := range []struct {
		name      string
		session   authport.SessionRef
		csrf      authport.CSRFToken
		wantError error
	}{
		{name: "invalid session", session: "not-a-token", csrf: validCSRF, wantError: authport.ErrUnauthenticated},
		{name: "invalid csrf", session: validSession, csrf: "not-a-token", wantError: authport.ErrCSRFInvalid},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeAuthUoW{}
			repository := &fakeAuthRepository{}
			service := newTestAuthService(t, uow, repository, Options{})

			err := service.Invalidate(context.Background(), testCase.session, testCase.csrf)
			if !errors.Is(err, testCase.wantError) {
				t.Fatalf("Invalidate() error = %v, want %v", err, testCase.wantError)
			}
			if uow.calls != 0 || repository.revokeCalls != 0 {
				t.Fatalf("malformed token reached transaction/repository: uow=%d revoke=%d", uow.calls, repository.revokeCalls)
			}
		})
	}
}

func TestInvalidateRevokesSessionWithOnlyHashes(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	session := authport.SessionRef(rawToken(sequenceBytes(32)))
	csrf := authport.CSRFToken(rawToken(bytes.Repeat([]byte{99}, 32)))
	uow := &fakeAuthUoW{}
	repository := &fakeAuthRepository{}
	service := newTestAuthService(t, uow, repository, Options{Clock: func() time.Time { return now }})

	if err := service.Invalidate(context.Background(), session, csrf); err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}
	assertTokenHash(t, repository.revokeSessionHash, string(session))
	assertTokenHash(t, repository.revokeCSRFHash, string(csrf))
	if bytes.Equal(repository.revokeSessionHash, []byte(session)) || bytes.Equal(repository.revokeCSRFHash, []byte(csrf)) {
		t.Fatal("repository received raw browser token material")
	}
	if !repository.revokedAt.Equal(now.UTC()) || repository.revokedAt.Location() != time.UTC {
		t.Fatalf("Revoke() time = %v, want %v UTC", repository.revokedAt, now.UTC())
	}
	if uow.calls != 1 || repository.revokeCalls != 1 {
		t.Fatalf("calls = uow:%d revoke:%d, want 1/1", uow.calls, repository.revokeCalls)
	}
}

func TestInvalidateFailsClosedForMissingOrUnavailableSession(t *testing.T) {
	sentinel := errors.New("revoke store unavailable")
	for _, testCase := range []struct {
		name            string
		repositoryError error
		wantError       error
		wantUnderlying  error
	}{
		{name: "no rows", repositoryError: pgx.ErrNoRows, wantError: authport.ErrCSRFInvalid},
		{name: "repository failure", repositoryError: sentinel, wantError: authport.ErrAuthenticationUnavailable, wantUnderlying: sentinel},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &fakeAuthRepository{revokeErr: testCase.repositoryError}
			service := newTestAuthService(t, &fakeAuthUoW{}, repository, Options{})

			err := service.Invalidate(
				context.Background(),
				authport.SessionRef(rawToken(sequenceBytes(32))),
				authport.CSRFToken(rawToken(bytes.Repeat([]byte{99}, 32))),
			)
			if !errors.Is(err, testCase.wantError) {
				t.Fatalf("Invalidate() error = %v, want %v", err, testCase.wantError)
			}
			if testCase.wantUnderlying != nil && !errors.Is(err, testCase.wantUnderlying) {
				t.Fatalf("Invalidate() error = %v, want underlying %v", err, testCase.wantUnderlying)
			}
		})
	}
}

func TestAuthorizeNilServiceFailsClosed(t *testing.T) {
	service := (*Service)(nil)
	for _, testCase := range []struct {
		name       string
		ctx        context.Context
		principal  authport.Principal
		capability authport.Capability
	}{
		{name: "nil context and principal", ctx: nil},
		{name: "valid principal and known capability", ctx: context.Background(), principal: usablePrincipal(), capability: authport.CapabilityCustomersRead},
		{name: "invalid principal and arbitrary capability", ctx: context.Background(), principal: authport.Principal{AdminUserID: -1}, capability: authport.Capability("*")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := service.Authorize(testCase.ctx, testCase.principal, testCase.capability); !errors.Is(err, authport.ErrUnauthorized) {
				t.Fatalf("Authorize() error = %v, want ErrUnauthorized", err)
			}
		})
	}
}

func TestValidateCSRFBindsHashesToCurrentActiveSession(t *testing.T) {
	now := time.Date(2026, 8, 11, 14, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	session := rawToken(sequenceBytes(32))
	csrf := rawToken(bytes.Repeat([]byte{91}, 32))
	uow := &fakeAuthUoW{}
	repository := &fakeAuthRepository{csrfValid: true}
	service := newTestAuthService(t, uow, repository, Options{Clock: func() time.Time { return now }})

	if err := service.ValidateCSRF(context.Background(), authport.SessionRef(session), authport.CSRFToken(csrf)); err != nil {
		t.Fatalf("ValidateCSRF() error = %v", err)
	}
	if uow.calls != 1 || repository.csrfCalls != 1 || !repository.csrfAt.Equal(now.UTC()) {
		t.Fatalf("calls/time = uow:%d csrf:%d at:%s, want 1/1/%s", uow.calls, repository.csrfCalls, repository.csrfAt, now.UTC())
	}
	assertTokenHash(t, repository.csrfSessionHash, session)
	assertTokenHash(t, repository.csrfTokenHash, csrf)
	if bytes.Equal(repository.csrfSessionHash, []byte(session)) || bytes.Equal(repository.csrfTokenHash, []byte(csrf)) {
		t.Fatal("repository received raw CSRF or session material")
	}
}

func TestValidateCSRFFailsClosed(t *testing.T) {
	session := authport.SessionRef(rawToken(sequenceBytes(32)))
	csrf := authport.CSRFToken(rawToken(bytes.Repeat([]byte{92}, 32)))
	sentinel := errors.New("database unavailable")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name       string
		ctx        context.Context
		session    authport.SessionRef
		csrf       authport.CSRFToken
		uowErr     error
		csrfValid  bool
		csrfErr    error
		want       error
		underlying error
		wantCalls  int
	}{
		{name: "malformed session", ctx: context.Background(), session: "bad", csrf: csrf, want: authport.ErrUnauthenticated},
		{name: "malformed csrf", ctx: context.Background(), session: session, csrf: "bad", want: authport.ErrCSRFInvalid},
		{name: "nil context", session: session, csrf: csrf, want: authport.ErrAuthenticationUnavailable},
		{name: "cancelled context", ctx: cancelled, session: session, csrf: csrf, want: authport.ErrAuthenticationUnavailable},
		{name: "session token mismatch", ctx: context.Background(), session: session, csrf: csrf, want: authport.ErrCSRFInvalid, wantCalls: 1},
		{name: "repository error", ctx: context.Background(), session: session, csrf: csrf, csrfErr: sentinel, want: authport.ErrAuthenticationUnavailable, underlying: sentinel, wantCalls: 1},
		{name: "uow error", ctx: context.Background(), session: session, csrf: csrf, uowErr: sentinel, want: authport.ErrAuthenticationUnavailable, underlying: sentinel},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeAuthUoW{err: testCase.uowErr}
			repository := &fakeAuthRepository{csrfValid: testCase.csrfValid, csrfErr: testCase.csrfErr}
			service := newTestAuthService(t, uow, repository, Options{})
			err := service.ValidateCSRF(testCase.ctx, testCase.session, testCase.csrf)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("ValidateCSRF() error = %v, want %v", err, testCase.want)
			}
			if testCase.underlying != nil && !errors.Is(err, testCase.underlying) {
				t.Fatalf("ValidateCSRF() error = %v, want underlying %v", err, testCase.underlying)
			}
			if repository.csrfCalls != testCase.wantCalls {
				t.Fatalf("ValidateCSRF() repository calls = %d, want %d", repository.csrfCalls, testCase.wantCalls)
			}
		})
	}
}

func TestValidateCSRFNilServiceFailsClosed(t *testing.T) {
	var service *Service
	err := service.ValidateCSRF(
		context.Background(),
		authport.SessionRef(rawToken(sequenceBytes(32))),
		authport.CSRFToken(rawToken(bytes.Repeat([]byte{93}, 32))),
	)
	if !errors.Is(err, authport.ErrAuthenticationUnavailable) {
		t.Fatalf("ValidateCSRF() error = %v, want ErrAuthenticationUnavailable", err)
	}
}

func TestNewServiceRejectsInvalidDependenciesAndLifetimes(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		uow      platformport.UnitOfWork
		repo     repository
		lifetime time.Duration
	}{
		{name: "nil unit of work", repo: &fakeAuthRepository{}, lifetime: DefaultSessionLifetime},
		{name: "nil repository", uow: &fakeAuthUoW{}, lifetime: DefaultSessionLifetime},
		{name: "lifetime below minimum", uow: &fakeAuthUoW{}, repo: &fakeAuthRepository{}, lifetime: time.Minute - time.Nanosecond},
		{name: "lifetime above maximum", uow: &fakeAuthUoW{}, repo: &fakeAuthRepository{}, lifetime: 24*time.Hour + time.Nanosecond},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service, err := NewService(testCase.uow, testCase.repo, Options{Lifetime: testCase.lifetime})
			if service != nil || !errors.Is(err, authport.ErrAuthenticationUnavailable) {
				t.Fatalf("NewService() = %v, %v; want nil, ErrAuthenticationUnavailable", service, err)
			}
		})
	}
}

func newTestAuthService(t *testing.T, uow platformport.UnitOfWork, repo repository, options Options) *Service {
	t.Helper()
	service, err := NewService(uow, repo, options)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func safeVerifiedLogin() authport.VerifiedLogin {
	return authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: "corp-1", SubjectID: "user_1"}
}

func usablePrincipal() authport.Principal {
	return authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}
}

func usableLoginUser() authstore.LoginUser {
	return authstore.LoginUser{Principal: usablePrincipal(), SessionVersion: 1}
}

func sequenceBytes(length int) []byte {
	data := make([]byte, length)
	for index := range data {
		data[index] = byte(index)
	}
	return data
}

func rawToken(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func assertRawURLToken(t *testing.T, token string, wantRaw []byte) {
	t.Helper()
	if len(token) != 43 || strings.ContainsAny(token, "+/=") {
		t.Fatalf("token = %q, want a 43-character unpadded base64url token", token)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("DecodeString(%q) error = %v", token, err)
	}
	if len(decoded) != 32 || !bytes.Equal(decoded, wantRaw) {
		t.Fatalf("decoded token = %x, want 32 random bytes %x", decoded, wantRaw)
	}
}

func assertTokenHash(t *testing.T, got []byte, raw string) {
	t.Helper()
	want := sha256.Sum256([]byte(raw))
	if len(got) != sha256.Size || !bytes.Equal(got, want[:]) {
		t.Fatalf("token hash = %x, want SHA-256(%q) = %x", got, raw, want)
	}
}
