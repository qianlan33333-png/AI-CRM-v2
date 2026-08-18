package app

import (
	"context"
	"encoding/csv"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestSubmissionResultsNormalAndEmptyStayStable(t *testing.T) {
	latest := testSubmissionTime(11, 0)
	store := &submissionStoreStub{result: surveyport.SubmissionResult{
		QuestionnaireID: 41, SubmissionCount: 2, LatestSubmittedAt: latest, AverageScore: 7.5,
		Rules: []surveyport.ScoreRule{},
	}}
	service := NewSubmissionService(testUOW{}, store)
	result, err := service.Results(context.Background(), 41)
	if err != nil || result.SubmissionCount != 2 || result.LatestSubmittedAt != latest || result.AverageScore != 7.5 || result.Rules == nil {
		t.Fatalf("Results() = %#v, %v", result, err)
	}

	store.result = surveyport.SubmissionResult{QuestionnaireID: 41, Rules: []surveyport.ScoreRule{}}
	empty, err := service.Results(context.Background(), 41)
	if err != nil || empty.SubmissionCount != 0 || !empty.LatestSubmittedAt.IsZero() || empty.AverageScore != 0 || len(empty.Rules) != 0 {
		t.Fatalf("empty Results() = %#v, %v", empty, err)
	}
}

func TestSubmissionResultsRejectsInvalidIDAndPropagatesNotFoundAndUnavailable(t *testing.T) {
	service := NewSubmissionService(testUOW{}, &submissionStoreStub{})
	if _, err := service.Results(context.Background(), 0); !errors.Is(err, ErrInvalidSubmissionPage) {
		t.Fatalf("Results(0) error = %v", err)
	}
	for name, storeErr := range map[string]error{"missing": ErrNotFound, "db": errors.New("connection refused")} {
		want := storeErr
		if name == "db" {
			want = ErrUnavailable
		}
		service := NewSubmissionService(testUOW{}, &submissionStoreStub{owner: true, resultErr: storeErr, listErr: storeErr, countErr: storeErr, definitionErr: storeErr, exportErr: storeErr})
		if _, err := service.Results(context.Background(), 41); !errors.Is(err, want) {
			t.Fatalf("%s Results() error = %v, want %v", name, err, want)
		}
		if _, err := service.List(context.Background(), 41, 20, 0); !errors.Is(err, want) {
			t.Fatalf("%s List() error = %v, want %v", name, err, want)
		}
		if _, err := service.Export(context.Background(), 41); !errors.Is(err, want) {
			t.Fatalf("%s Export() error = %v, want %v", name, err, want)
		}
	}
}

func TestSubmissionResultsFailsClosedOnCorruptAggregate(t *testing.T) {
	for name, result := range map[string]surveyport.SubmissionResult{
		"wrong owner":    {QuestionnaireID: 42, Rules: []surveyport.ScoreRule{}},
		"negative count": {QuestionnaireID: 41, SubmissionCount: -1, Rules: []surveyport.ScoreRule{}},
		"NaN average":    {QuestionnaireID: 41, SubmissionCount: 1, AverageScore: math.NaN(), Rules: []surveyport.ScoreRule{}},
		"ghost latest":   {QuestionnaireID: 41, LatestSubmittedAt: testSubmissionTime(9, 0), Rules: []surveyport.ScoreRule{}},
	} {
		service := NewSubmissionService(testUOW{}, &submissionStoreStub{result: result})
		if _, err := service.Results(context.Background(), 41); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s Results() error = %v", name, err)
		}
	}
}

func TestSubmissionListPagingOrderAndCloneIsolation(t *testing.T) {
	store := &submissionStoreStub{
		owner: true, total: 3,
		items: []surveyport.Submission{
			testSubmission(43, testSubmissionTime(11, 0)),
			testSubmission(42, testSubmissionTime(11, 0)),
		},
	}
	service := NewSubmissionService(testUOW{}, store)
	page, err := service.List(context.Background(), 41, 20, 0)
	if err != nil || page.Total != 3 || page.Limit != 20 || page.Offset != 0 || len(page.Items) != 2 || page.Items[0].ID != 43 || page.Items[1].ID != 42 {
		t.Fatalf("List() = %#v, %v", page, err)
	}
	if store.listLimit != 20 || store.listOffset != 0 {
		t.Fatalf("store paging = %d/%d", store.listLimit, store.listOffset)
	}
	page.Items[0].FinalTags[0] = "mutated"
	page.Items[0].Answers[0].SelectedOptions[0].OptionText = "mutated"
	if store.items[0].FinalTags[0] != "qualified" || store.items[0].Answers[0].SelectedOptions[0].OptionText != "选项" {
		t.Fatal("List() exposed the store snapshot to mutation")
	}
}

