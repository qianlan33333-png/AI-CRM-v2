package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestIdentityContactProcessorMapsVerifiedFactsToIdentityIngest(t *testing.T) {
	occurredAt := time.Date(2026, 8, 13, 9, 10, 11, 0, time.FixedZone("CST", 8*60*60))
	for name, test := range map[string]struct {
		fact          IdentityContactFact
		wantEventType string
		wantSource    string
	}{
		"callback inbox": {
			fact:          IdentityContactFact{Source: IdentityContactFromCallback, FactID: "inbox-42", CorpID: "corp-a", ExternalUserID: "external-user-1", OccurredAt: occurredAt},
			wantEventType: "wecom.external_contact.callback_observed",
			wantSource:    "wecom.callback",
		},
		"directory sync": {
			fact:          IdentityContactFact{Source: IdentityContactFromSync, FactID: "sync-run-7:page-2:item-3", CorpID: "corp-a", ExternalUserID: "external-user-2", OccurredAt: occurredAt},
			wantEventType: "wecom.external_contact.sync_observed",
			wantSource:    "wecom.sync",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ingestor := &fakeIdentityIngestor{result: identityport.IngestResult{Status: identityport.IngestAttributed, CustomerID: 17, EventID: 23}}
			processor, err := NewIdentityContactProcessor(ingestor)
			if err != nil {
				t.Fatal(err)
			}
			result, err := processor.Process(context.Background(), test.fact)
			if err != nil || !reflect.DeepEqual(result, ingestor.result) {
				t.Fatalf("Process() = %#v, %v", result, err)
			}
			if len(ingestor.commands) != 1 {
				t.Fatalf("Ingest calls = %d, want 1", len(ingestor.commands))
			}
			command := ingestor.commands[0]
			wantRef := identityport.IDRef{
				Kind:      identityport.KindWeComExternalUserID,
				Scope:     "wecom-corp:corp-a",
				Value:     test.fact.ExternalUserID,
				Assurance: identityport.AssuranceVerified,
				Source:    test.wantSource,
			}
			if len(command.Refs) != 1 || command.Refs[0] != wantRef || command.EventType != test.wantEventType ||
				command.Source != test.wantSource || !command.OccurredAt.Equal(occurredAt.UTC()) {
				t.Fatalf("Ingest command = %#v", command)
			}
			var payload map[string]string
			if err := json.Unmarshal(command.Payload, &payload); err != nil || !reflect.DeepEqual(payload, map[string]string{"source": string(test.fact.Source)}) {
				t.Fatalf("payload = %s, %v", command.Payload, err)
			}
			if !strings.HasPrefix(command.IdempotencyKey, "wecom.identity_contact:"+string(test.fact.Source)+":") || len(command.IdempotencyKey) != len("wecom.identity_contact:")+len(test.fact.Source)+1+64 {
				t.Fatalf("idempotency key = %q", command.IdempotencyKey)
			}
			for _, raw := range []string{test.fact.CorpID, test.fact.ExternalUserID, test.fact.FactID} {
				if strings.Contains(string(command.Payload), raw) || strings.Contains(command.IdempotencyKey, raw) {
					t.Fatalf("command leaked raw input %q: %#v", raw, command)
				}
			}
		})
	}
}

