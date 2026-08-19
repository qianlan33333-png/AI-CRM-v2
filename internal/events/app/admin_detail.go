package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

var (
	ErrAdminDetailUnavailable = errors.New("internal event observation unavailable")
	ErrAdminDetailNotFound    = errors.New("internal event not found")
)

type AdminDetailService struct {
	repository eventport.AdminDetailRepository
	now        func() time.Time
}

type AdminDetailResult struct {
	Item       AdminReadListItem
	ObservedAt time.Time
}

func NewAdminDetailService(repository eventport.AdminDetailRepository, now func() time.Time) *AdminDetailService {
	if now == nil {
		now = time.Now
	}
	return &AdminDetailService{repository: repository, now: now}
}

func (service *AdminDetailService) Get(ctx context.Context, eventID eventport.EventID) (AdminDetailResult, error) {
	if service == nil || service.repository == nil || eventID <= 0 {
		return AdminDetailResult{}, ErrAdminDetailUnavailable
	}
	snapshot, err := service.repository.Read(ctx, eventID)
	if err != nil {
		return AdminDetailResult{}, errors.Join(ErrAdminDetailUnavailable, err)
	}
	if !snapshot.Found {
		if len(snapshot.Deliveries) != 0 {
			return AdminDetailResult{}, ErrAdminDetailUnavailable
		}
		return AdminDetailResult{}, ErrAdminDetailNotFound
	}
	if snapshot.Event.EventID != eventID || !validAdminDetailEvent(snapshot.Event) {
		return AdminDetailResult{}, ErrAdminDetailUnavailable
	}
	if snapshot.Deliveries == nil {
		snapshot.Deliveries = make([]eventport.AdminReadDelivery, 0)
	}
	seenConsumers := make(map[string]struct{}, len(snapshot.Deliveries))
	deliveries := make([]eventport.AdminReadDelivery, 0, len(snapshot.Deliveries))
	for _, delivery := range snapshot.Deliveries {
		if delivery.EventID != eventID || !validAdminDetailDelivery(delivery) || !adminReadBindingMatches(snapshot.Event.EventType, delivery.Consumer) {
			return AdminDetailResult{}, ErrAdminDetailUnavailable
		}
		if _, exists := seenConsumers[delivery.Consumer]; exists {
			return AdminDetailResult{}, ErrAdminDetailUnavailable
		}
		seenConsumers[delivery.Consumer] = struct{}{}
		deliveries = append(deliveries, delivery)
	}
	sort.SliceStable(deliveries, func(left, right int) bool {
		leftOrder := adminReadConsumerOrder(deliveries[left].Consumer)
		rightOrder := adminReadConsumerOrder(deliveries[right].Consumer)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return deliveries[left].Consumer < deliveries[right].Consumer
	})

	observedAt := service.now().UTC()
	if observedAt.IsZero() {
		return AdminDetailResult{}, ErrAdminDetailUnavailable
	}
	item := AdminReadListItem{
		EventID: snapshot.Event.EventID, EventType: snapshot.Event.EventType,
		OccurredAt: snapshot.Event.OccurredAt.UTC(), Dispatched: snapshot.Event.Dispatched,
		Deliveries: make([]AdminReadListDelivery, 0, len(deliveries)),
	}
	for _, delivery := range deliveries {
		item.Deliveries = append(item.Deliveries, AdminReadListDelivery{
			Consumer: delivery.Consumer, Status: delivery.Status, AttemptCount: delivery.AttemptCount,
			CompletedAt: cloneAdminReadTime(delivery.CompletedAt),
		})
	}
	return AdminDetailResult{Item: item, ObservedAt: observedAt}, nil
}

func validAdminDetailEvent(event eventport.AdminReadEvent) bool {
	return event.EventID > 0 && validAdminDetailText(event.EventType) && !event.OccurredAt.IsZero()
}

func validAdminDetailDelivery(delivery eventport.AdminReadDelivery) bool {
	if delivery.EventID <= 0 || delivery.AttemptCount < 0 || !validAdminReadStatus(delivery.Status) {
		return false
	}
	terminal := delivery.Status == string(eventport.DeliveryCompleted) || delivery.Status == string(eventport.DeliveryFinalFailed) || delivery.Status == string(eventport.DeliveryOutcomeUnknown)
	if terminal {
		return delivery.CompletedAt != nil && !delivery.CompletedAt.IsZero()
	}
	return delivery.CompletedAt == nil
}

func validAdminDetailText(value string) bool {
	if value == "" || !utf8.ValidString(value) || len(value) > 200 || strings.Trim(value, " \t\r\n\v\f") != value {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}
