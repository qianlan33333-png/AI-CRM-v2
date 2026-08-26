package client

import (
	"context"
	"strings"

	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

var ErrPrivateMessageTargetRejected = wecomport.ErrPrivateMessageTargetRejected

// PrivateMessageTemplateRequest is the exact single-recipient subset of
// WeCom's add_msg_template request used by one outbound task. It deliberately
// excludes batches, media, and any local Customer identity.
type PrivateMessageTemplateRequest = wecomport.PrivateMessageTemplateRequest

// PrivateMessageTemplate is the provider acknowledgement for a successfully
// created message template. It is not delivery confirmation.
type PrivateMessageTemplate = wecomport.PrivateMessageTemplate

// CreatePrivateMessageTemplate creates a WeCom external-contact private
// message template. As with the other CustomerAcquisitionClient writes, any
// response ambiguity after the request crosses the provider boundary becomes
// ErrWriteOutcomeUnknown and is never retried here.
func (client *CustomerAcquisitionClient) CreatePrivateMessageTemplate(ctx context.Context, request PrivateMessageTemplateRequest) (PrivateMessageTemplate, error) {
	if client == nil || !validPrivateMessageTemplateRequest(request) {
		return PrivateMessageTemplate{}, ErrInvalidConfig
	}
	var payload struct {
		ErrCode int      `json:"errcode"`
		ErrMsg  string   `json:"errmsg"`
		Message string   `json:"msgid"`
		Fail    []string `json:"fail_list"`
	}
	if err := client.write(ctx, "/cgi-bin/externalcontact/add_msg_template", map[string]any{
		"chat_type":       "single",
		"sender":          request.Sender,
		"external_userid": []string{request.ExternalUserID},
		"allow_select":    false,
		"text":            map[string]string{"content": request.Text},
	}, &payload); err != nil {
		return PrivateMessageTemplate{}, err
	}
	if len(payload.Fail) != 0 {
		return PrivateMessageTemplate{}, ErrPrivateMessageTargetRejected
	}
	if !validProviderID(payload.Message) {
		return PrivateMessageTemplate{}, ErrWriteOutcomeUnknown
	}
	return PrivateMessageTemplate{MessageID: payload.Message}, nil
}

func validPrivateMessageTemplateRequest(request PrivateMessageTemplateRequest) bool {
	return validRequiredText(request.Sender, 128) && validRequiredText(request.ExternalUserID, 1024) &&
		validRequiredText(request.Text, 4000) && strings.TrimSpace(request.Text) == request.Text
}
