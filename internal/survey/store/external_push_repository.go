package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
)

type ExternalPushRepository struct{}

var _ surveyapp.ExternalPushStore = (*ExternalPushRepository)(nil)

func NewExternalPushRepository() *ExternalPushRepository { return &ExternalPushRepository{} }
func (r *ExternalPushRepository) BindExternalPush(ctx context.Context, v surveyapp.ExternalPushBinding) (surveyapp.ExternalPushBinding, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return v, surveyapp.ErrExternalPushUnavailable
	}
	id, e := pushEffectID(v.EffectID)
	if e != nil {
		return v, e
	}
	row, e := q.BindSurveyExternalPush(ctx, surveydb.BindSurveyExternalPushParams{QuestionnaireID: int64(v.QuestionnaireID), PublicSubmissionID: v.SubmissionID, CustomerID: v.CustomerID, ExternalEffectID: id, SourceRefDigest: v.SourceRefDigest, TargetRefDigest: v.TargetRefDigest, PayloadDigest: v.PayloadDigest, PolicyVersionHash: v.PolicyVersionHash})
	if e != nil {
		return v, e
	}
	return surveyapp.ExternalPushBinding{ID: row.ID, QuestionnaireID: surveyport.ID(row.QuestionnaireID), SubmissionID: row.PublicSubmissionID, CustomerID: row.CustomerID, EffectID: v.EffectID, State: v.State, CreatedAt: row.CreatedAt.Time}, nil
}
func (r *ExternalPushRepository) GetExternalPush(ctx context.Context, qid surveyport.ID, sid int64) (surveyapp.ExternalPushBinding, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushUnavailable
	}
	row, e := q.GetSurveyExternalPush(ctx, surveydb.GetSurveyExternalPushParams{QuestionnaireID: int64(qid), PublicSubmissionID: sid})
	if e != nil {
		return surveyapp.ExternalPushBinding{}, e
	}
	pa, _ := row.ProviderAccepted.(bool)
	dp, _ := row.DeliveryProven.(bool)
	return surveyapp.ExternalPushBinding{ID: row.ID, QuestionnaireID: surveyport.ID(row.QuestionnaireID), SubmissionID: row.PublicSubmissionID, CustomerID: row.CustomerID, EffectID: fmt.Sprintf("eer_%d", row.ExternalEffectID), State: eer.State(row.State), ProviderAccepted: pa, DeliveryProven: dp, CreatedAt: row.CreatedAt.Time}, nil
}
func pushEffectID(v string) (int64, error) {
	if !strings.HasPrefix(v, "eer_") {
		return 0, surveyapp.ErrExternalPushUnavailable
	}
	n, e := strconv.ParseInt(strings.TrimPrefix(v, "eer_"), 10, 64)
	if e != nil || n < 1 {
		return 0, surveyapp.ErrExternalPushUnavailable
	}
	return n, nil
}
