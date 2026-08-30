package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type inventory struct {
	Items []inventoryItem `json:"items"`
}

type inventoryItem struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type planManifest struct {
	Version        int        `json:"version"`
	TargetDatabase string     `json:"target_database"`
	GeneratedAt    time.Time  `json:"generated_at"`
	Items          []planItem `json:"items"`
	ContentSHA256  string     `json:"content_sha256"`
}

type planItem struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Reason    string `json:"reason"`
}

func createPlan(ctx context.Context, config config) error {
	var source inventory
	if err := readJSON(config.inventory, &source); err != nil {
		return fmt.Errorf("read inventory: %w", err)
	}
	if len(source.Items) == 0 {
		return errors.New("cleanup inventory is empty")
	}
	manifest := planManifest{Version: 1, TargetDatabase: "aicrm_v2_core", GeneratedAt: time.Now().UTC()}
	seen := map[string]struct{}{}
	for _, item := range source.Items {
		if err := validateInventoryItem(item); err != nil {
			return err
		}
		key := item.Type + "\x00" + item.Name
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate cleanup object: %s %s", item.Type, item.Name)
		}
		seen[key] = struct{}{}
		size, err := resolveSize(ctx, config.databaseURL, item)
		if err != nil {
			return fmt.Errorf("resolve %s %s: %w", item.Type, item.Name, err)
		}
		manifest.Items = append(manifest.Items, planItem{Type: item.Type, Name: item.Name, SizeBytes: size, Reason: item.Reason})
	}
	digest, err := manifestDigest(manifest)
	if err != nil {
		return err
	}
	manifest.ContentSHA256 = digest
	if err = writeJSON(config.manifest, manifest); err != nil {
		return err
	}
	sealed, err := fileSHA256(config.manifest)
	if err != nil {
		return err
	}
	fmt.Printf("manifest=%s content_sha256=%s file_sha256=%s objects=%d\n", config.manifest, digest, sealed, len(manifest.Items))
	return nil
}

func manifestDigest(manifest planManifest) (string, error) {
	manifest.ContentSHA256 = ""
	payload, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func validateInventoryItem(item inventoryItem) error {
	if item.Reason == "" || strings.TrimSpace(item.Reason) != item.Reason {
		return errors.New("every cleanup object needs an exact reason")
	}
	if strings.ContainsAny(item.Name, "*$?[]{}\n\r\t") || strings.Contains(item.Name, "..") || strings.HasPrefix(item.Name, "~") {
		return fmt.Errorf("unsafe cleanup object name: %q", item.Name)
	}
	lower := strings.ToLower(item.Name)
	for _, protected := range []string{"aicrm_v2_core", "hxc", "frozen", "archive"} {
		if strings.Contains(lower, protected) {
			return fmt.Errorf("protected cleanup object: %q", item.Name)
		}
	}
	if strings.Contains(lower, "v1") || containsNameToken(lower, "aa") {
		return fmt.Errorf("protected cleanup object: %q", item.Name)
	}
	switch item.Type {
	case "database":
		if item.Name == "" || item.Name == "postgres" || strings.HasPrefix(lower, "template") {
			return fmt.Errorf("protected database: %q", item.Name)
		}
	case "container", "volume":
		if item.Name == "" || strings.Contains(item.Name, "/") {
			return fmt.Errorf("unsafe %s: %q", item.Type, item.Name)
		}
	case "path":
		clean := filepath.Clean(item.Name)
		workingDirectory, _ := os.Getwd()
		homeDirectory, _ := os.UserHomeDir()
		containsWorkingDirectory := workingDirectory != "" && (clean == workingDirectory || strings.HasPrefix(workingDirectory, clean+string(filepath.Separator)))
		if !filepath.IsAbs(clean) || clean != item.Name || clean == "/" || clean == homeDirectory || containsWorkingDirectory || len(strings.Split(strings.TrimPrefix(clean, "/"), "/")) < 3 || strings.Contains(lower, "/v1/") || strings.Contains(lower, "/aa/") {
			return fmt.Errorf("unsafe cleanup path: %q", item.Name)
		}
	default:
		return fmt.Errorf("unsupported cleanup object type: %q", item.Type)
	}
	return nil
}

func containsNameToken(value, token string) bool {
	for _, part := range strings.FieldsFunc(value, func(char rune) bool {
		return char == '/' || char == '-' || char == '_' || char == '.'
	}) {
		if part == token {
			return true
		}
	}
	return false
}

func resolveSize(ctx context.Context, databaseURL string, item inventoryItem) (int64, error) {
	switch item.Type {
	case "database":
		if databaseURL == "" {
			return 0, errors.New("AICRM_CLEANUP_ADMIN_DATABASE_URL is required for database objects")
		}
		pool, err := pgxpool.New(ctx, databaseURL)
		if err != nil {
			return 0, err
		}
		defer pool.Close()
		var size int64
		if err = pool.QueryRow(ctx, `SELECT pg_database_size(datname) FROM pg_database WHERE datname=$1`, item.Name).Scan(&size); err != nil {
			return 0, err
		}
		return size, nil
	case "container":
		output, err := exec.CommandContext(ctx, "docker", "container", "inspect", "--size", "--format", "{{.SizeRw}}", item.Name).Output()
		if err != nil {
			return 0, err
		}
		return strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	case "volume":
		output, err := exec.CommandContext(ctx, "docker", "volume", "inspect", "--format", "{{.Mountpoint}}", item.Name).Output()
		if err != nil {
			return 0, err
		}
		return directorySize(strings.TrimSpace(string(output)))
	case "path":
		return directorySize(item.Name)
	default:
		return 0, errors.New("unsupported cleanup object")
	}
}

func directorySize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			total += info.Size()
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return total, err
}
