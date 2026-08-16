package domainverification

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReaderServesOnlyValidFiles(t *testing.T) {
	root := t.TempDir()
	reader, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"WW_verify_alpha-1.txt": " \nverify-ww\t ",
		"MP_verify_beta_2.txt":  "\nverify-mp\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{"WW_verify_alpha-1.txt": "verify-ww", "MP_verify_beta_2.txt": "verify-mp"} {
		got, err := reader.Read(name)
		if err != nil || got != want {
			t.Fatalf("Read(%q) = %q, %v; want %q, nil", name, got, err, want)
		}
	}
}

func TestReaderFailsClosedForUnsafeInputs(t *testing.T) {
	root := t.TempDir()
	reader, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "WW_verify_directory.txt"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "WW_verify_nonutf8.txt"), []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "WW_verify_large.txt"), []byte(strings.Repeat("x", MaxFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "WW_verify_outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "WW_verify_link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"WW_verify_directory.txt", "WW_verify_nonutf8.txt", "WW_verify_large.txt", "WW_verify_link.txt",
		"WW_verify_missing.txt", "../WW_verify_alpha.txt", "WW_verify_alpha.txt/extra", "MP_verify_.txt", "not-a-verification.txt",
	} {
		if _, err := reader.Read(name); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Read(%q) error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestReaderRejectsUnsafeConfigurationAndMissingDirectory(t *testing.T) {
	for _, directory := range []string{"relative", " " + t.TempDir(), "/"} {
		if _, err := New(directory); !errors.Is(err, ErrUnsafeConfig) {
			t.Fatalf("New(%q) error = %v, want ErrUnsafeConfig", directory, err)
		}
	}
	linkedRoot := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(t.TempDir(), linkedRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := New(linkedRoot); !errors.Is(err, ErrUnsafeConfig) {
		t.Fatalf("New(symlink) error = %v, want ErrUnsafeConfig", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	reader, err := New(missing)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read("WW_verify_missing.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing root read error = %v, want ErrNotFound", err)
	}
	if err := os.Mkdir(missing, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read("WW_verify_missing.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read failure error = %v, want ErrNotFound", err)
	}
}
