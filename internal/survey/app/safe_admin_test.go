package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type safeAdminUOW struct{ err error }

func (u safeAdminUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	if u.err != nil {
		return u.err
	}
	return fn(ctx)
}

type safeAdminStoreStub struct {
	resultsFn func(context.Context, surveyport.ID) (surveyport.SubmissionResult, error)
	existsFn  func(context.Context, surveyport.ID) (bool, error)
	countFn   func(context.Context, surveyport.ID) (int64, error)
	listFn    func(context.Context, surveyport.ID, int32, int32) ([]surveyport.Submission, error)
}

func (s safeAdminStoreStub) Results(ctx context.Context, id surveyport.ID) (surveyport.SubmissionResult, error) {
	if s.resultsFn == nil {
		return surveyport.SubmissionResult{}, errors.New("unexpected Results")
	}
	return s.resultsFn(ctx, id)
}
func (s safeAdminStoreStub) SubmissionOwnerExists(ctx context.Context, id surveyport.ID) (bool, error) {
	if s.existsFn == nil {
		return false, errors.New("unexpected SubmissionOwnerExists")
	}
	return s.existsFn(ctx, id)
}
func (s safeAdminStoreStub) CountSubmissions(ctx context.Context, id surveyport.ID) (int64, error) {
	if s.countFn == nil {
		return 0, errors.New("unexpected CountSubmissions")
	}
	return s.countFn(ctx, id)
}
func (s safeAdminStoreStub) ListSubmissions(ctx context.Context, id surveyport.ID, limit, offset int32) ([]surveyport.Submission, error) {
	if s.listFn == nil {
		return nil, errors.New("unexpected ListSubmissions")
	}
	return s.listFn(ctx, id, limit, offset)
}
func (safeAdminStoreStub) ExportDefinition(context.Context, surveyport.ID) (string, []surveyport.SubmissionExportQuestion, error) {
	return "", nil, errors.New("unexpected ExportDefinition")
}
func (safeAdminStoreStub) ExportSubmissions(context.Context, surveyport.ID, int32) ([]surveyport.Submission, error) {
	return nil, errors.New("unexpected ExportSubmissions")
}

func TestSafeAnalysisAggregatesOnlyChoiceIdentifiers(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	items := []surveyport.Submission{
		safeSensitiveSubmission(12, stamp, []surveyport.SubmissionAnswer{
			{QuestionID: 10, QuestionType: surveyport.SingleChoice, QuestionTitle: "private-title-2", SortOrder: 0, SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 101, OptionText: "secret-label-2"}}},
			{QuestionID: 20, QuestionType: surveyport.MultiChoice, QuestionTitle: "private-title", SortOrder: 1, SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 203, OptionText: "secret-label"}, {OptionID: 201, OptionText: "another-label"}}},
			{QuestionID: 30, QuestionType: surveyport.Textarea, QuestionTitle: "free-text", SortOrder: 2, TextValue: "respondent free text must disappear"},
			{QuestionID: 40, QuestionType: surveyport.Mobile, QuestionTitle: "mobile", SortOrder: 3, TextValue: "13800138000"},
		}),
		safeSensitiveSubmission(11, stamp.Add(-time.Minute), []surveyport.SubmissionAnswer{
			{QuestionID: 10, QuestionType: surveyport.SingleChoice, QuestionTitle: "private-title-2", SortOrder: 0, SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 102, OptionText: "hidden"}}},
			{QuestionID: 20, QuestionType: surveyport.MultiChoice, QuestionTitle: "private-title", SortOrder: 1, SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 201, OptionText: "hidden"}}},
		}),
	}
	store := safeAdminStoreStub{
		existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
		resultsFn: func(context.Context, surveyport.ID) (surveyport.SubmissionResult, error) {
			return surveyport.SubmissionResult{QuestionnaireID: 7, SubmissionCount: 2, LatestSubmittedAt: stamp, AverageScore: 8.5}, nil
		},
		listFn: func(_ context.Context, id surveyport.ID, limit, offset int32) ([]surveyport.Submission, error) {
			if id != 7 || limit != 2 || offset != 0 {
				t.Fatalf("unexpected list request id=%d limit=%d offset=%d", id, limit, offset)
			}
			return items, nil
		},
	}
	service := NewSafeAdminService(safeAdminUOW{}, store)
	got, err := service.SafeAnalysis(context.Background(), 7, 1, 1)
	if err != nil {
		t.Fatalf("SafeAnalysis() error = %v", err)
	}
	if !got.OK || !got.LocalOnly || got.RealExternalCallExecuted || !got.Deidentified || got.ContainsRawIdentity || got.ContainsFreeText {
		t.Fatalf("unsafe flags: %#v", got)
	}
	if got.TotalQuestions != 2 || len(got.Questions) != 1 || got.Questions[0].QuestionID != 20 {
		t.Fatalf("unexpected aggregate page: %#v", got)
	}
	question := got.Questions[0]
	if question.AnsweredCount != 2 || len(question.Options) != 2 || question.Options[0].OptionID != 201 || question.Options[0].SelectionCount != 2 || question.Options[1].OptionID != 203 || question.Options[1].SelectionCount != 1 {
		t.Fatalf("unexpected option aggregates: %#v", question)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSensitiveText(t, string(encoded), "external-user-secret", "union-secret", "openid-secret", "respondent free text must disappear", "13800138000", "secret-label", "private-title", "result-token-secret", "https://secret.example")
}

