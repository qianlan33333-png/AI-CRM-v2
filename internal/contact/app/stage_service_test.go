package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type stageAttemptContextKey struct{}

type fakeStageUoW struct {
	calls, callbackCalls int
	attempts             int
	err                  error
}

func (uow *fakeStageUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	attempts := uow.attempts
	if attempts == 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		uow.callbackCalls++
		if err := callback(context.WithValue(ctx, stageAttemptContextKey{}, attempt)); err != nil {
			return err
		}
	}
	return nil
}

type fakeStageRepository struct {
	listStages   []contactport.Stage
	insertStage  contactport.Stage
	renameStage  contactport.Stage
	listErr      error
	insertErr    error
	renameErr    error
	listCalls    int
	insertCalls  int
	renameCalls  int
	listAttempts []int
	inserted     []contactport.CreateStageCommand
	renamed      []contactport.RenameStageCommand
	sequence     *[]string
}

func (repository *fakeStageRepository) ListStages(ctx context.Context) ([]contactport.Stage, error) {
	repository.listCalls++
	repository.listAttempts = append(repository.listAttempts, stageAttempt(ctx))
	if repository.sequence != nil {
		*repository.sequence = append(*repository.sequence, "repository.list")
	}
	return repository.listStages, repository.listErr
}

func (repository *fakeStageRepository) InsertStage(ctx context.Context, command contactport.CreateStageCommand) (contactport.Stage, error) {
	repository.insertCalls++
	repository.inserted = append(repository.inserted, command)
	if repository.sequence != nil {
		*repository.sequence = append(*repository.sequence, "repository.insert")
	}
	return repository.insertStage, repository.insertErr
}

func (repository *fakeStageRepository) RenameStage(ctx context.Context, command contactport.RenameStageCommand) (contactport.Stage, error) {
	repository.renameCalls++
	repository.renamed = append(repository.renamed, command)
	if repository.sequence != nil {
		*repository.sequence = append(*repository.sequence, "repository.rename")
	}
	return repository.renameStage, repository.renameErr
}

func stageAttempt(ctx context.Context) int {
	attempt, _ := ctx.Value(stageAttemptContextKey{}).(int)
	return attempt
}

func (repository *fakeStageRepository) calls() int {
	return repository.listCalls + repository.insertCalls + repository.renameCalls
}

type fakeStageAppender struct {
	calls    int
	events   []eventport.Event
	attempts []int
	err      error
	sequence *[]string
}

func (appender *fakeStageAppender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	appender.calls++
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	appender.events = append(appender.events, event)
	appender.attempts = append(appender.attempts, stageAttempt(ctx))
	if appender.sequence != nil {
		*appender.sequence = append(*appender.sequence, "event.append")
	}
	return eventport.EventID(appender.calls), appender.err
}

func newTestStageService(
	uow *fakeStageUoW,
	repository *fakeStageRepository,
	events *fakeStageAppender,
) *StageService {
	return &StageService{
		uow:        uow,
		repository: repository,
		events:     events,
		now: func() time.Time {
			return time.Date(2026, time.August, 11, 9, 30, 0, 0, time.UTC)
		},
		newEventKey: func() (string, error) { return "test-key", nil },
	}
}

func TestStageServiceFailsClosedWithoutRequiredDependencies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*StageService)
	}{
		{name: "unit of work", mutate: func(service *StageService) { service.uow = nil }},
		{name: "repository", mutate: func(service *StageService) { service.repository = nil }},
		{name: "event appender", mutate: func(service *StageService) { service.events = nil }},
		{name: "clock", mutate: func(service *StageService) { service.now = nil }},
		{name: "event key generator", mutate: func(service *StageService) { service.newEventKey = nil }},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeStageUoW{}
			repository := &fakeStageRepository{}
			events := &fakeStageAppender{}
			service := newTestStageService(uow, repository, events)
			testCase.mutate(service)

			stages, err := service.ListStages(context.Background())
			if err == nil {
				t.Fatal("ListStages() error = nil, want missing dependency error")
			}
			if stages != nil {
				t.Fatalf("ListStages() stages = %#v, want nil", stages)
			}
			if uow.calls != 0 || repository.calls() != 0 || events.calls != 0 {
				t.Fatalf("missing dependency reached collaborators: uow=%d repository=%d events=%d", uow.calls, repository.calls(), events.calls)
			}
		})
	}

	t.Run("nil receiver", func(t *testing.T) {
		var service *StageService
		stages, err := service.ListStages(context.Background())
		if err == nil {
			t.Fatal("ListStages() error = nil, want missing dependency error")
		}
		if stages != nil {
			t.Fatalf("ListStages() stages = %#v, want nil", stages)
		}
	})
}

