package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type mutationAttemptKey struct{}

type fakeMutationUoW struct {
	calls, callbacks int
	attempts         int
	err              error
}

func (uow *fakeMutationUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	attempts := uow.attempts
	if attempts == 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		uow.callbacks++
		if err := callback(context.WithValue(ctx, mutationAttemptKey{}, attempt)); err != nil {
			return err
		}
	}
	return nil
}

type fakeMutationStore struct {
	updateResult  CustomerProfileMutation
	stageResult   CustomerStageMutation
	addChanged    bool
	removeChanged bool

	updateErr, stageErr, addErr, removeErr, customerEventErr           error
	updateCalls, stageCalls, addCalls, removeCalls, customerEventCalls int
	updates                                                            []CustomerUpdateCommand
	stages                                                             []CustomerStageCommand
	addTags                                                            []CustomerTagCommand
	removeTags                                                         []CustomerTagCommand
	customerEvents                                                     []CustomerEventAppend
	sequence                                                           *[]string
	attempts                                                           []int
}

func (store *fakeMutationStore) UpdateCustomer(ctx context.Context, command CustomerUpdateCommand) (CustomerProfileMutation, error) {
	store.updateCalls++
	store.updates = append(store.updates, command)
	store.record(ctx, "store.update")
	return store.updateResult, store.updateErr
}

func (store *fakeMutationStore) SetCustomerStage(ctx context.Context, command CustomerStageCommand) (CustomerStageMutation, error) {
	store.stageCalls++
	store.stages = append(store.stages, command)
	store.record(ctx, "store.stage")
	return store.stageResult, store.stageErr
}

func (store *fakeMutationStore) AddCustomerTag(ctx context.Context, command CustomerTagCommand) (bool, error) {
	store.addCalls++
	store.addTags = append(store.addTags, command)
	store.record(ctx, "store.tag.add")
	return store.addChanged, store.addErr
}

func (store *fakeMutationStore) RemoveCustomerTag(ctx context.Context, command CustomerTagCommand) (bool, error) {
	store.removeCalls++
	store.removeTags = append(store.removeTags, command)
	store.record(ctx, "store.tag.remove")
	return store.removeChanged, store.removeErr
}

func (store *fakeMutationStore) AppendCustomerEvent(ctx context.Context, event CustomerEventAppend) (contactport.EventID, error) {
	store.customerEventCalls++
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	store.customerEvents = append(store.customerEvents, event)
	store.record(ctx, "customer_event.append")
	return contactport.EventID(store.customerEventCalls), store.customerEventErr
}

func (store *fakeMutationStore) record(ctx context.Context, step string) {
	attempt, _ := ctx.Value(mutationAttemptKey{}).(int)
	store.attempts = append(store.attempts, attempt)
	if store.sequence != nil {
		*store.sequence = append(*store.sequence, step)
	}
}

func (store *fakeMutationStore) calls() int {
	return store.updateCalls + store.stageCalls + store.addCalls + store.removeCalls + store.customerEventCalls
}

type fakeMutationAppender struct {
	calls    int
	events   []eventport.Event
	attempts []int
	err      error
	sequence *[]string
}

type fakeMutationDeliveryAcceptor struct{}

func (fakeMutationDeliveryAcceptor) Accept(context.Context, eventport.EventID, string) error {
	return nil
}

func (appender *fakeMutationAppender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	appender.calls++
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	appender.events = append(appender.events, event)
	attempt, _ := ctx.Value(mutationAttemptKey{}).(int)
	appender.attempts = append(appender.attempts, attempt)
	if appender.sequence != nil {
		*appender.sequence = append(*appender.sequence, "event_log.append")
	}
	return eventport.EventID(appender.calls), appender.err
}

func newTestMutationService(uow *fakeMutationUoW, store *fakeMutationStore, events *fakeMutationAppender) *CustomerMutationService {
	return &CustomerMutationService{
		uow:        uow,
		store:      store,
		events:     events,
		deliveries: fakeMutationDeliveryAcceptor{},
		now:        mutationTime,
		newEventKey: func() (string, error) {
			return "fixed-key", nil
		},
	}
}

func mutationTime() time.Time {
	return time.Date(2026, time.August, 12, 17, 45, 30, 987654321, time.FixedZone("CST", 8*60*60))
}

func mutationCustomer() CustomerRecord {
	return CustomerRecord{ID: 19, Name: "Ada", Extra: json.RawMessage(`{}`), CreatedAt: mutationTime().UTC(), UpdatedAt: mutationTime().UTC()}
}

func validUpdate() CustomerUpdateCommand {
	name := "Ada"
	return CustomerUpdateCommand{ID: 19, Name: &name, Actor: "admin:7"}
}

func validStage() CustomerStageCommand {
	stageID := int64(23)
	return CustomerStageCommand{ID: 19, StageID: &stageID, Actor: "admin:7"}
}

func validTag() CustomerTagCommand {
	return CustomerTagCommand{ID: 19, TagID: 29, Actor: "admin:7"}
}

func stringPtr(value string) *string { return &value }

func rawPtr(value string) *json.RawMessage {
	raw := json.RawMessage(value)
	return &raw
}

