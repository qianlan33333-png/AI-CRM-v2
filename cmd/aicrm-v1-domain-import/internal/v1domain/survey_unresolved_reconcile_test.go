package v1domain

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSurveyUnresolvedHistoryReconcileUsesOnlyIsolatedDefinitionGapSources(t *testing.T) {
	if len(surveyUnresolvedTargets) != 2 {
		t.Fatalf("target count=%d", len(surveyUnresolvedTargets))
	}
	for source, target := range map[string]string{
		"public/questionnaire_submissions":        "survey_v1_unresolved_submissions",
		"public/questionnaire_submission_answers": "survey_v1_unresolved_answers",
	} {
		if surveyUnresolvedTargets[source] != target {
			t.Fatalf("source %s target=%q", source, surveyUnresolvedTargets[source])
		}
	}
	if surveyUnresolvedTargets["public/questionnaire_questions"] != "" || surveyUnresolvedTargets["public/questionnaire_options"] != "" {
		t.Fatal("resolved definition tables must not join unresolved-history reconcile")
	}
}

func TestSurveyUnresolvedHistoryReconcileRejectsInvalidScopeBeforeDatabase(t *testing.T) {
	var pool *pgxpool.Pool
	for _, run := range []string{"", " archive-run", "archive-run "} {
		if _, err := ReconcileSurveyUnresolvedHistory(context.Background(), pool, run); !errors.Is(err, ErrInvalidScope) {
			t.Fatalf("run %q error=%v", run, err)
		}
	}
}

func TestVerifySurveyUnresolvedHistoryTargetFailsClosedWithoutCallerTransaction(t *testing.T) {
	if _, err := verifySurveyUnresolvedHistoryTarget(context.Background(), nil, reconciliationRow{}, nil, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("nil caller transaction error=%v", err)
	}
}
