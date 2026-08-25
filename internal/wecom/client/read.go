package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ReaderConfig constructs the sole W3 business endpoint. It has no write or
// synchronization methods, and it does not persist returned data.
type ReaderConfig struct {
	BaseURL       string
	HTTPClient    *http.Client
	TokenProvider TokenProvider
}

// ExternalContactReader only exposes the paginated, read-only follow-user
// projection of /cgi-bin/externalcontact/get.
type ExternalContactReader struct {
	baseURL       *url.URL
	httpClient    *http.Client
	tokenProvider TokenProvider
}

func NewExternalContactReader(config ReaderConfig) (*ExternalContactReader, error) {
	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil || config.HTTPClient == nil || config.TokenProvider == nil {
		return nil, ErrInvalidConfig
	}
	return &ExternalContactReader{baseURL: baseURL, httpClient: config.HTTPClient, tokenProvider: config.TokenProvider}, nil
}

// FollowUserPage is a minimal cursor page. It deliberately omits contact
// profile fields and is not an Identity or Contact write model.
type FollowUserPage struct {
	UserIDs    []string
	NextCursor string
}

// ExternalContactPage is the minimal, read-only page returned by
// /cgi-bin/externalcontact/list. It deliberately carries no profile data and
// is not an Identity or Contact model.
type ExternalContactPage struct {
	ExternalUserIDs []string
	NextCursor      string
}

func (reader *ExternalContactReader) FollowUsers(ctx context.Context, externalUserID, cursor string) (FollowUserPage, error) {
	if reader == nil || externalUserID == "" {
		return FollowUserPage{}, ErrInvalidConfig
	}
	token, err := reader.tokenProvider.Token(ctx)
	if err != nil {
		return FollowUserPage{}, err
	}
	endpoint := reader.baseURL.ResolveReference(&url.URL{Path: "/cgi-bin/externalcontact/get"})
	query := url.Values{}
	query.Set("access_token", token.Value())
	query.Set("external_userid", externalUserID)
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return FollowUserPage{}, ErrInvalidConfig
	}
	response, err := reader.httpClient.Do(request)
	if err != nil {
		return FollowUserPage{}, mapRequestError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return FollowUserPage{}, fmt.Errorf("%w: status %d", ErrUnexpectedResponse, response.StatusCode)
	}
	var payload struct {
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
		FollowUser []struct {
			UserID string `json:"userid"`
		} `json:"follow_user"`
		NextCursor string `json:"next_cursor"`
	}
	if err = decodeResponse(response.Body, &payload); err != nil {
		return FollowUserPage{}, err
	}
	if payload.ErrCode != 0 {
		return FollowUserPage{}, apiError(payload.ErrCode, payload.ErrMsg)
	}
	page := FollowUserPage{NextCursor: payload.NextCursor, UserIDs: make([]string, 0, len(payload.FollowUser))}
	for _, user := range payload.FollowUser {
		if user.UserID == "" {
			return FollowUserPage{}, ErrUnexpectedResponse
		}
		page.UserIDs = append(page.UserIDs, user.UserID)
	}
	return page, nil
}

// ListExternalContacts reads one cursor page for one WeCom staff userid. It
// does not persist results; W4's transaction-bound state adapter owns cursor
// advancement separately.
func (reader *ExternalContactReader) ListExternalContacts(ctx context.Context, staffUserID, cursor string) (ExternalContactPage, error) {
	if reader == nil || staffUserID == "" {
		return ExternalContactPage{}, ErrInvalidConfig
	}
	token, err := reader.tokenProvider.Token(ctx)
	if err != nil {
		return ExternalContactPage{}, err
	}
	endpoint := reader.baseURL.ResolveReference(&url.URL{Path: "/cgi-bin/externalcontact/list"})
	query := url.Values{}
	query.Set("access_token", token.Value())
	query.Set("userid", staffUserID)
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ExternalContactPage{}, ErrInvalidConfig
	}
	response, err := reader.httpClient.Do(request)
	if err != nil {
		return ExternalContactPage{}, mapRequestError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ExternalContactPage{}, fmt.Errorf("%w: status %d", ErrUnexpectedResponse, response.StatusCode)
	}
	var payload struct {
		ErrCode         int      `json:"errcode"`
		ErrMsg          string   `json:"errmsg"`
		ExternalUserIDs []string `json:"external_userid"`
		NextCursor      string   `json:"next_cursor"`
	}
	if err = decodeResponse(response.Body, &payload); err != nil {
		return ExternalContactPage{}, err
	}
	if payload.ErrCode != 0 {
		return ExternalContactPage{}, apiError(payload.ErrCode, payload.ErrMsg)
	}
	seen := make(map[string]struct{}, len(payload.ExternalUserIDs))
	page := ExternalContactPage{NextCursor: payload.NextCursor, ExternalUserIDs: make([]string, 0, len(payload.ExternalUserIDs))}
	for _, externalUserID := range payload.ExternalUserIDs {
		if !validExternalContactUserID(externalUserID) {
			return ExternalContactPage{}, ErrUnexpectedResponse
		}
		if _, duplicate := seen[externalUserID]; duplicate {
			return ExternalContactPage{}, ErrUnexpectedResponse
		}
		seen[externalUserID] = struct{}{}
		page.ExternalUserIDs = append(page.ExternalUserIDs, externalUserID)
	}
	return page, nil
}

func validExternalContactUserID(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}