func TestSafeAnalysisCapsScanAndMarksPartial(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	calls := 0
	store := safeAdminStoreStub{
		existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
		resultsFn: func(context.Context, surveyport.ID) (surveyport.SubmissionResult, error) {
			return surveyport.SubmissionResult{QuestionnaireID: 9, SubmissionCount: SafeAnalysisScanLimit + 1, LatestSubmittedAt: stamp, AverageScore: 1}, nil
		},
		listFn: func(_ context.Context, _ surveyport.ID, limit, offset int32) ([]surveyport.Submission, error) {
			calls++
			rows := make([]surveyport.Submission, limit)
			for index := range rows {
				id := int64(SafeAnalysisScanLimit) - int64(offset) - int64(index)
				rows[index] = safeSensitiveSubmission(id, stamp.Add(-time.Duration(offset+int32(index))*time.Second), nil)
				rows[index].QuestionnaireID = 9
			}
			return rows, nil
		},
	}
	service := NewSafeAdminService(safeAdminUOW{}, store)
	got, err := service.SafeAnalysis(context.Background(), 9, 50, 0)
	if err != nil {
		t.Fatalf("SafeAnalysis() error = %v", err)
	}
	if got.ScannedSubmissionCount != SafeAnalysisScanLimit || got.AggregationComplete || calls != int(SafeAnalysisScanLimit/int64(SafeAnalysisChunkLimit)) {
		t.Fatalf("scan cap not enforced: scanned=%d complete=%v calls=%d", got.ScannedSubmissionCount, got.AggregationComplete, calls)
	}
}

