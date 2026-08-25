package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	ChannelAcquisitionEntrantReceiptDefaultLimit = 20
	ChannelAcquisitionEntrantReceiptMaximumLimit = 50
)

var (
	ErrInvalidChannelAcquisitionEntrantReceipt     = errors.New("invalid channel acquisition entrant receipt request")
	ErrChannelAcquisitionEntrantReceiptNotFound    = errors.New("channel acquisition entrant receipt not found")
	ErrChannelAcquisitionEntrantReceiptConflict    = errors.New("channel acquisition entrant receipt conflict")
	ErrChannelAcquisitionEntrantReceiptUnavailable = errors.New("channel acquisition entrant receipt unavailable")
)

type ChannelAcquisitionEntrantReceiptItem struct {
	ReceiptID       int64                                       `json:"receipt_id"`
	ChannelID       int64                                       `json:"channel_id,omitempty"`
	EffectID        string                                      `json:"effect_id,omitempty"`
	Kind            contactport.AcquisitionAssetKind            `json:"kind,omitempty"`
	AssetVersion    int64                                       `json:"asset_version,omitempty"`
	Status          contactport.ChannelAcquisitionEntrantStatus `json:"status"`
	CustomerID      int64                                       `json:"customer_id,omitempty"`
	CustomerEventID int64                                       `json:"customer_event_id,omitempty"`
	OccurredAt      time.Time                                   `json:"occurred_at"`
	ReconciledAt    *time.Time                                  `json:"reconciled_at,omitempty"`
	ReconcileReason string                                      `json:"reconcile_reason,omitempty"`
	CreatedAt       time.Time                                   `json:"created_at"`
	UpdatedAt       time.Time                                   `json:"updated_at"`
}

type ChannelAcquisitionEntrantReceiptListInput struct {
	ActorID   int64
	ChannelID int64
	Limit     int
	Cursor    string
}

type ChannelAcquisitionEntrantReceiptPage struct {
	Items      []ChannelAcquisitionEntrantReceiptItem `json:"items"`
	Limit      int                                    `json:"limit"`
	HasMore    bool                                   `json:"has_more"`
	NextCursor string                                 `json:"next_cursor"`
}

type ReconcileChannelAcquisitionEntrantReceiptCommand struct {
	ActorID        int64
	ChannelID      int64
	ReceiptID      int64
	EffectID       string
	CustomerID     int64
	Reason         string
	IdempotencyKey string
}

type ChannelAcquisitionEntrantReceiptStore interface {
	ListChannelAcquisitionEntrantReceipts(context.Context, int64, int64, int, int64) ([]ChannelAcquisitionEntrantReceiptItem, error)
	GetChannelAcquisitionEntrantReceipt(context.Context, int64, int64, int64) (ChannelAcquisitionEntrantReceiptItem, error)
	ReconcileChannelAcquisitionEntrantReceipt(context.Context, ReconcileChannelAcquisitionEntrantReceiptCommand, string, string) (ChannelAcquisitionEntrantReceiptItem, error)
}

type ChannelAcquisitionEntrantReceiptService struct {
	uow   platformport.UnitOfWork
	store ChannelAcquisitionEntrantReceiptStore
	codec *ChannelAcquisitionEntrantReceiptCursorCodec
}

