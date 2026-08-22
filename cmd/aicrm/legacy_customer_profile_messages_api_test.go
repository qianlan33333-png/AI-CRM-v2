package main

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
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestLegacyCustomerProfileMessagesResolvesFrozenHintsAndReturnsOnlySafeMetadata(t *testing.T) {
	sentAt := time.Date(2026, time.August, 21, 10, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name                  string
		rawQuery              string
		wantUnionCalls        int
		wantIdentityCalls     int
		wantUnionValue        string
		wantExternalUserValue string
	}{
		{
			name:           "unionid",
			rawQuery:       "unionid=raw-union-secret",
			wantUnionCalls: 1,
			wantUnionValue: "raw-union-secret",
		},
		{
			name:                  "external userid",
			rawQuery:              "external_userid=raw-external-secret",
			wantIdentityCalls:     1,
			wantExternalUserValue: "raw-external-secret",
		},
		{
			name:                  "consistent dual hints",
			rawQuery:              "unionid=raw-union-secret&external_userid=raw-external-secret",
			wantUnionCalls:        1,
			wantIdentityCalls:     1,
			wantUnionValue:        "raw-union-secret",
			wantExternalUserValue: "raw-external-secret",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail := &legacyCustomerProfileMessagesDetailStub{}
			identity := &legacyCustomerProfileMessagesIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
			union := &legacyCustomerProfileMessagesUnionStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
			archive := &legacyCustomerProfileMessagesArchiveStub{
				unsafeRecords: []wecomapp.ArchiveMessage{{
					SourceMessageID: "raw-provider-id", ExternalUserID: "raw-external-secret", Sender: "raw-sender", Receiver: "raw-receiver", Content: "raw message body",
				}},
				page: wecomport.CustomerChatSummaryPage{
					Items: []wecomport.CustomerChatSummary{
						{ChatType: "private", MessageType: "text", SentAt: sentAt},
						{ChatType: "group", MessageType: "image", SentAt: sentAt.Add(-time.Minute)},
					},
					Total: 2,
					Limit: wecomapp.MessageArchiveDefaultLimit,
				},
			}
			handler := &Handler{
				customerDetail:        detail,
				identityResolve:       identity,
				messageArchiveUnionID: union,
				messageArchive:        archive,
				weComCorpID:           "corp-profile-messages",
			}
			response := serveLegacyCustomerProfileMessages(handler, authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, test.rawQuery))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertLegacyCustomerProfileMessagesSecurityHeaders(t, response)
			if union.calls != test.wantUnionCalls || identity.calls != test.wantIdentityCalls || detail.calls != 1 || archive.summaryCalls != 1 {
				t.Fatalf("calls union/identity/detail/summary=%d/%d/%d/%d", union.calls, identity.calls, detail.calls, archive.summaryCalls)
			}
			if archive.listCalls != 0 || archive.syncCalls != 0 || archive.healthCalls != 0 {
				t.Fatalf("unsafe archive methods called: list=%d sync=%d health=%d", archive.listCalls, archive.syncCalls, archive.healthCalls)
			}
			if detail.input.ID != 44 || archive.query.CustomerID != 44 || archive.query.Limit != wecomapp.MessageArchiveDefaultLimit || archive.query.Offset != 0 || archive.query.ChatType != "" {
				t.Fatalf("detail=%+v archive query=%+v", detail.input, archive.query)
			}
			if union.value != test.wantUnionValue {
				t.Fatalf("union value=%q want=%q", union.value, test.wantUnionValue)
			}
			if test.wantIdentityCalls > 0 {
				wantRef := identityport.IDRef{
					Kind:      identityport.KindWeComExternalUserID,
					Scope:     "wecom-corp:corp-profile-messages",
					Value:     test.wantExternalUserValue,
					Assurance: identityport.AssuranceVerified,
					Source:    "legacy-customer-profile-messages",
				}
				if identity.ref != wantRef {
					t.Fatalf("identity ref=%+v want=%+v", identity.ref, wantRef)
				}
			}
			var payload legacyCustomerProfileMessagesSuccess
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if !payload.OK || payload.Count != 2 || payload.Limit != wecomapp.MessageArchiveDefaultLimit || payload.SourceStatus != "message_archive_read_model" || payload.RouteOwner != archiveRouteOwner || payload.RealExternalCallExecuted || len(payload.Messages) != 2 {
				t.Fatalf("payload=%+v", payload)
			}
			if payload.Messages[0] != (legacyCustomerProfileMessage{ChatType: "private", MessageType: "text", SendTime: sentAt.Format(time.RFC3339)}) ||
				payload.Messages[1] != (legacyCustomerProfileMessage{ChatType: "group", MessageType: "image", SendTime: sentAt.Add(-time.Minute).Format(time.RFC3339)}) {
				t.Fatalf("messages=%+v", payload.Messages)
			}
			assertLegacyCustomerProfileMessagesNoSensitiveFields(t, response.Body.String(),
				"raw-union-secret", "raw-external-secret", "raw message body", "raw-provider-id", "raw-sender", "raw-receiver")
		})
	}
}

