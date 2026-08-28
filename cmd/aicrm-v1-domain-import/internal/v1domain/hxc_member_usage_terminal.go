package v1domain

import (
	"context"
	"crypto/sha256"
	"strings"

	"github.com/jackc/pgx/v5"
	memberusage "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxcmemberusagehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type hxcMemberUsageTerminalDB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type hxcMemberUsageTerminalBatchSource interface {
	archiveRecords(context.Context, string, [][]byte) ([]hxcMemberUsageArchiveRecord, error)
	archiveReceipts(context.Context, string, [][]byte) ([]hxcMemberUsageArchiveReceipt, error)
}

// HXCMemberUsageTerminalReader verifies a bounded source batch against its
// immutable record and original generic archive receipt. It has no write path.
type HXCMemberUsageTerminalReader struct {
	source hxcMemberUsageTerminalBatchSource
}

var _ memberusage.TerminalReader = (*HXCMemberUsageTerminalReader)(nil)

// NewHXCMemberUsageTerminalReader accepts either a caller transaction or a
// read-only query-capable DBTX. The caller owns transaction lifetime.
func NewHXCMemberUsageTerminalReader(db hxcMemberUsageTerminalDB) *HXCMemberUsageTerminalReader {
	return &HXCMemberUsageTerminalReader{source: hxcMemberUsageTerminalSQL{db: db}}
}

func (reader *HXCMemberUsageTerminalReader) VerifyArchiveTerminals(ctx context.Context, run string, values []memberusage.SourceEnvelope) error {
	if reader == nil || reader.source == nil || ctx == nil || ctx.Err() != nil || run == "" || strings.TrimSpace(run) != run || len(values) == 0 || len(values) > memberusage.StreamBatchSize {
		return ErrInvalidScope
	}
	keys := make([][]byte, 0, len(values))
	expected := make(map[[sha256.Size]byte]memberusage.SourceEnvelope, len(values))
	ordinals := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value.SourceOrdinal < 1 || value.SourceKeyHMAC == ([sha256.Size]byte{}) || value.PayloadHMAC == ([sha256.Size]byte{}) || value.FieldHMAC == ([sha256.Size]byte{}) {
			return ErrConflict
		}
		if _, found := expected[value.SourceKeyHMAC]; found {
			return ErrConflict
		}
		if _, found := ordinals[value.SourceOrdinal]; found {
			return ErrConflict
		}
		expected[value.SourceKeyHMAC] = value
		ordinals[value.SourceOrdinal] = struct{}{}
		key := make([]byte, sha256.Size)
		copy(key, value.SourceKeyHMAC[:])
		keys = append(keys, key)
	}
	records, err := reader.source.archiveRecords(ctx, run, keys)
	if err != nil {
		return ErrConflict
	}
	receipts, err := reader.source.archiveReceipts(ctx, run, keys)
	if err != nil {
		return ErrConflict
	}
	return verifyHXCMemberUsageTerminalBatch(run, expected, records, receipts)
}

type hxcMemberUsageArchiveRecord struct {
	RunID, AdapterID, TableID string
	SourceOrdinal             int64
	SourceKeyDigest           [sha256.Size]byte
	PayloadDigest             [sha256.Size]byte
	FieldDigest               [sha256.Size]byte
	SchemaDigest              [sha256.Size]byte
}

type hxcMemberUsageArchiveReceipt struct {
	RunID, AdapterID, TableID, Disposition, Operation string
	SourceKeyDigest                                   [sha256.Size]byte
	PayloadDigest                                     [sha256.Size]byte
	FieldDigest                                       [sha256.Size]byte
	MappingDigest                                     [sha256.Size]byte
	PolicyDigest                                      [sha256.Size]byte
	MutationDigest                                    [sha256.Size]byte
}

func verifyHXCMemberUsageTerminalBatch(run string, expected map[[sha256.Size]byte]memberusage.SourceEnvelope, records []hxcMemberUsageArchiveRecord, receipts []hxcMemberUsageArchiveReceipt) error {
	if run == "" || len(expected) == 0 || len(expected) > memberusage.StreamBatchSize || len(records) != len(expected) || len(receipts) != len(expected) {
		return ErrConflict
	}
	schemas := make(map[[sha256.Size]byte][sha256.Size]byte, len(records))
	for _, record := range records {
		value, found := expected[record.SourceKeyDigest]
		if !found || record.RunID != run || record.AdapterID != v1archive.DefaultAdapterID || record.TableID != memberusage.MemberUsageProjectionTableID || record.SourceOrdinal != value.SourceOrdinal || record.PayloadDigest != value.PayloadHMAC || record.FieldDigest != value.FieldHMAC || record.SchemaDigest == ([sha256.Size]byte{}) {
			return ErrConflict
		}
		if _, duplicate := schemas[record.SourceKeyDigest]; duplicate {
			return ErrConflict
		}
		schemas[record.SourceKeyDigest] = record.SchemaDigest
	}
	if len(schemas) != len(expected) {
		return ErrConflict
	}
	seen := make(map[[sha256.Size]byte]struct{}, len(receipts))
	for _, receipt := range receipts {
		value, found := expected[receipt.SourceKeyDigest]
		schema, archived := schemas[receipt.SourceKeyDigest]
		if !found || !archived || receipt.RunID != run || receipt.AdapterID != v1archive.DefaultAdapterID || receipt.TableID != memberusage.MemberUsageProjectionTableID || receipt.PayloadDigest != value.PayloadHMAC || receipt.FieldDigest != value.FieldHMAC || receipt.Disposition != "archive" || receipt.Operation != "" || receipt.MappingDigest != hxcMemberUsageArchiveMappingDigest(schema) || receipt.PolicyDigest != v1archive.ArchivePolicyDigest() || receipt.MutationDigest != hxcMemberUsageArchiveMutationDigest(value, schema) {
			return ErrConflict
		}
		if _, duplicate := seen[receipt.SourceKeyDigest]; duplicate {
			return ErrConflict
		}
		seen[receipt.SourceKeyDigest] = struct{}{}
	}
	if len(seen) != len(expected) {
		return ErrConflict
	}
	return nil
}

