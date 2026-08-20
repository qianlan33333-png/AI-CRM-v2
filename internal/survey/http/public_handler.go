// Package surveyhttp supplies native Survey HTTP handlers. Routing and OAS
// registration intentionally remain with the Lane E integration owner.
package surveyhttp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const anonymousCookie = "aicrm_survey_anon"

type PublicService interface {
	Definition(context.Context, string) (surveyport.PublicQuestionnaire, error)
	Submit(context.Context, surveyport.PublicSubmissionCommand) (surveyport.PublicSubmissionReceipt, string, error)
	Result(context.Context, string) (surveyport.PublicSubmissionResult, error)
	Publish(context.Context, surveyport.PublishPublicDefinitionCommand) (surveyapp.PublicDefinitionRecord, error)
	Disable(context.Context, surveyport.DisablePublicDefinitionCommand) (surveyapp.PublicDefinitionRecord, error)
	Analytics(context.Context, surveyport.ID, int64) (surveyport.PublicAnalytics, error)
}

type PublicHandler struct {
	Service   PublicService
	CookieKey [32]byte
	// AbuseKey is injected from the server-side Survey configuration. It never
	// reaches the browser and is domain-separated from result/cookie keys.
	AbuseKey [32]byte
	Now      func() time.Time
}

func NewPublicHandler(service PublicService, cookieKey, abuseKey [32]byte) *PublicHandler {
	return &PublicHandler{Service: service, CookieKey: cookieKey, AbuseKey: abuseKey, Now: time.Now}
}

func (h *PublicHandler) GetDefinition(w http.ResponseWriter, r *http.Request, slug string) {
	h.headers(w)
	if r.Method != http.MethodGet {
		method(w, http.MethodGet)
		return
	}
	if !surveyapp.ValidPublicSlug(slug) {
		h.reply(w, nil, surveyapp.ErrInvalidPublicInput)
		return
	}
	out, err := h.Service.Definition(r.Context(), slug)
	h.reply(w, out, err)
}
func (h *PublicHandler) Submit(w http.ResponseWriter, r *http.Request, slug string) {
	h.headers(w)
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	if !surveyapp.ValidPublicSlug(slug) {
		h.reply(w, nil, surveyapp.ErrInvalidPublicInput)
		return
	}
	var body struct {
		Version       int64                               `json:"version"`
		SubmissionKey string                              `json:"submission_key"`
		Answers       []surveyport.PublicSubmissionAnswer `json:"answers"`
	}
	if !decode(w, r, &body) {
		return
	}
	anon, rate, err := h.anonymous(w, r)
	if err != nil {
		h.reply(w, nil, surveyapp.ErrPublicUnavailable)
		return
	}
	receipt, token, err := h.Service.Submit(r.Context(), surveyport.PublicSubmissionCommand{Slug: slug, Version: body.Version, SubmissionKey: body.SubmissionKey, AnonymousDigest: anon, RateDigest: rate, Answers: body.Answers})
	if err != nil {
		h.reply(w, nil, err)
		return
	}
	write(w, http.StatusAccepted, struct {
		Receipt     surveyport.PublicSubmissionReceipt `json:"receipt"`
		ResultToken string                             `json:"result_token"`
	}{receipt, token})
}
func (h *PublicHandler) QueryResult(w http.ResponseWriter, r *http.Request) {
	h.headers(w)
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	var body struct {
		ResultToken string `json:"result_token"`
	}
	if !decode(w, r, &body) {
		return
	}
	out, err := h.Service.Result(r.Context(), body.ResultToken)
	h.reply(w, out, err)
}

