package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type localProductsStub struct {
	product productport.Product
	err     error
	command productport.UpdateCommand
}

func (stub *localProductsStub) Update(_ context.Context, command productport.UpdateCommand) (productport.Product, error) {
	stub.command = command
	return stub.product, stub.err
}

type localEntitlementsStub struct {
	items         []productport.LocalEntitlement
	item          productport.LocalEntitlement
	err           error
	listProductID productport.ID
	listLimit     int32
	grant         productport.GrantLocalEntitlementCommand
	revoke        productport.RevokeLocalEntitlementCommand
}

func (stub *localEntitlementsStub) List(_ context.Context, productID productport.ID, limit int32) ([]productport.LocalEntitlement, error) {
	stub.listProductID, stub.listLimit = productID, limit
	return stub.items, stub.err
}
func (stub *localEntitlementsStub) Get(_ context.Context, _ productport.EntitlementID) (productport.LocalEntitlement, error) {
	return stub.item, stub.err
}
func (stub *localEntitlementsStub) Grant(_ context.Context, command productport.GrantLocalEntitlementCommand) (productport.LocalEntitlement, error) {
	stub.grant = command
	return stub.item, stub.err
}
func (stub *localEntitlementsStub) Revoke(_ context.Context, command productport.RevokeLocalEntitlementCommand) (productport.LocalEntitlement, error) {
	stub.revoke = command
	return stub.item, stub.err
}

func TestLocalMutationHandlerMapsFrozenCommandsAndClosedDTOs(t *testing.T) {
	products := &localProductsStub{product: localHTTPProduct()}
	entitlements := &localEntitlementsStub{item: localHTTPEntitlement("active")}
	handler, err := NewLocalMutationHandler(products, entitlements)
	if err != nil {
		t.Fatal(err)
	}

	update := httptest.NewRecorder()
	handler.UpdateProduct(update, localRequest(t, http.MethodPut, `{"expected_version":1,"name":"更新","description":"说明","price_minor":2,"currency":"CNY","stock_quantity":3}`, authport.CapabilityProductsWrite, true, true), 8)
	if update.Code != http.StatusOK || products.command != (productport.UpdateCommand{ID: 8, ExpectedVersion: 1, Name: "更新", Description: "说明", PriceMinor: 2, Currency: "CNY", StockQuantity: 3, Actor: 7, IdempotencyKey: "local-operation-key-7"}) {
		t.Fatalf("update status=%d command=%+v body=%s", update.Code, products.command, update.Body.String())
	}
	var product map[string]any
	if err = json.Unmarshal(update.Body.Bytes(), &product); err != nil || len(product) != 12 || product["version"] != float64(2) || product["legacy_admin_projection"] != nil {
		t.Fatalf("product response=%v err=%v", product, err)
	}

	grant := httptest.NewRecorder()
	handler.GrantProductLocalEntitlement(grant, localRequest(t, http.MethodPost, `{"order_id":44}`, authport.CapabilityEntitlementsWrite, true, true), 8)
	if grant.Code != http.StatusCreated || entitlements.grant != (productport.GrantLocalEntitlementCommand{ProductID: 8, OrderID: 44, Actor: 7, IdempotencyKey: "local-operation-key-7"}) {
		t.Fatalf("grant status=%d command=%+v body=%s", grant.Code, entitlements.grant, grant.Body.String())
	}
	var entitlement map[string]any
	if err = json.Unmarshal(grant.Body.Bytes(), &entitlement); err != nil || len(entitlement) != 8 || entitlement["granted_by"] != nil || entitlement["revoked_by"] != nil {
		t.Fatalf("entitlement response=%v err=%v", entitlement, err)
	}

	revoke := httptest.NewRecorder()
	handler.RevokeProductLocalEntitlement(revoke, localRequest(t, http.MethodPost, `{"expected_version":1}`, authport.CapabilityEntitlementsWrite, true, true), 19)
	if revoke.Code != http.StatusOK || entitlements.revoke != (productport.RevokeLocalEntitlementCommand{ID: 19, ExpectedVersion: 1, Actor: 7, IdempotencyKey: "local-operation-key-7"}) {
		t.Fatalf("revoke status=%d command=%+v body=%s", revoke.Code, entitlements.revoke, revoke.Body.String())
	}
}