type mutationOperation struct {
	name              string
	run               func(*CustomerMutationService) (CustomerRecord, error)
	runWithScope      func(*CustomerMutationService, *int64) (CustomerRecord, error)
	configure         func(*fakeMutationStore)
	storeCalls        func(*fakeMutationStore) int
	scopeOwnerStaffID func(*fakeMutationStore) []*int64
	setStoreError     func(*fakeMutationStore, error)
	writeStep         string
	eventType         string
}

func mutationOperations() []mutationOperation {
	return []mutationOperation{
		{
			name: "update",
			run: func(service *CustomerMutationService) (CustomerRecord, error) {
				return service.Update(context.Background(), validUpdate())
			},
			runWithScope: func(service *CustomerMutationService, scopeOwnerStaffID *int64) (CustomerRecord, error) {
				command := validUpdate()
				command.ScopeOwnerStaffID = scopeOwnerStaffID
				return service.Update(context.Background(), command)
			},
			configure: func(store *fakeMutationStore) {
				store.updateResult = CustomerProfileMutation{Customer: mutationCustomer(), StateChange: true}
			},
			storeCalls: func(store *fakeMutationStore) int { return store.updateCalls },
			scopeOwnerStaffID: func(store *fakeMutationStore) []*int64 {
				values := make([]*int64, len(store.updates))
				for index, command := range store.updates {
					values[index] = command.ScopeOwnerStaffID
				}
				return values
			},
			setStoreError: func(store *fakeMutationStore, err error) { store.updateErr = err },
			writeStep:     "store.update", eventType: eventport.EvCustomerUpdated,
		},
		{
			name: "set stage",
			run: func(service *CustomerMutationService) (CustomerRecord, error) {
				return service.SetStage(context.Background(), validStage())
			},
			runWithScope: func(service *CustomerMutationService, scopeOwnerStaffID *int64) (CustomerRecord, error) {
				command := validStage()
				command.ScopeOwnerStaffID = scopeOwnerStaffID
				return service.SetStage(context.Background(), command)
			},
			configure: func(store *fakeMutationStore) {
				store.stageResult = CustomerStageMutation{Customer: mutationCustomer(), StateChange: true}
			},
			storeCalls: func(store *fakeMutationStore) int { return store.stageCalls },
			scopeOwnerStaffID: func(store *fakeMutationStore) []*int64 {
				values := make([]*int64, len(store.stages))
				for index, command := range store.stages {
					values[index] = command.ScopeOwnerStaffID
				}
				return values
			},
			setStoreError: func(store *fakeMutationStore, err error) { store.stageErr = err },
			writeStep:     "store.stage", eventType: eventport.EvStageChanged,
		},
		{
			name: "add tag",
			run: func(service *CustomerMutationService) (CustomerRecord, error) {
				return CustomerRecord{}, service.AddTag(context.Background(), validTag())
			},
			runWithScope: func(service *CustomerMutationService, scopeOwnerStaffID *int64) (CustomerRecord, error) {
				command := validTag()
				command.ScopeOwnerStaffID = scopeOwnerStaffID
				return CustomerRecord{}, service.AddTag(context.Background(), command)
			},
			configure:  func(store *fakeMutationStore) { store.addChanged = true },
			storeCalls: func(store *fakeMutationStore) int { return store.addCalls },
			scopeOwnerStaffID: func(store *fakeMutationStore) []*int64 {
				values := make([]*int64, len(store.addTags))
				for index, command := range store.addTags {
					values[index] = command.ScopeOwnerStaffID
				}
				return values
			},
			setStoreError: func(store *fakeMutationStore, err error) { store.addErr = err },
			writeStep:     "store.tag.add", eventType: eventport.EvTagApplied,
		},
		{
			name: "remove tag",
			run: func(service *CustomerMutationService) (CustomerRecord, error) {
				return CustomerRecord{}, service.RemoveTag(context.Background(), validTag())
			},
			runWithScope: func(service *CustomerMutationService, scopeOwnerStaffID *int64) (CustomerRecord, error) {
				command := validTag()
				command.ScopeOwnerStaffID = scopeOwnerStaffID
				return CustomerRecord{}, service.RemoveTag(context.Background(), command)
			},
			configure:  func(store *fakeMutationStore) { store.removeChanged = true },
			storeCalls: func(store *fakeMutationStore) int { return store.removeCalls },
			scopeOwnerStaffID: func(store *fakeMutationStore) []*int64 {
				values := make([]*int64, len(store.removeTags))
				for index, command := range store.removeTags {
					values[index] = command.ScopeOwnerStaffID
				}
				return values
			},
			setStoreError: func(store *fakeMutationStore, err error) { store.removeErr = err },
			writeStep:     "store.tag.remove", eventType: eventport.EvTagRemoved,
		},
	}
}

