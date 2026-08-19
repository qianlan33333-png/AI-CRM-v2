package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type couponUpsertRequest struct {
	Name                 string                  `json:"name"`
	DiscountAmountTotal  int64                   `json:"discount_amount_total"`
	TotalIssueLimit      int64                   `json:"total_issue_limit"`
	PerUserIssueLimit    *int64                  `json:"per_user_issue_limit,omitempty"`
	ClaimStartsAt        time.Time               `json:"claim_starts_at"`
	ClaimEndsAt          time.Time               `json:"claim_ends_at"`
	ValidityMode         couponport.ValidityMode `json:"validity_mode"`
	UseStartsAt          *time.Time              `json:"use_starts_at"`
	UseEndsAt            *time.Time              `json:"use_ends_at"`
	RelativeValidityDays *int32                  `json:"relative_validity_days"`
	Instructions         *string                 `json:"instructions,omitempty"`
	TargetRefs           []string                `json:"target_refs"`
}

func (handler *Handler) ListCoupons(w http.ResponseWriter, r *http.Request) {
	if handler.coupons == nil {
		writeCouponError(w, couponapp.ErrUnavailable)
		return
	}
	limit, offset, search, status, err := couponPage(r)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	page, err := handler.coupons.List(r.Context(), limit, offset, search, status)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coupons": page.Items, "items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
}
func (handler *Handler) GetCoupon(w http.ResponseWriter, r *http.Request) {
	if handler.coupons == nil {
		writeCouponError(w, couponapp.ErrUnavailable)
		return
	}
	id, err := couponID(r)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	item, err := handler.coupons.Get(r.Context(), id)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coupon": item, "data": map[string]any{"coupon": item}})
}
func (handler *Handler) CreateCoupon(w http.ResponseWriter, r *http.Request) {
	if handler.coupons == nil {
		writeCouponError(w, couponapp.ErrUnavailable)
		return
	}
	command, err := couponCommand(w, r, 0)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	item, err := handler.coupons.Create(r.Context(), command)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coupon": item, "coupon_id": item.ID, "fallback_used": false, "create_replay_safe": false, "real_external_call_executed": false})
}
func (handler *Handler) UpdateCoupon(w http.ResponseWriter, r *http.Request) {
	if handler.coupons == nil {
		writeCouponError(w, couponapp.ErrUnavailable)
		return
	}
	id, err := couponID(r)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	command, err := couponCommand(w, r, id)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	item, err := handler.coupons.UpdateDraft(r.Context(), command)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coupon": item, "fallback_used": false, "real_external_call_executed": false})
}
func (handler *Handler) PublishCoupon(w http.ResponseWriter, r *http.Request) {
	handler.couponTransition(w, r, true)
}
func (handler *Handler) StopCoupon(w http.ResponseWriter, r *http.Request) {
	handler.couponTransition(w, r, false)
}
func (handler *Handler) couponTransition(w http.ResponseWriter, r *http.Request, publish bool) {
	if handler.coupons == nil {
		writeCouponError(w, couponapp.ErrUnavailable)
		return
	}
	id, err := couponID(r)
	if err != nil {
		writeCouponError(w, err)
		return
	}
	principal, ok := authport.PrincipalFromContext(r.Context())
	if !ok || principal.AdminUserID < 1 {
		writeCouponError(w, authport.ErrUnauthorized)
		return
	}
	operation := "stop"
	if publish {
		operation = "publish"
	}
	key := "coupon:" + operation + ":" + strconv.FormatInt(int64(id), 10)
	var item couponport.Coupon
	if publish {
		item, err = handler.coupons.Publish(r.Context(), id, principal.AdminUserID, key)
	} else {
		item, err = handler.coupons.Stop(r.Context(), id, principal.AdminUserID, key)
	}
	if err != nil {
		writeCouponError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coupon": item, "status": item.Status, "idempotent_same_state": true, "fallback_used": false, "real_external_call_executed": false})
}

func couponCommand(w http.ResponseWriter, r *http.Request, id couponport.ID) (couponport.UpsertCommand, error) {
	principal, ok := authport.PrincipalFromContext(r.Context())
	if !ok || principal.AdminUserID < 1 {
		return couponport.UpsertCommand{}, authport.ErrUnauthorized
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	var body couponUpsertRequest
	if decoder.Decode(&body) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return couponport.UpsertCommand{}, couponapp.ErrInvalidCoupon
	}
	perUser := int64(1)
	if body.PerUserIssueLimit != nil {
		perUser = *body.PerUserIssueLimit
	}
	instructions := ""
	if body.Instructions != nil {
		instructions = *body.Instructions
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return couponport.UpsertCommand{}, couponapp.ErrUnavailable
	}
	return couponport.UpsertCommand{Coupon: couponport.Coupon{ID: id, Name: body.Name, DiscountAmountTotal: body.DiscountAmountTotal, Currency: "CNY", TotalIssueLimit: body.TotalIssueLimit, PerUserIssueLimit: perUser, ClaimStartsAt: body.ClaimStartsAt, ClaimEndsAt: body.ClaimEndsAt, ValidityMode: body.ValidityMode, UseStartsAt: body.UseStartsAt, UseEndsAt: body.UseEndsAt, RelativeValidityDays: body.RelativeValidityDays, Instructions: instructions, TargetRefs: body.TargetRefs}, Actor: principal.AdminUserID, IdempotencyKey: "legacy-coupon:" + hex.EncodeToString(token[:])}, nil
}
func couponPage(r *http.Request) (int32, int32, string, string, error) {
	q := r.URL.Query()
	for key := range q {
		if key != "limit" && key != "offset" && key != "q" && key != "status" {
			return 0, 0, "", "", couponapp.ErrInvalidCoupon
		}
	}
	limit, offset := int64(couponapp.DefaultLimit), int64(0)
	var err error
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 32)
	}
	if err == nil {
		if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
			offset, err = strconv.ParseInt(raw, 10, 32)
		}
	}
	if err != nil || limit < 1 || limit > int64(couponapp.MaximumLimit) || offset < 0 || offset > int64(couponapp.MaximumOffset) {
		return 0, 0, "", "", couponapp.ErrInvalidCoupon
	}
	return int32(limit), int32(offset), q.Get("q"), q.Get("status"), nil
}
func couponID(r *http.Request) (couponport.ID, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "coupon_id")), 10, 64)
	if err != nil || id < 1 {
		return 0, couponapp.ErrNotFound
	}
	return couponport.ID(id), nil
}
func writeCouponError(w http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, couponapp.ErrInvalidCoupon), errors.Is(err, couponapp.ErrInvalidTarget):
		status, code = http.StatusBadRequest, platformhttp.CodeMalformedRequest
	case errors.Is(err, couponapp.ErrNotFound):
		status, code = http.StatusNotFound, platformhttp.CodeNotFound
	case errors.Is(err, couponapp.ErrConflict), errors.Is(err, couponapp.ErrRulesFrozen), errors.Is(err, couponapp.ErrNotClaimable):
		status, code = http.StatusConflict, platformhttp.CodeConflict
	case errors.Is(err, authport.ErrUnauthenticated):
		status, code = http.StatusUnauthorized, platformhttp.CodeUnauthenticated
	case errors.Is(err, authport.ErrUnauthorized):
		status, code = http.StatusForbidden, platformhttp.CodeUnauthorized
	}
	platformhttp.MarkCompatibilityError(w, code)
	writeJSON(w, status, map[string]any{"ok": false, "detail": err.Error(), "message": err.Error()})
}
