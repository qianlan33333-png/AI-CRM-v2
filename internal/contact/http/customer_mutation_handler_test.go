package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestNewCustomerMutationHandlerRejectsNilApplications(t *testing.T) {
	if handler, err := NewCustomerMutationHandler(nil); err == nil || handler != nil {
		t.Fatalf("NewCustomerMutationHandler(nil) = %#v, %v; want nil, fail-closed error", handler, err)
	}

	var typedNil *customerMutationApplicationStub
	if handler, err := NewCustomerMutationHandler(typedNil); err == nil || handler != nil {
		t.Fatalf("NewCustomerMutationHandler(typed nil) = %#v, %v; want nil, fail-closed error", handler, err)
	}
}

func TestCustomerMutationGeneratedRoutesPassGlobalAndOwnerScopesExactly(t *testing.T) {
	ownerStaffID := int64(71)
	callers := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		wantOwner     *int64
	}{
		{
			name:      "admin global",
			principal: authport.Principal{AdminUserID: 101, Role: authport.RoleAdmin},
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomersWrite,
				Scope:      authport.ScopeGlobal,
			},
		},
		{
			name:      "ops global",
			principal: authport.Principal{AdminUserID: 102, Role: authport.RoleOps},
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomersWrite,
				Scope:      authport.ScopeGlobal,
			},
		},
		{
			name:      "sales exact owner",
			principal: authport.Principal{AdminUserID: 103, Role: authport.RoleSales, StaffID: &ownerStaffID},
			authorization: authport.Authorization{
				Capability:   authport.CapabilityCustomersWrite,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: ownerStaffID,
			},
			wantOwner: &ownerStaffID,
		},
	}
	operations := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "update", method: http.MethodPatch, path: "/api/v1/customers/41", body: "{\"name\":\"Ada\"}"},
		{name: "set stage", method: http.MethodPut, path: "/api/v1/customers/41/stage", body: "{\"stage_id\":17}"},
		{name: "add tag", method: http.MethodPut, path: "/api/v1/customers/41/tags/13"},
		{name: "remove tag", method: http.MethodDelete, path: "/api/v1/customers/41/tags/13"},
	}

	for _, caller := range callers {
		caller := caller
		for _, operation := range operations {
			operation := operation
			t.Run(caller.name+"/"+operation.name, func(t *testing.T) {
				application := &customerMutationApplicationStub{}
				response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
					t, operation.method, operation.path, operation.body, &caller.principal, &caller.authorization, true,
				))

				switch operation.name {
				case "update":
					assertCustomerMutationSuccess(t, response)
					if len(application.updateCommands) != 1 || application.totalCalls() != 1 {
						t.Fatalf("update/total calls = %d/%d, want 1/1", len(application.updateCommands), application.totalCalls())
					}
					command := application.updateCommands[0]
					assertCustomerMutationScope(t, command.ID, command.ScopeOwnerStaffID, command.Actor, caller.principal, caller.wantOwner)
					if command.Name == nil || *command.Name != "Ada" || command.AvatarURL.Set || command.Gender.Set ||
						command.OwnerStaffID.Set || command.ChannelID.Set || command.Extra != nil {
						t.Fatalf("update command = %#v, want exact one-field name patch", command)
					}
				case "set stage":
					assertCustomerMutationSuccess(t, response)
					if len(application.stageCommands) != 1 || application.totalCalls() != 1 {
						t.Fatalf("stage/total calls = %d/%d, want 1/1", len(application.stageCommands), application.totalCalls())
					}
					command := application.stageCommands[0]
					assertCustomerMutationScope(t, command.ID, command.ScopeOwnerStaffID, command.Actor, caller.principal, caller.wantOwner)
					if command.StageID == nil || *command.StageID != 17 {
						t.Fatalf("stage command = %#v, want stage_id=17", command)
					}
				case "add tag":
					assertCustomerMutationNoContent(t, response)
					if len(application.addTagCommands) != 1 || application.totalCalls() != 1 {
						t.Fatalf("add-tag/total calls = %d/%d, want 1/1", len(application.addTagCommands), application.totalCalls())
					}
					command := application.addTagCommands[0]
					assertCustomerMutationScope(t, command.ID, command.ScopeOwnerStaffID, command.Actor, caller.principal, caller.wantOwner)
					if command.TagID != 13 {
						t.Fatalf("add-tag command = %#v, want tag_id=13", command)
					}
				case "remove tag":
					assertCustomerMutationNoContent(t, response)
					if len(application.removeTagCommands) != 1 || application.totalCalls() != 1 {
						t.Fatalf("remove-tag/total calls = %d/%d, want 1/1", len(application.removeTagCommands), application.totalCalls())
					}
					command := application.removeTagCommands[0]
					assertCustomerMutationScope(t, command.ID, command.ScopeOwnerStaffID, command.Actor, caller.principal, caller.wantOwner)
					if command.TagID != 13 {
						t.Fatalf("remove-tag command = %#v, want tag_id=13", command)
					}
				default:
					t.Fatalf("unhandled operation %q", operation.name)
				}
			})
		}
	}
}

