package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	opsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/ops/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/readiness"
)

type fixedSystemHealthSource struct {
	input readiness.Input
}

type staticReadinessObserver struct {
	snapshot opsstore.ReadinessSnapshot
}

func (observer staticReadinessObserver) Observe(context.Context) (opsstore.ReadinessSnapshot, error) {
	return observer.snapshot, nil
}

type staticQueueObserver struct{ count uint64 }

func (observer staticQueueObserver) CountOutcomeUnknown(context.Context) (uint64, error) {
	return observer.count, nil
}

func TestPostgresSystemHealthSourceMapsOnlyBoundedOwnerObservations(t *testing.T) {
	source := postgresSystemHealthSource{
		platformObserver: staticReadinessObserver{snapshot: opsstore.ReadinessSnapshot{CurrentSchemaCompatible: true}},
		queueObserver:    staticQueueObserver{count: 1_000},
		production:       true, releaseSHA: strings.Repeat("a", 40), realCallsEnabled: true,
	}
	input := source.Observe(context.Background())
	if input.Database.Probe != readiness.ProbeHealthy || input.Migration.Compatibility != readiness.MigrationCompatible ||
		input.Queues.Probe != readiness.ProbeHealthy || input.Queues.UnknownAfterDispatch != 1_000 || !input.WeCom.RealCallsEnabled {
		t.Fatalf("mapped input = %+v", input)
	}
}

func (source fixedSystemHealthSource) Observe(context.Context) readiness.Input { return source.input }

func TestSystemHealthHandlerReturnsSafePublicReadiness(t *testing.T) {
	handler, err := newSystemHealthHandler(fixedSystemHealthSource{input: readiness.Input{
		Production:   true,
		Database:     readiness.DatabaseObservation{Kind: readiness.DatabasePostgres, Probe: readiness.ProbeHealthy},
		WeCom:        readiness.WeComObservation{RealCallsEnabled: false},
		Release:      readiness.ReleaseObservation{SHAComplete: true},
		RuntimeUnits: readiness.ComponentObservation{Status: readiness.ComponentWarning},
		Migration:    readiness.MigrationObservation{Compatibility: readiness.MigrationCompatible},
		Queues:       readiness.QueueObservation{Probe: readiness.ProbeHealthy, UnknownAfterDispatch: 123},
	}})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, systemHealthPath, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response = %d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	var response readiness.Response
	if err = json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(response.WarningComponents, []readiness.Component{readiness.ComponentRuntimeUnit}) {
		t.Fatalf("warning components = %v", response.WarningComponents)
	}
	if response.Components[5].UnknownAfterDispatchCount == nil || *response.Components[5].UnknownAfterDispatchCount != readiness.MaxUnknownAfterDispatchCount {
		t.Fatalf("bounded queue report = %+v", response.Components[5])
	}
	for _, forbidden := range []string{"release_sha", "database_url", "token", "payload", "external_userid"} {
		if stringsContainsFold(recorder.Body.String(), forbidden) {
			t.Fatalf("response contains forbidden %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestSystemHealthHandlerFailsClosed(t *testing.T) {
	handler, err := newSystemHealthHandler(fixedSystemHealthSource{input: readiness.Input{
		Database:     readiness.DatabaseObservation{Kind: readiness.DatabasePostgres, Probe: readiness.ProbeFailed},
		RuntimeUnits: readiness.ComponentObservation{Status: readiness.ComponentWarning},
		Migration:    readiness.MigrationObservation{Compatibility: readiness.MigrationIncompatible},
		Queues:       readiness.QueueObservation{Probe: readiness.ProbeFailed},
	}})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, systemHealthPath, nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNewSystemHealthHandlerRejectsNilSource(t *testing.T) {
	if _, err := newSystemHealthHandler(nil); !errors.Is(err, errInvalidAPIComponent) {
		t.Fatalf("error = %v", err)
	}
}

func TestFinalRouterKeepsSystemHealthPublic(t *testing.T) {
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	systemHealth, err := newSystemHealthHandler(fixedSystemHealthSource{input: readiness.Input{
		Database:     readiness.DatabaseObservation{Kind: readiness.DatabasePostgres, Probe: readiness.ProbeHealthy},
		Release:      readiness.ReleaseObservation{SHAComplete: true},
		RuntimeUnits: readiness.ComponentObservation{Status: readiness.ComponentReady},
		Migration:    readiness.MigrationObservation{Compatibility: readiness.MigrationCompatible},
		Queues:       readiness.QueueObservation{Probe: readiness.ProbeHealthy},
	}})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, &Handler{auth: service, systemHealth: systemHealth},
	)
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, systemHealthPath, nil))
	if response.Code != http.StatusOK || len(service.capabilities()) != 0 {
		t.Fatalf("GET %s status/capabilities = %d/%v, want 200/none", systemHealthPath, response.Code, service.capabilities())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, systemHealthPath, nil))
	if response.Code != http.StatusBadRequest || len(service.capabilities()) != 0 {
		t.Fatalf("POST %s status/capabilities = %d/%v, want 400/none", systemHealthPath, response.Code, service.capabilities())
	}
}

func stringsContainsFold(value, fragment string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(fragment))
}
