-- name: LockAIAudienceOperationMemberProjection :exec
LOCK TABLE public.ai_audience_operation_member_projection IN EXCLUSIVE MODE;

-- name: DeleteAIAudienceOperationMembers :exec
DELETE FROM public.ai_audience_operation_member_projection;

-- name: InsertAIAudienceOperationMember :exec
INSERT INTO public.ai_audience_operation_member_projection (
  sender_userid,
  display_name,
  synced_at
) VALUES ($1, $2, $3);

-- name: ListAIAudienceOperationMembers :many
SELECT sender_userid, display_name
FROM public.ai_audience_operation_member_projection
ORDER BY sender_userid ASC;
