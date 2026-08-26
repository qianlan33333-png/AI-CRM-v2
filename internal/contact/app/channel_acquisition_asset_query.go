package app

import (
	"context"
	"errors"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	ChannelAcquisitionAssetDefaultLimit = 20
	ChannelAcquisitionAssetMaximumLimit = 50
)

type ChannelAcquisitionAssetItem struct {
	EffectID             string
	ChannelID            int64
	Kind                 contactport.AcquisitionAssetKind
	AssetVersion         int64
	SupersedesVersion    int64
	State                eer.State
	AcceptReceiptID      string
	QueueReceiptID       string
	AttemptReceiptDigest eer.Digest
	ReconcileReceiptID   string
	AssetURL             string
	EntrantReady         bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ReconciledAt         *time.Time
}

type ChannelAcquisitionAssetListInput struct {
	ChannelID int64
	Limit     int
	Cursor    string
}

type ChannelAcquisitionAssetPage struct {
	Items      []ChannelAcquisitionAssetItem
	Limit      int
	HasMore    bool
	NextCursor string
}

type ChannelAcquisitionAssetReadStore interface {
	ReadChannelAcquisitionAssetChannel(context.Context, int64) (bool, error)
	GetChannelAcquisitionAsset(context.Context, int64, string) (ChannelAcquisitionAssetItem, error)
	ListChannelAcquisitionAssets(context.Context, int64, int64, int) ([]ChannelAcquisitionAssetItem, error)
}

type ChannelAcquisitionAssetQueryService struct {
	uow   platformport.UnitOfWork
	store ChannelAcquisitionAssetReadStore
	codec *ChannelAcquisitionAssetCursorCodec
}

func NewChannelAcquisitionAssetQueryService(uow platformport.UnitOfWork, store ChannelAcquisitionAssetReadStore, codec *ChannelAcquisitionAssetCursorCodec) (*ChannelAcquisitionAssetQueryService, error) {
	if channelAcquisitionAssetNil(uow) || channelAcquisitionAssetNil(store) || !channelAcquisitionAssetCursorReady(codec) {
		return nil, ErrChannelAcquisitionAssetUnavailable
	}
	return &ChannelAcquisitionAssetQueryService{uow: uow, store: store, codec: codec}, nil
}

func (service *ChannelAcquisitionAssetQueryService) Get(ctx context.Context, channelID int64, effectID string) (ChannelAcquisitionAssetItem, error) {
	if !service.ready(ctx) || channelID < 1 || effectID == "" {
		return ChannelAcquisitionAssetItem{}, ErrChannelAcquisitionAssetNotFound
	}
	var item ChannelAcquisitionAssetItem
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		item, err = service.store.GetChannelAcquisitionAsset(tx, channelID, effectID)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrChannelAcquisitionAssetNotFound) {
			return ChannelAcquisitionAssetItem{}, ErrChannelAcquisitionAssetNotFound
		}
		return ChannelAcquisitionAssetItem{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	if !validChannelAcquisitionAssetItem(item, channelID) || item.EffectID != effectID {
		return ChannelAcquisitionAssetItem{}, ErrChannelAcquisitionAssetNotFound
	}
	return item, nil
}

func (service *ChannelAcquisitionAssetQueryService) List(ctx context.Context, input ChannelAcquisitionAssetListInput) (ChannelAcquisitionAssetPage, error) {
	if !service.ready(ctx) {
		return ChannelAcquisitionAssetPage{}, ErrChannelAcquisitionAssetUnavailable
	}
	limit := input.Limit
	if limit == 0 {
		limit = ChannelAcquisitionAssetDefaultLimit
	}
	if input.ChannelID < 1 || limit < 1 || limit > ChannelAcquisitionAssetMaximumLimit || len(input.Cursor) > channelAcquisitionAssetMaximumCursorLength {
		return ChannelAcquisitionAssetPage{}, ErrInvalidChannelAcquisitionAsset
	}
	var after int64
	var err error
	if input.Cursor != "" {
		after, err = service.codec.Decode(input.Cursor, input.ChannelID)
		if err != nil {
			return ChannelAcquisitionAssetPage{}, ErrInvalidChannelAcquisitionAsset
		}
	}
	var records []ChannelAcquisitionAssetItem
	err = service.uow.Within(ctx, func(tx context.Context) error {
		exists, storeErr := service.store.ReadChannelAcquisitionAssetChannel(tx, input.ChannelID)
		if storeErr != nil {
			return storeErr
		}
		if !exists {
			return ErrChannelAcquisitionAssetNotFound
		}
		records, storeErr = service.store.ListChannelAcquisitionAssets(tx, input.ChannelID, after, limit+1)
		return storeErr
	})
	if err != nil {
		if errors.Is(err, ErrChannelAcquisitionAssetNotFound) {
			return ChannelAcquisitionAssetPage{}, ErrChannelAcquisitionAssetNotFound
		}
		return ChannelAcquisitionAssetPage{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	if len(records) > limit+1 || !validChannelAcquisitionAssetItems(records, input.ChannelID, after) {
		return ChannelAcquisitionAssetPage{}, ErrChannelAcquisitionAssetUnavailable
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	page := ChannelAcquisitionAssetPage{Items: records, Limit: limit, HasMore: hasMore}
	if hasMore {
		page.NextCursor, err = service.codec.Encode(input.ChannelID, records[len(records)-1].EffectID)
		if err != nil {
			return ChannelAcquisitionAssetPage{}, ErrChannelAcquisitionAssetUnavailable
		}
	}
	return page, nil
}

func (service *ChannelAcquisitionAssetQueryService) ready(ctx context.Context) bool {
	return service != nil && ctx != nil && ctx.Err() == nil && !channelAcquisitionAssetNil(service.uow) && !channelAcquisitionAssetNil(service.store) && channelAcquisitionAssetCursorReady(service.codec)
}

func validChannelAcquisitionAssetItems(items []ChannelAcquisitionAssetItem, channelID, after int64) bool {
	var previous int64
	for index, item := range items {
		if !validChannelAcquisitionAssetItem(item, channelID) {
			return false
		}
		id, err := channelAcquisitionAssetNumericEffectID(item.EffectID)
		if err != nil || after > 0 && id >= after || index > 0 && id >= previous {
			return false
		}
		previous = id
	}
	return true
}

func validChannelAcquisitionAssetItem(item ChannelAcquisitionAssetItem, channelID int64) bool {
	if item.ChannelID != channelID || item.AssetVersion < 1 || item.SupersedesVersion < 0 || item.AssetVersion <= item.SupersedesVersion ||
		item.AcceptReceiptID == "" || item.EntrantReady || item.CreatedAt.IsZero() || item.UpdatedAt.Before(item.CreatedAt) {
		return false
	}
	switch item.Kind {
	case contactport.AcquisitionAssetQRCode, contactport.AcquisitionAssetLink:
	default:
		return false
	}
	switch item.State {
	case eer.StateAccepted, eer.StateQueued, channelAcquisitionAssetStateAttempted, eer.StateExecuted, eer.StateFinalFailed, eer.StateOutcomeUnknown, eer.StateReconciled:
	default:
		return false
	}
	if item.AssetURL != "" && (item.State != eer.StateExecuted || !validChannelAcquisitionAssetURL(item.AssetURL)) {
		return false
	}
	return true
}
