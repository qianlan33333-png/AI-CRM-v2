package main

import (
	"context"
	"errors"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type radarMarketingAPIStub struct {
	err           error
	empty         bool
	limit, offset int32
	v0            radarport.HistoricalRadarClick
	v1            automationport.HistoricalMarketingAutomationConfig
	v2            automationport.HistoricalMarketingAutomationRule
}

func (s *radarMarketingAPIStub) GetHistoricalRadarClick(context.Context, int64) (radarport.HistoricalRadarClick, error) {
	return s.v0, s.err
}
func (s *radarMarketingAPIStub) ListHistoricalRadarClick(_ context.Context, q radarport.RadarClickHistoryQuery) ([]radarport.HistoricalRadarClick, int64, error) {
	s.limit, s.offset = q.Limit, q.Offset
	if s.empty {
		return nil, 0, s.err
	}
	return []radarport.HistoricalRadarClick{s.v0}, 1, s.err
}
func (s *radarMarketingAPIStub) GetHistoricalMarketingAutomationConfig(context.Context, int64) (automationport.HistoricalMarketingAutomationConfig, error) {
	return s.v1, s.err
}
func (s *radarMarketingAPIStub) ListHistoricalMarketingAutomationConfig(_ context.Context, q automationport.MarketingConfigHistoryQuery) ([]automationport.HistoricalMarketingAutomationConfig, int64, error) {
	s.limit, s.offset = q.Limit, q.Offset
	if s.empty {
		return nil, 0, s.err
	}
	return []automationport.HistoricalMarketingAutomationConfig{s.v1}, 1, s.err
}
func (s *radarMarketingAPIStub) GetHistoricalMarketingAutomationRule(context.Context, int64) (automationport.HistoricalMarketingAutomationRule, error) {
	return s.v2, s.err
}
func (s *radarMarketingAPIStub) ListHistoricalMarketingAutomationRule(_ context.Context, q automationport.MarketingConfigHistoryQuery) ([]automationport.HistoricalMarketingAutomationRule, int64, error) {
	s.limit, s.offset = q.Limit, q.Offset
	if s.empty {
		return nil, 0, s.err
	}
	return []automationport.HistoricalMarketingAutomationRule{s.v2}, 1, s.err
}
func radarMarketingAPIFixture() *radarMarketingAPIStub {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	return &radarMarketingAPIStub{
		v0: radarport.HistoricalRadarClick{ID: 7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, SourceFieldDigest: [32]byte{1}, SourceID: 1, LinkSourceID: 1, RadarLinkID: nil, CustomerID: nil, Code: "observed", RawStage: "observed", SourceChannel: "observed", TargetTypeSnapshot: "observed", SourceChannelSnapshot: "observed", ErrorCode: "observed", CreatedAt: at, OpenIDDigest: [32]byte{1}, UnionIDDigest: [32]byte{1}, ExternalUserIDDigest: [32]byte{1}, CampaignIDDigest: [32]byte{1}, StaffIDDigest: [32]byte{1}, UserAgentDigest: [32]byte{1}, IPDigest: [32]byte{1}, PersonIDDigest: [32]byte{1}, IPHashDigest: [32]byte{1}, CampaignSnapshotDigest: [32]byte{1}, StaffSnapshotDigest: [32]byte{1}, RefererDigest: [32]byte{1}, QueryParamsDigest: [32]byte{1}},
		v1: automationport.HistoricalMarketingAutomationConfig{ID: 7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, SourceFieldDigest: [32]byte{1}, SourceID: 1, AutomationKey: "observed", AutomationName: "observed", TargetEvent: "observed", ChannelType: "observed", OriginalStatus: "observed", DoNotStartAfterHour: -1, CreatedAt: at, UpdatedAt: at, ConfigPayloadDigest: [32]byte{1}},
		v2: automationport.HistoricalMarketingAutomationRule{ID: 7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, SourceFieldDigest: [32]byte{1}, SourceID: 1, ConfigID: 1, ConfigSourceID: 1, QuestionnaireSourceID: nil, QuestionSourceID: nil, RuleCode: "observed", RuleName: "observed", AnswerMatchType: "observed", ScoreDelta: -1, SegmentHint: "observed", StageHint: "observed", OriginalActive: true, SortOrder: -1, CreatedAt: at, UpdatedAt: at, AnswerMatchValueDigest: [32]byte{1}, RulePayloadDigest: [32]byte{1}},
	}
}
func radarMarketingAPIRouter(t *testing.T, s *radarMarketingAPIStub, role authport.Role) http.Handler {
	t.Helper()
	auth := &audienceHistoryAPIAuth{role: role}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.radarClickHistory = s
	legacy.marketingConfigHistory = s
	ah, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), ah, ah, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
func TestRadarMarketingHistoryAPIReadOnly(t *testing.T) {
	for _, path := range []string{"/api/admin/radar-click-history", "/api/admin/marketing-config-history/configs", "/api/admin/marketing-config-history/rules"} {
		for _, suffix := range []string{"?limit=5&offset=0", "/7"} {
			s := radarMarketingAPIFixture()
			router := radarMarketingAPIRouter(t, s, authport.RoleAdmin)
			out := httptest.NewRecorder()
			router.ServeHTTP(out, legacyRequest(http.MethodGet, path+suffix, legacyToken(101)))
			if out.Code != 200 || out.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s%s code=%d body=%s", path, suffix, out.Code, out.Body.String())
			}
			for _, required := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"source_id":1`} {
				if !strings.Contains(out.Body.String(), required) {
					t.Fatalf("missing %s", required)
				}
			}
			if strings.Contains(out.Body.String(), "digest") {
				t.Fatal("private digest exposed")
			}
			if suffix[0] == '?' && (s.limit != 5 || s.offset != 0) {
				t.Fatal("pagination lost")
			}
		}
	}
}
func TestRadarMarketingHistoryAPIFailsClosed(t *testing.T) {
	for _, path := range []string{"/api/admin/radar-click-history", "/api/admin/marketing-config-history/configs", "/api/admin/marketing-config-history/rules"} {
		s := radarMarketingAPIFixture()
		s.empty = true
		router := radarMarketingAPIRouter(t, s, authport.RoleAdmin)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, legacyRequest(http.MethodGet, path, legacyToken(101)))
		if out.Code != 200 || !strings.Contains(out.Body.String(), `"items":[]`) {
			t.Fatal("empty not represented")
		}
		for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=2&limit=3", "?unknown=1", "/0", "/07", "/7?limit=1"} {
			out = httptest.NewRecorder()
			router.ServeHTTP(out, legacyRequest(http.MethodGet, path+suffix, legacyToken(101)))
			if out.Code != 400 {
				t.Fatalf("%s%s code=%d", path, suffix, out.Code)
			}
		}
		s.err = errors.New("PRIVATE-ERROR")
		out = httptest.NewRecorder()
		router.ServeHTTP(out, legacyRequest(http.MethodGet, path, legacyToken(101)))
		if out.Code != 503 || strings.Contains(out.Body.String(), "PRIVATE-ERROR") {
			t.Fatal("error not closed")
		}
		out = httptest.NewRecorder()
		radarMarketingAPIRouter(t, nil, authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodGet, path, legacyToken(101)))
		if out.Code != 503 {
			t.Fatal("typed nil not closed")
		}
		out = httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, path, nil))
		if out.Code != 401 {
			t.Fatal("anonymous admitted")
		}
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			out = httptest.NewRecorder()
			router.ServeHTTP(out, legacyRequest(method, path, legacyToken(101)))
			if out.Code >= 200 && out.Code < 300 {
				t.Fatal("write accepted")
			}
		}
	}
}
