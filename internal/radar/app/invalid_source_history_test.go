package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

func TestInvalidRadarLinkHistoryWriterReplayAndPrivateDrift(t *testing.T) {
	store := &invalidRadarLinkHistoryTestStore{}
	journal := &invalidRadarLinkHistoryTestJournal{}
	writer, err := NewInvalidSourceHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := invalidRadarLinkHistoryFixture(1)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	receipt, err := writer.ImportHistoricalInvalidRadarLink(context.Background(), source, value)
	if err != nil || receipt.Kind != invalidRadarLinkHistoryKind || receipt.Replayed || receipt.TargetID != 9 {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	if store.value.SourceID != -9 || store.value.Code != "" || store.value.Title != " \n" || store.value.CreatedAt.Location() != time.UTC || store.value.CreatedAt.Nanosecond()%1000 != 0 || !store.value.CreatedAt.After(store.value.UpdatedAt) {
		t.Fatalf("source fact changed: %#v", store.value)
	}
	replayed, err := writer.ImportHistoricalInvalidRadarLink(context.Background(), source, value)
	if err != nil || !replayed.Replayed || store.creates != 1 {
		t.Fatalf("replay=%+v creates=%d err=%v", replayed, store.creates, err)
	}
	value.DestinationURLDigest[0]++
	if _, err := writer.ImportHistoricalInvalidRadarLink(context.Background(), source, value); !errors.Is(err, radarport.ErrInvalidSourceHistoryConflict) {
		t.Fatalf("private drift accepted: %v", err)
	}
}
func TestInvalidRadarLinkHistoryDigestCoversRootsAndRejectsInvalidInput(t *testing.T) {
	value := invalidRadarLinkHistoryFixture(2)
	value.ID = 11
	first, err := DigestHistoricalInvalidRadarLink(value)
	if err != nil {
		t.Fatal(err)
	}
	value.RedactedRoots[0] = "changed"
	second, err := DigestHistoricalInvalidRadarLink(value)
	if err != nil || first == second {
		t.Fatalf("roots omitted: %x %x %v", first, second, err)
	}
	value = invalidRadarLinkHistoryFixture(3)
	value.QuarantineReason = "other"
	if _, err := DigestHistoricalInvalidRadarLink(value); !errors.Is(err, radarport.ErrInvalidSourceHistoryInvalid) {
		t.Fatalf("invalid reason accepted: %v", err)
	}
}
func TestInvalidRadarLinkHistoryPrivateFieldsAreNotPublicJSON(t *testing.T) {
	encoded, err := json.Marshal(invalidRadarLinkHistoryFixture(6))
	if err != nil || strings.Contains(string(encoded), "destination_url_digest") || strings.Contains(string(encoded), "private_digest") || strings.Contains(string(encoded), "redacted_roots") {
		t.Fatalf("private radar evidence leaked: %s %v", encoded, err)
	}
}
func TestInvalidRadarLinkHistoryWriterRejectsInvalidDependenciesAndSource(t *testing.T) {
	if _, err := NewInvalidSourceHistoryWriter(nil, &invalidRadarLinkHistoryTestJournal{}); !errors.Is(err, radarport.ErrInvalidSourceHistoryUnavailable) {
		t.Fatalf("nil store: %v", err)
	}
	writer, _ := NewInvalidSourceHistoryWriter(&invalidRadarLinkHistoryTestStore{}, &invalidRadarLinkHistoryTestJournal{})
	if _, err := writer.ImportHistoricalInvalidRadarLink(context.Background(), "bad", invalidRadarLinkHistoryFixture(5)); !errors.Is(err, radarport.ErrInvalidSourceHistoryInvalid) {
		t.Fatalf("bad source accepted: %v", err)
	}
}

type invalidRadarLinkHistoryTestStore struct {
	value   radarport.HistoricalInvalidRadarLink
	creates int
}

func (store *invalidRadarLinkHistoryTestStore) CreateHistoricalInvalidRadarLink(_ context.Context, value radarport.HistoricalInvalidRadarLink) (radarport.HistoricalInvalidRadarLink, error) {
	store.creates++
	value.ID = 9
	value = normalizeHistoricalInvalidRadarLink(value)
	store.value = value
	return value, nil
}
func (store *invalidRadarLinkHistoryTestStore) GetHistoricalInvalidRadarLink(_ context.Context, id int64) (radarport.HistoricalInvalidRadarLink, error) {
	if id != store.value.ID {
		return radarport.HistoricalInvalidRadarLink{}, radarport.ErrInvalidSourceHistoryUnavailable
	}
	return store.value, nil
}

type invalidRadarLinkHistoryTestJournal struct {
	receipt radarport.InvalidSourceHistoryReceipt
	found   bool
}

func (j *invalidRadarLinkHistoryTestJournal) LoadInvalidSourceHistory(_ context.Context, _, _ string) (radarport.InvalidSourceHistoryReceipt, bool, error) {
	return j.receipt, j.found, nil
}
func (j *invalidRadarLinkHistoryTestJournal) RecordInvalidSourceHistory(_ context.Context, r radarport.InvalidSourceHistoryReceipt) error {
	j.receipt = r
	j.found = true
	return nil
}
func invalidRadarLinkHistoryFixture(first byte) radarport.HistoricalInvalidRadarLink {
	value := radarport.HistoricalInvalidRadarLink{SourceID: -9, Code: "", Title: " \n", RedactedRoots: []string{"destination"}, CreatedAt: time.Date(2026, 8, 29, 10, 11, 12, 123456789, time.FixedZone("source", 8*3600)), UpdatedAt: time.Date(2026, 8, 29, 9, 11, 12, 987654321, time.FixedZone("source", 8*3600)), QuarantineReason: "invalid_radar_definition"}
	for i := range value.SourceKeyDigest {
		value.SourceKeyDigest[i] = first + 1
		value.SourcePayloadDigest[i] = first + 2
		value.SourceFieldDigest[i] = first + 3
		value.PrivateDigest[i] = first + 4
		value.DestinationURLDigest[i] = first + 5
	}
	return value
}
