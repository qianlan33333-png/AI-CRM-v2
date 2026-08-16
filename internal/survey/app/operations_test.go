package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestQuestionnaireOperationsReadAndPageCarrier(t *testing.T) {
	service, dependencies := newOperationsTestService()
	dependencies.store.setProjection(OperationsProjection{
		QuestionnaireID: 1,
		Completion:      CompletionConfiguration{NavigationTargetID: "target-channel", ChannelID: 7},
		ExternalPush:    ExternalPushConfiguration{Enabled: true, ConfigurationReference: "push-config-1"},
	})

	got, err := service.GetOperations(context.Background(), 1, 42)
	if err != nil || got.QuestionnaireID != 1 || got.Completion.NavigationTargetID != "target-channel" || !got.ExternalPush.Enabled {
		t.Fatalf("GET operations = %#v, %v", got, err)
	}
	got.Completion.NavigationTargetID = "mutated-by-caller"
	again, err := service.GetOperations(context.Background(), 1, 42)
	if err != nil || again.Completion.NavigationTargetID != "target-channel" {
		t.Fatalf("GET operations clone = %#v, %v", again, err)
	}

	page, err := service.OperationsPage(context.Background(), 1, 42)
	if err != nil || page.TemplateKey != QuestionnaireOperationsTemplate || page.State != "ready" || page.Operations == nil || len(page.Panels) != 2 || page.Panels[0] != OperationsPanelCompletion || page.Panels[1] != OperationsPanelExternalPush || !page.TestPushAvailable || !page.PushLogsAvailable {
		t.Fatalf("page carrier = %#v, %v", page, err)
	}
	if calls := dependencies.authorizer.snapshot(); len(calls) != 3 || calls[0].Capability != CapabilityAdminRead || calls[0].CSRFRequired {
		t.Fatalf("read authorization requirements = %#v", calls)
	}

	missingPage, err := service.OperationsPage(context.Background(), 999, 42)
	if err != nil || missingPage.State != "placeholder" || missingPage.PlaceholderReason != "not_found" || missingPage.Operations != nil {
		t.Fatalf("missing page = %#v, %v", missingPage, err)
	}
	if _, err = service.GetOperations(context.Background(), 999, 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing API read error = %v, want ErrNotFound", err)
	}

	dependencies.authorizer.denied = true
	if _, err = service.GetOperations(context.Background(), 1, 42); !errors.Is(err, ErrOperationsForbidden) {
		t.Fatalf("denied read error = %v, want ErrOperationsForbidden", err)
	}
}

func TestQuestionnaireOperationsSavesHaveManageAndCSRFRequirements(t *testing.T) {
	service, dependencies := newOperationsTestService()
	completion, err := service.SaveCompletionOperations(context.Background(), SaveCompletionOperationsCommand{
		QuestionnaireID: 1,
		Actor:           42,
		IdempotencyKey:  operationsTestKey("completion"),
		Completion:      CompletionConfiguration{NavigationTargetID: "target-channel", ChannelID: 7},
	})
	if err != nil || completion.Completion.NavigationTargetID != "target-channel" || completion.Completion.ChannelID != 7 {
		t.Fatalf("save completion = %#v, %v", completion, err)
	}
	if dependencies.store.completionSaves() != 1 || dependencies.queue.callCount() != 0 || dependencies.target.callCount() != 1 || dependencies.channel.callCount() != 1 {
		t.Fatalf("completion dependencies: saves=%d queue=%d targets=%d channels=%d", dependencies.store.completionSaves(), dependencies.queue.callCount(), dependencies.target.callCount(), dependencies.channel.callCount())
	}

	external, err := service.SaveExternalPushOperations(context.Background(), SaveExternalPushOperationsCommand{
		QuestionnaireID: 1,
		Actor:           42,
		IdempotencyKey:  operationsTestKey("external"),
		ExternalPush:    ExternalPushConfiguration{Enabled: true, ConfigurationReference: "push-config-1"},
	})
	if err != nil || !external.ExternalPush.Enabled || external.ExternalPush.ConfigurationReference != "push-config-1" {
		t.Fatalf("save external push = %#v, %v", external, err)
	}
	if dependencies.store.externalPushSaves() != 1 || dependencies.queue.callCount() != 0 {
		t.Fatalf("external save must not queue: saves=%d queue=%d", dependencies.store.externalPushSaves(), dependencies.queue.callCount())
	}
	calls := dependencies.authorizer.snapshot()
	if len(calls) != 2 {
		t.Fatalf("authorization calls = %#v", calls)
	}
	for _, call := range calls {
		if call.Capability != CapabilityManageQuestionnaire || !call.CSRFRequired || call.Actor != 42 {
			t.Fatalf("command authorization requirement = %#v", call)
		}
	}
	events := dependencies.events.snapshot()
	if len(events) != 2 || events[0].Type != eventport.EvSurveyUpdated || events[1].Type != eventport.EvSurveyUpdated {
		t.Fatalf("operation events = %#v", events)
	}

	dependencies.authorizer.denied = true
	if _, err = service.SaveExternalPushOperations(context.Background(), SaveExternalPushOperationsCommand{
		QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("denied"), ExternalPush: ExternalPushConfiguration{},
	}); !errors.Is(err, ErrOperationsForbidden) {
		t.Fatalf("denied command error = %v, want ErrOperationsForbidden", err)
	}
	if dependencies.store.externalPushSaves() != 1 {
		t.Fatalf("denied command persisted a configuration")
	}
}

