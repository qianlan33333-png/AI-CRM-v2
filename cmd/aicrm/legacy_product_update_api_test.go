package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const legacyProductUpdateTestKey = "legacy-product-update-key-0001"

type legacyProductUpdateStub struct {
	mu            sync.Mutex
	updateCalls   int
	getCalls      int
	externalCalls int
	commands      []productport.UpdateCommand
	update        func(productport.UpdateCommand) (productport.Product, error)
	get           func(productport.ID) (productport.Product, error)
}

func (stub *legacyProductUpdateStub) ListLegacy(context.Context, int32, int32) (productport.LegacyPage, error) {
	return productport.LegacyPage{}, nil
}

func (stub *legacyProductUpdateStub) Create(context.Context, productport.CreateCommand) (productport.Product, error) {
	return productport.Product{}, productapp.ErrUnavailable
}

func (stub *legacyProductUpdateStub) Update(_ context.Context, command productport.UpdateCommand) (productport.Product, error) {
	stub.mu.Lock()
	stub.updateCalls++
	stub.commands = append(stub.commands, command)
	update := stub.update
	stub.mu.Unlock()
	if update == nil {
		return productport.Product{}, productapp.ErrUnavailable
	}
	return update(command)
}

func (stub *legacyProductUpdateStub) Get(_ context.Context, id productport.ID) (productport.Product, error) {
	stub.mu.Lock()
	stub.getCalls++
	get := stub.get
	stub.mu.Unlock()
	if get == nil {
		return productport.Product{}, productapp.ErrUnavailable
	}
	return get(id)
}

func (stub *legacyProductUpdateStub) counts() (int, int, int) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.updateCalls, stub.getCalls, stub.externalCalls
}

func (stub *legacyProductUpdateStub) lastCommand() productport.UpdateCommand {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.commands) == 0 {
		return productport.UpdateCommand{}
	}
	return stub.commands[len(stub.commands)-1]
}

