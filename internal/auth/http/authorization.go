package authhttp

import (
	"errors"
	"net/http"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

// Authorize binds one frozen operation capability to its handler. It must run
// after Authenticate and before the domain handler.
func (handler *Handler) Authorize(capability authport.Capability, next http.Handler) (http.Handler, error) {
	if handler == nil || nilService(handler.auth) || !capability.Known() || next == nil {
		return nil, authport.ErrUnauthorized
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := authport.PrincipalFromContext(request.Context())
		if !ok {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
			return
		}
		authorization, err := handler.auth.Authorize(request.Context(), principal, capability)
		if err != nil {
			code := platformhttp.CodeUnauthorized
			if !errors.Is(err, authport.ErrUnauthorized) {
				code = platformhttp.CodeDependencyUnavailable
			}
			platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
			return
		}
		ctx, err := authport.WithAuthorization(request.Context(), authorization)
		if err != nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, err))
			return
		}
		next.ServeHTTP(writer, request.WithContext(ctx))
	}), nil
}

// AuthorizeOptional preserves an anonymous request but requires the declared
// capability whenever AuthenticateOptional attached a browser principal.
func (handler *Handler) AuthorizeOptional(capability authport.Capability, next http.Handler) (http.Handler, error) {
	if handler == nil || nilService(handler.auth) || !capability.Known() || next == nil {
		return nil, authport.ErrUnauthorized
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, authenticated := authport.PrincipalFromContext(request.Context())
		if !authenticated {
			next.ServeHTTP(writer, request)
			return
		}
		authorization, err := handler.auth.Authorize(request.Context(), principal, capability)
		if err != nil {
			code := platformhttp.CodeUnauthorized
			if !errors.Is(err, authport.ErrUnauthorized) {
				code = platformhttp.CodeDependencyUnavailable
			}
			platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
			return
		}
		ctx, err := authport.WithAuthorization(request.Context(), authorization)
		if err != nil {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, err))
			return
		}
		next.ServeHTTP(writer, request.WithContext(ctx))
	}), nil
}

// RequireCSRF validates the browser token against the current server-side
// session. It is mounted only on cookie-authenticated state-changing routes.
func (handler *Handler) RequireCSRF(next http.Handler) (http.Handler, error) {
	if handler == nil || nilService(handler.auth) || next == nil {
		return nil, authport.ErrUnauthorized
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session, ok := authport.SessionFromContext(request.Context())
		if !ok {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
			return
		}
		values := request.Header.Values("X-CSRF-Token")
		if len(values) != 1 {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrCSRFInvalid))
			return
		}
		if err := handler.auth.ValidateCSRF(request.Context(), session, authport.CSRFToken(values[0])); err != nil {
			code := platformhttp.CodeUnauthorized
			if errors.Is(err, authport.ErrUnauthenticated) {
				code = platformhttp.CodeUnauthenticated
			} else if errors.Is(err, authport.ErrAuthenticationUnavailable) {
				code = platformhttp.CodeDependencyUnavailable
			}
			platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
			return
		}
		next.ServeHTTP(writer, request)
	}), nil
}
