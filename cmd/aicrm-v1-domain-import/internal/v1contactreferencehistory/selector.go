package v1contactreferencehistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var (
	ErrInvalidSelection = errors.New("contact reference selection invalid")
	ErrSealedDrift      = errors.New("contact reference sealed source drift")
)

// ArchiveSource streams authenticated, redacted rows from one reconciled V2
// archive run. It is read-only.
type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

// ArchiveTerminalReceipt is one immutable generic archive receipt. Unlike a
// domain import receipt it has no target scope or import version.
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

// ArchiveTerminalReader is root-owned. It must enumerate generic archive
// terminals for the exact run and table from data_migration_row_receipts.
type ArchiveTerminalReader interface {
	EachArchiveTerminal(context.Context, string, string, func(ArchiveTerminalReceipt) error) error
}

type SelectionOptions struct {
	ArchiveRunID  string
	SourceHMACKey []byte
}

// BindingCandidate is inert V1 source evidence. Its Fact contains no resolved
// person, Customer, owner, staff, or identity relation.
type BindingCandidate struct {
	SourceOrdinal int64
	Fact          ExternalContactBindingFact
}

// DirectoryMemberCandidate is inert V1 source evidence. It is not a Staff
// creation, login, role, or current-directory action.
type DirectoryMemberCandidate struct {
	SourceOrdinal int64
	Fact          DirectoryMemberFact
}

type Selection struct {
	Bindings         []BindingCandidate
	DirectoryMembers []DirectoryMemberCandidate
}

func (value Selection) Total() int { return len(value.Bindings) + len(value.DirectoryMembers) }

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

// Select validates each complete source table before returning any candidate.
// It does not write, resolve a relation, or reclassify old archive terminals.
func (selector *Selector) Select(ctx context.Context, options SelectionOptions) (Selection, error) {
	if selector == nil || selector.archive == nil || selector.terminals == nil || ctx == nil || options.ArchiveRunID == "" || strings.TrimSpace(options.ArchiveRunID) != options.ArchiveRunID || len(options.SourceHMACKey) < sha256.Size {
		return Selection{}, ErrInvalidSelection
	}
	bindings, err := selectTable(ctx, selector.archive, selector.terminals, options, ExternalContactBindingsTableID, AdaptExternalContactBinding, func(value ExternalContactBindingFact) SourceEnvelope { return value.Source })
	if err != nil {
		return Selection{}, err
	}
	directory, err := selectTable(ctx, selector.archive, selector.terminals, options, AdminWeComDirectoryMembersTableID, AdaptDirectoryMember, func(value DirectoryMemberFact) SourceEnvelope { return value.Source })
	if err != nil {
		return Selection{}, err
	}
	result := Selection{Bindings: make([]BindingCandidate, 0, len(bindings)), DirectoryMembers: make([]DirectoryMemberCandidate, 0, len(directory))}
	for _, value := range bindings {
		result.Bindings = append(result.Bindings, BindingCandidate{SourceOrdinal: value.ordinal, Fact: value.fact})
	}
	for _, value := range directory {
		result.DirectoryMembers = append(result.DirectoryMembers, DirectoryMemberCandidate{SourceOrdinal: value.ordinal, Fact: value.fact})
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
		if receipt.ArchiveRunID != archiveRunID || receipt.AdapterID != v1archive.DefaultAdapterID || receipt.TableID != table ||
			receipt.SourceKeyDigest == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.FieldDigest == ([sha256.Size]byte{}) ||
			receipt.Disposition != "archive" || receipt.Operation != "" {
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
