package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
)

type QuestionnaireRepository struct{}

var _ surveyapp.Store = (*QuestionnaireRepository)(nil)

func NewQuestionnaireRepository() *QuestionnaireRepository { return &QuestionnaireRepository{} }

func queries(ctx context.Context) (*surveydb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return surveydb.New(tx), nil
}

func (r *QuestionnaireRepository) ListOffset(ctx context.Context, limit, offset int32) ([]surveyport.Questionnaire, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || limit < 1 || offset < 0 {
		return nil, unavailable(err)
	}
	rows, err := q.ListQuestionnairesOffset(ctx, surveydb.ListQuestionnairesOffsetParams{RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, unavailable(err)
	}
	result := make([]surveyport.Questionnaire, len(rows))
	for i, row := range rows {
		result[i], err = mapQuestionnaire(row.ID, row.Slug, row.Name, row.Title, row.Description, row.AnswerDisplayMode,
			row.AssessmentEnabled, row.AssessmentConfig, row.IsDisabled, row.CreatedBy, row.Version, row.SubmissionCount,
			row.CreatedAt, row.UpdatedAt, row.Questions)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *QuestionnaireRepository) Count(ctx context.Context) (int64, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return 0, unavailable(err)
	}
	total, err := q.CountQuestionnaires(ctx)
	if err != nil || total < 0 {
		return 0, unavailable(err)
	}
	return total, nil
}

func (r *QuestionnaireRepository) Get(ctx context.Context, id surveyport.ID) (surveyport.Questionnaire, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	row, err := q.GetQuestionnaire(ctx, int64(id))
	if err != nil {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	return mapQuestionnaire(row.ID, row.Slug, row.Name, row.Title, row.Description, row.AnswerDisplayMode,
		row.AssessmentEnabled, row.AssessmentConfig, row.IsDisabled, row.CreatedBy, row.Version, row.SubmissionCount,
		row.CreatedAt, row.UpdatedAt, row.Questions)
}

func (r *QuestionnaireRepository) Create(ctx context.Context, command surveyport.CreateCommand, now time.Time) (surveyport.Questionnaire, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	stamp := pgtype.Timestamptz{Time: now, Valid: true}
	id, err := q.CreateQuestionnaire(ctx, surveydb.CreateQuestionnaireParams{
		Slug: command.Slug, Name: command.Name, Title: command.Title, Description: command.Description,
		AnswerDisplayMode: string(command.AnswerDisplayMode), IsDisabled: command.IsDisabled,
		CreatedBy: command.Actor, CreatedAt: stamp,
	})
	if err != nil {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	if _, err = q.FinalizeQuestionnaireSlug(ctx, id); err != nil {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	if err := r.replaceChildren(ctx, id, command.Questions, stamp); err != nil {
		return surveyport.Questionnaire{}, err
	}
	if total, err := q.IncrementQuestionnaireCount(ctx); err != nil || total < 1 {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	return r.Get(ctx, surveyport.ID(id))
}

func (r *QuestionnaireRepository) Update(ctx context.Context, id surveyport.ID, command surveyport.UpdateCommand, now time.Time) (surveyport.Questionnaire, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	stamp := pgtype.Timestamptz{Time: now, Valid: true}
	updated, err := q.UpdateQuestionnaire(ctx, surveydb.UpdateQuestionnaireParams{
		QuestionnaireID: int64(id), Slug: command.Slug, Name: command.Name, Title: command.Title,
		Description: command.Description, AnswerDisplayMode: string(command.AnswerDisplayMode),
		IsDisabled: command.IsDisabled, UpdatedAt: stamp,
	})
	if err != nil || updated != int64(id) {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	if err = q.DeleteQuestionnaireChildren(ctx, int64(id)); err != nil {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	if err = r.replaceChildren(ctx, int64(id), command.Questions, stamp); err != nil {
		return surveyport.Questionnaire{}, err
	}
	return r.Get(ctx, id)
}

func (r *QuestionnaireRepository) SetDisabled(ctx context.Context, id surveyport.ID, disabled bool, now time.Time) (surveyport.Questionnaire, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	updated, err := q.SetQuestionnaireDisabled(ctx, surveydb.SetQuestionnaireDisabledParams{QuestionnaireID: int64(id), IsDisabled: disabled, UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if err != nil || updated != int64(id) {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	return r.Get(ctx, id)
}

func (r *QuestionnaireRepository) Delete(ctx context.Context, id surveyport.ID) (surveyport.Questionnaire, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	item, err := r.Get(ctx, id)
	if err != nil {
		return surveyport.Questionnaire{}, err
	}
	if !item.IsDisabled {
		return surveyport.Questionnaire{}, surveyapp.ErrInvalidQuestionnaire
	}
	deleted, err := q.DeleteDisabledQuestionnaire(ctx, int64(id))
	if err != nil || deleted != int64(id) {
		return surveyport.Questionnaire{}, unavailable(err)
	}
	if total, countErr := q.DecrementQuestionnaireCount(ctx); countErr != nil || total < 0 {
		return surveyport.Questionnaire{}, unavailable(countErr)
	}
	return item, nil
}

func (r *QuestionnaireRepository) replaceChildren(ctx context.Context, questionnaireID int64, questions []surveyport.Question, stamp pgtype.Timestamptz) error {
	q, err := queries(ctx)
	if err != nil {
		return unavailable(err)
	}
	for _, question := range questions {
		validation, marshalErr := json.Marshal(question.Validation)
		if marshalErr != nil {
			return surveyapp.ErrUnavailable
		}
		questionID, insertErr := q.InsertQuestionnaireQuestion(ctx, surveydb.InsertQuestionnaireQuestionParams{
			QuestionnaireID: questionnaireID, QuestionType: string(question.Type), Title: question.Title, Required: question.Required,
			SortOrder: int32(question.SortOrder), PlaceholderText: question.PlaceholderText,
			AssessmentDimensionKey: question.AssessmentDimensionKey, SidebarProfileField: question.SidebarProfileField,
			Validation: validation, CreatedAt: stamp,
		})
		if insertErr != nil {
			return unavailable(insertErr)
		}
		for _, option := range question.Options {
			codes, marshalErr := json.Marshal(option.TagCodes)
			if marshalErr != nil {
				return surveyapp.ErrUnavailable
			}
			if _, insertErr = q.InsertQuestionnaireOption(ctx, surveydb.InsertQuestionnaireOptionParams{
				QuestionID: questionID, OptionText: option.OptionText, Score: option.Score,
				AssessmentTypeKey: option.AssessmentTypeKey, TagCodes: codes, IsOther: option.IsOther,
				OtherPlaceholder: option.OtherPlaceholder, OtherMaxLength: int32(option.OtherMaximumLength),
				SortOrder: int32(option.SortOrder), CreatedAt: stamp,
			}); insertErr != nil {
				return unavailable(insertErr)
			}
		}
	}
	return nil
}

func (r *QuestionnaireRepository) Reserve(ctx context.Context, operation string, value surveyapp.Reservation) (surveyapp.Receipt, bool, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return surveyapp.Receipt{}, false, unavailable(err)
	}
	row, err := q.ReserveQuestionnaireOperationReceipt(ctx, surveydb.ReserveQuestionnaireOperationReceiptParams{
		Operation: operation, ActorScope: value.ActorScope, KeyDigest: value.KeyDigest[:], PayloadDigest: value.PayloadDigest[:],
		CreatedAt: pgtype.Timestamptz{Time: value.CreatedAt, Valid: true},
	})
	if err == nil {
		return receipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return surveyapp.Receipt{}, false, unavailable(err)
	}
	old, err := q.GetQuestionnaireOperationReceipt(ctx, surveydb.GetQuestionnaireOperationReceiptParams{Operation: operation, ActorScope: value.ActorScope, KeyDigest: value.KeyDigest[:]})
	if err != nil {
		return surveyapp.Receipt{}, false, unavailable(err)
	}
	return receipt(old.ID, old.Operation, old.ActorScope, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}

func (r *QuestionnaireRepository) Complete(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (surveyapp.Receipt, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 || !json.Valid(snapshot) {
		return surveyapp.Receipt{}, unavailable(err)
	}
	row, err := q.CompleteQuestionnaireOperationReceipt(ctx, surveydb.CompleteQuestionnaireOperationReceiptParams{
		ID: id, ResultSnapshot: snapshot, CompletedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return surveyapp.Receipt{}, unavailable(err)
	}
	return receipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func (r *QuestionnaireRepository) ReserveManagement(ctx context.Context, operation string, value surveyapp.Reservation) (surveyapp.Receipt, bool, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return surveyapp.Receipt{}, false, unavailable(err)
	}
	row, err := q.ReserveQuestionnaireManagementReceipt(ctx, surveydb.ReserveQuestionnaireManagementReceiptParams{
		Operation: operation, ActorScope: value.ActorScope, KeyDigest: value.KeyDigest[:], PayloadDigest: value.PayloadDigest[:],
		CreatedAt: pgtype.Timestamptz{Time: value.CreatedAt, Valid: true},
	})
	if err == nil {
		return receipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return surveyapp.Receipt{}, false, unavailable(err)
	}
	old, err := q.GetQuestionnaireManagementReceipt(ctx, surveydb.GetQuestionnaireManagementReceiptParams{Operation: operation, ActorScope: value.ActorScope, KeyDigest: value.KeyDigest[:]})
	if err != nil {
		return surveyapp.Receipt{}, false, unavailable(err)
	}
	return receipt(old.ID, old.Operation, old.ActorScope, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}

func (r *QuestionnaireRepository) CompleteManagement(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (surveyapp.Receipt, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 || !json.Valid(snapshot) {
		return surveyapp.Receipt{}, unavailable(err)
	}
	row, err := q.CompleteQuestionnaireManagementReceipt(ctx, surveydb.CompleteQuestionnaireManagementReceiptParams{
		ID: id, ResultSnapshot: snapshot, CompletedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return surveyapp.Receipt{}, unavailable(err)
	}
	return receipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func mapQuestionnaire(id int64, slug, name, title, description, mode string, assessment bool, config []byte,
	disabled bool, createdBy, version, submissions int64, createdAt, updatedAt pgtype.Timestamptz, rawQuestions any,
) (surveyport.Questionnaire, error) {
	if !createdAt.Valid || !updatedAt.Valid {
		return surveyport.Questionnaire{}, surveyapp.ErrUnavailable
	}
	var questionsJSON []byte
	switch value := rawQuestions.(type) {
	case []byte:
		questionsJSON = append([]byte{}, value...)
	case string:
		questionsJSON = []byte(value)
	default:
		var err error
		questionsJSON, err = json.Marshal(value)
		if err != nil {
			return surveyport.Questionnaire{}, surveyapp.ErrUnavailable
		}
	}
	var questions []surveyport.Question
	if json.Unmarshal(questionsJSON, &questions) != nil {
		return surveyport.Questionnaire{}, surveyapp.ErrUnavailable
	}
	return surveyport.Questionnaire{
		ID: surveyport.ID(id), Slug: slug, Name: name, Title: title, Description: description,
		AnswerDisplayMode: surveyport.AnswerDisplayMode(mode), AssessmentEnabled: assessment,
		AssessmentConfig: append(json.RawMessage{}, config...), IsDisabled: disabled, CreatedBy: createdBy,
		Version: version, SubmissionCount: submissions, Questions: questions, ScoreRules: []surveyport.ScoreRule{},
		CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time,
	}, nil
}

func receipt(id int64, operation, actor string, key, payload []byte, state string, snapshot []byte) surveyapp.Receipt {
	var result surveyapp.Receipt
	result.ID, result.Operation, result.ActorScope, result.State = id, operation, actor, state
	result.ResultSnapshot = append(json.RawMessage{}, snapshot...)
	copy(result.KeyDigest[:], key)
	copy(result.PayloadDigest[:], payload)
	return result
}

func unavailable(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return surveyapp.ErrNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return surveyapp.ErrConflict
		}
		return err
	}
	return surveyapp.ErrUnavailable
}
