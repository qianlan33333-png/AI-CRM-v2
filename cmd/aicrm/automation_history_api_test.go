package main

import (
	"context"
	"errors"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type automationHistoryAPIStub struct {
	sop    automationport.HistoricalAutomationSOP
	config automationport.HistoricalAutomationConfig
	prompt automationport.HistoricalAutomationPrompt
	agent  automationport.HistoricalAutomationAgent
	err    error
	empty  bool
	total  int64
	calls  int
	kind   string
	id     int64
	query  automationport.AutomationHistoryQuery
}

func (s *automationHistoryAPIStub) GetHistoricalAutomationSOP(_ context.Context, id int64) (automationport.HistoricalAutomationSOP, error) {
	s.calls++
	s.kind = "sop"
	s.id = id
	return s.sop, s.err
}
func (s *automationHistoryAPIStub) ListHistoricalAutomationSOPs(_ context.Context, q automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationSOP, int64, error) {
	s.calls++
	s.kind = "sop"
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []automationport.HistoricalAutomationSOP{s.sop}, s.total, s.err
}

func (s *automationHistoryAPIStub) GetHistoricalAutomationConfig(_ context.Context, id int64) (automationport.HistoricalAutomationConfig, error) {
	s.calls++
	s.kind = "config"
	s.id = id
	return s.config, s.err
}
func (s *automationHistoryAPIStub) ListHistoricalAutomationConfigs(_ context.Context, q automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationConfig, int64, error) {
	s.calls++
	s.kind = "config"
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []automationport.HistoricalAutomationConfig{s.config}, s.total, s.err
}

func (s *automationHistoryAPIStub) GetHistoricalAutomationPrompt(_ context.Context, id int64) (automationport.HistoricalAutomationPrompt, error) {
	s.calls++
	s.kind = "prompt"
	s.id = id
	return s.prompt, s.err
}
func (s *automationHistoryAPIStub) ListHistoricalAutomationPrompts(_ context.Context, q automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationPrompt, int64, error) {
	s.calls++
	s.kind = "prompt"
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []automationport.HistoricalAutomationPrompt{s.prompt}, s.total, s.err
}

func (s *automationHistoryAPIStub) GetHistoricalAutomationAgent(_ context.Context, id int64) (automationport.HistoricalAutomationAgent, error) {
	s.calls++
	s.kind = "agent"
	s.id = id
	return s.agent, s.err
}
func (s *automationHistoryAPIStub) ListHistoricalAutomationAgents(_ context.Context, q automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationAgent, int64, error) {
	s.calls++
	s.kind = "agent"
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []automationport.HistoricalAutomationAgent{s.agent}, s.total, s.err
}

func automationHistoryAPIFixture() *automationHistoryAPIStub {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	identity := automationport.HistoricalAutomationIdentity{ID: 7, SourceID: 9, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}}
	return &automationHistoryAPIStub{total: 1,
		sop:    automationport.HistoricalAutomationSOP{HistoricalAutomationIdentity: identity, PoolKey: "source pool", DayIndex: -3, ContentMasked: "已遮罩 138****5678", ImagesDigest: [32]byte{3}, OriginalEnabled: true, CreatedAt: at, UpdatedAt: at},
		config: automationport.HistoricalAutomationConfig{HistoricalAutomationIdentity: identity, AgentCode: "old", DisplayName: "旧配置", ScenarioCode: "source", DraftVersion: -1, PublishedVersion: 0, PublishedAt: "source civil time", SubmittedForPublish: true, ActorsDigest: [32]byte{4}, ConfigDigest: [32]byte{5}, CreatedAt: at, UpdatedAt: at},
		prompt: automationport.HistoricalAutomationPrompt{HistoricalAutomationIdentity: identity, AgentCode: "old", DisplayName: "旧提示词", Version: -2, OriginalEnabled: true, PromptDigest: [32]byte{6}, CreatedAt: at, UpdatedAt: at},
		agent:  automationport.HistoricalAutomationAgent{HistoricalAutomationIdentity: identity, ProgramSourceID: 11, WorkflowSourceID: 0, NodeSourceID: -1, TaskSourceID: 0, AgentCode: "old", AgentName: "旧执行器", OriginalType: "source", OriginalStatus: "source", SortOrder: -4, ArchivedAt: "source civil time", ActorsDigest: [32]byte{7}, ConfigurationDigest: [32]byte{8}, CreatedAt: at, UpdatedAt: at},
	}
}
func automationHistoryAPIRouter(t *testing.T, reader automationport.AutomationHistoryReader, auth *audienceHistoryAPIAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.automationHistory = reader
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
func TestAutomationHistoryFinalRoutesTypedReadOnly(t *testing.T) {
	reader := automationHistoryAPIFixture()
	auth := &audienceHistoryAPIAuth{role: authport.RoleAdmin}
	router := automationHistoryAPIRouter(t, reader, auth)
	for _, tc := range []struct{ path, kind, want string }{
		{"sops", "sop", `"day_index":-3`},
		{"configs", "config", `"published_at":"source civil time"`},
		{"prompts", "prompt", `"version":-2`},
		{"agents", "agent", `"node_source_id":-1`},
	} {
		for _, suffix := range []string{"?limit=1&offset=0", "/7"} {
			r := httptest.NewRecorder()
			router.ServeHTTP(r, legacyRequest(http.MethodGet, "/api/admin/automation-history/"+tc.path+suffix, legacyToken(101)))
			if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s%s: %d %s", tc.path, suffix, r.Code, r.Body.String())
			}
			for _, want := range []string{tc.want, `"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"created_at":"2026-08-28T01:02:03.123456Z"`} {
				if !strings.Contains(r.Body.String(), want) {
					t.Fatalf("missing %s: %s", want, r.Body.String())
				}
			}
			for _, bad := range []string{"raw_payload", "source_identifier", "session_token", `"prompt":`, `"config":`, `"actors":`} {
				if strings.Contains(r.Body.String(), bad) {
					t.Fatalf("unexpected raw field %s", bad)
				}
			}
			if reader.kind != tc.kind {
				t.Fatalf("wrong reader %q", reader.kind)
			}
		}
	}
	if reader.calls != 8 || reader.id != 7 || reader.query.Limit != 1 || reader.query.Offset != 0 || auth.csrfCalls != 0 || len(auth.capabilities) != 8 {
		t.Fatalf("reader/auth inputs %+v %+v", reader, auth)
	}
}
func TestAutomationHistoryFinalRoutesFailClosed(t *testing.T) {
	for _, kind := range []string{"sops", "configs", "prompts", "agents"} {
		t.Run(kind, func(t *testing.T) {
			reader := automationHistoryAPIFixture()
			router := automationHistoryAPIRouter(t, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
			for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=1&limit=2", "?offset=2147483648", "?unknown=1", "?limit=%zz", "/0", "/01", "/7?limit=1"} {
				r := httptest.NewRecorder()
				router.ServeHTTP(r, legacyRequest(http.MethodGet, "/api/admin/automation-history/"+kind+suffix, legacyToken(101)))
				if r.Code != 400 {
					t.Fatalf("invalid %s status=%d", suffix, r.Code)
				}
			}
			if reader.calls != 0 {
				t.Fatal("invalid query reached reader")
			}
			for _, state := range []string{"error", "count", "identity", "time"} {
				broken := automationHistoryAPIFixture()
				switch state {
				case "error":
					broken.err = errors.New("private downstream details")
				case "count":
					broken.total = 2
				case "identity":
					broken.sop.SourceKeyDigest = [32]byte{}
					broken.config.SourceKeyDigest = [32]byte{}
					broken.prompt.SourceKeyDigest = [32]byte{}
					broken.agent.SourceKeyDigest = [32]byte{}
				case "time":
					broken.sop.CreatedAt = time.Time{}
					broken.config.CreatedAt = time.Time{}
					broken.prompt.CreatedAt = time.Time{}
					broken.agent.CreatedAt = time.Time{}
				}
				r := httptest.NewRecorder()
				automationHistoryAPIRouter(t, broken, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(r, legacyRequest(http.MethodGet, "/api/admin/automation-history/"+kind, legacyToken(101)))
				if r.Code != 503 || strings.Contains(r.Body.String(), "private downstream") {
					t.Fatalf("%s status/body=%d %s", state, r.Code, r.Body.String())
				}
			}
			r := httptest.NewRecorder()
			router.ServeHTTP(r, legacyRequest(http.MethodGet, "/api/admin/automation-history/"+kind+"/8", legacyToken(101)))
			if r.Code != 503 {
				t.Fatal("wrong returned ID accepted")
			}
			for _, missing := range []automationport.AutomationHistoryReader{nil, (*automationHistoryAPIStub)(nil)} {
				r := httptest.NewRecorder()
				automationHistoryAPIRouter(t, missing, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(r, legacyRequest(http.MethodGet, "/api/admin/automation-history/"+kind, legacyToken(101)))
				if r.Code != 503 {
					t.Fatal("missing reader accepted")
				}
			}
			reader.empty = true
			reader.total = 0
			r = httptest.NewRecorder()
			router.ServeHTTP(r, legacyRequest(http.MethodGet, "/api/admin/automation-history/"+kind, legacyToken(101)))
			if r.Code != 200 || !strings.Contains(r.Body.String(), `"items":[]`) {
				t.Fatalf("empty status=%d %s", r.Code, r.Body.String())
			}
		})
	}
}
func TestAutomationHistoryFinalRoutesAuthorizationAndNoWrites(t *testing.T) {
	for _, kind := range []string{"sops", "configs", "prompts", "agents"} {
		for _, tc := range []struct {
			role  authport.Role
			token string
			want  int
		}{
			{authport.RoleAdmin, "", 401}, {authport.Role("ops"), legacyToken(101), 403},
		} {
			reader := automationHistoryAPIFixture()
			router := automationHistoryAPIRouter(t, reader, &audienceHistoryAPIAuth{role: tc.role})
			for _, suffix := range []string{"", "/7"} {
				r := httptest.NewRecorder()
				router.ServeHTTP(r, legacyRequest(http.MethodGet, "/api/admin/automation-history/"+kind+suffix, tc.token))
				if r.Code != tc.want || reader.calls != 0 {
					t.Fatalf("auth %s status=%d calls=%d", kind, r.Code, reader.calls)
				}
			}
		}
		reader := automationHistoryAPIFixture()
		router := automationHistoryAPIRouter(t, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			r := httptest.NewRecorder()
			router.ServeHTTP(r, legacyRequest(method, "/api/admin/automation-history/"+kind, legacyToken(101)))
			if r.Code < 400 || reader.calls != 0 {
				t.Fatalf("write method accepted %s %d", method, r.Code)
			}
		}
	}
}
