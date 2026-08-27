package media

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdaptV1MiniProgramLibraryDropsProviderMaterialAndPreservesStaticTimes(t *testing.T) {
	created := time.Date(2026, 7, 1, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	updated := created.Add(time.Hour)
	expires := updated.Add(time.Hour)
	definition, err := AdaptV1MiniProgramLibrary(V1MiniProgramLibraryRow{ID: 91, Name: "历史卡片", AppID: "wx-history", PagePath: "pages/history", Title: "历史标题",
		ThumbnailImageURL: "https://expired.example/cover", ThumbnailImageBase64: "base64", ThumbnailMediaID: "expired-media", ThumbnailMediaExpiresAt: &expires,
		Enabled: true, CreatedAt: created, UpdatedAt: updated}, "public/miniprogram_library/91", [32]byte{1}, 7)
	if err != nil {
		t.Fatal(err)
	}
	item := definition.Item
	if item.Enabled || !definition.ProviderMaterialDropped || item.ThumbnailImageURL != "" || item.ThumbnailImageBase64 != "" || item.ThumbnailImageID != nil || item.ThumbnailMediaID != "" || item.ThumbnailMediaExpiresAt != nil {
		t.Fatalf("unsafe target definition = %#v", definition)
	}
	if !item.CreatedAt.Equal(created) || !item.UpdatedAt.Equal(updated) || item.CreatedBy != 7 || item.UpdatedBy != 7 || item.Name != "历史卡片" || item.AppID != "wx-history" || item.PagePath != "pages/history" || item.Title != "历史标题" {
		t.Fatalf("definition = %#v", definition)
	}
}

func TestAdaptV1MiniProgramLibraryRejectsInvalidStaticDefinition(t *testing.T) {
	stamp := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for _, source := range []V1MiniProgramLibraryRow{
		{ID: 0, AppID: "wx", PagePath: "pages/a", Title: "a", CreatedAt: stamp, UpdatedAt: stamp},
		{ID: 1, AppID: "wx", PagePath: "pages/a", Title: "a", CreatedAt: stamp, UpdatedAt: stamp.Add(-time.Second)},
		{ID: 1, AppID: "wx", PagePath: "pages/a", Title: "a", CreatedAt: time.Time{}, UpdatedAt: stamp},
	} {
		if _, err := AdaptV1MiniProgramLibrary(source, "public/miniprogram_library/1", [32]byte{1}, 7); !errors.Is(err, ErrHistoricalMiniProgramInvalid) {
			t.Fatalf("source=%#v err=%v", source, err)
		}
	}
	if _, err := AdaptV1MiniProgramLibrary(V1MiniProgramLibraryRow{ID: 1, AppID: "wx", PagePath: "pages/a", Title: "a", CreatedAt: stamp, UpdatedAt: stamp}, " public/miniprogram_library/1", [32]byte{1}, 7); !errors.Is(err, ErrHistoricalMiniProgramInvalid) {
		t.Fatalf("source key error = %v", err)
	}
}

func TestHistoricalMiniProgramWriterReplaysAndNeverStartsExternalWork(t *testing.T) {
	store := &historicalMiniProgramMemoryStore{}
	journal := &historicalMiniProgramMemoryJournal{receipts: map[string]HistoricalMiniProgramReceipt{}}
	writer, err := NewHistoricalMiniProgramWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	definition := historicalMiniProgramFixture(t)
	receipt, err := writer.Import(context.Background(), definition)
	if err != nil || receipt.Replayed || receipt.TargetMiniProgramID != 1 || len(store.definitions) != 1 || journal.records != 1 {
		t.Fatalf("receipt/store/journal = %#v/%#v/%d/%d", receipt, err, len(store.definitions), journal.records)
	}
	if store.providerCalls != 0 || store.operationReceipts != 0 || store.events != 0 || store.thumbnailCache != 0 {
		t.Fatalf("side effects = %#v", store)
	}
	replayed, err := writer.Import(context.Background(), definition)
	if err != nil || !replayed.Replayed || len(store.definitions) != 1 || journal.records != 1 {
		t.Fatalf("replay/store/journal = %#v/%d/%d err=%v", replayed, len(store.definitions), journal.records, err)
	}
}

func TestHistoricalMiniProgramWriterClosesDigestDriftAndTargetConflict(t *testing.T) {
	definition := historicalMiniProgramFixture(t)
	t.Run("digest drift", func(t *testing.T) {
		journal := &historicalMiniProgramMemoryJournal{receipts: map[string]HistoricalMiniProgramReceipt{definition.SourceIdentifier: {SourceIdentifier: definition.SourceIdentifier, SourceID: definition.SourceID, PayloadDigest: definition.PayloadDigest, TargetMiniProgramID: 1, ProviderMaterialDropped: definition.ProviderMaterialDropped}}}
		writer, _ := NewHistoricalMiniProgramWriter(&historicalMiniProgramMemoryStore{}, journal)
		definition.PayloadDigest[0]++
		if _, err := writer.Import(context.Background(), definition); !errors.Is(err, ErrHistoricalMiniProgramConflict) {
			t.Fatalf("digest drift error = %v", err)
		}
	})
	t.Run("existing target", func(t *testing.T) {
		store := &historicalMiniProgramMemoryStore{insertErr: ErrHistoricalMiniProgramConflict}
		writer, _ := NewHistoricalMiniProgramWriter(store, &historicalMiniProgramMemoryJournal{receipts: map[string]HistoricalMiniProgramReceipt{}})
		if _, err := writer.Import(context.Background(), definition); !errors.Is(err, ErrHistoricalMiniProgramConflict) || len(store.definitions) != 0 {
			t.Fatalf("target conflict error/store = %v/%#v", err, store.definitions)
		}
	})
}

type historicalMiniProgramMemoryStore struct {
	definitions                                              []HistoricalMiniProgramDefinition
	insertErr                                                error
	providerCalls, operationReceipts, events, thumbnailCache int
}

func (store *historicalMiniProgramMemoryStore) InsertHistoricalMiniProgram(_ context.Context, definition HistoricalMiniProgramDefinition) (int64, error) {
	if store.insertErr != nil {
		return 0, store.insertErr
	}
	store.definitions = append(store.definitions, definition)
	return int64(len(store.definitions)), nil
}

type historicalMiniProgramMemoryJournal struct {
	receipts map[string]HistoricalMiniProgramReceipt
	records  int
}

func (journal *historicalMiniProgramMemoryJournal) LoadHistoricalMiniProgram(_ context.Context, source string) (HistoricalMiniProgramReceipt, bool, error) {
	receipt, found := journal.receipts[source]
	return receipt, found, nil
}

func (journal *historicalMiniProgramMemoryJournal) RecordHistoricalMiniProgram(_ context.Context, receipt HistoricalMiniProgramReceipt) error {
	journal.receipts[receipt.SourceIdentifier] = receipt
	journal.records++
	return nil
}

func historicalMiniProgramFixture(t *testing.T) HistoricalMiniProgramDefinition {
	t.Helper()
	stamp := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	definition, err := AdaptV1MiniProgramLibrary(V1MiniProgramLibraryRow{ID: 18, Name: "历史素材", AppID: "wx-history", PagePath: "pages/history", Title: "历史素材", CreatedAt: stamp, UpdatedAt: stamp}, "public/miniprogram_library/18", [32]byte{9}, 7)
	if err != nil {
		t.Fatal(err)
	}
	return definition
}
