package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sort"
	"strconv"

	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

var (
	ErrPublicUnavailable  = errors.New("public questionnaire unavailable")
	ErrInvalidPublicInput = errors.New("invalid public questionnaire input")
)

// PublicDefinition projects only a fully published choice-only questionnaire.
// It is also the service-layer guard used before a snapshot is persisted.
func PublicDefinition(source surveyport.Questionnaire) (surveyport.PublicQuestionnaire, error) {
	if source.ID < 1 || source.Version < 1 || source.IsDisabled || !ValidPublicSlug(source.Slug) ||
		(source.AnswerDisplayMode != surveyport.AllInOne && source.AnswerDisplayMode != surveyport.OneByOne) ||
		len(source.Questions) == 0 || source.AssessmentEnabled || len(source.ScoreRules) != 0 {
		return surveyport.PublicQuestionnaire{}, ErrInvalidPublicInput
	}
	result := surveyport.PublicQuestionnaire{ID: source.ID, Slug: source.Slug, Title: source.Title, Description: source.Description, AnswerDisplayMode: source.AnswerDisplayMode, Version: source.Version, Questions: make([]surveyport.PublicQuestion, 0, len(source.Questions))}
	for index, question := range source.Questions {
		if question.ID < 1 || question.SortOrder != index || (question.Type != surveyport.SingleChoice && question.Type != surveyport.MultiChoice) ||
			question.AssessmentDimensionKey != "" || question.SidebarProfileField != "" || question.PlaceholderText != "" || len(question.Options) == 0 ||
			question.Validation.MinimumSelections == nil || question.Validation.MaximumSelections == nil {
			return surveyport.PublicQuestionnaire{}, ErrInvalidPublicInput
		}
		minimum, maximum := *question.Validation.MinimumSelections, *question.Validation.MaximumSelections
		if minimum < 0 || maximum < 1 || minimum > maximum || maximum > len(question.Options) || question.Type == surveyport.SingleChoice && maximum != 1 || question.Required && minimum == 0 {
			return surveyport.PublicQuestionnaire{}, ErrInvalidPublicInput
		}
		projected := surveyport.PublicQuestion{ID: int64(question.ID), Type: question.Type, Title: question.Title, Required: question.Required, SortOrder: question.SortOrder, Minimum: minimum, Maximum: maximum, Options: make([]surveyport.PublicOption, 0, len(question.Options))}
		for optionIndex, option := range question.Options {
			if option.ID < 1 || option.SortOrder != optionIndex || option.IsOther || option.Score != 0 || option.AssessmentTypeKey != "" || len(option.TagCodes) != 0 || option.OtherPlaceholder != "" || option.OtherMaximumLength != 0 {
				return surveyport.PublicQuestionnaire{}, ErrInvalidPublicInput
			}
			projected.Options = append(projected.Options, surveyport.PublicOption{ID: int64(option.ID), OptionText: option.OptionText, SortOrder: option.SortOrder})
		}
		result.Questions = append(result.Questions, projected)
	}
	return result, nil
}

// ValidPublicSlug is the single public carrier/storage contract. Keeping the
// alphabet deliberately small prevents ambiguous path and query encodings.
func ValidPublicSlug(value string) bool {
	if len(value) < 1 || len(value) > 120 || !publicSlugAlphaNumeric(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		ch := value[index]
		if ch != '-' && !publicSlugAlphaNumeric(ch) {
			return false
		}
	}
	return true
}

func publicSlugAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

// NormalizePublicSubmission rejects every unknown, duplicate, free-text, and
// identity-shaped input before it can reach persistence.
func NormalizePublicSubmission(definition surveyport.PublicQuestionnaire, input surveyport.PublicSubmissionCommand) (surveyport.PublicSubmissionCommand, error) {
	if definition.ID < 1 || definition.Version < 1 || input.Slug != definition.Slug || input.Version != definition.Version || !base64URLKey(input.SubmissionKey) || zeroDigest(input.AnonymousDigest) || zeroDigest(input.RateDigest) || len(input.Answers) > len(definition.Questions) {
		return surveyport.PublicSubmissionCommand{}, ErrInvalidPublicInput
	}
	questions := make(map[int64]surveyport.PublicQuestion, len(definition.Questions))
	for _, question := range definition.Questions {
		questions[question.ID] = question
	}
	seenQuestions := make(map[int64]bool, len(input.Answers))
	answers := append([]surveyport.PublicSubmissionAnswer(nil), input.Answers...)
	for index := range answers {
		answer := &answers[index]
		question, ok := questions[answer.QuestionID]
		if !ok || seenQuestions[answer.QuestionID] || len(answer.OptionIDs) < question.Minimum || len(answer.OptionIDs) > question.Maximum {
			return surveyport.PublicSubmissionCommand{}, ErrInvalidPublicInput
		}
		seenOptions := map[int64]bool{}
		allowed := map[int64]bool{}
		for _, option := range question.Options {
			allowed[option.ID] = true
		}
		for _, optionID := range answer.OptionIDs {
			if !allowed[optionID] || seenOptions[optionID] {
				return surveyport.PublicSubmissionCommand{}, ErrInvalidPublicInput
			}
			seenOptions[optionID] = true
		}
		sort.Slice(answer.OptionIDs, func(i, j int) bool { return answer.OptionIDs[i] < answer.OptionIDs[j] })
		seenQuestions[answer.QuestionID] = true
	}
	for _, question := range definition.Questions {
		if question.Required && !seenQuestions[question.ID] {
			return surveyport.PublicSubmissionCommand{}, ErrInvalidPublicInput
		}
	}
	sort.Slice(answers, func(i, j int) bool { return answers[i].QuestionID < answers[j].QuestionID })
	input.Answers = answers
	return input, nil
}

// DerivePublicResultToken keeps the raw opaque result token out of storage.
// The persisted receipt has only its SHA-256 digest; replay uses its stored
// submission-key digest, submission id, and definition version.
func DerivePublicResultToken(key [32]byte, submissionKeyDigest [32]byte, submissionID, version int64) (string, error) {
	if zeroDigest(key) || submissionID < 1 || version < 1 {
		return "", ErrPublicUnavailable
	}
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte("aicrm.survey.public.result.v1\x00"))
	_, _ = mac.Write(submissionKeyDigest[:])
	_, _ = mac.Write([]byte("\x00" + strconv.FormatInt(submissionID, 10) + "\x00" + strconv.FormatInt(version, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func base64URLKey(value string) bool {
	if len(value) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == value
}
func zeroDigest(value [32]byte) bool { var zero [32]byte; return hmac.Equal(value[:], zero[:]) }
