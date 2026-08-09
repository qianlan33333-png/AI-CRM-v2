package platformhttp

import (
	"context"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/api/generated"
)

func TestHealthHandlerReturnsFrozenResponse(t *testing.T) {
	t.Parallel()

	handler := NewHealthHandler()
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
