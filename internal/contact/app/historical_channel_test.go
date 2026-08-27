package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var historicalChannelTestTime = time.Date(2026, 8, 28, 8, 0, 0, 123456789, time.FixedZone("CST", 8*60*60))

func TestHistoricalChannelWriterCreatesOnlyInactiveSafeDefinition(t *testing.T) {
	store := newHistoricalChannelStore()
	journal := newHistoricalChannelJournal()
	writer, err := NewHistoricalChannelWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.Import(context.Background(), historicalChannelDefinition())
	if err != nil || receipt.Replayed || receipt.TargetID != 1 || receipt.TargetDigest == ([32]byte{}) || store.creates != 1 || journal.records != 1 {
		t.Fatalf("receipt=%+v creates=%d records=%d err=%v", receipt, store.creates, journal.records, err)
	}
	record := store.records[receipt.TargetID]
	if record.Status != "inactive" || record.CreatedBy != 17 || record.UpdatedBy != 17 || !record.CreatedAt.Equal(historicalChannelTestTime.UTC().Truncate(time.Microsecond)) || !record.UpdatedAt.Equal(historicalChannelTestTime.Add(time.Hour).UTC().Truncate(time.Microsecond)) {
		t.Fatalf("record=%+v", record)
	}
	var projection map[string]any
	if err = json.Unmarshal(record.Projection, &projection); err != nil {
		t.Fatal(err)
	}
	if projection["channel_type"] != "qrcode" || projection["carrier_type"] != "qrcode" || projection["channel_code"] != "v1-course" || projection["channel_name"] != "课程渠道" || projection["status"] != "inactive" || projection["auto_accept_friend"] != false {
		t.Fatalf("projection=%s", record.Projection)
	}
	for _, key := range []string{"scene_value", "qr_url", "owner_staff_id", "customer_channel", "link_url", "final_url", "welcome_message", "entry_tag_id", "entry_tag_name", "entry_tag_group_name"} {
		if projection[key] != "" {
			t.Fatalf("projection %s = %#v, want empty", key, projection[key])
		}
	}
	for _, key := range []string{"welcome_image_library_ids", "welcome_miniprogram_library_ids", "welcome_attachment_library_ids", "welcome_group_invite_library_ids"} {
		values, ok := projection[key].([]any)
		if !ok || len(values) != 0 {
			t.Fatalf("projection %s = %#v, want []", key, projection[key])
		}
	}
	if _, found := projection["assignees"]; found {
		t.Fatalf("projection leaked assignees: %s", record.Projection)
	}
}

