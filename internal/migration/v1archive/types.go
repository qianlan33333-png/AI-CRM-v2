// Package v1archive provides the non-domain-specific, read-only V1 archive
// import. It never activates legacy queues, jobs, sessions, or provider work.
package v1archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrInvalidConfig        = errors.New("invalid V1 archive configuration")
	ErrSameDatabase         = errors.New("V1 archive source and target are the same database")
	ErrSourceNotReadOnly    = errors.New("V1 archive source is not read-only")
	ErrInvalidManifest      = errors.New("invalid V1 archive source manifest")
	ErrMissingPrimaryKey    = errors.New("V1 archive source table has no primary key")
	ErrPayloadConflict      = errors.New("V1 archive source key has conflicting payload")
	ErrRunConflict          = errors.New("V1 archive run conflicts with stored run")
	ErrReconciliationFailed = errors.New("V1 archive reconciliation failed")
)

type Mode string

const (
	ModePreflight Mode = "preflight"
	ModeFull      Mode = "full"
	ModeReconcile Mode = "reconcile"
)

func (mode Mode) Valid() bool {
	return mode == ModePreflight || mode == ModeFull || mode == ModeReconcile
}

type Config struct {
	SourceHMACKey     []byte
	ArchiveKey        []byte
	ArchiveKeyVersion int
	BatchSize         int
}

func (config Config) Validate() error {
	if len(config.SourceHMACKey) < sha256.Size || len(config.ArchiveKey) != 32 || config.ArchiveKeyVersion < 1 || config.BatchSize < 1 || config.BatchSize > 10_000 {
		return ErrInvalidConfig
	}
	if equalBytes(config.SourceHMACKey, config.ArchiveKey) {
		return ErrInvalidConfig
	}
	return nil
}

type Column struct {
	Ordinal int32  `json:"ordinal"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"not_null"`
}

type Table struct {
	Name       string   `json:"name"`
	Columns    []Column `json:"columns"`
	PrimaryKey []string `json:"primary_key"`
	RowCount   int64    `json:"row_count"`
}

func (table Table) Validate() error {
	if strings.TrimSpace(table.Name) != table.Name || table.Name == "" || len(table.Columns) == 0 || table.RowCount < 0 {
		return ErrInvalidManifest
	}
	columns := make(map[string]struct{}, len(table.Columns))
	var previous int32
	for _, column := range table.Columns {
		if column.Ordinal <= previous || strings.TrimSpace(column.Name) != column.Name || column.Name == "" || strings.TrimSpace(column.Type) != column.Type || column.Type == "" {
			return fmt.Errorf("%w: invalid column in %s", ErrInvalidManifest, table.Name)
		}
		if _, exists := columns[column.Name]; exists {
			return fmt.Errorf("%w: duplicate column in %s", ErrInvalidManifest, table.Name)
		}
		columns[column.Name] = struct{}{}
		previous = column.Ordinal
	}
	seen := make(map[string]struct{}, len(table.PrimaryKey))
	for _, column := range table.PrimaryKey {
		if _, exists := columns[column]; !exists {
			return fmt.Errorf("%w: primary key column %s is absent from %s", ErrInvalidManifest, column, table.Name)
		}
		if _, exists := seen[column]; exists {
			return fmt.Errorf("%w: duplicate primary key column in %s", ErrInvalidManifest, table.Name)
		}
		seen[column] = struct{}{}
	}
	return nil
}

type SourceIdentity struct {
	SystemID string `json:"system_id"`
	Database string `json:"database"`
	Role     string `json:"role"`
}

func (identity SourceIdentity) Validate() error {
	if strings.TrimSpace(identity.SystemID) != identity.SystemID || identity.SystemID == "" || strings.TrimSpace(identity.Database) != identity.Database || identity.Database == "" || strings.TrimSpace(identity.Role) != identity.Role || identity.Role == "" || strings.Contains(identity.SystemID+identity.Database+identity.Role, "/") {
		return ErrInvalidManifest
	}
	return nil
}

func (identity SourceIdentity) Equal(other SourceIdentity) bool {
	return identity.SystemID == other.SystemID && identity.Database == other.Database
}

type Manifest struct {
	Source SourceIdentity `json:"source"`
	Tables []Table        `json:"tables"`
}

