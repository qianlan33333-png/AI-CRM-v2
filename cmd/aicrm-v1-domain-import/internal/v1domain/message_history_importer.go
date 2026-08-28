package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1messagehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

// MessageHistoryWriter owns the target projection and verifies target replay.
// It must use the same caller transaction as the importer receipt journal.
type MessageHistoryWriter interface {
	Write(context.Context, string, [sha256.Size]byte, wecomport.HistoricalMessage) (wecomport.MessageHistoryReceipt, error)
}

// MessageHistoryResolver accepts only a V1 unionid and returns a previously
// verified historical DM01 customer mapping. A nil result remains unresolved.
type MessageHistoryResolver interface {
	ResolveHistoricalMessageCustomer(context.Context, string) (*int64, error)
}

type MessageHistoryImportJournal interface {
	wecomport.MessageHistoryJournal
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
	ValidateMessageHistoryImportScope(string) error
}

type MessageHistoryImportResult struct {
	Imported, Quarantined, Replayed int
}

// MessageHistoryImporter streams one source table. It never retains an
// archived raw message body beyond its current callback.
type MessageHistoryImporter struct {
	archive  ArchiveSource
	uow      UnitOfWork
	writer   MessageHistoryWriter
	resolver MessageHistoryResolver
	journal  MessageHistoryImportJournal
}

func NewMessageHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer MessageHistoryWriter, resolver MessageHistoryResolver, journal MessageHistoryImportJournal) (*MessageHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &MessageHistoryImporter{archive: archive, uow: uow, writer: writer, resolver: resolver, journal: journal}, nil
}

func (importer *MessageHistoryImporter) Import(ctx context.Context, archiveRunID string) (MessageHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.resolver == nil || importer.journal == nil {
		return MessageHistoryImportResult{}, ErrInvalidScope
	}
	if err := importer.journal.ValidateMessageHistoryImportScope(archiveRunID); err != nil {
		return MessageHistoryImportResult{}, err
	}

	result := MessageHistoryImportResult{}
	seenKeys := map[[sha256.Size]byte]struct{}{}
	seenSourceIDs := map[int64]struct{}{}
	expectedOrdinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, messageHistoryTableID, func(row v1archive.ArchivedRow) error {
		if !validMessageHistoryArchiveRow(row, expectedOrdinal) {
			return ErrConflict
		}
		expectedOrdinal++
		if _, duplicate := seenKeys[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seenKeys[row.SourceKeyHMAC] = struct{}{}

		decision, reason := adaptArchivedMessageHistory(row)
		if decision.Fact != nil {
			if _, duplicate := seenSourceIDs[decision.Fact.Source.ID]; duplicate {
				return ErrConflict
			}
			seenSourceIDs[decision.Fact.Source.ID] = struct{}{}
		}

		replayed := false
		imported := false
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			// A retry may execute this callback more than once; expose only the
			// outcome of the successful transaction attempt.
			replayed, imported = false, false
			if reason != "" || decision.Disposition != v1messagehistory.Candidate || decision.Fact == nil {
				if reason == "" {
					reason = "invalid_message_history"
				}
				var err error
				replayed, err = recordMessageHistoryQuarantine(tx, importer.journal, row, reason)
				return err
			}
			value, err := importer.messageValue(tx, row, *decision.Fact)
			if err != nil {
				return err
			}
			receipt, err := importer.writer.Write(tx, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, value)
			if errors.Is(err, wecomport.ErrMessageHistoryInvalid) {
				var terminalErr error
				replayed, terminalErr = recordMessageHistoryQuarantine(tx, importer.journal, row, "message_target_invalid")
				return terminalErr
			}
			if err != nil {
				return err
			}
			if err = verifyMessageHistoryReceipt(tx, importer.journal, row, receipt); err != nil {
				return err
			}
			replayed, imported = receipt.Replayed, true
			return nil
		}); err != nil {
			return err
		}
		if imported {
			result.Imported++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
		return nil
	})
	return result, err
}

func validMessageHistoryArchiveRow(row v1archive.ArchivedRow, ordinal int64) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == messageHistoryTableID && row.SourceOrdinal == ordinal &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{})
}

