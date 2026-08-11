package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

const usageLine = "Usage: aicrm-river-migrate --direction=up"

type migrationRunner func(context.Context, string) error

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, migrateUp)) }

func run(args []string, stdout, stderr io.Writer, migrate migrationRunner) int {
	if stdout == nil || stderr == nil {
		return appruntime.ExitUsage
	}
	if len(args) != 1 || args[0] != "--direction=up" || migrate == nil {
		fmt.Fprintln(stderr, "aicrm-river-migrate: invalid arguments")
		fmt.Fprintln(stderr, usageLine)
		return appruntime.ExitUsage
	}

	config, err := appconfig.Load(appruntime.RoleAPI)
	if err != nil {
		fmt.Fprintln(stderr, "aicrm-river-migrate: startup configuration invalid")
		return appruntime.ExitRuntime
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err = migrate(ctx, config.Database.URL.Value()); err != nil {
		fmt.Fprintln(stderr, "aicrm-river-migrate: migration failed")
		return appruntime.ExitRuntime
	}
	fmt.Fprintln(stdout, "aicrm-river-migrate: River migration up completed")
	return appruntime.ExitOK
}

func migrateUp(ctx context.Context, databaseURL string) error {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	poolConfig.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return errors.New("database unavailable")
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		return errors.New("database unavailable")
	}
	if err = platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		return errors.New("river migration failed")
	}
	return nil
}
