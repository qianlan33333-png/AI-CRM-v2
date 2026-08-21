package app

import (
	"context"
	"errors"
	"sort"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	SafeAnalysisDefaultQuestionLimit  int32 = 50
	SafeAnalysisMaximumQuestionLimit  int32 = 100
	SafeAnalysisMaximumQuestionOffset int32 = 1_000_000
	SafeAnalysisScanLimit             int64 = 10_000
	SafeAnalysisChunkLimit            int32 = 100

	SafeExportPreviewDefaultLimit  int32 = 3
	SafeExportPreviewMaximumLimit  int32 = 3
	SafeExportPreviewMaximumOffset int32 = 1_000_000
)

var safeExportPreviewDefaultFields = []surveyport.SafeExportPreviewField{
	surveyport.SafePreviewRowNumber,
	surveyport.SafePreviewSubmittedAt,
	surveyport.SafePreviewScore,
	surveyport.SafePreviewChoiceOptionIDs,
}

// SafeAdminService creates de-identified Survey projections from the existing
// immutable submission snapshots. It reuses the already closed SubmissionStore
// and never resolves identity values or calls another domain/provider.
type SafeAdminService struct {
	uow   platformport.UnitOfWork
	store SubmissionStore
}

func NewSafeAdminService(uow platformport.UnitOfWork, store SubmissionStore) *SafeAdminService {
	return &SafeAdminService{uow: uow, store: store}
}

func (s *SafeAdminService) SafeAnalysis(
	ctx context.Context,
	questionnaireID surveyport.ID,
	limit, offset int32,
) (surveyport.SafeSubmissionAnalysis, error) {
	if questionnaireID < 1 || limit < 1 || limit > SafeAnalysisMaximumQuestionLimit || offset < 0 || offset > SafeAnalysisMaximumQuestionOffset {
		return surveyport.SafeSubmissionAnalysis{}, ErrInvalidSubmissionPage
	}
	if !safeAdminReady(s) || ctx == nil || ctx.Err() != nil {
		return surveyport.SafeSubmissionAnalysis{}, ErrUnavailable
	}

	var output surveyport.SafeSubmissionAnalysis
	err := s.uow.Within(ctx, func(tx context.Context) error {
		exists, err := s.store.SubmissionOwnerExists(tx, questionnaireID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}

		stats, err := s.store.Results(tx, questionnaireID)
		if err != nil {
			return err
		}
		if !validSubmissionResult(stats, questionnaireID) {
			return ErrUnavailable
		}

		groups := make(map[int64]*safeQuestionAccumulator)
		scanTarget := stats.SubmissionCount
		if scanTarget > SafeAnalysisScanLimit {
			scanTarget = SafeAnalysisScanLimit
		}
		seenSubmissionIDs := make(map[int64]struct{}, int(scanTarget))
		var firstSubmittedAt time.Time
		var previous *surveyport.Submission
		var scanned int64
		for scanned < scanTarget {
			chunk := int64(SafeAnalysisChunkLimit)
			if remaining := scanTarget - scanned; remaining < chunk {
				chunk = remaining
			}
			items, readErr := s.store.ListSubmissions(tx, questionnaireID, int32(chunk), int32(scanned))
			if readErr != nil {
				return readErr
			}
			if len(items) > int(chunk) {
				return ErrUnavailable
			}
			if len(items) == 0 {
				break
			}
			for itemIndex := range items {
				item := items[itemIndex]
				if !validSubmission(item, questionnaireID) {
					return ErrUnavailable
				}
				if previous != nil && compareSubmission(*previous, item) > 0 {
					return ErrUnavailable
				}
				if _, duplicate := seenSubmissionIDs[item.ID]; duplicate {
					return ErrUnavailable
				}
				seenSubmissionIDs[item.ID] = struct{}{}
				if scanned == 0 && itemIndex == 0 {
					firstSubmittedAt = item.SubmittedAt
				}
				if err := accumulateSafeChoices(groups, item); err != nil {
					return err
				}
				copyItem := item
				previous = &copyItem
			}
			scanned += int64(len(items))
			if len(items) < int(chunk) {
				break
			}
		}
		if scanned != scanTarget {
			// Owner, count and rows are read inside one UoW. A premature end is
			// therefore an inconsistent read, not a successful partial result.
			return ErrUnavailable
		}
		if scanned > 0 && !firstSubmittedAt.Equal(stats.LatestSubmittedAt) {
			return ErrUnavailable
		}

		questions, err := freezeSafeQuestionAggregates(groups)
		if err != nil {
			return err
		}
		totalQuestions := int32(len(questions))
		page := []surveyport.SafeEnumQuestionAggregate{}
		if offset < totalQuestions {
			end := int64(offset) + int64(limit)
			if end > int64(totalQuestions) {
				end = int64(totalQuestions)
			}
			page = append(page, questions[offset:int32(end)]...)
		}

		var latest *time.Time
		if !stats.LatestSubmittedAt.IsZero() {
			value := stats.LatestSubmittedAt.UTC()
			latest = &value
		}
		output = surveyport.SafeSubmissionAnalysis{
			OK:                       true,
			QuestionnaireID:          questionnaireID,
			Stats:                    surveyport.SafeSubmissionStats{SubmissionCount: stats.SubmissionCount, LatestSubmittedAt: latest, AverageScore: stats.AverageScore},
			Questions:                page,
			TotalQuestions:           totalQuestions,
			Limit:                    limit,
			Offset:                   offset,
			ScannedSubmissionCount:   scanned,
			AggregationComplete:      scanned == stats.SubmissionCount,
			Deidentified:             true,
			ContainsRawIdentity:      false,
			ContainsFreeText:         false,
			LocalOnly:                true,
			RealExternalCallExecuted: false,
		}
		return nil
	})
	if err != nil {
		return surveyport.SafeSubmissionAnalysis{}, classifySafeAdmin(err)
	}
	return cloneSafeAnalysis(output), nil
}

