package config

import (
	"context"
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

func TestLoadWeComDirectorySyncIsExplicitAndStaffScoped(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:                    "postgres://db/aicrm",
		workerPoolMaxConnsEnv:             "9",
		criticalWorkersEnv:                "2",
		eventWorkersEnv:                   "1",
		outboundWorkersEnv:                "1",
		syncWorkersEnv:                    "1",
		heavyWorkersEnv:                   "1",
		aiWorkersEnv:                      "1",
		identityHMACKeyEnv:                strings.Repeat("A", 43),
		weComOAuthCorpIDEnv:               "corp-fixture",
		weComOAuthSecretEnv:               "oauth-secret-sentinel",
		weComOAuthCallbackEnv:             "https://crm.example.test/auth/wecom/callback",
		weComDirectorySyncEnabledEnv:      "true",
		weComDirectorySyncStaffUserIDsEnv: "staff-1,staff-2",
	}
	root, err := load(appruntime.RoleWorker, mapLookup(values))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if !root.WeCom.DirectorySync.Enabled || !root.WeCom.OAuth.Enabled || !reflect.DeepEqual(root.WeCom.DirectorySync.StaffUserIDs, []string{"staff-1", "staff-2"}) {
		t.Fatalf("directory sync = %#v, oauth = %#v", root.WeCom.DirectorySync, root.WeCom.OAuth)
	}

	values[weComDirectorySyncEnabledEnv] = "false"
	delete(values, weComDirectorySyncStaffUserIDsEnv)
	root, err = load(appruntime.RoleWorker, mapLookup(values))
	if err != nil || root.WeCom.DirectorySync.Enabled {
		t.Fatalf("disabled directory sync = %#v, %v", root.WeCom.DirectorySync, err)
	}

	values[weComDirectorySyncEnabledEnv] = "true"
	values[weComDirectorySyncStaffUserIDsEnv] = "staff-1,staff-1"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.directory_sync.staff_user_ids is invalid" {
		t.Fatalf("duplicate directory sync staff error = %v", err)
	}

	delete(values, weComOAuthSecretEnv)
	values[weComDirectorySyncStaffUserIDsEnv] = "staff-1"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.oauth requires corp_id, secret, and callback_url together; wecom.directory_sync requires configured oauth credentials" {
		t.Fatalf("directory sync oauth error = %v", err)
	}
}

func TestLoadWeComCustomerAcquisitionIsIndependentExplicitAndRedacted(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:                                 "postgres://db/aicrm",
		workerPoolMaxConnsEnv:                          "9",
		criticalWorkersEnv:                             "2",
		eventWorkersEnv:                                "1",
		outboundWorkersEnv:                             "1",
		syncWorkersEnv:                                 "1",
		heavyWorkersEnv:                                "1",
		aiWorkersEnv:                                   "1",
		weComCustomerAcquisitionEnabledEnv:             "true",
		weComCustomerAcquisitionCorpIDEnv:              "ch02-corp",
		weComCustomerAcquisitionSecretEnv:              "ch02-secret-must-not-leak",
		weComCustomerAcquisitionPermissionConfirmedEnv: "true",
	}
	root, err := load(appruntime.RoleWorker, mapLookup(values))
	if err != nil || !root.WeCom.CustomerAcquisition.Enabled || !root.WeCom.CustomerAcquisition.PermissionConfirmed ||
		root.WeCom.CustomerAcquisition.CorpID != "ch02-corp" || root.WeCom.CustomerAcquisition.Secret.Value() != values[weComCustomerAcquisitionSecretEnv] {
		t.Fatalf("customer acquisition=%#v err=%v", root.WeCom.CustomerAcquisition, err)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, values[weComCustomerAcquisitionSecretEnv]) {
			t.Fatalf("Root formatting leaked CH02 credential: %q", formatted)
		}
	}
	if root.WeCom.OAuth.Enabled || root.WeCom.Sidebar.Enabled || root.WeCom.Callback.Enabled {
		t.Fatalf("CH02 unexpectedly enabled unrelated WeCom credentials: %#v", root.WeCom)
	}

	values[weComCustomerAcquisitionPermissionConfirmedEnv] = "false"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.customer_acquisition.permission_confirmed must be true when enabled" {
		t.Fatalf("permission prerequisite error=%v", err)
	}
	delete(values, weComCustomerAcquisitionSecretEnv)
	values[weComCustomerAcquisitionPermissionConfirmedEnv] = "true"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.customer_acquisition requires corp_id, secret, and permission_confirmed together" {
		t.Fatalf("partial CH02 credential error=%v", err)
	}

	values = map[string]string{
		databaseURLEnv: "postgres://db/aicrm", workerPoolMaxConnsEnv: "9", criticalWorkersEnv: "2", eventWorkersEnv: "1",
		outboundWorkersEnv: "1", syncWorkersEnv: "1", heavyWorkersEnv: "1", aiWorkersEnv: "1",
		weComCustomerAcquisitionEnabledEnv: "false",
	}
	root, err = load(appruntime.RoleWorker, mapLookup(values))
	if err != nil || root.WeCom.CustomerAcquisition.Enabled {
		t.Fatalf("disabled customer acquisition=%#v err=%v", root.WeCom.CustomerAcquisition, err)
	}
	values[weComCustomerAcquisitionCorpIDEnv] = "must-not-be-accepted"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.customer_acquisition credentials require enabled=true" {
		t.Fatalf("disabled credential error=%v", err)
	}
}

