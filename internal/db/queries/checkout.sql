-- name: GetCheckoutItems :many
SELECT g.id         AS gundam_id,
       g.name       AS gundam_name,
       g.price      AS gundam_price,
       gi.url       AS gundam_image_url,
       s.id         AS seller_id,
       s.full_name  AS seller_name,
       s.avatar_url AS seller_avatar_url
FROM gundams g
         JOIN users s ON g.owner_id = s.id
         JOIN gundam_images gi ON gi.gundam_id = g.id
    AND gi.is_primary = true
WHERE g.id = ANY(sqlc.arg(item_ids)::int8[]);