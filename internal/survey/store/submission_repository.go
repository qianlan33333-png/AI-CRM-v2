package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
)

// SubmissionRepository reads only Survey-owned submission snapshot tables.
// It never joins or resolves Contact, Identity, WeCom, or Outbound tables.
type SubmissionRepository struct{}

var _ surveyapp.SubmissionStore = (*SubmissionRepository)(nil)
var _ surveyapp.CustomerAnswerCandidateStore = (*SubmissionRepository)(nil)

func NewSubmissionRepository() *SubmissionRepository { return &SubmissionRepository{} }

func (r *SubmissionRepository) Results(ctx context.Context, id surveyport.ID) (surveyport.SubmissionResult, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 {
		return surveyport.SubmissionResult{}, unavailable(err)
	}
	row, err := q.GetQuestionnaireSubmissionResults(ctx, int64(id))
	if err != nil {
		return surveyport.SubmissionResult{}, unavailable(err)
	}
	latest, err := submissionLatestTime(row.LatestSubmittedAt)
	if err != nil || row.QuestionnaireID != int64(id) || row.SubmissionCount < 0 {
		return surveyport.SubmissionResult{}, surveyapp.ErrUnavailable
	}
	if row.SubmissionCount == 0 && (!latest.IsZero() || row.AverageScore != 0) {
		return surveyport.SubmissionResult{}, surveyapp.ErrUnavailable
	}
	return surveyport.SubmissionResult{
		QuestionnaireID:   surveyport.ID(row.QuestionnaireID),
		SubmissionCount:   row.SubmissionCount,
		LatestSubmittedAt: latest,
		AverageScore:      row.AverageScore,
		Rules:             []surveyport.ScoreRule{},
	}, nil
}

func (r *SubmissionRepository) SubmissionOwnerExists(ctx context.Context, id surveyport.ID) (bool, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 {
		return false, unavailable(err)
	}
	return q.QuestionnaireSubmissionOwnerExists(ctx, int64(id))
}

func (r *SubmissionRepository) CountSubmissions(ctx context.Context, id surveyport.ID) (int64, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 {
		return 0, unavailable(err)
	}
	total, err := q.CountQuestionnaireSubmissions(ctx, int64(id))
	if err != nil || total < 0 {
		return 0, unavailable(err)
	}
	return total, nil
}

func (r *SubmissionRepository) ListSubmissions(ctx context.Context, id surveyport.ID, limit, offset int32) ([]surveyport.Submission, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 || limit < 1 || offset < 0 {
		return nil, unavailable(err)
	}
	rows, err := q.ListQuestionnaireSubmissions(ctx, surveydb.ListQuestionnaireSubmissionsParams{
		QuestionnaireID: int64(id), RowOffset: offset, RowLimit: limit,
	})
	if err != nil {
		return nil, unavailable(err)
	}
	result := make([]surveyport.Submission, len(rows))
	for i, row := range rows {
		result[i], err = mapSubmission(row.ID, row.QuestionnaireID, row.RespondentKey, row.Openid, row.Unionid,
			row.ExternalUserid, row.CustomerName, row.FollowUserUserid, row.MatchedBy, row.Mobile,
			row.SourceChannel, row.CampaignID, row.StaffID, row.TotalScore, row.FinalTags,
			row.ResultToken, row.RedirectUrlSnapshot, row.SubmittedAt, row.CreatedAt, row.Answers)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *SubmissionRepository) ExportDefinition(ctx context.Context, id surveyport.ID) (string, []surveyport.SubmissionExportQuestion, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 {
		return "", nil, unavailable(err)
	}
	row, err := q.GetQuestionnaireSubmissionExportDefinition(ctx, int64(id))
	if err != nil {
		return "", nil, unavailable(err)
	}
	raw, err := jsonBytes(row.Questions)
	if err != nil {
		return "", nil, surveyapp.ErrUnavailable
	}
	var questions []struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		SortOrder int    `json:"sort_order"`
	}
	if json.Unmarshal(raw, &questions) != nil {
		return "", nil, surveyapp.ErrUnavailable
	}
	result := make([]surveyport.SubmissionExportQuestion, len(questions))
	for i, question := range questions {
		result[i] = surveyport.SubmissionExportQuestion{ID: question.ID, Title: question.Title, SortOrder: question.SortOrder}
	}
	return row.Slug, result, nil
}

func (r *SubmissionRepository) ListRecentCustomerAnswerCandidates(ctx context.Context, limit int32) ([]surveyapp.CustomerAnswerCandidate, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || limit < 1 || limit > surveyapp.CustomerAnswerScanLimit+1 {
		return nil, unavailable(err)
	}
	rows, err := q.ListRecentCustomerAnswerCandidates(ctx, limit)
	if err != nil {
		return nil, unavailable(err)
	}
	result := make([]surveyapp.CustomerAnswerCandidate, len(rows))
	for index, row := range rows {
		mapped, mapErr := mapSubmission(row.ID, row.QuestionnaireID, "", "", row.Unionid,
			row.ExternalUserid, "", "", "", row.Mobile, "", "", "", row.TotalScore,
			[]byte("[]"), "", "", row.SubmittedAt, row.SubmittedAt, row.Answers)
		err = mapErr
		if err != nil {
			return nil, err
		}
		result[index] = surveyapp.CustomerAnswerCandidate{
			ID: mapped.ID, QuestionnaireID: mapped.QuestionnaireID, UnionID: mapped.UnionID,
			ExternalUserID: mapped.ExternalUserID, Mobile: mapped.Mobile, TotalScore: mapped.TotalScore,
			SubmittedAt: mapped.SubmittedAt, Answers: mapped.Answers,
		}
	}
	return result, nil
}

