package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	operationsSaveCompletion        = "operations_completion_save"
	operationsSaveExternalPush      = "operations_external_push_save"
	operationsQueueExternalPushTest = "operations_external_push_test_queue"

	ExternalPushTestQueued = "queued"

	ExternalPushLogDefaultLimit  int32 = 50
	ExternalPushLogMaximumLimit  int32 = 100
	ExternalPushLogMaximumOffset int32 = 1_000_000
)

var (
	ErrInvalidOperations         = errors.New("invalid questionnaire operations command")
	ErrExternalPushNotConfigured = errors.New("questionnaire external push is not locally configured")
)

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

// OperationsStore contains only questionnaire-owned local facts. In
// particular, it has no provider, webhook, River, or external-effect API.
type OperationsStore interface {
	ReadOperations(context.Context, surveyport.ID) (surveyport.OperationsProjection, error)
	SaveCompletionOperations(context.Context, surveyport.ID, surveyport.CompletionOperations, time.Time) error
	SaveExternalPushOperations(context.Context, surveyport.ID, surveyport.ExternalPushOperations, time.Time) error
	ReserveOperations(context.Context, string, OperationsReservation) (OperationsReceipt, bool, error)
	CompleteOperations(context.Context, int64, json.RawMessage, time.Time) (OperationsReceipt, error)
	CreateQueuedExternalPushTest(context.Context, surveyport.ID, int64, time.Time) (int64, error)
	ReadExternalPushTest(context.Context, surveyport.ID, int64) (surveyport.ExternalPushTest, error)
	CountExternalPushTests(context.Context, *surveyport.ID) (int64, error)
	ListExternalPushTests(context.Context, *surveyport.ID, int32, int32) ([]surveyport.ExternalPushTest, error)
}

type OperationsService struct {
	uow    platformport.UnitOfWork
	store  OperationsStore
	events eventport.Appender
	now    func() time.Time
}

func NewOperationsService(uow platformport.UnitOfWork, store OperationsStore, events eventport.Appender) *OperationsService {
	return &OperationsService{uow: uow, store: store, events: events, now: time.Now}
}

func (s *OperationsService) Get(ctx context.Context, questionnaireID surveyport.ID) (surveyport.OperationsProjection, error) {
	if !operationsReady(s) || ctx == nil || ctx.Err() != nil {
		return surveyport.OperationsProjection{}, ErrUnavailable
	}
	if questionnaireID < 1 {
		return surveyport.OperationsProjection{}, ErrNotFound
	}
	var projection surveyport.OperationsProjection
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		projection, readErr = s.store.ReadOperations(tx, questionnaireID)
		return readErr
	})
	if err != nil {
		return surveyport.OperationsProjection{}, classifyOperations(err)
	}
	if !validOperationsProjection(projection, questionnaireID) {
		return surveyport.OperationsProjection{}, ErrUnavailable
	}
	return cloneOperationsProjection(projection), nil
}

func (s *OperationsService) SaveCompletion(ctx context.Context, command surveyport.SaveCompletionOperationsCommand) (surveyport.OperationsProjection, error) {
	if !operationsReady(s) || ctx == nil || ctx.Err() != nil {
		return surveyport.OperationsProjection{}, ErrUnavailable
	}
	if !validSaveCompletionCommand(command) {
		return surveyport.OperationsProjection{}, ErrInvalidOperations
	}
	payload, err := json.Marshal(struct {
		QuestionnaireID surveyport.ID                   `json:"questionnaire_id"`
		Completion      surveyport.CompletionOperations `json:"completion"`
	}{QuestionnaireID: command.QuestionnaireID, Completion: command.Completion})
	if err != nil {
		return surveyport.OperationsProjection{}, ErrInvalidOperations
	}
	return s.save(ctx, operationsSaveCompletion, command.QuestionnaireID, command.Actor, command.IdempotencyKey, sha256.Sum256(payload), func(tx context.Context, now time.Time) (surveyport.OperationsProjection, error) {
		if err := s.store.SaveCompletionOperations(tx, command.QuestionnaireID, command.Completion, now); err != nil {
			return surveyport.OperationsProjection{}, err
		}
		projection, err := s.store.ReadOperations(tx, command.QuestionnaireID)
		if err != nil {
			return surveyport.OperationsProjection{}, err
		}
		if !validOperationsProjection(projection, command.QuestionnaireID) || projection.Completion != command.Completion {
			return surveyport.OperationsProjection{}, ErrUnavailable
		}
		return projection, nil
	})
}

