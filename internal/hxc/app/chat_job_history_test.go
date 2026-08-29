package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestHXCChatJobDigestCoversEveryField(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789000, time.UTC)
	value := hxcChatJobFixture(1, at)
	value.ID = 1
	changes := []func(*hxc.HistoricalHXCChatJob){
		func(v *hxc.HistoricalHXCChatJob) { v.ID++ },
		func(v *hxc.HistoricalHXCChatJob) { v.SourceID++ },
		func(v *hxc.HistoricalHXCChatJob) { v.SourceKeyDigest[0]++ },
		func(v *hxc.HistoricalHXCChatJob) { v.SourcePayloadDigest[0]++ },
		func(v *hxc.HistoricalHXCChatJob) { v.SourceFieldDigest[0]++ },
		func(v *hxc.HistoricalHXCChatJob) { n := *v.QueueSourceID + 1; v.QueueSourceID = &n },
		func(v *hxc.HistoricalHXCChatJob) { n := *v.MemberSourceID + 1; v.MemberSourceID = &n },
		func(v *hxc.HistoricalHXCChatJob) { v.ExternalContactID += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.Phone += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.ExternalMessageID += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.ExternalSessionID += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.LaohuangTaskID += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.RequestPayloadJSON = json.RawMessage(`{"request":2}`) },
		func(v *hxc.HistoricalHXCChatJob) { v.AcceptedPayloadJSON = json.RawMessage(`{"accepted":2}`) },
		func(v *hxc.HistoricalHXCChatJob) { v.CallbackPayloadJSON = json.RawMessage(`{"callback":2}`) },
		func(v *hxc.HistoricalHXCChatJob) { v.OriginalStatus += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.ReplyText += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.ErrorCode += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.ErrorMessage += "x" },
		func(v *hxc.HistoricalHXCChatJob) { v.SendChannel += "x" },
		func(v *hxc.HistoricalHXCChatJob) { n := *v.SendRecordSourceID + 1; v.SendRecordSourceID = &n },
		func(v *hxc.HistoricalHXCChatJob) { v.SendResultJSON = json.RawMessage(`{"result":2}`) },
		func(v *hxc.HistoricalHXCChatJob) { v.CreatedAt = v.CreatedAt.Add(time.Microsecond) },
		func(v *hxc.HistoricalHXCChatJob) { v.UpdatedAt = v.UpdatedAt.Add(time.Microsecond) },
		func(v *hxc.HistoricalHXCChatJob) { v.FinishedAtSource += "x" },
	}
	baseline, err := HistoricalHXCChatJobDigest(value)
	if err != nil || baseline == ([32]byte{}) {
		t.Fatalf("baseline digest = %x, %v", baseline, err)
	}
	for index, change := range changes {
		changed := value
		change(&changed)
		digest, err := HistoricalHXCChatJobDigest(changed)
		if err != nil || digest == baseline {
			t.Fatalf("field mutation %d digest = %x, %v", index, digest, err)
		}
	}
}

func TestHXCChatJobDigestRequiresValidRawJSON(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	for name, raw := range map[string]json.RawMessage{"nil": nil, "empty": {}, "invalid": json.RawMessage("not-json")} {
		t.Run(name, func(t *testing.T) {
			value := hxcChatJobFixture(2, at)
			value.ID, value.RequestPayloadJSON = 1, raw
			if _, err := HistoricalHXCChatJobDigest(value); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
				t.Fatalf("invalid raw JSON = %v", err)
			}
		})
	}
	value := hxcChatJobFixture(3, at)
	value.ID, value.RequestPayloadJSON = 1, json.RawMessage("null")
	if _, err := HistoricalHXCChatJobDigest(value); err != nil {
		t.Fatalf("literal null = %v", err)
	}
}

func TestHXCChatJobDigestKeepsRawJSONBytes(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	compact := hxcChatJobFixture(3, at)
	compact.ID = 1
	spaced := compact
	spaced.RequestPayloadJSON = json.RawMessage(`{ "request": 1 }`)
	first, err := HistoricalHXCChatJobDigest(compact)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HistoricalHXCChatJobDigest(spaced)
	if err != nil || first == second {
		t.Fatalf("raw JSON byte preservation = %x, %v", second, err)
	}
}

func TestHXCChatJobWriterReplayReadbackAndAliases(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789123, time.FixedZone("source", 8*3600))
	store := &hxcChatJobStoreFake{values: map[int64]hxc.HistoricalHXCChatJob{}}
	journal := &hxcChatJobJournalFake{entries: map[string]hxc.HXCHistoryReceipt{}}
	writer, err := NewHXCChatJobHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := hxcChatJobFixture(4, at)
	value.SourceID = -7
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	receipt, err := writer.ImportChatJob(context.Background(), source, value)
	if err != nil || receipt.Kind != hxc.HXCHistoryChatJob || receipt.Replayed || receipt.TargetID != 1 {
		t.Fatalf("first import = %#v, %v", receipt, err)
	}
	stored := store.values[receipt.TargetID]
	if stored.CreatedAt.Location() != time.UTC || stored.CreatedAt.Nanosecond() != 456789000 || string(stored.RequestPayloadJSON) != `{"request":1}` {
		t.Fatalf("stored source fidelity lost: %#v", stored)
	}
	value.RequestPayloadJSON[0] = '['
	*value.QueueSourceID = 99
	if string(store.values[receipt.TargetID].RequestPayloadJSON) != `{"request":1}` || *store.values[receipt.TargetID].QueueSourceID == 99 {
		t.Fatal("writer retained caller aliases")
	}
	value = hxcChatJobFixture(4, at)
	value.SourceID = -7
	replay, err := writer.ImportChatJob(context.Background(), source, value)
	if err != nil || !replay.Replayed || replay.TargetDigest != receipt.TargetDigest || store.creates != 1 {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	value.Phone += "drift"
	if _, err := writer.ImportChatJob(context.Background(), source, value); !errors.Is(err, hxc.ErrHXCHistoryConflict) {
		t.Fatalf("private drift = %v", err)
	}
}

