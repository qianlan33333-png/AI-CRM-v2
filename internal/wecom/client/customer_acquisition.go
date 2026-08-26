package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrWriteOutcomeUnknown means a write may have reached WeCom, but the client
// cannot prove its result. Callers must reconcile it with a get operation; it
// is deliberately never retried here.
var ErrWriteOutcomeUnknown = errors.New("WeCom write outcome unknown")
var ErrBusinessWriteNotDispatched = errors.New("WeCom business write not dispatched")

// CustomerAcquisitionClient is the narrow provider boundary for CH02 contact
// ways and customer-acquisition links. Construction is inert: it does not
// obtain a token or call WeCom.
type CustomerAcquisitionClient struct {
	baseURL       *url.URL
	httpClient    *http.Client
	tokenProvider TokenProvider
}

type CustomerAcquisitionClientConfig struct {
	BaseURL       string
	HTTPClient    *http.Client
	TokenProvider TokenProvider
}

func NewCustomerAcquisitionClient(config CustomerAcquisitionClientConfig) (*CustomerAcquisitionClient, error) {
	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil || config.HTTPClient == nil || config.TokenProvider == nil {
		return nil, ErrInvalidConfig
	}
	return &CustomerAcquisitionClient{baseURL: baseURL, httpClient: config.HTTPClient, tokenProvider: config.TokenProvider}, nil
}

// ContactWayRequest is the minimal supported "contact me" publication. It
// intentionally excludes welcome-message payloads and other unrelated APIs.
type ContactWayRequest struct {
	Type       int
	Scene      int
	Remark     string
	SkipVerify bool
	State      string
	UserIDs    []string
	PartyIDs   []int64
}

type ContactWay struct {
	ConfigID   string
	QRCodeURL  string
	Type       int
	Scene      int
	UserIDs    []string
	PartyIDs   []int64
	SkipVerify bool
	State      string
}

type ContactWayPage struct {
	ContactWays []ContactWay
	NextCursor  string
}

// PublishContactWay creates one provider-side contact way. It does not retry
// after dispatch: a timeout, disconnect, non-200, or malformed response is an
// outcome that the domain must reconcile before any subsequent write.
func (client *CustomerAcquisitionClient) PublishContactWay(ctx context.Context, input ContactWayRequest) (ContactWay, error) {
	if client == nil || !validContactWayRequest(input) {
		return ContactWay{}, ErrInvalidConfig
	}
	var payload struct {
		ErrCode  int    `json:"errcode"`
		ErrMsg   string `json:"errmsg"`
		ConfigID string `json:"config_id"`
		QRCode   string `json:"qr_code"`
	}
	err := client.write(ctx, "/cgi-bin/externalcontact/add_contact_way", map[string]any{
		"type": input.Type, "scene": input.Scene, "remark": input.Remark, "skip_verify": input.SkipVerify,
		"state": input.State, "user": input.UserIDs, "party": input.PartyIDs,
	}, &payload)
	if err != nil {
		return ContactWay{}, err
	}
	if !validProviderID(payload.ConfigID) || input.Scene == 2 && !validOpaqueHTTPSURL(payload.QRCode) {
		return ContactWay{}, ErrWriteOutcomeUnknown
	}
	return ContactWay{ConfigID: payload.ConfigID, QRCodeURL: payload.QRCode, Type: input.Type, Scene: input.Scene, UserIDs: cloneStrings(input.UserIDs), PartyIDs: cloneInt64s(input.PartyIDs), SkipVerify: input.SkipVerify, State: input.State}, nil
}

func (client *CustomerAcquisitionClient) GetContactWay(ctx context.Context, configID string) (ContactWay, error) {
	if client == nil || !validProviderID(configID) {
		return ContactWay{}, ErrInvalidConfig
	}
	var payload struct {
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
		ContactWay struct {
			Type       int      `json:"type"`
			Scene      int      `json:"scene"`
			QRCode     string   `json:"qr_code"`
			SkipVerify bool     `json:"skip_verify"`
			State      string   `json:"state"`
			UserIDs    []string `json:"user"`
			PartyIDs   []int64  `json:"party"`
		} `json:"contact_way"`
	}
	if err := client.read(ctx, "/cgi-bin/externalcontact/get_contact_way", map[string]string{"config_id": configID}, &payload); err != nil {
		return ContactWay{}, err
	}
	value := ContactWay{ConfigID: configID, QRCodeURL: payload.ContactWay.QRCode, Type: payload.ContactWay.Type, Scene: payload.ContactWay.Scene, UserIDs: payload.ContactWay.UserIDs, PartyIDs: payload.ContactWay.PartyIDs, SkipVerify: payload.ContactWay.SkipVerify, State: payload.ContactWay.State}
	if !validContactWay(value, value.Scene == 2) {
		return ContactWay{}, ErrUnexpectedResponse
	}
	return value, nil
}

