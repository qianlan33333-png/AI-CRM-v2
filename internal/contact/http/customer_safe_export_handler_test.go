package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

type customerSafeExportApplicationStub struct {
	command                 contactapp.CustomerSafeExportCreate
	export                  contactapp.CustomerSafeExport
	rows                    []contactapp.CustomerSafeExportRow
	getErr                  error
	downloadErr             error
	getActor, downloadActor int64
}

func (stub *customerSafeExportApplicationStub) Create(_ context.Context, command contactapp.CustomerSafeExportCreate) (contactapp.CustomerSafeExport, error) {
	stub.command = command
	return contactapp.CustomerSafeExport{ID: "cse_0123456789abcdef0123456789abcdef", Watermark: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)}, nil
}
func (stub *customerSafeExportApplicationStub) Get(_ context.Context, _ string, actor int64, _ *int64) (contactapp.CustomerSafeExport, error) {
	stub.getActor = actor
	return stub.export, stub.getErr
}
func (stub *customerSafeExportApplicationStub) Download(_ context.Context, _ string, actor int64, _ *int64) (contactapp.CustomerSafeExport, []contactapp.CustomerSafeExportRow, error) {
	stub.downloadActor = actor
	return stub.export, stub.rows, stub.downloadErr
}

func TestCustomerSafeExportDownloadHTTPHeadersAndErrors(t *testing.T) {
	owner := int64(42)
	application := &customerSafeExportApplicationStub{export: contactapp.CustomerSafeExport{ID: "cse_0123456789abcdef0123456789abcdef", Watermark: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)}, rows: []contactapp.CustomerSafeExportRow{{CustomerID: 7, DisplayName: "Doe, \"A\"\n\t=formula"}}}
	handler, err := NewCustomerSafeExportHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &owner}, "safe-export")
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: owner})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.Download(response, httptest.NewRequest(http.MethodGet, "/api/v1/customer-exports/cse_0123456789abcdef0123456789abcdef/download", nil).WithContext(ctx), "cse_0123456789abcdef0123456789abcdef")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/csv; charset=utf-8" || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Contains(response.Body.Bytes(), []byte("\"Doe, \"\"A\"\"\n\t=formula\"")) {
		t.Fatalf("status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	application.downloadErr = contactapp.ErrCustomerSafeExportNotFound
	response = httptest.NewRecorder()
	handler.Download(response, httptest.NewRequest(http.MethodGet, "/api/v1/customer-exports/cse_0123456789abcdef0123456789abcdef/download", nil).WithContext(ctx), "cse_0123456789abcdef0123456789abcdef")
	if response.Code != http.StatusNotFound || bytes.Contains(response.Body.Bytes(), []byte("customer_id")) {
		t.Fatalf("cross actor error status=%d body=%q", response.Code, response.Body.String())
	}
	badCtx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, "safe-export")
	badCtx, err = authport.WithAuthorization(badCtx, authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.Get(response, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(badCtx), "cse_0123456789abcdef0123456789abcdef")
	if response.Code != http.StatusForbidden {
		t.Fatalf("wrong capability status=%d", response.Code)
	}
}

func TestCustomerSafeExportHandlerScopesSalesAndUsesReceiptKey(t *testing.T) {
	owner := int64(42)
	application := &customerSafeExportApplicationStub{}
	handler, err := NewCustomerSafeExportHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &owner}, "safe-export")
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: owner})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/customer-exports", bytes.NewBufferString(`{"owner_staff_id":42,"keyword":"customer"}`)).WithContext(ctx)
	request.Header.Set("Idempotency-Key", "customer-safe-export-http-key-0001")
	response := httptest.NewRecorder()
	handler.Create(response, request, generated.CreateCustomerSafeExportParams{IdempotencyKey: "customer-safe-export-http-key-0001"})
	if response.Code != http.StatusCreated || application.command.OwnerScopeStaffID == nil || *application.command.OwnerScopeStaffID != owner || application.command.Filter.OwnerStaffID == nil || *application.command.Filter.OwnerStaffID != owner {
		t.Fatalf("status=%d command=%+v body=%s", response.Code, application.command, response.Body.String())
	}
}

func TestCustomerSafeExportCSVFormulaSafety(t *testing.T) {
	for _, testCase := range []struct{ in, want string }{{"=x", "'=x"}, {"+x", "'+x"}, {"-x", "'-x"}, {"@x", "'@x"}, {"\t=tab", "'\t=tab"}, {"\r+cr", "'\r+cr"}, {"\n-at", "'\n-at"}, {"  -space", "'  -space"}, {"normal", "normal"}} {
		if got := csvSafe(testCase.in); got != testCase.want {
			t.Fatalf("csvSafe(%q)=%q want=%q", testCase.in, got, testCase.want)
		}
	}
}