func TestSafeAnalysisFailsClosed(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		store safeAdminStoreStub
		want  error
	}{
		{
			name:  "missing questionnaire",
			store: safeAdminStoreStub{existsFn: func(context.Context, surveyport.ID) (bool, error) { return false, nil }},
			want:  ErrNotFound,
		},
		{
			name:  "backend unavailable",
			store: safeAdminStoreStub{existsFn: func(context.Context, surveyport.ID) (bool, error) { return false, errors.New("db unavailable") }},
			want:  ErrUnavailable,
		},
		{
			name: "cross questionnaire row",
			store: safeAdminStoreStub{
				existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
				resultsFn: func(context.Context, surveyport.ID) (surveyport.SubmissionResult, error) {
					return surveyport.SubmissionResult{QuestionnaireID: 7, SubmissionCount: 1, LatestSubmittedAt: stamp, AverageScore: 1}, nil
				},
				listFn: func(context.Context, surveyport.ID, int32, int32) ([]surveyport.Submission, error) {
					item := safeSensitiveSubmission(1, stamp, nil)
					item.QuestionnaireID = 8
					return []surveyport.Submission{item}, nil
				},
			},
			want: ErrUnavailable,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := NewSafeAdminService(safeAdminUOW{}, test.store)
			_, err := service.SafeAnalysis(context.Background(), 7, 50, 0)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSafeAnalysisRejectsPrematureEndOfSnapshot(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	store := safeAdminStoreStub{
		existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
		resultsFn: func(context.Context, surveyport.ID) (surveyport.SubmissionResult, error) {
			return surveyport.SubmissionResult{QuestionnaireID: 7, SubmissionCount: 2, LatestSubmittedAt: stamp, AverageScore: 1}, nil
		},
		listFn: func(context.Context, surveyport.ID, int32, int32) ([]surveyport.Submission, error) {
			return []surveyport.Submission{safeSensitiveSubmission(2, stamp, nil)}, nil
		},
	}
	service := NewSafeAdminService(safeAdminUOW{}, store)
	if _, err := service.SafeAnalysis(context.Background(), 7, 50, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SafeAnalysis() error = %v, want unavailable", err)
	}
}

func TestSafeAdminRejectsUnstableSubmissionOrdering(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	older := safeSensitiveSubmission(1, stamp.Add(-time.Minute), nil)
	newer := safeSensitiveSubmission(2, stamp, nil)
	store := safeAdminStoreStub{
		existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
		resultsFn: func(context.Context, surveyport.ID) (surveyport.SubmissionResult, error) {
			return surveyport.SubmissionResult{QuestionnaireID: 7, SubmissionCount: 2, LatestSubmittedAt: stamp, AverageScore: 1}, nil
		},
		countFn: func(context.Context, surveyport.ID) (int64, error) { return 2, nil },
		listFn: func(context.Context, surveyport.ID, int32, int32) ([]surveyport.Submission, error) {
			return []surveyport.Submission{older, newer}, nil
		},
	}
	service := NewSafeAdminService(safeAdminUOW{}, store)
	if _, err := service.SafeAnalysis(context.Background(), 7, 50, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SafeAnalysis() error = %v, want unavailable", err)
	}
	if _, err := service.SafeExportPreview(context.Background(), 7, surveyport.SafeExportPreviewRequest{Limit: 2}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SafeExportPreview() error = %v, want unavailable", err)
	}
}

func TestSafeExportPreviewWhitelistAndProjection(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	item := safeSensitiveSubmission(55, stamp, []surveyport.SubmissionAnswer{
		{QuestionID: 10, QuestionType: surveyport.SingleChoice, QuestionTitle: "secret question 10", SortOrder: 0, SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 101, OptionText: "secret option 3"}}},
		{QuestionID: 20, QuestionType: surveyport.MultiChoice, QuestionTitle: "secret question", SortOrder: 1, SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 202, OptionText: "secret option"}, {OptionID: 201, OptionText: "secret option 2"}}},
		{QuestionID: 30, QuestionType: surveyport.Textarea, QuestionTitle: "secret textarea", SortOrder: 2, TextValue: "very private answer"},
	})
	store := safeAdminStoreStub{
		existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
		countFn:  func(context.Context, surveyport.ID) (int64, error) { return 4, nil },
		listFn: func(_ context.Context, id surveyport.ID, limit, offset int32) ([]surveyport.Submission, error) {
			if id != 7 || limit != 1 || offset != 2 {
				t.Fatalf("unexpected list request id=%d limit=%d offset=%d", id, limit, offset)
			}
			return []surveyport.Submission{item}, nil
		},
	}
	service := NewSafeAdminService(safeAdminUOW{}, store)
	got, err := service.SafeExportPreview(context.Background(), 7, surveyport.SafeExportPreviewRequest{
		Fields: []surveyport.SafeExportPreviewField{surveyport.SafePreviewRowNumber, surveyport.SafePreviewChoiceOptionIDs},
		Limit:  1, Offset: 2,
	})
	if err != nil {
		t.Fatalf("SafeExportPreview() error = %v", err)
	}
	if !got.OK || got.FileCreated || !got.LocalOnly || got.RealExternalCallExecuted || got.ContainsRawIdentity || got.ContainsFreeText || len(got.Rows) != 1 || got.Rows[0].RowNumber == nil || *got.Rows[0].RowNumber != 3 {
		t.Fatalf("unexpected safe preview: %#v", got)
	}
	if got.Rows[0].SubmittedAt != nil || got.Rows[0].Score != nil || got.Rows[0].ChoiceOptionIDs == nil || len(*got.Rows[0].ChoiceOptionIDs) != 2 {
		t.Fatalf("field whitelist not applied: %#v", got.Rows[0])
	}
	choices := *got.Rows[0].ChoiceOptionIDs
	if choices[0].QuestionID != 10 || choices[1].QuestionID != 20 || choices[1].OptionIDs[0] != 201 {
		t.Fatalf("choice projection not stable: %#v", choices)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSensitiveText(t, string(encoded), "external-user-secret", "union-secret", "very private answer", "secret question", "secret option", "result-token-secret", "https://secret.example")
}

