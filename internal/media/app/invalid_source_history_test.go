package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

func TestInvalidAssetHistoryWriterReplayAndPrivateDrift(t *testing.T) {
	store := &invalidAssetHistoryTestStore{}
	journal := &invalidAssetHistoryTestJournal{}
	writer, err := NewInvalidSourceHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := invalidAssetHistoryFixture(1)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	receipt, err := writer.ImportHistoricalInvalidAsset(context.Background(), source, value)
	if err != nil || receipt.Kind != invalidAssetHistoryKind || receipt.Replayed || receipt.TargetID != 7 {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	if store.value.SourceID != -8 || store.value.Name != "" || store.value.FileName != " \n" || store.value.CreatedAt.After(store.value.UpdatedAt) == false || store.value.CreatedAt.Location() != time.UTC || store.value.CreatedAt.Nanosecond()%1000 != 0 {
		t.Fatalf("source fact changed: %#v", store.value)
	}
	replayed, err := writer.ImportHistoricalInvalidAsset(context.Background(), source, value)
	if err != nil || !replayed.Replayed || store.creates != 1 {
		t.Fatalf("replay=%+v creates=%d err=%v", replayed, store.creates, err)
	}
	value.PrivateDigest[0]++
	if _, err := writer.ImportHistoricalInvalidAsset(context.Background(), source, value); !errors.Is(err, mediaport.ErrInvalidSourceHistoryConflict) {
		t.Fatalf("private drift accepted: %v", err)
	}
}

func TestInvalidAssetHistoryDigestCoversRootsAndRejectsInvalidInput(t *testing.T) {
	value := invalidAssetHistoryFixture(2)
	value.ID = 11
	first, err := DigestHistoricalInvalidAsset(value)
	if err != nil {
		t.Fatal(err)
	}
	value.RedactedRoots[0] = "changed"
	second, err := DigestHistoricalInvalidAsset(value)
	if err != nil || first == second {
		t.Fatalf("roots omitted: %x %x %v", first, second, err)
	}
	value = invalidAssetHistoryFixture(3)
	value.Kind = "video"
	if _, err := DigestHistoricalInvalidAsset(value); !errors.Is(err, mediaport.ErrInvalidSourceHistoryInvalid) {
		t.Fatalf("invalid kind accepted: %v", err)
	}
	value = invalidAssetHistoryFixture(4)
	value.RedactedRoots = nil
	if _, err := DigestHistoricalInvalidAsset(value); !errors.Is(err, mediaport.ErrInvalidSourceHistoryInvalid) {
		t.Fatalf("nil roots accepted: %v", err)
	}
}

func TestInvalidAssetHistoryPrivateFieldsAreNotPublicJSON(t *testing.T) {
	value := invalidAssetHistoryFixture(6)
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) == "" || containsInvalidAssetPrivateJSON(string(encoded)) {
		t.Fatalf("private invalid asset evidence leaked: %s %v", encoded, err)
	}
}

func TestInvalidAssetHistoryWriterRejectsInvalidDependenciesAndSource(t *testing.T) {
	if _, err := NewInvalidSourceHistoryWriter(nil, &invalidAssetHistoryTestJournal{}); !errors.Is(err, mediaport.ErrInvalidSourceHistoryUnavailable) {
		t.Fatalf("nil store: %v", err)
	}
	writer, _ := NewInvalidSourceHistoryWriter(&invalidAssetHistoryTestStore{}, &invalidAssetHistoryTestJournal{})
	value := invalidAssetHistoryFixture(5)
	if _, err := writer.ImportHistoricalInvalidAsset(context.Background(), "bad", value); !errors.Is(err, mediaport.ErrInvalidSourceHistoryInvalid) {
		t.Fatalf("bad source accepted: %v", err)
	}
}

type invalidAssetHistoryTestStore struct {
	value   mediaport.HistoricalInvalidAsset
	creates int
}

func (store *invalidAssetHistoryTestStore) CreateHistoricalInvalidAsset(_ context.Context, value mediaport.HistoricalInvalidAsset) (mediaport.HistoricalInvalidAsset, error) {
	store.creates++
	value.ID = 7
	value = normalizeHistoricalInvalidAsset(value)
	store.value = value
	return value, nil
}
func (store *invalidAssetHistoryTestStore) GetHistoricalInvalidAsset(_ context.Context, id int64) (mediaport.HistoricalInvalidAsset, error) {
	if id != store.value.ID {
		return mediaport.HistoricalInvalidAsset{}, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	return store.value, nil
}

type invalidAssetHistoryTestJournal struct {
	receipt mediaport.InvalidSourceHistoryReceipt
	found   bool
}

func (journal *invalidAssetHistoryTestJournal) LoadInvalidSourceHistory(_ context.Context, _ string, _ string) (mediaport.InvalidSourceHistoryReceipt, bool, error) {
	return journal.receipt, journal.found, nil
}
func (journal *invalidAssetHistoryTestJournal) RecordInvalidSourceHistory(_ context.Context, receipt mediaport.InvalidSourceHistoryReceipt) error {
	journal.receipt = receipt
	journal.found = true
	return nil
}
func invalidAssetHistoryFixture(first byte) mediaport.HistoricalInvalidAsset {
	value := mediaport.HistoricalInvalidAsset{Kind: "image", SourceID: -8, Name: "", FileName: " \n", MIMEType: "", FileSize: -1, OriginalEnabled: false, RedactedRoots: []string{"payload", "content"}, CreatedAt: time.Date(2026, 8, 29, 10, 11, 12, 123456789, time.FixedZone("source", 8*3600)), UpdatedAt: time.Date(2026, 8, 29, 9, 11, 12, 987654321, time.FixedZone("source", 8*3600)), QuarantineReason: "invalid_static_media_definition"}
	for i := range value.SourceKeyDigest {
		value.SourceKeyDigest[i] = first + 1
		value.SourcePayloadDigest[i] = first + 2
		value.SourceFieldDigest[i] = first + 3
		value.PrivateDigest[i] = first + 4
		value.ContentDigest[i] = first + 5
	}
	return value
}

var _ = sha256.Size

func containsInvalidAssetPrivateJSON(value string) bool {
	for _, field := range []string{"source_key_digest", "source_payload_digest", "source_field_digest", "private_digest", "redacted_roots", "content_digest"} {
		if strings.Contains(value, field) {
			return true
		}
	}
	return false
}
