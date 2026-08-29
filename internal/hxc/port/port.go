package port

import (
	"context"
	"time"
)

type SenderConfig struct {
	ID           string    `json:"id"`
	SenderUserID string    `json:"sender_userid"`
	DisplayName  string    `json:"display_name"`
	Priority     int       `json:"priority"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type SenderConfigReader interface {
	ListSenderConfigs(context.Context) ([]SenderConfig, error)
}
