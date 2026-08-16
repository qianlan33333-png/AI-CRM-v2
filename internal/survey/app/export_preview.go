package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	ExportPreviewLimit = 3

	ExportPreviewPermission = "manage_questionnaire"
	ExportPreviewStatus     = "export_preview_planned"
	ExportPreviewEffectType = "questionnaire.export.preview"
)

var (
	ErrInvalidExportPreview = errors.New("invalid questionnaire export preview")
	ErrExportPreviewDenied  = errors.New("questionnaire export preview forbidden")
)

var defaultExportPreviewFields = []string{"submission_id", "external_userid", "answers", "created_at"}

// ExportPreviewAuthorizer is the future human-session -> Actor seam. A
// central route must bind it only after authenticating a human session,
// enforcing global manage_questionnaire, and validating CSRF for this POST.
type ExportPreviewAuthorizer interface {
	AuthorizeExportPreview(context.Context, string) error
}

// ExportPreviewStore is deliberately an app-local seam. The target submission
// and answer-snapshot schema is not yet approved, so this candidate cannot
// ship a database adapter or migration. Its receipt API stores only command
// digests and state; it must never persist a sample's raw answers or identity.
type ExportPreviewStore interface {
	ReserveExportPreview(context.Context, ExportPreviewReservation) (ExportPreviewReceipt, bool, error)
	CompleteExportPreview(context.Context, int64, time.Time) (ExportPreviewReceipt, error)
	ReadExportPreview(context.Context, surveyport.ID, int32) (ExportPreviewSnapshot, error)
}

type ExportPreviewService struct {
	uow        platformport.UnitOfWork
	store      ExportPreviewStore
	authorizer ExportPreviewAuthorizer
	now        func() time.Time
}

func NewExportPreviewService(uow platformport.UnitOfWork, store ExportPreviewStore, authorizer ExportPreviewAuthorizer) *ExportPreviewService {
	return &ExportPreviewService{uow: uow, store: store, authorizer: authorizer, now: time.Now}
}

type ExportPreviewCommand struct {
	QuestionnaireID surveyport.ID
	ActorID         int64
	IdempotencyKey  string
	Fields          []string
}

