package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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
		if current, err := repo.GetCurrentPublicDefinition(tx, surveyport.ID(qid)); err != nil || current.ID != def.ID || current.State != "public" || current.View.Version != def.View.Version {
			if err != nil {
				return err
			}
			return surveyapp.ErrUnavailable
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
		disabled, err := repo.DisablePublicDefinition(tx, def.View.ID, def.View.Version, now)
		if err != nil || disabled.View.Slug != "survey-it" || disabled.State != "disabled" {
			if err == nil {
				return surveyapp.ErrUnavailable
			}
			return err
		}
		if _, err = repo.GetCurrentPublicDefinition(tx, surveyport.ID(qid)); err != surveyapp.ErrNotFound {
			return surveyapp.ErrUnavailable
		}
		return errRollback
	})
	if err != errRollback {
		t.Fatalf("round trip=%v", err)
	}
}

// The real jsonb receipts must survive Publish/Disable and exact-key replay.
// Every repository write uses the outer transaction, which is always rolled back.
func TestPublicManagementPostgreSQLJSONBReplay(t *testing.T) {
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
	events := &surveyOperationsIntegrationEvents{}
	service := surveyapp.NewPublicService(publicManagementIntegrationUOW{}, repo, events, [32]byte{1})
	var questionnaireID surveyport.ID
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		db, err := platformstore.TxFromContext(tx)
		if err != nil {
			return err
		}
		q, err := NewQuestionnaireRepository().Create(tx, surveyport.CreateCommand{
			Actor: 1,
			Questionnaire: surveyport.Questionnaire{
				Slug: "survey-public-management-it", Name: "UAT public management", Title: "UAT", Description: "Test only",
				AnswerDisplayMode: surveyport.AllInOne,
				Questions: []surveyport.Question{{Type: surveyport.SingleChoice, Title: "Choose", Required: true,
					Validation: surveyport.Validation{MinimumSelections: intp(1), MaximumSelections: intp(1)},
					Options:    []surveyport.Option{{OptionText: "A", TagCodes: []string{}}}}},
			},
		}, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("create fixture: %w", err)
		}
		questionnaireID = q.ID
		var published surveyapp.PublicDefinitionRecord
		for index, operation := range []string{"publish", "disable"} {
			key := "survey-pg-public-" + operation + "-01"
			call := func() (surveyapp.PublicDefinitionRecord, error) {
				if operation == "publish" {
					return service.Publish(tx, surveyport.PublishPublicDefinitionCommand{QuestionnaireID: q.ID, ExpectedQuestionnaireVersion: q.Version, Actor: 1, IdempotencyKey: key})
				}
				return service.Disable(tx, surveyport.DisablePublicDefinitionCommand{QuestionnaireID: q.ID, ExpectedDefinitionVersion: published.View.Version, Actor: 1, IdempotencyKey: key})
			}
			first, err := call()
			if err != nil {
				return fmt.Errorf("%s: %w", operation, err)
			}
			if operation == "publish" {
				published = first
				if first.ID < 1 || first.State != "public" || first.View.ID != q.ID || len(first.View.Questions) != 1 || len(first.View.Questions[0].Options) != 1 {
					return fmt.Errorf("publish returned incomplete definition")
				}
			} else if first.ID != published.ID || first.State != "disabled" || !reflect.DeepEqual(first.View, published.View) {
				return fmt.Errorf("disable changed immutable definition")
			}
			replay, err := call()
			if err != nil || !reflect.DeepEqual(first, replay) || events.count != index+1 {
				return fmt.Errorf("%s replay changed result or duplicated event: %v", operation, err)
			}
			keyDigest := sha256.Sum256([]byte(key))
			var snapshot json.RawMessage
			if err = db.QueryRow(tx, `SELECT result_snapshot FROM questionnaire_public_management_receipts WHERE operation=$1 AND actor_scope='admin:1' AND key_digest=$2 AND state='completed'`, operation, keyDigest[:]).Scan(&snapshot); err != nil {
				return err
			}
			encoded, err := json.Marshal(first)
			var stored surveyapp.PublicDefinitionRecord
			if err != nil || string(encoded) == string(snapshot) || json.Unmarshal(snapshot, &stored) != nil || !reflect.DeepEqual(stored, first) {
				return fmt.Errorf("%s did not round-trip through reformatted jsonb", operation)
			}
		}
		if _, err = repo.GetCurrentPublicDefinition(tx, q.ID); err != surveyapp.ErrNotFound {
			return fmt.Errorf("disabled definition remained public: %v", err)
		}
		return errRollback
	})
	if err != errRollback {
		t.Fatalf("public management round trip: %v", err)
	}
	var remaining int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM questionnaires WHERE id=$1`, int64(questionnaireID)).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("fixture rollback remaining=%d err=%v", remaining, err)
	}
}

// Reuse the already-bound real transaction; production UoW rejects nesting.
type publicManagementIntegrationUOW struct{}

func (publicManagementIntegrationUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	if _, err := platformstore.TxFromContext(ctx); err != nil {
		return err
	}
	return callback(ctx)
}

var errRollback = &rollbackSentinel{}

type rollbackSentinel struct{}

func (*rollbackSentinel) Error() string { return "rollback" }
func intp(v int) *int                   { return &v }
