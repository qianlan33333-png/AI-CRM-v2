package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

type adminGroupOpsHistoryStub struct {
	err                 error
	empty, inconsistent bool
	calls               int
	kind                string
	planID              int64
	limit, offset       int32
}

func (s *adminGroupOpsHistoryStub) capture(kind string, id int64, limit, offset int32) int64 {
	s.calls++
	s.kind, s.planID, s.limit, s.offset = kind, id, limit, offset
	if s.empty && !s.inconsistent {
		return 0
	}
	return 21
}
func (s *adminGroupOpsHistoryStub) ListHistoricalPlans(_ context.Context, limit, offset int32) ([]groupopsport.HistoricalPlan, int64, error) {
	total := s.capture("plans", 0, limit, offset)
	if s.empty {
		return nil, total, s.err
	}
	at := time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)
	return []groupopsport.HistoricalPlan{{Plan: groupopsport.Plan{ID: 7, Name: "原计划", Status: groupopsport.PlanArchived, Revision: 1, CreatedBy: 2, UpdatedBy: 2, CreatedAt: at, UpdatedAt: at}, SourcePlanID: 81, SourceCode: " old-code ", PlanType: "old-kind", OriginalStatus: "active"}}, total, s.err
}
func (s *adminGroupOpsHistoryStub) ListHistoricalDirectory(_ context.Context, limit, offset int32) ([]groupopsport.HistoricalDirectory, int64, error) {
	total := s.capture("directory", 0, limit, offset)
	if s.empty {
		return nil, total, s.err
	}
	internal, external := int32(0), int32(31)
	return []groupopsport.HistoricalDirectory{{ID: 8, SourceKind: "wecom_group_chat_snapshots", ChatReference: "historical-chat", InternalMemberCount: &internal, ExternalMemberCount: &external, OriginalStatus: "", RecordedAt: time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)}}, total, s.err
}
func (s *adminGroupOpsHistoryStub) ListHistoricalGroups(_ context.Context, id int64, limit, offset int32) ([]groupopsport.HistoricalGroup, int64, error) {
	total := s.capture("groups", id, limit, offset)
	if s.empty {
		return nil, total, s.err
	}
	at := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	removed := at.Add(-time.Hour)
	return []groupopsport.HistoricalGroup{{ID: 9, SourceGroupID: 82, SourcePlanID: 81, PlanID: id, ChatReference: "history-group", DisplayName: " 原群名 ", OriginalStatus: "removed", CreatedAt: at, RemovedAt: &removed}}, total, s.err
}
func (s *adminGroupOpsHistoryStub) ListHistoricalNodes(_ context.Context, id int64, limit, offset int32) ([]groupopsport.HistoricalNode, int64, error) {
	total := s.capture("nodes", id, limit, offset)
	if s.empty {
		return nil, total, s.err
	}
	at := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	return []groupopsport.HistoricalNode{{ID: 10, SourceNodeID: 83, SourcePlanID: 81, PlanID: id, DayIndex: 0, TriggerTime: " 次日8:05 ", SortOrder: 0, OriginalStatus: "paused", ContentPackage: json.RawMessage(`{"number":9007199254740993,"items":[2,1]}`), CreatedAt: at, UpdatedAt: at.Add(-time.Hour)}}, total, s.err
}

type adminGroupOpsHistoryAuth struct {
	recordingAuth
	role      authport.Role
	csrfCalls int
}

func (s *adminGroupOpsHistoryAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 1, Role: s.role}, nil
}
func (s *adminGroupOpsHistoryAuth) Authorize(ctx context.Context, p authport.Principal, c authport.Capability) (authport.Authorization, error) {
	if p.Role != authport.RoleAdmin || c != authport.CapabilityAdminRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return s.recordingAuth.Authorize(ctx, p, c)
}
func (s *adminGroupOpsHistoryAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	s.csrfCalls++
	return nil
}