func adaptArchivedMessageHistory(row v1archive.ArchivedRow) (v1messagehistory.Result, string) {
	optionalRedactions := map[string]struct{}{"seq": {}, "owner_userid": {}, "receiver": {}, "content": {}}
	for _, field := range row.RedactedFields {
		if _, optional := optionalRedactions[field]; !optional {
			return v1messagehistory.Result{}, "redacted_message_history_field"
		}
	}
	payload := row.Payload
	if len(row.RedactedFields) > 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(payload, &fields) != nil || fields == nil {
			return v1messagehistory.Result{}, "invalid_message_history"
		}
		for _, field := range row.RedactedFields {
			fields[field] = json.RawMessage("null")
		}
		var err error
		payload, err = json.Marshal(fields)
		if err != nil {
			return v1messagehistory.Result{}, "invalid_message_history"
		}
	}
	decision := v1messagehistory.AdaptMessage(payload)
	if decision.Disposition != v1messagehistory.Candidate || decision.Fact == nil {
		if decision.Reason == "" {
			return decision, "invalid_message_history"
		}
		return decision, decision.Reason
	}
	if decision.Fact.Source.Content != nil && strings.ContainsRune(*decision.Fact.Source.Content, '\x00') {
		return decision, "message_content_nul"
	}
	if strings.ContainsRune(decision.Fact.Source.ChatType, '\x00') || strings.ContainsRune(decision.Fact.Source.MessageType, '\x00') ||
		strings.ContainsRune(decision.Fact.Source.SendTime, '\x00') {
		return decision, "message_history_nul"
	}
	return decision, ""
}

func (importer *MessageHistoryImporter) messageValue(ctx context.Context, row v1archive.ArchivedRow, fact v1messagehistory.Fact) (wecomport.HistoricalMessage, error) {
	if fact.CreatedAt == nil || fact.Source.ID < 1 {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryInvalid
	}
	var customerID *int64
	if fact.Source.UnionID != "" {
		resolved, err := importer.resolver.ResolveHistoricalMessageCustomer(ctx, fact.Source.UnionID)
		if err != nil {
			return wecomport.HistoricalMessage{}, err
		}
		if resolved != nil {
			if *resolved < 1 {
				return wecomport.HistoricalMessage{}, ErrConflict
			}
			id := *resolved
			customerID = &id
		}
	}
	masked, err := maskMessageHistoryContent(fact.Source.Content)
	if err != nil {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryInvalid
	}
	value := wecomport.HistoricalMessage{SourceID: fact.Source.ID, Sequence: copyMessageHistoryInt64(fact.Source.Seq), CustomerID: customerID,
		ChatType: fact.Source.ChatType, MessageType: fact.Source.MessageType, ContentMasked: masked, OriginalSendTime: fact.Source.SendTime,
		SendTimeBasis: fact.SendTimeBasis, SentAt: copyMessageHistoryTime(fact.SentAt), CreatedAt: fact.CreatedAt.UTC().Truncate(time.Microsecond),
		SourcePayloadDigest: row.PayloadHMAC}
	return value, nil
}

func maskMessageHistoryContent(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if !utf8.ValidString(*value) || strings.ContainsRune(*value, '\x00') {
		return nil, wecomport.ErrMessageHistoryInvalid
	}
	var masked strings.Builder
	masked.Grow(len(*value))
	for offset := 0; offset < len(*value); {
		end := offset
		if (*value)[offset] == '+' {
			end++
		}
		for end < len(*value) && (*value)[end] >= '0' && (*value)[end] <= '9' {
			end++
		}
		if messageHistoryPhoneLike((*value)[offset:end]) {
			masked.WriteString("[masked-phone]")
			offset = end
			continue
		}
		_, width := utf8.DecodeRuneInString((*value)[offset:])
		masked.WriteString((*value)[offset : offset+width])
		offset += width
	}
	result := masked.String()
	return &result, nil
}

func messageHistoryPhoneLike(value string) bool {
	digits := strings.TrimPrefix(value, "+86")
	return len(digits) == 11 && digits[0] == '1' && digits[1] >= '3' && digits[1] <= '9'
}

func copyMessageHistoryInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyMessageHistoryTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC().Truncate(time.Microsecond)
	return &copy
}

func recordMessageHistoryQuarantine(ctx context.Context, journal MessageHistoryImportJournal, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" || journal == nil {
		return false, ErrInvalidScope
	}
	sourceIdentifier := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := journal.LoadTerminal(ctx, sourceIdentifier)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" ||
			existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.Record(ctx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
		Disposition: "quarantine", Reason: reason})
}

func verifyMessageHistoryReceipt(ctx context.Context, journal MessageHistoryImportJournal, row v1archive.ArchivedRow, receipt wecomport.MessageHistoryReceipt) error {
	if journal == nil || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := journal.LoadTerminal(ctx, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" ||
		terminal.Reason != "" || terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}
