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
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.questionnaires (
			slug, name, title, description, answer_display_mode,
			assessment_enabled, assessment_config, is_disabled, created_by,
			version, submission_count, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`, value.Slug, value.Name, value.Title, value.Description, string(value.AnswerDisplayMode),
		value.AssessmentEnabled, assessmentConfig, value.IsDisabled, actor, value.Version,
		value.SubmissionCount, value.CreatedAt, value.UpdatedAt).Scan(&id)
	if err != nil {
		return 0, unavailable(err)
	}
	q := surveydbForTx(tx)
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
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.questionnaire_questions (
			questionnaire_id, type, title, required, sort_order, placeholder_text,
			assessment_dimension_key, sidebar_profile_field, validation,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11)
		RETURNING id
	`, questionnaireID, string(value.Type), value.Title, value.Required, value.SortOrder,
		value.PlaceholderText, value.AssessmentDimensionKey, value.SidebarProfileField,
		validation, value.CreatedAt, value.UpdatedAt).Scan(&id)
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
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.questionnaire_options (
			question_id, option_text, score, assessment_type_key, tag_codes,
			is_other, other_placeholder, other_max_length, sort_order,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`, questionID, value.OptionText, value.Score, value.AssessmentTypeKey, tags,
		value.IsOther, value.OtherPlaceholder, value.OtherMaxLength, value.SortOrder,
		value.CreatedAt, value.UpdatedAt).Scan(&id)
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
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.questionnaire_submissions (
			questionnaire_id, respondent_key, openid, unionid, external_userid,
			customer_name, follow_user_userid, matched_by, mobile, source_channel,
			campaign_id, staff_id, total_score, final_tags, result_token,
			redirect_url_snapshot, submitted_at, created_at
		) VALUES ($1, '', '', $2, '', '', $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12, $13, $14)
		RETURNING id
	`, questionnaireID, value.UnionID, value.FollowUserUserID, value.MatchedBy, value.Mobile,
		value.SourceChannel, value.CampaignID, value.StaffID, value.TotalScore, tags,
		value.ResultToken, value.RedirectURLSnapshot, value.SubmittedAt, value.CreatedAt).Scan(&id)
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
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.questionnaire_submission_answers (
			submission_id, question_id, question_type, question_title, sort_order,
			selected_options, text_value, created_at
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
		RETURNING id
	`, submissionID, questionID, string(value.QuestionType), value.QuestionTitle, value.SortOrder,
		selectedOptions, value.TextValue, value.CreatedAt).Scan(&id)
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