func (manifest Manifest) Validate() error {
	if err := manifest.Source.Validate(); err != nil {
		return err
	}
	if len(manifest.Tables) == 0 {
		return ErrInvalidManifest
	}
	seen := make(map[string]struct{}, len(manifest.Tables))
	last := ""
	for _, table := range manifest.Tables {
		if err := table.Validate(); err != nil {
			return err
		}
		if table.Name <= last {
			return fmt.Errorf("%w: tables are not ordered", ErrInvalidManifest)
		}
		if _, exists := seen[table.Name]; exists {
			return fmt.Errorf("%w: duplicate table %s", ErrInvalidManifest, table.Name)
		}
		seen[table.Name] = struct{}{}
		last = table.Name
	}
	return nil
}

func (manifest Manifest) Digest() ([sha256.Size]byte, error) {
	if err := manifest.Validate(); err != nil {
		return [sha256.Size]byte{}, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("marshal V1 archive manifest: %w", err)
	}
	return sha256.Sum256(encoded), nil
}

func (manifest Manifest) DigestHex() (string, error) {
	digest, err := manifest.Digest()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest[:]), nil
}

// Snapshot is backed by one read-only repeatable-read V1 transaction.
// Callers can inspect only catalog metadata and canonical row JSON.
type Snapshot interface {
	Identity() SourceIdentity
	Manifest(context.Context) (Manifest, error)
	EachRow(context.Context, Table, func(sourceKeyJSON, payloadJSON []byte) error) error
}

type Source interface {
	WithSnapshot(context.Context, func(Snapshot) error) error
	Close()
}

type Record struct {
	RunID             string
	Table             string
	Ordinal           int64
	SourceKeyHMAC     [sha256.Size]byte
	PayloadHMAC       [sha256.Size]byte
	FieldHMAC         [sha256.Size]byte
	SchemaDigest      [sha256.Size]byte
	ArchiveKeyVersion int
	RedactedPaths     []string
	Nonce             []byte
	Ciphertext        []byte
}

type Run struct {
	ID                string
	AdapterID         string
	Source            SourceIdentity
	SourceDumpDigest  [sha256.Size]byte
	RepositorySHA     string
	SnapshotDigest    [sha256.Size]byte
	SchemaDigest      [sha256.Size]byte
	PolicyDigest      [sha256.Size]byte
	ArchiveKeyVersion int
}

func (run Run) Validate() error {
	if strings.TrimSpace(run.ID) != run.ID || run.ID == "" || len(run.ID) > 128 || strings.TrimSpace(run.AdapterID) != run.AdapterID || run.AdapterID == "" || len(run.RepositorySHA) != 40 || !isLowerHex(run.RepositorySHA) || run.SourceDumpDigest == ([sha256.Size]byte{}) || run.SnapshotDigest == ([sha256.Size]byte{}) || run.SchemaDigest == ([sha256.Size]byte{}) || run.PolicyDigest == ([sha256.Size]byte{}) || run.ArchiveKeyVersion < 1 {
		return ErrInvalidConfig
	}
	if err := run.Source.Validate(); err != nil {
		return err
	}
	return nil
}

func isLowerHex(value string) bool {
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

type TableSummary struct {
	Table  string
	Count  int64
	Digest [sha256.Size]byte
}

type Summary struct {
	RunID  string
	Tables []TableSummary
}

func (summary Summary) TotalCount() int64 {
	var total int64
	for _, table := range summary.Tables {
		total += table.Count
	}
	return total
}

func (summary Summary) Validate() error {
	if strings.TrimSpace(summary.RunID) != summary.RunID || summary.RunID == "" {
		return ErrInvalidConfig
	}
	last := ""
	for _, table := range summary.Tables {
		if strings.TrimSpace(table.Table) != table.Table || table.Table == "" || table.Table <= last || table.Count < 0 {
			return ErrInvalidConfig
		}
		last = table.Table
	}
	return nil
}

type TargetWriter interface {
	EnsureRun(context.Context, Run, Manifest) error
	WriteBatch(context.Context, []Record) error
	Complete(context.Context, Summary) error
	Run(context.Context, string) (Run, bool, error)
	Summary(context.Context, string) (Summary, error)
	Reconcile(context.Context, Summary) error
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
