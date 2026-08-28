package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

var audienceHistoryListPaths = []struct {
	path, kind string
	parent     int64
	want       []string
}{
	{"/api/admin/audience-history/groups", "groups", 0, []string{`"source_id":101`, `"name":"历史分组"`, `"created_at":"2026-08-28T00:00:00.123456Z"`}},
	{"/api/admin/audience-history/packages", "packages", 0, []string{`"source_id":102`, `"group_history_id":null`, `"incremental_interval_seconds":-1`, `"runtime_digest":[2,2`}},
	{"/api/admin/audience-history/packages/71/versions", "versions", 71, []string{`"package_history_id":71`, `"version_number":-2`, `"template_version":-3`, `"definition_digest":[3,3`}},
	{"/api/admin/audience-history/packages/71/senders", "senders", 71, []string{`"package_history_id":71`, `"staff_id":null`, `"priority":-4`, `"original_status":"disabled"`}},
	{"/api/admin/audience-history/rules", "rules", 0, []string{`"source_id":105`, `"owner_staff_id":null`, `"rule_type":"legacy"`, `"original_status":"inactive"`}},
	{"/api/admin/audience-history/rules/72/versions", "rule_versions", 72, []string{`"rule_history_id":72`, `"version":-5`, `"published_at":null`, `"definition_digest":[6,6`}},
	{"/api/admin/audience-history/definitions", "definitions", 0, []string{`"source_id":107`, `"cached_headcount":-6`, `"usage_count":-7`, `"last_refreshed_at":null`}},
	{"/api/admin/audience-history/packages/71/members", "members", 71, []string{`"package_history_id":71`, `"customer_id":null`, `"identity_kind":"unionid"`, `"payload_digest":[8,8`}},
}

var audienceHistoryDetailPaths = []struct {
	path, kind string
	want       []string
}{
	{"/api/admin/audience-history/packages/81", "package", []string{`"id":81`, `"source_id":180`, `"current_version_source_id":42`, `"lookback_seconds":-8`, `"runtime_digest":[9,9`}},
	{"/api/admin/audience-history/definitions/82", "definition", []string{`"id":82`, `"source_id":181`, `"sql_dialect":"postgres"`, `"cached_headcount":-9`, `"definition_digest":[10,10`}},
}