func TestHXCChatJobWriterFailsClosed(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	var typedNil *hxcChatJobStoreFake
	if writer, err := NewHXCChatJobHistoryWriter(typedNil, &hxcChatJobJournalFake{}); writer != nil || !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatalf("typed nil writer = %v, %v", writer, err)
	}
	store := &hxcChatJobStoreFake{values: map[int64]hxc.HistoricalHXCChatJob{}, createErr: errors.New("write failed")}
	journal := &hxcChatJobJournalFake{entries: map[string]hxc.HXCHistoryReceipt{}}
	writer, err := NewHXCChatJobHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := hxcChatJobFixture(5, at)
	if _, err := writer.ImportChatJob(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatalf("write error = %v", err)
	}
	if len(journal.entries) != 0 {
		t.Fatalf("failed write recorded receipt: %#v", journal.entries)
	}
	value = hxcChatJobFixture(6, at)
	value.SourceFieldDigest = [32]byte{}
	if _, err := writer.ImportChatJob(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatalf("field HMAC = %v", err)
	}
	store.createErr = nil
	journal.recordErr = errors.New("receipt write failed")
	value = hxcChatJobFixture(7, at)
	if _, err := writer.ImportChatJob(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatalf("receipt write error = %v", err)
	}
	journal.recordErr = nil
	value = hxcChatJobFixture(8, at)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	journal.entries[hxc.HXCHistoryChatJob+":"+source] = hxc.HXCHistoryReceipt{Kind: hxc.HXCHistoryChatJob, SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetID: 1}
	if _, err := writer.ImportChatJob(context.Background(), source, value); !errors.Is(err, hxc.ErrHXCHistoryConflict) {
		t.Fatalf("malformed receipt = %v", err)
	}
	journal.loadErr = errors.New("receipt read failed")
	if _, err := writer.ImportChatJob(context.Background(), source, value); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatalf("receipt read error = %v", err)
	}
}

type hxcChatJobStoreFake struct {
	next      int64
	creates   int
	values    map[int64]hxc.HistoricalHXCChatJob
	createErr error
}

func (s *hxcChatJobStoreFake) CreateHistoricalHXCChatJob(_ context.Context, value hxc.HistoricalHXCChatJob) (hxc.HistoricalHXCChatJob, error) {
	if s.createErr != nil {
		return hxc.HistoricalHXCChatJob{}, s.createErr
	}
	s.next++
	s.creates++
	value.ID = s.next
	s.values[value.ID] = value
	return value, nil
}

func (s *hxcChatJobStoreFake) GetHistoricalHXCChatJob(_ context.Context, id int64) (hxc.HistoricalHXCChatJob, error) {
	value, ok := s.values[id]
	if !ok {
		return hxc.HistoricalHXCChatJob{}, hxc.ErrHXCHistoryUnavailable
	}
	return value, nil
}

type hxcChatJobJournalFake struct {
	entries   map[string]hxc.HXCHistoryReceipt
	loadErr   error
	recordErr error
}

func (j *hxcChatJobJournalFake) LoadHXCHistory(_ context.Context, kind, source string) (hxc.HXCHistoryReceipt, bool, error) {
	if j.loadErr != nil {
		return hxc.HXCHistoryReceipt{}, false, j.loadErr
	}
	value, ok := j.entries[kind+":"+source]
	return value, ok, nil
}

func (j *hxcChatJobJournalFake) RecordHXCHistory(_ context.Context, value hxc.HXCHistoryReceipt) error {
	if j.recordErr != nil {
		return j.recordErr
	}
	if j.entries == nil {
		j.entries = map[string]hxc.HXCHistoryReceipt{}
	}
	j.entries[value.Kind+":"+value.SourceIdentifier] = value
	return nil
}

func hxcChatJobFixture(first byte, at time.Time) hxc.HistoricalHXCChatJob {
	queue, member, sendRecord := int64(-2), int64(0), int64(-3)
	return hxc.HistoricalHXCChatJob{SourceID: int64(first) - 10, SourceKeyDigest: hxcChatJobDigestByte(first), SourcePayloadDigest: hxcChatJobDigestByte(first + 30), SourceFieldDigest: hxcChatJobDigestByte(first + 60), QueueSourceID: &queue, MemberSourceID: &member, ExternalContactID: "external", Phone: "phone", ExternalMessageID: "message", ExternalSessionID: "session", LaohuangTaskID: "task", RequestPayloadJSON: json.RawMessage(`{"request":1}`), AcceptedPayloadJSON: json.RawMessage("null"), CallbackPayloadJSON: json.RawMessage("[]"), OriginalStatus: "old", ReplyText: "reply", ErrorCode: "code", ErrorMessage: "error", SendChannel: "channel", SendRecordSourceID: &sendRecord, SendResultJSON: json.RawMessage(`"result"`), CreatedAt: at, UpdatedAt: at.Add(-time.Second), FinishedAtSource: "2026-08-29 01:02:03"}
}

func hxcChatJobDigestByte(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
