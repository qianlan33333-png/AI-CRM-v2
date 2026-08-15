package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	DefaultLimit        int32 = 50
	MaximumLimit        int32 = 200
	MaximumLegacyOffset int32 = 1_000_000
)

var (
	ErrInvalidQuestionnaire  = errors.New("invalid questionnaire")
	ErrInvalidSchema         = errors.New("invalid questionnaire schema")
	ErrAssessmentUnavailable = errors.New("F02 assessment not yet available")
	ErrInvalidPage           = errors.New("invalid questionnaire page")
	ErrNotFound              = errors.New("questionnaire not found")
	ErrConflict              = errors.New("questionnaire command conflict")
	ErrUnavailable           = errors.New("questionnaire CRUD unavailable")
)

type Receipt struct {
	ID             int64
	Operation      string
	ActorScope     string
	KeyDigest      [32]byte
	PayloadDigest  [32]byte
	State          string
	ResultSnapshot json.RawMessage
}

type Reservation struct {
	ActorScope    string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type Store interface {
	ListOffset(context.Context, int32, int32) ([]surveyport.Questionnaire, error)
	Count(context.Context) (int64, error)
	Get(context.Context, surveyport.ID) (surveyport.Questionnaire, error)
	Create(context.Context, surveyport.CreateCommand, time.Time) (surveyport.Questionnaire, error)
	Update(context.Context, surveyport.ID, surveyport.UpdateCommand, time.Time) (surveyport.Questionnaire, error)
	SetDisabled(context.Context, surveyport.ID, bool, time.Time) (surveyport.Questionnaire, error)
	Delete(context.Context, surveyport.ID) (surveyport.Questionnaire, error)
	Reserve(context.Context, string, Reservation) (Receipt, bool, error)
	Complete(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
	ReserveManagement(context.Context, string, Reservation) (Receipt, bool, error)
	CompleteManagement(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
}

type Service struct {
	uow    platformport.UnitOfWork
	store  Store
	events eventport.Appender
	now    func() time.Time
}

func NewService(uow platformport.UnitOfWork, store Store, events eventport.Appender) *Service {
	return &Service{uow: uow, store: store, events: events, now: time.Now}
}

func (s *Service) ListLegacy(ctx context.Context, limit, offset int32) (surveyport.LegacyPage, error) {
	if !ready(s) || limit < 1 || limit > MaximumLimit || offset < 0 || offset > MaximumLegacyOffset {
		return surveyport.LegacyPage{}, ErrInvalidPage
	}
	result := surveyport.LegacyPage{Limit: limit, Offset: offset}
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result.Items, err = s.store.ListOffset(tx, limit, offset)
		if err == nil {
			result.Total, err = s.store.Count(tx)
		}
		return err
	})
	if err != nil {
		return surveyport.LegacyPage{}, classify(err)
	}
	if result.Total < 0 || len(result.Items) > int(limit) || int64(offset) > result.Total && len(result.Items) != 0 || !validStoredList(result.Items) {
		return surveyport.LegacyPage{}, ErrUnavailable
	}
	return clonePage(result), nil
}

func (s *Service) Get(ctx context.Context, id surveyport.ID) (surveyport.Questionnaire, error) {
	if !ready(s) {
		return surveyport.Questionnaire{}, ErrUnavailable
	}
	if id < 1 {
		return surveyport.Questionnaire{}, ErrNotFound
	}
	var result surveyport.Questionnaire
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result, err = s.store.Get(tx, id)
		return err
	})
	if err != nil {
		return surveyport.Questionnaire{}, classify(err)
	}
	if result.ID != id || !validStored(result) {
		return surveyport.Questionnaire{}, ErrUnavailable
	}
	return cloneQuestionnaire(result), nil
}