func TestQuestionnaireExternalPushTestIsQueuedAndIdempotent(t *testing.T) {
	service, dependencies := newOperationsTestService()
	dependencies.store.setProjection(OperationsProjection{QuestionnaireID: 1, ExternalPush: ExternalPushConfiguration{Enabled: true, ConfigurationReference: "push-config-1"}})
	command := QueueExternalPushTestCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("queued")}

	first, err := service.QueueExternalPushTest(context.Background(), command)
	if err != nil {
		t.Fatalf("queue test = %v", err)
	}
	if first.TestRunID != "test-run-1" || first.Status != ExternalPushTestQueued || first.AttemptCount != 0 || first.SideEffectExecuted || first.ProviderResultReceived || first.UnknownAfterDispatch {
		t.Fatalf("unsafe queue result = %#v", first)
	}
	second, err := service.QueueExternalPushTest(context.Background(), command)
	if err != nil || second != first {
		t.Fatalf("queue replay = %#v, %v; first=%#v", second, err, first)
	}
	if dependencies.queue.callCount() != 1 || dependencies.events.count() != 1 || dependencies.store.completions() != 1 {
		t.Fatalf("queue replay duplicated work: queue=%d events=%d completions=%d", dependencies.queue.callCount(), dependencies.events.count(), dependencies.store.completions())
	}
	queued := dependencies.queue.snapshot()
	if len(queued) != 1 || queued[0].QuestionnaireID != 1 || queued[0].Actor != 42 || queued[0].OperationReceiptID < 1 {
		t.Fatalf("queued work = %#v", queued)
	}
}

func TestQuestionnaireExternalPushTestFailsClosed(t *testing.T) {
	service, dependencies := newOperationsTestService()
	blocked := QueueExternalPushTestCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("unconfigured")}
	if _, err := service.QueueExternalPushTest(context.Background(), blocked); !errors.Is(err, ErrExternalPushTestBlocked) {
		t.Fatalf("unconfigured test error = %v, want ErrExternalPushTestBlocked", err)
	}
	if dependencies.queue.callCount() != 0 || dependencies.events.count() != 0 || dependencies.store.completions() != 0 {
		t.Fatalf("unconfigured request should not queue or complete")
	}

	dependencies.store.setProjection(OperationsProjection{QuestionnaireID: 1, ExternalPush: ExternalPushConfiguration{Enabled: true, ConfigurationReference: "push-config-1"}})
	dependencies.queue.setResult(ExternalPushTestResult{TestRunID: "test-run-unsafe", Status: ExternalPushTestQueued, SideEffectExecuted: true})
	if _, err := service.QueueExternalPushTest(context.Background(), QueueExternalPushTestCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("unsafe")}); !errors.Is(err, ErrExternalPushTestBlocked) {
		t.Fatalf("side-effect result error = %v, want ErrExternalPushTestBlocked", err)
	}
	dependencies.queue.setResult(ExternalPushTestResult{TestRunID: "test-run-unknown", Status: ExternalPushTestQueued, UnknownAfterDispatch: true})
	if _, err := service.QueueExternalPushTest(context.Background(), QueueExternalPushTestCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("unknown")}); !errors.Is(err, ErrExternalPushTestBlocked) {
		t.Fatalf("unknown-after-dispatch error = %v, want ErrExternalPushTestBlocked", err)
	}
	if dependencies.events.count() != 0 || dependencies.store.completions() != 0 || dependencies.queue.callCount() != 2 {
		t.Fatalf("unsafe queue response must not become success: queue=%d events=%d completions=%d", dependencies.queue.callCount(), dependencies.events.count(), dependencies.store.completions())
	}
}

