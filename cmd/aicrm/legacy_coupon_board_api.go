package main

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	paymentIdentityCookieName = "aicrm_payment_identity"
	sidebarGrantCookieName    = "aicrm_sidebar_grant"
)

var couponPageTemplate = template.Must(template.New("coupon-compat").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>优惠券</title><main><h1>{{.Title}}</h1>{{if .Coupon}}<p>{{.Coupon.Name}}</p>{{end}}{{range .Items}}<p>{{.Name}} · {{.Status}}</p>{{end}}{{range .Claims}}<p>{{.ClaimRef}} · {{.Status}}</p>{{end}}</main></html>`))

func (h *Handler) CouponListPage(w http.ResponseWriter, r *http.Request) {
	page, e := h.coupons.List(r.Context(), 100, 0, "", "")
	if e != nil {
		writeCouponError(w, e)
		return
	}
	renderCouponPage(w, "优惠券", page.Items, couponport.Coupon{})
}
func (h *Handler) CouponNewPage(w http.ResponseWriter, _ *http.Request) {
	renderCouponPage(w, "新建优惠券", nil, couponport.Coupon{})
}
func (h *Handler) CouponEditPage(w http.ResponseWriter, r *http.Request) {
	item, e := h.coupons.Get(r.Context(), mustCouponID(r))
	if e != nil {
		writeCouponError(w, e)
		return
	}
	renderCouponPage(w, "编辑优惠券", nil, item)
}
func (h *Handler) CouponDataPage(w http.ResponseWriter, r *http.Request) {
	item, e := h.coupons.Get(r.Context(), mustCouponID(r))
	if e != nil {
		writeCouponError(w, e)
		return
	}
	b := h.boardOrFail(w)
	if b == nil {
		return
	}
	claims, e := b.ListClaims(r.Context(), item.ID, 100, 0)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	renderCouponDataPage(w, item, claims.Items)
}
func renderCouponPage(w http.ResponseWriter, title string, items []couponport.Coupon, item couponport.Coupon) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = couponPageTemplate.Execute(w, struct {
		Title  string
		Items  []couponport.Coupon
		Coupon couponport.Coupon
	}{Title: title, Items: items, Coupon: item})
}
func renderCouponDataPage(w http.ResponseWriter, item couponport.Coupon, claims []couponport.Claim) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = couponPageTemplate.Execute(w, struct {
		Title  string
		Items  []couponport.Coupon
		Coupon couponport.Coupon
		Claims []couponport.Claim
	}{Title: "优惠券领取数据", Coupon: item, Claims: claims})
}

func (h *Handler) CouponProductOptions(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.products == nil {
		writeCouponError(w, couponapp.ErrUnavailable)
		return
	}
	limit, offset, productType, q, e := couponProductOptionsPage(r)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	if productType == "service_period" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": []any{}, "total": 0, "limit": limit, "offset": offset})
		return
	}
	page, e := h.products.ListLegacy(r.Context(), limit, offset)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	items := make([]map[string]any, 0, len(page.Items))
	needle := strings.ToLower(q)
	for _, p := range page.Items {
		if needle != "" && !strings.Contains(strings.ToLower(p.Name), needle) {
			continue
		}
		items = append(items, map[string]any{"id": p.ID, "target_ref": "standard_product:" + strconv.FormatInt(int64(p.ID), 10), "name": p.Name, "price_minor": p.PriceMinor, "currency": p.Currency})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "total": len(items), "limit": page.Limit, "offset": page.Offset})
}
func (h *Handler) CouponClaims(w http.ResponseWriter, r *http.Request) {
	b := h.boardOrFail(w)
	if b == nil {
		return
	}
	limit, offset, _, _, e := couponPage(r)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	page, e := b.ListClaims(r.Context(), mustCouponID(r), limit, offset)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	items := make([]map[string]any, len(page.Items))
	for i, c := range page.Items {
		items[i] = map[string]any{"id": c.ID, "claim_ref": c.ClaimRef, "status": c.Status, "claimed_at": c.ClaimedAt}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
}
func (h *Handler) CouponShare(w http.ResponseWriter, r *http.Request) {
	item, e := h.coupons.Get(r.Context(), mustCouponID(r))
	if e != nil {
		writeCouponError(w, e)
		return
	}
	if item.Status != "published" {
		writeCouponError(w, couponapp.ErrConflict)
		return
	}
	slug := couponSlug(item.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "public_slug": slug, "url": "/c/" + slug})
}
func (h *Handler) CouponArchive(w http.ResponseWriter, r *http.Request) {
	h.boardMutation(w, r, "archive")
}
func (h *Handler) CouponDelete(w http.ResponseWriter, r *http.Request) {
	h.boardMutation(w, r, "delete")
}
func (h *Handler) CouponCopy(w http.ResponseWriter, r *http.Request) { h.boardMutation(w, r, "copy") }
func (h *Handler) boardMutation(w http.ResponseWriter, r *http.Request, op string) {
	b := h.boardOrFail(w)
	if b == nil {
		return
	}
	p, ok := authport.PrincipalFromContext(r.Context())
	if !ok || p.AdminUserID < 1 {
		writeCouponError(w, authport.ErrUnauthorized)
		return
	}
	key, e := couponIdempotencyKey(r)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	id := mustCouponID(r)
	var item couponport.Coupon
	switch op {
	case "archive":
		item, e = b.Archive(r.Context(), id, p.AdminUserID, key)
	case "delete":
		item, e = b.Delete(r.Context(), id, p.AdminUserID, key)
	case "copy":
		item, e = b.Copy(r.Context(), id, p.AdminUserID, key)
	}
	if e != nil {
		writeCouponError(w, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coupon": item})
}
func (h *Handler) H5AvailableCoupons(w http.ResponseWriter, r *http.Request) {
	b := h.boardOrFail(w)
	if b == nil {
		return
	}
	customer, e := h.paymentCustomer(r, b)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	target, e := h5AvailableTarget(r)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	items, e := b.ListAvailable(r.Context(), target, customer)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}
func (h *Handler) H5Coupon(w http.ResponseWriter, r *http.Request) {
	item, e := h.publicCoupon(r)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coupon": item})
}
func (h *Handler) H5ClaimCoupon(w http.ResponseWriter, r *http.Request) {
	b := h.boardOrFail(w)
	if b == nil {
		return
	}
	// This route uses an ambient HttpOnly payment cookie. The frozen H5
	// transport has no body CSRF token, so it accepts only an unequivocally
	// same-origin browser request and an empty body.
	if !sameOriginBrowserRequest(r) {
		writeCouponError(w, authport.ErrUnauthorized)
		return
	}
	if e := requireEmptyCouponBody(r); e != nil {
		writeCouponError(w, e)
		return
	}
	customer, e := h.paymentCustomer(r, b)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	item, e := h.publicCoupon(r)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	key, e := couponIdempotencyKey(r)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	claim, e := b.Claim(r.Context(), couponport.ClaimCommand{CouponID: item.ID, CustomerID: customer, IdempotencyKey: key})
	if e != nil {
		writeCouponError(w, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "claim_ref": claim.ClaimRef, "status": claim.Status, "claimed_at": claim.ClaimedAt})
}
func (h *Handler) PublicCouponPage(w http.ResponseWriter, r *http.Request) {
	item, e := h.publicCoupon(r)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	renderCouponPage(w, "优惠券", nil, item)
}
func (h *Handler) SidebarCoupons(w http.ResponseWriter, r *http.Request) {
	b := h.boardOrFail(w)
	if b == nil {
		return
	}
	customer, e := h.sidebarCustomer(r, b)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	items, e := b.ListSidebarCoupons(r.Context(), customer)
	if e != nil {
		writeCouponError(w, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}
func (h *Handler) boardOrFail(w http.ResponseWriter) couponBoardApplication {
	if h == nil || h.couponBoard == nil {
		writeCouponError(w, couponapp.ErrUnavailable)
		return nil
	}
	return h.couponBoard
}

// BindCouponPaymentIdentityAccount runs before AccountBudgetMiddleware so the
// public H5 routes can retain per-resolved-customer concurrency isolation
// without ever accepting a browser-provided customer identifier.
func (h *Handler) BindCouponPaymentIdentityAccount(next http.Handler) (http.Handler, error) {
	if h == nil || next == nil {
		return nil, couponapp.ErrUnavailable
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.couponBoard == nil {
			writeCouponError(w, couponapp.ErrUnavailable)
			return
		}
		customer, e := h.paymentCustomer(r, h.couponBoard)
		if e != nil {
			writeCouponError(w, e)
			return
		}
		ctx, e := platformhttp.ContextWithAccountID(r.Context(), "coupon-payment:"+strconv.FormatInt(customer, 10))
		if e != nil {
			writeCouponError(w, couponapp.ErrUnavailable)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	}), nil
}

func (h *Handler) BindCouponSidebarGrantAccount(next http.Handler) (http.Handler, error) {
	if h == nil || next == nil {
		return nil, couponapp.ErrUnavailable
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.couponBoard == nil {
			writeCouponError(w, couponapp.ErrUnavailable)
			return
		}
		customer, e := h.sidebarCustomer(r, h.couponBoard)
		if e != nil {
			writeCouponError(w, e)
			return
		}
		ctx, e := platformhttp.ContextWithAccountID(r.Context(), "coupon-sidebar:"+strconv.FormatInt(customer, 10))
		if e != nil {
			writeCouponError(w, couponapp.ErrUnavailable)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	}), nil
}
func (h *Handler) paymentCustomer(r *http.Request, b couponBoardApplication) (int64, error) {
	token, e := oneCouponBrowserToken(r, paymentIdentityCookieName)
	if e != nil {
		return 0, e
	}
	id, e := b.ResolvePaymentIdentitySession(r.Context(), token)
	if e != nil || id < 1 {
		return 0, authport.ErrUnauthenticated
	}
	return id, nil
}
func (h *Handler) sidebarCustomer(r *http.Request, b couponBoardApplication) (int64, error) {
	token, e := oneCouponBrowserToken(r, sidebarGrantCookieName)
	if e != nil {
		return 0, e
	}
	id, e := b.ResolveSidebarGrant(r.Context(), token)
	if e != nil || id < 1 {
		return 0, authport.ErrUnauthenticated
	}
	return id, nil
}
func oneCouponBrowserToken(r *http.Request, name string) (string, error) {
	if r == nil {
		return "", authport.ErrUnauthenticated
	}
	var token string
	for _, c := range r.Cookies() {
		if c.Name != name {
			continue
		}
		if token != "" || !validToken(c.Value) {
			return "", authport.ErrUnauthenticated
		}
		token = c.Value
	}
	if token == "" {
		return "", authport.ErrUnauthenticated
	}
	return token, nil
}
func (h *Handler) publicCoupon(r *http.Request) (couponport.Coupon, error) {
	id, e := parseCouponSlug(chi.URLParam(r, "public_slug"))
	if e != nil {
		return couponport.Coupon{}, couponapp.ErrNotFound
	}
	item, e := h.coupons.Get(r.Context(), id)
	if e != nil || item.Status != "published" {
		return couponport.Coupon{}, couponapp.ErrNotFound
	}
	return item, nil
}
func mustCouponID(r *http.Request) couponport.ID {
	id, e := couponID(r)
	if e != nil {
		return 0
	}
	return id
}
func couponSlug(id couponport.ID) string { return "c-" + strconv.FormatInt(int64(id), 10) }
func parseCouponSlug(raw string) (couponport.ID, error) {
	if !strings.HasPrefix(raw, "c-") {
		return 0, errors.New("bad slug")
	}
	id, e := strconv.ParseInt(strings.TrimPrefix(raw, "c-"), 10, 64)
	if e != nil || id < 1 {
		return 0, errors.New("bad slug")
	}
	return couponport.ID(id), nil
}
func couponIdempotencyKey(r *http.Request) (string, error) {
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 || len(values[0]) < 16 || len(values[0]) > 128 || strings.TrimSpace(values[0]) != values[0] {
		return "", couponapp.ErrInvalidCoupon
	}
	return values[0], nil
}
func couponProductOptionsPage(r *http.Request) (int32, int32, string, string, error) {
	q := r.URL.Query()
	for key := range q {
		if key != "limit" && key != "offset" && key != "q" && key != "product_type" {
			return 0, 0, "", "", couponapp.ErrInvalidCoupon
		}
	}
	limit, offset := int64(20), int64(0)
	var e error
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		limit, e = strconv.ParseInt(raw, 10, 32)
	}
	if e == nil {
		if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
			offset, e = strconv.ParseInt(raw, 10, 32)
		}
	}
	productType := strings.TrimSpace(q.Get("product_type"))
	if productType == "" {
		productType = "all"
	}
	query := strings.TrimSpace(q.Get("q"))
	if e != nil || limit < 1 || limit > 100 || offset < 0 || len(query) > 80 || productType != "all" && productType != "standard_product" && productType != "service_period" {
		return 0, 0, "", "", couponapp.ErrInvalidCoupon
	}
	return int32(limit), int32(offset), productType, query, nil
}
func h5AvailableTarget(r *http.Request) (string, error) {
	if r == nil || r.URL == nil {
		return "", couponapp.ErrInvalidTarget
	}
	q := r.URL.Query()
	if len(q) != 1 || len(q["target_ref"]) != 1 {
		return "", couponapp.ErrInvalidTarget
	}
	target := strings.TrimSpace(q.Get("target_ref"))
	if target == "" || len(target) > 200 || target != q.Get("target_ref") {
		return "", couponapp.ErrInvalidTarget
	}
	return target, nil
}
func requireEmptyCouponBody(r *http.Request) error {
	if r == nil || r.Body == nil {
		return nil
	}
	if r.ContentLength == 0 {
		return nil
	}
	read, e := io.CopyN(io.Discard, r.Body, 1)
	if e != nil && !errors.Is(e, io.EOF) {
		return couponapp.ErrInvalidCoupon
	}
	if read != 0 {
		return couponapp.ErrInvalidCoupon
	}
	return nil
}
