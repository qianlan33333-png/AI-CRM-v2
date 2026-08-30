package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const usage = "Usage: aicrm-whitelist-import --mode=<digest|import|reconcile> [--run-id=<id>]"

type cliConfig struct {
	mode            string
	runID           string
	sourceURL       string
	targetURL       string
	sourceDigest    string
	archiveRunID    string
	allowTestName   bool
	allowTestSource bool
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "aicrm-whitelist-import:", err)
		os.Exit(2)
	}
}

func run(ctx context.Context, args []string) error {
	config, err := parseConfig(args, os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Hour)
	defer cancel()

	switch config.mode {
	case "digest":
		result, digestErr := sourceDigest(ctx, config.sourceURL, config.archiveRunID, config.allowTestSource)
		if digestErr != nil {
			return digestErr
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "import":
		result, importErr := importWhitelist(ctx, config)
		if importErr != nil {
			return importErr
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "reconcile":
		pool, poolErr := pgxpool.New(ctx, config.targetURL)
		if poolErr != nil {
			return errors.New("target database unavailable")
		}
		defer pool.Close()
		result, reconcileErr := reconcileWhitelist(ctx, pool, config.runID)
		if reconcileErr != nil {
			return reconcileErr
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	default:
		return errors.New(usage)
	}
}

func parseConfig(args []string, getenv func(string) string) (cliConfig, error) {
	flags := flag.NewFlagSet("aicrm-whitelist-import", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	mode := flags.String("mode", "", "digest|import|reconcile")
	runID := flags.String("run-id", "", "stable whitelist import run id")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return cliConfig{}, errors.New(usage)
	}
	config := cliConfig{
		mode: *mode, runID: *runID,
		sourceURL:       strings.TrimSpace(getenv("AICRM_WHITELIST_SOURCE_DATABASE_URL")),
		targetURL:       strings.TrimSpace(getenv("AICRM_DATABASE_URL")),
		sourceDigest:    strings.TrimSpace(getenv("AICRM_WHITELIST_SOURCE_DIGEST")),
		archiveRunID:    strings.TrimSpace(getenv("AICRM_WHITELIST_ARCHIVE_RUN_ID")),
		allowTestName:   getenv("AICRM_WHITELIST_ALLOW_TEST_TARGET") == "1",
		allowTestSource: getenv("AICRM_WHITELIST_ALLOW_TEST_SOURCE") == "1",
	}
	if config.mode != "digest" && config.mode != "import" && config.mode != "reconcile" {
		return cliConfig{}, errors.New(usage)
	}
	if (config.mode == "digest" || config.mode == "import") && config.sourceURL == "" {
		return cliConfig{}, errors.New("AICRM_WHITELIST_SOURCE_DATABASE_URL is required")
	}
	if (config.mode == "digest" || config.mode == "import") && config.archiveRunID == "" && !config.allowTestSource {
		return cliConfig{}, errors.New("AICRM_WHITELIST_ARCHIVE_RUN_ID is required")
	}
	if (config.mode == "import" || config.mode == "reconcile") && config.targetURL == "" {
		return cliConfig{}, errors.New("AICRM_DATABASE_URL is required")
	}
	if config.mode != "digest" && !validRunID(config.runID) {
		return cliConfig{}, errors.New("--run-id must match wli_[A-Za-z0-9_-]{8,80}")
	}
	if config.mode == "import" && !isSHA256(config.sourceDigest) {
		return cliConfig{}, errors.New("AICRM_WHITELIST_SOURCE_DIGEST must be 64 lowercase hex characters")
	}
	return config, nil
}

func validRunID(value string) bool {
	if !strings.HasPrefix(value, "wli_") || len(value) < 12 || len(value) > 84 {
		return false
	}
	for _, char := range value[4:] {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
