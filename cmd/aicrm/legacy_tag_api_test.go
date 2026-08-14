package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

type legacyTagStub struct {
	catalog contactapp.LegacyTagCatalog
	err     error
	command contactapp.LegacyTagCommand
	writes  int
}

func (s *legacyTagStub) List(context.Context) (contactapp.LegacyTagCatalog, error) {
	return s.catalog, s.err
}
func (s *legacyTagStub) CreateGroup(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, contactapp.LegacyTag, error) {
	s.command = c
	s.writes++
	return s.catalog.Groups[0], s.catalog.Tags[0], s.err
}
func (s *legacyTagStub) UpdateGroup(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error) {
	s.command = c
	s.writes++
	return s.catalog.Groups[0], s.err
}
func (s *legacyTagStub) ArchiveGroup(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error) {
	s.command = c
	s.writes++
	return s.catalog.Groups[0], s.err
}
func (s *legacyTagStub) CreateTag(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	s.command = c
	s.writes++
	return s.catalog.Tags[0], s.err
}
func (s *legacyTagStub) UpdateTag(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	s.command = c
	s.writes++
	return s.catalog.Tags[0], s.err
}
func (s *legacyTagStub) ArchiveTag(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	s.command = c
	s.writes++
	return s.catalog.Tags[0], s.err
}

func TestB02LegacyTagCatalogEnvelopeAndWriteCompatibility(t *testing.T) {
	stub := &legacyTagStub{catalog: legacyTagFixture()}
	router, auth := legacyTagRouter(t, stub)
	read := httptest.NewRecorder()
	router.ServeHTTP(read, legacyRequest(http.MethodGet, "/api/admin/wecom/tags", legacyToken(131)))
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"groups"`) || !strings.Contains(read.Body.String(), `"total_tags":1`) || !strings.Contains(read.Body.String(), `"tag_limit":1000`) {
		t.Fatalf("read=%d %s", read.Code, read.Body.String())
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityCustomersRead {
		t.Fatalf("read capability=%v", got)
	}
	auth.reset()
	create := httptest.NewRecorder()
	req := legacyChannelWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客","actor":{"id":999},"idempotency_key":"body-key","trace_id":"trace-1","dry_run":false}`)
	req.Header.Set("Idempotency-Key", "header-key")
	router.ServeHTTP(create, req)
	if create.Code != 200 || stub.writes != 1 || stub.command.Actor != 1 || stub.command.IdempotencyKey != "header-key" || stub.command.TraceID != "trace-1" {
		t.Fatalf("create=%d writes=%d command=%#v body=%s", create.Code, stub.writes, stub.command, create.Body.String())
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityCustomersWrite {
		t.Fatalf("write capability=%v", got)
	}
	patch := httptest.NewRecorder()
	router.ServeHTTP(patch, legacyChannelWriteRequest(http.MethodPatch, "/api/admin/wecom/tags/2", `{"tag_name":"成交"}`))
	if patch.Code != 200 || stub.command.TagID != 2 {
		t.Fatalf("patch=%d command=%#v body=%s", patch.Code, stub.command, patch.Body.String())
	}
}

func TestB02LegacyTagCatalogDegradesAndCSRFRejects(t *testing.T) {
	stub := &legacyTagStub{catalog: legacyTagFixture(), err: contactapp.ErrLegacyTagUnavailable}
	router, _ := legacyTagRouter(t, stub)
	read := httptest.NewRecorder()
	router.ServeHTTP(read, legacyRequest(http.MethodGet, "/api/admin/wecom/tags", legacyToken(132)))
	body := read.Body.String()
	if read.Code != 200 || !strings.Contains(body, `"error_code":"production_read_unavailable"`) || !strings.Contains(body, `"groups":[]`) || !strings.Contains(body, `"fixture_used":false`) {
		t.Fatalf("degraded=%d %s", read.Code, body)
	}
	stub.err = nil
	bad := legacyRequest(http.MethodPost, "/api/admin/wecom/tags", legacyToken(133))
	bad.Body = io.NopCloser(strings.NewReader(`{"group_id":1,"group_name":"组","tag_name":"标签"}`))
	bad.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, bad)
	if response.Code != http.StatusForbidden || stub.writes != 0 {
		t.Fatalf("csrf=%d writes=%d body=%s", response.Code, stub.writes, response.Body.String())
	}
}

func legacyTagFixture() contactapp.LegacyTagCatalog {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	return contactapp.LegacyTagCatalog{Groups: []contactapp.LegacyTagGroup{{ID: 1, Name: "客户阶段"}}, Tags: []contactapp.LegacyTag{{ID: 2, GroupID: 1, GroupName: "客户阶段", Name: "新客"}}, SyncedAt: now}
}
func legacyTagRouter(t *testing.T, tags legacyTagApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, e := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if e != nil {
		t.Fatal(e)
	}
	legacy.legacyTags = tags
	authHandler, e := authhttp.NewHandler(service)
	if e != nil {
		t.Fatal(e)
	}
	router, e := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if e != nil {
		t.Fatal(e)
	}
	return router, service
}
