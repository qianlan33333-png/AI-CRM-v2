package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
)

func TestHandlerReadUsesReadPermissionWithoutCSRF(t *testing.T) {
	query := useropsport.DirectoryQuery{Keyword: "founder", Limit: 25}
	application := &applicationStub{listCustomers: func(_ context.Context, got useropsport.DirectoryQuery) (useropsport.DirectoryPage, error) {
		if !reflect.DeepEqual(got, query) {
			t.Fatalf("query = %#v", got)
		}
		return useropsport.DirectoryPage{Safety: useropsport.LocalSafety()}, nil
	}}
	authorizer := &authorizerStub{actor: Actor{ID: 8}}
	csrf := &csrfStub{}
	handler := newHandler(t, application, authorizer, csrf)

	result, err := handler.ListCustomers(httptest.NewRequest(stdhttp.MethodGet, "/ignored", nil), query)
	if err != nil || !reflect.DeepEqual(result.Safety, useropsport.LocalSafety()) || !reflect.DeepEqual(authorizer.permissions, []Permission{PermissionAdminRead}) || csrf.calls != 0 {
		t.Fatalf("result/error/permissions/csrf = %#v / %v / %#v / %d", result, err, authorizer.permissions, csrf.calls)
	}
}

func TestHandlerCreatePlanInjectsAuthorizedActorAndIdempotencyAfterCSRF(t *testing.T) {
	application := &applicationStub{createPlan: func(_ context.Context, input useropsport.CreateLocalPlanInput) (useropsport.LocalPlanResult, error) {
		if input.ActorID != 44 || input.IdempotencyKey != "userops-http-key-001" {
			t.Fatalf("input = %#v", input)
		}
		return useropsport.LocalPlanResult{Plan: domain.LocalPlan{ID: 1}, Safety: useropsport.LocalSafety()}, nil
	}}
	authorizer := &authorizerStub{actor: Actor{ID: 44}}
	csrf := &csrfStub{}
	handler := newHandler(t, application, authorizer, csrf)
	request := httptest.NewRequest(stdhttp.MethodPost, "/ignored", nil)
	request.Header.Set("Idempotency-Key", "userops-http-key-001")

	result, err := handler.CreateLocalPlan(request, useropsport.CreateLocalPlanInput{ActorID: 99, IdempotencyKey: "caller-controlled"})
	if err != nil || result.Plan.ID != 1 || !reflect.DeepEqual(authorizer.permissions, []Permission{PermissionAdminWrite}) || csrf.calls != 1 {
		t.Fatalf("result/error/permissions/csrf = %#v / %v / %#v / %d", result, err, authorizer.permissions, csrf.calls)
	}
}

func TestHandlerRejectsWriteBeforeApplicationOnCSRFFailure(t *testing.T) {
	application := &applicationStub{}
	authorizer := &authorizerStub{actor: Actor{ID: 44}}
	csrf := &csrfStub{err: ErrCSRFInvalid}
	handler := newHandler(t, application, authorizer, csrf)
	request := httptest.NewRequest(stdhttp.MethodPut, "/ignored", nil)
	request.Header.Set("Idempotency-Key", "userops-http-key-002")

	_, err := handler.SetDND(request, useropsport.UpsertDNDInput{CustomerID: 7, Reason: "local"})
	if !errors.Is(err, ErrCSRFInvalid) || application.setDNDCalls != 0 || csrf.calls != 1 {
		t.Fatalf("error/calls/csrf = %v / %d / %d", err, application.setDNDCalls, csrf.calls)
	}
}

func TestHandlerPreviewIsReadOnlyAndDoesNotRequireIdempotency(t *testing.T) {
	application := &applicationStub{preview: func(_ context.Context, input useropsport.BatchPreviewInput) (useropsport.BatchPreview, error) {
		if !reflect.DeepEqual(input.CustomerIDs, []domain.CustomerID{7}) {
			t.Fatalf("input = %#v", input)
		}
		return useropsport.BatchPreview{TargetCustomerIDs: []domain.CustomerID{7}, Safety: useropsport.LocalSafety()}, nil
	}}
	authorizer := &authorizerStub{actor: Actor{ID: 44}}
	csrf := &csrfStub{}
	handler := newHandler(t, application, authorizer, csrf)

	result, err := handler.PreviewBatch(httptest.NewRequest(stdhttp.MethodPost, "/ignored", nil), useropsport.BatchPreviewInput{CustomerIDs: []domain.CustomerID{7}})
	if err != nil || len(result.TargetCustomerIDs) != 1 || application.createPlanCalls != 0 || csrf.calls != 0 || !reflect.DeepEqual(authorizer.permissions, []Permission{PermissionAdminRead}) {
		t.Fatalf("result/error/writes/csrf/permissions = %#v / %v / %d / %d / %#v", result, err, application.createPlanCalls, csrf.calls, authorizer.permissions)
	}
}