func TestLegacyCustomerProfileMessagesDualHintsMustResolveIndependentlyToTheSameCustomer(t *testing.T) {
	for _, test := range []struct {
		name     string
		union    identityport.ResolveResult
		external identityport.ResolveResult
	}{
		{"different customer ids", identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 45}},
		{"union conflict", identityport.ResolveResult{Status: identityport.ResolveConflict}, identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}},
		{"external conflict", identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, identityport.ResolveResult{Status: identityport.ResolveConflict}},
		{"union not found", identityport.ResolveResult{Status: identityport.ResolveNotFound}, identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}},
		{"external not found", identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, identityport.ResolveResult{Status: identityport.ResolveNotFound}},
		{"both not found", identityport.ResolveResult{Status: identityport.ResolveNotFound}, identityport.ResolveResult{Status: identityport.ResolveNotFound}},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail := &legacyCustomerProfileMessagesDetailStub{}
			identity := &legacyCustomerProfileMessagesIdentityStub{result: test.external}
			union := &legacyCustomerProfileMessagesUnionStub{result: test.union}
			archive := &legacyCustomerProfileMessagesArchiveStub{}
			response := serveLegacyCustomerProfileMessages(&Handler{
				customerDetail: detail, identityResolve: identity, messageArchiveUnionID: union,
				messageArchive: archive, weComCorpID: "corp-profile-messages",
			}, authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, "unionid=union-44&external_userid=external-44"))
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"error_code":"identity_hint_conflict"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if union.calls != 1 || identity.calls != 1 || detail.calls != 0 || archive.summaryCalls != 0 || archive.listCalls != 0 || archive.syncCalls != 0 {
				t.Fatalf("calls union/identity/detail/summary/list/sync=%d/%d/%d/%d/%d/%d", union.calls, identity.calls, detail.calls, archive.summaryCalls, archive.listCalls, archive.syncCalls)
			}
			assertLegacyCustomerProfileMessagesSecurityHeaders(t, response)
			assertLegacyCustomerProfileMessagesNoSensitiveFields(t, response.Body.String(), "union-44", "external-44")
		})
	}
}

