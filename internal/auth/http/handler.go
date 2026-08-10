package authhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	SessionCookieName = "aicrm_session"
	CSRFCookieName    = "aicrm_csrf"
)

type Handler struct {
	generated.Unimplemented
	auth authport.Service
}

var _ generated.ServerInterface = (*Handler)(nil)

func NewHandler(auth authport.Service) (*Handler, error) {
	if auth == nil {
		return nil, authport.ErrAuthenticationUnavailable
	}
	return &Handler{auth: auth}, nil
}

// Authenticate protects a route group. Public routes must be mounted outside it.
func (handler *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if handler == nil || handler.auth == nil || next == nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, authport.ErrAuthenticationUnavailable))
			return
		}
		cookie, err := request.Cookie(SessionCookieName)
		if err != nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
			return
		}
		session := authport.SessionRef(cookie.Value)
		principal, err := handler.auth.Authenticate(request.Context(), session)
		if err != nil {
			code := platformhttp.CodeUnauthenticated
			if errors.Is(err, authport.ErrAuthenticationUnavailable) {
				code = platformhttp.CodeDependencyUnavailable
			}
			platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
			return
		}
		ctx := authport.WithAuthenticatedSession(request.Context(), principal, session)
		ctx, err = platformhttp.ContextWithAccountID(ctx, accountID(principal.AdminUserID))
		if err != nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, err))
			return
		}
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func (handler *Handler) GetAuthSession(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(generated.AuthSessionResponse{
		AdminUserId: principal.AdminUserID,
		Role:        generated.AuthSessionResponseRole(principal.Role),
		StaffId:     principal.StaffID,
	})
}

func (handler *Handler) LogoutAdmin(writer http.ResponseWriter, request *http.Request, params generated.LogoutAdminParams) {
	session, ok := authport.SessionFromContext(request.Context())
	if !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
		return
	}
	if err := handler.auth.Invalidate(request.Context(), session, authport.CSRFToken(params.XCSRFToken)); err != nil {
		code := platformhttp.CodeUnauthorized
		if errors.Is(err, authport.ErrUnauthenticated) {
			code = platformhttp.CodeUnauthenticated
		} else if errors.Is(err, authport.ErrAuthenticationUnavailable) {
			code = platformhttp.CodeDependencyUnavailable
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
		return
	}
	ClearBrowserSession(writer)
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusNoContent)
}

func WriteBrowserSession(writer http.ResponseWriter, session authport.BrowserSession) error {
	if writer == nil || session.Session == "" || session.CSRF == "" || !session.ExpiresAt.After(time.Now().UTC()) {
		return authport.ErrAuthenticationUnavailable
	}
	maxAge := int(time.Until(session.ExpiresAt).Seconds())
	if maxAge < 1 {
		return authport.ErrAuthenticationUnavailable
	}
	http.SetCookie(writer, &http.Cookie{Name: SessionCookieName, Value: string(session.Session), Path: "/", Expires: session.ExpiresAt, MaxAge: maxAge, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	// The browser must echo this non-HttpOnly token in X-CSRF-Token. It is not a bearer credential by itself.
	http.SetCookie(writer, &http.Cookie{Name: CSRFCookieName, Value: string(session.CSRF), Path: "/", Expires: session.ExpiresAt, MaxAge: maxAge, Secure: true, HttpOnly: false, SameSite: http.SameSiteStrictMode})
	return nil
}

func ClearBrowserSession(writer http.ResponseWriter) {
	http.SetCookie(writer, expiredCookie(SessionCookieName, true, http.SameSiteLaxMode))
	http.SetCookie(writer, expiredCookie(CSRFCookieName, false, http.SameSiteStrictMode))
}

func expiredCookie(name string, httpOnly bool, sameSite http.SameSite) *http.Cookie {
	return &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0).UTC(), Secure: true, HttpOnly: httpOnly, SameSite: sameSite}
}

func accountID(id int64) string {
	const digits = "0123456789"
	if id < 1 {
		return "invalid"
	}
	var buffer [20]byte
	position := len(buffer)
	for id > 0 {
		position--
		buffer[position] = digits[id%10]
		id /= 10
	}
	return "admin:" + string(buffer[position:])
}
