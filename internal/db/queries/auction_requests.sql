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