func TestCustomerMutationFailsClosedWithoutDependencies(t *testing.T) {
	dependencies := []struct {
		name   string
		mutate func(*CustomerMutationService)
	}{
		{name: "unit of work", mutate: func(service *CustomerMutationService) { service.uow = nil }},
		{name: "store", mutate: func(service *CustomerMutationService) { service.store = nil }},
		{name: "event appender", mutate: func(service *CustomerMutationService) { service.events = nil }},
		{name: "clock", mutate: func(service *CustomerMutationService) { service.now = nil }},
		{name: "event key", mutate: func(service *CustomerMutationService) { service.newEventKey = nil }},
	}
	for _, dependency := range dependencies {
		for _, operation := range mutationOperations() {
			t.Run(dependency.name+"/"+operation.name, func(t *testing.T) {
				uow, store, events := &fakeMutationUoW{}, &fakeMutationStore{}, &fakeMutationAppender{}
				operation.configure(store)
				service := newTestMutationService(uow, store, events)
				dependency.mutate(service)

				customer, err := operation.run(service)
				if !errors.Is(err, ErrCustomerMutationFailed) {
					t.Fatalf("error = %v, want ErrCustomerMutationFailed", err)
				}
				assertZeroCustomer(t, customer)
				if uow.calls != 0 || store.calls() != 0 || events.calls != 0 {
					t.Fatalf("missing dependency reached collaborators: uow=%d store=%d event_log=%d", uow.calls, store.calls(), events.calls)
				}
			})
		}
	}

	t.Run("nil receiver", func(t *testing.T) {
		var service *CustomerMutationService
		customer, err := service.Update(context.Background(), validUpdate())
		if !errors.Is(err, ErrCustomerMutationFailed) {
			t.Fatalf("error = %v, want ErrCustomerMutationFailed", err)
		}
		assertZeroCustomer(t, customer)
	})
}

func TestCustomerMutationRejectsAllInvalidCommandsBeforeTransaction(t *testing.T) {
	zero, negative := int64(0), int64(-1)
	invalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name string
		run  func(*CustomerMutationService) (CustomerRecord, error)
	}{
		{name: "profile empty patch", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			return service.Update(context.Background(), CustomerUpdateCommand{ID: 19, Actor: "admin:7"})
		}},
		{name: "profile zero id", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.ID = 0
			return service.Update(context.Background(), command)
		}},
		{name: "profile negative id", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.ID = -1
			return service.Update(context.Background(), command)
		}},
		{name: "profile empty actor", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Actor = ""
			return service.Update(context.Background(), command)
		}},
		{name: "profile padded actor", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Actor = " admin:7"
			return service.Update(context.Background(), command)
		}},
		{name: "profile long actor", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Actor = contactport.Actor(strings.Repeat("a", 201))
			return service.Update(context.Background(), command)
		}},
		{name: "profile invalid utf8 actor", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Actor = contactport.Actor(invalidUTF8)
			return service.Update(context.Background(), command)
		}},
		{name: "profile invalid utf8 name", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Name = stringPtr(invalidUTF8)
			return service.Update(context.Background(), command)
		}},
		{name: "profile non HTTP avatar", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.AvatarURL = NullablePatch[string]{Set: true, Value: stringPtr("ftp://example.test/avatar")}
			return service.Update(context.Background(), command)
		}},
		{name: "profile avatar user info", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.AvatarURL = NullablePatch[string]{Set: true, Value: stringPtr("https://user@example.test/avatar")}
			return service.Update(context.Background(), command)
		}},
		{name: "profile avatar no host", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.AvatarURL = NullablePatch[string]{Set: true, Value: stringPtr("https:///avatar")}
			return service.Update(context.Background(), command)
		}},
		{name: "profile zero owner", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.OwnerStaffID = NullablePatch[int64]{Set: true, Value: &zero}
			return service.Update(context.Background(), command)
		}},
		{name: "profile negative owner", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.OwnerStaffID = NullablePatch[int64]{Set: true, Value: &negative}
			return service.Update(context.Background(), command)
		}},
		{name: "profile zero channel", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.ChannelID = NullablePatch[int64]{Set: true, Value: &zero}
			return service.Update(context.Background(), command)
		}},
		{name: "profile negative channel", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.ChannelID = NullablePatch[int64]{Set: true, Value: &negative}
			return service.Update(context.Background(), command)
		}},
		{name: "profile malformed extra", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Extra = rawPtr("{")
			return service.Update(context.Background(), command)
		}},
		{name: "profile array extra", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Extra = rawPtr("[]")
			return service.Update(context.Background(), command)
		}},
		{name: "profile null extra", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Extra = rawPtr("null")
			return service.Update(context.Background(), command)
		}},
		{name: "profile nested external identity", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validUpdate()
			command.Extra = rawPtr(`{"profile":{"unionid":"identity-secret"}}`)
			return service.Update(context.Background(), command)
		}},
		{name: "stage zero customer", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validStage()
			command.ID = 0
			return service.SetStage(context.Background(), command)
		}},
		{name: "stage negative customer", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validStage()
			command.ID = -1
			return service.SetStage(context.Background(), command)
		}},
		{name: "stage zero target", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validStage()
			command.StageID = &zero
			return service.SetStage(context.Background(), command)
		}},
		{name: "stage negative target", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validStage()
			command.StageID = &negative
			return service.SetStage(context.Background(), command)
		}},
		{name: "stage invalid actor", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validStage()
			command.Actor = contactport.Actor(invalidUTF8)
			return service.SetStage(context.Background(), command)
		}},
		{name: "tag zero customer", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validTag()
			command.ID = 0
			return CustomerRecord{}, service.AddTag(context.Background(), command)
		}},
		{name: "tag negative tag", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validTag()
			command.TagID = -1
			return CustomerRecord{}, service.AddTag(context.Background(), command)
		}},
		{name: "tag zero tag", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validTag()
			command.TagID = 0
			return CustomerRecord{}, service.RemoveTag(context.Background(), command)
		}},
		{name: "tag padded actor", run: func(service *CustomerMutationService) (CustomerRecord, error) {
			command := validTag()
			command.Actor = "admin:7 "
			return CustomerRecord{}, service.RemoveTag(context.Background(), command)
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store, events := &fakeMutationUoW{}, &fakeMutationStore{}, &fakeMutationAppender{}
			customer, err := testCase.run(newTestMutationService(uow, store, events))
			if !errors.Is(err, ErrInvalidCustomerMutation) {
				t.Fatalf("error = %v, want ErrInvalidCustomerMutation", err)
			}
			assertZeroCustomer(t, customer)
			if uow.calls != 0 || store.calls() != 0 || events.calls != 0 {
				t.Fatalf("invalid input reached collaborators: uow=%d store=%d event_log=%d", uow.calls, store.calls(), events.calls)
			}
		})
	}
}