func TestLoadWeComTagCatalogRequiresIndependentReadPermission(t *testing.T) {
	values := map[string]string{
		databaseURLEnv: "postgres://db/aicrm", workerPoolMaxConnsEnv: "9", criticalWorkersEnv: "2", eventWorkersEnv: "1",
		outboundWorkersEnv: "1", syncWorkersEnv: "1", heavyWorkersEnv: "1", aiWorkersEnv: "1",
		weComTagCatalogEnabledEnv: "true", weComTagCatalogCorpIDEnv: "tag-corp",
		weComTagCatalogSecretEnv: "tag-secret-must-not-leak", weComTagCatalogPermissionConfirmedEnv: "true",
	}
	root, err := load(appruntime.RoleWorker, mapLookup(values))
	if err != nil || !root.WeCom.TagCatalog.Enabled || !root.WeCom.TagCatalog.PermissionConfirmed ||
		root.WeCom.TagCatalog.CorpID != "tag-corp" || root.WeCom.TagCatalog.Secret.Value() != values[weComTagCatalogSecretEnv] {
		t.Fatalf("tag catalog=%#v err=%v", root.WeCom.TagCatalog, err)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, values[weComTagCatalogSecretEnv]) {
			t.Fatalf("Root formatting leaked tag catalog credential: %q", formatted)
		}
	}
	if root.WeCom.CustomerAcquisition.Enabled || root.WeCom.Outbound.Enabled {
		t.Fatalf("tag catalog unexpectedly enabled unrelated WeCom credentials: %#v", root.WeCom)
	}

	values[weComTagCatalogPermissionConfirmedEnv] = "false"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.tag_catalog.permission_confirmed must be true when enabled" {
		t.Fatalf("permission prerequisite error=%v", err)
	}
	delete(values, weComTagCatalogSecretEnv)
	values[weComTagCatalogPermissionConfirmedEnv] = "true"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.tag_catalog requires corp_id, secret, and permission_confirmed together" {
		t.Fatalf("partial tag catalog credential error=%v", err)
	}
}