func TestCustomerMutationGeneratedRoutesRequireExactlyOneCSRFHeader(t *testing.T) {
	operations := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "update", method: http.MethodPatch, path: "/api/v1/customers/41", body: "{\"name\":\"Ada\"}"},
		{name: "set stage", method: http.MethodPut, path: "/api/v1/customers/41/stage", body: "{\"stage_id\":17}"},
		{name: "add tag", method: http.MethodPut, path: "/api/v1/customers/41/tags/13"},
		{name: "remove tag", method: http.MethodDelete, path: "/api/v1/customers/41/tags/13"},
	}
	principal := authport.Principal{AdminUserID: 201, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name+"/missing", func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
				t, operation.method, operation.path, operation.body, &principal, &authorization, false,
			))
			assertCustomerMutationError(t, response, http.StatusBadRequest, platformhttp.CodeMalformedRequest)
			assertCustomerMutationNoCalls(t, application)
		})

		t.Run(operation.name+"/duplicate", func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			request := mutationRequest(t, operation.method, operation.path, operation.body, &principal, &authorization, true)
			request.Header.Add("X-CSRF-Token", "second-csrf-token")
			response := serveCustomerMutation(t, mutationRouter(t, application), request)
			assertCustomerMutationError(t, response, http.StatusBadRequest, platformhttp.CodeMalformedRequest)
			assertCustomerMutationNoCalls(t, application)
		})
	}
}

func TestCustomerMutationRoutesRequireSessionAndServerBoundCSRF(t *testing.T) {
	operations := []struct {
		name   string
		method string
		path   string
		body   string
		tag    bool
	}{
		{name: "update", method: http.MethodPatch, path: "/api/v1/customers/41", body: "{\"name\":\"Ada\"}"},
		{name: "set stage", method: http.MethodPut, path: "/api/v1/customers/41/stage", body: "{\"stage_id\":17}"},
		{name: "add tag", method: http.MethodPut, path: "/api/v1/customers/41/tags/13", tag: true},
		{name: "remove tag", method: http.MethodDelete, path: "/api/v1/customers/41/tags/13", tag: true},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name+"/missing server csrf", func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			auth := customerMutationAuthStubForAdmin()
			response := serveCustomerMutation(t, mutationProtectedRouter(t, application, auth), mutationProtectedRequest(
				operation.method, operation.path, operation.body, true, false,
			))
			assertCustomerMutationError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
			assertCustomerMutationNoCalls(t, application)
			if auth.authenticateCalls != 1 || auth.validateCSRFFalls != 0 || auth.authorizeCalls != 0 {
				t.Fatalf("authenticate/csrf/authorize calls = %d/%d/%d, want 1/0/0",
					auth.authenticateCalls, auth.validateCSRFFalls, auth.authorizeCalls)
			}
		})

		t.Run(operation.name+"/valid session and csrf", func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			auth := customerMutationAuthStubForAdmin()
			response := serveCustomerMutation(t, mutationProtectedRouter(t, application, auth), mutationProtectedRequest(
				operation.method, operation.path, operation.body, true, true,
			))
			if operation.tag {
				assertCustomerMutationNoContent(t, response)
			} else {
				assertCustomerMutationSuccess(t, response)
			}
			if application.totalCalls() != 1 {
				t.Fatalf("application calls = %d, want 1", application.totalCalls())
			}
			if auth.authenticateCalls != 1 || auth.validateCSRFFalls != 1 || auth.authorizeCalls != 1 ||
				len(auth.authorizedCapabilities) != 1 || auth.authorizedCapabilities[0] != authport.CapabilityCustomersWrite {
				t.Fatalf("authenticate/csrf/authorize/capabilities = %d/%d/%d/%#v, want 1/1/1/customers.write",
					auth.authenticateCalls, auth.validateCSRFFalls, auth.authorizeCalls, auth.authorizedCapabilities)
			}
		})
	}

	t.Run("missing session is unauthenticated before csrf and application", func(t *testing.T) {
		application := &customerMutationApplicationStub{}
		auth := customerMutationAuthStubForAdmin()
		response := serveCustomerMutation(t, mutationProtectedRouter(t, application, auth), mutationProtectedRequest(
			http.MethodPatch, "/api/v1/customers/41", "{\"name\":\"Ada\"}", false, true,
		))
		assertCustomerMutationError(t, response, http.StatusUnauthorized, platformhttp.CodeUnauthenticated)
		assertCustomerMutationNoCalls(t, application)
		if auth.authenticateCalls != 0 || auth.validateCSRFFalls != 0 || auth.authorizeCalls != 0 {
			t.Fatalf("authenticate/csrf/authorize calls = %d/%d/%d, want all zero",
				auth.authenticateCalls, auth.validateCSRFFalls, auth.authorizeCalls)
		}
	})

	t.Run("rejected server csrf stops authorization and application", func(t *testing.T) {
		application := &customerMutationApplicationStub{}
		auth := customerMutationAuthStubForAdmin()
		auth.validateCSRFErr = authport.ErrCSRFInvalid
		response := serveCustomerMutation(t, mutationProtectedRouter(t, application, auth), mutationProtectedRequest(
			http.MethodPatch, "/api/v1/customers/41", "{\"name\":\"Ada\"}", true, true,
		))
		assertCustomerMutationError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
		assertCustomerMutationNoCalls(t, application)
		if auth.authenticateCalls != 1 || auth.validateCSRFFalls != 1 || auth.authorizeCalls != 0 {
			t.Fatalf("authenticate/csrf/authorize calls = %d/%d/%d, want 1/1/0",
				auth.authenticateCalls, auth.validateCSRFFalls, auth.authorizeCalls)
		}
	})
}

