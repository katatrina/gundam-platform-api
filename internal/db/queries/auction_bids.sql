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