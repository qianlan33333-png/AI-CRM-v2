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

var ErrAdminReadUnavailable = errors.New("internal event observation unavailable")

type AdminReadService struct {
	repository eventport.AdminReadRepository
	now        func() time.Time
}

type AdminReadListItem struct {
	EventID    eventport.EventID
	EventType  string
	OccurredAt time.Time
	Dispatched bool
	Deliveries []AdminReadListDelivery
}

type AdminReadListDelivery struct {
	Consumer     string
	Status       string
	AttemptCount int32
	CompletedAt  *time.Time
}

type AdminReadListResult struct {
	Items      []AdminReadListItem
	Total      int64
	Limit      int64
	Offset     int64
	ObservedAt time.Time
}

type AdminReadDeliveryCounts struct {
	Pending        int64
	Processing     int64
	Completed      int64
	FinalFailed    int64
	OutcomeUnknown int64
}

type AdminReadDiagnosticResult struct {
	Filters                eventport.AdminReadQuery
	EventCount             int64
	UndispatchedEventCount int64
	DeliveryCounts         AdminReadDeliveryCounts
	ConsumerRegistry       []eventport.AdminReadBinding
	ObservedAt             time.Time
	ObservedDomains        []string
	UnobservedDomains      []string
}

func NewAdminReadService(repository eventport.AdminReadRepository, now func() time.Time) *AdminReadService {
	if now == nil {
		now = time.Now
	}
	return &AdminReadService{repository: repository, now: now}
}

