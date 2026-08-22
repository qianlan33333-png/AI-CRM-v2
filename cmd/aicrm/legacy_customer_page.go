package main

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	legacyCustomerListPagePath             = "/admin/customers"
	legacyCustomerDetailPagePattern        = "/admin/customers/{customer_id}"
	legacyCustomerContextPageRoot          = "/admin/customer-360"
	legacyCustomerContextPagePattern       = "/admin/customer-360/{customer_id}"
	legacyCustomerMaxSafeID          int64 = 9007199254740991
)

type legacyCustomerPageKind uint8

const (
	legacyCustomerPageUnknown legacyCustomerPageKind = iota
	legacyCustomerPageList
	legacyCustomerPageDetail
	legacyCustomerPageContext
)

type legacyCustomerPageRoute struct {
	kind       legacyCustomerPageKind
	pathname   string
	customerID int64
	legacyKey  string
}

func parseLegacyCustomerPagePath(pathname string) (legacyCustomerPageRoute, bool) {
	if pathname == legacyCustomerListPagePath {
		return legacyCustomerPageRoute{kind: legacyCustomerPageList, pathname: pathname}, true
	}

	for _, candidate := range []struct {
		prefix string
		kind   legacyCustomerPageKind
	}{
		{prefix: legacyCustomerListPagePath + "/", kind: legacyCustomerPageDetail},
		{prefix: legacyCustomerContextPageRoot + "/", kind: legacyCustomerPageContext},
	} {
		if !strings.HasPrefix(pathname, candidate.prefix) {
			continue
		}
		rawID := strings.TrimPrefix(pathname, candidate.prefix)
		if rawID == "" || len(rawID) > 1024 || strings.ContainsAny(rawID, "/\\") ||
			strings.TrimSpace(rawID) != rawID || !utf8.ValidString(rawID) || strings.IndexFunc(rawID, unicode.IsControl) >= 0 {
			return legacyCustomerPageRoute{}, false
		}
		allDigits := true
		for index := range rawID {
			if rawID[index] < '0' || rawID[index] > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			if candidate.kind != legacyCustomerPageDetail || rawID[0] == '-' || rawID[0] == '+' {
				return legacyCustomerPageRoute{}, false
			}
			return legacyCustomerPageRoute{kind: candidate.kind, pathname: pathname, legacyKey: rawID}, true
		}
		if rawID[0] == '0' {
			return legacyCustomerPageRoute{}, false
		}
		customerID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || customerID < 1 || customerID > legacyCustomerMaxSafeID {
			return legacyCustomerPageRoute{}, false
		}
		return legacyCustomerPageRoute{
			kind:       candidate.kind,
			pathname:   pathname,
			customerID: customerID,
		}, true
	}

	return legacyCustomerPageRoute{}, false
}

func legacyCustomerPageRouteForRequest(request *http.Request) (legacyCustomerPageRoute, bool) {
	if request == nil || request.URL == nil {
		return legacyCustomerPageRoute{}, false
	}
	if request.URL.RawQuery != "" || request.URL.ForceQuery || request.URL.RawPath != "" {
		return legacyCustomerPageRoute{}, false
	}
	pathname := request.URL.Path
	if pathname == "" || pathname != request.URL.EscapedPath() {
		return legacyCustomerPageRoute{}, false
	}
	return parseLegacyCustomerPagePath(pathname)
}

func legacyCustomerPageAuthorized(request *http.Request, capability authport.Capability) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 &&
		authorizationOK && authorization.Capability == capability
}

func serveLegacyCustomerPage(
	writer http.ResponseWriter,
	request *http.Request,
	kind legacyCustomerPageKind,
	capability authport.Capability,
) {
	setLegacyCustomerPageHeaders(writer)
	if !legacyCustomerPageAuthorized(request, capability) {
		platformhttp.WriteError(
			writer,
			request,
			platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized),
		)
		return
	}
	route, ok := legacyCustomerPageRouteForRequest(request)
	if !ok || route.kind != kind {
		writeLegacyCustomerPageNotFoundResponse(writer)
		return
	}
	http.Redirect(
		writer,
		request,
		"/?legacy_admin_path="+url.QueryEscape(route.pathname),
		http.StatusFound,
	)
}

// CustomerListPage is an authenticated HTML carrier only. The existing Web
// shell and existing CustomerListPage own all rendering and safe data reads.
func (handler *Handler) CustomerListPage(writer http.ResponseWriter, request *http.Request) {
	serveLegacyCustomerPage(writer, request, legacyCustomerPageList, authport.CapabilityCustomersRead)
}

