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
