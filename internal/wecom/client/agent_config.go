package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// AgentConfigTicketClient reads the two read-only WeCom tickets required by
// wx.config and wx.agentConfig. It has no provider write capability.
type AgentConfigTicketClient struct {
	baseURL       *url.URL
	httpClient    *http.Client
	tokenProvider TokenProvider
	now           func() time.Time
}

type AgentConfigTicketClientConfig struct {
	BaseURL       string
	HTTPClient    *http.Client
	TokenProvider TokenProvider
	Now           func() time.Time
}

type AgentConfigTicket struct {
	Value     string
	ExpiresAt time.Time
}

func NewAgentConfigTicketClient(config AgentConfigTicketClientConfig) (*AgentConfigTicketClient, error) {
	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil || config.HTTPClient == nil || config.TokenProvider == nil || config.Now == nil {
		return nil, ErrInvalidConfig
	}
	return &AgentConfigTicketClient{baseURL: baseURL, httpClient: config.HTTPClient, tokenProvider: config.TokenProvider, now: config.Now}, nil
}

func (client *AgentConfigTicketClient) FetchAgentConfigTicket(ctx context.Context) (AgentConfigTicket, error) {
	return client.fetchTicket(ctx, "agent_config")
}

func (client *AgentConfigTicketClient) FetchConfigTicket(ctx context.Context) (AgentConfigTicket, error) {
	return client.fetchTicket(ctx, "jsapi")
}

func (client *AgentConfigTicketClient) fetchTicket(ctx context.Context, ticketType string) (AgentConfigTicket, error) {
	if client == nil || ctx == nil {
		return AgentConfigTicket{}, ErrInvalidConfig
	}
	if ticketType != "jsapi" && ticketType != "agent_config" {
		return AgentConfigTicket{}, ErrInvalidConfig
	}
	token, err := client.tokenProvider.Token(ctx)
	if err != nil {
		return AgentConfigTicket{}, err
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: "/cgi-bin/ticket/get"})
	query := url.Values{}
	query.Set("access_token", token.Value())
	query.Set("type", ticketType)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return AgentConfigTicket{}, ErrInvalidConfig
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return AgentConfigTicket{}, mapRequestError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return AgentConfigTicket{}, fmt.Errorf("%w: status %d", ErrUnexpectedResponse, response.StatusCode)
	}
	var payload struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		Ticket    string `json:"ticket"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if err = decodeResponse(response.Body, &payload); err != nil {
		return AgentConfigTicket{}, err
	}
	if payload.ErrCode != 0 {
		return AgentConfigTicket{}, apiError(payload.ErrCode, payload.ErrMsg)
	}
	if !validProviderTicket(payload.Ticket) || payload.ExpiresIn < 61 || payload.ExpiresIn > 24*60*60 {
		return AgentConfigTicket{}, ErrUnexpectedResponse
	}
	now := client.now().UTC()
	if now.IsZero() {
		return AgentConfigTicket{}, ErrInvalidConfig
	}
	return AgentConfigTicket{Value: payload.Ticket, ExpiresAt: now.Add(time.Duration(payload.ExpiresIn) * time.Second)}, nil
}

func validProviderTicket(value string) bool {
	if len(value) < 1 || len(value) > 2048 || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}
