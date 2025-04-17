-- name: CreateExchangePostItems :many
INSERT INTO exchange_post_items (id,
                                 post_id,
                                 gundam_id)
SELECT UNNEST(@item_ids::uuid[]),
       @post_id::uuid, UNNEST(@gundam_ids::bigint[]) RETURNING *;