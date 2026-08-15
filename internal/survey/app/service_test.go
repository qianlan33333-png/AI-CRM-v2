package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestQuestionnaireCreateReplayListAndGet(t *testing.T) {
	service, store, events := testService()
	created, err := service.Create(context.Background(), testCommand(7, "questionnaire-create-0001"))
	if err != nil || created.ID != 1 || created.Slug != "questionnaire-1" || len(created.Questions) != 2 || len(events.items) != 1 {
		t.Fatalf("Create() = %#v, events=%d, err=%v", created, len(events.items), err)
	}
	if events.items[0].Type != eventport.EvSurveyCreated || string(events.items[0].Payload) != `{"questionnaire_id":1,"actor":7}` {
		t.Fatalf("event = %#v", events.items[0])
	}
	replay, err := service.Create(context.Background(), testCommand(7, "questionnaire-create-0001"))
	if err != nil || replay.ID != created.ID || store.createCalls != 1 || len(events.items) != 1 {
		t.Fatalf("replay = %#v, creates=%d events=%d err=%v", replay, store.createCalls, len(events.items), err)
	}
	page, err := service.ListLegacy(context.Background(), 50, 0)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("ListLegacy() = %#v, %v", page, err)
	}
	detail, err := service.Get(context.Background(), created.ID)
	if err != nil || detail.ID != created.ID || detail.Questions[0].Options[0].OptionText != "增长" {
		t.Fatalf("Get() = %#v, %v", detail, err)
	}
}

func TestQuestionnaireCreateRejectsFrozenBoundariesWithoutEffects(t *testing.T) {
	tests := []struct {
		name string
		edit func(*surveyport.CreateCommand)
		want error
	}{
		{"F02 enabled", func(c *surveyport.CreateCommand) { c.AssessmentEnabled = true }, ErrAssessmentUnavailable},
		{"F02 config", func(c *surveyport.CreateCommand) { c.AssessmentConfig = json.RawMessage(`{"dimension":"growth"}`) }, ErrAssessmentUnavailable},
		{"F02 rules", func(c *surveyport.CreateCommand) { c.ScoreRules = []surveyport.ScoreRule{{SortOrder: 0}} }, ErrAssessmentUnavailable},
		{"mobile maximum", func(c *surveyport.CreateCommand) { value := 33; c.Questions[1].Validation.MaximumLength = &value }, ErrInvalidSchema},
		{"duplicate option order", func(c *surveyport.CreateCommand) { c.Questions[0].Options[1].SortOrder = 0 }, ErrInvalidSchema},
		{"short key", func(c *surveyport.CreateCommand) { c.IdempotencyKey = "short" }, ErrInvalidQuestionnaire},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, store, events := testService()
			command := testCommand(7, "questionnaire-create-0002")
			test.edit(&command)
			if _, err := service.Create(context.Background(), command); !errors.Is(err, test.want) {
				t.Fatalf("Create() error=%v want=%v", err, test.want)
			}
			if store.createCalls != 0 || len(store.receipts) != 0 || len(events.items) != 0 {
				t.Fatal("invalid input leaked business effects")
			}
		})
	}
}

func TestQuestionnaireIdempotencyIsActorScopedAndPayloadBound(t *testing.T) {
	service, store, events := testService()
	key := "questionnaire-shared-0001"
	first, err := service.Create(context.Background(), testCommand(7, key))
	if err != nil {
		t.Fatal(err)
	}
	conflict := testCommand(7, key)
	conflict.Title = "不同载荷"
	if _, err = service.Create(context.Background(), conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload conflict=%v", err)
	}
	second, err := service.Create(context.Background(), testCommand(8, key))
	if err != nil || second.ID == first.ID || store.createCalls != 2 || len(events.items) != 2 || events.items[0].IdempotencyKey == events.items[1].IdempotencyKey {
		t.Fatalf("actor isolation first=%d second=%d creates=%d events=%#v err=%v", first.ID, second.ID, store.createCalls, events.items, err)
	}
}