func (r *SubmissionRepository) ExportSubmissions(ctx context.Context, id surveyport.ID, limit int32) ([]surveyport.Submission, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || id < 1 || limit < 1 {
		return nil, unavailable(err)
	}
	rows, err := q.ListQuestionnaireSubmissionExportRows(ctx, surveydb.ListQuestionnaireSubmissionExportRowsParams{
		QuestionnaireID: int64(id), RowLimit: limit,
	})
	if err != nil {
		return nil, unavailable(err)
	}
	result := make([]surveyport.Submission, len(rows))
	for i, row := range rows {
		result[i], err = mapSubmission(row.ID, row.QuestionnaireID, row.RespondentKey, row.Openid, row.Unionid,
			row.ExternalUserid, row.CustomerName, row.FollowUserUserid, row.MatchedBy, row.Mobile,
			row.SourceChannel, row.CampaignID, row.StaffID, row.TotalScore, row.FinalTags,
			row.ResultToken, row.RedirectUrlSnapshot, row.SubmittedAt, row.CreatedAt, row.Answers)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func mapSubmission(id, questionnaireID int64, respondentKey, openid, unionid, externalUserid, customerName,
	followUserUserid, matchedBy, mobile, sourceChannel, campaignID, staffID string, totalScore float64,
	rawTags []byte, resultToken, redirectURLSnapshot string, submittedAt, createdAt pgtype.Timestamptz, rawAnswers any,
) (surveyport.Submission, error) {
	if !submittedAt.Valid || !createdAt.Valid {
		return surveyport.Submission{}, surveyapp.ErrUnavailable
	}
	tagsJSON, err := jsonBytes(rawTags)
	if err != nil {
		return surveyport.Submission{}, surveyapp.ErrUnavailable
	}
	var tags []string
	if json.Unmarshal(tagsJSON, &tags) != nil {
		return surveyport.Submission{}, surveyapp.ErrUnavailable
	}
	for _, tag := range tags {
		if tag == "" {
			return surveyport.Submission{}, surveyapp.ErrUnavailable
		}
	}
	answersJSON, err := jsonBytes(rawAnswers)
	if err != nil {
		return surveyport.Submission{}, surveyapp.ErrUnavailable
	}
	var answers []struct {
		QuestionID      int64  `json:"question_id"`
		QuestionType    string `json:"question_type"`
		QuestionTitle   string `json:"question_title"`
		SortOrder       int    `json:"sort_order"`
		SelectedOptions []struct {
			OptionID   int64  `json:"option_id"`
			OptionText string `json:"option_text"`
		} `json:"selected_options"`
		TextValue string `json:"text_value"`
	}
	if json.Unmarshal(answersJSON, &answers) != nil {
		return surveyport.Submission{}, surveyapp.ErrUnavailable
	}
	mappedAnswers := make([]surveyport.SubmissionAnswer, len(answers))
	for i, answer := range answers {
		options := make([]surveyport.SubmissionAnswerOption, len(answer.SelectedOptions))
		for j, option := range answer.SelectedOptions {
			if option.OptionID < 1 {
				return surveyport.Submission{}, surveyapp.ErrUnavailable
			}
			options[j] = surveyport.SubmissionAnswerOption{OptionID: option.OptionID, OptionText: option.OptionText}
		}
		mappedAnswers[i] = surveyport.SubmissionAnswer{
			QuestionID: answer.QuestionID, QuestionType: surveyport.QuestionType(answer.QuestionType),
			QuestionTitle: answer.QuestionTitle, SortOrder: answer.SortOrder,
			SelectedOptions: options, TextValue: answer.TextValue,
		}
	}
	return surveyport.Submission{
		ID: id, QuestionnaireID: surveyport.ID(questionnaireID),
		RespondentKey: respondentKey, OpenID: openid, UnionID: unionid, ExternalUserID: externalUserid,
		CustomerName: customerName, FollowUserUserID: followUserUserid, MatchedBy: matchedBy, Mobile: mobile,
		SourceChannel: sourceChannel, CampaignID: campaignID, StaffID: staffID,
		TotalScore: totalScore, FinalTags: tags, ResultToken: resultToken,
		RedirectURLSnapshot: redirectURLSnapshot, SubmittedAt: submittedAt.Time, CreatedAt: createdAt.Time,
		Answers: mappedAnswers,
	}, nil
}

func jsonBytes(raw any) ([]byte, error) {
	switch value := raw.(type) {
	case nil:
		return []byte("[]"), nil
	case []byte:
		return append([]byte{}, value...), nil
	case string:
		return []byte(value), nil
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return encoded, nil
	}
}

func submissionLatestTime(raw any) (time.Time, error) {
	switch value := raw.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return value, nil
	case pgtype.Timestamptz:
		if !value.Valid {
			return time.Time{}, nil
		}
		return value.Time, nil
	default:
		return time.Time{}, errors.New("unexpected latest_submitted_at type")
	}
}
