package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1wecomcontacthistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	weComContactHistoryDomain        = "wecom-contact-history"
	weComContactHistoryImportVersion = "v1-wecom-contact-history-a1"

	weComContactEventLogKind   = "wecom_contact_event_log"
	weComContactFollowUserKind = "wecom_contact_follow_user"

	weComContactEventLogTargetTable   = "contact_v1_wecom_event_log_history"
	weComContactFollowUserTargetTable = "contact_v1_wecom_follow_user_history"

	weComContactEventLogRows   = 56880
	weComContactFollowUserRows = 50872
)

// WeComContactHistoryJournal keeps exactly the two immutable Contact-owned
// receipt streams. It does not expose a current customer, owner, callback, or
// Provider operation.
type WeComContactHistoryJournal struct {
	eventLogs, followUsers *v1domain.Journal
}

var _ contactport.WeComContactHistoryJournal = (*WeComContactHistoryJournal)(nil)

func NewWeComContactHistoryJournal(eventLogJournal, followUserJournal *v1domain.Journal) (*WeComContactHistoryJournal, error) {
	if eventLogJournal == nil || followUserJournal == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return &WeComContactHistoryJournal{eventLogs: eventLogJournal, followUsers: followUserJournal}, nil
}

func newWeComContactHistoryJournal(archiveRunID string) (*WeComContactHistoryJournal, error) {
	if archiveRunID == "" {
		return nil, v1domain.ErrInvalidScope
	}
	eventLogs, err := v1domain.NewJournal(v1domain.Scope{
		ImportVersion: weComContactHistoryImportVersion,
		ArchiveRunID:  archiveRunID,
		AdapterID:     v1archive.DefaultAdapterID,
		TableID:       v1wecomcontacthistory.ExternalContactEventLogsTableID,
		TargetDomain:  "contact",
		TargetTable:   weComContactEventLogTargetTable,
	})
	if err != nil {
		return nil, err
	}
	followUsers, err := v1domain.NewJournal(v1domain.Scope{
		ImportVersion: weComContactHistoryImportVersion,
		ArchiveRunID:  archiveRunID,
		AdapterID:     v1archive.DefaultAdapterID,
		TableID:       v1wecomcontacthistory.ExternalContactFollowUsersTableID,
		TargetDomain:  "contact",
		TargetTable:   weComContactFollowUserTargetTable,
	})
	if err != nil {
		return nil, err
	}
	return NewWeComContactHistoryJournal(eventLogs, followUsers)
}

func (journal *WeComContactHistoryJournal) LoadWeComContactHistory(ctx context.Context, kind, sourceIdentifier string) (contactport.WeComContactHistoryReceipt, bool, error) {
	selected, err := journal.selectJournal(kind)
	if err != nil || ctx == nil {
		return contactport.WeComContactHistoryReceipt{}, false, v1domain.ErrInvalidScope
	}
	terminal, found, err := selected.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return contactport.WeComContactHistoryReceipt{}, found, err
	}
	receipt, err := weComContactHistoryReceipt(kind, sourceIdentifier, terminal)
	return receipt, err == nil, err
}

func (journal *WeComContactHistoryJournal) RecordWeComContactHistory(ctx context.Context, receipt contactport.WeComContactHistoryReceipt) error {
	selected, err := journal.selectJournal(receipt.Kind)
	if err != nil || ctx == nil {
		return v1domain.ErrInvalidScope
	}
	terminal, err := weComContactHistoryTerminal(receipt)
	if err != nil {
		return err
	}
	return selected.Record(ctx, terminal)
}

func (journal *WeComContactHistoryJournal) loadTerminal(ctx context.Context, kind, sourceIdentifier string) (v1domain.TerminalReceipt, bool, error) {
	selected, err := journal.selectJournal(kind)
	if err != nil || ctx == nil {
		return v1domain.TerminalReceipt{}, false, v1domain.ErrInvalidScope
	}
	return selected.LoadTerminal(ctx, sourceIdentifier)
}