func TestCustomerMutationHandlerFailsClosedBeforeApplication(t *testing.T) {
	salesOwner := int64(83)
	differentOwner := int64(84)
	tests := []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
		wantStatus    int
		wantCode      platformhttp.ErrorCode
	}{
		{
			name:       "missing authorization",
			principal:  &authport.Principal{AdminUserID: 301, Role: authport.RoleAdmin},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name: "missing authenticated principal",
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersWrite,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
		{
			name:      "wrong capability",
			principal: &authport.Principal{AdminUserID: 302, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales cannot claim global scope",
			principal: &authport.Principal{AdminUserID: 303, Role: authport.RoleSales, StaffID: &salesOwner},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersWrite,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "admin cannot claim owner scope",
			principal: &authport.Principal{AdminUserID: 304, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomersWrite,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: salesOwner,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales owner mismatch",
			principal: &authport.Principal{AdminUserID: 305, Role: authport.RoleSales, StaffID: &salesOwner},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomersWrite,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: differentOwner,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales lacks staff identity",
			principal: &authport.Principal{AdminUserID: 306, Role: authport.RoleSales},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomersWrite,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: salesOwner,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "invalid authenticated principal",
			principal: &authport.Principal{AdminUserID: 0, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersWrite,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
				t, http.MethodPatch, "/api/v1/customers/41", "{\"name\":\"Ada\"}", testCase.principal, testCase.authorization, true,
			))
			assertCustomerMutationError(t, response, testCase.wantStatus, testCase.wantCode)
			assertCustomerMutationNoCalls(t, application)
		})
	}
}

func TestCustomerMutationUpdatePreservesPresenceNullAndSmallintBounds(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		assert func(*testing.T, contactapp.CustomerUpdateCommand)
	}{
		{
			name: "omitted nullable fields remain omitted",
			body: "{\"name\":\"Ada\"}",
			assert: func(t *testing.T, command contactapp.CustomerUpdateCommand) {
				t.Helper()
				if command.Name == nil || *command.Name != "Ada" || command.AvatarURL.Set || command.Gender.Set ||
					command.OwnerStaffID.Set || command.ChannelID.Set || command.Extra != nil {
					t.Fatalf("command = %#v, want only name present", command)
				}
			},
		},
		{
			name: "explicit null remains an explicit clear",
			body: "{\"avatar_url\":null,\"gender\":null,\"owner_staff_id\":null,\"channel_id\":null,\"extra\":{}}",
			assert: func(t *testing.T, command contactapp.CustomerUpdateCommand) {
				t.Helper()
				if command.Name != nil {
					t.Fatalf("name = %v, want omitted", command.Name)
				}
				assertCustomerMutationStringPatch(t, command.AvatarURL, true, nil)
				assertCustomerMutationInt16Patch(t, command.Gender, true, nil)
				assertCustomerMutationInt64Patch(t, command.OwnerStaffID, true, nil)
				assertCustomerMutationInt64Patch(t, command.ChannelID, true, nil)
				if command.Extra == nil || string(*command.Extra) != "{}" {
					t.Fatalf("extra = %q, want explicit empty object", command.Extra)
				}
			},
		},
		{
			name: "all fields preserve their values",
			body: "{\"name\":\"Ada\",\"avatar_url\":\"https://cdn.example.test/a.png\",\"gender\":7,\"owner_staff_id\":23,\"channel_id\":29,\"extra\":{\"score\":9007199254740993}}",
			assert: func(t *testing.T, command contactapp.CustomerUpdateCommand) {
				t.Helper()
				if command.Name == nil || *command.Name != "Ada" {
					t.Fatalf("name = %v, want Ada", command.Name)
				}
				avatar := "https://cdn.example.test/a.png"
				gender := int16(7)
				owner := int64(23)
				channel := int64(29)
				assertCustomerMutationStringPatch(t, command.AvatarURL, true, &avatar)
				assertCustomerMutationInt16Patch(t, command.Gender, true, &gender)
				assertCustomerMutationInt64Patch(t, command.OwnerStaffID, true, &owner)
				assertCustomerMutationInt64Patch(t, command.ChannelID, true, &channel)
				if command.Extra == nil || string(*command.Extra) != "{\"score\":9007199254740993}" {
					t.Fatalf("extra = %q, want lossless JSON object", command.Extra)
				}
			},
		},
		{
			name: "smallint lower bound is accepted",
			body: "{\"gender\":-32768}",
			assert: func(t *testing.T, command contactapp.CustomerUpdateCommand) {
				t.Helper()
				value := int16(-32768)
				assertCustomerMutationInt16Patch(t, command.Gender, true, &value)
			},
		},
		{
			name: "smallint upper bound is accepted",
			body: "{\"gender\":32767}",
			assert: func(t *testing.T, command contactapp.CustomerUpdateCommand) {
				t.Helper()
				value := int16(32767)
				assertCustomerMutationInt16Patch(t, command.Gender, true, &value)
			},
		},
	}
	principal := authport.Principal{AdminUserID: 401, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
				t, http.MethodPatch, "/api/v1/customers/41", testCase.body, &principal, &authorization, true,
			))
			assertCustomerMutationSuccess(t, response)
			if len(application.updateCommands) != 1 || application.totalCalls() != 1 {
				t.Fatalf("update/total calls = %d/%d, want 1/1", len(application.updateCommands), application.totalCalls())
			}
			command := application.updateCommands[0]
			assertCustomerMutationScope(t, command.ID, command.ScopeOwnerStaffID, command.Actor, principal, nil)
			testCase.assert(t, command)
		})
	}
}

