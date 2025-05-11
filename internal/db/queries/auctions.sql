-- name: CountSellerActiveAuctions :one
SELECT COUNT(*)
FROM auctions
WHERE seller_id = $1
  AND status IN ('scheduled', 'active');

-- name: CreateAuction :one
INSERT INTO auctions (id,
                      request_id,
                      gundam_id,
                      seller_id,
                      gundam_snapshot,
                      starting_price,
                      bid_increment,
                      buy_now_price,
                      current_price,
                      deposit_rate,
                      deposit_amount,
                      start_time,
                      end_time)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING *;

-- name: UpdateAuction :one
UPDATE auctions
SET status     = COALESCE(sqlc.narg('status'), status),
    updated_at = now()
WHERE id = $1 RETURNING *;

-- name: GetAuctionByID :one
SELECT *
FROM auctions
WHERE id = $1;