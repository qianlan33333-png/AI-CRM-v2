package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func TestLoadValidatesSelectedRoleConfiguration(t *testing.T) {
	base := map[string]string{
		databaseURLEnv:            "postgres://aicrm:secret@db.internal:5432/aicrm?sslmode=require",
		apiListenAddressEnv:       "127.0.0.1:8080",
		apiPoolMaxConnsEnv:        "10",
		workerPoolMaxConnsEnv:     "9",
		criticalWorkersEnv:        "2",
		eventWorkersEnv:           "1",
		outboundWorkersEnv:        "1",
		syncWorkersEnv:            "1",
		heavyWorkersEnv:           "1",
		aiWorkersEnv:              "1",
		identityHMACKeyEnv:        strings.Repeat("A", 43),
		domainVerificationDirEnv:  "/var/lib/aicrm/domain-verification",
		applicationEnvironmentEnv: "production",
		releaseSHAEnv:             strings.Repeat("a", 40),
	}
	var identityKey IdentityHMACKey
	sQueues := QueueConcurrency{Critical: 2, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1}
	tests := []struct {
		name string
		role appruntime.Role
		omit string
		want Root
	}{
		{name: "api", role: appruntime.RoleAPI, omit: workerPoolMaxConnsEnv, want: Root{Database: Database{URL: DatabaseURL{value: base[databaseURLEnv]}}, API: API{ListenAddress: base[apiListenAddressEnv], PoolMaxConns: 10}, Identity: Identity{HMACKey: identityKey}, DomainVerification: DomainVerification{Directory: base[domainVerificationDirEnv]}, Release: Release{Environment: base[applicationEnvironmentEnv], SHA: base[releaseSHAEnv]}, LegacyHealth: LegacyHealthSnapshot{DatabaseIsPostgres: true}}},
		{name: "worker", role: appruntime.RoleWorker, omit: apiListenAddressEnv, want: Root{Database: Database{URL: DatabaseURL{value: base[databaseURLEnv]}}, Worker: Worker{PoolMaxConns: 9, Queues: sQueues}, LegacyHealth: LegacyHealthSnapshot{DatabaseIsPostgres: true}}},
		{name: "all", role: appruntime.RoleAll, want: Root{Database: Database{URL: DatabaseURL{value: base[databaseURLEnv]}}, API: API{ListenAddress: base[apiListenAddressEnv], PoolMaxConns: 10}, Worker: Worker{PoolMaxConns: 9, Queues: sQueues}, Identity: Identity{HMACKey: identityKey}, DomainVerification: DomainVerification{Directory: base[domainVerificationDirEnv]}, Release: Release{Environment: base[applicationEnvironmentEnv], SHA: base[releaseSHAEnv]}, LegacyHealth: LegacyHealthSnapshot{DatabaseIsPostgres: true}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := cloneValues(base)
			delete(values, test.omit)
			got, err := load(test.role, mapLookup(values))
			if err != nil {
				t.Fatalf("load() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("load() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestLoadReturnsAllRelevantProblemsWithoutValues(t *testing.T) {
	const sentinel = "database-password-sentinel"
	values := map[string]string{
		databaseURLEnv:        "not-a-url-" + sentinel,
		apiListenAddressEnv:   "127.0.0.1",
		apiPoolMaxConnsEnv:    "zero",
		workerPoolMaxConnsEnv: "0",
		criticalWorkersEnv:    "2",
		eventWorkersEnv:       "1",
		outboundWorkersEnv:    "1",
		syncWorkersEnv:        "1",
		heavyWorkersEnv:       "1",
		aiWorkersEnv:          "1",
		identityHMACKeyEnv:    "bad-" + sentinel,
	}
	_, err := load(appruntime.RoleAll, mapLookup(values))
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("load() error = %v, want ErrInvalid", err)
	}
	want := "invalid startup configuration: database.url must be a valid postgres URL; api.listen_address must be host:port; api.pool_max_conns must be a positive integer; identity.hmac_key must be 32-byte canonical base64url; worker.pool_max_conns must be a positive integer"
	if err.Error() != want {
		t.Fatalf("load() error = %q, want %q", err, want)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("load() error exposed input sentinel %q", sentinel)
	}
}

func TestLoadRejectsMissingFieldsAndInvalidRole(t *testing.T) {
	_, err := load(appruntime.RoleAll, mapLookup(nil))
	want := "invalid startup configuration: database.url is required; api.listen_address is required; api.pool_max_conns is required; identity.hmac_key is required; worker.pool_max_conns is required; worker.queues.critical is required; worker.queues.event is required; worker.queues.outbound is required; worker.queues.sync is required; worker.queues.heavy is required; worker.queues.ai is required"
	if err == nil || err.Error() != want {
		t.Fatalf("missing load() error = %v, want %q", err, want)
	}
	_, err = load(appruntime.Role("invalid"), mapLookup(map[string]string{databaseURLEnv: "postgres://db/aicrm"}))
	if err == nil || err.Error() != "invalid startup configuration: process.role is invalid" {
		t.Fatalf("invalid-role load() error = %v", err)
	}
}

func TestWorkerQueueBudgetIsFailClosed(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:        "postgres://db/aicrm",
		workerPoolMaxConnsEnv: "8",
		criticalWorkersEnv:    "2",
		eventWorkersEnv:       "1",
		outboundWorkersEnv:    "1",
		syncWorkersEnv:        "1",
		heavyWorkersEnv:       "1",
		aiWorkersEnv:          "1",
	}
	_, err := load(appruntime.RoleWorker, mapLookup(values))
	want := "invalid startup configuration: worker.pool_max_conns must be at least queue concurrency total + 2"
	if err == nil || err.Error() != want {
		t.Fatalf("undersized worker pool error = %v, want %q", err, want)
	}
	values[workerPoolMaxConnsEnv] = "9"
	values[heavyWorkersEnv] = "0"
	_, err = load(appruntime.RoleWorker, mapLookup(values))
	want = "invalid startup configuration: worker.queues.heavy must be a positive integer"
	if err == nil || err.Error() != want {
		t.Fatalf("invalid heavy queue error = %v, want %q", err, want)
	}
}

func TestStartupFieldValidationBoundaries(t *testing.T) {
	for _, databaseURL := range []string{"postgres://db/aicrm", "postgresql://user:pass@[::1]:5432/aicrm?sslmode=disable", "postgres://db/aicrm?description_cache_capacity=1"} {
		if !validDatabaseURL(databaseURL) {
			t.Fatalf("validDatabaseURL(%q) = false", databaseURL)
		}
	}
	for _, databaseURL := range []string{"", "http://db/aicrm", "postgres:///aicrm", "postgres://db/", " postgres://db/aicrm", "postgres://db/aicrm#fragment", "postgres://db/aicrm?description_cache_capacity=0", "postgres://db/aicrm?description_cache_capacity=-1", "postgres://db/aicrm?description_cache_capacity=1&description_cache_capacity=2"} {
		if validDatabaseURL(databaseURL) {
			t.Fatalf("validDatabaseURL(%q) = true", databaseURL)
		}
	}
	for _, address := range []string{"127.0.0.1:8080", ":8080", "[::1]:65535"} {
		if !validListenAddress(address) {
			t.Fatalf("validListenAddress(%q) = false", address)
		}
	}
	for _, address := range []string{"localhost", "127.0.0.1:0", "127.0.0.1:65536", " 127.0.0.1:8080"} {
		if validListenAddress(address) {
			t.Fatalf("validListenAddress(%q) = true", address)
		}
	}
}

func TestLoadWeComCallbackIsAtomicAndRedacted(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:         "postgres://db/aicrm",
		apiListenAddressEnv:    "127.0.0.1:8080",
		apiPoolMaxConnsEnv:     "1",
		weComCallbackCorpIDEnv: "wx5823bf96d3bd56c7",
		weComCallbackTokenEnv:  "callback-token-sentinel",
		weComCallbackAESKeyEnv: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		identityHMACKeyEnv:     strings.Repeat("A", 43),
	}
	root, err := load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || !root.WeCom.Callback.Enabled || root.WeCom.Callback.CorpID != values[weComCallbackCorpIDEnv] {
		t.Fatalf("callback load = %#v, %v", root.WeCom.Callback, err)
	}
	if root.WeCom.Callback.Token.Value() != values[weComCallbackTokenEnv] || root.WeCom.Callback.EncodingAESKey.Value() != values[weComCallbackAESKeyEnv] {
		t.Fatal("callback credentials were not preserved")
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, values[weComCallbackTokenEnv]) || strings.Contains(formatted, values[weComCallbackAESKeyEnv]) {
			t.Fatalf("Root formatting leaked callback credential: %q", formatted)
		}
	}
	delete(values, weComCallbackTokenEnv)
	_, err = load(appruntime.RoleAPI, mapLookup(values))
	if err == nil || err.Error() != "invalid startup configuration: wecom.callback requires corp_id, token, and aes_key together" {
		t.Fatalf("partial callback configuration error = %v", err)
	}
	values[weComCallbackTokenEnv] = " callback-token-sentinel"
	_, err = load(appruntime.RoleAPI, mapLookup(values))
	if err == nil || err.Error() != "invalid startup configuration: wecom.callback.token is invalid" || strings.Contains(err.Error(), values[weComCallbackTokenEnv]) {
		t.Fatalf("invalid callback token error = %v", err)
	}
}

func TestLoadWeComOAuthIsAtomicAndRedacted(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:        "postgres://db/aicrm",
		apiListenAddressEnv:   "127.0.0.1:8080",
		apiPoolMaxConnsEnv:    "1",
		weComOAuthCorpIDEnv:   "corp-fixture",
		weComOAuthSecretEnv:   "oauth-secret-sentinel",
		weComOAuthCallbackEnv: "https://crm.example.test/auth/wecom/callback",
		identityHMACKeyEnv:    strings.Repeat("A", 43),
	}
	root, err := load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || !root.WeCom.OAuth.Enabled || root.WeCom.OAuth.CorpID != "corp-fixture" || root.WeCom.OAuth.CallbackURL != values[weComOAuthCallbackEnv] ||
		root.WeCom.OAuth.Secret.Value() != values[weComOAuthSecretEnv] {
		t.Fatalf("oauth load = %#v, %v", root.WeCom.OAuth, err)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, values[weComOAuthSecretEnv]) {
			t.Fatalf("Root formatting leaked OAuth credential: %q", formatted)
		}
	}
	delete(values, weComOAuthSecretEnv)
	if _, err = load(appruntime.RoleAPI, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.oauth requires corp_id, secret, and callback_url together" {
		t.Fatalf("partial OAuth error = %v", err)
	}
	values[weComOAuthSecretEnv] = "oauth-secret-sentinel"
	values[weComOAuthCallbackEnv] = "https://evil.example/callback"
	if _, err = load(appruntime.RoleAPI, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.oauth.callback_url is invalid" {
		t.Fatalf("unsafe OAuth callback error = %v", err)
	}
}