func (service *AdminReadService) List(ctx context.Context, query eventport.AdminReadQuery) (AdminReadListResult, error) {
	if service == nil || service.repository == nil || !validAdminReadQuery(query, true) {
		return AdminReadListResult{}, ErrAdminReadUnavailable
	}
	snapshot, err := service.repository.Read(ctx, query.EventType)
	if err != nil {
		return AdminReadListResult{}, errors.Join(ErrAdminReadUnavailable, err)
	}
	selected, err := selectAdminReadEvents(snapshot, query)
	if err != nil {
		return AdminReadListResult{}, err
	}
	observedAt := service.now().UTC()
	if observedAt.IsZero() {
		return AdminReadListResult{}, ErrAdminReadUnavailable
	}
	result := AdminReadListResult{
		Items:      make([]AdminReadListItem, 0),
		Total:      int64(len(selected)),
		Limit:      query.Limit,
		Offset:     query.Offset,
		ObservedAt: observedAt,
	}
	start := query.Offset
	if start >= int64(len(selected)) {
		return result, nil
	}
	end := start + query.Limit
	if end > int64(len(selected)) {
		end = int64(len(selected))
	}
	result.Items = make([]AdminReadListItem, 0, end-start)
	for _, event := range selected[start:end] {
		item := AdminReadListItem{
			EventID: event.event.EventID, EventType: event.event.EventType, OccurredAt: event.event.OccurredAt.UTC(),
			Dispatched: event.event.Dispatched, Deliveries: make([]AdminReadListDelivery, 0, len(event.deliveries)),
		}
		for _, delivery := range event.deliveries {
			item.Deliveries = append(item.Deliveries, AdminReadListDelivery{
				Consumer: delivery.Consumer, Status: delivery.Status, AttemptCount: delivery.AttemptCount,
				CompletedAt: cloneAdminReadTime(delivery.CompletedAt),
			})
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (service *AdminReadService) Diagnostics(ctx context.Context, query eventport.AdminReadQuery) (AdminReadDiagnosticResult, error) {
	if service == nil || service.repository == nil || !validAdminReadQuery(query, false) {
		return AdminReadDiagnosticResult{}, ErrAdminReadUnavailable
	}
	snapshot, err := service.repository.Read(ctx, query.EventType)
	if err != nil {
		return AdminReadDiagnosticResult{}, errors.Join(ErrAdminReadUnavailable, err)
	}
	selected, err := selectAdminReadEvents(snapshot, query)
	if err != nil {
		return AdminReadDiagnosticResult{}, err
	}
	observedAt := service.now().UTC()
	if observedAt.IsZero() {
		return AdminReadDiagnosticResult{}, ErrAdminReadUnavailable
	}
	result := AdminReadDiagnosticResult{
		Filters: query, EventCount: int64(len(selected)), ConsumerRegistry: eventport.AdminReadBindings(),
		ObservedAt: observedAt, ObservedDomains: []string{"event_log", "event_deliveries"},
		UnobservedDomains: []string{"river_queue", "outbound_provider", "external_delivery"},
	}
	for _, selectedEvent := range selected {
		if !selectedEvent.event.Dispatched {
			result.UndispatchedEventCount++
		}
		for _, delivery := range selectedEvent.deliveries {
			if !adminReadDeliveryMatches(delivery, query.Consumer, query.Status) {
				continue
			}
			switch delivery.Status {
			case string(eventport.DeliveryPending):
				result.DeliveryCounts.Pending++
			case string(eventport.DeliveryProcessing):
				result.DeliveryCounts.Processing++
			case string(eventport.DeliveryCompleted):
				result.DeliveryCounts.Completed++
			case string(eventport.DeliveryFinalFailed):
				result.DeliveryCounts.FinalFailed++
			case string(eventport.DeliveryOutcomeUnknown):
				result.DeliveryCounts.OutcomeUnknown++
			default:
				return AdminReadDiagnosticResult{}, ErrAdminReadUnavailable
			}
		}
	}
	return result, nil
}

type selectedAdminReadEvent struct {
	event      eventport.AdminReadEvent
	deliveries []eventport.AdminReadDelivery
}

func selectAdminReadEvents(snapshot eventport.AdminReadSnapshot, query eventport.AdminReadQuery) ([]selectedAdminReadEvent, error) {
	events := make(map[eventport.EventID]eventport.AdminReadEvent, len(snapshot.Events))
	for _, event := range snapshot.Events {
		if !validAdminReadEvent(event) || event.EventID == 0 {
			return nil, ErrAdminReadUnavailable
		}
		if _, exists := events[event.EventID]; exists {
			return nil, ErrAdminReadUnavailable
		}
		events[event.EventID] = event
	}
	deliveries := make(map[eventport.EventID][]eventport.AdminReadDelivery, len(events))
	seenConsumers := make(map[eventport.EventID]map[string]struct{}, len(events))
	for _, delivery := range snapshot.Deliveries {
		if !validAdminReadDelivery(delivery) {
			return nil, ErrAdminReadUnavailable
		}
		event, exists := events[delivery.EventID]
		if !exists || !adminReadBindingMatches(event.EventType, delivery.Consumer) {
			return nil, ErrAdminReadUnavailable
		}
		if seenConsumers[delivery.EventID] == nil {
			seenConsumers[delivery.EventID] = make(map[string]struct{})
		}
		if _, exists := seenConsumers[delivery.EventID][delivery.Consumer]; exists {
			return nil, ErrAdminReadUnavailable
		}
		seenConsumers[delivery.EventID][delivery.Consumer] = struct{}{}
		deliveries[delivery.EventID] = append(deliveries[delivery.EventID], delivery)
	}
	selected := make([]selectedAdminReadEvent, 0, len(events))
	for _, event := range events {
		if query.EventType != "" && event.EventType != query.EventType {
			continue
		}
		eventDeliveries := append([]eventport.AdminReadDelivery(nil), deliveries[event.EventID]...)
		sort.SliceStable(eventDeliveries, func(left, right int) bool {
			leftOrder := adminReadConsumerOrder(eventDeliveries[left].Consumer)
			rightOrder := adminReadConsumerOrder(eventDeliveries[right].Consumer)
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
			return eventDeliveries[left].Consumer < eventDeliveries[right].Consumer
		})
		if query.Consumer != "" || query.Status != "" {
			matched := false
			for _, delivery := range eventDeliveries {
				if adminReadDeliveryMatches(delivery, query.Consumer, query.Status) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		selected = append(selected, selectedAdminReadEvent{event: event, deliveries: eventDeliveries})
	}
	sort.SliceStable(selected, func(left, right int) bool {
		if !selected[left].event.OccurredAt.Equal(selected[right].event.OccurredAt) {
			return selected[left].event.OccurredAt.After(selected[right].event.OccurredAt)
		}
		return selected[left].event.EventID > selected[right].event.EventID
	})
	return selected, nil
}

func validAdminReadQuery(query eventport.AdminReadQuery, withPagination bool) bool {
	if (query.EventType != "" && !validAdminReadText(query.EventType)) || (query.Consumer != "" && !validAdminReadText(query.Consumer)) || (query.Status != "" && !validAdminReadText(query.Status)) {
		return false
	}
	if query.Consumer != "" {
		if _, ok := eventport.AdminReadBindingForConsumer(query.Consumer); !ok {
			return false
		}
	}
	if query.Status != "" && !validAdminReadStatus(query.Status) {
		return false
	}
	if withPagination && (query.Limit < 1 || query.Limit > 200 || query.Offset < 0 || query.Offset > 100000) {
		return false
	}
	return true
}

func validAdminReadEvent(event eventport.AdminReadEvent) bool {
	return event.EventID > 0 && validAdminReadText(event.EventType) && !event.OccurredAt.IsZero()
}

func validAdminReadDelivery(delivery eventport.AdminReadDelivery) bool {
	if delivery.EventID <= 0 || delivery.AttemptCount < 0 || !validAdminReadStatus(delivery.Status) {
		return false
	}
	terminal := delivery.Status == string(eventport.DeliveryCompleted) || delivery.Status == string(eventport.DeliveryFinalFailed) || delivery.Status == string(eventport.DeliveryOutcomeUnknown)
	if terminal {
		return delivery.CompletedAt != nil && !delivery.CompletedAt.IsZero()
	}
	return delivery.CompletedAt == nil
}

func validAdminReadStatus(status string) bool {
	for _, allowed := range eventport.AdminReadStatuses() {
		if status == allowed {
			return true
		}
	}
	return false
}

func validAdminReadText(value string) bool {
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

func adminReadBindingMatches(eventType, consumer string) bool {
	binding, ok := eventport.AdminReadBindingForConsumer(consumer)
	if !ok {
		return false
	}
	for _, allowedEventType := range binding.EventTypes {
		if eventType == allowedEventType {
			return true
		}
	}
	return false
}

func adminReadDeliveryMatches(delivery eventport.AdminReadDelivery, consumer, status string) bool {
	return (consumer == "" || delivery.Consumer == consumer) && (status == "" || delivery.Status == status)
}

func adminReadConsumerOrder(consumer string) int {
	bindings := eventport.AdminReadBindings()
	for index, binding := range bindings {
		if binding.Consumer == consumer {
			return index
		}
	}
	return len(bindings) + 1
}

func cloneAdminReadTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
