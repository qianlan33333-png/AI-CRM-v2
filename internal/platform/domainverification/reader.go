// Package domainverification serves the narrowly-scoped WeChat verification
// files from an operator-controlled directory outside the repository.
package domainverification

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	EnvironmentDirectory = "AICRM_DOMAIN_VERIFICATION_DIR"
	DefaultDirectory     = "/var/lib/aicrm/domain-verification"
	MaxFileBytes         = 64 << 10
)

var (
	ErrNotFound     = errors.New("domain verification file not found")
	ErrUnsafeConfig = errors.New("unsafe domain verification configuration")
	verifyFileName  = regexp.MustCompile(`^(?:WW|MP)_verify_[A-Za-z0-9_-]+\.txt$`)
)

// Reader is immutable after construction. The configured directory is always
// absolute, canonical, and opened without following a symlink for each read.
type Reader struct{ root string }

// New rejects paths that could make the process read a repository, working
// directory, or an indeterminate symlink target. A missing directory remains a
// valid disabled state and returns a public 404 at request time.
func New(directory string) (*Reader, error) {
	if directory == "" {
		directory = DefaultDirectory
	}
	if strings.TrimSpace(directory) != directory || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || directory == string(filepath.Separator) {
		return nil, ErrUnsafeConfig
	}
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return &Reader{root: directory}, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, ErrUnsafeConfig
	}
	root, err := canonicalDirectory(directory)
	if err != nil {
		return nil, ErrUnsafeConfig
	}
	return &Reader{root: root}, nil
}

// Read returns only a valid, regular UTF-8 verification file. Every failure
// deliberately collapses to ErrNotFound so filesystem shape is never exposed.
func (reader *Reader) Read(filename string) (string, error) {
	if reader == nil || !verifyFileName.MatchString(filename) {
		return "", ErrNotFound
	}
	content, err := readRegularFile(reader.root, filename, MaxFileBytes)
	if err != nil {
		return "", ErrNotFound
	}
	return strings.TrimSpace(string(content)), nil
}