func TestLegacyCustomerProfileMessagesRejectsUnsupportedAndMalformedQueriesBeforeAnyRead(t *testing.T) {
	for _, test := range []struct {
		name, rawQuery, errorCode string
	}{
		{"missing identity", "", "invalid_identity_hint"},
		{"user id", "user_id=44", "unsupported_identity_hint"},
		{"user id with valid hint", "unionid=union-44&user_id=44", "unsupported_identity_hint"},
		{"empty unionid", "unionid=", "invalid_identity_hint"},
		{"empty external userid", "external_userid=", "invalid_identity_hint"},
		{"duplicate unionid", "unionid=one&unionid=two", "invalid_identity_hint"},
		{"duplicate external userid", "external_userid=one&external_userid=two", "invalid_identity_hint"},
		{"unionid control", "unionid=one%0Atwo", "invalid_identity_hint"},
		{"external userid control", "external_userid=one%09two", "invalid_identity_hint"},
		{"malformed escape", "unionid=%ZZ", "invalid_identity_hint"},
		{"unknown openid", "openid=value", "invalid_identity_hint"},
		{"mobile is not accepted", "mobile=13800138000", "invalid_identity_hint"},
		{"limit is not accepted", "unionid=union-44&limit=30", "invalid_identity_hint"},
		{"fetch all without identity", "fetch_all=true", "invalid_identity_hint"},
		{"empty fetch all", "unionid=union-44&fetch_all=", "invalid_fetch_all"},
		{"numeric fetch all", "unionid=union-44&fetch_all=1", "invalid_fetch_all"},
		{"uppercase fetch all", "unionid=union-44&fetch_all=TRUE", "invalid_fetch_all"},
		{"trimmed fetch all is rejected", "unionid=union-44&fetch_all=%20true", "invalid_fetch_all"},
		{"fetch all control", "unionid=union-44&fetch_all=true%0A", "invalid_fetch_all"},
		{"duplicate fetch all", "unionid=union-44&fetch_all=true&fetch_all=false", "invalid_fetch_all"},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail := &legacyCustomerProfileMessagesDetailStub{}
			identity := &legacyCustomerProfileMessagesIdentityStub{}
			union := &legacyCustomerProfileMessagesUnionStub{}
			archive := &legacyCustomerProfileMessagesArchiveStub{}
			request := authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, test.rawQuery)
			response := serveLegacyCustomerProfileMessages(&Handler{
				customerDetail: detail, identityResolve: identity, messageArchiveUnionID: union,
				messageArchive: archive, weComCorpID: "corp-profile-messages",
			}, request)
			if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"error_code":"`+test.errorCode+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if detail.calls != 0 || identity.calls != 0 || union.calls != 0 || archive.summaryCalls != 0 || archive.listCalls != 0 || archive.syncCalls != 0 {
				t.Fatalf("reads occurred detail/identity/union/summary/list/sync=%d/%d/%d/%d/%d/%d", detail.calls, identity.calls, union.calls, archive.summaryCalls, archive.listCalls, archive.syncCalls)
			}
			assertLegacyCustomerProfileMessagesSecurityHeaders(t, response)
		})
	}

	t.Run("invalid UTF-8 raw query", func(t *testing.T) {
		request := authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, "")
		request.URL.RawQuery = string([]byte{'u', 'n', 'i', 'o', 'n', 'i', 'd', '=', 0xff})
		archive := &legacyCustomerProfileMessagesArchiveStub{}
		response := serveLegacyCustomerProfileMessages(&Handler{customerDetail: &legacyCustomerProfileMessagesDetailStub{}, messageArchive: archive}, request)
		if response.Code != http.StatusUnprocessableEntity || archive.summaryCalls != 0 {
			t.Fatalf("status=%d summary calls=%d body=%s", response.Code, archive.summaryCalls, response.Body.String())
		}
	})
}

func TestLegacyCustomerProfileMessagesFetchAllUsesOnlyExistingArchiveBounds(t *testing.T) {
	for _, test := range []struct {
		name, rawQuery string
		wantLimit      int32
	}{
		{"default first page", "external_userid=external-44", wecomapp.MessageArchiveDefaultLimit},
		{"explicit first page", "external_userid=external-44&fetch_all=false", wecomapp.MessageArchiveDefaultLimit},
		{"bounded fetch all", "external_userid=external-44&fetch_all=true", wecomapp.MessageArchiveMaximumLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			items := make([]wecomport.CustomerChatSummary, test.wantLimit)
			start := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
			for index := range items {
				items[index] = wecomport.CustomerChatSummary{ChatType: "private", MessageType: "text", SentAt: start.Add(-time.Duration(index) * time.Second)}
			}
			archive := &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{
				Items: items, Total: int64(test.wantLimit) + 500, Limit: test.wantLimit,
			}}
			response := serveLegacyCustomerProfileMessages(&Handler{
				customerDetail:  &legacyCustomerProfileMessagesDetailStub{},
				identityResolve: &legacyCustomerProfileMessagesIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}},
				messageArchive:  archive,
				weComCorpID:     "corp-profile-messages",
			}, authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, test.rawQuery))
			if response.Code != http.StatusOK || archive.summaryCalls != 1 || archive.query.Limit != test.wantLimit || archive.query.Offset != 0 || archive.listCalls != 0 || archive.syncCalls != 0 {
				t.Fatalf("status/calls/query=%d/%d/%+v list=%d sync=%d body=%s", response.Code, archive.summaryCalls, archive.query, archive.listCalls, archive.syncCalls, response.Body.String())
			}
			var payload legacyCustomerProfileMessagesSuccess
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.Limit != test.wantLimit || payload.Count != int(test.wantLimit) || len(payload.Messages) != int(test.wantLimit) || payload.RealExternalCallExecuted {
				t.Fatalf("payload=%+v err=%v", payload, err)
			}
		})
	}
}

