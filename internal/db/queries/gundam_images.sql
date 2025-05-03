-- name: GetImageByURL :one
SELECT *
FROM gundam_images
WHERE url = $1
  AND gundam_id = $2 LIMIT 1;

-- name: GetGundamPrimaryImageURL :one
SELECT url
FROM gundam_images
WHERE gundam_id = $1
  AND is_primary = true;

-- name: GetGundamSecondaryImageURLs :many
SELECT url
FROM gundam_images
WHERE gundam_id = $1
  AND is_primary = false
ORDER BY created_at DESC;

-- name: UpdateGundamPrimaryImage :exec
UPDATE gundam_images
SET url = $2
WHERE gundam_id = $1
  AND is_primary = true;

-- name: DeleteGundamImage :exec
DELETE
FROM gundam_images
WHERE gundam_id = $1
  AND url = $2;