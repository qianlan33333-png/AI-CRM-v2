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

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	QuestionnaireOperationsTemplate = "questionnaire_operations"
	OperationsPanelCompletion       = "completion"
	OperationsPanelExternalPush     = "external_push"
	ExternalPushTestQueued          = "queued"

	CapabilityAdminRead            = "admin_read"
	CapabilityManageQuestionnaire  = "manage_questionnaire"
	operationSaveCompletion        = "operations_completion_save"
	operationSaveExternalPush      = "operations_external_push_save"
	operationQueueExternalPushTest = "operations_external_push_test_queue"
)

var (
	ErrInvalidOperations       = errors.New("invalid questionnaire operations command")
	ErrOperationsForbidden     = errors.New("questionnaire operations forbidden")
	ErrExternalPushTestBlocked = errors.New("questionnaire external push test blocked")
)

// OperationsAccess is a route-facing authorization requirement. The future M
// adapter must construct it from a human session and validate CSRF before it
// calls either command method; this service never accepts a caller-supplied
// boolean that could bypass that check.
type OperationsAccess struct {
	Actor        int64
	Capability   string
	CSRFRequired bool
}

// OperationsAuthorizer is deliberately local to the survey application. It
// keeps the future HTTP/session adapter out of this non-central candidate.
type OperationsAuthorizer interface {
	RequireQuestionnaireOperations(context.Context, OperationsAccess) error
}

// CompletionTargetReader is the only seam for the canonical navigation target
// boundary. TargetID is an opaque reference: no redirect URL is persisted or
// resolved by this service.
type CompletionTargetReader interface {
	ReadCompletionTarget(context.Context, string) (CompletionTarget, error)
}

type CompletionTarget struct {
	ID                     string
	Available              bool
	RequiresChannelBinding bool
}

// ChannelReader is a read-only survey-local seam. Its future adapter must use
// the Channel read port; direct reads of another domain's tables are forbidden.
type ChannelReader interface {
	ReadChannelForCompletion(context.Context, int64) (CompletionChannel, error)
}

type CompletionChannel struct {
	ID         int64
	Selectable bool
}

// CompletionConfiguration preserves the product split without reimplementing
// redirect logic. NavigationTargetID can represent the canonical QR or mini
// program target; ChannelID is used only when that target requires a binding.
type CompletionConfiguration struct {
	NavigationTargetID string `json:"navigation_target_id,omitempty"`
	ChannelID          int64  `json:"channel_id,omitempty"`
}

// ExternalPushConfiguration contains an opaque configuration reference only.
// It never contains a provider URL, token, webhook payload, retry policy, or
// historical effect data.
type ExternalPushConfiguration struct {
	Enabled                bool   `json:"enabled"`
	ConfigurationReference string `json:"configuration_reference,omitempty"`
}

type OperationsProjection struct {
	QuestionnaireID surveyport.ID             `json:"questionnaire_id"`
	Completion      CompletionConfiguration   `json:"completion"`
	ExternalPush    ExternalPushConfiguration `json:"external_push"`
}

// OperationsPageCarrier is the data-only carrier for LEGACY-API-0067. The M
// replay binds it to the existing operations UI template; this candidate adds
// no HTML, JavaScript, CSS, redirects, or log routes.
type OperationsPageCarrier struct {
	TemplateKey       string                `json:"template_key"`
	QuestionnaireID   surveyport.ID         `json:"questionnaire_id"`
	State             string                `json:"state"`
	PlaceholderReason string                `json:"placeholder_reason,omitempty"`
	Panels            []string              `json:"panels"`
	Operations        *OperationsProjection `json:"operations,omitempty"`
	TestPushAvailable bool                  `json:"test_push_available"`
	PushLogsAvailable bool                  `json:"push_logs_available"`
}