func TestLegacyCustomerProfileMessagesRequiresAdminGlobalCapability(t *testing.T) {
	validAuthorization := authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}
	for _, test := range []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
	}{
		{"no principal or authorization", nil, nil},
		{"principal without authorization", &authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, nil},
		{"zero admin id", &authport.Principal{Role: authport.RoleAdmin}, &validAuthorization},
		{"ops role", &authport.Principal{AdminUserID: 9, Role: authport.RoleOps}, &validAuthorization},
		{"sales role", &authport.Principal{AdminUserID: 9, Role: authport.RoleSales}, &validAuthorization},
		{"wrong capability", &authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, &authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal}},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail := &legacyCustomerProfileMessagesDetailStub{}
			identity := &legacyCustomerProfileMessagesIdentityStub{}
			union := &legacyCustomerProfileMessagesUnionStub{}
			archive := &legacyCustomerProfileMessagesArchiveStub{}
			request := legacyCustomerProfileMessagesRequest(http.MethodGet, "unionid=union-44", test.principal, test.authorization)
			response := serveLegacyCustomerProfileMessages(&Handler{
				customerDetail: detail, identityResolve: identity, messageArchiveUnionID: union,
				messageArchive: archive, weComCorpID: "corp-profile-messages",
			}, request)
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"error_code":"customer_profile_messages_forbidden"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if detail.calls != 0 || identity.calls != 0 || union.calls != 0 || archive.summaryCalls != 0 || archive.listCalls != 0 || archive.syncCalls != 0 {
				t.Fatalf("unauthorized request reached dependencies")
			}
			assertLegacyCustomerProfileMessagesSecurityHeaders(t, response)
		})
	}
}

