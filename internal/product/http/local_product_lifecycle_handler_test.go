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

func TestLocalProductLifecycleHandlerMapsClosedCommandsAndDTOs(t *testing.T) {
	application := &localProductLifecycleHTTPStub{product: localLifecycleHTTPProduct(), share: productport.LocalProductShare{
		ProductID: 8, ProductCode: "wechat-8", Lifecycle: productport.LocalProductEnabled, Available: false, Reason: productapp.LocalProductShareUnavailableReason,
	}}
	handler, err := NewLocalProductLifecycleHandler(application)
	if err != nil {
		t.Fatal(err)
	}

	enable := httptest.NewRecorder()
	handler.SetLocalProductEnabled(enable, localRequest(t, http.MethodPost, `{"expected_version":1}`, authport.CapabilityProductsWrite, true, true), 8, true)
	if enable.Code != http.StatusOK || application.enabledCommand.ID != 8 || !application.enabledCommand.Enabled || application.enabledCommand.ExpectedVersion != 1 {
		t.Fatalf("enable status/command=%d/%+v body=%s", enable.Code, application.enabledCommand, enable.Body.String())
	}
	var enabledBody map[string]any
	if err := json.Unmarshal(enable.Body.Bytes(), &enabledBody); err != nil || enabledBody["lifecycle"] != "enabled" || enabledBody["enabled"] != true || enabledBody["legacy_admin_projection"] != nil {
		t.Fatalf("enable body=%#v err=%v", enabledBody, err)
	}

	copyResponse := httptest.NewRecorder()
	handler.CopyLocalProduct(copyResponse, localRequest(t, http.MethodPost, `{"expected_version":2}`, authport.CapabilityProductsWrite, true, true), 8)
	if copyResponse.Code != http.StatusCreated || application.copyCommand.ID != 8 || application.copyCommand.ExpectedVersion != 2 {
		t.Fatalf("copy status/command=%d/%+v body=%s", copyResponse.Code, application.copyCommand, copyResponse.Body.String())
	}

	deleteResponse := httptest.NewRecorder()
	handler.DeleteLocalProduct(deleteResponse, localRequest(t, http.MethodDelete, `{"expected_version":3}`, authport.CapabilityProductsWrite, true, true), 8)
	if deleteResponse.Code != http.StatusOK || application.deleteCommand.ID != 8 || !application.deleteResult.Deleted {
		t.Fatalf("delete status/command=%d/%+v result=%+v body=%s", deleteResponse.Code, application.deleteCommand, application.deleteResult, deleteResponse.Body.String())
	}

	shareResponse := httptest.NewRecorder()
	handler.ShareLocalProduct(shareResponse, localRequest(t, http.MethodGet, "", authport.CapabilityProductsRead, false, false), 8)
	if shareResponse.Code != http.StatusOK || application.shareID != 8 {
		t.Fatalf("share status/id=%d/%d body=%s", shareResponse.Code, application.shareID, shareResponse.Body.String())
	}
	var shareBody map[string]any
	if err := json.Unmarshal(shareResponse.Body.Bytes(), &shareBody); err != nil || shareBody["ok"] != true || shareBody["available"] != false || shareBody["reason"] != productapp.LocalProductShareUnavailableReason || shareBody["purchase_url"] != nil || shareBody["qr_code_url"] != nil {
		t.Fatalf("share body=%#v err=%v", shareBody, err)
	}
}

