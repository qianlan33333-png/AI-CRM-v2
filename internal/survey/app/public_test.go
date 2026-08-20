package app

import (
	"crypto/sha256"
	"encoding/base64"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	"testing"
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
	if _, err = PublicDefinition(source); err == nil {
		t.Fatal("mobile must not publish")
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
