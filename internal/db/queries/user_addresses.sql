-- name: CreateUserAddress :one
INSERT INTO user_addresses (user_id, full_name, phone_number, province_name, district_name, ghn_district_id, ward_name,
                            ghn_ward_code,
                            detail, is_primary, is_pickup_address)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: GetUserAddressForUpdate :one
SELECT *
FROM user_addresses
WHERE id = sqlc.arg('address_id')
  AND user_id = sqlc.arg('user_id') FOR UPDATE;

-- name: ListUserAddresses :many
SELECT *
FROM user_addresses
WHERE user_id = $1
ORDER BY is_primary DESC, is_pickup_address DESC, created_at DESC;

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

-- name: UpdateUserAddress :one
UPDATE user_addresses
SET full_name         = COALESCE(sqlc.narg('full_name'), full_name),
    phone_number      = COALESCE(sqlc.narg('phone_number'), phone_number),
    province_name     = COALESCE(sqlc.narg('province_name'), province_name),
    district_name     = COALESCE(sqlc.narg('district_name'), district_name),
    ghn_district_id   = COALESCE(sqlc.narg('ghn_district_id'), ghn_district_id),
    ward_name         = COALESCE(sqlc.narg('ward_name'), ward_name),
    ghn_ward_code     = COALESCE(sqlc.narg('ghn_ward_code'), ghn_ward_code),
    detail            = COALESCE(sqlc.narg('detail'), detail),
    is_primary        = COALESCE(sqlc.narg('is_primary'), is_primary),
    is_pickup_address = COALESCE(sqlc.narg('is_pickup_address'), is_pickup_address)
WHERE id = sqlc.arg('address_id')
  AND user_id = sqlc.arg('user_id')
RETURNING *;

-- name: DeleteUserAddress :exec
DELETE
FROM user_addresses
WHERE id = sqlc.arg('address_id')
  AND user_id = sqlc.arg('user_id');

-- name: GetUserPickupAddress :one
SELECT *
FROM user_addresses
WHERE user_id = $1
  AND is_pickup_address = true;