func TestStageServiceRejectsInvalidCommandsBeforeTransaction(t *testing.T) {
	tests := []struct {
		name string
		run  func(*StageService) (contactport.Stage, error)
	}{
		{
			name: "create empty name",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), contactport.CreateStageCommand{Name: "", Actor: "admin:1"})
			},
		},
		{
			name: "create leading name whitespace",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), contactport.CreateStageCommand{Name: " prospect", Actor: "admin:1"})
			},
		},
		{
			name: "create trailing name whitespace",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), contactport.CreateStageCommand{Name: "prospect ", Actor: "admin:1"})
			},
		},
		{
			name: "create empty actor",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), contactport.CreateStageCommand{Name: "prospect", Actor: ""})
			},
		},
		{
			name: "create padded actor",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), contactport.CreateStageCommand{Name: "prospect", Actor: "admin:1 "})
			},
		},
		{
			name: "create invalid config",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), contactport.CreateStageCommand{Name: "prospect", Actor: "admin:1", Config: json.RawMessage(`{`)})
			},
		},
		{
			name: "rename zero id",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), contactport.RenameStageCommand{ID: 0, Name: "qualified", Actor: "admin:1"})
			},
		},
		{
			name: "rename negative id",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), contactport.RenameStageCommand{ID: -1, Name: "qualified", Actor: "admin:1"})
			},
		},
		{
			name: "rename empty name",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), contactport.RenameStageCommand{ID: 1, Name: "", Actor: "admin:1"})
			},
		},
		{
			name: "rename padded name",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), contactport.RenameStageCommand{ID: 1, Name: "qualified ", Actor: "admin:1"})
			},
		},
		{
			name: "rename empty actor",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), contactport.RenameStageCommand{ID: 1, Name: "qualified", Actor: ""})
			},
		},
		{
			name: "rename padded actor",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), contactport.RenameStageCommand{ID: 1, Name: "qualified", Actor: " admin:1"})
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeStageUoW{}
			repository := &fakeStageRepository{}
			events := &fakeStageAppender{}

			stage, err := testCase.run(newTestStageService(uow, repository, events))
			if !errors.Is(err, contactport.ErrInvalidStage) {
				t.Fatalf("command error = %v, want ErrInvalidStage", err)
			}
			assertZeroStage(t, stage)
			if uow.calls != 0 || repository.calls() != 0 || events.calls != 0 {
				t.Fatalf("invalid command reached collaborators: uow=%d repository=%d events=%d", uow.calls, repository.calls(), events.calls)
			}
		})
	}
}

func TestStageServiceListsThroughUnitOfWork(t *testing.T) {
	wantStages := []contactport.Stage{{ID: 4, Name: "prospect", SortOrder: -1, Config: json.RawMessage(`[]`)}}
	uow := &fakeStageUoW{}
	repository := &fakeStageRepository{listStages: wantStages}
	events := &fakeStageAppender{}

	stages, err := newTestStageService(uow, repository, events).ListStages(context.Background())
	if err != nil {
		t.Fatalf("ListStages() error = %v", err)
	}
	if !reflect.DeepEqual(stages, wantStages) {
		t.Fatalf("ListStages() stages = %#v, want %#v", stages, wantStages)
	}
	if uow.calls != 1 || uow.callbackCalls != 1 || repository.listCalls != 1 || events.calls != 0 {
		t.Fatalf("ListStages() calls = uow:%d callbacks:%d repository:%d events:%d, want 1/1/1/0", uow.calls, uow.callbackCalls, repository.listCalls, events.calls)
	}
	if !reflect.DeepEqual(repository.listAttempts, []int{1}) {
		t.Fatalf("ListStages() transaction attempts = %v, want [1]", repository.listAttempts)
	}
}

