package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	staticTailHistoryImportVersion = "v1-static-tail-history-a1"

	staticTailGroupInviteKind   = "group_invite"
	staticTailPageSliceKind     = "product_page_slice"
	staticTailCycleStrategyKind = "cycle_strategy"
	staticTailCycleVersionKind  = "cycle_version"
	staticTailCycleDocumentKind = "cycle_document"

	staticTailGroupInviteTable = "public/group_invite_library"
	staticTailPageSliceTable   = "public/wechat_pay_product_page_slices"
	staticTailStrategyTable    = "public/operation_cycle_strategies"
	staticTailVersionTable     = "public/operation_cycle_strategy_versions"
	staticTailDocumentTable    = "public/operation_cycle_strategy_version_documents"

	staticTailGroupInviteTarget = "media_v1_group_invite_history"
	staticTailPageSliceTarget   = "product_v1_page_slice_history"
	staticTailStrategyTarget    = "operation_cycle_v1_strategy_history"
	staticTailVersionTarget     = "operation_cycle_v1_version_history"
	staticTailDocumentTarget    = "operation_cycle_v1_document_history"
)

type staticTailHistoryScope struct {
	kind, table, domain, target string
}

var staticTailHistoryScopes = [...]staticTailHistoryScope{
	{staticTailGroupInviteKind, staticTailGroupInviteTable, "media", staticTailGroupInviteTarget},
	{staticTailPageSliceKind, staticTailPageSliceTable, "product", staticTailPageSliceTarget},
	{staticTailCycleStrategyKind, staticTailStrategyTable, "operationcycle", staticTailStrategyTarget},
	{staticTailCycleVersionKind, staticTailVersionTable, "operationcycle", staticTailVersionTarget},
	{staticTailCycleDocumentKind, staticTailDocumentTable, "operationcycle", staticTailDocumentTarget},
}

type staticTailHistoryTerminalJournal interface {
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
}

// StaticTailHistoryJournal keeps three owner-owned receipt APIs on the exact
// five source scopes. It only records immutable history and has no runtime
// operation path.
type StaticTailHistoryJournal struct {
	journals map[string]staticTailHistoryTerminalJournal
}

var (
	_ mediaport.StaticMediaHistoryJournal     = (*StaticTailHistoryJournal)(nil)
	_ productport.StaticProductHistoryJournal = (*StaticTailHistoryJournal)(nil)
	_ cycleport.StaticCycleHistoryJournal     = (*StaticTailHistoryJournal)(nil)
)

func NewStaticTailHistoryJournal(groupInvites, pageSlices, strategies, versions, documents *Journal) (*StaticTailHistoryJournal, error) {
	values := map[string]*Journal{
		staticTailGroupInviteKind:   groupInvites,
		staticTailPageSliceKind:     pageSlices,
		staticTailCycleStrategyKind: strategies,
		staticTailCycleVersionKind:  versions,
		staticTailCycleDocumentKind: documents,
	}
	if !validStaticTailHistoryJournalScopes(values) {
		return nil, ErrInvalidScope
	}
	terminals := make(map[string]staticTailHistoryTerminalJournal, len(values))
	for kind, journal := range values {
		terminals[kind] = journal
	}
	return newStaticTailHistoryJournal(terminals)
}

func newStaticTailHistoryJournal(journals map[string]staticTailHistoryTerminalJournal) (*StaticTailHistoryJournal, error) {
	if len(journals) != len(staticTailHistoryScopes) {
		return nil, ErrInvalidScope
	}
	for _, scope := range staticTailHistoryScopes {
		if journals[scope.kind] == nil {
			return nil, ErrInvalidScope
		}
	}
	return &StaticTailHistoryJournal{journals: journals}, nil
}

func (journal *StaticTailHistoryJournal) ValidateStaticTailHistoryImportScope(run string) error {
	if journal == nil || run == "" || len(journal.journals) != len(staticTailHistoryScopes) {
		return ErrInvalidScope
	}
	for _, scope := range staticTailHistoryScopes {
		selected, ok := journal.journals[scope.kind].(*Journal)
		if !ok || selected == nil || !validStaticTailHistoryScope(selected, scope) || selected.scope.ArchiveRunID != run {
			return ErrInvalidScope
		}
	}
	return nil
}