func TestCustomerMutationUpdateRejectsStrictBodiesAndOutOfRangeGender(t *testing.T) {
	oversize := "{\"name\":\"" + strings.Repeat("x", maxCustomerMutationBodyBytes) + "\"}"
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   platformhttp.ErrorCode
		hidden     []string
	}{
		{
			name:       "unknown top-level field",
			body:       "{\"name\":\"Ada\",\"external_userid\":\"identity-secret\"}",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
			hidden:     []string{"external_userid", "identity-secret"},
		},
		{
			name:       "duplicate top-level field",
			body:       "{\"name\":\"Ada\",\"name\":\"Bea\"}",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
			hidden:     []string{"Ada", "Bea"},
		},
		{
			name:       "duplicate nested extra field",
			body:       "{\"extra\":{\"label\":\"one\",\"label\":\"two\"}}",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
			hidden:     []string{"label", "one", "two"},
		},
		{
			name:       "multiple json values",
			body:       "{\"name\":\"Ada\"} {\"name\":\"Bea\"}",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
			hidden:     []string{"Ada", "Bea"},
		},
		{
			name:       "non-object array",
			body:       "[]",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
		},
		{
			name:       "non-object null",
			body:       "null",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
		},
		{
			name:       "extra must be object",
			body:       "{\"extra\":[]}",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
		},
		{
			name:       "malformed json",
			body:       "{\"name\":",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
		},
		{
			name:       "body over one mebibyte",
			body:       oversize,
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
			hidden:     []string{"xxxxxxxx"},
		},
		{
			name:       "empty patch",
			body:       "{}",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   platformhttp.CodeValidationFailed,
		},
		{
			name:       "gender below smallint",
			body:       "{\"gender\":-32769}",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   platformhttp.CodeValidationFailed,
		},
		{
			name:       "gender above smallint",
			body:       "{\"gender\":32768}",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   platformhttp.CodeValidationFailed,
		},
	}
	principal := authport.Principal{AdminUserID: 501, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
				t, http.MethodPatch, "/api/v1/customers/41", testCase.body, &principal, &authorization, true,
			))
			assertCustomerMutationError(t, response, testCase.wantStatus, testCase.wantCode)
			assertCustomerMutationNoCalls(t, application)
			assertCustomerMutationDoesNotContain(t, response, testCase.hidden...)
		})
	}
}

func TestCustomerMutationStageRequiresStageIDAndSupportsExplicitNull(t *testing.T) {
	principal := authport.Principal{AdminUserID: 601, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal}
	valid := []struct {
		name      string
		body      string
		wantStage *int64
	}{
		{name: "explicit null clears stage", body: "{\"stage_id\":null}"},
		{name: "positive stage is passed exactly", body: "{\"stage_id\":55}", wantStage: customerMutationInt64(55)},
	}

	for _, testCase := range valid {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
				t, http.MethodPut, "/api/v1/customers/41/stage", testCase.body, &principal, &authorization, true,
			))
			assertCustomerMutationSuccess(t, response)
			if len(application.stageCommands) != 1 || application.totalCalls() != 1 {
				t.Fatalf("stage/total calls = %d/%d, want 1/1", len(application.stageCommands), application.totalCalls())
			}
			command := application.stageCommands[0]
			assertCustomerMutationScope(t, command.ID, command.ScopeOwnerStaffID, command.Actor, principal, nil)
			if !customerMutationInt64PointersEqual(command.StageID, testCase.wantStage) {
				t.Fatalf("stage_id = %v, want %v", command.StageID, testCase.wantStage)
			}
		})
	}

	invalid := []struct {
		name string
		body string
	}{
		{name: "missing required stage id", body: "{}"},
		{name: "unknown stage object member", body: "{\"other\":55}"},
		{name: "extra stage object member", body: "{\"stage_id\":55,\"other\":1}"},
		{name: "duplicate stage id", body: "{\"stage_id\":55,\"stage_id\":56}"},
		{name: "non-integer stage id", body: "{\"stage_id\":\"55\"}"},
		{name: "multiple values", body: "{\"stage_id\":55} {\"stage_id\":56}"},
		{name: "non-object", body: "[]"},
	}
	for _, testCase := range invalid {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
				t, http.MethodPut, "/api/v1/customers/41/stage", testCase.body, &principal, &authorization, true,
			))
			assertCustomerMutationError(t, response, http.StatusBadRequest, platformhttp.CodeMalformedRequest)
			assertCustomerMutationNoCalls(t, application)
		})
	}

	t.Run("semantic stage validation remains a stable 422", func(t *testing.T) {
		application := &customerMutationApplicationStub{
			setStage: func(context.Context, contactapp.CustomerStageCommand) (contactapp.CustomerRecord, error) {
				return contactapp.CustomerRecord{}, contactapp.ErrInvalidCustomerMutation
			},
		}
		response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
			t, http.MethodPut, "/api/v1/customers/41/stage", "{\"stage_id\":0}", &principal, &authorization, true,
		))
		assertCustomerMutationError(t, response, http.StatusUnprocessableEntity, platformhttp.CodeValidationFailed)
		if len(application.stageCommands) != 1 || application.totalCalls() != 1 {
			t.Fatalf("stage/total calls = %d/%d, want 1/1", len(application.stageCommands), application.totalCalls())
		}
	})
}

