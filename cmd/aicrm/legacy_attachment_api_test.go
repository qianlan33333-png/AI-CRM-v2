package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type legacyAttachmentStub struct {
	list          mediaport.AttachmentListPage
	listErr       error
	listQuery     mediaport.AttachmentListQuery
	listCalls     int
	attachment    mediaport.Attachment
	getErr        error
	getID         int64
	getCalls      int
	upload        mediaport.Attachment
	uploadErr     error
	uploadCommand mediaport.AttachmentUploadCommand
	uploadCalls   int
	updated       mediaport.Attachment
	updateErr     error
	updateCommand mediaport.AttachmentUpdateCommand
	updateCalls   int
	deleted       mediaapp.AttachmentDeleteResult
	deleteErr     error
	deleteCommand mediaport.AttachmentDeleteCommand
	deleteCalls   int
	download      mediaapp.AttachmentDownload
	downloadErr   error
	downloadID    int64
	downloadCalls int
}

func (stub *legacyAttachmentStub) List(_ context.Context, query mediaport.AttachmentListQuery) (mediaport.AttachmentListPage, error) {
	stub.listCalls++
	stub.listQuery = query
	return stub.list, stub.listErr
}

func (stub *legacyAttachmentStub) Get(_ context.Context, id int64) (mediaport.Attachment, error) {
	stub.getCalls++
	stub.getID = id
	return stub.attachment, stub.getErr
}

func (stub *legacyAttachmentStub) Upload(_ context.Context, command mediaport.AttachmentUploadCommand) (mediaport.Attachment, error) {
	stub.uploadCalls++
	stub.uploadCommand = command
	return stub.upload, stub.uploadErr
}

func (stub *legacyAttachmentStub) Update(_ context.Context, command mediaport.AttachmentUpdateCommand) (mediaport.Attachment, error) {
	stub.updateCalls++
	stub.updateCommand = command
	return stub.updated, stub.updateErr
}

func (stub *legacyAttachmentStub) Delete(_ context.Context, command mediaport.AttachmentDeleteCommand) (mediaapp.AttachmentDeleteResult, error) {
	stub.deleteCalls++
	stub.deleteCommand = command
	return stub.deleted, stub.deleteErr
}

func (stub *legacyAttachmentStub) Download(_ context.Context, id int64) (mediaapp.AttachmentDownload, error) {
	stub.downloadCalls++
	stub.downloadID = id
	return stub.download, stub.downloadErr
}

func TestLegacyAttachmentListAndMultipartAliasesRemainPrivate(t *testing.T) {
	attachment := testLegacyAttachment()
	stub := &legacyAttachmentStub{
		list:   mediaport.AttachmentListPage{Items: []mediaport.Attachment{attachment}, Total: 1, Limit: 100, Offset: 0},
		upload: attachment,
	}
	auth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}}
	router := legacyAttachmentRouterWithAuth(t, stub, auth)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyAttachmentRequest(http.MethodGet, "/api/admin/attachment-library", "", nil, true, false))
	if response.Code != http.StatusOK || stub.listCalls != 1 || stub.listQuery != (mediaport.AttachmentListQuery{Limit: 100, Offset: 0, EnabledOnly: true}) ||
		len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 {
		t.Fatalf("status=%d list=%#v auth=%#v body=%s", response.Code, stub.listQuery, auth, response.Body.String())
	}
	var page map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page) != 4 || strings.Contains(response.Body.String(), "blob") || strings.Contains(response.Body.String(), "checksum") || strings.Contains(response.Body.String(), "provider") || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("headers=%v body=%s", response.Header(), response.Body.String())
	}

	for _, path := range []string{legacyAttachmentCollectionPath, legacyAttachmentUploadPath} {
		t.Run(path, func(t *testing.T) {
			body, contentType := legacyAttachmentMultipart(t, []legacyAttachmentPart{
				{name: "attachment", filename: "guide.pdf", mediaType: "application/pdf", value: testPDFBytes()},
				{name: "name", value: []byte("Guide")},
				{name: "tags", value: []byte("onboarding, pdf")},
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyAttachmentRequest(http.MethodPost, path, "", body, true, true, contentType))
			if response.Code != http.StatusOK || stub.uploadCalls == 0 || stub.uploadCommand.Actor != 7 || stub.uploadCommand.IdempotencyKey != "attachment-key-0001" ||
				stub.uploadCommand.FileName != "guide.pdf" || stub.uploadCommand.DeclaredType != "application/pdf" || stub.uploadCommand.Name != "Guide" ||
				!bytes.Equal(stub.uploadCommand.Content, testPDFBytes()) || strings.Join(stub.uploadCommand.Tags, ",") != "onboarding,pdf" || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d command=%#v headers=%v body=%s", response.Code, stub.uploadCommand, response.Header(), response.Body.String())
			}
		})
	}
}

