package manifest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Collect opens a repeatable-read, read-only PostgreSQL transaction and only
// reads catalogs plus count/max aggregates. It does not read row payloads.
func Collect(ctx context.Context, databaseURL string, schemas []string) (Manifest, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return Manifest{}, fmt.Errorf("database URL is required")
	}
	if len(schemas) == 0 {
		schemas = []string{"public"}
	}
	for index := range schemas {
		schemas[index] = strings.TrimSpace(schemas[index])
		if schemas[index] == "" {
			return Manifest{}, fmt.Errorf("schema at index %d is empty", index)
		}
	}
	sort.Strings(schemas)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return Manifest{}, fmt.Errorf("connect manifest source: %w", err)
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Manifest{}, fmt.Errorf("begin read-only manifest transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tables, err := listTables(ctx, tx, schemas)
	if err != nil {
		return Manifest{}, err
	}
	for index := range tables {
		if err := enrichTable(ctx, tx, &tables[index]); err != nil {
			return Manifest{}, err
		}
	}
	manifest := Manifest{Version: Version, GeneratedAt: time.Now().UTC(), Schemas: schemas, Tables: tables}
	if err := manifest.Normalize(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func listTables(ctx context.Context, tx pgx.Tx, schemas []string) ([]Table, error) {
	rows, err := tx.Query(ctx, `
SELECT namespace.nspname, relation.relname
FROM pg_catalog.pg_class relation
JOIN pg_catalog.pg_namespace namespace ON namespace.oid = relation.relnamespace
WHERE relation.relkind IN ('r', 'p')
  AND namespace.nspname = ANY($1)
ORDER BY namespace.nspname, relation.relname`, schemas)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	var tables []Table
	for rows.Next() {
		var table Table
		if err := rows.Scan(&table.Schema, &table.Table); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables: %w", err)
	}
	return tables, nil
}

func enrichTable(ctx context.Context, tx pgx.Tx, table *Table) error {
	columns, err := listColumns(ctx, tx, table.TableKey)
	if err != nil {
		return err
	}
	table.Columns = columns
	if table.PrimaryKey, err = listPrimaryKey(ctx, tx, table.TableKey); err != nil {
		return err
	}
	if table.ForeignKeys, err = listForeignKeys(ctx, tx, table.TableKey); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+quotedTable(table.TableKey)).Scan(&table.RowCount); err != nil {
		return fmt.Errorf("count %s: %w", table.TableKey.String(), err)
	}
	table.Watermark, err = collectWatermark(ctx, tx, table.TableKey, columns)
	if err != nil {
		return err
	}
	return nil
}

func listColumns(ctx context.Context, tx pgx.Tx, key TableKey) ([]Column, error) {
	rows, err := tx.Query(ctx, `
SELECT attribute.attname, pg_catalog.format_type(attribute.atttypid, attribute.atttypmod),
       NOT attribute.attnotnull, attribute.attnum
FROM pg_catalog.pg_attribute attribute
JOIN pg_catalog.pg_class relation ON relation.oid = attribute.attrelid
JOIN pg_catalog.pg_namespace namespace ON namespace.oid = relation.relnamespace
WHERE namespace.nspname = $1 AND relation.relname = $2
  AND attribute.attnum > 0 AND NOT attribute.attisdropped
ORDER BY attribute.attnum`, key.Schema, key.Table)
	if err != nil {
		return nil, fmt.Errorf("list columns %s: %w", key.String(), err)
	}
	defer rows.Close()
	var columns []Column
	for rows.Next() {
		var column Column
		if err := rows.Scan(&column.Name, &column.Type, &column.Nullable, &column.Ordinal); err != nil {
			return nil, fmt.Errorf("scan column %s: %w", key.String(), err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns %s: %w", key.String(), err)
	}
	return columns, nil
}

func listPrimaryKey(ctx context.Context, tx pgx.Tx, key TableKey) ([]string, error) {
	var columns []string
	err := tx.QueryRow(ctx, `
SELECT COALESCE(array_agg(attribute.attname ORDER BY key_column.ordinality), ARRAY[]::text[])
FROM pg_catalog.pg_index index
JOIN pg_catalog.pg_class relation ON relation.oid = index.indrelid
JOIN pg_catalog.pg_namespace namespace ON namespace.oid = relation.relnamespace
JOIN LATERAL unnest(index.indkey) WITH ORDINALITY AS key_column(attnum, ordinality) ON TRUE
JOIN pg_catalog.pg_attribute attribute ON attribute.attrelid = relation.oid AND attribute.attnum = key_column.attnum
WHERE namespace.nspname = $1 AND relation.relname = $2 AND index.indisprimary`, key.Schema, key.Table).Scan(&columns)
	if err != nil {
		return nil, fmt.Errorf("primary key %s: %w", key.String(), err)
	}
	return columns, nil
}

func listForeignKeys(ctx context.Context, tx pgx.Tx, key TableKey) ([]ForeignKey, error) {
	rows, err := tx.Query(ctx, `
SELECT
  ARRAY(SELECT local_attribute.attname
        FROM unnest(foreign_key.conkey) WITH ORDINALITY AS local_key(attnum, ordinality)
        JOIN pg_catalog.pg_attribute local_attribute ON local_attribute.attrelid = foreign_key.conrelid AND local_attribute.attnum = local_key.attnum
        ORDER BY local_key.ordinality),
  referenced_namespace.nspname, referenced_relation.relname,
  ARRAY(SELECT referenced_attribute.attname
        FROM unnest(foreign_key.confkey) WITH ORDINALITY AS referenced_key(attnum, ordinality)
        JOIN pg_catalog.pg_attribute referenced_attribute ON referenced_attribute.attrelid = foreign_key.confrelid AND referenced_attribute.attnum = referenced_key.attnum
        ORDER BY referenced_key.ordinality),
  foreign_key.confupdtype::text, foreign_key.confdeltype::text
FROM pg_catalog.pg_constraint AS foreign_key
JOIN pg_catalog.pg_class relation ON relation.oid = foreign_key.conrelid
JOIN pg_catalog.pg_namespace namespace ON namespace.oid = relation.relnamespace
JOIN pg_catalog.pg_class referenced_relation ON referenced_relation.oid = foreign_key.confrelid
JOIN pg_catalog.pg_namespace referenced_namespace ON referenced_namespace.oid = referenced_relation.relnamespace
WHERE foreign_key.contype = 'f' AND namespace.nspname = $1 AND relation.relname = $2
ORDER BY referenced_namespace.nspname, referenced_relation.relname, foreign_key.conname`, key.Schema, key.Table)
	if err != nil {
		return nil, fmt.Errorf("foreign keys %s: %w", key.String(), err)
	}
	defer rows.Close()
	var foreignKeys []ForeignKey
	for rows.Next() {
		var foreignKey ForeignKey
		if err := rows.Scan(&foreignKey.Columns, &foreignKey.Reference.Schema, &foreignKey.Reference.Table, &foreignKey.ReferenceColumns, &foreignKey.OnUpdate, &foreignKey.OnDelete); err != nil {
			return nil, fmt.Errorf("scan foreign key %s: %w", key.String(), err)
		}
		foreignKeys = append(foreignKeys, foreignKey)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign keys %s: %w", key.String(), err)
	}
	return foreignKeys, nil
}

var watermarkColumns = []string{"updated_at", "modified_at", "created_at", "occurred_at", "event_at", "recorded_at", "timestamp"}

func collectWatermark(ctx context.Context, tx pgx.Tx, key TableKey, columns []Column) (Watermark, error) {
	available := make(map[string]string, len(columns))
	for _, column := range columns {
		available[column.Name] = strings.ToLower(column.Type)
	}
	for _, candidate := range watermarkColumns {
		dataType, found := available[candidate]
		if !found || !(strings.Contains(dataType, "timestamp") || dataType == "date") {
			continue
		}
		var value *string
		query := "SELECT max(" + quoteIdentifier(candidate) + ")::text FROM " + quotedTable(key)
		if err := tx.QueryRow(ctx, query).Scan(&value); err != nil {
			return Watermark{}, fmt.Errorf("watermark %s.%s: %w", key.String(), candidate, err)
		}
		watermark := Watermark{Column: candidate}
		if value != nil {
			watermark.Value = *value
		}
		return watermark, nil
	}
	return Watermark{}, nil
}

func quotedTable(key TableKey) string {
	return quoteIdentifier(key.Schema) + "." + quoteIdentifier(key.Table)
}

func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }
