-- name: GetMediaContentReferenceEligibility :one
SELECT CASE sqlc.arg(ref_kind)::text
  WHEN 'image' THEN EXISTS (SELECT 1 FROM media_images WHERE id = sqlc.arg(ref_id)::bigint AND enabled)
  WHEN 'attachment' THEN EXISTS (SELECT 1 FROM media_attachments WHERE id = sqlc.arg(ref_id)::bigint AND enabled)
  WHEN 'miniprogram' THEN EXISTS (SELECT 1 FROM media_miniprograms WHERE id = sqlc.arg(ref_id)::bigint AND enabled)
  WHEN 'group_invite' THEN EXISTS (SELECT 1 FROM media_group_invites WHERE id = sqlc.arg(ref_id)::bigint AND enabled AND archived_at IS NULL)
  ELSE FALSE END AS eligible;

-- name: CreateMediaContentPackage :one
INSERT INTO media_content_packages (name, content_text, enabled, created_by, updated_by, created_at, updated_at)
VALUES (sqlc.arg(name), sqlc.arg(content_text), sqlc.arg(enabled), sqlc.arg(actor_id), sqlc.arg(actor_id), sqlc.arg(now), sqlc.arg(now))
RETURNING id, name, content_text, enabled, version, created_by, updated_by, created_at, updated_at;

-- name: InsertMediaContentPackageImageRef :exec
INSERT INTO media_content_package_refs (package_id, position, ref_kind, image_id) VALUES (sqlc.arg(package_id), sqlc.arg(position), 'image', sqlc.arg(ref_id));
-- name: InsertMediaContentPackageAttachmentRef :exec
INSERT INTO media_content_package_refs (package_id, position, ref_kind, attachment_id) VALUES (sqlc.arg(package_id), sqlc.arg(position), 'attachment', sqlc.arg(ref_id));
-- name: InsertMediaContentPackageMiniprogramRef :exec
INSERT INTO media_content_package_refs (package_id, position, ref_kind, miniprogram_id) VALUES (sqlc.arg(package_id), sqlc.arg(position), 'miniprogram', sqlc.arg(ref_id));
-- name: InsertMediaContentPackageGroupInviteRef :exec
INSERT INTO media_content_package_refs (package_id, position, ref_kind, group_invite_id) VALUES (sqlc.arg(package_id), sqlc.arg(position), 'group_invite', sqlc.arg(ref_id));

-- name: ListMediaContentPackageRefs :many
SELECT position, ref_kind, COALESCE(image_id, attachment_id, miniprogram_id, group_invite_id) AS ref_id
FROM media_content_package_refs WHERE package_id = sqlc.arg(package_id) ORDER BY position;

-- name: GetMediaContentPackage :one
SELECT id, name, content_text, enabled, version, created_by, updated_by, created_at, updated_at
FROM media_content_packages WHERE id = sqlc.arg(package_id);

-- name: UpdateMediaContentPackage :one
UPDATE media_content_packages SET name = sqlc.arg(name), content_text = sqlc.arg(content_text), enabled = sqlc.arg(enabled),
  version = version + 1, updated_by = sqlc.arg(actor_id), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(package_id) AND version = sqlc.arg(expected_version)
RETURNING id, name, content_text, enabled, version, created_by, updated_by, created_at, updated_at;

-- name: CreateMediaCampaignDeliveryBinding :one
INSERT INTO media_campaign_delivery_bindings (campaign_code, plan_id, package_id, group_invite_id, created_by, updated_by, created_at, updated_at)
VALUES (sqlc.arg(campaign_code), sqlc.arg(plan_id), sqlc.arg(package_id), sqlc.arg(group_invite_id), sqlc.arg(actor_id), sqlc.arg(actor_id), sqlc.arg(now), sqlc.arg(now))
RETURNING id, campaign_code, plan_id, package_id, group_invite_id, version, created_by, updated_by, created_at, updated_at;