func TestLoadWeComOutboundIsIndependentExplicitAndRedacted(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:                      "postgres://db/aicrm",
		workerPoolMaxConnsEnv:               "9",
		criticalWorkersEnv:                  "2",
		eventWorkersEnv:                     "1",
		outboundWorkersEnv:                  "1",
		syncWorkersEnv:                      "1",
		heavyWorkersEnv:                     "1",
		aiWorkersEnv:                        "1",
		weComOutboundEnabledEnv:             "true",
		weComOutboundCorpIDEnv:              "outbound-corp",
		weComOutboundSecretEnv:              "outbound-secret-must-not-leak",
		weComOutboundPermissionConfirmedEnv: "true",
	}
	root, err := load(appruntime.RoleWorker, mapLookup(values))
	if err != nil || !root.WeCom.Outbound.Enabled || !root.WeCom.Outbound.PermissionConfirmed ||
		root.WeCom.Outbound.CorpID != "outbound-corp" || root.WeCom.Outbound.Secret.Value() != values[weComOutboundSecretEnv] {
		t.Fatalf("outbound=%#v err=%v", root.WeCom.Outbound, err)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, values[weComOutboundSecretEnv]) {
			t.Fatalf("Root formatting leaked outbound credential: %q", formatted)
		}
	}
	if root.WeCom.OAuth.Enabled || root.WeCom.Sidebar.Enabled || root.WeCom.Callback.Enabled || root.WeCom.CustomerAcquisition.Enabled {
		t.Fatalf("outbound unexpectedly enabled unrelated WeCom credentials: %#v", root.WeCom)
	}

	values[weComOutboundPermissionConfirmedEnv] = "false"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.outbound.permission_confirmed must be true when enabled" {
		t.Fatalf("permission prerequisite error=%v", err)
	}
	delete(values, weComOutboundSecretEnv)
	values[weComOutboundPermissionConfirmedEnv] = "true"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.outbound requires corp_id, secret, and permission_confirmed together" {
		t.Fatalf("partial outbound credential error=%v", err)
	}

	values = map[string]string{
		databaseURLEnv: "postgres://db/aicrm", workerPoolMaxConnsEnv: "9", criticalWorkersEnv: "2", eventWorkersEnv: "1",
		outboundWorkersEnv: "1", syncWorkersEnv: "1", heavyWorkersEnv: "1", aiWorkersEnv: "1",
		weComOutboundEnabledEnv: "false",
	}
	root, err = load(appruntime.RoleWorker, mapLookup(values))
	if err != nil || root.WeCom.Outbound.Enabled {
		t.Fatalf("disabled outbound=%#v err=%v", root.WeCom.Outbound, err)
	}
	values[weComOutboundCorpIDEnv] = "must-not-be-accepted"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wecom.outbound credentials require enabled=true" {
		t.Fatalf("disabled credential error=%v", err)
	}
}

