package main

import (
	"encoding/json"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	"net/http"
	"strconv"
)

type contentPackageRequest struct {
	Name            string                 `json:"name"`
	ContentText     string                 `json:"content_text"`
	Enabled         bool                   `json:"enabled"`
	Refs            []mediaport.ContentRef `json:"refs"`
	ExpectedVersion int64                  `json:"expected_version"`
}

func (h *Handler) ContentPackagePreview(w http.ResponseWriter, r *http.Request) {
	var c mediaport.ContentPackageCommand
	if h == nil || h.contentDelivery == nil || json.NewDecoder(r.Body).Decode(&c) != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrContentDeliveryInvalid))
		return
	}
	c.Actor, _ = legacyAttachmentPrincipal(r, authport.CapabilityMediaLibraryWrite)
	v, e := h.contentDelivery.Preview(r.Context(), c)
	if e != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, e))
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) ContentPackageCreate(w http.ResponseWriter, r *http.Request) {
	h.contentPackageMutate(w, r, false)
}
func (h *Handler) ContentPackageUpdate(w http.ResponseWriter, r *http.Request) {
	h.contentPackageMutate(w, r, true)
}
func (h *Handler) contentPackageMutate(w http.ResponseWriter, r *http.Request, update bool) {
	var body contentPackageRequest
	if h == nil || h.contentDelivery == nil || json.NewDecoder(r.Body).Decode(&body) != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrContentDeliveryInvalid))
		return
	}
	c := mediaport.ContentPackageCommand{Name: body.Name, ContentText: body.ContentText, Enabled: body.Enabled, Refs: body.Refs}
	c.Actor, _ = legacyAttachmentPrincipal(r, authport.CapabilityMediaLibraryWrite)
	c.IdempotencyKey = r.Header.Get("Idempotency-Key")
	var v mediaport.ContentPackage
	var e error
	if update {
		id, _ := strconv.ParseInt(r.PathValue("package_id"), 10, 64)
		v, e = h.contentDelivery.Update(r.Context(), mediaport.ContentPackageUpdateCommand{ID: id, ExpectedVersion: body.ExpectedVersion, ContentPackageCommand: c})
	} else {
		v, e = h.contentDelivery.Create(r.Context(), c)
	}
	if e != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeConflict, e))
		return
	}
	writeJSON(w, http.StatusOK, v)
}
