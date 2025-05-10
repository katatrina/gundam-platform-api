-- name: CreateAuctionRequest :one
INSERT INTO auction_requests (id,
                              gundam_id,
                              seller_id,
                              gundam_snapshot,
                              starting_price,
                              bid_increment,
                              buy_now_price,
                              deposit_rate,
                              deposit_amount,
                              start_time,
                              end_time)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: CountExistingPendingAuctionRequest :one
SELECT COUNT(*)
FROM auction_requests
WHERE gundam_id = $1
  AND status = 'pending';

-- name: ListSellerAuctionRequests :many
SELECT *
FROM auction_requests
WHERE seller_id = $1
  AND status = COALESCE(sqlc.narg('status'), status)
ORDER BY created_at DESC;

-- name: ListAuctionRequests :many
SELECT *
FROM auction_requests
WHERE status = COALESCE(sqlc.narg('status'), status)
ORDER BY created_at DESC;

-- name: GetAuctionRequestByID :one
SELECT *
FROM auction_requests
WHERE id = $1;

-- name: DeleteAuctionRequest :exec
DELETE
FROM auction_requests
WHERE id = $1;

-- name: UpdateAuctionRequest :one
UPDATE auction_requests
SET status          = COALESCE(sqlc.narg('status'), status),
    rejected_by     = COALESCE(sqlc.narg('rejected_by'), rejected_by),
    rejected_reason = COALESCE(sqlc.narg('rejected_reason'), rejected_reason),
    updated_at      = now()
WHERE id = $1 RETURNING *;