func (journal *WeComContactHistoryJournal) recordTerminal(ctx context.Context, kind string, receipt v1domain.TerminalReceipt) error {
	selected, err := journal.selectJournal(kind)
	if err != nil || ctx == nil {
		return v1domain.ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *WeComContactHistoryJournal) selectJournal(kind string) (*v1domain.Journal, error) {
	if journal == nil {
		return nil, v1domain.ErrInvalidScope
	}
	switch kind {
	case weComContactEventLogKind:
		if journal.eventLogs == nil {
			return nil, v1domain.ErrInvalidScope
		}
		return journal.eventLogs, nil
	case weComContactFollowUserKind:
		if journal.followUsers == nil {
			return nil, v1domain.ErrInvalidScope
		}
		return journal.followUsers, nil
	default:
		return nil, v1domain.ErrInvalidScope
	}
}

func weComContactHistoryReceipt(kind, sourceIdentifier string, terminal v1domain.TerminalReceipt) (contactport.WeComContactHistoryReceipt, error) {
	sourceKey, err := v1domain.ParseSourceIdentifier(sourceIdentifier)
	targetID, targetErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || targetErr != nil || targetID < 1 || strconv.FormatInt(targetID, 10) != terminal.TargetID ||
		(kind != weComContactEventLogKind && kind != weComContactFollowUserKind) ||
		sourceKey == ([sha256.Size]byte{}) || terminal.SourceKeyDigest != sourceKey || terminal.PayloadDigest == ([sha256.Size]byte{}) ||
		terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetDigest == ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 {
		return contactport.WeComContactHistoryReceipt{}, v1domain.ErrConflict
	}
	return contactport.WeComContactHistoryReceipt{Kind: kind, SourceIdentifier: sourceIdentifier, PayloadDigest: terminal.PayloadDigest, TargetID: targetID, TargetDigest: terminal.TargetDigest}, nil
}

func weComContactHistoryTerminal(receipt contactport.WeComContactHistoryReceipt) (v1domain.TerminalReceipt, error) {
	sourceKey, err := v1domain.ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || receipt.Kind == "" || (receipt.Kind != weComContactEventLogKind && receipt.Kind != weComContactFollowUserKind) ||
		sourceKey == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 ||
		receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed {
		return v1domain.TerminalReceipt{}, v1domain.ErrInvalidScope
	}
	return v1domain.TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

type weComContactHistoryWriter interface {
	ImportHistoricalWeComExternalContactEventLog(context.Context, string, contactport.HistoricalWeComExternalContactEventLog) (contactport.WeComContactHistoryReceipt, error)
	ImportHistoricalWeComExternalContactFollowUser(context.Context, string, contactport.HistoricalWeComExternalContactFollowUser) (contactport.WeComContactHistoryReceipt, error)
}

type weComContactHistoryImportJournal interface {
	contactport.WeComContactHistoryJournal
	loadTerminal(context.Context, string, string) (v1domain.TerminalReceipt, bool, error)
	recordTerminal(context.Context, string, v1domain.TerminalReceipt) error
}

type WeComContactHistoryImportResult struct {
	ImportedEventLogs, ImportedFollowUsers       int
	QuarantinedEventLogs, QuarantinedFollowUsers int
	Replayed                                     int
}

func (result WeComContactHistoryImportResult) terminalCount() int {
	return result.ImportedEventLogs + result.ImportedFollowUsers + result.QuarantinedEventLogs + result.QuarantinedFollowUsers
}

// WeComContactHistoryImporter writes only immutable Contact-owned projections.
// It has no resolver: historical WeCom identifiers never activate a customer
// or select an owner.
type WeComContactHistoryImporter struct {
	archive v1domain.ArchiveSource
	uow     v1domain.UnitOfWork
	writer  weComContactHistoryWriter
	journal weComContactHistoryImportJournal

	expectedEventLogRows   int
	expectedFollowUserRows int
}

func NewWeComContactHistoryImporter(archiveReader v1domain.ArchiveSource, uow v1domain.UnitOfWork, writer *contactapp.WeComContactHistoryWriter, journal *WeComContactHistoryJournal) (*WeComContactHistoryImporter, error) {
	return newWeComContactHistoryImporter(archiveReader, uow, writer, journal)
}

func newWeComContactHistoryImporter(archiveReader v1domain.ArchiveSource, uow v1domain.UnitOfWork, writer weComContactHistoryWriter, journal weComContactHistoryImportJournal) (*WeComContactHistoryImporter, error) {
	return newWeComContactHistoryImporterWithExpectedRows(archiveReader, uow, writer, journal, weComContactEventLogRows, weComContactFollowUserRows)
}

// newWeComContactHistoryImporterWithExpectedRows keeps the immutable
// production counts in the public constructor while allowing the private
// importer tests to exercise both source tables with compact fixtures.
func newWeComContactHistoryImporterWithExpectedRows(archiveReader v1domain.ArchiveSource, uow v1domain.UnitOfWork, writer weComContactHistoryWriter, journal weComContactHistoryImportJournal, expectedEventLogRows, expectedFollowUserRows int) (*WeComContactHistoryImporter, error) {
	if archiveReader == nil || uow == nil || writer == nil || journal == nil {
		return nil, v1domain.ErrInvalidScope
	}
	if expectedEventLogRows < 0 || expectedFollowUserRows < 0 {
		return nil, v1domain.ErrInvalidScope
	}
	return &WeComContactHistoryImporter{
		archive: archiveReader, uow: uow, writer: writer, journal: journal,
		expectedEventLogRows: expectedEventLogRows, expectedFollowUserRows: expectedFollowUserRows,
	}, nil
}

func (importer *WeComContactHistoryImporter) Import(ctx context.Context, archiveRunID string) (WeComContactHistoryImportResult, error) {
	if importer == nil || ctx == nil || archiveRunID == "" || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil {
		return WeComContactHistoryImportResult{}, v1domain.ErrInvalidScope
	}
	result := WeComContactHistoryImportResult{}
	if err := importer.importEventLogs(ctx, archiveRunID, &result); err != nil {
		return WeComContactHistoryImportResult{}, err
	}
	if err := importer.importFollowUsers(ctx, archiveRunID, &result); err != nil {
		return WeComContactHistoryImportResult{}, err
	}
	if result.terminalCount() != importer.expectedEventLogRows+importer.expectedFollowUserRows {
		return WeComContactHistoryImportResult{}, v1domain.ErrConflict
	}
	return result, nil
}

func (importer *WeComContactHistoryImporter) importEventLogs(ctx context.Context, archiveRunID string, result *WeComContactHistoryImportResult) error {
	rows, err := importer.readRows(ctx, archiveRunID, v1wecomcontacthistory.ExternalContactEventLogsTableID, importer.expectedEventLogRows)
	if err != nil {
		return err
	}
	decisions := v1wecomcontacthistory.AdaptHistory(rows, nil).EventLogs
	for index, decision := range decisions {
		if err = importer.importEventLog(ctx, rows[index], decision, result); err != nil {
			return err
		}
	}
	return nil
}

func (importer *WeComContactHistoryImporter) importFollowUsers(ctx context.Context, archiveRunID string, result *WeComContactHistoryImportResult) error {
	rows, err := importer.readRows(ctx, archiveRunID, v1wecomcontacthistory.ExternalContactFollowUsersTableID, importer.expectedFollowUserRows)
	if err != nil {
		return err
	}
	decisions := v1wecomcontacthistory.AdaptHistory(nil, rows).FollowUsers
	for index, decision := range decisions {
		if err = importer.importFollowUser(ctx, rows[index], decision, result); err != nil {
			return err
		}
	}
	return nil
}

func (importer *WeComContactHistoryImporter) readRows(ctx context.Context, archiveRunID, tableID string, expected int) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0, expected)
	seen := make(map[[sha256.Size]byte]struct{}, expected)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal != int64(len(rows)+1) ||
			row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
			return v1domain.ErrConflict
		}
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return v1domain.ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, row)
		return nil
	})
	if err != nil || len(rows) != expected {
		if err != nil {
			return nil, err
		}
		return nil, v1domain.ErrConflict
	}
	return rows, nil
}

