package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestCustomerAcquisitionLinkHandlerDecodesSnakeCaseAndReturnsOnlyListSummaries(t *testing.T) {
	application := &customerAcquisitionLinkApplicationStub{
		page:   wecomport.CustomerAcquisitionLinkPage{Links: []wecomport.CustomerAcquisitionLink{{LinkID: "link-1", URL: "https://work.weixin.qq.com/unsafe"}}},
		create: wecomapp.CustomerAcquisitionLinkReceipt{ID: 8, State: wecomapp.CustomerAcquisitionLinkExecuted, OutcomeDigest: [32]byte{1}, BusinessEndpointDispatched: true, RealExternalCallExecuted: true, Link: &wecomport.CustomerAcquisitionLink{LinkID: "link-1", LinkName: "获客", URL: "https://work.weixin.qq.com/ca/link-1", UserIDs: []string{"staff-1"}, DepartmentIDs: []int64{7}, SkipVerify: true}},
	}
	handler := NewCustomerAcquisitionLinkHandler(application)
	create := customerAcquisitionLinkRequest(http.MethodPost, CustomerAcquisitionLinksPath, `{"link_name":"获客","user_ids":["staff-1"],"department_ids":[7],"skip_verify":true}`, authport.CapabilityChannelsWrite)
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusAccepted || application.createCommand.Input.LinkName != "获客" || application.createCommand.Input.UserIDs[0] != "staff-1" || application.createCommand.Input.DepartmentIDs[0] != 7 || !application.createCommand.Input.SkipVerify {
		t.Fatalf("status/command/body=%d/%+v/%s", created.Code, application.createCommand, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "secret") || strings.Contains(created.Body.String(), "provider_receipt_digest") || strings.Contains(created.Body.String(), `"resolution"`) || !strings.Contains(created.Body.String(), `"outcome_digest":"`) || !strings.Contains(created.Body.String(), `"real_external_call_executed":true`) {
		t.Fatalf("unsafe or incomplete receipt=%s", created.Body.String())
	}
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, customerAcquisitionLinkRequest(http.MethodGet, CustomerAcquisitionLinksPath, "", authport.CapabilityChannelsRead))
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"link_id":"link-1"`) || strings.Contains(listed.Body.String(), "unsafe") {
		t.Fatalf("list status/body=%d/%s", listed.Code, listed.Body.String())
	}
}

func TestCustomerAcquisitionLinkReceiptOmitsZeroOutcomeDigest(t *testing.T) {
	value := customerAcquisitionLinkReceiptValue(wecomapp.CustomerAcquisitionLinkReceipt{ID: 8, State: wecomapp.CustomerAcquisitionLinkReserved})
	if _, exists := value["outcome_digest"]; exists {
		t.Fatalf("zero local outcome digest must not serialize as evidence: %+v", value)
	}
	if _, exists := value["resolution"]; exists {
		t.Fatalf("unreconciled receipt must not serialize an empty resolution: %+v", value)
	}
	reconciled := customerAcquisitionLinkReceiptValue(wecomapp.CustomerAcquisitionLinkReceipt{ID: 8, State: wecomapp.CustomerAcquisitionLinkReconciled, Resolution: wecomapp.CustomerAcquisitionLinkProviderApplied})
	if reconciled["resolution"] != wecomapp.CustomerAcquisitionLinkProviderApplied {
		t.Fatalf("reconciled receipt must serialize its resolution: %+v", reconciled)
	}
}

func TestCustomerAcquisitionLinkHandlerFailsClosedForUnknownInputAndMissingAuthorization(t *testing.T) {
	application := &customerAcquisitionLinkApplicationStub{}
	handler := NewCustomerAcquisitionLinkHandler(application)
	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, customerAcquisitionLinkRequest(http.MethodPost, CustomerAcquisitionLinksPath, `{"link_name":"获客","user_ids":["staff-1"],"department_ids":[],"skip_verify":false,"provider_secret":"forbidden"}`, authport.CapabilityChannelsWrite))
	if unknown.Code != http.StatusBadRequest || application.createCalls != 0 {
		t.Fatalf("unknown input status/calls=%d/%d", unknown.Code, application.createCalls)
	}
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, CustomerAcquisitionLinksPath, nil))
	if denied.Code != http.StatusForbidden || application.listCalls != 0 {
		t.Fatalf("unauthorized status/calls=%d/%d", denied.Code, application.listCalls)
	}
}

func TestCustomerAcquisitionLinkHandlerRejectsOnlyMappedLegacyActions(t *testing.T) {
	application := &customerAcquisitionLinkApplicationStub{setEnabledErr: wecomapp.ErrCustomerAcquisitionLinkUnsupported}
	handler := NewCustomerAcquisitionLinkHandler(application)
	enable := httptest.NewRecorder()
	handler.ServeHTTP(enable, customerAcquisitionLinkRequest(http.MethodPost, CustomerAcquisitionLinksPath+"/link-1/enable", `{}`, authport.CapabilityChannelsWrite))
	if enable.Code != http.StatusUnprocessableEntity || application.setEnabledCalls != 1 || application.setEnabledValue != true || !strings.Contains(enable.Body.String(), "customer_acquisition_link_unsupported") || strings.Contains(enable.Body.String(), "real_external_call_executed") {
		t.Fatalf("enable status/calls/body=%d/%d/%+v/%s", enable.Code, application.setEnabledCalls, application.setEnabledValue, enable.Body.String())
	}
	invalidBody := httptest.NewRecorder()
	handler.ServeHTTP(invalidBody, customerAcquisitionLinkRequest(http.MethodPost, CustomerAcquisitionLinksPath+"/link-1/disable", `{"unexpected":true}`, authport.CapabilityChannelsWrite))
	if invalidBody.Code != http.StatusBadRequest || application.setEnabledCalls != 1 {
		t.Fatalf("invalid action body status/calls=%d/%d", invalidBody.Code, application.setEnabledCalls)
	}
	sync := httptest.NewRecorder()
	handler.ServeHTTP(sync, customerAcquisitionLinkRequest(http.MethodPost, CustomerAcquisitionLinksPath+"/link-1/sync", "", authport.CapabilityChannelsWrite))
	if sync.Code != http.StatusNotFound || application.setEnabledCalls != 1 {
		t.Fatalf("unmapped sync status/calls=%d/%d", sync.Code, application.setEnabledCalls)
	}
}

type customerAcquisitionLinkApplicationStub struct {
	page            wecomport.CustomerAcquisitionLinkPage
	create          wecomapp.CustomerAcquisitionLinkReceipt
	createCommand   wecomapp.CustomerAcquisitionLinkCommand
	listCalls       int
	createCalls     int
	setEnabledCalls int
	setEnabledValue bool
	setEnabledErr   error
}

func (stub *customerAcquisitionLinkApplicationStub) List(context.Context, string, int) (wecomport.CustomerAcquisitionLinkPage, error) {
	stub.listCalls++
	return stub.page, nil
}

func (*customerAcquisitionLinkApplicationStub) Get(context.Context, string) (wecomport.CustomerAcquisitionLink, error) {
	return wecomport.CustomerAcquisitionLink{}, nil
}

func (stub *customerAcquisitionLinkApplicationStub) Create(_ context.Context, command wecomapp.CustomerAcquisitionLinkCommand) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	stub.createCalls++
	stub.createCommand = command
	return stub.create, nil
}

func (*customerAcquisitionLinkApplicationStub) Update(context.Context, wecomapp.CustomerAcquisitionLinkCommand) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	return wecomapp.CustomerAcquisitionLinkReceipt{}, nil
}

func (*customerAcquisitionLinkApplicationStub) Delete(context.Context, wecomapp.CustomerAcquisitionLinkCommand) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	return wecomapp.CustomerAcquisitionLinkReceipt{}, nil
}

func (stub *customerAcquisitionLinkApplicationStub) SetEnabled(_ context.Context, _ string, enabled bool) error {
	stub.setEnabledCalls++
	stub.setEnabledValue = enabled
	return stub.setEnabledErr
}

func (*customerAcquisitionLinkApplicationStub) Reconcile(context.Context, wecomapp.ReconcileCustomerAcquisitionLinkCommand) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	return wecomapp.CustomerAcquisitionLinkReceipt{}, nil
}

func customerAcquisitionLinkRequest(method, path, body string, capability authport.Capability) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Idempotency-Key", "customer-acquisition-link-key-0001")
	contextWithSession := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, "customer-acquisition-session")
	contextWithAuthorization, _ := authport.WithAuthorization(contextWithSession, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	return request.WithContext(contextWithAuthorization)
}
