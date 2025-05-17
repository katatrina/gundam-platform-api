-- name: CreateAuctionBid :one
INSERT INTO auction_bids (id,
                          auction_id,
                          bidder_id,
                          participant_id,
                          amount)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetAuctionBidByID :one
SELECT *
FROM auction_bids
WHERE id = $1;

-- name: ListUserAuctionBids :many
SELECT *
FROM auction_bids
WHERE bidder_id = $1
  AND auction_id = $2
ORDER BY created_at DESC;