type OperationsReservation struct {
	ActorScope    string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type OperationsReceipt struct {
	ID             int64
	Operation      string
	ActorScope     string
	KeyDigest      [32]byte
	PayloadDigest  [32]byte
	State          string
	ResultSnapshot json.RawMessage
}

// OperationsStore is a target-schema seam. A future M replay supplies the
// survey-owned repository and migration; no current table or waterline is
// claimed by this candidate.
type OperationsStore interface {
	ReadOperations(context.Context, surveyport.ID) (OperationsProjection, error)
	SaveCompletionOperations(context.Context, surveyport.ID, CompletionConfiguration, time.Time) (OperationsProjection, error)
	SaveExternalPushOperations(context.Context, surveyport.ID, ExternalPushConfiguration, time.Time) (OperationsProjection, error)
	ReserveOperations(context.Context, string, OperationsReservation) (OperationsReceipt, bool, error)
	CompleteOperations(context.Context, int64, json.RawMessage, time.Time) (OperationsReceipt, error)
}

// QueuedExternalPushTest deliberately has no endpoint, credential, payload, or
// provider client. A future worker resolves the owned configuration only after
// the separate EXTERNAL_GATE is authorized.
type QueuedExternalPushTest struct {
	QuestionnaireID    surveyport.ID
	Actor              int64
	OperationReceiptID int64
	CreatedAt          time.Time
}

type ExternalPushTestQueue interface {
	QueueExternalPushTest(context.Context, QueuedExternalPushTest) (ExternalPushTestResult, error)
}

// ExternalPushTestResult is the exact safe success boundary for LEGACY-API-0439:
// queued is not accepted, dispatched, provider-tested, or provider-successful.
type ExternalPushTestResult struct {
	TestRunID              string `json:"test_run_id"`
	Status                 string `json:"status"`
	AttemptCount           int32  `json:"attempt_count"`
	SideEffectExecuted     bool   `json:"side_effect_executed"`
	ProviderResultReceived bool   `json:"provider_result_received"`
	UnknownAfterDispatch   bool   `json:"unknown_after_dispatch"`
}

type SaveCompletionOperationsCommand struct {
	QuestionnaireID surveyport.ID
	Actor           int64
	IdempotencyKey  string
	Completion      CompletionConfiguration
}

type SaveExternalPushOperationsCommand struct {
	QuestionnaireID surveyport.ID
	Actor           int64
	IdempotencyKey  string
	ExternalPush    ExternalPushConfiguration
}

type QueueExternalPushTestCommand struct {
	QuestionnaireID surveyport.ID
	Actor           int64
	IdempotencyKey  string
}

type OperationsService struct {
	uow        platformport.UnitOfWork
	store      OperationsStore
	authorizer OperationsAuthorizer
	targets    CompletionTargetReader
	channels   ChannelReader
	queue      ExternalPushTestQueue
	events     eventport.Appender
	now        func() time.Time
}

func NewOperationsService(uow platformport.UnitOfWork, store OperationsStore, authorizer OperationsAuthorizer, targets CompletionTargetReader, channels ChannelReader, queue ExternalPushTestQueue, events eventport.Appender) *OperationsService {
	return &OperationsService{uow: uow, store: store, authorizer: authorizer, targets: targets, channels: channels, queue: queue, events: events, now: time.Now}
}

// GetOperations is the read model for LEGACY-API-0436.
func (s *OperationsService) GetOperations(ctx context.Context, questionnaireID surveyport.ID, actor int64) (OperationsProjection, error) {
	if !operationsReady(s) {
		return OperationsProjection{}, ErrUnavailable
	}
	if questionnaireID < 1 {
		return OperationsProjection{}, ErrNotFound
	}
	if err := s.authorize(ctx, actor, CapabilityAdminRead, false); err != nil {
		return OperationsProjection{}, err
	}
	return s.readOperations(ctx, questionnaireID)
}

// OperationsPage is the data-only carrier for LEGACY-API-0067. Missing and
// unavailable data render the legacy-compatible placeholder state, while API
// reads retain their respective 404/503 errors.
func (s *OperationsService) OperationsPage(ctx context.Context, questionnaireID surveyport.ID, actor int64) (OperationsPageCarrier, error) {
	if !operationsReady(s) {
		return OperationsPageCarrier{}, ErrUnavailable
	}
	if err := s.authorize(ctx, actor, CapabilityAdminRead, false); err != nil {
		return OperationsPageCarrier{}, err
	}
	page := operationsPage(questionnaireID)
	if questionnaireID < 1 {
		page.State, page.PlaceholderReason = "placeholder", "not_found"
		return page, nil
	}
	projection, err := s.readOperations(ctx, questionnaireID)
	if err == nil {
		page.State, page.Operations = "ready", projectionPointer(projection)
		return page, nil
	}
	switch {
	case errors.Is(err, ErrNotFound):
		page.PlaceholderReason = "not_found"
	case errors.Is(err, ErrAssessmentUnavailable):
		page.PlaceholderReason = "assessment_unavailable"
	default:
		page.PlaceholderReason = "unavailable"
	}
	page.State = "placeholder"
	return page, nil
}

func (s *OperationsService) SaveCompletionOperations(ctx context.Context, input SaveCompletionOperationsCommand) (OperationsProjection, error) {
	if !operationsReady(s) {
		return OperationsProjection{}, ErrUnavailable
	}
	if err := validateCompletionCommand(input); err != nil {
		return OperationsProjection{}, err
	}
	if err := s.authorize(ctx, input.Actor, CapabilityManageQuestionnaire, true); err != nil {
		return OperationsProjection{}, err
	}
	payload, err := json.Marshal(struct {
		QuestionnaireID surveyport.ID           `json:"questionnaire_id"`
		Completion      CompletionConfiguration `json:"completion"`
	}{input.QuestionnaireID, input.Completion})
	if err != nil {
		return OperationsProjection{}, ErrInvalidOperations
	}
	return s.saveCompletion(ctx, input, sha256.Sum256(payload))
}

func (s *OperationsService) SaveExternalPushOperations(ctx context.Context, input SaveExternalPushOperationsCommand) (OperationsProjection, error) {
	if !operationsReady(s) {
		return OperationsProjection{}, ErrUnavailable
	}
	if err := validateExternalPushCommand(input); err != nil {
		return OperationsProjection{}, err
	}
	if err := s.authorize(ctx, input.Actor, CapabilityManageQuestionnaire, true); err != nil {
		return OperationsProjection{}, err
	}
	payload, err := json.Marshal(struct {
		QuestionnaireID surveyport.ID             `json:"questionnaire_id"`
		ExternalPush    ExternalPushConfiguration `json:"external_push"`
	}{input.QuestionnaireID, input.ExternalPush})
	if err != nil {
		return OperationsProjection{}, ErrInvalidOperations
	}
	return s.saveExternalPush(ctx, input, sha256.Sum256(payload))
}

func (s *OperationsService) QueueExternalPushTest(ctx context.Context, input QueueExternalPushTestCommand) (ExternalPushTestResult, error) {
	if !operationsReady(s) {
		return ExternalPushTestResult{}, ErrUnavailable
	}
	if input.QuestionnaireID < 1 || input.Actor < 1 || !validKey(input.IdempotencyKey) {
		return ExternalPushTestResult{}, ErrInvalidOperations
	}
	if err := s.authorize(ctx, input.Actor, CapabilityManageQuestionnaire, true); err != nil {
		return ExternalPushTestResult{}, err
	}
	now := s.now().UTC()
	if now.IsZero() {
		return ExternalPushTestResult{}, ErrUnavailable
	}
	payloadDigest := sha256.Sum256([]byte(fmt.Sprintf("%d", input.QuestionnaireID)))
	reservation := newOperationsReservation(input.Actor, input.IdempotencyKey, payloadDigest, now)
	var result ExternalPushTestResult
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveOperations(tx, operationQueueExternalPushTest, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !operationsReceiptMatches(receipt, operationQueueExternalPushTest, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeExternalPushTestResult(receipt.ResultSnapshot, &result) || !validQueuedExternalPushTest(result) {
				return ErrUnavailable
			}
			return nil
		}
		projection, readErr := s.store.ReadOperations(tx, input.QuestionnaireID)
		if readErr != nil {
			return readErr
		}
		if !validOperationsProjection(projection, input.QuestionnaireID) {
			return ErrUnavailable
		}
		if !projection.ExternalPush.Enabled || !validOpaqueReference(projection.ExternalPush.ConfigurationReference) {
			return ErrExternalPushTestBlocked
		}
		result, reserveErr = s.queue.QueueExternalPushTest(tx, QueuedExternalPushTest{QuestionnaireID: input.QuestionnaireID, Actor: input.Actor, OperationReceiptID: receipt.ID, CreatedAt: now})
		if reserveErr != nil {
			return reserveErr
		}
		if !validQueuedExternalPushTest(result) {
			return ErrExternalPushTestBlocked
		}
		if reserveErr = appendSurveyEvent(tx, s.events, eventport.EvSurveyUpdated, input.QuestionnaireID, input.Actor, input.IdempotencyKey, now, "external_push_test_queued"); reserveErr != nil {
			return reserveErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteOperations(tx, receipt.ID, snapshot, now)
		if completeErr != nil || !operationsReceiptMatches(completed, operationQueueExternalPushTest, reservation) || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return ExternalPushTestResult{}, classifyOperations(err)
	}
	return cloneExternalPushTestResult(result), nil
}

func (s *OperationsService) saveCompletion(ctx context.Context, input SaveCompletionOperationsCommand, payloadDigest [32]byte) (OperationsProjection, error) {
	now := s.now().UTC()
	if now.IsZero() {
		return OperationsProjection{}, ErrUnavailable
	}
	reservation := newOperationsReservation(input.Actor, input.IdempotencyKey, payloadDigest, now)
	var result OperationsProjection
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveOperations(tx, operationSaveCompletion, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !operationsReceiptMatches(receipt, operationSaveCompletion, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeOperationsProjection(receipt.ResultSnapshot, &result) || !validOperationsProjection(result, input.QuestionnaireID) {
				return ErrUnavailable
			}
			return nil
		}
		if reserveErr = s.validateCompletionConfiguration(tx, input.Completion); reserveErr != nil {
			return reserveErr
		}
		result, reserveErr = s.store.SaveCompletionOperations(tx, input.QuestionnaireID, cloneCompletionConfiguration(input.Completion), now)
		if reserveErr != nil {
			return reserveErr
		}
		if !validOperationsProjection(result, input.QuestionnaireID) || !sameCompletion(result.Completion, input.Completion) {
			return ErrUnavailable
		}
		if reserveErr = appendSurveyEvent(tx, s.events, eventport.EvSurveyUpdated, input.QuestionnaireID, input.Actor, input.IdempotencyKey, now, "operations_completion_saved"); reserveErr != nil {
			return reserveErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteOperations(tx, receipt.ID, snapshot, now)
		if completeErr != nil || !operationsReceiptMatches(completed, operationSaveCompletion, reservation) || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return OperationsProjection{}, classifyOperations(err)
	}
	return cloneOperationsProjection(result), nil
}

func (s *OperationsService) saveExternalPush(ctx context.Context, input SaveExternalPushOperationsCommand, payloadDigest [32]byte) (OperationsProjection, error) {
	now := s.now().UTC()
	if now.IsZero() {
		return OperationsProjection{}, ErrUnavailable
	}
	reservation := newOperationsReservation(input.Actor, input.IdempotencyKey, payloadDigest, now)
	var result OperationsProjection
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveOperations(tx, operationSaveExternalPush, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !operationsReceiptMatches(receipt, operationSaveExternalPush, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeOperationsProjection(receipt.ResultSnapshot, &result) || !validOperationsProjection(result, input.QuestionnaireID) {
				return ErrUnavailable
			}
			return nil
		}
		result, reserveErr = s.store.SaveExternalPushOperations(tx, input.QuestionnaireID, cloneExternalPushConfiguration(input.ExternalPush), now)
		if reserveErr != nil {
			return reserveErr
		}
		if !validOperationsProjection(result, input.QuestionnaireID) || !sameExternalPush(result.ExternalPush, input.ExternalPush) {
			return ErrUnavailable
		}
		if reserveErr = appendSurveyEvent(tx, s.events, eventport.EvSurveyUpdated, input.QuestionnaireID, input.Actor, input.IdempotencyKey, now, "operations_external_push_saved"); reserveErr != nil {
			return reserveErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteOperations(tx, receipt.ID, snapshot, now)
		if completeErr != nil || !operationsReceiptMatches(completed, operationSaveExternalPush, reservation) || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return OperationsProjection{}, classifyOperations(err)
	}
	return cloneOperationsProjection(result), nil
}

func (s *OperationsService) readOperations(ctx context.Context, questionnaireID surveyport.ID) (OperationsProjection, error) {
	var result OperationsProjection
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		result, readErr = s.store.ReadOperations(tx, questionnaireID)
		return readErr
	})
	if err != nil {
		return OperationsProjection{}, classifyOperations(err)
	}
	if !validOperationsProjection(result, questionnaireID) {
		return OperationsProjection{}, ErrUnavailable
	}
	return cloneOperationsProjection(result), nil
}

func (s *OperationsService) authorize(ctx context.Context, actor int64, capability string, csrf bool) error {
	if actor < 1 || s.authorizer == nil {
		return ErrOperationsForbidden
	}
	if err := s.authorizer.RequireQuestionnaireOperations(ctx, OperationsAccess{Actor: actor, Capability: capability, CSRFRequired: csrf}); err != nil {
		return ErrOperationsForbidden
	}
	return nil
}

func (s *OperationsService) validateCompletionConfiguration(ctx context.Context, value CompletionConfiguration) error {
	if value.NavigationTargetID == "" {
		if value.ChannelID != 0 {
			return ErrInvalidOperations
		}
		return nil
	}
	target, err := s.targets.ReadCompletionTarget(ctx, value.NavigationTargetID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrInvalidOperations
		}
		return ErrUnavailable
	}
	if target.ID != value.NavigationTargetID || !target.Available {
		return ErrUnavailable
	}
	if target.RequiresChannelBinding != (value.ChannelID > 0) {
		return ErrInvalidOperations
	}
	if value.ChannelID == 0 {
		return nil
	}
	channel, err := s.channels.ReadChannelForCompletion(ctx, value.ChannelID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrInvalidOperations
		}
		return ErrUnavailable
	}
	if channel.ID != value.ChannelID || !channel.Selectable {
		return ErrInvalidOperations
	}
	return nil
}

