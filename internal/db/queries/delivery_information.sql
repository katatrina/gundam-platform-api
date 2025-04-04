-- name: CreateDeliveryInformation :one
INSERT INTO delivery_information (user_id,
                                  full_name,
                                  phone_number,
                                  province_name,
                                  district_name,
                                  ghn_district_id,
                                  ward_name,
                                  ghn_ward_code,
                                  detail)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetDeliveryInformation :one
SELECT *
FROM delivery_information
WHERE id = $1;