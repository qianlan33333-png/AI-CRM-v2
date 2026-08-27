-- name: InsertHistoricalMiniProgram :one
INSERT INTO public.media_miniprograms
(legacy_source_id,name,app_id,page_path,title,enabled,created_by,updated_by,version,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,FALSE,$6,$7,$8,$9,$10) RETURNING id;
