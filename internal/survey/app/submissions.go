package app

import (
	"bytes"
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
	SubmissionDefaultLimit  int32 = 20
	SubmissionMaximumLimit  int32 = 100
	SubmissionMaximumOffset int32 = 1_000_000
	SubmissionExportLimit   int32 = 10_000
)

var ErrInvalidSubmissionPage = errors.New("invalid questionnaire submission page")

// SubmissionStore is the Survey-owned read seam for submission snapshots. It
// only ever reads Survey tables; identity values stay opaque snapshots.
type SubmissionStore interface {
	Results(context.Context, surveyport.ID) (surveyport.SubmissionResult, error)
	SubmissionOwnerExists(context.Context, surveyport.ID) (bool, error)
	CountSubmissions(context.Context, surveyport.ID) (int64, error)
	ListSubmissions(context.Context, surveyport.ID, int32, int32) ([]surveyport.Submission, error)
	ExportDefinition(context.Context, surveyport.ID) (string, []surveyport.SubmissionExportQuestion, error)
	ExportSubmissions(context.Context, surveyport.ID, int32) ([]surveyport.Submission, error)
}

type SubmissionService struct {
	uow   platformport.UnitOfWork
	store SubmissionStore
}

func NewSubmissionService(uow platformport.UnitOfWork, store SubmissionStore) *SubmissionService {
	return &SubmissionService{uow: uow, store: store}
}

func (s *SubmissionService) Results(ctx context.Context, questionnaireID surveyport.ID) (surveyport.SubmissionResult, error) {
	if !submissionReady(s) || questionnaireID < 1 {
		return surveyport.SubmissionResult{}, ErrInvalidSubmissionPage
	}
	var result surveyport.SubmissionResult
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		result, readErr = s.store.Results(tx, questionnaireID)
		return readErr
	})
	if err != nil {
		return surveyport.SubmissionResult{}, classifySubmission(err)
	}
	if !validSubmissionResult(result, questionnaireID) {
		return surveyport.SubmissionResult{}, ErrUnavailable
	}
	return cloneSubmissionResult(result), nil
}

func (s *SubmissionService) List(ctx context.Context, questionnaireID surveyport.ID, limit, offset int32) (surveyport.SubmissionPage, error) {
	if !submissionReady(s) || questionnaireID < 1 || !validSubmissionPageRequest(limit, offset) {
		return surveyport.SubmissionPage{}, ErrInvalidSubmissionPage
	}
	page := surveyport.SubmissionPage{Limit: limit, Offset: offset}
	err := s.uow.Within(ctx, func(tx context.Context) error {
		exists, readErr := s.store.SubmissionOwnerExists(tx, questionnaireID)
		if readErr != nil {
			return readErr
		}
		if !exists {
			return ErrNotFound
		}
		items, readErr := s.store.ListSubmissions(tx, questionnaireID, limit, offset)
		if readErr != nil {
			return readErr
		}
		total, readErr := s.store.CountSubmissions(tx, questionnaireID)
		if readErr != nil {
			return readErr
		}
		page.Items, page.Total = items, total
		return nil
	})
	if err != nil {
		return surveyport.SubmissionPage{}, classifySubmission(err)
	}
	if !validSubmissionPage(page, questionnaireID) {
		return surveyport.SubmissionPage{}, ErrUnavailable
	}
	return cloneSubmissionPage(page), nil
}