func (s *SafeAdminService) SafeExportPreview(
	ctx context.Context,
	questionnaireID surveyport.ID,
	request surveyport.SafeExportPreviewRequest,
) (surveyport.SafeExportPreview, error) {
	request, err := normalizeSafePreviewRequest(request)
	if err != nil || questionnaireID < 1 {
		return surveyport.SafeExportPreview{}, ErrInvalidSubmissionPage
	}
	if !safeAdminReady(s) || ctx == nil || ctx.Err() != nil {
		return surveyport.SafeExportPreview{}, ErrUnavailable
	}

	var output surveyport.SafeExportPreview
	err = s.uow.Within(ctx, func(tx context.Context) error {
		exists, readErr := s.store.SubmissionOwnerExists(tx, questionnaireID)
		if readErr != nil {
			return readErr
		}
		if !exists {
			return ErrNotFound
		}
		total, readErr := s.store.CountSubmissions(tx, questionnaireID)
		if readErr != nil {
			return readErr
		}
		if total < 0 {
			return ErrUnavailable
		}
		if int64(request.Offset) > total {
			return ErrUnavailable
		}
		items, readErr := s.store.ListSubmissions(tx, questionnaireID, request.Limit, request.Offset)
		if readErr != nil {
			return readErr
		}
		expectedRows := int64(request.Limit)
		if remaining := total - int64(request.Offset); remaining < expectedRows {
			expectedRows = remaining
		}
		if int64(len(items)) != expectedRows {
			return ErrUnavailable
		}
		rows := make([]surveyport.SafeExportPreviewRow, len(items))
		seenSubmissionIDs := make(map[int64]struct{}, len(items))
		for index := range items {
			if !validSubmission(items[index], questionnaireID) || index > 0 && compareSubmission(items[index-1], items[index]) > 0 {
				return ErrUnavailable
			}
			if _, duplicate := seenSubmissionIDs[items[index].ID]; duplicate {
				return ErrUnavailable
			}
			seenSubmissionIDs[items[index].ID] = struct{}{}
			row, projectErr := safePreviewRow(items[index], int64(request.Offset)+int64(index)+1, request.Fields)
			if projectErr != nil {
				return projectErr
			}
			rows[index] = row
		}
		output = surveyport.SafeExportPreview{
			OK:                       true,
			QuestionnaireID:          questionnaireID,
			Fields:                   append([]surveyport.SafeExportPreviewField(nil), request.Fields...),
			Rows:                     rows,
			Total:                    total,
			Limit:                    request.Limit,
			Offset:                   request.Offset,
			HasMore:                  int64(request.Offset)+int64(len(rows)) < total,
			FileCreated:              false,
			Deidentified:             true,
			ContainsRawIdentity:      false,
			ContainsFreeText:         false,
			LocalOnly:                true,
			RealExternalCallExecuted: false,
		}
		return nil
	})
	if err != nil {
		return surveyport.SafeExportPreview{}, classifySafeAdmin(err)
	}
	return cloneSafePreview(output), nil
}