func (s *Service) Create(ctx context.Context, input surveyport.CreateCommand) (surveyport.Questionnaire, error) {
	command, err := normalize(input)
	if err != nil {
		return surveyport.Questionnaire{}, err
	}
	if !ready(s) {
		return surveyport.Questionnaire{}, ErrUnavailable
	}
	now := s.now().UTC()
	if now.IsZero() {
		return surveyport.Questionnaire{}, ErrUnavailable
	}
	digest, err := payloadDigest(command)
	if err != nil {
		return surveyport.Questionnaire{}, ErrInvalidQuestionnaire
	}
	actorScope := fmt.Sprintf("admin:%d", command.Actor)
	reservation := Reservation{ActorScope: actorScope, KeyDigest: sha256.Sum256([]byte(command.IdempotencyKey)), PayloadDigest: digest, CreatedAt: now}
	var result surveyport.Questionnaire
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, err := s.store.Reserve(tx, "create", reservation)
		if err != nil {
			return err
		}
		if !receiptMatches(receipt, "create", reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], digest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeSnapshot(receipt.ResultSnapshot, &result) || !validStored(result) {
				return ErrUnavailable
			}
			canonical, marshalErr := json.Marshal(result)
			if marshalErr != nil || !jsonEquivalent(canonical, receipt.ResultSnapshot) {
				return ErrUnavailable
			}
			return nil
		}
		if receipt.State != "in_progress" || len(receipt.ResultSnapshot) != 0 {
			return ErrUnavailable
		}
		result, err = s.store.Create(tx, command, now)
		if err != nil {
			return err
		}
		if !validStored(result) {
			return ErrUnavailable
		}
		snapshot, err := json.Marshal(result)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			QuestionnaireID surveyport.ID `json:"questionnaire_id"`
			Actor           int64         `json:"actor"`
		}{result.ID, command.Actor})
		if err != nil {
			return err
		}
		eventDigest := sha256.Sum256([]byte(actorScope + "\x00" + command.IdempotencyKey))
		if _, err = s.events.Append(tx, eventport.Event{Type: eventport.EvSurveyCreated, Payload: payload, OccurredAt: now, IdempotencyKey: "survey.create:" + hex.EncodeToString(eventDigest[:])}); err != nil {
			return err
		}
		completed, err := s.store.Complete(tx, receipt.ID, snapshot, now)
		if err != nil || !receiptMatches(completed, "create", reservation) || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return surveyport.Questionnaire{}, classify(err)
	}
	return cloneQuestionnaire(result), nil
}

