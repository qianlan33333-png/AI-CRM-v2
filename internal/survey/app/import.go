package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

var (
	ErrInvalidImport     = errors.New("invalid survey history import")
	ErrImportUnavailable = errors.New("survey history import unavailable")
)

// ImportStore is the Survey-owned target seam. Every method requires the
// transaction-bound context supplied by ImportService; no method can open a
// second transaction or call a provider.
type ImportStore interface {
	Reserve(context.Context, string, Reservation) (Receipt, bool, error)
	Complete(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
	CreateImportedQuestionnaire(context.Context, surveyport.ImportQuestionnaire, int64) (int64, error)
	CreateImportedQuestion(context.Context, int64, surveyport.ImportQuestion, json.RawMessage) (int64, error)
	CreateImportedOption(context.Context, int64, surveyport.ImportOption) (int64, error)
	CreateImportedSubmission(context.Context, int64, surveyport.ImportSubmission) (int64, error)
	CreateImportedAnswer(context.Context, int64, int64, surveyport.ImportAnswer, json.RawMessage) (int64, error)
}

// ImportService writes one complete source questionnaire aggregate. It has no
// event appender by design: historical import must not publish a new business
// event or trigger external push, outbound, or provider work.
type ImportService struct {
	uow   platformport.UnitOfWork
	store ImportStore
	now   func() time.Time
}

func NewImportService(uow platformport.UnitOfWork, store ImportStore) *ImportService {
	return &ImportService{uow: uow, store: store, now: time.Now}
}

// ImportServiceWithClock is useful for deterministic owner-level tests. The
// production path should use NewImportService.
func NewImportServiceWithClock(uow platformport.UnitOfWork, store ImportStore, now func() time.Time) *ImportService {
	return &ImportService{uow: uow, store: store, now: now}
}

type preparedImport struct {
	aggregate surveyport.ValidatedImportAggregate
	digest    [32]byte
}

type importReceiptSnapshot struct {
	Version int                     `json:"version"`
	Result  surveyport.ImportResult `json:"result"`
}

func (s *ImportService) Import(ctx context.Context, request surveyport.ImportRequest) (surveyport.ImportResult, error) {
	if s == nil || ctx == nil || s.uow == nil || s.store == nil || s.now == nil {
		return surveyport.ImportResult{}, ErrImportUnavailable
	}
	prepared, err := prepareImport(request)
	if err != nil {
		return surveyport.ImportResult{}, err
	}
	now := s.now().UTC()
	if now.IsZero() {
		return surveyport.ImportResult{}, ErrImportUnavailable
	}
	actorScope := "migration:" + strconv.FormatInt(request.MigrationActor, 10)
	keyDigest := sha256.Sum256([]byte("survey.import/v1\x00" + request.RunID + "\x00" + request.IdempotencyKey + "\x00" + strconv.FormatInt(request.Aggregate.Questionnaire.SourceID, 10)))
	reservation := Reservation{ActorScope: actorScope, KeyDigest: keyDigest, PayloadDigest: prepared.digest, CreatedAt: now}

	var result surveyport.ImportResult
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.Reserve(tx, "create", reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !receiptMatches(receipt, "create", reservation) {
			return ErrImportUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], prepared.digest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" {
				return ErrImportUnavailable
			}
			result, err = decodeImportSnapshot(receipt.ResultSnapshot, prepared.aggregate)
			if err != nil {
				return err
			}
			result.ReceiptID = receipt.ID
			result.Replayed = true
			return nil
		}
		if receipt.State != "in_progress" || len(receipt.ResultSnapshot) != 0 {
			return ErrImportUnavailable
		}

		result = surveyport.ImportResult{ReceiptID: receipt.ID, Mapping: emptyImportIDMap()}
		targetQuestionnaire, writeErr := s.store.CreateImportedQuestionnaire(tx, prepared.aggregate.Questionnaire, request.MigrationActor)
		if writeErr != nil {
			return writeErr
		}
		result.Mapping.Questionnaires[prepared.aggregate.Questionnaire.SourceID] = targetQuestionnaire

		questionTargets := make(map[int64]int64, len(prepared.aggregate.Questions))
		for _, question := range prepared.aggregate.Questions {
			target, writeErr := s.store.CreateImportedQuestion(tx, targetQuestionnaire, question, question.Validation)
			if writeErr != nil {
				return writeErr
			}
			questionTargets[question.SourceID] = target
			result.Mapping.Questions[question.SourceID] = surveyport.ImportQuestionReference{
				TargetID: target, QuestionnaireID: targetQuestionnaire, Type: question.Type,
				Title: question.Title, SortOrder: question.SortOrder,
			}
			result.ImportedQuestions++
		}

		optionTargets := make(map[int64]int64, len(prepared.aggregate.Options))
		for _, option := range prepared.aggregate.Options {
			targetQuestion := questionTargets[option.SourceQuestionID]
			target, writeErr := s.store.CreateImportedOption(tx, targetQuestion, option)
			if writeErr != nil {
				return writeErr
			}
			optionTargets[option.SourceID] = target
			result.Mapping.Options[option.SourceID] = surveyport.ImportOptionReference{TargetID: target, QuestionID: targetQuestion}
			result.ImportedOptions++
		}

		submissionTargets := make(map[int64]int64, len(prepared.aggregate.Submissions))
		for _, submission := range prepared.aggregate.Submissions {
			target, writeErr := s.store.CreateImportedSubmission(tx, targetQuestionnaire, submission)
			if writeErr != nil {
				return writeErr
			}
			submissionTargets[submission.SourceID] = target
			result.Mapping.Submissions[submission.SourceID] = surveyport.ImportSubmissionReference{TargetID: target, QuestionnaireID: targetQuestionnaire}
			result.ImportedSubmissions++
		}

		for _, answer := range prepared.aggregate.Answers {
			targetSubmission := submissionTargets[answer.SourceSubmissionID]
			targetQuestion := questionTargets[answer.SourceQuestionID]
			selectedOptions, encodeErr := resolveImportAnswerOptions(answer, optionTargets)
			if encodeErr != nil {
				return encodeErr
			}
			target, writeErr := s.store.CreateImportedAnswer(tx, targetSubmission, targetQuestion, answer, selectedOptions)
			if writeErr != nil {
				return writeErr
			}
			result.Mapping.Answers[answer.SourceID] = target
			result.ImportedAnswers++
		}

		// Receipt snapshots are replay payloads, not response envelopes. Keep
		// transaction-local receipt metadata out of the persisted digest so a
		// replay can validate it independently of the receipt row ID.
		snapshotResult := result
		snapshotResult.ReceiptID = 0
		snapshotResult.Replayed = false
		snapshot, marshalErr := json.Marshal(importReceiptSnapshot{Version: 1, Result: snapshotResult})
		if marshalErr != nil {
			return ErrImportUnavailable
		}
		completed, completeErr := s.store.Complete(tx, receipt.ID, snapshot, now)
		if completeErr != nil || !receiptMatches(completed, "create", reservation) || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrImportUnavailable
		}
		return nil
	})
	if err != nil {
		return surveyport.ImportResult{}, err
	}
	return result, nil
}

