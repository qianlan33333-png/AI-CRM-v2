package surveyhttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
)

const h5IdentityCookie = "aicrm_survey_h5_identity"

// H5OAuthHandler is intentionally kept as a small edge adapter. The service
// owns state claiming and canonical identity resolution; this handler only
// issues a short-lived, signed browser proof after a successful callback.
type H5OAuthHandler struct {
	Service interface {
		Start(context.Context, string) (string, time.Time, error)
		Callback(context.Context, string, string) (surveyapp.H5CanonicalIdentity, string, error)
	}
	Key [32]byte
	Now func() time.Time
}

func NewH5OAuthHandler(service interface {
	Start(context.Context, string) (string, time.Time, error)
	Callback(context.Context, string, string) (surveyapp.H5CanonicalIdentity, string, error)
}, key [32]byte) *H5OAuthHandler {
	return &H5OAuthHandler{Service: service, Key: key, Now: time.Now}
}

func (h *H5OAuthHandler) Start(w http.ResponseWriter, r *http.Request) {
	h.headers(w)
	if r == nil || r.Method != http.MethodGet || h == nil || h.Service == nil {
		h.error(w, http.StatusBadRequest, "identity_required")
		return
	}
	next := r.URL.Query().Get("next")
	if len(r.URL.Query()["next"]) != 1 {
		h.error(w, http.StatusBadRequest, "identity_required")
		return
	}
	url, _, err := h.Service.Start(r.Context(), next)
	if err != nil {
		h.error(w, http.StatusServiceUnavailable, "identity_unavailable")
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *H5OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	h.headers(w)
	if r == nil || r.Method != http.MethodGet || h == nil || h.Service == nil || h.Key == [32]byte{} || h.Now == nil {
		h.error(w, http.StatusBadRequest, "identity_required")
		return
	}
	states, codes := r.URL.Query()["state"], r.URL.Query()["code"]
	if len(states) != 1 || len(codes) != 1 {
		h.error(w, http.StatusBadRequest, "identity_required")
		return
	}
	identity, next, err := h.Service.Callback(r.Context(), states[0], codes[0])
	if err != nil || identity.CustomerID < 1 || identity.ExpiresAt.Before(h.Now().UTC()) {
		h.error(w, http.StatusUnauthorized, "identity_required")
		return
	}
	value, err := h.encode(identity)
	if err != nil {
		h.error(w, http.StatusServiceUnavailable, "identity_unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: h5IdentityCookie, Value: value, Path: "/api/h5/surveys/", MaxAge: int(time.Until(identity.ExpiresAt).Seconds()), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, next, http.StatusFound)
}

func (h *H5OAuthHandler) Identity(r *http.Request) (surveyapp.H5CanonicalIdentity, error) {
	if h == nil || r == nil || h.Key == [32]byte{} || h.Now == nil {
		return surveyapp.H5CanonicalIdentity{}, surveyapp.ErrH5IdentityRequired
	}
	cookie, err := r.Cookie(h5IdentityCookie)
	if err != nil {
		return surveyapp.H5CanonicalIdentity{}, surveyapp.ErrH5IdentityRequired
	}
	return h.decode(cookie.Value)
}

func (h *H5OAuthHandler) encode(identity surveyapp.H5CanonicalIdentity) (string, error) {
	payload, err := json.Marshal(struct {
		CustomerID int64 `json:"customer_id"`
		ExpiresAt  int64 `json:"expires_at"`
	}{identity.CustomerID, identity.ExpiresAt.UTC().Unix()})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, h.Key[:])
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (h *H5OAuthHandler) decode(value string) (surveyapp.H5CanonicalIdentity, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return surveyapp.H5CanonicalIdentity{}, surveyapp.ErrH5IdentityRequired
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return surveyapp.H5CanonicalIdentity{}, surveyapp.ErrH5IdentityRequired
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(signature) != sha256.Size {
		return surveyapp.H5CanonicalIdentity{}, surveyapp.ErrH5IdentityRequired
	}
	mac := hmac.New(sha256.New, h.Key[:])
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return surveyapp.H5CanonicalIdentity{}, surveyapp.ErrH5IdentityRequired
	}
	var raw struct {
		CustomerID int64 `json:"customer_id"`
		ExpiresAt  int64 `json:"expires_at"`
	}
	if json.Unmarshal(payload, &raw) != nil || raw.CustomerID < 1 || raw.ExpiresAt < 1 {
		return surveyapp.H5CanonicalIdentity{}, surveyapp.ErrH5IdentityRequired
	}
	identity := surveyapp.H5CanonicalIdentity{CustomerID: raw.CustomerID, ExpiresAt: time.Unix(raw.ExpiresAt, 0).UTC()}
	if !identity.ExpiresAt.After(h.Now().UTC()) {
		return surveyapp.H5CanonicalIdentity{}, surveyapp.ErrH5IdentityRequired
	}
	return identity, nil
}

func (h *H5OAuthHandler) headers(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func (h *H5OAuthHandler) error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"code":"` + code + `"}\n`))
}
