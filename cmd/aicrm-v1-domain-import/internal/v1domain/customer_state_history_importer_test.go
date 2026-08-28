package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type customerStateArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (source customerStateArchiveFake) EachTableRow(ctx context.Context, run, table string, each func(v1archive.ArchivedRow) error) error {
	if ctx == nil || run != "archive-run" {
		return ErrInvalidScope
	}
	for _, row := range source.rows[table] {
		if err := each(row); err != nil {
			return err
		}
	}
	return nil
}

type customerStateUOWFake struct{}
type customerStateTxKey struct{}

func (customerStateUOWFake) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, customerStateTxKey{}, true))
}

type customerStateJournalFake struct{ terminals map[string]TerminalReceipt }

func newCustomerStateJournalFake() *customerStateJournalFake {
	return &customerStateJournalFake{terminals: map[string]TerminalReceipt{}}
}
func customerStateJournalKey(kind, source string) string { return kind + "/" + source }
func (journal *customerStateJournalFake) ValidateCustomerStateHistoryImportScope(run string) error {
	if journal == nil || run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}
func (journal *customerStateJournalFake) LoadCustomerStateHistoryTerminal(_ context.Context, kind, source string) (TerminalReceipt, bool, error) {
	value, found := journal.terminals[customerStateJournalKey(kind, source)]
	return value, found, nil
}
func (journal *customerStateJournalFake) RecordCustomerStateHistoryTerminal(_ context.Context, kind string, receipt TerminalReceipt) error {
	key := customerStateJournalKey(kind, SourceIdentifier(receipt.SourceKeyDigest))
	if existing, found := journal.terminals[key]; found && (existing.SourceKeyDigest != receipt.SourceKeyDigest || existing.PayloadDigest != receipt.PayloadDigest || existing.Disposition != receipt.Disposition || existing.Reason != receipt.Reason || existing.TargetID != receipt.TargetID || existing.TargetDigest != receipt.TargetDigest || len(existing.Metadata) != len(receipt.Metadata)) {
		return ErrConflict
	}
	journal.terminals[key] = receipt
	return nil
}
func (journal *customerStateJournalFake) LoadCustomerStateHistory(ctx context.Context, kind, source string) (contactport.CustomerStateHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadCustomerStateHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return contactport.CustomerStateHistoryReceipt{}, found, err
	}
	if terminal.Disposition != "import" || terminal.TargetID == "" {
		return contactport.CustomerStateHistoryReceipt{}, false, ErrConflict
	}
	id, err := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil {
		return contactport.CustomerStateHistoryReceipt{}, false, err
	}
	return contactport.CustomerStateHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, true, nil
}
func (journal *customerStateJournalFake) RecordCustomerStateHistory(ctx context.Context, receipt contactport.CustomerStateHistoryReceipt) error {
	if ctx.Value(customerStateTxKey{}) != true || receipt.Replayed || receipt.TargetID < 1 {
		return ErrConflict
	}
	return journal.RecordCustomerStateHistoryTerminal(ctx, receipt.Kind, TerminalReceipt{SourceKeyDigest: mustCustomerStateSourceKey(receipt.SourceIdentifier), PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest})
}
func mustCustomerStateSourceKey(value string) [sha256.Size]byte {
	key, _ := ParseSourceIdentifier(value)
	return key
}

type customerStateWriterFake struct {
	journal                   *customerStateJournalFake
	next                      int64
	writes                    map[string]int
	snapshot                  contactport.HistoricalCustomerStatusSnapshot
	change                    contactport.HistoricalCustomerStatusChange
	term                      contactport.HistoricalClassTermTagMapping
	returnInvalid, badReceipt bool
}