func prepareImport(request surveyport.ImportRequest) (preparedImport, error) {
	if request.MigrationActor < 1 || !validImportKey(request.RunID, 128) || !validImportKey(request.IdempotencyKey, 128) {
		return preparedImport{}, ErrInvalidImport
	}
	aggregate := cloneImportAggregate(request.Aggregate)
	if err := validateImportAggregate(&aggregate, request.MigrationActor); err != nil {
		return preparedImport{}, err
	}
	payload, err := json.Marshal(struct {
		Actor     int64                               `json:"actor"`
		RunID     string                              `json:"run_id"`
		Aggregate surveyport.ValidatedImportAggregate `json:"aggregate"`
	}{request.MigrationActor, request.RunID, aggregate})
	if err != nil {
		return preparedImport{}, ErrInvalidImport
	}
	return preparedImport{aggregate: aggregate, digest: sha256.Sum256(payload)}, nil
}

func validateImportAggregate(aggregate *surveyport.ValidatedImportAggregate, actor int64) error {
	if aggregate == nil || aggregate.Questionnaire.SourceID < 1 || actor < 1 {
		return ErrInvalidImport
	}
	questionnaire := &aggregate.Questionnaire
	if questionnaire.Version != 1 || questionnaire.SubmissionCount != 0 || questionnaire.AssessmentEnabled || !emptyObject(questionnaire.AssessmentConfig) ||
		!validImportText(questionnaire.Slug, 200, false) || !validImportText(questionnaire.Name, 120, true) || !validImportText(questionnaire.Title, 300, true) ||
		!validImportText(questionnaire.Description, 10000, false) || (questionnaire.AnswerDisplayMode != surveyport.AllInOne && questionnaire.AnswerDisplayMode != surveyport.OneByOne) ||
		!validImportTimeRange(questionnaire.CreatedAt, questionnaire.UpdatedAt) {
		return ErrInvalidImport
	}

	questionBySource := make(map[int64]surveyport.ImportQuestion, len(aggregate.Questions))
	optionsByQuestion := make(map[int64][]surveyport.ImportOption, len(aggregate.Questions))
	for _, question := range aggregate.Questions {
		if question.SourceID < 1 || question.SourceQuestionnaireID != questionnaire.SourceID || questionBySource[question.SourceID].SourceID != 0 ||
			!validImportQuestionBase(question) || !validImportTimeRange(question.CreatedAt, question.UpdatedAt) {
			return ErrInvalidImport
		}
		questionBySource[question.SourceID] = question
	}
	for _, option := range aggregate.Options {
		if option.SourceID < 1 || option.SourceQuestionID < 1 || questionBySource[option.SourceQuestionID].SourceID == 0 ||
			!validImportOptionBase(option) || !validImportTimeRange(option.CreatedAt, option.UpdatedAt) {
			return ErrInvalidImport
		}
		for _, existing := range optionsByQuestion[option.SourceQuestionID] {
			if existing.SourceID == option.SourceID {
				return ErrInvalidImport
			}
		}
		optionsByQuestion[option.SourceQuestionID] = append(optionsByQuestion[option.SourceQuestionID], option)
	}

	orderedQuestions := append([]surveyport.ImportQuestion(nil), aggregate.Questions...)
	sort.SliceStable(orderedQuestions, func(i, j int) bool {
		if orderedQuestions[i].SortOrder != orderedQuestions[j].SortOrder {
			return orderedQuestions[i].SortOrder < orderedQuestions[j].SortOrder
		}
		return orderedQuestions[i].SourceID < orderedQuestions[j].SourceID
	})
	domainQuestions := make([]surveyport.Question, len(orderedQuestions))
	for index, question := range orderedQuestions {
		options := optionsByQuestion[question.SourceID]
		sort.SliceStable(options, func(i, j int) bool {
			if options[i].SortOrder != options[j].SortOrder {
				return options[i].SortOrder < options[j].SortOrder
			}
			return options[i].SourceID < options[j].SourceID
		})
		domainQuestion := surveyport.Question{Type: question.Type, Title: question.Title, Required: question.Required, SortOrder: question.SortOrder,
			PlaceholderText: question.PlaceholderText, AssessmentDimensionKey: question.AssessmentDimensionKey, SidebarProfileField: question.SidebarProfileField}
		if len(question.Validation) > 0 && !json.Valid(question.Validation) {
			return ErrInvalidImport
		}
		if len(question.Validation) > 0 && json.Unmarshal(question.Validation, &domainQuestion.Validation) != nil {
			return ErrInvalidImport
		}
		for _, option := range options {
			tags, err := importTags(option.TagCodes)
			if err != nil {
				return err
			}
			domainQuestion.Options = append(domainQuestion.Options, surveyport.Option{OptionText: option.OptionText, Score: option.Score,
				AssessmentTypeKey: option.AssessmentTypeKey, TagCodes: tags, IsOther: option.IsOther,
				OtherPlaceholder: option.OtherPlaceholder, OtherMaximumLength: option.OtherMaxLength, SortOrder: option.SortOrder})
		}
		domainQuestions[index] = domainQuestion
	}
	domainQuestionnaire := surveyport.Questionnaire{Name: questionnaire.Name, Title: questionnaire.Title, Description: questionnaire.Description,
		AnswerDisplayMode: questionnaire.AnswerDisplayMode, AssessmentEnabled: questionnaire.AssessmentEnabled,
		AssessmentConfig: cloneJSON(questionnaire.AssessmentConfig), Slug: questionnaire.Slug, IsDisabled: questionnaire.IsDisabled,
		Questions: domainQuestions}
	normalized, err := normalize(surveyport.CreateCommand{Questionnaire: domainQuestionnaire, Actor: actor, IdempotencyKey: "survey-import-validation-key"})
	if err != nil {
		return ErrInvalidImport
	}
	for index := range orderedQuestions {
		question := findImportQuestion(aggregate.Questions, orderedQuestions[index].SourceID)
		if question == nil {
			return ErrInvalidImport
		}
		question.Validation, _ = json.Marshal(normalized.Questions[index].Validation)
		question.Title = normalized.Questions[index].Title
		question.Type = normalized.Questions[index].Type
		question.Required = normalized.Questions[index].Required
		question.SortOrder = normalized.Questions[index].SortOrder
		question.PlaceholderText = normalized.Questions[index].PlaceholderText
		question.AssessmentDimensionKey = normalized.Questions[index].AssessmentDimensionKey
		question.SidebarProfileField = normalized.Questions[index].SidebarProfileField
		for optionIndex := range normalized.Questions[index].Options {
			option := findImportOption(aggregate.Options, orderedQuestions[index].SourceID, normalized.Questions[index].Options[optionIndex].SortOrder)
			if option == nil {
				return ErrInvalidImport
			}
			option.OptionText = normalized.Questions[index].Options[optionIndex].OptionText
			option.Score = normalized.Questions[index].Options[optionIndex].Score
			option.AssessmentTypeKey = normalized.Questions[index].Options[optionIndex].AssessmentTypeKey
			option.TagCodes, _ = json.Marshal(normalized.Questions[index].Options[optionIndex].TagCodes)
			option.IsOther = normalized.Questions[index].Options[optionIndex].IsOther
			option.OtherPlaceholder = normalized.Questions[index].Options[optionIndex].OtherPlaceholder
			option.OtherMaxLength = normalized.Questions[index].Options[optionIndex].OtherMaximumLength
			option.SortOrder = normalized.Questions[index].Options[optionIndex].SortOrder
		}
	}
	aggregate.Questionnaire.AssessmentConfig = cloneJSON(normalized.AssessmentConfig)
	aggregate.Questionnaire.AssessmentEnabled = normalized.AssessmentEnabled
	aggregate.Questionnaire.Name, aggregate.Questionnaire.Title, aggregate.Questionnaire.Description = normalized.Name, normalized.Title, normalized.Description
	aggregate.Questionnaire.Slug, aggregate.Questionnaire.AnswerDisplayMode = normalized.Slug, normalized.AnswerDisplayMode

	submissionBySource := make(map[int64]surveyport.ImportSubmission, len(aggregate.Submissions))
	for _, submission := range aggregate.Submissions {
		if submission.SourceID < 1 || submission.SourceQuestionnaireID != questionnaire.SourceID || submissionBySource[submission.SourceID].SourceID != 0 ||
			!validImportSubmission(submission) {
			return ErrInvalidImport
		}
		submissionBySource[submission.SourceID] = submission
	}
	questionBySource = make(map[int64]surveyport.ImportQuestion, len(aggregate.Questions))
	for _, question := range aggregate.Questions {
		questionBySource[question.SourceID] = question
	}
	answerIDs := make(map[int64]struct{}, len(aggregate.Answers))
	for _, answer := range aggregate.Answers {
		if answer.SourceID < 1 || answer.SourceSubmissionID < 1 || answer.SourceQuestionID < 1 {
			return ErrInvalidImport
		}
		if _, exists := answerIDs[answer.SourceID]; exists {
			return ErrInvalidImport
		}
		answerIDs[answer.SourceID] = struct{}{}
		submission, submissionOK := submissionBySource[answer.SourceSubmissionID]
		question, questionOK := questionBySource[answer.SourceQuestionID]
		if !submissionOK || !questionOK || question.Type != answer.QuestionType || question.SortOrder != answer.SortOrder ||
			submission.SourceQuestionnaireID != questionnaire.SourceID || !validImportAnswer(answer) {
			return ErrInvalidImport
		}
		seenOptions := make(map[int64]struct{}, len(answer.SelectedOptions))
		for _, selected := range answer.SelectedOptions {
			option, optionOK := findImportOptionByID(aggregate.Options, selected.SourceOptionID)
			if !optionOK || option.SourceQuestionID != answer.SourceQuestionID || !validImportText(selected.OptionText, 500, true) {
				return ErrInvalidImport
			}
			if _, exists := seenOptions[selected.SourceOptionID]; exists {
				return ErrInvalidImport
			}
			seenOptions[selected.SourceOptionID] = struct{}{}
		}
	}
	sort.SliceStable(aggregate.Questions, func(i, j int) bool { return aggregate.Questions[i].SortOrder < aggregate.Questions[j].SortOrder })
	sort.SliceStable(aggregate.Options, func(i, j int) bool {
		if aggregate.Options[i].SourceQuestionID != aggregate.Options[j].SourceQuestionID {
			return aggregate.Options[i].SourceQuestionID < aggregate.Options[j].SourceQuestionID
		}
		return aggregate.Options[i].SortOrder < aggregate.Options[j].SortOrder
	})
	sort.SliceStable(aggregate.Submissions, func(i, j int) bool { return aggregate.Submissions[i].SourceID < aggregate.Submissions[j].SourceID })
	sort.SliceStable(aggregate.Answers, func(i, j int) bool {
		if aggregate.Answers[i].SourceSubmissionID != aggregate.Answers[j].SourceSubmissionID {
			return aggregate.Answers[i].SourceSubmissionID < aggregate.Answers[j].SourceSubmissionID
		}
		if aggregate.Answers[i].SortOrder != aggregate.Answers[j].SortOrder {
			return aggregate.Answers[i].SortOrder < aggregate.Answers[j].SortOrder
		}
		return aggregate.Answers[i].SourceID < aggregate.Answers[j].SourceID
	})
	return nil
}

