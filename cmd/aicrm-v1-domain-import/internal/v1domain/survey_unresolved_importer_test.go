package v1domain

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type surveyUnresolvedImporterTxKey struct{}

type surveyUnresolvedImporterUOW struct{}

func (surveyUnresolvedImporterUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(context.WithValue(ctx, surveyUnresolvedImporterTxKey{}, true))
}

type surveyUnresolvedImporterReferences struct {
	questionnaire, customer *int64
	verifyErr               error
	verified                []v1archive.ArchivedRow
	questionnaireCalls      int
	customerCalls           int
}

func (references *surveyUnresolvedImporterReferences) ResolveSurveyUnresolvedQuestionnaire(ctx context.Context, _ int64) (*int64, error) {
	if ctx.Value(surveyUnresolvedImporterTxKey{}) != true {
		return nil, errors.New("questionnaire resolution outside transaction")
	}
	references.questionnaireCalls++
	return copySurveyUnresolvedID(references.questionnaire), nil
}

func (references *surveyUnresolvedImporterReferences) ResolveSurveyUnresolvedCustomer(ctx context.Context, _ string) (*int64, error) {
	if ctx.Value(surveyUnresolvedImporterTxKey{}) != true {
		return nil, errors.New("customer resolution outside transaction")
	}
	references.customerCalls++
	return copySurveyUnresolvedID(references.customer), nil
}

func (references *surveyUnresolvedImporterReferences) VerifySurveyUnresolvedSource(ctx context.Context, row v1archive.ArchivedRow) error {
	if ctx.Value(surveyUnresolvedImporterTxKey{}) != true {
		return errors.New("source verification outside transaction")
	}
	references.verified = append(references.verified, row)
	return references.verifyErr
}

type surveyUnresolvedImporterWriter struct {
	submissions map[string]surveyport.SurveyUnresolvedHistoryReceipt
	answers     map[string]surveyport.SurveyUnresolvedHistoryReceipt

	submissionValues []surveyport.HistoricalUnresolvedSurveySubmission
	answerValues     []surveyport.HistoricalUnresolvedSurveyAnswer
	badReceipt       bool
}

func newSurveyUnresolvedImporterWriter() *surveyUnresolvedImporterWriter {
	return &surveyUnresolvedImporterWriter{
		submissions: map[string]surveyport.SurveyUnresolvedHistoryReceipt{},
		answers:     map[string]surveyport.SurveyUnresolvedHistoryReceipt{},
	}
}

func (writer *surveyUnresolvedImporterWriter) ImportHistoricalUnresolvedSurveySubmission(ctx context.Context, source string, value surveyport.HistoricalUnresolvedSurveySubmission) (surveyport.SurveyUnresolvedHistoryReceipt, error) {
	if ctx.Value(surveyUnresolvedImporterTxKey{}) != true {
		return surveyport.SurveyUnresolvedHistoryReceipt{}, errors.New("submission write outside transaction")
	}
	if receipt, found := writer.submissions[source]; found {
		receipt.Replayed = true
		return receipt, nil
	}
	writer.submissionValues = append(writer.submissionValues, value)
	receipt := surveyUnresolvedImporterReceipt(surveyUnresolvedSubmissionReceiptKind, source, value.SourcePayloadDigest, int64(700+len(writer.submissionValues)))
	if writer.badReceipt {
		receipt.PayloadDigest = sha256.Sum256([]byte("wrong-payload"))
	}
	writer.submissions[source] = receipt
	return receipt, nil
}

func (writer *surveyUnresolvedImporterWriter) ImportHistoricalUnresolvedSurveyAnswer(ctx context.Context, source string, value surveyport.HistoricalUnresolvedSurveyAnswer) (surveyport.SurveyUnresolvedHistoryReceipt, error) {
	if ctx.Value(surveyUnresolvedImporterTxKey{}) != true {
		return surveyport.SurveyUnresolvedHistoryReceipt{}, errors.New("answer write outside transaction")
	}
	if receipt, found := writer.answers[source]; found {
		receipt.Replayed = true
		return receipt, nil
	}
	writer.answerValues = append(writer.answerValues, value)
	receipt := surveyUnresolvedImporterReceipt(surveyUnresolvedAnswerReceiptKind, source, value.SourcePayloadDigest, int64(800+len(writer.answerValues)))
	if writer.badReceipt {
		receipt.PayloadDigest = sha256.Sum256([]byte("wrong-payload"))
	}
	writer.answers[source] = receipt
	return receipt, nil
}