func TestQuestionnaireOperationsBoundariesAndConflicts(t *testing.T) {
	service, dependencies := newOperationsTestService()
	invalidURL := SaveCompletionOperationsCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("url"), Completion: CompletionConfiguration{NavigationTargetID: "https://provider.invalid"}}
	if _, err := service.SaveCompletionOperations(context.Background(), invalidURL); !errors.Is(err, ErrInvalidOperations) {
		t.Fatalf("raw URL error = %v, want ErrInvalidOperations", err)
	}
	if dependencies.target.callCount() != 0 || dependencies.store.completionSaves() != 0 {
		t.Fatalf("raw URL reached dependency or store")
	}

	missingChannel := SaveCompletionOperationsCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("missing-channel"), Completion: CompletionConfiguration{NavigationTargetID: "target-channel"}}
	if _, err := service.SaveCompletionOperations(context.Background(), missingChannel); !errors.Is(err, ErrInvalidOperations) {
		t.Fatalf("missing channel error = %v, want ErrInvalidOperations", err)
	}
	dependencies.channel.setChannel(CompletionChannel{ID: 7, Selectable: false})
	nonSelectable := SaveCompletionOperationsCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("disabled-channel"), Completion: CompletionConfiguration{NavigationTargetID: "target-channel", ChannelID: 7}}
	if _, err := service.SaveCompletionOperations(context.Background(), nonSelectable); !errors.Is(err, ErrInvalidOperations) {
		t.Fatalf("non-selectable channel error = %v, want ErrInvalidOperations", err)
	}

	key := operationsTestKey("conflict")
	first := SaveExternalPushOperationsCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: key, ExternalPush: ExternalPushConfiguration{}}
	if _, err := service.SaveExternalPushOperations(context.Background(), first); err != nil {
		t.Fatalf("first save = %v", err)
	}
	conflict := first
	conflict.ExternalPush = ExternalPushConfiguration{Enabled: true, ConfigurationReference: "push-config-1"}
	if _, err := service.SaveExternalPushOperations(context.Background(), conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("idempotency conflict error = %v, want ErrConflict", err)
	}

	dependencies.store.setReadError(errors.New("database unavailable"))
	if _, err := service.GetOperations(context.Background(), 1, 42); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unavailable API read = %v, want ErrUnavailable", err)
	}
	page, err := service.OperationsPage(context.Background(), 1, 42)
	if err != nil || page.State != "placeholder" || page.PlaceholderReason != "unavailable" {
		t.Fatalf("unavailable page = %#v, %v", page, err)
	}
}

func operationsTestKey(name string) string { return fmt.Sprintf("operations-test-key-%s", name) }

type operationsDependencies struct {
	store      *operationsTestStore
	authorizer *operationsTestAuthorizer
	target     *operationsTestTargets
	channel    *operationsTestChannels
	queue      *operationsTestQueue
	events     *operationsTestEvents
}

func newOperationsTestService() (*OperationsService, operationsDependencies) {
	dependencies := operationsDependencies{
		store:      newOperationsTestStore(),
		authorizer: &operationsTestAuthorizer{},
		target:     newOperationsTestTargets(),
		channel:    newOperationsTestChannels(),
		queue:      &operationsTestQueue{result: ExternalPushTestResult{TestRunID: "test-run-1", Status: ExternalPushTestQueued}},
		events:     &operationsTestEvents{},
	}
	dependencies.store.setProjection(OperationsProjection{QuestionnaireID: 1, Completion: CompletionConfiguration{}, ExternalPush: ExternalPushConfiguration{}})
	dependencies.target.setTarget(CompletionTarget{ID: "target-channel", Available: true, RequiresChannelBinding: true})
	dependencies.target.setTarget(CompletionTarget{ID: "target-standalone", Available: true, RequiresChannelBinding: false})
	dependencies.channel.setChannel(CompletionChannel{ID: 7, Selectable: true})
	service := NewOperationsService(operationsTestUOW{}, dependencies.store, dependencies.authorizer, dependencies.target, dependencies.channel, dependencies.queue, dependencies.events)
	service.now = func() time.Time { return time.Date(2026, time.August, 16, 10, 0, 0, 0, time.UTC) }
	return service, dependencies
}