func TestLoadWeComSidebarIsAtomicAndUsesIndependentCallback(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:          "postgres://db/aicrm",
		apiListenAddressEnv:     "127.0.0.1:8080",
		apiPoolMaxConnsEnv:      "1",
		identityHMACKeyEnv:      strings.Repeat("A", 43),
		weComSidebarCorpIDEnv:   "sidebar-corp",
		weComSidebarSecretEnv:   "sidebar-secret-sentinel",
		weComSidebarCallbackEnv: "https://crm.example.test/api/sidebar/v2/oauth/callback",
		weComSidebarAgentIDEnv:  "73",
		weComSidebarHostsEnv:    "crm.example.test,sidebar.example.test",
	}
	root, err := load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || !root.WeCom.Sidebar.Enabled || root.WeCom.Sidebar.CorpID != "sidebar-corp" || root.WeCom.Sidebar.AgentID != 73 ||
		!reflect.DeepEqual(root.WeCom.Sidebar.AllowedHosts, []string{"crm.example.test", "sidebar.example.test"}) || root.WeCom.Sidebar.Secret.Value() != values[weComSidebarSecretEnv] {
		t.Fatalf("sidebar load = %#v, %v", root.WeCom.Sidebar, err)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, values[weComSidebarSecretEnv]) {
			t.Fatalf("Root formatting leaked Sidebar credential: %q", formatted)
		}
	}
	delete(values, weComSidebarAgentIDEnv)
	if _, err = load(appruntime.RoleAPI, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.sidebar requires corp_id, secret, callback_url, agent_id, and allowed_hosts together" {
		t.Fatalf("partial Sidebar error = %v", err)
	}
	values[weComSidebarAgentIDEnv] = "73"
	values[weComSidebarCallbackEnv] = "https://crm.example.test/auth/wecom/callback"
	if _, err = load(appruntime.RoleAPI, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.sidebar.callback_url is invalid" {
		t.Fatalf("shared Callback error = %v", err)
	}
}

