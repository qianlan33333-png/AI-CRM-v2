// Package groupopsdirectory provides the read-only WeCom directory projection
// used by Group Ops. It owns no Group Ops state and never creates staff.
package groupopsdirectory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

const maxResponseBytes = 1 << 20

var (
	ErrInvalidConfig      = errors.New("invalid WeCom Group Ops directory configuration")
	ErrUnexpectedResponse = errors.New("invalid WeCom Group Ops directory response")
	ErrUpstream           = errors.New("WeCom Group Ops directory API rejected request")
)

// OwnerStaffResolver resolves one active CRM staff id to exactly one WeCom
// userid. The application supplies this narrow adapter; this package never
// creates or updates staff records.
type OwnerStaffResolver interface {
	ResolveActiveWeComUserID(context.Context, int64) (string, error)
}

// ActiveStaffDirectory is the read-only Contact projection used to constrain
// selectable Group Ops senders to existing active local staff.
type ActiveStaffDirectory interface {
	ListActiveWeComStaff(context.Context) ([]ActiveStaff, error)
}

type ActiveStaff struct {
	WeComUserID string
	DisplayName string
}

// TokenProvider is implemented by wecomclient.CachingTokenProvider. Refresh
// is intentionally available only for documented expired-token responses.
type TokenProvider interface {
	Token(context.Context) (wecomclient.AccessToken, error)
	RefreshToken(context.Context) (wecomclient.AccessToken, error)
}

type Config struct {
	BaseURL     string
	HTTPClient  *http.Client
	Token       TokenProvider
	OwnerStaff  OwnerStaffResolver
	ActiveStaff ActiveStaffDirectory
}

// Source implements the Group Ops read-only directory port. Construction does
// not make a WeCom request; calls happen only through the port methods.
type Source struct {
	baseURL     *url.URL
	httpClient  *http.Client
	token       TokenProvider
	ownerStaff  OwnerStaffResolver
	activeStaff ActiveStaffDirectory
}

var _ groupopsport.GroupDirectorySource = (*Source)(nil)

func New(config Config) (*Source, error) {
	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil || config.HTTPClient == nil || config.Token == nil || config.OwnerStaff == nil || config.ActiveStaff == nil {
		return nil, ErrInvalidConfig
	}
	return &Source{baseURL: baseURL, httpClient: config.HTTPClient, token: config.Token, ownerStaff: config.OwnerStaff, activeStaff: config.ActiveStaff}, nil
}

func (source *Source) ListOwnedGroups(ctx context.Context, ownerStaffID int64, limit int32) (groupopsport.GroupDirectorySnapshot, error) {
	if source == nil || ctx == nil || ownerStaffID < 1 || limit < 1 || limit > 1000 {
		return groupopsport.GroupDirectorySnapshot{}, ErrInvalidConfig
	}
	ownerUserID, err := source.ownerStaff.ResolveActiveWeComUserID(ctx, ownerStaffID)
	if err != nil || !validText(ownerUserID, 128) {
		return groupopsport.GroupDirectorySnapshot{}, ErrUnexpectedResponse
	}
	chatIDs, err := source.listGroupChatIDs(ctx, ownerUserID, limit)
	if err != nil {
		return groupopsport.GroupDirectorySnapshot{}, err
	}
	items := make([]groupopsport.GroupDirectoryItem, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		group, getErr := source.getGroupChat(ctx, chatID)
		if getErr != nil {
			return groupopsport.GroupDirectorySnapshot{}, getErr
		}
		if group.ChatID != chatID || group.Owner != ownerUserID || !validText(group.Name, 128) || !validMembers(group.MemberCount) {
			return groupopsport.GroupDirectorySnapshot{}, ErrUnexpectedResponse
		}
		items = append(items, groupopsport.GroupDirectoryItem{ChatReference: chatID, OwnerStaffID: ownerStaffID, DisplayName: group.Name, MemberCount: group.MemberCount})
	}
	return groupopsport.GroupDirectorySnapshot{Items: items, Complete: true}, nil
}

