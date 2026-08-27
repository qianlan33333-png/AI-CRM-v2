package membergrid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type externalShareManagementStub struct {
	result  SetExternalShareResult
	err     error
	command SetExternalShareCommand
	calls   int
}

func (application *externalShareManagementStub) SetExternalShare(_ context.Context, command SetExternalShareCommand) (SetExternalShareResult, error) {
	application.calls++
	application.command = command
	return application.result, application.err
}

func externalShareFragment(t *testing.T, application *externalShareManagementStub, authorizer *fakeManagementAuthorizer, csrf *fakeManagementCSRF) http.Handler {
	t.Helper()
	if authorizer == nil {
		authorizer = &fakeManagementAuthorizer{actor: ManagementActor{ID: 17}}
	}
	if csrf == nil {
		csrf = &fakeManagementCSRF{}
	}
	handler, err := NewExternalShareManagementHandler(application, authorizer, csrf)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := NewExternalShareManagementRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
}

func TestExternalShareHTTPReturnsOneTimeFragmentPath(t *testing.T) {
	token := "mgshare1.share_abcdefghijklmnopqrstuv.abcdefghijklmnopqrstuvwxyzABCDEFGHijk"
	application := &externalShareManagementStub{result: SetExternalShareResult{
		Share:       ExternalShare{ServiceProductID: 8, ShareID: "share_abcdefghijklmnopqrstuv", Enabled: true, Version: 1},
		PublicToken: token, TokenIssued: true,
	}}
	authorizer := &fakeManagementAuthorizer{actor: ManagementActor{ID: 23}}
	csrf := &fakeManagementCSRF{}
	fragment := externalShareFragment(t, application, authorizer, csrf)
	request := managementRequest(http.MethodPut, RoutePrefix+"/8/member-grid/share-settings", `{"enabled":true,"expected_version":0}`, "external-share-enable-0001")
	response := httptest.NewRecorder()
	fragment.ServeHTTP(response, request)
	if response.Code != http.StatusOK || application.calls != 1 || csrf.calls != 1 || len(authorizer.capabilities) != 1 || authorizer.capabilities[0] != CapabilityProductsWrite {
		t.Fatalf("status/calls/csrf/capabilities=%d/%d/%d/%v body=%s", response.Code, application.calls, csrf.calls, authorizer.capabilities, response.Body.String())
	}
	if application.command != (SetExternalShareCommand{ServiceProductID: 8, Enabled: true, ExpectedVersion: 0, ActorID: 23, IdempotencyKey: "external-share-enable-0001"}) {
		t.Fatalf("command=%+v", application.command)
	}
	var body SetExternalShareResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || !body.ExternalShareEnabled || body.ExternalShareVersion != 1 || !body.TokenIssued || body.PublicPath != PublicSharePagePath+"#"+token || body.RealExternalCallExecuted {
		t.Fatalf("body=%+v", body)
	}
	if strings.Contains(response.Body.String(), "share_id") || strings.Contains(response.Body.String(), "service_product_id") {
		t.Fatalf("response leaked internal identifiers: %s", response.Body.String())
	}
}

func TestExternalShareHTTPDisableDoesNotReturnToken(t *testing.T) {
	application := &externalShareManagementStub{result: SetExternalShareResult{Share: ExternalShare{ServiceProductID: 8, Enabled: false, Version: 2}}}
	fragment := externalShareFragment(t, application, nil, nil)
	response := httptest.NewRecorder()
	fragment.ServeHTTP(response, managementRequest(http.MethodPut, RoutePrefix+"/8/member-grid/share-settings", `{"enabled":false,"expected_version":1}`, "external-share-disable-001"))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "public_path") || strings.Contains(response.Body.String(), "mgshare1") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestExternalShareHTTPRejectsIncompleteOrUnsafeRequests(t *testing.T) {
	for name, request := range map[string]*http.Request{
		"missing version": managementRequest(http.MethodPut, RoutePrefix+"/8/member-grid/share-settings", `{"enabled":true}`, "external-share-enable-0001"),
		"unknown field":   managementRequest(http.MethodPut, RoutePrefix+"/8/member-grid/share-settings", `{"enabled":true,"expected_version":0,"token":"x"}`, "external-share-enable-0001"),
		"missing key":     managementRequest(http.MethodPut, RoutePrefix+"/8/member-grid/share-settings", `{"enabled":true,"expected_version":0}`, ""),
		"query":           managementRequest(http.MethodPut, RoutePrefix+"/8/member-grid/share-settings?x=1", `{"enabled":true,"expected_version":0}`, "external-share-enable-0001"),
		"wrong method":    managementRequest(http.MethodPost, RoutePrefix+"/8/member-grid/share-settings", `{"enabled":true,"expected_version":0}`, "external-share-enable-0001"),
	} {
		t.Run(name, func(t *testing.T) {
			application := &externalShareManagementStub{}
			response := httptest.NewRecorder()
			externalShareFragment(t, application, nil, nil).ServeHTTP(response, request)
			if response.Code < 400 || application.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, application.calls, response.Body.String())
			}
		})
	}
}