func (journal *StaticTailHistoryJournal) LoadStaticMediaHistory(ctx context.Context, kind, source string) (mediaport.StaticMediaHistoryReceipt, bool, error) {
	if kind != staticTailGroupInviteKind {
		return mediaport.StaticMediaHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.load(ctx, kind, source)
	if err != nil || !found {
		return mediaport.StaticMediaHistoryReceipt{}, found, err
	}
	receipt, err := staticTailReceiptFromTerminal(kind, source, terminal)
	if err != nil {
		return mediaport.StaticMediaHistoryReceipt{}, false, err
	}
	return mediaport.StaticMediaHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: receipt.payload, TargetDigest: receipt.target, TargetID: receipt.id}, true, nil
}

func (journal *StaticTailHistoryJournal) RecordStaticMediaHistory(ctx context.Context, receipt mediaport.StaticMediaHistoryReceipt) error {
	if receipt.Kind != staticTailGroupInviteKind {
		return ErrInvalidScope
	}
	terminal, err := staticTailTerminalFromReceipt(receipt.Kind, receipt.SourceIdentifier, receipt.PayloadDigest, receipt.TargetDigest, receipt.TargetID, receipt.Replayed)
	if err != nil {
		return err
	}
	return journal.record(ctx, receipt.Kind, terminal)
}

func (journal *StaticTailHistoryJournal) LoadStaticProductHistory(ctx context.Context, kind, source string) (productport.StaticProductHistoryReceipt, bool, error) {
	if kind != staticTailPageSliceKind {
		return productport.StaticProductHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.load(ctx, kind, source)
	if err != nil || !found {
		return productport.StaticProductHistoryReceipt{}, found, err
	}
	receipt, err := staticTailReceiptFromTerminal(kind, source, terminal)
	if err != nil {
		return productport.StaticProductHistoryReceipt{}, false, err
	}
	return productport.StaticProductHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: receipt.payload, TargetDigest: receipt.target, TargetID: receipt.id}, true, nil
}

func (journal *StaticTailHistoryJournal) RecordStaticProductHistory(ctx context.Context, receipt productport.StaticProductHistoryReceipt) error {
	if receipt.Kind != staticTailPageSliceKind {
		return ErrInvalidScope
	}
	terminal, err := staticTailTerminalFromReceipt(receipt.Kind, receipt.SourceIdentifier, receipt.PayloadDigest, receipt.TargetDigest, receipt.TargetID, receipt.Replayed)
	if err != nil {
		return err
	}
	return journal.record(ctx, receipt.Kind, terminal)
}

func (journal *StaticTailHistoryJournal) LoadStaticCycleHistory(ctx context.Context, kind, source string) (cycleport.StaticCycleHistoryReceipt, bool, error) {
	if !staticTailCycleKind(kind) {
		return cycleport.StaticCycleHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.load(ctx, kind, source)
	if err != nil || !found {
		return cycleport.StaticCycleHistoryReceipt{}, found, err
	}
	receipt, err := staticTailReceiptFromTerminal(kind, source, terminal)
	if err != nil {
		return cycleport.StaticCycleHistoryReceipt{}, false, err
	}
	return cycleport.StaticCycleHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: receipt.payload, TargetDigest: receipt.target, TargetID: receipt.id}, true, nil
}

func (journal *StaticTailHistoryJournal) RecordStaticCycleHistory(ctx context.Context, receipt cycleport.StaticCycleHistoryReceipt) error {
	if !staticTailCycleKind(receipt.Kind) {
		return ErrInvalidScope
	}
	terminal, err := staticTailTerminalFromReceipt(receipt.Kind, receipt.SourceIdentifier, receipt.PayloadDigest, receipt.TargetDigest, receipt.TargetID, receipt.Replayed)
	if err != nil {
		return err
	}
	return journal.record(ctx, receipt.Kind, terminal)
}

