package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type slot struct {
	name string
	run  Component
}
type componentResult struct {
	index    int
	err      error
	canceled bool
}

// Run starts the selected process components and owns their shared lifecycle.
func Run(ctx context.Context, role Role, components Components) error {
	return runWithGrace(ctx, role, components, ShutdownGrace)
}

func runWithGrace(ctx context.Context, role Role, components Components, grace time.Duration) error {
	slots, err := selectedSlots(role, components)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan componentResult, len(slots))
	for i, s := range slots {
		go func(i int, s slot) {
			err := s.run.Run(runCtx)
			results <- componentResult{i, err, runCtx.Err() != nil}
		}(i, s)
	}
	returned, errs := make([]bool, len(slots)), make([]error, len(slots))
	remaining := len(slots)
	select {
	case <-ctx.Done():
		cancel()
	case result := <-results:
		returned[result.index], remaining = true, remaining-1
		errs[result.index] = classify(slots[result.index].name, result.err, !result.canceled)
		cancel()
	}
	if remaining == 0 {
		return errors.Join(errs...)
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	for remaining > 0 {
		select {
		case result := <-results:
			returned[result.index], remaining = true, remaining-1
			errs[result.index] = classify(slots[result.index].name, result.err, !result.canceled)
		case <-timer.C:
			for i, s := range slots {
				if !returned[i] {
					errs[i] = fmt.Errorf("%w: %s", ErrShutdownTimeout, s.name)
				}
			}
			return errors.Join(errs...)
		}
	}
	return errors.Join(errs...)
}

func selectedSlots(role Role, components Components) ([]slot, error) {
	switch role {
	case RoleAPI:
		if components.API == nil {
			return nil, missing("api")
		}
		return []slot{{"api", components.API}}, nil
	case RoleWorker:
		if components.Worker == nil {
			return nil, missing("worker")
		}
		return []slot{{"worker", components.Worker}}, nil
	case RoleAll:
		var names []string
		if components.API == nil {
			names = append(names, "api")
		}
		if components.Worker == nil {
			names = append(names, "worker")
		}
		if len(names) != 0 {
			return nil, missing(names...)
		}
		return []slot{{"api", components.API}, {"worker", components.Worker}}, nil
	default:
		return nil, ErrInvalidRole
	}
}

func missing(names ...string) error {
	return fmt.Errorf("%w: %s", ErrMissingComponent, strings.Join(names, ", "))
}

func classify(name string, err error, unexpected bool) error {
	if err == nil || cancellationOnly(err) {
		if unexpected {
			return fmt.Errorf("%s: %w", name, ErrUnexpectedStop)
		}
		return nil
	}
	return fmt.Errorf("%s: %w", name, err)
}

func cancellationOnly(err error) bool {
	switch unwrapped := err.(type) {
	case interface{ Unwrap() []error }:
		children := unwrapped.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if child == nil || !cancellationOnly(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		if child := unwrapped.Unwrap(); child != nil {
			return cancellationOnly(child)
		}
	}
	return err == context.Canceled
}
