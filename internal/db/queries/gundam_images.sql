-- name: CreateGundamImage :exec
INSERT INTO gundam_images (gundam_id, url, is_primary)
VALUES ($1, $2, $3);

-- name: ListGundamImages :many
SELECT *
FROM gundam_images
WHERE gundam_id = $1;
