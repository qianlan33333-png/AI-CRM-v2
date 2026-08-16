package app

import (
	"context"
	"encoding/csv"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	SubmissionAnalysisDefaultLimit  int32 = 20
	SubmissionAnalysisMaximumLimit  int32 = 10_000
	SubmissionAnalysisMaximumOffset int32 = 1_000_000
	SubmissionExportLimit           int32 = 10_000
)

var (
	ErrInvalidSubmissionPage = errors.New("invalid questionnaire submission page")
	ErrIdentityConflict      = errors.New("questionnaire submission identity conflict")
	ErrSubmissionForbidden   = errors.New("questionnaire submission read forbidden")
)

type SubmissionAnalysisPermission string

const (
	SubmissionAnalysisAdminRead    SubmissionAnalysisPermission = "admin_read"
	SubmissionAnalysisCustomerRead SubmissionAnalysisPermission = "read_customer"
)

// SubmissionAnalysisAuthorizer is the leaf's future Session -> Actor seam.
// It freezes the legacy permission distinction: summaries and detail require
// admin_read; the PII-bearing browser export requires read_customer.
type SubmissionAnalysisAuthorizer interface {
	AuthorizeSubmissionAnalysis(context.Context, SubmissionAnalysisPermission) error
}

// SubmissionAnalysisStore is deliberately an app-local seam. The target
// submission and answer-snapshot migration does not exist yet, so this
// candidate must not provide a database implementation that pretends it does.
// A future Survey-owned store adapter must return only one questionnaire's
// snapshots and fail closed on an unresolved OneID conflict.
type SubmissionAnalysisStore interface {
	Results(context.Context, surveyport.ID) (SubmissionResults, error)
	ListSubmissions(context.Context, surveyport.ID, int32, int32) (SubmissionPage, error)
	ExportSnapshot(context.Context, surveyport.ID, int32) (QuestionnaireExportSnapshot, error)
}

type SubmissionAnalysisService struct {
	uow        platformport.UnitOfWork
	store      SubmissionAnalysisStore
	authorizer SubmissionAnalysisAuthorizer
}

func NewSubmissionAnalysisService(uow platformport.UnitOfWork, store SubmissionAnalysisStore, authorizer SubmissionAnalysisAuthorizer) *SubmissionAnalysisService {
	return &SubmissionAnalysisService{uow: uow, store: store, authorizer: authorizer}
}

type SubmissionResults struct {
	QuestionnaireID   surveyport.ID
	SubmissionCount   int64
	LatestSubmittedAt time.Time
	AverageScore      float64
	Rules             []SubmissionScoreRule
}