// Update replaces a questionnaire definition atomically. This is intentionally
// a full-definition write: the legacy editor posts the nested schema it has
// just loaded, so accepting a partial patch would invent a new contract.
func (s *Service) Update(ctx context.Context, id surveyport.ID, input surveyport.UpdateCommand) (surveyport.Questionnaire, error) {
	if id < 1 {
		return surveyport.Questionnaire{}, ErrNotFound
	}
	command, err := normalizeUpdate(input)
	if err != nil {
		return surveyport.Questionnaire{}, err
	}
	if !ready(s) {
		return surveyport.Questionnaire{}, ErrUnavailable
	}
	now := s.now().UTC()
	digest, err := updatePayloadDigest(id, command)
	if err != nil {
		return surveyport.Questionnaire{}, ErrInvalidQuestionnaire
	}
	reservation := Reservation{ActorScope: fmt.Sprintf("admin:%d", command.Actor), KeyDigest: sha256.Sum256([]byte(command.IdempotencyKey)), PayloadDigest: digest, CreatedAt: now}
	var result surveyport.Questionnaire
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveManagement(tx, "update", reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !receiptMatches(receipt, "update", reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], digest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeSnapshot(receipt.ResultSnapshot, &result) || result.ID != id || !validStored(result) {
				return ErrUnavailable
			}
			return nil
		}
		result, reserveErr = s.store.Update(tx, id, command, now)
		if reserveErr != nil {
			return reserveErr
		}
		if result.ID != id || !validStored(result) {
			return ErrUnavailable
		}
		if reserveErr = appendSurveyEvent(tx, s.events, eventport.EvSurveyUpdated, id, command.Actor, command.IdempotencyKey, now, "update"); reserveErr != nil {
			return reserveErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteManagement(tx, receipt.ID, snapshot, now)
		if completeErr != nil {
			return completeErr
		}
		if !receiptMatches(completed, "update", reservation) || completed.State != "completed" {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return surveyport.Questionnaire{}, classify(err)
	}
	return cloneQuestionnaire(result), nil
}

func (s *Service) SetDisabled(ctx context.Context, id surveyport.ID, disabled bool, actor int64, key string) (surveyport.Questionnaire, error) {
	if id < 1 {
		return surveyport.Questionnaire{}, ErrNotFound
	}
	if !ready(s) || actor < 1 || !validKey(key) {
		return surveyport.Questionnaire{}, ErrInvalidQuestionnaire
	}
	now := s.now().UTC()
	payload := fmt.Sprintf("%d:%t", id, disabled)
	reservation := Reservation{ActorScope: fmt.Sprintf("admin:%d", actor), KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: sha256.Sum256([]byte(payload)), CreatedAt: now}
	operation := "enable"
	if disabled {
		operation = "disable"
	}
	var result surveyport.Questionnaire
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveManagement(tx, operation, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !receiptMatches(receipt, operation, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeSnapshot(receipt.ResultSnapshot, &result) || result.ID != id || !validStored(result) {
				return ErrUnavailable
			}
			return nil
		}
		result, reserveErr = s.store.SetDisabled(tx, id, disabled, now)
		if reserveErr != nil {
			return reserveErr
		}
		if result.ID != id || result.IsDisabled != disabled || !validStored(result) {
			return ErrUnavailable
		}
		if reserveErr = appendSurveyEvent(tx, s.events, eventport.EvSurveyUpdated, id, actor, key, now, operation); reserveErr != nil {
			return reserveErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteManagement(tx, receipt.ID, snapshot, now)
		if completeErr != nil {
			return completeErr
		}
		if !receiptMatches(completed, operation, reservation) || completed.State != "completed" {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return surveyport.Questionnaire{}, classify(err)
	}
	return cloneQuestionnaire(result), nil
}

func (s *Service) Delete(ctx context.Context, id surveyport.ID, actor int64, key string) (surveyport.DeleteResult, error) {
	if id < 1 {
		return surveyport.DeleteResult{}, ErrNotFound
	}
	if !ready(s) || actor < 1 || !validKey(key) {
		return surveyport.DeleteResult{}, ErrInvalidQuestionnaire
	}
	now := s.now().UTC()
	reservation := Reservation{ActorScope: fmt.Sprintf("admin:%d", actor), KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: sha256.Sum256([]byte(fmt.Sprintf("%d", id))), CreatedAt: now}
	var result surveyport.DeleteResult
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.ReserveManagement(tx, "delete", reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !receiptMatches(receipt, "delete", reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeDeleteSnapshot(receipt.ResultSnapshot, &result) || !result.Deleted || result.Questionnaire.ID != id || !validStored(result.Questionnaire) {
				return ErrUnavailable
			}
			return nil
		}
		item, deleteErr := s.store.Delete(tx, id)
		if deleteErr != nil {
			return deleteErr
		}
		if item.ID != id || !item.IsDisabled || !validStored(item) {
			return ErrUnavailable
		}
		result = surveyport.DeleteResult{Questionnaire: item, Deleted: true}
		if deleteErr = appendSurveyEvent(tx, s.events, eventport.EvSurveyDeleted, id, actor, key, now, "delete"); deleteErr != nil {
			return deleteErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := s.store.CompleteManagement(tx, receipt.ID, snapshot, now)
		if completeErr != nil {
			return completeErr
		}
		if !receiptMatches(completed, "delete", reservation) || completed.State != "completed" {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return surveyport.DeleteResult{}, classify(err)
	}
	result.Questionnaire = cloneQuestionnaire(result.Questionnaire)
	return result, nil
}

func (s *Service) Duplicate(ctx context.Context, id surveyport.ID, actor int64, key, title, slug string) (surveyport.Questionnaire, error) {
	source, err := s.Get(ctx, id)
	if err != nil {
		return surveyport.Questionnaire{}, err
	}
	if title == "" {
		title = source.Title + " Copy"
	}
	if slug == "" {
		digest := sha256.Sum256([]byte(key))
		slug = fmt.Sprintf("%s-copy-%x", source.Slug, digest[:3])
	}
	questions := cloneQuestions(source.Questions)
	for questionIndex := range questions {
		questions[questionIndex].ID = 0
		for optionIndex := range questions[questionIndex].Options {
			questions[questionIndex].Options[optionIndex].ID = 0
		}
	}
	return s.Create(ctx, surveyport.CreateCommand{Questionnaire: surveyport.Questionnaire{Name: source.Name, Title: title, Description: source.Description, AnswerDisplayMode: source.AnswerDisplayMode, AssessmentConfig: source.AssessmentConfig, Slug: slug, IsDisabled: true, Questions: questions, ScoreRules: source.ScoreRules}, Actor: actor, IdempotencyKey: key})
}

func normalize(input surveyport.CreateCommand) (surveyport.CreateCommand, error) {
	config, empty, err := emptyConfig(input.AssessmentConfig)
	if err != nil {
		return surveyport.CreateCommand{}, ErrInvalidQuestionnaire
	}
	if input.AssessmentEnabled || !empty || len(input.ScoreRules) != 0 {
		return surveyport.CreateCommand{}, ErrAssessmentUnavailable
	}
	if input.Actor < 1 || !validKey(input.IdempotencyKey) {
		return surveyport.CreateCommand{}, ErrInvalidQuestionnaire
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Slug = strings.TrimSpace(input.Slug)
	if !required(input.Name, 120) || !required(input.Title, 300) || !optional(input.Description, 10_000) || !optional(input.Slug, 200) || input.AnswerDisplayMode != surveyport.AllInOne && input.AnswerDisplayMode != surveyport.OneByOne {
		return surveyport.CreateCommand{}, ErrInvalidQuestionnaire
	}
	questions, err := normalizeQuestions(input.Questions, false)
	if err != nil {
		return surveyport.CreateCommand{}, err
	}
	input.ID, input.CreatedBy, input.Version, input.SubmissionCount = 0, 0, 0, 0
	input.CreatedAt, input.UpdatedAt = time.Time{}, time.Time{}
	input.AssessmentConfig, input.ScoreRules, input.Questions = config, []surveyport.ScoreRule{}, questions
	return input, nil
}

func normalizeUpdate(input surveyport.UpdateCommand) (surveyport.UpdateCommand, error) {
	command := surveyport.CreateCommand{Questionnaire: input.Questionnaire, Actor: input.Actor, IdempotencyKey: input.IdempotencyKey}
	command.ID, command.CreatedBy, command.Version, command.SubmissionCount = 0, 0, 0, 0
	command.CreatedAt, command.UpdatedAt = time.Time{}, time.Time{}
	for index := range command.Questions {
		command.Questions[index].ID = 0
		for optionIndex := range command.Questions[index].Options {
			command.Questions[index].Options[optionIndex].ID = 0
		}
	}
	normalized, err := normalize(command)
	if err != nil {
		return surveyport.UpdateCommand{}, err
	}
	return surveyport.UpdateCommand{Questionnaire: normalized.Questionnaire, Actor: normalized.Actor, IdempotencyKey: normalized.IdempotencyKey}, nil
}

func normalizeQuestions(source []surveyport.Question, stored bool) ([]surveyport.Question, error) {
	if len(source) < 1 || len(source) > 100 {
		return nil, ErrInvalidSchema
	}
	result := cloneQuestions(source)
	orders := make(map[int]bool, len(result))
	for i := range result {
		q := &result[i]
		q.Title, q.AssessmentDimensionKey = strings.TrimSpace(q.Title), strings.TrimSpace(q.AssessmentDimensionKey)
		q.SidebarProfileField, q.PlaceholderText = strings.TrimSpace(q.SidebarProfileField), strings.TrimSpace(q.PlaceholderText)
		if q.SortOrder < 0 || orders[q.SortOrder] || !required(q.Title, 500) || !optional(q.AssessmentDimensionKey, 200) || !optional(q.SidebarProfileField, 200) || !optional(q.PlaceholderText, 500) || stored && q.ID < 1 || !stored && q.ID != 0 {
			return nil, ErrInvalidSchema
		}
		orders[q.SortOrder] = true
		if err := normalizeQuestion(q, stored); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].SortOrder < result[j].SortOrder })
	for i := range result {
		if result[i].SortOrder != i {
			return nil, ErrInvalidSchema
		}
	}
	return result, nil
}

func normalizeQuestion(q *surveyport.Question, stored bool) error {
	switch q.Type {
	case surveyport.SingleChoice, surveyport.MultiChoice:
		if q.PlaceholderText != "" || q.Validation.MinimumLength != nil || q.Validation.MaximumLength != nil {
			return ErrInvalidSchema
		}
		options, err := normalizeOptions(q.Options, stored)
		if err != nil {
			return err
		}
		q.Options = options
		minimum, maximum := 0, len(options)
		if q.Required {
			minimum = 1
		}
		if q.Validation.MinimumSelections != nil {
			minimum = *q.Validation.MinimumSelections
		}
		if q.Validation.MaximumSelections != nil {
			maximum = *q.Validation.MaximumSelections
		}
		if q.Type == surveyport.SingleChoice {
			if q.Validation.MaximumSelections != nil && maximum != 1 {
				return ErrInvalidSchema
			}
			maximum = 1
		}
		if minimum < 0 || maximum < 1 || minimum > maximum || maximum > len(options) || q.Required && minimum == 0 {
			return ErrInvalidSchema
		}
		q.Validation.MinimumSelections, q.Validation.MaximumSelections = intRef(minimum), intRef(maximum)
	case surveyport.Textarea, surveyport.Mobile:
		if len(q.Options) != 0 || q.Validation.MinimumSelections != nil || q.Validation.MaximumSelections != nil {
			return ErrInvalidSchema
		}
		minimum, maximum, ceiling := 0, 2000, 10_000
		if q.Type == surveyport.Mobile {
			maximum, ceiling = 32, 32
		}
		if q.Required {
			minimum = 1
		}
		if q.Validation.MinimumLength != nil {
			minimum = *q.Validation.MinimumLength
		}
		if q.Validation.MaximumLength != nil {
			maximum = *q.Validation.MaximumLength
		}
		if minimum < 0 || maximum < 1 || maximum > ceiling || minimum > maximum || q.Required && minimum == 0 {
			return ErrInvalidSchema
		}
		q.Options = []surveyport.Option{}
		q.Validation.MinimumLength, q.Validation.MaximumLength = intRef(minimum), intRef(maximum)
	default:
		return ErrInvalidSchema
	}
	return nil
}

func normalizeOptions(source []surveyport.Option, stored bool) ([]surveyport.Option, error) {
	if len(source) < 1 || len(source) > 100 {
		return nil, ErrInvalidSchema
	}
	result := cloneOptions(source)
	orders, texts, otherCount := map[int]bool{}, map[string]bool{}, 0
	for i := range result {
		o := &result[i]
		o.OptionText, o.AssessmentTypeKey, o.OtherPlaceholder = strings.TrimSpace(o.OptionText), strings.TrimSpace(o.AssessmentTypeKey), strings.TrimSpace(o.OtherPlaceholder)
		key := strings.ToLower(o.OptionText)
		if o.SortOrder < 0 || orders[o.SortOrder] || texts[key] || math.IsNaN(o.Score) || math.IsInf(o.Score, 0) || !required(o.OptionText, 500) || !optional(o.AssessmentTypeKey, 200) || !optional(o.OtherPlaceholder, 500) || stored && o.ID < 1 || !stored && o.ID != 0 {
			return nil, ErrInvalidSchema
		}
		orders[o.SortOrder], texts[key] = true, true
		codes, err := canonicalCodes(o.TagCodes)
		if err != nil {
			return nil, err
		}
		o.TagCodes = codes
		if o.IsOther {
			otherCount++
			if o.OtherMaximumLength == 0 {
				o.OtherMaximumLength = 200
			}
			if otherCount > 1 || o.OtherMaximumLength < 1 || o.OtherMaximumLength > 2000 {
				return nil, ErrInvalidSchema
			}
		} else if o.OtherPlaceholder != "" || o.OtherMaximumLength != 0 {
			return nil, ErrInvalidSchema
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].SortOrder < result[j].SortOrder })
	for i := range result {
		if result[i].SortOrder != i {
			return nil, ErrInvalidSchema
		}
	}
	return result, nil
}

func canonicalCodes(source []string) ([]string, error) {
	if len(source) > 100 {
		return nil, ErrInvalidSchema
	}
	result, seen := make([]string, len(source)), map[string]bool{}
	for i, raw := range source {
		value := strings.TrimSpace(raw)
		if !required(value, 200) || seen[value] {
			return nil, ErrInvalidSchema
		}
		seen[value], result[i] = true, value
	}
	return result, nil
}

func validStored(q surveyport.Questionnaire) bool {
	config, empty, err := emptyConfig(q.AssessmentConfig)
	if err != nil || !empty || !jsonEquivalent(config, q.AssessmentConfig) || q.ID < 1 || q.CreatedBy < 1 || q.Version < 1 || q.SubmissionCount < 0 || q.AssessmentEnabled || len(q.ScoreRules) != 0 || !required(q.Slug, 200) || q.Slug != strings.TrimSpace(q.Slug) || !required(q.Name, 120) || q.Name != strings.TrimSpace(q.Name) || !required(q.Title, 300) || q.Title != strings.TrimSpace(q.Title) || !optional(q.Description, 10_000) || q.Description != strings.TrimSpace(q.Description) || q.AnswerDisplayMode != surveyport.AllInOne && q.AnswerDisplayMode != surveyport.OneByOne || q.CreatedAt.IsZero() || q.UpdatedAt.Before(q.CreatedAt) {
		return false
	}
	questions, err := normalizeQuestions(q.Questions, true)
	return err == nil && reflect.DeepEqual(questions, q.Questions)
}

func validStoredList(items []surveyport.Questionnaire) bool {
	var previous surveyport.ID
	for _, item := range items {
		if item.ID <= previous || !validStored(item) {
			return false
		}
		previous = item.ID
	}
	return true
}

func payloadDigest(command surveyport.CreateCommand) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Name, Title, Description string
		Mode                     surveyport.AnswerDisplayMode
		Config                   json.RawMessage
		Slug                     string
		Disabled                 bool
		Questions                []surveyport.Question
	}{command.Name, command.Title, command.Description, command.AnswerDisplayMode, command.AssessmentConfig, command.Slug, command.IsDisabled, command.Questions})
	return sha256.Sum256(encoded), err
}

