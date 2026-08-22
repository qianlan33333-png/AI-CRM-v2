package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/http"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

type servicePeriodMemberRouteSpy struct {
	addCalls  int
	listCalls int
	add       memberport.AddCommand
}

func (spy *servicePeriodMemberRouteSpy) Add(_ context.Context, command memberport.AddCommand) (memberdomain.Member, error) {
	spy.addCalls++
	spy.add = command
	return memberdomain.Member{}, nil
}

func (*servicePeriodMemberRouteSpy) Expire(context.Context, memberport.TransitionCommand) (memberdomain.Member, error) {
	return memberdomain.Member{}, nil
}

func (*servicePeriodMemberRouteSpy) Remove(context.Context, memberport.TransitionCommand) (memberdomain.Member, error) {
	return memberdomain.Member{}, nil
}

func (*servicePeriodMemberRouteSpy) UpdateFields(context.Context, memberport.UpdateFieldsCommand) (memberdomain.Member, error) {
	return memberdomain.Member{}, nil
}

func (*servicePeriodMemberRouteSpy) Get(context.Context, int64, string) (memberdomain.Member, error) {
	return memberdomain.Member{}, nil
}

func (spy *servicePeriodMemberRouteSpy) List(context.Context, memberport.ListQuery) (memberport.ListResult, error) {
	spy.listCalls++
	return memberport.ListResult{Items: []memberdomain.Member{}, Limit: memberport.DefaultLimit}, nil
}

func (*servicePeriodMemberRouteSpy) Export(context.Context, memberport.ExportQuery) (memberport.ExportResult, error) {
	return memberport.ExportResult{}, nil
}

func TestServicePeriodMemberRoutesUseCentralGlobalAuthorizationAndCSRF(t *testing.T) {
	authService := &legacyAuthStub{}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	application := &servicePeriodMemberRouteSpy{}
	memberHandler, err := memberhttp.NewHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{Handler: authHandler, servicePeriodMembers: memberHandler}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		authHandler,
		candidate,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/admin/service-period-products/7/members"
	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, path, nil))
	if anonymous.Code != http.StatusUnauthorized || application.listCalls != 0 {
		t.Fatalf("anonymous status/calls=%d/%d body=%s", anonymous.Code, application.listCalls, anonymous.Body.String())
	}

	listRequest := httptest.NewRequest(http.MethodGet, path, nil)
	listRequest.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(31)})
	list := httptest.NewRecorder()
	router.ServeHTTP(list, listRequest)
	if list.Code != http.StatusOK || application.listCalls != 1 {
		t.Fatalf("list status/calls=%d/%d body=%s", list.Code, application.listCalls, list.Body.String())
	}

	body := bytes.NewBufferString(`{"customer_id":9,"source":"manual"}`)
	missingCSRFRequest := httptest.NewRequest(http.MethodPost, path, nil)
	missingCSRFRequest.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(32)})
	missingCSRFRequest.Body = io.NopCloser(body)
	missingCSRFRequest.Header.Set("Content-Type", "application/json")
	missingCSRFRequest.Header.Set("Idempotency-Key", "service-member-add-0001")
	missingCSRF := httptest.NewRecorder()
	router.ServeHTTP(missingCSRF, missingCSRFRequest)
	if missingCSRF.Code != http.StatusForbidden || application.addCalls != 0 {
		t.Fatalf("missing csrf status/calls=%d/%d body=%s", missingCSRF.Code, application.addCalls, missingCSRF.Body.String())
	}

	createRequest := httptest.NewRequest(http.MethodPost, path, nil)
	createRequest.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(33)})
	createRequest.Body = io.NopCloser(bytes.NewBufferString(`{"customer_id":9,"source":"manual"}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Idempotency-Key", "service-member-add-0002")
	createRequest.Header.Set("X-CSRF-Token", legacyToken(34))
	created := httptest.NewRecorder()
	router.ServeHTTP(created, createRequest)
	if created.Code != http.StatusCreated || application.addCalls != 1 || application.add.ServiceProductID != 7 ||
		application.add.CustomerID != 9 || application.add.Source != memberdomain.SourceManual || application.add.ActorID != 1 {
		t.Fatalf("create status/calls/command=%d/%d/%+v body=%s", created.Code, application.addCalls, application.add, created.Body.String())
	}
}