func (source *Source) RefreshOperationMembers(ctx context.Context, pageSize int32) ([]groupopsport.OperationMember, error) {
	if source == nil || ctx == nil || pageSize < 1 || pageSize > 100 {
		return nil, ErrInvalidConfig
	}
	var response struct {
		FollowUser []string `json:"follow_user"`
	}
	if err := source.get(ctx, "/cgi-bin/externalcontact/get_follow_user_list", &response); err != nil {
		return nil, err
	}
	followers := make(map[string]struct{}, len(response.FollowUser))
	for _, userID := range response.FollowUser {
		if !validText(userID, 128) {
			return nil, ErrUnexpectedResponse
		}
		followers[userID] = struct{}{}
	}
	staff, err := source.activeStaff.ListActiveWeComStaff(ctx)
	if err != nil {
		return nil, err
	}
	byUserID := make(map[string]string, len(staff))
	for _, entry := range staff {
		if !validText(entry.WeComUserID, 128) || !validText(entry.DisplayName, 128) {
			return nil, ErrUnexpectedResponse
		}
		if existing, found := byUserID[entry.WeComUserID]; found && existing != entry.DisplayName {
			return nil, ErrUnexpectedResponse
		}
		byUserID[entry.WeComUserID] = entry.DisplayName
	}
	userIDs := make([]string, 0, len(byUserID))
	for userID := range byUserID {
		if _, follows := followers[userID]; follows {
			userIDs = append(userIDs, userID)
		}
	}
	sort.Strings(userIDs)
	if len(userIDs) > int(pageSize) {
		userIDs = userIDs[:pageSize]
	}
	items := make([]groupopsport.OperationMember, 0, len(userIDs))
	for _, userID := range userIDs {
		items = append(items, groupopsport.OperationMember{SenderUserID: userID, DisplayName: byUserID[userID]})
	}
	return items, nil
}

func (source *Source) listGroupChatIDs(ctx context.Context, ownerUserID string, limit int32) ([]string, error) {
	chatIDs := []string{}
	seenChatIDs := map[string]struct{}{}
	seenCursors := map[string]struct{}{}
	for cursor := ""; ; {
		if _, duplicate := seenCursors[cursor]; duplicate {
			return nil, ErrUnexpectedResponse
		}
		seenCursors[cursor] = struct{}{}
		var response struct {
			GroupChatList []struct {
				ChatID string `json:"chat_id"`
			} `json:"group_chat_list"`
			NextCursor string `json:"next_cursor"`
		}
		payload := map[string]any{
			"owner_filter":  map[string][]string{"userid_list": []string{ownerUserID}},
			"status_filter": 0,
			"cursor":        cursor,
			"limit":         limit,
		}
		if err := source.post(ctx, "/cgi-bin/externalcontact/groupchat/list", payload, &response); err != nil {
			return nil, err
		}
		if !validCursor(response.NextCursor) || len(response.GroupChatList) > int(limit) {
			return nil, ErrUnexpectedResponse
		}
		for _, item := range response.GroupChatList {
			if !validText(item.ChatID, 1024) {
				return nil, ErrUnexpectedResponse
			}
			if _, duplicate := seenChatIDs[item.ChatID]; duplicate {
				return nil, ErrUnexpectedResponse
			}
			seenChatIDs[item.ChatID] = struct{}{}
			chatIDs = append(chatIDs, item.ChatID)
		}
		if response.NextCursor == "" {
			return chatIDs, nil
		}
		cursor = response.NextCursor
	}
}

type groupChat struct {
	ChatID      string
	Name        string
	Owner       string
	MemberCount int32
}

