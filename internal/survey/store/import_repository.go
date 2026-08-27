package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
)

var _ surveyapp.ImportStore = (*QuestionnaireRepository)(nil)

// CreateImportedQuestionnaire inserts one historical definition using a V2
// generated ID. The caller owns the transaction and supplies the one actor
// for the complete aggregate.
func (r *QuestionnaireRepository) CreateImportedQuestionnaire(ctx context.Context, value surveyport.ImportQuestionnaire, actor int64) (int64, error) {
	tx, err := importTx(ctx, r)
	if err != nil {
		return 0, err
	}
	assessmentConfig := value.AssessmentConfig
	if len(assessmentConfig) == 0 {
		assessmentConfig = json.RawMessage(`{}`)
	}
	q := surveydbForTx(tx)
	id, err := q.InsertHistoricalQuestionnaire(ctx, surveydb.InsertHistoricalQuestionnaireParams{
		Slug: value.Slug, Name: value.Name, Title: value.Title, Description: value.Description,
		AnswerDisplayMode: string(value.AnswerDisplayMode), AssessmentEnabled: value.AssessmentEnabled,
		AssessmentConfig: assessmentConfig, IsDisabled: value.IsDisabled, CreatedBy: actor,
		Version: int32(value.Version), SubmissionCount: int32(value.SubmissionCount),
		CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt),
	})
	if err != nil {
		return 0, unavailable(err)
	}
	if _, err = q.FinalizeQuestionnaireSlug(ctx, id); err != nil {
		return 0, unavailable(err)
	}
	if _, err = q.IncrementQuestionnaireCount(ctx); err != nil {
		return 0, unavailable(err)
	}
	return id, nil
}

// CreateImportedQuestion preserves the source timestamp pair rather than
// replacing it with the import clock.
func (r *QuestionnaireRepository) CreateImportedQuestion(ctx context.Context, questionnaireID int64, value surveyport.ImportQuestion, validation json.RawMessage) (int64, error) {
	tx, err := importTx(ctx, r)
	if err != nil {
		return 0, err
	}
	if len(validation) == 0 {
		validation = json.RawMessage(`{}`)
	}
	id, err := surveydbForTx(tx).InsertHistoricalQuestion(ctx, surveydb.InsertHistoricalQuestionParams{
		QuestionnaireID: questionnaireID, QuestionType: string(value.Type), Title: value.Title,
		Required: value.Required, SortOrder: int32(value.SortOrder), PlaceholderText: value.PlaceholderText,
		AssessmentDimensionKey: value.AssessmentDimensionKey, SidebarProfileField: value.SidebarProfileField,
		Validation: validation, CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt),
	})
	if err != nil {
		return 0, unavailable(err)
	}
	return id, nil
}

func (r *QuestionnaireRepository) CreateImportedOption(ctx context.Context, questionID int64, value surveyport.ImportOption) (int64, error) {
	tx, err := importTx(ctx, r)
	if err != nil {
		return 0, err
	}
	tags := value.TagCodes
	if len(tags) == 0 {
		tags = json.RawMessage(`[]`)
	}
	id, err := surveydbForTx(tx).InsertHistoricalOption(ctx, surveydb.InsertHistoricalOptionParams{
		QuestionID: questionID, OptionText: value.OptionText, Score: value.Score,
		AssessmentTypeKey: value.AssessmentTypeKey, TagCodes: tags, IsOther: value.IsOther,
		OtherPlaceholder: value.OtherPlaceholder, OtherMaxLength: int32(value.OtherMaxLength),
		SortOrder: int32(value.SortOrder), CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt),
	})
	if err != nil {
		return 0, unavailable(err)
	}
	return id, nil
}

func (r *QuestionnaireRepository) CreateImportedSubmission(ctx context.Context, questionnaireID int64, value surveyport.ImportSubmission) (int64, error) {
	tx, err := importTx(ctx, r)
	if err != nil {
		return 0, err
	}
	tags := value.FinalTags
	if len(tags) == 0 {
		tags = json.RawMessage(`[]`)
	}
	id, err := surveydbForTx(tx).InsertHistoricalSubmission(ctx, surveydb.InsertHistoricalSubmissionParams{
		QuestionnaireID: questionnaireID, Unionid: value.UnionID, FollowUserUserid: value.FollowUserUserID,
		MatchedBy: value.MatchedBy, Mobile: value.Mobile, SourceChannel: value.SourceChannel,
		CampaignID: value.CampaignID, StaffID: value.StaffID, TotalScore: value.TotalScore,
		FinalTags: tags, ResultToken: value.ResultToken, RedirectUrlSnapshot: value.RedirectURLSnapshot,
		SubmittedAt: timestamp(value.SubmittedAt), CreatedAt: timestamp(value.CreatedAt),
	})
	if err != nil {
		return 0, unavailable(err)
	}
	return id, nil
}

func (r *QuestionnaireRepository) CreateImportedAnswer(ctx context.Context, submissionID, questionID int64, value surveyport.ImportAnswer, selectedOptions json.RawMessage) (int64, error) {
	tx, err := importTx(ctx, r)
	if err != nil {
		return 0, err
	}
	if len(selectedOptions) == 0 {
		selectedOptions = json.RawMessage(`[]`)
	}
	id, err := surveydbForTx(tx).InsertHistoricalAnswer(ctx, surveydb.InsertHistoricalAnswerParams{
		SubmissionID: submissionID, QuestionID: questionID, QuestionType: string(value.QuestionType),
		QuestionTitle: value.QuestionTitle, SortOrder: int32(value.SortOrder), SelectedOptions: selectedOptions,
		TextValue: value.TextValue, CreatedAt: timestamp(value.CreatedAt),
	})
	if err != nil {
		return 0, unavailable(err)
	}
	return id, nil
}

func importTx(ctx context.Context, r *QuestionnaireRepository) (pgx.Tx, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if r == nil || err != nil {
		return nil, unavailable(err)
	}
	return tx, nil
}

func surveydbForTx(tx pgx.Tx) *surveydb.Queries {
	return surveydb.New(tx)
}