func TestCustomerMutationTagsRemainIdempotentNoContent(t *testing.T) {
	principal := authport.Principal{AdminUserID: 701, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal}
	operations := []struct {
		name   string
		method string
		path   string
	}{
		{name: "add", method: http.MethodPut, path: "/api/v1/customers/41/tags/13"},
		{name: "remove", method: http.MethodDelete, path: "/api/v1/customers/41/tags/13"},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			router := mutationRouter(t, application)
			for attempt := 0; attempt < 2; attempt++ {
				response := serveCustomerMutation(t, router, mutationRequest(
					t, operation.method, operation.path, "", &principal, &authorization, true,
				))
				assertCustomerMutationNoContent(t, response)
			}
			switch operation.name {
			case "add":
				if len(application.addTagCommands) != 2 || application.totalCalls() != 2 {
					t.Fatalf("add-tag/total calls = %d/%d, want 2/2", len(application.addTagCommands), application.totalCalls())
				}
				for _, command := range application.addTagCommands {
					assertCustomerMutationScope(t, command.ID, command.ScopeOwnerStaffID, command.Actor, principal, nil)
					if command.TagID != 13 {
						t.Fatalf("add-tag command = %#v, want tag_id=13", command)
					}
				}
			case "remove":
				if len(application.removeTagCommands) != 2 || application.totalCalls() != 2 {
					t.Fatalf("remove-tag/total calls = %d/%d, want 2/2", len(application.removeTagCommands), application.totalCalls())
				}
				for _, command := range application.removeTagCommands {
					assertCustomerMutationScope(t, command.ID, command.ScopeOwnerStaffID, command.Actor, principal, nil)
					if command.TagID != 13 {
						t.Fatalf("remove-tag command = %#v, want tag_id=13", command)
					}
				}
			}
		})
	}
}

func TestCustomerMutationErrorResponsesAreStableAndDoNotLeakCauses(t *testing.T) {
	const secret = "postgres://mutator:private-password@127.0.0.1/aicrm"
	principal := authport.Principal{AdminUserID: 801, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal}
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		configure  func(*customerMutationApplicationStub)
		wantStatus int
		wantCode   platformhttp.ErrorCode
		wantCalls  int
	}{
		{
			name:       "malformed body",
			method:     http.MethodPatch,
			path:       "/api/v1/customers/41",
			body:       "{\"name\":",
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
		},
		{
			name:   "not found update",
			method: http.MethodPatch,
			path:   "/api/v1/customers/41",
			body:   "{\"name\":\"Ada\"}",
			configure: func(application *customerMutationApplicationStub) {
				application.update = func(context.Context, contactapp.CustomerUpdateCommand) (contactapp.CustomerRecord, error) {
					return contactapp.CustomerRecord{}, errors.Join(contactapp.ErrCustomerNotFound, errors.New(secret))
				}
			},
			wantStatus: http.StatusNotFound,
			wantCode:   platformhttp.CodeNotFound,
			wantCalls:  1,
		},
		{
			name:   "conflict stage",
			method: http.MethodPut,
			path:   "/api/v1/customers/41/stage",
			body:   "{\"stage_id\":17}",
			configure: func(application *customerMutationApplicationStub) {
				application.setStage = func(context.Context, contactapp.CustomerStageCommand) (contactapp.CustomerRecord, error) {
					return contactapp.CustomerRecord{}, errors.Join(contactapp.ErrCustomerConflict, errors.New(secret))
				}
			},
			wantStatus: http.StatusConflict,
			wantCode:   platformhttp.CodeConflict,
			wantCalls:  1,
		},
		{
			name:   "validation add tag",
			method: http.MethodPut,
			path:   "/api/v1/customers/41/tags/13",
			configure: func(application *customerMutationApplicationStub) {
				application.addTag = func(context.Context, contactapp.CustomerTagCommand) error {
					return errors.Join(contactapp.ErrInvalidCustomerMutation, errors.New(secret))
				}
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   platformhttp.CodeValidationFailed,
			wantCalls:  1,
		},
		{
			name:   "dependency remove tag",
			method: http.MethodDelete,
			path:   "/api/v1/customers/41/tags/13",
			configure: func(application *customerMutationApplicationStub) {
				application.removeTag = func(context.Context, contactapp.CustomerTagCommand) error {
					return errors.New(secret)
				}
			},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   platformhttp.CodeDependencyUnavailable,
			wantCalls:  1,
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerMutationApplicationStub{}
			if testCase.configure != nil {
				testCase.configure(application)
			}
			response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
				t, testCase.method, testCase.path, testCase.body, &principal, &authorization, true,
			))
			assertCustomerMutationError(t, response, testCase.wantStatus, testCase.wantCode)
			assertCustomerMutationDoesNotContain(t, response, secret, "private-password", "customer mutation", "sqlstate")
			if application.totalCalls() != testCase.wantCalls {
				t.Fatalf("application calls = %d, want %d", application.totalCalls(), testCase.wantCalls)
			}
		})
	}
}

