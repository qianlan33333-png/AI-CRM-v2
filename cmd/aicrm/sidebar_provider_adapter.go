package main

import (
	"context"
	"errors"
	"log/slog"

	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

// sidebarOAuthProvider translates only trusted WeCom enterprise identities;
// the sidebar app owns all state, session, customer, and owner binding.
type sidebarOAuthProvider struct{ client *wecomclient.HumanOAuthClient }

func (provider sidebarOAuthProvider) CorpID() string {
	if provider.client == nil {
		return ""
	}
	return provider.client.CorpID()
}

func (provider sidebarOAuthProvider) AuthorizationURL(state string) (string, error) {
	if provider.client == nil {
		return "", errors.New("sidebar oauth provider unavailable")
	}
	return provider.client.AuthorizationURL(state)
}

func (provider sidebarOAuthProvider) Exchange(ctx context.Context, code string) (sidebarapp.OAuthIdentity, error) {
	if provider.client == nil {
		return sidebarapp.OAuthIdentity{}, errors.New("sidebar oauth provider unavailable")
	}
	identity, err := provider.client.Exchange(ctx, code)
	if err != nil {
		category, providerCode := sidebarOAuthFailureFields(err)
		slog.WarnContext(ctx, "sidebar_oauth_provider_exchange_failed", "category", category, "provider_code", providerCode)
		return sidebarapp.OAuthIdentity{}, err
	}
	return sidebarapp.OAuthIdentity{CorpID: string(identity.CorpID), UserID: identity.UserID}, nil
}

func sidebarOAuthFailureFields(err error) (string, int) {
	var providerError *wecomclient.APIError
	if errors.As(err, &providerError) {
		return "upstream_rejected", providerError.Code
	}
	switch {
	case errors.Is(err, wecomclient.ErrRequestTimeout):
		return "timeout", 0
	case errors.Is(err, wecomclient.ErrTransport):
		return "transport", 0
	case errors.Is(err, wecomclient.ErrUnexpectedResponse):
		return "unexpected_response", 0
	default:
		return "unknown", 0
	}
}

// sidebarAgentConfigTicketProvider exposes the two read-only tickets needed
// for wx.config and wx.agentConfig. It has no provider write capability.
type sidebarAgentConfigTicketProvider struct {
	client *wecomclient.AgentConfigTicketClient
}

func (provider sidebarAgentConfigTicketProvider) FetchAgentConfigTicket(ctx context.Context) (sidebarapp.AgentConfigTicket, error) {
	if provider.client == nil {
		return sidebarapp.AgentConfigTicket{}, errors.New("sidebar agent config provider unavailable")
	}
	ticket, err := provider.client.FetchAgentConfigTicket(ctx)
	if err != nil {
		category, providerCode := sidebarOAuthFailureFields(err)
		slog.WarnContext(ctx, "sidebar_jssdk_ticket_failed", "stage", "agent_config", "category", category, "provider_code", providerCode)
		return sidebarapp.AgentConfigTicket{}, err
	}
	return sidebarapp.AgentConfigTicket{Value: ticket.Value, ExpiresAt: ticket.ExpiresAt}, nil
}

func (provider sidebarAgentConfigTicketProvider) FetchConfigTicket(ctx context.Context) (sidebarapp.AgentConfigTicket, error) {
	if provider.client == nil {
		return sidebarapp.AgentConfigTicket{}, errors.New("sidebar config provider unavailable")
	}
	ticket, err := provider.client.FetchConfigTicket(ctx)
	if err != nil {
		category, providerCode := sidebarOAuthFailureFields(err)
		slog.WarnContext(ctx, "sidebar_jssdk_ticket_failed", "stage", "config", "category", category, "provider_code", providerCode)
		return sidebarapp.AgentConfigTicket{}, err
	}
	return sidebarapp.AgentConfigTicket{Value: ticket.Value, ExpiresAt: ticket.ExpiresAt}, nil
}