func (client *CustomerAcquisitionClient) ListContactWays(ctx context.Context, cursor string, limit int) (ContactWayPage, error) {
	if client == nil || !validCursor(cursor) || limit < 1 || limit > 1000 {
		return ContactWayPage{}, ErrInvalidConfig
	}
	var payload struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		ContactWays []struct {
			ConfigID string `json:"config_id"`
		} `json:"contact_way"`
		NextCursor string `json:"next_cursor"`
	}
	if err := client.read(ctx, "/cgi-bin/externalcontact/list_contact_way", map[string]any{"cursor": cursor, "limit": limit}, &payload); err != nil {
		return ContactWayPage{}, err
	}
	if !validCursor(payload.NextCursor) || payload.ContactWays == nil || len(payload.ContactWays) > limit {
		return ContactWayPage{}, ErrUnexpectedResponse
	}
	page := ContactWayPage{ContactWays: make([]ContactWay, 0, len(payload.ContactWays)), NextCursor: payload.NextCursor}
	seen := make(map[string]struct{}, len(payload.ContactWays))
	for _, item := range payload.ContactWays {
		if !validProviderID(item.ConfigID) {
			return ContactWayPage{}, ErrUnexpectedResponse
		}
		if _, duplicate := seen[item.ConfigID]; duplicate {
			return ContactWayPage{}, ErrUnexpectedResponse
		}
		seen[item.ConfigID] = struct{}{}
		page.ContactWays = append(page.ContactWays, ContactWay{ConfigID: item.ConfigID})
	}
	return page, nil
}

// ReconcileContactWay is an explicit name for a safe provider read used after
// an outcome-unknown publication.
func (client *CustomerAcquisitionClient) ReconcileContactWay(ctx context.Context, configID string) (ContactWay, error) {
	return client.GetContactWay(ctx, configID)
}

// ListFollowUsers reads the configured application's current external-contact
// service users. It is a read-only provider boundary: it neither creates local
// staff nor changes any channel assignment.
func (client *CustomerAcquisitionClient) ListFollowUsers(ctx context.Context) ([]string, error) {
	if client == nil {
		return nil, ErrInvalidConfig
	}
	var payload struct {
		ErrCode    int      `json:"errcode"`
		ErrMsg     string   `json:"errmsg"`
		FollowUser []string `json:"follow_user"`
	}
	if err := client.get(ctx, "/cgi-bin/externalcontact/get_follow_user_list", &payload); err != nil {
		return nil, err
	}
	if payload.FollowUser == nil {
		return nil, ErrUnexpectedResponse
	}
	seen := make(map[string]struct{}, len(payload.FollowUser))
	users := make([]string, 0, len(payload.FollowUser))
	for _, userID := range payload.FollowUser {
		if !validRequiredText(userID, 128) {
			return nil, ErrUnexpectedResponse
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		users = append(users, userID)
	}
	sort.Strings(users)
	return users, nil
}

type CustomerAcquisitionLinkRequest struct {
	LinkName      string
	UserIDs       []string
	DepartmentIDs []int64
	SkipVerify    bool
}

type CustomerAcquisitionLink struct {
	LinkID        string
	LinkName      string
	URL           string
	UserIDs       []string
	DepartmentIDs []int64
	SkipVerify    bool
}

type CustomerAcquisitionLinkPage struct {
	Links      []CustomerAcquisitionLink
	NextCursor string
}

func (client *CustomerAcquisitionClient) CreateCustomerAcquisitionLink(ctx context.Context, input CustomerAcquisitionLinkRequest) (CustomerAcquisitionLink, error) {
	if client == nil || !validCustomerAcquisitionLinkRequest(input) {
		return CustomerAcquisitionLink{}, ErrInvalidConfig
	}
	var payload struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		LinkID  string `json:"link_id"`
		URL     string `json:"url"`
	}
	err := client.write(ctx, "/cgi-bin/externalcontact/customer_acquisition/create_link", map[string]any{
		"link_name":   input.LinkName,
		"range":       map[string]any{"user_list": input.UserIDs, "department_list": input.DepartmentIDs},
		"skip_verify": input.SkipVerify,
	}, &payload)
	if err != nil {
		return CustomerAcquisitionLink{}, err
	}
	value := CustomerAcquisitionLink{LinkID: payload.LinkID, LinkName: input.LinkName, URL: payload.URL, UserIDs: cloneStrings(input.UserIDs), DepartmentIDs: cloneInt64s(input.DepartmentIDs), SkipVerify: input.SkipVerify}
	if !validCustomerAcquisitionLink(value, true) {
		return CustomerAcquisitionLink{}, ErrWriteOutcomeUnknown
	}
	return value, nil
}

