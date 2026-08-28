package v1domain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"reflect"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	surveyUnresolvedSubmissionReceiptKind = "unresolved_survey_submission"
	surveyUnresolvedAnswerReceiptKind     = "unresolved_survey_answer"
	surveyUnresolvedPrivateDigestDomain   = "ai-crm-v2/v1-survey-unresolved-history"
)

// SurveyUnresolvedHistoryWriter owns the inert Survey history rows. Its
// methods are called inside the importer's caller-bound transaction.
type SurveyUnresolvedHistoryWriter interface {
	ImportHistoricalUnresolvedSurveySubmission(context.Context, string, surveyport.HistoricalUnresolvedSurveySubmission) (surveyport.SurveyUnresolvedHistoryReceipt, error)
	ImportHistoricalUnresolvedSurveyAnswer(context.Context, string, surveyport.HistoricalUnresolvedSurveyAnswer) (surveyport.SurveyUnresolvedHistoryReceipt, error)
}

// SurveyUnresolvedHistoryReferences only resolves already-verified historical
// links. Nil links remain unresolved source history rather than guessed IDs.
type SurveyUnresolvedHistoryReferences interface {
	ResolveSurveyUnresolvedQuestionnaire(context.Context, int64) (*int64, error)
	ResolveSurveyUnresolvedCustomer(context.Context, string) (*int64, error)
	VerifySurveyUnresolvedSource(context.Context, v1archive.ArchivedRow) error
}

type SurveyUnresolvedHistoryImportResult struct {
	ImportedSubmissions int
	ImportedAnswers     int
	Replayed            int
}

type SurveyUnresolvedHistoryImporter struct {
	archive       ArchiveSource
	uow           UnitOfWork
	writer        SurveyUnresolvedHistoryWriter
	references    SurveyUnresolvedHistoryReferences
	sourceHMACKey []byte
}

func NewSurveyUnresolvedHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer SurveyUnresolvedHistoryWriter, references SurveyUnresolvedHistoryReferences, sourceHMACKey []byte) (*SurveyUnresolvedHistoryImporter, error) {
	if nilSurveyUnresolvedImporter(archive) || nilSurveyUnresolvedImporter(uow) || nilSurveyUnresolvedImporter(writer) || nilSurveyUnresolvedImporter(references) || len(sourceHMACKey) < sha256.Size {
		return nil, ErrInvalidScope
	}
	return &SurveyUnresolvedHistoryImporter{
		archive: archive, uow: uow, writer: writer, references: references, sourceHMACKey: append([]byte(nil), sourceHMACKey...),
	}, nil
}

// Import writes only the source groups that the earlier Survey import isolated
// because their historical definition cannot be resolved. It does not modify
// the old quarantine receipts or any current Survey definition.
func (importer *SurveyUnresolvedHistoryImporter) Import(ctx context.Context, archiveRunID string) (SurveyUnresolvedHistoryImportResult, error) {
	if importer == nil || ctx == nil || archiveRunID == "" || nilSurveyUnresolvedImporter(importer.archive) || nilSurveyUnresolvedImporter(importer.uow) || nilSurveyUnresolvedImporter(importer.writer) || nilSurveyUnresolvedImporter(importer.references) || len(importer.sourceHMACKey) < sha256.Size {
		return SurveyUnresolvedHistoryImportResult{}, ErrInvalidScope
	}
	candidates, err := BuildSurveyUnresolvedCandidates(ctx, importer.archive, archiveRunID)
	if err != nil {
		return SurveyUnresolvedHistoryImportResult{}, err
	}

	result := SurveyUnresolvedHistoryImportResult{}
	submissionTargets := make(map[int64]int64, len(candidates.Submissions))
	for _, candidate := range candidates.Submissions {
		receipt, err := importer.importSubmission(ctx, candidate)
		if err != nil {
			return SurveyUnresolvedHistoryImportResult{}, err
		}
		if receipt.Replayed {
			result.Replayed++
		} else {
			result.ImportedSubmissions++
		}
		if _, duplicate := submissionTargets[candidate.Source.SourceID]; duplicate {
			return SurveyUnresolvedHistoryImportResult{}, ErrConflict
		}
		submissionTargets[candidate.Source.SourceID] = receipt.TargetID
	}
	for _, candidate := range candidates.Answers {
		submissionID, found := submissionTargets[candidate.Source.SubmissionSourceID]
		if !found || submissionID < 1 {
			return SurveyUnresolvedHistoryImportResult{}, ErrConflict
		}
		receipt, err := importer.importAnswer(ctx, candidate, submissionID)
		if err != nil {
			return SurveyUnresolvedHistoryImportResult{}, err
		}
		if receipt.Replayed {
			result.Replayed++
		} else {
			result.ImportedAnswers++
		}
	}
	if result.ImportedSubmissions+result.ImportedAnswers+result.Replayed != len(candidates.Submissions)+len(candidates.Answers) {
		return SurveyUnresolvedHistoryImportResult{}, ErrConflict
	}
	return result, nil
}

