package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type thumbnailCorp struct{}

func (thumbnailCorp) CorpID(context.Context) (string, error) { return "corp-1", nil }

type thumbnailIdentity struct{}

func (thumbnailIdentity) ResolveOrCreate(context.Context, identityport.IDRef) (identityport.ResolveResult, error) {
	return identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 41}, nil
}

func thumbnailViewerPrincipal() authport.Principal {
	staffID := int64(7)
	return authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin, StaffID: &staffID}
}

type thumbnailPhones struct{}

func (thumbnailPhones) BindPhone(context.Context, sidebarapp.PhoneBindingCommand) (string, error) {
	return "already_bound", nil
}

type thumbnailProfiles struct{ profile contactport.SidebarProfile }

func (profiles thumbnailProfiles) ResolveSidebarProfile(context.Context, contactport.CustomerID) (contactport.SidebarProfile, error) {
	return profiles.profile, nil
}
func (profiles thumbnailProfiles) ReadSidebarProfile(context.Context, contactport.CustomerID, int64) (contactport.SidebarProfile, error) {
	return profiles.profile, nil
}
func (profiles thumbnailProfiles) UpdateSidebarProfile(context.Context, contactport.SidebarProfileUpdateCommand) (contactport.SidebarProfile, error) {
	return profiles.profile, nil
}

type thumbnailSurveys struct{}

func (thumbnailSurveys) ListCustomerSurveyAnswers(context.Context, contactport.CustomerID, int32) (surveyport.CustomerSurveyAnswerPage, error) {
	return surveyport.CustomerSurveyAnswerPage{}, nil
}

type thumbnailOrders struct{}

func (thumbnailOrders) List(context.Context, orderport.Filter) (orderport.Page, error) {
	return orderport.Page{}, nil
}

type thumbnailMembers struct{}

func (thumbnailMembers) Get(context.Context, int64, string) (sidebarapp.PeriodicMember, error) {
	return sidebarapp.PeriodicMember{}, sidebarapp.ErrNotFound
}
func (thumbnailMembers) UpdateRemark(context.Context, sidebarapp.PeriodicRemarkCommand) (sidebarapp.PeriodicMember, error) {
	return sidebarapp.PeriodicMember{}, sidebarapp.ErrNotFound
}
func (thumbnailMembers) ListCustomer(context.Context, sidebarapp.PeriodicListQuery) (sidebarapp.PeriodicListResult, error) {
	return sidebarapp.PeriodicListResult{}, nil
}

type thumbnailMedia struct {
	exists  bool
	variant mediaport.ImageVariant
}

func (*thumbnailMedia) ListImages(context.Context, mediaport.ImageListQuery) (mediaport.ImageListPage, error) {
	return mediaport.ImageListPage{}, nil
}
func (*thumbnailMedia) Facets(context.Context) (mediaport.ImageFacets, error) {
	return mediaport.ImageFacets{}, nil
}
func (media *thumbnailMedia) LocalImageExists(context.Context, int64) (bool, error) {
	return media.exists, nil
}
func (media *thumbnailMedia) GetImageVariant(context.Context, int64, string) (mediaport.ImageVariant, error) {
	return media.variant, nil
}

func TestThumbnailStatusHTTPReturnsOnlyPendingOrNotFound(t *testing.T) {
	principal := thumbnailViewerPrincipal()
	profiles := thumbnailProfiles{contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: 7, Name: "customer", UpdatedAt: time.Now().UTC()}}
	media := &thumbnailMedia{exists: true, variant: mediaport.ImageVariant{Content: []byte("png"), MediaType: "image/png", ETag: `"thumb"`}}
	service, err := sidebarapp.NewService(thumbnailCorp{}, thumbnailIdentity{}, thumbnailPhones{}, profiles, thumbnailSurveys{}, thumbnailOrders{}, thumbnailMembers{}, media, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	minted, err := service.MintContext(context.Background(), principal, authport.SessionRef("sidebar-test-session"), true, "wm_external_41")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authport.WithAuthenticatedSession(context.Background(), principal, "sidebar-test-session")
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	call := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ThumbnailStatus(response, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/materials/image/31/thumbnail", nil).WithContext(ctx), minted.Token, 31)
		return response
	}

	ready := call()
	var payload struct {
		Status string `json:"status"`
	}
	if err = json.Unmarshal(ready.Body.Bytes(), &payload); err != nil || ready.Code != http.StatusAccepted || ready.Header().Get("X-Thumbnail-Status") != "pending" || payload.Status != "pending" {
		t.Fatalf("existing response=%d headers=%v body=%s err=%v", ready.Code, ready.Header(), ready.Body.String(), err)
	}
	media.exists = false
	missing := call()
	if missing.Code != http.StatusNotFound || missing.Header().Get("X-Thumbnail-Status") != "" {
		t.Fatalf("missing response=%d headers=%v body=%s", missing.Code, missing.Header(), missing.Body.String())
	}
}

