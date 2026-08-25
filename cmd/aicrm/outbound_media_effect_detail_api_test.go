package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

type outboundMediaEffectDetailStub struct {
	detail                  mediaapp.OutboundMediaEffectDetail
	err                     error
	contentPackageID, calls int64
	targetRef               string
}

func (s *outboundMediaEffectDetailStub) ReadOutboundMediaEffectDetail(_ context.Context, contentPackageID int64, targetRef string) (mediaapp.OutboundMediaEffectDetail, error) {
	s.calls++
	s.contentPackageID, s.targetRef = contentPackageID, targetRef
	return s.detail, s.err
}

func TestGetOutboundMediaEffectDetailReturnsPIIMinimalDetail(t *testing.T) {
	stub := &outboundMediaEffectDetailStub{detail: mediaapp.OutboundMediaEffectDetail{ContentPackageID: 42, EffectID: "eer_7", State: "accepted"}}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/outbound-media/42/effects/external_contact_7", nil)
	request.SetPathValue("content_package_id", "42")
	request.SetPathValue("target_ref", "external_contact_7")
	response := httptest.NewRecorder()
	(&Handler{outboundMediaDetail: stub}).GetOutboundMediaEffectDetail(response, request)
	if response.Code != http.StatusOK || stub.calls != 1 || stub.contentPackageID != 42 || stub.targetRef != "external_contact_7" {
		t.Fatalf("status=%d stub=%#v body=%s", response.Code, stub, response.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 5 || string(body["content_package_id"]) != "42" || string(body["effect_id"]) != `"eer_7"` || string(body["state"]) != `"accepted"` || string(body["provider_accepted"]) != "false" || string(body["delivery_proven"]) != "false" {
		t.Fatalf("body=%s", response.Body.String())
	}
	for _, forbidden := range []string{"target", "digest", "payload", "receipt", "customer"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("response leaks %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestGetOutboundMediaEffectDetailRejectsInvalidPathAndMapsNotFound(t *testing.T) {
	stub := &outboundMediaEffectDetailStub{err: errors.New("not found")}
	for _, test := range []struct {
		name, packageID, targetRef string
		want                       int
	}{
		{name: "invalid target", packageID: "42", targetRef: "external/contact", want: http.StatusBadRequest},
		{name: "invalid package", packageID: "0", targetRef: "external_contact", want: http.StatusBadRequest},
		{name: "missing", packageID: "42", targetRef: "external_contact", want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.SetPathValue("content_package_id", test.packageID)
			request.SetPathValue("target_ref", test.targetRef)
			response := httptest.NewRecorder()
			(&Handler{outboundMediaDetail: stub}).GetOutboundMediaEffectDetail(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestOutboundMediaEffectDetailRouteRequiresMediaReadWithoutCSRF(t *testing.T) {
	stub := &outboundMediaEffectDetailStub{detail: mediaapp.OutboundMediaEffectDetail{ContentPackageID: 42, EffectID: "eer_7", State: "accepted"}}
	auth := &legacyMediaAuthStub{}
	router := outboundMediaEffectDetailRouter(t, auth, stub)
	request := httptest.NewRequest(http.MethodGet, "/api/admin/outbound-media/42/effects/external_contact_7", nil)
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(71)})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || stub.calls != 1 || auth.authenticateCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 {
		t.Fatalf("status=%d stub=%#v auth=%#v body=%s", response.Code, stub, auth, response.Body.String())
	}

	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/admin/outbound-media/42/effects/external_contact_7", nil))
	if anonymous.Code != http.StatusUnauthorized || stub.calls != 1 {
		t.Fatalf("anonymous status=%d calls=%d body=%s", anonymous.Code, stub.calls, anonymous.Body.String())
	}

	forbiddenStub := &outboundMediaEffectDetailStub{detail: stub.detail}
	forbiddenRouter := outboundMediaEffectDetailRouter(t, &legacyMediaAuthStub{authorizeErr: authport.ErrUnauthorized}, forbiddenStub)
	forbiddenRequest := httptest.NewRequest(http.MethodGet, "/api/admin/outbound-media/42/effects/external_contact_7", nil)
	forbiddenRequest.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(72)})
	forbidden := httptest.NewRecorder()
	forbiddenRouter.ServeHTTP(forbidden, forbiddenRequest)
	if forbidden.Code != http.StatusForbidden || forbiddenStub.calls != 0 {
		t.Fatalf("forbidden status=%d calls=%d body=%s", forbidden.Code, forbiddenStub.calls, forbidden.Body.String())
	}
}

func outboundMediaEffectDetailRouter(t *testing.T, service authport.Service, detail outboundMediaEffectDetailApplication) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsAndMedia(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.outboundMediaDetail = detail
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
