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