func TestCustomerMutationPassesGlobalAndOwnerScopeToStore(t *testing.T) {
	ownerStaffID := int64(71)
	scopes := []struct {
		name  string
		value *int64
	}{
		{name: "global", value: nil},
		{name: "owner", value: &ownerStaffID},
	}

	for _, operation := range mutationOperations() {
		operation := operation
		for _, scope := range scopes {
			scope := scope
			t.Run(operation.name+"/"+scope.name, func(t *testing.T) {
				uow, store, events := &fakeMutationUoW{}, &fakeMutationStore{}, &fakeMutationAppender{}
				operation.configure(store)

				_, err := operation.runWithScope(newTestMutationService(uow, store, events), scope.value)
				if err != nil {
					t.Fatalf("mutation error = %v", err)
				}
				if uow.calls != 1 || uow.callbacks != 1 || operation.storeCalls(store) != 1 {
					t.Fatalf("mutation calls = uow:%d callbacks:%d store:%d, want 1/1/1", uow.calls, uow.callbacks, operation.storeCalls(store))
				}
				assertScopeOwnerStaffIDs(t, operation.scopeOwnerStaffID(store), scope.value, 1)
			})
		}
	}
}

func TestCustomerMutationRejectsNonPositiveScopeBeforeUnitOfWork(t *testing.T) {
	zero, negative := int64(0), int64(-71)
	for _, operation := range mutationOperations() {
		operation := operation
		for _, scope := range []struct {
			name  string
			value *int64
		}{
			{name: "zero", value: &zero},
			{name: "negative", value: &negative},
		} {
			scope := scope
			t.Run(operation.name+"/"+scope.name, func(t *testing.T) {
				uow, store, events := &fakeMutationUoW{}, &fakeMutationStore{}, &fakeMutationAppender{}
				operation.configure(store)

				customer, err := operation.runWithScope(newTestMutationService(uow, store, events), scope.value)
				if !errors.Is(err, ErrInvalidCustomerMutation) {
					t.Fatalf("error = %v, want ErrInvalidCustomerMutation", err)
				}
				assertZeroCustomer(t, customer)
				if uow.calls != 0 || uow.callbacks != 0 || store.calls() != 0 || events.calls != 0 {
					t.Fatalf("invalid scope reached collaborators: uow=%d callbacks=%d store=%d event_log=%d", uow.calls, uow.callbacks, store.calls(), events.calls)
				}
			})
		}
	}
}

func TestCustomerMutationKeepsOwnerScopeAcrossUnitOfWorkRetries(t *testing.T) {
	ownerStaffID := int64(71)
	for _, operation := range mutationOperations() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			uow := &fakeMutationUoW{attempts: 3}
			store, events := &fakeMutationStore{}, &fakeMutationAppender{}
			operation.configure(store)

			_, err := operation.runWithScope(newTestMutationService(uow, store, events), &ownerStaffID)
			if err != nil {
				t.Fatalf("mutation error = %v", err)
			}
			if uow.calls != 1 || uow.callbacks != 3 || operation.storeCalls(store) != 3 {
				t.Fatalf("retry calls = uow:%d callbacks:%d store:%d, want 1/3/3", uow.calls, uow.callbacks, operation.storeCalls(store))
			}
			assertScopeOwnerStaffIDs(t, operation.scopeOwnerStaffID(store), &ownerStaffID, 3)
		})
	}
}

