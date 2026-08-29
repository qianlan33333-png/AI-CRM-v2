package v1audienceactivityhistory

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const FixedBatchSize = 250

const (
	ReasonFieldRedacted = "audience_activity_field_redacted"
	ReasonShapeInvalid  = "audience_activity_shape_invalid"
)

var ErrArchiveRow = errors.New("audience activity archive row invalid")

type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

// SourceEnvelope is authenticated private archive proof, never an API model.
type SourceEnvelope struct {
	SourceOrdinal int64
	SourceKeyHMAC [sha256.Size]byte
	PayloadHMAC   [sha256.Size]byte
	FieldHMAC     [sha256.Size]byte
}

type TerminalVerifier interface {
	VerifyAudienceActivityTerminal(context.Context, string, SourceEnvelope, Disposition, string) error
}

type BatchConsumer interface {
	ConsumeAudienceActivityPackageRunBatch(context.Context, []PackageRunResult) error
	ConsumeAudienceActivityMemberEventBatch(context.Context, []MemberEventResult) error
}

type StreamSummary struct {
	PackageRuns, MemberEvents int64
	Candidates, Quarantined   int64
}

func AdaptPackageRunArchive(row v1archive.ArchivedRow, key []byte, expectedOrdinal int64) PackageRunResult {
	envelope, err := audienceActivityEnvelope(row, PackageRunsTableID, key, expectedOrdinal)
	if err != nil {
		return PackageRunResult{Source: sourceEnvelope(row), Disposition: DispositionQuarantine, Reason: ReasonShapeInvalid}
	}
	if len(row.RedactedFields) != 0 {
		return PackageRunResult{Source: envelope, Disposition: DispositionQuarantine, Reason: ReasonFieldRedacted}
	}
	result := adaptPackageRun(row.Payload)
	result.Source = envelope
	return result
}

func AdaptMemberEventArchive(row v1archive.ArchivedRow, key []byte, expectedOrdinal int64) MemberEventResult {
	envelope, err := audienceActivityEnvelope(row, MemberEventsTableID, key, expectedOrdinal)
	if err != nil {
		return MemberEventResult{Source: sourceEnvelope(row), Disposition: DispositionQuarantine, Reason: ReasonShapeInvalid}
	}
	if len(row.RedactedFields) != 0 {
		return MemberEventResult{Source: envelope, Disposition: DispositionQuarantine, Reason: ReasonFieldRedacted}
	}
	result := adaptMemberEvent(row.Payload)
	result.Source = envelope
	return result
}

// Stream authenticates every row before it reaches the owner. Ordinals reset
// for each sealed source table; no table-sized collection or sorting occurs.
func Stream(ctx context.Context, archive ArchiveSource, run string, key []byte, verifier TerminalVerifier, consumer BatchConsumer) (StreamSummary, error) {
	if ctx == nil || archive == nil || verifier == nil || run == "" || strings.TrimSpace(run) != run || len(key) < sha256.Size {
		return StreamSummary{}, ErrArchiveRow
	}
	var summary StreamSummary
	if err := streamRuns(ctx, archive, run, key, verifier, consumer, &summary); err != nil {
		return StreamSummary{}, err
	}
	if err := streamEvents(ctx, archive, run, key, verifier, consumer, &summary); err != nil {
		return StreamSummary{}, err
	}
	return summary, nil
}

