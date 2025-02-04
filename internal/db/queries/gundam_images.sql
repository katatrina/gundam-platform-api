-- name: CreateGundamImage :exec
INSERT INTO gundam_images (gundam_id, image_url, is_primary)
VALUES ($1, $2, $3);