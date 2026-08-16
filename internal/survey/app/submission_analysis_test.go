package app

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestSubmissionAnalysisReadsSummaryAndOrderedPage(t *testing.T) {
	store := &submissionAnalysisStoreStub{
		results: SubmissionResults{QuestionnaireID: 41, SubmissionCount: 2, LatestSubmittedAt: testSubmissionTime(11, 0), AverageScore: 7.5, Rules: []SubmissionScoreRule{{ID: 9, QuestionnaireID: 41, SortOrder: 0, TagCodes: []string{"qualified"}}}},
		page: SubmissionPage{Total: 2, Limit: 20, Offset: 0, Items: []Submission{
			testSubmission(42, testSubmissionTime(11, 0)),
			testSubmission(41, testSubmissionTime(11, 0)),
		}},
	}
	service := NewSubmissionAnalysisService(testUOW{}, store, &submissionAnalysisAccessStub{})

	results, err := service.Results(context.Background(), 41)
	if err != nil || results.SubmissionCount != 2 || results.LatestSubmittedAt != testSubmissionTime(11, 0) || results.AverageScore != 7.5 || len(results.Rules) != 1 {
		t.Fatalf("Results() = %#v, %v", results, err)
	}
	page, err := service.List(context.Background(), 41, 20, 0)
	if err != nil || page.Total != 2 || len(page.Items) != 2 || page.Items[0].ID != 42 || page.Items[1].ID != 41 || store.listLimit != 20 || store.listOffset != 0 {
		t.Fatalf("List() = %#v, calls=%d/%d, %v", page, store.listLimit, store.listOffset, err)
	}
	page.Items[0].FinalTags[0] = "mutated"
	if store.page.Items[0].FinalTags[0] != "qualified" {
		t.Fatal("List() exposed the store's PII-bearing snapshot to mutation")
	}
}

func TestSubmissionAnalysisAllowsEmptySummary(t *testing.T) {
	service := NewSubmissionAnalysisService(testUOW{}, &submissionAnalysisStoreStub{
		results: SubmissionResults{QuestionnaireID: 41, SubmissionCount: 0, AverageScore: 0, Rules: []SubmissionScoreRule{}},
	}, &submissionAnalysisAccessStub{})
	results, err := service.Results(context.Background(), 41)
	if err != nil || results.SubmissionCount != 0 || !results.LatestSubmittedAt.IsZero() || results.AverageScore != 0 {
		t.Fatalf("empty Results() = %#v, %v", results, err)
	}
}

func TestSubmissionAnalysisRejectsInvalidPagingAndUnorderedRows(t *testing.T) {
	store := &submissionAnalysisStoreStub{page: SubmissionPage{Total: 2, Limit: 20, Offset: 0, Items: []Submission{
		testSubmission(41, testSubmissionTime(10, 0)), testSubmission(42, testSubmissionTime(11, 0)),
	}}}
	service := NewSubmissionAnalysisService(testUOW{}, store, &submissionAnalysisAccessStub{})
	for _, request := range []struct{ limit, offset int32 }{{0, 0}, {-1, 0}, {20, -1}, {SubmissionAnalysisMaximumLimit + 1, 0}, {20, SubmissionAnalysisMaximumOffset + 1}} {
		if _, err := service.List(context.Background(), 41, request.limit, request.offset); !errors.Is(err, ErrInvalidSubmissionPage) {
			t.Fatalf("List(%d, %d) error = %v", request.limit, request.offset, err)
		}
	}
	if _, err := service.List(context.Background(), 41, 20, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unordered List() error = %v", err)
	}
}

func TestSubmissionAnalysisFailsClosedForPIIAuthorization(t *testing.T) {
	store := &submissionAnalysisStoreStub{export: QuestionnaireExportSnapshot{
		QuestionnaireID: 41, Total: 1, Questions: []SubmissionQuestion{{ID: "q1", Title: "问题", SortOrder: 0}},
		Submissions: []Submission{testSubmission(41, testSubmissionTime(11, 0))},
	}}
	access := &submissionAnalysisAccessStub{denied: SubmissionAnalysisCustomerRead}
	service := NewSubmissionAnalysisService(testUOW{}, store, access)
	if _, err := service.Export(context.Background(), 41); !errors.Is(err, ErrSubmissionForbidden) || store.exportLimit != 0 {
		t.Fatalf("PII Export() error=%v exportLimit=%d", err, store.exportLimit)
	}
	if len(access.seen) != 1 || access.seen[0] != SubmissionAnalysisCustomerRead {
		t.Fatalf("permissions=%v", access.seen)
	}
}