func TestDatabaseURLFormattingIsRedacted(t *testing.T) {
	value := DatabaseURL{value: "postgres://user:secret@db/aicrm"}
	root := Root{Database: Database{URL: value}}
	for _, formatted := range []string{fmt.Sprint(value), fmt.Sprintf("%#v", value)} {
		if formatted != "[REDACTED]" {
			t.Fatalf("formatted DatabaseURL = %q, want redaction", formatted)
		}
	}
	for _, formatted := range []string{fmt.Sprintf("%+v", root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, "secret") {
			t.Fatalf("formatted Root = %q, want redaction", formatted)
		}
	}
	if value.Value() != "postgres://user:secret@db/aicrm" {
		t.Fatal("DatabaseURL.Value() did not preserve the validated value")
	}
}

func TestLoadFoldsLegacyHealthSnapshotWithoutRetainingSecrets(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:                            "postgres://db/aicrm",
		apiListenAddressEnv:                       "127.0.0.1:8080",
		apiPoolMaxConnsEnv:                        "1",
		identityHMACKeyEnv:                        strings.Repeat("A", 43),
		legacySecretKeyEnv:                        "legacy-secret-sentinel",
		legacyWeChatShopCallbackTokenEnv:          "legacy-callback-token-sentinel",
		legacyProductionEnvironmentEnvs[0]:        " production ",
		legacyAllowMissingWeChatShopCallbackToken: " YeS ",
	}
	root, err := load(appruntime.RoleAPI, mapLookup(values))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	want := LegacyHealthSnapshot{
		DatabaseIsPostgres:                  true,
		ProductionEnvironment:               true,
		SecretKeyPresent:                    true,
		WeChatShopCallbackTokenPresent:      true,
		AllowMissingWeChatShopCallbackToken: true,
	}
	if root.LegacyHealth != want {
		t.Fatalf("legacy health snapshot = %#v, want %#v", root.LegacyHealth, want)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, "sentinel") {
			t.Fatalf("Root formatting retained legacy secret material: %q", formatted)
		}
	}
}

