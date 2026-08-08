//go:build p0s01_acceptance

package p0s01_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func TestParseCLIContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    appruntime.CLIResult
		wantErr bool
	}{
		{name: "api equals", args: []string{"--role=api"}, want: appruntime.CLIResult{Role: appruntime.RoleAPI}},
		{name: "worker split", args: []string{"--role", "worker"}, want: appruntime.CLIResult{Role: appruntime.RoleWorker}},
		{name: "all", args: []string{"--role=all"}, want: appruntime.CLIResult{Role: appruntime.RoleAll}},
		{name: "help long", args: []string{"--help"}, want: appruntime.CLIResult{Help: true}},
		{name: "help short", args: []string{"-h"}, want: appruntime.CLIResult{Help: true}},
		{name: "missing", wantErr: true},
		{name: "unknown", args: []string{"--role=unknown"}, wantErr: true},
		{name: "uppercase", args: []string{"--role=API"}, wantErr: true},
		{name: "whitespace", args: []string{"--role", " api "}, wantErr: true},
		{name: "combined", args: []string{"--role=api,worker"}, wantErr: true},
		{name: "duplicate", args: []string{"--role=api", "--role=worker"}, wantErr: true},
		{name: "position", args: []string{"api"}, wantErr: true},
		{name: "unknown flag", args: []string{"--debug"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := appruntime.ParseCLI(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseCLI() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCLI() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseCLI() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRunRoleSelectionAndConcurrency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		role       appruntime.Role
		wantAPI    int32
		wantWorker int32
	}{
		{name: "api", role: appruntime.RoleAPI, wantAPI: 1},
		{name: "worker", role: appruntime.RoleWorker, wantWorker: 1},
		{name: "all", role: appruntime.RoleAll, wantAPI: 1, wantWorker: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			started := make(chan string, 2)
			var apiCalls atomic.Int32
			var workerCalls atomic.Int32
			components := appruntime.Components{
				API: appruntime.ComponentFunc(func(ctx context.Context) error {
					apiCalls.Add(1)
					started <- "api"
					<-ctx.Done()
					return ctx.Err()
				}),
				Worker: appruntime.ComponentFunc(func(ctx context.Context) error {
					workerCalls.Add(1)
					started <- "worker"
					<-ctx.Done()
					return ctx.Err()
				}),
			}

			done := make(chan error, 1)
			go func() { done <- appruntime.Run(ctx, tt.role, components) }()

			wantStarts := int(tt.wantAPI + tt.wantWorker)
			for range wantStarts {
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("selected components did not start concurrently")
				}
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Run() did not stop after cancellation")
			}
			if got := apiCalls.Load(); got != tt.wantAPI {
				t.Fatalf("API calls = %d, want %d", got, tt.wantAPI)
			}
			if got := workerCalls.Load(); got != tt.wantWorker {
				t.Fatalf("Worker calls = %d, want %d", got, tt.wantWorker)
			}
		})
	}
}

func TestRunRejectsMissingAllComponentBeforeStart(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := appruntime.Run(context.Background(), appruntime.RoleAll, appruntime.Components{
		API: appruntime.ComponentFunc(func(context.Context) error {
			calls.Add(1)
			return nil
		}),
	})
	if !errors.Is(err, appruntime.ErrMissingComponent) {
		t.Fatalf("Run() error = %v, want ErrMissingComponent", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("API calls = %d, want 0", got)
	}
}

func TestRunClassifiesUnexpectedStop(t *testing.T) {
	t.Parallel()

	done := make(chan error, 1)
	go func() {
		done <- appruntime.Run(context.Background(), appruntime.RoleAPI, appruntime.Components{
			API: appruntime.ComponentFunc(func(context.Context) error { return nil }),
		})
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not classify an early component stop")
	}
	if !errors.Is(err, appruntime.ErrUnexpectedStop) {
		t.Fatalf("Run() error = %v, want ErrUnexpectedStop", err)
	}
}

func TestRunCancelsSiblingOnError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("api failed")
	workerStarted := make(chan struct{})
	workerCanceled := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- appruntime.Run(context.Background(), appruntime.RoleAll, appruntime.Components{
			API: appruntime.ComponentFunc(func(context.Context) error {
				<-workerStarted
				return sentinel
			}),
			Worker: appruntime.ComponentFunc(func(ctx context.Context) error {
				close(workerStarted)
				<-ctx.Done()
				close(workerCanceled)
				return ctx.Err()
			}),
		})
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() blocked instead of starting both all-role components")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run() error = %v, want sentinel", err)
	}
	select {
	case <-workerCanceled:
	case <-time.After(time.Second):
		t.Fatal("worker sibling was not canceled")
	}
}