func resolveImportAnswerOptions(answer surveyport.ImportAnswer, targets map[int64]int64) (json.RawMessage, error) {
	resolved := make([]struct {
		OptionID   int64  `json:"option_id"`
		OptionText string `json:"option_text"`
	}, len(answer.SelectedOptions))
	for index, selected := range answer.SelectedOptions {
		target, ok := targets[selected.SourceOptionID]
		if !ok || target < 1 {
			return nil, ErrInvalidImport
		}
		resolved[index].OptionID, resolved[index].OptionText = target, selected.OptionText
	}
	value, err := json.Marshal(resolved)
	if err != nil {
		return nil, ErrInvalidImport
	}
	return value, nil
}

func decodeImportSnapshot(raw json.RawMessage, aggregate surveyport.ValidatedImportAggregate) (surveyport.ImportResult, error) {
	var snapshot importReceiptSnapshot
	if !json.Valid(raw) || json.Unmarshal(raw, &snapshot) != nil || snapshot.Version != 1 || snapshot.Result.ReceiptID != 0 || snapshot.Result.Replayed {
		return surveyport.ImportResult{}, ErrImportUnavailable
	}
	if !validImportResult(snapshot.Result, aggregate) {
		return surveyport.ImportResult{}, ErrImportUnavailable
	}
	return snapshot.Result, nil
}

