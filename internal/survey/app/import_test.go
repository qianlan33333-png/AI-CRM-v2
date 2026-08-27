package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type importTestTxKey struct{}

type importTestUOW struct {
	calls int
}

var _ platformport.UnitOfWork = (*importTestUOW)(nil)

func (uow *importTestUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(context.WithValue(ctx, importTestTxKey{}, true))
}

type importTestStore struct {
	receipt       Receipt
	hasReceipt    bool
	nextID        int64
	writeCalls    int
	completeCalls int
	questionnaire surveyport.ImportQuestionnaire
	questions     []surveyport.ImportQuestion
	options       []surveyport.ImportOption
	submissions   []surveyport.ImportSubmission
	answers       []importTestAnswer
}

type importTestAnswer struct {
	submissionID   int64
	questionID     int64
	selectedOption json.RawMessage
	value          surveyport.ImportAnswer
}

var _ ImportStore = (*importTestStore)(nil)

func (s *importTestStore) Reserve(ctx context.Context, operation string, reservation Reservation) (Receipt, bool, error) {
	if err := importTestTx(ctx); err != nil {
		return Receipt{}, false, err
	}
	if !s.hasReceipt {
		s.hasReceipt = true
		s.receipt = Receipt{
			ID: 900, Operation: operation, ActorScope: reservation.ActorScope,
			KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest,
			State: "in_progress",
		}
		return s.receipt, true, nil
	}
	return s.receipt, false, nil
}

func (s *importTestStore) Complete(ctx context.Context, id int64, snapshot json.RawMessage, _ time.Time) (Receipt, error) {
	if err := importTestTx(ctx); err != nil {
		return Receipt{}, err
	}
	if id != s.receipt.ID || s.receipt.State != "in_progress" {
		return Receipt{}, ErrUnavailable
	}
	s.completeCalls++
	s.receipt.State = "completed"
	s.receipt.ResultSnapshot = append(json.RawMessage(nil), snapshot...)
	return s.receipt, nil
}

func (s *importTestStore) CreateImportedQuestionnaire(ctx context.Context, value surveyport.ImportQuestionnaire, actor int64) (int64, error) {
	if err := importTestTx(ctx); err != nil {
		return 0, err
	}
	s.writeCalls++
	s.questionnaire = value
	if actor < 1 {
		return 0, ErrInvalidImport
	}
	return s.newID(), nil
}

func (s *importTestStore) CreateImportedQuestion(ctx context.Context, _ int64, value surveyport.ImportQuestion, _ json.RawMessage) (int64, error) {
	if err := importTestTx(ctx); err != nil {
		return 0, err
	}
	s.writeCalls++
	s.questions = append(s.questions, value)
	return s.newID(), nil
}

func (s *importTestStore) CreateImportedOption(ctx context.Context, _ int64, value surveyport.ImportOption) (int64, error) {
	if err := importTestTx(ctx); err != nil {
		return 0, err
	}
	s.writeCalls++
	s.options = append(s.options, value)
	return s.newID(), nil
}

func (s *importTestStore) CreateImportedSubmission(ctx context.Context, _ int64, value surveyport.ImportSubmission) (int64, error) {
	if err := importTestTx(ctx); err != nil {
		return 0, err
	}
	s.writeCalls++
	s.submissions = append(s.submissions, value)
	return s.newID(), nil
}

func (s *importTestStore) CreateImportedAnswer(ctx context.Context, submissionID, questionID int64, value surveyport.ImportAnswer, selectedOptions json.RawMessage) (int64, error) {
	if err := importTestTx(ctx); err != nil {
		return 0, err
	}
	s.writeCalls++
	s.answers = append(s.answers, importTestAnswer{
		submissionID: submissionID, questionID: questionID,
		selectedOption: append(json.RawMessage(nil), selectedOptions...), value: value,
	})
	return s.newID(), nil
}

func (s *importTestStore) newID() int64 {
	s.nextID++
	return s.nextID
}

func importTestTx(ctx context.Context) error {
	if ctx == nil || ctx.Value(importTestTxKey{}) != true {
		return errors.New("test store requires transaction context")
	}
	return nil
}

func importTestAggregate() surveyport.ValidatedImportAggregate {
	createdAt := time.Date(2025, 7, 1, 8, 0, 0, 0, time.UTC)
	submittedAt := createdAt.Add(5 * time.Minute)
	return surveyport.ValidatedImportAggregate{
		Questionnaire: surveyport.ImportQuestionnaire{
			SourceID: 1001, Slug: "history-survey", Name: "History Survey", Title: "历史问卷",
			Description: "历史定义", AnswerDisplayMode: surveyport.AllInOne, AssessmentConfig: json.RawMessage(`{}`),
			Version: 1, CreatedAt: createdAt, UpdatedAt: createdAt,
		},
		Questions: []surveyport.ImportQuestion{{
			SourceID: 2001, SourceQuestionnaireID: 1001, Type: surveyport.SingleChoice,
			Title: "目标", Required: false, SortOrder: 0, Validation: json.RawMessage(`{}`),
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}},
		Options: []surveyport.ImportOption{{
			SourceID: 3001, SourceQuestionID: 2001, OptionText: "增长", SortOrder: 0,
			TagCodes: json.RawMessage(`[]`), CreatedAt: createdAt, UpdatedAt: createdAt,
		}},
		Submissions: []surveyport.ImportSubmission{{
			SourceID: 4001, SourceQuestionnaireID: 1001, UnionID: "union-history-1",
			MatchedBy: "legacy", FinalTags: json.RawMessage(`[]`), SubmittedAt: submittedAt, CreatedAt: submittedAt,
		}},
		Answers: []surveyport.ImportAnswer{{
			SourceID: 5001, SourceSubmissionID: 4001, SourceQuestionID: 2001,
			QuestionType: surveyport.SingleChoice, QuestionTitle: "目标", SortOrder: 0,
			SelectedOptions: []surveyport.ImportAnswerOption{{SourceOptionID: 3001, OptionText: "增长"}}, CreatedAt: submittedAt,
		}},
	}
}