func TestHandlerRejectsDuplicateIdempotencyKey(t *testing.T) {
	application := &applicationStub{}
	handler := newHandler(t, application, &authorizerStub{actor: Actor{ID: 44}}, &csrfStub{})
	request := httptest.NewRequest(stdhttp.MethodDelete, "/ignored", nil)
	request.Header.Add("Idempotency-Key", "userops-http-key-003")
	request.Header.Add("Idempotency-Key", "userops-http-key-004")

	_, err := handler.ClearDND(request, useropsport.ClearDNDInput{CustomerID: 7, ExpectedVersion: 1})
	if !errors.Is(err, useropsport.ErrInvalid) || application.clearDNDCalls != 0 {
		t.Fatalf("error/calls = %v / %d", err, application.clearDNDCalls)
	}
}

func newHandler(t *testing.T, application useropsport.Application, authorizer Authorizer, csrf CSRFVerifier) *Handler {
	t.Helper()
	handler, err := NewHandler(application, authorizer, csrf)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

type applicationStub struct {
	overview        func(context.Context, useropsport.DirectoryQuery) (useropsport.Overview, error)
	listCustomers   func(context.Context, useropsport.DirectoryQuery) (useropsport.DirectoryPage, error)
	detail          func(context.Context, domain.CustomerID) (useropsport.CustomerDetailResult, error)
	export          func(context.Context, useropsport.SafeExportRequest) (useropsport.SafeExport, error)
	preview         func(context.Context, useropsport.BatchPreviewInput) (useropsport.BatchPreview, error)
	createPlan      func(context.Context, useropsport.CreateLocalPlanInput) (useropsport.LocalPlanResult, error)
	setDND          func(context.Context, useropsport.UpsertDNDInput) (useropsport.DNDMutationResult, error)
	clearDND        func(context.Context, useropsport.ClearDNDInput) (useropsport.DNDMutationResult, error)
	sendRecords     func(context.Context, useropsport.SendRecordQuery) (useropsport.SendRecordPage, error)
	createPlanCalls int
	setDNDCalls     int
	clearDNDCalls   int
}

func (stub *applicationStub) Overview(ctx context.Context, input useropsport.DirectoryQuery) (useropsport.Overview, error) {
	if stub.overview == nil {
		return useropsport.Overview{}, useropsport.ErrUnavailable
	}
	return stub.overview(ctx, input)
}

func (stub *applicationStub) ListCustomers(ctx context.Context, input useropsport.DirectoryQuery) (useropsport.DirectoryPage, error) {
	if stub.listCustomers == nil {
		return useropsport.DirectoryPage{}, useropsport.ErrUnavailable
	}
	return stub.listCustomers(ctx, input)
}

func (stub *applicationStub) GetCustomerDetail(ctx context.Context, customerID domain.CustomerID) (useropsport.CustomerDetailResult, error) {
	if stub.detail == nil {
		return useropsport.CustomerDetailResult{}, useropsport.ErrUnavailable
	}
	return stub.detail(ctx, customerID)
}

func (stub *applicationStub) SafeExport(ctx context.Context, input useropsport.SafeExportRequest) (useropsport.SafeExport, error) {
	if stub.export == nil {
		return useropsport.SafeExport{}, useropsport.ErrUnavailable
	}
	return stub.export(ctx, input)
}

func (stub *applicationStub) PreviewBatch(ctx context.Context, input useropsport.BatchPreviewInput) (useropsport.BatchPreview, error) {
	if stub.preview == nil {
		return useropsport.BatchPreview{}, useropsport.ErrUnavailable
	}
	return stub.preview(ctx, input)
}

func (stub *applicationStub) CreateLocalPlan(ctx context.Context, input useropsport.CreateLocalPlanInput) (useropsport.LocalPlanResult, error) {
	stub.createPlanCalls++
	if stub.createPlan == nil {
		return useropsport.LocalPlanResult{}, useropsport.ErrUnavailable
	}
	return stub.createPlan(ctx, input)
}

func (stub *applicationStub) SetDND(ctx context.Context, input useropsport.UpsertDNDInput) (useropsport.DNDMutationResult, error) {
	stub.setDNDCalls++
	if stub.setDND == nil {
		return useropsport.DNDMutationResult{}, useropsport.ErrUnavailable
	}
	return stub.setDND(ctx, input)
}

func (stub *applicationStub) ClearDND(ctx context.Context, input useropsport.ClearDNDInput) (useropsport.DNDMutationResult, error) {
	stub.clearDNDCalls++
	if stub.clearDND == nil {
		return useropsport.DNDMutationResult{}, useropsport.ErrUnavailable
	}
	return stub.clearDND(ctx, input)
}

func (stub *applicationStub) ListSendRecords(ctx context.Context, input useropsport.SendRecordQuery) (useropsport.SendRecordPage, error) {
	if stub.sendRecords == nil {
		return useropsport.SendRecordPage{}, useropsport.ErrUnavailable
	}
	return stub.sendRecords(ctx, input)
}

type authorizerStub struct {
	actor       Actor
	err         error
	permissions []Permission
}

func (stub *authorizerStub) Authorize(_ context.Context, permission Permission) (Actor, error) {
	stub.permissions = append(stub.permissions, permission)
	return stub.actor, stub.err
}

type csrfStub struct {
	calls int
	err   error
}

func (stub *csrfStub) Verify(*stdhttp.Request) error {
	stub.calls++
	return stub.err
}