func audienceHistoryFinalRouter(t *testing.T, reader segmentport.AudienceHistoryReader, service *audienceHistoryAPIAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.audienceHistory = reader
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy,
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestFinalRouterAudienceHistoryAdminReadWithoutCSRF(t *testing.T) {
	reader := &audienceHistoryAPIReader{}
	service := &audienceHistoryAPIAuth{role: authport.RoleAdmin}
	router := audienceHistoryFinalRouter(t, reader, service)

	for _, route := range audienceHistoryListPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, route.path+"?limit=1&offset=0", legacyToken(0x91)))
		assertAudienceHistoryListResponse(t, response, route.want)
		if reader.kind != route.kind || reader.parentID != route.parent || reader.limit != 1 || reader.offset != 0 {
			t.Fatalf("%s reader input kind/parent/page=%s/%d/%d/%d", route.path, reader.kind, reader.parentID, reader.limit, reader.offset)
		}
	}
	for _, route := range audienceHistoryDetailPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, route.path, legacyToken(0x92)))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"source":"v1_history"`) || !strings.Contains(response.Body.String(), `"read_only":true`) || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
			t.Fatalf("%s response=%d %s", route.path, response.Code, response.Body.String())
		}
		for _, want := range route.want {
			if !strings.Contains(response.Body.String(), want) {
				t.Fatalf("%s missing %s: %s", route.path, want, response.Body.String())
			}
		}
		if reader.kind != route.kind {
			t.Fatalf("%s reader kind=%q", route.path, reader.kind)
		}
	}
	if reader.calls != len(audienceHistoryListPaths)+len(audienceHistoryDetailPaths) || service.csrfCalls != 0 || len(service.capabilities) != reader.calls {
		t.Fatalf("calls/csrf/capabilities=%d/%d/%v", reader.calls, service.csrfCalls, service.capabilities)
	}
	for _, capability := range service.capabilities {
		if capability != authport.CapabilityAdminRead {
			t.Fatalf("unexpected capability %q", capability)
		}
	}
	currentCookie := httptest.NewRequest(http.MethodGet, audienceHistoryListPaths[0].path+"?limit=1", nil)
	currentCookie.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(0x93)})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, currentCookie)
	if response.Code != http.StatusOK || service.csrfCalls != 0 {
		t.Fatalf("current human session status/csrf=%d/%d", response.Code, service.csrfCalls)
	}
}

func TestFinalRouterAudienceHistoryProtectsReads(t *testing.T) {
	for _, test := range []struct {
		name    string
		role    authport.Role
		request func(string) *http.Request
		want    int
	}{
		{"anonymous", "", func(path string) *http.Request { return httptest.NewRequest(http.MethodGet, path, nil) }, http.StatusUnauthorized},
		{"ops", authport.RoleOps, func(path string) *http.Request { return legacyRequest(http.MethodGet, path, legacyToken(0x94)) }, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &audienceHistoryAPIReader{}
			service := &audienceHistoryAPIAuth{role: test.role}
			router := audienceHistoryFinalRouter(t, reader, service)
			for _, route := range audienceHistoryListPaths {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, test.request(route.path+"?limit=1"))
				if response.Code != test.want {
					t.Fatalf("%s %s status=%d body=%s", test.name, route.path, response.Code, response.Body.String())
				}
			}
			for _, route := range audienceHistoryDetailPaths {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, test.request(route.path))
				if response.Code != test.want {
					t.Fatalf("%s %s status=%d body=%s", test.name, route.path, response.Code, response.Body.String())
				}
			}
			if reader.calls != 0 || service.csrfCalls != 0 {
				t.Fatalf("denied request calls/csrf=%d/%d", reader.calls, service.csrfCalls)
			}
		})
	}
}

func TestFinalRouterAudienceHistoryRejectsNonCanonicalPathsAndQueries(t *testing.T) {
	reader := &audienceHistoryAPIReader{}
	router := audienceHistoryFinalRouter(t, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
	for _, route := range audienceHistoryListPaths {
		for _, query := range []string{"limit=0", "limit=101", "limit=-1", "limit=", "limit=1.5", "limit=1&limit=2", "offset=-1", "offset=0&offset=1", "offset=2147483648", "unknown=true", "package_id=2", "limit=%zz", "limit=1;offset=2"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, route.path+"?"+query, legacyToken(0x95)))
			if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s?%s response=%d %s", route.path, query, response.Code, response.Body.String())
			}
		}
	}
	for _, route := range audienceHistoryDetailPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, route.path+"?limit=1", legacyToken(0x96)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("detail query %s status=%d", route.path, response.Code)
		}
	}
	for _, pattern := range []string{
		"/api/admin/audience-history/packages/{id}/versions", "/api/admin/audience-history/packages/{id}/senders", "/api/admin/audience-history/packages/{id}/members", "/api/admin/audience-history/rules/{id}/versions", "/api/admin/audience-history/packages/{id}", "/api/admin/audience-history/definitions/{id}",
	} {
		for _, id := range []string{"0", "-1", "01", "+1", "1.0", "x", "9223372036854775808"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, strings.ReplaceAll(pattern, "{id}", id), legacyToken(0x97)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("path %s id=%s status=%d body=%s", pattern, id, response.Code, response.Body.String())
			}
		}
	}
	if reader.calls != 0 {
		t.Fatalf("invalid request reached reader %d times", reader.calls)
	}
}

func TestFinalRouterAudienceHistoryNilEmptyAndFailuresFailClosed(t *testing.T) {
	reader := &audienceHistoryAPIReader{empty: true}
	service := &audienceHistoryAPIAuth{role: authport.RoleAdmin}
	router := audienceHistoryFinalRouter(t, reader, service)
	for _, route := range audienceHistoryListPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, route.path, legacyToken(0x98)))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) || !strings.Contains(response.Body.String(), `"total":0`) || reader.limit != 50 || reader.offset != 0 {
			t.Fatalf("empty %s response=%d %s", route.path, response.Code, response.Body.String())
		}
	}
	for _, failure := range []string{"reader", "count"} {
		reader.empty, reader.err, reader.inconsistent = false, nil, failure == "count"
		if failure == "reader" {
			reader.err = errors.New("database detail must not leak")
		}
		for _, route := range audienceHistoryListPaths {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, route.path+"?limit=2", legacyToken(0x99)))
			if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "database detail") || strings.Contains(response.Body.String(), `"items"`) || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s %s response=%d %s", failure, route.path, response.Code, response.Body.String())
			}
		}
	}
	reader.empty, reader.inconsistent, reader.err = false, false, errors.New("detail database failure")
	for _, route := range audienceHistoryDetailPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, route.path, legacyToken(0x9a)))
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "detail database") {
			t.Fatalf("detail failure %s response=%d %s", route.path, response.Code, response.Body.String())
		}
	}
	var typedNil *audienceHistoryAPIReader
	typedNilRouter := audienceHistoryFinalRouter(t, typedNil, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
	for _, path := range append(audienceHistoryRouteStrings(), audienceHistoryDetailRouteStrings()...) {
		response := httptest.NewRecorder()
		typedNilRouter.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0x9b)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("typed nil %s status=%d", path, response.Code)
		}
	}
}

func TestFinalRouterAudienceHistoryReadOnlyRoutesRejectWrites(t *testing.T) {
	reader := &audienceHistoryAPIReader{}
	router := audienceHistoryFinalRouter(t, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
	for _, path := range append(audienceHistoryRouteStrings(), audienceHistoryDetailRouteStrings()...) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodPost, path, legacyToken(0x9c)))
		if response.Code != http.StatusBadRequest && response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	if reader.calls != 0 {
		t.Fatalf("write reached reader %d times", reader.calls)
	}
}

func assertAudienceHistoryListResponse(t *testing.T, response *httptest.ResponseRecorder, wants []string) {
	t.Helper()
	var payload struct {
		Source   string            `json:"source"`
		ReadOnly bool              `json:"read_only"`
		External bool              `json:"real_external_call_executed"`
		Items    []json.RawMessage `json:"items"`
		Total    int64             `json:"total"`
		Limit    int32             `json:"limit"`
		Offset   int32             `json:"offset"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &payload) != nil || payload.Source != "v1_history" || !payload.ReadOnly || payload.External || payload.Total != 1 || payload.Limit != 1 || payload.Offset != 0 || len(payload.Items) != 1 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list response=%d %s", response.Code, response.Body.String())
	}
	for _, want := range wants {
		if !strings.Contains(string(payload.Items[0]), want) {
			t.Fatalf("missing %s: %s", want, payload.Items[0])
		}
	}
}

