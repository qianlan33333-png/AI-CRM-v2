package http

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	AlipayTransactionsPath          = "/admin/alipay/transactions"
	ServiceProductsPath             = "/admin/service-period-products"
	ServiceProductNewPath           = "/admin/service-period-products/new"
	ServiceProductEditPattern       = "/admin/service-period-products/{service_product_id}/edit"
	ServiceProductDataPattern       = "/admin/service-period-products/{service_product_id}/data"
	WeChatPayProductsPath           = "/admin/wechat-pay/products"
	WeChatPayProductNewPath         = "/admin/wechat-pay/products/new"
	WeChatPayProductEditPattern     = "/admin/wechat-pay/products/{product_id}/edit"
	WeChatPayTransactionsPath       = "/admin/wechat-pay/transactions"
	WeChatPayTransactionPattern     = "/admin/wechat-pay/transactions/{order_id}"
	WeChatShopTransactionsPath      = "/admin/wechat-shop/transactions"
	WeChatShopTransactionPattern    = "/admin/wechat-shop/transactions/{order_id}"
	legacyAdminPathParameter        = "legacy_admin_path"
	maximumCommerceIdentifierLength = 200
)

var errCommerceWorkspaceNotFound = errors.New("commerce workspace not found")

// WorkspacePages carries only the eleven not-yet-owned commerce administration
// routes into the same-origin SPA. It owns no product, member, payment,
// refund, provider, or external-effect contract and therefore cannot infer or
// mutate any of those facts.
type WorkspacePages struct{}

func NewWorkspacePages() *WorkspacePages { return &WorkspacePages{} }

func (handler *WorkspacePages) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setWorkspacePageHeaders(writer)
	if request == nil {
		return
	}
	target, matched := workspacePageTarget(request.URL.Path)
	if !matched {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, errCommerceWorkspaceNotFound))
		return
	}
	if request.Method != http.MethodGet {
		WriteWorkspacePageMethodNotAllowed(writer, request)
		return
	}
	if !workspacePageAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	http.Redirect(writer, request, "/?"+legacyAdminPathParameter+"="+url.QueryEscape(target), http.StatusFound)
}

func IsWorkspacePagePattern(pattern string) bool {
	switch pattern {
	case AlipayTransactionsPath,
		ServiceProductsPath,
		ServiceProductNewPath,
		ServiceProductEditPattern,
		ServiceProductDataPattern,
		WeChatPayProductNewPath,
		WeChatPayProductEditPattern,
		WeChatPayTransactionsPath,
		WeChatPayTransactionPattern,
		WeChatShopTransactionsPath,
		WeChatShopTransactionPattern:
		return true
	default:
		return false
	}
}

func workspacePageTarget(path string) (string, bool) {
	switch path {
	case AlipayTransactionsPath,
		ServiceProductsPath,
		ServiceProductNewPath,
		WeChatPayProductNewPath,
		WeChatPayTransactionsPath,
		WeChatShopTransactionsPath:
		return path, true
	}
	for _, route := range []struct {
		prefix string
		suffix string
	}{
		{ServiceProductsPath + "/", "/edit"},
		{ServiceProductsPath + "/", "/data"},
		{WeChatPayProductsPath + "/", "/edit"},
		{WeChatPayTransactionsPath + "/", ""},
		{WeChatShopTransactionsPath + "/", ""},
	} {
		if !strings.HasPrefix(path, route.prefix) || !strings.HasSuffix(path, route.suffix) {
			continue
		}
		identifier := strings.TrimSuffix(strings.TrimPrefix(path, route.prefix), route.suffix)
		if safeCommerceIdentifier(identifier) {
			return path, true
		}
	}
	return "", false
}

func safeCommerceIdentifier(value string) bool {
	if value == "" || value == "." || value == ".." || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > maximumCommerceIdentifierLength || strings.ContainsAny(value, "/\\") {
		return false
	}
	for _, character := range value {
		if character == '%' || character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func workspacePageAuthorized(request *http.Request) bool {
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin &&
		authorizationOK && authorization.Capability == authport.CapabilityAdminRead &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func setWorkspacePageHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func WorkspacePageSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setWorkspacePageHeaders(writer)
		if next != nil {
			next.ServeHTTP(writer, request)
		}
	})
}

func WriteWorkspacePageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	setWorkspacePageHeaders(writer)
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