func importTestRequest() surveyport.ImportRequest {
	return surveyport.ImportRequest{
		MigrationActor: 7, RunID: "survey-history-run-2025", IdempotencyKey: "survey-history-key-001",
		Aggregate: importTestAggregate(),
	}
}

func newImportTestService(uow *importTestUOW, store *importTestStore) *ImportService {
	return NewImportServiceWithClock(uow, store, func() time.Time {
		return time.Date(2026, 8, 28, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	})
}

func TestImportWritesOneAggregateInOneUnitOfWork(t *testing.T) {
	uow, store := &importTestUOW{}, &importTestStore{nextID: 100}
	result, err := newImportTestService(uow, store).Import(context.Background(), importTestRequest())
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if uow.calls != 1 || store.writeCalls != 5 || store.completeCalls != 1 {
		t.Fatalf("calls = uow:%d writes:%d complete:%d, want 1/5/1", uow.calls, store.writeCalls, store.completeCalls)
	}
	if result.ReceiptID != 900 || result.Replayed || result.ImportedQuestions != 1 || result.ImportedOptions != 1 || result.ImportedSubmissions != 1 || result.ImportedAnswers != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Mapping.Questionnaires[1001] < 1 || result.Mapping.Questions[2001].QuestionnaireID != result.Mapping.Questionnaires[1001] || result.Mapping.Options[3001].QuestionID != result.Mapping.Questions[2001].TargetID || result.Mapping.Submissions[4001].QuestionnaireID != result.Mapping.Questionnaires[1001] || result.Mapping.Answers[5001] < 1 {
		t.Fatalf("incomplete ID mapping: %+v", result.Mapping)
	}
	if !store.questionnaire.CreatedAt.Equal(time.Date(2025, 7, 1, 8, 0, 0, 0, time.UTC)) || !store.submissions[0].SubmittedAt.Equal(time.Date(2025, 7, 1, 8, 5, 0, 0, time.UTC)) {
		t.Fatalf("historical timestamps were not preserved: %+v %+v", store.questionnaire, store.submissions)
	}
	var selected []struct {
		OptionID int64 `json:"option_id"`
	}
	if json.Unmarshal(store.answers[0].selectedOption, &selected) != nil || len(selected) != 1 || selected[0].OptionID == 3001 || selected[0].OptionID < 1 {
		t.Fatalf("answer did not resolve to V2 option ID: %s", store.answers[0].selectedOption)
	}
}

func TestImportReplaysSameDigestWithoutWrites(t *testing.T) {
	uow, store := &importTestUOW{}, &importTestStore{nextID: 100}
	service := newImportTestService(uow, store)
	request := importTestRequest()
	first, err := service.Import(context.Background(), request)
	if err != nil {
		t.Fatalf("first Import() error = %v", err)
	}
	second, err := service.Import(context.Background(), request)
	if err != nil {
		t.Fatalf("replay Import() error = %v", err)
	}
	if !second.Replayed || second.ReceiptID != first.ReceiptID || store.writeCalls != 5 || store.completeCalls != 1 || uow.calls != 2 {
		t.Fatalf("replay did not stay read-only: first=%+v second=%+v writes=%d complete=%d uow=%d", first, second, store.writeCalls, store.completeCalls, uow.calls)
	}
	if second.Mapping.Questionnaires[1001] != first.Mapping.Questionnaires[1001] || second.Mapping.Answers[5001] != first.Mapping.Answers[5001] {
		t.Fatalf("replay mapping changed: first=%+v second=%+v", first.Mapping, second.Mapping)
	}
}

func TestImportRejectsPayloadConflictWithoutWrites(t *testing.T) {
	uow, store := &importTestUOW{}, &importTestStore{nextID: 100}
	service := newImportTestService(uow, store)
	request := importTestRequest()
	if _, err := service.Import(context.Background(), request); err != nil {
		t.Fatalf("first Import() error = %v", err)
	}
	conflict := request
	conflict.Aggregate.Questionnaire.Title = "不同历史定义"
	if _, err := service.Import(context.Background(), conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting payload error = %v, want ErrConflict", err)
	}
	if store.writeCalls != 5 || store.completeCalls != 1 {
		t.Fatalf("conflict performed writes: writes=%d complete=%d", store.writeCalls, store.completeCalls)
	}
}

func TestImportRejectsBrokenSourceRelationsBeforeTransaction(t *testing.T) {
	uow, store := &importTestUOW{}, &importTestStore{nextID: 100}
	request := importTestRequest()
	request.Aggregate.Options[0].SourceQuestionID = 9999
	_, err := newImportTestService(uow, store).Import(context.Background(), request)
	if !errors.Is(err, ErrInvalidImport) || uow.calls != 0 || store.writeCalls != 0 {
		t.Fatalf("broken relation result = %v, uow=%d writes=%d", err, uow.calls, store.writeCalls)
	}
}

func TestImportRequiresTransactionBoundStoreContext(t *testing.T) {
	store := &importTestStore{}
	if _, err := store.CreateImportedQuestionnaire(context.Background(), importTestAggregate().Questionnaire, 7); err == nil || err.Error() != "test store requires transaction context" {
		t.Fatalf("direct writer error = %v", err)
	}
}
