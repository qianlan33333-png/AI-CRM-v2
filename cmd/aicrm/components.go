package main

import (
	"context"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func components(_ appconfig.Root) appruntime.Components {
	wait := appruntime.ComponentFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	return appruntime.Components{API: wait, Worker: wait}
}
