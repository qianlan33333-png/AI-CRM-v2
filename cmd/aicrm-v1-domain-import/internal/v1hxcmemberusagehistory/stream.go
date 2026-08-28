package v1hxcmemberusagehistory

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const StreamBatchSize = 250

var (
	ErrInvalidStream            = errors.New("HXC member usage stream invalid")
	ErrSealedStreamDrift        = errors.New("HXC member usage sealed stream drift")
	ErrTerminalVerification     = errors.New("HXC member usage archive terminal verification failed")
	ErrStreamConsumer           = errors.New("HXC member usage stream consumer failed")
	ErrStreamContextInterrupted = errors.New("HXC member usage stream interrupted")
)

// ArchiveSource streams only immutable V2 archive rows. It cannot query V1 or
// write a domain target.
type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

// TerminalReader verifies the exact generic archive terminals for one bounded
// source batch. The root-owned implementation is responsible for the SQL
// evidence; no prior domain receipt is assumed here.
type TerminalReader interface {
	VerifyArchiveTerminals(context.Context, string, []SourceEnvelope) error
}

type StreamOptions struct {
	ArchiveRunID  string
	SourceHMACKey []byte
}

// StreamSummary is returned only after the full source stream completes.
// SourceRollingDigest follows the sealed archive order: ordinal then source,
// payload, and field HMACs for every accepted observation.
type StreamSummary struct {
	SourceCount         int64
	SourceRollingDigest [sha256.Size]byte
}

// ConsumeMemberUsageBatch receives a batch only after its archive terminals
// have been verified. A nil consumer performs a complete read-only preflight.
type ConsumeMemberUsageBatch func(context.Context, []MemberUsageObservationFact) error

type Streamer struct {
	archive   ArchiveSource
	terminals TerminalReader
}

func NewStreamer(archive ArchiveSource, terminals TerminalReader) (*Streamer, error) {
	if archive == nil || terminals == nil {
		return nil, ErrInvalidStream
	}
	return &Streamer{archive: archive, terminals: terminals}, nil
}

// Stream validates the entire immutable source in fixed batches. Callers that
// intend to import must first call it with a nil consumer, then rerun it with
// their same-transaction consumer after a successful complete preflight.
func (streamer *Streamer) Stream(ctx context.Context, options StreamOptions, consume ConsumeMemberUsageBatch) (StreamSummary, error) {
	if streamer == nil || streamer.archive == nil || streamer.terminals == nil || ctx == nil || ctx.Err() != nil ||
		options.ArchiveRunID == "" || strings.TrimSpace(options.ArchiveRunID) != options.ArchiveRunID || len(options.SourceHMACKey) < sha256.Size {
		return StreamSummary{}, ErrInvalidStream
	}

	digest := sha256.New()
	batch := make([]MemberUsageObservationFact, 0, StreamBatchSize)
	envelopes := make([]SourceEnvelope, 0, StreamBatchSize)
	batchKeys := make(map[[sha256.Size]byte]struct{}, StreamBatchSize)
	var accepted int64
	expectedOrdinal := int64(1)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := streamer.terminals.VerifyArchiveTerminals(ctx, options.ArchiveRunID, append([]SourceEnvelope(nil), envelopes...)); err != nil {
			return ErrTerminalVerification
		}
		if consume != nil {
			if err := consume(ctx, append([]MemberUsageObservationFact(nil), batch...)); err != nil {
				return ErrStreamConsumer
			}
		}
		for _, envelope := range envelopes {
			writeEnvelopeDigest(digest, envelope)
			accepted++
		}
		batch = batch[:0]
		envelopes = envelopes[:0]
		clear(batchKeys)
		return nil
	}

	err := streamer.archive.EachTableRow(ctx, options.ArchiveRunID, MemberUsageProjectionTableID, func(row v1archive.ArchivedRow) error {
		if ctx.Err() != nil {
			return ErrStreamContextInterrupted
		}
		result := AdaptMemberUsageObservation(row, options.SourceHMACKey, expectedOrdinal)
		if result.Disposition != DispositionCandidate || result.Fact == nil || result.Reason != "" || !sameArchiveEnvelope(result.Fact.Source, row) {
			return ErrSealedStreamDrift
		}
		if _, duplicate := batchKeys[row.SourceKeyHMAC]; duplicate {
			return ErrSealedStreamDrift
		}
		batchKeys[row.SourceKeyHMAC] = struct{}{}
		batch = append(batch, *result.Fact)
		envelopes = append(envelopes, result.Fact.Source)
		expectedOrdinal++
		if len(batch) == StreamBatchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrStreamContextInterrupted) {
			return StreamSummary{}, ErrStreamContextInterrupted
		}
		if errors.Is(err, ErrTerminalVerification) || errors.Is(err, ErrStreamConsumer) || errors.Is(err, ErrSealedStreamDrift) {
			return StreamSummary{}, err
		}
		return StreamSummary{}, ErrSealedStreamDrift
	}
	if err := flush(); err != nil {
		return StreamSummary{}, err
	}
	var result StreamSummary
	result.SourceCount = accepted
	copy(result.SourceRollingDigest[:], digest.Sum(nil))
	return result, nil
}

func sameArchiveEnvelope(envelope SourceEnvelope, row v1archive.ArchivedRow) bool {
	return envelope.SourceOrdinal == row.SourceOrdinal && envelope.SourceKeyHMAC == row.SourceKeyHMAC &&
		envelope.PayloadHMAC == row.PayloadHMAC && envelope.FieldHMAC == row.FieldHMAC &&
		!zeroDigest(envelope.SourceKeyHMAC) && !zeroDigest(envelope.PayloadHMAC) && !zeroDigest(envelope.FieldHMAC)
}

func writeEnvelopeDigest(digest interface{ Write([]byte) (int, error) }, envelope SourceEnvelope) {
	var ordinal [8]byte
	binary.BigEndian.PutUint64(ordinal[:], uint64(envelope.SourceOrdinal))
	_, _ = digest.Write(ordinal[:])
	_, _ = digest.Write(envelope.SourceKeyHMAC[:])
	_, _ = digest.Write(envelope.PayloadHMAC[:])
	_, _ = digest.Write(envelope.FieldHMAC[:])
}
