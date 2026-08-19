// Package app owns Events' read-only delivery-lineage projection.
package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const deliveryLineageMaximumLimit int32 = 1_000_101

var ErrDeliveryLineageUnavailable = errors.New("event delivery lineage unavailable")

// DeliveryLineageStoreRecord remains an Events-only input. The EventID and
// Consumer never cross the public port; they are transformed into a keyed ID.
type DeliveryLineageStoreRecord struct {
	EventID            eventport.EventID
	Consumer           string
	State              eventport.DeliveryStatus
	AttemptCount       int32
	UpdatedAt          time.Time
	SameTimestampCount int64
}

type deliveryLineageStore interface {
	ListDeliveryLineage(context.Context, int32) ([]DeliveryLineageStoreRecord, error)
}

type DeliveryLineageReader struct {
	uow   platformport.UnitOfWork
	store deliveryLineageStore
	key   [32]byte
}

var _ eventport.DeliveryLineageReader = (*DeliveryLineageReader)(nil)

func NewDeliveryLineageReader(uow platformport.UnitOfWork, store deliveryLineageStore, key []byte) (*DeliveryLineageReader, error) {
	if uow == nil || store == nil || len(key) != sha256.Size {
		return nil, ErrDeliveryLineageUnavailable
	}
	reader := &DeliveryLineageReader{uow: uow, store: store}
	copy(reader.key[:], key)
	return reader, nil
}

func (reader *DeliveryLineageReader) ListDeliveryLineage(ctx context.Context, limit int32) (eventport.DeliveryLineagePage, error) {
	if ctx == nil || ctx.Err() != nil || limit < 1 || limit > deliveryLineageMaximumLimit || reader == nil || reader.uow == nil || reader.store == nil {
		return eventport.DeliveryLineagePage{}, ErrDeliveryLineageUnavailable
	}
	var records []DeliveryLineageStoreRecord
	if err := reader.uow.Within(ctx, func(txCtx context.Context) error {
		var err error
		records, err = reader.store.ListDeliveryLineage(txCtx, limit)
		return err
	}); err != nil {
		return eventport.DeliveryLineagePage{}, errors.Join(ErrDeliveryLineageUnavailable, err)
	}
	if len(records) > int(limit) || !validDeliveryLineageRecords(records) {
		return eventport.DeliveryLineagePage{}, ErrDeliveryLineageUnavailable
	}
	page := eventport.DeliveryLineagePage{Items: make([]eventport.DeliveryLineageItem, 0, len(records)), Complete: true}
	for _, record := range records {
		page.Items = append(page.Items, eventport.DeliveryLineageItem{
			LineageID:     deliveryLineageID(reader.key[:], record.EventID, record.Consumer),
			InternalState: string(record.State), AttemptCount: record.AttemptCount, UpdatedAt: record.UpdatedAt.UTC(),
		})
	}
	sort.Slice(page.Items, func(left, right int) bool {
		if !page.Items[left].UpdatedAt.Equal(page.Items[right].UpdatedAt) {
			return page.Items[left].UpdatedAt.After(page.Items[right].UpdatedAt)
		}
		return page.Items[left].LineageID < page.Items[right].LineageID
	})
	return page, nil
}

func deliveryLineageID(key []byte, eventID eventport.EventID, consumer string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("delivery-lineage:v1\x00"))
	_, _ = mac.Write([]byte(strconv.FormatInt(int64(eventID), 10)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(consumer))
	return "event-delivery:v1:" + hex.EncodeToString(mac.Sum(nil))
}

func validDeliveryLineageRecords(records []DeliveryLineageStoreRecord) bool {
	for index, record := range records {
		if record.EventID <= 0 || record.Consumer == "" || strings.TrimSpace(record.Consumer) != record.Consumer || len(record.Consumer) > 200 || !validDeliveryStatus(record.State) || record.AttemptCount < 0 || record.UpdatedAt.IsZero() || record.SameTimestampCount < 1 {
			return false
		}
		if index > 0 && record.UpdatedAt.After(records[index-1].UpdatedAt) {
			return false
		}
	}
	for start := 0; start < len(records); {
		end := start + 1
		for end < len(records) && records[end].UpdatedAt.Equal(records[start].UpdatedAt) {
			end++
		}
		groupSize := int64(end - start)
		for _, record := range records[start:end] {
			if record.SameTimestampCount != groupSize {
				return false
			}
		}
		start = end
	}
	return true
}

func validDeliveryStatus(status eventport.DeliveryStatus) bool {
	switch status {
	case eventport.DeliveryPending, eventport.DeliveryProcessing, eventport.DeliveryCompleted, eventport.DeliveryFinalFailed, eventport.DeliveryOutcomeUnknown:
		return true
	default:
		return false
	}
}
