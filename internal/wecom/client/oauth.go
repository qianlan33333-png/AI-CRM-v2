package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

const (
	ProductionBaseURL      = "https://qyapi.weixin.qq.com"
	ProductionAuthorizeURL = "https://open.weixin.qq.com/connect/oauth2/authorize"
	ProductionDesktopURL   = "https://login.work.weixin.qq.com/wwlogin/sso/login"
)

type HumanOAuthConfig struct {
	BaseURL       string
	AuthorizeURL  string
	CallbackURL   string
	CallbackPath  string
	CorpID        CorpID
	HTTPClient    *http.Client
	TokenProvider TokenProvider
	// DesktopAgentID selects the administrator Web login flow; zero preserves
	// the existing in-client OAuth flow used by Sidebar.
	DesktopAgentID int64
}

// HumanOAuthClient is the narrow provider adapter used by the admin browser
// login. It exchanges a one-time provider code for one enterprise member
// userid; it owns no session, RBAC, redirect, or persistence behavior.
type HumanOAuthClient struct {
	baseURL        *url.URL
	authorizeURL   *url.URL
	callbackURL    string
	corpID         CorpID
	desktopAgentID int64
	httpClient     *http.Client
	tokenProvider  TokenProvider
}

type HumanIdentity struct {
	CorpID CorpID
	UserID string
}

func NewHumanOAuthClient(config HumanOAuthConfig) (*HumanOAuthClient, error) {
	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil || config.HTTPClient == nil || config.TokenProvider == nil || !validCorpID(string(config.CorpID)) || config.DesktopAgentID < 0 {
		return nil, ErrInvalidConfig
	}
	authorizeURL, err := url.Parse(config.AuthorizeURL)
	if err != nil || authorizeURL.Scheme == "" || authorizeURL.Host == "" || authorizeURL.User != nil || authorizeURL.RawQuery != "" || authorizeURL.Fragment != "" {
		return nil, ErrInvalidConfig
	}
	callbackPath := config.CallbackPath
	if callbackPath == "" {
		callbackPath = "/auth/wecom/callback"
	}
	callbackURL, err := url.Parse(config.CallbackURL)
	if err != nil || callbackURL.Scheme != "https" || callbackURL.Host == "" || callbackURL.User != nil || callbackURL.RawQuery != "" || callbackURL.Fragment != "" || callbackURL.Path != callbackPath || !validCallbackPath(callbackPath) {
		return nil, ErrInvalidConfig
	}
	if config.DesktopAgentID > 0 && callbackPath != "/auth/wecom/callback" {
		return nil, ErrInvalidConfig
	}
	return &HumanOAuthClient{
		baseURL: baseURL, authorizeURL: authorizeURL, callbackURL: callbackURL.String(), corpID: config.CorpID, desktopAgentID: config.DesktopAgentID,
		httpClient: config.HTTPClient, tokenProvider: config.TokenProvider,
	}, nil
}

func validCallbackPath(value string) bool {
	return value == "/auth/wecom/callback" || value == "/api/sidebar/v2/oauth/callback"
}

func (client *HumanOAuthClient) CorpID() string {
	if client == nil {
		return ""
	}
	return string(client.corpID)
}

func (client *HumanOAuthClient) AuthorizationURL(state string) (string, error) {
	if client == nil || !validOAuthState(state) {
		return "", ErrInvalidConfig
	}
	if client.desktopAgentID > 0 {
		endpoint, err := url.Parse(ProductionDesktopURL)
		if err != nil {
			return "", ErrInvalidConfig
		}
		query := url.Values{}
		query.Set("login_type", "CorpApp")
		query.Set("appid", string(client.corpID))
		query.Set("agentid", strconv.FormatInt(client.desktopAgentID, 10))
		query.Set("redirect_uri", client.callbackURL)
		query.Set("state", state)
		endpoint.RawQuery = query.Encode()
		return endpoint.String(), nil
	}
	endpoint := *client.authorizeURL
	query := url.Values{}
	query.Set("appid", string(client.corpID))
	query.Set("redirect_uri", client.callbackURL)
	query.Set("response_type", "code")
	query.Set("scope", "snsapi_base")
	query.Set("state", state)
	endpoint.RawQuery = query.Encode()
	endpoint.Fragment = "wechat_redirect"
	return endpoint.String(), nil
}

func (client *HumanOAuthClient) Exchange(ctx context.Context, code string) (HumanIdentity, error) {
	if client == nil || !validOAuthCode(code) {
		return HumanIdentity{}, ErrInvalidConfig
	}
	token, err := client.tokenProvider.Token(ctx)
	if err != nil {
		return HumanIdentity{}, err
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: "/cgi-bin/auth/getuserinfo"})
	query := url.Values{}
	query.Set("access_token", token.Value())
	query.Set("code", code)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return HumanIdentity{}, ErrInvalidConfig
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return HumanIdentity{}, mapRequestError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return HumanIdentity{}, fmt.Errorf("%w: status %d", ErrUnexpectedResponse, response.StatusCode)
	}
	var payload struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		UserID  string `json:"userid"`
		OpenID  string `json:"openid"`
	}
	if err = decodeResponse(response.Body, &payload); err != nil {
		return HumanIdentity{}, err
	}
	if payload.ErrCode != 0 {
		return HumanIdentity{}, apiError(payload.ErrCode, payload.ErrMsg)
	}
	// A public OpenID without an enterprise userid is not an authorized human
	// admin identity and must never be promoted into an internal session.
	if !validProviderSubject(payload.UserID) {
		return HumanIdentity{}, ErrUnexpectedResponse
	}
	return HumanIdentity{CorpID: client.corpID, UserID: payload.UserID}, nil
}

func validOAuthState(value string) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(value) == 43 && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func validOAuthCode(value string) bool {
	if len(value) < 1 || len(value) > 512 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validCorpID(value string) bool {
	return validProviderSubject(value)
}

func validProviderSubject(value string) bool {
	if len(value) < 1 || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-' || character == '.' || character == ':' || character == '@') {
			return false
		}
	}
	return true
}