func TestQuestionnaireManagementUpdateEnableDisableDeleteAndDuplicate(t *testing.T) {
	service, _, events := testService()
	created, err := service.Create(context.Background(), testCommand(7, "questionnaire-create-management-0001"))
	if err != nil {
		t.Fatal(err)
	}
	update := surveyport.UpdateCommand{Questionnaire: created, Actor: 7, IdempotencyKey: "questionnaire-update-management-0001"}
	update.Title = "更新后的标题"
	updated, err := service.Update(context.Background(), created.ID, update)
	if err != nil || updated.Title != "更新后的标题" || updated.Version != 2 || len(events.items) != 2 || events.items[1].Type != eventport.EvSurveyUpdated {
		t.Fatalf("Update()=%#v events=%#v err=%v", updated, events.items, err)
	}
	replay, err := service.Update(context.Background(), created.ID, update)
	if err != nil || replay.ID != updated.ID || len(events.items) != 2 {
		t.Fatalf("revision replay=%#v events=%d err=%v", replay, len(events.items), err)
	}
	disabled, err := service.SetDisabled(context.Background(), created.ID, true, 7, "questionnaire-disable-management-0001")
	if err != nil || !disabled.IsDisabled {
		t.Fatalf("disable=%#v err=%v", disabled, err)
	}
	if _, err = service.Delete(context.Background(), created.ID, 7, "questionnaire-delete-management-0001"); err != nil {
		t.Fatalf("Delete() err=%v", err)
	}
	if _, err = service.Get(context.Background(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete err=%v", err)
	}

	second, err := service.Create(context.Background(), testCommand(7, "questionnaire-create-management-0002"))
	if err != nil {
		t.Fatal(err)
	}
	copy, err := service.Duplicate(context.Background(), second.ID, 7, "questionnaire-duplicate-management-0001", "", "")
	if err != nil || copy.ID == second.ID || !copy.IsDisabled || copy.Title != second.Title+" Copy" {
		t.Fatalf("duplicate=%#v err=%v", copy, err)
	}
}

func TestQuestionnaireReadFailsClosedForUnorderedStoreRows(t *testing.T) {
	service, store, _ := testService()
	for i := 0; i < 2; i++ {
		command := testCommand(7, fmt.Sprintf("questionnaire-create-%04d", i+10))
		command.Name = fmt.Sprintf("问卷%d", i)
		command.Title = command.Name
		if _, err := service.Create(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	store.reverse = true
	if _, err := service.ListLegacy(context.Background(), 50, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unordered list error=%v", err)
	}
}

func testCommand(actor int64, key string) surveyport.CreateCommand {
	return surveyport.CreateCommand{
		Questionnaire: surveyport.Questionnaire{
			Name: "入门问卷", Title: "欢迎填写", Description: "用于了解你的目标",
			AnswerDisplayMode: surveyport.AllInOne, AssessmentConfig: json.RawMessage(`{}`),
			Questions: []surveyport.Question{
				{Type: surveyport.SingleChoice, Title: "你的目标", Required: true, SortOrder: 0,
					Options: []surveyport.Option{{OptionText: "增长", TagCodes: []string{}, SortOrder: 0}, {OptionText: "交付", TagCodes: []string{}, SortOrder: 1}}},
				{Type: surveyport.Mobile, Title: "手机号", Required: true, SortOrder: 1, Options: []surveyport.Option{}},
			}, ScoreRules: []surveyport.ScoreRule{},
		}, Actor: actor, IdempotencyKey: key,
	}
}

type testUOW struct{}

func (testUOW) Within(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type testStore struct {
	items       []surveyport.Questionnaire
	receipts    map[string]Receipt
	createCalls int
	reverse     bool
}

func (s *testStore) ListOffset(_ context.Context, limit, offset int32) ([]surveyport.Questionnaire, error) {
	items := append([]surveyport.Questionnaire{}, s.items...)
	if s.reverse && len(items) > 1 {
		items[0], items[1] = items[1], items[0]
	}
	if int(offset) >= len(items) {
		return []surveyport.Questionnaire{}, nil
	}
	end := int(offset + limit)
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], nil
}
func (s *testStore) Count(context.Context) (int64, error) { return int64(len(s.items)), nil }
func (s *testStore) Get(_ context.Context, id surveyport.ID) (surveyport.Questionnaire, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return surveyport.Questionnaire{}, ErrNotFound
}
func (s *testStore) Create(_ context.Context, command surveyport.CreateCommand, now time.Time) (surveyport.Questionnaire, error) {
	s.createCalls++
	item := command.Questionnaire
	item.ID, item.CreatedBy, item.Version = surveyport.ID(len(s.items)+1), command.Actor, 1
	if item.Slug == "" {
		item.Slug = fmt.Sprintf("questionnaire-%d", item.ID)
	}
	item.CreatedAt, item.UpdatedAt, item.ScoreRules = now, now, []surveyport.ScoreRule{}
	nextID := surveyport.ID(100)
	for questionIndex := range item.Questions {
		item.Questions[questionIndex].ID = nextID
		nextID++
		for optionIndex := range item.Questions[questionIndex].Options {
			item.Questions[questionIndex].Options[optionIndex].ID = nextID
			nextID++
		}
	}
	s.items = append(s.items, item)
	return item, nil
}
func (s *testStore) Update(_ context.Context, id surveyport.ID, command surveyport.UpdateCommand, now time.Time) (surveyport.Questionnaire, error) {
	for index, item := range s.items {
		if item.ID != id {
			continue
		}
		next := command.Questionnaire
		next.ID, next.CreatedBy, next.Version = id, item.CreatedBy, item.Version+1
		next.CreatedAt, next.UpdatedAt, next.SubmissionCount, next.ScoreRules = item.CreatedAt, now, item.SubmissionCount, []surveyport.ScoreRule{}
		nextID := surveyport.ID(300)
		for questionIndex := range next.Questions {
			next.Questions[questionIndex].ID = nextID
			nextID++
			for optionIndex := range next.Questions[questionIndex].Options {
				next.Questions[questionIndex].Options[optionIndex].ID = nextID
				nextID++
			}
		}
		s.items[index] = next
		return next, nil
	}
	return surveyport.Questionnaire{}, ErrNotFound
}
func (s *testStore) SetDisabled(_ context.Context, id surveyport.ID, disabled bool, now time.Time) (surveyport.Questionnaire, error) {
	for index := range s.items {
		if s.items[index].ID == id {
			s.items[index].IsDisabled, s.items[index].Version, s.items[index].UpdatedAt = disabled, s.items[index].Version+1, now
			return s.items[index], nil
		}
	}
	return surveyport.Questionnaire{}, ErrNotFound
}
func (s *testStore) Delete(_ context.Context, id surveyport.ID) (surveyport.Questionnaire, error) {
	for index, item := range s.items {
		if item.ID == id {
			if !item.IsDisabled {
				return surveyport.Questionnaire{}, ErrInvalidQuestionnaire
			}
			s.items = append(s.items[:index], s.items[index+1:]...)
			return item, nil
		}
	}
	return surveyport.Questionnaire{}, ErrNotFound
}
func (s *testStore) Reserve(_ context.Context, operation string, value Reservation) (Receipt, bool, error) {
	key := operation + ":" + value.ActorScope + ":" + fmt.Sprintf("%x", value.KeyDigest)
	if old, ok := s.receipts[key]; ok {
		return old, false, nil
	}
	receipt := Receipt{ID: int64(len(s.receipts) + 1), Operation: operation, ActorScope: value.ActorScope, KeyDigest: value.KeyDigest, PayloadDigest: value.PayloadDigest, State: "in_progress"}
	s.receipts[key] = receipt
	return receipt, true, nil
}
func (s *testStore) Complete(_ context.Context, id int64, snapshot json.RawMessage, _ time.Time) (Receipt, error) {
	for key, receipt := range s.receipts {
		if receipt.ID == id {
			receipt.State, receipt.ResultSnapshot = "completed", append(json.RawMessage{}, snapshot...)
			s.receipts[key] = receipt
			return receipt, nil
		}
	}
	return Receipt{}, ErrUnavailable
}

func (s *testStore) ReserveManagement(ctx context.Context, operation string, value Reservation) (Receipt, bool, error) {
	return s.Reserve(ctx, operation, value)
}

func (s *testStore) CompleteManagement(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (Receipt, error) {
	return s.Complete(ctx, id, snapshot, now)
}

type testEvents struct{ items []eventport.Event }

func (e *testEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	e.items = append(e.items, event)
	return eventport.EventID(len(e.items)), nil
}

func testService() (*Service, *testStore, *testEvents) {
	store, events := &testStore{receipts: map[string]Receipt{}}, &testEvents{}
	service := NewService(testUOW{}, store, events)
	service.now = func() time.Time { return time.Date(2026, 8, 15, 7, 0, 0, 0, time.UTC) }
	return service, store, events
}
