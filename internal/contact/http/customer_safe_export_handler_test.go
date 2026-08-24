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
	command contactapp.CustomerSafeExportCreate
}

func (stub *customerSafeExportApplicationStub) Create(_ context.Context, command contactapp.CustomerSafeExportCreate) (contactapp.CustomerSafeExport, error) {
	stub.command = command
	return contactapp.CustomerSafeExport{ID: "cse_0123456789abcdef0123456789abcdef", Watermark: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)}, nil
}
func (*customerSafeExportApplicationStub) Get(context.Context, string, int64) (contactapp.CustomerSafeExport, error) {
	return contactapp.CustomerSafeExport{}, nil
}
func (*customerSafeExportApplicationStub) Download(context.Context, string, int64, *int64) (contactapp.CustomerSafeExport, []contactapp.CustomerSafeExportRow, error) {
	return contactapp.CustomerSafeExport{}, nil, nil
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
	for _, testCase := range []struct{ in, want string }{{"=x", "'=x"}, {"+x", "'+x"}, {"-x", "'-x"}, {"@x", "'@x"}, {"normal", "normal"}} {
		if got := csvSafe(testCase.in); got != testCase.want {
			t.Fatalf("csvSafe(%q)=%q want=%q", testCase.in, got, testCase.want)
		}
	}
}