type safeQuestionAccumulator struct {
	questionID    int64
	questionType  surveyport.QuestionType
	sortOrder     int
	answeredCount int64
	options       map[int64]int64
}

func accumulateSafeChoices(groups map[int64]*safeQuestionAccumulator, submission surveyport.Submission) error {
	seenAnswers := make(map[int64]struct{}, len(submission.Answers))
	for _, answer := range submission.Answers {
		if answer.QuestionID < 1 || answer.SortOrder < 0 {
			return ErrUnavailable
		}
		if _, duplicate := seenAnswers[answer.QuestionID]; duplicate {
			return ErrUnavailable
		}
		seenAnswers[answer.QuestionID] = struct{}{}
		switch answer.QuestionType {
		case surveyport.Textarea, surveyport.Mobile:
			if len(answer.SelectedOptions) != 0 {
				return ErrUnavailable
			}
			// Free-text and mobile answers are intentionally discarded before a
			// response DTO is constructed.
			continue
		case surveyport.SingleChoice:
			if len(answer.SelectedOptions) > 1 {
				return ErrUnavailable
			}
		case surveyport.MultiChoice:
		default:
			return ErrUnavailable
		}

		group := groups[answer.QuestionID]
		if group == nil {
			group = &safeQuestionAccumulator{questionID: answer.QuestionID, questionType: answer.QuestionType, sortOrder: answer.SortOrder, options: map[int64]int64{}}
			groups[answer.QuestionID] = group
		} else if group.questionType != answer.QuestionType || group.sortOrder != answer.SortOrder {
			return ErrUnavailable
		}

		seenOptions := make(map[int64]struct{}, len(answer.SelectedOptions))
		for _, option := range answer.SelectedOptions {
			if option.OptionID < 1 {
				return ErrUnavailable
			}
			if _, duplicate := seenOptions[option.OptionID]; duplicate {
				return ErrUnavailable
			}
			seenOptions[option.OptionID] = struct{}{}
			group.options[option.OptionID]++
		}
		if len(answer.SelectedOptions) > 0 {
			group.answeredCount++
		}
	}
	return nil
}