func (s *OperationsService) SaveExternalPush(ctx context.Context, command surveyport.SaveExternalPushOperationsCommand) (surveyport.OperationsProjection, error) {
	if !operationsReady(s) || ctx == nil || ctx.Err() != nil {
		return surveyport.OperationsProjection{}, ErrUnavailable
	}
	if !validSaveExternalPushCommand(command) {
		return surveyport.OperationsProjection{}, ErrInvalidOperations
	}
	payload, err := json.Marshal(struct {
		QuestionnaireID surveyport.ID                     `json:"questionnaire_id"`
		ExternalPush    surveyport.ExternalPushOperations `json:"external_push"`
	}{QuestionnaireID: command.QuestionnaireID, ExternalPush: command.ExternalPush})
	if err != nil {
		return surveyport.OperationsProjection{}, ErrInvalidOperations
	}
	return s.save(ctx, operationsSaveExternalPush, command.QuestionnaireID, command.Actor, command.IdempotencyKey, sha256.Sum256(payload), func(tx context.Context, now time.Time) (surveyport.OperationsProjection, error) {
		if err := s.store.SaveExternalPushOperations(tx, command.QuestionnaireID, command.ExternalPush, now); err != nil {
			return surveyport.OperationsProjection{}, err
		}
		projection, err := s.store.ReadOperations(tx, command.QuestionnaireID)
		if err != nil {
			return surveyport.OperationsProjection{}, err
		}
		if !validOperationsProjection(projection, command.QuestionnaireID) || projection.ExternalPush != command.ExternalPush {
			return surveyport.OperationsProjection{}, ErrUnavailable
		}
		return projection, nil
	})
}

