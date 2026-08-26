// Package provider adapts authorised provider clients to outbound task sends.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

var ErrInvalidWeComPrivateMessageProvider = errors.New("invalid WeCom private message provider")

// targetResolver returns the unique verified external WeCom identity and the
// active owner staff WeCom user ID for one Customer. resolved=false is a
// business refusal (missing or ambiguous identity); err is an infrastructure
// failure and must not be converted into a terminal claim.
type targetResolver func(context.Context, int64) (sender, externalUserID string, resolved bool, err error)

// WeComPrivateMessageProvider implements the existing outbound ProviderAdapter
// for one approved text.notice.v1 task. It only uses an explicit payload target
// or an exact resolver; it never guesses from CustomerID.
type WeComPrivateMessageProvider struct {
	client  wecomport.PrivateMessageTemplateCreator
	resolve targetResolver
}

var _ outboundapp.ProviderAdapter = (*WeComPrivateMessageProvider)(nil)

func NewWeComPrivateMessageProvider(client wecomport.PrivateMessageTemplateCreator, resolve targetResolver) (*WeComPrivateMessageProvider, error) {
	if client == nil {
		return nil, ErrInvalidWeComPrivateMessageProvider
	}
	return &WeComPrivateMessageProvider{client: client, resolve: resolve}, nil
}

func (provider *WeComPrivateMessageProvider) Send(ctx context.Context, request outboundapp.SendRequest) (outboundapp.ProviderResult, error) {
	payload, valid := decodePrivateMessagePayload(request)
	if ctx == nil || provider == nil || provider.client == nil {
		return outboundapp.ProviderResult{}, ErrInvalidWeComPrivateMessageProvider
	}
	if !valid {
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureInvalidArgument, Code: "invalid_wecom_private_message_payload"}, nil
	}
	if payload.Sender == "" && payload.ExternalUserID == "" {
		if provider.resolve == nil {
			return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureInvalidArgument, Code: "wecom_private_target_unavailable"}, nil
		}
		sender, externalUserID, resolved, resolveErr := provider.resolve(ctx, request.CustomerID)
		if resolveErr != nil {
			return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureTemporary, Code: "wecom_private_target_resolution_unavailable"}, nil
		}
		if !resolved || !validPrivateMessageText(sender, 128) || !validPrivateMessageText(externalUserID, 1024) {
			return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureInvalidArgument, Code: "wecom_private_target_unavailable"}, nil
		}
		payload.Sender, payload.ExternalUserID = sender, externalUserID
	}
	result, err := provider.client.CreatePrivateMessageTemplate(ctx, wecomport.PrivateMessageTemplateRequest{
		Sender: payload.Sender, ExternalUserID: payload.ExternalUserID, Text: payload.Text,
	})
	if err != nil {
		return classifyWeComPrivateMessageError(err), nil
	}
	if strings.TrimSpace(result.MessageID) == "" {
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureInvalidResult, Code: "wecom_missing_msgid"}, nil
	}
	return outboundapp.ProviderResult{MessageID: result.MessageID}, nil
}

type privateMessagePayload struct {
	Sender         string `json:"sender"`
	ExternalUserID string `json:"external_userid"`
	Text           string `json:"text"`
}

func decodePrivateMessagePayload(request outboundapp.SendRequest) (privateMessagePayload, bool) {
	if request.TaskID <= 0 || request.CustomerID <= 0 || request.TemplateKey != outboundapp.TemplateTextNoticeV1 {
		return privateMessagePayload{}, false
	}
	var payload privateMessagePayload
	decoder := json.NewDecoder(strings.NewReader(string(request.Payload)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&payload) != nil || decoder.Decode(&struct{}{}) == nil ||
		!validPrivateMessageText(payload.Text, 4000) ||
		(payload.Sender == "") != (payload.ExternalUserID == "") ||
		(payload.Sender != "" && (!validPrivateMessageText(payload.Sender, 128) || !validPrivateMessageText(payload.ExternalUserID, 1024))) {
		return privateMessagePayload{}, false
	}
	return payload, true
}

func validPrivateMessageText(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.TrimSpace(value) == value
}

func classifyWeComPrivateMessageError(err error) outboundapp.ProviderResult {
	switch {
	case errors.Is(err, wecomport.ErrWriteOutcomeUnknown):
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureConnection, Code: "wecom_write_outcome_unknown"}
	case errors.Is(err, wecomport.ErrPrivateMessageTargetRejected):
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureRecipientUnavailable, Code: "wecom_private_target_rejected"}
	case errors.Is(err, wecomport.ErrBusinessWriteNotDispatched), errors.Is(err, wecomport.ErrRequestTimeout), errors.Is(err, wecomport.ErrTransport):
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureTemporary, Code: "wecom_not_dispatched"}
	case errors.Is(err, wecomport.ErrUpstream):
		var apiErr *wecomport.APIError
		if errors.As(err, &apiErr) {
			code := fmt.Sprintf("wecom_errcode_%d", apiErr.Code)
			switch apiErr.Code {
			case 45009, 45011, 45035:
				return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureRateLimited, Code: code}
			case 84061:
				return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureRecipientUnavailable, Code: code}
			default:
				return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureInvalidArgument, Code: code}
			}
		}
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureInvalidArgument, Code: "wecom_upstream"}
	default:
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureInvalidResult, Code: "wecom_provider_error"}
	}
}