type SubmissionScoreRule struct {
	ID              int64
	QuestionnaireID surveyport.ID
	MinimumScore    *float64
	MaximumScore    *float64
	TagCodes        []string
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Submission is the Survey-owned submission snapshot. Identity values are
// intentionally values from a future Identity read port, never a direct query
// into another domain's table.
type Submission struct {
	ID                  int64
	QuestionnaireID     surveyport.ID
	RespondentKey       string
	OpenID              string
	UnionID             string
	ExternalUserID      string
	CustomerName        string
	FollowUserUserID    string
	MatchedBy           string
	Mobile              string
	SourceChannel       string
	CampaignID          string
	StaffID             string
	TotalScore          float64
	FinalTags           []string
	ResultToken         string
	RedirectURLSnapshot string
	SubmittedAt         time.Time
	CreatedAt           time.Time
	Answers             map[string]SubmissionAnswer
	AnswerSnapshots     []SubmissionAnswerSnapshot
}

type SubmissionAnswer struct {
	SelectedOptionIDs []string
	OtherText         string
	TextValue         string
}

type SubmissionAnswerSnapshot struct {
	QuestionID                  string
	QuestionType                string
	QuestionTitleSnapshot       string
	SelectedOptionIDs           []string
	SelectedOptionTextsSnapshot []string
	TextValue                   string
}

type SubmissionPage struct {
	Items  []Submission
	Total  int64
	Limit  int32
	Offset int32
}

type QuestionnaireExportSnapshot struct {
	QuestionnaireID surveyport.ID
	Slug            string
	Questions       []SubmissionQuestion
	Submissions     []Submission
	Total           int64
	SourceStatus    string
}

type SubmissionQuestion struct {
	ID        string
	Title     string
	SortOrder int
}

type CSVDownload struct {
	Filename    string
	ContentType string
	Headers     map[string]string
	Body        []byte
	Fields      []string
	Total       int64
}

func (s *SubmissionAnalysisService) Results(ctx context.Context, questionnaireID surveyport.ID) (SubmissionResults, error) {
	if !submissionAnalysisReady(s) || ctx == nil || ctx.Err() != nil || questionnaireID < 1 {
		return SubmissionResults{}, ErrInvalidSubmissionPage
	}
	if err := s.authorize(ctx, SubmissionAnalysisAdminRead); err != nil {
		return SubmissionResults{}, err
	}
	var result SubmissionResults
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		result, readErr = s.store.Results(tx, questionnaireID)
		return readErr
	})
	if err != nil {
		return SubmissionResults{}, classifySubmissionAnalysis(err)
	}
	if !validSubmissionResults(result, questionnaireID) {
		return SubmissionResults{}, ErrUnavailable
	}
	return cloneSubmissionResults(result), nil
}

func (s *SubmissionAnalysisService) List(ctx context.Context, questionnaireID surveyport.ID, limit, offset int32) (SubmissionPage, error) {
	if !submissionAnalysisReady(s) || ctx == nil || ctx.Err() != nil || questionnaireID < 1 || !validSubmissionPageRequest(limit, offset) {
		return SubmissionPage{}, ErrInvalidSubmissionPage
	}
	if err := s.authorize(ctx, SubmissionAnalysisAdminRead); err != nil {
		return SubmissionPage{}, err
	}
	var page SubmissionPage
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		page, readErr = s.store.ListSubmissions(tx, questionnaireID, limit, offset)
		return readErr
	})
	if err != nil {
		return SubmissionPage{}, classifySubmissionAnalysis(err)
	}
	if !validSubmissionPage(page, questionnaireID, limit, offset) {
		return SubmissionPage{}, ErrUnavailable
	}
	return cloneSubmissionPage(page), nil
}

func (s *SubmissionAnalysisService) Export(ctx context.Context, questionnaireID surveyport.ID) (CSVDownload, error) {
	if !submissionAnalysisReady(s) || ctx == nil || ctx.Err() != nil || questionnaireID < 1 {
		return CSVDownload{}, ErrInvalidSubmissionPage
	}
	if err := s.authorize(ctx, SubmissionAnalysisCustomerRead); err != nil {
		return CSVDownload{}, err
	}
	var snapshot QuestionnaireExportSnapshot
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		snapshot, readErr = s.store.ExportSnapshot(tx, questionnaireID, SubmissionExportLimit)
		return readErr
	})
	if err != nil {
		return CSVDownload{}, classifySubmissionAnalysis(err)
	}
	if !validExportSnapshot(snapshot, questionnaireID) {
		return CSVDownload{}, ErrUnavailable
	}
	download, err := EncodeQuestionnaireSubmissionCSV(snapshot)
	if err != nil {
		return CSVDownload{}, ErrUnavailable
	}
	return download, nil
}

