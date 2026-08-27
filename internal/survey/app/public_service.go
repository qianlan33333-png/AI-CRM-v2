package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

var ErrPublicRateLimited = errors.New("public questionnaire rate limited")

const publicRateWindow = 10 * time.Minute

// PublicDefinitionRecord deliberately keeps the storage id out of public DTOs.
type PublicDefinitionRecord struct {
	ID    int64
	State string
	View  surveyport.PublicQuestionnaire
}

type PublicReceipt struct {
	ID                  int64
	DefinitionID        int64
	SubmissionKeyDigest [32]byte
	PayloadDigest       [32]byte
	ResultTokenDigest   [32]byte
	State               string
	ResultSnapshot      json.RawMessage
}

type PublicManagementReceipt struct {
	ID             int64
	Operation      string
	ActorScope     string
	KeyDigest      [32]byte
	PayloadDigest  [32]byte
	State          string
	ResultSnapshot json.RawMessage
}

// PublicStore is Survey-owned. Implementations must never join Contact,
// Identity, WeCom, Outbound, or legacy questionnaire_submissions.
type PublicStore interface {
	GetPublishedBySlug(context.Context, string) (PublicDefinitionRecord, error)
	GetPublicDefinition(context.Context, surveyport.ID, int64) (PublicDefinitionRecord, error)
	GetCurrentPublicDefinition(context.Context, surveyport.ID) (PublicDefinitionRecord, error)
	GetQuestionnaire(context.Context, surveyport.ID) (surveyport.Questionnaire, error)
	CreatePublicDefinition(context.Context, surveyport.Questionnaire, time.Time) (PublicDefinitionRecord, error)
	DisablePublicDefinition(context.Context, surveyport.ID, int64, time.Time) (PublicDefinitionRecord, error)
	ReservePublicManagement(context.Context, string, PublicManagementReceipt, time.Time) (PublicManagementReceipt, bool, error)
	CompletePublicManagement(context.Context, int64, json.RawMessage, time.Time) (PublicManagementReceipt, error)
	ReservePublicReceipt(context.Context, PublicDefinitionRecord, [32]byte, [32]byte, [32]byte, time.Time) (PublicReceipt, bool, error)
	ConsumePublicRate(context.Context, int64, [32]byte, [32]byte, time.Time) error
	CreatePublicSubmission(context.Context, int64, int64, time.Time, []surveyport.PublicSubmissionAnswer) (int64, error)
	CompletePublicReceipt(context.Context, int64, [32]byte, json.RawMessage, time.Time) (PublicReceipt, error)
	LookupPublicResult(context.Context, [32]byte) (surveyport.PublicSubmissionResult, error)
	PublicAnalytics(context.Context, PublicDefinitionRecord) (surveyport.PublicAnalytics, error)
}

type PublicService struct {
	uow      platformport.UnitOfWork
	store    PublicStore
	events   eventport.Appender
	tokenKey [32]byte
	now      func() time.Time
	binder   PublicSubmissionBinder
}

type PublicSubmissionBinder interface {
	BindPublicSubmission(context.Context, PublicDefinitionRecord, int64, surveyport.PublicSubmissionCommand, time.Time) error
}

func NewPublicService(uow platformport.UnitOfWork, store PublicStore, events eventport.Appender, tokenKey [32]byte) *PublicService {
	return &PublicService{uow: uow, store: store, events: events, tokenKey: tokenKey, now: time.Now}
}
func NewPublicServiceWithBinder(uow platformport.UnitOfWork, store PublicStore, events eventport.Appender, tokenKey [32]byte, binder PublicSubmissionBinder) *PublicService {
	s := NewPublicService(uow, store, events, tokenKey)
	s.binder = binder
	return s
}
func (s *PublicService) SetBinder(binder PublicSubmissionBinder) {
	if s != nil {
		s.binder = binder
	}
}

func (s *PublicService) Definition(ctx context.Context, slug string) (surveyport.PublicQuestionnaire, error) {
	if !publicReady(s) {
		return surveyport.PublicQuestionnaire{}, ErrPublicUnavailable
	}
	if !ValidPublicSlug(slug) {
		return surveyport.PublicQuestionnaire{}, ErrInvalidPublicInput
	}
	var record PublicDefinitionRecord
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var err error
		record, err = s.store.GetPublishedBySlug(tx, slug)
		return err
	})
	if err != nil || record.State != "public" {
		return surveyport.PublicQuestionnaire{}, publicClassify(err)
	}
	return record.View, nil
}

