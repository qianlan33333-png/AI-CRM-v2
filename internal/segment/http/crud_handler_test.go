package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestCRUDHandlerClosesFrozenRuntimeOperations(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	segment := segmentport.Segment{ID: 7, Name: "高意向", Definition: []byte(`{"field":"is_deleted","op":"eq","value":false}`), RefreshMode: segmentport.RefreshModeManual, RefreshStatus: segmentport.RefreshStatusIdle, LifecycleStatus: segmentport.LifecycleStatusActive, CreatedAt: now, UpdatedAt: now}
	member := segmentapp.MemberRecord{ID: 9, Name: "客户", Extra: []byte(`{"source":"fixture"}`), CreatedAt: now, UpdatedAt: now}
	stub := &crudApplicationStub{segment: segment, page: segmentport.Page{Items: []segmentport.Segment{segment}}, memberPage: segmentapp.MemberPage{Items: []segmentapp.MemberRecord{member}}}
	handler, err := NewCRUDHandler(stub)
	if err != nil {
		t.Fatal(err)
	}

	list := httptest.NewRecorder()
	handler.ListSegments(list, crudRequest(t, http.MethodGet, "/api/v1/segments", "", authport.CapabilitySegmentsRead), generated.ListSegmentsParams{})
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body.String())
	}
	get := httptest.NewRecorder()
	handler.GetSegment(get, crudRequest(t, http.MethodGet, "/api/v1/segments/7", "", authport.CapabilitySegmentsRead), 7)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"lifecycle_status":"active"`) {
		t.Fatalf("get = %d: %s", get.Code, get.Body.String())
	}

	create := httptest.NewRecorder()
	handler.CreateSegment(create, crudRequest(t, http.MethodPost, "/api/v1/segments", `{"name":"高意向","definition":{"field":"is_deleted","op":"eq","value":false},"refresh_mode":"manual"}`, authport.CapabilitySegmentsWrite), generated.CreateSegmentParams{IdempotencyKey: "segment-create-0001"})
	if create.Code != http.StatusCreated || stub.create.Actor != "admin:42" || stub.create.IdempotencyKey != "segment-create-0001" {
		t.Fatalf("create = %d %#v: %s", create.Code, stub.create, create.Body.String())
	}

	update := httptest.NewRecorder()
	handler.UpdateSegment(update, crudRequest(t, http.MethodPatch, "/api/v1/segments/7", `{"refresh_mode":"manual","refresh_cron":null}`, authport.CapabilitySegmentsWrite), 7, generated.UpdateSegmentParams{IdempotencyKey: "segment-update-0001"})
	if update.Code != http.StatusOK || !stub.update.RefreshCronSet || stub.update.RefreshCron != nil || stub.update.SegmentID != 7 {
		t.Fatalf("mutation response = %d %#v: %s", update.Code, stub.update, update.Body.String())
	}

	stub.segment.LifecycleStatus = segmentport.LifecycleStatusArchived
	archive := httptest.NewRecorder()
	handler.ArchiveSegment(archive, crudRequest(t, http.MethodDelete, "/api/v1/segments/7", "", authport.CapabilitySegmentsWrite), 7, generated.ArchiveSegmentParams{IdempotencyKey: "segment-archive-0001"})
	if archive.Code != http.StatusOK || stub.archive.SegmentID != 7 || stub.archive.IdempotencyKey != "segment-archive-0001" || !strings.Contains(archive.Body.String(), `"lifecycle_status":"archived"`) {
		t.Fatalf("archive = %d %#v: %s", archive.Code, stub.archive, archive.Body.String())
	}

	members := httptest.NewRecorder()
	handler.ListSegmentMembers(members, crudRequest(t, http.MethodGet, "/api/v1/segments/7/members", "", authport.CapabilitySegmentsRead), 7, generated.ListSegmentMembersParams{})
	if members.Code != http.StatusOK || !strings.Contains(members.Body.String(), `"id":9`) {
		t.Fatalf("members = %d: %s", members.Code, members.Body.String())
	}
}

func TestCRUDHandlerFailsClosed(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name           string
		body           string
		capability     authport.Capability
		applicationErr error
		wantStatus     int
		wantCode       platformhttp.ErrorCode
	}{
		{name: "unknown body field", body: `{"name":"x","definition":{},"refresh_mode":"manual","escape":"forbidden"}`, capability: authport.CapabilitySegmentsWrite, wantStatus: 400, wantCode: platformhttp.CodeMalformedRequest},
		{name: "duplicate body field", body: `{"name":"x","name":"y","definition":{},"refresh_mode":"manual"}`, capability: authport.CapabilitySegmentsWrite, wantStatus: 400, wantCode: platformhttp.CodeMalformedRequest},
		{name: "wrong capability", body: `{"name":"x","definition":{},"refresh_mode":"manual"}`, capability: authport.CapabilitySegmentsRead, wantStatus: 403, wantCode: platformhttp.CodeUnauthorized},
		{name: "validation", body: `{"name":"x","definition":{},"refresh_mode":"manual"}`, capability: authport.CapabilitySegmentsWrite, applicationErr: segmentapp.ErrInvalidSegmentCommand, wantStatus: 422, wantCode: platformhttp.CodeValidationFailed},
		{name: "conflict", body: `{"name":"x","definition":{},"refresh_mode":"manual"}`, capability: authport.CapabilitySegmentsWrite, applicationErr: segmentapp.ErrSegmentCommandConflict, wantStatus: 409, wantCode: platformhttp.CodeConflict},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			handler, err := NewCRUDHandler(&crudApplicationStub{err: test.applicationErr})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.CreateSegment(response, crudRequest(t, http.MethodPost, "/api/v1/segments", test.body, test.capability), generated.CreateSegmentParams{IdempotencyKey: "segment-create-0001"})
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			var body struct {
				Code platformhttp.ErrorCode `json:"code"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body.Code != test.wantCode {
				t.Fatalf("error = %#v, %v; want %s", body, err, test.wantCode)
			}
		})
	}
}