func NewChannelAcquisitionEntrantReceiptService(uow platformport.UnitOfWork, store ChannelAcquisitionEntrantReceiptStore, codec *ChannelAcquisitionEntrantReceiptCursorCodec) (*ChannelAcquisitionEntrantReceiptService, error) {
	if channelAcquisitionAssetNil(uow) || channelAcquisitionAssetNil(store) || !channelAcquisitionEntrantReceiptCursorReady(codec) {
		return nil, ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	return &ChannelAcquisitionEntrantReceiptService{uow: uow, store: store, codec: codec}, nil
}

func (service *ChannelAcquisitionEntrantReceiptService) List(ctx context.Context, input ChannelAcquisitionEntrantReceiptListInput) (ChannelAcquisitionEntrantReceiptPage, error) {
	if service == nil || ctx == nil || ctx.Err() != nil || channelAcquisitionAssetNil(service.uow) || channelAcquisitionAssetNil(service.store) || !channelAcquisitionEntrantReceiptCursorReady(service.codec) {
		return ChannelAcquisitionEntrantReceiptPage{}, ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	limit := input.Limit
	if limit == 0 {
		limit = ChannelAcquisitionEntrantReceiptDefaultLimit
	}
	if input.ActorID < 1 || input.ChannelID < 1 || limit < 1 || limit > ChannelAcquisitionEntrantReceiptMaximumLimit || len(input.Cursor) > channelAcquisitionEntrantReceiptMaximumCursorLength {
		return ChannelAcquisitionEntrantReceiptPage{}, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	after := int64(0)
	var err error
	if input.Cursor != "" {
		after, err = service.codec.Decode(input.Cursor, input.ActorID, input.ChannelID)
		if err != nil {
			return ChannelAcquisitionEntrantReceiptPage{}, err
		}
	}
	var records []ChannelAcquisitionEntrantReceiptItem
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		records, storeErr = service.store.ListChannelAcquisitionEntrantReceipts(txCtx, input.ActorID, input.ChannelID, limit+1, after)
		return storeErr
	})
	if err != nil {
		return ChannelAcquisitionEntrantReceiptPage{}, entrantReceiptServiceError(err)
	}
	if len(records) > limit+1 {
		return ChannelAcquisitionEntrantReceiptPage{}, ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	page := ChannelAcquisitionEntrantReceiptPage{Items: records, Limit: limit, HasMore: hasMore}
	if hasMore {
		page.NextCursor, err = service.codec.Encode(input.ActorID, input.ChannelID, records[len(records)-1].ReceiptID)
		if err != nil {
			return ChannelAcquisitionEntrantReceiptPage{}, ErrChannelAcquisitionEntrantReceiptUnavailable
		}
	}
	return page, nil
}

func (service *ChannelAcquisitionEntrantReceiptService) Get(ctx context.Context, actorID, channelID, receiptID int64) (ChannelAcquisitionEntrantReceiptItem, error) {
	if service == nil || ctx == nil || actorID < 1 || channelID < 1 || receiptID < 1 {
		return ChannelAcquisitionEntrantReceiptItem{}, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	var item ChannelAcquisitionEntrantReceiptItem
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		item, storeErr = service.store.GetChannelAcquisitionEntrantReceipt(txCtx, actorID, channelID, receiptID)
		return storeErr
	})
	if err != nil {
		return ChannelAcquisitionEntrantReceiptItem{}, entrantReceiptServiceError(err)
	}
	return item, nil
}

func (service *ChannelAcquisitionEntrantReceiptService) Reconcile(ctx context.Context, command ReconcileChannelAcquisitionEntrantReceiptCommand) (ChannelAcquisitionEntrantReceiptItem, error) {
	trimmedReason := strings.TrimSpace(command.Reason)
	if service == nil || ctx == nil || command.ActorID < 1 || command.ChannelID < 1 || command.ReceiptID < 1 || command.CustomerID < 1 ||
		channelAcquisitionAssetNumericEffectIDMust(command.EffectID) < 1 || command.Reason == "" || command.Reason != trimmedReason || len(command.Reason) > 200 || len(command.IdempotencyKey) < 8 || len(command.IdempotencyKey) > 200 || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey {
		return ChannelAcquisitionEntrantReceiptItem{}, ErrInvalidChannelAcquisitionEntrantReceipt
	}
	keyDigest := channelAcquisitionEntrantReceiptDigest("key", strconv.FormatInt(command.ActorID, 10), command.IdempotencyKey)
	payloadDigest := channelAcquisitionEntrantReceiptDigest("payload", strconv.FormatInt(command.ChannelID, 10), strconv.FormatInt(command.ReceiptID, 10), command.EffectID, strconv.FormatInt(command.CustomerID, 10), command.Reason)
	var item ChannelAcquisitionEntrantReceiptItem
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		item, storeErr = service.store.ReconcileChannelAcquisitionEntrantReceipt(txCtx, command, keyDigest, payloadDigest)
		return storeErr
	})
	if err != nil {
		return ChannelAcquisitionEntrantReceiptItem{}, entrantReceiptServiceError(err)
	}
	return item, nil
}

func channelAcquisitionAssetNumericEffectIDMust(value string) int64 {
	if !strings.HasPrefix(value, "eer_") {
		return 0
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "eer_"), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func channelAcquisitionEntrantReceiptDigest(label string, parts ...string) string {
	sum := sha256.Sum256([]byte("contact.acquisition.entrant.receipt.v1\x00" + label + "\x00" + strings.Join(parts, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func entrantReceiptServiceError(err error) error {
	switch {
	case errors.Is(err, ErrChannelAcquisitionEntrantReceiptNotFound):
		return ErrChannelAcquisitionEntrantReceiptNotFound
	case errors.Is(err, ErrChannelAcquisitionEntrantReceiptConflict):
		return ErrChannelAcquisitionEntrantReceiptConflict
	default:
		return errors.Join(ErrChannelAcquisitionEntrantReceiptUnavailable, err)
	}
}