func (source *Source) getGroupChat(ctx context.Context, chatID string) (groupChat, error) {
	var response struct {
		GroupChat struct {
			ChatID     string            `json:"chat_id"`
			Name       string            `json:"name"`
			Owner      string            `json:"owner"`
			MemberList []json.RawMessage `json:"member_list"`
		} `json:"group_chat"`
	}
	if err := source.post(ctx, "/cgi-bin/externalcontact/groupchat/get", map[string]any{"chat_id": chatID, "need_name": 0}, &response); err != nil {
		return groupChat{}, err
	}
	if len(response.GroupChat.MemberList) > math.MaxInt32 {
		return groupChat{}, ErrUnexpectedResponse
	}
	for _, member := range response.GroupChat.MemberList {
		var object map[string]json.RawMessage
		if len(member) == 0 || !strictUnmarshal(member, &object) || len(object) == 0 {
			return groupChat{}, ErrUnexpectedResponse
		}
	}
	return groupChat{ChatID: response.GroupChat.ChatID, Name: response.GroupChat.Name, Owner: response.GroupChat.Owner, MemberCount: int32(len(response.GroupChat.MemberList))}, nil
}

func (source *Source) get(ctx context.Context, path string, target any) error {
	return source.request(ctx, http.MethodGet, path, nil, target)
}

func (source *Source) post(ctx context.Context, path string, payload any, target any) error {
	return source.request(ctx, http.MethodPost, path, payload, target)
}

func (source *Source) request(ctx context.Context, method, path string, payload, target any) error {
	token, err := source.token.Token(ctx)
	if err != nil || token.Value() == "" {
		return tokenError(err)
	}
	err = source.requestWithToken(ctx, method, path, token.Value(), payload, target)
	var apiErr *apiError
	if !errors.As(err, &apiErr) || !expiredTokenCode(apiErr.Code) {
		return err
	}
	token, refreshErr := source.token.RefreshToken(ctx)
	if refreshErr != nil || token.Value() == "" {
		return err
	}
	return source.requestWithToken(ctx, method, path, token.Value(), payload, target)
}

func (source *Source) requestWithToken(ctx context.Context, method, path, token string, payload, target any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return ErrInvalidConfig
		}
		body = bytes.NewReader(encoded)
	}
	endpoint := source.baseURL.ResolveReference(&url.URL{Path: path})
	query := endpoint.Query()
	query.Set("access_token", token)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return ErrInvalidConfig
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := source.httpClient.Do(request)
	if err != nil {
		return requestError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrUnexpectedResponse, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return ErrUnexpectedResponse
	}
	var envelope struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if !strictUnmarshal(data, &envelope) {
		return ErrUnexpectedResponse
	}
	if envelope.ErrCode != 0 {
		return fmt.Errorf("%w: %w", ErrUpstream, &apiError{Code: envelope.ErrCode})
	}
	if !strictUnmarshal(data, target) {
		return ErrUnexpectedResponse
	}
	return nil
}

type apiError struct{ Code int }

func (err *apiError) Error() string { return fmt.Sprintf("WeCom API error %d", err.Code) }

func expiredTokenCode(code int) bool { return code == 40014 || code == 42001 }

func parseBaseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, ErrInvalidConfig
	}
	return parsed, nil
}

func strictUnmarshal(data []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	return decoder.Decode(target) == nil && decoder.Decode(&struct{}{}) == io.EOF
}

func requestError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrUnexpectedResponse, context.DeadlineExceeded)
	}
	return ErrUnexpectedResponse
}

func tokenError(err error) error {
	if err == nil {
		return ErrUnexpectedResponse
	}
	return fmt.Errorf("%w: %w", ErrUnexpectedResponse, err)
}

func validCursor(value string) bool { return len(value) <= 2048 && validTextOrEmpty(value, 2048) }

func validMembers(count int32) bool { return count > 0 }

func validText(value string, limit int) bool {
	return value != "" && validTextOrEmpty(value, limit)
}

func validTextOrEmpty(value string, limit int) bool {
	return len(value) <= limit && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}