// CustomerDetailPage keeps numeric OneID carriers unchanged. A safe legacy
// unionid is resolved only after authorization and redirected without ever
// reflecting the raw key.
func (handler *Handler) CustomerDetailPage(writer http.ResponseWriter, request *http.Request) {
	setLegacyCustomerPageHeaders(writer)
	if !legacyCustomerPageAuthorized(request, authport.CapabilityCustomersRead) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	route, ok := legacyCustomerPageRouteForRequest(request)
	if !ok || route.kind != legacyCustomerPageDetail {
		writeLegacyCustomerPageNotFoundResponse(writer)
		return
	}
	if route.legacyKey == "" {
		http.Redirect(writer, request, "/?legacy_admin_path="+url.QueryEscape(route.pathname), http.StatusFound)
		return
	}
	if handler == nil || nilLegacyDependency(handler.messageArchiveUnionID) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrMessageArchiveUnionLookupFailed))
		return
	}
	result, err := handler.messageArchiveUnionID.ResolveUnionID(request.Context(), route.legacyKey)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrMessageArchiveUnionLookupFailed))
		return
	}
	switch result.Status {
	case identityport.ResolveNotFound:
		if result.CustomerID != 0 {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrMessageArchiveUnionLookupFailed))
			return
		}
		writeLegacyCustomerPageNotFoundResponse(writer)
	case identityport.ResolveFound:
		if result.CustomerID <= 0 || int64(result.CustomerID) > legacyCustomerMaxSafeID {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrMessageArchiveUnionLookupFailed))
			return
		}
		http.Redirect(writer, request, legacyCustomerListPagePath+"/"+strconv.FormatInt(int64(result.CustomerID), 10), http.StatusFound)
	default:
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrMessageArchiveUnionLookupFailed))
	}
}

// CustomerContextPage is an authenticated HTML carrier only. The Web shell
// reuses the existing Customer 360 safe-read panel and its existing API.
func (handler *Handler) CustomerContextPage(writer http.ResponseWriter, request *http.Request) {
	serveLegacyCustomerPage(writer, request, legacyCustomerPageContext, authport.CapabilityCustomerEventsRead)
}

func legacyCustomerPageKindForPattern(pattern string) (legacyCustomerPageKind, bool) {
	switch pattern {
	case legacyCustomerListPagePath:
		return legacyCustomerPageList, true
	case legacyCustomerDetailPagePattern:
		return legacyCustomerPageDetail, true
	case legacyCustomerContextPagePattern:
		return legacyCustomerPageContext, true
	default:
		return legacyCustomerPageUnknown, false
	}
}

func isLegacyCustomerPagePattern(pattern string) bool {
	_, ok := legacyCustomerPageKindForPattern(pattern)
	return ok
}

// legacyCustomerPageRouteGuard is installed outside authentication. Malformed
// customer-shaped paths therefore receive one fixed response before any
// session, authorization, budget, or application handler can observe them.
func legacyCustomerPageRouteGuard(pattern string, next http.Handler) http.Handler {
	kind, known := legacyCustomerPageKindForPattern(pattern)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		route, ok := legacyCustomerPageRouteForRequest(request)
		if !known || !ok || route.kind != kind {
			writeLegacyCustomerPageNotFoundResponse(writer)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func setLegacyCustomerPageHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func legacyCustomerPageSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setLegacyCustomerPageHeaders(writer)
		next.ServeHTTP(writer, request)
	})
}

// legacyCustomerPageNamespaceGuard runs before Chi route matching. Chi can
// reject encoded or backslash-containing paths before its NotFound handler,
// so keep the customer alias namespace fail-closed at the outer boundary.
func legacyCustomerPageNamespaceGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request != nil && request.URL != nil &&
			(legacyCustomerPageNamespace(request.URL.Path) ||
				legacyCustomerPageNamespace(request.URL.EscapedPath())) {
			if _, ok := legacyCustomerPageRouteForRequest(request); !ok {
				writeLegacyCustomerPageNotFoundResponse(writer)
				return
			}
		}
		next.ServeHTTP(writer, request)
	})
}

func writeLegacyCustomerPageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	setLegacyCustomerPageHeaders(writer)
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func rawLegacyCustomerPageNamespace(pathname string) bool {
	for _, root := range []string{legacyCustomerListPagePath, legacyCustomerContextPageRoot} {
		backslashRoot := "/" + strings.ReplaceAll(strings.TrimPrefix(root, "/"), "/", "\\")
		for _, form := range []string{root, backslashRoot} {
			if pathname == form || strings.HasPrefix(pathname, form+"/") || strings.HasPrefix(pathname, form+"\\") {
				return true
			}
		}
	}
	return false
}

func legacyCustomerPageNamespace(pathname string) bool {
	candidate := pathname
	for pass := 0; pass < 3; pass++ {
		if rawLegacyCustomerPageNamespace(candidate) {
			return true
		}
		decoded, err := url.PathUnescape(candidate)
		if err != nil || decoded == candidate {
			return false
		}
		candidate = decoded
	}
	return false
}

func writeLegacyCustomerPageNotFound(writer http.ResponseWriter, request *http.Request) bool {
	if request == nil || request.URL == nil {
		return false
	}
	if !legacyCustomerPageNamespace(request.URL.Path) && !legacyCustomerPageNamespace(request.URL.EscapedPath()) {
		return false
	}
	writeLegacyCustomerPageNotFoundResponse(writer)
	return true
}

func writeLegacyCustomerPageNotFoundResponse(writer http.ResponseWriter) {
	setLegacyCustomerPageHeaders(writer)
	writer.Header().Set(
		"Content-Security-Policy",
		"default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
	)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusNotFound)
	_, _ = writer.Write([]byte(
		"<!doctype html><html lang=\"zh-CN\"><meta charset=\"utf-8\">" +
			"<title>客户页面不存在</title><main><h1>客户页面不存在</h1>" +
			"<p>该地址不是已冻结的客户安全读取路由。</p><a href=\"" +
			html.EscapeString(legacyCustomerListPagePath) +
			"\">返回客户列表</a></main></html>",
	))
}
