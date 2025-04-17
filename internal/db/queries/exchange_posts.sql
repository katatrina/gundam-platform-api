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