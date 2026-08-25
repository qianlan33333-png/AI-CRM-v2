package surveyhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
)

func TestH5OAuthCallbackIssuesSignedCanonicalIdentityOnly(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	key := [32]byte{1}
	handler := NewH5OAuthHandler(h5HTTPService{}, key)
	handler.Now = func() time.Time { return now }
	response := httptest.NewRecorder()
	handler.Callback(response, httptest.NewRequest(http.MethodGet, "/api/h5/surveys/oauth/callback?state=s&code=c&external_userid=forged", nil))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/s/survey-1" {
		t.Fatalf("status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != h5IdentityCookie || cookies[0].HttpOnly != true || cookies[0].Secure != true {
		t.Fatalf("cookies=%+v", cookies)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/h5/surveys/survey-1/submit", nil)
	req.AddCookie(cookies[0])
	identity, err := handler.Identity(req)
	if err != nil || identity.CustomerID != 42 {
		t.Fatalf("Identity=%+v err=%v", identity, err)
	}
	req.Header.Set("Cookie", h5IdentityCookie+"=forged")
	if _, err := handler.Identity(req); err == nil {
		t.Fatal("forged identity cookie accepted")
	}
}

type h5HTTPService struct{}

func (h5HTTPService) Start(context.Context, string) (string, time.Time, error) {
	return "https://provider.example", time.Now(), nil
}
func (h5HTTPService) Callback(context.Context, string, string) (surveyapp.H5CanonicalIdentity, string, error) {
	return surveyapp.H5CanonicalIdentity{CustomerID: 42, ExpiresAt: time.Date(2026, 8, 25, 0, 5, 0, 0, time.UTC)}, "/s/survey-1", nil
}
