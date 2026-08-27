package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
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

type publicManagementStore struct {
	PublicStore
	questionnaire surveyport.Questionnaire
	record        PublicDefinitionRecord
	receipt       PublicManagementReceipt
	rewrite       func(json.RawMessage) json.RawMessage
	creates       int
	disables      int
}

func (s *publicManagementStore) GetQuestionnaire(context.Context, surveyport.ID) (surveyport.Questionnaire, error) {
	return s.questionnaire, nil
}

func (s *publicManagementStore) CreatePublicDefinition(context.Context, surveyport.Questionnaire, time.Time) (PublicDefinitionRecord, error) {
	s.creates++
	return s.record, nil
}

func (s *publicManagementStore) DisablePublicDefinition(context.Context, surveyport.ID, int64, time.Time) (PublicDefinitionRecord, error) {
	s.disables++
	return s.record, nil
}

func (s *publicManagementStore) ReservePublicManagement(_ context.Context, _ string, receipt PublicManagementReceipt, _ time.Time) (PublicManagementReceipt, bool, error) {
	if s.receipt.ID > 0 {
		return s.receipt, false, nil
	}
	receipt.ID, receipt.State = 1, "in_progress"
	s.receipt = receipt
	return receipt, true, nil
}

func (s *publicManagementStore) CompletePublicManagement(_ context.Context, _ int64, snapshot json.RawMessage, _ time.Time) (PublicManagementReceipt, error) {
	s.receipt.State, s.receipt.ResultSnapshot = "completed", s.rewrite(snapshot)
	return s.receipt, nil
}

func TestPublicManagementJSONBSnapshotComparison(t *testing.T) {
	for _, operation := range []string{"publish", "disable"} {
		for _, tc := range []struct {
			name   string
			mutate func(*PublicDefinitionRecord)
		}{
			{name: "jsonb_whitespace_and_key_order"},
			{name: "storage_id", mutate: func(r *PublicDefinitionRecord) { r.ID++ }},
			{name: "state", mutate: func(r *PublicDefinitionRecord) { r.State = "other" }},
			{name: "questionnaire_id", mutate: func(r *PublicDefinitionRecord) { r.View.ID++ }},
			{name: "slug", mutate: func(r *PublicDefinitionRecord) { r.View.Slug = "other" }},
			{name: "title", mutate: func(r *PublicDefinitionRecord) { r.View.Title = "other" }},
			{name: "description", mutate: func(r *PublicDefinitionRecord) { r.View.Description = "other" }},
			{name: "display_mode", mutate: func(r *PublicDefinitionRecord) { r.View.AnswerDisplayMode = surveyport.OneByOne }},
			{name: "version", mutate: func(r *PublicDefinitionRecord) { r.View.Version++ }},
			{name: "questions_null", mutate: func(r *PublicDefinitionRecord) { r.View.Questions = nil }},
			{name: "questions_empty", mutate: func(r *PublicDefinitionRecord) { r.View.Questions = []surveyport.PublicQuestion{} }},
			{name: "question_id", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].ID++ }},
			{name: "question_type", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Type = surveyport.MultiChoice }},
			{name: "question_title", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Title = "other" }},
			{name: "question_required", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Required = false }},
			{name: "question_order", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].SortOrder++ }},
			{name: "question_minimum", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Minimum = 0 }},
			{name: "question_maximum", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Maximum++ }},
			{name: "options_null", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Options = nil }},
			{name: "options_empty", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Options = []surveyport.PublicOption{} }},
			{name: "option_id", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Options[0].ID++ }},
			{name: "option_text", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Options[0].OptionText = "other" }},
			{name: "option_order", mutate: func(r *PublicDefinitionRecord) { r.View.Questions[0].Options[0].SortOrder++ }},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				minimum, maximum := 1, 1
				// Adjacent int64 IDs above 2^53 must not become equal via float64.
				const id = 9007199254740992
				q := surveyport.Questionnaire{ID: id, Slug: "uat-public", Title: "UAT", Description: "Test only", AnswerDisplayMode: surveyport.AllInOne, Version: 2, Questions: []surveyport.Question{{ID: id, Type: surveyport.SingleChoice, Title: "Choose", Required: true, Validation: surveyport.Validation{MinimumSelections: &minimum, MaximumSelections: &maximum}, Options: []surveyport.Option{{ID: id, OptionText: "A"}}}}}
				view, err := PublicDefinition(q)
				if err != nil {
					t.Fatal(err)
				}
				state := "public"
				if operation == "disable" {
					state = "disabled"
				}
				store := &publicManagementStore{questionnaire: q, record: PublicDefinitionRecord{ID: id, State: state, View: view}}
				store.rewrite = func(snapshot json.RawMessage) json.RawMessage {
					var record PublicDefinitionRecord
					if err := json.Unmarshal(snapshot, &record); err != nil {
						t.Fatal(err)
					}
					if tc.mutate != nil {
						tc.mutate(&record)
					}
					encoded, err := json.Marshal(record)
					if err != nil {
						t.Fatal(err)
					}
					value, ok := decodeJSON(encoded)
					if !ok {
						t.Fatal("invalid fixture JSON")
					}
					// jsonb changes object key order and whitespace, not array order.
					encoded, err = json.MarshalIndent(value, "", "  ")
					if err != nil || string(encoded) == string(snapshot) {
						t.Fatalf("fixture did not rewrite snapshot: %v", err)
					}
					return encoded
				}
				events := &testEvents{}
				service := NewPublicService(testUOW{}, store, events, [32]byte{1})
				call := func() (PublicDefinitionRecord, error) {
					if operation == "publish" {
						return service.Publish(context.Background(), surveyport.PublishPublicDefinitionCommand{QuestionnaireID: q.ID, ExpectedQuestionnaireVersion: q.Version, Actor: 1, IdempotencyKey: "uat-public-publish-01"})
					}
					return service.Disable(context.Background(), surveyport.DisablePublicDefinitionCommand{QuestionnaireID: q.ID, ExpectedDefinitionVersion: q.Version, Actor: 1, IdempotencyKey: "uat-public-disable-01"})
				}
				got, err := call()
				if tc.mutate != nil {
					if !errors.Is(err, ErrPublicUnavailable) || got.ID != 0 || len(events.items) != 0 {
						t.Fatalf("changed snapshot accepted: result=%+v err=%v events=%d", got, err, len(events.items))
					}
					return
				}
				if err != nil || !reflect.DeepEqual(got, store.record) || len(events.items) != 1 {
					t.Fatalf("jsonb snapshot rejected: result=%+v err=%v events=%d", got, err, len(events.items))
				}
				replayed, err := call()
				if err != nil || !reflect.DeepEqual(replayed, got) || store.creates+store.disables != 1 || len(events.items) != 1 {
					t.Fatalf("replay changed result: result=%+v err=%v writes=%d events=%d", replayed, err, store.creates+store.disables, len(events.items))
				}
			})
		}
	}
}