func (s *SubmissionService) Export(ctx context.Context, questionnaireID surveyport.ID) (surveyport.SubmissionCSVDownload, error) {
	if !submissionReady(s) || questionnaireID < 1 {
		return surveyport.SubmissionCSVDownload{}, ErrInvalidSubmissionPage
	}
	snapshot := surveyport.SubmissionExportSnapshot{QuestionnaireID: questionnaireID}
	err := s.uow.Within(ctx, func(tx context.Context) error {
		slug, questions, readErr := s.store.ExportDefinition(tx, questionnaireID)
		if readErr != nil {
			return readErr
		}
		submissions, readErr := s.store.ExportSubmissions(tx, questionnaireID, SubmissionExportLimit)
		if readErr != nil {
			return readErr
		}
		total, readErr := s.store.CountSubmissions(tx, questionnaireID)
		if readErr != nil {
			return readErr
		}
		snapshot.Slug, snapshot.Questions, snapshot.Submissions, snapshot.Total = slug, questions, submissions, total
		return nil
	})
	if err != nil {
		return surveyport.SubmissionCSVDownload{}, classifySubmission(err)
	}
	if !validExportSnapshot(snapshot, questionnaireID) {
		return surveyport.SubmissionCSVDownload{}, ErrUnavailable
	}
	return EncodeSubmissionCSV(snapshot)
}

