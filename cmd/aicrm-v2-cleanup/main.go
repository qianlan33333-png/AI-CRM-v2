package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const usage = "Usage: aicrm-v2-cleanup (--plan --inventory=<file> --manifest=<file> | --apply --manifest=<file> --manifest-sha256=<sha256> --reconciliation=<file> --smoke=<file>)"

type config struct {
	plan, apply           bool
	inventory, manifest   string
	reconciliation, smoke string
	databaseURL           string
	manifestSHA256        string
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "aicrm-v2-cleanup:", err)
		os.Exit(2)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string) error {
	config, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if config.plan {
		return createPlan(ctx, config)
	}
	return applyPlan(ctx, config)
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	set := flag.NewFlagSet("aicrm-v2-cleanup", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	var value config
	set.BoolVar(&value.plan, "plan", false, "resolve exact cleanup inventory")
	set.BoolVar(&value.apply, "apply", false, "apply an unchanged cleanup manifest")
	set.StringVar(&value.inventory, "inventory", "", "exact inventory JSON")
	set.StringVar(&value.manifest, "manifest", "", "sealed plan JSON")
	set.StringVar(&value.manifestSHA256, "manifest-sha256", "", "exact file SHA-256 printed by --plan")
	set.StringVar(&value.reconciliation, "reconciliation", "", "passed reconciliation receipt JSON")
	set.StringVar(&value.smoke, "smoke", "", "passed id-dev smoke receipt JSON")
	if err := set.Parse(args); err != nil || set.NArg() != 0 || value.plan == value.apply || value.manifest == "" {
		return config{}, errors.New(usage)
	}
	if value.plan && (value.inventory == "" || value.manifestSHA256 != "" || value.reconciliation != "" || value.smoke != "") {
		return config{}, errors.New(usage)
	}
	if value.apply && (value.inventory != "" || !isSHA256(value.manifestSHA256) || value.reconciliation == "" || value.smoke == "") {
		return config{}, errors.New(usage)
	}
	value.databaseURL = strings.TrimSpace(getenv("AICRM_CLEANUP_ADMIN_DATABASE_URL"))
	return value, nil
}

func readJSON(path string, target any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}

func writeJSON(path string, value any) error {
	if !filepath.IsAbs(path) {
		return errors.New("output manifest path must be absolute")
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o600)
}

func fileSHA256(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