func TestLegacyProductUpdateSuccessAndSafeReadback(t *testing.T) {
	base := legacyProductUpdateBaseProduct(t)
	var readback productport.Product
	stub := &legacyProductUpdateStub{}
	stub.update = func(command productport.UpdateCommand) (productport.Product, error) {
		readback = legacyProductUpdateResult(base, command)
		return readback, nil
	}
	stub.get = func(id productport.ID) (productport.Product, error) {
		if id != base.ID {
			t.Fatalf("readback id=%d, want %d", id, base.ID)
		}
		return readback, nil
	}
	response := serveLegacyProductUpdate(t, stub, authport.RoleAdmin, authport.Authorization{
		Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal,
	}, legacyProductUpdateTestKey, legacyProductUpdateBody(7, "更新后的商品", "本地说明", 12800, "CNY", 19))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	assertLegacyProductUpdateSecurityHeaders(t, response)
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["source_status"] != "v2_product_catalog" || payload["fallback_used"] != false || payload["real_external_call_executed"] != false {
		t.Fatalf("envelope=%#v", payload)
	}
	product, ok := payload["product"].(map[string]any)
	if !ok {
		t.Fatalf("product=%#v", payload["product"])
	}
	if product["id"] != float64(base.ID) || product["version"] != float64(8) || product["product_code"] != base.ProductCode ||
		product["name"] != "更新后的商品" || product["title"] != "更新后的商品" || product["description"] != "本地说明" ||
		product["price_cents"] != float64(12800) || product["amount_total"] != float64(12800) || product["currency"] != "CNY" ||
		product["stock_quantity"] != float64(19) || !reflect.DeepEqual(product["images"], []any{"https://local.invalid/product.png"}) {
		t.Fatalf("product=%#v", product)
	}
	command := stub.lastCommand()
	if command.ID != base.ID || command.ExpectedVersion != 7 || command.Name != "更新后的商品" || command.Description != "本地说明" ||
		command.PriceMinor != 12800 || command.Currency != "CNY" || command.StockQuantity != 19 || command.Actor != 41 || command.IdempotencyKey != legacyProductUpdateTestKey {
		t.Fatalf("command=%+v", command)
	}
	if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 1 || getCalls != 1 || externalCalls != 0 {
		t.Fatalf("calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
	}
}

func TestLegacyProductUpdateReplayAndSameKeyPayloadConflict(t *testing.T) {
	base := legacyProductUpdateBaseProduct(t)
	var mu sync.Mutex
	var receiptPayload string
	var receipt productport.Product
	stub := &legacyProductUpdateStub{}
	stub.update = func(command productport.UpdateCommand) (productport.Product, error) {
		payload := fmt.Sprintf("%d|%d|%s|%s|%d|%s|%d", command.ID, command.ExpectedVersion, command.Name, command.Description, command.PriceMinor, command.Currency, command.StockQuantity)
		mu.Lock()
		defer mu.Unlock()
		if receiptPayload == "" {
			receiptPayload = payload
			receipt = legacyProductUpdateResult(base, command)
			return receipt, nil
		}
		if receiptPayload != payload {
			return productport.Product{}, productapp.ErrConflict
		}
		return receipt, nil
	}
	stub.get = func(productport.ID) (productport.Product, error) {
		mu.Lock()
		defer mu.Unlock()
		return receipt, nil
	}
	body := legacyProductUpdateBody(7, "可重放更新", "说明", 9900, "CNY", 2)
	first := serveLegacyProductUpdate(t, stub, authport.RoleOps, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey, body)
	second := serveLegacyProductUpdate(t, stub, authport.RoleOps, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey, body)
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() {
		t.Fatalf("replay status/body=%d/%d\nfirst=%s\nsecond=%s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	conflict := serveLegacyProductUpdate(t, stub, authport.RoleOps, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey,
		legacyProductUpdateBody(7, "同键不同载荷", "说明", 9900, "CNY", 2))
	assertLegacyProductUpdateError(t, conflict, http.StatusConflict, "CONFLICT")
	if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 3 || getCalls != 2 || externalCalls != 0 {
		t.Fatalf("calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
	}
}

func TestLegacyProductUpdateMapsDomainErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "cas conflict", err: productapp.ErrConflict, wantStatus: http.StatusConflict, wantCode: "CONFLICT"},
		{name: "not found", err: productapp.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "invalid command", err: productapp.ErrInvalidProduct, wantStatus: http.StatusUnprocessableEntity, wantCode: "VALIDATION_FAILED"},
		{name: "dependency unavailable", err: productapp.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "DEPENDENCY_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyProductUpdateStub{update: func(productport.UpdateCommand) (productport.Product, error) {
				return productport.Product{}, test.err
			}}
			response := serveLegacyProductUpdate(t, stub, authport.RoleAdmin, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey,
				legacyProductUpdateBody(7, "更新", "说明", 1, "CNY", 0))
			assertLegacyProductUpdateError(t, response, test.wantStatus, test.wantCode)
			if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 1 || getCalls != 0 || externalCalls != 0 {
				t.Fatalf("calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
			}
		})
	}
}