type operationsTestUOW struct{}

func (operationsTestUOW) Within(ctx context.Context, apply func(context.Context) error) error {
	return apply(ctx)
}

type operationsTestAuthorizer struct {
	mu     sync.Mutex
	denied bool
	calls  []OperationsAccess
}

func (value *operationsTestAuthorizer) RequireQuestionnaireOperations(_ context.Context, access OperationsAccess) error {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.calls = append(value.calls, access)
	if value.denied {
		return errors.New("denied")
	}
	return nil
}

func (value *operationsTestAuthorizer) snapshot() []OperationsAccess {
	value.mu.Lock()
	defer value.mu.Unlock()
	return append([]OperationsAccess{}, value.calls...)
}

type operationsTestTargets struct {
	mu      sync.Mutex
	targets map[string]CompletionTarget
	calls   int
	err     error
}

func newOperationsTestTargets() *operationsTestTargets {
	return &operationsTestTargets{targets: map[string]CompletionTarget{}}
}

func (value *operationsTestTargets) ReadCompletionTarget(_ context.Context, id string) (CompletionTarget, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.calls++
	if value.err != nil {
		return CompletionTarget{}, value.err
	}
	target, ok := value.targets[id]
	if !ok {
		return CompletionTarget{}, ErrNotFound
	}
	return target, nil
}

func (value *operationsTestTargets) setTarget(target CompletionTarget) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.targets[target.ID] = target
}

func (value *operationsTestTargets) callCount() int {
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.calls
}

type operationsTestChannels struct {
	mu       sync.Mutex
	channels map[int64]CompletionChannel
	calls    int
	err      error
}

func newOperationsTestChannels() *operationsTestChannels {
	return &operationsTestChannels{channels: map[int64]CompletionChannel{}}
}

func (value *operationsTestChannels) ReadChannelForCompletion(_ context.Context, id int64) (CompletionChannel, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.calls++
	if value.err != nil {
		return CompletionChannel{}, value.err
	}
	channel, ok := value.channels[id]
	if !ok {
		return CompletionChannel{}, ErrNotFound
	}
	return channel, nil
}

func (value *operationsTestChannels) setChannel(channel CompletionChannel) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.channels[channel.ID] = channel
}

func (value *operationsTestChannels) callCount() int {
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.calls
}

type operationsTestQueue struct {
	mu     sync.Mutex
	result ExternalPushTestResult
	err    error
	items  []QueuedExternalPushTest
}

func (value *operationsTestQueue) QueueExternalPushTest(_ context.Context, command QueuedExternalPushTest) (ExternalPushTestResult, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.items = append(value.items, command)
	if value.err != nil {
		return ExternalPushTestResult{}, value.err
	}
	return value.result, nil
}

func (value *operationsTestQueue) setResult(result ExternalPushTestResult) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.result = result
}

func (value *operationsTestQueue) callCount() int {
	value.mu.Lock()
	defer value.mu.Unlock()
	return len(value.items)
}

func (value *operationsTestQueue) snapshot() []QueuedExternalPushTest {
	value.mu.Lock()
	defer value.mu.Unlock()
	return append([]QueuedExternalPushTest{}, value.items...)
}

type operationsTestEvents struct {
	mu    sync.Mutex
	items []eventport.Event
}

func (value *operationsTestEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.items = append(value.items, event)
	return eventport.EventID(len(value.items)), nil
}

func (value *operationsTestEvents) count() int {
	value.mu.Lock()
	defer value.mu.Unlock()
	return len(value.items)
}

func (value *operationsTestEvents) snapshot() []eventport.Event {
	value.mu.Lock()
	defer value.mu.Unlock()
	return append([]eventport.Event{}, value.items...)
}

type operationsTestStore struct {
	mu                 sync.Mutex
	projections        map[surveyport.ID]OperationsProjection
	receipts           map[string]OperationsReceipt
	readErr            error
	nextReceiptID      int64
	completionWrites   int
	externalPushWrites int
	completeWrites     int
}

func newOperationsTestStore() *operationsTestStore {
	return &operationsTestStore{projections: map[surveyport.ID]OperationsProjection{}, receipts: map[string]OperationsReceipt{}}
}