func TestSubmissionListRejectsInvalidPagingMissingOwnerAndUnorderedRows(t *testing.T) {
	service := NewSubmissionService(testUOW{}, &submissionStoreStub{owner: true})
	for _, request := range []struct{ limit, offset int32 }{{0, 0}, {-1, 0}, {101, 0}, {20, -1}, {20, SubmissionMaximumOffset + 1}} {
		if _, err := service.List(context.Background(), 41, request.limit, request.offset); !errors.Is(err, ErrInvalidSubmissionPage) {
			t.Fatalf("List(%d, %d) error = %v", request.limit, request.offset, err)
		}
	}
	missing := NewSubmissionService(testUOW{}, &submissionStoreStub{owner: false})
	if _, err := missing.List(context.Background(), 41, 20, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing owner List() error = %v", err)
	}
	unordered := NewSubmissionService(testUOW{}, &submissionStoreStub{
		owner: true, total: 2,
		items: []surveyport.Submission{testSubmission(41, testSubmissionTime(10, 0)), testSubmission(42, testSubmissionTime(11, 0))},
	})
	if _, err := unordered.List(context.Background(), 41, 20, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unordered List() error = %v", err)
	}
}

func TestSubmissionExportLocksLimitAndEncodesSafeCSV(t *testing.T) {
	store := &submissionStoreStub{
		definitionSlug: "activation",
		definition: []surveyport.SubmissionExportQuestion{
			{ID: 51, Title: "目标", SortOrder: 0},
			{ID: 52, Title: "目标", SortOrder: 1},
		},
		export: []surveyport.Submission{
			{
				ID: 42, QuestionnaireID: 41, SubmittedAt: time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC),
				CreatedAt:      time.Date(2026, 8, 16, 1, 2, 4, 0, time.UTC),
				ExternalUserID: "=danger", CustomerName: "小璨\n同学", UnionID: "union-42", Mobile: "+8613800000000",
				TotalScore: 7.5, FinalTags: []string{"A", "B"},
				Answers: []surveyport.SubmissionAnswer{
					{QuestionID: 51, QuestionType: surveyport.Textarea, QuestionTitle: "目标", SortOrder: 0, SelectedOptions: []surveyport.SubmissionAnswerOption{}, TextValue: "=SUM(1,1)"},
					{QuestionID: 52, QuestionType: surveyport.SingleChoice, QuestionTitle: "目标", SortOrder: 1, SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 61, OptionText: "增长"}}, TextValue: "备注"},
					{QuestionID: 53, QuestionType: surveyport.Mobile, QuestionTitle: "联系方式", SortOrder: 2, SelectedOptions: []surveyport.SubmissionAnswerOption{}, TextValue: "+1-555"},
				},
			},
			{
				ID: 41, QuestionnaireID: 41, SubmittedAt: time.Date(2026, 8, 16, 1, 2, 2, 0, time.UTC),
				CreatedAt: time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC),
				FinalTags: []string{},
				Answers: []surveyport.SubmissionAnswer{
					{QuestionID: 51, QuestionType: surveyport.Textarea, QuestionTitle: "目标", SortOrder: 0, SelectedOptions: []surveyport.SubmissionAnswerOption{}, TextValue: "line one\nline two"},
				},
			},
		},
		total: 2,
	}
	service := NewSubmissionService(testUOW{}, store)
	download, err := service.Export(context.Background(), 41)
	if err != nil {
		t.Fatal(err)
	}
	if store.exportLimit != SubmissionExportLimit {
		t.Fatalf("export limit = %d", store.exportLimit)
	}
	if download.ContentType != "text/csv; charset=utf-8" || download.Filename != "questionnaire-activation-submissions.csv" || download.Total != 2 {
		t.Fatalf("download = %#v", download)
	}
	body := string(download.Body)
	if !strings.HasPrefix(body, "\ufeff") || !strings.Contains(body, "\r\n") {
		t.Fatalf("BOM/CRLF missing: %q", body[:40])
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantHeaders := "submission_id|submitted_at|external_userid|用户昵称|unionid|mobile|score|final_tags|目标|目标 (2)|联系方式"
	if len(rows) != 3 || strings.Join(rows[0], "|") != wantHeaders {
		t.Fatalf("headers/rows = %#v", rows)
	}
	if rows[1][1] != "2026-08-16 09:02:03" || rows[1][2] != "'=danger" || rows[1][3] != "小璨\n同学" || rows[1][7] != "A、B" ||
		rows[1][8] != "'=SUM(1,1)" || rows[1][9] != "增长：备注" || rows[1][10] != "'+1-555" {
		t.Fatalf("first CSV row = %#v", rows[1])
	}
	if rows[2][2] != "" || rows[2][8] != "line one\nline two" || rows[2][9] != "" || rows[2][10] != "" {
		t.Fatalf("second CSV row = %#v", rows[2])
	}
}