func TestLocalMutationHandlerFailsClosedForAuthorizationBodyAndEntitlementErrors(t *testing.T) {
	products := &localProductsStub{product: localHTTPProduct()}
	entitlements := &localEntitlementsStub{item: localHTTPEntitlement("active")}
	handler, _ := NewLocalMutationHandler(products, entitlements)

	for _, test := range []struct {
		name       string
		request    *http.Request
		call       func(http.ResponseWriter, *http.Request)
		wantStatus int
	}{
		{
			name: "wrong capability", request: localRequest(t, http.MethodPut, `{"expected_version":1,"name":"更新","description":"说明","price_minor":2,"currency":"CNY","stock_quantity":3}`, authport.CapabilityProductsRead, false, true),
			call: func(w http.ResponseWriter, r *http.Request) { handler.UpdateProduct(w, r, 8) }, wantStatus: http.StatusForbidden,
		},
		{
			name: "unknown grant field", request: localRequest(t, http.MethodPost, `{"order_id":44,"customer_id":9}`, authport.CapabilityEntitlementsWrite, true, true),
			call: func(w http.ResponseWriter, r *http.Request) { handler.GrantProductLocalEntitlement(w, r, 8) }, wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing idempotency", request: localRequest(t, http.MethodPost, `{"expected_version":1}`, authport.CapabilityEntitlementsWrite, true, false),
			call: func(w http.ResponseWriter, r *http.Request) { handler.RevokeProductLocalEntitlement(w, r, 19) }, wantStatus: http.StatusBadRequest,
		},
		{
			name: "non JSON mutation", request: func() *http.Request {
				r := localRequest(t, http.MethodPost, `{"order_id":44}`, authport.CapabilityEntitlementsWrite, true, true)
				r.Header.Set("Content-Type", "text/plain")
				return r
			}(),
			call: func(w http.ResponseWriter, r *http.Request) { handler.GrantProductLocalEntitlement(w, r, 8) }, wantStatus: http.StatusBadRequest,
		},
		{
			name: "trimmed idempotency only", request: func() *http.Request {
				r := localRequest(t, http.MethodPost, `{"order_id":44}`, authport.CapabilityEntitlementsWrite, true, true)
				r.Header.Set("Idempotency-Key", " local-operation-key-7")
				return r
			}(),
			call: func(w http.ResponseWriter, r *http.Request) { handler.GrantProductLocalEntitlement(w, r, 8) }, wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			test.call(response, test.request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}

	entitlements.err = productapp.ErrEntitlementNotFound
	missing := httptest.NewRecorder()
	handler.GetProductLocalEntitlement(missing, localRequest(t, http.MethodGet, "", authport.CapabilityEntitlementsRead, false, false), 404)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing entitlement status=%d body=%s", missing.Code, missing.Body.String())
	}
	entitlements.err = productapp.ErrConflict
	conflict := httptest.NewRecorder()
	handler.GrantProductLocalEntitlement(conflict, localRequest(t, http.MethodPost, `{"order_id":44}`, authport.CapabilityEntitlementsWrite, true, true), 8)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("duplicate order status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestLocalMutationHandlerListHasExactItemsEnvelope(t *testing.T) {
	entitlement := localHTTPEntitlement("revoked")
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	entitlement.RevokedAt = &now
	entitlements := &localEntitlementsStub{items: []productport.LocalEntitlement{entitlement}}
	handler, _ := NewLocalMutationHandler(&localProductsStub{product: localHTTPProduct()}, entitlements)
	response := httptest.NewRecorder()
	handler.ListProductLocalEntitlements(response, localRequest(t, http.MethodGet, "", authport.CapabilityEntitlementsRead, false, false), 8, 0)
	if response.Code != http.StatusOK || entitlements.listProductID != 8 || entitlements.listLimit != productapp.DefaultLimit {
		t.Fatalf("status=%d product=%d limit=%d body=%s", response.Code, entitlements.listProductID, entitlements.listLimit, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body) != 1 {
		t.Fatalf("list body=%v err=%v", body, err)
	}
}

func TestNewLocalMutationHandlerRejectsTypedNil(t *testing.T) {
	var products *localProductsStub
	if handler, err := NewLocalMutationHandler(products, &localEntitlementsStub{}); handler != nil || !errors.Is(err, productapp.ErrUnavailable) {
		t.Fatalf("handler=%v err=%v", handler, err)
	}
}

func TestLocalMutationResponsesRejectInvalidTimesAndFields(t *testing.T) {
	product := localHTTPProduct()
	product.UpdatedAt = product.CreatedAt.Add(-time.Nanosecond)
	if _, err := localProductResponse(product); !errors.Is(err, productapp.ErrUnavailable) {
		t.Fatalf("product time err=%v", err)
	}
	product = localHTTPProduct()
	product.Currency = "12$"
	if _, err := localProductResponse(product); !errors.Is(err, productapp.ErrUnavailable) {
		t.Fatalf("product currency err=%v", err)
	}
	entitlement := localHTTPEntitlement("revoked")
	revokedAt := entitlement.GrantedAt.Add(-time.Nanosecond)
	entitlement.RevokedAt = &revokedAt
	if _, err := localEntitlementResponse(entitlement); !errors.Is(err, productapp.ErrUnavailable) {
		t.Fatalf("entitlement time err=%v", err)
	}
}

func localRequest(t *testing.T, method, body string, capability authport.Capability, withCSRF, withIdempotency bool) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, "/api/v1/products/8", strings.NewReader(body))
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, "session-7")
	var err error
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	request = request.WithContext(ctx)
	if withCSRF {
		request.Header.Set("X-CSRF-Token", "csrf-7")
	}
	if withIdempotency {
		request.Header.Set("Idempotency-Key", "local-operation-key-7")
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func localHTTPProduct() productport.Product {
	now := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	return productport.Product{ID: 8, ProductCode: "SKU-8", Name: "产品", Description: "说明", PriceMinor: 2, Currency: "CNY", StockQuantity: 3, Images: []string{}, CreatedBy: 7, CreatedAt: now, UpdatedAt: now, Version: 2}
}

func localHTTPEntitlement(state string) productport.LocalEntitlement {
	return productport.LocalEntitlement{ID: 19, ProductID: 8, OrderID: 44, CustomerID: 9, State: state, Version: 1, GrantedAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)}
}
