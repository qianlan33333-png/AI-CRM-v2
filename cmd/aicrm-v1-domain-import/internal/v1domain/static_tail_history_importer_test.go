package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type staticTailImporterContextKey struct{}

func TestStaticTailHistoryImporterWrites54ReadOnlyFactsAndReplays(t *testing.T) {
	archive := staticTailImporterFixture(t)
	journal := newStaticTailImporterJournal()
	mediaWriter := &staticTailImporterMediaWriter{base: staticTailImporterWriterBase{journal: journal}}
	productWriter := &staticTailImporterProductWriter{base: staticTailImporterWriterBase{journal: journal}}
	cycleWriter := &staticTailImporterCycleWriter{base: staticTailImporterWriterBase{journal: journal}}
	importer, err := NewStaticTailHistoryImporter(archive, staticTailImporterUOW{}, mediaWriter, productWriter, cycleWriter, journal)
	if err != nil {
		t.Fatal(err)
	}
	first, err := importer.Import(context.Background(), "archive-run")
	if err != nil || first.SourceCount() != 54 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	for _, check := range []struct {
		table string
		count int
	}{
		{staticTailGroupInviteTable, 4}, {staticTailPageSliceTable, 46}, {staticTailStrategyTable, 1}, {staticTailVersionTable, 2}, {staticTailDocumentTable, 1},
	} {
		if got := first.Tables[check.table]; got.Imported != check.count || got.Quarantined != 0 || got.Replayed != 0 {
			t.Fatalf("table=%s result=%+v", check.table, got)
		}
	}
	if len(mediaWriter.values) != 4 || len(productWriter.values) != 46 || len(cycleWriter.strategies) != 1 || len(cycleWriter.versions) != 2 || len(cycleWriter.documents) != 1 {
		t.Fatalf("owner writes media=%d product=%d cycle=%d/%d/%d", len(mediaWriter.values), len(productWriter.values), len(cycleWriter.strategies), len(cycleWriter.versions), len(cycleWriter.documents))
	}
	if cycleWriter.versions[0].StrategyHistoryID != cycleWriter.strategyIDs[10] || cycleWriter.documents[0].VersionHistoryID != cycleWriter.versionIDs[102] {
		t.Fatalf("cycle source IDs substituted for historical parents: strategies=%v versions=%v documents=%#v", cycleWriter.strategyIDs, cycleWriter.versionIDs, cycleWriter.documents[0])
	}
	firstPage := productWriter.values[0]
	if firstPage.SourcePayloadDigest != archive.rows[staticTailPageSliceTable][0].PayloadHMAC || firstPage.SourceKeyDigest != archive.rows[staticTailPageSliceTable][0].SourceKeyHMAC {
		t.Fatalf("sealed archive HMAC binding lost: %#v row=%#v", firstPage, archive.rows[staticTailPageSliceTable][0])
	}
	if mediaWriter.values[0].RoomBaseSourceID != nil || cycleWriter.versions[1].EffectiveFrom != nil || cycleWriter.documents[0].CopyGuideGeneratedAt != nil {
		t.Fatalf("explicit source null changed during mapping: group=%#v version=%#v document=%#v", mediaWriter.values[0], cycleWriter.versions[1], cycleWriter.documents[0])
	}

	second, err := importer.Import(context.Background(), "archive-run")
	if err != nil || second.SourceCount() != 54 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	for _, check := range []struct {
		table string
		count int
	}{
		{staticTailGroupInviteTable, 4}, {staticTailPageSliceTable, 46}, {staticTailStrategyTable, 1}, {staticTailVersionTable, 2}, {staticTailDocumentTable, 1},
	} {
		if got := second.Tables[check.table]; got.Imported != check.count || got.Replayed != check.count || got.Quarantined != 0 {
			t.Fatalf("replay table=%s result=%+v", check.table, got)
		}
	}
}