func newCustomerStateWriterFake(journal *customerStateJournalFake) *customerStateWriterFake {
	return &customerStateWriterFake{journal: journal, next: 100, writes: map[string]int{}}
}
func (writer *customerStateWriterFake) ImportCustomerStatusSnapshot(ctx context.Context, source string, value contactport.HistoricalCustomerStatusSnapshot) (contactport.CustomerStateHistoryReceipt, error) {
	writer.snapshot = value
	return writer.write(ctx, customerStateHistorySnapshotKind, source, value.SourcePayloadDigest)
}
func (writer *customerStateWriterFake) ImportCustomerStatusChange(ctx context.Context, source string, value contactport.HistoricalCustomerStatusChange) (contactport.CustomerStateHistoryReceipt, error) {
	writer.change = value
	return writer.write(ctx, customerStateHistoryChangeKind, source, value.SourcePayloadDigest)
}
func (writer *customerStateWriterFake) ImportClassTermTagMapping(ctx context.Context, source string, value contactport.HistoricalClassTermTagMapping) (contactport.CustomerStateHistoryReceipt, error) {
	writer.term = value
	return writer.write(ctx, customerStateHistoryTermKind, source, value.SourcePayloadDigest)
}
func (writer *customerStateWriterFake) write(ctx context.Context, kind, source string, payload [sha256.Size]byte) (contactport.CustomerStateHistoryReceipt, error) {
	if ctx.Value(customerStateTxKey{}) != true {
		return contactport.CustomerStateHistoryReceipt{}, errors.New("missing caller transaction")
	}
	if writer.returnInvalid {
		return contactport.CustomerStateHistoryReceipt{}, contactport.ErrCustomerStateHistoryInvalid
	}
	if existing, found, err := writer.journal.LoadCustomerStateHistory(ctx, kind, source); err != nil || found {
		if err != nil {
			return contactport.CustomerStateHistoryReceipt{}, err
		}
		existing.Replayed = true
		return existing, nil
	}
	writer.next++
	writer.writes[kind]++
	digest := sha256.Sum256([]byte(kind + "/" + source))
	receipt := contactport.CustomerStateHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: writer.next, TargetDigest: digest}
	if err := writer.journal.RecordCustomerStateHistory(ctx, receipt); err != nil {
		return contactport.CustomerStateHistoryReceipt{}, err
	}
	if writer.badReceipt {
		receipt.TargetID++
	}
	return receipt, nil
}

func TestCustomerStateHistoryImporterImportsThreeFactsAndReplays(t *testing.T) {
	archive := customerStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		customerStateHistorySnapshotTable: {customerStateRow(customerStateHistorySnapshotTable, 1, 1, customerStateSnapshotPayload())},
		customerStateHistoryChangeTable:   {customerStateRow(customerStateHistoryChangeTable, 1, 2, customerStateChangePayload())},
		customerStateHistoryTermTable:     {customerStateRow(customerStateHistoryTermTable, 1, 3, customerStateTermPayload())},
	}}
	journal := newCustomerStateJournalFake()
	writer := newCustomerStateWriterFake(journal)
	importer, err := NewCustomerStateHistoryImporter(archive, customerStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	got, err := importer.Import(context.Background(), "archive-run")
	if err != nil || got != (CustomerStateHistoryImportResult{ImportedSnapshots: 1, ImportedChanges: 1, ImportedTermMappings: 1}) {
		t.Fatalf("first import=%+v err=%v", got, err)
	}
	if writer.writes[customerStateHistorySnapshotKind] != 1 || writer.writes[customerStateHistoryChangeKind] != 1 || writer.writes[customerStateHistoryTermKind] != 1 {
		t.Fatalf("writes=%v", writer.writes)
	}
	if writer.snapshot.CustomerNameSnapshot != "customer" || writer.snapshot.OwnerUserIDSnapshot != "owner" || writer.snapshot.SourceFieldDigest == ([sha256.Size]byte{}) ||
		writer.change.SourceID != -1 || writer.change.UnionID != "union" || writer.term.SourceID != 0 || writer.term.ClassTermNo != -1 || writer.term.OriginalActive || writer.term.StrategySourceID != "strategy" {
		t.Fatalf("private fact fidelity lost: snapshot=%+v change=%+v term=%+v", writer.snapshot, writer.change, writer.term)
	}
	replayed, err := importer.Import(context.Background(), "archive-run")
	if err != nil || replayed != (CustomerStateHistoryImportResult{ImportedSnapshots: 1, ImportedChanges: 1, ImportedTermMappings: 1, Replayed: 3}) {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	if writer.writes[customerStateHistorySnapshotKind] != 1 || writer.writes[customerStateHistoryChangeKind] != 1 || writer.writes[customerStateHistoryTermKind] != 1 {
		t.Fatalf("replay wrote=%v", writer.writes)
	}
}

func TestCustomerStateHistoryImporterQuarantinesOwnerInvalidAndRejectsReceiptDrift(t *testing.T) {
	row := customerStateRow(customerStateHistorySnapshotTable, 1, 1, customerStateSnapshotPayload())
	journal := newCustomerStateJournalFake()
	writer := newCustomerStateWriterFake(journal)
	writer.returnInvalid = true
	importer, err := NewCustomerStateHistoryImporter(customerStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{customerStateHistorySnapshotTable: {row}}}, customerStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	got, err := importer.Import(context.Background(), "archive-run")
	if err != nil || got.Quarantined != 1 || got.ImportedSnapshots != 0 {
		t.Fatalf("owner invalid result=%+v err=%v", got, err)
	}

	journal = newCustomerStateJournalFake()
	writer = newCustomerStateWriterFake(journal)
	writer.badReceipt = true
	importer, err = NewCustomerStateHistoryImporter(customerStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{customerStateHistorySnapshotTable: {row}}}, customerStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("receipt drift err=%v", err)
	}
}

