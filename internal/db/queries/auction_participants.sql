-- name: CreateAuctionParticipant :one
INSERT INTO auction_participants (id,
                                  auction_id,
                                  user_id,
                                  deposit_amount,
                                  deposit_entry_id)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

