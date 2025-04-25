-- name: CreateExchangeItem :one
INSERT INTO exchange_items (id,
                            exchange_id,
                            gundam_id,
                            name,
                            slug,
                            grade,
                            scale,
                            quantity,
                            price,
                            weight,
                            image_url,
                            owner_id,
                            is_from_poster)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING *;