func TestLoadLegacyHealthEnvironmentBoundaries(t *testing.T) {
	base := map[string]string{
		databaseURLEnv:      "postgres://db/aicrm",
		apiListenAddressEnv: "127.0.0.1:8080",
		apiPoolMaxConnsEnv:  "1",
		identityHMACKeyEnv:  strings.Repeat("A", 43),
	}
	loadSnapshot := func(t *testing.T, extra map[string]string) LegacyHealthSnapshot {
		t.Helper()
		values := cloneValues(base)
		for key, value := range extra {
			values[key] = value
		}
		root, err := load(appruntime.RoleAPI, mapLookup(values))
		if err != nil {
			t.Fatalf("load() error = %v", err)
		}
		return root.LegacyHealth
	}

	if snapshot := loadSnapshot(t, nil); snapshot != (LegacyHealthSnapshot{DatabaseIsPostgres: true}) {
		t.Fatalf("absent legacy names must stay absent: %#v", snapshot)
	}
	// The v2 environment name must never substitute for the legacy aliases.
	if snapshot := loadSnapshot(t, map[string]string{applicationEnvironmentEnv: "production"}); snapshot.ProductionEnvironment {
		t.Fatalf("AICRM_ENV must not feed the legacy production snapshot: %#v", snapshot)
	}
	for _, alias := range legacyProductionEnvironmentEnvs {
		for _, value := range []string{"prod", " Production ", "PROD"} {
			if snapshot := loadSnapshot(t, map[string]string{alias: value}); !snapshot.ProductionEnvironment {
				t.Fatalf("%s=%q must mark the legacy production environment", alias, value)
			}
		}
		for _, value := range []string{"", "staging", "prod-local"} {
			if snapshot := loadSnapshot(t, map[string]string{alias: value}); snapshot.ProductionEnvironment {
				t.Fatalf("%s=%q must not mark the legacy production environment", alias, value)
			}
		}
	}
	for _, value := range []string{"1", "true", " TRUE ", "yes", "on"} {
		if snapshot := loadSnapshot(t, map[string]string{legacyAllowMissingWeChatShopCallbackToken: value}); !snapshot.AllowMissingWeChatShopCallbackToken {
			t.Fatalf("allow-missing %q must fold to true", value)
		}
	}
	for _, value := range []string{"0", "false", "enabled", ""} {
		if snapshot := loadSnapshot(t, map[string]string{legacyAllowMissingWeChatShopCallbackToken: value}); snapshot.AllowMissingWeChatShopCallbackToken {
			t.Fatalf("allow-missing %q must fold to false", value)
		}
	}
	// Worker-only startup never reads the legacy presence names.
	workerValues := map[string]string{
		databaseURLEnv:                     "postgres://db/aicrm",
		workerPoolMaxConnsEnv:              "9",
		criticalWorkersEnv:                 "2",
		eventWorkersEnv:                    "1",
		outboundWorkersEnv:                 "1",
		syncWorkersEnv:                     "1",
		heavyWorkersEnv:                    "1",
		aiWorkersEnv:                       "1",
		legacySecretKeyEnv:                 "legacy-secret-sentinel",
		legacyProductionEnvironmentEnvs[1]: "production",
	}
	worker, err := load(appruntime.RoleWorker, mapLookup(workerValues))
	if err != nil {
		t.Fatalf("worker load() error = %v", err)
	}
	if worker.LegacyHealth != (LegacyHealthSnapshot{DatabaseIsPostgres: true}) {
		t.Fatalf("worker legacy health snapshot = %#v", worker.LegacyHealth)
	}
}