func (s *OperationsService) QueueExternalPushTest(ctx context.Context, command surveyport.QueueExternalPushTestCommand) (surveyport.ExternalPushTest, error) {
	if !operationsReady(s) || ctx == nil || ctx.Err() != nil {
		return surveyport.ExternalPushTest{}, ErrUnavailable
	}
	if !validQueueExternalPushTestCommand(command) {
		return surveyport.ExternalPushTest{}, ErrInvalidOperations
	}
	now := s.now().UTC()
	if now.IsZero() {
		return surveyport.ExternalPushTest{}, ErrUnavailable
	}
	payload, err := json.Marshal(struct {
		QuestionnaireID surveyport.ID `json:"questionnaire_id"`
	}{QuestionnaireID: command.QuestionnaireID})
	if err != nil {
		return surveyport.ExternalPushTest{}, ErrInvalidOperations
	}
	reservation := newOperationsReservation(command.Actor, command.IdempotencyKey, sha256.Sum256(payload), now)
	var result surveyport.ExternalPushTest
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveOperations(tx, operationsQueueExternalPushTest, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !operationsReceiptMatches(receipt, operationsQueueExternalPushTest, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeExternalPushTest(receipt.ResultSnapshot, &result) || !validExternalPushTest(result, command.QuestionnaireID) {
				return ErrUnavailable
			}
			return nil
		}
		projection, readErr := s.store.ReadOperations(tx, command.QuestionnaireID)
		if readErr != nil {
			return readErr
		}
		if !validOperationsProjection(projection, command.QuestionnaireID) {
			return ErrUnavailable
		}
		if !projection.ExternalPush.Enabled || !validOpaqueReference(projection.ExternalPush.ConfigurationReference) {
			return ErrExternalPushNotConfigured
		}
		testRunID, createErr := s.store.CreateQueuedExternalPushTest(tx, command.QuestionnaireID, receipt.ID, now)
		if createErr != nil {
			return createErr
		}
		result, readErr = s.store.ReadExternalPushTest(tx, command.QuestionnaireID, testRunID)
		if readErr != nil {
			return readErr
		}
		if !validExternalPushTest(result, command.QuestionnaireID) {
			return ErrUnavailable
		}
		if eventErr := appendSurveyEvent(tx, s.events, eventport.EvSurveyUpdated, command.QuestionnaireID, command.Actor, command.IdempotencyKey, now, "operations_external_push_test_queued"); eventErr != nil {
			return eventErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteOperations(tx, receipt.ID, snapshot, now)
		if completeErr != nil || !operationsReceiptMatches(completed, operationsQueueExternalPushTest, reservation) || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return surveyport.ExternalPushTest{}, classifyOperations(err)
	}
	return result, nil
}

// ListExternalPushLogs returns only local queue facts. A nil questionnaire ID
// means the global log; a non-nil value scopes the page and proves ownership.
func (s *OperationsService) ListExternalPushLogs(ctx context.Context, questionnaireID *surveyport.ID, limit, offset int32) (surveyport.ExternalPushLogPage, error) {
	if !operationsReady(s) || ctx == nil || ctx.Err() != nil || limit < 1 || limit > ExternalPushLogMaximumLimit || offset < 0 || offset > ExternalPushLogMaximumOffset {
		return surveyport.ExternalPushLogPage{}, ErrInvalidOperations
	}
	if questionnaireID != nil && *questionnaireID < 1 {
		return surveyport.ExternalPushLogPage{}, ErrInvalidOperations
	}
	page := surveyport.ExternalPushLogPage{Limit: limit, Offset: offset, Items: []surveyport.ExternalPushTest{}, LocalOnly: true}
	err := s.uow.Within(ctx, func(tx context.Context) error {
		if questionnaireID != nil {
			projection, readErr := s.store.ReadOperations(tx, *questionnaireID)
			if readErr != nil {
				return readErr
			}
			if !validOperationsProjection(projection, *questionnaireID) {
				return ErrUnavailable
			}
		}
		var readErr error
		page.Total, readErr = s.store.CountExternalPushTests(tx, questionnaireID)
		if readErr != nil {
			return readErr
		}
		page.Items, readErr = s.store.ListExternalPushTests(tx, questionnaireID, limit, offset)
		return readErr
	})
	if err != nil {
		return surveyport.ExternalPushLogPage{}, classifyOperations(err)
	}
	if !validExternalPushLogPage(page, questionnaireID) {
		return surveyport.ExternalPushLogPage{}, ErrUnavailable
	}
	page.HasMore = int64(page.Offset)+int64(len(page.Items)) < page.Total
	return cloneExternalPushLogPage(page), nil
}

func (s *OperationsService) save(
	ctx context.Context,
	operation string,
	questionnaireID surveyport.ID,
	actor int64,
	key string,
	payloadDigest [32]byte,
	write func(context.Context, time.Time) (surveyport.OperationsProjection, error),
) (surveyport.OperationsProjection, error) {
	now := s.now().UTC()
	if now.IsZero() {
		return surveyport.OperationsProjection{}, ErrUnavailable
	}
	reservation := newOperationsReservation(actor, key, payloadDigest, now)
	var result surveyport.OperationsProjection
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveOperations(tx, operation, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !operationsReceiptMatches(receipt, operation, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeOperationsProjection(receipt.ResultSnapshot, &result) || !validOperationsProjection(result, questionnaireID) {
				return ErrUnavailable
			}
			return nil
		}
		result, reserveErr = write(tx, now)
		if reserveErr != nil {
			return reserveErr
		}
		if !validOperationsProjection(result, questionnaireID) {
			return ErrUnavailable
		}
		if eventErr := appendSurveyEvent(tx, s.events, eventport.EvSurveyUpdated, questionnaireID, actor, key, now, operation); eventErr != nil {
			return eventErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteOperations(tx, receipt.ID, snapshot, now)
		if completeErr != nil || !operationsReceiptMatches(completed, operation, reservation) || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return surveyport.OperationsProjection{}, classifyOperations(err)
	}
	return cloneOperationsProjection(result), nil
}

func newOperationsReservation(actor int64, key string, payloadDigest [32]byte, now time.Time) OperationsReservation {
	return OperationsReservation{
		ActorScope: fmt.Sprintf("admin:%d", actor), KeyDigest: sha256.Sum256([]byte(key)),
		PayloadDigest: payloadDigest, CreatedAt: now,
	}
}

func operationsReceiptMatches(receipt OperationsReceipt, operation string, reservation OperationsReservation) bool {
	return receipt.ID > 0 && receipt.Operation == operation && receipt.ActorScope == reservation.ActorScope &&
		subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 &&
		(receipt.State == "in_progress" || receipt.State == "completed")
}

func validSaveCompletionCommand(command surveyport.SaveCompletionOperationsCommand) bool {
	return command.QuestionnaireID > 0 && command.Actor > 0 && validKey(command.IdempotencyKey) && validCompletionOperations(command.Completion)
}

func validSaveExternalPushCommand(command surveyport.SaveExternalPushOperationsCommand) bool {
	return command.QuestionnaireID > 0 && command.Actor > 0 && validKey(command.IdempotencyKey) && validExternalPushOperations(command.ExternalPush)
}

func validQueueExternalPushTestCommand(command surveyport.QueueExternalPushTestCommand) bool {
	return command.QuestionnaireID > 0 && command.Actor > 0 && validKey(command.IdempotencyKey)
}

func validOperationsProjection(value surveyport.OperationsProjection, questionnaireID surveyport.ID) bool {
	return value.QuestionnaireID == questionnaireID && questionnaireID > 0 && value.LocalOnly &&
		validCompletionOperations(value.Completion) && validExternalPushOperations(value.ExternalPush)
}

func validCompletionOperations(value surveyport.CompletionOperations) bool {
	if value.ChannelID < 0 || !validOptionalOpaqueReference(value.NavigationTargetID) {
		return false
	}
	return value.NavigationTargetID != "" || value.ChannelID == 0
}

func validExternalPushOperations(value surveyport.ExternalPushOperations) bool {
	if !value.Enabled {
		return value.ConfigurationReference == ""
	}
	return validOpaqueReference(value.ConfigurationReference)
}

func validOptionalOpaqueReference(value string) bool {
	return value == "" || validOpaqueReference(value)
}

func validOpaqueReference(value string) bool {
	if !utf8.ValidString(value) || value == "" || utf8.RuneCountInString(value) > 128 || strings.TrimSpace(value) != value || strings.Contains(value, "://") {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("-_.:", character) {
			continue
		}
		return false
	}
	return true
}

func validExternalPushTest(value surveyport.ExternalPushTest, questionnaireID surveyport.ID) bool {
	return value.TestRunID > 0 && value.QuestionnaireID == questionnaireID && questionnaireID > 0 &&
		value.Status == ExternalPushTestQueued && value.AttemptCount == 0 && !value.SideEffectExecuted &&
		!value.ProviderResultReceived && !value.UnknownAfterDispatch && !value.AutoRetryAllowed &&
		!value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() && !value.UpdatedAt.Before(value.CreatedAt)
}

func validExternalPushLogPage(page surveyport.ExternalPushLogPage, questionnaireID *surveyport.ID) bool {
	if !page.LocalOnly || page.Total < 0 || page.Limit < 1 || page.Limit > ExternalPushLogMaximumLimit || page.Offset < 0 ||
		int64(page.Offset) > page.Total || len(page.Items) > int(page.Limit) || page.Items == nil {
		return false
	}
	for index, item := range page.Items {
		if questionnaireID != nil && item.QuestionnaireID != *questionnaireID || !validExternalPushTest(item, item.QuestionnaireID) {
			return false
		}
		if index > 0 {
			previous := page.Items[index-1]
			if previous.CreatedAt.Before(item.CreatedAt) || previous.CreatedAt.Equal(item.CreatedAt) && previous.TestRunID <= item.TestRunID {
				return false
			}
		}
	}
	return true
}

func decodeOperationsProjection(raw json.RawMessage, result *surveyport.OperationsProjection) bool {
	return decodeOperationsSnapshot(raw, result)
}

func decodeExternalPushTest(raw json.RawMessage, result *surveyport.ExternalPushTest) bool {
	return decodeOperationsSnapshot(raw, result)
}

func decodeOperationsSnapshot(raw json.RawMessage, result any) bool {
	if result == nil || len(raw) == 0 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(result) != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

func cloneOperationsProjection(value surveyport.OperationsProjection) surveyport.OperationsProjection {
	return value
}

func cloneExternalPushLogPage(value surveyport.ExternalPushLogPage) surveyport.ExternalPushLogPage {
	value.Items = append([]surveyport.ExternalPushTest{}, value.Items...)
	return value
}

func operationsReady(service *OperationsService) bool {
	return service != nil && service.uow != nil && service.store != nil && service.events != nil && service.now != nil
}

func classifyOperations(err error) error {
	switch {
	case errors.Is(err, ErrInvalidOperations), errors.Is(err, ErrExternalPushNotConfigured), errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict):
		return err
	default:
		return ErrUnavailable
	}
}