func TestLoadAPIReceivesOnlyWeComOutboundQueueProjection(t *testing.T) {
	const secret = "worker-secret-must-not-enter-api-config"
	values := map[string]string{
		databaseURLEnv:                      "postgres://db/aicrm",
		apiListenAddressEnv:                 "127.0.0.1:8080",
		apiPoolMaxConnsEnv:                  "10",
		identityHMACKeyEnv:                  strings.Repeat("A", 43),
		weComOutboundEnabledEnv:             "true",
		weComOutboundCorpIDEnv:              "outbound-corp",
		weComOutboundSecretEnv:              secret,
		weComOutboundPermissionConfirmedEnv: "true",
	}
	root, err := load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || !root.WeCom.Outbound.Enabled || !root.WeCom.Outbound.PermissionConfirmed || root.WeCom.Outbound.CorpID != "outbound-corp" || root.WeCom.Outbound.Secret.Value() != "" {
		t.Fatalf("api outbound=%#v err=%v", root.WeCom.Outbound, err)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("API config leaked worker credential: %q", formatted)
		}
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

func TestLoadAPIClientSecretMapIsOptionalStrictAndRedacted(t *testing.T) {
	base := map[string]string{
		databaseURLEnv:      "postgres://db/aicrm",
		apiListenAddressEnv: "127.0.0.1:8080",
		apiPoolMaxConnsEnv:  "1",
		identityHMACKeyEnv:  strings.Repeat("A", 43),
	}
	root, err := load(appruntime.RoleAPI, mapLookup(base))
	if err != nil || root.APIClient.SecretMap.Configured() || root.APIClient.SecretMap.Verify("secret://adminops/api_client/identity.reader/current", strings.Repeat("A", 43)) {
		t.Fatalf("optional api-client secret map load = %#v, %v", root.APIClient, err)
	}
	const secretRef = "secret://adminops/api_client/identity.reader/current"
	secret := strings.Repeat("A", 42) + "E"
	values := cloneValues(base)
	values[apiClientSecretMapEnv] = `{"` + secretRef + `":"` + secret + `"}`
	root, err = load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || !root.APIClient.SecretMap.Configured() || !root.APIClient.SecretMap.Verify(secretRef, secret) || root.APIClient.SecretMap.Verify(secretRef, strings.Repeat("A", 43)) {
		t.Fatalf("api-client secret map load = %#v, %v", root.APIClient, err)
	}
	verified, verifyErr := root.APIClient.SecretMap.VerifyAPIClientSecret(context.Background(), secretRef, secret)
	if verifyErr != nil || !verified {
		t.Fatalf("api-client secret verifier = %t, %v", verified, verifyErr)
	}
	if formatted := fmt.Sprintf("%#v", root); strings.Contains(formatted, secret) || strings.Contains(formatted, secretRef) {
		t.Fatalf("Root formatting exposed the API-client secret map: %s", formatted)
	}
	for _, invalid := range []string{
		`{}`,
		`not-json`,
		`{"bad-ref":"` + secret + `"}`,
		`{"` + secretRef + `":"secret-map-sentinel"}`,
	} {
		values[apiClientSecretMapEnv] = invalid
		_, err = load(appruntime.RoleAPI, mapLookup(values))
		if err == nil || err.Error() == "" || strings.Contains(err.Error(), "secret-map-sentinel") {
			t.Fatalf("invalid api-client secret map error = %v", err)
		}
	}
}

func TestLoadGroupOpsWebhookSecretIsOptionalStrictAndRedacted(t *testing.T) {
	base := map[string]string{databaseURLEnv: "postgres://db/aicrm", apiListenAddressEnv: "127.0.0.1:8080", apiPoolMaxConnsEnv: "1", identityHMACKeyEnv: strings.Repeat("A", 43)}
	root, err := load(appruntime.RoleAPI, mapLookup(base))
	if err != nil || root.GroupOps.WebhookSecret.Configured() {
		t.Fatalf("optional webhook secret=%#v err=%v", root.GroupOps, err)
	}
	values := cloneValues(base)
	values[groupOpsWebhookSecretEnv] = base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	root, err = load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || !root.GroupOps.WebhookSecret.Configured() || strings.Contains(fmt.Sprintf("%#v", root), values[groupOpsWebhookSecretEnv]) {
		t.Fatalf("configured webhook secret=%#v err=%v", root.GroupOps, err)
	}
	values[groupOpsWebhookSecretEnv] = "bad-group-ops-webhook-secret"
	if _, err = load(appruntime.RoleAPI, mapLookup(values)); err == nil || !strings.Contains(err.Error(), "group_ops.webhook_secret must be 32-byte canonical base64url") {
		t.Fatalf("invalid webhook secret err=%v", err)
	}
}

func TestLoadAIAudienceWebhookSecretIsOptionalStrictAndRedacted(t *testing.T) {
	values := map[string]string{databaseURLEnv: "postgres://db/aicrm", apiListenAddressEnv: "127.0.0.1:8080", apiPoolMaxConnsEnv: "1", identityHMACKeyEnv: strings.Repeat("A", 43)}
	root, err := load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || root.AIAudience.WebhookSecret.Configured() {
		t.Fatalf("absent secret root=%#v err=%v", root, err)
	}
	values[aiAudienceWebhookSecretEnv] = base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	root, err = load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || !root.AIAudience.WebhookSecret.Configured() || strings.Contains(fmt.Sprintf("%#v", root), values[aiAudienceWebhookSecretEnv]) {
		t.Fatalf("configured secret root=%#v err=%v", root, err)
	}
	values[aiAudienceWebhookSecretEnv] = "bad-ai-audience-webhook-secret"
	if _, err = load(appruntime.RoleAPI, mapLookup(values)); !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), values[aiAudienceWebhookSecretEnv]) {
		t.Fatalf("invalid secret err=%v", err)
	}
}

