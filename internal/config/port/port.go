// Package port freezes the cross-domain non-secret settings contract.
package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Key string

const (
	WeComCorpID           Key = "wecom.corp_id"
	WeComAgentID          Key = "wecom.agent_id"
	OutboundRatePerSecond Key = "outbound.rate_per_second"
	OutboundMaxAttempts   Key = "outbound.max_attempts"

	DatabaseURL           Key = "database.url"
	WeComSecret           Key = "wecom.secret"
	WeComCallbackToken    Key = "wecom.callback_token"
	WeComCallbackAESKey   Key = "wecom.callback_aes_key"
	AIAPIKey              Key = "ai.api_key"
	AuthJWTSecret         Key = "auth.jwt_secret"
	ExtensionAPIKeyPepper Key = "extension.api_key_pepper"
	WebhookMasterKey      Key = "gateway.webhook_master_key"
)

var (
	ErrUnknownSetting      = errors.New("unknown setting key")
	ErrSecretSetting       = errors.New("secret setting forbidden")
	ErrInvalidSetting      = errors.New("invalid setting value")
	ErrSettingNotFound     = errors.New("setting not found")
	ErrIdempotencyConflict = errors.New("setting idempotency conflict")
)

type Setting struct {
	Key       Key
	Value     json.RawMessage
	UpdatedBy string
	UpdatedAt time.Time
}

type SetCommand struct {
	Key       Key
	Value     json.RawMessage
	Actor     string
	RequestID string
}

type Service interface {
	Get(context.Context, Key) (Setting, error)
	Set(context.Context, SetCommand) (Setting, error)
}
