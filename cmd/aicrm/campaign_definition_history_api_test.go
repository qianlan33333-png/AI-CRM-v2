package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

type definitionHistoryAPIReader struct {
	calls          int
	empty, invalid bool
	filter         *int64
	err            error
}

func (f *definitionHistoryAPIReader) GetHistoricalCampaignDefinition(context.Context, int64) (campaignport.HistoricalCampaignDefinition, error) {
	f.calls++
	v := definitionHistoryAPIFact()
	if f.invalid {
		v.PrivateDigest = [32]byte{}
	}
	return v, f.err
}
func (f *definitionHistoryAPIReader) ListHistoricalCampaignDefinitions(ctx context.Context, _, _ int32) ([]campaignport.HistoricalCampaignDefinition, int64, error) {
	v, e := f.GetHistoricalCampaignDefinition(ctx, 1)
	if f.empty {
		return nil, 0, e
	}
	return []campaignport.HistoricalCampaignDefinition{v}, 1, e
}
func (f *definitionHistoryAPIReader) ListHistoricalCampaignDefinitionSteps(_ context.Context, filter *int64, _, _ int32) ([]campaignport.HistoricalCampaignDefinitionStep, int64, error) {
	f.calls++
	f.filter = filter
	if f.empty {
		return nil, 0, f.err
	}
	v := definitionHistoryAPIFact()
	return []campaignport.HistoricalCampaignDefinitionStep{{ID: 2, SourceID: -2, CampaignSourceID: -1, SourceParentState: "unresolved_definition", CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, OriginalDisposition: "quarantine", OriginalReason: "legacy_shape", ContentDigest: [32]byte{1}, PrivateDigest: v.PrivateDigest, SourceKeyDigest: v.SourceKeyDigest, SourcePayloadDigest: v.SourcePayloadDigest, SourceFieldDigest: v.SourceFieldDigest}}, 1, f.err
}
func definitionHistoryAPIFact() campaignport.HistoricalCampaignDefinition {
	stamp := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	return campaignport.HistoricalCampaignDefinition{ID: 1, SourceID: -1, Code: "legacy", CreatedAt: stamp, UpdatedAt: stamp, OriginalDisposition: "archive", OriginalReason: "not_current", PrivateDigest: [32]byte{1}, SourceKeyDigest: [32]byte{2}, SourcePayloadDigest: [32]byte{3}, SourceFieldDigest: [32]byte{4}, RedactedRoots: []string{"approval_token_hash"}}
}
func definitionHistoryAPIRouter(t *testing.T, reader campaignport.CampaignDefinitionHistoryReader, role authport.Role) http.Handler {
	t.Helper()
	auth := &legacyAuthStub{principal: authport.Principal{AdminUserID: 1, Role: role}, csrfErr: authport.ErrUnauthorized}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.campaignDefinitionHistory = reader
	h, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), h, h, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
func TestCampaignDefinitionHistoryAPIReadOnlyAndPrivateFields(t *testing.T) {
	f := &definitionHistoryAPIReader{}
	router := definitionHistoryAPIRouter(t, f, authport.RoleAdmin)
	paths := []string{"/api/admin/campaign-history/definitions", "/api/admin/campaign-history/definitions/1", "/api/admin/campaign-history/definition-steps?campaign_source_id=-1"}
	for _, path := range paths {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, legacyRequest(http.MethodGet, path, legacyToken(0xb1)))
		body := w.Body.String()
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, `"read_only":true`) || !strings.Contains(body, `"real_external_call_executed":false`) {
			t.Fatalf("%s: %d %s", path, w.Code, body)
		}
		for _, private := range []string{"Digest", "digest", "RedactedRoots", "approval_token_hash"} {
			if strings.Contains(body, private) {
				t.Fatalf("private field leaked: %s", private)
			}
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		before := f.calls
		w = httptest.NewRecorder()
		router.ServeHTTP(w, legacyRequest(http.MethodPost, path, legacyToken(0xb1)))
		if w.Code < 400 || f.calls != before {
			t.Fatal("history write reached reader")
		}
		for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			want := 401
			if role == authport.RoleOps {
				r = legacyRequest(http.MethodGet, path, legacyToken(0xb1))
				want = 403
			}
			w = httptest.NewRecorder()
			definitionHistoryAPIRouter(t, f, role).ServeHTTP(w, r)
			if w.Code != want || f.calls != before {
				t.Fatalf("auth %s=%d", role, w.Code)
			}
		}
	}
	if f.filter == nil || *f.filter != -1 {
		t.Fatal("signed source filter lost")
	}
}
func TestCampaignDefinitionHistoryAPIRejectsInvalidAndFailsClosed(t *testing.T) {
	f := &definitionHistoryAPIReader{}
	router := definitionHistoryAPIRouter(t, f, authport.RoleAdmin)
	for _, suffix := range []string{"definitions?limit=0", "definitions?offset=-1", "definitions?limit=1&limit=2", "definitions?mobile=123", "definitions/01", "definitions/1?limit=1", "definition-steps?campaign_source_id=01", "definition-steps?campaign_source_id=1&campaign_source_id=2", "definition-steps?x=1"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/campaign-history/"+suffix, legacyToken(0xb1)))
		if w.Code != 400 || f.calls != 0 {
			t.Fatalf("%s = %d", suffix, w.Code)
		}
	}
	for _, reader := range []campaignport.CampaignDefinitionHistoryReader{nil, &definitionHistoryAPIReader{err: campaignport.ErrCampaignHistoryUnavailable}, &definitionHistoryAPIReader{invalid: true}} {
		w := httptest.NewRecorder()
		definitionHistoryAPIRouter(t, reader, authport.RoleAdmin).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/campaign-history/definitions", legacyToken(0xb1)))
		if w.Code != 503 {
			t.Fatalf("failure = %d %s", w.Code, w.Body.String())
		}
	}
	f.empty = true
	w := httptest.NewRecorder()
	router.ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/campaign-history/definitions", legacyToken(0xb1)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("empty = %d %s", w.Code, w.Body.String())
	}
}
