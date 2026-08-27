package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1channel"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const channelDefinitionTableID = "public/automation_channel"

var channelTableIDs = [...]string{
	channelDefinitionTableID,
	"public/automation_channel_assignee",
	"public/automation_channel_contact",
	"public/automation_channel_entry_effect_log",
	"public/automation_channel_entry_runtime",
	"public/automation_channel_qrcode_asset",
	"public/automation_channel_scene_alias",
	"public/channel_welcome_effect_dependency",
	"public/channel_welcome_effect_graph",
}

type HistoricalChannelWriter interface {
	Import(context.Context, contactport.HistoricalChannelDefinition) (contactport.HistoricalChannelReceipt, error)
}

type ChannelImportResult struct {
	Imported    int
	Archived    int
	Quarantined int
	Replayed    int
}

type ChannelImporter struct {
	archive  ArchiveSource
	uow      UnitOfWork
	writer   HistoricalChannelWriter
	journals map[string]*Journal
	actorID  int64
}

// NewChannelImporter accepts exactly the nine frozen channel source tables.
// Every journal belongs to the same import/archive scope and uses Contact's
// channels target, even where a row's terminal outcome is archive/quarantine.
func NewChannelImporter(archive ArchiveSource, uow UnitOfWork, writer HistoricalChannelWriter, journals map[string]*Journal, actorID int64) (*ChannelImporter, error) {
	if archive == nil || uow == nil || writer == nil || actorID < 1 || !validChannelJournals(journals) {
		return nil, ErrInvalidScope
	}
	return &ChannelImporter{archive: archive, uow: uow, writer: writer, journals: journals, actorID: actorID}, nil
}

func (importer *ChannelImporter) Import(ctx context.Context, archiveRunID string) (ChannelImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.actorID < 1 ||
		!validChannelJournals(importer.journals) || archiveRunID == "" || archiveRunID != importer.journals[channelDefinitionTableID].scope.ArchiveRunID {
		return ChannelImportResult{}, ErrInvalidScope
	}
	result := ChannelImportResult{}
	for _, tableID := range channelTableIDs {
		journal := importer.journals[tableID]
		err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
			if !validChannelArchiveRow(row, tableID) {
				return ErrConflict
			}
			var outcome channelRowOutcome
			if tableID == channelDefinitionTableID {
				outcome = importer.importDefinition(ctx, journal, row)
			} else {
				outcome = importer.recordAuxiliary(ctx, journal, row)
			}
			if outcome.err != nil {
				return outcome.err
			}
			result.Imported += outcome.imported
			result.Archived += outcome.archived
			result.Quarantined += outcome.quarantined
			if outcome.replayed {
				result.Replayed++
			}
			return nil
		})
		if err != nil {
			return ChannelImportResult{}, err
		}
	}
	return result, nil
}

type channelRowOutcome struct {
	imported, archived, quarantined int
	replayed                        bool
	err                             error
}

