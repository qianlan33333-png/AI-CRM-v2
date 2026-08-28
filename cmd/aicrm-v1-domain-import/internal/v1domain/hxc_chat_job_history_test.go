package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	chatjobhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxcchatjobhistory"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestHXCChatJobFactPreservesPrivateAndNullableSourceFields(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789123, time.FixedZone("source", 8*3600))
	source := hxcChatJobSourceFixture(1, at)
	fact, err := hxcChatJobFact(source)
	if err != nil {
		t.Fatal(err)
	}
	fact.ID = 1
	if _, err = hxcapp.HistoricalHXCChatJobDigest(fact); err != nil || fact.SourceID != -1 || fact.QueueSourceID == nil || *fact.QueueSourceID != -2 || fact.MemberSourceID != nil || fact.SendRecordSourceID == nil || *fact.SendRecordSourceID != 0 || fact.CreatedAt.Location() != time.UTC || fact.CreatedAt.Nanosecond() != 456789000 || fact.FinishedAtSource != "not-a-time" {
		t.Fatalf("fact=%+v err=%v", fact, err)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || string(encoded) == "" || containsAny(string(encoded), source.ExternalContactID, source.Phone, source.ExternalMessageID, source.ExternalSessionID, source.LaohuangTaskID, source.ReplyText, source.ErrorCode, source.ErrorMessage) {
		t.Fatalf("private value leaked: %s err=%v", encoded, err)
	}
	source.RequestPayloadJSON[0] = '['
	*source.QueueID = 99
	if string(fact.RequestPayloadJSON) != `{"request":1}` || *fact.QueueSourceID == 99 {
		t.Fatal("fact retained source aliases")
	}
}

func TestHXCChatJobEntriesBindScopeAndVerifyFullTargetDigest(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789000, time.UTC)
	first := hxcChatJobSourceFixture(2, at)
	second := hxcChatJobSourceFixture(1, at)
	selection := chatjobhistory.Selection{Jobs: []chatjobhistory.ChatJobCandidate{{SourceOrdinal: 1, Fact: first}, {SourceOrdinal: 2, Fact: second}}}
	store := &hxcChatJobTargetFake{values: map[int64]hxcport.HistoricalHXCChatJob{}}
	entries, err := hxcChatJobEntries(context.Background(), selection, "v1-full-archive-20260827", hxcChatJobTxStub{}, store, store)
	if err != nil || len(entries) != 2 || entries[0].scope.ImportVersion != HXCChatJobHistoryVersion || entries[0].scope.TableID != chatjobhistory.ChatJobsTableID || entries[0].scope.TargetDomain != "hxc" || entries[0].scope.TargetTable != hxcChatJobHistoryTarget || entries[0].kind != hxcport.HXCHistoryChatJob || entries[0].source != SourceIdentifier(entries[0].key) || entries[0].source > entries[1].source {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	stored, err := hxcChatJobFact(second)
	if err != nil {
		t.Fatal(err)
	}
	stored.ID = 7
	store.values[7] = stored
	var entry hxcChatJobEntry
	for _, candidate := range entries {
		if candidate.key == second.Source.SourceKeyDigest {
			entry = candidate
			break
		}
	}
	digest, err := entry.verify(context.Background(), 7)
	if err != nil || digest == ([sha256.Size]byte{}) {
		t.Fatalf("verify=%x err=%v", digest, err)
	}
	stored.Phone += "drift"
	store.values[7] = stored
	if _, err = entry.verify(context.Background(), 7); !errors.Is(err, ErrConflict) {
		t.Fatalf("private target drift=%v", err)
	}
	duplicate := chatjobhistory.Selection{Jobs: []chatjobhistory.ChatJobCandidate{{SourceOrdinal: 1, Fact: first}, {SourceOrdinal: 2, Fact: first}}}
	if _, err = hxcChatJobEntries(context.Background(), duplicate, "run", hxcChatJobTxStub{}, store, store); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate source=%v", err)
	}
	if _, err = hxcChatJobEntries(nil, selection, "run", hxcChatJobTxStub{}, store, store); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("nil context=%v", err)
	}
}