func streamRuns(ctx context.Context, archive ArchiveSource, run string, key []byte, verifier TerminalVerifier, consumer BatchConsumer, summary *StreamSummary) error {
	ordinal := int64(1)
	batch := make([]PackageRunResult, 0, FixedBatchSize)
	err := archive.EachTableRow(ctx, run, PackageRunsTableID, func(row v1archive.ArchivedRow) error {
		if _, err := audienceActivityEnvelope(row, PackageRunsTableID, key, ordinal); err != nil {
			return err
		}
		value := AdaptPackageRunArchive(row, key, ordinal)
		if err := verifier.VerifyAudienceActivityTerminal(ctx, PackageRunsTableID, value.Source, value.Disposition, value.Reason); err != nil {
			return err
		}
		summary.PackageRuns++
		if value.Disposition == DispositionCandidate {
			summary.Candidates++
		} else {
			summary.Quarantined++
		}
		batch = append(batch, value)
		ordinal++
		if len(batch) == FixedBatchSize {
			if consumer != nil {
				if err := consumer.ConsumeAudienceActivityPackageRunBatch(ctx, batch); err != nil {
					return err
				}
			}
			batch = batch[:0]
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(batch) != 0 && consumer != nil {
		return consumer.ConsumeAudienceActivityPackageRunBatch(ctx, batch)
	}
	return nil
}

func streamEvents(ctx context.Context, archive ArchiveSource, run string, key []byte, verifier TerminalVerifier, consumer BatchConsumer, summary *StreamSummary) error {
	ordinal := int64(1)
	batch := make([]MemberEventResult, 0, FixedBatchSize)
	err := archive.EachTableRow(ctx, run, MemberEventsTableID, func(row v1archive.ArchivedRow) error {
		if _, err := audienceActivityEnvelope(row, MemberEventsTableID, key, ordinal); err != nil {
			return err
		}
		value := AdaptMemberEventArchive(row, key, ordinal)
		if err := verifier.VerifyAudienceActivityTerminal(ctx, MemberEventsTableID, value.Source, value.Disposition, value.Reason); err != nil {
			return err
		}
		summary.MemberEvents++
		if value.Disposition == DispositionCandidate {
			summary.Candidates++
		} else {
			summary.Quarantined++
		}
		batch = append(batch, value)
		ordinal++
		if len(batch) == FixedBatchSize {
			if consumer != nil {
				if err := consumer.ConsumeAudienceActivityMemberEventBatch(ctx, batch); err != nil {
					return err
				}
			}
			batch = batch[:0]
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(batch) != 0 && consumer != nil {
		return consumer.ConsumeAudienceActivityMemberEventBatch(ctx, batch)
	}
	return nil
}

func audienceActivityEnvelope(row v1archive.ArchivedRow, table string, key []byte, ordinal int64) (SourceEnvelope, error) {
	if ordinal < 1 || len(key) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
		return SourceEnvelope{}, ErrArchiveRow
	}
	canonical, _, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) {
		return SourceEnvelope{}, ErrArchiveRow
	}
	tableName := strings.TrimPrefix(table, "public/")
	payload, err := v1archive.PayloadHMAC(key, tableName, canonical)
	if err != nil || !hmac.Equal(payload[:], row.PayloadHMAC[:]) {
		return SourceEnvelope{}, ErrArchiveRow
	}
	field, err := v1archive.FieldHMAC(key, tableName, row.RedactedFields)
	if err != nil || !hmac.Equal(field[:], row.FieldHMAC[:]) {
		return SourceEnvelope{}, ErrArchiveRow
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(row.Payload, &fields) != nil || fields == nil || fields["id"] == nil || bytes.Equal(bytes.TrimSpace(fields["id"]), []byte("null")) {
		return SourceEnvelope{}, ErrArchiveRow
	}
	keyJSON, err := json.Marshal([]json.RawMessage{fields["id"]})
	if err != nil {
		return SourceEnvelope{}, ErrArchiveRow
	}
	source, err := v1archive.SourceKeyHMAC(key, tableName, keyJSON)
	if err != nil || !hmac.Equal(source[:], row.SourceKeyHMAC[:]) {
		return SourceEnvelope{}, ErrArchiveRow
	}
	return sourceEnvelope(row), nil
}

func sourceEnvelope(row v1archive.ArchivedRow) SourceEnvelope {
	return SourceEnvelope{SourceOrdinal: row.SourceOrdinal, SourceKeyHMAC: row.SourceKeyHMAC, PayloadHMAC: row.PayloadHMAC, FieldHMAC: row.FieldHMAC}
}
