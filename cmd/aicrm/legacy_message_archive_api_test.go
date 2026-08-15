package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
)

func TestLegacyMessageArchiveSearchUsesOneIDOwnerScopeAndMasksContent(t *testing.T) {
	owner := int64(7)
	archive := &legacyArchiveServiceStub{records: []wecomapp.ArchiveMessage{{
		ID: "1", SourceMessageID: "message-1", ExternalUserID: "ext-1", ChatType: "private", WithUserID: "staff-7",
		Sender: "external-1", Receiver: "staff-7", MessageType: "text", Content: "请联系13800138000", SentAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
	}}}
	identity := &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 101}}
	detail := &legacyCustomerDetailStub{result: legacyCustomerDetailResult(owner)}
	handler := &Handler{auth: &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &owner}},
		messageArchive: archive, identityResolve: identity, customerDetail: detail, weComCorpID: "corp-fixture"}
	endpoint := legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.SearchArchivedMessages)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/messages/search?external_userid=ext-1&keyword=%E8%81%94%E7%B3%BB", legacyToken(21)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if identity.calls != 1 || identity.ref.Kind != identityport.KindWeComExternalUserID || identity.ref.Scope != "wecom-corp:corp-fixture" ||
		detail.calls != 1 || detail.input.OwnerStaffID == nil || *detail.input.OwnerStaffID != owner || archive.listCalls != 1 || archive.query.CustomerID != 101 || archive.query.Keyword != "联系" {
		t.Fatalf("identity=%#v detail=%#v archive=%#v", identity, detail, archive)
	}
	if strings.Contains(response.Body.String(), "13800138000") || strings.Contains(response.Body.String(), "external-1") || !strings.Contains(response.Body.String(), "[masked-phone]") {
		t.Fatalf("archive response did not redact content/identifier: %s", response.Body.String())
	}
}

func TestLegacyExternalChatRecordsUsesUnionResolverWithoutGuessingScope(t *testing.T) {
	archive := &legacyArchiveServiceStub{records: []wecomapp.ArchiveMessage{{
		ID: "2", SourceMessageID: "message-2", ExternalUserID: "ext-2", ChatType: "private", WithUserID: "staff-7",
		Sender: "ext-2", Receiver: "staff-7", MessageType: "text", Content: "已脱敏内容", SentAt: time.Unix(100, 0).UTC(),
	}}, total: 2}
	union := &legacyArchiveUnionStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 101}}
	detail := &legacyCustomerDetailStub{result: legacyCustomerDetailResult(7)}
	handler := &Handler{auth: &legacyAuthStub{}, messageArchive: archive, messageArchiveUnionID: union,
		identityResolve: &legacyIdentityStub{}, customerDetail: detail}
	endpoint := legacyRoute(t, handler, authport.CapabilityMessageArchiveExternalRead, handler.ListExternalChatRecords)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/external/chat-records?unionid=union-sensitive&start_time=100&chat_scene=%E7%A7%81%E4%BF%A1", legacyToken(22)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if union.calls != 1 || union.value != "union-sensitive" || archive.query.CustomerID != 101 || archive.query.ChatType != "private" || archive.query.WithUserID != "HuangYouCan" || archive.query.Limit != archiveExternalPageLimit {
		t.Fatalf("union=%#v archive=%#v", union, archive)
	}
	var payload struct {
		OK             bool   `json:"ok"`
		ExternalUserID string `json:"external_userid"`
		NextCursor     string `json:"next_cursor"`
		Items          []struct {
			UnionID string `json:"unionid"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || !payload.OK || payload.ExternalUserID != "ext-2" || payload.NextCursor == "" || len(payload.Items) != 1 || payload.Items[0].UnionID != "" {
		t.Fatalf("payload=%#v err=%v", payload, err)
	}
}

func TestLegacyArchiveSyncAcceptsCommandButNeverClaimsProviderExecution(t *testing.T) {
	archive := &legacyArchiveServiceStub{receipt: wecomapp.ArchiveSyncReceipt{ID: 9, State: wecomapp.ArchiveSyncAccepted, EventID: 31}}
	handler := &Handler{auth: &legacyAuthStub{}, messageArchive: archive}
	endpoint := legacyRoute(t, handler, authport.CapabilityMessageArchiveExecute, handler.RequestArchiveSync)
	request := legacyRequest(http.MethodPost, "/api/archive/sync", legacyToken(23))
	request.Header.Set("Idempotency-Key", "archive-sync-http-key-0001")
	request.Body = ioNopCloser(`{"start_time":"2026-08-01 00:00:00","limit":50,"max_pages":2}`)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || archive.syncCalls != 1 || archive.command.Actor != "admin:1" || archive.command.Limit != 50 || archive.command.MaxPages != 2 {
		t.Fatalf("status=%d archive=%#v body=%s", response.Code, archive, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"side_effect_executed":false`) || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) || strings.Contains(response.Body.String(), "queued") {
		t.Fatalf("sync response claimed unsupported execution: %s", response.Body.String())
	}
}

