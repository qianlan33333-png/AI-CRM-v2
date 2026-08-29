package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/product/membergrid"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
)

type memberGridRouteSpy struct {
	accessCalls int
	queryCalls  int
	updateCalls int
	query       membergrid.QueryInput
	update      membergrid.UpdateFieldsCommand
}

func (spy *memberGridRouteSpy) Access(_ context.Context, productID int64) (membergrid.AccessResponse, error) {
	spy.accessCalls++
	return membergrid.AccessResponse{ProductID: productID, CanView: true, CanQuery: true}, nil
}

func (*memberGridRouteSpy) Schema(_ context.Context, productID int64) (membergrid.SchemaResponse, error) {
	return membergrid.SchemaResponse{ServiceProductID: productID}, nil
}

func (*memberGridRouteSpy) MemberViews(_ context.Context, productID int64) (membergrid.MemberViewsResponse, error) {
	return membergrid.MemberViewsResponse{ProductID: productID}, nil
}

func (spy *memberGridRouteSpy) Query(_ context.Context, input membergrid.QueryInput) (membergrid.QueryResponse, error) {
	spy.queryCalls++
	spy.query = input
	return membergrid.QueryResponse{Rows: []membergrid.MemberRow{}, Limit: input.Limit}, nil
}

func (spy *memberGridRouteSpy) UpdateFields(_ context.Context, command membergrid.UpdateFieldsCommand) (memberdomain.Member, error) {
	spy.updateCalls++
	spy.update = command
	return memberdomain.Member{}, nil
}

func TestMemberGridRoutesUseLegacyAuthenticationAndReadCapabilities(t *testing.T) {
	service := &legacyAuthStub{}
	legacy, err := NewHandlerWithOutboundAndProducts(
		service,
		&legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{},
		&legacyCancelStub{},
		&legacyRetryStub{},
		&legacyProductStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	application := &memberGridRouteSpy{}
	leaf, err := membergrid.NewHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	legacy.memberGrid, err = membergrid.NewRouteFragment(leaf)
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		authHandler,
		authHandler,
		legacy,
	)
	if err != nil {
		t.Fatal(err)
	}

	accessPath := membergrid.RoutePrefix + "/42/member-grid/access"
	queryPath := membergrid.RoutePrefix + "/43/member-grid/query"

	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, accessPath, nil))
	if anonymous.Code != http.StatusUnauthorized || application.accessCalls != 0 {
		t.Fatalf("anonymous status/calls=%d/%d body=%s", anonymous.Code, application.accessCalls, anonymous.Body.String())
	}

	access := httptest.NewRecorder()
	router.ServeHTTP(access, legacyRequest(http.MethodGet, accessPath, legacyToken(221)))
	if access.Code != http.StatusOK || application.accessCalls != 1 {
		t.Fatalf("access status/calls=%d/%d body=%s", access.Code, application.accessCalls, access.Body.String())
	}

	queryRequest := legacyRequest(http.MethodPost, queryPath, legacyToken(222))
	queryRequest.Body = io.NopCloser(strings.NewReader(`{}`))
	queryRequest.Header.Set("Content-Type", "application/json")
	query := httptest.NewRecorder()
	router.ServeHTTP(query, queryRequest)
	if query.Code != http.StatusOK || application.queryCalls != 1 || application.query.ProductID != 43 ||
		application.query.State != membergrid.StateAll || application.query.Limit != membergrid.DefaultLimit {
		t.Fatalf("query status/calls/input=%d/%d/%+v body=%s", query.Code, application.queryCalls, application.query, query.Body.String())
	}
}

func TestMemberGridFieldsRouteUsesMemberGridWriteAndCSRF(t *testing.T) {
	service := &legacyAuthStub{}
	legacy, err := NewHandlerWithOutboundAndProducts(
		service,
		&legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{},
		&legacyCancelStub{},
		&legacyRetryStub{},
		&legacyProductStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	application := &memberGridRouteSpy{}
	leaf, err := membergrid.NewHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	legacy.memberGrid, err = membergrid.NewRouteFragment(leaf)
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		authHandler,
		authHandler,
		legacy,
	)
	if err != nil {
		t.Fatal(err)
	}
	path := membergrid.RoutePrefix + "/43/members/spm_0000000000000000000001/fields"
	missingCSRF := legacyRequest(http.MethodPut, path, legacyToken(31))
	missingCSRF.Body = io.NopCloser(strings.NewReader(`{"expected_version":2,"remark":"备注"}`))
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.Header.Set("Idempotency-Key", "member-grid-fields-0001")
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, missingCSRF)
	if denied.Code != http.StatusForbidden || application.updateCalls != 0 {
		t.Fatalf("missing csrf status/calls=%d/%d body=%s", denied.Code, application.updateCalls, denied.Body.String())
	}

	request := legacyRequest(http.MethodPut, path, legacyToken(32))
	request.Body = io.NopCloser(strings.NewReader(`{"expected_version":2,"remark":"备注","alliance":"联盟"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "member-grid-fields-0001")
	request.Header.Set("X-CSRF-Token", legacyToken(33))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || application.updateCalls != 1 || application.update.ProductID != 43 ||
		application.update.MemberRef != "spm_0000000000000000000001" || application.update.ExpectedVersion != 2 {
		t.Fatalf("status/calls/command=%d/%d/%+v body=%s", response.Code, application.updateCalls, application.update, response.Body.String())
	}
}

func TestMemberGridExternalShareRouteUsesProductsWriteAndCSRF(t *testing.T) {
	service := &legacyAuthStub{}
	legacy, err := NewHandler(service, &legacyCustomerStub{result: legacyCustomerResult()})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	legacy.memberGridExternalShare = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization, ok := authport.AuthorizationFromContext(request.Context())
		if !ok || authorization.Capability != authport.CapabilityProductsWrite || authorization.Scope != authport.ScopeGlobal {
			t.Fatalf("authorization=%+v ok=%v", authorization, ok)
		}
		calls++
		writer.WriteHeader(http.StatusNoContent)
	})
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.NotFoundHandler(),
		authHandler,
		authHandler,
		legacy,
	)
	if err != nil {
		t.Fatal(err)
	}
	path := membergrid.RoutePrefix + "/42/member-grid/share-settings"

	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodPut, path, nil))
	if anonymous.Code != http.StatusUnauthorized || calls != 0 {
		t.Fatalf("anonymous status/calls=%d/%d", anonymous.Code, calls)
	}

	missingCSRF := httptest.NewRecorder()
	router.ServeHTTP(missingCSRF, legacyRequest(http.MethodPut, path, legacyToken(221)))
	if missingCSRF.Code != http.StatusForbidden || calls != 0 {
		t.Fatalf("missing CSRF status/calls=%d/%d", missingCSRF.Code, calls)
	}

	request := legacyRequest(http.MethodPut, path, legacyToken(221))
	request.Header.Set("X-CSRF-Token", legacyToken(222))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("authorized status/calls=%d/%d body=%s", response.Code, calls, response.Body.String())
	}
}
