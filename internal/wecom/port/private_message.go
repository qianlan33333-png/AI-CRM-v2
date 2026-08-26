// Package port freezes narrow cross-domain WeCom contracts.
package port

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrWriteOutcomeUnknown          = errors.New("WeCom write outcome unknown")
	ErrBusinessWriteNotDispatched   = errors.New("WeCom business write not dispatched")
	ErrPrivateMessageTargetRejected = errors.New("WeCom private message target rejected")
	ErrRequestTimeout               = errors.New("WeCom request timed out")
	ErrTransport                    = errors.New("WeCom transport failure")
	ErrUpstream                     = errors.New("WeCom API rejected request")
)

// PrivateMessageTemplateRequest is the exact single-recipient subset of
// WeCom's add_msg_template request used by one outbound task.
type PrivateMessageTemplateRequest struct {
	Sender         string
	ExternalUserID string
	Text           string
}

// PrivateMessageTemplate is provider acceptance, not delivery confirmation.
type PrivateMessageTemplate struct {
	MessageID string
}

type PrivateMessageTemplateCreator interface {
	CreatePrivateMessageTemplate(context.Context, PrivateMessageTemplateRequest) (PrivateMessageTemplate, error)
}

// APIError exposes WeCom's numeric code without exposing credentials.
type APIError struct {
	Code    int
	Message string
}

func (err *APIError) Error() string {
	return fmt.Sprintf("WeCom API error %d: %s", err.Code, err.Message)
}
