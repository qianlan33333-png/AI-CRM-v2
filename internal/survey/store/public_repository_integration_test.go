package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

// Uses the already-migrated dedicated local database. It is opt-in so normal
// unit runs cannot touch PostgreSQL; each scenario rolls its transaction back.
func TestPublicRepositoryPostgreSQLRoundTrip(t *testing.T) {
	url := os.Getenv("AICRM_SURVEY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AICRM_SURVEY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := NewPublicRepository()
	uow := platformstore.NewUnitOfWork(pool)
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	err = uow.Within(ctx, func(tx context.Context) error {
		db, err := platformstore.TxFromContext(tx)
		if err != nil {
			return err
		}
		var qid int64
		err = db.QueryRow(tx, `INSERT INTO questionnaires(slug,name,title,description,answer_display_mode,assessment_enabled,assessment_config,is_disabled,created_by,version,submission_count,created_at,updated_at) VALUES('survey-it','it','匿名','', 'all_in_one',false,'{}',false,1,1,0,$1,$1) RETURNING id`, now).Scan(&qid)
		if err != nil {
			return err
		}
		source := surveyport.Questionnaire{ID: surveyport.ID(qid), Slug: "survey-it", Title: "匿名", AnswerDisplayMode: surveyport.AllInOne, Version: 1, Questions: []surveyport.Question{{ID: 101, Type: surveyport.SingleChoice, Title: "选择", Required: true, SortOrder: 0, Validation: surveyport.Validation{MinimumSelections: intp(1), MaximumSelections: intp(1)}, Options: []surveyport.Option{{ID: 201, OptionText: "A", SortOrder: 0}}}}}
		def, err := repo.CreatePublicDefinition(tx, source, now)
		if err != nil {
			return err
		}
		if _, err = repo.GetPublishedBySlug(tx, "survey-it"); err != nil {
			return err
		}
		anon, key, payload := sha256.Sum256([]byte("anon")), sha256.Sum256([]byte("key")), sha256.Sum256([]byte("payload"))
		receipt, owned, err := repo.ReservePublicReceipt(tx, def, anon, key, payload, now)
		if err != nil || !owned {
			return err
		}
		sid, err := repo.CreatePublicSubmission(tx, receipt.ID, def.ID, now, []surveyport.PublicSubmissionAnswer{{QuestionID: def.View.Questions[0].ID, OptionIDs: []int64{def.View.Questions[0].Options[0].ID}}})
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(surveyport.PublicSubmissionReceipt{QuestionnaireID: def.View.ID, QuestionnaireSlug: def.View.Slug, DefinitionVersion: def.View.Version, SubmissionID: sid})
		token := sha256.Sum256([]byte("token"))
		if _, err = repo.CompletePublicReceipt(tx, receipt.ID, token, snapshot, now); err != nil {
			return err
		}
		if _, owned, err = repo.ReservePublicReceipt(tx, def, anon, key, payload, now); err != nil || owned {
			return surveyapp.ErrUnavailable
		}
		if _, err = repo.LookupPublicResult(tx, token); err != nil {
			return err
		}
		if _, err = repo.PublicAnalytics(tx, def); err != nil {
			return err
		}
		for i := 0; i < 5; i++ {
			rotatedCookie := sha256.Sum256([]byte("rotated-cookie-" + string(rune(i))))
			if err = repo.ConsumePublicRate(tx, def.ID, anon, rotatedCookie, now); err != nil {
				return err
			}
		}
		if err = repo.ConsumePublicRate(tx, def.ID, anon, sha256.Sum256([]byte("fresh-cookie")), now); err != surveyapp.ErrPublicRateLimited {
			return surveyapp.ErrUnavailable
		}
		otherSource := sha256.Sum256([]byte("other-controlled-source"))
		if err = repo.ConsumePublicRate(tx, def.ID, otherSource, sha256.Sum256([]byte("other-cookie")), now); err != nil {
			return err
		}
		if _, err = repo.DisablePublicDefinition(tx, def.View.ID, def.View.Version, now); err != nil {
			return err
		}
		return errRollback
	})
	if err != errRollback {
		t.Fatalf("round trip=%v", err)
	}
}

var errRollback = &rollbackSentinel{}

type rollbackSentinel struct{}

func (*rollbackSentinel) Error() string { return "rollback" }
func intp(v int) *int                   { return &v }