func (importer *WeComContactHistoryImporter) importEventLog(ctx context.Context, row v1archive.ArchivedRow, decision v1wecomcontacthistory.Result[v1wecomcontacthistory.ExternalContactEventLogFact], result *WeComContactHistoryImportResult) error {
	replayed, imported := false, false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, imported = false, false
		if decision.Disposition != v1wecomcontacthistory.DispositionCandidate || decision.Fact == nil {
			var err error
			replayed, err = importer.recordQuarantine(tx, weComContactEventLogKind, row, fixedWeComContactHistoryReason(decision.Reason, "invalid_wecom_contact_event_log"))
			return err
		}
		value, err := weComContactEventLogValue(row, *decision.Fact)
		if err != nil {
			return err
		}
		receipt, err := importer.writer.ImportHistoricalWeComExternalContactEventLog(tx, v1domain.SourceIdentifier(row.SourceKeyHMAC), value)
		if errors.Is(err, contactport.ErrWeComContactHistoryInvalid) {
			replayed, err = importer.recordQuarantine(tx, weComContactEventLogKind, row, "wecom_contact_event_log_target_invalid")
			return err
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, weComContactEventLogKind, row, receipt); err != nil {
			return err
		}
		replayed, imported = receipt.Replayed, true
		return nil
	}); err != nil {
		return err
	}
	if imported {
		result.ImportedEventLogs++
	} else {
		result.QuarantinedEventLogs++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func (importer *WeComContactHistoryImporter) importFollowUser(ctx context.Context, row v1archive.ArchivedRow, decision v1wecomcontacthistory.Result[v1wecomcontacthistory.ExternalContactFollowUserFact], result *WeComContactHistoryImportResult) error {
	replayed, imported := false, false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, imported = false, false
		if decision.Disposition != v1wecomcontacthistory.DispositionCandidate || decision.Fact == nil {
			var err error
			replayed, err = importer.recordQuarantine(tx, weComContactFollowUserKind, row, fixedWeComContactHistoryReason(decision.Reason, "invalid_wecom_contact_follow_user"))
			return err
		}
		value, err := weComContactFollowUserValue(row, *decision.Fact)
		if err != nil {
			return err
		}
		receipt, err := importer.writer.ImportHistoricalWeComExternalContactFollowUser(tx, v1domain.SourceIdentifier(row.SourceKeyHMAC), value)
		if errors.Is(err, contactport.ErrWeComContactHistoryInvalid) {
			replayed, err = importer.recordQuarantine(tx, weComContactFollowUserKind, row, "wecom_contact_follow_user_target_invalid")
			return err
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, weComContactFollowUserKind, row, receipt); err != nil {
			return err
		}
		replayed, imported = receipt.Replayed, true
		return nil
	}); err != nil {
		return err
	}
	if imported {
		result.ImportedFollowUsers++
	} else {
		result.QuarantinedFollowUsers++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func (importer *WeComContactHistoryImporter) recordQuarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, v1domain.ErrInvalidScope
	}
	sourceIdentifier := v1domain.SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := importer.journal.loadTerminal(ctx, kind, sourceIdentifier)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" ||
			existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, v1domain.ErrConflict
		}
		return true, nil
	}
	return false, importer.journal.recordTerminal(ctx, kind, v1domain.TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func (importer *WeComContactHistoryImporter) verifyReceipt(ctx context.Context, kind string, row v1archive.ArchivedRow, receipt contactport.WeComContactHistoryReceipt) error {
	if receipt.Kind != kind || receipt.SourceIdentifier != v1domain.SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return v1domain.ErrConflict
	}
	stored, found, err := importer.journal.LoadWeComContactHistory(ctx, kind, receipt.SourceIdentifier)
	if err != nil || !found || stored.Kind != kind || stored.SourceIdentifier != receipt.SourceIdentifier || stored.PayloadDigest != receipt.PayloadDigest ||
		stored.TargetID != receipt.TargetID || stored.TargetDigest != receipt.TargetDigest {
		return v1domain.ErrConflict
	}
	return nil
}