func TestLoadWeChatPayProviderIsExplicitAndRedacted(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:                  "postgres://db/aicrm",
		workerPoolMaxConnsEnv:           "9",
		criticalWorkersEnv:              "2",
		eventWorkersEnv:                 "1",
		outboundWorkersEnv:              "1",
		syncWorkersEnv:                  "1",
		heavyWorkersEnv:                 "1",
		aiWorkersEnv:                    "1",
		weChatPayEnabledEnv:             "true",
		weChatPayAppIDEnv:               "wx-app-1",
		weChatPayMerchantIDEnv:          "merchant-1",
		weChatPayMerchantSerialEnv:      "merchant-serial-1",
		weChatPayMerchantPrivateKeyEnv:  "merchant-private-key-sentinel",
		weChatPayAPIV3KeyEnv:            "0123456789ABCDEF0123456789ABCDEF",
		weChatPayPlatformSerialEnv:      "platform-serial-1",
		weChatPayPlatformCertificateEnv: "platform-certificate-sentinel",
		weChatPayPaymentNotifyURLEnv:    "https://crm.example.test/api/public/wechat-pay/callbacks/payment",
		weChatPayRefundNotifyURLEnv:     "https://crm.example.test/api/public/wechat-pay/callbacks/refund",
		weChatPayPermissionConfirmedEnv: "true",
	}
	root, err := load(appruntime.RoleWorker, mapLookup(values))
	if err != nil || !root.Commerce.WeChatPay.Enabled || !root.Commerce.WeChatPay.PermissionConfirmed || root.Commerce.WeChatPay.AppID != "wx-app-1" {
		t.Fatalf("wechat pay config = %#v, %v", root.Commerce.WeChatPay, err)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, values[weChatPayMerchantPrivateKeyEnv]) || strings.Contains(formatted, values[weChatPayAPIV3KeyEnv]) || strings.Contains(formatted, values[weChatPayPlatformCertificateEnv]) {
			t.Fatalf("Root formatting leaked payment credentials: %q", formatted)
		}
	}
	values[weChatPayPermissionConfirmedEnv] = "false"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wechat_pay.permission_confirmed must be true when enabled" {
		t.Fatalf("unconfirmed payment config error = %v", err)
	}
}

func TestLoadWeChatPayAPIDoesNotReadMerchantWriteCredential(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:                  "postgres://db/aicrm",
		apiListenAddressEnv:             "127.0.0.1:8080",
		apiPoolMaxConnsEnv:              "1",
		identityHMACKeyEnv:              strings.Repeat("A", 43),
		weChatPayEnabledEnv:             "true",
		weChatPayAppIDEnv:               "wx-app-1",
		weChatPayMerchantIDEnv:          "merchant-1",
		weChatPayAPIV3KeyEnv:            "0123456789ABCDEF0123456789ABCDEF",
		weChatPayPlatformSerialEnv:      "platform-serial-1",
		weChatPayPlatformCertificateEnv: "platform-certificate-sentinel",
		weChatPayMerchantPrivateKeyEnv:  "must-not-enter-api-config",
		weChatPayPermissionConfirmedEnv: "must-not-be-read-by-api",
		weChatPayPaymentNotifyURLEnv:    "must-not-be-read-by-api",
		weChatPayRefundNotifyURLEnv:     "must-not-be-read-by-api",
	}
	root, err := load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || !root.Commerce.WeChatPay.Enabled || root.Commerce.WeChatPay.MerchantPrivateKey.Value() != "" || root.Commerce.WeChatPay.PermissionConfirmed || root.Commerce.WeChatPay.PaymentNotifyURL != "" || root.Commerce.WeChatPay.RefundNotifyURL != "" {
		t.Fatalf("API payment config = %#v, %v", root.Commerce.WeChatPay, err)
	}
	formatted := fmt.Sprintf("%#v", root)
	for _, forbidden := range []string{values[weChatPayMerchantPrivateKeyEnv], values[weChatPayPermissionConfirmedEnv], values[weChatPayPaymentNotifyURLEnv]} {
		if strings.Contains(formatted, forbidden) {
			t.Fatalf("API config read worker-only value %q", forbidden)
		}
	}
}

