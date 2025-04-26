-- name: CreateExchangePost :one
INSERT INTO "exchange_posts" ("id",
                              "user_id",
                              "content",
                              "post_image_urls")
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UpdateExchangePost :one
UPDATE "exchange_posts"
SET "post_image_urls" = $2
WHERE "id" = $1 RETURNING *;

-- name: ListExchangePosts :many
SELECT *
FROM "exchange_posts"
WHERE status = coalesce(sqlc.narg('status'), status)
ORDER BY created_at DESC, updated_at DESC;

-- name: ListUserExchangePosts :many
SELECT *
FROM "exchange_posts"
WHERE user_id = sqlc.arg('user_id')
  AND status = coalesce(sqlc.narg('status'), status)
ORDER BY created_at DESC, updated_at DESC;

-- name: GetUserExchangePost :one
SELECT *
FROM "exchange_posts"
WHERE id = sqlc.arg('post_id')
  AND user_id = sqlc.arg('user_id')
  AND status = coalesce(sqlc.narg('status'), status) LIMIT 1;

-- name: ListExchangePostItems :many
SELECT *
FROM "exchange_post_items"
WHERE post_id = $1
ORDER BY created_at DESC;

-- name: GetExchangePost :one
SELECT *
FROM "exchange_posts"
WHERE id = $1;

-- name: DeleteExchangePost :one
DELETE
FROM "exchange_posts"
WHERE id = $1 RETURNING *;

