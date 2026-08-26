package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxWeComPrivateMessageResponseBytes = 1 << 20

var (
	errWeComPrivateMessageInvalid        = errors.New("invalid WeCom private message client")
	errWeComPrivateMessageNotDispatched  = errors.New("WeCom private message not dispatched")
	errWeComPrivateMessageOutcomeUnknown = errors.New("WeCom private message outcome unknown")
	errWeComPrivateMessageTargetRejected = errors.New("WeCom private message target rejected")
	errWeComPrivateMessageUpstream       = errors.New("WeCom private message API rejected request")
)

type privateMessageTemplateRequest struct {
	Sender         string
	ExternalUserID string
	Text           string
}

type privateMessageTemplateResult struct {
	MessageID string
}

type weComPrivateMessageAPIError struct {
	Code int
}

func (err *weComPrivateMessageAPIError) Error() string {
	return fmt.Sprintf("WeCom private message API error %d", err.Code)
}

type WeComPrivateMessageClientConfig struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      func(context.Context) (string, error)
}

type weComPrivateMessageClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      func(context.Context) (string, error)
}

func NewWeComPrivateMessageClient(config WeComPrivateMessageClientConfig) (*weComPrivateMessageClient, error) {
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") || config.HTTPClient == nil || config.Token == nil {
		return nil, errWeComPrivateMessageInvalid
	}
	return &weComPrivateMessageClient{baseURL: baseURL, httpClient: config.HTTPClient, token: config.Token}, nil
}

func (client *weComPrivateMessageClient) CreatePrivateMessageTemplate(ctx context.Context, request privateMessageTemplateRequest) (privateMessageTemplateResult, error) {
	if ctx == nil || client == nil || client.httpClient == nil || client.token == nil ||
		!validPrivateMessageText(request.Sender, 128) || !validPrivateMessageText(request.ExternalUserID, 1024) || !validPrivateMessageText(request.Text, 4000) {
		return privateMessageTemplateResult{}, errWeComPrivateMessageInvalid
	}
	token, err := client.token(ctx)
	if err != nil || token == "" || strings.TrimSpace(token) != token {
		return privateMessageTemplateResult{}, errors.Join(errWeComPrivateMessageNotDispatched, err)
	}
	body, err := json.Marshal(map[string]any{
		"chat_type": "single", "sender": request.Sender, "external_userid": []string{request.ExternalUserID},
		"allow_select": false, "text": map[string]string{"content": request.Text},
	})
	if err != nil {
		return privateMessageTemplateResult{}, errWeComPrivateMessageInvalid
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: "/cgi-bin/externalcontact/add_msg_template"})
	query := url.Values{}
	query.Set("access_token", token)
	endpoint.RawQuery = query.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return privateMessageTemplateResult{}, errWeComPrivateMessageNotDispatched
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return privateMessageTemplateResult{}, errWeComPrivateMessageOutcomeUnknown
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return privateMessageTemplateResult{}, errWeComPrivateMessageOutcomeUnknown
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxWeComPrivateMessageResponseBytes+1))
	if err != nil || len(data) > maxWeComPrivateMessageResponseBytes {
		return privateMessageTemplateResult{}, errWeComPrivateMessageOutcomeUnknown
	}
	var payload struct {
		ErrCode int      `json:"errcode"`
		Message string   `json:"msgid"`
		Fail    []string `json:"fail_list"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&payload) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return privateMessageTemplateResult{}, errWeComPrivateMessageOutcomeUnknown
	}
	if payload.ErrCode != 0 {
		return privateMessageTemplateResult{}, fmt.Errorf("%w: %w", errWeComPrivateMessageUpstream, &weComPrivateMessageAPIError{Code: payload.ErrCode})
	}
	if len(payload.Fail) != 0 {
		return privateMessageTemplateResult{}, errWeComPrivateMessageTargetRejected
	}
	if !validPrivateMessageText(payload.Message, 1024) {
		return privateMessageTemplateResult{}, errWeComPrivateMessageOutcomeUnknown
	}
	return privateMessageTemplateResult{MessageID: payload.Message}, nil
}
