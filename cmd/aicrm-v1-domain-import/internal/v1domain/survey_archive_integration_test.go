package v1domain

import (
	"context"
	"errors"
	"flag"
	"strconv"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

var surveyArchiveRun = flag.String("survey-archive-run", "", "optional reconciled V2 archive run for read-only Survey validation")

var errSurveyValidated = errors.New("survey validated without opening a target transaction")

type surveyValidationOnlyUOW struct{}

func (surveyValidationOnlyUOW) Within(context.Context, func(context.Context) error) error {
	return errSurveyValidated
}

// Any accidental call to the embedded nil store fails the test. Validation
// must finish before the UoW sentinel, without accessing a target writer.
type unreachableSurveyStore struct{ surveyapp.ImportStore }

func TestReconciledSurveyArchiveValidatesWithoutWrites(t *testing.T) {
	if *surveyArchiveRun == "" {
		t.Skip("supply -survey-archive-run and V2 archive environment for read-only validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("cannot open V2 archive for validation")
	}
	defer archive.Close()
	questionnaires, err := readArchivedValues[surveyQuestionnaireJSON](ctx, archive, *surveyArchiveRun, "public/questionnaires")
	if err != nil {
		t.Fatal(err)
	}
	questions, err := readArchivedValues[surveyQuestionJSON](ctx, archive, *surveyArchiveRun, "public/questionnaire_questions")
	if err != nil {
		t.Fatal(err)
	}
	options, err := readArchivedValues[surveyOptionJSON](ctx, archive, *surveyArchiveRun, "public/questionnaire_options")
	if err != nil {
		t.Fatal(err)
	}
	submissions, err := readArchivedValues[surveySubmissionJSON](ctx, archive, *surveyArchiveRun, "public/questionnaire_submissions")
	if err != nil {
		t.Fatal(err)
	}
	answers, err := readArchivedValues[surveyAnswerJSON](ctx, archive, *surveyArchiveRun, "public/questionnaire_submission_answers")
	if err != nil {
		t.Fatal(err)
	}
	if len(questionnaires) == 0 {
		t.Fatal("archive contains no questionnaires")
	}
	service := surveyapp.NewImportService(surveyValidationOnlyUOW{}, &unreachableSurveyStore{})
	consumedQuestions, consumedOptions := map[int64]bool{}, map[int64]bool{}
	consumedSubmissions, consumedAnswers := map[int64]bool{}, map[int64]bool{}
	for _, questionnaire := range questionnaires {
		t.Run(strconv.FormatInt(questionnaire.value.ID, 10), func(t *testing.T) {
			aggregate, _ := buildSurveyAggregate(questionnaire, groupSurveyQuestions(questions), groupSurveyOptions(options),
				groupSurveySubmissions(submissions), groupSurveyAnswers(answers), consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers)
			request := surveyport.ImportRequest{MigrationActor: 1, RunID: surveyImportRunID,
				IdempotencyKey: SourceIdentifier(questionnaire.archive.SourceKeyHMAC), Aggregate: aggregate}
			_, err := service.Import(ctx, request)
			if !errors.Is(err, errSurveyValidated) {
				t.Errorf("aggregate validation failed: %v (questions=%d options=%d submissions=%d answers=%d)", err,
					len(aggregate.Questions), len(aggregate.Options), len(aggregate.Submissions), len(aggregate.Answers))
			}
		})
	}
	if orphans := collectSurveyOrphans(questions, options, submissions, answers, consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers); len(orphans) != 0 {
		t.Errorf("unresolved archive relations: %d rows", len(orphans))
	}
	t.Logf("read-only validation: questionnaires=%d questions=%d options=%d submissions=%d answers=%d", len(questionnaires), len(questions), len(options), len(submissions), len(answers))
}
