package main

import (
	"context"
	"encoding/json"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	"net/http"
)

type outboundMediaAcceptedApplication interface {
	AcceptPublishedContentPackageForOutbound(context.Context, int64, string, string) (eer.Projection, bool, error)
}
type outboundMediaAcceptedRequest struct {
	ContentPackageID int64  `json:"content_package_id"`
	TargetRef        string `json:"target_ref"`
}

func (h *Handler) AcceptOutboundMedia(w http.ResponseWriter, r *http.Request) {
	var body outboundMediaAcceptedRequest
	if h == nil || h.outboundMediaAccepted == nil || json.NewDecoder(r.Body).Decode(&body) != nil || body.ContentPackageID < 1 || body.TargetRef == "" {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrPublishedOutboundInvalid))
		return
	}
	p, replay, e := h.outboundMediaAccepted.AcceptPublishedContentPackageForOutbound(r.Context(), body.ContentPackageID, body.TargetRef, r.Header.Get("Idempotency-Key"))
	if e != nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeConflict, e))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"effect_id": p.ID, "state": p.State, "replay": replay})
}