func weComContactEventLogValue(row v1archive.ArchivedRow, source v1wecomcontacthistory.ExternalContactEventLogFact) (contactport.HistoricalWeComExternalContactEventLog, error) {
	if !sameWeComContactEnvelope(row, source.Source) {
		return contactport.HistoricalWeComExternalContactEventLog{}, v1domain.ErrConflict
	}
	return contactport.HistoricalWeComExternalContactEventLog{
		SourceKeyDigest: [sha256.Size]byte(source.Source.SourceKeyDigest), SourcePayloadDigest: [sha256.Size]byte(source.Source.PayloadDigest), SourceFieldDigest: [sha256.Size]byte(source.Source.FieldDigest),
		SourceID: source.SourceID, CorpIDDigest: [sha256.Size]byte(source.CorpIDDigest), EventType: source.EventType, ChangeType: source.ChangeType,
		ExternalUserIDDigest: [sha256.Size]byte(source.ExternalUserIDDigest), UserIDDigest: [sha256.Size]byte(source.UserIDDigest), EventTime: copyWeComContactInt64(source.EventTime),
		EventKeyDigest: [sha256.Size]byte(source.EventKeyDigest), PayloadXMLDigest: [sha256.Size]byte(source.PayloadXMLDigest), PayloadJSONDigest: [sha256.Size]byte(source.PayloadJSONDigest),
		ProcessStatus: source.ProcessStatus, RetryCount: source.RetryCount, ErrorMessageDigest: [sha256.Size]byte(source.ErrorMessageDigest),
		CreatedAt: weComContactTime(source.CreatedAt), UpdatedAt: weComContactTime(source.UpdatedAt), IdentitySyncStatus: source.IdentitySyncStatus,
		IdentitySyncErrorCodeDigest: [sha256.Size]byte(source.IdentitySyncErrorCodeDigest), IdentitySyncErrorMessageDigest: [sha256.Size]byte(source.IdentitySyncErrorMessageDigest), IdentitySyncResponseDigest: [sha256.Size]byte(source.IdentitySyncResponseDigest),
	}, nil
}