// EncodeQuestionnaireSubmissionCSV preserves the legacy browser-download
// transport: UTF-8 BOM, text/csv, attachment filename and CRLF records. The
// formula prefix is an explicit R2 safety hardening requested for the new
// candidate; it applies to every cell, including question headers.
func EncodeQuestionnaireSubmissionCSV(snapshot QuestionnaireExportSnapshot) (CSVDownload, error) {
	if !validExportSnapshot(snapshot, snapshot.QuestionnaireID) {
		return CSVDownload{}, ErrUnavailable
	}
	fields, questionHeaders := exportFields(snapshot.Questions, snapshot.Submissions)
	var out strings.Builder
	writer := csv.NewWriter(&out)
	writer.UseCRLF = true
	if err := writer.Write(csvSafeRow(fields)); err != nil {
		return CSVDownload{}, err
	}
	for _, submission := range snapshot.Submissions {
		row := make([]string, 0, len(fields))
		answers := exportAnswersByQuestion(submission.AnswerSnapshots)
		for _, field := range fields[:len(fields)-len(questionHeaders)] {
			row = append(row, exportBaseValue(submission, field))
		}
		for _, question := range questionHeaders {
			row = append(row, answers[question.ID])
		}
		if err := writer.Write(csvSafeRow(row)); err != nil {
			return CSVDownload{}, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return CSVDownload{}, err
	}
	slug := strings.TrimSpace(snapshot.Slug)
	if slug == "" {
		slug = "questionnaire-" + strconv.FormatInt(int64(snapshot.QuestionnaireID), 10)
	}
	filename := strings.ReplaceAll("questionnaire-"+slug+"-submissions.csv", `"`, "")
	sourceStatus := strings.TrimSpace(snapshot.SourceStatus)
	if sourceStatus == "" {
		sourceStatus = "next_command"
	}
	return CSVDownload{
		Filename:    filename,
		ContentType: "text/csv; charset=utf-8",
		Headers: map[string]string{
			"Content-Disposition":   `attachment; filename="` + filename + `"`,
			"X-AICRM-Route-Owner":   "ai_crm_next",
			"X-AICRM-Source-Status": sourceStatus,
			"X-AICRM-Fallback-Used": "false",
		},
		Body:   append([]byte("\ufeff"), []byte(out.String())...),
		Fields: append([]string(nil), fields...),
		Total:  snapshot.Total,
	}, nil
}

func validSubmissionPageRequest(limit, offset int32) bool {
	return limit >= 1 && limit <= SubmissionAnalysisMaximumLimit && offset >= 0 && offset <= SubmissionAnalysisMaximumOffset
}

func validSubmissionResults(result SubmissionResults, questionnaireID surveyport.ID) bool {
	if result.QuestionnaireID != questionnaireID || result.SubmissionCount < 0 || math.IsNaN(result.AverageScore) || math.IsInf(result.AverageScore, 0) {
		return false
	}
	if result.SubmissionCount == 0 && (!result.LatestSubmittedAt.IsZero() || result.AverageScore != 0) {
		return false
	}
	for index, rule := range result.Rules {
		if rule.QuestionnaireID != questionnaireID || rule.ID < 1 || rule.SortOrder < 0 || (index > 0 && compareRule(result.Rules[index-1], rule) > 0) {
			return false
		}
	}
	return true
}

func validSubmissionPage(page SubmissionPage, questionnaireID surveyport.ID, limit, offset int32) bool {
	if page.Total < 0 || page.Total < int64(len(page.Items)) || page.Limit != limit || page.Offset != offset || len(page.Items) > int(limit) || int64(offset) > page.Total && len(page.Items) != 0 {
		return false
	}
	for index, item := range page.Items {
		if !validSubmission(item, questionnaireID) || (index > 0 && compareSubmission(page.Items[index-1], item) > 0) {
			return false
		}
	}
	return true
}

func validExportSnapshot(snapshot QuestionnaireExportSnapshot, questionnaireID surveyport.ID) bool {
	if snapshot.QuestionnaireID != questionnaireID || snapshot.Total < int64(len(snapshot.Submissions)) || len(snapshot.Submissions) > int(SubmissionExportLimit) {
		return false
	}
	for index, question := range snapshot.Questions {
		if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Title) == "" || question.SortOrder < 0 || (index > 0 && compareQuestion(snapshot.Questions[index-1], question) > 0) {
			return false
		}
	}
	for index, item := range snapshot.Submissions {
		if !validSubmission(item, questionnaireID) || (index > 0 && compareSubmission(snapshot.Submissions[index-1], item) > 0) {
			return false
		}
	}
	return true
}

