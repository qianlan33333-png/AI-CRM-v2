package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/http"
)

type outboundCampaignRootApplication struct {
	result  outbound.CampaignHandoffSummary
	accepts int
}

func (application *outboundCampaignRootApplication) Accept(context.Context, outboundapp.AcceptCampaignHandoffCommand) (outbound.CampaignHandoffSummary, error) {
	application.accepts++
	return application.result, nil
}
func (application *outboundCampaignRootApplication) Get(context.Context, string, string) (outbound.CampaignHandoffSummary, error) {
	return application.result, nil
}

func TestOutboundCampaignHandoffRootValidatesCSRFExactlyOnce(t *testing.T) {
	csrf := legacyToken(0x41)
	auth := &campaignInitiationRootAuth{expectedCSRF: csrf}
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	application := &outboundCampaignRootApplication{result: campaignHandoffRootSummary()}
	handoffHandler, err := outboundhttp.NewCampaignHandoffHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(auth, &legacyCustomerStub{result: legacyCustomerResult()})
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{Handler: authHandler, outboundCampaignHandoff: handoffHandler}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, candidate, legacy)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/outbound/campaign-handoffs/spring-campaign/" + application.result.PlanID + "/accept"
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"expected_review_version":3}`))
	request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(0x40)})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "aaaaaaaaaaaaaaaa")
	request.Header.Set("X-CSRF-Token", csrf)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || auth.csrfCalls != 1 || application.accepts != 1 {
		t.Fatalf("status=%d csrf=%d accepts=%d body=%s", response.Code, auth.csrfCalls, application.accepts, response.Body)
	}
}

func campaignHandoffRootSummary() outbound.CampaignHandoffSummary {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return outbound.CampaignHandoffSummary{ID: 9, CampaignCode: "spring-campaign", PlanID: "ctp_" + digest, ReviewVersion: 3, Status: outbound.CampaignHandoffHeld, TargetCount: 2, StepCount: 1, HeldCount: 2, NotEvaluatedCount: 2, AcceptedAt: time.Date(2026, 8, 23, 3, 4, 5, 0, time.UTC), Safety: outbound.LocalCampaignHandoffSafety()}
}
