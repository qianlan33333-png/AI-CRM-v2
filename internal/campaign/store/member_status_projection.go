package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

// ListLatestCampaignMemberStatuses deliberately selects one immutable plan
// rather than aggregating recipients across campaign versions. Missing local
// recipient-review sidecars project as pending_review.
func (r *Repository) ListLatestCampaignMemberStatuses(ctx context.Context, campaignCode string, status campaign.TouchPlanRecipientReviewStatus, limit, offset int32) (campaign.CampaignMemberStatusSnapshot, error) {
	if !campaign.ValidCampaignCode(campaignCode) || status != "" && !status.Valid() || limit < 1 || limit > campaign.MaximumCampaignMemberPage || offset < 0 {
		return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrUnavailable
	}
	tx, err := r.transaction(ctx)
	if err != nil {
		return campaign.CampaignMemberStatusSnapshot{}, err
	}
	var filter *string
	if status != "" {
		value := string(status)
		filter = &value
	}
	rows, err := tx.Query(ctx, `WITH selected AS (
  SELECT (
    SELECT plan.id
    FROM public.cloud_campaign_touch_plans AS plan
    WHERE plan.campaign_code = campaign.campaign_code
    ORDER BY plan.created_at DESC, plan.id DESC
    LIMIT 1
  ) AS plan_id
  FROM public.cloud_campaigns AS campaign
  WHERE campaign.campaign_code = $1
), projected AS (
  SELECT selected.plan_id,
         target.customer_id,
         COALESCE(review.status, 'pending_review') AS status
  FROM selected
  JOIN public.cloud_campaign_touch_plan_targets AS target
    ON target.plan_id = selected.plan_id
  LEFT JOIN public.cloud_campaign_touch_plan_recipient_reviews AS review
    ON review.plan_id = target.plan_id
   AND review.customer_id = target.customer_id
  WHERE $2::text IS NULL OR COALESCE(review.status, 'pending_review') = $2
), page AS (
  SELECT plan_id, customer_id, status
  FROM projected
  ORDER BY customer_id
  LIMIT $3 OFFSET $4
)
SELECT selected.plan_id,
       (SELECT count(*) FROM projected) AS total,
       page.customer_id,
       page.status
FROM selected
LEFT JOIN page ON TRUE
ORDER BY page.customer_id`, campaignCode, filter, limit, offset)
	if err != nil {
		return campaign.CampaignMemberStatusSnapshot{}, err
	}
	defer rows.Close()
	var result campaign.CampaignMemberStatusSnapshot
	seen := false
	first := true
	for rows.Next() {
		seen = true
		var planID, projectedStatus pgtype.Text
		var customerID pgtype.Int8
		var total int64
		if err = rows.Scan(&planID, &total, &customerID, &projectedStatus); err != nil {
			return campaign.CampaignMemberStatusSnapshot{}, err
		}
		if !first && (result.Total != total || result.PlanID != "" && planID.Valid && result.PlanID != planID.String) {
			return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrUnavailable
		}
		first = false
		result.Total = total
		if planID.Valid {
			result.PlanID = planID.String
		}
		if customerID.Valid != projectedStatus.Valid {
			return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrUnavailable
		}
		if customerID.Valid {
			result.Items = append(result.Items, campaign.CampaignMemberStatus{PlanID: planID.String, CustomerID: customerID.Int64, Status: campaign.TouchPlanRecipientReviewStatus(projectedStatus.String)})
		}
	}
	if err = rows.Err(); err != nil {
		return campaign.CampaignMemberStatusSnapshot{}, err
	}
	if !seen {
		return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrNotFound
	}
	if result.Items == nil {
		result.Items = []campaign.CampaignMemberStatus{}
	}
	return result, nil
}
