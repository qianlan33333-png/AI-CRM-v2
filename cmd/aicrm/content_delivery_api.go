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
type contentPDFPartRequest struct {
	SHA256  string `json:"sha256"`
	Content []byte `json:"content"`
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

func (h *Handler) PDFMultipartInitiate(w http.ResponseWriter, r *http.Request) {
	var command mediaport.AttachmentUploadInitiateCommand
	if h == nil || h.contentDelivery == nil || json.NewDecoder(r.Body).Decode(&command) != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrContentDeliveryInvalid))
		return
	}
	key, err := legacyAttachmentIdempotencyKey(r)
	actor, ok := legacyAttachmentPrincipal(r, authport.CapabilityMediaLibraryWrite)
	if err != nil || !ok {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrContentDeliveryInvalid))
		return
	}
	command.Actor, command.IdempotencyKey = actor, key
	id, err := h.contentDelivery.InitiatePDF(r.Context(), command)
	if err != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeConflict, err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"upload_id": id})
}

func (h *Handler) PDFMultipartPart(w http.ResponseWriter, r *http.Request) {
	var body contentPDFPartRequest
	if h == nil || h.contentDelivery == nil || json.NewDecoder(r.Body).Decode(&body) != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrContentDeliveryInvalid))
		return
	}
	key, err := legacyAttachmentIdempotencyKey(r)
	actor, ok := legacyAttachmentPrincipal(r, authport.CapabilityMediaLibraryWrite)
	uploadID, idErr := strconv.ParseInt(r.PathValue("upload_id"), 10, 64)
	part, partErr := strconv.ParseInt(r.PathValue("part_number"), 10, 32)
	if err != nil || !ok || idErr != nil || partErr != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrContentDeliveryInvalid))
		return
	}
	err = h.contentDelivery.PutPDFPart(r.Context(), mediaport.AttachmentUploadPartCommand{UploadID: uploadID, PartNumber: int32(part), SHA256: body.SHA256, Content: body.Content, Actor: actor, IdempotencyKey: key})
	if err != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeConflict, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PDFMultipartComplete(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.contentDelivery == nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeInternal, mediaapp.ErrContentDeliveryUnavailable))
		return
	}
	key, err := legacyAttachmentIdempotencyKey(r)
	actor, ok := legacyAttachmentPrincipal(r, authport.CapabilityMediaLibraryWrite)
	uploadID, idErr := strconv.ParseInt(r.PathValue("upload_id"), 10, 64)
	if err != nil || !ok || idErr != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrContentDeliveryInvalid))
		return
	}
	attachmentID, err := h.contentDelivery.CompletePDF(r.Context(), mediaport.AttachmentUploadCompleteCommand{UploadID: uploadID, Actor: actor, IdempotencyKey: key})
	if err != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeConflict, err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"attachment_id": attachmentID})
}