func TestSubmissionAnalysisFailsClosedWhenAuthorizationIsUnavailable(t *testing.T) {
	service := NewSubmissionAnalysisService(testUOW{}, &submissionAnalysisStoreStub{}, &submissionAnalysisAccessStub{unavailable: true})
	if _, err := service.Results(context.Background(), 41); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Results() authorization error = %v", err)
	}
}

func TestSubmissionAnalysisFailsClosedForIdentityConflictMissingOwnerAndUnavailableStore(t *testing.T) {
	for name, storeErr := range map[string]error{
		"OneID conflict": ErrIdentityConflict,
		"missing owner":  ErrNotFound,
		"DB unavailable": errors.New("connection refused"),
	} {
		t.Run(name, func(t *testing.T) {
			service := NewSubmissionAnalysisService(testUOW{}, &submissionAnalysisStoreStub{resultErr: storeErr, pageErr: storeErr, exportErr: storeErr}, &submissionAnalysisAccessStub{})
			want := storeErr
			if name == "DB unavailable" {
				want = ErrUnavailable
			}
			if _, err := service.Results(context.Background(), 41); !errors.Is(err, want) {
				t.Fatalf("Results() error = %v, want %v", err, want)
			}
			if _, err := service.List(context.Background(), 41, 20, 0); !errors.Is(err, want) {
				t.Fatalf("List() error = %v, want %v", err, want)
			}
			if _, err := service.Export(context.Background(), 41); !errors.Is(err, want) {
				t.Fatalf("Export() error = %v, want %v", err, want)
			}
		})
	}
}

func TestEncodeQuestionnaireSubmissionCSVPiiOrderTimezoneNewlinesAndFormulaSafety(t *testing.T) {
	snapshot := QuestionnaireExportSnapshot{
		QuestionnaireID: 41,
		Slug:            "activation",
		SourceStatus:    "next_command",
		Total:           2,
		Questions: []SubmissionQuestion{
			{ID: "q1", Title: "目标", SortOrder: 0},
			{ID: "q2", Title: "目标", SortOrder: 1},
		},
		Submissions: []Submission{
			{
				ID: 42, QuestionnaireID: 41, SubmittedAt: time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC), ExternalUserID: "=danger", CustomerName: "小璨\n同学", UnionID: "union-42", Mobile: "+8613800000000", TotalScore: 7.5, FinalTags: []string{"A", "B"},
				AnswerSnapshots: []SubmissionAnswerSnapshot{
					{QuestionID: "q1", QuestionType: "textarea", QuestionTitleSnapshot: "目标", TextValue: "=SUM(1,1)"},
					{QuestionID: "q2", QuestionType: "single_choice", QuestionTitleSnapshot: "目标", SelectedOptionTextsSnapshot: []string{"增长"}, TextValue: "备注"},
					{QuestionID: "q3", QuestionType: "mobile", QuestionTitleSnapshot: "联系方式", TextValue: "+1-555"},
				},
			},
			{
				ID: 41, QuestionnaireID: 41, SubmittedAt: time.Date(2026, 8, 16, 1, 2, 2, 0, time.UTC), ExternalUserID: "ext-41", CustomerName: "", UnionID: "", Mobile: "", TotalScore: 0, FinalTags: []string{},
				AnswerSnapshots: []SubmissionAnswerSnapshot{{QuestionID: "q1", QuestionType: "textarea", QuestionTitleSnapshot: "目标", TextValue: "line one\nline two"}},
			},
		},
	}
	download, err := EncodeQuestionnaireSubmissionCSV(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if download.ContentType != "text/csv; charset=utf-8" || download.Filename != "questionnaire-activation-submissions.csv" || download.Headers["Content-Disposition"] != `attachment; filename="questionnaire-activation-submissions.csv"` || download.Headers["X-AICRM-Fallback-Used"] != "false" || !strings.HasPrefix(string(download.Body), "\ufeff") || !strings.Contains(string(download.Body), "\r\n") {
		t.Fatalf("download headers/body = %#v / %q", download, download.Body)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(download.Body), "\ufeff")))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantHeaders := []string{"submission_id", "submitted_at", "external_userid", "用户昵称", "unionid", "mobile", "score", "final_tags", "目标", "目标 (2)", "联系方式"}
	if len(rows) != 3 || strings.Join(rows[0], "|") != strings.Join(wantHeaders, "|") {
		t.Fatalf("headers/rows = %#v", rows)
	}
	if rows[1][1] != "2026-08-16 09:02:03" || rows[1][2] != "'=danger" || rows[1][3] != "小璨\n同学" || rows[1][7] != "A、B" || rows[1][8] != "'=SUM(1,1)" || rows[1][9] != "增长：备注" || rows[1][10] != "'+1-555" {
		t.Fatalf("first CSV row = %#v", rows[1])
	}
	if rows[2][8] != "line one\nline two" {
		t.Fatalf("multiline answer = %q", rows[2][8])
	}
}

