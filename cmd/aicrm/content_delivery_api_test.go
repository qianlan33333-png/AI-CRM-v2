package main

import (
	"context"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type contentDeliveryStub struct {
	initiated       bool
	initiateCommand mediaport.AttachmentUploadInitiateCommand
	part            bool
	completed       bool
	bound           bool
}

func (s *contentDeliveryStub) Preview(context.Context, mediaport.ContentPackageCommand) (mediaport.ContentPackage, error) {
	return mediaport.ContentPackage{}, nil
}
func (s *contentDeliveryStub) Create(context.Context, mediaport.ContentPackageCommand) (mediaport.ContentPackage, error) {
	return mediaport.ContentPackage{}, nil
}
func (s *contentDeliveryStub) Update(context.Context, mediaport.ContentPackageUpdateCommand) (mediaport.ContentPackage, error) {
	return mediaport.ContentPackage{}, nil
}
func (s *contentDeliveryStub) Bind(context.Context, mediaport.DeliveryBindingCommand) (mediaport.DeliveryBinding, error) {
	s.bound = true
	return mediaport.DeliveryBinding{}, nil
}
func (s *contentDeliveryStub) GetBinding(context.Context, string, string) (mediaport.DeliveryBinding, error) {
	return mediaport.DeliveryBinding{}, nil
}

func TestContentDeliveryBindingWriteUsesService(t *testing.T) {
	stub := &contentDeliveryStub{}
	h := &Handler{contentDelivery: stub}
	w := httptest.NewRecorder()
	r := contentRequest(t, http.MethodPost, "/api/admin/campaigns/c/plan/p/content-delivery-binding", `{"package_id":1,"group_invite_id":2}`)
	r.SetPathValue("campaign_code", "c")
	r.SetPathValue("plan_id", "p")
	h.ContentDeliveryBindingCreate(w, r)
	if w.Code != http.StatusOK || !stub.bound {
		t.Fatalf("code=%d bound=%v", w.Code, stub.bound)
	}
}
func (s *contentDeliveryStub) InitiatePDF(_ context.Context, command mediaport.AttachmentUploadInitiateCommand) (int64, error) {
	s.initiated = true
	s.initiateCommand = command
	return 11, nil
}
func (s *contentDeliveryStub) PutPDFPart(context.Context, mediaport.AttachmentUploadPartCommand) error {
	s.part = true
	return nil
}
func (s *contentDeliveryStub) CompletePDF(context.Context, mediaport.AttachmentUploadCompleteCommand) (int64, error) {
	s.completed = true
	return 12, nil
}
func contentRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Idempotency-Key", "content-delivery-key-0001")
	ctx := authport.WithAuthenticatedSession(r.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, "s")
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityMediaLibraryWrite, Scope: authport.ScopeGlobal})
	return r.WithContext(ctx)
}
func TestPDFMultipartHandlersRequireIdempotencyAndUseService(t *testing.T) {
	stub := &contentDeliveryStub{}
	h := &Handler{contentDelivery: stub}
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{{http.MethodPost, "/api/admin/attachment-library/uploads", `{"file_name":"x.pdf","name":"x","size":1,"sha256":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}`, 201}, {http.MethodPut, "/api/admin/attachment-library/uploads/11/parts/1", `{"sha256":"sha256:0000000000000000000000000000000000000000000000000000000000000000","content":""}`, 204}, {http.MethodPost, "/api/admin/attachment-library/uploads/11/complete", ``, 200}} {
		w := httptest.NewRecorder()
		r := contentRequest(t, tc.method, tc.path, tc.body)
		if tc.method == http.MethodPut {
			r.SetPathValue("upload_id", "11")
			r.SetPathValue("part_number", "1")
		}
		if strings.HasSuffix(tc.path, "complete") {
			r.SetPathValue("upload_id", "11")
		}
		switch tc.method {
		case http.MethodPut:
			h.PDFMultipartPart(w, r)
		case http.MethodPost:
			if strings.HasSuffix(tc.path, "complete") {
				h.PDFMultipartComplete(w, r)
			} else {
				h.PDFMultipartInitiate(w, r)
			}
		}
		if w.Code != tc.want {
			t.Fatalf("%s=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
	if !stub.initiated || !stub.part || !stub.completed {
		t.Fatal("service calls missing")
	}
}

func TestPDFMultipartInitiateDecodesOpenAPIFileName(t *testing.T) {
	stub := &contentDeliveryStub{}
	handler := &Handler{contentDelivery: stub}
	response := httptest.NewRecorder()
	request := contentRequest(t, http.MethodPost, "/api/admin/attachment-library/uploads", `{"file_name":"uat.pdf","name":"UAT","description":"disabled test","size":1872,"sha256":"sha256:0000000000000000000000000000000000000000000000000000000000000000","enabled":false,"actor":999,"idempotency_key":"untrusted-body-key"}`)
	handler.PDFMultipartInitiate(response, request)
	want := mediaport.AttachmentUploadInitiateCommand{FileName: "uat.pdf", Name: "UAT", Description: "disabled test", Size: 1872, SHA256: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Enabled: false, Actor: 7, IdempotencyKey: "content-delivery-key-0001"}
	if response.Code != http.StatusCreated || stub.initiateCommand != want {
		t.Fatalf("status=%d command=%+v want=%+v", response.Code, stub.initiateCommand, want)
	}
}
