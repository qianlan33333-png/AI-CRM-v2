package port

import (
	"context"
	"time"
)

const AdminReadRegistryID = "v2-internal-events.v1"

type AdminReadBinding struct {
	Consumer   string
	EventTypes []string
}

var adminReadBindings = []AdminReadBinding{
	{Consumer: ConsumerAutomationTagTrigger, EventTypes: []string{EvTagApplied}},
	{Consumer: ConsumerStatsTagApplied, EventTypes: []string{EvTagApplied}},
	{Consumer: ConsumerOperationCycleFact, EventTypes: []string{EvOperationCycleFact}},
}

var adminReadStatuses = []string{
	string(DeliveryPending),
	string(DeliveryProcessing),
	string(DeliveryCompleted),
	string(DeliveryFinalFailed),
	string(DeliveryOutcomeUnknown),
}

func AdminReadBindings() []AdminReadBinding {
	bindings := make([]AdminReadBinding, len(adminReadBindings))
	for index, binding := range adminReadBindings {
		bindings[index] = AdminReadBinding{Consumer: binding.Consumer, EventTypes: append([]string(nil), binding.EventTypes...)}
	}
	return bindings
}

func AdminReadStatuses() []string { return append([]string(nil), adminReadStatuses...) }

func AdminReadBindingForConsumer(consumer string) (AdminReadBinding, bool) {
	for _, binding := range adminReadBindings {
		if binding.Consumer == consumer {
			return AdminReadBinding{Consumer: binding.Consumer, EventTypes: append([]string(nil), binding.EventTypes...)}, true
		}
	}
	return AdminReadBinding{}, false
}

type AdminReadQuery struct {
	EventType string
	Consumer  string
	Status    string
	Limit     int64
	Offset    int64
}

type AdminReadEvent struct {
	EventID    EventID
	EventType  string
	OccurredAt time.Time
	Dispatched bool
}

type AdminReadDelivery struct {
	EventID      EventID
	Consumer     string
	Status       string
	AttemptCount int32
	CompletedAt  *time.Time
}

type AdminReadSnapshot struct {
	Events     []AdminReadEvent
	Deliveries []AdminReadDelivery
}

type AdminReadRepository interface {
	Read(context.Context, string) (AdminReadSnapshot, error)
}
