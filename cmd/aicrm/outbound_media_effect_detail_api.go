package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type outboundMediaEffectDetailApplication interface {
	ReadOutboundMediaEffectDetail(context.Context, int64, string) (mediaapp.OutboundMediaEffectDetail, error)
}

func (h *Handler) GetOutboundMediaEffectDetail(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.outboundMediaDetail == nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, mediaapp.ErrContentDeliveryUnavailable))
		return
	}
	contentPackageID, err := strconv.ParseInt(r.PathValue("content_package_id"), 10, 64)
	targetRef := r.PathValue("target_ref")
	if err != nil || contentPackageID < 1 || !validOutboundMediaTargetRef(targetRef) {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrOutboundMediaEffectDetailInvalid))
		return
	}
	detail, err := h.outboundMediaDetail.ReadOutboundMediaEffectDetail(r.Context(), contentPackageID, targetRef)
	if err != nil {
		code := platformhttp.CodeNotFound
		if errors.Is(err, mediaapp.ErrOutboundMediaEffectDetailInvalid) {
			code = platformhttp.CodeMalformedRequest
		}
		platformhttp.WriteError(w, r, platformhttp.NewError(code, err))
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func validOutboundMediaTargetRef(value string) bool {
	if len(value) == 0 || len(value) > 512 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}
