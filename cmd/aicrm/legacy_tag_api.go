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

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type legacyTagBody struct {
	GroupName      string          `json:"group_name"`
	FirstTagName   string          `json:"first_tag_name"`
	GroupID        int64           `json:"group_id"`
	TagName        string          `json:"tag_name"`
	Actor          json.RawMessage `json:"actor"`
	IdempotencyKey string          `json:"idempotency_key"`
	TraceID        string          `json:"trace_id"`
	DryRun         bool            `json:"dry_run"`
}

func (handler *Handler) ListLegacyTags(w http.ResponseWriter, r *http.Request) {
	if handler == nil || nilLegacyDependency(handler.legacyTags) || r == nil {
		writeLegacyTagCatalog(w, contactapp.LegacyTagCatalog{}, true)
		return
	}
	catalog, err := handler.legacyTags.List(r.Context())
	if err != nil {
		writeLegacyTagCatalog(w, contactapp.LegacyTagCatalog{}, true)
		return
	}
	writeLegacyTagCatalog(w, catalog, false)
}

func writeLegacyTagCatalog(w http.ResponseWriter, c contactapp.LegacyTagCatalog, degraded bool) {
	groups := make([]map[string]any, 0, len(c.Groups))
	byID := make(map[int64]map[string]any, len(c.Groups))
	for _, g := range c.Groups {
		item := map[string]any{"group_id": g.ID, "group_name": g.Name, "name": g.Name, "sort_order": g.SortOrder, "tags": []any{}}
		groups = append(groups, item)
		byID[g.ID] = item
	}
	tags := make([]map[string]any, 0, len(c.Tags))
	for _, t := range c.Tags {
		item := map[string]any{"tag_id": t.ID, "id": t.ID, "group_id": t.GroupID, "group_name": t.GroupName, "tag_name": t.Name, "name": t.Name, "sort_order": t.SortOrder}
		tags = append(tags, item)
		if g := byID[t.GroupID]; g != nil {
			g["tags"] = append(g["tags"].([]any), item)
		}
	}
	payload := map[string]any{"ok": true, "items": tags, "tags": tags, "groups": groups, "count": len(tags), "total_tags": len(tags), "tag_limit": contactapp.LegacyTagLimit, "synced_at": c.SyncedAt, "source_status": "local_catalog", "read_model_status": "ready", "route_owner": "ai_crm_next", "fallback_used": false, "real_external_call_executed": false, "sync_executed": false, "fixture_used": false}
	if degraded {
		payload["groups"] = []any{}
		payload["tags"] = []any{}
		payload["items"] = []any{}
		payload["count"] = 0
		payload["total_tags"] = 0
		payload["error_code"] = "production_read_unavailable"
		payload["source_status"] = "production_unavailable"
		payload["read_model_status"] = "unavailable"
		payload["synced_at"] = ""
	}
	writeJSON(w, http.StatusOK, payload)
}

func (handler *Handler) CreateLegacyTagGroup(w http.ResponseWriter, r *http.Request) {
	body, c, ok := handler.legacyTagCommand(w, r)
	if !ok {
		return
	}
	if body.GroupName == "" || body.FirstTagName == "" {
		writeLegacyTagError(w, contactapp.ErrInvalidLegacyTag)
		return
	}
	if body.DryRun {
		writeLegacyTagSuccess(w, "group_create_validated", nil, true)
		return
	}
	g, t, err := handler.legacyTags.CreateGroup(r.Context(), c)
	if err != nil {
		writeLegacyTagError(w, err)
		return
	}
	writeLegacyTagSuccess(w, "group_created", map[string]any{"group": g, "tag": t}, false)
}
func (handler *Handler) MutateLegacyTagGroup(w http.ResponseWriter, r *http.Request) {
	body, c, ok := handler.legacyTagCommand(w, r)
	if !ok {
		return
	}
	id, e := parseLegacyTagID(chi.URLParam(r, "group_id"))
	if e != nil {
		writeLegacyTagError(w, e)
		return
	}
	c.GroupID = id
	if r.Method == http.MethodDelete {
		if body.DryRun {
			writeLegacyTagSuccess(w, "group_archive_validated", nil, true)
			return
		}
		g, e := handler.legacyTags.ArchiveGroup(r.Context(), c)
		if e != nil {
			writeLegacyTagError(w, e)
			return
		}
		writeLegacyTagSuccess(w, "group_archived", map[string]any{"group": g}, false)
		return
	}
	if body.GroupName == "" {
		writeLegacyTagError(w, contactapp.ErrInvalidLegacyTag)
		return
	}
	if body.DryRun {
		writeLegacyTagSuccess(w, "group_update_validated", nil, true)
		return
	}
	g, e := handler.legacyTags.UpdateGroup(r.Context(), c)
	if e != nil {
		writeLegacyTagError(w, e)
		return
	}
	writeLegacyTagSuccess(w, "group_updated", map[string]any{"group": g}, false)
}
func (handler *Handler) CreateLegacyTag(w http.ResponseWriter, r *http.Request) {
	body, c, ok := handler.legacyTagCommand(w, r)
	if !ok {
		return
	}
	if body.GroupID < 1 || body.GroupName == "" || body.TagName == "" {
		writeLegacyTagError(w, contactapp.ErrInvalidLegacyTag)
		return
	}
	if body.DryRun {
		writeLegacyTagSuccess(w, "tag_create_validated", nil, true)
		return
	}
	t, e := handler.legacyTags.CreateTag(r.Context(), c)
	if e != nil {
		writeLegacyTagError(w, e)
		return
	}
	writeLegacyTagSuccess(w, "tag_created", map[string]any{"tag": t}, false)
}
func (handler *Handler) MutateLegacyTag(w http.ResponseWriter, r *http.Request) {
	body, c, ok := handler.legacyTagCommand(w, r)
	if !ok {
		return
	}
	id, e := parseLegacyTagID(chi.URLParam(r, "tag_id"))
	if e != nil {
		writeLegacyTagError(w, e)
		return
	}
	c.TagID = id
	if r.Method == http.MethodDelete {
		if body.DryRun {
			writeLegacyTagSuccess(w, "tag_archive_validated", nil, true)
			return
		}
		t, e := handler.legacyTags.ArchiveTag(r.Context(), c)
		if e != nil {
			writeLegacyTagError(w, e)
			return
		}
		writeLegacyTagSuccess(w, "tag_archived", map[string]any{"tag": t}, false)
		return
	}
	if body.TagName == "" {
		writeLegacyTagError(w, contactapp.ErrInvalidLegacyTag)
		return
	}
	if body.DryRun {
		writeLegacyTagSuccess(w, "tag_update_validated", nil, true)
		return
	}
	t, e := handler.legacyTags.UpdateTag(r.Context(), c)
	if e != nil {
		writeLegacyTagError(w, e)
		return
	}
	writeLegacyTagSuccess(w, "tag_updated", map[string]any{"tag": t}, false)
}