func TestLocalProductLifecycleHandlerFailsClosedForAuthAndRequestBoundaries(t *testing.T) {
	application := &localProductLifecycleHTTPStub{product: localLifecycleHTTPProduct(), share: productport.LocalProductShare{ProductID: 8, ProductCode: "wechat-8", Lifecycle: productport.LocalProductDraft, Reason: productapp.LocalProductShareUnavailableReason}}
	handler, _ := NewLocalProductLifecycleHandler(application)
	valid := `{"expected_version":1}`
	tests := []struct {
		name       string
		request    *http.Request
		call       func(http.ResponseWriter, *http.Request)
		wantStatus int
	}{
		{name: "wrong capability", request: localRequest(t, http.MethodPost, valid, authport.CapabilityProductsRead, true, true), call: func(w http.ResponseWriter, r *http.Request) { handler.SetLocalProductEnabled(w, r, 8, true) }, wantStatus: http.StatusForbidden},
		{name: "missing csrf context", request: localRequest(t, http.MethodPost, valid, authport.CapabilityProductsWrite, false, true), call: func(w http.ResponseWriter, r *http.Request) { handler.SetLocalProductEnabled(w, r, 8, true) }, wantStatus: http.StatusOK},
		{name: "missing idempotency", request: localRequest(t, http.MethodPost, valid, authport.CapabilityProductsWrite, true, false), call: func(w http.ResponseWriter, r *http.Request) { handler.SetLocalProductEnabled(w, r, 8, true) }, wantStatus: http.StatusBadRequest},
		{name: "unknown field", request: localRequest(t, http.MethodPost, `{"expected_version":1,"provider_token":"forbidden"}`, authport.CapabilityProductsWrite, true, true), call: func(w http.ResponseWriter, r *http.Request) { handler.CopyLocalProduct(w, r, 8) }, wantStatus: http.StatusBadRequest},
		{name: "zero version", request: localRequest(t, http.MethodPost, `{"expected_version":0}`, authport.CapabilityProductsWrite, true, true), call: func(w http.ResponseWriter, r *http.Request) { handler.DeleteLocalProduct(w, r, 8) }, wantStatus: http.StatusBadRequest},
		{name: "share query", request: localRequest(t, http.MethodGet, "", authport.CapabilityProductsRead, false, false), call: func(w http.ResponseWriter, r *http.Request) {
			r.URL.RawQuery = "ready=true"
			handler.ShareLocalProduct(w, r, 8)
		}, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			test.call(response, test.request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}

	application.err = productapp.ErrLocalProductDeleteNotAllowed
	conflict := httptest.NewRecorder()
	handler.DeleteLocalProduct(conflict, localRequest(t, http.MethodDelete, valid, authport.CapabilityProductsWrite, true, true), 8)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("delete conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestLocalProductLifecycleHandlerRejectsInvalidApplicationSnapshots(t *testing.T) {
	application := &localProductLifecycleHTTPStub{product: localLifecycleHTTPProduct()}
	handler, _ := NewLocalProductLifecycleHandler(application)
	for name, mutate := range map[string]func(*productport.LocalProduct){
		"lifecycle":               func(product *productport.LocalProduct) { product.Lifecycle = "unknown" },
		"product_code_whitespace": func(product *productport.LocalProduct) { product.ProductCode = " invalid" },
		"currency_lowercase":      func(product *productport.LocalProduct) { product.Currency = "cny" },
		"image_whitespace":        func(product *productport.LocalProduct) { product.Images = []string{" invalid"} },
	} {
		t.Run(name, func(t *testing.T) {
			product := localLifecycleHTTPProduct()
			mutate(&product)
			if _, err := localProductLifecycleProductResponse(product); !errors.Is(err, productapp.ErrUnavailable) {
				t.Fatalf("invalid product error=%v", err)
			}
		})
	}

	application.share = productport.LocalProductShare{ProductID: 8, ProductCode: "wechat-8", Lifecycle: productport.LocalProductDraft, Available: false}
	response := httptest.NewRecorder()
	handler.ShareLocalProduct(response, localRequest(t, http.MethodGet, "", authport.CapabilityProductsRead, false, false), 8)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("invalid share status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestNewLocalProductLifecycleHandlerRejectsTypedNil(t *testing.T) {
	var application *localProductLifecycleHTTPStub
	if handler, err := NewLocalProductLifecycleHandler(application); handler != nil || !errors.Is(err, productapp.ErrUnavailable) {
		t.Fatalf("handler=%v err=%v", handler, err)
	}
}

type localProductLifecycleHTTPStub struct {
	product        productport.LocalProduct
	share          productport.LocalProductShare
	err            error
	enabledCommand productport.SetLocalProductEnabledCommand
	copyCommand    productport.CopyLocalProductCommand
	deleteCommand  productport.DeleteLocalProductCommand
	deleteResult   productport.DeleteLocalProductResult
	shareID        productport.ID
}

func (stub *localProductLifecycleHTTPStub) SetLocalProductEnabled(_ context.Context, command productport.SetLocalProductEnabledCommand) (productport.LocalProduct, error) {
	stub.enabledCommand = command
	if stub.err != nil {
		return productport.LocalProduct{}, stub.err
	}
	result := stub.product
	result.Version = command.ExpectedVersion + 1
	result.Enabled = command.Enabled
	if command.Enabled {
		result.Lifecycle = productport.LocalProductEnabled
	} else {
		result.Lifecycle = productport.LocalProductDisabled
	}
	return result, nil
}

func (stub *localProductLifecycleHTTPStub) CopyLocalProduct(_ context.Context, command productport.CopyLocalProductCommand) (productport.LocalProduct, error) {
	stub.copyCommand = command
	if stub.err != nil {
		return productport.LocalProduct{}, stub.err
	}
	result := stub.product
	result.ID = command.ID + 100
	result.ProductCode += "-copy"
	result.Version = 1
	result.Lifecycle = productport.LocalProductDraft
	result.Enabled = false
	return result, nil
}

func (stub *localProductLifecycleHTTPStub) DeleteLocalProduct(_ context.Context, command productport.DeleteLocalProductCommand) (productport.DeleteLocalProductResult, error) {
	stub.deleteCommand = command
	if stub.err != nil {
		return productport.DeleteLocalProductResult{}, stub.err
	}
	stub.deleteResult = productport.DeleteLocalProductResult{ProductID: command.ID, Deleted: true}
	return stub.deleteResult, nil
}

func (stub *localProductLifecycleHTTPStub) ShareLocalProduct(_ context.Context, id productport.ID) (productport.LocalProductShare, error) {
	stub.shareID = id
	if stub.err != nil {
		return productport.LocalProductShare{}, stub.err
	}
	return stub.share, nil
}

func localLifecycleHTTPProduct() productport.LocalProduct {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	return productport.LocalProduct{ID: 8, ProductCode: "wechat-8", Name: "商品", Description: "本地", PriceMinor: 9900, Currency: "CNY", StockQuantity: 2, Images: []string{"https://local.invalid/a.png"}, CreatedBy: 7, CreatedAt: now, UpdatedAt: now, Lifecycle: productport.LocalProductDraft, Enabled: false, Version: 1}
}

func TestLocalProductLifecycleHandlerClosedDTOHasNoProviderFields(t *testing.T) {
	response, err := localProductLifecycleProductResponse(localLifecycleHTTPProduct())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil || strings.Contains(string(encoded), "provider") || strings.Contains(string(encoded), "payment") {
		t.Fatalf("response=%s err=%v", encoded, err)
	}
}