func TestStageServiceMutationsAppendEventsAfterRepository(t *testing.T) {
	now := time.Date(2026, time.August, 11, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	tests := []struct {
		name          string
		run           func(*StageService) (contactport.Stage, error)
		stage         contactport.Stage
		wantSequence  []string
		wantType      string
		wantKeyPrefix string
		wantPayload   map[string]json.RawMessage
		assertCommand func(*testing.T, *fakeStageRepository)
	}{
		{
			name: "create",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), contactport.CreateStageCommand{
					Name: "prospect", SortOrder: -2, Actor: "admin:7",
				})
			},
			stage:         contactport.Stage{ID: 21, Name: "prospect", SortOrder: -2, Config: json.RawMessage(`[]`)},
			wantSequence:  []string{"repository.insert", "event.append"},
			wantType:      "stage.created",
			wantKeyPrefix: "stage.created:",
			wantPayload: map[string]json.RawMessage{
				"stage_id":   json.RawMessage(`21`),
				"name":       json.RawMessage(`"prospect"`),
				"sort_order": json.RawMessage(`-2`),
				"actor":      json.RawMessage(`"admin:7"`),
			},
			assertCommand: func(t *testing.T, repository *fakeStageRepository) {
				t.Helper()
				if len(repository.inserted) != 1 {
					t.Fatalf("InsertStage() commands = %d, want 1", len(repository.inserted))
				}
				if got := repository.inserted[0].Config; string(got) != `{}` {
					t.Fatalf("InsertStage() config = %s, want {}", got)
				}
			},
		},
		{
			name: "rename",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), contactport.RenameStageCommand{
					ID: 24, Name: "qualified", Actor: "admin:8",
				})
			},
			stage:         contactport.Stage{ID: 24, Name: "qualified", SortOrder: 5, Config: json.RawMessage(`{"color":"blue"}`)},
			wantSequence:  []string{"repository.rename", "event.append"},
			wantType:      "stage.renamed",
			wantKeyPrefix: "stage.renamed:",
			wantPayload: map[string]json.RawMessage{
				"stage_id": json.RawMessage(`24`),
				"name":     json.RawMessage(`"qualified"`),
				"actor":    json.RawMessage(`"admin:8"`),
			},
			assertCommand: func(t *testing.T, repository *fakeStageRepository) {
				t.Helper()
				if len(repository.renamed) != 1 {
					t.Fatalf("RenameStage() commands = %d, want 1", len(repository.renamed))
				}
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			sequence := make([]string, 0, 2)
			uow := &fakeStageUoW{}
			repository := &fakeStageRepository{insertStage: testCase.stage, renameStage: testCase.stage, sequence: &sequence}
			events := &fakeStageAppender{sequence: &sequence}
			service := newTestStageService(uow, repository, events)
			service.now = func() time.Time { return now }
			service.newEventKey = func() (string, error) { return "fixed-key", nil }

			stage, err := testCase.run(service)
			if err != nil {
				t.Fatalf("mutation error = %v", err)
			}
			if !reflect.DeepEqual(stage, testCase.stage) {
				t.Fatalf("mutation stage = %#v, want %#v", stage, testCase.stage)
			}
			if !reflect.DeepEqual(sequence, testCase.wantSequence) {
				t.Fatalf("write sequence = %v, want %v", sequence, testCase.wantSequence)
			}
			if uow.calls != 1 || uow.callbackCalls != 1 || events.calls != 1 {
				t.Fatalf("mutation calls = uow:%d callbacks:%d events:%d, want 1/1/1", uow.calls, uow.callbackCalls, events.calls)
			}
			testCase.assertCommand(t, repository)
			assertStageEvent(t, events.events[0], testCase.wantType, testCase.wantKeyPrefix, now.UTC(), testCase.wantPayload)
		})
	}
}

