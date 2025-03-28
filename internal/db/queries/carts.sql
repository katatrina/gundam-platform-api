-- name: GetOrCreateCartIfNotExists :one
INSERT INTO carts (user_id)
VALUES ($1) ON CONFLICT (user_id) DO
UPDATE
    SET id = carts.id -- This is a no-op update to force a return
    RETURNING id;

-- name: GetCartByUserID :one
SELECT id
FROM carts
WHERE user_id = $1;

-- name: AddCartItem :one
WITH inserted_item AS (
INSERT
INTO cart_items (cart_id, gundam_id)
VALUES ($1, $2)
    RETURNING id, cart_id, gundam_id
    )
SELECT ci.id        AS cart_item_id,
       g.id         AS gundam_id,
       g.name       AS gundam_name,
       g.price      AS gundam_price,
       gi.url       AS gundam_image_url,
       s.id         AS seller_id,
       s.full_name  AS seller_name,
       s.avatar_url AS seller_avatar_url
FROM inserted_item ci
         JOIN gundams g ON ci.gundam_id = g.id
         JOIN users s ON g.owner_id = s.id
         JOIN gundam_images gi
              ON gi.gundam_id = g.id
                  AND gi.is_primary = true;

-- name: ListCartItemsWithDetails :many
SELECT ci.id        AS cart_item_id,
       g.id         AS gundam_id,
       g.name       AS gundam_name,
       g.price      AS gundam_price,
       gi.url       AS gundam_image_url,
       s.id         AS seller_id,
       s.full_name  AS seller_name,
       s.avatar_url AS seller_avatar_url
FROM cart_items ci
         JOIN gundams g ON ci.gundam_id = g.id
         JOIN users s ON g.owner_id = s.id
         JOIN gundam_images gi
              ON gi.gundam_id = g.id
                  AND gi.is_primary = true
WHERE ci.cart_id = $1
  AND g.status = 'selling'
  AND g.deleted_at IS NULL;

-- name: RemoveCartItem :exec
DELETE
FROM cart_items
WHERE id = $1
  AND cart_id = $2;

-- name: CheckCartItemExists :one
SELECT EXISTS (SELECT 1
               FROM cart_items
               WHERE cart_id = $1
                 AND gundam_id = $2);