func TestSafeExportPreviewRejectsUnsafeOrUnboundedFields(t *testing.T) {
	t.Parallel()
	service := NewSafeAdminService(safeAdminUOW{}, safeAdminStoreStub{})
	tests := []surveyport.SafeExportPreviewRequest{
		{Fields: []surveyport.SafeExportPreviewField{"external_userid"}, Limit: 1},
		{Fields: []surveyport.SafeExportPreviewField{"answers"}, Limit: 1},
		{Fields: []surveyport.SafeExportPreviewField{surveyport.SafePreviewScore, surveyport.SafePreviewScore}, Limit: 1},
		{Limit: SafeExportPreviewMaximumLimit + 1},
		{Limit: 1, Offset: -1},
	}
	for _, request := range tests {
		if _, err := service.SafeExportPreview(context.Background(), 7, request); !errors.Is(err, ErrInvalidSubmissionPage) {
			t.Fatalf("request %#v error = %v, want invalid", request, err)
		}
	}
}

func TestSafeExportPreviewMapsMissingAndUnavailable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		store safeAdminStoreStub
		want  error
	}{
		{"missing", safeAdminStoreStub{existsFn: func(context.Context, surveyport.ID) (bool, error) { return false, nil }}, ErrNotFound},
		{"unavailable", safeAdminStoreStub{existsFn: func(context.Context, surveyport.ID) (bool, error) { return false, errors.New("db") }}, ErrUnavailable},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := NewSafeAdminService(safeAdminUOW{}, test.store)
			_, err := service.SafeExportPreview(context.Background(), 7, surveyport.SafeExportPreviewRequest{Limit: 1})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func safeSensitiveSubmission(id int64, submittedAt time.Time, answers []surveyport.SubmissionAnswer) surveyport.Submission {
	return surveyport.Submission{
		ID: id, QuestionnaireID: 7,
		RespondentKey: "respondent-key-secret", OpenID: "openid-secret", UnionID: "union-secret", ExternalUserID: "external-user-secret",
		CustomerName: "customer-name-secret", FollowUserUserID: "follow-user-secret", MatchedBy: "mobile", Mobile: "13900139000",
		SourceChannel: "source-secret", CampaignID: "campaign-secret", StaffID: "staff-secret", TotalScore: 8.5,
		FinalTags: []string{"tag-secret"}, ResultToken: "result-token-secret", RedirectURLSnapshot: "https://secret.example",
		SubmittedAt: submittedAt, CreatedAt: submittedAt, Answers: answers,
	}
}

func assertNoSensitiveText(t *testing.T, body string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if strings.Contains(body, value) {
			t.Fatalf("response leaked %q: %s", value, body)
		}
	}
}

func TestSafeAnalysisRejectsInconsistentLatestAndInvalidChoiceShape(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		item surveyport.Submission
	}{
		{
			name: "latest timestamp mismatch",
			item: safeSensitiveSubmission(1, stamp.Add(-time.Minute), nil),
		},
		{
			name: "single choice contains multiple options",
			item: safeSensitiveSubmission(1, stamp, []surveyport.SubmissionAnswer{{
				QuestionID: 1, QuestionType: surveyport.SingleChoice, QuestionTitle: "single", SortOrder: 0,
				SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 10, OptionText: "a"}, {OptionID: 11, OptionText: "b"}},
			}}),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := safeAdminStoreStub{
				existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
				resultsFn: func(context.Context, surveyport.ID) (surveyport.SubmissionResult, error) {
					return surveyport.SubmissionResult{QuestionnaireID: 7, SubmissionCount: 1, LatestSubmittedAt: stamp, AverageScore: 1}, nil
				},
				listFn: func(context.Context, surveyport.ID, int32, int32) ([]surveyport.Submission, error) {
					return []surveyport.Submission{test.item}, nil
				},
			}
			service := NewSafeAdminService(safeAdminUOW{}, store)
			if _, err := service.SafeAnalysis(context.Background(), 7, 50, 0); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("SafeAnalysis() error = %v, want unavailable", err)
			}
		})
	}
}