func (handler *Handler) legacyTagCommand(w http.ResponseWriter, r *http.Request) (legacyTagBody, contactapp.LegacyTagCommand, bool) {
	if handler == nil || nilLegacyDependency(handler.legacyTags) || r == nil {
		writeLegacyTagError(w, contactapp.ErrLegacyTagUnavailable)
		return legacyTagBody{}, contactapp.LegacyTagCommand{}, false
	}
	p, ok := authport.PrincipalFromContext(r.Context())
	if !ok || p.AdminUserID < 1 {
		writeLegacyTagError(w, authport.ErrUnauthorized)
		return legacyTagBody{}, contactapp.LegacyTagCommand{}, false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	var b legacyTagBody
	if d.Decode(&b) != nil || !errors.Is(d.Decode(&struct{}{}), io.EOF) {
		writeLegacyTagError(w, contactapp.ErrInvalidLegacyTag)
		return b, contactapp.LegacyTagCommand{}, false
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(b.IdempotencyKey)
	}
	if key == "" {
		var v [16]byte
		if _, e := rand.Read(v[:]); e != nil {
			writeLegacyTagError(w, contactapp.ErrLegacyTagUnavailable)
			return b, contactapp.LegacyTagCommand{}, false
		}
		key = "legacy-tag:" + hex.EncodeToString(v[:])
	}
	return b, contactapp.LegacyTagCommand{Actor: p.AdminUserID, IdempotencyKey: key, TraceID: b.TraceID, GroupName: b.GroupName, FirstTagName: b.FirstTagName, GroupID: b.GroupID, TagName: b.TagName}, true
}
func parseLegacyTagID(raw string) (int64, error) {
	id, e := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if e != nil || id < 1 {
		return 0, contactapp.ErrLegacyTagNotFound
	}
	return id, nil
}
func writeLegacyTagSuccess(w http.ResponseWriter, reason string, result map[string]any, dry bool) {
	p := map[string]any{"ok": true, "reason": reason, "source_status": "local_catalog", "route_owner": "ai_crm_next", "fallback_used": false, "real_external_call_executed": false, "sync_executed": false, "fixture_used": false, "dry_run": dry}
	for k, v := range result {
		p[k] = v
	}
	writeJSON(w, http.StatusOK, p)
}
func writeLegacyTagError(w http.ResponseWriter, e error) {
	status, code := http.StatusServiceUnavailable, "production_unavailable"
	if errors.Is(e, contactapp.ErrInvalidLegacyTag) {
		status, code = http.StatusBadRequest, "input_error"
	} else if errors.Is(e, contactapp.ErrLegacyTagNotFound) {
		status, code = http.StatusNotFound, "not_found"
	} else if errors.Is(e, authport.ErrUnauthorized) {
		status, code = http.StatusForbidden, "unauthorized"
	}
	platformhttp.MarkCompatibilityError(w, platformhttp.CodeDependencyUnavailable)
	writeJSON(w, status, map[string]any{"ok": false, "error_code": code, "detail": e.Error(), "source_status": "local_catalog_error", "route_owner": "ai_crm_next", "fallback_used": false, "real_external_call_executed": false, "sync_executed": false, "fixture_used": false})
}