func updatePayloadDigest(id surveyport.ID, command surveyport.UpdateCommand) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		ID      surveyport.ID
		Command surveyport.UpdateCommand
	}{id, command})
	return sha256.Sum256(encoded), err
}

func emptyConfig(raw json.RawMessage) (json.RawMessage, bool, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(`{}`), true, nil
	}
	value, ok := decodeJSON([]byte(trimmed))
	if !ok {
		return nil, false, ErrInvalidQuestionnaire
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return json.RawMessage(`{}`), true, nil
		}
	case []any:
		if len(typed) == 0 {
			return json.RawMessage(`{}`), true, nil
		}
	}
	return json.RawMessage(trimmed), false, nil
}

func receiptMatches(receipt Receipt, operation string, reservation Reservation) bool {
	return receipt.ID > 0 && receipt.Operation == operation && receipt.ActorScope == reservation.ActorScope && subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 && (receipt.State == "in_progress" || receipt.State == "completed")
}

func decodeSnapshot(raw json.RawMessage, destination *surveyport.Questionnaire) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	return decoder.Decode(destination) == nil && errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}
func decodeDeleteSnapshot(raw json.RawMessage, destination *surveyport.DeleteResult) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	return decoder.Decode(destination) == nil && errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func appendSurveyEvent(ctx context.Context, events eventport.Appender, eventType string, id surveyport.ID, actor int64, key string, occurredAt time.Time, operation string) error {
	payload, err := json.Marshal(struct {
		QuestionnaireID surveyport.ID `json:"questionnaire_id"`
		Actor           int64         `json:"actor"`
		Operation       string        `json:"operation"`
	}{id, actor, operation})
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s\x00%s", actor, operation, key)))
	_, err = events.Append(ctx, eventport.Event{Type: eventType, Payload: payload, OccurredAt: occurredAt, IdempotencyKey: "survey." + operation + ":" + hex.EncodeToString(digest[:])})
	return err
}