func validateCompletionCommand(input SaveCompletionOperationsCommand) error {
	if input.QuestionnaireID < 1 || input.Actor < 1 || !validKey(input.IdempotencyKey) || !validCompletionShape(input.Completion) {
		return ErrInvalidOperations
	}
	return nil
}

func validateExternalPushCommand(input SaveExternalPushOperationsCommand) error {
	if input.QuestionnaireID < 1 || input.Actor < 1 || !validKey(input.IdempotencyKey) || !validExternalPushConfiguration(input.ExternalPush) {
		return ErrInvalidOperations
	}
	return nil
}

func validOperationsProjection(value OperationsProjection, questionnaireID surveyport.ID) bool {
	return value.QuestionnaireID == questionnaireID && questionnaireID > 0 && validCompletionShape(value.Completion) && validExternalPushConfiguration(value.ExternalPush)
}

func validCompletionShape(value CompletionConfiguration) bool {
	if value.NavigationTargetID == "" {
		return value.ChannelID == 0
	}
	return value.ChannelID >= 0 && validOpaqueReference(value.NavigationTargetID)
}

func validExternalPushConfiguration(value ExternalPushConfiguration) bool {
	return value.Enabled && validOpaqueReference(value.ConfigurationReference) || !value.Enabled && value.ConfigurationReference == ""
}

