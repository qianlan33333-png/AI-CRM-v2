package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	cli, err := appruntime.ParseCLI(args)
	if err != nil {
		message := "aicrm: invalid arguments"
		if errors.Is(err, appruntime.ErrInvalidRole) {
			message = "aicrm: --role must be one of api, worker, all"
			if len(args) == 0 {
				message = "aicrm: --role is required"
			}
		}
		fmt.Fprintln(os.Stderr, message)
		fmt.Fprintln(os.Stderr, appruntime.UsageLine)
		return appruntime.ExitUsage
	}
	if cli.Help {
		fmt.Fprintln(os.Stdout, appruntime.UsageLine)
		return appruntime.ExitOK
	}
	startupConfig, err := appconfig.Load(cli.Role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aicrm: %v\n", err)
		return appruntime.ExitRuntime
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	processComponents, err := components(cli.Role, startupConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aicrm: initialize components: %v\n", err)
		return appruntime.ExitRuntime
	}
	if err := appruntime.Run(ctx, cli.Role, processComponents); err != nil {
		fmt.Fprintf(os.Stderr, "aicrm: %v\n", err)
		return appruntime.ExitRuntime
	}
	return appruntime.ExitOK
}