func TestHXCChatJobHistoryJournalRejectsMalformedReceipts(t *testing.T) {
	bridge := hxcChatJobHistoryJournal{journal: &Journal{scope: Scope{ImportVersion: HXCChatJobHistoryVersion, ArchiveRunID: "run", AdapterID: "v1_full_archive", TableID: chatjobhistory.ChatJobsTableID, TargetDomain: "hxc", TargetTable: hxcChatJobHistoryTarget}}}
	source := SourceIdentifier(hxcChatJobDigest("source"))
	valid := hxcport.HXCHistoryReceipt{Kind: hxcport.HXCHistoryChatJob, SourceIdentifier: source, PayloadDigest: hxcChatJobDigest("payload"), TargetDigest: hxcChatJobDigest("target"), TargetID: 1}
	invalid := valid
	invalid.Replayed = true
	if err := bridge.RecordHXCHistory(context.Background(), invalid); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("replayed receipt=%v", err)
	}
	invalid = valid
	invalid.Kind = "other"
	if err := bridge.RecordHXCHistory(context.Background(), invalid); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong kind=%v", err)
	}
	invalid = valid
	invalid.SourceIdentifier = "not-hex"
	if err := bridge.RecordHXCHistory(context.Background(), invalid); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("bad source=%v", err)
	}
}

type hxcChatJobTargetFake struct {
	values map[int64]hxcport.HistoricalHXCChatJob
}

func (fake *hxcChatJobTargetFake) CreateHistoricalHXCChatJob(_ context.Context, value hxcport.HistoricalHXCChatJob) (hxcport.HistoricalHXCChatJob, error) {
	return value, errors.New("unused")
}
func (fake *hxcChatJobTargetFake) GetHistoricalHXCChatJob(_ context.Context, id int64) (hxcport.HistoricalHXCChatJob, error) {
	value, found := fake.values[id]
	if !found {
		return hxcport.HistoricalHXCChatJob{}, errors.New("not found")
	}
	return value, nil
}
func (fake *hxcChatJobTargetFake) ListHistoricalHXCChatJob(context.Context, hxcport.HXCChatJobHistoryQuery) ([]hxcport.HistoricalHXCChatJob, int64, error) {
	return nil, 0, errors.New("unused")
}

type hxcChatJobTxStub struct{}

func (hxcChatJobTxStub) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("unused") }
func (hxcChatJobTxStub) Commit(context.Context) error          { return errors.New("unused") }
func (hxcChatJobTxStub) Rollback(context.Context) error        { return errors.New("unused") }
func (hxcChatJobTxStub) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unused")
}
func (hxcChatJobTxStub) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (hxcChatJobTxStub) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (hxcChatJobTxStub) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("unused")
}
func (hxcChatJobTxStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unused")
}
func (hxcChatJobTxStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unused")
}
func (hxcChatJobTxStub) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (hxcChatJobTxStub) Conn() *pgx.Conn                                  { return nil }

func hxcChatJobSourceFixture(seed byte, at time.Time) chatjobhistory.ChatJobFact {
	queue, sendRecord := int64(-2), int64(0)
	return chatjobhistory.ChatJobFact{
		Source:   chatjobhistory.SourceEnvelope{SourceKeyDigest: hxcChatJobDigest(string([]byte{seed, 1})), PayloadDigest: hxcChatJobDigest(string([]byte{seed, 2})), FieldDigest: hxcChatJobDigest(string([]byte{seed, 3}))},
		SourceID: int64(seed) - 2, QueueID: &queue, ExternalContactID: "external", Phone: "phone", ExternalMessageID: "message", ExternalSessionID: "session", LaohuangTaskID: "task",
		RequestPayloadJSON: json.RawMessage(`{"request":1}`), AcceptedPayloadJSON: json.RawMessage("null"), CallbackPayloadJSON: json.RawMessage("[]"), OriginalStatus: "source-status", ReplyText: "reply", ErrorCode: "code", ErrorMessage: "error", SendChannel: "channel", SendRecordID: &sendRecord, SendResultJSON: json.RawMessage(`"result"`),
		CreatedAt: at, UpdatedAt: at.Add(-time.Second), FinishedAt: "not-a-time",
	}
}

func hxcChatJobDigest(value string) [sha256.Size]byte { return sha256.Sum256([]byte(value)) }

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if term != "" && strings.Contains(value, term) {
			return true
		}
	}
	return false
}
