package app

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

func TestRadarClickHistoryWriterCreatesAndReplaysActualTarget(t *testing.T) {
	store := &radarClickHistoryStoreFake{}
	journal := &radarClickHistoryJournalFake{}
	writer, err := NewRadarClickHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := radarClickHistoryFixture(1)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	first, err := writer.ImportHistoricalRadarClick(context.Background(), source, value)
	if err != nil || first.Replayed || first.TargetID != 1 || store.createCalls != 1 || journal.recordCalls != 1 {
		t.Fatalf("first=%+v create=%d record=%d err=%v", first, store.createCalls, journal.recordCalls, err)
	}
	second, err := writer.ImportHistoricalRadarClick(context.Background(), source, value)
	if err != nil || !second.Replayed || store.createCalls != 1 || store.getCalls != 1 {
		t.Fatalf("second=%+v create=%d get=%d err=%v", second, store.createCalls, store.getCalls, err)
	}
}

func TestRadarClickHistoryWriterRejectsTargetDriftAndInvalidReferences(t *testing.T) {
	store := &radarClickHistoryStoreFake{}
	journal := &radarClickHistoryJournalFake{}
	writer, err := NewRadarClickHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := radarClickHistoryFixture(2)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	if _, err = writer.ImportHistoricalRadarClick(context.Background(), source, value); err != nil {
		t.Fatal(err)
	}
	drifted := store.values[1]
	drifted.Code = "drift"
	store.values[1] = drifted
	if _, err = writer.ImportHistoricalRadarClick(context.Background(), source, value); !errors.Is(err, radarport.ErrRadarClickHistoryConflict) {
		t.Fatalf("target drift err=%v", err)
	}

	invalid := radarClickHistoryFixture(3)
	zero := int64(0)
	invalid.RadarLinkID = &zero
	if _, err = writer.ImportHistoricalRadarClick(context.Background(), hex.EncodeToString(invalid.SourceKeyDigest[:]), invalid); !errors.Is(err, radarport.ErrRadarClickHistoryInvalid) || store.createCalls != 1 {
		t.Fatalf("invalid reference err=%v create=%d", err, store.createCalls)
	}
	invalid = radarClickHistoryFixture(4)
	invalid.Code = "bad\x00value"
	if _, err = writer.ImportHistoricalRadarClick(context.Background(), hex.EncodeToString(invalid.SourceKeyDigest[:]), invalid); !errors.Is(err, radarport.ErrRadarClickHistoryInvalid) {
		t.Fatalf("nul text err=%v", err)
	}
}

func TestHistoricalRadarClickDigestBindsPrivateAndSourceDigests(t *testing.T) {
	value := withHistoricalRadarClickID(radarClickHistoryFixture(5), 9)
	base, err := HistoricalRadarClickDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	value.SourceFieldDigest[0]++
	if changed, err := HistoricalRadarClickDigest(value); err != nil || changed == base {
		t.Fatalf("source field digest change=%x err=%v", changed, err)
	}
	value = withHistoricalRadarClickID(radarClickHistoryFixture(5), 9)
	value.QueryParamsDigest[0]++
	if changed, err := HistoricalRadarClickDigest(value); err != nil || changed == base {
		t.Fatalf("private digest change=%x err=%v", changed, err)
	}
	value = withHistoricalRadarClickID(radarClickHistoryFixture(5), 9)
	value.CreatedAt = value.CreatedAt.UTC().Add(789 * time.Nanosecond)
	if normalized, err := HistoricalRadarClickDigest(value); err != nil || normalized != base {
		t.Fatalf("time normalization digest=%x err=%v", normalized, err)
	}
}

type radarClickHistoryStoreFake struct {
	values      map[int64]radarport.HistoricalRadarClick
	createCalls int
	getCalls    int
}

func (store *radarClickHistoryStoreFake) CreateHistoricalRadarClick(_ context.Context, value radarport.HistoricalRadarClick) (radarport.HistoricalRadarClick, error) {
	if store.values == nil {
		store.values = map[int64]radarport.HistoricalRadarClick{}
	}
	store.createCalls++
	value.ID = int64(store.createCalls)
	store.values[value.ID] = value
	return value, nil
}
func (store *radarClickHistoryStoreFake) GetHistoricalRadarClick(_ context.Context, id int64) (radarport.HistoricalRadarClick, error) {
	store.getCalls++
	value, found := store.values[id]
	if !found {
		return radarport.HistoricalRadarClick{}, radarport.ErrRadarClickHistoryUnavailable
	}
	return value, nil
}

type radarClickHistoryJournalFake struct {
	values      map[string]radarport.RadarClickHistoryReceipt
	recordCalls int
}

func (journal *radarClickHistoryJournalFake) LoadRadarClickHistory(_ context.Context, kind, source string) (radarport.RadarClickHistoryReceipt, bool, error) {
	if kind != radarClickHistoryKind {
		return radarport.RadarClickHistoryReceipt{}, false, radarport.ErrRadarClickHistoryInvalid
	}
	value, found := journal.values[source]
	return value, found, nil
}
func (journal *radarClickHistoryJournalFake) RecordRadarClickHistory(_ context.Context, value radarport.RadarClickHistoryReceipt) error {
	if journal.values == nil {
		journal.values = map[string]radarport.RadarClickHistoryReceipt{}
	}
	journal.recordCalls++
	journal.values[value.SourceIdentifier] = value
	return nil
}

func radarClickHistoryFixture(first byte) radarport.HistoricalRadarClick {
	link, customer := int64(8), int64(9)
	value := radarport.HistoricalRadarClick{SourceID: int64(first), LinkSourceID: int64(first) + 10, RadarLinkID: &link, CustomerID: &customer,
		Code: "", RawStage: "opened", SourceChannel: "", TargetTypeSnapshot: "", SourceChannelSnapshot: "", ErrorCode: "", CreatedAt: time.Date(2026, 8, 28, 10, 11, 12, 123456789, time.FixedZone("x", 8*3600))}
	for index := range value.SourceKeyDigest {
		value.SourceKeyDigest[index] = first + 1
		value.SourcePayloadDigest[index] = first + 2
		value.SourceFieldDigest[index] = first + 3
		value.OpenIDDigest[index] = first + 4
		value.UnionIDDigest[index] = first + 5
		value.ExternalUserIDDigest[index] = first + 6
		value.CampaignIDDigest[index] = first + 7
		value.StaffIDDigest[index] = first + 8
		value.UserAgentDigest[index] = first + 9
		value.IPDigest[index] = first + 10
		value.PersonIDDigest[index] = first + 11
		value.IPHashDigest[index] = first + 12
		value.CampaignSnapshotDigest[index] = first + 13
		value.StaffSnapshotDigest[index] = first + 14
		value.RefererDigest[index] = first + 15
		value.QueryParamsDigest[index] = first + 16
	}
	return value
}
