package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1radarhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

const (
	RadarClickHistoryImportVersion = "v1-radar-click-history-a1"
	RadarClickHistoryDomain        = "radar-click-history"
	RadarClickHistoryKind          = "radar_click"
	RadarClickHistoryTarget        = "radar_v1_click_history"
)

type RadarClickHistoryWriter interface {
	ImportHistoricalRadarClick(context.Context, string, radarport.HistoricalRadarClick) (radarport.RadarClickHistoryReceipt, error)
}

type RadarClickHistoryReferenceResolver interface {
	ResolveHistoricalRadarClick(context.Context, v1radarhistory.ClickFact) (*int64, *int64, error)
}

type RadarClickHistoryImportJournal interface {
	radarport.RadarClickHistoryJournal
	LoadRadarClickHistoryTerminal(context.Context, string) (TerminalReceipt, bool, error)
	RecordRadarClickHistoryTerminal(context.Context, TerminalReceipt) error
}

type RadarClickHistoryImportResult struct{ Imported, Quarantined, Replayed int }

type RadarClickHistoryImporter struct {
	archive    ArchiveSource
	uow        UnitOfWork
	writer     RadarClickHistoryWriter
	references RadarClickHistoryReferenceResolver
	journal    RadarClickHistoryImportJournal
	expected   int
}

func NewRadarClickHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer RadarClickHistoryWriter, references RadarClickHistoryReferenceResolver, journal RadarClickHistoryImportJournal) (*RadarClickHistoryImporter, error) {
	return newRadarClickHistoryImporter(archive, uow, writer, references, journal, 1735)
}

func NewRadarClickHistoryJournal(journal *Journal) (*radarClickHistoryJournal, error) {
	if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ImportVersion != RadarClickHistoryImportVersion || journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != v1radarhistory.ClickTableID || journal.scope.TargetDomain != "radar" || journal.scope.TargetTable != RadarClickHistoryTarget {
		return nil, ErrInvalidScope
	}
	return &radarClickHistoryJournal{journal: journal}, nil
}

type radarClickHistoryJournal struct{ journal *Journal }

var _ RadarClickHistoryImportJournal = (*radarClickHistoryJournal)(nil)

func (j *radarClickHistoryJournal) LoadRadarClickHistory(ctx context.Context, kind, source string) (radarport.RadarClickHistoryReceipt, bool, error) {
	if kind != RadarClickHistoryKind {
		return radarport.RadarClickHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := j.LoadRadarClickHistoryTerminal(ctx, source)
	if err != nil || !found {
		return radarport.RadarClickHistoryReceipt{}, found, err
	}
	return radarClickHistoryReceipt(source, terminal)
}

func (j *radarClickHistoryJournal) RecordRadarClickHistory(ctx context.Context, receipt radarport.RadarClickHistoryReceipt) error {
	if receipt.Kind != RadarClickHistoryKind {
		return ErrInvalidScope
	}
	terminal, err := radarClickHistoryTerminal(receipt)
	if err != nil {
		return err
	}
	return j.RecordRadarClickHistoryTerminal(ctx, terminal)
}

func (j *radarClickHistoryJournal) LoadRadarClickHistoryTerminal(ctx context.Context, source string) (TerminalReceipt, bool, error) {
	if j == nil || j.journal == nil {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return j.journal.LoadTerminal(ctx, source)
}

func (j *radarClickHistoryJournal) RecordRadarClickHistoryTerminal(ctx context.Context, receipt TerminalReceipt) error {
	if j == nil || j.journal == nil {
		return ErrInvalidScope
	}
	return j.journal.Record(ctx, receipt)
}

func newRadarClickHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer RadarClickHistoryWriter, references RadarClickHistoryReferenceResolver, journal RadarClickHistoryImportJournal, expected int) (*RadarClickHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || references == nil || journal == nil || expected < 0 {
		return nil, ErrInvalidScope
	}
	return &RadarClickHistoryImporter{archive: archive, uow: uow, writer: writer, references: references, journal: journal, expected: expected}, nil
}

func (i *RadarClickHistoryImporter) Import(ctx context.Context, run string) (RadarClickHistoryImportResult, error) {
	if i == nil || ctx == nil || run == "" || i.archive == nil || i.uow == nil || i.writer == nil || i.references == nil || i.journal == nil {
		return RadarClickHistoryImportResult{}, ErrInvalidScope
	}
	rows, err := i.readRows(ctx, run)
	if err != nil {
		return RadarClickHistoryImportResult{}, err
	}
	if len(rows) != i.expected {
		return RadarClickHistoryImportResult{}, ErrConflict
	}
	decisions := v1radarhistory.AdaptClicks(radarClickPayloads(rows))
	result := RadarClickHistoryImportResult{}
	for index := range rows {
		if err = i.importRow(ctx, rows[index], decisions[index], &result); err != nil {
			return RadarClickHistoryImportResult{}, err
		}
	}
	return result, nil
}

func (i *RadarClickHistoryImporter) readRows(ctx context.Context, run string) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0, i.expected)
	seen := map[[sha256.Size]byte]struct{}{}
	err := i.archive.EachTableRow(ctx, run, v1radarhistory.ClickTableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != v1radarhistory.ClickTableID || row.SourceOrdinal != int64(len(rows)+1) || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
			return ErrConflict
		}
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