func TestStageServiceMutationErrorsPropagateAndReturnZeroStage(t *testing.T) {
	repositoryErr := errors.New("repository unavailable")
	appenderErr := errors.New("event append unavailable")
	keyErr := errors.New("event key unavailable")
	uowErr := errors.New("transaction unavailable")
	tests := []struct {
		name           string
		run            func(*StageService) (contactport.Stage, error)
		configure      func(*fakeStageUoW, *fakeStageRepository, *fakeStageAppender, *StageService)
		wantErr        error
		wantCallbacks  int
		wantRepoCalls  int
		wantEventCalls int
	}{
		{
			name: "create unit of work error",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), validCreateStageCommand())
			},
			configure: func(uow *fakeStageUoW, _ *fakeStageRepository, _ *fakeStageAppender, _ *StageService) {
				uow.err = uowErr
			},
			wantErr: uowErr,
		},
		{
			name: "create repository error",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), validCreateStageCommand())
			},
			configure: func(_ *fakeStageUoW, repository *fakeStageRepository, _ *fakeStageAppender, _ *StageService) {
				repository.insertErr = repositoryErr
			},
			wantErr: repositoryErr, wantCallbacks: 1, wantRepoCalls: 1,
		},
		{
			name: "rename repository error",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), validRenameStageCommand())
			},
			configure: func(_ *fakeStageUoW, repository *fakeStageRepository, _ *fakeStageAppender, _ *StageService) {
				repository.renameErr = repositoryErr
			},
			wantErr: repositoryErr, wantCallbacks: 1, wantRepoCalls: 1,
		},
		{
			name: "create appender error",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), validCreateStageCommand())
			},
			configure: func(_ *fakeStageUoW, _ *fakeStageRepository, events *fakeStageAppender, _ *StageService) {
				events.err = appenderErr
			},
			wantErr: appenderErr, wantCallbacks: 1, wantRepoCalls: 1, wantEventCalls: 1,
		},
		{
			name: "rename appender error",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), validRenameStageCommand())
			},
			configure: func(_ *fakeStageUoW, _ *fakeStageRepository, events *fakeStageAppender, _ *StageService) {
				events.err = appenderErr
			},
			wantErr: appenderErr, wantCallbacks: 1, wantRepoCalls: 1, wantEventCalls: 1,
		},
		{
			name: "create invalid clock",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), validCreateStageCommand())
			},
			configure: func(_ *fakeStageUoW, _ *fakeStageRepository, _ *fakeStageAppender, service *StageService) {
				service.now = func() time.Time { return time.Time{} }
			},
			wantCallbacks: 1,
		},
		{
			name: "rename invalid clock",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), validRenameStageCommand())
			},
			configure: func(_ *fakeStageUoW, _ *fakeStageRepository, _ *fakeStageAppender, service *StageService) {
				service.now = func() time.Time { return time.Time{} }
			},
			wantCallbacks: 1,
		},
		{
			name: "create event key error",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), validCreateStageCommand())
			},
			configure: func(_ *fakeStageUoW, _ *fakeStageRepository, _ *fakeStageAppender, service *StageService) {
				service.newEventKey = func() (string, error) { return "", keyErr }
			},
			wantErr: keyErr, wantCallbacks: 1,
		},
		{
			name: "rename event key error",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), validRenameStageCommand())
			},
			configure: func(_ *fakeStageUoW, _ *fakeStageRepository, _ *fakeStageAppender, service *StageService) {
				service.newEventKey = func() (string, error) { return "", keyErr }
			},
			wantErr: keyErr, wantCallbacks: 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeStageUoW{}
			repository := &fakeStageRepository{
				insertStage: contactport.Stage{ID: 1, Name: "prospect"},
				renameStage: contactport.Stage{ID: 1, Name: "qualified"},
			}
			events := &fakeStageAppender{}
			service := newTestStageService(uow, repository, events)
			testCase.configure(uow, repository, events, service)

			stage, err := testCase.run(service)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("mutation error = %v, want %v", err, testCase.wantErr)
				}
			} else if err == nil {
				t.Fatal("mutation error = nil, want failure")
			}
			assertZeroStage(t, stage)
			if uow.calls != 1 || uow.callbackCalls != testCase.wantCallbacks || repository.calls() != testCase.wantRepoCalls || events.calls != testCase.wantEventCalls {
				t.Fatalf("failure calls = uow:%d callbacks:%d repository:%d events:%d, want 1/%d/%d/%d", uow.calls, uow.callbackCalls, repository.calls(), events.calls, testCase.wantCallbacks, testCase.wantRepoCalls, testCase.wantEventCalls)
			}
		})
	}
}

