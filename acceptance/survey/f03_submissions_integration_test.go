package survey_acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
)

func TestF03SubmissionReadsResultsPagingExportAndGuards(t *testing.T) {
	pool, ctx := openPool(t)
	service := f03Service(pool)
	actor := int64(5601)
	created, err := realService(pool).Create(ctx, questionnaireCommand(actor, unique("f03-create"), unique("f03-name")))
	if err != nil {
		t.Fatal(err)
	}
	firstQuestionID := int64(created.Questions[0].ID)
	secondQuestionID := int64(created.Questions[1].ID)

	empty, err := service.Results(ctx, created.ID)
	if err != nil || empty.SubmissionCount != 0 || !empty.LatestSubmittedAt.IsZero() || empty.AverageScore != 0 || len(empty.Rules) != 0 {
		t.Fatalf("empty results=%#v err=%v", empty, err)
	}
	if _, err = service.List(ctx, created.ID, 20, 0); err != nil {
		t.Fatalf("empty list err=%v", err)
	}

	base := time.Date(2026, 8, 18, 1, 0, 0, 0, time.UTC)
	first := f03InsertSubmission(t, ctx, pool, created.ID, "ext-f03-a", 8, base.Add(-2*time.Hour))
	second := f03InsertSubmission(t, ctx, pool, created.ID, "ext-f03-b", 7, base.Add(-time.Hour))
	third := f03InsertSubmission(t, ctx, pool, created.ID, "=ext-f03-c", 9, base.Add(-time.Hour))
	f03InsertAnswer(t, ctx, pool, first, firstQuestionID, "single_choice", "目标", 0, `[{"option_id": 61, "option_text": "增长"}]`, "")
	f03InsertAnswer(t, ctx, pool, first, secondQuestionID, "mobile", "手机号", 1, `[]`, "13800000000")
	f03InsertAnswer(t, ctx, pool, second, firstQuestionID, "single_choice", "目标", 0, `[{"option_id": 62, "option_text": "交付"}]`, "补充")
	f03InsertAnswer(t, ctx, pool, third, firstQuestionID, "single_choice", "目标", 0, `[]`, "=HYPERLINK(1)")

	var storedCount int64
	if err = pool.QueryRow(ctx, `SELECT submission_count FROM questionnaires WHERE id=$1`, int64(created.ID)).Scan(&storedCount); err != nil || storedCount != 3 {
		t.Fatalf("trigger count=%d err=%v", storedCount, err)
	}

	results, err := service.Results(ctx, created.ID)
	if err != nil || results.SubmissionCount != 3 || !results.LatestSubmittedAt.Equal(base.Add(-time.Hour)) || results.AverageScore != 8 {
		t.Fatalf("results=%#v err=%v", results, err)
	}

	page, err := service.List(ctx, created.ID, 2, 0)
	if err != nil || page.Total != 3 || len(page.Items) != 2 || page.Items[0].ID != third || page.Items[1].ID != second {
		t.Fatalf("page1=%#v err=%v", page, err)
	}
	if len(page.Items[0].Answers) != 1 || page.Items[0].Answers[0].TextValue != "=HYPERLINK(1)" {
		t.Fatalf("answers=%#v", page.Items[0].Answers)
	}
	rest, err := service.List(ctx, created.ID, 2, 2)
	if err != nil || rest.Total != 3 || len(rest.Items) != 1 || rest.Items[0].ID != first {
		t.Fatalf("page2=%#v err=%v", rest, err)
	}
	if _, err = service.List(ctx, created.ID, 0, 0); !errors.Is(err, surveyapp.ErrInvalidSubmissionPage) {
		t.Fatalf("invalid page err=%v", err)
	}
	if _, err = service.List(ctx, 999999999, 20, 0); !errors.Is(err, surveyapp.ErrNotFound) {
		t.Fatalf("missing owner list err=%v", err)
	}
	if _, err = service.Results(ctx, 999999999); !errors.Is(err, surveyapp.ErrNotFound) {
		t.Fatalf("missing owner results err=%v", err)
	}
	if _, err = service.Export(ctx, 999999999); !errors.Is(err, surveyapp.ErrNotFound) {
		t.Fatalf("missing owner export err=%v", err)
	}

	download, err := service.Export(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	body := string(download.Body)
	if download.Filename != "questionnaire-"+created.Slug+"-submissions.csv" || download.Total != 3 || !strings.HasPrefix(body, "\ufeff") {
		t.Fatalf("download=%#v body=%q", download, body[:32])
	}
	if !strings.Contains(body, "目标") || !strings.Contains(body, "手机号") || !strings.Contains(body, "'=HYPERLINK(1)") || !strings.Contains(body, "交付：补充") {
		t.Fatalf("csv body=%q", body)
	}

	if _, err = pool.Exec(ctx, `UPDATE questionnaire_submissions SET mobile='x' WHERE id=$1`, first); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("immutable update err=%v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE questionnaire_submission_answers SET text_value='x' WHERE submission_id=$1`, first); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("immutable answer update err=%v", err)
	}

	updated := created
	updated.Title = unique("f03-revision")
	if _, err = realService(pool).Update(ctx, created.ID, surveyport.UpdateCommand{Questionnaire: updated, Actor: actor, IdempotencyKey: unique("f03-update")}); err != nil {
		t.Fatal(err)
	}
	replaced, err := service.Export(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(replaced.Body), "手机号 (2)") {
		t.Fatalf("snapshot-only question lost after definition replace: %q", replaced.Body)
	}

	if _, err = pool.Exec(ctx, `DELETE FROM questionnaire_submissions WHERE questionnaire_id=$1`, int64(created.ID)); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT submission_count FROM questionnaires WHERE id=$1`, int64(created.ID)).Scan(&storedCount); err != nil || storedCount != 0 {
		t.Fatalf("count after cleanup=%d err=%v", storedCount, err)
	}
	cleaned, err := service.Results(ctx, created.ID)
	if err != nil || cleaned.SubmissionCount != 0 || !cleaned.LatestSubmittedAt.IsZero() || cleaned.AverageScore != 0 {
		t.Fatalf("cleaned results=%#v err=%v", cleaned, err)
	}
}

func TestF03SubmissionCatalogOwnershipAndIndexes(t *testing.T) {
	pool, ctx := openPool(t)
	var submissionTables, tenantColumns, invalidIndexes, crossDomainFKs int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('questionnaire_submissions','questionnaire_submission_answers')),
      (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name LIKE 'questionnaire_submission%' AND column_name ~* 'tenant|workspace|organization'),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('questionnaire_submissions'::regclass,'questionnaire_submission_answers'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('questionnaire_submissions'::regclass,'questionnaire_submission_answers'::regclass)
        AND contype='f' AND confrelid NOT IN ('questionnaires'::regclass,'questionnaire_submissions'::regclass))`).Scan(&submissionTables, &tenantColumns, &invalidIndexes, &crossDomainFKs)
	if err != nil || submissionTables != 2 || tenantColumns != 0 || invalidIndexes != 0 || crossDomainFKs != 0 {
		t.Fatalf("catalog=%d/%d/%d/%d err=%v", submissionTables, tenantColumns, invalidIndexes, crossDomainFKs, err)
	}
	var pageIndex bool
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) = 1 FROM pg_indexes WHERE schemaname='public' AND tablename='questionnaire_submissions' AND indexname='questionnaire_submissions_page'`).Scan(&pageIndex); err != nil || !pageIndex {
		t.Fatalf("page index=%v err=%v", pageIndex, err)
	}
}

func f03Service(pool *pgxpool.Pool) *surveyapp.SubmissionService {
	return surveyapp.NewSubmissionService(platformstore.NewUnitOfWork(pool), surveystore.NewSubmissionRepository())
}

func f03InsertSubmission(t *testing.T, ctx context.Context, pool *pgxpool.Pool, questionnaireID surveyport.ID, externalUserID string, score float64, submittedAt time.Time) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(ctx, `INSERT INTO questionnaire_submissions
      (questionnaire_id, respondent_key, unionid, external_userid, customer_name, matched_by, mobile,
       source_channel, campaign_id, staff_id, total_score, final_tags, result_token, redirect_url_snapshot,
       submitted_at, created_at)
      VALUES ($1, $2, $3, $4, $5, 'external_userid', '13100000000', 'wecom', 'camp-f03', 'staff-f03', $6,
       '["qualified"]'::jsonb, $7, '', $8, $8) RETURNING id`,
		int64(questionnaireID), "resp-"+externalUserID, "union-"+externalUserID, externalUserID, "f03 用户", score,
		"token-"+externalUserID, submittedAt).Scan(&id)
	if err != nil || id < 1 {
		t.Fatalf("insert submission=%d err=%v", id, err)
	}
	return id
}

func f03InsertAnswer(t *testing.T, ctx context.Context, pool *pgxpool.Pool, submissionID, questionID int64, questionType, title string, sortOrder int, selectedOptions, textValue string) {
	t.Helper()
	createdAt := time.Date(2026, 8, 18, 1, 0, 1, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO questionnaire_submission_answers
      (submission_id, question_id, question_type, question_title, sort_order, selected_options, text_value, created_at)
      VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)`,
		submissionID, questionID, questionType, title, sortOrder, selectedOptions, textValue, createdAt); err != nil {
		t.Fatalf("insert answer err=%v", err)
	}
}
