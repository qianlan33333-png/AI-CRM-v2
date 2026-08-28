package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"time"

	survey "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	unresolvedSurveySubmissionKind = "unresolved_survey_submission"
	unresolvedSurveyAnswerKind     = "unresolved_survey_answer"
)

type SurveyUnresolvedHistoryWriter struct {
	store   survey.SurveyUnresolvedHistoryStore
	journal survey.SurveyUnresolvedHistoryJournal
}

func NewSurveyUnresolvedHistoryWriter(store survey.SurveyUnresolvedHistoryStore, journal survey.SurveyUnresolvedHistoryJournal) (*SurveyUnresolvedHistoryWriter, error) {
	if nilSurveyUnresolved(store) || nilSurveyUnresolved(journal) {
		return nil, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	return &SurveyUnresolvedHistoryWriter{store: store, journal: journal}, nil
}

func (w *SurveyUnresolvedHistoryWriter) ImportHistoricalUnresolvedSurveySubmission(ctx context.Context, source string, value survey.HistoricalUnresolvedSurveySubmission) (survey.SurveyUnresolvedHistoryReceipt, error) {
	value = normalizeUnresolvedSubmission(value)
	return importUnresolvedSurvey(w, ctx, unresolvedSurveySubmissionKind, source, value, HistoricalUnresolvedSurveySubmissionDigest,
		func(v survey.HistoricalUnresolvedSurveySubmission, id int64) survey.HistoricalUnresolvedSurveySubmission {
			v.ID = id
			return v
		},
		func() (survey.HistoricalUnresolvedSurveySubmission, error) {
			return w.store.CreateHistoricalUnresolvedSurveySubmission(ctx, value)
		},
		func(id int64) (survey.HistoricalUnresolvedSurveySubmission, error) {
			return w.store.GetHistoricalUnresolvedSurveySubmission(ctx, id)
		})
}

func (w *SurveyUnresolvedHistoryWriter) ImportHistoricalUnresolvedSurveyAnswer(ctx context.Context, source string, value survey.HistoricalUnresolvedSurveyAnswer) (survey.SurveyUnresolvedHistoryReceipt, error) {
	value = normalizeUnresolvedAnswer(value)
	return importUnresolvedSurvey(w, ctx, unresolvedSurveyAnswerKind, source, value, HistoricalUnresolvedSurveyAnswerDigest,
		func(v survey.HistoricalUnresolvedSurveyAnswer, id int64) survey.HistoricalUnresolvedSurveyAnswer {
			v.ID = id
			return v
		},
		func() (survey.HistoricalUnresolvedSurveyAnswer, error) {
			return w.store.CreateHistoricalUnresolvedSurveyAnswer(ctx, value)
		},
		func(id int64) (survey.HistoricalUnresolvedSurveyAnswer, error) {
			return w.store.GetHistoricalUnresolvedSurveyAnswer(ctx, id)
		})
}

func importUnresolvedSurvey[T any](w *SurveyUnresolvedHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, create func() (T, error), get func(int64) (T, error)) (survey.SurveyUnresolvedHistoryReceipt, error) {
	var empty survey.SurveyUnresolvedHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilSurveyUnresolved(w.store) || nilSurveyUnresolved(w.journal) {
		return empty, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	key, payload, id, ok := unresolvedIdentity(value)
	if !ok || id != 0 || key == ([32]byte{}) || payload == ([32]byte{}) || source != hex.EncodeToString(key[:]) {
		return empty, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	receipt, found, err := w.journal.LoadSurveyUnresolvedHistory(ctx, kind, source)
	if err != nil {
		return empty, unresolvedError(err)
	}
	if found {
		if !validUnresolvedReceipt(receipt, kind, source, payload) {
			return empty, survey.ErrSurveyUnresolvedHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, unresolvedError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, survey.ErrSurveyUnresolvedHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, unresolvedError(err)
	}
	_, _, targetID, ok := unresolvedIdentity(actual)
	if !ok || targetID < 1 {
		return empty, survey.ErrSurveyUnresolvedHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, survey.ErrSurveyUnresolvedHistoryConflict
	}
	receipt = survey.SurveyUnresolvedHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err := w.journal.RecordSurveyUnresolvedHistory(ctx, receipt); err != nil {
		return empty, unresolvedError(err)
	}
	return receipt, nil
}

func HistoricalUnresolvedSurveySubmissionDigest(v survey.HistoricalUnresolvedSurveySubmission) ([32]byte, error) {
	if !validUnresolvedSubmission(v, true) {
		return [32]byte{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	return unresolvedDigest(unresolvedSurveySubmissionKind, struct {
		ID                                                                            int64
		Key, Payload, Field                                                           [32]byte
		SourceID, QuestionnaireSourceID                                               int64
		QuestionnaireID, CustomerID                                                   *int64
		MatchedBy, SourceChannel                                                      string
		TotalScore                                                                    float64
		FinalTags                                                                     json.RawMessage
		SubmittedAt, CreatedAt                                                        time.Time
		UnionID, FollowUserUserID, CampaignID, StaffID, RedirectURL, AssessmentResult [32]byte
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.SourceID, v.QuestionnaireSourceID, v.QuestionnaireID, v.CustomerID, v.MatchedBy, v.SourceChannel, v.TotalScore, v.FinalTags, v.SubmittedAt, v.CreatedAt, v.UnionIDDigest, v.FollowUserUserIDDigest, v.CampaignIDDigest, v.StaffIDDigest, v.RedirectURLDigest, v.AssessmentResultDigest})
}

func HistoricalUnresolvedSurveyAnswerDigest(v survey.HistoricalUnresolvedSurveyAnswer) ([32]byte, error) {
	if !validUnresolvedAnswer(v, true) {
		return [32]byte{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	return unresolvedDigest(unresolvedSurveyAnswerKind, struct {
		ID                                                                               int64
		Key, Payload, Field                                                              [32]byte
		SourceID, SubmissionID, SubmissionSourceID, QuestionSourceID                     int64
		QuestionType, QuestionTitleSnapshot                                              string
		SelectedOptionIDs, SelectedOptionTexts, SelectedOptionScores, SelectedOptionTags json.RawMessage
		TextValue                                                                        string
		ScoreContribution                                                                float64
		CreatedAt                                                                        time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.SourceID, v.SubmissionID, v.SubmissionSourceID, v.QuestionSourceID, v.QuestionType, v.QuestionTitleSnapshot, v.SelectedOptionIDs, v.SelectedOptionTexts, v.SelectedOptionScores, v.SelectedOptionTags, v.TextValue, v.ScoreContribution, v.CreatedAt})
}

func unresolvedDigest(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{kind, value})
	if err != nil {
		return [32]byte{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validUnresolvedIdentity(id int64, key, payload, field [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && key != ([32]byte{}) && payload != ([32]byte{}) && field != ([32]byte{})
}
func validUnresolvedTime(value time.Time, stored bool) bool {
	return !value.IsZero() && (!stored || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func validJSON(value json.RawMessage) bool { return len(value) > 0 && json.Valid(value) }
func validOptionalID(value *int64) bool    { return value == nil || *value > 0 }
func validScore(value float64) bool        { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func validUnresolvedSubmission(v survey.HistoricalUnresolvedSurveySubmission, stored bool) bool {
	return validUnresolvedIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && validOptionalID(v.QuestionnaireID) && validOptionalID(v.CustomerID) && validJSON(v.FinalTags) && validScore(v.TotalScore) && validUnresolvedTime(v.SubmittedAt, stored) && validUnresolvedTime(v.CreatedAt, stored) && v.UnionIDDigest != ([32]byte{}) && v.FollowUserUserIDDigest != ([32]byte{}) && v.CampaignIDDigest != ([32]byte{}) && v.StaffIDDigest != ([32]byte{}) && v.RedirectURLDigest != ([32]byte{}) && v.AssessmentResultDigest != ([32]byte{})
}
func validUnresolvedAnswer(v survey.HistoricalUnresolvedSurveyAnswer, stored bool) bool {
	return validUnresolvedIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && v.SubmissionID > 0 && validJSON(v.SelectedOptionIDs) && validJSON(v.SelectedOptionTexts) && validJSON(v.SelectedOptionScores) && validJSON(v.SelectedOptionTags) && validScore(v.ScoreContribution) && validUnresolvedTime(v.CreatedAt, stored)
}
func normalizeUnresolvedTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
func normalizeUnresolvedSubmission(v survey.HistoricalUnresolvedSurveySubmission) survey.HistoricalUnresolvedSurveySubmission {
	v.SubmittedAt, v.CreatedAt = normalizeUnresolvedTime(v.SubmittedAt), normalizeUnresolvedTime(v.CreatedAt)
	return v
}
func normalizeUnresolvedAnswer(v survey.HistoricalUnresolvedSurveyAnswer) survey.HistoricalUnresolvedSurveyAnswer {
	v.CreatedAt = normalizeUnresolvedTime(v.CreatedAt)
	return v
}
func unresolvedIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch v := value.(type) {
	case survey.HistoricalUnresolvedSurveySubmission:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case survey.HistoricalUnresolvedSurveyAnswer:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}
func validUnresolvedReceipt(v survey.SurveyUnresolvedHistoryReceipt, kind, source string, payload [32]byte) bool {
	return v.Kind == kind && v.SourceIdentifier == source && v.PayloadDigest == payload && v.TargetID > 0 && v.TargetDigest != ([32]byte{})
}
func unresolvedError(err error) error {
	if errors.Is(err, survey.ErrSurveyUnresolvedHistoryInvalid) {
		return survey.ErrSurveyUnresolvedHistoryInvalid
	}
	if errors.Is(err, survey.ErrSurveyUnresolvedHistoryConflict) {
		return survey.ErrSurveyUnresolvedHistoryConflict
	}
	return survey.ErrSurveyUnresolvedHistoryUnavailable
}
func nilSurveyUnresolved(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