func (client *CustomerAcquisitionClient) GetCustomerAcquisitionLink(ctx context.Context, linkID string) (CustomerAcquisitionLink, error) {
	if client == nil || !validProviderID(linkID) {
		return CustomerAcquisitionLink{}, ErrInvalidConfig
	}
	var payload struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Link    struct {
			LinkID     string `json:"link_id"`
			LinkName   string `json:"link_name"`
			URL        string `json:"url"`
			SkipVerify bool   `json:"skip_verify"`
			Range      struct {
				UserIDs       []string `json:"user_list"`
				DepartmentIDs []int64  `json:"department_list"`
			} `json:"range"`
		} `json:"link"`
	}
	if err := client.read(ctx, "/cgi-bin/externalcontact/customer_acquisition/get", map[string]string{"link_id": linkID}, &payload); err != nil {
		return CustomerAcquisitionLink{}, err
	}
	value := CustomerAcquisitionLink{LinkID: payload.Link.LinkID, LinkName: payload.Link.LinkName, URL: payload.Link.URL, UserIDs: payload.Link.Range.UserIDs, DepartmentIDs: payload.Link.Range.DepartmentIDs, SkipVerify: payload.Link.SkipVerify}
	if value.LinkID != linkID || !validCustomerAcquisitionLink(value, true) {
		return CustomerAcquisitionLink{}, ErrUnexpectedResponse
	}
	return value, nil
}

func (client *CustomerAcquisitionClient) ListCustomerAcquisitionLinks(ctx context.Context, cursor string, limit int) (CustomerAcquisitionLinkPage, error) {
	if client == nil || !validCursor(cursor) || limit < 1 || limit > 1000 {
		return CustomerAcquisitionLinkPage{}, ErrInvalidConfig
	}
	var payload struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Links   []struct {
			LinkID string `json:"link_id"`
		} `json:"link"`
		NextCursor string `json:"next_cursor"`
	}
	if err := client.read(ctx, "/cgi-bin/externalcontact/customer_acquisition/list_link", map[string]any{"cursor": cursor, "limit": limit}, &payload); err != nil {
		return CustomerAcquisitionLinkPage{}, err
	}
	if !validCursor(payload.NextCursor) || payload.Links == nil || len(payload.Links) > limit {
		return CustomerAcquisitionLinkPage{}, ErrUnexpectedResponse
	}
	page := CustomerAcquisitionLinkPage{Links: make([]CustomerAcquisitionLink, 0, len(payload.Links)), NextCursor: payload.NextCursor}
	seen := make(map[string]struct{}, len(payload.Links))
	for _, item := range payload.Links {
		if !validProviderID(item.LinkID) {
			return CustomerAcquisitionLinkPage{}, ErrUnexpectedResponse
		}
		if _, duplicate := seen[item.LinkID]; duplicate {
			return CustomerAcquisitionLinkPage{}, ErrUnexpectedResponse
		}
		seen[item.LinkID] = struct{}{}
		page.Links = append(page.Links, CustomerAcquisitionLink{LinkID: item.LinkID})
	}
	return page, nil
}

func (client *CustomerAcquisitionClient) ReconcileCustomerAcquisitionLink(ctx context.Context, linkID string) (CustomerAcquisitionLink, error) {
	return client.GetCustomerAcquisitionLink(ctx, linkID)
}

func (client *CustomerAcquisitionClient) read(ctx context.Context, path string, requestBody any, target any) error {
	return client.request(ctx, http.MethodPost, path, requestBody, target, false)
}

