package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type acquisitionAssetReadStore struct {
	exists bool
	items  []ChannelAcquisitionAssetItem
}

func (store *acquisitionAssetReadStore) ReadChannelAcquisitionAssetChannel(context.Context, int64) (bool, error) {
	return store.exists, nil
}
func (store *acquisitionAssetReadStore) GetChannelAcquisitionAsset(_ context.Context, channelID int64, effectID string) (ChannelAcquisitionAssetItem, error) {
	for _, item := range store.items {
		if item.ChannelID == channelID && item.EffectID == effectID {
			return item, nil
		}
	}
	return ChannelAcquisitionAssetItem{}, ErrChannelAcquisitionAssetNotFound
}
func (store *acquisitionAssetReadStore) ListChannelAcquisitionAssets(_ context.Context, _ int64, after int64, limit int) ([]ChannelAcquisitionAssetItem, error) {
	result := make([]ChannelAcquisitionAssetItem, 0, limit)
	for _, item := range store.items {
		id, _ := channelAcquisitionAssetNumericEffectID(item.EffectID)
		if after == 0 || id < after {
			result = append(result, item)
		}
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func TestCH02AcquisitionAssetQueryUsesChannelBoundOpaqueCursorAndSafeProjection(t *testing.T) {
	now := time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC)
	item := func(id int64) ChannelAcquisitionAssetItem {
		return ChannelAcquisitionAssetItem{EffectID: "eer_" + strconv.FormatInt(id, 10), ChannelID: 41, Kind: contactport.AcquisitionAssetQRCode, AssetVersion: id, SupersedesVersion: id - 1, State: eer.StateQueued, AcceptReceiptID: "eerop_1", QueueReceiptID: "eerop_2", CreatedAt: now, UpdatedAt: now}
	}
	executed := item(3)
	executed.State = eer.StateExecuted
	executed.AssetURL = "https://work.weixin.qq.com/q/config-safe"
	store := &acquisitionAssetReadStore{exists: true, items: []ChannelAcquisitionAssetItem{executed, item(2), item(1)}}
	codec, err := NewChannelAcquisitionAssetCursorCodec([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return now }
	service, err := NewChannelAcquisitionAssetQueryService(&acquisitionAssetTestUoW{}, store, codec)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.List(context.Background(), ChannelAcquisitionAssetListInput{ChannelID: 41, Limit: 2})
	if err != nil || !page.HasMore || page.NextCursor == "" || strings.Contains(page.NextCursor, "eer_2") || len(page.Items) != 2 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if _, err = service.List(context.Background(), ChannelAcquisitionAssetListInput{ChannelID: 42, Limit: 2, Cursor: page.NextCursor}); !errors.Is(err, ErrInvalidChannelAcquisitionAsset) {
		t.Fatalf("cross-channel cursor err=%v", err)
	}
	got, err := service.Get(context.Background(), 41, "eer_3")
	if err != nil || got.EffectID != "eer_3" || got.AssetURL != executed.AssetURL || got.EntrantReady {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
