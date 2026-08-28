package v1hxcruntimehistory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	OldImportVersion          = "v1-hxc-history-a1"
	OldTargetDomain           = "hxc"
	OldRuntimeArchiveTarget   = "hxc_v1_runtime_archive"
	SenderConfigArchiveReason = "hxc_sender_config_archive_only"
	SendRecordArchiveReason   = "hxc_send_record_archive_only"
)

var (
	ErrInvalidSelection = errors.New("hxc runtime history selection invalid")
	ErrSealedDrift      = errors.New("hxc runtime history sealed source drift")
)

// ArchiveSource streams authenticated, redacted V1 archive material for one
// reconciled archive run. It is read-only.
type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

// TerminalScope is the expected old Journal scope. It is not copied from the
// archive/quarantine receipt target fields: those fields are deliberately NULL
// for archive terminals and must be checked by the concrete reader.
type TerminalScope struct {
	ImportVersion string
	ArchiveRunID  string
	AdapterID     string
	TableID       string
	TargetDomain  string
	TargetTable   string
}

// TerminalReceipt is the safely decoded old terminal. Verified means the
// concrete reader has checked the old sealed reconciliation for its scope.
type TerminalReceipt struct {
	SourceKeyDigest [sha256.Size]byte
	PayloadDigest   [sha256.Size]byte
	Disposition     string
	Reason          string
	TargetID        string
	TargetDigest    [sha256.Size]byte
	Metadata        map[string]any
	Verified        bool
}

// TerminalReader must enumerate every terminal in the supplied exact scope.
// Enumeration, rather than a point lookup, lets the selector reject both a
// missing archived row and an extra old terminal before any future write.
type TerminalReader interface {
	EachTerminal(context.Context, TerminalScope, func(TerminalReceipt) error) error
}

type SelectionOptions struct {
	ArchiveRunID  string
	SourceHMACKey []byte
}

type SelectedSenderConfig struct {
	SourceKeyDigest     OpaqueDigest `json:"-"`
	SourcePayloadDigest OpaqueDigest `json:"-"`
	SourceFieldDigest   OpaqueDigest `json:"-"`
	SourceOrdinal       int64        `json:"-"`
	Fact                SenderConfigFact
}

type SelectedSendRecord struct {
	SourceKeyDigest     OpaqueDigest `json:"-"`
	SourcePayloadDigest OpaqueDigest `json:"-"`
	SourceFieldDigest   OpaqueDigest `json:"-"`
	SourceOrdinal       int64        `json:"-"`
	Fact                SendRecordFact
}

type Selection struct {
	SenderConfigs []SelectedSenderConfig
	SendRecords   []SelectedSendRecord
}

type Summary struct {
	SenderConfigs int
	SendRecords   int
}

func (summary Summary) Total() int { return summary.SenderConfigs + summary.SendRecords }

func (selection Selection) Summary() Summary {
	return Summary{SenderConfigs: len(selection.SenderConfigs), SendRecords: len(selection.SendRecords)}
}

// Select validates both complete source tables and the complete matching set
// of prior archive-only terminals. It never writes, reactivates, or resolves
// a sender, task, customer, queue, or Provider effect.
func Select(ctx context.Context, archive ArchiveSource, terminals TerminalReader, options SelectionOptions) (Selection, error) {
	if ctx == nil || archive == nil || terminals == nil || options.ArchiveRunID == "" || strings.TrimSpace(options.ArchiveRunID) != options.ArchiveRunID || len(options.SourceHMACKey) < sha256.Size {
		return Selection{}, ErrInvalidSelection
	}
	configs, err := selectRows(ctx, archive, terminals, options, SenderConfigTableID, SenderConfigArchiveReason, AdaptSenderConfig, func(value SenderConfigFact) int64 { return value.SourceID })
	if err != nil {
		return Selection{}, err
	}
	records, err := selectRows(ctx, archive, terminals, options, SendRecordsTableID, SendRecordArchiveReason, AdaptSendRecord, func(value SendRecordFact) int64 { return value.SourceID })
	if err != nil {
		return Selection{}, err
	}
	result := Selection{SenderConfigs: make([]SelectedSenderConfig, 0, len(configs)), SendRecords: make([]SelectedSendRecord, 0, len(records))}
	for _, value := range configs {
		result.SenderConfigs = append(result.SenderConfigs, SelectedSenderConfig{SourceKeyDigest: value.row.SourceKeyHMAC, SourcePayloadDigest: value.row.PayloadHMAC, SourceFieldDigest: value.row.FieldHMAC, SourceOrdinal: value.row.SourceOrdinal, Fact: value.fact})
	}
	for _, value := range records {
		result.SendRecords = append(result.SendRecords, SelectedSendRecord{SourceKeyDigest: value.row.SourceKeyHMAC, SourcePayloadDigest: value.row.PayloadHMAC, SourceFieldDigest: value.row.FieldHMAC, SourceOrdinal: value.row.SourceOrdinal, Fact: value.fact})
	}
	return result, nil
}

