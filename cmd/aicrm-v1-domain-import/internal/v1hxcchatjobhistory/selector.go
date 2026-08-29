package v1hxcchatjobhistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var (
	ErrInvalidSelection = errors.New("HXC chat-job selection invalid")
	ErrSealedDrift      = errors.New("HXC chat-job sealed source drift")
)

// ArchiveSource streams one sealed V2 archive table. It has no V1 database or
// target-store capability.
type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

// ArchiveTerminalReceipt is one generic data_migration_row_receipts terminal.
// It records the original archive disposition and is not a HXC domain receipt.
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

// ArchiveTerminalReader must enumerate every generic terminal for the exact
// archive run and source table. No pre-existing domain receipt is required.
type ArchiveTerminalReader interface {
	EachArchiveTerminal(context.Context, string, string, func(ArchiveTerminalReceipt) error) error
}

type SelectionOptions struct {
	ArchiveRunID  string
	SourceHMACKey []byte
}

type ChatJobCandidate struct {
	SourceOrdinal int64
	Fact          ChatJobFact
}

// Selection is complete read-only source evidence. It does not establish a
// current relation or execute a historical job.
type Selection struct {
	Jobs []ChatJobCandidate
}

func (selection Selection) Total() int { return len(selection.Jobs) }

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

// Select authenticates all archived chat-job rows and pairs each with exactly
// one generic archive terminal before returning any candidate.
func (selector *Selector) Select(ctx context.Context, options SelectionOptions) (Selection, error) {
	if selector == nil || selector.archive == nil || selector.terminals == nil || ctx == nil || options.ArchiveRunID == "" ||
		strings.TrimSpace(options.ArchiveRunID) != options.ArchiveRunID || len(options.SourceHMACKey) < sha256.Size {
		return Selection{}, ErrInvalidSelection
	}

	rows := make(map[[sha256.Size]byte]ChatJobCandidate)
	ordered := make([]ChatJobCandidate, 0)
	ordinal := int64(1)
	err := selector.archive.EachTableRow(ctx, options.ArchiveRunID, ChatJobsTableID, func(row v1archive.ArchivedRow) error {
		if row.SourceOrdinal != ordinal {
			return ErrSealedDrift
		}
		fact, err := AdaptChatJob(row, options.SourceHMACKey)
		if err != nil || fact.Source.SourceKeyDigest != row.SourceKeyHMAC || fact.Source.PayloadDigest != row.PayloadHMAC || fact.Source.FieldDigest != row.FieldHMAC {
			return ErrSealedDrift
		}
		if _, duplicate := rows[row.SourceKeyHMAC]; duplicate {
			return ErrSealedDrift
		}
		candidate := ChatJobCandidate{SourceOrdinal: row.SourceOrdinal, Fact: fact}
		rows[row.SourceKeyHMAC] = candidate
		ordered = append(ordered, candidate)
		ordinal++
		return nil
	})
	if err != nil {
		return Selection{}, ErrSealedDrift
	}
	if err := validateArchiveTerminals(ctx, selector.terminals, options.ArchiveRunID, rows); err != nil {
		return Selection{}, err
	}
	return Selection{Jobs: ordered}, nil
}

func validateArchiveTerminals(ctx context.Context, terminals ArchiveTerminalReader, archiveRunID string, rows map[[sha256.Size]byte]ChatJobCandidate) error {
	seen := make(map[[sha256.Size]byte]struct{}, len(rows))
	err := terminals.EachArchiveTerminal(ctx, archiveRunID, ChatJobsTableID, func(receipt ArchiveTerminalReceipt) error {
		if receipt.ArchiveRunID != archiveRunID || receipt.AdapterID != v1archive.DefaultAdapterID || receipt.TableID != ChatJobsTableID ||
			receipt.SourceKeyDigest == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.FieldDigest == ([sha256.Size]byte{}) ||
			receipt.Disposition != "archive" || receipt.Operation != "" {
			return ErrSealedDrift
		}
		candidate, found := rows[receipt.SourceKeyDigest]
		if !found || candidate.Fact.Source.PayloadDigest != receipt.PayloadDigest || candidate.Fact.Source.FieldDigest != receipt.FieldDigest {
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