func validImportResult(result surveyport.ImportResult, aggregate surveyport.ValidatedImportAggregate) bool {
	if result.Mapping.Questionnaires == nil || result.Mapping.Questions == nil || result.Mapping.Options == nil || result.Mapping.Submissions == nil || result.Mapping.Answers == nil {
		return false
	}
	targetQuestionnaire := result.Mapping.Questionnaires[aggregate.Questionnaire.SourceID]
	if targetQuestionnaire < 1 || len(result.Mapping.Questionnaires) != 1 || result.ImportedQuestions != len(aggregate.Questions) ||
		result.ImportedOptions != len(aggregate.Options) || result.ImportedSubmissions != len(aggregate.Submissions) || result.ImportedAnswers != len(aggregate.Answers) {
		return false
	}
	for _, question := range aggregate.Questions {
		ref, ok := result.Mapping.Questions[question.SourceID]
		if !ok || ref.TargetID < 1 || ref.QuestionnaireID != targetQuestionnaire || ref.Type != question.Type || ref.SortOrder != question.SortOrder {
			return false
		}
	}
	for _, option := range aggregate.Options {
		ref, ok := result.Mapping.Options[option.SourceID]
		question, questionOK := result.Mapping.Questions[option.SourceQuestionID]
		if !ok || ref.TargetID < 1 || !questionOK || ref.QuestionID != question.TargetID {
			return false
		}
	}
	for _, submission := range aggregate.Submissions {
		ref, ok := result.Mapping.Submissions[submission.SourceID]
		if !ok || ref.TargetID < 1 || ref.QuestionnaireID != targetQuestionnaire {
			return false
		}
	}
	for _, answer := range aggregate.Answers {
		if target, ok := result.Mapping.Answers[answer.SourceID]; !ok || target < 1 {
			return false
		}
	}
	return len(result.Mapping.Questions) == len(aggregate.Questions) && len(result.Mapping.Options) == len(aggregate.Options) &&
		len(result.Mapping.Submissions) == len(aggregate.Submissions) && len(result.Mapping.Answers) == len(aggregate.Answers)
}

