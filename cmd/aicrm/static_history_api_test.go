package main

import (
	"context"
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
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type staticHistoryAPIStub struct {
	calls        int
	mediaQ       mediaport.StaticMediaHistoryQuery
	productQ     productport.StaticProductHistoryQuery
	cycleQ       cycleport.StaticCycleHistoryQuery
	observationQ cycleport.CycleObservationQuery
	metric       cycleport.HistoricalCycleMetric
	reference    cycleport.HistoricalCycleReference
	err          error
	empty        bool
	duplicate    bool
	badParent    bool
	group        mediaport.HistoricalGroupInvite
	page         productport.HistoricalProductPageSlice
	strategy     cycleport.HistoricalCycleStrategy
	version      cycleport.HistoricalCycleVersion
	document     cycleport.HistoricalCycleDocument
}

func (s *staticHistoryAPIStub) GetHistoricalGroupInvite(context.Context, int64) (mediaport.HistoricalGroupInvite, error) {
	s.calls++
	return s.group, s.err
}
func (s *staticHistoryAPIStub) ListHistoricalGroupInvite(_ context.Context, q mediaport.StaticMediaHistoryQuery) ([]mediaport.HistoricalGroupInvite, int64, error) {
	s.calls++
	s.mediaQ = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []mediaport.HistoricalGroupInvite{s.group, s.group}, 2, s.err
	}
	return []mediaport.HistoricalGroupInvite{s.group}, 1, s.err
}
func (s *staticHistoryAPIStub) GetHistoricalProductPageSlice(context.Context, int64) (productport.HistoricalProductPageSlice, error) {
	s.calls++
	return s.page, s.err
}
func (s *staticHistoryAPIStub) ListHistoricalProductPageSlice(_ context.Context, q productport.StaticProductHistoryQuery) ([]productport.HistoricalProductPageSlice, int64, error) {
	s.calls++
	s.productQ = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []productport.HistoricalProductPageSlice{s.page, s.page}, 2, s.err
	}
	return []productport.HistoricalProductPageSlice{s.page}, 1, s.err
}
func (s *staticHistoryAPIStub) GetHistoricalCycleStrategy(context.Context, int64) (cycleport.HistoricalCycleStrategy, error) {
	s.calls++
	return s.strategy, s.err
}
func (s *staticHistoryAPIStub) ListHistoricalCycleStrategy(_ context.Context, q cycleport.StaticCycleHistoryQuery) ([]cycleport.HistoricalCycleStrategy, int64, error) {
	s.calls++
	s.cycleQ = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []cycleport.HistoricalCycleStrategy{s.strategy, s.strategy}, 2, s.err
	}
	return []cycleport.HistoricalCycleStrategy{s.strategy}, 1, s.err
}
func (s *staticHistoryAPIStub) GetHistoricalCycleVersion(context.Context, int64) (cycleport.HistoricalCycleVersion, error) {
	s.calls++
	return s.version, s.err
}
func (s *staticHistoryAPIStub) ListHistoricalCycleVersion(_ context.Context, q cycleport.StaticCycleHistoryQuery) ([]cycleport.HistoricalCycleVersion, int64, error) {
	s.calls++
	s.cycleQ = q
	if s.empty {
		return nil, 0, s.err
	}
	value := s.version
	if s.badParent {
		value.StrategyHistoryID++
	}
	if s.duplicate {
		return []cycleport.HistoricalCycleVersion{value, value}, 2, s.err
	}
	return []cycleport.HistoricalCycleVersion{value}, 1, s.err
}
func (s *staticHistoryAPIStub) GetHistoricalCycleDocument(context.Context, int64) (cycleport.HistoricalCycleDocument, error) {
	s.calls++
	return s.document, s.err
}
func (s *staticHistoryAPIStub) ListHistoricalCycleDocument(_ context.Context, q cycleport.StaticCycleHistoryQuery) ([]cycleport.HistoricalCycleDocument, int64, error) {
	s.calls++
	s.cycleQ = q
	if s.empty {
		return nil, 0, s.err
	}
	value := s.document
	if s.badParent {
		value.VersionHistoryID++
	}
	if s.duplicate {
		return []cycleport.HistoricalCycleDocument{value, value}, 2, s.err
	}
	return []cycleport.HistoricalCycleDocument{value}, 1, s.err
}