func TestCustomerMutationResponsesStayStrictlyChannelNeutral(t *testing.T) {
	principal := authport.Principal{AdminUserID: 901, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal}

	t.Run("channel-neutral customer is rendered without identity fields", func(t *testing.T) {
		customer := customerMutationValidCustomer()
		customer.Extra = json.RawMessage("{\"metadata\":{\"score\":9007199254740993}}")
		application := &customerMutationApplicationStub{
			update: func(context.Context, contactapp.CustomerUpdateCommand) (contactapp.CustomerRecord, error) {
				return customer, nil
			},
		}
		response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
			t, http.MethodPatch, "/api/v1/customers/41", "{\"name\":\"Ada\"}", &principal, &authorization, true,
		))
		assertCustomerMutationSuccess(t, response)
		var body map[string]json.RawMessage
		decodeCustomerMutationJSON(t, response, &body)
		if _, found := body["external_userid"]; found {
			t.Fatalf("response contains forbidden external_userid: %s", response.Body.String())
		}
		if _, found := body["unionid"]; found {
			t.Fatalf("response contains forbidden unionid: %s", response.Body.String())
		}
		var extra map[string]json.RawMessage
		if err := json.Unmarshal(body["extra"], &extra); err != nil || extra["metadata"] == nil {
			t.Fatalf("response extra = %s, want channel-neutral metadata: %v", body["extra"], err)
		}
		assertCustomerMutationDoesNotContain(t, response, "external_userid", "unionid", "identity-secret")
	})

	t.Run("external identity returned by application is hidden", func(t *testing.T) {
		customer := customerMutationValidCustomer()
		customer.Extra = json.RawMessage("{\"nested\":{\"external_userid\":\"identity-secret\"}}")
		application := &customerMutationApplicationStub{
			update: func(context.Context, contactapp.CustomerUpdateCommand) (contactapp.CustomerRecord, error) {
				return customer, nil
			},
		}
		response := serveCustomerMutation(t, mutationRouter(t, application), mutationRequest(
			t, http.MethodPatch, "/api/v1/customers/41", "{\"name\":\"Ada\"}", &principal, &authorization, true,
		))
		assertCustomerMutationError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
		assertCustomerMutationDoesNotContain(t, response, "external_userid", "identity-secret", "invalid customer")
		if len(application.updateCommands) != 1 || application.totalCalls() != 1 {
			t.Fatalf("update/total calls = %d/%d, want 1/1", len(application.updateCommands), application.totalCalls())
		}
	})
}

type customerMutationApplicationStub struct {
	update    func(context.Context, contactapp.CustomerUpdateCommand) (contactapp.CustomerRecord, error)
	setStage  func(context.Context, contactapp.CustomerStageCommand) (contactapp.CustomerRecord, error)
	addTag    func(context.Context, contactapp.CustomerTagCommand) error
	removeTag func(context.Context, contactapp.CustomerTagCommand) error

	updateCommands    []contactapp.CustomerUpdateCommand
	stageCommands     []contactapp.CustomerStageCommand
	addTagCommands    []contactapp.CustomerTagCommand
	removeTagCommands []contactapp.CustomerTagCommand
}

var _ customerMutationApplication = (*customerMutationApplicationStub)(nil)

type customerMutationAuthServiceStub struct {
	principal       authport.Principal
	authorization   authport.Authorization
	authenticateErr error
	authorizeErr    error
	validateCSRFErr error

	authenticateCalls      int
	validateCSRFFalls      int
	authorizeCalls         int
	authorizedCapabilities []authport.Capability
}

var _ authport.Service = (*customerMutationAuthServiceStub)(nil)

func customerMutationAuthStubForAdmin() *customerMutationAuthServiceStub {
	return &customerMutationAuthServiceStub{
		principal: authport.Principal{AdminUserID: 211, Role: authport.RoleAdmin},
		authorization: authport.Authorization{
			Capability: authport.CapabilityCustomersWrite,
			Scope:      authport.ScopeGlobal,
		},
	}
}

func (stub *customerMutationAuthServiceStub) Authenticate(
	context.Context,
	authport.SessionRef,
) (authport.Principal, error) {
	stub.authenticateCalls++
	return stub.principal, stub.authenticateErr
}