func surveyUnresolvedImporterReceipt(kind, source string, payload [sha256.Size]byte, targetID int64) surveyport.SurveyUnresolvedHistoryReceipt {
	return surveyport.SurveyUnresolvedHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: targetID, TargetDigest: sha256.Sum256([]byte(kind + "/" + source))}
}

func TestSurveyUnresolvedHistoryImporterWritesVerifiedWholeGroups(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 123456000, time.UTC)
	archive := surveyUnresolvedFixture(t, stamp, 999, []int64{100})
	questionnaireID, customerID := int64(41), int64(51)
	references := &surveyUnresolvedImporterReferences{questionnaire: &questionnaireID, customer: &customerID}
	writer := newSurveyUnresolvedImporterWriter()
	key := bytes.Repeat([]byte{7}, sha256.Size)
	importer, err := NewSurveyUnresolvedHistoryImporter(archive, surveyUnresolvedImporterUOW{}, writer, references, key)
	if err != nil {
		t.Fatal(err)
	}

	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatalf("import unresolved survey history: %v", err)
	}
	if result != (SurveyUnresolvedHistoryImportResult{ImportedSubmissions: 1, ImportedAnswers: 2}) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(references.verified) != 3 || references.questionnaireCalls != 1 || references.customerCalls != 1 {
		t.Fatalf("verification/reference calls=%d/%d/%d", len(references.verified), references.questionnaireCalls, references.customerCalls)
	}
	if len(writer.submissionValues) != 1 || len(writer.answerValues) != 2 {
		t.Fatalf("writer values=%d/%d", len(writer.submissionValues), len(writer.answerValues))
	}
	submission := writer.submissionValues[0]
	if submission.SourceID != 201 || submission.QuestionnaireSourceID != 1 || submission.QuestionnaireID == nil || *submission.QuestionnaireID != questionnaireID || submission.CustomerID == nil || *submission.CustomerID != customerID {
		t.Fatalf("submission references changed: %+v", submission)
	}
	if submission.SourceKeyDigest != surveyUnresolvedRowBySource(t, archive.tables["public/questionnaire_submissions"], 201).SourceKeyHMAC ||
		submission.SourcePayloadDigest != surveyUnresolvedRowBySource(t, archive.tables["public/questionnaire_submissions"], 201).PayloadHMAC ||
		submission.SourceFieldDigest != surveyUnresolvedRowBySource(t, archive.tables["public/questionnaire_submissions"], 201).FieldHMAC {
		t.Fatal("submission archive bindings changed")
	}
	if submission.UnionIDDigest != surveyUnresolvedExpectedPrivateDigest(key, "unionid", []byte("union")) ||
		submission.FollowUserUserIDDigest != surveyUnresolvedExpectedPrivateDigest(key, "follow_user_userid", []byte("staff")) ||
		submission.CampaignIDDigest != surveyUnresolvedExpectedPrivateDigest(key, "campaign_id", nil) ||
		submission.StaffIDDigest != surveyUnresolvedExpectedPrivateDigest(key, "staff_id", nil) ||
		submission.RedirectURLDigest != surveyUnresolvedExpectedPrivateDigest(key, "redirect_url_snapshot", nil) ||
		submission.AssessmentResultDigest != surveyUnresolvedExpectedPrivateDigest(key, "assessment_result_snapshot", []byte(`{}`)) {
		t.Fatal("private source digests changed")
	}
	if submission.UnionIDDigest == submission.FollowUserUserIDDigest || !bytes.Equal(submission.FinalTags, []byte(`["tag-a"]`)) {
		t.Fatal("private-domain separation or JSON snapshot changed")
	}
	firstAnswer := writer.answerValues[0]
	if firstAnswer.SourceID != 301 || firstAnswer.SubmissionSourceID != 201 || firstAnswer.SubmissionID != 701 ||
		!bytes.Equal(firstAnswer.SelectedOptionIDs, []byte(`[100]`)) || !bytes.Equal(firstAnswer.SelectedOptionTexts, []byte(`["historical option"]`)) ||
		!bytes.Equal(firstAnswer.SelectedOptionScores, []byte(`[7]`)) || !bytes.Equal(firstAnswer.SelectedOptionTags, []byte(`[["tag-a"]]`)) {
		t.Fatalf("answer snapshot changed: %+v", firstAnswer)
	}

	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if result != (SurveyUnresolvedHistoryImportResult{Replayed: 3}) || len(writer.submissionValues) != 1 || len(writer.answerValues) != 2 {
		t.Fatalf("replay changed target writes/result: %+v writes=%d/%d", result, len(writer.submissionValues), len(writer.answerValues))
	}
}