func TestLegacyProductUpdateRejectsClosedJSONViolations(t *testing.T) {
	valid := legacyProductUpdateBody(7, "更新", "说明", 1, "CNY", 0)
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown", body: strings.TrimSuffix(valid, "}") + `,"unknown":true}`},
		{name: "duplicate", body: `{"expected_version":7,"name":"更新","na\u006de":"重复","description":"说明","price_minor":1,"currency":"CNY","stock_quantity":0}`},
		{name: "missing", body: `{"expected_version":7,"name":"更新","description":"说明","price_minor":1,"currency":"CNY"}`},
		{name: "null", body: `{"expected_version":7,"name":"更新","description":"说明","price_minor":1,"currency":"CNY","stock_quantity":null}`},
		{name: "wrong type", body: `{"expected_version":"7","name":"更新","description":"说明","price_minor":1,"currency":"CNY","stock_quantity":0}`},
		{name: "array root", body: `[]`},
		{name: "extra body", body: valid + `{}`},
		{name: "malformed", body: `{"expected_version":7`},
		{name: "empty", body: ``},
		{name: "product code immutable", body: strings.TrimSuffix(valid, "}") + `,"product_code":"forbidden"}`},
		{name: "images immutable", body: strings.TrimSuffix(valid, "}") + `,"images":[]}`},
		{name: "projection opaque", body: strings.TrimSuffix(valid, "}") + `,"legacy_admin_projection":{}}`},
		{name: "legacy status", body: strings.TrimSuffix(valid, "}") + `,"status":"active"}`},
		{name: "provider token", body: strings.TrimSuffix(valid, "}") + `,"provider_token":"secret"}`},
		{name: "payment payload", body: strings.TrimSuffix(valid, "}") + `,"payment_payload":{}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyProductUpdateStub{}
			response := serveLegacyProductUpdate(t, stub, authport.RoleAdmin, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey, test.body)
			assertLegacyProductUpdateError(t, response, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
			if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 0 || getCalls != 0 || externalCalls != 0 {
				t.Fatalf("calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
			}
		})
	}
}

func TestLegacyProductUpdateRejectsMediaBodyPathAndIdempotencyBoundaries(t *testing.T) {
	validBody := legacyProductUpdateBody(7, "更新", "说明", 1, "CNY", 0)
	baseAuthorization := authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}
	for _, test := range []struct {
		name        string
		path        string
		body        []byte
		contentType []string
		keys        []string
	}{
		{name: "missing content type", path: "/api/admin/wechat-pay/products/81", body: []byte(validBody), keys: []string{legacyProductUpdateTestKey}},
		{name: "wrong content type", path: "/api/admin/wechat-pay/products/81", body: []byte(validBody), contentType: []string{"text/plain"}, keys: []string{legacyProductUpdateTestKey}},
		{name: "duplicate content type", path: "/api/admin/wechat-pay/products/81", body: []byte(validBody), contentType: []string{"application/json", "application/json"}, keys: []string{legacyProductUpdateTestKey}},
		{name: "body over limit", path: "/api/admin/wechat-pay/products/81", body: []byte(legacyProductUpdateBody(7, "更新", strings.Repeat("x", 70<<10), 1, "CNY", 0)), contentType: []string{"application/json"}, keys: []string{legacyProductUpdateTestKey}},
		{name: "invalid utf8", path: "/api/admin/wechat-pay/products/81", body: append([]byte(`{"expected_version":7,"name":"`), append([]byte{0xff}, []byte(`","description":"说明","price_minor":1,"currency":"CNY","stock_quantity":0}`)...)...), contentType: []string{"application/json"}, keys: []string{legacyProductUpdateTestKey}},
		{name: "missing idempotency", path: "/api/admin/wechat-pay/products/81", body: []byte(validBody), contentType: []string{"application/json"}},
		{name: "duplicate idempotency", path: "/api/admin/wechat-pay/products/81", body: []byte(validBody), contentType: []string{"application/json"}, keys: []string{legacyProductUpdateTestKey, legacyProductUpdateTestKey}},
		{name: "short idempotency", path: "/api/admin/wechat-pay/products/81", body: []byte(validBody), contentType: []string{"application/json"}, keys: []string{"too-short"}},
		{name: "spaced idempotency", path: "/api/admin/wechat-pay/products/81", body: []byte(validBody), contentType: []string{"application/json"}, keys: []string{" " + legacyProductUpdateTestKey}},
		{name: "long idempotency", path: "/api/admin/wechat-pay/products/81", body: []byte(validBody), contentType: []string{"application/json"}, keys: []string{strings.Repeat("k", 129)}},
		{name: "zero id", path: "/api/admin/wechat-pay/products/0", body: []byte(validBody), contentType: []string{"application/json"}, keys: []string{legacyProductUpdateTestKey}},
		{name: "malformed id", path: "/api/admin/wechat-pay/products/not-an-id", body: []byte(validBody), contentType: []string{"application/json"}, keys: []string{legacyProductUpdateTestKey}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyProductUpdateStub{}
			request := httptest.NewRequest(http.MethodPut, test.path, strings.NewReader(string(test.body)))
			for _, value := range test.contentType {
				request.Header.Add("Content-Type", value)
			}
			for _, value := range test.keys {
				request.Header.Add("Idempotency-Key", value)
			}
			request = request.WithContext(legacyProductUpdateAuthorizedContext(t, request.Context(), authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, baseAuthorization))
			response := httptest.NewRecorder()
			legacyProductUpdateRouter(stub).ServeHTTP(response, request)
			assertLegacyProductUpdateError(t, response, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
			if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 0 || getCalls != 0 || externalCalls != 0 {
				t.Fatalf("calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
			}
		})
	}
}

func TestLegacyProductUpdateRequiresAdminOrOpsProductsWriteGlobalScope(t *testing.T) {
	base := legacyProductUpdateBaseProduct(t)
	body := legacyProductUpdateBody(7, "授权更新", "说明", 1, "CNY", 0)
	for _, test := range []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
		wantStatus    int
		wantCode      string
		allowed       bool
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHENTICATED"},
		{name: "missing authorization", principal: &authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, wantStatus: http.StatusForbidden, wantCode: "UNAUTHORIZED"},
		{name: "sales", principal: &authport.Principal{AdminUserID: 41, Role: authport.RoleSales}, authorization: &authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, wantStatus: http.StatusForbidden, wantCode: "UNAUTHORIZED"},
		{name: "invalid actor", principal: &authport.Principal{AdminUserID: 0, Role: authport.RoleAdmin}, authorization: &authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, wantStatus: http.StatusForbidden, wantCode: "UNAUTHORIZED"},
		{name: "read capability", principal: &authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, authorization: &authport.Authorization{Capability: authport.CapabilityProductsRead, Scope: authport.ScopeGlobal}, wantStatus: http.StatusForbidden, wantCode: "UNAUTHORIZED"},
		{name: "owner scope", principal: &authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, authorization: &authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 9}, wantStatus: http.StatusForbidden, wantCode: "UNAUTHORIZED"},
		{name: "admin", principal: &authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, authorization: &authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, wantStatus: http.StatusOK, allowed: true},
		{name: "ops", principal: &authport.Principal{AdminUserID: 41, Role: authport.RoleOps}, authorization: &authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, wantStatus: http.StatusOK, allowed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var current productport.Product
			stub := &legacyProductUpdateStub{}
			stub.update = func(command productport.UpdateCommand) (productport.Product, error) {
				current = legacyProductUpdateResult(base, command)
				return current, nil
			}
			stub.get = func(productport.ID) (productport.Product, error) { return current, nil }
			request := httptest.NewRequest(http.MethodPut, "/api/admin/wechat-pay/products/81", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", legacyProductUpdateTestKey)
			ctx := request.Context()
			if test.principal != nil {
				ctx = authport.WithAuthenticatedSession(ctx, *test.principal, authport.SessionRef("unit-test-session"))
			}
			if test.authorization != nil {
				var err error
				ctx, err = authport.WithAuthorization(ctx, *test.authorization)
				if err != nil {
					t.Fatal(err)
				}
			}
			request = request.WithContext(ctx)
			response := httptest.NewRecorder()
			legacyProductUpdateRouter(stub).ServeHTTP(response, request)
			if test.allowed {
				if response.Code != test.wantStatus {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			} else {
				assertLegacyProductUpdateError(t, response, test.wantStatus, test.wantCode)
			}
			updateCalls, getCalls, externalCalls := stub.counts()
			if test.allowed && (updateCalls != 1 || getCalls != 1) || !test.allowed && (updateCalls != 0 || getCalls != 0) || externalCalls != 0 {
				t.Fatalf("allowed=%v calls update/get/external=%d/%d/%d", test.allowed, updateCalls, getCalls, externalCalls)
			}
		})
	}
}

func TestLegacyProductUpdateReadbackMismatchFails503WithoutRetry(t *testing.T) {
	base := legacyProductUpdateBaseProduct(t)
	projection, err := productapp.CanonicalLegacyAdminProjection(json.RawMessage(`{"schema_version":1,"status":"active"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*productport.Product)
	}{
		{name: "id", mutate: func(product *productport.Product) { product.ID++ }},
		{name: "version", mutate: func(product *productport.Product) { product.Version++ }},
		{name: "product code", mutate: func(product *productport.Product) { product.ProductCode = "changed" }},
		{name: "images", mutate: func(product *productport.Product) { product.Images = []string{"https://local.invalid/changed.png"} }},
		{name: "legacy projection", mutate: func(product *productport.Product) { product.LegacyAdminProjection = projection }},
		{name: "name", mutate: func(product *productport.Product) { product.Name += " changed" }},
		{name: "description", mutate: func(product *productport.Product) { product.Description += " changed" }},
		{name: "price", mutate: func(product *productport.Product) { product.PriceMinor++ }},
		{name: "currency", mutate: func(product *productport.Product) { product.Currency = "USD" }},
		{name: "stock", mutate: func(product *productport.Product) { product.StockQuantity++ }},
		{name: "created by", mutate: func(product *productport.Product) { product.CreatedBy++ }},
		{name: "created at", mutate: func(product *productport.Product) { product.CreatedAt = product.CreatedAt.Add(time.Second) }},
		{name: "updated at", mutate: func(product *productport.Product) { product.UpdatedAt = product.UpdatedAt.Add(time.Second) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			updated := legacyProductUpdateResult(base, productport.UpdateCommand{ID: base.ID, ExpectedVersion: 7, Name: "更新", Description: "说明", PriceMinor: 1, Currency: "CNY", StockQuantity: 0, Actor: 41, IdempotencyKey: legacyProductUpdateTestKey})
			readback := cloneLegacyProduct(updated)
			test.mutate(&readback)
			stub := &legacyProductUpdateStub{
				update: func(productport.UpdateCommand) (productport.Product, error) { return updated, nil },
				get:    func(productport.ID) (productport.Product, error) { return readback, nil },
			}
			response := serveLegacyProductUpdate(t, stub, authport.RoleAdmin, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey,
				legacyProductUpdateBody(7, "更新", "说明", 1, "CNY", 0))
			assertLegacyProductUpdateError(t, response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
			if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 1 || getCalls != 1 || externalCalls != 0 {
				t.Fatalf("calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
			}
		})
	}

	stub := &legacyProductUpdateStub{
		update: func(command productport.UpdateCommand) (productport.Product, error) {
			return legacyProductUpdateResult(base, command), nil
		},
		get: func(productport.ID) (productport.Product, error) {
			return productport.Product{}, errors.New("readback failed")
		},
	}
	response := serveLegacyProductUpdate(t, stub, authport.RoleAdmin, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey,
		legacyProductUpdateBody(7, "更新", "说明", 1, "CNY", 0))
	assertLegacyProductUpdateError(t, response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
	if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 1 || getCalls != 1 || externalCalls != 0 {
		t.Fatalf("readback error calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
	}
}

func TestLegacyProductUpdateUnsafeApplicationResultFails503(t *testing.T) {
	base := legacyProductUpdateBaseProduct(t)
	for _, test := range []struct {
		name   string
		mutate func(*productport.Product)
	}{
		{name: "version jump", mutate: func(product *productport.Product) { product.Version++ }},
		{name: "invalid id", mutate: func(product *productport.Product) { product.ID++ }},
		{name: "missing creator", mutate: func(product *productport.Product) { product.CreatedBy = 0 }},
		{name: "zero created at", mutate: func(product *productport.Product) { product.CreatedAt = time.Time{} }},
		{name: "updated before created", mutate: func(product *productport.Product) { product.UpdatedAt = product.CreatedAt.Add(-time.Second) }},
		{name: "invalid legacy mapping", mutate: func(product *productport.Product) { product.LegacyAdminProjection = json.RawMessage(`{`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := legacyProductUpdateResult(base, productport.UpdateCommand{
				ID: base.ID, ExpectedVersion: 7, Name: "更新", Description: "说明",
				PriceMinor: 1, Currency: "CNY", StockQuantity: 0,
			})
			test.mutate(&result)
			stub := &legacyProductUpdateStub{
				update: func(productport.UpdateCommand) (productport.Product, error) { return result, nil },
				get:    func(productport.ID) (productport.Product, error) { return cloneLegacyProduct(result), nil },
			}
			response := serveLegacyProductUpdate(t, stub, authport.RoleAdmin, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey,
				legacyProductUpdateBody(7, "更新", "说明", 1, "CNY", 0))
			assertLegacyProductUpdateError(t, response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
			if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 1 || getCalls != 1 || externalCalls != 0 {
				t.Fatalf("calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
			}
		})
	}
}

func TestLegacyProductUpdateMissingDependencyFailsClosed(t *testing.T) {
	for _, products := range []legacyProductApplication{nil, (*legacyProductUpdateStub)(nil)} {
		request := httptest.NewRequest(http.MethodPut, "/api/admin/wechat-pay/products/81", strings.NewReader(legacyProductUpdateBody(7, "更新", "说明", 1, "CNY", 0)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", legacyProductUpdateTestKey)
		request = request.WithContext(legacyProductUpdateAuthorizedContext(t, request.Context(), authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}))
		response := httptest.NewRecorder()
		legacyProductUpdateRouter(products).ServeHTTP(response, request)
		assertLegacyProductUpdateError(t, response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
	}
}

func TestLegacyProductUpdateResponseExcludesRawOpaqueSensitiveAndExecutionFields(t *testing.T) {
	base := legacyProductUpdateBaseProduct(t)
	var current productport.Product
	stub := &legacyProductUpdateStub{}
	stub.update = func(command productport.UpdateCommand) (productport.Product, error) {
		current = legacyProductUpdateResult(base, command)
		return current, nil
	}
	stub.get = func(productport.ID) (productport.Product, error) { return current, nil }
	response := serveLegacyProductUpdate(t, stub, authport.RoleAdmin, authport.Authorization{Capability: authport.CapabilityProductsWrite, Scope: authport.ScopeGlobal}, legacyProductUpdateTestKey,
		legacyProductUpdateBody(7, "更新", "说明", 1, "CNY", 0))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	raw := response.Body.String()
	for _, forbidden := range []string{
		`"legacy_admin_projection"`, `"schema_version"`, `"expected_version"`, `"idempotency_key"`,
		`"provider_token"`, `"payment_payload"`, `"receipt"`, `"actor"`, `"created_by"`,
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("response leaked %s: %s", forbidden, raw)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"real_external_call_executed", "payment_request_executed", "real_wechat_pay_executed", "real_alipay_executed", "provider_signature_verified", "real_refund_executed"} {
		if payload[key] != false {
			t.Fatalf("%s=%#v", key, payload[key])
		}
	}
	if updateCalls, getCalls, externalCalls := stub.counts(); updateCalls != 1 || getCalls != 1 || externalCalls != 0 {
		t.Fatalf("calls update/get/external=%d/%d/%d", updateCalls, getCalls, externalCalls)
	}
}

// The exact baseline's existing legacy product test stub predates Update. This
// method keeps the old list/get/create tests compiling after the minimal
// legacyProductApplication extension.
func (stub *legacyProductStub) Update(_ context.Context, command productport.UpdateCommand) (productport.Product, error) {
	if stub.err != nil {
		return productport.Product{}, stub.err
	}
	result := cloneLegacyProduct(stub.product)
	result.ID = command.ID
	result.Version = command.ExpectedVersion + 1
	result.Name = strings.TrimSpace(command.Name)
	result.Description = strings.TrimSpace(command.Description)
	result.PriceMinor = command.PriceMinor
	result.Currency = strings.ToUpper(strings.TrimSpace(command.Currency))
	result.StockQuantity = command.StockQuantity
	return result, nil
}

func legacyProductUpdateBaseProduct(t *testing.T) productport.Product {
	t.Helper()
	projection, err := productapp.CanonicalLegacyAdminProjection(json.RawMessage(`{"schema_version":1,"status":"draft","enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	return productport.Product{
		ID: 81, ProductCode: "sku-local-81", Name: "原商品", Description: "原说明", PriceMinor: 8800,
		Currency: "CNY", StockQuantity: 11, Images: []string{"https://local.invalid/product.png"}, CreatedBy: 7,
		CreatedAt: created, UpdatedAt: created.Add(time.Minute), Version: 7, LegacyAdminProjection: projection,
	}
}

func legacyProductUpdateResult(base productport.Product, command productport.UpdateCommand) productport.Product {
	result := cloneLegacyProduct(base)
	result.ID = command.ID
	result.Name = strings.TrimSpace(command.Name)
	result.Description = strings.TrimSpace(command.Description)
	result.PriceMinor = command.PriceMinor
	result.Currency = strings.ToUpper(strings.TrimSpace(command.Currency))
	result.StockQuantity = command.StockQuantity
	result.Version = command.ExpectedVersion + 1
	result.UpdatedAt = base.UpdatedAt.Add(time.Second)
	return result
}

func cloneLegacyProduct(product productport.Product) productport.Product {
	product.Images = append([]string(nil), product.Images...)
	product.LegacyAdminProjection = append(json.RawMessage(nil), product.LegacyAdminProjection...)
	return product
}

func legacyProductUpdateBody(expectedVersion int64, name, description string, priceMinor int64, currency string, stockQuantity int32) string {
	payload, _ := json.Marshal(map[string]any{
		"expected_version": expectedVersion,
		"name":             name,
		"description":      description,
		"price_minor":      priceMinor,
		"currency":         currency,
		"stock_quantity":   stockQuantity,
	})
	return string(payload)
}

func serveLegacyProductUpdate(t *testing.T, stub *legacyProductUpdateStub, role authport.Role, authorization authport.Authorization, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/wechat-pay/products/81", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	request = request.WithContext(legacyProductUpdateAuthorizedContext(t, request.Context(), authport.Principal{AdminUserID: 41, Role: role}, authorization))
	response := httptest.NewRecorder()
	legacyProductUpdateRouter(stub).ServeHTTP(response, request)
	return response
}

func legacyProductUpdateAuthorizedContext(t *testing.T, ctx context.Context, principal authport.Principal, authorization authport.Authorization) context.Context {
	t.Helper()
	ctx = authport.WithAuthenticatedSession(ctx, principal, authport.SessionRef("unit-test-session"))
	ctx, err := authport.WithAuthorization(ctx, authorization)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func legacyProductUpdateRouter(products legacyProductApplication) http.Handler {
	router := chi.NewRouter()
	handler := &Handler{products: products}
	router.Put("/api/admin/wechat-pay/products/{product_id}", handler.UpdateProduct)
	return router
}

func assertLegacyProductUpdateError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status=%d want=%d body=%s", response.Code, wantStatus, response.Body.String())
	}
	assertLegacyProductUpdateSecurityHeaders(t, response)
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != wantCode || payload["message"] == "" || payload["request_id"] == "" {
		t.Fatalf("payload=%#v", payload)
	}
	for _, forbidden := range []string{"ok", "product", "fallback_used", "source_status", "real_external_call_executed", "detail", "error"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("error leaked %s: %#v", forbidden, payload)
		}
	}
}

func assertLegacyProductUpdateSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("security headers cache=%q nosniff=%q content-type=%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"), response.Header().Get("Content-Type"))
	}
}