// Test-only composition. Production route registration is a separate main integration.
func adminGroupOpsHistoryRouter(t *testing.T, reader groupopsport.HistoricalReader, auth *adminGroupOpsHistoryAuth) http.Handler {
	t.Helper()
	h := newAdminGroupOpsHistory(reader)
	a, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	for _, route := range []struct {
		path    string
		handler http.HandlerFunc
	}{{adminGroupOpsHistoryPlansPath, h.ListPlans}, {adminGroupOpsHistoryDirectoryPath, h.ListDirectory}, {adminGroupOpsHistoryGroupsPath, h.ListGroups}, {adminGroupOpsHistoryNodesPath, h.ListNodes}} {
		protected, err := a.Authorize(authport.CapabilityAdminRead, route.handler)
		if err != nil {
			t.Fatal(err)
		}
		r.Method(http.MethodGet, route.path, a.Authenticate(protected))
	}
	return r
}

var adminGroupOpsHistoryPaths = []string{adminGroupOpsHistoryPlansPath, adminGroupOpsHistoryDirectoryPath, strings.ReplaceAll(adminGroupOpsHistoryGroupsPath, "{plan_id}", "7"), strings.ReplaceAll(adminGroupOpsHistoryNodesPath, "{plan_id}", "7")}

func TestAdminGroupOpsHistoryDTOsAndReadOnlyPagination(t *testing.T) {
	s := &adminGroupOpsHistoryStub{}
	auth := &adminGroupOpsHistoryAuth{role: authport.RoleAdmin}
	r := adminGroupOpsHistoryRouter(t, s, auth)
	for index, path := range adminGroupOpsHistoryPaths {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, legacyRequest(http.MethodGet, path+"?limit=20&offset=20", legacyToken(71)))
		var page struct {
			Source        string            `json:"source"`
			ReadOnly      bool              `json:"read_only"`
			External      bool              `json:"real_external_call_executed"`
			PlanID        string            `json:"plan_id"`
			Items         []json.RawMessage `json:"items"`
			Total         int64             `json:"total"`
			Limit, Offset int32
		}
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Source != "v1_history" || !page.ReadOnly || page.External || page.Total != 21 || page.Limit != 20 || page.Offset != 20 || len(page.Items) != 1 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("response %d %s", response.Code, response.Body)
		}
		if s.kind != []string{"plans", "directory", "groups", "nodes"}[index] || s.limit != 20 || s.offset != 20 || index > 1 && (page.PlanID != "7" || s.planID != 7) {
			t.Fatal("reader lost route or pagination")
		}
		for _, want := range [][]string{
			{`"plan_id":"7"`, `"status":"archived"`, `"original_status":"active"`, `"source_code":" old-code "`, `"owner_staff_id":null`, `"archived_at":null`, `"created_at":"2026-08-28T00:00:00.123456Z"`},
			{`"source_kind":"wecom_group_chat_snapshots"`, `"source_id":null`, `"display_name":null`, `"owner_name":null`, `"member_count":null`, `"original_status":""`},
			{`"plan_id":"7"`, `"source_plan_id":81`, `"owner_staff_id":null`, `"display_name":" 原群名 "`, `"removed_at":"2026-08-27T23:00:00Z"`},
			{`"plan_id":"7"`, `"day_index":0`, `"sort_order":0`, `"trigger_time":" 次日8:05 "`, `"original_status":"paused"`, `"number":9007199254740993`, `"items":[2,1]`},
		}[index] {
			if !strings.Contains(string(page.Items[0]), want) {
				t.Fatalf("missing fact %s: %s", want, page.Items[0])
			}
		}
	}
	if auth.csrfCalls != 0 || len(auth.capabilities()) != 4 {
		t.Fatal("GET must use admin.read without CSRF")
	}
}