func validQueuedExternalPushTest(value ExternalPushTestResult) bool {
	return validOpaqueReference(value.TestRunID) && value.Status == ExternalPushTestQueued && value.AttemptCount == 0 && !value.SideEffectExecuted && !value.ProviderResultReceived && !value.UnknownAfterDispatch
}

func validOpaqueReference(value string) bool {
	if !required(value, 128) || value != strings.TrimSpace(value) || strings.Contains(value, "://") {
		return false
	}
	for _, runeValue := range value {
		if runeValue >= 'a' && runeValue <= 'z' || runeValue >= 'A' && runeValue <= 'Z' || runeValue >= '0' && runeValue <= '9' || strings.ContainsRune("-_.:", runeValue) {
			continue
		}
		return false
	}
	return true
}

func newOperationsReservation(actor int64, key string, payload [32]byte, now time.Time) OperationsReservation {
	return OperationsReservation{ActorScope: fmt.Sprintf("admin:%d", actor), KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: payload, CreatedAt: now}
}

func operationsReceiptMatches(receipt OperationsReceipt, operation string, reservation OperationsReservation) bool {
	return receipt.ID > 0 && receipt.Operation == operation && receipt.ActorScope == reservation.ActorScope && subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1
}

func decodeOperationsProjection(raw json.RawMessage, result *OperationsProjection) bool {
	return result != nil && len(raw) != 0 && json.Unmarshal(raw, result) == nil
}