func validSubmission(item Submission, questionnaireID surveyport.ID) bool {
	if item.ID < 1 || item.QuestionnaireID != questionnaireID || item.SubmittedAt.IsZero() || math.IsNaN(item.TotalScore) || math.IsInf(item.TotalScore, 0) {
		return false
	}
	for _, answer := range item.AnswerSnapshots {
		if strings.TrimSpace(answer.QuestionID) == "" || strings.TrimSpace(answer.QuestionType) == "" {
			return false
		}
	}
	return true
}

func compareRule(left, right SubmissionScoreRule) int {
	if left.SortOrder != right.SortOrder {
		if left.SortOrder < right.SortOrder {
			return -1
		}
		return 1
	}
	if left.ID < right.ID {
		return -1
	}
	if left.ID > right.ID {
		return 1
	}
	return 0
}

func compareQuestion(left, right SubmissionQuestion) int {
	if left.SortOrder != right.SortOrder {
		if left.SortOrder < right.SortOrder {
			return -1
		}
		return 1
	}
	return strings.Compare(left.ID, right.ID)
}

// compareSubmission returns a negative number when left is correctly before
// right in submitted_at DESC, id DESC order.
func compareSubmission(left, right Submission) int {
	if !left.SubmittedAt.Equal(right.SubmittedAt) {
		if left.SubmittedAt.After(right.SubmittedAt) {
			return -1
		}
		return 1
	}
	if left.ID > right.ID {
		return -1
	}
	if left.ID < right.ID {
		return 1
	}
	return 0
}

func exportFields(questions []SubmissionQuestion, submissions []Submission) ([]string, []SubmissionQuestion) {
	base := []string{"submission_id", "submitted_at", "external_userid", "用户昵称", "unionid", "mobile", "score", "final_tags"}
	titleByID := make(map[string]string, len(questions))
	order := make([]string, 0, len(questions))
	for _, question := range questions {
		id, title := strings.TrimSpace(question.ID), strings.TrimSpace(question.Title)
		if id == "" || title == "" {
			continue
		}
		if _, exists := titleByID[id]; !exists {
			titleByID[id] = title
			order = append(order, id)
		}
	}
	for _, submission := range submissions {
		for _, answer := range submission.AnswerSnapshots {
			id, title := strings.TrimSpace(answer.QuestionID), strings.TrimSpace(answer.QuestionTitleSnapshot)
			if id == "" || title == "" {
				continue
			}
			if _, exists := titleByID[id]; !exists {
				titleByID[id] = title
				order = append(order, id)
			}
		}
	}
	seenTitles := make(map[string]int, len(order))
	questionHeaders := make([]SubmissionQuestion, 0, len(order))
	for _, id := range order {
		title := titleByID[id]
		seenTitles[title]++
		if seenTitles[title] > 1 {
			title += " (" + strconv.Itoa(seenTitles[title]) + ")"
		}
		questionHeaders = append(questionHeaders, SubmissionQuestion{ID: id, Title: title})
	}
	fields := append(append([]string(nil), base...), make([]string, len(questionHeaders))...)
	for index, question := range questionHeaders {
		fields[len(base)+index] = question.Title
	}
	return fields, questionHeaders
}

func exportBaseValue(item Submission, field string) string {
	switch field {
	case "submission_id":
		return strconv.FormatInt(item.ID, 10)
	case "submitted_at":
		return beijingCSVTime(item.SubmittedAt)
	case "external_userid":
		return item.ExternalUserID
	case "用户昵称":
		return item.CustomerName
	case "unionid":
		return item.UnionID
	case "mobile":
		return item.Mobile
	case "score":
		return strconv.FormatFloat(item.TotalScore, 'f', -1, 64)
	case "final_tags":
		return strings.Join(item.FinalTags, "、")
	default:
		return ""
	}
}