func (stub *customerMutationAuthServiceStub) Authorize(
	_ context.Context,
	_ authport.Principal,
	capability authport.Capability,
) (authport.Authorization, error) {
	stub.authorizeCalls++
	stub.authorizedCapabilities = append(stub.authorizedCapabilities, capability)
	return stub.authorization, stub.authorizeErr
}

func (stub *customerMutationAuthServiceStub) ValidateCSRF(
	context.Context,
	authport.SessionRef,
	authport.CSRFToken,
) error {
	stub.validateCSRFFalls++
	return stub.validateCSRFErr
}

func (*customerMutationAuthServiceStub) Invalidate(
	context.Context,
	authport.SessionRef,
	authport.CSRFToken,
) error {
	return nil
}

func (stub *customerMutationApplicationStub) Update(
	ctx context.Context,
	command contactapp.CustomerUpdateCommand,
) (contactapp.CustomerRecord, error) {
	stub.updateCommands = append(stub.updateCommands, command)
	if stub.update != nil {
		return stub.update(ctx, command)
	}
	return customerMutationValidCustomer(), nil
}

func (stub *customerMutationApplicationStub) SetStage(
	ctx context.Context,
	command contactapp.CustomerStageCommand,
) (contactapp.CustomerRecord, error) {
	stub.stageCommands = append(stub.stageCommands, command)
	if stub.setStage != nil {
		return stub.setStage(ctx, command)
	}
	return customerMutationValidCustomer(), nil
}

func (stub *customerMutationApplicationStub) AddTag(ctx context.Context, command contactapp.CustomerTagCommand) error {
	stub.addTagCommands = append(stub.addTagCommands, command)
	if stub.addTag != nil {
		return stub.addTag(ctx, command)
	}
	return nil
}

func (stub *customerMutationApplicationStub) RemoveTag(ctx context.Context, command contactapp.CustomerTagCommand) error {
	stub.removeTagCommands = append(stub.removeTagCommands, command)
	if stub.removeTag != nil {
		return stub.removeTag(ctx, command)
	}
	return nil
}

func (stub *customerMutationApplicationStub) totalCalls() int {
	return len(stub.updateCommands) + len(stub.stageCommands) + len(stub.addTagCommands) + len(stub.removeTagCommands)
}

type customerMutationGeneratedServer struct {
	generated.Unimplemented
	handler *CustomerMutationHandler
}

var _ generated.ServerInterface = (*customerMutationGeneratedServer)(nil)

func (server *customerMutationGeneratedServer) UpdateCustomer(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	params generated.UpdateCustomerParams,
) {
	server.handler.UpdateCustomer(writer, request, customerID, params)
}

func (server *customerMutationGeneratedServer) SetCustomerStage(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	params generated.SetCustomerStageParams,
) {
	server.handler.SetCustomerStage(writer, request, customerID, params)
}

func (server *customerMutationGeneratedServer) AddCustomerTag(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	tagID generated.TagID,
	params generated.AddCustomerTagParams,
) {
	server.handler.AddCustomerTag(writer, request, customerID, tagID, params)
}

func (server *customerMutationGeneratedServer) RemoveCustomerTag(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	tagID generated.TagID,
	params generated.RemoveCustomerTagParams,
) {
	server.handler.RemoveCustomerTag(writer, request, customerID, tagID, params)
}

func mutationRouter(t *testing.T, application customerMutationApplication) http.Handler {
	t.Helper()
	handler, err := NewCustomerMutationHandler(application)
	if err != nil {
		t.Fatalf("NewCustomerMutationHandler() error = %v", err)
	}
	return generated.HandlerWithOptions(&customerMutationGeneratedServer{handler: handler}, generated.ChiServerOptions{
		BaseRouter:       chi.NewRouter(),
		ErrorHandlerFunc: platformhttp.RequestErrorHandler,
	})
}

func mutationProtectedRouter(
	t *testing.T,
	application customerMutationApplication,
	auth authport.Service,
) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatalf("authhttp.NewHandler() error = %v", err)
	}
	var route http.Handler = mutationRouter(t, application)
	route, err = authHandler.Authorize(authport.CapabilityCustomersWrite, route)
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	route, err = authHandler.RequireCSRF(route)
	if err != nil {
		t.Fatalf("RequireCSRF() error = %v", err)
	}
	return authHandler.Authenticate(route)
}

func mutationRequest(
	t *testing.T,
	method string,
	path string,
	body string,
	principal *authport.Principal,
	authorization *authport.Authorization,
	withCSRF bool,
) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if withCSRF {
		request.Header.Set("X-CSRF-Token", "customer-mutation-csrf-token")
	}
	ctx := context.Background()
	if principal != nil {
		ctx = authport.WithAuthenticatedSession(ctx, *principal, "customer-mutation-test-session")
	}
	if authorization != nil {
		var err error
		ctx, err = authport.WithAuthorization(ctx, *authorization)
		if err != nil {
			t.Fatalf("WithAuthorization(%#v) error = %v", *authorization, err)
		}
	}
	return request.WithContext(ctx)
}

