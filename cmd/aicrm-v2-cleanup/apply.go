package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type gateReceipt struct {
	Status         string `json:"status"`
	TargetDatabase string `json:"target_database"`
	SourceDigest   string `json:"source_digest"`
	ReleaseSHA     string `json:"release_sha"`
}

func applyPlan(ctx context.Context, config config) error {
	fileDigest, err := fileSHA256(config.manifest)
	if err != nil || fileDigest != config.manifestSHA256 {
		return errors.New("cleanup manifest file SHA-256 does not match the sealed plan")
	}
	var manifest planManifest
	if err := readJSON(config.manifest, &manifest); err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	digest, err := manifestDigest(manifest)
	if err != nil || manifest.Version != 1 || manifest.TargetDatabase != "aicrm_v2_core" || manifest.ContentSHA256 != digest || len(manifest.Items) == 0 {
		return errors.New("cleanup manifest is invalid or has been modified")
	}
	for _, item := range manifest.Items {
		if err = validateInventoryItem(inventoryItem{Type: item.Type, Name: item.Name, Reason: item.Reason}); err != nil || item.SizeBytes < 0 {
			return errors.New("cleanup manifest contains an unsafe object")
		}
	}
	reconciliation, smoke, err := readGates(config.reconciliation, config.smoke)
	if err != nil {
		return err
	}
	if reconciliation.TargetDatabase != manifest.TargetDatabase || smoke.TargetDatabase != manifest.TargetDatabase || reconciliation.SourceDigest != smoke.SourceDigest || reconciliation.ReleaseSHA != smoke.ReleaseSHA {
		return errors.New("reconciliation and smoke receipts do not describe the same release")
	}
	for _, item := range manifest.Items {
		if err = applyItem(ctx, config.databaseURL, item); err != nil {
			return fmt.Errorf("delete %s %s: %w", item.Type, item.Name, err)
		}
		fmt.Printf("deleted type=%s name=%s planned_size_bytes=%d reason=%q\n", item.Type, item.Name, item.SizeBytes, item.Reason)
	}
	return nil
}

func readGates(reconciliationPath, smokePath string) (gateReceipt, gateReceipt, error) {
	var reconciliation, smoke gateReceipt
	if err := readJSON(reconciliationPath, &reconciliation); err != nil {
		return gateReceipt{}, gateReceipt{}, fmt.Errorf("read reconciliation receipt: %w", err)
	}
	if err := readJSON(smokePath, &smoke); err != nil {
		return gateReceipt{}, gateReceipt{}, fmt.Errorf("read smoke receipt: %w", err)
	}
	for _, receipt := range []gateReceipt{reconciliation, smoke} {
		if receipt.Status != "passed" || receipt.TargetDatabase != "aicrm_v2_core" || !isSHA256(receipt.SourceDigest) || !validReleaseSHA(receipt.ReleaseSHA) {
			return gateReceipt{}, gateReceipt{}, errors.New("cleanup gate receipt is not passed and complete")
		}
	}
	return reconciliation, smoke, nil
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

func applyItem(ctx context.Context, databaseURL string, item planItem) error {
	switch item.Type {
	case "database":
		if databaseURL == "" {
			return errors.New("AICRM_CLEANUP_ADMIN_DATABASE_URL is required")
		}
		pool, err := pgxpool.New(ctx, databaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		statement := "DROP DATABASE " + pgx.Identifier{item.Name}.Sanitize() + " WITH (FORCE)"
		_, err = pool.Exec(ctx, statement)
		return err
	case "container":
		return exec.CommandContext(ctx, "docker", "container", "rm", "--force", "--", item.Name).Run()
	case "volume":
		return exec.CommandContext(ctx, "docker", "volume", "rm", "--", item.Name).Run()
	case "path":
		return os.RemoveAll(filepath.Clean(item.Name))
	default:
		return errors.New("unsupported cleanup object")
	}
}

func validReleaseSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