func TestSubmissionAnalysisExportLocksLimitAndDoesNotLeakSourceSnapshot(t *testing.T) {
	store := &submissionAnalysisStoreStub{export: QuestionnaireExportSnapshot{
		QuestionnaireID: 41, Slug: "", Total: 1, Questions: []SubmissionQuestion{{ID: "q1", Title: "问题", SortOrder: 0}},
		Submissions: []Submission{testSubmission(41, testSubmissionTime(11, 0))},
	}}
	service := NewSubmissionAnalysisService(testUOW{}, store, &submissionAnalysisAccessStub{})
	download, err := service.Export(context.Background(), 41)
	if err != nil || store.exportLimit != SubmissionExportLimit || download.Filename != "questionnaire-questionnaire-41-submissions.csv" {
		t.Fatalf("Export() = %#v, limit=%d, err=%v", download, store.exportLimit, err)
	}
}

type submissionAnalysisStoreStub struct {
	results     SubmissionResults
	page        SubmissionPage
	export      QuestionnaireExportSnapshot
	resultErr   error
	pageErr     error
	exportErr   error
	listLimit   int32
	listOffset  int32
	exportLimit int32
}

func (store *submissionAnalysisStoreStub) Results(context.Context, surveyport.ID) (SubmissionResults, error) {
	return store.results, store.resultErr
}

func (store *submissionAnalysisStoreStub) ListSubmissions(_ context.Context, _ surveyport.ID, limit, offset int32) (SubmissionPage, error) {
	store.listLimit, store.listOffset = limit, offset
	return store.page, store.pageErr
}

func (store *submissionAnalysisStoreStub) ExportSnapshot(_ context.Context, _ surveyport.ID, limit int32) (QuestionnaireExportSnapshot, error) {
	store.exportLimit = limit
	return store.export, store.exportErr
}

func testSubmission(id int64, submittedAt time.Time) Submission {
	return Submission{
		ID: id, QuestionnaireID: 41, SubmittedAt: submittedAt, ExternalUserID: "ext", CustomerName: "name", UnionID: "union", Mobile: "mobile", TotalScore: 7.5, FinalTags: []string{"qualified"},
		AnswerSnapshots: []SubmissionAnswerSnapshot{{QuestionID: "q1", QuestionType: "single_choice", QuestionTitleSnapshot: "问题", SelectedOptionTextsSnapshot: []string{"选项"}}},
	}
}

func testSubmissionTime(hour, minute int) time.Time {
	return time.Date(2026, 8, 16, hour, minute, 0, 0, time.UTC)
}

type submissionAnalysisAccessStub struct {
	denied      SubmissionAnalysisPermission
	unavailable bool
	seen        []SubmissionAnalysisPermission
}

func (stub *submissionAnalysisAccessStub) AuthorizeSubmissionAnalysis(_ context.Context, permission SubmissionAnalysisPermission) error {
	stub.seen = append(stub.seen, permission)
	if permission == stub.denied {
		return ErrSubmissionForbidden
	}
	if stub.unavailable {
		return errors.New("session backend unavailable")
	}
	return nil
}
