package app

import (
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
	listStages    []contactport.Stage
	insertStage   contactport.Stage
	renameStage   contactport.Stage
	archiveStage  contactport.Stage
	reorderStages []contactport.Stage
	listErr       error
	insertErr     error
	renameErr     error
	archiveErr    error
	reorderErr    error
	listCalls     int
	insertCalls   int
	renameCalls   int
	archiveCalls  int
	reorderCalls  int
	listAttempts  []int
	inserted      []contactport.CreateStageCommand
	renamed       []contactport.RenameStageCommand
	archived      []contactport.ArchiveStageCommand
	reordered     [][]contactport.StageID
	sequence      *[]string
	receipt       StageReceipt
	receiptOwned  bool
}

func (repository *fakeStageRepository) ListStages(ctx context.Context) ([]contactport.Stage, error) {
	repository.listCalls++
	repository.listAttempts = append(repository.listAttempts, stageAttempt(ctx))
	if repository.sequence != nil {
		*repository.sequence = append(*repository.sequence, "repository.list")
	}
	return repository.listStages, repository.listErr
}

func (repository *fakeStageRepository) GetStage(_ context.Context, id contactport.StageID) (contactport.Stage, error) {
	if repository.archiveStage.ID == id {
		return repository.archiveStage, nil
	}
	for _, stage := range repository.listStages {
		if stage.ID == id {
			return stage, nil
		}
	}
	return contactport.Stage{}, contactport.ErrStageNotFound
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

func (repository *fakeStageRepository) ReorderStages(_ context.Context, ids []contactport.StageID) ([]contactport.Stage, error) {
	repository.reorderCalls++
	repository.reordered = append(repository.reordered, append([]contactport.StageID(nil), ids...))
	if repository.sequence != nil {
		*repository.sequence = append(*repository.sequence, "repository.reorder")
	}
	return repository.reorderStages, repository.reorderErr
}

func (repository *fakeStageRepository) ArchiveStage(_ context.Context, command contactport.ArchiveStageCommand, _ time.Time) (contactport.Stage, error) {
	repository.archiveCalls++
	repository.archived = append(repository.archived, command)
	if repository.sequence != nil {
		*repository.sequence = append(*repository.sequence, "repository.archive")
	}
	return repository.archiveStage, repository.archiveErr
}

func (repository *fakeStageRepository) ReserveStageReceipt(_ context.Context, reservation StageReceiptReservation) (StageReceipt, bool, error) {
	if repository.receipt.ID == 0 {
		repository.receipt = StageReceipt{ID: 1, Operation: reservation.Operation, Actor: reservation.Actor, KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, State: "in_progress"}
		repository.receiptOwned = true
	}
	return repository.receipt, repository.receiptOwned, nil
}

func (repository *fakeStageRepository) CompleteStageReceipt(_ context.Context, _ int64, ids []contactport.StageID, _ time.Time) (StageReceipt, error) {
	repository.receipt.State = "completed"
	repository.receipt.ResultIDs = append([]contactport.StageID(nil), ids...)
	repository.receiptOwned = false
	return repository.receipt, nil
}

func stageAttempt(ctx context.Context) int {
	attempt, _ := ctx.Value(stageAttemptContextKey{}).(int)
	return attempt
}

func (repository *fakeStageRepository) calls() int {
	return repository.listCalls + repository.insertCalls + repository.renameCalls + repository.archiveCalls + repository.reorderCalls
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
					Name: "prospect", SortOrder: -2, Actor: "admin:7", IdempotencyKey: "stage-create-key-01",
				})
			},
			stage:         contactport.Stage{ID: 21, Name: "prospect", SortOrder: -2, Config: json.RawMessage(`[]`)},
			wantSequence:  []string{"repository.insert", "event.append"},
			wantType:      "stage.created",
			wantKeyPrefix: "stage.create:",
			wantPayload: map[string]json.RawMessage{
				"stage_ids": json.RawMessage(`[21]`),
				"actor":     json.RawMessage(`"admin:7"`),
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
					ID: 24, Name: "qualified", Actor: "admin:8", IdempotencyKey: "stage-rename-key-01",
				})
			},
			stage:         contactport.Stage{ID: 24, Name: "qualified", SortOrder: 5, Config: json.RawMessage(`{"color":"blue"}`)},
			wantSequence:  []string{"repository.rename", "event.append"},
			wantType:      "stage.renamed",
			wantKeyPrefix: "stage.rename:",
			wantPayload: map[string]json.RawMessage{
				"stage_ids": json.RawMessage(`[24]`),
				"actor":     json.RawMessage(`"admin:8"`),
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
			wantCallbacks: 0,
		},
		{
			name: "rename invalid clock",
			run: func(service *StageService) (contactport.Stage, error) {
				return service.RenameStage(context.Background(), validRenameStageCommand())
			},
			configure: func(_ *fakeStageUoW, _ *fakeStageRepository, _ *fakeStageAppender, service *StageService) {
				service.now = func() time.Time { return time.Time{} }
			},
			wantCallbacks: 0,
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
			wantUOWCalls := 1
			if strings.Contains(testCase.name, "invalid clock") {
				wantUOWCalls = 0
			}
			if uow.calls != wantUOWCalls || uow.callbackCalls != testCase.wantCallbacks || repository.calls() != testCase.wantRepoCalls || events.calls != testCase.wantEventCalls {
				t.Fatalf("failure calls = uow:%d callbacks:%d repository:%d events:%d, want %d/%d/%d/%d", uow.calls, uow.callbackCalls, repository.calls(), events.calls, wantUOWCalls, testCase.wantCallbacks, testCase.wantRepoCalls, testCase.wantEventCalls)
			}
		})
	}
}