func TestStageServiceGeneratesNewKeyForEachTransactionAttempt(t *testing.T) {
	tests := []struct {
		name       string
		run        func(*StageService) (contactport.Stage, error)
		stage      contactport.Stage
		wantPrefix string
	}{
		{
			name: "create",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.CreateStage(context.Background(), validCreateStageCommand())
			},
			stage: contactport.Stage{ID: 3, Name: "prospect"}, wantPrefix: "stage.created:",
		},
		{
			name: "rename",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), validRenameStageCommand())
			},
			stage: contactport.Stage{ID: 3, Name: "qualified"}, wantPrefix: "stage.renamed:",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeStageUoW{attempts: 3}
			repository := &fakeStageRepository{insertStage: testCase.stage, renameStage: testCase.stage}
			events := &fakeStageAppender{}
			service := newTestStageService(uow, repository, events)
			keys := []string{"retry-one", "retry-two", "retry-three"}
			keyCalls := 0
			service.newEventKey = func() (string, error) {
				key := keys[keyCalls]
				keyCalls++
				return key, nil
			}

			stage, err := testCase.run(service)
			if err != nil {
				t.Fatalf("mutation error = %v", err)
			}
			if !reflect.DeepEqual(stage, testCase.stage) {
				t.Fatalf("mutation stage = %#v, want %#v", stage, testCase.stage)
			}
			if uow.calls != 1 || uow.callbackCalls != 3 || repository.calls() != 3 || events.calls != 3 || keyCalls != 3 {
				t.Fatalf("retry calls = uow:%d callbacks:%d repository:%d events:%d keys:%d, want 1/3/3/3/3", uow.calls, uow.callbackCalls, repository.calls(), events.calls, keyCalls)
			}
			if !reflect.DeepEqual(events.attempts, []int{1, 2, 3}) {
				t.Fatalf("event attempts = %v, want [1 2 3]", events.attempts)
			}

			seen := make(map[string]struct{}, len(events.events))
			for index, event := range events.events {
				if !strings.HasPrefix(event.IdempotencyKey, testCase.wantPrefix) {
					t.Fatalf("event %d key = %q, want prefix %q", index, event.IdempotencyKey, testCase.wantPrefix)
				}
				if event.IdempotencyKey != testCase.wantPrefix+keys[index] {
					t.Fatalf("event %d key = %q, want %q", index, event.IdempotencyKey, testCase.wantPrefix+keys[index])
				}
				if _, exists := seen[event.IdempotencyKey]; exists {
					t.Fatalf("event %d repeated key %q", index, event.IdempotencyKey)
				}
				seen[event.IdempotencyKey] = struct{}{}
			}
		})
	}
}

func TestRandomEventKeyIsDistinct128BitHex(t *testing.T) {
	first, err := randomEventKey()
	if err != nil {
		t.Fatalf("first randomEventKey() error = %v", err)
	}
	second, err := randomEventKey()
	if err != nil {
		t.Fatalf("second randomEventKey() error = %v", err)
	}
	for index, key := range []string{first, second} {
		if len(key) != 32 {
			t.Fatalf("key %d length = %d, want 32", index, len(key))
		}
		decoded, err := hex.DecodeString(key)
		if err != nil || len(decoded) != 16 || strings.ToLower(key) != key {
			t.Fatalf("key %d = %q, want 16-byte lowercase hex: decoded=%x error=%v", index, key, decoded, err)
		}
	}
	if first == second {
		t.Fatalf("randomEventKey() repeated %q", first)
	}
}

func validCreateStageCommand() contactport.CreateStageCommand {
	return contactport.CreateStageCommand{Name: "prospect", SortOrder: 2, Config: json.RawMessage(`[]`), Actor: "admin:1"}
}

func validRenameStageCommand() contactport.RenameStageCommand {
	return contactport.RenameStageCommand{ID: 1, Name: "qualified", Actor: "admin:1"}
}

func assertZeroStage(t *testing.T, stage contactport.Stage) {
	t.Helper()
	if !reflect.DeepEqual(stage, contactport.Stage{}) {
		t.Fatalf("stage = %#v, want zero value", stage)
	}
}

func assertStageEvent(
	t *testing.T,
	event eventport.Event,
	wantType string,
	wantKeyPrefix string,
	wantOccurredAt time.Time,
	wantPayload map[string]json.RawMessage,
) {
	t.Helper()
	if event.Type != wantType {
		t.Fatalf("event type = %q, want %q", event.Type, wantType)
	}
	if event.CustomerID != 0 {
		t.Fatalf("event customer ID = %d, want global-stage zero", event.CustomerID)
	}
	if !event.OccurredAt.Equal(wantOccurredAt) || event.OccurredAt.Location() != time.UTC {
		t.Fatalf("event occurred at = %v (%v), want %v UTC", event.OccurredAt, event.OccurredAt.Location(), wantOccurredAt)
	}
	if !strings.HasPrefix(event.IdempotencyKey, wantKeyPrefix) {
		t.Fatalf("event key = %q, want prefix %q", event.IdempotencyKey, wantKeyPrefix)
	}
	if event.IdempotencyKey != wantKeyPrefix+"fixed-key" {
		t.Fatalf("event key = %q, want %q", event.IdempotencyKey, wantKeyPrefix+"fixed-key")
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("event payload = %s, want valid JSON: %v", event.Payload, err)
	}
	if len(payload) != len(wantPayload) {
		t.Fatalf("event payload keys = %v, want %v", payload, wantPayload)
	}
	for field, want := range wantPayload {
		got, exists := payload[field]
		if !exists || string(got) != string(want) {
			t.Fatalf("event payload[%q] = %s, want %s", field, got, want)
		}
	}
}