func TestHistoricalChannelWriterReplayAndTargetDrift(t *testing.T) {
	store := newHistoricalChannelStore()
	journal := newHistoricalChannelJournal()
	writer, _ := NewHistoricalChannelWriter(store, journal)
	definition := historicalChannelDefinition()
	first, err := writer.Import(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	second, err := writer.Import(context.Background(), definition)
	if err != nil || !second.Replayed || second != (contactport.HistoricalChannelReceipt{SourceIdentifier: first.SourceIdentifier, PayloadDigest: first.PayloadDigest, TargetDigest: first.TargetDigest, TargetID: first.TargetID, Replayed: true}) || store.creates != 1 || store.gets != 1 || journal.records != 1 {
		t.Fatalf("first=%+v second=%+v creates=%d gets=%d records=%d err=%v", first, second, store.creates, store.gets, journal.records, err)
	}
	changed := store.records[first.TargetID]
	changed.Name = "drift"
	store.records[first.TargetID] = changed
	if _, err = writer.Import(context.Background(), definition); !errors.Is(err, contactport.ErrHistoricalChannelConflict) {
		t.Fatalf("drift error=%v", err)
	}
}

func TestHistoricalChannelWriterRejectsConflictsAndInvalidDefinitions(t *testing.T) {
	store := newHistoricalChannelStore()
	journal := newHistoricalChannelJournal()
	writer, _ := NewHistoricalChannelWriter(store, journal)
	definition := historicalChannelDefinition()
	if _, err := writer.Import(context.Background(), definition); err != nil {
		t.Fatal(err)
	}
	changed := definition
	changed.PayloadDigest[0]++
	if _, err := writer.Import(context.Background(), changed); !errors.Is(err, contactport.ErrHistoricalChannelConflict) {
		t.Fatalf("payload drift error=%v", err)
	}
	for _, invalid := range []contactport.HistoricalChannelDefinition{
		func() contactport.HistoricalChannelDefinition {
			value := definition
			value.ChannelType = "provider"
			return value
		}(),
		func() contactport.HistoricalChannelDefinition {
			value := definition
			value.LegacyConfigDigest = "sha256:BAD"
			return value
		}(),
		func() contactport.HistoricalChannelDefinition {
			value := definition
			value.UpdatedAt = value.CreatedAt.Add(-time.Nanosecond)
			return value
		}(),
	} {
		if _, err := writer.Import(context.Background(), invalid); !errors.Is(err, contactport.ErrHistoricalChannelInvalid) {
			t.Fatalf("invalid=%+v error=%v", invalid, err)
		}
	}
}

func TestHistoricalChannelWriterRequiresTransactionBoundDependenciesAndPropagatesErrors(t *testing.T) {
	if writer, err := NewHistoricalChannelWriter(nil, newHistoricalChannelJournal()); writer != nil || !errors.Is(err, contactport.ErrHistoricalChannelUnavailable) {
		t.Fatalf("missing store writer=%v err=%v", writer, err)
	}
	if writer, err := NewHistoricalChannelWriter(newHistoricalChannelStore(), nil); writer != nil || !errors.Is(err, contactport.ErrHistoricalChannelUnavailable) {
		t.Fatalf("missing journal writer=%v err=%v", writer, err)
	}
	store, journal := newHistoricalChannelStore(), newHistoricalChannelJournal()
	writer, _ := NewHistoricalChannelWriter(store, journal)
	if _, err := writer.Import(nil, historicalChannelDefinition()); !errors.Is(err, contactport.ErrHistoricalChannelUnavailable) {
		t.Fatalf("nil context error=%v", err)
	}
	loadErr := errors.New("load failed")
	journal.loadErr = loadErr
	if _, err := writer.Import(context.Background(), historicalChannelDefinition()); !errors.Is(err, loadErr) {
		t.Fatalf("load error=%v", err)
	}
	journal.loadErr = nil
	createErr := errors.New("create failed")
	store.createErr = createErr
	if _, err := writer.Import(context.Background(), historicalChannelDefinition()); !errors.Is(err, createErr) {
		t.Fatalf("create error=%v", err)
	}
	store.createErr = nil
	recordErr := errors.New("record failed")
	journal.recordErr = recordErr
	if _, err := writer.Import(context.Background(), historicalChannelDefinition()); !errors.Is(err, recordErr) {
		t.Fatalf("record error=%v", err)
	}

	stableStore, stableJournal := newHistoricalChannelStore(), newHistoricalChannelJournal()
	stableWriter, _ := NewHistoricalChannelWriter(stableStore, stableJournal)
	if _, err := stableWriter.Import(context.Background(), historicalChannelDefinition()); err != nil {
		t.Fatal(err)
	}
	getErr := errors.New("get failed")
	stableStore.getErr = getErr
	if _, err := stableWriter.Import(context.Background(), historicalChannelDefinition()); !errors.Is(err, getErr) {
		t.Fatalf("get error=%v", err)
	}
}

func TestHistoricalChannelTargetDigestUsesJSONSemantics(t *testing.T) {
	record, err := historicalChannelRecord(historicalChannelDefinition())
	if err != nil {
		t.Fatal(err)
	}
	record.ID = 9
	first, err := HistoricalChannelTargetDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	record.Projection = json.RawMessage(` { "channel_name" : "课程渠道", "channel_code":"v1-course", "carrier_type":"qrcode", "channel_type":"qrcode", "schema_version":1, "status":"inactive", "scene_value":"", "qr_url":"", "owner_staff_id":"", "customer_channel":"", "link_url":"", "final_url":"", "welcome_message":"", "welcome_image_library_ids":[], "welcome_miniprogram_library_ids":[], "welcome_attachment_library_ids":[], "welcome_group_invite_library_ids":[], "auto_accept_friend":false, "entry_tag_id":"", "entry_tag_name":"", "entry_tag_group_name":"", "assignment_mode":"single_owner", "assignment_strategy":"ratio", "overflow_policy":"least_loaded", "assignment_config_json":{} } `)
	second, err := HistoricalChannelTargetDigest(record)
	if err != nil || first != second {
		t.Fatalf("first=%x second=%x err=%v", first, second, err)
	}
}

func historicalChannelDefinition() contactport.HistoricalChannelDefinition {
	return contactport.HistoricalChannelDefinition{
		SourceIdentifier: "automation_channel:49", PayloadDigest: sha256.Sum256([]byte("archived-row-49")),
		Code: "v1-course", Name: "课程渠道", ChannelType: "qrcode", CarrierType: "qrcode",
		LegacyConfigDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Actor: 17,
		CreatedAt: historicalChannelTestTime, UpdatedAt: historicalChannelTestTime.Add(time.Hour),
	}
}

type historicalChannelStore struct {
	records               map[int64]contactport.HistoricalChannelRecord
	nextID, creates, gets int64
	createErr, getErr     error
}

func newHistoricalChannelStore() *historicalChannelStore {
	return &historicalChannelStore{records: map[int64]contactport.HistoricalChannelRecord{}, nextID: 1}
}
func (store *historicalChannelStore) CreateHistoricalChannel(_ context.Context, record contactport.HistoricalChannelRecord) (contactport.HistoricalChannelRecord, error) {
	if store.createErr != nil {
		return contactport.HistoricalChannelRecord{}, store.createErr
	}
	record.ID, store.nextID, store.creates = store.nextID, store.nextID+1, store.creates+1
	store.records[record.ID] = record
	return record, nil
}
func (store *historicalChannelStore) GetHistoricalChannel(_ context.Context, id int64) (contactport.HistoricalChannelRecord, error) {
	store.gets++
	if store.getErr != nil {
		return contactport.HistoricalChannelRecord{}, store.getErr
	}
	record, found := store.records[id]
	if !found {
		return contactport.HistoricalChannelRecord{}, errors.New("historical channel not found")
	}
	return record, nil
}

type historicalChannelJournal struct {
	receipts           map[string]contactport.HistoricalChannelReceipt
	loadErr, recordErr error
	records            int64
}

func newHistoricalChannelJournal() *historicalChannelJournal {
	return &historicalChannelJournal{receipts: map[string]contactport.HistoricalChannelReceipt{}}
}
func (journal *historicalChannelJournal) LoadHistoricalChannel(_ context.Context, source string) (contactport.HistoricalChannelReceipt, bool, error) {
	if journal.loadErr != nil {
		return contactport.HistoricalChannelReceipt{}, false, journal.loadErr
	}
	receipt, found := journal.receipts[source]
	return receipt, found, nil
}
func (journal *historicalChannelJournal) RecordHistoricalChannel(_ context.Context, receipt contactport.HistoricalChannelReceipt) error {
	if journal.recordErr != nil {
		return journal.recordErr
	}
	if _, found := journal.receipts[receipt.SourceIdentifier]; found {
		return contactport.ErrHistoricalChannelConflict
	}
	journal.receipts[receipt.SourceIdentifier], journal.records = receipt, journal.records+1
	return nil
}