func TestSafeExportPreviewRequiresCompletePageAndAcceptsTerminalOffset(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	shortStore := safeAdminStoreStub{
		existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
		countFn:  func(context.Context, surveyport.ID) (int64, error) { return 2, nil },
		listFn: func(context.Context, surveyport.ID, int32, int32) ([]surveyport.Submission, error) {
			return []surveyport.Submission{safeSensitiveSubmission(2, stamp, nil)}, nil
		},
	}
	if _, err := NewSafeAdminService(safeAdminUOW{}, shortStore).SafeExportPreview(
		context.Background(), 7, surveyport.SafeExportPreviewRequest{Limit: 2},
	); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("short preview error = %v, want unavailable", err)
	}

	terminalStore := safeAdminStoreStub{
		existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
		countFn:  func(context.Context, surveyport.ID) (int64, error) { return 2, nil },
		listFn: func(_ context.Context, _ surveyport.ID, limit, offset int32) ([]surveyport.Submission, error) {
			if limit != 3 || offset != 2 {
				t.Fatalf("terminal list request = %d/%d", limit, offset)
			}
			return []surveyport.Submission{}, nil
		},
	}
	got, err := NewSafeAdminService(safeAdminUOW{}, terminalStore).SafeExportPreview(
		context.Background(), 7, surveyport.SafeExportPreviewRequest{Limit: 3, Offset: 2},
	)
	if err != nil {
		t.Fatalf("terminal preview error = %v", err)
	}
	if len(got.Rows) != 0 || got.Total != 2 || got.HasMore || got.Offset != 2 || got.Limit != 3 {
		t.Fatalf("terminal preview = %#v", got)
	}
}

func TestSafeExportPreviewPreservesSelectedEmptyChoiceArray(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	store := safeAdminStoreStub{
		existsFn: func(context.Context, surveyport.ID) (bool, error) { return true, nil },
		countFn:  func(context.Context, surveyport.ID) (int64, error) { return 1, nil },
		listFn: func(context.Context, surveyport.ID, int32, int32) ([]surveyport.Submission, error) {
			return []surveyport.Submission{safeSensitiveSubmission(1, stamp, []surveyport.SubmissionAnswer{{
				QuestionID: 30, QuestionType: surveyport.Textarea, QuestionTitle: "discarded", SortOrder: 0, TextValue: "private",
			}})}, nil
		},
	}
	service := NewSafeAdminService(safeAdminUOW{}, store)
	selected, err := service.SafeExportPreview(context.Background(), 7, surveyport.SafeExportPreviewRequest{
		Fields: []surveyport.SafeExportPreviewField{surveyport.SafePreviewChoiceOptionIDs}, Limit: 1,
	})
	if err != nil || selected.Rows[0].ChoiceOptionIDs == nil || len(*selected.Rows[0].ChoiceOptionIDs) != 0 {
		t.Fatalf("selected empty choice field = %#v, %v", selected, err)
	}
	body, err := json.Marshal(selected.Rows[0])
	if err != nil || !strings.Contains(string(body), `"choice_option_ids":[]`) {
		t.Fatalf("selected empty field JSON = %s, %v", body, err)
	}

	unselected, err := service.SafeExportPreview(context.Background(), 7, surveyport.SafeExportPreviewRequest{
		Fields: []surveyport.SafeExportPreviewField{surveyport.SafePreviewRowNumber}, Limit: 1,
	})
	if err != nil || unselected.Rows[0].ChoiceOptionIDs != nil {
		t.Fatalf("unselected choice field = %#v, %v", unselected, err)
	}
	body, err = json.Marshal(unselected.Rows[0])
	if err != nil || strings.Contains(string(body), "choice_option_ids") {
		t.Fatalf("unselected field JSON = %s, %v", body, err)
	}
}

func TestSafeAdminRejectsInvalidArgumentsAndUnavailableDependencies(t *testing.T) {
	t.Parallel()
	service := NewSafeAdminService(safeAdminUOW{}, safeAdminStoreStub{})
	for _, request := range []struct {
		id            surveyport.ID
		limit, offset int32
	}{
		{0, 50, 0}, {7, 0, 0}, {7, SafeAnalysisMaximumQuestionLimit + 1, 0}, {7, 1, -1}, {7, 1, SafeAnalysisMaximumQuestionOffset + 1},
	} {
		if _, err := service.SafeAnalysis(context.Background(), request.id, request.limit, request.offset); !errors.Is(err, ErrInvalidSubmissionPage) {
			t.Fatalf("SafeAnalysis(%#v) error = %v, want invalid", request, err)
		}
	}
	if _, err := (*SafeAdminService)(nil).SafeAnalysis(context.Background(), 7, 1, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil service error = %v, want unavailable", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.SafeExportPreview(ctx, 7, surveyport.SafeExportPreviewRequest{Limit: 1}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancelled preview error = %v, want unavailable", err)
	}
}