func TestLoadWeChatShopOrderSyncIsWorkerOnlyExplicitAndRedacted(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:                   "postgres://db/aicrm",
		workerPoolMaxConnsEnv:            "9",
		criticalWorkersEnv:               "2",
		eventWorkersEnv:                  "1",
		outboundWorkersEnv:               "1",
		syncWorkersEnv:                   "1",
		heavyWorkersEnv:                  "1",
		aiWorkersEnv:                     "1",
		weChatShopOrderSyncEnabledEnv:    "true",
		weChatShopAppIDEnv:               "wx-shop-app-1",
		weChatShopAppSecretEnv:           "shop-app-secret-sentinel",
		weChatShopPermissionConfirmedEnv: "true",
	}
	root, err := load(appruntime.RoleWorker, mapLookup(values))
	if err != nil || !root.Commerce.WeChatShopOrder.Enabled || !root.Commerce.WeChatShopOrder.PermissionConfirmed || root.Commerce.WeChatShopOrder.AppID != "wx-shop-app-1" {
		t.Fatalf("wechat shop order config = %#v, %v", root.Commerce.WeChatShopOrder, err)
	}
	for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root)} {
		if strings.Contains(formatted, values[weChatShopAppSecretEnv]) {
			t.Fatalf("Root formatting leaked WeChat Shop credential: %q", formatted)
		}
	}
	values[weChatShopPermissionConfirmedEnv] = "false"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wechat_shop.order_sync.permission_confirmed must be true when enabled" {
		t.Fatalf("unconfirmed WeChat Shop config error = %v", err)
	}
	values[weChatShopOrderSyncEnabledEnv] = "false"
	if _, err = load(appruntime.RoleWorker, mapLookup(values)); err == nil || err.Error() != "invalid startup configuration: wechat_shop.order_sync credentials require enabled=true" {
		t.Fatalf("disabled WeChat Shop credential error = %v", err)
	}
}

func TestLoadWeChatShopOrderSyncCredentialIsNotReadByAPI(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:                   "postgres://db/aicrm",
		apiListenAddressEnv:              "127.0.0.1:8080",
		apiPoolMaxConnsEnv:               "1",
		identityHMACKeyEnv:               strings.Repeat("A", 43),
		weChatShopOrderSyncEnabledEnv:    "not-a-boolean",
		weChatShopAppIDEnv:               "not valid",
		weChatShopAppSecretEnv:           "must-not-enter-api-config",
		weChatShopPermissionConfirmedEnv: "not-a-boolean",
	}
	root, err := load(appruntime.RoleAPI, mapLookup(values))
	if err != nil || root.Commerce.WeChatShopOrder.Enabled || root.Commerce.WeChatShopOrder.AppSecret.Value() != "" {
		t.Fatalf("API WeChat Shop config = %#v, %v", root.Commerce.WeChatShopOrder, err)
	}
	if strings.Contains(fmt.Sprintf("%#v", root), values[weChatShopAppSecretEnv]) {
		t.Fatal("API config read worker-only WeChat Shop secret")
	}
}

