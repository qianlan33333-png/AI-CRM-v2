package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestPublicDefinitionAndInputAreChoiceOnly(t *testing.T) {
	min, max := 1, 1
	source := surveyport.Questionnaire{ID: 1, Slug: "public", Title: "匿名", AnswerDisplayMode: surveyport.AllInOne, Version: 2, Questions: []surveyport.Question{{ID: 11, Type: surveyport.SingleChoice, Title: "选择", Required: true, SortOrder: 0, Validation: surveyport.Validation{MinimumSelections: &min, MaximumSelections: &max}, Options: []surveyport.Option{{ID: 21, OptionText: "A", SortOrder: 0}}}}}
	definition, err := PublicDefinition(source)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	raw := sha256.Sum256([]byte("browser"))
	_, err = NormalizePublicSubmission(definition, surveyport.PublicSubmissionCommand{Slug: "public", Version: 2, SubmissionKey: key, AnonymousDigest: raw, RateDigest: raw, Answers: []surveyport.PublicSubmissionAnswer{{QuestionID: 11, OptionIDs: []int64{21}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NormalizePublicSubmission(definition, surveyport.PublicSubmissionCommand{Slug: "public", Version: 2, SubmissionKey: key, AnonymousDigest: raw, Answers: []surveyport.PublicSubmissionAnswer{{QuestionID: 11, OptionIDs: []int64{21}}}}); err == nil {
		t.Fatal("missing source rate digest must fail closed")
	}
	source.Questions[0].Type = surveyport.Mobile
	if _, err = PublicDefinition(source); !errors.Is(err, ErrInvalidPublicInput) {
		t.Fatal("mobile must not publish")
	}
	for _, unsafe := range []string{"Public", "public survey", "public_1", "-public", "public/1", strings.Repeat("a", 121)} {
		source.Questions[0].Type = surveyport.SingleChoice
		source.Slug = unsafe
		if _, err = PublicDefinition(source); !errors.Is(err, ErrInvalidPublicInput) {
			t.Fatalf("unsafe public slug accepted: %q", unsafe)
		}
	}
}

func TestPublicResultTokenIsStableAndNeverRawStored(t *testing.T) {
	key := sha256.Sum256([]byte("secret"))
	digest := sha256.Sum256([]byte("submission"))
	first, err := DerivePublicResultToken(key, digest, 9, 2)
	if err != nil || len(first) != 43 {
		t.Fatalf("%q %v", first, err)
	}
	second, _ := DerivePublicResultToken(key, digest, 9, 2)
	if first != second {
		t.Fatal("unstable")
	}
	third, _ := DerivePublicResultToken(key, digest, 10, 2)
	if first == third {
		t.Fatal("must bind submission")
	}
}

type analyticsStore struct {
	current       PublicDefinitionRecord
	currentErr    error
	analytics     surveyport.PublicAnalytics
	currentCalls  int
	explicitCalls int
}

func (*analyticsStore) GetPublishedBySlug(context.Context, string) (PublicDefinitionRecord, error) {
	return PublicDefinitionRecord{}, ErrUnavailable
}
func (s *analyticsStore) GetPublicDefinition(_ context.Context, _ surveyport.ID, _ int64) (PublicDefinitionRecord, error) {
	s.explicitCalls++
	return PublicDefinitionRecord{}, ErrUnavailable
}
func (s *analyticsStore) GetCurrentPublicDefinition(context.Context, surveyport.ID) (PublicDefinitionRecord, error) {
	s.currentCalls++
	return s.current, s.currentErr
}
func (*analyticsStore) GetQuestionnaire(context.Context, surveyport.ID) (surveyport.Questionnaire, error) {
	return surveyport.Questionnaire{}, ErrUnavailable
}
func (*analyticsStore) CreatePublicDefinition(context.Context, surveyport.Questionnaire, time.Time) (PublicDefinitionRecord, error) {
	return PublicDefinitionRecord{}, ErrUnavailable
}
func (*analyticsStore) DisablePublicDefinition(context.Context, surveyport.ID, int64, time.Time) (PublicDefinitionRecord, error) {
	return PublicDefinitionRecord{}, ErrUnavailable
}
func (*analyticsStore) ReservePublicManagement(context.Context, string, PublicManagementReceipt, time.Time) (PublicManagementReceipt, bool, error) {
	return PublicManagementReceipt{}, false, ErrUnavailable
}
func (*analyticsStore) CompletePublicManagement(context.Context, int64, json.RawMessage, time.Time) (PublicManagementReceipt, error) {
	return PublicManagementReceipt{}, ErrUnavailable
}
func (*analyticsStore) ReservePublicReceipt(context.Context, PublicDefinitionRecord, [32]byte, [32]byte, [32]byte, time.Time) (PublicReceipt, bool, error) {
	return PublicReceipt{}, false, ErrUnavailable
}
func (*analyticsStore) ConsumePublicRate(context.Context, int64, [32]byte, [32]byte, time.Time) error {
	return ErrUnavailable
}
func (*analyticsStore) CreatePublicSubmission(context.Context, int64, int64, time.Time, []surveyport.PublicSubmissionAnswer) (int64, error) {
	return 0, ErrUnavailable
}
func (*analyticsStore) CompletePublicReceipt(context.Context, int64, [32]byte, json.RawMessage, time.Time) (PublicReceipt, error) {
	return PublicReceipt{}, ErrUnavailable
}
func (*analyticsStore) LookupPublicResult(context.Context, [32]byte) (surveyport.PublicSubmissionResult, error) {
	return surveyport.PublicSubmissionResult{}, ErrUnavailable
}
func (s *analyticsStore) PublicAnalytics(context.Context, PublicDefinitionRecord) (surveyport.PublicAnalytics, error) {
	return s.analytics, nil
}

func TestPublicAnalyticsVersionZeroUsesCurrentPublicSnapshot(t *testing.T) {
	store := &analyticsStore{
		current:   PublicDefinitionRecord{ID: 9, State: "public", View: surveyport.PublicQuestionnaire{ID: 7, Slug: "public", Version: 3}},
		analytics: surveyport.PublicAnalytics{QuestionnaireID: 7, DefinitionVersion: 3, Slug: "public", State: "public", Questions: []surveyport.PublicAnalyticsQuestion{}},
	}
	var key [32]byte
	key[0] = 1
	service := NewPublicService(testUOW{}, store, &testEvents{}, key)
	got, err := service.Analytics(context.Background(), 7, 0)
	if err != nil || got.QuestionnaireID != 7 || got.DefinitionVersion != 3 || store.currentCalls != 1 || store.explicitCalls != 0 {
		t.Fatalf("analytics=%+v err=%v current=%d explicit=%d", got, err, store.currentCalls, store.explicitCalls)
	}
	store.currentErr = ErrNotFound
	if _, err = service.Analytics(context.Background(), 7, 0); err != ErrNotFound {
		t.Fatalf("no current public definition error=%v", err)
	}
	store.currentErr = nil
	store.current.State = "disabled"
	if _, err = service.Analytics(context.Background(), 7, 0); err != ErrNotFound {
		t.Fatalf("non-public current definition error=%v", err)
	}
	if _, err = service.Analytics(context.Background(), 7, -1); err != ErrInvalidPublicInput {
		t.Fatalf("negative version error=%v", err)
	}
}
