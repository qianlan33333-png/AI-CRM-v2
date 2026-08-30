-- name: ListMediaImagePage :many
WITH normalized AS MATERIALIZED (
    SELECT
        image.id,
        image.name,
        image.file_name,
        image.mime_type,
        image.file_size,
        image.enabled,
        image.description,
        image.tags,
        image.category,
        image.width,
        image.height,
        image.created_at,
        image.updated_at,
        ARRAY(
            SELECT left(candidate.tag, 64)
            FROM (
                SELECT btrim(part.value, $8::text) AS tag, part.ordinality
                FROM unnest(string_to_array(image.tags, ',')) WITH ORDINALITY AS part(value, ordinality)
            ) AS candidate
            WHERE candidate.tag <> ''
              AND (
                  char_length(candidate.tag) > 64
                  OR NOT EXISTS (
                      SELECT 1
                      FROM unnest(string_to_array(image.tags, ',')) WITH ORDINALITY AS earlier(value, ordinality)
                      WHERE earlier.ordinality < candidate.ordinality
                        AND btrim(earlier.value, $8::text) <> ''
                        AND left(btrim(earlier.value, $8::text), 64) COLLATE "C" = candidate.tag COLLATE "C"
                  )
              )
            ORDER BY candidate.ordinality
            LIMIT 50
        )::text[] AS normalized_tags
    FROM media_images AS image
),
filtered AS MATERIALIZED (
    SELECT *
    FROM normalized
    WHERE (
        $1::text = ''
        OR name ILIKE ('%' || $1::text || '%')
        OR file_name ILIKE ('%' || $1::text || '%')
        OR description ILIKE ('%' || $1::text || '%')
        OR category ILIKE ('%' || $1::text || '%')
        OR EXISTS (
            SELECT 1
            FROM unnest(normalized_tags) AS item_tag(value)
            WHERE item_tag.value ILIKE ('%' || $1::text || '%')
        )
    )
      AND ($2::text = '' OR category COLLATE "C" = $2::text COLLATE "C")
      AND (
          cardinality($3::text[]) = 0
          OR EXISTS (
              SELECT 1
              FROM unnest($3::text[]) AS wanted_tag(value)
              WHERE EXISTS (
                  SELECT 1
                  FROM unnest(normalized_tags) AS item_tag(value)
                  WHERE item_tag.value COLLATE "C" = wanted_tag.value COLLATE "C"
              )
          )
      )
      AND NOT EXISTS (
          SELECT 1
          FROM jsonb_array_elements($4::jsonb) AS wanted_group(value)
          WHERE NOT EXISTS (
              SELECT 1
              FROM jsonb_array_elements_text(wanted_group.value) AS wanted_tag(value)
              WHERE EXISTS (
                  SELECT 1
                  FROM unnest(normalized_tags) AS item_tag(value)
                  WHERE item_tag.value COLLATE "C" = wanted_tag.value COLLATE "C"
              )
          )
      )
      AND (
          NOT $5::boolean
          OR description = ''
          OR category = ''
          OR cardinality(normalized_tags) = 0
      )
      AND (NOT $9::boolean OR enabled)
),
total AS (
    SELECT count(*)::bigint AS value
    FROM filtered
),
page AS (
    SELECT *
    FROM filtered
    ORDER BY updated_at DESC, id DESC
    LIMIT $6::bigint
    OFFSET $7::bigint
)
SELECT
    total.value AS total,
    page.id,
    page.name,
    page.file_name,
    page.mime_type,
    page.file_size,
    page.enabled,
    page.description,
    page.tags,
    page.category,
    page.width,
    page.height,
    page.created_at,
    page.updated_at
FROM total
LEFT JOIN page ON TRUE
ORDER BY page.updated_at DESC NULLS LAST, page.id DESC NULLS LAST;

-- name: CountEnabledMediaImages :one
SELECT count(*)::bigint FROM media_images WHERE enabled;