// Admin methods deliberately receive the authenticated actor from the carrier.
// The shared router performs CSRF/capability enforcement before invoking them.
func (h *PublicHandler) Publish(w http.ResponseWriter, r *http.Request, questionnaireID surveyport.ID, actor int64) {
	h.adminHeaders(w)
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	if !singleIdempotencyKey(r) {
		h.adminReply(w, r, nil, surveyapp.ErrInvalidPublicInput)
		return
	}
	var body struct {
		ExpectedVersion int64 `json:"expected_questionnaire_version"`
	}
	if !decodeAdmin(w, r, &body) {
		return
	}
	out, err := h.Service.Publish(r.Context(), surveyport.PublishPublicDefinitionCommand{QuestionnaireID: questionnaireID, ExpectedQuestionnaireVersion: body.ExpectedVersion, Actor: actor, IdempotencyKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		h.adminReply(w, r, nil, err)
		return
	}
	write(w, http.StatusOK, managementView(out))
}
func (h *PublicHandler) Disable(w http.ResponseWriter, r *http.Request, questionnaireID surveyport.ID, actor int64) {
	h.adminHeaders(w)
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	if !singleIdempotencyKey(r) {
		h.adminReply(w, r, nil, surveyapp.ErrInvalidPublicInput)
		return
	}
	var body struct {
		ExpectedVersion int64 `json:"expected_definition_version"`
	}
	if !decodeAdmin(w, r, &body) {
		return
	}
	out, err := h.Service.Disable(r.Context(), surveyport.DisablePublicDefinitionCommand{QuestionnaireID: questionnaireID, ExpectedDefinitionVersion: body.ExpectedVersion, Actor: actor, IdempotencyKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		h.adminReply(w, r, nil, err)
		return
	}
	write(w, http.StatusOK, managementView(out))
}
func (h *PublicHandler) Analytics(w http.ResponseWriter, r *http.Request, questionnaireID surveyport.ID, version int64) {
	h.adminHeaders(w)
	if r.Method != http.MethodGet {
		method(w, http.MethodGet)
		return
	}
	out, err := h.Service.Analytics(r.Context(), questionnaireID, version)
	h.adminReply(w, r, out, err)
}
func (h *PublicHandler) Page(w http.ResponseWriter, r *http.Request, slug string) {
	h.headers(w)
	if r.Method != http.MethodGet {
		method(w, http.MethodGet)
		return
	}
	if !surveyapp.ValidPublicSlug(slug) {
		h.reply(w, nil, surveyapp.ErrInvalidPublicInput)
		return
	}
	// The actual web bundle is mounted by the existing SPA. Never manufacture a
	// second asset path here (or expose result tokens in a carrier document).
	w.Header().Set("Location", "/?public_survey_slug="+slug)
	w.WriteHeader(http.StatusFound)
}
func (h *PublicHandler) headers(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
func (h *PublicHandler) adminHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
}
func managementView(record surveyapp.PublicDefinitionRecord) struct {
	QuestionnaireID   surveyport.ID `json:"questionnaire_id"`
	Slug              string        `json:"slug"`
	DefinitionVersion int64         `json:"definition_version"`
	State             string        `json:"state"`
} {
	return struct {
		QuestionnaireID   surveyport.ID `json:"questionnaire_id"`
		Slug              string        `json:"slug"`
		DefinitionVersion int64         `json:"definition_version"`
		State             string        `json:"state"`
	}{record.View.ID, record.View.Slug, record.View.Version, record.State}
}
func (h *PublicHandler) anonymous(w http.ResponseWriter, r *http.Request) ([32]byte, [32]byte, error) {
	if h == nil || h.CookieKey == [32]byte{} || h.AbuseKey == [32]byte{} {
		return [32]byte{}, [32]byte{}, errors.New("survey anonymous digest key is unavailable")
	}
	var raw string
	if c, err := r.Cookie(anonymousCookie); err == nil {
		raw = c.Value
	}
	if raw == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return [32]byte{}, [32]byte{}, err
		}
		raw = base64.RawURLEncoding.EncodeToString(b)
		http.SetCookie(w, &http.Cookie{Name: anonymousCookie, Value: raw, Path: "/api/public", MaxAge: int((30 * 24 * time.Hour).Seconds()), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return [32]byte{}, [32]byte{}, errors.New("invalid anonymous cookie")
	}
	mac := hmac.New(sha256.New, h.CookieKey[:])
	_, _ = mac.Write([]byte("aicrm.survey.public.anon.v1\x00" + raw))
	var cookieDigest [32]byte
	copy(cookieDigest[:], mac.Sum(nil))
	ip, err := controlledRemoteIP(r)
	if err != nil {
		return [32]byte{}, [32]byte{}, err
	}
	mac = hmac.New(sha256.New, h.AbuseKey[:])
	_, _ = mac.Write([]byte("aicrm.survey.public.source.v1\x00" + ip.String()))
	var sourceDigest [32]byte
	copy(sourceDigest[:], mac.Sum(nil))
	return cookieDigest, sourceDigest, nil
}
func controlledRemoteIP(r *http.Request) (netip.Addr, error) {
	if r == nil {
		return netip.Addr{}, errors.New("untrusted remote address")
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, errors.New("untrusted remote address")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, errors.New("untrusted remote address")
	}
	ip = ip.Unmap()
	if !ip.IsLoopback() {
		// Direct clients cannot influence their rate key with forwarding headers.
		return ip, nil
	}
	// The frozen production topology has one same-host nginx TLS terminator.
	// Only that loopback peer is trusted, and nginx's appended right-most XFF
	// hop wins over any attacker-supplied prefix. Missing/malformed forwarding
	// data fails closed rather than collapsing every public user into 127.0.0.1.
	values := r.Header.Values("X-Forwarded-For")
	if len(values) != 1 {
		return netip.Addr{}, errors.New("trusted proxy forwarding address is unavailable")
	}
	hops := strings.Split(values[0], ",")
	client, err := netip.ParseAddr(strings.TrimSpace(hops[len(hops)-1]))
	if err != nil || !client.IsValid() || client.IsUnspecified() || client.IsMulticast() {
		return netip.Addr{}, errors.New("trusted proxy forwarding address is invalid")
	}
	return client.Unmap(), nil
}
func (h *PublicHandler) reply(w http.ResponseWriter, out any, err error) {
	if err == nil {
		write(w, http.StatusOK, out)
		return
	}
	switch {
	case errors.Is(err, surveyapp.ErrInvalidPublicInput):
		write(w, http.StatusBadRequest, map[string]string{"code": "invalid_public_input"})
	case errors.Is(err, surveyapp.ErrNotFound):
		write(w, http.StatusNotFound, map[string]string{"code": "not_found"})
	case errors.Is(err, surveyapp.ErrConflict):
		write(w, http.StatusConflict, map[string]string{"code": "idempotency_conflict"})
	case errors.Is(err, surveyapp.ErrPublicRateLimited):
		write(w, http.StatusTooManyRequests, map[string]string{"code": "rate_limited"})
	default:
		write(w, http.StatusServiceUnavailable, map[string]string{"code": "unavailable"})
	}
}
func (h *PublicHandler) adminReply(w http.ResponseWriter, r *http.Request, out any, err error) {
	if err == nil {
		write(w, http.StatusOK, out)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, surveyapp.ErrInvalidPublicInput):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, surveyapp.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, surveyapp.ErrConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(w, r, platformhttp.NewError(code, err))
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	de := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	de.DisallowUnknownFields()
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || strings.ToLower(mediaType) != "application/json" {
		write(w, http.StatusBadRequest, map[string]string{"code": "invalid_public_input"})
		return false
	}
	if de.Decode(out) != nil || de.Decode(&struct{}{}) != io.EOF {
		write(w, http.StatusBadRequest, map[string]string{"code": "invalid_public_input"})
		return false
	}
	return true
}
func decodeAdmin(w http.ResponseWriter, r *http.Request, out any) bool {
	de := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	de.DisallowUnknownFields()
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || strings.ToLower(mediaType) != "application/json" || de.Decode(out) != nil || de.Decode(&struct{}{}) != io.EOF {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, surveyapp.ErrInvalidPublicInput))
		return false
	}
	return true
}
func singleIdempotencyKey(r *http.Request) bool {
	if r == nil {
		return false
	}
	values := r.Header.Values("Idempotency-Key")
	return len(values) == 1 && len(values[0]) >= 16 && len(values[0]) <= 128 && values[0] == strings.TrimSpace(values[0])
}
func write(w http.ResponseWriter, status int, out any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(out)
}
func method(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	write(w, http.StatusMethodNotAllowed, map[string]string{"code": "method_not_allowed"})
}