func TestStaticTailHistoryImporterQuarantinesRedactionAndOwnerFailureWithoutCurrentWrites(t *testing.T) {
	archive := staticTailImporterFixture(t)
	archive.rows[staticTailGroupInviteTable][0].RedactedFields = []string{"name"}
	journal := newStaticTailImporterJournal()
	mediaWriter := &staticTailImporterMediaWriter{base: staticTailImporterWriterBase{journal: journal}}
	productWriter := &staticTailImporterProductWriter{base: staticTailImporterWriterBase{journal: journal}}
	cycleWriter := &staticTailImporterCycleWriter{base: staticTailImporterWriterBase{journal: journal}, invalid: staticTailCycleStrategyKind}
	importer, err := NewStaticTailHistoryImporter(archive, staticTailImporterUOW{}, mediaWriter, productWriter, cycleWriter, journal)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result.SourceCount() != 54 || result.Tables[staticTailGroupInviteTable].Quarantined != 1 || result.Tables[staticTailStrategyTable].Quarantined != 1 || result.Tables[staticTailVersionTable].Quarantined != 2 || result.Tables[staticTailDocumentTable].Quarantined != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(mediaWriter.values) != 3 || len(cycleWriter.strategies) != 0 || len(cycleWriter.versions) != 0 || len(cycleWriter.documents) != 0 {
		t.Fatalf("quarantined definition reached an owner: media=%d cycles=%d/%d/%d", len(mediaWriter.values), len(cycleWriter.strategies), len(cycleWriter.versions), len(cycleWriter.documents))
	}
	row := archive.rows[staticTailGroupInviteTable][0]
	terminal, found, err := journal.LoadTerminal(context.Background(), row.TableID, SourceIdentifier(row.SourceKeyHMAC))
	if err != nil || !found || terminal.Disposition != "quarantine" || terminal.Reason != "group_invite_library_shape_invalid" {
		t.Fatalf("redacted row terminal=%+v found=%t err=%v", terminal, found, err)
	}
}

func TestStaticTailHistoryImporterRejectsBadArchiveBindingAndRollsBack(t *testing.T) {
	archive := staticTailImporterFixture(t)
	archive.rows[staticTailPageSliceTable][0].PayloadHMAC = [sha256.Size]byte{}
	journal := newStaticTailImporterJournal()
	importer, err := NewStaticTailHistoryImporter(archive, staticTailImporterUOW{}, &staticTailImporterMediaWriter{base: staticTailImporterWriterBase{journal: journal}}, &staticTailImporterProductWriter{base: staticTailImporterWriterBase{journal: journal}}, &staticTailImporterCycleWriter{base: staticTailImporterWriterBase{journal: journal}}, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("bad payload HMAC err=%v", err)
	}

	archive = staticTailImporterFixture(t)
	journal = newStaticTailImporterJournal()
	mediaWriter := &staticTailImporterMediaWriter{base: staticTailImporterWriterBase{journal: journal}}
	productWriter := &staticTailImporterProductWriter{base: staticTailImporterWriterBase{journal: journal}}
	cycleWriter := &staticTailImporterCycleWriter{base: staticTailImporterWriterBase{journal: journal}}
	importer, err = NewStaticTailHistoryImporter(archive, staticTailImporterUOW{err: errors.New("rollback")}, mediaWriter, productWriter, cycleWriter, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); err == nil || len(mediaWriter.values) != 0 || len(productWriter.values) != 0 || len(cycleWriter.strategies) != 0 {
		t.Fatalf("uow failure wrote a target: err=%v media=%d product=%d strategy=%d", err, len(mediaWriter.values), len(productWriter.values), len(cycleWriter.strategies))
	}
}

type staticTailImporterArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive staticTailImporterArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type staticTailImporterUOW struct{ err error }

func (uow staticTailImporterUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	if uow.err != nil {
		return uow.err
	}
	return callback(context.WithValue(ctx, staticTailImporterContextKey{}, "transaction"))
}

type staticTailImporterJournal struct {
	terminals map[string]map[string]TerminalReceipt
}

func newStaticTailImporterJournal() *staticTailImporterJournal {
	return &staticTailImporterJournal{terminals: map[string]map[string]TerminalReceipt{}}
}

func (journal *staticTailImporterJournal) ValidateStaticTailHistoryImportScope(run string) error {
	if journal == nil || run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}

func (journal *staticTailImporterJournal) LoadTerminal(_ context.Context, table, source string) (TerminalReceipt, bool, error) {
	if journal == nil {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	value, found := journal.terminals[table][source]
	return value, found, nil
}

func (journal *staticTailImporterJournal) RecordTerminal(_ context.Context, table string, receipt TerminalReceipt) error {
	if journal == nil {
		return ErrInvalidScope
	}
	if journal.terminals[table] == nil {
		journal.terminals[table] = map[string]TerminalReceipt{}
	}
	source := SourceIdentifier(receipt.SourceKeyDigest)
	if found, exists := journal.terminals[table][source]; exists && !reflect.DeepEqual(found, receipt) {
		return ErrConflict
	}
	journal.terminals[table][source] = receipt
	return nil
}

type staticTailImporterWriterBase struct {
	journal *staticTailImporterJournal
	nextID  int64
}

func (writer *staticTailImporterWriterBase) write(ctx context.Context, table, kind, source string, payload [sha256.Size]byte) (staticTailWriteReceipt, error) {
	if ctx.Value(staticTailImporterContextKey{}) != "transaction" {
		return staticTailWriteReceipt{}, errors.New("writer was outside caller transaction")
	}
	if existing, found, err := writer.journal.LoadTerminal(ctx, table, source); err != nil {
		return staticTailWriteReceipt{}, err
	} else if found {
		if existing.Disposition != "import" || existing.PayloadDigest != payload || existing.TargetDigest == ([sha256.Size]byte{}) {
			return staticTailWriteReceipt{}, ErrConflict
		}
		id, err := positiveID(existing.TargetID)
		if err != nil {
			return staticTailWriteReceipt{}, err
		}
		return staticTailWriteReceipt{kind: kind, source: source, payload: payload, target: existing.TargetDigest, targetID: id, replayed: true}, nil
	}
	writer.nextID++
	target := sha256.Sum256([]byte(kind + "\\x00" + source))
	key, err := ParseSourceIdentifier(source)
	if err != nil {
		return staticTailWriteReceipt{}, err
	}
	if err = writer.journal.RecordTerminal(ctx, table, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: payload, Disposition: "import", TargetID: strconv.FormatInt(writer.nextID, 10), TargetDigest: target}); err != nil {
		return staticTailWriteReceipt{}, err
	}
	return staticTailWriteReceipt{kind: kind, source: source, payload: payload, target: target, targetID: writer.nextID}, nil
}