func radarClickPayloads(rows []v1archive.ArchivedRow) []json.RawMessage {
	values := make([]json.RawMessage, len(rows))
	for index := range rows {
		values[index] = append(json.RawMessage(nil), rows[index].Payload...)
	}
	return values
}

func (i *RadarClickHistoryImporter) importRow(ctx context.Context, row v1archive.ArchivedRow, decision v1radarhistory.Result, result *RadarClickHistoryImportResult) error {
	imported, replayed := false, false
	err := i.uow.Within(ctx, func(tx context.Context) error {
		imported, replayed = false, false
		if len(row.RedactedFields) != 0 || decision.Disposition != v1radarhistory.DispositionCandidate || decision.Fact == nil {
			reason := decision.Reason
			if len(row.RedactedFields) != 0 {
				reason = "radar_click_source_redacted"
			}
			if reason == "" {
				reason = "invalid_radar_click"
			}
			var recordErr error
			replayed, recordErr = recordRadarClickHistoryQuarantine(tx, i.journal, row, reason)
			return recordErr
		}
		radarLinkID, customerID, resolveErr := i.references.ResolveHistoricalRadarClick(tx, *decision.Fact)
		if resolveErr != nil {
			return resolveErr
		}
		value := radarClickHistoryValue(row, *decision.Fact, radarLinkID, customerID)
		receipt, writeErr := i.writer.ImportHistoricalRadarClick(tx, SourceIdentifier(row.SourceKeyHMAC), value)
		if errors.Is(writeErr, radarport.ErrRadarClickHistoryInvalid) {
			var recordErr error
			replayed, recordErr = recordRadarClickHistoryQuarantine(tx, i.journal, row, "radar_click_target_invalid")
			return recordErr
		}
		if writeErr != nil {
			return writeErr
		}
		if verifyErr := verifyRadarClickHistoryReceipt(tx, i.journal, row, receipt); verifyErr != nil {
			return verifyErr
		}
		imported, replayed = true, receipt.Replayed
		return nil
	})
	if err != nil {
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
}

func radarClickHistoryValue(row v1archive.ArchivedRow, fact v1radarhistory.ClickFact, radarLinkID, customerID *int64) radarport.HistoricalRadarClick {
	return radarport.HistoricalRadarClick{SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC, SourceID: fact.SourceID, LinkSourceID: fact.LinkSourceID, RadarLinkID: copyRadarClickID(radarLinkID), CustomerID: copyRadarClickID(customerID), Code: fact.Code, RawStage: fact.RawStage, SourceChannel: fact.SourceChannel, TargetTypeSnapshot: fact.TargetTypeSnapshot, SourceChannelSnapshot: fact.SourceChannelSnapshot, ErrorCode: fact.ErrorCode, CreatedAt: fact.CreatedAt.UTC().Truncate(time.Microsecond), OpenIDDigest: [sha256.Size]byte(fact.Sensitive.OpenID), UnionIDDigest: [sha256.Size]byte(fact.Sensitive.UnionID), ExternalUserIDDigest: [sha256.Size]byte(fact.Sensitive.ExternalUserID), CampaignIDDigest: [sha256.Size]byte(fact.Sensitive.CampaignID), StaffIDDigest: [sha256.Size]byte(fact.Sensitive.StaffID), UserAgentDigest: [sha256.Size]byte(fact.Sensitive.UserAgent), IPDigest: [sha256.Size]byte(fact.Sensitive.IP), PersonIDDigest: [sha256.Size]byte(fact.Sensitive.PersonID), IPHashDigest: [sha256.Size]byte(fact.Sensitive.IPHash), CampaignSnapshotDigest: [sha256.Size]byte(fact.Sensitive.CampaignSnapshot), StaffSnapshotDigest: [sha256.Size]byte(fact.Sensitive.StaffSnapshot), RefererDigest: [sha256.Size]byte(fact.Sensitive.Referer), QueryParamsDigest: [sha256.Size]byte(fact.Sensitive.QueryParams)}
}

func copyRadarClickID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func recordRadarClickHistoryQuarantine(ctx context.Context, journal RadarClickHistoryImportJournal, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := journal.LoadRadarClickHistoryTerminal(ctx, source)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.RecordRadarClickHistoryTerminal(ctx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func verifyRadarClickHistoryReceipt(ctx context.Context, journal RadarClickHistoryImportJournal, row v1archive.ArchivedRow, receipt radarport.RadarClickHistoryReceipt) error {
	if receipt.Kind != RadarClickHistoryKind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	stored, found, err := journal.LoadRadarClickHistory(ctx, RadarClickHistoryKind, receipt.SourceIdentifier)
	if err != nil || !found || stored.Kind != receipt.Kind || stored.SourceIdentifier != receipt.SourceIdentifier || stored.PayloadDigest != receipt.PayloadDigest || stored.TargetID != receipt.TargetID || stored.TargetDigest != receipt.TargetDigest {
		return ErrConflict
	}
	return nil
}

func radarClickHistoryReceipt(source string, terminal TerminalReceipt) (radarport.RadarClickHistoryReceipt, bool, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || idErr != nil || key == ([sha256.Size]byte{}) || terminal.SourceKeyDigest != key || terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 || id < 1 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return radarport.RadarClickHistoryReceipt{}, false, ErrConflict
	}
	return radarport.RadarClickHistoryReceipt{Kind: RadarClickHistoryKind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, true, nil
}

func radarClickHistoryTerminal(receipt radarport.RadarClickHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || receipt.Kind != RadarClickHistoryKind || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}
