package main

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestRunRejectsSameConfiguredDSNBeforeOpeningConnections(t *testing.T) {
	runtime := v1archive.Runtime{SourceDatabaseURL: "postgres://reader@db.example/legacy", TargetDatabaseURL: "postgres://reader@db.example/legacy", SourceHMACKey: strings.Repeat("h", 32), ArchiveKey: strings.Repeat("a", 32)}
	err := run([]string{"--mode=full", "--run-id=v1-full", "--source-dump-path=/does/not/matter", "--repository-sha=0123456789abcdef0123456789abcdef01234567"}, runtime)
	if !errors.Is(err, v1archive.ErrSameDatabase) {
		t.Fatalf("same DSN error = %v", err)
	}
}

func TestDigestRegularFileUsesFileContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.dump")
	contents := []byte("immutable-v1-snapshot")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := digestRegularFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := sha256.Sum256(contents); got != want {
		t.Fatalf("digest = %x, want %x", got, want)
	}
}

func TestConfiguredSameDatabaseRecognizesDifferentCredentials(t *testing.T) {
	if !configuredSameDatabase("postgres://reader@db.example:5432/legacy", "postgres://writer@db.example:5432/legacy") {
		t.Fatal("same server/database with different credentials accepted")
	}
	if configuredSameDatabase("postgres://reader@db.example:5432/legacy", "postgres://writer@db.example:5432/aicrm") {
		t.Fatal("different database rejected")
	}
}
