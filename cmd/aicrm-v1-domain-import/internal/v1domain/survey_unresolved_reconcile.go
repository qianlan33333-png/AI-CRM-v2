package v1domain

import (
	"context"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
)

func ReconcileSurveyUnresolvedHistory(ctx context.Context, pool *pgxpool.Pool, run string) (ReconciliationResult, error) {
	return reconcileTables(ctx, pool, SurveyUnresolvedHistoryImportVersion, run, []string{"public/questionnaire_submissions", "public/questionnaire_submission_answers"})
}

func verifySurveyUnresolvedHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, archiveField []byte, targets map[string]map[string]struct{}) (string, error) {
	if tx == nil || row.TargetDomain == nil || *row.TargetDomain != "survey" || row.TargetTable == nil || *row.TargetTable != surveyUnresolvedTargets[row.TableID] || row.TargetID == nil || len(archiveField) != 32 {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	reader := surveystore.NewSurveyUnresolvedHistoryReader(tx)
	var key, payload, field, digest [32]byte
	switch row.TableID {
	case "public/questionnaire_submissions":
		v, e := reader.GetHistoricalUnresolvedSurveySubmission(ctx, id)
		if e != nil || v.ID != id {
			return "", ErrConflict
		}
		key, payload, field = v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest
		digest, err = surveyapp.HistoricalUnresolvedSurveySubmissionDigest(v)
	case "public/questionnaire_submission_answers":
		v, e := reader.GetHistoricalUnresolvedSurveyAnswer(ctx, id)
		if e != nil || v.ID != id || !containsTarget(targets, "survey_v1_unresolved_submissions", strconv.FormatInt(v.SubmissionID, 10)) {
			return "", ErrConflict
		}
		parent, e := reader.GetHistoricalUnresolvedSurveySubmission(ctx, v.SubmissionID)
		if e != nil || parent.SourceID != v.SubmissionSourceID {
			return "", ErrConflict
		}
		key, payload, field = v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest
		digest, err = surveyapp.HistoricalUnresolvedSurveyAnswerDigest(v)
	default:
		return "", ErrConflict
	}
	if err != nil || !equalBytes(key[:], row.SourceKeyDigest) || !equalBytes(payload[:], row.PayloadDigest) || !equalBytes(field[:], archiveField) || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}