func (s *PublicService) Submit(ctx context.Context, input surveyport.PublicSubmissionCommand) (surveyport.PublicSubmissionReceipt, string, error) {
	if !publicReady(s) {
		return surveyport.PublicSubmissionReceipt{}, "", ErrPublicUnavailable
	}
	if !ValidPublicSlug(input.Slug) {
		return surveyport.PublicSubmissionReceipt{}, "", ErrInvalidPublicInput
	}
	now := s.now().UTC()
	if now.IsZero() {
		return surveyport.PublicSubmissionReceipt{}, "", ErrPublicUnavailable
	}
	var receipt surveyport.PublicSubmissionReceipt
	var resultToken string
	err := s.uow.Within(ctx, func(tx context.Context) error {
		record, err := s.store.GetPublishedBySlug(tx, input.Slug)
		if err != nil || record.State != "public" {
			return publicClassify(err)
		}
		command, err := NormalizePublicSubmission(record.View, input)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			Version int64                               `json:"version"`
			Answers []surveyport.PublicSubmissionAnswer `json:"answers"`
		}{command.Version, command.Answers})
		if err != nil {
			return ErrPublicUnavailable
		}
		payloadDigest, keyDigest := sha256.Sum256(payload), sha256.Sum256([]byte(command.SubmissionKey))
		reserved, owned, err := s.store.ReservePublicReceipt(tx, record, command.AnonymousDigest, keyDigest, payloadDigest, now)
		if err != nil {
			return err
		}
		if reserved.ID < 1 || reserved.DefinitionID != record.ID || reserved.SubmissionKeyDigest != keyDigest {
			return ErrPublicUnavailable
		}
		if reserved.PayloadDigest != payloadDigest {
			if !owned {
				return ErrConflict
			}
			return ErrPublicUnavailable
		}
		if !owned {
			if reserved.State != "completed" || json.Unmarshal(reserved.ResultSnapshot, &receipt) != nil || receipt.SubmissionID < 1 {
				return ErrPublicUnavailable
			}
			resultToken, err = DerivePublicResultToken(s.tokenKey, reserved.SubmissionKeyDigest, receipt.SubmissionID, receipt.DefinitionVersion)
			if err != nil || sha256.Sum256([]byte(resultToken)) != reserved.ResultTokenDigest || receipt.QuestionnaireID != record.View.ID || receipt.QuestionnaireSlug != record.View.Slug || receipt.DefinitionVersion != record.View.Version {
				return ErrPublicUnavailable
			}
			return nil
		}
		window := now.Truncate(publicRateWindow)
		if err = s.store.ConsumePublicRate(tx, record.ID, command.RateDigest, command.AnonymousDigest, window); err != nil {
			return err
		}
		submissionID, err := s.store.CreatePublicSubmission(tx, reserved.ID, record.ID, now, command.Answers)
		if err != nil {
			return err
		}
		if input.CanonicalCustomerID > 0 {
			if s.binder == nil {
				return ErrH5IdentityRequired
			}
			if err := s.binder.BindPublicSubmission(tx, record, submissionID, input, now); err != nil {
				return err
			}
		}
		resultToken, err = DerivePublicResultToken(s.tokenKey, keyDigest, submissionID, record.View.Version)
		if err != nil {
			return err
		}
		receipt = surveyport.PublicSubmissionReceipt{QuestionnaireID: record.View.ID, QuestionnaireSlug: record.View.Slug, DefinitionVersion: record.View.Version, SubmissionID: submissionID}
		snapshot, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		resultTokenDigest := sha256.Sum256([]byte(resultToken))
		completed, err := s.store.CompletePublicReceipt(tx, reserved.ID, resultTokenDigest, snapshot, now)
		var completedSnapshot surveyport.PublicSubmissionReceipt
		if err != nil || json.Unmarshal(completed.ResultSnapshot, &completedSnapshot) != nil || completed.ID != reserved.ID || completed.DefinitionID != record.ID || completed.SubmissionKeyDigest != keyDigest || completed.PayloadDigest != payloadDigest || completed.ResultTokenDigest != resultTokenDigest || completed.State != "completed" || completedSnapshot != receipt {
			return ErrPublicUnavailable
		}
		payload, err = json.Marshal(struct {
			QuestionnaireID   surveyport.ID `json:"questionnaire_id"`
			DefinitionVersion int64         `json:"definition_version"`
			SubmissionID      int64         `json:"submission_id"`
		}{record.View.ID, record.View.Version, submissionID})
		if err != nil {
			return err
		}
		tokenDigest := sha256.Sum256([]byte(resultToken))
		_, err = s.events.Append(tx, eventport.Event{Type: "survey.public_submitted", Payload: payload, OccurredAt: now, IdempotencyKey: "survey.public:" + hex.EncodeToString(tokenDigest[:])})
		return err
	})
	if err != nil {
		return surveyport.PublicSubmissionReceipt{}, "", publicClassify(err)
	}
	return receipt, resultToken, nil
}