func TestAdminGroupOpsHistoryInvalidQueryAndIDDoNotRead(t *testing.T) {
	s := &adminGroupOpsHistoryStub{}
	r := adminGroupOpsHistoryRouter(t, s, &adminGroupOpsHistoryAuth{role: authport.RoleAdmin})
	for _, path := range adminGroupOpsHistoryPaths {
		for _, query := range []string{"limit=0", "limit=101", "limit=-1", "limit=", "limit=1.5", "limit=1&limit=2", "offset=-1", "offset=", "offset=0&offset=1", "offset=2147483648", "execute=true", "plan_id=2", "limit=%zz", "limit=1;offset=2"} {
			response := httptest.NewRecorder()
			r.ServeHTTP(response, legacyRequest(http.MethodGet, path+"?"+query, legacyToken(72)))
			if response.Code != 400 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("query %s: %d %s", query, response.Code, response.Body)
			}
		}
	}
	for _, pattern := range []string{adminGroupOpsHistoryGroupsPath, adminGroupOpsHistoryNodesPath} {
		for _, id := range []string{"0", "-1", "01", "+1", "1.0", "x", "9223372036854775808"} {
			response := httptest.NewRecorder()
			r.ServeHTTP(response, legacyRequest(http.MethodGet, strings.ReplaceAll(pattern, "{plan_id}", id), legacyToken(72)))
			if response.Code != 400 {
				t.Fatalf("id %s: %d %s", id, response.Code, response.Body)
			}
		}
	}
	if s.calls != 0 {
		t.Fatal("invalid request reached reader")
	}
}

func TestAdminGroupOpsHistoryEmptyAndFailures(t *testing.T) {
	s := &adminGroupOpsHistoryStub{empty: true}
	r := adminGroupOpsHistoryRouter(t, s, &adminGroupOpsHistoryAuth{role: authport.RoleAdmin})
	for _, path := range adminGroupOpsHistoryPaths {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(73)))
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"items":[]`) || !strings.Contains(response.Body.String(), `"total":0`) || s.limit != 50 || s.offset != 0 {
			t.Fatalf("empty/default %d %s", response.Code, response.Body)
		}
		current := httptest.NewRequest(http.MethodGet, path, nil)
		current.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(73)})
		response = httptest.NewRecorder()
		r.ServeHTTP(response, current)
		if response.Code != 200 {
			t.Fatalf("current V2 cookie: %d", response.Code)
		}
	}
	for _, failure := range []string{"reader", "inconsistent"} {
		s.err = nil
		s.inconsistent = failure == "inconsistent"
		if failure == "reader" {
			s.err = errors.New("secret database payload")
		}
		for _, path := range adminGroupOpsHistoryPaths {
			response := httptest.NewRecorder()
			r.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(73)))
			if response.Code != 503 || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), `"items"`) || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("failure %d %s", response.Code, response.Body)
			}
		}
	}
	var missing *adminGroupOpsHistoryStub
	r = adminGroupOpsHistoryRouter(t, missing, &adminGroupOpsHistoryAuth{role: authport.RoleAdmin})
	response := httptest.NewRecorder()
	r.ServeHTTP(response, legacyRequest(http.MethodGet, adminGroupOpsHistoryPlansPath, legacyToken(73)))
	if response.Code != 503 {
		t.Fatal("typed nil reader did not fail closed")
	}
}

func TestAdminGroupOpsHistoryRequiresAdminRead(t *testing.T) {
	for _, role := range []authport.Role{"", authport.RoleOps, authport.RoleSales} {
		s := &adminGroupOpsHistoryStub{}
		auth := &adminGroupOpsHistoryAuth{role: role}
		r := adminGroupOpsHistoryRouter(t, s, auth)
		for _, path := range adminGroupOpsHistoryPaths {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			want := 401
			if role != "" {
				request = legacyRequest(http.MethodGet, path, legacyToken(74))
				want = 403
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != want {
				t.Fatalf("role %s: %d want %d", role, response.Code, want)
			}
		}
		if s.calls != 0 || auth.csrfCalls != 0 {
			t.Fatal("denied request reached reader or CSRF")
		}
	}
	s := &adminGroupOpsHistoryStub{}
	r := adminGroupOpsHistoryRouter(t, s, &adminGroupOpsHistoryAuth{role: authport.RoleAdmin})
	for _, path := range adminGroupOpsHistoryPaths {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, legacyRequest(http.MethodPost, path, legacyToken(75)))
		if response.Code != 405 {
			t.Fatal("history route accepted POST")
		}
	}
	if s.calls != 0 {
		t.Fatal("POST reached reader")
	}
}
