// Command aicrm-v1-import archives a complete V1 PostgreSQL public schema
// through one source read-only snapshot. It does not run domain migrations or
// activate any imported queue, session, webhook, or provider work.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func main() {
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if err := run(os.Args[1:], v1archive.Runtime{
		SourceDatabaseURL: environment.SourceDatabaseURL,
		TargetDatabaseURL: environment.TargetDatabaseURL,
		SourceHMACKey:     environment.SourceHMACKey,
		ArchiveKey:        environment.ArchiveKey,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "aicrm-v1-import:", err)
		os.Exit(2)
	}
}

func run(args []string, environment v1archive.Runtime) error {
	flags := flag.NewFlagSet("aicrm-v1-import", flag.ContinueOnError)
	modeValue := flags.String("mode", "", "preflight|full|reconcile")
	runID := flags.String("run-id", "", "required for full and reconcile")
	dumpPath := flags.String("source-dump-path", "", "required path to the immutable V1 dump")
	repositorySHA := flags.String("repository-sha", "", "required 40-character V1 repository commit SHA")
	batchSize := flags.Int("batch-size", 200, "encrypted archive records per target transaction")
	if err := flags.Parse(args); err != nil {
		return err
	}
	mode := v1archive.Mode(*modeValue)
	if !mode.Valid() || environment.SourceDatabaseURL == "" {
		return fmt.Errorf("mode and AICRM_V1_ARCHIVE_SOURCE_DATABASE_URL are required")
	}
	if mode != v1archive.ModePreflight {
		if environment.TargetDatabaseURL == "" || environment.SourceHMACKey == "" || environment.ArchiveKey == "" || *runID == "" || *dumpPath == "" || *repositorySHA == "" {
			return fmt.Errorf("target runtime, run-id, source-dump-path and repository-sha are required for %s", mode)
		}
		if configuredSameDatabase(environment.SourceDatabaseURL, environment.TargetDatabaseURL) {
			return v1archive.ErrSameDatabase
		}
	}
	var dumpDigest [sha256.Size]byte
	if mode != v1archive.ModePreflight {
		computed, err := digestRegularFile(*dumpPath)
		if err != nil {
			return fmt.Errorf("source-dump-path: %w", err)
		}
		dumpDigest = computed
	}
	ctx := context.Background()
	source, err := v1archive.OpenPostgresSource(ctx, environment.SourceDatabaseURL)
	if err != nil {
		return err
	}
	defer source.Close()
	var target *v1archive.PostgresTarget
	var writer v1archive.TargetWriter
	if mode != v1archive.ModePreflight {
		target, err = v1archive.OpenPostgresTarget(ctx, environment.TargetDatabaseURL)
		if err != nil {
			return err
		}
		defer target.Close()
		writer = target
	}
	config := v1archive.Config{SourceHMACKey: []byte(environment.SourceHMACKey), ArchiveKey: []byte(environment.ArchiveKey), ArchiveKeyVersion: 1, BatchSize: *batchSize}
	run := v1archive.Run{ID: *runID, AdapterID: v1archive.DefaultAdapterID, SourceDumpDigest: dumpDigest, RepositorySHA: *repositorySHA}
	result, err := v1archive.Execute(ctx, config, mode, run, source, writer)
	if err != nil {
		return err
	}
	return printResult(result)
}

func configuredSameDatabase(sourceDSN, targetDSN string) bool {
	if strings.TrimSpace(sourceDSN) == strings.TrimSpace(targetDSN) {
		return true
	}
	source, sourceErr := pgxpool.ParseConfig(sourceDSN)
	target, targetErr := pgxpool.ParseConfig(targetDSN)
	if sourceErr != nil || targetErr != nil {
		return false
	}
	return source.ConnConfig.Host == target.ConnConfig.Host && source.ConnConfig.Port == target.ConnConfig.Port && source.ConnConfig.Database == target.ConnConfig.Database
}

func digestRegularFile(path string) ([sha256.Size]byte, error) {
	var result [sha256.Size]byte
	info, err := os.Lstat(path)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("regular non-symlink file required")
	}
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return result, err
	}
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func printResult(result v1archive.Result) error {
	rowCount := int64(0)
	for _, table := range result.Manifest.Tables {
		rowCount += table.RowCount
	}
	payload := struct {
		Mode           v1archive.Mode `json:"mode"`
		TableCount     int            `json:"table_count"`
		RowCount       int64          `json:"row_count"`
		ManifestSHA256 string         `json:"manifest_sha256"`
	}{Mode: result.Mode, TableCount: len(result.Manifest.Tables), RowCount: rowCount, ManifestSHA256: hex.EncodeToString(result.ManifestDigest[:])}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
