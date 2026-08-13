package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

func TestAppendExternalEventRejectsInvalidCommandsBeforeTransactionAccess(t *testing.T) {
	valid := contactport.ExternalEventCommand{
		CustomerID:     1,
		EventType:      "extension.payment_succeeded",
		Payload:        json.RawMessage(`{"order_id":"order-1"}`),
		Actor:          "ext:payments",
		OccurredAt:     time.Date(2026, 8, 13, 1, 2, 3, 0, time.UTC),
		IdempotencyKey: "payments:event-1",
	}
	invalidUTF8 := string([]byte{0xff})
	tests := map[string]func(*contactport.ExternalEventCommand){
		"missing customer": func(command *contactport.ExternalEventCommand) { command.CustomerID = 0 },
		"blank event type": func(command *contactport.ExternalEventCommand) { command.EventType = "" },
		"untrimmed event type": func(command *contactport.ExternalEventCommand) {
			command.EventType = " extension.payment_succeeded"
		},
		"long event type": func(command *contactport.ExternalEventCommand) {
			command.EventType = strings.Repeat("e", maximumExternalEventType+1)
		},
		"invalid event utf8": func(command *contactport.ExternalEventCommand) { command.EventType = invalidUTF8 },
		"non object payload": func(command *contactport.ExternalEventCommand) {
			command.Payload = json.RawMessage(`[]`)
		},
		"trailing payload": func(command *contactport.ExternalEventCommand) {
			command.Payload = json.RawMessage(`{} {}`)
		},
		"blank actor":      func(command *contactport.ExternalEventCommand) { command.Actor = "" },
		"zero occurred at": func(command *contactport.ExternalEventCommand) { command.OccurredAt = time.Time{} },
		"blank key":        func(command *contactport.ExternalEventCommand) { command.IdempotencyKey = "" },
		"untrimmed key": func(command *contactport.ExternalEventCommand) {
			command.IdempotencyKey = "payments:event-1 "
		},
		"long key": func(command *contactport.ExternalEventCommand) {
			command.IdempotencyKey = strings.Repeat("k", maximumExternalEventKey+1)
		},
	}
	repository := NewMergePortRepository()
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			command := valid
			mutate(&command)
			if _, err := repository.AppendExternalEvent(context.Background(), command); !errors.Is(err, contactport.ErrInvalidMergeCommand) {
				t.Fatalf("AppendExternalEvent() error=%v, want invalid command", err)
			}
		})
	}
	if _, err := repository.AppendExternalEvent(context.Background(), valid); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("valid command outside UoW error=%v", err)
	}
	var nilRepository *MergePortRepository
	if _, err := nilRepository.AppendExternalEvent(context.Background(), valid); !errors.Is(err, contactport.ErrInvalidMergeCommand) {
		t.Fatalf("nil repository error=%v", err)
	}
}
