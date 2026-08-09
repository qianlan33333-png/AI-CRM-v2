package platformriver

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testLifecycle struct {
	start   func(context.Context) error
	stop    func(context.Context) error
	stopped chan struct{}
}

func (l *testLifecycle) Start(ctx context.Context) error {
	return l.start(ctx)
}

func (l *testLifecycle) Stop(ctx context.Context) error {
	return l.stop(ctx)
}

func (l *testLifecycle) Stopped() <-chan struct{} {
	return l.stopped
}

func TestNewRuntimeRetainsLifecycle(t *testing.T) {
	lifecycle := &testLifecycle{}
	got := NewRuntime(lifecycle)
	if got == nil || got.lifecycle != lifecycle {
		t.Fatal("NewRuntime did not retain lifecycle")
	}
}

func TestRuntimeStartErrorIsPreserved(t *testing.T) {
	want := errors.New("start failed")
	stops := 0
	lifecycle := &testLifecycle{
		start: func(context.Context) error { return want },
		stop: func(context.Context) error {
			stops++
			return nil
		},
		stopped: make(chan struct{}),
	}

	if got := NewRuntime(lifecycle).Run(context.Background()); got != want {
		t.Fatalf("Run error = %v, want original %v", got, want)
	}
	if stops != 0 {
		t.Fatalf("Stop calls = %d, want 0", stops)
	}
}

func TestRuntimeCancellationStopsAndPreservesError(t *testing.T) {
	started := make(chan struct{})
	stopContextIsLive := make(chan bool, 1)
	want := errors.New("stop failed")
	lifecycle := &testLifecycle{
		start: func(context.Context) error {
			close(started)
			return nil
		},
		stop: func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			stopContextIsLive <- ok && ctx.Err() == nil && time.Until(deadline) > 0
			return want
		},
		stopped: make(chan struct{}),
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- NewRuntime(lifecycle).Run(parent) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Start was not called")
	}
	cancel()

	select {
	case got := <-done:
		if got != want {
			t.Fatalf("Run error = %v, want original %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after parent cancellation")
	}
	select {
	case live := <-stopContextIsLive:
		if !live {
			t.Fatal("Stop did not receive a live bounded shutdown context")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop was not called")
	}
}

func TestRuntimeEarlyStoppedReturnsUnexpectedStop(t *testing.T) {
	stopped := make(chan struct{})
	close(stopped)
	stops := 0
	lifecycle := &testLifecycle{
		start:   func(context.Context) error { return nil },
		stop:    func(context.Context) error { stops++; return nil },
		stopped: stopped,
	}

	err := NewRuntime(lifecycle).Run(context.Background())
	if err == nil {
		t.Fatal("Run error = nil, want unexpected-stop error")
	}
	if stops != 0 {
		t.Fatalf("Stop calls = %d, want 0", stops)
	}
}

func TestMigrateRejectsInvalidDirection(t *testing.T) {
	err := Migrate(context.Background(), nil, Direction("sideways"), nil)
	if !errors.Is(err, ErrInvalidDirection) {
		t.Fatalf("Migrate error = %v, want ErrInvalidDirection", err)
	}
	if got, want := err.Error(), `platform river migration: invalid direction "sideways"`; got != want {
		t.Fatalf("Migrate error = %q, want %q", got, want)
	}
}
