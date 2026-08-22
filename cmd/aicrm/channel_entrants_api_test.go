package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contacthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/http"
)

type channelEntrantsRouteSpy struct {
	calls int
	input contactapp.ChannelEntrantsInput
}

func (spy *channelEntrantsRouteSpy) List(
	_ context.Context,
	input contactapp.ChannelEntrantsInput,
) (contactapp.ChannelEntrantsResponse, error) {
	spy.calls++
	spy.input = input
	return contactapp.ChannelEntrantsResponse{
		ChannelID:       input.ChannelID,
		Items:           []contactapp.ChannelEntrantItem{},
		Limit:           input.Limit,
		LocalProjection: true,
	}, nil
}

func TestChannelEntrantsRouteUsesLegacyAuthenticationAndCustomersRead(t *testing.T) {
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
	application := &channelEntrantsRouteSpy{}
	leaf, err := contacthttp.NewChannelEntrantsHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	legacy.channelEntrants, err = contacthttp.NewChannelEntrantsRouteFragment(leaf)
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

	path := contacthttp.ChannelEntrantsRoutePrefix + "/42/contacts?limit=20"
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, path, nil))
	if unauthenticated.Code != http.StatusUnauthorized || application.calls != 0 {
		t.Fatalf("unauthenticated status/calls=%d/%d body=%s", unauthenticated.Code, application.calls, unauthenticated.Body.String())
	}

	authenticated := httptest.NewRecorder()
	router.ServeHTTP(authenticated, legacyRequest(http.MethodGet, path, legacyToken(231)))
	if authenticated.Code != http.StatusOK || application.calls != 1 ||
		application.input.ChannelID != 42 || application.input.Limit != 20 {
		t.Fatalf("authenticated status/calls/input=%d/%d/%+v body=%s", authenticated.Code, application.calls, application.input, authenticated.Body.String())
	}
	if authenticated.Header().Get("Cache-Control") != "private, no-store" ||
		authenticated.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers=%v", authenticated.Header())
	}
}