func TestLegacyCustomerProfileMessagesReturnsNotFoundWithoutArchiveRead(t *testing.T) {
	for _, test := range []struct {
		name, rawQuery string
		identity       identityport.ResolveResult
		union          identityport.ResolveResult
		detailErr      error
	}{
		{"union missing", "unionid=union-missing", identityport.ResolveResult{}, identityport.ResolveResult{Status: identityport.ResolveNotFound}, nil},
		{"external missing", "external_userid=external-missing", identityport.ResolveResult{Status: identityport.ResolveNotFound}, identityport.ResolveResult{}, nil},
		{"resolved customer projection missing", "external_userid=external-44", identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, identityport.ResolveResult{}, contactapp.ErrCustomerNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail := &legacyCustomerProfileMessagesDetailStub{err: test.detailErr}
			archive := &legacyCustomerProfileMessagesArchiveStub{}
			response := serveLegacyCustomerProfileMessages(&Handler{
				customerDetail:        detail,
				identityResolve:       &legacyCustomerProfileMessagesIdentityStub{result: test.identity},
				messageArchiveUnionID: &legacyCustomerProfileMessagesUnionStub{result: test.union},
				messageArchive:        archive,
				weComCorpID:           "corp-profile-messages",
			}, authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, test.rawQuery))
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"customer_not_found"`) || archive.summaryCalls != 0 || archive.listCalls != 0 || archive.syncCalls != 0 {
				t.Fatalf("status=%d archive=%+v body=%s", response.Code, archive, response.Body.String())
			}
			assertLegacyCustomerProfileMessagesSecurityHeaders(t, response)
		})
	}
}

func TestLegacyCustomerProfileMessagesFailsClosedForDependenciesAndMalformedProjection(t *testing.T) {
	validIdentity := &legacyCustomerProfileMessagesIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
	validDetail := &legacyCustomerProfileMessagesDetailStub{}
	validPage := wecomport.CustomerChatSummaryPage{
		Items: []wecomport.CustomerChatSummary{{ChatType: "private", MessageType: "text", SentAt: time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)}},
		Total: 1, Limit: wecomapp.MessageArchiveDefaultLimit,
	}
	for _, test := range []struct {
		name    string
		handler *Handler
	}{
		{"missing customer detail", &Handler{identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: validPage}, weComCorpID: "corp-profile-messages"}},
		{"missing external resolver", &Handler{customerDetail: validDetail, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: validPage}, weComCorpID: "corp-profile-messages"}},
		{"missing corp scope", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: validPage}}},
		{"external resolver error", &Handler{customerDetail: validDetail, identityResolve: &legacyCustomerProfileMessagesIdentityStub{err: errors.New("identity unavailable")}, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: validPage}, weComCorpID: "corp-profile-messages"}},
		{"malformed found identity", &Handler{customerDetail: validDetail, identityResolve: &legacyCustomerProfileMessagesIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound}}, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: validPage}, weComCorpID: "corp-profile-messages"}},
		{"customer projection dependency error", &Handler{customerDetail: &legacyCustomerProfileMessagesDetailStub{err: contactapp.ErrCustomerDetailUnavailable}, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: validPage}, weComCorpID: "corp-profile-messages"}},
		{"missing archive", &Handler{customerDetail: validDetail, identityResolve: validIdentity, weComCorpID: "corp-profile-messages"}},
		{"archive without safe reader", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveOnlyStub{}, weComCorpID: "corp-profile-messages"}},
		{"safe archive unavailable", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{err: wecomport.ErrCustomerChatSummaryUnavailable}, weComCorpID: "corp-profile-messages"}},
		{"page limit mismatch", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Limit: 19}}, weComCorpID: "corp-profile-messages"}},
		{"page offset mismatch", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Limit: 20, Offset: 1}}, weComCorpID: "corp-profile-messages"}},
		{"negative total", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Limit: 20, Total: -1}}, weComCorpID: "corp-profile-messages"}},
		{"total smaller than page", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Limit: 20, Items: validPage.Items}}, weComCorpID: "corp-profile-messages"}},
		{"invalid chat type", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Limit: 20, Total: 1, Items: []wecomport.CustomerChatSummary{{ChatType: "room", MessageType: "text", SentAt: validPage.Items[0].SentAt}}}}, weComCorpID: "corp-profile-messages"}},
		{"invalid message type", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Limit: 20, Total: 1, Items: []wecomport.CustomerChatSummary{{ChatType: "private", MessageType: "raw body", SentAt: validPage.Items[0].SentAt}}}}, weComCorpID: "corp-profile-messages"}},
		{"zero sent time", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Limit: 20, Total: 1, Items: []wecomport.CustomerChatSummary{{ChatType: "private", MessageType: "text"}}}}, weComCorpID: "corp-profile-messages"}},
		{"ascending projection", &Handler{customerDetail: validDetail, identityResolve: validIdentity, messageArchive: &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Limit: 20, Total: 2, Items: []wecomport.CustomerChatSummary{{ChatType: "private", MessageType: "text", SentAt: validPage.Items[0].SentAt}, {ChatType: "private", MessageType: "text", SentAt: validPage.Items[0].SentAt.Add(time.Minute)}}}}, weComCorpID: "corp-profile-messages"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serveLegacyCustomerProfileMessages(test.handler, authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, "external_userid=external-44"))
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"error_code":"customer_profile_messages_unavailable"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertLegacyCustomerProfileMessagesSecurityHeaders(t, response)
			assertLegacyCustomerProfileMessagesNoSensitiveFields(t, response.Body.String(), "external-44", "raw body")
		})
	}

	t.Run("dual resolver error still reads both hints before failing closed", func(t *testing.T) {
		identity := &legacyCustomerProfileMessagesIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
		union := &legacyCustomerProfileMessagesUnionStub{err: context.DeadlineExceeded}
		archive := &legacyCustomerProfileMessagesArchiveStub{}
		response := serveLegacyCustomerProfileMessages(&Handler{
			customerDetail: &legacyCustomerProfileMessagesDetailStub{}, identityResolve: identity,
			messageArchiveUnionID: union, messageArchive: archive, weComCorpID: "corp-profile-messages",
		}, authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, "unionid=union-44&external_userid=external-44"))
		if response.Code != http.StatusServiceUnavailable || union.calls != 1 || identity.calls != 1 || archive.summaryCalls != 0 {
			t.Fatalf("status/calls=%d/%d/%d/%d body=%s", response.Code, union.calls, identity.calls, archive.summaryCalls, response.Body.String())
		}
	})
}

func TestLegacyCustomerProfileMessagesReturnsAnAuthoritativeEmptyProjection(t *testing.T) {
	archive := &legacyCustomerProfileMessagesArchiveStub{page: wecomport.CustomerChatSummaryPage{Items: []wecomport.CustomerChatSummary{}, Limit: wecomapp.MessageArchiveDefaultLimit}}
	response := serveLegacyCustomerProfileMessages(&Handler{
		customerDetail:        &legacyCustomerProfileMessagesDetailStub{},
		messageArchiveUnionID: &legacyCustomerProfileMessagesUnionStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}},
		messageArchive:        archive,
	}, authorizedLegacyCustomerProfileMessagesRequest(http.MethodGet, "unionid=union-44"))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"messages":[]`) || !strings.Contains(response.Body.String(), `"count":0`) || archive.summaryCalls != 1 {
		t.Fatalf("status=%d body=%s archive=%+v", response.Code, response.Body.String(), archive)
	}
	assertLegacyCustomerProfileMessagesNoSensitiveFields(t, response.Body.String(), "union-44")
}