// EncodeSubmissionCSV freezes the browser-download transport: UTF-8 BOM,
// CRLF records, formula-injection escaping on every cell, Asia/Shanghai
// timestamps, and definition-order question columns with stable duplicate
// suffixes. Snapshot-only questions keep their historical answers appended
// after the definition columns in first-seen submission order.
func EncodeSubmissionCSV(snapshot surveyport.SubmissionExportSnapshot) (surveyport.SubmissionCSVDownload, error) {
	if !validExportSnapshot(snapshot, snapshot.QuestionnaireID) {
		return surveyport.SubmissionCSVDownload{}, ErrUnavailable
	}
	fields, questionIDs := exportFields(snapshot.Questions, snapshot.Submissions)
	var out bytes.Buffer
	out.WriteString("\ufeff")
	writer := csv.NewWriter(&out)
	writer.UseCRLF = true
	if err := writer.Write(csvSafeRow(fields)); err != nil {
		return surveyport.SubmissionCSVDownload{}, ErrUnavailable
	}
	baseCount := len(fields) - len(questionIDs)
	for _, submission := range snapshot.Submissions {
		answers := exportAnswersByQuestion(submission.Answers)
		row := make([]string, 0, len(fields))
		for _, field := range fields[:baseCount] {
			row = append(row, exportBaseValue(submission, field))
		}
		for _, questionID := range questionIDs {
			row = append(row, answers[questionID])
		}
		if err := writer.Write(csvSafeRow(row)); err != nil {
			return surveyport.SubmissionCSVDownload{}, ErrUnavailable
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return surveyport.SubmissionCSVDownload{}, ErrUnavailable
	}
	filename := "questionnaire-" + safeExportSlug(snapshot.Slug, snapshot.QuestionnaireID) + "-submissions.csv"
	return surveyport.SubmissionCSVDownload{
		Filename:    filename,
		ContentType: "text/csv; charset=utf-8",
		Body:        out.Bytes(),
		Total:       snapshot.Total,
	}, nil
}

func safeExportSlug(slug string, id surveyport.ID) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(slug) {
		safe := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
		if safe {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(builder.String(), "-.")
	if value == "" {
		value = strconv.FormatInt(int64(id), 10)
	}
	if len(value) > 80 {
		value = strings.Trim(value[:80], "-.")
	}
	if value == "" {
		value = strconv.FormatInt(int64(id), 10)
	}
	return value
}

func validSubmissionPageRequest(limit, offset int32) bool {
	return limit >= 1 && limit <= SubmissionMaximumLimit && offset >= 0 && offset <= SubmissionMaximumOffset
}

func validSubmissionResult(result surveyport.SubmissionResult, questionnaireID surveyport.ID) bool {
	if result.QuestionnaireID != questionnaireID || result.SubmissionCount < 0 || math.IsNaN(result.AverageScore) || math.IsInf(result.AverageScore, 0) {
		return false
	}
	if result.SubmissionCount == 0 {
		return result.LatestSubmittedAt.IsZero() && result.AverageScore == 0
	}
	return !result.LatestSubmittedAt.IsZero()
}

func validSubmissionPage(page surveyport.SubmissionPage, questionnaireID surveyport.ID) bool {
	if page.Total < 0 || len(page.Items) > int(page.Limit) ||
		int64(page.Offset) >= page.Total && len(page.Items) != 0 ||
		int64(page.Offset)+int64(len(page.Items)) > page.Total {
		return false
	}
	for index, item := range page.Items {
		if !validSubmission(item, questionnaireID) || index > 0 && compareSubmission(page.Items[index-1], item) > 0 {
			return false
		}
	}
	return true
}

func validExportSnapshot(snapshot surveyport.SubmissionExportSnapshot, questionnaireID surveyport.ID) bool {
	if snapshot.QuestionnaireID != questionnaireID || snapshot.Total < int64(len(snapshot.Submissions)) ||
		len(snapshot.Submissions) > int(SubmissionExportLimit) {
		return false
	}
	seen := make(map[int64]bool, len(snapshot.Questions))
	for index, question := range snapshot.Questions {
		if question.ID < 1 || seen[question.ID] || strings.TrimSpace(question.Title) == "" || question.SortOrder < 0 ||
			index > 0 && compareExportQuestion(snapshot.Questions[index-1], question) > 0 {
			return false
		}
		seen[question.ID] = true
	}
	for index, item := range snapshot.Submissions {
		if !validSubmission(item, questionnaireID) || index > 0 && compareSubmission(snapshot.Submissions[index-1], item) > 0 {
			return false
		}
	}
	return true
}

func validSubmission(item surveyport.Submission, questionnaireID surveyport.ID) bool {
	if item.ID < 1 || item.QuestionnaireID != questionnaireID || item.SubmittedAt.IsZero() || item.CreatedAt.IsZero() ||
		math.IsNaN(item.TotalScore) || math.IsInf(item.TotalScore, 0) {
		return false
	}
	if !optional(item.RespondentKey, 200) || !optional(item.OpenID, 200) || !optional(item.UnionID, 200) ||
		!optional(item.ExternalUserID, 200) || !optional(item.CustomerName, 300) || !optional(item.FollowUserUserID, 200) ||
		!optional(item.MatchedBy, 50) || !optional(item.Mobile, 32) || !optional(item.SourceChannel, 100) ||
		!optional(item.CampaignID, 200) || !optional(item.StaffID, 200) || !optional(item.ResultToken, 200) ||
		!optional(item.RedirectURLSnapshot, 2000) {
		return false
	}
	for _, tag := range item.FinalTags {
		if !required(tag, 200) || strings.TrimSpace(tag) == "" {
			return false
		}
	}
	seen := make(map[int64]bool, len(item.Answers))
	for index, answer := range item.Answers {
		if answer.QuestionID < 1 || seen[answer.QuestionID] || !required(answer.QuestionTitle, 500) ||
			strings.TrimSpace(answer.QuestionTitle) == "" || !optional(answer.TextValue, 10_000) ||
			answer.SortOrder < 0 || index > 0 && compareAnswer(item.Answers[index-1], answer) > 0 {
			return false
		}
		for _, option := range answer.SelectedOptions {
			if option.OptionID < 1 || !required(option.OptionText, 500) || strings.TrimSpace(option.OptionText) == "" {
				return false
			}
		}
		switch answer.QuestionType {
		case surveyport.SingleChoice, surveyport.MultiChoice, surveyport.Textarea, surveyport.Mobile:
		default:
			return false
		}
		seen[answer.QuestionID] = true
	}
	return true
}

func compareSubmission(left, right surveyport.Submission) int {
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

func compareAnswer(left, right surveyport.SubmissionAnswer) int {
	if left.SortOrder != right.SortOrder {
		if left.SortOrder < right.SortOrder {
			return -1
		}
		return 1
	}
	if left.QuestionID < right.QuestionID {
		return -1
	}
	if left.QuestionID > right.QuestionID {
		return 1
	}
	return 0
}

func compareExportQuestion(left, right surveyport.SubmissionExportQuestion) int {
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

func exportFields(questions []surveyport.SubmissionExportQuestion, submissions []surveyport.Submission) ([]string, []int64) {
	base := []string{"submission_id", "submitted_at", "external_userid", "用户昵称", "unionid", "mobile", "score", "final_tags"}
	titleByID := make(map[int64]string, len(questions))
	order := make([]int64, 0, len(questions))
	for _, question := range questions {
		if question.ID < 1 || strings.TrimSpace(question.Title) == "" {
			continue
		}
		if _, exists := titleByID[question.ID]; !exists {
			titleByID[question.ID] = question.Title
			order = append(order, question.ID)
		}
	}
	for _, submission := range submissions {
		for _, answer := range submission.Answers {
			if answer.QuestionID < 1 || strings.TrimSpace(answer.QuestionTitle) == "" {
				continue
			}
			if _, exists := titleByID[answer.QuestionID]; !exists {
				titleByID[answer.QuestionID] = answer.QuestionTitle
				order = append(order, answer.QuestionID)
			}
		}
	}
	seenTitles := make(map[string]int, len(order))
	fields := append([]string(nil), base...)
	for _, id := range order {
		title := titleByID[id]
		seenTitles[title]++
		if seenTitles[title] > 1 {
			title += " (" + strconv.Itoa(seenTitles[title]) + ")"
		}
		fields = append(fields, title)
	}
	return fields, order
}

func exportBaseValue(item surveyport.Submission, field string) string {
	switch field {
	case "submission_id":
		return strconv.FormatInt(item.ID, 10)
	case "submitted_at":
		return shanghaiCSVTime(item.SubmittedAt)
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

func exportAnswersByQuestion(answers []surveyport.SubmissionAnswer) map[int64]string {
	result := make(map[int64]string, len(answers))
	for _, answer := range answers {
		if answer.QuestionID < 1 {
			continue
		}
		if answer.QuestionType == surveyport.Textarea || answer.QuestionType == surveyport.Mobile {
			result[answer.QuestionID] = answer.TextValue
			continue
		}
		values := make([]string, 0, len(answer.SelectedOptions)+1)
		for _, option := range answer.SelectedOptions {
			if text := strings.TrimSpace(option.OptionText); text != "" {
				values = append(values, text)
			}
		}
		if text := strings.TrimSpace(answer.TextValue); text != "" {
			if len(values) > 0 {
				values[len(values)-1] += "：" + text
			} else {
				values = append(values, text)
			}
		}
		result[answer.QuestionID] = strings.Join(values, "、")
	}
	return result
}

var shanghaiZone = time.FixedZone("Asia/Shanghai", 8*60*60)

func shanghaiCSVTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(shanghaiZone).Format("2006-01-02 15:04:05")
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

func classifySubmission(err error) error {
	switch {
	case errors.Is(err, ErrInvalidSubmissionPage), errors.Is(err, ErrNotFound):
		return err
	default:
		return ErrUnavailable
	}
}

func submissionReady(s *SubmissionService) bool {
	return s != nil && s.uow != nil && s.store != nil
}

func cloneSubmissionResult(value surveyport.SubmissionResult) surveyport.SubmissionResult {
	result := value
	result.Rules = append([]surveyport.ScoreRule{}, value.Rules...)
	return result
}

func cloneSubmissionPage(value surveyport.SubmissionPage) surveyport.SubmissionPage {
	result := value
	result.Items = cloneSubmissions(value.Items)
	return result
}

func cloneSubmissions(values []surveyport.Submission) []surveyport.Submission {
	result := make([]surveyport.Submission, len(values))
	for index, value := range values {
		result[index] = value
		result[index].FinalTags = append([]string{}, value.FinalTags...)
		result[index].Answers = make([]surveyport.SubmissionAnswer, len(value.Answers))
		for answerIndex, answer := range value.Answers {
			result[index].Answers[answerIndex] = answer
			result[index].Answers[answerIndex].SelectedOptions = append([]surveyport.SubmissionAnswerOption{}, answer.SelectedOptions...)
		}
	}
	return result
}