func validCreateStageCommand() contactport.CreateStageCommand {
	return contactport.CreateStageCommand{Name: "prospect", SortOrder: 2, Config: json.RawMessage(`[]`), Actor: "admin:1", IdempotencyKey: "stage-create-key-01"}
}

func TestStageServiceReorderAndArchiveUseOneLocalUOWEvent(t *testing.T) {
	first := contactport.Stage{ID: 1, Name: "new", SortOrder: 0, Config: json.RawMessage(`{}`)}
	second := contactport.Stage{ID: 2, Name: "qualified", SortOrder: 1, Config: json.RawMessage(`{}`)}

	t.Run("reorder exact active set", func(t *testing.T) {
		sequence := []string{}
		uow := &fakeStageUoW{}
		repository := &fakeStageRepository{listStages: []contactport.Stage{first, second}, reorderStages: []contactport.Stage{second, first}, sequence: &sequence}
		events := &fakeStageAppender{sequence: &sequence}
		stages, err := newTestStageService(uow, repository, events).ReorderStages(context.Background(), contactport.ReorderStagesCommand{IDs: []contactport.StageID{2, 1}, Actor: "admin:9", IdempotencyKey: "stage-reorder-key"})
		if err != nil || !reflect.DeepEqual(stages, []contactport.Stage{second, first}) {
			t.Fatalf("ReorderStages() = %#v, %v", stages, err)
		}
		if !reflect.DeepEqual(sequence, []string{"repository.list", "repository.reorder", "event.append"}) || len(events.events) != 1 || events.events[0].Type != "stage.reordered" {
			t.Fatalf("sequence/events = %#v/%#v", sequence, events.events)
		}
	})

	t.Run("archive preserves customer references", func(t *testing.T) {
		sequence := []string{}
		uow := &fakeStageUoW{}
		repository := &fakeStageRepository{archiveStage: first, sequence: &sequence}
		events := &fakeStageAppender{sequence: &sequence}
		stage, err := newTestStageService(uow, repository, events).ArchiveStage(context.Background(), contactport.ArchiveStageCommand{ID: 1, Actor: "admin:9", IdempotencyKey: "stage-archive-key"})
		if err != nil || stage.ID != 1 || !reflect.DeepEqual(sequence, []string{"repository.archive", "event.append"}) || len(events.events) != 1 || events.events[0].Type != "stage.archived" {
			t.Fatalf("ArchiveStage() stage=%#v err=%v sequence=%#v events=%#v", stage, err, sequence, events.events)
		}
	})
}

func TestStageServiceReorderRejectsStaleOrDuplicateSetsBeforeWrite(t *testing.T) {
	for name, command := range map[string]contactport.ReorderStagesCommand{
		"duplicate": {IDs: []contactport.StageID{1, 1}, Actor: "admin:1", IdempotencyKey: "stage-duplicate-key"},
		"missing":   {IDs: []contactport.StageID{1}, Actor: "admin:1", IdempotencyKey: "stage-missing-key---"},
	} {
		t.Run(name, func(t *testing.T) {
			uow := &fakeStageUoW{}
			repository := &fakeStageRepository{listStages: []contactport.Stage{{ID: 1}, {ID: 2}}}
			_, err := newTestStageService(uow, repository, &fakeStageAppender{}).ReorderStages(context.Background(), command)
			if !errors.Is(err, contactport.ErrInvalidStage) && !errors.Is(err, contactport.ErrStageConflict) {
				t.Fatalf("ReorderStages(%#v) error = %v", command, err)
			}
			if repository.reorderCalls != 0 {
				t.Fatalf("reorder write calls = %d, want 0", repository.reorderCalls)
			}
		})
	}
}

func TestStageCreateReceiptReplaysAndRejectsPayloadDrift(t *testing.T) {
	created := contactport.Stage{ID: 7, Name: "prospect", SortOrder: 0, Config: json.RawMessage(`{}`)}
	uow := &fakeStageUoW{}
	repository := &fakeStageRepository{insertStage: created, listStages: []contactport.Stage{created}}
	events := &fakeStageAppender{}
	service := newTestStageService(uow, repository, events)
	command := contactport.CreateStageCommand{Name: "prospect", Actor: "admin:7", IdempotencyKey: "stage-create-key-07"}

	first, err := service.CreateStage(context.Background(), command)
	if err != nil || first.ID != created.ID {
		t.Fatalf("first CreateStage() = %#v, %v", first, err)
	}
	second, err := service.CreateStage(context.Background(), command)
	if err != nil || second.ID != created.ID {
		t.Fatalf("replay CreateStage() = %#v, %v", second, err)
	}
	if repository.insertCalls != 1 || events.calls != 1 {
		t.Fatalf("replay writes/events = %d/%d, want 1/1", repository.insertCalls, events.calls)
	}
	_, err = service.CreateStage(context.Background(), contactport.CreateStageCommand{Name: "different", Actor: command.Actor, IdempotencyKey: command.IdempotencyKey})
	if !errors.Is(err, contactport.ErrStageConflict) || repository.insertCalls != 1 || events.calls != 1 {
		t.Fatalf("payload drift err/writes/events = %v/%d/%d", err, repository.insertCalls, events.calls)
	}
}

func validRenameStageCommand() contactport.RenameStageCommand {
	return contactport.RenameStageCommand{ID: 1, Name: "qualified", Actor: "admin:1", IdempotencyKey: "stage-rename-key-01"}
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