func exportAnswersByQuestion(snapshots []SubmissionAnswerSnapshot) map[string]string {
	answers := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		id := strings.TrimSpace(snapshot.QuestionID)
		if id == "" {
			continue
		}
		if snapshot.QuestionType == "textarea" || snapshot.QuestionType == "mobile" {
			answers[id] = strings.TrimSpace(snapshot.TextValue)
			continue
		}
		values := make([]string, 0, len(snapshot.SelectedOptionTextsSnapshot)+1)
		for _, option := range snapshot.SelectedOptionTextsSnapshot {
			if option = strings.TrimSpace(option); option != "" {
				values = append(values, option)
			}
		}
		text := strings.TrimSpace(snapshot.TextValue)
		if text != "" && len(values) > 0 {
			values[len(values)-1] += "：" + text
		} else if text != "" {
			values = append(values, text)
		}
		answers[id] = strings.Join(values, "、")
	}
	return answers
}

func beijingCSVTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05")
}

func csvSafeRow(row []string) []string {
	result := make([]string, len(row))
	for index, value := range row {
		result[index] = csvSafeCell(value)
	}
	return result
}

func csvSafeCell(value string) string {
	if value == "" {
		return ""
	}
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func classifySubmissionAnalysis(err error) error {
	switch {
	case errors.Is(err, ErrInvalidSubmissionPage), errors.Is(err, ErrNotFound), errors.Is(err, ErrIdentityConflict), errors.Is(err, ErrSubmissionForbidden):
		return err
	default:
		return ErrUnavailable
	}
}

func submissionAnalysisReady(s *SubmissionAnalysisService) bool {
	return s != nil && s.uow != nil && s.store != nil && s.authorizer != nil
}

func (s *SubmissionAnalysisService) authorize(ctx context.Context, permission SubmissionAnalysisPermission) error {
	if err := s.authorizer.AuthorizeSubmissionAnalysis(ctx, permission); err != nil {
		if errors.Is(err, ErrSubmissionForbidden) {
			return ErrSubmissionForbidden
		}
		return ErrUnavailable
	}
	return nil
}

func cloneSubmissionResults(value SubmissionResults) SubmissionResults {
	result := value
	result.Rules = append([]SubmissionScoreRule(nil), value.Rules...)
	for index := range result.Rules {
		result.Rules[index].TagCodes = append([]string(nil), value.Rules[index].TagCodes...)
		if value.Rules[index].MinimumScore != nil {
			minimum := *value.Rules[index].MinimumScore
			result.Rules[index].MinimumScore = &minimum
		}
		if value.Rules[index].MaximumScore != nil {
			maximum := *value.Rules[index].MaximumScore
			result.Rules[index].MaximumScore = &maximum
		}
	}
	return result
}

func cloneSubmissionPage(value SubmissionPage) SubmissionPage {
	result := value
	result.Items = cloneSubmissions(value.Items)
	return result
}

func cloneSubmissions(values []Submission) []Submission {
	result := make([]Submission, len(values))
	for index, value := range values {
		result[index] = value
		result[index].FinalTags = append([]string(nil), value.FinalTags...)
		result[index].AnswerSnapshots = append([]SubmissionAnswerSnapshot(nil), value.AnswerSnapshots...)
		for answerIndex := range result[index].AnswerSnapshots {
			result[index].AnswerSnapshots[answerIndex].SelectedOptionIDs = append([]string(nil), value.AnswerSnapshots[answerIndex].SelectedOptionIDs...)
			result[index].AnswerSnapshots[answerIndex].SelectedOptionTextsSnapshot = append([]string(nil), value.AnswerSnapshots[answerIndex].SelectedOptionTextsSnapshot...)
		}
		if value.Answers != nil {
			result[index].Answers = make(map[string]SubmissionAnswer, len(value.Answers))
			for key, answer := range value.Answers {
				answer.SelectedOptionIDs = append([]string(nil), answer.SelectedOptionIDs...)
				result[index].Answers[key] = answer
			}
		}
	}
	return result
}
