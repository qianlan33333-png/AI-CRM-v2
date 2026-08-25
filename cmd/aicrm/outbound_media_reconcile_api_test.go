package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

const outboundMediaReconcileBody = `{"generation":3,"fence":9,"lease_expires_at":"2026-08-25T10:00:00Z","evidence_ref":"provider_receipt_7","provider_accepted":true,"delivery_proven":true}`

type outboundMediaReconcileStub struct {
	command mediaapp.OutboundMediaReconcileCommand
	err     error
	calls   int
}

func (s *outboundMediaReconcileStub) Reconcile(_ context.Context, command mediaapp.OutboundMediaReconcileCommand) (mediaapp.OutboundMediaReconcileResult, error) {
	s.calls++
	s.command = command
	if s.err != nil {
		return mediaapp.OutboundMediaReconcileResult{}, s.err
	}
	return mediaapp.OutboundMediaReconcileResult{EffectID: "eer_7", State: "reconciled", ProviderAccepted: true, DeliveryProven: true, Replay: s.calls > 1}, nil
}

func TestReconcileOutboundMediaHashesEvidenceAndReturnsPIIMinimalReceipt(t *testing.T) {
	stub := &outboundMediaReconcileStub{}
	request := outboundMediaReconcileHTTPRequest(true, true, "outbound-media-reconcile-key-0001")
	request.SetPathValue("content_package_id", "42")
	request.SetPathValue("target_ref", "external_contact_7")
	response := httptest.NewRecorder()
	(&Handler{outboundMediaReconcile: stub}).ReconcileOutboundMedia(response, request)
	if response.Code != http.StatusOK || stub.calls != 1 || stub.command.ContentPackageID != 42 || stub.command.TargetRef != "external_contact_7" || stub.command.Generation != 3 || stub.command.Fence != 9 || !stub.command.LeaseExpiresAt.Equal(time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)) || stub.command.EvidenceDigest != outboundMediaEvidenceDigest("provider_receipt_7") || stub.command.EvidenceDigest == "provider_receipt_7" || stub.command.IdempotencyKey != "outbound-media-reconcile-key-0001" {
		t.Fatalf("status=%d stub=%#v body=%s", response.Code, stub, response.Body.String())
	}
	body := decodeOutboundMediaReconcileBody(t, response.Body.Bytes())
	if len(body) != 5 || string(body["effect_id"]) != `"eer_7"` || string(body["state"]) != `"reconciled"` || string(body["provider_accepted"]) != "true" || string(body["delivery_proven"]) != "true" || string(body["replay"]) != "false" {
		t.Fatalf("body=%s", response.Body.String())
	}
	assertOutboundMediaReconcilePIIMinimal(t, body, response.Body.String())
}

func TestReconcileOutboundMediaRejectsBadInputAndMapsConflict(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		key  string
		want int
		err  error
	}{
		{name: "missing idempotency key", body: outboundMediaReconcileBody, want: http.StatusBadRequest},
		{name: "malformed body", body: `{`, key: "outbound-media-reconcile-key-0002", want: http.StatusBadRequest},
		{name: "invalid evidence", body: strings.Replace(outboundMediaReconcileBody, "provider_receipt_7", "  ", 1), key: "outbound-media-reconcile-key-0002", want: http.StatusBadRequest},
		{name: "conflict", body: outboundMediaReconcileBody, key: "outbound-media-reconcile-key-0002", want: http.StatusConflict, err: mediaapp.ErrOutboundMediaReconcileConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &outboundMediaReconcileStub{err: test.err}
			request := outboundMediaReconcileHTTPRequest(true, true, test.key)
			request.Body = io.NopCloser(strings.NewReader(test.body))
			request.SetPathValue("content_package_id", "42")
			request.SetPathValue("target_ref", "external_contact_7")
			response := httptest.NewRecorder()
			(&Handler{outboundMediaReconcile: stub}).ReconcileOutboundMedia(response, request)
			if response.Code != test.want || (test.err == nil && stub.calls != 0) || (test.err != nil && stub.calls != 1) || strings.Contains(response.Body.String(), "provider_receipt_7") {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
			}
		})
	}
}