func (value *operationsTestStore) ReadOperations(_ context.Context, id surveyport.ID) (OperationsProjection, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.readErr != nil {
		return OperationsProjection{}, value.readErr
	}
	result, ok := value.projections[id]
	if !ok {
		return OperationsProjection{}, ErrNotFound
	}
	return cloneOperationsProjection(result), nil
}

func (value *operationsTestStore) SaveCompletionOperations(_ context.Context, id surveyport.ID, completion CompletionConfiguration, _ time.Time) (OperationsProjection, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.readErr != nil {
		return OperationsProjection{}, value.readErr
	}
	result, ok := value.projections[id]
	if !ok {
		return OperationsProjection{}, ErrNotFound
	}
	result.Completion = cloneCompletionConfiguration(completion)
	value.projections[id] = result
	value.completionWrites++
	return cloneOperationsProjection(result), nil
}

func (value *operationsTestStore) SaveExternalPushOperations(_ context.Context, id surveyport.ID, external ExternalPushConfiguration, _ time.Time) (OperationsProjection, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.readErr != nil {
		return OperationsProjection{}, value.readErr
	}
	result, ok := value.projections[id]
	if !ok {
		return OperationsProjection{}, ErrNotFound
	}
	result.ExternalPush = cloneExternalPushConfiguration(external)
	value.projections[id] = result
	value.externalPushWrites++
	return cloneOperationsProjection(result), nil
}

func (value *operationsTestStore) ReserveOperations(_ context.Context, operation string, reservation OperationsReservation) (OperationsReceipt, bool, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	key := operationsReceiptKey(operation, reservation.ActorScope, reservation.KeyDigest)
	if receipt, ok := value.receipts[key]; ok {
		return cloneOperationsReceipt(receipt), false, nil
	}
	value.nextReceiptID++
	receipt := OperationsReceipt{ID: value.nextReceiptID, Operation: operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, State: "in_progress"}
	value.receipts[key] = receipt
	return cloneOperationsReceipt(receipt), true, nil
}

func (value *operationsTestStore) CompleteOperations(_ context.Context, id int64, snapshot json.RawMessage, _ time.Time) (OperationsReceipt, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	for key, receipt := range value.receipts {
		if receipt.ID != id {
			continue
		}
		receipt.State = "completed"
		receipt.ResultSnapshot = append(json.RawMessage{}, snapshot...)
		value.receipts[key] = receipt
		value.completeWrites++
		return cloneOperationsReceipt(receipt), nil
	}
	return OperationsReceipt{}, ErrNotFound
}

func (value *operationsTestStore) setProjection(projection OperationsProjection) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.projections[projection.QuestionnaireID] = cloneOperationsProjection(projection)
}

func (value *operationsTestStore) setReadError(err error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.readErr = err
}

func (value *operationsTestStore) completionSaves() int {
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.completionWrites
}

func (value *operationsTestStore) externalPushSaves() int {
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.externalPushWrites
}

func (value *operationsTestStore) completions() int {
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.completeWrites
}

func operationsReceiptKey(operation, actor string, digest [32]byte) string {
	return fmt.Sprintf("%s:%s:%x", operation, actor, digest)
}

func cloneOperationsReceipt(value OperationsReceipt) OperationsReceipt {
	value.ResultSnapshot = append(json.RawMessage{}, value.ResultSnapshot...)
	return value
}

func TestQuestionnaireOperationsReceiptKeysAreActorScoped(t *testing.T) {
	service, dependencies := newOperationsTestService()
	command := SaveExternalPushOperationsCommand{QuestionnaireID: 1, Actor: 42, IdempotencyKey: operationsTestKey("actor-scoped"), ExternalPush: ExternalPushConfiguration{}}
	if _, err := service.SaveExternalPushOperations(context.Background(), command); err != nil {
		t.Fatalf("actor 42 save = %v", err)
	}
	command.Actor = 43
	if _, err := service.SaveExternalPushOperations(context.Background(), command); err != nil {
		t.Fatalf("actor 43 independent save = %v", err)
	}
	if dependencies.store.externalPushSaves() != 2 || dependencies.store.completions() != 2 {
		t.Fatalf("actor scope did not isolate receipts")
	}
	if digest := sha256.Sum256([]byte(command.IdempotencyKey)); operationsReceiptKey(operationSaveExternalPush, "admin:42", digest) == operationsReceiptKey(operationSaveExternalPush, "admin:43", digest) {
		t.Fatalf("test receipt key must contain actor scope")
	}
}