func TestCustomerMutationWrapsCrossOwnerNotFoundWithoutEvents(t *testing.T) {
	ownerStaffID := int64(71)
	for _, operation := range mutationOperations() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			uow, store, events := &fakeMutationUoW{}, &fakeMutationStore{}, &fakeMutationAppender{}
			operation.configure(store)
			operation.setStoreError(store, ErrCustomerNotFound)
			service := newTestMutationService(uow, store, events)
			keyCalls := 0
			service.newEventKey = func() (string, error) {
				keyCalls++
				return "must-not-be-used", nil
			}

			customer, err := operation.runWithScope(service, &ownerStaffID)
			if !errors.Is(err, ErrCustomerMutationFailed) || !errors.Is(err, ErrCustomerNotFound) {
				t.Fatalf("error = %v, want wrapped ErrCustomerMutationFailed and ErrCustomerNotFound", err)
			}
			assertZeroCustomer(t, customer)
			if uow.calls != 1 || uow.callbacks != 1 || operation.storeCalls(store) != 1 {
				t.Fatalf("not-found calls = uow:%d callbacks:%d store:%d, want 1/1/1", uow.calls, uow.callbacks, operation.storeCalls(store))
			}
			assertScopeOwnerStaffIDs(t, operation.scopeOwnerStaffID(store), &ownerStaffID, 1)
			if store.customerEventCalls != 0 || events.calls != 0 || keyCalls != 0 {
				t.Fatalf("cross-owner not found wrote or prepared events: customer_events=%d event_log=%d key_calls=%d", store.customerEventCalls, events.calls, keyCalls)
			}
		})
	}
}

func TestCustomerMutationWritesMatchingEventsInOrder(t *testing.T) {
	from := int64(17)
	tests := []struct {
		name        string
		operation   mutationOperation
		wantPayload map[string]json.RawMessage
	}{
		{
			name: "update", operation: mutationOperations()[0],
			wantPayload: map[string]json.RawMessage{"customer_id": json.RawMessage(`19`), "actor": json.RawMessage(`"admin:7"`)},
		},
		{
			name: "set stage", operation: func() mutationOperation {
				operation := mutationOperations()[1]
				operation.configure = func(store *fakeMutationStore) {
					store.stageResult = CustomerStageMutation{Customer: mutationCustomer(), PreviousID: &from, StateChange: true}
				}
				return operation
			}(),
			wantPayload: map[string]json.RawMessage{"customer_id": json.RawMessage(`19`), "from_stage_id": json.RawMessage(`17`), "to_stage_id": json.RawMessage(`23`), "actor": json.RawMessage(`"admin:7"`)},
		},
		{
			name: "add tag", operation: mutationOperations()[2],
			wantPayload: map[string]json.RawMessage{"customer_id": json.RawMessage(`19`), "tag_id": json.RawMessage(`29`), "actor": json.RawMessage(`"admin:7"`)},
		},
		{
			name: "remove tag", operation: mutationOperations()[3],
			wantPayload: map[string]json.RawMessage{"customer_id": json.RawMessage(`19`), "tag_id": json.RawMessage(`29`), "actor": json.RawMessage(`"admin:7"`)},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			sequence := make([]string, 0, 3)
			store := &fakeMutationStore{sequence: &sequence}
			testCase.operation.configure(store)
			events := &fakeMutationAppender{sequence: &sequence}
			customer, err := testCase.operation.run(newTestMutationService(&fakeMutationUoW{}, store, events))
			if err != nil {
				t.Fatalf("mutation error = %v", err)
			}
			if testCase.operation.name == "update" || testCase.operation.name == "set stage" {
				if !reflect.DeepEqual(customer, mutationCustomer()) {
					t.Fatalf("customer = %#v, want %#v", customer, mutationCustomer())
				}
			} else {
				assertZeroCustomer(t, customer)
			}
			wantSequence := []string{testCase.operation.writeStep, "customer_event.append", "event_log.append"}
			if !reflect.DeepEqual(sequence, wantSequence) {
				t.Fatalf("write sequence = %v, want %v", sequence, wantSequence)
			}
			if store.customerEventCalls != 1 || events.calls != 1 {
				t.Fatalf("event writes = customer_events:%d event_log:%d, want 1/1", store.customerEventCalls, events.calls)
			}
			if !reflect.DeepEqual(store.attempts, []int{1, 1}) || !reflect.DeepEqual(events.attempts, []int{1}) {
				t.Fatalf("transaction-bound contexts = store:%v event_log:%v, want [1 1]/[1]", store.attempts, events.attempts)
			}
			assertMatchingEvents(t, store.customerEvents[0], events.events[0], testCase.operation.eventType, testCase.operation.eventType+":fixed-key", mutationTime().UTC(), testCase.wantPayload)
		})
	}
}

func TestCustomerMutationUpdateNoopWritesNoEventsAndDoesNotConsumeKey(t *testing.T) {
	sequence := make([]string, 0, 3)
	store := &fakeMutationStore{
		updateResult: CustomerProfileMutation{Customer: mutationCustomer(), StateChange: false},
		sequence:     &sequence,
	}
	events := &fakeMutationAppender{sequence: &sequence}
	service := newTestMutationService(&fakeMutationUoW{}, store, events)
	keyCalls := 0
	service.newEventKey = func() (string, error) {
		keyCalls++
		return "must-not-be-used", nil
	}

	customer, err := service.Update(context.Background(), validUpdate())
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !reflect.DeepEqual(customer, mutationCustomer()) {
		t.Fatalf("Update() customer = %#v, want %#v", customer, mutationCustomer())
	}
	if !reflect.DeepEqual(sequence, []string{"store.update"}) || store.customerEventCalls != 0 || events.calls != 0 || keyCalls != 0 {
		t.Fatalf("profile no-op must write/consume nothing after store: sequence=%v customer_events=%d event_log=%d key_calls=%d", sequence, store.customerEventCalls, events.calls, keyCalls)
	}
}