type ExportPreviewReservation struct {
	ActorScope    string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type ExportPreviewReceipt struct {
	ID            int64
	ActorScope    string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	State         string
}

type ExportPreviewSnapshot struct {
	QuestionnaireID surveyport.ID
	EstimatedCount  int64
	Submissions     []ExportPreviewSubmission
}

// ExportPreviewSubmission contains a Survey-owned projection plus values from
// a future legal Identity read port. It is confined to a sensitive admin
// response and is never logged or copied into an idempotency receipt.
type ExportPreviewSubmission struct {
	ID               int64
	QuestionnaireID  surveyport.ID
	ExternalUserID   string
	OpenID           string
	UnionID          string
	Mobile           string
	RespondentKey    string
	CustomerName     string
	FollowUserUserID string
	Answers          json.RawMessage
	CreatedAt        time.Time
	SubmittedAt      time.Time
}

type ExportPreviewResult struct {
	WriteModelStatus string
	ExportPreview    ExportPreviewProjection
	SideEffectPlan   ExportPreviewPlan
}

type ExportPreviewProjection struct {
	Fields         []string
	EstimatedCount int64
	MaskedSample   []map[string]any
	FileCreated    bool
}

type ExportPreviewPlan struct {
	EffectType               string
	AdapterName              string
	AdapterMode              string
	TargetType               string
	TargetID                 string
	Status                   string
	RiskLevel                string
	RequiresApproval         bool
	PayloadSummary           ExportPreviewPlanSummary
	RealExternalCallExecuted bool
}

type ExportPreviewPlanSummary struct {
	QuestionnaireID surveyport.ID
	Fields          []string
	EstimatedCount  int64
}

// DecodeExportPreviewFields is the internal DTO boundary that a future HTTP
// handler will call after decoding JSON. Only a JSON object is accepted;
// non-object input maps to the frozen HTTP 400 path. Omitted or empty fields
// retain the frozen legacy default order.
func DecodeExportPreviewFields(value any) ([]string, error) {
	payload, ok := value.(map[string]any)
	if !ok || payload == nil {
		return nil, ErrInvalidExportPreview
	}
	raw, present := payload["fields"]
	if !present || raw == nil {
		return clonePreviewFields(defaultExportPreviewFields), nil
	}
	fields, ok := raw.([]any)
	if !ok {
		return nil, ErrInvalidExportPreview
	}
	if len(fields) == 0 {
		return clonePreviewFields(defaultExportPreviewFields), nil
	}
	result := make([]string, len(fields))
	for index, field := range fields {
		name, isString := field.(string)
		if !isString || !validPreviewField(name) {
			return nil, ErrInvalidExportPreview
		}
		result[index] = name
	}
	return result, nil
}

func (s *ExportPreviewService) Preview(ctx context.Context, input ExportPreviewCommand) (ExportPreviewResult, error) {
	command, err := normalizeExportPreviewCommand(input)
	if err != nil {
		return ExportPreviewResult{}, err
	}
	if !exportPreviewReady(s) || ctx == nil || ctx.Err() != nil {
		return ExportPreviewResult{}, ErrUnavailable
	}
	if err := s.authorize(ctx); err != nil {
		return ExportPreviewResult{}, err
	}
	now := s.now().UTC()
	if now.IsZero() {
		return ExportPreviewResult{}, ErrUnavailable
	}
	payloadDigest, err := exportPreviewPayloadDigest(command)
	if err != nil {
		return ExportPreviewResult{}, ErrInvalidExportPreview
	}
	reservation := ExportPreviewReservation{
		ActorScope:    fmt.Sprintf("admin:%d", command.ActorID),
		KeyDigest:     sha256.Sum256([]byte(command.IdempotencyKey)),
		PayloadDigest: payloadDigest,
		CreatedAt:     now,
	}
	var result ExportPreviewResult
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveExportPreview(tx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !previewReceiptMatches(receipt, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned && receipt.State != "completed" {
			return ErrUnavailable
		}
		snapshot, readErr := s.store.ReadExportPreview(tx, command.QuestionnaireID, ExportPreviewLimit)
		if readErr != nil {
			return readErr
		}
		if !validExportPreviewSnapshot(snapshot, command.QuestionnaireID) {
			return ErrUnavailable
		}
		result = exportPreviewResult(snapshot, command.Fields)
		if !owned {
			return nil
		}
		completed, completeErr := s.store.CompleteExportPreview(tx, receipt.ID, now)
		if completeErr != nil || !previewReceiptMatches(completed, reservation) || completed.State != "completed" {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return ExportPreviewResult{}, classifyExportPreview(err)
	}
	return cloneExportPreviewResult(result), nil
}

func normalizeExportPreviewCommand(input ExportPreviewCommand) (ExportPreviewCommand, error) {
	if input.QuestionnaireID < 1 || input.ActorID < 1 || !validKey(input.IdempotencyKey) {
		return ExportPreviewCommand{}, ErrInvalidExportPreview
	}
	if len(input.Fields) == 0 {
		input.Fields = clonePreviewFields(defaultExportPreviewFields)
		return input, nil
	}
	if len(input.Fields) > 100 {
		return ExportPreviewCommand{}, ErrInvalidExportPreview
	}
	for _, field := range input.Fields {
		if !validPreviewField(field) {
			return ExportPreviewCommand{}, ErrInvalidExportPreview
		}
	}
	input.Fields = clonePreviewFields(input.Fields)
	return input, nil
}

func exportPreviewPayloadDigest(command ExportPreviewCommand) ([32]byte, error) {
	payload, err := json.Marshal(struct {
		QuestionnaireID surveyport.ID
		Fields          []string
	}{QuestionnaireID: command.QuestionnaireID, Fields: command.Fields})
	return sha256.Sum256(payload), err
}

func validPreviewField(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 200
}

func validExportPreviewSnapshot(snapshot ExportPreviewSnapshot, questionnaireID surveyport.ID) bool {
	if snapshot.QuestionnaireID != questionnaireID || snapshot.EstimatedCount < int64(len(snapshot.Submissions)) || len(snapshot.Submissions) > ExportPreviewLimit {
		return false
	}
	for index, submission := range snapshot.Submissions {
		if submission.ID < 1 || submission.QuestionnaireID != questionnaireID || submission.SubmittedAt.IsZero() || !jsonObject(submission.Answers) || (index > 0 && comparePreviewSubmission(snapshot.Submissions[index-1], submission) > 0) {
			return false
		}
	}
	return true
}

func comparePreviewSubmission(left, right ExportPreviewSubmission) int {
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

func jsonObject(raw json.RawMessage) bool {
	if !json.Valid(raw) {
		return false
	}
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func exportPreviewResult(snapshot ExportPreviewSnapshot, fields []string) ExportPreviewResult {
	masked := make([]map[string]any, len(snapshot.Submissions))
	for index, submission := range snapshot.Submissions {
		masked[index] = maskedPreviewSubmission(submission, fields)
	}
	return ExportPreviewResult{
		WriteModelStatus: ExportPreviewStatus,
		ExportPreview: ExportPreviewProjection{
			Fields: fields, EstimatedCount: snapshot.EstimatedCount, MaskedSample: masked, FileCreated: false,
		},
		SideEffectPlan: ExportPreviewPlan{
			EffectType: ExportPreviewEffectType, AdapterName: "storage", AdapterMode: "real_blocked", TargetType: "questionnaire",
			TargetID: fmt.Sprintf("%d", snapshot.QuestionnaireID), Status: "planned", RiskLevel: "medium", RequiresApproval: true,
			PayloadSummary:           ExportPreviewPlanSummary{QuestionnaireID: snapshot.QuestionnaireID, Fields: clonePreviewFields(fields), EstimatedCount: snapshot.EstimatedCount},
			RealExternalCallExecuted: false,
		},
	}
}

func maskedPreviewSubmission(submission ExportPreviewSubmission, fields []string) map[string]any {
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		switch field {
		case "submission_id":
			result[field] = fmt.Sprintf("%d", submission.ID)
		case "answers":
			result[field] = append(json.RawMessage(nil), submission.Answers...)
		case "created_at":
			if submission.CreatedAt.IsZero() {
				result[field] = submission.SubmittedAt
			} else {
				result[field] = submission.CreatedAt
			}
		case "submitted_at":
			result[field] = submission.SubmittedAt
		case "external_userid":
			result[field] = maskTopLevelIdentity(submission.ExternalUserID)
		case "openid":
			result[field] = maskTopLevelIdentity(submission.OpenID)
		case "unionid":
			result[field] = maskTopLevelIdentity(submission.UnionID)
		case "mobile":
			result[field] = maskTopLevelIdentity(submission.Mobile)
		case "respondent_key":
			result[field] = maskTopLevelIdentity(submission.RespondentKey)
		case "customer_name":
			result[field] = maskTopLevelIdentity(submission.CustomerName)
		case "follow_user_userid":
			result[field] = maskTopLevelIdentity(submission.FollowUserUserID)
		default:
			result[field] = nil
		}
	}
	return result
}

func maskTopLevelIdentity(value string) string {
	if value == "" {
		return ""
	}
	return "masked"
}

func previewReceiptMatches(receipt ExportPreviewReceipt, reservation ExportPreviewReservation) bool {
	return receipt.ID > 0 && receipt.ActorScope == reservation.ActorScope && subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 && (receipt.State == "in_progress" || receipt.State == "completed")
}

func (s *ExportPreviewService) authorize(ctx context.Context) error {
	if err := s.authorizer.AuthorizeExportPreview(ctx, ExportPreviewPermission); err != nil {
		if errors.Is(err, ErrExportPreviewDenied) {
			return ErrExportPreviewDenied
		}
		return ErrUnavailable
	}
	return nil
}

func exportPreviewReady(s *ExportPreviewService) bool {
	return s != nil && s.uow != nil && s.store != nil && s.authorizer != nil && s.now != nil
}

func classifyExportPreview(err error) error {
	switch {
	case errors.Is(err, ErrInvalidExportPreview), errors.Is(err, ErrExportPreviewDenied), errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict):
		return err
	default:
		return ErrUnavailable
	}
}

func clonePreviewFields(fields []string) []string {
	return append([]string(nil), fields...)
}

func cloneExportPreviewResult(value ExportPreviewResult) ExportPreviewResult {
	result := value
	result.ExportPreview.Fields = clonePreviewFields(value.ExportPreview.Fields)
	result.ExportPreview.MaskedSample = make([]map[string]any, len(value.ExportPreview.MaskedSample))
	for index, row := range value.ExportPreview.MaskedSample {
		result.ExportPreview.MaskedSample[index] = make(map[string]any, len(row))
		for key, item := range row {
			if raw, ok := item.(json.RawMessage); ok {
				result.ExportPreview.MaskedSample[index][key] = append(json.RawMessage(nil), raw...)
				continue
			}
			result.ExportPreview.MaskedSample[index][key] = item
		}
	}
	result.SideEffectPlan.PayloadSummary.Fields = clonePreviewFields(value.SideEffectPlan.PayloadSummary.Fields)
	return result
}
