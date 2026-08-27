-- name: InsertHistoricalCampaignDefinition :exec
INSERT INTO public.cloud_campaigns
(campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,'rejected','paused',1,$3,$4,$5,$6);

-- name: InsertHistoricalCampaignStep :exec
INSERT INTO public.cloud_campaign_steps (campaign_code,step_index,delay_minutes,content)
VALUES ($1,$2,$3,$4);
