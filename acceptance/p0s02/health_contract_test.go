//go:build p0s02_acceptance

package p0s02_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/api/generated"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestHealthHandlerReturnsFrozenTypedResponse(t *testing.T) {
	t.Parallel()

	handler := platformhttp.NewHealthHandler()
	var _ generated.StrictServerInterface = handler

	response, err := handler.GetHealthz(context.Background(), generated.GetHealthzRequestObject{})
	if err != nil {
		t.Fatalf("GetHealthz() error = %v", err)
	}
	typed, ok := response.(generated.GetHealthz200JSONResponse)
	if !ok {
		t.Fatalf("GetHealthz() response type = %T, want generated.GetHealthz200JSONResponse", response)
	}
	if typed.Status != generated.Ok {
		t.Fatalf("GetHealthz() status = %q, want %q", typed.Status, generated.Ok)
	}
}

func TestHealthHandlerThroughGeneratedStrictRouter(t *testing.T) {
	t.Parallel()

	handler := generated.Handler(generated.NewStrictHandler(platformhttp.NewHealthHandler(), nil))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("GET /healthz Content-Type = %q, want application/json", got)
	}
	if got, want := recorder.Body.String(), "{\"status\":\"ok\"}\n"; got != want {
		t.Fatalf("GET /healthz body = %q, want %q", got, want)
	}
}
