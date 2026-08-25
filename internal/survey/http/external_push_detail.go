package surveyhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type ExternalPushDetailApplication interface {
	Detail(context.Context, surveyport.ID, int64) (surveyapp.ExternalPushBinding, error)
}
type ExternalPushDetailHandler struct{ Application ExternalPushDetailApplication }

func (h *ExternalPushDetailHandler) Get(w http.ResponseWriter, r *http.Request, qid surveyport.ID, submissionID int64) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	a, ok := authport.AuthorizationFromContext(r.Context())
	if !ok || a.Capability != authport.CapabilityQuestionnairesRead || a.Scope != authport.ScopeGlobal {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "permission_denied"})
		return
	}
	if h == nil || h.Application == nil || qid < 1 || submissionID < 1 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "invalid_request"})
		return
	}
	v, e := h.Application.Detail(r.Context(), qid, submissionID)
	if e != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found"})
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		SubmissionID     int64  `json:"submission_id"`
		EffectID         string `json:"effect_id"`
		State            string `json:"state"`
		ProviderAccepted bool   `json:"provider_accepted"`
		DeliveryProven   bool   `json:"delivery_proven"`
	}{v.SubmissionID, v.EffectID, string(v.State), v.ProviderAccepted, v.DeliveryProven})
}
func ParseExternalPushDetailPath(path string) (surveyport.ID, int64, bool) {
	p := strings.Split(strings.Trim(path, "/"), "/")
	if len(p) != 6 || p[0] != "api" || p[1] != "admin" || p[2] != "questionnaires" || p[4] != "submissions" || p[5] == "" {
		return 0, 0, false
	}
	q, e := strconv.ParseInt(p[3], 10, 64)
	s, f := strconv.ParseInt(p[5], 10, 64)
	return surveyport.ID(q), s, e == nil && f == nil && q > 0 && s > 0
}
