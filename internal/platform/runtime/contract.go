// Package runtime owns process-role parsing and lifecycle composition.
package runtime

import (
	"context"
	"errors"
	"time"
)

// Role identifies the only supported process compositions.
type Role string

const (
	RoleAPI    Role = "api"
	RoleWorker Role = "worker"
	RoleAll    Role = "all"
)

const (
	ExitOK      = 0
	ExitRuntime = 1
	ExitUsage   = 2
)

const (
	// ShutdownGrace bounds component shutdown after the parent context cancels.
	ShutdownGrace = 10 * time.Second
	// UsageLine is the stable first line emitted for help and usage errors.
	UsageLine = "Usage: aicrm --role=<api|worker|all>"
)

var (
	// ErrInvalidRole is returned for missing, unknown, or non-canonical role text.
	ErrInvalidRole = errors.New("invalid process role")
	// ErrMissingComponent means the selected role cannot be composed atomically.
	ErrMissingComponent = errors.New("selected component is missing")
	// ErrUnexpectedStop means a component stopped before parent cancellation.
	ErrUnexpectedStop = errors.New("component stopped unexpectedly")
	// ErrShutdownTimeout means at least one component ignored graceful shutdown.
	ErrShutdownTimeout = errors.New("component shutdown timed out")
)

// CLIResult is the complete output of strict command-line parsing.
type CLIResult struct {
	Role Role
	Help bool
}

// Component is a long-running, context-bound process component.
type Component interface {
	Run(context.Context) error
}

// ComponentFunc adapts a function to Component.
type ComponentFunc func(context.Context) error

// Run implements Component.
func (f ComponentFunc) Run(ctx context.Context) error {
	return f(ctx)
}

// Components contains the only two process capabilities P0-S01 may compose.
// Later slices replace their placeholder implementations at the composition root.
type Components struct {
	API    Component
	Worker Component
}
