package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type consoleApplicationStub struct {
	resolveResult identityport.ResolveResult
	bindResult    identityport.BindResult
	err           error
	resolveRef    identityport.IDRef
	bindCommand   identityport.BindCommand
}

func (stub *consoleApplicationStub) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	stub.resolveRef = ref
	return stub.resolveResult, stub.err
}

func (stub *consoleApplicationStub) Bind(_ context.Context, command identityport.BindCommand) (identityport.BindResult, error) {
	stub.bindCommand = command
	return stub.bindResult, stub.err
}

func TestConsoleHandlerResolvesDeclaredIdentityWithoutEchoingRawValue(t *testing.T) {
	stub := &consoleApplicationStub{resolveResult: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 42}}
	handler, err := NewConsoleHandler(stub)
	if err != nil {
		t.Fatal(err)
	}
	request := reviewRequest(t, http.MethodPost, `{"ref":{"type":"phone","scope":"phone:e164","value":"+8613800138000"}}`, authport.CapabilityIdentityResolve)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ResolveIdentity(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "+8613800138000") || stub.resolveRef.Assurance != identityport.AssuranceDeclared || stub.resolveRef.Source != "admin" {
		t.Fatalf("status=%d body=%q ref=%+v", response.Code, response.Body.String(), stub.resolveRef)
	}
	var body struct {
		Status     string `json:"status"`
		CustomerID int64  `json:"customer_id"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Status != "found" || body.CustomerID != 42 {
		t.Fatalf("body=%+v err=%v", body, err)
	}
}

func TestConsoleHandlerBindsOneDeclaredIdentityWithReceiptKey(t *testing.T) {
	stub := &consoleApplicationStub{bindResult: identityport.BindResult{Status: identityport.BindManualReview, ReviewID: 31}}
	handler, err := NewConsoleHandler(stub)
	if err != nil {
		t.Fatal(err)
	}
	request := reviewRequest(t, http.MethodPost, `{"customer_id":42,"ref":{"type":"phone","scope":"phone:e164","value":"+8613800138000"}}`, authport.CapabilityIdentityBind)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.BindIdentity(response, request, generated.BindIdentityParams{IdempotencyKey: generated.IdempotencyKey("identity-console-bind-0001")})
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "+8613800138000") || stub.bindCommand.CustomerID != contactport.CustomerID(42) || stub.bindCommand.Actor != contactport.Actor("admin:7") || stub.bindCommand.Ref.Assurance != identityport.AssuranceDeclared || stub.bindCommand.IdempotencyKey != "identity-console-bind-0001" {
		t.Fatalf("status=%d body=%q command=%+v", response.Code, response.Body.String(), stub.bindCommand)
	}
	if !strings.Contains(response.Body.String(), `"status":"manual_review"`) || !strings.Contains(response.Body.String(), `"review_id":31`) {
		t.Fatalf("response=%q", response.Body.String())
	}
}

func TestConsoleHandlerFailsClosedForMalformedOrUnauthorizedCommands(t *testing.T) {
	stub := &consoleApplicationStub{}
	handler, err := NewConsoleHandler(stub)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		capability authport.Capability
		body       string
		key        string
		status     int
	}{
		{name: "unknown field", capability: authport.CapabilityIdentityResolve, body: `{"ref":{"type":"phone","scope":"phone:e164","value":"+861","extra":true}}`, status: http.StatusBadRequest},
		{name: "unknown kind", capability: authport.CapabilityIdentityResolve, body: `{"ref":{"type":"unknown","scope":"phone:e164","value":"+861"}}`, status: http.StatusUnprocessableEntity},
		{name: "short key", capability: authport.CapabilityIdentityBind, body: `{"customer_id":42,"ref":{"type":"phone","scope":"phone:e164","value":"+861"}}`, key: "short", status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := reviewRequest(t, http.MethodPost, test.body, test.capability)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			if test.capability == authport.CapabilityIdentityResolve {
				handler.ResolveIdentity(response, request)
			} else {
				handler.BindIdentity(response, request, generated.BindIdentityParams{IdempotencyKey: generated.IdempotencyKey(test.key)})
			}
			if response.Code != test.status {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
	stub.err = identityapp.ErrIdentityBindIdempotencyConflict
	request := reviewRequest(t, http.MethodPost, `{"customer_id":42,"ref":{"type":"phone","scope":"phone:e164","value":"+861"}}`, authport.CapabilityIdentityBind)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.BindIdentity(response, request, generated.BindIdentityParams{IdempotencyKey: generated.IdempotencyKey("identity-console-bind-0002")})
	if response.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d", response.Code)
	}
}

func TestConsoleHandlerRejectsTypedNilApplication(t *testing.T) {
	var stub *consoleApplicationStub
	if handler, err := NewConsoleHandler(stub); handler != nil || !errors.Is(err, identityapp.ErrIdentityResolveFailed) {
		t.Fatalf("handler=%v err=%v", handler, err)
	}
}