func TestSubmissionExportRejectsCorruptSnapshotsAndSanitizesSlug(t *testing.T) {
	for name, mutate := range map[string]func(*submissionStoreStub){
		"extra rows": func(s *submissionStoreStub) { s.total = 0 },
		"bad type": func(s *submissionStoreStub) {
			s.export[0].Answers[0].QuestionType = "computed"
		},
		"duplicate question": func(s *submissionStoreStub) {
			s.export[0].Answers = append(s.export[0].Answers, s.export[0].Answers[0])
		},
	} {
		store := &submissionStoreStub{
			definitionSlug: "q", total: 1,
			definition: []surveyport.SubmissionExportQuestion{{ID: 51, Title: "目标", SortOrder: 0}},
			export:     []surveyport.Submission{testSubmission(41, testSubmissionTime(11, 0))},
		}
		mutate(store)
		service := NewSubmissionService(testUOW{}, store)
		if _, err := service.Export(context.Background(), 41); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s Export() error = %v", name, err)
		}
	}

	store := &submissionStoreStub{
		definitionSlug: ` 小心"注入"  `,
		definition:     []surveyport.SubmissionExportQuestion{{ID: 51, Title: "目标", SortOrder: 0}},
		export:         []surveyport.Submission{testSubmission(41, testSubmissionTime(11, 0))},
		total:          1,
	}
	download, err := NewSubmissionService(testUOW{}, store).Export(context.Background(), 41)
	if err != nil || strings.ContainsAny(download.Filename, `" `) || !strings.HasPrefix(download.Filename, "questionnaire-") || !strings.HasSuffix(download.Filename, "-submissions.csv") {
		t.Fatalf("slug filename = %q, %v", download.Filename, err)
	}
}

func TestSubmissionServiceFailsClosedWhenNotReady(t *testing.T) {
	var service *SubmissionService
	if _, err := service.Results(context.Background(), 41); !errors.Is(err, ErrInvalidSubmissionPage) {
		t.Fatalf("nil Results() error = %v", err)
	}
	if _, err := NewSubmissionService(testUOW{}, nil).List(context.Background(), 41, 20, 0); !errors.Is(err, ErrInvalidSubmissionPage) {
		t.Fatalf("nil store List() error = %v", err)
	}
}

func testSubmission(id int64, submittedAt time.Time) surveyport.Submission {
	return surveyport.Submission{
		ID: id, QuestionnaireID: 41, SubmittedAt: submittedAt, CreatedAt: submittedAt,
		ExternalUserID: "ext", CustomerName: "name", UnionID: "union", Mobile: "mobile",
		TotalScore: 7.5, FinalTags: []string{"qualified"},
		Answers: []surveyport.SubmissionAnswer{{
			QuestionID: 51, QuestionType: surveyport.SingleChoice, QuestionTitle: "目标", SortOrder: 0,
			SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 61, OptionText: "选项"}},
		}},
	}
}