func emptyImportIDMap() surveyport.ImportIDMap {
	return surveyport.ImportIDMap{
		Questionnaires: map[int64]int64{}, Questions: map[int64]surveyport.ImportQuestionReference{},
		Options: map[int64]surveyport.ImportOptionReference{}, Submissions: map[int64]surveyport.ImportSubmissionReference{}, Answers: map[int64]int64{},
	}
}

func cloneImportAggregate(source surveyport.ValidatedImportAggregate) surveyport.ValidatedImportAggregate {
	result := source
	result.Questionnaire.AssessmentConfig = cloneJSON(source.Questionnaire.AssessmentConfig)
	result.Questions = append([]surveyport.ImportQuestion(nil), source.Questions...)
	result.Options = append([]surveyport.ImportOption(nil), source.Options...)
	result.Submissions = append([]surveyport.ImportSubmission(nil), source.Submissions...)
	result.Answers = make([]surveyport.ImportAnswer, len(source.Answers))
	for index, answer := range source.Answers {
		result.Answers[index] = answer
		result.Answers[index].SelectedOptions = append([]surveyport.ImportAnswerOption(nil), answer.SelectedOptions...)
	}
	for index := range result.Questions {
		result.Questions[index].Validation = cloneJSON(source.Questions[index].Validation)
	}
	for index := range result.Options {
		result.Options[index].TagCodes = cloneJSON(source.Options[index].TagCodes)
	}
	for index := range result.Submissions {
		result.Submissions[index].FinalTags = cloneJSON(source.Submissions[index].FinalTags)
	}
	return result
}