func TestLegacyCustomerProfileMessagesRejectsEveryNonGETMethodBeforeAuthenticationOrRead(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			detail := &legacyCustomerProfileMessagesDetailStub{}
			identity := &legacyCustomerProfileMessagesIdentityStub{}
			union := &legacyCustomerProfileMessagesUnionStub{}
			archive := &legacyCustomerProfileMessagesArchiveStub{}
			request := legacyCustomerProfileMessagesRequest(method, "user_id=44", nil, nil)
			response := serveLegacyCustomerProfileMessages(&Handler{
				customerDetail: detail, identityResolve: identity, messageArchiveUnionID: union,
				messageArchive: archive, weComCorpID: "corp-profile-messages",
			}, request)
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("status=%d allow=%q body=%s", response.Code, response.Header().Get("Allow"), response.Body.String())
			}
			if detail.calls != 0 || identity.calls != 0 || union.calls != 0 || archive.summaryCalls != 0 || archive.listCalls != 0 || archive.syncCalls != 0 {
				t.Fatalf("non-GET reached dependencies")
			}
			assertLegacyCustomerProfileMessagesSecurityHeaders(t, response)
		})
	}
}

func serveLegacyCustomerProfileMessages(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	http.HandlerFunc(handler.GetCustomerProfileMessages).ServeHTTP(response, request)
	return response
}

func authorizedLegacyCustomerProfileMessagesRequest(method, rawQuery string) *http.Request {
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}
	return legacyCustomerProfileMessagesRequest(method, rawQuery, &principal, &authorization)
}

func legacyCustomerProfileMessagesRequest(method, rawQuery string, principal *authport.Principal, authorization *authport.Authorization) *http.Request {
	request := httptest.NewRequest(method, legacyCustomerProfileMessagesPath, nil)
	request.URL.RawQuery = rawQuery
	ctx := request.Context()
	if principal != nil {
		ctx = authport.WithAuthenticatedSession(ctx, *principal, authport.SessionRef("profile-messages-test-session"))
	}
	if authorization != nil {
		if authorizedContext, err := authport.WithAuthorization(ctx, *authorization); err == nil {
			ctx = authorizedContext
		}
	}
	return request.WithContext(ctx)
}

func assertLegacyCustomerProfileMessagesSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers cache-control=%q nosniff=%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
}

func assertLegacyCustomerProfileMessagesNoSensitiveFields(t *testing.T, body string, sensitiveValues ...string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, forbiddenKey := range []string{
		`"unionid"`, `"external_userid"`, `"user_id"`, `"customer_id"`, `"content"`,
		`"message_body"`, `"sender"`, `"receiver"`, `"provider"`, `"provider_id"`,
		`"receipt"`, `"payload"`, `"opaque"`, `"msgid"`, `"source_id"`, `"with_userid"`,
	} {
		if strings.Contains(lower, forbiddenKey) {
			t.Fatalf("forbidden key %s in %s", forbiddenKey, body)
		}
	}
	for _, sensitive := range sensitiveValues {
		if sensitive != "" && strings.Contains(body, sensitive) {
			t.Fatalf("sensitive value %q in %s", sensitive, body)
		}
	}
}

