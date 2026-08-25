package surveyhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const ExternalPushReconcilePath = ExternalPushDetailPath + "/reconcile"

type ExternalPushReconcileApplication interface {
	Reconcile(context.Context, surveyapp.ExternalPushReconcileCommand) (surveyapp.ExternalPushBinding, error)
}

type ExternalPushReconcileHandler struct {
	Application ExternalPushReconcileApplication
}

func (h *ExternalPushReconcileHandler) Reconcile(w http.ResponseWriter, r *http.Request, questionnaireID surveyport.ID, submissionID int64) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r == nil || r.Method != http.MethodPost || h == nil || h.Application == nil || questionnaireID < 1 || submissionID < 1 {
		writeExternalPushReconcileError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	authorization, ok := authport.AuthorizationFromContext(r.Context())
	if !ok || authorization.Capability != authport.CapabilityQuestionnairesWrite || authorization.Scope != authport.ScopeGlobal {
		writeExternalPushReconcileError(w, http.StatusForbidden, "permission_denied")
		return
	}
	key, ok := externalPushReconcileKey(r)
	if !ok {
		writeExternalPushReconcileError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var body externalPushReconcileRequest
	if !decodeExternalPushReconcile(w, r, &body) {
		writeExternalPushReconcileError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, body.LeaseExpiresAt)
	evidenceDigest, validDigest := parseExternalPushDigest(body.EvidenceDigest)
	if err != nil || !validDigest || body.EffectID == "" || body.Generation < 1 || body.Fence < 1 || body.ProviderAccepted == nil || body.DeliveryProven == nil {
		writeExternalPushReconcileError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	value, err := h.Application.Reconcile(r.Context(), surveyapp.ExternalPushReconcileCommand{
		QuestionnaireID: questionnaireID, SubmissionID: submissionID,
		Lease:          eer.Lease{EffectID: body.EffectID, Generation: body.Generation, Fence: body.Fence, ExpiresAt: expiresAt.UTC()},
		EvidenceDigest: evidenceDigest, ProviderAccepted: *body.ProviderAccepted, DeliveryProven: *body.DeliveryProven, IdempotencyKey: key,
	})
	if err != nil {
		switch {
		case errors.Is(err, surveyapp.ErrExternalPushNotFound):
			writeExternalPushReconcileError(w, http.StatusNotFound, "not_found")
		case errors.Is(err, surveyapp.ErrExternalPushReconcileRequired), errors.Is(err, surveyapp.ErrExternalPushReconcileConflict),
			errors.Is(err, eer.ErrReconcileRequired), errors.Is(err, eer.ErrLeaseFence), errors.Is(err, eer.ErrPayloadMismatch):
			writeExternalPushReconcileError(w, http.StatusConflict, "reconcile_conflict")
		default:
			writeExternalPushReconcileError(w, http.StatusServiceUnavailable, "unavailable")
		}
		return
	}
	_ = json.NewEncoder(w).Encode(externalPushReconcileResponse{SubmissionID: value.SubmissionID, EffectID: value.EffectID, State: string(value.State), ProviderAccepted: value.ProviderAccepted, DeliveryProven: value.DeliveryProven})
}

type externalPushReconcileRequest struct {
	EffectID         string `json:"effect_id"`
	Generation       int64  `json:"generation"`
	Fence            int64  `json:"fence"`
	LeaseExpiresAt   string `json:"lease_expires_at"`
	EvidenceDigest   string `json:"evidence_digest"`
	ProviderAccepted *bool  `json:"provider_accepted"`
	DeliveryProven   *bool  `json:"delivery_proven"`
}
type externalPushReconcileResponse struct {
	SubmissionID     int64  `json:"submission_id"`
	EffectID         string `json:"effect_id"`
	State            string `json:"state"`
	ProviderAccepted bool   `json:"provider_accepted"`
	DeliveryProven   bool   `json:"delivery_proven"`
}

func ParseExternalPushReconcilePath(path string) (surveyport.ID, int64, bool) {
	if !strings.HasSuffix(path, "/reconcile") {
		return 0, 0, false
	}
	detailPath := strings.TrimSuffix(path, "/reconcile")
	if strings.HasSuffix(detailPath, "/") {
		return 0, 0, false
	}
	return ParseExternalPushDetailPath(detailPath)
}

func externalPushReconcileKey(r *http.Request) (string, bool) {
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 || len(values[0]) < 16 {
		return "", false
	}
	return values[0], true
}

func decodeExternalPushReconcile(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(&struct{}{}) == io.EOF
}

func parseExternalPushDigest(value string) ([32]byte, bool) {
	var digest [32]byte
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return digest, false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	if err != nil || len(decoded) != sha256.Size {
		return digest, false
	}
	copy(digest[:], decoded)
	return digest, true
}

func writeExternalPushReconcileError(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
