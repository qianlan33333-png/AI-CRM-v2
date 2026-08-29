package v1cycleobservationhistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var (
	ErrInvalidSelection = errors.New("cycle observation selection invalid")
	ErrSealedDrift      = errors.New("cycle observation sealed source drift")
)

// ArchiveSource streams one sealed V2 archive table. It has no source-DB or
// target-store capability.
type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

// ArchiveTerminalReceipt is one generic data_migration_row_receipts terminal.
// archive is its original terminal disposition; it is not a domain receipt.
type ArchiveTerminalReceipt struct {
	ArchiveRunID    string
	AdapterID       string
	TableID         string
	SourceKeyDigest [sha256.Size]byte
	PayloadDigest   [sha256.Size]byte
	FieldDigest     [sha256.Size]byte
	Disposition     string
	Operation       string
}

// ArchiveTerminalReader is root-owned and must enumerate every generic
// terminal for the supplied run and source table.
type ArchiveTerminalReader interface {
	EachArchiveTerminal(context.Context, string, string, func(ArchiveTerminalReceipt) error) error
}

type SelectionOptions struct {
	ArchiveRunID  string
	SourceHMACKey []byte
}

type MetricCandidate struct {
	SourceOrdinal int64
	Fact          MetricFact
}

type ReferenceCandidate struct {
	SourceOrdinal int64
	Fact          ReferenceFact
}

// Selection is complete immutable source evidence. It has no V2 run or
// snapshot relation and does not make any row executable.
type Selection struct {
	Metrics    []MetricCandidate
	References []ReferenceCandidate
}

func (value Selection) Total() int { return len(value.Metrics) + len(value.References) }

type Selector struct {
	archive   ArchiveSource
	terminals ArchiveTerminalReader
}

func NewSelector(archive ArchiveSource, terminals ArchiveTerminalReader) (*Selector, error) {
	if archive == nil || terminals == nil {
		return nil, ErrInvalidSelection
	}
	return &Selector{archive: archive, terminals: terminals}, nil
}

// Select authenticates all source rows and all generic archive terminals
// before it returns either source table. It does not write or reclassify old
// archive terminals.
func (selector *Selector) Select(ctx context.Context, options SelectionOptions) (Selection, error) {
	if selector == nil || selector.archive == nil || selector.terminals == nil || ctx == nil || options.ArchiveRunID == "" || strings.TrimSpace(options.ArchiveRunID) != options.ArchiveRunID || len(options.SourceHMACKey) < sha256.Size {
		return Selection{}, ErrInvalidSelection
	}
	metrics, err := selectTable(ctx, selector.archive, selector.terminals, options, MetricsTableID, AdaptMetric, func(value MetricFact) SourceEnvelope { return value.Source })
	if err != nil {
		return Selection{}, err
	}
	references, err := selectTable(ctx, selector.archive, selector.terminals, options, ReferencesTableID, AdaptReference, func(value ReferenceFact) SourceEnvelope { return value.Source })
	if err != nil {
		return Selection{}, err
	}
	result := Selection{Metrics: make([]MetricCandidate, 0, len(metrics)), References: make([]ReferenceCandidate, 0, len(references))}
	for _, value := range metrics {
		result.Metrics = append(result.Metrics, MetricCandidate{SourceOrdinal: value.ordinal, Fact: value.fact})
	}
	for _, value := range references {
		result.References = append(result.References, ReferenceCandidate{SourceOrdinal: value.ordinal, Fact: value.fact})
	}
	return result, nil
}

type selectedRow[T any] struct {
	ordinal int64
	fact    T
	source  SourceEnvelope
}

func selectTable[T any](ctx context.Context, archive ArchiveSource, terminals ArchiveTerminalReader, options SelectionOptions, table string, adapt func(v1archive.ArchivedRow, []byte) (T, error), source func(T) SourceEnvelope) ([]selectedRow[T], error) {
	rows := make(map[[sha256.Size]byte]selectedRow[T])
	ordered := make([]selectedRow[T], 0)
	ordinal := int64(1)
	err := archive.EachTableRow(ctx, options.ArchiveRunID, table, func(row v1archive.ArchivedRow) error {
		if row.SourceOrdinal != ordinal {
			return ErrSealedDrift
		}
		fact, err := adapt(row, options.SourceHMACKey)
		if err != nil {
			return ErrSealedDrift
		}
		value := selectedRow[T]{ordinal: row.SourceOrdinal, fact: fact, source: source(fact)}
		if value.source.SourceKeyDigest != row.SourceKeyHMAC || value.source.PayloadDigest != row.PayloadHMAC || value.source.FieldDigest != row.FieldHMAC {
			return ErrSealedDrift
		}
		if _, found := rows[row.SourceKeyHMAC]; found {
			return ErrSealedDrift
		}
		rows[row.SourceKeyHMAC] = value
		ordered = append(ordered, value)
		ordinal++
		return nil
	})
	if err != nil {
		return nil, ErrSealedDrift
	}
	if err := validateArchiveTerminals(ctx, terminals, options.ArchiveRunID, table, rows); err != nil {
		return nil, err
	}
	return ordered, nil
}

func validateArchiveTerminals[T any](ctx context.Context, terminals ArchiveTerminalReader, archiveRunID, table string, rows map[[sha256.Size]byte]selectedRow[T]) error {
	seen := make(map[[sha256.Size]byte]struct{}, len(rows))
	err := terminals.EachArchiveTerminal(ctx, archiveRunID, table, func(receipt ArchiveTerminalReceipt) error {
		if receipt.ArchiveRunID != archiveRunID || receipt.AdapterID != v1archive.DefaultAdapterID || receipt.TableID != table || receipt.SourceKeyDigest == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.FieldDigest == ([sha256.Size]byte{}) || receipt.Disposition != "archive" || receipt.Operation != "" {
			return ErrSealedDrift
		}
		row, found := rows[receipt.SourceKeyDigest]
		if !found || row.source.PayloadDigest != receipt.PayloadDigest || row.source.FieldDigest != receipt.FieldDigest {
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