func (s *PublicService) Result(ctx context.Context, token string) (surveyport.PublicSubmissionResult, error) {
	if !publicReady(s) || !base64URLKey(token) {
		return surveyport.PublicSubmissionResult{}, ErrInvalidPublicInput
	}
	var result surveyport.PublicSubmissionResult
	digest := sha256.Sum256([]byte(token))
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result, err = s.store.LookupPublicResult(tx, digest)
		return err
	})
	if err != nil || !result.LocalOnly || result.ExternalExecuted || result.SubmissionID < 1 || result.DefinitionVersion < 1 || result.SubmittedAt.IsZero() {
		return surveyport.PublicSubmissionResult{}, publicClassify(err)
	}
	return result, nil
}

func (s *PublicService) Publish(ctx context.Context, command surveyport.PublishPublicDefinitionCommand) (PublicDefinitionRecord, error) {
	if !publicReady(s) || command.QuestionnaireID < 1 || command.ExpectedQuestionnaireVersion < 1 || command.Actor < 1 || !validKey(command.IdempotencyKey) {
		return PublicDefinitionRecord{}, ErrInvalidPublicInput
	}
	var record PublicDefinitionRecord
	err := s.uow.Within(ctx, func(tx context.Context) error {
		now := s.now().UTC()
		payload, err := json.Marshal(struct {
			QuestionnaireID surveyport.ID `json:"questionnaire_id"`
			ExpectedVersion int64         `json:"expected_questionnaire_version"`
		}{command.QuestionnaireID, command.ExpectedQuestionnaireVersion})
		if err != nil {
			return ErrPublicUnavailable
		}
		payloadDigest, keyDigest := sha256.Sum256(payload), sha256.Sum256([]byte(command.IdempotencyKey))
		reserved, owned, err := s.store.ReservePublicManagement(tx, "publish", PublicManagementReceipt{
			Operation: "publish", ActorScope: fmt.Sprintf("admin:%d", command.Actor), KeyDigest: keyDigest, PayloadDigest: payloadDigest,
		}, now)
		if err != nil {
			return err
		}
		if reserved.ID < 1 || reserved.Operation != "publish" || reserved.ActorScope != fmt.Sprintf("admin:%d", command.Actor) || reserved.KeyDigest != keyDigest {
			return ErrPublicUnavailable
		}
		if reserved.PayloadDigest != payloadDigest {
			if !owned {
				return ErrConflict
			}
			return ErrPublicUnavailable
		}
		if !owned {
			if reserved.State != "completed" || json.Unmarshal(reserved.ResultSnapshot, &record) != nil || record.ID < 1 || record.State != "public" || record.View.ID != command.QuestionnaireID || record.View.Version < 1 || !ValidPublicSlug(record.View.Slug) {
				return ErrPublicUnavailable
			}
			return nil
		}
		q, err := s.store.GetQuestionnaire(tx, command.QuestionnaireID)
		if err != nil {
			return err
		}
		if q.Version != command.ExpectedQuestionnaireVersion {
			return ErrConflict
		}
		if _, err = PublicDefinition(q); err != nil {
			return err
		}
		record, err = s.store.CreatePublicDefinition(tx, q, now)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(record)
		if err != nil {
			return ErrPublicUnavailable
		}
		completed, err := s.store.CompletePublicManagement(tx, reserved.ID, snapshot, now)
		if err != nil || completed.ID != reserved.ID || completed.Operation != "publish" || completed.ActorScope != reserved.ActorScope || completed.KeyDigest != keyDigest || completed.PayloadDigest != payloadDigest || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrPublicUnavailable
		}
		eventPayload, err := json.Marshal(struct {
			QuestionnaireID   surveyport.ID `json:"questionnaire_id"`
			DefinitionVersion int64         `json:"definition_version"`
		}{record.View.ID, record.View.Version})
		if err != nil {
			return ErrPublicUnavailable
		}
		_, err = s.events.Append(tx, eventport.Event{Type: "survey.public_published", Payload: eventPayload, OccurredAt: now, IdempotencyKey: "survey.public.publish:" + hex.EncodeToString(keyDigest[:])})
		return err
	})
	if err != nil {
		return PublicDefinitionRecord{}, publicClassify(err)
	}
	return record, nil
}