func TestCustomerStateHistoryImporterQuarantinesRedactedCandidateWithoutWrite(t *testing.T) {
	row := customerStateRow(customerStateHistorySnapshotTable, 1, 1, customerStateSnapshotPayload())
	row.RedactedFields = []string{"unionid"}
	journal := newCustomerStateJournalFake()
	writer := newCustomerStateWriterFake(journal)
	importer, err := NewCustomerStateHistoryImporter(customerStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{customerStateHistorySnapshotTable: {row}}}, customerStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	got, err := importer.Import(context.Background(), "archive-run")
	if err != nil || got.Quarantined != 1 || got.ImportedSnapshots != 0 || len(writer.writes) != 0 {
		t.Fatalf("result=%+v writes=%v err=%v", got, writer.writes, err)
	}
	terminal, found, err := journal.LoadCustomerStateHistoryTerminal(context.Background(), customerStateHistorySnapshotKind, SourceIdentifier(row.SourceKeyHMAC))
	if err != nil || !found || terminal.Disposition != "quarantine" || terminal.Reason != "customer_state_history_retained_field_redacted" {
		t.Fatalf("terminal=%+v found=%t err=%v", terminal, found, err)
	}
}

func TestCustomerStateHistoryImporterRejectsUnsignedOrOutOfOrderArchive(t *testing.T) {
	for name, change := range map[string]func(*v1archive.ArchivedRow){
		"field_digest": func(row *v1archive.ArchivedRow) { row.FieldHMAC = [sha256.Size]byte{} },
		"ordinal":      func(row *v1archive.ArchivedRow) { row.SourceOrdinal = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			row := customerStateRow(customerStateHistorySnapshotTable, 1, 1, customerStateSnapshotPayload())
			change(&row)
			journal := newCustomerStateJournalFake()
			writer := newCustomerStateWriterFake(journal)
			importer, err := NewCustomerStateHistoryImporter(customerStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{customerStateHistorySnapshotTable: {row}}}, customerStateUOWFake{}, writer, journal)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || len(writer.writes) != 0 {
				t.Fatalf("err=%v writes=%v", err, writer.writes)
			}
		})
	}
}

func customerStateRow(table string, ordinal, seed int64, payload []byte) v1archive.ArchivedRow {
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/key/%d", table, seed))), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/field/%d", table, seed))), Payload: payload}
}
func customerStateSnapshotPayload() []byte {
	return customerStatePayload(map[string]any{"signup_status": "", "signup_label_name": "label", "customer_name_snapshot": "customer", "owner_userid_snapshot": "owner", "set_by_userid": "actor", "set_at": customerStateTime(), "wecom_tag_sync_status": "", "wecom_tag_sync_error": "", "status_flags_json": map[string]any{"flag": false}, "created_at": customerStateTime(), "updated_at": customerStateTime(), "unionid": "union"})
}
func customerStateChangePayload() []byte {
	return customerStatePayload(map[string]any{"id": int64(-1), "old_signup_status": "old", "new_signup_status": "new", "old_label_name": "", "new_label_name": "label", "customer_name_snapshot": "customer", "owner_userid_snapshot": "owner", "set_by_userid": "actor", "set_at": customerStateTime(), "wecom_tag_sync_status": "", "wecom_tag_sync_error": "", "status_flags_json": nil, "created_at": customerStateTime(), "unionid": "union"})
}
func customerStateTermPayload() []byte {
	return customerStatePayload(map[string]any{"id": int64(0), "tag_group_name": "group", "tag_name": "tag", "class_term_no": int32(-1), "class_term_label": "", "is_active": false, "created_at": customerStateTime(), "updated_at": customerStateTime(), "strategy_id": "strategy", "group_id": "group", "tag_id": "tag"})
}
func customerStatePayload(value map[string]any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}
func customerStateTime() string {
	return time.Date(2026, 8, 28, 1, 2, 3, 456000000, time.UTC).Format(time.RFC3339Nano)
}
