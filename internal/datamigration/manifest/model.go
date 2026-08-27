// Package manifest builds and reconciles metadata-only PostgreSQL manifests.
// It never selects business rows.
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const Version = 1

// Manifest is a content-safe schema and aggregate-count inventory.
type Manifest struct {
	Version     int       `json:"version"`
	GeneratedAt time.Time `json:"generated_at"`
	Schemas     []string  `json:"schemas"`
	Tables      []Table   `json:"tables"`
	Digest      string    `json:"digest"`
}

type TableKey struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
}

func (key TableKey) String() string { return key.Schema + "." + key.Table }

type Table struct {
	TableKey
	Columns      []Column     `json:"columns"`
	PrimaryKey   []string     `json:"primary_key"`
	ForeignKeys  []ForeignKey `json:"foreign_keys"`
	RowCount     int64        `json:"row_count"`
	Watermark    Watermark    `json:"watermark"`
	SchemaDigest string       `json:"schema_digest"`
}

type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Ordinal  int    `json:"ordinal"`
}

type ForeignKey struct {
	Columns          []string `json:"columns"`
	Reference        TableKey `json:"reference"`
	ReferenceColumns []string `json:"reference_columns"`
	OnUpdate         string   `json:"on_update"`
	OnDelete         string   `json:"on_delete"`
}

// Watermark is an aggregate maximum of a timestamp/date column. It is empty
// when a table has no recognised timestamp/date column; no row value is read.
type Watermark struct {
	Column string `json:"column,omitempty"`
	Value  string `json:"value,omitempty"`
}

func (watermark Watermark) Empty() bool { return watermark.Column == "" }

func (manifest *Manifest) Normalize() error {
	if manifest.Version == 0 {
		manifest.Version = Version
	}
	if manifest.Version != Version {
		return fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	for index := range manifest.Schemas {
		manifest.Schemas[index] = strings.TrimSpace(manifest.Schemas[index])
		if manifest.Schemas[index] == "" {
			return fmt.Errorf("schemas[%d] is empty", index)
		}
	}
	sort.Strings(manifest.Schemas)
	for index := range manifest.Tables {
		if err := manifest.Tables[index].Normalize(); err != nil {
			return err
		}
	}
	sort.Slice(manifest.Tables, func(left, right int) bool {
		return manifest.Tables[left].TableKey.String() < manifest.Tables[right].TableKey.String()
	})
	seen := make(map[string]struct{}, len(manifest.Tables))
	for _, table := range manifest.Tables {
		key := table.TableKey.String()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate table %q", key)
		}
		seen[key] = struct{}{}
	}
	digest, err := digestJSON(struct {
		Version int      `json:"version"`
		Schemas []string `json:"schemas"`
		Tables  []Table  `json:"tables"`
	}{Version: manifest.Version, Schemas: manifest.Schemas, Tables: manifest.Tables})
	if err != nil {
		return err
	}
	manifest.Digest = digest
	return nil
}

func (table *Table) Normalize() error {
	table.Schema = strings.TrimSpace(table.Schema)
	table.Table = strings.TrimSpace(table.Table)
	if table.Schema == "" || table.Table == "" {
		return fmt.Errorf("table key is incomplete")
	}
	if table.RowCount < 0 {
		return fmt.Errorf("table %q has negative row count", table.TableKey.String())
	}
	sort.Slice(table.Columns, func(left, right int) bool { return table.Columns[left].Ordinal < table.Columns[right].Ordinal })
	for index, column := range table.Columns {
		if strings.TrimSpace(column.Name) == "" || strings.TrimSpace(column.Type) == "" || column.Ordinal <= 0 {
			return fmt.Errorf("table %q has invalid column at index %d", table.TableKey.String(), index)
		}
	}
	sort.Strings(table.PrimaryKey)
	sort.Slice(table.ForeignKeys, func(left, right int) bool {
		return foreignKeyKey(table.ForeignKeys[left]) < foreignKeyKey(table.ForeignKeys[right])
	})
	digest, err := digestJSON(struct {
		TableKey
		Columns     []Column     `json:"columns"`
		PrimaryKey  []string     `json:"primary_key"`
		ForeignKeys []ForeignKey `json:"foreign_keys"`
	}{TableKey: table.TableKey, Columns: table.Columns, PrimaryKey: table.PrimaryKey, ForeignKeys: table.ForeignKeys})
	if err != nil {
		return err
	}
	table.SchemaDigest = digest
	return nil
}

func foreignKeyKey(key ForeignKey) string {
	return strings.Join(key.Columns, ",") + "->" + key.Reference.String() + "(" + strings.Join(key.ReferenceColumns, ",") + ")"
}

func digestJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (manifest Manifest) TableMap() map[string]Table {
	tables := make(map[string]Table, len(manifest.Tables))
	for _, table := range manifest.Tables {
		tables[table.TableKey.String()] = table
	}
	return tables
}
