package main

import (
	"context"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func components() appruntime.Components {
	wait := appruntime.ComponentFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	return appruntime.Components{API: wait, Worker: wait}
}