func TestLegacyAttachmentDetailUpdateDeleteAndPrivateDownload(t *testing.T) {
	attachment := testLegacyAttachment()
	stub := &legacyAttachmentStub{
		attachment: attachment,
		updated:    attachment,
		deleted:    mediaapp.AttachmentDeleteResult{ID: attachment.ID, Deleted: true, HardDeleted: true},
		download:   mediaapp.AttachmentDownload{Attachment: attachment, Content: testPDFBytes()},
	}
	router := legacyAttachmentRouterWithAuth(t, stub, &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyAttachmentRequest(http.MethodGet, "/api/admin/attachment-library/42", "", nil, true, false))
	if response.Code != http.StatusOK || stub.getCalls != 1 || stub.getID != 42 || strings.Contains(response.Body.String(), "content") {
		t.Fatalf("get status=%d calls=%d body=%s", response.Code, stub.getCalls, response.Body.String())
	}

	response = httptest.NewRecorder()
	update := []byte(`{"expected_version":3,"name":"Renamed","description":"Private","tags":["handbook"],"enabled":false}`)
	router.ServeHTTP(response, legacyAttachmentRequest(http.MethodPut, "/api/admin/attachment-library/42", "", bytes.NewReader(update), true, true, "application/json"))
	if response.Code != http.StatusOK || stub.updateCalls != 1 || stub.updateCommand.AttachmentID != 42 || stub.updateCommand.ExpectedVersion != 3 || stub.updateCommand.Actor != 7 ||
		stub.updateCommand.IdempotencyKey != "attachment-key-0001" || stub.updateCommand.Name != "Renamed" || stub.updateCommand.Description != "Private" ||
		strings.Join(stub.updateCommand.Tags, ",") != "handbook" || stub.updateCommand.Enabled {
		t.Fatalf("update status=%d command=%#v body=%s", response.Code, stub.updateCommand, response.Body.String())
	}

	stub.deleteErr = mediaapp.ErrAttachmentHasReferences
	stub.deleted = mediaapp.AttachmentDeleteResult{ID: 42, References: mediaapp.AttachmentDeleteReferences{AutomationAgents: []int64{2}, Channels: []int64{3}, RadarLinks: []int64{4}}}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, legacyAttachmentRequest(http.MethodDelete, "/api/admin/attachment-library/42", "", nil, true, true))
	if response.Code != http.StatusConflict || stub.deleteCalls != 1 || stub.deleteCommand != (mediaport.AttachmentDeleteCommand{AttachmentID: 42, Actor: 7, IdempotencyKey: "attachment-key-0001"}) ||
		!strings.Contains(response.Body.String(), `"automation_agents":[{"id":2}]`) || !strings.Contains(response.Body.String(), `"channels":[{"id":3}]`) || !strings.Contains(response.Body.String(), `"radar_links":[{"id":4}]`) {
		t.Fatalf("delete status=%d command=%#v headers=%v body=%s", response.Code, stub.deleteCommand, response.Header(), response.Body.String())
	}

	stub.deleteErr = nil
	stub.deleted = mediaapp.AttachmentDeleteResult{ID: 42, Deleted: true, HardDeleted: true}
	deleteCalls := stub.deleteCalls
	response = httptest.NewRecorder()
	chunkedDelete := legacyAttachmentRequest(http.MethodDelete, "/api/admin/attachment-library/42", "", strings.NewReader("unexpected"), true, true)
	chunkedDelete.ContentLength = -1
	router.ServeHTTP(response, chunkedDelete)
	if response.Code != http.StatusBadRequest || stub.deleteCalls != deleteCalls {
		t.Fatalf("chunked delete status=%d calls=%d, want 400/%d", response.Code, stub.deleteCalls, deleteCalls)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, legacyAttachmentRequest(http.MethodGet, "/api/admin/attachment-library/42/download", "", nil, true, false))
	if response.Code != http.StatusOK || stub.downloadCalls != 1 || stub.downloadID != 42 || !bytes.Equal(response.Body.Bytes(), testPDFBytes()) ||
		response.Header().Get("Content-Type") != "application/pdf" || response.Header().Get("Content-Disposition") == "" || response.Header().Get("Cache-Control") != "private, no-store" ||
		response.Header().Get("Content-Security-Policy") != "sandbox" || response.Header().Get("Referrer-Policy") != "no-referrer" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("download status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.Bytes())
	}
}