func (s *staticHistoryAPIStub) GetHistoricalCycleMetric(context.Context, int64) (cycleport.HistoricalCycleMetric, error) {
	s.calls++
	return s.metric, s.err
}
func (s *staticHistoryAPIStub) ListHistoricalCycleMetric(_ context.Context, q cycleport.CycleObservationQuery) ([]cycleport.HistoricalCycleMetric, int64, error) {
	s.calls++
	s.observationQ = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []cycleport.HistoricalCycleMetric{s.metric, s.metric}, 2, s.err
	}
	return []cycleport.HistoricalCycleMetric{s.metric}, 1, s.err
}
func (s *staticHistoryAPIStub) GetHistoricalCycleReference(context.Context, int64) (cycleport.HistoricalCycleReference, error) {
	s.calls++
	return s.reference, s.err
}
func (s *staticHistoryAPIStub) ListHistoricalCycleReference(_ context.Context, q cycleport.CycleObservationQuery) ([]cycleport.HistoricalCycleReference, int64, error) {
	s.calls++
	s.observationQ = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []cycleport.HistoricalCycleReference{s.reference, s.reference}, 2, s.err
	}
	return []cycleport.HistoricalCycleReference{s.reference}, 1, s.err
}

func staticHistoryAPIFixture() *staticHistoryAPIStub {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	key, payload := [32]byte{1}, [32]byte{2}
	optional := at.Add(time.Second)
	return &staticHistoryAPIStub{
		metric:    cycleport.HistoricalCycleMetric{ID: 12, SourceID: -12, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: [32]byte{3}, RunSourceID: -7, LastSnapshotSourceID: -8, LimitationsJSON: []byte("null"), CreatedAt: at, UpdatedAt: at},
		reference: cycleport.HistoricalCycleReference{ID: 13, SourceID: -13, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: [32]byte{3}, RunSourceID: -7, LastSnapshotSourceID: -8, Href: "https://private.invalid/never-fetch", CreatedAt: at, UpdatedAt: at},
		group:     mediaport.HistoricalGroupInvite{ID: 7, SourceID: -1, SourceKeyDigest: key, SourcePayloadDigest: payload, Name: "", Title: "", Description: "", OriginalState: "", RoomBaseName: "", OriginalBindingState: "", CreatedAt: at, UpdatedAt: at.Add(-time.Second)},
		page:      productport.HistoricalProductPageSlice{ID: 8, SourceID: -2, SourceKeyDigest: key, SourcePayloadDigest: payload, ProductSourceID: -3, ImageSourceID: -4, SortOrder: -5, CreatedAt: at, UpdatedAt: at.Add(-time.Second)},
		strategy:  cycleport.HistoricalCycleStrategy{ID: 9, SourceID: -6, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategyKey: "", Title: "", Description: "", Cadence: "", Timezone: "", OriginalStatus: "", CurrentVersion: -7, CreatedAt: at, UpdatedAt: at.Add(-time.Second)},
		version:   cycleport.HistoricalCycleVersion{ID: 10, SourceID: -8, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategySourceID: -9, StrategyHistoryID: 9, Version: -10, Label: "", Objective: "", VersionHash: "", EffectiveFrom: &optional, OriginalGovernance: "", OperationSkillHash: "", CreatedAt: at},
		document:  cycleport.HistoricalCycleDocument{ID: 11, SourceID: -11, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategyVersionSourceID: -12, VersionHistoryID: 10, SchemaVersion: "", ExecutionGuideSHA256: "", ExecutionGuideGeneratedAt: &optional, CopyGuideSHA256: "", MeasurementGuideSHA256: "", DocumentPackHash: "", CreatedAt: at},
	}
}

