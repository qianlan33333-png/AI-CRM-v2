package v1domain

import (
	"context"
	"errors"
	"flag"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
)

var surveyArchiveRun = flag.String("survey-archive-run", "", "optional reconciled V2 archive run for read-only Survey validation")
var surveyArchiveRollback = flag.Bool("survey-archive-rollback", false, "exercise Survey import SQL in one forced-rollback transaction")

var errSurveyValidated = errors.New("survey validated without opening a target transaction")
var errSurveyRollback = errors.New("rollback Survey import rehearsal")

type surveyValidationOnlyUOW struct{}

func (surveyValidationOnlyUOW) Within(context.Context, func(context.Context) error) error {
	return errSurveyValidated
}

// Any accidental call to the embedded nil store fails the test. Validation
// must finish before the UoW sentinel, without accessing a target writer.
type unreachableSurveyStore struct{ surveyapp.ImportStore }

// surveyRollbackOnlyUOW lets the import service complete its owner-owned SQL,
// then rolls it back and reports success so its mapped result can be checked.
type surveyRollbackOnlyUOW struct{ inner *platformstore.UnitOfWork }

func (uow surveyRollbackOnlyUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	err := uow.inner.Within(ctx, func(tx context.Context) error {
		if err := callback(tx); err != nil {
			return err
		}
		return errSurveyRollback
	})
	if errors.Is(err, errSurveyRollback) {
		return nil
	}
	return err
}

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
	importableRows, quarantinedRows := 0, 0
	for _, questionnaire := range questionnaires {
		t.Run(strconv.FormatInt(questionnaire.value.ID, 10), func(t *testing.T) {
			aggregate, rows := buildSurveyAggregate(questionnaire, groupSurveyQuestions(questions), groupSurveyOptions(options),
				groupSurveySubmissions(submissions), groupSurveyAnswers(answers), consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers)
			partition := partitionSurveyAggregate(aggregate, rows)
			assertSurveyPartitionConservesRows(t, rows, partition)
			importableRows += len(partition.ImportRows)
			quarantinedRows += len(partition.QuarantinedRows)
			request := surveyport.ImportRequest{MigrationActor: 1, RunID: surveyImportRunID,
				IdempotencyKey: SourceIdentifier(questionnaire.archive.SourceKeyHMAC), Aggregate: partition.Importable}
			_, err := service.Import(ctx, request)
			if !errors.Is(err, errSurveyValidated) {
				t.Errorf("aggregate validation failed: %v (questions=%d options=%d submissions=%d answers=%d)", err,
					len(partition.Importable.Questions), len(partition.Importable.Options), len(partition.Importable.Submissions), len(partition.Importable.Answers))
			}
		})
	}
	if orphans := collectSurveyOrphans(questions, options, submissions, answers, consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers); len(orphans) != 0 {
		t.Errorf("unresolved archive relations: %d rows", len(orphans))
	}
	t.Logf("read-only validation: questionnaires=%d questions=%d options=%d submissions=%d answers=%d importable_rows=%d quarantine_rows=%d", len(questionnaires), len(questions), len(options), len(submissions), len(answers), importableRows, quarantinedRows)
}

func TestReconciledSurveyArchiveImportRollback(t *testing.T) {
	if *surveyArchiveRun == "" || !*surveyArchiveRollback {
		t.Skip("supply -survey-archive-run and -survey-archive-rollback for the forced-rollback SQL rehearsal")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, environment.TargetDatabaseURL)
	if err != nil {
		t.Fatal("cannot open V2 rehearsal database")
	}
	defer pool.Close()
	before, err := surveydb.New(pool).CountQuestionnaires(ctx)
	if err != nil {
		t.Fatal("cannot count Survey definitions before rehearsal")
	}
	if before != 0 {
		t.Fatalf("rollback rehearsal requires an empty Survey target, found questionnaires=%d", before)
	}
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("cannot open V2 archive for rollback rehearsal")
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
	service := surveyapp.NewImportService(surveyRollbackOnlyUOW{platformstore.NewUnitOfWork(pool)}, surveystore.NewQuestionnaireRepository())
	consumedQuestions, consumedOptions := map[int64]bool{}, map[int64]bool{}
	consumedSubmissions, consumedAnswers := map[int64]bool{}, map[int64]bool{}
	importableRows, quarantinedRows := 0, 0
	for _, questionnaire := range questionnaires {
		aggregate, rows := buildSurveyAggregate(questionnaire, groupSurveyQuestions(questions), groupSurveyOptions(options),
			groupSurveySubmissions(submissions), groupSurveyAnswers(answers), consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers)
		partition := partitionSurveyAggregate(aggregate, rows)
		assertSurveyPartitionConservesRows(t, rows, partition)
		imported, err := service.Import(ctx, surveyport.ImportRequest{MigrationActor: 1, RunID: surveyImportRunID,
			IdempotencyKey: SourceIdentifier(questionnaire.archive.SourceKeyHMAC), Aggregate: partition.Importable})
		if err != nil {
			t.Fatalf("questionnaire %d rollback rehearsal failed: %v", questionnaire.value.ID, err)
		}
		if imported.ImportedQuestions != len(partition.Importable.Questions) || imported.ImportedOptions != len(partition.Importable.Options) ||
			imported.ImportedSubmissions != len(partition.Importable.Submissions) || imported.ImportedAnswers != len(partition.Importable.Answers) {
			t.Fatalf("questionnaire %d rollback result does not cover importable aggregate", questionnaire.value.ID)
		}
		importableRows += len(partition.ImportRows)
		quarantinedRows += len(partition.QuarantinedRows)
	}
	after, err := surveydb.New(pool).CountQuestionnaires(ctx)
	if err != nil {
		t.Fatal("cannot count Survey definitions after rehearsal")
	}
	if after != before {
		t.Fatalf("rollback rehearsal persisted questionnaires: before=%d after=%d", before, after)
	}
	if orphans := collectSurveyOrphans(questions, options, submissions, answers, consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers); len(orphans) != 0 {
		t.Fatalf("unresolved archive relations: %d rows", len(orphans))
	}
	t.Logf("forced rollback rehearsal: questionnaires=%d importable_rows=%d quarantine_rows=%d persisted_questionnaires=%d", len(questionnaires), importableRows, quarantinedRows, after)
}
