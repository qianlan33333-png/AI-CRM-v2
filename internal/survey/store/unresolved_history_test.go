package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	survey "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

var surveyUnresolvedHistoryPostgresDSN = flag.String("survey-unresolved-history-postgres-dsn", "", "opt-in PostgreSQL DSN for unresolved survey history")
var errSurveyUnresolvedRollback = errors.New("rollback unresolved survey history")

func unresolvedStoreDigest(value string) [32]byte { return sha256.Sum256([]byte(value)) }
func unresolvedStoreSubmission() survey.HistoricalUnresolvedSurveySubmission {
	at := time.Date(2026, 8, 28, 10, 0, 0, 123456000, time.UTC)
	return survey.HistoricalUnresolvedSurveySubmission{SourceKeyDigest: unresolvedStoreDigest("key"), SourcePayloadDigest: unresolvedStoreDigest("payload"), SourceFieldDigest: unresolvedStoreDigest("field"), FinalTags: []byte("null"), SubmittedAt: at, CreatedAt: at, UnionIDDigest: unresolvedStoreDigest("union"), FollowUserUserIDDigest: unresolvedStoreDigest("follow"), CampaignIDDigest: unresolvedStoreDigest("campaign"), StaffIDDigest: unresolvedStoreDigest("staff"), RedirectURLDigest: unresolvedStoreDigest("redirect"), AssessmentResultDigest: unresolvedStoreDigest("assessment")}
}
func unresolvedStoreAnswer(submissionID int64) survey.HistoricalUnresolvedSurveyAnswer {
	at := time.Date(2026, 8, 28, 10, 0, 0, 123456000, time.UTC)
	return survey.HistoricalUnresolvedSurveyAnswer{SourceKeyDigest: unresolvedStoreDigest("answer-key"), SourcePayloadDigest: unresolvedStoreDigest("answer-payload"), SourceFieldDigest: unresolvedStoreDigest("answer-field"), SubmissionID: submissionID, SelectedOptionIDs: []byte("null"), SelectedOptionTexts: []byte("[]"), SelectedOptionScores: []byte("[]"), SelectedOptionTags: []byte("[]"), CreatedAt: at}
}

func TestSurveyUnresolvedHistoryQueryValidation(t *testing.T) {
	if badSubmission(unresolvedStoreSubmission()) || badAnswer(unresolvedStoreAnswer(1)) {
		t.Fatal("invalid PostgreSQL round-trip fixture")
	}
	for _, q := range []survey.SurveyUnresolvedHistoryQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if !badQuery(q) {
			t.Fatalf("accepted %+v", q)
		}
	}
	if badQuery(survey.SurveyUnresolvedHistoryQuery{Limit: 1}) {
		t.Fatal("rejected valid query")
	}
}
func TestSurveyUnresolvedHistoryTypedNilReaderFailsClosed(t *testing.T) {
	var pool *pgxpool.Pool
	reader := NewSurveyUnresolvedHistoryReader(pool)
	if _, _, err := reader.ListHistoricalUnresolvedSurveySubmissions(context.Background(), survey.SurveyUnresolvedHistoryQuery{Limit: 1}); !errors.Is(err, survey.ErrSurveyUnresolvedHistoryUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestSurveyUnresolvedHistoryPostgresRoundTripRollback(t *testing.T) {
	dsn := *surveyUnresolvedHistoryPostgresDSN
	if dsn == "" {
		t.Skip("survey-unresolved-history-postgres-dsn is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewSurveyUnresolvedHistoryStore()
	reader := NewSurveyUnresolvedHistoryReader(pool)
	uow := platformstore.NewUnitOfWork(pool)
	err = uow.Within(ctx, func(tx context.Context) error {
		submission := unresolvedStoreSubmission()
		got, err := store.CreateHistoricalUnresolvedSurveySubmission(tx, submission)
		if err != nil {
			return err
		}
		if got.QuestionnaireID != nil || got.CustomerID != nil {
			return errors.New("nullable relation changed")
		}
		answer := unresolvedStoreAnswer(got.ID)
		if _, err = store.CreateHistoricalUnresolvedSurveyAnswer(tx, answer); err != nil {
			return err
		}
		listed, total, err := reader.ListHistoricalUnresolvedSurveySubmissions(tx, survey.SurveyUnresolvedHistoryQuery{Limit: 1})
		if err != nil || total != 1 || len(listed) != 1 {
			return errors.New("submission list")
		}
		answers, total, err := reader.ListHistoricalUnresolvedSurveyAnswers(tx, got.ID, survey.SurveyUnresolvedHistoryQuery{Limit: 1})
		if err != nil || total != 1 || len(answers) != 1 {
			return errors.New("answer list")
		}
		return errSurveyUnresolvedRollback
	})
	if !errors.Is(err, errSurveyUnresolvedRollback) {
		t.Fatalf("rollback=%v", err)
	}
}