func (s *PublicService) Disable(ctx context.Context, command surveyport.DisablePublicDefinitionCommand) (PublicDefinitionRecord, error) {
	if !publicReady(s) || command.QuestionnaireID < 1 || command.ExpectedDefinitionVersion < 1 || command.Actor < 1 || !validKey(command.IdempotencyKey) {
		return PublicDefinitionRecord{}, ErrInvalidPublicInput
	}
	var record PublicDefinitionRecord
	err := s.uow.Within(ctx, func(tx context.Context) error {
		now := s.now().UTC()
		payload, err := json.Marshal(struct {
			QuestionnaireID surveyport.ID `json:"questionnaire_id"`
			ExpectedVersion int64         `json:"expected_definition_version"`
		}{command.QuestionnaireID, command.ExpectedDefinitionVersion})
		if err != nil {
			return ErrPublicUnavailable
		}
		payloadDigest, keyDigest := sha256.Sum256(payload), sha256.Sum256([]byte(command.IdempotencyKey))
		reserved, owned, err := s.store.ReservePublicManagement(tx, "disable", PublicManagementReceipt{Operation: "disable", ActorScope: fmt.Sprintf("admin:%d", command.Actor), KeyDigest: keyDigest, PayloadDigest: payloadDigest}, now)
		if err != nil {
			return err
		}
		if reserved.ID < 1 || reserved.Operation != "disable" || reserved.ActorScope != fmt.Sprintf("admin:%d", command.Actor) || reserved.KeyDigest != keyDigest {
			return ErrPublicUnavailable
		}
		if reserved.PayloadDigest != payloadDigest {
			if !owned {
				return ErrConflict
			}
			return ErrPublicUnavailable
		}
		if !owned {
			if reserved.State != "completed" || json.Unmarshal(reserved.ResultSnapshot, &record) != nil || record.ID < 1 || record.State != "disabled" || record.View.ID != command.QuestionnaireID || record.View.Version != command.ExpectedDefinitionVersion || !ValidPublicSlug(record.View.Slug) {
				return ErrPublicUnavailable
			}
			return nil
		}
		record, err = s.store.DisablePublicDefinition(tx, command.QuestionnaireID, command.ExpectedDefinitionVersion, now)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(record)
		if err != nil {
			return ErrPublicUnavailable
		}
		completed, err := s.store.CompletePublicManagement(tx, reserved.ID, snapshot, now)
		if err != nil || completed.ID != reserved.ID || completed.Operation != "disable" || completed.ActorScope != reserved.ActorScope || completed.KeyDigest != keyDigest || completed.PayloadDigest != payloadDigest || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrPublicUnavailable
		}
		eventPayload, err := json.Marshal(struct {
			QuestionnaireID   surveyport.ID `json:"questionnaire_id"`
			DefinitionVersion int64         `json:"definition_version"`
		}{record.View.ID, record.View.Version})
		if err != nil {
			return ErrPublicUnavailable
		}
		_, err = s.events.Append(tx, eventport.Event{Type: "survey.public_disabled", Payload: eventPayload, OccurredAt: now, IdempotencyKey: "survey.public.disable:" + hex.EncodeToString(keyDigest[:])})
		return err
	})
	if err != nil {
		return PublicDefinitionRecord{}, publicClassify(err)
	}
	return record, nil
}

func (s *PublicService) Analytics(ctx context.Context, questionnaireID surveyport.ID, definitionVersion int64) (surveyport.PublicAnalytics, error) {
	if !publicReady(s) || questionnaireID < 1 || definitionVersion < 0 {
		return surveyport.PublicAnalytics{}, ErrInvalidPublicInput
	}
	var result surveyport.PublicAnalytics
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var record PublicDefinitionRecord
		var err error
		if definitionVersion == 0 {
			record, err = s.store.GetCurrentPublicDefinition(tx, questionnaireID)
		} else {
			record, err = s.store.GetPublicDefinition(tx, questionnaireID, definitionVersion)
		}
		if err != nil {
			return err
		}
		if definitionVersion == 0 && record.State != "public" {
			return ErrNotFound
		}
		result, err = s.store.PublicAnalytics(tx, record)
		if err == nil && (result.QuestionnaireID != record.View.ID || result.DefinitionVersion != record.View.Version || result.Slug != record.View.Slug || result.State != record.State || !ValidPublicSlug(result.Slug) || result.State != "public" && result.State != "disabled") {
			return ErrPublicUnavailable
		}
		return err
	})
	if err != nil {
		return surveyport.PublicAnalytics{}, publicClassify(err)
	}
	return result, nil
}

func publicReady(s *PublicService) bool {
	return s != nil && s.uow != nil && s.store != nil && s.events != nil && !zeroDigest(s.tokenKey) && s.now != nil
}
func publicClassify(err error) error {
	if errors.Is(err, ErrPublicRateLimited) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalidPublicInput) || errors.Is(err, ErrPublicUnavailable) || errors.Is(err, ErrNotFound) {
		return err
	}
	return ErrPublicUnavailable
}