type channelDefinitionJSON struct {
	ID          *int64     `json:"id"`
	ChannelCode *string    `json:"channel_code"`
	ChannelName *string    `json:"channel_name"`
	ChannelType *string    `json:"channel_type"`
	CarrierType *string    `json:"carrier_type"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

func (importer *ChannelImporter) importDefinition(ctx context.Context, journal *Journal, row v1archive.ArchivedRow) channelRowOutcome {
	definition, disposition, reason := channelDefinition(row, importer.actorID)
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		if definition == nil {
			value, err := recordChannelTerminal(tx, journal, row, disposition, reason)
			replayed = value
			return err
		}
		sourceIdentifier := SourceIdentifier(row.SourceKeyHMAC)
		existing, found, err := journal.LoadTerminal(tx, sourceIdentifier)
		if err != nil {
			return err
		}
		if found {
			if !sameImportedChannelTerminal(existing, row) {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		receipt, err := importer.writer.Import(tx, *definition)
		if err != nil {
			return err
		}
		if receipt.Replayed || !sameHistoricalChannelReceipt(receipt, *definition) {
			return ErrConflict
		}
		// The Contact writer owns the actual target and its receipt. Re-read it
		// under the source lock so a writer that omitted provenance fails closed.
		recorded, recordedFound, err := journal.LoadTerminal(tx, sourceIdentifier)
		if err != nil {
			return err
		}
		if !recordedFound || !sameHistoricalChannelTerminal(recorded, receipt) {
			return ErrConflict
		}
		return nil
	})
	if err != nil {
		return channelRowOutcome{err: err}
	}
	if definition == nil {
		if disposition == "archive" {
			return channelRowOutcome{archived: 1, replayed: replayed}
		}
		return channelRowOutcome{quarantined: 1, replayed: replayed}
	}
	return channelRowOutcome{imported: 1, replayed: replayed}
}

func (importer *ChannelImporter) recordAuxiliary(ctx context.Context, journal *Journal, row v1archive.ArchivedRow) channelRowOutcome {
	decision := v1channel.ClassifyAuxiliaryTable(channelTableName(row.TableID))
	disposition, reason := "", decision.Reason
	switch decision.Disposition {
	case v1channel.Archive:
		disposition = "archive"
	case v1channel.Blocked, v1channel.Quarantine:
		// A blocked V1 relation has no formal V2 target yet. It is not done and
		// therefore becomes a terminal quarantine, not an archive success.
		disposition = "quarantine"
	default:
		return channelRowOutcome{err: ErrConflict}
	}
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		var err error
		replayed, err = recordChannelTerminal(tx, journal, row, disposition, reason)
		return err
	})
	if err != nil {
		return channelRowOutcome{err: err}
	}
	if disposition == "archive" {
		return channelRowOutcome{archived: 1, replayed: replayed}
	}
	return channelRowOutcome{quarantined: 1, replayed: replayed}
}

func channelDefinition(row v1archive.ArchivedRow, actorID int64) (*contactport.HistoricalChannelDefinition, string, string) {
	if redactedChannelDefinition(row.RedactedFields) {
		return nil, "quarantine", "redacted_channel_definition"
	}
	var source channelDefinitionJSON
	if json.Unmarshal(row.Payload, &source) != nil || source.ID == nil || source.ChannelCode == nil || source.ChannelName == nil ||
		source.ChannelType == nil || source.CarrierType == nil || source.CreatedAt == nil || source.UpdatedAt == nil {
		return nil, "quarantine", "invalid_channel_definition"
	}
	decision := v1channel.ConvertAutomationChannel(v1channel.AutomationChannelRow{
		SourceID: *source.ID, ChannelCode: *source.ChannelCode, ChannelName: *source.ChannelName,
		ChannelType: *source.ChannelType, CarrierType: *source.CarrierType,
		CreatedAt: *source.CreatedAt, UpdatedAt: *source.UpdatedAt, SourcePayload: row.Payload,
	}, actorID)
	switch decision.Disposition {
	case v1channel.Candidate:
		if decision.Candidate == nil {
			return nil, "quarantine", "invalid_channel_definition"
		}
		candidate := decision.Candidate
		return &contactport.HistoricalChannelDefinition{
			SourceIdentifier: SourceIdentifier(row.SourceKeyHMAC), PayloadDigest: row.PayloadHMAC,
			Code: candidate.Code, Name: candidate.Name, ChannelType: candidate.Config.ChannelType,
			CarrierType: candidate.Config.CarrierType, LegacyConfigDigest: candidate.SourcePayloadDigest,
			Actor: candidate.MigrationActorID, CreatedAt: candidate.CreatedAt, UpdatedAt: candidate.UpdatedAt,
		}, "", ""
	case v1channel.Archive:
		return nil, "archive", decision.Reason
	case v1channel.Quarantine:
		return nil, "quarantine", decision.Reason
	default:
		return nil, "quarantine", "invalid_channel_definition"
	}
}

func recordChannelTerminal(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, disposition, reason string) (bool, error) {
	if disposition != "archive" && disposition != "quarantine" || reason == "" {
		return false, ErrConflict
	}
	existing, found, err := journal.LoadTerminal(ctx, SourceIdentifier(row.SourceKeyHMAC))
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != disposition ||
			existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != [sha256.Size]byte{} || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.Record(ctx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: disposition, Reason: reason})
}

func sameImportedChannelTerminal(terminal TerminalReceipt, row v1archive.ArchivedRow) bool {
	if terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" ||
		terminal.Reason != "" || terminal.TargetDigest == [sha256.Size]byte{} {
		return false
	}
	targetID, err := strconv.ParseInt(terminal.TargetID, 10, 64)
	return err == nil && targetID > 0 && strconv.FormatInt(targetID, 10) == terminal.TargetID
}

func sameHistoricalChannelReceipt(receipt contactport.HistoricalChannelReceipt, definition contactport.HistoricalChannelDefinition) bool {
	return receipt.SourceIdentifier == definition.SourceIdentifier && receipt.PayloadDigest == definition.PayloadDigest &&
		receipt.TargetID > 0 && receipt.TargetDigest != [sha256.Size]byte{}
}

func sameHistoricalChannelTerminal(terminal TerminalReceipt, receipt contactport.HistoricalChannelReceipt) bool {
	return terminal.Disposition == "import" && terminal.Reason == "" && terminal.TargetID == strconv.FormatInt(receipt.TargetID, 10) &&
		terminal.TargetDigest == receipt.TargetDigest && terminal.PayloadDigest == receipt.PayloadDigest
}

func redactedChannelDefinition(fields []string) bool {
	for _, field := range fields {
		switch field {
		case "id", "channel_code", "channel_name", "channel_type", "carrier_type", "created_at", "updated_at":
			return true
		}
	}
	return false
}

func validChannelArchiveRow(row v1archive.ArchivedRow, tableID string) bool {
	return row.TableID == tableID && row.AdapterID == v1archive.DefaultAdapterID && row.SourceOrdinal > 0 &&
		row.SourceKeyHMAC != [sha256.Size]byte{} && row.PayloadHMAC != [sha256.Size]byte{} && row.FieldHMAC != [sha256.Size]byte{}
}

func validChannelJournals(journals map[string]*Journal) bool {
	if len(journals) != len(channelTableIDs) {
		return false
	}
	var importVersion, archiveRunID string
	for index, tableID := range channelTableIDs {
		journal := journals[tableID]
		if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.TableID != tableID ||
			journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TargetDomain != "contact" || journal.scope.TargetTable != "channels" {
			return false
		}
		if index == 0 {
			importVersion, archiveRunID = journal.scope.ImportVersion, journal.scope.ArchiveRunID
		} else if journal.scope.ImportVersion != importVersion || journal.scope.ArchiveRunID != archiveRunID {
			return false
		}
	}
	return true
}

func channelTableName(tableID string) string {
	return tableID[len("public/"):]
}