func TestLegacyAttachmentRejectsInvalidInputBeforeApplicationAndGuardsMethods(t *testing.T) {
	attachment := testLegacyAttachment()
	stub := &legacyAttachmentStub{list: mediaport.AttachmentListPage{Items: []mediaport.Attachment{}, Total: 0, Limit: 100, Offset: 0}, upload: attachment}
	router := legacyAttachmentRouterWithAuth(t, stub, &legacyMediaAuthStub{})

	for _, test := range []struct {
		name, method, path string
		body               io.Reader
		contentType        string
		want               int
		allow              string
	}{
		{"collection method", http.MethodPatch, legacyAttachmentCollectionPath, nil, "", http.StatusMethodNotAllowed, "GET, POST"},
		{"upload method", http.MethodGet, legacyAttachmentUploadPath, nil, "", http.StatusMethodNotAllowed, "POST"},
		{"detail method", http.MethodPatch, "/api/admin/attachment-library/1", nil, "", http.StatusMethodNotAllowed, "GET, PUT, DELETE"},
		{"download method", http.MethodPost, "/api/admin/attachment-library/1/download", nil, "", http.StatusMethodNotAllowed, "GET"},
		{"json placeholder", http.MethodPost, legacyAttachmentCollectionPath, strings.NewReader(`{}`), "application/json", http.StatusBadRequest, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := legacyAttachmentRequest(test.method, test.path, "", test.body, true, test.method == http.MethodPost || test.method == http.MethodPut || test.method == http.MethodDelete, test.contentType)
			if strings.HasPrefix(test.contentType, "multipart/form-data") {
				request.Header.Set("Content-Type", test.contentType)
			}
			router.ServeHTTP(response, request)
			if response.Code != test.want || response.Header().Get("Allow") != test.allow || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d allow=%q headers=%v body=%s", response.Code, response.Header().Get("Allow"), response.Header(), response.Body.String())
			}
		})
	}
	if stub.uploadCalls != 0 {
		t.Fatalf("invalid requests reached upload: %d", stub.uploadCalls)
	}

	unknownBody, unknownContentType := legacyAttachmentMultipart(t, []legacyAttachmentPart{
		{name: "attachment", filename: "guide.pdf", mediaType: "application/pdf", value: testPDFBytes()},
		{name: "remote_url", value: []byte("https://example.invalid/file.pdf")},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyAttachmentRequest(http.MethodPost, legacyAttachmentCollectionPath, "", unknownBody, true, true, unknownContentType))
	if response.Code != http.StatusBadRequest || stub.uploadCalls != 0 {
		t.Fatalf("unknown multipart status=%d calls=%d body=%s", response.Code, stub.uploadCalls, response.Body.String())
	}

	stub.uploadErr = errors.New("postgres attachment=42 checksum=secret")
	body, contentType := legacyAttachmentMultipart(t, []legacyAttachmentPart{{name: "attachment", filename: "guide.pdf", mediaType: "application/pdf", value: testPDFBytes()}})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, legacyAttachmentRequest(http.MethodPost, legacyAttachmentCollectionPath, "", body, true, true, contentType))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"DEPENDENCY_UNAVAILABLE"`) || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("error status=%d body=%s", response.Code, response.Body.String())
	}
}

type legacyAttachmentPart struct {
	name, filename, mediaType string
	value                     []byte
}

func legacyAttachmentMultipart(t *testing.T, parts []legacyAttachmentPart) (*bytes.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		if part.filename == "" {
			if err := writer.WriteField(part.name, string(part.value)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="`+part.name+`"; filename="`+part.filename+`"`)
		header.Set("Content-Type", part.mediaType)
		file, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(part.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(body.Bytes()), writer.FormDataContentType()
}

func legacyAttachmentRequest(method, path, rawQuery string, body io.Reader, withSession, withCSRF bool, contentType ...string) *http.Request {
	request := httptest.NewRequest(method, path, body)
	request.URL.RawQuery = rawQuery
	if len(contentType) != 0 && contentType[0] != "" {
		request.Header.Set("Content-Type", contentType[0])
	}
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete {
		request.Header.Set("Idempotency-Key", "attachment-key-0001")
	}
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(93)})
	}
	if withCSRF {
		request.Header.Set("X-CSRF-Token", legacyToken(94))
	}
	return request
}

func legacyAttachmentRouterWithAuth(t *testing.T, attachments legacyAttachmentApplication, service authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsAndMedia(service, &legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.attachments = attachments
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func testLegacyAttachment() mediaport.Attachment {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	return mediaport.Attachment{ID: 42, Name: "Guide", FileName: "guide.pdf", MimeType: "application/pdf", FileSize: int64(len(testPDFBytes())),
		Description: "Private", Tags: []string{"onboarding"}, Enabled: true, Version: 3, CreatedBy: 7, UpdatedBy: 7, CreatedAt: now, UpdatedAt: now}
}

func testPDFBytes() []byte { return []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n") }