type selectedRow[T any] struct {
	row  v1archive.ArchivedRow
	fact T
}

func selectRows[T any](ctx context.Context, archive ArchiveSource, terminals TerminalReader, options SelectionOptions, table, reason string, adapt func([]byte, []byte) (T, error), sourceID func(T) int64) ([]selectedRow[T], error) {
	if table == "" || reason == "" {
		return nil, ErrInvalidSelection
	}
	values := make([]selectedRow[T], 0)
	rows := make(map[[sha256.Size]byte]v1archive.ArchivedRow)
	ids := make(map[int64]struct{})
	ordinal := int64(1)
	err := archive.EachTableRow(ctx, options.ArchiveRunID, table, func(row v1archive.ArchivedRow) error {
		if !validArchivedRow(row, table, ordinal, options.SourceHMACKey) {
			return ErrSealedDrift
		}
		fact, err := adapt(row.Payload, options.SourceHMACKey)
		if err != nil {
			return ErrSealedDrift
		}
		id := sourceID(fact)
		key, err := sourceKeyDigest(options.SourceHMACKey, table, id)
		if err != nil || key != row.SourceKeyHMAC {
			return ErrSealedDrift
		}
		if _, found := rows[row.SourceKeyHMAC]; found {
			return ErrSealedDrift
		}
		if _, found := ids[id]; found {
			return ErrSealedDrift
		}
		rows[row.SourceKeyHMAC] = row
		ids[id] = struct{}{}
		values = append(values, selectedRow[T]{row: row, fact: fact})
		ordinal++
		return nil
	})
	if err != nil || len(rows) == 0 {
		return nil, ErrSealedDrift
	}
	scope := TerminalScope{ImportVersion: OldImportVersion, ArchiveRunID: options.ArchiveRunID, AdapterID: v1archive.DefaultAdapterID, TableID: table, TargetDomain: OldTargetDomain, TargetTable: OldRuntimeArchiveTarget}
	if err := validateTerminals(ctx, terminals, scope, reason, rows); err != nil {
		return nil, err
	}
	return values, nil
}

func validArchivedRow(row v1archive.ArchivedRow, table string, ordinal int64, key []byte) bool {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || len(row.Payload) == 0 || !json.Valid(row.Payload) {
		return false
	}
	canonical, roots, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) || !sameStrings(roots, row.RedactedFields) {
		return false
	}
	payload, payloadErr := v1archive.PayloadHMAC(key, archiveTableName(table), canonical)
	fields, fieldsErr := v1archive.FieldHMAC(key, archiveTableName(table), roots)
	return payloadErr == nil && fieldsErr == nil && payload == row.PayloadHMAC && fields == row.FieldHMAC
}

func validateTerminals(ctx context.Context, terminals TerminalReader, scope TerminalScope, reason string, rows map[[sha256.Size]byte]v1archive.ArchivedRow) error {
	seen := make(map[[sha256.Size]byte]struct{}, len(rows))
	err := terminals.EachTerminal(ctx, scope, func(receipt TerminalReceipt) error {
		if !receipt.Verified || receipt.Disposition != "archive" || receipt.Reason != reason || receipt.TargetID != "" || receipt.TargetDigest != ([sha256.Size]byte{}) || receipt.Metadata == nil || len(receipt.Metadata) != 0 {
			return ErrSealedDrift
		}
		row, found := rows[receipt.SourceKeyDigest]
		if !found || row.PayloadHMAC != receipt.PayloadDigest {
			return ErrSealedDrift
		}
		if _, duplicate := seen[receipt.SourceKeyDigest]; duplicate {
			return ErrSealedDrift
		}
		seen[receipt.SourceKeyDigest] = struct{}{}
		return nil
	})
	if err != nil || len(seen) != len(rows) {
		return ErrSealedDrift
	}
	return nil
}

func sourceKeyDigest(key []byte, table string, id int64) ([sha256.Size]byte, error) {
	return v1archive.SourceKeyHMAC(key, archiveTableName(table), []byte("["+strconv.FormatInt(id, 10)+"]"))
}

func archiveTableName(table string) string { return strings.TrimPrefix(table, "public/") }

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
