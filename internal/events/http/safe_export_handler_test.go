package http

import (
	"bytes"
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type safeExportApplicationStub struct {
	command eventapp.InternalEventSafeExportCreate
	result  eventapp.InternalEventSafeExport
	rows    []eventapp.InternalEventSafeExportRow
	err     error
	actor   int64
}

func (s *safeExportApplicationStub) Create(_ context.Context, c eventapp.InternalEventSafeExportCreate) (eventapp.InternalEventSafeExport, error) {
	s.command = c
	return s.result, s.err
}
func (s *safeExportApplicationStub) Get(_ context.Context, _ string, a int64) (eventapp.InternalEventSafeExport, error) {
	s.actor = a
	return s.result, s.err
}
func (s *safeExportApplicationStub) Download(_ context.Context, _ string, a int64) (eventapp.InternalEventSafeExport, []eventapp.InternalEventSafeExportRow, error) {
	s.actor = a
	return s.result, s.rows, s.err
}
func safeExportRequest(method, path string, body []byte) *stdhttp.Request {
	ctx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, "ee01")
	var err error
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	if err != nil {
		panic(err)
	}
	return httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
}
func TestSafeExportHTTPCreateAndDownloadAreClosed(t *testing.T) {
	stamp := time.Date(2026, 8, 25, 1, 2, 3, 0, time.UTC)
	stub := &safeExportApplicationStub{result: eventapp.InternalEventSafeExport{ID: "ese_0123456789abcdef0123456789abcdef", Watermark: stamp, CreatedAt: stamp}, rows: []eventapp.InternalEventSafeExportRow{{EventID: 1, EventType: "=formula", OccurredAt: stamp, Dispatched: false}}}
	h, err := NewSafeExportHandler(stub)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := safeExportRequest(stdhttp.MethodPost, SafeExportPath, []byte(`{"event_type":"customer.tag_applied"}`))
	r.Header.Set("Idempotency-Key", "internal-event-safe-export-key-01")
	h.Create(w, r)
	if w.Code != stdhttp.StatusCreated || stub.command.ActorID != 7 || stub.command.Filter.EventType != "customer.tag_applied" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create=%d command=%+v headers=%v", w.Code, stub.command, w.Header())
	}
	w = httptest.NewRecorder()
	h.Download(w, safeExportRequest(stdhttp.MethodGet, SafeExportPath+"/"+stub.result.ID+"/download", nil))
	if w.Code != stdhttp.StatusOK || w.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Contains(w.Body.Bytes(), []byte("'=formula")) {
		t.Fatalf("download=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
	}
}
func TestSafeExportHTTPRejectsWrongRoleAndMismatchedErrors(t *testing.T) {
	h, _ := NewSafeExportHandler(&safeExportApplicationStub{})
	ctx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, "ee01")
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	w := httptest.NewRecorder()
	h.Get(w, httptest.NewRequest(stdhttp.MethodGet, SafeExportPath+"/ese_0123456789abcdef0123456789abcdef", nil).WithContext(ctx))
	if w.Code != stdhttp.StatusForbidden {
		t.Fatalf("ops=%d", w.Code)
	}
	stub := &safeExportApplicationStub{err: eventapp.ErrInternalEventSafeExportConflict}
	h, _ = NewSafeExportHandler(stub)
	w = httptest.NewRecorder()
	h.Get(w, safeExportRequest(stdhttp.MethodGet, SafeExportPath+"/ese_0123456789abcdef0123456789abcdef", nil))
	if w.Code != stdhttp.StatusConflict {
		t.Fatalf("conflict=%d", w.Code)
	}
	stub.err = errors.Join(eventapp.ErrInternalEventSafeExportUnavailable, errors.New("db"))
	w = httptest.NewRecorder()
	h.Get(w, safeExportRequest(stdhttp.MethodGet, SafeExportPath+"/ese_0123456789abcdef0123456789abcdef", nil))
	if w.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("unavailable=%d", w.Code)
	}
	_ = platformhttp.CodeConflict
}

func TestSafeExportHTTPRejectsMalformedUnknownAndOversizedJSON(t *testing.T) {
	h, _ := NewSafeExportHandler(&safeExportApplicationStub{})
	for name, body := range map[string]string{
		"malformed": `{`,
		"unknown":   `{"payload":"forbidden"}`,
		"oversized": `{"event_type":"` + strings.Repeat("x", 1100) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			request := safeExportRequest(stdhttp.MethodPost, SafeExportPath, []byte(body))
			request.Header.Set("Idempotency-Key", "internal-event-safe-export-key-01")
			response := httptest.NewRecorder()
			h.Create(response, request)
			if response.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

type safeExportFailingWriter struct{}

func (safeExportFailingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestSafeExportCSVFormulaSafetyAndWriterFailure(t *testing.T) {
	for _, value := range []string{"=formula", "+formula", "-formula", "@formula", " \t\r\n=formula"} {
		if got := safeCSV(value); got != "'"+value {
			t.Fatalf("safeCSV(%q)=%q", value, got)
		}
	}
	if err := writeSafeExportCSV(safeExportFailingWriter{}, []eventapp.InternalEventSafeExportRow{{EventID: 1, EventType: "safe.event", OccurredAt: time.Now()}}); err == nil {
		t.Fatal("CSV writer failure was ignored")
	}
}
