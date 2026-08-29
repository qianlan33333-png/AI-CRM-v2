package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	chatjobhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxcchatjobhistory"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const HXCChatJobHistoryVersion = "v1-hxc-chat-job-history-a1"

const hxcChatJobHistoryTarget = "hxc_v1_chat_job_history"

// HXCChatJobHistoryResult reports only sealed historical writes. It never
// starts, retries, sends, or otherwise revives a V1 chat job.
type HXCChatJobHistoryResult struct {
	Selected, Imported, Replayed int
	Reconciliation               *ReconciliationResult `json:",omitempty"`
}

// hxcChatJobHistoryJournal is the narrow per-table bridge for the existing
// HXC typed receipt protocol. It keeps target creation and generic receipts in
// the caller transaction selected by Journal.
type hxcChatJobHistoryJournal struct{ journal *Journal }

var _ hxcport.HXCHistoryJournal = hxcChatJobHistoryJournal{}

func (bridge hxcChatJobHistoryJournal) LoadHXCHistory(ctx context.Context, kind, source string) (hxcport.HXCHistoryReceipt, bool, error) {
	if bridge.journal == nil || bridge.journal.scope.ImportVersion != HXCChatJobHistoryVersion || kind != hxcport.HXCHistoryChatJob {
		return hxcport.HXCHistoryReceipt{}, false, ErrInvalidScope
	}
	value, found, err := bridge.journal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return hxcport.HXCHistoryReceipt{}, found, err
	}
	key, keyErr := ParseSourceIdentifier(source)
	id, idErr := positiveID(value.TargetID)
	if keyErr != nil || idErr != nil || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) || value.SourceKeyDigest != key || value.PayloadDigest == ([sha256.Size]byte{}) || value.TargetDigest == ([sha256.Size]byte{}) || value.Disposition != "import" || value.Reason != "" || len(value.Metadata) != 0 || strconv.FormatInt(id, 10) != value.TargetID {
		return hxcport.HXCHistoryReceipt{}, false, ErrConflict
	}
	return hxcport.HXCHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: value.PayloadDigest, TargetID: id, TargetDigest: value.TargetDigest}, true, nil
}

func (bridge hxcChatJobHistoryJournal) RecordHXCHistory(ctx context.Context, value hxcport.HXCHistoryReceipt) error {
	if bridge.journal == nil || bridge.journal.scope.ImportVersion != HXCChatJobHistoryVersion || value.Kind != hxcport.HXCHistoryChatJob || value.Replayed || value.TargetID < 1 || value.PayloadDigest == ([32]byte{}) || value.TargetDigest == ([32]byte{}) {
		return ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(value.SourceIdentifier)
	if err != nil || key == ([sha256.Size]byte{}) || value.SourceIdentifier != SourceIdentifier(key) {
		return ErrInvalidScope
	}
	return bridge.journal.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: value.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(value.TargetID, 10), TargetDigest: value.TargetDigest})
}

type hxcChatJobEntry struct {
	scope               Scope
	kind, source        string
	key, payload, field [sha256.Size]byte
	journal             *Journal
	write               func(context.Context) (hxcport.HXCHistoryReceipt, error)
	verify              func(context.Context, int64) ([sha256.Size]byte, error)
}

