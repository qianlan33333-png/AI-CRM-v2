package runtime

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseCLI(t *testing.T) {
	tests := []struct {
		name, role            string
		args                  []string
		help, roleErr, anyErr bool
	}{
		{"api equals", "api", []string{"--role=api"}, false, false, false}, {"api split", "api", []string{"--role", "api"}, false, false, false},
		{"worker equals", "worker", []string{"--role=worker"}, false, false, false}, {"worker split", "worker", []string{"--role", "worker"}, false, false, false},
		{"all equals", "all", []string{"--role=all"}, false, false, false}, {"all split", "all", []string{"--role", "all"}, false, false, false},
		{"help", "", []string{"--help"}, true, false, false}, {"short help", "", []string{"-h"}, true, false, false},
		{"missing", "", nil, false, true, false}, {"empty equals", "", []string{"--role="}, false, true, false}, {"empty split", "", []string{"--role"}, false, true, false},
		{"unknown", "", []string{"--role=x"}, false, true, false}, {"uppercase", "", []string{"--role=API"}, false, true, false}, {"whitespace", "", []string{"--role", " api "}, false, true, false},
		{"combined", "", []string{"--role=api,worker"}, false, true, false}, {"duplicate", "", []string{"--role=api", "--role=worker"}, false, false, true},
		{"unknown flag", "", []string{"--debug"}, false, false, true}, {"position", "", []string{"api"}, false, false, true}, {"help mixed", "", []string{"--help", "--role=api"}, false, false, true},
	}
	for _, tt := range tests {
		got, err := ParseCLI(tt.args)
		if tt.roleErr && !errors.Is(err, ErrInvalidRole) || tt.anyErr && (err == nil || errors.Is(err, ErrInvalidRole)) {
			t.Fatalf("%s: ParseCLI() error = %v", tt.name, err)
		}
		if !tt.roleErr && !tt.anyErr && (err != nil || got.Help != tt.help || string(got.Role) != tt.role) {
			t.Fatalf("%s: ParseCLI() = %#v, %v", tt.name, got, err)
		}
	}
}
func TestRunRoleSelection(t *testing.T) {
	for _, tt := range []struct {
		role        Role
		api, worker int32
	}{{RoleAPI, 1, 0}, {RoleWorker, 0, 1}, {RoleAll, 1, 1}} {
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{}, 2)
		var api, worker atomic.Int32
		component := func(n *atomic.Int32) ComponentFunc {
			return func(ctx context.Context) error { n.Add(1); started <- struct{}{}; <-ctx.Done(); return ctx.Err() }
		}
		done := make(chan error, 1)
		go func() { done <- Run(ctx, tt.role, Components{component(&api), component(&worker)}) }()
		for range int(tt.api + tt.worker) {
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("selected components did not start concurrently")
			}
		}
		cancel()
		if err := <-done; err != nil || api.Load() != tt.api || worker.Load() != tt.worker {
			t.Fatalf("%s: err=%v calls api=%d worker=%d", tt.role, err, api.Load(), worker.Load())
		}
	}
}
func TestRunImmediateCases(t *testing.T) {
	sentinel := errors.New("component failed")
	var calls atomic.Int32
	count := ComponentFunc(func(context.Context) error { calls.Add(1); return nil })
	cases := []struct {
		role Role
		c    Components
		want error
	}{
		{Role("bad"), Components{count, count}, ErrInvalidRole}, {RoleAll, Components{API: count}, ErrMissingComponent},
		{RoleAPI, Components{API: ComponentFunc(func(context.Context) error { return nil })}, ErrUnexpectedStop},
		{RoleAPI, Components{API: ComponentFunc(func(context.Context) error { return context.Canceled })}, ErrUnexpectedStop},
		{RoleAPI, Components{API: ComponentFunc(func(context.Context) error { return sentinel })}, sentinel},
	}
	for _, tt := range cases {
		if err := Run(context.Background(), tt.role, tt.c); !errors.Is(err, tt.want) {
			t.Fatalf("Run() = %v, want %v", err, tt.want)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("preflight started %d components", calls.Load())
	}
}
func TestRunCancelsSibling(t *testing.T) {
	sentinel := errors.New("api failed")
	started, canceled := make(chan struct{}), make(chan struct{})
	err := Run(context.Background(), RoleAll, Components{
		API:    ComponentFunc(func(context.Context) error { <-started; return sentinel }),
		Worker: ComponentFunc(func(ctx context.Context) error { close(started); <-ctx.Done(); close(canceled); return ctx.Err() }),
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run() = %v", err)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("sibling was not canceled")
	}
}
func TestRunParentCancellation(t *testing.T) {
	shutdownErr, cleanupErr := errors.New("shutdown failed"), errors.New("cleanup failed")
	for _, tt := range []struct{ returned, want error }{
		{nil, nil}, {shutdownErr, shutdownErr}, {errors.Join(context.Canceled, cleanupErr), cleanupErr},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		started, done := make(chan struct{}), make(chan error, 1)
		go func() {
			done <- runWithGrace(ctx, RoleAPI, Components{API: ComponentFunc(func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				if tt.returned != nil {
					return tt.returned
				}
				return ctx.Err()
			})}, time.Second)
		}()
		<-started
		cancel()
		err := <-done
		if tt.want == nil && err != nil || tt.want != nil && !errors.Is(err, tt.want) {
			t.Fatalf("Run() = %v, want %v", err, tt.want)
		}
	}
}
func TestRunShutdownTimeoutOrder(t *testing.T) {
	started := make(chan struct{}, 2)
	failWorker, releaseAPI := make(chan struct{}), make(chan struct{})
	workerErr := errors.New("worker-timeout-companion-error")
	done := make(chan error, 1)
	go func() {
		done <- runWithGrace(context.Background(), RoleAll, Components{
			API:    ComponentFunc(func(context.Context) error { started <- struct{}{}; <-releaseAPI; return nil }),
			Worker: ComponentFunc(func(context.Context) error { started <- struct{}{}; <-failWorker; return workerErr }),
		}, 10*time.Millisecond)
	}()
	for range 2 {
		<-started
	}
	close(failWorker)
	err := <-done
	close(releaseAPI)
	requireErrorOrder(t, err, ErrShutdownTimeout, workerErr)
	if !strings.Contains(err.Error(), "api") {
		t.Fatalf("timeout missing api slot: %v", err)
	}
}
func TestRunErrorOrder(t *testing.T) {
	started, release := make(chan struct{}, 2), make(chan struct{})
	apiErr, workerErr := errors.New("api-error"), errors.New("worker-error")
	component := func(err error) ComponentFunc {
		return func(context.Context) error { started <- struct{}{}; <-release; return err }
	}
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), RoleAll, Components{component(apiErr), component(workerErr)})
	}()
	for range 2 {
		<-started
	}
	close(release)
	requireErrorOrder(t, <-done, apiErr, workerErr)
}
func requireErrorOrder(t *testing.T, err, first, second error) {
	t.Helper()
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("error = %v, want both %v and %v", err, first, second)
	}
	text := err.Error()
	a, b := strings.Index(text, first.Error()), strings.Index(text, second.Error())
	if a < 0 || b < 0 || a >= b {
		t.Fatalf("error order = %q", text)
	}
}
