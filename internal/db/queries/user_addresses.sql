-- name: CreateUserAddress :one
INSERT INTO user_addresses (user_id, full_name, phone_number, province_name, district_name, ghn_district_id, ward_name,
                            ghn_ward_code,
                            detail, is_primary, is_pickup_address)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: ListUserAddresses :many
SELECT *
FROM user_addresses
WHERE user_id = $1
ORDER BY is_primary DESC, created_at DESC;

-- name: UnsetPrimaryAddress :exec
UPDATE user_addresses
SET is_primary = false
WHERE user_id = $1
  AND is_primary = true;

-- name: UnsetPickupAddress :exec
UPDATE user_addresses
SET is_pickup_address = false
WHERE user_id = $1
  AND is_pickup_address = true;

-- name: UpdateUserAddress :exec
UPDATE user_addresses
SET is_primary = COALESCE(sqlc.narg('is_primary'), is_primary),
    updated_at = now()
WHERE id = sqlc.arg('address_id');