func weComContactFollowUserValue(row v1archive.ArchivedRow, source v1wecomcontacthistory.ExternalContactFollowUserFact) (contactport.HistoricalWeComExternalContactFollowUser, error) {
	if !sameWeComContactEnvelope(row, source.Source) {
		return contactport.HistoricalWeComExternalContactFollowUser{}, v1domain.ErrConflict
	}
	return contactport.HistoricalWeComExternalContactFollowUser{
		SourceKeyDigest: [sha256.Size]byte(source.Source.SourceKeyDigest), SourcePayloadDigest: [sha256.Size]byte(source.Source.PayloadDigest), SourceFieldDigest: [sha256.Size]byte(source.Source.FieldDigest),
		SourceID: source.SourceID, CorpIDDigest: [sha256.Size]byte(source.CorpIDDigest), ExternalUserIDDigest: [sha256.Size]byte(source.ExternalUserIDDigest), UserIDDigest: [sha256.Size]byte(source.UserIDDigest),
		RelationStatus: source.RelationStatus, IsPrimary: source.IsPrimary, RemarkDigest: [sha256.Size]byte(source.RemarkDigest), DescriptionDigest: [sha256.Size]byte(source.DescriptionDigest),
		AddWay: copyWeComContactInt32(source.AddWay), State: source.State, OperUserIDDigest: [sha256.Size]byte(source.OperUserIDDigest), CreateTime: copyWeComContactInt64(source.CreateTime), RawFollowUserDigest: [sha256.Size]byte(source.RawFollowUserDigest),
		FirstSeenAt: weComContactTime(source.FirstSeenAt), LastSeenAt: weComContactTime(source.LastSeenAt), CreatedAt: weComContactTime(source.CreatedAt), UpdatedAt: weComContactTime(source.UpdatedAt),
	}, nil
}

func sameWeComContactEnvelope(row v1archive.ArchivedRow, source v1wecomcontacthistory.SourceEnvelope) bool {
	return [sha256.Size]byte(source.SourceKeyDigest) == row.SourceKeyHMAC && [sha256.Size]byte(source.PayloadDigest) == row.PayloadHMAC && [sha256.Size]byte(source.FieldDigest) == row.FieldHMAC
}

func fixedWeComContactHistoryReason(reason, fallback string) string {
	if reason == "" {
		return fallback
	}
	return reason
}

func copyWeComContactInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyWeComContactInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func weComContactTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

// newWeComContactHistoryReader is intentionally caller-transaction bound so
// reconciliation sees the same target rows as the import receipt.
func newWeComContactHistoryReader(tx pgx.Tx) contactport.WeComContactHistoryReader {
	return contactstore.NewWeComContactHistoryReader(tx)
}