func findImportQuestion(items []surveyport.ImportQuestion, sourceID int64) *surveyport.ImportQuestion {
	for index := range items {
		if items[index].SourceID == sourceID {
			return &items[index]
		}
	}
	return nil
}

func findImportOption(items []surveyport.ImportOption, questionID int64, sortOrder int) *surveyport.ImportOption {
	for index := range items {
		if items[index].SourceQuestionID == questionID && items[index].SortOrder == sortOrder {
			return &items[index]
		}
	}
	return nil
}

func findImportOptionByID(items []surveyport.ImportOption, sourceID int64) (surveyport.ImportOption, bool) {
	for _, option := range items {
		if option.SourceID == sourceID {
			return option, true
		}
	}
	return surveyport.ImportOption{}, false
}

func validImportQuestionBase(question surveyport.ImportQuestion) bool {
	return (question.Type == surveyport.SingleChoice || question.Type == surveyport.MultiChoice || question.Type == surveyport.Textarea || question.Type == surveyport.Mobile) &&
		validImportText(question.Title, 500, true) && validImportText(question.PlaceholderText, 500, false) &&
		validImportText(question.AssessmentDimensionKey, 200, false) && validImportText(question.SidebarProfileField, 200, false) && question.SortOrder >= 0
}

func validImportOptionBase(option surveyport.ImportOption) bool {
	if !validImportText(option.OptionText, 500, true) || !validImportText(option.AssessmentTypeKey, 200, false) || option.SortOrder < 0 ||
		math.IsNaN(option.Score) || math.IsInf(option.Score, 0) || !validImportText(option.OtherPlaceholder, 500, false) || option.OtherMaxLength < 0 {
		return false
	}
	if option.IsOther {
		return option.OtherMaxLength >= 1 && option.OtherMaxLength <= 2000
	}
	return option.OtherPlaceholder == "" && option.OtherMaxLength == 0
}