func TestLoadWeChatShopRefundProviderIsRoleScopedAndRedacted(t *testing.T) {
	workerValues := map[string]string{
		databaseURLEnv:                "postgres://db/aicrm",
		workerPoolMaxConnsEnv:         "9",
		criticalWorkersEnv:            "2",
		eventWorkersEnv:               "1",
		outboundWorkersEnv:            "1",
		syncWorkersEnv:                "1",
		heavyWorkersEnv:               "1",
		aiWorkersEnv:                  "1",
		weChatShopRefundEnabledEnv:    "true",
		weChatShopAppIDEnv:            "wx-shop-app-1",
		weChatShopAppSecretEnv:        "refund-app-secret-sentinel",
		weChatShopRefundPermissionEnv: "true",
		weChatShopCallbackTokenEnv:    "must-not-enter-worker-config",
		weChatShopCallbackAESKeyEnv:   "not-a-valid-worker-value",
	}
	worker, err := load(appruntime.RoleWorker, mapLookup(workerValues))
	if err != nil || !worker.Commerce.WeChatShopRefund.Enabled || !worker.Commerce.WeChatShopRefund.PermissionConfirmed || worker.Commerce.WeChatShopRefund.AppID != "wx-shop-app-1" || worker.Commerce.WeChatShopRefund.CallbackToken.Value() != "" || worker.Commerce.WeChatShopRefund.CallbackAESKey.Value() != "" {
		t.Fatalf("worker refund config = %#v, %v", worker.Commerce.WeChatShopRefund, err)
	}
	if formatted := fmt.Sprintf("%#v", worker); strings.Contains(formatted, workerValues[weChatShopAppSecretEnv]) || strings.Contains(formatted, workerValues[weChatShopCallbackTokenEnv]) {
		t.Fatalf("worker config leaked or read wrong-role refund credential: %q", formatted)
	}

	apiValues := map[string]string{
		databaseURLEnv:                "postgres://db/aicrm",
		apiListenAddressEnv:           "127.0.0.1:8080",
		apiPoolMaxConnsEnv:            "1",
		identityHMACKeyEnv:            strings.Repeat("A", 43),
		weChatShopRefundEnabledEnv:    "true",
		weChatShopAppIDEnv:            "wx-shop-app-1",
		weChatShopCallbackTokenEnv:    "refund-callback-token-sentinel",
		weChatShopCallbackAESKeyEnv:   strings.Repeat("A", 43),
		weChatShopAppSecretEnv:        "must-not-enter-api-config",
		weChatShopRefundPermissionEnv: "must-not-be-read-by-api",
	}
	api, err := load(appruntime.RoleAPI, mapLookup(apiValues))
	if err != nil || !api.Commerce.WeChatShopRefund.Enabled || api.Commerce.WeChatShopRefund.AppSecret.Value() != "" || api.Commerce.WeChatShopRefund.PermissionConfirmed || api.Commerce.WeChatShopRefund.CallbackToken.Value() == "" || api.Commerce.WeChatShopRefund.CallbackAESKey.Value() == "" {
		t.Fatalf("API refund config = %#v, %v", api.Commerce.WeChatShopRefund, err)
	}
	formatted := fmt.Sprintf("%#v", api)
	for _, forbidden := range []string{apiValues[weChatShopCallbackTokenEnv], apiValues[weChatShopCallbackAESKeyEnv], apiValues[weChatShopAppSecretEnv], apiValues[weChatShopRefundPermissionEnv]} {
		if strings.Contains(formatted, forbidden) {
			t.Fatalf("API config leaked or read wrong-role refund credential %q", forbidden)
		}
	}
}

func TestLoadWeChatShopRefundRequiresExplicitPermissionAndCallbackMaterial(t *testing.T) {
	workerValues := map[string]string{
		databaseURLEnv: "postgres://db/aicrm", workerPoolMaxConnsEnv: "9", criticalWorkersEnv: "2", eventWorkersEnv: "1",
		outboundWorkersEnv: "1", syncWorkersEnv: "1", heavyWorkersEnv: "1", aiWorkersEnv: "1",
		weChatShopRefundEnabledEnv: "true", weChatShopAppIDEnv: "wx-shop-app-1", weChatShopAppSecretEnv: "refund-secret", weChatShopRefundPermissionEnv: "false",
	}
	if _, err := load(appruntime.RoleWorker, mapLookup(workerValues)); err == nil || err.Error() != "invalid startup configuration: wechat_shop.refund.permission_confirmed must be true when enabled" {
		t.Fatalf("unconfirmed refund config error = %v", err)
	}
	apiValues := map[string]string{
		databaseURLEnv: "postgres://db/aicrm", apiListenAddressEnv: "127.0.0.1:8080", apiPoolMaxConnsEnv: "1", identityHMACKeyEnv: strings.Repeat("A", 43),
		weChatShopRefundEnabledEnv: "true", weChatShopAppIDEnv: "wx-shop-app-1", weChatShopCallbackTokenEnv: "token",
	}
	if _, err := load(appruntime.RoleAPI, mapLookup(apiValues)); err == nil || err.Error() != "invalid startup configuration: wechat_shop.refund callback requires token and aes_key" {
		t.Fatalf("incomplete callback config error = %v", err)
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