func (source hxcMemberUsageTerminalSQL) archiveRecords(ctx context.Context, run string, keys [][]byte) ([]hxcMemberUsageArchiveRecord, error) {
	if source.db == nil {
		return nil, ErrInvalidScope
	}
	rows, err := source.db.Query(ctx, `SELECT run_id,adapter_id,table_id,source_ordinal,source_key_digest,payload_digest,field_digest,schema_digest
FROM public.v1_archive_records
WHERE run_id=$1 AND adapter_id=$2 AND table_id=$3 AND source_key_digest=ANY($4::bytea[])`, run, v1archive.DefaultAdapterID, memberusage.MemberUsageProjectionTableID, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]hxcMemberUsageArchiveRecord, 0, len(keys))
	for rows.Next() {
		var value hxcMemberUsageArchiveRecord
		var sourceKey, payload, field, schema []byte
		if err = rows.Scan(&value.RunID, &value.AdapterID, &value.TableID, &value.SourceOrdinal, &sourceKey, &payload, &field, &schema); err != nil || len(sourceKey) != sha256.Size || len(payload) != sha256.Size || len(field) != sha256.Size || len(schema) != sha256.Size {
			return nil, ErrConflict
		}
		copy(value.SourceKeyDigest[:], sourceKey)
		copy(value.PayloadDigest[:], payload)
		copy(value.FieldDigest[:], field)
		copy(value.SchemaDigest[:], schema)
		result = append(result, value)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (source hxcMemberUsageTerminalSQL) archiveReceipts(ctx context.Context, run string, keys [][]byte) ([]hxcMemberUsageArchiveReceipt, error) {
	if source.db == nil {
		return nil, ErrInvalidScope
	}
	rows, err := source.db.Query(ctx, `SELECT run_id,adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,operation,mapping_digest,policy_digest,mutation_digest
FROM public.data_migration_row_receipts
WHERE run_id=$1 AND adapter_id=$2 AND table_id=$3 AND source_key_digest=ANY($4::bytea[])`, run, v1archive.DefaultAdapterID, memberusage.MemberUsageProjectionTableID, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]hxcMemberUsageArchiveReceipt, 0, len(keys))
	for rows.Next() {
		var value hxcMemberUsageArchiveReceipt
		var sourceKey, payload, field, mapping, policy, mutation []byte
		if err = rows.Scan(&value.RunID, &value.AdapterID, &value.TableID, &sourceKey, &payload, &field, &value.Disposition, &value.Operation, &mapping, &policy, &mutation); err != nil || len(sourceKey) != sha256.Size || len(payload) != sha256.Size || len(field) != sha256.Size || len(mapping) != sha256.Size || len(policy) != sha256.Size || len(mutation) != sha256.Size {
			return nil, ErrConflict
		}
		copy(value.SourceKeyDigest[:], sourceKey)
		copy(value.PayloadDigest[:], payload)
		copy(value.FieldDigest[:], field)
		copy(value.MappingDigest[:], mapping)
		copy(value.PolicyDigest[:], policy)
		copy(value.MutationDigest[:], mutation)
		result = append(result, value)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type hxcMemberUsageTerminalSQL struct{ db hxcMemberUsageTerminalDB }

func hxcMemberUsageArchiveMappingDigest(schema [sha256.Size]byte) [sha256.Size]byte {
	return sha256.Sum256(append([]byte("aicrm/v1archive/mapping/v1\x00"+strings.TrimPrefix(memberusage.MemberUsageProjectionTableID, "public/")+"\x00"), schema[:]...))
}

func hxcMemberUsageArchiveMutationDigest(value memberusage.SourceEnvelope, schema [sha256.Size]byte) [sha256.Size]byte {
	buffer := make([]byte, 0, len("aicrm/v1archive/mutation/v1\x00")+sha256.Size*4)
	buffer = append(buffer, []byte("aicrm/v1archive/mutation/v1\x00")...)
	buffer = append(buffer, value.SourceKeyHMAC[:]...)
	buffer = append(buffer, value.PayloadHMAC[:]...)
	buffer = append(buffer, value.FieldHMAC[:]...)
	buffer = append(buffer, schema[:]...)
	return sha256.Sum256(buffer)
}