type staticTailImporterMediaWriter struct {
	base   staticTailImporterWriterBase
	values []mediaport.HistoricalGroupInvite
}

func (writer *staticTailImporterMediaWriter) ImportGroupInvite(ctx context.Context, source string, value mediaport.HistoricalGroupInvite) (mediaport.StaticMediaHistoryReceipt, error) {
	writer.values = append(writer.values, value)
	receipt, err := writer.base.write(ctx, staticTailGroupInviteTable, staticTailGroupInviteKind, source, value.SourcePayloadDigest)
	return mediaport.StaticMediaHistoryReceipt{Kind: receipt.kind, SourceIdentifier: receipt.source, PayloadDigest: receipt.payload, TargetDigest: receipt.target, TargetID: receipt.targetID, Replayed: receipt.replayed}, err
}

type staticTailImporterProductWriter struct {
	base   staticTailImporterWriterBase
	values []productport.HistoricalProductPageSlice
}

func (writer *staticTailImporterProductWriter) ImportProductPageSlice(ctx context.Context, source string, value productport.HistoricalProductPageSlice) (productport.StaticProductHistoryReceipt, error) {
	writer.values = append(writer.values, value)
	receipt, err := writer.base.write(ctx, staticTailPageSliceTable, staticTailPageSliceKind, source, value.SourcePayloadDigest)
	return productport.StaticProductHistoryReceipt{Kind: receipt.kind, SourceIdentifier: receipt.source, PayloadDigest: receipt.payload, TargetDigest: receipt.target, TargetID: receipt.targetID, Replayed: receipt.replayed}, err
}

type staticTailImporterCycleWriter struct {
	base        staticTailImporterWriterBase
	invalid     string
	strategies  []cycleport.HistoricalCycleStrategy
	versions    []cycleport.HistoricalCycleVersion
	documents   []cycleport.HistoricalCycleDocument
	strategyIDs map[int64]int64
	versionIDs  map[int64]int64
}

func (writer *staticTailImporterCycleWriter) ImportCycleStrategy(ctx context.Context, source string, value cycleport.HistoricalCycleStrategy) (cycleport.StaticCycleHistoryReceipt, error) {
	if writer.invalid == staticTailCycleStrategyKind {
		return cycleport.StaticCycleHistoryReceipt{}, cycleport.ErrStaticCycleHistoryInvalid
	}
	writer.strategies = append(writer.strategies, value)
	receipt, err := writer.base.write(ctx, staticTailStrategyTable, staticTailCycleStrategyKind, source, value.SourcePayloadDigest)
	if err == nil {
		if writer.strategyIDs == nil {
			writer.strategyIDs = map[int64]int64{}
		}
		writer.strategyIDs[value.SourceID] = receipt.targetID
	}
	return cycleport.StaticCycleHistoryReceipt{Kind: receipt.kind, SourceIdentifier: receipt.source, PayloadDigest: receipt.payload, TargetDigest: receipt.target, TargetID: receipt.targetID, Replayed: receipt.replayed}, err
}

func (writer *staticTailImporterCycleWriter) ImportCycleVersion(ctx context.Context, source string, value cycleport.HistoricalCycleVersion) (cycleport.StaticCycleHistoryReceipt, error) {
	if writer.invalid == staticTailCycleVersionKind {
		return cycleport.StaticCycleHistoryReceipt{}, cycleport.ErrStaticCycleHistoryInvalid
	}
	writer.versions = append(writer.versions, value)
	receipt, err := writer.base.write(ctx, staticTailVersionTable, staticTailCycleVersionKind, source, value.SourcePayloadDigest)
	if err == nil {
		if writer.versionIDs == nil {
			writer.versionIDs = map[int64]int64{}
		}
		writer.versionIDs[value.SourceID] = receipt.targetID
	}
	return cycleport.StaticCycleHistoryReceipt{Kind: receipt.kind, SourceIdentifier: receipt.source, PayloadDigest: receipt.payload, TargetDigest: receipt.target, TargetID: receipt.targetID, Replayed: receipt.replayed}, err
}

