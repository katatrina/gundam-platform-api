-- name: CreateAuctionParticipant :one
INSERT INTO auction_participants (id,
                                  auction_id,
                                  user_id,
                                  deposit_amount,
                                  deposit_entry_id)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: IncrementAuctionParticipants :one
UPDATE auctions
SET total_participants = total_participants + 1
WHERE id = $1 RETURNING *;