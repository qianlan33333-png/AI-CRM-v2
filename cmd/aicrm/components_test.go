package main

import (
	"context"
	"errors"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func TestBuildComponentsConstructsOnlySelectedRole(t *testing.T) {
	for _, test := range []struct {
		role                appruntime.Role
		wantAPI, wantWorker int
	}{
		{role: appruntime.RoleAPI, wantAPI: 1},
		{role: appruntime.RoleWorker, wantWorker: 1},
		{role: appruntime.RoleAll, wantAPI: 1, wantWorker: 1},
	} {
		t.Run(string(test.role), func(t *testing.T) {
			apiCalls, workerCalls := 0, 0
			component := appruntime.ComponentFunc(func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
			got, err := buildComponents(test.role, appconfig.Root{}, componentBuilders{
				api: func(appconfig.Root) (appruntime.Component, error) {
					apiCalls++
					return component, nil
				},
				worker: func(appconfig.Root) (appruntime.Component, error) {
					workerCalls++
					return component, nil
				},
			})
			if err != nil {
				t.Fatalf("buildComponents() error = %v", err)
			}
			if apiCalls != test.wantAPI || workerCalls != test.wantWorker {
				t.Fatalf("builders called api/worker = %d/%d, want %d/%d", apiCalls, workerCalls, test.wantAPI, test.wantWorker)
			}
			if (got.API != nil) != (test.wantAPI == 1) || (got.Worker != nil) != (test.wantWorker == 1) {
				t.Fatalf("components = %#v", got)
			}
		})
	}
}

func TestBuildComponentsFailsBeforeConstructingUnselectedRole(t *testing.T) {
	sentinel := errors.New("worker constructor failed")
	apiCalls, workerCalls := 0, 0
	_, err := buildComponents(appruntime.RoleWorker, appconfig.Root{}, componentBuilders{
		api: func(appconfig.Root) (appruntime.Component, error) {
			apiCalls++
			return nil, nil
		},
		worker: func(appconfig.Root) (appruntime.Component, error) {
			workerCalls++
			return nil, sentinel
		},
	})
	if !errors.Is(err, sentinel) || apiCalls != 0 || workerCalls != 1 {
		t.Fatalf("buildComponents() error/calls = %v %d/%d", err, apiCalls, workerCalls)
	}
	_, err = buildComponents(appruntime.Role("invalid"), appconfig.Root{}, componentBuilders{
		api: func(appconfig.Root) (appruntime.Component, error) {
			apiCalls++
			return nil, nil
		},
		worker: func(appconfig.Root) (appruntime.Component, error) {
			workerCalls++
			return nil, nil
		},
	})
	if !errors.Is(err, appruntime.ErrInvalidRole) || apiCalls != 0 || workerCalls != 1 {
		t.Fatalf("invalid role error/calls = %v %d/%d", err, apiCalls, workerCalls)
	}
}