func TestCustomerMutationFailsClosedForExternalIdentityReturnedByStore(t *testing.T) {
	polluted := mutationCustomer()
	polluted.Extra = json.RawMessage(`{"nested":{"wecomTagId":"identity-secret"}}`)
	for _, operation := range mutationOperations()[:2] {
		t.Run(operation.name, func(t *testing.T) {
			store := &fakeMutationStore{}
			operation.configure(store)
			store.updateResult.Customer = polluted
			store.stageResult.Customer = polluted
			events := &fakeMutationAppender{}
			customer, err := operation.run(newTestMutationService(&fakeMutationUoW{}, store, events))
			if !errors.Is(err, ErrCustomerMutationFailed) {
				t.Fatalf("mutation error = %v, want fail-closed", err)
			}
			assertZeroCustomer(t, customer)
			if store.customerEventCalls != 0 || events.calls != 0 {
				t.Fatalf("polluted result emitted events: customer=%d domain=%d", store.customerEventCalls, events.calls)
			}
		})
	}
}

func TestCustomerMutationPropagatesEveryTransactionWriteFailure(t *testing.T) {
	sentinel := errors.New("write unavailable")
	for _, operation := range mutationOperations() {
		for failure, label := range []string{"contact store", "customer events", "event log"} {
			t.Run(operation.name+"/"+label, func(t *testing.T) {
				sequence := make([]string, 0, 3)
				store := &fakeMutationStore{sequence: &sequence}
				operation.configure(store)
				switch failure {
				case 0:
					switch operation.name {
					case "update":
						store.updateErr = sentinel
					case "set stage":
						store.stageErr = sentinel
					case "add tag":
						store.addErr = sentinel
					case "remove tag":
						store.removeErr = sentinel
					}
				case 1:
					store.customerEventErr = sentinel
				}
				events := &fakeMutationAppender{sequence: &sequence}
				if failure == 2 {
					events.err = sentinel
				}

				customer, err := operation.run(newTestMutationService(&fakeMutationUoW{}, store, events))
				if !errors.Is(err, ErrCustomerMutationFailed) || !errors.Is(err, sentinel) {
					t.Fatalf("error = %v, want stable mutation failure and original sentinel", err)
				}
				assertZeroCustomer(t, customer)
				wantSequence := []string{operation.writeStep}
				if failure >= 1 {
					wantSequence = append(wantSequence, "customer_event.append")
				}
				if failure == 2 {
					wantSequence = append(wantSequence, "event_log.append")
				}
				if !reflect.DeepEqual(sequence, wantSequence) {
					t.Fatalf("failure write sequence = %v, want %v", sequence, wantSequence)
				}
			})
		}
	}
}

func TestCustomerMutationMapsContextAndUnitOfWorkErrors(t *testing.T) {
	for _, original := range []error{context.Canceled, context.DeadlineExceeded, errors.New("begin failed")} {
		for _, operation := range mutationOperations() {
			t.Run(operation.name+"/"+original.Error(), func(t *testing.T) {
				uow := &fakeMutationUoW{err: original}
				store, events := &fakeMutationStore{}, &fakeMutationAppender{}
				operation.configure(store)
				customer, err := operation.run(newTestMutationService(uow, store, events))
				if !errors.Is(err, ErrCustomerMutationFailed) || !errors.Is(err, original) {
					t.Fatalf("error = %v, want stable mutation failure and %v", err, original)
				}
				assertZeroCustomer(t, customer)
				if uow.calls != 1 || uow.callbacks != 0 || store.calls() != 0 || events.calls != 0 {
					t.Fatalf("uow failure reached collaborators: uow=%d callbacks=%d store=%d event_log=%d", uow.calls, uow.callbacks, store.calls(), events.calls)
				}
			})
		}
	}
}

func TestCustomerMutationSetStageSupportsClearPositiveAndIdempotentNoop(t *testing.T) {
	previous, target := int64(17), int64(31)
	tests := []struct {
		name        string
		command     CustomerStageCommand
		mutation    CustomerStageMutation
		wantPayload map[string]json.RawMessage
	}{
		{
			name: "clear", command: CustomerStageCommand{ID: 19, Actor: "admin:7"},
			mutation:    CustomerStageMutation{Customer: mutationCustomer(), PreviousID: &previous, StateChange: true},
			wantPayload: map[string]json.RawMessage{"customer_id": json.RawMessage(`19`), "from_stage_id": json.RawMessage(`17`), "to_stage_id": json.RawMessage(`null`), "actor": json.RawMessage(`"admin:7"`)},
		},
		{
			name: "set positive", command: CustomerStageCommand{ID: 19, StageID: &target, Actor: "admin:7"},
			mutation:    CustomerStageMutation{Customer: mutationCustomer(), StateChange: true},
			wantPayload: map[string]json.RawMessage{"customer_id": json.RawMessage(`19`), "from_stage_id": json.RawMessage(`null`), "to_stage_id": json.RawMessage(`31`), "actor": json.RawMessage(`"admin:7"`)},
		},
		{
			name: "same stage noop", command: CustomerStageCommand{ID: 19, StageID: &target, Actor: "admin:7"},
			mutation: CustomerStageMutation{Customer: mutationCustomer(), PreviousID: &target, StateChange: false},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			sequence := make([]string, 0, 3)
			store := &fakeMutationStore{stageResult: testCase.mutation, sequence: &sequence}
			events := &fakeMutationAppender{sequence: &sequence}
			customer, err := newTestMutationService(&fakeMutationUoW{}, store, events).SetStage(context.Background(), testCase.command)
			if err != nil {
				t.Fatalf("SetStage() error = %v", err)
			}
			if !reflect.DeepEqual(customer, mutationCustomer()) || len(store.stages) != 1 || !reflect.DeepEqual(store.stages[0], testCase.command) {
				t.Fatalf("SetStage() state/customer = %#v %#v, want command=%#v customer=%#v", store.stages, customer, testCase.command, mutationCustomer())
			}
			if !testCase.mutation.StateChange {
				if !reflect.DeepEqual(sequence, []string{"store.stage"}) || store.customerEventCalls != 0 || events.calls != 0 {
					t.Fatalf("same-stage no-op must write zero events: sequence=%v customer_events=%d event_log=%d", sequence, store.customerEventCalls, events.calls)
				}
				return
			}
			assertMatchingEvents(t, store.customerEvents[0], events.events[0], eventport.EvStageChanged, eventport.EvStageChanged+":fixed-key", mutationTime().UTC(), testCase.wantPayload)
		})
	}
}

