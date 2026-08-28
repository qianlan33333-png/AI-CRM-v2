package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestSurveyUnresolvedHistoryJournalPinsKindsToExactSourceTargets(t *testing.T) {
	journal, err := NewSurveyUnresolvedHistoryJournal("archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(journal.journals) != 2 {
		t.Fatalf("journal kinds=%d", len(journal.journals))
	}
	for kind, want := range map[string][2]string{
		surveyUnresolvedSubmissionReceiptKind: {"public/questionnaire_submissions", "survey_v1_unresolved_submissions"},
		surveyUnresolvedAnswerReceiptKind:     {"public/questionnaire_submission_answers", "survey_v1_unresolved_answers"},
	} {
		value := journal.journals[kind]
		if value == nil || value.scope.ImportVersion != SurveyUnresolvedHistoryImportVersion || value.scope.ArchiveRunID != "archive-run" || value.scope.AdapterID != v1archive.DefaultAdapterID || value.scope.TableID != want[0] || value.scope.TargetDomain != "survey" || value.scope.TargetTable != want[1] {
			t.Fatalf("kind %s is not pinned to its source/target scope", kind)
		}
	}
	for _, run := range []string{"", " archive-run", "archive-run "} {
		if candidate, err := NewSurveyUnresolvedHistoryJournal(run); !errors.Is(err, ErrInvalidScope) || candidate != nil {
			t.Fatalf("unsafe run %q = %v/%v", run, candidate, err)
		}
	}
}

func TestSurveyUnresolvedHistoryJournalRejectsCrossKindOrUnsafeReceiptBeforeIO(t *testing.T) {
	journal, err := NewSurveyUnresolvedHistoryJournal("archive-run")
	if err != nil {
		t.Fatal(err)
	}
	source := SourceIdentifier(surveyUnresolvedJournalDigest(1))
	receipt := surveyport.SurveyUnresolvedHistoryReceipt{
		Kind: surveyUnresolvedSubmissionReceiptKind, SourceIdentifier: source,
		PayloadDigest: surveyUnresolvedJournalDigest(2), TargetID: 3, TargetDigest: surveyUnresolvedJournalDigest(4),
	}
	if _, _, err = journal.LoadSurveyUnresolvedHistory(context.Background(), "other", source); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("unknown kind load error=%v", err)
	}
	for name, mutate := range map[string]func(*surveyport.SurveyUnresolvedHistoryReceipt){
		"unknown-kind":     func(value *surveyport.SurveyUnresolvedHistoryReceipt) { value.Kind = "other" },
		"malformed-source": func(value *surveyport.SurveyUnresolvedHistoryReceipt) { value.SourceIdentifier = "not-a-source-digest" },
		"zero-target":      func(value *surveyport.SurveyUnresolvedHistoryReceipt) { value.TargetID = 0 },
		"replayed":         func(value *surveyport.SurveyUnresolvedHistoryReceipt) { value.Replayed = true },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := receipt
			mutate(&candidate)
			if recordErr := journal.RecordSurveyUnresolvedHistory(context.Background(), candidate); !errors.Is(recordErr, ErrInvalidScope) {
				t.Fatalf("record error=%v", recordErr)
			}
		})
	}
	if _, _, err = journal.LoadSurveyUnresolvedHistory(context.Background(), surveyUnresolvedSubmissionReceiptKind, source); err == nil {
		t.Fatal("load outside caller transaction accepted")
	}
}

func surveyUnresolvedJournalDigest(first byte) (digest [sha256.Size]byte) {
	digest[0] = first
	return digest
}
