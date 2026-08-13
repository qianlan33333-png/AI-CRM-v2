package config

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func TestLoadValidatesSelectedRoleConfiguration(t *testing.T) {
	base := map[string]string{
		databaseURLEnv:        "postgres://aicrm:secret@db.internal:5432/aicrm?sslmode=require",
		apiListenAddressEnv:   "127.0.0.1:8080",
		apiPoolMaxConnsEnv:    "10",
		workerPoolMaxConnsEnv: "9",
		criticalWorkersEnv:    "2",
		eventWorkersEnv:       "1",
		outboundWorkersEnv:    "1",
		syncWorkersEnv:        "1",
		heavyWorkersEnv:       "1",
		aiWorkersEnv:          "1",
	}
	sQueues := QueueConcurrency{Critical: 2, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1}
	tests := []struct {
		name string
		role appruntime.Role
		omit string
		want Root
	}{
		{name: "api", role: appruntime.RoleAPI, omit: workerPoolMaxConnsEnv, want: Root{Database: Database{URL: DatabaseURL{value: base[databaseURLEnv]}}, API: API{ListenAddress: base[apiListenAddressEnv], PoolMaxConns: 10}}},
		{name: "worker", role: appruntime.RoleWorker, omit: apiListenAddressEnv, want: Root{Database: Database{URL: DatabaseURL{value: base[databaseURLEnv]}}, Worker: Worker{PoolMaxConns: 9, Queues: sQueues}}},
		{name: "all", role: appruntime.RoleAll, want: Root{Database: Database{URL: DatabaseURL{value: base[databaseURLEnv]}}, API: API{ListenAddress: base[apiListenAddressEnv], PoolMaxConns: 10}, Worker: Worker{PoolMaxConns: 9, Queues: sQueues}}},
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
	}
	_, err := load(appruntime.RoleAll, mapLookup(values))
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("load() error = %v, want ErrInvalid", err)
	}
	want := "invalid startup configuration: database.url must be a valid postgres URL; api.listen_address must be host:port; api.pool_max_conns must be a positive integer; worker.pool_max_conns must be a positive integer"
	if err.Error() != want {
		t.Fatalf("load() error = %q, want %q", err, want)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("load() error exposed input sentinel %q", sentinel)
	}
}

func TestLoadRejectsMissingFieldsAndInvalidRole(t *testing.T) {
	_, err := load(appruntime.RoleAll, mapLookup(nil))
	want := "invalid startup configuration: database.url is required; api.listen_address is required; api.pool_max_conns is required; worker.pool_max_conns is required; worker.queues.critical is required; worker.queues.event is required; worker.queues.outbound is required; worker.queues.sync is required; worker.queues.heavy is required; worker.queues.ai is required"
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
