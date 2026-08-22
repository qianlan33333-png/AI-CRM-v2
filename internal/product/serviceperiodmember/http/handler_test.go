package http

import (
	"bytes"
	"context"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

type applicationStub struct {
	addCommand memberport.AddCommand
	addErr     error
	listCalls  int
}

func (stub *applicationStub) Add(_ context.Context, command memberport.AddCommand) (memberdomain.Member, error) {
	stub.addCommand = command
	return memberdomain.Member{}, stub.addErr
}
func (*applicationStub) Expire(context.Context, memberport.TransitionCommand) (memberdomain.Member, error) {
	return memberdomain.Member{}, nil
}
func (*applicationStub) Remove(context.Context, memberport.TransitionCommand) (memberdomain.Member, error) {
	return memberdomain.Member{}, nil
}
func (*applicationStub) UpdateFields(context.Context, memberport.UpdateFieldsCommand) (memberdomain.Member, error) {
	return memberdomain.Member{}, nil
}
func (*applicationStub) Get(context.Context, int64, string) (memberdomain.Member, error) {
	return memberdomain.Member{}, nil
}
func (stub *applicationStub) List(context.Context, memberport.ListQuery) (memberport.ListResult, error) {
	stub.listCalls++
	return memberport.ListResult{}, nil
}
func (*applicationStub) Export(context.Context, memberport.ExportQuery) (memberport.ExportResult, error) {
	return memberport.ExportResult{Filename: "ignored.csv", ContentType: "text/csv; charset=utf-8", Body: []byte("member_ref\n")}, nil
}

func TestMissingCentralAuthorizationCannotAuthorizeMemberWrites(t *testing.T) {
	application := &applicationStub{}
	handler, err := NewHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(stdhttp.MethodPost, "/members", bytes.NewBufferString(`{"customer_id":9,"source":"manual"}`))
	request = request.WithContext(authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 3, Role: authport.RoleOps}, authport.SessionRef("session")))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "member-write-0001")
	response := httptest.NewRecorder()
	handler.Add(response, request, 7)
	if response.Code != stdhttp.StatusForbidden || application.addCommand.ServiceProductID != 0 {
		t.Fatalf("status/command=%d/%#v body=%s", response.Code, application.addCommand, response.Body.String())
	}
}

func TestPaidOrderSourceRedlineMapsToConflict(t *testing.T) {
	application := &applicationStub{addErr: memberport.ErrPaidOrderSourceBlocked}
	handler, _ := NewHandler(application)
	request := authorizedRequest(t, stdhttp.MethodPost, "/members", bytes.NewBufferString(`{"customer_id":9,"source":"paid_order"}`), authport.CapabilityEntitlementsWrite, authport.ScopeGlobal)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "member-write-0002")
	response := httptest.NewRecorder()
	handler.Add(response, request, 7)
	if response.Code != stdhttp.StatusConflict || application.addCommand.Source != memberdomain.SourcePaidOrder || application.addCommand.ActorID != 3 {
		t.Fatalf("status/command=%d/%#v body=%s", response.Code, application.addCommand, response.Body.String())
	}
}

func TestListRejectsUnknownOrRepeatedQueryParameters(t *testing.T) {
	application := &applicationStub{}
	handler, _ := NewHandler(application)
	for _, target := range []string{"/members?unionid=secret", "/members?limit=1&limit=2"} {
		request := authorizedRequest(t, stdhttp.MethodGet, target, nil, authport.CapabilityEntitlementsRead, authport.ScopeGlobal)
		response := httptest.NewRecorder()
		handler.List(response, request, 7)
		if response.Code != stdhttp.StatusUnprocessableEntity || application.listCalls != 0 {
			t.Fatalf("target/status/calls=%s/%d/%d body=%s", target, response.Code, application.listCalls, response.Body.String())
		}
	}
}

func TestExportSetsFixedSafeDownloadHeaders(t *testing.T) {
	handler, _ := NewHandler(&applicationStub{})
	request := authorizedRequest(t, stdhttp.MethodPost, "/members/export", bytes.NewBufferString(`{"columns":["member_ref"]}`), authport.CapabilityEntitlementsRead, authport.ScopeGlobal)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.Export(response, request, 7)
	if response.Code != stdhttp.StatusOK || response.Header().Get("Content-Disposition") != `attachment; filename="service-period-members.csv"` || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Body.String() != "member_ref\n" {
		t.Fatalf("status/headers/body=%d/%v/%q", response.Code, response.Header(), response.Body.String())
	}
}

func authorizedRequest(t *testing.T, method, target string, body *bytes.Buffer, capability authport.Capability, scope authport.ScopeKind) *stdhttp.Request {
	t.Helper()
	var request *stdhttp.Request
	if body == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, body)
	}
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 3, Role: authport.RoleOps}, authport.SessionRef("session"))
	authorization := authport.Authorization{Capability: capability, Scope: scope}
	ctx, err := authport.WithAuthorization(ctx, authorization)
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}
