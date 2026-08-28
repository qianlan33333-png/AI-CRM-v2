package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	survey "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type unresolvedTestStore struct {
	submission survey.HistoricalUnresolvedSurveySubmission
	answer     survey.HistoricalUnresolvedSurveyAnswer
	next       int64
}

func (s *unresolvedTestStore) CreateHistoricalUnresolvedSurveySubmission(_ context.Context, v survey.HistoricalUnresolvedSurveySubmission) (survey.HistoricalUnresolvedSurveySubmission, error) {
	s.next++
	v.ID = s.next
	s.submission = v
	return v, nil
}
func (s *unresolvedTestStore) GetHistoricalUnresolvedSurveySubmission(_ context.Context, id int64) (survey.HistoricalUnresolvedSurveySubmission, error) {
	if id != s.submission.ID {
		return survey.HistoricalUnresolvedSurveySubmission{}, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	return s.submission, nil
}
func (s *unresolvedTestStore) CreateHistoricalUnresolvedSurveyAnswer(_ context.Context, v survey.HistoricalUnresolvedSurveyAnswer) (survey.HistoricalUnresolvedSurveyAnswer, error) {
	s.next++
	v.ID = s.next
	s.answer = v
	return v, nil
}
func (s *unresolvedTestStore) GetHistoricalUnresolvedSurveyAnswer(_ context.Context, id int64) (survey.HistoricalUnresolvedSurveyAnswer, error) {
	if id != s.answer.ID {
		return survey.HistoricalUnresolvedSurveyAnswer{}, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	return s.answer, nil
}

type unresolvedTestJournal struct {
	values map[string]survey.SurveyUnresolvedHistoryReceipt
}

func (j *unresolvedTestJournal) LoadSurveyUnresolvedHistory(_ context.Context, kind, source string) (survey.SurveyUnresolvedHistoryReceipt, bool, error) {
	v, ok := j.values[kind+source]
	return v, ok, nil
}
func (j *unresolvedTestJournal) RecordSurveyUnresolvedHistory(_ context.Context, v survey.SurveyUnresolvedHistoryReceipt) error {
	j.values[v.Kind+v.SourceIdentifier] = v
	return nil
}
func unresolvedTestDigest(seed string) [32]byte { return sha256.Sum256([]byte(seed)) }
func unresolvedSubmission() survey.HistoricalUnresolvedSurveySubmission {
	at := time.Date(2026, 8, 28, 10, 0, 0, 123456789, time.FixedZone("CST", 8*3600))
	return survey.HistoricalUnresolvedSurveySubmission{SourceKeyDigest: unresolvedTestDigest("key"), SourcePayloadDigest: unresolvedTestDigest("payload"), SourceFieldDigest: unresolvedTestDigest("field"), SourceID: -1, QuestionnaireSourceID: 0, MatchedBy: "", SourceChannel: "", TotalScore: 0, FinalTags: []byte("null"), SubmittedAt: at, CreatedAt: at, UnionIDDigest: unresolvedTestDigest("union"), FollowUserUserIDDigest: unresolvedTestDigest("follow"), CampaignIDDigest: unresolvedTestDigest("campaign"), StaffIDDigest: unresolvedTestDigest("staff"), RedirectURLDigest: unresolvedTestDigest("redirect"), AssessmentResultDigest: unresolvedTestDigest("assessment")}
}
func unresolvedAnswer(submissionID int64) survey.HistoricalUnresolvedSurveyAnswer {
	at := time.Date(2026, 8, 28, 10, 0, 0, 123456789, time.FixedZone("CST", 8*3600))
	return survey.HistoricalUnresolvedSurveyAnswer{SourceKeyDigest: unresolvedTestDigest("answer-key"), SourcePayloadDigest: unresolvedTestDigest("answer-payload"), SourceFieldDigest: unresolvedTestDigest("answer-field"), SourceID: 0, SubmissionID: submissionID, SubmissionSourceID: -1, QuestionSourceID: 0, QuestionType: "", QuestionTitleSnapshot: "", SelectedOptionIDs: []byte("null"), SelectedOptionTexts: []byte("[]"), SelectedOptionScores: []byte("[]"), SelectedOptionTags: []byte("[]"), TextValue: "", CreatedAt: at}
}

func TestSurveyUnresolvedHistoryReplayAndPrivateDigest(t *testing.T) {
	store := &unresolvedTestStore{}
	journal := &unresolvedTestJournal{values: map[string]survey.SurveyUnresolvedHistoryReceipt{}}
	writer, err := NewSurveyUnresolvedHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	v := unresolvedSubmission()
	source := hex.EncodeToString(v.SourceKeyDigest[:])
	first, err := writer.ImportHistoricalUnresolvedSurveySubmission(context.Background(), source, v)
	if err != nil {
		t.Fatal(err)
	}
	second, err := writer.ImportHistoricalUnresolvedSurveySubmission(context.Background(), source, v)
	if err != nil || !second.Replayed || second.TargetID != first.TargetID {
		t.Fatalf("replay=%+v err=%v", second, err)
	}
	changed := store.submission
	changed.AssessmentResultDigest = unresolvedTestDigest("changed")
	store.submission = changed
	if _, err = writer.ImportHistoricalUnresolvedSurveySubmission(context.Background(), source, v); !errors.Is(err, survey.ErrSurveyUnresolvedHistoryConflict) {
		t.Fatalf("private digest drift=%v", err)
	}
}
func TestHistoricalUnresolvedSurveyAnswerDigestRejectsInvalidJSON(t *testing.T) {
	v := unresolvedAnswer(1)
	v.SelectedOptionTags = []byte("{")
	if _, err := HistoricalUnresolvedSurveyAnswerDigest(v); !errors.Is(err, survey.ErrSurveyUnresolvedHistoryInvalid) {
		t.Fatalf("err=%v", err)
	}
}