func mutationProtectedRequest(
	method string,
	path string,
	body string,
	withSession bool,
	withCSRF bool,
) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if withSession {
		request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "customer-mutation-server-session"})
	}
	if withCSRF {
		request.Header.Set("X-CSRF-Token", "customer-mutation-csrf-token")
	}
	return request
}

func serveCustomerMutation(t *testing.T, handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertCustomerMutationSuccess(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	assertCustomerMutationJSONHeaders(t, response)
	var body map[string]json.RawMessage
	decodeCustomerMutationJSON(t, response, &body)
	var id int64
	if err := json.Unmarshal(body["id"], &id); err != nil || id != 41 {
		t.Fatalf("response id = %s, want 41: %v", body["id"], err)
	}
}

func assertCustomerMutationNoContent(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("204 body = %q, want empty", response.Body.String())
	}
}

func assertCustomerMutationError(
	t *testing.T,
	response *httptest.ResponseRecorder,
	wantStatus int,
	wantCode platformhttp.ErrorCode,
) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	assertCustomerMutationJSONHeaders(t, response)
	var body map[string]json.RawMessage
	decodeCustomerMutationJSON(t, response, &body)
	var code platformhttp.ErrorCode
	var message string
	var requestID string
	if err := json.Unmarshal(body["code"], &code); err != nil {
		t.Fatalf("error code JSON = %v; body=%s", err, response.Body.String())
	}
	if err := json.Unmarshal(body["message"], &message); err != nil {
		t.Fatalf("error message JSON = %v; body=%s", err, response.Body.String())
	}
	if err := json.Unmarshal(body["request_id"], &requestID); err != nil {
		t.Fatalf("error request_id JSON = %v; body=%s", err, response.Body.String())
	}
	if code != wantCode || message == "" || requestID == "" {
		t.Fatalf("error body code/message/request_id = %q/%q/%q, want %q/nonempty/nonempty", code, message, requestID, wantCode)
	}
}

func assertCustomerMutationJSONHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func assertCustomerMutationDoesNotContain(t *testing.T, response *httptest.ResponseRecorder, values ...string) {
	t.Helper()
	body := response.Body.String()
	for _, value := range values {
		if value != "" && strings.Contains(body, value) {
			t.Fatalf("response leaked %q: %s", value, body)
		}
	}
}

func assertCustomerMutationNoCalls(t *testing.T, application *customerMutationApplicationStub) {
	t.Helper()
	if application.totalCalls() != 0 {
		t.Fatalf("application calls update/stage/add/remove = %d/%d/%d/%d, want all zero",
			len(application.updateCommands), len(application.stageCommands),
			len(application.addTagCommands), len(application.removeTagCommands))
	}
}

func assertCustomerMutationScope(
	t *testing.T,
	id contactport.CustomerID,
	gotOwner *int64,
	gotActor contactport.Actor,
	principal authport.Principal,
	wantOwner *int64,
) {
	t.Helper()
	if id != 41 {
		t.Fatalf("customer ID = %d, want 41", id)
	}
	if !customerMutationInt64PointersEqual(gotOwner, wantOwner) {
		t.Fatalf("scope owner = %v, want %v", gotOwner, wantOwner)
	}
	wantActor := contactport.Actor("admin:" + strconv.FormatInt(principal.AdminUserID, 10))
	if gotActor != wantActor {
		t.Fatalf("actor = %q, want %q", gotActor, wantActor)
	}
}

func assertCustomerMutationStringPatch(
	t *testing.T,
	got contactapp.NullablePatch[string],
	wantSet bool,
	want *string,
) {
	t.Helper()
	if got.Set != wantSet || !customerMutationStringPointersEqual(got.Value, want) {
		t.Fatalf("string patch = %#v, want Set=%t Value=%v", got, wantSet, want)
	}
}

func assertCustomerMutationInt16Patch(
	t *testing.T,
	got contactapp.NullablePatch[int16],
	wantSet bool,
	want *int16,
) {
	t.Helper()
	if got.Set != wantSet || !customerMutationInt16PointersEqual(got.Value, want) {
		t.Fatalf("int16 patch = %#v, want Set=%t Value=%v", got, wantSet, want)
	}
}

func assertCustomerMutationInt64Patch(
	t *testing.T,
	got contactapp.NullablePatch[int64],
	wantSet bool,
	want *int64,
) {
	t.Helper()
	if got.Set != wantSet || !customerMutationInt64PointersEqual(got.Value, want) {
		t.Fatalf("int64 patch = %#v, want Set=%t Value=%v", got, wantSet, want)
	}
}

func decodeCustomerMutationJSON(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("response JSON error = %v; body=%q", err, response.Body.String())
	}
}

func customerMutationValidCustomer() contactapp.CustomerRecord {
	createdAt := time.Date(2026, time.August, 11, 8, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.August, 12, 8, 30, 0, 0, time.UTC)
	return contactapp.CustomerRecord{
		ID:        41,
		Name:      "mutated customer",
		Extra:     json.RawMessage("{\"source\":\"customer-mutation-handler-test\"}"),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

func customerMutationInt64(value int64) *int64 {
	return &value
}

func customerMutationInt64PointersEqual(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func customerMutationInt16PointersEqual(left, right *int16) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func customerMutationStringPointersEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
