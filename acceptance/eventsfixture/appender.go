// Package eventsfixture exposes Events-owned dependencies to acceptance tests.
package eventsfixture

import (
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
)

// NewAppender returns the real transaction-bound Events appender.
func NewAppender() eventport.Appender {
	return eventstore.NewAppender()
}