func (importer *SurveyUnresolvedHistoryImporter) importSubmission(ctx context.Context, candidate SurveyUnresolvedSubmissionCandidate) (surveyport.SurveyUnresolvedHistoryReceipt, error) {
	var receipt surveyport.SurveyUnresolvedHistoryReceipt
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		receipt = surveyport.SurveyUnresolvedHistoryReceipt{}
		if !validSurveyUnresolvedImportRow(candidate.ArchivedRow, "public/questionnaire_submissions") {
			return ErrConflict
		}
		if err := importer.references.VerifySurveyUnresolvedSource(tx, candidate.ArchivedRow); err != nil {
			return err
		}
		questionnaireID, err := importer.references.ResolveSurveyUnresolvedQuestionnaire(tx, candidate.Source.QuestionnaireSourceID)
		if err != nil {
			return err
		}
		customerID, err := importer.references.ResolveSurveyUnresolvedCustomer(tx, candidate.Source.UnionID)
		if err != nil {
			return err
		}
		if !validSurveyUnresolvedResolvedID(questionnaireID) || !validSurveyUnresolvedResolvedID(customerID) {
			return ErrConflict
		}
		value := surveyport.HistoricalUnresolvedSurveySubmission{
			SourceKeyDigest: candidate.ArchivedRow.SourceKeyHMAC, SourcePayloadDigest: candidate.ArchivedRow.PayloadHMAC, SourceFieldDigest: candidate.ArchivedRow.FieldHMAC,
			SourceID: candidate.Source.SourceID, QuestionnaireSourceID: candidate.Source.QuestionnaireSourceID,
			QuestionnaireID: copySurveyUnresolvedID(questionnaireID), CustomerID: copySurveyUnresolvedID(customerID),
			MatchedBy: candidate.Source.MatchedBy, SourceChannel: candidate.Source.SourceChannel, TotalScore: candidate.Source.TotalScore,
			FinalTags: cloneSurveyUnresolvedRaw(candidate.Source.FinalTags), SubmittedAt: candidate.Source.SubmittedAt, CreatedAt: candidate.Source.CreatedAt,
			UnionIDDigest:          importer.privateDigest("unionid", []byte(candidate.Source.UnionID)),
			FollowUserUserIDDigest: importer.privateDigest("follow_user_userid", []byte(candidate.Source.FollowUserUserID)),
			CampaignIDDigest:       importer.privateDigest("campaign_id", []byte(candidate.Source.CampaignID)),
			StaffIDDigest:          importer.privateDigest("staff_id", []byte(candidate.Source.StaffID)),
			RedirectURLDigest:      importer.privateDigest("redirect_url_snapshot", []byte(candidate.Source.RedirectURLSnapshot)),
			AssessmentResultDigest: importer.privateDigest("assessment_result_snapshot", candidate.Source.AssessmentResult),
		}
		receipt, err = importer.writer.ImportHistoricalUnresolvedSurveySubmission(tx, SourceIdentifier(candidate.ArchivedRow.SourceKeyHMAC), value)
		if err != nil {
			return err
		}
		return verifySurveyUnresolvedReceipt(surveyUnresolvedSubmissionReceiptKind, candidate.ArchivedRow, receipt)
	})
	return receipt, err
}