func (client *CustomerAcquisitionClient) write(ctx context.Context, path string, requestBody any, target any) error {
	return client.request(ctx, http.MethodPost, path, requestBody, target, true)
}

func (client *CustomerAcquisitionClient) get(ctx context.Context, path string, target any) error {
	return client.request(ctx, http.MethodGet, path, nil, target, false)
}

func (client *CustomerAcquisitionClient) request(ctx context.Context, method, path string, requestBody any, target any, write bool) error {
	if ctx == nil {
		return ErrInvalidConfig
	}
	token, err := client.tokenProvider.Token(ctx)
	if err != nil {
		if write {
			return fmt.Errorf("%w: %w", ErrBusinessWriteNotDispatched, err)
		}
		return err
	}
	retry := false
	for {
		code, err := client.requestWithToken(ctx, method, path, requestBody, token, target, write)
		if err != nil {
			return err
		}
		// A write has already crossed the provider boundary before its 42001
		// response arrives. Retrying it with a refreshed token could create a
		// second link/contact-way, so it is always reconciled as unknown.
		if code == 42001 && write {
			return ErrWriteOutcomeUnknown
		}
		if code != 42001 || retry {
			if code != 0 {
				return apiError(code, "WeCom rejected request")
			}
			return nil
		}
		refresher, ok := client.tokenProvider.(interface {
			RefreshToken(context.Context) (AccessToken, error)
		})
		if !ok {
			return apiError(code, "WeCom rejected request")
		}
		token, err = refresher.RefreshToken(ctx)
		if err != nil {
			return err
		}
		retry = true
	}
}

func (client *CustomerAcquisitionClient) requestWithToken(ctx context.Context, method, path string, requestBody any, token AccessToken, target any, write bool) (int, error) {
	var body io.Reader
	if method == http.MethodPost {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return 0, ErrInvalidConfig
		}
		body = bytes.NewReader(encoded)
	} else if method != http.MethodGet || requestBody != nil {
		return 0, ErrInvalidConfig
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: path})
	query := url.Values{}
	query.Set("access_token", token.Value())
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return 0, ErrInvalidConfig
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		if write {
			return 0, unknownWriteError(err)
		}
		return 0, safeReadRequestError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if write {
			return 0, ErrWriteOutcomeUnknown
		}
		return 0, fmt.Errorf("%w: status %d", ErrUnexpectedResponse, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		if write {
			return 0, ErrWriteOutcomeUnknown
		}
		return 0, ErrUnexpectedResponse
	}
	var envelope struct {
		ErrCode int `json:"errcode"`
	}
	if !decodeSingleJSON(data, &envelope) || !decodeSingleJSON(data, target) {
		if write {
			return 0, ErrWriteOutcomeUnknown
		}
		return 0, ErrUnexpectedResponse
	}
	return envelope.ErrCode, nil
}

func decodeSingleJSON(data []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}

func unknownWriteError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrWriteOutcomeUnknown, ErrRequestTimeout)
	}
	return fmt.Errorf("%w: %w", ErrWriteOutcomeUnknown, ErrTransport)
}

func safeReadRequestError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrRequestTimeout, context.DeadlineExceeded)
	}
	return ErrTransport
}

func validContactWayRequest(value ContactWayRequest) bool {
	return (value.Type == 1 || value.Type == 2) && (value.Scene == 1 || value.Scene == 2) && validOptionalText(value.Remark, 120) && validOptionalText(value.State, 120) && validStringSlice(value.UserIDs, 0, 500) && validPositiveInt64Slice(value.PartyIDs, 500) && (value.Type != 1 || len(value.UserIDs) == 1 && len(value.PartyIDs) == 0) && (value.Type != 2 || len(value.UserIDs)+len(value.PartyIDs) > 0)
}

func validContactWay(value ContactWay, requireURL bool) bool {
	return validProviderID(value.ConfigID) && (value.Type == 1 || value.Type == 2) && (value.Scene == 1 || value.Scene == 2) && validOptionalText(value.State, 120) && validStringSlice(value.UserIDs, 0, 500) && validPositiveInt64Slice(value.PartyIDs, 500) && (value.Type != 1 || len(value.UserIDs) == 1 && len(value.PartyIDs) == 0) && (value.Type != 2 || len(value.UserIDs)+len(value.PartyIDs) > 0) && (!requireURL || validOpaqueHTTPSURL(value.QRCodeURL)) && (value.QRCodeURL == "" || validOpaqueHTTPSURL(value.QRCodeURL))
}

