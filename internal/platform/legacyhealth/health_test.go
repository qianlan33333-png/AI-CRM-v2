package legacyhealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryMatchesLegacyNormalPostgresSnapshot(t *testing.T) {
	t.Parallel()

	payload := NewQuery(RuntimeSnapshot{
		DatabaseIsPostgres: true, SecretKeyPresent: true, WeChatShopCallbackTokenPresent: true,
	}).Execute()
	if payload != (Payload{
		OK: true, Status: "ok", Service: "aicrm-next", SecretKeyPresent: true,
		WeChatShopCallbackTokenPresent: true, WeChatShopCallbackTokenRequired: false,
		Database: "postgres", DatabaseMode: "postgres", FixtureMode: false,
		ProductionDataReady: true, ProductionDataMode: true, RepositoryPolicy: "production_repositories_required",
		RuntimeOwner: "ai_crm_next", LegacyRuntimeEnabled: false, Warning: "",
	}) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestQueryMatchesLegacyFixtureAndProductionBoundaries(t *testing.T) {
	t.Parallel()

	fixture := NewQuery(RuntimeSnapshot{}).Execute()
	if fixture != (Payload{
		OK: true, Status: "ok", Service: "aicrm-next", SecretKeyPresent: false,
		WeChatShopCallbackTokenPresent: false, WeChatShopCallbackTokenRequired: false,
		Database: "fixture", DatabaseMode: "fixture", FixtureMode: true,
		ProductionDataReady: false, ProductionDataMode: false, RepositoryPolicy: "fixture_repositories_allowed",
		RuntimeOwner: "ai_crm_next", LegacyRuntimeEnabled: false, Warning: "fixture data mode",
	}) {
		t.Fatalf("fixture payload = %#v", fixture)
	}
	degraded := NewQuery(RuntimeSnapshot{ProductionEnvironment: true}).Execute()
	if degraded != (Payload{
		OK: false, Status: "degraded", Service: "aicrm-next", SecretKeyPresent: false,
		WeChatShopCallbackTokenPresent: false, WeChatShopCallbackTokenRequired: true,
		Database: "fixture", DatabaseMode: "fixture", FixtureMode: true,
		ProductionDataReady: false, ProductionDataMode: false, RepositoryPolicy: "production_repositories_required",
		RuntimeOwner: "ai_crm_next", LegacyRuntimeEnabled: false,
		Warning: "production runtime is using fixture data; production data is not ready",
	}) {
		t.Fatalf("degraded payload = %#v", degraded)
	}
	allowedMissing := NewQuery(RuntimeSnapshot{DatabaseIsPostgres: true, ProductionEnvironment: true, AllowMissingWeChatShopCallbackToken: true}).Execute()
	if allowedMissing.WeChatShopCallbackTokenRequired || !allowedMissing.OK || allowedMissing.Status != "ok" {
		t.Fatalf("allowed-missing payload = %#v", allowedMissing)
	}
}

func TestQueryRepresentsMissingConfigurationWithoutLeakingSecrets(t *testing.T) {
	t.Parallel()

	payload := NewQuery(RuntimeSnapshot{DatabaseIsPostgres: true}).Execute()
	if payload.SecretKeyPresent || payload.WeChatShopCallbackTokenPresent || payload.WeChatShopCallbackTokenRequired {
		t.Fatalf("missing configuration payload = %#v", payload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret_key\":\"", "callback_token\":\"", "AICRM_", "password"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("health payload leaked sensitive material marker %q: %s", forbidden, encoded)
		}
	}
}

func TestHandlerPreservesLegacyGETHTTPShapeAndDoesNotAddCachePolicy(t *testing.T) {
	t.Parallel()

	handler := NewHandler(NewQuery(RuntimeSnapshot{DatabaseIsPostgres: true, SecretKeyPresent: true}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != contentTypeJSON || response.Header().Get("Cache-Control") != "" {
		t.Fatalf("status/content-type/cache = %d/%q/%q", response.Code, response.Header().Get("Content-Type"), response.Header().Get("Cache-Control"))
	}
	var payload Payload
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Database != "postgres" || !payload.SecretKeyPresent {
		t.Fatalf("payload = %#v, err = %v", payload, err)
	}
}

func TestHandlerRejectsNonGETLikeLegacyRouter(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(NewQuery(RuntimeSnapshot{})).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/health", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet ||
		response.Header().Get("Content-Type") != contentTypeJSON || response.Body.String() != `{"detail":"Method Not Allowed"}` {
		t.Fatalf("method error = status %d allow %q type %q body %q", response.Code, response.Header().Get("Allow"), response.Header().Get("Content-Type"), response.Body.String())
	}
}