func TestCustomerMutationTagsWriteEventsOnlyWhenChanged(t *testing.T) {
	tests := []struct {
		name         string
		run          func(*CustomerMutationService) error
		configure    func(*fakeMutationStore)
		wantSequence []string
		wantType     string
	}{
		{
			name: "add changed", run: func(service *CustomerMutationService) error { return service.AddTag(context.Background(), validTag()) },
			configure:    func(store *fakeMutationStore) { store.addChanged = true },
			wantSequence: []string{"store.tag.add", "customer_event.append", "event_log.append"}, wantType: eventport.EvTagApplied,
		},
		{
			name: "remove changed", run: func(service *CustomerMutationService) error {
				return service.RemoveTag(context.Background(), validTag())
			},
			configure:    func(store *fakeMutationStore) { store.removeChanged = true },
			wantSequence: []string{"store.tag.remove", "customer_event.append", "event_log.append"}, wantType: eventport.EvTagRemoved,
		},
		{
			name: "add existing noop", run: func(service *CustomerMutationService) error { return service.AddTag(context.Background(), validTag()) },
			configure: func(*fakeMutationStore) {}, wantSequence: []string{"store.tag.add"},
		},
		{
			name: "remove absent noop", run: func(service *CustomerMutationService) error {
				return service.RemoveTag(context.Background(), validTag())
			},
			configure: func(*fakeMutationStore) {}, wantSequence: []string{"store.tag.remove"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			sequence := make([]string, 0, 3)
			store := &fakeMutationStore{sequence: &sequence}
			testCase.configure(store)
			events := &fakeMutationAppender{sequence: &sequence}
			if err := testCase.run(newTestMutationService(&fakeMutationUoW{}, store, events)); err != nil {
				t.Fatalf("tag mutation error = %v", err)
			}
			if !reflect.DeepEqual(sequence, testCase.wantSequence) {
				t.Fatalf("tag write sequence = %v, want %v", sequence, testCase.wantSequence)
			}
			if testCase.wantType == "" {
				if store.customerEventCalls != 0 || events.calls != 0 {
					t.Fatalf("idempotent no-op wrote events: customer_events=%d event_log=%d", store.customerEventCalls, events.calls)
				}
				return
			}
			assertMatchingEvents(t, store.customerEvents[0], events.events[0], testCase.wantType, testCase.wantType+":fixed-key", mutationTime().UTC(), map[string]json.RawMessage{"customer_id": json.RawMessage(`19`), "tag_id": json.RawMessage(`29`), "actor": json.RawMessage(`"admin:7"`)})
		})
	}
}

func TestCustomerMutationMakesANewEventKeyForEveryTransactionAttempt(t *testing.T) {
	for _, operation := range mutationOperations() {
		t.Run(operation.name, func(t *testing.T) {
			uow := &fakeMutationUoW{attempts: 3}
			store, events := &fakeMutationStore{}, &fakeMutationAppender{}
			operation.configure(store)
			service := newTestMutationService(uow, store, events)
			keys := []string{"retry-one", "retry-two", "retry-three"}
			keyCalls := 0
			service.newEventKey = func() (string, error) {
				key := keys[keyCalls]
				keyCalls++
				return key, nil
			}

			customer, err := operation.run(service)
			if err != nil {
				t.Fatalf("mutation error = %v", err)
			}
			if operation.name == "update" || operation.name == "set stage" {
				if !reflect.DeepEqual(customer, mutationCustomer()) {
					t.Fatalf("customer = %#v, want %#v", customer, mutationCustomer())
				}
			} else {
				assertZeroCustomer(t, customer)
			}
			if uow.calls != 1 || uow.callbacks != 3 || operation.storeCalls(store) != 3 || store.customerEventCalls != 3 || events.calls != 3 || keyCalls != 3 {
				t.Fatalf("retry calls = uow:%d callbacks:%d store:%d customer_events:%d event_log:%d keys:%d, want 1/3/3/3/3/3", uow.calls, uow.callbacks, operation.storeCalls(store), store.customerEventCalls, events.calls, keyCalls)
			}
			if !reflect.DeepEqual(events.attempts, []int{1, 2, 3}) {
				t.Fatalf("event attempts = %v, want [1 2 3]", events.attempts)
			}
			for index, event := range events.events {
				if event.IdempotencyKey != operation.eventType+":"+keys[index] {
					t.Fatalf("attempt %d idempotency key = %q, want %q", index+1, event.IdempotencyKey, operation.eventType+":"+keys[index])
				}
			}
		})
	}
}