func (writer *staticTailImporterCycleWriter) ImportCycleDocument(ctx context.Context, source string, value cycleport.HistoricalCycleDocument) (cycleport.StaticCycleHistoryReceipt, error) {
	if writer.invalid == staticTailCycleDocumentKind {
		return cycleport.StaticCycleHistoryReceipt{}, cycleport.ErrStaticCycleHistoryInvalid
	}
	writer.documents = append(writer.documents, value)
	receipt, err := writer.base.write(ctx, staticTailDocumentTable, staticTailCycleDocumentKind, source, value.SourcePayloadDigest)
	return cycleport.StaticCycleHistoryReceipt{Kind: receipt.kind, SourceIdentifier: receipt.source, PayloadDigest: receipt.payload, TargetDigest: receipt.target, TargetID: receipt.targetID, Replayed: receipt.replayed}, err
}

func staticTailImporterFixture(t *testing.T) staticTailImporterArchive {
	t.Helper()
	at := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.UTC)
	rows := map[string][]v1archive.ArchivedRow{}
	for id := 1; id <= 4; id++ {
		rows[staticTailGroupInviteTable] = append(rows[staticTailGroupInviteTable], staticTailImporterRow(t, staticTailGroupInviteTable, int64(id), map[string]any{"id": id, "name": "", "title": "", "description": "", "pic_url": "signed-url", "join_url": "signed-url", "config_id": "config", "state": "archived", "chat_id_list": []string{}, "auto_create_room": false, "room_base_name": "", "room_base_id": nil, "enabled": false, "created_at": at, "updated_at": at, "chat_id": "", "binding_status": "unbound"}))
	}
	for index := 0; index < 46; index++ {
		rows[staticTailPageSliceTable] = append(rows[staticTailPageSliceTable], staticTailImporterRow(t, staticTailPageSliceTable, int64(index+1), map[string]any{"id": 800 + index, "product_id": 700, "image_library_id": 900 + index, "sort_order": index, "enabled": index != 45, "created_at": at, "updated_at": at}))
	}
	rows[staticTailStrategyTable] = append(rows[staticTailStrategyTable], staticTailImporterRow(t, staticTailStrategyTable, 1, map[string]any{"id": 10, "tenant_id": "signed", "strategy_key": "legacy", "title": "", "description": "", "cadence": "weekly", "timezone": "Asia/Shanghai", "status": "paused", "current_version": 2, "created_at": at, "updated_at": at}))
	for index, id := range []int{101, 102} {
		rows[staticTailVersionTable] = append(rows[staticTailVersionTable], staticTailImporterRow(t, staticTailVersionTable, int64(index+1), map[string]any{"id": id, "strategy_id": 10, "version": index + 1, "label": "", "objective": "", "definition_json": map[string]any{}, "version_hash": "hash", "effective_from": nil, "created_at": at, "governance_status": "unconfirmed", "confirmed_by": "", "confirmed_at": nil, "confirmation_note": "", "operation_skill_json": map[string]any{}, "operation_skill_hash": "skill"}))
	}
	rows[staticTailDocumentTable] = append(rows[staticTailDocumentTable], staticTailImporterRow(t, staticTailDocumentTable, 1, map[string]any{"id": 300, "strategy_version_id": 102, "schema_version": "v1", "execution_guide_markdown": "", "execution_guide_sha256": "execution", "execution_guide_generated_at": nil, "execution_guide_source": "", "copy_guide_markdown": "", "copy_guide_sha256": "copy", "copy_guide_generated_at": nil, "copy_guide_source": "", "measurement_guide_markdown": "", "measurement_guide_sha256": "measurement", "measurement_guide_generated_at": nil, "measurement_guide_source": "", "execution_contract_json": map[string]any{}, "document_pack_hash": "pack", "created_at": at}))
	return staticTailImporterArchive{rows: rows}
}

func staticTailImporterRow(t *testing.T, table string, ordinal int64, value any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(table + "\\x00" + strconv.FormatInt(ordinal, 10)))
	payloadDigest := sha256.Sum256(append([]byte("payload\\x00"), payload...))
	fieldDigest := sha256.Sum256([]byte("field\\x00" + table + "\\x00" + strconv.FormatInt(ordinal, 10)))
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: digest, PayloadHMAC: payloadDigest, FieldHMAC: fieldDigest, Payload: payload}
}