// hxcChatJobEntries turns the complete sealed selection into sorted historical
// writes. The later Run entrypoint supplies the generated store and reader;
// this core deliberately does not depend on a not-yet-generated constructor.
func hxcChatJobEntries(ctx context.Context, selected chatjobhistory.Selection, run string, tx pgx.Tx, store hxcport.HXCChatJobHistoryStore, reader hxcport.HXCChatJobHistoryReader) ([]hxcChatJobEntry, error) {
	if ctx == nil || run == "" || tx == nil || store == nil || reader == nil {
		return nil, ErrInvalidScope
	}
	entries := make([]hxcChatJobEntry, 0, selected.Total())
	for _, selectedJob := range selected.Jobs {
		if selectedJob.SourceOrdinal < 1 {
			return nil, ErrConflict
		}
		fact, err := hxcChatJobFact(selectedJob.Fact)
		if err != nil {
			return nil, err
		}
		scope := Scope{ImportVersion: HXCChatJobHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: chatjobhistory.ChatJobsTableID, TargetDomain: "hxc", TargetTable: hxcChatJobHistoryTarget}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		writer, err := hxcapp.NewHXCChatJobHistoryWriter(store, hxcChatJobHistoryJournal{journal: journal})
		if err != nil {
			return nil, err
		}
		probe := fact
		probe.ID = 1
		if _, err = hxcapp.HistoricalHXCChatJobDigest(probe); err != nil {
			return nil, err
		}
		source := SourceIdentifier(fact.SourceKeyDigest)
		entries = append(entries, hxcChatJobEntry{
			scope: scope, kind: hxcport.HXCHistoryChatJob, source: source, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (hxcport.HXCHistoryReceipt, error) {
				return writer.ImportChatJob(ctx, source, fact)
			},
			verify: func(ctx context.Context, id int64) ([sha256.Size]byte, error) {
				actual, err := reader.GetHistoricalHXCChatJob(ctx, id)
				if err != nil || actual.ID != id {
					return [sha256.Size]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, wantErr := hxcapp.HistoricalHXCChatJobDigest(expected)
				got, gotErr := hxcapp.HistoricalHXCChatJobDigest(actual)
				if wantErr != nil || gotErr != nil || want != got {
					return [sha256.Size]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].scope.TableID+"/"+entries[i].source < entries[j].scope.TableID+"/"+entries[j].source
	})
	for index, entry := range entries {
		if entry.source != SourceIdentifier(entry.key) || entry.key == ([sha256.Size]byte{}) || entry.payload == ([sha256.Size]byte{}) || entry.field == ([sha256.Size]byte{}) || (index > 0 && entries[index-1].source == entry.source) {
			return nil, ErrConflict
		}
	}
	return entries, nil
}

func hxcChatJobFact(value chatjobhistory.ChatJobFact) (hxcport.HistoricalHXCChatJob, error) {
	if value.Source.SourceKeyDigest == ([sha256.Size]byte{}) || value.Source.PayloadDigest == ([sha256.Size]byte{}) || value.Source.FieldDigest == ([sha256.Size]byte{}) || !json.Valid(value.RequestPayloadJSON) || !json.Valid(value.AcceptedPayloadJSON) || !json.Valid(value.CallbackPayloadJSON) || !json.Valid(value.SendResultJSON) || value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() {
		return hxcport.HistoricalHXCChatJob{}, ErrConflict
	}
	return hxcport.HistoricalHXCChatJob{
		SourceID: value.SourceID, SourceKeyDigest: value.Source.SourceKeyDigest, SourcePayloadDigest: value.Source.PayloadDigest, SourceFieldDigest: value.Source.FieldDigest,
		QueueSourceID: cloneHXCChatJobSourceID(value.QueueID), MemberSourceID: cloneHXCChatJobSourceID(value.MemberID), ExternalContactID: value.ExternalContactID, Phone: value.Phone,
		ExternalMessageID: value.ExternalMessageID, ExternalSessionID: value.ExternalSessionID, LaohuangTaskID: value.LaohuangTaskID,
		RequestPayloadJSON: cloneHXCChatJobSourceJSON(value.RequestPayloadJSON), AcceptedPayloadJSON: cloneHXCChatJobSourceJSON(value.AcceptedPayloadJSON), CallbackPayloadJSON: cloneHXCChatJobSourceJSON(value.CallbackPayloadJSON),
		OriginalStatus: value.OriginalStatus, ReplyText: value.ReplyText, ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage, SendChannel: value.SendChannel,
		SendRecordSourceID: cloneHXCChatJobSourceID(value.SendRecordID), SendResultJSON: cloneHXCChatJobSourceJSON(value.SendResultJSON),
		CreatedAt: value.CreatedAt.UTC().Truncate(time.Microsecond), UpdatedAt: value.UpdatedAt.UTC().Truncate(time.Microsecond), FinishedAtSource: value.FinishedAt,
	}, nil
}

func cloneHXCChatJobSourceID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneHXCChatJobSourceJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