func freezeSafeQuestionAggregates(groups map[int64]*safeQuestionAccumulator) ([]surveyport.SafeEnumQuestionAggregate, error) {
	result := make([]surveyport.SafeEnumQuestionAggregate, 0, len(groups))
	for _, group := range groups {
		if group == nil || group.questionID < 1 || group.answeredCount < 0 {
			return nil, ErrUnavailable
		}
		optionIDs := make([]int64, 0, len(group.options))
		for optionID, count := range group.options {
			if optionID < 1 || count < 0 {
				return nil, ErrUnavailable
			}
			optionIDs = append(optionIDs, optionID)
		}
		sort.Slice(optionIDs, func(i, j int) bool { return optionIDs[i] < optionIDs[j] })
		options := make([]surveyport.SafeEnumOptionAggregate, len(optionIDs))
		for index, optionID := range optionIDs {
			options[index] = surveyport.SafeEnumOptionAggregate{OptionID: optionID, SelectionCount: group.options[optionID]}
		}
		result = append(result, surveyport.SafeEnumQuestionAggregate{
			QuestionID: group.questionID, QuestionType: group.questionType, SortOrder: group.sortOrder,
			AnsweredCount: group.answeredCount, Options: options,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SortOrder != result[j].SortOrder {
			return result[i].SortOrder < result[j].SortOrder
		}
		return result[i].QuestionID < result[j].QuestionID
	})
	return result, nil
}

func normalizeSafePreviewRequest(request surveyport.SafeExportPreviewRequest) (surveyport.SafeExportPreviewRequest, error) {
	if request.Limit == 0 {
		request.Limit = SafeExportPreviewDefaultLimit
	}
	if request.Limit < 1 || request.Limit > SafeExportPreviewMaximumLimit || request.Offset < 0 || request.Offset > SafeExportPreviewMaximumOffset {
		return surveyport.SafeExportPreviewRequest{}, ErrInvalidSubmissionPage
	}
	if len(request.Fields) == 0 {
		request.Fields = append([]surveyport.SafeExportPreviewField(nil), safeExportPreviewDefaultFields...)
	}
	if len(request.Fields) > len(safeExportPreviewDefaultFields) {
		return surveyport.SafeExportPreviewRequest{}, ErrInvalidSubmissionPage
	}
	seen := make(map[surveyport.SafeExportPreviewField]struct{}, len(request.Fields))
	for _, field := range request.Fields {
		if !safePreviewFieldAllowed(field) {
			return surveyport.SafeExportPreviewRequest{}, ErrInvalidSubmissionPage
		}
		if _, duplicate := seen[field]; duplicate {
			return surveyport.SafeExportPreviewRequest{}, ErrInvalidSubmissionPage
		}
		seen[field] = struct{}{}
	}
	request.Fields = append([]surveyport.SafeExportPreviewField(nil), request.Fields...)
	return request, nil
}

func safePreviewFieldAllowed(field surveyport.SafeExportPreviewField) bool {
	switch field {
	case surveyport.SafePreviewRowNumber, surveyport.SafePreviewSubmittedAt, surveyport.SafePreviewScore, surveyport.SafePreviewChoiceOptionIDs:
		return true
	default:
		return false
	}
}

func safePreviewRow(submission surveyport.Submission, rowNumber int64, fields []surveyport.SafeExportPreviewField) (surveyport.SafeExportPreviewRow, error) {
	var row surveyport.SafeExportPreviewRow
	for _, field := range fields {
		switch field {
		case surveyport.SafePreviewRowNumber:
			value := rowNumber
			row.RowNumber = &value
		case surveyport.SafePreviewSubmittedAt:
			value := submission.SubmittedAt.UTC()
			row.SubmittedAt = &value
		case surveyport.SafePreviewScore:
			value := submission.TotalScore
			row.Score = &value
		case surveyport.SafePreviewChoiceOptionIDs:
			answers, err := safeChoiceAnswers(submission.Answers)
			if err != nil {
				return surveyport.SafeExportPreviewRow{}, err
			}
			row.ChoiceOptionIDs = &answers
		default:
			return surveyport.SafeExportPreviewRow{}, ErrInvalidSubmissionPage
		}
	}
	return row, nil
}

func safeChoiceAnswers(answers []surveyport.SubmissionAnswer) ([]surveyport.SafeChoiceAnswerPreview, error) {
	result := make([]surveyport.SafeChoiceAnswerPreview, 0, len(answers))
	seenQuestions := make(map[int64]struct{}, len(answers))
	for _, answer := range answers {
		if answer.QuestionID < 1 || answer.SortOrder < 0 {
			return nil, ErrUnavailable
		}
		if _, duplicate := seenQuestions[answer.QuestionID]; duplicate {
			return nil, ErrUnavailable
		}
		seenQuestions[answer.QuestionID] = struct{}{}
		switch answer.QuestionType {
		case surveyport.Textarea, surveyport.Mobile:
			if len(answer.SelectedOptions) != 0 {
				return nil, ErrUnavailable
			}
			continue
		case surveyport.SingleChoice:
			if len(answer.SelectedOptions) > 1 {
				return nil, ErrUnavailable
			}
		case surveyport.MultiChoice:
		default:
			return nil, ErrUnavailable
		}
		ids := make([]int64, 0, len(answer.SelectedOptions))
		seenOptions := make(map[int64]struct{}, len(answer.SelectedOptions))
		for _, option := range answer.SelectedOptions {
			if option.OptionID < 1 {
				return nil, ErrUnavailable
			}
			if _, duplicate := seenOptions[option.OptionID]; duplicate {
				return nil, ErrUnavailable
			}
			seenOptions[option.OptionID] = struct{}{}
			ids = append(ids, option.OptionID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		result = append(result, surveyport.SafeChoiceAnswerPreview{QuestionID: answer.QuestionID, QuestionType: answer.QuestionType, SortOrder: answer.SortOrder, OptionIDs: ids})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SortOrder != result[j].SortOrder {
			return result[i].SortOrder < result[j].SortOrder
		}
		return result[i].QuestionID < result[j].QuestionID
	})
	return result, nil
}

func safeAdminReady(service *SafeAdminService) bool {
	return service != nil && service.uow != nil && service.store != nil
}

func classifySafeAdmin(err error) error {
	switch {
	case errors.Is(err, ErrInvalidSubmissionPage), errors.Is(err, ErrNotFound):
		return err
	default:
		return ErrUnavailable
	}
}

func cloneSafeAnalysis(value surveyport.SafeSubmissionAnalysis) surveyport.SafeSubmissionAnalysis {
	result := value
	if value.Stats.LatestSubmittedAt != nil {
		latest := *value.Stats.LatestSubmittedAt
		result.Stats.LatestSubmittedAt = &latest
	}
	result.Questions = make([]surveyport.SafeEnumQuestionAggregate, len(value.Questions))
	for index := range value.Questions {
		result.Questions[index] = value.Questions[index]
		result.Questions[index].Options = append([]surveyport.SafeEnumOptionAggregate(nil), value.Questions[index].Options...)
	}
	return result
}

func cloneSafePreview(value surveyport.SafeExportPreview) surveyport.SafeExportPreview {
	result := value
	result.Fields = append([]surveyport.SafeExportPreviewField(nil), value.Fields...)
	result.Rows = make([]surveyport.SafeExportPreviewRow, len(value.Rows))
	for index := range value.Rows {
		result.Rows[index] = value.Rows[index]
		if value.Rows[index].RowNumber != nil {
			rowNumber := *value.Rows[index].RowNumber
			result.Rows[index].RowNumber = &rowNumber
		}
		if value.Rows[index].SubmittedAt != nil {
			submittedAt := *value.Rows[index].SubmittedAt
			result.Rows[index].SubmittedAt = &submittedAt
		}
		if value.Rows[index].Score != nil {
			score := *value.Rows[index].Score
			result.Rows[index].Score = &score
		}
		if value.Rows[index].ChoiceOptionIDs != nil {
			answers := make([]surveyport.SafeChoiceAnswerPreview, len(*value.Rows[index].ChoiceOptionIDs))
			for answerIndex := range *value.Rows[index].ChoiceOptionIDs {
				answers[answerIndex] = (*value.Rows[index].ChoiceOptionIDs)[answerIndex]
				answers[answerIndex].OptionIDs = append([]int64(nil), (*value.Rows[index].ChoiceOptionIDs)[answerIndex].OptionIDs...)
			}
			result.Rows[index].ChoiceOptionIDs = &answers
		}
	}
	return result
}