func TestOutboundMediaReconcileRouteRequiresMediaWriteCSRFAndIdempotency(t *testing.T) {
	stub := &outboundMediaReconcileStub{}
	auth := &legacyMediaAuthStub{}
	router := outboundMediaReconcileRouter(t, auth, stub)

	success := httptest.NewRecorder()
	router.ServeHTTP(success, outboundMediaReconcileHTTPRequest(true, true, "outbound-media-reconcile-key-0003"))
	if success.Code != http.StatusOK || stub.calls != 1 || auth.authenticateCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryWrite || auth.csrfCalls != 1 {
		t.Fatalf("success status=%d stub=%#v auth=%#v body=%s", success.Code, stub, auth, success.Body.String())
	}
	assertOutboundMediaReconcilePIIMinimal(t, decodeOutboundMediaReconcileBody(t, success.Body.Bytes()), success.Body.String())

	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, outboundMediaReconcileHTTPRequest(true, true, "outbound-media-reconcile-key-0003"))
	replayBody := decodeOutboundMediaReconcileBody(t, replay.Body.Bytes())
	if replay.Code != http.StatusOK || stub.calls != 2 || string(replayBody["replay"]) != "true" {
		t.Fatalf("replay status=%d calls=%d body=%s", replay.Code, stub.calls, replay.Body.String())
	}

	for _, test := range []struct {
		name    string
		service authport.Service
		request *http.Request
		want    int
	}{
		{name: "anonymous", service: &legacyMediaAuthStub{}, request: outboundMediaReconcileHTTPRequest(false, false, "outbound-media-reconcile-key-0004"), want: http.StatusUnauthorized},
		{name: "forbidden", service: &legacyMediaAuthStub{authorizeErr: authport.ErrUnauthorized}, request: outboundMediaReconcileHTTPRequest(true, true, "outbound-media-reconcile-key-0004"), want: http.StatusForbidden},
		{name: "csrf", service: &legacyMediaAuthStub{}, request: outboundMediaReconcileHTTPRequest(true, false, "outbound-media-reconcile-key-0004"), want: http.StatusForbidden},
		{name: "idempotency", service: &legacyMediaAuthStub{}, request: outboundMediaReconcileHTTPRequest(true, true, ""), want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			blocked := &outboundMediaReconcileStub{}
			response := httptest.NewRecorder()
			outboundMediaReconcileRouter(t, test.service, blocked).ServeHTTP(response, test.request)
			if response.Code != test.want || blocked.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, blocked.calls, response.Body.String())
			}
		})
	}

	conflict := &outboundMediaReconcileStub{err: mediaapp.ErrOutboundMediaReconcileConflict}
	response := httptest.NewRecorder()
	outboundMediaReconcileRouter(t, &legacyMediaAuthStub{}, conflict).ServeHTTP(response, outboundMediaReconcileHTTPRequest(true, true, "outbound-media-reconcile-key-0005"))
	if response.Code != http.StatusConflict || conflict.calls != 1 || strings.Contains(response.Body.String(), "provider_receipt_7") {
		t.Fatalf("conflict status=%d calls=%d body=%s", response.Code, conflict.calls, response.Body.String())
	}
}

func outboundMediaReconcileRouter(t *testing.T, service authport.Service, reconcile outboundMediaReconcileApplication) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsAndMedia(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.outboundMediaReconcile = reconcile
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func outboundMediaReconcileHTTPRequest(withSession, withCSRF bool, key string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/admin/outbound-media/42/effects/external_contact_7/reconcile", strings.NewReader(outboundMediaReconcileBody))
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(73)})
	}
	if withCSRF {
		request.Header.Set("X-CSRF-Token", legacyToken(74))
	}
	return request
}

func decodeOutboundMediaReconcileBody(t *testing.T, encoded []byte) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func assertOutboundMediaReconcilePIIMinimal(t *testing.T, body map[string]json.RawMessage, encoded string) {
	t.Helper()
	for _, forbidden := range []string{"content_package", "target", "digest", "payload", "receipt", "evidence", "customer", "idempotency"} {
		if _, ok := body[forbidden]; ok || strings.Contains(encoded, forbidden) {
			t.Fatalf("response leaks %q: %s", forbidden, encoded)
		}
	}
}

func TestReconcileOutboundMediaMapsUnavailable(t *testing.T) {
	stub := &outboundMediaReconcileStub{err: errors.New("store unavailable")}
	request := outboundMediaReconcileHTTPRequest(true, true, "outbound-media-reconcile-key-0006")
	request.SetPathValue("content_package_id", "42")
	request.SetPathValue("target_ref", "external_contact_7")
	response := httptest.NewRecorder()
	(&Handler{outboundMediaReconcile: stub}).ReconcileOutboundMedia(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "store unavailable") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
