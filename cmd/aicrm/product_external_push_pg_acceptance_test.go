package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	producthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/http"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

var commerce87DatabaseURL = flag.String("p4-commerce87-database-url", "", "isolated PostgreSQL 16.14 Commerce87 database URL")

func TestCommerceExternalPushCanonicalPG16(t *testing.T) {
	if *commerce87DatabaseURL == "" {
		t.Skip("-p4-commerce87-database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*commerce87DatabaseURL, acceptancefixtures.CommerceExternalPushDatabaseName); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *commerce87DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	queries := productdb.New(pool)
	version, err := queries.CommerceExternalPushAcceptanceServerVersion(ctx)
	if err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q error=%v", version, err)
	}

	uow := platformstore.NewUnitOfWork(pool)
	repository := productstore.NewCatalogRepository()
	productService := productapp.NewService(uow, repository, eventstore.NewAppender())
	code := fmt.Sprintf("commerce87-http-%d", time.Now().UnixNano())
	product, err := productService.Create(ctx, productport.CreateCommand{
		ProductCode: code, Name: "Commerce87 local push", Description: "local only", PriceMinor: 1,
		Currency: "CNY", StockQuantity: 0, Images: []string{}, LegacyAdminProjection: productapp.DefaultLegacyAdminProjection(),
		Actor: 8701, IdempotencyKey: code + "-create",
	})
	if err != nil {
		t.Fatal(err)
	}
	externalEffectsRepository := eerstore.NewRepository(pool, uow)
	externalEffectsRuntime, err := eer.NewService(externalEffectsRepository)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := producthttp.NewExternalPushHandler(
		productapp.NewCommerceExternalPushService(uow, repository, productstore.NewCommerceExternalPushEERAccepter(externalEffectsRuntime)),
		productExternalPushAuthorizer{}, productExternalPushCSRF{},
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{productExternalPush: leaf}
	allowedAuth := &commerce87Auth{}
	router := commerce87CanonicalRouter(t, candidate, allowedAuth)
	path := fmt.Sprintf("/api/admin/wechat-pay/products/%d/external-push", product.ID)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, commerce87Request(http.MethodPut, path, `{"enabled":true,"configuration_reference":"commerce87-local"}`, "commerce87-save-key", false, false))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}
	missingCSRF := httptest.NewRecorder()
	router.ServeHTTP(missingCSRF, commerce87Request(http.MethodPut, path, `{"enabled":true,"configuration_reference":"commerce87-local"}`, "commerce87-save-key", true, false))
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status=%d body=%s", missingCSRF.Code, missingCSRF.Body.String())
	}
	denied := httptest.NewRecorder()
	commerce87CanonicalRouter(t, candidate, &commerce87Auth{authorizeErr: authport.ErrUnauthorized}).ServeHTTP(
		denied, commerce87Request(http.MethodPut, path, `{"enabled":true,"configuration_reference":"commerce87-local"}`, "commerce87-save-key", true, true),
	)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("rbac status=%d body=%s", denied.Code, denied.Body.String())
	}
	assertCommerce87Facts(t, ctx, queries, product.ID, 0, 0, 0, 0, 0)

	saveBody := `{"enabled":true,"configuration_reference":"commerce87-local"}`
	firstSave := commerce87Serve(router, commerce87Request(http.MethodPut, path, saveBody, "commerce87-save-key", true, true))
	replayedSave := commerce87Serve(router, commerce87Request(http.MethodPut, path, saveBody, "commerce87-save-key", true, true))
	if firstSave.Code != http.StatusOK || replayedSave.Code != http.StatusOK || firstSave.Body.String() != replayedSave.Body.String() {
		t.Fatalf("save status=%d/%d exact_replay=%t first=%s second=%s", firstSave.Code, replayedSave.Code, firstSave.Body.String() == replayedSave.Body.String(), firstSave.Body.String(), replayedSave.Body.String())
	}
	changedSave := commerce87Serve(router, commerce87Request(http.MethodPut, path, `{"enabled":true,"configuration_reference":"commerce87-changed"}`, "commerce87-save-key", true, true))
	if changedSave.Code != http.StatusConflict {
		t.Fatalf("changed save status=%d body=%s", changedSave.Code, changedSave.Body.String())
	}

	testPath := path + "/test"
	firstTest := commerce87Serve(router, commerce87Request(http.MethodPost, testPath, "", "commerce87-test-key", true, true))
	replayedTest := commerce87Serve(router, commerce87Request(http.MethodPost, testPath, "", "commerce87-test-key", true, true))
	if firstTest.Code != http.StatusAccepted || replayedTest.Code != http.StatusAccepted || firstTest.Body.String() != replayedTest.Body.String() {
		t.Fatalf("test status=%d/%d exact_replay=%t first=%s second=%s", firstTest.Code, replayedTest.Code, firstTest.Body.String() == replayedTest.Body.String(), firstTest.Body.String(), replayedTest.Body.String())
	}
	var accepted map[string]any
	if err = json.Unmarshal(firstTest.Body.Bytes(), &accepted); err != nil || accepted["state"] != "accepted" || accepted["provider_accepted"] != false || accepted["delivery_proven"] != false || accepted["real_external_call_executed"] != false || accepted["auto_retry_allowed"] != false {
		t.Fatalf("accepted=%v error=%v", accepted, err)
	}
	differentKey := commerce87Serve(router, commerce87Request(http.MethodPost, testPath, "", "commerce87-test-other-key", true, true))
	if differentKey.Code != http.StatusConflict {
		t.Fatalf("same immutable config with different key status=%d body=%s", differentKey.Code, differentKey.Body.String())
	}
	facts := assertCommerce87Facts(t, ctx, queries, product.ID, 1, 2, 1, 1, 1)
	if facts.Attempts != 0 || facts.RiverBoundEffects != 0 || facts.ProviderOrDelivery != 0 {
		t.Fatalf("attempts/river/provider_or_delivery=%d/%d/%d", facts.Attempts, facts.RiverBoundEffects, facts.ProviderOrDelivery)
	}
	if got := allowedAuth.capabilitySnapshot(); len(got) < 5 || got[len(got)-1] != authport.CapabilityProductsWrite {
		t.Fatalf("canonical capabilities=%v", got)
	}
}

