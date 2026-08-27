package v1archive

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSource obtains every source fact through a single repeatable-read,
// read-only transaction. It deliberately has no method that can execute a
// caller-provided mutation.
type PostgresSource struct {
	pool *pgxpool.Pool
}

func OpenPostgresSource(ctx context.Context, dsn string) (*PostgresSource, error) {
	if dsn == "" {
		return nil, ErrInvalidConfig
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse V1 archive source DSN: %w", err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open V1 archive source: %w", err)
	}
	return &PostgresSource{pool: pool}, nil
}

func (source *PostgresSource) Close() {
	if source != nil && source.pool != nil {
		source.pool.Close()
	}
}

func (source *PostgresSource) WithSnapshot(ctx context.Context, callback func(Snapshot) error) error {
	if source == nil || source.pool == nil || callback == nil {
		return ErrInvalidConfig
	}
	tx, err := source.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin V1 read-only snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var readOnly string
	if err = tx.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readOnly); err != nil {
		return fmt.Errorf("verify V1 read-only snapshot: %w", err)
	}
	if err = validateSourceSafety(readOnly == "on", true, true, true); err != nil {
		return err
	}
	identity, err := sourceIdentity(ctx, tx)
	if err != nil {
		return err
	}
	if err = sourceRoleReadOnly(ctx, tx); err != nil {
		return err
	}
	if err = callback(&postgresSnapshot{tx: tx, identity: identity}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type postgresSnapshot struct {
	tx       pgx.Tx
	identity SourceIdentity
}

func (snapshot *postgresSnapshot) Identity() SourceIdentity { return snapshot.identity }

func (snapshot *postgresSnapshot) Manifest(ctx context.Context) (Manifest, error) {
	if snapshot == nil || snapshot.tx == nil {
		return Manifest{}, ErrInvalidManifest
	}
	tables, err := listPublicTables(ctx, snapshot.tx)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{Source: snapshot.identity, Tables: tables}
	if err = manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (snapshot *postgresSnapshot) EachRow(ctx context.Context, table Table, callback func(sourceKeyJSON, payloadJSON []byte) error) error {
	if snapshot == nil || snapshot.tx == nil || callback == nil {
		return ErrInvalidConfig
	}
	if err := table.Validate(); err != nil {
		return err
	}
	query := tableRowsSQL(table)
	rows, err := snapshot.tx.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("stream V1 public.%s: %w", table.Name, err)
	}
	defer rows.Close()
	for rows.Next() {
		var sourceKey, payload string
		if err = rows.Scan(&sourceKey, &payload); err != nil {
			return fmt.Errorf("scan V1 public.%s: %w", table.Name, err)
		}
		if err = callback([]byte(sourceKey), []byte(payload)); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate V1 public.%s: %w", table.Name, err)
	}
	return nil
}

func sourceIdentity(ctx context.Context, tx pgx.Tx) (SourceIdentity, error) {
	var identity SourceIdentity
	err := tx.QueryRow(ctx, "SELECT system_identifier::text, current_database(), current_user FROM pg_control_system()").Scan(&identity.SystemID, &identity.Database, &identity.Role)
	if err != nil {
		return SourceIdentity{}, fmt.Errorf("identify V1 source: %w", err)
	}
	if err = identity.Validate(); err != nil {
		return SourceIdentity{}, err
	}
	return identity, nil
}

// sourceRoleReadOnly rejects a source role that has effective DDL or DML
// privileges over any V1 public base table. The separate transaction check in
// WithSnapshot prevents writes even if a role changes after this check.
func sourceRoleReadOnly(ctx context.Context, tx pgx.Tx) error {
	const query = `
SELECT NOT EXISTS (
  SELECT 1
  FROM pg_catalog.pg_class AS class
  JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = class.relnamespace
  WHERE namespace.nspname = 'public'
    AND class.relkind IN ('r', 'p')
    AND (
      has_table_privilege(current_user, class.oid, 'INSERT')
      OR has_table_privilege(current_user, class.oid, 'UPDATE')
      OR has_table_privilege(current_user, class.oid, 'DELETE')
      OR has_table_privilege(current_user, class.oid, 'TRUNCATE')
    )
),
NOT has_schema_privilege(current_user, 'public', 'CREATE'),
NOT EXISTS (
  SELECT 1 FROM pg_catalog.pg_roles
  WHERE rolname = current_user AND (rolsuper OR rolbypassrls)
)`
	var noWriteTables, noSchemaCreate, safeRole bool
	if err := tx.QueryRow(ctx, query).Scan(&noWriteTables, &noSchemaCreate, &safeRole); err != nil {
		return fmt.Errorf("verify V1 source privileges: %w", err)
	}
	return validateSourceSafety(true, noWriteTables, noSchemaCreate, safeRole)
}

func validateSourceSafety(transactionReadOnly, noWriteTables, noSchemaCreate, safeRole bool) error {
	if !transactionReadOnly || !noWriteTables || !noSchemaCreate || !safeRole {
		return ErrSourceNotReadOnly
	}
	return nil
}

func listPublicTables(ctx context.Context, tx pgx.Tx) ([]Table, error) {
	if err := ensureNoPublicPartitions(ctx, tx); err != nil {
		return nil, err
	}
	const tablesQuery = `
SELECT class.oid::bigint, class.relname
FROM pg_catalog.pg_class AS class
JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = class.relnamespace
WHERE namespace.nspname = 'public'
  AND class.relkind IN ('r', 'p')
ORDER BY class.relname`
	rows, err := tx.Query(ctx, tablesQuery)
	if err != nil {
		return nil, fmt.Errorf("list V1 public base tables: %w", err)
	}
	defer rows.Close()
	type tableRef struct {
		oid  int64
		name string
	}
	references := make([]tableRef, 0)
	for rows.Next() {
		var oid int64
		var name string
		if err = rows.Scan(&oid, &name); err != nil {
			return nil, err
		}
		references = append(references, tableRef{oid: oid, name: name})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	tables := make([]Table, 0, len(references))
	for _, reference := range references {
		oid, name := reference.oid, reference.name
		columns, err := listColumns(ctx, tx, oid)
		if err != nil {
			return nil, fmt.Errorf("list V1 public.%s columns: %w", name, err)
		}
		primaryKey, err := listPrimaryKey(ctx, tx, oid)
		if err != nil {
			return nil, fmt.Errorf("list V1 public.%s primary key: %w", name, err)
		}
		rowCount, err := tableRowCount(ctx, tx, name)
		if err != nil {
			return nil, fmt.Errorf("count V1 public.%s rows: %w", name, err)
		}
		table := Table{Name: name, Columns: columns, PrimaryKey: primaryKey, RowCount: rowCount}
		if err = table.Validate(); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	sort.Slice(tables, func(left, right int) bool { return tables[left].Name < tables[right].Name })
	return tables, nil
}

func ensureNoPublicPartitions(ctx context.Context, tx pgx.Tx) error {
	const query = `
SELECT EXISTS (
  SELECT 1
  FROM pg_catalog.pg_inherits AS inheritance
  JOIN pg_catalog.pg_class AS parent ON parent.oid = inheritance.inhparent
  JOIN pg_catalog.pg_namespace AS parent_namespace ON parent_namespace.oid = parent.relnamespace
  JOIN pg_catalog.pg_class AS child ON child.oid = inheritance.inhrelid
  JOIN pg_catalog.pg_namespace AS child_namespace ON child_namespace.oid = child.relnamespace
  WHERE parent_namespace.nspname = 'public' OR child_namespace.nspname = 'public'
)`
	var found bool
	if err := tx.QueryRow(ctx, query).Scan(&found); err != nil {
		return fmt.Errorf("inspect V1 public partitions: %w", err)
	}
	if found {
		return fmt.Errorf("%w: partitioned V1 tables require an explicit root-or-leaf policy", ErrInvalidManifest)
	}
	return nil
}

func tableRowCount(ctx context.Context, tx pgx.Tx, table string) (int64, error) {
	query := "SELECT count(*) FROM " + pgx.Identifier{"public", table}.Sanitize()
	var count int64
	if err := tx.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func listColumns(ctx context.Context, tx pgx.Tx, tableOID int64) ([]Column, error) {
	const query = `
SELECT attribute.attnum::integer, attribute.attname, pg_catalog.format_type(attribute.atttypid, attribute.atttypmod), attribute.attnotnull
FROM pg_catalog.pg_attribute AS attribute
WHERE attribute.attrelid = $1
  AND attribute.attnum > 0
  AND NOT attribute.attisdropped
ORDER BY attribute.attnum`
	rows, err := tx.Query(ctx, query, tableOID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make([]Column, 0)
	for rows.Next() {
		var column Column
		if err = rows.Scan(&column.Ordinal, &column.Name, &column.Type, &column.NotNull); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}

func listPrimaryKey(ctx context.Context, tx pgx.Tx, tableOID int64) ([]string, error) {
	const query = `
SELECT attribute.attname
FROM pg_catalog.pg_index AS index
JOIN LATERAL unnest(index.indkey) WITH ORDINALITY AS key(attnum, ordinal) ON TRUE
JOIN pg_catalog.pg_attribute AS attribute ON attribute.attrelid = index.indrelid AND attribute.attnum = key.attnum
WHERE index.indrelid = $1
  AND index.indisprimary
ORDER BY key.ordinal`
	rows, err := tx.Query(ctx, query, tableOID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	primaryKey := make([]string, 0)
	for rows.Next() {
		var column string
		if err = rows.Scan(&column); err != nil {
			return nil, err
		}
		primaryKey = append(primaryKey, column)
	}
	return primaryKey, rows.Err()
}

func tableRowsSQL(table Table) string {
	qualified := pgx.Identifier{"public", table.Name}.Sanitize()
	if len(table.PrimaryKey) == 0 {
		// Five frozen V1 tables have no primary key. Payload plus duplicate ordinal
		// is stable across restored snapshots; ctid only orders indistinguishable
		// duplicates and is not part of the generated key or archived payload.
		payload := "to_jsonb(source_row)"
		return "SELECT jsonb_build_array('__aicrm_keyless__', payload, duplicate_ordinal)::text, payload::text FROM (SELECT " + payload + " AS payload, row_number() OVER (PARTITION BY " + payload + "::text ORDER BY source_row.ctid) AS duplicate_ordinal FROM " + qualified + " AS source_row) AS keyless_rows ORDER BY payload::text, duplicate_ordinal"
	}
	columns := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		columns = append(columns, pgx.Identifier{column.Name}.Sanitize())
	}
	primaryKey := make([]string, 0, len(table.PrimaryKey))
	for _, column := range table.PrimaryKey {
		primaryKey = append(primaryKey, "row."+pgx.Identifier{column}.Sanitize())
	}
	return "SELECT jsonb_build_array(" + stringsJoin(primaryKey, ", ") + ")::text, to_jsonb(row)::text FROM (SELECT " + stringsJoin(columns, ", ") + " FROM " + qualified + ") AS row ORDER BY " + stringsJoin(primaryKey, ", ")
}

func stringsJoin(values []string, separator string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += separator + value
	}
	return result
}

var _ Source = (*PostgresSource)(nil)
var _ Snapshot = (*postgresSnapshot)(nil)
