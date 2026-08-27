// datamigration_manifest captures schema-only inventory and reconciliation
// evidence for a V1-to-V2 migration. It deliberately has no data-export mode.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/datamigration/manifest"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "datamigration_manifest:", err)
		os.Exit(1)
	}
}

func run(arguments []string, stdout, stderr io.Writer) error {
	if len(arguments) == 0 {
		return usage(stderr)
	}
	switch arguments[0] {
	case "collect":
		return collect(arguments[1:], stdout)
	case "compare":
		return compare(arguments[1:], stdout)
	case "reconcile":
		return reconcile(arguments[1:], stdout)
	case "-h", "--help", "help":
		return usage(stdout)
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func usage(writer io.Writer) error {
	_, err := fmt.Fprintln(writer, `usage:
  datamigration_manifest collect --database-url <url> [--schemas public,other] --output <manifest.json>
  datamigration_manifest compare --left <manifest.json> --right <manifest.json> --output <comparison.json>
  datamigration_manifest reconcile --source <manifest.json> [--target <manifest.json>] --dispositions <dispositions.json> --output <report.json>

collect only reads PostgreSQL catalogs plus COUNT(*) and MAX(timestamp/date) aggregates.
The database URL is never written to manifests or reports.`)
	return err
}

func collect(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("collect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	databaseURL := flags.String("database-url", "", "PostgreSQL URL")
	schemaCSV := flags.String("schemas", "public", "comma-separated schemas")
	output := flags.String("output", "", "output manifest path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("--output is required")
	}
	result, err := manifest.Collect(context.Background(), *databaseURL, splitCSV(*schemaCSV))
	if err != nil {
		return err
	}
	if err := writeJSON(*output, result); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "wrote %d tables to %s\n", len(result.Tables), *output)
	return err
}

func compare(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	leftPath := flags.String("left", "", "left manifest path")
	rightPath := flags.String("right", "", "right manifest path")
	output := flags.String("output", "", "comparison output path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *leftPath == "" || *rightPath == "" || *output == "" {
		return errors.New("--left, --right and --output are required")
	}
	left, err := readManifest(*leftPath)
	if err != nil {
		return err
	}
	right, err := readManifest(*rightPath)
	if err != nil {
		return err
	}
	result, err := manifest.Compare(left, right)
	if err != nil {
		return err
	}
	if err := writeJSON(*output, result); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "wrote comparison to %s\n", *output)
	return err
}

func reconcile(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sourcePath := flags.String("source", "", "source manifest path")
	targetPath := flags.String("target", "", "optional target manifest path")
	dispositionsPath := flags.String("dispositions", "", "disposition document path")
	output := flags.String("output", "", "reconciliation output path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *sourcePath == "" || *dispositionsPath == "" || *output == "" {
		return errors.New("--source, --dispositions and --output are required")
	}
	source, err := readManifest(*sourcePath)
	if err != nil {
		return err
	}
	var target *manifest.Manifest
	if *targetPath != "" {
		value, readErr := readManifest(*targetPath)
		if readErr != nil {
			return readErr
		}
		target = &value
	}
	var dispositions manifest.DispositionDocument
	if err := readJSON(*dispositionsPath, &dispositions); err != nil {
		return fmt.Errorf("read dispositions: %w", err)
	}
	result, err := manifest.Reconcile(source, target, dispositions)
	if err != nil {
		return err
	}
	if err := writeJSON(*output, result); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "wrote reconciliation to %s (complete=%t)\n", *output, result.SourceCountEqualsTerminalDisposition)
	return err
}

func splitCSV(value string) []string {
	var values []string
	for _, candidate := range strings.Split(value, ",") {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			values = append(values, candidate)
		}
	}
	return values
}

func readManifest(path string) (manifest.Manifest, error) {
	var value manifest.Manifest
	if err := readJSON(path, &value); err != nil {
		return manifest.Manifest{}, fmt.Errorf("read manifest %q: %w", path, err)
	}
	return value, nil
}

func readJSON(path string, destination any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