func commerce87CanonicalRouter(t *testing.T, candidate *candidateHandler, service authport.Service) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func commerce87Request(method, path, body, key string, session, csrf bool) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if session {
		request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "commerce87-session"})
	}
	if csrf {
		request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
	}
	return request
}

func commerce87Serve(router http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertCommerce87Facts(t *testing.T, ctx context.Context, queries *productdb.Queries, productID productport.ID, configurations, productReceipts, effects, effectReceipts, bindings int) productdb.ReadCommerceExternalPushAcceptanceFactsRow {
	t.Helper()
	facts, err := queries.ReadCommerceExternalPushAcceptanceFacts(ctx, int64(productID))
	if err != nil || facts.Configurations != int64(configurations) || facts.ProductReceipts != int64(productReceipts) || facts.Effects != int64(effects) || facts.EffectReceipts != int64(effectReceipts) || facts.Bindings != int64(bindings) {
		t.Fatalf("facts config/product_receipts/effects/effect_receipts/bindings=%d/%d/%d/%d/%d want=%d/%d/%d/%d/%d error=%v", facts.Configurations, facts.ProductReceipts, facts.Effects, facts.EffectReceipts, facts.Bindings, configurations, productReceipts, effects, effectReceipts, bindings, err)
	}
	return facts
}

type commerce87Auth struct {
	mu           sync.Mutex
	authorizeErr error
	capabilities []authport.Capability
}

func (*commerce87Auth) Authenticate(_ context.Context, session authport.SessionRef) (authport.Principal, error) {
	if session != "commerce87-session" {
		return authport.Principal{}, authport.ErrUnauthenticated
	}
	return authport.Principal{AdminUserID: 8701, Role: authport.RoleAdmin}, nil
}

func (service *commerce87Auth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	service.mu.Lock()
	service.capabilities = append(service.capabilities, capability)
	service.mu.Unlock()
	if service.authorizeErr != nil {
		return authport.Authorization{}, service.authorizeErr
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (*commerce87Auth) ValidateCSRF(_ context.Context, session authport.SessionRef, csrf authport.CSRFToken) error {
	if session != "commerce87-session" || csrf != authport.CSRFToken(strings.Repeat("A", 43)) {
		return authport.ErrCSRFInvalid
	}
	return nil
}

func (*commerce87Auth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (service *commerce87Auth) capabilitySnapshot() []authport.Capability {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]authport.Capability(nil), service.capabilities...)
}
