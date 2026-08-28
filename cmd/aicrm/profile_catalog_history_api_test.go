package main

import (
	"context"
	"errors"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type profileHistoryTestReader struct {
	segmentport.ProfileCatalogHistoryReader
	contactport.SignupTagHistoryReader
	err                                       error
	empty, invalid, inconsistent, wrongParent bool
	calls                                     int
	query                                     segmentport.ProfileCatalogHistoryQuery
}

func profileTestTemplate(id int64) segmentport.HistoricalProfileTemplate {
	return segmentport.HistoricalProfileTemplate{ID: id, SourceID: -7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, TemplateCode: "旧事实", TemplateName: "旧事实", QuestionnaireSourceID: nil, SegmentationQuestionSourceID: nil, ProgramSourceID: nil, Description: "旧事实", OriginalEnabled: false, Version: -7, CreatedByDigest: [32]byte{1}, UpdatedByDigest: [32]byte{1}, CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC), UpdatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)}
}
func (reader *profileHistoryTestReader) ListHistoricalProfileTemplates(ctx context.Context, query segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileTemplate, int64, error) {
	reader.calls++
	reader.query = query
	if reader.empty {
		return nil, 0, reader.err
	}
	item := profileTestTemplate(81)
	if reader.invalid {
		item.SourcePayloadDigest = [32]byte{}
	}

	total := int64(1)
	if reader.inconsistent {
		total = 2
	}
	return []segmentport.HistoricalProfileTemplate{item}, total, reader.err
}
func (reader *profileHistoryTestReader) GetHistoricalProfileTemplate(ctx context.Context, id int64) (segmentport.HistoricalProfileTemplate, error) {
	reader.calls++
	item := profileTestTemplate(id)
	if reader.invalid {
		item.SourcePayloadDigest = [32]byte{}
	}
	return item, reader.err
}
func profileTestCategory(id int64) segmentport.HistoricalProfileCategory {
	return segmentport.HistoricalProfileCategory{ID: id, SourceID: -7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, TemplateSourceID: -7, TemplateHistoryID: 71, CategoryKey: "旧事实", CategoryName: "旧事实", Description: "旧事实", SortOrder: -7, OriginalEnabled: false, CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC), UpdatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)}
}
func (reader *profileHistoryTestReader) ListHistoricalProfileCategories(ctx context.Context, query segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileCategory, int64, error) {
	reader.calls++
	reader.query = query
	if reader.empty {
		return nil, 0, reader.err
	}
	item := profileTestCategory(81)
	if reader.invalid {
		item.SourcePayloadDigest = [32]byte{}
	}
	if reader.wrongParent {
		item.TemplateHistoryID = 999
	}
	total := int64(1)
	if reader.inconsistent {
		total = 2
	}
	return []segmentport.HistoricalProfileCategory{item}, total, reader.err
}
func profileTestOptionMapping(id int64) segmentport.HistoricalProfileOptionMapping {
	return segmentport.HistoricalProfileOptionMapping{ID: id, SourceID: -7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, TemplateSourceID: -7, CategorySourceID: -7, TemplateHistoryID: 71, CategoryHistoryID: 72, QuestionSourceID: -7, OptionSourceID: -7, CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)}
}
func (reader *profileHistoryTestReader) ListHistoricalProfileOptionMappings(ctx context.Context, query segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileOptionMapping, int64, error) {
	reader.calls++
	reader.query = query
	if reader.empty {
		return nil, 0, reader.err
	}
	item := profileTestOptionMapping(81)
	if reader.invalid {
		item.SourcePayloadDigest = [32]byte{}
	}
	if reader.wrongParent {
		item.TemplateHistoryID = 999
	}
	total := int64(1)
	if reader.inconsistent {
		total = 2
	}
	return []segmentport.HistoricalProfileOptionMapping{item}, total, reader.err
}
func profileTestSignupTagRule(id int64) contactport.HistoricalSignupTagRule {
	return contactport.HistoricalSignupTagRule{ID: id, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, TagSourceID: "旧事实", TagName: "旧事实", SignupStatus: "旧事实", OriginalActive: false, UpdatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)}
}
func (reader *profileHistoryTestReader) ListHistoricalSignupTagRules(ctx context.Context, limit, offset int32) ([]contactport.HistoricalSignupTagRule, int64, error) {
	reader.calls++
	reader.query = segmentport.ProfileCatalogHistoryQuery{Limit: limit, Offset: offset}
	if reader.empty {
		return nil, 0, reader.err
	}
	item := profileTestSignupTagRule(81)
	if reader.invalid {
		item.SourcePayloadDigest = [32]byte{}
	}

	total := int64(1)
	if reader.inconsistent {
		total = 2
	}
	return []contactport.HistoricalSignupTagRule{item}, total, reader.err
}
func profileHistoryRouter(t *testing.T, reader *profileHistoryTestReader, auth authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	if reader != nil {
		legacy.profileCatalogHistory = reader
		legacy.signupTagHistory = reader
	}
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

var profileHistoryPaths = []string{
	"/api/admin/profile-catalog-history/templates",
	"/api/admin/profile-catalog-history/templates/71",
	"/api/admin/profile-catalog-history/templates/71/categories",
	"/api/admin/profile-catalog-history/templates/71/categories/72/option-mappings",
	"/api/admin/profile-catalog-history/signup-tag-rules",
}

func TestProfileHistoryFinalRouter(t *testing.T) {
	reader := &profileHistoryTestReader{}
	auth := &messageHistoryAPIAuth{role: authport.RoleAdmin}
	router := profileHistoryRouter(t, reader, auth)
	for index, path := range profileHistoryPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest("GET", path, legacyToken(0xb1)))
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
		for _, want := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"source_payload_digest":[1,0`} {
			if !strings.Contains(response.Body.String(), want) {
				t.Fatalf("missing %s", want)
			}
		}
		if index == 0 && (!strings.Contains(response.Body.String(), `"source_id":-7`) || !strings.Contains(response.Body.String(), `"original_enabled":false`)) {
			t.Fatal("source signed or false fact lost")
		}
		if index == 2 && (reader.query.TemplateHistoryID == nil || *reader.query.TemplateHistoryID != 71) {
			t.Fatal("template filter lost")
		}
		if index == 3 && (reader.query.CategoryHistoryID == nil || *reader.query.CategoryHistoryID != 72) {
			t.Fatal("category filter lost")
		}
	}
	if auth.csrfCalls != 0 {
		t.Fatal("read invokes CSRF")
	}
	for _, capability := range auth.capabilities {
		if capability != authport.CapabilityAdminRead {
			t.Fatal(capability)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest("GET", profileHistoryPaths[0]+"?limit=2&offset=0", legacyToken(0xb1)))
	if response.Code != 200 || reader.query.Limit != 2 || reader.query.Offset != 0 {
		t.Fatal("pagination not forwarded")
	}
}
func TestProfileHistoryDenialsAndInvalid(t *testing.T) {
	for _, role := range []authport.Role{"", authport.RoleOps} {
		reader := &profileHistoryTestReader{}
		router := profileHistoryRouter(t, reader, &messageHistoryAPIAuth{role: role})
		for _, path := range profileHistoryPaths {
			request := httptest.NewRequest("GET", path, nil)
			want := 401
			if role != "" {
				request = legacyRequest("GET", path, legacyToken(0xb2))
				want = 403
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != want {
				t.Fatalf("denied %s=%d", path, response.Code)
			}
		}
		if reader.calls != 0 {
			t.Fatal("denial reached DB")
		}
	}
	reader := &profileHistoryTestReader{}
	router := profileHistoryRouter(t, reader, &messageHistoryAPIAuth{role: authport.RoleAdmin})
	for _, query := range []string{"limit=0", "limit=101", "offset=-1", "limit=1&limit=2", "unknown=1", "limit=%zz", "limit=1;offset=2"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest("GET", profileHistoryPaths[0]+"?"+query, legacyToken(0xb2)))
		if response.Code != 400 {
			t.Fatalf("query %s=%d", query, response.Code)
		}
	}
	for _, path := range []string{"/api/admin/profile-catalog-history/templates/0", "/api/admin/profile-catalog-history/templates/01/categories", "/api/admin/profile-catalog-history/templates/71/categories/-1/option-mappings", profileHistoryPaths[1] + "?limit=2"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest("GET", path, legacyToken(0xb2)))
		if response.Code != 400 {
			t.Fatalf("path %s=%d", path, response.Code)
		}
	}
	for _, path := range profileHistoryPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest("POST", path, legacyToken(0xb2)))
		if response.Code >= 200 && response.Code < 300 {
			t.Fatal("unexpected history mutation")
		}
	}
	if reader.calls != 0 {
		t.Fatal("invalid reached DB")
	}
}
func TestProfileHistoryFailClosedAndEmpty(t *testing.T) {
	for _, reader := range []*profileHistoryTestReader{nil, {err: errors.New("db unavailable")}, {invalid: true}, {inconsistent: true}} {
		router := profileHistoryRouter(t, reader, &messageHistoryAPIAuth{role: authport.RoleAdmin})
		for index, path := range profileHistoryPaths {
			if reader != nil && reader.inconsistent && index == 1 {
				continue
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest("GET", path, legacyToken(0xb3)))
			if response.Code != 503 || strings.Contains(response.Body.String(), "旧事实") {
				t.Fatalf("failure leaked/succeeded %s=%d", path, response.Code)
			}
		}
	}
	router := profileHistoryRouter(t, &profileHistoryTestReader{wrongParent: true}, &messageHistoryAPIAuth{role: authport.RoleAdmin})
	for _, path := range profileHistoryPaths[2:4] {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest("GET", path, legacyToken(0xb3)))
		if response.Code != 503 {
			t.Fatal("cross-parent row leaked")
		}
	}
	router = profileHistoryRouter(t, &profileHistoryTestReader{empty: true}, &messageHistoryAPIAuth{role: authport.RoleAdmin})
	for index, path := range profileHistoryPaths {
		if index == 1 {
			continue
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest("GET", path, legacyToken(0xb3)))
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"items":[]`) {
			t.Fatal("empty is not real empty")
		}
	}
}