func (importer *SurveyUnresolvedHistoryImporter) importAnswer(ctx context.Context, candidate SurveyUnresolvedAnswerCandidate, submissionID int64) (surveyport.SurveyUnresolvedHistoryReceipt, error) {
	var receipt surveyport.SurveyUnresolvedHistoryReceipt
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		receipt = surveyport.SurveyUnresolvedHistoryReceipt{}
		if !validSurveyUnresolvedImportRow(candidate.ArchivedRow, "public/questionnaire_submission_answers") || submissionID < 1 {
			return ErrConflict
		}
		if err := importer.references.VerifySurveyUnresolvedSource(tx, candidate.ArchivedRow); err != nil {
			return err
		}
		optionIDs, err := surveyUnresolvedJSONSnapshot(candidate.ArchivedRow.Payload, "selected_option_ids")
		if err != nil {
			return err
		}
		optionTexts, err := surveyUnresolvedJSONSnapshot(candidate.ArchivedRow.Payload, "selected_option_texts_snapshot")
		if err != nil {
			return err
		}
		optionScores, err := surveyUnresolvedJSONSnapshot(candidate.ArchivedRow.Payload, "selected_option_scores_snapshot")
		if err != nil {
			return err
		}
		optionTags, err := surveyUnresolvedJSONSnapshot(candidate.ArchivedRow.Payload, "selected_option_tags_snapshot")
		if err != nil {
			return err
		}
		value := surveyport.HistoricalUnresolvedSurveyAnswer{
			SourceKeyDigest: candidate.ArchivedRow.SourceKeyHMAC, SourcePayloadDigest: candidate.ArchivedRow.PayloadHMAC, SourceFieldDigest: candidate.ArchivedRow.FieldHMAC,
			SourceID: candidate.Source.SourceID, SubmissionID: submissionID, SubmissionSourceID: candidate.Source.SubmissionSourceID,
			QuestionSourceID: candidate.Source.QuestionSourceID, QuestionType: candidate.Source.QuestionType, QuestionTitleSnapshot: candidate.Source.QuestionTitleSnapshot,
			SelectedOptionIDs: optionIDs, SelectedOptionTexts: optionTexts, SelectedOptionScores: optionScores, SelectedOptionTags: optionTags,
			TextValue: candidate.Source.TextValue, ScoreContribution: candidate.Source.ScoreContribution, CreatedAt: candidate.Source.CreatedAt,
		}
		receipt, err = importer.writer.ImportHistoricalUnresolvedSurveyAnswer(tx, SourceIdentifier(candidate.ArchivedRow.SourceKeyHMAC), value)
		if err != nil {
			return err
		}
		return verifySurveyUnresolvedReceipt(surveyUnresolvedAnswerReceiptKind, candidate.ArchivedRow, receipt)
	})
	return receipt, err
}

func validSurveyUnresolvedImportRow(row v1archive.ArchivedRow, tableID string) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == tableID && row.SourceOrdinal > 0 &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{})
}

func validSurveyUnresolvedResolvedID(value *int64) bool {
	return value == nil || *value > 0
}

func copySurveyUnresolvedID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneSurveyUnresolvedRaw(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func (importer *SurveyUnresolvedHistoryImporter) privateDigest(field string, value []byte) [sha256.Size]byte {
	mac := hmac.New(sha256.New, importer.sourceHMACKey)
	_, _ = mac.Write([]byte(surveyUnresolvedPrivateDigestDomain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(field))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(value)
	var digest [sha256.Size]byte
	copy(digest[:], mac.Sum(nil))
	return digest
}

func surveyUnresolvedJSONSnapshot(payload []byte, field string) (json.RawMessage, error) {
	var values map[string]json.RawMessage
	if !json.Valid(payload) || json.Unmarshal(payload, &values) != nil {
		return nil, ErrConflict
	}
	value, found := values[field]
	if !found || !json.Valid(value) {
		return nil, ErrConflict
	}
	return cloneSurveyUnresolvedRaw(value), nil
}

func verifySurveyUnresolvedReceipt(kind string, row v1archive.ArchivedRow, receipt surveyport.SurveyUnresolvedHistoryReceipt) error {
	if receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	return nil
}

func nilSurveyUnresolvedImporter(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Func) && reflected.IsNil()
}
