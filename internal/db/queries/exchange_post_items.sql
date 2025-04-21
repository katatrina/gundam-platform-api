-- name: CreateExchangePostItems :many
INSERT INTO exchange_post_items (id,
                                 post_id,
                                 gundam_id)
SELECT UNNEST(@item_ids::uuid[]),
       @post_id::uuid, UNNEST(@gundam_ids::bigint[]) RETURNING *;

-- name: GetExchangePostItemByGundamID :one
SELECT *
FROM exchange_post_items
WHERE gundam_id = @gundam_id::bigint
  AND post_id = @post_id::uuid
LIMIT 1;