package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
)

// ListLatestCampaignMemberStatuses deliberately selects one immutable plan
// rather than aggregating recipients across campaign versions. Missing local
// recipient-review sidecars project as pending_review.
func (r *Repository) ListLatestCampaignMemberStatuses(ctx context.Context, campaignCode string, status campaign.TouchPlanRecipientReviewStatus, limit, offset int32) (campaign.CampaignMemberStatusSnapshot, error) {
	if !campaign.ValidCampaignCode(campaignCode) || status != "" && !status.Valid() || limit < 1 || limit > campaign.MaximumCampaignMemberPage || offset < 0 {
		return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.CampaignMemberStatusSnapshot{}, err
	}
	filter := pgtype.Text{}
	if status != "" {
		filter = pgtype.Text{String: string(status), Valid: true}
	}
	rows, err := queries.ListLatestCampaignMemberStatuses(ctx, campaigndb.ListLatestCampaignMemberStatusesParams{
		CampaignCode: campaignCode,
		StatusFilter: filter,
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		return campaign.CampaignMemberStatusSnapshot{}, err
	}
	if len(rows) == 0 {
		return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrNotFound
	}
	var result campaign.CampaignMemberStatusSnapshot
	for index, row := range rows {
		if index > 0 && (result.Total != row.Total || result.PlanID != row.PlanID) {
			return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrUnavailable
		}
		result.Total = row.Total
		result.PlanID = row.PlanID
		if row.CustomerID.Valid != row.Status.Valid {
			return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrUnavailable
		}
		if row.CustomerID.Valid {
			result.Items = append(result.Items, campaign.CampaignMemberStatus{PlanID: row.PlanID, CustomerID: row.CustomerID.Int64, Status: campaign.TouchPlanRecipientReviewStatus(row.Status.String)})
		}
	}
	if result.Items == nil {
		result.Items = []campaign.CampaignMemberStatus{}
	}
	return result, nil
}
