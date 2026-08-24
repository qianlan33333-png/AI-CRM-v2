// Command aicrm-dm01 starts one operator-controlled DM01 local import mode.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
)

const (
	sourceDatabaseURLEnv = "AICRM_DM01_SOURCE_DATABASE_URL"
	targetDatabaseURLEnv = "AICRM_DATABASE_URL"
	hmacKeyEnv           = "AICRM_DM01_SOURCE_HMAC_KEY"
	archiveKeyEnv        = "AICRM_DM01_ARCHIVE_KEY"
)

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "aicrm-dm01:", err)
		os.Exit(2)
	}
}

func run(args []string, getenv func(string) string) error {
	flags := flag.NewFlagSet("aicrm-dm01", flag.ContinueOnError)
	mode := flags.String("mode", "", "preflight|full|incremental|reconcile")
	manifestPath := flags.String("source-manifest", "", "manifest path")
	manifestDigest := flags.String("manifest-sha256", "", "manifest sha256")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !migration.RunMode(*mode).Valid() || *manifestPath == "" || *manifestDigest == "" {
		return fmt.Errorf("mode, source-manifest and manifest-sha256 are required")
	}
	source, target, hmacKey, archiveKey := getenv(sourceDatabaseURLEnv), getenv(targetDatabaseURLEnv), getenv(hmacKeyEnv), getenv(archiveKeyEnv)
	if source == "" || target == "" || hmacKey == "" || archiveKey == "" {
		return fmt.Errorf("DM01 runtime configuration is incomplete")
	}
	if source == target {
		return fmt.Errorf("source and target database URLs must differ")
	}
	if sha256.Sum256([]byte(hmacKey)) == sha256.Sum256([]byte(archiveKey)) {
		return fmt.Errorf("source HMAC and archive keys must differ")
	}
	_, _, err := migration.LoadManifest(*manifestPath, *manifestDigest)
	return err
}