func TestMemberPreviewRejectsChannelIdentityLeak(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	handler, err := NewCRUDHandler(&crudApplicationStub{memberPage: segmentapp.MemberPage{Items: []segmentapp.MemberRecord{{ID: 1, Extra: []byte(`{"external_userid":"secret"}`), CreatedAt: now, UpdatedAt: now}}}})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ListSegmentMembers(response, crudRequest(t, http.MethodGet, "/api/v1/segments/1/members", "", authport.CapabilitySegmentsRead), 1, generated.ListSegmentMembersParams{})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

type crudApplicationStub struct {
	segment    segmentport.Segment
	page       segmentport.Page
	memberPage segmentapp.MemberPage
	create     segmentport.CreateCommand
	update     segmentapp.UpdateInput
	archive    segmentport.ArchiveCommand
	err        error
}

func (stub *crudApplicationStub) List(context.Context, string, int32) (segmentport.Page, error) {
	return stub.page, stub.err
}
func (stub *crudApplicationStub) Get(context.Context, segmentport.SegmentID) (segmentport.Segment, error) {
	return stub.segment, stub.err
}
func (stub *crudApplicationStub) Create(_ context.Context, command segmentport.CreateCommand) (segmentport.Segment, error) {
	stub.create = command
	return stub.segment, stub.err
}
func (stub *crudApplicationStub) UpdateHTTP(_ context.Context, input segmentapp.UpdateInput) (segmentport.Segment, error) {
	stub.update = input
	return stub.segment, stub.err
}
func (stub *crudApplicationStub) Archive(_ context.Context, command segmentport.ArchiveCommand) (segmentport.Segment, error) {
	stub.archive = command
	return stub.segment, stub.err
}
func (stub *crudApplicationStub) ListMemberRecords(context.Context, segmentport.SegmentID, string, int32) (segmentapp.MemberPage, error) {
	return stub.memberPage, stub.err
}

func crudRequest(t *testing.T, method, target, body string, capability authport.Capability) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 42, Role: authport.RoleAdmin}, "session")
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}