func (journal *StaticTailHistoryJournal) LoadTerminal(ctx context.Context, table, source string) (TerminalReceipt, bool, error) {
	scope, ok := staticTailHistoryScopeForTable(table)
	if !ok {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return journal.load(ctx, scope.kind, source)
}

func (journal *StaticTailHistoryJournal) RecordTerminal(ctx context.Context, table string, receipt TerminalReceipt) error {
	scope, ok := staticTailHistoryScopeForTable(table)
	if !ok {
		return ErrInvalidScope
	}
	return journal.record(ctx, scope.kind, receipt)
}

func (journal *StaticTailHistoryJournal) load(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
	selected, err := journal.selectJournal(kind)
	if err != nil || ctx == nil {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(source)
	if err != nil || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return selected.LoadTerminal(ctx, source)
}

func (journal *StaticTailHistoryJournal) record(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.selectJournal(kind)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *StaticTailHistoryJournal) selectJournal(kind string) (staticTailHistoryTerminalJournal, error) {
	if journal == nil || journal.journals == nil || !staticTailHistoryKind(kind) {
		return nil, ErrInvalidScope
	}
	selected := journal.journals[kind]
	if selected == nil {
		return nil, ErrInvalidScope
	}
	return selected, nil
}

type staticTailReceipt struct {
	id              int64
	payload, target [sha256.Size]byte
}

func staticTailReceiptFromTerminal(kind, source string, terminal TerminalReceipt) (staticTailReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || !staticTailHistoryKind(kind) || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) ||
		terminal.SourceKeyDigest != key || terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return staticTailReceipt{}, ErrConflict
	}
	return staticTailReceipt{id: id, payload: terminal.PayloadDigest, target: terminal.TargetDigest}, nil
}

func staticTailTerminalFromReceipt(kind, source string, payload, target [sha256.Size]byte, id int64, replayed bool) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	if err != nil || !staticTailHistoryKind(kind) || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) || payload == ([sha256.Size]byte{}) ||
		target == ([sha256.Size]byte{}) || id < 1 || replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: payload, Disposition: "import", TargetID: strconv.FormatInt(id, 10), TargetDigest: target}, nil
}

func validStaticTailHistoryJournalScopes(journals map[string]*Journal) bool {
	if len(journals) != len(staticTailHistoryScopes) {
		return false
	}
	var run string
	for _, scope := range staticTailHistoryScopes {
		journal := journals[scope.kind]
		if !validStaticTailHistoryScope(journal, scope) {
			return false
		}
		if run == "" {
			run = journal.scope.ArchiveRunID
		} else if run != journal.scope.ArchiveRunID {
			return false
		}
	}
	return run != ""
}

func validStaticTailHistoryScope(journal *Journal, expected staticTailHistoryScope) bool {
	return journal != nil && journal.tx != nil && journal.scope.valid() && journal.scope.ImportVersion == staticTailHistoryImportVersion &&
		journal.scope.AdapterID == v1archive.DefaultAdapterID && journal.scope.TableID == expected.table && journal.scope.TargetDomain == expected.domain && journal.scope.TargetTable == expected.target
}

func staticTailHistoryScopeForKind(kind string) (staticTailHistoryScope, bool) {
	for _, scope := range staticTailHistoryScopes {
		if scope.kind == kind {
			return scope, true
		}
	}
	return staticTailHistoryScope{}, false
}

func staticTailHistoryScopeForTable(table string) (staticTailHistoryScope, bool) {
	for _, scope := range staticTailHistoryScopes {
		if scope.table == table {
			return scope, true
		}
	}
	return staticTailHistoryScope{}, false
}

func staticTailHistoryKind(kind string) bool {
	_, ok := staticTailHistoryScopeForKind(kind)
	return ok
}

func staticTailCycleKind(kind string) bool {
	return kind == staticTailCycleStrategyKind || kind == staticTailCycleVersionKind || kind == staticTailCycleDocumentKind
}