func testSubmissionTime(hour, minute int) time.Time {
	return time.Date(2026, 8, 16, hour, minute, 0, 0, time.UTC)
}

type submissionStoreStub struct {
	result         surveyport.SubmissionResult
	resultErr      error
	owner          bool
	ownerErr       error
	total          int64
	countErr       error
	items          []surveyport.Submission
	listErr        error
	definitionSlug string
	definition     []surveyport.SubmissionExportQuestion
	definitionErr  error
	export         []surveyport.Submission
	exportErr      error
	listLimit      int32
	listOffset     int32
	exportLimit    int32
}

func (store *submissionStoreStub) Results(context.Context, surveyport.ID) (surveyport.SubmissionResult, error) {
	return store.result, store.resultErr
}

func (store *submissionStoreStub) SubmissionOwnerExists(context.Context, surveyport.ID) (bool, error) {
	return store.owner, store.ownerErr
}

func (store *submissionStoreStub) CountSubmissions(context.Context, surveyport.ID) (int64, error) {
	return store.total, store.countErr
}

func (store *submissionStoreStub) ListSubmissions(_ context.Context, _ surveyport.ID, limit, offset int32) ([]surveyport.Submission, error) {
	store.listLimit, store.listOffset = limit, offset
	return store.items, store.listErr
}

func (store *submissionStoreStub) ExportDefinition(context.Context, surveyport.ID) (string, []surveyport.SubmissionExportQuestion, error) {
	return store.definitionSlug, store.definition, store.definitionErr
}

func (store *submissionStoreStub) ExportSubmissions(_ context.Context, _ surveyport.ID, limit int32) ([]surveyport.Submission, error) {
	store.exportLimit = limit
	return store.export, store.exportErr
}

func TestSubmissionResultsRequiresLatestWhenCountPositive(t *testing.T) {
	for name, result := range map[string]surveyport.SubmissionResult{
		"positive without latest": {QuestionnaireID: 41, SubmissionCount: 1, AverageScore: 7.5, Rules: []surveyport.ScoreRule{}},
		"empty with average":      {QuestionnaireID: 41, AverageScore: 0.5, Rules: []surveyport.ScoreRule{}},
	} {
		service := NewSubmissionService(testUOW{}, &submissionStoreStub{result: result})
		if _, err := service.Results(context.Background(), 41); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s Results() error = %v", name, err)
		}
	}
	service := NewSubmissionService(testUOW{}, &submissionStoreStub{result: surveyport.SubmissionResult{
		QuestionnaireID: 41, SubmissionCount: 1, LatestSubmittedAt: testSubmissionTime(11, 0), AverageScore: 7.5, Rules: []surveyport.ScoreRule{},
	}})
	if _, err := service.Results(context.Background(), 41); err != nil {
		t.Fatalf("positive with latest Results() error = %v", err)
	}
}

func TestSubmissionListRejectsImpossiblePages(t *testing.T) {
	item := testSubmission(41, testSubmissionTime(11, 0))
	for name, page := range map[string]struct {
		total         int64
		limit, offset int32
		items         []surveyport.Submission
	}{
		"offset equals total with rows": {total: 1, limit: 20, offset: 1, items: []surveyport.Submission{item}},
		"offset beyond total with rows": {total: 1, limit: 20, offset: 5, items: []surveyport.Submission{item}},
		"page spans past total":         {total: 3, limit: 20, offset: 2, items: []surveyport.Submission{item, item}},
		"more rows than total":          {total: 1, limit: 20, offset: 0, items: []surveyport.Submission{item, item}},
	} {
		store := &submissionStoreStub{owner: true, total: page.total, items: page.items}
		service := NewSubmissionService(testUOW{}, store)
		if _, err := service.List(context.Background(), 41, page.limit, page.offset); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s List() error = %v", name, err)
		}
	}
	store := &submissionStoreStub{owner: true, total: 1, items: []surveyport.Submission{}}
	service := NewSubmissionService(testUOW{}, store)
	page, err := service.List(context.Background(), 41, 20, 1)
	if err != nil || page.Total != 1 || len(page.Items) != 0 {
		t.Fatalf("offset==total empty page = %#v, %v", page, err)
	}
}

