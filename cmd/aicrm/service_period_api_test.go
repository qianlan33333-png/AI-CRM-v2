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
	producthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/http/serviceperiod"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type servicePeriodRouteSpy struct {
	listCalls   int
	getCalls    int
	createCalls int
	create      productport.CreateServicePeriodProductCommand
}

func (spy *servicePeriodRouteSpy) ListServicePeriodProducts(context.Context, int32, int32) (productport.ServicePeriodPage, error) {
	spy.listCalls++
	return productport.ServicePeriodPage{OK: true, Items: []productport.ServicePeriodProduct{}, Limit: 50}, nil
}

func (spy *servicePeriodRouteSpy) GetServicePeriodProduct(_ context.Context, id productport.ID) (productport.ServicePeriodProduct, error) {
	spy.getCalls++
	return productport.ServicePeriodProduct{ServiceProductID: id, ProductCode: "period-local", Name: "周期商品", Currency: "CNY", Lifecycle: productport.ServicePeriodEnabled, Enabled: true, Version: 1}, nil
}

func (spy *servicePeriodRouteSpy) CreateServicePeriodProduct(_ context.Context, command productport.CreateServicePeriodProductCommand) (productport.ServicePeriodProduct, error) {
	spy.createCalls++
	spy.create = command
	return productport.ServicePeriodProduct{}, nil
}

func (*servicePeriodRouteSpy) UpdateServicePeriodProduct(context.Context, productport.UpdateServicePeriodProductCommand) (productport.ServicePeriodProduct, error) {
	return productport.ServicePeriodProduct{}, nil
}

func (*servicePeriodRouteSpy) SetServicePeriodProductEnabled(context.Context, productport.SetServicePeriodProductEnabledCommand) (productport.ServicePeriodProduct, error) {
	return productport.ServicePeriodProduct{}, nil
}

func (*servicePeriodRouteSpy) CopyServicePeriodProduct(context.Context, productport.CopyServicePeriodProductCommand) (productport.ServicePeriodProduct, error) {
	return productport.ServicePeriodProduct{}, nil
}

func (*servicePeriodRouteSpy) ArchiveServicePeriodProduct(context.Context, productport.ArchiveServicePeriodProductCommand) (productport.ServicePeriodProduct, error) {
	return productport.ServicePeriodProduct{}, nil
}

func TestServicePeriodRoutesUseLegacyAuthenticationAuthorizationAndCSRF(t *testing.T) {
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
	application := &servicePeriodRouteSpy{}
	legacy.servicePeriod, err = producthttp.NewHandler(application)
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

	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, producthttp.BasePath, nil))
	if anonymous.Code != http.StatusUnauthorized || application.listCalls != 0 {
		t.Fatalf("anonymous status/calls=%d/%d body=%s", anonymous.Code, application.listCalls, anonymous.Body.String())
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, legacyRequest(http.MethodGet, producthttp.BasePath, legacyToken(211)))
	if list.Code != http.StatusOK || application.listCalls != 1 {
		t.Fatalf("list status/calls=%d/%d body=%s", list.Code, application.listCalls, list.Body.String())
	}

	share := httptest.NewRecorder()
	router.ServeHTTP(share, legacyRequest(http.MethodGet, producthttp.BasePath+"/7/share", legacyToken(215)))
	if share.Code != http.StatusOK || application.getCalls != 1 || !strings.Contains(share.Body.String(), `"public_path":"/p/service_period/7"`) {
		t.Fatalf("share status/calls/body=%d/%d/%s", share.Code, application.getCalls, share.Body.String())
	}

	body := `{"product_code":"period-local","name":"周期商品","description":"","price_minor":16800,"currency":"CNY","stock_quantity":10}`
	missingCSRFRequest := legacyRequest(http.MethodPost, producthttp.BasePath, legacyToken(212))
	missingCSRFRequest.Body = io.NopCloser(strings.NewReader(body))
	missingCSRFRequest.Header.Set("Content-Type", "application/json")
	missingCSRFRequest.Header.Set("Idempotency-Key", "service-period-create-0001")
	missingCSRF := httptest.NewRecorder()
	router.ServeHTTP(missingCSRF, missingCSRFRequest)
	if missingCSRF.Code != http.StatusForbidden || application.createCalls != 0 {
		t.Fatalf("missing csrf status/calls=%d/%d body=%s", missingCSRF.Code, application.createCalls, missingCSRF.Body.String())
	}

	createRequest := legacyRequest(http.MethodPost, producthttp.BasePath, legacyToken(213))
	createRequest.Body = io.NopCloser(strings.NewReader(body))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Idempotency-Key", "service-period-create-0002")
	createRequest.Header.Set("X-CSRF-Token", legacyToken(214))
	create := httptest.NewRecorder()
	router.ServeHTTP(create, createRequest)
	if create.Code != http.StatusCreated || application.createCalls != 1 || application.create.Actor != 1 ||
		application.create.ProductCode != "period-local" || application.create.PriceMinor != 16800 {
		t.Fatalf("create status/calls/command=%d/%d/%+v body=%s", create.Code, application.createCalls, application.create, create.Body.String())
	}
}
