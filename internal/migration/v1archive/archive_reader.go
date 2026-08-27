package v1archive

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArchivedRow is authenticated V1 source material read from the immutable
// 00107 archive. Payload contains the already-redacted canonical JSON.
type ArchivedRow struct {
	AdapterID      string
	TableID        string
	SourceOrdinal  int64
	SourceKeyHMAC  [sha256.Size]byte
	PayloadHMAC    [sha256.Size]byte
	FieldHMAC      [sha256.Size]byte
	Payload        []byte
	RedactedFields []string
}

type PostgresArchiveReader struct {
	pool       *pgxpool.Pool
	archiveKey []byte
}

func OpenPostgresArchiveReader(ctx context.Context, dsn string, archiveKey []byte) (*PostgresArchiveReader, error) {
	if dsn == "" || len(archiveKey) != 32 {
		return nil, ErrInvalidConfig
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse V1 archive target DSN: %w", err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open V1 archive target: %w", err)
	}
	return &PostgresArchiveReader{pool: pool, archiveKey: append([]byte(nil), archiveKey...)}, nil
}

func (reader *PostgresArchiveReader) Close() {
	if reader != nil && reader.pool != nil {
		reader.pool.Close()
	}
}

// EachTableRow streams one archived table inside a repeatable-read, read-only
// transaction. It cannot change archive or target state.
func (reader *PostgresArchiveReader) EachTableRow(ctx context.Context, runID, tableID string, callback func(ArchivedRow) error) error {
	if reader == nil || reader.pool == nil || len(reader.archiveKey) != 32 || runID == "" || tableID == "" || callback == nil {
		return ErrInvalidConfig
	}
	tx, err := reader.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var phase string
	if err = tx.QueryRow(ctx, `SELECT migration.phase FROM public.v1_archive_runs AS archive
JOIN public.data_migration_runs AS migration ON migration.run_id=archive.run_id
WHERE archive.run_id=$1`, runID).Scan(&phase); err != nil {
		return err
	}
	if phase != "reconciled" {
		return fmt.Errorf("V1 archive run is not reconciled")
	}
	rows, err := tx.Query(ctx, `SELECT adapter_id,table_id,source_ordinal,source_key_digest,payload_digest,
field_digest,schema_digest,nonce,ciphertext,key_version,compression,redaction_metadata
FROM public.v1_archive_records
WHERE run_id=$1 AND table_id=$2 ORDER BY source_ordinal`, runID, tableID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value ArchivedRow
		var sourceKey, payload, fields, schema []byte
		var nonce, ciphertext []byte
		var redactionMetadata []byte
		var keyVersion int
		var compression string
		if err = rows.Scan(&value.AdapterID, &value.TableID, &value.SourceOrdinal, &sourceKey, &payload,
			&fields, &schema, &nonce, &ciphertext, &keyVersion, &compression, &redactionMetadata); err != nil {
			return err
		}
		if len(sourceKey) != sha256.Size || len(payload) != sha256.Size || len(fields) != sha256.Size || len(schema) != sha256.Size || compression != "none" {
			return ErrArchiveTampered
		}
		if err = json.Unmarshal(redactionMetadata, &value.RedactedFields); err != nil {
			return ErrArchiveTampered
		}
		copy(value.SourceKeyHMAC[:], sourceKey)
		copy(value.PayloadHMAC[:], payload)
		copy(value.FieldHMAC[:], fields)
		var schemaDigest [sha256.Size]byte
		copy(schemaDigest[:], schema)
		value.Payload, err = DecryptRecord(reader.archiveKey, Record{
			RunID: runID, Table: tableName(tableID), Ordinal: value.SourceOrdinal,
			SourceKeyHMAC: value.SourceKeyHMAC, PayloadHMAC: value.PayloadHMAC,
			FieldHMAC: value.FieldHMAC, SchemaDigest: schemaDigest,
			ArchiveKeyVersion: keyVersion, Nonce: nonce, Ciphertext: ciphertext,
		})
		if err != nil {
			return fmt.Errorf("decrypt V1 archive %s row %d: %w", tableID, value.SourceOrdinal, err)
		}
		if err = callback(value); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func tableName(tableID string) string {
	for index := range tableID {
		if tableID[index] == '/' && index+1 < len(tableID) {
			return tableID[index+1:]
		}
	}
	return tableID
}

func IsRedacted(row ArchivedRow, field string) bool {
	for _, redacted := range row.RedactedFields {
		if redacted == field {
			return true
		}
	}
	return false
}
