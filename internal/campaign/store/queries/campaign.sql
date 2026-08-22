-- The generated query package is intentionally retained for PG16 schema
-- validation and future read-model reporting; command SQL stays transaction
-- bound in postgres.go so typed Plan/Command values are checked before save.
-- name: LockCloudCampaignDeleteReferences :exec
LOCK TABLE public.cloud_campaign_local_plans, public.cloud_campaign_local_commands IN SHARE MODE;
