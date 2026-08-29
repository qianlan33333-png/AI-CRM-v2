package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	timeline "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1customertimelinehistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var customerTimelineHistoryTestKey = bytes.Repeat([]byte{0x71}, sha256.Size)

func TestCustomerTimelineHistoryImportReplayAndReconcile(t *testing.T) {
	state := newCustomerTimelineHistoryState()
	ready := &customerTimelineHistoryReady{}
	redactedPayload := customerTimelineHistoryPayload(4, "[REDACTED]")
	archive := &customerTimelineHistoryArchive{rows: []v1archive.ArchivedRow{
		customerTimelineHistoryRow(t, 9, 1, "verified"),
		customerTimelineHistoryRow(t, -3, 2, "unresolved"),
		customerTimelineHistoryRedactedRow(t, 4, 3, redactedPayload, "unionid"),
	}}
	resolver := customerTimelineHistoryResolver{"verified": customerTimelineHistoryInt64Pointer(17)}
	importer := customerTimelineHistoryImporter(t, ready, archive, state, resolver)

	first, err := importer.Import(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if first != (CustomerTimelineHistoryImportResult{Imported: 2, Quarantined: 1}) || ready.calls != 1 || state.uow.commits != 2 || len(state.targets) != 2 || len(state.terminals) != 3 {
		t.Fatalf("first import=%#v ready=%d state=%#v", first, ready.calls, state)
	}
	if got := state.targets[1].CustomerID; got == nil || *got != 17 {
		t.Fatalf("verified unionid did not retain the verified customer: %#v", got)
	}
	if got := state.targets[2].CustomerID; got != nil {
		t.Fatalf("unresolved unionid guessed customer: %#v", got)
	}

	replay, err := importer.Import(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if replay != (CustomerTimelineHistoryImportResult{Replayed: 3}) || len(state.targets) != 2 || len(state.terminals) != 3 {
		t.Fatalf("replay=%#v targets=%d terminal=%d", replay, len(state.targets), len(state.terminals))
	}

	reconciled, err := importer.Reconcile(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.SelectedSourceCount != 3 || reconciled.ReceiptCount != 3 || reconciled.ImportedCount != 2 || reconciled.QuarantinedCount != 1 || reconciled.VerifiedCount != 3 || reconciled.Replayed || reconciled.ComparisonDigest == ([sha256.Size]byte{}) || state.reconciliation == nil || state.sealRecords != 1 {
		t.Fatalf("first reconcile=%#v seal=%#v", reconciled, state.reconciliation)
	}
	reconciledReplay, err := importer.Reconcile(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if !reconciledReplay.Replayed || reconciledReplay.ComparisonDigest != reconciled.ComparisonDigest || state.reconciliation == nil || state.reconciliation.ComparisonDigest != reconciled.ComparisonDigest || state.sealRecords != 1 {
		t.Fatalf("reconcile replay=%#v seal=%#v", reconciledReplay, state.reconciliation)
	}
}

func TestCustomerTimelineHistoryQuarantineAndBatchRollback(t *testing.T) {
	t.Run("redacted fact becomes a terminal quarantine without target", func(t *testing.T) {
		state := newCustomerTimelineHistoryState()
		row := customerTimelineHistoryRow(t, 1, 1, "")
		payload := customerTimelineHistoryPayload(1, "")
		payload["unionid"] = "[REDACTED]"
		row = customerTimelineHistoryRedactedRow(t, 1, 1, payload, "unionid")
		importer := customerTimelineHistoryImporter(t, &customerTimelineHistoryReady{}, &customerTimelineHistoryArchive{rows: []v1archive.ArchivedRow{row}}, state, customerTimelineHistoryResolver{})
		result, err := importer.Import(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
		if err != nil || result != (CustomerTimelineHistoryImportResult{Quarantined: 1}) || len(state.targets) != 0 || len(state.terminals) != 1 {
			t.Fatalf("result=%#v targets=%d terminal=%d err=%v", result, len(state.targets), len(state.terminals), err)
		}
		for _, terminal := range state.terminals {
			if terminal.Disposition != timeline.DispositionQuarantine || terminal.Reason != timeline.ReasonFieldRedacted || terminal.TargetID != 0 {
				t.Fatalf("unexpected quarantine terminal: %#v", terminal)
			}
		}
	})

	t.Run("receipt conflict rolls the whole source batch back", func(t *testing.T) {
		state := newCustomerTimelineHistoryState()
		state.failTerminalAt = 1
		redacted := customerTimelineHistoryPayload(2, "[REDACTED]")
		archive := &customerTimelineHistoryArchive{rows: []v1archive.ArchivedRow{
			customerTimelineHistoryRow(t, 1, 1, ""), customerTimelineHistoryRedactedRow(t, 2, 2, redacted, "unionid"),
		}}
		importer := customerTimelineHistoryImporter(t, &customerTimelineHistoryReady{}, archive, state, customerTimelineHistoryResolver{})
		result, err := importer.Import(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
		if !errors.Is(err, errCustomerTimelineHistoryRecord) || result != (CustomerTimelineHistoryImportResult{}) || len(state.targets) != 0 || len(state.terminals) != 0 || state.uow.rollbacks != 1 {
			t.Fatalf("err=%v result=%#v targets=%d terminals=%d rollbacks=%d", err, result, len(state.targets), len(state.terminals), state.uow.rollbacks)
		}
	})
}

func TestCustomerTimelineHistoryFailsClosedBeforeWriteAndOnTargetDrift(t *testing.T) {
	state := newCustomerTimelineHistoryState()
	ready := &customerTimelineHistoryReady{err: errors.New("not reconciled")}
	archive := &customerTimelineHistoryArchive{rows: []v1archive.ArchivedRow{customerTimelineHistoryRow(t, 1, 1, "")}}
	importer := customerTimelineHistoryImporter(t, ready, archive, state, customerTimelineHistoryResolver{})
	if _, err := importer.Import(context.Background(), "timeline-run", customerTimelineHistoryTestKey); !errors.Is(err, ready.err) || archive.calls != 0 || state.uow.calls != 1 || state.uow.commits != 0 || state.uow.rollbacks != 1 {
		t.Fatalf("archive readiness did not stop writes: err=%v archive=%d uow=%d", err, archive.calls, state.uow.calls)
	}

	ready.err = nil
	if _, err := importer.Import(context.Background(), "timeline-run", customerTimelineHistoryTestKey); err != nil {
		t.Fatal(err)
	}
	first, err := importer.Reconcile(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
	if err != nil {
		t.Fatal(err)
	}
	sealed := *state.reconciliation
	tampered := state.targets[1]
	tampered.Title = "tampered private title"
	state.targets[1] = tampered
	if _, err := importer.Reconcile(context.Background(), "timeline-run", customerTimelineHistoryTestKey); !errors.Is(err, ErrConflict) || state.reconciliation == nil || *state.reconciliation != sealed || state.sealRecords != 1 || state.uow.rollbacks == 0 || first.Replayed {
		t.Fatalf("target drift accepted or seal changed: err=%v seal=%#v records=%d rollbacks=%d", err, state.reconciliation, state.sealRecords, state.uow.rollbacks)
	}
}

func TestCustomerTimelineHistoryReconcileRechecksVerifiedUnionIDBinding(t *testing.T) {
	state := newCustomerTimelineHistoryState()
	resolver := customerTimelineHistoryResolver{"verified": customerTimelineHistoryInt64Pointer(17)}
	archive := &customerTimelineHistoryArchive{rows: []v1archive.ArchivedRow{customerTimelineHistoryRow(t, 1, 1, "verified")}}
	importer := customerTimelineHistoryImporter(t, &customerTimelineHistoryReady{}, archive, state, resolver)
	if _, err := importer.Import(context.Background(), "timeline-run", customerTimelineHistoryTestKey); err != nil {
		t.Fatal(err)
	}
	resolver["verified"] = nil
	if _, err := importer.Reconcile(context.Background(), "timeline-run", customerTimelineHistoryTestKey); !errors.Is(err, ErrConflict) || state.reconciliation != nil {
		t.Fatalf("unverified customer reference accepted: err=%v seal=%#v", err, state.reconciliation)
	}
}

func TestCustomerTimelineHistoryReconcileKeepsSealStableAcrossBatchRetry(t *testing.T) {
	state := newCustomerTimelineHistoryState()
	archive := &customerTimelineHistoryArchive{rows: []v1archive.ArchivedRow{customerTimelineHistoryRow(t, 1, 1, "")}}
	importer := customerTimelineHistoryImporter(t, &customerTimelineHistoryReady{}, archive, state, customerTimelineHistoryResolver{})
	if _, err := importer.Import(context.Background(), "timeline-run", customerTimelineHistoryTestKey); err != nil {
		t.Fatal(err)
	}
	state.uow.retryAtCall = state.uow.calls + 2 // readiness is next; retry the source/target comparison batch.
	retried, err := importer.Reconcile(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
	if err != nil || state.uow.retries != 1 || state.reconciliation == nil {
		t.Fatalf("reconciliation retry failed: result=%#v retries=%d seal=%#v err=%v", retried, state.uow.retries, state.reconciliation, err)
	}
	state.reconciliation = nil
	state.sealRecords = 0
	state.uow.retryAtCall = 0
	plain, err := importer.Reconcile(context.Background(), "timeline-run", customerTimelineHistoryTestKey)
	if err != nil || retried.ComparisonDigest != plain.ComparisonDigest || retried.SelectedSourceCount != plain.SelectedSourceCount {
		t.Fatalf("retry altered aggregate proof: retry=%#v plain=%#v err=%v", retried, plain, err)
	}
}

type customerTimelineHistoryReady struct {
	calls int
	err   error
}

func (ready *customerTimelineHistoryReady) VerifyCustomerTimelineArchiveReady(_ context.Context, run string) error {
	ready.calls++
	if run != "timeline-run" {
		return ErrInvalidScope
	}
	return ready.err
}

type customerTimelineHistoryArchive struct {
	rows  []v1archive.ArchivedRow
	calls int
}

func (archive *customerTimelineHistoryArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	archive.calls++
	if run != "timeline-run" || table != timeline.TableID || callback == nil {
		return ErrInvalidScope
	}
	for _, row := range archive.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type customerTimelineHistoryResolver map[string]*int64

func (resolver customerTimelineHistoryResolver) ResolveVerifiedCustomerTimelineUnionID(_ context.Context, unionID string) (*int64, error) {
	if value, found := resolver[unionID]; found && value != nil {
		copy := *value
		return &copy, nil
	}
	return nil, nil
}

var errCustomerTimelineHistoryRecord = errors.New("timeline terminal record failed")

type customerTimelineHistoryState struct {
	nextID         int64
	targets        map[int64]contact.HistoricalCustomerTimelineEvent
	writerReceipts map[string]contact.CustomerTimelineHistoryReceipt
	terminals      map[[sha256.Size]byte]CustomerTimelineTerminal
	reconciliation *CustomerTimelineReconciliationSeal
	failTerminalAt int
	terminalCalls  int
	sealRecords    int
	uow            customerTimelineHistoryUOW
}

func newCustomerTimelineHistoryState() *customerTimelineHistoryState {
	state := &customerTimelineHistoryState{nextID: 1, targets: map[int64]contact.HistoricalCustomerTimelineEvent{}, writerReceipts: map[string]contact.CustomerTimelineHistoryReceipt{}, terminals: map[[sha256.Size]byte]CustomerTimelineTerminal{}}
	state.uow.state = state
	return state
}

type customerTimelineHistoryUOW struct {
	state          *customerTimelineHistoryState
	calls, commits int
	rollbacks      int
	retryAtCall    int
	retries        int
}

func (uow *customerTimelineHistoryUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if ctx == nil || callback == nil {
		return ErrInvalidScope
	}
	before := uow.state.clone()
	if err := callback(ctx); err != nil {
		uow.state.restore(before)
		uow.rollbacks++
		return err
	}
	if uow.retryAtCall == uow.calls {
		uow.state.restore(before)
		uow.retries++
		if err := callback(ctx); err != nil {
			uow.state.restore(before)
			uow.rollbacks++
			return err
		}
	}
	uow.commits++
	return nil
}

func (state *customerTimelineHistoryState) clone() customerTimelineHistoryState {
	copy := *state
	copy.targets = make(map[int64]contact.HistoricalCustomerTimelineEvent, len(state.targets))
	for key, value := range state.targets {
		copy.targets[key] = value
	}
	copy.writerReceipts = make(map[string]contact.CustomerTimelineHistoryReceipt, len(state.writerReceipts))
	for key, value := range state.writerReceipts {
		copy.writerReceipts[key] = value
	}
	copy.terminals = make(map[[sha256.Size]byte]CustomerTimelineTerminal, len(state.terminals))
	for key, value := range state.terminals {
		copy.terminals[key] = value
	}
	if state.reconciliation != nil {
		value := *state.reconciliation
		copy.reconciliation = &value
	}
	return copy
}

func (state *customerTimelineHistoryState) restore(before customerTimelineHistoryState) {
	next, targets, writerReceipts, terminals, reconciliation := before.nextID, before.targets, before.writerReceipts, before.terminals, before.reconciliation
	state.nextID, state.targets, state.writerReceipts, state.terminals, state.reconciliation = next, targets, writerReceipts, terminals, reconciliation
}

func (state *customerTimelineHistoryState) ImportHistoricalCustomerTimelineEvent(_ context.Context, source string, value contact.HistoricalCustomerTimelineEvent) (contact.CustomerTimelineHistoryReceipt, error) {
	if source != hex.EncodeToString(value.SourceKeyDigest[:]) {
		return contact.CustomerTimelineHistoryReceipt{}, ErrConflict
	}
	if receipt, found := state.writerReceipts[source]; found {
		receipt.Replayed = true
		return receipt, nil
	}
	value.ID = state.nextID
	state.nextID++
	digest, err := contactapp.HistoricalCustomerTimelineEventDigest(value)
	if err != nil {
		return contact.CustomerTimelineHistoryReceipt{}, err
	}
	state.targets[value.ID] = value
	receipt := contact.CustomerTimelineHistoryReceipt{Kind: customerTimelineHistoryKind, SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetDigest: digest, TargetID: value.ID}
	state.writerReceipts[source] = receipt
	state.terminals[value.SourceKeyDigest] = CustomerTimelineTerminal{
		Version: customerTimelineHistoryVersion, ArchiveRunID: "timeline-run", TableID: timeline.TableID, Kind: customerTimelineHistoryKind,
		SourceKeyHMAC: value.SourceKeyDigest, PayloadHMAC: value.SourcePayloadDigest, FieldHMAC: value.SourceFieldDigest,
		Disposition: timeline.DispositionCandidate, TargetID: value.ID, TargetDigest: digest,
	}
	return receipt, nil
}

func (state *customerTimelineHistoryState) GetHistoricalCustomerTimelineEvent(_ context.Context, id int64) (contact.HistoricalCustomerTimelineEvent, error) {
	value, found := state.targets[id]
	if !found {
		return contact.HistoricalCustomerTimelineEvent{}, ErrConflict
	}
	return value, nil
}

func (state *customerTimelineHistoryState) LoadCustomerTimelineTerminal(_ context.Context, version string, key [sha256.Size]byte) (CustomerTimelineTerminal, bool, error) {
	if version != customerTimelineHistoryVersion {
		return CustomerTimelineTerminal{}, false, ErrConflict
	}
	value, found := state.terminals[key]
	return value, found, nil
}

func (state *customerTimelineHistoryState) RecordCustomerTimelineTerminal(_ context.Context, value CustomerTimelineTerminal) error {
	state.terminalCalls++
	if state.failTerminalAt == state.terminalCalls {
		return errCustomerTimelineHistoryRecord
	}
	state.terminals[value.SourceKeyHMAC] = value
	return nil
}

func (state *customerTimelineHistoryState) LoadCustomerTimelineReconciliationSeal(_ context.Context, version, run string) (CustomerTimelineReconciliationSeal, bool, error) {
	if version != customerTimelineHistoryVersion || run != "timeline-run" {
		return CustomerTimelineReconciliationSeal{}, false, ErrConflict
	}
	if state.reconciliation == nil {
		return CustomerTimelineReconciliationSeal{}, false, nil
	}
	return *state.reconciliation, true, nil
}

func (state *customerTimelineHistoryState) RecordCustomerTimelineReconciliationSeal(_ context.Context, value CustomerTimelineReconciliationSeal) error {
	if state.reconciliation != nil {
		return ErrConflict
	}
	state.sealRecords++
	copy := value
	state.reconciliation = &copy
	return nil
}

func customerTimelineHistoryImporter(t *testing.T, ready CustomerTimelineArchiveReady, archive timeline.ArchiveSource, state *customerTimelineHistoryState, resolver CustomerTimelineResolver) *CustomerTimelineHistoryImporter {
	t.Helper()
	importer, err := NewCustomerTimelineHistoryImporter(ready, archive, &state.uow, state, resolver, state, state, state)
	if err != nil {
		t.Fatal(err)
	}
	return importer
}

func customerTimelineHistoryPayload(id int64, union string) map[string]any {
	at := time.Date(2026, 8, 29, 3, 4, 5, 678901234, time.FixedZone("source", 8*3600))
	return map[string]any{
		"id": id, "event_id": "event-" + strconv.FormatInt(id, 10), "event_type": "legacy", "event_time": at,
		"title": "private title", "summary": "private summary", "source_table": "legacy_source", "source_id": strconv.FormatInt(id, 10),
		"metadata_json": json.RawMessage(`null`), "created_at": at.Add(time.Second), "unionid": union,
	}
}

func customerTimelineHistoryRow(t *testing.T, id, ordinal int64, union string) v1archive.ArchivedRow {
	t.Helper()
	return customerTimelineHistorySealedRow(t, id, ordinal, customerTimelineHistoryPayload(id, union), nil)
}

func customerTimelineHistoryRedactedRow(t *testing.T, id, ordinal int64, payload map[string]any, root string) v1archive.ArchivedRow {
	t.Helper()
	return customerTimelineHistorySealedRow(t, id, ordinal, payload, []string{root})
}

func customerTimelineHistorySealedRow(t *testing.T, id, ordinal int64, payload map[string]any, roots []string) v1archive.ArchivedRow {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _, err := v1archive.RedactPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	source, err := v1archive.SourceKeyHMAC(customerTimelineHistoryTestKey, "customer_timeline_event_next", []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(customerTimelineHistoryTestKey, "customer_timeline_event_next", canonical)
	if err != nil {
		t.Fatal(err)
	}
	fieldHMAC, err := v1archive.FieldHMAC(customerTimelineHistoryTestKey, "customer_timeline_event_next", roots)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: timeline.TableID, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: fieldHMAC, Payload: canonical, RedactedFields: roots}
}

func customerTimelineHistoryInt64Pointer(value int64) *int64 { return &value }

var _ CustomerTimelineWriter = (*customerTimelineHistoryState)(nil)
var _ CustomerTimelineTargetReader = (*customerTimelineHistoryState)(nil)
var _ CustomerTimelineImportJournal = (*customerTimelineHistoryState)(nil)
var _ CustomerTimelineReconciliationJournal = (*customerTimelineHistoryState)(nil)
var _ timeline.ArchiveSource = (*customerTimelineHistoryArchive)(nil)