func jsonEquivalent(left, right []byte) bool {
	a, ok := decodeJSON(left)
	if !ok {
		return false
	}
	b, ok := decodeJSON(right)
	return ok && reflect.DeepEqual(a, b)
}

func decodeJSON(raw []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	return value, errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func ready(s *Service) bool {
	return s != nil && s.uow != nil && s.store != nil && s.events != nil && s.now != nil
}
func classify(err error) error {
	switch {
	case errors.Is(err, ErrInvalidQuestionnaire), errors.Is(err, ErrInvalidSchema), errors.Is(err, ErrAssessmentUnavailable), errors.Is(err, ErrInvalidPage), errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict):
		return err
	default:
		return ErrUnavailable
	}
}
func required(value string, maximum int) bool { return value != "" && optional(value, maximum) }
func optional(value string, maximum int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum
}
func validKey(value string) bool {
	return len(value) >= 16 && len(value) <= 128 && strings.TrimSpace(value) == value && utf8.ValidString(value)
}
func intRef(value int) *int { return &value }
func clonePage(page surveyport.LegacyPage) surveyport.LegacyPage {
	source := page.Items
	page.Items = make([]surveyport.Questionnaire, len(source))
	for i := range source {
		page.Items[i] = cloneQuestionnaire(source[i])
	}
	return page
}
func cloneQuestionnaire(source surveyport.Questionnaire) surveyport.Questionnaire {
	result := source
	result.AssessmentConfig = append(json.RawMessage{}, source.AssessmentConfig...)
	result.Questions = cloneQuestions(source.Questions)
	result.ScoreRules = append([]surveyport.ScoreRule{}, source.ScoreRules...)
	return result
}
func cloneQuestions(source []surveyport.Question) []surveyport.Question {
	result := make([]surveyport.Question, len(source))
	for i, q := range source {
		result[i] = q
		result[i].Validation = surveyport.Validation{MinimumSelections: cloneInt(q.Validation.MinimumSelections), MaximumSelections: cloneInt(q.Validation.MaximumSelections), MinimumLength: cloneInt(q.Validation.MinimumLength), MaximumLength: cloneInt(q.Validation.MaximumLength)}
		result[i].Options = cloneOptions(q.Options)
	}
	return result
}
func cloneOptions(source []surveyport.Option) []surveyport.Option {
	result := make([]surveyport.Option, len(source))
	for i, option := range source {
		result[i] = option
		result[i].TagCodes = append([]string{}, option.TagCodes...)
	}
	return result
}
func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	return intRef(*value)
}
