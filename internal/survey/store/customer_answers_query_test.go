package store

import (
	"os"
	"strings"
	"testing"
)

func TestCustomerAnswerCandidateQueryReadsOnlyMatchingHintsAndSafeAnswerFields(t *testing.T) {
	raw, err := os.ReadFile("queries/submissions.sql")
	if err != nil {
		t.Fatal(err)
	}
	contents := string(raw)
	start := strings.Index(contents, "-- name: ListRecentCustomerAnswerCandidates :many")
	end := strings.Index(contents[start+1:], "-- name:")
	if start < 0 || end < 0 {
		t.Fatal("customer answer candidate query boundary is missing")
	}
	query := contents[start : start+1+end]
	for _, forbidden := range []string{
		"respondent_key", "openid", "customer_name", "question_title", "option_text", "text_value",
		"follow_user_userid", "result_token", "redirect_url_snapshot",
	} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("candidate query reads forbidden field %q: %s", forbidden, query)
		}
	}
	for _, required := range []string{"unionid", "external_userid", "mobile", "option_id", "LIMIT sqlc.arg(row_limit)"} {
		if !strings.Contains(query, required) {
			t.Fatalf("candidate query is missing %q: %s", required, query)
		}
	}
}
