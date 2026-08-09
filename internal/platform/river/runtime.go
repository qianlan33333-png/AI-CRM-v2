package platformriver

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

type Runtime struct {
	lifecycle Lifecycle
}

func NewRuntime(lifecycle Lifecycle) *Runtime {
	return &Runtime{lifecycle: lifecycle}
}

func (r *Runtime) Run(parent context.Context) error {
	if err := r.lifecycle.Start(context.WithoutCancel(parent)); err != nil {
		return err
	}

	select {
	case <-parent.Done():
		return r.stop(parent)
	case <-r.lifecycle.Stopped():
		select {
		case <-parent.Done():
			return r.stop(parent)
		default:
			return runtime.ErrUnexpectedStop
		}
	}
}

func (r *Runtime) stop(parent context.Context) error {
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(parent), runtime.ShutdownGrace)
	defer cancel()
	return r.lifecycle.Stop(shutdown)
}
