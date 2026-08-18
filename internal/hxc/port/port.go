package port

import (
	"context"
	"time"
)

type SenderConfig struct {
	ID           string
	SenderUserID string
	DisplayName  string
	Priority     int
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
type SenderConfigReader interface {
	ListSenderConfigs(context.Context) ([]SenderConfig, error)
}