func audienceHistoryRouteStrings() []string {
	paths := make([]string, 0, len(audienceHistoryListPaths))
	for _, route := range audienceHistoryListPaths {
		paths = append(paths, route.path)
	}
	return paths
}
func audienceHistoryDetailRouteStrings() []string {
	paths := make([]string, 0, len(audienceHistoryDetailPaths))
	for _, route := range audienceHistoryDetailPaths {
		paths = append(paths, route.path)
	}
	return paths
}

type audienceHistoryAPIReader struct {
	err                 error
	empty, inconsistent bool
	calls               int
	kind                string
	parentID            int64
	limit, offset       int32
}

func (reader *audienceHistoryAPIReader) capture(kind string, parentID int64, limit, offset int32) int64 {
	reader.calls++
	reader.kind, reader.parentID, reader.limit, reader.offset = kind, parentID, limit, offset
	if reader.empty {
		return 0
	}
	if reader.inconsistent {
		return 2
	}
	return 1
}
func (reader *audienceHistoryAPIReader) ListHistoricalAudienceGroups(_ context.Context, limit, offset int32) ([]segmentport.HistoricalAudienceGroup, int64, error) {
	total := reader.capture("groups", 0, limit, offset)
	if reader.empty {
		return nil, total, reader.err
	}
	return []segmentport.HistoricalAudienceGroup{{ID: 11, SourceID: 101, Name: "历史分组", CreatedAt: audienceHistoryAPITime(), UpdatedAt: audienceHistoryAPITime()}}, total, reader.err
}
func (reader *audienceHistoryAPIReader) ListHistoricalAudiencePackages(_ context.Context, limit, offset int32) ([]segmentport.HistoricalAudiencePackage, int64, error) {
	total := reader.capture("packages", 0, limit, offset)
	if reader.empty {
		return nil, total, reader.err
	}
	version := int64(41)
	return []segmentport.HistoricalAudiencePackage{{ID: 12, SourceID: 102, CurrentVersionSourceID: &version, PackageKey: "legacy-package", Name: "历史包", NaturalLanguageDefinition: "历史描述", OriginalStatus: "active", QueryMode: "legacy", IdentityPolicy: "unionid", IncrementalIntervalSecs: -1, LookbackSecs: -2, Timezone: "Asia/Shanghai", CreatedAt: audienceHistoryAPITime(), UpdatedAt: audienceHistoryAPITime(), RuntimeDigest: audienceHistoryAPIDigest(2)}}, total, reader.err
}
func (reader *audienceHistoryAPIReader) ListHistoricalAudienceVersions(_ context.Context, packageID int64, limit, offset int32) ([]segmentport.HistoricalAudienceVersion, int64, error) {
	total := reader.capture("versions", packageID, limit, offset)
	if reader.empty {
		return nil, total, reader.err
	}
	templateVersion := int64(-3)
	return []segmentport.HistoricalAudienceVersion{{ID: 13, SourceID: 103, PackageHistoryID: packageID, VersionNumber: -2, OriginalStatus: "published", AIPrompt: "历史提示", AIRationale: "历史依据", NaturalLanguageExplanation: "历史解释", CreatedAt: audienceHistoryAPITime(), TemplateKey: "legacy", TemplateVersion: &templateVersion, DefinitionDigest: audienceHistoryAPIDigest(3)}}, total, reader.err
}
func (reader *audienceHistoryAPIReader) ListHistoricalAudienceSenders(_ context.Context, packageID int64, limit, offset int32) ([]segmentport.HistoricalAudienceSender, int64, error) {
	total := reader.capture("senders", packageID, limit, offset)
	if reader.empty {
		return nil, total, reader.err
	}
	return []segmentport.HistoricalAudienceSender{{ID: 14, SourceID: 104, PackageHistoryID: packageID, DisplayName: "历史发送人", Priority: -4, OriginalStatus: "disabled", CreatedAt: audienceHistoryAPITime(), UpdatedAt: audienceHistoryAPITime()}}, total, reader.err
}
func (reader *audienceHistoryAPIReader) ListHistoricalAudienceRules(_ context.Context, limit, offset int32) ([]segmentport.HistoricalAudienceRule, int64, error) {
	total := reader.capture("rules", 0, limit, offset)
	if reader.empty {
		return nil, total, reader.err
	}
	return []segmentport.HistoricalAudienceRule{{ID: 15, SourceID: 105, RuleKey: "legacy-rule", DisplayName: "历史规则", Description: "历史描述", RuleType: "legacy", OriginalStatus: "inactive", CreatedAt: audienceHistoryAPITime(), UpdatedAt: audienceHistoryAPITime()}}, total, reader.err
}
func (reader *audienceHistoryAPIReader) ListHistoricalAudienceRuleVersions(_ context.Context, ruleID int64, limit, offset int32) ([]segmentport.HistoricalAudienceRuleVersion, int64, error) {
	total := reader.capture("rule_versions", ruleID, limit, offset)
	if reader.empty {
		return nil, total, reader.err
	}
	return []segmentport.HistoricalAudienceRuleVersion{{ID: 16, SourceID: 106, RuleHistoryID: ruleID, Version: -5, ExecutorType: "legacy", OriginalStatus: "archived", CreatedAt: audienceHistoryAPITime(), DefinitionDigest: audienceHistoryAPIDigest(6)}}, total, reader.err
}
func (reader *audienceHistoryAPIReader) ListHistoricalAudienceDefinitions(_ context.Context, limit, offset int32) ([]segmentport.HistoricalAudienceDefinition, int64, error) {
	total := reader.capture("definitions", 0, limit, offset)
	if reader.empty {
		return nil, total, reader.err
	}
	return []segmentport.HistoricalAudienceDefinition{{ID: 17, SourceID: 107, Code: "legacy-code", DisplayName: "历史定义", Description: "历史描述", SourceType: "sql", SQLDialect: "postgres", OriginalStatus: "active", Version: -1, CachedHeadcount: -6, UsageCount: -7, CreatedAt: audienceHistoryAPITime(), UpdatedAt: audienceHistoryAPITime(), DefinitionDigest: audienceHistoryAPIDigest(7)}}, total, reader.err
}
func (reader *audienceHistoryAPIReader) ListHistoricalAudienceMembers(_ context.Context, packageID int64, limit, offset int32) ([]segmentport.HistoricalAudienceMember, int64, error) {
	total := reader.capture("members", packageID, limit, offset)
	if reader.empty {
		return nil, total, reader.err
	}
	return []segmentport.HistoricalAudienceMember{{ID: 18, SourceID: 108, PackageHistoryID: packageID, IdentityKind: "unionid", OriginalStatus: "exited", FirstEnteredAt: audienceHistoryAPITime(), LastSeenAt: audienceHistoryAPITime(), LastUpdatedAt: audienceHistoryAPITime(), CreatedAt: audienceHistoryAPITime(), UpdatedAt: audienceHistoryAPITime(), PayloadDigest: audienceHistoryAPIDigest(8)}}, total, reader.err
}
func (reader *audienceHistoryAPIReader) GetHistoricalAudiencePackage(_ context.Context, id int64) (segmentport.HistoricalAudiencePackage, error) {
	reader.capture("package", id, 0, 0)
	version := int64(42)
	return segmentport.HistoricalAudiencePackage{ID: id, SourceID: 180, CurrentVersionSourceID: &version, PackageKey: "detail-package", Name: "历史包", NaturalLanguageDefinition: "历史描述", OriginalStatus: "paused", QueryMode: "legacy", IdentityPolicy: "unionid", LookbackSecs: -8, CreatedAt: audienceHistoryAPITime(), UpdatedAt: audienceHistoryAPITime(), RuntimeDigest: audienceHistoryAPIDigest(9)}, reader.err
}
func (reader *audienceHistoryAPIReader) GetHistoricalAudienceDefinition(_ context.Context, id int64) (segmentport.HistoricalAudienceDefinition, error) {
	reader.capture("definition", id, 0, 0)
	return segmentport.HistoricalAudienceDefinition{ID: id, SourceID: 181, Code: "detail-definition", DisplayName: "历史定义", Description: "历史描述", SourceType: "sql", SQLDialect: "postgres", OriginalStatus: "archived", Version: -2, CachedHeadcount: -9, UsageCount: -10, CreatedAt: audienceHistoryAPITime(), UpdatedAt: audienceHistoryAPITime(), DefinitionDigest: audienceHistoryAPIDigest(10)}, reader.err
}

func audienceHistoryAPITime() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC) }
func audienceHistoryAPIDigest(seed byte) [32]byte {
	var digest [32]byte
	for index := range digest {
		digest[index] = seed
	}
	return digest
}

type audienceHistoryAPIAuth struct {
	role         authport.Role
	csrfCalls    int
	capabilities []authport.Capability
}

func (service *audienceHistoryAPIAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 1, Role: service.role}, nil
}
func (service *audienceHistoryAPIAuth) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.Role != authport.RoleAdmin || capability != authport.CapabilityAdminRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	service.capabilities = append(service.capabilities, capability)
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}
func (service *audienceHistoryAPIAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	service.csrfCalls++
	return nil
}
func (*audienceHistoryAPIAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

var _ segmentport.AudienceHistoryReader = (*audienceHistoryAPIReader)(nil)
var _ authport.Service = (*audienceHistoryAPIAuth)(nil)