func TestLegacyArchiveBoundaryAndDeprecatedRoutesFailClosed(t *testing.T) {
	archive := &legacyArchiveServiceStub{}
	handler := &Handler{auth: &legacyAuthStub{}, messageArchive: archive, identityResolve: &legacyIdentityStub{}, customerDetail: &legacyCustomerDetailStub{}}
	search := legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.SearchArchivedMessages)
	response := httptest.NewRecorder()
	search.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/messages/search?external_userid=ext-1&keyword=x&limit=201", legacyToken(24)))
	if response.Code != http.StatusBadRequest || archive.listCalls != 0 {
		t.Fatalf("boundary status=%d calls=%d body=%s", response.Code, archive.listCalls, response.Body.String())
	}
	deprecated := legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.DeprecatedMessageArchive)
	response = httptest.NewRecorder()
	deprecated.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/messages/archive", legacyToken(25)))
	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), `"replacement_route":"/api/messages/search"`) {
		t.Fatalf("deprecated status=%d body=%s", response.Code, response.Body.String())
	}
}

type legacyArchiveServiceStub struct {
	health               wecomapp.ArchiveHealth
	records              []wecomapp.ArchiveMessage
	total                int64
	receipt              wecomapp.ArchiveSyncReceipt
	err                  error
	query                wecomapp.ArchiveQuery
	command              wecomapp.ArchiveSyncCommand
	listCalls, syncCalls int
}

func (stub *legacyArchiveServiceStub) Health(context.Context) (wecomapp.ArchiveHealth, error) {
	return stub.health, stub.err
}

func (stub *legacyArchiveServiceStub) List(_ context.Context, query wecomapp.ArchiveQuery) ([]wecomapp.ArchiveMessage, int64, error) {
	stub.listCalls++
	stub.query = query
	total := stub.total
	if total == 0 {
		total = int64(len(stub.records))
	}
	return append([]wecomapp.ArchiveMessage(nil), stub.records...), total, stub.err
}

func (stub *legacyArchiveServiceStub) RequestSync(_ context.Context, command wecomapp.ArchiveSyncCommand) (wecomapp.ArchiveSyncReceipt, error) {
	stub.syncCalls++
	stub.command = command
	return stub.receipt, stub.err
}

type legacyArchiveUnionStub struct {
	result identityport.ResolveResult
	err    error
	value  string
	calls  int
}

func (stub *legacyArchiveUnionStub) ResolveUnionID(_ context.Context, value string) (identityport.ResolveResult, error) {
	stub.calls++
	stub.value = value
	return stub.result, stub.err
}

func ioNopCloser(value string) io.ReadCloser { return io.NopCloser(strings.NewReader(value)) }

var _ legacyMessageArchiveApplication = (*legacyArchiveServiceStub)(nil)
var _ legacyMessageArchiveUnionResolver = (*legacyArchiveUnionStub)(nil)
