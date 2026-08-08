// Package platformriver freezes the River adapter lifecycle boundary.
package platformriver

import (
	"context"
	"errors"
)

const PinnedVersion = "v0.24.0"

var ErrInvalidDirection = errors.New("invalid River migration direction")

// Lifecycle is the River-neutral client surface. v0.24.0's Client satisfies it.
type Lifecycle interface {
	Start(context.Context) error
	Stop(context.Context) error
	Stopped() <-chan struct{}
}

// Direction accepts only up/down; MigrateOptions keeps the API River-neutral.
type Direction string

const (
	DirectionUp   Direction = "up"
	DirectionDown Direction = "down"
)

type MigrateOptions struct{ TargetVersion int }

// Infrastructure delivery is at-least-once; consumers own durable idempotency.