-- name: GetMediaCampaignDeliveryBinding :one
SELECT id, campaign_code, plan_id, package_id, group_invite_id, version, created_by, updated_by, created_at, updated_at
FROM media_campaign_delivery_bindings WHERE campaign_code = sqlc.arg(campaign_code) AND plan_id = sqlc.arg(plan_id);

-- name: UpdateMediaCampaignDeliveryBinding :one
UPDATE media_campaign_delivery_bindings SET package_id = sqlc.arg(package_id), group_invite_id = sqlc.arg(group_invite_id), version = version + 1,
  updated_by = sqlc.arg(actor_id), updated_at = sqlc.arg(now)
WHERE campaign_code = sqlc.arg(campaign_code) AND plan_id = sqlc.arg(plan_id) AND version = sqlc.arg(expected_version)
RETURNING id, campaign_code, plan_id, package_id, group_invite_id, version, created_by, updated_by, created_at, updated_at;

-- name: DeleteMediaCampaignDeliveryBinding :execrows
DELETE FROM media_campaign_delivery_bindings WHERE campaign_code = sqlc.arg(campaign_code) AND plan_id = sqlc.arg(plan_id) AND version = sqlc.arg(expected_version);

-- name: InitiateMediaAttachmentUpload :one
INSERT INTO media_attachment_uploads (file_name, name, description, tags, enabled, expected_size, expected_digest, created_by, state, created_at)
VALUES (sqlc.arg(file_name), sqlc.arg(name), sqlc.arg(description), sqlc.arg(tags), sqlc.arg(enabled), sqlc.arg(expected_size), sqlc.arg(expected_digest), sqlc.arg(actor_id), 'initiated', sqlc.arg(now))
RETURNING id, state, expected_size, expected_digest, created_at;

-- name: PutMediaAttachmentUploadPart :exec
INSERT INTO media_attachment_upload_parts (upload_id, part_number, digest, content, created_at)
VALUES (sqlc.arg(upload_id), sqlc.arg(part_number), sqlc.arg(digest), sqlc.arg(content), sqlc.arg(now))
ON CONFLICT (upload_id, part_number) DO UPDATE SET digest = EXCLUDED.digest, content = EXCLUDED.content, created_at = EXCLUDED.created_at
WHERE media_attachment_upload_parts.digest = EXCLUDED.digest AND media_attachment_upload_parts.content = EXCLUDED.content;

-- name: ReadMediaAttachmentUploadForCompletion :one
SELECT id, file_name, name, description, tags, enabled, expected_size, expected_digest, created_by, state, attachment_id
FROM media_attachment_uploads WHERE id = sqlc.arg(upload_id) FOR UPDATE;

-- name: InsertOutboundMediaEffectBinding :one
INSERT INTO outbound_media_effect_bindings (content_package_id, target_digest, snapshot_digest, effect_id, created_at)
VALUES (sqlc.arg(content_package_id), sqlc.arg(target_digest), sqlc.arg(snapshot_digest), sqlc.arg(effect_id), sqlc.arg(created_at))
ON CONFLICT (content_package_id, target_digest) DO NOTHING
RETURNING id, content_package_id, target_digest, snapshot_digest, effect_id, created_at;

-- name: GetOutboundMediaEffectBinding :one
SELECT id, content_package_id, target_digest, snapshot_digest, effect_id, created_at
FROM outbound_media_effect_bindings
WHERE content_package_id = sqlc.arg(content_package_id) AND target_digest = sqlc.arg(target_digest);

-- name: ListMediaAttachmentUploadParts :many
SELECT part_number, digest, content FROM media_attachment_upload_parts WHERE upload_id = sqlc.arg(upload_id) ORDER BY part_number;

-- name: CompleteMediaAttachmentUpload :one
UPDATE media_attachment_uploads SET state = 'completed', attachment_id = sqlc.arg(attachment_id), completed_at = sqlc.arg(now)
WHERE id = sqlc.arg(upload_id) AND state = 'initiated'
RETURNING id, attachment_id, completed_at;
