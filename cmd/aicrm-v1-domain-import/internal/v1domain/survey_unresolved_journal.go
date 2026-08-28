package v1domain

import (
	"context"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const SurveyUnresolvedHistoryImportVersion = "v1-survey-unresolved-history-a1"

var surveyUnresolvedTargets = map[string]string{
	"public/questionnaire_submissions":        "survey_v1_unresolved_submissions",
	"public/questionnaire_submission_answers": "survey_v1_unresolved_answers",
}

type SurveyUnresolvedHistoryJournal struct{ journals map[string]*Journal }

func NewSurveyUnresolvedHistoryJournal(run string) (*SurveyUnresolvedHistoryJournal, error) {
	j := &SurveyUnresolvedHistoryJournal{journals: map[string]*Journal{}}
	for kind, source := range map[string]string{"unresolved_survey_submission": "public/questionnaire_submissions", "unresolved_survey_answer": "public/questionnaire_submission_answers"} {
		value, err := NewJournal(Scope{ImportVersion: SurveyUnresolvedHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: source, TargetDomain: "survey", TargetTable: surveyUnresolvedTargets[source]})
		if err != nil {
			return nil, err
		}
		j.journals[kind] = value
	}
	return j, nil
}

func (j *SurveyUnresolvedHistoryJournal) LoadSurveyUnresolvedHistory(ctx context.Context, kind, source string) (surveyport.SurveyUnresolvedHistoryReceipt, bool, error) {
	if j == nil || j.journals[kind] == nil {
		return surveyport.SurveyUnresolvedHistoryReceipt{}, false, ErrInvalidScope
	}
	r, found, err := j.journals[kind].LoadTerminal(ctx, source)
	if err != nil || !found {
		return surveyport.SurveyUnresolvedHistoryReceipt{}, found, err
	}
	id, err := positiveID(r.TargetID)
	if err != nil || r.Disposition != "import" || r.Reason != "" || len(r.Metadata) != 0 {
		return surveyport.SurveyUnresolvedHistoryReceipt{}, false, ErrConflict
	}
	return surveyport.SurveyUnresolvedHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: r.PayloadDigest, TargetID: id, TargetDigest: r.TargetDigest}, true, nil
}

func (j *SurveyUnresolvedHistoryJournal) RecordSurveyUnresolvedHistory(ctx context.Context, r surveyport.SurveyUnresolvedHistoryReceipt) error {
	if j == nil || j.journals[r.Kind] == nil || r.TargetID < 1 || r.Replayed {
		return ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(r.SourceIdentifier)
	if err != nil {
		return err
	}
	return j.journals[r.Kind].Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: r.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(r.TargetID, 10), TargetDigest: r.TargetDigest})
}