func TestCustomerMutationFailsClosedForClockAndEventKeyFailures(t *testing.T) {
	keyErr := errors.New("event key source failed")
	failures := []struct {
		name      string
		configure func(*CustomerMutationService)
		original  error
	}{
		{name: "zero clock", configure: func(service *CustomerMutationService) { service.now = func() time.Time { return time.Time{} } }},
		{name: "event key error", configure: func(service *CustomerMutationService) {
			service.newEventKey = func() (string, error) { return "", keyErr }
		}, original: keyErr},
		{name: "empty event key", configure: func(service *CustomerMutationService) {
			service.newEventKey = func() (string, error) { return "", nil }
		}},
	}

	for _, operation := range mutationOperations() {
		for _, failure := range failures {
			t.Run(operation.name+"/"+failure.name, func(t *testing.T) {
				store, events := &fakeMutationStore{}, &fakeMutationAppender{}
				operation.configure(store)
				service := newTestMutationService(&fakeMutationUoW{}, store, events)
				failure.configure(service)

				customer, err := operation.run(service)
				if !errors.Is(err, ErrCustomerMutationFailed) {
					t.Fatalf("error = %v, want ErrCustomerMutationFailed", err)
				}
				if failure.original != nil && !errors.Is(err, failure.original) {
					t.Fatalf("error = %v, want original %v", err, failure.original)
				}
				assertZeroCustomer(t, customer)
				if store.customerEventCalls != 0 || events.calls != 0 {
					t.Fatalf("metadata failure wrote events: customer_events=%d event_log=%d", store.customerEventCalls, events.calls)
				}
			})
		}
	}
}

func assertZeroCustomer(t *testing.T, customer CustomerRecord) {
	t.Helper()
	if !reflect.DeepEqual(customer, CustomerRecord{}) {
		t.Fatalf("customer = %#v, want zero value", customer)
	}
}

func assertScopeOwnerStaffIDs(t *testing.T, actual []*int64, expected *int64, count int) {
	t.Helper()
	if len(actual) != count {
		t.Fatalf("scope values = %v, want %d values", actual, count)
	}
	for index, value := range actual {
		if expected == nil {
			if value != nil {
				t.Fatalf("scope value[%d] = %d, want nil global scope", index, *value)
			}
			continue
		}
		if value == nil || *value != *expected {
			t.Fatalf("scope value[%d] = %v, want owner %d", index, value, *expected)
		}
	}
}

func assertMatchingEvents(t *testing.T, customerEvent CustomerEventAppend, domainEvent eventport.Event, wantType, wantKey string, wantOccurredAt time.Time, wantPayload map[string]json.RawMessage) {
	t.Helper()
	if customerEvent.CustomerID != contactport.CustomerID(domainEvent.CustomerID) {
		t.Fatalf("customer IDs = customer_events:%d event_log:%d, want equal", customerEvent.CustomerID, domainEvent.CustomerID)
	}
	if customerEvent.EventType != wantType || domainEvent.Type != wantType {
		t.Fatalf("event types = customer_events:%q event_log:%q, want %q", customerEvent.EventType, domainEvent.Type, wantType)
	}
	if !bytes.Equal(customerEvent.Payload, domainEvent.Payload) {
		t.Fatalf("payloads differ: customer_events=%s event_log=%s", customerEvent.Payload, domainEvent.Payload)
	}
	if !customerEvent.OccurredAt.Equal(domainEvent.OccurredAt) || !domainEvent.OccurredAt.Equal(wantOccurredAt) || customerEvent.OccurredAt.Location() != time.UTC || domainEvent.OccurredAt.Location() != time.UTC {
		t.Fatalf("occurredAt = customer_events:%v event_log:%v, want %v UTC", customerEvent.OccurredAt, domainEvent.OccurredAt, wantOccurredAt)
	}
	if domainEvent.IdempotencyKey != wantKey {
		t.Fatalf("idempotency key = %q, want %q", domainEvent.IdempotencyKey, wantKey)
	}
	var actual map[string]json.RawMessage
	if err := json.Unmarshal(domainEvent.Payload, &actual); err != nil {
		t.Fatalf("payload = %s, want valid JSON: %v", domainEvent.Payload, err)
	}
	if len(actual) != len(wantPayload) {
		t.Fatalf("payload fields = %v, want %v", actual, wantPayload)
	}
	for field, expected := range wantPayload {
		if got, ok := actual[field]; !ok || !bytes.Equal(got, expected) {
			t.Fatalf("payload[%q] = %s, want %s", field, got, expected)
		}
	}
}
