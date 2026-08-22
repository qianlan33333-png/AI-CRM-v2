package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestOperationsSaveCompletionIsAtomicAndIdempotent(t *testing.T) {
	service, store, events, uow := newOperationsTestService()
	command := surveyport.SaveCompletionOperationsCommand{
		QuestionnaireID: 7,
		Actor:           41,
		IdempotencyKey:  localTestRequestID("completion-one"),
		Completion: surveyport.CompletionOperations{
			NavigationTargetID: "target-7",
			ChannelID:          9,
		},
	}
	got, err := service.SaveCompletion(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if got.Completion != command.Completion || !got.LocalOnly || store.projections[7].Completion != command.Completion {
		t.Fatalf("completion readback=%#v stored=%#v", got, store.projections[7])
	}
	if len(store.receipts) != 1 || receiptState(store.receipts) != "completed" || len(events.items) != 1 || events.items[0].Type != eventport.EvSurveyUpdated {
		t.Fatalf("receipt/event state=%#v/%#v", store.receipts, events.items)
	}
	if store.nonTransactionalCalls != 0 || uow.rollbacks != 0 {
		t.Fatalf("transaction boundary calls=%d rollbacks=%d", store.nonTransactionalCalls, uow.rollbacks)
	}
	replay, err := service.SaveCompletion(context.Background(), command)
	if err != nil || replay != got || store.saveCompletionCalls != 1 || len(events.items) != 1 {
		t.Fatalf("replay=%#v saves=%d events=%d err=%v", replay, store.saveCompletionCalls, len(events.items), err)
	}
	invalid := command
	invalid.IdempotencyKey = localTestRequestID("completion-two")
	invalid.Completion.NavigationTargetID = "https://provider.invalid"
	if _, err := service.SaveCompletion(context.Background(), invalid); !errors.Is(err, ErrInvalidOperations) || len(store.receipts) != 1 || len(events.items) != 1 {
		t.Fatalf("unsafe reference error=%v receipts=%d events=%d", err, len(store.receipts), len(events.items))
	}
}

func localTestRequestID(label string) string {
	return "request-" + label + "-0000000000000000"
}

func TestOperationsStrictReadbackAndEventFailuresRollback(t *testing.T) {
	t.Run("mismatched readback", func(t *testing.T) {
		service, store, events, uow := newOperationsTestService()
		store.mismatchCompletionReadback = true
		_, err := service.SaveCompletion(context.Background(), surveyport.SaveCompletionOperationsCommand{
			QuestionnaireID: 7, Actor: 41, IdempotencyKey: "survey-operations-readback-0001",
			Completion: surveyport.CompletionOperations{NavigationTargetID: "target-7"},
		})
		if !errors.Is(err, ErrUnavailable) || len(store.receipts) != 0 || len(events.items) != 0 || store.projections[7].Completion.NavigationTargetID != "" || uow.rollbacks != 1 {
			t.Fatalf("readback err=%v receipts=%d events=%d projection=%#v rollbacks=%d", err, len(store.receipts), len(events.items), store.projections[7], uow.rollbacks)
		}
	})
	t.Run("event append failure", func(t *testing.T) {
		service, store, events, uow := newOperationsTestService()
		events.fail = errors.New("event log unavailable")
		_, err := service.SaveExternalPush(context.Background(), surveyport.SaveExternalPushOperationsCommand{
			QuestionnaireID: 7, Actor: 41, IdempotencyKey: "survey-operations-event-0001",
			ExternalPush: surveyport.ExternalPushOperations{Enabled: true, ConfigurationReference: "config-7"},
		})
		if !errors.Is(err, ErrUnavailable) || len(store.receipts) != 0 || len(events.items) != 0 || store.projections[7].ExternalPush.Enabled || uow.rollbacks != 1 {
			t.Fatalf("event err=%v receipts=%d events=%d projection=%#v rollbacks=%d", err, len(store.receipts), len(events.items), store.projections[7], uow.rollbacks)
		}
	})
}

func TestOperationsExternalPushConflictAndQueuedTestHasNoExternalEffect(t *testing.T) {
	service, store, events, _ := newOperationsTestService()
	save := surveyport.SaveExternalPushOperationsCommand{
		QuestionnaireID: 7, Actor: 41, IdempotencyKey: "survey-operations-external-0001",
		ExternalPush: surveyport.ExternalPushOperations{Enabled: true, ConfigurationReference: "config-7"},
	}
	if _, err := service.SaveExternalPush(context.Background(), save); err != nil {
		t.Fatal(err)
	}
	conflict := save
	conflict.ExternalPush.ConfigurationReference = "config-8"
	if _, err := service.SaveExternalPush(context.Background(), conflict); !errors.Is(err, ErrConflict) || store.saveExternalPushCalls != 1 {
		t.Fatalf("idempotency conflict=%v writes=%d", err, store.saveExternalPushCalls)
	}
	testCommand := surveyport.QueueExternalPushTestCommand{QuestionnaireID: 7, Actor: 41, IdempotencyKey: "survey-operations-queue-0001"}
	queued, err := service.QueueExternalPushTest(context.Background(), testCommand)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != ExternalPushTestQueued || queued.AttemptCount != 0 || queued.SideEffectExecuted || queued.ProviderResultReceived || queued.UnknownAfterDispatch || queued.AutoRetryAllowed {
		t.Fatalf("unsafe queued test=%#v", queued)
	}
	if len(store.testRuns) != 1 || store.providerCalls != 0 || len(events.items) != 2 {
		t.Fatalf("runs=%#v providerCalls=%d events=%d", store.testRuns, store.providerCalls, len(events.items))
	}
	replay, err := service.QueueExternalPushTest(context.Background(), testCommand)
	if err != nil || replay != queued || len(store.testRuns) != 1 || len(events.items) != 2 {
		t.Fatalf("queue replay=%#v runs=%d events=%d err=%v", replay, len(store.testRuns), len(events.items), err)
	}
}

func TestOperationsLogsAreScopedAndFailClosed(t *testing.T) {
	service, store, _, _ := newOperationsTestService()
	store.exists[8] = true
	stamp := time.Date(2026, time.August, 22, 8, 0, 0, 0, time.UTC)
	store.testRuns = []surveyport.ExternalPushTest{
		queuedExternalPushTest(3, 7, stamp),
		queuedExternalPushTest(2, 8, stamp),
		queuedExternalPushTest(1, 7, stamp.Add(-time.Minute)),
	}
	global, err := service.ListExternalPushLogs(context.Background(), nil, 2, 0)
	if err != nil || global.Total != 3 || !global.HasMore || len(global.Items) != 2 || global.Items[0].TestRunID != 3 || global.Items[1].TestRunID != 2 || !global.LocalOnly {
		t.Fatalf("global=%#v err=%v", global, err)
	}
	questionnaireID := surveyport.ID(7)
	local, err := service.ListExternalPushLogs(context.Background(), &questionnaireID, 50, 0)
	if err != nil || local.Total != 2 || local.HasMore || len(local.Items) != 2 || local.Items[0].QuestionnaireID != questionnaireID || local.Items[1].QuestionnaireID != questionnaireID {
		t.Fatalf("local=%#v err=%v", local, err)
	}
	store.testRuns[0].ProviderResultReceived = true
	if _, err := service.ListExternalPushLogs(context.Background(), nil, 50, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsafe log projection error=%v", err)
	}
	missing := surveyport.ID(99)
	if _, err := service.ListExternalPushLogs(context.Background(), &missing, 50, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing questionnaire error=%v", err)
	}
}

type operationsTxContextKey struct{}

type operationsTestUOW struct {
	store     *operationsTestStore
	events    *operationsTestEvents
	rollbacks int
	nextID    int
}

func (u *operationsTestUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	u.nextID++
	before := u.store.snapshot()
	eventCount := len(u.events.items)
	err := fn(context.WithValue(ctx, operationsTxContextKey{}, u.nextID))
	if err != nil {
		u.rollbacks++
		u.store.restore(before)
		u.events.items = u.events.items[:eventCount]
	}
	return err
}

type operationsTestStoreSnapshot struct {
	projections           map[surveyport.ID]surveyport.OperationsProjection
	receipts              map[string]OperationsReceipt
	testRuns              []surveyport.ExternalPushTest
	nextTestID            int64
	saveCompletionCalls   int
	saveExternalPushCalls int
	providerCalls         int
	nonTransactionalCalls int
}

type operationsTestStore struct {
	exists                     map[surveyport.ID]bool
	projections                map[surveyport.ID]surveyport.OperationsProjection
	receipts                   map[string]OperationsReceipt
	testRuns                   []surveyport.ExternalPushTest
	nextTestID                 int64
	saveCompletionCalls        int
	saveExternalPushCalls      int
	providerCalls              int
	nonTransactionalCalls      int
	mismatchCompletionReadback bool
}

func newOperationsTestStore() *operationsTestStore {
	return &operationsTestStore{
		exists: map[surveyport.ID]bool{7: true},
		projections: map[surveyport.ID]surveyport.OperationsProjection{
			7: {QuestionnaireID: 7, Completion: surveyport.CompletionOperations{}, ExternalPush: surveyport.ExternalPushOperations{}, LocalOnly: true},
		},
		receipts: map[string]OperationsReceipt{},
	}
}

func (s *operationsTestStore) transactional(ctx context.Context) error {
	if _, ok := ctx.Value(operationsTxContextKey{}).(int); !ok {
		s.nonTransactionalCalls++
		return ErrUnavailable
	}
	return nil
}

func (s *operationsTestStore) ReadOperations(ctx context.Context, id surveyport.ID) (surveyport.OperationsProjection, error) {
	if err := s.transactional(ctx); err != nil {
		return surveyport.OperationsProjection{}, err
	}
	if !s.exists[id] {
		return surveyport.OperationsProjection{}, ErrNotFound
	}
	value, ok := s.projections[id]
	if !ok {
		value = surveyport.OperationsProjection{QuestionnaireID: id, Completion: surveyport.CompletionOperations{}, ExternalPush: surveyport.ExternalPushOperations{}, LocalOnly: true}
	}
	if s.mismatchCompletionReadback {
		value.Completion.NavigationTargetID = "different-target"
	}
	return value, nil
}

func (s *operationsTestStore) SaveCompletionOperations(ctx context.Context, id surveyport.ID, value surveyport.CompletionOperations, _ time.Time) error {
	if err := s.transactional(ctx); err != nil {
		return err
	}
	if !s.exists[id] {
		return ErrNotFound
	}
	projection, _ := s.ReadOperations(ctx, id)
	projection.Completion, projection.QuestionnaireID, projection.LocalOnly = value, id, true
	s.projections[id] = projection
	s.saveCompletionCalls++
	return nil
}

func (s *operationsTestStore) SaveExternalPushOperations(ctx context.Context, id surveyport.ID, value surveyport.ExternalPushOperations, _ time.Time) error {
	if err := s.transactional(ctx); err != nil {
		return err
	}
	if !s.exists[id] {
		return ErrNotFound
	}
	projection, _ := s.ReadOperations(ctx, id)
	projection.ExternalPush, projection.QuestionnaireID, projection.LocalOnly = value, id, true
	s.projections[id] = projection
	s.saveExternalPushCalls++
	return nil
}

func (s *operationsTestStore) ReserveOperations(ctx context.Context, operation string, reservation OperationsReservation) (OperationsReceipt, bool, error) {
	if err := s.transactional(ctx); err != nil {
		return OperationsReceipt{}, false, err
	}
	key := fmt.Sprintf("%s:%s:%x", operation, reservation.ActorScope, reservation.KeyDigest)
	if receipt, ok := s.receipts[key]; ok {
		return receipt, false, nil
	}
	receipt := OperationsReceipt{
		ID: int64(len(s.receipts) + 1), Operation: operation, ActorScope: reservation.ActorScope,
		KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, State: "in_progress",
	}
	s.receipts[key] = receipt
	return receipt, true, nil
}

func (s *operationsTestStore) CompleteOperations(ctx context.Context, id int64, snapshot json.RawMessage, _ time.Time) (OperationsReceipt, error) {
	if err := s.transactional(ctx); err != nil {
		return OperationsReceipt{}, err
	}
	for key, receipt := range s.receipts {
		if receipt.ID != id || receipt.State != "in_progress" {
			continue
		}
		receipt.State, receipt.ResultSnapshot = "completed", append(json.RawMessage{}, snapshot...)
		s.receipts[key] = receipt
		return receipt, nil
	}
	return OperationsReceipt{}, ErrUnavailable
}

func (s *operationsTestStore) CreateQueuedExternalPushTest(ctx context.Context, id surveyport.ID, _ int64, now time.Time) (int64, error) {
	if err := s.transactional(ctx); err != nil {
		return 0, err
	}
	if !s.exists[id] {
		return 0, ErrNotFound
	}
	s.nextTestID++
	s.testRuns = append(s.testRuns, queuedExternalPushTest(s.nextTestID, id, now))
	return s.nextTestID, nil
}

func (s *operationsTestStore) ReadExternalPushTest(ctx context.Context, id surveyport.ID, testRunID int64) (surveyport.ExternalPushTest, error) {
	if err := s.transactional(ctx); err != nil {
		return surveyport.ExternalPushTest{}, err
	}
	for _, value := range s.testRuns {
		if value.QuestionnaireID == id && value.TestRunID == testRunID {
			return value, nil
		}
	}
	return surveyport.ExternalPushTest{}, ErrNotFound
}

func (s *operationsTestStore) CountExternalPushTests(ctx context.Context, id *surveyport.ID) (int64, error) {
	if err := s.transactional(ctx); err != nil {
		return 0, err
	}
	var total int64
	for _, value := range s.testRuns {
		if id == nil || value.QuestionnaireID == *id {
			total++
		}
	}
	return total, nil
}

func (s *operationsTestStore) ListExternalPushTests(ctx context.Context, id *surveyport.ID, limit, offset int32) ([]surveyport.ExternalPushTest, error) {
	if err := s.transactional(ctx); err != nil {
		return nil, err
	}
	values := make([]surveyport.ExternalPushTest, 0, len(s.testRuns))
	for _, value := range s.testRuns {
		if id == nil || value.QuestionnaireID == *id {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(left, right int) bool {
		if values[left].CreatedAt.Equal(values[right].CreatedAt) {
			return values[left].TestRunID > values[right].TestRunID
		}
		return values[left].CreatedAt.After(values[right].CreatedAt)
	})
	if int(offset) >= len(values) {
		return []surveyport.ExternalPushTest{}, nil
	}
	end := int(offset + limit)
	if end > len(values) {
		end = len(values)
	}
	return append([]surveyport.ExternalPushTest{}, values[offset:end]...), nil
}

func (s *operationsTestStore) snapshot() operationsTestStoreSnapshot {
	projections := make(map[surveyport.ID]surveyport.OperationsProjection, len(s.projections))
	for key, value := range s.projections {
		projections[key] = value
	}
	receipts := make(map[string]OperationsReceipt, len(s.receipts))
	for key, value := range s.receipts {
		value.ResultSnapshot = append(json.RawMessage{}, value.ResultSnapshot...)
		receipts[key] = value
	}
	return operationsTestStoreSnapshot{
		projections: projections, receipts: receipts, testRuns: append([]surveyport.ExternalPushTest{}, s.testRuns...), nextTestID: s.nextTestID,
		saveCompletionCalls: s.saveCompletionCalls, saveExternalPushCalls: s.saveExternalPushCalls, providerCalls: s.providerCalls,
		nonTransactionalCalls: s.nonTransactionalCalls,
	}
}

func (s *operationsTestStore) restore(snapshot operationsTestStoreSnapshot) {
	s.projections, s.receipts, s.testRuns, s.nextTestID = snapshot.projections, snapshot.receipts, snapshot.testRuns, snapshot.nextTestID
	s.saveCompletionCalls, s.saveExternalPushCalls, s.providerCalls, s.nonTransactionalCalls = snapshot.saveCompletionCalls, snapshot.saveExternalPushCalls, snapshot.providerCalls, snapshot.nonTransactionalCalls
}

type operationsTestEvents struct {
	items []eventport.Event
	fail  error
}

func (e *operationsTestEvents) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	if _, ok := ctx.Value(operationsTxContextKey{}).(int); !ok {
		return 0, ErrUnavailable
	}
	if e.fail != nil {
		return 0, e.fail
	}
	e.items = append(e.items, event)
	return eventport.EventID(len(e.items)), nil
}

func newOperationsTestService() (*OperationsService, *operationsTestStore, *operationsTestEvents, *operationsTestUOW) {
	store := newOperationsTestStore()
	events := &operationsTestEvents{}
	uow := &operationsTestUOW{store: store, events: events}
	service := NewOperationsService(uow, store, events)
	service.now = func() time.Time { return time.Date(2026, time.August, 22, 9, 0, 0, 0, time.UTC) }
	return service, store, events, uow
}

func queuedExternalPushTest(id int64, questionnaireID surveyport.ID, stamp time.Time) surveyport.ExternalPushTest {
	return surveyport.ExternalPushTest{
		TestRunID: id, QuestionnaireID: questionnaireID, Status: ExternalPushTestQueued, AttemptCount: 0,
		SideEffectExecuted: false, ProviderResultReceived: false, UnknownAfterDispatch: false, AutoRetryAllowed: false,
		CreatedAt: stamp.UTC(), UpdatedAt: stamp.UTC(),
	}
}

func receiptState(receipts map[string]OperationsReceipt) string {
	for _, receipt := range receipts {
		return receipt.State
	}
	return ""
}