func decodeExternalPushTestResult(raw json.RawMessage, result *ExternalPushTestResult) bool {
	return result != nil && len(raw) != 0 && json.Unmarshal(raw, result) == nil
}

func operationsPage(questionnaireID surveyport.ID) OperationsPageCarrier {
	return OperationsPageCarrier{TemplateKey: QuestionnaireOperationsTemplate, QuestionnaireID: questionnaireID, Panels: []string{OperationsPanelCompletion, OperationsPanelExternalPush}, TestPushAvailable: true, PushLogsAvailable: true}
}

func projectionPointer(value OperationsProjection) *OperationsProjection {
	copyValue := cloneOperationsProjection(value)
	return &copyValue
}

func cloneOperationsProjection(value OperationsProjection) OperationsProjection {
	value.Completion = cloneCompletionConfiguration(value.Completion)
	value.ExternalPush = cloneExternalPushConfiguration(value.ExternalPush)
	return value
}

func cloneCompletionConfiguration(value CompletionConfiguration) CompletionConfiguration {
	return value
}
func cloneExternalPushConfiguration(value ExternalPushConfiguration) ExternalPushConfiguration {
	return value
}
func cloneExternalPushTestResult(value ExternalPushTestResult) ExternalPushTestResult { return value }
func sameCompletion(left, right CompletionConfiguration) bool                         { return left == right }
func sameExternalPush(left, right ExternalPushConfiguration) bool                     { return left == right }

func operationsReady(s *OperationsService) bool {
	return s != nil && s.uow != nil && s.store != nil && s.authorizer != nil && s.targets != nil && s.channels != nil && s.queue != nil && s.events != nil && s.now != nil
}

func classifyOperations(err error) error {
	switch {
	case errors.Is(err, ErrInvalidOperations), errors.Is(err, ErrOperationsForbidden), errors.Is(err, ErrExternalPushTestBlocked), errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict), errors.Is(err, ErrAssessmentUnavailable):
		return err
	default:
		return ErrUnavailable
	}
}
