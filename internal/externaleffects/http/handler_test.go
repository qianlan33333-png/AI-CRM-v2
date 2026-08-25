package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
)

type appStub struct {
	cancel func(context.Context, eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error)
}

func (s appStub) List(context.Context, int32) ([]eer.Projection, error) {
	return []eer.Projection{{ID: "eer_1", Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateQueued, Generation: 2, UpdatedAt: time.Now()}}, nil
}
func (s appStub) Detail(context.Context, string) (eer.Projection, error) {
	return eer.Projection{ID: "eer_1", Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateQueued, Generation: 2, UpdatedAt: time.Now()}, nil
}
func (s appStub) Diagnostics(context.Context) (eer.Diagnostics, error) {
	return eer.Diagnostics{Queued: 1}, nil
}
func (s appStub) Cancel(ctx context.Context, c eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error) {
	if s.cancel != nil {
		return s.cancel(ctx, c)
	}
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (s appStub) Retry(context.Context, eer.RetryCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (s appStub) Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}

func TestOperatorReadControlsAreNoStoreAndCancelHashesExactlyOneKey(t *testing.T) {
	var command eer.CancelCommand
	handler, err := NewHandler(appStub{cancel: func(_ context.Context, got eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error) {
		command = got
		return eer.Projection{ID: got.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateCancelled, Generation: 2, UpdatedAt: time.Now()}, eer.OperationReceipt{}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRecorder()
	handler.List(list, httptest.NewRequest(http.MethodGet, "/external-effects", nil), 0)
	if list.Code != http.StatusOK || list.Header().Get("Cache-Control") != "no-store" || !strings.Contains(list.Body.String(), `"state":"queued"`) {
		t.Fatalf("list=%d %s", list.Code, list.Body.String())
	}
	missing := httptest.NewRecorder()
	handler.Cancel(missing, httptest.NewRequest(http.MethodPost, "/external-effects/eer_1/cancel", nil), "eer_1")
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing key=%d", missing.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/external-effects/eer_1/cancel", nil)
	request.Header.Set("Idempotency-Key", "operator-key")
	response := httptest.NewRecorder()
	handler.Cancel(response, request, "eer_1")
	if response.Code != http.StatusOK || command.EffectID != "eer_1" || !strings.HasPrefix(string(command.ReceiptKeyDigest), "sha256:") {
		t.Fatalf("cancel=%d %+v", response.Code, command)
	}
}
