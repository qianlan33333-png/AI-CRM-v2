package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
)

type ExternalPushRepository struct{}

var _ surveyapp.ExternalPushStore = (*ExternalPushRepository)(nil)
var _ surveyapp.ExternalPushLogReader = (*ExternalPushRepository)(nil)

func NewExternalPushRepository() *ExternalPushRepository { return &ExternalPushRepository{} }

func (r *ExternalPushRepository) ListExternalPushLogs(ctx context.Context, questionnaireID *surveyport.ID, limit, offset int32) ([]surveyapp.ExternalPushLog, int64, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if r == nil || err != nil || limit < 1 || limit > 100 || offset < 0 || questionnaireID != nil && *questionnaireID < 1 {
		return nil, 0, surveyapp.ErrExternalPushUnavailable
	}
	query, countQuery := externalPushLogsAll, externalPushLogsCountAll
	arguments := []any{limit, offset}
	countArguments := []any{}
	if questionnaireID != nil {
		query, countQuery = externalPushLogsForQuestionnaire, externalPushLogsCountForQuestionnaire
		arguments = []any{int64(*questionnaireID), limit, offset}
		countArguments = []any{int64(*questionnaireID)}
	}
	var total int64
	if err := tx.QueryRow(ctx, countQuery, countArguments...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []surveyapp.ExternalPushLog{}
	for rows.Next() {
		var item surveyapp.ExternalPushLog
		var effectID int64
		var state string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.BindingID, &item.QuestionnaireID, &item.SubmissionID, &item.CustomerID, &effectID, &state, &item.AttemptCount, &item.ProviderAccepted, &item.DeliveryProven, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}
		item.EffectID = fmt.Sprintf("eer_%d", effectID)
		item.State = eer.State(state)
		item.CreatedAt, item.UpdatedAt = createdAt.UTC(), updatedAt.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

const externalPushLogsSelect = `
SELECT b.id, b.questionnaire_id, b.public_submission_id, b.customer_id,
       b.external_effect_id, e.state, e.attempt_count,
       COALESCE(bool_or(r.provider_accepted), FALSE),
       COALESCE(bool_or(r.delivery_proven), FALSE),
       b.created_at, e.updated_at
FROM public.questionnaire_submission_external_push_bindings b
JOIN public.external_effects e ON e.id = b.external_effect_id AND e.owner = 'survey' AND e.kind = 'survey_webhook'
LEFT JOIN public.questionnaire_external_push_delivery_receipts r ON r.binding_id = b.id
`

const externalPushLogsAll = externalPushLogsSelect + `
GROUP BY b.id, e.id
ORDER BY b.created_at DESC, b.id DESC
LIMIT $1 OFFSET $2`

const externalPushLogsForQuestionnaire = externalPushLogsSelect + `
WHERE b.questionnaire_id = $1
GROUP BY b.id, e.id
ORDER BY b.created_at DESC, b.id DESC
LIMIT $2 OFFSET $3`

const externalPushLogsCountAll = `
SELECT count(*)
FROM public.questionnaire_submission_external_push_bindings b
JOIN public.external_effects e ON e.id = b.external_effect_id AND e.owner = 'survey' AND e.kind = 'survey_webhook'`

const externalPushLogsCountForQuestionnaire = externalPushLogsCountAll + `
WHERE b.questionnaire_id = $1`

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
