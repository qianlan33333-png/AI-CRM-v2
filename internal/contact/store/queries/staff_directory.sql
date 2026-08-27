-- name: ListEligibleStaffDirectory :many
SELECT id, wecom_userid, name, updated_at
FROM public.staff
WHERE is_active
  AND btrim(wecom_userid) <> ''
ORDER BY btrim(wecom_userid), wecom_userid;

-- name: LockEligibleStaffDirectoryByWeComUserID :many
SELECT id, wecom_userid, name, updated_at
FROM public.staff
WHERE wecom_userid = $1
  AND is_active
FOR SHARE;

-- name: LockActiveStaffExists :one
SELECT TRUE AS active
FROM public.staff
WHERE id = $1
  AND is_active
FOR SHARE;
