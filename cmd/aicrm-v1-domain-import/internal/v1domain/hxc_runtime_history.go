package v1domain

import (
	"context"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	runtimehistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxcruntimehistory"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcstore "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const HXCRuntimeHistoryVersion = "v1-hxc-runtime-history-a1"

type HXCRuntimeHistoryResult struct {
	Selected, Imported, Replayed int
	Reconciliation               *ReconciliationResult `json:",omitempty"`
}

// This bridge records only the new, HXC-owned historical targets. Earlier
// archive terminals remain unchanged; no current sender or send work is created.
type hxcRuntimeJournal struct {
	journal *Journal
	kind    string
}

func (j hxcRuntimeJournal) LoadHXCHistory(ctx context.Context, kind, source string) (hxcport.HXCHistoryReceipt, bool, error) {
	if kind != j.kind || j.journal == nil {
		return hxcport.HXCHistoryReceipt{}, false, ErrInvalidScope
	}
	value, found, err := j.journal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return hxcport.HXCHistoryReceipt{}, found, err
	}
	id, err := positiveID(value.TargetID)
	if err != nil || value.TargetID != strconv.FormatInt(id, 10) || SourceIdentifier(value.SourceKeyDigest) != source || value.Disposition != "import" || value.Reason != "" || len(value.Metadata) != 0 || value.PayloadDigest == ([32]byte{}) || value.TargetDigest == ([32]byte{}) {
		return hxcport.HXCHistoryReceipt{}, false, ErrConflict
	}
	return hxcport.HXCHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: value.PayloadDigest, TargetID: id, TargetDigest: value.TargetDigest}, true, nil
}

func (j hxcRuntimeJournal) RecordHXCHistory(ctx context.Context, value hxcport.HXCHistoryReceipt) error {
	key, err := ParseSourceIdentifier(value.SourceIdentifier)
	if err != nil || key == ([32]byte{}) || SourceIdentifier(key) != value.SourceIdentifier || value.Kind != j.kind || j.journal == nil || value.Replayed || value.TargetID < 1 || value.PayloadDigest == ([32]byte{}) || value.TargetDigest == ([32]byte{}) {
		return ErrInvalidScope
	}
	return j.journal.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: value.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(value.TargetID, 10), TargetDigest: value.TargetDigest})
}

type hxcRuntimeEntry struct {
	scope               Scope
	kind, source        string
	key, payload, field [32]byte
	journal             *Journal
	write               func(context.Context) (hxcport.HXCHistoryReceipt, error)
	verify              func(context.Context, int64) ([32]byte, error)
}

func hxcRuntimeEntries(selected runtimehistory.Selection, run string, tx pgx.Tx) ([]hxcRuntimeEntry, error) {
	if run == "" || tx == nil {
		return nil, ErrInvalidScope
	}
	entries := make([]hxcRuntimeEntry, 0, len(selected.SenderConfigs)+len(selected.SendRecords))
	reader := hxcstore.NewHXCHistoryReader(tx)
	for _, source := range selected.SenderConfigs {
		v := source.Fact
		fact := hxcport.HistoricalHXCSenderConfig{HistoricalHXCRuntimeIdentity: hxcport.HistoricalHXCRuntimeIdentity{SourceID: v.SourceID, SourceKeyDigest: source.SourceKeyDigest, SourcePayloadDigest: source.SourcePayloadDigest, SourceFieldDigest: source.SourceFieldDigest, PrivateDigest: [32]byte(v.PrivateDigest)}, Priority: v.Priority, OriginalIsActive: v.OriginalIsActive, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
		scope := Scope{ImportVersion: HXCRuntimeHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: runtimehistory.SenderConfigTableID, TargetDomain: "hxc", TargetTable: "hxc_v1_sender_config_history"}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		writer, err := hxcapp.NewHXCRuntimeHistoryWriter(hxcstore.NewHXCHistoryStore(), hxcRuntimeJournal{journal: journal, kind: hxcport.HXCHistorySenderConfig})
		if err != nil {
			return nil, err
		}
		key := SourceIdentifier(source.SourceKeyDigest)
		entries = append(entries, hxcRuntimeEntry{scope: scope, kind: hxcport.HXCHistorySenderConfig, source: key, key: source.SourceKeyDigest, payload: source.SourcePayloadDigest, field: source.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (hxcport.HXCHistoryReceipt, error) {
				return writer.ImportSenderConfig(ctx, key, fact)
			},
			verify: func(ctx context.Context, id int64) ([32]byte, error) {
				actual, err := reader.GetHistoricalHXCSenderConfig(ctx, id)
				if err != nil || actual.ID != id {
					return [32]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, e1 := hxcapp.HistoricalHXCSenderConfigDigest(expected)
				got, e2 := hxcapp.HistoricalHXCSenderConfigDigest(actual)
				if e1 != nil || e2 != nil || want != got {
					return [32]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	for _, source := range selected.SendRecords {
		v := source.Fact
		fact := hxcport.HistoricalHXCSendRecord{HistoricalHXCRuntimeIdentity: hxcport.HistoricalHXCRuntimeIdentity{SourceID: v.SourceID, SourceKeyDigest: source.SourceKeyDigest, SourcePayloadDigest: source.SourcePayloadDigest, SourceFieldDigest: source.SourceFieldDigest, PrivateDigest: [32]byte(v.PrivateDigest)}, TaskType: v.TaskType, OriginalStatus: v.OriginalStatus, SelectedCount: v.SelectedCount, EligibleCount: v.EligibleCount, SentCount: v.SentCount, SkippedCount: v.SkippedCount, PlannedCount: v.PlannedCount, QueuedCount: v.QueuedCount, DispatchingCount: v.DispatchingCount, SucceededCount: v.SucceededCount, FailedCount: v.FailedCount, BlockedCount: v.BlockedCount, CancelledCount: v.CancelledCount, ImageCount: v.ImageCount, IncludeDoNotDisturb: v.IncludeDoNotDisturb, TargetSource: v.TargetSource, TargetSourceID: v.TargetSourceID, CreatedAt: v.CreatedAt, LastStatusSyncAt: v.LastStatusSyncAt, LastRefreshedAt: v.LastRefreshedAt}
		scope := Scope{ImportVersion: HXCRuntimeHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: runtimehistory.SendRecordsTableID, TargetDomain: "hxc", TargetTable: "hxc_v1_send_record_history"}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		writer, err := hxcapp.NewHXCRuntimeHistoryWriter(hxcstore.NewHXCHistoryStore(), hxcRuntimeJournal{journal: journal, kind: hxcport.HXCHistorySendRecord})
		if err != nil {
			return nil, err
		}
		key := SourceIdentifier(source.SourceKeyDigest)
		entries = append(entries, hxcRuntimeEntry{scope: scope, kind: hxcport.HXCHistorySendRecord, source: key, key: source.SourceKeyDigest, payload: source.SourcePayloadDigest, field: source.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (hxcport.HXCHistoryReceipt, error) {
				return writer.ImportSendRecord(ctx, key, fact)
			},
			verify: func(ctx context.Context, id int64) ([32]byte, error) {
				actual, err := reader.GetHistoricalHXCSendRecord(ctx, id)
				if err != nil || actual.ID != id {
					return [32]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, e1 := hxcapp.HistoricalHXCSendRecordDigest(expected)
				got, e2 := hxcapp.HistoricalHXCSendRecordDigest(actual)
				if e1 != nil || e2 != nil || want != got {
					return [32]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].scope.TableID+"/"+entries[i].source < entries[j].scope.TableID+"/"+entries[j].source
	})
	return entries, nil
}