func TestBootstrapHTTPReturnsWorkbenchOnlyForAuthorizedViewer(t *testing.T) {
	principal := thumbnailViewerPrincipal()
	profiles := thumbnailProfiles{contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: 7, Name: "customer-private", UpdatedAt: time.Now().UTC()}}
	service, err := sidebarapp.NewService(thumbnailCorp{}, thumbnailIdentity{}, thumbnailPhones{}, profiles, thumbnailSurveys{}, thumbnailOrders{}, thumbnailMembers{}, &thumbnailMedia{}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	call := func(ctx context.Context) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/sidebar/v2/bootstrap", strings.NewReader(`{"external_userid":"wm_external_41"}`)).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.Bootstrap(response, request)
		return response
	}

	unauthenticated := call(context.Background())
	if unauthenticated.Code != http.StatusOK || !strings.Contains(unauthenticated.Body.String(), `"state":"viewer_session_required"`) {
		t.Fatalf("unauthenticated response=%d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}
	for _, forbidden := range []string{"customer-private", `"customer_id"`, `"owner_staff_id"`, `"context_token"`, `"workbench"`} {
		if strings.Contains(unauthenticated.Body.String(), forbidden) {
			t.Fatalf("unauthenticated response leaked %q: %s", forbidden, unauthenticated.Body.String())
		}
	}

	authorizedContext := authport.WithAuthenticatedSession(context.Background(), principal, "sidebar-test-session")
	ready := call(authorizedContext)
	if ready.Code != http.StatusOK || !strings.Contains(ready.Body.String(), `"state":"ready"`) || !strings.Contains(ready.Body.String(), `"context_token"`) || !strings.Contains(ready.Body.String(), `"workbench"`) || !strings.Contains(ready.Body.String(), "customer-private") {
		t.Fatalf("ready response=%d body=%s", ready.Code, ready.Body.String())
	}
}

func TestThumbnailPreviewHTTPReturnsLocalBytesAndETag(t *testing.T) {
	principal := thumbnailViewerPrincipal()
	profiles := thumbnailProfiles{contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: 7, Name: "customer", UpdatedAt: time.Now().UTC()}}
	media := &thumbnailMedia{exists: true, variant: mediaport.ImageVariant{Content: []byte("image-bytes"), MediaType: "image/png", ETag: `"thumb-etag"`}}
	service, err := sidebarapp.NewService(thumbnailCorp{}, thumbnailIdentity{}, thumbnailPhones{}, profiles, thumbnailSurveys{}, thumbnailOrders{}, thumbnailMembers{}, media, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	minted, err := service.MintContext(context.Background(), principal, authport.SessionRef("sidebar-test-session"), true, "wm_external_41")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authport.WithAuthenticatedSession(context.Background(), principal, "sidebar-test-session")
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/materials/image/31/preview", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ThumbnailPreview(response, request, minted.Token, 31)
	if response.Code != http.StatusOK || response.Body.String() != "image-bytes" || response.Header().Get("Content-Type") != "image/png" || response.Header().Get("ETag") != `"thumb-etag"` {
		t.Fatalf("preview response=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}

	cachedRequest := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/materials/image/31/preview", nil).WithContext(ctx)
	cachedRequest.Header.Set("If-None-Match", `"thumb-etag"`)
	cached := httptest.NewRecorder()
	handler.ThumbnailPreview(cached, cachedRequest, minted.Token, 31)
	if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
		t.Fatalf("cached response=%d body=%q", cached.Code, cached.Body.String())
	}
}
