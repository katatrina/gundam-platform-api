-- name: CreateOrderDelivery :one
INSERT INTO order_deliveries (order_id,
                              ghn_order_code,
                              expected_delivery_time,
                              status,
                              overall_status,
                              from_delivery_id,
                              to_delivery_id)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