type legacyCustomerProfileMessagesDetailStub struct {
	result contactapp.CustomerDetailStoreResult
	err    error
	input  contactapp.CustomerDetailInput
	calls  int
}

func (stub *legacyCustomerProfileMessagesDetailStub) Get(_ context.Context, input contactapp.CustomerDetailInput) (contactapp.CustomerDetailStoreResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

type legacyCustomerProfileMessagesIdentityStub struct {
	result identityport.ResolveResult
	err    error
	ref    identityport.IDRef
	calls  int
}

func (stub *legacyCustomerProfileMessagesIdentityStub) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	stub.calls++
	stub.ref = ref
	return stub.result, stub.err
}

type legacyCustomerProfileMessagesUnionStub struct {
	result identityport.ResolveResult
	err    error
	value  string
	calls  int
}

func (stub *legacyCustomerProfileMessagesUnionStub) ResolveUnionID(_ context.Context, value string) (identityport.ResolveResult, error) {
	stub.calls++
	stub.value = value
	return stub.result, stub.err
}

type legacyCustomerProfileMessagesArchiveStub struct {
	page          wecomport.CustomerChatSummaryPage
	err           error
	query         wecomport.CustomerChatSummaryQuery
	summaryCalls  int
	listCalls     int
	syncCalls     int
	healthCalls   int
	unsafeRecords []wecomapp.ArchiveMessage
}

func (stub *legacyCustomerProfileMessagesArchiveStub) ListCustomerChatSummaries(_ context.Context, query wecomport.CustomerChatSummaryQuery) (wecomport.CustomerChatSummaryPage, error) {
	stub.summaryCalls++
	stub.query = query
	return stub.page, stub.err
}

func (stub *legacyCustomerProfileMessagesArchiveStub) Health(context.Context) (wecomapp.ArchiveHealth, error) {
	stub.healthCalls++
	return wecomapp.ArchiveHealth{}, nil
}

func (stub *legacyCustomerProfileMessagesArchiveStub) List(context.Context, wecomapp.ArchiveQuery) ([]wecomapp.ArchiveMessage, int64, error) {
	stub.listCalls++
	return append([]wecomapp.ArchiveMessage(nil), stub.unsafeRecords...), int64(len(stub.unsafeRecords)), nil
}

func (stub *legacyCustomerProfileMessagesArchiveStub) RequestSync(context.Context, wecomapp.ArchiveSyncCommand) (wecomapp.ArchiveSyncReceipt, error) {
	stub.syncCalls++
	return wecomapp.ArchiveSyncReceipt{}, nil
}

type legacyCustomerProfileMessagesArchiveOnlyStub struct{}

func (*legacyCustomerProfileMessagesArchiveOnlyStub) Health(context.Context) (wecomapp.ArchiveHealth, error) {
	return wecomapp.ArchiveHealth{}, nil
}

func (*legacyCustomerProfileMessagesArchiveOnlyStub) List(context.Context, wecomapp.ArchiveQuery) ([]wecomapp.ArchiveMessage, int64, error) {
	return nil, 0, nil
}

func (*legacyCustomerProfileMessagesArchiveOnlyStub) RequestSync(context.Context, wecomapp.ArchiveSyncCommand) (wecomapp.ArchiveSyncReceipt, error) {
	return wecomapp.ArchiveSyncReceipt{}, nil
}

var _ customerDetailApplication = (*legacyCustomerProfileMessagesDetailStub)(nil)
var _ identityResolveApplication = (*legacyCustomerProfileMessagesIdentityStub)(nil)
var _ legacyMessageArchiveUnionResolver = (*legacyCustomerProfileMessagesUnionStub)(nil)
var _ legacyMessageArchiveApplication = (*legacyCustomerProfileMessagesArchiveStub)(nil)
var _ wecomport.CustomerChatSummaryReader = (*legacyCustomerProfileMessagesArchiveStub)(nil)
var _ legacyMessageArchiveApplication = (*legacyCustomerProfileMessagesArchiveOnlyStub)(nil)