func validCustomerAcquisitionLinkRequest(value CustomerAcquisitionLinkRequest) bool {
	return validRequiredText(value.LinkName, 120) && validStringSlice(value.UserIDs, 0, 500) && validPositiveInt64Slice(value.DepartmentIDs, 500) && len(value.UserIDs)+len(value.DepartmentIDs) > 0
}

func validCustomerAcquisitionLink(value CustomerAcquisitionLink, requireURL bool) bool {
	return validProviderID(value.LinkID) && validRequiredText(value.LinkName, 120) && validStringSlice(value.UserIDs, 0, 500) && validPositiveInt64Slice(value.DepartmentIDs, 500) && len(value.UserIDs)+len(value.DepartmentIDs) > 0 && (!requireURL || validOpaqueHTTPSURL(value.URL)) && (value.URL == "" || validOpaqueHTTPSURL(value.URL))
}

func validProviderID(value string) bool { return validRequiredText(value, 1024) }
func validCursor(value string) bool     { return validOptionalText(value, 1024) }

func validRequiredText(value string, limit int) bool {
	return value != "" && validOptionalText(value, limit)
}

func validOptionalText(value string, limit int) bool {
	return len(value) <= limit && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validStringSlice(values []string, min, max int) bool {
	if len(values) < min || len(values) > max {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validProviderID(value) {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validPositiveInt64Slice(values []int64, max int) bool {
	if len(values) > max {
		return false
	}
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value < 1 {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validOpaqueHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == "" && strings.TrimSpace(value) == value
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }
func cloneInt64s(values []int64) []int64    { return append([]int64(nil), values...) }

// DisabledCustomerAcquisitionClient is the safe default before controlled
// provider composition. Every method returns before token acquisition or HTTP.
type DisabledCustomerAcquisitionClient struct{}

var ErrCustomerAcquisitionDisabled = errors.New("WeCom customer acquisition is disabled")

func NewDisabledCustomerAcquisitionClient() *DisabledCustomerAcquisitionClient {
	return &DisabledCustomerAcquisitionClient{}
}

func (*DisabledCustomerAcquisitionClient) PublishContactWay(context.Context, ContactWayRequest) (ContactWay, error) {
	return ContactWay{}, ErrCustomerAcquisitionDisabled
}
func (*DisabledCustomerAcquisitionClient) GetContactWay(context.Context, string) (ContactWay, error) {
	return ContactWay{}, ErrCustomerAcquisitionDisabled
}
func (*DisabledCustomerAcquisitionClient) ListContactWays(context.Context, string, int) (ContactWayPage, error) {
	return ContactWayPage{}, ErrCustomerAcquisitionDisabled
}
func (*DisabledCustomerAcquisitionClient) ReconcileContactWay(context.Context, string) (ContactWay, error) {
	return ContactWay{}, ErrCustomerAcquisitionDisabled
}
func (*DisabledCustomerAcquisitionClient) ListFollowUsers(context.Context) ([]string, error) {
	return nil, ErrCustomerAcquisitionDisabled
}
func (*DisabledCustomerAcquisitionClient) CreateCustomerAcquisitionLink(context.Context, CustomerAcquisitionLinkRequest) (CustomerAcquisitionLink, error) {
	return CustomerAcquisitionLink{}, ErrCustomerAcquisitionDisabled
}
func (*DisabledCustomerAcquisitionClient) GetCustomerAcquisitionLink(context.Context, string) (CustomerAcquisitionLink, error) {
	return CustomerAcquisitionLink{}, ErrCustomerAcquisitionDisabled
}
func (*DisabledCustomerAcquisitionClient) ListCustomerAcquisitionLinks(context.Context, string, int) (CustomerAcquisitionLinkPage, error) {
	return CustomerAcquisitionLinkPage{}, ErrCustomerAcquisitionDisabled
}
func (*DisabledCustomerAcquisitionClient) ReconcileCustomerAcquisitionLink(context.Context, string) (CustomerAcquisitionLink, error) {
	return CustomerAcquisitionLink{}, ErrCustomerAcquisitionDisabled
}
