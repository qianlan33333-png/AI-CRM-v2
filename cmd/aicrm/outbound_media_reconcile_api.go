package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type outboundMediaReconcileApplication interface {
	Reconcile(context.Context, mediaapp.OutboundMediaReconcileCommand) (mediaapp.OutboundMediaReconcileResult, error)
}

type outboundMediaReconcileRequest struct {
	Generation       int64     `json:"generation"`
	Fence            int64     `json:"fence"`
	LeaseExpiresAt   time.Time `json:"lease_expires_at"`
	EvidenceRef      string    `json:"evidence_ref"`
	ProviderAccepted bool      `json:"provider_accepted"`
	DeliveryProven   bool      `json:"delivery_proven"`
}

type outboundMediaReconcileResponse struct {
	EffectID         string `json:"effect_id"`
	State            string `json:"state"`
	ProviderAccepted bool   `json:"provider_accepted"`
	DeliveryProven   bool   `json:"delivery_proven"`
	Replay           bool   `json:"replay"`
}

func (h *Handler) ReconcileOutboundMedia(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.outboundMediaReconcile == nil {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, mediaapp.ErrOutboundMediaReconcileUnavailable))
		return
	}
	var body outboundMediaReconcileRequest
	contentPackageID, idErr := strconv.ParseInt(r.PathValue("content_package_id"), 10, 64)
	targetRef := r.PathValue("target_ref")
	_, keyErr := legacyAttachmentIdempotencyKey(r)
	if json.NewDecoder(r.Body).Decode(&body) != nil || idErr != nil || contentPackageID < 1 || !validOutboundMediaTargetRef(targetRef) || keyErr != nil || !validOutboundMediaEvidenceRef(body.EvidenceRef) {
		platformhttp.WriteError(w, r, platformhttp.NewError(platformhttp.CodeMalformedRequest, mediaapp.ErrOutboundMediaReconcileInvalid))
		return
	}
	result, err := h.outboundMediaReconcile.Reconcile(r.Context(), mediaapp.OutboundMediaReconcileCommand{
		ContentPackageID: contentPackageID,
		TargetRef:        targetRef,
		Generation:       body.Generation,
		Fence:            body.Fence,
		LeaseExpiresAt:   body.LeaseExpiresAt,
		EvidenceDigest:   outboundMediaEvidenceDigest(body.EvidenceRef),
		ProviderAccepted: body.ProviderAccepted,
		DeliveryProven:   body.DeliveryProven,
	})
	if err != nil {
		code := platformhttp.CodeDependencyUnavailable
		switch {
		case errors.Is(err, mediaapp.ErrOutboundMediaReconcileInvalid):
			code = platformhttp.CodeMalformedRequest
		case errors.Is(err, mediaapp.ErrOutboundMediaReconcileConflict):
			code = platformhttp.CodeConflict
		}
		platformhttp.WriteError(w, r, platformhttp.NewError(code, err))
		return
	}
	writeJSON(w, http.StatusOK, outboundMediaReconcileResponse{
		EffectID: result.EffectID, State: result.State, ProviderAccepted: result.ProviderAccepted, DeliveryProven: result.DeliveryProven, Replay: result.Replay,
	})
}

func validOutboundMediaEvidenceRef(value string) bool {
	return len(value) > 0 && len(value) <= 512 && utf8.ValidString(value) && value == strings.TrimSpace(value)
}

func outboundMediaEvidenceDigest(value string) string {
	sum := sha256.Sum256([]byte("outbound-media-manual-reconcile-evidence\x00" + value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