func TestIdentityContactProcessorReplayUsesStableCommandAndResult(t *testing.T) {
	fact := sampleIdentityContactFact()
	ingestor := &fakeIdentityIngestor{result: identityport.IngestResult{Status: identityport.IngestPending, PendingEventID: 31}}
	processor, err := NewIdentityContactProcessor(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	first, err := processor.Process(context.Background(), fact)
	if err != nil {
		t.Fatal(err)
	}
	second, err := processor.Process(context.Background(), fact)
	if err != nil || !reflect.DeepEqual(first, second) || len(ingestor.commands) != 2 || !reflect.DeepEqual(ingestor.commands[0], ingestor.commands[1]) {
		t.Fatalf("replay = %#v/%#v commands=%#v err=%v", first, second, ingestor.commands, err)
	}
}

func TestIdentityContactProcessorScopesIdempotencyByCorp(t *testing.T) {
	ingestor := &fakeIdentityIngestor{result: identityport.IngestResult{Status: identityport.IngestPending, PendingEventID: 31}}
	processor, err := NewIdentityContactProcessor(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	first := sampleIdentityContactFact()
	second := first
	second.CorpID = "corp-b"
	if _, err = processor.Process(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err = processor.Process(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if len(ingestor.commands) != 2 || ingestor.commands[0].IdempotencyKey == ingestor.commands[1].IdempotencyKey {
		t.Fatalf("cross-corp idempotency keys = %#v", ingestor.commands)
	}
}

func TestIdentityContactProcessorSurfacesRetryableFailureWithoutChangingCommand(t *testing.T) {
	cause := errors.New("identity transaction interrupted")
	ingestor := &fakeIdentityIngestor{errors: []error{cause, nil}, result: identityport.IngestResult{Status: identityport.IngestConflict, PendingEventID: 37}}
	processor, err := NewIdentityContactProcessor(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = processor.Process(context.Background(), sampleIdentityContactFact()); !errors.Is(err, ErrIdentityContactIngestFailed) || !errors.Is(err, cause) {
		t.Fatalf("first Process() error = %v", err)
	}
	result, err := processor.Process(context.Background(), sampleIdentityContactFact())
	if err != nil || result.Status != identityport.IngestConflict || len(ingestor.commands) != 2 || !reflect.DeepEqual(ingestor.commands[0], ingestor.commands[1]) {
		t.Fatalf("retry = %#v, commands=%#v, err=%v", result, ingestor.commands, err)
	}
}

func TestIdentityContactProcessorFailsClosedForInvalidInputOrResult(t *testing.T) {
	valid := sampleIdentityContactFact()
	invalidFacts := []IdentityContactFact{
		{},
		{Source: "unknown", FactID: valid.FactID, CorpID: valid.CorpID, ExternalUserID: valid.ExternalUserID, OccurredAt: valid.OccurredAt},
		{Source: valid.Source, FactID: " fact ", CorpID: valid.CorpID, ExternalUserID: valid.ExternalUserID, OccurredAt: valid.OccurredAt},
		{Source: valid.Source, FactID: valid.FactID, CorpID: "corp with spaces", ExternalUserID: valid.ExternalUserID, OccurredAt: valid.OccurredAt},
		{Source: valid.Source, FactID: valid.FactID, CorpID: valid.CorpID, ExternalUserID: " external ", OccurredAt: valid.OccurredAt},
	}
	for _, fact := range invalidFacts {
		ingestor := &fakeIdentityIngestor{}
		processor, err := NewIdentityContactProcessor(ingestor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = processor.Process(context.Background(), fact); !errors.Is(err, ErrInvalidIdentityContactFact) || len(ingestor.commands) != 0 {
			t.Fatalf("invalid fact %#v = %v, calls=%d", fact, err, len(ingestor.commands))
		}
	}

	if processor, err := NewIdentityContactProcessor(nil); processor != nil || !errors.Is(err, ErrInvalidIdentityContactProcessor) {
		t.Fatalf("NewIdentityContactProcessor(nil) = %#v, %v", processor, err)
	}
	ingestor := &fakeIdentityIngestor{result: identityport.IngestResult{Status: "unexpected"}}
	processor, err := NewIdentityContactProcessor(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = processor.Process(context.Background(), valid); !errors.Is(err, ErrInvalidIdentityContactResult) {
		t.Fatalf("invalid result error = %v", err)
	}
}

func sampleIdentityContactFact() IdentityContactFact {
	return IdentityContactFact{
		Source:         IdentityContactFromCallback,
		FactID:         "inbox-42",
		CorpID:         "corp-a",
		ExternalUserID: "external-user-1",
		OccurredAt:     time.Date(2026, 8, 13, 1, 2, 3, 0, time.UTC),
	}
}

type fakeIdentityIngestor struct {
	commands []identityport.IngestCommand
	errors   []error
	result   identityport.IngestResult
}

func (ingestor *fakeIdentityIngestor) Ingest(_ context.Context, command identityport.IngestCommand) (identityport.IngestResult, error) {
	ingestor.commands = append(ingestor.commands, command)
	if len(ingestor.errors) > 0 {
		err := ingestor.errors[0]
		ingestor.errors = ingestor.errors[1:]
		if err != nil {
			return identityport.IngestResult{}, err
		}
	}
	return ingestor.result, nil
}