func TestSurveyUnresolvedHistoryImporterFailsBeforeReferencesOrWritesOnSourceVerification(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	references := &surveyUnresolvedImporterReferences{verifyErr: errors.New("old quarantine missing")}
	writer := newSurveyUnresolvedImporterWriter()
	importer, err := NewSurveyUnresolvedHistoryImporter(surveyUnresolvedFixture(t, stamp, 999, []int64{100}), surveyUnresolvedImporterUOW{}, writer, references, bytes.Repeat([]byte{1}, sha256.Size))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); err == nil {
		t.Fatal("unverified old quarantine was accepted")
	}
	if len(references.verified) != 1 || references.questionnaireCalls != 0 || references.customerCalls != 0 || len(writer.submissionValues) != 0 || len(writer.answerValues) != 0 {
		t.Fatal("source verification failure reached references or writer")
	}
}

func TestSurveyUnresolvedHistoryImporterRejectsReceiptBeforeAnswers(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	writer := newSurveyUnresolvedImporterWriter()
	writer.badReceipt = true
	importer, err := NewSurveyUnresolvedHistoryImporter(surveyUnresolvedFixture(t, stamp, 999, []int64{100}), surveyUnresolvedImporterUOW{}, writer, &surveyUnresolvedImporterReferences{}, bytes.Repeat([]byte{2}, sha256.Size))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("bad receipt error=%v", err)
	}
	if len(writer.submissionValues) != 1 || len(writer.answerValues) != 0 {
		t.Fatal("answers were written from an unverified submission receipt")
	}
}

func TestSurveyUnresolvedJSONSnapshotPreservesNullAndPrivateDigestIsDomainSeparated(t *testing.T) {
	value, err := surveyUnresolvedJSONSnapshot([]byte(`{"selected_option_ids":null}`), "selected_option_ids")
	if err != nil || !bytes.Equal(value, []byte("null")) {
		t.Fatalf("JSON null was not preserved: %q %v", value, err)
	}
	key := bytes.Repeat([]byte{3}, sha256.Size)
	importer, err := NewSurveyUnresolvedHistoryImporter(surveyUnresolvedArchiveFake{}, surveyUnresolvedImporterUOW{}, newSurveyUnresolvedImporterWriter(), &surveyUnresolvedImporterReferences{}, key)
	if err != nil {
		t.Fatal(err)
	}
	if importer.privateDigest("unionid", []byte("same")) == importer.privateDigest("staff_id", []byte("same")) || importer.privateDigest("assessment_result_snapshot", []byte("null")) == ([sha256.Size]byte{}) {
		t.Fatal("private HMAC domain separation changed")
	}
}

func TestNewSurveyUnresolvedHistoryImporterRejectsInvalidSourceKey(t *testing.T) {
	if _, err := NewSurveyUnresolvedHistoryImporter(surveyUnresolvedArchiveFake{}, surveyUnresolvedImporterUOW{}, newSurveyUnresolvedImporterWriter(), &surveyUnresolvedImporterReferences{}, []byte("short")); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("invalid key error=%v", err)
	}
}

func surveyUnresolvedExpectedPrivateDigest(key []byte, field string, value []byte) [sha256.Size]byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(surveyUnresolvedPrivateDigestDomain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(field))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(value)
	var digest [sha256.Size]byte
	copy(digest[:], mac.Sum(nil))
	return digest
}