func TestLoadSurveyPublicKeyIsOptionalStrictAndRedacted(t *testing.T) {
	base := map[string]string{
		databaseURLEnv:      "postgres://db/aicrm",
		apiListenAddressEnv: "127.0.0.1:8080",
		apiPoolMaxConnsEnv:  "1",
		identityHMACKeyEnv:  strings.Repeat("A", 43),
	}
	root, err := load(appruntime.RoleAPI, mapLookup(base))
	if err != nil || string(root.Survey.PublicKey.Value()) != string(make([]byte, 32)) {
		t.Fatalf("optional survey key load = %#v, %v", root.Survey, err)
	}
	values := cloneValues(base)
	values[surveyPublicKeyEnv] = strings.Repeat("A", 43)
	root, err = load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || len(root.Survey.PublicKey.Value()) != 32 {
		t.Fatalf("configured survey key load = %#v, %v", root.Survey, err)
	}
	if strings.Contains(fmt.Sprintf("%#v", root), values[surveyPublicKeyEnv]) {
		t.Fatal("Root formatting exposed the Survey public key")
	}
	values[surveyPublicKeyEnv] = "bad-survey-key-sentinel"
	_, err = load(appruntime.RoleAPI, mapLookup(values))
	if err == nil || err.Error() != "invalid startup configuration: survey.public_key must be 32-byte canonical base64url" || strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("invalid survey key error = %v", err)
	}
}

func TestLoadAPIClientJWTSecretIsOptionalStrictAndRedacted(t *testing.T) {
	base := map[string]string{
		databaseURLEnv:      "postgres://db/aicrm",
		apiListenAddressEnv: "127.0.0.1:8080",
		apiPoolMaxConnsEnv:  "1",
		identityHMACKeyEnv:  strings.Repeat("A", 43),
	}
	root, err := load(appruntime.RoleAPI, mapLookup(base))
	if err != nil || string(root.APIClient.JWTSecret.Value()) != string(make([]byte, 32)) {
		t.Fatalf("optional api-client JWT secret load = %#v, %v", root.APIClient, err)
	}
	values := cloneValues(base)
	values[apiClientJWTSecretEnv] = base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	root, err = load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || len(root.APIClient.JWTSecret.Value()) != 32 {
		t.Fatalf("configured api-client JWT secret load = %#v, %v", root.APIClient, err)
	}
	if strings.Contains(fmt.Sprintf("%#v", root), values[apiClientJWTSecretEnv]) {
		t.Fatal("Root formatting exposed the API-client JWT secret")
	}
	values[apiClientJWTSecretEnv] = "bad-api-client-key-sentinel"
	_, err = load(appruntime.RoleAPI, mapLookup(values))
	if err == nil || err.Error() != "invalid startup configuration: api_client.jwt_secret must be 32-byte canonical base64url" || strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("invalid api-client JWT secret error = %v", err)
	}
}

func mapLookup(values map[string]string) environmentLookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func cloneValues(values map[string]string) map[string]string {
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