func TestValidSubmissionFieldCapsAndRequiredFields(t *testing.T) {
	long := func(runes int) string { return strings.Repeat("问", runes) }
	cases := map[string]func(*surveyport.Submission){
		"respondent_key 201":   func(s *surveyport.Submission) { s.RespondentKey = long(201) },
		"openid 201":           func(s *surveyport.Submission) { s.OpenID = long(201) },
		"unionid 201":          func(s *surveyport.Submission) { s.UnionID = long(201) },
		"external_userid 201":  func(s *surveyport.Submission) { s.ExternalUserID = long(201) },
		"customer_name 301":    func(s *surveyport.Submission) { s.CustomerName = long(301) },
		"follow_user 201":      func(s *surveyport.Submission) { s.FollowUserUserID = long(201) },
		"matched_by 51":        func(s *surveyport.Submission) { s.MatchedBy = long(51) },
		"mobile 33":            func(s *surveyport.Submission) { s.Mobile = long(33) },
		"source_channel 101":   func(s *surveyport.Submission) { s.SourceChannel = long(101) },
		"campaign_id 201":      func(s *surveyport.Submission) { s.CampaignID = long(201) },
		"staff_id 201":         func(s *surveyport.Submission) { s.StaffID = long(201) },
		"result_token 201":     func(s *surveyport.Submission) { s.ResultToken = long(201) },
		"redirect_url 2001":    func(s *surveyport.Submission) { s.RedirectURLSnapshot = long(2001) },
		"final_tag 201":        func(s *surveyport.Submission) { s.FinalTags = []string{long(201)} },
		"empty final_tag":      func(s *surveyport.Submission) { s.FinalTags = []string{""} },
		"whitespace final_tag": func(s *surveyport.Submission) { s.FinalTags = []string{"   "} },
		"invalid utf8 tag":     func(s *surveyport.Submission) { s.FinalTags = []string{string([]byte{0xff})} },
		"zero created_at":      func(s *surveyport.Submission) { s.CreatedAt = time.Time{} },
		"question_title 501":   func(s *surveyport.Submission) { s.Answers[0].QuestionTitle = long(501) },
		"empty question title": func(s *surveyport.Submission) { s.Answers[0].QuestionTitle = "" },
		"blank question title": func(s *surveyport.Submission) { s.Answers[0].QuestionTitle = "   " },
		"text_value 10001":     func(s *surveyport.Submission) { s.Answers[0].TextValue = long(10_001) },
		"option_id zero":       func(s *surveyport.Submission) { s.Answers[0].SelectedOptions[0].OptionID = 0 },
		"option_text 501":      func(s *surveyport.Submission) { s.Answers[0].SelectedOptions[0].OptionText = long(501) },
		"empty option_text":    func(s *surveyport.Submission) { s.Answers[0].SelectedOptions[0].OptionText = "" },
		"blank option_text":    func(s *surveyport.Submission) { s.Answers[0].SelectedOptions[0].OptionText = "   " },
	}
	for name, mutate := range cases {
		item := testSubmission(41, testSubmissionTime(11, 0))
		mutate(&item)
		if validSubmission(item, 41) {
			t.Fatalf("%s accepted", name)
		}
		store := &submissionStoreStub{owner: true, total: 1, items: []surveyport.Submission{item}}
		service := NewSubmissionService(testUOW{}, store)
		if _, err := service.List(context.Background(), 41, 20, 0); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s List() error = %v", name, err)
		}
	}
	item := testSubmission(41, testSubmissionTime(11, 0))
	item.CustomerName = long(300)
	item.FinalTags = []string{" " + long(198) + " "}
	item.RedirectURLSnapshot = long(2000)
	item.Answers[0].QuestionTitle = " " + long(498) + " "
	item.Answers[0].TextValue = long(10_000)
	item.Answers[0].SelectedOptions[0].OptionText = " " + long(498) + " "
	if !validSubmission(item, 41) {
		t.Fatal("boundary rune counts must pass: caps count Unicode characters, not bytes")
	}
}