func validImportSubmission(submission surveyport.ImportSubmission) bool {
	return validImportText(submission.UnionID, 200, false) && validImportText(submission.FollowUserUserID, 200, false) &&
		validImportText(submission.MatchedBy, 50, false) && validImportText(submission.Mobile, 32, false) &&
		validImportText(submission.SourceChannel, 100, false) && validImportText(submission.CampaignID, 200, false) &&
		validImportText(submission.StaffID, 200, false) && validImportText(submission.ResultToken, 200, false) &&
		utf8.ValidString(submission.RedirectURLSnapshot) && utf8.RuneCountInString(submission.RedirectURLSnapshot) <= 2000 &&
		!math.IsNaN(submission.TotalScore) && !math.IsInf(submission.TotalScore, 0) && validImportTags(submission.FinalTags) &&
		!submission.SubmittedAt.IsZero() && !submission.CreatedAt.IsZero()
}

func validImportAnswer(answer surveyport.ImportAnswer) bool {
	return (answer.QuestionType == surveyport.SingleChoice || answer.QuestionType == surveyport.MultiChoice || answer.QuestionType == surveyport.Textarea || answer.QuestionType == surveyport.Mobile) &&
		validImportText(answer.QuestionTitle, 500, true) && answer.SortOrder >= 0 && validImportText(answer.TextValue, 10000, false) && !answer.CreatedAt.IsZero()
}

func importTags(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var tags []string
	if !json.Valid(raw) || json.Unmarshal(raw, &tags) != nil {
		return nil, ErrInvalidImport
	}
	seen := make(map[string]struct{}, len(tags))
	for index := range tags {
		tags[index] = strings.TrimSpace(tags[index])
		if !validImportText(tags[index], 200, true) {
			return nil, ErrInvalidImport
		}
		if _, exists := seen[tags[index]]; exists {
			return nil, ErrInvalidImport
		}
		seen[tags[index]] = struct{}{}
	}
	return tags, nil
}

func validImportTags(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var tags []string
	if json.Unmarshal(raw, &tags) != nil || tags == nil {
		return false
	}
	for _, tag := range tags {
		if !validImportText(tag, 200, true) {
			return false
		}
	}
	return true
}

func emptyObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value != nil && len(value) == 0
}

func validImportText(value string, maximum int, required bool) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) == value && utf8.RuneCountInString(value) <= maximum && (!required || value != "")
}

func validImportTimeRange(createdAt, updatedAt time.Time) bool {
	return !createdAt.IsZero() && !updatedAt.IsZero() && !updatedAt.Before(createdAt)
}

func validImportKey(value string, maximum int) bool {
	return len(value) >= 16 && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value)
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}