func staticHistoryAPIRouter(t *testing.T, media mediaport.StaticMediaHistoryReader, product productport.StaticProductHistoryReader, cycleReader cycleport.StaticCycleHistoryReader, auth *audienceHistoryAPIAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.staticMediaHistory, legacy.staticProductHistory, legacy.staticCycleHistory = media, product, cycleReader
	legacy.cycleObservationHistory, _ = cycleReader.(cycleport.CycleObservationReader)
	a, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), a, a, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestStaticHistoryFinalRoutesAdminReadOnly(t *testing.T) {
	for _, route := range []struct {
		name, list, detail string
		parent             string
	}{
		{"group", "/api/admin/static-history/group-invites", "/api/admin/static-history/group-invites/7", ""},
		{"page", "/api/admin/static-history/page-slices", "/api/admin/static-history/page-slices/8", ""},
		{"strategy", "/api/admin/static-history/cycle-strategies", "/api/admin/static-history/cycle-strategies/9", ""},
		{"version", "/api/admin/static-history/cycle-versions", "/api/admin/static-history/cycle-versions/10", "&strategy_history_id=9"},
		{"document", "/api/admin/static-history/cycle-documents", "/api/admin/static-history/cycle-documents/11", "&version_history_id=10"},
		{"metric", "/api/admin/static-history/cycle-metrics", "/api/admin/static-history/cycle-metrics/12", ""},
		{"reference", "/api/admin/static-history/cycle-references", "/api/admin/static-history/cycle-references/13", ""},
	} {
		t.Run(route.name, func(t *testing.T) {
			reader, auth := staticHistoryAPIFixture(), &audienceHistoryAPIAuth{role: authport.RoleAdmin}
			router := staticHistoryAPIRouter(t, reader, reader, reader, auth)
			for _, path := range []string{route.list + "?limit=1&offset=0" + route.parent, route.detail} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(131)))
				if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
				}
				if route.name == "metric" || route.name == "reference" {
					for _, private := range []string{"source_key_digest", "source_payload_digest", "source_field_digest", "href", "private.invalid"} {
						if strings.Contains(response.Body.String(), private) {
							t.Fatalf("private observation field leaked: %s", private)
						}
					}
					if route.name == "metric" && !strings.Contains(response.Body.String(), `"limitations":null`) {
						t.Fatal("literal JSON null lost")
					}
				}
				for _, want := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"source_id":-`} {
					if !strings.Contains(response.Body.String(), want) {
						t.Fatalf("%s missing %s", path, want)
					}
				}
			}
			if auth.csrfCalls != 0 || len(auth.capabilities) != 2 || auth.capabilities[0] != authport.CapabilityAdminRead {
				t.Fatalf("AdminRead/CSRF=%v/%d", auth.capabilities, auth.csrfCalls)
			}
			if (route.name == "metric" || route.name == "reference") && reader.observationQ != (cycleport.CycleObservationQuery{Limit: 1, Offset: 0}) {
				t.Fatalf("observation pagination not bound: %+v", reader.observationQ)
			}
			if route.name == "version" && (reader.cycleQ.StrategyHistoryID == nil || *reader.cycleQ.StrategyHistoryID != 9 || reader.cycleQ.VersionHistoryID != nil) {
				t.Fatalf("version parent not bound: %+v", reader.cycleQ)
			}
			if route.name == "document" && (reader.cycleQ.VersionHistoryID == nil || *reader.cycleQ.VersionHistoryID != 10 || reader.cycleQ.StrategyHistoryID != nil) {
				t.Fatalf("document parent not bound: %+v", reader.cycleQ)
			}
		})
	}
	reader, auth := staticHistoryAPIFixture(), &audienceHistoryAPIAuth{role: authport.RoleAdmin}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/static-history/group-invites?limit=1", nil)
	request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(132)})
	response := httptest.NewRecorder()
	staticHistoryAPIRouter(t, reader, reader, reader, auth).ServeHTTP(response, request)
	if response.Code != http.StatusOK || auth.csrfCalls != 0 {
		t.Fatalf("current human session status=%d csrf=%d", response.Code, auth.csrfCalls)
	}
}

func TestStaticHistoryRejectsInvalidAndUnavailableResponses(t *testing.T) {
	routes := []string{"group-invites", "page-slices", "cycle-strategies", "cycle-versions", "cycle-documents", "cycle-metrics", "cycle-references"}
	for _, route := range routes {
		reader := staticHistoryAPIFixture()
		router := staticHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
		queries := []string{"limit=0", "limit=101", "offset=-1", "limit=1&limit=2", "unknown=1", "limit=%zz"}
		switch route {
		case "cycle-versions":
			queries = append(queries, "version_history_id=10", "strategy_history_id=9&strategy_history_id=10")
		case "cycle-documents":
			queries = append(queries, "strategy_history_id=9", "version_history_id=10&version_history_id=11")
		default:
			queries = append(queries, "strategy_history_id=9", "version_history_id=10")
		}
		for _, query := range queries {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+route+"?"+query, legacyToken(133)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("%s?%s status=%d", route, query, response.Code)
			}
		}
		for _, id := range []string{"0", "01", "-1", "x"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+route+"/"+id, legacyToken(134)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("%s id=%s status=%d", route, id, response.Code)
			}
		}
		if reader.calls != 0 {
			t.Fatalf("invalid request reached reader=%d", reader.calls)
		}
	}
	for _, test := range []struct {
		path    string
		prepare func(*staticHistoryAPIStub)
	}{
		{"group-invites?limit=2", func(s *staticHistoryAPIStub) { s.duplicate = true }},
		{"page-slices?limit=2", func(s *staticHistoryAPIStub) { s.duplicate = true }},
		{"cycle-strategies?limit=2", func(s *staticHistoryAPIStub) { s.duplicate = true }},
		{"cycle-metrics?limit=2", func(s *staticHistoryAPIStub) { s.duplicate = true }},
		{"cycle-references?limit=2", func(s *staticHistoryAPIStub) { s.duplicate = true }},
		{"cycle-metrics", func(s *staticHistoryAPIStub) { s.metric.LimitationsJSON = []byte("{") }},
		{"cycle-versions?limit=1&strategy_history_id=9", func(s *staticHistoryAPIStub) { s.badParent = true }},
		{"cycle-documents?limit=1&version_history_id=10", func(s *staticHistoryAPIStub) { s.badParent = true }},
	} {
		reader := staticHistoryAPIFixture()
		test.prepare(reader)
		response := httptest.NewRecorder()
		staticHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+test.path, legacyToken(135)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"group-invites", "page-slices", "cycle-strategies", "cycle-versions", "cycle-documents", "cycle-metrics", "cycle-references"} {
		reader := staticHistoryAPIFixture()
		reader.err = errors.New("private database detail")
		response := httptest.NewRecorder()
		staticHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+path, legacyToken(136)))
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "private database") {
			t.Fatalf("error %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	for _, test := range []struct {
		path    string
		corrupt func(*staticHistoryAPIStub)
		wrongID func(*staticHistoryAPIStub)
	}{
		{"group-invites", func(s *staticHistoryAPIStub) { s.group.SourceKeyDigest = [32]byte{} }, func(s *staticHistoryAPIStub) { s.group.ID++ }},
		{"page-slices", func(s *staticHistoryAPIStub) { s.page.SourcePayloadDigest = [32]byte{} }, func(s *staticHistoryAPIStub) { s.page.ID++ }},
		{"cycle-strategies", func(s *staticHistoryAPIStub) { s.strategy.SourceKeyDigest = [32]byte{} }, func(s *staticHistoryAPIStub) { s.strategy.ID++ }},
		{"cycle-versions", func(s *staticHistoryAPIStub) { s.version.SourcePayloadDigest = [32]byte{} }, func(s *staticHistoryAPIStub) { s.version.ID++ }},
		{"cycle-documents", func(s *staticHistoryAPIStub) { s.document.SourceKeyDigest = [32]byte{} }, func(s *staticHistoryAPIStub) { s.document.ID++ }},
		{"cycle-metrics", func(s *staticHistoryAPIStub) { s.metric.SourceFieldDigest = [32]byte{} }, func(s *staticHistoryAPIStub) { s.metric.ID++ }},
		{"cycle-references", func(s *staticHistoryAPIStub) { s.reference.SourcePayloadDigest = [32]byte{} }, func(s *staticHistoryAPIStub) { s.reference.ID++ }},
	} {
		reader := staticHistoryAPIFixture()
		test.corrupt(reader)
		response := httptest.NewRecorder()
		staticHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+test.path, legacyToken(137)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("digest %s status=%d", test.path, response.Code)
		}
		reader = staticHistoryAPIFixture()
		test.wrongID(reader)
		response = httptest.NewRecorder()
		staticHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+test.path+"/7", legacyToken(138)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("detail ID %s status=%d", test.path, response.Code)
		}
	}
}

func TestStaticHistoryEmptyNilAuthorizationAndNoWrites(t *testing.T) {
	paths := []string{"group-invites", "page-slices", "cycle-strategies", "cycle-versions", "cycle-documents", "cycle-metrics", "cycle-references"}
	for _, path := range paths {
		reader := staticHistoryAPIFixture()
		reader.empty = true
		response := httptest.NewRecorder()
		staticHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+path, legacyToken(137)))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
			t.Fatalf("empty %s status=%d", path, response.Code)
		}
		for _, test := range []struct {
			role  authport.Role
			token string
			want  int
		}{{authport.RoleAdmin, "", http.StatusUnauthorized}, {authport.RoleOps, legacyToken(138), http.StatusForbidden}} {
			reader := staticHistoryAPIFixture()
			response := httptest.NewRecorder()
			staticHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: test.role}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+path, test.token))
			if response.Code != test.want || reader.calls != 0 {
				t.Fatalf("auth %s status=%d calls=%d", path, response.Code, reader.calls)
			}
		}
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			reader := staticHistoryAPIFixture()
			response := httptest.NewRecorder()
			staticHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(method, "/api/admin/static-history/"+path, legacyToken(139)))
			if response.Code >= 200 && response.Code < 300 || reader.calls != 0 {
				t.Fatalf("write %s %s status=%d calls=%d", method, path, response.Code, reader.calls)
			}
		}
	}
	for _, dependencies := range []struct {
		media   mediaport.StaticMediaHistoryReader
		product productport.StaticProductHistoryReader
		cycle   cycleport.StaticCycleHistoryReader
		path    string
	}{
		{nil, staticHistoryAPIFixture(), staticHistoryAPIFixture(), "group-invites"}, {staticHistoryAPIFixture(), nil, staticHistoryAPIFixture(), "page-slices"}, {staticHistoryAPIFixture(), staticHistoryAPIFixture(), nil, "cycle-strategies"},
		{staticHistoryAPIFixture(), staticHistoryAPIFixture(), nil, "cycle-metrics"}, {staticHistoryAPIFixture(), staticHistoryAPIFixture(), nil, "cycle-references"},
	} {
		response := httptest.NewRecorder()
		staticHistoryAPIRouter(t, dependencies.media, dependencies.product, dependencies.cycle, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/"+dependencies.path, legacyToken(140)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil %s status=%d", dependencies.path, response.Code)
		}
	}
	var typedNil *staticHistoryAPIStub
	response := httptest.NewRecorder()
	staticHistoryAPIRouter(t, typedNil, typedNil, typedNil, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/static-history/group-invites", legacyToken(141)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("typed nil status=%d", response.Code)
	}
}

var _ mediaport.StaticMediaHistoryReader = (*staticHistoryAPIStub)(nil)
var _ productport.StaticProductHistoryReader = (*staticHistoryAPIStub)(nil)
var _ cycleport.StaticCycleHistoryReader = (*staticHistoryAPIStub)(nil)
